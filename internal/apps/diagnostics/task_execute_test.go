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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type fakeDiagnosticProcessEventReader struct {
	filteredRows      []*monitor.ProcessEventWithHost
	filteredErr       error
	clusterEvents     []*monitor.ProcessEvent
	clusterEventsErr  error
	listEventsCalls   int
	clusterEventCalls int
}

func (f *fakeDiagnosticProcessEventReader) ListEvents(_ context.Context, _ *monitor.ProcessEventFilter) ([]*monitor.ProcessEventWithHost, int64, error) {
	f.listEventsCalls++
	if f.filteredErr != nil {
		return nil, 0, f.filteredErr
	}
	return f.filteredRows, int64(len(f.filteredRows)), nil
}

func (f *fakeDiagnosticProcessEventReader) ListClusterEvents(_ context.Context, _ uint, _ int) ([]*monitor.ProcessEvent, error) {
	f.clusterEventCalls++
	if f.clusterEventsErr != nil {
		return nil, f.clusterEventsErr
	}
	return f.clusterEvents, nil
}

func TestResolveDiagnosticCollectionWindow_prefersTaskLookbackAndCurrentTimeWhenTaskOverrides(t *testing.T) {
	finishedAt := time.Date(2026, 3, 14, 10, 30, 0, 0, time.UTC)
	task := &DiagnosticTask{LookbackMinutes: 90}
	detail := &ClusterInspectionReportDetailData{
		Report: &ClusterInspectionReportInfo{
			LookbackMinutes: 30,
			FinishedAt:      &finishedAt,
		},
	}

	window := resolveDiagnosticCollectionWindow(task, detail)

	if window.LookbackMinutes != 90 {
		t.Fatalf("expected lookback 90, got %d", window.LookbackMinutes)
	}
	now := time.Now().UTC()
	if window.End.Before(now.Add(-5*time.Second)) || window.End.After(now.Add(5*time.Second)) {
		t.Fatalf("expected end near now, got %s (finishedAt=%s)", window.End, finishedAt)
	}
	expectedStart := window.End.Add(-90 * time.Minute)
	if !window.Start.Equal(expectedStart) {
		t.Fatalf("expected start %s, got %s", expectedStart, window.Start)
	}
}

func TestFilterDiagnosticAlertsByWindow_includesOverlapResolvedAndExplicitSource(t *testing.T) {
	start := time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	resolvedAt := start.Add(5 * time.Minute)
	closedAt := start.Add(10 * time.Minute)

	alerts := []*monitoringapp.AlertInstance{
		{
			AlertID:    "firing-overlap",
			Status:     monitoringapp.AlertDisplayStatusFiring,
			FiringAt:   start.Add(-1 * time.Hour),
			LastSeenAt: start.Add(2 * time.Minute),
		},
		{
			AlertID:    "resolved-in-window",
			Status:     monitoringapp.AlertDisplayStatusResolved,
			FiringAt:   start.Add(-10 * time.Minute),
			LastSeenAt: resolvedAt,
			ResolvedAt: &resolvedAt,
		},
		{
			AlertID:    "closed-in-window",
			Status:     monitoringapp.AlertDisplayStatusClosed,
			FiringAt:   start.Add(-10 * time.Minute),
			LastSeenAt: closedAt,
			ClosedAt:   &closedAt,
		},
		{
			AlertID:    "stale-firing",
			Status:     monitoringapp.AlertDisplayStatusFiring,
			FiringAt:   start.Add(-2 * time.Hour),
			LastSeenAt: start.Add(-1 * time.Minute),
		},
		{
			AlertID:    "explicit-source",
			Status:     monitoringapp.AlertDisplayStatusClosed,
			FiringAt:   start.Add(-24 * time.Hour),
			LastSeenAt: start.Add(-24 * time.Hour),
		},
	}

	filtered := filterDiagnosticAlertsByWindow(alerts, start, end, "explicit-source")
	gotIDs := make([]string, 0, len(filtered))
	for _, item := range filtered {
		if item != nil {
			gotIDs = append(gotIDs, item.AlertID)
		}
	}

	expected := []string{"firing-overlap", "resolved-in-window", "explicit-source"}
	if len(gotIDs) != len(expected) {
		t.Fatalf("expected %d alerts, got %d: %v", len(expected), len(gotIDs), gotIDs)
	}
	for _, id := range expected {
		if !containsString(gotIDs, id) {
			t.Fatalf("expected alert %q in filtered list, got %v", id, gotIDs)
		}
	}
	if containsString(gotIDs, "closed-in-window") || containsString(gotIDs, "stale-firing") {
		t.Fatalf("unexpected alerts in filtered list: %v", gotIDs)
	}
}

func TestBuildDiagnosticBundleManifest_compactsMetadata(t *testing.T) {
	start := time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	task := &DiagnosticTask{
		ID:              12,
		ClusterID:       7,
		TriggerSource:   DiagnosticTaskSourceInspectionFinding,
		SourceRef:       DiagnosticTaskSourceRef{InspectionReportID: 3, InspectionFindingID: 4},
		Options:         DiagnosticTaskOptions{IncludeThreadDump: true}.Normalize(),
		Status:          DiagnosticTaskStatusSucceeded,
		Summary:         "diagnostic summary",
		LookbackMinutes: 30,
		CreatedBy:       99,
		CreatedByName:   "tester",
		StartedAt:       timePtr(start),
		CompletedAt:     timePtr(end),
	}
	state := &diagnosticBundleExecutionState{
		WindowStart:     timePtr(start),
		WindowEnd:       timePtr(end),
		LookbackMinutes: 30,
	}

	manifest := buildDiagnosticBundleManifest(task, []*diagnosticBundleArtifact{}, state)
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	text := string(payload)

	if !strings.Contains(text, `"lookback_minutes":30`) {
		t.Fatalf("expected lookback_minutes in manifest, got %s", text)
	}
	if !strings.Contains(text, `"window_start"`) || !strings.Contains(text, `"window_end"`) {
		t.Fatalf("expected window range in manifest, got %s", text)
	}
	if strings.Contains(text, `"created_by"`) || strings.Contains(text, `"created_by_name"`) {
		t.Fatalf("creator metadata should not be present in manifest: %s", text)
	}
	if strings.Contains(text, `"started_at"`) || strings.Contains(text, `"completed_at"`) {
		t.Fatalf("execution timestamps should not be present in manifest: %s", text)
	}
}

func TestResolveDiagnosticPrimaryCategory_keepsUnknownForGenericFailures(t *testing.T) {
	state := &diagnosticBundleExecutionState{
		ErrorGroup: &SeatunnelErrorGroup{
			Title:         "Task TaskGroupLocation failed in Job SeaTunnel_Job",
			SampleMessage: "Begin to cancel other tasks in this pipeline.",
		},
	}

	if got := resolveDiagnosticPrimaryCategory(state); got != "unknown" {
		t.Fatalf("expected unknown category for generic failure, got %s", got)
	}

	focus := buildDiagnosticPrimaryFocus(state)
	if strings.Contains(focus, "外部依赖连通性") || strings.Contains(focus, "dependency reachability") {
		t.Fatalf("expected generic fallback focus, got %q", focus)
	}
}

func TestResolveDiagnosticPrimaryCategory_detectsDependencyOnlyForStrongSignals(t *testing.T) {
	state := &diagnosticBundleExecutionState{
		ErrorGroup: &SeatunnelErrorGroup{
			Title:         "java.net.UnknownHostException",
			SampleMessage: "dns lookup failed: no such host",
		},
	}

	if got := resolveDiagnosticPrimaryCategory(state); got != "dependency" {
		t.Fatalf("expected dependency category, got %s", got)
	}
}

func TestMapInspectionFindingToDiagnosticCategory_doesNotTreatGenericErrorAsDependency(t *testing.T) {
	finding := &ClusterInspectionFindingInfo{
		CheckCode: "GENERIC_FAILURE",
		Summary:   "Task failed with generic error",
	}

	if got := mapInspectionFindingToDiagnosticCategory(finding); got != "unknown" {
		t.Fatalf("expected unknown category for generic error finding, got %s", got)
	}
}

func TestBuildDiagnosticConfigTypesForTarget(t *testing.T) {
	tests := []struct {
		name string
		mode cluster.DeploymentMode
		role string
		want []appconfig.ConfigType
	}{
		{
			name: "hybrid master-worker",
			mode: cluster.DeploymentModeHybrid,
			role: string(cluster.NodeRoleMasterWorker),
			want: []appconfig.ConfigType{
				appconfig.ConfigTypeSeatunnel,
				appconfig.ConfigTypeHazelcast,
				appconfig.ConfigTypeHazelcastClient,
			},
		},
		{
			name: "separated master",
			mode: cluster.DeploymentModeSeparated,
			role: string(cluster.NodeRoleMaster),
			want: []appconfig.ConfigType{
				appconfig.ConfigTypeSeatunnel,
				appconfig.ConfigTypeHazelcastMaster,
				appconfig.ConfigTypeHazelcastClient,
			},
		},
		{
			name: "separated worker",
			mode: cluster.DeploymentModeSeparated,
			role: string(cluster.NodeRoleWorker),
			want: []appconfig.ConfigType{
				appconfig.ConfigTypeSeatunnel,
				appconfig.ConfigTypeHazelcastWorker,
				appconfig.ConfigTypeHazelcastClient,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDiagnosticConfigTypesForTarget(tt.mode, tt.role)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d config types, got %d: %v", len(tt.want), len(got), got)
			}
			for index, item := range tt.want {
				if got[index] != item {
					t.Fatalf("expected config type %q at index %d, got %q", item, index, got[index])
				}
			}
		})
	}
}

func TestDetectDiagnosticConfigFormat(t *testing.T) {
	if got := detectDiagnosticConfigFormat("seatunnel.yaml"); got != "yaml" {
		t.Fatalf("expected yaml, got %s", got)
	}
	if got := detectDiagnosticConfigFormat("log4j2.properties"); got != "properties" {
		t.Fatalf("expected properties, got %s", got)
	}
	if got := detectDiagnosticConfigFormat("jvm_options"); got != "text" {
		t.Fatalf("expected text, got %s", got)
	}
}

func TestExtractDiagnosticConfigHighlights_extractsYAMLAndJVMSettings(t *testing.T) {
	yamlContent := `
metrics:
  enabled: true
  prometheus:
    enabled: true
seatunnel:
  engine:
    backup-count: 2
    checkpoint:
      interval: 10000
`
	yamlHighlights := extractDiagnosticConfigHighlights("seatunnel.yaml", "/opt/seatunnel/config/seatunnel.yaml", yamlContent)
	if !containsConfigHighlight(yamlHighlights, "Metrics", "true") {
		t.Fatalf("expected metrics highlight, got %#v", yamlHighlights)
	}
	if !containsConfigHighlight(yamlHighlights, "Prometheus", "true") {
		t.Fatalf("expected prometheus highlight, got %#v", yamlHighlights)
	}
	if !containsConfigHighlight(yamlHighlights, "Backup Count", "2") {
		t.Fatalf("expected backup-count highlight, got %#v", yamlHighlights)
	}

	jvmContent := `
# comment
-Xms2g
-Xmx4g
-XX:+HeapDumpOnOutOfMemoryError
-XX:HeapDumpPath=/tmp/heap.hprof
`
	jvmHighlights := extractDiagnosticConfigHighlights("jvm_options", "/opt/seatunnel/config/jvm_options", jvmContent)
	if !containsConfigHighlight(jvmHighlights, "Xms", "2g") {
		t.Fatalf("expected Xms highlight, got %#v", jvmHighlights)
	}
	if !containsConfigHighlight(jvmHighlights, "Xmx", "4g") {
		t.Fatalf("expected Xmx highlight, got %#v", jvmHighlights)
	}
	if !containsConfigHighlight(jvmHighlights, "OOM HeapDump", "true") {
		t.Fatalf("expected heap dump highlight, got %#v", jvmHighlights)
	}
}

func TestBuildDiagnosticExtraConfigFilesForTarget(t *testing.T) {
	got := buildDiagnosticExtraConfigFilesForTarget(cluster.DeploymentModeSeparated, string(cluster.NodeRoleMaster))
	expected := []string{"seatunnel-env.sh", "log4j2.properties", "log4j2_client.properties", "plugin_config", "jvm_client_options", "jvm_master_options"}
	if len(got) != len(expected) {
		t.Fatalf("expected %d extra files, got %d: %v", len(expected), len(got), got)
	}
	for index, item := range expected {
		if got[index] != item {
			t.Fatalf("expected %q at index %d, got %q", item, index, got[index])
		}
	}
}

func TestBuildDiagnosticPrometheusSignalSpecs_includeGCOldGen(t *testing.T) {
	specs := buildDiagnosticPrometheusSignalSpecs(6)
	keys := make(map[string]diagnosticPrometheusSignalSpec, len(specs))
	for _, spec := range specs {
		keys[spec.Key] = spec
	}

	oldGen, ok := keys["old_gen_usage_high"]
	if !ok {
		t.Fatalf("expected old_gen_usage_high signal spec")
	}
	if !strings.Contains(oldGen.PromQL, "jvm_memory_pool_bytes_used") || !strings.Contains(oldGen.PromQL, "Old Gen") {
		t.Fatalf("unexpected old-gen promql: %s", oldGen.PromQL)
	}

	gc, ok := keys["gc_time_ratio_high"]
	if !ok {
		t.Fatalf("expected gc_time_ratio_high signal spec")
	}
	if gc.Unit != "percent" {
		t.Fatalf("expected percent unit for gc signal, got %s", gc.Unit)
	}
	if !strings.Contains(gc.PromQL, "jvm_gc_collection_seconds_sum") {
		t.Fatalf("unexpected gc promql: %s", gc.PromQL)
	}
}

func TestBuildDiagnosticBundleHTMLMetricsPanel_prioritizesAnomalies(t *testing.T) {
	panel := buildDiagnosticBundleHTMLMetricsPanel(&diagnosticPrometheusSnapshot{
		Signals: []diagnosticPrometheusSignal{
			{Title: "CPU", Status: "warning"},
			{Title: "Heap", Status: "critical"},
			{Title: "FD", Status: "healthy"},
		},
	})
	if panel == nil {
		t.Fatal("expected metrics panel")
	}
	if panel.AnomalyCount != 2 {
		t.Fatalf("expected anomaly count 2, got %d", panel.AnomalyCount)
	}
	if len(panel.HighlightedSignals) != 2 {
		t.Fatalf("expected 2 highlighted signals, got %d", len(panel.HighlightedSignals))
	}
	if len(panel.AdditionalSignals) != 1 {
		t.Fatalf("expected 1 additional signal, got %d", len(panel.AdditionalSignals))
	}
}

func TestResolveDiagnosticRiskTone_prefersProcessFailureAndInspection(t *testing.T) {
	state := &diagnosticBundleExecutionState{
		ProcessEvents: []*monitor.ProcessEvent{
			{EventType: monitor.EventTypeRestartFailed, CreatedAt: time.Now().UTC()},
		},
	}
	if got := resolveDiagnosticRiskTone(&DiagnosticTask{}, state); got != "critical" {
		t.Fatalf("expected critical, got %s", got)
	}

	state = &diagnosticBundleExecutionState{
		InspectionDetail: &ClusterInspectionReportDetailData{
			Report: &ClusterInspectionReportInfo{WarningCount: 1},
		},
	}
	if got := resolveDiagnosticRiskTone(&DiagnosticTask{}, state); got != "warning" {
		t.Fatalf("expected warning, got %s", got)
	}
}

func TestBuildDiagnosticBundleHTMLSignalCards_prefersCoreSignals(t *testing.T) {
	state := &diagnosticBundleExecutionState{
		MetricsSnapshot: &diagnosticPrometheusSnapshot{
			Signals: []diagnosticPrometheusSignal{
				{Key: "fd_usage_high", Title: "FD", Status: "healthy"},
				{Key: "memory_usage_high", Title: "Heap", Status: "warning", Series: []diagnosticPrometheusSeriesSummary{{Instance: "n1", MaxValue: 0.9, LastValue: 0.4}}},
				{Key: "cpu_usage_high", Title: "CPU", Status: "healthy", Series: []diagnosticPrometheusSeriesSummary{{Instance: "n1", MaxValue: 0.2, LastValue: 0.1}}},
				{Key: "gc_time_ratio_high", Title: "GC", Status: "warning", Series: []diagnosticPrometheusSeriesSummary{{Instance: "n1", MaxValue: 18, LastValue: 2}}},
			},
		},
	}
	cards := buildDiagnosticBundleHTMLSignalCards(state)
	if len(cards) != 3 {
		t.Fatalf("expected 3 cards, got %d", len(cards))
	}
	if cards[0].Key != "cpu_usage_high" || cards[1].Key != "memory_usage_high" || cards[2].Key != "gc_time_ratio_high" {
		t.Fatalf("unexpected card order: %#v", cards)
	}
}

func TestBuildDiagnosticBundleHTMLCategoryCards_countsPrimarySignals(t *testing.T) {
	state := &diagnosticBundleExecutionState{
		ErrorGroup: &SeatunnelErrorGroup{
			Title:         "Failed to initialize connection",
			SampleMessage: "DEADLINE_EXCEEDED timeout",
		},
		ProcessEvents: []*monitor.ProcessEvent{
			{EventType: monitor.EventTypeRestartFailed, CreatedAt: time.Now().UTC()},
		},
		MetricsSnapshot: &diagnosticPrometheusSnapshot{
			Signals: []diagnosticPrometheusSignal{
				{Key: "memory_usage_high", Status: "warning"},
			},
		},
	}
	cards := buildDiagnosticBundleHTMLCategoryCards(&DiagnosticTask{}, state)
	if len(cards) != 5 {
		t.Fatalf("expected 5 category cards, got %d", len(cards))
	}
	if cards[0].Count == 0 {
		t.Fatalf("expected dependency category to be counted, got %#v", cards[0])
	}
}

func TestExtractDiagnosticLogWindowContent_filtersByTimeWindow(t *testing.T) {
	start := time.Date(2026, 3, 15, 10, 0, 0, 0, time.Local)
	end := start.Add(5 * time.Minute)
	content := strings.Join([]string{
		"[] 2026-03-15 09:58:00,000 ERROR [x] [main] - before",
		"[] 2026-03-15 10:01:00,000 ERROR [x] [main] - matched",
		"\tat example.Stack",
		"[] 2026-03-15 10:04:00,000 WARN [x] [main] - matched-2",
		"[] 2026-03-15 10:07:00,000 ERROR [x] [main] - after",
	}, "\n")

	got, matchedWindow, sawTimestamp := extractDiagnosticLogWindowContent([]string{content}, start, end)
	if !matchedWindow || !sawTimestamp {
		t.Fatalf("expected matchedWindow and sawTimestamp to be true, got matched=%v sawTimestamp=%v", matchedWindow, sawTimestamp)
	}
	if strings.Contains(got, "before") || strings.Contains(got, "after") {
		t.Fatalf("expected out-of-window entries to be filtered, got %s", got)
	}
	if !strings.Contains(got, "matched") || !strings.Contains(got, "matched-2") {
		t.Fatalf("expected in-window entries to remain, got %s", got)
	}
	if !strings.Contains(got, "example.Stack") {
		t.Fatalf("expected stack trace lines to stay attached, got %s", got)
	}
}

func TestBuildDiagnosticConfigPreview_keepsFullContent(t *testing.T) {
	lines := make([]string, 0, 64)
	for i := 0; i < 64; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d=value", i))
	}
	content := strings.Join(lines, "\n")
	if got := buildDiagnosticConfigPreview(content); got != content {
		t.Fatalf("expected full config preview, got truncated content: %s", got)
	}
}

func TestExecuteCollectProcessEventsStep_fallsBackWhenFilteredQueryReturnsEmpty(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&DiagnosticTask{}, &DiagnosticTaskStep{}, &DiagnosticStepLog{}); err != nil {
		t.Fatalf("auto migrate diagnostics task models: %v", err)
	}

	now := time.Now().UTC()
	reader := &fakeDiagnosticProcessEventReader{
		filteredRows: []*monitor.ProcessEventWithHost{},
		clusterEvents: []*monitor.ProcessEvent{
			{
				ID:          1,
				ClusterID:   6,
				NodeID:      4,
				HostID:      4,
				EventType:   monitor.EventTypeNodeOffline,
				ProcessName: "seatunnel",
				CreatedAt:   now.Add(-10 * time.Minute),
			},
			{
				ID:          2,
				ClusterID:   6,
				NodeID:      4,
				HostID:      4,
				EventType:   monitor.EventTypeNodeRecovered,
				ProcessName: "seatunnel",
				CreatedAt:   now.Add(-5 * time.Minute),
			},
		},
	}

	service := NewServiceWithRepository(NewRepository(database), nil, reader, nil)
	task := &DiagnosticTask{
		ID:              1,
		ClusterID:       6,
		LookbackMinutes: 60,
	}
	step := &DiagnosticTaskStep{
		ID:   1,
		Code: DiagnosticStepCodeCollectProcessEvents,
	}
	state := &diagnosticBundleExecutionState{}
	bundleDir := t.TempDir()

	if err := service.executeCollectProcessEventsStep(t.Context(), task, step, state, bundleDir); err != nil {
		t.Fatalf("executeCollectProcessEventsStep returned error: %v", err)
	}
	if reader.listEventsCalls == 0 {
		t.Fatal("expected filtered ListEvents to be called")
	}
	if reader.clusterEventCalls == 0 {
		t.Fatal("expected fallback ListClusterEvents to be called")
	}
	if len(state.ProcessEvents) != 2 {
		t.Fatalf("expected 2 process events after fallback, got %d", len(state.ProcessEvents))
	}
	payload, err := os.ReadFile(bundleDir + "/process-events.json")
	if err != nil {
		t.Fatalf("read process-events artifact: %v", err)
	}
	if !strings.Contains(string(payload), "node_offline") || !strings.Contains(string(payload), "node_recovered") {
		t.Fatalf("expected process-events artifact to contain fallback events, got %s", string(payload))
	}
}

func TestQueryDiagnosticPrometheusSignal_marksWarningWhenThresholdBreached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"instance":"node-1"},"values":[[1,"0.91"],[2,"0.95"],[3,"0.93"]]}]}}`))
	}))
	defer server.Close()

	signal, err := queryDiagnosticPrometheusSignal(
		t.Context(),
		server.URL,
		diagnosticCollectionWindow{
			Start: time.Unix(0, 0).UTC(),
			End:   time.Unix(180, 0).UTC(),
		},
		60,
		diagnosticPrometheusSignalSpec{
			Key:            "cpu_usage_high",
			Title:          "CPU",
			Unit:           "cores",
			Threshold:      0.8,
			ThresholdText:  "> 0.8 cores",
			StatusOnBreach: "warning",
			Comparator:     "gt",
			PromQL:         "test_cpu_metric",
		},
	)
	if err != nil {
		t.Fatalf("queryDiagnosticPrometheusSignal returned error: %v", err)
	}
	if signal.Status != "warning" {
		t.Fatalf("expected warning status, got %s", signal.Status)
	}
	if len(signal.Series) != 1 || signal.Series[0].MaxValue <= 0.8 {
		t.Fatalf("expected breached series summary, got %+v", signal.Series)
	}
}

func TestResolveDiagnosticErrorContext_fallsBackToLatestGroupWithinWindow(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&SeatunnelErrorGroup{}, &SeatunnelErrorEvent{}); err != nil {
		t.Fatalf("auto migrate error context models: %v", err)
	}

	repo := NewRepository(database)
	service := NewServiceWithRepository(repo, nil, nil, nil)
	now := time.Now().UTC()
	group := &SeatunnelErrorGroup{
		Fingerprint:        "fp-1",
		FingerprintVersion: DefaultFingerprintVersion,
		Title:              "DEADLINE_EXCEEDED",
		SampleMessage:      "Failed to initialize connection",
		OccurrenceCount:    3,
		FirstSeenAt:        now.Add(-2 * time.Hour),
		LastSeenAt:         now.Add(-30 * time.Minute),
		LastClusterID:      6,
		LastNodeID:         4,
		LastHostID:         4,
	}
	if err := repo.CreateErrorGroup(t.Context(), group); err != nil {
		t.Fatalf("create error group: %v", err)
	}
	event := &SeatunnelErrorEvent{
		ErrorGroupID: group.ID,
		Fingerprint:  group.Fingerprint,
		ClusterID:    6,
		NodeID:       4,
		HostID:       4,
		AgentID:      "agent-1",
		Role:         "master/worker",
		InstallDir:   "/opt/seatunnel",
		SourceFile:   "/opt/seatunnel/logs/seatunnel-engine-server.log",
		OccurredAt:   now.Add(-20 * time.Minute),
		Message:      "Failed to initialize connection",
		Evidence:     "DEADLINE_EXCEEDED",
	}
	if err := repo.CreateErrorEvent(t.Context(), event); err != nil {
		t.Fatalf("create error event: %v", err)
	}

	resolvedGroup, resolvedEvents, err := service.resolveDiagnosticErrorContext(t.Context(), &DiagnosticTask{
		ClusterID:       6,
		TriggerSource:   DiagnosticTaskSourceManual,
		LookbackMinutes: 1440,
	}, diagnosticCollectionWindow{
		Start: now.Add(-24 * time.Hour),
		End:   now,
	})
	if err != nil {
		t.Fatalf("resolveDiagnosticErrorContext returned error: %v", err)
	}
	if resolvedGroup == nil || resolvedGroup.ID != group.ID {
		t.Fatalf("expected fallback error group %d, got %+v", group.ID, resolvedGroup)
	}
	if len(resolvedEvents) != 1 || resolvedEvents[0].ID != event.ID {
		t.Fatalf("expected fallback error events to include event %d, got %+v", event.ID, resolvedEvents)
	}
}

func TestDiagnosticBundleHTMLTemplateParsesAndRendersMetricsPanels(t *testing.T) {
	tmpl, err := newDiagnosticBundleHTMLTemplate(DiagnosticLanguageEN)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	now := time.Now().UTC()
	bundleDir := t.TempDir()
	logDir := filepath.Join(bundleDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	fullLogPath := filepath.Join(logDir, "host-1-master.log")
	logLines := make([]string, 0, diagnosticHTMLLogPreviewLineLimit+5)
	for index := 1; index <= diagnosticHTMLLogPreviewLineLimit+5; index++ {
		logLines = append(logLines, fmt.Sprintf("line-%04d", index))
	}
	logContent := strings.Join(logLines, "\n")
	if err := os.WriteFile(fullLogPath, []byte(logContent), 0o644); err != nil {
		t.Fatalf("write full log: %v", err)
	}
	payload := &diagnosticBundleHTMLPayload{
		GeneratedAt: now,
		Health: diagnosticBundleHTMLHealthSummary{
			Tone:    "warning",
			Title:   "warning",
			Summary: "summary",
			Metrics: []diagnosticBundleHTMLMetricCard{{Label: "time", Value: "30m"}},
		},
		Task: diagnosticBundleHTMLTaskSummary{
			ID:        1,
			Status:    DiagnosticTaskStatusSucceeded,
			Summary:   "task",
			CreatedBy: "tester",
		},
		KeySignals: []diagnosticBundleHTMLSignalCard{{
			Key:            "cpu_usage_high",
			Title:          "CPU",
			Status:         "warning",
			Summary:        "cpu summary",
			ThresholdText:  "> 0.8",
			Instance:       "node-1",
			LastValue:      "70.0%",
			PeakValue:      "90.0%",
			PeakAt:         formatDiagnosticBundleTime(&now),
			Interpretation: "peak reached threshold",
			Threshold:      0.8,
			Comparator:     "gt",
			Unit:           "ratio",
			Points: []diagnosticPrometheusPoint{
				{Timestamp: now.Add(-4 * time.Minute), Value: 0.2},
				{Timestamp: now.Add(-3 * time.Minute), Value: 0.4},
				{Timestamp: now.Add(-2 * time.Minute), Value: 0.9},
				{Timestamp: now.Add(-1 * time.Minute), Value: 0.7},
			},
		}},
		MetricsSnapshot: &diagnosticBundleHTMLMetricsPanel{
			SignalCount:  1,
			AnomalyCount: 1,
			HighlightedSignals: []diagnosticPrometheusSignal{{
				Title:         "CPU",
				ThresholdText: "> 0.8",
				Status:        "warning",
				Unit:          "ratio",
				Threshold:     0.8,
				Comparator:    "gt",
				Summary:       "cpu summary",
				Series: []diagnosticPrometheusSeriesSummary{{
					Instance:  "node-1",
					MinValue:  0.2,
					MaxValue:  0.9,
					LastValue: 0.7,
					Samples:   5,
					MaxAt:     timePtr(now),
					Points: []diagnosticPrometheusPoint{
						{Timestamp: now.Add(-4 * time.Minute), Value: 0.2},
						{Timestamp: now.Add(-3 * time.Minute), Value: 0.4},
						{Timestamp: now.Add(-2 * time.Minute), Value: 0.9},
						{Timestamp: now.Add(-1 * time.Minute), Value: 0.7},
					},
				}},
			}},
			CollectionNotes: []string{"partial query failed"},
		},
		ErrorContext: buildDiagnosticBundleHTMLErrorPanel(bundleDir, 44, &SeatunnelErrorGroup{
			Title:           "Sample Error Group",
			ExceptionClass:  "java.lang.RuntimeException",
			OccurrenceCount: 3,
			FirstSeenAt:     now.Add(-10 * time.Minute),
			LastSeenAt:      now,
			SampleMessage:   "runtime failed",
		}, nil, []diagnosticCollectedLogSample{{
			HostID:      1,
			HostName:    "host-1",
			HostIP:      "127.0.0.1",
			Role:        "master",
			SourceFile:  "/opt/seatunnel/logs/master.log",
			LocalPath:   fullLogPath,
			WindowStart: now.Add(-5 * time.Minute),
			WindowEnd:   now,
			Content:     logContent,
		}}),
		ConfigSnapshot: &diagnosticBundleHTMLConfigPanel{
			FileCount:          2,
			KeyHighlightCount:  1,
			DirectoryCount:     1,
			ChangedConfigCount: 1,
			KeyHighlights: []diagnosticConfigKeyHighlight{{
				HostID:     1,
				Role:       "master",
				ConfigType: "seatunnel.yaml",
				RemotePath: "/opt/seatunnel/config/seatunnel.yaml",
				Items: []diagnosticConfigKeyValue{{
					Label: "Metrics",
					Value: "true",
				}},
			}},
			FilePreviews: []diagnosticConfigFilePreview{{
				HostID:     1,
				Role:       "master",
				ConfigType: "seatunnel.yaml",
				RemotePath: "/opt/seatunnel/config/seatunnel.yaml",
				Preview:    "metrics:\\n  enabled: true",
			}},
			RecentChanges: []diagnosticConfigChangeRecord{{
				ConfigType: "seatunnel.yaml",
				HostScope:  "template",
				Version:    2,
				FilePath:   "config/seatunnel.yaml",
				UpdatedAt:  now,
			}},
			Files: []diagnosticConfigSnapshotFile{{
				HostID:      1,
				Role:        "master",
				ConfigType:  "seatunnel.yaml",
				RemotePath:  "/opt/seatunnel/config/seatunnel.yaml",
				SizeBytes:   1024,
				ContentHash: "1234567890abcdef",
			}},
			ConfigChanges: []diagnosticConfigChangeRecord{{
				ConfigType: "seatunnel.yaml",
				HostScope:  "template",
				Version:    2,
				FilePath:   "config/seatunnel.yaml",
				UpdatedAt:  now,
			}},
			DirectoryManifests: []diagnosticDirectoryManifest{{
				Directory:  "/tmp/connectors",
				EntryCount: 1,
				Entries: []diagnosticDirectoryManifestItem{{
					Name:    "connector-fake.jar",
					Path:    "/tmp/connectors/connector-fake.jar",
					Size:    123,
					ModTime: now,
				}},
			}},
			ConfigFileEntries: []diagnosticBundleHTMLConfigFileEntry{{
				HostID:     1,
				Role:       "master",
				ConfigType: "seatunnel.yaml",
				RemotePath: "/opt/seatunnel/config/seatunnel.yaml",
				Items: []diagnosticConfigKeyValue{{
					Label: "Metrics",
					Value: "true",
				}},
				Preview: "metrics:\n  enabled: true",
			}, {
				HostID:     2,
				Role:       "worker",
				ConfigType: "hazelcast.yaml",
				RemotePath: "/opt/seatunnel/config/hazelcast.yaml",
				Preview:    "enabled: false",
			}},
			CollectionNotes: []diagnosticConfigSnapshotNote{{
				HostID:     1,
				Role:       "master",
				ConfigType: "jvm_master_options",
				Message:    "file missing",
			}},
		},
		ThreadDumps: []diagnosticBundleHTMLThreadDumpItem{{
			HostID:       1,
			HostLabel:    "127.0.0.1 (主机 #1)",
			Role:         "master",
			Tool:         "jcmd Thread.print",
			RelativePath: "thread-dumps/thread-dump-host-1-master.txt",
			PreviewURL:   "/api/v1/diagnostics/tasks/44/files/thread-dumps/thread-dump-host-1-master.txt",
			SizeBytes:    2048,
			SizeLabel:    "2.0 KB",
			Preview:      "Full thread dump Java HotSpot...",
			TotalLines:   50,
		}},
		PassedChecks: []diagnosticBundleHTMLAdvice{{
			Title:   "进程运行正常 / Process Running",
			Details: "所有节点均处于 RUNNING 状态 / All nodes are running",
		}},
	}

	var enBuf bytes.Buffer
	payload.Language = DiagnosticLanguageEN
	if err := tmpl.Execute(&enBuf, payload); err != nil {
		t.Fatalf("execute template: %v", err)
	}

	zhHTML, err := renderDiagnosticBundleHTMLDocument(payload, DiagnosticLanguageZH)
	if err != nil {
		t.Fatalf("render zh document: %v", err)
	}

	enHTML := enBuf.String()
	if !strings.Contains(enHTML, "lang=\"en\"") || !strings.Contains(enHTML, "More Signals") {
		t.Fatalf("expected english document to contain english metrics evidence panel")
	}
	if !strings.Contains(string(zhHTML), "lang=\"zh-CN\"") || !strings.Contains(string(zhHTML), "更多指标") {
		t.Fatalf("expected chinese document to contain chinese metrics evidence panel")
	}
	if strings.Contains(enHTML, "data-lang-button=\"zh\"") || strings.Contains(enHTML, "data-lang-button=\"en\"") {
		t.Fatalf("expected rendered html to remove language toggles")
	}
	if strings.Contains(enHTML, "lang-switch") || strings.Contains(enHTML, "i18n-zh") || strings.Contains(enHTML, "i18n-en") {
		t.Fatalf("expected rendered html to remove bilingual toggle styles")
	}
	if !strings.Contains(enHTML, "data-inner-tab-group=\"evidence\"") || !strings.Contains(enHTML, "data-inner-tab-group=\"appendix\"") {
		t.Fatalf("expected rendered html to contain nested inner tabs")
	}
	if strings.Contains(enHTML, "借鉴 Allure categories") {
		t.Fatalf("expected rendered html to remove internal allure guidance copy")
	}
	if !strings.Contains(enHTML, "Configurations &amp; Previews") && !strings.Contains(enHTML, "Key Runtime Settings") {
		t.Fatalf("expected english rendered html to contain config section")
	}
	if !strings.Contains(enHTML, "connector-fake.jar") || !strings.Contains(enHTML, "<svg") || !strings.Contains(enHTML, "CPU") {
		t.Fatalf("expected english rendered html to contain config inventory details")
	}
	if !strings.Contains(string(zhHTML), "配置详情与预览") && !strings.Contains(string(zhHTML), "关键配置摘要") {
		t.Fatalf("expected chinese rendered html to contain localized config label")
	}
	if !strings.Contains(string(zhHTML), "复制内容") {
		t.Fatalf("expected chinese rendered html to contain localized copy label")
	}
	if !strings.Contains(enHTML, "View Full Log") || !strings.Contains(enHTML, "Open in New Window") {
		t.Fatalf("expected english rendered html to contain full log actions")
	}
	if !strings.Contains(enHTML, "/api/v1/diagnostics/tasks/44/files/logs/host-1-master.log") {
		t.Fatalf("expected english rendered html to contain online preview file url")
	}
	if !strings.Contains(enHTML, "line-1000") || strings.Contains(enHTML, "line-1005") {
		t.Fatalf("expected english rendered html to contain only preview log lines")
	}
	if !strings.Contains(enHTML, "stx-brand-logo") || !strings.Contains(enHTML, "STX") {
		t.Fatalf("expected english document to contain STX brand lockup and logo")
	}
	if !strings.Contains(enHTML, "rel=\"icon\"") {
		t.Fatalf("expected rendered html to contain favicon link")
	}
	if !strings.Contains(enHTML, "theme-toggle-btn") || !strings.Contains(enHTML, "data-theme") {
		t.Fatalf("expected rendered html to contain theme toggle and data-theme")
	}
	if !strings.Contains(enHTML, "data-inner-tab-key=\"threaddump\"") {
		t.Fatalf("expected rendered html to contain threaddump inner tab")
	}
	if !strings.Contains(enHTML, "config-preview-details") {
		t.Fatalf("expected rendered html to contain collapsible config preview details")
	}
	if !strings.Contains(enHTML, `data-inner-tab-group="config-files"`) || !strings.Contains(enHTML, `role="tablist"`) {
		t.Fatalf("expected file-scoped configuration tabs")
	}
	if !strings.Contains(enHTML, `id="config-file-tab-0"`) || !strings.Contains(enHTML, `id="config-file-tab-1"`) || !strings.Contains(enHTML, `id="config-file-panel-1"`) {
		t.Fatalf("expected separate accessible tab and panel for each file")
	}
	if !strings.Contains(enHTML, `id="config-file-tab-1" aria-controls="config-file-panel-1" aria-selected="false" tabindex="-1"`) {
		t.Fatalf("expected only the first config tab to be active initially")
	}
	if !strings.Contains(enHTML, "View Redacted Config") || !strings.Contains(string(zhHTML), "查看已脱敏配置") {
		t.Fatalf("expected rendered html to contain config preview toggle labels")
	}
	if !strings.Contains(enHTML, "prefers-color-scheme: dark") {
		t.Fatalf("expected document to contain dark mode styles")
	}
	if !strings.Contains(enHTML, "Appendix &amp; Nodes") {
		t.Fatalf("expected english document to contain renamed appendix tab")
	}
	if !strings.Contains(string(zhHTML), "附录与节点") {
		t.Fatalf("expected chinese document to contain renamed appendix tab")
	}
	if strings.Contains(enHTML, "data-tab-link=\"tab-overview\" href=\"#tab-overview\">\n          <div class=\"meta\">\n            <div class=\"title\">Overview</div>\n          </div>\n          <span class=\"count\">") {
		t.Fatalf("expected overview tab badge to be removed")
	}
	if !strings.Contains(enHTML, "passed-checklist-grid") || !strings.Contains(enHTML, "passed-check-item") {
		t.Fatalf("expected rendered html to contain passed checklist grid")
	}
	if !strings.Contains(enHTML, "collection-notes-card") || !strings.Contains(enHTML, "collection-notes-hint") {
		t.Fatalf("expected rendered html to contain collection notes card and hint")
	}
	if !strings.Contains(enHTML, "debug-details") {
		t.Fatalf("expected rendered html to contain collapsible debug details")
	}
}

func TestFormatDiagnosticMetricValue_percent(t *testing.T) {
	if got := formatDiagnosticMetricValue("percent", 12.345); got != "12.3%" {
		t.Fatalf("expected percent metric formatting, got %s", got)
	}
}

func TestBuildDiagnosticLogSampleFileName(t *testing.T) {
	tests := []struct {
		name       string
		hostID     uint
		hostName   string
		sourcePath string
		expected   string
	}{
		{
			name:       "prefer sanitized host name when present",
			hostID:     4,
			hostName:   "prod node/01",
			sourcePath: "/opt/seatunnel/logs/seatunnel-engine-server.log",
			expected:   "prod-node-01-seatunnel-engine-server.log",
		},
		{
			name:       "append log extension when source has no extension",
			hostID:     7,
			hostName:   "host-a",
			sourcePath: "/opt/seatunnel/logs/stdout",
			expected:   "host-a-stdout.log",
		},
		{
			name:       "fallback to host id when host name is empty",
			hostID:     9,
			hostName:   "",
			sourcePath: "",
			expected:   "host-9-log-sample.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildDiagnosticLogSampleFileName(tt.hostID, tt.hostName, tt.sourcePath); got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsConfigHighlight(items []diagnosticConfigKeyValue, label, value string) bool {
	for _, item := range items {
		if strings.Contains(item.Label, label) && item.Value == value {
			return true
		}
	}
	return false
}

func rebuildOfflineReportForTask(t *testing.T, taskID uint) {
	bundleDir := fmt.Sprintf("../../../data/storage/diagnostics/tasks/%d", taskID)
	manifestPath := filepath.Join(bundleDir, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Logf("task %d manifest not found, skipping local re-render", taskID)
		return
	}
	var manifest diagnosticBundleManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("unmarshal manifest for task %d: %v", taskID, err)
	}

	errorContextBytes, _ := os.ReadFile(filepath.Join(bundleDir, "error-context.json"))
	var errorContextPayload struct {
		RelatedDiagnosticTask *DiagnosticTask                    `json:"related_diagnostic_task"`
		Report                *ClusterInspectionReportDetailData `json:"report"`
	}
	_ = json.Unmarshal(errorContextBytes, &errorContextPayload)

	configSnapshotBytes, _ := os.ReadFile(filepath.Join(bundleDir, "config-snapshot.json"))
	var configSnapshot diagnosticConfigSnapshotSummary
	_ = json.Unmarshal(configSnapshotBytes, &configSnapshot)

	task := errorContextPayload.RelatedDiagnosticTask
	if task == nil {
		task = &DiagnosticTask{
			ID:            taskID,
			ClusterID:     manifest.ClusterID,
			TriggerSource: manifest.TriggerSource,
			Status:        manifest.Status,
			Summary:       manifest.Summary,
			Options:       manifest.Options,
			BundleDir:     bundleDir,
			ManifestPath:  manifestPath,
			IndexPath:     filepath.Join(bundleDir, "index.html"),
		}
	}
	task.Status = DiagnosticTaskStatusSucceeded
	task.BundleDir = bundleDir
	task.ManifestPath = manifestPath
	task.IndexPath = filepath.Join(bundleDir, "index.html")

	// 尝试从本地 sqlite 读取真实的已完成步骤与节点执行记录
	// Try loading authentic completed steps and node executions from local sqlite
	dbPath := "../../../data/seatunnelx.db"
	if _, err := os.Stat(dbPath); err == nil {
		db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
		if err == nil {
			var dbSteps []DiagnosticTaskStep
			if err := db.Table("diagnostics_task_steps").Where("task_id = ?", taskID).Order("sequence asc").Find(&dbSteps).Error; err == nil && len(dbSteps) > 0 {
				task.Steps = dbSteps
			}
			var dbNodes []DiagnosticNodeExecution
			if err := db.Table("diagnostics_task_nodes").Where("task_id = ?", taskID).Find(&dbNodes).Error; err == nil && len(dbNodes) > 0 {
				task.NodeExecutions = dbNodes
			}
		}
	}

	state := &diagnosticBundleExecutionState{
		InspectionDetail: errorContextPayload.Report,
		ConfigSnapshot:   &configSnapshot,
		Artifacts:        manifest.Artifacts,
	}

	payload := buildDiagnosticBundleHTMLPayload(task, state, bundleDir, manifest.Artifacts)

	renderTargets := []struct {
		Path string
		Lang DiagnosticLanguage
	}{
		{Path: filepath.Join(bundleDir, "index.html"), Lang: DiagnosticLanguageZH},
		{Path: filepath.Join(bundleDir, "index.zh.html"), Lang: DiagnosticLanguageZH},
		{Path: filepath.Join(bundleDir, "index.en.html"), Lang: DiagnosticLanguageEN},
	}
	for _, target := range renderTargets {
		content, err := renderDiagnosticBundleHTMLDocument(payload, target.Lang)
		if err != nil {
			t.Fatalf("render %s: %v", target.Path, err)
		}
		if err := os.WriteFile(target.Path, content, 0o644); err != nil {
			t.Fatalf("write %s: %v", target.Path, err)
		}
	}
}

func TestRebuildTask9OfflineReport(t *testing.T) {
	rebuildOfflineReportForTask(t, 9)
	rebuildOfflineReportForTask(t, 10)
}

func TestStepExecutionStatusSyncAndConfigPairing(t *testing.T) {
	// 1. 验证 buildDiagnosticBundleHTMLConfigPanel 聚合 ConfigFileEntries 并将摘要与预览配对
	// 1. Verify buildDiagnosticBundleHTMLConfigPanel aggregates ConfigFileEntries and pairs highlights with previews
	cfgSummary := &diagnosticConfigSnapshotSummary{
		Files: []diagnosticConfigSnapshotFile{
			{HostID: 1, HostName: "host-1", NodeID: 1, Role: "master", ConfigType: "seatunnel.yaml", RemotePath: "/etc/st/seatunnel.yaml"},
			{HostID: 2, HostName: "host-2", NodeID: 2, Role: "worker", ConfigType: "hazelcast.yaml", RemotePath: "/etc/st/hazelcast.yaml"},
		},
		KeyHighlights: []diagnosticConfigKeyHighlight{
			{HostID: 1, ConfigType: "seatunnel.yaml", RemotePath: "/etc/st/seatunnel.yaml", Items: []diagnosticConfigKeyValue{{Label: "Cluster", Value: "seatunnel"}}},
		},
		FilePreviews: []diagnosticConfigFilePreview{
			{HostID: 1, ConfigType: "seatunnel.yaml", RemotePath: "/etc/st/seatunnel.yaml", Preview: "cluster: seatunnel"},
			{HostID: 2, ConfigType: "hazelcast.yaml", RemotePath: "/etc/st/hazelcast.yaml", Preview: "hazelcast: enabled"},
		},
	}
	cfgPanel := buildDiagnosticBundleHTMLConfigPanel(cfgSummary)
	if cfgPanel == nil || len(cfgPanel.ConfigFileEntries) != 2 {
		t.Fatalf("expected 2 ConfigFileEntries, got %v", cfgPanel)
	}
	if len(cfgPanel.ConfigFileEntries[0].Items) != 1 || cfgPanel.ConfigFileEntries[0].Preview != "cluster: seatunnel" {
		t.Fatalf("expected entry 0 to have both highlights and preview, got %+v", cfgPanel.ConfigFileEntries[0])
	}
	if len(cfgPanel.ConfigFileEntries[1].Items) != 0 || cfgPanel.ConfigFileEntries[1].Preview != "hazelcast: enabled" {
		t.Fatalf("expected entry 1 to have empty highlights and preview, got %+v", cfgPanel.ConfigFileEntries[1])
	}

	// 2. 验证 buildDiagnosticBundleHTMLThreadDumps 收集线程栈与行数、大小格式化
	// 2. Verify buildDiagnosticBundleHTMLThreadDumps collects thread dumps with line count and size formatting
	tmpDir := t.TempDir()
	dumpDir := filepath.Join(tmpDir, "thread-dumps")
	if err := os.MkdirAll(dumpDir, 0o755); err != nil {
		t.Fatalf("mkdir dumpDir: %v", err)
	}
	dumpFile := filepath.Join(dumpDir, "thread-dump-host-10-hybrid.txt")
	sampleContent := "Full thread dump Java HotSpot...\nline 1\nline 2\nline 3\n"
	if err := os.WriteFile(dumpFile, []byte(sampleContent), 0o644); err != nil {
		t.Fatalf("write dumpFile: %v", err)
	}
	task := &DiagnosticTask{
		ID: 10,
		SelectedNodes: []DiagnosticTaskNodeTarget{
			{HostID: 10, HostName: "node-10", HostIP: "192.168.1.10", Role: "hybrid", NodeID: 12},
		},
	}
	dumps := buildDiagnosticBundleHTMLThreadDumps(tmpDir, nil, task)
	if len(dumps) != 1 {
		t.Fatalf("expected 1 thread dump, got %d", len(dumps))
	}
	if dumps[0].HostID != 10 || dumps[0].Role != "hybrid" {
		t.Fatalf("expected host 10 and role hybrid, got %+v", dumps[0])
	}
	if !strings.Contains(dumps[0].Preview, "Full thread dump") {
		t.Fatalf("expected preview to contain sample content, got %s", dumps[0].Preview)
	}
	if dumps[0].RelativePath != "thread-dumps/thread-dump-host-10-hybrid.txt" {
		t.Fatalf("unexpected relative path: %s", dumps[0].RelativePath)
	}
}

// TestBuildDiagnosticConfigPanelRedactsLegacyPreviews 验证旧快照在生成报告时也不会回显凭证。
// TestBuildDiagnosticConfigPanelRedactsLegacyPreviews ensures historical snapshots are masked when rendered.
func TestBuildDiagnosticConfigPanelRedactsLegacyPreviews(t *testing.T) {
	summary := &diagnosticConfigSnapshotSummary{
		Files: []diagnosticConfigSnapshotFile{
			{HostID: 1, ConfigType: "seatunnel.yaml", RemotePath: "/config/seatunnel.yaml"},
			{HostID: 2, ConfigType: "hazelcast.yaml", RemotePath: "/config/hazelcast.yaml"},
		},
		FilePreviews: []diagnosticConfigFilePreview{
			{HostID: 1, ConfigType: "seatunnel.yaml", RemotePath: "/config/seatunnel.yaml", Preview: "imap:\n  password: imap-secret"},
			{HostID: 2, ConfigType: "hazelcast.yaml", RemotePath: "/config/hazelcast.yaml", Preview: "url: imaps://user:mail-secret@mail.example"},
		},
	}
	panel := buildDiagnosticBundleHTMLConfigPanel(summary)
	if len(panel.ConfigFileEntries) != 2 || len(panel.FilePreviews) != 2 {
		t.Fatalf("expected two separate file entries and previews, got %#v", panel)
	}
	for _, entry := range panel.ConfigFileEntries {
		if strings.Contains(entry.Preview, "imap-secret") || strings.Contains(entry.Preview, "mail-secret") {
			t.Fatalf("unmasked credential in report panel for %s", entry.ConfigType)
		}
	}
	if strings.Contains(panel.FilePreviews[0].Preview, "imap-secret") || strings.Contains(panel.FilePreviews[1].Preview, "mail-secret") {
		t.Fatal("unmasked credential in legacy preview fields")
	}
}

// TestCollectDiagnosticConfigArtifactPersistsOnlyRedactedContent 验证诊断包文件、预览与摘要均不保存凭证。
// TestCollectDiagnosticConfigArtifactPersistsOnlyRedactedContent checks that artifact files, previews and highlights omit credentials.
func TestCollectDiagnosticConfigArtifactPersistsOnlyRedactedContent(t *testing.T) {
	repo := newDiagnosticTaskServiceRepository(t)
	service := NewServiceWithRepository(repo, nil, nil, nil)
	task := &DiagnosticTask{ClusterID: 1, TriggerSource: DiagnosticTaskSourceManual, Status: DiagnosticTaskStatusRunning}
	if err := repo.db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	step := &DiagnosticTaskStep{TaskID: task.ID, Code: DiagnosticStepCodeCollectConfigSnapshot, Status: DiagnosticTaskStatusRunning}
	if err := repo.db.Create(step).Error; err != nil {
		t.Fatal(err)
	}
	node := &DiagnosticNodeExecution{TaskID: task.ID, TaskStepID: &step.ID, HostID: 4, Role: "worker", Status: DiagnosticTaskStatusRunning}
	if err := repo.db.Create(node).Error; err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	state := &diagnosticBundleExecutionState{}
	summary := &diagnosticConfigSnapshotSummary{}
	content := "ck:\n  url: jdbc:clickhouse://reader:ck-secret@db:8123/events\nimap:\n  password: imap-secret\n  enabled: true"
	target := DiagnosticTaskNodeTarget{HostID: 4, Role: "worker", NodeID: 8}
	if err := service.collectDiagnosticConfigArtifact(t.Context(), task, step, state, summary, configDir, target, "seatunnel.yaml", "/opt/seatunnel/config/seatunnel.yaml", content, node, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(summary.Files[0].LocalPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{string(stored), summary.FilePreviews[0].Preview} {
		if strings.Contains(text, "ck-secret") || strings.Contains(text, "imap-secret") {
			t.Fatal("credential was persisted in a diagnostics artifact")
		}
		if !strings.Contains(text, "enabled: true") || !strings.Contains(text, "******") {
			t.Fatalf("expected readable, redacted config, got %q", text)
		}
	}
	if summary.Files[0].ContentHash != buildDiagnosticContentHash(string(stored)) {
		t.Fatal("artifact hash must be calculated from the persisted redacted content")
	}
}
