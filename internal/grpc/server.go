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

// Package grpc provides the gRPC server implementation for Agent communication.
// grpc 包提供用于 Agent 通信的 gRPC 服务器实现。
package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/LeonYoah/stx/internal/apps/agent"
	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/host"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Default configuration values for gRPC server
// gRPC 服务器的默认配置值
const (
	// DefaultGRPCPort is the default port for gRPC server.
	// DefaultGRPCPort 是 gRPC 服务器的默认端口。
	DefaultGRPCPort = 17890

	// DefaultMaxRecvMsgSize is the default maximum receive message size (16MB).
	// DefaultMaxRecvMsgSize 是默认的最大接收消息大小（16MB）。
	DefaultMaxRecvMsgSize = 16 * 1024 * 1024

	// DefaultMaxSendMsgSize is the default maximum send message size (16MB).
	// DefaultMaxSendMsgSize 是默认的最大发送消息大小（16MB）。
	DefaultMaxSendMsgSize = 16 * 1024 * 1024

	// DefaultHeartbeatInterval is the default heartbeat interval for Agent configuration.
	// DefaultHeartbeatInterval 是 Agent 配置的默认心跳间隔。
	DefaultHeartbeatInterval = 10
)

// Errors for gRPC server operations
// gRPC 服务器操作的错误定义
var (
	// ErrServerNotRunning indicates the server is not running.
	// ErrServerNotRunning 表示服务器未运行。
	ErrServerNotRunning = errors.New("grpc: server is not running")

	// ErrServerAlreadyRunning indicates the server is already running.
	// ErrServerAlreadyRunning 表示服务器已在运行。
	ErrServerAlreadyRunning = errors.New("grpc: server is already running")

	// ErrInvalidTLSConfig indicates invalid TLS configuration.
	// ErrInvalidTLSConfig 表示无效的 TLS 配置。
	ErrInvalidTLSConfig = errors.New("grpc: invalid TLS configuration")
)

// ServerConfig holds configuration for the gRPC server.
// ServerConfig 保存 gRPC 服务器的配置。
// Requirements: 8.1 - Configures gRPC server options including TLS and interceptors.
type ServerConfig struct {
	// Port is the port number for the gRPC server.
	// Port 是 gRPC 服务器的端口号。
	Port int

	// TLSEnabled indicates whether TLS is enabled.
	// TLSEnabled 表示是否启用 TLS。
	TLSEnabled bool

	// CertFile is the path to the TLS certificate file.
	// CertFile 是 TLS 证书文件的路径。
	CertFile string

	// KeyFile is the path to the TLS key file.
	// KeyFile 是 TLS 密钥文件的路径。
	KeyFile string

	// CAFile is the path to the CA certificate file for client verification.
	// CAFile 是用于客户端验证的 CA 证书文件路径。
	CAFile string

	// MaxRecvMsgSize is the maximum receive message size in bytes.
	// MaxRecvMsgSize 是最大接收消息大小（字节）。
	MaxRecvMsgSize int

	// MaxSendMsgSize is the maximum send message size in bytes.
	// MaxSendMsgSize 是最大发送消息大小（字节）。
	MaxSendMsgSize int

	// HeartbeatInterval is the heartbeat interval to send to Agents (seconds).
	// HeartbeatInterval 是发送给 Agent 的心跳间隔（秒）。
	HeartbeatInterval int
}

// Server represents the gRPC server for Agent communication.
// Server 表示用于 Agent 通信的 gRPC 服务器。
// Requirements: 8.1 - Implements gRPC server with TLS support and interceptors.
type Server struct {
	pb.UnimplementedAgentServiceServer

	// config holds the server configuration.
	// config 保存服务器配置。
	config *ServerConfig

	// grpcServer is the underlying gRPC server instance.
	// grpcServer 是底层的 gRPC 服务器实例。
	grpcServer *grpc.Server

	// agentManager manages Agent connections.
	// agentManager 管理 Agent 连接。
	agentManager *agent.Manager

	// hostService provides host management operations.
	// hostService 提供主机管理操作。
	hostService *host.Service

	// auditRepo provides audit log operations.
	// auditRepo 提供审计日志操作。
	auditRepo *audit.Repository

	// logger is the logger instance.
	// logger 是日志记录器实例。
	logger *zap.Logger

	// running indicates if the server is running.
	// running 表示服务器是否正在运行。
	running bool

	// listener is the network listener.
	// listener 是网络监听器。
	listener net.Listener
}

// NewServer creates a new gRPC server instance.
// NewServer 创建一个新的 gRPC 服务器实例。
// Requirements: 8.1 - Creates gRPC server with configuration.
func NewServer(config *ServerConfig, agentManager *agent.Manager, hostService *host.Service, auditRepo *audit.Repository, logger *zap.Logger) *Server {
	if config == nil {
		config = &ServerConfig{}
	}

	// Set default values
	// 设置默认值
	if config.Port <= 0 {
		config.Port = DefaultGRPCPort
	}
	if config.MaxRecvMsgSize <= 0 {
		config.MaxRecvMsgSize = DefaultMaxRecvMsgSize
	}
	if config.MaxSendMsgSize <= 0 {
		config.MaxSendMsgSize = DefaultMaxSendMsgSize
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = DefaultHeartbeatInterval
	}

	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	return &Server{
		config:       config,
		agentManager: agentManager,
		hostService:  hostService,
		auditRepo:    auditRepo,
		logger:       logger,
	}
}

// Start starts the gRPC server.
// Start 启动 gRPC 服务器。
// Requirements: 8.1 - Starts gRPC server with configured options.
func (s *Server) Start(ctx context.Context) error {
	if s.running {
		return ErrServerAlreadyRunning
	}

	// Build server options
	// 构建服务器选项
	opts, err := s.buildServerOptions()
	if err != nil {
		return fmt.Errorf("failed to build server options: %w", err)
	}

	// Create gRPC server
	// 创建 gRPC 服务器
	s.grpcServer = grpc.NewServer(opts...)

	// Register AgentService
	// 注册 AgentService
	pb.RegisterAgentServiceServer(s.grpcServer, s)

	// Create listener
	// 创建监听器
	addr := fmt.Sprintf(":%d", s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	s.running = true
	s.logger.Info("gRPC server starting",
		zap.Int("port", s.config.Port),
		zap.Bool("tls_enabled", s.config.TLSEnabled),
	)

	// Start serving in a goroutine
	// 在 goroutine 中启动服务
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			s.logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully stops the gRPC server.
// Stop 优雅地停止 gRPC 服务器。
func (s *Server) Stop() {
	if !s.running {
		return
	}

	s.logger.Info("Stopping gRPC server")

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	s.running = false
	s.logger.Info("gRPC server stopped")
}

// IsRunning returns whether the server is running.
// IsRunning 返回服务器是否正在运行。
func (s *Server) IsRunning() bool {
	return s.running
}

// GetPort returns the server port.
// GetPort 返回服务器端口。
func (s *Server) GetPort() int {
	return s.config.Port
}

// buildServerOptions builds gRPC server options based on configuration.
// buildServerOptions 根据配置构建 gRPC 服务器选项。
func (s *Server) buildServerOptions() ([]grpc.ServerOption, error) {
	var opts []grpc.ServerOption

	// Add message size options
	// 添加消息大小选项
	opts = append(opts,
		grpc.MaxRecvMsgSize(s.config.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(s.config.MaxSendMsgSize),
	)

	// Add keepalive options
	// 添加 keepalive 选项
	opts = append(opts,
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Minute,
			Time:                  5 * time.Minute,
			Timeout:               20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	)

	// Add TLS credentials if enabled
	// 如果启用则添加 TLS 凭证
	if s.config.TLSEnabled {
		creds, err := s.loadTLSCredentials()
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(creds))
	}

	// Add interceptors
	// 添加拦截器
	opts = append(opts,
		grpc.ChainUnaryInterceptor(
			s.loggingUnaryInterceptor,
			s.recoveryUnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			s.loggingStreamInterceptor,
			s.recoveryStreamInterceptor,
		),
	)

	return opts, nil
}

// loadTLSCredentials loads TLS credentials from files.
// loadTLSCredentials 从文件加载 TLS 凭证。
func (s *Server) loadTLSCredentials() (credentials.TransportCredentials, error) {
	// Load server certificate and key
	// 加载服务器证书和密钥
	cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Load CA certificate for client verification if provided
	// 如果提供了 CA 证书则加载用于客户端验证
	if s.config.CAFile != "" {
		caCert, err := os.ReadFile(s.config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, ErrInvalidTLSConfig
		}

		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return credentials.NewTLS(tlsConfig), nil
}

// loggingUnaryInterceptor logs unary RPC calls.
// loggingUnaryInterceptor 记录一元 RPC 调用。
func (s *Server) loggingUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()

	// Get peer info
	// 获取对端信息
	peerAddr := "unknown"
	if p, ok := peer.FromContext(ctx); ok {
		peerAddr = p.Addr.String()
	}

	// Call handler
	// 调用处理器
	resp, err := handler(ctx, req)

	// Log the call
	// 记录调用
	duration := time.Since(start)
	if err != nil {
		s.logger.Warn("gRPC unary call failed",
			zap.String("method", info.FullMethod),
			zap.String("peer", peerAddr),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
	} else {
		s.logger.Debug("gRPC unary call completed",
			zap.String("method", info.FullMethod),
			zap.String("peer", peerAddr),
			zap.Duration("duration", duration),
		)
	}

	return resp, err
}

// recoveryUnaryInterceptor recovers from panics in unary handlers.
// recoveryUnaryInterceptor 从一元处理器的 panic 中恢复。
func (s *Server) recoveryUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("gRPC unary handler panic",
				zap.String("method", info.FullMethod),
				zap.Any("panic", r),
			)
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(ctx, req)
}

// loggingStreamInterceptor logs stream RPC calls.
// loggingStreamInterceptor 记录流式 RPC 调用。
func (s *Server) loggingStreamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()

	// Get peer info
	// 获取对端信息
	peerAddr := "unknown"
	if p, ok := peer.FromContext(ss.Context()); ok {
		peerAddr = p.Addr.String()
	}

	s.logger.Debug("gRPC stream started",
		zap.String("method", info.FullMethod),
		zap.String("peer", peerAddr),
	)

	// Call handler
	// 调用处理器
	err := handler(srv, ss)

	// Log the call
	// 记录调用
	duration := time.Since(start)
	if err != nil {
		// Use debug level for expected errors (InvalidArgument, NotFound)
		// 对于预期的错误（InvalidArgument, NotFound）使用 debug 级别
		code := status.Code(err)
		if code == codes.InvalidArgument || code == codes.NotFound {
			s.logger.Debug("gRPC stream ended with expected error",
				zap.String("method", info.FullMethod),
				zap.String("peer", peerAddr),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			s.logger.Warn("gRPC stream ended with error",
				zap.String("method", info.FullMethod),
				zap.String("peer", peerAddr),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		}
	} else {
		s.logger.Debug("gRPC stream ended",
			zap.String("method", info.FullMethod),
			zap.String("peer", peerAddr),
			zap.Duration("duration", duration),
		)
	}

	return err
}

// recoveryStreamInterceptor recovers from panics in stream handlers.
// recoveryStreamInterceptor 从流式处理器的 panic 中恢复。
func (s *Server) recoveryStreamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("gRPC stream handler panic",
				zap.String("method", info.FullMethod),
				zap.Any("panic", r),
			)
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(srv, ss)
}
