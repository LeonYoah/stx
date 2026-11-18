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
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// hostPanels 是每台主机渲染的面板：CPU 按 mode 拆分、内存按 kind、负载 / 网络 / 磁盘 IO /
// 文件系统使用率 / TCP / fd。selector 里 %[1]s 固定注入 job="stx_host" + host_id。
// hostPanels are the charts rendered per host: CPU by mode, memory by kind, load / network /
// disk IO / filesystem usage / TCP / fds. %[1]s injects job="stx_host" plus the host_id selector.
var hostPanels = []detailPanel{
	{key: "cpu", unit: "per_second", queries: []detailQuery{
		{dimLabel: "mode", kind: detailRate, expr: `sum by (mode) (rate(stx_host_cpu_seconds_total{%[1]s}[5m]))`},
	}},
	{key: "memory", unit: "bytes", queries: []detailQuery{
		{dimLabel: "kind", expr: `max by (kind) (stx_host_memory_bytes{%[1]s, kind=~"available|cached|buffers|free"})`},
		{dim: "total", expr: `max(stx_host_memory_bytes{%[1]s, kind="total"})`},
	}},
	{key: "load", unit: "count", queries: []detailQuery{
		{dimLabel: "kind", expr: `max by (kind) (stx_host_load{%[1]s})`},
	}},
	{key: "net", unit: "per_second", queries: []detailQuery{
		{dim: "rx", dimLabel: "iface", kind: detailRate, expr: `sum by (iface) (rate(stx_host_net_receive_bytes_total{%[1]s}[5m]))`},
		{dim: "tx", dimLabel: "iface", kind: detailRate, expr: `sum by (iface) (rate(stx_host_net_transmit_bytes_total{%[1]s}[5m]))`},
	}},
	{key: "disk_io", unit: "per_second", queries: []detailQuery{
		{dim: "read", dimLabel: "device", kind: detailRate, expr: `sum by (device) (rate(stx_host_disk_read_bytes_total{%[1]s}[5m]))`},
		{dim: "write", dimLabel: "device", kind: detailRate, expr: `sum by (device) (rate(stx_host_disk_write_bytes_total{%[1]s}[5m]))`},
	}},
	{key: "disk_util", unit: "percent", queries: []detailQuery{
		{dimLabel: "device", kind: detailRate, expr: `100 * sum by (device) (rate(stx_host_disk_io_time_seconds_total{%[1]s}[5m]))`},
	}},
	{key: "fs_used", unit: "percent", queries: []detailQuery{
		{dimLabel: "mountpoint", expr: `100 * (1 - stx_host_fs_avail_bytes{%[1]s} / clamp_min(stx_host_fs_size_bytes{%[1]s}, 1))`},
	}},
	{key: "fs_inodes", unit: "percent", queries: []detailQuery{
		{dimLabel: "mountpoint", expr: `100 * (1 - stx_host_fs_files_free{%[1]s} / clamp_min(stx_host_fs_files{%[1]s}, 1))`},
	}},
	{key: "tcp", unit: "count", queries: []detailQuery{
		{dim: "established", expr: `max(stx_host_tcp_established{%[1]s})`},
	}},
	{key: "fds", unit: "count", queries: []detailQuery{
		{dim: "allocated", expr: `max(stx_host_fds_allocated{%[1]s})`},
	}},
}

// HomeHostEntry 是一台主机 + 它的指标面板。
// HomeHostEntry is one host plus its metric panels.
type HomeHostEntry struct {
	ID               uint          `json:"id"`
	Name             string        `json:"name"`
	IP               string        `json:"ip"`
	HostType         string        `json:"host_type"`
	AgentStatus      string        `json:"agent_status"`
	MetricsSupported bool          `json:"metrics_supported"`
	CPUCores         int           `json:"cpu_cores,omitempty"`
	TotalMemoryBytes int64         `json:"total_memory_bytes,omitempty"`
	LastHeartbeatMs  *int64        `json:"last_heartbeat_ms,omitempty"`
	Panels           []DetailPanel `json:"panels"`
}

// HostDashboard 是 GET /api/v1/dashboard/home/hosts 的返回。
// HostDashboard is the payload of GET /api/v1/dashboard/home/hosts.
type HostDashboard struct {
	Source        string          `json:"source"`
	Reason        string          `json:"reason,omitempty"`
	Range         string          `json:"range"`
	StartMs       int64           `json:"start_ms"`
	EndMs         int64           `json:"end_ms"`
	StepSeconds   int             `json:"step_seconds"`
	Hosts         []HomeHostEntry `json:"hosts"`
	PartialErrors []string        `json:"partial_errors,omitempty"`
}

// HostInventory 解析主机清单（dashboard → host 服务）。
// HostInventory resolves the host list (dashboard → host service).
type HostInventory interface {
	List(ctx context.Context, filter *host.HostFilter) ([]*host.Host, int64, error)
}

// HostClusterNodes 解析集群节点 → 主机映射（cluster_id 过滤用）。
// HostClusterNodes resolves cluster nodes → host mapping (for the cluster_id filter).
type HostClusterNodes interface {
	GetNodes(ctx context.Context, clusterID uint) ([]*cluster.NodeInfo, error)
}

// HostDashboardService 组装主机视图：清单 + 每台主机的指标面板。
// HostDashboardService assembles the hosts view: inventory plus per-host metric panels.
type HostDashboardService struct {
	rangeQuery func(ctx context.Context, query string, start, end time.Time, step int) ([]promquery.Series, error)
	hosts      HostInventory
	clusters   HostClusterNodes
	now        func() time.Time
}

// NewHostDashboardService 创建主机视图服务。
// NewHostDashboardService creates the hosts dashboard service.
func NewHostDashboardService(hosts HostInventory, clusters HostClusterNodes) *HostDashboardService {
	return &HostDashboardService{
		rangeQuery: func(ctx context.Context, query string, start, end time.Time, step int) ([]promquery.Series, error) {
			return promquery.Range(ctx, query, start, end, step)
		},
		hosts:    hosts,
		clusters: clusters,
		now:      time.Now,
	}
}

// hostTask 是一台主机的一条面板查询（并发单元）。
// hostTask is one panel query of one host (the concurrency unit).
type hostTask struct {
	host  int
	panel int
	dim   string
	dimL  string
	query string
	out   []promquery.Series
	err   error
}

// GetHostDashboard 返回主机清单与每台主机的面板；主机列表不受可观测开关影响，
// source=none 时面板为空但清单照回（与集群页签同一份降级语义）。
// GetHostDashboard returns the host inventory plus per-host panels; the inventory is
// unaffected by the observability switch — with source=none panels come back empty but
// hosts still list (same degradation semantics as the cluster tab).
func (s *HostDashboardService) GetHostDashboard(ctx context.Context, clusterID uint, rangeKey string, custom *promquery.Window) (*HostDashboard, error) {
	now := s.now().UTC()
	rangeName, start, end, err := promquery.ResolveWindow(rangeKey, custom, alignPresetNow(now, custom))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidClusterDashboardQuery, err)
	}
	step := promquery.RangeSteps[rangeName]
	if step == 0 {
		step = promquery.AutoStep(end.Sub(start))
	}
	out := &HostDashboard{
		Range: rangeName, StartMs: start.UnixMilli(), EndMs: end.UnixMilli(),
		StepSeconds: step, Hosts: []HomeHostEntry{},
	}

	// 主机清单：全部主机，绑定 agent 的 bare_metal 才有指标；cluster_id 限定到有节点的子集。
	// Host inventory: all hosts; only bare_metal hosts with a bound agent carry metrics;
	// cluster_id narrows to hosts that run nodes of that cluster.
	hosts, _, err := s.hosts.List(ctx, &host.HostFilter{})
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	allowed := map[uint]bool(nil)
	if clusterID > 0 && s.clusters != nil {
		nodes, err := s.clusters.GetNodes(ctx, clusterID)
		if err != nil {
			return nil, fmt.Errorf("list cluster nodes: %w", err)
		}
		allowed = map[uint]bool{}
		for _, node := range nodes {
			if node != nil && node.HostID > 0 {
				allowed[node.HostID] = true
			}
		}
	}
	entries := []*host.Host{}
	for _, item := range hosts {
		if item == nil {
			continue
		}
		if allowed != nil && !allowed[item.ID] {
			continue
		}
		entries = append(entries, item)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	if reason := promquery.UnavailableReason(); reason != "" {
		out.Source, out.Reason = promquery.SourceNone, reason
	} else {
		out.Source = promquery.SourcePrometheus
	}

	for _, item := range entries {
		entry := HomeHostEntry{
			ID: item.ID, Name: item.Name, IP: item.IPAddress,
			HostType:    string(item.HostType),
			AgentStatus: string(item.AgentStatus),
			// 指标只有「bare_metal + 已绑定 agent」一条链路（E3 链路 B）。
			// Metrics only exist through the "bare_metal + bound agent" path (E3 link B).
			MetricsSupported: item.HostType == host.HostTypeBareMetal && strings.TrimSpace(item.AgentID) != "",
			CPUCores:         item.CPUCores,
			TotalMemoryBytes: item.TotalMemory,
			Panels:           []DetailPanel{},
		}
		if item.LastHeartbeat != nil {
			ms := item.LastHeartbeat.UnixMilli()
			entry.LastHeartbeatMs = &ms
		}
		out.Hosts = append(out.Hosts, entry)
	}
	if out.Source != promquery.SourcePrometheus {
		return out, nil
	}

	// 并发跑每台主机的每条面板查询。/ Fan out every panel query of every host concurrently.
	tasks := []*hostTask{}
	for h, item := range entries {
		if !out.Hosts[h].MetricsSupported {
			continue
		}
		sel := fmt.Sprintf(`job="stx_host",host_id=%s`, strconv.Quote(strconv.FormatUint(uint64(item.ID), 10)))
		for p, panel := range hostPanels {
			for _, q := range panel.queries {
				tasks = append(tasks, &hostTask{host: h, panel: p, dim: q.dim, dimL: q.dimLabel, query: fmt.Sprintf(adaptDetailExpr(q.kind, q.expr, step), sel)})
			}
		}
	}
	var wg gosync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task *hostTask) {
			defer wg.Done()
			task.out, task.err = s.rangeQuery(ctx, task.query, start, end, step)
		}(task)
	}
	wg.Wait()

	partialSeen := map[string]bool{}
	for h := range entries {
		entry := &out.Hosts[h]
		// 不支持的主机不出面板（而不是一排空图）。/ Unsupported hosts get no panels at all
		// (rather than a row of empty charts).
		if !entry.MetricsSupported {
			continue
		}
		for p, panel := range hostPanels {
			item := DetailPanel{Key: panel.key, Unit: panel.unit, Series: []DetailSeries{}}
			failed := false
			for _, task := range tasks {
				if task.host != h || task.panel != p {
					continue
				}
				if task.err != nil {
					failed = true
					break
				}
				for _, serie := range task.out {
					item.Series = append(item.Series, DetailSeries{
						Instance: out.Hosts[h].Name,
						Label:    detailSeriesLabel(task, serie),
						Points:   serie.Points,
					})
				}
			}
			if failed && !partialSeen[panel.key] {
				out.PartialErrors = append(out.PartialErrors, panel.key)
				partialSeen[panel.key] = true
			}
			sort.Slice(item.Series, func(a, b int) bool { return item.Series[a].Label < item.Series[b].Label })
			entry.Panels = append(entry.Panels, item)
		}
	}
	return out, nil
}

// detailSeriesLabel 取序列的图例名：维度标签优先；同面板多查询（dim 非空）时
// 前置方向词（如 "rx eth0"）避免撞名。
// detailSeriesLabel picks a series' legend name: the dimension label wins; when a panel
// merges several queries (dim set) the direction word is prefixed (e.g. "rx eth0") to
// avoid name collisions.
func detailSeriesLabel(task *hostTask, serie promquery.Series) string {
	label := strings.TrimSpace(serie.Metric[task.dimL])
	if label == "" {
		label = task.dimL
	}
	switch {
	case task.dim != "" && label != "":
		return task.dim + " " + label
	case label != "":
		return label
	default:
		return task.dim
	}
}

// HostDashboardHandler 处理主机视图接口。
// HostDashboardHandler serves the hosts dashboard endpoint.
type HostDashboardHandler struct {
	service *HostDashboardService
}

// NewHostDashboardHandler 创建主机视图处理器。
// NewHostDashboardHandler creates the hosts dashboard handler.
func NewHostDashboardHandler(service *HostDashboardService) *HostDashboardHandler {
	return &HostDashboardHandler{service: service}
}

// GetHostDashboard 返回主机清单与每台主机的指标面板。
// GetHostDashboard returns the host inventory with per-host metric panels.
// @Summary 首页主机视图（CPU / 内存 / 负载 / 网络 / 磁盘 / 文件系统，按主机分组）
// @Tags Dashboard
// @Produce json
// @Param cluster_id query int false "只看跑该集群节点的主机"
// @Param range query string false "时间范围：15m|1h|6h|24h|7d|30d|90d|1y" default(1h)
// @Param start query string false "自定义起点：RFC3339 或秒 / 毫秒时间戳，优先于 range"
// @Param end query string false "自定义终点，缺省为当前时间；跨度不超过 366 天"
// @Success 200 {object} DashboardDataResponse{data=HostDashboard}
// @Failure 400 {object} DashboardDataResponse
// @Router /api/v1/dashboard/home/hosts [get]
func (h *HostDashboardHandler) GetHostDashboard(c *gin.Context) {
	var clusterID uint
	if raw := strings.TrimSpace(c.Query("cluster_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "invalid cluster_id"})
			return
		}
		clusterID = uint(parsed)
	}
	window, err := promquery.ParseWindow(c.Query("start"), c.Query("end"), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: err.Error()})
		return
	}
	data, err := h.service.GetHostDashboard(c.Request.Context(), clusterID, c.Query("range"), window)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidClusterDashboardQuery) {
			status = http.StatusBadRequest
		}
		c.JSON(status, DashboardDataResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DashboardDataResponse{Data: data})
}
