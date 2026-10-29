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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// switchableServer 是一个可以切换"卡住 / 502 / 200"的 webhook 服务。
// switchableServer is a webhook server that can switch between hanging, 502 and 200.
type switchableServer struct {
	server *httptest.Server
	hits   atomic.Int32
	mode   atomic.Int32 // 0 = 502, 1 = 200, 2 = hang
	body   atomic.Value // 最近一次请求体 / latest request body (string)
}

func (s *switchableServer) lastBody() string {
	body, _ := s.body.Load().(string)
	return body
}

func newSwitchableServer(t *testing.T) *switchableServer {
	t.Helper()
	s := &switchableServer{}
	release := make(chan struct{})
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.body.Store(string(raw))
		s.hits.Add(1)
		switch s.mode.Load() {
		case 1:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case 2:
			select {
			case <-release:
			case <-r.Context().Done():
			}
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	t.Cleanup(func() {
		close(release)
		s.server.Close()
	})
	return s
}

func (f *syncAlertFixture) addWebhookChannel(t *testing.T, name string, server *switchableServer) *NotificationChannel {
	t.Helper()
	channel := &NotificationChannel{Name: name, Type: NotificationChannelTypeWebhook, Enabled: true, Endpoint: server.server.URL + "/hook"}
	if err := f.repo.CreateNotificationChannel(f.ctx, channel); err != nil {
		t.Fatal(err)
	}
	return channel
}

func (f *syncAlertFixture) fireTaskOn(t *testing.T, taskID uint, cooldown int, channelIDs ...uint) {
	t.Helper()
	if _, err := f.service.SaveTaskAlertConfig(f.ctx, syncalert.TaskRef{ID: taskID, Name: fmt.Sprintf("task-%d", taskID), ClusterID: f.clusterID, CreatorID: 1}, &syncalert.TaskConfig{
		Enabled: true, Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: channelIDs, CooldownMinutes: cooldown, SendRecovery: true,
	}); err != nil {
		t.Fatalf("configure task %d: %v", taskID, err)
	}
	_ = f.hub.handle(f.ctx, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, taskID, 1000+taskID))
}

func (f *syncAlertFixture) ageAllFiring(t *testing.T, by time.Duration) {
	t.Helper()
	if err := f.repo.db.Model(&NotificationDelivery{}).Where("event_type = ? AND status <> ?", string(NotificationDeliveryEventTypeFiring), string(NotificationDeliveryStatusSent)).
		UpdateColumn("updated_at", time.Now().UTC().Add(-by)).Error; err != nil {
		t.Fatal(err)
	}
}

func shortSweepBudget(t *testing.T, budget, sendTimeout time.Duration) {
	t.Helper()
	prevBudget, prevTimeout := syncAlertSweepBudget, syncAlertSweepSendTimeout
	syncAlertSweepBudget, syncAlertSweepSendTimeout = budget, sendTimeout
	t.Cleanup(func() { syncAlertSweepBudget, syncAlertSweepSendTimeout = prevBudget, prevTimeout })
}

// #85 复审 P2：巡检预算不能饿死排在后面的健康渠道。重试按渠道轮转、健康渠道优先、可疑渠道按最近失败轮换。
// #85 review P2: the sweep budget must not starve a healthy channel that sorts late. Retries round-robin by channel,
// healthy channels first, suspect ones rotated by least recent failure.
func TestSyncAlertHub_RetryRoundRobinDoesNotStarveLateChannel(t *testing.T) {
	t.Run("one-hung-channel-many-rows", func(t *testing.T) {
		shortSweepBudget(t, 500*time.Millisecond, 300*time.Millisecond)
		f := setupSyncAlertFixture(t)
		hung, healthy := newSwitchableServer(t), newSwitchableServer(t)
		a, c := f.addWebhookChannel(t, "A", hung), f.addWebhookChannel(t, "C", healthy)
		for taskID := uint(1); taskID <= 30; taskID++ {
			f.fireTaskOn(t, taskID, 0, a.ID)
		}
		f.fireTaskOn(t, 31, 0, c.ID) // C 的记录排在最后 / C's row sorts last
		hung.mode.Store(2)
		healthy.mode.Store(1)
		f.ageAllFiring(t, time.Hour)
		_ = f.hub.sweep(f.ctx)
		if healthy.hits.Load() != 2 {
			t.Fatalf("the healthy channel must be served in the first sweep, got %d retries", healthy.hits.Load()-1)
		}
		if hung.hits.Load() != 31 {
			t.Fatalf("the hung channel gets one try per sweep, got %d", hung.hits.Load()-30)
		}
	})

	t.Run("two-hung-channels-rotate", func(t *testing.T) {
		shortSweepBudget(t, 500*time.Millisecond, 300*time.Millisecond)
		f := setupSyncAlertFixture(t)
		hungA, hungB, healthy := newSwitchableServer(t), newSwitchableServer(t), newSwitchableServer(t)
		a, b, c := f.addWebhookChannel(t, "A", hungA), f.addWebhookChannel(t, "B", hungB), f.addWebhookChannel(t, "C", healthy)
		for taskID := uint(1); taskID <= 10; taskID++ {
			f.fireTaskOn(t, taskID, 0, a.ID)
			f.fireTaskOn(t, 100+taskID, 0, b.ID)
		}
		f.fireTaskOn(t, 200, 0, c.ID)
		hungA.mode.Store(2)
		hungB.mode.Store(2)
		healthy.mode.Store(1)
		served := 0
		for sweep := 1; sweep <= 2 && served == 0; sweep++ {
			f.ageAllFiring(t, time.Hour)
			_ = f.hub.sweep(f.ctx)
			if healthy.hits.Load() > 1 {
				served = sweep
			}
		}
		if served == 0 {
			t.Fatalf("the channel behind two hung channels must be served within 2 sweeps")
		}
	})

	t.Run("healthy-channel-before-suspect", func(t *testing.T) {
		shortSweepBudget(t, 500*time.Millisecond, 300*time.Millisecond)
		f := setupSyncAlertFixture(t)
		hungA, hungB, healthy := newSwitchableServer(t), newSwitchableServer(t), newSwitchableServer(t)
		a, b, c := f.addWebhookChannel(t, "A", hungA), f.addWebhookChannel(t, "B", hungB), f.addWebhookChannel(t, "C", healthy)
		for taskID := uint(1); taskID <= 10; taskID++ {
			f.fireTaskOn(t, taskID, 0, a.ID)
			f.fireTaskOn(t, 100+taskID, 0, b.ID)
		}
		f.fireTaskOn(t, 200, 0, c.ID)
		// C 之后发送成功过（另一条告警送达）：C 是健康渠道，排在两个可疑渠道前面。
		// C delivered something since (another alert went out): C is healthy and goes before both suspect channels.
		healthy.mode.Store(1)
		f.fireTaskOn(t, 201, 0, c.ID)
		hungA.mode.Store(2)
		hungB.mode.Store(2)
		f.ageAllFiring(t, time.Hour)
		_ = f.hub.sweep(f.ctx)
		if healthy.hits.Load() != 3 {
			t.Fatalf("the healthy channel must be served in the first sweep, got %d hits", healthy.hits.Load())
		}
	})
}

// #85 复审 P3：一个渠道送达、另一个渠道的首次告警重试失败时，策略状态保持 partial，不被重试改成 failed；
// 失败渠道之后送达则变成 sent。
// #85 review P3: with one channel delivered and the other channel's first alert retry failing, the policy status stays
// partial instead of being overwritten to failed; once the failing channel delivers it becomes sent.
func TestSyncAlertHub_RetryKeepsPartialPolicyStatus(t *testing.T) {
	f := setupSyncAlertFixture(t)
	good, bad := newSwitchableServer(t), newSwitchableServer(t)
	good.mode.Store(1)
	goodCh, badCh := f.addWebhookChannel(t, "good", good), f.addWebhookChannel(t, "bad", bad)
	f.fireTaskOn(t, 7, 60, goodCh.ID, badCh.ID)
	policyID := policyOfTask(t, f, 7).ID
	status := func() AlertPolicyExecutionStatus {
		policy, err := f.repo.GetAlertPolicyByID(f.ctx, policyID)
		if err != nil {
			t.Fatal(err)
		}
		return policy.LastExecutionStatus
	}
	if got := status(); got != AlertPolicyExecutionStatusPartial {
		t.Fatalf("after the first send: want partial, got %s", got)
	}
	f.ageAllFiring(t, time.Hour)
	_ = f.hub.sweep(f.ctx)
	if bad.hits.Load() != 2 {
		t.Fatalf("the failing channel should have been retried, got %d hits", bad.hits.Load())
	}
	if got := status(); got != AlertPolicyExecutionStatusPartial {
		t.Fatalf("a failing retry on one of two channels must keep partial, got %s", got)
	}
	bad.mode.Store(1)
	f.ageAllFiring(t, time.Hour)
	_ = f.hub.sweep(f.ctx)
	if got := status(); got != AlertPolicyExecutionStatusSent {
		t.Fatalf("all channels delivered: want sent, got %s", got)
	}
}

// exhaustFirstAlert 让任务 7 的首次告警用完退避重试，返回它的 source key。
// exhaustFirstAlert uses up task 7's first alert backoff retries and returns its source key.
func exhaustFirstAlert(t *testing.T, f *syncAlertFixture, server *switchableServer, channel *NotificationChannel) string {
	t.Helper()
	f.fireTaskOn(t, 7, 60, channel.ID)
	sourceKey := buildSyncAlertSourceKey(f.episodes(t)[0].ID)
	for attempt := 2; attempt <= syncDeliveryRetryMaxAttempts; attempt++ {
		f.ageAllFiring(t, syncDeliveryRetryMaxDelay)
		_ = f.hub.sweep(f.ctx)
	}
	firing, err := f.repo.GetNotificationDeliveryByDedupKey(f.ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
	if err != nil || firing == nil || firing.Status != string(NotificationDeliveryStatusExhausted) || server.hits.Load() != int32(syncDeliveryRetryMaxAttempts) {
		t.Fatalf("setup: first alert should be exhausted after %d attempts: %+v hits=%d", syncDeliveryRetryMaxAttempts, firing, server.hits.Load())
	}
	// 刚刚失败过一次：距离下一次 30 分钟重试还早。
	// It just failed again: the next 30 minute retry is far away.
	return sourceKey
}

// #85 复审 P3：首次告警用完重试后，渠道恢复（重新保存 / 别的告警发送成功）时下一轮巡检立即补发，不等 30 分钟；
// 如果告警在这段时间里就恢复了，用户也会收到一条带"首次告警未送达"说明的恢复通知。
// #85 review P3: after a first alert's retries are used up, once the channel recovers (saved again / another alert
// delivered) the next sweep resends right away instead of waiting 30 minutes; and if the alert recovers within that
// window the user still gets one recovery noting the first alert was not delivered.
func TestSyncAlertHub_ExhaustedFirstAlertAfterChannelRecovers(t *testing.T) {
	t.Run("channel-saved-retries-on-next-sweep", func(t *testing.T) {
		f := setupSyncAlertFixture(t)
		server := newSwitchableServer(t)
		channel := f.addWebhookChannel(t, "ops", server)
		sourceKey := exhaustFirstAlert(t, f, server, channel)
		server.mode.Store(1)
		enabled := true
		if _, err := f.service.UpdateNotificationChannel(f.ctx, channel.ID, &UpsertNotificationChannelRequest{
			Name: channel.Name, Type: channel.Type, Enabled: &enabled, Endpoint: channel.Endpoint,
		}); err != nil {
			t.Fatalf("save channel: %v", err)
		}
		_ = f.hub.sweep(f.ctx)
		firing, _ := f.repo.GetNotificationDeliveryByDedupKey(f.ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
		if server.hits.Load() != int32(syncDeliveryRetryMaxAttempts)+1 || firing.Status != string(NotificationDeliveryStatusSent) {
			t.Fatalf("a re-saved channel must get the exhausted first alert on the next sweep: hits=%d %+v", server.hits.Load(), firing)
		}
	})

	t.Run("another-alert-delivered-retries-on-next-sweep", func(t *testing.T) {
		f := setupSyncAlertFixture(t)
		server := newSwitchableServer(t)
		channel := f.addWebhookChannel(t, "ops", server)
		sourceKey := exhaustFirstAlert(t, f, server, channel)
		server.mode.Store(1)
		f.fireTaskOn(t, 8, 0, channel.ID) // 另一条告警送达 / another alert is delivered
		hits := server.hits.Load()
		_ = f.hub.sweep(f.ctx)
		firing, _ := f.repo.GetNotificationDeliveryByDedupKey(f.ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
		if server.hits.Load() != hits+1 || firing.Status != string(NotificationDeliveryStatusSent) {
			t.Fatalf("after a successful send the exhausted first alert goes out on the next sweep: hits=%d %+v", server.hits.Load()-hits, firing)
		}
		// 之后再失败就重新按退避来，不会每轮都重发。
		// Later failures follow the backoff again instead of resending every sweep.
		_ = f.hub.sweep(f.ctx)
		if server.hits.Load() != hits+1 {
			t.Fatalf("nothing more to send, got %d", server.hits.Load()-hits)
		}
	})

	t.Run("alert-recovers-before-cap", func(t *testing.T) {
		f := setupSyncAlertFixture(t)
		server := newSwitchableServer(t)
		channel := f.addWebhookChannel(t, "ops", server)
		sourceKey := exhaustFirstAlert(t, f, server, channel)
		server.mode.Store(1)
		// 渠道恢复了，但没有任何信号告诉我们；告警在 30 分钟重试之前就恢复。
		// The channel is back but nothing tells us; the alert recovers before the 30 minute retry.
		hits := server.hits.Load()
		if err := resolveTask7(t, f); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if server.hits.Load() != hits+1 || !strings.Contains(server.lastBody(), `"status":"resolved"`) ||
			!strings.Contains(server.lastBody(), `"first_alert_delivered":false`) || !strings.Contains(server.lastBody(), "此前的告警通知未能送达") {
			t.Fatalf("the user must receive one recovery noting the undelivered alert: hits=%d body=%s", server.hits.Load()-hits, server.lastBody())
		}
		_ = f.hub.sweep(f.ctx)
		f.ageAllFiring(t, time.Hour)
		_ = f.hub.sweep(f.ctx)
		if server.hits.Load() != hits+1 {
			t.Fatalf("no stale first alert after the recovery, got %d more", server.hits.Load()-hits-1)
		}
		firing, _ := f.repo.GetNotificationDeliveryByDedupKey(f.ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
		if firing.Status != string(NotificationDeliveryStatusCanceled) {
			t.Fatalf("the first alert retry is canceled once the alert recovered: %+v", firing)
		}
	})
}
