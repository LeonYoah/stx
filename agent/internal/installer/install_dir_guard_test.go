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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveManagedInstallDirRejectsUnmanagedDir(t *testing.T) {
	targetDir := t.TempDir()

	_, err := RemoveManagedInstallDir(targetDir)
	if err == nil || !strings.Contains(err.Error(), "please remove it manually") {
		t.Fatalf("expected managed install validation error, got %v", err)
	}
	if _, statErr := os.Stat(targetDir); statErr != nil {
		t.Fatalf("expected unmanaged dir to remain, got %v", statErr)
	}
}

func TestRemoveManagedInstallDirRejectsLegacyLookingDirWithoutMarker(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "legacy-install")
	mustWriteFile(t, filepath.Join(targetDir, "bin", "seatunnel-cluster.sh"), "echo start")
	mustWriteFile(t, filepath.Join(targetDir, "bin", "stop-seatunnel-cluster.sh"), "echo stop")

	_, err := RemoveManagedInstallDir(targetDir)
	if err == nil || !strings.Contains(err.Error(), "please remove it manually") {
		t.Fatalf("expected manual removal guidance, got %v", err)
	}
	if _, statErr := os.Stat(targetDir); statErr != nil {
		t.Fatalf("expected legacy-looking dir to remain, got %v", statErr)
	}
}

func TestInstallerManagerUninstallAllowsMarkerManagedDir(t *testing.T) {
	manager := NewInstallerManager()
	t.Setenv(stxJavaProxyHomeEnvVar, t.TempDir())
	targetDir := filepath.Join(t.TempDir(), "managed-install")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	if err := WriteManagedInstallMarker(targetDir); err != nil {
		t.Fatalf("WriteManagedInstallMarker returned error: %v", err)
	}

	if err := manager.Uninstall(context.Background(), targetDir); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	if _, statErr := os.Stat(targetDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected marker-managed dir removed, stat err=%v", statErr)
	}
}
