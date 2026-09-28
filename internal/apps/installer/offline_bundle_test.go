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
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndImportOfflineBundle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	packageDir := filepath.Join(root, "packages")
	pluginsDir := filepath.Join(root, "plugins")
	tempDir := filepath.Join(root, "temp")
	_ = os.MkdirAll(filepath.Join(pluginsDir, "9.9.91", "connectors"), 0o755)
	_ = os.MkdirAll(packageDir, 0o755)
	_ = os.MkdirAll(tempDir, 0o755)

	if err := os.WriteFile(filepath.Join(packageDir, "apache-seatunnel-9.9.91-bin.tar.gz"), []byte("fake-seatunnel-bin"), 0o644); err != nil {
		t.Fatalf("write package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "9.9.91", "connectors", "connector-fake-9.9.91.jar"), []byte("fake-plugin"), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	svc := NewService(packageDir, nil)
	svc.tempDir = tempDir

	include := true
	info, err := svc.CreateOfflineBundle(t.Context(), &OfflineBundleCreateRequest{
		Version:        "9.9.91",
		IncludePlugins: &include,
	})
	if err != nil {
		t.Fatalf("CreateOfflineBundle: %v", err)
	}
	if info.SeatunnelVersion != "9.9.91" {
		t.Fatalf("unexpected version: %s", info.SeatunnelVersion)
	}
	if info.PluginCount < 1 {
		t.Fatalf("expected plugins in bundle, got %d", info.PluginCount)
	}

	path, err := svc.ResolveOfflineBundlePath(info.FileName)
	if err != nil {
		t.Fatalf("ResolveOfflineBundlePath: %v", err)
	}

	importRoot := filepath.Join(root, "import-site")
	importPackages := filepath.Join(importRoot, "packages")
	_ = os.MkdirAll(importPackages, 0o755)
	_ = os.MkdirAll(filepath.Join(importRoot, "plugins"), 0o755)
	importSvc := NewService(importPackages, nil)
	importSvc.tempDir = tempDir

	imported, err := importSvc.ImportOfflineBundle(t.Context(), path)
	if err != nil {
		t.Fatalf("ImportOfflineBundle: %v", err)
	}
	if imported.SeatunnelVersion != "9.9.91" {
		t.Fatalf("import version: %s", imported.SeatunnelVersion)
	}
	if _, err := os.Stat(filepath.Join(importPackages, "apache-seatunnel-9.9.91-bin.tar.gz")); err != nil {
		t.Fatalf("imported package missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(importRoot, "plugins", "9.9.91", "connectors", "connector-fake-9.9.91.jar")); err != nil {
		t.Fatalf("imported plugin missing: %v", err)
	}
}

func TestCreateOfflineBundleRequiresLocalPackage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := NewService(filepath.Join(root, "packages"), nil)
	svc.tempDir = filepath.Join(root, "temp")
	_ = os.MkdirAll(svc.tempDir, 0o755)
	_, err := svc.CreateOfflineBundle(t.Context(), &OfflineBundleCreateRequest{Version: "1.2.3"})
	if !errors.Is(err, ErrOfflineBundlePackageMissing) {
		t.Fatalf("expected ErrOfflineBundlePackageMissing, got %v", err)
	}
}
