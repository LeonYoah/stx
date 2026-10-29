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
//   - 同一个渠道本轮失败一次后，跳过它本轮剩下的所有投递。
//
// Sweep budget: the sync alert sweep and signal handling run serially on one goroutine, so a hung channel (each send
// may wait notificationSendTimeout) could stretch one sweep past ten minutes while the signal queue fills up and drops
// signals. Each sweep therefore:
//   - starts no new send after syncAlertSweepBudget (the rest waits for the next sweep);
//   - waits at most syncAlertSweepSendTimeout for one send;
//   - skips every remaining delivery of a channel for the rest of the sweep once it failed in this sweep.
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
	if g == nil {
		return alertSweepSend
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, failed := g.failed[channelID]; failed {
		return alertSweepSkipChannelFailed
	}
	if time.Since(g.startedAt) >= g.budget {
		return alertSweepSkipBudget
	}
	return alertSweepSend
}

// budgetExceeded 报告本轮时间预算是否已用完。
// budgetExceeded reports whether the sweep's time budget is used up.
func (g *alertSweepGuard) budgetExceeded() bool {
	if g == nil {
		return false
	}
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
