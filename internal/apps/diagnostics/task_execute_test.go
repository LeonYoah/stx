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
			FileCount:          1,
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
			CollectionNotes: []diagnosticConfigSnapshotNote{{
				HostID:     1,
				Role:       "master",
				ConfigType: "jvm_master_options",
				Message:    "file missing",
			}},
		},
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
	if !strings.Contains(enHTML, "Key Runtime Settings") || !strings.Contains(enHTML, "connector-fake.jar") || !strings.Contains(enHTML, "<svg") || !strings.Contains(enHTML, "CPU") {
		t.Fatalf("expected english rendered html to contain config inventory details")
	}
	if !strings.Contains(string(zhHTML), "关键配置摘要") || !strings.Contains(string(zhHTML), "复制预览") {
		t.Fatalf("expected chinese rendered html to contain localized config and copy labels")
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
