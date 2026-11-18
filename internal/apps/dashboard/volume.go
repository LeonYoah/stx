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
	"container/list"
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
	"week":         true,
	"month":        true,
}

// volumeGranularityFor 按窗口跨度自动挑桶粒度：跨度越短桶越细，与其他面板按窗口选 step 同理；
// 短窗用刻钟保证还有几根柱子可看，长窗逐级退到自然日 / 周 / 月。
// volumeGranularityFor picks the bucket granularity from the window span — shorter spans get finer
// buckets, the same way the other panels pick their step. Short windows use quarter-hours so the
// chart still has a few bars; long windows fall back stepwise to calendar days / weeks / months.
func volumeGranularityFor(span time.Duration) string {
	switch {
	case span <= 6*time.Hour:
		return "quarter_hour"
	case span <= 72*time.Hour:
		return "hour"
	case span <= 31*24*time.Hour:
		return "day"
	case span <= 182*24*time.Hour:
		return "week"
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

// VolumeBucket 是一个日 / 周 / 月桶；partial=true 表示桶被时间窗或最早样本裁掉了一头，
// no_data=true 表示整桶早于最早样本（数值恒为 0）。
// VolumeBucket is one day/week/month bucket; partial=true means the window or the oldest sample
// clipped one side of it, and no_data=true means the whole bucket predates the oldest sample
// (its values are always 0).
type VolumeBucket struct {
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
	Label   string `json:"label"`
	Partial bool   `json:"partial"`
	NoData  bool   `json:"no_data,omitempty"`
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

// volumeSpan 是一次待执行的 increase 查询：key 标识归属（window/today/month 或桶序号），
// span 为区间秒数，at 为求值时刻。
// volumeSpan is one increase query to run: key identifies its owner (window/today/month or a
// bucket index), span is the range length in seconds, at the evaluation time.
type volumeSpan struct {
	key    string
	bucket int // -1 表示属于 totals / -1 means it belongs to totals
	start  time.Time
	end    time.Time
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
	dayCache     *volumeDayCache
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
		dayCache:     newVolumeDayCache(volumeDayCacheCap),
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
	if reason := promquery.UnavailableReason(); reason != "" {
		out.Source, out.Reason = promquery.SourceNone, reason
		return out, nil
	}
	out.Source = promquery.SourcePrometheus

	loc := timex.DisplayLocation()
	if granularity == "auto" {
		granularity = volumeGranularityFor(end.Sub(start))
	}
	out.Granularity = granularity
	if granularity != "" {
		out.Buckets = volumeBuckets(start, end, loc, granularity)
		markVolumeNoData(out.Buckets, out.OldestSampleMs)
	}
	sel := ""
	if clusterID > 0 {
		sel = fmt.Sprintf(`cluster_id=%s`, strconv.Quote(strconv.FormatUint(uint64(clusterID), 10)))
	}
	splitBy := volumeSplits[split]

	out.Totals = &VolumeTotals{
		TodayStartMs: floorLocal(end, loc, false).UnixMilli(),
		MonthStartMs: floorLocal(end, loc, true).UnixMilli(),
	}
	bucketValues := map[int]map[string]map[string]float64{}
	groupSet := map[string]bool{}

	// 亚日粒度窗口最多 72h，保持逐桶即时查询；日 / 周 / 月以及仅看总量时走逐日增量 + 范围查询。
	// Sub-day granularity windows are at most 72h and keep per-bucket instant queries; day/week/month
	// granularity (and totals-only calls) go through daily increments plus range queries.
	if granularity == "quarter_hour" || granularity == "hour" {
		s.fillSubDayVolume(ctx, out, start, end, loc, sel, splitBy, bucketValues, groupSet)
	} else {
		s.fillDailyVolume(ctx, out, start, end, loc, sel, splitBy, bucketValues, groupSet)
	}
	if len(out.Buckets) > 0 {
		out.Series, out.Groups = s.buildSeries(ctx, actor, out.Buckets, bucketValues, groupSet, split)
	}
	return out, nil
}

// fillSubDayVolume 亚日粒度的旧路径：每个桶一条 increase 即时查询，窗口 / 今日 / 本月三组总量
// 单独发（超过一天的区间按日拆开，避免单条 increase 加载过多样本）。
// fillSubDayVolume is the legacy path for sub-day granularities: one increase instant query per
// bucket, plus separate window/today/month totals spans (any span longer than a day is split into
// day chunks so a single increase never loads too many samples).
func (s *VolumeService) fillSubDayVolume(ctx context.Context, out *HomeVolume, start, end time.Time, loc *time.Location, sel, splitBy string, bucketValues map[int]map[string]map[string]float64, groupSet map[string]bool) {
	// 桶切分 + totals 三个临时桶合并成统一的 span 列表，一次性并发查询。
	// Buckets plus the three ad-hoc totals spans collapse into one span list queried concurrently.
	spans := []volumeSpan{
		{key: "window", bucket: -1, start: start, end: end},
		{key: "today", bucket: -1, start: time.UnixMilli(out.Totals.TodayStartMs), end: end},
		{key: "month", bucket: -1, start: time.UnixMilli(out.Totals.MonthStartMs), end: end},
	}
	for i, b := range out.Buckets {
		spans = append(spans, volumeSpan{key: strconv.Itoa(i), bucket: i, start: time.UnixMilli(b.StartMs), end: time.UnixMilli(b.EndMs)})
	}

	// 每个 span × 每个指标一条 increase 即时查询；totals 恒为全局聚合（不拆分）。
	// One increase instant query per span × metric; totals always aggregate globally (no split).
	type taskRef struct {
		span    volumeSpan
		metric  int
		counter string
		split   string
	}
	tasks := make([]*taskRef, 0, len(spans)*len(volumeMetrics))
	for _, span := range spans {
		for m, metric := range volumeMetrics {
			by := splitBy
			if span.bucket < 0 {
				by = ""
			}
			tasks = append(tasks, &taskRef{span: span, metric: m, counter: metric.counter, split: by})
		}
	}
	type taskResult struct {
		samples []promquery.Sample
		err     error
	}
	results := make([]taskResult, len(tasks))
	var wg gosync.WaitGroup
	for i, task := range tasks {
		wg.Add(1)
		go func(i int, task *taskRef) {
			defer wg.Done()
			results[i].samples, results[i].err = s.queryVolumeIncrease(ctx, task.counter, sel, task.split, task.span.start, task.span.end, loc)
		}(i, task)
	}
	wg.Wait()

	// 汇总 totals：sum 无分组 ⇒ 单样本，容错地把全部样本相加。
	// Assemble totals: sum without grouping yields one sample; sum all samples defensively.
	totalsPtr := map[string]*VolumeCounts{"window": &out.Totals.Window, "today": &out.Totals.Today, "month": &out.Totals.Month}
	failedTotals := map[string]bool{}
	for i, task := range tasks {
		res := results[i]
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
		addVolumeSamples(bucketValues, groupSet, task.span.bucket, metric, task.split, res.samples)
	}
	if len(failedTotals) > 0 {
		out.PartialErrors = append(out.PartialErrors, "totals")
	}
}

// addVolumeSamples 把一条桶查询的样本按分组累加进 bucketValues。
// addVolumeSamples accumulates one bucket query's samples into bucketValues by group.
func addVolumeSamples(bucketValues map[int]map[string]map[string]float64, groupSet map[string]bool, bucket int, metric, splitBy string, samples []promquery.Sample) {
	values := bucketValues[bucket]
	if values == nil {
		values = map[string]map[string]float64{}
		bucketValues[bucket] = values
	}
	if values[metric] == nil {
		values[metric] = map[string]float64{}
	}
	for _, sample := range samples {
		gid := "0"
		if splitBy != "" {
			gid = strings.TrimSpace(sample.Metric[splitBy])
			if gid == "" {
				gid = "0"
			}
		}
		groupSet[gid] = true
		values[metric][gid] += sample.Value
	}
}

// volumeDayMs 是一天的毫秒数；恰好等于这个值的完整日才能走定步长范围查询（DST 日不是 24h）。
// volumeDayMs is one day in milliseconds; only complete days of exactly this length may use the
// fixed-step range query (DST days are not 24h).
const volumeDayMs = int64(24 * 60 * 60 * 1000)

// volumeDayCacheCap 是逐日增量 LRU 的上限；键为 counter+selector+split+日零点。
// volumeDayCacheCap caps the daily-increment LRU; keys are counter+selector+split+day-midnight.
const volumeDayCacheCap = 4096

// volumeDayCache 是逐日增量结果的进程内 LRU：过去的完整日不再变化，长期缓存，重复打开首页
// 或换粒度时不用重查。今天的日段永不写入。
// volumeDayCache is an in-process LRU of per-day increment results: past complete days never
// change, so they are cached long-term and re-opening the home page or switching granularity
// does not re-query them. Segments covering today are never written.
type volumeDayCache struct {
	mu    gosync.Mutex
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

// volumeDayCacheEntry 是缓存里一天（或一天内一个分组）的聚合样本。
// volumeDayCacheEntry is one cached day's aggregated samples (per split group).
type volumeDayCacheEntry struct {
	key     string
	samples []promquery.Sample
}

func newVolumeDayCache(capacity int) *volumeDayCache {
	return &volumeDayCache{cap: capacity, ll: list.New(), items: map[string]*list.Element{}}
}

// get 返回缓存的日增量；命中时把条目移到队首。
// get returns a cached day increment; hits move the entry to the front.
func (c *volumeDayCache) get(key string) ([]promquery.Sample, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(volumeDayCacheEntry).samples, true
}

// put 写入日增量；超上限时淘汰最久未用的条目。
// put stores a day increment; the least recently used entry is evicted past the cap.
func (c *volumeDayCache) put(key string, samples []promquery.Sample) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value = volumeDayCacheEntry{key: key, samples: samples}
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(volumeDayCacheEntry{key: key, samples: samples})
	for len(c.items) > c.cap {
		back := c.ll.Back()
		if back == nil {
			break
		}
		delete(c.items, back.Value.(volumeDayCacheEntry).key)
		c.ll.Remove(back)
	}
}

// volumeDaySeg 是一段日粒度增量：窗口内的每个自然日（首末可能被裁剪成不足一天）对应一段，
// samples 为按拆分维度聚合后的值。
// volumeDaySeg is one day-granularity increment: each calendar day inside the window (the two
// edges may be clipped to less than a day) maps to one segment whose samples are aggregated per
// split group.
type volumeDaySeg struct {
	startMs    int64
	endMs      int64
	dayStartMs int64 // 所属自然日的零点 / local midnight of the owning day
	cacheKey   string
	instant    bool
	ranged     bool
	samples    []promquery.Sample
	err        error
}

// volumeDayCacheKey 组装日增量缓存键。
// volumeDayCacheKey builds the daily-increment cache key.
func volumeDayCacheKey(counter, sel, splitBy string, dayStartMs int64) string {
	return strings.Join([]string{counter, sel, splitBy, strconv.FormatInt(dayStartMs, 10)}, "|")
}

// queryDailySegments 把 [start,end) 切成逐日段并查询增量：
//   - 完整且恰为 86400s 的日段：先查 LRU，缺失的连续区间合并成一条
//     `sum by (<split>) (increase(<counter>[1d]))` 范围查询（step=86400，逐点即逐日）；
//   - 被窗口裁剪的首尾段、以及 DST 下不足 / 超过 24h 的完整日：各发一条即时查询；
//   - 已结束于「今天」之前的完整日写入缓存，今天永不缓存。
//
// queryDailySegments splits [start,end) into per-day segments and queries their increments:
//   - complete days of exactly 86400s hit the LRU first; missing contiguous runs collapse into
//     one `sum by (<split>) (increase(<counter>[1d]))` range query (step=86400, one point per day);
//   - window-clipped edge segments and complete days that are not 24h under DST each get one
//     instant query;
//   - complete days ending before today are written to the cache; today is never cached.
func (s *VolumeService) queryDailySegments(ctx context.Context, counter, sel, splitBy string, start, end time.Time, loc *time.Location, todayStartMs int64) []volumeDaySeg {
	days := volumeBuckets(start, end, loc, "day")
	segs := make([]volumeDaySeg, len(days))
	for i, b := range days {
		seg := &segs[i]
		seg.startMs, seg.endMs = b.StartMs, b.EndMs
		seg.dayStartMs = volumeFloor(time.UnixMilli(b.StartMs), loc, "day").UnixMilli()
		complete := !b.Partial
		cacheable := complete && b.EndMs <= todayStartMs
		if cacheable {
			seg.cacheKey = volumeDayCacheKey(counter, sel, splitBy, seg.dayStartMs)
			if hit, ok := s.dayCache.get(seg.cacheKey); ok {
				seg.samples = hit
				continue
			}
		}
		if complete && b.EndMs-b.StartMs == volumeDayMs {
			seg.ranged = true
		} else {
			seg.instant = true
		}
	}

	type segTask struct {
		i, j    int // 范围任务覆盖 segs[i..j]（含）；即时任务只用 i。/ Range tasks cover segs[i..j] inclusive; instant tasks use only i.
		instant bool
	}
	tasks := []segTask{}
	for i := 0; i < len(segs); i++ {
		switch {
		case segs[i].instant:
			tasks = append(tasks, segTask{i: i, instant: true})
		case segs[i].ranged:
			j := i
			for j+1 < len(segs) && segs[j+1].ranged {
				j++
			}
			tasks = append(tasks, segTask{i: i, j: j})
			i = j
		}
	}
	byEnd := make(map[int64]int, len(segs))
	for i := range segs {
		byEnd[segs[i].endMs] = i
	}
	var wg gosync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task segTask) {
			defer wg.Done()
			if task.instant {
				seg := &segs[task.i]
				span := time.Duration(seg.endMs-seg.startMs) * time.Millisecond
				seg.samples, seg.err = s.instantQuery(ctx, volumeQuery(counter, sel, splitBy, span), time.UnixMilli(seg.endMs))
				if seg.err == nil && seg.cacheKey != "" {
					s.dayCache.put(seg.cacheKey, seg.samples)
				}
				return
			}
			// 连续完整日一段范围查询：求值点落在每个零点，点 t 即 (t-1d, t] 的日增量。
			// One range query per contiguous run of complete days: evaluation lands on each
			// midnight and point t is the increment of (t-1d, t].
			q := volumeQuery(counter, sel, splitBy, 24*time.Hour)
			series, err := s.rangeQuery(ctx, q, time.UnixMilli(segs[task.i].endMs), time.UnixMilli(segs[task.j].endMs), int(24*time.Hour/time.Second))
			if err != nil {
				for k := task.i; k <= task.j; k++ {
					segs[k].err = err
				}
				return
			}
			for _, sr := range series {
				for _, pt := range sr.Points {
					idx, ok := byEnd[int64(pt[0])]
					if !ok {
						continue
					}
					segs[idx].samples = append(segs[idx].samples, promquery.Sample{Metric: sr.Metric, TsMs: int64(pt[0]), Value: pt[1]})
				}
			}
			for k := task.i; k <= task.j; k++ {
				if segs[k].cacheKey != "" {
					s.dayCache.put(segs[k].cacheKey, segs[k].samples)
				}
			}
		}(task)
	}
	wg.Wait()
	return segs
}

// fillDailyVolume 逐日增量路径：窗口 / 今日 / 本月总量与所有桶全部由逐日段累加而来，
// 长窗口下每指标最多 3 条查询（首段即时 + 连续完整日一条范围 + 尾段即时）。
// fillDailyVolume is the daily-increment path: window/today/month totals and every bucket are all
// accumulated from per-day segments, costing at most 3 queries per metric on long windows
// (head instant + one range query over contiguous complete days + tail instant).
func (s *VolumeService) fillDailyVolume(ctx context.Context, out *HomeVolume, start, end time.Time, loc *time.Location, sel, splitBy string, bucketValues map[int]map[string]map[string]float64, groupSet map[string]bool) {
	todayStartMs := out.Totals.TodayStartMs
	monthStartMs := out.Totals.MonthStartMs
	// 今日 / 本月总量可能落在窗口之外（自定义窗口在过去时）：把查询下界扩到最早的界，
	// 窗口外的日段只按全局口径（不拆分）查询并只用于 totals。
	// Today/month totals may fall outside the window (custom windows in the past): widen the query
	// lower bound to the earliest boundary; out-of-window segments are queried unsplit and feed
	// totals only.
	unionStart := start
	if ms := time.UnixMilli(monthStartMs); ms.Before(unionStart) {
		unionStart = ms
	}

	type metricDays struct {
		pre, win []volumeDaySeg
	}
	perMetric := make([]metricDays, len(volumeMetrics))
	var wg gosync.WaitGroup
	for m, metric := range volumeMetrics {
		wg.Add(1)
		go func(m int, counter string) {
			defer wg.Done()
			if unionStart.Before(start) {
				perMetric[m].pre = s.queryDailySegments(ctx, counter, sel, "", unionStart, start, loc, todayStartMs)
			}
			perMetric[m].win = s.queryDailySegments(ctx, counter, sel, splitBy, start, end, loc, todayStartMs)
		}(m, metric.counter)
	}
	wg.Wait()

	startMs := start.UnixMilli()
	failedTotals := false
	failedBuckets := map[int]bool{}
	bucketEnds := make([]int64, len(out.Buckets))
	for i, b := range out.Buckets {
		bucketEnds[i] = b.EndMs
	}
	for m, metric := range volumeMetrics {
		key := metric.key
		for _, segs := range [][]volumeDaySeg{perMetric[m].pre, perMetric[m].win} {
			for _, seg := range segs {
				if seg.err != nil {
					failedTotals = true
					if seg.startMs >= startMs && len(bucketEnds) > 0 {
						if idx := sort.Search(len(bucketEnds), func(i int) bool { return bucketEnds[i] > seg.startMs }); idx < len(bucketEnds) {
							failedBuckets[idx] = true
						}
					}
					continue
				}
				// 逐日段按所属自然日判定归属：段起日零点 ≥ 今日 / 本月界即计入对应总量。
				// Segments belong by their owning calendar day: a day starting at or after the
				// today/month boundary counts toward that total.
				if seg.dayStartMs >= monthStartMs {
					for _, sample := range seg.samples {
						addVolumeCount(&out.Totals.Month, key, sample.Value)
					}
				}
				if seg.dayStartMs >= todayStartMs {
					for _, sample := range seg.samples {
						addVolumeCount(&out.Totals.Today, key, sample.Value)
					}
				}
				if seg.startMs < startMs {
					continue
				}
				for _, sample := range seg.samples {
					addVolumeCount(&out.Totals.Window, key, sample.Value)
				}
				if len(out.Buckets) == 0 {
					continue
				}
				idx := sort.Search(len(bucketEnds), func(i int) bool { return bucketEnds[i] > seg.startMs })
				if idx < len(bucketEnds) {
					addVolumeSamples(bucketValues, groupSet, idx, key, splitBy, seg.samples)
				}
			}
		}
	}
	if failedTotals {
		out.PartialErrors = append(out.PartialErrors, "totals")
	}
	for idx := range failedBuckets {
		out.PartialErrors = append(out.PartialErrors, "bucket_"+strconv.Itoa(idx))
	}
}

// markVolumeNoData 按最早样本标记桶：整桶早于最早样本记 no_data（值恒为 0），
// 跨过最早样本的桶照常计算但记 partial。
// markVolumeNoData flags buckets against the oldest sample: buckets entirely before it get
// no_data (values stay 0); the bucket straddling it is computed normally but marked partial.
func markVolumeNoData(buckets []VolumeBucket, oldestMs int64) {
	if oldestMs <= 0 {
		return
	}
	for i := range buckets {
		if buckets[i].EndMs <= oldestMs {
			buckets[i].NoData = true
		} else if buckets[i].StartMs < oldestMs {
			buckets[i].Partial = true
		}
	}
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

// volumeIncreaseMaxSpan 是单次 increase 允许的最长窗口。超过则按显示时区切成日桶再求和，
// 避免本月 / 按月柱对每条序列一次加载 ~18 万点，在 max_samples=1000 万时大约 56 个长跑作业就触顶。
// volumeIncreaseMaxSpan is the longest window one increase may load. Longer spans are split into
// display-timezone day buckets and summed, so a month / monthly bar does not load ~180k points per
// series and trip max_samples=10M at about 56 long-running jobs.
const volumeIncreaseMaxSpan = 24 * time.Hour

// queryVolumeIncrease 对跨度 ≤1 天的区间发一条 increase；更长的区间按日桶拆开再把样本加总。
// 完整日桶的求值时刻落在当地零点，能命中 alignPresetNow 对齐后的 15s PromQL 缓存。
// queryVolumeIncrease issues one increase when the span is ≤1 day; longer spans are split into day
// buckets and the samples are summed. Full-day buckets evaluate at local midnight so they can hit
// the 15s PromQL cache after alignPresetNow.
func (s *VolumeService) queryVolumeIncrease(ctx context.Context, counter, sel, splitBy string, start, end time.Time, loc *time.Location) ([]promquery.Sample, error) {
	var merged []promquery.Sample
	for _, chunk := range volumeIncreaseChunks(start, end, loc) {
		samples, err := s.instantQuery(ctx, volumeQuery(counter, sel, splitBy, chunk.end.Sub(chunk.start)), chunk.end)
		if err != nil {
			return nil, err
		}
		merged = mergeVolumeSamples(merged, samples, splitBy)
	}
	return merged, nil
}

// volumeIncreaseChunks 把长窗口切成显示时区的自然日；DST 下单日超过 24h 时仍只发一条（约 25h，远低于上限）。
// volumeIncreaseChunks splits a long window onto display-timezone calendar days; a DST day longer
// than 24h stays one query (~25h, well under the sample cap).
func volumeIncreaseChunks(start, end time.Time, loc *time.Location) []volumeSpan {
	if !end.After(start) || end.Sub(start) <= volumeIncreaseMaxSpan {
		return []volumeSpan{{start: start, end: end}}
	}
	buckets := volumeBuckets(start, end, loc, "day")
	if len(buckets) <= 1 {
		return []volumeSpan{{start: start, end: end}}
	}
	chunks := make([]volumeSpan, 0, len(buckets))
	for _, bucket := range buckets {
		chunks = append(chunks, volumeSpan{
			start: time.UnixMilli(bucket.StartMs),
			end:   time.UnixMilli(bucket.EndMs),
		})
	}
	return chunks
}

// mergeVolumeSamples 把多段日桶的即时样本按分组加总；无拆分时合成一条。
// mergeVolumeSamples sums instant samples from day chunks by group; with no split it collapses to one sample.
func mergeVolumeSamples(dst, add []promquery.Sample, splitBy string) []promquery.Sample {
	if len(add) == 0 {
		return dst
	}
	if splitBy == "" {
		sum := 0.0
		ts := int64(0)
		for _, sample := range dst {
			sum += sample.Value
			if sample.TsMs > ts {
				ts = sample.TsMs
			}
		}
		for _, sample := range add {
			sum += sample.Value
			if sample.TsMs > ts {
				ts = sample.TsMs
			}
		}
		return []promquery.Sample{{Value: sum, TsMs: ts}}
	}
	type acc struct {
		metric map[string]string
		value  float64
		ts     int64
	}
	byID := map[string]*acc{}
	order := make([]string, 0, len(dst)+len(add))
	addOne := func(sample promquery.Sample) {
		id := strings.TrimSpace(sample.Metric[splitBy])
		if id == "" {
			id = "0"
		}
		item, ok := byID[id]
		if !ok {
			item = &acc{metric: sample.Metric}
			byID[id] = item
			order = append(order, id)
		}
		item.value += sample.Value
		if sample.TsMs > item.ts {
			item.ts = sample.TsMs
		}
	}
	for _, sample := range dst {
		addOne(sample)
	}
	for _, sample := range add {
		addOne(sample)
	}
	out := make([]promquery.Sample, 0, len(order))
	for _, id := range order {
		item := byID[id]
		out = append(out, promquery.Sample{Metric: item.metric, Value: item.value, TsMs: item.ts})
	}
	return out
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

// volumeFloor 返回 t 所在桶的起点，按显示时区的自然界对齐（刻钟 / 整点 / 日 / 周 / 月）；
// 周取当周周一零点。
// volumeFloor returns the start of the bucket containing t, aligned to the display timezone's
// natural boundary (quarter-hour / hour / day / week / month); weeks start on Monday 00:00.
func volumeFloor(t time.Time, loc *time.Location, granularity string) time.Time {
	l := t.In(loc)
	switch granularity {
	case "month":
		return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, loc)
	case "week":
		// Go 的 Weekday 周日为 0；换算成「距周一的天数」再回退到周一。
		// Go's Weekday starts at Sunday; convert to days-since-Monday then step back.
		return time.Date(l.Year(), l.Month(), l.Day()-(int(l.Weekday())+6)%7, 0, 0, 0, 0, loc)
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
	case "week":
		// 日历加法跨 DST 仍对；下一周起点仍是当地周一零点。
		// Calendar arithmetic stays correct across DST; the next week still starts at local
		// Monday midnight.
		return t.AddDate(0, 0, 7)
	case "day":
		return t.AddDate(0, 0, 1)
	case "hour":
		return t.Add(time.Hour)
	default:
		return t.Add(15 * time.Minute)
	}
}

// volumeLabel 是桶的展示标签：亚日粒度带时刻，日 / 周取起点日期 YYYY-MM-DD（周为周一），月取 YYYY-MM。
// volumeLabel renders a bucket label: sub-day granularities carry the time, days/weeks use the
// start date YYYY-MM-DD (the Monday for weeks), months YYYY-MM.
func volumeLabel(t time.Time, granularity string) string {
	switch granularity {
	case "month":
		return t.Format("2006-01")
	case "day", "week":
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
// @Param range query string false "时间范围：15m|1h|6h|24h|7d|30d|90d|1y" default(1h)
// @Param start query string false "自定义起点：RFC3339 或秒 / 毫秒时间戳，优先于 range"
// @Param end query string false "自定义终点，缺省为当前时间；跨度不超过 366 天"
// @Param granularity query string false "桶粒度：auto|quarter_hour|hour|day|week|month；auto 按窗口跨度选桶，缺省只回 totals 不出 buckets"
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
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "granularity must be auto|quarter_hour|hour|day|week|month"})
		return
	}
	split := strings.ToLower(strings.TrimSpace(c.DefaultQuery("split", "none")))
	if _, ok := volumeSplits[split]; !ok {
		c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "split must be none|cluster|task"})
		return
	}
	window, err := promquery.ParseWindow(c.Query("start"), c.Query("end"), time.Now())
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
