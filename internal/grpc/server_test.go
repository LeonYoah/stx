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

package grpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/agent"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// testServer wraps the gRPC server for testing.
// testServer 包装 gRPC 服务器用于测试。
type testServer struct {
	server       *Server
	grpcServer   *grpc.Server
	listener     *bufconn.Listener
	agentManager *agent.Manager
	serveErrCh   chan error
}

// newTestServer creates a new test server with in-memory connection.
// newTestServer 创建一个使用内存连接的测试服务器。
func newTestServer(t *testing.T) *testServer {
	listener := bufconn.Listen(bufSize)

	agentManager := agent.NewManager(nil)
	logger, _ := zap.NewDevelopment()

	config := &ServerConfig{
		Port:              9000,
		HeartbeatInterval: 10,
	}

	server := NewServer(config, agentManager, nil, nil, logger)

	// Create gRPC server manually for testing
	// 手动创建 gRPC 服务器用于测试
	grpcServer := grpc.NewServer()
	pb.RegisterAgentServiceServer(grpcServer, server)
	serveErrCh := make(chan error, 1)

	go func() {
		if err := grpcServer.Serve(listener); err != nil &&
			!errors.Is(err, grpc.ErrServerStopped) {
			serveErrCh <- err
		}
		close(serveErrCh)
	}()

	return &testServer{
		server:       server,
		grpcServer:   grpcServer,
		listener:     listener,
		agentManager: agentManager,
		serveErrCh:   serveErrCh,
	}
}

// dial creates a client connection to the test server.
// dial 创建到测试服务器的客户端连接。
func (ts *testServer) dial(ctx context.Context) (*grpc.ClientConn, error) {
	return grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return ts.listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// close closes the test server.
// close 关闭测试服务器。
func (ts *testServer) close() {
	if ts.grpcServer != nil {
		ts.grpcServer.Stop()
	}
	if ts.listener != nil {
		_ = ts.listener.Close()
	}
	select {
	case err, ok := <-ts.serveErrCh:
		if ok {
			panic(err)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

// getFreeTCPPort allocates a free TCP port for test servers.
// getFreeTCPPort 为测试服务器分配一个空闲 TCP 端口。
func getFreeTCPPort(t *testing.T) int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

// TestNewServer tests server creation with various configurations.
// TestNewServer 测试使用各种配置创建服务器。
func TestNewServer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	agentManager := agent.NewManager(nil)

	t.Run("with nil config uses defaults", func(t *testing.T) {
		server := NewServer(nil, agentManager, nil, nil, logger)
		assert.NotNil(t, server)
		assert.Equal(t, DefaultGRPCPort, server.config.Port)
		assert.Equal(t, DefaultMaxRecvMsgSize, server.config.MaxRecvMsgSize)
		assert.Equal(t, DefaultMaxSendMsgSize, server.config.MaxSendMsgSize)
		assert.Equal(t, DefaultHeartbeatInterval, server.config.HeartbeatInterval)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &ServerConfig{
			Port:              9001,
			MaxRecvMsgSize:    1024,
			MaxSendMsgSize:    2048,
			HeartbeatInterval: 5,
		}
		server := NewServer(config, agentManager, nil, nil, logger)
		assert.NotNil(t, server)
		assert.Equal(t, 9001, server.config.Port)
		assert.Equal(t, 1024, server.config.MaxRecvMsgSize)
		assert.Equal(t, 2048, server.config.MaxSendMsgSize)
		assert.Equal(t, 5, server.config.HeartbeatInterval)
	})

	t.Run("with nil logger creates default", func(t *testing.T) {
		server := NewServer(nil, agentManager, nil, nil, nil)
		assert.NotNil(t, server)
		assert.NotNil(t, server.logger)
	})
}

// TestServerStartStop tests server start and stop operations.
// TestServerStartStop 测试服务器启动和停止操作。
func TestServerStartStop(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	agentManager := agent.NewManager(nil)

	t.Run("start and stop server", func(t *testing.T) {
		config := &ServerConfig{
			Port: getFreeTCPPort(t),
		}
		server := NewServer(config, agentManager, nil, nil, logger)

		ctx := context.Background()
		err := server.Start(ctx)
		require.NoError(t, err)
		assert.True(t, server.IsRunning())

		// Give server time to start
		// 给服务器启动时间
		time.Sleep(100 * time.Millisecond)

		server.Stop()
		assert.False(t, server.IsRunning())
	})

	t.Run("double start returns error", func(t *testing.T) {
		config := &ServerConfig{
			Port: getFreeTCPPort(t),
		}
		server := NewServer(config, agentManager, nil, nil, logger)

		ctx := context.Background()
		err := server.Start(ctx)
		require.NoError(t, err)
		defer server.Stop()

		err = server.Start(ctx)
		assert.Equal(t, ErrServerAlreadyRunning, err)
	})

	t.Run("stop when not running is safe", func(t *testing.T) {
		config := &ServerConfig{
			Port: getFreeTCPPort(t),
		}
		server := NewServer(config, agentManager, nil, nil, logger)

		// Should not panic
		// 不应该 panic
		server.Stop()
		assert.False(t, server.IsRunning())
	})
}

// TestRegisterRPC tests the Register RPC method.
// TestRegisterRPC 测试 Register RPC 方法。
func TestRegisterRPC(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ts.dial(ctx)
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)

	t.Run("successful registration", func(t *testing.T) {
		req := &pb.RegisterRequest{
			AgentId:      "test-agent-001",
			Hostname:     "test-host",
			IpAddress:    "192.168.1.100",
			OsType:       "linux",
			Arch:         "amd64",
			AgentVersion: "1.0.0",
			SystemInfo: &pb.SystemInfo{
				CpuCores:      4,
				TotalMemory:   8 * 1024 * 1024 * 1024,
				TotalDisk:     100 * 1024 * 1024 * 1024,
				KernelVersion: "5.4.0",
			},
		}

		resp, err := client.Register(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "test-agent-001", resp.AssignedId)
		assert.NotNil(t, resp.Config)
		assert.Equal(t, int32(10), resp.Config.HeartbeatInterval)

		// Verify agent is registered in manager
		// 验证 Agent 已在管理器中注册
		agentConn, ok := ts.agentManager.GetAgent("test-agent-001")
		assert.True(t, ok)
		assert.Equal(t, "192.168.1.100", agentConn.IPAddress)
		assert.Equal(t, "test-host", agentConn.Hostname)
	})

	t.Run("registration without agent_id auto assigns id", func(t *testing.T) {
		req := &pb.RegisterRequest{
			Hostname:  "test-host",
			IpAddress: "192.168.1.101",
		}

		resp, err := client.Register(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.AssignedId)
		assert.NotNil(t, resp.Config)

		agentConn, ok := ts.agentManager.GetAgent(resp.AssignedId)
		assert.True(t, ok)
		assert.Equal(t, "192.168.1.101", agentConn.IPAddress)
		assert.Equal(t, "test-host", agentConn.Hostname)
	})

	t.Run("registration without ip_address fails", func(t *testing.T) {
		req := &pb.RegisterRequest{
			AgentId:  "test-agent-002",
			Hostname: "test-host",
		}

		resp, err := client.Register(ctx, req)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Message, "ip_address is required")
	})
}

// TestHeartbeatRPC tests the Heartbeat RPC method.
// TestHeartbeatRPC 测试 Heartbeat RPC 方法。
func TestHeartbeatRPC(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ts.dial(ctx)
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)

	// First register an agent
	// 首先注册一个 Agent
	regReq := &pb.RegisterRequest{
		AgentId:      "heartbeat-test-agent",
		Hostname:     "test-host",
		IpAddress:    "192.168.1.200",
		AgentVersion: "1.0.0",
	}
	_, err = client.Register(ctx, regReq)
	require.NoError(t, err)

	t.Run("successful heartbeat", func(t *testing.T) {
		req := &pb.HeartbeatRequest{
			AgentId:   "heartbeat-test-agent",
			Timestamp: time.Now().UnixMilli(),
			ResourceUsage: &pb.ResourceUsage{
				CpuUsage:        25.5,
				MemoryUsage:     60.0,
				DiskUsage:       45.0,
				AvailableMemory: 4 * 1024 * 1024 * 1024,
				AvailableDisk:   50 * 1024 * 1024 * 1024,
			},
		}

		resp, err := client.Heartbeat(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Greater(t, resp.ServerTime, int64(0))
	})

	t.Run("heartbeat without agent_id fails", func(t *testing.T) {
		req := &pb.HeartbeatRequest{
			Timestamp: time.Now().UnixMilli(),
		}

		_, err := client.Heartbeat(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent_id is required")
	})

	t.Run("heartbeat for unregistered agent fails", func(t *testing.T) {
		req := &pb.HeartbeatRequest{
			AgentId:   "non-existent-agent",
			Timestamp: time.Now().UnixMilli(),
		}

		_, err := client.Heartbeat(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestServerConfig tests server configuration.
// TestServerConfig 测试服务器配置。
func TestServerConfig(t *testing.T) {
	t.Run("default values are set correctly", func(t *testing.T) {
		config := &ServerConfig{}
		logger, _ := zap.NewDevelopment()
		server := NewServer(config, nil, nil, nil, logger)

		assert.Equal(t, DefaultGRPCPort, server.GetPort())
	})

	t.Run("custom port is preserved", func(t *testing.T) {
		config := &ServerConfig{
			Port: 8888,
		}
		logger, _ := zap.NewDevelopment()
		server := NewServer(config, nil, nil, nil, logger)

		assert.Equal(t, 8888, server.GetPort())
	})
}

// TestExtractAgentIDFromResponse tests the agent ID extraction function.
// TestExtractAgentIDFromResponse 测试 Agent ID 提取函数。
func TestExtractAgentIDFromResponse(t *testing.T) {
	t.Run("extracts ID from empty command_id", func(t *testing.T) {
		resp := &pb.CommandResponse{
			CommandId: "",
			Output:    "agent-123",
		}
		agentID := extractAgentIDFromResponse(resp)
		assert.Equal(t, "agent-123", agentID)
	})

	t.Run("extracts ID from AGENT_INIT command", func(t *testing.T) {
		resp := &pb.CommandResponse{
			CommandId: "AGENT_INIT",
			Output:    "agent-456",
		}
		agentID := extractAgentIDFromResponse(resp)
		assert.Equal(t, "agent-456", agentID)
	})

	t.Run("returns empty for regular command", func(t *testing.T) {
		resp := &pb.CommandResponse{
			CommandId: "cmd-789",
			Output:    "some output",
		}
		agentID := extractAgentIDFromResponse(resp)
		assert.Empty(t, agentID)
	})
}
