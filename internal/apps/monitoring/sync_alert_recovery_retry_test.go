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
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

func TestSyncRecoveryRetryDelay_Backoff(t *testing.T) {
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	for i, expected := range want {
		if got := syncRecoveryRetryDelay(i + 1); got != expected {
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
	delivery.AttemptCount = syncRecoveryRetryMaxAttempts
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
