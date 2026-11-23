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
	"sort"
	"strconv"
	"time"

	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
	"github.com/LeonYoah/stx/internal/version"
)

// platformHostLister 抽象 hosts 表读取：快照以 hosts 表为主集合并指标，
// 离线 / 旧版 Agent 也要出现在列表里。
// platformHostLister abstracts hosts-table reads: the snapshot uses hosts as the
// primary set and merges metrics on top, so offline / legacy agents still appear.
type platformHostLister interface {
	List(ctx context.Context, filter *host.HostFilter) ([]*host.Host, int64, error)
	GetHeartbeatTimeout() time.Duration
	GetProcessStartedAt() time.Time
}

// SetPlatformHostLister 注入 hosts 表读取依赖；未注入时 Agent 列表为空。
// SetPlatformHostLister injects the hosts-table dependency; without it the agent
// list stays empty.
func (s *Service) SetPlatformHostLister(l platformHostLister) {
	s.platformHosts = l
}

// PlatformSnapshot 是 GET /api/v1/monitoring/platform 与 stx monitoring platform
// 共用的一份平台自监控快照。
// PlatformSnapshot is the single self-monitoring snapshot shared by
// GET /api/v1/monitoring/platform and `stx monitoring platform`.
type PlatformSnapshot struct {
	// Available 为 false 时 Reason 说明降级原因（复用 observability reason 语义）。
	// Available=false carries the degradation reason (reusing observability reason codes).
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`

	Server *PlatformServerSnapshot `json:"server,omitempty"`
	Agents []PlatformAgentSnapshot `json:"agents"`
	// Trends 是 server 侧轻量趋势（最近 1h、60s 步长）；单条查询失败时该序列为 null。
	// Trends is the lightweight server-side trend (last 1h at a 60s step); a failed
	// query leaves its series null.
	Trends *PlatformTrends `json:"trends,omitempty"`
}

// PlatformTrends 是平台快照携带的固定趋势序列集合（Points 为 [毫秒时间戳, 值]）。
// PlatformTrends is the fixed set of trend series carried by the snapshot
// (Points are [epoch ms, value]).
type PlatformTrends struct {
	RSSBytes       [][2]float64 `json:"rss_bytes,omitempty"`
	HeapInuseBytes [][2]float64 `json:"heap_inuse_bytes,omitempty"`
	HTTPRPS        [][2]float64 `json:"http_rps,omitempty"`
	HTTP5xxRatio   [][2]float64 `json:"http_5xx_ratio,omitempty"`
}

// PlatformServerSnapshot 是 STX server 自身的进程与控制面指标快照。
// PlatformServerSnapshot is the STX server process / control-plane metrics snapshot.
type PlatformServerSnapshot struct {
	Version         string               `json:"version"`
	UptimeSeconds   float64              `json:"uptime_seconds"`
	RSSBytes        float64              `json:"rss_bytes"`
	HeapInuseBytes  float64              `json:"heap_inuse_bytes"`
	Goroutines      float64              `json:"goroutines"`
	HTTPRPS         float64              `json:"http_rps"`
	HTTP5xxRatio    float64              `json:"http_5xx_ratio"`
	HTTPP95Seconds  float64              `json:"http_p95_seconds"`
	DBOpen          float64              `json:"db_open"`
	DBInUse         float64              `json:"db_in_use"`
	DBWaitCountRate float64              `json:"db_wait_count_rate"`
	AgentsConnected float64              `json:"agents_connected"`
	TSDB            PlatformTSDBSnapshot `json:"tsdb"`
}

// PlatformTSDBSnapshot 是内嵌 tsdb 的存储概况（来自 embedts.GetStorageStatus）。
// PlatformTSDBSnapshot is the embedded-tsdb storage overview (from embedts.GetStorageStatus).
type PlatformTSDBSnapshot struct {
	HeadSeries        uint64  `json:"head_series"`
	BlocksBytes       int64   `json:"blocks_bytes"`
	CompactionsFailed float64 `json:"compactions_failed"`
}

// PlatformAgentSnapshot 是一台 Agent 的自指标快照；指标缺失的字段为 null，
// 供前端显示 "—"（旧版 Agent 不上报 stx_agent_*）。
// PlatformAgentSnapshot is one agent's self-metrics snapshot; missing metrics stay
// null so the frontend can render "—" (legacy agents do not report stx_agent_*).
type PlatformAgentSnapshot struct {
	HostID            uint       `json:"host_id"`
	HostName          string     `json:"host_name"`
	Version           string     `json:"version"`
	RSSBytes          *float64   `json:"rss_bytes"`
	CPURatio          *float64   `json:"cpu_ratio"`
	Goroutines        *float64   `json:"goroutines"`
	UptimeSeconds     *float64   `json:"uptime_seconds"`
	CommandFailures1h *float64   `json:"command_failures_1h"`
	LastReportAt      *time.Time `json:"last_report_at"`
	Online            bool       `json:"online"`
}

// platformSnapshotQueries 是快照用到的全部即时查询；每条单独执行、失败降级为 0/空，
// 单条查询异常不拖垮整个快照。
// platformSnapshotQueries lists every instant query the snapshot needs; each runs on
// its own and degrades to 0/empty on failure so one bad query cannot sink the whole snapshot.
const (
	platformQueryBuildInfo    = `stx_server_build_info{job="stx_embedded"}`
	platformQueryUptime       = `time() - process_start_time_seconds{job="stx_embedded"}`
	platformQueryRSS          = `process_resident_memory_bytes{job="stx_embedded"}`
	platformQueryHeap         = `go_memstats_heap_inuse_bytes{job="stx_embedded"}`
	platformQueryGoroutines   = `go_goroutines{job="stx_embedded"}`
	platformQueryHTTPRPS      = `sum(rate(stx_server_http_requests_total[5m]))`
	platformQueryHTTP5xxRatio = `sum(rate(stx_server_http_requests_total{code="5xx"}[5m])) / sum(rate(stx_server_http_requests_total[5m]))`
	platformQueryHTTPP95      = `histogram_quantile(0.95, sum by (le) (rate(stx_server_http_request_duration_seconds_bucket[5m])))`
	platformQueryDBOpen       = `go_sql_open_connections{db_name="stx"}`
	platformQueryDBInUse      = `go_sql_in_use_connections{db_name="stx"}`
	platformQueryDBWaitRate   = `rate(go_sql_wait_count_total{db_name="stx"}[5m])`
	platformQueryAgentsConn   = `stx_server_agents_connected`
	platformQueryAgentRSS     = `stx_agent_process_resident_memory_bytes`
	platformQueryAgentCPURate = `rate(stx_agent_process_cpu_seconds_total[5m])`
	platformQueryAgentGorout  = `stx_agent_goroutines`
	platformQueryAgentUptime  = `stx_agent_uptime_seconds`
	platformQueryAgentCmdFail = `sum by (host_id) (increase(stx_agent_commands_total{result="failed"}[1h]))`
	// 最近上报时间取样本自身的时间戳（TsMs），stx_agent_uptime_seconds 缺失时
	// 回退 stx_host_load（旧版 Agent 只报主机指标）。
	// Last-report time reads the samples' own timestamps (TsMs); stx_agent_uptime_seconds
	// falls back to stx_host_load for legacy agents that only report host metrics.
	platformQueryAgentReportAt = `stx_agent_uptime_seconds or stx_host_load{kind="1m"}`
	platformQueryAgentBuild    = `stx_agent_build_info`
)

// GetPlatformSnapshot 组装平台自监控快照。
// observability 不可用时返回 available=false + reason（不报错），供调用方统一空态降级。
// GetPlatformSnapshot assembles the platform self-monitoring snapshot.
// When observability is unavailable it returns available=false plus the reason
// (no error) so callers degrade through the shared empty-state path.
func (s *Service) GetPlatformSnapshot(ctx context.Context) (*PlatformSnapshot, error) {
	snap := &PlatformSnapshot{Agents: []PlatformAgentSnapshot{}}
	if reason := promquery.UnavailableReason(); reason != "" {
		snap.Reason = reason
		return snap, nil
	}
	snap.Available = true
	now := time.Now()
	snap.Server = s.platformServerSnapshot(ctx, now)
	snap.Agents = s.platformAgentSnapshots(ctx, now)
	snap.Trends = platformTrends(ctx, now)
	return snap, nil
}

// platformTrends 组装固定四条 server 趋势：RSS / heap / QPS / 5xx 比例，最近 1h、
// 60s 步长。单条失败只置空该序列，不影响整体快照。
// platformTrends assembles the four fixed server trends: RSS / heap / QPS / 5xx
// ratio over the last hour at a 60s step. A failed query only blanks its own series.
func platformTrends(ctx context.Context, now time.Time) *PlatformTrends {
	start := now.Add(-time.Hour)
	firstPoints := func(promQL string) [][2]float64 {
		series, err := promquery.Range(ctx, promQL, start, now, 60)
		if err != nil || len(series) == 0 {
			if err != nil {
				logger.WarnF(ctx, "platform trend query failed expr=%q: %v", promQL, err)
			}
			return nil
		}
		return series[0].Points
	}
	return &PlatformTrends{
		RSSBytes:       firstPoints(platformQueryRSS),
		HeapInuseBytes: firstPoints(platformQueryHeap),
		HTTPRPS:        firstPoints(platformQueryHTTPRPS),
		HTTP5xxRatio:   firstPoints(platformQueryHTTP5xxRatio),
	}
}

// platformServerSnapshot 查询 server 侧指标并拼装概况。
// platformServerSnapshot queries the server-side metrics and composes the overview.
func (s *Service) platformServerSnapshot(ctx context.Context, now time.Time) *PlatformServerSnapshot {
	out := &PlatformServerSnapshot{Version: version.Version}
	for _, sample := range platformInstant(ctx, platformQueryBuildInfo, now) {
		if v := sample.Metric["version"]; v != "" {
			out.Version = v
		}
	}
	out.UptimeSeconds = platformScalar(ctx, platformQueryUptime, now)
	out.RSSBytes = platformScalar(ctx, platformQueryRSS, now)
	out.HeapInuseBytes = platformScalar(ctx, platformQueryHeap, now)
	out.Goroutines = platformScalar(ctx, platformQueryGoroutines, now)
	out.HTTPRPS = platformScalar(ctx, platformQueryHTTPRPS, now)
	out.HTTP5xxRatio = platformScalar(ctx, platformQueryHTTP5xxRatio, now)
	out.HTTPP95Seconds = platformScalar(ctx, platformQueryHTTPP95, now)
	out.DBOpen = platformScalar(ctx, platformQueryDBOpen, now)
	out.DBInUse = platformScalar(ctx, platformQueryDBInUse, now)
	out.DBWaitCountRate = platformScalar(ctx, platformQueryDBWaitRate, now)
	out.AgentsConnected = platformScalar(ctx, platformQueryAgentsConn, now)
	if status, ok := embedts.GetStorageStatus(); ok {
		out.TSDB = PlatformTSDBSnapshot{
			HeadSeries:        status.HeadSeries,
			BlocksBytes:       status.BlocksBytes,
			CompactionsFailed: status.CompactionsFailed,
		}
	}
	return out
}

// platformAgentSnapshots 以 hosts 表（bare_metal）为主集合，按 host_id 合并指标。
// 指标缺失的字段保持 nil；online 与 host 列表同一判定口径（心跳超时 + 进程启动时间）。
// platformAgentSnapshots merges metrics onto the hosts table (bare_metal) keyed by
// host_id. Missing metrics stay nil; online uses the same rule as the host list
// (heartbeat timeout + process start time).
func (s *Service) platformAgentSnapshots(ctx context.Context, now time.Time) []PlatformAgentSnapshot {
	out := []PlatformAgentSnapshot{}
	if s.platformHosts == nil {
		return out
	}
	hosts, _, err := s.platformHosts.List(ctx, &host.HostFilter{
		HostType: host.HostTypeBareMetal,
		PageSize: 10000,
	})
	if err != nil {
		logger.WarnF(ctx, "failed to list hosts for platform snapshot, agent list left empty: %v", err)
		return out
	}

	rss := platformByHost(ctx, platformQueryAgentRSS, now)
	cpuRate := platformByHost(ctx, platformQueryAgentCPURate, now)
	goroutines := platformByHost(ctx, platformQueryAgentGorout, now)
	uptime := platformByHost(ctx, platformQueryAgentUptime, now)
	cmdFail := platformByHost(ctx, platformQueryAgentCmdFail, now)
	reportAt := platformLastReportAt(ctx, platformQueryAgentReportAt, now)
	buildVersions := map[uint]string{}
	for _, sample := range platformInstant(ctx, platformQueryAgentBuild, now) {
		if hid, ok := platformHostID(sample.Metric); ok {
			buildVersions[hid] = sample.Metric["version"]
		}
	}

	timeout := s.platformHosts.GetHeartbeatTimeout()
	since := s.platformHosts.GetProcessStartedAt()
	for _, h := range hosts {
		if h == nil {
			continue
		}
		agent := PlatformAgentSnapshot{
			HostID:   h.ID,
			HostName: h.Name,
			Version:  h.AgentVersion,
			Online:   h.IsOnlineWithSince(timeout, since),
		}
		if agent.Version == "" {
			agent.Version = buildVersions[h.ID]
		}
		if v, ok := rss[h.ID]; ok {
			agent.RSSBytes = v
		}
		if v, ok := cpuRate[h.ID]; ok {
			agent.CPURatio = v
		}
		if v, ok := goroutines[h.ID]; ok {
			agent.Goroutines = v
		}
		if v, ok := uptime[h.ID]; ok {
			agent.UptimeSeconds = v
		}
		if v, ok := cmdFail[h.ID]; ok {
			agent.CommandFailures1h = v
		}
		if v, ok := reportAt[h.ID]; ok {
			ts := time.UnixMilli(v)
			agent.LastReportAt = &ts
		}
		out = append(out, agent)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HostID < out[j].HostID })
	return out
}

// platformInstant 执行一条即时查询，失败只记 WARN 并返回空结果。
// platformInstant runs one instant query; failures only WARN and yield empty results.
func platformInstant(ctx context.Context, promQL string, at time.Time) []promquery.Sample {
	samples, err := promquery.Instant(ctx, promQL, at)
	if err != nil {
		logger.WarnF(ctx, "platform snapshot query failed expr=%q: %v", promQL, err)
		return nil
	}
	return samples
}

// platformScalar 取即时查询的第一个样本值；空结果或 NaN 归 0。
// platformScalar reads the first sample of an instant query; empty or NaN collapses to 0.
func platformScalar(ctx context.Context, promQL string, at time.Time) float64 {
	samples := platformInstant(ctx, promQL, at)
	if len(samples) == 0 {
		return 0
	}
	v := samples[0].Value
	if v != v { // NaN
		return 0
	}
	return v
}

// platformHostID 解析样本上的 host_id 标签为 uint。
// platformHostID parses the host_id label of a sample into a uint.
func platformHostID(metric map[string]string) (uint, bool) {
	raw, ok := metric["host_id"]
	if !ok || raw == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

// platformLastReportAt 聚合每个 host_id 的最新样本时间戳（毫秒）。
// platformLastReportAt folds each host_id's newest sample timestamp (ms).
func platformLastReportAt(ctx context.Context, promQL string, at time.Time) map[uint]int64 {
	out := map[uint]int64{}
	for _, sample := range platformInstant(ctx, promQL, at) {
		hid, ok := platformHostID(sample.Metric)
		if !ok {
			continue
		}
		if sample.TsMs > out[hid] {
			out[hid] = sample.TsMs
		}
	}
	return out
}

// platformByHost 把一条按 host_id 出样本的即时查询聚合成 host_id → 值。
// platformByHost collapses a per-host_id instant query into a host_id → value map.
func platformByHost(ctx context.Context, promQL string, at time.Time) map[uint]*float64 {
	out := map[uint]*float64{}
	for _, sample := range platformInstant(ctx, promQL, at) {
		hid, ok := platformHostID(sample.Metric)
		if !ok {
			continue
		}
		v := sample.Value
		out[hid] = &v
	}
	return out
}
