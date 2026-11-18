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
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// jobRankLimit 是 Top N 的条数；hotspotLimit 是每类算子热点的条数。
// jobRankLimit is the Top N size; hotspotLimit is the number of hotspots per kind.
const (
	jobRankLimit      = 8
	checkpointLimit   = 10
	hotspotLimit      = 5
	unknownPluginName = "?"
)

// ErrInvalidJobDashboardQuery 表示作业大盘参数非法。
// ErrInvalidJobDashboardQuery reports an invalid job dashboard parameter.
var ErrInvalidJobDashboardQuery = errors.New("dashboard: invalid job dashboard query")

// JobDashboardSource 提供作业摘要、运行日历与事件标注（sync 服务实现）。
// JobDashboardSource supplies job summaries, the run calendar and event markers (implemented by the sync service).
type JobDashboardSource interface {
	DescribeJobsForDashboard(ctx context.Context, actor executionapp.Actor, ids []uint) (map[uint]syncapp.JobBrief, error)
	JobRunCalendar(ctx context.Context, clusterID uint, weeks int, now time.Time) (*syncapp.JobCalendar, error)
	JobAnnotationsBetween(ctx context.Context, actor executionapp.Actor, clusterID uint, start, end time.Time) ([]syncapp.JobAnnotation, error)
}

// JobRank 是 Top N / Checkpoint / 热点里的一项；作业字段来自 DB，Viewable=false 时前端不可点开。
// JobRank is one Top N / checkpoint / hotspot entry; job fields come from the DB, and Viewable=false is display-only.
type JobRank struct {
	JobID      uint     `json:"job_id"`
	TaskID     uint     `json:"task_id,omitempty"`
	ClusterID  uint     `json:"cluster_id,omitempty"`
	Name       string   `json:"name"`
	Value      float64  `json:"value"`
	Viewable   bool     `json:"viewable"`
	MaxSeconds *float64 `json:"max_seconds,omitempty"`
	Failures   *float64 `json:"failures,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Vertex     string   `json:"vertex,omitempty"`
}

// FlowLink 是一条源连接器 → 目标连接器的写入量。
// FlowLink is the rows written from one source connector to one sink connector.
type FlowLink struct {
	Source string  `json:"source"`
	Sink   string  `json:"sink"`
	Value  float64 `json:"value"`
}

// ThroughputSeries 是全局吞吐的一条序列（read / write / read_bytes / write_bytes）。
// ThroughputSeries is one global throughput series (read / write / read_bytes / write_bytes).
type ThroughputSeries struct {
	Key    string       `json:"key"`
	Points [][2]float64 `json:"points"`
}

// JobDashboard 是 GET /api/v1/dashboard/home/jobs 的返回。聚合为全局口径，运行日历始终来自 DB。
// JobDashboard is the payload of GET /api/v1/dashboard/home/jobs. Aggregates are global; the run calendar always comes
// from the DB.
type JobDashboard struct {
	Source        string `json:"source"`
	Reason        string `json:"reason,omitempty"`
	Range         string `json:"range"`
	StartMs       int64  `json:"start_ms"`
	EndMs         int64  `json:"end_ms"`
	StepSeconds   int    `json:"step_seconds"`
	WindowSeconds int    `json:"window_seconds"`
	Throughput     []ThroughputSeries      `json:"throughput"`
	Annotations    []syncapp.JobAnnotation `json:"annotations"`
	TopWrite       []JobRank               `json:"top_write"`
	TopCDC         []JobRank               `json:"top_cdc"`
	Flow           []FlowLink              `json:"flow"`
	Checkpoints    []JobRank               `json:"checkpoints"`
	Hotspots       []JobRank               `json:"hotspots"`
	Calendar       *syncapp.JobCalendar    `json:"calendar"`
	PartialErrors  []string                `json:"partial_errors,omitempty"`
}

// JobDashboardService 组合 Prometheus 与 sync，生成首页作业大盘。
// JobDashboardService combines Prometheus with sync into the home job dashboard.
type JobDashboardService struct {
	jobs         JobDashboardSource
	now          func() time.Time
	rangeQuery   rangeQueryFunc
	instantQuery instantQueryFunc
	// instantLongQuery 走放宽超时引擎，供 increase(x[window]) 这类扫满窗口的即时查询。
	// instantLongQuery runs on the relaxed-timeout engine for instant queries whose range
	// selector spans the whole window (e.g. increase(x[1y])).
	instantLongQuery instantQueryFunc
}

// NewJobDashboardService 创建作业大盘服务。
// NewJobDashboardService creates the job dashboard service.
func NewJobDashboardService(jobs JobDashboardSource) *JobDashboardService {
	return &JobDashboardService{jobs: jobs, now: time.Now, rangeQuery: dashboardPromCache.wrapRange(promquery.Range), instantQuery: dashboardPromCache.wrapInstant(promquery.Instant), instantLongQuery: dashboardPromCache.wrapInstant(promquery.InstantLong)}
}

// GetJobDashboard 生成作业大盘。只有时间窗非法时返回错误；其余失败记入 PartialErrors。
// Top N / Checkpoint / 热点按整个时间窗聚合（平均速率、最大值），历史窗口与已结束作业也能出数。
// GetJobDashboard builds the job dashboard. Only an invalid window returns an error; other failures go to
// PartialErrors. Top N / checkpoints / hotspots aggregate over the whole window (average rate, maximum), so historic
// windows and finished jobs still show up.
func (s *JobDashboardService) GetJobDashboard(ctx context.Context, actor executionapp.Actor, clusterID uint, rangeKey string, custom *promquery.Window, weeks int) (*JobDashboard, error) {
	now := s.now().UTC()
	// 预设窗口的终点按缓存粒度取整，见 alignPresetNow。/ Preset window ends are aligned to the cache granularity; see alignPresetNow.
	rangeName, start, end, err := promquery.ResolveWindow(rangeKey, custom, alignPresetNow(now, custom))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJobDashboardQuery, err)
	}
	step := promquery.RangeSteps[rangeName]
	if step == 0 {
		step = promquery.AutoStep(end.Sub(start))
	}
	span := int(end.Sub(start).Seconds())
	out := &JobDashboard{
		Range: rangeName, StartMs: start.UnixMilli(), EndMs: end.UnixMilli(), StepSeconds: step, WindowSeconds: span,
		Throughput:     []ThroughputSeries{}, Annotations: []syncapp.JobAnnotation{}, TopWrite: []JobRank{}, TopCDC: []JobRank{},
		Flow: []FlowLink{}, Checkpoints: []JobRank{}, Hotspots: []JobRank{},
	}
	if s.jobs != nil {
		if cal, err := s.jobs.JobRunCalendar(ctx, clusterID, weeks, now); err != nil {
			out.PartialErrors = append(out.PartialErrors, "calendar")
		} else {
			out.Calendar = cal
		}
		if items, err := s.jobs.JobAnnotationsBetween(ctx, actor, clusterID, start, end); err != nil {
			out.PartialErrors = append(out.PartialErrors, "annotations")
		} else {
			out.Annotations = items
		}
	}

	if reason := promquery.UnavailableReason(); reason != "" {
		out.Source, out.Reason = promquery.SourceNone, reason
		return out, nil
	}
	out.Source = promquery.SourcePrometheus
	s.fillMetrics(ctx, out, actor, clusterID, start, end, step, span)
	return out, nil
}

// fillMetrics 并发执行全部 Prometheus 查询，再用 DB 补作业名并组装各区块。
// fillMetrics runs every Prometheus query concurrently, then fills job names from the DB and assembles the blocks.
func (s *JobDashboardService) fillMetrics(ctx context.Context, out *JobDashboard, actor executionapp.Actor, clusterID uint, start, end time.Time, step, span int) {
	sel := ""
	if clusterID > 0 {
		sel = fmt.Sprintf(`cluster_id=%s`, strconv.Quote(strconv.FormatUint(uint64(clusterID), 10)))
	}
	window := fmt.Sprintf("%ds", span)
	// 吞吐速率窗口取 max(1m, step)：长步长时覆盖整个取样间隔，不会漏掉峰值。
	// Throughput rate windows use max(1m, step): coarse steps cover the whole sampling
	// interval instead of missing peaks.
	rateWindow := promquery.RangeWindow(step, time.Minute)
	tasks := []*promTask{
		{key: "read", step: step, query: fmt.Sprintf(`sum(rate(stx_job_source_received_rows_total{%s}[%s]))`, sel, rateWindow)},
		{key: "write", step: step, query: fmt.Sprintf(`sum(rate(stx_job_sink_write_rows_total{%s}[%s]))`, sel, rateWindow)},
		{key: "read_bytes", step: step, query: fmt.Sprintf(`sum(rate(stx_job_source_received_bytes_total{%s}[%s]))`, sel, rateWindow)},
		{key: "write_bytes", step: step, query: fmt.Sprintf(`sum(rate(stx_job_sink_write_bytes_total{%s}[%s]))`, sel, rateWindow)},
		{key: "written", instant: true, query: fmt.Sprintf(`sum by (job_id) (increase(stx_job_sink_write_rows_total{%s}[%s]))`, sel, window)},
		{key: "cdc", instant: true, query: fmt.Sprintf(`topk(%d, max by (job_id) (max_over_time(stx_job_cdc_delay_seconds{%s}[%s])))`, jobRankLimit, sel, window)},
		{key: "ckpt_duration", instant: true, query: fmt.Sprintf(`topk(%d, max by (job_id) (max_over_time(stx_job_checkpoint_duration_seconds{%s}[%s])))`, checkpointLimit, sel, window)},
		{key: "ckpt_failed", instant: true, query: fmt.Sprintf(`sum by (job_id) (increase(stx_job_checkpoint_failed_total{%s}[%s]))`, sel, window)},
		{key: "busy", instant: true, query: fmt.Sprintf(`topk(%d, max by (job_id, vertex) (max_over_time(stx_job_vertex_busy_ratio{%s}[%s])))`, hotspotLimit, sel, window)},
		{key: "backpressure", instant: true, query: fmt.Sprintf(`topk(%d, max by (job_id, edge) (max_over_time(stx_job_edge_backpressure_ratio{%s}[%s])))`, hotspotLimit, sel, window)},
	}
	// 6 条窗口级即时查询（increase/max_over_time [window]）在超过 31 天的窗口下要扫满全部
	// 历史数据，必须走放宽超时的长引擎，否则生产密度下会超时进 partial_errors。
	// The six window-spanning instant queries (increase/max_over_time [window]) scan the full
	// history beyond the 31-day threshold, so they must run on the relaxed-timeout engine or
	// they time out into partial_errors under production density.
	instantFn := s.instantQuery
	if end.Sub(start) > promquery.LongWindowSpan {
		instantFn = s.instantLongQuery
	}
	var wg gosync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task *promTask) {
			defer wg.Done()
			if task.instant {
				task.samples, task.err = instantFn(ctx, task.query, end)
			} else {
				task.series, task.err = s.rangeQuery(ctx, task.query, start, end, task.step)
			}
		}(task)
	}
	wg.Wait()
	byKey := map[string]*promTask{}
	for _, task := range tasks {
		byKey[task.key] = task
		if task.err != nil {
			out.PartialErrors = append(out.PartialErrors, task.key)
		}
	}
	for _, key := range []string{"read", "write", "read_bytes", "write_bytes"} {
		points := [][2]float64{}
		if series := byKey[key].series; len(series) > 0 {
			points = series[0].Points
		}
		out.Throughput = append(out.Throughput, ThroughputSeries{Key: key, Points: points})
	}

	// 收集所有出现的 job_id，一次查 DB 补名称、插件与可见性。/ Gather every job_id and look them up in one DB call.
	idSet := map[uint]struct{}{}
	for _, key := range []string{"written", "cdc", "ckpt_duration", "ckpt_failed", "busy", "backpressure"} {
		for _, sample := range byKey[key].samples {
			if id := sampleJobID(sample); id > 0 {
				idSet[id] = struct{}{}
			}
		}
	}
	ids := make([]uint, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	briefs := map[uint]syncapp.JobBrief{}
	if s.jobs != nil && len(ids) > 0 {
		if found, err := s.jobs.DescribeJobsForDashboard(ctx, actor, ids); err != nil {
			out.PartialErrors = append(out.PartialErrors, "jobs")
		} else {
			briefs = found
		}
	}
	rank := func(sample promquery.Sample, value float64) JobRank {
		id := sampleJobID(sample)
		item := JobRank{JobID: id, Value: value, Name: "#" + strconv.FormatUint(uint64(id), 10)}
		if brief, ok := briefs[id]; ok {
			item.TaskID, item.ClusterID, item.Name, item.Viewable = brief.TaskID, brief.ClusterID, brief.Name, brief.Viewable
		}
		return item
	}

	// 写入 Top N：窗口内写入行数 / 窗口秒数 = 平均写入速率。/ Top writers: rows written in the window / window seconds.
	written := byKey["written"].samples
	sort.Slice(written, func(i, j int) bool { return written[i].Value > written[j].Value })
	for _, sample := range written {
		if sample.Value <= 0 || len(out.TopWrite) >= jobRankLimit {
			continue
		}
		out.TopWrite = append(out.TopWrite, rank(sample, sample.Value/float64(span)))
	}
	out.TopCDC = rankedSamples(byKey["cdc"].samples, rank, jobRankLimit)

	failures := map[uint]float64{}
	for _, sample := range byKey["ckpt_failed"].samples {
		failures[sampleJobID(sample)] = sample.Value
	}
	for _, item := range rankedSamples(byKey["ckpt_duration"].samples, rank, checkpointLimit) {
		maxSeconds, failed := item.Value, failures[item.JobID]
		item.MaxSeconds, item.Failures = &maxSeconds, &failed
		out.Checkpoints = append(out.Checkpoints, item)
	}
	for _, kind := range []struct{ key, label string }{{"busy", "vertex"}, {"backpressure", "edge"}} {
		samples := append([]promquery.Sample{}, byKey[kind.key].samples...)
		sort.Slice(samples, func(i, j int) bool { return samples[i].Value > samples[j].Value })
		for i, sample := range samples {
			if i >= hotspotLimit || sample.Value <= 0 {
				break
			}
			item := rank(sample, sample.Value)
			item.Kind, item.Vertex = kind.key, sample.Metric[kind.label]
			out.Hotspots = append(out.Hotspots, item)
		}
	}
	sort.SliceStable(out.Hotspots, func(i, j int) bool { return out.Hotspots[i].Value > out.Hotspots[j].Value })
	out.Flow = buildFlow(written, briefs)
}

// rankedSamples 按值降序取前 limit 个正值样本并转成 JobRank。
// rankedSamples takes the top limit positive samples by value and turns them into JobRank entries.
func rankedSamples(samples []promquery.Sample, rank func(promquery.Sample, float64) JobRank, limit int) []JobRank {
	sorted := append([]promquery.Sample{}, samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value > sorted[j].Value })
	out := []JobRank{}
	for _, sample := range sorted {
		if sample.Value <= 0 || len(out) >= limit {
			continue
		}
		out = append(out, rank(sample, sample.Value))
	}
	return out
}

// buildFlow 把每个作业的窗口写入行数分摊到它的源 → 目标插件对（多个目标时平均分摊），解析不到插件记为「?」。
// buildFlow spreads each job's rows written in the window over its source → sink plugin pairs (evenly across several
// sinks); unknown plugins become "?".
func buildFlow(written []promquery.Sample, briefs map[uint]syncapp.JobBrief) []FlowLink {
	totals := map[[2]string]float64{}
	for _, sample := range written {
		if sample.Value <= 0 {
			continue
		}
		brief := briefs[sampleJobID(sample)]
		source := unknownPluginName
		if len(brief.Sources) > 0 {
			source = strings.Join(brief.Sources, " + ")
		}
		sinks := brief.Sinks
		if len(sinks) == 0 {
			sinks = []string{unknownPluginName}
		}
		share := sample.Value / float64(len(sinks))
		for _, sink := range sinks {
			totals[[2]string{source, sink}] += share
		}
	}
	out := make([]FlowLink, 0, len(totals))
	for pair, value := range totals {
		out = append(out, FlowLink{Source: pair[0], Sink: pair[1], Value: value})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Source+out[i].Sink < out[j].Source+out[j].Sink
	})
	return out
}

// sampleJobID 读样本的 job_id 标签，非法时返回 0。
// sampleJobID reads the sample's job_id label, or 0 when it is invalid.
func sampleJobID(sample promquery.Sample) uint {
	id, err := strconv.ParseUint(strings.TrimSpace(sample.Metric["job_id"]), 10, 64)
	if err != nil {
		return 0
	}
	return uint(id)
}

// JobDashboardHandler 处理作业大盘接口。
// JobDashboardHandler serves the job dashboard endpoint.
type JobDashboardHandler struct {
	service *JobDashboardService
}

// NewJobDashboardHandler 创建作业大盘处理器。
// NewJobDashboardHandler creates the job dashboard handler.
func NewJobDashboardHandler(service *JobDashboardService) *JobDashboardHandler {
	return &JobDashboardHandler{service: service}
}

// GetJobDashboard 返回首页作业大盘。
// GetJobDashboard returns the home job dashboard.
// @Summary 首页作业大盘（吞吐 / Top N / 流向 / Checkpoint / 热点 / 运行日历）
// @Tags Dashboard
// @Produce json
// @Param cluster_id query int false "集群 ID，空表示全部集群"
// @Param range query string false "时间范围：15m|1h|6h|24h|7d|30d|90d|1y" default(1h)
// @Param start query string false "自定义起点：RFC3339 或秒 / 毫秒时间戳，优先于 range"
// @Param end query string false "自定义终点，缺省为当前时间；跨度不超过 366 天"
// @Param weeks query int false "运行日历周数（1~52）" default(16)
// @Success 200 {object} DashboardDataResponse{data=JobDashboard}
// @Failure 400 {object} DashboardDataResponse
// @Router /api/v1/dashboard/home/jobs [get]
func (h *JobDashboardHandler) GetJobDashboard(c *gin.Context) {
	var clusterID uint
	if raw := strings.TrimSpace(c.Query("cluster_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "invalid cluster_id"})
			return
		}
		clusterID = uint(parsed)
	}
	weeks := 16
	if raw := strings.TrimSpace(c.Query("weeks")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 52 {
			c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "weeks must be 1-52"})
			return
		}
		weeks = parsed
	}
	window, err := promquery.ParseWindow(c.Query("start"), c.Query("end"), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: err.Error()})
		return
	}
	actor := executionapp.Actor{}
	if user := auth.GetUserFromContext(c); user != nil {
		actor = executionapp.Actor{UserID: uint64(user.ID), IsAdmin: user.IsAdmin}
	}
	data, err := h.service.GetJobDashboard(c.Request.Context(), actor, clusterID, c.Query("range"), window, weeks)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidJobDashboardQuery) {
			status = http.StatusBadRequest
		}
		c.JSON(status, DashboardDataResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DashboardDataResponse{Data: data})
}
