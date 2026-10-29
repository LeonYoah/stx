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
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

const (
	// EngineEventIngestMaxBytes 限制单次上报请求体；引擎每批最多 1000 条事件。
	// EngineEventIngestMaxBytes bounds one report body; the engine sends at most 1000 events per batch.
	EngineEventIngestMaxBytes = 4 << 20

	engineEventPayloadMaxBytes   = 16 << 10
	engineEventRetention         = 7 * 24 * time.Hour
	engineEventRefreshDebounce   = time.Second
	engineEventRefreshConcurrent = 4
	// engineEventExpectWindow = 引擎固定 10 秒批量间隔 + 余量。
	// engineEventExpectWindow is the engine's fixed 10s batch interval plus slack.
	engineEventExpectWindow = 45 * time.Second
	// cdcDelayFreshWindow 内没有新的延迟事件，就认为延迟已低于 1 秒（引擎只在 ≥1 秒时上报）。
	// Without a delay event within cdcDelayFreshWindow the delay is considered below 1s (the engine only reports ≥1s).
	cdcDelayFreshWindow = 60 * time.Second
)

// 事件类型与通道状态常量。
// Event type and channel state constants.
const (
	EngineEventTypeJobStatus      = "JOB_STATUS"
	EngineEventTypeMessageDelayed = "READER_MESSAGE_DELAYED"
	EngineEventTypeStainTrace     = "STAIN_TRACE"

	EventChannelStateDisabled       = "disabled"
	EventChannelStatePendingRestart = "pending_restart"
	EventChannelStateHealthy        = "healthy"
	EventChannelStateDegraded       = "degraded"
)

// EngineEventReportSettings 是要写入 seatunnel.yaml 的 event-report-http 内容（sync 侧定义，router 适配到 config 应用）。
// EngineEventReportSettings is the event-report-http content for seatunnel.yaml (defined in sync and adapted to the
// config app in the router).
type EngineEventReportSettings struct {
	URL               string
	Headers           map[string]string
	ReportNonTerminal bool
}

// EngineEventReportNodeResult 是一份 seatunnel.yaml 的写入结果。
// EngineEventReportNodeResult is the write outcome for one seatunnel.yaml.
type EngineEventReportNodeResult struct {
	ConfigID uint   `json:"config_id"`
	HostID   *uint  `json:"host_id,omitempty"`
	Template bool   `json:"template"`
	Changed  bool   `json:"changed"`
	Pushed   bool   `json:"pushed"`
	Pending  bool   `json:"pending"`
	Error    string `json:"error,omitempty"`
}

// EventReportConfigApplier 写入或移除集群 seatunnel.yaml 中的 event-report-http（settings 为 nil 表示移除）。
// EventReportConfigApplier writes or removes event-report-http in the cluster seatunnel.yaml (nil settings remove it).
type EventReportConfigApplier interface {
	ApplyEventReportConfig(ctx context.Context, clusterID uint, settings *EngineEventReportSettings, userID uint) ([]EngineEventReportNodeResult, error)
}

// SetEventReportConfigApplier 注入 seatunnel.yaml 写入器。
// SetEventReportConfigApplier injects the seatunnel.yaml writer.
func (s *Service) SetEventReportConfigApplier(applier EventReportConfigApplier) {
	s.eventConfigApplier = applier
}

// UpdateEventChannelRequest 是 PUT /api/v1/sync/event-channels/:cluster_id 的请求体。
// UpdateEventChannelRequest is the body of PUT /api/v1/sync/event-channels/:cluster_id.
type UpdateEventChannelRequest struct {
	Enabled     *bool `json:"enabled"`
	RotateToken bool  `json:"rotate_token"`
}

// EventChannelStatus 是事件通道的对外视图；token 只以 token_set 表示，永不返回原文。
// EventChannelStatus is the public view of an event channel; the token is only exposed as token_set.
type EventChannelStatus struct {
	ClusterID         uint                          `json:"cluster_id"`
	Enabled           bool                          `json:"enabled"`
	State             string                        `json:"state"`
	TokenSet          bool                          `json:"token_set"`
	URL               string                        `json:"url"`
	Version           string                        `json:"version"`
	Supported         bool                          `json:"supported"`
	ReportNonTerminal bool                          `json:"report_non_terminal"`
	RestartRequired   bool                          `json:"restart_required"`
	ConfigAppliedAt   *time.Time                    `json:"config_applied_at,omitempty"`
	ConfigApplyError  string                        `json:"config_apply_error,omitempty"`
	LastEventAt       *time.Time                    `json:"last_event_at,omitempty"`
	LastBatchAt       *time.Time                    `json:"last_batch_at,omitempty"`
	ReceivedCount     int64                         `json:"received_count"`
	DuplicateCount    int64                         `json:"duplicate_count"`
	DroppedCount      int64                         `json:"dropped_count"`
	ParseErrorCount   int64                         `json:"parse_error_count"`
	AuthFailCount     int64                         `json:"auth_fail_count"`
	ExpectMissedAt    *time.Time                    `json:"expect_missed_at,omitempty"`
	ExpectSatisfiedAt *time.Time                    `json:"expect_satisfied_at,omitempty"`
	Configs           []EngineEventReportNodeResult `json:"configs,omitempty"`
}

// EngineEventIngestResult 是接收端对一批事件的处理摘要。
// EngineEventIngestResult summarizes how one event batch was handled.
type EngineEventIngestResult struct {
	Accepted   int    `json:"accepted"`
	Duplicates int    `json:"duplicates"`
	Dropped    int    `json:"dropped"`
	Ignored    string `json:"ignored,omitempty"`
}

// JobCDCDelay 是作业最近的 CDC 发射延迟摘要（内存聚合，不落库）。
// JobCDCDelay summarizes a job's recent CDC emit delay (aggregated in memory, never persisted).
type JobCDCDelay struct {
	LatestMs int64     `json:"latest_ms"`
	MaxMs    int64     `json:"max_1m_ms"`
	At       time.Time `json:"at"`
	Fresh    bool      `json:"fresh"`
}

// engineEventState 保存事件运行时的内存状态，零值可用。
// engineEventState holds in-memory event runtime state; its zero value is ready to use.
type engineEventState struct {
	mu           gosync.Mutex
	pending      map[uint]time.Time          // jobID -> first event time awaiting refresh
	expectations map[string]eventExpectation // engineJobID -> expectation
	cdcDelays    map[uint]*cdcDelayState
	// lastFullReconcile 记录每个集群上次完整对账的时间（事件模式用）。
	// lastFullReconcile records each cluster's last full reconcile time (used by event mode).
	lastFullReconcile map[uint]time.Time
	now               func() time.Time
}

type eventExpectation struct {
	clusterID uint
	deadline  time.Time
}

type cdcDelayState struct {
	latestMs      int64
	maxMs         int64
	windowStartMs int64
	lastSampleMs  int64
	at            time.Time
}

func (e *engineEventState) clock() time.Time {
	if e.now != nil {
		return e.now()
	}
	return time.Now()
}

// buildEngineEventToken 用 SessionSecret 对 集群 ID + nonce 做 HMAC，token 不落库。
// buildEngineEventToken HMACs cluster ID + nonce with SessionSecret; the token itself is never stored.
func buildEngineEventToken(clusterID uint, nonce string) string {
	secret := ""
	if config.Config != nil {
		secret = strings.TrimSpace(config.Config.App.SessionSecret)
	}
	if secret == "" || clusterID == 0 || strings.TrimSpace(nonce) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("engine-events:" + strconv.FormatUint(uint64(clusterID), 10) + ":" + strings.TrimSpace(nonce)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// validateEngineEventAuthorization 以常量时间比较 "Bearer <token>"。
// validateEngineEventAuthorization compares "Bearer <token>" in constant time.
func validateEngineEventAuthorization(clusterID uint, nonce, authorization string) bool {
	expected := buildEngineEventToken(clusterID, nonce)
	if expected == "" {
		return false
	}
	token := strings.TrimSpace(authorization)
	if len(token) < 7 || !strings.EqualFold(token[:7], "bearer ") {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(token[7:])))
}

func newEventTokenNonce() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

// engineEventsURL 返回写入 seatunnel.yaml 的接收地址。
// engineEventsURL returns the receiver URL written into seatunnel.yaml.
func engineEventsURL(clusterID uint) string {
	base := strings.TrimRight(strings.TrimSpace(config.GetExternalURL()), "/")
	if base == "" {
		return ""
	}
	return base + "/api/v1/sync/engine-events/" + strconv.FormatUint(uint64(clusterID), 10)
}

// eventChannelState 根据通道记录计算状态。
// eventChannelState derives the channel state from its record.
func eventChannelState(channel *EngineEventChannel) string {
	switch {
	case channel == nil || !channel.Enabled:
		return EventChannelStateDisabled
	case channel.LastEventAt == nil:
		return EventChannelStatePendingRestart
	case channel.ExpectMissedAt != nil && (channel.ExpectSatisfiedAt == nil || channel.ExpectMissedAt.After(*channel.ExpectSatisfiedAt)):
		return EventChannelStateDegraded
	default:
		return EventChannelStateHealthy
	}
}

func (s *Service) clusterVersion(ctx context.Context, clusterID uint) string {
	if s.clusterVersionProvider == nil || clusterID == 0 {
		return ""
	}
	version, err := s.clusterVersionProvider.GetClusterVersion(ctx, clusterID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(version)
}

func (s *Service) toEventChannelStatus(ctx context.Context, clusterID uint, channel *EngineEventChannel) *EventChannelStatus {
	version := s.clusterVersion(ctx, clusterID)
	status := &EventChannelStatus{
		ClusterID: clusterID,
		State:     EventChannelStateDisabled,
		URL:       engineEventsURL(clusterID),
		Version:   version,
		Supported: seatunnel.SupportsJobStateEvents(version),
	}
	if channel == nil {
		return status
	}
	status.Enabled = channel.Enabled
	status.State = eventChannelState(channel)
	status.TokenSet = channel.Enabled && strings.TrimSpace(channel.TokenNonce) != ""
	status.ReportNonTerminal = channel.ReportNonTerminal
	status.RestartRequired = status.State == EventChannelStatePendingRestart
	status.ConfigAppliedAt = channel.ConfigAppliedAt
	status.ConfigApplyError = channel.ConfigApplyError
	status.LastEventAt = channel.LastEventAt
	status.LastBatchAt = channel.LastBatchAt
	status.ReceivedCount = channel.ReceivedCount
	status.DuplicateCount = channel.DuplicateCount
	status.DroppedCount = channel.DroppedCount
	status.ParseErrorCount = channel.ParseErrorCount
	status.AuthFailCount = channel.AuthFailCount
	status.ExpectMissedAt = channel.ExpectMissedAt
	status.ExpectSatisfiedAt = channel.ExpectSatisfiedAt
	return status
}

// GetEventChannel 返回集群事件通道状态；没有记录时返回 disabled。
// GetEventChannel returns a cluster event channel status; without a record it reports disabled.
func (s *Service) GetEventChannel(ctx context.Context, clusterID uint) (*EventChannelStatus, error) {
	channel, err := s.repo.GetEngineEventChannel(ctx, clusterID)
	if err != nil && err != ErrEventChannelNotFound {
		return nil, err
	}
	return s.toEventChannelStatus(ctx, clusterID, channel), nil
}

// UpdateEventChannelForActor 开启、关闭或轮换事件通道，并把 event-report-http 写入 seatunnel.yaml；仅管理员可操作。
// UpdateEventChannelForActor enables, disables or rotates the event channel and writes event-report-http into
// seatunnel.yaml; admins only.
func (s *Service) UpdateEventChannelForActor(ctx context.Context, actor executionapp.Actor, clusterID uint, req *UpdateEventChannelRequest) (*EventChannelStatus, error) {
	if !actor.IsAdmin {
		return nil, ErrEventChannelPermissionDenied
	}
	if req == nil {
		req = &UpdateEventChannelRequest{}
	}
	if s.eventConfigApplier == nil {
		return nil, ErrEventChannelApplierMissing
	}
	channel, err := s.repo.GetEngineEventChannel(ctx, clusterID)
	if err != nil && err != ErrEventChannelNotFound {
		return nil, err
	}
	if channel == nil {
		channel = &EngineEventChannel{ClusterID: clusterID}
	}
	previousState := eventChannelState(channel)
	enable := channel.Enabled
	if req.Enabled != nil {
		enable = *req.Enabled
	}
	userID := uint(actor.UserID)
	now := s.events.clock()

	var settings *EngineEventReportSettings
	if enable {
		version := s.clusterVersion(ctx, clusterID)
		if !seatunnel.SupportsJobStateEvents(version) {
			return nil, ErrEventChannelVersionUnsupported
		}
		url := engineEventsURL(clusterID)
		if url == "" {
			return nil, ErrEventChannelExternalURLMissing
		}
		if config.Config == nil || strings.TrimSpace(config.Config.App.SessionSecret) == "" {
			return nil, ErrEventChannelSecretMissing
		}
		rotated := false
		if req.RotateToken || strings.TrimSpace(channel.TokenNonce) == "" || !channel.Enabled {
			nonce, nonceErr := newEventTokenNonce()
			if nonceErr != nil {
				return nil, nonceErr
			}
			channel.TokenNonce = nonce
			rotated = true
		}
		channel.ReportNonTerminal = seatunnel.SupportsNonTerminalJobStateEvents(version)
		settings = &EngineEventReportSettings{
			URL:               url,
			Headers:           map[string]string{"Authorization": "Bearer " + buildEngineEventToken(clusterID, channel.TokenNonce)},
			ReportNonTerminal: channel.ReportNonTerminal,
		}
		if rotated || !channel.Enabled {
			// 新配置要等集群重启后才生效，重新进入 pending_restart。
			// New config only takes effect after a cluster restart, so re-enter pending_restart.
			channel.LastEventAt = nil
			channel.ExpectMissedAt = nil
			channel.ExpectSatisfiedAt = nil
		}
	}

	results, applyErr := s.eventConfigApplier.ApplyEventReportConfig(ctx, clusterID, settings, userID)
	channel.Enabled = enable
	if applyErr != nil {
		channel.ConfigApplyError = applyErr.Error()
	} else {
		channel.ConfigApplyError = summarizeEventConfigErrors(results)
		channel.ConfigAppliedAt = &now
	}
	if !enable {
		channel.TokenNonce = ""
	}
	if err := s.repo.SaveEngineEventChannel(ctx, channel); err != nil {
		return nil, err
	}
	// 关闭或重配通道会离开降级状态，结束 D2 告警。
	// Disabling or reconfiguring the channel leaves the degraded state and ends the D2 alert.
	s.reportEventChannelState(clusterID, previousState, eventChannelState(channel))
	if applyErr != nil {
		return nil, applyErr
	}
	status := s.toEventChannelStatus(ctx, clusterID, channel)
	status.RestartRequired = true
	status.Configs = results
	return status, nil
}

func summarizeEventConfigErrors(results []EngineEventReportNodeResult) string {
	var parts []string
	for _, item := range results {
		if item.Error == "" {
			continue
		}
		target := "template"
		if item.HostID != nil {
			target = "host " + strconv.FormatUint(uint64(*item.HostID), 10)
		}
		parts = append(parts, target+": "+item.Error)
	}
	return strings.Join(parts, "; ")
}

// IngestEngineEvents 接收 event-report-http 的一批事件。token 错误返回 ErrEventChannelUnauthorized；
// 其他任何问题（通道未开启、内容非法、未知作业）都返回成功，避免引擎重试同一批而堵塞后续事件。
// IngestEngineEvents receives one event-report-http batch. A bad token returns ErrEventChannelUnauthorized; every
// other problem (channel disabled, invalid content, unknown job) succeeds so the engine never retries the same batch
// and blocks later events.
func (s *Service) IngestEngineEvents(ctx context.Context, clusterID uint, authorization string, body []byte) (*EngineEventIngestResult, error) {
	result := &EngineEventIngestResult{}
	channel, err := s.repo.GetEngineEventChannel(ctx, clusterID)
	if err != nil {
		if err == ErrEventChannelNotFound {
			result.Ignored = "channel_disabled"
			return result, nil
		}
		return nil, err
	}
	if !channel.Enabled {
		result.Ignored = "channel_disabled"
		return result, nil
	}
	if !validateEngineEventAuthorization(clusterID, channel.TokenNonce, authorization) {
		_ = s.repo.UpdateEngineEventChannelStats(ctx, clusterID, map[string]int64{"auth_fail_count": 1}, nil)
		return nil, ErrEventChannelUnauthorized
	}
	now := s.events.clock()

	var raw []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		logger.WarnF(ctx, "[Sync] engine event batch is not a JSON array: cluster=%d bytes=%d", clusterID, len(body))
		_ = s.repo.UpdateEngineEventChannelStats(ctx, clusterID, map[string]int64{"parse_error_count": 1}, map[string]any{"last_batch_at": now})
		result.Ignored = "invalid_payload"
		return result, nil
	}

	jobCache := map[string]*JobInstance{}
	events := make([]*EngineEvent, 0, len(raw))
	seen := map[string]*EngineEvent{}
	statusJobs := map[string]*JobInstance{}
	type delaySample struct {
		jobID uint
		atMs  int64
		delay int64
	}
	var delaySamples []delaySample
	for _, item := range raw {
		event, job, keep := s.sanitizeEngineEvent(ctx, clusterID, item, jobCache, now)
		if !keep {
			result.Dropped++
			continue
		}
		if event.EventType == EngineEventTypeMessageDelayed && job != nil {
			// 延迟摘要用每条样本（按原始时间单调更新），不受分钟去重影响。
			// The delay summary uses every sample (monotonic by original time), unaffected by minute dedupe.
			delaySamples = append(delaySamples, delaySample{jobID: job.ID, atMs: int64Field(item, "createdTime"), delay: int64Field(event.Payload, "delay_ms")})
		}
		if kept, ok := seen[event.DedupeKey]; ok {
			if event.EventType == EngineEventTypeMessageDelayed && int64Field(event.Payload, "delay_ms") > int64Field(kept.Payload, "delay_ms") {
				// 同一分钟桶保留本批最大延迟。 / Keep the batch maximum within one minute bucket.
				kept.Payload["delay_ms"] = event.Payload["delay_ms"]
			}
			result.Duplicates++
			continue
		}
		seen[event.DedupeKey] = event
		events = append(events, event)
		if job != nil && event.EventType == EngineEventTypeJobStatus {
			statusJobs[event.EngineJobID] = job
		}
	}

	keys := make([]string, 0, len(events))
	for _, event := range events {
		keys = append(keys, event.DedupeKey)
	}
	existing, err := s.repo.ExistingEngineEventDedupeKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	fresh := make([]*EngineEvent, 0, len(events))
	for _, event := range events {
		if existing[event.DedupeKey] {
			result.Duplicates++
			continue
		}
		fresh = append(fresh, event)
	}
	if err := s.repo.InsertEngineEventsIgnoreDuplicates(ctx, fresh); err != nil {
		return nil, err
	}
	result.Accepted = len(fresh)

	// 重放的旧状态事件不再触发刷新；新事件按作业防抖刷新。
	// Replayed status events do not trigger refreshes; new ones refresh their job with debouncing.
	freshStatus := map[string]bool{}
	for _, event := range fresh {
		if event.EventType == EngineEventTypeJobStatus {
			freshStatus[event.EngineJobID] = true
		}
	}
	satisfied := false
	for engineJobID, job := range statusJobs {
		if freshStatus[engineJobID] {
			s.enqueueEventRefresh(job.ID)
		}
		if s.satisfyEventExpectation(engineJobID) {
			satisfied = true
		}
	}
	for _, sample := range delaySamples {
		s.recordCDCDelay(sample.jobID, sample.delay, sample.atMs)
	}

	fields := map[string]any{"last_batch_at": now}
	if len(raw) > 0 {
		fields["last_event_at"] = now
	}
	if satisfied {
		fields["expect_satisfied_at"] = now
	}
	_ = s.updateEventChannelStatsWithAlert(ctx, clusterID, map[string]int64{
		"received_count":  int64(result.Accepted),
		"duplicate_count": int64(result.Duplicates),
		"dropped_count":   int64(result.Dropped),
	}, fields)
	return result, nil
}

// sanitizeEngineEvent 清洗单条事件：丢弃 STAIN_TRACE，删除 READER_MESSAGE_DELAYED 的 record（行数据），限制 payload 大小，计算去重键。
// sanitizeEngineEvent cleans one event: drops STAIN_TRACE, removes READER_MESSAGE_DELAYED.record (row data), bounds
// the payload size and computes the dedupe key.
func (s *Service) sanitizeEngineEvent(ctx context.Context, clusterID uint, item map[string]any, jobCache map[string]*JobInstance, now time.Time) (*EngineEvent, *JobInstance, bool) {
	eventType := strings.ToUpper(strings.TrimSpace(stringField(item, "eventType")))
	if eventType == "" || eventType == EngineEventTypeStainTrace {
		return nil, nil, false
	}
	engineJobID := strings.TrimSpace(stringField(item, "jobId"))
	createdTime := int64Field(item, "createdTime")
	if createdTime <= 0 {
		createdTime = now.UnixMilli()
	}
	payload := JSONMap{}
	for key, value := range item {
		switch key {
		case "eventType", "jobId", "createdTime", "record":
			continue
		}
		payload[key] = value
	}
	jobStatus := strings.ToUpper(strings.TrimSpace(stringField(item, "jobStatus")))
	dedupeTime := createdTime
	if eventType == EngineEventTypeMessageDelayed {
		// 只保留延迟数值；按分钟去重，作为时间线点。
		// Keep only the delay value; dedupe per minute as one timeline point.
		payload = JSONMap{"delay_ms": int64Field(item, "delayTime")}
		dedupeTime = createdTime - createdTime%60000
		createdTime = dedupeTime
	}
	if strings.HasPrefix(eventType, "SCHEMA_CHANGE_") {
		// 时间线只需要表名、子事件与列信息；去掉每层都带的整表 schema（changeAfter），避免超出大小上限。
		// The timeline only needs table, sub-events and columns; drop the full table schema (changeAfter) repeated at
		// every level so the payload stays within the size limit.
		payload = JSONMap(stripPayloadKeys(map[string]any(payload), "changeAfter"))
	}
	// 纵深防御：连接器的表 options 里可能带账号、密码或带密钥的 url，入库前统一脱敏。
	// Defense in depth: connector table options may carry users, passwords or URLs with secrets, so redact before storing.
	payload = JSONMap(audit.RedactDetails(audit.AuditDetails(payload)))
	if encoded, err := json.Marshal(payload); err != nil || len(encoded) > engineEventPayloadMaxBytes {
		payload = scalarOnlyPayload(payload)
		payload["truncated"] = true
	}

	var job *JobInstance
	if engineJobID != "" {
		cached, ok := jobCache[engineJobID]
		if !ok {
			cached = s.findClusterJob(ctx, clusterID, engineJobID)
			jobCache[engineJobID] = cached
		}
		job = cached
	}
	event := &EngineEvent{
		ClusterID:   clusterID,
		EngineJobID: engineJobID,
		EventType:   eventType,
		JobStatus:   jobStatus,
		EventTimeMs: createdTime,
		Payload:     payload,
		ReceivedAt:  now,
	}
	if job != nil {
		id := job.ID
		event.JobInstanceID = &id
	}
	event.DedupeKey = engineEventDedupeKey(clusterID, engineJobID, eventType, jobStatus, dedupeTime, payload)
	return event, job, true
}

// findClusterJob 找到属于该集群的最新作业实例；属于别的集群时视为未知作业，防止跨集群伪造。
// findClusterJob finds the newest job instance of this cluster; jobs of other clusters count as unknown to block
// cross-cluster spoofing.
func (s *Service) findClusterJob(ctx context.Context, clusterID uint, engineJobID string) *JobInstance {
	job, err := s.repo.FindLatestJobInstanceByEngineJobID(ctx, engineJobID)
	if err != nil || job == nil {
		return nil
	}
	if clusterIDForJob(ctx, s, job, map[uint]uint{}) != clusterID {
		return nil
	}
	return job
}

func engineEventDedupeKey(clusterID uint, engineJobID, eventType, jobStatus string, createdTime int64, payload JSONMap) string {
	var canonical []byte
	if eventType == EngineEventTypeMessageDelayed {
		canonical = []byte(fmt.Sprintf("%d|%s|%s|%d", clusterID, engineJobID, eventType, createdTime))
	} else {
		encoded, _ := json.Marshal(payload)
		canonical = []byte(fmt.Sprintf("%d|%s|%s|%s|%d|%s", clusterID, engineJobID, eventType, jobStatus, createdTime, encoded))
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// stripPayloadKeys 递归删除指定键（map 与数组都会深入）。
// stripPayloadKeys recursively removes the given keys from maps and arrays.
func stripPayloadKeys(value map[string]any, keys ...string) map[string]any {
	drop := map[string]bool{}
	for _, key := range keys {
		drop[key] = true
	}
	var walk func(any) any
	walk = func(node any) any {
		switch typed := node.(type) {
		case map[string]any:
			out := make(map[string]any, len(typed))
			for key, item := range typed {
				if drop[key] {
					continue
				}
				out[key] = walk(item)
			}
			return out
		case []any:
			out := make([]any, len(typed))
			for index, item := range typed {
				out[index] = walk(item)
			}
			return out
		default:
			return node
		}
	}
	result, _ := walk(value).(map[string]any)
	return result
}

func scalarOnlyPayload(payload JSONMap) JSONMap {
	out := JSONMap{}
	for key, value := range payload {
		switch typed := value.(type) {
		case string:
			if len(typed) > 512 {
				typed = typed[:512]
			}
			out[key] = typed
		case bool, json.Number, float64, int64, int:
			out[key] = typed
		}
	}
	return out
}

func stringField(item map[string]any, key string) string {
	switch value := item[key].(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", value)
	}
}

func int64Field(item map[string]any, key string) int64 {
	value, ok := anyToInt64(item[key])
	if !ok {
		return 0
	}
	return value
}

// enqueueEventRefresh 登记一次防抖刷新；同一作业在防抖窗口内的多条事件只刷新一次。
// enqueueEventRefresh records a debounced refresh; several events of one job within the window refresh once.
func (s *Service) enqueueEventRefresh(jobID uint) {
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.pending == nil {
		state.pending = map[uint]time.Time{}
	}
	if _, exists := state.pending[jobID]; !exists {
		state.pending[jobID] = state.clock()
	}
}

// takeDueEventRefreshes 取出已过防抖窗口的作业。
// takeDueEventRefreshes pops jobs whose debounce window has passed.
func (s *Service) takeDueEventRefreshes() []uint {
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	now := state.clock()
	var due []uint
	for jobID, at := range state.pending {
		if now.Sub(at) >= engineEventRefreshDebounce {
			due = append(due, jobID)
			delete(state.pending, jobID)
		}
	}
	return due
}

// registerEventExpectation 在提交成功后登记"应收到状态事件"的预期，只针对开启了非终态上报的健康通道。
// registerEventExpectation records an expected status event after a successful submit, only for channels that
// report non-terminal states.
func (s *Service) registerEventExpectation(ctx context.Context, job *JobInstance, clusterID uint) {
	if s == nil || s.repo == nil || job == nil || clusterID == 0 || strings.TrimSpace(job.EngineJobID) == "" {
		return
	}
	channel, err := s.repo.GetEngineEventChannel(ctx, clusterID)
	if err != nil || channel == nil || !channel.Enabled || !channel.ReportNonTerminal || channel.LastEventAt == nil {
		return
	}
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.expectations == nil {
		state.expectations = map[string]eventExpectation{}
	}
	state.expectations[strings.TrimSpace(job.EngineJobID)] = eventExpectation{clusterID: clusterID, deadline: state.clock().Add(engineEventExpectWindow)}
}

func (s *Service) satisfyEventExpectation(engineJobID string) bool {
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	if _, ok := state.expectations[engineJobID]; !ok {
		return false
	}
	delete(state.expectations, engineJobID)
	return true
}

// takeMissedEventExpectations 取出已超时的预期，返回受影响的集群。
// takeMissedEventExpectations pops expired expectations and returns the affected clusters.
func (s *Service) takeMissedEventExpectations() []uint {
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	now := state.clock()
	clusters := map[uint]bool{}
	for engineJobID, expectation := range state.expectations {
		if now.After(expectation.deadline) {
			clusters[expectation.clusterID] = true
			delete(state.expectations, engineJobID)
		}
	}
	out := make([]uint, 0, len(clusters))
	for clusterID := range clusters {
		out = append(out, clusterID)
	}
	return out
}

// recordCDCDelay 用一条延迟样本更新作业的 CDC 延迟摘要（最近值与 1 分钟窗口最大值）。
// 按样本的原始时间单调推进：换主重放的旧样本时间更早，会被自然忽略。
// recordCDCDelay updates a job's CDC delay summary (latest value and 1-minute window max) from one sample. It only
// moves forward by the sample's original time, so older samples replayed after a master failover are ignored.
func (s *Service) recordCDCDelay(jobID uint, delayMs int64, sampleMs int64) {
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.cdcDelays == nil {
		state.cdcDelays = map[uint]*cdcDelayState{}
	}
	current, ok := state.cdcDelays[jobID]
	if ok && sampleMs > 0 && sampleMs <= current.lastSampleMs {
		return
	}
	if !ok || sampleMs-current.windowStartMs > time.Minute.Milliseconds() {
		current = &cdcDelayState{windowStartMs: sampleMs}
		state.cdcDelays[jobID] = current
	}
	current.latestMs = delayMs
	if delayMs > current.maxMs {
		current.maxMs = delayMs
	}
	current.lastSampleMs = sampleMs
	current.at = state.clock()
	// B4 判定器也用这条样本（锁顺序固定为 events → cdcAlerts）。
	// The B4 evaluator takes the same sample (lock order is always events → cdcAlerts).
	s.addCDCDelaySample(jobID, delayMs, sampleMs, current.at)
}

// jobCDCDelay 返回作业的 CDC 延迟摘要，没有记录时返回 nil。
// jobCDCDelay returns a job's CDC delay summary, or nil when none is recorded.
func (s *Service) jobCDCDelay(jobID uint) *JobCDCDelay {
	if s == nil {
		return nil
	}
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	current, ok := state.cdcDelays[jobID]
	if !ok {
		return nil
	}
	return &JobCDCDelay{
		LatestMs: current.latestMs,
		MaxMs:    current.maxMs,
		At:       current.at,
		Fresh:    state.clock().Sub(current.at) <= cdcDelayFreshWindow,
	}
}

// decorateJobRuntime 为响应补充只在内存里的运行时字段（CDC 延迟）。
// decorateJobRuntime fills in-memory runtime fields (CDC delay) for responses.
func (s *Service) decorateJobRuntime(job *JobInstance) {
	if job == nil {
		return
	}
	job.CDCDelay = s.jobCDCDelay(job.ID)
}

// StartEngineEventRuntime 启动事件运行时：防抖刷新、预期检查、B4 延迟告警判定、过期清理。
// StartEngineEventRuntime starts the event runtime: debounced refreshes, expectation checks, B4 delay alert rounds
// and retention purge.
func (s *Service) StartEngineEventRuntime(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	go func() {
		refreshTicker := time.NewTicker(250 * time.Millisecond)
		expectTicker := time.NewTicker(5 * time.Second)
		purgeTicker := time.NewTicker(time.Hour)
		// B4（CDC 延迟告警）每 30 秒一轮；没有注入告警模块时这一轮直接返回。
		// B4 (CDC delay alert) runs every 30s; each round returns at once without the alert module.
		cdcDelayTicker := time.NewTicker(cdcDelayEvalInterval)
		defer cdcDelayTicker.Stop()
		defer refreshTicker.Stop()
		defer expectTicker.Stop()
		defer purgeTicker.Stop()
		limiter := make(chan struct{}, engineEventRefreshConcurrent)
		for {
			select {
			case <-ctx.Done():
				return
			case <-refreshTicker.C:
				for _, jobID := range s.takeDueEventRefreshes() {
					limiter <- struct{}{}
					go func(id uint) {
						defer func() { <-limiter }()
						refreshCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
						defer cancel()
						if _, err := s.RefreshJobByID(refreshCtx, id); err != nil {
							logger.WarnF(refreshCtx, "[Sync] event-triggered refresh failed: job=%d err=%v", id, err)
						}
					}(jobID)
				}
			case <-expectTicker.C:
				now := s.events.clock()
				for _, clusterID := range s.takeMissedEventExpectations() {
					logger.WarnF(ctx, "[Sync] expected engine status event not received in time: cluster=%d", clusterID)
					_ = s.updateEventChannelStatsWithAlert(ctx, clusterID, nil, map[string]any{"expect_missed_at": now})
				}
			case <-cdcDelayTicker.C:
				s.evaluateCDCDelayAlerts(ctx)
			case <-purgeTicker.C:
				if removed, err := s.repo.PurgeEngineEventsBefore(ctx, s.events.clock().Add(-engineEventRetention)); err != nil {
					logger.WarnF(ctx, "[Sync] purge engine events failed: %v", err)
				} else if removed > 0 {
					logger.InfoF(ctx, "[Sync] purged %d expired engine events", removed)
				}
			}
		}
	}()
}

// JobEventListResult 是作业事件时间线的分页结果。
// JobEventListResult is a paged job event timeline.
type JobEventListResult struct {
	Total  int64          `json:"total"`
	Events []*EngineEvent `json:"events"`
}

// ListJobEventsForActor 在权限检查后返回作业事件时间线。
// ListJobEventsForActor returns a job's event timeline after enforcing permission.
func (s *Service) ListJobEventsForActor(ctx context.Context, actor executionapp.Actor, jobID uint, eventTypes []string, page, size int) (*JobEventListResult, error) {
	if _, err := s.getJobInstanceForActor(ctx, actor, jobID); err != nil {
		return nil, err
	}
	if size > 200 {
		size = 200
	}
	normalized := make([]string, 0, len(eventTypes))
	for _, eventType := range eventTypes {
		if trimmed := strings.ToUpper(strings.TrimSpace(eventType)); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	events, total, err := s.repo.ListEngineEventsByJob(ctx, jobID, normalized, page, size)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []*EngineEvent{}
	}
	return &JobEventListResult{Total: total, Events: events}, nil
}
