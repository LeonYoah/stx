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
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// detailQueryKind 标记面板表达式的取样语义，决定长步长时怎么调整窗口。
// detailQueryKind marks a panel expression's sampling semantics, driving how the window
// adapts to coarse steps.
type detailQueryKind int

const (
	// detailGaugeAvg 是均值语义的仪表面板：步长超过 5m 时包 avg_over_time（默认零值）。
	// detailGaugeAvg is a mean-semantics gauge panel: wrapped in avg_over_time once the step
	// exceeds 5m (the zero value so most queries stay unannotated).
	detailGaugeAvg detailQueryKind = iota
	// detailGaugeMax 是峰值语义的仪表面板（CDC 延迟、队列积压）：步长超过 5m 时包 max_over_time。
	// detailGaugeMax is a peak-semantics gauge panel (CDC delay, queue backlog): wrapped in
	// max_over_time once the step exceeds 5m.
	detailGaugeMax
	// detailRate 是计数器速率：窗口取 max(5m, step)，长步长覆盖整个取样间隔。
	// detailRate is a counter rate: the window becomes max(5m, step) so coarse steps cover
	// the whole sampling interval.
	detailRate
	// detailRaw 原样执行，不做步长适配（如已自带窗口的表达式）。
	// detailRaw runs verbatim with no step adaptation (e.g. expressions with their own windows).
	detailRaw
)

// adaptDetailExpr 按取样语义把表达式适配到给定步长。
// adaptDetailExpr adapts an expression to the given step per its sampling semantics.
func adaptDetailExpr(kind detailQueryKind, expr string, step int) string {
	switch kind {
	case detailRate:
		return strings.ReplaceAll(expr, "[5m]", "["+promquery.RangeWindow(step, 5*time.Minute)+"]")
	case detailGaugeMax:
		return promquery.SampleGauge(step, "max", expr)
	case detailRaw:
		return expr
	default:
		return promquery.SampleGauge(step, "avg", expr)
	}
}

// detailQuery 是面板里的一条查询：dimLabel 非空时序列名取样本该标签（如 pool / gc / type），
// 否则用固定维度名 dim（如 current / rx）。
// detailQuery is one query inside a panel: when dimLabel is set the series name comes from that
// sample label (pool / gc / type…); otherwise the fixed dimension name dim is used (current / rx…).
type detailQuery struct {
	dim      string
	dimLabel string
	kind     detailQueryKind
	expr     string // %[1]s 为公共 selector / %[1]s is the shared selector
}

// detailPanel 是细节分组里的一张图：若干 detailQuery 合并成一个单位一致的面板。
// detailPanel is one chart inside a detail group: several detailQueries merged into one unit-consistent panel.
type detailPanel struct {
	key     string
	unit    string // bytes|percent|count|per_second|ms
	queries []detailQuery
}

// clusterDetailGroups 定义五个细节分组的面板与查询；全部原始表达式，不新增录制规则。
// clusterDetailGroups defines the panels and queries of the five detail groups; all raw expressions,
// no new recording rules.
var clusterDetailGroups = map[string][]detailPanel{
	"jvm": {
		{key: "heap_pools", unit: "bytes", queries: []detailQuery{
			{dimLabel: "pool", expr: `sum by (instance, pool) (jvm_memory_pool_bytes_used{%[1]s, pool=~".*(Eden|Survivor|Old|Tenured).*"})`},
		}},
		{key: "nonheap", unit: "bytes", queries: []detailQuery{
			{dimLabel: "pool", expr: `sum by (instance, pool) (jvm_memory_pool_bytes_used{%[1]s, pool=~"Metaspace|Code.*|Compressed Class Space"})`},
		}},
		{key: "buffer", unit: "bytes", queries: []detailQuery{
			{dimLabel: "pool", expr: `sum by (instance, pool) (jvm_buffer_pool_used_bytes{%[1]s})`},
		}},
		{key: "heap", unit: "bytes", queries: []detailQuery{
			{dim: "used", expr: `sum by (instance) (jvm_memory_bytes_used{%[1]s, area="heap"})`},
			{dim: "committed", expr: `sum by (instance) (jvm_memory_bytes_committed{%[1]s, area="heap"})`},
		}},
	},
	"gc": {
		{key: "gc_rate", unit: "per_second", queries: []detailQuery{
			{dimLabel: "gc", kind: detailRate, expr: `sum by (instance, gc) (rate(jvm_gc_collection_seconds_count{%[1]s}[5m]))`},
		}},
		{key: "gc_pause", unit: "percent", queries: []detailQuery{
			{dimLabel: "gc", kind: detailRate, expr: `100 * sum by (instance, gc) (rate(jvm_gc_collection_seconds_sum{%[1]s}[5m]))`},
		}},
		{key: "threads", unit: "count", queries: []detailQuery{
			{dim: "current", expr: `max by (instance) (jvm_threads_current{%[1]s})`},
			{dim: "daemon", expr: `max by (instance) (jvm_threads_daemon{%[1]s})`},
			{dim: "peak", expr: `max by (instance) (jvm_threads_peak{%[1]s})`},
		}},
		{key: "thread_states", unit: "count", queries: []detailQuery{
			{dimLabel: "state", expr: `sum by (instance, state) (jvm_threads_state{%[1]s, state=~"BLOCKED|WAITING|TIMED_WAITING"})`},
		}},
		{key: "deadlocked", unit: "count", queries: []detailQuery{
			{dim: "deadlocked", expr: `max by (instance) (jvm_threads_deadlocked{%[1]s})`},
			{dim: "monitor", expr: `max by (instance) (jvm_threads_deadlocked_monitor{%[1]s})`},
		}},
	},
	"sched": {
		{key: "pool", unit: "count", queries: []detailQuery{
			{dim: "size", expr: `max by (instance) (job_thread_pool_poolSize{%[1]s})`},
			{dim: "active", expr: `max by (instance) (job_thread_pool_activeCount{%[1]s})`},
			// 队列积压按峰值取样。/ Queued backlog samples peak semantics.
			{dim: "queued", kind: detailGaugeMax, expr: `max by (instance) (job_thread_pool_queueTaskCount{%[1]s})`},
		}},
		{key: "reject", unit: "per_second", queries: []detailQuery{
			{dim: "reject", kind: detailRate, expr: `sum by (instance) (rate(job_thread_pool_rejection_total{%[1]s}[5m]))`},
		}},
		{key: "op_latency", unit: "ms", queries: []detailQuery{
			// 延迟是峰值语义：长窗口保住尖刺。/ Latency is peak semantics: long windows keep spikes.
			{dim: "request_slot", kind: detailGaugeMax, expr: `max by (instance) (request_slot_operation_last_invocation_latency_ms{%[1]s})`},
			{dim: "report_metrics", kind: detailGaugeMax, expr: `max by (instance) (report_metrics_operation_last_invocation_latency_ms{%[1]s})`},
		}},
		{key: "hz_queue", unit: "count", queries: []detailQuery{
			{dimLabel: "type", kind: detailGaugeMax, expr: `sum by (instance, type) (hazelcast_executor_queueSize{%[1]s})`},
		}},
		{key: "partitions", unit: "count", queries: []detailQuery{
			{dim: "total", expr: `max by (instance) (hazelcast_partition_partitionCount{%[1]s})`},
			{dim: "active", expr: `max by (instance) (hazelcast_partition_activePartition{%[1]s})`},
		}},
		// members 面板在 fillDetail 里补参考线：node_count 对照 DB 节点数；序列按 cluster_id
		// 聚合（无 instance 标签），装配时把集群名填进 Instance，否则「全部集群」下全是
		// 「 · members」无法分辨。
		// The members panel gets its reference line in fillDetail: node_count vs the DB node
		// count. Series aggregate by cluster_id (no instance label) — the assembly step fills
		// Instance with the cluster name, otherwise "all clusters" renders rows of
		// " · members" with nothing telling them apart.
		{key: "members", unit: "count", queries: []detailQuery{
			{dim: "members", expr: `max by (cluster_id) (node_count{%[1]s})`},
		}},
	},
	"store": {
		{key: "store_owned", unit: "count", queries: []detailQuery{
			{dimLabel: "store", expr: `sum by (instance, store) (engine_state_store_local_owned_entries{%[1]s})`},
		}},
		{key: "store_backup", unit: "count", queries: []detailQuery{
			{dimLabel: "store", expr: `sum by (instance, store) (engine_state_store_local_backup_entries{%[1]s})`},
		}},
		{key: "store_heap", unit: "bytes", queries: []detailQuery{
			// 只画按节点汇总的总量：13 个 store × 节点数的明细噪音太大，关注的是
			// 状态堆占用是否持续增长。/ Only the per-node total is charted: 13 stores ×
			// nodes of detail is noise — what matters is whether the state heap keeps growing.
			{dim: "total", expr: `sum by (instance) (engine_state_store_local_heap_cost_bytes{%[1]s})`},
		}},
	},
	"proc": {
		{key: "offheap", unit: "bytes", queries: []detailQuery{
			{dim: "rss", expr: `max by (instance) (process_resident_memory_bytes{%[1]s})`},
			// 堆外 = 非堆已用 + direct/mapped buffer（JVM 口径的堆外总量，天然非负）。
			// 不取 RSS − 已提交堆：-Xms 大且未开 AlwaysPreTouch 时，已提交的堆页
			// 未必真正驻留，RSS 会小于 committed 而算出负数。
			// Off-heap = non-heap used + direct/mapped buffers (the JVM-defined
			// off-heap total, inherently non-negative). RSS − committed heap is not
			// used: with a large -Xms and no AlwaysPreTouch, committed heap pages are
			// not yet resident and RSS < committed yields a negative series.
			{dim: "offheap", expr: `sum by (instance) (jvm_memory_bytes_used{%[1]s, area="nonheap"}) + on (instance) sum by (instance) (jvm_buffer_pool_used_bytes{%[1]s})`},
		}},
		{key: "fds", unit: "count", queries: []detailQuery{
			{dim: "open", expr: `max by (instance) (process_open_fds{%[1]s})`},
			{dim: "max", expr: `max by (instance) (process_max_fds{%[1]s})`},
		}},
		{key: "classes", unit: "count", queries: []detailQuery{
			{dim: "loaded", expr: `max by (instance) (jvm_classes_currently_loaded{%[1]s})`},
		}},
		{key: "class_load_rate", unit: "per_second", queries: []detailQuery{
			{dim: "loaded", kind: detailRate, expr: `sum by (instance) (rate(jvm_classes_loaded_total{%[1]s}[5m]))`},
			{dim: "unloaded", kind: detailRate, expr: `sum by (instance) (rate(jvm_classes_unloaded_total{%[1]s}[5m]))`},
		}},
	},
}

// DetailSeries 是细节面板的一条序列：Instance 决定颜色，Label 是维度名（图例）。
// DetailSeries is one series inside a detail panel: Instance picks the color, Label is the legend name.
type DetailSeries struct {
	Instance string       `json:"instance"`
	Label    string       `json:"label"`
	Points   [][2]float64 `json:"points"`
}

// DetailPanel 是细节分组的一张图：单位 + 按节点（可带维度）的序列 + 可选参考线。
// DetailPanel is one chart in a detail group: a unit, per-node (optionally per-dimension) series and
// an optional reference line.
type DetailPanel struct {
	Key       string         `json:"key"`
	Unit      string         `json:"unit"`
	Reference *float64       `json:"reference,omitempty"`
	Series    []DetailSeries `json:"series"`
}

// DetailEvent 是趋势上的事件标注：节点重启（启动时刻前移）或集群成员数变化。
// DetailEvent is a marker on the trends: a node restart (start time moving forward) or a cluster
// membership change.
type DetailEvent struct {
	TsMs  int64  `json:"ts_ms"`
	Kind  string `json:"kind"` // node_restart|members_change
	Label string `json:"label"`
}

// ClusterDetail 是 GET /api/v1/dashboard/home/clusters/detail 的返回。
// ClusterDetail is the payload of GET /api/v1/dashboard/home/clusters/detail.
type ClusterDetail struct {
	Source      string `json:"source"`
	Reason      string `json:"reason,omitempty"`
	Range       string `json:"range"`
	StartMs     int64  `json:"start_ms"`
	EndMs       int64  `json:"end_ms"`
	StepSeconds int    `json:"step_seconds"`
	Group          string        `json:"group"`
	Panels         []DetailPanel `json:"panels"`
	Events         []DetailEvent `json:"events"`
	PartialErrors  []string      `json:"partial_errors,omitempty"`
}

// detailTask 是一条并发执行的面板查询。
// detailTask is one panel query run concurrently.
type detailTask struct {
	panel int
	dim   string
	dimL  string
	query string
	out   []promquery.Series
	err   error
}

// GetClusterDetail 生成一个细节分组的面板数据；只加载请求的那一组（懒加载）。
// GetClusterDetail builds the panels of one detail group; only the requested group is loaded (lazy).
func (s *ClusterDashboardService) GetClusterDetail(ctx context.Context, clusterID uint, rangeKey string, custom *promquery.Window, group string) (*ClusterDetail, error) {
	now := s.now().UTC()
	rangeName, start, end, err := promquery.ResolveWindow(rangeKey, custom, alignPresetNow(now, custom))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidClusterDashboardQuery, err)
	}
	step := promquery.RangeSteps[rangeName]
	if step == 0 {
		step = promquery.AutoStep(end.Sub(start))
	}
	out := &ClusterDetail{
		Range: rangeName, StartMs: start.UnixMilli(), EndMs: end.UnixMilli(),
		StepSeconds: step, Group: group, Panels: []DetailPanel{}, Events: []DetailEvent{},
	}
	if reason := promquery.UnavailableReason(); reason != "" {
		out.Source, out.Reason = promquery.SourceNone, reason
		return out, nil
	}
	out.Source = promquery.SourcePrometheus

	sel := `job="seatunnel_engine_http"`
	if clusterID > 0 {
		sel += fmt.Sprintf(`,cluster_id=%s`, strconv.Quote(strconv.FormatUint(uint64(clusterID), 10)))
	}
	panels := clusterDetailGroups[group]
	tasks := []*detailTask{}
	for p, panel := range panels {
		for _, q := range panel.queries {
			tasks = append(tasks, &detailTask{panel: p, dim: q.dim, dimL: q.dimLabel, query: fmt.Sprintf(adaptDetailExpr(q.kind, q.expr, step), sel)})
		}
	}
	// 事件检测与面板同批并发：启动时刻 / 成员数两条 range 序列，Go 侧差分。
	// Event detection shares the same concurrent batch: the start-time and member-count range
	// series are diffed in Go.
	restartTask := &detailTask{panel: -1, query: fmt.Sprintf(`max by (instance) (process_start_time_seconds{%s})`, sel)}
	memberTask := &detailTask{panel: -2, query: fmt.Sprintf(`max by (cluster_id) (node_count{%s})`, sel)}
	tasks = append(tasks, restartTask, memberTask)

	var wg gosync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task *detailTask) {
			defer wg.Done()
			task.out, task.err = s.rangeQuery(ctx, task.query, start, end, step)
		}(task)
	}
	wg.Wait()

	// cluster_id → 集群名：members 面板的图例与 members_change 事件都要带上集群身份，
	// 否则「全部集群」视图里多条 members 序列同名、事件也说不清是哪个集群。
	// cluster_id → cluster name: both the members-panel legend and members_change events
	// carry cluster identity — otherwise the "all clusters" view shows identically named
	// members series and events that say nothing about which cluster changed.
	clusterNames := map[string]string{}
	if s.clusters != nil {
		if items, _, err := s.clusters.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000}); err == nil {
			for _, item := range items {
				if item != nil {
					clusterNames[strconv.FormatUint(uint64(item.ID), 10)] = item.Name
				}
			}
		}
	}
	clusterName := func(cid string) string {
		if name := strings.TrimSpace(clusterNames[cid]); name != "" {
			return name
		}
		// 找不到名字时回退 #<id>，与读写量图例的 `#3` 风格一致，避免中文界面混入英文。
		// Fall back to #<id> when the name is unknown — same style as the volume legend
		// (`#3`) so a Chinese UI doesn't leak an English "cluster" prefix.
		return "#" + cid
	}

	for i, panel := range panels {
		item := DetailPanel{Key: panel.key, Unit: panel.unit, Series: []DetailSeries{}}
		for _, task := range tasks {
			if task.panel != i {
				continue
			}
			if task.err != nil {
				out.PartialErrors = append(out.PartialErrors, panel.key)
				break
			}
			for _, serie := range task.out {
				label := task.dim
				if task.dimL != "" {
					label = strings.TrimSpace(serie.Metric[task.dimL])
					if label == "" {
						label = task.dimL
					}
				}
				instance := strings.TrimSpace(serie.Metric["instance"])
				// members 序列无 instance（按 cluster_id 聚合）→ 用集群名顶替。
				// members series carry no instance (aggregated by cluster_id) → fill in the
				// cluster name.
				if instance == "" {
					if cid := strings.TrimSpace(serie.Metric["cluster_id"]); cid != "" {
						instance = clusterName(cid)
					}
				}
				item.Series = append(item.Series, DetailSeries{
					Instance: instance,
					Label:    label,
					Points:   serie.Points,
				})
			}
		}
		sort.Slice(item.Series, func(a, b int) bool {
			if item.Series[a].Instance != item.Series[b].Instance {
				return item.Series[a].Instance < item.Series[b].Instance
			}
			return item.Series[a].Label < item.Series[b].Label
		})
		out.Panels = append(out.Panels, item)
	}
	if restartTask.err != nil {
		out.PartialErrors = append(out.PartialErrors, "event_restart")
	}
	if memberTask.err != nil {
		out.PartialErrors = append(out.PartialErrors, "event_members")
	}
	out.Events = detailEvents(restartTask.out, memberTask.out, clusterName)
	// members 面板的参考线 = 该集群 DB 登记的节点数，用于对照 Hazelcast 成员数。
	// The members panel's reference line is the cluster's node count from the DB, to compare
	// against the Hazelcast member count.
	if group == "sched" && s.clusters != nil && clusterID > 0 {
		if nodes, err := s.clusters.GetNodes(ctx, clusterID); err == nil {
			total := float64(len(nodes))
			for i := range out.Panels {
				if out.Panels[i].Key == "members" {
					out.Panels[i].Reference = &total
				}
			}
		}
	}
	return out, nil
}

// detailEvents 对两条 range 序列做差分：启动时刻上升记节点重启，成员数变化记成员变更；
// 成员事件带上集群名前缀（clusterName 为 nil 时退回 "cluster <id>"）。
// detailEvents diffs two range series: a rising start time marks a node restart, a member-count
// change marks a membership change; member events carry the cluster name prefix (a nil
// clusterName falls back to "cluster <id>").
func detailEvents(startSeries, memberSeries []promquery.Series, clusterName func(string) string) []DetailEvent {
	if clusterName == nil {
		clusterName = func(cid string) string { return "cluster " + cid }
	}
	events := []DetailEvent{}
	for _, serie := range startSeries {
		for i := 1; i < len(serie.Points); i++ {
			if serie.Points[i][1] > serie.Points[i-1][1] {
				events = append(events, DetailEvent{
					TsMs: int64(serie.Points[i][0]), Kind: "node_restart",
					Label: strings.TrimSpace(serie.Metric["instance"]),
				})
			}
		}
	}
	for _, serie := range memberSeries {
		for i := 1; i < len(serie.Points); i++ {
			if serie.Points[i][1] != serie.Points[i-1][1] {
				cid := strings.TrimSpace(serie.Metric["cluster_id"])
				events = append(events, DetailEvent{
					TsMs: int64(serie.Points[i][0]), Kind: "members_change",
					Label: fmt.Sprintf("%s: %v → %v", clusterName(cid), serie.Points[i-1][1], serie.Points[i][1]),
				})
			}
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].TsMs < events[j].TsMs })
	return events
}

// ClusterDetailHandler 处理集群细节分组接口。
// ClusterDetailHandler serves the cluster detail endpoint.
type ClusterDetailHandler struct {
	service *ClusterDashboardService
}

// NewClusterDetailHandler 创建集群细节处理器。
// NewClusterDetailHandler creates the cluster detail handler.
func NewClusterDetailHandler(service *ClusterDashboardService) *ClusterDetailHandler {
	return &ClusterDetailHandler{service: service}
}

// GetClusterDetail 返回一个细节分组（懒加载：只查该组的指标）。
// GetClusterDetail returns one detail group (lazy: only that group's metrics are queried).
// @Summary 首页集群细节分组（JVM / GC 线程 / 调度 Hazelcast / 状态存储 / 进程网络）
// @Tags Dashboard
// @Produce json
// @Param cluster_id query int true "集群 ID"
// @Param group query string true "细节分组：jvm|gc|sched|store|proc"
// @Param range query string false "时间范围：15m|1h|6h|24h|7d|30d|90d|1y" default(1h)
// @Param start query string false "自定义起点：RFC3339 或秒 / 毫秒时间戳，优先于 range"
// @Param end query string false "自定义终点，缺省为当前时间；跨度不超过 366 天"
// @Success 200 {object} DashboardDataResponse{data=ClusterDetail}
// @Failure 400 {object} DashboardDataResponse
// @Router /api/v1/dashboard/home/clusters/detail [get]
func (h *ClusterDetailHandler) GetClusterDetail(c *gin.Context) {
	var clusterID uint
	if raw := strings.TrimSpace(c.Query("cluster_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "invalid cluster_id"})
			return
		}
		clusterID = uint(parsed)
	}
	group := strings.ToLower(strings.TrimSpace(c.Query("group")))
	if _, ok := clusterDetailGroups[group]; !ok {
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "group must be jvm|gc|sched|store|proc"})
		return
	}
	window, err := promquery.ParseWindow(c.Query("start"), c.Query("end"), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: err.Error()})
		return
	}
	data, err := h.service.GetClusterDetail(c.Request.Context(), clusterID, c.Query("range"), window, group)
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
