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

package monitoring

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/monitor"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AlertEventQueryFilter defines query options for alert events.
// AlertEventQueryFilter 定义告警事件查询条件。
type AlertEventQueryFilter struct {
	ClusterID uint
	StartTime *time.Time
	EndTime   *time.Time
	Page      int
	PageSize  int
}

// AlertEventSource represents joined alert event rows from DB.
// AlertEventSource 表示从 DB 联表查询出的告警事件记录。
type AlertEventSource struct {
	ID          uint                     `json:"id"`
	ClusterID   uint                     `json:"cluster_id"`
	ClusterName string                   `json:"cluster_name"`
	NodeID      uint                     `json:"node_id"`
	HostID      uint                     `json:"host_id"`
	EventType   monitor.ProcessEventType `json:"event_type"`
	PID         int                      `json:"pid"`
	ProcessName string                   `json:"process_name"`
	InstallDir  string                   `json:"install_dir"`
	Role        string                   `json:"role"`
	Details     string                   `json:"details"`
	CreatedAt   time.Time                `json:"created_at"`
	Hostname    string                   `json:"hostname"`
	IP          string                   `json:"ip"`
}

// RemoteAlertClusterStat represents aggregated remote alert counters per cluster.
// RemoteAlertClusterStat 表示按集群聚合的远程告警计数。
type RemoteAlertClusterStat struct {
	ClusterID     string `json:"cluster_id"`
	ActiveCount   int64  `json:"active_count"`
	CriticalCount int64  `json:"critical_count"`
}

// Repository provides data access for monitoring center.
// Repository 提供监控中心的数据访问能力。
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new monitoring repository.
// NewRepository 创建监控中心仓库。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListCriticalEvents returns critical process events joined with host and cluster info.
// ListCriticalEvents 返回关键进程事件（含主机和集群信息）。
func (r *Repository) ListCriticalEvents(ctx context.Context, filter *AlertEventQueryFilter) ([]*AlertEventSource, int64, error) {
	if filter == nil {
		filter = &AlertEventQueryFilter{}
	}

	criticalTypes := alertableProcessEventTypes()

	countQuery := r.db.WithContext(ctx).
		Table("process_events").
		Where("process_events.event_type IN ?", criticalTypes)

	if filter.ClusterID > 0 {
		countQuery = countQuery.Where("process_events.cluster_id = ?", filter.ClusterID)
	}
	if filter.StartTime != nil {
		countQuery = countQuery.Where("process_events.created_at >= ?", filter.StartTime)
	}
	if filter.EndTime != nil {
		countQuery = countQuery.Where("process_events.created_at <= ?", filter.EndTime)
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := r.db.WithContext(ctx).
		Table("process_events").
		Select("process_events.*, hosts.name AS hostname, hosts.ip_address AS ip, clusters.name AS cluster_name").
		Joins("LEFT JOIN hosts ON process_events.host_id = hosts.id").
		Joins("LEFT JOIN clusters ON process_events.cluster_id = clusters.id").
		Where("process_events.event_type IN ?", criticalTypes)

	if filter.ClusterID > 0 {
		query = query.Where("process_events.cluster_id = ?", filter.ClusterID)
	}
	if filter.StartTime != nil {
		query = query.Where("process_events.created_at >= ?", filter.StartTime)
	}
	if filter.EndTime != nil {
		query = query.Where("process_events.created_at <= ?", filter.EndTime)
	}

	if filter.Page > 0 && filter.PageSize > 0 {
		offset := (filter.Page - 1) * filter.PageSize
		query = query.Offset(offset).Limit(filter.PageSize)
	}

	query = query.Order("process_events.created_at DESC")

	var rows []*AlertEventSource
	if err := query.Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// ListRulesByClusterID returns alert rules for one cluster.
// ListRulesByClusterID 返回指定集群的告警规则。
func (r *Repository) ListRulesByClusterID(ctx context.Context, clusterID uint) ([]*AlertRule, error) {
	var rules []*AlertRule
	err := r.db.WithContext(ctx).
		Where("cluster_id = ?", clusterID).
		Order("id ASC").
		Find(&rules).Error
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// GetRuleByID gets one alert rule by ID.
// GetRuleByID 根据 ID 获取单条告警规则。
func (r *Repository) GetRuleByID(ctx context.Context, id uint) (*AlertRule, error) {
	var rule AlertRule
	err := r.db.WithContext(ctx).First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetRuleByClusterAndKey gets one rule by cluster and rule key.
// GetRuleByClusterAndKey 根据集群和规则键获取规则。
func (r *Repository) GetRuleByClusterAndKey(ctx context.Context, clusterID uint, ruleKey string) (*AlertRule, error) {
	var rule AlertRule
	err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND rule_key = ?", clusterID, ruleKey).
		First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// CreateRule creates one alert rule.
// CreateRule 创建告警规则。
func (r *Repository) CreateRule(ctx context.Context, rule *AlertRule) error {
	return r.db.WithContext(ctx).Create(rule).Error
}

// SaveRule updates one alert rule.
// SaveRule 更新告警规则。
func (r *Repository) SaveRule(ctx context.Context, rule *AlertRule) error {
	return r.db.WithContext(ctx).Save(rule).Error
}

// GetEventStateByEventID returns state for one event. Nil if not found.
// GetEventStateByEventID 返回单个事件状态，未找到返回 nil。
func (r *Repository) GetEventStateByEventID(ctx context.Context, eventID uint) (*AlertEventState, error) {
	var state AlertEventState
	err := r.db.WithContext(ctx).Where("event_id = ?", eventID).First(&state).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

// SaveEventState creates or updates alert event state.
// SaveEventState 创建或更新告警事件状态。
func (r *Repository) SaveEventState(ctx context.Context, state *AlertEventState) error {
	if state == nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("event_id = ?", state.EventID).
		Assign(map[string]interface{}{
			"cluster_id":      state.ClusterID,
			"status":          state.Status,
			"acknowledged_by": state.AcknowledgedBy,
			"acknowledged_at": state.AcknowledgedAt,
			"silenced_by":     state.SilencedBy,
			"silenced_until":  state.SilencedUntil,
			"note":            state.Note,
			"updated_at":      time.Now().UTC(),
		}).
		FirstOrCreate(&AlertEventState{EventID: state.EventID}).Error
}

// ListEventStatesByEventIDs returns states mapped by event_id.
// ListEventStatesByEventIDs 根据事件 ID 列表返回状态映射。
func (r *Repository) ListEventStatesByEventIDs(ctx context.Context, eventIDs []uint) (map[uint]*AlertEventState, error) {
	result := make(map[uint]*AlertEventState)
	if len(eventIDs) == 0 {
		return result, nil
	}

	var states []*AlertEventState
	if err := r.db.WithContext(ctx).
		Where("event_id IN ?", eventIDs).
		Find(&states).Error; err != nil {
		return nil, err
	}

	for _, s := range states {
		result[s.EventID] = s
	}
	return result, nil
}

// ListNotificationChannels returns all notification channels.
// ListNotificationChannels 返回全部通知渠道。
func (r *Repository) ListNotificationChannels(ctx context.Context) ([]*NotificationChannel, error) {
	var channels []*NotificationChannel
	if err := r.db.WithContext(ctx).
		Order("id DESC").
		Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

// GetNotificationChannelByID returns one channel by ID.
// GetNotificationChannelByID 根据 ID 返回单条通知渠道。
func (r *Repository) GetNotificationChannelByID(ctx context.Context, id uint) (*NotificationChannel, error) {
	var channel NotificationChannel
	if err := r.db.WithContext(ctx).First(&channel, id).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// CreateNotificationChannel creates a channel.
// CreateNotificationChannel 创建通知渠道。
func (r *Repository) CreateNotificationChannel(ctx context.Context, channel *NotificationChannel) error {
	return r.db.WithContext(ctx).Select("*").Create(channel).Error
}

// SaveNotificationChannel updates a channel.
// SaveNotificationChannel 更新通知渠道。
func (r *Repository) SaveNotificationChannel(ctx context.Context, channel *NotificationChannel) error {
	return r.db.WithContext(ctx).Save(channel).Error
}

// DeleteNotificationChannel deletes channel by ID.
// DeleteNotificationChannel 根据 ID 删除通知渠道。
func (r *Repository) DeleteNotificationChannel(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&NotificationChannel{}, id).Error
}

// UpsertRemoteAlert creates or updates one normalized remote alert record.
// UpsertRemoteAlert 创建或更新一条标准化远程告警记录。
func (r *Repository) UpsertRemoteAlert(ctx context.Context, record *RemoteAlertRecord) error {
	if record == nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "fingerprint"},
				{Name: "starts_at"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":           record.Status,
				"receiver":         record.Receiver,
				"alert_name":       record.AlertName,
				"severity":         record.Severity,
				"cluster_id":       record.ClusterID,
				"cluster_name":     record.ClusterName,
				"env":              record.Env,
				"generator_url":    record.GeneratorURL,
				"summary":          record.Summary,
				"description":      record.Description,
				"labels_json":      record.LabelsJSON,
				"annotations_json": record.AnnotationsJSON,
				"ends_at":          record.EndsAt,
				"resolved_at":      record.ResolvedAt,
				"last_received_at": record.LastReceivedAt,
				"updated_at":       time.Now().UTC(),
			}),
		}).
		Create(record).Error
}

// ListRemoteAlerts returns remote alert records with pagination and filters.
// ListRemoteAlerts 返回远程告警记录列表（支持分页与过滤）。
func (r *Repository) ListRemoteAlerts(ctx context.Context, filter *RemoteAlertFilter) ([]*RemoteAlertRecord, int64, error) {
	if filter == nil {
		filter = &RemoteAlertFilter{}
	}

	query := r.db.WithContext(ctx).Model(&RemoteAlertRecord{})
	if filter.ClusterID != "" {
		query = query.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.Status != "" {
		query = query.Where("LOWER(status) = ?", strings.ToLower(filter.Status))
	}
	if filter.StartTime != nil {
		query = query.Where("starts_at >= ?", filter.StartTime.Unix())
	}
	if filter.EndTime != nil {
		query = query.Where("starts_at <= ?", filter.EndTime.Unix())
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if filter.Page > 0 && filter.PageSize > 0 {
		offset := (filter.Page - 1) * filter.PageSize
		query = query.Offset(offset).Limit(filter.PageSize)
	}

	var rows []*RemoteAlertRecord
	if err := query.Order("starts_at DESC").Order("id DESC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetRemoteAlertClusterStats returns active/critical remote alert counters grouped by cluster_id.
// GetRemoteAlertClusterStats 返回按 cluster_id 聚合的远程告警活动/严重数。
func (r *Repository) GetRemoteAlertClusterStats(ctx context.Context, clusterIDs []string) (map[string]*RemoteAlertClusterStat, error) {
	result := make(map[string]*RemoteAlertClusterStat)
	if len(clusterIDs) == 0 {
		return result, nil
	}

	query := r.db.WithContext(ctx).Table((&RemoteAlertRecord{}).TableName()).
		Select(
			"cluster_id, "+
				"SUM(CASE WHEN LOWER(status) <> 'resolved' THEN 1 ELSE 0 END) AS active_count, "+
				"SUM(CASE WHEN LOWER(status) <> 'resolved' AND LOWER(severity) = 'critical' THEN 1 ELSE 0 END) AS critical_count",
		).
		Where("cluster_id IN ?", clusterIDs).
		Group("cluster_id")

	var rows []*RemoteAlertClusterStat
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		result[row.ClusterID] = row
	}
	return result, nil
}
