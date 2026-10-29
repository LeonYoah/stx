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
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// cdcSim 用假时钟驱动 B4 状态机：每 10 秒一批样本（与引擎投递间隔一致），每 30 秒一轮判定。
// cdcSim drives the B4 state machine with a fake clock: one sample batch every 10s (the engine delivery interval)
// and one evaluation round every 30s.
type cdcSim struct {
	t         *testing.T
	st        *cdcDelayJobState
	now       time.Time
	threshold int
	duration  int
	pending   []cdcDelaySample
	readCount *float64
	degraded  bool
	actions   []timedAction
}

type timedAction struct {
	at     time.Time
	action cdcDelayAction
}

func newCDCSim(t *testing.T, phase cdcDelayPhase, threshold, duration int) *cdcSim {
	return &cdcSim{t: t, st: &cdcDelayJobState{phase: phase}, now: time.Date(2026, 10, 29, 2, 0, 0, 0, time.UTC), threshold: threshold, duration: duration}
}

// tick 推进 10 秒；delaySeconds ≥ 0 时这 10 秒收到一条样本，每 3 次（30 秒）跑一轮。
// tick advances 10s; with delaySeconds ≥ 0 one sample arrives, and every third tick (30s) runs a round.
func (c *cdcSim) tick(delaySeconds ...int64) []cdcDelayAction {
	c.now = c.now.Add(10 * time.Second)
	for _, d := range delaySeconds {
		c.pending = append(c.pending, cdcDelaySample{ms: d * 1000, at: c.now})
	}
	if c.now.Unix()%30 != 0 {
		return nil
	}
	actions := c.st.step(c.now, c.threshold, c.duration, cdcDelayRoundInput{samples: c.pending, readCount: c.readCount, channelDegraded: c.degraded})
	c.pending = nil
	for _, action := range actions {
		c.actions = append(c.actions, timedAction{at: c.now, action: action})
	}
	return actions
}

// round 跑满一轮（3 次 tick），每 10 秒的样本取 values 中对应的值（不足时重复最后一个；空表示没有样本）。
// round runs one full round (3 ticks); each 10s batch takes the matching value (repeating the last; empty = none).
func (c *cdcSim) round(values ...int64) []cdcDelayAction {
	var out []cdcDelayAction
	for i := 0; i < 3; i++ {
		if len(values) == 0 {
			out = append(out, c.tick()...)
			continue
		}
		v := values[len(values)-1]
		if i < len(values) {
			v = values[i]
		}
		out = append(out, c.tick(v)...)
	}
	return out
}

func (c *cdcSim) count(kind syncalert.SignalKind) int {
	n := 0
	for _, a := range c.actions {
		if a.action.kind == kind {
			n++
		}
	}
	return n
}

func (c *cdcSim) first(kind syncalert.SignalKind) (timedAction, bool) {
	for _, a := range c.actions {
		if a.action.kind == kind {
			return a, true
		}
	}
	return timedAction{}, false
}

// 验收：持续高于阈值，达到持续时间后 ≤60 秒内首次告警，之后只 Refresh，不重复 Fire。
// Acceptance: sustained above the threshold, the first alert comes ≤60s after the duration is reached; afterwards
// only Refresh, never another Fire.
func TestCDCDelayFiresWithin60sAfterDuration(t *testing.T) {
	for offset := 0; offset < 3; offset++ {
		sim := newCDCSim(t, cdcPhaseNone, 60, 5)
		for i := 0; i < offset; i++ {
			sim.tick() // 让越过阈值的时刻落在轮次内的不同位置。 / Shift the crossing within a round.
		}
		crossedAt := sim.now.Add(10 * time.Second)
		for i := 0; i < 60; i++ {
			sim.tick(120)
		}
		fire, ok := sim.first(syncalert.KindFire)
		if !ok {
			t.Fatalf("offset %d: sustained delay must fire", offset)
		}
		if late := fire.at.Sub(crossedAt.Add(5 * time.Minute)); late < 0 || late > 60*time.Second {
			t.Fatalf("offset %d: fire %v after the duration, want within [0, 60s]", offset, late)
		}
		if sim.count(syncalert.KindFire) != 1 || sim.count(syncalert.KindRefresh) == 0 {
			t.Fatalf("offset %d: want exactly one Fire then Refreshes, got fire=%d refresh=%d", offset, sim.count(syncalert.KindFire), sim.count(syncalert.KindRefresh))
		}
		detail := fire.action.detail
		if detail.State != syncalert.CDCDelayStateHigh || detail.CurrentDelayMs != 120_000 || detail.ThresholdSeconds != 60 || detail.DurationMinutes != 5 || detail.WindowMaxDelayMs != 120_000 {
			t.Fatalf("fire detail wrong: %+v", detail)
		}
	}
}

// 验收：告警后连续两轮低于阈值，≤90 秒内发恢复；中途个别回落的轮次不触发。
// Acceptance: after alerting, two low rounds in a row recover within ≤90s.
func TestCDCDelayRecoversWithin90s(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 1)
	for i := 0; i < 12; i++ {
		sim.tick(120)
	}
	if sim.count(syncalert.KindFire) != 1 {
		t.Fatalf("expected fire, got %d", sim.count(syncalert.KindFire))
	}
	// 在轮次中间回落，最坏情况。 / Drop mid-round, the worst case.
	sim.tick(120)
	droppedAt := sim.now.Add(10 * time.Second)
	for i := 0; i < 12; i++ {
		sim.tick(5)
	}
	resolve, ok := sim.first(syncalert.KindResolve)
	if !ok || resolve.action.reason != syncalert.ResolveReasonRecovered || resolve.action.detail.RecoveryBasis != syncalert.CDCDelayRecoveryBySamples {
		t.Fatalf("expected recovered by samples, got %+v ok=%v", resolve, ok)
	}
	if took := resolve.at.Sub(droppedAt); took > 90*time.Second {
		t.Fatalf("recovery took %v, want ≤90s", took)
	}
	if sim.count(syncalert.KindResolve) != 1 {
		t.Fatalf("recovery must be sent once, got %d", sim.count(syncalert.KindResolve))
	}
}

// 短暂尖峰（短于持续时间）不告警；大事务单个尖峰也不会让一轮变成 high（按中位数分类）。
// A short spike (shorter than the duration) never fires; a single big-transaction spike does not make a round high
// either (rounds are classified by median).
func TestCDCDelayShortSpikeDoesNotFire(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 5)
	for i := 0; i < 8; i++ { // 4 分钟高 / 4 minutes high
		sim.round(120)
	}
	for i := 0; i < 4; i++ {
		sim.round(3)
	}
	for i := 0; i < 20; i++ {
		sim.round(3, 900, 3) // 每轮一个 15 分钟的尖峰 / one 15-minute spike per round
	}
	if n := sim.count(syncalert.KindFire); n != 0 {
		t.Fatalf("short or single-sample spikes must not fire, got %d", n)
	}
}

// catchUpRun 让延迟从 300 秒线性下降到 end 秒（M=5，第 11 轮恰好达到持续时间），返回是否告警。
// catchUpRun lowers the delay linearly from 300s to end seconds (M=5; round 11 is the first that meets the duration)
// and reports whether it fired.
func catchUpRun(t *testing.T, end int64) bool {
	sim := newCDCSim(t, cdcPhaseNone, 60, 5)
	rounds := int64(11)
	for k := int64(0); k < rounds; k++ {
		v := 300 - (300-end)*k/(rounds-1)
		sim.round(v)
	}
	return sim.count(syncalert.KindFire) > 0
}

// 验收：追赶判定的具体数值。300→260 降了 13% 算在追赶不告警；300→280 只降 6.7% 告警；持平、上涨告警。
// Acceptance with concrete numbers: 300→260 (−13%) is catching up, no alert; 300→280 (−6.7%) alerts; flat or rising
// alerts.
func TestCDCDelayCatchUpConcreteNumbers(t *testing.T) {
	if catchUpRun(t, 260) {
		t.Fatalf("300→260 is catching up and must not alert")
	}
	if !catchUpRun(t, 280) {
		t.Fatalf("300→280 is too slow and must alert")
	}
	if !catchUpRun(t, 300) {
		t.Fatalf("flat delay must alert")
	}
	if !catchUpRun(t, 400) {
		t.Fatalf("rising delay must alert")
	}
}

// 基线用每轮中位数：第一轮里一个 3000 秒的大事务尖峰不会让 300→280 被当成"在追赶"。
// The baseline uses per-round medians: a 3000s big-transaction spike in the first round does not turn 300→280 into
// "catching up".
func TestCDCDelayCatchUpBaselineIgnoresSpike(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 5)
	sim.round(300, 3000, 300)
	for k := int64(1); k < 11; k++ {
		sim.round(300 - 2*k)
	}
	fire, ok := sim.first(syncalert.KindFire)
	if !ok {
		t.Fatalf("a spike in the first round must not hide a slow catch-up")
	}
	if fire.action.detail.WindowMaxDelayMs != 3_000_000 {
		t.Fatalf("window max should still show the spike for context, got %d", fire.action.detail.WindowMaxDelayMs)
	}
}

// 验收：追赶期最长 30 分钟；一直在追赶但 30 分钟后仍高于阈值，照常告警。
// Acceptance: catch-up is capped at 30 minutes; still above the threshold after 30 minutes of catch-up alerts.
func TestCDCDelayCatchUpCappedAt30Minutes(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 5)
	v := 6000.0
	var catchUpStart time.Time
	for i := 0; i < 200 && sim.count(syncalert.KindFire) == 0; i++ {
		sim.round(int64(v))
		v *= 0.98 // 每轮降 2%，5 分钟降约 18%：一直算在追赶。 / −2% per round, ~−18% per 5 min: always catching up.
		if catchUpStart.IsZero() && !sim.st.catchUpSince.IsZero() {
			catchUpStart = sim.st.catchUpSince
		}
	}
	fire, ok := sim.first(syncalert.KindFire)
	if !ok || catchUpStart.IsZero() {
		t.Fatalf("catch-up must eventually alert (fire=%v catchUp=%v)", ok, catchUpStart)
	}
	if got := fire.at.Sub(catchUpStart); got < 30*time.Minute || got > 30*time.Minute+cdcDelayEvalInterval {
		t.Fatalf("catch-up alerted after %v, want 30 minutes", got)
	}
	if fire.action.detail.CurrentDelayMs < 60_000 {
		t.Fatalf("delay must still be above the threshold at the capped alert: %+v", fire.action.detail)
	}
}

// 没有延迟样本、读取条数在增长：判为低，连续两轮后恢复，依据为读取条数。
// No samples but a growing read count counts as low; two rounds recover with the read-count basis.
func TestCDCDelayRecoveryInferredFromReadCount(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 1)
	count := 100.0
	sim.readCount = &count
	for i := 0; i < 4; i++ {
		sim.round(120)
		count += 50
	}
	if sim.count(syncalert.KindFire) != 1 {
		t.Fatalf("expected fire")
	}
	for i := 0; i < 2; i++ {
		count += 50
		sim.round()
	}
	resolve, ok := sim.first(syncalert.KindResolve)
	if !ok || resolve.action.detail.RecoveryBasis != syncalert.CDCDelayRecoveryByReadCount {
		t.Fatalf("expected recovery inferred from read count, got %+v", resolve)
	}
	if msg := cdcDelayMessage(resolve.action, &JobInstance{ID: 3, TaskID: 1}); !strings.Contains(msg, "根据读取条数推断") {
		t.Fatalf("recovery message must say it is inferred from read counts: %q", msg)
	}
}

// 没有样本、读取条数不增长或通道降级：只 Refresh（无法确认），不恢复；每 5 分钟刷新一次，超过 24 小时停止提醒。
// No samples with a flat read count or a degraded channel: only Refresh (unconfirmable), never recover; refresh every
// 5 minutes and pause reminders after 24 hours.
func TestCDCDelayUnknownRefreshesAndPausesAfter24h(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 1)
	count := 100.0
	sim.readCount = &count
	for i := 0; i < 4; i++ {
		sim.round(120)
	}
	before := len(sim.actions)
	refreshes := 0
	var last cdcDelayAction
	for i := 0; i < 20; i++ { // 10 分钟 / 10 minutes
		for _, a := range sim.round() {
			if a.kind == syncalert.KindResolve {
				t.Fatalf("unconfirmable must not resolve")
			}
			refreshes++
			last = a
		}
	}
	if refreshes != 2 || len(sim.actions)-before != 2 {
		t.Fatalf("expected a refresh on entry and one after 5 minutes, got %d", refreshes)
	}
	if last.detail.State != syncalert.CDCDelayStateUnknown || last.detail.UnknownReason != syncalert.CDCDelayUnknownNoProgress || last.detail.RemindersPaused {
		t.Fatalf("unexpected unknown detail: %+v", last.detail)
	}
	msg := cdcDelayMessage(last, &JobInstance{ID: 3, TaskID: 1})
	for _, want := range []string{"无法确认", "可能原因", "2026-10-29", "最后一次读取延迟 2分"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("unknown message missing %q: %q", want, msg)
		}
	}
	// 通道降级同样是无法确认。 / A degraded channel is unconfirmable as well.
	sim.degraded = true
	sim.st.unknownRefreshAt = time.Time{}
	if actions := sim.round(); len(actions) != 1 || actions[0].detail.UnknownReason != syncalert.CDCDelayUnknownChannelDegraded {
		t.Fatalf("degraded channel should refresh as channel_degraded, got %+v", actions)
	}
	// 推进到 24 小时：发一次带 RemindersPaused 的 Refresh，阶段保留。
	// Advance to 24 hours: one Refresh with RemindersPaused, the episode stays.
	sim.now = sim.st.unknownSince.Add(cdcDelayUnknownReminderCutoff - 30*time.Second)
	paused := false
	for i := 0; i < 3; i++ {
		for _, a := range sim.round() {
			if a.detail.RemindersPaused {
				paused = true
				if !strings.Contains(cdcDelayMessage(a, &JobInstance{ID: 3, TaskID: 1}), "已停止提醒") {
					t.Fatalf("paused message must say reminders stopped")
				}
			}
		}
	}
	if !paused || sim.count(syncalert.KindResolve) != 0 {
		t.Fatalf("after 24h reminders pause without resolving (paused=%v)", paused)
	}
}

// 验收：STX 重启后 phase=maybe，已有阶段时用当前值 Refresh（不一直提醒重启前的旧值）；连续低两轮补发一次恢复。
// Acceptance: after an STX restart (phase=maybe) an existing episode is refreshed with current values instead of
// reminding with the stale pre-restart value; two low rounds send one recovery.
func TestCDCDelayRestartMaybeRefreshesThenResolvesOnce(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseMaybe, 60, 5)
	actions := sim.round(90)
	if len(actions) != 1 || actions[0].kind != syncalert.KindRefresh || actions[0].detail.CurrentDelayMs != 90_000 {
		t.Fatalf("maybe phase must refresh with the current value, got %+v", actions)
	}
	sim.round(5)
	sim.round(5)
	sim.round(5)
	if sim.count(syncalert.KindResolve) != 1 || sim.count(syncalert.KindFire) != 0 {
		t.Fatalf("expected exactly one resolve after restart, got resolve=%d fire=%d", sim.count(syncalert.KindResolve), sim.count(syncalert.KindFire))
	}
	// 重启后持续高：按正常规则再 Fire（monitoring 会合并到已有阶段）。
	// Sustained high after a restart fires normally (monitoring merges it into the existing episode).
	sim2 := newCDCSim(t, cdcPhaseMaybe, 60, 1)
	for i := 0; i < 4; i++ {
		sim2.round(120)
	}
	if sim2.count(syncalert.KindFire) != 1 {
		t.Fatalf("sustained high after restart should fire once, got %d", sim2.count(syncalert.KindFire))
	}
}

// 验收：告警中修改阈值或持续时间会重新计时，不会误发"恢复"。
// Acceptance: changing the threshold or duration while alerting restarts timing and never sends a false "recovered".
func TestCDCDelayConfigChangeRestartsTimingWithoutFalseRecovery(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 1)
	for i := 0; i < 4; i++ {
		sim.round(120)
	}
	if sim.count(syncalert.KindFire) != 1 {
		t.Fatalf("expected fire")
	}
	// 阈值调到 600 秒：120 秒不再超标，回落要静默关闭（rule_changed），不是恢复。
	// Threshold raised to 600s: 120s is no longer above it and must close silently (rule_changed), not recover.
	sim.threshold = 600
	sim.round(120)
	sim.round(120)
	resolve, ok := sim.first(syncalert.KindResolve)
	if !ok || resolve.action.reason != syncalert.ResolveReasonRuleChanged {
		t.Fatalf("expected a silent rule_changed close, got %+v", resolve)
	}
	if resolve.action.detail.State == syncalert.CDCDelayStateRecovered {
		t.Fatalf("a parameter change must not look like a recovery")
	}

	// 持续时间改长：仍然超标时从修改时刻重新计时，新持续时间到了才合并 Fire。
	// Duration lengthened while still above: timing restarts from the change and Fire only after the new duration.
	sim2 := newCDCSim(t, cdcPhaseNone, 60, 1)
	for i := 0; i < 4; i++ {
		sim2.round(120)
	}
	sim2.duration = 3
	changedAt := sim2.now
	for i := 0; i < 8; i++ {
		sim2.round(120)
	}
	if sim2.count(syncalert.KindResolve) != 0 {
		t.Fatalf("changing the duration while still high must not resolve")
	}
	fires := 0
	for _, a := range sim2.actions {
		if a.action.kind == syncalert.KindFire {
			fires++
			if fires == 2 && a.at.Sub(changedAt) < 3*time.Minute {
				t.Fatalf("re-armed fire came %v after the change, before the new duration", a.at.Sub(changedAt))
			}
		}
	}
	if fires != 2 {
		t.Fatalf("expected the re-armed Fire after the new duration, got %d fires", fires)
	}
}

func TestMedianInt64(t *testing.T) {
	if medianInt64([]int64{5, 1, 3}) != 3 || medianInt64([]int64{4, 1, 3, 2}) != 2 || medianInt64(nil) != 0 {
		t.Fatalf("median wrong")
	}
}

// --- Service 层：候选作业、配置、通道与静默关闭 / Service level: candidates, config, channel and silent closes ---

type cdcServiceFixture struct {
	service  *Service
	reporter *recordingAlertReporter
	store    *memoryTaskAlertStore
	job      *JobInstance
	now      time.Time
}

func newCDCServiceFixture(t *testing.T) *cdcServiceFixture {
	t.Helper()
	withEventConfig(t, "secret", "http://stx")
	service, _ := newEventTestService(t, "3.0.0")
	f := &cdcServiceFixture{service: service, now: time.Now().UTC()}
	service.events.now = func() time.Time { return f.now }
	f.reporter = attachReporter(service)
	f.store = &memoryTaskAlertStore{configs: map[uint]*syncalert.TaskConfig{}, refs: map[uint]syncalert.TaskRef{}}
	service.SetTaskAlertPolicyStore(f.store)
	auth := enableChannel(t, service, 5)
	// 收到过事件，通道为 healthy。 / One event makes the channel healthy.
	if _, err := service.IngestEngineEvents(context.Background(), 5, auth, []byte(`[{"jobId":"x","jobStatus":"RUNNING","createdTime":1,"eventType":"JOB_STATUS"}]`)); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	f.job = createEventJob(t, service, "2001", 5)
	return f
}

func (f *cdcServiceFixture) configure(rules []string, threshold, duration int) {
	f.store.configs[f.job.TaskID] = (&syncalert.TaskConfig{Configured: true, Enabled: true, Rules: rules, ChannelIDs: []uint{1}, CDCDelayThresholdSeconds: threshold, CDCDelayDurationMinutes: duration}).Normalize()
	f.service.invalidateCDCDelayConfig(f.job.TaskID)
}

// round 推进 30 秒，期间每 10 秒收到一条延迟样本，然后跑一轮。
// round advances 30s with one delay sample every 10s, then runs one round.
func (f *cdcServiceFixture) round(delaySeconds int64) {
	for i := 0; i < 3; i++ {
		f.now = f.now.Add(10 * time.Second)
		if delaySeconds >= 0 {
			f.service.recordCDCDelay(f.job.ID, delaySeconds*1000, f.now.UnixMilli())
		}
	}
	f.service.evaluateCDCDelayAlerts(context.Background())
}

func (f *cdcServiceFixture) b4(kind syncalert.SignalKind) []syncalert.Signal {
	scope := syncalert.ResolveScope("")
	if kind == syncalert.KindResolve {
		scope = syncalert.ResolveCDCDelay
	}
	return f.reporter.find(kind, syncalert.RuleCDCDelayHigh, scope)
}

func TestCDCDelayServiceFiresOnlyWhenConfigured(t *testing.T) {
	f := newCDCServiceFixture(t)
	for i := 0; i < 6; i++ {
		f.round(120)
	}
	if f.reporter.count() != 0 {
		t.Fatalf("no B4 config means no signal, got %d", f.reporter.count())
	}
	f.configure([]string{syncalert.RuleJobFailed}, 0, 0)
	for i := 0; i < 6; i++ {
		f.round(120)
	}
	if f.reporter.count() != 0 {
		t.Fatalf("B4 not selected means no signal, got %d", f.reporter.count())
	}
	f.configure([]string{syncalert.RuleJobFailed, syncalert.RuleCDCDelayHigh}, 30, 1)
	for i := 0; i < 4; i++ {
		f.round(120)
	}
	fires := f.b4(syncalert.KindFire)
	if len(fires) != 1 || fires[0].JobID != f.job.ID || fires[0].ClusterID != 5 || fires[0].CDCDelay == nil || fires[0].CDCDelay.ThresholdSeconds != 30 {
		t.Fatalf("expected one B4 fire with detail, got %+v", fires)
	}
	if !strings.Contains(fires[0].Message, "读取延迟") || strings.Contains(fires[0].Message, "下游") {
		t.Fatalf("message should talk about read delay: %q", fires[0].Message)
	}
}

func TestCDCDelayServiceSkipsBatchPreviewAndStaleSamples(t *testing.T) {
	f := newCDCServiceFixture(t)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	ctx := context.Background()
	for _, mutate := range []func(*JobInstance){
		func(j *JobInstance) { j.RunType = RunTypePreview },
		func(j *JobInstance) { j.SubmitSpec["submitted_content"] = "env {\n  job.mode = \"BATCH\"\n}" },
	} {
		job := createEventJob(t, f.service, "3001", 5)
		mutate(job)
		if err := f.service.repo.db.Save(job).Error; err != nil {
			t.Fatalf("save: %v", err)
		}
		for i := 0; i < 6; i++ {
			f.now = f.now.Add(10 * time.Second)
			f.service.recordCDCDelay(job.ID, 120_000, f.now.UnixMilli())
			f.service.evaluateCDCDelayAlerts(ctx)
		}
		for _, signal := range f.reporter.signals {
			if signal.JobID == job.ID {
				t.Fatalf("batch / preview jobs must not take part: %+v", signal)
			}
		}
		job.Status = JobStatusCanceled
		f.service.repo.db.Save(job)
	}
	// 超过 2 分钟的旧样本（STX 停机后重投的积压）不计入。
	// Samples older than 2 minutes (backlog replayed after downtime) are not counted.
	for i := 0; i < 8; i++ {
		f.now = f.now.Add(30 * time.Second)
		f.service.recordCDCDelay(f.job.ID, 120_000, f.now.Add(-10*time.Minute).UnixMilli()+int64(i))
		f.service.evaluateCDCDelayAlerts(ctx)
	}
	if len(f.b4(syncalert.KindFire)) != 0 {
		t.Fatalf("stale samples must not fire")
	}
}

func TestCDCDelayServiceSilentCloses(t *testing.T) {
	f := newCDCServiceFixture(t)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	for i := 0; i < 4; i++ {
		f.round(120)
	}
	if len(f.b4(syncalert.KindFire)) != 1 {
		t.Fatalf("expected fire")
	}
	// 去掉 B4：静默关闭 rule_removed。 / Removing B4 closes silently with rule_removed.
	f.configure([]string{syncalert.RuleJobFailed}, 30, 1)
	f.round(120)
	resolves := f.b4(syncalert.KindResolve)
	if len(resolves) != 1 || resolves[0].ResolveReason != syncalert.ResolveReasonRuleRemoved {
		t.Fatalf("expected rule_removed, got %+v", resolves)
	}

	// 通道关闭：静默关闭 channel_unavailable，只发一次。
	// Channel turned off: close silently with channel_unavailable, once.
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	for i := 0; i < 4; i++ {
		f.round(120)
	}
	disabled := false
	if _, err := f.service.UpdateEventChannelForActor(context.Background(), adminActor, 5, &UpdateEventChannelRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("disable channel: %v", err)
	}
	f.round(120)
	f.round(120)
	var channelOff int
	for _, s := range f.b4(syncalert.KindResolve) {
		if s.ResolveReason == syncalert.ResolveReasonChannelOff {
			channelOff++
		}
	}
	if channelOff != 1 {
		t.Fatalf("expected one channel_unavailable close, got %d", channelOff)
	}

	// 作业结束：静默关闭 job_ended，带作业 ID。 / Job end: close silently with job_ended and the job ID.
	previous := f.job.Status
	f.job.Status = JobStatusCanceled
	f.service.reportJobTransition(context.Background(), f.job, previous)
	var ended []syncalert.Signal
	for _, s := range f.b4(syncalert.KindResolve) {
		if s.ResolveReason == syncalert.ResolveReasonJobEnded {
			ended = append(ended, s)
		}
	}
	if len(ended) != 1 || ended[0].JobID != f.job.ID {
		t.Fatalf("expected one job_ended close for the job, got %+v", ended)
	}
}

// 样本写入与判定并发运行（go test -race）。 / Sample ingestion and rounds run concurrently (go test -race).
func TestCDCDelayConcurrentSamplesAndRounds(t *testing.T) {
	f := newCDCServiceFixture(t)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	start := time.Now().UTC()
	f.service.events.now = func() time.Time { return start }
	var wg gosync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				f.service.recordCDCDelay(f.job.ID, 120_000, start.UnixMilli()+int64(w*1000+i))
			}
		}(w)
	}
	for i := 0; i < 20; i++ {
		f.service.evaluateCDCDelayAlerts(context.Background())
	}
	wg.Wait()
}
