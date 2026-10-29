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
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// SyncAlertEpisodeScope 区分作业级（按任务）与集群级的告警阶段。
// SyncAlertEpisodeScope separates job-level (per task) from cluster-level alert episodes.
type SyncAlertEpisodeScope string

const (
	// SyncAlertEpisodeScopeTask 是作业级阶段，按"集群 + 任务 + 规则"区分。
	// SyncAlertEpisodeScopeTask is a job-level episode keyed by cluster + task + rule.
	SyncAlertEpisodeScopeTask SyncAlertEpisodeScope = "task"
	// SyncAlertEpisodeScopeCluster 是集群级阶段，按"集群 + 规则"区分。
	// SyncAlertEpisodeScopeCluster is a cluster-level episode keyed by cluster + rule.
	SyncAlertEpisodeScopeCluster SyncAlertEpisodeScope = "cluster"
)

// SyncAlertEpisodeStatus 是告警阶段的状态。
// SyncAlertEpisodeStatus is the status of an alert episode.
type SyncAlertEpisodeStatus string

const (
	// SyncAlertEpisodeFiring 表示问题仍在。
	// SyncAlertEpisodeFiring means the problem still exists.
	SyncAlertEpisodeFiring SyncAlertEpisodeStatus = "firing"
	// SyncAlertEpisodeResolved 表示问题已恢复，或者阶段已被关闭、策略已删除。
	// SyncAlertEpisodeResolved means the problem recovered, or the episode was closed or its policy deleted.
	SyncAlertEpisodeResolved SyncAlertEpisodeStatus = "resolved"
)

// 阶段结束的原因。
// Reasons an episode ends.
const (
	syncAlertResolveRecovered     = "recovered"
	syncAlertResolveClosed        = "closed"
	syncAlertResolvePolicyDeleted = "policy_deleted"
)

// SyncAlertEpisode 是一次同步作业告警阶段：同一个键第一次出问题时建立，之后只累计次数，恢复或关闭时结束。
// 同一个键同一时间最多一个 firing 阶段，由 SyncAlertHub 的单个 worker 串行"先查后写"保证。
// SyncAlertEpisode is one sync job alert episode: it opens on the first problem for a key, then only counts
// occurrences, and ends on recovery or close. At most one firing episode exists per key at a time, guaranteed by
// the single SyncAlertHub worker doing find-then-write serially.
type SyncAlertEpisode struct {
	ID                uint                   `json:"id" gorm:"primaryKey;autoIncrement"`
	Scope             SyncAlertEpisodeScope  `json:"scope" gorm:"size:20;not null;index:idx_sync_alert_episode_key,priority:1"`
	ClusterID         uint                   `json:"cluster_id" gorm:"not null;default:0;index:idx_sync_alert_episode_key,priority:2"`
	TaskID            uint                   `json:"task_id" gorm:"not null;default:0;index:idx_sync_alert_episode_key,priority:3"`
	RuleKey           string                 `json:"rule_key" gorm:"size:80;not null;index:idx_sync_alert_episode_key,priority:4"`
	Status            SyncAlertEpisodeStatus `json:"status" gorm:"size:20;not null;index"`
	Severity          AlertSeverity          `json:"severity" gorm:"size:20"`
	ClusterName       string                 `json:"cluster_name" gorm:"size:255"`
	TaskName          string                 `json:"task_name" gorm:"size:255"`
	TaskCreatorID     uint                   `json:"task_creator_id" gorm:"not null;default:0"`
	OccurrenceCount   int                    `json:"occurrence_count" gorm:"not null;default:1"`
	FirstFiredAt      time.Time              `json:"first_fired_at" gorm:"index"`
	LastOccurredAt    time.Time              `json:"last_occurred_at"`
	LastJobID         uint                   `json:"last_job_id" gorm:"not null;default:0"`
	LastEngineJobID   string                 `json:"last_engine_job_id" gorm:"size:64"`
	LastTriggerSource string                 `json:"last_trigger_source" gorm:"size:40"`
	LastMessage       string                 `json:"last_message" gorm:"type:text"`
	// LastDetail 是最近一次信号带的结构化明细（JSON），目前只有 B4 的 syncalert.CDCDelayDetail；新增列，旧版本忽略。
	// LastDetail is the structured detail (JSON) of the latest signal, currently only B4's syncalert.CDCDelayDetail;
	// an added column that older versions ignore.
	LastDetail    string     `json:"last_detail,omitempty" gorm:"type:text"`
	ResolvedAt    *time.Time `json:"resolved_at" gorm:"index"`
	ResolveReason string     `json:"resolve_reason" gorm:"size:40"`
	CreatedAt     time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定告警阶段表名。
// TableName specifies the alert episode table name.
func (SyncAlertEpisode) TableName() string {
	return "monitoring_sync_alert_episodes"
}

// buildSyncAlertSourceKey 返回告警阶段在告警中心、投递记录和人工处理状态里使用的来源键。
// buildSyncAlertSourceKey returns the source key an episode uses in the alert center, deliveries and handling state.
func buildSyncAlertSourceKey(episodeID uint) string {
	return fmt.Sprintf("sync:episode:%d", episodeID)
}

// parseSyncAlertSourceKey 解析 sync:episode:<id>。
// parseSyncAlertSourceKey parses sync:episode:<id>.
func parseSyncAlertSourceKey(parts []string) (uint, bool) {
	if len(parts) != 3 || parts[0] != "sync" || parts[1] != "episode" {
		return 0, false
	}
	id, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}

// SyncAlertEpisodeFilter 是告警中心查询阶段的条件。
// SyncAlertEpisodeFilter is the alert center's query over episodes.
type SyncAlertEpisodeFilter struct {
	ClusterID uint
	TaskID    uint
	Since     *time.Time
	Until     *time.Time
	Limit     int
}

// CreateSyncAlertEpisode 新建告警阶段。
// CreateSyncAlertEpisode creates an alert episode.
func (r *Repository) CreateSyncAlertEpisode(ctx context.Context, episode *SyncAlertEpisode) error {
	return r.db.WithContext(ctx).Create(episode).Error
}

// SaveSyncAlertEpisode 保存告警阶段。
// SaveSyncAlertEpisode saves an alert episode.
func (r *Repository) SaveSyncAlertEpisode(ctx context.Context, episode *SyncAlertEpisode) error {
	return r.db.WithContext(ctx).Save(episode).Error
}

// GetSyncAlertEpisodeByID 按 ID 读取告警阶段；不存在时返回 nil。
// GetSyncAlertEpisodeByID loads an episode by ID; it returns nil when missing.
func (r *Repository) GetSyncAlertEpisodeByID(ctx context.Context, id uint) (*SyncAlertEpisode, error) {
	var episode SyncAlertEpisode
	err := r.db.WithContext(ctx).First(&episode, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &episode, nil
}

// FindFiringSyncAlertEpisode 查同一个键的 firing 阶段；没有时返回 nil。
// FindFiringSyncAlertEpisode finds the firing episode of a key; it returns nil when there is none.
func (r *Repository) FindFiringSyncAlertEpisode(ctx context.Context, scope SyncAlertEpisodeScope, clusterID, taskID uint, ruleKey string) (*SyncAlertEpisode, error) {
	var episode SyncAlertEpisode
	err := r.db.WithContext(ctx).
		Where("scope = ? AND cluster_id = ? AND task_id = ? AND rule_key = ? AND status = ?", scope, clusterID, taskID, ruleKey, SyncAlertEpisodeFiring).
		Order("id DESC").
		First(&episode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &episode, nil
}

// ListFiringSyncAlertEpisodesByTask 列出任务在给定规则上的 firing 阶段（不区分集群）。
// ListFiringSyncAlertEpisodesByTask lists a task's firing episodes for the given rules (any cluster).
func (r *Repository) ListFiringSyncAlertEpisodesByTask(ctx context.Context, taskID uint, ruleKeys []string) ([]*SyncAlertEpisode, error) {
	var episodes []*SyncAlertEpisode
	query := r.db.WithContext(ctx).
		Where("scope = ? AND task_id = ? AND status = ?", SyncAlertEpisodeScopeTask, taskID, SyncAlertEpisodeFiring)
	if len(ruleKeys) > 0 {
		query = query.Where("rule_key IN ?", ruleKeys)
	}
	if err := query.Order("id ASC").Find(&episodes).Error; err != nil {
		return nil, err
	}
	return episodes, nil
}

// ListFiringSyncAlertEpisodes 列出全部 firing 阶段，供提醒巡检使用。
// ListFiringSyncAlertEpisodes lists every firing episode for the reminder sweep.
func (r *Repository) ListFiringSyncAlertEpisodes(ctx context.Context, limit int) ([]*SyncAlertEpisode, error) {
	if limit <= 0 {
		limit = 500
	}
	var episodes []*SyncAlertEpisode
	if err := r.db.WithContext(ctx).
		Where("status = ?", SyncAlertEpisodeFiring).
		Order("id ASC").
		Limit(limit).
		Find(&episodes).Error; err != nil {
		return nil, err
	}
	return episodes, nil
}

// ListSyncAlertEpisodes 按条件列出阶段（最近的在前），供告警中心使用。
// ListSyncAlertEpisodes lists episodes matching the filter (newest first) for the alert center.
func (r *Repository) ListSyncAlertEpisodes(ctx context.Context, filter *SyncAlertEpisodeFilter) ([]*SyncAlertEpisode, error) {
	if filter == nil {
		filter = &SyncAlertEpisodeFilter{}
	}
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	query := r.db.WithContext(ctx).Model(&SyncAlertEpisode{})
	if filter.ClusterID > 0 {
		query = query.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.TaskID > 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	if filter.Since != nil {
		query = query.Where("last_occurred_at >= ?", filter.Since.UTC())
	}
	if filter.Until != nil {
		query = query.Where("first_fired_at <= ?", filter.Until.UTC())
	}
	var episodes []*SyncAlertEpisode
	if err := query.Order("id DESC").Limit(limit).Find(&episodes).Error; err != nil {
		return nil, err
	}
	return episodes, nil
}

// GetSyncJobAlertPolicyByTaskID 读取任务的作业告警策略；没有时返回 nil。
// GetSyncJobAlertPolicyByTaskID loads the task's job alert policy; it returns nil when there is none.
func (r *Repository) GetSyncJobAlertPolicyByTaskID(ctx context.Context, taskID uint) (*AlertPolicy, error) {
	var policy AlertPolicy
	err := r.db.WithContext(ctx).
		Where("policy_type = ? AND task_id = ?", AlertPolicyBuilderKindSyncJob, taskID).
		Order("id ASC").
		First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

// normalizeSyncAlertText 去掉首尾空白并按字符截断。
// normalizeSyncAlertText trims surrounding whitespace and truncates by characters.
func normalizeSyncAlertText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// syncAlertEpisodeClusterID 返回阶段所在集群（字符串），阶段不存在时返回 ErrAlertInstanceNotFound。
// syncAlertEpisodeClusterID returns the episode's cluster as a string, or ErrAlertInstanceNotFound when missing.
func (s *Service) syncAlertEpisodeClusterID(ctx context.Context, episodeID uint) (string, error) {
	episode, err := s.repo.GetSyncAlertEpisodeByID(ctx, episodeID)
	if err != nil {
		return "", err
	}
	if episode == nil {
		return "", ErrAlertInstanceNotFound
	}
	return strconv.FormatUint(uint64(episode.ClusterID), 10), nil
}

// buildSyncAlertInstances 把同步作业告警阶段转成告警中心条目，筛选语义与本地事件一致。
// buildSyncAlertInstances turns sync job alert episodes into alert center items with the same filtering semantics
// as local events.
func (s *Service) buildSyncAlertInstances(ctx context.Context, filter *AlertInstanceFilter, now time.Time) ([]*AlertInstance, error) {
	episodeFilter := &SyncAlertEpisodeFilter{Since: filter.StartTime, Until: filter.EndTime, Limit: 500}
	if filter.ClusterID != "" {
		clusterID, err := strconv.ParseUint(strings.TrimSpace(filter.ClusterID), 10, 32)
		if err != nil {
			return []*AlertInstance{}, nil
		}
		episodeFilter.ClusterID = uint(clusterID)
	}
	episodes, err := s.repo.ListSyncAlertEpisodes(ctx, episodeFilter)
	if err != nil {
		return nil, err
	}
	sourceKeys := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		sourceKeys = append(sourceKeys, buildSyncAlertSourceKey(episode.ID))
	}
	stateMap, err := s.repo.ListAlertStatesBySourceKeys(ctx, sourceKeys)
	if err != nil {
		return nil, err
	}

	items := make([]*AlertInstance, 0, len(episodes))
	for _, episode := range episodes {
		severity := normalizeAlertPolicySeverity(episode.Severity)
		if filter.Severity != "" && !strings.EqualFold(string(severity), string(filter.Severity)) {
			continue
		}
		lifecycle := AlertLifecycleStatusFiring
		if episode.Status == SyncAlertEpisodeResolved {
			lifecycle = AlertLifecycleStatusResolved
		}
		if filter.LifecycleStatus != "" && lifecycle != filter.LifecycleStatus {
			continue
		}
		sourceKey := buildSyncAlertSourceKey(episode.ID)
		state := stateMap[sourceKey]
		handling := resolveAlertHandlingStatus(state, now)
		if filter.HandlingStatus != "" && handling != filter.HandlingStatus {
			continue
		}
		display := resolveAlertDisplayStatus(lifecycle, handling)
		if filter.Status != "" && display != filter.Status {
			continue
		}
		lastSeen := episode.LastOccurredAt.UTC()
		if episode.ResolvedAt != nil && episode.ResolvedAt.After(lastSeen) {
			lastSeen = episode.ResolvedAt.UTC()
		}
		item := &AlertInstance{
			AlertID:         sourceKey,
			SourceType:      AlertSourceTypeSync,
			ClusterID:       strconv.FormatUint(uint64(episode.ClusterID), 10),
			ClusterName:     episode.ClusterName,
			Severity:        severity,
			AlertName:       syncAlertRuleName(episode.RuleKey),
			RuleKey:         episode.RuleKey,
			Summary:         buildSyncAlertInstanceSummary(episode),
			Description:     normalizeSyncAlertText(episode.LastMessage, syncAlertMessageDisplayLimit),
			Status:          display,
			LifecycleStatus: lifecycle,
			HandlingStatus:  handling,
			CreatedAt:       episode.FirstFiredAt.UTC(),
			FiringAt:        episode.FirstFiredAt.UTC(),
			LastSeenAt:      lastSeen,
			ResolvedAt:      toUTCTimePointer(episode.ResolvedAt),
			SourceRef: &AlertInstanceSourceRef{
				TaskID:          episode.TaskID,
				TaskName:        episode.TaskName,
				JobID:           episode.LastJobID,
				EngineJobID:     episode.LastEngineJobID,
				TriggerSource:   episode.LastTriggerSource,
				OccurrenceCount: episode.OccurrenceCount,
				Link:            syncAlertRelativeLink(episode),
				ResolveReason:   episode.ResolveReason,
				RemindersPaused: syncAlertRemindersPaused(episode),
			},
		}
		applyAlertStateToInstance(item, state)
		items = append(items, item)
	}
	return items, nil
}

// buildSyncAlertInstanceSummary 返回告警中心的一行摘要：主体 · 作业 · 累计次数。
// buildSyncAlertInstanceSummary returns the alert center one-liner: subject · job · occurrences.
func buildSyncAlertInstanceSummary(episode *SyncAlertEpisode) string {
	parts := []string{syncAlertSubject(episode)}
	if episode.LastJobID > 0 {
		parts = append(parts, fmt.Sprintf("作业 #%d", episode.LastJobID))
	}
	if episode.OccurrenceCount > 1 {
		parts = append(parts, fmt.Sprintf("累计 %d 次", episode.OccurrenceCount))
	}
	if episode.Status == SyncAlertEpisodeFiring {
		if detail := episodeCDCDelayDetail(episode); detail != nil && detail.RemindersPaused {
			parts = append(parts, "无法确认已超过 24 小时，已停止提醒")
		}
	}
	if episode.Status == SyncAlertEpisodeResolved {
		if note := syncAlertAutoCloseNote(episode.ResolveReason); note != "" {
			parts = append(parts, note)
		}
	}
	return strings.Join(parts, " · ")
}

// syncAlertRelativeLink 返回控制台内的作业链接；集群级阶段没有任务时返回空。
// syncAlertRelativeLink returns the in-console job link; cluster-level episodes without a task return empty.
func syncAlertRelativeLink(episode *SyncAlertEpisode) string {
	if episode.TaskID == 0 {
		return ""
	}
	link := fmt.Sprintf("/sync?task=%d", episode.TaskID)
	if episode.LastJobID > 0 {
		link += fmt.Sprintf("&job=%d", episode.LastJobID)
	}
	return link
}
