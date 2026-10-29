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
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 同步告警恢复通知的失败重试：恢复通知只在阶段结束时发一次，渠道当时不可用就会永久丢失，
// 所以巡检时按指数退避重试失败的恢复投递，直到成功或达到次数上限。
// Retrying failed sync alert recovery notifications: recovery goes out only once when the episode ends, so a channel
// that is down at that moment would lose it forever. The sweep retries failed recovery deliveries with exponential
// backoff until they succeed or reach the attempt limit.

const (
	// syncRecoveryRetryMaxAttempts 是恢复投递的最大尝试次数（含首次发送）。
	// syncRecoveryRetryMaxAttempts is the maximum number of recovery delivery attempts (the first send included).
	syncRecoveryRetryMaxAttempts = 6
	// syncRecoveryRetryBaseDelay / syncRecoveryRetryMaxDelay 是退避的起始间隔和上限：1、2、4、8、16 分钟。
	// syncRecoveryRetryBaseDelay / syncRecoveryRetryMaxDelay are the initial backoff and its cap: 1, 2, 4, 8, 16 minutes.
	syncRecoveryRetryBaseDelay = time.Minute
	syncRecoveryRetryMaxDelay  = 30 * time.Minute
	syncRecoveryRetryBatchSize = 100
)

// syncRecoveryRetryDelay 返回第 attempt 次失败之后需要等待的时间。
// syncRecoveryRetryDelay returns how long to wait after the attempt-th failure.
func syncRecoveryRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := syncRecoveryRetryBaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= syncRecoveryRetryMaxDelay {
			return syncRecoveryRetryMaxDelay
		}
	}
	return delay
}

// ListRetryableSyncRecoveryDeliveries 返回还没用完重试次数的失败恢复投递（同步告警）。
// ListRetryableSyncRecoveryDeliveries returns failed sync alert recovery deliveries that still have attempts left.
func (r *Repository) ListRetryableSyncRecoveryDeliveries(ctx context.Context, maxAttempts, limit int) ([]*NotificationDelivery, error) {
	var deliveries []*NotificationDelivery
	err := r.db.WithContext(ctx).
		Where("source_type = ? AND event_type = ? AND status IN ? AND attempt_count < ?",
			string(AlertSourceTypeSync), string(NotificationDeliveryEventTypeResolved),
			[]string{string(NotificationDeliveryStatusFailed), string(NotificationDeliveryStatusRetrying)}, maxAttempts).
		Order("updated_at ASC").
		Limit(limit).
		Find(&deliveries).Error
	return deliveries, err
}

// retryFailedRecoveries 重试到期的失败恢复投递；返回的错误只用于日志，不影响其他巡检。
// retryFailedRecoveries retries failed recovery deliveries whose backoff has elapsed; the returned error is only
// logged and never blocks the rest of the sweep.
func (h *SyncAlertHub) retryFailedRecoveries(ctx context.Context) error {
	repo := h.service.repo
	deliveries, err := repo.ListRetryableSyncRecoveryDeliveries(ctx, syncRecoveryRetryMaxAttempts, syncRecoveryRetryBatchSize)
	if err != nil || len(deliveries) == 0 {
		return err
	}
	now := timeNowUTC()
	var due []*NotificationDelivery
	for _, delivery := range deliveries {
		if !delivery.UpdatedAt.IsZero() && now.Before(delivery.UpdatedAt.UTC().Add(syncRecoveryRetryDelay(delivery.AttemptCount))) {
			continue
		}
		due = append(due, delivery)
	}
	if len(due) == 0 {
		return nil
	}

	channels, err := repo.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}
	channelMap := make(map[uint]*NotificationChannel, len(channels))
	for _, channel := range channels {
		if channel != nil {
			channelMap[channel.ID] = channel
		}
	}

	var errs []error
	for _, delivery := range due {
		if err := h.retryRecoveryDelivery(ctx, delivery, channelMap); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// retryRecoveryDelivery 重发一条恢复投递：阶段必须已恢复、之后没有新的告警阶段，策略仍启用且开启恢复通知。
// retryRecoveryDelivery resends one recovery delivery: the episode must be recovered with no newer firing episode,
// and the policy must still be enabled with recovery notifications on.
func (h *SyncAlertHub) retryRecoveryDelivery(ctx context.Context, delivery *NotificationDelivery, channelMap map[uint]*NotificationChannel) error {
	repo := h.service.repo
	episodeID, ok := parseSyncAlertSourceKey(strings.Split(strings.TrimSpace(delivery.SourceKey), ":"))
	if !ok {
		return nil
	}
	episode, err := repo.GetSyncAlertEpisodeByID(ctx, episodeID)
	if err != nil || episode == nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if episode.Status != SyncAlertEpisodeResolved || episode.ResolveReason != syncAlertResolveRecovered {
		return nil
	}
	// 同一个键已经重新告警了：迟到的"已恢复"只会误导，不再补发。
	// The same key is firing again: a late "recovered" would only mislead, so do not send it.
	firing, err := repo.FindFiringSyncAlertEpisode(ctx, episode.Scope, episode.ClusterID, episode.TaskID, episode.RuleKey)
	if err != nil {
		return err
	}
	if firing != nil {
		return nil
	}
	if delivery.PolicyID == 0 {
		return nil
	}
	policy, err := repo.GetAlertPolicyByID(ctx, delivery.PolicyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if policy == nil || !policy.Enabled || !policy.SendRecovery {
		return nil
	}
	channel := channelMap[delivery.ChannelID]
	if channel == nil || !channel.Enabled {
		return nil
	}

	var extra []string
	if episode.Scope == SyncAlertEpisodeScopeTask && policy.NotifyTaskOwner {
		extra = h.service.resolveUserEmails(ctx, episode.TaskCreatorID)
	}
	message := buildSyncAlertMessage(episode, policy, NotificationDeliveryEventTypeResolved, extra, h.clock())
	result, sendErr := h.service.dispatchAlertDelivery(ctx, message, policy, channel, NotificationDeliveryEventTypeResolved, delivery.SourceKey, true)
	if sendErr != nil {
		if delivery.AttemptCount+1 >= syncRecoveryRetryMaxAttempts {
			log.Printf("[Monitoring] sync alert recovery notification gave up after %d attempts: source=%s channel=%d err=%v / 同步告警恢复通知重试次数已用完",
				delivery.AttemptCount+1, delivery.SourceKey, delivery.ChannelID, sendErr)
		}
		return sendErr
	}
	if result != nil && result.DeliveryCountDelta > 0 {
		return repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, &AlertPolicyExecutionStateUpdate{
			DeliveryCountDelta:  result.DeliveryCountDelta,
			LastDeliveredAt:     result.LastDeliveredAt,
			LastExecutionStatus: AlertPolicyExecutionStatusSent,
		})
	}
	return nil
}
