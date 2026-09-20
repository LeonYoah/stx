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

// Package router 提供 HTTP 路由配置
// Package router provides HTTP routing configuration
package router

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/LeonYoah/stx/docs"
	"github.com/LeonYoah/stx/internal/apps/admin"
	"github.com/LeonYoah/stx/internal/apps/agent"
	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/apps/capability"
	"github.com/LeonYoah/stx/internal/apps/cluster"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	"github.com/LeonYoah/stx/internal/apps/dashboard"
	"github.com/LeonYoah/stx/internal/apps/deepwiki"
	"github.com/LeonYoah/stx/internal/apps/diagnostics"
	"github.com/LeonYoah/stx/internal/apps/discovery"
	"github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/apps/health"
	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/apps/oauth"
	"github.com/LeonYoah/stx/internal/apps/plugin"
	"github.com/LeonYoah/stx/internal/apps/releasebundle"
	"github.com/LeonYoah/stx/internal/apps/stupgrade"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/apps/task"
	"github.com/LeonYoah/stx/internal/apps/userwrite"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	grpcServer "github.com/LeonYoah/stx/internal/grpc"
	"github.com/LeonYoah/stx/internal/otel_trace"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"github.com/LeonYoah/stx/internal/session"
	"github.com/LeonYoah/stx/internal/tlsbootstrap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"
)

func Serve() {
	auth.OnUserBound = audit.BindOperator
	ctx := context.Background()

	// Initialize OpenTelemetry tracing (based on config)
	// 初始化 OpenTelemetry 追踪（根据配置）
	otel_trace.Init()
	defer otel_trace.Shutdown(ctx)

	// 运行模式
	// Set run mode
	if config.Config.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 初始化数据库（根据配置自动选择 SQLite、MySQL 或 PostgreSQL）
	// Initialize database (auto-select SQLite, MySQL or PostgreSQL based on config)
	if err := db.InitDatabase(); err != nil {
		log.Fatalf("[API] 初始化数据库失败: %v\n", err)
	}

	// 初始化 gRPC 服务器（如果启用）
	// Initialize gRPC server (if enabled)
	// Requirements: 1.1, 3.4 - Starts gRPC server and heartbeat timeout detection
	var grpcSrv *grpcServer.Server
	var agentManager *agent.Manager
	if config.IsGRPCEnabled() {
		// Provision gRPC TLS only when grpc.tls_enabled is explicitly true.
		// 仅当 grpc.tls_enabled 显式为 true 时准备 gRPC TLS。
		if _, err := tlsbootstrap.EnsureGRPCTLS(tlsbootstrap.Options{}); err != nil {
			log.Printf("[TLS] 准备 gRPC TLS 失败，将按当前配置继续: %v / Failed to bootstrap gRPC TLS, continuing with current config: %v\n", err, err)
		}
		grpcSrv, agentManager = initGRPCServer(ctx)
		if grpcSrv != nil {
			defer grpcSrv.Stop()
		}
		if agentManager != nil {
			defer agentManager.Stop()
		}
	} else {
		log.Println("[API] gRPC 服务器已禁用 / gRPC server is disabled")
	}

	// 初始化路由
	// Initialize router
	r := gin.New()
	r.Use(gin.Recovery())

	// 初始化会话存储（默认使用内存会话）
	// Initialize session store (uses in-memory sessions by default)
	if err := session.InitSessionStore(); err != nil {
		log.Fatalf("[API] 初始化会话存储失败: %v\n", err)
	}
	r.Use(sessions.Sessions(config.Config.App.SessionCookieName, session.GinStore))

	// 初始化 OAuth 提供商（GitHub、Google）
	// Initialize OAuth providers (GitHub, Google)
	oauth.InitOAuthProviders()

	// 补充中间件
	// Add middleware
	r.Use(otelgin.Middleware(config.Config.App.AppName), loggerMiddleware())

	apiGroup := r.Group(config.Config.App.APIPrefix)
	{
		if config.Config.App.Env == "development" {
			// Swagger
			apiGroup.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
		}

		// API V1
		apiV1Router := apiGroup.Group("/v1")
		{
			// 公共执行服务在写接口注册前创建，避免业务处理器依赖全局可变实例。
			// The shared execution service is created before write routes so business handlers do not depend on mutable globals.
			auditRepo := audit.NewRepository(db.DB(context.Background()))
			executionRepo := execution.NewRepository(db.DB(context.Background()))
			executionProviders := execution.NewProviderRegistry()
			executionService := execution.NewService(executionRepo, executionProviders)
			executionService.SetAuditRepository(auditRepo)
			userWriteHandler := userwrite.NewHandler(executionService, auditRepo)

			// Health
			apiV1Router.GET("/health", health.Health)

			// Auth（统一认证接口，支持密码登录和 OAuth 登录）
			apiV1Router.POST("/auth/login", auth.Login)
			apiV1Router.POST("/auth/logout", auth.LoginRequired(), auth.Logout)
			apiV1Router.GET("/auth/user-info", auth.LoginRequired(), auth.GetUserInfo)
			apiV1Router.PUT("/auth/profile", auth.LoginRequired(), userWriteHandler.UpdateProfile)
			apiV1Router.POST("/auth/cli/login", auth.CLIClientRequired(), auth.CLILogin)
			apiV1Router.POST("/auth/cli/logout", auth.CLIAuthRequired(), auth.CLILogout)
			apiV1Router.GET("/auth/cli/whoami", auth.CLIAuthRequired(), auth.CLIWhoAmI)
			apiV1Router.GET("/capabilities", auth.CLIAuthRequired(), capability.List)

			// OAuth（备选登录方式：GitHub、Google）
			apiV1Router.GET("/oauth/providers", oauth.GetEnabledProvidersHandler)
			apiV1Router.GET("/oauth/login", oauth.GetLoginURL)
			apiV1Router.POST("/oauth/callback", oauth.Callback)

			// Admin
			adminRouter := apiV1Router.Group("/admin")
			adminRouter.Use(auth.LoginRequired(), admin.LoginAdminRequired())
			{
				// User 用户管理
				userAdminRouter := adminRouter.Group("/users")
				{
					userAdminRouter.GET("", admin.ListUsersHandler)
					userAdminRouter.POST("", userWriteHandler.CreateUser)
					userAdminRouter.GET("/:id", admin.GetUserHandler)
					userAdminRouter.PUT("/:id", userWriteHandler.UpdateUser)
					userAdminRouter.DELETE("/:id", userWriteHandler.DeleteUser)
				}
			}

			// Host 主机管理
			// Initialize host service and handler
			// 初始化主机服务和处理器
			hostRepo := host.NewRepository(db.DB(context.Background()))
			clusterRepo := cluster.NewRepository(db.DB(context.Background()))

			// 公共执行协议只保存统一状态和安全信息，具体执行仍由业务模块负责。
			// The shared execution contract stores common state and safety data while business modules remain responsible for actual work.
			executionHandler := execution.NewHandler(executionService)
			executionRouter := apiV1Router.Group("/executions")
			executionRouter.Use(auth.LoginRequired())
			{
				executionRouter.GET("/:id", executionHandler.Get)
				executionRouter.GET("/:id/wait", executionHandler.Wait)
				executionRouter.POST("/:id/cancel", executionHandler.Cancel)
			}

			hostService := host.NewService(hostRepo, clusterRepo, &host.ServiceConfig{
				HeartbeatTimeout: time.Duration(config.Config.GRPC.HeartbeatTimeout) * time.Second,
				ControlPlaneAddr: config.GetExternalURL(),
			})
			hostHandler := host.NewHandler(hostService, auditRepo)

			hostRouter := apiV1Router.Group("/hosts")
			hostRouter.Use(auth.LoginRequired())
			{
				hostRouter.POST("", hostHandler.CreateHost)
				hostRouter.GET("", hostHandler.ListHosts)
				hostRouter.GET("/:id", hostHandler.GetHost)
				hostRouter.PUT("/:id", hostHandler.UpdateHost)
				hostRouter.DELETE("/:id", hostHandler.DeleteHost)
				hostRouter.GET("/:id/install-command", hostHandler.GetInstallCommand)
			}

			// Dashboard Overview 仪表盘概览
			// Initialize dashboard overview service and handler
			// 初始化仪表盘概览服务和处理器
			overviewService := dashboard.NewOverviewService(hostRepo, clusterRepo, auditRepo, time.Duration(config.Config.GRPC.HeartbeatTimeout)*time.Second, hostService.GetProcessStartedAt())
			overviewHandler := dashboard.NewOverviewHandler(overviewService)

			overviewRouter := apiV1Router.Group("/dashboard/overview")
			overviewRouter.Use(auth.LoginRequired())
			{
				overviewRouter.GET("", overviewHandler.GetOverviewData)
				overviewRouter.GET("/stats", overviewHandler.GetOverviewStats)
				overviewRouter.GET("/clusters", overviewHandler.GetClusterSummaries)
				overviewRouter.GET("/hosts", overviewHandler.GetHostSummaries)
				overviewRouter.GET("/activities", overviewHandler.GetRecentActivities)
			}

			// Cluster 集群管理
			// Initialize cluster service and handler
			// 初始化集群服务和处理器
			clusterService := cluster.NewService(clusterRepo, hostService, &cluster.ServiceConfig{
				HeartbeatTimeout: time.Duration(config.Config.GRPC.HeartbeatTimeout) * time.Second,
			})

			// Inject agent command sender if agent manager is available
			// 如果 Agent Manager 可用，注入 Agent 命令发送器
			if agentManager != nil {
				clusterService.SetAgentCommandSender(&agentCommandSenderAdapter{manager: agentManager, auditRepo: auditRepo})
				clusterService.SetConfigAgentClient(&configAgentClientAdapter{
					manager:     agentManager,
					hostService: hostService,
				})
				log.Println("[API] Agent command sender injected into cluster service / Agent 命令发送器已注入集群服务")
			}

			clusterHandler := cluster.NewHandler(clusterService, auditRepo)

			clusterRouter := apiV1Router.Group("/clusters")
			clusterRouter.Use(auth.LoginRequired())
			{
				// Cluster CRUD 集群增删改查
				clusterRouter.POST("", clusterHandler.CreateCluster)
				clusterRouter.GET("", clusterHandler.ListClusters)
				clusterRouter.GET("/:id", clusterHandler.GetCluster)
				clusterRouter.PUT("/:id", clusterHandler.UpdateCluster)
				clusterRouter.DELETE("/:id", clusterHandler.DeleteCluster)

				// Node management 节点管理
				clusterRouter.POST("/:id/nodes", clusterHandler.AddNode)
				clusterRouter.POST("/:id/nodes/batch", clusterHandler.AddNodes)
				clusterRouter.GET("/:id/nodes", clusterHandler.GetNodes)
				clusterRouter.PUT("/:id/nodes/:nodeId", clusterHandler.UpdateNode)
				clusterRouter.DELETE("/:id/nodes/:nodeId", clusterHandler.RemoveNode)
				clusterRouter.POST("/:id/nodes/precheck", clusterHandler.PrecheckNode)

				// Node operations 节点操作
				clusterRouter.POST("/:id/nodes/:nodeId/start", clusterHandler.StartNode)
				clusterRouter.POST("/:id/nodes/:nodeId/stop", clusterHandler.StopNode)
				clusterRouter.POST("/:id/nodes/:nodeId/restart", clusterHandler.RestartNode)
				clusterRouter.GET("/:id/nodes/:nodeId/logs", clusterHandler.GetNodeLogs)

				// Cluster operations 集群操作
				clusterRouter.POST("/:id/start", clusterHandler.StartCluster)
				clusterRouter.POST("/:id/stop", clusterHandler.StopCluster)
				clusterRouter.POST("/:id/restart", clusterHandler.RestartCluster)
				clusterRouter.GET("/:id/status", clusterHandler.GetClusterStatus)
				clusterRouter.GET("/:id/stx-java-proxy/status", clusterHandler.GetSTXJavaProxyStatus)
				clusterRouter.GET("/:id/stx-java-proxy/logs", clusterHandler.PreviewSTXJavaProxyServiceLog)
				clusterRouter.POST("/:id/stx-java-proxy/start", clusterHandler.StartSTXJavaProxy)
				clusterRouter.POST("/:id/stx-java-proxy/stop", clusterHandler.StopSTXJavaProxy)
				clusterRouter.POST("/:id/stx-java-proxy/restart", clusterHandler.RestartSTXJavaProxy)
				clusterRouter.GET("/:id/runtime-storage", clusterHandler.GetRuntimeStorage)
				clusterRouter.POST("/:id/runtime-storage/:kind/validate", clusterHandler.ValidateRuntimeStorage)
				clusterRouter.POST("/:id/runtime-storage/:kind/apply", clusterHandler.ApplyRuntimeStorage)
				clusterRouter.POST("/:id/log-mode", clusterHandler.SwitchJobLogMode)
				clusterRouter.POST("/:id/runtime-storage/:kind/list", clusterHandler.ListRuntimeStorage)
				clusterRouter.POST("/:id/runtime-storage/:kind/preview", clusterHandler.PreviewRuntimeStorage)
				clusterRouter.POST("/:id/runtime-storage/checkpoint/inspect", clusterHandler.InspectCheckpointRuntimeStorage)
				clusterRouter.POST("/:id/runtime-storage/imap/inspect", clusterHandler.InspectIMAPRuntimeStorage)
				clusterRouter.POST("/:id/runtime-storage/imap/cleanup", clusterHandler.CleanupIMAPStorage)
				clusterRouter.Any("/:id/webui", clusterHandler.ProxyWebUI)
				clusterRouter.Any("/:id/webui/*proxyPath", clusterHandler.ProxyWebUI)
			}

			// Monitor 监控配置管理
			// Initialize monitor service and handler
			// 初始化监控服务和处理器
			// Requirements: 5.1, 5.2, 6.4 - Monitor config and process events API
			monitorRepo := monitor.NewRepository(db.DB(context.Background()))
			monitorService := monitor.NewService(monitorRepo)

			// Inject node provider and config sender if agent manager is available
			// 如果 Agent Manager 可用，注入节点提供者和配置发送器
			if agentManager != nil {
				monitorService.SetNodeProvider(&monitorClusterNodeProviderAdapter{
					clusterService: clusterService,
					hostService:    hostService,
				})
				monitorService.SetConfigSender(&monitorAgentConfigSenderAdapter{
					manager: agentManager,
				})
				log.Println("[API] Node provider and config sender injected into monitor service / 节点提供者和配置发送器已注入监控服务")
			}

			// Delete cluster 前：先向各 Agent 推送关闭监控配置（停止监控且不再重启进程），再清理 DB 中的监控配置与事件；cluster.Service 已向各节点 Agent 发送 stop 命令
			clusterService.SetOnBeforeClusterDelete(func(ctx context.Context, clusterID uint) {
				monitorService.PushDisableConfigForCluster(ctx, clusterID)
				_ = monitorService.DeleteConfig(ctx, clusterID)
				_ = monitorService.DeleteClusterEvents(ctx, clusterID)
			})

			monitorHandler := monitor.NewHandler(monitorService)

			// Monitor config routes on clusters 集群监控配置路由
			clusterRouter.GET("/:id/monitor-config", monitorHandler.GetMonitorConfig)
			clusterRouter.PUT("/:id/monitor-config", monitorHandler.UpdateMonitorConfig)
			clusterRouter.GET("/:id/events", monitorHandler.ListProcessEvents)
			clusterRouter.GET("/:id/events/stats", monitorHandler.GetEventStats)

			// Monitoring center 监控中心
			monitoringRepo := monitoringapp.NewRepository(db.DB(context.Background()))
			monitoringService := monitoringapp.NewService(clusterService, monitorService, monitoringRepo)
			if err := monitoringService.SyncManagedAlertingArtifacts(ctx); err != nil {
				log.Printf("[Monitoring] sync managed alerting artifacts failed: %v", err)
			}
			monitorService.SetOnEventRecorded(monitoringService.DispatchAlertPolicyEvent)
			monitoringService.StartNodeHealthEvaluator(ctx)
			clusterHandler.SetOnOperationExecuted(func(ctx context.Context, event *cluster.OperationEvent) error {
				if event == nil {
					return nil
				}
				var processEventType monitor.ProcessEventType
				switch {
				case event.NodeID > 0 && event.Operation == cluster.OperationStop:
					processEventType = monitor.EventTypeNodeStopRequested
				case event.NodeID > 0 && event.Operation == cluster.OperationRestart:
					processEventType = monitor.EventTypeNodeRestartRequested
				case event.Operation == cluster.OperationRestart:
					processEventType = monitor.EventTypeClusterRestartRequested
				default:
					return nil
				}
				details := map[string]string{
					"trigger":   event.Trigger,
					"operator":  event.Operator,
					"success":   fmt.Sprintf("%t", event.Success),
					"message":   event.Message,
					"scope":     "cluster",
					"operation": string(event.Operation),
				}
				if strings.TrimSpace(event.ClusterName) != "" {
					details["cluster_name"] = strings.TrimSpace(event.ClusterName)
				}
				if event.NodeID > 0 {
					details["scope"] = "node"
					details["node_id"] = strconv.FormatUint(uint64(event.NodeID), 10)
				}
				if event.HostID > 0 {
					details["host_id"] = strconv.FormatUint(uint64(event.HostID), 10)
				}
				if strings.TrimSpace(event.HostName) != "" {
					details["host_name"] = strings.TrimSpace(event.HostName)
				}
				if strings.TrimSpace(event.HostIP) != "" {
					details["host_ip"] = strings.TrimSpace(event.HostIP)
				}
				if strings.TrimSpace(event.Role) != "" {
					details["role"] = strings.TrimSpace(event.Role)
				}
				detailsJSON, _ := json.Marshal(details)
				processName := strings.TrimSpace(event.ClusterName)
				if event.NodeID > 0 {
					processName = strings.TrimSpace(event.HostName)
				}
				if processName == "" && event.NodeID > 0 {
					processName = fmt.Sprintf("node %d", event.NodeID)
				}
				if processName == "" {
					processName = "cluster restart"
				}
				return monitorService.RecordEvent(ctx, &monitor.ProcessEvent{
					ClusterID:   event.ClusterID,
					NodeID:      event.NodeID,
					HostID:      event.HostID,
					EventType:   processEventType,
					ProcessName: processName,
					Role:        strings.TrimSpace(event.Role),
					Details:     string(detailsJSON),
				})
			})
			monitoringHandler := monitoringapp.NewHandler(monitoringService)
			diagnosticsService := diagnostics.NewService(clusterService, monitorService, monitoringService)
			diagnosticsService.SetHostReader(hostService)
			diagnosticsService.SetAgentCommandSender(&agentCommandSenderAdapter{manager: agentManager, auditRepo: auditRepo})
			diagnosticsService.SetExecutionService(executionService)
			executionProviders.Register("diagnostics", diagnosticsService)
			diagnosticsService.StartAutoPolicyRuntime(ctx)
			diagnosticsHandler := diagnostics.NewHandler(diagnosticsService)
			diagnosticsHandler.SetAuditRepository(auditRepo)

			// Public remote-observability integration endpoints (no login required).
			// 远程可观测集成公开接口（无需登录）。
			if config.Config.Observability.Enabled {
				promDiscoveryPath := normalizeAPIV1RoutePath(config.Config.Observability.Prometheus.HTTPSDPath, "/monitoring/prometheus/discovery")
				alertWebhookPath := normalizeAPIV1RoutePath(config.Config.Observability.Alertmanager.WebhookPath, "/monitoring/alertmanager/webhook")
				apiV1Router.GET(promDiscoveryPath, monitoringHandler.GetPrometheusDiscovery)
				apiV1Router.POST(alertWebhookPath, monitoringHandler.AlertmanagerWebhook)
			}

			monitoringRouter := apiV1Router.Group("/monitoring")
			monitoringRouter.Use(auth.LoginRequired())
			{
				monitoringRouter.GET("/overview", monitoringHandler.GetOverview)
				monitoringRouter.GET("/clusters/:id/overview", monitoringHandler.GetClusterOverview)
				monitoringRouter.GET("/alert-policies", monitoringHandler.ListAlertPolicies)
				monitoringRouter.GET("/alert-policies/:id/executions", monitoringHandler.ListAlertPolicyExecutions)
				monitoringRouter.POST("/alert-policies", monitoringHandler.CreateAlertPolicy)
				monitoringRouter.PUT("/alert-policies/:id", monitoringHandler.UpdateAlertPolicy)
				monitoringRouter.DELETE("/alert-policies/:id", monitoringHandler.DeleteAlertPolicy)
				monitoringRouter.GET("/alert-instances", monitoringHandler.ListAlertInstances)
				monitoringRouter.GET("/alerts", monitoringHandler.ListAlerts)
				monitoringRouter.GET("/remote-alerts", monitoringHandler.ListRemoteAlerts)
				monitoringRouter.POST("/alert-instances/:id/ack", monitoringHandler.AcknowledgeAlertInstance)
				monitoringRouter.POST("/alert-instances/:id/silence", monitoringHandler.SilenceAlertInstance)
				monitoringRouter.POST("/alert-instances/:id/close", monitoringHandler.CloseAlertInstance)
				monitoringRouter.POST("/alerts/:eventId/ack", monitoringHandler.AcknowledgeAlert)
				monitoringRouter.POST("/alerts/:eventId/silence", monitoringHandler.SilenceAlert)
				monitoringRouter.GET("/clusters/:id/rules", monitoringHandler.ListClusterRules)
				monitoringRouter.PUT("/clusters/:id/rules/:ruleId", monitoringHandler.UpdateClusterRule)
				monitoringRouter.GET("/integration/status", monitoringHandler.GetIntegrationStatus)
				monitoringRouter.GET("/alert-policies/bootstrap", monitoringHandler.GetAlertPolicyCenterBootstrap)
				monitoringRouter.GET("/notifiable-users", monitoringHandler.ListNotifiableUsers)
				monitoringRouter.GET("/platform-health", monitoringHandler.GetPlatformHealth)
				monitoringRouter.GET("/notification-channels", monitoringHandler.ListNotificationChannels)
				monitoringRouter.POST("/notification-channels", monitoringHandler.CreateNotificationChannel)
				monitoringRouter.PUT("/notification-channels/:id", monitoringHandler.UpdateNotificationChannel)
				monitoringRouter.DELETE("/notification-channels/:id", monitoringHandler.DeleteNotificationChannel)
				monitoringRouter.POST("/notification-channels/test", monitoringHandler.TestNotificationChannelDraft)
				monitoringRouter.POST("/notification-channels/test-connection", monitoringHandler.TestNotificationChannelConnection)
				monitoringRouter.POST("/notification-channels/:id/test", monitoringHandler.TestNotificationChannel)
				monitoringRouter.GET("/notification-deliveries", monitoringHandler.ListNotificationDeliveries)
				monitoringRouter.GET("/notification-routes", monitoringHandler.ListNotificationRoutes)
				monitoringRouter.POST("/notification-routes", monitoringHandler.CreateNotificationRoute)
				monitoringRouter.PUT("/notification-routes/:id", monitoringHandler.UpdateNotificationRoute)
				monitoringRouter.DELETE("/notification-routes/:id", monitoringHandler.DeleteNotificationRoute)
			}

			diagnosticsRouter := apiV1Router.Group("/diagnostics")
			diagnosticsRouter.Use(auth.LoginRequired())
			{
				diagnosticsRouter.GET("/bootstrap", diagnosticsHandler.GetWorkspaceBootstrap)
				diagnosticsRouter.GET("/resources", diagnosticsHandler.ListDiagnosticResources)
				diagnosticsRouter.GET("/resources/:code", diagnosticsHandler.GetDiagnosticResource)
				diagnosticsRouter.POST("/resources/:code/run", diagnosticsHandler.RunDiagnosticResource)
				diagnosticsRouter.POST("/inspections", diagnosticsHandler.StartInspection)
				diagnosticsRouter.GET("/inspections", diagnosticsHandler.ListInspectionReports)
				diagnosticsRouter.GET("/inspections/:id", diagnosticsHandler.GetInspectionReportDetail)
				diagnosticsRouter.POST("/tasks", diagnosticsHandler.CreateDiagnosticTask)
				diagnosticsRouter.GET("/tasks", diagnosticsHandler.ListDiagnosticTasks)
				diagnosticsRouter.GET("/tasks/:id", diagnosticsHandler.GetDiagnosticTask)
				diagnosticsRouter.POST("/tasks/:id/start", diagnosticsHandler.StartDiagnosticTask)
				diagnosticsRouter.GET("/tasks/:id/steps", diagnosticsHandler.ListDiagnosticTaskSteps)
				diagnosticsRouter.GET("/tasks/:id/artifacts", diagnosticsHandler.ListDiagnosticTaskArtifacts)
				diagnosticsRouter.GET("/tasks/:id/logs", diagnosticsHandler.ListDiagnosticTaskLogs)
				diagnosticsRouter.GET("/tasks/:id/events/stream", diagnosticsHandler.StreamDiagnosticTaskEvents)
				diagnosticsRouter.GET("/tasks/:id/html", diagnosticsHandler.PreviewDiagnosticTaskHTML)
				diagnosticsRouter.GET("/tasks/:id/files/*path", diagnosticsHandler.PreviewDiagnosticTaskFile)
				diagnosticsRouter.GET("/tasks/:id/bundle", diagnosticsHandler.DownloadDiagnosticTaskBundle)
				diagnosticsRouter.GET("/errors/groups", diagnosticsHandler.ListSeatunnelErrorGroups)
				diagnosticsRouter.GET("/errors/events", diagnosticsHandler.ListSeatunnelErrorEvents)
				diagnosticsRouter.GET("/errors/groups/:id", diagnosticsHandler.GetSeatunnelErrorGroupDetail)
				diagnosticsRouter.GET("/auto-policies/templates", diagnosticsHandler.ListBuiltinConditionTemplates)
				diagnosticsRouter.GET("/auto-policies", diagnosticsHandler.ListAutoPolicies)
				diagnosticsRouter.POST("/auto-policies", diagnosticsHandler.CreateAutoPolicy)
				diagnosticsRouter.GET("/auto-policies/:id", diagnosticsHandler.GetAutoPolicy)
				diagnosticsRouter.PUT("/auto-policies/:id", diagnosticsHandler.UpdateAutoPolicy)
				diagnosticsRouter.DELETE("/auto-policies/:id", diagnosticsHandler.DeleteAutoPolicy)
				diagnosticsRouter.GET("/troubleshooting-memories", diagnosticsHandler.ListTroubleshootingMemories)
				diagnosticsRouter.POST("/troubleshooting-memories", diagnosticsHandler.CreateTroubleshootingMemory)
				diagnosticsRouter.GET("/troubleshooting-memories/:id", diagnosticsHandler.GetTroubleshootingMemory)
				diagnosticsRouter.PUT("/troubleshooting-memories/:id", diagnosticsHandler.UpdateTroubleshootingMemory)
				diagnosticsRouter.DELETE("/troubleshooting-memories/:id", diagnosticsHandler.DeleteTroubleshootingMemory)
			}

			// Platform cluster health summary (powered by monitoring remote integration).
			// 平台集群健康摘要（由监控远程集成能力提供）。
			clusterRouter.GET("/health", monitoringHandler.GetClustersHealth)

			// Grafana 代理是高频请求路径，使用轻量会话校验降低每请求数据库开销。
			monitoringGrafanaProxyRouter := apiV1Router.Group("/monitoring/proxy/grafana")
			monitoringGrafanaProxyRouter.Use(auth.LoginRequiredSessionOnly())
			{
				monitoringGrafanaProxyRouter.Any("", monitoringHandler.ProxyGrafana)
				monitoringGrafanaProxyRouter.Any("/*proxyPath", monitoringHandler.ProxyGrafana)
			}

			// Discovery 集群发现
			// Initialize discovery service and handler
			// 初始化发现服务和处理器
			// Requirements: 1.2, 1.9, 9.3, 9.4 - Cluster discovery API
			discoveryService := discovery.NewService()
			discoveryService.SetHostProvider(&discoveryHostProviderAdapter{hostService: hostService})
			// Inject agent discoverer if agent manager is available
			// 如果 Agent Manager 可用，注入 Agent 发现器
			if agentManager != nil {
				discoveryService.SetAgentDiscoverer(&discoveryAgentDiscovererAdapter{manager: agentManager})
				log.Println("[API] Agent discoverer injected into discovery service / Agent 发现器已注入发现服务")
			}
			discoveryHandler := discovery.NewHandler(discoveryService)

			// Discovery routes on hosts 主机发现路由
			hostRouter.POST("/:id/discover", discoveryHandler.TriggerDiscovery)
			hostRouter.POST("/:id/discover/confirm", discoveryHandler.ConfirmDiscovery)
			hostRouter.POST("/:id/discover-processes", discoveryHandler.DiscoverProcesses)

			// Agent 分发 API（无需认证，供目标主机下载安装）
			// Agent distribution API (no authentication required, for target hosts to download and install)
			agentHandler := agent.NewHandler(&agent.HandlerConfig{
				ControlPlaneAddr:  config.GetExternalURL(),
				AgentBinaryDir:    "./lib/agent",
				GRPCPort:          fmt.Sprintf("%d", config.GetGRPCPort()),
				HeartbeatInterval: config.Config.GRPC.HeartbeatInterval,
				TLSEnabled:        config.Config.GRPC.TLSEnabled,
				CAFile:            tlsbootstrap.AgentCAPath(),
			})

			agentRouter := apiV1Router.Group("/agent")
			{
				// GET /api/v1/agent/install.sh - 获取安装脚本
				// GET /api/v1/agent/install.sh - Get install script
				agentRouter.GET("/install.sh", agentHandler.GetInstallScript)

				// GET /api/v1/agent/uninstall.sh - 获取卸载脚本
				// GET /api/v1/agent/uninstall.sh - Get uninstall script
				agentRouter.GET("/uninstall.sh", agentHandler.GetUninstallScript)

				// GET /api/v1/agent/ca.crt - 下载 gRPC TLS CA（供 Agent 安装流使用）
				// GET /api/v1/agent/ca.crt - Download gRPC TLS CA for Agent install flow
				agentRouter.GET("/ca.crt", agentHandler.DownloadCA)

				// GET /api/v1/agent/download - 下载 Agent 二进制文件
				// GET /api/v1/agent/download - Download Agent binary
				agentRouter.GET("/download", agentHandler.DownloadAgent)

				// GET /api/v1/agent/assets/stx-java-proxy.jar - 下载 stx-java-proxy 薄 jar
				// GET /api/v1/agent/assets/stx-java-proxy.jar - Download stx-java-proxy thin jar
				agentRouter.GET("/assets/stx-java-proxy.jar", agentHandler.DownloadSTXJavaProxyJar)

				// GET /api/v1/agent/assets/stx-java-proxy.sh - 下载 stx-java-proxy 启动脚本
				// GET /api/v1/agent/assets/stx-java-proxy.sh - Download stx-java-proxy launcher script
				agentRouter.GET("/assets/stx-java-proxy.sh", agentHandler.DownloadSTXJavaProxyScript)
			}

			// STX 离线发布包分发 API（无需认证，供客户机器一键下载安装控制面）。
			// STX offline release bundle distribution API (no authentication required for one-click control-plane install).
			releaseBundleHandler := releasebundle.NewHandler(&releasebundle.HandlerConfig{
				ReleaseDir:    "./dist/releases",
				BundlePattern: releasebundle.DefaultBundlePattern,
			})
			releaseBundleRouter := apiV1Router.Group("/stx")
			{
				// GET /api/v1/stx/install.sh - 获取控制面一键安装脚本
				// GET /api/v1/stx/install.sh - Get control-plane one-click install script
				releaseBundleRouter.GET("/install.sh", releaseBundleHandler.GetInstallScript)

				// GET /api/v1/stx/download - 下载最新的 CentOS 7 兼容离线包
				// GET /api/v1/stx/download - Download the latest CentOS 7 compatible offline bundle
				releaseBundleRouter.GET("/download", releaseBundleHandler.DownloadBundle)
			}

			// Audit 审计日志 API
			// Audit log API
			// Initialize audit handler (auditRepo already created above)
			// 初始化审计处理器（auditRepo 已在上面创建）
			auditHandler := audit.NewHandler(auditRepo)

			// Command logs 命令日志
			commandRouter := apiV1Router.Group("/commands")
			commandRouter.Use(auth.LoginRequired())
			{
				// GET /api/v1/commands - 获取命令日志列表
				// GET /api/v1/commands - Get command logs list
				commandRouter.GET("", auditHandler.ListCommandLogs)

				// GET /api/v1/commands/:id - 获取命令日志详情
				// GET /api/v1/commands/:id - Get command log details
				commandRouter.GET("/:id", auditHandler.GetCommandLog)
			}

			// Audit logs 审计日志
			auditLogRouter := apiV1Router.Group("/audit-logs")
			auditLogRouter.Use(auth.LoginRequired())
			{
				// GET /api/v1/audit-logs - 获取审计日志列表
				// GET /api/v1/audit-logs - Get audit logs list
				auditLogRouter.GET("", auditHandler.ListAuditLogs)

				// GET /api/v1/audit-logs/:id - 获取审计日志详情
				// GET /api/v1/audit-logs/:id - Get audit log details
				auditLogRouter.GET("/:id", auditHandler.GetAuditLog)
			}

			// Installer SeaTunnel 安装管理
			// Initialize installer service and handler
			// 初始化安装服务和处理器
			// Follow configured packages_dir instead of a hard-coded repo path,
			// so E2E / tests can preload packages into their isolated storage.
			// 使用配置中的 packages_dir，而不是硬编码仓库路径，
			// 这样 E2E / 测试才能在各自隔离目录中预热安装包。
			installerService := installer.NewService("", nil)
			installerService.SetExecutionService(executionService)
			executionProviders.Register("installer", installerService)
			// Set host provider for precheck operations
			// 设置用于预检查操作的主机提供者
			installerService.SetHostProvider(&hostProviderAdapter{hostService: hostService})
			installerService.SetNodeJVMResolver(clusterService)
			// Inject agent manager if available
			// 如果 Agent Manager 可用，注入
			if agentManager != nil {
				installerService.SetAgentManager(&installerAgentManagerAdapter{
					manager:     agentManager,
					hostService: hostService,
				})
			}
			installerHandler := installer.NewHandler(installerService)

			// Package management routes 安装包管理路由
			packageRouter := apiV1Router.Group("/packages")
			packageRouter.Use(auth.LoginRequired())
			{
				// GET /api/v1/packages - 获取可用安装包列表
				// GET /api/v1/packages - List available packages
				packageRouter.GET("", installerHandler.ListPackages)

				// POST /api/v1/packages/versions/refresh - 刷新版本列表
				// POST /api/v1/packages/versions/refresh - Refresh version list
				packageRouter.POST("/versions/refresh", installerHandler.RefreshVersions)

				// GET /api/v1/packages/:version - 获取安装包信息
				// GET /api/v1/packages/:version - Get package info
				packageRouter.GET("/:version", installerHandler.GetPackageInfo)

				// POST /api/v1/packages/upload - 上传安装包
				// POST /api/v1/packages/upload - Upload package
				packageRouter.POST("/upload", installerHandler.UploadPackage)

				// POST /api/v1/packages/upload/chunk - 分片上传安装包
				// POST /api/v1/packages/upload/chunk - Upload package chunk
				packageRouter.POST("/upload/chunk", installerHandler.UploadPackageChunk)

				// POST /api/v1/packages/:version/source/upload - 单独上传或替换源码包
				// POST /api/v1/packages/:version/source/upload - Upload or replace source archive
				packageRouter.POST("/:version/source/upload", installerHandler.UploadSourcePackage)

				// POST /api/v1/packages/:version/source/fetch - 从镜像补充源码包
				// POST /api/v1/packages/:version/source/fetch - Fetch source archive from mirror
				packageRouter.POST("/:version/source/fetch", installerHandler.FetchSourcePackage)

				// GET /api/v1/packages/:version/source/download - 下载本地源码包
				// GET /api/v1/packages/:version/source/download - Download local source archive
				packageRouter.GET("/:version/source/download", installerHandler.DownloadSourcePackage)

				// DELETE /api/v1/packages/:version - 删除本地安装包
				// DELETE /api/v1/packages/:version - Delete local package
				packageRouter.DELETE("/:version", installerHandler.DeletePackage)

				// POST /api/v1/packages/download - 开始下载安装包到服务器
				// POST /api/v1/packages/download - Start downloading package to server
				packageRouter.POST("/download", installerHandler.StartDownload)

				// GET /api/v1/packages/downloads - 获取所有下载任务
				// GET /api/v1/packages/downloads - List all download tasks
				packageRouter.GET("/downloads", installerHandler.ListDownloads)

				// GET /api/v1/packages/download/:version - 获取下载状态
				// GET /api/v1/packages/download/:version - Get download status
				packageRouter.GET("/download/:version", installerHandler.GetDownloadStatus)

				// POST /api/v1/packages/download/:version/cancel - 取消下载
				// POST /api/v1/packages/download/:version/cancel - Cancel download
				packageRouter.POST("/download/:version/cancel", installerHandler.CancelDownload)
			}

			// Task 任务管理
			// Initialize task manager and handler
			// 初始化任务管理器和处理器
			taskManager := task.NewManager()
			taskHandler := task.NewHandler(taskManager)

			// Task management routes 任务管理路由
			taskRouter := apiV1Router.Group("/tasks")
			taskRouter.Use(auth.LoginRequired())
			{
				// POST /api/v1/tasks - 创建任务
				// POST /api/v1/tasks - Create task
				taskRouter.POST("", taskHandler.CreateTask)

				// GET /api/v1/tasks - 获取任务列表
				// GET /api/v1/tasks - List tasks
				taskRouter.GET("", taskHandler.ListTasks)

				// GET /api/v1/tasks/:id - 获取任务详情
				// GET /api/v1/tasks/:id - Get task details
				taskRouter.GET("/:id", taskHandler.GetTask)

				// POST /api/v1/tasks/:id/start - 开始执行任务
				// POST /api/v1/tasks/:id/start - Start task
				taskRouter.POST("/:id/start", taskHandler.StartTask)

				// POST /api/v1/tasks/:id/cancel - 取消任务
				// POST /api/v1/tasks/:id/cancel - Cancel task
				taskRouter.POST("/:id/cancel", taskHandler.CancelTask)

				// POST /api/v1/tasks/:id/retry - 重试任务
				// POST /api/v1/tasks/:id/retry - Retry task
				taskRouter.POST("/:id/retry", taskHandler.RetryTask)
			}

			// Sync studio 数据同步工作台
			syncRepo := syncapp.NewRepository(db.DB(context.Background()))
			syncService := syncapp.NewService(syncRepo)
			syncService.SetExecutionService(executionService)
			executionProviders.Register("sync", syncService)
			syncService.SetEngineClient(syncapp.NewSeaTunnelEngineClient())
			syncService.SetRuntimeResolver(syncapp.NewDefaultClusterRuntimeResolver(clusterRepo, hostRepo))
			syncService.SetExecutionTargetResolver(syncapp.NewDefaultExecutionTargetResolver(clusterRepo, hostRepo))
			syncService.SetClusterLogProvider(clusterService)
			syncService.SetClusterVersionProvider(clusterService)
			syncService.SetConfigToolClient(syncapp.NewDefaultConfigToolClient())
			syncService.SetConfigToolResolver(syncapp.NewDefaultConfigToolResolver(clusterService))
			if agentManager != nil {
				syncService.SetAgentCommandSender(&agentCommandSenderAdapter{manager: agentManager, auditRepo: auditRepo})
			}
			syncService.StartPreviewRuntime(ctx)
			syncService.StartTaskScheduleRuntime(ctx)
			syncHandler := syncapp.NewHandler(syncService)

			apiV1Router.POST("/sync/preview/collect", syncHandler.CollectPreview)

			syncRouter := apiV1Router.Group("/sync")
			syncRouter.Use(auth.LoginRequired())
			{
				syncRouter.GET("/tree", syncHandler.GetTaskTree)

				syncTaskRouter := syncRouter.Group("/tasks")
				{
					syncTaskRouter.POST("", syncHandler.CreateTask)
					syncTaskRouter.GET("", syncHandler.ListTasks)
					syncTaskRouter.GET("/:id", syncHandler.GetTask)
					syncTaskRouter.PUT("/:id", syncHandler.UpdateTask)
					syncTaskRouter.DELETE("/:id", syncHandler.DeleteTask)
					syncTaskRouter.POST("/:id/publish", syncHandler.PublishTask)
					syncTaskRouter.GET("/:id/versions", syncHandler.ListTaskVersions)
					syncTaskRouter.POST("/:id/versions/:versionId/rollback", syncHandler.RollbackTaskVersion)
					syncTaskRouter.DELETE("/:id/versions/:versionId", syncHandler.DeleteTaskVersion)
					syncTaskRouter.POST("/:id/validate", syncHandler.ValidateTask)
					syncTaskRouter.POST("/:id/test-connections", syncHandler.TestTaskConnections)
					syncTaskRouter.POST("/:id/dag", syncHandler.GetTaskDAG)
					syncTaskRouter.POST("/:id/preview", syncHandler.PreviewTask)
					syncTaskRouter.POST("/:id/preview/sink-savemode", syncHandler.PreviewSinkSaveMode)
					syncTaskRouter.POST("/:id/submit", syncHandler.SubmitTask)
				}

				syncGlobalVarRouter := syncRouter.Group("/global-variables")
				{
					syncGlobalVarRouter.GET("", syncHandler.ListGlobalVariables)
					syncGlobalVarRouter.POST("", syncHandler.CreateGlobalVariable)
					syncGlobalVarRouter.PUT("/:id", syncHandler.UpdateGlobalVariable)
					syncGlobalVarRouter.DELETE("/:id", syncHandler.DeleteGlobalVariable)
				}

				syncPluginRouter := syncRouter.Group("/plugins")
				{
					syncPluginRouter.POST("/list", syncHandler.ListPluginFactories)
					syncPluginRouter.POST("/options", syncHandler.GetPluginOptions)
					syncPluginRouter.POST("/template", syncHandler.RenderPluginTemplate)
					syncPluginRouter.POST("/enum-values", syncHandler.ListPluginEnumValues)
					syncPluginRouter.POST("/enum-catalog", syncHandler.ListPluginEnumCatalog)
				}

				syncJobRouter := syncRouter.Group("/jobs")
				{
					syncJobRouter.GET("", syncHandler.ListJobs)
					syncJobRouter.GET("/:id", syncHandler.GetJob)
					syncJobRouter.GET("/:id/preview", syncHandler.GetPreviewSnapshot)
					syncJobRouter.GET("/:id/checkpoint", syncHandler.GetJobCheckpointSnapshot)
					syncJobRouter.GET("/:id/logs", syncHandler.GetJobLogs)
					syncJobRouter.POST("/:id/recover", syncHandler.RecoverJob)
					syncJobRouter.POST("/:id/cancel", syncHandler.CancelJob)
				}
			}

			// Host tasks route 主机任务路由
			// GET /api/v1/hosts/:id/tasks - 获取主机任务列表
			// GET /api/v1/hosts/:id/tasks - List host tasks
			hostRouter.GET("/:id/tasks", taskHandler.ListHostTasks)

			// Plugin 插件市场管理
			// Initialize plugin repository, service and handler
			// 初始化插件仓库、服务和处理器
			pluginRepo := plugin.NewRepository(db.DB(context.Background()))
			pluginService := plugin.NewService(pluginRepo)
			// Inject cluster service for version validation
			// 注入集群服务用于版本校验
			pluginService.SetClusterGetter(clusterService)

			// Inject agent command sender for plugin installation to cluster nodes
			// 注入 Agent 命令发送器用于将插件安装到集群节点
			if agentManager != nil {
				pluginService.SetAgentCommandSender(&pluginAgentCommandSenderAdapter{manager: agentManager})
				pluginService.SetClusterNodeGetter(&clusterNodeGetterAdapter{clusterService: clusterService})
				pluginService.SetHostInfoGetter(&hostInfoGetterAdapter{hostService: hostService})
				log.Println("[API] Agent command sender injected into plugin service / Agent 命令发送器已注入插件服务")

				// Inject plugin transferer into installer service for plugin transfer during installation
				// 将插件传输器注入安装服务，用于安装过程中的插件传输
				installerService.SetPluginTransferer(pluginService)
				log.Println("[API] Plugin transferer injected into installer service / 插件传输器已注入安装服务")

				// Inject node status updater into installer service for updating node status after installation
				// 将节点状态更新器注入安装服务，用于安装后更新节点状态
				installerService.SetNodeStatusUpdater(clusterService)
				log.Println("[API] Node status updater injected into installer service / 节点状态更新器已注入安装服务")

				// Inject node starter into installer service for starting nodes after installation
				// 将节点启动器注入安装服务，用于安装后启动节点
				installerService.SetNodeStarter(clusterService)
				log.Println("[API] Node starter injected into installer service / 节点启动器已注入安装服务")
			}

			pluginHandler := plugin.NewHandler(pluginService, auditRepo)

			// Plugin marketplace routes 插件市场路由
			pluginRouter := apiV1Router.Group("/plugins")
			pluginRouter.Use(auth.LoginRequired())
			{
				// GET /api/v1/plugins - 获取可用插件列表
				// GET /api/v1/plugins - List available plugins
				pluginRouter.GET("", pluginHandler.ListAvailablePlugins)

				// POST /api/v1/plugins/refresh - 刷新连接器目录
				// POST /api/v1/plugins/refresh - Refresh connector catalog
				pluginRouter.POST("/refresh", pluginHandler.RefreshAvailablePlugins)

				// GET /api/v1/plugins/local - 获取已下载的本地插件列表
				// GET /api/v1/plugins/local - List locally downloaded plugins
				pluginRouter.GET("/local", pluginHandler.ListLocalPlugins)

				// GET /api/v1/plugins/downloads - 获取活动下载任务列表
				// GET /api/v1/plugins/downloads - List active download tasks
				pluginRouter.GET("/downloads", pluginHandler.ListActiveDownloads)

				// POST /api/v1/plugins/download-all - 一键下载所有插件
				// POST /api/v1/plugins/download-all - Download all plugins
				pluginRouter.POST("/download-all", pluginHandler.DownloadAllPlugins)

				// GET /api/v1/plugins/:name - 获取插件详情
				// GET /api/v1/plugins/:name - Get plugin info
				pluginRouter.GET("/:name", pluginHandler.GetPluginInfo)

				// POST /api/v1/plugins/:name/download - 下载插件到 Control Plane
				// POST /api/v1/plugins/:name/download - Download plugin to Control Plane
				pluginRouter.POST("/:name/download", pluginHandler.DownloadPlugin)

				// GET /api/v1/plugins/:name/download/status - 获取下载状态
				// GET /api/v1/plugins/:name/download/status - Get download status
				pluginRouter.GET("/:name/download/status", pluginHandler.GetDownloadStatus)

				// DELETE /api/v1/plugins/:name/local - 删除本地插件文件
				// DELETE /api/v1/plugins/:name/local - Delete local plugin file
				pluginRouter.DELETE("/:name/local", pluginHandler.DeleteLocalPlugin)

				// GET /api/v1/plugins/:name/dependencies - 获取插件依赖配置
				// GET /api/v1/plugins/:name/dependencies - Get plugin dependencies
				pluginRouter.GET("/:name/dependencies", pluginHandler.ListDependencies)

				// POST /api/v1/plugins/:name/dependencies/upload - 上传自定义依赖 Jar
				// POST /api/v1/plugins/:name/dependencies/upload - Upload a custom dependency Jar
				pluginRouter.POST("/:name/dependencies/upload", pluginHandler.UploadDependency)

				// POST /api/v1/plugins/:name/dependencies/disables - 禁用官方依赖
				// POST /api/v1/plugins/:name/dependencies/disables - Disable one official dependency
				pluginRouter.POST("/:name/dependencies/disables", pluginHandler.DisableDependency)

				// DELETE /api/v1/plugins/:name/dependencies/disables/:disableId - 重新启用官方依赖
				// DELETE /api/v1/plugins/:name/dependencies/disables/:disableId - Enable one disabled official dependency
				pluginRouter.DELETE("/:name/dependencies/disables/:disableId", pluginHandler.EnableDependency)

				// GET /api/v1/plugins/:name/official-dependencies - 获取官方依赖基线
				// GET /api/v1/plugins/:name/official-dependencies - Get official dependency profiles
				pluginRouter.GET("/:name/official-dependencies", pluginHandler.GetOfficialDependencies)

				// POST /api/v1/plugins/:name/official-dependencies/analyze - 在线分析官方依赖
				// POST /api/v1/plugins/:name/official-dependencies/analyze - Analyze official dependency profiles
				pluginRouter.POST("/:name/official-dependencies/analyze", pluginHandler.AnalyzeOfficialDependencies)

				// POST /api/v1/plugins/:name/dependencies - 添加插件依赖
				// POST /api/v1/plugins/:name/dependencies - Add plugin dependency
				pluginRouter.POST("/:name/dependencies", pluginHandler.AddDependency)

				// DELETE /api/v1/plugins/:name/dependencies/:depId - 删除插件依赖
				// DELETE /api/v1/plugins/:name/dependencies/:depId - Delete plugin dependency
				pluginRouter.DELETE("/:name/dependencies/:depId", pluginHandler.DeleteDependency)
			}

			// Cluster plugin routes 集群插件路由
			// GET /api/v1/clusters/:id/plugins - 获取集群已安装插件
			// GET /api/v1/clusters/:id/plugins - Get cluster installed plugins
			clusterRouter.GET("/:id/plugins", pluginHandler.ListInstalledPlugins)

			// POST /api/v1/clusters/:id/plugins - 安装插件到集群
			// POST /api/v1/clusters/:id/plugins - Install plugin to cluster
			clusterRouter.POST("/:id/plugins", pluginHandler.InstallPlugin)

			// DELETE /api/v1/clusters/:id/plugins/:name - 卸载插件
			// DELETE /api/v1/clusters/:id/plugins/:name - Uninstall plugin
			clusterRouter.DELETE("/:id/plugins/:name", pluginHandler.UninstallPlugin)

			// PUT /api/v1/clusters/:id/plugins/:name/enable - 启用插件
			// PUT /api/v1/clusters/:id/plugins/:name/enable - Enable plugin
			clusterRouter.PUT("/:id/plugins/:name/enable", pluginHandler.EnablePlugin)

			// PUT /api/v1/clusters/:id/plugins/:name/disable - 禁用插件
			// PUT /api/v1/clusters/:id/plugins/:name/disable - Disable plugin
			clusterRouter.PUT("/:id/plugins/:name/disable", pluginHandler.DisablePlugin)

			// GET /api/v1/clusters/:id/plugins/:name/progress - 获取插件安装进度
			// GET /api/v1/clusters/:id/plugins/:name/progress - Get plugin installation progress
			clusterRouter.GET("/:id/plugins/:name/progress", pluginHandler.GetInstallProgress)

			// Config 配置文件管理
			// Initialize config repository, service and handler
			// 初始化配置仓库、服务和处理器
			configRepo := appconfig.NewRepository(db.DB(context.Background()))
			configAgentClient := &configAgentClientAdapter{manager: agentManager, hostService: hostService}
			configNodeInfoProvider := &configNodeInfoProviderAdapter{clusterService: clusterService}
			configService := appconfig.NewService(configRepo, &configHostProviderAdapter{hostService: hostService}, configNodeInfoProvider, configAgentClient)
			configService.SetPortMetadataUpdater(&configPortMetadataUpdaterAdapter{clusterRepo: clusterRepo})
			clusterService.SetRuntimeConfigStore(configService)
			configHandler := appconfig.NewHandler(configService)

			// Inject config initializer into installer service for initializing configs after installation
			// 将配置初始化器注入安装服务，用于安装后初始化配置
			installerService.SetConfigInitializer(configService)
			log.Println("[API] Config initializer injected into installer service / 配置初始化器已注入安装服务")

			// Config management routes 配置管理路由
			appconfig.RegisterRoutes(apiV1Router, configHandler)

			// SeaTunnel upgrade routes / SeaTunnel 升级路由
			stUpgradeRepo := stupgrade.NewRepository(db.DB(context.Background()))
			stUpgradeService := stupgrade.NewService(stUpgradeRepo)
			stUpgradeService.SetExecutionService(executionService)
			executionProviders.Register("stupgrade", stUpgradeService)
			stUpgradeService.SetClusterProvider(clusterService)
			stUpgradeService.SetHostProvider(hostService)
			stUpgradeService.SetPackageProvider(installerService)
			stUpgradeService.SetPluginProvider(pluginService)
			stUpgradeService.SetConfigProvider(configService)
			stUpgradeService.SetClusterOperator(clusterService)
			stUpgradeService.SetPackageTransferer(installerService)
			if agentManager != nil {
				stUpgradeService.SetAgentCommandSender(&installerAgentManagerAdapter{
					manager:     agentManager,
					hostService: hostService,
				})
			}
			stUpgradeHandler := stupgrade.NewHandler(stUpgradeService)

			stUpgradeRouter := apiV1Router.Group("/st-upgrade")
			stUpgradeRouter.Use(auth.LoginRequired())
			{
				stUpgradeRouter.POST("/precheck", stUpgradeHandler.RunPrecheck)
				stUpgradeRouter.POST("/plan", stUpgradeHandler.CreatePlan)
				stUpgradeRouter.GET("/plans/:id", stUpgradeHandler.GetPlan)
				stUpgradeRouter.POST("/execute", stUpgradeHandler.ExecutePlan)
				stUpgradeRouter.GET("/tasks", stUpgradeHandler.ListTasks)
				stUpgradeRouter.GET("/tasks/:id", stUpgradeHandler.GetTask)
				stUpgradeRouter.GET("/tasks/:id/steps", stUpgradeHandler.ListTaskSteps)
				stUpgradeRouter.GET("/tasks/:id/logs", stUpgradeHandler.ListTaskLogs)
				stUpgradeRouter.GET("/tasks/:id/events/stream", stUpgradeHandler.StreamTaskEvents)
			}

			// Installation routes on hosts 主机安装路由
			// POST /api/v1/hosts/:id/precheck - 运行预检查
			// POST /api/v1/hosts/:id/precheck - Run precheck
			hostRouter.POST("/:id/precheck", installerHandler.RunPrecheck)
			apiV1Router.POST("/installer/runtime-storage/validate", auth.LoginRequired(), installerHandler.ValidateRuntimeStorage)

			// POST /api/v1/hosts/:id/install - 开始安装
			// POST /api/v1/hosts/:id/install - Start installation
			hostRouter.POST("/:id/install", installerHandler.StartInstallation)

			// GET /api/v1/hosts/:id/install/status - 获取安装状态
			// GET /api/v1/hosts/:id/install/status - Get installation status
			hostRouter.GET("/:id/install/status", installerHandler.GetInstallationStatus)

			// POST /api/v1/hosts/:id/install/retry - 重试失败步骤
			// POST /api/v1/hosts/:id/install/retry - Retry failed step
			hostRouter.POST("/:id/install/retry", installerHandler.RetryStep)

			// POST /api/v1/hosts/:id/install/cancel - 取消安装
			// POST /api/v1/hosts/:id/install/cancel - Cancel installation
			hostRouter.POST("/:id/install/cancel", installerHandler.CancelInstallation)

			// DeepWiki 文档服务
			// DeepWiki documentation service
			deepwikiService := deepwiki.NewService(deepwiki.ServiceConfig{
				UseMCP:  false, // 使用直接 HTTP 模式 / Use direct HTTP mode
				Timeout: 30 * time.Second,
			})
			deepwikiHandler := deepwiki.NewHandler(deepwikiService)

			deepwikiRouter := apiV1Router.Group("/deepwiki")
			deepwikiRouter.Use(auth.LoginRequired())
			{
				// GET /api/v1/deepwiki/docs - 获取 SeaTunnel 文档
				// GET /api/v1/deepwiki/docs - Get SeaTunnel documentation
				deepwikiRouter.GET("/docs", deepwikiHandler.GetDocs)

				// POST /api/v1/deepwiki/fetch - 获取指定仓库文档
				// POST /api/v1/deepwiki/fetch - Fetch documentation for specific repository
				deepwikiRouter.POST("/fetch", deepwikiHandler.FetchDocs)

				// POST /api/v1/deepwiki/search - 搜索文档
				// POST /api/v1/deepwiki/search - Search documentation
				deepwikiRouter.POST("/search", deepwikiHandler.Search)
			}
		}
	}

	// Serve HTTP API
	// 启动 HTTP API 服务
	log.Printf("[API] HTTP 服务器启动于 %s / HTTP server starting on %s\n", config.Config.App.Addr, config.Config.App.Addr)
	if err := r.Run(config.Config.App.Addr); err != nil {
		log.Fatalf("[API] serve api failed: %v\n", err)
	}
}

// initGRPCServer initializes and starts the gRPC server for Agent communication.
// initGRPCServer 初始化并启动用于 Agent 通信的 gRPC 服务器。
// Requirements: 1.1, 3.4 - Starts gRPC server and heartbeat timeout detection.
func initGRPCServer(ctx context.Context) (*grpcServer.Server, *agent.Manager) {
	grpcConfig := config.GetGRPCConfig()

	// 创建 logger
	// Create logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Printf("[gRPC] 创建 logger 失败: %v / Failed to create logger: %v\n", err, err)
		logger, _ = zap.NewDevelopment()
	}

	// 初始化 Agent Manager
	// Initialize Agent Manager
	// Requirements: 3.4 - Starts heartbeat timeout detection goroutine
	agentManager := agent.NewManager(&agent.ManagerConfig{
		HeartbeatInterval: time.Duration(grpcConfig.HeartbeatInterval) * time.Second,
		HeartbeatTimeout:  time.Duration(grpcConfig.HeartbeatTimeout) * time.Second,
		CheckInterval:     5 * time.Second,
	})

	// 初始化 Host Service 用于 Agent 状态更新
	// Initialize Host Service for Agent status updates
	hostRepo := host.NewRepository(db.DB(ctx))
	clusterRepo := cluster.NewRepository(db.DB(ctx))
	hostService := host.NewService(hostRepo, clusterRepo, &host.ServiceConfig{
		HeartbeatTimeout: time.Duration(grpcConfig.HeartbeatTimeout) * time.Second,
		ControlPlaneAddr: config.GetExternalURL(),
	})

	// 设置 Host 状态更新器
	// Set Host status updater
	agentManager.SetHostUpdater(&hostStatusUpdaterAdapter{hostService: hostService})

	// 初始化 Audit Repository 用于日志记录
	// Initialize Audit Repository for logging
	auditRepo := audit.NewRepository(db.DB(ctx))

	// 创建 gRPC 服务器配置
	// Create gRPC server configuration
	serverConfig := &grpcServer.ServerConfig{
		Port:              grpcConfig.Port,
		TLSEnabled:        grpcConfig.TLSEnabled,
		CertFile:          grpcConfig.CertFile,
		KeyFile:           grpcConfig.KeyFile,
		CAFile:            grpcConfig.CAFile,
		MaxRecvMsgSize:    grpcConfig.MaxRecvMsgSize * 1024 * 1024, // MB to bytes
		MaxSendMsgSize:    grpcConfig.MaxSendMsgSize * 1024 * 1024, // MB to bytes
		HeartbeatInterval: grpcConfig.HeartbeatInterval,
	}

	// 创建并启动 gRPC 服务器
	// Create and start gRPC server
	srv := grpcServer.NewServer(serverConfig, agentManager, hostService, auditRepo, logger)

	// Set cluster node provider for gRPC handlers (for monitor config push after agent registration)
	// 设置 gRPC 处理器的集群节点提供者（用于 Agent 注册后推送监控配置）
	clusterService := cluster.NewService(clusterRepo, hostService, &cluster.ServiceConfig{
		HeartbeatTimeout: time.Duration(grpcConfig.HeartbeatTimeout) * time.Second,
	})
	monitorRepo := monitor.NewRepository(db.DB(ctx))
	monitorService := monitor.NewService(monitorRepo)
	monitoringRepo := monitoringapp.NewRepository(db.DB(ctx))
	monitoringService := monitoringapp.NewService(clusterService, monitorService, monitoringRepo)
	if err := monitoringService.SyncManagedAlertingArtifacts(ctx); err != nil {
		log.Printf("[Monitoring] sync managed alerting artifacts failed: %v", err)
	}
	monitorService.SetOnEventRecorded(monitoringService.DispatchAlertPolicyEvent)
	grpcServer.SetClusterNodeProvider(&grpcClusterNodeProviderAdapter{
		clusterService: clusterService,
		monitorService: monitorService,
	})
	// Set monitor service for gRPC handlers (for recording process events)
	// 设置 gRPC 处理器的监控服务（用于记录进程事件）
	grpcServer.SetMonitorService(monitorService)
	diagnosticsService := diagnostics.NewService(clusterService, monitorService, monitoringService)
	diagnosticsService.SetHostReader(hostService)
	diagnosticsService.SetAgentCommandSender(&agentCommandSenderAdapter{manager: agentManager, auditRepo: auditRepo})
	grpcServer.SetDiagnosticsService(diagnosticsService)
	log.Println("[gRPC] Cluster node provider, monitor service and diagnostics service set for gRPC handlers / 已为 gRPC 处理器设置集群节点提供者、监控服务和诊断服务")

	if err := srv.Start(ctx); err != nil {
		log.Printf("[gRPC] 启动 gRPC 服务器失败: %v / Failed to start gRPC server: %v\n", err, err)
		return nil, nil
	}

	log.Printf("[gRPC] gRPC 服务器启动于端口 %d / gRPC server started on port %d\n", grpcConfig.Port, grpcConfig.Port)

	// 启动 Agent Manager 后台任务（心跳超时检测）
	// Start Agent Manager background tasks (heartbeat timeout detection)
	if err := agentManager.Start(ctx); err != nil {
		log.Printf("[gRPC] 启动 Agent Manager 失败: %v / Failed to start Agent Manager: %v\n", err, err)
	}

	return srv, agentManager
}

// hostStatusUpdaterAdapter adapts host.Service to agent.HostStatusUpdater interface.
// hostStatusUpdaterAdapter 将 host.Service 适配到 agent.HostStatusUpdater 接口。
type hostStatusUpdaterAdapter struct {
	hostService *host.Service
}

// UpdateAgentStatus updates the agent status for a host by pre-bound host ID or IP address.
// UpdateAgentStatus 根据预绑定主机 ID 或 IP 地址更新主机的 Agent 状态。
func (a *hostStatusUpdaterAdapter) UpdateAgentStatus(ctx context.Context, hostID uint, ipAddress string, agentID string, version string, systemInfo *agent.SystemInfo, hostname string, localIPs []string) (uint, error) {
	var sysInfo *host.SystemInfo
	if systemInfo != nil {
		sysInfo = &host.SystemInfo{
			OSType:      systemInfo.OSType,
			Arch:        systemInfo.Arch,
			CPUCores:    systemInfo.CPUCores,
			TotalMemory: systemInfo.TotalMemory,
			TotalDisk:   systemInfo.TotalDisk,
		}
	}

	h, err := a.hostService.UpdateAgentStatus(ctx, hostID, ipAddress, agentID, version, sysInfo, hostname, localIPs)
	if err != nil {
		return 0, err
	}
	return h.ID, nil
}

// UpdateHeartbeat updates the heartbeat data for a host.
// UpdateHeartbeat 更新主机的心跳数据。
func (a *hostStatusUpdaterAdapter) UpdateHeartbeat(ctx context.Context, agentID string, cpuUsage, memoryUsage, diskUsage float64) error {
	return a.hostService.UpdateHeartbeat(ctx, agentID, cpuUsage, memoryUsage, diskUsage)
}

// MarkHostOffline marks a host as offline by agent ID.
// MarkHostOffline 根据 Agent ID 将主机标记为离线。
func (a *hostStatusUpdaterAdapter) MarkHostOffline(ctx context.Context, agentID string) error {
	h, err := a.hostService.GetByAgentID(ctx, agentID)
	if err != nil {
		return err
	}
	return a.hostService.UpdateAgentStatusByID(ctx, h.ID, host.AgentStatusOffline, agentID, h.AgentVersion)
}

// agentCommandSenderAdapter adapts agent.Manager to cluster.AgentCommandSender interface.
// agentCommandSenderAdapter 将 agent.Manager 适配到 cluster.AgentCommandSender 接口。
type agentCommandSenderAdapter struct {
	manager   *agent.Manager
	auditRepo *audit.Repository
}

// SendCommand sends a command to an agent and returns the result.
// SendCommand 向 Agent 发送命令并返回结果。
func (a *agentCommandSenderAdapter) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	// Convert command type string to pb.CommandType
	// 将命令类型字符串转换为 pb.CommandType
	cmdType := a.stringToCommandType(commandType)

	// Add sub_command parameter for precheck commands
	// 为预检查命令添加 sub_command 参数
	if cmdType == pb.CommandType_PRECHECK && params["sub_command"] == "" {
		params["sub_command"] = commandType
	}

	timeout := 30 * time.Second
	switch commandType {
	case "get_logs", "thread_dump":
		timeout = 2 * time.Minute
	case "jvm_dump":
		timeout = 10 * time.Minute
	case "pull_config":
		timeout = 1 * time.Minute
	}

	// Send command with command-specific timeout
	// 使用命令级超时发送命令
	startedAt := time.Now()
	commandID := uuid.NewString()
	a.recordPendingCommandLog(ctx, commandID, agentID, commandType, params, startedAt)
	resp, err := a.manager.SendCommandWithID(ctx, commandID, agentID, cmdType, params, timeout)
	if err != nil {
		a.recordFailedCommandLog(ctx, commandID, err)
		return false, "", err
	}
	a.recordCommandLog(ctx, agentID, commandType, params, startedAt, resp)

	// Convert response to (bool, string, error)
	// 将响应转换为 (bool, string, error)
	success := resp.Status == pb.CommandStatus_SUCCESS
	message := resp.Output
	if resp.Error != "" {
		message = resp.Error
	}

	return success, message, nil
}

func (a *agentCommandSenderAdapter) recordCommandLog(ctx context.Context, agentID, commandType string, params map[string]string, startedAt time.Time, resp *pb.CommandResponse) {
	if a.auditRepo == nil || resp == nil || strings.TrimSpace(resp.CommandId) == "" {
		return
	}
	metadata := audit.CommandMetadataFromContext(ctx)
	parameters := make(audit.CommandParameters, len(params))
	for key, value := range params {
		parameters[key] = value
	}
	var createdBy *uint
	if metadata.OwnerUserID > 0 {
		ownerUserID := metadata.OwnerUserID
		createdBy = &ownerUserID
	}
	finishedAt := time.Now()
	commandLog := &audit.CommandLog{
		CommandID:      resp.CommandId,
		RequestID:      strings.TrimSpace(metadata.RequestID),
		ExecutionID:    strings.TrimSpace(metadata.ExecutionID),
		AgentID:        agentID,
		CommandType:    commandType,
		ClientType:     strings.TrimSpace(metadata.ClientType),
		DisplayCommand: buildAgentDisplayCommand(commandType, params, resp.Output),
		Parameters:     parameters,
		Status:         commandStatusFromAgentResponse(resp.Status),
		Progress:       int(resp.Progress),
		Output:         resp.Output,
		Error:          resp.Error,
		StartedAt:      &startedAt,
		FinishedAt:     &finishedAt,
		CreatedBy:      createdBy,
	}
	existing, lookupErr := a.auditRepo.GetCommandLogByCommandID(ctx, resp.CommandId)
	var commandLogErr error
	if lookupErr == nil {
		commandLog.ID = existing.ID
		commandLog.CreatedAt = existing.CreatedAt
		commandLogErr = a.auditRepo.UpdateCommandLog(ctx, commandLog)
	} else if errors.Is(lookupErr, audit.ErrCommandLogNotFound) {
		commandLogErr = a.auditRepo.CreateCommandLog(ctx, commandLog)
	} else {
		commandLogErr = lookupErr
	}
	if commandLogErr != nil && !errors.Is(commandLogErr, audit.ErrCommandIDDuplicate) {
		log.Printf("[Audit] 保存 Agent 命令日志失败: command_id=%s err=%v", resp.CommandId, commandLogErr)
	}
	// Agent 命令是控制层操作的执行细节，写入命令日志即可，不单独占审计行。
	// Agent commands are execution details of a control-plane action. Keep them in command logs, not as audit rows.
}

// recordPendingCommandLog 在发送前记录命令，确保连接失败和超时也能审计。
// recordPendingCommandLog records a command before dispatch so connection failures and timeouts remain auditable.
func (a *agentCommandSenderAdapter) recordPendingCommandLog(ctx context.Context, commandID, agentID, commandType string, params map[string]string, startedAt time.Time) {
	if a.auditRepo == nil {
		return
	}
	metadata := audit.CommandMetadataFromContext(ctx)
	parameters := make(audit.CommandParameters, len(params))
	for key, value := range params {
		parameters[key] = value
	}
	var createdBy *uint
	if metadata.OwnerUserID > 0 {
		ownerUserID := metadata.OwnerUserID
		createdBy = &ownerUserID
	}
	commandLog := &audit.CommandLog{
		CommandID:      commandID,
		RequestID:      strings.TrimSpace(metadata.RequestID),
		ExecutionID:    strings.TrimSpace(metadata.ExecutionID),
		AgentID:        agentID,
		CommandType:    commandType,
		ClientType:     strings.TrimSpace(metadata.ClientType),
		DisplayCommand: buildAgentDisplayCommand(commandType, params, ""),
		Parameters:     parameters,
		Status:         audit.CommandStatusPending,
		Progress:       0,
		StartedAt:      &startedAt,
		CreatedBy:      createdBy,
	}
	if err := a.auditRepo.CreateCommandLog(ctx, commandLog); err != nil && !errors.Is(err, audit.ErrCommandIDDuplicate) {
		log.Printf("[Audit] 保存待发送 Agent 命令失败: command_id=%s err=%v", commandID, err)
	}
}

// recordFailedCommandLog 将未得到 Agent 响应的命令更新为失败终态。
// recordFailedCommandLog marks a command that received no Agent response as failed.
func (a *agentCommandSenderAdapter) recordFailedCommandLog(ctx context.Context, commandID string, commandErr error) {
	if a.auditRepo == nil || strings.TrimSpace(commandID) == "" {
		return
	}
	item, err := a.auditRepo.GetCommandLogByCommandID(ctx, commandID)
	if err != nil {
		return
	}
	finishedAt := time.Now()
	item.Status = audit.CommandStatusFailed
	item.Error = commandErr.Error()
	item.FinishedAt = &finishedAt
	if err := a.auditRepo.UpdateCommandLog(ctx, item); err != nil {
		log.Printf("[Audit] 更新失败 Agent 命令日志失败: command_id=%s err=%v", commandID, err)
	}
}

// buildAgentDisplayCommand 生成经过仓库脱敏后可展示的 Agent 操作说明。
// buildAgentDisplayCommand builds an Agent action description that is redacted by the repository before storage.
func buildAgentDisplayCommand(commandType string, params map[string]string, output string) string {
	var result struct {
		Tool       string `json:"tool"`
		PID        int    `json:"pid"`
		OutputPath string `json:"output_path"`
	}
	_ = json.Unmarshal([]byte(output), &result)
	if result.PID > 0 {
		switch commandType {
		case "thread_dump":
			if result.Tool == "jcmd" {
				return fmt.Sprintf("jcmd %d Thread.print -l", result.PID)
			}
			if result.Tool == "jstack" {
				return fmt.Sprintf("jstack -l %d", result.PID)
			}
		case "jvm_dump":
			if result.Tool == "jcmd" {
				return fmt.Sprintf("jcmd %d GC.heap_dump %q", result.PID, result.OutputPath)
			}
			if result.Tool == "jmap" {
				return fmt.Sprintf("jmap -dump:live,format=b,file=%q %d", result.OutputPath, result.PID)
			}
		}
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	parts = append(parts, "agent:"+commandType)
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Quote(params[key]))
	}
	return strings.Join(parts, " ")
}

func commandStatusFromAgentResponse(status pb.CommandStatus) audit.CommandStatus {
	switch status {
	case pb.CommandStatus_RUNNING:
		return audit.CommandStatusRunning
	case pb.CommandStatus_SUCCESS:
		return audit.CommandStatusSuccess
	case pb.CommandStatus_FAILED:
		return audit.CommandStatusFailed
	case pb.CommandStatus_CANCELLED:
		return audit.CommandStatusCancelled
	default:
		return audit.CommandStatusPending
	}
}

// stringToCommandType converts a command type string to pb.CommandType.
// stringToCommandType 将命令类型字符串转换为 pb.CommandType。
func (a *agentCommandSenderAdapter) stringToCommandType(cmdType string) pb.CommandType {
	switch cmdType {
	case "check_port", "check_directory", "check_http", "check_process", "check_java", "check_tcp", "check_path_ready", "stat_path", "cleanup_path", "stx_java_proxy_probe", "stx_java_proxy_stat", "stx_java_proxy_list", "stx_java_proxy_preview", "stx_java_proxy_inspect_checkpoint", "stx_java_proxy_inspect_checkpoint_source_state", "stx_java_proxy_inspect_imap_wal", "sync_local_run", "sync_local_status", "sync_local_stop", "sync_local_logs", "sync_job_logs", "full":
		return pb.CommandType_PRECHECK
	case "install":
		return pb.CommandType_INSTALL
	case "uninstall":
		return pb.CommandType_UNINSTALL
	case "upgrade":
		return pb.CommandType_UPGRADE
	case "start":
		return pb.CommandType_START
	case "stop":
		return pb.CommandType_STOP
	case "restart":
		return pb.CommandType_RESTART
	case "status":
		return pb.CommandType_STATUS
	case "get_logs":
		return pb.CommandType_COLLECT_LOGS
	case "thread_dump":
		return pb.CommandType_THREAD_DUMP
	case "jvm_dump":
		return pb.CommandType_JVM_DUMP
	case "pull_config":
		return pb.CommandType_PULL_CONFIG
	case "remove_install_dir":
		return pb.CommandType_REMOVE_INSTALL_DIR
	default:
		return pb.CommandType_PRECHECK
	}
}

// ==================== Plugin Service Adapters 插件服务适配器 ====================

// pluginAgentCommandSenderAdapter adapts agent.Manager to plugin.AgentCommandSender interface.
// pluginAgentCommandSenderAdapter 将 agent.Manager 适配到 plugin.AgentCommandSender 接口。
type pluginAgentCommandSenderAdapter struct {
	manager *agent.Manager
}

// SendCommand sends a command to an agent and returns the result.
// SendCommand 向 Agent 发送命令并返回结果。
func (a *pluginAgentCommandSenderAdapter) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	// Convert command type string to pb.CommandType
	// 将命令类型字符串转换为 pb.CommandType
	cmdType := a.stringToCommandType(commandType)

	// Use longer timeout for plugin transfer (5 minutes)
	// 插件传输使用更长的超时时间（5 分钟）
	timeout := 5 * time.Minute
	if commandType == "install_plugin" {
		timeout = 2 * time.Minute
	}

	resp, err := a.manager.SendCommand(ctx, agentID, cmdType, params, timeout)
	if err != nil {
		return false, "", err
	}

	// For transfer_plugin command, RUNNING status means chunk received successfully
	// 对于 transfer_plugin 命令，RUNNING 状态表示块接收成功
	success := resp.Status == pb.CommandStatus_SUCCESS
	if commandType == "transfer_plugin" {
		// Accept both SUCCESS and RUNNING as success for chunk transfer
		// 对于块传输，接受 SUCCESS 和 RUNNING 作为成功
		success = resp.Status == pb.CommandStatus_SUCCESS || resp.Status == pb.CommandStatus_RUNNING
	}

	message := resp.Output
	if resp.Error != "" {
		message = resp.Error
	}

	return success, message, nil
}

// stringToCommandType converts a command type string to pb.CommandType for plugin operations.
// stringToCommandType 将命令类型字符串转换为 pb.CommandType 用于插件操作。
func (a *pluginAgentCommandSenderAdapter) stringToCommandType(cmdType string) pb.CommandType {
	switch cmdType {
	case "transfer_plugin":
		return pb.CommandType_TRANSFER_PLUGIN
	case "install_plugin":
		return pb.CommandType_INSTALL_PLUGIN
	case "uninstall_plugin":
		return pb.CommandType_UNINSTALL_PLUGIN
	case "list_plugins":
		return pb.CommandType_LIST_PLUGINS
	default:
		return pb.CommandType_TRANSFER_PLUGIN
	}
}

// clusterNodeGetterAdapter adapts cluster.Service to plugin.ClusterNodeGetter interface.
// clusterNodeGetterAdapter 将 cluster.Service 适配到 plugin.ClusterNodeGetter 接口。
type clusterNodeGetterAdapter struct {
	clusterService *cluster.Service
}

// GetClusterNodes returns all nodes for a cluster.
// GetClusterNodes 返回集群的所有节点。
func (a *clusterNodeGetterAdapter) GetClusterNodes(ctx context.Context, clusterID uint) ([]plugin.ClusterNodeInfo, error) {
	nodes, err := a.clusterService.GetNodes(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	result := make([]plugin.ClusterNodeInfo, len(nodes))
	for i, node := range nodes {
		result[i] = plugin.ClusterNodeInfo{
			NodeID:     node.ID,
			HostID:     node.HostID,
			InstallDir: node.InstallDir,
		}
	}
	return result, nil
}

// hostInfoGetterAdapter adapts host.Service to plugin.HostInfoGetter interface.
// hostInfoGetterAdapter 将 host.Service 适配到 plugin.HostInfoGetter 接口。
type hostInfoGetterAdapter struct {
	hostService *host.Service
}

// GetHostAgentID returns the Agent ID for a host.
// GetHostAgentID 返回主机的 Agent ID。
func (a *hostInfoGetterAdapter) GetHostAgentID(ctx context.Context, hostID uint) (string, error) {
	h, err := a.hostService.Get(ctx, hostID)
	if err != nil {
		return "", err
	}
	return h.AgentID, nil
}

// ==================== Installer Service Adapters 安装服务适配器 ====================

// hostProviderAdapter adapts host.Service to installer.HostProvider interface.
// hostProviderAdapter 将 host.Service 适配到 installer.HostProvider 接口。
type hostProviderAdapter struct {
	hostService *host.Service
}

// GetHostByID returns host information by ID for installer precheck.
// GetHostByID 根据 ID 返回主机信息，用于安装预检查。
func (a *hostProviderAdapter) GetHostByID(ctx context.Context, hostID uint) (*installer.HostInfo, error) {
	h, err := a.hostService.Get(ctx, hostID)
	if err != nil {
		return nil, err
	}

	return &installer.HostInfo{
		ID:          h.ID,
		Name:        h.Name,
		AgentID:     h.AgentID,
		AgentStatus: string(h.AgentStatus),
		LastSeen:    h.LastHeartbeat,
	}, nil
}

// installerAgentManagerAdapter adapts agent.Manager to installer.AgentManager interface.
// installerAgentManagerAdapter 将 agent.Manager 适配到 installer.AgentManager 接口。
type installerAgentManagerAdapter struct {
	manager     *agent.Manager
	hostService *host.Service
}

// GetAgentByHostID returns the agent connection for a host.
// GetAgentByHostID 返回主机的 Agent 连接。
func (a *installerAgentManagerAdapter) GetAgentByHostID(hostID uint) (agentID string, connected bool) {
	// Get host info to find agent ID
	// 获取主机信息以找到 agent ID
	ctx := context.Background()
	h, err := a.hostService.Get(ctx, hostID)
	if err != nil || h == nil {
		return "", false
	}

	if h.AgentID == "" {
		return "", false
	}

	// Check if agent is connected in agent manager
	// 检查 agent 是否在 agent manager 中连接
	conn, ok := a.manager.GetAgent(h.AgentID)
	if !ok || conn == nil {
		return "", false
	}

	// Check if agent status is connected
	// 检查 agent 状态是否为已连接
	if conn.GetStatus() != agent.AgentStatusConnected {
		return "", false
	}

	return h.AgentID, true
}

// SendInstallCommand sends an installation command to an agent.
// SendInstallCommand 向 Agent 发送安装命令。
func (a *installerAgentManagerAdapter) SendInstallCommand(ctx context.Context, agentID string, params map[string]string) (commandID string, err error) {
	// Use async command to allow polling for status updates
	// 使用异步命令以允许轮询状态更新
	return a.manager.SendCommandAsync(agentID, pb.CommandType_INSTALL, params, 30*time.Minute)
}

// GetCommandStatus returns the status of a command.
// GetCommandStatus 返回命令的状态。
func (a *installerAgentManagerAdapter) GetCommandStatus(commandID string) (status string, progress int, message string, err error) {
	return a.manager.GetCommandStatus(commandID)
}

// SendCommand sends a command to an agent and returns the result.
// SendCommand 向 Agent 发送命令并返回结果。
func (a *installerAgentManagerAdapter) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (success bool, output string, err error) {
	// Convert command type string to pb.CommandType
	// 将命令类型字符串转换为 pb.CommandType
	cmdType := a.stringToCommandType(commandType)

	// Add sub_command parameter for precheck commands
	// 为预检查命令添加 sub_command 参数
	if cmdType == pb.CommandType_PRECHECK && params["sub_command"] == "" {
		params["sub_command"] = commandType
	}

	// Send command with 30 second timeout
	// 使用 30 秒超时发送命令
	resp, err := a.manager.SendCommand(ctx, agentID, cmdType, params, 30*time.Second)
	if err != nil {
		return false, "", err
	}

	// Convert response to (bool, string, error)
	// 将响应转换为 (bool, string, error)
	success = resp.Status == pb.CommandStatus_SUCCESS
	message := resp.Output
	if resp.Error != "" {
		message = resp.Error
	}

	return success, message, nil
}

// stringToCommandType converts a command type string to pb.CommandType.
// stringToCommandType 将命令类型字符串转换为 pb.CommandType。
func (a *installerAgentManagerAdapter) stringToCommandType(cmdType string) pb.CommandType {
	switch cmdType {
	case "check_port", "check_directory", "check_http", "check_process", "check_java", "check_tcp", "check_path_ready", "stat_path", "cleanup_path", "stx_java_proxy_probe", "stx_java_proxy_stat", "stx_java_proxy_list", "stx_java_proxy_preview", "stx_java_proxy_inspect_checkpoint", "stx_java_proxy_inspect_checkpoint_source_state", "stx_java_proxy_inspect_imap_wal", "sync_local_run", "sync_local_status", "sync_local_stop", "full":
		return pb.CommandType_PRECHECK
	case "install":
		return pb.CommandType_INSTALL
	case "uninstall":
		return pb.CommandType_UNINSTALL
	case "upgrade":
		return pb.CommandType_UPGRADE
	case "start":
		return pb.CommandType_START
	case "stop":
		return pb.CommandType_STOP
	case "restart":
		return pb.CommandType_RESTART
	case "status":
		return pb.CommandType_STATUS
	case "get_logs":
		return pb.CommandType_COLLECT_LOGS
	case "transfer_package":
		return pb.CommandType_TRANSFER_PACKAGE
	default:
		return pb.CommandType_PRECHECK
	}
}

// SendTransferPackageCommand sends a package transfer chunk to an agent.
// SendTransferPackageCommand 向 Agent 发送安装包传输块。
func (a *installerAgentManagerAdapter) SendTransferPackageCommand(ctx context.Context, agentID string, version string, fileName string, chunk []byte, offset int64, totalSize int64, isLast bool, checksum string) (success bool, receivedBytes int64, localPath string, err error) {
	// Build parameters / 构建参数
	params := map[string]string{
		"version":    version,
		"file_name":  fileName,
		"chunk":      base64.StdEncoding.EncodeToString(chunk),
		"offset":     fmt.Sprintf("%d", offset),
		"total_size": fmt.Sprintf("%d", totalSize),
		"is_last":    fmt.Sprintf("%t", isLast),
	}
	if checksum != "" {
		params["checksum"] = checksum
	}

	// Use longer timeout for package transfer (5 minutes per chunk)
	// 安装包传输使用更长的超时时间（每块 5 分钟）
	resp, err := a.manager.SendCommand(ctx, agentID, pb.CommandType_TRANSFER_PACKAGE, params, 5*time.Minute)
	if err != nil {
		return false, 0, "", err
	}

	// Accept both SUCCESS and RUNNING as success for chunk transfer
	// 对于块传输，接受 SUCCESS 和 RUNNING 作为成功
	success = resp.Status == pb.CommandStatus_SUCCESS || resp.Status == pb.CommandStatus_RUNNING

	// Parse response to get received bytes and local path
	// 解析响应获取已接收字节数和本地路径
	if resp.Output != "" {
		var transferResp struct {
			Success       bool   `json:"success"`
			Message       string `json:"message"`
			ReceivedBytes int64  `json:"received_bytes"`
			LocalPath     string `json:"local_path"`
		}
		if jsonErr := json.Unmarshal([]byte(resp.Output), &transferResp); jsonErr == nil {
			receivedBytes = transferResp.ReceivedBytes
			localPath = transferResp.LocalPath
			if !transferResp.Success {
				success = false
			}
		}
	}

	if resp.Error != "" {
		return false, receivedBytes, localPath, fmt.Errorf("%s", resp.Error)
	}

	return success, receivedBytes, localPath, nil
}

// ==================== Config Service Adapters 配置服务适配器 ====================

// configHostProviderAdapter adapts host.Service to appconfig.HostProvider interface.
// configHostProviderAdapter 将 host.Service 适配到 appconfig.HostProvider 接口。
type configHostProviderAdapter struct {
	hostService *host.Service
}

// GetHostByID returns host information by ID for config service.
// GetHostByID 根据 ID 返回主机信息，用于配置服务。
func (a *configHostProviderAdapter) GetHostByID(ctx context.Context, hostID uint) (*appconfig.HostInfo, error) {
	h, err := a.hostService.Get(ctx, hostID)
	if err != nil {
		return nil, err
	}

	return &appconfig.HostInfo{
		ID:        h.ID,
		Name:      h.Name,
		IPAddress: h.IPAddress,
	}, nil
}

// configAgentClientAdapter adapts agent.Manager to appconfig.AgentClient interface.
// configAgentClientAdapter 将 agent.Manager 适配到 appconfig.AgentClient 接口。
type configAgentClientAdapter struct {
	manager     *agent.Manager
	hostService *host.Service
}

// PullConfig pulls config file content from a host via Agent.
// PullConfig 通过 Agent 从主机拉取配置文件内容。
func (a *configAgentClientAdapter) PullConfig(ctx context.Context, hostID uint, installDir string, configType appconfig.ConfigType) (string, error) {
	// Get agent ID for the host
	// 获取主机的 Agent ID
	h, err := a.hostService.Get(ctx, hostID)
	if err != nil {
		return "", fmt.Errorf("failed to get host: %w", err)
	}

	if h.AgentID == "" {
		return "", fmt.Errorf("host %d has no agent", hostID)
	}

	// Send PULL_CONFIG command to agent
	// 向 Agent 发送 PULL_CONFIG 命令
	params := map[string]string{
		"install_dir": installDir,
		"config_type": string(configType),
	}

	resp, err := a.manager.SendCommand(ctx, h.AgentID, pb.CommandType_PULL_CONFIG, params, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to send pull config command: %w", err)
	}

	if resp.Status != pb.CommandStatus_SUCCESS {
		return "", fmt.Errorf("pull config failed: %s", resp.Error)
	}

	// Parse response to extract content
	// 解析响应以提取内容
	var result struct {
		Success    bool   `json:"success"`
		Message    string `json:"message"`
		ConfigType string `json:"config_type"`
		Content    string `json:"content"`
	}
	if err := json.Unmarshal([]byte(resp.Output), &result); err != nil {
		return "", fmt.Errorf("failed to parse pull config response: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("pull config failed: %s", result.Message)
	}

	return result.Content, nil
}

// PushConfig pushes config file content to a host via Agent.
// PushConfig 通过 Agent 将配置文件内容推送到主机。
func (a *configAgentClientAdapter) PushConfig(ctx context.Context, hostID uint, installDir string, configType appconfig.ConfigType, content string) error {
	// Get agent ID for the host
	// 获取主机的 Agent ID
	h, err := a.hostService.Get(ctx, hostID)
	if err != nil {
		return fmt.Errorf("failed to get host: %w", err)
	}

	if h.AgentID == "" {
		return fmt.Errorf("host %d has no agent", hostID)
	}

	// Send UPDATE_CONFIG command to agent
	// 向 Agent 发送 UPDATE_CONFIG 命令
	params := map[string]string{
		"install_dir": installDir,
		"config_type": string(configType),
		"content":     content,
		"backup":      "true",
	}

	resp, err := a.manager.SendCommand(ctx, h.AgentID, pb.CommandType_UPDATE_CONFIG, params, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to send update config command: %w", err)
	}

	if resp.Status != pb.CommandStatus_SUCCESS {
		return fmt.Errorf("update config failed: %s", resp.Error)
	}

	return nil
}

// configNodeInfoProviderAdapter adapts cluster.Service to appconfig.NodeInfoProvider interface.
// configNodeInfoProviderAdapter 将 cluster.Service 适配到 appconfig.NodeInfoProvider 接口。
type configNodeInfoProviderAdapter struct {
	clusterService *cluster.Service
}

// GetNodeInstallDir returns the install directory for a node by cluster ID and host ID.
// GetNodeInstallDir 根据集群 ID 和主机 ID 返回节点的安装目录。
func (a *configNodeInfoProviderAdapter) GetNodeInstallDir(ctx context.Context, clusterID uint, hostID uint) (string, error) {
	return a.clusterService.GetNodeInstallDir(ctx, clusterID, hostID)
}

// configPortMetadataUpdaterAdapter adapts cluster.Repository to appconfig.PortMetadataUpdater interface.
// configPortMetadataUpdaterAdapter 将 cluster.Repository 适配到 appconfig.PortMetadataUpdater 接口。
type configPortMetadataUpdaterAdapter struct {
	clusterRepo *cluster.Repository
}

// UpdateSeatunnelAPIPortByHost updates the API port metadata for non-worker nodes on a host.
// UpdateSeatunnelAPIPortByHost 更新指定主机上非 worker 节点的 API 端口元数据。
func (a *configPortMetadataUpdaterAdapter) UpdateSeatunnelAPIPortByHost(ctx context.Context, clusterID uint, hostID uint, port int) error {
	if a == nil || a.clusterRepo == nil {
		return nil
	}
	nodes, err := a.clusterRepo.GetNodesByHostID(ctx, hostID)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if node == nil || node.ClusterID != clusterID || node.Role == cluster.NodeRoleWorker {
			continue
		}
		if node.APIPort == port {
			continue
		}
		node.APIPort = port
		if err := a.clusterRepo.UpdateNode(ctx, node); err != nil {
			return err
		}
	}
	return nil
}

// UpdateHazelcastPortByHost updates hazelcast-related port metadata for nodes on a host.
// UpdateHazelcastPortByHost 更新指定主机上节点的 Hazelcast 端口元数据。
func (a *configPortMetadataUpdaterAdapter) UpdateHazelcastPortByHost(ctx context.Context, clusterID uint, hostID uint, configType appconfig.ConfigType, port int) error {
	if a == nil || a.clusterRepo == nil {
		return nil
	}
	nodes, err := a.clusterRepo.GetNodesByHostID(ctx, hostID)
	if err != nil {
		return err
	}
	var clusterObj *cluster.Cluster
	for _, node := range nodes {
		if node == nil || node.ClusterID != clusterID {
			continue
		}
		shouldUpdate := false
		switch configType {
		case appconfig.ConfigTypeHazelcast:
			shouldUpdate = node.Role == cluster.NodeRoleMasterWorker || node.Role == cluster.NodeRoleMaster
		case appconfig.ConfigTypeHazelcastMaster:
			shouldUpdate = node.Role == cluster.NodeRoleMaster
		case appconfig.ConfigTypeHazelcastWorker:
			shouldUpdate = node.Role == cluster.NodeRoleWorker
		}
		if !shouldUpdate || node.HazelcastPort == port {
			continue
		}
		node.HazelcastPort = port
		if err := a.clusterRepo.UpdateNode(ctx, node); err != nil {
			return err
		}
		if clusterObj == nil {
			clusterObj, _ = a.clusterRepo.GetByID(ctx, clusterID, false)
		}
	}
	if clusterObj != nil {
		if clusterObj.Config == nil {
			clusterObj.Config = cluster.ClusterConfig{}
		}
		switch configType {
		case appconfig.ConfigTypeHazelcast, appconfig.ConfigTypeHazelcastMaster:
			ensureClusterConfigPath(clusterObj.Config, "ports")["master_hazelcast_port"] = port
		case appconfig.ConfigTypeHazelcastWorker:
			ensureClusterConfigPath(clusterObj.Config, "ports")["worker_port"] = port
		}
		if err := a.clusterRepo.Update(ctx, clusterObj); err != nil {
			return err
		}
	}
	return nil
}

// UpdateClusterJobLogMode syncs runtime job log mode inferred from log4j2.properties.
// UpdateClusterJobLogMode 同步由 log4j2.properties 反向解析出的作业日志模式。
func (a *configPortMetadataUpdaterAdapter) UpdateClusterJobLogMode(ctx context.Context, clusterID uint, mode string) error {
	if a == nil || a.clusterRepo == nil || strings.TrimSpace(mode) == "" {
		return nil
	}
	clusterObj, err := a.clusterRepo.GetByID(ctx, clusterID, false)
	if err != nil {
		return err
	}
	if clusterObj.Config == nil {
		clusterObj.Config = cluster.ClusterConfig{}
	}
	ensureClusterConfigPath(clusterObj.Config, "runtime")["job_log_mode"] = mode
	return a.clusterRepo.Update(ctx, clusterObj)
}

func ensureClusterConfigPath(cfg cluster.ClusterConfig, key string) map[string]interface{} {
	current, ok := cfg[key].(map[string]interface{})
	if !ok || current == nil {
		current = map[string]interface{}{}
		cfg[key] = current
	}
	return current
}

// ==================== Discovery Service Adapters 发现服务适配器 ====================

// discoveryHostProviderAdapter adapts host.Service to discovery.HostProvider interface.
// discoveryHostProviderAdapter 将 host.Service 适配到 discovery.HostProvider 接口。
type discoveryHostProviderAdapter struct {
	hostService *host.Service
}

// GetHostByID returns host information by ID for discovery service.
// GetHostByID 根据 ID 返回主机信息，用于发现服务。
func (a *discoveryHostProviderAdapter) GetHostByID(ctx context.Context, hostID uint) (*discovery.HostInfo, error) {
	h, err := a.hostService.Get(ctx, hostID)
	if err != nil {
		return nil, err
	}

	return &discovery.HostInfo{
		ID:          h.ID,
		Name:        h.Name,
		IPAddress:   h.IPAddress,
		AgentID:     h.AgentID,
		AgentStatus: string(h.AgentStatus),
	}, nil
}

// GetHostByAgentID returns host information by agent ID.
// GetHostByAgentID 根据 Agent ID 返回主机信息。
func (a *discoveryHostProviderAdapter) GetHostByAgentID(ctx context.Context, agentID string) (*discovery.HostInfo, error) {
	h, err := a.hostService.GetByAgentID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	return &discovery.HostInfo{
		ID:          h.ID,
		Name:        h.Name,
		IPAddress:   h.IPAddress,
		AgentID:     h.AgentID,
		AgentStatus: string(h.AgentStatus),
	}, nil
}

// discoveryAgentDiscovererAdapter adapts agent.Manager to discovery.AgentDiscoverer interface.
// discoveryAgentDiscovererAdapter 将 agent.Manager 适配到 discovery.AgentDiscoverer 接口。
type discoveryAgentDiscovererAdapter struct {
	manager *agent.Manager
}

// TriggerDiscovery triggers cluster discovery on an agent.
// TriggerDiscovery 在 Agent 上触发集群发现。
func (a *discoveryAgentDiscovererAdapter) TriggerDiscovery(ctx context.Context, agentID string) ([]*discovery.DiscoveredCluster, error) {
	// Send DISCOVER_CLUSTERS command to agent
	// 向 Agent 发送 DISCOVER_CLUSTERS 命令
	resp, err := a.manager.SendCommand(ctx, agentID, pb.CommandType_DISCOVER_CLUSTERS, nil, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to send discover clusters command: %w / 发送发现集群命令失败: %w", err, err)
	}

	if resp.Status != pb.CommandStatus_SUCCESS {
		return nil, fmt.Errorf("discover clusters failed: %s / 发现集群失败: %s", resp.Error, resp.Error)
	}

	// For now, return empty list - the simplified discovery only returns processes
	// 目前返回空列表 - 简化版发现只返回进程
	return []*discovery.DiscoveredCluster{}, nil
}

// DiscoverProcesses discovers SeaTunnel processes on an agent (simplified).
// DiscoverProcesses 在 Agent 上发现 SeaTunnel 进程（简化版）。
// Only returns PID, role, and install_dir - no config parsing.
// 只返回 PID、角色和安装目录 - 不解析配置。
func (a *discoveryAgentDiscovererAdapter) DiscoverProcesses(ctx context.Context, agentID string) ([]*discovery.DiscoveredProcess, error) {
	// Send DISCOVER_CLUSTERS command to agent (reuses the same command type)
	// 向 Agent 发送 DISCOVER_CLUSTERS 命令（复用相同的命令类型）
	resp, err := a.manager.SendCommand(ctx, agentID, pb.CommandType_DISCOVER_CLUSTERS, nil, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to send discover processes command: %w / 发送发现进程命令失败: %w", err, err)
	}

	if resp.Status != pb.CommandStatus_SUCCESS {
		return nil, fmt.Errorf("discover processes failed: %s / 发现进程失败: %s", resp.Error, resp.Error)
	}

	// Parse the JSON output to extract discovered processes
	// 解析 JSON 输出以提取发现的进程
	return parseDiscoveredProcessesJSON(resp.Output)
}

// parseDiscoveredProcessesJSON parses the agent JSON output to extract discovered processes.
// parseDiscoveredProcessesJSON 解析 Agent JSON 输出以提取发现的进程。
func parseDiscoveredProcessesJSON(output string) ([]*discovery.DiscoveredProcess, error) {
	// Define the expected JSON structure / 定义预期的 JSON 结构
	type ProcessInfo struct {
		PID           int    `json:"pid"`
		Role          string `json:"role"`
		InstallDir    string `json:"install_dir"`
		Version       string `json:"version"`        // SeaTunnel version / SeaTunnel 版本
		HazelcastPort int    `json:"hazelcast_port"` // Hazelcast cluster port / Hazelcast 集群端口
		APIPort       int    `json:"api_port"`       // REST API port / REST API 端口
	}
	type DiscoveryResult struct {
		Success   bool          `json:"success"`
		Message   string        `json:"message"`
		Processes []ProcessInfo `json:"processes"`
	}

	var result DiscoveryResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		// Fallback to text parsing for backward compatibility
		// 为了向后兼容，回退到文本解析
		log.Printf("[Discovery] JSON parse failed, trying text parse: %v / JSON 解析失败，尝试文本解析: %v", err, err)
		return parseDiscoveredProcessesText(output), nil
	}

	if !result.Success {
		return nil, fmt.Errorf("discovery failed: %s / 发现失败: %s", result.Message, result.Message)
	}

	// Convert to discovery.DiscoveredProcess / 转换为 discovery.DiscoveredProcess
	processes := make([]*discovery.DiscoveredProcess, 0, len(result.Processes))
	for _, p := range result.Processes {
		processes = append(processes, &discovery.DiscoveredProcess{
			PID:           p.PID,
			Role:          p.Role,
			InstallDir:    p.InstallDir,
			Version:       p.Version,
			HazelcastPort: p.HazelcastPort,
			APIPort:       p.APIPort,
		})
	}

	return processes, nil
}

// parseDiscoveredProcessesText parses the agent text output (legacy format).
// parseDiscoveredProcessesText 解析 Agent 文本输出（旧格式）。
func parseDiscoveredProcessesText(output string) []*discovery.DiscoveredProcess {
	var processes []*discovery.DiscoveredProcess

	// Parse output line by line
	// 逐行解析输出
	// Format: "- PID: 12345, Role: master, InstallDir: /opt/seatunnel"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- PID:") {
			continue
		}

		// Parse the line / 解析行
		var pid int
		var role, installDir string

		// Extract PID / 提取 PID
		if pidStart := strings.Index(line, "PID:"); pidStart != -1 {
			pidStr := line[pidStart+4:]
			if commaIdx := strings.Index(pidStr, ","); commaIdx != -1 {
				pidStr = strings.TrimSpace(pidStr[:commaIdx])
			}
			fmt.Sscanf(pidStr, "%d", &pid)
		}

		// Extract Role / 提取角色
		if roleStart := strings.Index(line, "Role:"); roleStart != -1 {
			roleStr := line[roleStart+5:]
			if commaIdx := strings.Index(roleStr, ","); commaIdx != -1 {
				role = strings.TrimSpace(roleStr[:commaIdx])
			}
		}

		// Extract InstallDir / 提取安装目录
		if dirStart := strings.Index(line, "InstallDir:"); dirStart != -1 {
			installDir = strings.TrimSpace(line[dirStart+11:])
		}

		if pid > 0 && role != "" && installDir != "" {
			processes = append(processes, &discovery.DiscoveredProcess{
				PID:        pid,
				Role:       role,
				InstallDir: installDir,
			})
		}
	}

	return processes
}

// monitorClusterNodeProviderAdapter adapts cluster.Service and host.Service to monitor.ClusterNodeProvider interface.
// monitorClusterNodeProviderAdapter 将 cluster.Service 和 host.Service 适配到 monitor.ClusterNodeProvider 接口。
type monitorClusterNodeProviderAdapter struct {
	clusterService *cluster.Service
	hostService    *host.Service
}

// GetNodesByClusterID returns all nodes for a cluster with agent info.
// GetNodesByClusterID 返回集群的所有节点及其 Agent 信息。
func (a *monitorClusterNodeProviderAdapter) GetNodesByClusterID(ctx context.Context, clusterID uint) ([]*monitor.NodeInfoForMonitor, error) {
	nodes, err := a.clusterService.GetNodes(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	result := make([]*monitor.NodeInfoForMonitor, 0, len(nodes))
	for _, node := range nodes {
		// Get agent ID for this host / 获取此主机的 Agent ID
		agentID := ""
		h, err := a.hostService.Get(ctx, node.HostID)
		if err == nil && h != nil {
			agentID = h.AgentID
		}

		result = append(result, &monitor.NodeInfoForMonitor{
			HostID:     node.HostID,
			AgentID:    agentID,
			InstallDir: node.InstallDir,
			Role:       string(node.Role),
			ProcessPID: node.ProcessPID,
		})
	}

	return result, nil
}

// monitorAgentConfigSenderAdapter adapts agent.Manager to monitor.AgentConfigSender interface.
// monitorAgentConfigSenderAdapter 将 agent.Manager 适配到 monitor.AgentConfigSender 接口。
type monitorAgentConfigSenderAdapter struct {
	manager *agent.Manager
}

// SendMonitorConfig sends monitor config and tracked processes to agent.
// SendMonitorConfig 向 Agent 发送监控配置和跟踪的进程列表。
func (a *monitorAgentConfigSenderAdapter) SendMonitorConfig(ctx context.Context, agentID string, config *monitor.MonitorConfig, processes []*monitor.TrackedProcessInfo) error {
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
			log.Printf("[Monitor] Failed to marshal processes: %v / 序列化进程列表失败: %v", err, err)
		} else {
			params["tracked_processes"] = string(processesJSON)
		}
	}

	// Send UPDATE_MONITOR_CONFIG command to agent / 向 Agent 发送 UPDATE_MONITOR_CONFIG 命令
	_, err := a.manager.SendCommand(ctx, agentID, pb.CommandType_UPDATE_MONITOR_CONFIG, params, 30*time.Second)
	return err
}

// grpcClusterNodeProviderAdapter adapts cluster.Service to grpc.ClusterNodeProvider interface.
// grpcClusterNodeProviderAdapter 将 cluster.Service 适配到 grpc.ClusterNodeProvider 接口。
type grpcClusterNodeProviderAdapter struct {
	clusterService *cluster.Service
	monitorService *monitor.Service
}

// GetNodeByHostAndInstallDirAndRole returns cluster and node ID by host ID, install dir and role.
// GetNodeByHostAndInstallDirAndRole 根据主机 ID、安装目录和角色返回集群和节点 ID。
func (a *grpcClusterNodeProviderAdapter) GetNodeByHostAndInstallDirAndRole(ctx context.Context, hostID uint, installDir, role string) (clusterID, nodeID uint, found bool, err error) {
	return a.clusterService.GetNodeByHostAndInstallDirAndRole(ctx, hostID, installDir, role)
}

// GetNodesByHostID returns all nodes on a specific host with their cluster's monitor config.
// GetNodesByHostID 返回特定主机上的所有节点及其集群的监控配置。
func (a *grpcClusterNodeProviderAdapter) GetNodesByHostID(ctx context.Context, hostID uint) ([]*grpcServer.NodeWithMonitorConfig, error) {
	nodes, err := a.clusterService.GetNodesByHostID(ctx, hostID)
	if err != nil {
		return nil, err
	}

	result := make([]*grpcServer.NodeWithMonitorConfig, 0, len(nodes))
	configCache := make(map[uint]*monitor.MonitorConfig) // Cache configs by cluster ID / 按集群 ID 缓存配置

	for _, node := range nodes {
		// Get or cache monitor config for this cluster / 获取或缓存此集群的监控配置
		config, ok := configCache[node.ClusterID]
		if !ok {
			config, _ = a.monitorService.GetOrCreateConfig(ctx, node.ClusterID)
			configCache[node.ClusterID] = config
		}

		result = append(result, &grpcServer.NodeWithMonitorConfig{
			ClusterID:     node.ClusterID,
			NodeID:        node.ID,
			InstallDir:    node.InstallDir,
			Role:          string(node.Role),
			ProcessPID:    node.ProcessPID,
			MonitorConfig: config,
		})
	}

	return result, nil
}

// UpdateNodeProcessStatus updates the process PID and status for a node.
// UpdateNodeProcessStatus 更新节点的进程 PID 和状态。
func (a *grpcClusterNodeProviderAdapter) UpdateNodeProcessStatus(ctx context.Context, nodeID uint, pid int, status string) error {
	return a.clusterService.UpdateNodeProcessStatus(ctx, nodeID, pid, status)
}

// RefreshClusterStatusFromNodes recalculates cluster status from its nodes (e.g. after heartbeat).
// RefreshClusterStatusFromNodes 根据节点状态重新计算集群状态（如心跳更新节点后）。
func (a *grpcClusterNodeProviderAdapter) RefreshClusterStatusFromNodes(ctx context.Context, clusterID uint) {
	a.clusterService.RefreshClusterStatusFromNodes(ctx, clusterID)
}

// GetClusterNodeDisplayInfo returns cluster name and node display for audit resource name.
// GetClusterNodeDisplayInfo 返回集群名及节点展示，用于审计资源名称。
func (a *grpcClusterNodeProviderAdapter) GetClusterNodeDisplayInfo(ctx context.Context, clusterID, nodeID uint) (clusterName, nodeDisplay string) {
	return a.clusterService.GetClusterNodeDisplayInfo(ctx, clusterID, nodeID)
}

func normalizeAPIV1RoutePath(rawPath, fallback string) string {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		path = fallback
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Support both full path (/api/v1/...) and api-v1-relative path (/monitoring/...).
	// 同时支持完整路径（/api/v1/...）和 v1 相对路径（/monitoring/...）。
	if strings.HasPrefix(path, "/api/v1/") {
		path = strings.TrimPrefix(path, "/api/v1")
		if path == "" {
			path = "/"
		}
	}
	return path
}
