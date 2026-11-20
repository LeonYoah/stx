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
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

type notificationSendAttempt struct {
	RequestPayload string
	StatusCode     int
	ResponseBody   string
	SentAt         *time.Time
}

// TestNotificationChannel performs one test send against a notification channel.
// TestNotificationChannel 对指定通知渠道执行一次测试发送。
func (s *Service) TestNotificationChannel(ctx context.Context, id uint, req *NotificationChannelTestRequest) (*NotificationChannelTestResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if id == 0 {
		return nil, fmt.Errorf("invalid channel id")
	}

	channel, err := s.repo.GetNotificationChannelByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotificationChannelNotFound
		}
		return nil, err
	}

	delivery := &NotificationDelivery{
		AlertID:      fmt.Sprintf("test-channel-%d", channel.ID),
		SourceType:   "system_test",
		SourceKey:    fmt.Sprintf("test:channel:%d:%d", channel.ID, timex.Now().UnixNano()),
		AlertName:    "STX notification test",
		ChannelID:    channel.ID,
		ChannelName:  strings.TrimSpace(channel.Name),
		EventType:    string(NotificationDeliveryEventTypeTest),
		Status:       string(NotificationDeliveryStatusSending),
		AttemptCount: 1,
	}
	if err := s.repo.CreateNotificationDelivery(ctx, delivery); err != nil {
		return nil, err
	}

	recipients, receiver, err := s.resolveNotificationChannelTestRecipients(ctx, channel, req)
	if err != nil {
		return nil, err
	}

	attempt, sendErr := sendTestNotification(ctx, channel, recipients)
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
		return &NotificationChannelTestResult{
			ChannelID:    channel.ID,
			DeliveryID:   delivery.ID,
			Status:       delivery.Status,
			Receiver:     receiver,
			SentAt:       delivery.SentAt,
			LastError:    delivery.LastError,
			StatusCode:   delivery.ResponseStatusCode,
			ResponseBody: delivery.ResponseBodyExcerpt,
		}, nil
	}

	delivery.Status = string(NotificationDeliveryStatusSent)
	if err := s.repo.SaveNotificationDelivery(ctx, delivery); err != nil {
		return nil, err
	}
	return &NotificationChannelTestResult{
		ChannelID:    channel.ID,
		DeliveryID:   delivery.ID,
		Status:       delivery.Status,
		Receiver:     receiver,
		SentAt:       delivery.SentAt,
		StatusCode:   delivery.ResponseStatusCode,
		ResponseBody: delivery.ResponseBodyExcerpt,
	}, nil
}

// TestNotificationChannelDraft performs one test send against an unsaved notification channel draft.
// TestNotificationChannelDraft 对未保存的通知渠道草稿执行一次测试发送。
func (s *Service) TestNotificationChannelDraft(ctx context.Context, req *NotificationChannelDraftTestRequest) (*NotificationChannelTestResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("empty request")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	channelReq, err := s.restoreDraftNotificationChannel(ctx, req.ChannelID, req.Channel)
	if err != nil {
		return nil, err
	}
	channel, err := buildNotificationChannelFromUpsertRequest(channelReq)
	if err != nil {
		return nil, err
	}
	recipients, receiver, err := s.resolveNotificationChannelTestRecipients(ctx, channel, &NotificationChannelTestRequest{
		ReceiverUserID: req.ReceiverUserID,
	})
	if err != nil {
		return nil, err
	}

	attempt, sendErr := sendTestNotification(ctx, channel, recipients)
	result := &NotificationChannelTestResult{
		ChannelID: 0,
		Receiver:  receiver,
	}
	if attempt != nil {
		result.SentAt = attempt.SentAt
		result.StatusCode = attempt.StatusCode
		result.ResponseBody = attempt.ResponseBody
	}
	if sendErr != nil {
		result.Status = string(NotificationDeliveryStatusFailed)
		result.LastError = sendErr.Error()
		return result, nil
	}
	result.Status = string(NotificationDeliveryStatusSent)
	return result, nil
}

// restoreDraftNotificationChannel 用已保存渠道的真实值替换草稿里的脱敏密钥 / 地址；channelID 为 0 时只拒绝占位符。
// restoreDraftNotificationChannel swaps masked secrets / addresses in a draft for the saved channel's real values;
// with channelID 0 it only rejects placeholders.
func (s *Service) restoreDraftNotificationChannel(ctx context.Context, channelID uint, req *UpsertNotificationChannelRequest) (*UpsertNotificationChannelRequest, error) {
	var existing *NotificationChannel
	if channelID > 0 && s.repo != nil {
		channel, err := s.repo.GetNotificationChannelByID(ctx, channelID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrNotificationChannelNotFound
			}
			return nil, err
		}
		existing = channel
	}
	return restoreMaskedNotificationChannelFields(req, existing)
}

// TestSavedNotificationChannelConnection probes a draft connection, reusing the saved channel's masked secrets.
// TestSavedNotificationChannelConnection 测试草稿连接，脱敏的密钥沿用已保存渠道的真实值。
func (s *Service) TestSavedNotificationChannelConnection(ctx context.Context, channelID uint, req *UpsertNotificationChannelRequest) (*NotificationChannelConnectionTestResult, error) {
	if req == nil {
		return nil, fmt.Errorf("empty request")
	}
	restored, err := s.restoreDraftNotificationChannel(ctx, channelID, req)
	if err != nil {
		return nil, err
	}
	return s.TestNotificationChannelConnection(ctx, restored)
}

// TestNotificationChannelConnection validates and probes one draft notification channel connection.
// TestNotificationChannelConnection 校验并测试一份通知渠道草稿配置的连接可用性。
func (s *Service) TestNotificationChannelConnection(ctx context.Context, req *UpsertNotificationChannelRequest) (*NotificationChannelConnectionTestResult, error) {
	if req == nil {
		return nil, fmt.Errorf("empty request")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.Type != NotificationChannelTypeEmail || req.Config == nil || req.Config.Email == nil {
		return nil, fmt.Errorf("only email channel connection test is supported")
	}

	result := &NotificationChannelConnectionTestResult{}
	checkedAt := timex.Now()
	result.CheckedAt = &checkedAt

	emailConfig := req.Config.Email.Normalize()
	if err := emailConfig.Validate(); err != nil {
		return nil, err
	}
	if err := testSMTPConnection(ctx, emailConfig); err != nil {
		result.Status = string(NotificationDeliveryStatusFailed)
		configJSON, _ := marshalNotificationChannelConfig(NotificationChannelTypeEmail, req.Config)
		result.LastError = redactNotificationText(err.Error(), &NotificationChannel{Type: NotificationChannelTypeEmail, ConfigJSON: configJSON})
		return result, nil
	}

	result.Status = string(NotificationDeliveryStatusSent)
	return result, nil
}

func sendTestNotification(ctx context.Context, channel *NotificationChannel, recipients []string) (*notificationSendAttempt, error) {
	payload, err := buildTestPayload(channel, recipients)
	if err != nil {
		return nil, err
	}
	return sendNotification(ctx, channel, payload)
}

func buildTestPayload(channel *NotificationChannel, recipients []string) (interface{}, error) {
	if channel == nil {
		return nil, fmt.Errorf("notification channel not found")
	}

	message := "这是一条来自 STX 的测试通知，收到即表示渠道配置可用。"
	switch channel.Type {
	case NotificationChannelTypeWebhook:
		return map[string]interface{}{
			"title":        "STX 通知测试",
			"message":      message,
			"channel_name": channel.Name,
			"channel_type": channel.Type,
			"sent_at":      timex.Now().Format(time.RFC3339),
		}, nil
	case NotificationChannelTypeWeCom, NotificationChannelTypeDingTalk:
		return map[string]interface{}{
			"msgtype": "text",
			"text": map[string]string{
				"content": fmt.Sprintf("[STX] %s", message),
			},
		}, nil
	case NotificationChannelTypeFeishu:
		return map[string]interface{}{
			"msg_type": "text",
			"content": map[string]string{
				"text": fmt.Sprintf("[STX] %s", message),
			},
		}, nil
	case NotificationChannelTypeEmail:
		return &emailNotificationPayload{
			Subject: "[STX] 通知测试",
			Text:    fmt.Sprintf("[STX] %s", message),
			To:      recipients,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type")
	}
}
