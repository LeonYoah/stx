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
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// flakyAgentClient 对 offline 中的主机推送失败，记录每次推送的内容。
// flakyAgentClient fails pushes to offline hosts and records every pushed content.
type flakyAgentClient struct {
	offline map[uint]bool
	pushed  map[uint][]string
}

func (c *flakyAgentClient) PullConfig(_ context.Context, _ uint, _ string, _ ConfigType) (string, error) {
	return "", nil
}

func (c *flakyAgentClient) PushConfig(_ context.Context, hostID uint, _ string, _ ConfigType, content string) error {
	if c.offline[hostID] {
		return errors.New("agent not connected")
	}
	if c.pushed == nil {
		c.pushed = map[uint][]string{}
	}
	c.pushed[hostID] = append(c.pushed[hostID], content)
	return nil
}

type stubHostProvider struct{}

func (stubHostProvider) GetHostByID(_ context.Context, id uint) (*HostInfo, error) {
	return &HostInfo{ID: id, Name: "host", IPAddress: "10.0.0.1"}, nil
}

const (
	syncTestOld = "seatunnel:\n  engine:\n    backup-count: 1\n"
	syncTestNew = "seatunnel:\n  engine:\n    backup-count: 2\n"
)

func newNodeSyncTestService(t *testing.T) (*Service, *gorm.DB, *flakyAgentClient) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Config{}, &ConfigVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	agent := &flakyAgentClient{offline: map[uint]bool{}}
	svc := NewService(NewRepository(db), stubHostProvider{}, &testNodeInfoProvider{installDir: "/opt/seatunnel"}, agent)
	return svc, db, agent
}

func seedNodeSyncConfigs(t *testing.T, db *gorm.DB, hosts ...uint) map[uint]*Config {
	t.Helper()
	template := &Config{ClusterID: 1, ConfigType: ConfigTypeSeatunnel, FilePath: GetConfigFilePath(ConfigTypeSeatunnel), Content: syncTestNew, Version: 2}
	if err := db.Create(template).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}
	nodes := map[uint]*Config{}
	for _, h := range hosts {
		hostID := h
		node := &Config{ClusterID: 1, HostID: &hostID, ConfigType: ConfigTypeSeatunnel, FilePath: GetConfigFilePath(ConfigTypeSeatunnel), Content: syncTestOld, Version: 1}
		if err := db.Create(node).Error; err != nil {
			t.Fatalf("create node config: %v", err)
		}
		nodes[h] = node
	}
	return nodes
}

func reloadConfig(t *testing.T, db *gorm.DB, id uint) *Config {
	t.Helper()
	var c Config
	if err := db.First(&c, id).Error; err != nil {
		t.Fatalf("reload config %d: %v", id, err)
	}
	return &c
}

func TestSyncTemplateToAllNodes_OfflineNodeMarkedPendingWithoutVersionBump(t *testing.T) {
	svc, db, agent := newNodeSyncTestService(t)
	nodes := seedNodeSyncConfigs(t, db, 11, 12, 13)
	agent.offline[12] = true

	result, err := svc.SyncTemplateToAllNodes(context.Background(), 1, ConfigTypeSeatunnel, 7)
	if err != nil {
		t.Fatalf("SyncTemplateToAllNodes: %v", err)
	}
	if result.SyncedCount != 2 || result.PendingCount != 1 || len(result.PushErrors) != 1 || len(result.NodeResults) != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, nr := range result.NodeResults {
		want := ConfigSyncStatusSynced
		if nr.HostID == 12 {
			want = ConfigSyncStatusPendingPush
		}
		if nr.Status != want {
			t.Fatalf("host %d status = %s, want %s", nr.HostID, nr.Status, want)
		}
	}
	if !strings.Contains(SyncResultMessage(result), "partially") {
		t.Fatalf("message should report partial sync: %s", SyncResultMessage(result))
	}

	offline := reloadConfig(t, db, nodes[12].ID)
	if offline.Version != 1 || offline.Content.String() != syncTestOld {
		t.Fatalf("offline node must keep its real version/content, got v%d %q", offline.Version, offline.Content)
	}
	if !offline.HasPendingPush() || offline.PendingContent.String() != syncTestNew || offline.SyncError == "" || offline.PendingSince == nil {
		t.Fatalf("offline node must be marked pending: %+v", offline)
	}
	online := reloadConfig(t, db, nodes[11].ID)
	if online.Version != 2 || online.Content.String() != syncTestNew || online.HasPendingPush() {
		t.Fatalf("online node should be committed: %+v", online)
	}
	var versions int64
	db.Model(&ConfigVersion{}).Where("config_id = ?", nodes[12].ID).Count(&versions)
	if versions != 0 {
		t.Fatalf("offline node must not get a version row, got %d", versions)
	}

	info, err := svc.Get(context.Background(), nodes[12].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.SyncStatus != ConfigSyncStatusPendingPush || info.PendingContent != syncTestNew {
		t.Fatalf("ConfigInfo should expose pending state: %+v", info)
	}

	// 再次同步：已同步节点为 unchanged，不重复推送。
	// Re-sync: synced nodes are unchanged and not pushed again.
	agent.pushed = nil
	result, err = svc.SyncTemplateToAllNodes(context.Background(), 1, ConfigTypeSeatunnel, 7)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if result.UnchangedCount != 2 || result.PendingCount != 1 || len(agent.pushed) != 0 {
		t.Fatalf("unexpected second result: %+v pushed=%v", result, agent.pushed)
	}
}

func TestRetryPendingPushes_CommitsAfterReconnect(t *testing.T) {
	svc, db, agent := newNodeSyncTestService(t)
	nodes := seedNodeSyncConfigs(t, db, 21)
	agent.offline[21] = true
	if _, err := svc.SyncTemplateToAllNodes(context.Background(), 1, ConfigTypeSeatunnel, 7); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// 仍离线：保持待推送。Still offline: stays pending.
	committed, err := svc.RetryPendingPushes(context.Background(), 21)
	if err != nil || committed != 0 {
		t.Fatalf("retry while offline: committed=%d err=%v", committed, err)
	}
	if c := reloadConfig(t, db, nodes[21].ID); !c.HasPendingPush() || c.Version != 1 {
		t.Fatalf("still offline must stay pending: %+v", c)
	}

	agent.offline[21] = false
	committed, err = svc.RetryPendingPushes(context.Background(), 21)
	if err != nil || committed != 1 {
		t.Fatalf("retry after reconnect: committed=%d err=%v", committed, err)
	}
	c := reloadConfig(t, db, nodes[21].ID)
	if c.HasPendingPush() || c.Version != 2 || c.Content.String() != syncTestNew || c.SyncError != "" || c.PendingSince != nil {
		t.Fatalf("expected committed config after re-push: %+v", c)
	}
	if got := agent.pushed[21]; len(got) != 1 || got[0] != syncTestNew {
		t.Fatalf("unexpected pushes: %v", got)
	}
	var version ConfigVersion
	if err := db.Where("config_id = ? AND version = ?", nodes[21].ID, 2).First(&version).Error; err != nil {
		t.Fatalf("expected version 2 row: %v", err)
	}
}

func TestUpdate_PushFailureKeepsVersionAndRevertClearsPending(t *testing.T) {
	svc, db, agent := newNodeSyncTestService(t)
	nodes := seedNodeSyncConfigs(t, db, 31)
	agent.offline[31] = true

	info, err := svc.Update(context.Background(), nodes[31].ID, &UpdateConfigRequest{Content: syncTestNew, Comment: "bump"}, 7)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if info.PushError == "" || info.Version != 1 || info.SyncStatus != ConfigSyncStatusPendingPush {
		t.Fatalf("update with offline node should be pending at v1: %+v", info)
	}

	// 用户改回原内容：放弃待推送。User reverts to the current content: pending is discarded.
	info, err = svc.Update(context.Background(), nodes[31].ID, &UpdateConfigRequest{Content: syncTestOld}, 7)
	if err != nil {
		t.Fatalf("revert Update: %v", err)
	}
	if info.SyncStatus != ConfigSyncStatusSynced || reloadConfig(t, db, nodes[31].ID).HasPendingPush() {
		t.Fatalf("revert should clear pending: %+v", info)
	}
}

func TestPromote_PushesToNodesAndMarksOfflinePending(t *testing.T) {
	svc, db, agent := newNodeSyncTestService(t)
	nodes := seedNodeSyncConfigs(t, db, 41, 42, 43)
	source := reloadConfig(t, db, nodes[41].ID)
	source.Content = "seatunnel:\n  engine:\n    backup-count: 3\n"
	if err := db.Save(source).Error; err != nil {
		t.Fatalf("save source: %v", err)
	}
	agent.offline[43] = true

	result, err := svc.Promote(context.Background(), source.ID, &PromoteConfigRequest{Comment: "promote"}, 7)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.UnchangedCount != 1 || result.SyncedCount != 1 || result.PendingCount != 1 {
		t.Fatalf("unexpected promote result: %+v", result)
	}
	if len(agent.pushed[42]) != 1 {
		t.Fatalf("promote must push to node 42, pushes=%v", agent.pushed)
	}
	if c := reloadConfig(t, db, nodes[43].ID); !c.HasPendingPush() || c.Version != 1 {
		t.Fatalf("offline node must be pending: %+v", c)
	}
}
