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
	gosync "sync"
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

const (
	// defaultReconcileClusterConcurrency 是未配置时同时对账的集群数上限。
	// defaultReconcileClusterConcurrency caps concurrently reconciled clusters when nothing is configured.
	defaultReconcileClusterConcurrency = 4
	// defaultReconcileMaxBackoff 是未配置时集群退避的上限。
	// defaultReconcileMaxBackoff caps a cluster's backoff when nothing is configured.
	defaultReconcileMaxBackoff = 120 * time.Second
	// defaultReconcileInterval 是未配置时的对账间隔，也是退避的起点。
	// defaultReconcileInterval is the reconcile interval when nothing is configured, and the first backoff step.
	defaultReconcileInterval = 10 * time.Second
	// directEndpointReconcileKey 是没有集群 ID、按 submit_spec 里的引擎地址直连对账的作业共用的调度键。
	// directEndpointReconcileKey is the scheduling key shared by jobs without a cluster ID that reconcile against
	// the engine address in their submit_spec.
	directEndpointReconcileKey uint = 0
)

// clusterReconcileState 记录单个集群的对账进度与退避。
// clusterReconcileState tracks one cluster's reconcile progress and backoff.
type clusterReconcileState struct {
	inFlight          bool
	consecutiveAborts int
	nextAttemptAt     time.Time
	// firstFailureAt 是这一段连续失败的开始时间，成功后清零；集群级告警用它判断持续了多久。
	// firstFailureAt starts the current run of failures and is cleared on success; cluster alerts use it to tell
	// how long the failure has lasted.
	firstFailureAt time.Time
	// succeededOnce 记录进程启动后是否成功过一次。
	// succeededOnce records whether the cluster has succeeded once since process start.
	succeededOnce bool
}

// clusterReconcileOutcome 描述一次集群对账结束后状态的变化，调用方据此打日志、发信号。
// clusterReconcileOutcome describes how a cluster's state changed after one reconcile, so the caller can log and
// emit signals.
type clusterReconcileOutcome struct {
	// Aborted 表示本轮因传输层失败或没有端点而提前结束。
	// Aborted means this pass stopped early on a transport failure or a missing endpoint.
	Aborted bool
	// EnteredBackoff 表示这是连续失败的第一轮（从正常进入退避）。
	// EnteredBackoff means this is the first failed pass in a row (normal → backoff).
	EnteredBackoff bool
	// Recovered 表示连续失败之后的第一次成功。
	// Recovered means the first success after a run of failures.
	Recovered bool
	// FirstSuccess 表示进程启动后这个集群第一次成功，用来结束重启前遗留的告警。
	// FirstSuccess means the cluster's first success since process start, used to close alerts left over from
	// before a restart.
	FirstSuccess      bool
	ConsecutiveAborts int
	Delay             time.Duration
	NextAttemptAt     time.Time
	FirstFailureAt    time.Time
}

// clusterReconcileScheduler 让各集群的对账在后台并行执行：慢集群不占用整个 tick，上一轮没结束的集群本轮跳过，
// 连续失败的集群按 10、20、40…秒推迟下一轮。
// clusterReconcileScheduler runs each cluster's reconcile in the background in parallel: a slow cluster does not
// hold up the whole tick, a cluster whose previous pass is still running is skipped, and a cluster that keeps
// failing has its next pass pushed back by 10, 20, 40… seconds.
type clusterReconcileScheduler struct {
	mu     gosync.Mutex
	states map[uint]*clusterReconcileState
	sem    chan struct{}
	// backfillInFlight 防止终态指标补拉在上一次没结束时重复启动。
	// backfillInFlight stops the terminal-metrics backfill from starting again while the previous one still runs.
	backfillInFlight bool
	now              func() time.Time
	// wg 只用来让测试等待后台对账结束。
	// wg only lets tests wait for background reconciles to finish.
	wg gosync.WaitGroup
}

func (r *clusterReconcileScheduler) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *clusterReconcileScheduler) stateLocked(key uint) *clusterReconcileState {
	if r.states == nil {
		r.states = map[uint]*clusterReconcileState{}
	}
	state, ok := r.states[key]
	if !ok {
		state = &clusterReconcileState{}
		r.states[key] = state
	}
	return state
}

// semaphoreLocked 按配置的并发上限懒创建信号量；运行中改配置要重启才生效。
// semaphoreLocked lazily creates the semaphore with the configured concurrency; a config change needs a restart.
func (r *clusterReconcileScheduler) semaphoreLocked() chan struct{} {
	if r.sem == nil {
		r.sem = make(chan struct{}, reconcileClusterConcurrency())
	}
	return r.sem
}

// tryDispatch 在集群空闲且不在退避期时，后台执行 run，结束后调用 done；返回是否已分派。
// tryDispatch runs run in the background when the cluster is idle and not backing off, then calls done; it
// reports whether the pass was dispatched.
func (r *clusterReconcileScheduler) tryDispatch(ctx context.Context, key uint, run func(context.Context) error, done func(error)) bool {
	r.mu.Lock()
	state := r.stateLocked(key)
	if state.inFlight || r.clock().Before(state.nextAttemptAt) {
		r.mu.Unlock()
		return false
	}
	state.inFlight = true
	sem := r.semaphoreLocked()
	r.wg.Add(1)
	r.mu.Unlock()

	go func() {
		defer r.wg.Done()
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			r.release(key)
			return
		}
		defer func() { <-sem }()
		err := run(ctx)
		done(err)
	}()
	return true
}

// release 只清除进行中标记，不改变退避状态（服务退出时用）。
// release only clears the in-flight flag without touching backoff (used on shutdown).
func (r *clusterReconcileScheduler) release(key uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stateLocked(key).inFlight = false
}

// finish 记录一次对账的结果并计算下一轮的时间：成功时复位；失败时按 interval 翻倍推迟，不超过 maxBackoff。
// 直连键（集群 ID 为 0）的作业可能分属不同地址，不做退避。
// finish records one pass's result and schedules the next one: success resets the state; failure pushes the next
// pass back by interval doubled each time, up to maxBackoff. The direct-endpoint key (cluster ID 0) may span
// several addresses, so it never backs off.
func (r *clusterReconcileScheduler) finish(key uint, err error, interval, maxBackoff time.Duration) clusterReconcileOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.stateLocked(key)
	state.inFlight = false
	if key == directEndpointReconcileKey {
		return clusterReconcileOutcome{Aborted: err != nil}
	}
	now := r.clock()
	if err == nil {
		// 恢复时带回失败了几轮、从什么时候开始，供日志和告警恢复使用。
		// On recovery report how many passes failed and since when, for logs and alert resolution.
		outcome := clusterReconcileOutcome{
			Recovered:         state.consecutiveAborts > 0,
			FirstSuccess:      !state.succeededOnce,
			ConsecutiveAborts: state.consecutiveAborts,
			FirstFailureAt:    state.firstFailureAt,
		}
		state.succeededOnce = true
		state.consecutiveAborts = 0
		state.nextAttemptAt = time.Time{}
		state.firstFailureAt = time.Time{}
		return outcome
	}
	state.consecutiveAborts++
	if state.firstFailureAt.IsZero() {
		state.firstFailureAt = now
	}
	delay := reconcileBackoffDelay(state.consecutiveAborts, interval, maxBackoff)
	state.nextAttemptAt = now.Add(delay)
	return clusterReconcileOutcome{
		Aborted:           true,
		EnteredBackoff:    state.consecutiveAborts == 1,
		ConsecutiveAborts: state.consecutiveAborts,
		Delay:             delay,
		NextAttemptAt:     state.nextAttemptAt,
		FirstFailureAt:    state.firstFailureAt,
	}
}

// inBackoff 判断集群当前是否处于退避期。
// inBackoff reports whether the cluster is currently backing off.
func (r *clusterReconcileScheduler) inBackoff(key uint) bool {
	if key == directEndpointReconcileKey {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.states[key]
	return ok && r.clock().Before(state.nextAttemptAt)
}

// tryStartBackfill 在上一次终态指标补拉结束后才允许启动新的一次。
// tryStartBackfill only lets a new terminal-metrics backfill start once the previous one has finished.
func (r *clusterReconcileScheduler) tryStartBackfill() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.backfillInFlight {
		return false
	}
	r.backfillInFlight = true
	r.wg.Add(1)
	return true
}

func (r *clusterReconcileScheduler) finishBackfill() {
	r.mu.Lock()
	r.backfillInFlight = false
	r.mu.Unlock()
	r.wg.Done()
}

// wait 等待所有后台对账结束，只给测试用。
// wait blocks until every background reconcile has finished; tests only.
func (r *clusterReconcileScheduler) wait() {
	r.wg.Wait()
}

// reconcileBackoffDelay 计算第 n 次连续失败后的推迟时间：interval × 2^(n-1)，不超过 maxBackoff。
// reconcileBackoffDelay returns the delay after the n-th failure in a row: interval × 2^(n-1), capped at maxBackoff.
func reconcileBackoffDelay(consecutiveAborts int, interval, maxBackoff time.Duration) time.Duration {
	if interval <= 0 {
		interval = defaultReconcileInterval
	}
	if maxBackoff < interval {
		maxBackoff = interval
	}
	delay := interval
	for i := 1; i < consecutiveAborts; i++ {
		delay *= 2
		if delay >= maxBackoff {
			return maxBackoff
		}
	}
	return delay
}

// reconcileInterval 返回配置的对账间隔；关闭或未配置时用 10 秒（只用于计算退避）。
// reconcileInterval returns the configured reconcile interval; 10s when unset or disabled (only used for backoff).
func reconcileInterval() time.Duration {
	if config.Config == nil || config.Config.Sync.JobReconcilerIntervalSeconds <= 0 {
		return defaultReconcileInterval
	}
	return time.Duration(config.Config.Sync.JobReconcilerIntervalSeconds) * time.Second
}

// reconcileClusterConcurrency 返回同时对账的集群数上限。
// reconcileClusterConcurrency returns how many clusters may reconcile at once.
func reconcileClusterConcurrency() int {
	if config.Config == nil || config.Config.Sync.JobReconcilerClusterConcurrency <= 0 {
		return defaultReconcileClusterConcurrency
	}
	return config.Config.Sync.JobReconcilerClusterConcurrency
}

// reconcileMaxBackoff 返回集群退避的上限。
// reconcileMaxBackoff returns the cap of a cluster's backoff.
func reconcileMaxBackoff() time.Duration {
	if config.Config == nil || config.Config.Sync.JobReconcilerMaxBackoffSeconds <= 0 {
		return defaultReconcileMaxBackoff
	}
	return time.Duration(config.Config.Sync.JobReconcilerMaxBackoffSeconds) * time.Second
}
