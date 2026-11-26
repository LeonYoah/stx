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

package sync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/gin-gonic/gin"
)

// recordingEventApplier 记录收到的 seatunnel.yaml 写入请求。
// recordingEventApplier records seatunnel.yaml write requests.
type recordingEventApplier struct {
	calls    []*EngineEventReportSettings
	clusters []uint
	err      error
}

func (r *recordingEventApplier) ApplyEventReportConfig(_ context.Context, clusterID uint, settings *EngineEventReportSettings, _ uint) ([]EngineEventReportNodeResult, error) {
	r.calls = append(r.calls, settings)
	r.clusters = append(r.clusters, clusterID)
	if r.err != nil {
		return nil, r.err
	}
	return []EngineEventReportNodeResult{{ConfigID: 1, Template: true, Changed: true}}, nil
}

func withEventConfig(t *testing.T, secret, externalURL string) {
	t.Helper()
	previousSecret := config.Config.App.SessionSecret
	previousURL := config.Config.App.ExternalURL
	config.Config.App.SessionSecret = secret
	config.Config.App.ExternalURL = externalURL
	t.Cleanup(func() {
		config.Config.App.SessionSecret = previousSecret
		config.Config.App.ExternalURL = previousURL
	})
}

func newEventTestService(t *testing.T, version string) (*Service, *recordingEventApplier) {
	t.Helper()
	service := newTestSyncService(t)
	if err := service.repo.db.AutoMigrate(&EngineEventChannel{}, &EngineEvent{}); err != nil {
		t.Fatalf("migrate event tables: %v", err)
	}
	applier := &recordingEventApplier{}
	service.SetEventReportConfigApplier(applier)
	service.SetClusterVersionProvider(fixedClusterVersionProvider{version: version})
	return service, applier
}

var adminActor = executionapp.Actor{UserID: 1, IsAdmin: true}

func enableChannel(t *testing.T, service *Service, clusterID uint) string {
	t.Helper()
	enabled := true
	if _, err := service.UpdateEventChannelForActor(context.Background(), adminActor, clusterID, &UpdateEventChannelRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("enable channel: %v", err)
	}
	channel, err := service.repo.GetEngineEventChannel(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("load channel: %v", err)
	}
	return "Bearer " + buildEngineEventToken(clusterID, channel.TokenNonce)
}

func TestEngineEventToken(t *testing.T) {
	withEventConfig(t, "secret-a", "http://stx")
	token := buildEngineEventToken(3, "nonce")
	if token == "" || buildEngineEventToken(4, "nonce") == token || buildEngineEventToken(3, "other") == token {
		t.Fatalf("token must depend on cluster and nonce")
	}
	if !validateEngineEventAuthorization(3, "nonce", "Bearer "+token) || !validateEngineEventAuthorization(3, "nonce", "bearer "+token) {
		t.Fatalf("valid bearer token rejected")
	}
	for _, bad := range []string{"", token, "Bearer wrong", "Basic " + token} {
		if validateEngineEventAuthorization(3, "nonce", bad) {
			t.Fatalf("invalid authorization %q accepted", bad)
		}
	}
	withEventConfig(t, "", "http://stx")
	if buildEngineEventToken(3, "nonce") != "" || validateEngineEventAuthorization(3, "nonce", "Bearer "+token) {
		t.Fatalf("missing secret must disable tokens")
	}
}

func TestEnableEventChannelForInstallUsesPackageVersion(t *testing.T) {
	withEventConfig(t, "secret", "http://stx.example.com:17800/")
	ctx := context.Background()

	t.Run("3.0 package enables non-terminal reporting even when the cluster record is older", func(t *testing.T) {
		service, applier := newEventTestService(t, "2.3.11")
		if err := service.EnableEventChannelForInstall(ctx, 7, "3.0.0"); err != nil {
			t.Fatalf("enable: %v", err)
		}
		if len(applier.calls) != 1 || !applier.calls[0].ReportNonTerminal {
			t.Fatalf("expected one non-terminal write, got %+v", applier.calls)
		}
		if applier.calls[0].URL != "http://stx.example.com:17800/api/v1/sync/engine-events/7" {
			t.Fatalf("unexpected url: %+v", applier.calls[0])
		}
	})

	t.Run("older package is a no-op", func(t *testing.T) {
		service, applier := newEventTestService(t, "3.0.0")
		if err := service.EnableEventChannelForInstall(ctx, 7, "2.3.12"); err != nil {
			t.Fatalf("enable: %v", err)
		}
		if len(applier.calls) != 0 {
			t.Fatalf("2.3.12 install must not write the event channel, got %d calls", len(applier.calls))
		}
	})
}

// TestEventChannelWaitingEventsLifecycle 覆盖 waiting_events ↔ pending_restart 语义：
// 安装期写入不标记重启（waiting_events），首批事件落库清除 restart_required 并转 healthy。
// TestEventChannelWaitingEventsLifecycle covers waiting_events vs pending_restart semantics:
// install-time writes need no restart (waiting_events), and the first event batch clears
// restart_required and turns the channel healthy.
func TestEventChannelWaitingEventsLifecycle(t *testing.T) {
	withEventConfig(t, "secret", "http://stx")
	ctx := context.Background()

	t.Run("install enable waits for events without restart", func(t *testing.T) {
		service, _ := newEventTestService(t, "3.0.0")
		if err := service.EnableEventChannelForInstall(ctx, 8, "3.0.0"); err != nil {
			t.Fatalf("enable for install: %v", err)
		}
		channel, err := service.repo.GetEngineEventChannel(ctx, 8)
		if err != nil {
			t.Fatalf("load channel: %v", err)
		}
		if channel.RestartRequired || eventChannelState(channel) != EventChannelStateWaitingEvents {
			t.Fatalf("install path must be waiting_events without restart, got %+v", channel)
		}
		status, err := service.GetEventChannel(ctx, 8)
		if err != nil || status.RestartRequired || status.State != EventChannelStateWaitingEvents {
			t.Fatalf("unexpected status: %+v err=%v", status, err)
		}

		// 首批事件落库：restart_required 清回 false，通道转 healthy。
		// First event batch: restart_required clears and the channel turns healthy.
		auth := "Bearer " + buildEngineEventToken(8, channel.TokenNonce)
		if _, err := service.IngestEngineEvents(ctx, 8, auth, []byte(`[{"jobId":"9","jobStatus":"RUNNING","createdTime":1,"eventType":"JOB_STATUS"}]`)); err != nil {
			t.Fatalf("ingest: %v", err)
		}
		channel, err = service.repo.GetEngineEventChannel(ctx, 8)
		if err != nil || channel.RestartRequired || eventChannelState(channel) != EventChannelStateHealthy {
			t.Fatalf("first event must clear restart flag and turn healthy, got %+v", channel)
		}
	})

	t.Run("actor enable then disable clears restart flag", func(t *testing.T) {
		service, _ := newEventTestService(t, "3.0.0")
		enabled, disabled := true, false
		status, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{Enabled: &enabled})
		if err != nil || status.State != EventChannelStatePendingRestart || !status.RestartRequired {
			t.Fatalf("actor enable must be pending_restart, got %+v err=%v", status, err)
		}
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{Enabled: &disabled}); err != nil {
			t.Fatalf("disable: %v", err)
		}
		channel, err := service.repo.GetEngineEventChannel(ctx, 9)
		if err != nil || channel.RestartRequired {
			t.Fatalf("disable must clear restart_required, got %+v", channel)
		}
	})
}

func TestUpdateEventChannel(t *testing.T) {
	withEventConfig(t, "secret", "http://stx.example.com:17800/")
	ctx := context.Background()
	enabled := true

	t.Run("non admin is denied", func(t *testing.T) {
		service, _ := newEventTestService(t, "3.0.0")
		_, err := service.UpdateEventChannelForActor(ctx, executionapp.Actor{UserID: 2}, 1, &UpdateEventChannelRequest{Enabled: &enabled})
		if !errors.Is(err, ErrEventChannelPermissionDenied) {
			t.Fatalf("expected permission denied, got %v", err)
		}
	})

	t.Run("old version is rejected", func(t *testing.T) {
		service, applier := newEventTestService(t, "2.3.11")
		_, err := service.UpdateEventChannelForActor(ctx, adminActor, 1, &UpdateEventChannelRequest{Enabled: &enabled})
		if !errors.Is(err, ErrEventChannelVersionUnsupported) || len(applier.calls) != 0 {
			t.Fatalf("expected version error without config writes, got %v calls=%d", err, len(applier.calls))
		}
	})

	t.Run("missing external url is rejected", func(t *testing.T) {
		withEventConfig(t, "secret", "")
		service, _ := newEventTestService(t, "3.0.0")
		_, err := service.UpdateEventChannelForActor(ctx, adminActor, 1, &UpdateEventChannelRequest{Enabled: &enabled})
		if !errors.Is(err, ErrEventChannelExternalURLMissing) {
			t.Fatalf("expected external url error, got %v", err)
		}
	})

	t.Run("3.0.0 enables non-terminal reporting and never exposes the token", func(t *testing.T) {
		service, applier := newEventTestService(t, "3.0.0")
		status, err := service.UpdateEventChannelForActor(ctx, adminActor, 7, &UpdateEventChannelRequest{Enabled: &enabled})
		if err != nil {
			t.Fatalf("enable: %v", err)
		}
		settings := applier.calls[0]
		if settings.URL != "http://stx.example.com:17800/api/v1/sync/engine-events/7" || !settings.ReportNonTerminal {
			t.Fatalf("unexpected settings: %+v", settings)
		}
		if !strings.HasPrefix(settings.Headers["Authorization"], "Bearer ") {
			t.Fatalf("missing bearer header: %+v", settings.Headers)
		}
		if !status.Enabled || !status.TokenSet || status.State != EventChannelStatePendingRestart || !status.RestartRequired {
			t.Fatalf("unexpected status: %+v", status)
		}
		// 再次开启但不轮换：token 保持不变。 / Re-enabling without rotation keeps the token.
		before := settings.Headers["Authorization"]
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 7, &UpdateEventChannelRequest{Enabled: &enabled}); err != nil {
			t.Fatalf("re-enable: %v", err)
		}
		if applier.calls[1].Headers["Authorization"] != before {
			t.Fatalf("token changed without rotation")
		}
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 7, &UpdateEventChannelRequest{RotateToken: true}); err != nil {
			t.Fatalf("rotate: %v", err)
		}
		if applier.calls[2].Headers["Authorization"] == before {
			t.Fatalf("rotation must change the token")
		}
	})

	t.Run("2.3.12 reports terminal states only and disable removes config", func(t *testing.T) {
		service, applier := newEventTestService(t, "2.3.12")
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 1, &UpdateEventChannelRequest{Enabled: &enabled}); err != nil {
			t.Fatalf("enable: %v", err)
		}
		if applier.calls[0].ReportNonTerminal {
			t.Fatalf("2.3.12 must not enable non-terminal reporting")
		}
		disabled := false
		status, err := service.UpdateEventChannelForActor(ctx, adminActor, 1, &UpdateEventChannelRequest{Enabled: &disabled})
		if err != nil {
			t.Fatalf("disable: %v", err)
		}
		if applier.calls[1] != nil || status.Enabled || status.TokenSet || status.State != EventChannelStateDisabled {
			t.Fatalf("disable must remove config and clear token: %+v", status)
		}
	})
}

func createEventJob(t *testing.T, service *Service, engineJobID string, clusterID uint) *JobInstance {
	t.Helper()
	job := &JobInstance{
		TaskID: 1, TaskVersion: 1, RunType: RunTypeRun, Status: JobStatusRunning,
		PlatformJobID: engineJobID, EngineJobID: engineJobID,
		SubmitSpec: JSONMap{"cluster_id": clusterID, "engine_base_url": "http://engine"},
	}
	if err := service.repo.CreateJobInstance(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job
}

func TestIngestEngineEvents(t *testing.T) {
	withEventConfig(t, "secret", "http://stx")
	ctx := context.Background()

	t.Run("disabled channel is acknowledged and ignored", func(t *testing.T) {
		service, _ := newEventTestService(t, "3.0.0")
		result, err := service.IngestEngineEvents(ctx, 5, "Bearer x", []byte(`[]`))
		if err != nil || result.Ignored != "channel_disabled" {
			t.Fatalf("expected ignored ack, got %+v %v", result, err)
		}
	})

	t.Run("bad token is unauthorized and counted", func(t *testing.T) {
		service, _ := newEventTestService(t, "3.0.0")
		enableChannel(t, service, 5)
		if _, err := service.IngestEngineEvents(ctx, 5, "Bearer wrong", []byte(`[]`)); !errors.Is(err, ErrEventChannelUnauthorized) {
			t.Fatalf("expected unauthorized, got %v", err)
		}
		channel, _ := service.repo.GetEngineEventChannel(ctx, 5)
		if channel.AuthFailCount != 1 {
			t.Fatalf("auth failure not counted: %+v", channel)
		}
	})

	t.Run("invalid payload is acknowledged", func(t *testing.T) {
		service, _ := newEventTestService(t, "3.0.0")
		auth := enableChannel(t, service, 5)
		result, err := service.IngestEngineEvents(ctx, 5, auth, []byte(`{"not":"array"}`))
		if err != nil || result.Ignored != "invalid_payload" {
			t.Fatalf("expected invalid_payload ack, got %+v %v", result, err)
		}
		channel, _ := service.repo.GetEngineEventChannel(ctx, 5)
		if channel.ParseErrorCount != 1 {
			t.Fatalf("parse error not counted: %+v", channel)
		}
	})

	t.Run("sanitizes, dedupes and dispatches", func(t *testing.T) {
		service, _ := newEventTestService(t, "3.0.0")
		auth := enableChannel(t, service, 5)
		job := createEventJob(t, service, "1001", 5)
		otherCluster := createEventJob(t, service, "2002", 9)
		body := []byte(`[
			{"jobId":"1001","jobName":"j","jobStatus":"RUNNING","createdTime":1792913172426,"eventType":"JOB_STATUS"},
			{"jobId":"1001","jobName":"j","jobStatus":"RUNNING","createdTime":1792913172426,"eventType":"JOB_STATUS"},
			{"jobId":"1001","createdTime":1792913172466,"eventType":"LIFECYCLE_READER_OPEN"},
			{"jobId":"1001","createdTime":1792913173000,"eventType":"READER_MESSAGE_DELAYED","delayTime":4200,"record":"SourceRecord{value=Struct{password=secret}}"},
			{"jobId":"1001","createdTime":1792913174000,"eventType":"READER_MESSAGE_DELAYED","delayTime":5100,"record":"secret row"},
			{"jobId":"1001","createdTime":1792913175000,"eventType":"STAIN_TRACE","payload":"AAAA"},
			{"jobId":"2002","jobStatus":"FAILED","createdTime":1792913176000,"eventType":"JOB_STATUS"},
			{"jobId":"3003","jobStatus":"FINISHED","createdTime":1792913177000,"eventType":"JOB_STATUS"}
		]`)
		result, err := service.IngestEngineEvents(ctx, 5, auth, body)
		if err != nil {
			t.Fatalf("ingest: %v", err)
		}
		// 去重后：1 条 RUNNING、1 条生命周期、1 条分钟级延迟、2002 与 3003 的状态 = 5；STAIN_TRACE 丢弃；
		// 重复的 RUNNING 和同一分钟的第二条延迟算重复。
		// After dedupe: RUNNING, lifecycle, one minute delay bucket, 2002 and 3003 statuses = 5; STAIN_TRACE dropped;
		// the duplicate RUNNING and the second delay in the same minute count as duplicates.
		if result.Accepted != 5 || result.Duplicates != 2 || result.Dropped != 1 {
			t.Fatalf("unexpected ingest result: %+v", result)
		}
		var stored []EngineEvent
		if err := service.repo.db.Order("id").Find(&stored).Error; err != nil {
			t.Fatalf("list events: %v", err)
		}
		for _, event := range stored {
			if _, ok := event.Payload["record"]; ok {
				t.Fatalf("record field must never be stored: %+v", event)
			}
			switch event.EngineJobID {
			case "1001":
				if event.JobInstanceID == nil || *event.JobInstanceID != job.ID {
					t.Fatalf("event not linked to job: %+v", event)
				}
			case "2002", "3003":
				if event.JobInstanceID != nil {
					t.Fatalf("job of another cluster or unknown job must not be linked: %+v (other=%d)", event, otherCluster.ID)
				}
			}
		}
		delay := service.jobCDCDelay(job.ID)
		if delay == nil || delay.LatestMs != 5100 || delay.MaxMs != 5100 || !delay.Fresh {
			t.Fatalf("cdc delay summary should follow the newest sample: %+v", delay)
		}
		var minute EngineEvent
		if err := service.repo.db.Where("event_type = ?", EngineEventTypeMessageDelayed).First(&minute).Error; err != nil {
			t.Fatalf("load minute bucket: %v", err)
		}
		if int64Field(minute.Payload, "delay_ms") != 5100 {
			t.Fatalf("minute bucket should keep the batch max, got %v", minute.Payload)
		}
		service.events.mu.Lock()
		_, pending := service.events.pending[job.ID]
		_, otherPending := service.events.pending[otherCluster.ID]
		service.events.mu.Unlock()
		if !pending || otherPending {
			t.Fatalf("only the linked job should be queued for refresh")
		}

		// 换主后整批重放：全部算重复，不再触发刷新。
		// A full replay after master failover: everything is a duplicate and no refresh is queued.
		service.events.mu.Lock()
		service.events.pending = nil
		service.events.mu.Unlock()
		replay, err := service.IngestEngineEvents(ctx, 5, auth, body)
		if err != nil || replay.Accepted != 0 {
			t.Fatalf("replay should not insert, got %+v %v", replay, err)
		}
		service.events.mu.Lock()
		replayPending := len(service.events.pending)
		service.events.mu.Unlock()
		if replayPending != 0 {
			t.Fatalf("replayed status must not queue refreshes")
		}
		channel, _ := service.repo.GetEngineEventChannel(ctx, 5)
		if channel.LastEventAt == nil || eventChannelState(channel) != EventChannelStateHealthy || channel.ReceivedCount != 5 {
			t.Fatalf("channel should be healthy after events: %+v", channel)
		}

		listed, err := service.ListJobEventsForActor(ctx, adminActor, job.ID, []string{"job_status"}, 1, 10)
		if err != nil || listed.Total != 1 || listed.Events[0].JobStatus != "RUNNING" {
			t.Fatalf("unexpected timeline: %+v %v", listed, err)
		}
	})
}

func TestEngineEventDebounceAndExpectations(t *testing.T) {
	withEventConfig(t, "secret", "http://stx")
	service, _ := newEventTestService(t, "3.0.0")
	now := time.Unix(10_000, 0)
	service.events.now = func() time.Time { return now }

	service.enqueueEventRefresh(1)
	service.enqueueEventRefresh(1)
	if due := service.takeDueEventRefreshes(); len(due) != 0 {
		t.Fatalf("refresh must wait for the debounce window, got %v", due)
	}
	now = now.Add(engineEventRefreshDebounce)
	if due := service.takeDueEventRefreshes(); len(due) != 1 || due[0] != 1 {
		t.Fatalf("expected a single debounced refresh, got %v", due)
	}

	ctx := context.Background()
	auth := enableChannel(t, service, 5)
	job := createEventJob(t, service, "1001", 5)
	// 通道还没收到过事件（pending_restart）时不登记预期。
	// No expectation is registered while the channel has never received events (pending_restart).
	service.registerEventExpectation(ctx, job, 5)
	if missed := service.takeMissedEventExpectations(); len(missed) != 0 || len(service.events.expectations) != 0 {
		t.Fatalf("expectation registered too early")
	}
	if _, err := service.IngestEngineEvents(ctx, 5, auth, []byte(`[{"jobId":"9","jobStatus":"RUNNING","createdTime":1,"eventType":"JOB_STATUS"}]`)); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	service.registerEventExpectation(ctx, job, 5)
	now = now.Add(engineEventExpectWindow + time.Second)
	if missed := service.takeMissedEventExpectations(); len(missed) != 1 || missed[0] != 5 {
		t.Fatalf("expected a missed expectation for cluster 5, got %v", missed)
	}

	service.registerEventExpectation(ctx, job, 5)
	if !service.satisfyEventExpectation("1001") || service.satisfyEventExpectation("1001") {
		t.Fatalf("expectation should be satisfied exactly once")
	}
}

func TestEventChannelState(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Minute)
	cases := []struct {
		name    string
		channel *EngineEventChannel
		want    string
	}{
		{name: "nil", channel: nil, want: EventChannelStateDisabled},
		{name: "disabled", channel: &EngineEventChannel{}, want: EventChannelStateDisabled},
		{name: "waiting for restart", channel: &EngineEventChannel{Enabled: true, RestartRequired: true}, want: EventChannelStatePendingRestart},
		{name: "waiting for first event", channel: &EngineEventChannel{Enabled: true}, want: EventChannelStateWaitingEvents},
		{name: "healthy", channel: &EngineEventChannel{Enabled: true, LastEventAt: &now}, want: EventChannelStateHealthy},
		{name: "missed expectation", channel: &EngineEventChannel{Enabled: true, LastEventAt: &now, ExpectMissedAt: &now}, want: EventChannelStateDegraded},
		{name: "recovered", channel: &EngineEventChannel{Enabled: true, LastEventAt: &now, ExpectMissedAt: &earlier, ExpectSatisfiedAt: &now}, want: EventChannelStateHealthy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := eventChannelState(tc.channel); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestIngestEngineEventsHandler_httpContract(t *testing.T) {
	withEventConfig(t, "secret", "http://stx")
	gin.SetMode(gin.TestMode)
	service, _ := newEventTestService(t, "3.0.0")
	auth := enableChannel(t, service, 5)
	router := gin.New()
	router.POST("/api/v1/sync/engine-events/:cluster_id", NewHandler(service).IngestEngineEvents)

	post := func(path, authorization, body string) int {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", authorization)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if code := post("/api/v1/sync/engine-events/5", "Bearer wrong", `[]`); code != http.StatusUnauthorized {
		t.Fatalf("bad token must be 401, got %d", code)
	}
	if code := post("/api/v1/sync/engine-events/5", auth, `not json`); code != http.StatusOK {
		t.Fatalf("invalid payload must still be 200, got %d", code)
	}
	if code := post("/api/v1/sync/engine-events/abc", auth, `[]`); code != http.StatusOK {
		t.Fatalf("invalid cluster id must still be 200, got %d", code)
	}
	if code := post("/api/v1/sync/engine-events/6", "Bearer x", `[]`); code != http.StatusOK {
		t.Fatalf("disabled channel must be 200, got %d", code)
	}
	large := "[" + strings.Repeat(`{"eventType":"LIFECYCLE_READER_OPEN","jobId":"1","createdTime":1},`, EngineEventIngestMaxBytes/60) + "{}]"
	if code := post("/api/v1/sync/engine-events/5", auth, large); code != http.StatusOK {
		t.Fatalf("oversized body must be acknowledged with 200, got %d", code)
	}
}

func TestIngestEngineEvents_schemaChangePayloadIsSlimmedAndRedacted(t *testing.T) {
	withEventConfig(t, "secret", "http://stx")
	ctx := context.Background()
	service, _ := newEventTestService(t, "3.0.0")
	auth := enableChannel(t, service, 5)
	job := createEventJob(t, service, "1001", 5)
	// 结构取自 3.0.0 Postgres-CDC 实测的 SCHEMA_CHANGE_UPDATE_COLUMNS。
	// Shape taken from a real 3.0.0 Postgres-CDC SCHEMA_CHANGE_UPDATE_COLUMNS event.
	body := []byte(`[{"eventType":"SCHEMA_CHANGE_UPDATE_COLUMNS","jobId":"1001","createdTime":1792929533000,
		"tableIdentifier":{"catalogName":"Postgres","databaseName":"seatunnel_test","schemaName":"public","tableName":"cdc_orders"},
		"sourceOptions":{"password":"leak","url":"jdbc:mysql://u:leak@db:3306/x?password=leak&useSSL=false"},
		"changeAfter":{"options":{"url":"jdbc:postgresql://u:pw@db:5432/x?password=leak","password":"leak"},"tableSchema":{"columns":[{"name":"id"}]}},
		"events":[{"eventType":"SCHEMA_CHANGE_ADD_COLUMN","column":{"name":"remark","sourceType":"varchar(32)"},"changeAfter":{"options":{"password":"leak"}}}]}]`)
	if _, err := service.IngestEngineEvents(ctx, 5, auth, body); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	listed, err := service.ListJobEventsForActor(ctx, adminActor, job.ID, nil, 1, 10)
	if err != nil || listed.Total != 1 {
		t.Fatalf("unexpected timeline: %+v %v", listed, err)
	}
	payload := listed.Events[0].Payload
	encoded, _ := json.Marshal(payload)
	if strings.Contains(string(encoded), "changeAfter") || strings.Contains(string(encoded), "leak") {
		t.Fatalf("schema payload must drop changeAfter and any secrets: %s", encoded)
	}
	events, _ := payload["events"].([]any)
	first, _ := events[0].(map[string]any)
	column, _ := first["column"].(map[string]any)
	if column["name"] != "remark" || payload["tableIdentifier"] == nil {
		t.Fatalf("table and column info must be kept: %s", encoded)
	}
}

func TestIngestEngineEvents_bumpsDelayAcrossBatches(t *testing.T) {
	withEventConfig(t, "secret", "http://stx")
	service, _ := newEventTestService(t, "3.0.0")
	auth := enableChannel(t, service, 5)
	_ = createEventJob(t, service, "1001", 5)
	ctx := context.Background()
	first := []byte(`[{"jobId":"1001","createdTime":1792913173000,"eventType":"READER_MESSAGE_DELAYED","delayTime":4200}]`)
	if _, err := service.IngestEngineEvents(ctx, 5, auth, first); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	second := []byte(`[{"jobId":"1001","createdTime":1792913174000,"eventType":"READER_MESSAGE_DELAYED","delayTime":9100}]`)
	result, err := service.IngestEngineEvents(ctx, 5, auth, second)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if result.Accepted != 0 || result.Duplicates < 1 {
		t.Fatalf("second batch should collide on the minute bucket: %+v", result)
	}
	var minute EngineEvent
	if err := service.repo.db.Where("event_type = ?", EngineEventTypeMessageDelayed).First(&minute).Error; err != nil {
		t.Fatalf("load minute bucket: %v", err)
	}
	if int64Field(minute.Payload, "delay_ms") != 9100 {
		t.Fatalf("cross-batch ingest must keep the larger delay, got %v", minute.Payload)
	}
}

// fakeK8sCallbackProvider 实现 ClusterK8sCallbackProvider 的最小内存版。
// fakeK8sCallbackProvider is a minimal in-memory ClusterK8sCallbackProvider.
type fakeK8sCallbackProvider struct {
	k8s       bool
	override  string
	setValues []string
}

func (f *fakeK8sCallbackProvider) IsKubernetesCluster(_ context.Context, _ uint) bool {
	return f.k8s
}

func (f *fakeK8sCallbackProvider) GetK8sSTXCallbackURL(_ context.Context, _ uint) (string, error) {
	return f.override, nil
}

func (f *fakeK8sCallbackProvider) SetK8sSTXCallbackURL(_ context.Context, _ uint, callbackURL string) error {
	f.override = callbackURL
	f.setValues = append(f.setValues, callbackURL)
	return nil
}

// TestUpdateEventChannelK8sCallbackResolution 覆盖两级 URL 解析、覆写持久化、清空覆写与 loopback 拦截。
// TestUpdateEventChannelK8sCallbackResolution covers two-level URL resolution, override
// persistence, override clearing and the loopback guard.
func TestUpdateEventChannelK8sCallbackResolution(t *testing.T) {
	ctx := context.Background()
	enabled := true

	newK8sService := func(t *testing.T, externalURL, override string) (*Service, *recordingEventApplier, *fakeK8sCallbackProvider) {
		t.Helper()
		withEventConfig(t, "secret", externalURL)
		service, applier := newEventTestService(t, "3.1.0")
		provider := &fakeK8sCallbackProvider{k8s: true, override: override}
		service.SetClusterK8sCallbackProvider(provider)
		return service, applier, provider
	}

	t.Run("k8s override wins over global external url", func(t *testing.T) {
		service, applier, _ := newK8sService(t, "http://stx.example.com:17800", "http://stx.stx-system.svc.cluster.local:17800/")
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{Enabled: &enabled}); err != nil {
			t.Fatalf("enable: %v", err)
		}
		want := "http://stx.stx-system.svc.cluster.local:17800/api/v1/sync/engine-events/9"
		if applier.calls[0].URL != want {
			t.Fatalf("url = %q, want %q", applier.calls[0].URL, want)
		}
	})

	t.Run("k8s without override falls back to global", func(t *testing.T) {
		service, applier, _ := newK8sService(t, "http://stx.example.com:17800", "")
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{Enabled: &enabled}); err != nil {
			t.Fatalf("enable: %v", err)
		}
		want := "http://stx.example.com:17800/api/v1/sync/engine-events/9"
		if applier.calls[0].URL != want {
			t.Fatalf("url = %q, want %q", applier.calls[0].URL, want)
		}
	})

	t.Run("loopback override is rejected before any config write", func(t *testing.T) {
		service, applier, _ := newK8sService(t, "http://stx.example.com:17800", "http://127.0.0.1:17800")
		_, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{Enabled: &enabled})
		if !errors.Is(err, ErrEventChannelCallbackLoopback) {
			t.Fatalf("expected loopback error, got %v", err)
		}
		if len(applier.calls) != 0 {
			t.Fatalf("configmap must not be written on loopback rejection, calls=%d", len(applier.calls))
		}
	})

	t.Run("loopback global url is rejected for k8s", func(t *testing.T) {
		service, applier, _ := newK8sService(t, "http://localhost:17800", "")
		_, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{Enabled: &enabled})
		if !errors.Is(err, ErrEventChannelCallbackLoopback) {
			t.Fatalf("expected loopback error, got %v", err)
		}
		if len(applier.calls) != 0 {
			t.Fatalf("no writes allowed, calls=%d", len(applier.calls))
		}
	})

	t.Run("bare-metal keeps loopback external url working (no k8s guard)", func(t *testing.T) {
		withEventConfig(t, "secret", "http://127.0.0.1:17800")
		service, applier := newEventTestService(t, "3.1.0")
		service.SetClusterK8sCallbackProvider(&fakeK8sCallbackProvider{k8s: false})
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{Enabled: &enabled}); err != nil {
			t.Fatalf("bare-metal loopback must not be rejected: %v", err)
		}
		if len(applier.calls) != 1 {
			t.Fatalf("expected one write, got %d", len(applier.calls))
		}
	})

	t.Run("stx_callback_url persistence and explicit clear", func(t *testing.T) {
		service, _, provider := newK8sService(t, "http://stx.example.com:17800", "")
		override := "http://10.0.2.2:17800"
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{STXCallbackURL: &override}); err != nil {
			t.Fatalf("persist override: %v", err)
		}
		if provider.override != override {
			t.Fatalf("override = %q, want %q", provider.override, override)
		}
		empty := ""
		if _, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{STXCallbackURL: &empty}); err != nil {
			t.Fatalf("clear override: %v", err)
		}
		if provider.override != "" {
			t.Fatalf("override must be cleared, got %q", provider.override)
		}
	})

	t.Run("non-k8s cluster rejects non-empty override", func(t *testing.T) {
		withEventConfig(t, "secret", "http://stx.example.com:17800")
		service, _ := newEventTestService(t, "3.1.0")
		service.SetClusterK8sCallbackProvider(&fakeK8sCallbackProvider{k8s: false})
		override := "http://10.0.2.2:17800"
		_, err := service.UpdateEventChannelForActor(ctx, adminActor, 9, &UpdateEventChannelRequest{STXCallbackURL: &override})
		if !errors.Is(err, ErrEventChannelCallbackNotApplicable) {
			t.Fatalf("expected not-applicable error, got %v", err)
		}
	})

	t.Run("status exposes override and both urls", func(t *testing.T) {
		service, _, _ := newK8sService(t, "http://stx.example.com:17800", "http://10.0.2.2:17800")
		status, err := service.GetEventChannel(ctx, 9)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !status.IsKubernetes || status.STXCallbackURL != "http://10.0.2.2:17800" {
			t.Fatalf("unexpected status: %+v", status)
		}
		if status.URL != "http://10.0.2.2:17800/api/v1/sync/engine-events/9" {
			t.Fatalf("resolved url wrong: %q", status.URL)
		}
		if status.DefaultURL != "http://stx.example.com:17800/api/v1/sync/engine-events/9" {
			t.Fatalf("default url wrong: %q", status.DefaultURL)
		}
	})
}

// TestIsLoopbackCallbackHost 覆盖回环判定的边界输入。
// TestIsLoopbackCallbackHost covers boundary inputs of the loopback detection.
func TestIsLoopbackCallbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost":             true,
		"127.0.0.1":             true,
		"127.1.2.3":             true,
		"::1":                   true,
		"0.0.0.0":               true,
		"":                      true,
		"10.0.2.2":              false,
		"stx.stx-system.svc":    false,
		"192.168.49.1":          false,
		"localhost.example.com": false,
	} {
		if got := isLoopbackCallbackHost(host); got != want {
			t.Fatalf("host %q = %v, want %v", host, got, want)
		}
	}
}
