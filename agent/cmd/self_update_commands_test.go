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
