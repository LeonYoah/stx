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
	// syncRecoveryRetryBatchSize 是每页扫描条数，也是每轮巡检最多重发的条数。
	// syncRecoveryRetryBatchSize is the page size and the maximum number of resends per sweep.
	syncRecoveryRetryBatchSize = 100
	// syncRecoveryRetryMaxScanPages 限制每轮巡检扫描的页数。
	// syncRecoveryRetryMaxScanPages bounds how many pages one sweep scans.
	syncRecoveryRetryMaxScanPages = 20
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

// ListRetryableSyncRecoveryDeliveries 按 id 升序分页返回还没用完重试次数的失败恢复投递（同步告警），afterID 为上一页最后一条。
// ListRetryableSyncRecoveryDeliveries pages (id ASC, after afterID) through failed sync alert recovery deliveries that
// still have attempts left.
func (r *Repository) ListRetryableSyncRecoveryDeliveries(ctx context.Context, maxAttempts int, afterID uint, limit int) ([]*NotificationDelivery, error) {
	var deliveries []*NotificationDelivery
	err := r.db.WithContext(ctx).
		Where("source_type = ? AND event_type = ? AND status IN ? AND attempt_count < ? AND id > ?",
			string(AlertSourceTypeSync), string(NotificationDeliveryEventTypeResolved),
			[]string{string(NotificationDeliveryStatusFailed), string(NotificationDeliveryStatusRetrying)}, maxAttempts, afterID).
		Order("id ASC").
		Limit(limit).
		Find(&deliveries).Error
	return deliveries, err
}

// MarkNotificationDeliveryCanceled 把一条投递记录置为 canceled 并记录原因。
// MarkNotificationDeliveryCanceled sets one delivery record to canceled and records the reason.
func (r *Repository) MarkNotificationDeliveryCanceled(ctx context.Context, id uint, lastError string) error {
	return r.db.WithContext(ctx).Model(&NotificationDelivery{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     string(NotificationDeliveryStatusCanceled),
			"last_error": lastError,
		}).Error
}

// retryFailedRecoveries 重试到期的失败恢复投递；返回的错误只用于日志，不影响其他巡检。
// 按 id 分页扫描全部可重试记录（最多 syncRecoveryRetryMaxScanPages 页），每轮最多重发 syncRecoveryRetryBatchSize 条；
// 不再需要重试的记录会被标成 canceled，未到退避时间的记录直接跳过，二者都不会挡住后面真正到期的投递。
// retryFailedRecoveries retries failed recovery deliveries whose backoff has elapsed; the returned error is only
// logged and never blocks the rest of the sweep. It pages by id through every retryable row (at most
// syncRecoveryRetryMaxScanPages pages) and resends at most syncRecoveryRetryBatchSize per sweep; rows that no longer
// need a retry are canceled and rows still in backoff are skipped, so neither can block a later row that is due.
func (h *SyncAlertHub) retryFailedRecoveries(ctx context.Context) error {
	repo := h.service.repo
	now := timeNowUTC()
	var (
		channelMap map[uint]*NotificationChannel
		errs       []error
		afterID    uint
		attempted  int
	)
	for page := 0; page < syncRecoveryRetryMaxScanPages && attempted < syncRecoveryRetryBatchSize; page++ {
		deliveries, err := repo.ListRetryableSyncRecoveryDeliveries(ctx, syncRecoveryRetryMaxAttempts, afterID, syncRecoveryRetryBatchSize)
		if err != nil {
			errs = append(errs, err)
			break
		}
		if len(deliveries) == 0 {
			break
		}
		afterID = deliveries[len(deliveries)-1].ID
		for _, delivery := range deliveries {
			if attempted >= syncRecoveryRetryBatchSize {
				break
			}
			if !delivery.UpdatedAt.IsZero() && now.Before(delivery.UpdatedAt.UTC().Add(syncRecoveryRetryDelay(delivery.AttemptCount))) {
				continue
			}
			if channelMap == nil {
				channels, err := repo.ListNotificationChannels(ctx)
				if err != nil {
					return errors.Join(append(errs, err)...)
				}
				channelMap = make(map[uint]*NotificationChannel, len(channels))
				for _, channel := range channels {
					if channel != nil {
						channelMap[channel.ID] = channel
					}
				}
			}
			sent, err := h.retryRecoveryDelivery(ctx, delivery, channelMap)
			if sent {
				attempted++
			}
			if err != nil {
				errs = append(errs, err)
			}
		}
		if len(deliveries) < syncRecoveryRetryBatchSize {
			break
		}
	}
	return errors.Join(errs...)
}

// retryRecoveryDelivery 重发一条恢复投递：阶段必须已恢复、之后没有新的告警阶段，策略仍启用且开启恢复通知。
// retryRecoveryDelivery resends one recovery delivery: the episode must be recovered with no newer firing episode,
// and the policy must still be enabled with recovery notifications on.
func (h *SyncAlertHub) retryRecoveryDelivery(ctx context.Context, delivery *NotificationDelivery, channelMap map[uint]*NotificationChannel) (bool, error) {
	repo := h.service.repo
	episodeID, ok := parseSyncAlertSourceKey(strings.Split(strings.TrimSpace(delivery.SourceKey), ":"))
	if !ok {
		return false, h.cancelRecoveryRetry(ctx, delivery, "invalid source key")
	}
	episode, err := repo.GetSyncAlertEpisodeByID(ctx, episodeID)
	if err != nil || episode == nil {
		if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
			return false, h.cancelRecoveryRetry(ctx, delivery, "alert episode no longer exists")
		}
		return false, err
	}
	if episode.Status != SyncAlertEpisodeResolved || episode.ResolveReason != syncAlertResolveRecovered {
		return false, h.cancelRecoveryRetry(ctx, delivery, "alert episode is not recovered")
	}
	// 同一个键已经重新告警了：迟到的"已恢复"只会误导，不再补发。
	// The same key is firing again: a late "recovered" would only mislead, so do not send it.
	firing, err := repo.FindFiringSyncAlertEpisode(ctx, episode.Scope, episode.ClusterID, episode.TaskID, episode.RuleKey)
	if err != nil {
		return false, err
	}
	if firing != nil {
		return false, h.cancelRecoveryRetry(ctx, delivery, "alert is firing again")
	}
	if delivery.PolicyID == 0 {
		return false, h.cancelRecoveryRetry(ctx, delivery, "delivery has no policy")
	}
	policy, err := repo.GetAlertPolicyByID(ctx, delivery.PolicyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, h.cancelRecoveryRetry(ctx, delivery, "alert policy no longer exists")
		}
		return false, err
	}
	if policy == nil || !policy.Enabled || !policy.SendRecovery {
		return false, h.cancelRecoveryRetry(ctx, delivery, "alert policy is disabled or recovery notifications are off")
	}
	channel := channelMap[delivery.ChannelID]
	if channel == nil || !channel.Enabled {
		return false, h.cancelRecoveryRetry(ctx, delivery, "notification channel is missing or disabled")
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
		return true, sendErr
	}
	if result != nil && result.DeliveryCountDelta > 0 {
		return true, repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, &AlertPolicyExecutionStateUpdate{
			DeliveryCountDelta:  result.DeliveryCountDelta,
			LastDeliveredAt:     result.LastDeliveredAt,
			LastExecutionStatus: AlertPolicyExecutionStatusSent,
		})
	}
	return true, nil
}

// cancelRecoveryRetry 把不再需要重试的恢复投递标记为 canceled（终态），让它退出后续扫描，不再占用批次。
// cancelRecoveryRetry marks a recovery delivery that no longer needs a retry as canceled (terminal) so it leaves later
// scans and stops taking batch slots.
func (h *SyncAlertHub) cancelRecoveryRetry(ctx context.Context, delivery *NotificationDelivery, reason string) error {
	lastError := "recovery retry canceled: " + reason
	if previous := strings.TrimSpace(delivery.LastError); previous != "" {
		lastError += "; last error: " + previous
	}
	return h.service.repo.MarkNotificationDeliveryCanceled(ctx, delivery.ID, lastError)
}
