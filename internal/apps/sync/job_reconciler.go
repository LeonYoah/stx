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
	"errors"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

const (
	// jobFinalMetricsGiveUpFails: after this many GetJobInfo failures on a finished list hit, settle status anyway.
	// 终态列表命中但 GetJobInfo 连续失败达到该次数后，放弃等指标并落状态。
	jobFinalMetricsGiveUpFails = 3
	// jobFinalMetricsGiveUpAge: alternate give-up when the job row is older than this without final metrics.
	jobFinalMetricsGiveUpAge = 2 * time.Minute
	// jobTerminalMetricsBackfillWindow: how far back to re-check terminal jobs missing a final metrics block.
	jobTerminalMetricsBackfillWindow = 24 * time.Hour
)

// StartJobReconciler 启动按集群对账 SeaTunnel 作业状态的后台任务。
// StartJobReconciler starts the per-cluster SeaTunnel job status reconciler.
func (s *Service) StartJobReconciler(ctx context.Context) {
	if s == nil || s.repo == nil || s.engineClient == nil {
		return
	}
	intervalSeconds := 10
	if config.Config != nil {
		if config.Config.Sync.JobReconcilerIntervalSeconds < 0 {
			return
		}
		if config.Config.Sync.JobReconcilerIntervalSeconds > 0 {
			intervalSeconds = config.Config.Sync.JobReconcilerIntervalSeconds
		}
	}
	interval := time.Duration(intervalSeconds) * time.Second
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.reconcileActiveJobs(ctx)
			}
		}
	}()
}

// reconcileActiveJobs 跑一轮对账：停止超时标记和本地作业在当前协程完成；各集群的引擎查询交给调度器在后台并行执行，
// 不等它们结束，所以一个慢集群不会拖住其他集群。
// reconcileActiveJobs runs one reconcile pass: stop-overdue marks and local jobs are handled inline, while each
// cluster's engine queries are dispatched to the scheduler and run in the background in parallel without being
// awaited, so one slow cluster cannot hold up the others.
func (s *Service) reconcileActiveJobs(ctx context.Context) error {
	// 没有活跃作业时也要补拉刚失败作业的错误信息。/ Backfill late error messages even when no job is active.
	s.backfillFailedJobErrors(ctx, time.Now())
	jobs, err := s.repo.ListNonTerminalJobInstances(ctx, 500)
	if err != nil || len(jobs) == 0 {
		return err
	}
	taskCluster := map[uint]uint{}
	grouped := map[uint][]*JobInstance{}
	var direct []*JobInstance
	now := time.Now()
	for _, job := range jobs {
		if job == nil {
			continue
		}
		// 停止超时只看本地记录的截止时间，引擎不可达时也要能标记。
		// Stop overdue only checks the locally recorded deadline, so it is flagged even when the engine is unreachable.
		s.markCancelOverdue(ctx, job, now)
		if submitSpecExecutionMode(job.SubmitSpec) == "local" {
			if _, refreshErr := s.refreshLocalJob(ctx, job); refreshErr != nil {
				s.recordJobSyncFailure(ctx, job)
			}
			continue
		}
		clusterID := clusterIDForJob(ctx, s, job, taskCluster)
		if clusterID == 0 {
			// 无 cluster_id 但 submit_spec 带引擎地址时仍对账（直连/外部_ref 兜底）。
			// Still reconcile when cluster_id is missing but submit_spec has an engine endpoint.
			if endpointFromSubmitSpec(job.SubmitSpec) == nil {
				continue
			}
			direct = append(direct, job)
			continue
		}
		grouped[clusterID] = append(grouped[clusterID], job)
	}
	if len(direct) > 0 {
		// 直连作业可能分属不同地址，逐个对账、互不止损，整体作为一个调度单元，不占用 tick。
		// Direct jobs may point at different addresses, so they reconcile one by one without stopping each other,
		// as one scheduling unit that does not hold the tick.
		s.reconcile.tryDispatch(ctx, directEndpointReconcileKey, func(ctx context.Context) error {
			for _, job := range direct {
				_ = s.reconcileClusterJobs(ctx, 0, []*JobInstance{job})
			}
			return nil
		}, func(err error) {
			s.reconcile.finish(directEndpointReconcileKey, err, reconcileInterval(), reconcileMaxBackoff())
		})
	}
	channels := s.eventChannelSnapshot(ctx)
	for clusterID, items := range grouped {
		s.reconcile.tryDispatch(ctx, clusterID, func(ctx context.Context) error {
			// 事件通道健康时只做轻量拉取，完整对账（running/finished 列表）降到每 60 秒一次兜底。
			// With a healthy event channel only light pulls run; full reconciles (running/finished lists) drop to a
			// 60-second safety net.
			if s.useEventModeReconcile(ctx, clusterID, channels) && !s.fullReconcileDue(clusterID) {
				return s.reconcileClusterJobsLight(ctx, clusterID, items)
			}
			if err := s.reconcileClusterJobs(ctx, clusterID, items); err != nil {
				return err
			}
			s.markFullReconcile(clusterID)
			return nil
		}, func(err error) {
			s.finishClusterReconcile(ctx, clusterID, err)
		})
	}
	s.startTerminalMetricsBackfill(ctx)
	return nil
}

// finishClusterReconcile 记录集群本轮的结果；只在进入退避和恢复时各打一条日志，不每轮都打。
// finishClusterReconcile records the cluster's result for this pass; it logs once when backoff starts and once on
// recovery, never on every pass.
func (s *Service) finishClusterReconcile(ctx context.Context, clusterID uint, err error) {
	outcome := s.reconcile.finish(clusterID, err, reconcileInterval(), reconcileMaxBackoff())
	s.reportClusterStatus(clusterID, outcome, err)
	switch {
	case outcome.EnteredBackoff:
		logger.WarnF(ctx, "[Sync] cluster engine queries failed, backing off reconcile: cluster=%d reason=%v next_attempt_at=%s",
			clusterID, err, outcome.NextAttemptAt.UTC().Format(time.RFC3339))
	case outcome.Recovered:
		logger.InfoF(ctx, "[Sync] cluster engine queries recovered, reconcile back to normal: cluster=%d failed_passes=%d since=%s",
			clusterID, outcome.ConsecutiveAborts, outcome.FirstFailureAt.UTC().Format(time.RFC3339))
	}
}

// clusterReconcileBackingOff 判断作业所在集群是否处于退避期；处于退避时刷新路径不再请求引擎。
// clusterReconcileBackingOff reports whether the job's cluster is backing off; refreshes skip the engine meanwhile.
func (s *Service) clusterReconcileBackingOff(ctx context.Context, job *JobInstance) bool {
	clusterID := clusterIDForJob(ctx, s, job, map[uint]uint{})
	return clusterID != 0 && s.reconcile.inBackoff(clusterID)
}

// recordRemainingSyncFailures 在本轮止损时，给还没处理到的作业各记一次同步失败。
// recordRemainingSyncFailures records one sync failure for every job not reached when the pass stops early.
func (s *Service) recordRemainingSyncFailures(ctx context.Context, jobs []*JobInstance) {
	for _, job := range jobs {
		if job == nil || strings.TrimSpace(job.EngineJobID) == "" {
			continue
		}
		s.recordJobSyncFailure(ctx, job)
	}
}

const (
	// eventModeUnwatchedRefreshInterval 是事件模式下无人查看的运行中作业拉 job-info 的间隔（与 metrics_log 对齐）。
	// eventModeUnwatchedRefreshInterval is how often unwatched running jobs pull job-info in event mode (aligned with metrics_log).
	eventModeUnwatchedRefreshInterval = 30 * time.Second
)

// eventModeFullReconcileInterval 读取配置；返回 0 表示关闭自适应（默认）。
// 自适应轮询默认关闭：事件平均晚到约 8 秒，不比 10 秒轮询快；放宽轮询只换来更少的引擎调用，
// 代价是事件悄悄丢失时状态最长滞后一个完整对账间隔。只有需要减轻引擎压力的大集群才配置正数开启。
// eventModeFullReconcileInterval reads the configuration; 0 means adaptive polling is disabled (the default).
// Adaptive polling is off by default: events arrive about 8s late on average, no faster than the 10s poll, so
// relaxing the poll only saves engine calls while a silently lost event can leave status stale for a whole full
// reconcile interval. Only large clusters that need to shed engine load should enable it with a positive value.
func eventModeFullReconcileInterval() time.Duration {
	if config.Config == nil {
		return 0
	}
	seconds := config.Config.Sync.JobReconcilerEventModeIntervalSeconds
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// eventChannelSnapshot 读取全部事件通道，失败时返回空表（等同于没有事件通道，保持原有轮询）。
// eventChannelSnapshot loads all event channels; on failure it returns an empty map (no channels, original polling).
func (s *Service) eventChannelSnapshot(ctx context.Context) map[uint]*EngineEventChannel {
	snapshot := map[uint]*EngineEventChannel{}
	if s == nil || s.repo == nil || eventModeFullReconcileInterval() <= 0 {
		return snapshot
	}
	channels, err := s.repo.ListEngineEventChannels(ctx)
	if err != nil {
		return snapshot
	}
	for _, channel := range channels {
		if channel != nil {
			snapshot[channel.ClusterID] = channel
		}
	}
	return snapshot
}

// useEventModeReconcile 判断集群能否走事件模式：通道健康、已开启非终态上报且集群版本支持。
// useEventModeReconcile reports whether a cluster can use event mode: healthy channel, non-terminal reporting on,
// and a supporting cluster version.
func (s *Service) useEventModeReconcile(ctx context.Context, clusterID uint, channels map[uint]*EngineEventChannel) bool {
	channel := channels[clusterID]
	if clusterID == 0 || channel == nil || !channel.ReportNonTerminal || eventChannelState(channel) != EventChannelStateHealthy {
		return false
	}
	return seatunnel.SupportsNonTerminalJobStateEvents(s.clusterVersion(ctx, clusterID))
}

// fullReconcileDue 判断距离上次完整对账是否已超过事件模式间隔。
// fullReconcileDue reports whether the event-mode interval has passed since the last full reconcile.
func (s *Service) fullReconcileDue(clusterID uint) bool {
	interval := eventModeFullReconcileInterval()
	if interval <= 0 {
		return true
	}
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	last, ok := state.lastFullReconcile[clusterID]
	return !ok || state.clock().Sub(last) >= interval
}

func (s *Service) markFullReconcile(clusterID uint) {
	state := &s.events
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.lastFullReconcile == nil {
		state.lastFullReconcile = map[uint]time.Time{}
	}
	state.lastFullReconcile[clusterID] = state.clock()
}

// reconcileClusterJobsLight 是事件模式下的轻量对账：不拉列表，只给有人查看或超过 30 秒未同步的运行中作业拉 job-info。
// 状态变化由事件触发刷新；其余情况由周期性的完整对账兜底。
// reconcileClusterJobsLight is the event-mode light pass: no list calls, only job-info for running jobs that are
// watched or unsynced for 30s. Status changes are refreshed by events; everything else is covered by the periodic
// full reconcile.
// 遇到传输层失败时结束本轮并返回该错误，剩下的作业各记一次同步失败。
// A transport failure ends the pass and is returned; the remaining jobs each record one sync failure.
func (s *Service) reconcileClusterJobsLight(ctx context.Context, clusterID uint, jobs []*JobInstance) error {
	now := s.events.clock()
	var due []*JobInstance
	for _, job := range jobs {
		if job == nil || job.Status != JobStatusRunning || strings.TrimSpace(job.EngineJobID) == "" {
			continue
		}
		stale := job.LastSyncedAt == nil || now.Sub(*job.LastSyncedAt) >= eventModeUnwatchedRefreshInterval
		if s.jobWatchedRecently(job.ID) || stale {
			due = append(due, job)
		}
	}
	if len(due) == 0 {
		return nil
	}
	var endpoint *EngineEndpoint
	if s.runtimeResolver != nil {
		if resolved, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil); err == nil {
			endpoint = resolved
		}
	}
	for i, job := range due {
		target := endpoint
		if target == nil {
			target = endpointFromSubmitSpec(job.SubmitSpec)
		}
		if target == nil {
			s.recordJobSyncFailure(ctx, job)
			continue
		}
		detail, err := s.engineClient.GetJobInfo(ctx, target, strings.TrimSpace(job.EngineJobID))
		if err != nil {
			s.recordJobSyncFailure(ctx, job)
			if isEngineTransportError(err) {
				s.recordRemainingSyncFailures(ctx, due[i+1:])
				return err
			}
			continue
		}
		if _, err := s.applyEngineJobInfoUpdate(ctx, job, detail); err != nil {
			s.recordJobSyncFailure(ctx, job)
		}
	}
	return nil
}

func clusterIDForJob(ctx context.Context, s *Service, job *JobInstance, cache map[uint]uint) uint {
	if job == nil {
		return 0
	}
	if id := uintValue(job.SubmitSpec, "cluster_id"); id > 0 {
		return id
	}
	if job.TaskID == 0 || s == nil || s.repo == nil {
		return 0
	}
	if cached, ok := cache[job.TaskID]; ok {
		return cached
	}
	task, err := s.repo.GetTaskByID(ctx, job.TaskID)
	if err != nil || task == nil {
		return 0
	}
	cache[job.TaskID] = task.ClusterID
	return task.ClusterID
}

// errReconcileNoEndpoint 表示集群当前找不到可用的引擎端点，本轮对账无法进行。
// errReconcileNoEndpoint means the cluster has no usable engine endpoint, so this pass cannot run.
var errReconcileNoEndpoint = errors.New("sync: no usable engine endpoint")

// reconcileClusterJobs 对一个集群做完整对账：running 列表、finished 列表，再按需拉 job-info。
// 返回非 nil 表示本轮提前结束（没有端点、running 列表失败、或任意一次请求遇到传输层失败），调度器据此退避；
// 引擎返回非 2xx 的 finished 列表或 job-info 不算，保持原来的逐个处理。
// reconcileClusterJobs fully reconciles one cluster: the running list, the finished lists, then job-info as needed.
// A non-nil result means the pass stopped early (no endpoint, running list failed, or any call hit a transport
// failure) and the scheduler backs off; a non-2xx finished list or job-info does not count and keeps the
// previous one-by-one handling.
func (s *Service) reconcileClusterJobs(ctx context.Context, clusterID uint, jobs []*JobInstance) error {
	if len(jobs) == 0 {
		return nil
	}
	var endpoint *EngineEndpoint
	if s.runtimeResolver != nil {
		resolved, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil)
		if err == nil {
			endpoint = resolved
		}
	}
	if endpoint == nil {
		for _, job := range jobs {
			endpoint = endpointFromSubmitSpec(job.SubmitSpec)
			if endpoint != nil {
				break
			}
		}
	}
	if endpoint == nil {
		for _, job := range jobs {
			s.recordJobSyncFailure(ctx, job)
		}
		return errReconcileNoEndpoint
	}

	running, runErr := s.engineClient.ListRunningJobs(ctx, endpoint)
	if runErr != nil {
		for _, job := range jobs {
			s.recordJobSyncFailure(ctx, job)
		}
		return runErr
	}
	runningMap := map[string]*EngineJobListItem{}
	for _, item := range running {
		if item == nil {
			continue
		}
		runningMap[strings.TrimSpace(item.JobID)] = item
	}

	finishedMap := map[string]*EngineJobListItem{}
	for _, state := range []string{"", "FINISHED", "FAILED", "CANCELED", "CANCELLED", "SAVEPOINT_DONE"} {
		items, err := s.engineClient.ListFinishedJobs(ctx, endpoint, state)
		if err != nil {
			// 超时或连不上：剩下的列表和 job-info 本轮都不再请求。某个状态的接口不存在（非 2xx）照旧跳过。
			// Timeout or unreachable: skip the remaining lists and job-info this pass. A missing state endpoint
			// (non-2xx) is still just skipped.
			if isEngineTransportError(err) {
				s.recordRemainingSyncFailures(ctx, jobs)
				return err
			}
			continue
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			finishedMap[strings.TrimSpace(item.JobID)] = item
		}
	}

	for i, job := range jobs {
		if job == nil {
			continue
		}
		engineJobID := strings.TrimSpace(job.EngineJobID)
		if engineJobID == "" {
			continue
		}
		if item, ok := runningMap[engineJobID]; ok {
			// Prefer job-info so running metrics (and 30s metrics_log) are captured, not only list status.
			// 优先拉 job-info，才能写入运行中指标与 30s metrics_log，而不是只同步列表状态。
			detail, detailErr := s.engineClient.GetJobInfo(ctx, endpoint, engineJobID)
			if detailErr != nil {
				if _, err := s.applyEngineJobInfoUpdate(ctx, job, engineListItemToInfo(item)); err != nil {
					s.recordJobSyncFailure(ctx, job)
				}
				if isEngineTransportError(detailErr) {
					s.recordRemainingSyncFailures(ctx, jobs[i+1:])
					return detailErr
				}
				continue
			}
			if _, err := s.applyEngineJobInfoUpdate(ctx, job, detail); err != nil {
				s.recordJobSyncFailure(ctx, job)
			}
			continue
		}
		if item, ok := finishedMap[engineJobID]; ok {
			// Always fetch job-info for finished jobs: finished list omits metrics, which would skip end metrics_log.
			// 终态必须拉 job-info：finished 列表无 metrics，否则会漏掉终态 metrics_log。
			info := engineListItemToInfo(item)
			detail, detailErr := s.engineClient.GetJobInfo(ctx, endpoint, engineJobID)
			if detailErr != nil {
				s.recordJobSyncFailure(ctx, job)
				// Do not permanently settle from list-only mid-run metrics; retry GetJobInfo on later ticks.
				// 不要仅凭列表落终态并冻住中途指标；后续 tick 继续补拉 job-info。
				giveUp := hasFinalMetricsLog(job.ResultPreview) ||
					job.SyncFailCount >= jobFinalMetricsGiveUpFails ||
					time.Since(job.UpdatedAt) >= jobFinalMetricsGiveUpAge
				if giveUp {
					if _, err := s.applyEngineJobInfoUpdate(ctx, job, info); err != nil {
						s.recordJobSyncFailure(ctx, job)
					}
				}
				if isEngineTransportError(detailErr) {
					s.recordRemainingSyncFailures(ctx, jobs[i+1:])
					return detailErr
				}
				continue
			}
			if _, err := s.applyEngineJobInfoUpdate(ctx, job, detail); err != nil {
				s.recordJobSyncFailure(ctx, job)
			}
			continue
		}
		// 列表里暂时看不到：可能是刚提交，或引擎侧已丢；低频打一次详情。
		// Not visible in list yet: may be freshly submitted or missing on engine; fetch detail occasionally.
		if time.Since(job.UpdatedAt) < 15*time.Second {
			continue
		}
		detail, detailErr := s.engineClient.GetJobInfo(ctx, endpoint, engineJobID)
		if detailErr != nil {
			s.recordJobSyncFailure(ctx, job)
			if isEngineTransportError(detailErr) {
				s.recordRemainingSyncFailures(ctx, jobs[i+1:])
				return detailErr
			}
			continue
		}
		if engineJobInfoLooksOrphaned(detail) {
			// 提交结果未知：截止前引擎可能还在规划作业，继续等；过了截止时间才判失败。
			// Unknown submit outcome: the engine may still be planning before the deadline; fail it only afterwards.
			if submitOutcomeIsUnknown(job) {
				if submitOutcomePending(job, time.Now()) {
					continue
				}
				s.expireUnknownSubmit(ctx, job, engineJobID)
				continue
			}
			// Cluster is up but the job vanished (restart / history expiry): end the zombie running state.
			// 集群可达但作业已消失（重启/历史过期）：结束僵死的 running。
			if time.Since(job.CreatedAt) < 30*time.Second {
				continue
			}
			orphaned := &EngineJobInfo{
				JobID:     engineJobID,
				JobStatus: "FAILED",
				ErrorMsg:  "engine no longer has this job (cluster restart or history expired) / 引擎已无此作业（集群重启或历史过期）",
				// 带时区偏移：无偏移字符串会被 parseEngineJobTime 按 SeaTunnel 时区解析，服务器时区不同就会偏移数小时。
				// Include the offset: a naive string is parsed in the SeaTunnel zone by parseEngineJobTime and lands
				// hours off whenever the server zone differs.
				FinishedTime: time.Now().UTC().Format(time.RFC3339),
			}
			if _, err := s.applyEngineJobInfoUpdate(ctx, job, orphaned); err != nil {
				s.recordJobSyncFailure(ctx, job)
			}
			continue
		}
		if _, err := s.applyEngineJobInfoUpdate(ctx, job, detail); err != nil {
			s.recordJobSyncFailure(ctx, job)
		}
	}
	return nil
}

// expireUnknownSubmit 在引擎可达且确认没有该作业、并且已过提交截止时间时，把待确认作业判为失败。
// expireUnknownSubmit fails a pending unknown-outcome job once the engine is reachable, confirms it has no such job,
// and the submit deadline has passed.
func (s *Service) expireUnknownSubmit(ctx context.Context, job *JobInstance, engineJobID string) {
	now := time.Now()
	markSubmitOutcome(job, submitOutcomeExpired, now)
	if err := s.repo.UpdateJobSyncMetadata(ctx, job.ID, map[string]any{"submit_spec": job.SubmitSpec}); err != nil {
		s.recordJobSyncFailure(ctx, job)
		return
	}
	expired := &EngineJobInfo{
		JobID:        engineJobID,
		JobStatus:    "FAILED",
		ErrorMsg:     submitOutcomeExpiredMessage(job),
		// 带时区偏移，避免按 SeaTunnel 时区解析后偏移数小时（同上方孤儿作业）。
		// Include the offset so parsing in the SeaTunnel zone does not shift it by hours (same as orphaned jobs above).
		FinishedTime: now.UTC().Format(time.RFC3339),
	}
	updated, err := s.applyEngineJobInfoUpdate(ctx, job, expired)
	if err != nil {
		s.recordJobSyncFailure(ctx, job)
		return
	}
	logger.WarnF(ctx, "[Sync] unknown submit outcome expired, job marked failed: job=%d task=%d engine_job_id=%s",
		job.ID, job.TaskID, engineJobID)
	if updated != nil && updated.Status == JobStatusFailed {
		s.reportSubmitExpired(ctx, updated)
	}
}

// markCancelOverdue 在取消中作业越过停止截止时间时记录一次 cancel_overdue_at；状态保持不变。
// markCancelOverdue records cancel_overdue_at once when a cancelling job passes its stop deadline; status is unchanged.
func (s *Service) markCancelOverdue(ctx context.Context, job *JobInstance, now time.Time) {
	if !isCancelOverdue(job, now) || strings.TrimSpace(stringValue(job.ResultPreview, cancelOverdueAtKey)) != "" {
		return
	}
	job.ResultPreview[cancelOverdueAtKey] = now.UTC().Format(time.RFC3339)
	if err := s.repo.UpdateJobStatus(ctx, job.ID, []JobStatus{job.Status}, map[string]any{"result_preview": job.ResultPreview}); err != nil {
		return
	}
	logger.WarnF(ctx, "[Sync] job did not stop before its stop deadline: job=%d task=%d engine_job_id=%s deadline=%s",
		job.ID, job.TaskID, strings.TrimSpace(job.EngineJobID), stringValue(job.ResultPreview, cancelDeadlineKey))
	s.reportStopOverdue(ctx, job)
}

// startTerminalMetricsBackfill 在后台补拉终态指标，上一次没结束时不再启动，不占用对账 tick。
// startTerminalMetricsBackfill backfills terminal metrics in the background; it does not start while the previous
// run is still going and never holds the reconcile tick.
func (s *Service) startTerminalMetricsBackfill(ctx context.Context) {
	if s == nil || s.repo == nil || s.engineClient == nil || !s.reconcile.tryStartBackfill() {
		return
	}
	go func() {
		defer s.reconcile.finishBackfill()
		s.backfillTerminalJobFinalMetrics(ctx)
	}()
}

// backfillTerminalJobFinalMetrics 对近期已终态但缺少终态指标块的作业再拉一次 job-info。
// 退避中的集群跳过；某个地址遇到传输层失败后，本次不再请求这个地址。
// backfillTerminalJobFinalMetrics re-fetches job-info for recent terminal jobs that still lack a final metrics block.
// Clusters in backoff are skipped, and an address that hits a transport failure is not called again this run.
func (s *Service) backfillTerminalJobFinalMetrics(ctx context.Context) {
	if s == nil || s.repo == nil || s.engineClient == nil {
		return
	}
	jobs, err := s.repo.ListRecentTerminalJobInstances(ctx, 100, time.Now().UTC().Add(-jobTerminalMetricsBackfillWindow))
	if err != nil || len(jobs) == 0 {
		return
	}
	unreachable := map[string]bool{}
	for _, job := range jobs {
		if job == nil || hasFinalMetricsLog(job.ResultPreview) {
			continue
		}
		if strings.TrimSpace(job.EngineJobID) == "" {
			continue
		}
		clusterID := uintValue(job.SubmitSpec, "cluster_id")
		if clusterID > 0 && s.reconcile.inBackoff(clusterID) {
			continue
		}
		endpoint := endpointFromSubmitSpec(job.SubmitSpec)
		if endpoint == nil && s.runtimeResolver != nil && clusterID > 0 {
			if resolved, resolveErr := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil); resolveErr == nil {
				endpoint = resolved
			}
		}
		if endpoint == nil || unreachable[endpoint.BaseURL] {
			continue
		}
		detail, detailErr := s.engineClient.GetJobInfo(ctx, endpoint, strings.TrimSpace(job.EngineJobID))
		if detailErr != nil || detail == nil {
			if isEngineTransportError(detailErr) {
				unreachable[endpoint.BaseURL] = true
			}
			continue
		}
		if _, applyErr := s.applyEngineJobInfoUpdate(ctx, job, detail); applyErr != nil {
			_ = applyErr
		}
	}
}

// engineJobInfoLooksOrphaned 识别 SeaTunnel 在作业不存在时返回的仅含 jobId 的空壳响应。
// engineJobInfoLooksOrphaned detects the empty {jobId} shell SeaTunnel returns for unknown jobs.
func engineJobInfoLooksOrphaned(info *EngineJobInfo) bool {
	if info == nil {
		return true
	}
	if strings.TrimSpace(info.JobStatus) != "" {
		return false
	}
	return strings.TrimSpace(info.CreateTime) == "" &&
		strings.TrimSpace(info.FinishedTime) == "" &&
		info.Metrics == nil &&
		info.JobDag == nil
}
