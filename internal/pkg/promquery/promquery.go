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
// Package promquery 是 STX 后端执行 PromQL 的公共入口：区间 / 即时查询、时间窗解析与可用性判定。
// 内嵌化后查询走 internal/pkg/embedts 的进程内 tsdb + promql.Engine；PromQL 模板由各业务模块持有，
// 前端不直连存储引擎。
// Package promquery is the shared entry point STX uses to run PromQL: range / instant queries,
// window parsing and availability checks. Since the embedding, queries run against the in-process
// tsdb + promql.Engine managed by internal/pkg/embedts; PromQL templates stay in each feature module
// and the browser never talks to the storage engine directly.
package promquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

// 数据来源与不可用原因，和作业趋势接口的 source / reason 取值一致。
// Data source and unavailability reasons, matching the job timeseries source / reason values.
const (
	SourcePrometheus = "prometheus"
	SourceNone       = "none"

	ReasonObservabilityDisabled = "observability_disabled"
	ReasonPrometheusUnavailable = "prometheus_unavailable"
)

// errNotReady 是引擎未就绪时 Range/Instant 返回的错误；调用方本就按错误走降级路径。
// errNotReady is returned by Range/Instant while the engine is not ready; callers already
// degrade on any error.
var errNotReady = errors.New("embedded tsdb is not ready")

// Series 是一条区间查询结果；Points 为 [毫秒时间戳, 值]。
// Series is one range query result; Points are [epoch ms, value].
type Series struct {
	Metric map[string]string
	Points [][2]float64
}

// Sample 是一条即时查询结果。
// Sample is one instant query result.
type Sample struct {
	Metric map[string]string
	TsMs   int64
	Value  float64
}

// UnavailableReason 返回查询不可用原因：observability 未启用返回 observability_disabled；
// 内嵌引擎未就绪返回 prometheus_unavailable；可用时返回空串。
// UnavailableReason reports why observability queries cannot run: observability_disabled when the
// feature is off, prometheus_unavailable while the embedded engine is not ready, and "" once up.
func UnavailableReason() string {
	if config.Config == nil || !config.Config.Observability.Enabled {
		return ReasonObservabilityDisabled
	}
	if !embedts.Ready() {
		return ReasonPrometheusUnavailable
	}
	return ""
}

// longRangeSpan 是长窗口阈值：跨度超过它时改用放宽超时（query_timeout ×4）的引擎，
// 一年窗口不会被默认 8s 截断；短窗口行为不变。
// longRangeSpan is the long-window threshold: spans beyond it run on the relaxed-timeout
// engine (query_timeout ×4) so a year window is not cut by the default 8s; short windows unchanged.
const longRangeSpan = 31 * 24 * time.Hour

// Range 执行区间查询（query_range 语义）。
// Range runs a range query (query_range semantics).
func Range(ctx context.Context, promQL string, start, end time.Time, stepSeconds int) ([]Series, error) {
	engine, queryable := embedts.QueryEngine()
	if end.Sub(start) > longRangeSpan {
		engine, queryable = embedts.QueryEngineLong()
	}
	if engine == nil || queryable == nil {
		return nil, errNotReady
	}
	qry, err := engine.NewRangeQuery(ctx, queryable, nil, promQL, start.UTC(), end.UTC(), time.Duration(stepSeconds)*time.Second)
	if err != nil {
		return nil, err
	}
	defer qry.Close()
	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, res.Err
	}
	logWarnings(ctx, promQL, res.Warnings)
	matrix, ok := res.Value.(promql.Matrix)
	if !ok {
		return nil, fmt.Errorf("embedded range query returned %s instead of matrix", valueTypeName(res.Value))
	}
	out := make([]Series, 0, len(matrix))
	for _, item := range matrix {
		series := Series{Metric: item.Metric.Map(), Points: make([][2]float64, 0, len(item.Floats))}
		for _, point := range item.Floats {
			// FPoint.T 即毫秒时间戳，与旧 HTTP 解析输出形状一致。
			// FPoint.T is already the epoch-ms timestamp, matching the old HTTP decode shape.
			series.Points = append(series.Points, [2]float64{float64(point.T), point.F})
		}
		out = append(out, series)
	}
	return out, nil
}

// Instant 执行即时查询（at 为零值时取当前时间）。
// Instant runs an instant query (now when at is zero).
func Instant(ctx context.Context, promQL string, at time.Time) ([]Sample, error) {
	engine, queryable := embedts.QueryEngine()
	if engine == nil || queryable == nil {
		return nil, errNotReady
	}
	if at.IsZero() {
		at = time.Now()
	}
	qry, err := engine.NewInstantQuery(ctx, queryable, nil, promQL, at.UTC())
	if err != nil {
		return nil, err
	}
	defer qry.Close()
	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, res.Err
	}
	logWarnings(ctx, promQL, res.Warnings)
	vector, ok := res.Value.(promql.Vector)
	if !ok {
		return nil, fmt.Errorf("embedded instant query returned %s instead of vector", valueTypeName(res.Value))
	}
	out := make([]Sample, 0, len(vector))
	for _, sample := range vector {
		out = append(out, Sample{Metric: sample.Metric.Map(), TsMs: sample.T, Value: sample.F})
	}
	return out, nil
}

// logWarnings 把查询警告落到日志；警告不致命，查询结果照常返回。
// logWarnings records query warnings; warnings are non-fatal and results are still returned.
func logWarnings(ctx context.Context, promQL string, warnings annotations.Annotations) {
	if len(warnings) == 0 {
		return
	}
	logger.WarnF(ctx, "[promquery] 查询警告 expr=%q warnings=%v / query warnings expr=%q warnings=%v", promQL, warnings.AsErrors(), promQL, warnings.AsErrors())
}

// valueTypeName 安全地返回结果值类型名，供错误信息使用。
// valueTypeName safely names a result value type for error messages.
func valueTypeName(v parser.Value) string {
	if v == nil {
		return "none"
	}
	return string(v.Type())
}

// CoerceSample 把 Prometheus 返回的时间戳或值（数字或字符串）转成 float64。
// CoerceSample converts a Prometheus timestamp or value (number or string) to float64.
func CoerceSample(raw interface{}) (float64, bool) {
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

// QuoteLabelRegex 把一组 ID 拼成 PromQL 正则标签值（"1|2|3"），只接受数字，杜绝注入。
// QuoteLabelRegex joins IDs into a PromQL regex label value ("1|2|3"); numbers only, so nothing can be injected.
func QuoteLabelRegex(ids []uint) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strconv.Quote(strings.Join(parts, "|"))
}
