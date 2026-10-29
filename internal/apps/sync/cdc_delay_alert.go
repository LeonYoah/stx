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
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
	"github.com/LeonYoah/stx/internal/pkg/timex"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

// B4（CDC 延迟告警）判定参数，见 .trellis/spec/backend/sync-alerts.md 的 B4 章节。
// B4 (CDC delay alert) evaluator parameters; see the B4 section of .trellis/spec/backend/sync-alerts.md.
const (
	// cdcDelayEvalInterval 是判定轮次间隔。
	// cdcDelayEvalInterval is the interval between evaluation rounds.
	cdcDelayEvalInterval = 30 * time.Second
	// cdcDelayStaleSample：样本时间早于"现在 − 2 分钟"的不计入 B4（STX 停机后引擎重投的积压旧批次）。
	// cdcDelayStaleSample: samples older than now − 2 minutes are not counted (backlog replayed after STX downtime).
	cdcDelayStaleSample = 2 * time.Minute
	// cdcDelayProgressWindow 内读取条数增长过，没有延迟样本时就判为低（延迟已低于 1 秒）。
	// cdcDelayProgressWindow: a read count that grew within it counts as low when there are no delay samples.
	cdcDelayProgressWindow = 60 * time.Second
	// cdcDelayLowRoundsToRecover 是判定恢复需要的连续低轮数（≥60 秒）。
	// cdcDelayLowRoundsToRecover is the number of consecutive low rounds needed to recover (≥60s).
	cdcDelayLowRoundsToRecover = 2
	// cdcDelayCatchUpPercent：最新一轮中位数 ≤ 基线的 90% 算正在追赶。
	// cdcDelayCatchUpPercent: the latest round median ≤ 90% of the baseline counts as catching up.
	cdcDelayCatchUpPercent = 90
	// cdcDelayCatchUpCap 是追赶期最长不告警的时间；追赶了 30 分钟仍高于阈值，照常告警。
	// cdcDelayCatchUpCap is the longest catch-up suppression; still above the threshold after 30 minutes alerts.
	cdcDelayCatchUpCap = 30 * time.Minute
	// cdcDelayUnknownRefreshInterval 是"无法确认"状态下刷新阶段内容的间隔。
	// cdcDelayUnknownRefreshInterval is how often an "unconfirmable" episode is refreshed.
	cdcDelayUnknownRefreshInterval = 5 * time.Minute
	// cdcDelayUnknownReminderCutoff："无法确认"超过 24 小时后停止提醒（阶段保留）。
	// cdcDelayUnknownReminderCutoff: reminders stop after 24 hours of "unconfirmable" (the episode stays).
	cdcDelayUnknownReminderCutoff = 24 * time.Hour
	// cdcDelayConfigCacheTTL 是任务告警配置的缓存时间。
	// cdcDelayConfigCacheTTL is how long a task alert config is cached.
	cdcDelayConfigCacheTTL = 60 * time.Second
	// cdcDelayMaxSamplesPerRound 限制每个作业每轮保留的样本数（引擎每 reader 每秒最多 0.5 条）。
	// cdcDelayMaxSamplesPerRound bounds samples kept per job per round (the engine sends ≤0.5/s per reader).
	cdcDelayMaxSamplesPerRound = 256
)

// cdcDelayPhase 是作业在 B4 判定器里的阶段。
// cdcDelayPhase is a job's phase in the B4 evaluator.
type cdcDelayPhase int

const (
	// cdcPhaseNone：没有告警。 / No alert.
	cdcPhaseNone cdcDelayPhase = iota
	// cdcPhaseFiring：已发 Fire。 / Fire was sent.
	cdcPhaseFiring
	// cdcPhaseMaybe：STX 启动后第一次看到作业，可能有重启前留下的阶段。
	// cdcPhaseMaybe: first sight after STX start; an episode from before the restart may exist.
	cdcPhaseMaybe
	// cdcPhaseRearm：告警中修改了阈值或持续时间，按新参数重新计时；回落时静默关闭（rule_changed），不发恢复。
	// cdcPhaseRearm: threshold or duration changed while alerting and timing restarted; a drop closes silently
	// (rule_changed) without recovery.
	cdcPhaseRearm
)

// cdcDelaySample 是一条延迟样本：值与 STX 收到它的时间。
// cdcDelaySample is one delay sample: its value and when STX received it.
type cdcDelaySample struct {
	ms int64
	at time.Time
}

// cdcDelayRoundPoint 是窗口里一轮 high 的统计：中位数用于追赶判定，最大值用于展示。
// cdcDelayRoundPoint is one high round in the window: the median drives catch-up, the max is for display.
type cdcDelayRoundPoint struct {
	at     time.Time
	median int64
	max    int64
}

// cdcDelayJobState 是一个作业的 B4 判定状态。
// cdcDelayJobState is one job's B4 evaluation state.
type cdcDelayJobState struct {
	phase            cdcDelayPhase
	thresholdSeconds int
	durationMinutes  int
	aboveSince       time.Time
	history          []cdcDelayRoundPoint
	catchUpSince     time.Time
	lowStreak        int
	lowByReadCount   bool
	unknownSince     time.Time
	unknownRefreshAt time.Time
	pausedSent       bool
	lastDelayMs      int64
	lastSampleAt     time.Time
	readCount        float64
	readCountKnown   bool
	progressAt       time.Time
}

// cdcDelayRoundInput 是一轮判定的输入。
// cdcDelayRoundInput is the input of one evaluation round.
type cdcDelayRoundInput struct {
	samples         []cdcDelaySample
	channelDegraded bool
	readCount       *float64
}

// cdcDelayAction 是判定器要发出的一条信号。
// cdcDelayAction is one signal the evaluator wants to send.
type cdcDelayAction struct {
	kind    syncalert.SignalKind
	reason  string
	detail  syncalert.CDCDelayDetail
	message string
}

type cdcDelayConfigEntry struct {
	cfg *syncalert.TaskConfig
	at  time.Time
}

// cdcDelayAlertState 是判定器的内存状态，零值可用；样本由事件接收端写入，判定轮读取并清空。
// cdcDelayAlertState is the evaluator's in-memory state (zero value ready); the event receiver adds samples and each
// round drains them.
type cdcDelayAlertState struct {
	mu      gosync.Mutex
	samples map[uint][]cdcDelaySample
	jobs    map[uint]*cdcDelayJobState
	configs map[uint]cdcDelayConfigEntry
}

// addCDCDelaySample 把一条样本计入本轮；sampleMs 是引擎记录的时间，早于"现在 − 2 分钟"的不计入。
// addCDCDelaySample adds one sample to the current round; sampleMs is the engine time and samples older than
// now − 2 minutes are skipped.
func (s *Service) addCDCDelaySample(jobID uint, delayMs, sampleMs int64, now time.Time) {
	if s == nil || jobID == 0 {
		return
	}
	if sampleMs > 0 && now.Sub(time.UnixMilli(sampleMs)) > cdcDelayStaleSample {
		return
	}
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.samples == nil {
		state.samples = map[uint][]cdcDelaySample{}
	}
	if len(state.samples[jobID]) >= cdcDelayMaxSamplesPerRound {
		return
	}
	state.samples[jobID] = append(state.samples[jobID], cdcDelaySample{ms: delayMs, at: now})
}

// invalidateCDCDelayConfig 让任务告警配置缓存失效（任务告警配置被修改时调用）。
// invalidateCDCDelayConfig drops the cached task alert config (called when it is changed).
func (s *Service) invalidateCDCDelayConfig(taskID uint) {
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.configs, taskID)
}

// forgetCDCDelayJob 清掉作业的判定状态和样本。
// forgetCDCDelayJob drops a job's evaluation state and samples.
func (s *Service) forgetCDCDelayJob(jobID uint) {
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.jobs, jobID)
	delete(state.samples, jobID)
}

// cdcDelayTaskConfig 返回任务告警配置（按任务缓存 60 秒）；读取失败返回 nil 和错误，不改变判定状态。
// cdcDelayTaskConfig returns the task alert config (cached per task for 60s); a failed read returns nil and the error
// without touching evaluation state.
func (s *Service) cdcDelayTaskConfig(ctx context.Context, taskID uint, now time.Time) (*syncalert.TaskConfig, error) {
	state := &s.cdcAlerts
	state.mu.Lock()
	if entry, ok := state.configs[taskID]; ok && now.Sub(entry.at) < cdcDelayConfigCacheTTL {
		state.mu.Unlock()
		return entry.cfg, nil
	}
	state.mu.Unlock()
	cfg, err := s.alertStore.GetTaskAlertConfig(ctx, taskID)
	if err != nil {
		return nil, err
	}
	state.mu.Lock()
	if state.configs == nil {
		state.configs = map[uint]cdcDelayConfigEntry{}
	}
	state.configs[taskID] = cdcDelayConfigEntry{cfg: cfg, at: now}
	state.mu.Unlock()
	return cfg, nil
}

// cdcDelayChannel 是一轮内某个集群的事件通道判断结果。
// cdcDelayChannel is the per-round event channel verdict of one cluster.
type cdcDelayChannel struct {
	unavailable bool
	degraded    bool
}

// evaluateCDCDelayAlerts 跑一轮 B4 判定：候选作业为运行中、非批、非数据预览且任务勾选了 B4 的作业。
// evaluateCDCDelayAlerts runs one B4 round over running, non-batch, non-preview jobs whose task selects B4.
func (s *Service) evaluateCDCDelayAlerts(ctx context.Context) {
	if s == nil || s.repo == nil || s.alertReporter == nil || s.alertStore == nil {
		return
	}
	now := s.events.clock()
	jobs, err := s.repo.ListNonTerminalJobInstances(ctx, 1000)
	if err != nil {
		logger.WarnF(ctx, "[Sync] list running jobs for CDC delay alerts failed: %v", err)
		return
	}
	running := map[uint]bool{}
	clusterCache := map[uint]uint{}
	channels := map[uint]cdcDelayChannel{}
	for _, job := range jobs {
		if job == nil || job.Status != JobStatusRunning || job.RunType == RunTypePreview || job.TaskID == 0 || jobIsBatch(job) {
			continue
		}
		running[job.ID] = true
		cfg, err := s.cdcDelayTaskConfig(ctx, job.TaskID, now)
		if err != nil {
			// 读不到配置时这一轮跳过，样本留到下一轮。 / Skip this round when the config is unreadable.
			logger.WarnF(ctx, "[Sync] load task alert config for CDC delay failed: task=%d err=%v", job.TaskID, err)
			continue
		}
		clusterID := clusterIDForJob(ctx, s, job, clusterCache)
		channel, ok := channels[clusterID]
		if !ok {
			channel = s.cdcDelayChannelVerdict(ctx, clusterID)
			channels[clusterID] = channel
		}
		for _, action := range s.stepCDCDelayJob(job, cfg, channel, now) {
			s.emitCDCDelayAction(ctx, job, action)
		}
	}
	// 不再运行的作业：清掉状态（结束时的静默关闭由 reportJobTransition 发出）。
	// Jobs no longer running drop their state (the silent close on end comes from reportJobTransition).
	state := &s.cdcAlerts
	state.mu.Lock()
	for jobID := range state.jobs {
		if !running[jobID] {
			delete(state.jobs, jobID)
		}
	}
	for jobID := range state.samples {
		if !running[jobID] {
			delete(state.samples, jobID)
		}
	}
	state.mu.Unlock()
}

// cdcDelayChannelVerdict 判断集群事件通道：关闭、待重启或集群版本明确不支持时不可用；版本取不到时不当作不支持。
// cdcDelayChannelVerdict judges the cluster event channel: off, pending restart, or a version known not to support it
// is unavailable; an unknown version is not treated as unsupported.
func (s *Service) cdcDelayChannelVerdict(ctx context.Context, clusterID uint) cdcDelayChannel {
	if clusterID == 0 {
		return cdcDelayChannel{unavailable: true}
	}
	channel, err := s.repo.GetEngineEventChannel(ctx, clusterID)
	if err != nil && err != ErrEventChannelNotFound {
		// 读不到通道状态：当作降级（无法确认），不关闭阶段。 / Unreadable: treat as degraded, never close.
		return cdcDelayChannel{degraded: true}
	}
	if err == ErrEventChannelNotFound {
		channel = nil
	}
	switch eventChannelState(channel) {
	case EventChannelStateDisabled, EventChannelStatePendingRestart:
		return cdcDelayChannel{unavailable: true}
	case EventChannelStateDegraded:
		return cdcDelayChannel{degraded: true}
	}
	if version := s.clusterVersion(ctx, clusterID); version != "" && !seatunnel.SupportsJobStateEvents(version) {
		return cdcDelayChannel{unavailable: true}
	}
	return cdcDelayChannel{}
}

// stepCDCDelayJob 处理一个作业的一轮：先处理规则去掉、通道不可用，再交给状态机。
// stepCDCDelayJob handles one job for one round: rule removal and channel loss first, then the state machine.
func (s *Service) stepCDCDelayJob(job *JobInstance, cfg *syncalert.TaskConfig, channel cdcDelayChannel, now time.Time) []cdcDelayAction {
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.jobs == nil {
		state.jobs = map[uint]*cdcDelayJobState{}
	}
	samples := state.samples[job.ID]
	delete(state.samples, job.ID)
	current, exists := state.jobs[job.ID]

	enabled := cfg != nil && cfg.Configured && cfg.Enabled && cfg.HasRule(syncalert.RuleCDCDelayHigh)
	if !enabled {
		// 去掉 B4 或停用：静默关闭（monitoring 在保存策略时也会关闭，这里重复也没有影响）。
		// B4 removed or disabled: close silently (monitoring also closes on save; a duplicate is harmless).
		delete(state.jobs, job.ID)
		if exists && current.phase != cdcPhaseNone {
			return []cdcDelayAction{{kind: syncalert.KindResolve, reason: syncalert.ResolveReasonRuleRemoved}}
		}
		return nil
	}
	if channel.unavailable {
		// 通道关闭、待重启或集群不支持：静默关闭；第一次看到的作业也发一次，关掉重启前留下的阶段。
		// Channel off, pending restart or unsupported: close silently; a first-seen job sends once too so episodes
		// from before a restart are closed.
		if !exists || current.phase != cdcPhaseNone {
			state.jobs[job.ID] = &cdcDelayJobState{phase: cdcPhaseNone, thresholdSeconds: cfg.CDCDelayThresholdSeconds, durationMinutes: cfg.CDCDelayDurationMinutes}
			return []cdcDelayAction{{kind: syncalert.KindResolve, reason: syncalert.ResolveReasonChannelOff}}
		}
		return nil
	}
	if !exists {
		current = &cdcDelayJobState{phase: cdcPhaseMaybe}
		state.jobs[job.ID] = current
	}
	input := cdcDelayRoundInput{samples: samples, channelDegraded: channel.degraded}
	if metrics, ok := job.ResultPreview["metrics"].(map[string]interface{}); ok {
		input.readCount = metricNumber(metrics, "SourceReceivedCount")
	}
	threshold := cfg.CDCDelayThresholdSeconds
	if threshold <= 0 {
		threshold = syncalert.DefaultCDCDelayThresholdSeconds
	}
	duration := cfg.CDCDelayDurationMinutes
	if duration <= 0 {
		duration = syncalert.DefaultCDCDelayDurationMinutes
	}
	actions := current.step(now, threshold, duration, input)
	for i := range actions {
		actions[i].message = cdcDelayMessage(actions[i], job)
	}
	return actions
}

// emitCDCDelayAction 把判定结果转成信号发出。
// emitCDCDelayAction turns an evaluator action into a signal.
func (s *Service) emitCDCDelayAction(ctx context.Context, job *JobInstance, action cdcDelayAction) {
	signal, ok := s.alertSignalForJob(ctx, action.kind, syncalert.RuleCDCDelayHigh, job, nil, action.message)
	if !ok {
		return
	}
	if action.kind == syncalert.KindResolve {
		signal.ResolveScope = syncalert.ResolveCDCDelay
		signal.ResolveReason = action.reason
	}
	if action.detail.State != "" {
		detail := action.detail
		signal.CDCDelay = &detail
	}
	s.reportAlert(signal)
}

// reportCDCDelayJobEnded 在实时作业进入终态时静默关闭它的 B4 阶段并清掉判定状态。
// reportCDCDelayJobEnded silently closes a streaming job's B4 episode when it ends and drops its evaluation state.
func (s *Service) reportCDCDelayJobEnded(ctx context.Context, instance *JobInstance) {
	if instance == nil || jobIsBatch(instance) {
		return
	}
	s.forgetCDCDelayJob(instance.ID)
	if signal, ok := s.alertSignalForJob(ctx, syncalert.KindResolve, syncalert.RuleCDCDelayHigh, instance, nil, ""); ok {
		signal.ResolveScope = syncalert.ResolveCDCDelay
		signal.ResolveReason = syncalert.ResolveReasonJobEnded
		s.reportAlert(signal)
	}
}

// step 是 B4 状态机（纯函数，不做 I/O）：按本轮输入分类为 high / low / unknown，并决定要发的信号。
// step is the B4 state machine (pure, no I/O): it classifies the round as high / low / unknown and decides which
// signals to send.
func (st *cdcDelayJobState) step(now time.Time, thresholdSeconds, durationMinutes int, in cdcDelayRoundInput) []cdcDelayAction {
	// 告警中修改阈值或持续时间：重新计时；已经在告警（或可能在告警）的转为 rearm，回落时不发"恢复"。
	// Threshold or duration changed while alerting: restart timing; an alerting (or maybe alerting) job becomes rearm
	// so a drop never sends "recovered".
	if st.thresholdSeconds != 0 && (st.thresholdSeconds != thresholdSeconds || st.durationMinutes != durationMinutes) {
		st.resetTiming()
		st.lowStreak = 0
		st.lowByReadCount = false
		st.unknownSince = time.Time{}
		st.unknownRefreshAt = time.Time{}
		st.pausedSent = false
		if st.phase == cdcPhaseFiring || st.phase == cdcPhaseMaybe {
			st.phase = cdcPhaseRearm
		}
	}
	st.thresholdSeconds = thresholdSeconds
	st.durationMinutes = durationMinutes

	if in.readCount != nil {
		if st.readCountKnown && *in.readCount > st.readCount {
			st.progressAt = now
		}
		st.readCount = *in.readCount
		st.readCountKnown = true
	}

	thresholdMs := int64(thresholdSeconds) * 1000
	duration := time.Duration(durationMinutes) * time.Minute
	switch {
	case len(in.samples) > 0:
		values := make([]int64, 0, len(in.samples))
		var maxMs int64
		for _, sample := range in.samples {
			values = append(values, sample.ms)
			if sample.ms > maxMs {
				maxMs = sample.ms
			}
		}
		last := in.samples[len(in.samples)-1]
		st.lastDelayMs = last.ms
		st.lastSampleAt = last.at
		median := medianInt64(values)
		if median >= thresholdMs {
			firstAbove := now
			for _, sample := range in.samples {
				if sample.ms >= thresholdMs {
					firstAbove = sample.at
					break
				}
			}
			return st.onHigh(now, duration, firstAbove, cdcDelayRoundPoint{at: now, median: median, max: maxMs})
		}
		return st.onLow(syncalert.CDCDelayRecoveryBySamples)
	case in.channelDegraded:
		return st.onUnknown(now, syncalert.CDCDelayUnknownChannelDegraded)
	case !st.progressAt.IsZero() && now.Sub(st.progressAt) <= cdcDelayProgressWindow:
		return st.onLow(syncalert.CDCDelayRecoveryByReadCount)
	default:
		return st.onUnknown(now, syncalert.CDCDelayUnknownNoProgress)
	}
}

func (st *cdcDelayJobState) resetTiming() {
	st.aboveSince = time.Time{}
	st.history = nil
	st.catchUpSince = time.Time{}
}

// onHigh 处理高于阈值的一轮。
// onHigh handles a round above the threshold.
func (st *cdcDelayJobState) onHigh(now time.Time, duration time.Duration, firstAbove time.Time, point cdcDelayRoundPoint) []cdcDelayAction {
	st.lowStreak = 0
	st.lowByReadCount = false
	st.unknownSince = time.Time{}
	st.unknownRefreshAt = time.Time{}
	st.pausedSent = false
	if st.aboveSince.IsZero() {
		st.aboveSince = firstAbove
	}
	st.history = append(st.history, point)
	// 只保留窗口 [now−M, now] 内的轮次；基线取窗口里最早一轮。
	// Keep only rounds within [now−M, now]; the baseline is the earliest one in the window.
	cut := 0
	for cut < len(st.history)-1 && now.Sub(st.history[cut].at) > duration {
		cut++
	}
	st.history = st.history[cut:]

	refresh := []cdcDelayAction{{kind: syncalert.KindRefresh, detail: st.highDetail()}}
	if st.phase == cdcPhaseFiring {
		return refresh
	}
	if now.Sub(st.aboveSince) < duration {
		if st.phase == cdcPhaseMaybe || st.phase == cdcPhaseRearm {
			// 重启或改参数后还在计时：已有阶段只刷新为当前值，不要一直用旧值提醒。
			// Still timing after a restart or parameter change: an existing episode is refreshed with the current
			// value instead of reminding with a stale one.
			return refresh
		}
		return nil
	}
	if st.catchingUp() {
		if st.catchUpSince.IsZero() {
			st.catchUpSince = now
		}
		if now.Sub(st.catchUpSince) < cdcDelayCatchUpCap {
			if st.phase == cdcPhaseMaybe || st.phase == cdcPhaseRearm {
				return refresh
			}
			return nil
		}
	}
	st.phase = cdcPhaseFiring
	st.catchUpSince = time.Time{}
	return []cdcDelayAction{{kind: syncalert.KindFire, detail: st.highDetail()}}
}

// catchingUp 判断是否正在追赶：窗口内至少两轮，最新一轮的中位数 ≤ 最早一轮中位数的 90%。用中位数而不是最大值做基线，
// 一轮里个别大事务尖峰不会把基线抬高，导致误判为"在追赶"而漏报。
// catchingUp reports catch-up: at least two rounds in the window and the latest round median ≤ 90% of the earliest
// round median. Medians rather than maxima keep a single big-transaction spike from inflating the baseline and
// hiding a real alert as "catching up".
func (st *cdcDelayJobState) catchingUp() bool {
	if len(st.history) < 2 {
		return false
	}
	baseline := st.history[0].median
	latest := st.history[len(st.history)-1].median
	return baseline > 0 && latest*100 <= baseline*cdcDelayCatchUpPercent
}

// onLow 处理低于阈值的一轮；连续两轮后按阶段发恢复或静默关闭。
// onLow handles a round below the threshold; after two in a row it recovers or closes silently depending on phase.
func (st *cdcDelayJobState) onLow(basis string) []cdcDelayAction {
	st.resetTiming()
	st.unknownSince = time.Time{}
	st.unknownRefreshAt = time.Time{}
	st.pausedSent = false
	st.lowStreak++
	if basis == syncalert.CDCDelayRecoveryByReadCount {
		st.lowByReadCount = true
	}
	if st.lowStreak < cdcDelayLowRoundsToRecover {
		return nil
	}
	recoveryBasis := syncalert.CDCDelayRecoveryBySamples
	if st.lowByReadCount {
		recoveryBasis = syncalert.CDCDelayRecoveryByReadCount
	}
	phase := st.phase
	st.phase = cdcPhaseNone
	switch phase {
	case cdcPhaseFiring, cdcPhaseMaybe:
		detail := st.baseDetail(syncalert.CDCDelayStateRecovered)
		detail.RecoveryBasis = recoveryBasis
		return []cdcDelayAction{{kind: syncalert.KindResolve, reason: syncalert.ResolveReasonRecovered, detail: detail}}
	case cdcPhaseRearm:
		return []cdcDelayAction{{kind: syncalert.KindResolve, reason: syncalert.ResolveReasonRuleChanged}}
	default:
		return nil
	}
}

// onUnknown 处理收不到延迟数据、也无法确认恢复的一轮：刚进入时和之后每 5 分钟刷新一次阶段，超过 24 小时停止提醒。
// onUnknown handles a round without delay data where recovery cannot be confirmed: the episode is refreshed on entry
// and every 5 minutes after; reminders stop after 24 hours.
func (st *cdcDelayJobState) onUnknown(now time.Time, reason string) []cdcDelayAction {
	st.lowStreak = 0
	st.lowByReadCount = false
	if st.unknownSince.IsZero() {
		st.unknownSince = now
	}
	if st.phase == cdcPhaseNone {
		return nil
	}
	paused := now.Sub(st.unknownSince) >= cdcDelayUnknownReminderCutoff
	due := st.unknownRefreshAt.IsZero() || now.Sub(st.unknownRefreshAt) >= cdcDelayUnknownRefreshInterval || (paused && !st.pausedSent)
	if !due {
		return nil
	}
	st.unknownRefreshAt = now
	if paused {
		st.pausedSent = true
	}
	detail := st.baseDetail(syncalert.CDCDelayStateUnknown)
	detail.UnknownReason = reason
	detail.UnknownSince = st.unknownSince
	detail.RemindersPaused = paused
	return []cdcDelayAction{{kind: syncalert.KindRefresh, detail: detail}}
}

func (st *cdcDelayJobState) baseDetail(state string) syncalert.CDCDelayDetail {
	return syncalert.CDCDelayDetail{
		State:            state,
		CurrentDelayMs:   st.lastDelayMs,
		ThresholdSeconds: st.thresholdSeconds,
		DurationMinutes:  st.durationMinutes,
		AboveSince:       st.aboveSince,
		LastSampleAt:     st.lastSampleAt,
	}
}

func (st *cdcDelayJobState) highDetail() syncalert.CDCDelayDetail {
	detail := st.baseDetail(syncalert.CDCDelayStateHigh)
	for _, point := range st.history {
		if point.max > detail.WindowMaxDelayMs {
			detail.WindowMaxDelayMs = point.max
		}
	}
	return detail
}

func medianInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// cdcDelayMessage 生成告警中心与通知里的中文说明（阶段的 LastMessage）。
// cdcDelayMessage builds the Chinese note shown in the alert center and notifications (the episode LastMessage).
func cdcDelayMessage(action cdcDelayAction, job *JobInstance) string {
	detail := action.detail
	switch {
	case action.kind == syncalert.KindResolve && action.reason == syncalert.ResolveReasonRuleChanged:
		return "告警参数已修改，按新参数重新计时后读取延迟未超标，自动关闭（不是恢复通知）"
	case action.kind == syncalert.KindResolve && action.reason == syncalert.ResolveReasonRuleRemoved:
		return "任务告警配置已去掉 CDC 延迟规则或已停用，自动关闭"
	case action.kind == syncalert.KindResolve && action.reason == syncalert.ResolveReasonChannelOff:
		return "集群事件通道已关闭、待重启或不支持，无法继续判断 CDC 延迟，自动关闭"
	case detail.State == syncalert.CDCDelayStateRecovered:
		if detail.RecoveryBasis == syncalert.CDCDelayRecoveryByReadCount {
			return "根据读取条数推断已恢复：最近没有延迟事件，读取条数在增长，读取延迟应已低于 1 秒"
		}
		return fmt.Sprintf("读取延迟已连续两轮低于阈值 %s（最近一次 %s）", syncalert.FormatDelayZH(int64(detail.ThresholdSeconds)*1000), syncalert.FormatDelayZH(detail.CurrentDelayMs))
	case detail.State == syncalert.CDCDelayStateUnknown:
		var b strings.Builder
		if detail.UnknownReason == syncalert.CDCDelayUnknownChannelDegraded {
			b.WriteString("事件通道降级，最近没有收到延迟数据，无法确认读取延迟是否恢复")
		} else {
			b.WriteString("最近 1 分钟没有收到延迟数据，读取条数也没有增长，无法确认读取延迟是否恢复")
		}
		b.WriteString(fmt.Sprintf("（自 %s 起）", formatCDCDelayTime(detail.UnknownSince)))
		if !detail.LastSampleAt.IsZero() {
			b.WriteString(fmt.Sprintf("；最后一次读取延迟 %s（%s）", syncalert.FormatDelayZH(detail.CurrentDelayMs), formatCDCDelayTime(detail.LastSampleAt)))
		}
		b.WriteString("。可能原因：源库暂时没有写入；作业读取卡住；事件通道中断。")
		if link := cdcDelayJobLink(job); link != "" {
			b.WriteString("作业：" + link)
		}
		if detail.RemindersPaused {
			b.WriteString("。无法确认已超过 24 小时，已停止提醒，告警保留，请人工确认后关闭。")
		}
		return b.String()
	case detail.State == syncalert.CDCDelayStateHigh:
		elapsed := 0
		if !detail.AboveSince.IsZero() {
			elapsed = int(detail.LastSampleAt.Sub(detail.AboveSince) / time.Minute)
			if elapsed < 0 {
				elapsed = 0
			}
		}
		return fmt.Sprintf("CDC 读取延迟 %s（近 %d 分钟最大 %s），阈值 %s，已持续 %d 分钟",
			syncalert.FormatDelayZH(detail.CurrentDelayMs), detail.DurationMinutes, syncalert.FormatDelayZH(detail.WindowMaxDelayMs),
			syncalert.FormatDelayZH(int64(detail.ThresholdSeconds)*1000), elapsed)
	default:
		return ""
	}
}

// formatCDCDelayTime 用界面时区格式化时间，带日期和时区，例如 2026-10-29 10:21:30 +08:00 (Asia/Shanghai)。
// formatCDCDelayTime formats a time in the display zone with date and zone, e.g.
// 2026-10-29 10:21:30 +08:00 (Asia/Shanghai).
func formatCDCDelayTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s (%s)", value.In(timex.DisplayLocation()).Format("2006-01-02 15:04:05 -07:00"), timex.DisplayTimezone())
}

// cdcDelayJobLink 返回作业深链 {console_url}/sync?task=&job=；没有配置控制台地址时返回空（与通知里的链接一致）。
// cdcDelayJobLink returns the job deep link {console_url}/sync?task=&job=; empty without a console URL (same as the
// notification link).
func cdcDelayJobLink(job *JobInstance) string {
	if job == nil || job.TaskID == 0 {
		return ""
	}
	base := config.GetConsoleURL()
	if base == "" {
		return ""
	}
	query := url.Values{}
	query.Set("task", strconv.FormatUint(uint64(job.TaskID), 10))
	query.Set("job", strconv.FormatUint(uint64(job.ID), 10))
	return base + "/sync?" + query.Encode()
}
