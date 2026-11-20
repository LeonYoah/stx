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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// ListAlertPolicies returns unified alert policy resources.
// ListAlertPolicies 返回统一告警策略资源列表。
// filter 为空时返回全部；policy_type、task_id 可选，用于在策略中心按"作业告警"和任务筛选。
// A nil filter returns everything; policy_type and task_id are optional and let the policy center filter job
// alerts by task.
func (s *Service) ListAlertPolicies(ctx context.Context, filter *AlertPolicyListFilter) (*AlertPolicyListData, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}

	policies, err := s.repo.ListAlertPolicies(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]*AlertPolicyDTO, 0, len(policies))
	var taskIDs []uint
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		if filter != nil && filter.PolicyType != "" && policy.PolicyType != filter.PolicyType {
			continue
		}
		if filter != nil && filter.TaskID > 0 && policy.TaskID != filter.TaskID {
			continue
		}
		if policy.TaskID > 0 {
			taskIDs = append(taskIDs, policy.TaskID)
		}
		items = append(items, toAlertPolicyDTO(policy))
	}
	if names := s.resolveSyncTaskNames(ctx, taskIDs); len(names) > 0 {
		for _, item := range items {
			if item.TaskID > 0 {
				item.TaskName = names[item.TaskID]
			}
		}
	}

	return &AlertPolicyListData{
		GeneratedAt: timeNowUTC(),
		Total:       len(items),
		Policies:    items,
	}, nil
}

// ListAlertPolicyExecutions returns notification execution history for one unified alert policy.
// ListAlertPolicyExecutions 返回单条统一告警策略的通知执行历史。
func (s *Service) ListAlertPolicyExecutions(ctx context.Context, policyID uint, filter *NotificationDeliveryFilter) (*NotificationDeliveryListData, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if policyID == 0 {
		return nil, ErrAlertPolicyInvalidID
	}

	if _, err := s.repo.GetAlertPolicyByID(ctx, policyID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAlertPolicyNotFound
		}
		return nil, err
	}

	if filter == nil {
		filter = &NotificationDeliveryFilter{}
	}
	filter.PolicyID = policyID
	return s.ListNotificationDeliveries(ctx, filter)
}

// CreateAlertPolicy creates one unified alert policy resource.
// CreateAlertPolicy 创建统一告警策略资源。
func (s *Service) CreateAlertPolicy(ctx context.Context, req *UpsertAlertPolicyRequest) (*AlertPolicyDTO, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := s.validateAlertPolicyScopes(ctx, req.ClusterID, req.NotificationChannelIDs, req.ReceiverUserIDs); err != nil {
		return nil, err
	}
	if err := s.validateSyncJobPolicy(ctx, s.repo, 0, req); err != nil {
		return nil, err
	}

	conditionsJSON, err := marshalAlertPolicyConditions(req.Conditions)
	if err != nil {
		return nil, err
	}
	channelIDsJSON, err := marshalAlertPolicyChannelIDs(req.NotificationChannelIDs)
	if err != nil {
		return nil, err
	}
	receiverUserIDsJSON, err := marshalAlertPolicyReceiverUserIDs(req.ReceiverUserIDs)
	if err != nil {
		return nil, err
	}

	policy := &AlertPolicy{
		Name:                       strings.TrimSpace(req.Name),
		Description:                strings.TrimSpace(req.Description),
		PolicyType:                 req.PolicyType,
		ClusterID:                  normalizeAlertPolicyClusterID(req.ClusterID),
		Severity:                   normalizeAlertPolicySeverity(req.Severity),
		Enabled:                    req.Enabled == nil || *req.Enabled,
		CooldownMinutes:            resolveAlertPolicyCooldownMinutes(req.CooldownMinutes),
		SendRecovery:               req.SendRecovery == nil || *req.SendRecovery,
		ConditionsJSON:             conditionsJSON,
		NotificationChannelIDsJSON: channelIDsJSON,
		ReceiverUserIDsJSON:        receiverUserIDsJSON,
		LastExecutionStatus:        AlertPolicyExecutionStatusIdle,
		TaskID:                     req.TaskID,
		NotifyTaskOwner:            req.NotifyTaskOwner != nil && *req.NotifyTaskOwner,
	}

	if err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &Repository{db: tx}
		if err := txRepo.CreateAlertPolicy(ctx, policy); err != nil {
			return err
		}
		policy.Enabled = req.Enabled == nil || *req.Enabled
		policy.SendRecovery = req.SendRecovery == nil || *req.SendRecovery
		if err := txRepo.SaveAlertPolicy(ctx, policy); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// 策略提交后触发评估器即时重建规则组；评估器未装配（observability 关闭或 tsdb 未就绪）时为空操作。
	// After the policy commits, ask the evaluator to rebuild its rule groups immediately; a no-op
	// when the evaluator is not assembled (observability off or tsdb not ready).
	s.requestMetricEvalReload()
	return toAlertPolicyDTO(policy), nil
}

// UpdateAlertPolicy updates one unified alert policy resource.
// UpdateAlertPolicy 更新统一告警策略资源。
func (s *Service) UpdateAlertPolicy(ctx context.Context, id uint, req *UpsertAlertPolicyRequest) (*AlertPolicyDTO, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if id == 0 {
		return nil, ErrAlertPolicyInvalidID
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := s.validateAlertPolicyScopes(ctx, req.ClusterID, req.NotificationChannelIDs, req.ReceiverUserIDs); err != nil {
		return nil, err
	}

	conditionsJSON, err := marshalAlertPolicyConditions(req.Conditions)
	if err != nil {
		return nil, err
	}
	channelIDsJSON, err := marshalAlertPolicyChannelIDs(req.NotificationChannelIDs)
	if err != nil {
		return nil, err
	}
	receiverUserIDsJSON, err := marshalAlertPolicyReceiverUserIDs(req.ReceiverUserIDs)
	if err != nil {
		return nil, err
	}

	var updated *AlertPolicy
	// 与冷却提醒互斥：停用或删除策略时关闭的阶段不会在同一时刻再被提醒。
	// Serialised with reminders: an episode closed by disabling or deleting the policy is never reminded at the same moment.
	s.syncAlertEpisodeMu.Lock()
	defer s.syncAlertEpisodeMu.Unlock()
	s.syncAlertPolicyVersion++
	if err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &Repository{db: tx}
		policy, err := txRepo.GetAlertPolicyByID(ctx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertPolicyNotFound
			}
			return err
		}

		// 作业告警策略不能和其他类型互转；所属任务创建后不可改，以库里的为准。
		// Job alert policies cannot switch type; the owning task is immutable and the stored one wins.
		if policy.PolicyType != AlertPolicyBuilderKindSyncJob {
			return ErrAlertPolicySyncJobTypeChange
		}
		req.TaskID = policy.TaskID
		if err := s.validateSyncJobPolicy(ctx, txRepo, policy.ID, req); err != nil {
			return err
		}
		if req.NotifyTaskOwner != nil {
			policy.NotifyTaskOwner = *req.NotifyTaskOwner
		}

		policy.Name = strings.TrimSpace(req.Name)
		policy.Description = strings.TrimSpace(req.Description)
		policy.ClusterID = normalizeAlertPolicyClusterID(req.ClusterID)
		policy.Severity = normalizeAlertPolicySeverity(req.Severity)
		policy.ConditionsJSON = conditionsJSON
		policy.NotificationChannelIDsJSON = channelIDsJSON
		policy.ReceiverUserIDsJSON = receiverUserIDsJSON
		if req.Enabled != nil {
			policy.Enabled = *req.Enabled
		}
		if req.CooldownMinutes != nil {
			policy.CooldownMinutes = *req.CooldownMinutes
		}
		if req.SendRecovery != nil {
			policy.SendRecovery = *req.SendRecovery
		}

		if err := txRepo.SaveAlertPolicy(ctx, policy); err != nil {
			return err
		}
		// 去掉 B4 或停用策略：正在告警的 B4 阶段静默关闭（rule_removed），告警中心显示原因。
		// Removing B4 or disabling the policy silently closes the firing B4 episode (rule_removed); the alert center
		// shows the reason.
		if policy.PolicyType == AlertPolicyBuilderKindSyncJob && policy.TaskID > 0 &&
			(!policy.Enabled || !syncJobPolicyHasRule(policy, syncalert.RuleCDCDelayHigh)) {
			if err := closeSyncTaskEpisodesByRules(ctx, txRepo, policy.TaskID, []string{syncalert.RuleCDCDelayHigh}, syncalert.ResolveReasonRuleRemoved); err != nil {
				return err
			}
		}

		updated = policy
		return nil
	}); err != nil {
		return nil, err
	}
	// 策略提交后触发评估器即时重建规则组（启停、阈值、窗口等改动即时生效）。
	// After the policy commits, ask the evaluator to rebuild its rule groups immediately
	// (enable/threshold/window changes take effect at once).
	s.requestMetricEvalReload()
	return toAlertPolicyDTO(updated), nil
}

// DeleteAlertPolicy deletes one unified alert policy resource.
// DeleteAlertPolicy 删除统一告警策略资源。
func (s *Service) DeleteAlertPolicy(ctx context.Context, id uint) error {
	if s.repo == nil {
		return fmt.Errorf("monitoring repository is not configured")
	}
	if id == 0 {
		return ErrAlertPolicyInvalidID
	}
	// 与冷却提醒互斥：停用或删除策略时关闭的阶段不会在同一时刻再被提醒。
	// Serialised with reminders: an episode closed by disabling or deleting the policy is never reminded at the same moment.
	s.syncAlertEpisodeMu.Lock()
	defer s.syncAlertEpisodeMu.Unlock()
	s.syncAlertPolicyVersion++
	if err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &Repository{db: tx}
		policy, err := txRepo.GetAlertPolicyByID(ctx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertPolicyNotFound
			}
			return err
		}

		if err := txRepo.DeleteAlertPolicy(ctx, id); err != nil {
			return err
		}
		// 删除作业告警策略后不再产生新告警，正在告警中的阶段一并结束，不发恢复通知。
		// Deleting a job alert policy stops new alerts and ends its firing episodes without recovery notifications.
		if policy.PolicyType == AlertPolicyBuilderKindSyncJob && policy.TaskID > 0 {
			if err := closeSyncTaskEpisodes(ctx, txRepo, policy.TaskID, syncAlertResolvePolicyDeleted); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	// 策略删除后即时重建规则组，让被删策略的规则立刻退出评估。
	// After the policy is deleted, rebuild rule groups at once so its rule leaves evaluation.
	s.requestMetricEvalReload()
	return nil
}

func (s *Service) validateAlertPolicyScopes(ctx context.Context, clusterID string, channelIDs []uint, receiverUserIDs []uint64) error {
	normalizedClusterID := normalizeAlertPolicyClusterID(clusterID)
	if normalizedClusterID != "" && normalizedClusterID != "all" {
		parsedClusterID, err := strconv.ParseUint(normalizedClusterID, 10, 32)
		if err != nil {
			return ErrAlertPolicyInvalidClusterScope
		}
		if _, err := s.clusterService.Get(ctx, uint(parsedClusterID)); err != nil {
			return ErrAlertPolicyClusterNotFound
		}
	}

	for _, channelID := range normalizeAlertPolicyChannelIDs(channelIDs) {
		if _, err := s.repo.GetNotificationChannelByID(ctx, channelID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertPolicyNotificationChannelNotFound
			}
			return err
		}
	}
	if err := s.validateAlertPolicyReceiverUsers(ctx, receiverUserIDs); err != nil {
		return err
	}
	return nil
}

func normalizeAlertPolicySeverity(severity AlertSeverity) AlertSeverity {
	switch severity {
	case AlertSeverityCritical:
		return AlertSeverityCritical
	default:
		return AlertSeverityWarning
	}
}

// defaultAlertReminderIntervalMinutes 是策略冷却期的默认分钟数（沿用原 AM repeat_interval 的取值）。
// defaultAlertReminderIntervalMinutes is the default policy cooldown in minutes (kept from the
// old AM repeat_interval value).
const defaultAlertReminderIntervalMinutes = 10

func resolveAlertPolicyCooldownMinutes(value *int) int {
	if value == nil {
		return defaultAlertReminderIntervalMinutes
	}
	return *value
}

func normalizeAlertPolicyExecutionStatus(status AlertPolicyExecutionStatus) AlertPolicyExecutionStatus {
	switch status {
	case AlertPolicyExecutionStatusMatched,
		AlertPolicyExecutionStatusSent,
		AlertPolicyExecutionStatusFailed,
		AlertPolicyExecutionStatusPartial:
		return status
	default:
		return AlertPolicyExecutionStatusIdle
	}
}

func normalizeAlertPolicyClusterID(clusterID string) string {
	value := strings.TrimSpace(clusterID)
	if value == "" || strings.EqualFold(value, "all") {
		return "all"
	}
	return value
}

func normalizeAlertPolicyChannelIDs(channelIDs []uint) []uint {
	if len(channelIDs) == 0 {
		return []uint{}
	}
	seen := make(map[uint]struct{}, len(channelIDs))
	result := make([]uint, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID == 0 {
			continue
		}
		if _, exists := seen[channelID]; exists {
			continue
		}
		seen[channelID] = struct{}{}
		result = append(result, channelID)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func marshalAlertPolicyConditions(conditions []*AlertPolicyConditionDTO) (string, error) {
	if len(conditions) == 0 {
		return "[]", nil
	}
	payload, err := json.Marshal(conditions)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalAlertPolicyConditions(raw string) []*AlertPolicyConditionDTO {
	if strings.TrimSpace(raw) == "" {
		return []*AlertPolicyConditionDTO{}
	}
	var conditions []*AlertPolicyConditionDTO
	if err := json.Unmarshal([]byte(raw), &conditions); err != nil {
		return []*AlertPolicyConditionDTO{}
	}
	return conditions
}

func marshalAlertPolicyChannelIDs(channelIDs []uint) (string, error) {
	payload, err := json.Marshal(normalizeAlertPolicyChannelIDs(channelIDs))
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalAlertPolicyChannelIDs(raw string) []uint {
	if strings.TrimSpace(raw) == "" {
		return []uint{}
	}
	var channelIDs []uint
	if err := json.Unmarshal([]byte(raw), &channelIDs); err != nil {
		return []uint{}
	}
	return normalizeAlertPolicyChannelIDs(channelIDs)
}

func marshalAlertPolicyReceiverUserIDs(userIDs []uint64) (string, error) {
	payload, err := json.Marshal(normalizeReceiverUserIDs(userIDs))
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalAlertPolicyReceiverUserIDs(raw string) []uint64 {
	if strings.TrimSpace(raw) == "" {
		return []uint64{}
	}
	var userIDs []uint64
	if err := json.Unmarshal([]byte(raw), &userIDs); err != nil {
		return []uint64{}
	}
	return normalizeReceiverUserIDs(userIDs)
}

func toAlertPolicyDTO(policy *AlertPolicy) *AlertPolicyDTO {
	if policy == nil {
		return nil
	}
	return &AlertPolicyDTO{
		ID:                     policy.ID,
		Name:                   policy.Name,
		Description:            policy.Description,
		PolicyType:             policy.PolicyType,
		TemplateKey:            policy.TemplateKey,
		LegacyRuleKey:          policy.LegacyRuleKey,
		ClusterID:              policy.ClusterID,
		Severity:               policy.Severity,
		Enabled:                policy.Enabled,
		CooldownMinutes:        policy.CooldownMinutes,
		SendRecovery:           policy.SendRecovery,
		Conditions:             unmarshalAlertPolicyConditions(policy.ConditionsJSON),
		NotificationChannelIDs: unmarshalAlertPolicyChannelIDs(policy.NotificationChannelIDsJSON),
		ReceiverUserIDs:        unmarshalAlertPolicyReceiverUserIDs(policy.ReceiverUserIDsJSON),
		MatchCount:             policy.MatchCount,
		DeliveryCount:          policy.DeliveryCount,
		LastMatchedAt:          toUTCTimePointer(policy.LastMatchedAt),
		LastDeliveredAt:        toUTCTimePointer(policy.LastDeliveredAt),
		LastExecutionStatus:    normalizeAlertPolicyExecutionStatus(policy.LastExecutionStatus),
		LastExecutionError:     strings.TrimSpace(policy.LastExecutionError),
		CreatedAt:              policy.CreatedAt,
		UpdatedAt:              policy.UpdatedAt,
		TaskID:                 policy.TaskID,
		NotifyTaskOwner:        policy.NotifyTaskOwner,
	}
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func timeNowUTC() time.Time {
	return timex.Now()
}
