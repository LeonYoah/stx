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
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// availabilityCells 是节点可用性状态条的格数；可用性查询的 step 就是窗口 / 格数，前端一点对应一格。
// availabilityCells is the number of cells in the node availability bar; the availability step is window / cells so
// each returned point maps to one cell in the UI.
const availabilityCells = 48

// ErrInvalidClusterDashboardQuery 表示集群大盘的时间窗参数非法。
// ErrInvalidClusterDashboardQuery reports an invalid cluster dashboard window parameter.
var ErrInvalidClusterDashboardQuery = errors.New("dashboard: invalid cluster dashboard query")

// ClusterHealthSource 提供集群健康与告警计数（monitoring 服务实现）。
// ClusterHealthSource supplies cluster health and alert counts (implemented by the monitoring service).
type ClusterHealthSource interface {
	GetClustersHealthFor(ctx context.Context, clusterID uint) (*monitoring.ClusterHealthData, error)
}

// ClusterNodeSource 提供集群与节点（cluster 服务实现）。
// ClusterNodeSource supplies clusters and nodes (implemented by the cluster service).
type ClusterNodeSource interface {
	List(ctx context.Context, filter *cluster.ClusterFilter) ([]*cluster.Cluster, int64, error)
	GetNodes(ctx context.Context, clusterID uint) ([]*cluster.NodeInfo, error)
}

// RunningJobCounter 提供每个集群的运行中作业数（sync 服务实现）。
// RunningJobCounter supplies running job counts per cluster (implemented by the sync service).
type RunningJobCounter interface {
	RunningJobCountsByCluster(ctx context.Context) (map[uint]int64, error)
}

// ClusterCard 是集群卡片：DB / 心跳字段始终有，指标字段仅在 Prometheus 可用时出现。
// ClusterCard is one cluster card: DB / heartbeat fields are always present, metric fields only with Prometheus.
type ClusterCard struct {
	ID             uint     `json:"id"`
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	HealthStatus   string   `json:"health_status"`
	NodesOnline    int      `json:"nodes_online"`
	NodesTotal     int      `json:"nodes_total"`
	RunningJobs    int64    `json:"running_jobs"`
	ActiveAlerts   int64    `json:"active_alerts"`
	CriticalAlerts int64    `json:"critical_alerts"`
	MaxHeapPercent *float64 `json:"max_heap_percent,omitempty"`
	EndpointsDown  *int     `json:"endpoints_down,omitempty"`
}

// ClusterNodeRow 是节点表的一行；instance = 主机 IP + Hazelcast 端口，与 HTTP SD 目标一致。
// ClusterNodeRow is one node table row; instance = host IP + Hazelcast port, matching the HTTP SD target.
type ClusterNodeRow struct {
	ClusterID     uint     `json:"cluster_id"`
	ClusterName   string   `json:"cluster_name"`
	NodeID        uint     `json:"node_id"`
	Instance      string   `json:"instance"`
	Role          string   `json:"role"`
	Status        string   `json:"status"`
	AgentOnline   bool     `json:"agent_online"`
	Master        *bool    `json:"master,omitempty"`
	ScrapeUp      *bool    `json:"scrape_up,omitempty"`
	NodeActive    *bool    `json:"node_active,omitempty"`
	HeapPercent   *float64 `json:"heap_percent,omitempty"`
	CPUCores      *float64 `json:"cpu_cores,omitempty"`
	Threads       *float64 `json:"threads,omitempty"`
	FDPercent     *float64 `json:"fd_percent,omitempty"`
	UptimeSeconds *float64 `json:"uptime_seconds,omitempty"`
}

// ClusterSignals 是异常信号（原 Grafana P0 / P3 / P4 面板）。
// ClusterSignals are the anomaly signals (the former Grafana P0 / P3 / P4 panels).
type ClusterSignals struct {
	ClustersSafe      int      `json:"clusters_safe"`
	ClustersTotal     int      `json:"clusters_total"`
	DeadlockedThreads *float64 `json:"deadlocked_threads,omitempty"`
	FDPercentMax      *float64 `json:"fd_percent_max,omitempty"`
	PoolRejectRate    *float64 `json:"pool_reject_rate,omitempty"`
	HZQueueUtilMax    *float64 `json:"hz_queue_util_max,omitempty"`
	EndpointsDown     int      `json:"endpoints_down"`
}

// DashboardSeries 是一条趋势；Points 为 [毫秒时间戳, 值]。
// DashboardSeries is one trend; Points are [epoch ms, value].
type DashboardSeries struct {
	Key    string       `json:"key"`
	Label  string       `json:"label"`
	Points [][2]float64 `json:"points"`
}

// ClusterDashboard 是 GET /api/v1/dashboard/home/clusters 的返回。
// ClusterDashboard is the payload of GET /api/v1/dashboard/home/clusters.
type ClusterDashboard struct {
	Source        string            `json:"source"`
	Reason        string            `json:"reason,omitempty"`
	Range         string            `json:"range"`
	StartMs       int64             `json:"start_ms"`
	EndMs         int64             `json:"end_ms"`
	StepSeconds   int               `json:"step_seconds"`
	CellSeconds   int               `json:"cell_seconds"`
	Clusters      []ClusterCard     `json:"clusters"`
	Nodes         []ClusterNodeRow  `json:"nodes"`
	Signals       *ClusterSignals   `json:"signals"`
	Series        []DashboardSeries `json:"series"`
	PartialErrors []string          `json:"partial_errors,omitempty"`
}

// ClusterDashboardService 组合 DB / 心跳与 Prometheus，生成首页集群大盘。
// ClusterDashboardService combines DB / heartbeat data with Prometheus into the home cluster dashboard.
type ClusterDashboardService struct {
	health   ClusterHealthSource
	clusters ClusterNodeSource
	jobs     RunningJobCounter
	now      func() time.Time
	// query 可在测试中替换；默认走 promquery。/ query is swappable in tests; promquery by default.
	rangeQuery   rangeQueryFunc
	instantQuery instantQueryFunc
}

// NewClusterDashboardService 创建集群大盘服务。
// NewClusterDashboardService creates the cluster dashboard service.
func NewClusterDashboardService(health ClusterHealthSource, clusters ClusterNodeSource, jobs RunningJobCounter) *ClusterDashboardService {
	return &ClusterDashboardService{
		health: health, clusters: clusters, jobs: jobs, now: time.Now,
		rangeQuery: dashboardPromCache.wrapRange(promquery.Range), instantQuery: dashboardPromCache.wrapInstant(promquery.Instant),
	}
}

// GetClusterDashboard 生成集群大盘。只有时间窗非法时返回错误；其余失败记入 PartialErrors。
// GetClusterDashboard builds the cluster dashboard. Only an invalid window returns an error; other failures go to
// PartialErrors.
func (s *ClusterDashboardService) GetClusterDashboard(ctx context.Context, clusterID uint, rangeKey string, custom *promquery.Window) (*ClusterDashboard, error) {
	now := s.now().UTC()
	// 预设窗口的终点按缓存粒度取整，见 alignPresetNow。/ Preset window ends are aligned to the cache granularity; see alignPresetNow.
	rangeName, start, end, err := promquery.ResolveWindow(rangeKey, custom, alignPresetNow(now, custom))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidClusterDashboardQuery, err)
	}
	step := promquery.RangeSteps[rangeName]
	if step == 0 {
		step = promquery.AutoStep(end.Sub(start))
	}
	cell := int(end.Sub(start).Seconds()) / availabilityCells
	if cell < step {
		cell = step
	}
	out := &ClusterDashboard{
		Range: rangeName, StartMs: start.UnixMilli(), EndMs: end.UnixMilli(), StepSeconds: step, CellSeconds: cell,
		Clusters: []ClusterCard{}, Nodes: []ClusterNodeRow{}, Series: []DashboardSeries{},
	}
	s.fillInventory(ctx, out, clusterID)

	baseURL, reason := promquery.BaseURL()
	if baseURL == "" {
		out.Source, out.Reason = promquery.SourceNone, reason
		return out, nil
	}
	out.Source = promquery.SourcePrometheus
	s.fillMetrics(ctx, out, baseURL, clusterID, start, end, step, cell)
	return out, nil
}

// fillInventory 填 DB / 心跳部分：集群卡片始终包含全部集群（便于切换范围），节点表只含范围内的集群。
// 告警计数按当前 clusterID 查询，避免单集群大盘把所有集群的事件再扫一遍。
// fillInventory fills the DB / heartbeat part: cards always cover every cluster (so the scope can be switched), the
// node table only the clusters in scope. Alert counts are loaded for the current clusterID so a single-cluster
// dashboard does not rescan every cluster's events.
func (s *ClusterDashboardService) fillInventory(ctx context.Context, out *ClusterDashboard, clusterID uint) {
	if s.clusters == nil {
		return
	}
	clusters, _, err := s.clusters.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		out.PartialErrors = append(out.PartialErrors, "clusters")
		return
	}
	health := map[uint]*monitoring.ClusterHealthItem{}
	if s.health != nil {
		if data, err := s.health.GetClustersHealthFor(ctx, clusterID); err != nil {
			out.PartialErrors = append(out.PartialErrors, "health")
		} else {
			for _, item := range data.Clusters {
				if item != nil {
					health[item.ClusterID] = item
				}
			}
		}
	}
	running := map[uint]int64{}
	if s.jobs != nil {
		if counts, err := s.jobs.RunningJobCountsByCluster(ctx); err != nil {
			out.PartialErrors = append(out.PartialErrors, "jobs")
		} else {
			running = counts
		}
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].ID < clusters[j].ID })
	for _, c := range clusters {
		if c == nil {
			continue
		}
		card := ClusterCard{ID: c.ID, Name: c.Name, Version: c.Version, HealthStatus: "unknown", RunningJobs: running[c.ID]}
		if item := health[c.ID]; item != nil {
			card.HealthStatus = item.HealthStatus
			card.NodesOnline, card.NodesTotal = item.OnlineNodes, item.TotalNodes
			card.ActiveAlerts, card.CriticalAlerts = item.ActiveAlerts, item.CriticalAlerts
		}
		out.Clusters = append(out.Clusters, card)
		if clusterID > 0 && c.ID != clusterID {
			continue
		}
		nodes, err := s.clusters.GetNodes(ctx, c.ID)
		if err != nil {
			out.PartialErrors = append(out.PartialErrors, "nodes")
			continue
		}
		for _, node := range nodes {
			if node == nil {
				continue
			}
			row := ClusterNodeRow{
				ClusterID: c.ID, ClusterName: c.Name, NodeID: node.ID, Role: string(node.Role),
				Status: string(node.Status), AgentOnline: node.IsOnline,
			}
			if host := strings.TrimSpace(node.HostIP); host != "" && node.HazelcastPort > 0 {
				row.Instance = net.JoinHostPort(host, strconv.Itoa(node.HazelcastPort))
			}
			out.Nodes = append(out.Nodes, row)
		}
	}
}

// promTask 是一条并发执行的 Prometheus 查询。
// promTask is one Prometheus query run concurrently.
type promTask struct {
	key     string
	query   string
	instant bool
	step    int
	series  []promquery.Series
	samples []promquery.Sample
	err     error
}

// fillMetrics 并发执行全部查询后组装趋势、节点即时值、卡片指标与异常信号。
// fillMetrics runs every query concurrently, then assembles trends, node instants, card metrics and signals.
func (s *ClusterDashboardService) fillMetrics(ctx context.Context, out *ClusterDashboard, baseURL string, clusterID uint, start, end time.Time, step, cell int) {
	// all 用于集群卡片（全部集群），sel 用于范围内的节点、趋势与信号。
	// all feeds the cluster cards (every cluster); sel feeds the in-scope nodes, trends and signals.
	all := `job="seatunnel_engine_http"`
	sel := all
	if clusterID > 0 {
		sel += fmt.Sprintf(`,cluster_id=%s`, strconv.Quote(strconv.FormatUint(uint64(clusterID), 10)))
	}
	heapExpr := fmt.Sprintf(`100 * max by (cluster_id, instance) (jvm_memory_bytes_used{%[1]s,area="heap"}) / max by (cluster_id, instance) (clamp_min(jvm_memory_bytes_max{%[1]s,area="heap"}, 1))`, sel)
	cpuExpr := fmt.Sprintf(`sum by (cluster_id, instance) (rate(process_cpu_seconds_total{%s}[5m]))`, sel)
	tasks := []*promTask{
		{key: "up_range", step: cell, query: fmt.Sprintf(`min by (instance) (min_over_time(up{%s}[%ds]))`, sel, cell)},
		{key: "state_range", step: cell, query: fmt.Sprintf(`min by (instance) (min_over_time(node_state{%s}[%ds]))`, sel, cell)},
		{key: "heap", step: step, query: heapExpr},
		{key: "cpu", step: step, query: cpuExpr},
		{key: "gc", step: step, query: fmt.Sprintf(`100 * sum by (cluster_id, instance) (rate(jvm_gc_collection_seconds_sum{%s}[5m]))`, sel)},
		{key: "job_state", step: step, query: fmt.Sprintf(`sum by (type) (job_count{%s})`, sel)},
		{key: "heap_now", instant: true, query: heapExpr},
		{key: "cpu_now", instant: true, query: cpuExpr},
		{key: "threads_now", instant: true, query: fmt.Sprintf(`max by (cluster_id, instance) (jvm_threads_current{%s})`, sel)},
		{key: "fd_now", instant: true, query: fmt.Sprintf(`100 * max by (cluster_id, instance) (process_open_fds{%[1]s}) / max by (cluster_id, instance) (clamp_min(process_max_fds{%[1]s}, 1))`, sel)},
		{key: "uptime_now", instant: true, query: fmt.Sprintf(`max by (cluster_id, instance) (time() - process_start_time_seconds{%s})`, sel)},
		{key: "up_now", instant: true, query: fmt.Sprintf(`max by (cluster_id, instance) (up{%s})`, all)},
		{key: "heap_all", instant: true, query: fmt.Sprintf(`100 * max by (cluster_id, instance) (jvm_memory_bytes_used{%[1]s,area="heap"}) / max by (cluster_id, instance) (clamp_min(jvm_memory_bytes_max{%[1]s,area="heap"}, 1))`, all)},
		{key: "state_now", instant: true, query: fmt.Sprintf(`min by (cluster_id, instance) (node_state{%s})`, sel)},
		{key: "cluster_info", instant: true, query: fmt.Sprintf(`max by (cluster_id, instance, master) (cluster_info{%s})`, sel)},
		{key: "cluster_safe", instant: true, query: fmt.Sprintf(`min by (cluster_id) (hazelcast_partition_isClusterSafe{%s})`, sel)},
		{key: "deadlocked", instant: true, query: fmt.Sprintf(`max(jvm_threads_deadlocked{%s})`, sel)},
		{key: "reject", instant: true, query: fmt.Sprintf(`sum(rate(job_thread_pool_rejection_total{%s}[5m]))`, sel)},
		{key: "hz_util", instant: true, query: fmt.Sprintf(`max(100 * hazelcast_executor_queueSize{%[1]s} / clamp_min(hazelcast_executor_queueSize{%[1]s} + hazelcast_executor_queueRemainingCapacity{%[1]s}, 1))`, sel)},
	}
	var wg gosync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task *promTask) {
			defer wg.Done()
			if task.instant {
				task.samples, task.err = s.instantQuery(ctx, baseURL, task.query, end)
			} else {
				task.series, task.err = s.rangeQuery(ctx, baseURL, task.query, start, end, task.step)
			}
		}(task)
	}
	wg.Wait()
	byKey := map[string]*promTask{}
	for _, task := range tasks {
		byKey[task.key] = task
		if task.err != nil {
			out.PartialErrors = append(out.PartialErrors, task.key)
		}
	}

	out.Series = append(out.Series, mergeAvailability(byKey["up_range"].series, byKey["state_range"].series)...)
	for _, key := range []string{"heap", "cpu", "gc"} {
		for _, item := range byKey[key].series {
			out.Series = append(out.Series, DashboardSeries{Key: key, Label: item.Metric["instance"], Points: item.Points})
		}
	}
	for _, item := range byKey["job_state"].series {
		out.Series = append(out.Series, DashboardSeries{Key: "job_state", Label: item.Metric["type"], Points: item.Points})
	}

	instant := func(key string) map[string]float64 {
		values := map[string]float64{}
		for _, sample := range byKey[key].samples {
			values[sample.Metric["instance"]] = sample.Value
		}
		return values
	}
	heap, cpu, threads, fd, uptime := instant("heap_now"), instant("cpu_now"), instant("threads_now"), instant("fd_now"), instant("uptime_now")
	up, state := instant("up_now"), instant("state_now")
	masters := map[string]bool{}
	reporters := map[string]string{}
	for _, sample := range byKey["cluster_info"].samples {
		master := strings.TrimSpace(sample.Metric["master"])
		instance := strings.TrimSpace(sample.Metric["instance"])
		if master != "" {
			masters[master] = true
		}
		if instance != "" {
			reporters[instance] = master
		}
	}
	ptr := func(values map[string]float64, key string) *float64 {
		if v, ok := values[key]; ok {
			return &v
		}
		return nil
	}
	flag := func(v bool) *bool { return &v }
	// 卡片指标按样本的 cluster_id 标签汇总，覆盖范围外的集群。
	// Card metrics aggregate by the sample's cluster_id label, so clusters outside the scope are covered too.
	maxHeap := map[uint]float64{}
	downByCluster := map[uint]int{}
	sampleCluster := func(sample promquery.Sample) uint {
		id, _ := strconv.ParseUint(sample.Metric["cluster_id"], 10, 64)
		return uint(id)
	}
	for _, sample := range byKey["heap_all"].samples {
		if id := sampleCluster(sample); sample.Value > maxHeap[id] {
			maxHeap[id] = sample.Value
		}
	}
	for _, sample := range byKey["up_now"].samples {
		if sample.Value < 1 {
			downByCluster[sampleCluster(sample)]++
		}
	}
	signals := &ClusterSignals{}
	portCount := map[string]int{}
	for i := range out.Nodes {
		_, port, err := net.SplitHostPort(out.Nodes[i].Instance)
		if err == nil {
			portCount[port]++
		}
	}
	for i := range out.Nodes {
		row := &out.Nodes[i]
		if row.Instance == "" {
			continue
		}
		row.HeapPercent, row.CPUCores = ptr(heap, row.Instance), ptr(cpu, row.Instance)
		row.Threads, row.FDPercent, row.UptimeSeconds = ptr(threads, row.Instance), ptr(fd, row.Instance), ptr(uptime, row.Instance)
		if len(masters) > 0 {
			row.Master = flag(instanceMatchesMaster(row.Instance, masters, reporters, portCount))
		}
		if v, ok := up[row.Instance]; ok {
			row.ScrapeUp = flag(v >= 1)
			if v < 1 {
				signals.EndpointsDown++
			}
		}
		if v, ok := state[row.Instance]; ok {
			row.NodeActive = flag(v == 1)
		}
		if row.FDPercent != nil && (signals.FDPercentMax == nil || *row.FDPercent > *signals.FDPercentMax) {
			v := *row.FDPercent
			signals.FDPercentMax = &v
		}
	}
	for i := range out.Clusters {
		card := &out.Clusters[i]
		if v, ok := maxHeap[card.ID]; ok && v > 0 {
			card.MaxHeapPercent = &v
		}
		down := downByCluster[card.ID]
		card.EndpointsDown = &down
	}
	for _, sample := range byKey["cluster_safe"].samples {
		signals.ClustersTotal++
		if sample.Value >= 1 {
			signals.ClustersSafe++
		}
	}
	first := func(key string) *float64 {
		if samples := byKey[key].samples; len(samples) > 0 {
			v := samples[0].Value
			return &v
		}
		return nil
	}
	signals.DeadlockedThreads, signals.PoolRejectRate, signals.HZQueueUtilMax = first("deadlocked"), first("reject"), first("hz_util")
	out.Signals = signals
}

// instanceMatchesMaster 判断抓取 instance 是否对应 cluster_info 的 master 标签。
// Hazelcast 常把 master 标成 127.0.0.1:port，而 HTTP SD / Prometheus instance 是主机 IP:port。
// 共享端口时只把上报 cluster_info 的 instance 标成 master，不再用 inventory 角色兜底。
// instanceMatchesMaster reports whether a scrape instance is the cluster_info master label.
// Hazelcast often labels master as 127.0.0.1:port while the HTTP SD / Prometheus instance is hostIP:port.
// When ports are shared, only the instance that reported cluster_info is master; inventory role is not a fallback.
func instanceMatchesMaster(instance string, masters map[string]bool, reporters map[string]string, portCount map[string]int) bool {
	if masters[instance] {
		return true
	}
	if _, ok := reporters[instance]; ok {
		return true
	}
	for master := range masters {
		if scrapeInstanceMatchesMaster(instance, master, reporters[instance], portCount) {
			return true
		}
	}
	return false
}

func scrapeInstanceMatchesMaster(instance, master, reporter string, portCount map[string]int) bool {
	if instance == master {
		return true
	}
	if reporter != "" && instance == reporter {
		return true
	}
	instanceHost, instancePort, err := net.SplitHostPort(instance)
	if err != nil {
		return false
	}
	masterHost, masterPort, err := net.SplitHostPort(master)
	if err != nil {
		return false
	}
	if instancePort != masterPort {
		return false
	}
	if sameDashboardHost(instanceHost, masterHost) {
		return true
	}
	if !isLoopbackHost(masterHost) && !isLoopbackHost(instanceHost) {
		return false
	}
	return portCount[instancePort] <= 1
}

func sameDashboardHost(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if strings.EqualFold(left, right) {
		return true
	}
	leftIP, rightIP := net.ParseIP(left), net.ParseIP(right)
	return leftIP != nil && rightIP != nil && leftIP.Equal(rightIP)
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// mergeAvailability 合成节点可用性：抓取正常（up≥1）且节点非下线（node_state 缺失或为 1）记 1，否则 0。
// mergeAvailability derives node availability: 1 when the scrape is up (up≥1) and the node is not down (node_state
// missing or 1), otherwise 0.
func mergeAvailability(upSeries, stateSeries []promquery.Series) []DashboardSeries {
	states := map[string]map[float64]float64{}
	for _, item := range stateSeries {
		byTs := map[float64]float64{}
		for _, point := range item.Points {
			byTs[point[0]] = point[1]
		}
		states[item.Metric["instance"]] = byTs
	}
	out := make([]DashboardSeries, 0, len(upSeries))
	for _, item := range upSeries {
		instance := item.Metric["instance"]
		points := make([][2]float64, 0, len(item.Points))
		for _, point := range item.Points {
			value := 0.0
			if point[1] >= 1 {
				value = 1
				if ns, ok := states[instance][point[0]]; ok && ns != 1 {
					value = 0
				}
			}
			points = append(points, [2]float64{point[0], value})
		}
		out = append(out, DashboardSeries{Key: "availability", Label: instance, Points: points})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// ClusterDashboardHandler 处理集群大盘接口。
// ClusterDashboardHandler serves the cluster dashboard endpoint.
type ClusterDashboardHandler struct {
	service *ClusterDashboardService
}

// NewClusterDashboardHandler 创建集群大盘处理器。
// NewClusterDashboardHandler creates the cluster dashboard handler.
func NewClusterDashboardHandler(service *ClusterDashboardService) *ClusterDashboardHandler {
	return &ClusterDashboardHandler{service: service}
}

// GetClusterDashboard 返回首页集群大盘。
// GetClusterDashboard returns the home cluster dashboard.
// @Summary 首页集群大盘（集群卡片 / 节点 / 趋势 / 异常信号）
// @Tags Dashboard
// @Produce json
// @Param cluster_id query int false "集群 ID，空表示全部集群"
// @Param range query string false "时间范围：15m|1h|6h|24h|7d" default(1h)
// @Param start query string false "自定义起点：RFC3339 或秒 / 毫秒时间戳，优先于 range"
// @Param end query string false "自定义终点，缺省为当前时间；跨度不超过 31 天"
// @Success 200 {object} DashboardDataResponse{data=ClusterDashboard}
// @Failure 400 {object} DashboardDataResponse
// @Router /api/v1/dashboard/home/clusters [get]
func (h *ClusterDashboardHandler) GetClusterDashboard(c *gin.Context) {
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
	data, err := h.service.GetClusterDashboard(c.Request.Context(), clusterID, c.Query("range"), window)
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
