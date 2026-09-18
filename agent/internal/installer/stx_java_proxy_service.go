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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logger"
	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
)

const stxJavaProxyStopTimeout = 8 * time.Second

// STXJavaProxyServiceStatus describes the current managed stx-java-proxy state.
type STXJavaProxyServiceStatus struct {
	Service  string `json:"service"`
	Managed  bool   `json:"managed"`
	Running  bool   `json:"running"`
	Healthy  bool   `json:"healthy"`
	Endpoint string `json:"endpoint,omitempty"`
	Port     int    `json:"port,omitempty"`
	PID      int    `json:"pid,omitempty"`
	LogPath  string `json:"log_path,omitempty"`
	StateDir string `json:"state_dir,omitempty"`
	Message  string `json:"message,omitempty"`
}

// StartManagedSTXJavaProxyService ensures the managed stx-java-proxy service is available.
// preferredPort>0 时优先使用该端口；否则回退到环境变量 / 落盘端口 / 默认 18080。
// When preferredPort > 0 it is tried first; otherwise env / persisted / default 18080 apply.
func StartManagedSTXJavaProxyService(ctx context.Context, installDir string, seatunnelVersion string, preferredPort int) (*STXJavaProxyServiceStatus, error) {
	status, _ := GetManagedSTXJavaProxyServiceStatus(ctx, installDir)
	// 已健康且端口匹配（或未指定端口）时直接返回，避免无谓重启。
	// Skip restart when already healthy and the port matches (or no preferred port was given).
	if status != nil && status.Healthy {
		if preferredPort <= 0 || status.Port <= 0 || status.Port == preferredPort {
			return status, nil
		}
		// 端口不一致时先停旧实例，再按指定端口启动。
		// Stop the old instance before starting on the preferred port.
		if _, stopErr := StopManagedSTXJavaProxyService(ctx, installDir); stopErr != nil {
			logger.WarnF(ctx, "[stx-java-proxy] stop before port switch failed: preferred=%d, current=%d, error=%v", preferredPort, status.Port, stopErr)
		}
	}

	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, preferredPort)
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
		}, nil
	}
	status.Message = firstNonBlank(status.Message, "stx-java-proxy service started")
	return status, nil
}

// GetManagedSTXJavaProxyServiceStatus returns the current stx-java-proxy state.
func GetManagedSTXJavaProxyServiceStatus(ctx context.Context, installDir string) (*STXJavaProxyServiceStatus, error) {
	status := &STXJavaProxyServiceStatus{
		Service:  "stx_java_proxy",
		Managed:  true,
		StateDir: stxJavaProxyServiceStateDir(installDir),
		LogPath:  filepath.Join(stxJavaProxyServiceStateDir(installDir), "service.log"),
	}

	if endpoint := strings.TrimSpace(os.Getenv(stxJavaProxyEndpointEnvVar)); endpoint != "" {
		normalized := strings.TrimRight(endpoint, "/")
		status.Managed = false
		status.Endpoint = normalized
		if port := stxJavaProxyPortFromEndpoint(normalized); port > 0 {
			status.Port = port
		}
		err := waitForSTXJavaProxyHealthy(ctx, normalized, 1500*time.Millisecond)
		status.Healthy = err == nil
		status.Running = status.Healthy
		if status.Healthy {
			status.Message = "using configured external stx-java-proxy endpoint"
		} else {
			status.Message = firstNonBlank(stxJavaProxyErrorString(err), "configured stx-java-proxy endpoint is unhealthy")
		}
		return status, nil
	}

	if bytes, err := os.ReadFile(filepath.Join(status.StateDir, "service.port")); err == nil {
		if port, ok := parseSTXJavaProxyPort(strings.TrimSpace(string(bytes))); ok {
			status.Port = port
			status.Endpoint = stxJavaProxyServiceBaseURL(port)
		}
	}
	if bytes, err := os.ReadFile(filepath.Join(status.StateDir, "service.pid")); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(bytes))); err == nil && pid > 0 {
			status.PID = pid
		}
	}

	if status.Endpoint == "" {
		for _, port := range stxJavaProxyPortCandidates(status.StateDir, 0) {
			if port <= 0 {
				continue
			}
			endpoint := stxJavaProxyServiceBaseURL(port)
			if err := waitForSTXJavaProxyHealthy(ctx, endpoint, 1200*time.Millisecond); err == nil {
				status.Endpoint = endpoint
				status.Port = port
				status.Healthy = true
				status.Running = true
				_ = os.MkdirAll(status.StateDir, 0o755)
				_ = os.WriteFile(filepath.Join(status.StateDir, "service.port"), []byte(strconv.Itoa(port)+"\n"), 0o644)
				break
			}
		}
	}

	if status.PID <= 0 && status.Port > 0 {
		if pid := stxJavaProxyPIDByPort(ctx, status.Port); pid > 0 {
			status.PID = pid
			_ = os.MkdirAll(status.StateDir, 0o755)
			_ = os.WriteFile(filepath.Join(status.StateDir, "service.pid"), []byte(strconv.Itoa(pid)+"\n"), 0o644)
		}
	}
	if status.PID > 0 {
		status.Running = stxJavaProxyPIDAlive(status.PID)
	}
	if status.Endpoint != "" && !status.Healthy {
		if err := waitForSTXJavaProxyHealthy(ctx, status.Endpoint, 1500*time.Millisecond); err == nil {
			status.Healthy = true
			status.Running = true
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
	pidFile := filepath.Join(stateDir, "service.pid")
	portFile := filepath.Join(stateDir, "service.port")
	if !fileExists(pidFile) && !fileExists(portFile) {
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
		if killErr := forceKillSTXJavaProxyProcesses(ctx, status); killErr != nil {
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
func forceKillSTXJavaProxyProcesses(ctx context.Context, status *STXJavaProxyServiceStatus) error {
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
	cmdlinePath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	content, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}
	cmdline := strings.ReplaceAll(string(content), "\x00", " ")
	return strings.Contains(cmdline, "StxJavaProxyApplication") ||
		strings.Contains(cmdline, "stx-java-proxy")
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

// stxJavaProxyPIDsByPort 返回指定 proxy 端口上的监听进程 PID。
// stxJavaProxyPIDsByPort returns listener PIDs on the given proxy port.
func stxJavaProxyPIDsByPort(ctx context.Context, port int) []int {
	if port <= 0 {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	commands := []string{
		fmt.Sprintf("lsof -ti tcp:%d -sTCP:LISTEN", port),
		fmt.Sprintf(`ss -ltnp '( sport = :%d )' | sed -n '2,$p' | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p'`, port),
	}
	var pids []int
	for _, shellCmd := range commands {
		cmd := exec.CommandContext(lookupCtx, "bash", "-lc", shellCmd)
		output, err := cmd.Output()
		if err != nil {
			continue
		}
		for _, field := range strings.Fields(strings.TrimSpace(string(output))) {
			pid, err := strconv.Atoi(field)
			if err == nil && pid > 0 {
				pids = append(pids, pid)
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

func defaultSTXJavaProxyVersion(version string) string {
	return firstNonBlank(strings.TrimSpace(version), seatunnelmeta.DefaultSTXJavaProxyVersion)
}

func stxJavaProxyErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
