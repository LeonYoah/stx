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

	"github.com/LeonYoah/stx/internal/processidentity"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"go.uber.org/zap"
)

type processIsolationNodeProvider struct {
	nodes   []*NodeWithMonitorConfig
	updates map[uint]*pb.ProcessStatus
}

func (p *processIsolationNodeProvider) GetNodeByHostAndInstallDirAndRole(context.Context, uint, string, string) (uint, uint, bool, error) {
	return 0, 0, false, nil
}

func (p *processIsolationNodeProvider) GetNodesByHostID(context.Context, uint) ([]*NodeWithMonitorConfig, error) {
	return p.nodes, nil
}

func (p *processIsolationNodeProvider) UpdateNodeProcessStatus(_ context.Context, nodeID uint, pid int, status string) error {
	p.updates[nodeID] = &pb.ProcessStatus{Pid: int32(pid), Status: status}
	return nil
}

func (p *processIsolationNodeProvider) RefreshClusterStatusFromNodes(context.Context, uint) {}

func (p *processIsolationNodeProvider) GetClusterNodeDisplayInfo(context.Context, uint, uint) (string, string) {
	return "", ""
}

func TestUpdateProcessStatusFromHeartbeatSeparatesInstallDirectories(t *testing.T) {
	provider := &processIsolationNodeProvider{
		nodes: []*NodeWithMonitorConfig{
			{ClusterID: 6, NodeID: 6, InstallDir: "/tmp/seatunnel-2.3.13", Role: "master/worker"},
			{ClusterID: 8, NodeID: 9, InstallDir: "/tmp/seatunnel-2.3.12", Role: "master/worker"},
		},
		updates: make(map[uint]*pb.ProcessStatus),
	}
	previousProvider := clusterNodeProvider
	clusterNodeProvider = provider
	t.Cleanup(func() { clusterNodeProvider = previousProvider })

	server := &Server{logger: zap.NewNop()}
	server.updateProcessStatusFromHeartbeat(context.Background(), 10, []*pb.ProcessStatus{
		{
			Name:   processidentity.ManagedName("/tmp/seatunnel-2.3.13", "master/worker"),
			Pid:    92322,
			Status: "running",
		},
		{
			Name:   processidentity.ManagedName("/tmp/seatunnel-2.3.12", "master/worker"),
			Pid:    92444,
			Status: "running",
		},
	})

	if got := provider.updates[6]; got == nil || got.Pid != 92322 {
		t.Fatalf("2.3.13 节点更新错误: %+v", got)
	}
	if got := provider.updates[9]; got == nil || got.Pid != 92444 {
		t.Fatalf("2.3.12 节点更新错误: %+v", got)
	}
}
