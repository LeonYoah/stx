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
	"fmt"
	"log"
	"strings"

	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
)

// ListBuiltinConditionTemplates returns a copy of all built-in condition templates.
// ListBuiltinConditionTemplates 返回所有内置条件模板的副本。
func (s *Service) ListBuiltinConditionTemplates() []*InspectionConditionTemplate {
	result := make([]*InspectionConditionTemplate, 0, len(BuiltinConditionTemplates))
	for i := range BuiltinConditionTemplates {
		t := BuiltinConditionTemplates[i]
		if !isConditionTemplateSupported(t.Code) {
			continue
		}
		result = append(result, &t)
	}
	return result
}

// CreateAutoPolicy creates a new auto-inspection policy.
// CreateAutoPolicy 创建一条新的自动巡检策略。
func (s *Service) CreateAutoPolicy(ctx context.Context, userID uint, req *CreateInspectionAutoPolicyRequest) (*InspectionAutoPolicyInfo, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request is nil", ErrInvalidAutoPolicyRequest)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidAutoPolicyRequest)
	}
	if len(req.Conditions) == 0 {
		return nil, fmt.Errorf("%w: at least one condition is required", ErrInvalidAutoPolicyRequest)
	}
	if err := validateAutoPolicyConditions(req.Conditions); err != nil {
		return nil, err
	}

	cooldownMinutes := req.CooldownMinutes
	if cooldownMinutes <= 0 {
		cooldownMinutes = 30
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Normalize task options for auto-created diagnostic tasks.
	taskOptions := DiagnosticTaskOptions{}
	if req.TaskOptions != nil {
		taskOptions = req.TaskOptions.Normalize()
	} else {
		taskOptions = DefaultDiagnosticTaskOptions()
	}
	if err := validateDiagnosticResourceSelection(taskOptions); err != nil {
		return nil, err
	}

	policy := &InspectionAutoPolicy{
		ClusterID:       req.ClusterID,
		Name:            name,
		Enabled:         enabled,
		Conditions:      req.Conditions,
		CooldownMinutes: cooldownMinutes,
		AutoCreateTask:  req.AutoCreateTask,
		AutoStartTask:   req.AutoStartTask,
		TaskOptions:     taskOptions,
	}
	if err := s.repo.CreateAutoPolicy(ctx, policy); err != nil {
		return nil, err
	}
	return policy.ToInfo(), nil
}

// UpdateAutoPolicy updates an existing auto-inspection policy.
// UpdateAutoPolicy 更新一条已有的自动巡检策略。
func (s *Service) UpdateAutoPolicy(ctx context.Context, id uint, req *UpdateInspectionAutoPolicyRequest) (*InspectionAutoPolicyInfo, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request is nil", ErrInvalidAutoPolicyRequest)
	}

	policy, err := s.repo.GetAutoPolicyByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", ErrInvalidAutoPolicyRequest)
		}
		policy.Name = name
	}
	if req.Enabled != nil {
		policy.Enabled = *req.Enabled
	}
	if req.Conditions != nil {
		if len(*req.Conditions) == 0 {
			return nil, fmt.Errorf("%w: at least one condition is required", ErrInvalidAutoPolicyRequest)
		}
		if err := validateAutoPolicyConditionsForUpdate(*req.Conditions); err != nil {
			return nil, err
		}
		policy.Conditions = *req.Conditions
	}
	if req.CooldownMinutes != nil {
		cooldown := *req.CooldownMinutes
		if cooldown <= 0 {
			cooldown = 30
		}
		policy.CooldownMinutes = cooldown
	}
	if req.AutoCreateTask != nil {
		policy.AutoCreateTask = *req.AutoCreateTask
	}
	if req.AutoStartTask != nil {
		policy.AutoStartTask = *req.AutoStartTask
	}
	if req.TaskOptions != nil {
		options := req.TaskOptions.Normalize()
		if err := validateDiagnosticResourceSelection(options); err != nil {
			return nil, err
		}
		policy.TaskOptions = options
	}

	if err := s.repo.UpdateAutoPolicy(ctx, policy); err != nil {
		return nil, err
	}
	return policy.ToInfo(), nil
}

// DeleteAutoPolicy deletes an auto-inspection policy by identifier.
// DeleteAutoPolicy 根据标识删除一条自动巡检策略。
func (s *Service) DeleteAutoPolicy(ctx context.Context, id uint) error {
	if s == nil || s.repo == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	return s.repo.DeleteAutoPolicy(ctx, id)
}

// GetAutoPolicy returns one auto-inspection policy by identifier.
// GetAutoPolicy 根据标识返回一条自动巡检策略。
func (s *Service) GetAutoPolicy(ctx context.Context, id uint) (*InspectionAutoPolicyInfo, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	policy, err := s.repo.GetAutoPolicyByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return policy.ToInfo(), nil
}

// ListAutoPolicies returns paginated auto-inspection policies.
// ListAutoPolicies 返回分页的自动巡检策略列表。
func (s *Service) ListAutoPolicies(ctx context.Context, clusterID *uint, page, pageSize int) (*InspectionAutoPolicyListData, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}

	policies, total, err := s.repo.ListAutoPolicies(ctx, clusterID, page, pageSize)
	if err != nil {
		return nil, err
	}

	page, pageSize = normalizePagination(page, pageSize)
	items := make([]*InspectionAutoPolicyInfo, 0, len(policies))
	for _, policy := range policies {
		items = append(items, policy.ToInfo())
	}
	return &InspectionAutoPolicyListData{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// WarnOnInvalidAutoPolicies 启动时扫描失效自动巡检策略（旧 PROM_* / NODE_UNHEALTHY 模板
// 或未选规则的 ALERT_FIRING），记一条 WARN 并列出受影响策略 ID。
// WarnOnInvalidAutoPolicies scans invalid auto policies at startup (retired PROM_* /
// NODE_UNHEALTHY templates or ALERT_FIRING without rule keys) and logs one WARN with the IDs.
func (s *Service) WarnOnInvalidAutoPolicies(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	policies, _, err := s.repo.ListAutoPolicies(ctx, nil, 1, 1000)
	if err != nil {
		log.Printf("[DiagnosticsAutoPolicy] WARN 扫描失效自动巡检策略失败 / failed to scan invalid auto policies: %v\n", err)
		return
	}
	invalid := make([]uint, 0)
	for _, policy := range policies {
		if policy != nil && policy.InvalidReason() != "" {
			invalid = append(invalid, policy.ID)
		}
	}
	if len(invalid) > 0 {
		log.Printf("[DiagnosticsAutoPolicy] WARN 发现 %d 条失效自动巡检策略（旧巡检条件，永远不会触发），受影响 ID: %v / found %d invalid auto policies (legacy conditions that never fire), affected IDs: %v\n",
			len(invalid), invalid, len(invalid), invalid)
	}
}

func validateAutoPolicyConditionsForUpdate(conditions InspectionConditionItems) error {
	return validateAutoPolicyConditions(conditions)
}

// validateAutoPolicyConditions 校验自动策略条件列表：条件码必须在内置模板内，
// ALERT_FIRING 额外要求规则键非空且都在告警规则目录里，阈值与窗口在合法范围。
// validateAutoPolicyConditions validates auto-policy conditions: template codes must exist in
// the builtin list, ALERT_FIRING additionally requires at least one rule key present in the
// alert rule catalog, and the threshold/window overrides must be within range.
func validateAutoPolicyConditions(conditions InspectionConditionItems) error {
	catalogKeys := alertRuleCatalogKeySet()
	for index := range conditions {
		condition := &conditions[index]
		if !isConditionTemplateSupported(condition.TemplateCode) {
			return fmt.Errorf("%w: unsupported condition template %s", ErrInvalidAutoPolicyRequest, condition.TemplateCode)
		}
		if _, ok := findBuiltinConditionTemplate(condition.TemplateCode); !ok {
			return fmt.Errorf("%w: unknown condition template %s", ErrInvalidAutoPolicyRequest, condition.TemplateCode)
		}
		if condition.TemplateCode != ConditionCodeAlertFiring {
			continue
		}
		condition.RuleKeys = normalizeAlertRuleKeys(condition.RuleKeys)
		if len(condition.RuleKeys) == 0 {
			return fmt.Errorf("%w: ALERT_FIRING requires at least one rule key", ErrInvalidAutoPolicyRequest)
		}
		for _, ruleKey := range condition.RuleKeys {
			if _, ok := catalogKeys[ruleKey]; !ok {
				return fmt.Errorf("%w: unknown alert rule key %s", ErrInvalidAutoPolicyRequest, ruleKey)
			}
		}
		if condition.ThresholdOverride != nil && *condition.ThresholdOverride < 1 {
			return fmt.Errorf("%w: ALERT_FIRING threshold must be at least 1", ErrInvalidAutoPolicyRequest)
		}
		if condition.WindowMinutesOverride != nil && (*condition.WindowMinutesOverride < 0 || *condition.WindowMinutesOverride > 1440) {
			return fmt.Errorf("%w: ALERT_FIRING window must be between 0 and 1440 minutes", ErrInvalidAutoPolicyRequest)
		}
	}
	return nil
}

// alertRuleCatalogKeySet 返回告警规则目录里全部可选规则键的集合。
// alertRuleCatalogKeySet returns the set of all selectable rule keys in the alert rule catalog.
func alertRuleCatalogKeySet() map[string]struct{} {
	keys := make(map[string]struct{}, 16)
	for _, item := range monitoringapp.AlertRuleCatalog("") {
		if item == nil {
			continue
		}
		keys[item.RuleKey] = struct{}{}
	}
	return keys
}

// normalizeAlertRuleKeys 去空白、去重规则键，保持原始顺序。
// normalizeAlertRuleKeys trims and dedupes rule keys while preserving order.
func normalizeAlertRuleKeys(ruleKeys []string) []string {
	result := make([]string, 0, len(ruleKeys))
	seen := make(map[string]struct{}, len(ruleKeys))
	for _, key := range ruleKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}
