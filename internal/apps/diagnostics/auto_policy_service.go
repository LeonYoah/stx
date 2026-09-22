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
	"strings"
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

func validateAutoPolicyConditionsForUpdate(conditions InspectionConditionItems) error {
	for _, condition := range conditions {
		if _, ok := findBuiltinConditionTemplate(condition.TemplateCode); !ok {
			return fmt.Errorf("%w: unknown condition template %s", ErrInvalidAutoPolicyRequest, condition.TemplateCode)
		}
	}
	return nil
}
func validateAutoPolicyConditions(conditions InspectionConditionItems) error {
	for _, condition := range conditions {
		if !isConditionTemplateSupported(condition.TemplateCode) {
			return fmt.Errorf("%w: unsupported condition template %s", ErrInvalidAutoPolicyRequest, condition.TemplateCode)
		}
		if _, ok := findBuiltinConditionTemplate(condition.TemplateCode); !ok {
			return fmt.Errorf("%w: unknown condition template %s", ErrInvalidAutoPolicyRequest, condition.TemplateCode)
		}
	}
	return nil
}
