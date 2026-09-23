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

// Package process provides SeaTunnel process lifecycle management for the Agent.
// process 包提供 Agent 的 SeaTunnel 进程生命周期管理功能。
//
// This package provides:
// 此包提供：
// - Start, Stop, Restart methods / 启动、停止、重启方法
// - Process status monitoring / 进程状态监控
// - Graceful shutdown with timeout / 带超时的优雅关闭
// - Process health checking / 进程健康检查
package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logger"
)

// Common errors for process management
// 进程管理的常见错误
var (
	// ErrProcessNotFound indicates the process was not found
	// ErrProcessNotFound 表示进程未找到
	ErrProcessNotFound = errors.New("process not found")

	// ErrProcessAlreadyRunning indicates the process is already running
	// ErrProcessAlreadyRunning 表示进程已在运行
	ErrProcessAlreadyRunning = errors.New("process is already running")

	// ErrProcessNotRunning indicates the process is not running
	// ErrProcessNotRunning 表示进程未运行
	ErrProcessNotRunning = errors.New("process is not running")

	// ErrStartFailed indicates the process failed to start
	// ErrStartFailed 表示进程启动失败
	ErrStartFailed = errors.New("process failed to start")

	// ErrStopFailed indicates the process failed to stop
	// ErrStopFailed 表示进程停止失败
	ErrStopFailed = errors.New("process failed to stop")

	// ErrStopTimeout indicates the process stop timed out
	// ErrStopTimeout 表示进程停止超时
	ErrStopTimeout = errors.New("process stop timed out")

	// ErrInvalidInstallDir indicates an invalid installation directory
	// ErrInvalidInstallDir 表示无效的安装目录
	ErrInvalidInstallDir = errors.New("invalid installation directory")

	// ErrManagerNotInitialized indicates the manager is not initialized
	// ErrManagerNotInitialized 表示管理器未初始化
	ErrManagerNotInitialized = errors.New("process manager not initialized")
)

// ProcessStatus represents the status of a managed process
// ProcessStatus 表示托管进程的状态
type ProcessStatus string

const (
	// StatusRunning indicates the process is running
	// StatusRunning 表示进程正在运行
	StatusRunning ProcessStatus = "running"

	// StatusStopped indicates the process is stopped
	// StatusStopped 表示进程已停止
	StatusStopped ProcessStatus = "stopped"

	// StatusStarting indicates the process is starting
	// StatusStarting 表示进程正在启动
	StatusStarting ProcessStatus = "starting"

	// StatusStopping indicates the process is stopping
	// StatusStopping 表示进程正在停止
	StatusStopping ProcessStatus = "stopping"

	// StatusUnknown indicates the process status is unknown
	// StatusUnknown 表示进程状态未知
	StatusUnknown ProcessStatus = "unknown"

	// StatusError indicates the process encountered an error
	// StatusError 表示进程遇到错误
	StatusError ProcessStatus = "error"
)

// Default configuration values
// 默认配置值
const (
	// DefaultGracefulTimeout is the default timeout for graceful shutdown (30 seconds)
	// DefaultGracefulTimeout 是优雅关闭的默认超时时间（30秒）
	DefaultGracefulTimeout = 30 * time.Second

	// DefaultMonitorInterval is the default interval for process monitoring (5 seconds)
	// DefaultMonitorInterval 是进程监控的默认间隔（5秒）
	DefaultMonitorInterval = 5 * time.Second

	// DefaultStartTimeout is the default timeout for process startup
	// DefaultStartTimeout 是进程启动的默认超时时间
	DefaultStartTimeout = 60 * time.Second

	// DefaultLogTailLines is the default number of log lines to collect on failure
	// DefaultLogTailLines 是失败时收集的默认日志行数
	DefaultLogTailLines = 100
)

// ManagedProcess represents a process managed by the ProcessManager
// ManagedProcess 表示由 ProcessManager 管理的进程
type ManagedProcess struct {
	// Name is the name of the process
	// Name 是进程的名称
	Name string `json:"name"`

	// PID is the process ID
	// PID 是进程 ID
	PID int `json:"pid"`

	// Status is the current status of the process
	// Status 是进程的当前状态
	Status ProcessStatus `json:"status"`

	// StartTime is when the process was started
	// StartTime 是进程启动的时间
	StartTime time.Time `json:"start_time"`

	// Uptime is the duration the process has been running
	// Uptime 是进程运行的持续时间
	Uptime time.Duration `json:"uptime"`

	// CPUUsage is the CPU usage percentage (0-100)
	// CPUUsage 是 CPU 使用率百分比（0-100）
	CPUUsage float64 `json:"cpu_usage"`

	// MemoryUsage is the memory usage in bytes
	// MemoryUsage 是内存使用量（字节）
	MemoryUsage int64 `json:"memory_usage"`

	// InstallDir is the installation directory
	// InstallDir 是安装目录
	InstallDir string `json:"install_dir"`

	// Role is the SeaTunnel node role used to isolate processes in the same install directory.
	// Role 是 SeaTunnel 节点角色，用于隔离同一安装目录中的不同进程。
	Role string `json:"role,omitempty"`

	// LastError is the last error encountered
	// LastError 是最后遇到的错误
	LastError string `json:"last_error,omitempty"`

	// cmd is the underlying exec.Cmd (internal use)
	// cmd 是底层的 exec.Cmd（内部使用）
	cmd *exec.Cmd

	// mu protects the process state
	// mu 保护进程状态
	mu sync.RWMutex
}

// ProcessInfo contains information about a process for external use
// ProcessInfo 包含用于外部使用的进程信息
type ProcessInfo struct {
	Name        string        `json:"name"`
	PID         int           `json:"pid"`
	Status      ProcessStatus `json:"status"`
	StartTime   time.Time     `json:"start_time"`
	Uptime      time.Duration `json:"uptime"`
	CPUUsage    float64       `json:"cpu_usage"`
	MemoryUsage int64         `json:"memory_usage"`
	InstallDir  string        `json:"install_dir"`
	Role        string        `json:"role,omitempty"`
	LastError   string        `json:"last_error,omitempty"`
}

// StartParams contains parameters for starting a process
// StartParams 包含启动进程的参数
type StartParams struct {
	// InstallDir is the SeaTunnel installation directory
	// InstallDir 是 SeaTunnel 安装目录
	InstallDir string `json:"install_dir"`

	// Role is the node role: "master", "worker", or empty for hybrid mode
	// Role 是节点角色："master"、"worker" 或空表示混合模式
	Role string `json:"role,omitempty"`

	// ConfigDir is the configuration directory (optional, defaults to InstallDir/config)
	// ConfigDir 是配置目录（可选，默认为 InstallDir/config）
	ConfigDir string `json:"config_dir,omitempty"`

	// LogDir is the log directory (optional, defaults to InstallDir/logs)
	// LogDir 是日志目录（可选，默认为 InstallDir/logs）
	LogDir string `json:"log_dir,omitempty"`

	// JVMOptions are additional JVM options
	// JVMOptions 是额外的 JVM 选项
	JVMOptions []string `json:"jvm_options,omitempty"`

	// Environment variables to set
	// 要设置的环境变量
	Environment map[string]string `json:"environment,omitempty"`

	// Timeout for startup (optional, defaults to DefaultStartTimeout)
	// 启动超时时间（可选，默认为 DefaultStartTimeout）
	Timeout time.Duration `json:"timeout,omitempty"`
}

// StopParams contains parameters for stopping a process
// StopParams 包含停止进程的参数
type StopParams struct {
	// Graceful indicates whether to attempt graceful shutdown first
	// Graceful 表示是否首先尝试优雅关闭
	Graceful bool `json:"graceful"`

	// Timeout is the timeout for graceful shutdown (defaults to DefaultGracefulTimeout)
	// Timeout 是优雅关闭的超时时间（默认为 DefaultGracefulTimeout）
	Timeout time.Duration `json:"timeout,omitempty"`

	// InstallDir is the SeaTunnel installation directory (for stop script)
	// InstallDir 是 SeaTunnel 安装目录（用于停止脚本）
	InstallDir string `json:"install_dir,omitempty"`

	// Role is the node role: "master", "worker", or empty for hybrid mode
	// Role 是节点角色："master"、"worker" 或空表示混合模式
	Role string `json:"role,omitempty"`
}

// ProcessEventHandler is a callback for process events
// ProcessEventHandler 是进程事件的回调
type ProcessEventHandler func(name string, event ProcessEvent, info *ProcessInfo)

// ProcessEvent represents a process lifecycle event
// ProcessEvent 表示进程生命周期事件
type ProcessEvent string

const (
	// EventStarted indicates the process has started
	// EventStarted 表示进程已启动
	EventStarted ProcessEvent = "started"

	// EventStopped indicates the process has stopped
	// EventStopped 表示进程已停止
	EventStopped ProcessEvent = "stopped"

	// EventCrashed indicates the process has crashed unexpectedly
	// EventCrashed 表示进程意外崩溃
	EventCrashed ProcessEvent = "crashed"

	// EventHealthy indicates the process is healthy
	// EventHealthy 表示进程健康
	EventHealthy ProcessEvent = "healthy"

	// EventUnhealthy indicates the process is unhealthy
	// EventUnhealthy 表示进程不健康
	EventUnhealthy ProcessEvent = "unhealthy"
)

// ProcessManager manages SeaTunnel process lifecycle
// ProcessManager 管理 SeaTunnel 进程生命周期
type ProcessManager struct {
	// processes stores managed processes by name
	// processes 按名称存储托管进程
	processes sync.Map

	// monitorCtx is the context for the monitor goroutine
	// monitorCtx 是监控 goroutine 的上下文
	monitorCtx context.Context

	// monitorCancel cancels the monitor goroutine
	// monitorCancel 取消监控 goroutine
	monitorCancel context.CancelFunc

	// monitorInterval is the interval for process monitoring
	// monitorInterval 是进程监控的间隔
	monitorInterval time.Duration

	// gracefulTimeout is the timeout for graceful shutdown
	// gracefulTimeout 是优雅关闭的超时时间
	gracefulTimeout time.Duration

	// eventHandler is called when process events occur
	// eventHandler 在进程事件发生时被调用
	eventHandler ProcessEventHandler

	// mu protects manager state
	// mu 保护管理器状态
	mu sync.RWMutex

	// running indicates if the manager is running
	// running 表示管理器是否正在运行
	running bool
}

// NewProcessManager creates a new ProcessManager instance
// NewProcessManager 创建一个新的 ProcessManager 实例
func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		monitorInterval: DefaultMonitorInterval,
		gracefulTimeout: DefaultGracefulTimeout,
	}
}

// SetMonitorInterval sets the monitoring interval
// SetMonitorInterval 设置监控间隔
func (m *ProcessManager) SetMonitorInterval(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.monitorInterval = interval
}

// SetGracefulTimeout sets the graceful shutdown timeout
// SetGracefulTimeout 设置优雅关闭超时时间
func (m *ProcessManager) SetGracefulTimeout(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gracefulTimeout = timeout
}

// SetEventHandler sets the event handler callback
// SetEventHandler 设置事件处理回调
func (m *ProcessManager) SetEventHandler(handler ProcessEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventHandler = handler
}

// Start starts the process manager and begins monitoring
// Start 启动进程管理器并开始监控
func (m *ProcessManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil // Already running / 已经在运行
	}

	m.monitorCtx, m.monitorCancel = context.WithCancel(ctx)
	m.running = true

	// Start the monitor goroutine / 启动监控 goroutine
	go m.monitorLoop()

	return nil
}

// Stop stops the process manager
// Stop 停止进程管理器
func (m *ProcessManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	if m.monitorCancel != nil {
		m.monitorCancel()
	}
	m.running = false

	return nil
}

// monitorLoop is the main monitoring loop
// monitorLoop 是主监控循环
func (m *ProcessManager) monitorLoop() {
	ticker := time.NewTicker(m.monitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.monitorCtx.Done():
			return
		case <-ticker.C:
			m.checkAllProcesses()
		}
	}
}

// checkAllProcesses checks the status of all managed processes
// checkAllProcesses 检查所有托管进程的状态
func (m *ProcessManager) checkAllProcesses() {
	m.processes.Range(func(key, value interface{}) bool {
		name := key.(string)
		proc := value.(*ManagedProcess)

		proc.mu.Lock()
		defer proc.mu.Unlock()

		// Skip if not running / 如果未运行则跳过
		if proc.Status != StatusRunning {
			return true
		}

		// Check if process is still alive / 检查进程是否仍然存活
		if proc.PID > 0 {
			alive := isProcessAlive(proc.PID)
			if !alive {
				// Process has crashed / 进程已崩溃
				proc.Status = StatusStopped
				proc.LastError = "Process exited unexpectedly / 进程意外退出"
				m.notifyEvent(name, EventCrashed, proc)
			} else {
				// Update metrics / 更新指标
				proc.Uptime = time.Since(proc.StartTime)
				cpu, mem := getProcessMetrics(proc.PID)
				proc.CPUUsage = cpu
				proc.MemoryUsage = mem
			}
		}

		return true
	})
}

// notifyEvent notifies the event handler of a process event
// notifyEvent 通知事件处理程序进程事件
func (m *ProcessManager) notifyEvent(name string, event ProcessEvent, proc *ManagedProcess) {
	m.mu.RLock()
	handler := m.eventHandler
	m.mu.RUnlock()

	if handler != nil {
		info := &ProcessInfo{
			Name:        proc.Name,
			PID:         proc.PID,
			Status:      proc.Status,
			StartTime:   proc.StartTime,
			Uptime:      proc.Uptime,
			CPUUsage:    proc.CPUUsage,
			MemoryUsage: proc.MemoryUsage,
			InstallDir:  proc.InstallDir,
			Role:        proc.Role,
			LastError:   proc.LastError,
		}
		handler(name, event, info)
	}
}

// StartProcess starts a SeaTunnel process
// StartProcess 启动 SeaTunnel 进程
// Requirements 6.1: Execute SeaTunnel startup script, wait for process to start, verify process is alive
// 需求 6.1：执行 SeaTunnel 启动脚本、等待进程启动完成、验证进程存活
func (m *ProcessManager) StartProcess(ctx context.Context, name string, params *StartParams) error {
	if params == nil {
		return errors.New("start params is nil")
	}

	if params.InstallDir == "" {
		return ErrInvalidInstallDir
	}

	// Check if process already exists and is running
	// 检查进程是否已存在且正在运行
	if existing, ok := m.processes.Load(name); ok {
		proc := existing.(*ManagedProcess)
		proc.mu.RLock()
		status := proc.Status
		proc.mu.RUnlock()

		if status == StatusRunning || status == StatusStarting {
			return ErrProcessAlreadyRunning
		}
	}

	// Validate installation directory / 验证安装目录
	startScript := getStartScript(params.InstallDir)
	if _, err := os.Stat(startScript); os.IsNotExist(err) {
		return fmt.Errorf("%w: start script not found at %s", ErrInvalidInstallDir, startScript)
	}

	// Create managed process / 创建托管进程
	proc := &ManagedProcess{
		Name:       name,
		Status:     StatusStarting,
		InstallDir: params.InstallDir,
		Role:       params.Role,
	}
	m.processes.Store(name, proc)

	// Set timeout / 设置超时
	timeout := params.Timeout
	if timeout == 0 {
		timeout = DefaultStartTimeout
	}

	startCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command / 构建命令
	cmd := buildStartCommand(startCtx, params)

	// Set up log capture / 设置日志捕获
	logDir := params.LogDir
	if logDir == "" {
		logDir = filepath.Join(params.InstallDir, "logs")
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		proc.mu.Lock()
		proc.Status = StatusError
		proc.LastError = fmt.Sprintf("Failed to create log directory: %v / 创建日志目录失败：%v", err, err)
		proc.mu.Unlock()
		return fmt.Errorf("%w: %v", ErrStartFailed, err)
	}

	// Create log file / 创建日志文件
	logFile := filepath.Join(logDir, fmt.Sprintf("seatunnel-%s.log", time.Now().Format("20060102-150405")))
	logWriter, err := os.Create(logFile)
	if err != nil {
		proc.mu.Lock()
		proc.Status = StatusError
		proc.LastError = fmt.Sprintf("Failed to create log file: %v / 创建日志文件失败：%v", err, err)
		proc.mu.Unlock()
		return fmt.Errorf("%w: %v", ErrStartFailed, err)
	}

	// Capture stdout and stderr / 捕获标准输出和标准错误
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	// Start the process / 启动进程
	if err := cmd.Start(); err != nil {
		logWriter.Close()
		proc.mu.Lock()
		proc.Status = StatusError
		proc.LastError = fmt.Sprintf("Failed to start process: %v / 启动进程失败：%v", err, err)
		proc.mu.Unlock()
		return fmt.Errorf("%w: %v", ErrStartFailed, err)
	}

	// Update process info / 更新进程信息
	proc.mu.Lock()
	proc.cmd = cmd
	proc.StartTime = time.Now()
	proc.mu.Unlock()

	// Wait for the startup script to complete
	// 等待启动脚本完成
	// The script with -d flag will fork a daemon process and exit
	// 使用 -d 参数的脚本会 fork 一个守护进程然后退出
	err = cmd.Wait()
	logWriter.Close()

	if err != nil {
		// Script failed / 脚本失败
		logs := collectStartupLogs(logFile, DefaultLogTailLines)
		proc.mu.Lock()
		proc.Status = StatusError
		proc.LastError = fmt.Sprintf("Start script failed: %v. Logs:\n%s / 启动脚本失败: %v。日志：\n%s", err, logs, err, logs)
		proc.mu.Unlock()
		return fmt.Errorf("%w: start script failed: %v", ErrStartFailed, err)
	}

	// Wait a bit for the daemon process to start
	// 等待守护进程启动
	time.Sleep(3 * time.Second)

	// Find the actual SeaTunnel process by searching for Java process
	// 通过搜索 Java 进程找到实际的 SeaTunnel 进程
	pid, err := findSeaTunnelProcess(params.InstallDir, params.Role)
	if err != nil || pid <= 0 {
		// Try to get logs for debugging / 尝试获取日志用于调试
		logs := collectStartupLogs(logFile, DefaultLogTailLines)
		proc.mu.Lock()
		proc.Status = StatusError
		proc.LastError = fmt.Sprintf("SeaTunnel process not found after start. Logs:\n%s / 启动后未找到 SeaTunnel 进程。日志：\n%s", logs, logs)
		proc.mu.Unlock()
		return fmt.Errorf("%w: SeaTunnel process not found after start", ErrStartFailed)
	}

	// Process started successfully / 进程启动成功
	proc.mu.Lock()
	proc.PID = pid
	proc.Status = StatusRunning
	proc.LastError = ""
	proc.mu.Unlock()

	m.notifyEvent(name, EventStarted, proc)

	// Start a goroutine to monitor the process
	// 启动一个 goroutine 监控进程
	go m.monitorProcess(name, proc)

	return nil
}

// findSeaTunnelProcess finds the SeaTunnel Java process by install dir and role
// findSeaTunnelProcess 通过安装目录和角色查找 SeaTunnel Java 进程
func findSeaTunnelProcess(installDir string, role string) (int, error) {
	pid, _, err := FindSeaTunnelProcess(context.Background(), installDir, role)
	return pid, err
}

// monitorProcess monitors a process and updates status when it exits
// monitorProcess 监控进程并在退出时更新状态
func (m *ProcessManager) monitorProcess(name string, proc *ManagedProcess) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		proc.mu.RLock()
		pid := proc.PID
		status := proc.Status
		proc.mu.RUnlock()

		if status != StatusRunning {
			return
		}

		if pid <= 0 || !isProcessAlive(pid) {
			proc.mu.Lock()
			proc.Status = StatusStopped
			proc.PID = 0
			proc.mu.Unlock()
			m.notifyEvent(name, EventCrashed, proc)
			return
		}
	}
}

// StopProcess stops a SeaTunnel process
// StopProcess 停止 SeaTunnel 进程
// Requirements 6.2: Send SIGTERM, wait for graceful shutdown (max 30s), send SIGKILL if timeout
// 需求 6.2：发送 SIGTERM 信号、等待进程优雅关闭（最长 30 秒）、若超时则发送 SIGKILL
func (m *ProcessManager) StopProcess(ctx context.Context, name string, params *StopParams) error {
	// Set timeout / 设置超时
	timeout := m.gracefulTimeout
	if params != nil && params.Timeout > 0 {
		timeout = params.Timeout
	}

	// Resolve the exact install directory and role before scanning processes.
	// 扫描进程前先确定准确的安装目录和角色。
	installDir := ""
	role := ""
	if params != nil {
		installDir = strings.TrimSpace(params.InstallDir)
		role = params.Role
	}
	if value, ok := m.processes.Load(name); ok {
		proc := value.(*ManagedProcess)
		proc.mu.RLock()
		if installDir == "" {
			installDir = strings.TrimSpace(proc.InstallDir)
		}
		if strings.TrimSpace(role) == "" {
			role = proc.Role
		}
		proc.mu.RUnlock()
	}
	if installDir == "" {
		return ErrInvalidInstallDir
	}

	processes, err := listSeaTunnelProcesses(ctx)
	if err != nil {
		return fmt.Errorf("%w: failed to inspect SeaTunnel processes: %v", ErrStopFailed, err)
	}
	pids := make([]int, 0)
	for _, proc := range processes {
		if matchesManagedSeaTunnelProcess(proc.CommandLine, installDir, role) {
			pids = append(pids, proc.PID)
		}
	}

	if len(pids) == 0 {
		logger.InfoF(ctx, "No SeaTunnel process found for install dir '%s' and role '%s' / 未找到安装目录 '%s'、角色 '%s' 的 SeaTunnel 进程", installDir, role, installDir, role)
		// Update tracked process status / 更新跟踪的进程状态
		if value, ok := m.processes.Load(name); ok {
			proc := value.(*ManagedProcess)
			proc.mu.Lock()
			proc.Status = StatusStopped
			proc.PID = 0
			proc.mu.Unlock()
		}
		return nil
	}

	// Send SIGTERM to all found processes / 向所有找到的进程发送 SIGTERM
	for _, pid := range pids {
		if err := sendSignal(pid, syscall.SIGTERM); err == nil {
			logger.InfoF(ctx, "Sent SIGTERM to process %d / 向进程 %d 发送 SIGTERM", pid, pid)
		}
	}

	// Wait for processes to exit gracefully / 等待进程优雅退出
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allDead := true
		for _, pid := range pids {
			if isProcessAlive(pid) {
				allDead = false
				break
			}
		}
		if allDead {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill any remaining processes / 强制杀死任何剩余的进程
	for _, pid := range pids {
		if isProcessAlive(pid) {
			_ = sendSignal(pid, syscall.SIGKILL)
			logger.WarnF(ctx, "Sent SIGKILL to process %d / 向进程 %d 发送 SIGKILL", pid, pid)
		}
	}

	// Update tracked process if exists / 如果存在则更新跟踪的进程
	if value, ok := m.processes.Load(name); ok {
		proc := value.(*ManagedProcess)
		proc.mu.Lock()
		proc.Status = StatusStopped
		proc.PID = 0
		proc.mu.Unlock()
		m.notifyEvent(name, EventStopped, proc)
	}

	logger.InfoF(ctx, "Stopped %d SeaTunnel process(es) for install dir '%s' and role '%s' / 停止了 %d 个安装目录 '%s'、角色 '%s' 的 SeaTunnel 进程", len(pids), installDir, role, len(pids), installDir, role)
	return nil
}

// RestartProcess restarts a SeaTunnel process
// RestartProcess 重启 SeaTunnel 进程
// Requirements 6.3: Stop first, wait for complete exit, then start
// 需求 6.3：先执行停止操作、等待进程完全退出、再执行启动操作
func (m *ProcessManager) RestartProcess(ctx context.Context, name string, startParams *StartParams, stopParams *StopParams) error {
	// Always stop the exact managed process before starting it again.
	// 重启前先准确停止对应的受管进程。
	if err := m.StopProcess(ctx, name, stopParams); err != nil {
		return err
	}
	// Wait for process to fully exit / 等待进程完全退出
	time.Sleep(3 * time.Second)

	// Start the process / 启动进程
	return m.StartProcess(ctx, name, startParams)
}

// GetStatus returns the status of a managed process
// GetStatus 返回托管进程的状态
// Requirements 6.5: Return process PID, uptime, CPU usage, memory usage, start time
// 需求 6.5：返回进程 PID、运行时长、CPU 使用率、内存使用量、启动时间
func (m *ProcessManager) GetStatus(ctx context.Context, name string) (*ProcessInfo, error) {
	value, ok := m.processes.Load(name)
	if !ok {
		return nil, ErrProcessNotFound
	}

	proc := value.(*ManagedProcess)
	proc.mu.RLock()
	defer proc.mu.RUnlock()

	// Update metrics if running / 如果正在运行则更新指标
	if proc.Status == StatusRunning && proc.PID > 0 {
		if isProcessAlive(proc.PID) {
			proc.Uptime = time.Since(proc.StartTime)
			cpu, mem := getProcessMetrics(proc.PID)
			proc.CPUUsage = cpu
			proc.MemoryUsage = mem
		} else {
			// Process died / 进程已死亡
			proc.Status = StatusStopped
		}
	}

	return &ProcessInfo{
		Name:        proc.Name,
		PID:         proc.PID,
		Status:      proc.Status,
		StartTime:   proc.StartTime,
		Uptime:      proc.Uptime,
		CPUUsage:    proc.CPUUsage,
		MemoryUsage: proc.MemoryUsage,
		InstallDir:  proc.InstallDir,
		Role:        proc.Role,
		LastError:   proc.LastError,
	}, nil
}

// ListProcesses returns information about all managed processes
// ListProcesses 返回所有托管进程的信息
func (m *ProcessManager) ListProcesses() []*ProcessInfo {
	var processes []*ProcessInfo

	m.processes.Range(func(key, value interface{}) bool {
		proc := value.(*ManagedProcess)
		proc.mu.RLock()
		info := &ProcessInfo{
			Name:        proc.Name,
			PID:         proc.PID,
			Status:      proc.Status,
			StartTime:   proc.StartTime,
			Uptime:      proc.Uptime,
			CPUUsage:    proc.CPUUsage,
			MemoryUsage: proc.MemoryUsage,
			InstallDir:  proc.InstallDir,
			Role:        proc.Role,
			LastError:   proc.LastError,
		}
		proc.mu.RUnlock()
		processes = append(processes, info)
		return true
	})

	return processes
}

// RemoveProcess removes a process from management (does not stop it)
// RemoveProcess 从管理中移除进程（不停止它）
func (m *ProcessManager) RemoveProcess(name string) {
	m.processes.Delete(name)
}

// IsRunning checks if a process is running
// IsRunning 检查进程是否正在运行
func (m *ProcessManager) IsRunning(name string) bool {
	value, ok := m.processes.Load(name)
	if !ok {
		return false
	}

	proc := value.(*ManagedProcess)
	proc.mu.RLock()
	defer proc.mu.RUnlock()

	return proc.Status == StatusRunning && proc.PID > 0 && isProcessAlive(proc.PID)
}

// StopAll stops all managed processes
// StopAll 停止所有托管进程
func (m *ProcessManager) StopAll(ctx context.Context) error {
	var lastErr error

	m.processes.Range(func(key, value interface{}) bool {
		name := key.(string)
		proc := value.(*ManagedProcess)

		proc.mu.RLock()
		status := proc.Status
		proc.mu.RUnlock()

		if status == StatusRunning {
			if err := m.StopProcess(ctx, name, &StopParams{Graceful: true, InstallDir: proc.InstallDir, Role: proc.Role}); err != nil {
				lastErr = err
			}
		}
		return true
	})

	return lastErr
}

// Helper functions / 辅助函数

// getStartScript returns the path to the SeaTunnel start script
// getStartScript 返回 SeaTunnel 启动脚本的路径
func getStartScript(installDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(installDir, "bin", "seatunnel-cluster.cmd")
	}
	return filepath.Join(installDir, "bin", "seatunnel-cluster.sh")
}

// getStopScript returns the path to the SeaTunnel stop script
// getStopScript 返回 SeaTunnel 停止脚本的路径
func getStopScript(installDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(installDir, "bin", "stop-seatunnel-cluster.cmd")
	}
	return filepath.Join(installDir, "bin", "stop-seatunnel-cluster.sh")
}

// buildStartCommand builds the command to start SeaTunnel
// buildStartCommand 构建启动 SeaTunnel 的命令
func buildStartCommand(ctx context.Context, params *StartParams) *exec.Cmd {
	startScript := getStartScript(params.InstallDir)

	// Build arguments based on role / 根据角色构建参数
	// -d: daemon mode / 守护进程模式
	// -r: role (master/worker for separated mode) / 角色（分离模式下的 master/worker）
	// For hybrid mode (empty, "hybrid", or "master/worker"), don't pass -r flag
	// 对于混合模式（空、"hybrid" 或 "master/worker"），不传 -r 参数
	args := []string{startScript, "-d"}
	if params.Role != "" && params.Role != "hybrid" && params.Role != "master/worker" {
		args = append(args, "-r", params.Role)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmdArgs := append([]string{"/c"}, args...)
		cmd = exec.CommandContext(ctx, "cmd", cmdArgs...)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/bash", args...)
		// Set process group so SeaTunnel process is independent of Agent
		// 设置进程组，使 SeaTunnel 进程独立于 Agent
		// This ensures Agent restart won't kill SeaTunnel processes
		// 这确保 Agent 重启不会杀死 SeaTunnel 进程
		setProcGroupAttr(cmd)
	}

	// Set working directory / 设置工作目录
	cmd.Dir = params.InstallDir

	// Set environment variables / 设置环境变量
	cmd.Env = os.Environ()

	// Add SEATUNNEL_HOME / 添加 SEATUNNEL_HOME
	cmd.Env = append(cmd.Env, fmt.Sprintf("SEATUNNEL_HOME=%s", params.InstallDir))

	// Add config directory / 添加配置目录
	if params.ConfigDir != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SEATUNNEL_CONFIG=%s", params.ConfigDir))
	}

	// Add custom environment variables / 添加自定义环境变量
	for k, v := range params.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add JVM options / 添加 JVM 选项
	if len(params.JVMOptions) > 0 {
		jvmOpts := strings.Join(params.JVMOptions, " ")
		cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_OPTS=%s", jvmOpts))
	}

	return cmd
}

// isProcessAlive checks if a process with the given PID is alive
// isProcessAlive 检查给定 PID 的进程是否存活
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0 to check
	// 在 Unix 上，FindProcess 总是成功，所以我们需要发送信号 0 来检查
	if runtime.GOOS != "windows" {
		err = process.Signal(syscall.Signal(0))
		return err == nil
	}

	// On Windows, we need a different approach
	// 在 Windows 上，我们需要不同的方法
	return checkProcessWindows(pid)
}

// checkProcessWindows checks if a process is alive on Windows
// checkProcessWindows 在 Windows 上检查进程是否存活
func checkProcessWindows(pid int) bool {
	// Use tasklist command to check if process exists
	// 使用 tasklist 命令检查进程是否存在
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), strconv.Itoa(pid))
}

// sendSignal sends a signal to a process
// sendSignal 向进程发送信号
func sendSignal(pid int, sig syscall.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		// On Windows, we can only kill the process
		// 在 Windows 上，我们只能终止进程
		if sig == syscall.SIGKILL || sig == syscall.SIGTERM {
			return process.Kill()
		}
		return nil
	}

	return process.Signal(sig)
}

// getProcessMetrics gets CPU and memory usage for a process
// getProcessMetrics 获取进程的 CPU 和内存使用率
func getProcessMetrics(pid int) (cpuUsage float64, memoryUsage int64) {
	if runtime.GOOS == "linux" {
		return getProcessMetricsLinux(pid)
	} else if runtime.GOOS == "darwin" {
		return getProcessMetricsDarwin(pid)
	} else if runtime.GOOS == "windows" {
		return getProcessMetricsWindows(pid)
	}
	return 0, 0
}

// getProcessMetricsLinux gets process metrics on Linux
// getProcessMetricsLinux 在 Linux 上获取进程指标
func getProcessMetricsLinux(pid int) (cpuUsage float64, memoryUsage int64) {
	// Read /proc/[pid]/stat for CPU info
	// 读取 /proc/[pid]/stat 获取 CPU 信息
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return 0, 0
	}

	// Parse stat file / 解析 stat 文件
	fields := strings.Fields(string(statData))
	if len(fields) < 24 {
		return 0, 0
	}

	// Read /proc/[pid]/statm for memory info
	// 读取 /proc/[pid]/statm 获取内存信息
	statmPath := fmt.Sprintf("/proc/%d/statm", pid)
	statmData, err := os.ReadFile(statmPath)
	if err != nil {
		return 0, 0
	}

	statmFields := strings.Fields(string(statmData))
	if len(statmFields) >= 2 {
		// RSS is in pages, convert to bytes (assuming 4KB pages)
		// RSS 以页为单位，转换为字节（假设 4KB 页）
		rss, _ := strconv.ParseInt(statmFields[1], 10, 64)
		memoryUsage = rss * 4096
	}

	// CPU usage calculation would require sampling over time
	// CPU 使用率计算需要随时间采样
	// For now, return 0 as a placeholder
	// 目前返回 0 作为占位符
	return 0, memoryUsage
}

// getProcessMetricsDarwin gets process metrics on macOS
// getProcessMetricsDarwin 在 macOS 上获取进程指标
func getProcessMetricsDarwin(pid int) (cpuUsage float64, memoryUsage int64) {
	// Use ps command to get process info
	// 使用 ps 命令获取进程信息
	cmd := exec.Command("ps", "-o", "rss=,pcpu=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	fields := strings.Fields(string(output))
	if len(fields) >= 2 {
		// RSS is in KB, convert to bytes
		// RSS 以 KB 为单位，转换为字节
		rss, _ := strconv.ParseInt(fields[0], 10, 64)
		memoryUsage = rss * 1024

		// CPU percentage
		// CPU 百分比
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		cpuUsage = cpu
	}

	return cpuUsage, memoryUsage
}

// getProcessMetricsWindows gets process metrics on Windows
// getProcessMetricsWindows 在 Windows 上获取进程指标
func getProcessMetricsWindows(pid int) (cpuUsage float64, memoryUsage int64) {
	// Use wmic command to get process info
	// 使用 wmic 命令获取进程信息
	cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", pid), "get", "WorkingSetSize", "/value")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	// Parse output / 解析输出
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "WorkingSetSize=") {
			value := strings.TrimPrefix(line, "WorkingSetSize=")
			value = strings.TrimSpace(value)
			mem, _ := strconv.ParseInt(value, 10, 64)
			memoryUsage = mem
		}
	}

	return 0, memoryUsage
}

// collectStartupLogs collects the last N lines from a log file
// collectStartupLogs 从日志文件收集最后 N 行
// Requirements 6.6: Collect startup logs and analyze failure reason
// 需求 6.6：收集启动日志、分析失败原因
func collectStartupLogs(logFile string, lines int) string {
	file, err := os.Open(logFile)
	if err != nil {
		return fmt.Sprintf("Failed to open log file: %v / 打开日志文件失败：%v", err, err)
	}
	defer file.Close()

	// Read all lines / 读取所有行
	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	// Get last N lines / 获取最后 N 行
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}

	return strings.Join(allLines[start:], "\n")
}

// ReadProcessLogs reads logs from a process log file
// ReadProcessLogs 从进程日志文件读取日志
func ReadProcessLogs(logDir string, lines int) (string, error) {
	// Find the most recent log file / 查找最新的日志文件
	files, err := os.ReadDir(logDir)
	if err != nil {
		return "", fmt.Errorf("failed to read log directory: %w / 读取日志目录失败：%w", err, err)
	}

	var latestFile string
	var latestTime time.Time

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasPrefix(file.Name(), "seatunnel-") || !strings.HasSuffix(file.Name(), ".log") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestFile = filepath.Join(logDir, file.Name())
		}
	}

	if latestFile == "" {
		return "", errors.New("no log files found / 未找到日志文件")
	}

	return collectStartupLogs(latestFile, lines), nil
}

// TailLogs tails a log file and sends lines to a channel
// TailLogs 跟踪日志文件并将行发送到通道
func TailLogs(ctx context.Context, logFile string, output chan<- string) error {
	file, err := os.Open(logFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Seek to end / 定位到末尾
	file.Seek(0, io.SeekEnd)

	reader := bufio.NewReader(file)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				return err
			}
			output <- strings.TrimRight(line, "\n\r")
		}
	}
}
