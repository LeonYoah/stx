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

package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/LeonYoah/stx/internal/logger"
)

// RecordRemovalCleanup 是「仅移除记录」后清理的结果。/ RecordRemovalCleanup is the result of cleaning up after "remove record only".
type RecordRemovalCleanup struct {
	// MembersPatched 表示剩余节点的 member-list 已修补；MembersSkippedReason 说明未修补的原因。
	// MembersPatched means the remaining nodes' member lists were patched; MembersSkippedReason explains why not.
	MembersPatched       bool     `json:"members_patched"`
	MembersSkippedReason string   `json:"members_skipped_reason,omitempty"`
	DeletedConfigs       int      `json:"deleted_configs"`
	MonitorRefreshed     bool     `json:"monitor_refreshed"`
	Warnings             []string `json:"warnings"`
}

// CleanupAfterRecordRemoval 在「仅移除记录」（不停进程，主机离线或进程被其他集群共用时的降级入口）删除节点记录后补齐清理：
//   - 修补剩余节点与模板的 member-list（只写文件、不重启），避免其他节点继续把它当成员；
//     若被移除节点或任一剩余节点的进程同时登记在其他集群，节点上的 hazelcast 文件为多个集群共用，修补会改到其他集群，跳过；
//   - 该主机在本集群已无节点时，删除其在本集群的配置记录，避免 sync-all 永远报待推送；
//   - 主机在线时按集群下发监控配置以移除本集群的跟踪项（共享进程时其他集群的跟踪项保留）。
//
// 清理尽力而为：节点记录已删除，失败只记入 Warnings 与日志。
// CleanupAfterRecordRemoval completes the cleanup after "remove record only" (no process stop; the fallback when the
// host is offline or the process is shared with another cluster) deleted a node record:
//   - patch the member list of the template and remaining nodes (files only, no restart) so they stop treating it as a
//     member; skipped when the removed node or any remaining node shares its process with another cluster, because the
//     hazelcast file on that host is then shared and patching it would change the other cluster;
//   - when the host has no node of this cluster left, delete its config records of this cluster so sync-all no longer
//     reports them as pending forever;
//   - when the host is online, push the cluster-scoped monitor config to drop this cluster's tracked entry (another
//     cluster's entry for a shared process is kept).
//
// Best effort: the node record is already gone, so failures only go to Warnings and the log.
func (s *ScaleService) CleanupAfterRecordRemoval(ctx context.Context, clusterID uint, removed *ClusterNode) *RecordRemovalCleanup {
	result := &RecordRemovalCleanup{Warnings: []string{}}
	if s == nil || s.svc == nil || removed == nil {
		return result
	}
	view, err := s.loadScaleView(ctx, clusterID, false)
	if err != nil {
		result.Warnings = append(result.Warnings, "load cluster: "+err.Error())
		return result
	}

	if s.config != nil {
		shared := s.sharedProcessClusters(ctx, removed)
		for _, node := range view.nodes {
			shared = append(shared, s.sharedProcessClusters(ctx, node)...)
		}
		switch {
		case len(shared) > 0:
			result.MembersSkippedReason = fmt.Sprintf("shared process with cluster(s) %s", strings.Join(uniqueStrings(shared), ", "))
		case len(view.nodes) == 0:
			result.MembersSkippedReason = "no remaining node"
		default:
			patch, patchErr := s.config.PatchMemberList(ctx, clusterID, string(view.cluster.DeploymentMode), clusterMemberList(view, 0),
				fmt.Sprintf("remove-record:node%d", removed.ID), 0)
			if patchErr != nil {
				result.Warnings = append(result.Warnings, "patch member list: "+patchErr.Error())
			} else {
				result.MembersPatched = true
				if patch != nil && patch.Partial() {
					result.Warnings = append(result.Warnings, "member list written to some nodes later (pending push)")
				}
			}
		}

		hostStillUsed := false
		for _, node := range view.nodes {
			if node.HostID == removed.HostID {
				hostStillUsed = true
				break
			}
		}
		if !hostStillUsed {
			deleted, deleteErr := s.config.DeleteHostConfigs(ctx, clusterID, removed.HostID)
			if deleteErr != nil {
				result.Warnings = append(result.Warnings, "delete host configs: "+deleteErr.Error())
			}
			result.DeletedConfigs = deleted
		}
	}

	if s.monitor != nil {
		if host, hostErr := s.svc.hostProvider.GetHostByID(ctx, removed.HostID); hostErr == nil && s.hostReady(host) {
			if pushErr := s.monitor.PushClusterMonitorConfigToHost(ctx, clusterID, removed.HostID); pushErr != nil {
				result.Warnings = append(result.Warnings, "refresh monitor: "+pushErr.Error())
			} else {
				result.MonitorRefreshed = true
			}
		}
	}

	s.svc.updateClusterStatusFromNodes(ctx, clusterID)
	logger.InfoF(ctx, "[ClusterScale] 仅移除记录后清理 / Cleanup after record-only removal: cluster=%d, node=%d, host=%d, members_patched=%t, skipped=%q, deleted_configs=%d, monitor=%t, warnings=%v",
		clusterID, removed.ID, removed.HostID, result.MembersPatched, result.MembersSkippedReason, result.DeletedConfigs, result.MonitorRefreshed, result.Warnings)
	return result
}

// uniqueStrings 去重并保持首次出现顺序。/ uniqueStrings de-duplicates while keeping first-seen order.
func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
