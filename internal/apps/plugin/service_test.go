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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type stubClusterVersionGetter struct {
	version string
	err     error
}

func (s *stubClusterVersionGetter) GetClusterVersion(ctx context.Context, clusterID uint) (string, error) {
	return s.version, s.err
}

func newTestPluginService(t *testing.T) (*Service, *Repository) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := database.AutoMigrate(
		&InstalledPlugin{},
		&PluginDependencyConfig{},
		&PluginDependencyDisable{},
		&PluginCatalogEntry{},
		&PluginDependencyProfile{},
		&PluginDependencyProfileItem{},
	); err != nil {
		t.Fatalf("failed to migrate plugin models: %v", err)
	}
	repo := NewRepository(database)
	service := NewService(repo)
	return service, repo
}

func createDependencyUploadFileHeader(t *testing.T, fieldName, fileName string, content []byte) *multipart.FileHeader {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file content failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/jdbc/dependencies/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(int64(body.Len()) + 1024); err != nil {
		t.Fatalf("parse multipart form failed: %v", err)
	}
	_, header, err := req.FormFile(fieldName)
	if err != nil {
		t.Fatalf("read multipart file failed: %v", err)
	}
	return header
}

func disableSeedAutoLoad(service *Service, versions ...string) {
	for _, version := range versions {
		if version == "" {
			continue
		}
		service.seedLoadedVersions[version] = true
	}
}

func newTestPluginServiceWithDownloader(t *testing.T, pluginsDir string) (*Service, *Repository) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := database.AutoMigrate(
		&InstalledPlugin{},
		&PluginDependencyConfig{},
		&PluginDependencyDisable{},
		&PluginCatalogEntry{},
		&PluginDependencyProfile{},
		&PluginDependencyProfileItem{},
	); err != nil {
		t.Fatalf("failed to migrate plugin models: %v", err)
	}
	repo := NewRepository(database)
	service := NewServiceWithDownloader(repo, pluginsDir)
	return service, repo
}

func TestListAvailablePluginsUsesDatabaseSnapshot(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()
	disableSeedAutoLoad(service, "9.9.9")

	if err := repo.UpsertCatalogEntries(ctx, []PluginCatalogEntry{{
		SeatunnelVersion: "9.9.9",
		PluginName:       "jdbc",
		DisplayName:      "Jdbc",
		ArtifactID:       "connector-jdbc",
		GroupID:          "org.apache.seatunnel",
		Category:         PluginCategoryConnector,
		Description:      "jdbc connector",
		Source:           PluginCatalogSourceRemote,
		SourceMirror:     string(MirrorSourceHuaweiCloud),
	}}); err != nil {
		t.Fatalf("failed to seed catalog: %v", err)
	}
	now := time.Now()
	if err := repo.db.WithContext(ctx).Model(&PluginCatalogEntry{}).
		Where("seatunnel_version = ? AND plugin_name = ?", "9.9.9", "jdbc").
		Updates(map[string]any{"refreshed_at": &now}).Error; err != nil {
		t.Fatalf("failed to update refreshed_at: %v", err)
	}

	service.SetPluginFetcher(func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
		t.Fatalf("database snapshot should avoid remote fetch")
		return nil, mirror, nil
	})

	result, err := service.ListAvailablePlugins(ctx, "9.9.9", MirrorSourceApache)
	if err != nil {
		t.Fatalf("ListAvailablePlugins returned error: %v", err)
	}
	if result.Source != PluginListSourceDatabase {
		t.Fatalf("expected source=database, got %q", result.Source)
	}
	if len(result.Plugins) != 1 || result.Plugins[0].Name != "jdbc" {
		t.Fatalf("unexpected plugins: %+v", result.Plugins)
	}
	if result.CatalogSourceMirror != string(MirrorSourceHuaweiCloud) {
		t.Fatalf("expected source mirror from db snapshot, got %q", result.CatalogSourceMirror)
	}
	if result.CatalogRefreshedAt == nil {
		t.Fatalf("expected catalog refreshed_at to be returned")
	}
}

func TestListInstalledPluginsEnrichesLocalMetadata(t *testing.T) {
	tempDir := t.TempDir()
	service, repo := newTestPluginServiceWithDownloader(t, tempDir)
	ctx := context.Background()

	installedAt := time.Now()
	if err := repo.db.WithContext(ctx).Create(&InstalledPlugin{
		ClusterID:   1,
		PluginName:  "jdbc",
		ArtifactID:  "connector-jdbc",
		Category:    PluginCategoryConnector,
		Version:     "2.3.13",
		Status:      PluginStatusInstalled,
		InstallPath: "/opt/seatunnel/connectors/connector-jdbc-2.3.13.jar",
		InstalledAt: installedAt,
	}).Error; err != nil {
		t.Fatalf("failed to create installed plugin: %v", err)
	}

	connectorPath := filepath.Join(tempDir, "2.3.13", "connectors", "connector-jdbc-2.3.13.jar")
	if err := os.MkdirAll(filepath.Dir(connectorPath), 0o755); err != nil {
		t.Fatalf("failed to create connector dir: %v", err)
	}
	if err := os.WriteFile(connectorPath, []byte("jar"), 0o644); err != nil {
		t.Fatalf("failed to create connector jar: %v", err)
	}

	metadata := localPluginMetadata{
		Name:                "jdbc",
		Version:             "2.3.13",
		ArtifactID:          "connector-jdbc",
		SelectedProfileKeys: []string{"mysql", "oracle"},
		AttachedConnectors:  []string{"connector-cdc-base"},
		Dependencies: []PluginDependency{{
			GroupID:    "mysql",
			ArtifactID: "mysql-connector-java",
			Version:    "8.0.27",
			TargetDir:  "plugins/connector-jdbc",
		}},
		UpdatedAt: time.Now(),
	}
	metadataPath := filepath.Join(tempDir, "2.3.13", "metadata", "jdbc.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		t.Fatalf("failed to create metadata dir: %v", err)
	}
	content, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("failed to marshal metadata: %v", err)
	}
	if err := os.WriteFile(metadataPath, content, 0o644); err != nil {
		t.Fatalf("failed to write metadata: %v", err)
	}

	plugins, err := service.ListInstalledPlugins(ctx, 1)
	if err != nil {
		t.Fatalf("ListInstalledPlugins returned error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 installed plugin, got %d", len(plugins))
	}
	if len(plugins[0].SelectedProfileKeys) != 2 {
		t.Fatalf("expected selected profiles to be enriched, got %+v", plugins[0].SelectedProfileKeys)
	}
	if len(plugins[0].AttachedConnectors) != 1 || plugins[0].AttachedConnectors[0] != "connector-cdc-base" {
		t.Fatalf("expected attached connectors to be enriched, got %+v", plugins[0].AttachedConnectors)
	}
	if len(plugins[0].Dependencies) != 1 || plugins[0].Dependencies[0].ArtifactID != "mysql-connector-java" {
		t.Fatalf("expected dependencies to be enriched, got %+v", plugins[0].Dependencies)
	}
}

func TestListLocalPlugins_keepsHttpBaseVisibleEvenWhenAutoAttached(t *testing.T) {
	tempDir := t.TempDir()
	service, _ := newTestPluginServiceWithDownloader(t, tempDir)

	version := "2.3.12"
	connectorsDir := filepath.Join(tempDir, version, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		t.Fatalf("failed to create connectors dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(connectorsDir, "connector-http-base-2.3.12.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatalf("failed to write http-base jar: %v", err)
	}
	if err := os.WriteFile(filepath.Join(connectorsDir, "connector-http-feishu-2.3.12.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatalf("failed to write http-feishu jar: %v", err)
	}

	metadataPath := filepath.Join(tempDir, version, "metadata", "http-feishu.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		t.Fatalf("failed to create metadata dir: %v", err)
	}
	content, err := json.Marshal(localPluginMetadata{
		Name:               "http-feishu",
		Version:            version,
		ArtifactID:         "connector-http-feishu",
		AttachedConnectors: []string{"connector-http-base"},
		UpdatedAt:          time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to marshal metadata: %v", err)
	}
	if err := os.WriteFile(metadataPath, content, 0o644); err != nil {
		t.Fatalf("failed to write metadata: %v", err)
	}

	plugins, err := service.ListLocalPlugins()
	if err != nil {
		t.Fatalf("ListLocalPlugins returned error: %v", err)
	}

	names := make(map[string]struct{}, len(plugins))
	for _, plugin := range plugins {
		names[plugin.Name] = struct{}{}
	}
	if _, ok := names["http-base"]; !ok {
		t.Fatalf("expected http-base to stay visible, got %+v", plugins)
	}
}

func TestListLocalPlugins_hidesFileBaseWhenOnlyAutoAttached(t *testing.T) {
	tempDir := t.TempDir()
	service, _ := newTestPluginServiceWithDownloader(t, tempDir)

	version := "2.3.12"
	connectorsDir := filepath.Join(tempDir, version, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		t.Fatalf("failed to create connectors dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(connectorsDir, "connector-file-base-2.3.12.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatalf("failed to write file-base jar: %v", err)
	}
	if err := os.WriteFile(filepath.Join(connectorsDir, "connector-file-oss-2.3.12.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatalf("failed to write file-oss jar: %v", err)
	}

	metadataPath := filepath.Join(tempDir, version, "metadata", "file-oss.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		t.Fatalf("failed to create metadata dir: %v", err)
	}
	content, err := json.Marshal(localPluginMetadata{
		Name:               "file-oss",
		Version:            version,
		ArtifactID:         "connector-file-oss",
		AttachedConnectors: []string{"connector-file-base"},
		UpdatedAt:          time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to marshal metadata: %v", err)
	}
	if err := os.WriteFile(metadataPath, content, 0o644); err != nil {
		t.Fatalf("failed to write metadata: %v", err)
	}

	plugins, err := service.ListLocalPlugins()
	if err != nil {
		t.Fatalf("ListLocalPlugins returned error: %v", err)
	}
	for _, plugin := range plugins {
		if plugin.Name == "file-base" {
			t.Fatalf("expected file-base to remain hidden when only auto-attached, got %+v", plugins)
		}
	}
}

func TestListInstalledPluginsReconcilesClusterVersion(t *testing.T) {
	tempDir := t.TempDir()
	service, repo := newTestPluginServiceWithDownloader(t, tempDir)
	service.SetClusterGetter(&stubClusterVersionGetter{version: "2.3.13"})
	ctx := context.Background()

	if err := repo.db.WithContext(ctx).Create(&InstalledPlugin{
		ClusterID:   1,
		PluginName:  "jdbc",
		ArtifactID:  "connector-jdbc",
		Category:    PluginCategoryConnector,
		Version:     "2.3.11",
		Status:      PluginStatusInstalled,
		InstallPath: "connectors/connector-jdbc-2.3.11.jar",
		InstalledAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("failed to create installed plugin: %v", err)
	}

	plugins, err := service.ListInstalledPlugins(ctx, 1)
	if err != nil {
		t.Fatalf("ListInstalledPlugins returned error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 installed plugin, got %d", len(plugins))
	}
	if plugins[0].Version != "2.3.13" {
		t.Fatalf("expected reconciled version 2.3.13, got %q", plugins[0].Version)
	}
	if plugins[0].InstallPath != "connectors/connector-jdbc-2.3.13.jar" {
		t.Fatalf("expected reconciled install path, got %q", plugins[0].InstallPath)
	}

	var stored InstalledPlugin
	if err := repo.db.WithContext(ctx).First(&stored, plugins[0].ID).Error; err != nil {
		t.Fatalf("failed to reload stored plugin: %v", err)
	}
	if stored.Version != "2.3.13" {
		t.Fatalf("expected stored version 2.3.13, got %q", stored.Version)
	}
}

func TestInstallPluginToClusterRecordsAttachedConnectorDependencies(t *testing.T) {
	tempDir := t.TempDir()
	service, repo := newTestPluginServiceWithDownloader(t, tempDir)
	ctx := context.Background()
	version := "2.3.13"

	service.SetPluginFetcher(func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
		return []Plugin{{
			Name:        "cdc-mysql",
			DisplayName: "Cdc Mysql",
			Category:    PluginCategoryConnector,
			Version:     version,
			Description: "Cdc Mysql connector",
			GroupID:     "org.apache.seatunnel",
			ArtifactID:  "connector-cdc-mysql",
		}}, MirrorSourceApache, nil
	})

	writeTestFile := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("jar"), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}
	writeTestFile(service.downloader.GetConnectorPath("connector-cdc-mysql", version))
	writeTestFile(service.downloader.GetDependencyPath("connector-jdbc", version, version, "connectors"))
	writeTestFile(service.downloader.GetDependencyPath("mysql-connector-java", "8.0.28", version, "plugins/connector-cdc-mysql"))

	installed, err := service.InstallPluginToCluster(ctx, 1, &InstallPluginRequest{
		PluginName: "cdc-mysql",
		Version:    version,
	})
	if err != nil {
		t.Fatalf("InstallPluginToCluster returned error: %v", err)
	}
	if installed.PluginName != "cdc-mysql" {
		t.Fatalf("expected primary plugin cdc-mysql, got %s", installed.PluginName)
	}

	plugins, err := repo.ListByCluster(ctx, 1)
	if err != nil {
		t.Fatalf("ListByCluster returned error: %v", err)
	}
	byName := make(map[string]InstalledPlugin, len(plugins))
	for _, item := range plugins {
		byName[item.PluginName] = item
	}
	if _, ok := byName["cdc-mysql"]; !ok {
		t.Fatalf("expected primary plugin record, got %+v", plugins)
	}
	jdbc, ok := byName["jdbc"]
	if !ok {
		t.Fatalf("expected attached jdbc connector record, got %+v", plugins)
	}
	if jdbc.ArtifactID != "connector-jdbc" || jdbc.InstallPath != "connectors/connector-jdbc-2.3.13.jar" {
		t.Fatalf("unexpected jdbc record: %+v", jdbc)
	}
	if _, ok := byName["mysql-connector-java"]; ok {
		t.Fatalf("expected non-connector dependency to stay out of installed plugins, got %+v", plugins)
	}
}

func TestListAvailablePluginsFetchesRemoteAndPersistsCatalog(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()
	disableSeedAutoLoad(service, "9.9.9")

	service.SetPluginFetcher(func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
		if mirror != MirrorSourceAliyun {
			t.Fatalf("expected catalog fetch to use requested mirror, got %q", mirror)
		}
		return []Plugin{{
			Name:        "hive",
			DisplayName: "Hive",
			Category:    PluginCategoryConnector,
			Version:     version,
			Description: "Hive connector",
			GroupID:     "org.apache.seatunnel",
			ArtifactID:  "connector-hive",
		}}, MirrorSourceApache, nil
	})

	result, err := service.ListAvailablePlugins(ctx, "9.9.9", MirrorSourceAliyun)
	if err != nil {
		t.Fatalf("ListAvailablePlugins returned error: %v", err)
	}
	if result.Source != PluginListSourceRemote {
		t.Fatalf("expected source=remote, got %q", result.Source)
	}
	if result.CatalogSourceMirror != string(MirrorSourceApache) {
		t.Fatalf("expected response source mirror=apache, got %q", result.CatalogSourceMirror)
	}
	if result.CatalogRefreshedAt == nil {
		t.Fatalf("expected response refreshed_at")
	}

	entries, err := repo.ListCatalogEntriesByVersion(ctx, "9.9.9")
	if err != nil {
		t.Fatalf("failed to read catalog entries: %v", err)
	}
	if len(entries) != 1 || entries[0].PluginName != "hive" {
		t.Fatalf("unexpected persisted entries: %+v", entries)
	}
	if entries[0].Source != PluginCatalogSourceRemote {
		t.Fatalf("expected persisted source=remote, got %q", entries[0].Source)
	}
	if entries[0].SourceMirror != string(MirrorSourceApache) {
		t.Fatalf("expected persisted source_mirror=apache, got %q", entries[0].SourceMirror)
	}
	if entries[0].RefreshedAt == nil {
		t.Fatalf("expected persisted refreshed_at")
	}
}

func TestListAvailablePluginsFallsBackToBundledSeedWhenRemoteRateLimited(t *testing.T) {
	service, _ := newTestPluginService(t)
	ctx := context.Background()
	calls := 0

	service.SetPluginFetcher(func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
		calls++
		return nil, mirror, errors.New("unexpected status code: 429")
	})

	result, err := service.ListAvailablePlugins(ctx, "2.3.13", MirrorSourceApache)
	if err != nil {
		t.Fatalf("ListAvailablePlugins returned error: %v", err)
	}
	if result.Source != PluginListSourceSeed {
		t.Fatalf("expected source=seed, got %q", result.Source)
	}
	if result.Total == 0 {
		t.Fatalf("expected bundled seed plugins to be returned")
	}
	if calls != 1 {
		t.Fatalf("expected first call to try remote once, got %d", calls)
	}

	cached, err := service.ListAvailablePlugins(ctx, "2.3.13", MirrorSourceApache)
	if err != nil {
		t.Fatalf("second ListAvailablePlugins returned error: %v", err)
	}
	if cached.Source != PluginListSourceSeed || !cached.CacheHit {
		t.Fatalf("expected cached seed fallback, got source=%q cache_hit=%t", cached.Source, cached.CacheHit)
	}
	if calls != 1 {
		t.Fatalf("expected remote fetch to be skipped during cooldown, got %d calls", calls)
	}
}

func TestListAvailablePluginsIgnoresLegacySeedCatalogSnapshot(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()

	if err := repo.UpsertCatalogEntries(ctx, []PluginCatalogEntry{
		{
			SeatunnelVersion: "2.3.13",
			PluginName:       "legacy-seed-only",
			DisplayName:      "Legacy Seed Only",
			ArtifactID:       "connector-legacy-seed-only",
			GroupID:          "org.apache.seatunnel",
			Category:         PluginCategoryConnector,
			Description:      "seed-only connector",
			Source:           PluginCatalogSourceSeed,
		},
		{
			SeatunnelVersion: "2.3.13",
			PluginName:       "stale-remote",
			DisplayName:      "Stale Remote",
			ArtifactID:       "connector-stale-remote",
			GroupID:          "org.apache.seatunnel",
			Category:         PluginCategoryConnector,
			Description:      "stale remote connector",
			Source:           PluginCatalogSourceRemote,
		},
	}); err != nil {
		t.Fatalf("failed to seed legacy catalog entry: %v", err)
	}

	service.SetPluginFetcher(func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
		return []Plugin{{
			Name:        "jdbc",
			DisplayName: "Jdbc",
			Category:    PluginCategoryConnector,
			Version:     version,
			Description: "Jdbc connector",
			GroupID:     "org.apache.seatunnel",
			ArtifactID:  "connector-jdbc",
		}}, MirrorSourceApache, nil
	})

	result, err := service.ListAvailablePlugins(ctx, "2.3.13", MirrorSourceApache)
	if err != nil {
		t.Fatalf("ListAvailablePlugins returned error: %v", err)
	}
	if result.Source != PluginListSourceRemote {
		t.Fatalf("expected source=remote when only seed catalog exists, got %q", result.Source)
	}
	if len(result.Plugins) != 1 || result.Plugins[0].Name != "jdbc" {
		t.Fatalf("unexpected plugins: %+v", result.Plugins)
	}

	entries, err := repo.ListCatalogEntriesByVersion(ctx, "2.3.13")
	if err != nil {
		t.Fatalf("failed to read refreshed catalog entries: %v", err)
	}
	if len(entries) != 1 || entries[0].PluginName != "jdbc" || entries[0].Source != PluginCatalogSourceRemote {
		t.Fatalf("expected legacy seed snapshot to be replaced by remote catalog, got %+v", entries)
	}
}

func TestListAvailablePluginsFor2312UsesRemoteCatalogAndSeedDependencyBaseline(t *testing.T) {
	service, _ := newTestPluginService(t)
	ctx := context.Background()

	service.SetPluginFetcher(func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
		return []Plugin{
			{
				Name:        "hive",
				DisplayName: "Hive",
				Category:    PluginCategoryConnector,
				Version:     version,
				Description: "Hive connector",
				GroupID:     "org.apache.seatunnel",
				ArtifactID:  "connector-hive",
			},
			{
				Name:        "file-base",
				DisplayName: "File Base",
				Category:    PluginCategoryConnector,
				Version:     version,
				Description: "Hidden base connector",
				GroupID:     "org.apache.seatunnel",
				ArtifactID:  "connector-file-base",
			},
			{
				Name:        "jdbc",
				DisplayName: "Jdbc",
				Category:    PluginCategoryConnector,
				Version:     version,
				Description: "Jdbc connector",
				GroupID:     "org.apache.seatunnel",
				ArtifactID:  "connector-jdbc",
			},
		}, MirrorSourceApache, nil
	})

	result, err := service.ListAvailablePlugins(ctx, "2.3.12", MirrorSourceApache)
	if err != nil {
		t.Fatalf("ListAvailablePlugins returned error: %v", err)
	}
	if result.Source != PluginListSourceRemote {
		t.Fatalf("expected source=remote, got %q", result.Source)
	}
	if len(result.Plugins) != 2 {
		t.Fatalf("expected 2 visible plugins after filtering hidden ones, got %d", len(result.Plugins))
	}

	names := make(map[string]Plugin)
	for _, item := range result.Plugins {
		names[item.Name] = item
	}
	if _, ok := names["file-base"]; ok {
		t.Fatalf("expected hidden plugin to be removed from marketplace result")
	}
	if names["hive"].DependencyStatus != PluginDependencyStatusReadyExact {
		t.Fatalf("expected hive dependency baseline from bundled seed, got %q", names["hive"].DependencyStatus)
	}
	if names["jdbc"].DependencyStatus != PluginDependencyStatusUnknown {
		t.Fatalf("expected jdbc unknown due to multiple seeded profiles, got %q", names["jdbc"].DependencyStatus)
	}
}

func TestAddDependencyUsesIsolatedTargetDirFor2312(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()

	if err := repo.UpsertCatalogEntries(ctx, []PluginCatalogEntry{{
		SeatunnelVersion: "2.3.12",
		PluginName:       "doris",
		DisplayName:      "Doris",
		ArtifactID:       "connector-doris",
		GroupID:          "org.apache.seatunnel",
		Category:         PluginCategoryConnector,
		Source:           PluginCatalogSourceSeed,
	}}); err != nil {
		t.Fatalf("failed to seed catalog: %v", err)
	}

	dep, err := service.AddDependency(ctx, &AddDependencyRequest{
		PluginName:       "doris",
		SeatunnelVersion: "2.3.12",
		GroupID:          "mysql",
		ArtifactID:       "mysql-connector-java",
		Version:          "8.0.28",
	})
	if err != nil {
		t.Fatalf("AddDependency returned error: %v", err)
	}
	if dep.TargetDir != "plugins/connector-doris" {
		t.Fatalf("expected isolated target dir, got %q", dep.TargetDir)
	}
}

func TestBundledSeedFileS3UsesFileBaseAndExcludesBundledHadoopJars(t *testing.T) {
	service, _ := newTestPluginService(t)
	ctx := context.Background()

	result, err := service.GetOfficialDependencies(ctx, "file-s3", "2.3.12", "")
	if err != nil {
		t.Fatalf("GetOfficialDependencies returned error: %v", err)
	}
	if result.DependencyStatus != PluginDependencyStatusReadyExact {
		t.Fatalf("expected ready_exact, got %q", result.DependencyStatus)
	}
	if len(result.EffectiveDependencies) == 0 {
		t.Fatalf("expected effective dependencies for file-s3")
	}

	foundFileBase := false
	for _, item := range result.EffectiveDependencies {
		if item.ArtifactID == "connector-file-base" && item.TargetDir == "connectors" {
			foundFileBase = true
		}
		if item.ArtifactID == "seatunnel-hadoop3-3.1.4-uber" || item.ArtifactID == "seatunnel-hadoop-aws" || item.ArtifactID == "hadoop-aws" {
			t.Fatalf("expected bundled hadoop jars to be excluded, found %s", item.ArtifactID)
		}
	}
	if !foundFileBase {
		t.Fatalf("expected file-s3 to auto-attach connector-file-base")
	}
}

func TestGetOfficialDependenciesResolvesFallbackProfile(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()
	disableSeedAutoLoad(service, "2.3.13")

	profile := &PluginDependencyProfile{
		SeatunnelVersion:    "2.3.12",
		PluginName:          "hive",
		ArtifactID:          "connector-hive",
		ProfileKey:          "default",
		EngineScope:         "zeta",
		SourceKind:          PluginDependencyProfileSourceOfficialSeed,
		BaselineVersionUsed: "2.3.12",
		ResolutionMode:      DependencyResolutionModeExact,
		TargetDir:           "lib",
		Confidence:          "high",
		IsDefault:           true,
		Items: []PluginDependencyProfileItem{{
			GroupID:    "org.apache.hive",
			ArtifactID: "hive-exec",
			Version:    "3.1.3",
			TargetDir:  "lib",
			Required:   true,
		}},
	}
	if err := repo.UpsertDependencyProfile(ctx, profile); err != nil {
		t.Fatalf("failed to upsert profile: %v", err)
	}

	result, err := service.GetOfficialDependencies(ctx, "hive", "2.3.13", "")
	if err != nil {
		t.Fatalf("GetOfficialDependencies returned error: %v", err)
	}
	if result.DependencyStatus != PluginDependencyStatusReadyFallback {
		t.Fatalf("expected fallback status, got %q", result.DependencyStatus)
	}
	if result.BaselineVersionUsed != "2.3.12" {
		t.Fatalf("expected baseline 2.3.12, got %q", result.BaselineVersionUsed)
	}
	if result.DependencyResolutionMode != DependencyResolutionModeFallback {
		t.Fatalf("expected fallback mode, got %q", result.DependencyResolutionMode)
	}
	if result.DependencyCount != 1 {
		t.Fatalf("expected dependency_count=1, got %d", result.DependencyCount)
	}
}

func TestGetOfficialDependenciesReturnsUnknownForAmbiguousProfiles(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()
	disableSeedAutoLoad(service, "2.3.12")

	for _, key := range []string{"oracle", "hivejdbc"} {
		if err := repo.UpsertDependencyProfile(ctx, &PluginDependencyProfile{
			SeatunnelVersion:    "2.3.12",
			PluginName:          "jdbc",
			ArtifactID:          "connector-jdbc",
			ProfileKey:          key,
			EngineScope:         "zeta",
			SourceKind:          PluginDependencyProfileSourceOfficialSeed,
			BaselineVersionUsed: "2.3.12",
			ResolutionMode:      DependencyResolutionModeExact,
			TargetDir:           "lib",
			Confidence:          "high",
			IsDefault:           false,
			Items: []PluginDependencyProfileItem{{
				GroupID:    "test.group",
				ArtifactID: key,
				Version:    "1.0.0",
				TargetDir:  "lib",
				Required:   true,
			}},
		}); err != nil {
			t.Fatalf("failed to seed profile %s: %v", key, err)
		}
	}

	result, err := service.GetOfficialDependencies(ctx, "jdbc", "2.3.12", "")
	if err != nil {
		t.Fatalf("GetOfficialDependencies returned error: %v", err)
	}
	if result.DependencyStatus != PluginDependencyStatusUnknown {
		t.Fatalf("expected unknown status, got %q", result.DependencyStatus)
	}
	if len(result.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(result.Profiles))
	}
	if len(result.EffectiveDependencies) != 0 {
		t.Fatalf("expected no effective deps for ambiguous profiles, got %d", len(result.EffectiveDependencies))
	}
}

func TestGetOfficialDependenciesTreatsHiveJdbcAsHiveAlias(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()
	disableSeedAutoLoad(service, "2.3.13")

	if err := repo.UpsertDependencyProfile(ctx, &PluginDependencyProfile{
		SeatunnelVersion:    "2.3.13",
		PluginName:          "jdbc",
		ArtifactID:          "connector-jdbc",
		ProfileKey:          "hive",
		EngineScope:         "zeta",
		SourceKind:          PluginDependencyProfileSourceOfficialSeed,
		BaselineVersionUsed: "2.3.13",
		ResolutionMode:      DependencyResolutionModeExact,
		TargetDir:           "plugins/connector-jdbc",
		Confidence:          "manual",
		IsDefault:           false,
		Items: []PluginDependencyProfileItem{{
			GroupID:    "org.apache.hive",
			ArtifactID: "hive-jdbc",
			Version:    "3.1.3",
			TargetDir:  "plugins/connector-jdbc",
			Required:   true,
		}},
	}); err != nil {
		t.Fatalf("failed to seed hive profile: %v", err)
	}
	if err := repo.UpsertDependencyProfile(ctx, &PluginDependencyProfile{
		SeatunnelVersion:    "2.3.12",
		PluginName:          "jdbc",
		ArtifactID:          "connector-jdbc",
		ProfileKey:          "hivejdbc",
		EngineScope:         "zeta",
		SourceKind:          PluginDependencyProfileSourceRuntimeAnalyzed,
		BaselineVersionUsed: "2.3.12",
		ResolutionMode:      DependencyResolutionModeRuntime,
		TargetDir:           "plugins/connector-jdbc",
		Confidence:          "medium",
		IsDefault:           false,
		Items: []PluginDependencyProfileItem{{
			GroupID:    "org.apache.hive",
			ArtifactID: "hive-jdbc",
			Version:    "3.1.3",
			TargetDir:  "plugins/connector-jdbc",
			Required:   true,
		}},
	}); err != nil {
		t.Fatalf("failed to seed hivejdbc runtime profile: %v", err)
	}

	result, err := service.GetOfficialDependencies(ctx, "jdbc", "2.3.13", "")
	if err != nil {
		t.Fatalf("GetOfficialDependencies returned error: %v", err)
	}
	if len(result.Profiles) != 1 {
		t.Fatalf("expected 1 selected profile after alias merge, got %d", len(result.Profiles))
	}
	if result.Profiles[0].ProfileKey != "hive" {
		t.Fatalf("expected selected profile key hive, got %q", result.Profiles[0].ProfileKey)
	}
	if result.Profiles[0].SeatunnelVersion != "2.3.13" {
		t.Fatalf("expected exact 2.3.13 hive profile, got %q", result.Profiles[0].SeatunnelVersion)
	}
}

func TestGetOfficialDependenciesUsesJdbcProfileMatrix(t *testing.T) {
	service, _ := newTestPluginService(t)
	ctx := context.Background()

	testCases := []struct {
		name           string
		version        string
		profileKey     string
		expectedStatus PluginDependencyStatus
		expectedItems  map[string]string
	}{
		{
			name:           "mysql keeps pinned version",
			version:        "2.3.13",
			profileKey:     "mysql",
			expectedStatus: PluginDependencyStatusReadyExact,
			expectedItems: map[string]string{
				"mysql-connector-java": "8.0.27",
			},
		},
		{
			name:           "oracle uses three locked artifacts",
			version:        "2.3.13",
			profileKey:     "oracle",
			expectedStatus: PluginDependencyStatusReadyExact,
			expectedItems: map[string]string{
				"ojdbc8":      "12.2.0.1",
				"xdb6":        "12.2.0.1",
				"xmlparserv2": "12.2.0.1",
			},
		},
		{
			name:           "redshift old range",
			version:        "2.3.12",
			profileKey:     "redshift",
			expectedStatus: PluginDependencyStatusReadyExact,
			expectedItems: map[string]string{
				"redshift-jdbc42": "2.1.0.9",
			},
		},
		{
			name:           "redshift new range",
			version:        "2.3.13",
			profileKey:     "redshift",
			expectedStatus: PluginDependencyStatusReadyExact,
			expectedItems: map[string]string{
				"redshift-jdbc42": "2.1.0.30",
			},
		},
		{
			name:           "sap hana old range",
			version:        "2.3.10",
			profileKey:     "sap-hana",
			expectedStatus: PluginDependencyStatusReadyFallback,
			expectedItems: map[string]string{
				"ngdbc": "2.14.7",
			},
		},
		{
			name:           "sap hana new range",
			version:        "2.3.13",
			profileKey:     "sap-hana",
			expectedStatus: PluginDependencyStatusReadyExact,
			expectedItems: map[string]string{
				"ngdbc": "2.23.10",
			},
		},
		{
			name:           "duckdb starts at 2.3.13",
			version:        "2.3.13",
			profileKey:     "duckdb",
			expectedStatus: PluginDependencyStatusReadyExact,
			expectedItems: map[string]string{
				"duckdb_jdbc": "1.3.1.0",
			},
		},
		{
			name:           "aws dsql keeps only postgres driver",
			version:        "2.3.13",
			profileKey:     "aws-dsql",
			expectedStatus: PluginDependencyStatusReadyExact,
			expectedItems: map[string]string{
				"postgresql": "42.4.3",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.GetOfficialDependencies(ctx, "jdbc", tc.version, tc.profileKey)
			if err != nil {
				t.Fatalf("GetOfficialDependencies returned error: %v", err)
			}
			if result.DependencyStatus != tc.expectedStatus {
				t.Fatalf("expected %q, got %q", tc.expectedStatus, result.DependencyStatus)
			}
			if len(result.EffectiveDependencies) != len(tc.expectedItems) {
				t.Fatalf("expected %d effective deps, got %d: %+v", len(tc.expectedItems), len(result.EffectiveDependencies), result.EffectiveDependencies)
			}
			for _, dep := range result.EffectiveDependencies {
				expectedVersion, ok := tc.expectedItems[dep.ArtifactID]
				if !ok {
					t.Fatalf("unexpected artifact %s", dep.ArtifactID)
				}
				if dep.Version != expectedVersion {
					t.Fatalf("expected %s version %s, got %s", dep.ArtifactID, expectedVersion, dep.Version)
				}
			}
		})
	}
}

func TestGetEffectiveDependenciesMergesOfficialAndUserDeps(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()
	disableSeedAutoLoad(service, "9.9.9")

	if err := repo.UpsertDependencyProfile(ctx, &PluginDependencyProfile{
		SeatunnelVersion:    "9.9.9",
		PluginName:          "hive",
		ArtifactID:          "connector-hive",
		ProfileKey:          "default",
		EngineScope:         "zeta",
		SourceKind:          PluginDependencyProfileSourceOfficialSeed,
		BaselineVersionUsed: "9.9.9",
		ResolutionMode:      DependencyResolutionModeExact,
		TargetDir:           "lib",
		Confidence:          "high",
		IsDefault:           true,
		Items: []PluginDependencyProfileItem{{
			GroupID:    "org.apache.hive",
			ArtifactID: "hive-exec",
			Version:    "3.1.3",
			TargetDir:  "lib",
			Required:   true,
		}},
	}); err != nil {
		t.Fatalf("failed to seed official profile: %v", err)
	}
	if err := repo.UpsertDependency(ctx, &PluginDependencyConfig{
		PluginName:       "hive",
		SeatunnelVersion: "9.9.9",
		GroupID:          "org.apache.thrift",
		ArtifactID:       "libfb303",
		Version:          "0.9.3",
		TargetDir:        "lib",
		SourceType:       PluginDependencySourceMaven,
	}); err != nil {
		t.Fatalf("failed to seed user dependency: %v", err)
	}

	deps, err := service.GetEffectiveDependencies(ctx, "hive", "9.9.9", nil)
	if err != nil {
		t.Fatalf("GetEffectiveDependencies returned error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %+v", len(deps), deps)
	}
}

func TestAnalyzeOfficialDependenciesStoresRuntimeProfile(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()
	disableSeedAutoLoad(service, "9.9.9")

	service.officialDocFetcher = func(ctx context.Context, version, docSlug string) (string, error) {
		return `# Oracle CDC
| Oracle | https://mvnrepository.com/artifact/com.oracle.database.jdbc/ojdbc8 |
`, nil
	}
	service.mavenVersionLookup = func(ctx context.Context, groupID, artifactID string) (string, error) {
		return "1.2.3", nil
	}

	result, err := service.AnalyzeOfficialDependencies(ctx, "cdc-oracle", "9.9.9", "", true)
	if err != nil {
		t.Fatalf("AnalyzeOfficialDependencies returned error: %v", err)
	}
	if result.DependencyStatus != PluginDependencyStatusRuntimeAnalyzed {
		t.Fatalf("expected runtime analyzed status, got %q", result.DependencyStatus)
	}
	if result.DependencyCount != 2 {
		t.Fatalf("expected 2 dependencies, got %d", result.DependencyCount)
	}

	profiles, err := repo.ListDependencyProfilesByPlugin(ctx, "cdc-oracle")
	if err != nil {
		t.Fatalf("failed to list stored profiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 stored profile, got %d", len(profiles))
	}
	if profiles[0].SourceKind != PluginDependencyProfileSourceRuntimeAnalyzed {
		t.Fatalf("expected runtime_analyzed source kind, got %q", profiles[0].SourceKind)
	}
}

func TestGetOfficialDependenciesMarksDisabledAndEffectiveRemovesThem(t *testing.T) {
	service, repo := newTestPluginService(t)
	ctx := context.Background()
	disableSeedAutoLoad(service, "9.9.9")

	if err := repo.UpsertDependencyProfile(ctx, &PluginDependencyProfile{
		SeatunnelVersion:    "9.9.9",
		PluginName:          "doris",
		ArtifactID:          "connector-doris",
		ProfileKey:          "default",
		EngineScope:         "zeta",
		SourceKind:          PluginDependencyProfileSourceOfficialSeed,
		BaselineVersionUsed: "9.9.9",
		ResolutionMode:      DependencyResolutionModeExact,
		TargetDir:           "plugins/connector-doris",
		Confidence:          "high",
		IsDefault:           true,
		Items: []PluginDependencyProfileItem{{
			GroupID:    "mysql",
			ArtifactID: "mysql-connector-java",
			Version:    "8.0.28",
			TargetDir:  "plugins/connector-doris",
			Required:   true,
		}},
	}); err != nil {
		t.Fatalf("failed to seed profile: %v", err)
	}

	item, err := service.DisableDependency(ctx, &DisableDependencyRequest{
		PluginName:       "doris",
		SeatunnelVersion: "9.9.9",
		GroupID:          "mysql",
		ArtifactID:       "mysql-connector-java",
		Version:          "8.0.28",
		TargetDir:        "plugins/connector-doris",
	})
	if err != nil {
		t.Fatalf("DisableDependency returned error: %v", err)
	}
	if item == nil || item.ID == 0 {
		t.Fatalf("expected disable record to be created")
	}

	result, err := service.GetOfficialDependencies(ctx, "doris", "9.9.9", "")
	if err != nil {
		t.Fatalf("GetOfficialDependencies returned error: %v", err)
	}
	if len(result.DisabledDependencies) != 1 {
		t.Fatalf("expected 1 disabled dependency, got %d", len(result.DisabledDependencies))
	}
	if len(result.EffectiveDependencies) != 0 {
		t.Fatalf("expected disabled official dependency to be removed from effective list, got %+v", result.EffectiveDependencies)
	}
	if len(result.Profiles) != 1 || len(result.Profiles[0].Items) != 1 || !result.Profiles[0].Items[0].Disabled {
		t.Fatalf("expected profile item to be marked disabled: %+v", result.Profiles)
	}
}

func TestUploadDependencyStoresJarAndParticipatesInEffectiveDependencies(t *testing.T) {
	service, _ := newTestPluginService(t)
	ctx := context.Background()

	tempDir := t.TempDir()
	oldPluginsDir := config.Config.Storage.PluginsDir
	config.Config.Storage.PluginsDir = tempDir
	defer func() { config.Config.Storage.PluginsDir = oldPluginsDir }()

	header := createDependencyUploadFileHeader(t, "file", "custom-driver-1.0.0.jar", []byte("custom-jar-data"))
	dep, err := service.UploadDependency(ctx, &UploadDependencyRequest{
		PluginName:       "activemq",
		SeatunnelVersion: "2.3.13",
		GroupID:          "custom.company",
	}, header)
	if err != nil {
		t.Fatalf("UploadDependency returned error: %v", err)
	}
	if dep.SourceType != PluginDependencySourceUpload {
		t.Fatalf("expected upload source type, got %q", dep.SourceType)
	}
	if dep.TargetDir != "plugins/connector-activemq" {
		t.Fatalf("expected isolated target dir, got %q", dep.TargetDir)
	}
	if dep.StoredPath == "" {
		t.Fatalf("expected stored path to be set")
	}
	if _, err := os.Stat(dep.StoredPath); err != nil {
		t.Fatalf("expected stored jar to exist: %v", err)
	}

	effective, err := service.GetEffectiveDependencies(ctx, "activemq", "2.3.13", nil)
	if err != nil {
		t.Fatalf("GetEffectiveDependencies returned error: %v", err)
	}
	if len(effective) != 1 {
		t.Fatalf("expected 1 effective dependency, got %d", len(effective))
	}
	if effective[0].SourceType != PluginDependencySourceUpload {
		t.Fatalf("expected uploaded dependency source, got %q", effective[0].SourceType)
	}
	if effective[0].StoredPath == "" {
		t.Fatalf("expected effective dependency to carry stored path")
	}
	if filepath.Base(dep.StoredPath) == "" {
		t.Fatalf("expected stored file path to be valid")
	}
}

func TestBuildPluginPreparationFingerprint_IsStableAndDetectsChanges(t *testing.T) {
	depsA := []PluginDependency{
		{
			GroupID:    "mysql",
			ArtifactID: "mysql-connector-java",
			Version:    "8.0.27",
			TargetDir:  "plugins/connector-jdbc",
			SourceType: PluginDependencySourceOfficial,
		},
		{
			GroupID:          "uploaded",
			ArtifactID:       "custom-driver",
			Version:          "1.0.0",
			TargetDir:        "plugins/connector-jdbc",
			SourceType:       PluginDependencySourceUpload,
			OriginalFileName: "custom-driver-1.0.0.jar",
			StoredPath:       "/tmp/custom-driver-1.0.0.jar",
		},
	}
	depsB := []PluginDependency{
		depsA[1],
		depsA[0],
	}

	hashA, err := buildPluginPreparationFingerprint(depsA)
	if err != nil {
		t.Fatalf("buildPluginPreparationFingerprint returned error: %v", err)
	}
	hashB, err := buildPluginPreparationFingerprint(depsB)
	if err != nil {
		t.Fatalf("buildPluginPreparationFingerprint returned error: %v", err)
	}
	if hashA != hashB {
		t.Fatalf("expected fingerprint to be order-independent, got %s vs %s", hashA, hashB)
	}

	depsC := append([]PluginDependency(nil), depsA...)
	depsC[1].StoredPath = "/tmp/custom-driver-1.0.1.jar"
	hashC, err := buildPluginPreparationFingerprint(depsC)
	if err != nil {
		t.Fatalf("buildPluginPreparationFingerprint returned error: %v", err)
	}
	if hashC == hashA {
		t.Fatalf("expected fingerprint to change when uploaded dependency payload changes")
	}
}
