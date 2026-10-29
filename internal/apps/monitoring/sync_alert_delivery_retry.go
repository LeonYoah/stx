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

// 同步告警投递的失败重试：首次告警（firing）和恢复通知（resolved）都只在阶段开始 / 结束时发一次，渠道当时不可用就会
// 永久丢失（冷却为 0 时也没有提醒兜底），所以巡检时按指数退避重试失败的投递，直到成功、达到次数上限或不再需要发送。
// 冷却提醒（reminder）不在这里重试：它有自己的投递记录，下个冷却周期自然会再发。
// Retrying failed sync alert deliveries: the first alert (firing) and the recovery (resolved) each go out once when
// the episode starts / ends, so a channel that is down at that moment would lose them forever (with cooldown 0 there
// is no reminder either). The sweep retries failed deliveries with exponential backoff until they succeed, reach the
// attempt limit or are no longer needed. Cooldown reminders are not retried here: they have their own delivery record
// and simply go out again after the next cooldown.

const (
	// syncDeliveryRetryMaxAttempts 是首次告警 / 恢复投递的最大尝试次数（含首次发送）。
	// syncDeliveryRetryMaxAttempts is the maximum number of first alert / recovery attempts (the first send included).
	syncDeliveryRetryMaxAttempts = 6
	// syncDeliveryRetryBaseDelay / syncDeliveryRetryMaxDelay 是退避的起始间隔和上限：1、2、4、8、16 分钟。
	// syncDeliveryRetryBaseDelay / syncDeliveryRetryMaxDelay are the initial backoff and its cap: 1, 2, 4, 8, 16 minutes.
	syncDeliveryRetryBaseDelay = time.Minute
	syncDeliveryRetryMaxDelay  = 30 * time.Minute
	// syncDeliveryRetryBatchSize 是每页扫描条数，也是每轮巡检最多重发的条数。
	// syncDeliveryRetryBatchSize is the page size and the maximum number of resends per sweep.
	syncDeliveryRetryBatchSize = 100
	// syncDeliveryRetryMaxScanPages 限制每轮巡检扫描的页数。
	// syncDeliveryRetryMaxScanPages bounds how many pages one sweep scans.
	syncDeliveryRetryMaxScanPages = 20
)

// syncDeliveryRetryDelay 返回第 attempt 次失败之后需要等待的时间。
// syncDeliveryRetryDelay returns how long to wait after the attempt-th failure.
func syncDeliveryRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := syncDeliveryRetryBaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= syncDeliveryRetryMaxDelay {
			return syncDeliveryRetryMaxDelay
		}
	}
	return delay
}

// ListRetryableSyncAlertDeliveries 按 id 升序分页返回还没用完重试次数的失败首次告警 / 恢复投递（同步告警），
// afterID 为上一页最后一条。
// ListRetryableSyncAlertDeliveries pages (id ASC, after afterID) through failed sync alert first alert / recovery
// deliveries that still have attempts left.
func (r *Repository) ListRetryableSyncAlertDeliveries(ctx context.Context, maxAttempts int, afterID uint, limit int) ([]*NotificationDelivery, error) {
	var deliveries []*NotificationDelivery
	err := r.db.WithContext(ctx).
		Where("source_type = ? AND event_type IN ? AND status IN ? AND attempt_count < ? AND id > ?",
			string(AlertSourceTypeSync),
			[]string{string(NotificationDeliveryEventTypeFiring), string(NotificationDeliveryEventTypeResolved)},
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

// retryFailedRecoveries 保留 sweep（sync_alerts.go）里的调用名，避免与并行修改 sync_alerts.go 的 PR 冲突；
// 现在同时重试首次告警和恢复通知。
// retryFailedRecoveries keeps the name sweep (sync_alerts.go) calls, to avoid conflicts with PRs editing
// sync_alerts.go in parallel; it now retries first alerts as well as recoveries.
func (h *SyncAlertHub) retryFailedRecoveries(ctx context.Context) error {
	return h.retryFailedDeliveries(ctx)
}

// retryFailedDeliveries 重试到期的失败首次告警 / 恢复投递；返回的错误只用于日志，不影响其他巡检。
// 按 id 分页扫描全部可重试记录（最多 syncDeliveryRetryMaxScanPages 页），每轮最多重发 syncDeliveryRetryBatchSize 条；
// 不再需要重试的记录会被标成 canceled，未到退避时间的记录直接跳过，二者都不会挡住后面真正到期的投递。
// retryFailedDeliveries retries failed first alert / recovery deliveries whose backoff has elapsed; the returned error
// is only logged and never blocks the rest of the sweep. It pages by id through every retryable row (at most
// syncDeliveryRetryMaxScanPages pages) and resends at most syncDeliveryRetryBatchSize per sweep; rows that no longer
// need a retry are canceled and rows still in backoff are skipped, so neither can block a later row that is due.
func (h *SyncAlertHub) retryFailedDeliveries(ctx context.Context) error {
	repo := h.service.repo
	now := timeNowUTC()
	var (
		channelMap map[uint]*NotificationChannel
		errs       []error
		afterID    uint
		attempted  int
	)
	for page := 0; page < syncDeliveryRetryMaxScanPages && attempted < syncDeliveryRetryBatchSize; page++ {
		deliveries, err := repo.ListRetryableSyncAlertDeliveries(ctx, syncDeliveryRetryMaxAttempts, afterID, syncDeliveryRetryBatchSize)
		if err != nil {
			errs = append(errs, err)
			break
		}
		if len(deliveries) == 0 {
			break
		}
		afterID = deliveries[len(deliveries)-1].ID
		for _, delivery := range deliveries {
			if attempted >= syncDeliveryRetryBatchSize {
				break
			}
			if !delivery.UpdatedAt.IsZero() && now.Before(delivery.UpdatedAt.UTC().Add(syncDeliveryRetryDelay(delivery.AttemptCount))) {
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
			sent, err := h.retrySyncAlertDelivery(ctx, delivery, channelMap)
			if sent {
				attempted++
			}
			if err != nil {
				errs = append(errs, err)
			}
		}
		if len(deliveries) < syncDeliveryRetryBatchSize {
			break
		}
	}
	return errors.Join(errs...)
}

// retrySyncAlertDelivery 重发一条失败的首次告警或恢复投递；返回是否真的发了一次。
//   - 首次告警：阶段必须仍在告警中、未被人工关闭（静默期间先跳过，不标终态），策略仍启用且仍匹配这条规则；
//   - 恢复：阶段必须已恢复、之后没有新的告警阶段，策略仍启用且开启恢复通知（首次告警必须已送达，见 RequireFiringSent）；
//   - 两者都要求渠道仍存在、启用且仍绑定在策略上；不满足时标记 canceled。
//
// retrySyncAlertDelivery resends one failed first alert or recovery delivery and reports whether it really sent.
//   - first alert: the episode must still be firing and not manually closed (silenced is skipped without a terminal
//     state), and the policy must still be enabled and match the rule;
//   - recovery: the episode must be recovered with no newer firing episode, and the policy must still be enabled with
//     recovery notifications on (the first alert must have been delivered, see RequireFiringSent);
//   - both need the channel to exist, be enabled and still be bound to the policy; otherwise the row is canceled.
func (h *SyncAlertHub) retrySyncAlertDelivery(ctx context.Context, delivery *NotificationDelivery, channelMap map[uint]*NotificationChannel) (bool, error) {
	repo := h.service.repo
	eventType := NotificationDeliveryEventType(strings.TrimSpace(delivery.EventType))
	episodeID, ok := parseSyncAlertSourceKey(strings.Split(strings.TrimSpace(delivery.SourceKey), ":"))
	if !ok {
		return false, h.cancelDeliveryRetry(ctx, delivery, "invalid source key")
	}
	episode, err := repo.GetSyncAlertEpisodeByID(ctx, episodeID)
	if err != nil || episode == nil {
		if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
			return false, h.cancelDeliveryRetry(ctx, delivery, "alert episode no longer exists")
		}
		return false, err
	}

	var policy *AlertPolicy
	switch eventType {
	case NotificationDeliveryEventTypeFiring:
		if episode.Status != SyncAlertEpisodeFiring {
			return false, h.cancelDeliveryRetry(ctx, delivery, "alert episode ended before the alert was delivered")
		}
		state, err := repo.GetAlertStateBySourceKey(ctx, delivery.SourceKey)
		if err != nil {
			return false, err
		}
		switch resolveAlertHandlingStatus(state, timeNowUTC()) {
		case AlertHandlingStatusClosed:
			return false, h.cancelDeliveryRetry(ctx, delivery, "alert was closed")
		case AlertHandlingStatusSilenced:
			// 静默是暂时的：先跳过，静默结束后再按退避重试。
			// Silence is temporary: skip for now and retry with backoff once it ends.
			return false, nil
		}
		if policy, err = h.retryFiringPolicy(ctx, episode, delivery); err != nil || policy == nil {
			return false, err
		}
	case NotificationDeliveryEventTypeResolved:
		if episode.Status != SyncAlertEpisodeResolved || episode.ResolveReason != syncAlertResolveRecovered {
			return false, h.cancelDeliveryRetry(ctx, delivery, "alert episode is not recovered")
		}
		// 同一个键已经重新告警了：迟到的"已恢复"只会误导，不再补发。
		// The same key is firing again: a late "recovered" would only mislead, so do not send it.
		firing, err := repo.FindFiringSyncAlertEpisode(ctx, episode.Scope, episode.ClusterID, episode.TaskID, episode.RuleKey)
		if err != nil {
			return false, err
		}
		if firing != nil {
			return false, h.cancelDeliveryRetry(ctx, delivery, "alert is firing again")
		}
		if policy, err = h.loadRetryPolicy(ctx, delivery); err != nil || policy == nil {
			return false, err
		}
		if !policy.SendRecovery {
			return false, h.cancelDeliveryRetry(ctx, delivery, "recovery notifications are off")
		}
	default:
		return false, h.cancelDeliveryRetry(ctx, delivery, "event type is not retried")
	}

	channel := channelMap[delivery.ChannelID]
	if channel == nil || !channel.Enabled {
		return false, h.cancelDeliveryRetry(ctx, delivery, "notification channel is missing or disabled")
	}
	if !alertPolicyHasChannel(policy, channel.ID) {
		return false, h.cancelDeliveryRetry(ctx, delivery, "notification channel is no longer bound to the policy")
	}

	var extra []string
	if episode.Scope == SyncAlertEpisodeScopeTask && policy.NotifyTaskOwner {
		extra = h.service.resolveUserEmails(ctx, episode.TaskCreatorID)
	}
	message := buildSyncAlertMessage(episode, policy, eventType, extra, h.clock())
	result, sendErr := h.service.dispatchAlertDeliveryWithOptions(ctx, message, policy, channel, eventType, delivery.SourceKey,
		alertDeliveryOptions{RequireFiringSent: eventType == NotificationDeliveryEventTypeResolved, Retry: true})
	if sendErr != nil {
		if delivery.AttemptCount+1 >= syncDeliveryRetryMaxAttempts {
			log.Printf("[Monitoring] sync alert %s notification gave up after %d attempts: source=%s channel=%d err=%v / 同步告警通知重试次数已用完",
				eventType, delivery.AttemptCount+1, delivery.SourceKey, delivery.ChannelID, sendErr)
		}
		return true, sendErr
	}
	if result == nil {
		// 投递前置条件不满足（比如恢复但首次告警未送达）：不会再满足，标记终态。
		// A delivery precondition failed (e.g. recovery while the first alert was never delivered): it will not
		// become true later, so mark the row terminal.
		return false, h.cancelDeliveryRetry(ctx, delivery, "delivery preconditions no longer hold")
	}
	if result.DeliveryCountDelta > 0 {
		return true, repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, &AlertPolicyExecutionStateUpdate{
			DeliveryCountDelta:  result.DeliveryCountDelta,
			LastDeliveredAt:     result.LastDeliveredAt,
			LastExecutionStatus: AlertPolicyExecutionStatusSent,
		})
	}
	return true, nil
}

// loadRetryPolicy 读取投递记录上的策略；策略缺失或停用时标记 canceled 并返回 nil。
// loadRetryPolicy loads the delivery's policy; a missing or disabled policy cancels the row and returns nil.
func (h *SyncAlertHub) loadRetryPolicy(ctx context.Context, delivery *NotificationDelivery) (*AlertPolicy, error) {
	if delivery.PolicyID == 0 {
		return nil, h.cancelDeliveryRetry(ctx, delivery, "delivery has no policy")
	}
	policy, err := h.service.repo.GetAlertPolicyByID(ctx, delivery.PolicyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, h.cancelDeliveryRetry(ctx, delivery, "alert policy no longer exists")
		}
		return nil, err
	}
	if policy == nil || !policy.Enabled {
		return nil, h.cancelDeliveryRetry(ctx, delivery, "alert policy is disabled")
	}
	return policy, nil
}

// retryFiringPolicy 确认首次告警重试时策略仍然适用：任务级要求仍是该任务的策略且勾选了规则；集群级要求集群规则启用、
// 策略仍绑定在这条规则上。不适用时标记 canceled 并返回 nil。
// retryFiringPolicy checks the policy still applies to a first alert retry: for tasks it must still be the task's
// policy with the rule selected; for clusters the cluster rule must be enabled and the policy still bound to it.
// Otherwise the row is canceled and nil is returned.
func (h *SyncAlertHub) retryFiringPolicy(ctx context.Context, episode *SyncAlertEpisode, delivery *NotificationDelivery) (*AlertPolicy, error) {
	policy, err := h.loadRetryPolicy(ctx, delivery)
	if err != nil || policy == nil {
		return nil, err
	}
	if episode.Scope == SyncAlertEpisodeScopeTask {
		current, err := h.service.repo.GetSyncJobAlertPolicyByTaskID(ctx, episode.TaskID)
		if err != nil {
			return nil, err
		}
		if current == nil || current.ID != policy.ID || !syncJobPolicyHasRule(current, episode.RuleKey) {
			return nil, h.cancelDeliveryRetry(ctx, delivery, "alert policy no longer covers the rule")
		}
		return current, nil
	}
	rule, err := h.clusterRule(ctx, episode.ClusterID, episode.RuleKey)
	if err != nil {
		return nil, err
	}
	if rule == nil || !rule.Enabled {
		return nil, h.cancelDeliveryRetry(ctx, delivery, "cluster alert rule is disabled")
	}
	policies, err := h.clusterPolicies(ctx, episode.ClusterID, episode.RuleKey)
	if err != nil {
		return nil, err
	}
	for _, candidate := range policies {
		if candidate != nil && candidate.ID == policy.ID {
			return candidate, nil
		}
	}
	return nil, h.cancelDeliveryRetry(ctx, delivery, "alert policy no longer covers the rule")
}

// alertPolicyHasChannel 判断渠道是否仍绑定在策略上。
// alertPolicyHasChannel reports whether the channel is still bound to the policy.
func alertPolicyHasChannel(policy *AlertPolicy, channelID uint) bool {
	for _, id := range unmarshalAlertPolicyChannelIDs(policy.NotificationChannelIDsJSON) {
		if id == channelID {
			return true
		}
	}
	return false
}

// cancelDeliveryRetry 把不再需要重试的投递标记为 canceled（终态），让它退出后续扫描，不再占用批次。
// cancelDeliveryRetry marks a delivery that no longer needs a retry as canceled (terminal) so it leaves later scans and
// stops taking batch slots.
func (h *SyncAlertHub) cancelDeliveryRetry(ctx context.Context, delivery *NotificationDelivery, reason string) error {
	lastError := "retry canceled: " + reason
	if previous := strings.TrimSpace(delivery.LastError); previous != "" {
		lastError += "; last error: " + previous
	}
	return h.service.repo.MarkNotificationDeliveryCanceled(ctx, delivery.ID, lastError)
}
