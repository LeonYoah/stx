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
	"time"

	"github.com/LeonYoah/stx/internal/config"
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

func (s *Service) reconcileActiveJobs(ctx context.Context) error {
	jobs, err := s.repo.ListNonTerminalJobInstances(ctx, 500)
	if err != nil || len(jobs) == 0 {
		return err
	}
	taskCluster := map[uint]uint{}
	grouped := map[uint][]*JobInstance{}
	for _, job := range jobs {
		if job == nil {
			continue
		}
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
			s.reconcileClusterJobs(ctx, 0, []*JobInstance{job})
			continue
		}
		grouped[clusterID] = append(grouped[clusterID], job)
	}
	channels := s.eventChannelSnapshot(ctx)
	for clusterID, items := range grouped {
		// 事件通道健康时只做轻量拉取，完整对账（running/finished 列表）降到每 60 秒一次兜底。
		// With a healthy event channel only light pulls run; full reconciles (running/finished lists) drop to a
		// 60-second safety net.
		if s.useEventModeReconcile(ctx, clusterID, channels) && !s.fullReconcileDue(clusterID) {
			s.reconcileClusterJobsLight(ctx, clusterID, items)
			continue
		}
		s.reconcileClusterJobs(ctx, clusterID, items)
		s.markFullReconcile(clusterID)
	}
	s.backfillTerminalJobFinalMetrics(ctx)
	return nil
}

const (
	// defaultEventModeFullReconcileInterval 是事件模式下完整对账的默认最长间隔。
	// defaultEventModeFullReconcileInterval is the default longest interval between full reconciles in event mode.
	defaultEventModeFullReconcileInterval = 60 * time.Second
	// eventModeUnwatchedRefreshInterval 是事件模式下无人查看的运行中作业拉 job-info 的间隔（与 metrics_log 对齐）。
	// eventModeUnwatchedRefreshInterval is how often unwatched running jobs pull job-info in event mode (aligned with metrics_log).
	eventModeUnwatchedRefreshInterval = 30 * time.Second
)

// eventModeFullReconcileInterval 读取配置；返回 0 表示关闭自适应。
// eventModeFullReconcileInterval reads the configuration; 0 means adaptive polling is disabled.
func eventModeFullReconcileInterval() time.Duration {
	if config.Config == nil {
		return defaultEventModeFullReconcileInterval
	}
	seconds := config.Config.Sync.JobReconcilerEventModeIntervalSeconds
	switch {
	case seconds < 0:
		return 0
	case seconds == 0:
		return defaultEventModeFullReconcileInterval
	default:
		return time.Duration(seconds) * time.Second
	}
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
func (s *Service) reconcileClusterJobsLight(ctx context.Context, clusterID uint, jobs []*JobInstance) {
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
		return
	}
	var endpoint *EngineEndpoint
	if s.runtimeResolver != nil {
		if resolved, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil); err == nil {
			endpoint = resolved
		}
	}
	for _, job := range due {
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
			continue
		}
		if _, err := s.applyEngineJobInfoUpdate(ctx, job, detail); err != nil {
			s.recordJobSyncFailure(ctx, job)
		}
	}
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

func (s *Service) reconcileClusterJobs(ctx context.Context, clusterID uint, jobs []*JobInstance) {
	if len(jobs) == 0 {
		return
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
		return
	}

	running, runErr := s.engineClient.ListRunningJobs(ctx, endpoint)
	if runErr != nil {
		for _, job := range jobs {
			s.recordJobSyncFailure(ctx, job)
		}
		return
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
			continue
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			finishedMap[strings.TrimSpace(item.JobID)] = item
		}
	}

	for _, job := range jobs {
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
			continue
		}
		if engineJobInfoLooksOrphaned(detail) {
			// Cluster is up but the job vanished (restart / history expiry): end the zombie running state.
			// 集群可达但作业已消失（重启/历史过期）：结束僵死的 running。
			if time.Since(job.CreatedAt) < 30*time.Second {
				continue
			}
			orphaned := &EngineJobInfo{
				JobID:     engineJobID,
				JobStatus: "FAILED",
				ErrorMsg:  "engine no longer has this job (cluster restart or history expired) / 引擎已无此作业（集群重启或历史过期）",
				FinishedTime: time.Now().In(time.Local).Format("2006-01-02 15:04:05"),
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
}

// backfillTerminalJobFinalMetrics 对近期已终态但缺少终态指标块的作业再拉一次 job-info。
// backfillTerminalJobFinalMetrics re-fetches job-info for recent terminal jobs that still lack a final metrics block.
func (s *Service) backfillTerminalJobFinalMetrics(ctx context.Context) {
	if s == nil || s.repo == nil || s.engineClient == nil {
		return
	}
	jobs, err := s.repo.ListRecentTerminalJobInstances(ctx, 100, time.Now().UTC().Add(-jobTerminalMetricsBackfillWindow))
	if err != nil || len(jobs) == 0 {
		return
	}
	for _, job := range jobs {
		if job == nil || hasFinalMetricsLog(job.ResultPreview) {
			continue
		}
		if strings.TrimSpace(job.EngineJobID) == "" {
			continue
		}
		endpoint := endpointFromSubmitSpec(job.SubmitSpec)
		if endpoint == nil && s.runtimeResolver != nil {
			clusterID := uintValue(job.SubmitSpec, "cluster_id")
			if clusterID > 0 {
				if resolved, resolveErr := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil); resolveErr == nil {
					endpoint = resolved
				}
			}
		}
		if endpoint == nil {
			continue
		}
		detail, detailErr := s.engineClient.GetJobInfo(ctx, endpoint, strings.TrimSpace(job.EngineJobID))
		if detailErr != nil || detail == nil {
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
