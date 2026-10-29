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

type alertPolicyLegacyBridgeRef struct {
	ClusterID     uint
	RuleKey       string
	HasLegacyRule bool
}

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
	template, err := resolveAlertPolicyTemplate(req.PolicyType, req.TemplateKey)
	if err != nil {
		return nil, err
	}
	if err := s.validateAlertPolicyScopes(ctx, req.ClusterID, req.NotificationChannelIDs, req.ReceiverUserIDs); err != nil {
		return nil, err
	}
	legacyRuleKey := resolveAlertPolicyLegacyRuleKey(req.PolicyType, template, req.LegacyRuleKey)
	if err := s.validateAlertPolicyLegacyBridge(ctx, 0, req.PolicyType, req.ClusterID, legacyRuleKey); err != nil {
		return nil, err
	}
	if req.PolicyType == AlertPolicyBuilderKindSyncJob {
		if err := s.validateSyncJobPolicy(ctx, s.repo, 0, req); err != nil {
			return nil, err
		}
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
		TemplateKey:                strings.TrimSpace(req.TemplateKey),
		LegacyRuleKey:              legacyRuleKey,
		ClusterID:                  normalizeAlertPolicyClusterID(req.ClusterID),
		Severity:                   normalizeAlertPolicySeverity(req.Severity),
		Enabled:                    req.Enabled == nil || *req.Enabled,
		CooldownMinutes:            resolveAlertPolicyCooldownMinutes(req.CooldownMinutes),
		SendRecovery:               req.SendRecovery == nil || *req.SendRecovery,
		PromQL:                     strings.TrimSpace(req.PromQL),
		ConditionsJSON:             conditionsJSON,
		NotificationChannelIDsJSON: channelIDsJSON,
		ReceiverUserIDsJSON:        receiverUserIDsJSON,
		LastExecutionStatus:        AlertPolicyExecutionStatusIdle,
	}
	if req.PolicyType == AlertPolicyBuilderKindSyncJob {
		policy.TaskID = req.TaskID
		policy.NotifyTaskOwner = req.NotifyTaskOwner != nil && *req.NotifyTaskOwner
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
		if err := s.syncAlertPolicyLegacyBridge(ctx, txRepo, policy); err != nil {
			return err
		}
		return s.syncManagedAlertingArtifactsWithRepo(ctx, txRepo)
	}); err != nil {
		return nil, err
	}
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
	template, err := resolveAlertPolicyTemplate(req.PolicyType, req.TemplateKey)
	if err != nil {
		return nil, err
	}
	if err := s.validateAlertPolicyScopes(ctx, req.ClusterID, req.NotificationChannelIDs, req.ReceiverUserIDs); err != nil {
		return nil, err
	}
	legacyRuleKey := resolveAlertPolicyLegacyRuleKey(req.PolicyType, template, req.LegacyRuleKey)
	if err := s.validateAlertPolicyLegacyBridge(ctx, id, req.PolicyType, req.ClusterID, legacyRuleKey); err != nil {
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
	if err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &Repository{db: tx}
		policy, err := txRepo.GetAlertPolicyByID(ctx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertPolicyNotFound
			}
			return err
		}

		previousBridgeRef, err := buildAlertPolicyLegacyBridgeRef(policy)
		if err != nil {
			return err
		}
		// 作业告警策略不能和其他类型互转；所属任务创建后不可改，以库里的为准。
		// Job alert policies cannot switch type; the owning task is immutable and the stored one wins.
		if (policy.PolicyType == AlertPolicyBuilderKindSyncJob) != (req.PolicyType == AlertPolicyBuilderKindSyncJob) {
			return ErrAlertPolicySyncJobTypeChange
		}
		if policy.PolicyType == AlertPolicyBuilderKindSyncJob {
			req.TaskID = policy.TaskID
			if err := s.validateSyncJobPolicy(ctx, txRepo, policy.ID, req); err != nil {
				return err
			}
			if req.NotifyTaskOwner != nil {
				policy.NotifyTaskOwner = *req.NotifyTaskOwner
			}
		}

		policy.Name = strings.TrimSpace(req.Name)
		policy.Description = strings.TrimSpace(req.Description)
		policy.PolicyType = req.PolicyType
		policy.TemplateKey = strings.TrimSpace(req.TemplateKey)
		policy.LegacyRuleKey = legacyRuleKey
		policy.ClusterID = normalizeAlertPolicyClusterID(req.ClusterID)
		policy.Severity = normalizeAlertPolicySeverity(req.Severity)
		policy.PromQL = strings.TrimSpace(req.PromQL)
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

		nextBridgeRef, err := buildAlertPolicyLegacyBridgeRef(policy)
		if err != nil {
			return err
		}
		if previousBridgeRef.HasLegacyRule && !sameAlertPolicyLegacyBridgeRef(previousBridgeRef, nextBridgeRef) {
			if err := s.restoreAlertPolicyLegacyBridge(ctx, txRepo, previousBridgeRef); err != nil {
				return err
			}
		}
		if err := s.syncAlertPolicyLegacyBridge(ctx, txRepo, policy); err != nil {
			return err
		}
		if err := s.syncManagedAlertingArtifactsWithRepo(ctx, txRepo); err != nil {
			return err
		}

		updated = policy
		return nil
	}); err != nil {
		return nil, err
	}
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
	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &Repository{db: tx}
		policy, err := txRepo.GetAlertPolicyByID(ctx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertPolicyNotFound
			}
			return err
		}

		bridgeRef, err := buildAlertPolicyLegacyBridgeRef(policy)
		if err != nil {
			return err
		}
		if bridgeRef.HasLegacyRule {
			if err := s.restoreAlertPolicyLegacyBridge(ctx, txRepo, bridgeRef); err != nil {
				return err
			}
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
		return s.syncManagedAlertingArtifactsWithRepo(ctx, txRepo)
	})
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

func (s *Service) validateAlertPolicyLegacyBridge(ctx context.Context, currentPolicyID uint, policyType AlertPolicyBuilderKind, clusterID string, legacyRuleKey string) error {
	if !isLegacyBackedAlertPolicy(policyType, legacyRuleKey) {
		return nil
	}

	parsedClusterID, hasConcreteCluster, err := parseAlertPolicyConcreteClusterID(clusterID)
	if err != nil {
		return err
	}
	if !hasConcreteCluster {
		return ErrAlertPolicyLegacyBridgeRequiresClusterScope
	}
	if defaultRuleByKey(parsedClusterID, legacyRuleKey) == nil {
		return ErrAlertPolicyLegacyRuleUnsupported
	}

	existingPolicy, err := s.repo.GetAlertPolicyByClusterAndLegacyRuleKey(
		ctx,
		strconv.FormatUint(uint64(parsedClusterID), 10),
		legacyRuleKey,
	)
	if err == nil && existingPolicy != nil && existingPolicy.ID != currentPolicyID {
		return ErrAlertPolicyLegacyBridgeConflict
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func (s *Service) syncAlertPolicyLegacyBridge(ctx context.Context, repo *Repository, policy *AlertPolicy) error {
	bridgeRef, err := buildAlertPolicyLegacyBridgeRef(policy)
	if err != nil {
		return err
	}
	if !bridgeRef.HasLegacyRule {
		return nil
	}

	if err := ensureDefaultRulesWithRepo(ctx, repo, bridgeRef.ClusterID); err != nil {
		return err
	}
	rule, err := repo.GetRuleByClusterAndKey(ctx, bridgeRef.ClusterID, bridgeRef.RuleKey)
	if err != nil {
		return err
	}
	rule.RuleName = strings.TrimSpace(policy.Name)
	rule.Description = strings.TrimSpace(policy.Description)
	rule.Severity = normalizeAlertPolicySeverity(policy.Severity)
	rule.Enabled = policy.Enabled
	return repo.SaveRule(ctx, rule)
}

func (s *Service) restoreAlertPolicyLegacyBridge(ctx context.Context, repo *Repository, bridgeRef alertPolicyLegacyBridgeRef) error {
	if !bridgeRef.HasLegacyRule {
		return nil
	}
	defaultRule := defaultRuleByKey(bridgeRef.ClusterID, bridgeRef.RuleKey)
	if defaultRule == nil {
		return ErrAlertPolicyLegacyRuleUnsupported
	}
	if err := ensureDefaultRulesWithRepo(ctx, repo, bridgeRef.ClusterID); err != nil {
		return err
	}

	rule, err := repo.GetRuleByClusterAndKey(ctx, bridgeRef.ClusterID, bridgeRef.RuleKey)
	if err != nil {
		return err
	}
	rule.RuleName = defaultRule.RuleName
	rule.Description = defaultRule.Description
	rule.Severity = defaultRule.Severity
	rule.Enabled = defaultRule.Enabled
	rule.Threshold = defaultRule.Threshold
	rule.WindowSeconds = defaultRule.WindowSeconds
	return repo.SaveRule(ctx, rule)
}

func normalizeAlertPolicySeverity(severity AlertSeverity) AlertSeverity {
	switch severity {
	case AlertSeverityCritical:
		return AlertSeverityCritical
	default:
		return AlertSeverityWarning
	}
}

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
		PromQL:                 policy.PromQL,
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

func resolveAlertPolicyTemplate(policyType AlertPolicyBuilderKind, templateKey string) (*AlertPolicyTemplateSummaryDTO, error) {
	if policyType == AlertPolicyBuilderKindCustomPromQL || policyType == AlertPolicyBuilderKindSyncJob {
		return nil, nil
	}

	normalizedTemplateKey := strings.TrimSpace(templateKey)
	for _, template := range allAlertPolicyTemplateSummaries() {
		if template == nil || template.Key != normalizedTemplateKey {
			continue
		}
		if template.SourceKind != string(policyType) {
			return nil, ErrAlertPolicyTemplateTypeMismatch
		}
		return template, nil
	}
	return nil, ErrAlertPolicyUnsupportedTemplate
}

func resolveAlertPolicyLegacyRuleKey(policyType AlertPolicyBuilderKind, template *AlertPolicyTemplateSummaryDTO, fallbackLegacyRuleKey string) string {
	if policyType == AlertPolicyBuilderKindCustomPromQL || policyType == AlertPolicyBuilderKindSyncJob {
		return ""
	}
	if template != nil {
		return strings.TrimSpace(template.LegacyRuleKey)
	}
	return strings.TrimSpace(fallbackLegacyRuleKey)
}

func isLegacyBackedAlertPolicy(policyType AlertPolicyBuilderKind, legacyRuleKey string) bool {
	return policyType == AlertPolicyBuilderKindPlatformHealth && strings.TrimSpace(legacyRuleKey) != ""
}

func parseAlertPolicyConcreteClusterID(clusterID string) (uint, bool, error) {
	normalizedClusterID := normalizeAlertPolicyClusterID(clusterID)
	if normalizedClusterID == "all" {
		return 0, false, nil
	}

	parsedClusterID, err := strconv.ParseUint(normalizedClusterID, 10, 32)
	if err != nil {
		return 0, false, ErrAlertPolicyInvalidClusterScope
	}
	return uint(parsedClusterID), true, nil
}

func buildAlertPolicyLegacyBridgeRef(policy *AlertPolicy) (alertPolicyLegacyBridgeRef, error) {
	if policy == nil || !isLegacyBackedAlertPolicy(policy.PolicyType, policy.LegacyRuleKey) {
		return alertPolicyLegacyBridgeRef{}, nil
	}

	clusterID, hasConcreteCluster, err := parseAlertPolicyConcreteClusterID(policy.ClusterID)
	if err != nil {
		return alertPolicyLegacyBridgeRef{}, err
	}
	if !hasConcreteCluster {
		return alertPolicyLegacyBridgeRef{}, nil
	}

	return alertPolicyLegacyBridgeRef{
		ClusterID:     clusterID,
		RuleKey:       strings.TrimSpace(policy.LegacyRuleKey),
		HasLegacyRule: true,
	}, nil
}

func sameAlertPolicyLegacyBridgeRef(left, right alertPolicyLegacyBridgeRef) bool {
	return left.HasLegacyRule == right.HasLegacyRule &&
		left.ClusterID == right.ClusterID &&
		left.RuleKey == right.RuleKey
}

func ensureDefaultRulesWithRepo(ctx context.Context, repo *Repository, clusterID uint) error {
	for _, tpl := range defaultClusterRules(clusterID) {
		_, err := repo.GetRuleByClusterAndKey(ctx, clusterID, tpl.RuleKey)
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if createErr := repo.CreateRule(ctx, tpl); createErr != nil {
			if isDuplicateKeyError(createErr) {
				continue
			}
			return createErr
		}
	}
	return nil
}
