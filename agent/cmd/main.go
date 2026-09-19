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

// Package main is the entry point for the STX Agent service.
// main 包是 STX Agent 服务的入口点。
//
// Agent is a daemon process deployed on physical/VM nodes that:
// Agent 是部署在物理机/VM 节点上的守护进程，负责：
// - Communicates with Control Plane via gRPC / 通过 gRPC 与 Control Plane 通信
// - Executes remote operations (install, start, stop, etc.) / 执行远程运维操作（安装、启动、停止等）
// - Reports heartbeat and resource usage / 上报心跳和资源使用情况
// - Manages SeaTunnel processes / 管理 SeaTunnel 进程
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/collector"
	"github.com/LeonYoah/stx/agent/internal/config"
	agentdiagnostics "github.com/LeonYoah/stx/agent/internal/diagnostics"
	"github.com/LeonYoah/stx/agent/internal/discovery"
	"github.com/LeonYoah/stx/agent/internal/executor"
	agentgrpc "github.com/LeonYoah/stx/agent/internal/grpc"
	"github.com/LeonYoah/stx/agent/internal/installer"
	"github.com/LeonYoah/stx/agent/internal/logger"
	"github.com/LeonYoah/stx/agent/internal/monitor"
	"github.com/LeonYoah/stx/agent/internal/process"
	"github.com/LeonYoah/stx/agent/internal/restart"
	"github.com/LeonYoah/stx/internal/seatunnel"
	"github.com/spf13/cobra"
)

// Version information, set at build time
// 版本信息，在构建时设置
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

var agentLogTimestampPattern = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:,\d{3})?)\b`)

// Agent represents the main agent service that integrates all components
// Agent 表示集成所有组件的主要 Agent 服务
// Requirements 1.1: Agent service startup - load config, init gRPC client, register with Control Plane
// 需求 1.1：Agent 服务启动 - 加载配置、初始化 gRPC 客户端、向 Control Plane 注册
type Agent struct {
	// config holds the agent configuration
	// config 保存 Agent 配置
	config *config.Config

	// ctx is the main context for the agent
	// ctx 是 Agent 的主上下文
	ctx context.Context

	// cancel cancels the main context
	// cancel 取消主上下文
	cancel context.CancelFunc

	// grpcClient is the gRPC client for Control Plane communication
	// grpcClient 是与 Control Plane 通信的 gRPC 客户端
	grpcClient *agentgrpc.Client

	// executor handles command execution and routing
	// executor 处理命令执行和路由
	executor *executor.CommandExecutor

	// processManager manages SeaTunnel process lifecycle
	// processManager 管理 SeaTunnel 进程生命周期
	processManager *process.ProcessManager

	// metricsCollector collects system and process metrics
	// metricsCollector 采集系统和进程指标
	metricsCollector *collector.MetricsCollector

	// installerManager handles SeaTunnel installation
	// installerManager 处理 SeaTunnel 安装
	installerManager *installer.InstallerManager

	// processMonitor monitors SeaTunnel process status
	// processMonitor 监控 SeaTunnel 进程状态
	processMonitor *monitor.ProcessMonitor

	// autoRestarter handles automatic process restart
	// autoRestarter 处理自动进程重启
	autoRestarter *restart.AutoRestarter

	// eventReporter handles process event reporting
	// eventReporter 处理进程事件上报
	eventReporter *monitor.EventReporter

	// errorCollector handles incremental Seatunnel ERROR log collection.
	// errorCollector 处理 Seatunnel ERROR 日志增量采集。
	errorCollector *agentdiagnostics.Collector

	// wg tracks running goroutines for graceful shutdown
	// wg 跟踪运行中的 goroutine 以实现优雅关闭
	wg sync.WaitGroup

	// running indicates if the agent is running
	// running 表示 Agent 是否正在运行
	running bool

	// mu protects the running state
	// mu 保护运行状态
	mu sync.RWMutex
}

// NewAgent creates a new Agent instance with all components initialized
// NewAgent 创建一个初始化所有组件的新 Agent 实例
func NewAgent(cfg *config.Config) *Agent {
	ctx, cancel := context.WithCancel(context.Background())

	// Create process manager / 创建进程管理器
	pm := process.NewProcessManager()

	// Create metrics collector with process manager / 使用进程管理器创建指标采集器
	mc := collector.NewMetricsCollector(pm)

	// Create command executor / 创建命令执行器
	exec := executor.NewCommandExecutor()

	// Create gRPC client / 创建 gRPC 客户端
	grpcClient := agentgrpc.NewClient(cfg)

	// Create installer manager / 创建安装管理器
	im := installer.NewInstallerManager()

	// Create process monitor / 创建进程监控器
	pmon := monitor.NewProcessMonitor()

	// Create auto restarter / 创建自动重启器
	ar := restart.NewAutoRestarter(pm)

	// Create event reporter / 创建事件上报器
	er := monitor.NewEventReporter(nil) // Will set report func later / 稍后设置上报函数

	// Create diagnostics error collector / 创建诊断错误采集器
	ec := agentdiagnostics.NewCollector(grpcClient)

	return &Agent{
		config:           cfg,
		ctx:              ctx,
		cancel:           cancel,
		grpcClient:       grpcClient,
		executor:         exec,
		processManager:   pm,
		metricsCollector: mc,
		installerManager: im,
		processMonitor:   pmon,
		autoRestarter:    ar,
		eventReporter:    er,
		errorCollector:   ec,
	}
}

// Run starts the Agent service and all its components
// Run 启动 Agent 服务及其所有组件
// Requirements 1.1: Agent startup - load config, init gRPC client, register with Control Plane
// Requirements 1.2: After successful registration, establish bidirectional gRPC stream
// Requirements 1.3: Send heartbeat every 10 seconds with resource usage
// 需求 1.1：Agent 启动 - 加载配置、初始化 gRPC 客户端、向 Control Plane 注册
// 需求 1.2：注册成功后，建立双向 gRPC 流连接
// 需求 1.3：每 10 秒发送心跳，包含资源使用率
func (a *Agent) Run() error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("agent is already running / Agent 已在运行")
	}
	a.running = true
	a.mu.Unlock()

	ctx := a.ctx
	logger.InfoF(ctx, "========================================")
	logger.InfoF(ctx, "  STX Agent Starting...")
	logger.InfoF(ctx, "  STX Agent 正在启动...")
	logger.InfoF(ctx, "========================================")
	logger.InfoF(ctx, "Version: %s, Commit: %s, Build: %s", Version, GitCommit, BuildTime)
	logger.InfoF(ctx, "Control Plane: %v", a.config.ControlPlane.Addresses)
	logger.InfoF(ctx, "Heartbeat Interval: %v", a.config.Heartbeat.Interval)
	logger.InfoF(ctx, "Log Level: %s", a.config.Log.Level)

	// Step 1: Start process manager for monitoring
	// 步骤 1：启动进程管理器进行监控
	logger.InfoF(ctx, "[1/8] Starting process manager... / 启动进程管理器...")
	if err := a.processManager.Start(a.ctx); err != nil {
		return fmt.Errorf("failed to start process manager: %w / 启动进程管理器失败：%w", err, err)
	}

	// Set up process event handler / 设置进程事件处理器
	a.processManager.SetEventHandler(a.handleProcessEvent)

	// Step 2: Start process monitor / 启动进程监控器
	logger.InfoF(ctx, "[2/8] Starting process monitor... / 启动进程监控器...")
	a.setupProcessMonitor()
	if err := a.processMonitor.Start(a.ctx); err != nil {
		logger.WarnF(ctx, "Warning: failed to start process monitor: %v / 警告：启动进程监控器失败：%v", err, err)
	}

	// Step 3: Initialize process discovery (simplified, no auto-scan)
	// 步骤 3：初始化进程发现（简化版，无自动扫描）
	logger.InfoF(ctx, "[3/8] Initializing process discovery... / 初始化进程发现...")

	// Step 4: Start event reporter / 启动事件上报器
	logger.InfoF(ctx, "[4/8] Starting event reporter... / 启动事件上报器...")
	a.eventReporter.Start()

	// Step 5: Register command handlers
	// 步骤 5：注册命令处理器
	logger.InfoF(ctx, "[5/8] Registering command handlers... / 注册命令处理器...")
	a.registerCommandHandlers()

	// Step 6: Connect to Control Plane
	// 步骤 6：连接到 Control Plane
	logger.InfoF(ctx, "[6/8] Connecting to Control Plane... / 连接到 Control Plane...")
	if err := a.connectToControlPlane(); err != nil {
		return fmt.Errorf("failed to connect to Control Plane: %w / 连接 Control Plane 失败：%w", err, err)
	}

	// Step 7: Register with Control Plane
	// 步骤 7：向 Control Plane 注册
	logger.InfoF(ctx, "[7/8] Registering with Control Plane... / 向 Control Plane 注册...")
	if err := a.registerWithControlPlane(); err != nil {
		return fmt.Errorf("failed to register with Control Plane: %w / 向 Control Plane 注册失败：%w", err, err)
	}

	// Step 8: Start background services
	// 步骤 8：启动后台服务
	logger.InfoF(ctx, "[8/8] Starting background services... / 启动后台服务...")
	a.startBackgroundServices()

	logger.InfoF(ctx, "========================================")
	logger.InfoF(ctx, "  Agent started successfully!")
	logger.InfoF(ctx, "  Agent 启动成功！")
	logger.InfoF(ctx, "========================================")

	// Wait for context cancellation (shutdown signal)
	// 等待上下文取消（关闭信号）
	<-a.ctx.Done()

	return nil
}

// setupProcessMonitor sets up the process monitor with callbacks
// setupProcessMonitor 设置进程监控器的回调
func (a *Agent) setupProcessMonitor() {
	// Set event handler / 设置事件处理器
	a.processMonitor.SetEventHandler(func(event *monitor.ProcessEvent) {
		ctx := context.Background()
		logger.InfoF(ctx, "[Agent] Process event: type=%s, name=%s, pid=%d / 进程事件：类型=%s，名称=%s，PID=%d",
			event.Type, event.Name, event.PID, event.Type, event.Name, event.PID)

		// Report event to Control Plane via gRPC / 通过 gRPC 向 Control Plane 上报事件
		go a.reportProcessEvent(event)
	})

	// Set crash handler to trigger auto restart / 设置崩溃处理器以触发自动重启
	a.processMonitor.SetCrashHandler(func(proc *monitor.TrackedProcess) {
		ctx := context.Background()
		logger.ErrorF(ctx, "[Agent] Process crashed: %s (PID: %d), triggering auto restart / 进程崩溃：%s（PID：%d），触发自动重启",
			proc.Name, proc.PID, proc.Name, proc.PID)
		if err := a.autoRestarter.OnProcessCrashed(proc); err != nil {
			logger.ErrorF(ctx, "[Agent] Auto restart failed: %v / 自动重启失败：%v", err, err)
		}
	})

	// Set restart callback to update PID and report event / 设置重启回调以更新 PID 并上报事件
	a.autoRestarter.SetCallback(func(processName string, success bool, err error) {
		ctx := context.Background()
		if !success {
			// Report restart failed event / 上报重启失败事件
			proc := a.processMonitor.GetTrackedProcess(processName)
			if proc != nil {
				event := &monitor.ProcessEvent{
					Type:      monitor.EventRestartFailed,
					PID:       proc.PID,
					Name:      processName,
					Timestamp: time.Now(),
					Details: map[string]interface{}{
						"install_dir": proc.InstallDir,
						"role":        proc.Role,
						"error":       err.Error(),
					},
				}
				go a.reportProcessEvent(event)
			}
			return
		}

		// Get new PID from process manager / 从进程管理器获取新 PID
		info, err := a.processManager.GetStatus(a.ctx, processName)
		if err != nil || info.PID <= 0 {
			logger.ErrorF(ctx, "[Agent] Failed to get new PID after restart: %v / 重启后获取新 PID 失败：%v", err, err)
			return
		}

		// Update process monitor with new PID / 使用新 PID 更新进程监控器
		a.processMonitor.UpdateProcessPID(processName, info.PID)

		// Get tracked process for details / 获取跟踪进程的详细信息
		proc := a.processMonitor.GetTrackedProcess(processName)
		installDir := ""
		role := ""
		if proc != nil {
			installDir = proc.InstallDir
			role = proc.Role
		}

		// Report restarted event with new PID / 上报带有新 PID 的重启事件
		event := &monitor.ProcessEvent{
			Type:      monitor.EventRestarted,
			PID:       info.PID,
			Name:      processName,
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"install_dir": installDir,
				"role":        role,
			},
		}
		go a.reportProcessEvent(event)

		logger.InfoF(ctx, "[Agent] Process restarted: %s (new PID: %d) / 进程已重启：%s（新 PID：%d）",
			processName, info.PID, processName, info.PID)
	})
}

// reportProcessEvent reports a process event to Control Plane via gRPC.
// reportProcessEvent 通过 gRPC 向 Control Plane 上报进程事件。
func (a *Agent) reportProcessEvent(event *monitor.ProcessEvent) {
	ctx := a.ctx
	if !a.grpcClient.IsConnected() {
		logger.WarnF(ctx, "[Agent] Not connected, caching event / 未连接，缓存事件")
		a.eventReporter.ReportEvent(event)
		return
	}

	// Convert monitor.ProcessEvent to pb.ProcessEventReport
	// 将 monitor.ProcessEvent 转换为 pb.ProcessEventReport
	var eventType pb.ProcessEventType
	switch event.Type {
	case monitor.EventStarted:
		eventType = pb.ProcessEventType_PROCESS_STARTED
	case monitor.EventStopped:
		eventType = pb.ProcessEventType_PROCESS_STOPPED
	case monitor.EventCrashed:
		eventType = pb.ProcessEventType_PROCESS_CRASHED
	case monitor.EventRestarted:
		eventType = pb.ProcessEventType_PROCESS_RESTARTED
	case monitor.EventRestartFailed:
		eventType = pb.ProcessEventType_PROCESS_RESTART_FAILED
	}

	installDir, role, details := extractProcessEventReportFields(event)

	report := &pb.ProcessEventReport{
		AgentId:     a.grpcClient.GetAgentID(),
		EventType:   eventType,
		Pid:         int32(event.PID),
		ProcessName: event.Name,
		InstallDir:  installDir,
		Role:        role,
		Timestamp:   event.Timestamp.UnixMilli(),
		Details:     details,
	}

	if err := a.grpcClient.ReportProcessEvent(a.ctx, report); err != nil {
		logger.ErrorF(ctx, "[Agent] Failed to report event, caching: %v / 上报事件失败，缓存：%v", err, err)
		a.eventReporter.ReportEvent(event)
		return
	}

	logger.InfoF(ctx, "[Agent] Event reported to Control Plane: type=%s, name=%s, pid=%d / 事件已上报到 Control Plane：类型=%s，名称=%s，PID=%d",
		event.Type, event.Name, event.PID, event.Type, event.Name, event.PID)
}

func extractProcessEventReportFields(event *monitor.ProcessEvent) (installDir, role string, details map[string]string) {
	details = make(map[string]string)
	if event == nil || event.Details == nil {
		return "", "", details
	}
	for key, value := range event.Details {
		details[key] = fmt.Sprint(value)
	}
	if dir, ok := event.Details["install_dir"].(string); ok {
		installDir = dir
	}
	if r, ok := event.Details["role"].(string); ok {
		role = r
	}
	return installDir, role, details
}

// connectToControlPlane establishes connection to Control Plane with retry
// connectToControlPlane 建立与 Control Plane 的连接（带重试）
func (a *Agent) connectToControlPlane() error {
	ctx := a.ctx
	// Create a context with timeout for initial connection
	// 为初始连接创建带超时的上下文
	connectCtx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	if err := a.grpcClient.Connect(connectCtx); err != nil {
		// If initial connection fails, start reconnection in background
		// 如果初始连接失败，在后台启动重连
		logger.WarnF(ctx, "Initial connection failed, will retry in background: %v", err)
		logger.WarnF(ctx, "初始连接失败，将在后台重试：%v", err)

		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.grpcClient.Reconnect(a.ctx); err != nil {
				logger.ErrorF(ctx, "Reconnection failed: %v / 重连失败：%v", err, err)
			}
		}()
		return nil // Don't fail startup, let reconnection handle it
	}

	logger.InfoF(ctx, "Connected to Control Plane / 已连接到 Control Plane")
	return nil
}

// registerWithControlPlane sends registration request to Control Plane
// registerWithControlPlane 向 Control Plane 发送注册请求
func (a *Agent) registerWithControlPlane() error {
	ctx := a.ctx
	if !a.grpcClient.IsConnected() {
		logger.WarnF(ctx, "Not connected yet, registration will happen after connection / 尚未连接，将在连接后注册")
		return nil
	}

	// Collect system info for registration / 收集系统信息用于注册
	sysInfo := a.metricsCollector.GetSystemInfo()
	hostname := a.metricsCollector.GetHostname()
	ipAddress := a.resolveAgentIP()
	localIPs := a.collectLocalIPAddresses(ipAddress)

	req := &pb.RegisterRequest{
		AgentId:      a.config.Agent.ID,
		Hostname:     hostname,
		IpAddress:    ipAddress,
		OsType:       runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: Version,
		SystemInfo:   sysInfo,
		HostId:       a.config.Agent.HostID,
		IpAddresses:  localIPs,
	}

	resp, err := a.grpcClient.Register(a.ctx, req)
	if err != nil {
		return fmt.Errorf("registration failed: %w / 注册失败：%w", err, err)
	}

	if !resp.Success {
		return fmt.Errorf("registration rejected: %s / 注册被拒绝：%s", resp.Message, resp.Message)
	}

	// Save the assigned agent ID / 保存分配的 Agent ID
	a.grpcClient.SetAgentID(resp.AssignedId)

	logger.InfoF(ctx, "Registered successfully with ID: %s / 注册成功，ID：%s", resp.AssignedId, resp.AssignedId)

	// Align diagnostics collector cursor with server-side persisted cursor.
	// 对齐诊断采集器游标，避免 Agent 重启后按 initialTail 回采导致重复上报。
	if a.errorCollector != nil {
		if cursors, err := a.grpcClient.GetDiagnosticsLogCursors(ctx); err != nil {
			logger.WarnF(ctx, "[Diagnostics] Failed to load diagnostics cursors: %v / 拉取诊断游标失败：%v", err, err)
		} else if cursors != nil {
			loaded := 0
			for _, item := range cursors.Cursors {
				if item == nil {
					continue
				}
				key := strings.TrimSpace(item.InstallDir) + "::" + strings.TrimSpace(item.Role) + "::" + strings.TrimSpace(item.SourceFile)
				if strings.TrimSpace(item.SourceFile) == "" || strings.TrimSpace(item.InstallDir) == "" || strings.TrimSpace(item.Role) == "" {
					continue
				}
				a.errorCollector.SetInitialCursor(key, item.Offset)
				loaded++
			}
			if loaded > 0 {
				logger.InfoF(ctx, "[Diagnostics] Loaded %d diagnostics cursors / 已加载 %d 条诊断游标", loaded, loaded)
			}
		}
	}

	// Set up event reporter with gRPC report function / 设置事件上报器的 gRPC 上报函数
	a.setupEventReporter()

	// Apply configuration from Control Plane if provided
	// 如果提供，应用来自 Control Plane 的配置
	if resp.Config != nil {
		a.applyRemoteConfig(resp.Config)
	}

	return nil
}

// resolveAgentIP 确定 Agent 用于注册的最佳 IP 地址。
// 优先级：1. 手动配置的 IP；2. 通向 Control Plane 的内核出口路由 IP；3. 网卡 IP 兜底。
// resolveAgentIP determines the best IP address for the Agent to register with.
// Priority: 1. Manually configured IP; 2. Kernel outbound route to Control Plane; 3. Network interface IP fallback.
func (a *Agent) resolveAgentIP() string {
	// 1. 配置文件中手动指定的 IP
	// 1. Manually specified IP in config
	if configuredIP := strings.TrimSpace(a.config.Agent.IP); configuredIP != "" {
		logger.InfoF(a.ctx, "Using manually configured Agent IP: %s / 使用手动配置的 Agent IP：%s", configuredIP, configuredIP)
		return configuredIP
	}

	// 2. 查询通往 Control Plane 的内核路由表出口 IP
	// 2. Kernel routing table lookup towards Control Plane
	for _, addr := range a.config.ControlPlane.Addresses {
		if outbound := a.metricsCollector.GetOutboundIP(addr); outbound != "" {
			// 如果出口 IP 不是回环地址，说明是真正的物理/局域网接口
			// If outbound IP is not loopback, it is the real network interface
			if outbound != "127.0.0.1" && outbound != "::1" {
				logger.InfoF(a.ctx, "Detected outbound IP towards Control Plane (%s): %s / 探测到通往 Control Plane (%s) 的出口 IP：%s", addr, outbound, addr, outbound)
				return outbound
			}
		}
	}

	// 3. 兜底回退到枚举物理网卡 IP (hostname -I)
	// 3. Fallback to network interfaces enumeration (hostname -I)
	fallbackIP := a.metricsCollector.GetIPAddress()
	if fallbackIP != "" {
		return fallbackIP
	}

	// 4. 若未找到其他有效 IP 则退回回环地址
	// 4. Default to loopback if nothing else found
	return "127.0.0.1"
}

// collectLocalIPAddresses 汇总本机全部网卡 IP，并确保主注册 IP 也在列表中。
// collectLocalIPAddresses aggregates all local interface IPs and ensures the primary register IP is included.
func (a *Agent) collectLocalIPAddresses(primaryIP string) []string {
	ips := a.metricsCollector.ListLocalIPAddresses()
	primaryIP = strings.TrimSpace(primaryIP)
	if primaryIP == "" || primaryIP == "127.0.0.1" || primaryIP == "::1" || primaryIP == "localhost" {
		return ips
	}
	for _, ip := range ips {
		if ip == primaryIP {
			return ips
		}
	}
	return append(ips, primaryIP)
}

// setupEventReporter sets up the event reporter with gRPC report function.
// setupEventReporter 设置事件上报器的 gRPC 上报函数。
func (a *Agent) setupEventReporter() {
	ctx := a.ctx
	agentID := a.grpcClient.GetAgentID()

	// Create a report function that sends events via gRPC
	// 创建一个通过 gRPC 发送事件的上报函数
	reportFunc := func(events []*monitor.ProcessEvent) error {
		for _, event := range events {
			// Convert monitor.ProcessEvent to pb.ProcessEventReport
			// 将 monitor.ProcessEvent 转换为 pb.ProcessEventReport
			var eventType pb.ProcessEventType
			switch event.Type {
			case monitor.EventStarted:
				eventType = pb.ProcessEventType_PROCESS_STARTED
			case monitor.EventStopped:
				eventType = pb.ProcessEventType_PROCESS_STOPPED
			case monitor.EventCrashed:
				eventType = pb.ProcessEventType_PROCESS_CRASHED
			case monitor.EventRestarted:
				eventType = pb.ProcessEventType_PROCESS_RESTARTED
			case monitor.EventRestartFailed:
				eventType = pb.ProcessEventType_PROCESS_RESTART_FAILED
			}

			installDir, role, details := extractProcessEventReportFields(event)

			report := &pb.ProcessEventReport{
				AgentId:     agentID,
				EventType:   eventType,
				Pid:         int32(event.PID),
				ProcessName: event.Name,
				InstallDir:  installDir,
				Role:        role,
				Timestamp:   event.Timestamp.UnixMilli(),
				Details:     details,
			}

			if err := a.grpcClient.ReportProcessEvent(a.ctx, report); err != nil {
				logger.ErrorF(ctx, "[Agent] Failed to report event: %v / 上报事件失败：%v", err, err)
				return err
			}
			logger.InfoF(ctx, "[Agent] Event reported: type=%s, name=%s, pid=%d / 事件已上报：类型=%s，名称=%s，PID=%d",
				event.Type, event.Name, event.PID, event.Type, event.Name, event.PID)
		}
		return nil
	}

	a.eventReporter.SetReportFunc(reportFunc)
	logger.InfoF(ctx, "[Agent] Event reporter configured / 事件上报器已配置")
}

// applyRemoteConfig applies configuration received from Control Plane
// applyRemoteConfig 应用从 Control Plane 接收的配置
func (a *Agent) applyRemoteConfig(cfg *pb.AgentConfig) {
	ctx := a.ctx
	logger.InfoF(ctx, "Received remote config from Control Plane: HeartbeatInterval=%d seconds / 收到来自 Control Plane 的远程配置：HeartbeatInterval=%d 秒", cfg.HeartbeatInterval, cfg.HeartbeatInterval)
	logger.InfoF(ctx, "Current local heartbeat interval: %v / 当前本地心跳间隔：%v", a.config.Heartbeat.Interval, a.config.Heartbeat.Interval)

	configChanged := false

	if cfg.HeartbeatInterval > 0 {
		newInterval := time.Duration(cfg.HeartbeatInterval) * time.Second
		if a.config.Heartbeat.Interval != newInterval {
			a.config.Heartbeat.Interval = newInterval
			configChanged = true
			logger.InfoF(ctx, "Applied heartbeat interval from Control Plane: %v / 已应用来自 Control Plane 的心跳间隔：%v", newInterval, newInterval)
		}
	} else {
		logger.WarnF(ctx, "Remote HeartbeatInterval is 0 or negative, keeping local config / 远程 HeartbeatInterval 为 0 或负数，保持本地配置")
	}

	// Persist config changes to local file / 将配置变更持久化到本地文件
	if configChanged {
		if err := a.persistConfigToFile(); err != nil {
			logger.WarnF(ctx, "Warning: Failed to persist config to file: %v / 警告：持久化配置到文件失败：%v", err, err)
		} else {
			logger.InfoF(ctx, "Config persisted to local file / 配置已持久化到本地文件")
		}
	}
}

// persistConfigToFile saves the current config to the local config file
// persistConfigToFile 将当前配置保存到本地配置文件
func (a *Agent) persistConfigToFile() error {
	ctx := a.ctx
	configPath := config.DefaultConfigPath

	// Read existing config file content
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Update heartbeat interval in the config content
	// Format: "interval: 10s" -> "interval: 60s"
	lines := strings.Split(string(content), "\n")
	inHeartbeatSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Detect heartbeat section
		if strings.Contains(strings.ToLower(trimmed), "heartbeat") {
			inHeartbeatSection = true
			continue
		}
		// Detect next section (non-indented line that's not empty or comment)
		if inHeartbeatSection && len(trimmed) > 0 && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inHeartbeatSection = false
		}
		// Update interval in heartbeat section
		if inHeartbeatSection && strings.HasPrefix(trimmed, "interval:") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = fmt.Sprintf("%sinterval: %s", indent, a.config.Heartbeat.Interval.String())
			logger.InfoF(ctx, "Updated config line: %s", lines[i])
			break
		}
	}

	// Write back to file
	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// startBackgroundServices starts all background goroutines
// startBackgroundServices 启动所有后台 goroutine
func (a *Agent) startBackgroundServices() {
	// Start heartbeat service / 启动心跳服务
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runHeartbeatLoop()
	}()

	// Start command stream listener / 启动命令流监听器
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runCommandStreamLoop()
	}()

	// Start connection monitor / 启动连接监控
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runConnectionMonitor()
	}()

	// Start diagnostics error collector / 启动诊断错误采集器
	if a.errorCollector != nil {
		a.errorCollector.Start(a.ctx)
	}
}

// runHeartbeatLoop runs the heartbeat sending loop
// runHeartbeatLoop 运行心跳发送循环
// Requirements 1.3: Send heartbeat every 10 seconds with resource usage
// 需求 1.3：每 10 秒发送心跳，包含资源使用率
func (a *Agent) runHeartbeatLoop() {
	ctx := a.ctx
	interval := a.config.Heartbeat.Interval
	if interval == 0 {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.InfoF(ctx, "Heartbeat loop started with interval: %v / 心跳循环已启动，间隔：%v", interval, interval)

	for {
		select {
		case <-a.ctx.Done():
			logger.InfoF(ctx, "Heartbeat loop stopped / 心跳循环已停止")
			return
		case <-ticker.C:
			a.sendHeartbeat()
		}
	}
}

// sendHeartbeat sends a single heartbeat to Control Plane
// sendHeartbeat 向 Control Plane 发送单次心跳
func (a *Agent) sendHeartbeat() {
	ctx := a.ctx
	if !a.grpcClient.IsConnected() {
		return // Skip if not connected / 如果未连接则跳过
	}

	// Collect metrics / 采集指标
	usage, _ := a.metricsCollector.Collect()

	// Collect tracked process status (same view as monitor loop) and send to Control Plane.
	// This allows the server to correct DB when it was stale (e.g. DB says PID=0/stopped but process is actually running).
	// 采集与监控巡检一致的进程状态并上报，便于服务端纠正 DB 与主机不一致（如 DB 显示停止但进程实际在跑）。
	trackedProcesses := a.processMonitor.GetAllTrackedProcesses()
	processes := make([]*pb.ProcessStatus, 0, len(trackedProcesses))
	for _, proc := range trackedProcesses {
		// Check if process is alive and get metrics / 检查进程是否存活并获取指标
		status := "stopped"
		if isProcessAlive(proc.PID) {
			status = "running"
		}
		cpuUsage, memUsage := getProcessMetrics(proc.PID)

		processes = append(processes, &pb.ProcessStatus{
			Name:        proc.Name,
			Pid:         int32(proc.PID),
			Status:      status,
			CpuUsage:    cpuUsage,
			MemoryUsage: memUsage,
		})
	}

	_, err := a.grpcClient.SendHeartbeat(a.ctx, usage, processes)
	if err != nil {
		logger.ErrorF(ctx, "Heartbeat failed: %v / 心跳失败：%v", err, err)

		// Check if agent needs to re-register (Control Plane restarted)
		// 检查是否需要重新注册（Control Plane 重启了）
		if isNotFoundError(err) {
			logger.WarnF(ctx, "Agent not found on Control Plane, re-registering... / Agent 在 Control Plane 上未找到，重新注册...")
			go func() {
				ctx := context.Background()
				if regErr := a.registerWithControlPlane(); regErr != nil {
					logger.ErrorF(ctx, "Re-registration failed: %v / 重新注册失败：%v", regErr, regErr)
				}
			}()
		}
	}
}

// runCommandStreamLoop runs the command stream listener loop
// runCommandStreamLoop 运行命令流监听循环
// Requirements 1.2: Establish bidirectional gRPC stream for commands
// Requirements 1.5: Receive commands, validate, execute, and report results
// 需求 1.2：建立双向 gRPC 流用于命令
// 需求 1.5：接收命令、验证、执行并上报结果
func (a *Agent) runCommandStreamLoop() {
	ctx := a.ctx
	logger.InfoF(ctx, "Command stream loop started / 命令流循环已启动")

	for {
		select {
		case <-a.ctx.Done():
			logger.InfoF(ctx, "Command stream loop stopped / 命令流循环已停止")
			return
		default:
		}

		if !a.grpcClient.IsConnected() {
			time.Sleep(1 * time.Second)
			continue
		}

		// Start command stream / 启动命令流
		err := a.grpcClient.StartCommandStream(a.ctx, a.handleCommand)
		if err != nil {
			logger.ErrorF(ctx, "Command stream error: %v, will retry... / 命令流错误：%v，将重试...", err, err)

			// Check if agent needs to re-register (Control Plane restarted)
			// 检查是否需要重新注册（Control Plane 重启了）
			if isNotFoundError(err) {
				logger.WarnF(ctx, "Agent not found on Control Plane, re-registering... / Agent 在 Control Plane 上未找到，重新注册...")
				if regErr := a.registerWithControlPlane(); regErr != nil {
					logger.ErrorF(ctx, "Re-registration failed: %v / 重新注册失败：%v", regErr, regErr)
				}
			}

			time.Sleep(5 * time.Second)
		}
	}
}

// handleCommand handles a command received from Control Plane
// handleCommand 处理从 Control Plane 接收的命令
func (a *Agent) handleCommand(ctx context.Context, cmd *pb.CommandRequest) (*pb.CommandResponse, error) {
	logger.InfoF(ctx, "Received command: %s (type: %s) / 收到命令：%s（类型：%s）",
		cmd.CommandId, cmd.Type.String(), cmd.CommandId, cmd.Type.String())

	// Create a progress reporter that sends updates via gRPC
	// 创建通过 gRPC 发送更新的进度上报器
	reporter := &executor.CallbackReporter{
		CommandID: cmd.CommandId,
		Callback: func(commandID string, progress int32, output string) error {
			resp := executor.CreateProgressResponse(commandID, progress, output)
			return a.grpcClient.ReportCommandResult(ctx, resp)
		},
	}

	// Execute the command / 执行命令
	resp, err := a.executor.Execute(ctx, cmd, reporter)
	if err != nil {
		logger.ErrorF(ctx, "Command %s failed: %v / 命令 %s 失败：%v", cmd.CommandId, err, cmd.CommandId, err)
	} else if resp.Status == pb.CommandStatus_FAILED {
		reason := resp.Error
		if reason == "" {
			reason = "unknown"
		}
		logger.WarnF(ctx, "Command %s (type: %s) completed: FAILED, reason: %s / 命令 %s（类型：%s）完成：失败，原因：%s",
			cmd.CommandId, cmd.Type.String(), reason, cmd.CommandId, cmd.Type.String(), reason)
	} else {
		logger.InfoF(ctx, "Command %s (type: %s) completed: %s / 命令 %s（类型：%s）完成：%s",
			cmd.CommandId, cmd.Type.String(), resp.Status.String(), cmd.CommandId, cmd.Type.String(), resp.Status.String())
	}

	return resp, err
}

// runConnectionMonitor monitors connection status and triggers reconnection
// runConnectionMonitor 监控连接状态并触发重连
// Requirements 1.4: Reconnect with exponential backoff on disconnect
// 需求 1.4：断开连接时使用指数退避重连
func (a *Agent) runConnectionMonitor() {
	ctx := a.ctx
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger.InfoF(ctx, "Connection monitor started / 连接监控已启动")

	for {
		select {
		case <-a.ctx.Done():
			logger.InfoF(ctx, "Connection monitor stopped / 连接监控已停止")
			return
		case <-ticker.C:
			if !a.grpcClient.IsConnected() {
				logger.WarnF(ctx, "Connection lost, attempting reconnection... / 连接丢失，尝试重连...")
				go func() {
					ctx := context.Background()
					if err := a.grpcClient.Reconnect(a.ctx); err != nil {
						logger.ErrorF(ctx, "Reconnection failed: %v / 重连失败：%v", err, err)
					} else {
						// Re-register after reconnection / 重连后重新注册
						if err := a.registerWithControlPlane(); err != nil {
							logger.ErrorF(ctx, "Re-registration failed: %v / 重新注册失败：%v", err, err)
						}
					}
				}()
			}
		}
	}
}

// handleProcessEvent handles process lifecycle events
// handleProcessEvent 处理进程生命周期事件
func (a *Agent) handleProcessEvent(name string, event process.ProcessEvent, info *process.ProcessInfo) {
	ctx := a.ctx
	logger.InfoF(ctx, "Process event: %s - %s (PID: %d, Status: %s) / 进程事件：%s - %s（PID：%d，状态：%s）",
		name, event, info.PID, info.Status, name, event, info.PID, info.Status)

	// TODO: Report process events to Control Plane
	// TODO: 向 Control Plane 上报进程事件
}

// registerCommandHandlers registers all command handlers with the executor
// registerCommandHandlers 向执行器注册所有命令处理器
func (a *Agent) registerCommandHandlers() {
	ctx := a.ctx
	// Register precheck handler / 注册预检查处理器
	a.executor.RegisterHandler(pb.CommandType_PRECHECK, a.handlePrecheckCommand)

	// Register installation handlers / 注册安装处理器
	a.executor.RegisterHandler(pb.CommandType_INSTALL, a.handleInstallCommand)
	a.executor.RegisterHandler(pb.CommandType_UNINSTALL, a.handleUninstallCommand)
	a.executor.RegisterHandler(pb.CommandType_UPGRADE, a.handleUpgradeCommand)

	// Register process management handlers / 注册进程管理处理器
	a.executor.RegisterHandler(pb.CommandType_START, a.handleStartCommand)
	a.executor.RegisterHandler(pb.CommandType_STOP, a.handleStopCommand)
	a.executor.RegisterHandler(pb.CommandType_RESTART, a.handleRestartCommand)
	a.executor.RegisterHandler(pb.CommandType_STATUS, a.handleStatusCommand)

	// Register diagnostic handlers / 注册诊断处理器
	a.executor.RegisterHandler(pb.CommandType_COLLECT_LOGS, a.handleCollectLogsCommand)
	a.executor.RegisterHandler(pb.CommandType_THREAD_DUMP, a.handleThreadDumpCommand)
	a.executor.RegisterHandler(pb.CommandType_JVM_DUMP, a.handleJVMDumpCommand)

	// Register cluster discovery and monitoring handlers / 注册集群发现和监控处理器
	a.executor.RegisterHandler(pb.CommandType_DISCOVER_CLUSTERS, a.handleDiscoverClustersCommand)
	a.executor.RegisterHandler(pb.CommandType_UPDATE_MONITOR_CONFIG, a.handleUpdateMonitorConfigCommand)
	a.executor.RegisterHandler(pb.CommandType_REMOVE_INSTALL_DIR, a.handleRemoveInstallDirCommand)

	// Initialize plugin manager and register plugin handlers / 初始化插件管理器并注册插件处理器
	executor.InitPluginManager(a.config.SeaTunnel.InstallDir)
	executor.RegisterPluginHandlers(a.executor)

	// Register package transfer handlers / 注册安装包传输处理器
	executor.RegisterPackageHandlers(a.executor)

	// Register config handlers / 注册配置处理器
	configHandlers := executor.NewConfigHandlers()
	configHandlers.RegisterHandlers(a.executor)

	logger.InfoF(ctx, "Registered %d command handlers / 已注册 %d 个命令处理器",
		len(a.executor.GetRegisteredTypes()), len(a.executor.GetRegisteredTypes()))
}

// Command handler implementations / 命令处理器实现

func (a *Agent) handlePrecheckCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	// Check if sub_command is specified for specific precheck operations
	// 检查是否指定了 sub_command 用于特定的预检查操作
	subCommand := cmd.Parameters["sub_command"]
	if subCommand != "" && subCommand != "full" {
		// Delegate to specific precheck handlers
		// 委托给特定的预检查处理器
		return executor.HandlePrecheckCommand(ctx, cmd, reporter)
	}

	reporter.Report(10, "Starting precheck... / 开始预检查...")

	// Create precheck params from command parameters
	// 从命令参数创建预检查参数
	params := &installer.PrecheckParams{
		InstallDir:     getParamString(cmd.Parameters, "install_dir", "/opt/seatunnel"),
		MinMemoryMB:    int64(getParamInt(cmd.Parameters, "min_memory_mb", 4096)),
		MinCPUCores:    getParamInt(cmd.Parameters, "min_cpu_cores", 2),
		MinDiskSpaceMB: int64(getParamInt(cmd.Parameters, "min_disk_mb", 10240)),
		Ports:          getParamIntSlice(cmd.Parameters, "required_ports", []int{5801, 8080}),
	}

	prechecker := installer.NewPrechecker(params)
	result, err := prechecker.RunAll(ctx)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	reporter.Report(100, "Precheck completed / 预检查完成")

	// Format result as output / 将结果格式化为输出
	output := formatPrecheckResult(result)
	return executor.CreateSuccessResponse(cmd.CommandId, output), nil
}

func (a *Agent) handleInstallCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	reporter.Report(5, "Preparing installation... / 准备安装...")

	// Create install params from command parameters
	// 从命令参数创建安装参数
	version := getParamString(cmd.Parameters, "version", seatunnel.DefaultVersion())
	installDir := getParamString(cmd.Parameters, "install_dir", seatunnel.DefaultInstallDir(version))
	params := &installer.InstallParams{
		Version:        version,
		InstallDir:     installDir,
		DeploymentMode: installer.DeploymentMode(getParamString(cmd.Parameters, "deployment_mode", "hybrid")),
		NodeRole:       installer.NodeRole(getParamString(cmd.Parameters, "node_role", "master")),
		ClusterPort:    getParamInt(cmd.Parameters, "cluster_port", 5801),
		WorkerPort:     getParamInt(cmd.Parameters, "worker_port", 5802),
		HTTPPort:       getParamInt(cmd.Parameters, "http_port", 8080),
		JavaProxyPort:  getParamInt(cmd.Parameters, "java_proxy_port", 0),
		ClusterID:      getParamString(cmd.Parameters, "cluster_id", ""),
	}
	if enableHTTPValue := strings.TrimSpace(getParamString(cmd.Parameters, "enable_http", "")); enableHTTPValue != "" {
		enableHTTP := strings.EqualFold(enableHTTPValue, "true")
		params.EnableHTTP = &enableHTTP
	}

	if dynamicSlotValue := strings.TrimSpace(getParamString(cmd.Parameters, "dynamic_slot", "")); dynamicSlotValue != "" {
		dynamicSlot := strings.EqualFold(dynamicSlotValue, "true")
		params.DynamicSlot = &dynamicSlot
	}
	if slotNumValue := strings.TrimSpace(getParamString(cmd.Parameters, "slot_num", "")); slotNumValue != "" {
		if slotNum, err := strconv.Atoi(slotNumValue); err == nil && slotNum > 0 {
			params.SlotNum = &slotNum
		}
	}
	if slotAllocationStrategy := strings.TrimSpace(getParamString(cmd.Parameters, "slot_allocation_strategy", "")); slotAllocationStrategy != "" {
		params.SlotAllocationStrategy = strings.ToUpper(slotAllocationStrategy)
	}
	if jobScheduleStrategy := strings.TrimSpace(getParamString(cmd.Parameters, "job_schedule_strategy", "")); jobScheduleStrategy != "" {
		params.JobScheduleStrategy = strings.ToUpper(jobScheduleStrategy)
	}
	if historyJobExpireValue := strings.TrimSpace(getParamString(cmd.Parameters, "history_job_expire_minutes", "")); historyJobExpireValue != "" {
		if historyJobExpireMinutes, err := strconv.Atoi(historyJobExpireValue); err == nil && historyJobExpireMinutes > 0 {
			params.HistoryJobExpireMinutes = &historyJobExpireMinutes
		}
	}
	if scheduledDeletionValue := strings.TrimSpace(getParamString(cmd.Parameters, "scheduled_deletion_enable", "")); scheduledDeletionValue != "" {
		scheduledDeletionEnable := strings.EqualFold(scheduledDeletionValue, "true")
		params.ScheduledDeletionEnable = &scheduledDeletionEnable
	}
	if jobLogMode := strings.TrimSpace(getParamString(cmd.Parameters, "job_log_mode", "")); jobLogMode != "" {
		params.JobLogMode = installer.JobLogMode(strings.ToLower(jobLogMode))
	}

	// Parse JVM config / 解析 JVM 配置
	jvmHybridHeap := getParamInt(cmd.Parameters, "jvm_hybrid_heap", 0)
	jvmMasterHeap := getParamInt(cmd.Parameters, "jvm_master_heap", 0)
	jvmWorkerHeap := getParamInt(cmd.Parameters, "jvm_worker_heap", 0)
	logger.InfoF(ctx, "[Install] JVM params received: hybrid=%d, master=%d, worker=%d", jvmHybridHeap, jvmMasterHeap, jvmWorkerHeap)
	if jvmHybridHeap > 0 || jvmMasterHeap > 0 || jvmWorkerHeap > 0 {
		params.JVM = &installer.JVMConfig{
			HybridHeapSize: jvmHybridHeap,
			MasterHeapSize: jvmMasterHeap,
			WorkerHeapSize: jvmWorkerHeap,
		}
		logger.InfoF(ctx, "[Install] JVM config created: %+v", params.JVM)
	} else {
		logger.InfoF(ctx, "[Install] JVM config not created (all values are 0)")
	}

	// Parse checkpoint config / 解析检查点配置
	checkpointStorageType := getParamString(cmd.Parameters, "checkpoint_storage_type", "")
	checkpointNamespace := getParamString(cmd.Parameters, "checkpoint_namespace", "")
	if checkpointStorageType != "" {
		params.Checkpoint = &installer.CheckpointConfig{
			StorageType: installer.CheckpointStorageType(checkpointStorageType),
			Namespace:   checkpointNamespace,
		}
		// HDFS config
		hdfsHost := getParamString(cmd.Parameters, "checkpoint_hdfs_host", "")
		if hdfsHost != "" {
			params.Checkpoint.HDFSNameNodeHost = hdfsHost
			params.Checkpoint.HDFSNameNodePort = getParamInt(cmd.Parameters, "checkpoint_hdfs_port", 8020)
		}
		// HDFS Kerberos config / HDFS Kerberos 配置
		kerberosPrincipal := getParamString(cmd.Parameters, "checkpoint_kerberos_principal", "")
		if kerberosPrincipal != "" {
			params.Checkpoint.KerberosPrincipal = kerberosPrincipal
		}
		kerberosKeytabPath := getParamString(cmd.Parameters, "checkpoint_kerberos_keytab_path", "")
		if kerberosKeytabPath != "" {
			params.Checkpoint.KerberosKeytabFilePath = kerberosKeytabPath
		}
		// HDFS HA config / HDFS HA 配置
		hdfsHAEnabled := getParamString(cmd.Parameters, "checkpoint_hdfs_ha_enabled", "")
		if hdfsHAEnabled == "true" {
			params.Checkpoint.HDFSHAEnabled = true
			params.Checkpoint.HDFSNameServices = getParamString(cmd.Parameters, "checkpoint_hdfs_name_services", "")
			params.Checkpoint.HDFSHANamenodes = getParamString(cmd.Parameters, "checkpoint_hdfs_ha_namenodes", "")
			params.Checkpoint.HDFSNamenodeRPCAddress1 = getParamString(cmd.Parameters, "checkpoint_hdfs_namenode_rpc_address_1", "")
			params.Checkpoint.HDFSNamenodeRPCAddress2 = getParamString(cmd.Parameters, "checkpoint_hdfs_namenode_rpc_address_2", "")
			params.Checkpoint.HDFSFailoverProxyProvider = getParamString(cmd.Parameters, "checkpoint_hdfs_failover_proxy_provider", "")
		}
		// OSS/S3 config
		storageEndpoint := getParamString(cmd.Parameters, "checkpoint_storage_endpoint", "")
		storageBucket := getParamString(cmd.Parameters, "checkpoint_storage_bucket", "")
		if storageEndpoint != "" || storageBucket != "" {
			params.Checkpoint.StorageEndpoint = storageEndpoint
			params.Checkpoint.StorageBucket = storageBucket
			params.Checkpoint.StorageAccessKey = getParamString(cmd.Parameters, "checkpoint_storage_access_key", "")
			params.Checkpoint.StorageSecretKey = getParamString(cmd.Parameters, "checkpoint_storage_secret_key", "")
		}
		params.Checkpoint.HdfsSitePath = getParamString(cmd.Parameters, "checkpoint_hdfs_site_path", "")
		params.Checkpoint.S3CredentialsProvider = getParamString(cmd.Parameters, "checkpoint_s3_credentials_provider", "")
		logger.InfoF(ctx, "[Install] Checkpoint config created: type=%s, namespace=%s", checkpointStorageType, checkpointNamespace)
	}

	// Parse IMAP config / 解析 IMAP 配置
	imapStorageType := getParamString(cmd.Parameters, "imap_storage_type", "")
	imapNamespace := getParamString(cmd.Parameters, "imap_namespace", "")
	if imapStorageType != "" {
		params.IMAP = &installer.IMAPConfig{
			StorageType: installer.IMAPStorageType(imapStorageType),
			Namespace:   imapNamespace,
		}
		imapHDFSHost := getParamString(cmd.Parameters, "imap_hdfs_host", "")
		if imapHDFSHost != "" {
			params.IMAP.HDFSNameNodeHost = imapHDFSHost
			params.IMAP.HDFSNameNodePort = getParamInt(cmd.Parameters, "imap_hdfs_port", 8020)
		}
		imapKerberosPrincipal := getParamString(cmd.Parameters, "imap_kerberos_principal", "")
		if imapKerberosPrincipal != "" {
			params.IMAP.KerberosPrincipal = imapKerberosPrincipal
		}
		imapKerberosKeytabPath := getParamString(cmd.Parameters, "imap_kerberos_keytab_path", "")
		if imapKerberosKeytabPath != "" {
			params.IMAP.KerberosKeytabFilePath = imapKerberosKeytabPath
		}
		if getParamString(cmd.Parameters, "imap_hdfs_ha_enabled", "") == "true" {
			params.IMAP.HDFSHAEnabled = true
			params.IMAP.HDFSNameServices = getParamString(cmd.Parameters, "imap_hdfs_name_services", "")
			params.IMAP.HDFSHANamenodes = getParamString(cmd.Parameters, "imap_hdfs_ha_namenodes", "")
			params.IMAP.HDFSNamenodeRPCAddress1 = getParamString(cmd.Parameters, "imap_hdfs_namenode_rpc_address_1", "")
			params.IMAP.HDFSNamenodeRPCAddress2 = getParamString(cmd.Parameters, "imap_hdfs_namenode_rpc_address_2", "")
			params.IMAP.HDFSFailoverProxyProvider = getParamString(cmd.Parameters, "imap_hdfs_failover_proxy_provider", "")
		}
		imapStorageEndpoint := getParamString(cmd.Parameters, "imap_storage_endpoint", "")
		imapStorageBucket := getParamString(cmd.Parameters, "imap_storage_bucket", "")
		if imapStorageEndpoint != "" || imapStorageBucket != "" {
			params.IMAP.StorageEndpoint = imapStorageEndpoint
			params.IMAP.StorageBucket = imapStorageBucket
			params.IMAP.StorageAccessKey = getParamString(cmd.Parameters, "imap_storage_access_key", "")
			params.IMAP.StorageSecretKey = getParamString(cmd.Parameters, "imap_storage_secret_key", "")
		}
		params.IMAP.HdfsSitePath = getParamString(cmd.Parameters, "imap_hdfs_site_path", "")
		params.IMAP.S3CredentialsProvider = getParamString(cmd.Parameters, "imap_s3_credentials_provider", "")
		logger.InfoF(ctx, "[Install] IMAP config created: type=%s, namespace=%s", imapStorageType, imapNamespace)
	}

	// Parse master addresses / 解析 master 地址列表
	masterAddressesStr := getParamString(cmd.Parameters, "master_addresses", "")
	if masterAddressesStr != "" {
		params.MasterAddresses = strings.Split(masterAddressesStr, ",")
	}

	// Parse worker addresses (for separated mode) / 解析 worker 地址列表（分离模式）
	workerAddressesStr := getParamString(cmd.Parameters, "worker_addresses", "")
	if workerAddressesStr != "" {
		params.WorkerAddresses = strings.Split(workerAddressesStr, ",")
	}

	// Parse install mode / 解析安装模式
	installMode := getParamString(cmd.Parameters, "install_mode", "online")
	if installMode == "offline" {
		params.Mode = installer.InstallModeOffline
	} else {
		params.Mode = installer.InstallModeOnline
	}

	// Parse package path (from gRPC transfer or local) / 解析安装包路径（来自 gRPC 传输或本地）
	packagePath := getParamString(cmd.Parameters, "package_path", "")
	if packagePath != "" {
		params.PackagePath = packagePath
		params.Mode = installer.InstallModeOffline // Use offline mode when package path is provided
	}

	// Parse mirror source / 解析镜像源
	mirror := getParamString(cmd.Parameters, "mirror", "")
	if mirror != "" {
		params.Mirror = installer.MirrorSource(mirror)
	}

	// Create progress adapter / 创建进度适配器
	installReporter := &installerProgressAdapter{
		reporter:  reporter,
		commandID: cmd.CommandId,
	}

	// Use InstallStepByStep for complete installation including JVM configuration
	// 使用 InstallStepByStep 进行完整安装，包括 JVM 配置
	result, err := a.installerManager.InstallStepByStep(ctx, params, installReporter)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	if !result.Success {
		return executor.CreateErrorResponse(cmd.CommandId, result.Message), fmt.Errorf("%s", result.Message)
	}

	return executor.CreateSuccessResponse(cmd.CommandId, "Installation completed / 安装完成"), nil
}

func (a *Agent) handleUninstallCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	reporter.Report(10, "Starting uninstallation... / 开始卸载...")

	installDir := getParamString(cmd.Parameters, "install_dir", "/opt/seatunnel")

	err := a.installerManager.Uninstall(ctx, installDir)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	return executor.CreateSuccessResponse(cmd.CommandId, "Uninstallation completed / 卸载完成"), nil
}

func (a *Agent) handleUpgradeCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	return a.handleManagedUpgradeCommand(ctx, cmd, reporter)
}

func (a *Agent) handleStartCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	if isSTXJavaProxyServiceCommand(cmd.Parameters) {
		reporter.Report(10, "Starting managed stx-java-proxy service... / 启动托管 stx-java-proxy 服务...")
		status, err := installer.StartManagedSTXJavaProxyService(
			ctx,
			getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir),
			getParamString(cmd.Parameters, "version", seatunnel.DefaultVersion()),
			getParamInt(cmd.Parameters, "port", 0),
		)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		reporter.Report(100, "Managed stx-java-proxy service started / 托管 stx-java-proxy 服务已启动")
		return createSTXJavaProxyCommandResponse(cmd.CommandId, status), nil
	}

	reporter.Report(10, "Starting SeaTunnel process... / 启动 SeaTunnel 进程...")

	role := getParamString(cmd.Parameters, "role", "")
	installDir := getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir)

	// Use role as process name for tracking / 使用角色作为进程名进行跟踪
	// For hybrid mode (empty, "hybrid", or "master/worker"), use "seatunnel"
	// 对于混合模式（空、"hybrid" 或 "master/worker"），使用 "seatunnel"
	processName := "seatunnel"
	if role != "" && role != "hybrid" && role != "master/worker" {
		processName = "seatunnel-" + role
	}

	params := &process.StartParams{
		InstallDir: installDir,
		Role:       role,
		ConfigDir:  getParamString(cmd.Parameters, "config_dir", ""),
		LogDir:     getParamString(cmd.Parameters, "log_dir", ""),
	}

	// Check if auto-restart is enabled to avoid conflict
	// 检查是否启用了自动重启以避免冲突
	if a.autoRestarter.IsEnabled() {
		// Auto-restart enabled: register process with PID=0, let auto-restarter handle the actual start
		// 自动重启已启用：用 PID=0 注册进程，让自动重启器处理实际的启动
		a.processMonitor.TrackProcessSilent(processName, 0, installDir, role, params)
		a.autoRestarter.ResetRestartCount(processName)
		logger.InfoF(ctx, "[Agent] Process registered for auto-start: %s (auto-restart will handle startup) / 进程已注册等待自动启动：%s（自动重启将处理启动）",
			processName, processName)
		reporter.Report(100, "Process registered for auto-start / 进程已注册等待自动启动")
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("Process registered for auto-start (role: %s) / 进程已注册等待自动启动（角色：%s）", role, role)), nil
	}

	// Auto-restart disabled: start process directly
	// 自动重启已禁用：直接启动进程
	err := a.processManager.StartProcess(ctx, processName, params)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	// Track the process / 跟踪进程
	// Get the PID from process manager / 从进程管理器获取 PID
	if info, err := a.processManager.GetStatus(ctx, processName); err == nil && info.PID > 0 {
		a.processMonitor.TrackProcess(processName, info.PID, installDir, role, params)
		logger.InfoF(ctx, "[Agent] Process started and tracked: %s (PID: %d) / 进程已启动并跟踪：%s（PID：%d）",
			processName, info.PID, processName, info.PID)
	}

	reporter.Report(100, "Process started / 进程已启动")
	return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("Process started successfully (role: %s) / 进程启动成功（角色：%s）", role, role)), nil
}

func (a *Agent) handleStopCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	if isSTXJavaProxyServiceCommand(cmd.Parameters) {
		reporter.Report(10, "Stopping managed stx-java-proxy service... / 停止托管 stx-java-proxy 服务...")
		status, err := installer.StopManagedSTXJavaProxyService(
			ctx,
			getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir),
		)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		reporter.Report(100, "Managed stx-java-proxy service stopped / 托管 stx-java-proxy 服务已停止")
		return createSTXJavaProxyCommandResponse(cmd.CommandId, status), nil
	}

	reporter.Report(10, "Stopping SeaTunnel process... / 停止 SeaTunnel 进程...")

	role := getParamString(cmd.Parameters, "role", "")
	installDir := getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir)
	graceful := getParamBool(cmd.Parameters, "graceful", true)

	// Use role as process name for tracking / 使用角色作为进程名进行跟踪
	// For hybrid mode (empty, "hybrid", or "master/worker"), use "seatunnel"
	// 对于混合模式（空、"hybrid" 或 "master/worker"），使用 "seatunnel"
	processName := "seatunnel"
	if role != "" && role != "hybrid" && role != "master/worker" {
		processName = "seatunnel-" + role
	}

	params := &process.StopParams{
		Graceful:   graceful,
		Timeout:    30 * time.Second,
		InstallDir: installDir,
		Role:       role,
	}

	err := a.processManager.StopProcess(ctx, processName, params)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	// Check if auto-restart is enabled to decide whether to untrack or set PID=0
	// 检查是否启用了自动重启，以决定是取消跟踪还是设置 PID=0
	if a.autoRestarter.IsEnabled() {
		// Auto-restart enabled: set PID=0, auto-restart will start it again
		// 自动重启已启用：设置 PID=0，自动重启会重新启动它
		a.processMonitor.UpdateProcessPID(processName, 0)
		logger.InfoF(ctx, "[Agent] Process stopped, PID set to 0 (auto-restart enabled): %s / 进程已停止，PID 设为 0（自动重启已启用）：%s",
			processName, processName)
	} else {
		// Auto-restart disabled: untrack the process completely
		// 自动重启已禁用：完全取消跟踪进程
		a.processMonitor.UntrackProcess(processName)
		logger.InfoF(ctx, "[Agent] Process stopped and untracked (auto-restart disabled): %s / 进程已停止并取消跟踪（自动重启已禁用）：%s",
			processName, processName)
	}

	reporter.Report(100, "Process stopped / 进程已停止")
	return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("Process stopped successfully (role: %s) / 进程停止成功（角色：%s）", role, role)), nil
}

func (a *Agent) handleRestartCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	if isSTXJavaProxyServiceCommand(cmd.Parameters) {
		reporter.Report(10, "Restarting managed stx-java-proxy service... / 重启托管 stx-java-proxy 服务...")
		installDir := getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir)
		if _, err := installer.StopManagedSTXJavaProxyService(ctx, installDir); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already stopped") {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		status, err := installer.StartManagedSTXJavaProxyService(
			ctx,
			installDir,
			getParamString(cmd.Parameters, "version", seatunnel.DefaultVersion()),
			getParamInt(cmd.Parameters, "port", 0),
		)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		reporter.Report(100, "Managed stx-java-proxy service restarted / 托管 stx-java-proxy 服务已重启")
		return createSTXJavaProxyCommandResponse(cmd.CommandId, status), nil
	}

	reporter.Report(10, "Restarting SeaTunnel process... / 重启 SeaTunnel 进程...")

	role := getParamString(cmd.Parameters, "role", "")
	installDir := getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir)

	// Use role as process name for tracking / 使用角色作为进程名进行跟踪
	// For hybrid mode (empty, "hybrid", or "master/worker"), use "seatunnel"
	// 对于混合模式（空、"hybrid" 或 "master/worker"），使用 "seatunnel"
	processName := "seatunnel"
	if role != "" && role != "hybrid" && role != "master/worker" {
		processName = "seatunnel-" + role
	}

	startParams := &process.StartParams{
		InstallDir: installDir,
		Role:       role,
	}
	stopParams := &process.StopParams{
		Graceful:   true,
		Timeout:    30 * time.Second,
		InstallDir: installDir,
		Role:       role,
	}

	// Check if auto-restart is enabled to avoid conflict
	// 检查是否启用了自动重启以避免冲突
	if a.autoRestarter.IsEnabled() {
		// Auto-restart enabled: stop process, then set PID=0 to let auto-restarter handle the start
		// 自动重启已启用：停止进程，然后设置 PID=0 让自动重启器处理启动
		err := a.processManager.StopProcess(ctx, processName, stopParams)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}

		// Register process with PID=0, auto-restarter will start it
		// 用 PID=0 注册进程，自动重启器会启动它
		a.processMonitor.TrackProcessSilent(processName, 0, installDir, role, startParams)
		a.autoRestarter.ResetRestartCount(processName)
		logger.InfoF(ctx, "[Agent] Process stopped, registered for auto-restart: %s / 进程已停止，已注册等待自动重启：%s",
			processName, processName)
		reporter.Report(100, "Process stopped, auto-restart will start it / 进程已停止，自动重启将启动它")
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("Process stopped, auto-restart will start it (role: %s) / 进程已停止，自动重启将启动它（角色：%s）", role, role)), nil
	}

	// Auto-restart disabled: restart process directly
	// 自动重启已禁用：直接重启进程
	err := a.processManager.RestartProcess(ctx, processName, startParams, stopParams)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	// Update tracking with new PID / 使用新 PID 更新跟踪
	if info, err := a.processManager.GetStatus(ctx, processName); err == nil && info.PID > 0 {
		a.processMonitor.TrackProcess(processName, info.PID, installDir, role, startParams)
		logger.InfoF(ctx, "[Agent] Process restarted and tracked: %s (PID: %d) / 进程已重启并跟踪：%s（PID：%d）",
			processName, info.PID, processName, info.PID)
	}

	reporter.Report(100, "Process restarted / 进程已重启")
	return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("Process restarted successfully (role: %s) / 进程重启成功（角色：%s）", role, role)), nil
}

func (a *Agent) handleStatusCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	if isSTXJavaProxyServiceCommand(cmd.Parameters) {
		status, err := installer.GetManagedSTXJavaProxyServiceStatus(
			ctx,
			getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir),
		)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		return createSTXJavaProxyCommandResponse(cmd.CommandId, status), nil
	}

	processName := getParamString(cmd.Parameters, "process_name", "seatunnel")

	info, err := a.processManager.GetStatus(ctx, processName)
	if err != nil {
		// Process not found is not an error, just return status
		// 进程未找到不是错误，只返回状态
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("Process not found: %s / 进程未找到：%s", processName, processName)), nil
	}

	output := fmt.Sprintf("Process: %s\nPID: %d\nStatus: %s\nUptime: %v\nCPU: %.2f%%\nMemory: %d bytes",
		info.Name, info.PID, info.Status, info.Uptime, info.CPUUsage, info.MemoryUsage)

	return executor.CreateSuccessResponse(cmd.CommandId, output), nil
}

func isSTXJavaProxyServiceCommand(params map[string]string) bool {
	service := strings.TrimSpace(getParamString(params, "service", ""))
	target := strings.TrimSpace(getParamString(params, "target", ""))
	return service == "stx_java_proxy" || target == "stx_java_proxy"
}

func createSTXJavaProxyCommandResponse(commandID string, status *installer.STXJavaProxyServiceStatus) *pb.CommandResponse {
	payload, err := json.Marshal(status)
	if err != nil {
		return executor.CreateErrorResponse(commandID, err.Error())
	}
	return executor.CreateSuccessResponse(commandID, string(payload))
}

func (a *Agent) handleCollectLogsCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	reporter.Report(10, "Collecting logs... / 收集日志...")

	logFile := getParamString(cmd.Parameters, "log_file", "")
	if logFile == "" {
		return executor.CreateErrorResponse(cmd.CommandId, "log_file parameter is required / 需要 log_file 参数"), nil
	}

	// Parameters / 参数
	lines := getParamInt(cmd.Parameters, "lines", 100)
	if lines <= 0 {
		lines = 100
	}
	if lines > 2000 {
		lines = 2000 // Max 2000 lines / 最多 2000 行
	}

	// mode: "tail" (default), "head", "all"
	// mode: "tail"（默认）, "head", "all", "list"
	mode := getParamString(cmd.Parameters, "mode", "tail")

	// filter: grep pattern (optional)
	// filter: grep 过滤模式（可选）
	filter := getParamString(cmd.Parameters, "filter", "")

	// date: specific date for rolling log files (e.g., "2025-12-18")
	// date: 滚动日志文件的特定日期（如 "2025-12-18"）
	date := getParamString(cmd.Parameters, "date", "")
	startTimeRaw := getParamString(cmd.Parameters, "start_time", "")
	endTimeRaw := getParamString(cmd.Parameters, "end_time", "")

	// If date is specified, try to find the dated log file
	// 如果指定了日期，尝试查找带日期的日志文件
	actualLogFile := logFile
	if date != "" {
		// Try common rolling log patterns / 尝试常见的滚动日志模式
		// Pattern 1: seatunnel-engine-master.log.2025-12-18
		// Pattern 2: seatunnel-engine-worker.log.2025-11-12-1 (with sequence number)
		// User may input full suffix like "2025-11-12-1" directly
		// 用户可能直接输入完整后缀如 "2025-11-12-1"
		datedFile := logFile + "." + date
		if _, err := os.Stat(datedFile); err == nil {
			actualLogFile = datedFile
		} else {
			// Try glob pattern to find matching files / 尝试 glob 模式查找匹配文件
			// This handles cases like user input "2025-11-12" and we find "2025-11-12-1"
			// 这处理用户输入 "2025-11-12" 而我们找到 "2025-11-12-1" 的情况
			matches, _ := filepath.Glob(logFile + "." + date + "*")
			if len(matches) > 0 {
				// Use the first match (or latest if multiple)
				// 使用第一个匹配（如果有多个则使用最新的）
				actualLogFile = matches[len(matches)-1]
			} else {
				// Still not found, keep the dated file path for error message
				// 仍未找到，保留带日期的文件路径用于错误消息
				actualLogFile = datedFile
			}
		}
	}

	fileInfo, statErr := os.Stat(actualLogFile)
	// Check if file exists / 检查文件是否存在
	if os.IsNotExist(statErr) {
		// List available log files / 列出可用的日志文件
		dir := filepath.Dir(logFile)
		base := filepath.Base(logFile)
		files, _ := filepath.Glob(filepath.Join(dir, base+"*"))
		availableFiles := strings.Join(files, ", ")
		return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Log file not found: %s. Available files: %s / 日志文件不存在: %s。可用文件: %s", actualLogFile, availableFiles, actualLogFile, availableFiles)), nil
	}
	if statErr != nil {
		return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to stat file: %v / 文件状态获取失败: %v", statErr, statErr)), nil
	}

	if mode == "list" {
		if !fileInfo.IsDir() {
			return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Path is not a directory: %s / 路径不是目录: %s", actualLogFile, actualLogFile)), nil
		}
		type directoryEntry struct {
			Name    string    `json:"name"`
			Path    string    `json:"path"`
			Size    int64     `json:"size"`
			Mode    string    `json:"mode"`
			ModTime time.Time `json:"mod_time"`
			IsDir   bool      `json:"is_dir"`
		}
		type directoryListing struct {
			Path    string           `json:"path"`
			Entries []directoryEntry `json:"entries"`
		}
		entries, err := os.ReadDir(actualLogFile)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to list directory: %v / 目录读取失败: %v", err, err)), nil
		}
		items := make([]directoryEntry, 0, len(entries))
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			items = append(items, directoryEntry{
				Name:    entry.Name(),
				Path:    filepath.Join(actualLogFile, entry.Name()),
				Size:    info.Size(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime(),
				IsDir:   info.IsDir(),
			})
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].IsDir != items[j].IsDir {
				return !items[i].IsDir
			}
			return items[i].Name < items[j].Name
		})
		payload, err := json.Marshal(directoryListing{
			Path:    actualLogFile,
			Entries: items,
		})
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to encode directory listing: %v / 目录列表编码失败: %v", err, err)), nil
		}
		reporter.Report(100, "Directory listed successfully / 目录读取成功")
		return executor.CreateSuccessResponse(cmd.CommandId, string(payload)), nil
	}

	var output []byte
	var err error

	switch mode {
	case "head":
		// Read first N lines / 读取前 N 行
		headCmd := exec.CommandContext(ctx, "head", "-n", strconv.Itoa(lines), actualLogFile)
		output, err = headCmd.Output()
		if err != nil {
			output, err = readFirstNLines(actualLogFile, lines)
		}
	case "all":
		// Read entire file (limited) / 读取整个文件（有限制）
		output, err = os.ReadFile(actualLogFile)
		if err == nil && (startTimeRaw != "" || endTimeRaw != "") {
			startTime, parseStartErr := parseAgentLogWindowTime(startTimeRaw)
			endTime, parseEndErr := parseAgentLogWindowTime(endTimeRaw)
			if parseStartErr != nil || parseEndErr != nil {
				return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to parse time window: start=%v end=%v / 解析时间窗失败: start=%v end=%v", parseStartErr, parseEndErr, parseStartErr, parseEndErr)), nil
			}
			filtered := filterAgentLogWindowContent(string(output), startTime, endTime)
			output = []byte(filtered)
		} else if err == nil && len(output) > 500*1024 {
			// Limit to 500KB / 限制为 500KB
			output = output[:500*1024]
			output = append(output, []byte("\n... [truncated / 已截断] ...")...)
		}
	default: // tail
		// Read last N lines / 读取最后 N 行
		tailCmd := exec.CommandContext(ctx, "tail", "-n", strconv.Itoa(lines), actualLogFile)
		output, err = tailCmd.Output()
		if err != nil {
			output, err = readLastNLines(actualLogFile, lines)
		}
	}

	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to read log file: %v / 读取日志文件失败: %v", err, err)), nil
	}

	// Apply filter if specified / 如果指定了过滤器则应用
	if filter != "" {
		filteredLines := []string{}
		for _, line := range strings.Split(string(output), "\n") {
			if strings.Contains(line, filter) {
				filteredLines = append(filteredLines, line)
			}
		}
		output = []byte(strings.Join(filteredLines, "\n"))
	}

	reporter.Report(100, "Logs collected / 日志收集完成")
	return executor.CreateSuccessResponse(cmd.CommandId, string(output)), nil
}

func (a *Agent) handleThreadDumpCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	reporter.Report(10, "Collecting thread dump... / 正在采集线程栈...")

	installDir := getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir)
	role := getParamString(cmd.Parameters, "role", "")
	outputDir := getParamString(cmd.Parameters, "output_dir", filepath.Join(installDir, "logs", "diagnostics"))

	result, err := agentdiagnostics.CollectThreadDump(ctx, installDir, role, outputDir)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to collect thread dump: %v / 采集线程栈失败：%v", err, err)), nil
	}
	payload, err := agentdiagnostics.MarshalThreadDumpResult(result)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to encode thread dump result: %v / 编码线程栈结果失败：%v", err, err)), nil
	}

	reporter.Report(100, "Thread dump collected / 线程栈采集完成")
	return executor.CreateSuccessResponse(cmd.CommandId, payload), nil
}

func parseAgentLogWindowTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func filterAgentLogWindowContent(content string, start, end time.Time) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	if !start.IsZero() {
		start = start.Local()
	}
	if !end.IsZero() {
		end = end.Local()
	}
	type logEntry struct {
		ts      time.Time
		hasTime bool
		lines   []string
	}
	appendLine := func(entries []logEntry, current *logEntry, line string) ([]logEntry, *logEntry) {
		if current == nil {
			entries = append(entries, logEntry{lines: []string{line}})
			return entries, &entries[len(entries)-1]
		}
		current.lines = append(current.lines, line)
		return entries, current
	}

	entries := make([]logEntry, 0, 256)
	var current *logEntry
	for _, line := range strings.Split(content, "\n") {
		if ts, ok := parseAgentLogTimestamp(line); ok {
			entries = append(entries, logEntry{
				ts:      ts,
				hasTime: true,
				lines:   []string{line},
			})
			current = &entries[len(entries)-1]
			continue
		}
		entries, current = appendLine(entries, current, line)
	}

	filtered := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.hasTime {
			if !start.IsZero() && entry.ts.Before(start) {
				continue
			}
			if !end.IsZero() && entry.ts.After(end) {
				continue
			}
		}
		filtered = append(filtered, entry.lines...)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func parseAgentLogTimestamp(line string) (time.Time, bool) {
	matches := agentLogTimestampPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return time.Time{}, false
	}
	value := strings.TrimSpace(matches[1])
	layouts := []string{"2006-01-02 15:04:05,000", "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func (a *Agent) handleJVMDumpCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	reporter.Report(10, "Preparing JVM dump... / 准备采集 JVM Dump...")

	installDir := getParamString(cmd.Parameters, "install_dir", a.config.SeaTunnel.InstallDir)
	role := getParamString(cmd.Parameters, "role", "")
	outputDir := getParamString(cmd.Parameters, "output_dir", filepath.Join(installDir, "logs", "diagnostics"))
	minFreeMB := getParamInt(cmd.Parameters, "min_free_mb", agentdiagnostics.DefaultJVMDumpMinFreeMB)

	result, err := agentdiagnostics.CollectJVMDump(ctx, installDir, role, outputDir, int64(minFreeMB)*1024*1024)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to create JVM dump: %v / 生成 JVM Dump 失败：%v", err, err)), nil
	}
	payload, err := agentdiagnostics.MarshalJVMDumpResult(result)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, fmt.Sprintf("Failed to encode JVM dump result: %v / 编码 JVM Dump 结果失败：%v", err, err)), nil
	}

	if result.Status == agentdiagnostics.DumpStatusSkipped {
		reporter.Report(100, "JVM dump skipped due to disk policy / JVM Dump 因磁盘策略被跳过")
	} else {
		reporter.Report(100, "JVM dump created / JVM Dump 采集完成")
	}
	return executor.CreateSuccessResponse(cmd.CommandId, payload), nil
}

// readLastNLines reads the last N lines from a file
// readLastNLines 从文件中读取最后 N 行
func readLastNLines(filename string, n int) ([]byte, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	startIdx := len(lines) - n
	if startIdx < 0 {
		startIdx = 0
	}
	return []byte(strings.Join(lines[startIdx:], "\n")), nil
}

// readFirstNLines reads the first N lines from a file
// readFirstNLines 从文件中读取前 N 行
func readFirstNLines(filename string, n int) ([]byte, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// Shutdown gracefully stops the Agent service
// Shutdown 优雅地停止 Agent 服务
// Requirements 1.6: Graceful shutdown - complete running tasks, send offline notification, close connections
// 需求 1.6：优雅关闭 - 完成正在执行的任务、发送下线通知、关闭连接
func (a *Agent) Shutdown() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	a.running = false
	a.mu.Unlock()

	ctx := context.Background()
	logger.InfoF(ctx, "========================================")
	logger.InfoF(ctx, "  Shutting down Agent service...")
	logger.InfoF(ctx, "  正在关闭 Agent 服务...")
	logger.InfoF(ctx, "========================================")

	// Step 1: Stop accepting new commands
	// 步骤 1：停止接受新命令
	logger.InfoF(ctx, "[1/6] Stopping command acceptance... / 停止接受命令...")

	// Step 2: Stop process monitor / 停止进程监控器
	logger.InfoF(ctx, "[2/6] Stopping process monitor... / 停止进程监控器...")
	if err := a.processMonitor.Stop(); err != nil {
		logger.WarnF(ctx, "Warning: Error stopping process monitor: %v / 警告：停止进程监控器时出错：%v", err, err)
	}

	// Step 3: Stop event reporter / 停止事件上报器
	logger.InfoF(ctx, "[3/6] Stopping event reporter... / 停止事件上报器...")
	a.eventReporter.Stop()

	// Step 4: Wait for running tasks to complete (with timeout)
	// 步骤 4：等待运行中的任务完成（带超时）
	logger.InfoF(ctx, "[4/6] Waiting for running tasks... / 等待运行中的任务...")
	// Note: The executor handles task completion internally
	// 注意：执行器内部处理任务完成

	// Step 5: Keep SeaTunnel processes running (do NOT stop them)
	// 步骤 5：保持 SeaTunnel 进程运行（不停止它们）
	// Agent restart should not affect running SeaTunnel processes
	// Agent 重启不应影响正在运行的 SeaTunnel 进程
	logger.InfoF(ctx, "[5/6] Keeping SeaTunnel processes running... / 保持 SeaTunnel 进程运行...")
	// Note: We intentionally do NOT call processManager.StopAll() here
	// 注意：我们故意不在这里调用 processManager.StopAll()

	// Stop process manager (just cleanup internal state, not the processes)
	// 停止进程管理器（只清理内部状态，不停止进程）
	if err := a.processManager.Stop(); err != nil {
		logger.WarnF(ctx, "Warning: Error stopping process manager: %v / 警告：停止进程管理器时出错：%v", err, err)
	}

	// Step 6: Close gRPC connection
	// 步骤 6：关闭 gRPC 连接
	logger.InfoF(ctx, "[6/6] Closing connections... / 关闭连接...")
	if err := a.grpcClient.Disconnect(); err != nil {
		logger.WarnF(ctx, "Warning: Error disconnecting: %v / 警告：断开连接时出错：%v", err, err)
	}

	// Cancel main context to stop all goroutines
	// 取消主上下文以停止所有 goroutine
	a.cancel()

	// Wait for all goroutines to finish (with timeout)
	// 等待所有 goroutine 完成（带超时）
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.InfoF(ctx, "All goroutines stopped / 所有 goroutine 已停止")
	case <-time.After(10 * time.Second):
		logger.WarnF(ctx, "Timeout waiting for goroutines / 等待 goroutine 超时")
	}

	logger.InfoF(ctx, "========================================")
	logger.InfoF(ctx, "  Agent shutdown complete")
	logger.InfoF(ctx, "  Agent 关闭完成")
	logger.InfoF(ctx, "========================================")
}

// Helper functions / 辅助函数

// getParamString gets a string parameter with default value
// getParamString 获取字符串参数，带默认值
func getParamString(params map[string]string, key, defaultValue string) string {
	if v, ok := params[key]; ok && v != "" {
		return v
	}
	return defaultValue
}

// getParamInt gets an integer parameter with default value
// getParamInt 获取整数参数，带默认值
func getParamInt(params map[string]string, key string, defaultValue int) int {
	if v, ok := params[key]; ok && v != "" {
		var result int
		if _, err := fmt.Sscanf(v, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

// getParamBool gets a boolean parameter with default value
// getParamBool 获取布尔参数，带默认值
func getParamBool(params map[string]string, key string, defaultValue bool) bool {
	if v, ok := params[key]; ok {
		return v == "true" || v == "1" || v == "yes"
	}
	return defaultValue
}

// getParamIntSlice gets an integer slice parameter with default value
// getParamIntSlice 获取整数切片参数，带默认值
func getParamIntSlice(params map[string]string, key string, defaultValue []int) []int {
	// For simplicity, return default value
	// 为简单起见，返回默认值
	// TODO: Implement parsing of comma-separated integers
	// TODO: 实现逗号分隔整数的解析
	return defaultValue
}

// formatPrecheckResult formats precheck result as string
// formatPrecheckResult 将预检查结果格式化为字符串
func formatPrecheckResult(result *installer.PrecheckResult) string {
	var sb string
	sb = "Precheck Results / 预检查结果:\n"
	sb += "================================\n"

	for _, item := range result.Items {
		statusIcon := "✓"
		if item.Status == installer.CheckStatusFailed {
			statusIcon = "✗"
		} else if item.Status == installer.CheckStatusWarning {
			statusIcon = "⚠"
		}
		sb += fmt.Sprintf("%s %s: %s\n", statusIcon, item.Name, item.Message)
	}

	sb += "================================\n"
	if result.OverallStatus == installer.CheckStatusPassed {
		sb += "Overall: PASSED / 总体：通过"
	} else if result.OverallStatus == installer.CheckStatusWarning {
		sb += "Overall: PASSED WITH WARNINGS / 总体：通过（有警告）"
	} else {
		sb += "Overall: FAILED / 总体：失败"
	}

	return sb
}

// installerProgressAdapter adapts installer.ProgressReporter to executor.ProgressReporter
// installerProgressAdapter 将 installer.ProgressReporter 适配到 executor.ProgressReporter
type installerProgressAdapter struct {
	reporter  executor.ProgressReporter
	commandID string
}

func (a *installerProgressAdapter) Report(step installer.InstallStep, progress int, message string) error {
	return a.reporter.Report(int32(progress), fmt.Sprintf("[%s] %s", step, message))
}

func (a *installerProgressAdapter) ReportStepStart(step installer.InstallStep) error {
	return a.reporter.Report(0, fmt.Sprintf("Starting step: %s / 开始步骤：%s", step, step))
}

func (a *installerProgressAdapter) ReportStepComplete(step installer.InstallStep) error {
	return a.reporter.Report(100, fmt.Sprintf("Completed step: %s / 完成步骤：%s", step, step))
}

func (a *installerProgressAdapter) ReportStepFailed(step installer.InstallStep, err error) error {
	return a.reporter.Report(0, fmt.Sprintf("Failed step: %s - %v / 失败步骤：%s - %v", step, err, step, err))
}

func (a *installerProgressAdapter) ReportStepSkipped(step installer.InstallStep, reason string) error {
	return a.reporter.Report(0, fmt.Sprintf("Skipped step: %s - %s / 跳过步骤：%s - %s", step, reason, step, reason))
}

// rootCmd is the root command for the Agent CLI
// rootCmd 是 Agent CLI 的根命令
var rootCmd = &cobra.Command{
	Use:   "stx-agent",
	Short: "STX Agent - Node daemon for SeaTunnel cluster management",
	Long: `STX Agent is a daemon process deployed on physical/VM nodes.
STX Agent 是部署在物理机/VM 节点上的守护进程。

It communicates with the Control Plane via gRPC to:
它通过 gRPC 与 Control Plane 通信，用于：
- Register and report heartbeat / 注册和上报心跳
- Execute installation and deployment commands / 执行安装和部署命令
- Manage SeaTunnel process lifecycle / 管理 SeaTunnel 进程生命周期
- Collect and report metrics / 采集和上报指标`,
	RunE: runAgent,
}

// versionCmd shows version information
// versionCmd 显示版本信息
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information / 打印版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		msg := fmt.Sprintf(
			"STX Agent\n  Version:    %s\n  Git Commit: %s\n  Build Time: %s\n  Go Version: %s\n  OS/Arch:    %s/%s\n",
			Version, GitCommit, BuildTime, runtime.Version(), runtime.GOOS, runtime.GOARCH,
		)
		// 同时打印到控制台和写入日志，保持 CLI 体验又统一日志出口
		fmt.Print(msg)
		logger.InfoF(ctx, "%s", strings.TrimSpace(msg))
	},
}

// configFile is the path to the configuration file
// configFile 是配置文件的路径
var (
	configFile string
	hostIDFlag uint64
	ipFlag     string
)

func init() {
	// Add flags to root command
	// 向根命令添加标志
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path (default: /etc/stx-agent/config.yaml)")
	rootCmd.PersistentFlags().Uint64Var(&hostIDFlag, "host-id", 0, "pre-assigned host ID from Control Plane / 来自 Control Plane 的预分配主机 ID")
	rootCmd.PersistentFlags().StringVar(&ipFlag, "ip", "", "manually override outbound IP / 手动覆盖出口 IP")

	// Add subcommands
	// 添加子命令
	rootCmd.AddCommand(versionCmd)
}

// runAgent is the main entry point for the Agent service
// runAgent 是 Agent 服务的主入口点
func runAgent(cmd *cobra.Command, args []string) error {
	// Load configuration
	// 加载配置
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w / 加载配置失败：%w", err, err)
	}

	// Apply CLI overrides if provided
	// 如果提供了命令行覆盖参数，则应用
	if hostIDFlag > 0 {
		cfg.Agent.HostID = hostIDFlag
	}
	if ipFlag != "" {
		cfg.Agent.IP = ipFlag
	}

	// Validate configuration
	// 验证配置
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w / 无效配置：%w", err, err)
	}

	// 初始化 Agent 日志（同时输出到控制台和文件）
	if err := logger.Init(cfg); err != nil {
		return fmt.Errorf("failed to init logger: %w / 初始化日志失败：%w", err, err)
	}

	// Create agent
	// 创建 Agent
	agent := NewAgent(cfg)

	// Setup signal handling for graceful shutdown
	// 设置信号处理以实现优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Run agent in goroutine
	// 在 goroutine 中运行 Agent
	errChan := make(chan error, 1)
	go func() {
		errChan <- agent.Run()
	}()

	// Wait for signal or error
	// 等待信号或错误
	select {
	case sig := <-sigChan:
		ctx := context.Background()
		logger.InfoF(ctx, "Received signal: %v / 收到信号：%v", sig, sig)
		agent.Shutdown()
	case err := <-errChan:
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// isNotFoundError checks if the error indicates agent not found on Control Plane
// isNotFoundError 检查错误是否表示 Agent 在 Control Plane 上未找到
// This typically happens when Control Plane restarts and loses in-memory agent state
// 这通常发生在 Control Plane 重启并丢失内存中的 Agent 状态时
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "NotFound") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "not registered") ||
		strings.Contains(errStr, "re-register")
}

// handleDiscoverClustersCommand handles the DISCOVER_CLUSTERS command (simplified)
// handleDiscoverClustersCommand 处理 DISCOVER_CLUSTERS 命令（简化版）
// Only scans for running SeaTunnel processes, returns PID, role, install_dir
// 只扫描运行中的 SeaTunnel 进程，返回 PID、角色、安装目录
func (a *Agent) handleDiscoverClustersCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	reporter.Report(10, "Scanning for SeaTunnel processes... / 正在扫描 SeaTunnel 进程...")

	// Use simplified process discovery / 使用简化的进程发现
	processDiscovery := discovery.NewProcessDiscovery()
	processes, err := processDiscovery.DiscoverProcesses()
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	reporter.Report(80, fmt.Sprintf("Found %d process(es) / 发现 %d 个进程", len(processes), len(processes)))

	// Format output as JSON for easier parsing / 格式化输出为 JSON 以便于解析
	// Include all discovered fields: PID, role, install_dir, version, hazelcast_port, api_port
	// 包含所有发现的字段：PID、角色、安装目录、版本、hazelcast端口、api端口
	type ProcessInfo struct {
		PID           int    `json:"pid"`
		Role          string `json:"role"`
		InstallDir    string `json:"install_dir"`
		Version       string `json:"version"`
		HazelcastPort int    `json:"hazelcast_port"`
		APIPort       int    `json:"api_port"`
	}
	type DiscoveryResult struct {
		Success   bool          `json:"success"`
		Message   string        `json:"message"`
		Processes []ProcessInfo `json:"processes"`
	}

	result := DiscoveryResult{
		Success:   true,
		Message:   fmt.Sprintf("Discovered %d SeaTunnel process(es) / 发现 %d 个 SeaTunnel 进程", len(processes), len(processes)),
		Processes: make([]ProcessInfo, 0, len(processes)),
	}
	for _, p := range processes {
		result.Processes = append(result.Processes, ProcessInfo{
			PID:           p.PID,
			Role:          p.Role,
			InstallDir:    p.InstallDir,
			Version:       p.Version,
			HazelcastPort: p.HazelcastPort,
			APIPort:       p.APIPort,
		})
	}

	jsonOutput, err := json.Marshal(result)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	reporter.Report(100, "Process discovery completed / 进程发现完成")
	return executor.CreateSuccessResponse(cmd.CommandId, string(jsonOutput)), nil
}

func (a *Agent) updateDiagnosticsTargets(ctx context.Context, trackedProcessesJSON string) {
	if a.errorCollector == nil {
		return
	}

	targets := make([]*agentdiagnostics.ScanTarget, 0)
	if strings.TrimSpace(trackedProcessesJSON) != "" {
		var trackedProcesses []struct {
			PID        int    `json:"pid"`
			Name       string `json:"name"`
			InstallDir string `json:"install_dir"`
			Role       string `json:"role"`
		}
		if err := json.Unmarshal([]byte(trackedProcessesJSON), &trackedProcesses); err != nil {
			logger.ErrorF(ctx, "[Agent] Failed to parse diagnostics tracked_processes: %v / 解析 diagnostics tracked_processes 失败：%v", err, err)
			return
		}
		for _, proc := range trackedProcesses {
			targets = append(targets, &agentdiagnostics.ScanTarget{
				Name:       proc.Name,
				InstallDir: proc.InstallDir,
				Role:       proc.Role,
			})
		}
	}

	a.errorCollector.ReplaceTargets(targets)
	logger.InfoF(ctx, "[Diagnostics] Updated %d scan targets / 已更新 %d 个诊断扫描目标", len(targets), len(targets))
}

// handleUpdateMonitorConfigCommand handles the UPDATE_MONITOR_CONFIG command
// handleUpdateMonitorConfigCommand 处理 UPDATE_MONITOR_CONFIG 命令
// Requirements 5.5: Apply new config immediately without restart
// 需求 5.5：立即应用新配置，无需重启
func (a *Agent) handleUpdateMonitorConfigCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	reporter.Report(10, "Updating monitor config... / 更新监控配置...")

	// Parse config from parameters / 从参数解析配置
	autoMonitorEnabled := getParamBool(cmd.Parameters, "auto_monitor", true)
	autoRestartEnabled := getParamBool(cmd.Parameters, "auto_restart", true)
	trackedProcessesJSON := getParamString(cmd.Parameters, "tracked_processes", "")
	config := &restart.RestartConfig{
		Enabled:        autoRestartEnabled,
		RestartDelay:   time.Duration(getParamInt(cmd.Parameters, "restart_delay", 10)) * time.Second,
		MaxRestarts:    getParamInt(cmd.Parameters, "max_restarts", 3),
		TimeWindow:     time.Duration(getParamInt(cmd.Parameters, "time_window", 300)) * time.Second,
		CooldownPeriod: time.Duration(getParamInt(cmd.Parameters, "cooldown_period", 1800)) * time.Second,
	}

	// Update auto restarter config / 更新自动重启器配置
	a.autoRestarter.UpdateConfig(config)

	// Update process monitor interval if specified / 如果指定则更新进程监控间隔
	if monitorInterval := getParamInt(cmd.Parameters, "monitor_interval", 0); monitorInterval > 0 {
		a.processMonitor.SetMonitorInterval(time.Duration(monitorInterval) * time.Second)
	}

	// Update diagnostics scan targets from Control Plane.
	// Diagnostics binds to managed monitoring targets instead of auto-restart.
	// 根据 Control Plane 下发的受管进程刷新诊断扫描目标。
	if autoMonitorEnabled {
		a.updateDiagnosticsTargets(ctx, trackedProcessesJSON)
	} else {
		a.updateDiagnosticsTargets(ctx, "")
	}

	// If auto-monitor is disabled, untrack all processes immediately.
	// 如果禁用了自动监控，静默取消跟踪所有进程（不发送事件，因为进程仍在运行）
	if !autoMonitorEnabled {
		trackedProcesses := a.processMonitor.GetAllTrackedProcesses()
		for _, proc := range trackedProcesses {
			a.processMonitor.UntrackProcessSilent(proc.Name)
			logger.InfoF(ctx, "[Agent] Auto-monitor disabled, stopped monitoring process: %s / 自动监控已禁用，停止监控进程：%s",
				proc.Name, proc.Name)
		}
		reporter.Report(100, "Monitor config updated (auto-monitor disabled, stopped monitoring) / 监控配置已更新（自动监控已禁用，停止监控）")
		return executor.CreateSuccessResponse(cmd.CommandId, "Monitor config updated, auto-monitor disabled / 监控配置已更新，自动监控已禁用"), nil
	}

	// Parse and track processes from Control Plane.
	// 解析并跟踪来自 Control Plane 的受管进程。
	if trackedProcessesJSON != "" {
		var trackedProcesses []struct {
			PID        int    `json:"pid"`
			Name       string `json:"name"`
			InstallDir string `json:"install_dir"`
			Role       string `json:"role"`
		}
		if err := json.Unmarshal([]byte(trackedProcessesJSON), &trackedProcesses); err != nil {
			logger.ErrorF(ctx, "[Agent] Failed to parse tracked_processes: %v / 解析 tracked_processes 失败：%v", err, err)
		} else {
			logger.InfoF(ctx, "[Agent] Received %d processes to track (auto_monitor=%t, auto_restart=%t) / 收到 %d 个需要跟踪的进程（自动监控=%t，自动拉起=%t）",
				len(trackedProcesses), autoMonitorEnabled, autoRestartEnabled, len(trackedProcesses), autoMonitorEnabled, autoRestartEnabled)

			expected := make(map[string]struct{}, len(trackedProcesses))
			for _, proc := range trackedProcesses {
				expected[proc.Name] = struct{}{}
			}
			for _, existing := range a.processMonitor.GetAllTrackedProcesses() {
				if _, ok := expected[existing.Name]; ok {
					continue
				}
				a.processMonitor.UntrackProcessSilent(existing.Name)
				logger.InfoF(ctx, "[Agent] Untracked stale monitored process: %s / 已移除过期受监控进程：%s", existing.Name, existing.Name)
			}

			for _, proc := range trackedProcesses {
				// Create start params for potential restart / 创建启动参数用于可能的重启
				startParams := &process.StartParams{
					InstallDir: proc.InstallDir,
					Role:       proc.Role,
				}

				if proc.PID > 0 {
					a.processMonitor.TrackProcessSilent(proc.Name, proc.PID, proc.InstallDir, proc.Role, startParams)
					logger.InfoF(ctx, "[Agent] Tracking running process (silent): %s (PID: %d, Role: %s, Dir: %s) / 静默跟踪运行中的进程：%s（PID：%d，角色：%s，目录：%s）",
						proc.Name, proc.PID, proc.Role, proc.InstallDir, proc.Name, proc.PID, proc.Role, proc.InstallDir)
				} else if autoRestartEnabled {
					a.processMonitor.TrackProcessSilent(proc.Name, 0, proc.InstallDir, proc.Role, startParams)
					logger.InfoF(ctx, "[Agent] Registered stopped process (will auto-restart): %s (Role: %s, Dir: %s) / 注册已停止的进程（将自动重启）：%s（角色：%s，目录：%s）",
						proc.Name, proc.Role, proc.InstallDir, proc.Name, proc.Role, proc.InstallDir)
				} else {
					logger.InfoF(ctx, "[Agent] Skip stopped process without auto-restart: %s (Role: %s, Dir: %s) / 自动拉起已禁用，跳过已停止进程：%s（角色：%s，目录：%s）",
						proc.Name, proc.Role, proc.InstallDir, proc.Name, proc.Role, proc.InstallDir)
				}
			}
		}
	}

	reporter.Report(100, "Monitor config updated / 监控配置已更新")
	return executor.CreateSuccessResponse(cmd.CommandId, "Monitor config updated successfully / 监控配置更新成功"), nil
}

// handleRemoveInstallDirCommand handles the REMOVE_INSTALL_DIR command (force delete: remove install directory on host).
// handleRemoveInstallDirCommand 处理 REMOVE_INSTALL_DIR 命令（强制删除：删除主机上的安装目录）。
func (a *Agent) handleRemoveInstallDirCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	reporter.Report(10, "Removing install directory... / 正在删除安装目录...")

	installDir := getParamString(cmd.Parameters, "install_dir", "")
	if installDir == "" {
		msg := "install_dir is required / install_dir 为必填"
		return executor.CreateErrorResponse(cmd.CommandId, msg), fmt.Errorf("%s", msg)
	}

	removedDir, err := installer.RemoveManagedInstallDir(installDir)
	if err != nil {
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	logger.InfoF(ctx, "[Agent] Removed install directory: %s / 已删除安装目录：%s", removedDir, removedDir)
	reporter.Report(100, "Install directory removed / 安装目录已删除")
	return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("Install directory removed: %s / 安装目录已删除：%s", removedDir, removedDir)), nil
}

// isProcessAlive checks if a process with the given PID is alive
// isProcessAlive 检查给定 PID 的进程是否存活
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0 to check
	// 在 Unix 上，FindProcess 总是成功，所以我们需要发送信号 0 来检查
	if runtime.GOOS != "windows" {
		err = process.Signal(syscall.Signal(0))
		return err == nil
	}

	// On Windows, use tasklist / 在 Windows 上使用 tasklist
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), strconv.Itoa(pid))
}

// getProcessMetrics gets CPU and memory usage for a process
// getProcessMetrics 获取进程的 CPU 和内存使用率
func getProcessMetrics(pid int) (cpuUsage float64, memoryUsage int64) {
	if pid <= 0 {
		return 0, 0
	}

	switch runtime.GOOS {
	case "linux":
		// Read /proc/[pid]/statm for memory info
		// 读取 /proc/[pid]/statm 获取内存信息
		statmPath := fmt.Sprintf("/proc/%d/statm", pid)
		statmData, err := os.ReadFile(statmPath)
		if err != nil {
			return 0, 0
		}
		statmFields := strings.Fields(string(statmData))
		if len(statmFields) >= 2 {
			// RSS is in pages, convert to bytes (assuming 4KB pages)
			// RSS 以页为单位，转换为字节（假设 4KB 页）
			rss, _ := strconv.ParseInt(statmFields[1], 10, 64)
			memoryUsage = rss * 4096
		}
		return 0, memoryUsage

	case "darwin":
		cmd := exec.Command("ps", "-o", "rss=,pcpu=", "-p", strconv.Itoa(pid))
		output, err := cmd.Output()
		if err != nil {
			return 0, 0
		}
		fields := strings.Fields(string(output))
		if len(fields) >= 2 {
			rss, _ := strconv.ParseInt(fields[0], 10, 64)
			memoryUsage = rss * 1024
			cpu, _ := strconv.ParseFloat(fields[1], 64)
			cpuUsage = cpu
		}
		return cpuUsage, memoryUsage

	case "windows":
		cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", pid), "get", "WorkingSetSize", "/value")
		output, err := cmd.Output()
		if err != nil {
			return 0, 0
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "WorkingSetSize=") {
				value := strings.TrimPrefix(line, "WorkingSetSize=")
				value = strings.TrimSpace(value)
				mem, _ := strconv.ParseInt(value, 10, 64)
				memoryUsage = mem
			}
		}
		return 0, memoryUsage

	default:
		return 0, 0
	}
}
