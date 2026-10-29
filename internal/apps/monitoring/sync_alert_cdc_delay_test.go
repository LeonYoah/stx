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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

func (f *syncAlertFixture) configureCDC(t *testing.T, taskID uint, rules []string, cooldown, threshold, duration int) *syncalert.TaskConfig {
	t.Helper()
	cfg, err := f.service.SaveTaskAlertConfig(f.ctx, syncalert.TaskRef{ID: taskID, Name: "orders-sync", ClusterID: f.clusterID, CreatorID: 1}, &syncalert.TaskConfig{
		Enabled: true, Rules: rules, ChannelIDs: []uint{f.channel.ID}, CooldownMinutes: cooldown, SendRecovery: true,
		CDCDelayThresholdSeconds: threshold, CDCDelayDurationMinutes: duration,
	})
	if err != nil {
		t.Fatalf("save task alert config: %v", err)
	}
	return cfg
}

func (f *syncAlertFixture) cdcSignal(kind syncalert.SignalKind, taskID, jobID uint, detail syncalert.CDCDelayDetail, message string) syncalert.Signal {
	signal := f.signal(kind, syncalert.RuleCDCDelayHigh, taskID, jobID)
	signal.Message = message
	signal.CDCDelay = &detail
	if kind == syncalert.KindResolve {
		signal.ResolveScope = syncalert.ResolveCDCDelay
	}
	return signal
}

func highDetail(currentMs int64, above time.Time) syncalert.CDCDelayDetail {
	return syncalert.CDCDelayDetail{State: syncalert.CDCDelayStateHigh, CurrentDelayMs: currentMs, WindowMaxDelayMs: currentMs + 28_000, ThresholdSeconds: 60, DurationMinutes: 5, AboveSince: above, LastSampleAt: above.Add(6 * time.Minute)}
}

// 配置存取：B4 阈值和持续时间写进条件并读回；0 = 默认；超出范围 400；直接调策略接口也校验。
// Config round trip: the B4 threshold and duration live in the condition and read back; 0 = default; out of range is
// 400; direct policy API calls are checked too.
func TestSyncAlertCDCDelay_configRoundTripAndValidation(t *testing.T) {
	f := setupSyncAlertFixture(t)
	saved := f.configureCDC(t, 7, []string{syncalert.RuleJobFailed, syncalert.RuleCDCDelayHigh}, 60, 120, 10)
	if saved.CDCDelayThresholdSeconds != 120 || saved.CDCDelayDurationMinutes != 10 || !saved.HasRule(syncalert.RuleCDCDelayHigh) {
		t.Fatalf("B4 params not stored: %+v", saved)
	}
	got, err := f.service.GetTaskAlertConfig(f.ctx, 7)
	if err != nil || got.CDCDelayThresholdSeconds != 120 || got.CDCDelayDurationMinutes != 10 {
		t.Fatalf("B4 params not read back: %+v err=%v", got, err)
	}
	defaults := f.configureCDC(t, 8, []string{syncalert.RuleJobFailed}, 60, 0, 0)
	if defaults.CDCDelayThresholdSeconds != 60 || defaults.CDCDelayDurationMinutes != 5 {
		t.Fatalf("0 / omitted should mean the defaults: %+v", defaults)
	}
	for _, bad := range [][2]int{{9, 5}, {86401, 5}, {60, 61}} {
		_, err := f.service.SaveTaskAlertConfig(f.ctx, syncalert.TaskRef{ID: 9, Name: "x", ClusterID: f.clusterID}, &syncalert.TaskConfig{
			Enabled: true, Rules: []string{syncalert.RuleCDCDelayHigh}, ChannelIDs: []uint{f.channel.ID}, CDCDelayThresholdSeconds: bad[0], CDCDelayDurationMinutes: bad[1],
		})
		if !errors.Is(err, syncalert.ErrInvalidConfig) {
			t.Fatalf("%v should be a 400, got %v", bad, err)
		}
	}
	req := taskConfigToPolicyRequest(syncalert.TaskRef{ID: 10, Name: "x"}, (&syncalert.TaskConfig{Rules: []string{syncalert.RuleCDCDelayHigh}, ChannelIDs: []uint{f.channel.ID}}).Normalize())
	req.Conditions[0].Threshold = "5"
	if err := req.Validate(); !errors.Is(err, ErrAlertPolicySyncJobInvalidCDCDelay) || getAlertPolicyStatusCode(err) != 400 {
		t.Fatalf("policy API must reject a bad B4 threshold with 400, got %v", err)
	}
	req.Conditions[0].Threshold = "abc"
	if err := req.Validate(); !errors.Is(err, ErrAlertPolicySyncJobInvalidCDCDelay) {
		t.Fatalf("non-integer threshold must be rejected, got %v", err)
	}
	req.Conditions[0].Threshold = ""
	req.Conditions[0].WindowMinutes = 0
	if err := req.Validate(); err != nil {
		t.Fatalf("empty threshold / 0 window mean defaults: %v", err)
	}
}

// 通知模板：标题带当前延迟；延迟不在"错误摘要"；webhook 带 current_delay_ms、threshold_seconds 等；Refresh 不计次不通知；
// 提醒标题带"[提醒] 已持续 X 分钟"，内容用最新一次 Refresh。
// Notification template: the title carries the current delay; the delay is not in the error summary; the webhook
// carries current_delay_ms, threshold_seconds and friends; Refresh neither counts nor notifies; reminder titles add
// "[提醒] 已持续 X 分钟" and use the latest Refresh.
func TestSyncAlertCDCDelay_templateRefreshAndReminder(t *testing.T) {
	withConsoleURL(t, "https://stx.example.com")
	f := setupSyncAlertFixture(t)
	f.configureCDC(t, 7, []string{syncalert.RuleCDCDelayHigh}, 1, 60, 5)
	above := f.now.Add(-6 * time.Minute)

	// 没有阶段时 Refresh 忽略。 / Refresh without an episode is ignored.
	f.handle(t, f.cdcSignal(syncalert.KindRefresh, 7, 70, highDetail(192_000, above), "x"))
	if len(f.episodes(t)) != 0 {
		t.Fatalf("refresh must not open an episode")
	}

	f.handle(t, f.cdcSignal(syncalert.KindFire, 7, 70, highDetail(192_000, above), "CDC 读取延迟 3分12秒"))
	if f.webhook.count() != 1 {
		t.Fatalf("fire should notify once, got %d", f.webhook.count())
	}
	body := f.webhook.last()
	title, _ := body["title"].(string)
	if !strings.Contains(title, "CDC 延迟过高（3分12秒）") || strings.Contains(title, "[提醒]") {
		t.Fatalf("unexpected title: %q", title)
	}
	text, _ := body["message"].(string)
	if strings.Contains(text, "错误摘要") || !strings.Contains(text, "当前读取延迟：3分12秒") || !strings.Contains(text, "告警条件：读取延迟超过 1分 并持续 5 分钟") {
		t.Fatalf("B4 body must use delay fields, not the error summary:\n%s", text)
	}
	alert, _ := body["alert"].(map[string]any)
	if alert["current_delay_ms"] != float64(192_000) || alert["threshold_seconds"] != float64(60) || alert["duration_minutes"] != float64(5) || alert["window_max_delay_ms"] != float64(220_000) || alert["delay_state"] != "high" {
		t.Fatalf("webhook must carry structured delay fields: %#v", alert)
	}

	// Refresh：不计次、不通知；另一个作业的 Refresh 不覆盖。
	// Refresh neither counts nor notifies; another job's Refresh does not overwrite.
	f.now = f.now.Add(30 * time.Second)
	f.handle(t, f.cdcSignal(syncalert.KindRefresh, 7, 70, highDetail(250_000, above), "CDC 读取延迟 4分10秒"))
	f.handle(t, f.cdcSignal(syncalert.KindRefresh, 7, 99, highDetail(1_000, above), "other job"))
	episode := f.episodes(t)[0]
	if episode.OccurrenceCount != 1 || f.webhook.count() != 1 || episode.LastMessage != "CDC 读取延迟 4分10秒" || episodeCDCDelayDetail(episode).CurrentDelayMs != 250_000 {
		t.Fatalf("refresh should update content only: %+v webhook=%d", episode, f.webhook.count())
	}

	backdateDelivery(t, f, buildSyncAlertSourceKey(episode.ID), 2*time.Minute)
	if err := f.hub.sweep(f.ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if f.webhook.count() != 2 {
		t.Fatalf("expected a reminder, got %d", f.webhook.count())
	}
	reminder := f.webhook.last()
	if title := reminder["title"].(string); !strings.Contains(title, "CDC 延迟过高（4分10秒） [提醒] 已持续 6 分钟") {
		t.Fatalf("reminder title should carry the latest delay and elapsed minutes: %q", title)
	}
	if ra, _ := reminder["alert"].(map[string]any); ra["reminder"] != true || ra["current_delay_ms"] != float64(250_000) {
		t.Fatalf("reminder webhook should be flagged and current: %#v", ra)
	}
}

// 无法确认超过 24 小时：停止提醒、阶段保留、摘要注明；之后读取条数推断恢复，恢复通知注明"根据读取条数推断"。
// Unconfirmable for over 24 hours: reminders stop, the episode stays and the summary says so; a later recovery
// inferred from read counts says "根据读取条数推断".
func TestSyncAlertCDCDelay_pausedRemindersAndReadCountRecovery(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.configureCDC(t, 7, []string{syncalert.RuleCDCDelayHigh}, 1, 60, 5)
	above := f.now.Add(-6 * time.Minute)
	f.handle(t, f.cdcSignal(syncalert.KindFire, 7, 70, highDetail(192_000, above), "high"))
	paused := highDetail(192_000, above)
	paused.State = syncalert.CDCDelayStateUnknown
	paused.UnknownReason = syncalert.CDCDelayUnknownNoProgress
	paused.UnknownSince = f.now.Add(-25 * time.Hour)
	paused.RemindersPaused = true
	f.handle(t, f.cdcSignal(syncalert.KindRefresh, 7, 70, paused, "无法确认……已停止提醒"))
	episode := f.episodes(t)[0]
	backdateDelivery(t, f, buildSyncAlertSourceKey(episode.ID), 2*time.Minute)
	if err := f.hub.sweep(f.ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if f.webhook.count() != 1 {
		t.Fatalf("paused reminders must not be sent, got %d", f.webhook.count())
	}
	list, err := f.service.ListAlertInstances(f.ctx, &AlertInstanceFilter{SourceType: AlertSourceTypeSync})
	if err != nil || len(list.Alerts) != 1 || !strings.Contains(list.Alerts[0].Summary, "已停止提醒") || !list.Alerts[0].SourceRef.RemindersPaused || list.Alerts[0].LifecycleStatus != AlertLifecycleStatusFiring {
		t.Fatalf("alert center should keep the episode and note the pause: %+v err=%v", list, err)
	}

	recovered := syncalert.CDCDelayDetail{State: syncalert.CDCDelayStateRecovered, ThresholdSeconds: 60, DurationMinutes: 5, RecoveryBasis: syncalert.CDCDelayRecoveryByReadCount}
	signal := f.cdcSignal(syncalert.KindResolve, 7, 70, recovered, "根据读取条数推断已恢复")
	signal.ResolveReason = syncalert.ResolveReasonRecovered
	f.handle(t, signal)
	if f.webhook.count() != 2 {
		t.Fatalf("recovery should notify, got %d", f.webhook.count())
	}
	body := f.webhook.last()
	if title := body["title"].(string); !strings.Contains(title, "[恢复]") || !strings.Contains(title, "根据读取条数推断") {
		t.Fatalf("recovery title should say inferred from read counts: %q", title)
	}
	if text := body["message"].(string); !strings.Contains(text, "恢复依据：根据读取条数推断") {
		t.Fatalf("recovery body should give the basis:\n%s", text)
	}
	if alert, _ := body["alert"].(map[string]any); alert["recovery_basis"] != "read_count" || alert["resolve_reason"] != "recovered" {
		t.Fatalf("recovery webhook should carry the basis: %#v", alert)
	}
}

// 作业结束、通道关闭、规则去掉都是静默关闭：不发通知，告警中心显示原因；JobID 不匹配时不关闭。
// Job end, channel off and rule removal close silently: no notification, the alert center shows the reason; a
// mismatched JobID does not close.
func TestSyncAlertCDCDelay_silentClosesShowReason(t *testing.T) {
	f := setupSyncAlertFixture(t)
	cases := []struct {
		reason string
		note   string
	}{
		{syncalert.ResolveReasonJobEnded, "作业已结束"},
		{syncalert.ResolveReasonChannelOff, "事件通道不可用"},
		{syncalert.ResolveReasonRuleChanged, "规则参数已修改"},
	}
	for i, tc := range cases {
		taskID := uint(20 + i)
		f.configureCDC(t, taskID, []string{syncalert.RuleCDCDelayHigh}, 60, 60, 5)
		f.handle(t, f.cdcSignal(syncalert.KindFire, taskID, 70, highDetail(120_000, f.now.Add(-6*time.Minute)), "high"))
		sent := f.webhook.count()
		mismatch := f.cdcSignal(syncalert.KindResolve, taskID, 71, syncalert.CDCDelayDetail{}, "")
		mismatch.CDCDelay = nil
		mismatch.ResolveReason = tc.reason
		f.handle(t, mismatch)
		closeSignal := mismatch
		closeSignal.JobID = 70
		f.handle(t, closeSignal)
		if f.webhook.count() != sent {
			t.Fatalf("%s must not notify", tc.reason)
		}
		list, err := f.service.ListAlertInstances(f.ctx, &AlertInstanceFilter{SourceType: AlertSourceTypeSync})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		found := false
		for _, item := range list.Alerts {
			if item.SourceRef != nil && item.SourceRef.TaskID == taskID {
				found = true
				if item.LifecycleStatus != AlertLifecycleStatusResolved || item.SourceRef.ResolveReason != tc.reason || !strings.Contains(item.Summary, tc.note) {
					t.Fatalf("%s: alert center should show the reason: %+v %+v", tc.reason, item, item.SourceRef)
				}
			}
		}
		if !found {
			t.Fatalf("%s: episode missing", tc.reason)
		}
	}

	// 去掉 B4：保存配置时 monitoring 直接关闭阶段（rule_removed），其他规则的阶段不动。
	// Removing B4: saving the config closes the B4 episode (rule_removed) and leaves other rules alone.
	f.configureCDC(t, 30, []string{syncalert.RuleJobFailed, syncalert.RuleCDCDelayHigh}, 60, 60, 5)
	f.handle(t, f.cdcSignal(syncalert.KindFire, 30, 70, highDetail(120_000, f.now), "high"))
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 30, 70))
	sent := f.webhook.count()
	f.configureCDC(t, 30, []string{syncalert.RuleJobFailed}, 60, 60, 5)
	if f.webhook.count() != sent {
		t.Fatalf("rule removal must not notify")
	}
	for _, episode := range f.episodes(t) {
		if episode.TaskID != 30 {
			continue
		}
		switch episode.RuleKey {
		case syncalert.RuleCDCDelayHigh:
			if episode.Status != SyncAlertEpisodeResolved || episode.ResolveReason != syncalert.ResolveReasonRuleRemoved {
				t.Fatalf("B4 episode should be closed as rule_removed: %+v", episode)
			}
		case syncalert.RuleJobFailed:
			if episode.Status != SyncAlertEpisodeFiring {
				t.Fatalf("other rules must stay firing: %+v", episode)
			}
		}
	}
}
