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

package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeEnv struct {
	exe      string
	execPath string
	execEnv  []string
	execErr  error
	probeVer string
	probeErr error
}

func setup(t *testing.T) *fakeEnv {
	t.Helper()
	dir := t.TempDir()
	f := &fakeEnv{exe: filepath.Join(dir, "stx-agent"), probeVer: "1.0.1"}
	if err := os.WriteFile(f.exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	origExe, origExec, origProbe := executablePath, execFn, versionProbe
	executablePath = func() (string, error) { return f.exe, nil }
	execFn = func(argv0 string, argv []string, envv []string) error {
		f.execPath, f.execEnv = argv0, envv
		return f.execErr
	}
	versionProbe = func(ctx context.Context, binary string) (string, error) { return f.probeVer, f.probeErr }
	t.Cleanup(func() {
		executablePath, execFn, versionProbe = origExe, origExec, origProbe
		_ = os.Unsetenv(EnvBackup)
		_ = os.Unsetenv(EnvFromVersion)
		_ = os.Unsetenv(EnvTrialTimeout)
	})
	return f
}

func writePkg(t *testing.T, content string) (string, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "stx-agent-linux-amd64")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return p, hex.EncodeToString(sum[:])
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyReplacesBinaryAndKeepsBackup(t *testing.T) {
	f := setup(t)
	pkg, sum := writePkg(t, "new-binary")
	res, err := Apply(context.Background(), ApplyRequest{PackagePath: pkg, Checksum: sum, TargetVersion: "v1.0.1"}, "1.0.0")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := readFile(t, f.exe); got != "new-binary" {
		t.Fatalf("exe = %q", got)
	}
	if got := readFile(t, res.Backup); got != "old-binary" {
		t.Fatalf("backup = %q", got)
	}
	if res.FromVersion != "1.0.0" || res.ToVersion != "1.0.1" {
		t.Fatalf("unexpected result %+v", res)
	}
	if _, err := os.Stat(f.exe + stagingSuffix); !os.IsNotExist(err) {
		t.Fatalf("staging file left behind")
	}
	if fi, _ := os.Stat(f.exe); fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("new binary not executable: %v", fi.Mode())
	}

	if err := Restart(res); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if f.execPath != f.exe || !containsEnv(f.execEnv, EnvBackup+"="+res.Backup) {
		t.Fatalf("restart exec = %s env has backup=%v", f.execPath, containsEnv(f.execEnv, EnvBackup+"="+res.Backup))
	}
}

func TestApplyRejectsBadInputWithoutTouchingBinary(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(f *fakeEnv, req *ApplyRequest)
		want   string
	}{
		{"checksum mismatch", func(f *fakeEnv, req *ApplyRequest) { req.Checksum = strings.Repeat("0", 64) }, "checksum mismatch"},
		{"missing checksum", func(f *fakeEnv, req *ApplyRequest) { req.Checksum = "" }, "checksum is required"},
		{"version mismatch", func(f *fakeEnv, req *ApplyRequest) { f.probeVer = "0.9.0" }, "expected 1.0.1"},
		{"not runnable", func(f *fakeEnv, req *ApplyRequest) { f.probeErr = errors.New("exec format error") }, "exec format error"},
		{"missing package", func(f *fakeEnv, req *ApplyRequest) { req.PackagePath = "/nonexistent/agent" }, "read new agent binary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			pkg, sum := writePkg(t, "new-binary")
			req := ApplyRequest{PackagePath: pkg, Checksum: sum, TargetVersion: "1.0.1"}
			tc.mutate(f, &req)
			_, err := Apply(context.Background(), req, "1.0.0")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if got := readFile(t, f.exe); got != "old-binary" {
				t.Fatalf("binary modified: %q", got)
			}
			if _, err := os.Stat(f.exe + backupSuffix); !os.IsNotExist(err) {
				t.Fatalf("backup should not be created on failure")
			}
			if _, err := os.Stat(f.exe + stagingSuffix); !os.IsNotExist(err) {
				t.Fatalf("staging file left behind")
			}
		})
	}
}

func TestRollbackRestoresBackupAndDropsTrialEnv(t *testing.T) {
	f := setup(t)
	pkg, sum := writePkg(t, "new-binary")
	res, err := Apply(context.Background(), ApplyRequest{PackagePath: pkg, Checksum: sum, TargetVersion: "1.0.1"}, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvBackup, res.Backup)
	if err := RollbackAndRestart(""); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := readFile(t, f.exe); got != "old-binary" {
		t.Fatalf("exe after rollback = %q", got)
	}
	if containsEnvKey(f.execEnv, EnvBackup) {
		t.Fatalf("rolled-back process must not inherit trial env")
	}
}

func TestRollbackWithoutBackupFails(t *testing.T) {
	setup(t)
	if err := RollbackAndRestart(""); !errors.Is(err, ErrNoBackup) {
		t.Fatalf("err = %v, want ErrNoBackup", err)
	}
	if HasBackup() {
		t.Fatalf("HasBackup should be false")
	}
}

func TestTrialWatch(t *testing.T) {
	t.Run("confirmed clears env", func(t *testing.T) {
		f := setup(t)
		t.Setenv(EnvBackup, f.exe+backupSuffix)
		tr := TrialFromEnv()
		if tr == nil || tr.Timeout != DefaultTrialTimeout {
			t.Fatalf("trial = %+v", tr)
		}
		confirmed := make(chan struct{})
		close(confirmed)
		if !tr.Watch(confirmed, nil, nil) {
			t.Fatal("expected confirmed")
		}
		if os.Getenv(EnvBackup) != "" {
			t.Fatal("trial env should be cleared after confirm")
		}
		if f.execPath != "" {
			t.Fatal("must not exec on confirm")
		}
	})
	t.Run("timeout rolls back", func(t *testing.T) {
		f := setup(t)
		if err := os.WriteFile(f.exe+backupSuffix, []byte("old-good"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvBackup, f.exe+backupSuffix)
		t.Setenv(EnvTrialTimeout, "20ms")
		tr := TrialFromEnv()
		var reason string
		start := time.Now()
		if tr.Watch(make(chan struct{}), nil, func(r string, err error) { reason = r }) {
			t.Fatal("expected rollback")
		}
		if time.Since(start) > 2*time.Second || !strings.Contains(reason, "did not register") {
			t.Fatalf("reason = %q", reason)
		}
		if readFile(t, f.exe) != "old-good" || f.execPath != f.exe {
			t.Fatalf("rollback not executed")
		}
	})
	t.Run("start failure rolls back", func(t *testing.T) {
		f := setup(t)
		if err := os.WriteFile(f.exe+backupSuffix, []byte("old-good"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvBackup, f.exe+backupSuffix)
		failed := make(chan struct{})
		close(failed)
		var reason string
		TrialFromEnv().Watch(make(chan struct{}), failed, func(r string, err error) { reason = r })
		if !strings.Contains(reason, "failed to start") || readFile(t, f.exe) != "old-good" {
			t.Fatalf("reason=%q exe=%q", reason, readFile(t, f.exe))
		}
	})
	t.Run("no env means no trial", func(t *testing.T) {
		setup(t)
		if TrialFromEnv() != nil {
			t.Fatal("expected nil trial")
		}
	})
}

func TestReplaceFileWithBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jar")
	dst := filepath.Join(dir, "dst.jar")
	_ = os.WriteFile(src, []byte("v2"), 0o644)
	backed, err := ReplaceFileWithBackup(src, dst, 0o644)
	if err != nil || backed {
		t.Fatalf("first replace: backed=%v err=%v", backed, err)
	}
	_ = os.WriteFile(src, []byte("v3"), 0o644)
	backed, err = ReplaceFileWithBackup(src, dst, 0o644)
	if err != nil || !backed {
		t.Fatalf("second replace: backed=%v err=%v", backed, err)
	}
	if readFile(t, dst) != "v3" || readFile(t, dst+backupSuffix) != "v2" {
		t.Fatalf("dst=%q bak=%q", readFile(t, dst), readFile(t, dst+backupSuffix))
	}
}

func TestDefaultVersionProbeParsesOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-agent")
	body := "#!/bin/sh\necho 'STX Agent'\necho '  Version:    1.2.3'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := defaultVersionProbe(context.Background(), script)
	if err != nil || got != "1.2.3" {
		t.Fatalf("probe = %q, %v", got, err)
	}
	bad := filepath.Join(dir, "bad-agent")
	_ = os.WriteFile(bad, []byte("not an executable"), 0o755)
	if _, err := defaultVersionProbe(context.Background(), bad); err == nil {
		t.Fatal("expected error for non-executable binary")
	}
}

func containsEnv(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func containsEnvKey(env []string, key string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return true
		}
	}
	return false
}
