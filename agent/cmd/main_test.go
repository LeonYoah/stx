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

package main

import (
	"testing"
	"time"

	"github.com/LeonYoah/stx/agent/internal/config"
	"github.com/LeonYoah/stx/agent/internal/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewAgent tests Agent creation
// TestNewAgent 测试 Agent 创建
func TestNewAgent(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			ID: "test-agent",
		},
		ControlPlane: config.ControlPlaneConfig{
			Addresses: []string{"localhost:9090"},
		},
		Heartbeat: config.HeartbeatConfig{
			Interval: 10 * time.Second,
		},
		Log: config.LogConfig{
			Level: "info",
		},
	}

	agent := NewAgent(cfg)
	require.NotNil(t, agent)
	assert.Equal(t, cfg, agent.config)
	assert.NotNil(t, agent.ctx)
	assert.NotNil(t, agent.cancel)
}

// TestAgentShutdown tests Agent shutdown
// TestAgentShutdown 测试 Agent 关闭
func TestAgentShutdown(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			ID: "test-agent",
		},
		ControlPlane: config.ControlPlaneConfig{
			Addresses: []string{"localhost:9090"},
		},
		Heartbeat: config.HeartbeatConfig{
			Interval: 10 * time.Second,
		},
		Log: config.LogConfig{
			Level: "info",
		},
	}

	agent := NewAgent(cfg)

	// Start agent in goroutine / 在 goroutine 中启动 Agent
	done := make(chan struct{})
	go func() {
		_ = agent.Run()
		close(done)
	}()

	// Give it a moment to start / 给它一点时间启动
	time.Sleep(100 * time.Millisecond)

	// Shutdown / 关闭
	agent.Shutdown()

	// Wait for agent to stop / 等待 Agent 停止
	select {
	case <-done:
		// Success / 成功
	case <-time.After(2 * time.Second):
		t.Fatal("Agent did not shutdown in time")
	}
}

// TestAgentContextCancellation tests that Agent respects context cancellation
// TestAgentContextCancellation 测试 Agent 是否遵守上下文取消
func TestAgentContextCancellation(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			ID: "test-agent",
		},
		ControlPlane: config.ControlPlaneConfig{
			Addresses: []string{"localhost:9090"},
		},
		Heartbeat: config.HeartbeatConfig{
			Interval: 10 * time.Second,
		},
		Log: config.LogConfig{
			Level: "info",
		},
	}

	// Use NewAgent to properly initialize all components
	// 使用 NewAgent 正确初始化所有组件
	agent := NewAgent(cfg)

	// Start agent in goroutine / 在 goroutine 中启动 Agent
	errChan := make(chan error, 1)
	go func() {
		errChan <- agent.Run()
	}()

	// Give it a moment to start / 给它一点时间启动
	time.Sleep(100 * time.Millisecond)

	// Cancel context via Shutdown / 通过 Shutdown 取消上下文
	agent.Shutdown()

	// Wait for agent to stop / 等待 Agent 停止
	select {
	case <-errChan:
		// Agent stopped (may have error due to no Control Plane, which is expected in test)
		// Agent 已停止（可能因为没有 Control Plane 而出错，这在测试中是预期的）
	case <-time.After(2 * time.Second):
		t.Fatal("Agent did not stop after context cancellation")
	}
}

// TestVersionCommand tests the version command
// TestVersionCommand 测试版本命令
func TestVersionCommand(t *testing.T) {
	// Just verify the command exists and doesn't panic
	// 只验证命令存在且不会 panic
	assert.NotNil(t, versionCmd)
	assert.Equal(t, "version", versionCmd.Use)
}

// TestRootCommand tests the root command
// TestRootCommand 测试根命令
func TestRootCommand(t *testing.T) {
	assert.NotNil(t, rootCmd)
	assert.Equal(t, "stx-agent", rootCmd.Use)
}

func TestExtractProcessEventReportFieldsPreservesDetails(t *testing.T) {
	event := &monitor.ProcessEvent{
		Type: monitor.EventRestartFailed,
		Details: map[string]interface{}{
			"install_dir": "/opt/seatunnel",
			"role":        "master/worker",
			"error":       "exit status 1",
			"retry_count": 3,
		},
	}

	installDir, role, details := extractProcessEventReportFields(event)
	assert.Equal(t, "/opt/seatunnel", installDir)
	assert.Equal(t, "master/worker", role)
	assert.Equal(t, "/opt/seatunnel", details["install_dir"])
	assert.Equal(t, "master/worker", details["role"])
	assert.Equal(t, "exit status 1", details["error"])
	assert.Equal(t, "3", details["retry_count"])
}
