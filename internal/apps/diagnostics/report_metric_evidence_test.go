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
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildDiagnosticMetricFindingsOnlyLinksBreachedInstances 验证同一信号的正常实例不会继承其他实例的告警。
// TestBuildDiagnosticMetricFindingsOnlyLinksBreachedInstances keeps a healthy instance separate from another breached instance.
func TestBuildDiagnosticMetricFindingsOnlyLinksBreachedInstances(t *testing.T) {
	start := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	point := func(minute int, value float64) diagnosticPrometheusPoint {
		return diagnosticPrometheusPoint{Timestamp: start.Add(time.Duration(minute) * time.Minute), Value: value}
	}
	snapshot := &diagnosticPrometheusSnapshot{
		WindowStart: start, WindowEnd: start.Add(time.Hour), StepSeconds: 120,
		Signals: []diagnosticPrometheusSignal{{
			Key: "memory_usage_high", Title: bilingualText("JVM Heap 使用率", "JVM Heap Usage"),
			Status: "warning", Comparator: "gt", Threshold: 0.8, ThresholdText: "> 80%", Unit: "ratio",
			Series: []diagnosticPrometheusSeriesSummary{
				{Instance: "worker-03", Points: []diagnosticPrometheusPoint{point(42, 0.87), point(40, 0.78), point(44, 0.86)}},
				{Instance: "worker-04", Points: []diagnosticPrometheusPoint{point(40, 0.5), point(42, 0.6)}},
			},
		}},
	}
	got := buildDiagnosticMetricFindings(snapshot)
	if len(got) != 1 || got[0].Metric == nil || got[0].Metric.Instance != "worker-03" {
		t.Fatalf("expected only the breached instance, got %#v", got)
	}
	metric := got[0].Metric
	if metric.FirstBreach == nil || !metric.FirstBreach.Equal(start.Add(42*time.Minute)) || !metric.HasTrend {
		t.Fatalf("expected actual first threshold crossing and time series, got %#v", metric)
	}
	if len(metric.Points) != 3 || metric.Points[0].Timestamp.After(metric.Points[1].Timestamp) {
		t.Fatalf("expected chronological samples, got %#v", metric.Points)
	}
	if !strings.Contains(got[0].Evidence, "86.0%") || got[0].CheckCode != "memory_usage_high" {
		t.Fatalf("expected a signal-backed observation, got %#v", got[0])
	}
	payload := &diagnosticBundleHTMLPayload{Findings: got, Task: diagnosticBundleHTMLTaskSummary{ID: 18}}
	document, err := renderDiagnosticBundleHTMLDocument(payload, DiagnosticLanguageZH)
	if err != nil {
		t.Fatal(err)
	}
	html := string(document)
	if !strings.Contains(html, `data-trend-range="focus"`) || !strings.Contains(html, `class="evidence-trend-svg"`) || strings.Contains(html, "worker-04") {
		t.Fatal("expected a real trend and focus control for the breached instance only")
	}
}

// TestDiagnosticMetricEvidenceNoFabricatedTrend 验证零发现、单点和采样缺口不会被画成连续曲线。
// TestDiagnosticMetricEvidenceNoFabricatedTrend checks zero findings, single samples and missing intervals.
func TestDiagnosticMetricEvidenceNoFabricatedTrend(t *testing.T) {
	start := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	point := func(minute int, value float64) diagnosticPrometheusPoint {
		return diagnosticPrometheusPoint{Timestamp: start.Add(time.Duration(minute) * time.Minute), Value: value}
	}
	zero := &diagnosticPrometheusSnapshot{
		WindowStart: start, WindowEnd: start.Add(time.Hour),
		Signals: []diagnosticPrometheusSignal{{Status: "healthy", Series: []diagnosticPrometheusSeriesSummary{{Points: []diagnosticPrometheusPoint{point(1, 0.5), point(2, 0.6)}}}}},
	}
	if got := buildDiagnosticMetricFindings(zero); len(got) != 0 {
		t.Fatalf("healthy signal must not create a finding: %#v", got)
	}
	one := &diagnosticPrometheusSnapshot{
		WindowStart: start, WindowEnd: start.Add(time.Hour), StepSeconds: 60,
		Signals: []diagnosticPrometheusSignal{{
			Key: "memory_usage_high", Title: "Heap", Status: "warning", Comparator: "gt", Threshold: 0.8,
			Unit: "ratio", Series: []diagnosticPrometheusSeriesSummary{{Instance: "worker-03", Points: []diagnosticPrometheusPoint{point(42, 0.87)}}},
		}},
	}
	got := buildDiagnosticMetricFindings(one)
	if len(got) != 1 || got[0].Metric.HasTrend || renderDiagnosticEvidenceChart(got[0].Metric, false, DiagnosticLanguageZH) != "" {
		t.Fatal("a single sample cannot produce a trend")
	}
	metric := &diagnosticBundleHTMLMetricEvidence{
		WindowStart: start, WindowEnd: start.Add(time.Hour), StepSeconds: 60,
		Threshold: 0.8, Comparator: "gt", Unit: "ratio",
		Points: []diagnosticPrometheusPoint{point(0, 0.5), point(1, 0.6), point(30, 0.87)},
	}
	svg := string(renderDiagnosticEvidenceChart(metric, false, DiagnosticLanguageEN))
	if strings.Count(svg, "M") < 2 || !strings.Contains(svg, "Threshold") && !strings.Contains(svg, "evidence-trend-threshold") {
		t.Fatalf("expected a path break across a sampling gap: %s", svg)
	}
	filtered := diagnosticSortedMetricPoints([]diagnosticPrometheusPoint{point(1, math.NaN()), point(2, 0.75), point(90, 0.9)}, start, start.Add(time.Hour))
	if len(filtered) != 1 || filtered[0].Value != 0.75 {
		t.Fatalf("invalid and out-of-window points must be excluded: %#v", filtered)
	}
}

// TestDiagnosticReportSignalInstances 验证各实例的采样独立展示，缺口不连线。
// TestDiagnosticReportSignalInstances checks per-instance samples and disconnected gaps in the generated HTML.
func TestDiagnosticReportSignalInstances(t *testing.T) {
	start := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	point := func(minute int, value float64) diagnosticPrometheusPoint {
		return diagnosticPrometheusPoint{Timestamp: start.Add(time.Duration(minute) * time.Minute), Value: value}
	}
	state := &diagnosticBundleExecutionState{MetricsSnapshot: &diagnosticPrometheusSnapshot{
		WindowStart: start, WindowEnd: start.Add(time.Hour), StepSeconds: 60,
		Signals: []diagnosticPrometheusSignal{{
			Key: "memory_usage_high", Title: "Heap", Status: "warning", Comparator: "gt", Threshold: .8, ThresholdText: "> 80%", Unit: "ratio",
			Series: []diagnosticPrometheusSeriesSummary{
				{Instance: "worker-01", Points: []diagnosticPrometheusPoint{point(0, .7), point(1, .9), point(40, .86)}},
				{Instance: "worker-02", Points: []diagnosticPrometheusPoint{point(2, .4)}},
			},
		}},
	}}
	cards := buildDiagnosticBundleHTMLSignalCards(state)
	if len(cards) != 2 || !cards[0].Metric.HasTrend || !cards[0].Metric.HasGaps || cards[1].Metric.HasTrend {
		t.Fatalf("expected separate trend and single value: %#v", cards)
	}
	if cards[1].Status != bilingualText("未越阈", "Within threshold") {
		t.Fatalf("non-breaching instance inherited signal warning: %s", cards[1].Status)
	}
	payload := &diagnosticBundleHTMLPayload{
		Task: diagnosticBundleHTMLTaskSummary{ID: 26}, KeySignals: cards,
		Timeline: buildDiagnosticBundleHTMLTimeline(nil, state), Findings: buildDiagnosticMetricFindings(state.MetricsSnapshot),
	}
	for _, lang := range []DiagnosticLanguage{DiagnosticLanguageZH, DiagnosticLanguageEN} {
		document, err := renderDiagnosticBundleHTMLDocument(payload, lang)
		if err != nil {
			t.Fatal(err)
		}
		html := string(document)
		if !strings.Contains(html, "worker-01") || !strings.Contains(html, "worker-02") || strings.Count(html, `class="evidence-trend-svg"`) < 2 {
			t.Fatalf("expected independently rendered signal and finding trends in %s", lang)
		}
		if !strings.Contains(html, `data-trend-range="focus"`) || !strings.Contains(html, `class="report-event-source"`) {
			t.Fatalf("expected focus window and event provenance in %s", lang)
		}
		if strings.Count(html, `class="report-signal-row"`) != 2 {
			t.Fatalf("expected two series cards in %s", lang)
		}
	}
	if trend, gaps := diagnosticMetricContinuity([]diagnosticPrometheusPoint{point(0, .7), point(40, .9)}, 60); trend || !gaps {
		t.Fatal("isolated samples must not produce a continuous trend")
	}
}

// TestDiagnosticReportIncompleteCollection 验证失败的采集不会被写成零问题。
// TestDiagnosticReportIncompleteCollection ensures failed collection is not presented as a clean run.
func TestDiagnosticReportIncompleteCollection(t *testing.T) {
	// 渲染步骤在任务结束前运行，running 不能被误写成采集失败。
	// The report step runs before finalization; running is not a collection failure.
	running := buildDiagnosticBundleHTMLPayload(&DiagnosticTask{ID: 28, Status: DiagnosticTaskStatusRunning}, nil, t.TempDir(), nil)
	if running.CollectionIncomplete {
		t.Fatal("running render stage must not be labeled failed collection")
	}
	payload := buildDiagnosticBundleHTMLPayload(&DiagnosticTask{ID: 27, Status: DiagnosticTaskStatusFailed}, nil, t.TempDir(), nil)
	if !payload.CollectionIncomplete {
		t.Fatal("failed task must mark collection incomplete")
	}
	document, err := renderDiagnosticBundleHTMLDocument(payload, DiagnosticLanguageZH)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(document), "采集未完整完成") || strings.Contains(string(document), "仅表示本次时间窗内的已执行检查没有产生发现项") {
		t.Fatal("failed collection must not use zero-finding success copy")
	}
}

// TestDiagnosticReportArtifactLinks 验证离线文件入口只指向诊断包内部，并保留原始来源信息。
// TestDiagnosticReportArtifactLinks verifies bundle-local file links and source metadata.
func TestDiagnosticReportArtifactLinks(t *testing.T) {
	bundle := t.TempDir()
	inside := filepath.Join(bundle, "config", "seatunnel.yaml")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("name: example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if got := resolveDiagnosticBundleRelativePath(bundle, outside); got != "" {
		t.Fatalf("outside file should not become a link: %q", got)
	}
	groups := buildDiagnosticBundleHTMLArtifactGroups(bundle, []*diagnosticBundleArtifact{{
		Category: "config_snapshot", Path: inside, HostName: "worker-03", Format: "yaml", SizeBytes: 14,
	}})
	payload := &diagnosticBundleHTMLPayload{ArtifactGroups: groups, Task: diagnosticBundleHTMLTaskSummary{ID: 29}}
	document, err := renderDiagnosticBundleHTMLDocument(payload, DiagnosticLanguageZH)
	if err != nil {
		t.Fatal(err)
	}
	html := string(document)
	if !strings.Contains(html, `href="/api/v1/diagnostics/tasks/29/files/config/seatunnel.yaml"`) || !strings.Contains(html, "worker-03") || !strings.Contains(html, "采集文件") {
		t.Fatal("expected a relative artifact link with its source")
	}
}
