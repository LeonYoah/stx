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
package promquery

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

// RangeCustom 是自定义起止时间窗在响应里的 range 值。
// RangeCustom is the response range value for a custom start/end window.
const RangeCustom = "custom"

// MaxCustomSpan 限制自定义时间窗跨度（约一年），避免一次拉过长的历史。
// MaxCustomSpan caps a custom window at about a year so one request cannot pull an unbounded history.
const MaxCustomSpan = 366 * 24 * time.Hour

// targetPoints 是自定义时间窗自动选 step 时每条序列的目标点数。
// targetPoints is the per-series point budget used to pick a step for custom windows.
const targetPoints = 240

// trendGaugeWindow 是趋势图仪表面板的取样阈值：步长不超过它时用原始表达式（点足够密），
// 超过后必须包 *_over_time 才能覆盖整个取样间隔。
// trendGaugeWindow is the sampling threshold for trend gauge panels: steps at or below it keep
// the raw expression (points are dense enough); beyond it *_over_time is required to cover the
// whole sampling interval.
const trendGaugeWindow = 5 * time.Minute

// vectorSelectorRe 匹配一个向量选择器（metric{labels}）；标签值里不出现裸花括号。
// vectorSelectorRe matches one vector selector (metric{labels}); bare braces never appear
// inside label values.
var vectorSelectorRe = regexp.MustCompile(`[a-zA-Z_:][a-zA-Z0-9_:]*\{[^{}]*\}`)

// RangeWindow 返回速率/聚合窗口的 PromQL 时长字面量 max(base, step)：长步长时让速率窗口
// 覆盖整个取样间隔，避免 rate 只算到每点前几分钟而漏掉峰值。
// RangeWindow returns the PromQL duration literal max(base, step) for rate/aggregate windows:
// with coarse steps the window covers the whole sampling interval instead of only a few
// minutes before each point.
func RangeWindow(stepSeconds int, base time.Duration) string {
	span := time.Duration(stepSeconds) * time.Second
	if span < base {
		span = base
	}
	return model.Duration(span).String()
}

// SampleGauge 在步长超过 5 分钟时把表达式里的每个向量选择器包成 fn_over_time(sel[step])，
// 否则原样返回；fn 取 "avg"（均值面板）或 "max"（峰值面板）。已带窗口的表达式
// （rate/min_over_time 等）不应走这里。
// SampleGauge wraps every vector selector in expr as fn_over_time(sel[step]) when the step
// exceeds five minutes, otherwise returns expr verbatim; fn is "avg" (mean panels) or "max"
// (peak panels). Expressions that already carry windows (rate/min_over_time) must not pass through.
func SampleGauge(stepSeconds int, fn, expr string) string {
	step := time.Duration(stepSeconds) * time.Second
	if step <= trendGaugeWindow {
		return expr
	}
	win := model.Duration(step).String()
	return vectorSelectorRe.ReplaceAllString(expr, fmt.Sprintf("%s_over_time($0[%s])", fn, win))
}

// RangeDurations / RangeSteps 是预设时间窗及其默认 step（秒）。
// RangeDurations / RangeSteps are the preset windows and their default steps (seconds).
var (
	RangeDurations = map[string]time.Duration{
		"15m": 15 * time.Minute,
		"1h":  time.Hour,
		"6h":  6 * time.Hour,
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
		"90d": 90 * 24 * time.Hour,
		"1y":  365 * 24 * time.Hour,
	}
	RangeSteps = map[string]int{
		"15m": 10,
		"1h":  15,
		"6h":  60,
		"24h": 300,
		"7d":  1800,
		"30d": 14400,
		"90d": 43200,
		"1y":  172800,
	}
)

// stepCandidates 是自定义时间窗可选的 step（秒），从细到粗；长窗口档位支撑约一年的查询。
// stepCandidates are the steps (seconds) a custom window may use, finest first; the coarse tiers
// support year-long windows.
var stepCandidates = []int{10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 14400, 21600, 43200, 86400, 172800}

// Window 是一个 UTC 起止时间窗。
// Window is a UTC start/end window.
type Window struct {
	Start time.Time
	End   time.Time
}

// ParseWindow 解析 start/end 查询参数：支持 RFC3339 与 Unix 时间戳（10 位及以下按秒，否则按毫秒）。
// 两者都为空返回 nil；只给 end 报错；只给 start 时 end 取 now。end 必须晚于 start，跨度不超过 366 天。
// ParseWindow parses the start/end query params: RFC3339 or Unix timestamps (10 digits or fewer are seconds, longer
// ones milliseconds). Both empty returns nil; end alone is an error; start alone ends at now. end must be after start
// and the span may not exceed 366 days.
func ParseWindow(startRaw, endRaw string, now time.Time) (*Window, error) {
	startRaw, endRaw = strings.TrimSpace(startRaw), strings.TrimSpace(endRaw)
	if startRaw == "" && endRaw == "" {
		return nil, nil
	}
	if startRaw == "" {
		return nil, fmt.Errorf("start is required when end is set")
	}
	start, err := parseInstant(startRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid start: %w", err)
	}
	end := now.UTC()
	if endRaw != "" {
		if end, err = parseInstant(endRaw); err != nil {
			return nil, fmt.Errorf("invalid end: %w", err)
		}
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end must be after start")
	}
	if end.Sub(start) > MaxCustomSpan {
		return nil, fmt.Errorf("window too long: at most %s", MaxCustomSpan)
	}
	return &Window{Start: start, End: end}, nil
}

// ResolveWindow 把预设 range 或自定义窗口统一成起止时刻与 range 名；预设 range 为空时取 1h。
// ResolveWindow turns a preset range or a custom window into start/end plus the range name; an empty preset means 1h.
func ResolveWindow(rangeKey string, custom *Window, now time.Time) (string, time.Time, time.Time, error) {
	if custom != nil {
		return RangeCustom, custom.Start.UTC(), custom.End.UTC(), nil
	}
	rangeKey = strings.ToLower(strings.TrimSpace(rangeKey))
	if rangeKey == "" {
		rangeKey = "1h"
	}
	dur, ok := RangeDurations[rangeKey]
	if !ok {
		return "", time.Time{}, time.Time{}, fmt.Errorf("unsupported range %q (want 15m|1h|6h|24h|7d|30d|90d|1y|30d|90d|1y, or start/end)", rangeKey)
	}
	end := now.UTC()
	return rangeKey, end.Add(-dur), end, nil
}

// parseInstant 把单个时间参数解析成 UTC 时刻。
// parseInstant parses one time param into a UTC instant.
func parseInstant(raw string) (time.Time, error) {
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

// AutoStep 为任意时间窗挑一个 step，使每条序列不超过目标点数。
// AutoStep picks a step for any window so each series stays within the point budget.
func AutoStep(span time.Duration) int {
	for _, step := range stepCandidates {
		if int(span/(time.Duration(step)*time.Second)) <= targetPoints {
			return step
		}
	}
	return stepCandidates[len(stepCandidates)-1]
}
