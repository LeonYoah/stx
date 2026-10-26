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

package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/logger"
)

// existingRecordForInstall 返回允许被重新安装覆盖的已有记录（仅 partial），已完整安装时返回 ErrPluginAlreadyExists。
// existingRecordForInstall returns an existing record that a new install may overwrite (partial only);
// a fully installed plugin yields ErrPluginAlreadyExists.
func (s *Service) existingRecordForInstall(ctx context.Context, clusterID uint, pluginName string) (*InstalledPlugin, error) {
	existing, err := s.repo.GetByClusterAndName(ctx, clusterID, pluginName)
	if err != nil {
		if errors.Is(err, ErrPluginNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if existing.Status == PluginStatusPartial {
		return existing, nil
	}
	return nil, ErrPluginAlreadyExists
}

// installPluginOnNodes 依次安装到各节点：先解析全部节点的 Agent（任何节点缺失则不触碰任何节点），
// 某节点失败后停止并回滚已安装节点。返回逐节点结果与首个失败原因。
// installPluginOnNodes installs to every node in order: all node Agents are resolved first (if any is
// missing no node is touched); after a node fails the remaining nodes are skipped and the nodes already
// installed are rolled back. It returns per-node results and the first failure.
func (s *Service) installPluginOnNodes(ctx context.Context, clusterID uint, req *InstallPluginRequest, artifactID string, deps []PluginDependency, nodes []ClusterNodeInfo, progress *PluginInstallStatus) ([]PluginNodeInstallResult, error) {
	results := make([]PluginNodeInstallResult, len(nodes))
	agentIDs := make([]string, len(nodes))
	for i, node := range nodes {
		results[i] = PluginNodeInstallResult{NodeID: node.NodeID, HostID: node.HostID, Status: PluginNodeStatusSkipped}
	}
	for i, node := range nodes {
		agentID, err := s.hostInfoGetter.GetHostAgentID(ctx, node.HostID)
		if err == nil && strings.TrimSpace(agentID) == "" {
			err = fmt.Errorf("agent not installed on host %d / 主机 %d 未安装 Agent", node.HostID, node.HostID)
		}
		if err != nil {
			results[i].Status = PluginNodeStatusFailed
			results[i].Message = err.Error()
			return results, fmt.Errorf("node %d (host %d): failed to resolve agent / 解析 Agent 失败: %w", node.NodeID, node.HostID, err)
		}
		agentIDs[i] = agentID
	}

	totalNodes := len(nodes)
	for i, node := range nodes {
		progress.Progress = 50 + (i * 50 / totalNodes)
		progress.Message = fmt.Sprintf("Installing to node %d/%d / 正在安装到节点 %d/%d", i+1, totalNodes, i+1, totalNodes)
		progress.NodeResults = append([]PluginNodeInstallResult(nil), results...)
		s.setInstallProgress(clusterID, req.PluginName, progress)

		if err := s.transferPluginToAgent(ctx, agentIDs[i], artifactID, req.PluginName, req.Version, node.InstallDir, deps, req.ProfileKeys); err != nil {
			results[i].Status = PluginNodeStatusFailed
			results[i].Message = err.Error()
			installErr := fmt.Errorf("node %d (host %d): %w", node.NodeID, node.HostID, err)

			progress.Message = "Install failed, rolling back installed nodes / 安装失败，正在回滚已安装节点"
			progress.NodeResults = append([]PluginNodeInstallResult(nil), results...)
			s.setInstallProgress(clusterID, req.PluginName, progress)

			for j := 0; j < i; j++ {
				if results[j].Status != PluginNodeStatusInstalled {
					continue
				}
				if rbErr := s.removePluginFromNode(ctx, agentIDs[j], artifactID, req.Version, nodes[j].InstallDir); rbErr != nil {
					results[j].Status = PluginNodeStatusRollbackFailed
					results[j].Message = rbErr.Error()
				} else {
					results[j].Status = PluginNodeStatusRolledBack
					results[j].Message = ""
				}
			}
			// 失败节点可能残留部分文件，尽力清理（不可达时忽略）。
			// The failed node may hold a partial file; clean it up best-effort (ignored when unreachable).
			if cleanupErr := s.removePluginFromNode(ctx, agentIDs[i], artifactID, req.Version, node.InstallDir); cleanupErr != nil {
				logger.WarnF(ctx, "[Plugin] cleanup on failed node %d skipped: %v", node.NodeID, cleanupErr)
			}
			return results, installErr
		}
		results[i].Status = PluginNodeStatusInstalled
	}
	return results, nil
}

// removePluginFromNode 删除节点上的连接器 jar（Agent 按 connector-<name>-<version>.jar 删除）。
// removePluginFromNode removes the connector jar from a node (the Agent deletes connector-<name>-<version>.jar).
func (s *Service) removePluginFromNode(ctx context.Context, agentID, artifactID, version, installDir string) error {
	params := map[string]string{
		"plugin_name":  strings.TrimPrefix(artifactID, "connector-"),
		"version":      version,
		"install_path": installDir,
	}
	success, message, err := s.agentCommandSender.SendCommand(ctx, agentID, "uninstall_plugin", params)
	if err != nil {
		return fmt.Errorf("rollback failed / 回滚失败: %w", err)
	}
	if !success {
		return fmt.Errorf("rollback failed / 回滚失败: %s", message)
	}
	return nil
}

// finishFailedInstall 根据回滚结果收尾：全部回滚成功则不留记录；仍有节点保留插件则写入 partial 记录，
// 让插件在列表中可见并可重试/卸载。返回去重后的错误。
// finishFailedInstall finalizes a failed install from the rollback outcome: when everything was rolled
// back no record is kept; when some nodes still have the plugin a partial record is stored so the plugin
// stays visible and can be retried or uninstalled. It returns a deduplicated error.
func (s *Service) finishFailedInstall(ctx context.Context, clusterID uint, req *InstallPluginRequest, pluginInfo *Plugin, artifactID string, existing *InstalledPlugin, results []PluginNodeInstallResult, installErr error, progress *PluginInstallStatus) error {
	// 合并此前部分安装中本次未触达节点的状态。
	// Merge states of nodes from a previous partial install that this attempt did not reach.
	if existing != nil && len(existing.NodeStatuses) > 0 {
		prev := make(map[uint]PluginNodeInstallResult, len(existing.NodeStatuses))
		for _, item := range existing.NodeStatuses {
			prev[item.NodeID] = item
		}
		for i := range results {
			if results[i].Status != PluginNodeStatusSkipped {
				continue
			}
			if old, ok := prev[results[i].NodeID]; ok && pluginPresentOnNode(old.Status) {
				results[i].Status = old.Status
				results[i].Message = old.Message
			}
		}
	}

	present := 0
	for _, item := range results {
		if pluginPresentOnNode(item.Status) {
			present++
		}
	}

	progress.Status = "failed"
	progress.NodeResults = append([]PluginNodeInstallResult(nil), results...)
	progress.Error = installErr.Error()

	if present == 0 {
		progress.Partial = false
		progress.Message = "Install failed; no node keeps the plugin / 安装失败，已回滚，所有节点均未保留插件"
		s.setInstallProgress(clusterID, req.PluginName, progress)
		if existing != nil {
			if err := s.repo.Delete(ctx, existing.ID); err != nil {
				logger.WarnF(ctx, "[Plugin] failed to delete stale partial record %d: %v", existing.ID, err)
			}
		}
		return fmt.Errorf("plugin install failed and was rolled back / 插件安装失败，已回滚: %w", installErr)
	}

	now := time.Now()
	record := &InstalledPlugin{
		ClusterID:    clusterID,
		PluginName:   req.PluginName,
		ArtifactID:   artifactID,
		Category:     pluginInfo.Category,
		Version:      req.Version,
		Status:       PluginStatusPartial,
		InstallPath:  fmt.Sprintf("connectors/%s-%s.jar", artifactID, req.Version),
		InstalledAt:  now,
		UpdatedAt:    now,
		NodeStatuses: append([]PluginNodeInstallResult(nil), results...),
	}
	var saveErr error
	if existing != nil {
		record.ID = existing.ID
		record.InstalledAt = existing.InstalledAt
		record.InstalledBy = existing.InstalledBy
		saveErr = s.repo.Update(ctx, record)
	} else {
		saveErr = s.repo.Create(ctx, record)
	}
	if saveErr != nil {
		logger.ErrorF(ctx, "[Plugin] failed to record partial install for %s: %v", req.PluginName, saveErr)
	}

	progress.Partial = true
	progress.Message = fmt.Sprintf("Install partially failed: %d/%d nodes still have the plugin; retry install or uninstall / 部分安装失败：%d/%d 个节点仍保留插件，请重试安装或卸载", present, len(results), present, len(results))
	s.setInstallProgress(clusterID, req.PluginName, progress)
	return fmt.Errorf("plugin install partially failed / 插件部分安装失败: %w", installErr)
}
