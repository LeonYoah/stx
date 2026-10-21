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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startFakeProcess 启动一个命令行带有指定标记的长驻进程（sh -c 'sleep' 的 $0/$@ 会出现在 /proc/<pid>/cmdline）。
// startFakeProcess starts a long-running process whose command line carries the given markers.
func startFakeProcess(t *testing.T, markers ...string) int {
	t.Helper()
	args := append([]string{"-c", "sleep 300"}, markers...)
	cmd := exec.Command("/bin/sh", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake process: %v", err)
	}
	go func() { _ = cmd.Wait() }()
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	waitFor(t, func() bool { return strings.Contains(stxJavaProxyProcessCommandLine(pid), markers[0]) })
	return pid
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func alive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil && !isZombie(pid) }

func isZombie(pid int) bool {
	state, ok := linuxProcessState(pid)
	return ok && (state == "Z" || state == "X")
}

type proxyRestartHarness struct {
	stateDir  string
	pid       int
	stopCalls int
	health    []error
}

func installProxyHarness(t *testing.T, h *proxyRestartHarness) {
	t.Helper()
	oldStatus, oldStop, oldHealth, oldTimeout := proxyStatusFn, proxyStopFn, proxyHealthFn, proxyRestartHealthTimeout
	t.Cleanup(func() {
		proxyStatusFn, proxyStopFn, proxyHealthFn, proxyRestartHealthTimeout = oldStatus, oldStop, oldHealth, oldTimeout
	})
	proxyRestartHealthTimeout = time.Second
	proxyStatusFn = func(ctx context.Context, installDir string) (*STXJavaProxyServiceStatus, error) {
		return &STXJavaProxyServiceStatus{Managed: true, Running: alive(h.pid), PID: h.pid, Port: 18080, StateDir: h.stateDir}, nil
	}
	proxyStopFn = func(ctx context.Context, installDir string) (*STXJavaProxyServiceStatus, error) {
		h.stopCalls++
		_ = syscall.Kill(h.pid, syscall.SIGKILL)
		waitFor(t, func() bool { return !alive(h.pid) })
		return &STXJavaProxyServiceStatus{Managed: true}, nil
	}
	proxyHealthFn = func(ctx context.Context, baseURL string, timeout time.Duration) error {
		if len(h.health) == 0 {
			return nil
		}
		err := h.health[0]
		h.health = h.health[1:]
		return err
	}
}

func skipUnlessLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("relies on /proc")
	}
}

// 重启运行中的 proxy：新进程沿用原命令行，SeaTunnel 进程 PID 不变。
// Restarting a running proxy relaunches the same command line and leaves the SeaTunnel process untouched.
func TestRestartRunningSTXJavaProxiesRelaunchesProxyOnly(t *testing.T) {
	skipUnlessLinux(t)
	dir := t.TempDir()
	jar := filepath.Join(dir, "stx-java-proxy-v2.jar")
	seatunnel := startFakeProcess(t, "org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer", "-DSEATUNNEL_HOME=/opt/st")
	proxy := startFakeProcess(t, "stx-java-proxy", "-cp", jar+":/opt/st/starter/seatunnel-starter.jar", "org.apache.seatunnel.stx.StxJavaProxyApplication")
	h := &proxyRestartHarness{stateDir: dir, pid: proxy}
	installProxyHarness(t, h)

	results := RestartRunningSTXJavaProxies(context.Background(), []string{"/opt/st"}, func() error {
		t.Fatal("rollback must not run when the restart is healthy")
		return nil
	})
	if len(results) != 1 || results[0].Status != ProxyRestartRestarted {
		t.Fatalf("unexpected results: %+v", results)
	}
	res := results[0]
	t.Cleanup(func() { _ = syscall.Kill(res.NewPID, syscall.SIGKILL) })
	if res.NewPID == 0 || res.NewPID == proxy || !alive(res.NewPID) {
		t.Fatalf("expected a new live proxy pid, got %+v", res)
	}
	if got := stxJavaProxyProcessCommandLine(res.NewPID); !strings.Contains(got, jar) {
		t.Fatalf("relaunched proxy must keep its command line, got %q", got)
	}
	if res.JarPath != jar {
		t.Fatalf("jar path = %q, want %q", res.JarPath, jar)
	}
	pidFile, _ := os.ReadFile(filepath.Join(dir, "service.pid"))
	if strings.TrimSpace(string(pidFile)) != strconv.Itoa(res.NewPID) {
		t.Fatalf("service.pid = %q, want %d", pidFile, res.NewPID)
	}
	if !alive(seatunnel) {
		t.Fatal("SeaTunnel process must never be stopped by a proxy restart")
	}
	if h.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", h.stopCalls)
	}
}

// 新 jar 启动后不健康：恢复备份 jar 后再次启动，状态为 rolled_back。
// When the proxy is unhealthy on the new jars, the backup jars are restored and it is relaunched: rolled_back.
func TestRestartRunningSTXJavaProxiesRollsBackOnUnhealthyStart(t *testing.T) {
	skipUnlessLinux(t)
	dir := t.TempDir()
	proxy := startFakeProcess(t, "stx-java-proxy", "-cp", filepath.Join(dir, "stx-java-proxy-v3.jar"))
	h := &proxyRestartHarness{stateDir: dir, pid: proxy, health: []error{errors.New("not healthy")}}
	installProxyHarness(t, h)
	rolledBack := false
	results := RestartRunningSTXJavaProxies(context.Background(), nil, func() error { rolledBack = true; return nil })
	if len(results) != 1 || results[0].Status != ProxyRestartRolledBack || !rolledBack {
		t.Fatalf("expected rolled_back with jar restore, got %+v (rollback=%v)", results, rolledBack)
	}
	t.Cleanup(func() { _ = syscall.Kill(results[0].NewPID, syscall.SIGKILL) })
	if !alive(results[0].NewPID) {
		t.Fatal("proxy must be running again on the restored jars")
	}
}

// 新旧 jar 都启动失败：报告 failed。/ Unhealthy on both new and restored jars reports failed.
func TestRestartRunningSTXJavaProxiesFailsWhenRestoreAlsoUnhealthy(t *testing.T) {
	skipUnlessLinux(t)
	dir := t.TempDir()
	proxy := startFakeProcess(t, "stx-java-proxy", "-cp", filepath.Join(dir, "stx-java-proxy-v2.jar"))
	h := &proxyRestartHarness{stateDir: dir, pid: proxy, health: []error{errors.New("bad new"), errors.New("bad old")}}
	installProxyHarness(t, h)
	results := RestartRunningSTXJavaProxies(context.Background(), nil, func() error { return nil })
	if len(results) != 1 || results[0].Status != ProxyRestartFailed {
		t.Fatalf("expected failed, got %+v", results)
	}
}

// 记录的 PID 实际是 SeaTunnel 进程时拒绝操作：不停止、不重启。
// When the recorded PID is actually a SeaTunnel process, refuse: no stop, no restart.
func TestRestartRunningSTXJavaProxiesRefusesSeaTunnelProcess(t *testing.T) {
	skipUnlessLinux(t)
	dir := t.TempDir()
	seatunnel := startFakeProcess(t, "org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer", "stx-java-proxy-v2.jar")
	h := &proxyRestartHarness{stateDir: dir, pid: seatunnel}
	installProxyHarness(t, h)
	results := RestartRunningSTXJavaProxies(context.Background(), nil, nil)
	if len(results) != 1 || results[0].Status != ProxyRestartSkipped {
		t.Fatalf("expected skipped, got %+v", results)
	}
	if h.stopCalls != 0 || !alive(seatunnel) {
		t.Fatalf("SeaTunnel process must not be stopped (stop calls %d, alive %v)", h.stopCalls, alive(seatunnel))
	}
}

// 没有运行中的 proxy 时只报告 not_running。/ Without a running proxy only not_running is reported.
func TestRestartRunningSTXJavaProxiesNotRunning(t *testing.T) {
	h := &proxyRestartHarness{stateDir: t.TempDir(), pid: 0}
	installProxyHarness(t, h)
	results := RestartRunningSTXJavaProxies(context.Background(), nil, nil)
	if len(results) != 1 || results[0].Status != ProxyRestartNotRunning || h.stopCalls != 0 {
		t.Fatalf("expected not_running without stop, got %+v (stops %d)", results, h.stopCalls)
	}
}

func TestIsSeaTunnelEngineCommandLine(t *testing.T) {
	if !IsSeaTunnelEngineCommandLine("java -cp x org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer -c conf") {
		t.Fatal("SeaTunnel server must be recognised")
	}
	if IsSeaTunnelEngineCommandLine("java -cp /a/stx-java-proxy-v2.jar:/opt/st/starter/seatunnel-starter.jar org.apache.seatunnel.stx.StxJavaProxyApplication") {
		t.Fatal("the proxy (which has SeaTunnel jars on its classpath) must not be treated as the engine")
	}
}
