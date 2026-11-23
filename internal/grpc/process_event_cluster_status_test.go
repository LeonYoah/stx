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

package grpc

import (
	"context"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/agent"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"go.uber.org/zap"
)

type processEventMonitorStub struct {
	recorded []monitor.ProcessEventType
}

func (s *processEventMonitorStub) GetConfig(context.Context, uint) (*monitor.MonitorConfig, error) {
	return nil, nil
}

func (s *processEventMonitorStub) RecordEventFromReport(
	_ context.Context,
	_, _, _ uint,
	eventType monitor.ProcessEventType,
	_ int,
	_, _, _ string,
	_ map[string]string,
) error {
	s.recorded = append(s.recorded, eventType)
	return nil
}

type processEventNodeProviderStub struct {
	clusterID      uint
	nodeID         uint
	updatedPID     int
	updatedStatus  string
	refreshCalls   []uint
	updateErr      error
}

func (p *processEventNodeProviderStub) GetNodeByHostAndInstallDirAndRole(
	context.Context, uint, string, string,
) (uint, uint, bool, error) {
	return p.clusterID, p.nodeID, true, nil
}

func (p *processEventNodeProviderStub) GetNodesByHostID(context.Context, uint) ([]*NodeWithMonitorConfig, error) {
	return nil, nil
}

func (p *processEventNodeProviderStub) UpdateNodeProcessStatus(_ context.Context, _ uint, pid int, status string) error {
	if p.updateErr != nil {
		return p.updateErr
	}
	p.updatedPID = pid
	p.updatedStatus = status
	return nil
}

func (p *processEventNodeProviderStub) RefreshClusterStatusFromNodes(_ context.Context, clusterID uint) {
	p.refreshCalls = append(p.refreshCalls, clusterID)
}

func (p *processEventNodeProviderStub) GetClusterNodeDisplayInfo(context.Context, uint, uint) (string, string) {
	return "cluster", "host - role"
}

func TestHandleProcessEventReportRefreshClusterStatusOnRestarted(t *testing.T) {
	manager := agent.NewManager(nil)
	_, err := manager.RegisterAgent(context.Background(), &pb.RegisterRequest{
		AgentId:   "agent-restart-status",
		Hostname:  "host-1",
		IpAddress: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	conn, ok := manager.GetAgent("agent-restart-status")
	if !ok {
		t.Fatal("agent missing after register")
	}
	conn.HostID = 42

	provider := &processEventNodeProviderStub{clusterID: 7, nodeID: 9}
	monitorStub := &processEventMonitorStub{}

	prevProvider := clusterNodeProvider
	prevMonitor := monitorService
	clusterNodeProvider = provider
	monitorService = monitorStub
	t.Cleanup(func() {
		clusterNodeProvider = prevProvider
		monitorService = prevMonitor
	})

	server := &Server{agentManager: manager, logger: zap.NewNop()}
	report := &pb.ProcessEventReport{
		EventType:   pb.ProcessEventType_PROCESS_RESTARTED,
		Pid:         3143723,
		ProcessName: "stx181-install-1",
		InstallDir:  "/data/seatunnel",
		Role:        "master/worker",
	}
	if err := server.HandleProcessEventReport(context.Background(), "agent-restart-status", report); err != nil {
		t.Fatalf("HandleProcessEventReport: %v", err)
	}

	if len(monitorStub.recorded) != 1 || monitorStub.recorded[0] != monitor.EventTypeRestarted {
		t.Fatalf("recorded events = %#v", monitorStub.recorded)
	}
	if provider.updatedPID != 3143723 || provider.updatedStatus != "running" {
		t.Fatalf("node update pid=%d status=%q", provider.updatedPID, provider.updatedStatus)
	}
	if len(provider.refreshCalls) != 1 || provider.refreshCalls[0] != 7 {
		t.Fatalf("RefreshClusterStatusFromNodes calls = %#v, want [7]", provider.refreshCalls)
	}
}

func TestHandleProcessEventReportSkipsRefreshWhenNodeUpdateFails(t *testing.T) {
	manager := agent.NewManager(nil)
	_, err := manager.RegisterAgent(context.Background(), &pb.RegisterRequest{
		AgentId:   "agent-restart-fail",
		Hostname:  "host-1",
		IpAddress: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	conn, ok := manager.GetAgent("agent-restart-fail")
	if !ok {
		t.Fatal("agent missing after register")
	}
	conn.HostID = 42

	provider := &processEventNodeProviderStub{
		clusterID: 3,
		nodeID:    4,
		updateErr: context.DeadlineExceeded,
	}
	prevProvider := clusterNodeProvider
	prevMonitor := monitorService
	clusterNodeProvider = provider
	monitorService = &processEventMonitorStub{}
	t.Cleanup(func() {
		clusterNodeProvider = prevProvider
		monitorService = prevMonitor
	})

	server := &Server{agentManager: manager, logger: zap.NewNop()}
	err = server.HandleProcessEventReport(context.Background(), "agent-restart-fail", &pb.ProcessEventReport{
		EventType:   pb.ProcessEventType_PROCESS_RESTARTED,
		Pid:         1,
		ProcessName: "p",
		InstallDir:  "/data/seatunnel",
		Role:        "master/worker",
	})
	if err != nil {
		t.Fatalf("HandleProcessEventReport: %v", err)
	}
	if len(provider.refreshCalls) != 0 {
		t.Fatalf("unexpected refresh calls: %#v", provider.refreshCalls)
	}
}
