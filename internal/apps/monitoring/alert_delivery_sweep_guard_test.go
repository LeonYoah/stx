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
	"testing"
	"time"
)

// beginRetryPhase 在提醒阶段已吃掉大半墙钟时，把重试剩余预算抬回至少一半。
// beginRetryPhase raises the remaining retry budget back to at least half when the reminder phase already burned most of the wall clock.
func TestAlertSweepGuard_BeginRetryPhaseReservesHalfBudget(t *testing.T) {
	g := &alertSweepGuard{
		startedAt: time.Now().Add(-1100 * time.Millisecond),
		budget:    time.Second,
		failed:    map[uint]struct{}{},
	}
	if !g.budgetExceeded() {
		t.Fatal("precondition: remind-phase wall clock should have exhausted the full budget")
	}
	g.beginRetryPhase()
	if g.budgetExceeded() {
		t.Fatal("after beginRetryPhase the full budget must not be exhausted yet")
	}
	if g.check(1) != alertSweepSend {
		t.Fatal("retry phase must still be allowed to send")
	}
	// 剩余应约为半额（允许调度抖动）。
	// Remaining should be about half (allow scheduling jitter).
	remaining := g.budget - time.Since(g.startedAt)
	if remaining < 400*time.Millisecond || remaining > 550*time.Millisecond {
		t.Fatalf("expected ~500ms reserved for retries, got %v", remaining)
	}
}

// 提醒阶段只用了很少时间时，beginRetryPhase 不得前移起点拉长总预算。
// When the reminder phase used little time, beginRetryPhase must not shift startedAt to inflate the total budget.
func TestAlertSweepGuard_BeginRetryPhaseNoOpWhenHalfRemains(t *testing.T) {
	started := time.Now().Add(-100 * time.Millisecond)
	g := &alertSweepGuard{startedAt: started, budget: time.Second, failed: map[uint]struct{}{}}
	g.beginRetryPhase()
	if !g.startedAt.Equal(started) {
		t.Fatalf("startedAt must stay put when half the budget remains, moved by %v", g.startedAt.Sub(started))
	}
}
