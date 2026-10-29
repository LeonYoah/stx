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
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// 根据读取条数推断的恢复：webhook 的 current_delay_ms 为 null（不是旧值 192000），并带 delay_inferred=true；
// 旧版本留下的明细（没有 delay_inferred 但依据为 read_count）同样输出 null。
// A recovery inferred from read counts sends current_delay_ms as null (not the old 192000) with delay_inferred=true;
// a detail left by an older version (no delay_inferred but basis read_count) is sent as null too.
func TestSyncAlertCDCDelay_inferredRecoveryClearsWebhookDelay(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		f := setupSyncAlertFixture(t)
		f.configureCDC(t, 7, []string{syncalert.RuleCDCDelayHigh}, 60, 60, 5)
		f.handle(t, f.cdcSignal(syncalert.KindFire, 7, 70, highDetail(192_000, f.now.Add(-6*time.Minute)), "high"))
		recovered := syncalert.CDCDelayDetail{State: syncalert.CDCDelayStateRecovered, ThresholdSeconds: 60, DurationMinutes: 5, RecoveryBasis: syncalert.CDCDelayRecoveryByReadCount, DelayInferred: true}
		if legacy {
			recovered.DelayInferred = false
			recovered.CurrentDelayMs = 192_000
		}
		signal := f.cdcSignal(syncalert.KindResolve, 7, 70, recovered, "根据读取条数推断已恢复")
		signal.ResolveReason = syncalert.ResolveReasonRecovered
		f.handle(t, signal)
		alert, _ := f.webhook.last()["alert"].(map[string]any)
		value, present := alert["current_delay_ms"]
		if !present || value != nil || alert["delay_inferred"] != true {
			t.Fatalf("legacy=%v: inferred recovery must send current_delay_ms=null and delay_inferred=true: %#v", legacy, alert)
		}
	}
}

// 队列过半时丢弃 B4 Refresh，剩下的容量留给 Fire / Resolve（例如 B1），大量高延迟作业不会把它们挤掉。
// Past half capacity B4 Refreshes are dropped and the rest stays for Fire / Resolve (e.g. B1), so many high-delay
// jobs never crowd them out.
func TestSyncAlertCDCDelay_refreshDroppedWhenQueueHalfFull(t *testing.T) {
	f := setupSyncAlertFixture(t)
	hub := f.service.NewSyncAlertHub() // 不启动，信号留在队列里。 / Not started; signals stay queued.
	refresh := f.cdcSignal(syncalert.KindRefresh, 7, 70, highDetail(120_000, f.now), "refresh")
	for i := 0; i < syncAlertRefreshQueueLimit; i++ {
		hub.ReportSyncAlert(refresh)
	}
	if len(hub.queue) != syncAlertRefreshQueueLimit {
		t.Fatalf("refreshes below the limit are queued, got %d", len(hub.queue))
	}
	hub.ReportSyncAlert(refresh)
	if len(hub.queue) != syncAlertRefreshQueueLimit || hub.droppedRefresh.Load() != 1 {
		t.Fatalf("a refresh past half capacity must be dropped (queue=%d dropped=%d)", len(hub.queue), hub.droppedRefresh.Load())
	}
	hub.ReportSyncAlert(f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 8, 80))
	if len(hub.queue) != syncAlertRefreshQueueLimit+1 || hub.dropped.Load() != 0 {
		t.Fatalf("B1 fire must still be queued (queue=%d)", len(hub.queue))
	}
}

// 停用告警的那一刻不发旧提醒：巡检按 ID 重读阶段，已被停用关闭的不再提醒；停用与提醒互斥。
// No stale reminder at the moment the alert is disabled: the sweep re-reads each episode by ID and skips one closed
// by the disable; disabling and reminding are mutually exclusive.
func TestSyncAlertCDCDelay_noReminderWhenDisabled(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.configureCDC(t, 7, []string{syncalert.RuleCDCDelayHigh}, 1, 60, 5)
	unknown := highDetail(192_000, f.now.Add(-6*time.Minute))
	unknown.State = syncalert.CDCDelayStateUnknown
	f.handle(t, f.cdcSignal(syncalert.KindFire, 7, 70, highDetail(192_000, f.now.Add(-6*time.Minute)), "high"))
	f.handle(t, f.cdcSignal(syncalert.KindRefresh, 7, 70, unknown, "无法确认"))
	listed, err := f.repo.ListFiringSyncAlertEpisodes(f.ctx, 10)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: %v %d", err, len(listed))
	}
	backdateDelivery(t, f, buildSyncAlertSourceKey(listed[0].ID), 2*time.Minute)

	// 停用会等正在进行的提醒检查结束（只是检查，不含网络投递）。 / Disabling waits for an in-flight reminder check (only the
	// check, never the network delivery).
	f.service.syncAlertEpisodeMu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := f.service.SaveTaskAlertConfig(f.ctx, syncalert.TaskRef{ID: 7, Name: "orders-sync", ClusterID: f.clusterID, CreatorID: 1}, &syncalert.TaskConfig{
			Enabled: false, Rules: []string{syncalert.RuleCDCDelayHigh}, ChannelIDs: []uint{f.channel.ID}, CooldownMinutes: 1, SendRecovery: true,
		})
		done <- err
	}()
	select {
	case err := <-done:
		f.service.syncAlertEpisodeMu.Unlock()
		t.Fatalf("disabling must wait for the reminder lock, finished early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	f.service.syncAlertEpisodeMu.Unlock()
	if err := <-done; err != nil {
		t.Fatalf("disable: %v", err)
	}

	// 巡检拿着停用之前列出的阶段：按 ID 重读后已关闭，不发提醒。
	// The sweep holds the episode listed before the disable: re-read by ID it is closed, so nothing is sent.
	before := f.webhook.count()
	if err := f.hub.remindEpisode(f.ctx, listed[0].ID); err != nil {
		t.Fatalf("remind: %v", err)
	}
	if err := f.hub.sweep(f.ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if f.webhook.count() != before {
		t.Fatalf("no reminder may be sent once disabled, got %d new", f.webhook.count()-before)
	}
	if episode := f.episodes(t)[0]; episode.Status != SyncAlertEpisodeResolved || episode.ResolveReason != syncalert.ResolveReasonRuleRemoved {
		t.Fatalf("disable closes the episode silently: %+v", episode)
	}
}

// 提醒内容是最新的：巡检前先处理已排队的 Refresh（与 30 秒判定同一时刻到达）。
// Reminder content is current: Refreshes already queued (arriving with the 30s round) are handled before the sweep.
func TestSyncAlertCDCDelay_reminderUsesQueuedRefresh(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.configureCDC(t, 7, []string{syncalert.RuleCDCDelayHigh}, 1, 300, 5)
	above := f.now.Add(-6 * time.Minute)
	f.handle(t, f.cdcSignal(syncalert.KindFire, 7, 70, highDetail(400_000, above), "high"))
	episode := f.episodes(t)[0]
	backdateDelivery(t, f, buildSyncAlertSourceKey(episode.ID), 2*time.Minute)
	recovering := highDetail(120_000, above)
	recovering.State = syncalert.CDCDelayStateRecovering
	recovering.ThresholdSeconds = 300
	f.hub.ReportSyncAlert(f.cdcSignal(syncalert.KindRefresh, 7, 70, recovering, "读取延迟已回落到 2分，低于阈值 5分；再确认一轮后自动关闭"))
	f.hub.drainQueued(f.ctx)
	if len(f.hub.queue) != 0 {
		t.Fatalf("queued signals must be handled before the sweep")
	}
	if err := f.hub.sweep(f.ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	reminder := f.webhook.last()
	title, _ := reminder["title"].(string)
	text, _ := reminder["message"].(string)
	if !strings.Contains(title, "（已回落，待确认） [提醒]") || !strings.Contains(text, "当前读取延迟：2分") || !strings.Contains(text, "读取延迟超过 5分") {
		t.Fatalf("reminder must use the queued refresh content:\n%s\n%s", title, text)
	}
	if alert, _ := reminder["alert"].(map[string]any); alert["delay_state"] != "recovering" || alert["threshold_seconds"] != float64(300) {
		t.Fatalf("reminder webhook must be current: %#v", alert)
	}
}

// 改变阶段状态的 Refresh（进入无法确认、停止提醒、回落待确认）不受队列过半的限制；TryReportSyncAlert 告诉调用方是否进了队列。
// State-changing Refreshes (entering unconfirmable, pausing reminders, recovering) bypass the half-queue limit, and
// TryReportSyncAlert tells the caller whether the signal was queued.
func TestSyncAlertCDCDelay_stateChangeRefreshBypassesHalfQueueLimit(t *testing.T) {
	f := setupSyncAlertFixture(t)
	hub := f.service.NewSyncAlertHub() // 不启动。 / Not started.
	refresh := f.cdcSignal(syncalert.KindRefresh, 7, 70, highDetail(120_000, f.now), "refresh")
	for i := 0; i < syncAlertRefreshQueueLimit; i++ {
		if !hub.TryReportSyncAlert(refresh) {
			t.Fatalf("refresh %d below the limit must be accepted", i)
		}
	}
	if hub.TryReportSyncAlert(refresh) {
		t.Fatalf("an ordinary refresh past half capacity must be rejected")
	}
	unknown := highDetail(120_000, f.now)
	unknown.State = syncalert.CDCDelayStateUnknown
	transition := f.cdcSignal(syncalert.KindRefresh, 7, 70, unknown, "无法确认")
	transition.StateChange = true
	if !hub.TryReportSyncAlert(transition) || len(hub.queue) != syncAlertRefreshQueueLimit+1 {
		t.Fatalf("a state-changing refresh must still be queued (queue=%d)", len(hub.queue))
	}
	// 缓冲区真满时照样拒绝并告诉调用方。 / A completely full buffer still rejects and says so.
	for len(hub.queue) < syncAlertQueueSize {
		hub.queue <- refresh
	}
	if hub.TryReportSyncAlert(transition) {
		t.Fatalf("a full buffer must reject")
	}
}

// 提醒投递不持锁：webhook 卡住时保存策略立刻返回；检查之后策略被保存（停用）的，投递前核对版本，这次提醒不发。
// Reminder delivery holds no lock: saving a policy returns at once while a webhook hangs; and a policy saved
// (disabled) after the check is caught by the version re-check before delivery, so that reminder is not sent.
func TestSyncAlertCDCDelay_hungWebhookDoesNotBlockPolicySave(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.configureCDC(t, 7, []string{syncalert.RuleCDCDelayHigh}, 1, 60, 5)
	f.handle(t, f.cdcSignal(syncalert.KindFire, 7, 70, highDetail(192_000, f.now.Add(-6*time.Minute)), "high"))
	episode := f.episodes(t)[0]
	backdateDelivery(t, f, buildSyncAlertSourceKey(episode.ID), 2*time.Minute)

	release := make(chan struct{})
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-time.After(30 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer hung.Close()
	defer close(release)
	f.channel.Endpoint = hung.URL
	if err := f.repo.SaveNotificationChannel(f.ctx, f.channel); err != nil {
		t.Fatalf("save channel: %v", err)
	}
	remindDone := make(chan error, 1)
	go func() { remindDone <- f.hub.remindEpisode(f.ctx, episode.ID) }()
	time.Sleep(300 * time.Millisecond) // 提醒已在投递，卡在 webhook。 / The reminder is delivering, stuck on the webhook.
	start := time.Now()
	f.configureCDC(t, 7, []string{syncalert.RuleCDCDelayHigh}, 1, 60, 5)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("policy save blocked %v behind a reminder to a hung webhook", took)
	}
	release <- struct{}{}
	<-remindDone

	// 检查之后、投递之前保存了策略（停用）：版本变了，不发。
	// A policy saved (disabled) between the check and the delivery: the version changed, nothing is sent.
	f.channel.Endpoint = f.webhook.server.URL
	if err := f.repo.SaveNotificationChannel(f.ctx, f.channel); err != nil {
		t.Fatalf("save channel: %v", err)
	}
	backdateDelivery(t, f, buildSyncAlertSourceKey(episode.ID), 2*time.Minute)
	before := f.webhook.count()
	f.hub.beforeRemindDispatch = func() {
		if _, err := f.service.SaveTaskAlertConfig(f.ctx, syncalert.TaskRef{ID: 7, Name: "orders-sync", ClusterID: f.clusterID, CreatorID: 1}, &syncalert.TaskConfig{
			Enabled: false, Rules: []string{syncalert.RuleCDCDelayHigh}, ChannelIDs: []uint{f.channel.ID}, CooldownMinutes: 1, SendRecovery: true,
		}); err != nil {
			t.Errorf("disable: %v", err)
		}
	}
	defer func() { f.hub.beforeRemindDispatch = nil }()
	if err := f.hub.remindEpisode(f.ctx, episode.ID); err != nil {
		t.Fatalf("remind: %v", err)
	}
	if f.webhook.count() != before {
		t.Fatalf("a reminder checked before the disable must not be delivered, got %d new", f.webhook.count()-before)
	}
}

// 只接受 TCP、不发问候语的 SMTP 服务器：连接截止时间在读问候语之前就设置，openSMTPClient 按超时返回，不会一直卡住。
// An SMTP server that accepts TCP but never greets: the deadline is set before the greeting is read, so
// openSMTPClient returns on timeout instead of hanging.
func TestOpenSMTPClient_silentServerTimesOut(t *testing.T) {
	previous := smtpIOTimeout
	smtpIOTimeout = 300 * time.Millisecond
	t.Cleanup(func() { smtpIOTimeout = previous })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 4)
	defer func() {
		select {
		case c := <-accepted:
			_ = c.Close()
		default:
		}
	}()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- c // 接受后什么也不发。 / Accept and never say anything.
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	done := make(chan error, 1)
	go func() {
		_, _, err := openSMTPClient(context.Background(), &NotificationChannelEmailConfig{Host: "127.0.0.1", Port: port, Security: NotificationEmailSecurityNone})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("a silent server must fail, not succeed")
		}
		var netErr net.Error
		if !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Fatalf("expected a timeout error, got %v", err)
		}
	case <-time.After(15 * time.Second): // 远大于测试里的 300 毫秒，留给合并 #83 后的"连接 + 读写"截止时间。 / Far above 300ms; room for #83's dial + IO deadline.
		t.Fatalf("openSMTPClient still blocked on a server that never greets")
	}
}

// t1 实测 B1：去掉 B4（不带参数）再勾回，阈值和持续时间仍是原来的 120 / 2；去掉期间读出的配置也是 120 / 2，
// 只保存参数的条件不算规则；策略中心按原样回写条件（快捷启停）也能通过校验并保留参数。
// t1 live test B1: removing B4 (without params) and adding it back keeps the original 120 / 2; the config read while
// B4 is off is 120 / 2 too and the params-only condition is not a rule; the policy center writing the conditions back
// as-is (quick enable toggle) passes validation and keeps the params.
func TestSyncAlertCDCDelay_paramsKeptWhenRuleRemovedAndAddedBack(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.configureCDC(t, 7, []string{syncalert.RuleJobFailed, syncalert.RuleCDCDelayHigh}, 5, 120, 2)
	// sync 的 PUT 不带 cdc_delay_* 时用已保存的值（TaskAlertRequest.toTaskConfig），这里模拟同样的调用。
	// A sync PUT without cdc_delay_* uses the stored values (TaskAlertRequest.toTaskConfig); simulate that call.
	stored, err := f.service.GetTaskAlertConfig(f.ctx, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	removed := f.configureCDC(t, 7, []string{syncalert.RuleJobFailed}, 5, stored.CDCDelayThresholdSeconds, stored.CDCDelayDurationMinutes)
	if removed.HasRule(syncalert.RuleCDCDelayHigh) || len(removed.Rules) != 1 || removed.CDCDelayThresholdSeconds != 120 || removed.CDCDelayDurationMinutes != 2 {
		t.Fatalf("removing B4 must keep 120 / 2 and report only sync_job_failed: %+v", removed)
	}
	got, err := f.service.GetTaskAlertConfig(f.ctx, 7)
	if err != nil || got.CDCDelayThresholdSeconds != 120 || got.CDCDelayDurationMinutes != 2 || !reflect.DeepEqual(got.Rules, []string{syncalert.RuleJobFailed}) {
		t.Fatalf("stored config while B4 is off: %+v err=%v", got, err)
	}

	// 策略中心快捷停用 / 启用：按 DTO 原样回写条件。 / Policy center quick toggle: conditions written back as-is.
	policy, err := f.repo.GetSyncJobAlertPolicyByTaskID(f.ctx, 7)
	if err != nil || policy == nil {
		t.Fatalf("policy: %v", err)
	}
	dto := toAlertPolicyDTO(policy)
	enabled := false
	cooldown := dto.CooldownMinutes
	sendRecovery := dto.SendRecovery
	notifyOwner := dto.NotifyTaskOwner
	if _, err := f.service.UpdateAlertPolicy(f.ctx, policy.ID, &UpsertAlertPolicyRequest{
		Name: dto.Name, PolicyType: dto.PolicyType, TemplateKey: dto.TemplateKey, ClusterID: dto.ClusterID, Severity: dto.Severity,
		Enabled: &enabled, CooldownMinutes: &cooldown, SendRecovery: &sendRecovery, Conditions: dto.Conditions,
		NotificationChannelIDs: dto.NotificationChannelIDs, ReceiverUserIDs: dto.ReceiverUserIDs, TaskID: dto.TaskID, NotifyTaskOwner: &notifyOwner,
	}); err != nil {
		t.Fatalf("quick toggle with the params-only condition must pass: %v", err)
	}
	if got, err = f.service.GetTaskAlertConfig(f.ctx, 7); err != nil || got.Enabled || got.CDCDelayThresholdSeconds != 120 || got.CDCDelayDurationMinutes != 2 {
		t.Fatalf("quick toggle must keep the params: %+v err=%v", got, err)
	}

	stored = got
	back := f.configureCDC(t, 7, []string{syncalert.RuleJobFailed, syncalert.RuleCDCDelayHigh}, 5, stored.CDCDelayThresholdSeconds, stored.CDCDelayDurationMinutes)
	if !back.HasRule(syncalert.RuleCDCDelayHigh) || back.CDCDelayThresholdSeconds != 120 || back.CDCDelayDurationMinutes != 2 {
		t.Fatalf("adding B4 back must restore 120 / 2: %+v", back)
	}
	policy, _ = f.repo.GetSyncJobAlertPolicyByTaskID(f.ctx, 7)
	if strings.Contains(policy.ConditionsJSON, syncJobCDCDelayParamsKey) {
		t.Fatalf("with B4 selected the params live in the B4 condition only: %s", policy.ConditionsJSON)
	}

	// 只保存参数的条件不能单独存在，也不能重复；参数同样校验范围。
	// The params-only condition cannot stand alone or repeat, and its params are range-checked too.
	params := &AlertPolicyConditionDTO{MetricKey: syncJobCDCDelayParamsKey, Operator: ">=", Threshold: "120", WindowMinutes: 2}
	rule := &AlertPolicyConditionDTO{MetricKey: syncalert.RuleJobFailed, Operator: ">=", Threshold: "1"}
	base := func(conditions ...*AlertPolicyConditionDTO) *UpsertAlertPolicyRequest {
		return &UpsertAlertPolicyRequest{Name: "x", PolicyType: AlertPolicyBuilderKindSyncJob, Severity: AlertSeverityCritical, TaskID: 7, Conditions: conditions, NotificationChannelIDs: []uint{f.channel.ID}}
	}
	if err := base(params).Validate(); !errors.Is(err, ErrAlertPolicySyncJobInvalidRule) {
		t.Fatalf("params alone is not a rule: %v", err)
	}
	if err := base(rule, params, params).Validate(); !errors.Is(err, ErrAlertPolicySyncJobInvalidRule) {
		t.Fatalf("params twice must be rejected: %v", err)
	}
	bad := *params
	bad.Threshold = "5"
	if err := base(rule, &bad).Validate(); !errors.Is(err, ErrAlertPolicySyncJobInvalidCDCDelay) {
		t.Fatalf("params out of range must be rejected: %v", err)
	}
	if err := base(rule, params).Validate(); err != nil {
		t.Fatalf("rule + params is valid: %v", err)
	}
}

// t1 实测 B3：B4 阶段因恢复以外的原因关闭（job_ended、channel_unavailable、rule_changed、rule_removed、policy_deleted）后，
// last_detail.state 改成 closed 并带 resolve_reason，不再停在 high / recovering；恢复关闭保留 recovered。
// t1 live test B3: after a B4 episode closes for a reason other than recovery (job_ended, channel_unavailable,
// rule_changed, rule_removed, policy_deleted) last_detail.state becomes closed with resolve_reason instead of staying
// at high / recovering; a recovery keeps recovered.
func TestSyncAlertCDCDelay_closedEpisodeDetailStateIsClosed(t *testing.T) {
	f := setupSyncAlertFixture(t)
	above := f.now.Add(-6 * time.Minute)
	recovering := highDetail(20_000, above)
	recovering.State = syncalert.CDCDelayStateRecovering

	b4Episode := func(taskID uint) *SyncAlertEpisode {
		t.Helper()
		for _, episode := range f.episodes(t) {
			if episode.TaskID == taskID && episode.RuleKey == syncalert.RuleCDCDelayHigh {
				return episode
			}
		}
		t.Fatalf("task %d: B4 episode missing", taskID)
		return nil
	}
	expectClosed := func(taskID uint, reason string) {
		t.Helper()
		episode := b4Episode(taskID)
		detail := episodeCDCDelayDetail(episode)
		if episode.Status != SyncAlertEpisodeResolved || episode.ResolveReason != reason || detail == nil ||
			detail.State != syncalert.CDCDelayStateClosed || detail.ResolveReason != reason {
			t.Fatalf("%s: detail state must be closed: %+v detail=%+v", reason, episode, detail)
		}
		if detail.ThresholdSeconds != 60 || detail.CurrentDelayMs == 0 {
			t.Fatalf("%s: the rest of the last detail is kept: %+v", reason, detail)
		}
	}

	// sync 发来的 Resolve：job_ended / channel_unavailable 停在 high，rule_changed 停在 recovering（rearm 期间的 Refresh）。
	// Resolves from sync: job_ended / channel_unavailable were at high, rule_changed at recovering (Refresh while rearmed).
	cases := []struct {
		reason     string
		recovering bool
	}{
		{syncalert.ResolveReasonJobEnded, false},
		{syncalert.ResolveReasonChannelOff, false},
		{syncalert.ResolveReasonRuleChanged, true},
	}
	for i, tc := range cases {
		taskID := uint(40 + i)
		f.configureCDC(t, taskID, []string{syncalert.RuleCDCDelayHigh}, 60, 60, 5)
		f.handle(t, f.cdcSignal(syncalert.KindFire, taskID, 70, highDetail(120_000, above), "high"))
		if tc.recovering {
			f.handle(t, f.cdcSignal(syncalert.KindRefresh, taskID, 70, recovering, "recovering"))
			if detail := episodeCDCDelayDetail(b4Episode(taskID)); detail == nil || detail.State != syncalert.CDCDelayStateRecovering {
				t.Fatalf("precondition: recovering detail, got %+v", detail)
			}
		}
		closeSignal := f.cdcSignal(syncalert.KindResolve, taskID, 70, syncalert.CDCDelayDetail{}, "")
		closeSignal.CDCDelay = nil
		closeSignal.ResolveReason = tc.reason
		f.handle(t, closeSignal)
		expectClosed(taskID, tc.reason)
	}

	// 保存配置去掉 B4：rule_removed。 / Saving the config without B4: rule_removed.
	f.configureCDC(t, 50, []string{syncalert.RuleJobFailed, syncalert.RuleCDCDelayHigh}, 60, 60, 5)
	f.handle(t, f.cdcSignal(syncalert.KindFire, 50, 70, highDetail(120_000, above), "high"))
	f.handle(t, f.cdcSignal(syncalert.KindRefresh, 50, 70, recovering, "recovering"))
	f.configureCDC(t, 50, []string{syncalert.RuleJobFailed}, 60, 60, 5)
	expectClosed(50, syncalert.ResolveReasonRuleRemoved)

	// 删除配置：policy_deleted。 / Deleting the config: policy_deleted.
	f.configureCDC(t, 51, []string{syncalert.RuleCDCDelayHigh}, 60, 60, 5)
	f.handle(t, f.cdcSignal(syncalert.KindFire, 51, 70, highDetail(120_000, above), "high"))
	if err := f.service.DeleteTaskAlertConfig(f.ctx, 51); err != nil {
		t.Fatalf("delete: %v", err)
	}
	expectClosed(51, syncAlertResolvePolicyDeleted)

	// 恢复：保留 sync 送来的 recovered 明细。 / Recovery keeps the recovered detail sent by sync.
	f.configureCDC(t, 52, []string{syncalert.RuleCDCDelayHigh}, 60, 60, 5)
	f.handle(t, f.cdcSignal(syncalert.KindFire, 52, 70, highDetail(120_000, above), "high"))
	recovered := highDetail(20_000, above)
	recovered.State = syncalert.CDCDelayStateRecovered
	recovered.RecoveryBasis = syncalert.CDCDelayRecoveryBySamples
	f.handle(t, f.cdcSignal(syncalert.KindResolve, 52, 70, recovered, "recovered"))
	episode := b4Episode(52)
	if detail := episodeCDCDelayDetail(episode); episode.Status != SyncAlertEpisodeResolved || detail == nil ||
		detail.State != syncalert.CDCDelayStateRecovered || detail.ResolveReason != "" {
		t.Fatalf("recovery keeps recovered: %+v detail=%+v", episode, detail)
	}
}
