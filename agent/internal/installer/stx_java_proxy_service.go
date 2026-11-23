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
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/LeonYoah/stx/agent/internal/javaenv"
	"github.com/LeonYoah/stx/agent/internal/logger"
	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
)

const stxJavaProxyStopTimeout = 8 * time.Second

// STXJavaProxyServiceStatus describes the current managed stx-java-proxy state.
type STXJavaProxyServiceStatus struct {
	Service   string                 `json:"service"`
	Managed   bool                   `json:"managed"`
	Running   bool                   `json:"running"`
	Healthy   bool                   `json:"healthy"`
	Endpoint  string                 `json:"endpoint,omitempty"`
	Port      int                    `json:"port,omitempty"`
	PID       int                    `json:"pid,omitempty"`
	LogPath   string                 `json:"log_path,omitempty"`
	StateDir  string                 `json:"state_dir,omitempty"`
	Message   string                 `json:"message,omitempty"`
	JvmOpts   string                 `json:"jvm_opts,omitempty"`
	JvmMemory map[string]interface{} `json:"jvm_memory,omitempty"`
}

// StartManagedSTXJavaProxyService ensures the managed stx-java-proxy service is available.
// preferredPort>0 时优先使用该端口；否则回退到环境变量 / 落盘端口 / 默认 18080。
// When preferredPort > 0 it is tried first; otherwise env / persisted / default 18080 apply.
// javaHome 是代理所在节点配置的 JDK（cluster_nodes.java_home），为空时与 SeaTunnel 一样使用 Agent 环境中的 java。
// javaHome is the configured JDK of the node hosting the proxy (cluster_nodes.java_home); empty uses the Agent
// environment java, just like SeaTunnel.
func StartManagedSTXJavaProxyService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	preferredPort int,
	jvmOpts string,
	javaHome string,
	assetBaseURL ...string,
) (*STXJavaProxyServiceStatus, error) {
	desiredJvmOpts := strings.TrimSpace(jvmOpts)
	javaHome = javaenv.NormalizeHome(javaHome)
	if javaHome == "" {
		javaHome = javaenv.HomeFromContext(ctx)
	}
	if javaHome != "" {
		// 配置的 JDK 不可用时直接失败，不回退到其他 java。
		// Fail fast on an unusable configured JDK instead of falling back to another java.
		if _, err := javaenv.ValidateHome(javaHome); err != nil {
			return nil, fmt.Errorf("invalid node JDK for stx-java-proxy / stx-java-proxy 的节点 JDK 不可用: %w", err)
		}
		ctx = javaenv.WithHome(ctx, javaHome)
	}

	// 启动前补齐本地脚本/jar：同机控制面目录拷贝或从控制面资产 API 拉取（DEF-R2-1）。
	// Materialize local script/jar before start: copy from co-located CP dirs or pull asset API (DEF-R2-1).
	assetBase := ""
	if len(assetBaseURL) > 0 {
		assetBase = strings.TrimSpace(assetBaseURL[0])
	}
	if err := EnsureSTXJavaProxySupportAssets(ctx, installDir, seatunnelVersion, assetBase); err != nil {
		logger.WarnF(ctx, "[stx-java-proxy] ensure support assets failed (will still try local candidates): %v / 补齐辅助资产失败（仍尝试本地候选）: %v", err, err)
	}

	status, _ := GetManagedSTXJavaProxyServiceStatus(ctx, installDir)
	// 已健康且端口 / JVM / 安装目录 / 代际 jar 均匹配时直接返回，避免无谓重启。
	// Skip restart when healthy and port / jvmOpts / installDir / epoch jar all match.
	if status != nil && status.Healthy {
		portMatches := preferredPort <= 0 || status.Port <= 0 || status.Port == preferredPort
		jvmMatches := desiredJvmOpts == "" || status.JvmOpts == desiredJvmOpts
		runtimeMatches := stxJavaProxyStatusMatchesDesired(status, installDir, seatunnelVersion)
		javaMatches := stxJavaProxyJavaMatchesDesired(status.PID, javaHome)
		if portMatches && jvmMatches && runtimeMatches && javaMatches {
			return status, nil
		}
		// 端口、JVM、安装目录或代际不一致时先停旧实例，再按目标参数启动。
		// Stop the old instance before starting with the preferred port / jvmOpts / runtime.
		if _, stopErr := StopManagedSTXJavaProxyService(ctx, installDir); stopErr != nil {
			logger.WarnF(ctx, "[stx-java-proxy] stop before config switch failed: preferredPort=%d, desiredJvmOpts=%s, installDir=%s, version=%s, error=%v", preferredPort, desiredJvmOpts, installDir, seatunnelVersion, stopErr)
		}
	}

	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, preferredPort, desiredJvmOpts)
	if err != nil {
		return nil, err
	}

	status, statusErr := GetManagedSTXJavaProxyServiceStatus(ctx, installDir)
	if statusErr != nil {
		return &STXJavaProxyServiceStatus{
			Service:  "stx_java_proxy",
			Managed:  true,
			Running:  true,
			Healthy:  true,
			Endpoint: baseURL,
			Message:  "stx-java-proxy service started",
			StateDir: stxJavaProxyServiceStateDir(installDir),
			LogPath:  filepath.Join(stxJavaProxyServiceStateDir(installDir), "service.log"),
			JvmOpts:  desiredJvmOpts,
		}, nil
	}
	status.Message = firstNonBlank(status.Message, "stx-java-proxy service started")
	return status, nil
}

// stxJavaProxyJavaMatchesDesired 判断运行中的代理是否使用节点配置的 JDK；未配置或无法读取进程时视为匹配（保持原有行为）。
// stxJavaProxyJavaMatchesDesired reports whether the running proxy uses the node's configured JDK; no configured
// JDK or an unreadable process counts as a match (the original behavior).
func stxJavaProxyJavaMatchesDesired(pid int, javaHome string) bool {
	javaHome = javaenv.NormalizeHome(javaHome)
	if javaHome == "" || pid <= 0 {
		return true
	}
	exe := javaenv.ProcessExecutable(pid)
	if exe == "" {
		return true
	}
	return javaenv.SameExecutable(exe, filepath.Join(javaHome, "bin", "java"))
}

// GetManagedSTXJavaProxyServiceStatus returns the current stx-java-proxy state.
func GetManagedSTXJavaProxyServiceStatus(ctx context.Context, installDir string) (*STXJavaProxyServiceStatus, error) {
	preferredPort := stxJavaProxyPortFromContext(ctx)
	status := &STXJavaProxyServiceStatus{
		Service:  "stx_java_proxy",
		Managed:  true,
		StateDir: stxJavaProxyServiceStateDir(installDir),
		LogPath:  filepath.Join(stxJavaProxyServiceStateDir(installDir), "service.log"),
	}

	// 升级兼容：引入实例分目录之前的状态文件直接写在 service 根目录，升级后仍在运行的
	// 旧 proxy 只存在于旧目录。新目录缺失时回退读取旧目录，否则旧进程既无法被认领也
	// 无法被停止，重启会因端口被占失败。
	// Upgrade compat: state files written before per-instance dirs lived directly under
	// the service dir, and an upgraded legacy proxy only exists there. Falling back to the
	// legacy dir lets status adopt (and Stop/Restart own) a proxy still holding its port.
	legacyDir := stxJavaProxyServiceLegacyStateDir(installDir)
	readStateFile := func(name string) (string, bool) {
		if bytes, err := os.ReadFile(filepath.Join(status.StateDir, name)); err == nil {
			return string(bytes), false
		}
		bytes, err := os.ReadFile(filepath.Join(legacyDir, name))
		if err != nil {
			return "", false
		}
		return string(bytes), true
	}

	if content, _ := readStateFile("service.jvm_opts"); content != "" {
		status.JvmOpts = strings.TrimSpace(content)
	}

	if endpoint := strings.TrimSpace(os.Getenv(stxJavaProxyEndpointEnvVar)); endpoint != "" {
		normalized := strings.TrimRight(endpoint, "/")
		status.Managed = false
		status.Endpoint = normalized
		if port := stxJavaProxyPortFromEndpoint(normalized); port > 0 {
			status.Port = port
		}
		err, mem := probeSTXJavaProxyHealth(ctx, normalized, 1500*time.Millisecond)
		status.Healthy = err == nil
		status.Running = status.Healthy
		if mem != nil {
			status.JvmMemory = mem
		}
		if status.Healthy {
			status.Message = "using configured external stx-java-proxy endpoint"
		} else {
			status.Message = firstNonBlank(stxJavaProxyErrorString(err), "configured stx-java-proxy endpoint is unhealthy")
		}
		return status, nil
	}

	portFromLegacy := false
	if content, fromLegacy := readStateFile("service.port"); content != "" {
		if port, ok := parseSTXJavaProxyPort(strings.TrimSpace(content)); ok {
			status.Port = port
			status.Endpoint = stxJavaProxyServiceBaseURL(port)
			portFromLegacy = fromLegacy
		}
	}
	pidFromLegacy := false
	if content, fromLegacy := readStateFile("service.pid"); content != "" {
		if pid, err := strconv.Atoi(strings.TrimSpace(content)); err == nil && pid > 0 {
			status.PID = pid
			pidFromLegacy = fromLegacy
		}
	}
	// 状态文件 PID 已失效时清零，后续按端口回填真实监听进程。
	// Clear a stale pid-file value so later port lookup can refill the live listener.
	if status.PID > 0 && !stxJavaProxyPIDAlive(status.PID) {
		status.PID = 0
	}
	// 归属校验：PID 复用到其他 installDir 的 proxy 时不能认领为本集群进程。
	// Ownership check: a PID reused by another installDir's proxy must not be claimed as ours.
	if status.PID > 0 && stxJavaProxyPIDForeignForInstallDir(status.PID, installDir) {
		status.PID = 0
		_ = os.Remove(filepath.Join(status.StateDir, "service.pid"))
	}

	if status.Endpoint == "" {
		checkPorts := stxJavaProxyPortCandidates(status.StateDir, preferredPort)
		// 显式指定端口时只探测该端口，不扫默认 18080 串到其他集群实例。
		// With an explicit port, probe only that port instead of scanning the default
		// 18080, which could belong to another cluster's instance.
		if preferredPort > 0 {
			checkPorts = []int{preferredPort}
		}
		for _, port := range checkPorts {
			if port <= 0 {
				continue
			}
			// 归属预检：监听者可确认属于其他 installDir 时直接跳过，不认领端点也不写 stateDir。
			// Ownership pre-check: a listener provably owned by another installDir is skipped
			// entirely — its endpoint is never claimed nor written into our stateDir.
			listenerPID := stxJavaProxyPIDByPort(ctx, port)
			if listenerPID > 0 && stxJavaProxyPIDForeignForInstallDir(listenerPID, installDir) {
				continue
			}
			endpoint := stxJavaProxyServiceBaseURL(port)
			if err, mem := probeSTXJavaProxyHealth(ctx, endpoint, 1200*time.Millisecond); err == nil {
				status.Endpoint = endpoint
				status.Port = port
				status.Healthy = true
				status.Running = true
				if mem != nil {
					status.JvmMemory = mem
				}
				_ = os.MkdirAll(status.StateDir, 0o755)
				_ = os.WriteFile(filepath.Join(status.StateDir, "service.port"), []byte(strconv.Itoa(port)+"\n"), 0o644)
				break
			}
		}
	}

	if status.PID <= 0 && status.Port > 0 {
		if pid := stxJavaProxyPIDByPort(ctx, status.Port); pid > 0 {
			if stxJavaProxyPIDForeignForInstallDir(pid, installDir) {
				// 落盘端口实际被他集群 proxy 占用：端点/端口清零并清掉陈旧 port 文件。
				// The persisted port is actually held by another cluster's proxy: clear
				// endpoint/port and drop the stale port file.
				status.Endpoint = ""
				status.Port = 0
				status.Healthy = false
				status.Running = false
				status.JvmMemory = nil
				_ = os.Remove(filepath.Join(status.StateDir, "service.port"))
			} else {
				status.PID = pid
				_ = os.MkdirAll(status.StateDir, 0o755)
				_ = os.WriteFile(filepath.Join(status.StateDir, "service.pid"), []byte(strconv.Itoa(pid)+"\n"), 0o644)
			}
		}
	}
	if status.PID > 0 {
		status.Running = stxJavaProxyPIDAlive(status.PID)
	}
	if status.Endpoint != "" && !status.Healthy {
		if err, mem := probeSTXJavaProxyHealth(ctx, status.Endpoint, 1500*time.Millisecond); err == nil {
			status.Healthy = true
			status.Running = true
			if mem != nil {
				status.JvmMemory = mem
			}
		}
	}

	// 从旧目录接管的在跑进程仍写旧日志文件；重启前预览继续指向旧路径，避免读到一个
	// 刚被 touch 出来的空文件。/ A running process adopted from the legacy dir still writes
	// its old log; keep the preview on it until restart so the preview is not an empty file.
	if status.Running && status.PID > 0 && (pidFromLegacy || portFromLegacy) {
		legacyLog := filepath.Join(legacyDir, "service.log")
		if _, err := os.Stat(legacyLog); err == nil {
			status.LogPath = legacyLog
		}
	}

	if status.Managed && strings.TrimSpace(status.LogPath) != "" && status.Running {
		if _, err := os.Stat(status.LogPath); os.IsNotExist(err) {
			_ = os.MkdirAll(filepath.Dir(status.LogPath), 0o755)
			_ = os.WriteFile(status.LogPath, []byte{}, 0o644)
		}
	}

	switch {
	case status.Healthy:
		status.Message = "stx-java-proxy service is healthy"
	case status.Running:
		status.Message = "stx-java-proxy service process is running but health check failed"
	default:
		status.Message = "stx-java-proxy service is not running"
	}

	return status, nil
}

// stopTrackedSTXJavaProxy 仅在已有 pid/port 状态文件时停止托管进程。
// 没有状态文件时直接返回，避免卸载探测默认端口误停无关监听。
// stopTrackedSTXJavaProxy stops the managed process only when pid/port state files exist.
// Skip when no state file is present so uninstall does not probe the default port and kill an unrelated listener.
func stopTrackedSTXJavaProxy(ctx context.Context, installDir string) {
	stateDir := stxJavaProxyServiceStateDir(installDir)
	// 旧版 Agent 的 service.pid/service.port 可能在实例分目录前的旧目录里，同样视为有
	// 可停目标；真正的归属与存活校验在 GetManaged 里做。/ Older agents may have left
	// service.pid/service.port in the pre-instance dir; either counts as a stoppable
	// target — ownership and liveness are still verified inside GetManaged.
	legacyDir := stxJavaProxyServiceLegacyStateDir(installDir)
	hasState := fileExists(filepath.Join(stateDir, "service.pid")) ||
		fileExists(filepath.Join(stateDir, "service.port")) ||
		fileExists(filepath.Join(legacyDir, "service.pid")) ||
		fileExists(filepath.Join(legacyDir, "service.port"))
	if !hasState {
		return
	}
	if _, err := StopManagedSTXJavaProxyService(ctx, installDir); err != nil {
		logger.WarnF(ctx, "[Uninstall] stop stx-java-proxy failed, continue uninstall: install_dir=%s, error=%v", installDir, err)
	}
}

// StopManagedSTXJavaProxyService stops the locally managed stx-java-proxy service.
func StopManagedSTXJavaProxyService(ctx context.Context, installDir string) (*STXJavaProxyServiceStatus, error) {
	status, err := GetManagedSTXJavaProxyServiceStatus(ctx, installDir)
	if err != nil {
		return nil, err
	}
	if status == nil {
		return nil, fmt.Errorf("stx-java-proxy service status is unavailable")
	}
	if !status.Managed {
		status.Message = "configured external stx-java-proxy endpoint cannot be stopped by agent"
		return status, errors.New(status.Message)
	}
	if status.PID <= 0 && !status.Running {
		status.Message = "stx-java-proxy service is already stopped"
		return status, nil
	}
	if status.PID <= 0 {
		status.Message = "stx-java-proxy service PID is unavailable; unable to stop managed process safely"
		return status, errors.New(status.Message)
	}

	process, err := os.FindProcess(status.PID)
	if err != nil {
		return status, err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return status, err
	}
	if waitErr := waitForSTXJavaProxyShutdown(ctx, status.Endpoint, status.PID, stxJavaProxyStopTimeout); waitErr != nil {
		logger.WarnF(ctx, "[stx-java-proxy] graceful stop timed out, force killing managed process: pid=%d, port=%d, error=%v", status.PID, status.Port, waitErr)
		if killErr := forceKillSTXJavaProxyProcesses(ctx, status, installDir); killErr != nil {
			status.Message = "failed to stop stx-java-proxy service"
			return status, fmt.Errorf("%w; force kill failed: %v", waitErr, killErr)
		}
	}

	_ = os.Remove(filepath.Join(status.StateDir, "service.pid"))
	stoppedStatus, statusErr := GetManagedSTXJavaProxyServiceStatus(ctx, installDir)
	if statusErr != nil {
		return &STXJavaProxyServiceStatus{
			Service:  "stx_java_proxy",
			Managed:  true,
			Running:  false,
			Healthy:  false,
			Endpoint: status.Endpoint,
			Port:     status.Port,
			LogPath:  status.LogPath,
			StateDir: status.StateDir,
			Message:  "stx-java-proxy service stopped",
		}, nil
	}
	stoppedStatus.Message = "stx-java-proxy service stopped"
	return stoppedStatus, nil
}

// forceKillSTXJavaProxyProcesses 在优雅停止超时后强制终止已知的 proxy 进程。
// forceKillSTXJavaProxyProcesses forcefully terminates known proxy processes after graceful shutdown times out.
func forceKillSTXJavaProxyProcesses(ctx context.Context, status *STXJavaProxyServiceStatus, installDir string) error {
	if status == nil {
		return fmt.Errorf("stx-java-proxy service status is unavailable")
	}

	pids := make([]int, 0, 4)
	if status.PID > 0 {
		pids = append(pids, status.PID)
	}
	if status.Port > 0 {
		pids = append(pids, stxJavaProxyPIDsByPort(ctx, status.Port)...)
	}
	pids = uniquePositivePIDs(pids)
	if len(pids) == 0 {
		return fmt.Errorf("no stx-java-proxy process found to force kill")
	}

	killed := 0
	for _, pid := range pids {
		if !stxJavaProxyPIDAlive(pid) {
			continue
		}
		// 端口发现的 PID 需要再次校验命令行，避免误杀同端口上的非 proxy 进程。
		// PIDs discovered from the port are validated by cmdline to avoid killing an unrelated listener.
		if pid != status.PID && !stxJavaProxyPIDMatches(pid) {
			logger.WarnF(ctx, "[stx-java-proxy] skip force killing non-proxy listener: pid=%d, port=%d", pid, status.Port)
			continue
		}
		// 归属校验：即便命令行确为 proxy，属于其他 installDir 的实例也绝不强杀。
		// Ownership check: even a genuine proxy process is never killed when it provably
		// belongs to another installDir's instance.
		if stxJavaProxyPIDForeignForInstallDir(pid, installDir) {
			logger.WarnF(ctx, "[stx-java-proxy] skip force killing proxy of another install dir: pid=%d, port=%d, installDir=%s", pid, status.Port, installDir)
			continue
		}
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("force kill stx-java-proxy pid %d: %w", pid, err)
		}
		killed++
		logger.WarnF(ctx, "[stx-java-proxy] sent SIGKILL to managed process: pid=%d", pid)
	}
	if killed == 0 {
		return fmt.Errorf("no live stx-java-proxy process was force killed")
	}

	if waitErr := waitForSTXJavaProxyShutdown(ctx, status.Endpoint, status.PID, 3*time.Second); waitErr != nil {
		return waitErr
	}
	return nil
}

// waitForSTXJavaProxyShutdown 等待服务进程和健康端点全部停止。
// waitForSTXJavaProxyShutdown waits for both the service process and health endpoint to stop.
func waitForSTXJavaProxyShutdown(ctx context.Context, endpoint string, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		pidAlive := pid > 0 && stxJavaProxyPIDAlive(pid)
		healthy := false
		if strings.TrimSpace(endpoint) != "" {
			healthy = waitForSTXJavaProxyHealthy(ctx, endpoint, 500*time.Millisecond) == nil
		}
		if !pidAlive && !healthy {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for stx-java-proxy service to stop")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func stxJavaProxyPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "linux" {
		if state, ok := linuxProcessState(pid); ok && (state == "Z" || state == "X") {
			return false
		}
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

// linuxProcessState 读取 /proc/<pid>/stat 并返回 Linux 单字符进程状态。
// linuxProcessState reads /proc/<pid>/stat and returns the one-letter Linux process state.
func linuxProcessState(pid int) (string, bool) {
	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	content, err := os.ReadFile(statPath)
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(content))
	if len(fields) < 3 {
		return "", false
	}
	return fields[2], true
}

// stxJavaProxyPIDMatches 检查进程命令行是否属于 stx-java-proxy。
// stxJavaProxyPIDMatches checks whether a process command line belongs to stx-java-proxy.
func stxJavaProxyPIDMatches(pid int) bool {
	return isSTXJavaProxyCommandLine(stxJavaProxyProcessCommandLine(pid))
}

// stxJavaProxyProcessCommandLine 跨平台读取进程完整命令行；失败返回空串。
// stxJavaProxyProcessCommandLine reads the full process command line cross-platform; empty on failure.
func stxJavaProxyProcessCommandLine(pid int) string {
	if pid <= 0 {
		return ""
	}
	if runtime.GOOS == "linux" {
		cmdlinePath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
		content, err := os.ReadFile(cmdlinePath)
		if err != nil {
			return ""
		}
		return strings.ReplaceAll(string(content), "\x00", " ")
	}

	// macOS / 其他：ps -axww 才能拿到接近完整的 argv（默认 ps 会截断）。
	// macOS / others: ps -axww is required for near-complete argv (default ps truncates).
	cmd := exec.Command("ps", "-axww", "-p", strconv.Itoa(pid), "-o", "args=")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// isSTXJavaProxyCommandLine 判断命令行是否为托管 stx-java-proxy 进程。
// isSTXJavaProxyCommandLine reports whether a command line belongs to managed stx-java-proxy.
func isSTXJavaProxyCommandLine(cmdline string) bool {
	cmdline = strings.TrimSpace(cmdline)
	if cmdline == "" {
		return false
	}
	return strings.Contains(cmdline, "StxJavaProxyApplication") ||
		strings.Contains(cmdline, "stx-java-proxy")
}

// stxJavaProxyPIDForeignForInstallDir 判断进程是否可确认不属于当前 installDir 的托管 proxy：
// 命令行可读且不携带本目录标记（-DSEATUNNEL_HOME/-Dstx.java.proxy.seatunnel.home）即视为他集群进程；
// installDir 为空或命令行不可读时不判外，保持旧行为兼容。
// stxJavaProxyPIDForeignForInstallDir reports whether a process provably belongs to another
// installDir's proxy: a readable cmdline missing this dir's markers counts as foreign; an empty
// installDir or unreadable cmdline stays non-foreign for backward compatibility.
func stxJavaProxyPIDForeignForInstallDir(pid int, installDir string) bool {
	installDir = filepath.Clean(strings.TrimSpace(installDir))
	if installDir == "" || installDir == "." || pid <= 0 {
		return false
	}
	cmdline := stxJavaProxyProcessCommandLine(pid)
	if cmdline == "" {
		return false
	}
	return !stxJavaProxyCommandLineMatchesDesired(cmdline, installDir, "")
}

// stxJavaProxyStatusMatchesDesired 判断当前托管状态是否已对齐目标安装目录与代际 jar。
// stxJavaProxyStatusMatchesDesired reports whether the managed status already matches the desired runtime.
func stxJavaProxyStatusMatchesDesired(status *STXJavaProxyServiceStatus, installDir, seatunnelVersion string) bool {
	if status == nil {
		return false
	}
	cmdline := stxJavaProxyProcessCommandLine(status.PID)
	if cmdline == "" && status.Port > 0 {
		cmdline = stxJavaProxyProcessCommandLine(stxJavaProxyPIDByPort(context.Background(), status.Port))
	}
	return stxJavaProxyCommandLineMatchesDesired(cmdline, installDir, seatunnelVersion)
}

// stxJavaProxyCommandLineMatchesDesired 校验命令行是否指向目标 SEATUNNEL_HOME 与代际 jar。
// stxJavaProxyCommandLineMatchesDesired checks whether cmdline points at the desired SEATUNNEL_HOME and epoch jar.
func stxJavaProxyCommandLineMatchesDesired(cmdline, installDir, seatunnelVersion string) bool {
	if !isSTXJavaProxyCommandLine(cmdline) {
		return false
	}
	installDir = filepath.Clean(strings.TrimSpace(installDir))
	if installDir != "" && installDir != "." {
		homeMarkers := []string{
			"-DSEATUNNEL_HOME=" + installDir,
			"-Dstx.java.proxy.seatunnel.home=" + installDir,
		}
		matchedHome := false
		for _, marker := range homeMarkers {
			if strings.Contains(cmdline, marker) {
				matchedHome = true
				break
			}
		}
		if !matchedHome {
			return false
		}
	}
	if strings.TrimSpace(seatunnelVersion) == "" {
		return true
	}
	jarName := seatunnelmeta.STXJavaProxyJarFileName(seatunnelVersion)
	return strings.Contains(cmdline, jarName)
}

func stxJavaProxyPortFromEndpoint(endpoint string) int {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 {
		return 0
	}
	return port
}

func stxJavaProxyPIDByPort(ctx context.Context, port int) int {
	pids := stxJavaProxyPIDsByPort(ctx, port)
	if len(pids) == 0 {
		return 0
	}
	return pids[0]
}

var ssPIDRegex = regexp.MustCompile(`pid=(\d+)`)

// stxJavaProxyPIDsByPort 返回指定 proxy 端口上的监听进程 PID。
// 优先使用轻量直接命令（ss / lsof），避免拉起高开销的 bash 交互子进程与无效外部命令。
// stxJavaProxyPIDsByPort returns listener PIDs on the given proxy port.
// Uses direct lightweight commands (ss / lsof) first, avoiding expensive login shells and failed probes.
func stxJavaProxyPIDsByPort(ctx context.Context, port int) []int {
	if port <= 0 {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var pids []int

	// 1. Linux 环境下优先直接调用 ss（无须 bash/sed，解析提速 10x 以上）
	// 1. On Linux, prefer calling ss directly without bash or sed (10x+ faster).
	if runtime.GOOS == "linux" {
		cmd := exec.CommandContext(lookupCtx, "ss", "-H", "-ltnp", fmt.Sprintf("sport = :%d", port))
		if output, err := cmd.Output(); err == nil && len(output) > 0 {
			matches := ssPIDRegex.FindAllStringSubmatch(string(output), -1)
			for _, m := range matches {
				if len(m) > 1 {
					if pid, err := strconv.Atoi(m[1]); err == nil && pid > 0 {
						pids = append(pids, pid)
					}
				}
			}
			if len(pids) > 0 {
				return uniquePositivePIDs(pids)
			}
		}
	}

	// 2. Darwin (macOS) 或未通过 ss 命中的场景，检查是否存在 lsof 并直接调用
	// 2. On macOS or when ss did not return any PID, check lsof directly.
	if _, err := exec.LookPath("lsof"); err == nil {
		cmd := exec.CommandContext(lookupCtx, "lsof", "-ti", fmt.Sprintf("tcp:%d", port), "-sTCP:LISTEN")
		if output, err := cmd.Output(); err == nil && len(output) > 0 {
			for _, field := range strings.Fields(strings.TrimSpace(string(output))) {
				if pid, err := strconv.Atoi(field); err == nil && pid > 0 {
					pids = append(pids, pid)
				}
			}
			if len(pids) > 0 {
				return uniquePositivePIDs(pids)
			}
		}
	}

	// 3. 兜底回退：当常规轻量命令均未命中时，使用标准 sh 执行兜底
	// 3. Fallback: if lightweight direct lookups yielded nothing, try standard sh fallback.
	fallbackCmd := exec.CommandContext(lookupCtx, "sh", "-c", fmt.Sprintf("ss -ltnp sport = :%d 2>/dev/null", port))
	if output, err := fallbackCmd.Output(); err == nil && len(output) > 0 {
		matches := ssPIDRegex.FindAllStringSubmatch(string(output), -1)
		for _, m := range matches {
			if len(m) > 1 {
				if pid, err := strconv.Atoi(m[1]); err == nil && pid > 0 {
					pids = append(pids, pid)
				}
			}
		}
	}

	return uniquePositivePIDs(pids)
}

// uniquePositivePIDs 按发现顺序去重正数进程 ID。
// uniquePositivePIDs deduplicates process IDs while preserving discovery order.
func uniquePositivePIDs(pids []int) []int {
	seen := make(map[int]struct{}, len(pids))
	result := make([]int, 0, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		result = append(result, pid)
	}
	return result
}

// defaultSTXJavaProxyVersion 返回给定 SeaTunnel 版本对应的 stx-java-proxy 代际标签。
// 空 version 时回退到内置默认代际（v2）。
// defaultSTXJavaProxyVersion returns the proxy epoch label for the given
// SeaTunnel version, falling back to the built-in default epoch (v2) when blank.
func defaultSTXJavaProxyVersion(version string) string {
	return seatunnelmeta.ProxyEpochForVersion(version)
}

func stxJavaProxyErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
