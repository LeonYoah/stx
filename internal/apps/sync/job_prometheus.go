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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// jobMetricsTableCap 是每个作业导出的表级序列上限，超出并入 other。
	// jobMetricsTableCap caps per-job table-level series; overflow rolls into "other".
	jobMetricsTableCap = 50

	// jobMetricsZeroStartWindow 是新作业补零值起点的时间窗：作业开始后这么久内首次被看到才补，
	// 更早开始的作业（STX 重启后接手）不补，避免把重启前的累计量算进当前窗口。
	// jobMetricsZeroStartWindow bounds zero-start injection: only jobs first seen within this
	// long after they started get a zero sample; older jobs (taken over after an STX restart)
	// do not, so pre-restart cumulatives are not counted into the current window.
	jobMetricsZeroStartWindow = 10 * time.Minute
)

// jobPrometheusExporter 持有作业级 Prometheus 指标注册表与上次计数，便于用 Counter 导出引擎累计值。
// jobPrometheusExporter holds the job-level Prometheus registry and last counter values so engine cumulatives can be
// exported as Counters.
type jobPrometheusExporter struct {
	reg *prometheus.Registry

	sourceRows       *prometheus.CounterVec
	sourceBytes      *prometheus.CounterVec
	sinkRows         *prometheus.CounterVec
	sinkBytes        *prometheus.CounterVec
	sinkCommitted    *prometheus.CounterVec
	tableRows        *prometheus.CounterVec
	cdcDelay         *prometheus.GaugeVec
	checkpointDur    *prometheus.GaugeVec
	checkpointFail   *prometheus.CounterVec
	vertexBusy       *prometheus.GaugeVec
	edgeBackpressure *prometheus.GaugeVec
	jobUp            *prometheus.GaugeVec

	mu       sync.Mutex
	lastVals map[string]float64  // key → last absolute counter value
	active   map[string]struct{} // job label key currently exported

	// deferDeletes 为 true 时（tsdb 直写协程在跑），终态作业的序列登记到 pendingDeletes，
	// 等下一轮导出把终值写入 tsdb 后再删；为 false 时立即删除。
	// When deferDeletes is true (the tsdb direct-write loop is running), terminal jobs are
	// queued in pendingDeletes and removed only after the next export has written their final
	// values; when false they are removed immediately.
	deferDeletes   bool
	pendingDeletes map[string]prometheus.Labels // job label key → labels
	// zeroAt 记录需要补零值起点的作业（job_id → 开始时间毫秒），见 jobMetricsZeroStartWindow。
	// zeroAt holds jobs that need a zero-start sample (job_id → start time in ms); see jobMetricsZeroStartWindow.
	zeroAt map[string]int64
}

var (
	jobPromOnce sync.Once
	jobProm     *jobPrometheusExporter
)

func getJobPrometheusExporter() *jobPrometheusExporter {
	jobPromOnce.Do(func() {
		jobProm = newJobPrometheusExporter()
	})
	return jobProm
}

func newJobPrometheusExporter() *jobPrometheusExporter {
	e := &jobPrometheusExporter{
		reg:            prometheus.NewRegistry(),
		lastVals:       make(map[string]float64),
		active:         make(map[string]struct{}),
		pendingDeletes: make(map[string]prometheus.Labels),
		zeroAt:         make(map[string]int64),
	}
	jobLabels := []string{"cluster_id", "task_id", "job_id"}
	e.sourceRows = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stx_job_source_received_rows_total",
		Help: "Cumulative source received rows for a STX sync job",
	}, jobLabels)
	e.sourceBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stx_job_source_received_bytes_total",
		Help: "Cumulative source received bytes for a STX sync job",
	}, jobLabels)
	e.sinkRows = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stx_job_sink_write_rows_total",
		Help: "Cumulative sink write rows for a STX sync job",
	}, jobLabels)
	e.sinkBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stx_job_sink_write_bytes_total",
		Help: "Cumulative sink write bytes for a STX sync job",
	}, jobLabels)
	e.sinkCommitted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stx_job_sink_committed_rows_total",
		Help: "Cumulative sink committed rows for a STX sync job",
	}, jobLabels)
	e.tableRows = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stx_job_table_rows_total",
		Help: "Cumulative per-table rows (direction=read|write) for a STX sync job",
	}, append(append([]string{}, jobLabels...), "direction", "table"))
	e.cdcDelay = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stx_job_cdc_delay_seconds",
		Help: "Latest CDC emit delay in seconds for a STX sync job",
	}, jobLabels)
	e.checkpointDur = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stx_job_checkpoint_duration_seconds",
		Help: "Latest completed checkpoint duration in seconds",
	}, jobLabels)
	e.checkpointFail = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stx_job_checkpoint_failed_total",
		Help: "Cumulative failed checkpoints observed by STX for a sync job",
	}, jobLabels)
	e.vertexBusy = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stx_job_vertex_busy_ratio",
		Help: "Latest vertex busy ratio (0-1) for a STX sync job",
	}, append(append([]string{}, jobLabels...), "vertex"))
	e.edgeBackpressure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stx_job_edge_backpressure_ratio",
		Help: "Latest edge backpressure ratio (0-1) for a STX sync job",
	}, append(append([]string{}, jobLabels...), "edge"))
	e.jobUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stx_job_up",
		Help: "1 when the STX sync job is running (labeled by status)",
	}, append(append([]string{}, jobLabels...), "status"))

	e.reg.MustRegister(
		e.sourceRows, e.sourceBytes, e.sinkRows, e.sinkBytes, e.sinkCommitted,
		e.tableRows, e.cdcDelay, e.checkpointDur, e.checkpointFail,
		e.vertexBusy, e.edgeBackpressure, e.jobUp,
	)
	return e
}

func jobPromLabels(instance *JobInstance) (clusterID, taskID, jobID string) {
	if instance == nil {
		return "0", "0", "0"
	}
	// cluster_id 优先取提交快照；没有时用 0（采集器有上下文时会再补）。
	// Prefer cluster_id from the submit snapshot; fall back to 0 (collectors with context can refine later).
	cid := uintValue(instance.SubmitSpec, "cluster_id")
	return strconv.FormatUint(uint64(cid), 10),
		strconv.FormatUint(uint64(instance.TaskID), 10),
		strconv.FormatUint(uint64(instance.ID), 10)
}

func (e *jobPrometheusExporter) setCounterAbs(vec *prometheus.CounterVec, labelValues []string, abs float64, key string) {
	if abs < 0 {
		abs = 0
	}
	e.mu.Lock()
	prev, ok := e.lastVals[key]
	e.lastVals[key] = abs
	e.mu.Unlock()
	delta := abs
	if ok {
		if abs >= prev {
			delta = abs - prev
		} else {
			// 累计值回绕（不应在同一 job_id 出现）：重新建立。
			// Counter went backwards (should not happen for the same job_id): re-seed.
			delta = abs
		}
	} else {
		delta = abs
	}
	if delta > 0 {
		vec.WithLabelValues(labelValues...).Add(delta)
	} else {
		// Ensure the series exists even when the absolute value is still 0.
		// 绝对值为 0 时也要露出序列。
		vec.WithLabelValues(labelValues...)
	}
}

// observeJobMetrics 用对账得到的 metrics 更新导出；仅在 STX observability 开启时写入。
// 终态作业先写入这次对账带来的终值，再删除序列（直写协程在跑时延迟到下一轮导出之后），
// 保证最终累计量进入 tsdb。
// observeJobMetrics updates export series from reconciler metrics; it writes only when STX
// observability is on. For terminal jobs it first records the final values carried by this
// reconcile, then removes the series (deferred past the next export while the direct-write
// loop runs) so the final cumulatives reach the tsdb.
func (e *jobPrometheusExporter) observeJobMetrics(instance *JobInstance, metrics map[string]interface{}) {
	if e == nil || instance == nil || config.Config == nil || !config.Config.Observability.Enabled {
		return
	}
	if isFinalNormalizedJobStatus(instance.Status) {
		if metrics != nil {
			e.applyJobMetrics(instance, metrics, false)
		}
		e.deleteJob(instance)
		return
	}
	e.applyJobMetrics(instance, metrics, true)
}

// applyJobMetrics 把对账 metrics 写进 Registry；updateUp 控制是否刷新 stx_job_up 的状态标签
// （终态时不改，序列随后整体删除）。
// applyJobMetrics writes reconcile metrics into the Registry; updateUp controls whether the
// stx_job_up status label is refreshed (left alone for terminal jobs, whose series are removed next).
func (e *jobPrometheusExporter) applyJobMetrics(instance *JobInstance, metrics map[string]interface{}, updateUp bool) {
	clusterID, taskID, jobID := jobPromLabels(instance)
	base := []string{clusterID, taskID, jobID}
	jobKey := clusterID + "|" + taskID + "|" + jobID

	e.mu.Lock()
	if _, seen := e.active[jobKey]; !seen {
		e.markZeroStartLocked(jobID, instance.StartedAt)
	}
	e.active[jobKey] = struct{}{}
	e.mu.Unlock()

	if v := metricNumber(metrics, "SourceReceivedCount"); v != nil {
		e.setCounterAbs(e.sourceRows, base, *v, jobKey+"|src_rows")
	}
	if v := metricNumber(metrics, "SourceReceivedBytes"); v != nil {
		e.setCounterAbs(e.sourceBytes, base, *v, jobKey+"|src_bytes")
	}
	if v := metricNumber(metrics, "SinkWriteCount"); v != nil {
		e.setCounterAbs(e.sinkRows, base, *v, jobKey+"|sink_rows")
	}
	if v := metricNumber(metrics, "SinkWriteBytes"); v != nil {
		e.setCounterAbs(e.sinkBytes, base, *v, jobKey+"|sink_bytes")
	}
	if v := metricNumber(metrics, "SinkCommittedCount"); v != nil {
		e.setCounterAbs(e.sinkCommitted, base, *v, jobKey+"|sink_committed")
	}

	e.observeTableRows(base, jobKey, metrics, singleLinkMetricTablesForJob(instance, metrics))

	if !updateUp {
		return
	}
	status := strings.ToLower(string(instance.Status))
	if status == "" {
		status = "unknown"
	}
	e.jobUp.DeletePartialMatch(prometheus.Labels{
		"cluster_id": clusterID, "task_id": taskID, "job_id": jobID,
	})
	e.jobUp.WithLabelValues(clusterID, taskID, jobID, status).Set(1)
}

// markZeroStartLocked 在作业首次被观察时判断是否需要补零值起点；调用方持有 e.mu。
// markZeroStartLocked decides on first observation whether a job needs a zero-start sample; the caller holds e.mu.
func (e *jobPrometheusExporter) markZeroStartLocked(jobID string, startedAt *time.Time) {
	if startedAt == nil || startedAt.IsZero() {
		return
	}
	age := time.Since(*startedAt)
	if age < 0 || age > jobMetricsZeroStartWindow {
		return
	}
	e.zeroAt[jobID] = startedAt.UnixMilli()
}

// zeroStartFor 返回作业的零值起点（毫秒）；没有登记时 ok=false。
// zeroStartFor returns a job's zero-start time in ms; ok is false when none is recorded.
func (e *jobPrometheusExporter) zeroStartFor(jobID string) (int64, bool) {
	if e == nil || jobID == "" {
		return 0, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	ts, ok := e.zeroAt[jobID]
	return ts, ok
}

// enableDeferredDeletes 由 tsdb 直写协程启动时调用：此后终态作业延迟到下一轮导出之后再删。
// enableDeferredDeletes is called when the tsdb direct-write loop starts: from then on terminal
// jobs are removed only after the next export.
func (e *jobPrometheusExporter) enableDeferredDeletes() {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.deferDeletes = true
	e.mu.Unlock()
}

// takePendingDeletes 取走并清空当前登记的待删作业。
// takePendingDeletes takes and clears the currently queued job deletions.
func (e *jobPrometheusExporter) takePendingDeletes() map[string]prometheus.Labels {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.pendingDeletes) == 0 {
		return nil
	}
	batch := e.pendingDeletes
	e.pendingDeletes = make(map[string]prometheus.Labels)
	return batch
}

// applyPendingDeletes 真正删除一批待删作业的序列。
// applyPendingDeletes actually removes the series of a batch of queued jobs.
func (e *jobPrometheusExporter) applyPendingDeletes(batch map[string]prometheus.Labels) {
	for prefix, labels := range batch {
		e.removeJobSeries(prefix, labels)
	}
}

func (e *jobPrometheusExporter) observeTableRows(base []string, jobKey string, metrics map[string]interface{}, alignment singleLinkMetricTables) {
	type scored struct {
		name              string
		read              float64
		write             float64
		committedFallback float64
		hasRead           bool
		hasWrite          bool
		hasCommitted      bool
	}
	rowsByName := map[string]*scored{}
	rowFor := func(name string) *scored {
		row := rowsByName[name]
		if row == nil {
			row = &scored{name: name}
			rowsByName[name] = row
		}
		return row
	}
	for _, key := range collectTableMetricKeysFrom(metrics, "TableSourceReceivedCount") {
		name := key
		if alignment.sourceTable != "" {
			name = alignment.sourceTable
		}
		row := rowFor(name)
		row.hasRead = true
		if v := tableMetricNumber(metrics, "TableSourceReceivedCount", key); v != nil {
			row.read += *v
		}
	}
	for _, key := range collectTableMetricKeysFrom(metrics, "TableSinkWriteCount", "TableSinkCommittedCount") {
		name := key
		if alignment.sinkTable != "" {
			name = alignment.sinkTable
		}
		row := rowFor(name)
		if v := tableMetricNumber(metrics, "TableSinkWriteCount", key); v != nil {
			row.write += *v
			row.hasWrite = true
		}
		if v := tableMetricNumber(metrics, "TableSinkCommittedCount", key); v != nil {
			row.committedFallback += *v
			row.hasCommitted = true
		}
	}
	// 单链路命中时与前端 dag-model.ts 保持一致：若作业级总量高于归并后的表级计数，抬高该单表下限。
	// Match frontend dag-model.ts on single-link jobs: raise zero or lower table counters to job-level totals.
	if alignment.sourceTable != "" && alignment.sinkTable != "" {
		if v := metricNumber(metrics, "SourceReceivedCount"); v != nil && *v > 0 {
			row := rowFor(alignment.sourceTable)
			row.hasRead = true
			if *v > row.read {
				row.read = *v
			}
		}
		if v := metricNumber(metrics, "SinkWriteCount"); v != nil && *v > 0 {
			row := rowFor(alignment.sinkTable)
			row.hasWrite = true
			if *v > row.write {
				row.write = *v
			}
		}
		if v := metricNumber(metrics, "SinkCommittedCount"); v != nil && *v > 0 {
			row := rowFor(alignment.sinkTable)
			row.hasCommitted = true
			if *v > row.committedFallback {
				row.committedFallback = *v
			}
		}
	}
	if len(rowsByName) == 0 {
		return
	}
	rows := make([]scored, 0, len(rowsByName))
	for _, row := range rowsByName {
		if row.hasCommitted && (!row.hasWrite || row.committedFallback > row.write) {
			row.write = row.committedFallback
			row.hasWrite = true
		}
		rows = append(rows, *row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		si := rows[i].read + rows[i].write
		sj := rows[j].read + rows[j].write
		if si == sj {
			return rows[i].name < rows[j].name
		}
		return si > sj
	})
	var otherRead, otherWrite float64
	var hasOtherRead, hasOtherWrite bool
	for i, row := range rows {
		table := row.name
		read, write := row.read, row.write
		if i >= jobMetricsTableCap {
			if row.hasRead || read > 0 {
				otherRead += read
				hasOtherRead = true
			}
			if row.hasWrite || write > 0 {
				otherWrite += write
				hasOtherWrite = true
			}
			continue
		}
		if row.hasRead || read > 0 {
			e.setCounterAbs(e.tableRows, append(append([]string{}, base...), "read", table), read, jobKey+"|tbl|read|"+table)
		}
		if row.hasWrite || write > 0 {
			e.setCounterAbs(e.tableRows, append(append([]string{}, base...), "write", table), write, jobKey+"|tbl|write|"+table)
		}
	}
	if hasOtherRead || otherRead > 0 {
		e.setCounterAbs(e.tableRows, append(append([]string{}, base...), "read", "other"), otherRead, jobKey+"|tbl|read|other")
	}
	if hasOtherWrite || otherWrite > 0 {
		e.setCounterAbs(e.tableRows, append(append([]string{}, base...), "write", "other"), otherWrite, jobKey+"|tbl|write|other")
	}
}

type singleLinkMetricTables struct {
	sourceTable string
	sinkTable   string
}

func isConcreteSingleTablePath(tablePath string) bool {
	trimmed := strings.TrimSpace(tablePath)
	if trimmed == "" || strings.EqualFold(trimmed, "default.default.default") {
		return false
	}
	if strings.Contains(trimmed, "${") || strings.ContainsAny(trimmed, "*?[]|") {
		return false
	}
	return true
}

func normalizeUnwrappedMetricLeaf(rawKey string) string {
	trimmed := strings.TrimSpace(rawKey)
	if trimmed == "" {
		return ""
	}
	path := strings.TrimSpace(metricTablePathRe.ReplaceAllString(trimmed, ""))
	if len(path) > len("Optional[]") && strings.EqualFold(path[:len("Optional[")], "Optional[") && strings.HasSuffix(path, "]") {
		path = strings.TrimSpace(path[len("Optional[") : len(path)-1])
	}
	if path == "" || strings.EqualFold(path, "default.default.default") {
		return ""
	}
	leaf := path
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		leaf = path[idx+1:]
	}
	leaf = strings.ToLower(strings.TrimSpace(leaf))
	return strings.TrimPrefix(leaf, "archive_")
}

func metricSideHasMultipleTables(metrics map[string]interface{}, mapKeys ...string) bool {
	prefixedPaths := map[string]struct{}{}
	leafNames := map[string]struct{}{}
	for _, rawKey := range collectTableMetricKeysFrom(metrics, mapKeys...) {
		trimmed := strings.TrimSpace(rawKey)
		if loc := metricTablePathRe.FindStringIndex(trimmed); loc != nil {
			path := strings.ToLower(strings.TrimSpace(trimmed[loc[1]:]))
			if path != "" && path != "default.default.default" {
				prefixedPaths[path] = struct{}{}
				if len(prefixedPaths) > 1 {
					return true
				}
			}
		}
		if leaf := normalizeUnwrappedMetricLeaf(trimmed); leaf != "" {
			leafNames[leaf] = struct{}{}
			if len(leafNames) > 1 {
				return true
			}
		}
	}
	return false
}

func metricsHasMultipleTables(metrics map[string]interface{}) bool {
	if len(metrics) == 0 {
		return false
	}
	return metricSideHasMultipleTables(metrics, "TableSourceReceivedCount", "TableSourceReceivedBytes", "TableSourceReceivedQPS") ||
		metricSideHasMultipleTables(metrics, "TableSinkWriteCount", "TableSinkWriteBytes", "TableSinkWriteQPS", "TableSinkCommittedCount", "TableSinkCommittedBytes")
}

func singleLinkMetricTablesForJob(instance *JobInstance, metrics map[string]interface{}) singleLinkMetricTables {
	var result singleLinkMetricTables
	if instance == nil || metricsHasMultipleTables(metrics) {
		return result
	}
	dag := mapValue(instance.ResultPreview, "job_dag")
	if dag == nil {
		return result
	}
	vertices := make([]map[string]any, 0)
	visit := func(raw any) {
		if entry, ok := raw.(map[string]any); ok {
			vertices = append(vertices, entry)
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
	sourceTable := singleMetricTablePath(vertices, "source")
	sinkTable := singleMetricTablePath(vertices, "sink")
	if sourceTable == "" || sinkTable == "" {
		return singleLinkMetricTables{}
	}
	return singleLinkMetricTables{sourceTable: sourceTable, sinkTable: sinkTable}
}

func singleMetricTablePath(vertices []map[string]any, vertexType string) string {
	matches := make([]map[string]any, 0, 1)
	for _, vertex := range vertices {
		typ, _ := vertex["type"].(string)
		if strings.EqualFold(strings.TrimSpace(typ), vertexType) {
			matches = append(matches, vertex)
		}
	}
	if len(matches) != 1 {
		return ""
	}
	paths := metricVertexTablePaths(matches[0])
	if len(paths) != 1 || !isConcreteSingleTablePath(paths[0]) {
		return ""
	}
	return paths[0]
}

func metricVertexTablePaths(vertex map[string]any) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	add := func(raw any) {
		value, ok := raw.(string)
		if !ok {
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		paths = append(paths, value)
	}
	switch rawPaths := vertex["tablePaths"].(type) {
	case []any:
		for _, raw := range rawPaths {
			add(raw)
		}
	case []string:
		for _, raw := range rawPaths {
			add(raw)
		}
	}
	return paths
}

// observeCDCDelay 写入 CDC 延迟 gauge（秒）。
// observeCDCDelay sets the CDC delay gauge in seconds.
func (e *jobPrometheusExporter) observeCDCDelay(instance *JobInstance, delaySeconds float64) {
	if e == nil || instance == nil || config.Config == nil || !config.Config.Observability.Enabled {
		return
	}
	if isFinalNormalizedJobStatus(instance.Status) {
		return
	}
	clusterID, taskID, jobID := jobPromLabels(instance)
	e.cdcDelay.WithLabelValues(clusterID, taskID, jobID).Set(delaySeconds)
}

// observeCheckpoint 写入最近一次完成 Checkpoint 耗时，并按失败次数增量累加失败计数。
// observeCheckpoint sets the latest completed checkpoint duration and increments the failed counter by newly seen failures.
func (e *jobPrometheusExporter) observeCheckpoint(instance *JobInstance, durationSeconds float64, failedTotal float64) {
	if e == nil || instance == nil || config.Config == nil || !config.Config.Observability.Enabled {
		return
	}
	clusterID, taskID, jobID := jobPromLabels(instance)
	base := []string{clusterID, taskID, jobID}
	jobKey := clusterID + "|" + taskID + "|" + jobID
	if durationSeconds >= 0 {
		e.checkpointDur.WithLabelValues(base...).Set(durationSeconds)
	}
	e.setCounterAbs(e.checkpointFail, base, failedTotal, jobKey+"|cp_fail")
}

// observeRealtime 写入算子忙碌度与边背压的最新点。
// observeRealtime sets the latest vertex busy and edge backpressure gauges.
func (e *jobPrometheusExporter) observeRealtime(instance *JobInstance, vertices []RealtimeVertex, edges []RealtimeEdge) {
	if e == nil || instance == nil || config.Config == nil || !config.Config.Observability.Enabled {
		return
	}
	clusterID, taskID, jobID := jobPromLabels(instance)
	for _, vertex := range vertices {
		if len(vertex.Points) == 0 {
			continue
		}
		point := vertex.Points[len(vertex.Points)-1]
		name := strings.TrimSpace(vertex.Name)
		if name == "" {
			name = "vertex-" + strconv.FormatInt(vertex.VertexID, 10)
		}
		e.vertexBusy.WithLabelValues(clusterID, taskID, jobID, name).Set(vertexBusyRatio(vertex.Type, point))
	}
	for _, edge := range edges {
		if len(edge.Points) == 0 {
			continue
		}
		point := edge.Points[len(edge.Points)-1]
		name := "queue-" + strconv.FormatInt(edge.QueueID, 10) + "→" + strconv.FormatInt(edge.TargetVertexID, 10)
		e.edgeBackpressure.WithLabelValues(clusterID, taskID, jobID, name).Set(realtimeFloat(point, "bpRatio"))
	}
}

// deleteJob 移除某作业的全部导出序列（终态时调用）；直写协程在跑时只登记，下一轮导出后再删。
// deleteJob removes every exported series for one job (called on terminal status); while the
// direct-write loop runs it only queues the job and the removal happens after the next export.
func (e *jobPrometheusExporter) deleteJob(instance *JobInstance) {
	if e == nil || instance == nil {
		return
	}
	clusterID, taskID, jobID := jobPromLabels(instance)
	labels := prometheus.Labels{"cluster_id": clusterID, "task_id": taskID, "job_id": jobID}
	prefix := clusterID + "|" + taskID + "|" + jobID
	e.mu.Lock()
	if e.deferDeletes {
		e.pendingDeletes[prefix] = labels
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()
	e.removeJobSeries(prefix, labels)
}

// removeJobSeries 删除某作业在 Registry 中的全部序列与内部状态。
// removeJobSeries drops every Registry series and internal state of one job.
func (e *jobPrometheusExporter) removeJobSeries(prefix string, labels prometheus.Labels) {
	e.sourceRows.DeletePartialMatch(labels)
	e.sourceBytes.DeletePartialMatch(labels)
	e.sinkRows.DeletePartialMatch(labels)
	e.sinkBytes.DeletePartialMatch(labels)
	e.sinkCommitted.DeletePartialMatch(labels)
	e.tableRows.DeletePartialMatch(labels)
	e.cdcDelay.DeletePartialMatch(labels)
	e.checkpointDur.DeletePartialMatch(labels)
	e.checkpointFail.DeletePartialMatch(labels)
	e.vertexBusy.DeletePartialMatch(labels)
	e.edgeBackpressure.DeletePartialMatch(labels)
	e.jobUp.DeletePartialMatch(labels)

	e.mu.Lock()
	delete(e.active, prefix)
	delete(e.zeroAt, labels["job_id"])
	for key := range e.lastVals {
		if strings.HasPrefix(key, prefix) {
			delete(e.lastVals, key)
		}
	}
	e.mu.Unlock()
}

// jobHasEngineObservability 根据 submit_spec.observability 判断该作业是否开启了引擎可观测性。
// jobHasEngineObservability reports whether the job enabled engine observability from submit_spec.observability.
func jobHasEngineObservability(instance *JobInstance) bool {
	if instance == nil || instance.SubmitSpec == nil {
		return false
	}
	raw, ok := instance.SubmitSpec["observability"]
	if !ok || raw == nil {
		return false
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		// JSONMap may decode nested maps as JSONMap / map[string]any already handled above.
		if jm, ok := raw.(JSONMap); ok {
			m = jm
		} else {
			return false
		}
	}
	if mapBool(m, "injected") {
		return true
	}
	reason := strings.TrimSpace(stringValue(m, "reason"))
	switch reason {
	case observabilityReasonUserConfigured, observabilityReasonAlreadyInjected, observabilityReasonInjected:
		return true
	default:
		return false
	}
}

// mapBool 从 map 中读布尔值，缺省或类型不对时返回 false。
// mapBool reads a bool from a map; missing or wrong types yield false.
func mapBool(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}
