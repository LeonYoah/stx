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

package layoutheal

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/LeonYoah/stx/install"
	"github.com/stretchr/testify/require"
)

// makeInstallDir 构造一个最小安装目录布局：bin/ 三个旧脚本 + lib/observability.sh + config.yaml。
// makeInstallDir builds a minimal install layout: three legacy bin/ scripts, lib/observability.sh and config.yaml.
func makeInstallDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin", "lib"), 0o755))
	for _, name := range []string{"start.sh", "stop.sh", "status.sh"} {
		// 旧版 start.sh 会 source lib/observability.sh，Heal 以此为旧布局标记。
		// Legacy start.sh sources lib/observability.sh; Heal takes it as the legacy-layout marker.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", name),
			[]byte("#!/bin/bash\n# STX legacy script with observability trio\nsource \"$BASE_DIR/bin/lib/observability.sh\"\n"), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", "lib", "observability.sh"),
		[]byte("#!/bin/bash\n# STX legacy observability helpers\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("port: 17800\n"), 0o644))
	return dir
}

func testOptions(dir string, logs *[]string) Options {
	return Options{
		InstallDir: dir,
		GOOS:       "linux",
		InDocker:   func() bool { return false },
		Getenv:     func(string) string { return "" },
		Now:        func() time.Time { return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC) },
		Logger: func(level, format string, args ...interface{}) {
			if logs != nil {
				*logs = append(*logs, level+" "+format)
			}
		},
	}
}

func embeddedScript(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := fs.ReadFile(install.BinScripts, "bin/"+name)
	require.NoError(t, err)
	return raw
}

func TestHeal_ReplacesBacksUpAndMovesLegacyScript(t *testing.T) {
	dir := makeInstallDir(t)
	var logs []string

	res, err := Heal(context.Background(), testOptions(dir, &logs))
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(dir), res.InstallDir)
	require.ElementsMatch(t, []string{"start.sh", "stop.sh", "status.sh"}, res.Replaced)
	require.Len(t, res.Moved, 1)
	require.NotEmpty(t, res.Backups)

	// 三个脚本都被内嵌版本替换，旧文件带 .bak- 备份且不以 .sh 结尾。
	// All three scripts are replaced by the embedded copies; the old files keep .bak- backups that never end in .sh.
	for _, name := range []string{"start.sh", "stop.sh", "status.sh"} {
		current, err := os.ReadFile(filepath.Join(dir, "bin", name))
		require.NoError(t, err)
		require.Equal(t, embeddedScript(t, name), current)
		fi, err := os.Stat(filepath.Join(dir, "bin", name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o755), fi.Mode().Perm())
	}
	require.NoFileExists(t, filepath.Join(dir, "bin", "lib", "observability.sh"))
	obsBak := filepath.Join(dir, "bin", "lib", "observability.sh.bak-20250601T120000Z")
	require.FileExists(t, obsBak)
	require.False(t, strings.HasSuffix(obsBak, ".sh"))
	require.FileExists(t, filepath.Join(dir, "bin", "start.sh.bak-20250601T120000Z"))

	// 有变更时打一条 INFO。/ One INFO log when something changed.
	require.Len(t, logs, 1)
	require.True(t, strings.HasPrefix(logs[0], "INFO"))
}

func TestHeal_IdempotentOnSecondRun(t *testing.T) {
	dir := makeInstallDir(t)
	_, err := Heal(context.Background(), testOptions(dir, nil))
	require.NoError(t, err)

	var logs []string
	res, err := Heal(context.Background(), testOptions(dir, &logs))
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(dir), res.InstallDir)
	require.Empty(t, res.Replaced)
	require.Empty(t, res.Moved)
	require.Empty(t, res.Backups)
	require.Empty(t, logs)
}

func TestHeal_SkipsAlreadyHealedLayout(t *testing.T) {
	// start.sh 已是新版（不再 source lib/observability.sh）且被管理员改过：布局已修复，
	// 不再覆盖，但仍返回 InstallDir 供收尾进程使用。
	// A new-style start.sh (no lib/observability.sh reference) customized by the admin means the
	// layout is already healed: nothing is overwritten, but InstallDir is still set for the reaper.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	custom := "#!/bin/bash\n# STX start script (admin customized)\necho custom\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", "start.sh"), []byte(custom), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("port: 17800\n"), 0o644))

	var logs []string
	res, err := Heal(context.Background(), testOptions(dir, &logs))
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(dir), res.InstallDir)
	require.Empty(t, res.Replaced)
	require.Empty(t, res.Moved)
	require.Empty(t, logs)

	raw, err := os.ReadFile(filepath.Join(dir, "bin", "start.sh"))
	require.NoError(t, err)
	require.Equal(t, custom, string(raw))
}

func TestHeal_SkipsDevDir(t *testing.T) {
	// 开发仓库布局：bin/start.sh 带 STX 标识但没有 config.yaml/stx.env，必须跳过。
	// Dev repo layout: bin/start.sh carries the STX marker but no config.yaml/stx.env — must skip.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", "start.sh"),
		[]byte("#!/bin/bash\n# STX dev helper\n"), 0o755))

	res, err := Heal(context.Background(), testOptions(dir, nil))
	require.NoError(t, err)
	require.Empty(t, res.InstallDir)
	require.FileExists(t, filepath.Join(dir, "bin", "start.sh"))
}

func TestHeal_Gates(t *testing.T) {
	dir := makeInstallDir(t)

	// STX_LAYOUT_HEAL=off / STX_LAYOUT_HEAL=off skips everything.
	opts := testOptions(dir, nil)
	opts.Getenv = func(k string) string {
		if k == LayoutHealEnv {
			return "off"
		}
		return ""
	}
	res, err := Heal(context.Background(), opts)
	require.NoError(t, err)
	require.Empty(t, res.InstallDir)

	// Docker 环境 / Docker containers are skipped.
	opts = testOptions(dir, nil)
	opts.InDocker = func() bool { return true }
	res, err = Heal(context.Background(), opts)
	require.NoError(t, err)
	require.Empty(t, res.InstallDir)

	// 非 Linux / Non-Linux platforms are skipped.
	opts = testOptions(dir, nil)
	opts.GOOS = "darwin"
	res, err = Heal(context.Background(), opts)
	require.NoError(t, err)
	require.Empty(t, res.InstallDir)
}

func TestHeal_CustomScriptsFS(t *testing.T) {
	dir := makeInstallDir(t)
	opts := testOptions(dir, nil)
	opts.ScriptsFS = fstest.MapFS{
		"bin/start.sh":  {Data: []byte("#!/bin/bash\necho new start\n"), Mode: 0o755},
		"bin/stop.sh":   {Data: []byte("#!/bin/bash\necho new stop\n"), Mode: 0o755},
		"bin/status.sh": {Data: []byte("#!/bin/bash\necho new status\n"), Mode: 0o755},
	}
	res, err := Heal(context.Background(), opts)
	require.NoError(t, err)
	require.Len(t, res.Replaced, 3)
	raw, err := os.ReadFile(filepath.Join(dir, "bin", "start.sh"))
	require.NoError(t, err)
	require.Equal(t, "#!/bin/bash\necho new start\n", string(raw))
}

// swapReapHooks 替换进程操作钩子并返回恢复函数。/ swapReapHooks installs process-op hooks and returns a restore func.
func swapReapHooks(t *testing.T, name func(int) (string, error), alive func(int) bool, signal func(int, syscall.Signal) error) {
	t.Helper()
	origName, origAlive, origSignal := procNameOf, processAlive, signalProc
	procNameOf, processAlive, signalProc = name, alive, signal
	t.Cleanup(func() {
		procNameOf, processAlive, signalProc = origName, origAlive, origSignal
	})
}

func TestReapOnce_PidReuseNeverSignals(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "observability", "prometheus", "prometheus.pid")
	require.NoError(t, os.MkdirAll(filepath.Dir(pidFile), 0o755))
	require.NoError(t, os.WriteFile(pidFile, []byte("4242\n"), 0o644))

	var signals []syscall.Signal
	swapReapHooks(t,
		func(pid int) (string, error) { require.Equal(t, 4242, pid); return "sshd", nil },
		func(int) bool { return true },
		func(_ int, sig syscall.Signal) error { signals = append(signals, sig); return nil },
	)

	done := reapOnce(dir, legacyProcs[0])
	// 名不符（pid 复用或 setsid fork→exec 窗口）绝不能发信号，也不删文件、不标记完成，
	// 下一轮重核；pid 真被复用时最多多等一个收尾窗口。
	// A mismatched name (pid reuse or the setsid fork->exec window) is never signaled and the
	// pid file is kept for the next sweep; a genuinely reused pid only costs one extra window.
	require.False(t, done)
	require.Empty(t, signals)
	require.FileExists(t, pidFile)
}

func TestReapOnce_NameMismatchRecoversAfterExec(t *testing.T) {
	// setsid X & 的 fork→exec 窗口里 exe 是 bash/setsid：本轮名不符只等待，exec 完成后
	// 下一轮进程名对上再正常收尾。
	// Inside the setsid fork->exec window exe is still bash/setsid: this sweep only waits, and
	// once exec completes the next sweep sees the real name and finishes the reap.
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "observability", "prometheus", "prometheus.pid")
	require.NoError(t, os.MkdirAll(filepath.Dir(pidFile), 0o755))
	require.NoError(t, os.WriteFile(pidFile, []byte("4242\n"), 0o644))

	name := "bash"
	dead := false
	var signals []syscall.Signal
	swapReapHooks(t,
		func(int) (string, error) { return name, nil },
		func(int) bool { return !dead },
		func(_ int, sig syscall.Signal) error {
			signals = append(signals, sig)
			if sig == syscall.SIGTERM {
				dead = true
			}
			return nil
		},
	)

	require.False(t, reapOnce(dir, legacyProcs[0]))
	require.Empty(t, signals)
	require.FileExists(t, pidFile)

	name = "prometheus"
	require.True(t, reapOnce(dir, legacyProcs[0]))
	require.Equal(t, []syscall.Signal{syscall.SIGTERM}, signals)
	require.NoFileExists(t, pidFile)
}

func TestReapOnce_KillsMatchingProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "observability", "alertmanager", "alertmanager.pid")
	require.NoError(t, os.MkdirAll(filepath.Dir(pidFile), 0o755))
	require.NoError(t, os.WriteFile(pidFile, []byte("777\n"), 0o644))

	dead := false
	var signals []syscall.Signal
	swapReapHooks(t,
		func(int) (string, error) { return "alertmanager", nil },
		func(int) bool { return !dead },
		func(_ int, sig syscall.Signal) error {
			signals = append(signals, sig)
			if sig == syscall.SIGTERM {
				dead = true
			}
			return nil
		},
	)

	done := reapOnce(dir, legacyProcs[1])
	require.True(t, done)
	require.Equal(t, []syscall.Signal{syscall.SIGTERM}, signals)
	require.NoFileExists(t, pidFile)
}

func TestReapOnce_KillAfterGraceExpires(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "observability", "grafana", "grafana.pid")
	require.NoError(t, os.MkdirAll(filepath.Dir(pidFile), 0o755))
	require.NoError(t, os.WriteFile(pidFile, []byte("999\n"), 0o644))

	origGrace := reapTermGrace
	reapTermGrace = 10 * time.Millisecond
	t.Cleanup(func() { reapTermGrace = origGrace })

	var signals []syscall.Signal
	swapReapHooks(t,
		func(int) (string, error) { return "grafana-server", nil },
		func(int) bool { return true },
		func(_ int, sig syscall.Signal) error { signals = append(signals, sig); return nil },
	)

	done := reapOnce(dir, legacyProcs[2])
	require.True(t, done)
	require.Equal(t, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}, signals)
}

func TestReapOnce_MissingPidFileKeepsPolling(t *testing.T) {
	dir := t.TempDir()
	swapReapHooks(t, nil, nil, nil)
	require.False(t, reapOnce(dir, legacyProcs[0]))
}

func TestReapOnce_IdentityReadFailsKeepsPolling(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "observability", "prometheus", "prometheus.pid")
	require.NoError(t, os.MkdirAll(filepath.Dir(pidFile), 0o755))
	require.NoError(t, os.WriteFile(pidFile, []byte("55\n"), 0o644))

	var signals []syscall.Signal
	swapReapHooks(t,
		func(int) (string, error) { return "", os.ErrNotExist },
		func(int) bool { return true },
		func(_ int, sig syscall.Signal) error { signals = append(signals, sig); return nil },
	)

	require.False(t, reapOnce(dir, legacyProcs[0]))
	require.Empty(t, signals)
	require.FileExists(t, pidFile)
}
