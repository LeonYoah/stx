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

// Package restart provides automatic process restart functionality for the Agent.
// restart 包提供 Agent 的自动进程重启功能。
//
// This package provides:
// 此包提供：
// - Automatic restart on process crash / 进程崩溃时自动重启
// - Restart count limiting / 重启次数限制
// - Cooldown period management / 冷却时间管理
// - Restart history tracking / 重启历史跟踪
package restart

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logger"
	"github.com/LeonYoah/stx/agent/internal/monitor"
	"github.com/LeonYoah/stx/agent/internal/process"
)

// Default configuration values
// 默认配置值
const (
	DefaultRestartDelay   = 10 * time.Second // 默认重启延迟 / Default restart delay
	DefaultMaxRestarts    = 3                // 默认最大重启次数 / Default max restarts
	DefaultTimeWindow     = 5 * time.Minute  // 默认时间窗口 / Default time window
	DefaultCooldownPeriod = 30 * time.Minute // 默认冷却时间 / Default cooldown period
)

// RestartConfig holds the restart configuration
// RestartConfig 保存重启配置
// Requirements 4.1, 4.2, 4.3, 4.4: Restart configuration
// 需求 4.1, 4.2, 4.3, 4.4：重启配置
type RestartConfig struct {
	Enabled        bool          `json:"enabled"`         // 是否启用自动重启 / Enable auto restart
	RestartDelay   time.Duration `json:"restart_delay"`   // 重启延迟 / Restart delay
	MaxRestarts    int           `json:"max_restarts"`    // 最大重启次数 / Max restart count
	TimeWindow     time.Duration `json:"time_window"`     // 时间窗口 / Time window
	CooldownPeriod time.Duration `json:"cooldown_period"` // 冷却时间 / Cooldown period
}

// DefaultRestartConfig returns the default restart configuration
// DefaultRestartConfig 返回默认重启配置
func DefaultRestartConfig() *RestartConfig {
	return &RestartConfig{
		Enabled:        true,
		RestartDelay:   DefaultRestartDelay,
		MaxRestarts:    DefaultMaxRestarts,
		TimeWindow:     DefaultTimeWindow,
		CooldownPeriod: DefaultCooldownPeriod,
	}
}

// RestartHistory tracks restart history for a process
// RestartHistory 跟踪进程的重启历史
type RestartHistory struct {
	ProcessName   string      `json:"process_name"`
	RestartCount  int         `json:"restart_count"`
	LastRestart   time.Time   `json:"last_restart"`
	WindowStart   time.Time   `json:"window_start"`
	CooldownUntil time.Time   `json:"cooldown_until"`
	RestartTimes  []time.Time `json:"restart_times"` // 重启时间列表 / List of restart times
}

// RestartCallback is called when a restart is performed
// RestartCallback 在执行重启时被调用
type RestartCallback func(processName string, success bool, err error)

// AutoRestarter handles automatic process restart on crash
// AutoRestarter 处理进程崩溃时的自动重启
// Requirements 4.1, 4.2, 4.3, 4.4, 4.5, 4.6: Auto restart with limits and cooldown
// 需求 4.1, 4.2, 4.3, 4.4, 4.5, 4.6：带限制和冷却的自动重启
type AutoRestarter struct {
	processManager *process.ProcessManager
	config         *RestartConfig
	restartHistory map[string]*RestartHistory
	callback       RestartCallback
	mu             sync.RWMutex
}

// NewAutoRestarter creates a new AutoRestarter instance
// NewAutoRestarter 创建一个新的 AutoRestarter 实例
func NewAutoRestarter(pm *process.ProcessManager) *AutoRestarter {
	return &AutoRestarter{
		processManager: pm,
		config:         DefaultRestartConfig(),
		restartHistory: make(map[string]*RestartHistory),
	}
}

// SetConfig sets the restart configuration
// SetConfig 设置重启配置
func (r *AutoRestarter) SetConfig(config *RestartConfig) {
	ctx := context.Background()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = config
	logger.InfoF(ctx, "[AutoRestarter] Config updated: enabled=%v, delay=%v, maxRestarts=%d, window=%v, cooldown=%v / 配置已更新",
		config.Enabled, config.RestartDelay, config.MaxRestarts, config.TimeWindow, config.CooldownPeriod)
}

// UpdateConfig updates the restart configuration
// UpdateConfig 更新重启配置
// Requirements 5.5: Apply new config immediately without restart
// 需求 5.5：立即应用新配置，无需重启
func (r *AutoRestarter) UpdateConfig(config *RestartConfig) {
	r.SetConfig(config)
}

// SetCallback sets the restart callback
// SetCallback 设置重启回调
func (r *AutoRestarter) SetCallback(callback RestartCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callback = callback
}

// OnProcessCrashed handles a process crash event
// OnProcessCrashed 处理进程崩溃事件
// Requirements 4.1, 4.7: Handle crash, check if should restart
// 需求 4.1, 4.7：处理崩溃，检查是否应该重启
func (r *AutoRestarter) OnProcessCrashed(proc *monitor.TrackedProcess) error {
	ctx := context.Background()
	r.mu.Lock()
	config := r.config
	r.mu.Unlock()

	if !config.Enabled {
		logger.InfoF(ctx, "[AutoRestarter] Auto restart disabled, skipping restart for %s / 自动重启已禁用，跳过 %s 的重启",
			proc.Name, proc.Name)
		return nil
	}

	// Check if should restart / 检查是否应该重启
	if !r.ShouldRestart(proc) {
		logger.WarnF(ctx, "[AutoRestarter] Restart limit reached or in cooldown for %s / %s 已达重启限制或在冷却中",
			proc.Name, proc.Name)
		return fmt.Errorf("restart limit reached or in cooldown / 已达重启限制或在冷却中")
	}

	// Wait for restart delay / 等待重启延迟
	logger.InfoF(ctx, "[AutoRestarter] Waiting %v before restarting %s / 等待 %v 后重启 %s",
		config.RestartDelay, proc.Name, config.RestartDelay, proc.Name)
	time.Sleep(config.RestartDelay)

	// Re-check enabled after delay: config may have been set to disabled (e.g. cluster deleted).
	// 延迟后再次检查是否启用：配置可能已被设为禁用（例如集群已删除）。
	r.mu.Lock()
	stillEnabled := r.config.Enabled
	r.mu.Unlock()
	if !stillEnabled {
		logger.InfoF(ctx, "[AutoRestarter] Auto restart disabled after delay, skipping restart for %s / 延迟后自动重启已禁用，跳过 %s 的重启",
			proc.Name, proc.Name)
		return nil
	}

	// Perform restart / 执行重启
	return r.DoRestart(context.Background(), proc)
}

// ShouldRestart checks if a process should be restarted
// ShouldRestart 检查进程是否应该重启
// Requirements 4.5, 4.6: Check restart count and cooldown
// 需求 4.5, 4.6：检查重启次数和冷却时间
func (r *AutoRestarter) ShouldRestart(proc *monitor.TrackedProcess) bool {
	ctx := context.Background()
	r.mu.Lock()
	defer r.mu.Unlock()

	if proc == nil {
		return false
	}

	if !r.config.Enabled {
		return false
	}

	if proc.ManuallyStopped {
		logger.InfoF(ctx, "[AutoRestarter] Process %s is manually stopped, skipping restart / 进程 %s 已被手动停止，跳过自动重启",
			proc.Name, proc.Name)
		return false
	}

	history, exists := r.restartHistory[proc.Name]
	if !exists {
		// No history, can restart / 无历史，可以重启
		return true
	}

	now := time.Now()

	// Check if in cooldown / 检查是否在冷却中
	if now.Before(history.CooldownUntil) {
		logger.InfoF(ctx, "[AutoRestarter] Process %s is in cooldown until %v / 进程 %s 在冷却中直到 %v",
			proc.Name, history.CooldownUntil, proc.Name, history.CooldownUntil)
		return false
	}

	// Check if cooldown has passed and reset counter / 检查冷却是否已过并重置计数器
	if now.After(history.CooldownUntil) && history.CooldownUntil.After(history.WindowStart) {
		// Cooldown passed, reset counter / 冷却已过，重置计数器
		r.resetHistoryLocked(proc.Name)
		return true
	}

	// Count restarts within time window / 计算时间窗口内的重启次数
	windowStart := now.Add(-r.config.TimeWindow)
	restartsInWindow := 0
	for _, t := range history.RestartTimes {
		if t.After(windowStart) {
			restartsInWindow++
		}
	}

	// Check if max restarts reached / 检查是否达到最大重启次数
	if restartsInWindow >= r.config.MaxRestarts {
		// Enter cooldown / 进入冷却
		history.CooldownUntil = now.Add(r.config.CooldownPeriod)
		logger.WarnF(ctx, "[AutoRestarter] Max restarts (%d) reached for %s, entering cooldown until %v / %s 已达最大重启次数（%d），进入冷却直到 %v",
			r.config.MaxRestarts, proc.Name, history.CooldownUntil, proc.Name, r.config.MaxRestarts, history.CooldownUntil)
		return false
	}

	return true
}

// DoRestart performs the actual restart
// DoRestart 执行实际的重启
// Requirements 4.2, 4.3: Use same startup parameters, report restart event
// 需求 4.2, 4.3：使用相同的启动参数，上报重启事件
func (r *AutoRestarter) DoRestart(ctx context.Context, proc *monitor.TrackedProcess) error {
	r.mu.Lock()
	callback := r.callback
	r.mu.Unlock()

	logger.InfoF(ctx, "[AutoRestarter] Restarting process %s... / 正在重启进程 %s...", proc.Name, proc.Name)

	// Get start params / 获取启动参数
	startParams := proc.StartParams
	if startParams == nil {
		startParams = &process.StartParams{
			InstallDir: proc.InstallDir,
			Role:       proc.Role,
		}
	}

	// Perform restart / 执行重启
	err := r.processManager.StartProcess(ctx, proc.Name, startParams)
	if err != nil {
		if errors.Is(err, process.ErrProcessAlreadyRunning) {
			logger.InfoF(ctx, "[AutoRestarter] Process %s already running, treating as success / 进程 %s 已在运行，视为成功", proc.Name, proc.Name)
			if callback != nil {
				callback(proc.Name, true, nil)
			}
			return nil
		}
		r.recordRestart(proc.Name)
		logger.ErrorF(ctx, "[AutoRestarter] Failed to restart %s: %v / 重启 %s 失败：%v", proc.Name, err, proc.Name, err)
		if callback != nil {
			callback(proc.Name, false, err)
		}
		return err
	}

	r.recordRestart(proc.Name)
	logger.InfoF(ctx, "[AutoRestarter] Successfully restarted %s / 成功重启 %s", proc.Name, proc.Name)
	if callback != nil {
		callback(proc.Name, true, nil)
	}

	return nil
}

// recordRestart records a restart in history
// recordRestart 在历史中记录重启
func (r *AutoRestarter) recordRestart(processName string) {
	ctx := context.Background()
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	history, exists := r.restartHistory[processName]
	if !exists {
		history = &RestartHistory{
			ProcessName:  processName,
			WindowStart:  now,
			RestartTimes: make([]time.Time, 0),
		}
		r.restartHistory[processName] = history
	}

	history.RestartCount++
	history.LastRestart = now
	history.RestartTimes = append(history.RestartTimes, now)

	// Clean up old restart times / 清理旧的重启时间
	windowStart := now.Add(-r.config.TimeWindow)
	var newTimes []time.Time
	for _, t := range history.RestartTimes {
		if t.After(windowStart) {
			newTimes = append(newTimes, t)
		}
	}
	history.RestartTimes = newTimes

	logger.InfoF(ctx, "[AutoRestarter] Recorded restart for %s, count in window: %d / 记录 %s 的重启，窗口内次数：%d",
		processName, len(history.RestartTimes), processName, len(history.RestartTimes))
}

// ResetRestartCount resets the restart count for a process
// ResetRestartCount 重置进程的重启计数
// Requirements 4.6: Reset counter after cooldown
// 需求 4.6：冷却后重置计数器
func (r *AutoRestarter) ResetRestartCount(processName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resetHistoryLocked(processName)
}

// resetHistoryLocked resets history (must be called with lock held)
// resetHistoryLocked 重置历史（必须在持有锁的情况下调用）
func (r *AutoRestarter) resetHistoryLocked(processName string) {
	ctx := context.Background()
	if history, exists := r.restartHistory[processName]; exists {
		history.RestartCount = 0
		history.RestartTimes = make([]time.Time, 0)
		history.WindowStart = time.Now()
		history.CooldownUntil = time.Time{}
		logger.InfoF(ctx, "[AutoRestarter] Reset restart count for %s / 重置 %s 的重启计数", processName, processName)
	}
}

// GetRestartHistory returns the restart history for a process
// GetRestartHistory 返回进程的重启历史
func (r *AutoRestarter) GetRestartHistory(processName string) *RestartHistory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if history, exists := r.restartHistory[processName]; exists {
		// Return a copy / 返回副本
		historyCopy := *history
		historyCopy.RestartTimes = make([]time.Time, len(history.RestartTimes))
		copy(historyCopy.RestartTimes, history.RestartTimes)
		return &historyCopy
	}
	return nil
}

// GetConfig returns the current configuration
// GetConfig 返回当前配置
func (r *AutoRestarter) GetConfig() *RestartConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	configCopy := *r.config
	return &configCopy
}

// IsInCooldown checks if a process is in cooldown
// IsInCooldown 检查进程是否在冷却中
func (r *AutoRestarter) IsInCooldown(processName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if history, exists := r.restartHistory[processName]; exists {
		return time.Now().Before(history.CooldownUntil)
	}
	return false
}

// IsEnabled returns whether auto restart is enabled
// IsEnabled 返回是否启用了自动重启
func (r *AutoRestarter) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config.Enabled
}
