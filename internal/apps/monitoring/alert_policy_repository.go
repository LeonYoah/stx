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

	"gorm.io/gorm"
)

// ListAlertPolicies returns all unified alert policies.
// ListAlertPolicies 返回全部统一告警策略。
func (r *Repository) ListAlertPolicies(ctx context.Context) ([]*AlertPolicy, error) {
	var policies []*AlertPolicy
	if err := r.db.WithContext(ctx).
		Order("updated_at DESC").
		Order("id DESC").
		Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}

// ListEnabledAlertPoliciesByClusterAndLegacyRuleKey returns enabled policies for one cluster/rule pair.
// ListEnabledAlertPoliciesByClusterAndLegacyRuleKey 返回指定集群和旧规则键下已启用的统一策略。
func (r *Repository) ListEnabledAlertPoliciesByClusterAndLegacyRuleKey(ctx context.Context, clusterID string, legacyRuleKey string) ([]*AlertPolicy, error) {
	var policies []*AlertPolicy
	if err := r.db.WithContext(ctx).
		Where(
			"cluster_id = ? AND enabled = ? AND (legacy_rule_key = ? OR ((legacy_rule_key = '' OR legacy_rule_key IS NULL) AND template_key = ?))",
			clusterID,
			true,
			legacyRuleKey,
			legacyRuleKey,
		).
		Order("updated_at DESC").
		Order("id DESC").
		Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}

// GetAlertPolicyByID returns one alert policy by ID.
// GetAlertPolicyByID 根据 ID 返回统一告警策略。
func (r *Repository) GetAlertPolicyByID(ctx context.Context, id uint) (*AlertPolicy, error) {
	var policy AlertPolicy
	if err := r.db.WithContext(ctx).First(&policy, id).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

// GetAlertPolicyByClusterAndLegacyRuleKey returns one alert policy by cluster scope and legacy rule key.
// GetAlertPolicyByClusterAndLegacyRuleKey 根据集群范围和旧规则键返回统一告警策略。
func (r *Repository) GetAlertPolicyByClusterAndLegacyRuleKey(ctx context.Context, clusterID string, legacyRuleKey string) (*AlertPolicy, error) {
	var policy AlertPolicy
	if err := r.db.WithContext(ctx).
		Where(
			"cluster_id = ? AND (legacy_rule_key = ? OR ((legacy_rule_key = '' OR legacy_rule_key IS NULL) AND template_key = ?))",
			clusterID,
			legacyRuleKey,
			legacyRuleKey,
		).
		First(&policy).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

// CreateAlertPolicy creates one alert policy resource.
// CreateAlertPolicy 创建统一告警策略资源。
func (r *Repository) CreateAlertPolicy(ctx context.Context, policy *AlertPolicy) error {
	return r.db.WithContext(ctx).Select("*").Create(policy).Error
}

// SaveAlertPolicy updates one alert policy resource.
// SaveAlertPolicy 更新统一告警策略资源。
func (r *Repository) SaveAlertPolicy(ctx context.Context, policy *AlertPolicy) error {
	return r.db.WithContext(ctx).Save(policy).Error
}

// ApplyAlertPolicyExecutionState updates execution aggregate fields for one policy.
// ApplyAlertPolicyExecutionState 更新单条策略的执行聚合字段。
func (r *Repository) ApplyAlertPolicyExecutionState(ctx context.Context, policyID uint, state *AlertPolicyExecutionStateUpdate) error {
	if policyID == 0 || state == nil {
		return nil
	}

	updates := map[string]interface{}{}
	// 状态为空表示本次没有执行结论（比如巡检预算用完、全部渠道延后），保留原来的最近执行状态。
	// An empty status means this run reached no conclusion (e.g. the sweep budget ran out and every channel was
	// deferred), so the last execution status is kept.
	if state.LastExecutionStatus != "" {
		updates["last_execution_status"] = normalizeAlertPolicyExecutionStatus(state.LastExecutionStatus)
		updates["last_execution_error"] = state.LastExecutionError
	}

	if state.MatchCountDelta > 0 {
		updates["match_count"] = gorm.Expr("match_count + ?", state.MatchCountDelta)
	}
	if state.DeliveryCountDelta > 0 {
		updates["delivery_count"] = gorm.Expr("delivery_count + ?", state.DeliveryCountDelta)
	}
	if state.LastMatchedAt != nil {
		updates["last_matched_at"] = state.LastMatchedAt
	}
	if state.LastDeliveredAt != nil {
		updates["last_delivered_at"] = state.LastDeliveredAt
	}

	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&AlertPolicy{}).
		Where("id = ?", policyID).
		Updates(updates).Error
}

// DeleteAlertPolicy deletes one alert policy resource by ID.
// DeleteAlertPolicy 根据 ID 删除统一告警策略资源。
func (r *Repository) DeleteAlertPolicy(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&AlertPolicy{}, id).Error
}
