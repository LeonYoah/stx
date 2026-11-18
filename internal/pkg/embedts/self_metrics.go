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
package embedts

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"

	"github.com/LeonYoah/stx/internal/logger"
)

const (
	// JobEmbedded 是内嵌 tsdb 自抓取写入的 job 标签（AC1）。
	// JobEmbedded is the job label stamped onto self-scraped series (AC1).
	JobEmbedded = "stx_embedded"
	// InstanceEmbedded 是内嵌自抓取的 instance 标签：单进程只有一份存储。
	// InstanceEmbedded is the instance label for self-scrapes: one process holds one store.
	InstanceEmbedded = "stx"
)

// errSelfNotReady 是内嵌 tsdb 未启动时 SelfScrapeOnce / GetStorageStatus 返回的哨兵错误。
// errSelfNotReady is the sentinel SelfScrapeOnce / GetStorageStatus return while the store is down.
var errSelfNotReady = errors.New("embedded tsdb is not ready")

var (
	selfMu      sync.Mutex
	selfTracker = NewSeriesTracker()
	selfTs      WriteTsWatermark
)

// selfInjectedLabels 是自抓取样本统一注入的标签。
// selfInjectedLabels are the labels injected into every self-scraped sample.
var selfInjectedLabels = map[string]string{
	"job":      JobEmbedded,
	"instance": InstanceEmbedded,
}

// StorageStatus 是内嵌 tsdb 的存储快照，供监控状态接口 / CLI 读取（AC2）。
// StorageStatus is a snapshot of embedded tsdb storage, read by integration status / CLI (AC2).
type StorageStatus struct {
	// HeadSeries 是 head 中仍跟踪的序列数。 / HeadSeries is the number of series still tracked in the head.
	HeadSeries uint64
	// HeadStaleSeries 是 head 中已写 stale 标记的序列数。 / HeadStaleSeries is the number of stale series in the head.
	HeadStaleSeries uint64
	// HeadBytes 是 head（WAL + 内存块）占用字节数。 / HeadBytes is the head (WAL + in-memory chunks) size in bytes.
	HeadBytes int64
	// BlocksBytes 是已持久化块占用字节数。 / BlocksBytes is the size of persisted blocks in bytes.
	BlocksBytes int64
	// OldestSampleMs 是最早样本的 Unix 毫秒时间戳；HasOldest 为 false 时忽略。
	// OldestSampleMs is the oldest sample's Unix millisecond timestamp; ignore when HasOldest is false.
	OldestSampleMs int64
	// HasOldest 表示 OldestSampleMs 是否有效。 / HasOldest reports whether OldestSampleMs is valid.
	HasOldest bool
	// CompactionsFailed 是 compaction 失败累计次数。 / CompactionsFailed is the compaction-failure counter.
	CompactionsFailed float64
	// SizeRetentions 是按容量截断累计次数。 / SizeRetentions is the size-based retention truncation counter.
	SizeRetentions float64
}

// registerRuntimeCollectors 把 Go runtime / 进程采集器挂到内嵌 registry，供自抓取观测 heap / RSS / GC。
// registerRuntimeCollectors attaches the Go runtime / process collectors to the embedded registry
// so self-scrapes can observe heap / RSS / GC.
func registerRuntimeCollectors(reg *prometheus.Registry) {
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

// RegisterSelfCollectors 把平台自监控采集器（selfmetrics / DBStats 等）注册进内嵌
// registry，供自抓取写回 tsdb。必须在 Start 成功之后调用；重复注册同名采集器静默
// 跳过，未就绪返回 errSelfNotReady。
// RegisterSelfCollectors registers platform self-monitoring collectors (selfmetrics /
// DBStats etc.) into the embedded registry so self-scrapes write them back. Call it
// only after Start succeeded; duplicate registrations are skipped silently, and a
// not-ready store returns errSelfNotReady.
func RegisterSelfCollectors(cs ...prometheus.Collector) error {
	inst := snapshot()
	if inst == nil || inst.registry == nil {
		return errSelfNotReady
	}
	for _, c := range cs {
		if c == nil {
			continue
		}
		if err := inst.registry.Register(c); err != nil {
			var already prometheus.AlreadyRegisteredError
			if errors.As(err, &already) {
				continue
			}
			return err
		}
	}
	return nil
}

// SelfScrapeOnce 对内嵌 registry 执行一次 Gather → Flatten → 写入 tsdb（job=stx_embedded）。
// 抓取循环每轮都会调用；测试也可单独调用。未就绪返回 errSelfNotReady。
// SelfScrapeOnce gathers the embedded registry, flattens it and writes into the tsdb with
// job=stx_embedded. The scrape loop calls it every round; tests may call it directly.
// It returns errSelfNotReady while the store is down.
func SelfScrapeOnce(ctx context.Context) error {
	inst := snapshot()
	if inst == nil || inst.registry == nil {
		return errSelfNotReady
	}
	mfs, err := inst.registry.Gather()
	if err != nil {
		return fmt.Errorf("gather embedded registry: %w", err)
	}
	samples := FlattenMetricFamilies(mfs, selfInjectedLabels)

	app := Appender(ctx)
	if app == nil {
		return errSelfNotReady
	}

	selfMu.Lock()
	defer selfMu.Unlock()
	tsMs := selfTs.NextNow()
	if err := AppendFlatSamples(app, samples, tsMs); err != nil {
		_ = app.Rollback()
		return fmt.Errorf("append self-scrape samples: %w", err)
	}

	written := make([]labels.Labels, 0, len(samples)+1)
	for _, sample := range samples {
		written = append(written, sample.Labels)
	}
	upLset := buildSeriesLabels("up", nil, nil, selfInjectedLabels)
	if _, err := app.Append(0, upLset, tsMs, 1); err != nil {
		_ = app.Rollback()
		return fmt.Errorf("append self-scrape up: %w", err)
	}
	written = append(written, upLset)

	for _, lset := range selfTracker.Diff(written) {
		if _, err := app.Append(0, lset, tsMs, StaleNaNValue); err != nil {
			_ = app.Rollback()
			return fmt.Errorf("append self-scrape stale marker: %w", err)
		}
	}
	return app.Commit()
}

// GetStorageStatus 返回当前内嵌存储快照；未就绪时 ok=false。
// GetStorageStatus returns the live embedded storage snapshot; ok is false while not ready.
func GetStorageStatus() (StorageStatus, bool) {
	inst := snapshot()
	if inst == nil || inst.db == nil {
		return StorageStatus{}, false
	}
	out := StorageStatus{}
	if head := inst.db.Head(); head != nil {
		out.HeadSeries = head.NumSeries()
		out.HeadStaleSeries = head.NumStaleSeries()
		out.HeadBytes = head.Size()
	}
	for _, block := range inst.db.Blocks() {
		if block == nil {
			continue
		}
		out.BlocksBytes += block.Size()
	}
	if oldest, ok := OldestSampleTime(); ok {
		out.HasOldest = true
		out.OldestSampleMs = oldest.UnixMilli()
	}
	if inst.registry != nil {
		out.CompactionsFailed = registryCounterValue(inst.registry, "prometheus_tsdb_compactions_failed_total")
		out.SizeRetentions = registryCounterValue(inst.registry, "prometheus_tsdb_size_retentions_total")
	}
	return out, true
}

// Compact 触发一次 head/block compaction（基准测试跨过 compaction 时使用）。未就绪返回错误。
// Compact triggers one head/block compaction (used by capacity benches to cross a compaction).
// It errors while the store is not ready.
func Compact(ctx context.Context) error {
	inst := snapshot()
	if inst == nil || inst.db == nil {
		return errSelfNotReady
	}
	return inst.db.Compact(ctx)
}

// CompactHead 把 [mint, maxt] 范围内的 head 落成块，供基准测试强制跨过一次 compaction。
// CompactHead persists the head in [mint, maxt] into a block so capacity benches can force one compaction.
func CompactHead(mint, maxt int64) error {
	inst := snapshot()
	if inst == nil || inst.db == nil {
		return errSelfNotReady
	}
	return inst.db.CompactHead(tsdb.NewRangeHead(inst.db.Head(), mint, maxt))
}

// resetSelfMetricsTracker 在 Close 时清空自抓取追踪器，避免重开目录后对旧序列补 stale。
// resetSelfMetricsTracker clears the self-scrape tracker on Close so a reopen does not stale-mark old series.
func resetSelfMetricsTracker() {
	selfMu.Lock()
	defer selfMu.Unlock()
	selfTracker.Reset()
}

// registryCounterValue 从 registry 读取指定 counter 的合计值；缺失时返回 0。
// registryCounterValue sums one named counter from the registry; missing metrics read as 0.
func registryCounterValue(reg *prometheus.Registry, name string) float64 {
	if reg == nil {
		return 0
	}
	mfs, err := reg.Gather()
	if err != nil {
		return 0
	}
	return sumCounterFamilies(mfs, name)
}

// sumCounterFamilies 合计指定名称的 counter / gauge 值。
// sumCounterFamilies sums counter / gauge values of the named metric family.
func sumCounterFamilies(mfs []*dto.MetricFamily, name string) float64 {
	var sum float64
	for _, mf := range mfs {
		if mf == nil || mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if m == nil {
				continue
			}
			if c := m.GetCounter(); c != nil {
				sum += c.GetValue()
			}
			if g := m.GetGauge(); g != nil {
				sum += g.GetValue()
			}
		}
	}
	return sum
}

// logSelfScrapeError 把自抓取失败记成 WARN，不阻断目标抓取轮次。
// logSelfScrapeError records a self-scrape failure as WARN without aborting the target scrape round.
func logSelfScrapeError(ctx context.Context, err error) {
	if err == nil || errors.Is(err, errSelfNotReady) {
		return
	}
	logger.WarnF(ctx, "[Observability] 内嵌 tsdb 自抓取失败: %v / embedded tsdb self-scrape failed: %v", err, err)
}
