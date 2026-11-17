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
	gosync "sync"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// newDetailTestService 返回一个 rangeQuery 按查询内容分派的假服务：
// 面板查询回 instance/pool 双标签序列，启动时刻回一段「上升」序列以触发重启事件。
// newDetailTestService returns a fake whose rangeQuery dispatches on the query text:
// panel queries get instance+pool labelled series, the start-time query returns a rising
// series to trigger a restart event.
func newDetailTestService(t *testing.T, queries *[]string) *ClusterDashboardService {
	t.Helper()
	svc := NewClusterDashboardService(fakeHealth{}, fakeClusterNodes{}, fakeRunning{})
	svc.now = func() time.Time { return time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC) }
	var mu gosync.Mutex
	svc.rangeQuery = func(_ context.Context, q string, _, _ time.Time, _ int) ([]promquery.Series, error) {
		mu.Lock()
		*queries = append(*queries, q)
		mu.Unlock()
		switch {
		case strings.Contains(q, "process_start_time_seconds"):
			// 10.0.0.1 在 2000ms 处启动时刻前移 = 一次重启；10.0.0.2 平稳。
			// 10.0.0.1's start time jumps forward at 2000ms = one restart; 10.0.0.2 is steady.
			return []promquery.Series{
				series("instance", "10.0.0.1:5801", [2]float64{1000, 100}, [2]float64{2000, 150}),
				series("instance", "10.0.0.2:5801", [2]float64{1000, 90}, [2]float64{2000, 90}),
			}, nil
		case strings.Contains(q, "node_count"):
			return []promquery.Series{
				series("cluster_id", "1", [2]float64{1000, 2}, [2]float64{2000, 3}),
			}, nil
		case strings.Contains(q, "jvm_gc_collection"):
			return nil, errors.New("gc query boom")
		case strings.Contains(q, "jvm_memory_pool_bytes_used"):
			return []promquery.Series{
				{Metric: map[string]string{"instance": "10.0.0.1:5801", "pool": "G1 Eden Space"}, Points: [][2]float64{{1000, 10}, {2000, 12}}},
				{Metric: map[string]string{"instance": "10.0.0.1:5801", "pool": "G1 Old Gen"}, Points: [][2]float64{{1000, 30}, {2000, 35}}},
				{Metric: map[string]string{"instance": "10.0.0.2:5801", "pool": "G1 Old Gen"}, Points: [][2]float64{{1000, 20}}},
			}, nil
		}
		return []promquery.Series{series("instance", "10.0.0.1:5801", [2]float64{1000, 1})}, nil
	}
	return svc
}

func TestClusterDetailJvmPanelsAndEvents(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newDetailTestService(t, &queries)
	got, err := svc.GetClusterDetail(context.Background(), 1, "1h", nil, "jvm")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if got.Source != "prometheus" || got.Group != "jvm" {
		t.Fatalf("meta = %s/%s", got.Source, got.Group)
	}
	if len(got.Panels) != 4 {
		t.Fatalf("panels = %d, want 4", len(got.Panels))
	}
	heapPools := got.Panels[0]
	if heapPools.Key != "heap_pools" || heapPools.Unit != "bytes" || len(heapPools.Series) != 3 {
		t.Fatalf("heap_pools = %+v", heapPools)
	}
	if heapPools.Series[0].Instance != "10.0.0.1:5801" || heapPools.Series[0].Label != "G1 Eden Space" {
		t.Fatalf("series[0] = %+v", heapPools.Series[0])
	}
	// 事件：一次重启 + 一次成员数变化，时间升序。
	// Events: one restart + one membership change, ascending by time.
	if len(got.Events) != 2 {
		t.Fatalf("events = %+v", got.Events)
	}
	if got.Events[0].Kind != "node_restart" || got.Events[0].Label != "10.0.0.1:5801" {
		t.Fatalf("event[0] = %+v", got.Events[0])
	}
	if got.Events[1].Kind != "members_change" || got.Events[1].Label != "2 → 3" {
		t.Fatalf("event[1] = %+v", got.Events[1])
	}
	// selector 必须带 cluster_id 过滤；面板查询应并发发起。
	// The selector must carry cluster_id; panel queries fan out concurrently.
	joined := strings.Join(queries, "\n")
	if !strings.Contains(joined, `cluster_id="1"`) {
		t.Fatalf("missing cluster_id filter in queries:\n%s", joined)
	}
	if !strings.Contains(joined, "process_start_time_seconds") || !strings.Contains(joined, "node_count") {
		t.Fatalf("event queries missing:\n%s", joined)
	}
}

func TestClusterDetailGcPartialError(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newDetailTestService(t, &queries)
	got, err := svc.GetClusterDetail(context.Background(), 1, "1h", nil, "gc")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(got.Panels) != 5 {
		t.Fatalf("panels = %d, want 5", len(got.Panels))
	}
	// gc_rate / gc_pause 两个面板共用 jvm_gc_collection 前缀查询，失败应记 partial_errors。
	// gc_rate and gc_pause share the jvm_gc_collection query prefix; the failure lands in partial_errors.
	found := false
	for _, key := range got.PartialErrors {
		if key == "gc_rate" || key == "gc_pause" {
			found = true
		}
	}
	if !found {
		t.Fatalf("partial_errors = %v", got.PartialErrors)
	}
}

func TestClusterDetailSchedMembersReference(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newDetailTestService(t, &queries)
	got, err := svc.GetClusterDetail(context.Background(), 1, "1h", nil, "sched")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	var members *DetailPanel
	for i := range got.Panels {
		if got.Panels[i].Key == "members" {
			members = &got.Panels[i]
		}
	}
	if members == nil || members.Reference == nil || *members.Reference != 2 {
		t.Fatalf("members panel = %+v", members)
	}
}

func TestClusterDetailWithoutPrometheus(t *testing.T) {
	withPrometheus(t, false)
	svc := newDetailTestService(t, nil)
	got, err := svc.GetClusterDetail(context.Background(), 0, "1h", nil, "proc")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if got.Source != "none" || got.Reason == "" || len(got.Panels) != 0 {
		t.Fatalf("unavailable payload = %+v", got)
	}
}

func TestDetailEventsDiffing(t *testing.T) {
	// 纯函数：启动时刻严格上升才记事件，成员数任意变化都记。
	// Pure function: only a strictly rising start time records a restart, any member-count
	// change records an event.
	events := detailEvents(
		[]promquery.Series{series("instance", "a:1", [2]float64{1, 5}, [2]float64{2, 5}, [2]float64{3, 9})},
		[]promquery.Series{series("cluster_id", "1", [2]float64{1, 3}, [2]float64{2, 3}, [2]float64{3, 3})},
	)
	if len(events) != 1 || events[0].Kind != "node_restart" || events[0].TsMs != 3 {
		t.Fatalf("events = %+v", events)
	}
	if got := detailEvents(nil, nil); len(got) != 0 {
		t.Fatalf("empty events = %+v", got)
	}
}
