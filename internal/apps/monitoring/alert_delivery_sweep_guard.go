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
	"sync"
	"time"
)

// 巡检预算：同步告警巡检和信号处理在同一个协程里串行执行，一个卡住的渠道（每次发送最多等 notificationSendTimeout）
// 会让一轮巡检拖上十几分钟，期间信号队列可能写满丢信号。所以每轮巡检：
//   - 超过 syncAlertSweepBudget 后不再发起新的发送（剩下的留到下一轮）；
//   - 每次发送最多等 syncAlertSweepSendTimeout；
//   - 同一个渠道本轮失败一次后，跳过它本轮剩下的所有投递；
//   - 冷却提醒最多用一半预算；进入退避重试前 beginRetryPhase 再保住至少一半预算（提醒阶段的 DB 巡检也会吃墙钟）；
//     退避重试按渠道轮转（健康渠道优先，见 retryFailedDeliveries），排在后面的健康渠道不会被卡住的渠道饿死。
//
// Sweep budget: the sync alert sweep and signal handling run serially on one goroutine, so a hung channel (each send
// may wait notificationSendTimeout) could stretch one sweep past ten minutes while the signal queue fills up and drops
// signals. Each sweep therefore:
//   - starts no new send after syncAlertSweepBudget (the rest waits for the next sweep);
//   - waits at most syncAlertSweepSendTimeout for one send;
//   - skips every remaining delivery of a channel for the rest of the sweep once it failed in this sweep;
//   - lets cooldown reminders use at most half the budget; beginRetryPhase then reserves at least half again before
//     backoff retries (reminder-phase DB walks burn wall clock too); retries round-robin across channels (healthy ones
//     first, see retryFailedDeliveries), so a healthy channel sorting late is not starved by hung ones.
var (
	syncAlertSweepBudget      = 30 * time.Second
	syncAlertSweepSendTimeout = 20 * time.Second
)

type alertSweepGuardKey struct{}

// alertSweepGuard 记录一轮巡检的开始时间和本轮失败过的渠道。
// alertSweepGuard tracks one sweep's start time and the channels that failed in it.
type alertSweepGuard struct {
	mu        sync.Mutex
	startedAt time.Time
	budget    time.Duration
	failed    map[uint]struct{}
}

// alertSweepSkip 说明本轮巡检为什么不发某个渠道。
// alertSweepSkip explains why the sweep does not send to a channel.
type alertSweepSkip int

const (
	alertSweepSend alertSweepSkip = iota
	// alertSweepSkipChannelFailed：渠道本轮已失败过，算作一次失败（不消耗投递记录的次数）。
	// alertSweepSkipChannelFailed: the channel already failed in this sweep; counts as a failure (no attempt used).
	alertSweepSkipChannelFailed
	// alertSweepSkipBudget：本轮时间预算已用完，延后到下一轮，不算失败。
	// alertSweepSkipBudget: the sweep's time budget is used up; deferred to the next sweep, not a failure.
	alertSweepSkipBudget
)

const alertSweepChannelFailedError = "skipped: notification channel already failed in this sweep / 渠道本轮巡检已失败，跳过"

// withAlertSweepGuard 给一轮巡检挂上预算守卫。
// withAlertSweepGuard attaches a budget guard to one sweep.
func withAlertSweepGuard(ctx context.Context) context.Context {
	return context.WithValue(ctx, alertSweepGuardKey{}, &alertSweepGuard{
		startedAt: time.Now(),
		budget:    syncAlertSweepBudget,
		failed:    map[uint]struct{}{},
	})
}

func alertSweepGuardFrom(ctx context.Context) *alertSweepGuard {
	if ctx == nil {
		return nil
	}
	guard, _ := ctx.Value(alertSweepGuardKey{}).(*alertSweepGuard)
	return guard
}

// check 返回本轮是否还能向该渠道发送。没有守卫（信号处理、手动测试）时总是可以发送。
// check reports whether the sweep may still send to the channel. Without a guard (signal handling, manual tests) the
// answer is always yes.
func (g *alertSweepGuard) check(channelID uint) alertSweepSkip {
	return g.checkPhase(channelID, true)
}

// checkPhase 同 check，但区分巡检阶段：冷却提醒（retry=false）最多用一半预算，保证排在后面的退避重试总有时间。
// checkPhase is check with the sweep phase: cooldown reminders (retry=false) may use at most half the budget, so the
// backoff retries that run afterwards always get time.
func (g *alertSweepGuard) checkPhase(channelID uint, retry bool) alertSweepSkip {
	if g == nil {
		return alertSweepSend
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, failed := g.failed[channelID]; failed {
		return alertSweepSkipChannelFailed
	}
	limit := g.budget
	if !retry {
		limit = g.budget / 2
	}
	if time.Since(g.startedAt) >= limit {
		return alertSweepSkipBudget
	}
	return alertSweepSend
}

// beginRetryPhase 在进入退避重试前调用：冷却提醒阶段即使不发网络（cooldown=0 只做 DB 巡检）也会消耗墙钟，
// 若已用掉超过一半预算，则把计时起点前移，保证重试阶段至少还有一半预算。这样"提醒最多半额、重试总有时间"
// 的契约在慢 CI / 大量 firing 阶段时也成立。
// beginRetryPhase is called before backoff retries: even a reminder phase with no network sends (cooldown=0, DB-only
// walks) burns wall clock; if more than half the budget is already gone, shift startedAt forward so the retry phase
// still has at least half the budget. This keeps the "reminders at most half, retries always get time" contract under
// slow CI or many firing episodes.
func (g *alertSweepGuard) beginRetryPhase() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	half := g.budget / 2
	elapsed := time.Since(g.startedAt)
	if remaining := g.budget - elapsed; remaining < half {
		// 让 time.Since(startedAt) == budget-half，即剩余刚好半额。
		// Make time.Since(startedAt) == budget-half so remaining equals half.
		g.startedAt = time.Now().Add(-(g.budget - half))
	}
}

// budgetExceeded 报告本轮时间预算是否已用完。
// budgetExceeded reports whether the sweep's time budget is used up.
func (g *alertSweepGuard) budgetExceeded() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Since(g.startedAt) >= g.budget
}

func (g *alertSweepGuard) recordFailure(channelID uint) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failed[channelID] = struct{}{}
}

// sendContext 返回单次发送用的 context：巡检内最多等 syncAlertSweepSendTimeout。只用于发送本身，投递记录的读写仍用
// 原来的 ctx，发送超时不会让记录卡在 sending。
// sendContext returns the context for one send: within a sweep it waits at most syncAlertSweepSendTimeout. It only
// wraps the send itself; reading and writing the delivery record keep the original ctx, so a send timeout never leaves
// the record stuck in sending.
func (g *alertSweepGuard) sendContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if g == nil {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, syncAlertSweepSendTimeout)
}
