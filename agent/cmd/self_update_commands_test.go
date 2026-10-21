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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/installer"
	"github.com/LeonYoah/stx/agent/internal/selfupdate"
)

type selfUpdateHooks struct {
	applied    *selfupdate.ApplyRequest
	applyErr   error
	restarted  bool
	restartErr error
	restored   bool
	rolledBack bool
	hasBackup  bool
	scheduled  []func()
}

func installSelfUpdateHooks(t *testing.T) *selfUpdateHooks {
	t.Helper()
	h := &selfUpdateHooks{hasBackup: true}
	oApply, oRestart, oRollback, oRestore, oHas, oSched, oDirs := selfUpdateApply, selfUpdateRestart, selfUpdateRollback, selfUpdateRestore, selfUpdateHasBak, scheduleAfter, proxyLibDirs
	oProxyRestart, oRunningJars := proxyRestartFn, proxyRunningJarsFn
	proxyRunningJarsFn = func(context.Context, []string) []string { return nil }
	proxyRestartFn = func(context.Context, []string, func() error) []installer.ProxyRestartResult {
		t.Fatal("proxy restart must only run when restart_running=true")
		return nil
	}
	selfUpdateApply = func(ctx context.Context, req selfupdate.ApplyRequest, cur string) (*selfupdate.ApplyResult, error) {
		h.applied = &req
		if h.applyErr != nil {
			return nil, h.applyErr
		}
		return &selfupdate.ApplyResult{Executable: "/opt/stx-agent", Backup: "/opt/stx-agent.bak", FromVersion: cur, ToVersion: req.TargetVersion}, nil
	}
	selfUpdateRestart = func(res *selfupdate.ApplyResult) error { h.restarted = true; return h.restartErr }
	selfUpdateRollback = func(string) error { h.rolledBack = true; return nil }
	selfUpdateRestore = func(string) (string, error) { h.restored = true; return "", nil }
	selfUpdateHasBak = func() bool { return h.hasBackup }
	scheduleAfter = func(d time.Duration, fn func()) { h.scheduled = append(h.scheduled, fn) }
	t.Cleanup(func() {
		selfUpdateApply, selfUpdateRestart, selfUpdateRollback, selfUpdateRestore, selfUpdateHasBak, scheduleAfter, proxyLibDirs = oApply, oRestart, oRollback, oRestore, oHas, oSched, oDirs
		proxyRestartFn, proxyRunningJarsFn = oProxyRestart, oRunningJars
	})
	return h
}

func upgradeCmd(sub string, params map[string]string) *pb.CommandRequest {
	p := map[string]string{"sub_command": sub}
	for k, v := range params {
		p[k] = v
	}
	return &pb.CommandRequest{CommandId: "c1", Type: pb.CommandType_UPGRADE, Parameters: p}
}

func TestAgentSelfUpdateSchedulesRestartAfterReply(t *testing.T) {
	h := installSelfUpdateHooks(t)
	a := &Agent{}
	resp, err := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentSelfUpdate, map[string]string{
		"package_path": "/var/pkg/stx-agent-linux-amd64", "checksum": "abc", "target_version": "v9.9.9",
	}), nil)
	if err != nil || resp.Status != pb.CommandStatus_SUCCESS {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if h.applied == nil || h.applied.TargetVersion != "9.9.9" || h.applied.Checksum != "abc" {
		t.Fatalf("apply request = %+v", h.applied)
	}
	var out selfupdate.ApplyResult
	if err := json.Unmarshal([]byte(resp.Output), &out); err != nil || out.ToVersion != "9.9.9" {
		t.Fatalf("output = %s", resp.Output)
	}
	// 重启必须在回复之后（延迟调度），而不是在处理函数内同步执行。
	if h.restarted || len(h.scheduled) != 1 {
		t.Fatalf("restart must be deferred; restarted=%v scheduled=%d", h.restarted, len(h.scheduled))
	}
	// 并发的第二次自升级被拒绝。
	if resp, _ := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentSelfUpdate, map[string]string{"target_version": "9.9.9"}), nil); resp.Status != pb.CommandStatus_FAILED {
		t.Fatalf("concurrent self-update should be rejected")
	}
	h.scheduled[0]()
	if !h.restarted || h.restored {
		t.Fatalf("restarted=%v restored=%v", h.restarted, h.restored)
	}
}

func TestAgentSelfUpdateExecFailureRestoresBackup(t *testing.T) {
	h := installSelfUpdateHooks(t)
	h.restartErr = errors.New("exec failed")
	a := &Agent{}
	if _, err := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentSelfUpdate, map[string]string{"target_version": "9.9.9"}), nil); err != nil {
		t.Fatal(err)
	}
	h.scheduled[0]()
	if !h.restored {
		t.Fatal("exec failure must restore the backup binary")
	}
	// 锁已释放，可再次升级 / lock released so another attempt is possible
	if !a.selfUpdateMu.TryLock() {
		t.Fatal("self-update lock not released")
	}
}

func TestAgentSelfUpdateApplyErrorIsReported(t *testing.T) {
	h := installSelfUpdateHooks(t)
	h.applyErr = errors.New("checksum mismatch")
	a := &Agent{}
	resp, err := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentSelfUpdate, map[string]string{"target_version": "9.9.9"}), nil)
	if err == nil || resp.Status != pb.CommandStatus_FAILED || !strings.Contains(resp.Error, "checksum mismatch") {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if len(h.scheduled) != 0 {
		t.Fatal("no restart may be scheduled after a failed apply")
	}
}

func TestAgentSelfUpdateAlreadyCurrent(t *testing.T) {
	h := installSelfUpdateHooks(t)
	a := &Agent{}
	resp, err := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentSelfUpdate, map[string]string{"target_version": currentVersionForTest()}), nil)
	if err != nil || resp.Status != pb.CommandStatus_SUCCESS || !strings.Contains(resp.Output, "already_current") {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if h.applied != nil || len(h.scheduled) != 0 {
		t.Fatal("already-current agent must not replace or restart")
	}
}

func TestAgentSelfRollback(t *testing.T) {
	h := installSelfUpdateHooks(t)
	a := &Agent{}
	h.hasBackup = false
	if resp, _ := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentSelfRollback, nil), nil); resp.Status != pb.CommandStatus_FAILED {
		t.Fatal("rollback without backup must fail")
	}
	h.hasBackup = true
	resp, err := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentSelfRollback, nil), nil)
	if err != nil || resp.Status != pb.CommandStatus_SUCCESS || len(h.scheduled) != 1 {
		t.Fatalf("resp=%+v err=%v scheduled=%d", resp, err, len(h.scheduled))
	}
	h.scheduled[0]()
	if !h.rolledBack {
		t.Fatal("rollback not executed")
	}
}

func TestAgentUpdateProxyJars(t *testing.T) {
	installSelfUpdateHooks(t)
	a := &Agent{}
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "stx-java-proxy-v3.jar")
	_ = os.WriteFile(src, []byte("new"), 0o644)

	// 未安装 proxy：跳过 / no proxy installed: skipped
	empty := t.TempDir()
	proxyLibDirs = func() []string { return []string{empty} }
	resp, err := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentUpdateProxyJars, map[string]string{"jar_paths": src}), nil)
	if err != nil || !strings.Contains(resp.Output, `"skipped":true`) {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}

	installed := t.TempDir()
	_ = os.WriteFile(filepath.Join(installed, "stx-java-proxy-v3.jar"), []byte("old"), 0o644)
	proxyLibDirs = func() []string { return []string{empty, installed} }
	resp, err = a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentUpdateProxyJars, map[string]string{"jar_paths": src}), nil)
	if err != nil || resp.Status != pb.CommandStatus_SUCCESS {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if b, _ := os.ReadFile(filepath.Join(installed, "stx-java-proxy-v3.jar")); string(b) != "new" {
		t.Fatalf("jar not updated: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(installed, "stx-java-proxy-v3.jar.bak")); string(b) != "old" {
		t.Fatalf("backup missing: %q", b)
	}

	bad := filepath.Join(srcDir, "evil.sh")
	_ = os.WriteFile(bad, []byte("x"), 0o644)
	if resp, _ := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentUpdateProxyJars, map[string]string{"jar_paths": bad}), nil); resp.Status != pb.CommandStatus_FAILED {
		t.Fatal("non proxy jar must be rejected")
	}
}

// 更新 jar 并重启运行中的 proxy：运行中 proxy 实际加载的 jar 也会被替换；重启失败时的回滚回调能恢复原 jar。
// Updating jars with restart_running also replaces the jar a running proxy loads, and the rollback callback handed
// to the restart restores the original jars.
func TestAgentUpdateProxyJarsRestartsRunningProxy(t *testing.T) {
	installSelfUpdateHooks(t)
	a := &Agent{}
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "stx-java-proxy-v2.jar")
	_ = os.WriteFile(src, []byte("new"), 0o644)
	installed := t.TempDir()
	defaultJar := filepath.Join(installed, "stx-java-proxy-v2.jar")
	_ = os.WriteFile(defaultJar, []byte("old"), 0o644)
	runningDir := t.TempDir()
	runningJar := filepath.Join(runningDir, "stx-java-proxy-v2.jar")
	_ = os.WriteFile(runningJar, []byte("old-running"), 0o644)
	proxyLibDirs = func() []string { return []string{installed} }
	proxyRunningJarsFn = func(context.Context, []string) []string { return []string{runningJar} }
	var gotDirs []string
	proxyRestartFn = func(ctx context.Context, dirs []string, rollback func() error) []installer.ProxyRestartResult {
		gotDirs = dirs
		for _, p := range []string{defaultJar, runningJar} {
			if b, _ := os.ReadFile(p); string(b) != "new" {
				t.Fatalf("%s must hold the new jar before the restart, got %q", p, b)
			}
		}
		if err := rollback(); err != nil {
			t.Fatalf("rollback: %v", err)
		}
		return []installer.ProxyRestartResult{{Status: installer.ProxyRestartRolledBack, OldPID: 10, NewPID: 11}}
	}
	resp, err := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentUpdateProxyJars, map[string]string{
		"jar_paths": src, "restart_running": "true", "install_dirs": "/opt/st-a,/opt/st-b",
	}), nil)
	if err != nil || resp.Status != pb.CommandStatus_SUCCESS {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if !strings.Contains(resp.Output, `"status":"rolled_back"`) {
		t.Fatalf("restart result missing from output: %s", resp.Output)
	}
	if strings.Join(gotDirs, ",") != "/opt/st-a,/opt/st-b" {
		t.Fatalf("install dirs = %v", gotDirs)
	}
	if b, _ := os.ReadFile(defaultJar); string(b) != "old" {
		t.Fatalf("rollback must restore the default jar, got %q", b)
	}
	if b, _ := os.ReadFile(runningJar); string(b) != "old-running" {
		t.Fatalf("rollback must restore the running jar, got %q", b)
	}
}

// 回滚 proxy jar：用 .bak 恢复并重启运行中的 proxy；没有备份时跳过且不重启。
// Rolling back proxy jars restores .bak and restarts running proxies; without backups it is skipped with no restart.
func TestAgentRollbackProxyJars(t *testing.T) {
	installSelfUpdateHooks(t)
	a := &Agent{}
	installed := t.TempDir()
	jar := filepath.Join(installed, "stx-java-proxy-v3.jar")
	_ = os.WriteFile(jar, []byte("new"), 0o644)
	proxyLibDirs = func() []string { return []string{installed} }
	resp, err := a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentRollbackProxyJars, nil), nil)
	if err != nil || !strings.Contains(resp.Output, `"skipped":true`) {
		t.Fatalf("without backups the rollback must be skipped: resp=%+v err=%v", resp, err)
	}
	_ = os.WriteFile(jar+".bak", []byte("old"), 0o644)
	restarted := false
	proxyRestartFn = func(ctx context.Context, dirs []string, rollback func() error) []installer.ProxyRestartResult {
		restarted = true
		if b, _ := os.ReadFile(jar); string(b) != "old" {
			t.Fatalf("jar must be restored before restarting, got %q", b)
		}
		return []installer.ProxyRestartResult{{Status: installer.ProxyRestartRestarted}}
	}
	resp, err = a.handleManagedUpgradeCommand(context.Background(), upgradeCmd(subCommandAgentRollbackProxyJars, nil), nil)
	if err != nil || resp.Status != pb.CommandStatus_SUCCESS || !restarted {
		t.Fatalf("resp=%+v err=%v restarted=%v", resp, err, restarted)
	}
	if !strings.Contains(resp.Output, jar) {
		t.Fatalf("restored jar missing from output: %s", resp.Output)
	}
}

func TestMarkRegisteredIsIdempotent(t *testing.T) {
	a := &Agent{registeredCh: make(chan struct{})}
	a.markRegistered()
	a.markRegistered()
	select {
	case <-a.registeredCh:
	default:
		t.Fatal("registeredCh should be closed")
	}
	(&Agent{}).markRegistered() // nil channel must not panic
}

func currentVersionForTest() string { return selfupdate.NormalizeVersion(stxversionVersion()) }
