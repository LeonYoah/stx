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
	"fmt"
	"strings"
	"testing"

	"github.com/LeonYoah/stx/internal/config"
	dto "github.com/prometheus/client_model/go"
)

func TestJobHasEngineObservability(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spec JSONMap
		want bool
	}{
		{name: "nil", want: false},
		{name: "injected", spec: JSONMap{"observability": map[string]any{"injected": true, "reason": "injected"}}, want: true},
		{name: "user configured", spec: JSONMap{"observability": map[string]any{"injected": false, "reason": "user_configured"}}, want: true},
		{name: "disabled", spec: JSONMap{"observability": map[string]any{"injected": false, "reason": "disabled_by_task"}}, want: false},
		{name: "batch", spec: JSONMap{"observability": map[string]any{"reason": "batch_job"}}, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			job := &JobInstance{ID: 1, SubmitSpec: tc.spec}
			if got := jobHasEngineObservability(job); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestJobPrometheusExporter_observeAndDelete(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	// Use a fresh exporter to avoid cross-test pollution of the process-wide singleton.
	// 使用全新 exporter，避免污染进程级单例。
	exp := newJobPrometheusExporter()

	job := &JobInstance{
		ID:         42,
		TaskID:     7,
		Status:     JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": float64(3)},
	}
	exp.observeJobMetrics(job, map[string]interface{}{
		"SourceReceivedCount": float64(100),
		"SinkWriteCount":      float64(90),
		"SinkCommittedCount":  float64(80),
		"TableSourceReceivedCount": map[string]interface{}{
			"a.t1": float64(60),
			"a.t2": float64(40),
		},
	})
	exp.observeCDCDelay(job, 1.5)

	mfs, err := exp.reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	foundSource := false
	for _, mf := range mfs {
		if mf.GetName() == "stx_job_source_received_rows_total" {
			foundSource = true
			if sumCounter(mf) < 100 {
				t.Fatalf("expected source counter >= 100, got %v", sumCounter(mf))
			}
		}
	}
	if !foundSource {
		t.Fatalf("missing source counter metric")
	}

	exp.deleteJob(job)
	mfs, err = exp.reg.Gather()
	if err != nil {
		t.Fatalf("gather after delete: %v", err)
	}
	for _, mf := range mfs {
		if strings.HasPrefix(mf.GetName(), "stx_job_") && sumCounter(mf)+sumGauge(mf) != 0 && len(mf.GetMetric()) > 0 {
			// After delete, series for this job should be gone; other jobs may remain (none here).
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "job_id" && lp.GetValue() == "42" {
						t.Fatalf("job series still present on %s", mf.GetName())
					}
				}
			}
		}
	}
}

func sumCounter(mf *dto.MetricFamily) float64 {
	var sum float64
	for _, m := range mf.GetMetric() {
		if m.Counter != nil {
			sum += m.Counter.GetValue()
		}
	}
	return sum
}

func sumGauge(mf *dto.MetricFamily) float64 {
	var sum float64
	for _, m := range mf.GetMetric() {
		if m.Gauge != nil {
			sum += m.Gauge.GetValue()
		}
	}
	return sum
}

func TestJobPrometheusExporter_alignsSingleLinkTableAliases(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	exp := newJobPrometheusExporter()
	job := &JobInstance{
		ID:         77,
		TaskID:     8,
		Status:     JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": float64(3)},
		ResultPreview: JSONMap{"job_dag": map[string]any{
			"vertexInfoMap": []any{
				map[string]any{"vertexId": 1, "type": "source", "tablePaths": []any{"wcan-common-tbox1-r1p24"}},
				map[string]any{"vertexId": 2, "type": "transform", "tablePaths": []any{}},
				map[string]any{"vertexId": 3, "type": "sink", "tablePaths": []any{"ah1110.ods_wcan_common_tbox1_json"}},
			},
		}},
	}
	exp.observeJobMetrics(job, map[string]interface{}{
		"TableSourceReceivedCount": map[string]interface{}{
			"Source[0].wcan-common-tbox1-r1p24": float64(0),
			"Optional[wcan-common-tbox1-r1p24]": float64(233132),
		},
		"TableSinkWriteCount": map[string]interface{}{
			"Sink[0].ah1110.ods_wcan_common_tbox1_json": float64(0),
			"default.default.default":                   float64(233132),
		},
		"TableSinkCommittedCount": map[string]interface{}{
			"Sink[0].ah1110.ods_wcan_common_tbox1_json": float64(0),
			"default.default.default":                   float64(232832),
		},
	})
	values := gatherTableRowSeries(t, exp, "77")
	if values["read|wcan-common-tbox1-r1p24"] != 233132 {
		t.Fatalf("aligned source series mismatch: %#v", values)
	}
	if values["write|ah1110.ods_wcan_common_tbox1_json"] != 233132 {
		t.Fatalf("aligned sink series mismatch: %#v", values)
	}
	if _, phantom := values["write|wcan-common-tbox1-r1p24"]; phantom {
		t.Fatalf("source-only table must not emit phantom write counter: %#v", values)
	}
	if _, phantom := values["read|ah1110.ods_wcan_common_tbox1_json"]; phantom {
		t.Fatalf("sink-only table must not emit phantom read counter: %#v", values)
	}
	for key := range values {
		if strings.Contains(key, "Optional[") || strings.Contains(key, "default.default.default") {
			t.Fatalf("raw alias series should not be exported after alignment: %#v", values)
		}
	}
}

func TestJobPrometheusExporter_doesNotAlignDynamicMultiTableMetrics(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	exp := newJobPrometheusExporter()
	job := &JobInstance{
		ID:         81,
		TaskID:     8,
		Status:     JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": float64(3)},
		ResultPreview: JSONMap{"job_dag": map[string]any{
			"vertexInfoMap": []any{
				map[string]any{"vertexId": 1, "type": "source", "tablePaths": []any{"db.orders_1"}},
				map[string]any{"vertexId": 2, "type": "sink", "tablePaths": []any{"ods.orders_1"}},
			},
		}},
	}
	exp.observeJobMetrics(job, map[string]interface{}{
		"TableSourceReceivedCount": map[string]interface{}{
			"Source[0].db.orders_1": float64(100),
			"Source[0].db.orders_2": float64(200),
		},
		"TableSinkWriteCount": map[string]interface{}{
			"Sink[0].ods.orders_1": float64(100),
			"Sink[0].ods.orders_2": float64(200),
		},
	})
	values := gatherTableRowSeries(t, exp, "81")
	if values["read|Source[0].db.orders_1"] != 100 || values["read|Source[0].db.orders_2"] != 200 {
		t.Fatalf("dynamic multi-table source series must remain separate: %#v", values)
	}
	if values["write|Sink[0].ods.orders_1"] != 100 || values["write|Sink[0].ods.orders_2"] != 200 {
		t.Fatalf("dynamic multi-table sink series must remain separate: %#v", values)
	}
}

func TestJobPrometheusExporter_singleLinkJobTotalFallback(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	exp := newJobPrometheusExporter()
	job := &JobInstance{
		ID:         82,
		TaskID:     8,
		Status:     JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": float64(3)},
		ResultPreview: JSONMap{"job_dag": map[string]any{
			"vertexInfoMap": []any{
				map[string]any{"vertexId": 1, "type": "source", "tablePaths": []any{"topic-a"}},
				map[string]any{"vertexId": 2, "type": "sink", "tablePaths": []any{"db.target"}},
			},
		}},
	}
	exp.observeJobMetrics(job, map[string]interface{}{
		"SourceReceivedCount": float64(300),
		"SinkWriteCount":      float64(300),
		"TableSourceReceivedCount": map[string]interface{}{
			"Source[0].topic-a": float64(0),
		},
		"TableSinkWriteCount": map[string]interface{}{
			"Sink[0].db.target": float64(0),
		},
	})
	values := gatherTableRowSeries(t, exp, "82")
	if values["read|topic-a"] != 300 || values["write|db.target"] != 300 {
		t.Fatalf("single-link job totals should raise zero per-table counters: %#v", values)
	}
}

func TestJobPrometheusExporter_doesNotAlignAmbiguousTableAliases(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	exp := newJobPrometheusExporter()
	job := &JobInstance{
		ID:         78,
		TaskID:     8,
		Status:     JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": float64(3)},
		ResultPreview: JSONMap{"job_dag": map[string]any{
			"vertexInfoMap": []any{
				map[string]any{"vertexId": 1, "type": "source", "tablePaths": []any{"topic-a"}},
				map[string]any{"vertexId": 2, "type": "source", "tablePaths": []any{"topic-b"}},
			},
		}},
	}
	exp.observeJobMetrics(job, map[string]interface{}{
		"TableSourceReceivedCount": map[string]interface{}{
			"Optional[topic-a]": float64(100),
		},
	})
	values := gatherTableRowSeries(t, exp, "78")
	if values["read|Optional[topic-a]"] != 100 {
		t.Fatalf("ambiguous raw key must remain unaligned: %#v", values)
	}
}

func TestJobPrometheusExporter_committedCountRaisesAlignedWriteFloor(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	exp := newJobPrometheusExporter()
	job := &JobInstance{
		ID:         79,
		TaskID:     8,
		Status:     JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": float64(3)},
		ResultPreview: JSONMap{"job_dag": map[string]any{
			"vertexInfoMap": []any{
				map[string]any{"vertexId": 1, "type": "source", "tablePaths": []any{"topic-a"}},
				map[string]any{"vertexId": 2, "type": "sink", "tablePaths": []any{"db.target"}},
			},
		}},
	}
	exp.observeJobMetrics(job, map[string]interface{}{
		"TableSinkWriteCount": map[string]interface{}{
			"Sink[0].db.target": float64(0),
		},
		"TableSinkCommittedCount": map[string]interface{}{
			"default.default.default": float64(42),
		},
	})
	values := gatherTableRowSeries(t, exp, "79")
	if values["write|db.target"] != 42 {
		t.Fatalf("committed fallback should raise the aligned write floor: %#v", values)
	}

	// 同 key 写入为 0、提交非 0 时，也应以提交量抬高下限。
	// When the same key reports 0 written and nonzero committed, committed count should also raise the floor.
	jobSameKey := &JobInstance{
		ID:            80,
		TaskID:        8,
		Status:        JobStatusRunning,
		SubmitSpec:    JSONMap{"cluster_id": float64(3)},
		ResultPreview: job.ResultPreview,
	}
	exp.observeJobMetrics(jobSameKey, map[string]interface{}{
		"TableSinkWriteCount": map[string]interface{}{
			"Sink[0].db.target": float64(0),
		},
		"TableSinkCommittedCount": map[string]interface{}{
			"Sink[0].db.target": float64(55),
		},
	})
	sameKeyValues := gatherTableRowSeries(t, exp, "80")
	if sameKeyValues["write|db.target"] != 55 {
		t.Fatalf("same-key committed fallback should raise the aligned write floor: %#v", sameKeyValues)
	}
}

func gatherTableRowSeries(t *testing.T, exp *jobPrometheusExporter, jobID string) map[string]float64 {
	t.Helper()
	mfs, err := exp.reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	values := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() != "stx_job_table_rows_total" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["job_id"] != jobID {
				continue
			}
			values[labels["direction"]+"|"+labels["table"]] = metric.GetCounter().GetValue()
		}
	}
	return values
}

func TestJobPrometheusExporter_tableCapRollsIntoOther(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	exp := newJobPrometheusExporter()

	tableCounts := make(map[string]interface{}, jobMetricsTableCap+2)
	for i := 0; i < jobMetricsTableCap+2; i++ {
		// Descending counts so sort keeps t000.. as top; overflow tables have the smallest totals.
		// 递减计数，保证排序后靠前的表保留，溢出表并入 other。
		tableCounts[fmt.Sprintf("t%03d", i)] = float64(jobMetricsTableCap + 2 - i)
	}
	job := &JobInstance{
		ID:         99,
		TaskID:     1,
		Status:     JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": float64(1)},
	}
	exp.observeJobMetrics(job, map[string]interface{}{
		"SourceReceivedCount":      float64(1000),
		"SinkWriteCount":           float64(1000),
		"TableSourceReceivedCount": tableCounts,
	})

	mfs, err := exp.reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var tableSeries, otherSeries int
	var otherValue float64
	for _, mf := range mfs {
		if mf.GetName() != "stx_job_table_rows_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			table := ""
			direction := ""
			for _, lp := range m.GetLabel() {
				switch lp.GetName() {
				case "table":
					table = lp.GetValue()
				case "direction":
					direction = lp.GetValue()
				}
			}
			if direction != "read" {
				continue
			}
			tableSeries++
			if table == "other" {
				otherSeries++
				if m.Counter != nil {
					otherValue = m.Counter.GetValue()
				}
			}
		}
	}
	// Cap named tables + one "other" bucket for the overflow.
	// 上限内具名表 + 一个 other 桶。
	if tableSeries != jobMetricsTableCap+1 {
		t.Fatalf("read table series=%d want %d (cap+other)", tableSeries, jobMetricsTableCap+1)
	}
	if otherSeries != 1 {
		t.Fatalf("other series=%d want 1", otherSeries)
	}
	// Overflow tables are the two smallest: 1 + 2 = 3.
	// 溢出的两张最小表：1 + 2 = 3。
	if otherValue != 3 {
		t.Fatalf("other read value=%v want 3", otherValue)
	}
}
