/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sync

import (
	"context"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// stxSyncJobsJobLabel 是进程内作业指标注入的 job 标签值（沿用退役 prometheus.yml 的 job_name）。
// stxSyncJobsJobLabel is the job label injected into in-process job metrics (kept from the
// retired prometheus.yml job_name).
const stxSyncJobsJobLabel = "stx_sync_jobs"

// jobMetricsInjectedLabels 是写入 stx_job_* 序列时统一注入的标签。
// jobMetricsInjectedLabels are the labels injected into every stx_job_* series.
var jobMetricsInjectedLabels = map[string]string{"job": stxSyncJobsJobLabel}

// jobMetricsGatherExporter 把进程内 Registry 的 gather 结果周期直写内嵌 tsdb：
// 不再走 HTTP exposition 被外部 Prometheus 抓取的路径（该公开端点已删除）。
// jobMetricsGatherExporter periodically writes the in-process Registry gather output straight
// into the embedded tsdb: the old path—an HTTP exposition scraped by an external Prometheus—
// was removed together with the public endpoint.
type jobMetricsGatherExporter struct {
	gather func() ([]*dto.MetricFamily, error)
	// source 提供终态作业的延迟删除与新作业零值起点；为 nil 时（测试）两项都不做。
	// source provides deferred deletion of terminal jobs and zero-start samples for new jobs;
	// both are skipped when nil (tests).
	source *jobPrometheusExporter
	// tracker 记录上一轮写入的序列集合；终态作业序列被 deleteJob 摘掉后，下一轮导出补 stale marker，
	// 防止 PromQL 查询里幽灵续存。
	// tracker records the label sets written last round; once a terminal job's series are removed
	// by deleteJob, the next export writes stale markers so PromQL queries stop ghosting them.
	tracker *embedts.SeriesTracker
	// ts 是本写入方的样本时间戳水位线：同毫秒连续两轮导出（含 stale marker）必须错开时间戳。
	// ts is this writer's timestamp watermark: consecutive export rounds inside one
	// millisecond (including stale markers) must carry distinct timestamps.
	ts embedts.WriteTsWatermark
}

// StartJobMetricsExporter 启动「gather → 内嵌 tsdb」导出协程；observability.enabled=false 或
// 内嵌 tsdb 未就绪时直接返回（协程都不起）。随传入 ctx 取消而停止。
// StartJobMetricsExporter starts the "gather → embedded tsdb" export goroutine; it returns
// immediately without launching anything when observability is off or the embedded tsdb is
// not ready. The goroutine stops with ctx cancellation.
func StartJobMetricsExporter(ctx context.Context) {
	if config.Config == nil || !config.Config.Observability.Enabled || !embedts.Ready() {
		return
	}
	opts, err := embedts.ScrapeOptionsFromObservability(config.Config.Observability)
	interval := embedts.DefaultScrapeInterval
	if err != nil {
		logger.WarnF(ctx, "[Observability] scrape_interval 解析失败，作业指标导出回退默认 %s: %v / failed to parse scrape_interval for job metrics export, falling back to %s: %v", interval, err, interval, err)
	} else {
		interval = opts.Interval
	}
	source := getJobPrometheusExporter()
	source.enableDeferredDeletes()
	exporter := &jobMetricsGatherExporter{
		gather:  source.reg.Gather,
		source:  source,
		tracker: embedts.NewSeriesTracker(),
	}
	go exporter.run(ctx, interval)
	logger.InfoF(ctx, "[Observability] stx_job_* 指标直写协程已启动: interval=%s / stx_job_* metrics direct-write loop started: interval=%s", interval, interval)
}

// run 是导出循环：每 interval 抓一次 Registry 并写入 tsdb。
// run is the export loop: it gathers the Registry and writes to tsdb every interval.
func (e *jobMetricsGatherExporter) run(ctx context.Context, interval time.Duration) {
	e.exportOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.exportOnce(ctx)
		}
	}
}

// exportOnce 执行一轮导出：Gather → 展平 → 打 job 标签 → 同一 appender 提交（含 up 与 stale marker）。
// tsdb 未就绪的轮次直接跳过（不写不报错），下次就绪后继续。
// exportOnce performs one export round: Gather → flatten → stamp the job label → commit on a
// single appender (including up and stale markers). Rounds where the tsdb is not ready are
// skipped silently; export resumes once it is back.
//
// 终态作业：先取走待删快照，再 gather 写入，最后才删序列——快照内作业的终值至少写入一次，
// 快照之后登记的留到下一轮。新作业的累计计数器序列首次写入前补一个零值起点样本，
// 让只被看到一两次的短作业也能算出 rate / increase。
// Terminal jobs: the pending-deletion snapshot is taken first, then the gather is written, and
// only then are the series removed—so every job in the snapshot has its final values written at
// least once, while jobs queued afterwards wait for the next round. Cumulative counter series of
// new jobs get a zero-start sample before their first write so short jobs seen only once or
// twice still yield rate / increase.
func (e *jobMetricsGatherExporter) exportOnce(ctx context.Context) {
	pending := e.source.takePendingDeletes()
	defer e.source.applyPendingDeletes(pending)

	app := embedts.Appender(ctx)
	if app == nil {
		return
	}
	tsMs := e.ts.NextNow()
	mfs, err := e.gather()
	if err != nil {
		logger.ErrorF(ctx, "[Observability] 作业指标 gather 失败: %v / job metrics gather failed: %v", err, err)
		return
	}
	samples := embedts.FlattenMetricFamilies(mfs, jobMetricsInjectedLabels)

	// 本轮写入的序列集合（含 up），交给 tracker 做消失判定。
	// This round's written series (including up) feed the disappearance check.
	written := make([]labels.Labels, 0, len(samples)+1)
	appendErr := false
	for _, sample := range samples {
		e.appendZeroStart(app, sample.Labels, tsMs)
		if _, err := app.Append(0, sample.Labels, tsMs, sample.Value); err != nil {
			appendErr = true
			continue
		}
		written = append(written, sample.Labels)
	}
	upLset := labels.FromStrings(labels.MetricName, "up", "job", stxSyncJobsJobLabel)
	if _, err := app.Append(0, upLset, tsMs, 1.0); err != nil {
		appendErr = true
	} else {
		written = append(written, upLset)
	}
	for _, lset := range e.tracker.Diff(written) {
		if _, err := app.Append(0, lset, tsMs, embedts.StaleNaNValue); err != nil {
			appendErr = true
		}
	}
	if err := app.Commit(); err != nil {
		logger.ErrorF(ctx, "job metrics export commit failed: %v", err)
	} else if appendErr {
		logger.WarnF(ctx, "[Observability] 作业指标导出部分样本写入失败（已提交其余样本）/ job metrics export had partial append failures (remaining samples committed)")
	}
}

// appendZeroStart 对新作业首次出现的累计计数器序列，在作业开始时刻（不晚于 tsMs-1）补一个 0 样本。
// 失败（如超出 head 可写下界）只放弃这个 0 样本，不影响本轮其余写入。
// appendZeroStart writes a 0 sample at the job start (no later than tsMs-1) for a new job's
// cumulative counter series on its first appearance. A failure (e.g. below the head's
// appendable bound) only drops that zero sample and does not affect the rest of the round.
func (e *jobMetricsGatherExporter) appendZeroStart(app storage.Appender, lset labels.Labels, tsMs int64) {
	if e.source == nil || !strings.HasSuffix(lset.Get(labels.MetricName), "_total") || e.tracker.Seen(lset) {
		return
	}
	zeroTs, ok := e.source.zeroStartFor(lset.Get("job_id"))
	if !ok {
		return
	}
	if zeroTs >= tsMs {
		zeroTs = tsMs - 1
	}
	_, _ = app.Append(0, lset, zeroTs, 0)
}
