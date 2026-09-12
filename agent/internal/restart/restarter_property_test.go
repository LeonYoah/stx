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

package restart

import (
	"testing"
	"time"

	"github.com/LeonYoah/stx/agent/internal/monitor"
	"pgregory.net/rapid"
)

// **Feature: seatunnel-process-monitor, Property 8: 重启次数限制**
// **Validates: Requirements 4.5**
// For any process, restart count within the configured time window should not exceed
// the maximum limit. After exceeding, auto restart should stop.
// 对于任何进程，在配置的时间窗口内重启次数不应超过最大限制，超限后应停止自动重启。
func TestProperty_RestartCountLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random config / 生成随机配置
		maxRestarts := rapid.IntRange(1, 5).Draw(t, "maxRestarts")
		timeWindow := time.Duration(rapid.IntRange(60, 300).Draw(t, "timeWindow")) * time.Second

		restarter := NewAutoRestarter(nil)
		restarter.SetConfig(&RestartConfig{
			Enabled:        true,
			RestartDelay:   1 * time.Second,
			MaxRestarts:    maxRestarts,
			TimeWindow:     timeWindow,
			CooldownPeriod: 30 * time.Minute,
		})

		// Generate random process name / 生成随机进程名
		processName := rapid.StringMatching(`seatunnel-[a-z0-9]+`).Draw(t, "processName")

		proc := &monitor.TrackedProcess{
			Name:            processName,
			PID:             1234,
			InstallDir:      "/opt/seatunnel",
			Role:            "hybrid",
			ManuallyStopped: false,
		}

		// Simulate restarts up to max / 模拟重启直到最大次数
		for i := 0; i < maxRestarts; i++ {
			if !restarter.ShouldRestart(proc) {
				t.Errorf("Should allow restart %d (max: %d)", i+1, maxRestarts)
			}
			restarter.recordRestart(processName)
		}

		// Next restart should be denied / 下一次重启应被拒绝
		if restarter.ShouldRestart(proc) {
			t.Errorf("Should NOT allow restart after reaching max (%d)", maxRestarts)
		}

		// Verify in cooldown / 验证在冷却中
		if !restarter.IsInCooldown(processName) {
			t.Errorf("Should be in cooldown after reaching max restarts")
		}
	})
}

// **Feature: seatunnel-process-monitor, Property 9: 冷却时间重置**
// **Validates: Requirements 4.6**
// For any process, after cooldown period passes, restart counter should be reset to 0.
// 对于任何进程，当冷却时间过后，重启计数器应被重置为 0。
func TestProperty_CooldownReset(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random process name / 生成随机进程名
		processName := rapid.StringMatching(`seatunnel-[a-z0-9]+`).Draw(t, "processName")

		restarter := NewAutoRestarter(nil)
		restarter.SetConfig(&RestartConfig{
			Enabled:        true,
			RestartDelay:   1 * time.Second,
			MaxRestarts:    3,
			TimeWindow:     5 * time.Minute,
			CooldownPeriod: 1 * time.Millisecond, // Very short for testing / 非常短用于测试
		})

		proc := &monitor.TrackedProcess{
			Name:            processName,
			PID:             1234,
			InstallDir:      "/opt/seatunnel",
			Role:            "hybrid",
			ManuallyStopped: false,
		}

		// Reach max restarts / 达到最大重启次数
		for i := 0; i < 3; i++ {
			restarter.recordRestart(processName)
		}

		// Should be in cooldown / 应在冷却中
		restarter.ShouldRestart(proc) // This triggers cooldown

		// Wait for cooldown to pass / 等待冷却过去
		time.Sleep(10 * time.Millisecond)

		// Reset restart count / 重置重启计数
		restarter.ResetRestartCount(processName)

		// Should allow restart again / 应再次允许重启
		if !restarter.ShouldRestart(proc) {
			t.Errorf("Should allow restart after cooldown reset")
		}

		// Verify history is reset / 验证历史已重置
		history := restarter.GetRestartHistory(processName)
		if history != nil && len(history.RestartTimes) > 0 {
			t.Errorf("Restart times should be empty after reset")
		}
	})
}

// TestAutoRestarter_ShouldRestart tests restart decision logic
// TestAutoRestarter_ShouldRestart 测试重启决策逻辑
func TestAutoRestarter_ShouldRestart(t *testing.T) {
	restarter := NewAutoRestarter(nil)
	restarter.SetConfig(&RestartConfig{
		Enabled:        true,
		RestartDelay:   1 * time.Second,
		MaxRestarts:    3,
		TimeWindow:     5 * time.Minute,
		CooldownPeriod: 30 * time.Minute,
	})

	testCases := []struct {
		name        string
		proc        *monitor.TrackedProcess
		setupFunc   func()
		wantRestart bool
	}{
		{
			name: "normal process should restart",
			proc: &monitor.TrackedProcess{
				Name:            "test-process",
				ManuallyStopped: false,
			},
			wantRestart: true,
		},
		{
			name: "manually stopped should not restart",
			proc: &monitor.TrackedProcess{
				Name:            "test-process-manual",
				ManuallyStopped: true,
			},
			wantRestart: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupFunc != nil {
				tc.setupFunc()
			}

			got := restarter.ShouldRestart(tc.proc)
			if got != tc.wantRestart {
				t.Errorf("ShouldRestart() = %v, want %v", got, tc.wantRestart)
			}
		})
	}
}

// TestAutoRestarter_DisabledConfig tests disabled auto restart
// TestAutoRestarter_DisabledConfig 测试禁用自动重启
func TestAutoRestarter_DisabledConfig(t *testing.T) {
	restarter := NewAutoRestarter(nil)
	restarter.SetConfig(&RestartConfig{
		Enabled: false,
	})

	proc := &monitor.TrackedProcess{
		Name:            "test-process",
		ManuallyStopped: false,
	}

	if restarter.ShouldRestart(proc) {
		t.Error("Should not restart when disabled")
	}
}

// TestAutoRestarter_ConfigUpdate tests config hot update
// TestAutoRestarter_ConfigUpdate 测试配置热更新
func TestAutoRestarter_ConfigUpdate(t *testing.T) {
	restarter := NewAutoRestarter(nil)

	// Initial config / 初始配置
	restarter.SetConfig(&RestartConfig{
		Enabled:     true,
		MaxRestarts: 3,
	})

	config := restarter.GetConfig()
	if config.MaxRestarts != 3 {
		t.Errorf("MaxRestarts should be 3, got %d", config.MaxRestarts)
	}

	// Update config / 更新配置
	restarter.UpdateConfig(&RestartConfig{
		Enabled:     true,
		MaxRestarts: 5,
	})

	config = restarter.GetConfig()
	if config.MaxRestarts != 5 {
		t.Errorf("MaxRestarts should be 5 after update, got %d", config.MaxRestarts)
	}
}
