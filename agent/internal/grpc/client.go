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

// Package grpc provides the gRPC client for Agent to communicate with Control Plane.
// grpc 包提供 Agent 与 Control Plane 通信的 gRPC 客户端。
package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/config"
	"github.com/LeonYoah/stx/agent/internal/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Default values for exponential backoff
// 指数退避的默认值
const (
	DefaultInitialBackoff = 1 * time.Second  // 初始退避时间
	DefaultMaxBackoff     = 60 * time.Second // 最大退避时间
	DefaultBackoffFactor  = 2.0              // 退避因子
)

// ExponentialBackoff implements exponential backoff reconnection strategy
// ExponentialBackoff 实现指数退避重连策略
type ExponentialBackoff struct {
	InitialInterval time.Duration // 初始间隔
	MaxInterval     time.Duration // 最大间隔
	Factor          float64       // 退避因子
	attempt         int           // 当前尝试次数
	mu              sync.Mutex    // 互斥锁
}

// NewExponentialBackoff creates a new ExponentialBackoff with default values
// NewExponentialBackoff 使用默认值创建新的 ExponentialBackoff
func NewExponentialBackoff() *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialInterval: DefaultInitialBackoff,
		MaxInterval:     DefaultMaxBackoff,
		Factor:          DefaultBackoffFactor,
		attempt:         0,
	}
}

// NextBackoff returns the next backoff duration
// NextBackoff 返回下一次退避时间
// Formula: delay = min(MaxInterval, InitialInterval * Factor^(attempt-1))
// 公式：delay = min(最大间隔, 初始间隔 * 因子^(尝试次数-1))
func (b *ExponentialBackoff) NextBackoff() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.attempt++

	// Calculate backoff: InitialInterval * Factor^(attempt-1)
	// 计算退避时间：初始间隔 * 因子^(尝试次数-1)
	backoff := float64(b.InitialInterval)
	for i := 1; i < b.attempt; i++ {
		backoff *= b.Factor
	}

	duration := time.Duration(backoff)
	if duration > b.MaxInterval {
		duration = b.MaxInterval
	}

	return duration
}

// CalculateBackoff calculates the backoff duration for a given attempt number
// CalculateBackoff 计算给定尝试次数的退避时间
// This is a pure function for testing purposes
// 这是一个用于测试的纯函数
func CalculateBackoff(attempt int, initialInterval, maxInterval time.Duration, factor float64) time.Duration {
	if attempt <= 0 {
		return initialInterval
	}

	// Calculate backoff: InitialInterval * Factor^(attempt-1)
	// 计算退避时间：初始间隔 * 因子^(尝试次数-1)
	backoff := float64(initialInterval)
	for i := 1; i < attempt; i++ {
		backoff *= factor
	}

	duration := time.Duration(backoff)
	if duration > maxInterval {
		duration = maxInterval
	}

	return duration
}

// Reset resets the backoff to initial state
// Reset 重置退避到初始状态
func (b *ExponentialBackoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.attempt = 0
}

// Attempt returns the current attempt number
// Attempt 返回当前尝试次数
func (b *ExponentialBackoff) Attempt() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.attempt
}

// CommandHandler is a function type for handling commands from Control Plane
// CommandHandler 是处理来自 Control Plane 指令的函数类型
type CommandHandler func(ctx context.Context, cmd *pb.CommandRequest) (*pb.CommandResponse, error)

// Client is the gRPC client for Agent to communicate with Control Plane
// Client 是 Agent 与 Control Plane 通信的 gRPC 客户端
type Client struct {
	config          *config.Config                                                  // Agent 配置
	conn            *grpc.ClientConn                                                // gRPC 连接
	client          pb.AgentServiceClient                                           // gRPC 客户端
	agentID         string                                                          // Agent ID
	backoff         *ExponentialBackoff                                             // 指数退避
	mu              sync.RWMutex                                                    // 读写锁
	connected       bool                                                            // 连接状态
	stopCh          chan struct{}                                                   // 停止信号通道
	heartbeatTicker *time.Ticker                                                    // 心跳定时器
	heartbeatMu     sync.Mutex                                                      // 心跳锁
	lastHeartbeat   time.Time                                                       // 最后心跳时间
	cmdStream       grpc.BidiStreamingClient[pb.CommandResponse, pb.CommandRequest] // 命令流
	cmdStreamMu     sync.Mutex                                                      // 命令流锁
}

// GetDiagnosticsLogCursors fetches diagnostics log cursors from Control Plane.
// GetDiagnosticsLogCursors 从 Control Plane 拉取诊断日志游标。
func (c *Client) GetDiagnosticsLogCursors(ctx context.Context) (*pb.DiagnosticsCursorResponse, error) {
	c.mu.RLock()
	client := c.client
	agentID := c.agentID
	c.mu.RUnlock()

	if client == nil {
		return nil, errors.New("client not connected")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return &pb.DiagnosticsCursorResponse{}, nil
	}
	return client.GetDiagnosticsLogCursors(ctx, &pb.DiagnosticsCursorRequest{AgentId: agentID})
}

// NewClient creates a new gRPC client
// NewClient 创建新的 gRPC 客户端
func NewClient(cfg *config.Config) *Client {
	return &Client{
		config:  cfg,
		agentID: cfg.Agent.ID,
		backoff: NewExponentialBackoff(),
		stopCh:  make(chan struct{}),
	}
}

// Connect establishes connection to Control Plane
// Connect 建立与 Control Plane 的连接
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// Try each address in the list
	// 尝试列表中的每个地址
	var lastErr error
	for _, addr := range c.config.ControlPlane.Addresses {
		conn, err := c.dialWithOptions(ctx, addr)
		if err != nil {
			lastErr = err
			continue
		}

		c.conn = conn
		c.client = pb.NewAgentServiceClient(conn)
		c.connected = true
		c.backoff.Reset()
		return nil
	}

	return fmt.Errorf("failed to connect to any Control Plane address: %w", lastErr)
}

// dialWithOptions creates a gRPC connection with appropriate options
// dialWithOptions 使用适当的选项创建 gRPC 连接
func (c *Client) dialWithOptions(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	// Configure TLS if enabled
	// 如果启用则配置 TLS
	if c.config.ControlPlane.TLS.Enabled {
		tlsConfig, err := c.loadTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Add authentication token if provided
	// 如果提供则添加认证 token
	if c.config.ControlPlane.Token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(&tokenAuth{token: c.config.ControlPlane.Token}))
	}

	return grpc.DialContext(ctx, addr, opts...)
}

// loadTLSConfig loads TLS configuration from files for one-way TLS (CA required) or optional mTLS.
// loadTLSConfig 从文件加载 TLS：单向 TLS 必须有 CA；客户端证书可选（mTLS）。
func (c *Client) loadTLSConfig() (*tls.Config, error) {
	tlsCfg := c.config.ControlPlane.TLS

	// One-way TLS requires a CA to trust the Control Plane server certificate.
	// 单向 TLS 需要 CA 以信任 Control Plane 服务端证书。
	if tlsCfg.CAFile == "" {
		return nil, errors.New("control_plane.tls.ca_file is required when TLS is enabled")
	}
	caCert, err := os.ReadFile(tlsCfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}
	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to append CA certificate")
	}

	// Optional client certificate for mTLS; both cert and key must be set together.
	// 可选的 mTLS 客户端证书；cert 与 key 必须同时配置。
	var certificates []tls.Certificate
	certSet := tlsCfg.CertFile != ""
	keySet := tlsCfg.KeyFile != ""
	if certSet != keySet {
		return nil, errors.New("control_plane.tls.cert_file and key_file must both be set for mTLS")
	}
	if certSet && keySet {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		certificates = append(certificates, cert)
	}

	return &tls.Config{
		Certificates: certificates,
		RootCAs:      rootCAs,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// tokenAuth implements grpc.PerRPCCredentials for token authentication
// tokenAuth 实现 grpc.PerRPCCredentials 用于 token 认证
type tokenAuth struct {
	token string
}

func (t *tokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + t.token,
	}, nil
}

func (t *tokenAuth) RequireTransportSecurity() bool {
	return false // Allow insecure for development / 开发环境允许不安全连接
}

// Disconnect closes the connection to Control Plane
// Disconnect 关闭与 Control Plane 的连接
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop heartbeat ticker
	// 停止心跳定时器
	c.heartbeatMu.Lock()
	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
		c.heartbeatTicker = nil
	}
	c.heartbeatMu.Unlock()

	// Close stop channel
	// 关闭停止信号通道
	select {
	case <-c.stopCh:
		// Already closed / 已经关闭
	default:
		close(c.stopCh)
	}

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.client = nil
		c.connected = false
		return err
	}

	c.connected = false
	return nil
}

// IsConnected returns whether the client is connected
// IsConnected 返回客户端是否已连接
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Reconnect attempts to reconnect with exponential backoff
// Reconnect 使用指数退避尝试重连
func (c *Client) Reconnect(ctx context.Context) error {
	// Disconnect first
	// 先断开连接
	_ = c.Disconnect()

	// Reset stop channel
	// 重置停止信号通道
	c.mu.Lock()
	c.stopCh = make(chan struct{})
	c.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Calculate backoff duration
		// 计算退避时间
		backoffDuration := c.backoff.NextBackoff()

		// Wait for backoff duration
		// 等待退避时间
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoffDuration):
		}

		// Try to connect
		// 尝试连接
		err := c.Connect(ctx)
		if err == nil {
			return nil
		}

		// Log reconnection attempt
		// 记录重连尝试
		logger.WarnF(ctx, "Reconnection attempt %d failed: %v, next retry in %v",
			c.backoff.Attempt(), err, c.backoff.NextBackoff())
	}
}

// GetClient returns the underlying gRPC client
// GetClient 返回底层 gRPC 客户端
func (c *Client) GetClient() pb.AgentServiceClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// GetAgentID returns the Agent ID
// GetAgentID 返回 Agent ID
func (c *Client) GetAgentID() string {
	return c.agentID
}

// SetAgentID sets the Agent ID (used after registration)
// SetAgentID 设置 Agent ID（注册后使用）
func (c *Client) SetAgentID(id string) {
	c.agentID = id
}

// Register sends a registration request to Control Plane
// Register 向 Control Plane 发送注册请求
func (c *Client) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, errors.New("client not connected")
	}

	resp, err := client.Register(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("registration failed: %w", err)
	}

	// Update agent ID if assigned by Control Plane
	// 如果 Control Plane 分配了 ID 则更新
	if resp.Success && resp.AssignedId != "" {
		c.SetAgentID(resp.AssignedId)
	}

	return resp, nil
}

// SendHeartbeat sends a heartbeat to Control Plane
// SendHeartbeat 向 Control Plane 发送心跳
func (c *Client) SendHeartbeat(ctx context.Context, usage *pb.ResourceUsage, processes []*pb.ProcessStatus) (*pb.HeartbeatResponse, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, errors.New("client not connected")
	}

	req := &pb.HeartbeatRequest{
		AgentId:       c.agentID,
		Timestamp:     time.Now().UnixMilli(),
		ResourceUsage: usage,
		Processes:     processes,
	}

	resp, err := client.Heartbeat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("heartbeat failed: %w", err)
	}

	// Update last heartbeat time
	// 更新最后心跳时间
	c.heartbeatMu.Lock()
	c.lastHeartbeat = time.Now()
	c.heartbeatMu.Unlock()

	return resp, nil
}

// StartHeartbeat starts the heartbeat timer
// StartHeartbeat 启动心跳定时器
func (c *Client) StartHeartbeat(ctx context.Context, interval time.Duration, getUsage func() (*pb.ResourceUsage, []*pb.ProcessStatus)) {
	c.heartbeatMu.Lock()
	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
	}
	c.heartbeatTicker = time.NewTicker(interval)
	c.heartbeatMu.Unlock()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case <-c.heartbeatTicker.C:
				usage, processes := getUsage()
				_, err := c.SendHeartbeat(ctx, usage, processes)
				if err != nil {
					// Log error
					// 记录错误
					logger.ErrorF(ctx, "Heartbeat failed: %v", err)
				}
			}
		}
	}()
}

// StopHeartbeat stops the heartbeat timer
// StopHeartbeat 停止心跳定时器
func (c *Client) StopHeartbeat() {
	c.heartbeatMu.Lock()
	defer c.heartbeatMu.Unlock()

	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
		c.heartbeatTicker = nil
	}
}

// GetLastHeartbeat returns the last heartbeat time
// GetLastHeartbeat 返回最后心跳时间
func (c *Client) GetLastHeartbeat() time.Time {
	c.heartbeatMu.Lock()
	defer c.heartbeatMu.Unlock()
	return c.lastHeartbeat
}

// SendLogEntries sends a batch of log entries to Control Plane through LogStream.
// SendLogEntries 通过 LogStream 向 Control Plane 发送一批日志条目。
func (c *Client) SendLogEntries(ctx context.Context, entries []*pb.LogEntry) error {
	c.mu.RLock()
	client := c.client
	agentID := c.agentID
	c.mu.RUnlock()

	if client == nil {
		return errors.New("client not connected")
	}
	if len(entries) == 0 {
		return nil
	}

	stream, err := client.LogStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to create log stream: %w", err)
	}

	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.AgentId == "" {
			entry.AgentId = agentID
		}
		if entry.Timestamp <= 0 {
			entry.Timestamp = time.Now().UnixMilli()
		}
		if err := stream.Send(entry); err != nil {
			return fmt.Errorf("failed to send log entry: %w", err)
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("failed to close log stream: %w", err)
	}
	if !resp.Success {
		return errors.New("control plane rejected log entries")
	}

	return nil
}

// StartCommandStream starts the bidirectional command stream
// StartCommandStream 启动双向指令流
func (c *Client) StartCommandStream(ctx context.Context, handler CommandHandler) error {
	c.mu.RLock()
	client := c.client
	agentID := c.agentID
	c.mu.RUnlock()

	if client == nil {
		return errors.New("client not connected")
	}

	if agentID == "" {
		return errors.New("agent ID not set, please register first")
	}

	// Create bidirectional stream
	// 创建双向流
	stream, err := client.CommandStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to create command stream: %w", err)
	}

	// Save stream for later use (e.g., sending process events)
	// 保存 stream 以便后续使用（如发送进程事件）
	c.cmdStreamMu.Lock()
	c.cmdStream = stream
	c.cmdStreamMu.Unlock()

	// Send initial message with Agent ID to identify ourselves
	// 发送包含 Agent ID 的初始消息来标识自己
	initMsg := &pb.CommandResponse{
		CommandId: "AGENT_INIT",
		Output:    agentID, // Agent ID is sent in the output field
		Status:    pb.CommandStatus_SUCCESS,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := stream.Send(initMsg); err != nil {
		return fmt.Errorf("failed to send init message: %w", err)
	}

	logger.InfoF(ctx, "Command stream established successfully for agent %s / 命令流建立成功，Agent: %s", agentID, agentID)

	// Start goroutine to receive commands and send responses
	// 启动 goroutine 接收指令并发送响应
	for {
		select {
		case <-ctx.Done():
			c.cmdStreamMu.Lock()
			c.cmdStream = nil
			c.cmdStreamMu.Unlock()
			return ctx.Err()
		case <-c.stopCh:
			c.cmdStreamMu.Lock()
			c.cmdStream = nil
			c.cmdStreamMu.Unlock()
			return nil
		default:
		}

		// Receive command from Control Plane
		// 从 Control Plane 接收指令
		cmd, err := stream.Recv()
		if err != nil {
			// Log error and exit (in production, handle reconnection)
			// 记录错误并退出（生产环境处理重连）
			return fmt.Errorf("command stream receive error: %w", err)
		}

		// Handle command in a separate goroutine
		// 在单独的 goroutine 中处理指令
		go func(cmd *pb.CommandRequest) {
			// Execute command handler
			// 执行指令处理器
			resp, err := handler(ctx, cmd)
			if err != nil {
				// Create error response
				// 创建错误响应
				resp = &pb.CommandResponse{
					CommandId: cmd.CommandId,
					Status:    pb.CommandStatus_FAILED,
					Error:     err.Error(),
					Timestamp: time.Now().UnixMilli(),
				}
			}

			// Send response back to Control Plane
			// 将响应发送回 Control Plane
			if sendErr := stream.Send(resp); sendErr != nil {
				logger.ErrorF(ctx, "Failed to send command response: %v", sendErr)
			}
		}(cmd)
	}
}

// ReportCommandResult sends a command execution result to Control Plane
// ReportCommandResult 向 Control Plane 发送指令执行结果
func (c *Client) ReportCommandResult(ctx context.Context, resp *pb.CommandResponse) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return errors.New("client not connected")
	}

	// Create a new stream for reporting
	// 创建新的流用于上报
	stream, err := client.CommandStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to create command stream: %w", err)
	}

	// Send the response
	// 发送响应
	if err := stream.Send(resp); err != nil {
		return fmt.Errorf("failed to send command result: %w", err)
	}

	return nil
}

// HeartbeatTracker tracks heartbeat timing for testing
// HeartbeatTracker 跟踪心跳时间用于测试
type HeartbeatTracker struct {
	mu         sync.Mutex
	timestamps []time.Time
}

// NewHeartbeatTracker creates a new HeartbeatTracker
// NewHeartbeatTracker 创建新的 HeartbeatTracker
func NewHeartbeatTracker() *HeartbeatTracker {
	return &HeartbeatTracker{
		timestamps: make([]time.Time, 0),
	}
}

// Record records a heartbeat timestamp
// Record 记录心跳时间戳
func (t *HeartbeatTracker) Record() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timestamps = append(t.timestamps, time.Now())
}

// GetTimestamps returns all recorded timestamps
// GetTimestamps 返回所有记录的时间戳
func (t *HeartbeatTracker) GetTimestamps() []time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]time.Time, len(t.timestamps))
	copy(result, t.timestamps)
	return result
}

// GetIntervals returns the intervals between heartbeats
// GetIntervals 返回心跳之间的间隔
func (t *HeartbeatTracker) GetIntervals() []time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.timestamps) < 2 {
		return nil
	}

	intervals := make([]time.Duration, len(t.timestamps)-1)
	for i := 1; i < len(t.timestamps); i++ {
		intervals[i-1] = t.timestamps[i].Sub(t.timestamps[i-1])
	}
	return intervals
}

// Clear clears all recorded timestamps
// Clear 清除所有记录的时间戳
func (t *HeartbeatTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timestamps = make([]time.Time, 0)
}

// ReportProcessEvent sends a process event to Control Plane.
// ReportProcessEvent 向 Control Plane 发送进程事件。
func (c *Client) ReportProcessEvent(ctx context.Context, event *pb.ProcessEventReport) error {
	c.cmdStreamMu.Lock()
	stream := c.cmdStream
	c.cmdStreamMu.Unlock()

	if stream == nil {
		return errors.New("command stream not established, cannot send process event")
	}

	// Serialize event to JSON / 将事件序列化为 JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal process event: %w", err)
	}

	// Send the event as a command response with special command ID
	// 将事件作为带有特殊命令 ID 的命令响应发送
	resp := &pb.CommandResponse{
		CommandId: "PROCESS_EVENT_REPORT",
		Output:    string(eventJSON),
		Status:    pb.CommandStatus_SUCCESS,
		Timestamp: time.Now().UnixMilli(),
	}

	c.cmdStreamMu.Lock()
	err = stream.Send(resp)
	c.cmdStreamMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send process event: %w", err)
	}

	return nil
}
