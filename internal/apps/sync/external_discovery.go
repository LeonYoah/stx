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
	"strings"
	stdsync "sync"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
)

const (
	defaultExternalDiscoveryInterval   = 60 * time.Second
	defaultExternalDiscoveryMaxPerPass = 200
)

// ClusterDiscoveryProvider 返回需要做外部作业发现的集群（未在集群配置里关闭）。
// ClusterDiscoveryProvider returns clusters that should be scanned for external jobs (not opted out in config).
type ClusterDiscoveryProvider interface {
	ListExternalDiscoveryClusterIDs(ctx context.Context) ([]uint, error)
}

// SetClusterDiscoveryProvider 注入外部作业发现的集群来源；未注入时不做外部发现。
// SetClusterDiscoveryProvider injects the cluster source for external discovery; without it nothing is discovered.
func (s *Service) SetClusterDiscoveryProvider(provider ClusterDiscoveryProvider) {
	s.clusterDiscoveryProvider = provider
}

// externalDiscoveryState 保存外部发现的内存状态：各集群上次发现时间、待确认的未知作业、进行中的 STX 提交。
// externalDiscoveryState holds in-memory discovery state: each cluster's last scan, unknown jobs awaiting a second
// sighting, and STX submits still in flight.
type externalDiscoveryState struct {
	mu        stdsync.Mutex
	lastRun   map[uint]time.Time
	wake      map[uint]bool
	firstSeen map[uint]map[string]time.Time
	inflight  map[string]int
}

func externalDiscoveryGloballyEnabled() bool {
	if config.Config == nil || config.Config.Sync.ExternalJobDiscoveryEnabled == nil {
		return true
	}
	return *config.Config.Sync.ExternalJobDiscoveryEnabled
}

func externalDiscoveryInterval() time.Duration {
	if config.Config == nil || config.Config.Sync.ExternalJobDiscoveryIntervalSeconds <= 0 {
		return defaultExternalDiscoveryInterval
	}
	return time.Duration(config.Config.Sync.ExternalJobDiscoveryIntervalSeconds) * time.Second
}

func externalDiscoveryMaxPerPass() int {
	if config.Config == nil || config.Config.Sync.ExternalJobDiscoveryMaxNewPerRound <= 0 {
		return defaultExternalDiscoveryMaxPerPass
	}
	return config.Config.Sync.ExternalJobDiscoveryMaxNewPerRound
}

// externalDiscoveryClusters 返回本轮开启外部发现的集群集合；全局关闭或未注入来源时为空。
// externalDiscoveryClusters returns the clusters with discovery on for this pass; empty when globally off or unwired.
func (s *Service) externalDiscoveryClusters(ctx context.Context) map[uint]bool {
	out := map[uint]bool{}
	if s == nil || s.clusterDiscoveryProvider == nil || !externalDiscoveryGloballyEnabled() {
		return out
	}
	ids, err := s.clusterDiscoveryProvider.ListExternalDiscoveryClusterIDs(ctx)
	if err != nil {
		logger.WarnF(ctx, "[Sync] list external discovery clusters failed: %v", err)
		return out
	}
	for _, id := range ids {
		if id > 0 {
			out[id] = true
		}
	}
	return out
}

// externalDiscoveryDue 判断只做发现的集群本轮是否该巡检：到了间隔、被事件唤醒，或有等待二次确认的未知作业。
// externalDiscoveryDue reports whether a discovery-only cluster should be scanned now: the interval elapsed, an
// event woke it, or unknown jobs await their second sighting.
func (s *Service) externalDiscoveryDue(clusterID uint, now time.Time) bool {
	state := &s.external
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.wake[clusterID] || len(state.firstSeen[clusterID]) > 0 {
		return true
	}
	last, ok := state.lastRun[clusterID]
	return !ok || now.Sub(last) >= externalDiscoveryInterval()
}

func (s *Service) markExternalDiscoveryRun(clusterID uint, now time.Time) {
	state := &s.external
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.lastRun == nil {
		state.lastRun = map[uint]time.Time{}
	}
	state.lastRun[clusterID] = now
	delete(state.wake, clusterID)
}

// requestExternalDiscovery 由未知作业的状态事件触发，让该集群下一轮立即巡检；发现本身仍走列表对账。
// requestExternalDiscovery is triggered by a status event for an unknown job so the cluster is scanned on the next
// tick; discovery itself still goes through the list reconcile.
func (s *Service) requestExternalDiscovery(clusterID uint) {
	if s == nil || clusterID == 0 || s.clusterDiscoveryProvider == nil || !externalDiscoveryGloballyEnabled() {
		return
	}
	state := &s.external
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.wake == nil {
		state.wake = map[uint]bool{}
	}
	state.wake[clusterID] = true
}

// trackInflightSubmit 在调用引擎提交前登记作业 ID，返回的函数在落库或失败后注销。
// 引擎会在提交接口返回前就把作业放进 running 列表，而 STX 在返回后才落库，期间发现不能把它当外部作业。
// trackInflightSubmit registers the job ID before the engine submit; the returned func unregisters it after the row
// is stored or the submit fails. The engine lists the job as running before the submit call returns while STX
// stores it afterwards, so discovery must not treat it as external meanwhile.
func (s *Service) trackInflightSubmit(jobID string) func() {
	jobID = strings.TrimSpace(jobID)
	if s == nil || jobID == "" {
		return func() {}
	}
	state := &s.external
	state.mu.Lock()
	if state.inflight == nil {
		state.inflight = map[string]int{}
	}
	state.inflight[jobID]++
	state.mu.Unlock()
	return func() {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.inflight[jobID] <= 1 {
			delete(state.inflight, jobID)
			return
		}
		state.inflight[jobID]--
	}
}

// discoverExternalJobs 把列表里库中没有的作业导入为外部作业，不发额外的引擎请求。
// 未知作业要连续两轮出现，或创建时间早于提交超时，才导入，以挡住多实例与重启时的 STX 提交竞态；
// 每轮最多导入 externalDiscoveryMaxPerPass 行，运行中优先，其余留到下一轮。
// discoverExternalJobs imports listed jobs missing from the database as external jobs without extra engine calls.
// An unknown job is imported only after two consecutive sightings or when it was created longer ago than the submit
// timeout, which covers STX submit races across instances and restarts. At most externalDiscoveryMaxPerPass rows are
// created per pass, running ones first; the rest wait for the next pass.
func (s *Service) discoverExternalJobs(ctx context.Context, clusterID uint, endpoint *EngineEndpoint, running, finished []*EngineJobListItem) error {
	if clusterID == 0 || s.repo == nil {
		return nil
	}
	type candidate struct {
		item    *EngineJobListItem
		running bool
	}
	var (
		candidates []candidate
		ids        []string
	)
	seen := map[string]struct{}{}
	collect := func(items []*EngineJobListItem, isRunning bool) {
		for _, item := range items {
			if item == nil {
				continue
			}
			id := strings.TrimSpace(item.JobID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			candidates = append(candidates, candidate{item: item, running: isRunning})
			ids = append(ids, id)
		}
	}
	collect(running, true)
	collect(finished, false)
	if len(ids) == 0 {
		s.resetExternalFirstSeen(clusterID, nil)
		return nil
	}
	existing, err := s.repo.ExistingEngineJobIDs(ctx, clusterID, ids)
	if err != nil {
		return err
	}
	now := time.Now()
	timeout := globalSubmitTimeout()
	state := &s.external
	state.mu.Lock()
	previous := state.firstSeen[clusterID]
	nextSeen := map[string]time.Time{}
	var eligible []candidate
	for _, c := range candidates {
		id := strings.TrimSpace(c.item.JobID)
		if _, ok := existing[id]; ok {
			continue
		}
		if state.inflight[id] > 0 {
			continue
		}
		first, sawBefore := previous[id]
		if !sawBefore {
			first = now
		}
		nextSeen[id] = first
		old := false
		if created := parseEngineJobTime(ctx, c.item.CreateTime, clusterID); created != nil && now.Sub(*created) >= timeout {
			old = true
		}
		if sawBefore || old {
			eligible = append(eligible, c)
		}
	}
	if state.firstSeen == nil {
		state.firstSeen = map[uint]map[string]time.Time{}
	}
	state.firstSeen[clusterID] = nextSeen
	state.mu.Unlock()

	limit := externalDiscoveryMaxPerPass()
	imported := 0
	for _, c := range eligible {
		if imported >= limit {
			break
		}
		if _, err := s.importExternalJob(ctx, clusterID, endpoint, c.item); err != nil {
			logger.WarnF(ctx, "[Sync] import external job failed: cluster=%d engine_job_id=%s err=%v", clusterID, c.item.JobID, err)
			continue
		}
		imported++
		state.mu.Lock()
		delete(state.firstSeen[clusterID], strings.TrimSpace(c.item.JobID))
		state.mu.Unlock()
	}
	if imported > 0 {
		logger.InfoF(ctx, "[Sync] imported external jobs: cluster=%d count=%d", clusterID, imported)
	}
	return nil
}

func (s *Service) resetExternalFirstSeen(clusterID uint, seen map[string]time.Time) {
	state := &s.external
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.firstSeen == nil {
		state.firstSeen = map[uint]map[string]time.Time{}
	}
	state.firstSeen[clusterID] = seen
}

// importExternalJob 按外部作业契约建行，再用列表项走一次常规状态更新，补齐状态、指标、错误与结束时间。
// importExternalJob creates the row per the external job contract, then runs the usual status update with the list
// item to fill status, metrics, error and finish time.
func (s *Service) importExternalJob(ctx context.Context, clusterID uint, endpoint *EngineEndpoint, item *EngineJobListItem) (*JobInstance, error) {
	engineJobID := strings.TrimSpace(item.JobID)
	mode := externalJobMode(item.JobDag)
	spec := JSONMap{
		"source":         string(JobSourceExternal),
		"trigger_source": string(JobSourceExternal),
		"cluster_id":     clusterID,
		"job_name":       strings.TrimSpace(item.JobName),
		"mode":           mode,
	}
	if endpoint != nil {
		spec["engine_api_mode"] = defaultString(endpoint.APIMode, "v2")
		if endpoint.BaseURL != "" {
			spec["engine_base_url"] = endpoint.BaseURL
		}
		if endpoint.LegacyURL != "" {
			spec["engine_legacy_base_url"] = endpoint.LegacyURL
		}
		if endpoint.ContextPath != "" {
			spec["engine_context_path"] = endpoint.ContextPath
		}
	}
	startedAt := parseEngineJobTime(ctx, item.CreateTime, clusterID)
	if startedAt == nil {
		now := time.Now().UTC()
		startedAt = &now
	}
	instance := &JobInstance{
		Source:      JobSourceExternal,
		RunType:     RunTypeExternal,
		EngineJobID: engineJobID,
		ClusterID:   clusterID,
		JobMode:     mode,
		Status:      JobStatusPending,
		SubmitSpec:  spec,
		StartedAt:   startedAt,
	}
	if err := s.repo.CreateJobInstance(ctx, instance); err != nil {
		return nil, err
	}
	if err := s.repo.BackfillEngineEventJobInstanceID(ctx, clusterID, engineJobID, instance.ID); err != nil {
		logger.WarnF(ctx, "[Sync] backfill external job events failed: cluster=%d job=%d err=%v", clusterID, instance.ID, err)
	}
	return s.applyEngineJobInfoUpdate(ctx, instance, engineListItemToInfo(item))
}

// externalJobMode 从引擎 jobDag.envOptions 读作业模式；有 jobDag 但没写 job.mode 时按引擎默认 batch，没有 jobDag 返回空。
// externalJobMode reads the mode from jobDag.envOptions; with a jobDag but no job.mode the engine default batch
// applies, and without a jobDag it is empty.
func externalJobMode(jobDag map[string]interface{}) string {
	if len(jobDag) == 0 {
		return ""
	}
	if env, ok := jobDag["envOptions"].(map[string]interface{}); ok {
		if raw, ok := env["job.mode"]; ok {
			if mode := normalizeJobMode(fmt.Sprint(raw)); mode != "" {
				return mode
			}
		}
	}
	return string(TaskModeBatch)
}

// mergeEndpointProtocolHints 保留动态解析得到的最新集群地址，仅继承快照里的协议形态（APIMode / ContextPath），
// 防止宿主机换 IP 或主备切换后仍打向旧地址，同时不丢 v1 兼容模式。
// mergeEndpointProtocolHints keeps the live cluster host/port from dynamic resolution and only inherits protocol
// hints (APIMode / ContextPath) from the submit snapshot, surviving IP changes and failover without losing v1 mode.
func mergeEndpointProtocolHints(resolved, snapshot *EngineEndpoint) *EngineEndpoint {
	if resolved == nil {
		return snapshot
	}
	merged := *resolved
	if snapshot != nil {
		if strings.TrimSpace(merged.APIMode) == "" && strings.TrimSpace(snapshot.APIMode) != "" {
			merged.APIMode = strings.TrimSpace(snapshot.APIMode)
		}
		if strings.TrimSpace(merged.ContextPath) == "" && strings.TrimSpace(snapshot.ContextPath) != "" {
			merged.ContextPath = strings.TrimSpace(snapshot.ContextPath)
		}
		if strings.EqualFold(strings.TrimSpace(merged.APIMode), "v1") && strings.TrimSpace(merged.LegacyURL) == "" {
			merged.LegacyURL = strings.TrimSpace(merged.BaseURL)
		}
	}
	return &merged
}

// jobEndpointResolveCache 在单轮对账 / 补拉循环内缓存任务定义与集群端点，避免批量作业放大 DB 与集群解析查询。
// jobEndpointResolveCache caches task definitions and cluster endpoints within a single reconcile/backfill pass
// so multi-job ticks do not amplify DB and cluster resolution queries.
type jobEndpointResolveCache struct {
	tasks       map[uint]*Task
	clusters    map[uint]*EngineEndpoint
	taskCluster map[uint]uint
}

func newJobEndpointResolveCache() *jobEndpointResolveCache {
	return &jobEndpointResolveCache{
		tasks:       map[uint]*Task{},
		clusters:    map[uint]*EngineEndpoint{},
		taskCluster: map[uint]uint{},
	}
}

// taskDefinitionForJob 读取作业关联任务的 Definition，供端点解析识别任务级显式 engine_base_url 覆盖；
// 传入 cache 时同一轮内每个 taskID 最多查库一次（含未命中缓存）。
// taskDefinitionForJob loads the linked task Definition so endpoint resolution honors task-pinned engine_base_url;
// with a cache each taskID is queried at most once per pass (including misses).
func (s *Service) taskDefinitionForJob(ctx context.Context, job *JobInstance, cache *jobEndpointResolveCache) JSONMap {
	if s == nil || s.repo == nil || job == nil || job.TaskID == 0 {
		return nil
	}
	if cache != nil {
		if task, ok := cache.tasks[job.TaskID]; ok {
			if task == nil {
				return nil
			}
			return task.Definition
		}
	}
	task, err := s.repo.GetTaskByID(ctx, job.TaskID)
	if err != nil {
		task = nil
	}
	if cache != nil {
		cache.tasks[job.TaskID] = task
		if task != nil && task.ClusterID > 0 {
			cache.taskCluster[job.TaskID] = task.ClusterID
		}
	}
	if task == nil {
		return nil
	}
	return task.Definition
}

// resolveJobEngineEndpoint 优先按作业所属集群动态解析当前在线节点地址（兼容宿主机改 IP、主备切换与外部作业），
// 仅在无集群信息或动态解析不可用时回退到提交快照里的引擎地址。
// resolveJobEngineEndpoint resolves the live cluster endpoint first (surviving host IP changes, master failover,
// and external jobs) and falls back to the submit snapshot only when no cluster is linked or resolution fails.
func (s *Service) resolveJobEngineEndpoint(ctx context.Context, job *JobInstance) *EngineEndpoint {
	return s.resolveJobEngineEndpointCached(ctx, job, nil)
}

// resolveJobEngineEndpointCached 是 resolveJobEngineEndpoint 的带单轮缓存版本。
// 任务定义显式指定了 engine_base_url 时按任务定义解析；否则按 clusterID 复用单轮内的集群解析结果并合并快照协议形态。
// resolveJobEngineEndpointCached is the per-pass cached variant of resolveJobEngineEndpoint.
// A task-pinned engine_base_url is resolved from taskDefinition; otherwise the cluster endpoint is cached per
// clusterID and merged with snapshot protocol hints.
func (s *Service) resolveJobEngineEndpointCached(ctx context.Context, job *JobInstance, cache *jobEndpointResolveCache) *EngineEndpoint {
	if job == nil {
		return nil
	}
	snapshot := endpointFromSubmitSpec(job.SubmitSpec)
	if s.runtimeResolver != nil {
		var taskCluster map[uint]uint
		if cache != nil {
			taskCluster = cache.taskCluster
		}
		clusterID := clusterIDForJob(ctx, s, job, taskCluster)
		if clusterID > 0 {
			taskDef := s.taskDefinitionForJob(ctx, job, cache)
			if endpointFromTaskDefinition(taskDef) != nil {
				if endpoint, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, taskDef); err == nil && endpoint != nil {
					return mergeEndpointProtocolHints(endpoint, snapshot)
				}
			} else if cache != nil {
				resolved, ok := cache.clusters[clusterID]
				if !ok {
					if endpoint, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil); err == nil {
						resolved = endpoint
					}
					cache.clusters[clusterID] = resolved
				}
				if resolved != nil {
					return mergeEndpointProtocolHints(resolved, snapshot)
				}
			} else if endpoint, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil); err == nil && endpoint != nil {
				return mergeEndpointProtocolHints(endpoint, snapshot)
			}
		}
	}
	return snapshot
}
