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
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// webhookRecorder 记录 webhook 收到的请求体。
// webhookRecorder records the bodies a webhook receives.
type webhookRecorder struct {
	mu     gosync.Mutex
	bodies []map[string]any
	server *httptest.Server
}

func newWebhookRecorder(t *testing.T) *webhookRecorder {
	t.Helper()
	recorder := &webhookRecorder{}
	recorder.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		recorder.mu.Lock()
		recorder.bodies = append(recorder.bodies, body)
		recorder.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(recorder.server.Close)
	return recorder
}

func (r *webhookRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.bodies)
}

func (r *webhookRecorder) last() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		return nil
	}
	return r.bodies[len(r.bodies)-1]
}

type syncAlertFixture struct {
	service   *Service
	repo      *Repository
	hub       *SyncAlertHub
	ctx       context.Context
	clusterID uint
	channel   *NotificationChannel
	webhook   *webhookRecorder
	now       time.Time
}

func setupSyncAlertFixture(t *testing.T) *syncAlertFixture {
	t.Helper()
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)
	webhook := newWebhookRecorder(t)
	channel := &NotificationChannel{Name: "ops-webhook", Type: NotificationChannelTypeWebhook, Enabled: true, Endpoint: webhook.server.URL}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	fixture := &syncAlertFixture{
		service:   service,
		repo:      repo,
		hub:       service.NewSyncAlertHub(),
		ctx:       ctx,
		clusterID: cluster.ID,
		channel:   channel,
		webhook:   webhook,
		now:       time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC),
	}
	fixture.hub.now = func() time.Time { return fixture.now }
	return fixture
}

func (f *syncAlertFixture) configureTask(t *testing.T, taskID uint, rules []string, cooldown int) *syncalert.TaskConfig {
	t.Helper()
	cfg, err := f.service.SaveTaskAlertConfig(f.ctx, syncalert.TaskRef{ID: taskID, Name: "orders-sync", ClusterID: f.clusterID, CreatorID: 1}, &syncalert.TaskConfig{
		Enabled:         true,
		Rules:           rules,
		ChannelIDs:      []uint{f.channel.ID},
		CooldownMinutes: cooldown,
		SendRecovery:    true,
	})
	if err != nil {
		t.Fatalf("save task alert config: %v", err)
	}
	return cfg
}

func (f *syncAlertFixture) signal(kind syncalert.SignalKind, rule string, taskID, jobID uint) syncalert.Signal {
	return syncalert.Signal{
		Kind:          kind,
		RuleKey:       rule,
		ClusterID:     f.clusterID,
		ClusterName:   "policy-bridge-cluster",
		TaskID:        taskID,
		TaskName:      "orders-sync",
		TaskCreatorID: 1,
		JobID:         jobID,
		EngineJobID:   "engine-9",
		TriggerSource: "schedule",
		Message:       "job failed: connection refused",
		OccurredAt:    f.now,
	}
}

func (f *syncAlertFixture) handle(t *testing.T, signal syncalert.Signal) {
	t.Helper()
	if err := f.hub.handle(f.ctx, signal); err != nil {
		t.Fatalf("handle %s/%s: %v", signal.Kind, signal.RuleKey, err)
	}
}

func (f *syncAlertFixture) episodes(t *testing.T) []*SyncAlertEpisode {
	t.Helper()
	episodes, err := f.repo.ListSyncAlertEpisodes(f.ctx, &SyncAlertEpisodeFilter{})
	if err != nil {
		t.Fatalf("list episodes: %v", err)
	}
	return episodes
}

func withExternalURL(t *testing.T, value string) {
	t.Helper()
	if config.Config == nil {
		t.Skip("config not loaded")
	}
	previous := config.Config.App.ExternalURL
	config.Config.App.ExternalURL = value
	t.Cleanup(func() { config.Config.App.ExternalURL = previous })
}

func TestSyncAlertHub_taskWithoutPolicyIsDropped(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
	if got := f.episodes(t); len(got) != 0 {
		t.Fatalf("no alert config means no episode, got %d", len(got))
	}
	if f.webhook.count() != 0 {
		t.Fatalf("no alert config means no notification")
	}

	// 只勾选了提交失败：作业失败同样丢弃。
	// Only submit-failed is selected: a job failure is dropped as well.
	f.configureTask(t, 7, []string{syncalert.RuleSubmitFailed}, 60)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 71))
	if got := f.episodes(t); len(got) != 0 {
		t.Fatalf("unselected rule must not open an episode, got %d", len(got))
	}
}

func TestSyncAlertHub_firesOnceCountsAndResolves(t *testing.T) {
	withExternalURL(t, "https://stx.example.com/")
	f := setupSyncAlertFixture(t)
	f.configureTask(t, 7, []string{syncalert.RuleJobFailed}, 60)

	for i := 0; i < 3; i++ {
		f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, uint(70+i)))
		f.now = f.now.Add(time.Minute)
	}
	if f.webhook.count() != 1 {
		t.Fatalf("repeated failures notify once, got %d", f.webhook.count())
	}
	body := f.webhook.last()
	alert, _ := body["alert"].(map[string]any)
	if alert["task_name"] != "orders-sync" || alert["job_id"] != float64(70) || alert["link"] != "https://stx.example.com/sync?job=70&task=7" {
		t.Fatalf("firing payload missing task/job/link: %#v", alert)
	}
	if !strings.Contains(body["title"].(string), "任务 orders-sync · 作业运行失败") {
		t.Fatalf("unexpected title: %v", body["title"])
	}
	episodes := f.episodes(t)
	if len(episodes) != 1 || episodes[0].OccurrenceCount != 3 || episodes[0].LastJobID != 72 {
		t.Fatalf("expected one episode counting 3, got %+v", episodes)
	}

	// 告警中心能看到这一条，带任务、作业和次数。
	// The alert center shows it with task, job and count.
	list, err := f.service.ListAlertInstances(f.ctx, &AlertInstanceFilter{SourceType: AlertSourceTypeSync})
	if err != nil || len(list.Alerts) != 1 {
		t.Fatalf("alert center: %+v err=%v", list, err)
	}
	item := list.Alerts[0]
	if item.Status != AlertDisplayStatusFiring || item.SourceRef.OccurrenceCount != 3 || item.SourceRef.JobID != 72 || item.SourceRef.Link != "/sync?task=7&job=72" {
		t.Fatalf("unexpected alert item: %+v %+v", item, item.SourceRef)
	}

	resolve := f.signal(syncalert.KindResolve, "", 7, 73)
	resolve.ResolveScope = syncalert.ResolveTaskRunHealthy
	f.handle(t, resolve)
	if f.webhook.count() != 2 || f.webhook.last()["alert"].(map[string]any)["status"] != "resolved" {
		t.Fatalf("expected a recovery notification, got %d", f.webhook.count())
	}
	if got := f.episodes(t)[0]; got.Status != SyncAlertEpisodeResolved || got.ResolveReason != syncAlertResolveRecovered {
		t.Fatalf("episode should be resolved: %+v", got)
	}

	// 恢复之后再失败，开新阶段并重新通知。
	// A failure after recovery opens a new episode and notifies again.
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 74))
	if f.webhook.count() != 3 || len(f.episodes(t)) != 2 {
		t.Fatalf("new failure should open a second episode: hits=%d episodes=%d", f.webhook.count(), len(f.episodes(t)))
	}
}

func TestSyncAlertHub_reminderHonorsCooldownAndSilence(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.configureTask(t, 7, []string{syncalert.RuleJobFailed}, 1)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, 70))
	if f.webhook.count() != 1 {
		t.Fatalf("first failure should notify")
	}
	episode := f.episodes(t)[0]
	sourceKey := buildSyncAlertSourceKey(episode.ID)

	if err := f.hub.sweep(f.ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if f.webhook.count() != 1 {
		t.Fatalf("no reminder before the cooldown, got %d", f.webhook.count())
	}

	// 把上次发送时间往前挪 2 分钟，模拟冷却已过。
	// Move the last send back two minutes to simulate the cooldown passing.
	backdateDelivery(t, f, sourceKey, 2*time.Minute)
	if err := f.hub.sweep(f.ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if f.webhook.count() != 2 {
		t.Fatalf("expected one reminder after the cooldown, got %d", f.webhook.count())
	}

	if _, err := f.service.SilenceAlertInstance(f.ctx, sourceKey, "ops", 30, "maintenance"); err != nil {
		t.Fatalf("silence: %v", err)
	}
	backdateDelivery(t, f, sourceKey, 2*time.Minute)
	if err := f.hub.sweep(f.ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if f.webhook.count() != 2 {
		t.Fatalf("silenced episodes must not remind, got %d", f.webhook.count())
	}
}

func backdateDelivery(t *testing.T, f *syncAlertFixture, sourceKey string, by time.Duration) {
	t.Helper()
	delivery, err := f.repo.GetNotificationDeliveryByDedupKey(f.ctx, sourceKey, f.channel.ID, string(NotificationDeliveryEventTypeFiring))
	if err != nil || delivery == nil {
		t.Fatalf("delivery not found: %v", err)
	}
	past := time.Now().UTC().Add(-by)
	delivery.SentAt = &past
	if err := f.repo.SaveNotificationDelivery(f.ctx, delivery); err != nil {
		t.Fatalf("backdate delivery: %v", err)
	}
}

func TestSyncAlertHub_closedEpisodeStartsOver(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.configureTask(t, 7, []string{syncalert.RuleSubmitFailed}, 60)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleSubmitFailed, 7, 0))
	first := f.episodes(t)[0]
	if _, err := f.service.CloseAlertInstance(f.ctx, buildSyncAlertSourceKey(first.ID), "ops", "known issue"); err != nil {
		t.Fatalf("close: %v", err)
	}
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleSubmitFailed, 7, 0))
	episodes := f.episodes(t)
	if len(episodes) != 2 || episodes[1].ResolveReason != syncAlertResolveClosed || episodes[0].Status != SyncAlertEpisodeFiring {
		t.Fatalf("closing must end the episode and the next failure opens a new one: %+v", episodes)
	}
	if f.webhook.count() != 2 {
		t.Fatalf("the new episode notifies again, got %d", f.webhook.count())
	}
}

func TestSyncAlertHub_stopTimeoutResolvesOnlyItsJob(t *testing.T) {
	f := setupSyncAlertFixture(t)
	f.configureTask(t, 7, []string{syncalert.RuleStopTimeout}, 60)
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleStopTimeout, 7, 80))

	other := f.signal(syncalert.KindResolve, "", 7, 81)
	other.ResolveScope = syncalert.ResolveJobTerminal
	f.handle(t, other)
	if f.episodes(t)[0].Status != SyncAlertEpisodeFiring {
		t.Fatalf("another job stopping must not resolve the episode")
	}
	same := f.signal(syncalert.KindResolve, "", 7, 80)
	same.ResolveScope = syncalert.ResolveJobTerminal
	f.handle(t, same)
	if f.episodes(t)[0].Status != SyncAlertEpisodeResolved {
		t.Fatalf("the stuck job stopping resolves the episode")
	}
}

func TestSyncAlertHub_clusterRules(t *testing.T) {
	f := setupSyncAlertFixture(t)

	// D2 没有策略：建阶段，不发通知。
	// D2 without a policy: an episode but no notification.
	degraded := f.signal(syncalert.KindFire, syncalert.RuleEventChannelDegraded, 0, 0)
	f.handle(t, degraded)
	episodes := f.episodes(t)
	if len(episodes) != 1 || episodes[0].Scope != SyncAlertEpisodeScopeCluster || episodes[0].TaskID != 0 {
		t.Fatalf("expected one cluster episode, got %+v", episodes)
	}
	if f.webhook.count() != 0 {
		t.Fatalf("no policy means no notification")
	}

	// D1：持续时间不到窗口不建阶段；到了窗口建阶段，并按集群策略通知。
	// D1: no episode before the window; once the window passes an episode opens and cluster policies notify.
	if _, err := f.service.CreateAlertPolicy(f.ctx, &UpsertAlertPolicyRequest{
		Name:                   "status unavailable",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            syncalert.RuleStatusUnavailable,
		ClusterID:              itoa(f.clusterID),
		Severity:               AlertSeverityWarning,
		NotificationChannelIDs: []uint{f.channel.ID},
	}); err != nil {
		t.Fatalf("create cluster policy: %v", err)
	}
	observe := f.signal(syncalert.KindObserve, syncalert.RuleStatusUnavailable, 0, 0)
	observe.Since = f.now.Add(-2 * time.Minute)
	f.handle(t, observe)
	if len(f.episodes(t)) != 1 {
		t.Fatalf("two minutes of failure is inside the 300s window")
	}
	observe.Since = f.now.Add(-6 * time.Minute)
	f.handle(t, observe)
	if len(f.episodes(t)) != 2 || f.webhook.count() != 1 {
		t.Fatalf("matured D1 should open an episode and notify: episodes=%d hits=%d", len(f.episodes(t)), f.webhook.count())
	}
	resolve := f.signal(syncalert.KindResolve, syncalert.RuleStatusUnavailable, 0, 0)
	resolve.ResolveScope = syncalert.ResolveClusterRule
	f.handle(t, resolve)
	if f.webhook.count() != 2 {
		t.Fatalf("recovery should notify the cluster policy, got %d", f.webhook.count())
	}

	// 集群规则停用后丢弃。
	// A disabled cluster rule drops the signal.
	rule, err := f.repo.GetRuleByClusterAndKey(f.ctx, f.clusterID, syncalert.RuleEventChannelDegraded)
	if err != nil {
		t.Fatalf("load rule: %v", err)
	}
	rule.Enabled = false
	if err := f.repo.SaveRule(f.ctx, rule); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	clear := f.signal(syncalert.KindResolve, syncalert.RuleEventChannelDegraded, 0, 0)
	clear.ResolveScope = syncalert.ResolveClusterRule
	f.handle(t, clear)
	before := len(f.episodes(t))
	f.handle(t, degraded)
	if len(f.episodes(t)) != before {
		t.Fatalf("disabled rule must not open episodes")
	}
}

func TestSyncAlertHub_reportDoesNotBlockWhenFull(t *testing.T) {
	f := setupSyncAlertFixture(t)
	done := make(chan struct{})
	go func() {
		for i := 0; i < syncAlertQueueSize+5; i++ {
			f.hub.ReportSyncAlert(f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 7, uint(i)))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("ReportSyncAlert blocked on a full buffer")
	}
	if got := f.hub.dropped.Load(); got != 5 {
		t.Fatalf("expected 5 dropped signals, got %d", got)
	}
}

func TestTaskAlertConfigStore(t *testing.T) {
	f := setupSyncAlertFixture(t)
	task := syncalert.TaskRef{ID: 9, Name: "cdc-orders", ClusterID: f.clusterID}

	empty, err := f.service.GetTaskAlertConfig(f.ctx, 9)
	if err != nil || empty.Configured {
		t.Fatalf("unconfigured task: %+v err=%v", empty, err)
	}

	saved, err := f.service.SaveTaskAlertConfig(f.ctx, task, &syncalert.TaskConfig{
		Enabled:    true,
		Rules:      []string{syncalert.RuleStopTimeout, syncalert.RuleJobFailed},
		ChannelIDs: []uint{f.channel.ID},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !saved.Configured || !reflect.DeepEqual(saved.Rules, []string{syncalert.RuleJobFailed, syncalert.RuleStopTimeout}) || saved.CooldownMinutes != 0 {
		t.Fatalf("unexpected saved config: %+v", saved)
	}

	// 再保存是更新，不新建。
	// Saving again updates instead of creating.
	updated, err := f.service.SaveTaskAlertConfig(f.ctx, task, &syncalert.TaskConfig{
		Enabled: false, Rules: []string{syncalert.RuleSubmitFailed}, ChannelIDs: []uint{f.channel.ID}, CooldownMinutes: 30,
	})
	if err != nil || updated.PolicyID != saved.PolicyID || updated.Enabled || updated.CooldownMinutes != 30 {
		t.Fatalf("update: %+v err=%v", updated, err)
	}

	// 通用接口不能给同一任务再建一条。
	// The generic API cannot add a second policy for the same task.
	_, err = f.service.CreateAlertPolicy(f.ctx, taskConfigToPolicyRequest(task, updated))
	if !errors.Is(err, ErrAlertPolicySyncJobConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	// 校验错误包装成 ErrInvalidConfig。
	// Validation errors are wrapped as ErrInvalidConfig.
	_, err = f.service.SaveTaskAlertConfig(f.ctx, task, &syncalert.TaskConfig{Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{999}})
	if !errors.Is(err, syncalert.ErrInvalidConfig) {
		t.Fatalf("missing channel should be invalid config, got %v", err)
	}
	email := &NotificationChannel{Name: "mail", Type: NotificationChannelTypeEmail, Enabled: true, ConfigJSON: `{"email":{"host":"smtp","port":25,"from":"stx@example.com"}}`}
	if err := f.repo.CreateNotificationChannel(f.ctx, email); err != nil {
		t.Fatalf("create email channel: %v", err)
	}
	_, err = f.service.SaveTaskAlertConfig(f.ctx, task, &syncalert.TaskConfig{Enabled: true, Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{email.ID}})
	if !errors.Is(err, syncalert.ErrInvalidConfig) {
		t.Fatalf("email without any recipient should be invalid, got %v", err)
	}
	if _, err := f.service.SaveTaskAlertConfig(f.ctx, task, &syncalert.TaskConfig{Enabled: true, Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{email.ID}, NotifyTaskOwner: true}); err != nil {
		t.Fatalf("email with notify-owner is valid: %v", err)
	}

	// 策略列表按类型和任务筛选，并带出任务名。
	// The policy list filters by type and task and shows the task name.
	f.service.SetSyncTaskNameResolver(staticTaskNames{9: "cdc-orders"})
	list, err := f.service.ListAlertPolicies(f.ctx, &AlertPolicyListFilter{PolicyType: AlertPolicyBuilderKindSyncJob, TaskID: 9})
	if err != nil || len(list.Policies) != 1 || list.Policies[0].TaskName != "cdc-orders" || !list.Policies[0].NotifyTaskOwner {
		t.Fatalf("filtered list: %+v err=%v", list, err)
	}

	// 删除配置：结束告警中的阶段，不再产生新告警。
	// Deleting the config ends firing episodes and stops new alerts.
	if _, err := f.service.SaveTaskAlertConfig(f.ctx, task, &syncalert.TaskConfig{Enabled: true, Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{f.channel.ID}}); err != nil {
		t.Fatalf("switch back to webhook: %v", err)
	}
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 9, 90))
	if err := f.service.DeleteTaskAlertConfig(f.ctx, 9); err != nil {
		t.Fatalf("delete: %v", err)
	}
	episodes := f.episodes(t)
	if len(episodes) != 1 || episodes[0].Status != SyncAlertEpisodeResolved || episodes[0].ResolveReason != syncAlertResolvePolicyDeleted {
		t.Fatalf("delete should close the episode: %+v", episodes)
	}
	f.handle(t, f.signal(syncalert.KindFire, syncalert.RuleJobFailed, 9, 91))
	if len(f.episodes(t)) != 1 {
		t.Fatalf("no new episodes after delete")
	}
}

func TestSyncAlertOwnerRecipients(t *testing.T) {
	f := setupSyncAlertFixture(t)
	users := []*auth.User{
		{Username: "owner", Email: "owner@example.com", IsActive: true},
		{Username: "no-mail", Email: "", IsActive: true},
	}
	for _, user := range users {
		if err := f.repo.db.Create(user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	if got := f.service.resolveUserEmails(f.ctx, uint(users[0].ID)); !reflect.DeepEqual(got, []string{"owner@example.com"}) {
		t.Fatalf("owner email: %v", got)
	}
	if got := f.service.resolveUserEmails(f.ctx, uint(users[1].ID)); len(got) != 0 {
		t.Fatalf("user without email yields nothing: %v", got)
	}
	merged := mergeAlertRecipients([]string{"ops@example.com", "Owner@example.com"}, []string{"owner@example.com"})
	if !reflect.DeepEqual(merged, []string{"ops@example.com", "Owner@example.com"}) {
		t.Fatalf("merge must de-duplicate case-insensitively: %v", merged)
	}

	episode := &SyncAlertEpisode{ID: 1, Scope: SyncAlertEpisodeScopeTask, TaskID: 7, TaskName: "orders", RuleKey: syncalert.RuleJobFailed, Severity: AlertSeverityCritical, OccurrenceCount: 1}
	message := buildSyncAlertMessage(episode, &AlertPolicy{Name: "p"}, NotificationDeliveryEventTypeFiring, []string{"owner@example.com"}, f.now)
	payload, err := message.Build(&NotificationChannel{Type: NotificationChannelTypeEmail}, mergeAlertRecipients(nil, message.ExtraRecipients))
	if err != nil {
		t.Fatalf("build email: %v", err)
	}
	mail := payload.(*emailNotificationPayload)
	if !reflect.DeepEqual(mail.To, []string{"owner@example.com"}) || !strings.Contains(mail.HTML, "orders") || !strings.Contains(mail.Subject, "作业运行失败") {
		t.Fatalf("unexpected email payload: %+v", mail)
	}
}

type staticTaskNames map[uint]string

func (s staticTaskNames) ResolveTaskNames(_ context.Context, ids []uint) map[uint]string {
	result := map[uint]string{}
	for _, id := range ids {
		if name, ok := s[id]; ok {
			result[id] = name
		}
	}
	return result
}

func itoa(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
