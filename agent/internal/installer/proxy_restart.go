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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logger"
)

// 运行中 stx-java-proxy 的重启结果 / restart outcome of a running stx-java-proxy
const (
	ProxyRestartRestarted  = "restarted"
	ProxyRestartNotRunning = "not_running"
	ProxyRestartRolledBack = "rolled_back"
	ProxyRestartFailed     = "failed"
	ProxyRestartSkipped    = "skipped"
)

// ProxyRestartResult 描述一个托管 stx-java-proxy 进程的重启结果。
// ProxyRestartResult describes the restart of one managed stx-java-proxy process.
type ProxyRestartResult struct {
	StateDir string `json:"state_dir,omitempty"`
	Status   string `json:"status"`
	OldPID   int    `json:"old_pid,omitempty"`
	NewPID   int    `json:"new_pid,omitempty"`
	Port     int    `json:"port,omitempty"`
	JarPath  string `json:"jar_path,omitempty"`
	Message  string `json:"message,omitempty"`
}

// proxyProcessSnapshot 是重启所需的原进程信息（argv/env/cwd/输出文件）。
// proxyProcessSnapshot holds what is needed to relaunch the process identically (argv/env/cwd/output file).
type proxyProcessSnapshot struct {
	Argv    []string
	Env     []string
	Dir     string
	LogPath string
}

// 测试钩子 / test hooks
var (
	proxyRestartHealthTimeout = 90 * time.Second
	proxyStatusFn             = GetManagedSTXJavaProxyServiceStatus
	proxyStopFn               = StopManagedSTXJavaProxyService
	proxyHealthFn             = waitForSTXJavaProxyHealthy
	proxySnapshotFn           = snapshotProxyProcess
	proxyCmdlineFn            = stxJavaProxyProcessCommandLine
)

// IsSeaTunnelEngineCommandLine 判断命令行是否属于 SeaTunnel/Hazelcast 引擎进程。自升级绝不触碰这类进程。
// IsSeaTunnelEngineCommandLine reports whether a command line belongs to a SeaTunnel/Hazelcast engine process.
// Self-upgrade never touches such processes.
func IsSeaTunnelEngineCommandLine(cmdline string) bool {
	for _, marker := range []string{
		"org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer",
		"org.apache.seatunnel.core.starter.seatunnel.SeaTunnelClient",
		"org.apache.seatunnel.core.starter.SeaTunnel",
		"seatunnel-cluster.sh",
		"com.hazelcast.core.server.HazelcastMemberStarter",
	} {
		if strings.Contains(cmdline, marker) {
			return true
		}
	}
	return false
}

// isRestartableProxyCommandLine 仅接受确实是 stx-java-proxy、且不是 SeaTunnel 引擎的进程。
// isRestartableProxyCommandLine accepts only processes that are stx-java-proxy and not a SeaTunnel engine.
func isRestartableProxyCommandLine(cmdline string) bool {
	return isSTXJavaProxyCommandLine(cmdline) && !IsSeaTunnelEngineCommandLine(cmdline)
}

// RestartRunningSTXJavaProxies 让每个正在运行的托管 stx-java-proxy 以完全相同的 argv/env/cwd 重启，从而加载已替换的 jar。
// 重启后健康检查失败时调用 rollback（恢复备份 jar）再按原方式启动一次；仍失败则报告 failed。
// 只处理命令行校验为 stx-java-proxy 的进程，SeaTunnel/Hazelcast 进程永远不会被停止或重启。
//
// RestartRunningSTXJavaProxies relaunches every running managed stx-java-proxy with the exact same argv/env/cwd so it
// loads the replaced jars. When the health check fails after the restart, rollback (restore backup jars) is called and
// the process is relaunched once more the same way; if that also fails the result is failed. Only processes whose
// command line verifies as stx-java-proxy are handled; SeaTunnel/Hazelcast processes are never stopped or restarted.
func RestartRunningSTXJavaProxies(ctx context.Context, installDirs []string, rollback func() error) []ProxyRestartResult {
	seen := map[string]bool{}
	var results []ProxyRestartResult
	for _, dir := range append(append([]string(nil), installDirs...), "") {
		status, err := proxyStatusFn(ctx, dir)
		if err != nil || status == nil {
			continue
		}
		key := status.StateDir
		if seen[key] {
			continue
		}
		seen[key] = true
		if !status.Managed {
			results = append(results, ProxyRestartResult{StateDir: key, Status: ProxyRestartSkipped, Message: "external stx-java-proxy endpoint is not managed by this agent / 外部 proxy 不由 Agent 管理"})
			continue
		}
		if !status.Running || status.PID <= 0 {
			continue
		}
		results = append(results, restartOneProxy(ctx, dir, status, rollback))
	}
	if len(results) == 0 {
		results = append(results, ProxyRestartResult{Status: ProxyRestartNotRunning, Message: "no running stx-java-proxy; new jars are used on its next start / 没有运行中的 proxy，下次启动即使用新 jar"})
	}
	return results
}

func restartOneProxy(ctx context.Context, installDir string, status *STXJavaProxyServiceStatus, rollback func() error) ProxyRestartResult {
	res := ProxyRestartResult{StateDir: status.StateDir, OldPID: status.PID, Port: status.Port}
	cmdline := proxyCmdlineFn(status.PID)
	if !isRestartableProxyCommandLine(cmdline) {
		res.Status = ProxyRestartSkipped
		res.Message = fmt.Sprintf("pid %d is not a stx-java-proxy process; refusing to touch it / pid %d 不是 stx-java-proxy，拒绝操作", status.PID, status.PID)
		return res
	}
	snap, err := proxySnapshotFn(status.PID)
	if err != nil {
		res.Status = ProxyRestartFailed
		res.Message = "cannot capture the running proxy command line, proxy left running on old jars: " + err.Error() + " / 无法读取 proxy 启动参数，保持旧 jar 运行"
		return res
	}
	if snap.LogPath == "" {
		snap.LogPath = status.LogPath
	}
	res.JarPath = proxyJarFromArgv(snap.Argv)
	if _, err := proxyStopFn(ctx, installDir); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already stopped") {
		res.Status = ProxyRestartFailed
		res.Message = "stop proxy failed: " + err.Error() + " / 停止 proxy 失败"
		return res
	}
	pid, err := launchProxy(snap, status)
	if err == nil {
		err = proxyHealthFn(ctx, stxJavaProxyServiceBaseURL(status.Port), proxyRestartHealthTimeout)
	}
	if err == nil {
		res.Status, res.NewPID = ProxyRestartRestarted, pid
		res.Message = fmt.Sprintf("restarted on the new jars (pid %d → %d) / 已以新 jar 重启", status.PID, pid)
		logger.InfoF(ctx, "[SelfUpdate] stx-java-proxy restarted pid %d -> %d / proxy 已重启", status.PID, pid)
		return res
	}
	firstErr := err
	logger.ErrorF(ctx, "[SelfUpdate] stx-java-proxy unhealthy after restart: %v; restoring backup jars / 重启后不健康，恢复备份 jar", err)
	killQuietly(pid)
	if rollback == nil {
		res.Status, res.Message = ProxyRestartFailed, "proxy unhealthy after restart and no backup to restore: "+firstErr.Error()
		return res
	}
	if rbErr := rollback(); rbErr != nil {
		res.Status, res.Message = ProxyRestartFailed, "proxy unhealthy after restart; restoring backup jars failed: "+rbErr.Error()
		return res
	}
	pid, err = launchProxy(snap, status)
	if err == nil {
		err = proxyHealthFn(ctx, stxJavaProxyServiceBaseURL(status.Port), proxyRestartHealthTimeout)
	}
	if err != nil {
		killQuietly(pid)
		res.Status, res.Message = ProxyRestartFailed, fmt.Sprintf("proxy unhealthy on new jars (%v) and on restored jars (%v) / 新旧 jar 均无法健康启动", firstErr, err)
		return res
	}
	res.Status, res.NewPID = ProxyRestartRolledBack, pid
	res.Message = fmt.Sprintf("new jars failed (%v); restored backup jars and restarted (pid %d) / 新 jar 启动失败，已恢复备份 jar 并重启", firstErr, pid)
	return res
}

// launchProxy 以原 argv/env/cwd 脱离会话启动 proxy（经 sh 后台化，由 init 收养，与原启动方式一致），并写回 service.pid。
// launchProxy starts the proxy detached with the original argv/env/cwd (backgrounded via sh so init adopts it, like
// the original launch) and records service.pid.
func launchProxy(snap proxyProcessSnapshot, status *STXJavaProxyServiceStatus) (int, error) {
	if len(snap.Argv) == 0 {
		return 0, fmt.Errorf("empty proxy command line")
	}
	logPath := snap.LogPath
	if logPath == "" {
		logPath = os.DevNull
	}
	args := append([]string{"-c", `log="$1"; shift; "$@" >>"$log" 2>&1 </dev/null & echo $!`, "sh", logPath}, snap.Argv...)
	cmd := exec.Command("/bin/sh", args...)
	cmd.Env = snap.Env
	cmd.Dir = snap.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("launch proxy: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("launch proxy: unexpected pid output %q", strings.TrimSpace(string(out)))
	}
	if status.StateDir != "" {
		_ = os.WriteFile(filepath.Join(status.StateDir, "service.pid"), []byte(strconv.Itoa(pid)+"\n"), 0o644)
	}
	return pid, nil
}

func killQuietly(pid int) {
	if pid > 0 && stxJavaProxyPIDMatches(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// snapshotProxyProcess 从 /proc 读取进程的 argv、环境、工作目录和标准输出文件。
// snapshotProxyProcess reads a process's argv, environment, working directory and stdout file from /proc.
func snapshotProxyProcess(pid int) (proxyProcessSnapshot, error) {
	var snap proxyProcessSnapshot
	base := filepath.Join("/proc", strconv.Itoa(pid))
	raw, err := os.ReadFile(filepath.Join(base, "cmdline"))
	if err != nil {
		return snap, err
	}
	snap.Argv = splitNul(raw)
	if len(snap.Argv) == 0 {
		return snap, fmt.Errorf("empty cmdline for pid %d", pid)
	}
	if env, err := os.ReadFile(filepath.Join(base, "environ")); err == nil {
		snap.Env = splitNul(env)
	} else {
		return snap, fmt.Errorf("read environ: %w", err)
	}
	if dir, err := os.Readlink(filepath.Join(base, "cwd")); err == nil {
		snap.Dir = dir
	}
	if out, err := os.Readlink(filepath.Join(base, "fd", "1")); err == nil && filepath.IsAbs(out) && !strings.HasPrefix(out, "/dev/") {
		if fi, err := os.Stat(out); err == nil && fi.Mode().IsRegular() {
			snap.LogPath = out
		}
	}
	return snap, nil
}

func splitNul(raw []byte) []string {
	parts := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// proxyJarFromArgv 返回命令行中使用的 stx-java-proxy jar 路径（-jar 或 classpath）。
// proxyJarFromArgv returns the stx-java-proxy jar path used by the command line (-jar or classpath).
func proxyJarFromArgv(argv []string) string {
	for _, arg := range argv {
		for _, part := range strings.Split(arg, string(os.PathListSeparator)) {
			base := filepath.Base(part)
			if strings.HasPrefix(base, "stx-java-proxy") && strings.HasSuffix(base, ".jar") {
				return part
			}
		}
	}
	return ""
}

// RunningSTXJavaProxyJars 返回正在运行的托管 proxy 实际使用的 jar 路径（用于同时替换不在默认目录中的 jar）。
// RunningSTXJavaProxyJars returns the jar paths used by running managed proxies (so jars outside the default dir are
// replaced too).
func RunningSTXJavaProxyJars(ctx context.Context, installDirs []string) []string {
	seen := map[string]bool{}
	var jars []string
	for _, dir := range append(append([]string(nil), installDirs...), "") {
		status, err := proxyStatusFn(ctx, dir)
		if err != nil || status == nil || !status.Running || status.PID <= 0 {
			continue
		}
		cmdline := proxyCmdlineFn(status.PID)
		if !isRestartableProxyCommandLine(cmdline) {
			continue
		}
		snap, err := proxySnapshotFn(status.PID)
		if err != nil {
			continue
		}
		if jar := proxyJarFromArgv(snap.Argv); jar != "" && !seen[jar] {
			seen[jar] = true
			jars = append(jars, jar)
		}
	}
	return jars
}
