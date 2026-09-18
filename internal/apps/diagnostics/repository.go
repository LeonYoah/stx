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
	"strings"
	"time"

	"gorm.io/gorm"
)

// Repository provides persistence for diagnostics domain models.
// Repository 为诊断领域模型提供持久化能力。
type Repository struct {
	db *gorm.DB
}

type recentErrorGroupBurst struct {
	ErrorGroupID uint                 `gorm:"column:error_group_id"`
	RecentCount  int64                `gorm:"column:recent_count"`
	Group        *SeatunnelErrorGroup `gorm:"-"`
}

// NewRepository creates a diagnostics repository.
// NewRepository 创建诊断仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func applyOccurredAtLowerBound(query *gorm.DB, column string, value time.Time) *gorm.DB {
	if query == nil || value.IsZero() {
		return query
	}
	value = value.UTC()
	if strings.EqualFold(query.Dialector.Name(), "sqlite") {
		return query.Where("julianday("+column+") >= julianday(?)", value.Format(time.RFC3339Nano))
	}
	return query.Where(column+" >= ?", value)
}

func applyOccurredAtUpperBound(query *gorm.DB, column string, value time.Time) *gorm.DB {
	if query == nil || value.IsZero() {
		return query
	}
	value = value.UTC()
	if strings.EqualFold(query.Dialector.Name(), "sqlite") {
		return query.Where("julianday("+column+") <= julianday(?)", value.Format(time.RFC3339Nano))
	}
	return query.Where(column+" <= ?", value)
}

// Transaction executes diagnostics writes in a single transaction.
// Transaction 在单个事务中执行诊断写操作。
func (r *Repository) Transaction(ctx context.Context, fn func(tx *Repository) error) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

// GetErrorGroupByFingerprint retrieves one error group by fingerprint.
// GetErrorGroupByFingerprint 根据指纹获取一个错误组。
func (r *Repository) GetErrorGroupByFingerprint(ctx context.Context, fingerprint string) (*SeatunnelErrorGroup, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var group SeatunnelErrorGroup
	if err := r.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSeatunnelErrorGroupNotFound
		}
		return nil, err
	}
	return &group, nil
}

// CreateErrorGroup persists one new error group.
// CreateErrorGroup 持久化一个新的错误组。
func (r *Repository) CreateErrorGroup(ctx context.Context, group *SeatunnelErrorGroup) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Create(group).Error
}

// UpdateErrorGroup updates one existing error group.
// UpdateErrorGroup 更新一个已有错误组。
func (r *Repository) UpdateErrorGroup(ctx context.Context, group *SeatunnelErrorGroup) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Save(group).Error
}

// CreateErrorEvent persists one new error event.
// CreateErrorEvent 持久化一个新的错误事件。
func (r *Repository) CreateErrorEvent(ctx context.Context, event *SeatunnelErrorEvent) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Create(event).Error
}

// GetLogCursor gets one cursor by agent + install_dir + role + source_file.
// GetLogCursor 根据 agent + install_dir + role + source_file 获取一个游标。
func (r *Repository) GetLogCursor(ctx context.Context, agentID, installDir, role, sourceFile string) (*SeatunnelLogCursor, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var cursor SeatunnelLogCursor
	if err := r.db.WithContext(ctx).
		Where("agent_id = ? AND install_dir = ? AND role = ? AND source_file = ?", agentID, installDir, role, sourceFile).
		First(&cursor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSeatunnelLogCursorNotFound
		}
		return nil, err
	}
	return &cursor, nil
}

// UpsertLogCursor creates or updates one source file cursor.
// UpsertLogCursor 创建或更新一个来源文件游标。
func (r *Repository) UpsertLogCursor(ctx context.Context, cursor *SeatunnelLogCursor) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}

	existing, err := r.GetLogCursor(ctx, cursor.AgentID, cursor.InstallDir, cursor.Role, cursor.SourceFile)
	if err != nil && !errors.Is(err, ErrSeatunnelLogCursorNotFound) {
		return err
	}
	if existing == nil {
		return r.db.WithContext(ctx).Create(cursor).Error
	}

	existing.HostID = cursor.HostID
	existing.ClusterID = cursor.ClusterID
	existing.NodeID = cursor.NodeID
	existing.CursorOffset = cursor.CursorOffset
	existing.LastOccurredAt = cursor.LastOccurredAt
	return r.db.WithContext(ctx).Save(existing).Error
}

// ListLogCursorsByAgent lists all log cursors for a given agent.
// ListLogCursorsByAgent 返回某个 Agent 的所有日志游标。
func (r *Repository) ListLogCursorsByAgent(ctx context.Context, agentID string) ([]*SeatunnelLogCursor, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var rows []*SeatunnelLogCursor
	if err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetErrorGroupByID retrieves one error group by identifier.
// GetErrorGroupByID 根据标识获取一个错误组。
func (r *Repository) GetErrorGroupByID(ctx context.Context, id uint) (*SeatunnelErrorGroup, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var group SeatunnelErrorGroup
	if err := r.db.WithContext(ctx).First(&group, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSeatunnelErrorGroupNotFound
		}
		return nil, err
	}
	return &group, nil
}

// ListErrorGroups lists aggregated error groups with optional event-scoped filters.
// ListErrorGroups 返回聚合后的错误组列表，支持事件维度过滤。
func (r *Repository) ListErrorGroups(ctx context.Context, filter *SeatunnelErrorGroupFilter) ([]*SeatunnelErrorGroup, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrDiagnosticsRepositoryUnavailable
	}

	query := r.db.WithContext(ctx).Model(&SeatunnelErrorGroup{})
	if filter == nil {
		filter = &SeatunnelErrorGroupFilter{}
	}
	query = r.applyGroupFilters(query, filter)
	if hasEventScopedGroupFilters(filter) {
		eventQuery := r.db.WithContext(ctx).Model(&SeatunnelErrorEvent{}).Select("distinct error_group_id")
		eventQuery = applyEventFilters(eventQuery, &SeatunnelErrorEventFilter{
			ClusterID:      filter.ClusterID,
			NodeID:         filter.NodeID,
			HostID:         filter.HostID,
			Role:           filter.Role,
			JobID:          filter.JobID,
			Keyword:        filter.Keyword,
			ExceptionClass: filter.ExceptionClass,
			StartTime:      filter.StartTime,
			EndTime:        filter.EndTime,
		})
		query = query.Where("id IN (?)", eventQuery)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePagination(filter.Page, filter.PageSize)
	var groups []*SeatunnelErrorGroup
	if err := query.Order("last_seen_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&groups).Error; err != nil {
		return nil, 0, err
	}
	return groups, total, nil
}

// ListErrorEvents lists error events with filters.
// ListErrorEvents 返回带过滤条件的错误事件列表。
func (r *Repository) ListErrorEvents(ctx context.Context, filter *SeatunnelErrorEventFilter) ([]*SeatunnelErrorEvent, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrDiagnosticsRepositoryUnavailable
	}
	if filter == nil {
		filter = &SeatunnelErrorEventFilter{}
	}

	query := applyEventFilters(r.db.WithContext(ctx).Model(&SeatunnelErrorEvent{}), filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePagination(filter.Page, filter.PageSize)
	var events []*SeatunnelErrorEvent
	if err := query.Order("occurred_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// ListEventsByGroupID lists recent events for one error group.
// ListEventsByGroupID 返回一个错误组的近期事件。
func (r *Repository) ListEventsByGroupID(ctx context.Context, filter *SeatunnelErrorEventFilter, limit int) ([]*SeatunnelErrorEvent, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if filter == nil || filter.ErrorGroupID == 0 {
		return nil, ErrInvalidSeatunnelErrorRequest
	}
	if limit <= 0 {
		limit = 20
	}
	query := applyEventFilters(r.db.WithContext(ctx).Model(&SeatunnelErrorEvent{}), filter)
	var events []*SeatunnelErrorEvent
	if err := query.Order("occurred_at DESC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// ListRecentErrorGroupBursts aggregates recent error events by group within one cluster.
// ListRecentErrorGroupBursts 返回单个集群在给定时间窗口内按错误组聚合的近期错误突增结果。
func (r *Repository) ListRecentErrorGroupBursts(ctx context.Context, clusterID uint, since time.Time, limit int) ([]*recentErrorGroupBurst, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if clusterID == 0 {
		return []*recentErrorGroupBurst{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	since = since.UTC()
	query := r.db.WithContext(ctx).
		Table((&SeatunnelErrorEvent{}).TableName()+" AS events").
		Select("events.error_group_id AS error_group_id, COUNT(*) AS recent_count").
		Where("events.cluster_id = ?", clusterID)
	query = applyOccurredAtLowerBound(query, "events.occurred_at", since)
	var rows []*recentErrorGroupBurst
	if err := query.
		Group("events.error_group_id").
		Order("recent_count DESC").
		Order("MAX(events.occurred_at) DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []*recentErrorGroupBurst{}, nil
	}

	groupIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.ErrorGroupID == 0 {
			continue
		}
		groupIDs = append(groupIDs, row.ErrorGroupID)
	}
	if len(groupIDs) == 0 {
		return []*recentErrorGroupBurst{}, nil
	}

	var groups []*SeatunnelErrorGroup
	if err := r.db.WithContext(ctx).Where("id IN ?", groupIDs).Find(&groups).Error; err != nil {
		return nil, err
	}
	groupMap := make(map[uint]*SeatunnelErrorGroup, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		groupMap[group.ID] = group
	}

	results := make([]*recentErrorGroupBurst, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		row.Group = groupMap[row.ErrorGroupID]
		if row.Group == nil {
			continue
		}
		results = append(results, row)
	}
	return results, nil
}

// CreateInspectionReport persists one new inspection report.
// CreateInspectionReport 持久化一条新的巡检报告。
func (r *Repository) CreateInspectionReport(ctx context.Context, report *ClusterInspectionReport) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Create(report).Error
}

// UpdateInspectionReport updates one existing inspection report.
// UpdateInspectionReport 更新一条已有巡检报告。
func (r *Repository) UpdateInspectionReport(ctx context.Context, report *ClusterInspectionReport) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Save(report).Error
}

// GetInspectionReportByID retrieves one inspection report by identifier.
// GetInspectionReportByID 根据标识获取一条巡检报告。
func (r *Repository) GetInspectionReportByID(ctx context.Context, id uint) (*ClusterInspectionReport, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var report ClusterInspectionReport
	if err := r.db.WithContext(ctx).First(&report, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInspectionReportNotFound
		}
		return nil, err
	}
	return &report, nil
}

// GetInspectionFindingByID retrieves one inspection finding by identifier.
// GetInspectionFindingByID 根据标识获取一条巡检发现项。
func (r *Repository) GetInspectionFindingByID(ctx context.Context, id uint) (*ClusterInspectionFinding, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var finding ClusterInspectionFinding
	if err := r.db.WithContext(ctx).First(&finding, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInspectionFindingNotFound
		}
		return nil, err
	}
	return &finding, nil
}

// ListInspectionReports lists inspection reports with filters.
// ListInspectionReports 返回带过滤条件的巡检报告列表。
func (r *Repository) ListInspectionReports(ctx context.Context, filter *ClusterInspectionReportFilter) ([]*ClusterInspectionReport, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrDiagnosticsRepositoryUnavailable
	}
	if filter == nil {
		filter = &ClusterInspectionReportFilter{}
	}

	query := applyInspectionReportFilters(r.db.WithContext(ctx).Model(&ClusterInspectionReport{}), filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePagination(filter.Page, filter.PageSize)
	var reports []*ClusterInspectionReport
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&reports).Error; err != nil {
		return nil, 0, err
	}
	return reports, total, nil
}

// CreateInspectionFinding persists one new inspection finding.
// CreateInspectionFinding 持久化一条新的巡检发现项。
func (r *Repository) CreateInspectionFinding(ctx context.Context, finding *ClusterInspectionFinding) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Create(finding).Error
}

// CreateInspectionFindings persists multiple inspection findings.
// CreateInspectionFindings 持久化多条巡检发现项。
func (r *Repository) CreateInspectionFindings(ctx context.Context, findings []*ClusterInspectionFinding) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	if len(findings) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&findings).Error
}

// ListInspectionFindings lists inspection findings with filters.
// ListInspectionFindings 返回带过滤条件的巡检发现项列表。
func (r *Repository) ListInspectionFindings(ctx context.Context, filter *ClusterInspectionFindingFilter) ([]*ClusterInspectionFinding, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrDiagnosticsRepositoryUnavailable
	}
	if filter == nil {
		filter = &ClusterInspectionFindingFilter{}
	}

	query := applyInspectionFindingFilters(r.db.WithContext(ctx).Model(&ClusterInspectionFinding{}), filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePagination(filter.Page, filter.PageSize)
	var findings []*ClusterInspectionFinding
	if err := query.Order("severity DESC, id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&findings).Error; err != nil {
		return nil, 0, err
	}
	return findings, total, nil
}

// ListInspectionFindingsByReportID lists inspection findings for one report.
// ListInspectionFindingsByReportID 返回一条巡检报告下的发现项列表。
func (r *Repository) ListInspectionFindingsByReportID(ctx context.Context, reportID uint) ([]*ClusterInspectionFinding, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var findings []*ClusterInspectionFinding
	if err := r.db.WithContext(ctx).
		Where("report_id = ?", reportID).
		Order("severity DESC, id ASC").
		Find(&findings).Error; err != nil {
		return nil, err
	}
	return findings, nil
}

func (r *Repository) applyGroupFilters(query *gorm.DB, filter *SeatunnelErrorGroupFilter) *gorm.DB {
	if filter == nil {
		return query
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		// 使用 LOWER 进行大小写不敏感模糊匹配，兼容多数据库。
		// Use LOWER for case-insensitive fuzzy matching, compatible with multiple databases.
		query = query.Where("LOWER(title) LIKE LOWER(?) OR LOWER(sample_message) LIKE LOWER(?) OR LOWER(exception_class) LIKE LOWER(?)", like, like, like)
	}
	if exceptionClass := strings.TrimSpace(filter.ExceptionClass); exceptionClass != "" {
		// 使用 LOWER 进行大小写不敏感异常类匹配，兼容多数据库。
		// Use LOWER for case-insensitive exception class matching, compatible with multiple databases.
		query = query.Where("LOWER(exception_class) LIKE LOWER(?)", "%"+exceptionClass+"%")
	}
	return query
}

func applyEventFilters(query *gorm.DB, filter *SeatunnelErrorEventFilter) *gorm.DB {
	if filter == nil {
		return query
	}
	if filter.ErrorGroupID > 0 {
		query = query.Where("error_group_id = ?", filter.ErrorGroupID)
	}
	if filter.ClusterID > 0 {
		query = query.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.NodeID > 0 {
		query = query.Where("node_id = ?", filter.NodeID)
	}
	if filter.HostID > 0 {
		query = query.Where("host_id = ?", filter.HostID)
	}
	if role := strings.TrimSpace(filter.Role); role != "" {
		query = query.Where("role = ?", role)
	}
	if jobID := strings.TrimSpace(filter.JobID); jobID != "" {
		query = query.Where("job_id = ?", jobID)
	}
	if filter.StartTime != nil {
		query = applyOccurredAtLowerBound(query, "occurred_at", *filter.StartTime)
	}
	if filter.EndTime != nil {
		query = applyOccurredAtUpperBound(query, "occurred_at", *filter.EndTime)
	}
	if exceptionClass := strings.TrimSpace(filter.ExceptionClass); exceptionClass != "" {
		// 使用 LOWER 进行大小写不敏感异常类匹配，兼容多数据库。
		// Use LOWER for case-insensitive exception class matching, compatible with multiple databases.
		query = query.Where("LOWER(exception_class) LIKE LOWER(?)", "%"+exceptionClass+"%")
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		// 使用 LOWER 进行多字段大小写不敏感模糊匹配，兼容多数据库。
		// Use LOWER for multi-field case-insensitive fuzzy matching, compatible with multiple databases.
		query = query.Where("LOWER(message) LIKE LOWER(?) OR LOWER(evidence) LIKE LOWER(?) OR LOWER(source_file) LIKE LOWER(?) OR LOWER(exception_class) LIKE LOWER(?)", like, like, like, like)
	}
	return query
}

func hasEventScopedGroupFilters(filter *SeatunnelErrorGroupFilter) bool {
	if filter == nil {
		return false
	}
	return filter.ClusterID > 0 || filter.NodeID > 0 || filter.HostID > 0 || strings.TrimSpace(filter.Role) != "" || strings.TrimSpace(filter.JobID) != "" || filter.StartTime != nil || filter.EndTime != nil
}

func applyInspectionReportFilters(query *gorm.DB, filter *ClusterInspectionReportFilter) *gorm.DB {
	if filter == nil {
		return query
	}
	if filter.ClusterID > 0 {
		query = query.Where("cluster_id = ?", filter.ClusterID)
	}
	if status := strings.TrimSpace(string(filter.Status)); status != "" {
		query = query.Where("status = ?", status)
	}
	if triggerSource := strings.TrimSpace(string(filter.TriggerSource)); triggerSource != "" {
		query = query.Where("trigger_source = ?", triggerSource)
	}
	if severity := strings.TrimSpace(string(filter.Severity)); severity != "" {
		findingQuery := query.Session(&gorm.Session{NewDB: true}).
			Model(&ClusterInspectionFinding{}).
			Select("distinct report_id").
			Where("severity = ?", severity)
		query = query.Where("id IN (?)", findingQuery)
	}
	if filter.StartTime != nil {
		query = query.Where("created_at >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		query = query.Where("created_at <= ?", *filter.EndTime)
	}
	return query
}

func applyInspectionFindingFilters(query *gorm.DB, filter *ClusterInspectionFindingFilter) *gorm.DB {
	if filter == nil {
		return query
	}
	if filter.ReportID > 0 {
		query = query.Where("report_id = ?", filter.ReportID)
	}
	if filter.ClusterID > 0 {
		query = query.Where("cluster_id = ?", filter.ClusterID)
	}
	if severity := strings.TrimSpace(string(filter.Severity)); severity != "" {
		query = query.Where("severity = ?", severity)
	}
	if category := strings.TrimSpace(filter.Category); category != "" {
		query = query.Where("category = ?", category)
	}
	if filter.RelatedNodeID > 0 {
		query = query.Where("related_node_id = ?", filter.RelatedNodeID)
	}
	if filter.RelatedHostID > 0 {
		query = query.Where("related_host_id = ?", filter.RelatedHostID)
	}
	if filter.RelatedErrorGroupID > 0 {
		query = query.Where("related_error_group_id = ?", filter.RelatedErrorGroupID)
	}
	return query
}

func normalizePagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}
