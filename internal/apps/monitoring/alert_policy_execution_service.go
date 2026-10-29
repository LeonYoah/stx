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
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// AlertPolicyExecutionStateUpdate describes aggregate execution changes for one policy.
// AlertPolicyExecutionStateUpdate 描述单条策略的一次执行聚合更新。
type AlertPolicyExecutionStateUpdate struct {
	MatchCountDelta     int
	DeliveryCountDelta  int
	LastMatchedAt       *time.Time
	LastDeliveredAt     *time.Time
	LastExecutionStatus AlertPolicyExecutionStatus
	LastExecutionError  string
}

type alertPolicyChannelDispatchResult struct {
	DeliveryCountDelta int
	LastDeliveredAt    *time.Time
	Successful         bool
	LastError          string
}

type alertPolicyDispatchSummary struct {
	AttemptedChannels   int
	SuccessfulChannels  int
	FailedChannels      int
	DeliveryCountDelta  int
	LastDeliveredAt     *time.Time
	LastExecutionStatus AlertPolicyExecutionStatus
	LastExecutionError  string
}

// DispatchAlertPolicyEvent evaluates one process event against unified alert policies.
// DispatchAlertPolicyEvent 根据统一告警策略执行一次进程事件评估。
func (s *Service) DispatchAlertPolicyEvent(ctx context.Context, event *monitor.ProcessEvent) error {
	if s.repo == nil || event == nil {
		return nil
	}
	if event.EventType == monitor.EventTypeNodeRecovered {
		return s.dispatchLocalResolvedAlertPolicyEvent(ctx, event)
	}
	if !isAlertableEventType(event.EventType) {
		return nil
	}

	ruleKey := eventTypeToRuleKey(event.EventType)
	if strings.TrimSpace(ruleKey) == "" {
		return nil
	}

	clusterID := strconv.FormatUint(uint64(event.ClusterID), 10)
	policies, err := s.repo.ListEnabledAlertPoliciesByClusterAndLegacyRuleKey(ctx, clusterID, ruleKey)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		return nil
	}

	clusterName := ""
	if s.clusterService != nil && event.ClusterID > 0 {
		clusterInfo, err := s.clusterService.Get(ctx, event.ClusterID)
		if err == nil && clusterInfo != nil {
			clusterName = strings.TrimSpace(clusterInfo.Name)
		}
	}

	channels, err := s.repo.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}
	channelMap := make(map[uint]*NotificationChannel, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channelMap[channel.ID] = channel
	}

	matchedAt := localAlertPolicyMatchedAt(event)
	var dispatchErrs []error
	for _, policy := range policies {
		if policy == nil {
			continue
		}

		summary, err := s.dispatchLocalAlertPolicy(ctx, event, clusterName, policy, channelMap)
		if err != nil {
			dispatchErrs = append(dispatchErrs, err)
		}

		stateUpdate := &AlertPolicyExecutionStateUpdate{
			MatchCountDelta:     1,
			DeliveryCountDelta:  summary.DeliveryCountDelta,
			LastMatchedAt:       &matchedAt,
			LastDeliveredAt:     summary.LastDeliveredAt,
			LastExecutionStatus: summary.LastExecutionStatus,
			LastExecutionError:  strings.TrimSpace(summary.LastExecutionError),
		}
		if err := s.repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, stateUpdate); err != nil {
			dispatchErrs = append(dispatchErrs, err)
		}
	}

	return errors.Join(dispatchErrs...)
}

func (s *Service) dispatchLocalAlertPolicyReminder(
	ctx context.Context,
	sourceEvent *monitor.ProcessEvent,
	clusterName string,
	policy *AlertPolicy,
	channelMap map[uint]*NotificationChannel,
) (alertPolicyDispatchSummary, error) {
	return s.dispatchLocalAlertPolicy(ctx, sourceEvent, clusterName, policy, channelMap)
}

func (s *Service) DispatchActiveNodeOfflineReminder(
	ctx context.Context,
	clusterID uint,
	clusterName string,
	nodeID uint,
) error {
	if s.repo == nil || s.monitorService == nil || clusterID == 0 || nodeID == 0 {
		return nil
	}

	sourceEvent, err := s.monitorService.GetLatestNodeEventByTypes(ctx, nodeID, []monitor.ProcessEventType{
		monitor.EventTypeNodeOffline,
	})
	if err != nil {
		return err
	}
	if sourceEvent == nil {
		return nil
	}

	policies, err := s.repo.ListEnabledAlertPoliciesByClusterAndLegacyRuleKey(
		ctx,
		strconv.FormatUint(uint64(clusterID), 10),
		AlertRuleKeyNodeOffline,
	)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		return nil
	}

	channels, err := s.repo.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}
	channelMap := make(map[uint]*NotificationChannel, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channelMap[channel.ID] = channel
	}

	var dispatchErrs []error
	for _, policy := range policies {
		if policy == nil {
			continue
		}

		summary, err := s.dispatchLocalAlertPolicyReminder(
			ctx,
			sourceEvent,
			clusterName,
			policy,
			channelMap,
		)
		if err != nil {
			dispatchErrs = append(dispatchErrs, err)
		}

		stateUpdate := &AlertPolicyExecutionStateUpdate{
			DeliveryCountDelta:  summary.DeliveryCountDelta,
			LastDeliveredAt:     summary.LastDeliveredAt,
			LastExecutionStatus: summary.LastExecutionStatus,
			LastExecutionError:  strings.TrimSpace(summary.LastExecutionError),
		}
		if err := s.repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, stateUpdate); err != nil {
			dispatchErrs = append(dispatchErrs, err)
		}
	}

	return errors.Join(dispatchErrs...)
}

func (s *Service) dispatchLocalResolvedAlertPolicyEvent(ctx context.Context, recoveryEvent *monitor.ProcessEvent) error {
	if s.repo == nil || recoveryEvent == nil || recoveryEvent.EventType != monitor.EventTypeNodeRecovered {
		return nil
	}

	sourceEvent, err := s.resolveLocalRecoveredAlertSourceEvent(ctx, recoveryEvent)
	if err != nil {
		return err
	}
	if sourceEvent == nil {
		return nil
	}

	clusterID := strconv.FormatUint(uint64(sourceEvent.ClusterID), 10)
	policies, err := s.repo.ListEnabledAlertPoliciesByClusterAndLegacyRuleKey(ctx, clusterID, AlertRuleKeyNodeOffline)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		return nil
	}

	clusterName := ""
	if s.clusterService != nil && sourceEvent.ClusterID > 0 {
		clusterInfo, err := s.clusterService.Get(ctx, sourceEvent.ClusterID)
		if err == nil && clusterInfo != nil {
			clusterName = strings.TrimSpace(clusterInfo.Name)
		}
	}

	channels, err := s.repo.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}
	channelMap := make(map[uint]*NotificationChannel, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channelMap[channel.ID] = channel
	}

	var dispatchErrs []error
	for _, policy := range policies {
		if policy == nil || !policy.SendRecovery {
			continue
		}

		summary, err := s.dispatchLocalResolvedAlertPolicy(ctx, sourceEvent, recoveryEvent, clusterName, policy, channelMap)
		if err != nil {
			dispatchErrs = append(dispatchErrs, err)
		}

		stateUpdate := &AlertPolicyExecutionStateUpdate{
			DeliveryCountDelta:  summary.DeliveryCountDelta,
			LastDeliveredAt:     summary.LastDeliveredAt,
			LastExecutionStatus: summary.LastExecutionStatus,
			LastExecutionError:  strings.TrimSpace(summary.LastExecutionError),
		}
		if err := s.repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, stateUpdate); err != nil {
			dispatchErrs = append(dispatchErrs, err)
		}
	}

	return errors.Join(dispatchErrs...)
}

func (s *Service) resolveLocalRecoveredAlertSourceEvent(ctx context.Context, recoveryEvent *monitor.ProcessEvent) (*monitor.ProcessEvent, error) {
	if recoveryEvent == nil || s.monitorService == nil {
		return nil, nil
	}

	if offlineEventID := parseLocalRecoveredOfflineEventID(recoveryEvent.Details); offlineEventID > 0 {
		sourceEvent, err := s.monitorService.GetEvent(ctx, offlineEventID)
		if err != nil && !errors.Is(err, monitor.ErrEventNotFound) {
			return nil, err
		}
		if err == nil && sourceEvent != nil && sourceEvent.EventType == monitor.EventTypeNodeOffline {
			return sourceEvent, nil
		}
	}

	sourceEvent, err := s.monitorService.GetLatestNodeEventByTypes(ctx, recoveryEvent.NodeID, []monitor.ProcessEventType{
		monitor.EventTypeNodeOffline,
	})
	if err != nil {
		return nil, err
	}
	if sourceEvent == nil || !sourceEvent.CreatedAt.Before(recoveryEvent.CreatedAt) {
		return nil, nil
	}
	return sourceEvent, nil
}

func (s *Service) dispatchLocalAlertPolicy(
	ctx context.Context,
	event *monitor.ProcessEvent,
	clusterName string,
	policy *AlertPolicy,
	channelMap map[uint]*NotificationChannel,
) (alertPolicyDispatchSummary, error) {
	if event == nil || policy == nil {
		return alertPolicyDispatchSummary{LastExecutionStatus: AlertPolicyExecutionStatusMatched}, nil
	}
	message := localProcessEventAlertMessage(event, nil, clusterName, policy, NotificationDeliveryEventTypeFiring)
	return s.dispatchAlertPolicyChannels(ctx, message, policy, channelMap, NotificationDeliveryEventTypeFiring, buildLocalAlertSourceKey(event.ID), false)
}

func (s *Service) dispatchLocalResolvedAlertPolicy(
	ctx context.Context,
	sourceEvent *monitor.ProcessEvent,
	recoveryEvent *monitor.ProcessEvent,
	clusterName string,
	policy *AlertPolicy,
	channelMap map[uint]*NotificationChannel,
) (alertPolicyDispatchSummary, error) {
	if sourceEvent == nil || recoveryEvent == nil || policy == nil {
		return alertPolicyDispatchSummary{LastExecutionStatus: AlertPolicyExecutionStatusMatched}, nil
	}
	message := localProcessEventAlertMessage(sourceEvent, recoveryEvent, clusterName, policy, NotificationDeliveryEventTypeResolved)
	return s.dispatchAlertPolicyChannels(ctx, message, policy, channelMap, NotificationDeliveryEventTypeResolved, buildLocalAlertSourceKey(sourceEvent.ID), true)
}

// alertMessage 是与来源无关的一条待投递通知：投递记录需要的字段，加上按渠道拼消息体的函数。
// 本地进程事件和同步作业告警都先组装成它，再走同一条投递路径（去重、冷却重发、静默、关闭、恢复前置条件）。
// alertMessage is one source-agnostic notification to deliver: the fields a delivery record needs plus a function
// that builds the per-channel body. Local process events and sync job alerts both build one and then share the same
// delivery path (dedupe, cooldown resend, silence, close, recovery precondition).
type alertMessage struct {
	SourceType  AlertSourceType
	ClusterID   string
	ClusterName string
	AlertName   string
	// ExtraRecipients 是策略接收人之外还要发送的邮件地址（例如任务创建人），解析失败的已经排除。
	// ExtraRecipients are email addresses to add on top of the policy receivers (such as the task creator); invalid
	// ones are already dropped.
	ExtraRecipients []string
	// Build 按渠道类型和邮件收件人拼出消息体。
	// Build assembles the body for the channel type and email recipients.
	Build func(channel *NotificationChannel, recipients []string) (interface{}, error)
}

// localProcessEventAlertMessage 把本地进程事件包装成 alertMessage，消息内容与重构前一致。
// localProcessEventAlertMessage wraps a local process event as an alertMessage with the same content as before.
func localProcessEventAlertMessage(
	sourceEvent *monitor.ProcessEvent,
	recoveryEvent *monitor.ProcessEvent,
	clusterName string,
	policy *AlertPolicy,
	deliveryEventType NotificationDeliveryEventType,
) *alertMessage {
	return &alertMessage{
		SourceType:  AlertSourceTypeLocalProcessEvent,
		ClusterID:   strconv.FormatUint(uint64(sourceEvent.ClusterID), 10),
		ClusterName: strings.TrimSpace(clusterName),
		AlertName:   strings.TrimSpace(firstNonEmpty(policy.Name, policy.TemplateKey, policy.LegacyRuleKey, "local alert policy")),
		Build: func(channel *NotificationChannel, recipients []string) (interface{}, error) {
			return buildLocalAlertPolicyPayload(channel, sourceEvent, recoveryEvent, clusterName, policy, deliveryEventType, recipients)
		},
	}
}

// dispatchAlertPolicyChannels 把一条通知投递到策略绑定的全部启用渠道，并汇总执行结果。
// dispatchAlertPolicyChannels delivers one notification to every enabled channel of the policy and summarizes the run.
func (s *Service) dispatchAlertPolicyChannels(
	ctx context.Context,
	message *alertMessage,
	policy *AlertPolicy,
	channelMap map[uint]*NotificationChannel,
	deliveryEventType NotificationDeliveryEventType,
	sourceKey string,
	requireFiringSent bool,
) (alertPolicyDispatchSummary, error) {
	summary := alertPolicyDispatchSummary{
		LastExecutionStatus: AlertPolicyExecutionStatusMatched,
	}
	if message == nil || policy == nil {
		return summary, nil
	}

	channelIDs := unmarshalAlertPolicyChannelIDs(policy.NotificationChannelIDsJSON)
	if len(channelIDs) == 0 {
		return summary, nil
	}

	var dispatchErrs []error
	for _, channelID := range channelIDs {
		channel := channelMap[channelID]
		if channel == nil || !channel.Enabled {
			continue
		}

		summary.AttemptedChannels++
		result, err := s.dispatchAlertDelivery(ctx, message, policy, channel, deliveryEventType, sourceKey, requireFiringSent)
		if result != nil {
			if result.Successful {
				summary.SuccessfulChannels++
			}
			if result.DeliveryCountDelta > 0 {
				summary.DeliveryCountDelta += result.DeliveryCountDelta
			}
			summary.LastDeliveredAt = laterUTCTimePointer(summary.LastDeliveredAt, result.LastDeliveredAt)
			if strings.TrimSpace(result.LastError) != "" {
				summary.LastExecutionError = strings.TrimSpace(result.LastError)
			}
		}
		if err != nil {
			summary.FailedChannels++
			summary.LastExecutionError = strings.TrimSpace(err.Error())
			dispatchErrs = append(dispatchErrs, err)
		}
	}

	summary.LastExecutionStatus = summarizeAlertPolicyExecutionStatus(summary)
	if summary.LastExecutionStatus == AlertPolicyExecutionStatusSent {
		summary.LastExecutionError = ""
	}
	if summary.LastExecutionStatus == AlertPolicyExecutionStatusMatched && summary.SuccessfulChannels == 0 && summary.FailedChannels == 0 {
		summary.LastExecutionError = ""
	}
	return summary, errors.Join(dispatchErrs...)
}

func summarizeAlertPolicyExecutionStatus(summary alertPolicyDispatchSummary) AlertPolicyExecutionStatus {
	switch {
	case summary.SuccessfulChannels > 0 && summary.FailedChannels > 0:
		return AlertPolicyExecutionStatusPartial
	case summary.SuccessfulChannels > 0:
		return AlertPolicyExecutionStatusSent
	case summary.FailedChannels > 0:
		return AlertPolicyExecutionStatusFailed
	default:
		return AlertPolicyExecutionStatusMatched
	}
}

func localAlertPolicyMatchedAt(event *monitor.ProcessEvent) time.Time {
	if event == nil || event.CreatedAt.IsZero() {
		return timeNowUTC()
	}
	return event.CreatedAt.UTC()
}

func laterUTCTimePointer(current *time.Time, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	next := candidate.UTC()
	if current == nil || current.Before(next) {
		return &next
	}
	return current
}

// alertDeliveryOptions 控制 dispatchAlertDeliveryWithOptions 的行为。
// alertDeliveryOptions controls dispatchAlertDeliveryWithOptions.
type alertDeliveryOptions struct {
	// RequireFiringSent 时，只有首次告警（firing）投递成功过才发恢复。
	// RequireFiringSent sends recovery only after the first alert (firing) delivery succeeded.
	RequireFiringSent bool
	// Retry 表示由退避重试发起：允许重发一条已存在但未送达的记录。不带 Retry 时，首次告警发送失败的记录交给退避重试，
	// 冷却提醒巡检不会去碰它。
	// Retry marks a call from the backoff retry: an existing undelivered record may be resent. Without it, a first
	// alert whose delivery failed is left to the backoff retry and the cooldown reminder sweep does not touch it.
	Retry bool
}

// dispatchAlertDelivery 投递一条通知到一个渠道：
//   - 人工关闭的不发；静默期间不发告警（恢复照发）；
//   - requireFiringSent 时，只有对应的首次告警发送成功过才发恢复；
//   - 首次告警已送达后，冷却时间到了才发提醒；提醒写独立的 reminder 投递记录，不改首次告警记录的状态；
//   - 首次告警发送失败的记录只由退避重试重发（见 dispatchAlertDeliveryWithOptions 的 Retry）。
//
// dispatchAlertDelivery delivers one notification to one channel:
//   - manually closed alerts are skipped; firing is skipped while silenced (recovery still goes out);
//   - with requireFiringSent, recovery only goes out after the first alert was sent successfully;
//   - once the first alert was delivered, a reminder goes out after the cooldown; reminders write their own reminder
//     delivery record and never change the first alert record's status;
//   - a first alert whose delivery failed is only resent by the backoff retry (Retry in
//     dispatchAlertDeliveryWithOptions).
func (s *Service) dispatchAlertDelivery(
	ctx context.Context,
	message *alertMessage,
	policy *AlertPolicy,
	channel *NotificationChannel,
	deliveryEventType NotificationDeliveryEventType,
	sourceKey string,
	requireFiringSent bool,
) (*alertPolicyChannelDispatchResult, error) {
	return s.dispatchAlertDeliveryWithOptions(ctx, message, policy, channel, deliveryEventType, sourceKey,
		alertDeliveryOptions{RequireFiringSent: requireFiringSent})
}

// dispatchAlertDeliveryWithOptions 是 dispatchAlertDelivery 的完整形式。
// dispatchAlertDeliveryWithOptions is the full form of dispatchAlertDelivery.
func (s *Service) dispatchAlertDeliveryWithOptions(
	ctx context.Context,
	message *alertMessage,
	policy *AlertPolicy,
	channel *NotificationChannel,
	deliveryEventType NotificationDeliveryEventType,
	sourceKey string,
	opts alertDeliveryOptions,
) (*alertPolicyChannelDispatchResult, error) {
	if message == nil || policy == nil || channel == nil {
		return nil, nil
	}

	sourceKey = strings.TrimSpace(sourceKey)
	alertState, err := s.repo.GetAlertStateBySourceKey(ctx, sourceKey)
	if err != nil {
		return nil, err
	}
	handlingStatus := resolveAlertHandlingStatus(alertState, timex.Now())
	if handlingStatus == AlertHandlingStatusClosed {
		return nil, nil
	}
	if handlingStatus == AlertHandlingStatusSilenced && deliveryEventType == NotificationDeliveryEventTypeFiring {
		return nil, nil
	}
	if opts.RequireFiringSent {
		firingDelivery, err := s.repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
		if err != nil {
			return nil, err
		}
		if firingDelivery == nil || NotificationDeliveryStatus(strings.TrimSpace(firingDelivery.Status)) != NotificationDeliveryStatusSent {
			return nil, nil
		}
	}

	delivery, err := s.repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(deliveryEventType))
	if err != nil {
		return nil, err
	}

	if delivery != nil && deliveryEventType == NotificationDeliveryEventTypeFiring {
		if NotificationDeliveryStatus(strings.TrimSpace(delivery.Status)) == NotificationDeliveryStatusSent {
			return s.dispatchAlertReminder(ctx, message, policy, channel, sourceKey, delivery)
		}
		if !opts.Retry {
			// 首次告警还没送达：交给退避重试，这里不重发，也不算一次新的失败。
			// The first alert is not delivered yet: the backoff retry owns it, so neither resend nor count a failure.
			return &alertPolicyChannelDispatchResult{LastError: strings.TrimSpace(delivery.LastError)}, nil
		}
	}

	if delivery == nil {
		delivery = newAlertNotificationDelivery(message, policy, channel, sourceKey, deliveryEventType)
		if err := s.repo.CreateNotificationDelivery(ctx, delivery); err != nil {
			return nil, err
		}
	} else {
		if NotificationDeliveryStatus(strings.TrimSpace(delivery.Status)) == NotificationDeliveryStatusSent {
			return &alertPolicyChannelDispatchResult{
				LastDeliveredAt: toUTCTimePointer(delivery.SentAt),
				Successful:      true,
			}, nil
		}
		resetAlertNotificationDelivery(delivery, message, policy, channel, deliveryEventType)
	}
	return s.sendAndRecordAlertDelivery(ctx, delivery, channel, message, policy)
}

// dispatchAlertReminder 在首次告警已送达且冷却时间已过时发冷却提醒。提醒用 (source_key, 渠道, reminder) 这条独立记录，
// 冷却从"首次告警送达时间"和"上次提醒尝试时间"中较晚的那个算起；提醒失败只记在提醒记录上，下个冷却周期再试。
// dispatchAlertReminder sends a cooldown reminder once the first alert was delivered and the cooldown has elapsed. It
// uses its own (source_key, channel, reminder) record; the cooldown counts from the later of the first alert's send
// time and the last reminder attempt. A failed reminder is only recorded on the reminder record and is tried again
// after the next cooldown.
func (s *Service) dispatchAlertReminder(
	ctx context.Context,
	message *alertMessage,
	policy *AlertPolicy,
	channel *NotificationChannel,
	sourceKey string,
	firing *NotificationDelivery,
) (*alertPolicyChannelDispatchResult, error) {
	delivered := &alertPolicyChannelDispatchResult{LastDeliveredAt: toUTCTimePointer(firing.SentAt), Successful: true}
	if policy.CooldownMinutes <= 0 || firing.SentAt == nil {
		return delivered, nil
	}
	reminder, err := s.repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeReminder))
	if err != nil {
		return nil, err
	}
	last := firing.SentAt.UTC()
	if reminder != nil {
		if reminder.SentAt != nil && reminder.SentAt.UTC().After(last) {
			last = reminder.SentAt.UTC()
			delivered.LastDeliveredAt = toUTCTimePointer(reminder.SentAt)
		}
		if !reminder.UpdatedAt.IsZero() && reminder.UpdatedAt.UTC().After(last) {
			last = reminder.UpdatedAt.UTC()
		}
	}
	if timeNowUTC().Before(last.Add(time.Duration(policy.CooldownMinutes) * time.Minute)) {
		return delivered, nil
	}
	if reminder == nil {
		reminder = newAlertNotificationDelivery(message, policy, channel, sourceKey, NotificationDeliveryEventTypeReminder)
		if err := s.repo.CreateNotificationDelivery(ctx, reminder); err != nil {
			return nil, err
		}
	} else {
		resetAlertNotificationDelivery(reminder, message, policy, channel, NotificationDeliveryEventTypeReminder)
	}
	return s.sendAndRecordAlertDelivery(ctx, reminder, channel, message, policy)
}

func newAlertNotificationDelivery(
	message *alertMessage,
	policy *AlertPolicy,
	channel *NotificationChannel,
	sourceKey string,
	eventType NotificationDeliveryEventType,
) *NotificationDelivery {
	return &NotificationDelivery{
		AlertID:      sourceKey,
		SourceType:   string(message.SourceType),
		SourceKey:    sourceKey,
		PolicyID:     policy.ID,
		ClusterID:    message.ClusterID,
		ClusterName:  message.ClusterName,
		AlertName:    message.AlertName,
		ChannelID:    channel.ID,
		ChannelName:  strings.TrimSpace(channel.Name),
		EventType:    string(eventType),
		Status:       string(NotificationDeliveryStatusSending),
		AttemptCount: 1,
	}
}

func resetAlertNotificationDelivery(
	delivery *NotificationDelivery,
	message *alertMessage,
	policy *AlertPolicy,
	channel *NotificationChannel,
	eventType NotificationDeliveryEventType,
) {
	delivery.ClusterID = message.ClusterID
	delivery.ClusterName = message.ClusterName
	delivery.PolicyID = policy.ID
	delivery.AlertName = strings.TrimSpace(firstNonEmpty(policy.Name, delivery.AlertName))
	delivery.ChannelName = strings.TrimSpace(channel.Name)
	delivery.EventType = string(eventType)
	delivery.Status = string(NotificationDeliveryStatusSending)
	delivery.LastError = ""
	delivery.RequestPayload = ""
	delivery.ResponseStatusCode = 0
	delivery.ResponseBodyExcerpt = ""
	delivery.SentAt = nil
	delivery.AttemptCount++
	if delivery.AttemptCount <= 0 {
		delivery.AttemptCount = 1
	}
}

// sendAndRecordAlertDelivery 发送消息并把结果写回投递记录。
// sendAndRecordAlertDelivery sends the message and writes the outcome back to the delivery record.
func (s *Service) sendAndRecordAlertDelivery(
	ctx context.Context,
	delivery *NotificationDelivery,
	channel *NotificationChannel,
	message *alertMessage,
	policy *AlertPolicy,
) (*alertPolicyChannelDispatchResult, error) {
	attempt, sendErr := s.sendAlertMessage(ctx, channel, message, policy)
	if attempt != nil {
		delivery.RequestPayload = attempt.RequestPayload
		delivery.ResponseStatusCode = attempt.StatusCode
		delivery.ResponseBodyExcerpt = attempt.ResponseBody
		delivery.SentAt = attempt.SentAt
	}
	if sendErr != nil {
		delivery.Status = string(NotificationDeliveryStatusFailed)
		delivery.LastError = sendErr.Error()
		if err := s.repo.SaveNotificationDelivery(ctx, delivery); err != nil {
			return nil, err
		}
		return &alertPolicyChannelDispatchResult{
			LastError: sendErr.Error(),
		}, sendErr
	}

	delivery.Status = string(NotificationDeliveryStatusSent)
	delivery.LastError = ""
	if err := s.repo.SaveNotificationDelivery(ctx, delivery); err != nil {
		return nil, err
	}
	return &alertPolicyChannelDispatchResult{
		DeliveryCountDelta: 1,
		LastDeliveredAt:    toUTCTimePointer(delivery.SentAt),
		Successful:         true,
	}, nil
}

// mergeAlertRecipients 合并策略接收人与额外收件人，按邮箱去重（忽略大小写）。
// mergeAlertRecipients merges policy receivers and extra recipients, de-duplicating emails case-insensitively.
func mergeAlertRecipients(base []string, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	return normalizeEmailRecipients(append(append([]string{}, base...), extra...))
}

// sendAlertMessage 解析邮件收件人（策略接收人加上额外收件人），按渠道拼消息体并发送。
// sendAlertMessage resolves email recipients (policy receivers plus extra recipients), builds the body for the
// channel and sends it.
func (s *Service) sendAlertMessage(
	ctx context.Context,
	channel *NotificationChannel,
	message *alertMessage,
	policy *AlertPolicy,
) (*notificationSendAttempt, error) {
	recipients, err := s.resolveAlertPolicyReceiverEmails(ctx, policy)
	if err != nil {
		return nil, err
	}
	recipients = mergeAlertRecipients(recipients, message.ExtraRecipients)
	payload, err := message.Build(channel, recipients)
	if err != nil {
		return nil, err
	}
	return sendNotification(ctx, channel, payload)
}

func buildLocalAlertPolicyPayload(
	channel *NotificationChannel,
	sourceEvent *monitor.ProcessEvent,
	recoveryEvent *monitor.ProcessEvent,
	clusterName string,
	policy *AlertPolicy,
	deliveryEventType NotificationDeliveryEventType,
	recipients []string,
) (interface{}, error) {
	if channel == nil {
		return nil, fmt.Errorf("notification channel not found")
	}
	if sourceEvent == nil {
		return nil, fmt.Errorf("process event is required")
	}
	if policy == nil {
		return nil, fmt.Errorf("alert policy is required")
	}

	title := buildLocalAlertPolicyMessageTitle(sourceEvent, policy, deliveryEventType)
	message := buildLocalAlertPolicyMessageText(sourceEvent, recoveryEvent, clusterName, policy, deliveryEventType)
	alert := map[string]interface{}{
		"source_type":     AlertSourceTypeLocalProcessEvent,
		"source_key":      buildLocalAlertSourceKey(sourceEvent.ID),
		"event_type":      sourceEvent.EventType,
		"status":          strings.ToLower(string(deliveryEventType)),
		"policy_id":       policy.ID,
		"policy_name":     strings.TrimSpace(policy.Name),
		"policy_type":     policy.PolicyType,
		"template_key":    strings.TrimSpace(policy.TemplateKey),
		"legacy_rule_key": strings.TrimSpace(policy.LegacyRuleKey),
		"severity":        policy.Severity,
		"cluster_id":      strconv.FormatUint(uint64(sourceEvent.ClusterID), 10),
		"cluster_name":    strings.TrimSpace(clusterName),
		"node_id":         sourceEvent.NodeID,
		"host_id":         sourceEvent.HostID,
		"process_name":    strings.TrimSpace(sourceEvent.ProcessName),
		"pid":             sourceEvent.PID,
		"role":            strings.TrimSpace(sourceEvent.Role),
		"event_id":        sourceEvent.ID,
		"fired_at":        sourceEvent.CreatedAt.UTC().Format(time.RFC3339),
		"details":         parseLocalAlertPolicyDetails(sourceEvent.Details),
	}
	if recoveryEvent != nil {
		alert["resolved_at"] = recoveryEvent.CreatedAt.UTC().Format(time.RFC3339)
		alert["resolution_event_id"] = recoveryEvent.ID
		alert["resolution_event_type"] = recoveryEvent.EventType
		alert["recovery_details"] = parseLocalAlertPolicyDetails(recoveryEvent.Details)
	}

	switch channel.Type {
	case NotificationChannelTypeWebhook:
		return map[string]interface{}{
			"title":   title,
			"message": message,
			"alert":   alert,
			"sent_at": timex.Now().Format(time.RFC3339),
		}, nil
	case NotificationChannelTypeWeCom, NotificationChannelTypeDingTalk:
		return map[string]interface{}{
			"msgtype": "text",
			"text": map[string]string{
				"content": message,
			},
		}, nil
	case NotificationChannelTypeFeishu:
		return map[string]interface{}{
			"msg_type": "text",
			"content": map[string]string{
				"text": message,
			},
		}, nil
	case NotificationChannelTypeEmail:
		return &emailNotificationPayload{
			Subject: title,
			Text:    message,
			HTML:    buildLocalAlertPolicyMessageHTML(sourceEvent, recoveryEvent, clusterName, policy, deliveryEventType),
			To:      recipients,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type")
	}
}

func buildLocalAlertPolicyMessageTitle(event *monitor.ProcessEvent, policy *AlertPolicy, deliveryEventType NotificationDeliveryEventType) string {
	severityLevel := AlertSeverityWarning
	policyName := ""
	templateKey := ""
	if policy != nil {
		severityLevel = normalizeAlertPolicySeverity(policy.Severity)
		policyName = policy.Name
		templateKey = policy.TemplateKey
	}
	severity := strings.ToUpper(string(severityLevel))
	if severity == "" {
		severity = "INFO"
	}
	resourceName := strings.TrimSpace(firstNonEmpty(policyName, templateKey, string(event.EventType), "告警策略"))
	if deliveryEventType == NotificationDeliveryEventTypeResolved {
		return fmt.Sprintf("[STX][恢复][%s] %s", resolveAlertSeverityLabelZH(severity), resourceName)
	}
	return fmt.Sprintf(
		"[STX][告警][%s] %s",
		resolveAlertSeverityLabelZH(severity),
		resourceName,
	)
}

func buildLocalAlertPolicyMessageText(
	event *monitor.ProcessEvent,
	recoveryEvent *monitor.ProcessEvent,
	clusterName string,
	policy *AlertPolicy,
	deliveryEventType NotificationDeliveryEventType,
) string {
	fields := buildLocalAlertPolicyMessageFields(event, recoveryEvent, clusterName, policy, deliveryEventType)
	parts := []string{buildLocalAlertPolicyMessageTitle(event, policy, deliveryEventType)}
	for _, field := range fields {
		if strings.TrimSpace(field.Value) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s：%s", field.Label, field.Value))
	}
	return strings.Join(parts, "\n")
}

type localAlertMessageField struct {
	Label     string
	Value     string
	Multiline bool
}

type localAlertMessageVisual struct {
	BadgeText        string
	Description      string
	PanelDescription string
	BannerBackground string
	PanelBackground  string
	PanelBorder      string
}

func buildLocalAlertPolicyMessageHTML(
	event *monitor.ProcessEvent,
	recoveryEvent *monitor.ProcessEvent,
	clusterName string,
	policy *AlertPolicy,
	deliveryEventType NotificationDeliveryEventType,
) string {
	fields := buildLocalAlertPolicyMessageFields(event, recoveryEvent, clusterName, policy, deliveryEventType)
	if len(fields) == 0 {
		return ""
	}

	title := buildLocalAlertPolicyMessageTitle(event, policy, deliveryEventType)
	severityLevel := AlertSeverityWarning
	if policy != nil {
		severityLevel = normalizeAlertPolicySeverity(policy.Severity)
	}
	severity := strings.ToUpper(string(severityLevel))
	if severity == "" {
		severity = "INFO"
	}
	visual := resolveLocalAlertMessageVisual(deliveryEventType)

	return renderAlertMessageHTML(title, severity, visual, fields)
}

// renderAlertMessageHTML 渲染告警邮件的 HTML 正文；本地进程事件和同步作业告警共用。
// renderAlertMessageHTML renders the HTML body of an alert email; shared by local process events and sync job alerts.
func renderAlertMessageHTML(title string, severity string, visual localAlertMessageVisual, fields []localAlertMessageField) string {
	var builder strings.Builder
	builder.WriteString("<!DOCTYPE html><html><body style=\"margin:0;padding:24px;background:#f8fafc;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#0f172a;\">")
	builder.WriteString("<div style=\"max-width:760px;margin:0 auto;background:#ffffff;border:1px solid #e2e8f0;border-radius:16px;overflow:hidden;\">")
	builder.WriteString("<div style=\"padding:20px 24px;background:")
	builder.WriteString(html.EscapeString(visual.BannerBackground))
	builder.WriteString(";color:#ffffff;\">")
	builder.WriteString("<div style=\"font-size:12px;letter-spacing:0.08em;text-transform:uppercase;opacity:0.78;\">STX 告警通知</div>")
	builder.WriteString("<div style=\"margin-top:8px;font-size:22px;font-weight:700;line-height:1.35;\">")
	builder.WriteString(html.EscapeString(title))
	builder.WriteString("</div>")
	builder.WriteString("<div style=\"margin-top:12px;display:flex;flex-wrap:wrap;gap:8px;align-items:center;\">")
	builder.WriteString("<span style=\"display:inline-block;padding:4px 10px;border-radius:999px;background:rgba(255,255,255,0.18);font-size:12px;font-weight:700;letter-spacing:0.02em;\">")
	builder.WriteString(html.EscapeString(visual.BadgeText))
	builder.WriteString("</span>")
	builder.WriteString("<span style=\"font-size:13px;opacity:0.92;\">级别：")
	builder.WriteString(html.EscapeString(resolveAlertSeverityLabelZH(severity)))
	builder.WriteString("</span></div>")
	builder.WriteString("<div style=\"margin-top:10px;font-size:13px;line-height:1.7;opacity:0.92;\">")
	builder.WriteString(html.EscapeString(visual.Description))
	builder.WriteString("</div></div>")
	builder.WriteString("<div style=\"padding:24px;\">")
	builder.WriteString("<div style=\"margin-bottom:16px;padding:14px 16px;border-radius:12px;background:")
	builder.WriteString(html.EscapeString(visual.PanelBackground))
	builder.WriteString(";border:1px solid ")
	builder.WriteString(html.EscapeString(visual.PanelBorder))
	builder.WriteString(";font-size:13px;line-height:1.7;color:#0f172a;\">")
	builder.WriteString(html.EscapeString(visual.PanelDescription))
	builder.WriteString("</div>")
	builder.WriteString("<table role=\"presentation\" width=\"100%\" cellspacing=\"0\" cellpadding=\"0\" style=\"border-collapse:collapse;\">")
	for _, field := range fields {
		if strings.TrimSpace(field.Value) == "" {
			continue
		}
		builder.WriteString("<tr>")
		builder.WriteString("<td style=\"width:180px;padding:10px 12px;border-bottom:1px solid #e2e8f0;background:#f8fafc;font-size:13px;font-weight:600;color:#334155;vertical-align:top;\">")
		builder.WriteString(html.EscapeString(field.Label))
		builder.WriteString("</td>")
		builder.WriteString("<td style=\"padding:10px 12px;border-bottom:1px solid #e2e8f0;font-size:13px;line-height:1.65;color:#0f172a;vertical-align:top;\">")
		if field.Multiline {
			builder.WriteString("<pre style=\"margin:0;white-space:pre-wrap;word-break:break-word;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;color:#0f172a;\">")
			builder.WriteString(html.EscapeString(field.Value))
			builder.WriteString("</pre>")
		} else {
			builder.WriteString(strings.ReplaceAll(html.EscapeString(field.Value), "\n", "<br/>"))
		}
		builder.WriteString("</td></tr>")
	}
	builder.WriteString("</table>")
	builder.WriteString("</div></div></body></html>")
	return builder.String()
}

func buildLocalAlertPolicyMessageFields(
	event *monitor.ProcessEvent,
	recoveryEvent *monitor.ProcessEvent,
	clusterName string,
	policy *AlertPolicy,
	deliveryEventType NotificationDeliveryEventType,
) []localAlertMessageField {
	if event == nil {
		return []localAlertMessageField{}
	}

	policyName := "unknown"
	policyID := uint(0)
	if policy != nil {
		policyName = strings.TrimSpace(firstNonEmpty(policy.Name, "unknown"))
		policyID = policy.ID
	}

	fields := []localAlertMessageField{
		{Label: "状态", Value: resolveLocalAlertDeliveryStateLabel(deliveryEventType)},
		{Label: "集群", Value: fmt.Sprintf("%s (%d)", strings.TrimSpace(firstNonEmpty(clusterName, "unknown")), event.ClusterID)},
		{Label: "策略", Value: fmt.Sprintf("%s (%d)", policyName, policyID)},
		{Label: "触发事件", Value: localizeProcessEventType(event.EventType)},
		{Label: "触发时间", Value: formatAlertNotificationTime(event.CreatedAt)},
	}
	if deliveryEventType == NotificationDeliveryEventTypeResolved && recoveryEvent != nil {
		fields = append(fields, localAlertMessageField{Label: "恢复事件", Value: localizeProcessEventType(recoveryEvent.EventType)})
		fields = append(fields, localAlertMessageField{Label: "恢复时间", Value: formatAlertNotificationTime(recoveryEvent.CreatedAt)})
	}

	details := localAlertPolicyDetailsMap(event.Details)
	appendField := func(label string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		fields = append(fields, localAlertMessageField{Label: label, Value: value})
	}
	appendMultilineField := func(label string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		fields = append(fields, localAlertMessageField{Label: label, Value: value, Multiline: true})
	}

	switch event.EventType {
	case monitor.EventTypeClusterRestartRequested, monitor.EventTypeNodeRestartRequested, monitor.EventTypeNodeStopRequested:
		scope := normalizeOperationScope(localAlertPolicyDetailValue(details, "scope"))
		if scope == "" {
			if event.NodeID > 0 {
				scope = "Node"
			} else {
				scope = "Cluster"
			}
		}
		appendField("范围", scope)
		operation := normalizeOperationName(localAlertPolicyDetailValue(details, "operation"))
		if operation == "" {
			if event.EventType == monitor.EventTypeNodeStopRequested {
				operation = "Stop"
			} else {
				operation = "Restart"
			}
		}
		appendField("操作", operation)
		appendField("操作人", localAlertPolicyDetailValue(details, "operator"))
		appendField("触发来源", localAlertPolicyDetailValue(details, "trigger"))
		appendField("已受理", localAlertPolicyDetailValue(details, "success"))
		appendField("执行结果", truncateAlertFieldText(localAlertPolicyDetailValue(details, "message")))
		appendField("主机", localAlertPolicyDetailValue(details, "host_name"))
		appendField("主机 IP", localAlertPolicyDetailValue(details, "host_ip"))
		appendField("角色", strings.TrimSpace(firstNonEmpty(localAlertPolicyDetailValue(details, "role"), event.Role, "unknown")))
		if event.NodeID > 0 {
			appendField("节点 ID", strconv.FormatUint(uint64(event.NodeID), 10))
		}
		if event.HostID > 0 {
			appendField("主机 ID", strconv.FormatUint(uint64(event.HostID), 10))
		}
	case monitor.EventTypeNodeOffline:
		appendField("原因", truncateAlertFieldText(localAlertPolicyDetailValue(details, "reason")))
		appendField("主机", localAlertPolicyDetailValue(details, "host_name"))
		appendField("主机 IP", localAlertPolicyDetailValue(details, "host_ip"))
		appendField("节点状态", localAlertPolicyDetailValue(details, "node_status"))
		appendField("异常起始", localAlertPolicyDetailValue(details, "observed_since"))
		appendField("宽限秒数", localAlertPolicyDetailValue(details, "grace_seconds"))
		if deliveryEventType == NotificationDeliveryEventTypeResolved && recoveryEvent != nil {
			recoveryDetails := localAlertPolicyDetailsMap(recoveryEvent.Details)
			appendField("恢复检测时间", localAlertPolicyDetailValue(recoveryDetails, "recovered_at"))
			appendField("恢复后节点状态", localAlertPolicyDetailValue(recoveryDetails, "node_status"))
		}
		appendField("角色", strings.TrimSpace(firstNonEmpty(event.Role, "unknown")))
		if event.NodeID > 0 {
			appendField("节点 ID", strconv.FormatUint(uint64(event.NodeID), 10))
		}
		if event.HostID > 0 {
			appendField("主机 ID", strconv.FormatUint(uint64(event.HostID), 10))
		}
	default:
		appendField("原因", truncateAlertFieldText(localAlertPolicyDetailValue(details, "reason")))
		appendField("进程", strings.TrimSpace(firstNonEmpty(event.ProcessName, "unknown")))
		appendField("PID", strconv.Itoa(event.PID))
		appendField("角色", strings.TrimSpace(firstNonEmpty(event.Role, "unknown")))
		if event.NodeID > 0 {
			appendField("节点 ID", strconv.FormatUint(uint64(event.NodeID), 10))
		}
		if event.HostID > 0 {
			appendField("主机 ID", strconv.FormatUint(uint64(event.HostID), 10))
		}
		if prettyDetails := prettyLocalAlertPolicyDetails(event.Details); prettyDetails != "" {
			appendMultilineField("详情", truncateAlertFieldText(prettyDetails))
		}
	}

	if policy != nil {
		if description := strings.TrimSpace(policy.Description); description != "" {
			appendField("策略说明", truncateAlertFieldText(description))
		}
	}
	appendField("链接", monitoringAlertsConsoleLink(strconv.FormatUint(uint64(event.ClusterID), 10)))
	return fields
}

// formatAlertNotificationTime 把时间格式化成一行：展示时区的本地时间加 UTC 偏移，如 "2026-10-29 10:30:00 (UTC+8)"。
// 展示时区来自 app.display_timezone，而不是进程本地时区；结构化的 webhook 字段仍保留 RFC3339 UTC。
// formatAlertNotificationTime renders the time on one line: local time in the display timezone plus its UTC offset,
// e.g. "2026-10-29 10:30:00 (UTC+8)". The display timezone comes from app.display_timezone rather than the process
// zone; structured webhook fields still carry RFC3339 UTC.
func formatAlertNotificationTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	localValue := value.UTC().In(timex.DisplayLocation())
	return fmt.Sprintf("%s (%s)", localValue.Format("2006-01-02 15:04:05"), formatUTCOffsetLabel(localValue))
}

// formatUTCOffsetLabel 返回 "UTC"、"UTC+8"、"UTC-4"、"UTC+5:30" 这样的偏移标签。
// formatUTCOffsetLabel returns an offset label such as "UTC", "UTC+8", "UTC-4" or "UTC+5:30".
func formatUTCOffsetLabel(value time.Time) string {
	_, offset := value.Zone()
	if offset == 0 {
		return "UTC"
	}
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours, minutes := offset/3600, (offset%3600)/60
	if minutes == 0 {
		return fmt.Sprintf("UTC%s%d", sign, hours)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, hours, minutes)
}

func resolveLocalAlertDeliveryStateLabel(deliveryEventType NotificationDeliveryEventType) string {
	if deliveryEventType == NotificationDeliveryEventTypeResolved {
		return "已恢复"
	}
	return "告警中"
}

func resolveLocalAlertMessageVisual(deliveryEventType NotificationDeliveryEventType) localAlertMessageVisual {
	if deliveryEventType == NotificationDeliveryEventTypeResolved {
		return localAlertMessageVisual{
			BadgeText:        "已恢复",
			Description:      "这是恢复通知，表示此前触发的告警条件已经解除，相关资源已回到健康状态。",
			PanelDescription: "该告警已恢复。你可以把这封邮件作为审计记录保留；如果仍需排查根因，再继续后续处理。",
			BannerBackground: "linear-gradient(135deg,#047857 0%,#16a34a 100%)",
			PanelBackground:  "#ecfdf5",
			PanelBorder:      "#86efac",
		}
	}
	return localAlertMessageVisual{
		BadgeText:        "告警中",
		Description:      "这是触发通知，表示当前告警条件仍然成立，可能需要运维立即关注。",
		PanelDescription: "该告警当前仍在触发。如果此策略启用了恢复通知，STX 会在条件恢复后单独发送一封恢复邮件。",
		BannerBackground: "linear-gradient(135deg,#991b1b 0%,#dc2626 100%)",
		PanelBackground:  "#fef2f2",
		PanelBorder:      "#fca5a5",
	}
}

func localAlertPolicyDetailsMap(raw string) map[string]interface{} {
	payload, ok := parseLocalAlertPolicyDetails(raw).(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return payload
}

func localAlertPolicyDetailValue(payload map[string]interface{}, key string) string {
	if len(payload) == 0 {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprintf("%v", payload[key]))
	if value == "" || value == "<nil>" {
		return ""
	}
	return value
}

// localizeProcessEventType 把进程事件类型翻成中文，括号里保留原始值便于检索，如 "进程崩溃 (crashed)"。
// localizeProcessEventType translates a process event type to Chinese and keeps the raw value in parentheses for
// searching, e.g. "进程崩溃 (crashed)".
func localizeProcessEventType(eventType monitor.ProcessEventType) string {
	raw := strings.TrimSpace(string(eventType))
	labels := map[monitor.ProcessEventType]string{
		monitor.EventTypeStarted:                 "进程启动",
		monitor.EventTypeStopped:                 "进程停止",
		monitor.EventTypeCrashed:                 "进程崩溃",
		monitor.EventTypeRestarted:               "进程已重启",
		monitor.EventTypeRestartFailed:           "进程重启失败",
		monitor.EventTypeRestartLimitReached:     "达到重启次数上限",
		monitor.EventTypeClusterRestartRequested: "集群重启请求",
		monitor.EventTypeNodeRestartRequested:    "节点重启请求",
		monitor.EventTypeNodeStopRequested:       "节点停止请求",
		monitor.EventTypeNodeOffline:             "节点离线",
		monitor.EventTypeNodeRecovered:           "节点恢复",
	}
	if raw == "" {
		return "unknown"
	}
	if label, ok := labels[monitor.ProcessEventType(raw)]; ok {
		return fmt.Sprintf("%s (%s)", label, raw)
	}
	return raw
}

func normalizeOperationScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "node":
		return "节点"
	case "cluster":
		return "集群"
	default:
		return ""
	}
}

func normalizeOperationName(operation string) string {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "start":
		return "启动"
	case "stop":
		return "停止"
	case "restart":
		return "重启"
	default:
		return ""
	}
}

func prettyLocalAlertPolicyDetails(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}
	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return trimmed
	}
	return string(pretty)
}

func parseLocalRecoveredOfflineEventID(raw string) uint {
	details := parseLocalAlertPolicyDetails(raw)
	payload, ok := details.(map[string]interface{})
	if !ok {
		return 0
	}

	value := strings.TrimSpace(fmt.Sprintf("%v", payload["offline_event_id"]))
	if value == "" || value == "<nil>" {
		return 0
	}

	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0
	}
	return uint(id)
}

func parseLocalAlertPolicyDetails(raw string) interface{} {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]interface{}{}
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		return payload
	}
	return trimmed
}
