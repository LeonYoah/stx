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

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

func (s *Service) deliverManagedRemoteAlertPolicyNotifications(
	ctx context.Context,
	record *RemoteAlertRecord,
	previousRecord *RemoteAlertRecord,
) (bool, error) {
	policyID, handled := parseManagedRemotePolicyID(record)
	if !handled {
		return false, nil
	}
	if policyID == 0 {
		return true, nil
	}

	policy, err := s.repo.GetAlertPolicyByID(ctx, policyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return true, err
	}
	if policy == nil || !policy.Enabled {
		return true, nil
	}

	summary, err := s.dispatchManagedRemoteAlertPolicy(ctx, record, policy)
	stateUpdate := &AlertPolicyExecutionStateUpdate{
		DeliveryCountDelta:  summary.DeliveryCountDelta,
		LastDeliveredAt:     summary.LastDeliveredAt,
		LastExecutionStatus: summary.LastExecutionStatus,
		LastExecutionError:  strings.TrimSpace(summary.LastExecutionError),
	}
	if previousRecord == nil && strings.EqualFold(strings.TrimSpace(record.Status), string(NotificationDeliveryEventTypeFiring)) {
		matchedAt := time.Unix(record.StartsAt, 0).UTC()
		stateUpdate.MatchCountDelta = 1
		stateUpdate.LastMatchedAt = &matchedAt
	}
	if applyErr := s.repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, stateUpdate); applyErr != nil {
		if err != nil {
			return true, errors.Join(err, applyErr)
		}
		return true, applyErr
	}
	return true, err
}

func (s *Service) dispatchManagedRemoteAlertPolicy(
	ctx context.Context,
	record *RemoteAlertRecord,
	policy *AlertPolicy,
) (alertPolicyDispatchSummary, error) {
	summary := alertPolicyDispatchSummary{LastExecutionStatus: AlertPolicyExecutionStatusMatched}
	if record == nil || policy == nil {
		return summary, nil
	}

	eventType := resolveNotificationDeliveryEventType(record.Status)
	if eventType == NotificationDeliveryEventTypeResolved && !policy.SendRecovery {
		return summary, nil
	}

	channelIDs := unmarshalAlertPolicyChannelIDs(policy.NotificationChannelIDsJSON)
	if len(channelIDs) == 0 {
		return summary, nil
	}

	channels, err := s.repo.ListNotificationChannels(ctx)
	if err != nil {
		return summary, err
	}
	channelMap := make(map[uint]*NotificationChannel, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channelMap[channel.ID] = channel
	}

	var dispatchErrs []error
	sourceKey := buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt)
	requireFiringSent := eventType == NotificationDeliveryEventTypeResolved
	for _, channelID := range channelIDs {
		channel := channelMap[channelID]
		if channel == nil || !channel.Enabled {
			continue
		}
		summary.AttemptedChannels++
		result, dispatchErr := s.dispatchManagedRemoteAlertPolicyDelivery(
			ctx,
			record,
			policy,
			channel,
			eventType,
			sourceKey,
			requireFiringSent,
		)
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
		if dispatchErr != nil {
			summary.FailedChannels++
			summary.LastExecutionError = strings.TrimSpace(dispatchErr.Error())
			dispatchErrs = append(dispatchErrs, dispatchErr)
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

func (s *Service) dispatchManagedRemoteAlertPolicyDelivery(
	ctx context.Context,
	record *RemoteAlertRecord,
	policy *AlertPolicy,
	channel *NotificationChannel,
	eventType NotificationDeliveryEventType,
	sourceKey string,
	requireFiringSent bool,
) (*alertPolicyChannelDispatchResult, error) {
	if record == nil || policy == nil || channel == nil {
		return nil, nil
	}

	alertState, err := s.repo.GetAlertStateBySourceKey(ctx, sourceKey)
	if err != nil {
		return nil, err
	}
	handlingStatus := resolveAlertHandlingStatus(alertState, timeNowUTC())
	if handlingStatus == AlertHandlingStatusClosed {
		return nil, nil
	}
	if handlingStatus == AlertHandlingStatusSilenced && eventType == NotificationDeliveryEventTypeFiring {
		return nil, nil
	}
	if requireFiringSent {
		firingDelivery, err := s.repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
		if err != nil {
			return nil, err
		}
		if firingDelivery == nil || NotificationDeliveryStatus(strings.TrimSpace(firingDelivery.Status)) != NotificationDeliveryStatusSent {
			return nil, nil
		}
	}

	delivery, err := s.repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(eventType))
	if err != nil {
		return nil, err
	}
	if delivery == nil {
		delivery = &NotificationDelivery{
			AlertID:      sourceKey,
			SourceType:   string(AlertSourceTypeRemoteAlertmanager),
			SourceKey:    sourceKey,
			PolicyID:     policy.ID,
			ClusterID:    strings.TrimSpace(record.ClusterID),
			ClusterName:  strings.TrimSpace(record.ClusterName),
			AlertName:    strings.TrimSpace(firstNonEmpty(record.AlertName, policy.Name, policy.TemplateKey, "远程指标告警")),
			ChannelID:    channel.ID,
			ChannelName:  strings.TrimSpace(channel.Name),
			EventType:    string(eventType),
			Status:       string(NotificationDeliveryStatusSending),
			AttemptCount: 1,
		}
		if err := s.repo.CreateNotificationDelivery(ctx, delivery); err != nil {
			return nil, err
		}
	} else {
		if NotificationDeliveryStatus(strings.TrimSpace(delivery.Status)) == NotificationDeliveryStatusSent {
			if !shouldResendManagedRemoteAlertPolicyDelivery(policy, delivery, eventType, timeNowUTC()) {
				return &alertPolicyChannelDispatchResult{
					LastDeliveredAt: toUTCTimePointer(delivery.SentAt),
					Successful:      true,
				}, nil
			}
		}
		delivery.PolicyID = policy.ID
		delivery.ClusterID = strings.TrimSpace(record.ClusterID)
		delivery.ClusterName = strings.TrimSpace(record.ClusterName)
		delivery.AlertName = strings.TrimSpace(firstNonEmpty(record.AlertName, policy.Name, delivery.AlertName))
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

	attempt, sendErr := s.sendManagedRemoteAlertPolicyNotification(ctx, channel, record, policy, eventType)
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
		return &alertPolicyChannelDispatchResult{LastError: sendErr.Error()}, sendErr
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

func shouldResendManagedRemoteAlertPolicyDelivery(
	policy *AlertPolicy,
	delivery *NotificationDelivery,
	eventType NotificationDeliveryEventType,
	now time.Time,
) bool {
	if policy == nil || delivery == nil {
		return false
	}
	if eventType != NotificationDeliveryEventTypeFiring {
		return false
	}
	if policy.CooldownMinutes <= 0 || delivery.SentAt == nil {
		return false
	}
	return !now.Before(delivery.SentAt.UTC().Add(time.Duration(policy.CooldownMinutes) * time.Minute))
}

func (s *Service) sendManagedRemoteAlertPolicyNotification(
	ctx context.Context,
	channel *NotificationChannel,
	record *RemoteAlertRecord,
	policy *AlertPolicy,
	eventType NotificationDeliveryEventType,
) (*notificationSendAttempt, error) {
	recipients, err := s.resolveAlertPolicyReceiverEmails(ctx, policy)
	if err != nil {
		return nil, err
	}
	payload, err := buildManagedRemoteAlertPolicyPayload(channel, record, policy, eventType, recipients)
	if err != nil {
		return nil, err
	}
	return sendNotification(ctx, channel, payload)
}

func buildManagedRemoteAlertPolicyPayload(
	channel *NotificationChannel,
	record *RemoteAlertRecord,
	policy *AlertPolicy,
	eventType NotificationDeliveryEventType,
	recipients []string,
) (interface{}, error) {
	if channel == nil {
		return nil, fmt.Errorf("notification channel not found")
	}
	if record == nil {
		return nil, fmt.Errorf("remote alert record is required")
	}

	title := buildRemoteAlertMessageTitle(record, eventType)
	message := buildRemoteAlertMessageText(record, eventType)
	labels := parseRemoteAlertLabels(record)
	annotations := parseRemoteAlertAnnotations(record)
	alert := map[string]interface{}{
		"source_type":  AlertSourceTypeRemoteAlertmanager,
		"source_key":   buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt),
		"event_type":   eventType,
		"status":       strings.ToLower(string(eventType)),
		"policy_id":    policy.ID,
		"policy_name":  strings.TrimSpace(policy.Name),
		"template_key": strings.TrimSpace(policy.TemplateKey),
		"cluster_id":   strings.TrimSpace(record.ClusterID),
		"cluster_name": strings.TrimSpace(record.ClusterName),
		"severity":     strings.TrimSpace(record.Severity),
		"alert_name":   strings.TrimSpace(record.AlertName),
		"labels":       labels,
		"annotations":  annotations,
		"starts_at":    time.Unix(record.StartsAt, 0).UTC().Format(time.RFC3339),
	}
	if record.ResolvedAt != nil {
		alert["resolved_at"] = record.ResolvedAt.UTC().Format(time.RFC3339)
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
			HTML:    buildRemoteAlertMessageHTML(record, eventType),
			To:      recipients,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type")
	}
}

func policyIDLabel(policy *AlertPolicy) string {
	if policy == nil {
		return ""
	}
	return strconv.FormatUint(uint64(policy.ID), 10)
}
