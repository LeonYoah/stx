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

	"github.com/LeonYoah/stx/internal/apps/auth"
	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// ErrInvalidVolumeQuery 表示读写量接口参数非法。
// ErrInvalidVolumeQuery reports an invalid volume query parameter.
var ErrInvalidVolumeQuery = errors.New("dashboard: invalid volume query")

// volumeMetrics 是参与读写量统计的四条计数器：读 / 写 × 行 / 字节。
// volumeMetrics lists the four counters behind read/write volume: read/write × rows/bytes.
var volumeMetrics = []struct {
	key     string
	counter string
}{
	{"read_rows", "stx_job_source_received_rows_total"},
	{"write_rows", "stx_job_sink_write_rows_total"},
	{"read_bytes", "stx_job_source_received_bytes_total"},
	{"write_bytes", "stx_job_sink_write_bytes_total"},
}

// volumeSplits 是允许的拆分维度；空串表示不拆分（全局聚合一条序列）。
// volumeSplits lists the allowed split dimensions; the empty string means no split
// (one globally aggregated series).
var volumeSplits = map[string]string{
	"none":    "",
	"cluster": "cluster_id",
	"task":    "task_id",
}

// volumeGranularities 是允许的桶粒度；空串 / auto 表示按窗口跨度自动选择。
// volumeGranularities lists the allowed bucket granularities; empty/auto derives one from the span.
var volumeGranularities = map[string]bool{
	"auto":         true,
	"quarter_hour": true,
	"hour":         true,
	"day":          true,
	"month":        true,
}

// volumeGranularityFor 按窗口跨度自动挑桶粒度：跨度越短桶越细，与其他面板按窗口选 step 同理；
// 短窗用刻钟保证还有几根柱子可看，长窗回到自然日 / 月。
// volumeGranularityFor picks the bucket granularity from the window span — shorter spans get finer
// buckets, the same way the other panels pick their step. Short windows use quarter-hours so the
// chart still has a few bars; long windows fall back to calendar days / months.
func volumeGranularityFor(span time.Duration) string {
	switch {
	case span <= 6*time.Hour:
		return "quarter_hour"
	case span <= 72*time.Hour:
		return "hour"
	case span <= 62*24*time.Hour:
		return "day"
	default:
		return "month"
	}
}

// VolumeTaskNames 按当前用户可见范围解析任务名（sync 服务实现），用于 split=task 的图例；
// 无权限的任务不出名，前端回退为「#id」。
// VolumeTaskNames resolves task names scoped to the actor's visibility (implemented by the sync
// service) for split=task legends; tasks the actor cannot view stay unnamed and the frontend
// falls back to "#id".
type VolumeTaskNames interface {
	ResolveTaskNamesForActor(ctx context.Context, actor executionapp.Actor, taskIDs []uint) map[uint]string
}

// VolumeClusterSource 提供集群列表（cluster 服务实现），用于 split=cluster 的图例。
// VolumeClusterSource lists clusters (implemented by the cluster service) for split=cluster legends.
type VolumeClusterSource interface {
	List(ctx context.Context, filter *clusterapp.ClusterFilter) ([]*clusterapp.Cluster, int64, error)
}

// VolumeCounts 是一个时间窗内四个方向的总量。
// VolumeCounts holds totals for the four directions within one window.
type VolumeCounts struct {
	ReadRows   float64 `json:"read_rows"`
	WriteRows  float64 `json:"write_rows"`
	ReadBytes  float64 `json:"read_bytes"`
	WriteBytes float64 `json:"write_bytes"`
}

// VolumeTotals 是指标卡用的三组总量：当前窗口 / 今日 / 本月；起止毫秒按显示时区。
// VolumeTotals feeds the KPI card: window / today / month totals; the boundary
// timestamps are in the display timezone.
type VolumeTotals struct {
	Window       VolumeCounts `json:"window"`
	Today        VolumeCounts `json:"today"`
	Month        VolumeCounts `json:"month"`
	TodayStartMs int64        `json:"today_start_ms"`
	MonthStartMs int64        `json:"month_start_ms"`
}

// VolumeBucket 是一个日 / 月桶；partial=true 表示桶被时间窗裁掉了一头。
// VolumeBucket is one day/month bucket; partial=true means the window clipped one side of it.
type VolumeBucket struct {
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
	Label   string `json:"label"`
	Partial bool   `json:"partial"`
}

// VolumeSeries 是「指标 × 分组」的一条桶值序列，Totals 与 Buckets 按下标对齐；
// split=none 时 GroupID 固定为 "0"（全部）。
// VolumeSeries is one metric × group sequence; Totals aligns with Buckets by index.
// With split=none GroupID is always "0" (all).
type VolumeSeries struct {
	Metric  string    `json:"metric"`
	GroupID string    `json:"group_id"`
	Totals  []float64 `json:"totals"`
}

// HomeVolume 是 GET /api/v1/dashboard/home/volume 的返回。聚合为全局口径。
// HomeVolume is the payload of GET /api/v1/dashboard/home/volume. Aggregates are global.
type HomeVolume struct {
	Source         string            `json:"source"`
	Reason         string            `json:"reason,omitempty"`
	Range          string            `json:"range"`
	StartMs        int64             `json:"start_ms"`
	EndMs          int64             `json:"end_ms"`
	Granularity    string            `json:"granularity,omitempty"`
	Split          string            `json:"split"`
	OldestSampleMs int64             `json:"oldest_sample_ms"`
	Totals         *VolumeTotals     `json:"totals"`
	Buckets        []VolumeBucket    `json:"buckets"`
	Series         []VolumeSeries    `json:"series"`
	Groups         map[string]string `json:"groups,omitempty"`
	PartialErrors  []string          `json:"partial_errors,omitempty"`
}

// volumeSpan 是一次待执行的 increase 即时查询：key 标识归属（window/today/month 或桶序号），
// start/end 为求值区间。
// volumeSpan is one instant increase query to run: key identifies its owner
// (window/today/month or a bucket index), start/end the evaluated range.
type volumeSpan struct {
	key    string
	bucket int // -1 表示属于 totals / -1 means it belongs to totals
	start  time.Time
	end    time.Time
}

// volumeQueryConcurrency 限制打到内嵌 PromQL 的并发查询数：日 / 月桶多时不加限制的话
// 一次请求就是上百条并发即时查询，足以拖垮引擎。
// volumeQueryConcurrency bounds concurrent queries against the embedded PromQL: with many
// day/month buckets an unbounded request fires hundreds of instant queries, enough to stall
// the engine.
const volumeQueryConcurrency = 4

// volumeMaxCustomSpan 放开读写量接口的自定义窗口上限：月桶只在 >62 天的窗口生效，
// 默认 31 天上限让 month 粒度永远不可达；400 天覆盖一整年加季度对比。
// volumeMaxCustomSpan loosens the custom-window cap for the volume endpoint: monthly
// buckets only engage beyond 62 days, so the default 31-day cap made month unreachable;
// 400 days covers a full year plus quarter comparisons.
const volumeMaxCustomSpan = 400 * 24 * time.Hour

// volumeEqualWidths 是等宽桶粒度 → 固定桶宽；这些粒度的满桶可合并成一条 range 查询。
// volumeEqualWidths maps equal-width bucket granularities to their fixed width — full
// buckets of these merge into a single range query per metric.
var volumeEqualWidths = map[string]time.Duration{
	"quarter_hour": 15 * time.Minute,
	"hour":         time.Hour,
}

// VolumeService 生成首页读写量数据（指标卡总量 + 日 / 月柱状图）。
// VolumeService builds the home data-volume payload (KPI totals + the day/month bar chart).
type VolumeService struct {
	tasks        VolumeTaskNames
	clusters     VolumeClusterSource
	now          func() time.Time
	instantQuery instantQueryFunc
	rangeQuery   rangeQueryFunc
	oldestSample func() (time.Time, bool)
}

// NewVolumeService 创建读写量服务。
// NewVolumeService creates the volume service.
func NewVolumeService(tasks VolumeTaskNames, clusters VolumeClusterSource) *VolumeService {
	return &VolumeService{
		tasks:        tasks,
		clusters:     clusters,
		now:          time.Now,
		instantQuery: dashboardPromCache.wrapInstant(promquery.Instant),
		rangeQuery:   dashboardPromCache.wrapRange(promquery.Range),
		oldestSample: embedts.OldestSampleTime,
	}
}

// GetVolume 生成读写量数据。只有时间窗非法时返回错误；单个桶 / 总量失败记入 PartialErrors，
// 其余桶照常返回（与大盘其它接口的降级策略一致）。
// GetVolume builds the volume payload. Only an invalid window errors out; a failed bucket or
// totals block lands in PartialErrors while the rest still returns (same degradation policy as
// the other home dashboards).
func (s *VolumeService) GetVolume(ctx context.Context, actor executionapp.Actor, clusterID uint, rangeKey string, custom *promquery.Window, granularity, split string) (*HomeVolume, error) {
	now := s.now().UTC()
	rangeName, start, end, err := promquery.ResolveWindow(rangeKey, custom, alignPresetNow(now, custom))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVolumeQuery, err)
	}
	out := &HomeVolume{
		Range: rangeName, StartMs: start.UnixMilli(), EndMs: end.UnixMilli(),
		Granularity: granularity, Split: split,
		Buckets: []VolumeBucket{}, Series: []VolumeSeries{},
	}
	if oldest, ok := s.oldestSample(); ok {
		out.OldestSampleMs = oldest.UnixMilli()
	}
	baseURL, reason := promquery.BaseURL()
	if baseURL == "" {
		out.Source, out.Reason = promquery.SourceNone, reason
		return out, nil
	}
	out.Source = promquery.SourcePrometheus

	loc := timex.DisplayLocation()
	// 「今日 / 本月」按真实当前时间计算，不跟随自定义时间窗的 end：
	// 否则查历史窗口时指标卡会谎报成那天 / 那月的量。求值时刻再按缓存粒度取整
	// （15s）——毫秒级 now 让每个请求的缓存键都不同，「本月」这种扫 31 天的
	// 重查询会永远 miss。
	// "today"/"month" are computed off real now rather than the custom window's end —
	// otherwise a historical window would misreport them as that day/month. The
	// evaluation instant is then truncated to the cache granularity (15s): a
	// millisecond-precision now gives every request a distinct key and the
	// month-long scan would never hit the cache.
	alignedNow := alignPresetNow(now, nil)
	totalsSpans := []volumeSpan{
		{key: "window", bucket: -1, start: start, end: end},
		{key: "today", bucket: -1, start: floorLocal(now, loc, false), end: alignedNow},
		{key: "month", bucket: -1, start: floorLocal(now, loc, true), end: alignedNow},
	}
	out.Totals = &VolumeTotals{
		TodayStartMs: totalsSpans[1].start.UnixMilli(),
		MonthStartMs: totalsSpans[2].start.UnixMilli(),
	}

	sel := ""
	if clusterID > 0 {
		sel = fmt.Sprintf(`cluster_id=%s`, strconv.Quote(strconv.FormatUint(uint64(clusterID), 10)))
	}
	splitBy := volumeSplits[split]

	// rangeTask 是等宽满桶合并后的一条 range 查询：每个求值点落在一个满桶的 EndMs 上，
	// increase[桶宽] 恰好覆盖该桶；endToBucket 把求值点映射回桶号。
	// A rangeTask is one merged range query for equal-width full buckets: each evaluation
	// lands on a full bucket's EndMs so increase[width] covers exactly that bucket;
	// endToBucket maps evaluation points back to bucket indices.
	type rangeTask struct {
		metric      int
		split       string
		query       string
		start, end  time.Time
		step        int
		endToBucket map[int64]int
	}

	instantSpans := append([]volumeSpan{}, totalsSpans...)
	rangeTasks := []*rangeTask{}
	if granularity != "" {
		if granularity == "auto" {
			granularity = volumeGranularityFor(end.Sub(start))
		}
		out.Granularity = granularity
		buckets := volumeBuckets(start, end, loc, granularity)
		out.Buckets = buckets
		fullWidth := volumeEqualWidths[granularity]
		fullIdx := []int{}
		for i, b := range buckets {
			span := volumeSpan{key: strconv.Itoa(i), bucket: i, start: time.UnixMilli(b.StartMs), end: time.UnixMilli(b.EndMs)}
			if fullWidth > 0 && b.EndMs-b.StartMs == fullWidth.Milliseconds() {
				fullIdx = append(fullIdx, i)
			} else {
				instantSpans = append(instantSpans, span)
			}
		}
		// 只有一个满桶时 range 查询没意义，退回即时查询。/ A lone full bucket gains
		// nothing from a range query — fall back to an instant one.
		if len(fullIdx) > 1 {
			endToBucket := map[int64]int{}
			for _, i := range fullIdx {
				endToBucket[buckets[i].EndMs] = i
			}
			rStart := time.UnixMilli(buckets[fullIdx[0]].EndMs)
			rEnd := time.UnixMilli(buckets[fullIdx[len(fullIdx)-1]].EndMs)
			for m, metric := range volumeMetrics {
				rangeTasks = append(rangeTasks, &rangeTask{
					metric: m, split: splitBy,
					query: volumeQuery(metric.counter, sel, splitBy, fullWidth),
					start: rStart, end: rEnd,
					step:        int(fullWidth.Seconds()),
					endToBucket: endToBucket,
				})
			}
		} else {
			for _, i := range fullIdx {
				b := buckets[i]
				instantSpans = append(instantSpans, volumeSpan{key: strconv.Itoa(i), bucket: i, start: time.UnixMilli(b.StartMs), end: time.UnixMilli(b.EndMs)})
			}
		}
	}

	// 即时任务：totals（不拆分）+ 残缺 / 变宽桶；与 range 任务共用 4 路并发额度。
	// Instant tasks: totals (unsplit) + clipped/variable-width buckets; shares the
	// 4-way concurrency budget with the range tasks.
	type instantTask struct {
		span   volumeSpan
		metric int
		query  string
		split  string
	}
	instantTasks := make([]*instantTask, 0, len(instantSpans)*len(volumeMetrics))
	for _, span := range instantSpans {
		for m, metric := range volumeMetrics {
			by := splitBy
			if span.bucket < 0 {
				by = ""
			}
			instantTasks = append(instantTasks, &instantTask{span: span, metric: m, split: by,
				query: volumeQuery(metric.counter, sel, by, span.end.Sub(span.start))})
		}
	}
	type instantResult struct {
		samples []promquery.Sample
		err     error
	}
	type rangeResult struct {
		series []promquery.Series
		err    error
	}
	iResults := make([]instantResult, len(instantTasks))
	rResults := make([]rangeResult, len(rangeTasks))
	var wg gosync.WaitGroup
	sem := make(chan struct{}, volumeQueryConcurrency)
	for i, task := range instantTasks {
		wg.Add(1)
		go func(i int, task *instantTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			iResults[i].samples, iResults[i].err = s.instantQuery(ctx, baseURL, task.query, task.span.end)
		}(i, task)
	}
	for i, task := range rangeTasks {
		wg.Add(1)
		go func(i int, task *rangeTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rResults[i].series, rResults[i].err = s.rangeQuery(ctx, baseURL, task.query, task.start, task.end, task.step)
		}(i, task)
	}
	wg.Wait()

	// 汇总 totals：sum 无分组 ⇒ 单样本，容错地把全部样本相加。
	// Assemble totals: sum without grouping yields one sample; sum all samples defensively.
	totalsPtr := map[string]*VolumeCounts{"window": &out.Totals.Window, "today": &out.Totals.Today, "month": &out.Totals.Month}
	failedTotals := map[string]bool{}
	// 桶值：bucketIdx → metric → groupID → value。/ Bucket values: bucketIdx → metric → groupID → value.
	bucketValues := map[int]map[string]map[string]float64{}
	groupSet := map[string]bool{}
	bucketValue := func(bi int, metric string) map[string]float64 {
		values := bucketValues[bi]
		if values == nil {
			values = map[string]map[string]float64{}
			bucketValues[bi] = values
		}
		if values[metric] == nil {
			values[metric] = map[string]float64{}
		}
		return values[metric]
	}
	groupID := func(metric map[string]string, split string) string {
		gid := "0"
		if split != "" {
			if g := strings.TrimSpace(metric[split]); g != "" {
				gid = g
			}
		}
		groupSet[gid] = true
		return gid
	}
	for i, task := range instantTasks {
		res := iResults[i]
		metric := volumeMetrics[task.metric].key
		if res.err != nil {
			if task.span.bucket < 0 {
				failedTotals[task.span.key] = true
			} else {
				out.PartialErrors = append(out.PartialErrors, "bucket_"+task.span.key)
			}
			continue
		}
		if task.span.bucket < 0 {
			for _, sample := range res.samples {
				addVolumeCount(totalsPtr[task.span.key], metric, sample.Value)
			}
			continue
		}
		for _, sample := range res.samples {
			bucketValue(task.span.bucket, metric)[groupID(sample.Metric, task.split)] += sample.Value
		}
	}
	// range 任务结果：series 上的拆分标签定 group，point[0] 的 EndMs 定桶号。
	// Range results: the series' split label picks the group, a point's EndMs picks the bucket.
	for i, task := range rangeTasks {
		res := rResults[i]
		metric := volumeMetrics[task.metric].key
		if res.err != nil {
			out.PartialErrors = append(out.PartialErrors, "buckets_"+metric)
			continue
		}
		for _, serie := range res.series {
			gid := groupID(serie.Metric, task.split)
			for _, pt := range serie.Points {
				if bi, ok := task.endToBucket[int64(pt[0])]; ok {
					bucketValue(bi, metric)[gid] += pt[1]
				}
			}
		}
	}
	if len(failedTotals) > 0 {
		out.PartialErrors = append(out.PartialErrors, "totals")
	}
	if len(out.Buckets) > 0 {
		out.Series, out.Groups = s.buildSeries(ctx, actor, out.Buckets, bucketValues, groupSet, split)
	}
	return out, nil
}

// buildSeries 把桶值组装成「指标 × 分组」序列并解析分组名称；没有样本的分组不补零序列（图上就是空）。
// buildSeries assembles bucket values into metric × group series and resolves group names; groups
// without any samples stay absent (they simply have no bar).
func (s *VolumeService) buildSeries(ctx context.Context, actor executionapp.Actor, buckets []VolumeBucket, values map[int]map[string]map[string]float64, groupSet map[string]bool, split string) ([]VolumeSeries, map[string]string) {
	groups := make([]string, 0, len(groupSet))
	for gid := range groupSet {
		groups = append(groups, gid)
	}
	sort.Strings(groups)
	series := []VolumeSeries{}
	for _, metric := range volumeMetrics {
		for _, gid := range groups {
			row := VolumeSeries{Metric: metric.key, GroupID: gid, Totals: make([]float64, len(buckets))}
			for i := range buckets {
				row.Totals[i] = values[i][metric.key][gid]
			}
			series = append(series, row)
		}
	}
	return series, s.resolveGroupNames(ctx, actor, groups, split)
}

// resolveGroupNames 按拆分维度解析分组显示名：cluster 走集群列表，task 走任务名查询；
// 解析不到的分组不下发名称，前端回退为「#id」。
// resolveGroupNames resolves display names per split dimension: clusters via the cluster list,
// tasks via the task-name lookup; unresolved groups are omitted and the frontend falls back to "#id".
func (s *VolumeService) resolveGroupNames(ctx context.Context, actor executionapp.Actor, groups []string, split string) map[string]string {
	names := map[string]string{}
	switch split {
	case "cluster":
		if s.clusters == nil {
			return names
		}
		items, _, err := s.clusters.List(ctx, &clusterapp.ClusterFilter{Page: 1, PageSize: 1000})
		if err != nil {
			return names
		}
		byID := map[uint]string{}
		for _, item := range items {
			if item != nil {
				byID[item.ID] = item.Name
			}
		}
		for _, gid := range groups {
			if id, err := strconv.ParseUint(gid, 10, 64); err == nil {
				if name := strings.TrimSpace(byID[uint(id)]); name != "" {
					names[gid] = name
				}
			}
		}
	case "task":
		if s.tasks == nil {
			return names
		}
		ids := make([]uint, 0, len(groups))
		for _, gid := range groups {
			if id, err := strconv.ParseUint(gid, 10, 64); err == nil && id > 0 {
				ids = append(ids, uint(id))
			}
		}
		for gid, name := range s.tasks.ResolveTaskNamesForActor(ctx, actor, ids) {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				names[strconv.FormatUint(uint64(gid), 10)] = trimmed
			}
		}
	}
	return names
}

// volumeQuery 拼一条 increase 即时查询；splitBy 为空时全局聚合。
// volumeQuery builds one increase instant query; empty splitBy aggregates globally.
func volumeQuery(counter, sel, splitBy string, span time.Duration) string {
	seconds := int64(span.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	inner := fmt.Sprintf("increase(%s{%s}[%ds])", counter, sel, seconds)
	if splitBy == "" {
		return fmt.Sprintf("sum(%s)", inner)
	}
	return fmt.Sprintf("sum by (%s) (%s)", splitBy, inner)
}

// floorLocal 返回 t 在显示时区的当日零点 / 当月一号零点。
// floorLocal returns t's day/month start in the display timezone.
func floorLocal(t time.Time, loc *time.Location, monthly bool) time.Time {
	l := t.In(loc)
	if monthly {
		return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, loc)
	}
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
}

// volumeFloor 返回 t 所在桶的起点，按显示时区的自然界对齐（刻钟 / 整点 / 日 / 月）。
// volumeFloor returns the start of the bucket containing t, aligned to the display timezone's
// natural boundary (quarter-hour / hour / day / month).
func volumeFloor(t time.Time, loc *time.Location, granularity string) time.Time {
	l := t.In(loc)
	switch granularity {
	case "month":
		return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, loc)
	case "day":
		return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
	case "hour":
		return time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), 0, 0, 0, loc)
	default: // quarter_hour
		return time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute()/15*15, 0, 0, loc)
	}
}

// volumeNext 返回桶的下一个起点：日 / 月走日历加法（跨 DST 仍对），刻钟 / 小时走固定步长
// （DST 中不存在的本地时刻被跳过，相邻桶自然衔接）。
// volumeNext advances a bucket start: days/months use calendar arithmetic (correct across DST);
// quarter-hours/hours use fixed steps (nonexistent local times under DST are skipped and
// neighbouring buckets join up naturally).
func volumeNext(t time.Time, granularity string) time.Time {
	switch granularity {
	case "month":
		return t.AddDate(0, 1, 0)
	case "day":
		return t.AddDate(0, 0, 1)
	case "hour":
		return t.Add(time.Hour)
	default:
		return t.Add(15 * time.Minute)
	}
}

// volumeLabel 是桶的展示标签：亚日粒度带时刻，日取 YYYY-MM-DD，月取 YYYY-MM。
// volumeLabel renders a bucket label: sub-day granularities carry the time, days are YYYY-MM-DD,
// months YYYY-MM.
func volumeLabel(t time.Time, granularity string) string {
	switch granularity {
	case "month":
		return t.Format("2006-01")
	case "day":
		return t.Format("2006-01-02")
	default:
		return t.Format("2006-01-02 15:04")
	}
}

// volumeBuckets 把 [start,end) 按显示时区的自然界切桶；被窗口裁掉一头的桶记 partial。
// volumeBuckets splits [start,end) on display-timezone natural boundaries; buckets clipped by
// the window are marked partial.
func volumeBuckets(start, end time.Time, loc *time.Location, granularity string) []VolumeBucket {
	buckets := []VolumeBucket{}
	cursor := volumeFloor(start, loc, granularity)
	for cursor.Before(end) {
		next := volumeNext(cursor, granularity)
		b := VolumeBucket{Label: volumeLabel(cursor, granularity)}
		// 端点恰好落在界上算完整桶，只有被窗口裁短才记 partial。
		// An endpoint landing exactly on a boundary still makes a complete bucket; only a
		// window-clipped side is partial.
		if cursor.Before(start) {
			b.StartMs = start.UnixMilli()
			b.Partial = true
		} else {
			b.StartMs = cursor.UnixMilli()
		}
		if next.After(end) {
			b.EndMs = end.UnixMilli()
			b.Partial = true
		} else {
			b.EndMs = next.UnixMilli()
		}
		buckets = append(buckets, b)
		cursor = next
	}
	return buckets
}

// addVolumeCount 把一条样本值累加进对应方向的总量。
// addVolumeCount accumulates one sample into the matching direction's total.
func addVolumeCount(counts *VolumeCounts, metric string, value float64) {
	switch metric {
	case "read_rows":
		counts.ReadRows += value
	case "write_rows":
		counts.WriteRows += value
	case "read_bytes":
		counts.ReadBytes += value
	case "write_bytes":
		counts.WriteBytes += value
	}
}

// VolumeHandler 处理读写量接口。
// VolumeHandler serves the data-volume endpoint.
type VolumeHandler struct {
	service *VolumeService
}

// NewVolumeHandler 创建读写量处理器。
// NewVolumeHandler creates the volume handler.
func NewVolumeHandler(service *VolumeService) *VolumeHandler {
	return &VolumeHandler{service: service}
}

// GetVolume 返回首页读写量：指标卡总量（窗口 / 今日 / 本月）与日 / 月桶序列。
// GetVolume returns home read/write volume: KPI totals (window / today / month) and day/month buckets.
// @Summary 首页读写量（窗口 / 今日 / 本月总量 + 日 / 月柱状图序列）
// @Tags Dashboard
// @Produce json
// @Param cluster_id query int false "集群 ID，空表示全部集群"
// @Param range query string false "时间范围：15m|1h|6h|24h|7d" default(1h)
// @Param start query string false "自定义起点：RFC3339 或秒 / 毫秒时间戳，优先于 range"
// @Param end query string false "自定义终点，缺省为当前时间；跨度不超过 400 天（>62 天进入月桶）"
// @Param granularity query string false "桶粒度：auto|quarter_hour|hour|day|month；auto 按窗口跨度选桶，缺省只回 totals 不出 buckets"
// @Param split query string false "拆分维度：none|cluster|task" default(none)
// @Success 200 {object} DashboardDataResponse{data=HomeVolume}
// @Failure 400 {object} DashboardDataResponse
// @Router /api/v1/dashboard/home/volume [get]
func (h *VolumeHandler) GetVolume(c *gin.Context) {
	var clusterID uint
	if raw := strings.TrimSpace(c.Query("cluster_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "invalid cluster_id"})
			return
		}
		clusterID = uint(parsed)
	}
	granularity := strings.ToLower(strings.TrimSpace(c.Query("granularity")))
	if granularity != "" && !volumeGranularities[granularity] {
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "granularity must be auto|quarter_hour|hour|day|month"})
		return
	}
	split := strings.ToLower(strings.TrimSpace(c.DefaultQuery("split", "none")))
	if _, ok := volumeSplits[split]; !ok {
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "split must be none|cluster|task"})
		return
	}
	window, err := promquery.ParseWindowWithSpan(c.Query("start"), c.Query("end"), time.Now(), volumeMaxCustomSpan)
	if err != nil {
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: err.Error()})
		return
	}
	actor := executionapp.Actor{}
	if user := auth.GetUserFromContext(c); user != nil {
		actor = executionapp.Actor{UserID: uint64(user.ID), IsAdmin: user.IsAdmin}
	}
	data, err := h.service.GetVolume(c.Request.Context(), actor, clusterID, c.Query("range"), window, granularity, split)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidVolumeQuery) {
			status = http.StatusBadRequest
		}
		c.JSON(status, DashboardDataResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DashboardDataResponse{Data: data})
}
