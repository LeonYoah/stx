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

// Package agent provides Agent connection management for the STX Control Plane.
// agent 包提供 STX Control Plane 的 Agent 连接管理功能。
package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

// Default configuration values
// 默认配置值
const (
	// DefaultHeartbeatInterval is the default interval for Agent heartbeats.
	// DefaultHeartbeatInterval 是 Agent 心跳的默认间隔。
	DefaultHeartbeatInterval = 10 * time.Second

	// DefaultHeartbeatTimeout is the default timeout for considering an Agent offline.
	// DefaultHeartbeatTimeout 是判断 Agent 离线的默认超时时间。
	DefaultHeartbeatTimeout = 30 * time.Second

	// DefaultCheckInterval is the default interval for checking heartbeat timeouts.
	// DefaultCheckInterval 是检查心跳超时的默认间隔。
	DefaultCheckInterval = 5 * time.Second
)

// AgentStatus represents the connection status of an Agent.
// AgentStatus 表示 Agent 的连接状态。
type AgentStatus string

const (
	// AgentStatusConnected indicates the Agent is connected and active.
	// AgentStatusConnected 表示 Agent 已连接且活跃。
	AgentStatusConnected AgentStatus = "connected"
	// AgentStatusDisconnected indicates the Agent has disconnected.
	// AgentStatusDisconnected 表示 Agent 已断开连接。
	AgentStatusDisconnected AgentStatus = "disconnected"
	// AgentStatusOffline indicates the Agent is offline (heartbeat timeout).
	// AgentStatusOffline 表示 Agent 离线（心跳超时）。
	AgentStatusOffline AgentStatus = "offline"
)

// Errors for Agent Manager operations
// Agent Manager 操作的错误定义
var (
	// ErrAgentNotFound indicates the Agent was not found.
	// ErrAgentNotFound 表示未找到 Agent。
	ErrAgentNotFound = errors.New("agent: agent not found")
	// ErrAgentNotConnected indicates the Agent is not connected.
	// ErrAgentNotConnected 表示 Agent 未连接。
	ErrAgentNotConnected = errors.New("agent: agent not connected")
	// ErrCommandTimeout indicates the command execution timed out.
	// ErrCommandTimeout 表示命令执行超时。
	ErrCommandTimeout = errors.New("agent: command execution timeout")
	// ErrStreamNotAvailable indicates the command stream is not available.
	// ErrStreamNotAvailable 表示命令流不可用。
	ErrStreamNotAvailable = errors.New("agent: command stream not available")
)

// AgentConnection represents an active connection to an Agent.
// AgentConnection 表示与 Agent 的活跃连接。
// Requirements: 1.2 - Manages bidirectional gRPC stream connection.
type AgentConnection struct {
	// AgentID is the unique identifier of the Agent.
	// AgentID 是 Agent 的唯一标识符。
	AgentID string

	// HostID is the ID of the host this Agent is running on.
	// HostID 是此 Agent 运行所在主机的 ID。
	HostID uint

	// IPAddress is the IP address of the Agent.
	// IPAddress 是 Agent 的 IP 地址。
	IPAddress string

	// Hostname is the hostname of the Agent.
	// Hostname 是 Agent 的主机名。
	Hostname string

	// Version is the Agent version.
	// Version 是 Agent 版本。
	Version string

	// Stream is the bidirectional gRPC stream for commands.
	// Stream 是用于命令的双向 gRPC 流。
	Stream grpc.BidiStreamingServer[pb.CommandResponse, pb.CommandRequest]

	// LastHeartbeat is the timestamp of the last heartbeat received.
	// LastHeartbeat 是收到的最后一次心跳的时间戳。
	LastHeartbeat time.Time

	// Status is the current connection status.
	// Status 是当前连接状态。
	Status AgentStatus

	// ConnectedAt is the timestamp when the Agent connected.
	// ConnectedAt 是 Agent 连接的时间戳。
	ConnectedAt time.Time

	// mu protects concurrent access to the connection.
	// mu 保护对连接的并发访问。
	mu sync.RWMutex
}

// IsOnline checks if the Agent is online based on heartbeat timeout.
// IsOnline 根据心跳超时检查 Agent 是否在线。
func (c *AgentConnection) IsOnline(timeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Status == AgentStatusConnected && time.Since(c.LastHeartbeat) <= timeout
}

// UpdateHeartbeat updates the last heartbeat timestamp.
// UpdateHeartbeat 更新最后心跳时间戳。
func (c *AgentConnection) UpdateHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastHeartbeat = time.Now()
}

// SetStatus sets the connection status.
// SetStatus 设置连接状态。
func (c *AgentConnection) SetStatus(status AgentStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Status = status
}

// GetStatus returns the current connection status.
// GetStatus 返回当前连接状态。
func (c *AgentConnection) GetStatus() AgentStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Status
}

// SetStream sets the command stream for the connection.
// SetStream 设置连接的命令流。
func (c *AgentConnection) SetStream(stream grpc.BidiStreamingServer[pb.CommandResponse, pb.CommandRequest]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Stream = stream
}

// GetStream returns the command stream.
// GetStream 返回命令流。
func (c *AgentConnection) GetStream() grpc.BidiStreamingServer[pb.CommandResponse, pb.CommandRequest] {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Stream
}

// CommandContext represents the context for a pending command.
// CommandContext 表示待处理命令的上下文。
type CommandContext struct {
	// CommandID is the unique identifier of the command.
	// CommandID 是命令的唯一标识符。
	CommandID string

	// AgentID is the ID of the Agent executing the command.
	// AgentID 是执行命令的 Agent 的 ID。
	AgentID string

	// Type is the command type.
	// Type 是命令类型。
	Type pb.CommandType

	// Parameters are the command parameters.
	// Parameters 是命令参数。
	Parameters map[string]string

	// Timeout is the command timeout duration.
	// Timeout 是命令超时时间。
	Timeout time.Duration

	// CreatedAt is when the command was created.
	// CreatedAt 是命令创建的时间。
	CreatedAt time.Time

	// ResultChan is the channel for receiving the command result.
	// ResultChan 是接收命令结果的通道。
	ResultChan chan *pb.CommandResponse

	// Done indicates if the command has completed.
	// Done 表示命令是否已完成。
	Done bool

	// LastStatus is the last known status of the command.
	// LastStatus 是命令的最后已知状态。
	LastStatus pb.CommandStatus

	// LastProgress is the last known progress (0-100).
	// LastProgress 是最后已知的进度 (0-100)。
	LastProgress int32

	// LastOutput is the last known output message.
	// LastOutput 是最后已知的输出消息。
	LastOutput string

	// LastError is the last known error message.
	// LastError 是最后已知的错误消息。
	LastError string

	// mu protects concurrent access.
	// mu 保护并发访问。
	mu sync.RWMutex
}

// MarkDone marks the command as done.
// MarkDone 将命令标记为已完成。
func (c *CommandContext) MarkDone() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Done = true
}

// IsDone checks if the command is done.
// IsDone 检查命令是否已完成。
func (c *CommandContext) IsDone() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Done
}

// HostStatusUpdater is an interface for updating host status.
// HostStatusUpdater 是更新主机状态的接口。
// This interface decouples the Agent Manager from the Host Service.
// 此接口将 Agent Manager 与 Host Service 解耦。
type HostStatusUpdater interface {
	// UpdateAgentStatus updates the agent status for a host by IP address.
	// UpdateAgentStatus 根据 IP 地址更新主机的 Agent 状态。
	// hostname is used when auto-creating a host (e.g. no host matches by IP).
	UpdateAgentStatus(ctx context.Context, ipAddress string, agentID string, version string, systemInfo *SystemInfo, hostname string) (hostID uint, err error)

	// UpdateHeartbeat updates the heartbeat data for a host.
	// UpdateHeartbeat 更新主机的心跳数据。
	UpdateHeartbeat(ctx context.Context, agentID string, cpuUsage, memoryUsage, diskUsage float64) error

	// MarkHostOffline marks a host as offline by agent ID.
	// MarkHostOffline 根据 Agent ID 将主机标记为离线。
	MarkHostOffline(ctx context.Context, agentID string) error
}

// SystemInfo represents system information from an Agent.
// SystemInfo 表示来自 Agent 的系统信息。
type SystemInfo struct {
	OSType      string
	Arch        string
	CPUCores    int
	TotalMemory int64
	TotalDisk   int64
}

// ManagerConfig holds configuration for the Agent Manager.
// ManagerConfig 保存 Agent Manager 的配置。
type ManagerConfig struct {
	// HeartbeatInterval is the expected interval between heartbeats.
	// HeartbeatInterval 是心跳之间的预期间隔。
	HeartbeatInterval time.Duration

	// HeartbeatTimeout is the timeout for considering an Agent offline.
	// HeartbeatTimeout 是判断 Agent 离线的超时时间。
	HeartbeatTimeout time.Duration

	// CheckInterval is the interval for checking heartbeat timeouts.
	// CheckInterval 是检查心跳超时的间隔。
	CheckInterval time.Duration
}

// Manager manages Agent connections and command dispatching.
// Manager 管理 Agent 连接和命令分发。
// Requirements: 1.2, 1.5 - Implements Agent connection management and command dispatching.
type Manager struct {
	// agents stores active Agent connections by agent ID.
	// agents 按 Agent ID 存储活跃的 Agent 连接。
	agents sync.Map // map[string]*AgentConnection

	// commands stores pending commands by command ID.
	// commands 按命令 ID 存储待处理的命令。
	commands sync.Map // map[string]*CommandContext

	// hostUpdater is used to update host status.
	// hostUpdater 用于更新主机状态。
	hostUpdater HostStatusUpdater

	// config holds the manager configuration.
	// config 保存管理器配置。
	config *ManagerConfig

	// stopChan is used to signal the manager to stop.
	// stopChan 用于通知管理器停止。
	stopChan chan struct{}

	// running indicates if the manager is running.
	// running 表示管理器是否正在运行。
	running bool

	// mu protects the running state.
	// mu 保护运行状态。
	mu sync.RWMutex
}

// NewManager creates a new Agent Manager instance.
// NewManager 创建一个新的 Agent Manager 实例。
func NewManager(config *ManagerConfig) *Manager {
	if config == nil {
		config = &ManagerConfig{}
	}

	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if config.HeartbeatTimeout <= 0 {
		config.HeartbeatTimeout = DefaultHeartbeatTimeout
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = DefaultCheckInterval
	}

	return &Manager{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// SetHostUpdater sets the host status updater.
// SetHostUpdater 设置主机状态更新器。
func (m *Manager) SetHostUpdater(updater HostStatusUpdater) {
	m.hostUpdater = updater
}

// Start starts the Agent Manager background tasks.
// Start 启动 Agent Manager 后台任务。
// Requirements: 3.4 - Starts heartbeat timeout detection goroutine.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = true
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	// Start heartbeat timeout checker
	// 启动心跳超时检查器
	go m.heartbeatChecker(ctx)

	return nil
}

// Stop stops the Agent Manager and all background tasks.
// Stop 停止 Agent Manager 和所有后台任务。
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	m.running = false
	close(m.stopChan)
}

// IsRunning returns whether the manager is running.
// IsRunning 返回管理器是否正在运行。
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// RegisterAgent registers a new Agent connection.
// RegisterAgent 注册一个新的 Agent 连接。
// Requirements: 1.2 - Handles Agent registration and connection management.
func (m *Manager) RegisterAgent(ctx context.Context, req *pb.RegisterRequest) (*AgentConnection, error) {
	// Create new connection
	// 创建新连接
	conn := &AgentConnection{
		AgentID:       req.AgentId,
		IPAddress:     req.IpAddress,
		Hostname:      req.Hostname,
		Version:       req.AgentVersion,
		Status:        AgentStatusConnected,
		ConnectedAt:   time.Now(),
		LastHeartbeat: time.Now(),
	}

	// Update host status if updater is available
	// 如果更新器可用，更新主机状态
	if m.hostUpdater != nil {
		var sysInfo *SystemInfo
		if req.SystemInfo != nil {
			sysInfo = &SystemInfo{
				CPUCores:    int(req.SystemInfo.CpuCores),
				TotalMemory: req.SystemInfo.TotalMemory,
				TotalDisk:   req.SystemInfo.TotalDisk,
			}
		}

		hostID, err := m.hostUpdater.UpdateAgentStatus(ctx, req.IpAddress, req.AgentId, req.AgentVersion, sysInfo, req.Hostname)
		if err != nil {
			// Log error but don't fail registration
			// 记录错误但不使注册失败
			// The Agent can still connect even if host update fails
			// 即使主机更新失败，Agent 仍然可以连接
		} else {
			conn.HostID = hostID
		}
	}

	// Store connection
	// 存储连接
	m.agents.Store(req.AgentId, conn)

	return conn, nil
}

// UnregisterAgent removes an Agent connection.
// UnregisterAgent 移除一个 Agent 连接。
func (m *Manager) UnregisterAgent(agentID string) {
	if conn, ok := m.agents.Load(agentID); ok {
		agentConn := conn.(*AgentConnection)
		agentConn.SetStatus(AgentStatusDisconnected)
	}
	m.agents.Delete(agentID)
}

// GetAgent retrieves an Agent connection by ID.
// GetAgent 根据 ID 获取 Agent 连接。
func (m *Manager) GetAgent(agentID string) (*AgentConnection, bool) {
	if conn, ok := m.agents.Load(agentID); ok {
		return conn.(*AgentConnection), true
	}
	return nil, false
}

// GetAgentByIP retrieves an Agent connection by IP address.
// GetAgentByIP 根据 IP 地址获取 Agent 连接。
func (m *Manager) GetAgentByIP(ipAddress string) (*AgentConnection, bool) {
	var found *AgentConnection
	m.agents.Range(func(key, value any) bool {
		conn := value.(*AgentConnection)
		if conn.IPAddress == ipAddress {
			found = conn
			return false // Stop iteration
		}
		return true
	})
	return found, found != nil
}

// ListAgents returns all connected Agents.
// ListAgents 返回所有已连接的 Agent。
func (m *Manager) ListAgents() []*AgentConnection {
	var agents []*AgentConnection
	m.agents.Range(func(key, value any) bool {
		agents = append(agents, value.(*AgentConnection))
		return true
	})
	return agents
}

// GetConnectedCount returns the number of connected Agents.
// GetConnectedCount 返回已连接的 Agent 数量。
func (m *Manager) GetConnectedCount() int {
	count := 0
	m.agents.Range(func(key, value any) bool {
		conn := value.(*AgentConnection)
		if conn.GetStatus() == AgentStatusConnected {
			count++
		}
		return true
	})
	return count
}

// HandleHeartbeat processes a heartbeat from an Agent.
// HandleHeartbeat 处理来自 Agent 的心跳。
// Requirements: 1.3 - Updates heartbeat timestamp and resource usage.
func (m *Manager) HandleHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) error {
	conn, ok := m.GetAgent(req.AgentId)
	if !ok {
		return ErrAgentNotFound
	}

	// Heartbeat proves the Agent is reachable again; recover status from offline/disconnected.
	// 心跳表明 Agent 已重新可达；将状态从 offline/disconnected 恢复为 connected。
	conn.SetStatus(AgentStatusConnected)

	// Update heartbeat timestamp
	// 更新心跳时间戳
	conn.UpdateHeartbeat()

	// Update host heartbeat data if updater is available
	// 如果更新器可用，更新主机心跳数据
	if m.hostUpdater != nil && req.ResourceUsage != nil {
		err := m.hostUpdater.UpdateHeartbeat(
			ctx,
			req.AgentId,
			req.ResourceUsage.CpuUsage,
			req.ResourceUsage.MemoryUsage,
			req.ResourceUsage.DiskUsage,
		)
		if err != nil {
			// Log error but don't fail heartbeat
			// 记录错误但不使心跳失败
		}
	}

	return nil
}

// SetAgentStream sets the command stream for an Agent.
// SetAgentStream 设置 Agent 的命令流。
// Requirements: 1.5 - Establishes bidirectional stream for command dispatching.
func (m *Manager) SetAgentStream(agentID string, stream grpc.BidiStreamingServer[pb.CommandResponse, pb.CommandRequest]) error {
	conn, ok := m.GetAgent(agentID)
	if !ok {
		return ErrAgentNotFound
	}

	// A valid command stream means the Agent is connected and can accept commands again.
	// 有效的命令流意味着 Agent 已重新连接并可再次接收命令。
	conn.SetStatus(AgentStatusConnected)
	conn.SetStream(stream)
	return nil
}

// SendCommand sends a command to an Agent and waits for the result.
// SendCommand 向 Agent 发送命令并等待结果。
// Requirements: 1.5 - Implements command dispatching and result receiving.
func (m *Manager) SendCommand(ctx context.Context, agentID string, cmdType pb.CommandType, params map[string]string, timeout time.Duration) (*pb.CommandResponse, error) {
	conn, ok := m.GetAgent(agentID)
	if !ok {
		return nil, ErrAgentNotFound
	}

	if conn.GetStatus() != AgentStatusConnected {
		return nil, ErrAgentNotConnected
	}

	stream := conn.GetStream()
	if stream == nil {
		return nil, ErrStreamNotAvailable
	}

	// Generate command ID
	// 生成命令 ID
	commandID := uuid.New().String()

	// Create command context
	// 创建命令上下文
	cmdCtx := &CommandContext{
		CommandID:  commandID,
		AgentID:    agentID,
		Type:       cmdType,
		Parameters: params,
		Timeout:    timeout,
		CreatedAt:  time.Now(),
		ResultChan: make(chan *pb.CommandResponse, 1),
	}

	// Store command context
	// 存储命令上下文
	m.commands.Store(commandID, cmdCtx)
	defer m.commands.Delete(commandID)

	// Create command request
	// 创建命令请求
	cmdReq := &pb.CommandRequest{
		CommandId:  commandID,
		Type:       cmdType,
		Parameters: params,
		Timeout:    int32(timeout.Seconds()),
	}

	// Send command through stream
	// 通过流发送命令
	if err := stream.Send(cmdReq); err != nil {
		return nil, err
	}

	// Wait for result with timeout
	// 带超时等待结果
	select {
	case result := <-cmdCtx.ResultChan:
		return result, nil
	case <-time.After(timeout):
		cmdCtx.MarkDone()
		return nil, ErrCommandTimeout
	case <-ctx.Done():
		cmdCtx.MarkDone()
		return nil, ctx.Err()
	}
}

// SendCommandAsync sends a command to an Agent without waiting for the result.
// SendCommandAsync 向 Agent 发送命令但不等待结果。
func (m *Manager) SendCommandAsync(agentID string, cmdType pb.CommandType, params map[string]string, timeout time.Duration) (string, error) {
	conn, ok := m.GetAgent(agentID)
	if !ok {
		return "", ErrAgentNotFound
	}

	if conn.GetStatus() != AgentStatusConnected {
		return "", ErrAgentNotConnected
	}

	stream := conn.GetStream()
	if stream == nil {
		return "", ErrStreamNotAvailable
	}

	// Generate command ID
	// 生成命令 ID
	commandID := uuid.New().String()

	// Create command context (without result channel for async)
	// 创建命令上下文（异步不需要结果通道）
	cmdCtx := &CommandContext{
		CommandID:    commandID,
		AgentID:      agentID,
		Type:         cmdType,
		Parameters:   params,
		Timeout:      timeout,
		CreatedAt:    time.Now(),
		LastStatus:   pb.CommandStatus_RUNNING, // Initialize as running / 初始化为运行中
		LastProgress: 0,
		LastOutput:   "Command sent, waiting for response / 命令已发送，等待响应",
	}

	// Store command context
	// 存储命令上下文
	m.commands.Store(commandID, cmdCtx)

	// Create command request
	// 创建命令请求
	cmdReq := &pb.CommandRequest{
		CommandId:  commandID,
		Type:       cmdType,
		Parameters: params,
		Timeout:    int32(timeout.Seconds()),
	}

	// Send command through stream
	// 通过流发送命令
	if err := stream.Send(cmdReq); err != nil {
		m.commands.Delete(commandID)
		return "", err
	}

	return commandID, nil
}

// HandleCommandResponse processes a command response from an Agent.
// HandleCommandResponse 处理来自 Agent 的命令响应。
// Requirements: 1.5 - Receives and processes command execution results.
func (m *Manager) HandleCommandResponse(resp *pb.CommandResponse) {
	cmdCtxVal, ok := m.commands.Load(resp.CommandId)
	if !ok {
		// Command not found, might have timed out
		// 命令未找到，可能已超时
		return
	}

	cmdCtx := cmdCtxVal.(*CommandContext)

	// Check if command is already done (timed out)
	// 检查命令是否已完成（超时）
	if cmdCtx.IsDone() {
		return
	}

	// Update last known status / 更新最后已知状态
	cmdCtx.mu.Lock()
	cmdCtx.LastStatus = resp.Status
	cmdCtx.LastProgress = resp.Progress
	cmdCtx.LastOutput = resp.Output
	cmdCtx.LastError = resp.Error
	cmdCtx.mu.Unlock()

	// Send result if channel exists
	// 如果通道存在则发送结果
	if cmdCtx.ResultChan != nil {
		select {
		case cmdCtx.ResultChan <- resp:
		default:
			// Channel full or closed
			// 通道已满或已关闭
		}
	}

	// Mark as done if terminal status
	// 如果是终止状态则标记为完成
	if resp.Status == pb.CommandStatus_SUCCESS ||
		resp.Status == pb.CommandStatus_FAILED ||
		resp.Status == pb.CommandStatus_CANCELLED {
		cmdCtx.MarkDone()
		// Don't delete immediately, keep for status queries
		// 不要立即删除，保留用于状态查询
		// Schedule deletion after 5 minutes
		// 5 分钟后计划删除
		go func(commandID string) {
			time.Sleep(5 * time.Minute)
			m.commands.Delete(commandID)
		}(resp.CommandId)
	}
}

// GetCommand retrieves a command context by ID.
// GetCommand 根据 ID 获取命令上下文。
func (m *Manager) GetCommand(commandID string) (*CommandContext, bool) {
	if cmdCtx, ok := m.commands.Load(commandID); ok {
		return cmdCtx.(*CommandContext), true
	}
	return nil, false
}

// GetCommandStatus retrieves the current status of a command.
// GetCommandStatus 获取命令的当前状态。
func (m *Manager) GetCommandStatus(commandID string) (status string, progress int, message string, err error) {
	cmdCtx, ok := m.GetCommand(commandID)
	if !ok {
		return "", 0, "", ErrAgentNotFound
	}

	cmdCtx.mu.RLock()
	defer cmdCtx.mu.RUnlock()

	// Convert pb.CommandStatus to string
	// 将 pb.CommandStatus 转换为字符串
	var statusStr string
	switch cmdCtx.LastStatus {
	case pb.CommandStatus_PENDING:
		statusStr = "pending"
	case pb.CommandStatus_RUNNING:
		statusStr = "running"
	case pb.CommandStatus_SUCCESS:
		statusStr = "success"
	case pb.CommandStatus_FAILED:
		statusStr = "failed"
	case pb.CommandStatus_CANCELLED:
		statusStr = "cancelled"
	default:
		statusStr = "unknown"
	}

	// Use error message if available, otherwise use output
	// 如果有错误消息则使用错误消息，否则使用输出
	msg := cmdCtx.LastOutput
	if cmdCtx.LastError != "" {
		msg = cmdCtx.LastError
	}

	return statusStr, int(cmdCtx.LastProgress), msg, nil
}

// HandleDisconnect handles an Agent disconnection.
// HandleDisconnect 处理 Agent 断开连接。
func (m *Manager) HandleDisconnect(agentID string) {
	conn, ok := m.GetAgent(agentID)
	if !ok {
		return
	}

	conn.SetStatus(AgentStatusDisconnected)
	conn.SetStream(nil)

	// Mark host as offline if updater is available
	// 如果更新器可用，将主机标记为离线
	if m.hostUpdater != nil {
		ctx := context.Background()
		_ = m.hostUpdater.MarkHostOffline(ctx, agentID)
	}
}

// heartbeatChecker runs in the background to check for heartbeat timeouts.
// heartbeatChecker 在后台运行以检查心跳超时。
// Requirements: 3.4 - Marks hosts as offline if no heartbeat received for 30 seconds.
func (m *Manager) heartbeatChecker(ctx context.Context) {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkHeartbeatTimeouts(ctx)
		case <-m.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// checkHeartbeatTimeouts checks all Agents for heartbeat timeouts.
// checkHeartbeatTimeouts 检查所有 Agent 的心跳超时。
func (m *Manager) checkHeartbeatTimeouts(ctx context.Context) {
	m.agents.Range(func(key, value any) bool {
		conn := value.(*AgentConnection)

		// Skip if already offline or disconnected
		// 如果已经离线或断开连接则跳过
		if conn.GetStatus() != AgentStatusConnected {
			return true
		}

		// Check if heartbeat timeout exceeded
		// 检查心跳是否超时
		if !conn.IsOnline(m.config.HeartbeatTimeout) {
			conn.SetStatus(AgentStatusOffline)

			// Update host status if updater is available
			// 如果更新器可用，更新主机状态
			if m.hostUpdater != nil {
				_ = m.hostUpdater.MarkHostOffline(ctx, conn.AgentID)
			}
		}

		return true
	})
}

// GetConfig returns the manager configuration.
// GetConfig 返回管理器配置。
func (m *Manager) GetConfig() *ManagerConfig {
	return m.config
}
