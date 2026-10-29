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
	"fmt"
	"strings"
	gosync "sync"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
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

// 短暂尖峰（短于持续时间）不告警；之后每个尖峰轮次（按最大值算 high）后面都跟着连续两轮低，计时每次都重新开始，也不告警。
// A short spike (shorter than the duration) never fires; isolated spike rounds (high by the round max) each followed
// by two low rounds restart timing every time and never fire either.
func TestCDCDelayShortSpikeDoesNotFire(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 5)
	for i := 0; i < 8; i++ { // 4 分钟高 / 4 minutes high
		sim.round(120)
	}
	for i := 0; i < 4; i++ {
		sim.round(3)
	}
	for i := 0; i < 20; i++ {
		sim.round(3, 900, 3) // 一个 15 分钟的大事务尖峰 / one 15-minute big-transaction spike
		sim.round(3)
		sim.round(3)
	}
	if n := sim.count(syncalert.KindFire); n != 0 {
		t.Fatalf("short or isolated spikes must not fire, got %d", n)
	}
}

// 验收（产品复现）：阈值 60 秒、持续 2 分钟，"3 轮 200 秒 + 1 轮 30 秒"循环 1 小时，必须告警。单独一轮低不重新计时。
// Acceptance (product repro): threshold 60s, duration 2 minutes, "3 rounds at 200s + 1 round at 30s" for one hour
// must fire. A single low round does not restart timing.
func TestCDCDelayIntermittentHighFires(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 2)
	start := sim.now
	for sim.now.Sub(start) < time.Hour {
		sim.round(200)
		sim.round(200)
		sim.round(200)
		sim.round(30)
	}
	fire, ok := sim.first(syncalert.KindFire)
	if !ok {
		t.Fatalf("intermittent high delay (3×200s + 1×30s) must fire")
	}
	// 第 5 轮（150 秒）时已超标 140 秒 ≥ 2 分钟。 / By round 5 (150s) the delay has been high for 140s ≥ 2 minutes.
	if got := fire.at.Sub(start); got > 150*time.Second {
		t.Fatalf("fire came %v after start, want ≤150s", got)
	}
	if sim.count(syncalert.KindFire) != 1 || sim.count(syncalert.KindResolve) != 0 {
		t.Fatalf("want one fire and no recovery for the hour, got fire=%d resolve=%d", sim.count(syncalert.KindFire), sim.count(syncalert.KindResolve))
	}
	// 单独一轮低：只把阶段刷新为"已回落、待确认"，不发恢复。
	// A single low round only refreshes the episode as recovering and never recovers.
	var recovering int
	for _, a := range sim.actions {
		if a.action.kind == syncalert.KindRefresh && a.action.detail.State == syncalert.CDCDelayStateRecovering {
			recovering++
			if a.action.detail.CurrentDelayMs != 30_000 {
				t.Fatalf("recovering refresh must carry the current delay, got %+v", a.action.detail)
			}
		}
	}
	if recovering == 0 {
		t.Fatalf("a low round while firing should refresh the episode with current values")
	}
	// 连续两轮低才重新计时：之后再高要重新攒满 2 分钟。
	// Only two low rounds in a row restart timing: high again afterwards needs a fresh 2 minutes.
	sim2 := newCDCSim(t, cdcPhaseNone, 60, 2)
	for i := 0; i < 20; i++ {
		sim2.round(200)
		sim2.round(200)
		sim2.round(30)
		sim2.round(30)
	}
	if n := sim2.count(syncalert.KindFire); n != 0 {
		t.Fatalf("two low rounds in a row restart timing; 60s highs must never fire, got %d", n)
	}
}

// 验收：多源作业两个源 5 秒、一个源卡在 900 秒，按本轮最大值判为高并告警；中位数只用作追赶基线。
// Acceptance: a multi-source job with two sources at 5s and one stuck at 900s is high by the round maximum and fires;
// the median only feeds the catch-up baseline.
func TestCDCDelayMultiSourceUsesRoundMax(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 1)
	for i := 0; i < 4; i++ {
		sim.round(5, 900, 5)
	}
	fire, ok := sim.first(syncalert.KindFire)
	if !ok {
		t.Fatalf("one source stuck at 900s among 5s sources must fire")
	}
	if fire.action.detail.CurrentDelayMs != 900_000 || fire.action.detail.WindowMaxDelayMs != 900_000 {
		t.Fatalf("the current delay is the slowest source: %+v", fire.action.detail)
	}
	for _, point := range sim.st.history {
		if point.median != 5_000 || point.max != 900_000 {
			t.Fatalf("history keeps the median for catch-up and the max for display: %+v", point)
		}
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
	// 推断的恢复不带最后一个旧样本（120 秒）作为当前延迟。 / An inferred recovery never reports the old 120s sample.
	if d := resolve.action.detail; d.CurrentDelayMs != 0 || !d.DelayInferred {
		t.Fatalf("inferred recovery must clear the current delay and flag it: %+v", d)
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
	return newCDCServiceFixtureVersion(t, "3.0.0")
}

func newCDCServiceFixtureVersion(t *testing.T, version string) *cdcServiceFixture {
	t.Helper()
	withEventConfig(t, "secret", "http://stx")
	service, _ := newEventTestService(t, version)
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
	// 时间差比作业基线大 2 分钟以上的样本（STX 停机后重投的积压）不计入：同一批里有新样本 5 秒，旧样本 120 秒。
	// Samples whose offset exceeds the job's baseline by more than 2 minutes (backlog replayed after downtime) are not
	// counted: each batch carries a fresh 5s sample and old 120s ones.
	auth := enableChannel(t, f.service, 5)
	for i := 0; i < 8; i++ {
		f.now = f.now.Add(30 * time.Second)
		fresh := f.now.UnixMilli()
		old := f.now.Add(-10 * time.Minute).UnixMilli()
		body := fmt.Sprintf(`[{"jobId":"2001","createdTime":%d,"eventType":"READER_MESSAGE_DELAYED","delayTime":120000},`+
			`{"jobId":"2001","createdTime":%d,"eventType":"READER_MESSAGE_DELAYED","delayTime":5000}]`, old+int64(i), fresh)
		if _, err := f.service.IngestEngineEvents(ctx, 5, auth, []byte(body)); err != nil {
			t.Fatalf("ingest: %v", err)
		}
		f.service.evaluateCDCDelayAlerts(ctx)
	}
	if len(f.b4(syncalert.KindFire)) != 0 {
		t.Fatalf("stale samples must not fire")
	}
	// 对照：同样的通道上，新样本持续 120 秒会告警。 / Control: fresh 120s samples on the same channel fire.
	for i := 0; i < 4; i++ {
		f.now = f.now.Add(30 * time.Second)
		body := fmt.Sprintf(`[{"jobId":"2001","createdTime":%d,"eventType":"READER_MESSAGE_DELAYED","delayTime":120000}]`, f.now.UnixMilli())
		if _, err := f.service.IngestEngineEvents(ctx, 5, auth, []byte(body)); err != nil {
			t.Fatalf("ingest: %v", err)
		}
		f.service.evaluateCDCDelayAlerts(ctx)
	}
	if len(f.b4(syncalert.KindFire)) != 1 {
		t.Fatalf("fresh samples must still fire after the stale ones")
	}
}

// 引擎时钟比 STX 慢 5 分钟（时钟差 > 2 分钟）：样本照常计入（按 STX 收到时间判断），并限频打 warning，不会全部被静默丢弃。
// The engine clock runs 5 minutes behind STX (skew > 2 minutes): samples still count (judged by STX receive time)
// and a rate-limited warning is logged instead of every sample being silently dropped.
func TestCDCDelayClockSkewStillCountsAndWarns(t *testing.T) {
	f := newCDCServiceFixture(t)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	var warnings []time.Duration
	previous := cdcDelaySkewWarn
	cdcDelaySkewWarn = func(_ uint, skew time.Duration) { warnings = append(warnings, skew) }
	t.Cleanup(func() { cdcDelaySkewWarn = previous })
	for i := 0; i < 4; i++ {
		for j := 0; j < 3; j++ {
			f.now = f.now.Add(10 * time.Second)
			f.service.recordCDCDelay(f.job.ID, 120_000, f.now.Add(-5*time.Minute).UnixMilli())
		}
		f.service.evaluateCDCDelayAlerts(context.Background())
	}
	if len(f.b4(syncalert.KindFire)) != 1 {
		t.Fatalf("a skewed engine clock must not hide the delay, fires=%d", len(f.b4(syncalert.KindFire)))
	}
	if len(warnings) != 1 || warnings[0] < 4*time.Minute {
		t.Fatalf("expected one rate-limited skew warning, got %v", warnings)
	}
	// 10 分钟后再打一条。 / Another one after 10 minutes.
	for i := 0; i < 22; i++ {
		f.now = f.now.Add(30 * time.Second)
		f.service.recordCDCDelay(f.job.ID, 120_000, f.now.Add(-5*time.Minute).UnixMilli())
	}
	if len(warnings) != 2 {
		t.Fatalf("expected a second warning after the rate-limit interval, got %d", len(warnings))
	}
}

// 多个 reader 交错上报：引擎时间不递增的样本也计入 B4（以及摘要的窗口最大值），不会被当成重放丢弃。
// Interleaved reports from several readers: samples whose engine time does not increase still reach B4 (and the
// summary window max) instead of being dropped as replays.
func TestCDCDelayInterleavedReadersNotDropped(t *testing.T) {
	f := newCDCServiceFixture(t)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 60, 1)
	auth := enableChannel(t, f.service, 5)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		f.now = f.now.Add(30 * time.Second)
		base := f.now.UnixMilli()
		// reader A 的时间更新、延迟低；reader B 慢一些上报，时间更早、延迟 900 秒。
		// Reader A has the newer time and a low delay; reader B reports later with an older time and 900s delay.
		body := fmt.Sprintf(`[{"jobId":"2001","createdTime":%d,"eventType":"READER_MESSAGE_DELAYED","delayTime":5000},`+
			`{"jobId":"2001","createdTime":%d,"eventType":"READER_MESSAGE_DELAYED","delayTime":900000},`+
			`{"jobId":"2001","createdTime":%d,"eventType":"READER_MESSAGE_DELAYED","delayTime":5000}]`, base, base-1500, base)
		if _, err := f.service.IngestEngineEvents(ctx, 5, auth, []byte(body)); err != nil {
			t.Fatalf("ingest: %v", err)
		}
		f.service.evaluateCDCDelayAlerts(ctx)
	}
	if len(f.b4(syncalert.KindFire)) != 1 {
		t.Fatalf("an interleaved 900s sample must count, fires=%d", len(f.b4(syncalert.KindFire)))
	}
	summary := f.service.jobCDCDelay(f.job.ID)
	if summary == nil || summary.MaxMs != 900_000 || summary.LatestMs != 5000 {
		t.Fatalf("summary should keep the interleaved max and the newest latest value: %+v", summary)
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

// 内容没变的 Refresh 不发：持续 120 秒的作业每 30 秒一轮，只在展示内容（例如"已持续 N 分钟"）变化时才发 Refresh。
// Refreshes with unchanged content are skipped: a job steady at 120s only refreshes when the displayed content (such
// as "已持续 N 分钟") changes, not every 30s round.
func TestCDCDelayRefreshOnlyWhenContentChanges(t *testing.T) {
	f := newCDCServiceFixture(t)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	for i := 0; i < 4; i++ {
		f.round(120)
	}
	if len(f.b4(syncalert.KindFire)) != 1 {
		t.Fatalf("expected fire")
	}
	before := len(f.b4(syncalert.KindRefresh))
	const rounds = 20
	for i := 0; i < rounds; i++ {
		f.round(120)
	}
	refreshes := f.b4(syncalert.KindRefresh)[before:]
	if len(refreshes) == 0 || len(refreshes) > rounds/2+1 {
		t.Fatalf("steady delay should refresh about once a minute (elapsed minutes change), got %d in %d rounds", len(refreshes), rounds)
	}
	previous := f.b4(syncalert.KindFire)[0].Message
	for _, r := range refreshes {
		if r.Message == previous {
			t.Fatalf("a refresh with unchanged content was sent: %q", r.Message)
		}
		previous = r.Message
	}
	// 延迟变了就立刻刷新。 / A changed delay refreshes right away.
	n := len(f.b4(syncalert.KindRefresh))
	f.round(300)
	if len(f.b4(syncalert.KindRefresh)) != n+1 {
		t.Fatalf("a changed delay must refresh")
	}
}

// 告警中改了阈值（rearm）后出现低于新阈值的一轮：阶段内容立刻刷新为新阈值和当前延迟，不再保留"无法确认、阈值 60 秒"的旧内容。
// After the threshold changes while alerting (rearm), a round below the new threshold refreshes the episode with the
// new threshold and current delay at once instead of keeping the stale "unconfirmable, threshold 60s" content.
func TestCDCDelayRearmLowRoundRefreshesCurrentContent(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 1)
	count := 100.0
	sim.readCount = &count
	for i := 0; i < 4; i++ {
		sim.round(120)
	}
	sim.round() // 无法确认（阈值 60 秒）。 / Unconfirmable at threshold 60s.
	if last := sim.actions[len(sim.actions)-1].action; last.detail.State != syncalert.CDCDelayStateUnknown || last.detail.ThresholdSeconds != 60 {
		t.Fatalf("expected an unknown refresh at threshold 60, got %+v", last)
	}
	sim.threshold = 300
	actions := sim.round(120)
	if len(actions) != 1 || actions[0].kind != syncalert.KindRefresh {
		t.Fatalf("a low round during rearm must refresh the content, got %+v", actions)
	}
	d := actions[0].detail
	if d.State != syncalert.CDCDelayStateRecovering || d.ThresholdSeconds != 300 || d.CurrentDelayMs != 120_000 {
		t.Fatalf("refresh must carry the new threshold and the current delay: %+v", d)
	}
	msg := cdcDelayMessage(actions[0], &JobInstance{ID: 3, TaskID: 1})
	if strings.Contains(msg, "无法确认") || !strings.Contains(msg, "5分") {
		t.Fatalf("refreshed message must be current: %q", msg)
	}
	if actions := sim.round(120); len(actions) != 1 || actions[0].kind != syncalert.KindResolve || actions[0].reason != syncalert.ResolveReasonRuleChanged {
		t.Fatalf("second low round closes silently with rule_changed, got %+v", actions)
	}
}

// 读配置期间配置被改（停用）：读到的旧配置不进缓存，下一轮立刻用新配置，不会带着旧配置继续 60 秒。
// The config changes (disabled) while it is being read: the old result is not cached, so the next round uses the new
// config at once instead of the old one for another 60s.
type racingTaskAlertStore struct {
	*memoryTaskAlertStore
	duringRead func()
	reads      int
}

func (r *racingTaskAlertStore) GetTaskAlertConfig(ctx context.Context, taskID uint) (*syncalert.TaskConfig, error) {
	r.reads++
	cfg, err := r.memoryTaskAlertStore.GetTaskAlertConfig(ctx, taskID)
	if hook := r.duringRead; hook != nil {
		r.duringRead = nil
		hook()
	}
	return cfg, err
}

func TestCDCDelayConfigCacheSkipsRacedRead(t *testing.T) {
	f := newCDCServiceFixture(t)
	store := &racingTaskAlertStore{memoryTaskAlertStore: f.store}
	f.service.SetTaskAlertPolicyStore(store)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	ctx := context.Background()
	store.duringRead = func() {
		// 保存停用：存储已更新，失效在读取返回之前完成。 / Save disables it; invalidation lands before the read returns.
		f.store.configs[f.job.TaskID] = (&syncalert.TaskConfig{Configured: true, Enabled: false, Rules: []string{syncalert.RuleCDCDelayHigh}, ChannelIDs: []uint{1}}).Normalize()
		f.service.invalidateCDCDelayConfig(f.job.TaskID)
	}
	if cfg, err := f.service.cdcDelayTaskConfig(ctx, f.job.TaskID, f.now); err != nil || cfg == nil || !cfg.Enabled {
		t.Fatalf("the raced read still returns what it read: %+v err=%v", cfg, err)
	}
	cfg, err := f.service.cdcDelayTaskConfig(ctx, f.job.TaskID, f.now)
	if err != nil || cfg.Enabled || store.reads != 2 {
		t.Fatalf("the raced result must not be cached (reads=%d cfg=%+v)", store.reads, cfg)
	}
	if _, err := f.service.cdcDelayTaskConfig(ctx, f.job.TaskID, f.now); err != nil || store.reads != 2 {
		t.Fatalf("a clean read is cached (reads=%d)", store.reads)
	}
}

// 老客户端的 PUT 不带 cdc_delay_*：保留已保存的自定义值（300 / 10）；明确填 0 才恢复默认；新建时不填用默认值。
// An old client's PUT without cdc_delay_* keeps the stored custom values (300 / 10); only an explicit 0 resets to the
// defaults, and a create without them uses the defaults.
func TestTaskAlertOmittedCDCDelayKeepsStoredValues(t *testing.T) {
	service, _, task := newOutcomeTestService(t, nil)
	ctx := context.Background()
	owner := executionapp.Actor{UserID: 1}
	store := &memoryTaskAlertStore{configs: map[uint]*syncalert.TaskConfig{}, refs: map[uint]syncalert.TaskRef{}}
	service.SetTaskAlertPolicyStore(store)
	rules := []string{syncalert.RuleJobFailed, syncalert.RuleCDCDelayHigh}

	cfg, err := service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: rules, ChannelIDs: []uint{1}})
	if err != nil || cfg.CDCDelayThresholdSeconds != syncalert.DefaultCDCDelayThresholdSeconds || cfg.CDCDelayDurationMinutes != syncalert.DefaultCDCDelayDurationMinutes {
		t.Fatalf("create without the fields uses the defaults: %+v err=%v", cfg, err)
	}
	threshold, duration := 300, 10
	if cfg, err = service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: rules, ChannelIDs: []uint{1}, CDCDelayThresholdSeconds: &threshold, CDCDelayDurationMinutes: &duration}); err != nil || cfg.CDCDelayThresholdSeconds != 300 {
		t.Fatalf("custom values: %+v err=%v", cfg, err)
	}
	// 老客户端：只改了接收人，不认识 cdc_delay_*。 / Old client: only changes receivers and knows nothing of cdc_delay_*.
	cfg, err = service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: rules, ChannelIDs: []uint{1}, ReceiverUserIDs: []uint{7}})
	if err != nil || cfg.CDCDelayThresholdSeconds != 300 || cfg.CDCDelayDurationMinutes != 10 {
		t.Fatalf("omitted fields must keep the stored 300 / 10: %+v err=%v", cfg, err)
	}
	// 只带一个字段：另一个保留。 / Only one field given: the other is kept.
	threshold = 120
	if cfg, err = service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: rules, ChannelIDs: []uint{1}, CDCDelayThresholdSeconds: &threshold}); err != nil || cfg.CDCDelayThresholdSeconds != 120 || cfg.CDCDelayDurationMinutes != 10 {
		t.Fatalf("one field given keeps the other: %+v err=%v", cfg, err)
	}
	zero := 0
	cfg, err = service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: rules, ChannelIDs: []uint{1}, CDCDelayThresholdSeconds: &zero, CDCDelayDurationMinutes: &zero})
	if err != nil || cfg.CDCDelayThresholdSeconds != syncalert.DefaultCDCDelayThresholdSeconds || cfg.CDCDelayDurationMinutes != syncalert.DefaultCDCDelayDurationMinutes {
		t.Fatalf("explicit 0 resets to the defaults: %+v err=%v", cfg, err)
	}
	// JSON 层：字段缺失解码为 nil，明确 0 解码为 0。 / JSON: a missing field decodes to nil, an explicit 0 to 0.
	var req TaskAlertRequest
	if err := json.Unmarshal([]byte(`{"rules":["sync_job_failed"],"cdc_delay_duration_minutes":0}`), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.CDCDelayThresholdSeconds != nil || req.CDCDelayDurationMinutes == nil || *req.CDCDelayDurationMinutes != 0 {
		t.Fatalf("omitted vs explicit 0 must be distinguishable: %+v", req)
	}
}

// 版本门槛：2.3.12 起引擎经事件通道上报 READER_MESSAGE_DELAYED，B4 在 2.3.12 集群上照常告警；版本降到 2.3.11 时静默关闭。
// Version gate: from 2.3.12 the engine reports READER_MESSAGE_DELAYED through the event channel, so B4 fires on a
// 2.3.12 cluster; once the version reads 2.3.11 it closes silently.
func TestCDCDelayVersionGate2312(t *testing.T) {
	f := newCDCServiceFixtureVersion(t, "2.3.12")
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	for i := 0; i < 4; i++ {
		f.round(120)
	}
	if len(f.b4(syncalert.KindFire)) != 1 {
		t.Fatalf("B4 must fire on a 2.3.12 cluster, got %d", len(f.b4(syncalert.KindFire)))
	}
	f.service.SetClusterVersionProvider(fixedClusterVersionProvider{version: "2.3.11"})
	f.round(120)
	resolves := f.b4(syncalert.KindResolve)
	if len(resolves) != 1 || resolves[0].ResolveReason != syncalert.ResolveReasonChannelOff {
		t.Fatalf("an unsupported version closes silently, got %+v", resolves)
	}
}

// Codex P2 / 回归：孤立的尖峰之间隔着收不到数据的时间（H、L、无数据 ×N、H，以及 H、无数据 ×N、H），不能把中间的空白算成
// 超标时间而立刻告警。之后持续超标满 M 分钟照常告警，"已持续"是真实的超标时长，不是几百分钟。
// Codex P2 / regression: isolated spikes separated by time without data (H, L, no data ×N, H, and H, no data ×N,
// H) must not count the gap as time above the threshold and fire at once. Sustained high delay afterwards still
// fires after M minutes, and "已持续" is the real time above the threshold, not hundreds of minutes.
func TestCDCDelayNoDataGapBetweenSpikesDoesNotFire(t *testing.T) {
	for _, withLow := range []bool{true, false} {
		for _, gapRounds := range []int{2, 20, 400} {
			sim := newCDCSim(t, cdcPhaseNone, 60, 5)
			sim.round(200)
			if withLow {
				sim.round(30)
			}
			for i := 0; i < gapRounds; i++ {
				sim.round() // 无样本、读取条数不涨：无法确认。 / No samples, no read progress: unconfirmable.
			}
			spikeAt := sim.now
			sim.round(200)
			if n := sim.count(syncalert.KindFire); n != 0 {
				t.Fatalf("withLow=%v gap=%d: an isolated spike after a gap must not fire, fires=%d", withLow, gapRounds, n)
			}
			for i := 0; i < 12; i++ {
				sim.round(200)
			}
			fire, ok := sim.first(syncalert.KindFire)
			if !ok {
				t.Fatalf("withLow=%v gap=%d: sustained high delay must still fire", withLow, gapRounds)
			}
			if took := fire.at.Sub(spikeAt); took < 5*time.Minute || took > 6*time.Minute {
				t.Fatalf("withLow=%v gap=%d: fire %v after timing restarted, want 5–6 minutes", withLow, gapRounds, took)
			}
			msg := cdcDelayMessage(fire.action, &JobInstance{ID: 3, TaskID: 1})
			var elapsed int
			if i := strings.Index(msg, "已持续 "); i < 0 {
				t.Fatalf("fire message lacks the elapsed time: %q", msg)
			} else if _, err := fmt.Sscanf(msg[i+len("已持续 "):], "%d", &elapsed); err != nil || elapsed < 5 || elapsed > 6 {
				t.Fatalf("withLow=%v gap=%d: fire message must claim 5–6 minutes, not the gap: %q", withLow, gapRounds, msg)
			}
		}
	}
	// 对照：中间只隔一轮非 high（间歇性高延迟）仍然累计。 / Control: a single non-high round in between still adds up.
	sim := newCDCSim(t, cdcPhaseNone, 60, 2)
	for i := 0; i < 10; i++ {
		sim.round(200)
		sim.round()
	}
	if sim.count(syncalert.KindFire) != 1 {
		t.Fatalf("one no-data round between highs must not restart timing, fires=%d", sim.count(syncalert.KindFire))
	}
}

// 多源追赶：一个 reader 卡住越来越慢，其他 reader 在追赶（中位数在降）。最大值没有降到基线最大值的 90% 以下，不算追赶，
// 超标满 M 分钟就告警，不会拖到第 12 分钟。
// Multi-source catch-up: one reader is stuck and growing while the others catch up (the median falls). The max does
// not drop to ≤90% of the baseline max, so it is not catching up and fires once M minutes are reached, not at minute
// 12.
func TestCDCDelayCatchUpRequiresMaxToDrop(t *testing.T) {
	sim := newCDCSim(t, cdcPhaseNone, 60, 5)
	start := sim.now
	stuck := int64(100)
	healthy := []int64{40, 40, 40, 40}
	for sim.now.Sub(start) < 15*time.Minute {
		sim.tick(append([]int64{stuck}, healthy...)...)
		stuck += 10
		for j := range healthy {
			if healthy[j] > 2 {
				healthy[j]--
			}
		}
	}
	fire, ok := sim.first(syncalert.KindFire)
	if !ok {
		t.Fatalf("the stuck reader must fire")
	}
	if took := fire.at.Sub(start); took > 6*time.Minute {
		t.Fatalf("a stuck reader among catching-up readers fired after %v, want ≤6m", took)
	}
	// 对照：所有 reader 一起下降（最大值也降一成以上）仍算追赶，不告警。
	// Control: all readers falling together (the max drops by more than a tenth too) is still catching up.
	sim = newCDCSim(t, cdcPhaseNone, 60, 5)
	value := int64(900)
	for i := 0; i < 36; i++ { // 6 分钟，每 10 秒降 5 秒。 / 6 minutes, 5s lower every 10s.
		sim.tick(value, value-50)
		value -= 5
	}
	if sim.count(syncalert.KindFire) != 0 {
		t.Fatalf("a real catch-up must not fire")
	}
}

// acceptingReporter 记录信号，并可以按规则拒收（模拟队列过半丢弃 Refresh）。
// acceptingReporter records signals and can reject some (simulating Refreshes dropped past half capacity).
type acceptingReporter struct {
	recordingAlertReporter
	reject func(syncalert.Signal) bool
	tried  []syncalert.Signal
}

func (r *acceptingReporter) TryReportSyncAlert(signal syncalert.Signal) bool {
	r.mu.Lock()
	r.tried = append(r.tried, signal)
	reject := r.reject != nil && r.reject(signal)
	r.mu.Unlock()
	if reject {
		return false
	}
	r.ReportSyncAlert(signal)
	return true
}

// 被丢弃的 Refresh 会重发：发送记录在信号被接收之后才更新，所以下一轮重发；改变状态的 Refresh 带 StateChange（接收方
// 不按排队上限丢弃）；内容不变时也每 5 分钟重发一次。
// A dropped Refresh is resent: the send record only updates once a signal is accepted, so the next round sends it
// again; state-changing Refreshes carry StateChange (the receiver does not drop them under the queue limit); and
// unchanged content is still resent every 5 minutes.
func TestCDCDelayDroppedRefreshIsResent(t *testing.T) {
	f := newCDCServiceFixture(t)
	reporter := &acceptingReporter{}
	f.service.SetAlertReporter(reporter)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 30, 1)
	for i := 0; i < 4; i++ {
		f.round(120)
	}
	if len(reporter.find(syncalert.KindFire, syncalert.RuleCDCDelayHigh, "")) != 1 {
		t.Fatalf("expected fire")
	}
	// 告警后的普通高延迟 Refresh 不是状态变化（重启后第一次看到作业的 maybe Refresh 是）。
	// Ordinary high Refreshes after the Fire are not state changes (the maybe Refresh on first sight after a restart
	// is).
	seen := len(reporter.find(syncalert.KindRefresh, syncalert.RuleCDCDelayHigh, ""))
	f.round(150)
	f.round(180)
	highs := reporter.find(syncalert.KindRefresh, syncalert.RuleCDCDelayHigh, "")[seen:]
	if len(highs) == 0 {
		t.Fatalf("a changed delay must refresh")
	}
	for _, r := range highs {
		if r.StateChange {
			t.Fatalf("an ordinary high refresh is not a state change: %+v", r.CDCDelay)
		}
	}
	// 进入"无法确认"的 Refresh 第一次被丢弃。 / The Refresh entering "unconfirmable" is dropped the first time.
	dropped := 0
	reporter.reject = func(s syncalert.Signal) bool {
		if s.Kind == syncalert.KindRefresh && dropped == 0 {
			dropped++
			return true
		}
		return false
	}
	f.round(-1)
	f.round(-1)
	var unknown []syncalert.Signal
	for _, r := range reporter.find(syncalert.KindRefresh, syncalert.RuleCDCDelayHigh, "") {
		if r.CDCDelay != nil && r.CDCDelay.State == syncalert.CDCDelayStateUnknown {
			unknown = append(unknown, r)
		}
	}
	if dropped != 1 || len(unknown) != 1 || !unknown[0].StateChange {
		t.Fatalf("the dropped unknown-entry refresh must be resent next round as a state change (dropped=%d accepted=%+v)", dropped, unknown)
	}
	// 30 分钟无法确认：内容不变，也每 5 分钟重发一次。 / 30 minutes unconfirmable: unchanged content still resends every 5 min.
	before := len(reporter.find(syncalert.KindRefresh, syncalert.RuleCDCDelayHigh, ""))
	for i := 0; i < 60; i++ {
		f.round(-1)
	}
	if got := len(reporter.find(syncalert.KindRefresh, syncalert.RuleCDCDelayHigh, "")) - before; got < 5 || got > 7 {
		t.Fatalf("expected a refresh about every 5 minutes over 30 minutes, got %d", got)
	}
	// 回落待确认也是状态变化。 / Entering recovering is a state change too.
	f.round(5)
	refreshes := reporter.find(syncalert.KindRefresh, syncalert.RuleCDCDelayHigh, "")
	last := refreshes[len(refreshes)-1]
	if last.CDCDelay == nil || last.CDCDelay.State != syncalert.CDCDelayStateRecovering || !last.StateChange {
		t.Fatalf("the recovering refresh must be a state change: %+v", last)
	}
}

// 一条引擎时间快 10 分钟的样本（时钟跳变 / 节点时间不准）不会让后面的样本都被当成旧样本：持续 300 秒的延迟照常告警。
// One sample whose engine time is 10 minutes ahead (clock jump / bad node) does not make later samples look stale:
// a sustained 300s delay still fires.
func TestCDCDelayFutureEngineSampleDoesNotPoisonLaterSamples(t *testing.T) {
	f := newCDCServiceFixture(t)
	f.configure([]string{syncalert.RuleCDCDelayHigh}, 60, 1)
	f.service.recordCDCDelay(f.job.ID, 5_000, f.now.Add(10*time.Minute).UnixMilli())
	for i := 0; i < 10; i++ {
		f.round(300)
	}
	if len(f.b4(syncalert.KindFire)) != 1 {
		t.Fatalf("a single future engine timestamp must not hide later samples, fires=%d", len(f.b4(syncalert.KindFire)))
	}
	if summary := f.service.jobCDCDelay(f.job.ID); summary == nil || summary.LatestMs != 300_000 {
		t.Fatalf("the summary must keep updating after a future sample: %+v", summary)
	}
}

// 多节点时钟差超过 2 分钟：reader A 所在节点时钟快 3 分钟（正常），reader B 卡住 600 秒，B 的样本照常计入并告警；
// 反过来 A 所在节点慢 3 分钟、A 卡住，A 的样本在持续 5 分钟后计入（不是一次性的积压重投），也会告警。
// Inter-node skew over 2 minutes: reader A's node runs 3 minutes ahead (healthy) and reader B is stuck at 600s, so B
// still counts and fires; mirrored, A's node runs 3 minutes behind and A is stuck, and A counts once this has lasted
// 5 minutes (not a one-off replay), so it fires too.
func TestCDCDelayInterNodeSkewKeepsStuckReader(t *testing.T) {
	run := func(skewA time.Duration, stuckA bool, rounds int) (*cdcServiceFixture, time.Time) {
		f := newCDCServiceFixture(t)
		f.configure([]string{syncalert.RuleCDCDelayHigh}, 60, 1)
		start := f.now
		for r := 0; r < rounds; r++ {
			for i := 0; i < 3; i++ {
				f.now = f.now.Add(10 * time.Second)
				delayA, delayB := int64(2_000), int64(600_000)
				if stuckA {
					delayA, delayB = 600_000, 2_000
				}
				f.service.noteCDCDelayBatch(f.job.ID, f.now.UnixMilli(), f.now)
				f.service.recordCDCDelay(f.job.ID, delayB, f.now.UnixMilli())
				f.service.recordCDCDelay(f.job.ID, delayA, f.now.Add(skewA).UnixMilli())
			}
			f.service.evaluateCDCDelayAlerts(context.Background())
		}
		return f, start
	}
	f, _ := run(3*time.Minute, false, 4)
	if len(f.b4(syncalert.KindFire)) != 1 {
		t.Fatalf("a reader on a node 3 minutes ahead must not hide the stuck reader, fires=%d", len(f.b4(syncalert.KindFire)))
	}
	f, start := run(-3*time.Minute, true, 20)
	fires := f.b4(syncalert.KindFire)
	if len(fires) != 1 {
		t.Fatalf("a stuck reader on a node 3 minutes behind must fire once its offset is adopted, fires=%d", len(fires))
	}
	if took := fires[0].OccurredAt.Sub(start); took > 0 && took > cdcDelayOffsetAdoptAfter+2*time.Minute {
		t.Fatalf("fired %v after start, want ≤ adopt window + duration + a round", took)
	}
}
