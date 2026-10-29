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
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

func TestSyncRecoveryRetryDelay_Backoff(t *testing.T) {
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	for i, expected := range want {
		if got := syncDeliveryRetryDelay(i + 1); got != expected {
			t.Fatalf("attempt %d: got %s want %s", i+1, got, expected)
		}
	}
}

func (c *imCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

func recoveryDelivery(t *testing.T, f *syncAlertFixture, sourceKey string) *NotificationDelivery {
	t.Helper()
	delivery, err := f.repo.GetNotificationDeliveryByDedupKey(f.ctx, sourceKey, f.channel.ID, string(NotificationDeliveryEventTypeResolved))
	if err != nil || delivery == nil {
		t.Fatalf("recovery delivery not found: %v", err)
	}
	return delivery
}

// ageDelivery 把投递记录的 updated_at 往前挪，模拟退避时间已过（UpdateColumn 不会触发 autoUpdateTime）。
// ageDelivery moves the delivery's updated_at back to simulate the backoff passing (UpdateColumn skips autoUpdateTime).
func ageDelivery(t *testing.T, f *syncAlertFixture, id uint, by time.Duration) {
	t.Helper()
	if err := f.repo.db.Model(&NotificationDelivery{}).Where("id = ?", id).UpdateColumn("updated_at", time.Now().UTC().Add(-by)).Error; err != nil {
		t.Fatalf("age delivery: %v", err)
	}
}

func setupFailingRecoveryFixture(t *testing.T) (*syncAlertFixture, *imCapture, string) {
	t.Helper()
	f := setupSyncAlertFixture(t)
	capture := newIMCapture(t, `{}`)
	f.channel.Endpoint = capture.server.URL + "/hook"
	if err := f.repo.SaveNotificationChannel(f.ctx, f.channel); err != nil {
		t.Fatal(err)
	}
	f.configureTask(t, 7, []string{syncalert.RuleJobFailed}, 60)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
	if capture.count() != 1 {
		t.Fatalf("firing notification expected, got %d", capture.count())
	}
	sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)

	// 恢复时渠道挂了：恢复投递失败。
	// The channel is down at recovery: the recovery delivery fails.
	capture.set(http.StatusBadGateway, `bad gateway`)
	resolve := f.signal(syncalert.KindResolve, "", 7, 71)
	resolve.ResolveScope = syncalert.ResolveTaskRunHealthy
	if err := f.hub.handle(f.ctx, resolve); err == nil {
		t.Fatalf("recovery dispatch should report the failure")
	}
	if delivery := recoveryDelivery(t, f, sourceKey); delivery.Status != string(NotificationDeliveryStatusFailed) || delivery.AttemptCount != 1 {
		t.Fatalf("recovery delivery should be failed after one attempt: %+v", delivery)
	}
	return f, capture, sourceKey
}

func TestSyncAlertHub_RetriesFailedRecoveryWithBackoff(t *testing.T) {
	f, capture, sourceKey := setupFailingRecoveryFixture(t)
	requests := capture.count()

	sweep := func() {
		t.Helper()
		_ = f.hub.sweep(f.ctx)
	}

	// 退避未到：不重试。
	// Backoff not elapsed: no retry.
	sweep()
	if capture.count() != requests {
		t.Fatalf("must not retry before the 1 minute backoff")
	}

	// 1 分钟后第一次重试，仍失败。
	// First retry after 1 minute, still failing.
	delivery := recoveryDelivery(t, f, sourceKey)
	ageDelivery(t, f, delivery.ID, 61*time.Second)
	sweep()
	if capture.count() != requests+1 {
		t.Fatalf("expected one retry after the backoff, got %d requests", capture.count()-requests)
	}
	if delivery = recoveryDelivery(t, f, sourceKey); delivery.AttemptCount != 2 || delivery.Status != string(NotificationDeliveryStatusFailed) {
		t.Fatalf("second attempt should be recorded as failed: %+v", delivery)
	}

	// 第二次失败后要等 2 分钟。
	// After the second failure the wait is 2 minutes.
	ageDelivery(t, f, delivery.ID, 61*time.Second)
	sweep()
	if capture.count() != requests+1 {
		t.Fatalf("must wait 2 minutes after the second failure")
	}

	// 渠道恢复：重试成功，之后不再发送。
	// The channel recovers: the retry succeeds and nothing more is sent.
	capture.set(http.StatusOK, `{}`)
	ageDelivery(t, f, delivery.ID, 121*time.Second)
	sweep()
	if capture.count() != requests+2 {
		t.Fatalf("expected the retry after 2 minutes, got %d", capture.count()-requests)
	}
	if delivery = recoveryDelivery(t, f, sourceKey); delivery.Status != string(NotificationDeliveryStatusSent) || delivery.AttemptCount != 3 {
		t.Fatalf("recovery should be sent on the third attempt: %+v", delivery)
	}
	body := capture.lastBody(t)
	if !containsJSON(body, `"status":"resolved"`) {
		t.Fatalf("retry must resend the recovery payload: %s", body)
	}
	ageDelivery(t, f, delivery.ID, time.Hour)
	sweep()
	if capture.count() != requests+2 {
		t.Fatalf("a sent recovery must not be resent")
	}
}

func TestSyncAlertHub_RecoveryRetryStopsAtLimitAndOnRefire(t *testing.T) {
	f, capture, sourceKey := setupFailingRecoveryFixture(t)
	requests := capture.count()

	delivery := recoveryDelivery(t, f, sourceKey)
	delivery.AttemptCount = syncDeliveryRetryMaxAttempts
	if err := f.repo.SaveNotificationDelivery(f.ctx, delivery); err != nil {
		t.Fatal(err)
	}
	ageDelivery(t, f, delivery.ID, time.Hour)
	_ = f.hub.sweep(f.ctx)
	if capture.count() != requests {
		t.Fatalf("exhausted retries must not resend")
	}

	// 次数没用完，但同一任务又开始告警：迟到的恢复不再补发。
	// Attempts left, but the task is firing again: the late recovery is not sent.
	delivery.AttemptCount = 1
	if err := f.repo.SaveNotificationDelivery(f.ctx, delivery); err != nil {
		t.Fatal(err)
	}
	capture.set(http.StatusOK, `{}`)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 72))
	requests = capture.count()
	ageDelivery(t, f, delivery.ID, time.Hour)
	_ = f.hub.sweep(f.ctx)
	if capture.count() != requests {
		t.Fatalf("a recovery for an episode that fired again must not be retried")
	}
}

func containsJSON(body []byte, fragment string) bool {
	return strings.Contains(string(body), fragment)
}

// 100+ 条可跳过的失败恢复投递（阶段已删除 / 无策略）和 100+ 条未到退避时间的记录排在真实恢复投递前面时，真实投递
// 必须在一轮巡检内发出；可跳过的记录被标成 canceled，不再进入后续扫描。
// With 100+ skippable failed recovery deliveries (episode deleted / no policy) and 100+ rows still in backoff ahead of
// a real due recovery, the real one must go out in a single sweep; skippable rows become canceled and leave later scans.
func TestSyncAlertHub_RecoveryRetrySkippableRowsDoNotStarveDueRecovery(t *testing.T) {
	f, capture, sourceKey := setupFailingRecoveryFixture(t)
	capture.set(http.StatusOK, `{}`)
	requests := capture.count()

	const skippable = syncDeliveryRetryBatchSize + 20
	const inBackoff = syncDeliveryRetryBatchSize + 20
	skippedIDs := make([]uint, 0, skippable)
	for i := 0; i < skippable+inBackoff; i++ {
		row := &NotificationDelivery{
			SourceType:   string(AlertSourceTypeSync),
			SourceKey:    buildSyncAlertSourceKey(uint(900000 + i)),
			ChannelID:    f.channel.ID,
			ChannelName:  f.channel.Name,
			EventType:    string(NotificationDeliveryEventTypeResolved),
			Status:       string(NotificationDeliveryStatusFailed),
			AttemptCount: 1,
			LastError:    "bad gateway",
		}
		if i%2 == 1 {
			row.PolicyID = 1
		}
		if i >= skippable {
			row.AttemptCount = syncDeliveryRetryMaxAttempts - 1
		}
		if err := f.repo.CreateNotificationDelivery(f.ctx, row); err != nil {
			t.Fatalf("create stale delivery: %v", err)
		}
		if i < skippable {
			ageDelivery(t, f, row.ID, 2*time.Hour)
			skippedIDs = append(skippedIDs, row.ID)
		}
	}
	// 把真实恢复投递挪到所有干扰记录之后（更大的 id、更新的 updated_at），复现它排在批次之外的情况。
	// Move the real recovery delivery behind every distractor (larger id, newer updated_at) so it starts outside the
	// first batch.
	real := recoveryDelivery(t, f, sourceKey)
	if err := f.repo.db.Model(&NotificationDelivery{}).Where("id = ?", real.ID).UpdateColumn("id", 10_000_000).Error; err != nil {
		t.Fatal(err)
	}
	ageDelivery(t, f, 10_000_000, 61*time.Second)

	_ = f.hub.sweep(f.ctx)
	if capture.count() != requests+1 {
		t.Fatalf("the due recovery must be sent in one sweep despite %d skippable and %d in-backoff rows, got %d requests",
			skippable, inBackoff, capture.count()-requests)
	}
	if delivery := recoveryDelivery(t, f, sourceKey); delivery.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("real recovery should be sent: %+v", delivery)
	}
	for _, id := range skippedIDs {
		var row NotificationDelivery
		if err := f.repo.db.First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		if row.Status != string(NotificationDeliveryStatusCanceled) || !strings.Contains(row.LastError, "retry canceled") {
			t.Fatalf("skippable row %d should be canceled: %+v", id, row)
		}
	}
	remaining, err := f.repo.ListRetryableSyncAlertDeliveries(f.ctx, syncDeliveryRetryMaxAttempts, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != inBackoff {
		t.Fatalf("only the in-backoff rows should stay retryable, got %d", len(remaining))
	}
}

// setupCapturedTaskAlert 建一个投递到 imCapture 的任务级告警配置，返回 fixture 和 capture。
// setupCapturedTaskAlert configures a task alert delivering to an imCapture and returns the fixture and capture.
func setupCapturedTaskAlert(t *testing.T, cooldown int) (*syncAlertFixture, *imCapture) {
	t.Helper()
	f := setupSyncAlertFixture(t)
	capture := newIMCapture(t, `{}`)
	f.channel.Endpoint = capture.server.URL + "/hook"
	if err := f.repo.SaveNotificationChannel(f.ctx, f.channel); err != nil {
		t.Fatal(err)
	}
	f.configureTask(t, 7, []string{syncalert.RuleJobFailed}, cooldown)
	return f, capture
}

func deliveryOf(t *testing.T, f *syncAlertFixture, sourceKey string, eventType NotificationDeliveryEventType) *NotificationDelivery {
	t.Helper()
	delivery, err := f.repo.GetNotificationDeliveryByDedupKey(f.ctx, sourceKey, f.channel.ID, string(eventType))
	if err != nil {
		t.Fatalf("load %s delivery: %v", eventType, err)
	}
	return delivery
}

func resolveTask7(t *testing.T, f *syncAlertFixture) error {
	t.Helper()
	resolve := f.signal(syncalert.KindResolve, "", 7, 71)
	resolve.ResolveScope = syncalert.ResolveTaskRunHealthy
	return f.hub.handle(f.ctx, resolve)
}

// 1：首次告警发送成功、冷却提醒发送失败、条件恢复 → 恢复通知照常发出；提醒写自己的 reminder 记录，不覆盖首次告警记录。
// 1: first alert sent, cooldown reminder fails, condition recovers -> the recovery still goes out; the reminder writes
// its own reminder record and never overwrites the first alert record.
func TestSyncAlertHub_ReminderFailureDoesNotBlockRecovery(t *testing.T) {
	f, capture := setupCapturedTaskAlert(t, 1)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
	if capture.count() != 1 {
		t.Fatalf("first alert expected, got %d", capture.count())
	}
	sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)

	// 冷却过后提醒，渠道此时挂了。
	// After the cooldown the reminder goes out while the channel is down.
	capture.set(http.StatusBadGateway, `bad gateway`)
	backdateDelivery(t, f, sourceKey, 2*time.Minute)
	_ = f.hub.sweep(f.ctx)
	if capture.count() != 2 {
		t.Fatalf("a reminder should be attempted after the cooldown, got %d requests", capture.count())
	}
	firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring)
	if firing == nil || firing.Status != string(NotificationDeliveryStatusSent) || firing.AttemptCount != 1 {
		t.Fatalf("the reminder failure must not touch the first alert record: %+v", firing)
	}
	reminder := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeReminder)
	if reminder == nil || reminder.Status != string(NotificationDeliveryStatusFailed) || reminder.AttemptCount != 1 {
		t.Fatalf("the failed reminder needs its own failed record: %+v", reminder)
	}

	// 失败的提醒不进退避重试：下一轮巡检不会马上重发提醒。
	// A failed reminder is not part of the backoff retry: the next sweep does not resend it right away.
	ageDelivery(t, f, reminder.ID, 61*time.Second)
	_ = f.hub.sweep(f.ctx)
	if capture.count() != 2 {
		t.Fatalf("a failed reminder waits for the next cooldown, got %d requests", capture.count())
	}

	// 渠道恢复、条件恢复：恢复通知发出。
	// Channel and condition recover: the recovery goes out.
	capture.set(http.StatusOK, `{}`)
	if err := resolveTask7(t, f); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if capture.count() != 3 || !containsJSON(capture.lastBody(t), `"status":"resolved"`) {
		t.Fatalf("recovery must be sent after a failed reminder, got %d requests", capture.count())
	}
	if resolved := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeResolved); resolved == nil || resolved.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("recovery delivery should be sent: %+v", resolved)
	}
}

// 1：提醒成功后下一次提醒从提醒时间算冷却；首次告警记录保持第一次发送的状态。
// 1: after a successful reminder the next cooldown counts from the reminder; the first alert record keeps its state.
func TestSyncAlertHub_ReminderHasOwnRecordAndCooldown(t *testing.T) {
	f, capture := setupCapturedTaskAlert(t, 1)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
	sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)
	backdateDelivery(t, f, sourceKey, 2*time.Minute)
	_ = f.hub.sweep(f.ctx)
	if capture.count() != 2 {
		t.Fatalf("expected one reminder, got %d requests", capture.count())
	}
	reminder := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeReminder)
	if reminder == nil || reminder.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("reminder record expected: %+v", reminder)
	}
	// 首次告警记录仍是 2 分钟前那次，但刚发过提醒：冷却未过，不再提醒。
	// The first alert record still says 2 minutes ago, but a reminder just went out: no reminder within the cooldown.
	_ = f.hub.sweep(f.ctx)
	if capture.count() != 2 {
		t.Fatalf("the cooldown must count from the last reminder, got %d requests", capture.count())
	}
	if firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); firing.AttemptCount != 1 {
		t.Fatalf("reminders must not bump the first alert record: %+v", firing)
	}
}

// 2：首次告警发送失败按退避重试，成功后之后的恢复通知照常发送；与冷却时间无关（0 和 60 分钟都一样）。
// 2: a failed first alert is retried with backoff and the later recovery goes out; independent of the cooldown (0 and
// 60 minutes behave the same).
func TestSyncAlertHub_FirstAlertFailureRetriedWithBackoff(t *testing.T) {
	for _, cooldown := range []int{0, 60} {
		t.Run(fmt.Sprintf("cooldown-%d", cooldown), func(t *testing.T) {
			f, capture := setupCapturedTaskAlert(t, cooldown)
			capture.set(http.StatusBadGateway, `bad gateway`)
			if err := f.hub.handle(f.ctx, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70)); err == nil {
				t.Fatalf("the failed first alert should be reported")
			}
			sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)
			firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring)
			if firing == nil || firing.Status != string(NotificationDeliveryStatusFailed) || firing.AttemptCount != 1 {
				t.Fatalf("first alert should be failed after one attempt: %+v", firing)
			}

			// 退避未到：不重发（冷却提醒巡检也不能绕过退避）。
			// Backoff not elapsed: no resend (the reminder sweep must not bypass the backoff either).
			_ = f.hub.sweep(f.ctx)
			if capture.count() != 1 {
				t.Fatalf("must not resend before the 1 minute backoff, got %d requests", capture.count())
			}

			// 1 分钟后重试，仍失败；再等 2 分钟后重试成功。
			// Retry after 1 minute, still failing; succeed after another 2 minutes.
			ageDelivery(t, f, firing.ID, 61*time.Second)
			_ = f.hub.sweep(f.ctx)
			if firing = deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); capture.count() != 2 || firing.AttemptCount != 2 || firing.Status != string(NotificationDeliveryStatusFailed) {
				t.Fatalf("second attempt should fail: requests=%d %+v", capture.count(), firing)
			}
			capture.set(http.StatusOK, `{}`)
			ageDelivery(t, f, firing.ID, 61*time.Second)
			_ = f.hub.sweep(f.ctx)
			if capture.count() != 2 {
				t.Fatalf("the second failure waits 2 minutes, got %d requests", capture.count())
			}
			ageDelivery(t, f, firing.ID, 121*time.Second)
			_ = f.hub.sweep(f.ctx)
			if firing = deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); capture.count() != 3 || firing.Status != string(NotificationDeliveryStatusSent) || firing.AttemptCount != 3 {
				t.Fatalf("third attempt should deliver the first alert: requests=%d %+v", capture.count(), firing)
			}
			if !containsJSON(capture.lastBody(t), `"status":"firing"`) {
				t.Fatalf("the retry must resend the firing payload: %s", capture.lastBody(t))
			}

			if err := resolveTask7(t, f); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if capture.count() != 4 || !containsJSON(capture.lastBody(t), `"status":"resolved"`) {
				t.Fatalf("recovery must follow a first alert delivered by retry, got %d requests", capture.count())
			}
		})
	}
}

// 2（#85 复审 P3 更新）：阶段在首次告警送达前就恢复：发一条带"首次告警未送达"说明的恢复通知（用户至少知道出过告警、
// 已恢复），首次告警重试标记 canceled，不再补发过时的告警。
// 2 (updated for #85 review P3): the episode recovers before the first alert was delivered: one recovery that notes
// the first alert was not delivered goes out (the user at least learns there was an alert and it recovered), and the
// first alert retry is canceled so no stale alert follows.
func TestSyncAlertHub_FirstAlertRetryStopsOnResolve(t *testing.T) {
	f, capture := setupCapturedTaskAlert(t, 0)
	capture.set(http.StatusBadGateway, `bad gateway`)
	_ = f.hub.handle(f.ctx, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
	sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)
	capture.set(http.StatusOK, `{}`)
	if err := resolveTask7(t, f); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if capture.count() != 2 || !containsJSON(capture.lastBody(t), `"status":"resolved"`) ||
		!containsJSON(capture.lastBody(t), `"first_alert_delivered":false`) || !strings.Contains(string(capture.lastBody(t)), "此前的告警通知未能送达") {
		t.Fatalf("expected one recovery noting the undelivered first alert, got %d requests: %s", capture.count(), capture.lastBody(t))
	}
	if resolved := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeResolved); resolved == nil || resolved.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("the recovery should be recorded as sent: %+v", resolved)
	}
	firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring)
	ageDelivery(t, f, firing.ID, time.Hour)
	_ = f.hub.sweep(f.ctx)
	if capture.count() != 2 {
		t.Fatalf("a resolved episode must not retry its first alert, got %d requests", capture.count())
	}
	if firing = deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); firing.Status != string(NotificationDeliveryStatusCanceled) ||
		!strings.Contains(firing.LastError, "retry canceled") {
		t.Fatalf("first alert retry should be canceled: %+v", firing)
	}
}

// 2：策略停用、渠道删除或解绑、告警被人工关闭时首次告警重试标记 canceled；静默期间只跳过，静默结束后照常重试
// （渠道停用见 TestSyncAlertHub_RetryWaitsForTemporarilyDisabledChannel）。
// 2: a disabled policy, a deleted or unbound channel, or a manually closed alert cancels the first alert retry;
// silence only skips it and the retry resumes once the silence ends (disabled channels: see
// TestSyncAlertHub_RetryWaitsForTemporarilyDisabledChannel).
func TestSyncAlertHub_FirstAlertRetryStopsWhenDisabled(t *testing.T) {
	cases := map[string]func(t *testing.T, f *syncAlertFixture, sourceKey string){
		"policy-disabled": func(t *testing.T, f *syncAlertFixture, _ string) {
			policy, err := f.repo.GetSyncJobAlertPolicyByTaskID(f.ctx, 7)
			if err != nil || policy == nil {
				t.Fatalf("policy: %v", err)
			}
			if err := f.repo.db.Model(&AlertPolicy{}).Where("id = ?", policy.ID).UpdateColumn("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
		},
		"channel-deleted": func(t *testing.T, f *syncAlertFixture, _ string) {
			if err := f.repo.db.Delete(&NotificationChannel{}, f.channel.ID).Error; err != nil {
				t.Fatal(err)
			}
		},
		"channel-unbound": func(t *testing.T, f *syncAlertFixture, _ string) {
			policy, err := f.repo.GetSyncJobAlertPolicyByTaskID(f.ctx, 7)
			if err != nil || policy == nil {
				t.Fatalf("policy: %v", err)
			}
			if err := f.repo.db.Model(&AlertPolicy{}).Where("id = ?", policy.ID).UpdateColumn("notification_channel_ids_json", "[]").Error; err != nil {
				t.Fatal(err)
			}
		},
		"alert-closed": func(t *testing.T, f *syncAlertFixture, sourceKey string) {
			if _, err := f.service.CloseAlertInstance(f.ctx, sourceKey, "ops", "known issue"); err != nil {
				t.Fatalf("close: %v", err)
			}
		},
	}
	for name, disable := range cases {
		t.Run(name, func(t *testing.T) {
			f, capture := setupCapturedTaskAlert(t, 0)
			capture.set(http.StatusBadGateway, `bad gateway`)
			_ = f.hub.handle(f.ctx, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
			sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)
			capture.set(http.StatusOK, `{}`)
			firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring)
			ageDelivery(t, f, firing.ID, time.Hour)
			disable(t, f, sourceKey)
			_ = f.hub.sweep(f.ctx)
			if capture.count() != 1 {
				t.Fatalf("%s: first alert must not be retried, got %d requests", name, capture.count())
			}
			var row NotificationDelivery
			if err := f.repo.db.First(&row, firing.ID).Error; err != nil {
				t.Fatal(err)
			}
			if row.Status != string(NotificationDeliveryStatusCanceled) {
				t.Fatalf("%s: first alert retry should be canceled: %+v", name, row)
			}
		})
	}

	t.Run("silenced-then-resumed", func(t *testing.T) {
		f, capture := setupCapturedTaskAlert(t, 0)
		capture.set(http.StatusBadGateway, `bad gateway`)
		_ = f.hub.handle(f.ctx, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
		sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)
		capture.set(http.StatusOK, `{}`)
		if _, err := f.service.SilenceAlertInstance(f.ctx, sourceKey, "ops", 30, "maintenance"); err != nil {
			t.Fatalf("silence: %v", err)
		}
		firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring)
		ageDelivery(t, f, firing.ID, time.Hour)
		_ = f.hub.sweep(f.ctx)
		if firing = deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); capture.count() != 1 || firing.Status != string(NotificationDeliveryStatusFailed) {
			t.Fatalf("silenced: skip without a terminal state: requests=%d %+v", capture.count(), firing)
		}
		if err := f.repo.db.Model(&AlertState{}).Where("source_key = ?", sourceKey).UpdateColumn("silenced_until", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
			t.Fatalf("end silence: %v", err)
		}
		_ = f.hub.sweep(f.ctx)
		if firing = deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); capture.count() != 2 || firing.Status != string(NotificationDeliveryStatusSent) {
			t.Fatalf("after the silence the retry resumes: requests=%d %+v", capture.count(), firing)
		}
	})
}

// 复审 4：渠道临时停用时恢复通知不标终态，重新启用后补发；首次告警同理。
// Re-review 4: while the channel is temporarily disabled the recovery is not made terminal and goes out once the
// channel is re-enabled; the first alert behaves the same.
func TestSyncAlertHub_RetryWaitsForTemporarilyDisabledChannel(t *testing.T) {
	setEnabled := func(t *testing.T, f *syncAlertFixture, enabled bool) {
		t.Helper()
		f.channel.Enabled = enabled
		if err := f.repo.SaveNotificationChannel(f.ctx, f.channel); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("recovery", func(t *testing.T) {
		f, capture, sourceKey := setupFailingRecoveryFixture(t)
		requests := capture.count()
		capture.set(http.StatusOK, `{}`)
		setEnabled(t, f, false)
		delivery := recoveryDelivery(t, f, sourceKey)
		ageDelivery(t, f, delivery.ID, time.Hour)
		_ = f.hub.sweep(f.ctx)
		if delivery = recoveryDelivery(t, f, sourceKey); capture.count() != requests || delivery.Status != string(NotificationDeliveryStatusFailed) || delivery.AttemptCount != 1 {
			t.Fatalf("disabled channel: skip without using an attempt: requests=%d %+v", capture.count()-requests, delivery)
		}
		setEnabled(t, f, true)
		_ = f.hub.sweep(f.ctx)
		if delivery = recoveryDelivery(t, f, sourceKey); capture.count() != requests+1 || delivery.Status != string(NotificationDeliveryStatusSent) {
			t.Fatalf("re-enabled channel must catch up the recovery: requests=%d %+v", capture.count()-requests, delivery)
		}
	})

	// #85 复审 2：恢复通知的 24 小时上限从渠道停用时算起，不是从上次尝试算起。
	// #85 re-review 2: the recovery's 24 hour limit counts from when the channel was disabled, not from the last attempt.
	t.Run("recovery-last-attempt-old-but-disabled-recently", func(t *testing.T) {
		f, capture, sourceKey := setupFailingRecoveryFixture(t)
		requests := capture.count()
		capture.set(http.StatusOK, `{}`)
		setEnabled(t, f, false)
		delivery := recoveryDelivery(t, f, sourceKey)
		ageDelivery(t, f, delivery.ID, 48*time.Hour)
		ageChannelDisabledAt(t, f, time.Hour)
		_ = f.hub.sweep(f.ctx)
		if delivery = recoveryDelivery(t, f, sourceKey); delivery.Status != string(NotificationDeliveryStatusFailed) {
			t.Fatalf("disabled for 1 hour: the recovery must keep waiting: %+v", delivery)
		}
		setEnabled(t, f, true)
		_ = f.hub.sweep(f.ctx)
		if delivery = recoveryDelivery(t, f, sourceKey); capture.count() != requests+1 || delivery.Status != string(NotificationDeliveryStatusSent) {
			t.Fatalf("re-enabled channel must catch up the recovery: requests=%d %+v", capture.count()-requests, delivery)
		}
	})

	t.Run("recovery-disabled-over-24h", func(t *testing.T) {
		f, capture, sourceKey := setupFailingRecoveryFixture(t)
		requests := capture.count()
		capture.set(http.StatusOK, `{}`)
		setEnabled(t, f, false)
		delivery := recoveryDelivery(t, f, sourceKey)
		ageDelivery(t, f, delivery.ID, time.Hour)
		ageChannelDisabledAt(t, f, syncDeliveryRetryDisabledChannelTTL+time.Hour)
		_ = f.hub.sweep(f.ctx)
		if delivery = recoveryDelivery(t, f, sourceKey); delivery.Status != string(NotificationDeliveryStatusCanceled) ||
			!strings.Contains(delivery.LastError, "disabled for over 24 hours") {
			t.Fatalf("a recovery waiting 24+ hours for a disabled channel is given up: %+v", delivery)
		}
		setEnabled(t, f, true)
		_ = f.hub.sweep(f.ctx)
		if capture.count() != requests {
			t.Fatalf("a canceled recovery is not sent after re-enabling, got %d requests", capture.count()-requests)
		}
	})

	t.Run("first-alert", func(t *testing.T) {
		f, capture := setupCapturedTaskAlert(t, 0)
		capture.set(http.StatusBadGateway, `bad gateway`)
		_ = f.hub.handle(f.ctx, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
		sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)
		capture.set(http.StatusOK, `{}`)
		setEnabled(t, f, false)
		firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring)
		ageDelivery(t, f, firing.ID, time.Hour)
		// 停用超过 24 小时也不放弃首次告警：告警仍在触发。
		// Even after 24+ hours disabled the first alert is not given up: the alert is still firing.
		ageChannelDisabledAt(t, f, 48*time.Hour)
		_ = f.hub.sweep(f.ctx)
		if firing = deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); capture.count() != 1 || firing.Status != string(NotificationDeliveryStatusFailed) {
			t.Fatalf("disabled channel: skip the first alert retry: requests=%d %+v", capture.count(), firing)
		}
		setEnabled(t, f, true)
		_ = f.hub.sweep(f.ctx)
		if firing = deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); capture.count() != 2 || firing.Status != string(NotificationDeliveryStatusSent) {
			t.Fatalf("re-enabled channel must catch up the first alert: requests=%d %+v", capture.count(), firing)
		}
	})
}

// ageChannelDisabledAt 把渠道的 disabled_at 往前挪，模拟渠道已经停用了一段时间。
// ageChannelDisabledAt moves the channel's disabled_at back to simulate it having been disabled for a while.
func ageChannelDisabledAt(t *testing.T, f *syncAlertFixture, by time.Duration) {
	t.Helper()
	if err := f.repo.db.Model(&NotificationChannel{}).Where("id = ?", f.channel.ID).UpdateColumn("disabled_at", time.Now().UTC().Add(-by)).Error; err != nil {
		t.Fatalf("age channel disabled_at: %v", err)
	}
}
