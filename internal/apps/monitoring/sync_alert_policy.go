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

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// SyncTaskNameResolver 按 ID 批量查询同步任务名称，由 sync 实现并在路由里注入。
// SyncTaskNameResolver looks up sync task names by ID in bulk; sync implements it and the router injects it.
type SyncTaskNameResolver interface {
	ResolveTaskNames(ctx context.Context, taskIDs []uint) map[uint]string
}

// SetSyncTaskNameResolver 注入任务名查询，用于策略列表显示任务名。
// SetSyncTaskNameResolver injects the task name lookup used by the policy list.
func (s *Service) SetSyncTaskNameResolver(resolver SyncTaskNameResolver) {
	s.syncTaskNames = resolver
}

func (s *Service) resolveSyncTaskNames(ctx context.Context, taskIDs []uint) map[uint]string {
	if s == nil || s.syncTaskNames == nil || len(taskIDs) == 0 {
		return nil
	}
	return s.syncTaskNames.ResolveTaskNames(ctx, taskIDs)
}

// validateSyncJobPolicy 校验作业告警策略：同一任务只能有一条，渠道必须存在且启用，选了邮件渠道时必须有人收得到。
// validateSyncJobPolicy validates a job alert policy: one per task, channels must exist and be enabled, and an email
// channel must reach somebody.
func (s *Service) validateSyncJobPolicy(ctx context.Context, repo *Repository, currentPolicyID uint, req *UpsertAlertPolicyRequest) error {
	existing, err := repo.GetSyncJobAlertPolicyByTaskID(ctx, req.TaskID)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != currentPolicyID {
		return ErrAlertPolicySyncJobConflict
	}
	notifyOwner := req.NotifyTaskOwner != nil && *req.NotifyTaskOwner
	for _, channelID := range normalizeAlertPolicyChannelIDs(req.NotificationChannelIDs) {
		channel, err := repo.GetNotificationChannelByID(ctx, channelID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertPolicyNotificationChannelNotFound
			}
			return err
		}
		if !channel.Enabled {
			return ErrAlertPolicySyncJobChannelDisabled
		}
		if channel.Type != NotificationChannelTypeEmail || len(req.ReceiverUserIDs) > 0 || notifyOwner {
			continue
		}
		channelConfig := unmarshalNotificationChannelConfig(channel.Type, channel.ConfigJSON)
		if channelConfig == nil || channelConfig.Email == nil || len(channelConfig.Email.Recipients) == 0 {
			return ErrAlertPolicySyncJobEmailRecipientMissing
		}
	}
	return nil
}

// closeSyncTaskEpisodes 结束任务全部 firing 阶段（不发通知），用于删除策略。
// closeSyncTaskEpisodes ends every firing episode of the task without notifications; used when a policy is deleted.
func closeSyncTaskEpisodes(ctx context.Context, repo *Repository, taskID uint, reason string) error {
	return closeSyncTaskEpisodesByRules(ctx, repo, taskID, nil, reason)
}

// closeSyncTaskEpisodesByRules 结束任务在给定规则上的 firing 阶段（rules 为空表示全部），不发通知。
// closeSyncTaskEpisodesByRules ends the task's firing episodes for the given rules (all when empty) without
// notifications.
func closeSyncTaskEpisodesByRules(ctx context.Context, repo *Repository, taskID uint, rules []string, reason string) error {
	episodes, err := repo.ListFiringSyncAlertEpisodesByTask(ctx, taskID, rules)
	if err != nil {
		return err
	}
	now := timeNowUTC()
	for _, episode := range episodes {
		episode.Status = SyncAlertEpisodeResolved
		episode.ResolvedAt = &now
		episode.ResolveReason = reason
		if err := repo.SaveSyncAlertEpisode(ctx, episode); err != nil {
			return err
		}
	}
	return nil
}

// GetTaskAlertConfig 读取任务告警配置；没有配置时返回 configured=false。实现 syncalert.TaskPolicyStore。
// GetTaskAlertConfig loads the task alert config; it returns configured=false when none exists. It implements
// syncalert.TaskPolicyStore.
func (s *Service) GetTaskAlertConfig(ctx context.Context, taskID uint) (*syncalert.TaskConfig, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	policy, err := s.repo.GetSyncJobAlertPolicyByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return &syncalert.TaskConfig{Configured: false, Rules: []string{}, ChannelIDs: []uint{}, ReceiverUserIDs: []uint{}}, nil
	}
	return syncJobPolicyToTaskConfig(policy), nil
}

// SaveTaskAlertConfig 新建或更新任务告警策略，立即生效；校验失败包装为 syncalert.ErrInvalidConfig（400）。
// SaveTaskAlertConfig creates or updates the task alert policy, effective immediately; validation failures are
// wrapped as syncalert.ErrInvalidConfig (400).
func (s *Service) SaveTaskAlertConfig(ctx context.Context, task syncalert.TaskRef, cfg *syncalert.TaskConfig) (*syncalert.TaskConfig, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if task.ID == 0 {
		return nil, syncalert.InvalidConfig("task is required")
	}
	normalized := cfg.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	req := taskConfigToPolicyRequest(task, normalized)
	if err := req.Validate(); err != nil {
		return nil, wrapSyncJobPolicyError(err)
	}
	existing, err := s.repo.GetSyncJobAlertPolicyByTaskID(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	var saved *AlertPolicyDTO
	if existing == nil {
		saved, err = s.CreateAlertPolicy(ctx, req)
	} else {
		saved, err = s.UpdateAlertPolicy(ctx, existing.ID, req)
	}
	if err != nil {
		return nil, wrapSyncJobPolicyError(err)
	}
	policy, err := s.repo.GetAlertPolicyByID(ctx, saved.ID)
	if err != nil {
		return nil, err
	}
	return syncJobPolicyToTaskConfig(policy), nil
}

// DeleteTaskAlertConfig 删除任务告警策略并结束告警中的阶段；没有配置时直接返回。
// DeleteTaskAlertConfig deletes the task alert policy and ends its firing episodes; it is a no-op without one.
func (s *Service) DeleteTaskAlertConfig(ctx context.Context, taskID uint) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("monitoring repository is not configured")
	}
	existing, err := s.repo.GetSyncJobAlertPolicyByTaskID(ctx, taskID)
	if err != nil || existing == nil {
		return err
	}
	return s.DeleteAlertPolicy(ctx, existing.ID)
}

// wrapSyncJobPolicyError 把策略层的校验错误转成 sync 能识别的 400 / 409。
// wrapSyncJobPolicyError turns policy-level validation errors into the 400 / 409 sync understands.
func wrapSyncJobPolicyError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrAlertPolicySyncJobConflict):
		return fmt.Errorf("%w: %s", syncalert.ErrConflict, err.Error())
	case getAlertPolicyStatusCode(err) == 400:
		return syncalert.InvalidConfig("%s", err.Error())
	default:
		return err
	}
}

// taskConfigToPolicyRequest 把任务告警配置转成通用策略请求；规则写进条件的 metric_key。
// taskConfigToPolicyRequest turns a task alert config into a generic policy request; rules go into condition
// metric keys.
func taskConfigToPolicyRequest(task syncalert.TaskRef, cfg *syncalert.TaskConfig) *UpsertAlertPolicyRequest {
	conditions := make([]*AlertPolicyConditionDTO, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		condition := &AlertPolicyConditionDTO{MetricKey: rule, Operator: ">=", Threshold: "1"}
		if rule == syncalert.RuleCDCDelayHigh {
			// B4 的阈值（秒）和持续时间（分钟）写进同一条条件，不改表。
			// B4 keeps its threshold (seconds) and duration (minutes) in the same condition; no schema change.
			condition.Threshold = strconv.Itoa(cfg.CDCDelayThresholdSeconds)
			condition.WindowMinutes = cfg.CDCDelayDurationMinutes
		}
		conditions = append(conditions, condition)
	}
	receivers := make([]uint64, 0, len(cfg.ReceiverUserIDs))
	for _, id := range cfg.ReceiverUserIDs {
		receivers = append(receivers, uint64(id))
	}
	clusterID := ""
	if task.ClusterID > 0 {
		clusterID = strconv.FormatUint(uint64(task.ClusterID), 10)
	}
	name := strings.TrimSpace(task.Name)
	if name == "" {
		name = fmt.Sprintf("#%d", task.ID)
	}
	enabled := cfg.Enabled
	cooldown := cfg.CooldownMinutes
	sendRecovery := cfg.SendRecovery
	notifyOwner := cfg.NotifyTaskOwner
	return &UpsertAlertPolicyRequest{
		Name:                   fmt.Sprintf("任务 %s 作业告警", name),
		PolicyType:             AlertPolicyBuilderKindSyncJob,
		TemplateKey:            string(AlertPolicyBuilderKindSyncJob),
		ClusterID:              clusterID,
		Severity:               AlertSeverityCritical,
		Enabled:                &enabled,
		CooldownMinutes:        &cooldown,
		SendRecovery:           &sendRecovery,
		Conditions:             conditions,
		NotificationChannelIDs: cfg.ChannelIDs,
		ReceiverUserIDs:        receivers,
		TaskID:                 task.ID,
		NotifyTaskOwner:        &notifyOwner,
	}
}

// syncJobPolicyToTaskConfig 把策略转回任务告警配置。
// syncJobPolicyToTaskConfig turns a policy back into a task alert config.
func syncJobPolicyToTaskConfig(policy *AlertPolicy) *syncalert.TaskConfig {
	receivers := unmarshalAlertPolicyReceiverUserIDs(policy.ReceiverUserIDsJSON)
	receiverIDs := make([]uint, 0, len(receivers))
	for _, id := range receivers {
		receiverIDs = append(receiverIDs, uint(id))
	}
	channelIDs := unmarshalAlertPolicyChannelIDs(policy.NotificationChannelIDsJSON)
	if channelIDs == nil {
		channelIDs = []uint{}
	}
	rules := syncJobPolicyRules(policy)
	if rules == nil {
		rules = []string{}
	}
	threshold, duration := syncJobPolicyCDCDelayParams(policy)
	return &syncalert.TaskConfig{
		Configured:               true,
		PolicyID:                 policy.ID,
		Enabled:                  policy.Enabled,
		Rules:                    rules,
		ChannelIDs:               channelIDs,
		ReceiverUserIDs:          receiverIDs,
		NotifyTaskOwner:          policy.NotifyTaskOwner,
		CooldownMinutes:          policy.CooldownMinutes,
		SendRecovery:             policy.SendRecovery,
		CDCDelayThresholdSeconds: threshold,
		CDCDelayDurationMinutes:  duration,
	}
}

// syncJobPolicyCDCDelayParams 从策略的 B4 条件取阈值和持续时间；没有 B4 条件或取不到时返回默认值。
// syncJobPolicyCDCDelayParams reads the threshold and duration from the policy's B4 condition; without one (or when
// unreadable) it returns the defaults.
func syncJobPolicyCDCDelayParams(policy *AlertPolicy) (int, int) {
	if policy != nil {
		for _, condition := range unmarshalAlertPolicyConditions(policy.ConditionsJSON) {
			if condition == nil || strings.TrimSpace(condition.MetricKey) != syncalert.RuleCDCDelayHigh {
				continue
			}
			if threshold, duration, err := cdcDelayConditionParams(condition); err == nil {
				return threshold, duration
			}
		}
	}
	return syncalert.DefaultCDCDelayThresholdSeconds, syncalert.DefaultCDCDelayDurationMinutes
}

// cdcDelayConditionParams 解析并校验 B4 条件：threshold 为秒（整数），window_minutes 为分钟；0 或空表示默认值。
// cdcDelayConditionParams parses and checks a B4 condition: threshold in whole seconds, window_minutes in minutes;
// 0 or empty means the default.
func cdcDelayConditionParams(condition *AlertPolicyConditionDTO) (int, int, error) {
	threshold := syncalert.DefaultCDCDelayThresholdSeconds
	if text := strings.TrimSpace(condition.Threshold); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil {
			return 0, 0, ErrAlertPolicySyncJobInvalidCDCDelay
		}
		if value != 0 {
			threshold = value
		}
	}
	duration := condition.WindowMinutes
	if duration == 0 {
		duration = syncalert.DefaultCDCDelayDurationMinutes
	}
	if syncalert.ValidateCDCDelayParams(threshold, duration) != nil {
		return 0, 0, ErrAlertPolicySyncJobInvalidCDCDelay
	}
	return threshold, duration, nil
}
