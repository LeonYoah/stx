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
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 同步告警投递的失败重试：首次告警（firing）和恢复通知（resolved）都只在阶段开始 / 结束时发一次，渠道当时不可用就会
// 永久丢失（冷却为 0 时也没有提醒兜底），所以巡检时按指数退避重试失败的投递，直到成功、达到次数上限或不再需要发送。
//   - 恢复通知：最多 syncDeliveryRetryMaxAttempts 次，用完标记 exhausted（终态）。
//   - 首次告警：前 syncDeliveryRetryMaxAttempts 次按 1/2/4/8/16 分钟退避，用完标记 exhausted，但只要告警仍在触发就
//     继续每 syncDeliveryRetryMaxDelay（30 分钟）重试一次——告警还在，就不放弃让用户知道（否则冷却提醒和恢复通知都
//     永远发不出去，因为它们都以首次告警送达为前提）。
// 冷却提醒（reminder）不在这里重试：它有自己的投递记录，下个冷却周期自然会再发。
// Retrying failed sync alert deliveries: the first alert (firing) and the recovery (resolved) each go out once when
// the episode starts / ends, so a channel that is down at that moment would lose them forever (with cooldown 0 there
// is no reminder either). The sweep retries failed deliveries with exponential backoff until they succeed, reach the
// attempt limit or are no longer needed.
//   - recovery: at most syncDeliveryRetryMaxAttempts attempts, then exhausted (terminal).
//   - first alert: the first syncDeliveryRetryMaxAttempts attempts back off 1/2/4/8/16 minutes and then the row is
//     marked exhausted, but while the alert keeps firing it is still retried every syncDeliveryRetryMaxDelay
//     (30 minutes): as long as the alert is active we never give up telling the user (otherwise neither the cooldown
//     reminder nor the recovery could ever go out, since both require the first alert to be delivered).
// Cooldown reminders are not retried here: they have their own delivery record and simply go out again after the next
// cooldown.

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
	// syncDeliveryRetryDisabledChannelTTL 是恢复通知等待停用渠道重新启用的最长时间（从渠道停用时算起）；超过后放弃，
	// 避免迟到太久的"已恢复"。首次告警不受这个限制：告警仍在触发就一直等。
	// syncDeliveryRetryDisabledChannelTTL is how long a recovery waits for a disabled channel to be re-enabled (counted
	// from when the channel was disabled); after that it is given up so a very late "recovered" is not sent. First
	// alerts are not limited by it: they keep waiting while the alert is firing.
	syncDeliveryRetryDisabledChannelTTL = 24 * time.Hour
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

// ListRetryableSyncAlertDeliveries 按 id 升序分页返回需要重试的同步告警投递，afterID 为上一页最后一条：
// 还没用完次数的失败恢复投递，以及所有未送达的首次告警（包括 exhausted，告警仍在触发时会继续低频重试）。
// ListRetryableSyncAlertDeliveries pages (id ASC, after afterID) through sync alert deliveries that need a retry:
// failed recoveries with attempts left, and every undelivered first alert (exhausted included, since it keeps being
// retried at a low rate while the alert is firing).
func (r *Repository) ListRetryableSyncAlertDeliveries(ctx context.Context, maxAttempts int, afterID uint, limit int) ([]*NotificationDelivery, error) {
	var deliveries []*NotificationDelivery
	err := r.db.WithContext(ctx).
		Where("source_type = ? AND id > ?", string(AlertSourceTypeSync), afterID).
		Where(r.db.
			Where("event_type = ? AND status IN ? AND attempt_count < ?", string(NotificationDeliveryEventTypeResolved),
				[]string{string(NotificationDeliveryStatusFailed), string(NotificationDeliveryStatusRetrying)}, maxAttempts).
			Or("event_type = ? AND status IN ?", string(NotificationDeliveryEventTypeFiring),
				[]string{string(NotificationDeliveryStatusFailed), string(NotificationDeliveryStatusRetrying),
					string(NotificationDeliveryStatusExhausted)})).
		Order("id ASC").
		Limit(limit).
		Find(&deliveries).Error
	return deliveries, err
}

// MarkNotificationDeliveryExhausted 把一条投递记录置为 exhausted（退避重试次数已用完），保留最近错误。
// MarkNotificationDeliveryExhausted sets one delivery record to exhausted (backoff retries used up), keeping the last
// error.
func (r *Repository) MarkNotificationDeliveryExhausted(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&NotificationDelivery{}).Where("id = ?", id).
		UpdateColumn("status", string(NotificationDeliveryStatusExhausted)).Error
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
//   - 按 id 分页扫描全部可重试记录（最多 syncDeliveryRetryMaxScanPages 页），按渠道分组后轮转发送：每轮每个渠道发一条，
//     健康渠道排在前面，可疑渠道（最近一次发送失败）排在后面并按最近失败时间从早到晚轮换；同一渠道本轮失败后跳过它
//     剩下的记录。这样排在后面的健康渠道不会因为前面卡住的渠道用完巡检预算而一直轮不到。
//   - 记录到期的条件：退避时间已过，或者渠道在失败之后已经恢复（之前的巡检 / 本轮冷却提醒 / 其他告警发送成功，或被
//     重新保存 / 启用），此时不再等退避（首次告警用完退避后最长要等 30 分钟）。
//   - 每轮最多重发 syncDeliveryRetryBatchSize 条；不再需要重试的记录被标成 canceled，不占发送名额。
//
// retryFailedDeliveries retries failed first alert / recovery deliveries that are due; the returned error is only
// logged and never blocks the rest of the sweep.
//   - It pages by id through every retryable row (at most syncDeliveryRetryMaxScanPages pages), groups them by
//     channel and round-robins: one row per channel per round, healthy channels first, suspect channels (latest send
//     failed) after them rotated by least recent failure; a channel that fails in this sweep is skipped for the rest
//     of it. A healthy channel that sorts late is therefore never starved by hung channels using up the sweep budget.
//   - A row is due when its backoff has elapsed, or when its channel recovered after a failure (a send succeeded in an
//     earlier sweep, in this sweep's reminders or for another alert, or the channel was saved / re-enabled): then the
//     backoff (up to 30 minutes after a first alert's retries are used up) is not waited out.
//   - At most syncDeliveryRetryBatchSize resends per sweep; rows that no longer need a retry are canceled and do not
//     take a send slot.
func (h *SyncAlertHub) retryFailedDeliveries(ctx context.Context) error {
	repo := h.service.repo
	health := &h.service.channelHealth
	now := timeNowUTC()
	guard := alertSweepGuardFrom(ctx)
	var (
		errs    []error
		afterID uint
		queues  = map[uint][]*NotificationDelivery{}
		order   []uint
	)
	for page := 0; page < syncDeliveryRetryMaxScanPages; page++ {
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
			if _, seen := queues[delivery.ChannelID]; !seen {
				order = append(order, delivery.ChannelID)
			}
			queues[delivery.ChannelID] = append(queues[delivery.ChannelID], delivery)
		}
		if len(deliveries) < syncDeliveryRetryBatchSize {
			break
		}
	}
	if len(order) == 0 {
		return errors.Join(errs...)
	}

	// 渠道顺序：健康渠道在前（保持最早记录的顺序），可疑渠道在后、最近失败越早越靠前。
	// Channel order: healthy channels first (keeping their earliest row's order), suspect ones after, least recently
	// failed first.
	type channelOrder struct {
		suspect       bool
		lastFailureAt time.Time
		recovered     bool
	}
	info := make(map[uint]channelOrder, len(order))
	for _, channelID := range order {
		suspect, lastFailureAt, recovered := health.snapshot(channelID)
		info[channelID] = channelOrder{suspect: suspect, lastFailureAt: lastFailureAt, recovered: recovered}
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := info[order[i]], info[order[j]]
		if a.suspect != b.suspect {
			return !a.suspect
		}
		if a.suspect {
			return a.lastFailureAt.Before(b.lastFailureAt)
		}
		return false
	})

	channels, err := repo.ListNotificationChannels(ctx)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	channelMap := make(map[uint]*NotificationChannel, len(channels))
	for _, channel := range channels {
		if channel != nil {
			channelMap[channel.ID] = channel
		}
	}

	due := func(delivery *NotificationDelivery) bool {
		if delivery.UpdatedAt.IsZero() || !now.Before(delivery.UpdatedAt.UTC().Add(syncDeliveryRetryDelay(delivery.AttemptCount))) {
			return true
		}
		// 渠道在失败之后恢复了（重试阶段开始前记录到发送成功或重新保存 / 启用）：不等退避。
		// The channel recovered after a failure (a success or a re-save / re-enable recorded before the retry phase
		// started): do not wait out the backoff.
		return info[delivery.ChannelID].recovered
	}

	attempted := 0
	early := map[uint]bool{}
	for progress := true; progress; {
		progress = false
		for _, channelID := range order {
			if attempted >= syncDeliveryRetryBatchSize || guard.budgetExceeded() {
				return errors.Join(errs...)
			}
			if guard.check(channelID) == alertSweepSkipChannelFailed {
				// 渠道本轮已失败：本轮不再发它，不消耗次数，下一轮再试。
				// The channel already failed in this sweep: skip it for now without using an attempt.
				continue
			}
			queue := queues[channelID]
			pick := -1
			for i, delivery := range queue {
				if due(delivery) {
					pick = i
					break
				}
			}
			if pick < 0 {
				continue
			}
			delivery := queue[pick]
			queues[channelID] = append(queue[:pick:pick], queue[pick+1:]...)
			progress = true
			if !delivery.UpdatedAt.IsZero() && now.Before(delivery.UpdatedAt.UTC().Add(syncDeliveryRetryDelay(delivery.AttemptCount))) {
				early[channelID] = true
			}
			sent, err := h.retrySyncAlertDelivery(ctx, delivery, channelMap)
			if sent {
				attempted++
			}
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	// 提前重试过的渠道清掉"已恢复"标记：之后再失败就重新按退避来。
	// Channels retried early drop their recovered mark: later failures follow the backoff again.
	for channelID := range early {
		health.consumeRecovered(channelID)
	}
	return errors.Join(errs...)
}

// retrySyncAlertDelivery 重发一条失败的首次告警或恢复投递；返回是否真的发了一次。
//   - 首次告警：阶段必须仍在告警中、未被人工关闭（静默期间先跳过，不标终态），策略仍启用且仍匹配这条规则；
//   - 恢复：阶段必须已恢复、之后没有新的告警阶段，策略仍启用且开启恢复通知（首次告警必须已送达，见 RequireFiringSent）；
//   - 两者都要求渠道仍存在且仍绑定在策略上，否则标记 canceled；渠道停用时先跳过、不消耗次数：首次告警一直等到重新
//     启用（告警仍在触发时），恢复通知从渠道停用起最多等 24 小时。
//
// retrySyncAlertDelivery resends one failed first alert or recovery delivery and reports whether it really sent.
//   - first alert: the episode must still be firing and not manually closed (silenced is skipped without a terminal
//     state), and the policy must still be enabled and match the rule;
//   - recovery: the episode must be recovered with no newer firing episode, and the policy must still be enabled with
//     recovery notifications on (the first alert must have been delivered, see RequireFiringSent);
//   - both need the channel to exist and still be bound to the policy, otherwise the row is canceled; a disabled channel
//     is skipped without using an attempt: a first alert waits until it is re-enabled (while the alert is firing), a
//     recovery waits at most 24 hours from when the channel was disabled.
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
	if channel == nil {
		return false, h.cancelDeliveryRetry(ctx, delivery, "notification channel no longer exists")
	}
	if !channel.Enabled {
		// 渠道停用（比如排查期间）：先跳过、不消耗次数，重新启用后照常补发。只有恢复通知在渠道停用超过
		// syncDeliveryRetryDisabledChannelTTL 后放弃（从停用时算起），避免迟到太久的"已恢复"；首次告警在告警仍触发时一直等。
		// A disabled channel (e.g. while debugging) is skipped without using an attempt and catches up once re-enabled.
		// Only a recovery is given up once the channel has been disabled for syncDeliveryRetryDisabledChannelTTL
		// (counted from when it was disabled), so a very late "recovered" is not sent; a first alert keeps waiting
		// while the alert is firing.
		if eventType == NotificationDeliveryEventTypeResolved && timeNowUTC().Sub(channel.disabledSince()) > syncDeliveryRetryDisabledChannelTTL {
			return false, h.cancelDeliveryRetry(ctx, delivery, "notification channel stayed disabled for over 24 hours")
		}
		return false, nil
	}
	if !alertPolicyHasChannel(policy, channel.ID) {
		return false, h.cancelDeliveryRetry(ctx, delivery, "notification channel is no longer bound to the policy")
	}

	var extra []string
	if episode.Scope == SyncAlertEpisodeScopeTask && policy.NotifyTaskOwner {
		extra = h.service.resolveUserEmails(ctx, episode.TaskCreatorID)
	}
	// 本文件唯一的 buildSyncAlertMessage 调用；重试只补发首次告警/恢复（非提醒），签名增加 reminder 参数时这里传 false。
	// The only buildSyncAlertMessage call in this file; retries only cover the first alert/recovery (never a reminder),
	// so if the signature gains a reminder flag pass false here.
	message := buildSyncAlertMessage(episode, policy, eventType, extra, h.clock())
	result, sendErr := h.service.dispatchAlertDeliveryWithOptions(ctx, message, policy, channel, eventType, delivery.SourceKey,
		alertDeliveryOptions{RequireFiringSent: eventType == NotificationDeliveryEventTypeResolved, Retry: true})
	if sendErr != nil {
		lastError := strings.TrimSpace(sendErr.Error())
		if delivery.AttemptCount+1 >= syncDeliveryRetryMaxAttempts {
			if eventType == NotificationDeliveryEventTypeFiring {
				log.Printf("[Monitoring] sync alert first notification used up its backoff retries after %d attempts, retrying every %s while firing: source=%s channel=%d err=%v / 同步告警首次通知退避重试已用完，告警持续期间每 %s 重试",
					delivery.AttemptCount+1, syncDeliveryRetryMaxDelay, delivery.SourceKey, delivery.ChannelID, sendErr, syncDeliveryRetryMaxDelay)
			} else {
				log.Printf("[Monitoring] sync alert %s notification gave up after %d attempts: source=%s channel=%d err=%v / 同步告警通知重试次数已用完",
					eventType, delivery.AttemptCount+1, delivery.SourceKey, delivery.ChannelID, sendErr)
			}
			lastError = "retries exhausted / 重试已用完: " + lastError
			if err := repo.MarkNotificationDeliveryExhausted(ctx, delivery.ID); err != nil {
				return true, errors.Join(sendErr, err)
			}
		}
		// 重试失败也写回策略的最近执行状态，不让它停在"命中"；其他渠道已送达时是 partial，不降级成 failed。
		// A failed retry is written to the policy's last execution status too, so it never stays at "matched"; when
		// other channels did deliver it is partial, not downgraded to failed.
		if err := repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, &AlertPolicyExecutionStateUpdate{
			LastExecutionStatus: h.retryPolicyStatus(ctx, policy, delivery, channelMap),
			LastExecutionError:  lastError,
		}); err != nil {
			return true, errors.Join(sendErr, err)
		}
		return true, sendErr
	}
	if result != nil && (result.Deferred || (result.Failed && result.LastError == alertSweepChannelFailedError)) {
		// 巡检预算守卫跳过了这次发送：留到下一轮，不算尝试。
		// The sweep guard skipped this send: leave it for the next sweep, not an attempt.
		return false, nil
	}
	if result == nil {
		// 投递前置条件不满足（比如恢复但首次告警未送达）：不会再满足，标记终态。
		// A delivery precondition failed (e.g. recovery while the first alert was never delivered): it will not
		// become true later, so mark the row terminal.
		return false, h.cancelDeliveryRetry(ctx, delivery, "delivery preconditions no longer hold")
	}
	if result.DeliveryCountDelta > 0 {
		status := h.retryPolicyStatus(ctx, policy, delivery, channelMap)
		update := &AlertPolicyExecutionStateUpdate{
			DeliveryCountDelta:  result.DeliveryCountDelta,
			LastDeliveredAt:     result.LastDeliveredAt,
			LastExecutionStatus: status,
		}
		if status == AlertPolicyExecutionStatusPartial {
			// 还有渠道没送达：保留原来的错误说明。
			// Some channel is still undelivered: keep the existing error text.
			if current, err := repo.GetAlertPolicyByID(ctx, policy.ID); err == nil && current != nil {
				update.LastExecutionError = current.LastExecutionError
			}
		}
		return true, repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, update)
	}
	return true, nil
}

// retryPolicyStatus 在一次重试之后按策略的全部启用渠道汇总同一告警、同一事件的投递结果：全部送达为 sent，部分送达为
// partial，都没送达为 failed。重试只补发失败的那个渠道，不会把其他渠道已经成功的 partial 降级成 failed。
// retryPolicyStatus summarizes, after one retry, the same alert and event across every enabled channel of the policy:
// all delivered is sent, some is partial, none is failed. A retry only resends the failed channel, so it never
// downgrades a partial (other channels delivered) to failed.
func (h *SyncAlertHub) retryPolicyStatus(ctx context.Context, policy *AlertPolicy, delivery *NotificationDelivery, channelMap map[uint]*NotificationChannel) AlertPolicyExecutionStatus {
	delivered, undelivered := 0, 0
	for _, channelID := range unmarshalAlertPolicyChannelIDs(policy.NotificationChannelIDsJSON) {
		channel := channelMap[channelID]
		if channel == nil || !channel.Enabled {
			continue
		}
		row, err := h.service.repo.GetNotificationDeliveryByDedupKey(ctx, delivery.SourceKey, channelID, delivery.EventType)
		if err != nil || row == nil {
			continue
		}
		if NotificationDeliveryStatus(strings.TrimSpace(row.Status)) == NotificationDeliveryStatusSent {
			delivered++
		} else {
			undelivered++
		}
	}
	switch {
	case delivered > 0 && undelivered > 0:
		return AlertPolicyExecutionStatusPartial
	case delivered > 0:
		return AlertPolicyExecutionStatusSent
	default:
		return AlertPolicyExecutionStatusFailed
	}
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
