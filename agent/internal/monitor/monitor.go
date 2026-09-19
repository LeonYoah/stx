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

// Package monitor provides process monitoring functionality for the Agent.
// monitor 包提供 Agent 的进程监控功能。
//
// This package provides:
// 此包提供：
// - Process status monitoring / 进程状态监控
// - Manual stop marking / 手动停止标记
// - Consecutive failure detection / 连续失败检测
// - Process event generation / 进程事件生成
package monitor

import (
	"context"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logger"
	"github.com/LeonYoah/stx/agent/internal/process"
)

// DefaultMonitorInterval is the default interval for process monitoring
// DefaultMonitorInterval 是进程监控的默认间隔
const DefaultMonitorInterval = 5 * time.Second

// DefaultConsecutiveFailThreshold is the number of consecutive failures before triggering restart
// DefaultConsecutiveFailThreshold 是触发重启前的连续失败次数
// Requirements 3.6: Trigger auto restart after 3 consecutive check failures
// 需求 3.6：连续 3 次检查失败后触发自动拉起
const DefaultConsecutiveFailThreshold = 3

// ProcessStatus represents the status of a monitored process
// ProcessStatus 表示被监控进程的状态
type ProcessStatus string

const (
	StatusRunning ProcessStatus = "running"
	StatusStopped ProcessStatus = "stopped"
	StatusUnknown ProcessStatus = "unknown"
)

// TrackedProcess represents a process being tracked by the monitor
// TrackedProcess 表示被监控器跟踪的进程
type TrackedProcess struct {
	PID              int                  `json:"pid"`
	Name             string               `json:"name"`
	ClusterID        string               `json:"cluster_id,omitempty"`
	InstallDir       string               `json:"install_dir"`
	Role             string               `json:"role"`
	Status           ProcessStatus        `json:"status"`
	ManuallyStopped  bool                 `json:"manually_stopped"`  // 是否手动停止 / Whether manually stopped
	Restarting       bool                 `json:"restarting"`        // 是否正在重启 / Whether restarting
	ConsecutiveFails int                  `json:"consecutive_fails"` // 连续检查失败次数 / Consecutive check failures
	LastCheck        time.Time            `json:"last_check"`
	StartParams      *process.StartParams `json:"start_params"`
}

// SetProcessClusterID 记录受监控进程所属的集群，用于按集群更新监控配置。
// SetProcessClusterID records the owning cluster so monitor configuration can be updated per cluster.
func (m *ProcessMonitor) SetProcessClusterID(name, clusterID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if proc, exists := m.trackedProcesses[name]; exists {
		proc.ClusterID = clusterID
	}
}

// ProcessEventType represents the type of process event
// ProcessEventType 表示进程事件类型
type ProcessEventType string

const (
	EventStarted       ProcessEventType = "started"
	EventStopped       ProcessEventType = "stopped"
	EventCrashed       ProcessEventType = "crashed"
	EventRestarted     ProcessEventType = "restarted"
	EventRestartFailed ProcessEventType = "restart_failed"
)

// ProcessEvent represents a process lifecycle event
// ProcessEvent 表示进程生命周期事件
// Requirements 3.3, 3.4: Generate and report process events
// 需求 3.3, 3.4：生成并上报进程事件
type ProcessEvent struct {
	Type      ProcessEventType       `json:"type"`
	PID       int                    `json:"pid"`
	Name      string                 `json:"name"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details"`
}

// ProcessEventHandler is called when process events occur
// ProcessEventHandler 在进程事件发生时被调用
type ProcessEventHandler func(event *ProcessEvent)

// CrashHandler is called when a process crash is detected
// CrashHandler 在检测到进程崩溃时被调用
type CrashHandler func(proc *TrackedProcess)

// ProcessMonitor monitors SeaTunnel processes and detects status changes
// ProcessMonitor 监控 SeaTunnel 进程并检测状态变化
// Requirements 3.1, 3.2, 3.3: Monitor process status every 5 seconds
// 需求 3.1, 3.2, 3.3：每 5 秒监控进程状态
type ProcessMonitor struct {
	trackedProcesses         map[string]*TrackedProcess // key: name
	monitorInterval          time.Duration
	consecutiveFailThreshold int
	eventHandler             ProcessEventHandler
	crashHandler             CrashHandler
	ctx                      context.Context
	cancel                   context.CancelFunc
	running                  bool
	mu                       sync.RWMutex
}

// NewProcessMonitor creates a new ProcessMonitor instance
// NewProcessMonitor 创建一个新的 ProcessMonitor 实例
func NewProcessMonitor() *ProcessMonitor {
	return &ProcessMonitor{
		trackedProcesses:         make(map[string]*TrackedProcess),
		monitorInterval:          DefaultMonitorInterval,
		consecutiveFailThreshold: DefaultConsecutiveFailThreshold,
	}
}

// SetMonitorInterval sets the monitoring interval
// SetMonitorInterval 设置监控间隔
func (m *ProcessMonitor) SetMonitorInterval(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.monitorInterval = interval
}

// SetConsecutiveFailThreshold sets the consecutive failure threshold
// SetConsecutiveFailThreshold 设置连续失败阈值
func (m *ProcessMonitor) SetConsecutiveFailThreshold(threshold int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consecutiveFailThreshold = threshold
}

// SetEventHandler sets the event handler callback
// SetEventHandler 设置事件处理回调
func (m *ProcessMonitor) SetEventHandler(handler ProcessEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventHandler = handler
}

// SetCrashHandler sets the crash handler callback
// SetCrashHandler 设置崩溃处理回调
func (m *ProcessMonitor) SetCrashHandler(handler CrashHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.crashHandler = handler
}

// Start starts the process monitor
// Start 启动进程监控器
func (m *ProcessMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.running = true
	m.mu.Unlock()

	logger.InfoF(ctx, "[ProcessMonitor] Starting with interval: %v / 启动，间隔：%v", m.monitorInterval, m.monitorInterval)

	go m.monitorLoop()

	return nil
}

// Stop stops the process monitor
// Stop 停止进程监控器
func (m *ProcessMonitor) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}
	m.running = false

	ctx := context.Background()
	logger.InfoF(ctx, "[ProcessMonitor] Stopped / 已停止")
	return nil
}

// monitorLoop runs the monitoring loop
// monitorLoop 运行监控循环
func (m *ProcessMonitor) monitorLoop() {
	ticker := time.NewTicker(m.monitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAllProcesses()
		}
	}
}

// checkAllProcesses checks the status of all tracked processes
// checkAllProcesses 检查所有跟踪进程的状态
// Requirements 3.1, 3.6: Check process status, detect consecutive failures
// 需求 3.1, 3.6：检查进程状态，检测连续失败
func (m *ProcessMonitor) checkAllProcesses() {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, proc := range m.trackedProcesses {
		// Skip if currently restarting / 如果正在重启则跳过
		if proc.Restarting {
			continue
		}

		// Handle PID=0 case: process is registered but not running, trigger auto-start
		// 处理 PID=0 的情况：进程已注册但未运行，触发自动启动
		if proc.PID <= 0 {
			// Only trigger if we have start params (meaning we can restart it)
			// 只有在有启动参数时才触发（意味着我们可以重启它）
			if proc.StartParams != nil {
				logger.InfoF(ctx, "[ProcessMonitor] Process %s has PID=0, triggering auto-start / 进程 %s 的 PID=0，触发自动启动",
					name, name)
				proc.Restarting = true // Mark as restarting to prevent duplicate triggers / 标记为正在重启以防止重复触发

				// Notify crash handler to start the process / 通知崩溃处理器启动进程
				if m.crashHandler != nil {
					procCopy := *proc
					go m.crashHandler(&procCopy)
				}
			}
			continue
		}

		// Check if process is alive and still matches the expected executable / 检查进程是否存活且仍然是期望的可执行文件
		alive := isProcessAlive(proc)
		proc.LastCheck = time.Now()

		if proc.ManuallyStopped {
			if alive {
				proc.Status = StatusRunning
			} else {
				proc.Status = StatusStopped
			}
			proc.ConsecutiveFails = 0
			proc.Restarting = false
			continue
		}

		if alive {
			// Process is running / 进程正在运行
			proc.Status = StatusRunning
			proc.ConsecutiveFails = 0
		} else {
			// Process is not running / 进程未运行
			proc.ConsecutiveFails++
			logger.WarnF(ctx, "[ProcessMonitor] Process %s (PID: %d) not alive, consecutive fails: %d / 进程 %s（PID：%d）不存活，连续失败：%d",
				name, proc.PID, proc.ConsecutiveFails, name, proc.PID, proc.ConsecutiveFails)

			// Check if threshold reached / 检查是否达到阈值
			if proc.ConsecutiveFails >= m.consecutiveFailThreshold {
				proc.Status = StatusStopped
				proc.Restarting = true // Mark as restarting to prevent duplicate triggers / 标记为正在重启以防止重复触发

				// Generate crash event / 生成崩溃事件
				event := &ProcessEvent{
					Type:      EventCrashed,
					PID:       proc.PID,
					Name:      proc.Name,
					Timestamp: time.Now(),
					Details: map[string]interface{}{
						"consecutive_fails": proc.ConsecutiveFails,
						"install_dir":       proc.InstallDir,
						"role":              proc.Role,
					},
				}
				m.notifyEvent(event)

				// Notify crash handler / 通知崩溃处理器
				if m.crashHandler != nil {
					// Make a copy to avoid race conditions / 复制以避免竞态条件
					procCopy := *proc
					go m.crashHandler(&procCopy)
				}
			}
		}
	}
}

// notifyEvent notifies the event handler
// notifyEvent 通知事件处理器
func (m *ProcessMonitor) notifyEvent(event *ProcessEvent) {
	if m.eventHandler != nil {
		go m.eventHandler(event)
	}
}

// TrackProcess starts tracking a process
// TrackProcess 开始跟踪一个进程
func (m *ProcessMonitor) TrackProcess(name string, pid int, installDir, role string, startParams *process.StartParams) {
	m.TrackProcessWithEvent(name, pid, installDir, role, startParams, true)
}

// TrackProcessSilent starts tracking a process without sending started event.
// Used when Agent restarts and re-tracks existing running processes.
// TrackProcessSilent 静默开始跟踪进程，不发送启动事件。
// 用于 Agent 重启后重新跟踪已运行的进程。
func (m *ProcessMonitor) TrackProcessSilent(name string, pid int, installDir, role string, startParams *process.StartParams) {
	m.TrackProcessWithEvent(name, pid, installDir, role, startParams, false)
}

// TrackProcessWithEvent starts tracking a process with optional event notification.
// TrackProcessWithEvent 开始跟踪进程，可选是否发送事件通知。
func (m *ProcessMonitor) TrackProcessWithEvent(name string, pid int, installDir, role string, startParams *process.StartParams, sendEvent bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc := &TrackedProcess{
		PID:              pid,
		Name:             name,
		InstallDir:       installDir,
		Role:             role,
		Status:           StatusRunning,
		ManuallyStopped:  false,
		ConsecutiveFails: 0,
		LastCheck:        time.Now(),
		StartParams:      startParams,
	}

	m.trackedProcesses[name] = proc
	ctx := context.Background()
	logger.InfoF(ctx, "[ProcessMonitor] Tracking process: %s (PID: %d) / 跟踪进程：%s（PID：%d）", name, pid, name, pid)

	// Generate started event only if requested / 仅在需要时生成启动事件
	if sendEvent {
		event := &ProcessEvent{
			Type:      EventStarted,
			PID:       pid,
			Name:      name,
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"install_dir": installDir,
				"role":        role,
			},
		}
		m.notifyEvent(event)
	}
}

// UntrackProcess stops tracking a process
// UntrackProcess 停止跟踪一个进程
func (m *ProcessMonitor) UntrackProcess(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if proc, exists := m.trackedProcesses[name]; exists {
		// Generate stopped event / 生成停止事件
		event := &ProcessEvent{
			Type:      EventStopped,
			PID:       proc.PID,
			Name:      name,
			Timestamp: time.Now(),
			Details:   map[string]interface{}{},
		}
		m.notifyEvent(event)

		delete(m.trackedProcesses, name)
		ctx := context.Background()
		logger.InfoF(ctx, "[ProcessMonitor] Untracked process: %s / 取消跟踪进程：%s", name, name)
	}
}

// UntrackProcessSilent stops tracking a process without sending events.
// Used when disabling auto-restart - the process is still running, we just stop monitoring it.
// UntrackProcessSilent 静默停止跟踪进程，不发送事件。
// 用于禁用自动重启时 - 进程仍在运行，我们只是停止监控它。
func (m *ProcessMonitor) UntrackProcessSilent(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.trackedProcesses[name]; exists {
		delete(m.trackedProcesses, name)
		ctx := context.Background()
		logger.InfoF(ctx, "[ProcessMonitor] Silently untracked process (still running): %s / 静默取消跟踪进程（仍在运行）：%s", name, name)
	}
}

// UpdateProcessPID updates the PID of a tracked process
// UpdateProcessPID 更新跟踪进程的 PID
func (m *ProcessMonitor) UpdateProcessPID(name string, newPID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if proc, exists := m.trackedProcesses[name]; exists {
		proc.PID = newPID
		if newPID > 0 {
			proc.Status = StatusRunning
			proc.ConsecutiveFails = 0
		} else {
			proc.Status = StatusStopped
		}
		proc.Restarting = false // Clear restarting flag / 清除重启标记
		ctx := context.Background()
		logger.InfoF(ctx, "[ProcessMonitor] Updated process PID: %s -> %d / 更新进程 PID：%s -> %d", name, newPID, name, newPID)
	}
}

// MarkManuallyStopped marks one tracked process as manually stopped.
// MarkManuallyStopped 将一个跟踪中的进程标记为手动停止。
func (m *ProcessMonitor) MarkManuallyStopped(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if proc, exists := m.trackedProcesses[name]; exists {
		proc.ManuallyStopped = true
		proc.Restarting = false
		proc.ConsecutiveFails = 0
	}
}

// ClearManuallyStopped clears the manual-stop flag for one tracked process.
// ClearManuallyStopped 清除一个跟踪中的进程的手动停止标记。
func (m *ProcessMonitor) ClearManuallyStopped(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if proc, exists := m.trackedProcesses[name]; exists {
		proc.ManuallyStopped = false
	}
}

// IsManuallyStopped returns whether one tracked process is manually stopped.
// IsManuallyStopped 返回一个跟踪中的进程是否被手动停止。
func (m *ProcessMonitor) IsManuallyStopped(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if proc, exists := m.trackedProcesses[name]; exists {
		return proc.ManuallyStopped
	}
	return false
}

// GetTrackedProcess returns a tracked process by name
// GetTrackedProcess 按名称返回跟踪的进程
func (m *ProcessMonitor) GetTrackedProcess(name string) *TrackedProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if proc, exists := m.trackedProcesses[name]; exists {
		// Return a copy / 返回副本
		procCopy := *proc
		return &procCopy
	}
	return nil
}

// GetAllTrackedProcesses returns all tracked processes
// GetAllTrackedProcesses 返回所有跟踪的进程
func (m *ProcessMonitor) GetAllTrackedProcesses() []*TrackedProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var processes []*TrackedProcess
	for _, proc := range m.trackedProcesses {
		procCopy := *proc
		processes = append(processes, &procCopy)
	}
	return processes
}

// isProcessAlive 检查被跟踪进程是否存活，并核对安装目录和角色。
// isProcessAlive checks whether a tracked process is alive and matches its install directory and role.
func isProcessAlive(proc *TrackedProcess) bool {
	if proc == nil {
		return false
	}
	pid := proc.PID
	if !isPidAlive(pid) {
		return false
	}

	if proc.InstallDir == "" {
		return true
	}

	matched, err := process.MatchesSeaTunnelProcess(context.Background(), pid, proc.InstallDir, proc.Role)
	if err != nil {
		// 进程列表临时读取失败时保留基础探活结果，避免误触发自动拉起。
		// Keep the basic liveness result when process inspection fails to avoid a false auto-restart.
		return true
	}
	return matched
}

// isPidAlive performs a basic liveness check for a PID using signal 0.
// isPidAlive 使用信号 0 对给定 PID 做基础存活检查。
func isPidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0 to check.
	// 在 Unix 上，FindProcess 总是成功，所以需要发送信号 0 来检查。
	if runtime.GOOS != "windows" {
		err = process.Signal(syscall.Signal(0))
		return err == nil
	}

	// On Windows, use a different approach.
	// 在 Windows 上，使用不同的方法。
	return checkProcessWindows(pid)
}

// checkProcessWindows checks if a process is alive on Windows
// checkProcessWindows 在 Windows 上检查进程是否存活
func checkProcessWindows(pid int) bool {
	// Simple check: try to open the process
	// 简单检查：尝试打开进程
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess doesn't actually check if process exists
	// We need to try to signal it
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
