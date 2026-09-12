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
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type fakeInspectionClusterReader struct {
	cluster   *cluster.Cluster
	getErr    error
	status    *cluster.ClusterStatusInfo
	statusErr error
}

func (f *fakeInspectionClusterReader) List(_ context.Context, _ *cluster.ClusterFilter) ([]*cluster.Cluster, int64, error) {
	return []*cluster.Cluster{}, 0, nil
}

func (f *fakeInspectionClusterReader) Get(_ context.Context, id uint) (*cluster.Cluster, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.cluster != nil {
		return f.cluster, nil
	}
	return &cluster.Cluster{ID: id, Name: "demo-cluster"}, nil
}

func (f *fakeInspectionClusterReader) GetStatus(_ context.Context, _ uint) (*cluster.ClusterStatusInfo, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}

type fakeInspectionEventReader struct {
	events []*monitor.ProcessEvent
	err    error
}

func (f *fakeInspectionEventReader) ListClusterEvents(_ context.Context, _ uint, _ int) ([]*monitor.ProcessEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}

type fakeInspectionAlertReader struct {
	data *monitoringapp.AlertInstanceListData
	err  error
}

func (f *fakeInspectionAlertReader) ListAlertInstances(_ context.Context, _ *monitoringapp.AlertInstanceFilter) (*monitoringapp.AlertInstanceListData, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

type fakeInspectionHostReader struct {
	hosts map[uint]*cluster.HostInfo
}

func (f *fakeInspectionHostReader) GetHostByID(_ context.Context, id uint) (*cluster.HostInfo, error) {
	if f == nil {
		return nil, nil
	}
	return f.hosts[id], nil
}

func newInspectionEvaluationRepository(t *testing.T) *Repository {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&SeatunnelErrorGroup{}, &SeatunnelErrorEvent{}, &ClusterInspectionReport{}, &ClusterInspectionFinding{}); err != nil {
		t.Fatalf("auto migrate diagnostics error group: %v", err)
	}
	return NewRepository(database)
}

func findInspectionFindingByCode(findings []*ClusterInspectionFindingInfo, code string) *ClusterInspectionFindingInfo {
	for _, finding := range findings {
		if finding != nil && finding.CheckCode == code {
			return finding
		}
	}
	return nil
}

func TestServiceEvaluateInspectionFindings_aggregatesManagedSignals(t *testing.T) {
	repo := newInspectionEvaluationRepository(t)
	ctx := t.Context()
	now := time.Now().UTC()

	group := &SeatunnelErrorGroup{
		Fingerprint:        "fp-1",
		FingerprintVersion: DefaultFingerprintVersion,
		Title:              "DEADLINE_EXCEEDED",
		SampleMessage:      "Milvus deadline exceeded",
		OccurrenceCount:    5,
		FirstSeenAt:        now.Add(-10 * time.Minute),
		LastSeenAt:         now.Add(-2 * time.Minute),
		LastClusterID:      7,
		LastNodeID:         4,
		LastHostID:         4,
	}
	if err := repo.CreateErrorGroup(ctx, group); err != nil {
		t.Fatalf("seed error group: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := repo.CreateErrorEvent(ctx, &SeatunnelErrorEvent{
			ErrorGroupID: group.ID,
			Fingerprint:  "fp-1",
			ClusterID:    7,
			NodeID:       4,
			HostID:       4,
			AgentID:      "agent-1",
			Role:         "worker",
			SourceFile:   "/opt/seatunnel/logs/seatunnel-engine-worker.log",
			OccurredAt:   now.Add(time.Duration(-2-i) * time.Minute),
			Message:      "DEADLINE_EXCEEDED",
			Evidence:     "Milvus deadline exceeded",
		}); err != nil {
			t.Fatalf("seed error event %d: %v", i, err)
		}
	}

	service := NewServiceWithRepository(
		repo,
		&fakeInspectionClusterReader{
			status: &cluster.ClusterStatusInfo{
				ClusterID:   7,
				ClusterName: "demo-cluster",
				Nodes: []*cluster.NodeStatusInfo{
					{
						NodeID:     4,
						HostID:     4,
						HostName:   "node-a",
						HostIP:     "10.0.0.4",
						Role:       cluster.NodeRoleWorker,
						Status:     cluster.NodeStatusOffline,
						IsOnline:   false,
						ProcessPID: 0,
					},
				},
			},
		},
		&fakeInspectionEventReader{
			events: []*monitor.ProcessEvent{
				{
					ClusterID:   7,
					NodeID:      4,
					HostID:      4,
					EventType:   monitor.EventTypeRestartFailed,
					ProcessName: "seatunnel-worker",
					Details:     `{"reason":"mock restart failure"}`,
					CreatedAt:   now.Add(-5 * time.Minute),
				},
			},
		},
		&fakeInspectionAlertReader{
			data: &monitoringapp.AlertInstanceListData{
				Alerts: []*monitoringapp.AlertInstance{
					{
						AlertID:     "alert-1",
						ClusterID:   "7",
						ClusterName: "demo-cluster",
						Severity:    monitoringapp.AlertSeverityCritical,
						AlertName:   "SeatunnelNodeOffline",
						Summary:     "node offline for 5 minutes",
						Status:      monitoringapp.AlertDisplayStatusFiring,
					},
				},
			},
		},
	)

	result, err := service.EvaluateInspectionFindings(ctx, 7, 30*time.Minute, defaultInspectionErrorThreshold)
	if err != nil {
		t.Fatalf("EvaluateInspectionFindings returned error: %v", err)
	}
	if result.FindingTotal != 4 {
		t.Fatalf("expected 4 findings, got %d", result.FindingTotal)
	}
	if result.CriticalCount != 3 || result.WarningCount != 1 || result.InfoCount != 0 {
		t.Fatalf("unexpected finding counters: %+v", result)
	}

	checkCodes := make(map[string]struct{}, len(result.Findings))
	for _, finding := range result.Findings {
		checkCodes[finding.CheckCode] = struct{}{}
	}
	for _, code := range []string{
		inspectionCheckOfflineNode,
		inspectionCheckRestartFailure,
		inspectionCheckRecentErrorBurst,
		inspectionCheckActiveAlert,
	} {
		if _, ok := checkCodes[code]; !ok {
			t.Fatalf("missing finding for check code %s", code)
		}
	}
}

func TestServiceEvaluateInspectionFindings_usesRecentErrorWindowInsteadOfHistoricalTotal(t *testing.T) {
	repo := newInspectionEvaluationRepository(t)
	ctx := t.Context()
	now := time.Now().UTC()

	group := &SeatunnelErrorGroup{
		Fingerprint:        "fp-historical",
		FingerprintVersion: DefaultFingerprintVersion,
		Title:              "Historical Burst",
		SampleMessage:      "historical issue",
		OccurrenceCount:    20,
		FirstSeenAt:        now.Add(-48 * time.Hour),
		LastSeenAt:         now.Add(-2 * time.Hour),
		LastClusterID:      8,
		LastNodeID:         5,
		LastHostID:         5,
	}
	if err := repo.CreateErrorGroup(ctx, group); err != nil {
		t.Fatalf("seed historical group: %v", err)
	}
	if err := repo.CreateErrorEvent(ctx, &SeatunnelErrorEvent{
		ErrorGroupID: group.ID,
		Fingerprint:  "fp-historical",
		ClusterID:    8,
		NodeID:       5,
		HostID:       5,
		AgentID:      "agent-2",
		Role:         "worker",
		SourceFile:   "/opt/seatunnel/logs/job-1.log",
		OccurredAt:   now.Add(-2 * time.Hour),
		Message:      "historical issue",
		Evidence:     "historical issue",
	}); err != nil {
		t.Fatalf("seed historical error event: %v", err)
	}
	if err := repo.CreateErrorEvent(ctx, &SeatunnelErrorEvent{
		ErrorGroupID: group.ID,
		Fingerprint:  "fp-historical",
		ClusterID:    8,
		NodeID:       5,
		HostID:       5,
		AgentID:      "agent-2",
		Role:         "worker",
		SourceFile:   "/opt/seatunnel/logs/job-1.log",
		OccurredAt:   now.Add(-5 * time.Minute),
		Message:      "historical issue",
		Evidence:     "historical issue",
	}); err != nil {
		t.Fatalf("seed recent error event: %v", err)
	}

	service := NewServiceWithRepository(
		repo,
		&fakeInspectionClusterReader{
			status: &cluster.ClusterStatusInfo{
				ClusterID:   8,
				ClusterName: "historical-cluster",
				Nodes:       []*cluster.NodeStatusInfo{},
			},
		},
		nil,
		nil,
	)

	result, err := service.EvaluateInspectionFindings(ctx, 8, 30*time.Minute, 2)
	if err != nil {
		t.Fatalf("EvaluateInspectionFindings returned error: %v", err)
	}
	if result.FindingTotal != 0 {
		t.Fatalf("expected no recent error burst finding when recent window count is below threshold, got %d", result.FindingTotal)
	}
}

func TestServiceEvaluateInspectionFindings_respectsCustomLookbackWindow(t *testing.T) {
	repo := newInspectionEvaluationRepository(t)
	ctx := t.Context()
	now := time.Now().UTC()

	group := &SeatunnelErrorGroup{
		Fingerprint:        "fp-window",
		FingerprintVersion: DefaultFingerprintVersion,
		Title:              "Window Sensitive Error",
		SampleMessage:      "window sensitive issue",
		OccurrenceCount:    4,
		FirstSeenAt:        now.Add(-40 * time.Minute),
		LastSeenAt:         now.Add(-12 * time.Minute),
		LastClusterID:      11,
		LastNodeID:         7,
		LastHostID:         7,
	}
	if err := repo.CreateErrorGroup(ctx, group); err != nil {
		t.Fatalf("seed window group: %v", err)
	}
	for _, occurredAt := range []time.Time{
		now.Add(-12 * time.Minute),
		now.Add(-16 * time.Minute),
		now.Add(-19 * time.Minute),
		now.Add(-35 * time.Minute),
	} {
		if err := repo.CreateErrorEvent(ctx, &SeatunnelErrorEvent{
			ErrorGroupID: group.ID,
			Fingerprint:  "fp-window",
			ClusterID:    11,
			NodeID:       7,
			HostID:       7,
			AgentID:      "agent-11",
			Role:         "worker",
			SourceFile:   "/opt/seatunnel/logs/job-11.log",
			OccurredAt:   occurredAt,
			Message:      "window sensitive issue",
			Evidence:     "window sensitive issue",
		}); err != nil {
			t.Fatalf("seed custom-window error event: %v", err)
		}
	}

	service := NewServiceWithRepository(
		repo,
		&fakeInspectionClusterReader{
			status: &cluster.ClusterStatusInfo{
				ClusterID:   11,
				ClusterName: "window-cluster",
				Nodes:       []*cluster.NodeStatusInfo{},
			},
		},
		nil,
		nil,
	)

	shortWindowResult, err := service.EvaluateInspectionFindings(ctx, 11, 10*time.Minute, defaultInspectionErrorThreshold)
	if err != nil {
		t.Fatalf("EvaluateInspectionFindings short window returned error: %v", err)
	}
	if shortWindowResult.FindingTotal != 0 {
		t.Fatalf("expected no recent burst within 10 minutes, got %d", shortWindowResult.FindingTotal)
	}

	longWindowResult, err := service.EvaluateInspectionFindings(ctx, 11, 20*time.Minute, defaultInspectionErrorThreshold)
	if err != nil {
		t.Fatalf("EvaluateInspectionFindings long window returned error: %v", err)
	}
	if longWindowResult.FindingTotal != 1 {
		t.Fatalf("expected one recent burst within 20 minutes, got %d", longWindowResult.FindingTotal)
	}
}

func TestServiceEvaluateInspectionFindings_handlesSQLiteOffsetTimestamps(t *testing.T) {
	repo := newInspectionEvaluationRepository(t)
	ctx := t.Context()
	nowUTC := time.Now().UTC()
	cst := time.FixedZone("UTC+8", 8*3600)

	group := &SeatunnelErrorGroup{
		Fingerprint:        "fp-offset",
		FingerprintVersion: DefaultFingerprintVersion,
		Title:              "Offset Timestamp Error",
		SampleMessage:      "offset timestamp issue",
		OccurrenceCount:    1,
		FirstSeenAt:        nowUTC.Add(-2 * time.Hour).In(cst),
		LastSeenAt:         nowUTC.Add(-2 * time.Hour).In(cst),
		LastClusterID:      12,
		LastNodeID:         8,
		LastHostID:         8,
	}
	if err := repo.CreateErrorGroup(ctx, group); err != nil {
		t.Fatalf("seed offset group: %v", err)
	}
	if err := repo.CreateErrorEvent(ctx, &SeatunnelErrorEvent{
		ErrorGroupID: group.ID,
		Fingerprint:  "fp-offset",
		ClusterID:    12,
		NodeID:       8,
		HostID:       8,
		AgentID:      "agent-12",
		Role:         "worker",
		SourceFile:   "/opt/seatunnel/logs/job-12.log",
		OccurredAt:   nowUTC.Add(-2 * time.Hour).In(cst),
		Message:      "offset timestamp issue",
		Evidence:     "offset timestamp issue",
	}); err != nil {
		t.Fatalf("seed offset timestamp error event: %v", err)
	}

	service := NewServiceWithRepository(
		repo,
		&fakeInspectionClusterReader{
			status: &cluster.ClusterStatusInfo{
				ClusterID:   12,
				ClusterName: "offset-cluster",
				Nodes:       []*cluster.NodeStatusInfo{},
			},
		},
		nil,
		nil,
	)

	result, err := service.EvaluateInspectionFindings(ctx, 12, 30*time.Minute, defaultInspectionErrorThreshold)
	if err != nil {
		t.Fatalf("EvaluateInspectionFindings with offset timestamps returned error: %v", err)
	}
	if result.FindingTotal != 0 {
		t.Fatalf("expected no recent burst for offset timestamp older than 30 minutes, got %d", result.FindingTotal)
	}
}

func TestServiceStartInspection_persistsCompletedReportAndSupportsSeverityFilter(t *testing.T) {
	repo := newInspectionEvaluationRepository(t)
	ctx := t.Context()
	now := time.Now().UTC()

	group := &SeatunnelErrorGroup{
		Fingerprint:        "fp-start",
		FingerprintVersion: DefaultFingerprintVersion,
		Title:              "Connection Timeout",
		SampleMessage:      "connection timeout",
		OccurrenceCount:    5,
		FirstSeenAt:        now.Add(-15 * time.Minute),
		LastSeenAt:         now.Add(-1 * time.Minute),
		LastClusterID:      9,
		LastNodeID:         6,
		LastHostID:         6,
	}
	if err := repo.CreateErrorGroup(ctx, group); err != nil {
		t.Fatalf("seed error group: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := repo.CreateErrorEvent(ctx, &SeatunnelErrorEvent{
			ErrorGroupID: group.ID,
			Fingerprint:  "fp-start",
			ClusterID:    9,
			NodeID:       6,
			HostID:       6,
			AgentID:      "agent-9",
			Role:         "worker",
			SourceFile:   "/opt/seatunnel/logs/job-9.log",
			OccurredAt:   now.Add(time.Duration(-1-i) * time.Minute),
			Message:      "connection timeout",
			Evidence:     "connection timeout",
		}); err != nil {
			t.Fatalf("seed error event %d: %v", i, err)
		}
	}

	service := NewServiceWithRepository(
		repo,
		&fakeInspectionClusterReader{
			cluster: &cluster.Cluster{ID: 9, Name: "cluster-9"},
			status: &cluster.ClusterStatusInfo{
				ClusterID:   9,
				ClusterName: "cluster-9",
				Nodes: []*cluster.NodeStatusInfo{
					{
						NodeID:     6,
						HostID:     6,
						HostName:   "node-9",
						HostIP:     "10.0.0.9",
						Role:       cluster.NodeRoleWorker,
						Status:     cluster.NodeStatusOffline,
						IsOnline:   false,
						ProcessPID: 0,
					},
				},
			},
		},
		&fakeInspectionEventReader{
			events: []*monitor.ProcessEvent{
				{
					ClusterID:   9,
					NodeID:      6,
					HostID:      6,
					EventType:   monitor.EventTypeRestartFailed,
					ProcessName: "seatunnel-worker",
					Details:     `{"reason":"restart failed"}`,
					CreatedAt:   now.Add(-3 * time.Minute),
				},
			},
		},
		&fakeInspectionAlertReader{
			data: &monitoringapp.AlertInstanceListData{
				Alerts: []*monitoringapp.AlertInstance{
					{
						AlertID:     "alert-9",
						ClusterID:   "9",
						ClusterName: "cluster-9",
						Severity:    monitoringapp.AlertSeverityCritical,
						AlertName:   "SeatunnelNodeOffline",
						Summary:     "node offline",
						Status:      monitoringapp.AlertDisplayStatusFiring,
					},
				},
			},
		},
	)
	service.SetHostReader(&fakeInspectionHostReader{
		hosts: map[uint]*cluster.HostInfo{
			6: {ID: 6, Name: "worker-6", IPAddress: "10.0.0.6"},
		},
	})

	detail, err := service.StartInspection(ctx, &StartClusterInspectionRequest{
		ClusterID:       9,
		TriggerSource:   InspectionTriggerSourceDiagnosticsWorkspace,
		LookbackMinutes: 60,
		ErrorThreshold:  4,
	}, 99, "alice")
	if err != nil {
		t.Fatalf("StartInspection returned error: %v", err)
	}
	if detail.Report == nil {
		t.Fatalf("expected inspection report detail")
	}
	if detail.Report.Status != InspectionReportStatusCompleted {
		t.Fatalf("expected completed report, got %s", detail.Report.Status)
	}
	if detail.Report.TriggerSource != InspectionTriggerSourceDiagnosticsWorkspace {
		t.Fatalf("expected diagnostics_workspace trigger source, got %s", detail.Report.TriggerSource)
	}
	if detail.Report.LookbackMinutes != 60 {
		t.Fatalf("expected lookback_minutes=60, got %d", detail.Report.LookbackMinutes)
	}
	if detail.Report.ErrorThreshold != 4 {
		t.Fatalf("expected error_threshold=4, got %d", detail.Report.ErrorThreshold)
	}
	if !strings.Contains(detail.Report.Summary, "60") {
		t.Fatalf("expected report summary to include lookback window, got %s", detail.Report.Summary)
	}
	if detail.Report.RequestedByUserID != 99 || detail.Report.RequestedBy != "alice" {
		t.Fatalf("unexpected requester: %+v", detail.Report)
	}
	if detail.Report.FindingTotal != 4 || len(detail.Findings) != 4 {
		t.Fatalf("expected 4 persisted findings, got report=%d detail=%d", detail.Report.FindingTotal, len(detail.Findings))
	}
	warningFinding := findInspectionFindingByCode(detail.Findings, inspectionCheckRecentErrorBurst)
	if warningFinding == nil {
		t.Fatalf("expected recent error burst finding in detail")
	}
	if warningFinding.RelatedNodeID != 6 || warningFinding.RelatedHostID != 6 {
		t.Fatalf("expected warning finding node scope from recent event, got %+v", warningFinding)
	}
	if warningFinding.RelatedHostName != "worker-6" || warningFinding.RelatedHostIP != "10.0.0.6" {
		t.Fatalf("expected warning finding host display, got %+v", warningFinding)
	}

	reports, err := service.ListInspectionReports(ctx, &ClusterInspectionReportFilter{
		ClusterID: 9,
		Severity:  InspectionFindingSeverityWarning,
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("ListInspectionReports returned error: %v", err)
	}
	if reports.Total != 1 || len(reports.Items) != 1 {
		t.Fatalf("expected 1 warning-filtered report, got total=%d len=%d", reports.Total, len(reports.Items))
	}
	if reports.Items[0].ErrorThreshold != 4 {
		t.Fatalf("expected listed report error_threshold=4, got %d", reports.Items[0].ErrorThreshold)
	}
}

func TestServiceStartInspection_marksFailedReportWhenEvaluationFails(t *testing.T) {
	repo := newInspectionEvaluationRepository(t)
	ctx := t.Context()

	service := NewServiceWithRepository(
		repo,
		&fakeInspectionClusterReader{
			cluster: &cluster.Cluster{ID: 10, Name: "cluster-10"},
			status: &cluster.ClusterStatusInfo{
				ClusterID:   10,
				ClusterName: "cluster-10",
				Nodes:       []*cluster.NodeStatusInfo{},
			},
		},
		&fakeInspectionEventReader{},
		&fakeInspectionAlertReader{err: errors.New("alert backend unavailable")},
	)

	detail, err := service.StartInspection(ctx, &StartClusterInspectionRequest{
		ClusterID:       10,
		TriggerSource:   InspectionTriggerSourceClusterDetail,
		LookbackMinutes: 15,
	}, 100, "bob")
	if err != nil {
		t.Fatalf("StartInspection returned error: %v", err)
	}
	if detail.Report == nil {
		t.Fatalf("expected failed inspection report detail")
	}
	if detail.Report.Status != InspectionReportStatusFailed {
		t.Fatalf("expected failed report, got %s", detail.Report.Status)
	}
	if !strings.Contains(detail.Report.ErrorMessage, "alert backend unavailable") {
		t.Fatalf("expected failure reason to be persisted, got %s", detail.Report.ErrorMessage)
	}
	if len(detail.Findings) != 0 {
		t.Fatalf("expected no persisted findings for failed inspection, got %d", len(detail.Findings))
	}
	if detail.Report.LookbackMinutes != 15 {
		t.Fatalf("expected failed report to persist lookback_minutes=15, got %d", detail.Report.LookbackMinutes)
	}
	if detail.Report.ErrorThreshold != defaultInspectionErrorThreshold {
		t.Fatalf("expected failed report to persist default error_threshold=%d, got %d", defaultInspectionErrorThreshold, detail.Report.ErrorThreshold)
	}
}
