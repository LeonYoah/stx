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

// CreateAutoPolicy persists one new auto-inspection policy.
// CreateAutoPolicy 持久化一条新的自动巡检策略。
func (r *Repository) CreateAutoPolicy(ctx context.Context, policy *InspectionAutoPolicy) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Create(policy).Error
}

// UpdateAutoPolicy updates one existing auto-inspection policy.
// UpdateAutoPolicy 更新一条已有的自动巡检策略。
func (r *Repository) UpdateAutoPolicy(ctx context.Context, policy *InspectionAutoPolicy) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return r.db.WithContext(ctx).Save(policy).Error
}

// DeleteAutoPolicy deletes one auto-inspection policy by identifier.
// DeleteAutoPolicy 根据标识删除一条自动巡检策略。
func (r *Repository) DeleteAutoPolicy(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	result := r.db.WithContext(ctx).Delete(&InspectionAutoPolicy{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAutoPolicyNotFound
	}
	return nil
}

// GetAutoPolicyByID retrieves one auto-inspection policy by identifier.
// GetAutoPolicyByID 根据标识获取一条自动巡检策略。
func (r *Repository) GetAutoPolicyByID(ctx context.Context, id uint) (*InspectionAutoPolicy, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var policy InspectionAutoPolicy
	if err := r.db.WithContext(ctx).First(&policy, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAutoPolicyNotFound
		}
		return nil, err
	}
	return &policy, nil
}

// ListAutoPolicies returns paginated auto-inspection policies with optional cluster filter.
// clusterID nil = all, clusterID pointing to 0 = global only.
// ListAutoPolicies 返回分页自动巡检策略，支持可选集群过滤。
// clusterID 为 nil 表示全部，指向 0 表示仅全局策略。
func (r *Repository) ListAutoPolicies(ctx context.Context, clusterID *uint, page, pageSize int) ([]*InspectionAutoPolicy, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrDiagnosticsRepositoryUnavailable
	}

	query := r.db.WithContext(ctx).Model(&InspectionAutoPolicy{})
	if clusterID != nil {
		query = query.Where("cluster_id = ?", *clusterID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize = normalizePagination(page, pageSize)
	var policies []*InspectionAutoPolicy
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&policies).Error; err != nil {
		return nil, 0, err
	}
	return policies, total, nil
}

// ListEnabledPoliciesForCluster returns enabled policies applicable to a cluster.
// It returns policies where ClusterID equals the given clusterID OR ClusterID is 0 (global).
// ListEnabledPoliciesForCluster 返回适用于指定集群的已启用策略。
// 返回 ClusterID 等于给定值或 ClusterID 为 0（全局）的策略。
func (r *Repository) ListEnabledPoliciesForCluster(ctx context.Context, clusterID uint) ([]*InspectionAutoPolicy, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var policies []*InspectionAutoPolicy
	if err := r.db.WithContext(ctx).
		Where("enabled = ? AND (cluster_id = ? OR cluster_id = 0)", true, clusterID).
		Order("cluster_id DESC, id ASC").
		Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}

// GetLastInspectionReportForCluster returns the most recent inspection report for a cluster.
// GetLastInspectionReportForCluster 返回指定集群的最近一条巡检报告。
func (r *Repository) GetLastInspectionReportForCluster(ctx context.Context, clusterID uint) (*ClusterInspectionReport, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var report ClusterInspectionReport
	if err := r.db.WithContext(ctx).
		Where("cluster_id = ?", clusterID).
		Order("created_at DESC").
		First(&report).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInspectionReportNotFound
		}
		return nil, err
	}
	return &report, nil
}

// GetLastInspectionReportForRequester returns the most recent inspection report for one cluster/requester pair.
// GetLastInspectionReportForRequester 返回指定集群与请求者组合下最近的一条巡检报告。
func (r *Repository) GetLastInspectionReportForRequester(ctx context.Context, clusterID uint, requestedBy string) (*ClusterInspectionReport, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var report ClusterInspectionReport
	if err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND requested_by = ?", clusterID, requestedBy).
		Order("created_at DESC").
		First(&report).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInspectionReportNotFound
		}
		return nil, err
	}
	return &report, nil
}

// SetInspectionReportAutoTrigger writes the auto-policy attribution (policy id + reason)
// onto one inspection report in a single update.
// SetInspectionReportAutoTrigger 一次性把自动巡检归因（策略 ID 与触发原因）写入一条巡检报告。
func (r *Repository) SetInspectionReportAutoTrigger(ctx context.Context, reportID uint, policyID uint, reason string) error {
	if r == nil || r.db == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	result := r.db.WithContext(ctx).
		Model(&ClusterInspectionReport{}).
		Where("id = ?", reportID).
		Updates(map[string]interface{}{
			"auto_policy_id":      policyID,
			"auto_trigger_reason": reason,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInspectionReportNotFound
	}
	return nil
}

// AutoPolicyTriggerStats aggregates trigger history for one auto policy.
// AutoPolicyTriggerStats 汇总一条自动巡检策略的触发历史。
type AutoPolicyTriggerStats struct {
	LastTriggeredAt *time.Time
	TriggerCount7d  int64
}

// ListAutoPolicyTriggerStats 按 auto_policy_id 聚合巡检报告，产出每条策略的最近触发时间
// 与近 7 天触发次数。固定两条分组聚合查询，避免随策略数 N+1。
// ListAutoPolicyTriggerStats aggregates inspection reports by auto_policy_id, yielding each
// policy's last trigger time and 7-day trigger count. It runs exactly two grouped aggregate
// queries, so the cost stays constant regardless of policy count.
func (r *Repository) ListAutoPolicyTriggerStats(ctx context.Context) (map[uint]*AutoPolicyTriggerStats, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	stats := make(map[uint]*AutoPolicyTriggerStats)

	if strings.EqualFold(r.db.Dialector.Name(), "sqlite") {
		// SQLite 把 created_at 存成带本地偏移的文本，裸 MAX 做字典序比较不可靠；
		// 用 julianday 取瞬时最大后由 strftime 还原为 UTC 文本。
		// SQLite stores created_at as local-offset text, where a bare MAX compares
		// lexicographically; MAX over julianday picks the true latest instant and
		// strftime converts it back to UTC text.
		type lastRow struct {
			AutoPolicyID    uint
			LastTriggeredAt string
		}
		var rows []lastRow
		if err := r.db.WithContext(ctx).
			Model(&ClusterInspectionReport{}).
			Select("auto_policy_id, strftime('%Y-%m-%d %H:%M:%f', MAX(julianday(created_at))) AS last_triggered_at").
			Where("auto_policy_id > 0").
			Group("auto_policy_id").
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			parsed, err := time.ParseInLocation("2006-01-02 15:04:05.999", row.LastTriggeredAt, time.UTC)
			if err != nil {
				continue
			}
			last := parsed.UTC()
			stats[row.AutoPolicyID] = &AutoPolicyTriggerStats{LastTriggeredAt: &last}
		}
	} else {
		type lastRow struct {
			AutoPolicyID    uint
			LastTriggeredAt time.Time
		}
		var rows []lastRow
		if err := r.db.WithContext(ctx).
			Model(&ClusterInspectionReport{}).
			Select("auto_policy_id, MAX(created_at) AS last_triggered_at").
			Where("auto_policy_id > 0").
			Group("auto_policy_id").
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			last := row.LastTriggeredAt
			stats[row.AutoPolicyID] = &AutoPolicyTriggerStats{LastTriggeredAt: &last}
		}
	}

	type countRow struct {
		AutoPolicyID uint
		Cnt          int64
	}
	var countRows []countRow
	countQuery := r.db.WithContext(ctx).
		Model(&ClusterInspectionReport{}).
		Select("auto_policy_id, COUNT(*) AS cnt").
		Where("auto_policy_id > 0").
		Group("auto_policy_id")
	countQuery = applyOccurredAtLowerBound(countQuery, "created_at", time.Now().Add(-7*24*time.Hour))
	if err := countQuery.Scan(&countRows).Error; err != nil {
		return nil, err
	}
	for _, row := range countRows {
		entry, ok := stats[row.AutoPolicyID]
		if !ok {
			entry = &AutoPolicyTriggerStats{}
			stats[row.AutoPolicyID] = entry
		}
		entry.TriggerCount7d = row.Cnt
	}
	return stats, nil
}
