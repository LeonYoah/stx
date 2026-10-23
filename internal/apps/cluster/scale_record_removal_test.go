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
	"reflect"
	"strings"
	"testing"
)

// wireRecordRemovalCleanup 与 router 一致地把清理挂到 Service.RemoveNode，并返回最近一次清理结果。
// wireRecordRemovalCleanup hooks the cleanup into Service.RemoveNode the same way the router does and returns the
// latest cleanup result.
func wireRecordRemovalCleanup(env *scaleTestEnv) **RecordRemovalCleanup {
	var last *RecordRemovalCleanup
	env.svc.SetOnNodeRecordRemoved(func(ctx context.Context, clusterID uint, node *ClusterNode) {
		last = env.scale.CleanupAfterRecordRemoval(ctx, clusterID, node)
	})
	return &last
}

// TestRemoveNodeRecordOnlyCleansUp 「仅移除记录」不停进程，但修补剩余成员列表、删除该主机配置记录并刷新监控。
// TestRemoveNodeRecordOnlyCleansUp: "remove record only" stops no process but patches the remaining member list,
// deletes the host's config records and refreshes monitoring.
func TestRemoveNodeRecordOnlyCleansUp(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	last := wireRecordRemovalCleanup(env)
	target, err := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
	if err != nil {
		t.Fatalf("load target: %v", err)
	}

	if err := env.svc.RemoveNode(ctx, env.cluster.ID, target.ID); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}
	result := *last
	if result == nil || !result.MembersPatched || result.MembersSkippedReason != "" {
		t.Fatalf("remaining member list must be patched: %+v", result)
	}
	if want := [][]string{{"10.0.0.1:5801"}}; !reflect.DeepEqual(env.config.patchMembers, want) {
		t.Fatalf("patched members = %v, want %v", env.config.patchMembers, want)
	}
	if !reflect.DeepEqual(env.config.deleted, []uint{2}) || result.DeletedConfigs != 3 {
		t.Fatalf("host 2 configs must be deleted: deleted=%v count=%d", env.config.deleted, result.DeletedConfigs)
	}
	if want := []string{fmt.Sprintf("%d/2", env.cluster.ID)}; !reflect.DeepEqual(env.monitor.pushes, want) || !result.MonitorRefreshed {
		t.Fatalf("monitor pushes = %v, want %v", env.monitor.pushes, want)
	}
	for _, cmd := range env.agent.commands {
		if cmd.commandType == "stop" || cmd.commandType == "restart" {
			t.Fatalf("record-only removal must not touch processes: %+v", env.agent.commands)
		}
	}
}

// TestRemoveNodeRecordOnlySkipsMembersForSharedProcess 被移除节点的进程同时登记在其他集群时，节点上的 hazelcast 文件
// 为两个集群共用，不修补成员列表；仍删除本集群对该主机的配置记录。
// TestRemoveNodeRecordOnlySkipsMembersForSharedProcess: when the removed node's process is also registered in another
// cluster the hazelcast file on it is shared, so member lists are not patched; this cluster's host configs are still
// deleted.
func TestRemoveNodeRecordOnlySkipsMembersForSharedProcess(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	last := wireRecordRemovalCleanup(env)
	target, err := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
	if err != nil {
		t.Fatalf("load target: %v", err)
	}
	other := &Cluster{Name: "XCEC-" + t.Name(), DeploymentMode: DeploymentModeHybrid, Version: env.cluster.Version, InstallDir: scaleTestInstallDir, Status: ClusterStatusRunning}
	if err := env.svc.repo.Create(ctx, other); err != nil {
		t.Fatalf("create other cluster: %v", err)
	}
	if err := env.svc.repo.AddNode(ctx, &ClusterNode{ClusterID: other.ID, HostID: 2, Role: NodeRoleMasterWorker, InstallDir: scaleTestInstallDir, HazelcastPort: 5801, APIPort: 8080, Status: NodeStatusRunning}); err != nil {
		t.Fatalf("add shared node: %v", err)
	}

	if err := env.svc.RemoveNode(ctx, env.cluster.ID, target.ID); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}
	result := *last
	if result == nil || result.MembersPatched || !strings.Contains(result.MembersSkippedReason, other.Name) {
		t.Fatalf("shared process must skip the member patch and name the other cluster: %+v", result)
	}
	if len(env.config.patchMembers) != 0 {
		t.Fatalf("member list must not be patched: %v", env.config.patchMembers)
	}
	if !reflect.DeepEqual(env.config.deleted, []uint{2}) {
		t.Fatalf("this cluster's host 2 configs must still be deleted: %v", env.config.deleted)
	}
}

// TestRemoveNodeRecordOnlyKeepsConfigsWhenHostStillUsed 分离模式下同主机仍有另一角色节点时保留主机配置。
// TestRemoveNodeRecordOnlyKeepsConfigsWhenHostStillUsed keeps the host configs when another role of the same host is
// still in the cluster (separated mode).
func TestRemoveNodeRecordOnlyKeepsConfigsWhenHostStillUsed(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeSeparated, map[uint]NodeRole{1: NodeRoleMaster, 2: NodeRoleWorker})
	last := wireRecordRemovalCleanup(env)
	extra := &ClusterNode{ClusterID: env.cluster.ID, HostID: 1, Role: NodeRoleWorker, InstallDir: scaleTestInstallDir, HazelcastPort: 5802, APIPort: 8081, Status: NodeStatusRunning}
	if err := env.svc.repo.AddNode(ctx, extra); err != nil {
		t.Fatalf("add extra worker: %v", err)
	}

	if err := env.svc.RemoveNode(ctx, env.cluster.ID, extra.ID); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}
	result := *last
	if result == nil || !result.MembersPatched {
		t.Fatalf("remaining member list must be patched: %+v", result)
	}
	if len(env.config.deleted) != 0 || result.DeletedConfigs != 0 {
		t.Fatalf("host 1 still runs the master, its configs must stay: %v", env.config.deleted)
	}
}
