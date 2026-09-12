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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

// ListNotificationDeliveries returns notification delivery history.
// ListNotificationDeliveries 返回通知投递历史。
func (s *Service) ListNotificationDeliveries(ctx context.Context, filter *NotificationDeliveryFilter) (*NotificationDeliveryListData, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if filter == nil {
		filter = &NotificationDeliveryFilter{}
	}
	if filter.Status != "" {
		switch filter.Status {
		case NotificationDeliveryStatusPending,
			NotificationDeliveryStatusSending,
			NotificationDeliveryStatusSent,
			NotificationDeliveryStatusFailed,
			NotificationDeliveryStatusRetrying,
			NotificationDeliveryStatusCanceled:
		default:
			return nil, fmt.Errorf("invalid notification delivery status")
		}
	}
	if filter.EventType != "" {
		switch filter.EventType {
		case NotificationDeliveryEventTypeFiring,
			NotificationDeliveryEventTypeResolved,
			NotificationDeliveryEventTypeTest:
		default:
			return nil, fmt.Errorf("invalid notification delivery event_type")
		}
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}

	rows, total, err := s.repo.ListNotificationDeliveries(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*NotificationDeliveryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toNotificationDeliveryDTO(row))
	}

	return &NotificationDeliveryListData{
		GeneratedAt: time.Now().UTC(),
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Total:       total,
		Deliveries:  items,
	}, nil
}

func (s *Service) deliverRemoteAlertNotifications(ctx context.Context, record *RemoteAlertRecord) error {
	if s.repo == nil || record == nil {
		return nil
	}

	routes, err := s.repo.ListNotificationRoutes(ctx)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
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

	sourceKey := buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt)
	state, err := s.repo.GetAlertStateBySourceKey(ctx, sourceKey)
	if err != nil {
		return err
	}
	currentHandlingStatus := resolveAlertHandlingStatus(state, time.Now().UTC())
	eventType := resolveNotificationDeliveryEventType(record.Status)
	processedChannels := make(map[uint]struct{})

	for _, route := range routes {
		if !matchRemoteAlertRoute(route, record) {
			continue
		}
		if currentHandlingStatus == AlertHandlingStatusClosed {
			continue
		}
		if eventType == NotificationDeliveryEventTypeResolved && !route.SendResolved {
			continue
		}
		if currentHandlingStatus == AlertHandlingStatusAcknowledged && route.MuteIfAcknowledged {
			continue
		}
		if currentHandlingStatus == AlertHandlingStatusSilenced && route.MuteIfSilenced {
			continue
		}

		channel := channelMap[route.ChannelID]
		if channel == nil || !channel.Enabled {
			continue
		}
		if _, exists := processedChannels[channel.ID]; exists {
			continue
		}
		if err := s.dispatchRemoteAlertDelivery(ctx, record, channel, eventType); err != nil {
			return err
		}
		processedChannels[channel.ID] = struct{}{}
	}
	return nil
}

func (s *Service) dispatchRemoteAlertDelivery(ctx context.Context, record *RemoteAlertRecord, channel *NotificationChannel, eventType NotificationDeliveryEventType) error {
	if record == nil || channel == nil {
		return nil
	}

	sourceKey := buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt)
	delivery, err := s.repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(eventType))
	if err != nil {
		return err
	}

	if delivery == nil {
		delivery = &NotificationDelivery{
			AlertID:      sourceKey,
			SourceType:   string(AlertSourceTypeRemoteAlertmanager),
			SourceKey:    sourceKey,
			ClusterID:    strings.TrimSpace(record.ClusterID),
			ClusterName:  strings.TrimSpace(record.ClusterName),
			AlertName:    strings.TrimSpace(record.AlertName),
			ChannelID:    channel.ID,
			ChannelName:  strings.TrimSpace(channel.Name),
			EventType:    string(eventType),
			Status:       string(NotificationDeliveryStatusSending),
			AttemptCount: 1,
		}
		if err := s.repo.CreateNotificationDelivery(ctx, delivery); err != nil {
			return err
		}
	} else {
		if NotificationDeliveryStatus(strings.TrimSpace(delivery.Status)) == NotificationDeliveryStatusSent {
			return nil
		}
		delivery.SourceType = string(AlertSourceTypeRemoteAlertmanager)
		delivery.ClusterID = strings.TrimSpace(record.ClusterID)
		delivery.ClusterName = strings.TrimSpace(record.ClusterName)
		delivery.AlertName = strings.TrimSpace(record.AlertName)
		delivery.ChannelName = strings.TrimSpace(channel.Name)
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
		if err := s.repo.SaveNotificationDelivery(ctx, delivery); err != nil {
			return err
		}
	}

	attempt, sendErr := sendRemoteAlertNotification(ctx, channel, record, eventType)
	if attempt != nil {
		delivery.RequestPayload = attempt.RequestPayload
		delivery.ResponseStatusCode = attempt.StatusCode
		delivery.ResponseBodyExcerpt = attempt.ResponseBody
		delivery.SentAt = attempt.SentAt
	}
	if sendErr != nil {
		delivery.Status = string(NotificationDeliveryStatusFailed)
		delivery.LastError = sendErr.Error()
		return s.repo.SaveNotificationDelivery(ctx, delivery)
	}

	delivery.Status = string(NotificationDeliveryStatusSent)
	delivery.LastError = ""
	return s.repo.SaveNotificationDelivery(ctx, delivery)
}

func sendRemoteAlertNotification(ctx context.Context, channel *NotificationChannel, record *RemoteAlertRecord, eventType NotificationDeliveryEventType) (*notificationSendAttempt, error) {
	payload, err := buildRemoteAlertPayload(channel, record, eventType)
	if err != nil {
		return nil, err
	}
	return sendNotification(ctx, channel, payload)
}

func sendNotification(ctx context.Context, channel *NotificationChannel, payload interface{}) (*notificationSendAttempt, error) {
	if channel == nil {
		return nil, fmt.Errorf("notification channel not found")
	}
	if channel.Type == NotificationChannelTypeEmail {
		return sendEmailNotification(ctx, channel, payload)
	}
	if strings.TrimSpace(channel.Endpoint) == "" {
		return nil, fmt.Errorf("channel endpoint is empty")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(channel.Endpoint), bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &notificationSendAttempt{RequestPayload: string(payloadBytes)}, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	body := string(bodyBytes)
	if len(body) > 1000 {
		body = body[:1000]
	}
	now := time.Now().UTC()
	attempt := &notificationSendAttempt{
		RequestPayload: string(payloadBytes),
		StatusCode:     resp.StatusCode,
		ResponseBody:   body,
		SentAt:         &now,
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return attempt, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return attempt, nil
}

func buildRemoteAlertPayload(channel *NotificationChannel, record *RemoteAlertRecord, eventType NotificationDeliveryEventType) (interface{}, error) {
	if channel == nil {
		return nil, fmt.Errorf("notification channel not found")
	}
	if record == nil {
		return nil, fmt.Errorf("remote alert record is required")
	}

	title := buildRemoteAlertMessageTitle(record, eventType)
	message := buildRemoteAlertMessageText(record, eventType)
	status := strings.ToUpper(string(eventType))
	startsAt := time.Unix(record.StartsAt, 0).UTC().Format(time.RFC3339)
	resolvedAt := ""
	if record.ResolvedAt != nil {
		resolvedAt = record.ResolvedAt.UTC().Format(time.RFC3339)
	}

	switch channel.Type {
	case NotificationChannelTypeWebhook:
		return map[string]interface{}{
			"title":   title,
			"message": message,
			"alert": map[string]interface{}{
				"source_type":  AlertSourceTypeRemoteAlertmanager,
				"source_key":   buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt),
				"event_type":   eventType,
				"status":       strings.ToLower(status),
				"alert_name":   strings.TrimSpace(record.AlertName),
				"severity":     strings.TrimSpace(record.Severity),
				"cluster_id":   strings.TrimSpace(record.ClusterID),
				"cluster_name": strings.TrimSpace(record.ClusterName),
				"receiver":     strings.TrimSpace(record.Receiver),
				"summary":      strings.TrimSpace(record.Summary),
				"description":  strings.TrimSpace(record.Description),
				"fingerprint":  strings.TrimSpace(record.Fingerprint),
				"starts_at":    startsAt,
				"resolved_at":  resolvedAt,
			},
			"sent_at": time.Now().UTC().Format(time.RFC3339),
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
		}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type")
	}
}

func buildRemoteAlertMessageTitle(record *RemoteAlertRecord, eventType NotificationDeliveryEventType) string {
	if record == nil {
		return "[STX][告警] 远程告警"
	}
	eventLabel := "告警"
	if eventType == NotificationDeliveryEventTypeResolved {
		eventLabel = "恢复"
	}
	return fmt.Sprintf(
		"[STX][%s][%s] %s",
		eventLabel,
		resolveAlertSeverityLabelZH(strings.TrimSpace(record.Severity)),
		strings.TrimSpace(firstNonEmpty(record.AlertName, "远程告警")),
	)
}

func buildRemoteAlertMessageText(record *RemoteAlertRecord, eventType NotificationDeliveryEventType) string {
	if record == nil {
		return "STX 远程告警"
	}
	parts := []string{buildRemoteAlertMessageTitle(record, eventType)}
	for _, field := range buildRemoteAlertMessageFields(record, eventType) {
		if strings.TrimSpace(field.Value) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s：%s", field.Label, field.Value))
	}
	return strings.Join(parts, "\n")
}

func buildRemoteAlertMessageHTML(record *RemoteAlertRecord, eventType NotificationDeliveryEventType) string {
	if record == nil {
		return ""
	}
	fields := buildRemoteAlertMessageFields(record, eventType)
	if len(fields) == 0 {
		return ""
	}

	title := buildRemoteAlertMessageTitle(record, eventType)
	visual := resolveLocalAlertMessageVisual(eventType)

	var builder strings.Builder
	builder.WriteString("<!DOCTYPE html><html><body style=\"margin:0;padding:24px;background:#f8fafc;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#0f172a;\">")
	builder.WriteString("<div style=\"max-width:760px;margin:0 auto;background:#ffffff;border:1px solid #e2e8f0;border-radius:16px;overflow:hidden;\">")
	builder.WriteString("<div style=\"padding:20px 24px;background:")
	builder.WriteString(visual.BannerBackground)
	builder.WriteString(";color:#ffffff;\">")
	builder.WriteString("<div style=\"font-size:12px;letter-spacing:0.08em;text-transform:uppercase;opacity:0.78;\">STX 远程告警</div>")
	builder.WriteString("<div style=\"margin-top:8px;font-size:22px;font-weight:700;line-height:1.35;\">")
	builder.WriteString(html.EscapeString(title))
	builder.WriteString("</div>")
	builder.WriteString("<div style=\"margin-top:12px;display:flex;flex-wrap:wrap;gap:8px;align-items:center;\">")
	builder.WriteString("<span style=\"display:inline-block;padding:4px 10px;border-radius:999px;background:rgba(255,255,255,0.18);font-size:12px;font-weight:700;letter-spacing:0.02em;\">")
	builder.WriteString(html.EscapeString(visual.BadgeText))
	builder.WriteString("</span>")
	builder.WriteString("<span style=\"font-size:13px;opacity:0.92;\">级别：")
	builder.WriteString(html.EscapeString(resolveAlertSeverityLabelZH(strings.TrimSpace(record.Severity))))
	builder.WriteString("</span></div>")
	builder.WriteString("<div style=\"margin-top:10px;font-size:13px;line-height:1.7;opacity:0.92;\">")
	builder.WriteString(html.EscapeString(resolveRemoteAlertBannerDescription(eventType)))
	builder.WriteString("</div></div>")
	builder.WriteString("<div style=\"padding:24px;\">")
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
		builder.WriteString(strings.ReplaceAll(html.EscapeString(field.Value), "\n", "<br/>"))
		builder.WriteString("</td></tr>")
	}
	builder.WriteString("</table></div></div></body></html>")
	return builder.String()
}

func buildRemoteAlertMessageFields(record *RemoteAlertRecord, eventType NotificationDeliveryEventType) []localAlertMessageField {
	if record == nil {
		return []localAlertMessageField{}
	}
	labels := parseRemoteAlertLabels(record)
	annotations := parseRemoteAlertAnnotations(record)
	fields := []localAlertMessageField{
		{Label: "状态", Value: resolveRemoteAlertLifecycleLabelZH(eventType)},
		{Label: "集群", Value: fmt.Sprintf("%s (%s)", strings.TrimSpace(firstNonEmpty(record.ClusterName, "未知集群")), strings.TrimSpace(firstNonEmpty(record.ClusterID, "unknown")))},
		{Label: "告警名称", Value: strings.TrimSpace(firstNonEmpty(record.AlertName, labels["alertname"], "远程告警"))},
		{Label: "级别", Value: resolveAlertSeverityLabelZH(strings.TrimSpace(record.Severity))},
		{Label: "触发时间", Value: formatAlertNotificationTime(time.Unix(record.StartsAt, 0).UTC())},
	}
	if eventType == NotificationDeliveryEventTypeResolved && record.ResolvedAt != nil {
		fields = append(fields, localAlertMessageField{Label: "恢复时间", Value: formatAlertNotificationTime(record.ResolvedAt.UTC())})
	}
	appendField := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		fields = append(fields, localAlertMessageField{Label: label, Value: value})
	}
	appendField("实例", labels["instance"])
	appendField("通知接收器", strings.TrimSpace(firstNonEmpty(record.Receiver, managedAlertmanagerDefaultReceiver)))
	appendField("当前值", strings.TrimSpace(firstNonEmpty(annotations["current_value"], annotations["value"])))
	appendField("触发条件", annotations["condition"])
	appendField("摘要", strings.TrimSpace(firstNonEmpty(record.Summary, annotations["summary"])))
	appendField("说明", strings.TrimSpace(firstNonEmpty(record.Description, annotations["description"])))
	appendField("指纹", strings.TrimSpace(firstNonEmpty(record.Fingerprint, "unknown")))
	return fields
}

func resolveRemoteAlertLifecycleLabelZH(eventType NotificationDeliveryEventType) string {
	if eventType == NotificationDeliveryEventTypeResolved {
		return "已恢复"
	}
	return "告警中"
}

func resolveRemoteAlertBannerDescription(eventType NotificationDeliveryEventType) string {
	if eventType == NotificationDeliveryEventTypeResolved {
		return "该指标告警已经恢复，当前条件已不再满足。"
	}
	return "该指标告警当前仍在触发，请尽快检查对应集群和实例。"
}

func resolveAlertSeverityLabelZH(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical":
		return "严重"
	case "warning":
		return "警告"
	default:
		return "提示"
	}
}

func matchRemoteAlertRoute(route *NotificationRoute, record *RemoteAlertRecord) bool {
	if route == nil || record == nil || !route.Enabled {
		return false
	}
	if sourceType := strings.TrimSpace(route.SourceType); sourceType != "" && !strings.EqualFold(sourceType, string(AlertSourceTypeRemoteAlertmanager)) {
		return false
	}
	if clusterID := strings.TrimSpace(route.ClusterID); clusterID != "" && !strings.EqualFold(clusterID, strings.TrimSpace(record.ClusterID)) {
		return false
	}
	if severity := strings.TrimSpace(route.Severity); severity != "" && !strings.EqualFold(severity, strings.TrimSpace(record.Severity)) {
		return false
	}
	if ruleKey := strings.TrimSpace(route.RuleKey); ruleKey != "" && !strings.EqualFold(ruleKey, strings.TrimSpace(record.AlertName)) {
		return false
	}
	return true
}

func resolveNotificationDeliveryEventType(status string) NotificationDeliveryEventType {
	if strings.EqualFold(strings.TrimSpace(status), string(AlertLifecycleStatusResolved)) {
		return NotificationDeliveryEventTypeResolved
	}
	return NotificationDeliveryEventTypeFiring
}

func toNotificationDeliveryDTO(delivery *NotificationDelivery) *NotificationDeliveryDTO {
	if delivery == nil {
		return nil
	}
	return &NotificationDeliveryDTO{
		ID:                 delivery.ID,
		AlertID:            delivery.AlertID,
		SourceType:         delivery.SourceType,
		SourceKey:          delivery.SourceKey,
		PolicyID:           delivery.PolicyID,
		ClusterID:          delivery.ClusterID,
		ClusterName:        delivery.ClusterName,
		AlertName:          delivery.AlertName,
		ChannelID:          delivery.ChannelID,
		ChannelName:        delivery.ChannelName,
		EventType:          delivery.EventType,
		Status:             delivery.Status,
		AttemptCount:       delivery.AttemptCount,
		LastError:          delivery.LastError,
		ResponseStatusCode: delivery.ResponseStatusCode,
		SentAt:             toUTCTimePointer(delivery.SentAt),
		CreatedAt:          delivery.CreatedAt.UTC(),
		UpdatedAt:          delivery.UpdatedAt.UTC(),
	}
}
