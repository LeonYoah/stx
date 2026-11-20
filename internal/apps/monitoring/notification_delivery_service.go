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

	"github.com/LeonYoah/stx/internal/pkg/timex"
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
			NotificationDeliveryStatusCanceled,
			NotificationDeliveryStatusExhausted:
		default:
			return nil, fmt.Errorf("invalid notification delivery status")
		}
	}
	if filter.EventType != "" {
		switch filter.EventType {
		case NotificationDeliveryEventTypeFiring,
			NotificationDeliveryEventTypeResolved,
			NotificationDeliveryEventTypeTest,
			NotificationDeliveryEventTypeReminder:
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
		GeneratedAt: timex.Now(),
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Total:       total,
		Deliveries:  items,
	}, nil
}

// notificationSendTimeout 是单次投递（含 SMTP 会话）的总超时，避免一个卡住的渠道拖住整个告警队列。
// notificationSendTimeout bounds one delivery (SMTP session included) so a hung channel cannot stall the alert queue.
var notificationSendTimeout = 60 * time.Second

// notificationResponseExcerptBytes 是投递记录 / 测试结果里保留的平台响应长度。
// notificationResponseExcerptBytes is how much of the platform response is kept in delivery records / test results.
const notificationResponseExcerptBytes = 1000

func sendNotification(ctx context.Context, channel *NotificationChannel, payload interface{}) (*notificationSendAttempt, error) {
	if channel == nil {
		return nil, fmt.Errorf("notification channel not found")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, notificationSendTimeout)
	defer cancel()

	attempt, err := sendNotificationRaw(ctx, channel, payload)
	if attempt != nil {
		attempt.RequestPayload = redactChannelValues(attempt.RequestPayload, channel)
		attempt.ResponseBody = redactChannelValues(attempt.ResponseBody, channel)
	}
	return attempt, redactNotificationError(err, channel)
}

func sendNotificationRaw(ctx context.Context, channel *NotificationChannel, payload interface{}) (*notificationSendAttempt, error) {
	if channel.Type == NotificationChannelTypeEmail {
		return sendEmailNotification(ctx, channel, payload)
	}
	endpoint := strings.TrimSpace(channel.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("channel endpoint is empty")
	}

	payload = fitNotificationPayload(channel.Type, payload)
	signedAt := timex.Now()
	endpoint, payload, err := signNotificationRequest(channel, endpoint, payload, signedAt)
	if err != nil {
		return nil, err
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	applyWebhookSignatureHeaders(req.Header, channel, payloadBytes, signedAt)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &notificationSendAttempt{RequestPayload: string(payloadBytes)}, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	now := timex.Now()
	attempt := &notificationSendAttempt{
		RequestPayload: string(payloadBytes),
		StatusCode:     resp.StatusCode,
		ResponseBody:   truncateUTF8Bytes(string(bodyBytes), notificationResponseExcerptBytes, "…"),
		SentAt:         &now,
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return attempt, fmt.Errorf("http status %d", resp.StatusCode)
	}
	// 钉钉 / 企业微信 / 飞书出错时也返回 HTTP 200，必须看响应体里的业务错误码。
	// DingTalk / WeCom / Feishu return HTTP 200 on errors too, so the business code in the body decides.
	if err := checkPlatformResponse(channel.Type, bodyBytes); err != nil {
		return attempt, err
	}
	return attempt, nil
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
	// 内嵌评估器给所有记录盖的是占位接收器名，对外展示「通知接收器: stx-webhook」没有信息量；
	// 只有旧外部 Alertmanager 写入的真实接收器名（非空且非占位值）才显示。
	// The embedded evaluator stamps a placeholder receiver on every record, so rendering
	// "receiver: stx-webhook" carries no information; only a real receiver name written by the
	// legacy external Alertmanager (non-empty and not the placeholder) is shown.
	if receiver := strings.TrimSpace(record.Receiver); receiver != "" && receiver != managedAlertmanagerDefaultReceiver {
		fields = append(fields, localAlertMessageField{Label: "通知接收器", Value: receiver})
	}
	appendField("当前值", strings.TrimSpace(firstNonEmpty(annotations["current_value"], annotations["value"])))
	appendField("触发条件", annotations["condition"])
	appendField("摘要", truncateAlertFieldText(firstNonEmpty(record.Summary, annotations["summary"])))
	appendField("说明", truncateAlertFieldText(firstNonEmpty(record.Description, annotations["description"])))
	appendField("指纹", strings.TrimSpace(firstNonEmpty(record.Fingerprint, "unknown")))
	appendField("链接", monitoringAlertsConsoleLink(record.ClusterID))
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
		LastError:          redactURLsInText(delivery.LastError),
		ResponseStatusCode: delivery.ResponseStatusCode,
		SentAt:             toUTCTimePointer(delivery.SentAt),
		CreatedAt:          delivery.CreatedAt.UTC(),
		UpdatedAt:          delivery.UpdatedAt.UTC(),
	}
}
