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

package embedts

import (
	"context"
	"math"
	"strconv"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
)

// FlatSample 是一条展平后的样本：完整标签集 + 数值；时间戳由写入方统一指定（抓取时刻）。
// FlatSample is one flattened sample: a complete label set plus a value; the timestamp is
// assigned by the writer (the scrape time) instead of honoring exposition timestamps.
type FlatSample struct {
	Labels labels.Labels
	Value  float64
}

// FlattenMetricFamilies 把 dto.MetricFamily 集合展平为可直接写入 tsdb 的样本序列：
//   - Gauge / Counter / Untyped 按原名取值；
//   - Summary 展开为 <name>{quantile}/<name>_sum/<name>_count；
//   - Histogram 展开为 <name>_bucket{le}/<name>_sum/<name>_count；
//   - injected 标签（job/instance/cluster 等）注入每条序列；exposition 自带 job/instance
//     标签按 honor_labels=false 约定改名为 exported_job/exported_instance；
//   - 样本自带时间戳一律忽略（不 honor_timestamps，与 Prometheus 默认行为一致）。
//
// FlattenMetricFamilies flattens dto.MetricFamily sets into samples ready for tsdb append:
//   - Gauge / Counter / Untyped are taken by name;
//   - Summary expands into <name>{quantile}/<name>_sum/<name>_count;
//   - Histogram expands into <name>_bucket{le}/<name>_sum/<name>_count;
//   - injected labels (job/instance/cluster etc.) are added to every series; exposition-side
//     job/instance labels are renamed to exported_job/exported_instance per honor_labels=false;
//   - exposition timestamps are ignored (no honor_timestamps, matching the Prometheus default).
func FlattenMetricFamilies(mfs []*dto.MetricFamily, injected map[string]string) []FlatSample {
	if len(mfs) == 0 {
		return nil
	}
	out := make([]FlatSample, 0, len(mfs)*2)
	for _, mf := range mfs {
		if mf == nil {
			continue
		}
		name := mf.GetName()
		if name == "" {
			continue
		}
		switch mf.GetType() {
		case dto.MetricType_GAUGE:
			for _, m := range mf.GetMetric() {
				if m == nil || m.GetGauge() == nil {
					continue
				}
				out = append(out, FlatSample{Labels: buildSeriesLabels(name, m.GetLabel(), nil, injected), Value: m.GetGauge().GetValue()})
			}
		case dto.MetricType_COUNTER:
			for _, m := range mf.GetMetric() {
				if m == nil || m.GetCounter() == nil {
					continue
				}
				out = append(out, FlatSample{Labels: buildSeriesLabels(name, m.GetLabel(), nil, injected), Value: m.GetCounter().GetValue()})
			}
		case dto.MetricType_UNTYPED:
			for _, m := range mf.GetMetric() {
				if m == nil || m.GetUntyped() == nil {
					continue
				}
				out = append(out, FlatSample{Labels: buildSeriesLabels(name, m.GetLabel(), nil, injected), Value: m.GetUntyped().GetValue()})
			}
		case dto.MetricType_SUMMARY:
			for _, m := range mf.GetMetric() {
				if m == nil || m.GetSummary() == nil {
					continue
				}
				summary := m.GetSummary()
				for _, q := range summary.GetQuantile() {
					if q == nil {
						continue
					}
					out = append(out, FlatSample{
						Labels: buildSeriesLabels(name, m.GetLabel(), map[string]string{"quantile": formatFloat(q.GetQuantile())}, injected),
						Value:  q.GetValue(),
					})
				}
				out = append(out,
					FlatSample{Labels: buildSeriesLabels(name+"_sum", m.GetLabel(), nil, injected), Value: summary.GetSampleSum()},
					FlatSample{Labels: buildSeriesLabels(name+"_count", m.GetLabel(), nil, injected), Value: float64(summary.GetSampleCount())},
				)
			}
		case dto.MetricType_HISTOGRAM:
			for _, m := range mf.GetMetric() {
				if m == nil || m.GetHistogram() == nil {
					continue
				}
				histogram := m.GetHistogram()
				for _, b := range histogram.GetBucket() {
					if b == nil {
						continue
					}
					out = append(out, FlatSample{
						Labels: buildSeriesLabels(name+"_bucket", m.GetLabel(), map[string]string{"le": formatFloat(b.GetUpperBound())}, injected),
						Value:  float64(b.GetCumulativeCount()),
					})
				}
				out = append(out,
					FlatSample{Labels: buildSeriesLabels(name+"_sum", m.GetLabel(), nil, injected), Value: histogram.GetSampleSum()},
					FlatSample{Labels: buildSeriesLabels(name+"_count", m.GetLabel(), nil, injected), Value: float64(histogram.GetSampleCount())},
				)
			}
		default:
			// 未知类型跳过：不阻断整轮抓取。
			// Unknown types are skipped so one bad family cannot break the whole scrape round.
			continue
		}
	}
	return out
}

// buildSeriesLabels 合成最终标签集：__name__ + 原始标签（剔除 job/instance → exported_*）+ 扩展标签 + 注入标签。
// buildSeriesLabels composes the final label set: __name__ + original labels (job/instance
// moved to exported_*) + extra labels + injected labels.
func buildSeriesLabels(name string, metricLabels []*dto.LabelPair, extra, injected map[string]string) labels.Labels {
	builder := labels.NewBuilder(labels.EmptyLabels())
	builder.Set(labels.MetricName, name)
	for _, lp := range metricLabels {
		if lp == nil {
			continue
		}
		labelName := lp.GetName()
		switch labelName {
		case "job":
			labelName = "exported_job"
		case "instance":
			labelName = "exported_instance"
		}
		builder.Set(labelName, lp.GetValue())
	}
	for k, v := range extra {
		builder.Set(k, v)
	}
	for k, v := range injected {
		builder.Set(k, v)
	}
	return builder.Labels()
}

// formatFloat 按 Prometheus 惯例格式化 quantile/le 标签值（如 0.5、+Inf）。
// formatFloat formats quantile / le label values following the Prometheus convention (e.g. 0.5, +Inf).
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// AppendFlatSamples 把展平后的样本按同一时间戳写入 appender（调用方负责 Commit）。
// AppendFlatSamples appends flattened samples on the appender with a single timestamp
// (the caller is responsible for Commit).
func AppendFlatSamples(app storage.Appender, samples []FlatSample, tsMs int64) error {
	if app == nil {
		return nil
	}
	for _, sample := range samples {
		if _, err := app.Append(0, sample.Labels, tsMs, sample.Value); err != nil {
			return err
		}
	}
	return nil
}

// StaleNaNValue 是 Prometheus 的 stale 标记值；目标或序列消失时写入它，防止 PromQL 查询里幽灵续存。
// StaleNaNValue is the Prometheus stale marker; written when a target or series disappears
// so PromQL queries stop seeing it after the lookback window instead of a ghosted last value.
var StaleNaNValue = math.Float64frombits(value.StaleNaN)

// SeriesTracker 按标签集哈希追踪某来源（一个抓取目标 / 一次 gather）上一轮写入的序列，
// 用于在序列消失时写 stale marker。
// SeriesTracker tracks the label sets a source (one scrape target / one gather round) wrote
// last round so disappeared series can be marked stale.
type SeriesTracker struct {
	prev map[uint64]labels.Labels
}

// NewSeriesTracker 构造空的序列追踪器。
// NewSeriesTracker creates an empty series tracker.
func NewSeriesTracker() *SeriesTracker {
	return &SeriesTracker{prev: make(map[uint64]labels.Labels)}
}

// Diff 用本轮实际写入的标签集更新追踪器，返回上一轮存在而本轮消失的标签集（需要写 stale marker）。
// Diff replaces the tracked set with this round's label sets and returns the label sets that
// existed last round but are gone now (the ones that need stale markers).
func (t *SeriesTracker) Diff(current []labels.Labels) []labels.Labels {
	if t == nil {
		return nil
	}
	now := make(map[uint64]labels.Labels, len(current))
	for _, lset := range current {
		now[lset.Hash()] = lset
	}
	var stale []labels.Labels
	for hash, lset := range t.prev {
		if _, ok := now[hash]; !ok {
			stale = append(stale, lset)
		}
	}
	t.prev = now
	return stale
}

// Reset 清空追踪器（来源整体消失时配合 stale 写入后调用）。
// Reset clears the tracker (used after writing stales when the whole source disappeared).
func (t *SeriesTracker) Reset() {
	if t == nil {
		return
	}
	t.prev = make(map[uint64]labels.Labels)
}

// snapshot 返回当前追踪的全部标签集（来源整体消失时用于补 stale marker）。
// snapshot returns every tracked label set (used to write stale markers when a whole
// source disappears).
func (t *SeriesTracker) snapshot() []labels.Labels {
	if t == nil || len(t.prev) == 0 {
		return nil
	}
	out := make([]labels.Labels, 0, len(t.prev))
	for _, lset := range t.prev {
		out = append(out, lset)
	}
	return out
}

// AppendStaleMarkers 在一组标签集上写 stale marker 并提交；无标签集时为空操作。
// AppendStaleMarkers writes stale markers for a batch of label sets and commits; a no-op on empty input.
func AppendStaleMarkers(ctx context.Context, lsets []labels.Labels, tsMs int64) error {
	if len(lsets) == 0 {
		return nil
	}
	app := Appender(ctx)
	if app == nil {
		return nil
	}
	for _, lset := range lsets {
		if _, err := app.Append(0, lset, tsMs, StaleNaNValue); err != nil {
			return err
		}
	}
	return app.Commit()
}

// WriteTsWatermark 为单个写入方维护严格递增的样本时间戳水位线：
// tsdb 拒绝同一时间戳上的覆盖写（duplicate sample），同毫秒内连续两轮写入（含 stale marker）必须错开。
// WriteTsWatermark keeps a strictly-increasing sample timestamp watermark per writer: the tsdb
// rejects overwrite appends on the same timestamp, so back-to-back rounds (including stale
// markers) inside one millisecond must be staggered.
type WriteTsWatermark struct {
	v atomic.Int64
}

// Next 返回 max(nowMs, last+1) 并更新水位线。
// Next returns max(nowMs, last+1) and advances the watermark.
func (w *WriteTsWatermark) Next(nowMs int64) int64 {
	for {
		last := w.v.Load()
		next := nowMs
		if next <= last {
			next = last + 1
		}
		if w.v.CompareAndSwap(last, next) {
			return next
		}
	}
}

// NextNow 是以当前时间为基准的 Next 便捷封装。
// NextNow is Next based on the current wall-clock time.
func (w *WriteTsWatermark) NextNow() int64 {
	return w.Next(time.Now().UnixMilli())
}
