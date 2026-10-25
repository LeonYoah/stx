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
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/agent"
	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/processidentity"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"go.uber.org/zap"
	grpclib "google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type trackedProcessParam struct {
	PID        int    `json:"pid"`
	Name       string `json:"name"`
	ClusterID  uint   `json:"cluster_id"`
	InstallDir string `json:"install_dir"`
	Role       string `json:"role"`
}

func decodeTracked(t *testing.T, params map[string]string) []trackedProcessParam {
	t.Helper()
	raw, ok := params["tracked_processes"]
	if !ok {
		t.Fatal("tracked_processes must always be present so the Agent replaces this cluster's entries")
	}
	var tracked []trackedProcessParam
	if err := json.Unmarshal([]byte(raw), &tracked); err != nil {
		t.Fatalf("decode tracked_processes %q: %v", raw, err)
	}
	return tracked
}

func TestBuildClusterMonitorParamsOnlyIncludesTargetCluster(t *testing.T) {
	cluster6Config := &monitor.MonitorConfig{ClusterID: 6, AutoMonitor: true, AutoRestart: false, MonitorInterval: 7, ConfigVersion: 3}
	nodes := []*NodeWithMonitorConfig{
		{ClusterID: 6, NodeID: 61, InstallDir: "/tmp/seatunnel-2.3.13", Role: "master/worker", ProcessPID: 606, MonitorConfig: cluster6Config},
		{ClusterID: 8, NodeID: 81, InstallDir: "/tmp/seatunnel-2.3.12", Role: "master/worker", ProcessPID: 808},
		{ClusterID: 6, NodeID: 62, InstallDir: "", Role: "worker"},
	}

	params, count, err := buildClusterMonitorParams(6, nodes, nil)
	if err != nil {
		t.Fatalf("buildClusterMonitorParams: %v", err)
	}
	if params["cluster_id"] != "6" {
		t.Fatalf("cluster_id must be the target cluster, got %q", params["cluster_id"])
	}
	if params["auto_restart"] != "false" || params["monitor_interval"] != "7" || params["config_version"] != "3" {
		t.Fatalf("monitor config should come from the cluster's nodes, got %+v", params)
	}
	tracked := decodeTracked(t, params)
	if count != 1 || len(tracked) != 1 {
		t.Fatalf("expected exactly one tracked process, got %d %+v", count, tracked)
	}
	if tracked[0].ClusterID != 6 || tracked[0].PID != 606 || tracked[0].Name != processidentity.ManagedName("/tmp/seatunnel-2.3.13", "master/worker") {
		t.Fatalf("unexpected tracked process: %+v", tracked[0])
	}
}

func TestBuildClusterMonitorParamsEmptyListWhenClusterHasNoNodeOnHost(t *testing.T) {
	nodes := []*NodeWithMonitorConfig{
		{ClusterID: 8, NodeID: 81, InstallDir: "/tmp/seatunnel-2.3.12", Role: "master/worker", ProcessPID: 808},
	}
	fallback := &monitor.MonitorConfig{ClusterID: 6, AutoMonitor: true, AutoRestart: true, MonitorInterval: 9, ConfigVersion: 5}

	params, count, err := buildClusterMonitorParams(6, nodes, fallback)
	if err != nil {
		t.Fatalf("buildClusterMonitorParams: %v", err)
	}
	if params["tracked_processes"] != "[]" || count != 0 {
		t.Fatalf("expected an explicit empty list, got %q (count=%d)", params["tracked_processes"], count)
	}
	if params["cluster_id"] != "6" || params["monitor_interval"] != "9" {
		t.Fatalf("fallback cluster config should be used, got %+v", params)
	}

	defaults, _, err := buildClusterMonitorParams(6, nil, nil)
	if err != nil {
		t.Fatalf("buildClusterMonitorParams: %v", err)
	}
	if defaults["tracked_processes"] != "[]" || defaults["auto_monitor"] != "true" {
		t.Fatalf("defaults should keep auto_monitor=true with an empty list, got %+v", defaults)
	}
}

// recordingCommandStream 记录下发的命令，并立即以 SUCCESS 回应。
// recordingCommandStream records dispatched commands and immediately answers SUCCESS.
type recordingCommandStream struct {
	grpclib.BidiStreamingServer[pb.CommandResponse, pb.CommandRequest]
	manager *agent.Manager
	mu      sync.Mutex
	sent    []*pb.CommandRequest
}

func (s *recordingCommandStream) Send(req *pb.CommandRequest) error {
	s.mu.Lock()
	s.sent = append(s.sent, req)
	s.mu.Unlock()
	go s.manager.HandleCommandResponse(&pb.CommandResponse{CommandId: req.CommandId, Status: pb.CommandStatus_SUCCESS})
	return nil
}

func TestPushClusterMonitorConfigToHostSendsEmptyListForRemovedCluster(t *testing.T) {
	ctx := context.Background()
	database, err := gorm.Open(sqlite.Open("file:cluster_monitor_push?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&host.Host{}); err != nil {
		t.Fatalf("migrate host: %v", err)
	}
	hostRepo := host.NewRepository(database)
	target := &host.Host{Name: "t2", HostType: host.HostTypeBareMetal, IPAddress: "10.66.0.1", AgentID: "agent-t2"}
	if err := hostRepo.Create(ctx, target); err != nil {
		t.Fatalf("create host: %v", err)
	}

	manager := agent.NewManager(nil)
	if _, err := manager.RegisterAgent(ctx, &pb.RegisterRequest{AgentId: "agent-t2", IpAddress: "10.66.0.1"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	stream := &recordingCommandStream{manager: manager}
	if err := manager.SetAgentStream("agent-t2", stream); err != nil {
		t.Fatalf("SetAgentStream: %v", err)
	}

	provider := &processIsolationNodeProvider{
		nodes: []*NodeWithMonitorConfig{
			{ClusterID: 8, NodeID: 81, InstallDir: "/tmp/seatunnel-2.3.12", Role: "master/worker", ProcessPID: 808},
		},
		updates: map[uint]*pb.ProcessStatus{},
	}
	previousProvider := clusterNodeProvider
	clusterNodeProvider = provider
	t.Cleanup(func() { clusterNodeProvider = previousProvider })

	server := &Server{agentManager: manager, hostService: host.NewService(hostRepo, nil, nil), logger: zap.NewNop()}
	if err := server.PushClusterMonitorConfigToHost(ctx, 6, target.ID); err != nil {
		t.Fatalf("PushClusterMonitorConfigToHost: %v", err)
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if len(stream.sent) != 1 {
		t.Fatalf("expected one command, got %d", len(stream.sent))
	}
	req := stream.sent[0]
	if req.Type != pb.CommandType_UPDATE_MONITOR_CONFIG {
		t.Fatalf("unexpected command type %s", req.Type)
	}
	if req.Parameters["cluster_id"] != "6" || req.Parameters["tracked_processes"] != "[]" {
		t.Fatalf("expected cluster 6 with an empty list, got %+v", req.Parameters)
	}
}

func TestPushClusterMonitorConfigToHostValidatesInput(t *testing.T) {
	server := &Server{logger: zap.NewNop()}
	if err := server.PushClusterMonitorConfigToHost(context.Background(), 0, 1); err == nil {
		t.Fatal("expected error for missing cluster id")
	}
	if err := server.PushClusterMonitorConfigToHost(context.Background(), 1, 1); !errors.Is(err, ErrClusterMonitorPushUnavailable) {
		t.Fatalf("expected ErrClusterMonitorPushUnavailable, got %v", err)
	}
}
