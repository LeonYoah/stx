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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/agent"
	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/diagnostics"
	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/processidentity"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Register handles Agent registration requests.
// Register 处理 Agent 注册请求。
// Requirements: 1.1, 3.2 - Handles Agent registration, matches host IP, updates Agent status.
func (s *Server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	s.logger.Info("Agent registration request received",
		zap.String("agent_id", req.AgentId),
		zap.String("hostname", req.Hostname),
		zap.String("ip_address", req.IpAddress),
		zap.String("version", req.AgentVersion),
	)

	// Validate request
	// 验证请求
	if req.IpAddress == "" {
		return &pb.RegisterResponse{
			Success: false,
			Message: "ip_address is required",
		}, nil
	}

	// Generate agent_id if not provided (first-time registration)
	// 如果未提供 agent_id，则生成一个（首次注册）
	if req.AgentId == "" {
		req.AgentId = generateAgentID(req.Hostname, req.IpAddress)
		s.logger.Info("Generated agent_id for new Agent",
			zap.String("agent_id", req.AgentId),
			zap.String("hostname", req.Hostname),
			zap.String("ip_address", req.IpAddress),
		)
	}

	// Register Agent with manager
	// 向管理器注册 Agent
	conn, err := s.agentManager.RegisterAgent(ctx, req)
	if err != nil {
		s.logger.Error("Failed to register Agent",
			zap.String("agent_id", req.AgentId),
			zap.Error(err),
		)
		return &pb.RegisterResponse{
			Success: false,
			Message: "failed to register agent: " + err.Error(),
		}, nil
	}

	// Try to match with existing host by IP address
	// 尝试通过 IP 地址匹配现有主机
	if s.hostService != nil {
		var sysInfo *host.SystemInfo
		if req.SystemInfo != nil {
			sysInfo = &host.SystemInfo{
				OSType:      req.OsType,
				Arch:        req.Arch,
				CPUCores:    int(req.SystemInfo.CpuCores),
				TotalMemory: req.SystemInfo.TotalMemory,
				TotalDisk:   req.SystemInfo.TotalDisk,
			}
		}

		updatedHost, err := s.hostService.UpdateAgentStatus(ctx, uint(req.HostId), req.IpAddress, req.AgentId, req.AgentVersion, sysInfo, req.Hostname, req.GetIpAddresses())
		if err != nil {
			// Log warning but don't fail registration
			// 记录警告但不使注册失败
			s.logger.Warn("Failed to update host agent status",
				zap.String("ip_address", req.IpAddress),
				zap.Error(err),
			)
		} else if updatedHost != nil {
			conn.HostID = updatedHost.ID
			s.logger.Info("Agent matched with host",
				zap.String("agent_id", req.AgentId),
				zap.Uint("host_id", updatedHost.ID),
				zap.String("host_name", updatedHost.Name),
			)
		}
	}

	// Build response with configuration
	// 构建带配置的响应
	s.logger.Info("Sending heartbeat interval to Agent / 向 Agent 发送心跳间隔",
		zap.Int("heartbeat_interval", s.config.HeartbeatInterval),
		zap.String("agent_id", req.AgentId),
	)

	response := &pb.RegisterResponse{
		Success:    true,
		Message:    "registration successful",
		AssignedId: req.AgentId,
		Config: &pb.AgentConfig{
			HeartbeatInterval: int32(s.config.HeartbeatInterval),
			LogLevel:          int32(pb.LogLevel_INFO),
			Extra:             make(map[string]string),
		},
	}

	s.logger.Info("Agent registered successfully",
		zap.String("agent_id", req.AgentId),
		zap.Uint("host_id", conn.HostID),
	)

	// Push monitor config to agent after registration (async)
	// Use background context since the gRPC request context may be canceled
	// 注册后异步推送监控配置到 Agent
	// 使用后台 context，因为 gRPC 请求 context 可能会被取消
	if conn.HostID > 0 {
		go s.pushMonitorConfigToAgent(context.Background(), req.AgentId, conn.HostID)
	}

	return response, nil
}

// GetDiagnosticsLogCursors returns all diagnostics log cursors for a given agent.
// GetDiagnosticsLogCursors 返回某个 Agent 的所有诊断日志游标。
func (s *Server) GetDiagnosticsLogCursors(ctx context.Context, req *pb.DiagnosticsCursorRequest) (*pb.DiagnosticsCursorResponse, error) {
	if req == nil || req.AgentId == "" {
		return &pb.DiagnosticsCursorResponse{}, nil
	}
	if !db.IsDatabaseInitialized() {
		return &pb.DiagnosticsCursorResponse{}, nil
	}

	// Lazily construct diagnostics repository; this RPC is infrequent and read-only.
	repo := diagnostics.NewRepository(db.DB(ctx))
	cursors, err := repo.ListLogCursorsByAgent(ctx, req.AgentId)
	if err != nil {
		s.logger.Error("GetDiagnosticsLogCursors failed",
			zap.String("agent_id", req.AgentId),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to list log cursors: %v", err)
	}

	resp := &pb.DiagnosticsCursorResponse{
		Cursors: make([]*pb.DiagnosticsCursor, 0, len(cursors)),
	}
	for _, c := range cursors {
		item := &pb.DiagnosticsCursor{
			InstallDir: c.InstallDir,
			Role:       c.Role,
			SourceFile: c.SourceFile,
			Offset:     c.CursorOffset,
		}
		if c.LastOccurredAt != nil {
			item.LastOccurredAt = c.LastOccurredAt.UnixMilli()
		}
		resp.Cursors = append(resp.Cursors, item)
	}
	return resp, nil
}

// Heartbeat handles Agent heartbeat requests.
// Heartbeat 处理 Agent 心跳请求。
// Requirements: 1.3, 3.3 - Processes heartbeat, updates host resource usage.
func (s *Server) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	// Validate request
	// 验证请求
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Handle heartbeat through manager
	// 通过管理器处理心跳
	if err := s.agentManager.HandleHeartbeat(ctx, req); err != nil {
		if err == agent.ErrAgentNotFound {
			return nil, status.Error(codes.NotFound, "agent not found, please re-register")
		}
		s.logger.Error("Failed to handle heartbeat",
			zap.String("agent_id", req.AgentId),
			zap.Error(err),
		)
		return nil, status.Error(codes.Internal, "failed to process heartbeat")
	}

	// Update host heartbeat data if host service is available
	// 如果主机服务可用，更新主机心跳数据
	if s.hostService != nil && req.ResourceUsage != nil {
		if err := s.hostService.UpdateHeartbeat(
			ctx,
			req.AgentId,
			req.ResourceUsage.CpuUsage,
			req.ResourceUsage.MemoryUsage,
			req.ResourceUsage.DiskUsage,
		); err != nil {
			// Log warning but don't fail heartbeat
			// 记录警告但不使心跳失败
			s.logger.Warn("Failed to update host heartbeat",
				zap.String("agent_id", req.AgentId),
				zap.Error(err),
			)
		}
	}

	// Update process status in cluster_nodes from agent's monitored state (periodic correction).
	// When auto-monitor is on, agent tracks processes and reports current PID + alive state in heartbeat;
	// this corrects DB when it was stale (e.g. PID=0 in DB but process actually running on host).
	// 用 Agent 监控结果周期性纠正 cluster_nodes：开启自动监控时，心跳携带当前 PID 与存活状态，可纠正 DB 与主机不一致（如 DB 为 PID=0 但进程实际在跑）。
	if clusterNodeProvider != nil && len(req.Processes) > 0 {
		conn, ok := s.agentManager.GetAgent(req.AgentId)
		if ok && conn.HostID > 0 {
			go s.updateProcessStatusFromHeartbeat(context.Background(), conn.HostID, req.Processes)
		}
	}

	return &pb.HeartbeatResponse{
		Success:    true,
		ServerTime: time.Now().UnixMilli(),
	}, nil
}

// CommandStream handles bidirectional streaming for command dispatch and result reporting.
// CommandStream 处理用于命令分发和结果上报的双向流。
// Requirements: 1.5, 8.6 - Implements bidirectional stream for command dispatching.
func (s *Server) CommandStream(stream grpc.BidiStreamingServer[pb.CommandResponse, pb.CommandRequest]) error {
	// Get peer info for logging
	// 获取对端信息用于日志记录
	peerAddr := "unknown"
	if p, ok := peer.FromContext(stream.Context()); ok {
		peerAddr = p.Addr.String()
	}

	s.logger.Info("CommandStream started", zap.String("peer", peerAddr))

	// First message should identify the Agent
	// 第一条消息应该标识 Agent
	firstMsg, err := stream.Recv()
	if err != nil {
		s.logger.Error("Failed to receive first message in CommandStream",
			zap.String("peer", peerAddr),
			zap.Error(err),
		)
		return status.Error(codes.InvalidArgument, "failed to receive agent identification")
	}

	// Extract agent_id from the first response (Agent sends its ID)
	// 从第一个响应中提取 agent_id（Agent 发送其 ID）
	agentID := extractAgentIDFromResponse(firstMsg)
	if agentID == "" {
		return status.Error(codes.InvalidArgument, "agent_id not provided in first message")
	}

	// Verify Agent is registered
	// 验证 Agent 已注册
	conn, ok := s.agentManager.GetAgent(agentID)
	if !ok {
		return status.Error(codes.NotFound, "agent not registered, please register first")
	}

	// Set the stream for this Agent
	// 为此 Agent 设置流
	if err := s.agentManager.SetAgentStream(agentID, stream); err != nil {
		s.logger.Error("Failed to set agent stream",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
		return status.Error(codes.Internal, "failed to set agent stream")
	}

	s.logger.Info("CommandStream established for Agent",
		zap.String("agent_id", agentID),
		zap.Uint("host_id", conn.HostID),
	)

	// Process the first message if it contains command response data
	// 如果第一条消息包含命令响应数据，则处理它
	if firstMsg.CommandId != "" {
		s.handleCommandResponse(agentID, firstMsg)
	}

	// Main loop: receive command responses from Agent
	// 主循环：从 Agent 接收命令响应
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				s.logger.Info("CommandStream closed by Agent",
					zap.String("agent_id", agentID),
				)
			} else {
				s.logger.Error("CommandStream receive error",
					zap.String("agent_id", agentID),
					zap.Error(err),
				)
			}

			// Handle Agent disconnect
			// 处理 Agent 断开连接
			s.agentManager.HandleDisconnect(agentID)
			return err
		}

		// Process command response
		// 处理命令响应
		s.handleCommandResponse(agentID, resp)
	}
}

// handleCommandResponse processes a command response from an Agent.
// handleCommandResponse 处理来自 Agent 的命令响应。
func (s *Server) handleCommandResponse(agentID string, resp *pb.CommandResponse) {
	if resp.CommandId == "" {
		return
	}

	// Handle special command IDs / 处理特殊命令 ID
	if resp.CommandId == "PROCESS_EVENT_REPORT" {
		s.logger.Info("Received PROCESS_EVENT_REPORT / 收到 PROCESS_EVENT_REPORT",
			zap.String("agent_id", agentID),
			zap.String("output_preview", truncateString(resp.Output, 200)),
		)

		// Parse process event from output / 从输出解析进程事件
		report, err := parseProcessEventFromResponse(resp.Output)
		if err != nil {
			s.logger.Error("Failed to parse process event report / 解析进程事件报告失败",
				zap.String("agent_id", agentID),
				zap.String("output", resp.Output),
				zap.Error(err),
			)
			return
		}

		s.logger.Info("Parsed process event report / 解析进程事件报告成功",
			zap.String("agent_id", agentID),
			zap.String("event_type", report.EventType.String()),
			zap.Int32("pid", report.Pid),
			zap.String("process_name", report.ProcessName),
			zap.String("install_dir", report.InstallDir),
			zap.String("role", report.Role),
		)

		// Handle the process event / 处理进程事件
		if err := s.HandleProcessEventReport(context.Background(), agentID, report); err != nil {
			s.logger.Error("Failed to handle process event report / 处理进程事件报告失败",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
		}
		return
	}

	s.logger.Debug("Received command response",
		zap.String("agent_id", agentID),
		zap.String("command_id", resp.CommandId),
		zap.String("status", resp.Status.String()),
		zap.Int32("progress", resp.Progress),
	)

	// Forward to agent manager
	// 转发给 Agent 管理器
	s.agentManager.HandleCommandResponse(resp)

	// Update command log in audit repository
	// 在审计仓库中更新命令日志
	if s.auditRepo != nil {
		s.updateCommandLog(resp)
	}
}

// truncateString truncates a string to the specified length.
// truncateString 将字符串截断到指定长度。
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// updateCommandLog updates the command log with the response data.
// updateCommandLog 使用响应数据更新命令日志。
func (s *Server) updateCommandLog(resp *pb.CommandResponse) {
	ctx := context.Background()

	// Get existing command log
	// 获取现有命令日志
	cmdLog, err := s.auditRepo.GetCommandLogByCommandID(ctx, resp.CommandId)
	if err != nil {
		// Command log might not exist yet, which is fine
		// 命令日志可能还不存在，这是正常的
		return
	}

	// Map protobuf status to audit status
	// 将 protobuf 状态映射到审计状态
	var auditStatus audit.CommandStatus
	switch resp.Status {
	case pb.CommandStatus_PENDING:
		auditStatus = audit.CommandStatusPending
	case pb.CommandStatus_RUNNING:
		auditStatus = audit.CommandStatusRunning
	case pb.CommandStatus_SUCCESS:
		auditStatus = audit.CommandStatusSuccess
	case pb.CommandStatus_FAILED:
		auditStatus = audit.CommandStatusFailed
	case pb.CommandStatus_CANCELLED:
		auditStatus = audit.CommandStatusCancelled
	default:
		auditStatus = audit.CommandStatusPending
	}

	// Update command log
	// 更新命令日志
	updates := map[string]interface{}{
		"status":   auditStatus,
		"progress": int(resp.Progress),
	}

	if resp.Output != "" {
		updates["output"] = cmdLog.Output + resp.Output
	}

	if resp.Error != "" {
		updates["error"] = resp.Error
	}

	// Set started_at if transitioning to running
	// 如果转换为运行状态，设置 started_at
	if auditStatus == audit.CommandStatusRunning && cmdLog.StartedAt == nil {
		now := time.Now()
		updates["started_at"] = now
	}

	// Set finished_at if terminal status
	// 如果是终止状态，设置 finished_at
	if auditStatus == audit.CommandStatusSuccess ||
		auditStatus == audit.CommandStatusFailed ||
		auditStatus == audit.CommandStatusCancelled {
		now := time.Now()
		updates["finished_at"] = now
	}

	if err := s.auditRepo.UpdateCommandLogStatus(ctx, cmdLog.ID, updates); err != nil {
		s.logger.Warn("Failed to update command log",
			zap.String("command_id", resp.CommandId),
			zap.Error(err),
		)
	}
}

// LogStream handles log streaming from Agents.
// LogStream 处理来自 Agent 的日志流。
// Requirements: 10.2, 10.3 - Receives Agent logs and stores to audit log.
func (s *Server) LogStream(stream grpc.ClientStreamingServer[pb.LogEntry, pb.LogStreamResponse]) error {
	// Get peer info for logging
	// 获取对端信息用于日志记录
	peerAddr := "unknown"
	if p, ok := peer.FromContext(stream.Context()); ok {
		peerAddr = p.Addr.String()
	}

	s.logger.Debug("LogStream started", zap.String("peer", peerAddr))

	var receivedCount int64
	var agentID string

	// Receive log entries
	// 接收日志条目
	for {
		entry, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				// Client finished sending
				// 客户端完成发送
				s.logger.Debug("LogStream completed",
					zap.String("agent_id", agentID),
					zap.Int64("received_count", receivedCount),
				)
				return stream.SendAndClose(&pb.LogStreamResponse{
					Success:       true,
					ReceivedCount: receivedCount,
				})
			}

			s.logger.Error("LogStream receive error",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
			return err
		}

		// Track agent ID from first entry
		// 从第一个条目跟踪 Agent ID
		if agentID == "" {
			agentID = entry.AgentId
		}

		// Route diagnostics seatunnel error logs into diagnostics domain,
		// otherwise keep existing audit log behavior.
		// seatunnel_error 进入 diagnostics 域，其余日志保持现有审计行为。
		if isSeatunnelErrorLogEntry(entry) {
			if err := s.handleSeatunnelErrorLogEntry(stream.Context(), entry); err != nil {
				s.logger.Warn("Failed to ingest seatunnel diagnostics log entry",
					zap.String("agent_id", entry.AgentId),
					zap.Error(err),
				)
				if s.auditRepo != nil {
					s.storeLogEntry(entry)
				}
			}
		} else if s.auditRepo != nil {
			s.storeLogEntry(entry)
		}

		receivedCount++
	}
}

// storeLogEntry stores a log entry to the audit log.
// storeLogEntry 将日志条目存储到审计日志。
func (s *Server) storeLogEntry(entry *pb.LogEntry) {
	ctx := context.Background()

	// Map log level to action
	// 将日志级别映射到操作
	action := "agent_log"
	switch entry.Level {
	case pb.LogLevel_ERROR:
		action = "agent_error"
	case pb.LogLevel_WARN:
		action = "agent_warning"
	}

	// Build details from log entry
	// 从日志条目构建详情
	details := audit.AuditDetails{
		"message":    entry.Message,
		"level":      entry.Level.String(),
		"timestamp":  entry.Timestamp,
		"command_id": entry.CommandId,
	}

	// Add extra fields
	// 添加额外字段
	for k, v := range entry.Fields {
		details[k] = v
	}

	// Create audit log entry
	// 创建审计日志条目
	auditLog := &audit.AuditLog{
		Username:     "agent",
		Action:       action,
		ResourceType: "agent",
		Trigger:      "auto",
		ResourceID:   entry.AgentId,
		Details:      details,
	}

	if err := s.auditRepo.CreateAuditLog(ctx, auditLog); err != nil {
		s.logger.Warn("Failed to store log entry",
			zap.String("agent_id", entry.AgentId),
			zap.Error(err),
		)
	}
}

func isSeatunnelErrorLogEntry(entry *pb.LogEntry) bool {
	if entry == nil || entry.Fields == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(entry.Fields["source"]), "seatunnel_error")
}

func (s *Server) handleSeatunnelErrorLogEntry(ctx context.Context, entry *pb.LogEntry) error {
	if diagnosticsService == nil {
		return diagnostics.ErrDiagnosticsRepositoryUnavailable
	}
	if s == nil || s.agentManager == nil || entry == nil {
		return diagnostics.ErrInvalidSeatunnelErrorRequest
	}

	conn, ok := s.agentManager.GetAgent(entry.AgentId)
	if !ok {
		return fmt.Errorf("agent connection not found for %s", entry.AgentId)
	}

	installDir := strings.TrimSpace(entry.Fields["install_dir"])
	role := strings.TrimSpace(entry.Fields["role"])
	sourceFile := strings.TrimSpace(entry.Fields["source_file"])
	if installDir == "" || role == "" || sourceFile == "" {
		return fmt.Errorf("missing diagnostics fields for seatunnel_error entry")
	}

	var clusterID, nodeID uint
	if clusterNodeProvider != nil && conn.HostID > 0 {
		resolvedClusterID, resolvedNodeID, _, err := clusterNodeProvider.GetNodeByHostAndInstallDirAndRole(ctx, conn.HostID, installDir, role)
		if err != nil {
			return err
		}
		clusterID = resolvedClusterID
		nodeID = resolvedNodeID
	}

	occurredAt := time.UnixMilli(entry.Timestamp)
	if entry.Timestamp <= 0 {
		occurredAt = time.Now()
	}

	return diagnosticsService.IngestSeatunnelError(ctx, &diagnostics.IngestSeatunnelErrorRequest{
		ClusterID:   clusterID,
		NodeID:      nodeID,
		HostID:      conn.HostID,
		AgentID:     entry.AgentId,
		Role:        role,
		InstallDir:  installDir,
		SourceFile:  sourceFile,
		SourceKind:  strings.TrimSpace(entry.Fields["source_kind"]),
		JobID:       strings.TrimSpace(entry.Fields["job_id"]),
		OccurredAt:  occurredAt,
		Message:     strings.TrimSpace(entry.Message),
		Evidence:    strings.TrimSpace(entry.Fields["body"]),
		CursorStart: parseInt64Field(entry.Fields["cursor_start"]),
		CursorEnd:   parseInt64Field(entry.Fields["cursor_end"]),
	})
}

func parseInt64Field(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// extractAgentIDFromResponse extracts the agent ID from a command response.
// extractAgentIDFromResponse 从命令响应中提取 Agent ID。
// The Agent sends its ID in the output field of the first message.
// Agent 在第一条消息的 output 字段中发送其 ID。
func extractAgentIDFromResponse(resp *pb.CommandResponse) string {
	// Convention: Agent sends its ID in the output field of the first message
	// 约定：Agent 在第一条消息的 output 字段中发送其 ID
	// The command_id will be empty or a special value like "AGENT_INIT"
	// command_id 将为空或特殊值如 "AGENT_INIT"
	if resp.CommandId == "" || resp.CommandId == "AGENT_INIT" {
		return resp.Output
	}
	return ""
}

// generateAgentID generates a unique agent ID based on hostname and IP address.
// generateAgentID 根据主机名和 IP 地址生成唯一的 Agent ID。
func generateAgentID(hostname, ipAddress string) string {
	// Create a deterministic ID based on hostname and IP
	// 根据主机名和 IP 创建确定性 ID
	data := fmt.Sprintf("%s-%s-%d", hostname, ipAddress, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	// Use first 16 characters of hex hash as agent ID
	// 使用十六进制哈希的前 16 个字符作为 Agent ID
	return fmt.Sprintf("agent-%x", hash[:8])
}

// ============================================================================
// Process Monitor gRPC Handlers (Task 11)
// 进程监控 gRPC 处理器
// ============================================================================

// MonitorService provides monitor-related operations for gRPC handlers.
// MonitorService 为 gRPC 处理器提供监控相关操作。
type MonitorService interface {
	GetConfig(ctx context.Context, clusterID uint) (*monitor.MonitorConfig, error)
	RecordEventFromReport(ctx context.Context, clusterID, nodeID, hostID uint, eventType monitor.ProcessEventType, pid int, processName, installDir, role string, details map[string]string) error
}

// ClusterNodeProvider provides cluster node information.
// ClusterNodeProvider 提供集群节点信息。
type ClusterNodeProvider interface {
	GetNodeByHostAndInstallDirAndRole(ctx context.Context, hostID uint, installDir, role string) (clusterID, nodeID uint, found bool, err error)
	// GetNodesByHostID returns all nodes on a specific host with their cluster's monitor config.
	// GetNodesByHostID 返回特定主机上的所有节点及其集群的监控配置。
	GetNodesByHostID(ctx context.Context, hostID uint) ([]*NodeWithMonitorConfig, error)
	// UpdateNodeProcessStatus updates the process PID and status for a node.
	// UpdateNodeProcessStatus 更新节点的进程 PID 和状态。
	UpdateNodeProcessStatus(ctx context.Context, nodeID uint, pid int, status string) error
	// RefreshClusterStatusFromNodes recalculates cluster status from its nodes (e.g. after heartbeat).
	// RefreshClusterStatusFromNodes 根据节点状态重新计算集群状态（如心跳更新节点后）。
	RefreshClusterStatusFromNodes(ctx context.Context, clusterID uint)
	// GetClusterNodeDisplayInfo returns cluster name and node display "主机名 - 角色" for audit resource name.
	// GetClusterNodeDisplayInfo 返回集群名及节点展示「主机名 - 角色」，用于审计资源名称。
	GetClusterNodeDisplayInfo(ctx context.Context, clusterID, nodeID uint) (clusterName, nodeDisplay string)
}

// NodeWithMonitorConfig represents a node with its cluster's monitor config.
// NodeWithMonitorConfig 表示带有集群监控配置的节点。
type NodeWithMonitorConfig struct {
	ClusterID     uint                   `json:"cluster_id"`
	NodeID        uint                   `json:"node_id"`
	InstallDir    string                 `json:"install_dir"`
	Role          string                 `json:"role"`
	ProcessPID    int                    `json:"process_pid"`
	MonitorConfig *monitor.MonitorConfig `json:"monitor_config"`
}

// monitorService is the monitor service for handling process events.
// monitorService 是处理进程事件的监控服务。
var monitorService MonitorService

// clusterNodeProvider provides cluster node information.
// clusterNodeProvider 提供集群节点信息。
var clusterNodeProvider ClusterNodeProvider

// diagnosticsService handles structured diagnostics evidence ingestion.
// diagnosticsService 处理结构化诊断证据入库。
var diagnosticsService *diagnostics.Service

// SetMonitorService sets the monitor service for gRPC handlers.
// SetMonitorService 设置 gRPC 处理器的监控服务。
func SetMonitorService(svc MonitorService) {
	monitorService = svc
}

// SetClusterNodeProvider sets the cluster node provider for gRPC handlers.
// SetClusterNodeProvider 设置 gRPC 处理器的集群节点提供者。
func SetClusterNodeProvider(provider ClusterNodeProvider) {
	clusterNodeProvider = provider
}

// SetDiagnosticsService sets the diagnostics service for gRPC handlers.
// SetDiagnosticsService 设置 gRPC 处理器的诊断服务。
func SetDiagnosticsService(svc *diagnostics.Service) {
	diagnosticsService = svc
}

// HandleDiscoverClusters handles DISCOVER_CLUSTERS command.
// HandleDiscoverClusters 处理 DISCOVER_CLUSTERS 命令。
// Requirements: 1.3, 1.7 - Trigger agent discovery
// Task 11.1
func (s *Server) HandleDiscoverClusters(ctx context.Context, agentID string, params map[string]string) (*pb.CommandResponse, error) {
	s.logger.Info("Handling DISCOVER_CLUSTERS command / 处理 DISCOVER_CLUSTERS 命令",
		zap.String("agent_id", agentID),
	)

	// Get agent connection / 获取 Agent 连接
	conn, ok := s.agentManager.GetAgent(agentID)
	if !ok {
		return nil, agent.ErrAgentNotFound
	}

	// Send DISCOVER_CLUSTERS command to agent / 向 Agent 发送 DISCOVER_CLUSTERS 命令
	cmdResp, err := s.agentManager.SendCommand(ctx, agentID, pb.CommandType_DISCOVER_CLUSTERS, params, 60*time.Second)
	if err != nil {
		s.logger.Error("Failed to send DISCOVER_CLUSTERS command / 发送 DISCOVER_CLUSTERS 命令失败",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
		return nil, err
	}

	s.logger.Info("DISCOVER_CLUSTERS command completed / DISCOVER_CLUSTERS 命令完成",
		zap.String("agent_id", agentID),
		zap.Uint("host_id", conn.HostID),
		zap.String("status", cmdResp.Status.String()),
	)

	return cmdResp, nil
}

// HandleUpdateMonitorConfig handles UPDATE_MONITOR_CONFIG command.
// HandleUpdateMonitorConfig 处理 UPDATE_MONITOR_CONFIG 命令。
// Requirements: 5.4, 7.5 - Push monitor config to agent
// Task 11.2
func (s *Server) HandleUpdateMonitorConfig(ctx context.Context, agentID string, config *monitor.MonitorConfig, processes []*monitor.TrackedProcessInfo) error {
	s.logger.Info("Handling UPDATE_MONITOR_CONFIG command / 处理 UPDATE_MONITOR_CONFIG 命令",
		zap.String("agent_id", agentID),
		zap.Uint("cluster_id", config.ClusterID),
		zap.Int("config_version", config.ConfigVersion),
		zap.Int("process_count", len(processes)),
	)

	// Build parameters / 构建参数
	params := map[string]string{
		"cluster_id":       fmt.Sprintf("%d", config.ClusterID),
		"config_version":   fmt.Sprintf("%d", config.ConfigVersion),
		"auto_monitor":     fmt.Sprintf("%t", config.AutoMonitor),
		"auto_restart":     fmt.Sprintf("%t", config.AutoRestart),
		"monitor_interval": fmt.Sprintf("%d", config.MonitorInterval),
		"restart_delay":    fmt.Sprintf("%d", config.RestartDelay),
		"max_restarts":     fmt.Sprintf("%d", config.MaxRestarts),
		"time_window":      fmt.Sprintf("%d", config.TimeWindow),
		"cooldown_period":  fmt.Sprintf("%d", config.CooldownPeriod),
	}

	// Add tracked processes as JSON / 添加跟踪进程列表（JSON 格式）
	if len(processes) > 0 {
		processesJSON, err := json.Marshal(processes)
		if err != nil {
			s.logger.Error("Failed to marshal processes / 序列化进程列表失败", zap.Error(err))
		} else {
			params["tracked_processes"] = string(processesJSON)
		}
	}

	// Send UPDATE_MONITOR_CONFIG command to agent / 向 Agent 发送 UPDATE_MONITOR_CONFIG 命令
	_, err := s.agentManager.SendCommand(ctx, agentID, pb.CommandType_UPDATE_MONITOR_CONFIG, params, 30*time.Second)
	if err != nil {
		s.logger.Error("Failed to send UPDATE_MONITOR_CONFIG command / 发送 UPDATE_MONITOR_CONFIG 命令失败",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("UPDATE_MONITOR_CONFIG command sent / UPDATE_MONITOR_CONFIG 命令已发送",
		zap.String("agent_id", agentID),
		zap.Uint("cluster_id", config.ClusterID),
		zap.Int("process_count", len(processes)),
	)

	return nil
}

// HandleProcessEventReport handles ProcessEventReport from agent.
// HandleProcessEventReport 处理来自 Agent 的 ProcessEventReport。
// Requirements: 3.4, 3.5 - Receive and process process events
// Task 11.3
func (s *Server) HandleProcessEventReport(ctx context.Context, agentID string, report *pb.ProcessEventReport) error {
	s.logger.Info("[ProcessEvent] Received event report / 收到事件上报",
		zap.String("agent_id", agentID),
		zap.String("event_type", report.EventType.String()),
		zap.Int32("pid", report.Pid),
		zap.String("process_name", report.ProcessName),
		zap.String("install_dir", report.InstallDir),
		zap.String("role", report.Role),
	)

	if monitorService == nil {
		s.logger.Error("[ProcessEvent] monitorService is nil / monitorService 为空")
		return nil
	}
	if clusterNodeProvider == nil {
		s.logger.Error("[ProcessEvent] clusterNodeProvider is nil / clusterNodeProvider 为空")
		return nil
	}

	// Get agent connection to find host ID / 获取 Agent 连接以查找主机 ID
	conn, ok := s.agentManager.GetAgent(agentID)
	if !ok {
		s.logger.Warn("[ProcessEvent] Agent not found / 未找到 Agent",
			zap.String("agent_id", agentID),
		)
		return agent.ErrAgentNotFound
	}

	s.logger.Info("[ProcessEvent] Found agent connection / 找到 Agent 连接",
		zap.String("agent_id", agentID),
		zap.Uint("host_id", conn.HostID),
	)

	// Find cluster and node by host ID, install dir and role / 根据主机 ID、安装目录和角色查找集群和节点
	clusterID, nodeID, found, err := clusterNodeProvider.GetNodeByHostAndInstallDirAndRole(ctx, conn.HostID, report.InstallDir, report.Role)
	if err != nil {
		s.logger.Error("[ProcessEvent] Failed to find cluster node / 查找集群节点失败",
			zap.Uint("host_id", conn.HostID),
			zap.String("install_dir", report.InstallDir),
			zap.String("role", report.Role),
			zap.Error(err),
		)
		return err
	}

	if !found {
		s.logger.Warn("[ProcessEvent] Cluster node not found / 未找到集群节点",
			zap.Uint("host_id", conn.HostID),
			zap.String("install_dir", report.InstallDir),
			zap.String("role", report.Role),
		)
		return nil
	}

	s.logger.Info("[ProcessEvent] Found cluster node / 找到集群节点",
		zap.Uint("cluster_id", clusterID),
		zap.Uint("node_id", nodeID),
	)

	// Map protobuf event type to monitor event type / 将 protobuf 事件类型映射到监控事件类型
	var eventType monitor.ProcessEventType
	switch report.EventType {
	case pb.ProcessEventType_PROCESS_STARTED:
		eventType = monitor.EventTypeStarted
	case pb.ProcessEventType_PROCESS_STOPPED:
		eventType = monitor.EventTypeStopped
	case pb.ProcessEventType_PROCESS_CRASHED:
		eventType = monitor.EventTypeCrashed
	case pb.ProcessEventType_PROCESS_RESTARTED:
		eventType = monitor.EventTypeRestarted
	case pb.ProcessEventType_PROCESS_RESTART_FAILED:
		eventType = monitor.EventTypeRestartFailed
	default:
		s.logger.Warn("Unknown process event type / 未知的进程事件类型",
			zap.String("event_type", report.EventType.String()),
		)
		return nil
	}

	// Record event / 记录事件
	err = monitorService.RecordEventFromReport(
		ctx,
		clusterID,
		nodeID,
		conn.HostID,
		eventType,
		int(report.Pid),
		report.ProcessName,
		report.InstallDir,
		report.Role,
		report.Details,
	)
	if err != nil {
		s.logger.Error("Failed to record process event / 记录进程事件失败",
			zap.Error(err),
		)
		return err
	}

	// Update node process status in database / 更新数据库中节点的进程状态
	var processStatus string
	switch eventType {
	case monitor.EventTypeStarted, monitor.EventTypeRestarted:
		processStatus = "running"
	case monitor.EventTypeStopped:
		processStatus = "stopped"
	case monitor.EventTypeCrashed, monitor.EventTypeRestartFailed:
		processStatus = "crashed"
	}

	s.logger.Info("Updating node process status / 更新节点进程状态",
		zap.Uint("node_id", nodeID),
		zap.Int32("pid", report.Pid),
		zap.String("status", processStatus),
		zap.String("event_type", string(eventType)),
	)

	if processStatus != "" {
		if err := clusterNodeProvider.UpdateNodeProcessStatus(ctx, nodeID, int(report.Pid), processStatus); err != nil {
			s.logger.Error("Failed to update node process status / 更新节点进程状态失败",
				zap.Uint("node_id", nodeID),
				zap.Int32("pid", report.Pid),
				zap.String("status", processStatus),
				zap.Error(err),
			)
		} else {
			s.logger.Info("Node process status updated successfully / 节点进程状态更新成功",
				zap.Uint("node_id", nodeID),
				zap.Int32("pid", report.Pid),
				zap.String("status", processStatus),
			)
		}
	}

	s.logger.Info("Process event recorded / 进程事件已记录",
		zap.Uint("cluster_id", clusterID),
		zap.Uint("node_id", nodeID),
		zap.String("event_type", string(eventType)),
	)

	// 写入审计日志，区分自动（Agent 上报）与手动（UI 操作）；资源名称与手动操作一致：集群名（主机名 - 角色）
	if s.auditRepo != nil {
		action := eventTypeToAuditAction(eventType)
		if action != "" {
			resourceID := strconv.FormatUint(uint64(clusterID), 10) + "/" + strconv.FormatUint(uint64(nodeID), 10)
			resourceName := report.ProcessName
			if clusterNodeProvider != nil {
				clusterName, nodeDisplay := clusterNodeProvider.GetClusterNodeDisplayInfo(ctx, clusterID, nodeID)
				if clusterName != "" {
					if nodeDisplay != "" {
						resourceName = clusterName + "（" + nodeDisplay + "）"
					} else {
						resourceName = clusterName
					}
				}
			}
			details := audit.AuditDetails{
				"event_type":   string(eventType),
				"process_name": report.ProcessName,
				"pid":          report.Pid,
				"agent_id":     agentID,
			}
			auditLog := &audit.AuditLog{
				Username:     "agent",
				Action:       action,
				ResourceType: "cluster_node",
				ResourceID:   resourceID,
				ResourceName: resourceName,
				Trigger:      "auto",
				Details:      details,
				IPAddress:    "",
				UserAgent:    "stx-agent",
			}
			if err := s.auditRepo.CreateAuditLog(ctx, auditLog); err != nil {
				s.logger.Warn("Failed to create audit log for process event / 进程事件写审计失败", zap.Error(err))
			}
		}
	}

	return nil
}

// eventTypeToAuditAction maps monitor event type to audit action string.
func eventTypeToAuditAction(t monitor.ProcessEventType) string {
	switch t {
	case monitor.EventTypeStarted:
		return "start"
	case monitor.EventTypeStopped:
		return "stop"
	case monitor.EventTypeRestarted:
		return "restart"
	case monitor.EventTypeCrashed:
		return "crashed"
	case monitor.EventTypeRestartFailed:
		return "restart_failed"
	default:
		return ""
	}
}

// parseProcessEventFromResponse parses process event from command response.
// parseProcessEventFromResponse 从命令响应中解析进程事件。
func parseProcessEventFromResponse(output string) (*pb.ProcessEventReport, error) {
	// First try to unmarshal as a map to handle both numeric and string enum values
	// 首先尝试解析为 map 以处理数字和字符串枚举值
	var rawData map[string]interface{}
	if err := json.Unmarshal([]byte(output), &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	report := &pb.ProcessEventReport{}

	// Parse agent_id / 解析 agent_id
	if v, ok := rawData["agent_id"].(string); ok {
		report.AgentId = v
	}

	// Parse event_type (can be number or string) / 解析 event_type（可以是数字或字符串）
	if v, ok := rawData["event_type"].(float64); ok {
		report.EventType = pb.ProcessEventType(int32(v))
	} else if v, ok := rawData["event_type"].(string); ok {
		// Handle string enum value / 处理字符串枚举值
		if enumVal, ok := pb.ProcessEventType_value[v]; ok {
			report.EventType = pb.ProcessEventType(enumVal)
		}
	}

	// Parse pid / 解析 pid
	if v, ok := rawData["pid"].(float64); ok {
		report.Pid = int32(v)
	}

	// Parse process_name / 解析 process_name
	if v, ok := rawData["process_name"].(string); ok {
		report.ProcessName = v
	}

	// Parse install_dir / 解析 install_dir
	if v, ok := rawData["install_dir"].(string); ok {
		report.InstallDir = v
	}

	// Parse role / 解析 role
	if v, ok := rawData["role"].(string); ok {
		report.Role = v
	}

	// Parse timestamp / 解析 timestamp
	if v, ok := rawData["timestamp"].(float64); ok {
		report.Timestamp = int64(v)
	}

	// Parse details / 解析 details
	if v, ok := rawData["details"].(map[string]interface{}); ok {
		report.Details = make(map[string]string)
		for k, val := range v {
			if s, ok := val.(string); ok {
				report.Details[k] = s
			}
		}
	}

	return report, nil
}

// pushMonitorConfigToAgent pushes monitor config to agent after registration.
// pushMonitorConfigToAgent 在注册后向 Agent 推送监控配置。
// This ensures agent has the correct process tracking info after restart.
// 这确保 Agent 重启后有正确的进程跟踪信息。
func (s *Server) pushMonitorConfigToAgent(ctx context.Context, agentID string, hostID uint) {
	// Wait for agent command stream to be ready / 等待 Agent 命令流就绪
	// Agent needs time to establish the bidirectional stream after registration
	// Agent 注册后需要时间建立双向流
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		time.Sleep(3 * time.Second)

		// Check if agent has command stream available / 检查 Agent 是否有可用的命令流
		conn, ok := s.agentManager.GetAgent(agentID)
		if ok && conn.Stream != nil {
			break
		}

		if i == maxRetries-1 {
			s.logger.Warn("Agent command stream not ready after retries, skipping config push / Agent 命令流重试后仍未就绪，跳过配置推送",
				zap.String("agent_id", agentID),
				zap.Int("retries", maxRetries),
			)
			return
		}
	}

	if clusterNodeProvider == nil {
		s.logger.Debug("Cluster node provider not configured, skipping monitor config push / 集群节点提供者未配置，跳过监控配置推送")
		return
	}

	// Get all nodes on this host / 获取此主机上的所有节点
	nodes, err := clusterNodeProvider.GetNodesByHostID(ctx, hostID)
	if err != nil {
		s.logger.Warn("Failed to get nodes for host / 获取主机节点失败",
			zap.Uint("host_id", hostID),
			zap.Error(err),
		)
		return
	}

	if len(nodes) == 0 {
		s.logger.Debug("No nodes found on host, skipping monitor config push / 主机上没有节点，跳过监控配置推送",
			zap.Uint("host_id", hostID),
		)
		return
	}

	// Group nodes by cluster and build tracked processes / 按集群分组节点并构建跟踪进程列表
	// For simplicity, we send all processes in one command with the first cluster's config
	// 为简单起见，我们使用第一个集群的配置发送所有进程
	// Include all nodes (even stopped ones) to sync manually_stopped state
	// 包含所有节点（即使已停止）以同步 manually_stopped 状态
	var trackedProcesses []map[string]interface{}
	var firstConfig *monitor.MonitorConfig

	for _, node := range nodes {
		// Include all nodes with install_dir, not just running ones
		// 包含所有有安装目录的节点，不仅仅是运行中的
		if node.InstallDir != "" {
			trackedProcesses = append(trackedProcesses, map[string]interface{}{
				"pid":         node.ProcessPID, // Can be 0 for stopped processes / 已停止的进程可以为 0
				"name":        processidentity.ManagedName(node.InstallDir, node.Role),
				"cluster_id":  node.ClusterID,
				"install_dir": node.InstallDir,
				"role":        node.Role,
			})
		}
		if firstConfig == nil && node.MonitorConfig != nil {
			firstConfig = node.MonitorConfig
		}
	}

	if len(trackedProcesses) == 0 {
		s.logger.Debug("No nodes to track on host / 主机上没有需要跟踪的节点",
			zap.Uint("host_id", hostID),
		)
		return
	}

	// Use default config if none found / 如果没有找到配置则使用默认配置
	if firstConfig == nil {
		firstConfig = monitor.DefaultMonitorConfig(0)
	}

	// Build parameters / 构建参数
	params := map[string]string{
		"cluster_id":       "0", // Multiple clusters, use 0 / 多个集群，使用 0
		"config_version":   fmt.Sprintf("%d", firstConfig.ConfigVersion),
		"auto_monitor":     fmt.Sprintf("%t", firstConfig.AutoMonitor),
		"auto_restart":     fmt.Sprintf("%t", firstConfig.AutoRestart),
		"monitor_interval": fmt.Sprintf("%d", firstConfig.MonitorInterval),
		"restart_delay":    fmt.Sprintf("%d", firstConfig.RestartDelay),
		"max_restarts":     fmt.Sprintf("%d", firstConfig.MaxRestarts),
		"time_window":      fmt.Sprintf("%d", firstConfig.TimeWindow),
		"cooldown_period":  fmt.Sprintf("%d", firstConfig.CooldownPeriod),
	}

	// Add tracked processes as JSON / 添加跟踪进程列表（JSON 格式）
	processesJSON, err := json.Marshal(trackedProcesses)
	if err != nil {
		s.logger.Error("Failed to marshal tracked processes / 序列化跟踪进程列表失败", zap.Error(err))
		return
	}
	params["tracked_processes"] = string(processesJSON)

	// Send UPDATE_MONITOR_CONFIG command / 发送 UPDATE_MONITOR_CONFIG 命令
	_, err = s.agentManager.SendCommand(ctx, agentID, pb.CommandType_UPDATE_MONITOR_CONFIG, params, 30*time.Second)
	if err != nil {
		s.logger.Warn("Failed to push monitor config to agent / 向 Agent 推送监控配置失败",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("Monitor config pushed to agent after registration / 注册后已向 Agent 推送监控配置",
		zap.String("agent_id", agentID),
		zap.Uint("host_id", hostID),
		zap.Int("process_count", len(trackedProcesses)),
	)
}

// updateProcessStatusFromHeartbeat updates cluster_nodes process status from heartbeat data.
// It unconditionally overwrites DB (process_pid and status) with agent-reported values; no check
// against previous state (e.g. no "do not overwrite if user just stopped"). This is the periodic
// correction so DB matches the agent's view of actual host state.
// updateProcessStatusFromHeartbeat 从心跳数据更新 cluster_nodes，会强制覆盖 DB 中的 process_pid 与 status，不做与旧状态对比；用于周期性纠正使 DB 与主机实际状态一致。
func (s *Server) updateProcessStatusFromHeartbeat(ctx context.Context, hostID uint, processes []*pb.ProcessStatus) {
	if clusterNodeProvider == nil {
		return
	}

	// Get all nodes on this host / 获取此主机上的所有节点
	nodes, err := clusterNodeProvider.GetNodesByHostID(ctx, hostID)
	if err != nil {
		s.logger.Warn("Failed to get nodes for heartbeat update / 获取节点用于心跳更新失败",
			zap.Uint("host_id", hostID),
			zap.Error(err),
		)
		return
	}

	// Create a map of process name to status / 创建进程名到状态的映射
	processMap := make(map[string]*pb.ProcessStatus)
	for _, proc := range processes {
		processMap[proc.Name] = proc
	}

	// Update each node's process status / 更新每个节点的进程状态
	clusterIDsSeen := make(map[uint]struct{})
	for _, node := range nodes {
		processName := processidentity.ManagedName(node.InstallDir, node.Role)

		// Find matching process / 查找匹配的进程
		proc, found := processMap[processName]
		if !found {
			continue
		}

		// Update node process status / 更新节点进程状态
		if err := clusterNodeProvider.UpdateNodeProcessStatus(ctx, node.NodeID, int(proc.Pid), proc.Status); err != nil {
			s.logger.Warn("Failed to update node process status from heartbeat / 从心跳更新节点进程状态失败",
				zap.Uint("node_id", node.NodeID),
				zap.Error(err),
			)
		}
		clusterIDsSeen[node.ClusterID] = struct{}{}
	}

	// Refresh cluster status from nodes so cluster.status stays in sync (not only on read).
	// 根据节点状态刷新集群 status，使集群状态随心跳持续更新。
	for cid := range clusterIDsSeen {
		clusterNodeProvider.RefreshClusterStatusFromNodes(ctx, cid)
	}
}
