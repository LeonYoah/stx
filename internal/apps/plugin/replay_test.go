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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const replayTestVersion = "9.9.9"

// replaySender 模拟 Agent：list_plugins 返回 nodePlugins；failTransfer 中的文件传输失败；记录全部命令。
// replaySender simulates an Agent: list_plugins returns nodePlugins, transfers of files in failTransfer
// fail, and every command is recorded.
type replaySender struct {
	mu           sync.Mutex
	nodePlugins  []map[string]string
	listErr      error
	failTransfer map[string]bool
	sent         []sentPluginCommand
}

func (s *replaySender) SendCommand(_ context.Context, agentID, command string, params map[string]string) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, sentPluginCommand{agentID: agentID, command: command, params: params})
	switch command {
	case "list_plugins":
		if s.listErr != nil {
			return false, "", s.listErr
		}
		plugins := s.nodePlugins
		if plugins == nil {
			plugins = []map[string]string{}
		}
		out, _ := json.Marshal(map[string]any{"success": true, "plugins": plugins})
		return true, string(out), nil
	case "transfer_plugin":
		if s.failTransfer[params["file_name"]] {
			return false, "disk full", nil
		}
	}
	return true, "ok", nil
}

func (s *replaySender) transferredFiles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := make([]string, 0)
	seen := map[string]bool{}
	for _, c := range s.sent {
		if c.command != "transfer_plugin" || seen[c.params["file_name"]] {
			continue
		}
		seen[c.params["file_name"]] = true
		files = append(files, c.params["target_dir"]+"/"+c.params["file_name"])
	}
	return files
}

func (s *replaySender) installs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, 0)
	for _, c := range s.sent {
		if c.command == "install_plugin" {
			result = append(result, c.params["artifact_id"])
		}
	}
	return result
}

func writeReplayFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newReplayTestService 准备：jdbc（画像 postgresql，依赖 postgresql 驱动）、fake（无元数据）、
// disabled 的 http、partial 的 hive（都需补齐），以及 available 的 redis（不补齐）；本地缓存中都有连接器文件。
// newReplayTestService prepares jdbc (profile postgresql with the postgresql driver), fake (no metadata),
// a disabled http and a partial hive (both replayed) plus an available redis (not replayed); every connector
// file is in the local cache.
func newReplayTestService(t *testing.T, sender *replaySender) (*Service, *Repository) {
	t.Helper()
	service, repo := newTestPluginServiceWithDownloader(t, t.TempDir())
	disableSeedAutoLoad(service, replayTestVersion)
	ctx := context.Background()

	if err := repo.UpsertDependencyProfile(ctx, &PluginDependencyProfile{
		SeatunnelVersion:    replayTestVersion,
		PluginName:          "jdbc",
		ArtifactID:          "connector-jdbc",
		ProfileKey:          "postgresql",
		EngineScope:         "zeta",
		SourceKind:          PluginDependencyProfileSourceOfficialSeed,
		BaselineVersionUsed: replayTestVersion,
		ResolutionMode:      DependencyResolutionModeExact,
		TargetDir:           "plugins/connector-jdbc",
		Confidence:          "high",
		Items: []PluginDependencyProfileItem{{
			GroupID: "org.postgresql", ArtifactID: "postgresql", Version: "42.4.3",
			TargetDir: "plugins/connector-jdbc", Required: true,
		}},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	now := time.Now()
	for _, record := range []*InstalledPlugin{
		{ClusterID: 1, PluginName: "jdbc", ArtifactID: "connector-jdbc", Category: PluginCategoryConnector, Version: replayTestVersion, Status: PluginStatusInstalled, InstalledAt: now},
		{ClusterID: 1, PluginName: "fake", ArtifactID: "connector-fake", Category: PluginCategoryConnector, Version: replayTestVersion, Status: PluginStatusEnabled, InstalledAt: now},
		{ClusterID: 1, PluginName: "http", ArtifactID: "connector-http", Category: PluginCategoryConnector, Version: replayTestVersion, Status: PluginStatusDisabled, InstalledAt: now},
		{ClusterID: 1, PluginName: "hive", ArtifactID: "connector-hive", Category: PluginCategoryConnector, Version: replayTestVersion, Status: PluginStatusPartial, InstalledAt: now},
		{ClusterID: 1, PluginName: "redis", ArtifactID: "connector-redis", Category: PluginCategoryConnector, Version: replayTestVersion, Status: PluginStatusAvailable, InstalledAt: now},
		{ClusterID: 2, PluginName: "kafka", ArtifactID: "connector-kafka", Category: PluginCategoryConnector, Version: replayTestVersion, Status: PluginStatusInstalled, InstalledAt: now},
	} {
		if err := repo.Create(ctx, record); err != nil {
			t.Fatalf("seed installed plugin: %v", err)
		}
	}
	for _, artifactID := range []string{"connector-jdbc", "connector-fake", "connector-http", "connector-hive", "connector-redis", "connector-kafka"} {
		writeReplayFile(t, service.downloader.GetConnectorPath(artifactID, replayTestVersion))
	}
	writeReplayFile(t, service.downloader.GetDependencyPath("postgresql", "42.4.3", replayTestVersion, "plugins/connector-jdbc"))
	if err := service.downloader.writePluginMetadata(&Plugin{Name: "jdbc", Version: replayTestVersion, ArtifactID: "connector-jdbc"}, []string{"postgresql"}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	service.SetHostInfoGetter(stubPluginHosts{agents: map[uint]string{21: "agent-t2"}})
	service.SetAgentCommandSender(sender)
	return service, repo
}

func replayResultsByName(report *PluginReplayReport) map[string]PluginReplayResult {
	byName := make(map[string]PluginReplayResult, len(report.Results))
	for _, result := range report.Results {
		byName[result.PluginName] = result
	}
	return byName
}

func TestReplayClusterPluginsToHostInstallsMissingWithProfiles(t *testing.T) {
	sender := &replaySender{}
	service, _ := newReplayTestService(t, sender)

	report, err := service.ReplayClusterPluginsToHost(context.Background(), 1, 21, "/opt/seatunnel")
	if err != nil {
		t.Fatalf("ReplayClusterPluginsToHost: %v", err)
	}
	if report.InstalledCount != 4 || report.SkippedCount != 1 || report.FailedCount != 0 || report.Partial() {
		t.Fatalf("unexpected report: %+v", report)
	}
	byName := replayResultsByName(report)
	if jdbc := byName["jdbc"]; jdbc.Status != PluginReplayInstalled || jdbc.ProfileUnknown || !reflect.DeepEqual(jdbc.ProfileKeys, []string{"postgresql"}) {
		t.Fatalf("jdbc result = %+v", jdbc)
	}
	if fake := byName["fake"]; fake.Status != PluginReplayInstalled || !fake.ProfileUnknown || fake.ProfileKeys == nil {
		t.Fatalf("fake result should be installed with profile_unknown, got %+v", fake)
	}
	// disabled 与 partial 都要补齐，只有 available 等非安装态记录跳过。
	// Disabled and partial plugins are replayed; only non-installed records such as available are skipped.
	if httpResult := byName["http"]; httpResult.Status != PluginReplayInstalled {
		t.Fatalf("disabled plugin should be replayed, got %+v", httpResult)
	}
	if hive := byName["hive"]; hive.Status != PluginReplayInstalled {
		t.Fatalf("partial plugin should be replayed, got %+v", hive)
	}
	if redis := byName["redis"]; redis.Status != PluginReplaySkipped {
		t.Fatalf("available plugin should be skipped, got %+v", redis)
	}
	if _, ok := byName["kafka"]; ok {
		t.Fatal("plugins of other clusters must not be replayed")
	}

	files := sender.transferredFiles()
	wantFiles := []string{
		"connectors/connector-jdbc-9.9.9.jar",
		"plugins/connector-jdbc/postgresql-42.4.3.jar",
		"connectors/connector-fake-9.9.9.jar",
		"connectors/connector-http-9.9.9.jar",
		"connectors/connector-hive-9.9.9.jar",
	}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("transferred files = %v, want %v", files, wantFiles)
	}
	if installs := sender.installs(); !reflect.DeepEqual(installs, []string{"connector-jdbc", "connector-fake", "connector-http", "connector-hive"}) {
		t.Fatalf("install commands = %v", installs)
	}
	for _, c := range sender.sent {
		if c.agentID != "agent-t2" || (c.command != "list_plugins" && c.params["install_path"] != "/opt/seatunnel") {
			t.Fatalf("command sent to wrong target: %+v", c)
		}
		if c.command == "uninstall_plugin" {
			t.Fatal("replay must never uninstall plugins from the node")
		}
	}
}

func TestReplayClusterPluginsToHostOnlyFillsMissing(t *testing.T) {
	sender := &replaySender{nodePlugins: []map[string]string{
		{"name": "jdbc", "version": replayTestVersion},
		{"name": "extra-only-on-node", "version": replayTestVersion},
	}}
	service, _ := newReplayTestService(t, sender)

	report, err := service.ReplayClusterPluginsToHost(context.Background(), 1, 21, "/opt/seatunnel")
	if err != nil {
		t.Fatalf("ReplayClusterPluginsToHost: %v", err)
	}
	byName := replayResultsByName(report)
	// jdbc 连接器已在节点上：只补传其驱动依赖，不重传连接器（2026-10-22 t2 实测驱动缺失的修复）。
	// The jdbc connector is already on the node: only its driver dependency is resent, not the connector (fix for
	// the missing driver found on t2, 2026-10-22).
	if byName["jdbc"].Status != PluginReplayInstalled || byName["fake"].Status != PluginReplayInstalled {
		t.Fatalf("unexpected results: %+v", report.Results)
	}
	files := sender.transferredFiles()
	if !containsString(files, "plugins/connector-jdbc/postgresql-42.4.3.jar") || containsString(files, "connectors/connector-jdbc-9.9.9.jar") {
		t.Fatalf("only the jdbc driver should be resent, got %v", files)
	}
	if installs := sender.installs(); !reflect.DeepEqual(installs, []string{"connector-fake", "connector-http", "connector-hive"}) {
		t.Fatalf("only the missing plugins should be installed, got %v", installs)
	}
}

func TestReplayClusterPluginsToHostContinuesAfterSingleFailure(t *testing.T) {
	sender := &replaySender{failTransfer: map[string]bool{"connector-jdbc-9.9.9.jar": true}}
	service, _ := newReplayTestService(t, sender)

	report, err := service.ReplayClusterPluginsToHost(context.Background(), 1, 21, "/opt/seatunnel")
	if err != nil {
		t.Fatalf("ReplayClusterPluginsToHost: %v", err)
	}
	byName := replayResultsByName(report)
	if byName["jdbc"].Status != PluginReplayFailed || byName["jdbc"].Message == "" {
		t.Fatalf("jdbc should fail with a message, got %+v", byName["jdbc"])
	}
	if byName["fake"].Status != PluginReplayInstalled {
		t.Fatalf("a single failure must not stop other plugins, got %+v", byName["fake"])
	}
	if !report.Partial() || report.FailedCount != 1 {
		t.Fatalf("report should be partial, got %+v", report)
	}
}

func TestReplayClusterPluginsToHostListFailureReplaysAll(t *testing.T) {
	sender := &replaySender{listErr: errors.New("timeout")}
	service, _ := newReplayTestService(t, sender)

	report, err := service.ReplayClusterPluginsToHost(context.Background(), 1, 21, "/opt/seatunnel")
	if err != nil {
		t.Fatalf("ReplayClusterPluginsToHost: %v", err)
	}
	if report.NodeListError == "" || report.InstalledCount != 4 {
		t.Fatalf("expected list error recorded and all plugins replayed, got %+v", report)
	}
}

func TestReplayClusterPluginsToHostPreconditions(t *testing.T) {
	service, _ := newTestPluginServiceWithDownloader(t, t.TempDir())
	if _, err := service.ReplayClusterPluginsToHost(context.Background(), 1, 21, "/opt/seatunnel"); !errors.Is(err, ErrPluginReplayUnavailable) {
		t.Fatalf("expected ErrPluginReplayUnavailable, got %v", err)
	}
	service.SetAgentCommandSender(&replaySender{})
	service.SetHostInfoGetter(stubPluginHosts{agents: map[uint]string{}})
	if _, err := service.ReplayClusterPluginsToHost(context.Background(), 1, 21, "/opt/seatunnel"); err == nil {
		t.Fatal("expected error for host without agent")
	}
	report, err := service.ReplayClusterPluginsToHost(context.Background(), 1, 21, " ")
	if err == nil || report == nil || report.Results == nil {
		t.Fatalf("expected install dir error with non-nil results, got %+v %v", report, err)
	}
}

func TestListReplayCandidatesMatchesReplayScope(t *testing.T) {
	service, _ := newReplayTestService(t, &replaySender{})

	candidates, err := service.ListReplayCandidates(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListReplayCandidates: %v", err)
	}
	names := make([]string, 0, len(candidates))
	byName := make(map[string]PluginReplayCandidate, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.PluginName)
		byName[candidate.PluginName] = candidate
		if candidate.ProfileKeys == nil {
			t.Fatalf("profile_keys must be [] not null: %+v", candidate)
		}
	}
	// 与补齐范围一致：available 的 redis 与其他集群的 kafka 不在列表中；按名称排序。
	// Same scope as replay: available redis and kafka of another cluster are excluded; sorted by name.
	if want := []string{"fake", "hive", "http", "jdbc"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("candidates = %v, want %v", names, want)
	}
	if jdbc := byName["jdbc"]; jdbc.ProfileUnknown || !reflect.DeepEqual(jdbc.ProfileKeys, []string{"postgresql"}) {
		t.Fatalf("jdbc candidate = %+v", jdbc)
	}
	if fake := byName["fake"]; !fake.ProfileUnknown {
		t.Fatalf("fake has no metadata and should be profile_unknown: %+v", fake)
	}
	empty, err := service.ListReplayCandidates(context.Background(), 99)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("cluster without plugins should return [], got %v %v", empty, err)
	}
}

// TestReplayInfersProfileFromCacheWhenMetadataMissing 元数据缺失时按缓存中已存在的依赖反推画像，驱动与已有节点一致；
// 依赖未缓存的画像不会被选中（2026-10-22 t2 实测：jdbc 只补了连接器、漏了 PostgreSQL 驱动）。
// TestReplayInfersProfileFromCacheWhenMetadataMissing infers the profile from cached dependencies when metadata is
// missing, so drivers match the existing nodes; profiles whose dependencies are not cached are not selected
// (found on t2, 2026-10-22: jdbc got the connector but not the PostgreSQL driver).
func TestReplayInfersProfileFromCacheWhenMetadataMissing(t *testing.T) {
	sender := &replaySender{}
	service, repo := newReplayTestService(t, sender)
	ctx := context.Background()
	if err := os.Remove(service.downloader.getMetadataPath("jdbc", replayTestVersion)); err != nil {
		t.Fatalf("remove metadata: %v", err)
	}
	// 另一个依赖未缓存的画像不应被推断出来。
	// Another profile whose dependency is not cached must not be inferred.
	if err := repo.UpsertDependencyProfile(ctx, &PluginDependencyProfile{
		SeatunnelVersion:    replayTestVersion,
		PluginName:          "jdbc",
		ArtifactID:          "connector-jdbc",
		ProfileKey:          "mysql",
		EngineScope:         "zeta",
		SourceKind:          PluginDependencyProfileSourceOfficialSeed,
		BaselineVersionUsed: replayTestVersion,
		ResolutionMode:      DependencyResolutionModeExact,
		TargetDir:           "plugins/connector-jdbc",
		Confidence:          "high",
		Items: []PluginDependencyProfileItem{{
			GroupID: "com.mysql", ArtifactID: "mysql-connector-j", Version: "8.0.33",
			TargetDir: "plugins/connector-jdbc", Required: true,
		}},
	}); err != nil {
		t.Fatalf("seed mysql profile: %v", err)
	}

	candidates, err := service.ListReplayCandidates(ctx, 1)
	if err != nil {
		t.Fatalf("ListReplayCandidates: %v", err)
	}
	for _, candidate := range candidates {
		if candidate.PluginName != "jdbc" {
			continue
		}
		if !candidate.ProfileInferred || candidate.ProfileUnknown || !reflect.DeepEqual(candidate.ProfileKeys, []string{"postgresql"}) {
			t.Fatalf("jdbc candidate should infer postgresql: %+v", candidate)
		}
	}

	report, err := service.ReplayClusterPluginsToHost(ctx, 1, 21, "/opt/seatunnel")
	if err != nil {
		t.Fatalf("ReplayClusterPluginsToHost: %v", err)
	}
	jdbc := replayResultsByName(report)["jdbc"]
	if jdbc.Status != PluginReplayInstalled || !jdbc.ProfileInferred || jdbc.ProfileUnknown || !reflect.DeepEqual(jdbc.ProfileKeys, []string{"postgresql"}) {
		t.Fatalf("jdbc replay should use the inferred profile: %+v", jdbc)
	}
	files := sender.transferredFiles()
	if !containsString(files, "plugins/connector-jdbc/postgresql-42.4.3.jar") {
		t.Fatalf("the cached PostgreSQL driver must be transferred: %v", files)
	}
	for _, file := range files {
		if strings.Contains(file, "mysql") {
			t.Fatalf("an uncached profile's driver must not be transferred: %v", files)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
