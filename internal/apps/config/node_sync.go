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

package config

import (
	"context"
	"fmt"
	"time"

	"github.com/LeonYoah/stx/internal/db"
)

// nodeApplyOutcome 描述一次节点配置变更的结果。
// nodeApplyOutcome describes the result of applying a change to one node config.
type nodeApplyOutcome struct {
	// Applied 表示已推送到节点并提交为新版本（或无需推送直接提交）。
	// Applied means the content was pushed and committed as a new version (or committed without push).
	Applied bool
	// Pending 表示推送失败，新内容已暂存为待推送，版本未提升。
	// Pending means the push failed and the new content was parked as pending without a version bump.
	Pending bool
	// PushError 推送失败原因。
	// PushError is the push failure reason.
	PushError string
}

// canPushNodeConfig 判断节点配置是否具备推送条件（节点配置且已配置 Agent 客户端）。
// canPushNodeConfig reports whether a node config can be pushed (node config with an Agent client configured).
func (s *Service) canPushNodeConfig(config *Config) bool {
	return config != nil && config.HostID != nil && s.nodeInfoProvider != nil && s.agentClient != nil
}

// applyNodeConfigContent 先推送到节点、成功后再提交新版本；推送失败时不提升版本，只记录待推送。
// 不具备推送条件（模板、未配置 Agent、未知安装目录）时直接提交，保持原有行为。
// applyNodeConfigContent pushes to the node first and commits a new version only after a successful push;
// on push failure the version is not bumped and the content is recorded as pending. When pushing is not
// possible (template, no Agent client, unknown install dir) it commits directly, keeping the old behavior.
func (s *Service) applyNodeConfigContent(ctx context.Context, config *Config, content, comment string, userID uint) (*nodeApplyOutcome, error) {
	if !s.canPushNodeConfig(config) {
		if err := s.commitConfigVersion(ctx, config, content, comment, userID); err != nil {
			return nil, err
		}
		return &nodeApplyOutcome{Applied: true}, nil
	}

	installDir, dirErr := s.nodeInfoProvider.GetNodeInstallDir(ctx, config.ClusterID, *config.HostID)
	if dirErr != nil {
		pushErr := "获取节点安装目录失败 / failed to resolve node install dir: " + dirErr.Error()
		if err := s.markPendingPush(ctx, config, content, comment, userID, pushErr); err != nil {
			return nil, err
		}
		return &nodeApplyOutcome{Pending: true, PushError: pushErr}, nil
	}
	if installDir == "" {
		if err := s.commitConfigVersion(ctx, config, content, comment, userID); err != nil {
			return nil, err
		}
		return &nodeApplyOutcome{Applied: true}, nil
	}

	if err := s.agentClient.PushConfig(ctx, *config.HostID, installDir, config.ConfigType, content); err != nil {
		pushErr := "推送配置到节点失败 / failed to push config to node: " + err.Error()
		if markErr := s.markPendingPush(ctx, config, content, comment, userID, pushErr); markErr != nil {
			return nil, markErr
		}
		return &nodeApplyOutcome{Pending: true, PushError: pushErr}, nil
	}
	if err := s.commitConfigVersion(ctx, config, content, comment, userID); err != nil {
		return nil, err
	}
	s.syncDerivedRuntimeMetadata(ctx, config.ClusterID, config.HostID, config.ConfigType, content)
	return &nodeApplyOutcome{Applied: true}, nil
}

// commitConfigVersion 提交新内容为新版本，并清除待推送标记。
// commitConfigVersion commits new content as a new version and clears the pending marker.
func (s *Service) commitConfigVersion(ctx context.Context, config *Config, content, comment string, userID uint) error {
	config.Content = db.ScriptText(content)
	config.Version = config.Version + 1
	config.UpdatedBy = userID
	config.UpdatedAt = time.Now()
	clearPendingPush(config)
	return s.repo.Transaction(ctx, func(tx *Repository) error {
		if err := tx.Update(ctx, config); err != nil {
			return err
		}
		return tx.CreateVersion(ctx, &ConfigVersion{
			ConfigID:  config.ID,
			Version:   config.Version,
			Content:   db.ScriptText(content),
			Comment:   comment,
			CreatedBy: userID,
		})
	})
}

// markPendingPush 记录待推送内容与失败原因，不修改当前内容与版本号。
// markPendingPush records the pending content and failure reason without touching current content/version.
func (s *Service) markPendingPush(ctx context.Context, config *Config, content, comment string, userID uint, reason string) error {
	now := time.Now()
	config.SyncStatus = ConfigSyncStatusPendingPush
	config.SyncError = truncateSyncError(reason)
	config.PendingContent = db.ScriptText(content)
	config.PendingComment = comment
	config.PendingBy = userID
	if config.PendingSince == nil {
		config.PendingSince = &now
	}
	return s.repo.Update(ctx, config)
}

func clearPendingPush(config *Config) {
	config.SyncStatus = ConfigSyncStatusSynced
	config.SyncError = ""
	config.PendingContent = ""
	config.PendingComment = ""
	config.PendingBy = 0
	config.PendingSince = nil
}

func truncateSyncError(reason string) string {
	const maxLen = 1000
	runes := []rune(reason)
	if len(runes) <= maxLen {
		return reason
	}
	return string(runes[:maxLen]) + "..."
}

// RetryPendingPushes 重推某主机上所有待推送的节点配置（Agent 重连后调用）。返回成功提交的数量。
// RetryPendingPushes re-pushes every pending node config of a host (called after the Agent reconnects)
// and returns how many were committed.
func (s *Service) RetryPendingPushes(ctx context.Context, hostID uint) (int, error) {
	if hostID == 0 {
		return 0, nil
	}
	pending, err := s.repo.ListPendingPushByHost(ctx, hostID)
	if err != nil {
		return 0, err
	}
	committed := 0
	var firstErr error
	for _, config := range pending {
		outcome, err := s.retryPendingPush(ctx, config)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if outcome != nil && outcome.Applied {
			committed++
		}
	}
	return committed, firstErr
}

func (s *Service) retryPendingPush(ctx context.Context, config *Config) (*nodeApplyOutcome, error) {
	if config == nil || !config.HasPendingPush() {
		return &nodeApplyOutcome{}, nil
	}
	content := config.PendingContent.String()
	comment := config.PendingComment
	if comment == "" {
		comment = "Re-pushed pending change / 重推待推送变更"
	}
	userID := config.PendingBy
	if config.Content.String() == content {
		clearPendingPush(config)
		if err := s.repo.Update(ctx, config); err != nil {
			return nil, err
		}
		return &nodeApplyOutcome{Applied: true}, nil
	}
	return s.applyNodeConfigContent(ctx, config, content, comment, userID)
}

// syncNodesToContent 将一组节点配置同步到目标内容，逐节点“先推送后提交”，并汇总逐节点结果。
// syncNodesToContent syncs node configs to the target content with push-then-commit per node and
// aggregates per-node results.
func (s *Service) syncNodesToContent(ctx context.Context, nodeConfigs []*Config, content, comment string, userID uint) (*SyncAllResult, error) {
	result := &SyncAllResult{
		NodeResults: make([]*NodeSyncResult, 0, len(nodeConfigs)),
		PushErrors:  make([]*PushError, 0),
	}
	for _, nc := range nodeConfigs {
		if nc == nil || nc.HostID == nil {
			continue
		}
		nodeResult := &NodeSyncResult{ConfigID: nc.ID, HostID: *nc.HostID}
		if s.hostProvider != nil {
			if host, err := s.hostProvider.GetHostByID(ctx, *nc.HostID); err == nil && host != nil {
				nodeResult.HostName = host.Name
				nodeResult.HostIP = host.IPAddress
			}
		}
		if nc.Content.String() == content && !nc.HasPendingPush() {
			nodeResult.Status = ConfigSyncStatusUnchanged
			nodeResult.Version = nc.Version
			result.UnchangedCount++
			result.NodeResults = append(result.NodeResults, nodeResult)
			continue
		}
		var (
			outcome *nodeApplyOutcome
			err     error
		)
		if nc.Content.String() == content {
			// 内容已一致但存在旧的待推送内容：目标已变，清除待推送即可。
			// Content already matches but a stale pending change exists: the target moved, just clear it.
			clearPendingPush(nc)
			err = s.repo.Update(ctx, nc)
			outcome = &nodeApplyOutcome{Applied: true}
		} else {
			outcome, err = s.applyNodeConfigContent(ctx, nc, content, comment, userID)
		}
		if err != nil {
			return nil, err
		}
		nodeResult.Version = nc.Version
		switch {
		case outcome.Pending:
			nodeResult.Status = ConfigSyncStatusPendingPush
			nodeResult.Message = outcome.PushError
			result.PendingCount++
			result.PushErrors = append(result.PushErrors, &PushError{HostID: *nc.HostID, HostIP: nodeResult.HostIP, Message: outcome.PushError})
		default:
			nodeResult.Status = ConfigSyncStatusSynced
			result.SyncedCount++
		}
		result.NodeResults = append(result.NodeResults, nodeResult)
	}
	return result, nil
}

// SyncResultMessage 生成与逐节点结果一致的汇总说明。
// SyncResultMessage builds a summary message consistent with the per-node results.
func SyncResultMessage(result *SyncAllResult) string {
	if result == nil {
		return ""
	}
	if result.PendingCount == 0 {
		return fmt.Sprintf("template synced: %d updated, %d unchanged / 模板已同步：%d 个节点已更新，%d 个节点无变化", result.SyncedCount, result.UnchangedCount, result.SyncedCount, result.UnchangedCount)
	}
	return fmt.Sprintf("template partially synced: %d updated, %d unchanged, %d pending push (will retry when the agent reconnects) / 模板部分同步：%d 个节点已更新，%d 个节点无变化，%d 个节点推送失败已标记待推送（Agent 重连后自动重推）", result.SyncedCount, result.UnchangedCount, result.PendingCount, result.SyncedCount, result.UnchangedCount, result.PendingCount)
}
