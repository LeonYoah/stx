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

package diagnostics

import (
	"fmt"
	"html/template"
	"math"
	"sort"
	"strings"
	"time"
)

// diagnosticBundleHTMLMetricEvidence 保存某个明确的信号和实例的真实采样；不通过标题推测关联。
// diagnosticBundleHTMLMetricEvidence keeps real samples for an explicit signal and instance, never an inferred title match.
type diagnosticBundleHTMLMetricEvidence struct {
	SignalKey     string                      `json:"signal_key"`
	Instance      string                      `json:"instance"`
	Threshold     float64                     `json:"threshold"`
	ThresholdText string                      `json:"threshold_text"`
	Comparator    string                      `json:"comparator"`
	Unit          string                      `json:"unit"`
	LastValue     string                      `json:"last_value"`
	LastAt        string                      `json:"last_at"`
	WindowStart   time.Time                   `json:"window_start"`
	WindowEnd     time.Time                   `json:"window_end"`
	FocusStart    time.Time                   `json:"focus_start"`
	FocusEnd      time.Time                   `json:"focus_end"`
	FirstBreach   *time.Time                  `json:"first_breach,omitempty"`
	HasTrend      bool                        `json:"has_trend"`
	HasGaps       bool                        `json:"has_gaps"`
	StepSeconds   int                         `json:"step_seconds"`
	Points        []diagnosticPrometheusPoint `json:"points"`
	FocusPoints   []diagnosticPrometheusPoint `json:"focus_points"`
}

// buildDiagnosticMetricFindings 把采集到的越阈指标作为独立发现项；每项只关联自己的实例与采样序列。
// buildDiagnosticMetricFindings creates a finding per breached metric instance, using only its own collected series.
func buildDiagnosticMetricFindings(snapshot *diagnosticPrometheusSnapshot) []diagnosticBundleHTMLFindingCard {
	if snapshot == nil {
		return nil
	}
	findings := make([]diagnosticBundleHTMLFindingCard, 0)
	for _, signal := range snapshot.Signals {
		severity := strings.ToLower(strings.TrimSpace(signal.Status))
		if severity != "critical" && severity != "warning" {
			continue
		}
		for _, series := range signal.Series {
			points := diagnosticSortedMetricPoints(series.Points, snapshot.WindowStart, snapshot.WindowEnd)
			breachAt := diagnosticFirstMetricBreach(points, signal.Comparator, signal.Threshold)
			// 汇总状态属于信号，不能把其他实例的越阈状态误写到当前实例。
			// Signal-level status must not be attributed to an instance whose own samples never breached.
			if breachAt == nil {
				continue
			}
			instance := strings.TrimSpace(series.Instance)
			metric := &diagnosticBundleHTMLMetricEvidence{
				SignalKey:     signal.Key,
				Instance:      instance,
				Threshold:     signal.Threshold,
				ThresholdText: strings.TrimSpace(signal.ThresholdText),
				Comparator:    signal.Comparator,
				Unit:          signal.Unit,
				LastValue:     formatDiagnosticMetricValue(signal.Unit, series.LastValue),
				LastAt:        formatDiagnosticBundleTime(series.LastAt),
				WindowStart:   snapshot.WindowStart,
				WindowEnd:     snapshot.WindowEnd,
				FirstBreach:   breachAt,
				StepSeconds:   snapshot.StepSeconds,
				Points:        points,
				FocusPoints:   []diagnosticPrometheusPoint{},
			}
			metric.HasTrend, metric.HasGaps = diagnosticMetricContinuity(points, snapshot.StepSeconds)
			if len(points) > 0 {
				metric.LastValue = formatDiagnosticMetricValue(signal.Unit, points[len(points)-1].Value)
				metric.LastAt = formatDiagnosticBundleTimeValue(points[len(points)-1].Timestamp)
			}
			metric.FocusStart = breachAt.Add(-10 * time.Minute)
			metric.FocusEnd = breachAt.Add(10 * time.Minute)
			if metric.FocusStart.Before(snapshot.WindowStart) {
				metric.FocusStart = snapshot.WindowStart
			}
			if metric.FocusEnd.After(snapshot.WindowEnd) {
				metric.FocusEnd = snapshot.WindowEnd
			}
			for _, point := range points {
				if !point.Timestamp.Before(metric.FocusStart) && !point.Timestamp.After(metric.FocusEnd) {
					metric.FocusPoints = append(metric.FocusPoints, point)
				}
			}
			focusTrend, _ := diagnosticMetricContinuity(metric.FocusPoints, snapshot.StepSeconds)
			if !focusTrend {
				metric.FocusPoints = nil
			}
			if !metric.WindowStart.Before(metric.WindowEnd) && len(points) > 1 {
				metric.WindowStart = points[0].Timestamp
				metric.WindowEnd = points[len(points)-1].Timestamp
			}
			observed := fmt.Sprintf("%s · %s · %s", metric.LastValue, metric.LastAt, metric.ThresholdText)
			if metric.ThresholdText == "" {
				observed = fmt.Sprintf("%s · %s", metric.LastValue, metric.LastAt)
			}
			findings = append(findings, diagnosticBundleHTMLFindingCard{
				Severity: severity,
				Category: "resource",
				Title:    signal.Title,
				Summary: bilingualText(
					fmt.Sprintf("实例 %s 在本次采样窗口内越过阈值。", instance),
					fmt.Sprintf("Instance %s crossed the threshold in this sampling window.", instance),
				),
				Evidence:  observed,
				Origin:    instance,
				CheckCode: signal.Key,
				Metric:    metric,
			})
		}
	}
	return findings
}

// diagnosticSortedMetricPoints 过滤非数值和窗口外的采样点，保持时间轴真实。
// diagnosticSortedMetricPoints discards invalid or out-of-window samples and preserves their actual timestamps.
func diagnosticSortedMetricPoints(points []diagnosticPrometheusPoint, start, end time.Time) []diagnosticPrometheusPoint {
	result := make([]diagnosticPrometheusPoint, 0, len(points))
	for _, point := range points {
		if point.Timestamp.IsZero() || math.IsNaN(point.Value) || math.IsInf(point.Value, 0) {
			continue
		}
		if start.Before(end) && (point.Timestamp.Before(start) || point.Timestamp.After(end)) {
			continue
		}
		result = append(result, point)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Timestamp.Before(result[j].Timestamp) })
	return result
}

// diagnosticMetricContinuity 只把采样步长内的相邻时间点视为线段，缺口另行提示。
// diagnosticMetricContinuity accepts adjacent distinct timestamps within the sampling gap limit.
func diagnosticMetricContinuity(points []diagnosticPrometheusPoint, stepSeconds int) (bool, bool) {
	gapLimit := time.Duration(stepSeconds) * time.Second * 5 / 2
	if gapLimit <= 0 {
		gapLimit = 5 * time.Minute
	}
	continuous, gaps := false, false
	for i := 1; i < len(points); i++ {
		delta := points[i].Timestamp.Sub(points[i-1].Timestamp)
		if delta > gapLimit {
			gaps = true
		} else if delta > 0 {
			continuous = true
		}
	}
	return continuous, gaps
}

// diagnosticFirstMetricBreach 只依据真实采样与明确的比较方向判断首次越阈。
// diagnosticFirstMetricBreach finds the first breach only from real samples and a known comparator.
func diagnosticFirstMetricBreach(points []diagnosticPrometheusPoint, comparator string, threshold float64) *time.Time {
	for _, point := range points {
		switch comparator {
		case "gt":
			if point.Value > threshold {
				at := point.Timestamp
				return &at
			}
		case "lt":
			if point.Value < threshold {
				at := point.Timestamp
				return &at
			}
		}
	}
	return nil
}

// renderDiagnosticEvidenceChart 按真实时间坐标绘图；缺口断线，单个采样点不画趋势。
// renderDiagnosticEvidenceChart uses real timestamps, splits sampling gaps, and never invents a line from one point.
func renderDiagnosticEvidenceChart(metric *diagnosticBundleHTMLMetricEvidence, focus bool, lang DiagnosticLanguage) template.HTML {
	if metric == nil {
		return ""
	}
	points, start, end := metric.Points, metric.WindowStart, metric.WindowEnd
	if focus {
		points, start, end = metric.FocusPoints, metric.FocusStart, metric.FocusEnd
	}
	if len(points) < 2 || !start.Before(end) || !points[0].Timestamp.Before(points[len(points)-1].Timestamp) {
		return ""
	}
	const width, height, left, right, top, bottom = 860.0, 245.0, 62.0, 24.0, 20.0, 38.0
	minValue, maxValue := points[0].Value, points[0].Value
	showThreshold := metric.Comparator == "gt" || metric.Comparator == "lt"
	if showThreshold {
		minValue = math.Min(minValue, metric.Threshold)
		maxValue = math.Max(maxValue, metric.Threshold)
	}
	for _, point := range points {
		minValue = math.Min(minValue, point.Value)
		maxValue = math.Max(maxValue, point.Value)
	}
	padding := math.Max((maxValue-minValue)*0.12, 0.02)
	minValue -= padding
	maxValue += padding
	if minValue == maxValue {
		maxValue++
	}
	x := func(at time.Time) float64 {
		return left + at.Sub(start).Seconds()/end.Sub(start).Seconds()*(width-left-right)
	}
	y := func(value float64) float64 { return top + (maxValue-value)/(maxValue-minValue)*(height-top-bottom) }
	var result strings.Builder
	label := chooseDiagnosticLocalizedText(diagnosticLocalizedText{ZH: "指标采样趋势", EN: "Metric sampling trend"}, lang)
	fmt.Fprintf(&result, `<svg viewBox="0 0 %.0f %.0f" class="evidence-trend-svg" role="img" aria-label="%s">`, width, height, template.HTMLEscapeString(label))
	if showThreshold {
		fmt.Fprintf(&result, `<line x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f" class="evidence-trend-threshold"/>`, left, width-right, y(metric.Threshold), y(metric.Threshold))
	}
	if metric.FirstBreach != nil && !metric.FirstBreach.Before(start) && !metric.FirstBreach.After(end) {
		fmt.Fprintf(&result, `<line x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f" class="evidence-trend-marker"/>`, x(*metric.FirstBreach), x(*metric.FirstBreach), top, height-bottom)
	}
	gapLimit := time.Duration(metric.StepSeconds) * time.Second * 5 / 2
	if gapLimit <= 0 {
		gapLimit = 5 * time.Minute
	}
	var path strings.Builder
	for i, point := range points {
		command := "L"
		if i == 0 || !point.Timestamp.After(points[i-1].Timestamp) || point.Timestamp.Sub(points[i-1].Timestamp) > gapLimit {
			command = "M"
		}
		fmt.Fprintf(&path, "%s%.1f %.1f ", command, x(point.Timestamp), y(point.Value))
	}
	fmt.Fprintf(&result, `<path d="%s" class="evidence-trend-path"/>`, path.String())
	for _, point := range points {
		pointLabel := fmt.Sprintf("%s · %s", formatDiagnosticBundleTimeValue(point.Timestamp), formatDiagnosticMetricValue(metric.Unit, point.Value))
		fmt.Fprintf(&result, `<circle cx="%.1f" cy="%.1f" r="4" class="evidence-trend-point" tabindex="0" aria-label="%s"><title>%s</title></circle>`, x(point.Timestamp), y(point.Value), template.HTMLEscapeString(pointLabel), template.HTMLEscapeString(pointLabel))
	}
	for _, tick := range []time.Time{start, start.Add(end.Sub(start) / 2), end} {
		anchor := "middle"
		if tick.Equal(start) {
			anchor = "start"
		} else if tick.Equal(end) {
			anchor = "end"
		}
		fmt.Fprintf(&result, `<text x="%.1f" y="%.1f" text-anchor="%s" class="evidence-trend-axis">%s</text>`, x(tick), height-8, anchor, template.HTMLEscapeString(tick.In(diagnosticDisplayLocation()).Format("15:04")))
	}
	result.WriteString(`</svg>`)
	return template.HTML(result.String())
}
