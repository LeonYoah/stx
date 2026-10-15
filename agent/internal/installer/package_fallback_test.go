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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecuteStepDownloadAgentLocalFallback 验证无传输信息时使用 Agent 本地安装包，且不访问网络。
// TestExecuteStepDownloadAgentLocalFallback verifies the Agent-local fallback (no network access).
func TestExecuteStepDownloadAgentLocalFallback(t *testing.T) {
	const version = "2.3.13"
	fileName := "apache-seatunnel-" + version + "-bin.tar.gz"

	t.Run("package_dir", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, fileName)
		if err := os.WriteFile(pkg, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := NewInstallerManager()
		m.SetLocalPackageDir(dir)
		params := &InstallParams{Version: version, Mode: InstallModeOnline, Mirror: MirrorAliyun}
		if err := m.executeStepDownload(context.Background(), params, &NoOpProgressReporter{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if params.PackagePath != pkg {
			t.Fatalf("PackagePath=%s want %s", params.PackagePath, pkg)
		}
	})

	t.Run("agent_package_path_dir_and_file", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, fileName)
		if err := os.WriteFile(pkg, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := NewInstallerManager()
		m.SetLocalPackageDir(t.TempDir()) // empty dir / 空目录
		for _, p := range []string{dir, pkg} {
			params := &InstallParams{Version: version, PackageFileName: fileName, AgentLocalPackagePath: p}
			if err := m.executeStepDownload(context.Background(), params, &NoOpProgressReporter{}); err != nil || params.PackagePath != pkg {
				t.Fatalf("path=%s: err=%v PackagePath=%s", p, err, params.PackagePath)
			}
		}
	})

	t.Run("missing_reports_paths", func(t *testing.T) {
		dir := t.TempDir()
		m := NewInstallerManager()
		m.SetLocalPackageDir(dir)
		params := &InstallParams{Version: version, PackagePath: "/nonexistent/transferred.tar.gz"}
		err := m.executeStepDownload(context.Background(), params, &NoOpProgressReporter{})
		if !errors.Is(err, ErrPackageNotFound) {
			t.Fatalf("expected ErrPackageNotFound, got %v", err)
		}
		want := filepath.Join(dir, fileName)
		for _, s := range []string{want, "/nonexistent/transferred.tar.gz", "Agent 本地未找到安装包", "no local package found on Agent"} {
			if !strings.Contains(err.Error(), s) {
				t.Fatalf("error %q missing %q", err.Error(), s)
			}
		}
	})
}
