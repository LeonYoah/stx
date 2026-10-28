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
	Source                      string                    `json:"source"`
	Reason                      string                    `json:"reason,omitempty"`
	Range                       string                    `json:"range"`
	StepSeconds                 int                       `json:"step_seconds"`
	EngineObservabilityEnabled  bool                      `json:"engine_observability_enabled"`
	Series                      []JobTimeseriesSeries     `json:"series"`
	Annotations                 []JobTimeseriesAnnotation `json:"annotations"`
}

// JobTimeseriesSeries 是一条命名时序。
// JobTimeseriesSeries is one named time series.
type JobTimeseriesSeries struct {
	Key    string      `json:"key"`
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

// GetJobTimeseriesForActor 在权限检查后返回作业时序（仅 Prometheus）。
// GetJobTimeseriesForActor returns job timeseries after enforcing permission (Prometheus only).
func (s *Service) GetJobTimeseriesForActor(ctx context.Context, actor executionapp.Actor, id uint, rangeKey, metricsCSV string, step int) (*JobTimeseriesResult, error) {
	instance, err := s.getJobInstanceForActor(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return s.getJobTimeseries(ctx, instance, rangeKey, metricsCSV, step)
}

func (s *Service) getJobTimeseries(ctx context.Context, instance *JobInstance, rangeKey, metricsCSV string, step int) (*JobTimeseriesResult, error) {
	rangeKey = strings.ToLower(strings.TrimSpace(rangeKey))
	if rangeKey == "" {
		rangeKey = "1h"
	}
	dur, ok := timeseriesRangeDurations[rangeKey]
	if !ok {
		return nil, fmt.Errorf("unsupported range %q (want 15m|1h|6h|24h|7d)", rangeKey)
	}
	if step <= 0 {
		step = timeseriesRangeSteps[rangeKey]
	}
	engineObs := jobHasEngineObservability(instance)
	out := &JobTimeseriesResult{
		Range:                      rangeKey,
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

	end := time.Now().UTC()
	start := end.Add(-dur)
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
		return []string{"throughput", "rows", "cdc_delay", "checkpoint", "busy", "backpressure"}
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
