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
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// refreshTask 处理 B4 的 Refresh：只更新告警中阶段的展示内容，不计次、不通知；没有阶段（或已被人工关闭）时忽略。
// JobID > 0 时只更新最近作业为该作业的阶段，避免别的作业的旧值覆盖。
// refreshTask handles a B4 Refresh: it only refreshes the firing episode's display content without counting or
// notifying, and is ignored without an episode (or when it was manually closed). With JobID > 0 only an episode whose
// last job is that job is touched, so another job's stale values never overwrite it.
func (h *SyncAlertHub) refreshTask(ctx context.Context, signal syncalert.Signal) error {
	if signal.TaskID == 0 || signal.RuleKey != syncalert.RuleCDCDelayHigh {
		return nil
	}
	episodes, err := h.service.repo.ListFiringSyncAlertEpisodesByTask(ctx, signal.TaskID, []string{syncalert.RuleCDCDelayHigh})
	if err != nil {
		return err
	}
	var errs []error
	for _, episode := range episodes {
		if signal.JobID > 0 && episode.LastJobID != signal.JobID {
			continue
		}
		closed, err := h.episodeClosed(ctx, episode)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if closed {
			continue
		}
		applySignalToEpisode(episode, signal)
		errs = append(errs, h.service.repo.SaveSyncAlertEpisode(ctx, episode))
	}
	return errors.Join(errs...)
}

// resolveCDCDelay 关闭任务的 B4 阶段：原因为空或 recovered 时按策略发恢复通知，其余原因静默关闭。
// resolveCDCDelay closes the task's B4 episode: an empty or recovered reason sends recovery per policy, other reasons
// close silently.
func (h *SyncAlertHub) resolveCDCDelay(ctx context.Context, signal syncalert.Signal) error {
	if signal.TaskID == 0 {
		return nil
	}
	repo := h.service.repo
	episodes, err := repo.ListFiringSyncAlertEpisodesByTask(ctx, signal.TaskID, []string{syncalert.RuleCDCDelayHigh})
	if err != nil || len(episodes) == 0 {
		return err
	}
	reason := strings.TrimSpace(signal.ResolveReason)
	if reason == "" {
		reason = syncalert.ResolveReasonRecovered
	}
	var policies []*AlertPolicy
	if reason == syncalert.ResolveReasonRecovered {
		policy, err := repo.GetSyncJobAlertPolicyByTaskID(ctx, signal.TaskID)
		if err != nil {
			return err
		}
		policies = singlePolicy(policy)
	}
	var errs []error
	for _, episode := range episodes {
		if signal.JobID > 0 && episode.LastJobID != signal.JobID {
			continue
		}
		if signal.CDCDelay != nil {
			setEpisodeCDCDelayDetail(episode, signal.CDCDelay)
		}
		if message := normalizeSyncAlertText(signal.Message, syncalert.MaxMessageLength); message != "" {
			episode.LastMessage = message
		}
		errs = append(errs, h.resolveEpisode(ctx, episode, signal.OccurredAt, reason, policies))
	}
	return errors.Join(errs...)
}

// episodeCDCDelayDetail 解析阶段上的 B4 明细；不是 B4 或没有明细时返回 nil。
// episodeCDCDelayDetail parses the episode's B4 detail; nil for other rules or without a detail.
func episodeCDCDelayDetail(episode *SyncAlertEpisode) *syncalert.CDCDelayDetail {
	if episode == nil || episode.RuleKey != syncalert.RuleCDCDelayHigh || strings.TrimSpace(episode.LastDetail) == "" {
		return nil
	}
	var detail syncalert.CDCDelayDetail
	if err := json.Unmarshal([]byte(episode.LastDetail), &detail); err != nil {
		return nil
	}
	return &detail
}

func setEpisodeCDCDelayDetail(episode *SyncAlertEpisode, detail *syncalert.CDCDelayDetail) {
	if episode == nil || detail == nil {
		return
	}
	if encoded, err := json.Marshal(detail); err == nil {
		episode.LastDetail = string(encoded)
	}
}

// markEpisodeCDCDelayClosed 在 B4 阶段因恢复以外的原因关闭时，把明细状态改成 closed 并记下原因，避免关闭后
// last_detail.state 还停在 high / recovering / unknown；恢复关闭保留 sync 送来的 recovered 明细。没有明细时不处理。
// markEpisodeCDCDelayClosed sets the detail state to closed with the reason when a B4 episode closes for a reason
// other than recovery, so last_detail.state no longer stays at high / recovering / unknown after the close; a
// recovery keeps the recovered detail sent by sync. Episodes without a detail are left alone.
func markEpisodeCDCDelayClosed(episode *SyncAlertEpisode, reason string) {
	if reason == "" || reason == syncalert.ResolveReasonRecovered {
		return
	}
	detail := episodeCDCDelayDetail(episode)
	if detail == nil {
		return
	}
	detail.State = syncalert.CDCDelayStateClosed
	detail.ResolveReason = reason
	setEpisodeCDCDelayDetail(episode, detail)
}

// syncAlertRemindersPaused 判断阶段是否因 B4 "无法确认"超过 24 小时而停止提醒。
// syncAlertRemindersPaused reports whether reminders stopped because B4 was "unconfirmable" for over 24 hours.
func syncAlertRemindersPaused(episode *SyncAlertEpisode) bool {
	if episode == nil || episode.Status != SyncAlertEpisodeFiring {
		return false
	}
	detail := episodeCDCDelayDetail(episode)
	return detail != nil && detail.RemindersPaused
}

// syncAlertAutoCloseNote 返回自动关闭原因的中文说明（告警中心摘要用）；恢复和人工关闭返回空。前端按 resolve_reason
// 另有中英文文案。
// syncAlertAutoCloseNote returns the Chinese note for an automatic close (alert center summary); recovery and manual
// close return empty. The frontend has its own zh/en copy keyed by resolve_reason.
func syncAlertAutoCloseNote(reason string) string {
	switch reason {
	case syncalert.ResolveReasonJobEnded:
		return "作业已结束，自动关闭"
	case syncalert.ResolveReasonChannelOff:
		return "事件通道不可用，自动关闭"
	case syncalert.ResolveReasonRuleRemoved:
		return "已取消该规则，自动关闭"
	case syncalert.ResolveReasonRuleChanged:
		return "规则参数已修改，按新参数未超标，自动关闭"
	case syncAlertResolvePolicyDeleted:
		return "告警配置已删除，自动关闭"
	default:
		return ""
	}
}

// cdcDelayElapsedMinutes 返回延迟超标已持续的分钟数：从首次超过阈值起算，没有时从阶段首次告警起算。
// cdcDelayElapsedMinutes returns how many minutes the delay has been above the threshold: from when it first crossed,
// or from the episode's first alert when unknown.
func cdcDelayElapsedMinutes(episode *SyncAlertEpisode, detail *syncalert.CDCDelayDetail, now time.Time) int {
	since := episode.FirstFiredAt
	if detail != nil && !detail.AboveSince.IsZero() {
		since = detail.AboveSince
	}
	if since.IsZero() || now.Before(since) {
		return 0
	}
	return int(now.Sub(since) / time.Minute)
}

// cdcDelayTitleSuffix 返回 B4 标题后缀：告警时"（3分12秒）"，无法确认时"（无法确认是否恢复）"，根据读取条数恢复时
// "（根据读取条数推断）"；提醒再加"[提醒] 已持续 X 分钟"。
// cdcDelayTitleSuffix returns the B4 title suffix: "（3分12秒）" when alerting, "（无法确认是否恢复）" when
// unconfirmable, "（根据读取条数推断）" for a recovery inferred from read counts; reminders add "[提醒] 已持续 X 分钟".
func cdcDelayTitleSuffix(episode *SyncAlertEpisode, eventType NotificationDeliveryEventType, reminder bool, now time.Time) string {
	detail := episodeCDCDelayDetail(episode)
	suffix := ""
	switch {
	case detail == nil:
	case eventType == NotificationDeliveryEventTypeResolved:
		if detail.RecoveryBasis == syncalert.CDCDelayRecoveryByReadCount {
			suffix = "（根据读取条数推断）"
		}
	case detail.State == syncalert.CDCDelayStateUnknown:
		suffix = "（无法确认是否恢复）"
	case detail.State == syncalert.CDCDelayStateRecovering:
		suffix = "（已回落，待确认）"
	case detail.CurrentDelayMs > 0:
		suffix = "（" + syncalert.FormatDelayZH(detail.CurrentDelayMs) + "）"
	}
	if reminder && eventType != NotificationDeliveryEventTypeResolved {
		suffix += fmt.Sprintf(" [提醒] 已持续 %d 分钟", cdcDelayElapsedMinutes(episode, detail, now))
	}
	return suffix
}

// cdcDelayFields 返回 B4 通知的专用字段，取代"错误摘要"。
// cdcDelayFields returns the B4-specific notification fields that replace the error summary.
func cdcDelayFields(episode *SyncAlertEpisode, eventType NotificationDeliveryEventType, now time.Time) []localAlertMessageField {
	detail := episodeCDCDelayDetail(episode)
	if detail == nil {
		// 没有明细（例如旧版本留下的阶段）：只展示说明。
		// No detail (e.g. an episode left by an older version): only show the note.
		return []localAlertMessageField{{Label: "说明", Value: syncalert.TruncateMessage(episode.LastMessage, syncAlertMessageDisplayLimit), Multiline: true}}
	}
	var fields []localAlertMessageField
	add := func(label, value string, multiline bool) {
		if strings.TrimSpace(value) != "" {
			fields = append(fields, localAlertMessageField{Label: label, Value: value, Multiline: multiline})
		}
	}
	resolved := eventType == NotificationDeliveryEventTypeResolved
	switch {
	case resolved:
		if detail.RecoveryBasis == syncalert.CDCDelayRecoveryByReadCount {
			add("恢复依据", "根据读取条数推断：最近没有延迟事件，读取条数在增长，读取延迟应已低于 1 秒", false)
		} else {
			add("恢复依据", "读取延迟连续两轮低于阈值", false)
			if detail.CurrentDelayMs > 0 {
				add("当前读取延迟", syncalert.FormatDelayZH(detail.CurrentDelayMs), false)
			}
		}
	case detail.State == syncalert.CDCDelayStateUnknown:
		if detail.CurrentDelayMs > 0 {
			add("最后一次读取延迟", syncalert.FormatDelayZH(detail.CurrentDelayMs), false)
		}
	case detail.State == syncalert.CDCDelayStateRecovering:
		if detail.DelayInferred {
			add("当前读取延迟", "根据读取条数推断应已低于 1 秒", false)
		} else {
			add("当前读取延迟", syncalert.FormatDelayZH(detail.CurrentDelayMs), false)
		}
		add("说明", "已低于阈值，再确认一轮后自动关闭", false)
	default:
		add("当前读取延迟", syncalert.FormatDelayZH(detail.CurrentDelayMs), false)
		if detail.WindowMaxDelayMs > 0 {
			add(fmt.Sprintf("近 %d 分钟最大读取延迟", detail.DurationMinutes), syncalert.FormatDelayZH(detail.WindowMaxDelayMs), false)
		}
	}
	add("告警条件", fmt.Sprintf("读取延迟超过 %s 并持续 %d 分钟", syncalert.FormatDelayZH(int64(detail.ThresholdSeconds)*1000), detail.DurationMinutes), false)
	if !resolved {
		add("已持续", fmt.Sprintf("%d 分钟", cdcDelayElapsedMinutes(episode, detail, now)), false)
	}
	if !resolved && detail.State == syncalert.CDCDelayStateUnknown {
		add("说明", syncalert.TruncateMessage(episode.LastMessage, syncAlertMessageDisplayLimit), true)
	}
	return fields
}

// appendCDCDelayWebhookFields 把 B4 的数值明细平铺进 webhook 的 alert 对象，方便下游直接取 current_delay_ms 等字段。
// appendCDCDelayWebhookFields flattens the B4 numbers into the webhook alert object so receivers can read
// current_delay_ms and friends directly.
func appendCDCDelayWebhookFields(alert map[string]interface{}, episode *SyncAlertEpisode) {
	detail := episodeCDCDelayDetail(episode)
	if detail == nil {
		return
	}
	alert["delay_state"] = detail.State
	// 根据读取条数推断的状态没有当前延迟：current_delay_ms 为 null（不是最后一个旧样本），delay_inferred=true。
	// A state inferred from the read count has no current delay: current_delay_ms is null (not the last old sample)
	// and delay_inferred=true.
	if detail.DelayInferred || (detail.State == syncalert.CDCDelayStateRecovered && detail.RecoveryBasis == syncalert.CDCDelayRecoveryByReadCount) {
		alert["current_delay_ms"] = nil
		alert["delay_inferred"] = true
	} else {
		alert["current_delay_ms"] = detail.CurrentDelayMs
	}
	alert["window_max_delay_ms"] = detail.WindowMaxDelayMs
	alert["threshold_seconds"] = detail.ThresholdSeconds
	alert["duration_minutes"] = detail.DurationMinutes
	if !detail.AboveSince.IsZero() {
		alert["above_since"] = detail.AboveSince.UTC().Format(time.RFC3339)
	}
	if !detail.LastSampleAt.IsZero() {
		alert["last_sample_at"] = detail.LastSampleAt.UTC().Format(time.RFC3339)
	}
	if detail.UnknownReason != "" {
		alert["unknown_reason"] = detail.UnknownReason
	}
	if !detail.UnknownSince.IsZero() {
		alert["unknown_since"] = detail.UnknownSince.UTC().Format(time.RFC3339)
	}
	if detail.RemindersPaused {
		alert["reminders_paused"] = true
	}
	if detail.RecoveryBasis != "" {
		alert["recovery_basis"] = detail.RecoveryBasis
	}
}
