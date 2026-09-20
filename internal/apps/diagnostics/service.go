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

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/db"
)

const (
	maxErrorMessageLength  = 2048
	maxErrorEvidenceLength = 8192
)

type clusterReader interface {
	List(ctx context.Context, filter *cluster.ClusterFilter) ([]*cluster.Cluster, int64, error)
	Get(ctx context.Context, id uint) (*cluster.Cluster, error)
	GetStatus(ctx context.Context, clusterID uint) (*cluster.ClusterStatusInfo, error)
}

type processEventReader interface {
	ListClusterEvents(ctx context.Context, clusterID uint, limit int) ([]*monitor.ProcessEvent, error)
}

type alertInstanceReader interface {
	ListAlertInstances(ctx context.Context, filter *monitoringapp.AlertInstanceFilter) (*monitoringapp.AlertInstanceListData, error)
}

type hostReader interface {
	GetHostByID(ctx context.Context, id uint) (*cluster.HostInfo, error)
}

type diagnosticAgentCommandSender interface {
	SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error)
}

// Service provides diagnostics workspace bootstrap data and owns the
// boundary between diagnostics and other runtime domains.
// Service 提供诊断中心初始化数据，并维护 diagnostics 与其他运行时领域之间的边界。
type Service struct {
	repo              *Repository
	configRepo        *appconfig.Repository
	clusterService    clusterReader
	hostService       hostReader
	monitorService    processEventReader
	monitoringService alertInstanceReader
	agentSender       diagnosticAgentCommandSender
	executionService  *executionapp.Service
	taskEvents        *diagnosticTaskEventHub
	taskEventsOnce    sync.Once
	autoPolicyRuntime sync.Once
	policyChecker     *AutoPolicyChecker
	troubleshooting   *TroubleshootingService
}

// SetExecutionService 设置诊断任务使用的公共执行服务。
// SetExecutionService sets the shared execution service used by diagnostic tasks.
func (s *Service) SetExecutionService(service *executionapp.Service) {
	if s == nil {
		return
	}
	s.executionService = service
}

// ListLogCursorsByAgent returns all log cursors for the given agent.
// ListLogCursorsByAgent 返回某个 Agent 的所有日志游标。
func (s *Service) ListLogCursorsByAgent(ctx context.Context, agentID string) ([]*SeatunnelLogCursor, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return []*SeatunnelLogCursor{}, nil
	}
	return s.repo.ListLogCursorsByAgent(ctx, agentID)
}

// NewService creates a diagnostics service using the global database when available.
// NewService 使用全局数据库（如果已初始化）创建诊断服务。
func NewService(clusterService *cluster.Service, monitorService *monitor.Service, monitoringService *monitoringapp.Service) *Service {
	var repo *Repository
	var configRepo *appconfig.Repository
	if db.IsDatabaseInitialized() {
		database := db.DB(context.Background())
		repo = NewRepository(database)
		configRepo = appconfig.NewRepository(database)
	}
	return newDiagnosticsService(repo, configRepo, clusterService, monitorService, monitoringService)
}

// NewServiceWithRepository creates a diagnostics service with an explicit repository.
// NewServiceWithRepository 使用显式仓储创建诊断服务。
func NewServiceWithRepository(repo *Repository, clusterService clusterReader, monitorService processEventReader, monitoringService alertInstanceReader) *Service {
	var configRepo *appconfig.Repository
	if db.IsDatabaseInitialized() {
		configRepo = appconfig.NewRepository(db.DB(context.Background()))
	}
	return newDiagnosticsService(repo, configRepo, clusterService, monitorService, monitoringService)
}

func newDiagnosticsService(repo *Repository, configRepo *appconfig.Repository, clusterService clusterReader, monitorService processEventReader, monitoringService alertInstanceReader) *Service {
	svc := &Service{
		repo:              repo,
		configRepo:        configRepo,
		clusterService:    clusterService,
		monitorService:    monitorService,
		monitoringService: monitoringService,
		taskEvents:        newDiagnosticTaskEventHub(),
	}
	if repo != nil {
		svc.policyChecker = NewAutoPolicyChecker(repo, svc)
		if repo.db != nil {
			svc.troubleshooting = NewTroubleshootingService(NewTroubleshootingRepository(repo.db))
		}
	}
	return svc
}

// SetHostReader sets the optional host reader used to enrich diagnostic node targets.
// SetHostReader 设置可选的主机读取依赖，用于补全诊断节点目标的主机上下文。
func (s *Service) SetHostReader(reader hostReader) {
	if s == nil {
		return
	}
	s.hostService = reader
}

// SetAgentCommandSender sets the optional Agent command sender used by bundle execution.
// SetAgentCommandSender 设置可选的 Agent 命令发送依赖，用于诊断包执行。
func (s *Service) SetAgentCommandSender(sender diagnosticAgentCommandSender) {
	if s == nil {
		return
	}
	s.agentSender = sender
}

// GetWorkspaceBootstrap returns the diagnostics workspace skeleton and
// contextual cluster filter options.
// GetWorkspaceBootstrap 返回诊断中心工作台骨架及上下文集群筛选项。
func (s *Service) GetWorkspaceBootstrap(ctx context.Context, req *WorkspaceBootstrapRequest) (*WorkspaceBootstrapData, error) {
	result := &WorkspaceBootstrapData{
		GeneratedAt: time.Now().UTC(),
		DefaultTab:  WorkspaceTabErrors,
		Tabs: []*WorkspaceTab{
			{
				Key:         WorkspaceTabErrors,
				Label:       bilingualText("错误中心", "Error Center"),
				Description: bilingualText("追踪结构化 Seatunnel ERROR 分组及其关联上下文。", "Track structured Seatunnel ERROR groups and related context."),
			},
			{
				Key:         WorkspaceTabInspections,
				Label:       bilingualText("巡检中心", "Inspections"),
				Description: bilingualText("基于受管运行时信号发起并查看集群巡检。", "Run and review cluster inspections based on managed runtime signals."),
			},
		},
		ClusterOptions: make([]*ClusterOption, 0),
		Boundaries: []*WorkspaceBoundary{
			{
				Key:         "errors",
				Title:       bilingualText("错误证据", "Error Evidence"),
				Description: bilingualText("诊断中心负责维护 Seatunnel ERROR 证据，并关联集群 / 告警上下文。", "Diagnostics owns Seatunnel ERROR evidence and links to cluster / alert context."),
			},
			{
				Key:         "inspection",
				Title:       bilingualText("巡检信号", "Inspection Signals"),
				Description: bilingualText("诊断中心消费监控、进程事件与告警信号用于巡检。", "Diagnostics consumes monitoring, process events, and alert signals for inspections."),
			},
		},
	}

	if s.clusterService != nil {
		clusters, _, err := s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
		if err != nil {
			return nil, err
		}
		for _, item := range clusters {
			result.ClusterOptions = append(result.ClusterOptions, &ClusterOption{
				ClusterID:   item.ID,
				ClusterName: item.Name,
			})
		}
	}

	contextInfo := &WorkspaceContext{
		Source:  strings.TrimSpace(req.Source),
		AlertID: strings.TrimSpace(req.AlertID),
	}
	if req.ClusterID != nil {
		contextInfo.ClusterID = req.ClusterID
		if s.clusterService != nil {
			clusterInfo, err := s.clusterService.Get(ctx, *req.ClusterID)
			if err != nil {
				return nil, err
			}
			contextInfo.ClusterName = clusterInfo.Name
		}
	}
	if contextInfo.ClusterID != nil || contextInfo.Source != "" || contextInfo.AlertID != "" {
		result.EntryContext = contextInfo
	}
	return result, nil
}

// IngestSeatunnelError persists one structured Seatunnel ERROR event and updates grouping metadata.
// IngestSeatunnelError 持久化一条结构化 Seatunnel ERROR 事件并更新分组元数据。
func (s *Service) IngestSeatunnelError(ctx context.Context, req *IngestSeatunnelErrorRequest) error {
	if s == nil || s.repo == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	if req == nil {
		return ErrInvalidSeatunnelErrorRequest
	}
	if strings.TrimSpace(req.AgentID) == "" || strings.TrimSpace(req.InstallDir) == "" || strings.TrimSpace(req.Role) == "" || strings.TrimSpace(req.SourceFile) == "" {
		return fmt.Errorf("%w: missing agent/install_dir/role/source_file", ErrInvalidSeatunnelErrorRequest)
	}

	req.AgentID = strings.TrimSpace(req.AgentID)
	req.Role = strings.TrimSpace(req.Role)
	req.InstallDir = strings.TrimSpace(req.InstallDir)
	req.SourceFile = strings.TrimSpace(req.SourceFile)
	req.SourceKind = strings.TrimSpace(req.SourceKind)
	req.JobID = strings.TrimSpace(req.JobID)
	req.Message = truncateString(strings.TrimSpace(req.Message), maxErrorMessageLength)
	req.Evidence = truncateString(strings.TrimSpace(req.Evidence), maxErrorEvidenceLength)
	if req.OccurredAt.IsZero() {
		req.OccurredAt = time.Now()
	}
	req.OccurredAt = req.OccurredAt.UTC()
	if req.CursorEnd < req.CursorStart {
		req.CursorEnd = req.CursorStart
	}

	fingerprint, normalized, exceptionClass, title := BuildErrorFingerprint(req.Message, req.Evidence)
	txErr := s.repo.Transaction(ctx, func(tx *Repository) error {
		cursor, err := tx.GetLogCursor(ctx, req.AgentID, req.InstallDir, req.Role, req.SourceFile)
		if err != nil && !errors.Is(err, ErrSeatunnelLogCursorNotFound) {
			return err
		}
		if cursor != nil && req.CursorEnd > 0 && req.CursorEnd <= cursor.CursorOffset {
			if req.CursorEnd < cursor.CursorOffset {
				cursor.CursorOffset = 0
			} else {
				return nil
			}
		}

		cursorOffset := req.CursorEnd
		if cursorOffset <= 0 {
			cursorOffset = req.CursorStart
		}
		if fingerprint == "" {
			return tx.UpsertLogCursor(ctx, &SeatunnelLogCursor{
				AgentID:        req.AgentID,
				HostID:         req.HostID,
				ClusterID:      req.ClusterID,
				NodeID:         req.NodeID,
				InstallDir:     req.InstallDir,
				Role:           req.Role,
				SourceFile:     req.SourceFile,
				CursorOffset:   cursorOffset,
				LastOccurredAt: &req.OccurredAt,
			})
		}

		group, err := tx.GetErrorGroupByFingerprint(ctx, fingerprint)
		if err != nil {
			if !errors.Is(err, ErrSeatunnelErrorGroupNotFound) {
				return err
			}
			group = &SeatunnelErrorGroup{
				Fingerprint:        fingerprint,
				FingerprintVersion: DefaultFingerprintVersion,
				Title:              title,
				ExceptionClass:     exceptionClass,
				NormalizedText:     normalized,
				SampleMessage:      req.Message,
				SampleEvidence:     req.Evidence,
				OccurrenceCount:    1,
				FirstSeenAt:        req.OccurredAt,
				LastSeenAt:         req.OccurredAt,
				LastClusterID:      req.ClusterID,
				LastNodeID:         req.NodeID,
				LastHostID:         req.HostID,
			}
			if err := tx.CreateErrorGroup(ctx, group); err != nil {
				return err
			}
		} else {
			if group.FirstSeenAt.IsZero() || req.OccurredAt.Before(group.FirstSeenAt) {
				group.FirstSeenAt = req.OccurredAt
			}
			if group.LastSeenAt.IsZero() || req.OccurredAt.After(group.LastSeenAt) {
				group.LastSeenAt = req.OccurredAt
			}
			group.OccurrenceCount++
			if title != "" {
				group.Title = title
			}
			if exceptionClass != "" {
				group.ExceptionClass = exceptionClass
			}
			if normalized != "" {
				group.NormalizedText = normalized
			}
			if req.Message != "" {
				group.SampleMessage = req.Message
			}
			if req.Evidence != "" {
				group.SampleEvidence = req.Evidence
			}
			group.LastClusterID = req.ClusterID
			group.LastNodeID = req.NodeID
			group.LastHostID = req.HostID
			if err := tx.UpdateErrorGroup(ctx, group); err != nil {
				return err
			}
		}

		event := &SeatunnelErrorEvent{
			ErrorGroupID:   group.ID,
			Fingerprint:    fingerprint,
			ClusterID:      req.ClusterID,
			NodeID:         req.NodeID,
			HostID:         req.HostID,
			AgentID:        req.AgentID,
			Role:           req.Role,
			InstallDir:     req.InstallDir,
			SourceFile:     req.SourceFile,
			SourceKind:     req.SourceKind,
			JobID:          req.JobID,
			OccurredAt:     req.OccurredAt,
			Message:        req.Message,
			ExceptionClass: exceptionClass,
			NormalizedText: normalized,
			Evidence:       req.Evidence,
			CursorStart:    req.CursorStart,
			CursorEnd:      req.CursorEnd,
		}
		if err := tx.CreateErrorEvent(ctx, event); err != nil {
			return err
		}

		return tx.UpsertLogCursor(ctx, &SeatunnelLogCursor{
			AgentID:        req.AgentID,
			HostID:         req.HostID,
			ClusterID:      req.ClusterID,
			NodeID:         req.NodeID,
			InstallDir:     req.InstallDir,
			Role:           req.Role,
			SourceFile:     req.SourceFile,
			CursorOffset:   cursorOffset,
			LastOccurredAt: &req.OccurredAt,
		})
	})
	if txErr != nil {
		return txErr
	}

	// 异步自动策略检查（不阻塞错误入库）
	// Async auto-policy check (non-blocking, does not block error ingestion)
	if s.policyChecker != nil && s.clusterService != nil && req.ClusterID > 0 {
		clusterID := req.ClusterID
		ec := exceptionClass
		msg := req.Message
		occurredAt := req.OccurredAt
		go func() {
			if err := s.policyChecker.CheckJavaErrorTrigger(context.Background(), clusterID, ec, msg); err != nil {
				log.Printf("[DiagnosticsAutoPolicy] auto-policy check failed: cluster_id=%d err=%v", clusterID, err)
			}
			if err := s.policyChecker.CheckErrorSpikePolicies(context.Background(), clusterID, occurredAt); err != nil {
				log.Printf("[DiagnosticsAutoPolicy] error-spike check failed: cluster_id=%d err=%v", clusterID, err)
			}
		}()
	}

	return nil
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

// ListSeatunnelErrorGroups returns paginated error groups.
// ListSeatunnelErrorGroups 返回分页错误组列表。
func (s *Service) ListSeatunnelErrorGroups(ctx context.Context, filter *SeatunnelErrorGroupFilter) (*SeatunnelErrorGroupsData, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if filter == nil {
		filter = &SeatunnelErrorGroupFilter{}
	}
	groups, total, err := s.repo.ListErrorGroups(ctx, filter)
	if err != nil {
		return nil, err
	}
	page, pageSize := normalizePagination(filter.Page, filter.PageSize)
	items := make([]*SeatunnelErrorGroupInfo, 0, len(groups))
	for _, group := range groups {
		items = append(items, s.buildSeatunnelErrorGroupInfo(ctx, group))
	}
	return &SeatunnelErrorGroupsData{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// ListSeatunnelErrorEvents returns paginated error events.
// ListSeatunnelErrorEvents 返回分页错误事件列表。
func (s *Service) ListSeatunnelErrorEvents(ctx context.Context, filter *SeatunnelErrorEventFilter) (*SeatunnelErrorEventsData, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if filter == nil {
		filter = &SeatunnelErrorEventFilter{}
	}
	events, total, err := s.repo.ListErrorEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	page, pageSize := normalizePagination(filter.Page, filter.PageSize)
	items := make([]*SeatunnelErrorEventInfo, 0, len(events))
	for _, event := range events {
		items = append(items, s.buildSeatunnelErrorEventInfo(ctx, event))
	}
	return &SeatunnelErrorEventsData{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// GetSeatunnelErrorGroupDetail returns one group and its recent events.
// GetSeatunnelErrorGroupDetail 返回一个错误组及其近期事件。
func (s *Service) GetSeatunnelErrorGroupDetail(ctx context.Context, filter *SeatunnelErrorEventFilter, eventLimit int) (*SeatunnelErrorGroupDetailData, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if filter == nil || filter.ErrorGroupID == 0 {
		return nil, ErrInvalidSeatunnelErrorRequest
	}
	group, err := s.repo.GetErrorGroupByID(ctx, filter.ErrorGroupID)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.ListEventsByGroupID(ctx, filter, eventLimit)
	if err != nil {
		return nil, err
	}
	items := make([]*SeatunnelErrorEventInfo, 0, len(events))
	for _, event := range events {
		items = append(items, s.buildSeatunnelErrorEventInfo(ctx, event))
	}
	groupInfo := s.buildSeatunnelErrorGroupInfo(ctx, group)
	if groupInfo != nil && len(events) > 0 {
		latestEvent := events[0]
		groupInfo.LastClusterID = latestEvent.ClusterID
		groupInfo.LastNodeID = latestEvent.NodeID
		groupInfo.LastHostID = latestEvent.HostID
		groupInfo.LastSeenAt = latestEvent.OccurredAt
		if display := s.resolveDiagnosticHostDisplayContext(ctx, latestEvent.HostID); display != nil {
			groupInfo.LastHostName = display.HostName
			groupInfo.LastHostIP = display.HostIP
		}
	}
	return &SeatunnelErrorGroupDetailData{Group: groupInfo, Events: items}, nil
}

func (s *Service) buildSeatunnelErrorGroupInfo(ctx context.Context, group *SeatunnelErrorGroup) *SeatunnelErrorGroupInfo {
	if group == nil {
		return nil
	}
	return group.ToInfo(s.resolveDiagnosticHostDisplayContext(ctx, group.LastHostID))
}

func (s *Service) buildSeatunnelErrorEventInfo(ctx context.Context, event *SeatunnelErrorEvent) *SeatunnelErrorEventInfo {
	if event == nil {
		return nil
	}
	return event.ToInfo(s.resolveDiagnosticHostDisplayContext(ctx, event.HostID))
}

func (s *Service) resolveDiagnosticHostDisplayContext(ctx context.Context, hostID uint) *DiagnosticHostDisplayContext {
	if s == nil || s.hostService == nil || hostID == 0 {
		return nil
	}
	hostInfo, err := s.hostService.GetHostByID(ctx, hostID)
	if err != nil || hostInfo == nil {
		return nil
	}
	return &DiagnosticHostDisplayContext{
		HostName: strings.TrimSpace(hostInfo.Name),
		HostIP:   strings.TrimSpace(hostInfo.IPAddress),
	}
}

// Troubleshooting 返回排障经验记忆库服务实例
// Troubleshooting returns the troubleshooting memory service instance
func (s *Service) Troubleshooting() *TroubleshootingService {
	if s == nil {
		return nil
	}
	return s.troubleshooting
}

// ListTroubleshootingMemories 检索排障经验列表
// ListTroubleshootingMemories queries troubleshooting memories
func (s *Service) ListTroubleshootingMemories(ctx context.Context, query *TroubleshootingMemoryQuery) ([]*TroubleshootingMemoryItem, int64, error) {
	if s == nil || s.troubleshooting == nil {
		return nil, 0, ErrDiagnosticsRepositoryUnavailable
	}
	return s.troubleshooting.ListMemories(ctx, query)
}

// GetTroubleshootingMemory 根据 ID 获取单条排障经验
// GetTroubleshootingMemory retrieves single troubleshooting memory by ID
func (s *Service) GetTroubleshootingMemory(ctx context.Context, id uint) (*TroubleshootingMemoryItem, error) {
	if s == nil || s.troubleshooting == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	return s.troubleshooting.GetMemory(ctx, id)
}

// CreateTroubleshootingMemory 创建排障经验
// CreateTroubleshootingMemory creates a new troubleshooting memory
func (s *Service) CreateTroubleshootingMemory(ctx context.Context, req *CreateTroubleshootingMemoryRequest) (*TroubleshootingMemoryItem, error) {
	if s == nil || s.troubleshooting == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	return s.troubleshooting.CreateMemory(ctx, req)
}

// UpdateTroubleshootingMemory 更新排障经验
// UpdateTroubleshootingMemory updates an existing troubleshooting memory
func (s *Service) UpdateTroubleshootingMemory(ctx context.Context, id uint, req *UpdateTroubleshootingMemoryRequest) (*TroubleshootingMemoryItem, error) {
	if s == nil || s.troubleshooting == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	return s.troubleshooting.UpdateMemory(ctx, id, req)
}

// DeleteTroubleshootingMemory 删除排障经验
// DeleteTroubleshootingMemory deletes a troubleshooting memory by ID
func (s *Service) DeleteTroubleshootingMemory(ctx context.Context, id uint) error {
	if s == nil || s.troubleshooting == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return s.troubleshooting.DeleteMemory(ctx, id)
}
