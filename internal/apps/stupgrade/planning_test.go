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

package stupgrade

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	hostapp "github.com/LeonYoah/stx/internal/apps/host"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	pluginapp "github.com/LeonYoah/stx/internal/apps/plugin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type stubClusterProvider struct {
	cluster *clusterapp.Cluster
	nodes   []*clusterapp.NodeInfo
}

func (s *stubClusterProvider) Get(ctx context.Context, id uint) (*clusterapp.Cluster, error) {
	return s.cluster, nil
}

func (s *stubClusterProvider) GetClusterNodesWithAgentInfo(ctx context.Context, clusterID uint) ([]*clusterapp.NodeInfo, error) {
	return s.nodes, nil
}

type stubHostProvider struct {
	hosts map[uint]*hostapp.Host
}

func (s *stubHostProvider) Get(ctx context.Context, id uint) (*hostapp.Host, error) {
	return s.hosts[id], nil
}

type stubPackageProvider struct {
	info *installerapp.PackageInfo
}

func (s *stubPackageProvider) GetPackageInfo(ctx context.Context, version string) (*installerapp.PackageInfo, error) {
	return s.info, nil
}

type stubPluginProvider struct {
	installed    []pluginapp.InstalledPlugin
	local        []pluginapp.LocalPlugin
	dependencies map[string][]pluginapp.PluginDependency
	requested    map[string][]string
	recorded     map[string]string
}

func (s *stubPluginProvider) ListInstalledPlugins(ctx context.Context, clusterID uint) ([]pluginapp.InstalledPlugin, error) {
	return s.installed, nil
}

func (s *stubPluginProvider) ListLocalPlugins() ([]pluginapp.LocalPlugin, error) {
	return s.local, nil
}

func (s *stubPluginProvider) GetPluginDependenciesForVersion(ctx context.Context, pluginName, version string) ([]pluginapp.PluginDependency, error) {
	if s.requested == nil {
		s.requested = make(map[string][]string)
	}
	s.requested[pluginName] = append(s.requested[pluginName], version)
	return s.dependencies[pluginName], nil
}

func (s *stubPluginProvider) GetPluginArtifactID(pluginName string) string {
	return "connector-" + pluginName
}

func (s *stubPluginProvider) TransferPluginToAgent(ctx context.Context, agentID, pluginName, version, installDir string, profileKeys []string) error {
	return nil
}

func (s *stubPluginProvider) RecordInstalledPlugin(ctx context.Context, clusterID uint, pluginName, version string) error {
	if s.recorded == nil {
		s.recorded = make(map[string]string)
	}
	s.recorded[pluginName] = version
	return nil
}

type stubConfigProvider struct {
	configs []*appconfig.ConfigInfo
}

func (s *stubConfigProvider) GetByCluster(ctx context.Context, clusterID uint) ([]*appconfig.ConfigInfo, error) {
	return s.configs, nil
}

func TestService_RunPrecheck_missingChecksumBlocks(t *testing.T) {
	service := newPlanningService(t, nil)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "target seatunnel content",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  1024,
		Checksum:  "",
	}})

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:     1,
		TargetVersion: "2.3.12",
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if result.Ready {
		t.Fatalf("expected precheck to be blocked when checksum is missing")
	}
	assertIssueCode(t, result.Issues, "checksum_missing")
}

func TestService_RunPrecheck_configMergePlanUsesEmptyConflictSlices(t *testing.T) {
	service := newPlanningService(t, nil)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "seatunnel: default",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  1024,
		Checksum:  "abc123",
	}})

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:     1,
		TargetVersion: "2.3.12",
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if result.ConfigMergePlan == nil {
		t.Fatalf("expected config merge plan to be present")
	}
	if len(result.ConfigMergePlan.Files) == 0 {
		t.Fatalf("expected config merge plan to include at least one file")
	}
	if result.ConfigMergePlan.Files[0].Conflicts == nil {
		t.Fatalf("expected config merge file conflicts to use an empty slice instead of nil")
	}

	payload, err := json.Marshal(result.ConfigMergePlan)
	if err != nil {
		t.Fatalf("failed to marshal config merge plan: %v", err)
	}
	if !strings.Contains(string(payload), `"conflicts":[]`) {
		t.Fatalf("expected serialized config merge plan to contain an empty conflicts array, got %s", string(payload))
	}
}

func TestService_RunPrecheck_configMergePlanUsesTargetContentFromPackage(t *testing.T) {
	service := newPlanningService(t, nil)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "seatunnel: target-version-default",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  1024,
		Checksum:  "abc123",
	}})

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:     1,
		TargetVersion: "2.3.12",
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if result.ConfigMergePlan == nil || len(result.ConfigMergePlan.Files) == 0 {
		t.Fatalf("expected config merge plan files to be present")
	}
	if got := result.ConfigMergePlan.Files[0].TargetContent; got != "seatunnel: target-version-default" {
		t.Fatalf("expected target content from package, got %q", got)
	}
	if got := result.ConfigMergePlan.Files[0].BaseContent; got == result.ConfigMergePlan.Files[0].TargetContent {
		t.Fatalf("expected base content and target content to differ in this test fixture")
	}
}

func TestService_RunPrecheck_missingTargetConfigInPackageBlocks(t *testing.T) {
	service := newPlanningService(t, nil)
	packagePath := createTestPackage(t, map[string]string{
		"config/hazelcast.yaml": "hazelcast: only",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  1024,
		Checksum:  "abc123",
	}})

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:     1,
		TargetVersion: "2.3.12",
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if result.Ready {
		t.Fatalf("expected precheck to be blocked when target package config is missing")
	}
	assertIssueCode(t, result.Issues, "package_config_missing")
}

func TestService_RunPrecheck_usesRequestedTargetInstallDir(t *testing.T) {
	service := newPlanningService(t, nil)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "seatunnel: default",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  1024,
		Checksum:  "abc123",
	}})

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:        1,
		TargetVersion:    "2.3.12",
		TargetInstallDir: "/data/apps/seatunnel-custom-2.3.12",
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if len(result.NodeTargets) != 1 {
		t.Fatalf("expected 1 node target, got %d", len(result.NodeTargets))
	}
	if got := result.NodeTargets[0].TargetInstallDir; got != "/data/apps/seatunnel-custom-2.3.12" {
		t.Fatalf("expected target install dir override to be used, got %q", got)
	}
}

func TestService_RunPrecheck_connectorManifestUsesPackageOverlayMode(t *testing.T) {
	service := newPlanningService(t, nil)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "seatunnel: default",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  1024,
		Checksum:  "abc123",
	}})

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:     1,
		TargetVersion: "2.3.12",
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if result.ConnectorManifest == nil {
		t.Fatalf("expected connector manifest to be present")
	}
	if got := result.ConnectorManifest.ReplacementMode; got != "package_overlay" {
		t.Fatalf("expected replacement_mode package_overlay, got %q", got)
	}
}

func TestService_CreatePlanFromRequest_persistsReadyPlan(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	service := newPlanningService(t, repo)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "seatunnel: default",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  4096,
		Checksum:  "abc123",
	}})
	service.SetPluginProvider(&stubPluginProvider{
		local: []pluginapp.LocalPlugin{{
			Name:          "jdbc",
			Version:       "2.3.12",
			Category:      pluginapp.PluginCategoryConnector,
			ConnectorPath: "/tmp/connector-jdbc-2.3.12.jar",
			DownloadedAt:  time.Now(),
		}},
		dependencies: map[string][]pluginapp.PluginDependency{
			"jdbc": {{
				GroupID:    "org.example",
				ArtifactID: "jdbc-extra",
				Version:    "1.0.0",
				TargetDir:  "lib",
			}},
		},
	})

	result, err := service.CreatePlanFromRequest(context.Background(), &CreatePlanRequest{
		PrecheckRequest: PrecheckRequest{
			ClusterID:      1,
			TargetVersion:  "2.3.12",
			ConnectorNames: []string{"jdbc"},
		},
	}, 7)
	if err != nil {
		t.Fatalf("CreatePlanFromRequest returned error: %v", err)
	}
	if result == nil || result.Plan == nil {
		t.Fatalf("expected plan to be created")
	}
	if result.Precheck == nil || !result.Precheck.Ready {
		t.Fatalf("expected precheck to succeed before plan creation")
	}
	if result.Plan.Snapshot.PackageManifest.Checksum != "abc123" {
		t.Fatalf("expected checksum to be persisted, got %q", result.Plan.Snapshot.PackageManifest.Checksum)
	}

	persisted, err := repo.GetPlanByID(context.Background(), result.Plan.ID)
	if err != nil {
		t.Fatalf("GetPlanByID returned error: %v", err)
	}
	if persisted.Snapshot.TargetVersion != "2.3.12" {
		t.Fatalf("expected target version 2.3.12, got %q", persisted.Snapshot.TargetVersion)
	}
	if len(persisted.Snapshot.NodeTargets) != 1 {
		t.Fatalf("expected 1 node target, got %d", len(persisted.Snapshot.NodeTargets))
	}
}

func TestService_CreatePlanFromRequest_splitsPluginDependencyManifest(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	service := newPlanningService(t, repo)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "seatunnel: default",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  4096,
		Checksum:  "abc123",
	}})
	service.SetPluginProvider(&stubPluginProvider{
		local: []pluginapp.LocalPlugin{{
			Name:          "jdbc",
			Version:       "2.3.12",
			Category:      pluginapp.PluginCategoryConnector,
			ConnectorPath: "/tmp/connector-jdbc-2.3.12.jar",
			DownloadedAt:  time.Now(),
		}},
		dependencies: map[string][]pluginapp.PluginDependency{
			"jdbc": {
				{
					GroupID:    "org.example",
					ArtifactID: "jdbc-extra",
					Version:    "1.0.0",
					TargetDir:  "lib",
				},
				{
					GroupID:    "org.example",
					ArtifactID: "oracle-extra",
					Version:    "2.0.0",
					TargetDir:  "plugins/connector-jdbc",
				},
			},
		},
	})

	result, err := service.CreatePlanFromRequest(context.Background(), &CreatePlanRequest{
		PrecheckRequest: PrecheckRequest{
			ClusterID:      1,
			TargetVersion:  "2.3.12",
			ConnectorNames: []string{"jdbc"},
		},
	}, 7)
	if err != nil {
		t.Fatalf("CreatePlanFromRequest returned error: %v", err)
	}
	if result == nil || result.Plan == nil {
		t.Fatalf("expected plan to be created")
	}
	if got := len(result.Plan.Snapshot.ConnectorManifest.Libraries); got != 1 {
		t.Fatalf("expected 1 lib dependency, got %d", got)
	}
	if got := len(result.Plan.Snapshot.ConnectorManifest.PluginDeps); got != 1 {
		t.Fatalf("expected 1 isolated dependency, got %d", got)
	}
	pluginDep := result.Plan.Snapshot.ConnectorManifest.PluginDeps[0]
	if pluginDep.TargetDir != "plugins/connector-jdbc" {
		t.Fatalf("expected plugin target dir to be preserved, got %q", pluginDep.TargetDir)
	}
	if pluginDep.RelativePath != "connector-jdbc/oracle-extra-2.0.0.jar" {
		t.Fatalf("expected plugin relative path to be generated, got %q", pluginDep.RelativePath)
	}
}

func TestService_CreatePlanFromRequest_usesTargetPluginVersionForDependencies(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	service := newPlanningService(t, repo)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "seatunnel: default",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  4096,
		Checksum:  "abc123",
	}})

	pluginProvider := &stubPluginProvider{
		installed: []pluginapp.InstalledPlugin{{
			PluginName: "jdbc",
			Version:    "2.3.11",
			Status:     pluginapp.PluginStatusInstalled,
		}},
		local: []pluginapp.LocalPlugin{{
			Name:          "jdbc",
			Version:       "2.3.12",
			Category:      pluginapp.PluginCategoryConnector,
			ConnectorPath: "/tmp/connector-jdbc-2.3.12.jar",
			DownloadedAt:  time.Now(),
		}},
		dependencies: map[string][]pluginapp.PluginDependency{
			"jdbc": {{
				GroupID:    "org.example",
				ArtifactID: "oracle-extra",
				Version:    "2.0.0",
				TargetDir:  "plugins/connector-jdbc",
			}},
		},
	}
	service.SetPluginProvider(pluginProvider)

	result, err := service.CreatePlanFromRequest(context.Background(), &CreatePlanRequest{
		PrecheckRequest: PrecheckRequest{
			ClusterID:      1,
			TargetVersion:  "2.3.12",
			ConnectorNames: []string{"jdbc"},
		},
	}, 7)
	if err != nil {
		t.Fatalf("CreatePlanFromRequest returned error: %v", err)
	}
	if result == nil || result.Plan == nil {
		t.Fatalf("expected plan to be created")
	}
	gotVersions := pluginProvider.requested["jdbc"]
	if len(gotVersions) == 0 {
		t.Fatalf("expected dependency lookup to be recorded")
	}
	if gotVersions[len(gotVersions)-1] != "2.3.12" {
		t.Fatalf("expected dependency lookup to use target plugin version 2.3.12, got %+v", gotVersions)
	}
}

func newPlanningService(t *testing.T, repo *Repository) *Service {
	t.Helper()
	service := NewService(repo)
	service.SetClusterProvider(&stubClusterProvider{
		cluster: &clusterapp.Cluster{
			ID:             1,
			Version:        "2.3.11",
			DeploymentMode: clusterapp.DeploymentModeHybrid,
		},
		nodes: []*clusterapp.NodeInfo{{
			ID:         11,
			ClusterID:  1,
			HostID:     101,
			HostName:   "node-a",
			HostIP:     "10.0.0.1",
			Role:       clusterapp.NodeRoleMasterWorker,
			InstallDir: "/opt/seatunnel-2.3.11",
			Status:     clusterapp.NodeStatusRunning,
			IsOnline:   true,
		}},
	})
	service.SetHostProvider(&stubHostProvider{hosts: map[uint]*hostapp.Host{
		101: {
			ID:        101,
			Name:      "node-a",
			IPAddress: "10.0.0.1",
			Arch:      "amd64",
			DiskUsage: 10,
			TotalDisk: 1024 * 1024 * 1024,
		},
	}})
	service.SetPluginProvider(&stubPluginProvider{})
	service.SetConfigProvider(&stubConfigProvider{configs: []*appconfig.ConfigInfo{{
		ID:         1,
		ClusterID:  1,
		ConfigType: appconfig.ConfigTypeSeatunnel,
		FilePath:   appconfig.GetConfigFilePath(appconfig.ConfigTypeSeatunnel),
		Content:    "seatunnel: default",
		IsTemplate: true,
	}}})
	return service
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := database.AutoMigrate(&UpgradePlanRecord{}, &UpgradeTask{}, &UpgradeTaskStep{}, &UpgradeNodeExecution{}, &UpgradeStepLog{}); err != nil {
		t.Fatalf("failed to migrate sqlite db: %v", err)
	}
	return database
}

func assertIssueCode(t *testing.T, issues []BlockingIssue, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("expected blocking issue %q, got %+v", code, issues)
}

func createTestPackage(t *testing.T, files map[string]string) string {
	t.Helper()
	packagePath := filepath.Join(t.TempDir(), "apache-seatunnel-test-bin.tar.gz")
	file, err := os.Create(packagePath)
	if err != nil {
		t.Fatalf("failed to create package file: %v", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for relativePath, content := range files {
		body := []byte(content)
		header := &tar.Header{
			Name: "apache-seatunnel-test/" + strings.TrimPrefix(relativePath, "/"),
			Mode: 0o644,
			Size: int64(len(body)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("failed to write tar header for %s: %v", relativePath, err)
		}
		if _, err := tarWriter.Write(body); err != nil {
			t.Fatalf("failed to write tar body for %s: %v", relativePath, err)
		}
	}

	return packagePath
}
