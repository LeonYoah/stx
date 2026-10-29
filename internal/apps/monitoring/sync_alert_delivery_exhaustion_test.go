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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

func policyOfTask(t *testing.T, f *syncAlertFixture, taskID uint) *AlertPolicy {
	t.Helper()
	policy, err := f.repo.GetSyncJobAlertPolicyByTaskID(f.ctx, taskID)
	if err != nil || policy == nil {
		t.Fatalf("policy of task %d: %v", taskID, err)
	}
	return policy
}

// #85 复审 2：冷却 60 分钟，首次告警退避重试用完（约 31 分钟）后告警仍在触发 → 渠道恢复后仍能收到告警，之后恢复通知照常发出。
// #85 复审 4：重试用完显示为 exhausted（重试已用完），失败期间策略最近执行状态是 failed，不是 matched。
// #85 re-review 2: cooldown 60, the first alert's backoff retries are used up (~31 minutes) while the alert keeps
// firing -> once the channel recovers a notification is still received, and the recovery then goes out.
// #85 re-review 4: used-up retries show as exhausted, and the policy's last execution status is failed (not matched)
// while sending fails.
func TestSyncAlertHub_FirstAlertKeepsRetryingAfterExhaustionWhileFiring(t *testing.T) {
	f, capture := setupCapturedTaskAlert(t, 60)
	capture.set(http.StatusBadGateway, `bad gateway`)
	_ = f.hub.handle(f.ctx, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
	sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)
	policyID := policyOfTask(t, f, 7).ID

	// 用完 1/2/4/8/16 分钟的退避：共 6 次尝试。
	// Use up the 1/2/4/8/16 minute backoff: 6 attempts in total.
	for attempt := 2; attempt <= syncDeliveryRetryMaxAttempts; attempt++ {
		firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring)
		ageDelivery(t, f, firing.ID, syncDeliveryRetryDelay(firing.AttemptCount)+time.Second)
		_ = f.hub.sweep(f.ctx)
		if capture.count() != attempt {
			t.Fatalf("attempt %d expected, got %d requests", attempt, capture.count())
		}
		policy, err := f.repo.GetAlertPolicyByID(f.ctx, policyID)
		if err != nil {
			t.Fatal(err)
		}
		if policy.LastExecutionStatus != AlertPolicyExecutionStatusFailed {
			t.Fatalf("attempt %d: the policy must show failed while the first alert is undelivered, got %s", attempt, policy.LastExecutionStatus)
		}
	}
	firing := deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring)
	if firing.Status != string(NotificationDeliveryStatusExhausted) || firing.AttemptCount != syncDeliveryRetryMaxAttempts {
		t.Fatalf("first alert should be exhausted after %d attempts: %+v", syncDeliveryRetryMaxAttempts, firing)
	}
	policy, _ := f.repo.GetAlertPolicyByID(f.ctx, policyID)
	if policy.LastExecutionStatus != AlertPolicyExecutionStatusFailed || !strings.Contains(policy.LastExecutionError, "retries exhausted") {
		t.Fatalf("the policy must show the exhaustion: %s %q", policy.LastExecutionStatus, policy.LastExecutionError)
	}
	// 冷却提醒巡检照常跑，也不能把策略状态改回 matched。
	// The cooldown reminder sweep keeps running and must not flip the policy back to matched either.
	_ = f.hub.sweep(f.ctx)
	if policy, _ = f.repo.GetAlertPolicyByID(f.ctx, policyID); policy.LastExecutionStatus != AlertPolicyExecutionStatusFailed {
		t.Fatalf("sweep without a due retry must keep failed, got %s", policy.LastExecutionStatus)
	}

	// 用完之后按 30 分钟上限低频重试：不到 30 分钟不发。
	// After exhaustion it retries at the 30 minute cap: nothing before 30 minutes.
	capture.set(http.StatusOK, `{}`)
	ageDelivery(t, f, firing.ID, 29*time.Minute)
	_ = f.hub.sweep(f.ctx)
	if capture.count() != syncDeliveryRetryMaxAttempts {
		t.Fatalf("must wait the 30 minute cap after exhaustion, got %d requests", capture.count())
	}
	ageDelivery(t, f, firing.ID, 31*time.Minute)
	_ = f.hub.sweep(f.ctx)
	if firing = deliveryOf(t, f, sourceKey, NotificationDeliveryEventTypeFiring); capture.count() != syncDeliveryRetryMaxAttempts+1 ||
		firing.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("the still-firing alert must be delivered once the channel recovers: requests=%d %+v", capture.count(), firing)
	}
	if !containsJSON(capture.lastBody(t), `"status":"firing"`) {
		t.Fatalf("expected the firing payload: %s", capture.lastBody(t))
	}
	if policy, _ = f.repo.GetAlertPolicyByID(f.ctx, policyID); policy.LastExecutionStatus != AlertPolicyExecutionStatusSent {
		t.Fatalf("delivered: the policy should show sent, got %s", policy.LastExecutionStatus)
	}

	if err := resolveTask7(t, f); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if capture.count() != syncDeliveryRetryMaxAttempts+2 || !containsJSON(capture.lastBody(t), `"status":"resolved"`) {
		t.Fatalf("the recovery must follow, got %d requests", capture.count())
	}
}

// #85 复审 4：恢复通知重试用完标记 exhausted（终态），之后不再发送；API 能按 exhausted 筛选。
// #85 re-review 4: a recovery whose retries are used up becomes exhausted (terminal) and is not sent again; the API can
// filter by exhausted.
func TestSyncAlertHub_RecoveryRetryExhaustedStatus(t *testing.T) {
	f, capture, sourceKey := setupFailingRecoveryFixture(t)
	delivery := recoveryDelivery(t, f, sourceKey)
	delivery.AttemptCount = syncDeliveryRetryMaxAttempts - 1
	if err := f.repo.SaveNotificationDelivery(f.ctx, delivery); err != nil {
		t.Fatal(err)
	}
	ageDelivery(t, f, delivery.ID, time.Hour)
	requests := capture.count()
	_ = f.hub.sweep(f.ctx)
	if delivery = recoveryDelivery(t, f, sourceKey); capture.count() != requests+1 || delivery.Status != string(NotificationDeliveryStatusExhausted) ||
		!strings.Contains(delivery.LastError, "502") {
		t.Fatalf("the last recovery attempt should mark exhausted and keep the error: %+v", delivery)
	}
	capture.set(http.StatusOK, `{}`)
	ageDelivery(t, f, delivery.ID, 2*time.Hour)
	_ = f.hub.sweep(f.ctx)
	if capture.count() != requests+1 {
		t.Fatalf("an exhausted recovery must not be resent")
	}
	list, err := f.service.ListNotificationDeliveries(f.ctx, &NotificationDeliveryFilter{Status: NotificationDeliveryStatusExhausted, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("filter by exhausted: %v", err)
	}
	if list == nil || list.Total != 1 {
		t.Fatalf("expected one exhausted delivery, got %+v", list)
	}
}

// #85 复审 3：一个渠道在本轮巡检里失败一次后，跳过它本轮剩下的投递；卡住的渠道每次发送最多等 syncAlertSweepSendTimeout，
// 一轮巡检不会被拖长；预算用完后不再发起新的发送、也不覆盖策略状态。
// #85 re-review 3: once a channel fails in a sweep its remaining deliveries are skipped for that sweep; a hung channel
// waits at most syncAlertSweepSendTimeout per send so the sweep stays short; after the budget nothing new is sent and the
// policy status is not overwritten.
func TestSyncAlertHub_SweepBudgetSkipsFailedChannel(t *testing.T) {
	var hits atomic.Int32
	var hang atomic.Bool
	release := make(chan struct{})
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if hang.Load() {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(func() {
		close(release)
		hung.Close()
	})

	previousTimeout := syncAlertSweepSendTimeout
	syncAlertSweepSendTimeout = 300 * time.Millisecond
	t.Cleanup(func() { syncAlertSweepSendTimeout = previousTimeout })

	f := setupSyncAlertFixture(t)
	f.channel.Endpoint = hung.URL + "/hook"
	if err := f.repo.SaveNotificationChannel(f.ctx, f.channel); err != nil {
		t.Fatal(err)
	}
	const tasks = 5
	for taskID := uint(1); taskID <= tasks; taskID++ {
		f.configureTask(t, taskID, []string{syncalert.RuleJobFailed}, 0)
		_ = f.hub.handle(f.ctx, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, taskID, 100+taskID))
	}
	if hits.Load() != tasks {
		t.Fatalf("expected %d failed first alerts, got %d", tasks, hits.Load())
	}
	var rows []*NotificationDelivery
	if err := f.repo.db.Where("event_type = ?", string(NotificationDeliveryEventTypeFiring)).Find(&rows).Error; err != nil || len(rows) != tasks {
		t.Fatalf("expected %d firing rows: %v", tasks, err)
	}
	for _, row := range rows {
		ageDelivery(t, f, row.ID, time.Hour)
	}

	// 渠道卡住：本轮只发一次（超时后跳过其余 4 条），整轮很快结束；被跳过的记录不消耗次数。
	// The channel hangs: one send this sweep (the other 4 are skipped after the timeout) and the sweep ends quickly;
	// skipped rows use no attempt.
	hang.Store(true)
	started := time.Now()
	_ = f.hub.sweep(f.ctx)
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("a hung channel must not stretch the sweep, took %s", elapsed)
	}
	if hits.Load() != tasks+1 {
		t.Fatalf("only one send to the failing channel per sweep, got %d", hits.Load()-tasks)
	}
	attempted := 0
	for _, row := range rows {
		var current NotificationDelivery
		if err := f.repo.db.First(&current, row.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.Status == string(NotificationDeliveryStatusSending) {
			t.Fatalf("a timed-out send must not leave the row in sending: %+v", current)
		}
		if current.AttemptCount == 2 {
			attempted++
		}
	}
	if attempted != 1 {
		t.Fatalf("exactly one row should have used an attempt, got %d", attempted)
	}

	// 预算用完：不再发起任何发送，也不改策略的最近执行状态。
	// Budget used up: no send is started and the policy's last execution status is kept.
	hang.Store(false)
	previousBudget := syncAlertSweepBudget
	syncAlertSweepBudget = 0
	t.Cleanup(func() { syncAlertSweepBudget = previousBudget })
	for _, row := range rows {
		ageDelivery(t, f, row.ID, time.Hour)
	}
	before := hits.Load()
	policy := policyOfTask(t, f, 1)
	if err := f.repo.db.Model(&AlertPolicy{}).Where("id = ?", policy.ID).UpdateColumn("cooldown_minutes", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.repo.ApplyAlertPolicyExecutionState(f.ctx, policy.ID, &AlertPolicyExecutionStateUpdate{
		LastExecutionStatus: AlertPolicyExecutionStatusFailed, LastExecutionError: "bad gateway",
	}); err != nil {
		t.Fatal(err)
	}
	_ = f.hub.sweep(f.ctx)
	if hits.Load() != before {
		t.Fatalf("no send after the budget, got %d", hits.Load()-before)
	}
	if policy, _ = f.repo.GetAlertPolicyByID(f.ctx, policy.ID); policy.LastExecutionStatus != AlertPolicyExecutionStatusFailed {
		t.Fatalf("a deferred sweep must keep the policy status, got %s", policy.LastExecutionStatus)
	}
}
