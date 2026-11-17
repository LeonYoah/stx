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
	"strconv"
	"strings"
	"testing"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// fakeVolumeTasks 提供固定的任务名映射。
// fakeVolumeTasks serves a fixed task-name map.
type fakeVolumeTasks struct {
	asked []uint
}

func (f *fakeVolumeTasks) ResolveTaskNamesForActor(_ context.Context, _ executionapp.Actor, ids []uint) map[uint]string {
	f.asked = ids
	return map[uint]string{10: "订单同步", 20: "ledger"}
}

// fakeVolumeClusters 提供固定的集群列表。
// fakeVolumeClusters serves a fixed cluster list.
type fakeVolumeClusters struct{}

func (fakeVolumeClusters) List(context.Context, *clusterapp.ClusterFilter) ([]*clusterapp.Cluster, int64, error) {
	return []*clusterapp.Cluster{{ID: 3, Name: "prod-cluster"}, {ID: 4, Name: "staging"}}, 2, nil
}

// newTestVolumeService 返回注入了假即时查询与固定时间的 VolumeService；
// samples 回调按查询语句决定返回值。
// newTestVolumeService returns a VolumeService with a fake instant query and fixed clock;
// the samples callback picks results per query.
func newTestVolumeService(queries *[]string, ats *[]int64, samples func(q string, at time.Time) ([]promquery.Sample, error)) *VolumeService {
	svc := NewVolumeService(&fakeVolumeTasks{}, fakeVolumeClusters{})
	// 固定 now=2026-11-16 10:00 UTC（显示时区默认 Asia/Shanghai = 当天 18:00）。
	// Fixed now = 2026-11-16 10:00 UTC (default display tz Asia/Shanghai = 18:00 same day).
	svc.now = func() time.Time { return time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC) }
	svc.oldestSample = func() (time.Time, bool) { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), true }
	svc.instantQuery = func(_ context.Context, _, q string, at time.Time) ([]promquery.Sample, error) {
		recordQuery(queries, q)
		if ats != nil {
			queryMu.Lock()
			*ats = append(*ats, at.UnixMilli())
			queryMu.Unlock()
		}
		return samples(q, at)
	}
	return svc
}

// volumeSamplesFor 返回一个按 [span 秒数] 给值的假样本回调：span 解析自查询里的 [Ns]。
// volumeSamplesFor returns a fake callback that yields a value derived from the [Ns] span.
func volumeSamplesFor(perSecond float64) func(string, time.Time) ([]promquery.Sample, error) {
	return func(q string, _ time.Time) ([]promquery.Sample, error) {
		start := strings.Index(q, "[")
		end := strings.Index(q, "s])")
		if start < 0 || end < 0 {
			return nil, nil
		}
		secs, err := strconv.ParseInt(q[start+1:end], 10, 64)
		if err != nil {
			return nil, nil
		}
		return []promquery.Sample{{Metric: map[string]string{}, Value: float64(secs) * perSecond}}, nil
	}
}

func TestVolumeBuckets_DaySplitShanghai(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// 窗口 11-15 12:00 → 11-16 18:00（当地时间）：两天，首末各缺半天。
	// Window 11-15 12:00 → 11-16 18:00 local: two days, both edges clipped.
	start := time.Date(2026, 11, 15, 12, 0, 0, 0, loc)
	end := time.Date(2026, 11, 16, 18, 0, 0, 0, loc)
	buckets := volumeBuckets(start, end, loc, "day")
	if len(buckets) != 2 {
		t.Fatalf("want 2 day buckets, got %d", len(buckets))
	}
	if !buckets[0].Partial || !buckets[1].Partial {
		t.Fatalf("both edge buckets should be partial: %+v", buckets)
	}
	if buckets[0].Label != "2026-11-15" || buckets[1].Label != "2026-11-16" {
		t.Fatalf("bad labels: %+v", buckets)
	}
	// 完整窗口验证：再往后一天零点结束。/ Full-window check: extend end to the next midnight.
	end = time.Date(2026, 11, 17, 0, 0, 0, 0, loc)
	buckets = volumeBuckets(start, end, loc, "day")
	if len(buckets) != 2 || buckets[1].Partial {
		t.Fatalf("last bucket should be complete: %+v", buckets)
	}
	if buckets[1].EndMs != end.UnixMilli() || buckets[1].StartMs != time.Date(2026, 11, 16, 0, 0, 0, 0, loc).UnixMilli() {
		t.Fatalf("bad last bucket span: %+v", buckets[1])
	}
}

func TestVolumeBuckets_MonthSplit(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// 跨 10/11 月边界的窗口：两个月桶，均被裁。/ Window over the Oct/Nov edge: two clipped month buckets.
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, loc)
	end := time.Date(2026, 11, 10, 0, 0, 0, 0, loc)
	buckets := volumeBuckets(start, end, loc, "month")
	if len(buckets) != 2 {
		t.Fatalf("want 2 month buckets, got %d", len(buckets))
	}
	if buckets[0].Label != "2026-10" || buckets[1].Label != "2026-11" {
		t.Fatalf("bad month labels: %+v", buckets)
	}
	// 10 月桶终点应正好在 11-01 零点。/ October bucket ends exactly at Nov 1 midnight.
	if buckets[0].EndMs != time.Date(2026, 11, 1, 0, 0, 0, 0, loc).UnixMilli() {
		t.Fatalf("month boundary misaligned: %+v", buckets[0])
	}
	if !buckets[0].Partial || !buckets[1].Partial {
		t.Fatalf("both months should be partial: %+v", buckets)
	}
}

func TestVolumeBuckets_DSTAligned(t *testing.T) {
	// 美东 2026-03-08 夏令时开始（周日 02:00 跳 1 小时）：跨切换的日桶仍须落在当地零点。
	// US Eastern DST starts 2026-03-08 (2am springs forward): day buckets across the switch must
	// still land on local midnights.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tz database unavailable")
	}
	start := time.Date(2026, 3, 7, 12, 0, 0, 0, loc)
	end := time.Date(2026, 3, 9, 12, 0, 0, 0, loc)
	buckets := volumeBuckets(start, end, loc, "day")
	if len(buckets) != 3 {
		t.Fatalf("want 3 buckets, got %d", len(buckets))
	}
	// 3-08 这天的桶起点必须是当地 00:00（尽管当天只有 23 小时）。
	// The 3-08 bucket must start at local midnight even though that day is 23h long.
	b := buckets[1]
	if b.Label != "2026-03-08" {
		t.Fatalf("bad middle label: %+v", b)
	}
	got := time.UnixMilli(b.StartMs).In(loc)
	if got.Hour() != 0 || got.Minute() != 0 || got.Day() != 8 {
		t.Fatalf("bucket start not at local midnight: %v", got)
	}
	// 相邻两个零点差 23 小时（DST），而不是固定 24 小时。/ Adjacent midnights are 23h apart (DST).
	if diff := buckets[2].StartMs - buckets[1].StartMs; diff != 23*3600*1000 {
		t.Fatalf("DST day length should be 23h, got %dms", diff)
	}
}

func TestVolumeBuckets_SubDayAligned(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// 10:07 → 12:40：刻钟桶 11 个，首 10:00 与末 12:30 均被裁。
	// 10:07 → 12:40: eleven quarter-hour buckets, the 10:00 and 12:30 edges clipped.
	start := time.Date(2026, 11, 16, 10, 7, 0, 0, loc)
	end := time.Date(2026, 11, 16, 12, 40, 0, 0, loc)
	buckets := volumeBuckets(start, end, loc, "quarter_hour")
	if len(buckets) != 11 {
		t.Fatalf("want 11 quarter-hour buckets, got %d: %+v", len(buckets), buckets)
	}
	if buckets[0].Label != "2026-11-16 10:00" || !buckets[0].Partial {
		t.Fatalf("bad first bucket: %+v", buckets[0])
	}
	if got := time.UnixMilli(buckets[1].StartMs).In(loc); got.Minute() != 15 {
		t.Fatalf("bucket not quarter-aligned: %v", got)
	}
	// 整点桶：10:07 → 12:40 给 10/11/12 三个点。
	// Hour buckets: 10:07 → 12:40 yields the 10, 11 and 12 o'clock buckets.
	hours := volumeBuckets(start, end, loc, "hour")
	if len(hours) != 3 || hours[0].Label != "2026-11-16 10:00" || hours[2].Label != "2026-11-16 12:00" {
		t.Fatalf("bad hour buckets: %+v", hours)
	}
}

func TestVolumeGranularityFor(t *testing.T) {
	cases := []struct {
		span time.Duration
		want string
	}{
		{15 * time.Minute, "quarter_hour"},
		{6 * time.Hour, "quarter_hour"},
		{24 * time.Hour, "hour"},
		{72 * time.Hour, "hour"},
		{7 * 24 * time.Hour, "day"},
		{62 * 24 * time.Hour, "day"},
		{90 * 24 * time.Hour, "month"},
	}
	for _, c := range cases {
		if got := volumeGranularityFor(c.span); got != c.want {
			t.Fatalf("span %s: got %q want %q", c.span, got, c.want)
		}
	}
}

func TestGetVolume_AutoGranularity(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newTestVolumeService(&queries, nil, volumeSamplesFor(1.0))
	// 1h 窗口 + auto：应落 quarter_hour（now=10:00 UTC = 上海 18:00，窗口 17:00–18:00）。
	// 1h window + auto resolves to quarter_hour (now=10:00 UTC = 18:00 Shanghai, window 17:00–18:00).
	data, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0, "1h", nil, "auto", "none")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if data.Granularity != "quarter_hour" {
		t.Fatalf("granularity = %q", data.Granularity)
	}
	if len(data.Buckets) != 4 {
		t.Fatalf("want 4 quarter-hour buckets, got %+v", data.Buckets)
	}
	// 窗口正好对齐 17:00 与 18:00 两个边界：首末桶完整不标 partial。
	// The window lands exactly on the 17:00 and 18:00 boundaries: edge buckets stay complete.
	if data.Buckets[0].Partial || data.Buckets[3].Partial {
		t.Fatalf("aligned window must not mark partial: %+v", data.Buckets)
	}
}

func TestVolumeIncreaseChunks_LongSpanSplitsDays(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 11, 16, 18, 0, 0, 0, loc)
	chunks := volumeIncreaseChunks(start, end, loc)
	if len(chunks) != 16 {
		t.Fatalf("want 16 day chunks, got %d", len(chunks))
	}
	if !chunks[0].start.Equal(start) || chunks[0].end.Sub(chunks[0].start) != 24*time.Hour {
		t.Fatalf("bad first chunk: %+v", chunks[0])
	}
	if !chunks[len(chunks)-1].end.Equal(end) || chunks[len(chunks)-1].end.Sub(chunks[len(chunks)-1].start) != 18*time.Hour {
		t.Fatalf("bad last chunk: %+v", chunks[len(chunks)-1])
	}
	short := volumeIncreaseChunks(end.Add(-6*time.Hour), end, loc)
	if len(short) != 1 {
		t.Fatalf("spans ≤1d must stay one query, got %d", len(short))
	}
}

func TestVolumeQuery(t *testing.T) {
	q := volumeQuery("stx_job_sink_write_rows_total", `cluster_id="3"`, "task_id", 90*time.Minute)
	want := `sum by (task_id) (increase(stx_job_sink_write_rows_total{cluster_id="3"}[5400s]))`
	if q != want {
		t.Fatalf("got %q want %q", q, want)
	}
	if q := volumeQuery("m", "", "", time.Second/2); !strings.Contains(q, "[1s]") {
		t.Fatalf("sub-second spans must clamp to 1s: %q", q)
	}
}

func TestGetVolume_TotalsAndDayBuckets(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	var ats []int64
	svc := newTestVolumeService(&queries, &ats, volumeSamplesFor(2.0))
	data, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0, "24h", nil, "day", "none")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if data.Source != promquery.SourcePrometheus {
		t.Fatalf("source = %q", data.Source)
	}
	// 窗口 24h（11-15 10:00 → 11-16 10:00 UTC = 上海 18:00 → 18:00）：两个日桶都被裁。
	// 24h window (UTC 10:00→10:00 = Shanghai 18:00→18:00): both day buckets clipped.
	if len(data.Buckets) != 2 || !data.Buckets[0].Partial || !data.Buckets[1].Partial {
		t.Fatalf("bad buckets: %+v", data.Buckets)
	}
	// 每秒 2 ⇒ 86400 秒窗口 = 172800；今日 18h = 129600；本月 15 天 18h = 2736000。
	// 2/s ⇒ 86400s window = 172800; today 18h = 129600; month 15d18h = 2736000.
	if data.Totals == nil || data.Totals.Window.WriteRows != 172800 {
		t.Fatalf("bad window total: %+v", data.Totals)
	}
	if data.Totals.Today.WriteRows != 18*3600*2 {
		t.Fatalf("bad today total: %+v", data.Totals.Today)
	}
	if data.Totals.Month.WriteRows != (15*86400+18*3600)*2 {
		t.Fatalf("bad month total: %+v", data.Totals.Month)
	}
	if data.Totals.TodayStartMs != time.Date(2026, 11, 16, 0, 0, 0, 0, timex.DisplayLocation()).UnixMilli() {
		t.Fatalf("bad today_start_ms: %d", data.Totals.TodayStartMs)
	}
	if data.OldestSampleMs != time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("bad oldest_sample_ms: %d", data.OldestSampleMs)
	}
	assertIncreaseSpansWithinDay(t, queries)
	// split=none：每指标一条 group_id="0"。/ split=none: one series per metric with group_id "0".
	if len(data.Series) != len(volumeMetrics) {
		t.Fatalf("want %d series, got %d", len(volumeMetrics), len(data.Series))
	}
	for _, s := range data.Series {
		if s.GroupID != "0" || len(s.Totals) != 2 {
			t.Fatalf("bad series: %+v", s)
		}
	}
}

func TestGetVolume_SplitByClusterNames(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newTestVolumeService(&queries, nil, func(q string, _ time.Time) ([]promquery.Sample, error) {
		if strings.Contains(q, "cluster_id=") && strings.Contains(q, "sum by (cluster_id)") {
			return []promquery.Sample{
				{Metric: map[string]string{"cluster_id": "3"}, Value: 10},
				{Metric: map[string]string{"cluster_id": "9"}, Value: 5},
			}, nil
		}
		return []promquery.Sample{{Metric: map[string]string{}, Value: 1}}, nil
	})
	data, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 3, "24h", nil, "day", "cluster")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if data.Groups["3"] != "prod-cluster" {
		t.Fatalf("cluster name not resolved: %+v", data.Groups)
	}
	if _, ok := data.Groups["9"]; ok {
		t.Fatalf("unknown cluster should not get a name: %+v", data.Groups)
	}
	// 9 号集群没有名字但有序列。/ Cluster 9 has a series but no name.
	found := false
	for _, s := range data.Series {
		if s.GroupID == "9" && s.Metric == "write_rows" && s.Totals[0] == 5 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing unnamed group series: %+v", data.Series)
	}
	// 集群过滤选择器必须出现在查询里。/ The cluster selector must reach the PromQL.
	joined := strings.Join(queries, ";")
	if !strings.Contains(joined, `cluster_id="3"`) {
		t.Fatalf("cluster selector missing in queries: %s", joined)
	}
}

func TestGetVolume_SplitByTaskNames(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newTestVolumeService(&queries, nil, func(q string, _ time.Time) ([]promquery.Sample, error) {
		if strings.Contains(q, "sum by (task_id)") {
			return []promquery.Sample{{Metric: map[string]string{"task_id": "10"}, Value: 7}}, nil
		}
		return []promquery.Sample{{Metric: map[string]string{}, Value: 1}}, nil
	})
	data, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0, "24h", nil, "day", "task")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if data.Groups["10"] != "订单同步" {
		t.Fatalf("task name not resolved: %+v", data.Groups)
	}
}

func TestGetVolume_NoGranularityOnlyTotals(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newTestVolumeService(&queries, nil, volumeSamplesFor(1.0))
	data, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0, "1h", nil, "", "none")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if len(data.Buckets) != 0 || len(data.Series) != 0 {
		t.Fatalf("no-granularity call must skip buckets: %+v", data)
	}
	if data.Totals == nil || data.Totals.Window.WriteRows != 3600 {
		t.Fatalf("bad totals: %+v", data.Totals)
	}
	// 窗口 1h、今日 18h 仍各 4 条；本月 16 个日桶 × 4 指标。不得再发一条 increase[15d]。
	// Window 1h and today 18h stay 4 queries each; the month is 16 day buckets × 4 metrics. A
	// single increase[15d] is not allowed.
	if len(queries) != 4+4+16*4 {
		t.Fatalf("want 72 totals queries after day-splitting month, got %d", len(queries))
	}
	assertIncreaseSpansWithinDay(t, queries)
}

func TestGetVolume_NoPrometheus(t *testing.T) {
	withPrometheus(t, false)
	svc := newTestVolumeService(nil, nil, nil)
	data, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0, "1h", nil, "day", "none")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if data.Source != promquery.SourceNone || data.Reason != promquery.ReasonObservabilityDisabled {
		t.Fatalf("bad degraded payload: %+v", data)
	}
	if data.Totals != nil || len(data.Buckets) != 0 {
		t.Fatalf("degraded payload must not carry totals/buckets: %+v", data)
	}
}

func TestGetVolume_PartialErrors(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newTestVolumeService(&queries, nil, func(q string, _ time.Time) ([]promquery.Sample, error) {
		if strings.Contains(q, "received_bytes") {
			return nil, errors.New("boom")
		}
		return []promquery.Sample{{Metric: map[string]string{}, Value: 3}}, nil
	})
	data, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0, "24h", nil, "day", "none")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	joined := strings.Join(data.PartialErrors, ",")
	if !strings.Contains(joined, "totals") || !strings.Contains(joined, "bucket_") {
		t.Fatalf("missing partial errors: %v", data.PartialErrors)
	}
	if data.Totals.Window.WriteRows != 3 {
		t.Fatalf("other directions must still fill: %+v", data.Totals)
	}
}

func TestGetVolume_MonthBucketsUseDayIncreases(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	loc, _ := time.LoadLocation("Asia/Shanghai")
	svc := newTestVolumeService(&queries, nil, volumeSamplesFor(1.0))
	custom := &promquery.Window{
		Start: time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
		End:   time.Date(2026, 11, 16, 18, 0, 0, 0, loc),
	}
	data, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0, "", custom, "month", "none")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if len(data.Buckets) != 3 {
		t.Fatalf("want Sep/Oct/Nov buckets, got %+v", data.Buckets)
	}
	assertIncreaseSpansWithinDay(t, queries)
	if data.Series[0].Totals[0] <= 0 || data.Series[0].Totals[1] <= 0 {
		t.Fatalf("day-chunked month bars should still sum: %+v", data.Series[0])
	}
}

func TestGetVolume_InvalidRange(t *testing.T) {
	svc := newTestVolumeService(nil, nil, nil)
	if _, err := svc.GetVolume(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0, "3d", nil, "", "none"); !errors.Is(err, ErrInvalidVolumeQuery) {
		t.Fatalf("want ErrInvalidVolumeQuery, got %v", err)
	}
}

// assertIncreaseSpansWithinDay 确认没有单条 increase 超过约一天（允许 DST 25h）。
// assertIncreaseSpansWithinDay checks no increase loads more than about a day (DST 25h allowed).
func assertIncreaseSpansWithinDay(t *testing.T, queries []string) {
	t.Helper()
	const maxSeconds = 25 * 3600
	for _, q := range queries {
		start := strings.Index(q, "[")
		end := strings.Index(q, "s])")
		if start < 0 || end < 0 || end <= start {
			continue
		}
		secs, err := strconv.ParseInt(q[start+1:end], 10, 64)
		if err != nil {
			t.Fatalf("parse increase span in %q: %v", q, err)
		}
		if secs > maxSeconds {
			t.Fatalf("increase span %ds exceeds 1d cap: %s", secs, q)
		}
	}
}
