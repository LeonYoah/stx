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
	// cdcDelayStaleSample：样本的时间差（STX 收到时间 − 引擎时间）比 max(该作业最近的时间差基线, 0) 大 2 分钟以上的，
	// 是重投的积压旧样本，不计入 B4。基线见 cdcDelayOffsetWindow。
	// cdcDelayStaleSample: a sample whose offset (STX receive time − engine time) exceeds max(the job's recent offset
	// baseline, 0) by more than 2 minutes is a replayed old sample and is not counted. See cdcDelayOffsetWindow.
	cdcDelayStaleSample = 2 * time.Minute
	// cdcDelayOffsetWindow：时间差基线取最近 10 分钟内计入样本的最大时间差；10 分钟没有计入的样本时基线过期，下一批样本
	// 重新建立基线。时钟跳变、个别时间不准的样本不会让后面的样本一直被当成旧样本。
	// cdcDelayOffsetWindow: the offset baseline is the largest offset among samples counted in the last 10 minutes;
	// with none counted for 10 minutes it expires and the next batch starts a new one, so a clock jump or one bad
	// timestamp never makes every later sample look stale.
	cdcDelayOffsetWindow = 10 * time.Minute
	// cdcDelayOffsetAdoptAfter：被判为旧样本的情况持续 5 分钟以上（不是一次性的积压重投，而是某个 worker 的时钟长期偏慢），
	// 改为计入并抬高基线，多节点时钟差不会让卡住的 reader 一直被丢掉。
	// cdcDelayOffsetAdoptAfter: when samples keep being judged stale for 5 minutes (not a one-off replay but a worker
	// whose clock runs behind), they are counted and raise the baseline, so inter-node skew never hides a stuck reader.
	cdcDelayOffsetAdoptAfter = 5 * time.Minute
	// cdcDelayRefreshResendInterval：内容没变的 Refresh 最多隔 5 分钟也重发一次，被丢弃的 Refresh 能自己补上。
	// cdcDelayRefreshResendInterval: a Refresh with unchanged content is still resent every 5 minutes so a dropped one
	// heals itself.
	cdcDelayRefreshResendInterval = 5 * time.Minute
	// cdcDelaySkewWarnInterval：引擎时间与 STX 收到时间相差超过 cdcDelayStaleSample 时，每个作业最多每 10 分钟打一条 warning。
	// cdcDelaySkewWarnInterval: when engine time and STX receive time differ by more than cdcDelayStaleSample, warn at
	// most once per job every 10 minutes.
	cdcDelaySkewWarnInterval = 10 * time.Minute
	// cdcDelayProgressWindow 内读取条数增长过，没有延迟样本时就判为低（延迟已低于 1 秒）。
	// cdcDelayProgressWindow: a read count that grew within it counts as low when there are no delay samples.
	cdcDelayProgressWindow = 60 * time.Second
	// cdcDelayLowRoundsToRecover 是判定恢复需要的连续低轮数（≥60 秒）。
	// cdcDelayLowRoundsToRecover is the number of consecutive low rounds needed to recover (≥60s).
	cdcDelayLowRoundsToRecover = 2
	// cdcDelayCatchUpPercent：最新一轮中位数 ≤ 基线的 90%，且最大值也 ≤ 基线最大值的 90%，才算正在追赶。
	// cdcDelayCatchUpPercent: catching up needs the latest round median ≤ 90% of the baseline median and the latest
	// round max ≤ 90% of the baseline max.
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

// cdcDelayRoundPoint 是窗口里一轮 high 的统计：中位数和最大值都用于追赶判定，最大值也用于展示。
// cdcDelayRoundPoint is one high round in the window: median and max both drive catch-up; the max is also displayed.
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
	// nonHighRounds 是自上一轮 high 以来连续的非 high 轮数（low 和无法确认都算）。不在告警中时，两轮 high 之间隔了
	// 两轮以上，且后一轮的最大延迟 < 阈值 + 两轮 high 的间隔（now − lastHighAt），超标计时从后一轮重新开始：孤立的
	// 尖峰不会借着中间的空白时间累计到持续时间。后一轮的最大延迟 ≥ 阈值 + 间隔时，说明间隔内延迟从未低于阈值（见 onHigh），
	// 照常累计。
	// nonHighRounds counts consecutive non-high rounds (low and unconfirmable alike) since the last high one. When not
	// alerting, two high rounds separated by two or more of them restart timing at the later one unless the later
	// round's max delay ≥ threshold + the time between them (now − lastHighAt), so isolated spikes never add up to the
	// duration across the gap. A max delay ≥ threshold + gap proves the delay never fell below the threshold during
	// the gap (see onHigh), so timing continues.
	nonHighRounds int
	// lastHighAt 是最近一轮 high 的判定时间。 / lastHighAt is when the latest high round was evaluated.
	lastHighAt       time.Time
	lowByReadCount   bool
	unknownSince     time.Time
	unknownRefreshAt time.Time
	pausedSent       bool
	lastDelayMs      int64
	lastSampleAt     time.Time
	readCount        float64
	readCountKnown   bool
	progressAt       time.Time
	// lastContentKey 是最近一次发出的 Fire / Refresh 的展示内容；内容没变的 Refresh 不再发，避免占满告警队列。
	// lastContentKey is the display content of the latest Fire / Refresh sent; a Refresh with unchanged content is
	// skipped so it never fills the alert queue.
	lastContentKey string
	// lastSentAt / lastSentState / lastSentPaused 记录最近一次被接收的 Fire / Refresh：用于每 5 分钟重发和判断状态变化。
	// lastSentAt / lastSentState / lastSentPaused record the latest accepted Fire / Refresh, for the 5-minute resend
	// and for spotting state changes.
	lastSentAt     time.Time
	lastSentState  string
	lastSentPaused bool
}

// cdcDelayRoundInput 是一轮判定的输入。
// cdcDelayRoundInput is the input of one evaluation round.
type cdcDelayRoundInput struct {
	samples         []cdcDelaySample
	channelDegraded bool
	readCount       *float64
	// staleOnly：本轮收到过延迟样本，但全部按时间差判为重投的旧样本、一条都没计入（例如引擎时钟回拨）。这一轮按"无数据"
	// 处理（无法确认），不按读取条数推断为低：延迟事件还在来，说明读取延迟仍 ≥ 1 秒。
	// staleOnly: delay samples arrived this round but every one was judged a replayed old sample by its offset and
	// none counted (e.g. the engine clock stepped back). The round is treated as no data (unconfirmable) and never
	// inferred as low from the read count: delay events still arriving means the read delay is still ≥ 1s.
	staleOnly bool
}

// cdcDelayAction 是判定器要发出的一条信号。
// cdcDelayAction is one signal the evaluator wants to send.
type cdcDelayAction struct {
	kind    syncalert.SignalKind
	reason  string
	detail  syncalert.CDCDelayDetail
	message string
	// key 是展示内容（cdcDelayContentKey）；stateChange 表示这条 Refresh 改变了阶段状态。
	// key is the display content (cdcDelayContentKey); stateChange marks a Refresh that changes the episode state.
	key         string
	stateChange bool
}

// cdcDelayOffsetState 是一个作业的时间差基线。
// cdcDelayOffsetState is one job's offset baseline.
type cdcDelayOffsetState struct {
	// points 是按判定轮（30 秒）分桶的最大时间差。 / points are the max offsets per 30-second bucket.
	points []cdcDelayOffsetPoint
	// staleSince / staleLastAt：这一段连续被判为旧样本的开始和最近一次时间；持续 cdcDelayOffsetAdoptAfter 后改为计入。
	// staleSince / staleLastAt: when the current run of stale verdicts started and was last seen; after
	// cdcDelayOffsetAdoptAfter such samples are counted.
	staleSince  time.Time
	staleLastAt time.Time
}

type cdcDelayOffsetPoint struct {
	at       time.Time
	offsetMs int64
}

// baseline 返回窗口内的最大时间差；窗口为空（基线过期）时返回 false。
// baseline returns the largest offset in the window, or false when the window is empty (the baseline expired).
func (o *cdcDelayOffsetState) baseline(now time.Time) (int64, bool) {
	cut := 0
	for cut < len(o.points) && now.Sub(o.points[cut].at) > cdcDelayOffsetWindow {
		cut++
	}
	o.points = o.points[cut:]
	if len(o.points) == 0 {
		return 0, false
	}
	largest := o.points[0].offsetMs
	for _, p := range o.points[1:] {
		if p.offsetMs > largest {
			largest = p.offsetMs
		}
	}
	return largest, true
}

func (o *cdcDelayOffsetState) add(now time.Time, offsetMs int64) {
	if n := len(o.points); n > 0 && now.Sub(o.points[n-1].at) < cdcDelayEvalInterval {
		if offsetMs > o.points[n-1].offsetMs {
			o.points[n-1].offsetMs = offsetMs
		}
		return
	}
	o.points = append(o.points, cdcDelayOffsetPoint{at: now, offsetMs: offsetMs})
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
	// configGen 在配置失效时加一；读配置期间被改过的结果不进缓存，避免缓存住改之前的旧配置。
	// configGen is bumped on invalidation; a read that raced with a change is not cached, so the pre-change config
	// never sticks in the cache.
	configGen map[uint]uint64
	// offsets 是每个作业最近计入样本的时间差（STX 收到时间 − 引擎时间），用于判断重投的旧样本。
	// offsets holds each job's recent offsets (STX receive time − engine time) of counted samples, used to spot
	// replayed old samples.
	offsets map[uint]*cdcDelayOffsetState
	// skewWarnAt 是每个作业最近一次时钟差 warning 的时间（限频）。
	// skewWarnAt is when each job last logged a clock skew warning (rate limit).
	skewWarnAt map[uint]time.Time
	// staleDropped 是每个作业本轮被判为旧样本、没有计入的样本数；判定轮读取并清空。
	// staleDropped counts each job's samples judged stale (not counted) in the current round; each round drains it.
	staleDropped map[uint]int
}

// cdcDelaySkewWarn 打印引擎时钟与 STX 时钟相差过大的 warning；测试里会替换。
// cdcDelaySkewWarn logs a warning about a large engine/STX clock difference; tests replace it.
var cdcDelaySkewWarn = func(jobID uint, skew time.Duration) {
	logger.WarnF(context.Background(), "[Sync] CDC delay samples of job %d carry engine times %s away from STX receive time "+
		"(clock skew between SeaTunnel and STX, or a replayed backlog); B4 keeps judging by receive time / "+
		"READER_MESSAGE_DELAYED 的引擎时间与 STX 收到时间相差 %s（时钟不同步或积压重投），B4 按收到时间继续判断",
		jobID, skew.Round(time.Second), skew.Round(time.Second))
}

// noteCDCDelayBatch 在一批事件的样本写入之前调用：作业还没有时间差基线（或已过期）时，用这一批里最新的引擎时间建立
// 基线，同一批里更早的重投样本才能被识别。
// noteCDCDelayBatch is called before a batch's samples are added: when the job has no offset baseline (or it
// expired), the batch's newest engine time starts one, so older replayed samples in the same batch are recognised.
func (s *Service) noteCDCDelayBatch(jobID uint, newestSampleMs int64, now time.Time) {
	if s == nil || jobID == 0 || newestSampleMs <= 0 {
		return
	}
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	o := state.offsetState(jobID)
	if _, ok := o.baseline(now); !ok {
		o.add(now, now.Sub(time.UnixMilli(newestSampleMs)).Milliseconds())
	}
}

func (state *cdcDelayAlertState) offsetState(jobID uint) *cdcDelayOffsetState {
	if state.offsets == nil {
		state.offsets = map[uint]*cdcDelayOffsetState{}
	}
	o := state.offsets[jobID]
	if o == nil {
		o = &cdcDelayOffsetState{}
		state.offsets[jobID] = o
	}
	return o
}

// acceptCDCDelayOffset 按时间差判断样本是否计入：不超过 max(基线, 0) + 2 分钟的计入并更新基线；超过的是重投的旧样本，
// 但这种情况持续 cdcDelayOffsetAdoptAfter 以上（某个 worker 时钟长期偏慢）时改为计入。没有基线时直接计入并建立基线。
// acceptCDCDelayOffset judges a sample by its offset: within max(baseline, 0) + 2 minutes it counts and feeds the
// baseline; beyond that it is a replayed old sample, unless that keeps happening for cdcDelayOffsetAdoptAfter (a
// worker whose clock runs behind), in which case it counts. Without a baseline the sample counts and starts one.
func (state *cdcDelayAlertState) acceptCDCDelayOffset(jobID uint, offsetMs int64, now time.Time) bool {
	o := state.offsetState(jobID)
	base, ok := o.baseline(now)
	if !ok {
		o.add(now, offsetMs)
		return true
	}
	if base < 0 {
		base = 0
	}
	if offsetMs <= base+cdcDelayStaleSample.Milliseconds() {
		o.add(now, offsetMs)
		return true
	}
	if o.staleSince.IsZero() || now.Sub(o.staleLastAt) > time.Minute {
		o.staleSince = now
	}
	o.staleLastAt = now
	if now.Sub(o.staleSince) >= cdcDelayOffsetAdoptAfter {
		o.add(now, offsetMs)
		return true
	}
	return false
}

// addCDCDelaySample 把一条样本计入本轮，样本时间记为 STX 收到的时间 now，返回是否计入。sampleMs 是引擎时间：时间差
// （now − sampleMs）按 acceptCDCDelayOffset 判断是否为重投的旧样本；只有一个时钟不准的样本、或多个 worker 之间时钟差
// 超过 2 分钟，都不会让后面的样本被一直丢掉。时间差超过 2 分钟（时钟不同步）照常计入，并限频打 warning。多个 reader
// 交错上报、时间不递增的样本都计入。
// addCDCDelaySample adds one sample to the current round, timestamped with the STX receive time now, and reports
// whether it counted. sampleMs is the engine time: the offset (now − sampleMs) is judged by acceptCDCDelayOffset, so
// neither one bad timestamp nor more than 2 minutes of skew between workers keeps dropping later samples. An offset
// beyond 2 minutes (clock skew) still counts, with a rate-limited warning. Interleaved samples from several readers,
// whose times do not increase, all count.
func (s *Service) addCDCDelaySample(jobID uint, delayMs, sampleMs int64, now time.Time) bool {
	if s == nil || jobID == 0 {
		return false
	}
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	if sampleMs > 0 {
		offset := now.Sub(time.UnixMilli(sampleMs))
		if !state.acceptCDCDelayOffset(jobID, offset.Milliseconds(), now) {
			if state.staleDropped == nil {
				state.staleDropped = map[uint]int{}
			}
			state.staleDropped[jobID]++
			return false
		}
		if offset > cdcDelayStaleSample || offset < -cdcDelayStaleSample {
			if state.skewWarnAt == nil {
				state.skewWarnAt = map[uint]time.Time{}
			}
			if last, ok := state.skewWarnAt[jobID]; !ok || now.Sub(last) >= cdcDelaySkewWarnInterval {
				state.skewWarnAt[jobID] = now
				cdcDelaySkewWarn(jobID, offset)
			}
		}
	}
	if state.samples == nil {
		state.samples = map[uint][]cdcDelaySample{}
	}
	if len(state.samples[jobID]) >= cdcDelayMaxSamplesPerRound {
		return true
	}
	state.samples[jobID] = append(state.samples[jobID], cdcDelaySample{ms: delayMs, at: now})
	return true
}

// invalidateCDCDelayConfig 让任务告警配置缓存失效（任务告警配置被修改时调用）。
// invalidateCDCDelayConfig drops the cached task alert config (called when it is changed).
func (s *Service) invalidateCDCDelayConfig(taskID uint) {
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.configs, taskID)
	if state.configGen == nil {
		state.configGen = map[uint]uint64{}
	}
	state.configGen[taskID]++
}

// forgetCDCDelayJob 清掉作业的判定状态和样本。
// forgetCDCDelayJob drops a job's evaluation state and samples.
func (s *Service) forgetCDCDelayJob(jobID uint) {
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.jobs, jobID)
	delete(state.samples, jobID)
	delete(state.staleDropped, jobID)
	delete(state.offsets, jobID)
	delete(state.skewWarnAt, jobID)
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
	gen := state.configGen[taskID]
	state.mu.Unlock()
	cfg, err := s.alertStore.GetTaskAlertConfig(ctx, taskID)
	if err != nil {
		return nil, err
	}
	state.mu.Lock()
	if state.configs == nil {
		state.configs = map[uint]cdcDelayConfigEntry{}
	}
	// 读取期间配置被改过：本轮用读到的值，但不缓存，下一轮重新读。
	// The config changed during the read: use it for this round only and read again next round.
	if state.configGen[taskID] == gen {
		state.configs[taskID] = cdcDelayConfigEntry{cfg: cfg, at: now}
	}
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
			accepted := s.emitCDCDelayAction(ctx, job, action)
			s.markCDCDelaySent(job.ID, action, accepted, now)
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
	for jobID := range state.staleDropped {
		if !running[jobID] {
			delete(state.staleDropped, jobID)
		}
	}
	for jobID := range state.offsets {
		if !running[jobID] {
			delete(state.offsets, jobID)
			delete(state.skewWarnAt, jobID)
		}
	}
	state.mu.Unlock()
}

// cdcDelayChannelVerdict 判断集群事件通道：关闭、待重启或集群版本明确不支持（<2.3.12，见 seatunnel.SupportsCDCDelayEvents）
// 时不可用；版本取不到时不当作不支持。
// cdcDelayChannelVerdict judges the cluster event channel: off, pending restart, or a version known not to support it
// (<2.3.12, see seatunnel.SupportsCDCDelayEvents) is unavailable; an unknown version is not treated as unsupported.
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
	if version := s.clusterVersion(ctx, clusterID); version != "" && !seatunnel.SupportsCDCDelayEvents(version) {
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
	staleDropped := state.staleDropped[job.ID]
	delete(state.staleDropped, job.ID)
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
	input := cdcDelayRoundInput{samples: samples, channelDegraded: channel.degraded, staleOnly: len(samples) == 0 && staleDropped > 0}
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
	out := make([]cdcDelayAction, 0, len(actions))
	for _, action := range actions {
		action.message = cdcDelayMessage(action, job)
		action.key = cdcDelayContentKey(action)
		if action.kind == syncalert.KindRefresh {
			// 内容没变就不发：大量高延迟作业每 30 秒一次的 Refresh 不会占满告警队列、挤掉 B1 等信号。内容没变也每 5 分钟
			// 重发一次，之前被丢弃的 Refresh 能补上。记录在信号被接收之后才更新（markCDCDelaySent）。
			// Skip unchanged content so many high-delay jobs refreshing every 30s never fill the alert queue and crowd
			// out B1 and other signals; unchanged content is still resent every 5 minutes so a dropped Refresh heals.
			// The record is only updated once the signal is accepted (markCDCDelaySent).
			if action.key == current.lastContentKey && now.Sub(current.lastSentAt) < cdcDelayRefreshResendInterval {
				continue
			}
			action.stateChange = cdcDelayRefreshIsStateChange(action.detail, current)
		}
		out = append(out, action)
	}
	return out
}

// cdcDelayRefreshIsStateChange 判断一条 Refresh 是否算"状态变化"（接收方不按 Refresh 排队上限丢弃）：只有进入"无法确认"
// 和停止提醒两种。这两种之后状态机不会每轮再发（无法确认每 5 分钟才刷新一次，停止提醒后不再提醒），被丢弃就要等很久；
// high ↔ 回落待确认之间的切换不算：延迟抖动的作业每轮都在切换，全部绕过上限会让排队上限失效，而这些 Refresh 内容变了，
// 被丢弃后下一轮就会重发，回落待确认之后的恢复是 Resolve，不受上限影响。
// cdcDelayRefreshIsStateChange reports whether a Refresh counts as a state change (the receiver does not drop it
// under the Refresh queue limit): only entering "unconfirmable" and pausing reminders. After those the state machine
// does not emit every round (unconfirmable refreshes every 5 minutes, paused never reminds), so a drop would linger.
// Switching between high and recovering does not count: a jittery job switches every round, and letting all of those
// bypass the limit would defeat it; their content changed, so a dropped one is resent next round, and the recovery
// after recovering is a Resolve, which the limit does not apply to.
func cdcDelayRefreshIsStateChange(detail syncalert.CDCDelayDetail, current *cdcDelayJobState) bool {
	if detail.State == syncalert.CDCDelayStateUnknown && current.lastSentState != syncalert.CDCDelayStateUnknown {
		return true
	}
	return detail.RemindersPaused && !current.lastSentPaused
}

// markCDCDelaySent 在信号发出后更新作业的发送记录：只有被接收的 Fire / Refresh 才记为已发，被丢弃的下一轮重发。
// markCDCDelaySent updates a job's send record after emitting: only an accepted Fire / Refresh counts as sent, so a
// dropped one is sent again next round.
func (s *Service) markCDCDelaySent(jobID uint, action cdcDelayAction, accepted bool, now time.Time) {
	state := &s.cdcAlerts
	state.mu.Lock()
	defer state.mu.Unlock()
	current := state.jobs[jobID]
	if current == nil {
		return
	}
	switch action.kind {
	case syncalert.KindResolve:
		current.lastContentKey = ""
		current.lastSentAt = time.Time{}
		current.lastSentState = ""
		current.lastSentPaused = false
	case syncalert.KindFire, syncalert.KindRefresh:
		if !accepted {
			// "无法确认"的 Refresh 状态机自己每 5 分钟才发一次：被丢弃时清掉节流，下一轮就重发。
			// The state machine only emits an "unconfirmable" Refresh every 5 minutes: when one is dropped, clear the
			// throttle so it is sent again next round.
			if action.detail.State == syncalert.CDCDelayStateUnknown {
				current.unknownRefreshAt = time.Time{}
				if action.detail.RemindersPaused {
					current.pausedSent = false
				}
			}
			return
		}
		current.lastContentKey = action.key
		current.lastSentAt = now
		current.lastSentState = action.detail.State
		current.lastSentPaused = action.detail.RemindersPaused
	}
}

// cdcDelayContentKey 是一条信号的展示内容：说明文字（已含格式化后的延迟、阈值、时长）加状态与提醒开关。
// cdcDelayContentKey is a signal's display content: the note (which already carries the formatted delay, threshold
// and elapsed time) plus the state and the reminder switch.
func cdcDelayContentKey(action cdcDelayAction) string {
	d := action.detail
	return fmt.Sprintf("%s|%s|%d|%d|%t|%t|%s", d.State, d.RecoveryBasis, d.ThresholdSeconds, d.DurationMinutes, d.RemindersPaused, d.DelayInferred, action.message)
}

// emitCDCDelayAction 把判定结果转成信号发出，返回信号是否被接收。
// emitCDCDelayAction turns an evaluator action into a signal and reports whether it was accepted.
func (s *Service) emitCDCDelayAction(ctx context.Context, job *JobInstance, action cdcDelayAction) bool {
	signal, ok := s.alertSignalForJob(ctx, action.kind, syncalert.RuleCDCDelayHigh, job, nil, action.message)
	if !ok {
		return true
	}
	signal.StateChange = action.stateChange
	if action.kind == syncalert.KindResolve {
		signal.ResolveScope = syncalert.ResolveCDCDelay
		signal.ResolveReason = action.reason
	}
	if action.detail.State != "" {
		detail := action.detail
		signal.CDCDelay = &detail
	}
	return s.reportAlert(signal)
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
		// 作业的延迟取本轮最慢的 reader：多源作业里一个源卡在高位，其他源正常，也要算高。
		// The job delay is the slowest reader of the round: one source stuck high among healthy ones still counts.
		st.lastDelayMs = maxMs
		st.lastSampleAt = last.at
		median := medianInt64(values)
		// 高低按本轮最大值判断；中位数只用作追赶判定的基线（见 catchingUp）。
		// High/low is judged by the round maximum; the median only feeds the catch-up baseline (see catchingUp).
		if maxMs >= thresholdMs {
			firstAbove := now
			for _, sample := range in.samples {
				if sample.ms >= thresholdMs {
					firstAbove = sample.at
					break
				}
			}
			return st.onHigh(now, duration, thresholdMs, firstAbove, cdcDelayRoundPoint{at: now, median: median, max: maxMs})
		}
		return st.onLow(syncalert.CDCDelayRecoveryBySamples)
	case in.staleOnly:
		return st.onUnknown(now, syncalert.CDCDelayUnknownStaleSamples)
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
func (st *cdcDelayJobState) onHigh(now time.Time, duration time.Duration, thresholdMs int64, firstAbove time.Time, point cdcDelayRoundPoint) []cdcDelayAction {
	// 不在告警中、和上一轮 high 之间隔了两轮以上非 high（low 或无法确认）：通常从这一轮重新计时，中间收不到数据的时间
	// 不算超标时间，孤立的尖峰不会凑成"已持续几百分钟"的告警。
	// 例外：这一轮最大延迟 ≥ 阈值 + 距上一轮 high 的时间（now − lastHighAt）时照常累计。读取位置只会前进：现在读到的
	// 是 now − D 时刻的数据，间隔内任一时刻 t 的读取位置都不超过它，延迟 ≥ t − (now − D) ≥ D − (now − lastHighAt)
	// ≥ 阈值，即间隔内延迟从未低于阈值。下游反压、sink flush 卡住时引擎只在偶尔真正发出记录时才上报事件，延迟一直在涨，
	// 这样的稀疏上报也能告警；孤立的尖峰（例如间隔 10 分钟后的 200 秒 < 60 秒 + 600 秒）仍然重新计时。
	// Not alerting and two or more non-high rounds (low or unconfirmable) since the previous high one: normally restart
	// timing at this round, so time without data never counts as time above the threshold and isolated spikes never
	// add up to an alert claiming hundreds of minutes.
	// Exception: when this round's max delay ≥ threshold + the time since the previous high round (now − lastHighAt),
	// timing continues. The read position only moves forward: it is now at now − D, and at any t in the gap it was no
	// further, so the delay at t was ≥ t − (now − D) ≥ D − (now − lastHighAt) ≥ threshold, i.e. it never dropped below
	// the threshold during the gap. With downstream backpressure or a stuck sink flush the engine only reports when it
	// actually emits records, so such sparse reporting of a growing delay still fires; an isolated spike (e.g. 200s
	// after a 10-minute gap < 60s + 600s) still restarts timing.
	if st.phase != cdcPhaseFiring && st.nonHighRounds >= cdcDelayLowRoundsToRecover {
		bridged := !st.lastHighAt.IsZero() && point.max >= thresholdMs+now.Sub(st.lastHighAt).Milliseconds()
		if !bridged {
			st.resetTiming()
		}
	}
	st.lastHighAt = now
	st.nonHighRounds = 0
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

// catchingUp 判断是否正在追赶：窗口内至少两轮，最新一轮的中位数 ≤ 最早一轮中位数的 90%，并且最新一轮的最大值也 ≤
// 最早一轮最大值的 90%。中位数让一轮里个别大事务尖峰不会把基线抬高（误判为"在追赶"而漏报）；最大值的条件让多源作业里
// 其他 reader 在追赶、一个 reader 卡住越来越慢时，不会被混在一起的中位数当成"在追赶"。
// catchingUp reports catch-up: at least two rounds in the window, the latest round median ≤ 90% of the earliest
// round median, and the latest round max ≤ 90% of the earliest round max. The median keeps a single big-transaction
// spike from inflating the baseline and hiding a real alert as "catching up"; the max condition keeps a multi-source
// job whose other readers catch up while one reader is stuck and growing from looking like it is catching up.
func (st *cdcDelayJobState) catchingUp() bool {
	if len(st.history) < 2 {
		return false
	}
	baseline := st.history[0]
	latest := st.history[len(st.history)-1]
	return baseline.median > 0 && latest.median*100 <= baseline.median*cdcDelayCatchUpPercent &&
		latest.max*100 <= baseline.max*cdcDelayCatchUpPercent
}

// onLow 处理低于阈值的一轮。和恢复一样，连续两轮低才算数：单独一轮低不重新计时（间歇性高延迟照样能累计到持续时间），
// 告警中只把阶段刷新为当前值；连续两轮后重新计时，并按阶段发恢复或静默关闭。
// onLow handles a round below the threshold. Like recovery, only two low rounds in a row count: a single low round
// does not restart timing (so intermittent high delay still adds up to the duration) and only refreshes an open
// episode with the current values; after two in a row timing restarts and the episode recovers or closes silently
// depending on phase.
func (st *cdcDelayJobState) onLow(basis string) []cdcDelayAction {
	st.unknownSince = time.Time{}
	st.unknownRefreshAt = time.Time{}
	st.pausedSent = false
	st.nonHighRounds++
	st.lowStreak++
	byReadCount := basis == syncalert.CDCDelayRecoveryByReadCount
	if byReadCount {
		st.lowByReadCount = true
	}
	if st.lowStreak < cdcDelayLowRoundsToRecover {
		if st.phase == cdcPhaseNone {
			return nil
		}
		// 告警中（或重启后、改参数后）的第一轮低：阶段内容更新为当前值，提醒不会再带着旧的延迟或旧阈值发出。
		// First low round of an open (maybe / re-armed) episode: refresh it with current values so a reminder never
		// carries a stale delay or threshold.
		detail := st.baseDetail(syncalert.CDCDelayStateRecovering)
		if byReadCount {
			detail.RecoveryBasis = syncalert.CDCDelayRecoveryByReadCount
			markCDCDelayInferred(&detail)
		}
		return []cdcDelayAction{{kind: syncalert.KindRefresh, detail: detail}}
	}
	st.resetTiming()
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
		if recoveryBasis == syncalert.CDCDelayRecoveryByReadCount {
			// 推断出的恢复没有当前延迟：不带最后一个旧样本（例如 192000），由 DelayInferred 标明。
			// An inferred recovery has no current delay: drop the last old sample (e.g. 192000) and flag DelayInferred.
			markCDCDelayInferred(&detail)
		}
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
	st.nonHighRounds++
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

// markCDCDelayInferred 标记明细里的延迟是推断的：清掉旧的当前延迟。
// markCDCDelayInferred flags the detail's delay as inferred and clears the stale current delay.
func markCDCDelayInferred(detail *syncalert.CDCDelayDetail) {
	detail.CurrentDelayMs = 0
	detail.DelayInferred = true
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
	case detail.State == syncalert.CDCDelayStateRecovering:
		if detail.DelayInferred {
			return fmt.Sprintf("最近没有延迟事件，读取条数在增长，读取延迟应已低于 1 秒（根据读取条数推断），阈值 %s；再确认一轮后自动关闭",
				syncalert.FormatDelayZH(int64(detail.ThresholdSeconds)*1000))
		}
		return fmt.Sprintf("读取延迟已回落到 %s，低于阈值 %s；再确认一轮后自动关闭", syncalert.FormatDelayZH(detail.CurrentDelayMs),
			syncalert.FormatDelayZH(int64(detail.ThresholdSeconds)*1000))
	case detail.State == syncalert.CDCDelayStateUnknown:
		var b strings.Builder
		switch detail.UnknownReason {
		case syncalert.CDCDelayUnknownChannelDegraded:
			b.WriteString("事件通道降级，最近没有收到延迟数据，无法确认读取延迟是否恢复")
		case syncalert.CDCDelayUnknownStaleSamples:
			b.WriteString("最近收到的延迟数据引擎时间比 STX 收到时间早 2 分钟以上（引擎时钟回拨或积压重投），暂不计入，无法确认读取延迟是否恢复")
		default:
			b.WriteString("最近 1 分钟没有收到延迟数据，读取条数也没有增长，无法确认读取延迟是否恢复")
		}
		b.WriteString(fmt.Sprintf("（自 %s 起）", formatCDCDelayTime(detail.UnknownSince)))
		if !detail.LastSampleAt.IsZero() {
			b.WriteString(fmt.Sprintf("；最后一次读取延迟 %s（%s）", syncalert.FormatDelayZH(detail.CurrentDelayMs), formatCDCDelayTime(detail.LastSampleAt)))
		}
		if detail.UnknownReason == syncalert.CDCDelayUnknownStaleSamples {
			b.WriteString("。请检查 SeaTunnel 节点与 STX 的时钟是否同步。")
		} else {
			b.WriteString("。可能原因：源库暂时没有写入；作业读取卡住；事件通道中断。")
		}
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
