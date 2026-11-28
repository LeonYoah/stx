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

package installer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigureIMAPStorageKeepsHazelcastTopLevelSections(t *testing.T) {
	installDir := t.TempDir()
	configDir := filepath.Join(installDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	original := `hazelcast:
  cluster-name: seatunnel
  network:
    join:
      tcp-ip:
        enabled: true
        member-list:
          - localhost
  properties:
    hazelcast.logging.type: log4j2
`
	if err := os.WriteFile(filepath.Join(configDir, "hazelcast.yaml"), []byte(original), 0o644); err != nil {
		t.Fatalf("failed to write hazelcast.yaml: %v", err)
	}

	manager := NewInstallerManager()
	params := &InstallParams{
		InstallDir:     installDir,
		DeploymentMode: DeploymentModeHybrid,
		IMAP: &IMAPConfig{
			StorageType:      IMAPStorageS3,
			Namespace:        "/seatunnel/imap/",
			StorageEndpoint:  "http://127.0.0.1:19000",
			StorageBucket:    "s3a://seatunnel-imap",
			StorageAccessKey: "minioadmin",
			StorageSecretKey: "minioadmin",
		},
	}

	if err := manager.configureIMAPStorage(params); err != nil {
		t.Fatalf("configureIMAPStorage returned error: %v", err)
	}

	outputBytes, err := os.ReadFile(filepath.Join(configDir, "hazelcast.yaml"))
	if err != nil {
		t.Fatalf("failed to read output hazelcast.yaml: %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(outputBytes, &parsed); err != nil {
		t.Fatalf("failed to parse output yaml: %v", err)
	}
	hazelcast := mustMap(t, parsed["hazelcast"])
	if hazelcast["cluster-name"] != "seatunnel" {
		t.Fatalf("expected cluster-name under hazelcast root, got %#v", hazelcast["cluster-name"])
	}
	if _, ok := hazelcast["network"]; !ok {
		t.Fatalf("expected network under hazelcast root, got %#v", hazelcast)
	}
	if _, ok := hazelcast["properties"]; !ok {
		t.Fatalf("expected properties under hazelcast root, got %#v", hazelcast)
	}
	mapNode := mustMap(t, hazelcast["map"])
	engineNode := mustMap(t, mapNode["engine*"])
	mapStore := mustMap(t, engineNode["map-store"])
	if mapStore["enabled"] != true {
		t.Fatalf("expected map-store enabled=true, got %#v", mapStore["enabled"])
	}
	monitorNode := mustMap(t, mapNode["engine_checkpoint_monitor"])
	monitorMapStore := mustMap(t, monitorNode["map-store"])
	if monitorMapStore["enabled"] != false {
		t.Fatalf("expected engine_checkpoint_monitor map-store enabled=false, got %#v", monitorMapStore["enabled"])
	}
}

func TestConfigureIMAPStorageNormalizesMalformedHazelcastMapLayout(t *testing.T) {
	installDir := t.TempDir()
	configDir := filepath.Join(installDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	malformed := `hazelcast:
  map:
    engine*:
      map-store:
        enabled: true
    engine_checkpoint_monitor:
      map-store:
        enabled: false
    cluster-name: seatunnel
    network:
      join:
        tcp-ip:
          enabled: true
          member-list:
            - localhost
    properties:
      hazelcast.logging.type: log4j2
`
	if err := os.WriteFile(filepath.Join(configDir, "hazelcast.yaml"), []byte(malformed), 0o644); err != nil {
		t.Fatalf("failed to write malformed hazelcast.yaml: %v", err)
	}

	manager := NewInstallerManager()
	params := &InstallParams{
		InstallDir:     installDir,
		DeploymentMode: DeploymentModeHybrid,
		IMAP: &IMAPConfig{
			StorageType: IMAPStorageDisabled,
		},
	}

	if err := manager.configureIMAPStorage(params); err != nil {
		t.Fatalf("configureIMAPStorage returned error: %v", err)
	}

	outputBytes, err := os.ReadFile(filepath.Join(configDir, "hazelcast.yaml"))
	if err != nil {
		t.Fatalf("failed to read output hazelcast.yaml: %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(outputBytes, &parsed); err != nil {
		t.Fatalf("failed to parse output yaml: %v", err)
	}
	hazelcast := mustMap(t, parsed["hazelcast"])
	mapNode := mustMap(t, hazelcast["map"])
	if _, ok := mapNode["cluster-name"]; ok {
		t.Fatalf("expected cluster-name to be moved out of hazelcast.map, got %#v", mapNode)
	}
	if hazelcast["cluster-name"] != "seatunnel" {
		t.Fatalf("expected cluster-name under hazelcast root, got %#v", hazelcast["cluster-name"])
	}
	mapStore := mustMap(t, mustMap(t, mapNode["engine*"])["map-store"])
	if mapStore["enabled"] != false {
		t.Fatalf("expected map-store enabled=false, got %#v", mapStore["enabled"])
	}
	if _, exists := mapNode["engine_checkpoint_monitor"]; exists {
		t.Fatalf("expected engine_checkpoint_monitor override to be removed when IMAP is disabled, got %#v", mapNode)
	}
}

func TestConfigureIMAPStorageSeparatedOnlyUpdatesMasterConfig(t *testing.T) {
	installDir := t.TempDir()
	configDir := filepath.Join(installDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	masterOriginal := `hazelcast:
  cluster-name: seatunnel
`
	workerOriginal := `hazelcast:
  cluster-name: worker-only
`
	if err := os.WriteFile(filepath.Join(configDir, "hazelcast-master.yaml"), []byte(masterOriginal), 0o644); err != nil {
		t.Fatalf("failed to write hazelcast-master.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "hazelcast-worker.yaml"), []byte(workerOriginal), 0o644); err != nil {
		t.Fatalf("failed to write hazelcast-worker.yaml: %v", err)
	}

	manager := NewInstallerManager()
	params := &InstallParams{
		InstallDir:     installDir,
		DeploymentMode: DeploymentModeSeparated,
		IMAP: &IMAPConfig{
			StorageType:      IMAPStorageS3,
			Namespace:        "/seatunnel/imap/",
			StorageEndpoint:  "http://127.0.0.1:19000",
			StorageBucket:    "s3a://seatunnel-imap",
			StorageAccessKey: "minioadmin",
			StorageSecretKey: "minioadmin",
		},
	}

	if err := manager.configureIMAPStorage(params); err != nil {
		t.Fatalf("configureIMAPStorage returned error: %v", err)
	}

	masterBytes, err := os.ReadFile(filepath.Join(configDir, "hazelcast-master.yaml"))
	if err != nil {
		t.Fatalf("failed to read output hazelcast-master.yaml: %v", err)
	}
	masterParsed := map[string]any{}
	if err := yaml.Unmarshal(masterBytes, &masterParsed); err != nil {
		t.Fatalf("failed to parse output master yaml: %v", err)
	}
	masterHazelcast := mustMap(t, masterParsed["hazelcast"])
	masterMapNode := mustMap(t, masterHazelcast["map"])
	masterMapStore := mustMap(t, mustMap(t, masterMapNode["engine*"])["map-store"])
	if masterMapStore["enabled"] != true {
		t.Fatalf("expected master map-store enabled=true, got %#v", masterMapStore["enabled"])
	}
	masterMonitorMapStore := mustMap(t, mustMap(t, masterMapNode["engine_checkpoint_monitor"])["map-store"])
	if masterMonitorMapStore["enabled"] != false {
		t.Fatalf("expected master engine_checkpoint_monitor map-store enabled=false, got %#v", masterMonitorMapStore["enabled"])
	}

	workerBytes, err := os.ReadFile(filepath.Join(configDir, "hazelcast-worker.yaml"))
	if err != nil {
		t.Fatalf("failed to read output hazelcast-worker.yaml: %v", err)
	}
	if string(workerBytes) != workerOriginal {
		t.Fatalf("expected worker hazelcast config to remain unchanged, got %s", string(workerBytes))
	}
}

func TestEnsureOSSLibraryDependenciesDownloadsMissingJars(t *testing.T) {
	installDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "jar:%s", filepath.Base(r.URL.Path))
	}))
	defer server.Close()

	originalMirrors := make(map[MirrorSource]string, len(mavenRepoBaseURLs))
	for key, value := range mavenRepoBaseURLs {
		originalMirrors[key] = value
	}
	for key := range mavenRepoBaseURLs {
		mavenRepoBaseURLs[key] = server.URL
	}
	t.Cleanup(func() {
		for key, value := range originalMirrors {
			mavenRepoBaseURLs[key] = value
		}
	})

	manager := NewInstallerManager()
	if err := manager.ensureOSSLibraryDependencies(context.Background(), installDir, MirrorAliyun); err != nil {
		t.Fatalf("ensureOSSLibraryDependencies returned error: %v", err)
	}

	for _, dep := range ossRuntimeDependencySpecs {
		targetPath := filepath.Join(installDir, "lib", dep.FileName())
		content, err := os.ReadFile(targetPath)
		if err != nil {
			t.Fatalf("failed to read dependency %s: %v", dep.FileName(), err)
		}
		if len(content) == 0 {
			t.Fatalf("expected dependency %s to be downloaded", dep.FileName())
		}
	}
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	typed, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %#v", value)
	}
	return typed
}
