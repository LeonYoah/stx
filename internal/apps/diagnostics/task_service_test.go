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
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type fakeDiagnosticTaskClusterReader struct {
	cluster *cluster.Cluster
	err     error
}

func (f *fakeDiagnosticTaskClusterReader) List(_ context.Context, _ *cluster.ClusterFilter) ([]*cluster.Cluster, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	if f.cluster == nil {
		return []*cluster.Cluster{}, 0, nil
	}
	return []*cluster.Cluster{f.cluster}, 1, nil
}

func (f *fakeDiagnosticTaskClusterReader) Get(_ context.Context, _ uint) (*cluster.Cluster, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cluster, nil
}

func (f *fakeDiagnosticTaskClusterReader) GetStatus(_ context.Context, _ uint) (*cluster.ClusterStatusInfo, error) {
	return &cluster.ClusterStatusInfo{}, nil
}

type fakeDiagnosticTaskHostReader struct {
	hosts map[uint]*cluster.HostInfo
}

func (f *fakeDiagnosticTaskHostReader) GetHostByID(_ context.Context, id uint) (*cluster.HostInfo, error) {
	return f.hosts[id], nil
}

func newDiagnosticTaskServiceRepository(t *testing.T) *Repository {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(
		&DiagnosticTask{},
		&DiagnosticTaskStep{},
		&DiagnosticNodeExecution{},
		&DiagnosticStepLog{},
		&ClusterInspectionReport{},
		&ClusterInspectionFinding{},
		&SeatunnelErrorGroup{},
	); err != nil {
		t.Fatalf("auto migrate diagnostics task service models: %v", err)
	}
	return NewRepository(database)
}

func TestServiceCreateDiagnosticTaskManualUsesSelectedNodesAndHostContext(t *testing.T) {
	repo := newDiagnosticTaskServiceRepository(t)
	service := NewServiceWithRepository(
		repo,
		&fakeDiagnosticTaskClusterReader{
			cluster: &cluster.Cluster{
				ID:         7,
				Name:       "demo-cluster",
				InstallDir: "/opt/seatunnel-cluster",
				Nodes: []cluster.ClusterNode{
					{ID: 1, HostID: 11, Role: cluster.NodeRoleMaster, InstallDir: "/opt/seatunnel-master"},
					{ID: 2, HostID: 12, Role: cluster.NodeRoleWorker},
				},
			},
		},
		nil,
		nil,
	)
	service.SetHostReader(&fakeDiagnosticTaskHostReader{
		hosts: map[uint]*cluster.HostInfo{
			11: {ID: 11, Name: "master-a", IPAddress: "10.0.0.11", AgentID: "agent-11"},
			12: {ID: 12, Name: "worker-b", IPAddress: "10.0.0.12", AgentID: "agent-12"},
		},
	})

	task, err := service.CreateDiagnosticTask(t.Context(), &CreateDiagnosticTaskRequest{
		ClusterID:       7,
		TriggerSource:   DiagnosticTaskSourceManual,
		SelectedNodeIDs: []uint{2},
	}, 1, "admin")
	if err != nil {
		t.Fatalf("CreateDiagnosticTask manual returned error: %v", err)
	}

	if task.Status != DiagnosticTaskStatusReady {
		t.Fatalf("expected ready task status, got %s", task.Status)
	}
	if task.TriggerSource != DiagnosticTaskSourceManual {
		t.Fatalf("unexpected task trigger source: %s", task.TriggerSource)
	}
	if len(task.SelectedNodes) != 1 {
		t.Fatalf("expected 1 selected node, got %d", len(task.SelectedNodes))
	}
	if task.SelectedNodes[0].NodeID != 2 || task.SelectedNodes[0].AgentID != "agent-12" {
		t.Fatalf("unexpected selected node snapshot: %+v", task.SelectedNodes[0])
	}
	if task.SelectedNodes[0].InstallDir != "/opt/seatunnel-cluster" {
		t.Fatalf("expected install dir fallback to cluster install dir, got %s", task.SelectedNodes[0].InstallDir)
	}
	if len(task.Steps) != len(DefaultDiagnosticTaskSteps()) {
		t.Fatalf("expected %d steps, got %d", len(DefaultDiagnosticTaskSteps()), len(task.Steps))
	}
	if len(task.NodeExecutions) != 1 {
		t.Fatalf("expected 1 node execution, got %d", len(task.NodeExecutions))
	}
	if task.NodeExecutions[0].CurrentStep != DefaultDiagnosticTaskSteps()[0].Code {
		t.Fatalf("unexpected initial node step: %+v", task.NodeExecutions[0])
	}
}

func TestServiceCreateDiagnosticTaskFromInspectionFindingInfersClusterAndNode(t *testing.T) {
	repo := newDiagnosticTaskServiceRepository(t)
	ctx := t.Context()

	report := &ClusterInspectionReport{
		ClusterID:     9,
		Status:        InspectionReportStatusCompleted,
		TriggerSource: InspectionTriggerSourceManual,
		Summary:       "inspection complete",
	}
	if err := repo.CreateInspectionReport(ctx, report); err != nil {
		t.Fatalf("seed inspection report: %v", err)
	}
	finding := &ClusterInspectionFinding{
		ReportID:       report.ID,
		ClusterID:      9,
		Severity:       InspectionFindingSeverityCritical,
		Category:       "runtime",
		CheckCode:      "check-runtime",
		CheckName:      "Runtime check",
		Summary:        "worker is unhealthy",
		RelatedNodeID:  1,
		RelatedHostID:  101,
		RelatedAlertID: "alert-1",
	}
	if err := repo.CreateInspectionFindings(ctx, []*ClusterInspectionFinding{finding}); err != nil {
		t.Fatalf("seed inspection finding: %v", err)
	}

	service := NewServiceWithRepository(
		repo,
		&fakeDiagnosticTaskClusterReader{
			cluster: &cluster.Cluster{
				ID:         9,
				Name:       "cluster-nine",
				InstallDir: "/opt/seatunnel-nine",
				Nodes: []cluster.ClusterNode{
					{ID: 1, HostID: 101, Role: cluster.NodeRoleWorker, InstallDir: "/opt/seatunnel-nine/worker"},
					{ID: 2, HostID: 102, Role: cluster.NodeRoleMaster, InstallDir: "/opt/seatunnel-nine/master"},
				},
			},
		},
		nil,
		nil,
	)
	service.SetHostReader(&fakeDiagnosticTaskHostReader{
		hosts: map[uint]*cluster.HostInfo{
			101: {ID: 101, Name: "worker-a", IPAddress: "10.0.0.101", AgentID: "agent-101"},
			102: {ID: 102, Name: "master-a", IPAddress: "10.0.0.102", AgentID: "agent-102"},
		},
	})

	task, err := service.CreateDiagnosticTask(ctx, &CreateDiagnosticTaskRequest{
		TriggerSource: DiagnosticTaskSourceInspectionFinding,
		SourceRef: DiagnosticTaskSourceRef{
			InspectionFindingID: finding.ID,
		},
	}, 2, "operator")
	if err != nil {
		t.Fatalf("CreateDiagnosticTask inspection finding returned error: %v", err)
	}

	if task.ClusterID != 9 {
		t.Fatalf("expected cluster 9, got %d", task.ClusterID)
	}
	if task.SourceRef.InspectionReportID != report.ID {
		t.Fatalf("expected report id %d, got %+v", report.ID, task.SourceRef)
	}
	if len(task.SelectedNodes) != 1 || task.SelectedNodes[0].NodeID != 1 {
		t.Fatalf("expected selected node inferred from finding, got %+v", task.SelectedNodes)
	}
	if task.Summary == "" {
		t.Fatalf("expected auto-generated summary")
	}
}

func TestServiceCreateDiagnosticTaskFromInspectionReportWithoutFinding(t *testing.T) {
	repo := newDiagnosticTaskServiceRepository(t)
	ctx := t.Context()
	report := &ClusterInspectionReport{
		ClusterID:     12,
		Status:        InspectionReportStatusCompleted,
		TriggerSource: InspectionTriggerSourceAuto,
		Summary:       "healthy inspection",
	}
	if err := repo.CreateInspectionReport(ctx, report); err != nil {
		t.Fatalf("seed inspection report: %v", err)
	}

	service := NewServiceWithRepository(
		repo,
		&fakeDiagnosticTaskClusterReader{cluster: &cluster.Cluster{
			ID:         12,
			Name:       "cluster-twelve",
			InstallDir: "/opt/seatunnel-twelve",
			Nodes: []cluster.ClusterNode{
				{ID: 1, HostID: 301, Role: cluster.NodeRoleWorker, InstallDir: "/opt/seatunnel-twelve/worker"},
			},
		}},
		nil,
		nil,
	)
	service.SetHostReader(&fakeDiagnosticTaskHostReader{hosts: map[uint]*cluster.HostInfo{
		301: {ID: 301, Name: "worker-301", IPAddress: "10.0.0.301", AgentID: "agent-301"},
	}})

	task, err := service.CreateDiagnosticTask(ctx, &CreateDiagnosticTaskRequest{
		TriggerSource: DiagnosticTaskSourceInspectionFinding,
		SourceRef: DiagnosticTaskSourceRef{
			InspectionReportID: report.ID,
		},
		Options: DefaultDiagnosticTaskOptions(),
	}, 3, "operator")
	if err != nil {
		t.Fatalf("CreateDiagnosticTask inspection report returned error: %v", err)
	}
	if task.ClusterID != report.ClusterID || task.SourceRef.InspectionReportID != report.ID {
		t.Fatalf("unexpected inspection report task linkage: %+v", task)
	}
	if len(task.SelectedNodes) != 1 || task.SelectedNodes[0].NodeID != 1 {
		t.Fatalf("expected all cluster nodes for report-only task, got %+v", task.SelectedNodes)
	}
	if task.Summary == "" || task.SourceRef.InspectionFindingID != 0 {
		t.Fatalf("unexpected report-only task summary or source: %+v", task)
	}
}

func TestServiceCreateDiagnosticTaskMarksOptionalDumpStepsSkippedByDefault(t *testing.T) {
	repo := newDiagnosticTaskServiceRepository(t)
	service := NewServiceWithRepository(
		repo,
		&fakeDiagnosticTaskClusterReader{
			cluster: &cluster.Cluster{
				ID:   10,
				Name: "cluster-ten",
				Nodes: []cluster.ClusterNode{
					{ID: 1, HostID: 201, Role: cluster.NodeRoleWorker, InstallDir: "/opt/seatunnel"},
				},
			},
		},
		nil,
		nil,
	)
	service.SetHostReader(&fakeDiagnosticTaskHostReader{
		hosts: map[uint]*cluster.HostInfo{
			201: {ID: 201, Name: "worker-201", IPAddress: "10.0.0.201", AgentID: "agent-201"},
		},
	})

	task, err := service.CreateDiagnosticTask(t.Context(), &CreateDiagnosticTaskRequest{
		ClusterID:     10,
		TriggerSource: DiagnosticTaskSourceManual,
	}, 1, "admin")
	if err != nil {
		t.Fatalf("CreateDiagnosticTask returned error: %v", err)
	}

	statusByCode := make(map[DiagnosticStepCode]DiagnosticTaskStatus, len(task.Steps))
	for _, step := range task.Steps {
		statusByCode[step.Code] = step.Status
	}
	if statusByCode[DiagnosticStepCodeCollectThreadDump] != DiagnosticTaskStatusSkipped {
		t.Fatalf("expected thread dump step skipped, got %s", statusByCode[DiagnosticStepCodeCollectThreadDump])
	}
	if statusByCode[DiagnosticStepCodeCollectJVMDump] != DiagnosticTaskStatusSkipped {
		t.Fatalf("expected jvm dump step skipped, got %s", statusByCode[DiagnosticStepCodeCollectJVMDump])
	}
}

func TestBuildDiagnosticLogCandidatesPrefersNodeScopedEvents(t *testing.T) {
	target := DiagnosticTaskNodeTarget{
		NodeID:     101,
		HostID:     21,
		Role:       "worker",
		InstallDir: "/opt/seatunnel-a",
	}
	events := []*SeatunnelErrorEvent{
		{
			NodeID:     101,
			HostID:     21,
			Role:       "worker",
			InstallDir: "/opt/seatunnel-a",
			SourceFile: "/opt/seatunnel-a/logs/node-101.log",
			OccurredAt: time.Now().UTC(),
		},
		{
			NodeID:     102,
			HostID:     21,
			Role:       "worker",
			InstallDir: "/opt/seatunnel-a",
			SourceFile: "/opt/seatunnel-a/logs/node-102.log",
			OccurredAt: time.Now().UTC(),
		},
		{
			NodeID:     0,
			HostID:     21,
			Role:       "master",
			InstallDir: "/opt/seatunnel-a",
			SourceFile: "/opt/seatunnel-a/logs/master.log",
			OccurredAt: time.Now().UTC(),
		},
	}

	candidates := buildDiagnosticLogCandidates(target, events)
	if len(candidates) == 0 {
		t.Fatalf("expected at least one candidate")
	}
	if candidates[0] != "/opt/seatunnel-a/logs/node-101.log" {
		t.Fatalf("expected node-specific candidate first, got %v", candidates)
	}
	for _, candidate := range candidates {
		if candidate == "/opt/seatunnel-a/logs/node-102.log" || candidate == "/opt/seatunnel-a/logs/master.log" {
			t.Fatalf("unexpected unrelated candidate included: %v", candidates)
		}
	}
}
