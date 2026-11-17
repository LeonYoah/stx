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
	"strings"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

type fakeJobSource struct {
	asked []uint
}

func (f *fakeJobSource) DescribeJobsForDashboard(_ context.Context, _ executionapp.Actor, ids []uint) (map[uint]syncapp.JobBrief, error) {
	f.asked = ids
	return map[uint]syncapp.JobBrief{
		1: {ID: 1, TaskID: 10, ClusterID: 1, Name: "orders", Sources: []string{"MySQL-CDC"}, Sinks: []string{"Doris", "Console"}, Viewable: true},
		2: {ID: 2, TaskID: 20, ClusterID: 1, Name: "ledger", Sources: []string{"Postgres-CDC"}, Sinks: []string{"Doris"}},
	}, nil
}

func (f *fakeJobSource) JobRunCalendar(context.Context, uint, int, time.Time) (*syncapp.JobCalendar, error) {
	return &syncapp.JobCalendar{Weeks: 16, EndDate: "2026-11-15"}, nil
}

func (f *fakeJobSource) JobAnnotationsBetween(context.Context, executionapp.Actor, uint, time.Time, time.Time) ([]syncapp.JobAnnotation, error) {
	return []syncapp.JobAnnotation{{TsMs: 1, Kind: "job_failed", JobID: 2}}, nil
}

func newTestJobService(source *fakeJobSource, queries *[]string) *JobDashboardService {
	svc := NewJobDashboardService(source)
	svc.now = func() time.Time { return time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC) }
	svc.rangeQuery = func(_ context.Context, q string, _, _ time.Time, _ int) ([]promquery.Series, error) {
		recordQuery(queries, q)
		if strings.Contains(q, "bytes") {
			return nil, errors.New("boom")
		}
		return []promquery.Series{{Metric: map[string]string{}, Points: [][2]float64{{1000, 5}}}}, nil
	}
	svc.instantQuery = func(_ context.Context, q string, _ time.Time) ([]promquery.Sample, error) {
		recordQuery(queries, q)
		job := func(id string, v float64, extra ...string) promquery.Sample {
			m := map[string]string{"job_id": id}
			for i := 0; i+1 < len(extra); i += 2 {
				m[extra[i]] = extra[i+1]
			}
			return promquery.Sample{Metric: m, Value: v}
		}
		switch {
		case strings.Contains(q, "increase(stx_job_sink_write_rows_total"):
			return []promquery.Sample{job("1", 7200), job("2", 3600), job("3", 0)}, nil
		case strings.Contains(q, "cdc_delay"):
			return []promquery.Sample{job("2", 74), job("1", 3)}, nil
		case strings.Contains(q, "checkpoint_duration"):
			return []promquery.Sample{job("1", 2.5)}, nil
		case strings.Contains(q, "checkpoint_failed"):
			return []promquery.Sample{job("1", 2)}, nil
		case strings.Contains(q, "busy_ratio"):
			return []promquery.Sample{job("1", 0.4, "vertex", "Source[0]"), job("2", 0.4, "vertex", "Sink[0]")}, nil
		case strings.Contains(q, "backpressure"):
			return []promquery.Sample{job("1", 0.82, "edge", "Sink[0]")}, nil
		}
		return nil, nil
	}
	return svc
}

func TestJobDashboardAggregatesOverTheWindow(t *testing.T) {
	withPrometheus(t, true)
	source := &fakeJobSource{}
	var queries []string
	got, err := newTestJobService(source, &queries).GetJobDashboard(context.Background(), executionapp.Actor{UserID: 2}, 0, "1h", nil, 16)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if got.WindowSeconds != 3600 || len(got.Throughput) != 4 || len(got.Throughput[1].Points) != 1 || len(got.Throughput[2].Points) != 0 {
		t.Fatalf("throughput = %+v", got.Throughput)
	}
	// 平均写入速率 = 窗口写入行数 / 3600；0 行的作业不进榜。/ Average rate = rows / 3600; zero-row jobs are left out.
	if len(got.TopWrite) != 2 || got.TopWrite[0].JobID != 1 || got.TopWrite[0].Value != 2 || got.TopWrite[1].Value != 1 {
		t.Fatalf("top write = %+v", got.TopWrite)
	}
	if got.TopWrite[1].Name != "ledger" || got.TopWrite[1].Viewable || !got.TopWrite[0].Viewable {
		t.Fatalf("names / viewable = %+v", got.TopWrite)
	}
	if got.TopCDC[0].JobID != 2 || got.TopCDC[0].Value != 74 {
		t.Fatalf("top cdc = %+v", got.TopCDC)
	}
	if len(got.Checkpoints) != 1 || *got.Checkpoints[0].MaxSeconds != 2.5 || *got.Checkpoints[0].Failures != 2 {
		t.Fatalf("checkpoints = %+v", got.Checkpoints)
	}
	if len(got.Hotspots) != 3 || got.Hotspots[0].Kind != "backpressure" || got.Hotspots[0].Vertex != "Sink[0]" {
		t.Fatalf("hotspots = %+v", got.Hotspots)
	}
	for _, item := range got.Hotspots {
		if item.Kind == "busy" && ((item.JobID == 1 && item.Vertex != "Source[0]") || (item.JobID == 2 && item.Vertex != "Sink[0]")) {
			t.Fatalf("equal values must keep their own vertex: %+v", got.Hotspots)
		}
	}
	// 作业 1 的 7200 行平均分给 Doris / Console；作业 2 的 3600 行全给 Doris。
	// Job 1's 7200 rows split across Doris / Console; job 2's 3600 rows all go to Doris.
	flow := map[string]float64{}
	for _, link := range got.Flow {
		flow[link.Source+">"+link.Sink] = link.Value
	}
	if flow["MySQL-CDC>Doris"] != 3600 || flow["MySQL-CDC>Console"] != 3600 || flow["Postgres-CDC>Doris"] != 3600 || len(flow) != 3 {
		t.Fatalf("flow = %v", flow)
	}
	if len(got.PartialErrors) != 2 || got.Calendar == nil || len(got.Annotations) != 1 {
		t.Fatalf("partial %v calendar %v annotations %v", got.PartialErrors, got.Calendar, got.Annotations)
	}
	for _, q := range queries {
		if strings.Contains(q, "increase(") && !strings.Contains(q, "[3600s]") {
			t.Fatalf("window aggregate %q must use the window span", q)
		}
	}
}

func TestJobDashboardScopesAndDegrades(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	if _, err := newTestJobService(&fakeJobSource{}, &queries).GetJobDashboard(context.Background(), executionapp.Actor{}, 3, "15m", nil, 16); err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	for _, q := range queries {
		if !strings.Contains(q, `cluster_id="3"`) {
			t.Fatalf("query %q not scoped", q)
		}
	}
	withPrometheus(t, false)
	queries = nil
	got, err := newTestJobService(&fakeJobSource{}, &queries).GetJobDashboard(context.Background(), executionapp.Actor{}, 0, "", nil, 16)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if got.Source != "none" || len(queries) != 0 || got.Calendar == nil || len(got.TopWrite) != 0 || got.TopWrite == nil {
		t.Fatalf("no-prometheus dashboard = %+v", got)
	}
	if _, err := newTestJobService(&fakeJobSource{}, &queries).GetJobDashboard(context.Background(), executionapp.Actor{}, 0, "3h", nil, 16); !errors.Is(err, ErrInvalidJobDashboardQuery) {
		t.Fatalf("bad range err = %v", err)
	}
}
