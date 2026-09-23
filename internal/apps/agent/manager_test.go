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

package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "github.com/LeonYoah/stx/internal/proto/agent"
)

// mockHostUpdater is a mock implementation of HostStatusUpdater for testing.
// mockHostUpdater 是用于测试的 HostStatusUpdater 模拟实现。
type mockHostUpdater struct {
	mu             sync.Mutex
	agentStatuses  map[string]string
	heartbeats     map[string]time.Time
	offlineAgents  []string
	updateAgentErr error
	heartbeatErr   error
	markOfflineErr error
}

func newMockHostUpdater() *mockHostUpdater {
	return &mockHostUpdater{
		agentStatuses: make(map[string]string),
		heartbeats:    make(map[string]time.Time),
		offlineAgents: make([]string, 0),
	}
}

func (m *mockHostUpdater) UpdateAgentStatus(ctx context.Context, hostID uint, ipAddress string, agentID string, version string, sysInfo *SystemInfo, hostname string, localIPs []string) (uint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateAgentErr != nil {
		return 0, m.updateAgentErr
	}
	m.agentStatuses[agentID] = "installed"
	if hostID > 0 {
		return hostID, nil
	}
	return 1, nil
}

func (m *mockHostUpdater) UpdateHeartbeat(ctx context.Context, agentID string, cpuUsage, memoryUsage, diskUsage float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.heartbeatErr != nil {
		return m.heartbeatErr
	}
	m.heartbeats[agentID] = time.Now()
	return nil
}

func (m *mockHostUpdater) MarkHostOffline(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markOfflineErr != nil {
		return m.markOfflineErr
	}
	m.offlineAgents = append(m.offlineAgents, agentID)
	m.agentStatuses[agentID] = "offline"
	return nil
}

func (m *mockHostUpdater) getOfflineAgents() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.offlineAgents))
	copy(result, m.offlineAgents)
	return result
}

func (m *mockHostUpdater) getAgentStatus(agentID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.agentStatuses[agentID]
}

// TestManagerCreation tests the creation of a new Manager.
// TestManagerCreation 测试创建新的 Manager。
func TestManagerCreation(t *testing.T) {
	// Test with nil config (should use defaults)
	// 使用 nil 配置测试（应使用默认值）
	m := NewManager(nil)
	if m == nil {
		t.Fatal("Expected non-nil manager")
	}
	if m.config.HeartbeatInterval != DefaultHeartbeatInterval {
		t.Errorf("Expected default heartbeat interval %v, got %v", DefaultHeartbeatInterval, m.config.HeartbeatInterval)
	}
	if m.config.HeartbeatTimeout != DefaultHeartbeatTimeout {
		t.Errorf("Expected default heartbeat timeout %v, got %v", DefaultHeartbeatTimeout, m.config.HeartbeatTimeout)
	}

	// Test with custom config
	// 使用自定义配置测试
	customConfig := &ManagerConfig{
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  15 * time.Second,
		CheckInterval:     2 * time.Second,
	}
	m2 := NewManager(customConfig)
	if m2.config.HeartbeatInterval != 5*time.Second {
		t.Errorf("Expected custom heartbeat interval 5s, got %v", m2.config.HeartbeatInterval)
	}
}

// TestAgentRegistration tests Agent registration functionality.
// TestAgentRegistration 测试 Agent 注册功能。
func TestAgentRegistration(t *testing.T) {
	m := NewManager(nil)
	mockUpdater := newMockHostUpdater()
	m.SetHostUpdater(mockUpdater)

	ctx := context.Background()

	// Register an Agent
	// 注册一个 Agent
	req := &pb.RegisterRequest{
		AgentId:      "agent-001",
		Hostname:     "host-001",
		IpAddress:    "192.168.1.100",
		OsType:       "linux",
		Arch:         "amd64",
		AgentVersion: "1.0.0",
		SystemInfo: &pb.SystemInfo{
			CpuCores:    4,
			TotalMemory: 8 * 1024 * 1024 * 1024,
			TotalDisk:   100 * 1024 * 1024 * 1024,
		},
	}

	conn, err := m.RegisterAgent(ctx, req)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	if conn.AgentID != "agent-001" {
		t.Errorf("Expected agent ID 'agent-001', got '%s'", conn.AgentID)
	}
	if conn.Status != AgentStatusConnected {
		t.Errorf("Expected status 'connected', got '%s'", conn.Status)
	}

	// Verify agent can be retrieved
	// 验证可以获取 Agent
	retrieved, ok := m.GetAgent("agent-001")
	if !ok {
		t.Fatal("Expected to find registered agent")
	}
	if retrieved.IPAddress != "192.168.1.100" {
		t.Errorf("Expected IP '192.168.1.100', got '%s'", retrieved.IPAddress)
	}

	// Verify agent can be retrieved by IP
	// 验证可以通过 IP 获取 Agent
	byIP, ok := m.GetAgentByIP("192.168.1.100")
	if !ok {
		t.Fatal("Expected to find agent by IP")
	}
	if byIP.AgentID != "agent-001" {
		t.Errorf("Expected agent ID 'agent-001', got '%s'", byIP.AgentID)
	}

	// Verify host updater was called
	// 验证主机更新器被调用
	if mockUpdater.getAgentStatus("agent-001") != "installed" {
		t.Error("Expected host updater to be called with 'installed' status")
	}
}

// TestAgentUnregistration tests Agent unregistration functionality.
// TestAgentUnregistration 测试 Agent 注销功能。
func TestAgentUnregistration(t *testing.T) {
	m := NewManager(nil)
	ctx := context.Background()

	// Register an Agent
	// 注册一个 Agent
	req := &pb.RegisterRequest{
		AgentId:   "agent-002",
		IpAddress: "192.168.1.101",
	}
	_, err := m.RegisterAgent(ctx, req)
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// Verify agent exists
	// 验证 Agent 存在
	_, ok := m.GetAgent("agent-002")
	if !ok {
		t.Fatal("Expected to find registered agent")
	}

	// Unregister agent
	// 注销 Agent
	m.UnregisterAgent("agent-002")

	// Verify agent is removed
	// 验证 Agent 已被移除
	_, ok = m.GetAgent("agent-002")
	if ok {
		t.Error("Expected agent to be removed after unregistration")
	}
}

// TestHeartbeatHandling tests heartbeat processing.
// TestHeartbeatHandling 测试心跳处理。
func TestHeartbeatHandling(t *testing.T) {
	m := NewManager(nil)
	mockUpdater := newMockHostUpdater()
	m.SetHostUpdater(mockUpdater)

	ctx := context.Background()

	// Register an Agent
	// 注册一个 Agent
	regReq := &pb.RegisterRequest{
		AgentId:   "agent-003",
		IpAddress: "192.168.1.102",
	}
	conn, _ := m.RegisterAgent(ctx, regReq)

	// Record initial heartbeat time
	// 记录初始心跳时间
	initialHeartbeat := conn.LastHeartbeat

	// Wait a bit
	// 等待一会儿
	time.Sleep(10 * time.Millisecond)

	// Send heartbeat
	// 发送心跳
	hbReq := &pb.HeartbeatRequest{
		AgentId:   "agent-003",
		Timestamp: time.Now().UnixMilli(),
		ResourceUsage: &pb.ResourceUsage{
			CpuUsage:    50.0,
			MemoryUsage: 60.0,
			DiskUsage:   70.0,
		},
	}

	err := m.HandleHeartbeat(ctx, hbReq)
	if err != nil {
		t.Fatalf("Failed to handle heartbeat: %v", err)
	}

	// Verify heartbeat was updated
	// 验证心跳已更新
	conn, _ = m.GetAgent("agent-003")
	if !conn.LastHeartbeat.After(initialHeartbeat) {
		t.Error("Expected heartbeat timestamp to be updated")
	}
}

// TestHeartbeatTimeout tests the heartbeat timeout detection.
// TestHeartbeatTimeout 测试心跳超时检测。
// Requirements: 3.4 - Marks hosts as offline if no heartbeat received for timeout period.
func TestHeartbeatTimeout(t *testing.T) {
	// Use short timeout for testing
	// 使用短超时进行测试
	config := &ManagerConfig{
		HeartbeatTimeout: 50 * time.Millisecond,
		CheckInterval:    10 * time.Millisecond,
	}
	m := NewManager(config)
	mockUpdater := newMockHostUpdater()
	m.SetHostUpdater(mockUpdater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register an Agent
	// 注册一个 Agent
	regReq := &pb.RegisterRequest{
		AgentId:   "agent-004",
		IpAddress: "192.168.1.103",
	}
	_, _ = m.RegisterAgent(ctx, regReq)

	// Start the manager
	// 启动管理器
	err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer m.Stop()

	// Wait for timeout to occur
	// 等待超时发生
	time.Sleep(100 * time.Millisecond)

	// Verify agent is marked offline
	// 验证 Agent 被标记为离线
	conn, ok := m.GetAgent("agent-004")
	if !ok {
		t.Fatal("Expected to find agent")
	}
	if conn.GetStatus() != AgentStatusOffline {
		t.Errorf("Expected agent status 'offline', got '%s'", conn.GetStatus())
	}

	// Verify host updater was called to mark offline
	// 验证主机更新器被调用以标记离线
	offlineAgents := mockUpdater.getOfflineAgents()
	found := false
	for _, id := range offlineAgents {
		if id == "agent-004" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected host updater to be called to mark agent offline")
	}
}

// TestAgentOnlineStatus tests the IsOnline method.
// TestAgentOnlineStatus 测试 IsOnline 方法。
func TestAgentOnlineStatus(t *testing.T) {
	conn := &AgentConnection{
		AgentID:       "test-agent",
		Status:        AgentStatusConnected,
		LastHeartbeat: time.Now(),
	}

	// Should be online with recent heartbeat
	// 最近有心跳应该在线
	if !conn.IsOnline(30 * time.Second) {
		t.Error("Expected agent to be online with recent heartbeat")
	}

	// Set old heartbeat
	// 设置旧心跳
	conn.LastHeartbeat = time.Now().Add(-60 * time.Second)
	if conn.IsOnline(30 * time.Second) {
		t.Error("Expected agent to be offline with old heartbeat")
	}

	// Set disconnected status
	// 设置断开连接状态
	conn.Status = AgentStatusDisconnected
	conn.LastHeartbeat = time.Now()
	if conn.IsOnline(30 * time.Second) {
		t.Error("Expected agent to be offline when disconnected")
	}
}

// TestManagerStartStop tests the Start and Stop methods.
// TestManagerStartStop 测试 Start 和 Stop 方法。
func TestManagerStartStop(t *testing.T) {
	m := NewManager(nil)

	ctx := context.Background()

	// Start manager
	// 启动管理器
	err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	if !m.IsRunning() {
		t.Error("Expected manager to be running after Start")
	}

	// Start again should be no-op
	// 再次启动应该是空操作
	err = m.Start(ctx)
	if err != nil {
		t.Fatalf("Second start should not fail: %v", err)
	}

	// Stop manager
	// 停止管理器
	m.Stop()

	if m.IsRunning() {
		t.Error("Expected manager to not be running after Stop")
	}

	// Stop again should be no-op
	// 再次停止应该是空操作
	m.Stop()
}

// TestListAgents tests the ListAgents method.
// TestListAgents 测试 ListAgents 方法。
func TestListAgents(t *testing.T) {
	m := NewManager(nil)
	ctx := context.Background()

	// Register multiple agents
	// 注册多个 Agent
	for i := 0; i < 3; i++ {
		req := &pb.RegisterRequest{
			AgentId:   "agent-list-" + string(rune('a'+i)),
			IpAddress: "192.168.1." + string(rune('1'+i)),
		}
		_, _ = m.RegisterAgent(ctx, req)
	}

	agents := m.ListAgents()
	if len(agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(agents))
	}
}

// TestGetConnectedCount tests the GetConnectedCount method.
// TestGetConnectedCount 测试 GetConnectedCount 方法。
func TestGetConnectedCount(t *testing.T) {
	m := NewManager(nil)
	ctx := context.Background()

	// Register agents
	// 注册 Agent
	for i := 0; i < 3; i++ {
		req := &pb.RegisterRequest{
			AgentId:   "agent-count-" + string(rune('a'+i)),
			IpAddress: "192.168.2." + string(rune('1'+i)),
		}
		_, _ = m.RegisterAgent(ctx, req)
	}

	count := m.GetConnectedCount()
	if count != 3 {
		t.Errorf("Expected 3 connected agents, got %d", count)
	}

	// Disconnect one agent
	// 断开一个 Agent
	conn, _ := m.GetAgent("agent-count-a")
	conn.SetStatus(AgentStatusDisconnected)

	count = m.GetConnectedCount()
	if count != 2 {
		t.Errorf("Expected 2 connected agents after disconnect, got %d", count)
	}
}

// TestHandleDisconnect tests the HandleDisconnect method.
// TestHandleDisconnect 测试 HandleDisconnect 方法。
func TestHandleDisconnect(t *testing.T) {
	m := NewManager(nil)
	mockUpdater := newMockHostUpdater()
	m.SetHostUpdater(mockUpdater)

	ctx := context.Background()

	// Register an Agent
	// 注册一个 Agent
	req := &pb.RegisterRequest{
		AgentId:   "agent-disconnect",
		IpAddress: "192.168.3.1",
	}
	_, _ = m.RegisterAgent(ctx, req)

	// Handle disconnect
	// 处理断开连接
	m.HandleDisconnect("agent-disconnect")

	// Verify status is disconnected
	// 验证状态为断开连接
	conn, ok := m.GetAgent("agent-disconnect")
	if !ok {
		t.Fatal("Expected to find agent")
	}
	if conn.GetStatus() != AgentStatusDisconnected {
		t.Errorf("Expected status 'disconnected', got '%s'", conn.GetStatus())
	}

	// Verify host was marked offline
	// 验证主机被标记为离线
	if mockUpdater.getAgentStatus("agent-disconnect") != "offline" {
		t.Error("Expected host to be marked offline")
	}
}

// TestHeartbeatNotFound tests heartbeat handling for non-existent agent.
// TestHeartbeatNotFound 测试不存在的 Agent 的心跳处理。
func TestHeartbeatNotFound(t *testing.T) {
	m := NewManager(nil)
	ctx := context.Background()

	hbReq := &pb.HeartbeatRequest{
		AgentId: "non-existent-agent",
	}

	err := m.HandleHeartbeat(ctx, hbReq)
	if err != ErrAgentNotFound {
		t.Errorf("Expected ErrAgentNotFound, got %v", err)
	}
}

// TestHeartbeatRestoresConnectedStatus verifies that a later heartbeat can recover
// an Agent from offline/disconnected state back to connected.
// TestHeartbeatRestoresConnectedStatus 验证后续心跳可将 Agent 从 offline/disconnected 恢复为 connected。
func TestHeartbeatRestoresConnectedStatus(t *testing.T) {
	m := NewManager(nil)
	ctx := context.Background()

	regReq := &pb.RegisterRequest{
		AgentId:   "agent-recover",
		IpAddress: "192.168.1.200",
	}
	_, _ = m.RegisterAgent(ctx, regReq)

	conn, ok := m.GetAgent("agent-recover")
	if !ok {
		t.Fatal("Expected to find agent")
	}

	conn.SetStatus(AgentStatusOffline)

	hbReq := &pb.HeartbeatRequest{
		AgentId:   "agent-recover",
		Timestamp: time.Now().UnixMilli(),
	}

	if err := m.HandleHeartbeat(ctx, hbReq); err != nil {
		t.Fatalf("Failed to handle heartbeat: %v", err)
	}

	if conn.GetStatus() != AgentStatusConnected {
		t.Errorf("Expected status 'connected' after heartbeat, got '%s'", conn.GetStatus())
	}
}
