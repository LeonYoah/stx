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

package router

import (
	"context"
	"fmt"
	"time"

	"github.com/LeonYoah/stx/internal/apps/agent"
	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/apps/selfupgrade"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
)

// selfUpgradeFleetAdapter 让控制面自升级通过现有 gRPC 命令通道与 DownloadPackage 流把新 Agent/Proxy 下发到各节点。
// selfUpgradeFleetAdapter lets the control plane self-upgrade push the new Agent/Proxy to nodes over the existing
// gRPC command channel and DownloadPackage stream.
type selfUpgradeFleetAdapter struct {
	manager     *agent.Manager
	hostService *host.Service
	// installDirs 返回该主机上 SeaTunnel 集群节点的安装目录（用于定位运行中的 stx-java-proxy）。
	// installDirs returns the SeaTunnel install dirs of cluster nodes on the host (to locate running stx-java-proxy).
	installDirs func(ctx context.Context, hostID uint) []string
}

func (a *selfUpgradeFleetAdapter) liveConn(hostID uint, agentID string) (agent.AgentConnectionSnapshot, bool) {
	var best agent.AgentConnectionSnapshot
	found := false
	for _, conn := range a.manager.ListAgents() {
		snap := conn.Snapshot()
		if snap.Status != agent.AgentStatusConnected {
			continue
		}
		if snap.HostID == hostID || (agentID != "" && snap.AgentID == agentID) {
			if !found || snap.ConnectedAt.After(best.ConnectedAt) {
				best, found = snap, true
			}
		}
	}
	return best, found
}

func (a *selfUpgradeFleetAdapter) toNode(h *host.Host) selfupgrade.FleetNode {
	n := selfupgrade.FleetNode{
		HostID:    h.ID,
		HostName:  h.Name,
		IPAddress: h.IPAddress,
		AgentID:   h.AgentID,
		Version:   h.AgentVersion,
		OS:        h.OSType,
		Arch:      h.Arch,
	}
	if snap, ok := a.liveConn(h.ID, h.AgentID); ok {
		n.Online = true
		n.AgentID = snap.AgentID
		n.Version = snap.Version
		n.ConnectedAt = snap.ConnectedAt
	}
	return n
}

// ListNodes 返回所有安装过 Agent 的主机。/ ListNodes returns every host that has an Agent installed.
func (a *selfUpgradeFleetAdapter) ListNodes(ctx context.Context) ([]selfupgrade.FleetNode, error) {
	hosts, _, err := a.hostService.List(ctx, &host.HostFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]selfupgrade.FleetNode, 0, len(hosts))
	for _, h := range hosts {
		if h == nil || h.AgentStatus == host.AgentStatusNotInstalled || (h.AgentID == "" && h.AgentVersion == "") {
			continue
		}
		out = append(out, a.toNode(h))
	}
	return out, nil
}

// Node 返回单个主机的最新状态。/ Node returns the latest state of one host.
func (a *selfUpgradeFleetAdapter) Node(ctx context.Context, hostID uint) (selfupgrade.FleetNode, bool) {
	h, err := a.hostService.Get(ctx, hostID)
	if err != nil || h == nil {
		return selfupgrade.FleetNode{}, false
	}
	n := a.toNode(h)
	if a.installDirs != nil {
		n.InstallDirs = a.installDirs(ctx, hostID)
	}
	return n, true
}

// clusterInstallDirsLookup 从集群节点表查出主机上的 SeaTunnel 安装目录（去重、去空）。
// clusterInstallDirsLookup looks up a host's SeaTunnel install dirs from cluster nodes (deduplicated, non-empty).
func clusterInstallDirsLookup(svc *cluster.Service) func(ctx context.Context, hostID uint) []string {
	if svc == nil {
		return nil
	}
	return func(ctx context.Context, hostID uint) []string {
		nodes, err := svc.GetNodesByHostID(ctx, hostID)
		if err != nil {
			return nil
		}
		seen := map[string]bool{}
		var out []string
		for _, n := range nodes {
			if n == nil || n.InstallDir == "" || seen[n.InstallDir] {
				continue
			}
			seen[n.InstallDir] = true
			out = append(out, n.InstallDir)
		}
		return out
	}
}

// TransferFile 复用 TRANSFER_PACKAGE 的 pull 模式传输文件。/ TransferFile reuses TRANSFER_PACKAGE pull mode.
func (a *selfUpgradeFleetAdapter) TransferFile(ctx context.Context, agentID, key, localPath, checksum string) (string, error) {
	res, err := a.manager.TransferPackage(ctx, agentID, key, localPath, checksum, agent.PackagePullOptions{
		StallTimeout: time.Minute,
		MaxDuration:  10 * time.Minute,
	})
	if err != nil {
		return "", err
	}
	return res.LocalPath, nil
}

// SendUpgradeCommand 发送 UPGRADE 子命令。/ SendUpgradeCommand sends an UPGRADE sub-command.
func (a *selfUpgradeFleetAdapter) SendUpgradeCommand(ctx context.Context, agentID string, params map[string]string, timeout time.Duration) (string, error) {
	resp, err := a.manager.SendCommand(ctx, agentID, pb.CommandType_UPGRADE, params, timeout)
	if err != nil {
		return "", err
	}
	if resp.GetStatus() != pb.CommandStatus_SUCCESS {
		msg := resp.GetError()
		if msg == "" {
			msg = resp.GetOutput()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return resp.GetOutput(), nil
}
