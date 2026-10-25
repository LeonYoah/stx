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
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"
)

// realtimeRequestTimeout 是单次实时接口请求的超时；引擎侧采集超时为 3 秒，这里保持同级。
// realtimeRequestTimeout bounds one realtime API call; it matches the engine's 3s collection timeout.
const realtimeRequestTimeout = 3 * time.Second

var (
	// ErrEngineNotMaster 表示请求的节点不是 active master（引擎返回 404 "Not a master node"）。
	// ErrEngineNotMaster means the node is not the active master (engine answered 404 "Not a master node").
	ErrEngineNotMaster = errors.New("sync: engine node is not the active master")
	// ErrEngineRealtimeDisabled 表示引擎的实时指标服务未启动（引擎返回 503）。
	// ErrEngineRealtimeDisabled means the engine realtime metrics service is not running (engine answered 503).
	ErrEngineRealtimeDisabled = errors.New("sync: engine realtime metrics service is unavailable")
)

// EngineRealtimeClient 是可选能力：实现了它的引擎客户端才能代理 /metrics/realtime。
// 不扩展 EngineClient，避免牵动大量测试桩。
// EngineRealtimeClient is an optional capability; only engine clients implementing it can proxy /metrics/realtime.
// It is kept separate from EngineClient so existing test stubs stay untouched.
type EngineRealtimeClient interface {
	GetRealtimeJobs(ctx context.Context, endpoint *EngineEndpoint) (*EngineRealtimeJobs, error)
	GetRealtimeVertices(ctx context.Context, endpoint *EngineEndpoint, jobID string, windowMs int64) (*EngineRealtimeSeries, error)
	GetRealtimeEdges(ctx context.Context, endpoint *EngineEndpoint, jobID string, windowMs int64) (*EngineRealtimeSeries, error)
}

// EngineRealtimeJobs 对应 GET /metrics/realtime/jobs。
// EngineRealtimeJobs mirrors GET /metrics/realtime/jobs.
type EngineRealtimeJobs struct {
	Jobs      []EngineRealtimeJobMeta `json:"jobs"`
	Collector map[string]any          `json:"collector"`
}

// EngineRealtimeJobMeta 是 active master 上一条运行中作业的采集状态。jobId 是 Java long，用 json.Number 防止精度丢失。
// EngineRealtimeJobMeta is one running job's collection status on the active master. jobId is a Java long,
// decoded as json.Number to avoid precision loss.
type EngineRealtimeJobMeta struct {
	JobID               json.Number `json:"jobId"`
	Enabled             bool        `json:"enabled"`
	BucketMs            int64       `json:"bucketMs"`
	RetentionMinutes    int         `json:"retentionMinutes"`
	LatestBucketStartMs int64       `json:"latestBucketStartMs"`
}

// EngineRealtimeSeries 对应 vertices/edges 两个接口；作业未知时引擎返回 {}，此时 Known=false。
// EngineRealtimeSeries mirrors the vertices/edges endpoints; the engine returns {} for unknown jobs, giving Known=false.
type EngineRealtimeSeries struct {
	Known    bool                   `json:"-"`
	Enabled  *bool                  `json:"enabled,omitempty"`
	BucketMs int64                  `json:"bucketMs"`
	FromMs   int64                  `json:"fromMs"`
	ToMs     int64                  `json:"toMs"`
	Vertices []EngineRealtimeVertex `json:"vertices"`
	Edges    []EngineRealtimeEdge   `json:"edges"`
}

// EngineRealtimeVertex 是一个 DAG 节点的时间序列，point 字段名保持引擎原样（camelCase）。
// EngineRealtimeVertex is one DAG vertex time series; point field names are kept as the engine sends them (camelCase).
type EngineRealtimeVertex struct {
	VertexID int64            `json:"vertexId"`
	Points   []map[string]any `json:"points"`
}

// EngineRealtimeEdge 是一条带中间队列的边的时间序列，按 queueId 区分，targetVertexId 指向下游节点。
// EngineRealtimeEdge is one queue-backed edge time series keyed by queueId; targetVertexId is the downstream vertex.
type EngineRealtimeEdge struct {
	QueueID        int64            `json:"queueId"`
	TargetVertexID int64            `json:"targetVertexId"`
	Points         []map[string]any `json:"points"`
}

// GetRealtimeJobs 请求 /metrics/realtime/jobs，也用来探测节点是否为 active master。
// GetRealtimeJobs calls /metrics/realtime/jobs; it doubles as the active-master probe.
func (c *SeaTunnelEngineClient) GetRealtimeJobs(ctx context.Context, endpoint *EngineEndpoint) (*EngineRealtimeJobs, error) {
	body, err := c.getRealtime(ctx, endpoint, "/metrics/realtime/jobs", nil)
	if err != nil {
		return nil, err
	}
	var result EngineRealtimeJobs
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("sync: decode realtime jobs: %w", err)
	}
	if result.Jobs == nil {
		result.Jobs = []EngineRealtimeJobMeta{}
	}
	return &result, nil
}

// GetRealtimeVertices 请求某作业的节点时间序列。
// GetRealtimeVertices fetches vertex time series for one job.
func (c *SeaTunnelEngineClient) GetRealtimeVertices(ctx context.Context, endpoint *EngineEndpoint, jobID string, windowMs int64) (*EngineRealtimeSeries, error) {
	return c.getRealtimeSeries(ctx, endpoint, jobID, "vertices", windowMs)
}

// GetRealtimeEdges 请求某作业的边（队列）时间序列。
// GetRealtimeEdges fetches edge (queue) time series for one job.
func (c *SeaTunnelEngineClient) GetRealtimeEdges(ctx context.Context, endpoint *EngineEndpoint, jobID string, windowMs int64) (*EngineRealtimeSeries, error) {
	return c.getRealtimeSeries(ctx, endpoint, jobID, "edges", windowMs)
}

func (c *SeaTunnelEngineClient) getRealtimeSeries(ctx context.Context, endpoint *EngineEndpoint, jobID, kind string, windowMs int64) (*EngineRealtimeSeries, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("sync: engine job id is required")
	}
	path := "/metrics/realtime/jobs/" + neturl.PathEscape(jobID) + "/" + kind
	body, err := c.getRealtime(ctx, endpoint, path, map[string]string{"windowMs": strconv.FormatInt(windowMs, 10)})
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("sync: decode realtime %s: %w", kind, err)
	}
	result := &EngineRealtimeSeries{Vertices: []EngineRealtimeVertex{}, Edges: []EngineRealtimeEdge{}}
	if len(raw) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(body, result); err != nil {
		return nil, fmt.Errorf("sync: decode realtime %s: %w", kind, err)
	}
	result.Known = true
	if result.Vertices == nil {
		result.Vertices = []EngineRealtimeVertex{}
	}
	if result.Edges == nil {
		result.Edges = []EngineRealtimeEdge{}
	}
	return result, nil
}

// getRealtime 发送 GET 并把引擎的非 2xx 映射为稳定错误：404 "Not a master node" → ErrEngineNotMaster，503 → ErrEngineRealtimeDisabled。
// getRealtime issues a GET and maps engine non-2xx responses to stable errors: 404 "Not a master node" →
// ErrEngineNotMaster, 503 → ErrEngineRealtimeDisabled.
func (c *SeaTunnelEngineClient) getRealtime(ctx context.Context, endpoint *EngineEndpoint, path string, params map[string]string) ([]byte, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("sync: engine client is not configured")
	}
	targetURL, err := buildEngineURL(endpoint, path, params)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, realtimeRequestTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return body, nil
	case resp.StatusCode == http.StatusNotFound && strings.Contains(strings.ToLower(string(body)), "not a master node"):
		return nil, ErrEngineNotMaster
	case resp.StatusCode == http.StatusServiceUnavailable:
		return nil, ErrEngineRealtimeDisabled
	default:
		return nil, fmt.Errorf("sync: realtime request failed: status=%d", resp.StatusCode)
	}
}
