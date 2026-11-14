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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
)

// JobTimeseriesResult 是 GET /sync/jobs/:id/timeseries 的返回；无本地兜底，source 仅为 prometheus 或 none。
// JobTimeseriesResult is the payload of GET /sync/jobs/:id/timeseries; there is no local fallback, so source is only
// "prometheus" or "none".
type JobTimeseriesResult struct {
	Source                     string                    `json:"source"`
	Reason                     string                    `json:"reason,omitempty"`
	Range                      string                    `json:"range"`
	StartMs                    int64                     `json:"start_ms"`
	EndMs                      int64                     `json:"end_ms"`
	StepSeconds                int                       `json:"step_seconds"`
	EngineObservabilityEnabled bool                      `json:"engine_observability_enabled"`
	Series                     []JobTimeseriesSeries     `json:"series"`
	Annotations                []JobTimeseriesAnnotation `json:"annotations"`
}

// JobTimeseriesSeries 是一条命名时序。
// JobTimeseriesSeries is one named time series.
type JobTimeseriesSeries struct {
	Key    string       `json:"key"`
	Label  string       `json:"label"`
	Unit   string       `json:"unit"`
	Points [][2]float64 `json:"points"` // [ts_ms, value]
}

// JobTimeseriesAnnotation 是叠加在图上的事件标记。
// JobTimeseriesAnnotation is an event marker overlaid on the charts.
type JobTimeseriesAnnotation struct {
	TsMs    int64  `json:"ts_ms"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	EventID uint   `json:"event_id,omitempty"`
}

type promQueryRangeResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

var timeseriesRangeSteps = map[string]int{
	"15m": 10,
	"1h":  15,
	"6h":  60,
	"24h": 300,
	"7d":  1800,
}

var timeseriesRangeDurations = map[string]time.Duration{
	"15m": 15 * time.Minute,
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

const (
	// timeseriesCustomRange 是自定义起止时间窗在响应里的 range 值。
	// timeseriesCustomRange is the response range value for a custom start/end window.
	timeseriesCustomRange = "custom"
	// timeseriesMaxCustomSpan 限制自定义时间窗跨度，避免一次拉过长的历史。
	// timeseriesMaxCustomSpan caps a custom window so one request cannot pull an unbounded history.
	timeseriesMaxCustomSpan = 31 * 24 * time.Hour
	// timeseriesTargetPoints 是自定义时间窗自动选 step 时每条序列的目标点数。
	// timeseriesTargetPoints is the per-series point budget used to pick a step for custom windows.
	timeseriesTargetPoints = 240
)

// timeseriesStepCandidates 是自定义时间窗可选的 step（秒），从细到粗。
// timeseriesStepCandidates are the steps (seconds) a custom window may use, finest first.
var timeseriesStepCandidates = []int{10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 14400}

// JobTimeseriesWindow 是自定义的起止时间窗（UTC）。
// JobTimeseriesWindow is a custom start/end window (UTC).
type JobTimeseriesWindow struct {
	Start time.Time
	End   time.Time
}

// ParseJobTimeseriesWindow 解析 start/end 查询参数：支持 RFC3339 与 Unix 时间戳（10 位及以下按秒，否则按毫秒）。
// 两者都为空返回 nil；只给 end 报错；只给 start 时 end 取 now。end 必须晚于 start，跨度不超过 31 天。
// ParseJobTimeseriesWindow parses the start/end query params: RFC3339 or Unix timestamps (10 digits or fewer are
// seconds, longer ones milliseconds). Both empty returns nil; end alone is an error; start alone ends at now. end must be
// after start and the span may not exceed 31 days.
func ParseJobTimeseriesWindow(startRaw, endRaw string, now time.Time) (*JobTimeseriesWindow, error) {
	startRaw, endRaw = strings.TrimSpace(startRaw), strings.TrimSpace(endRaw)
	if startRaw == "" && endRaw == "" {
		return nil, nil
	}
	if startRaw == "" {
		return nil, fmt.Errorf("start is required when end is set")
	}
	start, err := parseTimeseriesInstant(startRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid start: %w", err)
	}
	end := now.UTC()
	if endRaw != "" {
		if end, err = parseTimeseriesInstant(endRaw); err != nil {
			return nil, fmt.Errorf("invalid end: %w", err)
		}
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end must be after start")
	}
	if end.Sub(start) > timeseriesMaxCustomSpan {
		return nil, fmt.Errorf("window too long: at most %s", timeseriesMaxCustomSpan)
	}
	return &JobTimeseriesWindow{Start: start, End: end}, nil
}

// parseTimeseriesInstant 把单个时间参数解析成 UTC 时刻。
// parseTimeseriesInstant parses one time param into a UTC instant.
func parseTimeseriesInstant(raw string) (time.Time, error) {
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if len(strings.TrimPrefix(raw, "-")) <= 10 {
			return time.Unix(n, 0).UTC(), nil
		}
		return time.UnixMilli(n).UTC(), nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("want RFC3339 or unix timestamp, got %q", raw)
	}
	return t.UTC(), nil
}

// autoTimeseriesStep 为自定义时间窗挑一个 step，使每条序列不超过目标点数。
// autoTimeseriesStep picks a step for a custom window so each series stays within the point budget.
func autoTimeseriesStep(span time.Duration) int {
	for _, step := range timeseriesStepCandidates {
		if int(span/(time.Duration(step)*time.Second)) <= timeseriesTargetPoints {
			return step
		}
	}
	return timeseriesStepCandidates[len(timeseriesStepCandidates)-1]
}

// GetJobTimeseriesForActor 在权限检查后返回作业时序（仅 Prometheus）。window 非空时优先于 rangeKey。
// GetJobTimeseriesForActor returns job timeseries after enforcing permission (Prometheus only). A non-nil window wins
// over rangeKey.
func (s *Service) GetJobTimeseriesForActor(ctx context.Context, actor executionapp.Actor, id uint, rangeKey string, window *JobTimeseriesWindow, metricsCSV string, step int) (*JobTimeseriesResult, error) {
	instance, err := s.getJobInstanceForActor(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return s.getJobTimeseries(ctx, instance, rangeKey, window, metricsCSV, step)
}

func (s *Service) getJobTimeseries(ctx context.Context, instance *JobInstance, rangeKey string, window *JobTimeseriesWindow, metricsCSV string, step int) (*JobTimeseriesResult, error) {
	var start, end time.Time
	if window != nil {
		// 自定义时间窗：作业全程或日历选择的起止时刻。
		// Custom window: the job's lifetime or a range picked on the calendar.
		rangeKey = timeseriesCustomRange
		start, end = window.Start.UTC(), window.End.UTC()
		if step <= 0 {
			step = autoTimeseriesStep(end.Sub(start))
		}
	} else {
		rangeKey = strings.ToLower(strings.TrimSpace(rangeKey))
		if rangeKey == "" {
			rangeKey = "1h"
		}
		dur, ok := timeseriesRangeDurations[rangeKey]
		if !ok {
			return nil, fmt.Errorf("unsupported range %q (want 15m|1h|6h|24h|7d, or start/end)", rangeKey)
		}
		if step <= 0 {
			step = timeseriesRangeSteps[rangeKey]
		}
		end = time.Now().UTC()
		start = end.Add(-dur)
	}
	engineObs := jobHasEngineObservability(instance)
	out := &JobTimeseriesResult{
		Range:                      rangeKey,
		StartMs:                    start.UnixMilli(),
		EndMs:                      end.UnixMilli(),
		StepSeconds:                step,
		EngineObservabilityEnabled: engineObs,
		Series:                     []JobTimeseriesSeries{},
		Annotations:                []JobTimeseriesAnnotation{},
	}

	if config.Config == nil || !config.Config.Observability.Enabled {
		out.Source = "none"
		out.Reason = "observability_disabled"
		return out, nil
	}
	baseURL := strings.TrimSpace(config.Config.Observability.Prometheus.URL)
	if baseURL == "" {
		out.Source = "none"
		out.Reason = "prometheus_unavailable"
		return out, nil
	}

	jobID := strconv.FormatUint(uint64(instance.ID), 10)
	metricKeys := parseTimeseriesMetrics(metricsCSV)

	for _, key := range metricKeys {
		templates := timeseriesPromQL(key, jobID, engineObs)
		for _, tpl := range templates {
			vectors, err := queryPrometheusRange(ctx, baseURL, tpl.query, start, end, step)
			if err != nil {
				// Soft-fail per series so a single template error does not empty the whole response.
				// 单条模板失败不拖垮整份响应。
				continue
			}
			for _, vec := range vectors {
				label := tpl.label
				if name := strings.TrimSpace(vec.Metric["vertex"]); name != "" {
					label = name
				}
				if name := strings.TrimSpace(vec.Metric["edge"]); name != "" {
					label = name
				}
				if name := strings.TrimSpace(vec.Metric["table"]); name != "" {
					label = name
				}
				series := JobTimeseriesSeries{
					Key:    tpl.key,
					Label:  label,
					Unit:   tpl.unit,
					Points: make([][2]float64, 0, len(vec.Values)),
				}
				for _, sample := range vec.Values {
					if len(sample) < 2 {
						continue
					}
					ts, tsOK := coercePromSample(sample[0])
					val, valOK := coercePromSample(sample[1])
					if !tsOK || !valOK {
						continue
					}
					series.Points = append(series.Points, [2]float64{ts * 1000, val})
				}
				out.Series = append(out.Series, series)
			}
		}
	}

	out.Source = "prometheus"
	out.Annotations = s.loadTimeseriesAnnotations(ctx, instance, start, end)
	return out, nil
}

type promQLTemplate struct {
	key   string
	label string
	unit  string
	query string
}

func timeseriesPromQL(metricKey, jobID string, engineObs bool) []promQLTemplate {
	id := strconv.Quote(jobID)
	switch metricKey {
	case "throughput":
		return []promQLTemplate{
			{key: "throughput_read", label: "read", unit: "records/s",
				query: fmt.Sprintf(`sum(rate(stx_job_source_received_rows_total{job_id=%s}[1m]))`, id)},
			{key: "throughput_write", label: "write", unit: "records/s",
				query: fmt.Sprintf(`sum(rate(stx_job_sink_write_rows_total{job_id=%s}[1m]))`, id)},
		}
	case "rows":
		return []promQLTemplate{
			{key: "rows_read", label: "read", unit: "rows",
				query: fmt.Sprintf(`sum(stx_job_source_received_rows_total{job_id=%s})`, id)},
			{key: "rows_write", label: "write", unit: "rows",
				query: fmt.Sprintf(`sum(stx_job_sink_write_rows_total{job_id=%s})`, id)},
			{key: "rows_committed", label: "committed", unit: "rows",
				query: fmt.Sprintf(`sum(stx_job_sink_committed_rows_total{job_id=%s})`, id)},
		}
	case "cdc_delay":
		return []promQLTemplate{{
			key: "cdc_delay", label: "cdc_delay", unit: "seconds",
			query: fmt.Sprintf(`max(stx_job_cdc_delay_seconds{job_id=%s})`, id),
		}}
	case "checkpoint":
		return []promQLTemplate{
			{key: "checkpoint_duration", label: "duration", unit: "seconds",
				query: fmt.Sprintf(`max(stx_job_checkpoint_duration_seconds{job_id=%s})`, id)},
			{key: "checkpoint_failed", label: "failed", unit: "count",
				query: fmt.Sprintf(`sum(stx_job_checkpoint_failed_total{job_id=%s})`, id)},
		}
	case "table":
		// 表级指标来自作业指标采集（TableSourceReceivedCount/TableSinkWriteCount），与引擎可观测性无关，
		// 不受 engineObs 约束；表名在 table 标签上，直接作为 series label 返回。导出端对缺失方向补 0，
		// 会产生 Source[N]·写 / Sink[N]·读这类恒零镜像序列——按"该方向累计 > 0"过滤掉；吞吐序列用 and
		// 挂同一过滤，这样空闲的真实表仍显示为 0 而不消失。
		// Per-table metrics come from job metrics collection (TableSourceReceivedCount/TableSinkWriteCount),
		// independent of engine observability; the table label becomes the series label. The exporter fills
		// the missing direction with 0, which produces always-zero mirror series (Source[N]·write /
		// Sink[N]·read) — filtered by "cumulative > 0 on that direction"; throughput carries the same filter
		// via `and` so an idle real table still renders as 0 instead of disappearing.
		return []promQLTemplate{
			{key: "table_throughput_read", label: "read", unit: "records/s",
				query: fmt.Sprintf(`sum by (table) (rate(stx_job_table_rows_total{job_id=%s,direction="read"}[1m])) and on(table) (sum by (table) (stx_job_table_rows_total{job_id=%s,direction="read"}) > 0)`, id, id)},
			{key: "table_throughput_write", label: "write", unit: "records/s",
				query: fmt.Sprintf(`sum by (table) (rate(stx_job_table_rows_total{job_id=%s,direction="write"}[1m])) and on(table) (sum by (table) (stx_job_table_rows_total{job_id=%s,direction="write"}) > 0)`, id, id)},
			{key: "table_rows_read", label: "read", unit: "rows",
				query: fmt.Sprintf(`sum by (table) (stx_job_table_rows_total{job_id=%s,direction="read"}) > 0`, id)},
			{key: "table_rows_write", label: "write", unit: "rows",
				query: fmt.Sprintf(`sum by (table) (stx_job_table_rows_total{job_id=%s,direction="write"}) > 0`, id)},
		}
	case "busy":
		if !engineObs {
			return nil
		}
		return []promQLTemplate{{
			key: "busy", label: "busy", unit: "ratio",
			query: fmt.Sprintf(`stx_job_vertex_busy_ratio{job_id=%s}`, id),
		}}
	case "backpressure":
		if !engineObs {
			return nil
		}
		return []promQLTemplate{{
			key: "backpressure", label: "backpressure", unit: "ratio",
			query: fmt.Sprintf(`stx_job_edge_backpressure_ratio{job_id=%s}`, id),
		}}
	default:
		return nil
	}
}

func parseTimeseriesMetrics(csv string) []string {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return []string{"throughput", "rows", "cdc_delay", "checkpoint", "busy", "backpressure", "table"}
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func queryPrometheusRange(ctx context.Context, baseURL, promQL string, start, end time.Time, stepSeconds int) ([]struct {
	Metric map[string]string
	Values [][]interface{}
}, error) {
	apiURL := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/api/v1/query_range"
	params := url.Values{}
	params.Set("query", promQL)
	params.Set("start", strconv.FormatFloat(float64(start.Unix()), 'f', -1, 64))
	params.Set("end", strconv.FormatFloat(float64(end.Unix()), 'f', -1, 64))
	params.Set("step", strconv.Itoa(stepSeconds))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("prometheus query_range status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var payload promQueryRangeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Status), "success") {
		return nil, fmt.Errorf("prometheus query_range status %q: %s", payload.Status, strings.TrimSpace(payload.Error))
	}
	out := make([]struct {
		Metric map[string]string
		Values [][]interface{}
	}, 0, len(payload.Data.Result))
	for _, item := range payload.Data.Result {
		out = append(out, struct {
			Metric map[string]string
			Values [][]interface{}
		}{Metric: item.Metric, Values: item.Values})
	}
	return out, nil
}

func coercePromSample(raw interface{}) (float64, bool) {
	switch typed := raw.(type) {
	case float64:
		return typed, true
	case json.Number:
		v, err := typed.Float64()
		return v, err == nil
	case string:
		v, err := strconv.ParseFloat(typed, 64)
		return v, err == nil
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func (s *Service) loadTimeseriesAnnotations(ctx context.Context, instance *JobInstance, start, end time.Time) []JobTimeseriesAnnotation {
	if s == nil || s.repo == nil || instance == nil {
		return []JobTimeseriesAnnotation{}
	}
	// Reuse event listing with a generous page; filter by time client-side for simplicity.
	// 复用事件列表接口，页容量放大，时间过滤放在内存侧以保持实现简单。
	events, _, err := s.repo.ListEngineEventsByJob(ctx, instance.ID, nil, 1, 500)
	if err != nil {
		return []JobTimeseriesAnnotation{}
	}
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()
	out := make([]JobTimeseriesAnnotation, 0, 8)
	for _, ev := range events {
		if ev == nil {
			continue
		}
		ts := ev.EventTimeMs
		if ts < startMs || ts > endMs {
			continue
		}
		kind := strings.TrimSpace(ev.EventType)
		switch {
		case kind == EngineEventTypeJobStatus:
			kind = "status"
		case strings.HasPrefix(kind, "SCHEMA_CHANGE"):
			kind = "schema_change"
		case strings.Contains(kind, "LIFECYCLE") || strings.Contains(strings.ToLower(kind), "restart"):
			kind = "restart"
		case kind == EngineEventTypeMessageDelayed:
			continue // CDC delay is a series, not an annotation
		default:
			// keep original kind for other lifecycle events
		}
		summary := strings.TrimSpace(ev.EventType)
		if summary == "" {
			summary = kind
		}
		out = append(out, JobTimeseriesAnnotation{
			TsMs:    ts,
			Kind:    kind,
			Summary: summary,
			EventID: ev.ID,
		})
	}
	return out
}
