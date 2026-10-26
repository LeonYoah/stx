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
	"fmt"
	"sort"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

const (
	// 窗口默认 3 分钟，上限 10 分钟，与引擎 /metrics/realtime 一致。
	// Default window 3 minutes, capped at 10 minutes, matching the engine /metrics/realtime API.
	realtimeDefaultWindowMs int64 = 180000
	realtimeMaxWindowMs     int64 = 600000

	// realtimeMasterCacheTTL 是 active master 探测结果的缓存时间。
	// realtimeMasterCacheTTL is how long an active-master probe result is cached.
	realtimeMasterCacheTTL = 60 * time.Second
	// realtimeResultCacheTTL 合并多人同时查看同一作业的请求；引擎每 5 秒才出一个桶。
	// realtimeResultCacheTTL coalesces concurrent viewers of one job; the engine only produces a bucket every 5s.
	realtimeResultCacheTTL = 2 * time.Second
	// jobWatchWindow 内被读取过的作业视为"有人在看"，供自适应轮询使用。
	// Jobs read within jobWatchWindow count as watched for adaptive polling.
	jobWatchWindow = 30 * time.Second

	// realtimeHintBuckets 是瓶颈提示需要连续满足的桶数。
	// realtimeHintBuckets is the number of consecutive buckets a hint condition must hold for.
	realtimeHintBuckets = 3
)

// 实时数据不可用的稳定原因码，前端负责本地化。
// Stable reason codes for unavailable realtime data; the frontend localizes them.
const (
	RealtimeReasonVersionUnsupported = "version_unsupported"
	RealtimeReasonNotClusterMode     = "not_cluster_mode"
	RealtimeReasonNotRunning         = "not_running"
	RealtimeReasonNotEnabled         = "not_enabled"
	RealtimeReasonMasterUnreachable  = "master_unreachable"
	RealtimeReasonNoEngineJob        = "no_engine_job"
	RealtimeReasonCollecting         = "collecting"
	RealtimeReasonClientUnsupported  = "client_unsupported"
)

// 瓶颈提示码与阈值，规则来自上游 busyness-and-backpressure 文档的判定表。
// Bottleneck hint codes and thresholds derived from the upstream busyness-and-backpressure decision table.
const (
	RealtimeHintSinkSlow               = "sink_slow"
	RealtimeHintTransformBusy          = "transform_busy"
	RealtimeHintDownstreamBackpressure = "downstream_backpressure"
	RealtimeHintSourceIdle             = "source_idle"
	RealtimeHintSourceBusy             = "source_busy"

	realtimeBusyThreshold        = 0.8
	realtimeBackpressureWarning  = 0.2
	realtimeBackpressureCritical = 0.5
	realtimeQueueFullThreshold   = 0.8
	realtimeQueueEmptyThreshold  = 0.1
	realtimeIdleThreshold        = 0.8
	realtimeNoBackpressure       = 0.05
)

// JobRealtimeResult 是 GET /api/v1/sync/jobs/:id/realtime 的返回。引擎侧失败都用 available=false 加 reason 表达。
// JobRealtimeResult is the payload of GET /api/v1/sync/jobs/:id/realtime. Engine-side failures are expressed as
// available=false plus a reason.
type JobRealtimeResult struct {
	Available     bool             `json:"available"`
	Reason        string           `json:"reason"`
	JobID         uint             `json:"job_id"`
	EngineJobID   string           `json:"engine_job_id"`
	ClusterID     uint             `json:"cluster_id"`
	BucketMs      int64            `json:"bucket_ms"`
	WindowMs      int64            `json:"window_ms"`
	FromMs        int64            `json:"from_ms"`
	ToMs          int64            `json:"to_ms"`
	FetchedAtMs   int64            `json:"fetched_at_ms"`
	WebUIPath     string           `json:"webui_path"`
	Observability JSONMap          `json:"observability"`
	Vertices      []RealtimeVertex `json:"vertices"`
	Edges         []RealtimeEdge   `json:"edges"`
	Hints         []RealtimeHint   `json:"hints"`
}

// RealtimeVertex 是一个 DAG 节点的时间序列，type/name 来自作业 DAG。
// RealtimeVertex is one DAG vertex time series; type/name come from the job DAG.
type RealtimeVertex struct {
	VertexID int64            `json:"vertex_id"`
	Type     string           `json:"type"`
	Name     string           `json:"name"`
	Points   []map[string]any `json:"points"`
}

// RealtimeEdge 是一条带中间队列的边的时间序列。
// RealtimeEdge is one queue-backed edge time series.
type RealtimeEdge struct {
	QueueID        int64            `json:"queue_id"`
	TargetVertexID int64            `json:"target_vertex_id"`
	Points         []map[string]any `json:"points"`
}

// RealtimeHint 是一条瓶颈提示；文案由前端按 code 本地化。
// RealtimeHint is one bottleneck hint; the frontend localizes it by code.
type RealtimeHint struct {
	Code     string  `json:"code"`
	Severity string  `json:"severity"`
	VertexID *int64  `json:"vertex_id,omitempty"`
	QueueID  *int64  `json:"queue_id,omitempty"`
	Buckets  int     `json:"buckets"`
	Value    float64 `json:"value"`
}

// realtimeState 保存实时代理的进程内状态，零值可用。
// realtimeState holds in-process state for the realtime proxy; its zero value is ready to use.
type realtimeState struct {
	mu       gosync.Mutex
	masters  map[string]realtimeMasterEntry
	results  map[string]realtimeResultEntry
	inflight map[string]*realtimeCall
	watched  gosync.Map // jobID(uint) -> time.Time
	now      func() time.Time
}

type realtimeMasterEntry struct {
	endpoint  *EngineEndpoint
	expiresAt time.Time
}

type realtimeResultEntry struct {
	result    *JobRealtimeResult
	expiresAt time.Time
}

type realtimeCall struct {
	done   chan struct{}
	result *JobRealtimeResult
	err    error
}

func (r *realtimeState) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// markJobWatched 记录作业刚被读取过。
// markJobWatched records that the job was just read.
func (s *Service) markJobWatched(jobID uint) {
	if s == nil || jobID == 0 {
		return
	}
	s.realtime.watched.Store(jobID, s.realtime.clock())
}

// jobWatchedRecently 判断作业在 jobWatchWindow 内是否被读取过。
// jobWatchedRecently reports whether the job was read within jobWatchWindow.
func (s *Service) jobWatchedRecently(jobID uint) bool {
	if s == nil || jobID == 0 {
		return false
	}
	value, ok := s.realtime.watched.Load(jobID)
	if !ok {
		return false
	}
	at, _ := value.(time.Time)
	if s.realtime.clock().Sub(at) > jobWatchWindow {
		s.realtime.watched.Delete(jobID)
		return false
	}
	return true
}

// normalizeRealtimeWindow 把窗口限制在 (0, 10 分钟]，0 或负数取默认值。
// normalizeRealtimeWindow clamps the window to (0, 10 minutes]; zero or negative uses the default.
func normalizeRealtimeWindow(windowMs int64) int64 {
	if windowMs <= 0 {
		return realtimeDefaultWindowMs
	}
	if windowMs > realtimeMaxWindowMs {
		return realtimeMaxWindowMs
	}
	return windowMs
}

// GetJobRealtimeForActor 在权限检查后返回作业的实时指标与瓶颈提示。
// GetJobRealtimeForActor returns realtime metrics and bottleneck hints for a job after enforcing permission.
func (s *Service) GetJobRealtimeForActor(ctx context.Context, actor executionapp.Actor, id uint, windowMs int64) (*JobRealtimeResult, error) {
	job, err := s.getJobInstanceForActor(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	s.markJobWatched(job.ID)
	return s.getJobRealtime(ctx, job, normalizeRealtimeWindow(windowMs))
}

// isRealtimeObservableStatus 判断作业是否处于引擎仍在运行的状态。
// isRealtimeObservableStatus reports whether the job is in a state where the engine still runs it.
func isRealtimeObservableStatus(status JobStatus) bool {
	switch status {
	case JobStatusRunning, JobStatusCancelRequested, JobStatusCancelling, JobStatusDoingSavepoint:
		return true
	default:
		return false
	}
}

func (s *Service) getJobRealtime(ctx context.Context, job *JobInstance, windowMs int64) (*JobRealtimeResult, error) {
	result := &JobRealtimeResult{
		JobID:         job.ID,
		EngineJobID:   strings.TrimSpace(job.EngineJobID),
		WindowMs:      windowMs,
		Observability: JSONMap(mapValue(job.SubmitSpec, "observability")),
		Vertices:      []RealtimeVertex{},
		Edges:         []RealtimeEdge{},
		Hints:         []RealtimeHint{},
	}
	if result.Observability == nil {
		result.Observability = JSONMap{}
	}
	if submitSpecExecutionMode(job.SubmitSpec) == "local" {
		result.Reason = RealtimeReasonNotClusterMode
		return result, nil
	}
	if result.EngineJobID == "" {
		result.Reason = RealtimeReasonNoEngineJob
		return result, nil
	}
	clusterID := clusterIDForJob(ctx, s, job, map[uint]uint{})
	result.ClusterID = clusterID
	if clusterID > 0 {
		result.WebUIPath = fmt.Sprintf("/api/v1/clusters/%d/webui/#/jobs/%s", clusterID, result.EngineJobID)
	}
	version := strings.TrimSpace(stringValue(job.SubmitSpec, "engine_version"))
	if s.clusterVersionProvider != nil && clusterID > 0 {
		if resolved, err := s.clusterVersionProvider.GetClusterVersion(ctx, clusterID); err == nil && strings.TrimSpace(resolved) != "" {
			version = strings.TrimSpace(resolved)
		}
	}
	if !seatunnel.SupportsRealtimeObservability(version) {
		result.Reason = RealtimeReasonVersionUnsupported
		return result, nil
	}
	if !isRealtimeObservableStatus(job.Status) {
		result.Reason = RealtimeReasonNotRunning
		return result, nil
	}
	client, ok := s.engineClient.(EngineRealtimeClient)
	if !ok || client == nil {
		result.Reason = RealtimeReasonClientUnsupported
		return result, nil
	}
	return s.fetchRealtimeCoalesced(ctx, client, job, clusterID, result)
}

// fetchRealtimeCoalesced 用短时缓存加并发合并，保证同一作业同一窗口同一时刻只请求引擎一次。
// fetchRealtimeCoalesced uses a short cache plus in-flight coalescing so one job/window hits the engine once at a time.
func (s *Service) fetchRealtimeCoalesced(ctx context.Context, client EngineRealtimeClient, job *JobInstance, clusterID uint, base *JobRealtimeResult) (*JobRealtimeResult, error) {
	key := strconv.FormatUint(uint64(job.ID), 10) + ":" + strconv.FormatInt(base.WindowMs, 10)
	state := &s.realtime
	state.mu.Lock()
	if entry, ok := state.results[key]; ok && state.clock().Before(entry.expiresAt) {
		state.mu.Unlock()
		return entry.result, nil
	}
	if call, ok := state.inflight[key]; ok {
		state.mu.Unlock()
		select {
		case <-call.done:
			return call.result, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if state.inflight == nil {
		state.inflight = map[string]*realtimeCall{}
	}
	call := &realtimeCall{done: make(chan struct{})}
	state.inflight[key] = call
	state.mu.Unlock()

	call.result, call.err = s.fetchRealtime(ctx, client, job, clusterID, base)

	state.mu.Lock()
	delete(state.inflight, key)
	if call.err == nil && call.result != nil {
		if state.results == nil {
			state.results = map[string]realtimeResultEntry{}
		}
		now := state.clock()
		for cachedKey, entry := range state.results {
			if !now.Before(entry.expiresAt) {
				delete(state.results, cachedKey)
			}
		}
		state.results[key] = realtimeResultEntry{result: call.result, expiresAt: now.Add(realtimeResultCacheTTL)}
	}
	state.mu.Unlock()
	close(call.done)
	return call.result, call.err
}

// fetchRealtime 找到 active master 后并行拉取节点与边，非 master 时作废缓存重试一次。
// fetchRealtime locates the active master and fetches vertices and edges in parallel, retrying once after a
// stale master cache entry.
func (s *Service) fetchRealtime(ctx context.Context, client EngineRealtimeClient, job *JobInstance, clusterID uint, base *JobRealtimeResult) (*JobRealtimeResult, error) {
	result := *base
	candidates := s.realtimeCandidates(ctx, job, clusterID)
	cacheKey := realtimeMasterCacheKey(clusterID, candidates)
	for attempt := 0; attempt < 2; attempt++ {
		master := s.locateRealtimeMaster(ctx, client, cacheKey, candidates)
		if master == nil {
			result.Reason = RealtimeReasonMasterUnreachable
			return &result, nil
		}
		vertices, edges, err := fetchRealtimeSeries(ctx, client, master, result.EngineJobID, result.WindowMs)
		if err != nil {
			s.forgetRealtimeMaster(cacheKey)
			if errors.Is(err, ErrEngineNotMaster) && attempt == 0 {
				continue
			}
			result.Reason = RealtimeReasonMasterUnreachable
			return &result, nil
		}
		result.FetchedAtMs = s.realtime.clock().UnixMilli()
		return interpretRealtimeSeries(&result, job, vertices, edges), nil
	}
	result.Reason = RealtimeReasonMasterUnreachable
	return &result, nil
}

func fetchRealtimeSeries(ctx context.Context, client EngineRealtimeClient, endpoint *EngineEndpoint, engineJobID string, windowMs int64) (*EngineRealtimeSeries, *EngineRealtimeSeries, error) {
	var (
		wg                 gosync.WaitGroup
		vertices, edges    *EngineRealtimeSeries
		vertexErr, edgeErr error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		vertices, vertexErr = client.GetRealtimeVertices(ctx, endpoint, engineJobID, windowMs)
	}()
	go func() {
		defer wg.Done()
		edges, edgeErr = client.GetRealtimeEdges(ctx, endpoint, engineJobID, windowMs)
	}()
	wg.Wait()
	if vertexErr != nil {
		return nil, nil, vertexErr
	}
	if edgeErr != nil {
		return nil, nil, edgeErr
	}
	return vertices, edges, nil
}

// interpretRealtimeSeries 把引擎返回整理成 STX 结构，并判断 not_enabled / collecting。
// interpretRealtimeSeries converts engine responses into the STX structure and decides not_enabled / collecting.
func interpretRealtimeSeries(result *JobRealtimeResult, job *JobInstance, vertices, edges *EngineRealtimeSeries) *JobRealtimeResult {
	if !vertices.Known && !edges.Known {
		// 引擎 master 还没把该作业纳入采集（刚提交或已结束）。
		// The engine master has not picked the job up yet (just submitted) or already dropped it.
		result.Reason = RealtimeReasonCollecting
		return result
	}
	if (vertices.Enabled != nil && !*vertices.Enabled) || (edges.Enabled != nil && !*edges.Enabled) {
		result.Reason = RealtimeReasonNotEnabled
		return result
	}
	result.BucketMs = vertices.BucketMs
	if result.BucketMs == 0 {
		result.BucketMs = edges.BucketMs
	}
	result.FromMs = vertices.FromMs
	result.ToMs = vertices.ToMs
	types, names := realtimeVertexMeta(job)
	for _, vertex := range vertices.Vertices {
		points := sortRealtimePoints(vertex.Points)
		vertexType := types[vertex.VertexID]
		if vertexType == "" {
			vertexType = inferRealtimeVertexType(points)
		}
		result.Vertices = append(result.Vertices, RealtimeVertex{VertexID: vertex.VertexID, Type: vertexType, Name: names[vertex.VertexID], Points: points})
	}
	for _, edge := range edges.Edges {
		result.Edges = append(result.Edges, RealtimeEdge{QueueID: edge.QueueID, TargetVertexID: edge.TargetVertexID, Points: sortRealtimePoints(edge.Points)})
	}
	if !hasRealtimePoints(result) {
		result.Reason = RealtimeReasonCollecting
		return result
	}
	result.Available = true
	result.Hints = computeRealtimeHints(result.Vertices, result.Edges, realtimeDownstream(job))
	return result
}

func hasRealtimePoints(result *JobRealtimeResult) bool {
	for _, vertex := range result.Vertices {
		if len(vertex.Points) > 0 {
			return true
		}
	}
	for _, edge := range result.Edges {
		if len(edge.Points) > 0 {
			return true
		}
	}
	return false
}

// realtimeCandidates 返回可探测的节点：优先集群全部在线节点，否则回落到提交时记录的引擎地址。
// realtimeCandidates returns probe candidates: all online cluster nodes when available, otherwise the endpoint
// recorded at submit time.
func (s *Service) realtimeCandidates(ctx context.Context, job *JobInstance, clusterID uint) []*EngineEndpoint {
	if lister, ok := s.runtimeResolver.(ClusterEngineEndpointLister); ok && lister != nil && clusterID > 0 {
		if endpoints, err := lister.ListEngineEndpoints(ctx, clusterID, nil); err == nil && len(endpoints) > 0 {
			return endpoints
		}
	}
	if endpoint := endpointFromSubmitSpec(job.SubmitSpec); endpoint != nil {
		return []*EngineEndpoint{endpoint}
	}
	if s.runtimeResolver != nil && clusterID > 0 {
		if endpoint, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil); err == nil && endpoint != nil {
			return []*EngineEndpoint{endpoint}
		}
	}
	return nil
}

func realtimeMasterCacheKey(clusterID uint, candidates []*EngineEndpoint) string {
	if clusterID > 0 {
		return "cluster:" + strconv.FormatUint(uint64(clusterID), 10)
	}
	if len(candidates) > 0 && candidates[0] != nil {
		return "endpoint:" + strings.TrimSpace(candidates[0].BaseURL)
	}
	return ""
}

// locateRealtimeMaster 先用缓存，再依次探测候选节点的 /metrics/realtime/jobs，第一个 200 即 active master。
// locateRealtimeMaster uses the cache first, then probes /metrics/realtime/jobs on each candidate; the first 200
// is the active master.
func (s *Service) locateRealtimeMaster(ctx context.Context, client EngineRealtimeClient, cacheKey string, candidates []*EngineEndpoint) *EngineEndpoint {
	if cacheKey == "" || len(candidates) == 0 {
		return nil
	}
	state := &s.realtime
	state.mu.Lock()
	if entry, ok := state.masters[cacheKey]; ok && state.clock().Before(entry.expiresAt) {
		state.mu.Unlock()
		return entry.endpoint
	}
	state.mu.Unlock()
	for _, candidate := range candidates {
		if candidate == nil || strings.TrimSpace(candidate.BaseURL) == "" {
			continue
		}
		if _, err := client.GetRealtimeJobs(ctx, candidate); err != nil {
			continue
		}
		state.mu.Lock()
		if state.masters == nil {
			state.masters = map[string]realtimeMasterEntry{}
		}
		state.masters[cacheKey] = realtimeMasterEntry{endpoint: candidate, expiresAt: state.clock().Add(realtimeMasterCacheTTL)}
		state.mu.Unlock()
		return candidate
	}
	return nil
}

func (s *Service) forgetRealtimeMaster(cacheKey string) {
	state := &s.realtime
	state.mu.Lock()
	delete(state.masters, cacheKey)
	state.mu.Unlock()
}

// realtimeVertexMeta 从作业 DAG（job-info 的 jobDag.vertexInfoMap）读取节点类型与名称，兼容对象和数组两种形态。
// realtimeVertexMeta reads vertex types and names from the job DAG (job-info jobDag.vertexInfoMap), accepting both
// object and array shapes.
func realtimeVertexMeta(job *JobInstance) (map[int64]string, map[int64]string) {
	types := map[int64]string{}
	names := map[int64]string{}
	dag := mapValue(job.ResultPreview, "job_dag")
	if dag == nil {
		return types, names
	}
	visit := func(raw any) {
		entry, ok := raw.(map[string]any)
		if !ok {
			return
		}
		id, ok := anyToInt64(entry["vertexId"])
		if !ok {
			return
		}
		types[id] = strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", entry["type"])))
		// STX 配置预览 DAG 用 connectorType，引擎 job-info 用 vertexName（如 "pipeline-1 [Source[0]-FakeSource]"）。
		// The STX config preview DAG uses connectorType; engine job-info uses vertexName (e.g. "pipeline-1 [Source[0]-FakeSource]").
		for _, key := range []string{"connectorType", "vertexName"} {
			if name, ok := entry[key].(string); ok && strings.TrimSpace(name) != "" {
				names[id] = strings.TrimSpace(name)
				break
			}
		}
	}
	switch infoMap := dag["vertexInfoMap"].(type) {
	case map[string]any:
		for _, raw := range infoMap {
			visit(raw)
		}
	case []any:
		for _, raw := range infoMap {
			visit(raw)
		}
	}
	return types, names
}

// realtimeDownstream 从作业 DAG 的 pipelineEdges 建立 节点 → 下游节点 的映射。
// realtimeDownstream builds a vertex → downstream vertices map from the job DAG pipelineEdges.
func realtimeDownstream(job *JobInstance) map[int64][]int64 {
	downstream := map[int64][]int64{}
	dag := mapValue(job.ResultPreview, "job_dag")
	if dag == nil {
		return downstream
	}
	pipelines, ok := dag["pipelineEdges"].(map[string]any)
	if !ok {
		return downstream
	}
	for _, raw := range pipelines {
		edges, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, rawEdge := range edges {
			edge, ok := rawEdge.(map[string]any)
			if !ok {
				continue
			}
			from, okFrom := anyToInt64(edge["inputVertexId"])
			to, okTo := anyToInt64(edge["targetVertexId"])
			if okFrom && okTo {
				downstream[from] = append(downstream[from], to)
			}
		}
	}
	return downstream
}

// inferRealtimeVertexType 在作业 DAG 缺失时按非零字段推断节点类型。
// inferRealtimeVertexType infers the vertex type from non-zero fields when the job DAG is unavailable.
func inferRealtimeVertexType(points []map[string]any) string {
	for _, point := range points {
		switch {
		case realtimeFloat(point, "sourceReadNs") > 0 || realtimeFloat(point, "sourceIdleNs") > 0:
			return "source"
		case realtimeFloat(point, "sinkRecordsIn") > 0 || realtimeFloat(point, "sinkWriteNs") > 0:
			return "sink"
		case realtimeFloat(point, "transformRecordsIn") > 0 || realtimeFloat(point, "transformProcessNs") > 0:
			return "transform"
		}
	}
	return ""
}

func sortRealtimePoints(points []map[string]any) []map[string]any {
	if points == nil {
		return []map[string]any{}
	}
	sort.SliceStable(points, func(i, j int) bool {
		return realtimeFloat(points[i], "ts") < realtimeFloat(points[j], "ts")
	})
	return points
}

func realtimeFloat(point map[string]any, key string) float64 {
	switch value := point[key].(type) {
	case float64:
		return value
	case int64:
		return float64(value)
	case int:
		return float64(value)
	default:
		if parsed, ok := anyToInt64(value); ok {
			return float64(parsed)
		}
		return 0
	}
}

func anyToInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	case interface{ Int64() (int64, error) }:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

// lastRealtimePoints 返回最后 n 个点；不足 n 个时返回 nil。
// lastRealtimePoints returns the last n points, or nil when fewer than n exist.
func lastRealtimePoints(points []map[string]any, n int) []map[string]any {
	if len(points) < n {
		return nil
	}
	return points[len(points)-n:]
}

func allPoints(points []map[string]any, predicate func(map[string]any) bool) bool {
	if len(points) == 0 {
		return false
	}
	for _, point := range points {
		if !predicate(point) {
			return false
		}
	}
	return true
}

func vertexBusyRatio(vertexType string, point map[string]any) float64 {
	switch vertexType {
	case "sink":
		return realtimeFloat(point, "sinkBusyRatio")
	case "transform":
		return realtimeFloat(point, "transformBusyRatio")
	case "source":
		return realtimeFloat(point, "sourceReadRatio")
	default:
		return 0
	}
}

func edgeBackpressured(minBP float64) func(map[string]any) bool {
	return func(point map[string]any) bool {
		return realtimeFloat(point, "bpRatio") >= minBP && realtimeFloat(point, "queueFillRatio") >= realtimeQueueFullThreshold
	}
}

func int64Ptr(value int64) *int64 { return &value }

// computeRealtimeHints 依据最近 3 个桶计算瓶颈提示，结果按严重程度与节点编号排序。
// computeRealtimeHints derives bottleneck hints from the last 3 buckets, sorted by severity then vertex id.
func computeRealtimeHints(vertices []RealtimeVertex, edges []RealtimeEdge, downstream map[int64][]int64) []RealtimeHint {
	hints := []RealtimeHint{}
	inputEdges := map[int64][]RealtimeEdge{}
	for _, edge := range edges {
		inputEdges[edge.TargetVertexID] = append(inputEdges[edge.TargetVertexID], edge)
	}
	vertexByID := map[int64]RealtimeVertex{}
	for _, vertex := range vertices {
		vertexByID[vertex.VertexID] = vertex
	}
	explained := map[int64]bool{}

	for _, vertex := range vertices {
		recent := lastRealtimePoints(vertex.Points, realtimeHintBuckets)
		if recent == nil {
			continue
		}
		latest := recent[len(recent)-1]
		switch vertex.Type {
		case "sink", "transform":
			busy := allPoints(recent, func(p map[string]any) bool { return vertexBusyRatio(vertex.Type, p) >= realtimeBusyThreshold })
			if !busy {
				continue
			}
			for _, edge := range inputEdges[vertex.VertexID] {
				edgeRecent := lastRealtimePoints(edge.Points, realtimeHintBuckets)
				if !allPoints(edgeRecent, edgeBackpressured(realtimeBackpressureWarning)) {
					continue
				}
				code := RealtimeHintTransformBusy
				if vertex.Type == "sink" {
					code = RealtimeHintSinkSlow
				}
				severity := "warning"
				if realtimeFloat(edgeRecent[len(edgeRecent)-1], "bpRatio") >= realtimeBackpressureCritical {
					severity = "critical"
				}
				hints = append(hints, RealtimeHint{Code: code, Severity: severity, VertexID: int64Ptr(vertex.VertexID), QueueID: int64Ptr(edge.QueueID), Buckets: realtimeHintBuckets, Value: vertexBusyRatio(vertex.Type, latest)})
				explained[vertex.VertexID] = true
				break
			}
		case "source":
			observable := firstObservableEdges(vertex.VertexID, downstream, inputEdges)
			if allPoints(recent, func(p map[string]any) bool { return realtimeFloat(p, "sourceIdleRatio") >= realtimeIdleThreshold }) &&
				edgesAll(observable, func(p map[string]any) bool { return realtimeFloat(p, "queueFillRatio") <= realtimeQueueEmptyThreshold }) {
				hints = append(hints, RealtimeHint{Code: RealtimeHintSourceIdle, Severity: "info", VertexID: int64Ptr(vertex.VertexID), Buckets: realtimeHintBuckets, Value: realtimeFloat(latest, "sourceIdleRatio")})
				continue
			}
			if allPoints(recent, func(p map[string]any) bool { return realtimeFloat(p, "sourceReadRatio") >= realtimeBusyThreshold }) &&
				edgesAll(observable, func(p map[string]any) bool { return realtimeFloat(p, "bpRatio") < realtimeNoBackpressure }) {
				hints = append(hints, RealtimeHint{Code: RealtimeHintSourceBusy, Severity: "warning", VertexID: int64Ptr(vertex.VertexID), Buckets: realtimeHintBuckets, Value: realtimeFloat(latest, "sourceReadRatio")})
			}
		}
	}

	for _, edge := range edges {
		if explained[edge.TargetVertexID] {
			continue
		}
		recent := lastRealtimePoints(edge.Points, realtimeHintBuckets)
		if !allPoints(recent, edgeBackpressured(realtimeBackpressureCritical)) {
			continue
		}
		target, ok := vertexByID[edge.TargetVertexID]
		if ok {
			targetRecent := lastRealtimePoints(target.Points, 1)
			if targetRecent != nil && vertexBusyRatio(target.Type, targetRecent[0]) >= realtimeBusyThreshold {
				continue
			}
		}
		hints = append(hints, RealtimeHint{Code: RealtimeHintDownstreamBackpressure, Severity: "critical", VertexID: int64Ptr(edge.TargetVertexID), QueueID: int64Ptr(edge.QueueID), Buckets: realtimeHintBuckets, Value: realtimeFloat(recent[len(recent)-1], "bpRatio")})
	}

	severityRank := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(hints, func(i, j int) bool {
		if severityRank[hints[i].Severity] != severityRank[hints[j].Severity] {
			return severityRank[hints[i].Severity] < severityRank[hints[j].Severity]
		}
		return derefInt64(hints[i].VertexID) < derefInt64(hints[j].VertexID)
	})
	return hints
}

func derefInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

// firstObservableEdges 从 source 沿 DAG 向下找到第一层带队列指标的边。
// firstObservableEdges walks the DAG from a source to the first layer of queue-backed edges.
func firstObservableEdges(sourceID int64, downstream map[int64][]int64, inputEdges map[int64][]RealtimeEdge) []RealtimeEdge {
	visited := map[int64]bool{sourceID: true}
	frontier := []int64{sourceID}
	for len(frontier) > 0 {
		var found []RealtimeEdge
		var next []int64
		for _, vertexID := range frontier {
			for _, target := range downstream[vertexID] {
				if visited[target] {
					continue
				}
				visited[target] = true
				if edges := inputEdges[target]; len(edges) > 0 {
					found = append(found, edges...)
					continue
				}
				next = append(next, target)
			}
		}
		if len(found) > 0 {
			return found
		}
		frontier = next
	}
	return nil
}

// edgesAll 要求每条边最近 3 个桶都满足条件；没有可观测的边时视为满足。
// edgesAll requires the last 3 buckets of every edge to satisfy the predicate; no observable edge counts as satisfied.
func edgesAll(edges []RealtimeEdge, predicate func(map[string]any) bool) bool {
	for _, edge := range edges {
		if !allPoints(lastRealtimePoints(edge.Points, realtimeHintBuckets), predicate) {
			return false
		}
	}
	return true
}
