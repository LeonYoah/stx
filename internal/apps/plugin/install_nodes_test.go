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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type stubPluginNodes struct{ nodes []ClusterNodeInfo }

func (s stubPluginNodes) GetClusterNodes(_ context.Context, _ uint) ([]ClusterNodeInfo, error) {
	return s.nodes, nil
}

type stubPluginHosts struct{ agents map[uint]string }

func (s stubPluginHosts) GetHostAgentID(_ context.Context, hostID uint) (string, error) {
	return s.agents[hostID], nil
}

type sentPluginCommand struct {
	agentID string
	command string
	params  map[string]string
}

// stubPluginSender 对 offline 中的 Agent 返回错误；uninstallFail 中的 Agent 卸载失败。
// stubPluginSender errors for offline agents; agents in uninstallFail fail uninstall.
type stubPluginSender struct {
	mu            sync.Mutex
	offline       map[string]bool
	uninstallFail map[string]bool
	sent          []sentPluginCommand
}

func (s *stubPluginSender) SendCommand(_ context.Context, agentID, command string, params map[string]string) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, sentPluginCommand{agentID: agentID, command: command, params: params})
	if s.offline[agentID] {
		return false, "", errors.New("agent not connected")
	}
	if command == "uninstall_plugin" && s.uninstallFail[agentID] {
		return false, "permission denied", nil
	}
	return true, "ok", nil
}

func (s *stubPluginSender) commands(command string) []sentPluginCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []sentPluginCommand
	for _, c := range s.sent {
		if c.command == command {
			out = append(out, c)
		}
	}
	return out
}

func newPartialInstallTestService(t *testing.T, sender *stubPluginSender) (*Service, *Repository) {
	t.Helper()
	service, repo := newTestPluginServiceWithDownloader(t, t.TempDir())
	service.SetPluginFetcher(func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
		return []Plugin{{
			Name: "fakeconn", DisplayName: "Fake", Category: PluginCategoryConnector, Version: version,
			GroupID: "org.apache.seatunnel", ArtifactID: "connector-fakeconn",
		}}, MirrorSourceApache, nil
	})
	path := service.downloader.GetConnectorPath("connector-fakeconn", "2.3.13")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	service.SetClusterNodeGetter(stubPluginNodes{nodes: []ClusterNodeInfo{
		{NodeID: 1, HostID: 11, InstallDir: "/opt/st"},
		{NodeID: 2, HostID: 12, InstallDir: "/opt/st"},
		{NodeID: 3, HostID: 13, InstallDir: "/opt/st"},
	}})
	service.SetHostInfoGetter(stubPluginHosts{agents: map[uint]string{11: "a1", 12: "a2", 13: "a3"}})
	service.SetAgentCommandSender(sender)
	return service, repo
}

func TestInstallPluginToCluster_PartialFailureRollsBackInstalledNodes(t *testing.T) {
	sender := &stubPluginSender{offline: map[string]bool{"a2": true}}
	service, repo := newPartialInstallTestService(t, sender)
	ctx := context.Background()

	_, err := service.InstallPluginToCluster(ctx, 1, &InstallPluginRequest{PluginName: "fakeconn", Version: "2.3.13"})
	if err == nil {
		t.Fatal("expected install error")
	}
	if strings.Count(err.Error(), "agent not connected") != 1 {
		t.Fatalf("error text should not repeat the cause: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("error should mention rollback: %q", err.Error())
	}

	uninstalls := sender.commands("uninstall_plugin")
	if len(uninstalls) == 0 || uninstalls[0].agentID != "a1" || uninstalls[0].params["plugin_name"] != "fakeconn" || uninstalls[0].params["version"] != "2.3.13" {
		t.Fatalf("expected rollback uninstall on a1 with plugin_name=fakeconn, got %+v", uninstalls)
	}
	for _, c := range sender.sent {
		if c.agentID == "a3" {
			t.Fatalf("node after the failed node must not be touched: %+v", c)
		}
	}

	progress := service.GetInstallProgress(1, "fakeconn")
	if progress == nil || progress.Status != "failed" || progress.Partial || len(progress.NodeResults) != 3 {
		t.Fatalf("unexpected progress: %+v", progress)
	}
	want := []string{PluginNodeStatusRolledBack, PluginNodeStatusFailed, PluginNodeStatusSkipped}
	for i, item := range progress.NodeResults {
		if item.Status != want[i] {
			t.Fatalf("node %d status = %s, want %s", item.NodeID, item.Status, want[i])
		}
	}
	if strings.Count(progress.Error, "agent not connected") != 1 {
		t.Fatalf("progress error should not repeat the cause: %q", progress.Error)
	}
	if exists, _ := repo.ExistsByClusterAndName(ctx, 1, "fakeconn"); exists {
		t.Fatal("fully rolled back install must not leave a record")
	}
}

func TestInstallPluginToCluster_RollbackFailureKeepsPartialRecordAndAllowsRetry(t *testing.T) {
	sender := &stubPluginSender{offline: map[string]bool{"a2": true}, uninstallFail: map[string]bool{"a1": true}}
	service, repo := newPartialInstallTestService(t, sender)
	ctx := context.Background()

	_, err := service.InstallPluginToCluster(ctx, 1, &InstallPluginRequest{PluginName: "fakeconn", Version: "2.3.13"})
	if err == nil || !strings.Contains(err.Error(), "partially failed") {
		t.Fatalf("expected partial failure error, got %v", err)
	}
	record, err := repo.GetByClusterAndName(ctx, 1, "fakeconn")
	if err != nil {
		t.Fatalf("partial install must stay visible: %v", err)
	}
	if record.Status != PluginStatusPartial || len(record.NodeStatuses) != 3 || record.NodeStatuses[0].Status != PluginNodeStatusRollbackFailed {
		t.Fatalf("unexpected partial record: %+v", record)
	}
	if progress := service.GetInstallProgress(1, "fakeconn"); progress == nil || !progress.Partial {
		t.Fatalf("progress should be partial: %+v", progress)
	}

	// 节点恢复后重试安装：允许覆盖 partial 记录并补全为 installed。
	// Retry after the node recovers: overwrites the partial record and completes it.
	sender.offline = map[string]bool{}
	installed, err := service.InstallPluginToCluster(ctx, 1, &InstallPluginRequest{PluginName: "fakeconn", Version: "2.3.13"})
	if err != nil {
		t.Fatalf("retry install: %v", err)
	}
	if installed.ID != record.ID || installed.Status != PluginStatusInstalled {
		t.Fatalf("retry should complete the existing record: %+v", installed)
	}
	reloaded, err := repo.GetByClusterAndName(ctx, 1, "fakeconn")
	if err != nil || reloaded.Status != PluginStatusInstalled || len(reloaded.NodeStatuses) != 0 {
		t.Fatalf("unexpected record after retry: %+v err=%v", reloaded, err)
	}

	// 已完整安装后不允许重复安装。Fully installed plugins cannot be installed again.
	if _, err := service.InstallPluginToCluster(ctx, 1, &InstallPluginRequest{PluginName: "fakeconn", Version: "2.3.13"}); !errors.Is(err, ErrPluginAlreadyExists) {
		t.Fatalf("expected ErrPluginAlreadyExists, got %v", err)
	}
}

func TestInstallPluginToCluster_MissingAgentTouchesNoNode(t *testing.T) {
	sender := &stubPluginSender{}
	service, repo := newPartialInstallTestService(t, sender)
	service.SetHostInfoGetter(stubPluginHosts{agents: map[uint]string{11: "a1", 12: "a2"}})
	ctx := context.Background()

	if _, err := service.InstallPluginToCluster(ctx, 1, &InstallPluginRequest{PluginName: "fakeconn", Version: "2.3.13"}); err == nil {
		t.Fatal("expected error for missing agent")
	}
	if len(sender.sent) != 0 {
		t.Fatalf("no node should be touched, got %+v", sender.sent)
	}
	if exists, _ := repo.ExistsByClusterAndName(ctx, 1, "fakeconn"); exists {
		t.Fatal("no record expected")
	}
}

func TestUninstallPlugin_UsesArtifactDerivedName(t *testing.T) {
	sender := &stubPluginSender{}
	service, repo := newPartialInstallTestService(t, sender)
	ctx := context.Background()
	if err := repo.Create(ctx, &InstalledPlugin{ClusterID: 1, PluginName: "mysql-cdc", ArtifactID: "connector-cdc-mysql", Category: PluginCategoryConnector, Version: "2.3.13", Status: PluginStatusPartial}); err != nil {
		t.Fatal(err)
	}
	if err := service.UninstallPlugin(ctx, 1, "mysql-cdc"); err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}
	uninstalls := sender.commands("uninstall_plugin")
	if len(uninstalls) != 3 || uninstalls[0].params["plugin_name"] != "cdc-mysql" {
		t.Fatalf("expected 3 uninstalls with plugin_name=cdc-mysql, got %+v", uninstalls)
	}
	if exists, _ := repo.ExistsByClusterAndName(ctx, 1, "mysql-cdc"); exists {
		t.Fatal("record should be deleted")
	}
}
