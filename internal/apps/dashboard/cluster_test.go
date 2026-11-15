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

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

type fakeClusterNodes struct{}

func (fakeClusterNodes) List(context.Context, *cluster.ClusterFilter) ([]*cluster.Cluster, int64, error) {
	return []*cluster.Cluster{{ID: 2, Name: "b", Version: "3.0.0"}, {ID: 1, Name: "a", Version: "3.0.0"}}, 2, nil
}

func (fakeClusterNodes) GetNodes(_ context.Context, clusterID uint) ([]*cluster.NodeInfo, error) {
	if clusterID == 1 {
		return []*cluster.NodeInfo{
			{ID: 11, HostIP: "10.0.0.1", HazelcastPort: 5801, Role: cluster.NodeRoleMaster, Status: "running", IsOnline: true},
			{ID: 12, HostIP: "10.0.0.2", HazelcastPort: 5801, Role: cluster.NodeRoleWorker, Status: "running", IsOnline: true},
		}, nil
	}
	return []*cluster.NodeInfo{{ID: 21, HostIP: "10.0.1.1", HazelcastPort: 5802, Role: cluster.NodeRoleMasterWorker, IsOnline: false}}, nil
}

type fakeHealth struct{}

func (fakeHealth) GetClustersHealth(context.Context) (*monitoring.ClusterHealthData, error) {
	return &monitoring.ClusterHealthData{Clusters: []*monitoring.ClusterHealthItem{
		{ClusterID: 1, HealthStatus: "healthy", TotalNodes: 2, OnlineNodes: 2},
		{ClusterID: 2, HealthStatus: "unhealthy", TotalNodes: 1, ActiveAlerts: 2, CriticalAlerts: 1},
	}}, nil
}

type fakeRunning struct{}

func (fakeRunning) RunningJobCountsByCluster(context.Context) (map[uint]int64, error) {
	return map[uint]int64{1: 3}, nil
}

func withPrometheus(t *testing.T, enabled bool) {
	t.Helper()
	oldEnabled, oldURL := config.Config.Observability.Enabled, config.Config.Observability.Prometheus.URL
	t.Cleanup(func() {
		config.Config.Observability.Enabled, config.Config.Observability.Prometheus.URL = oldEnabled, oldURL
	})
	config.Config.Observability.Enabled, config.Config.Observability.Prometheus.URL = enabled, "http://prom.test"
}

func series(label, value string, points ...[2]float64) promquery.Series {
	return promquery.Series{Metric: map[string]string{label: value}, Points: points}
}

func sample(value float64, labels ...string) promquery.Sample {
	metric := map[string]string{}
	for i := 0; i+1 < len(labels); i += 2 {
		metric[labels[i]] = labels[i+1]
	}
	return promquery.Sample{Metric: metric, Value: value}
}

func newTestClusterService(queries *[]string) *ClusterDashboardService {
	svc := NewClusterDashboardService(fakeHealth{}, fakeClusterNodes{}, fakeRunning{})
	svc.now = func() time.Time { return time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC) }
	svc.rangeQuery = func(_ context.Context, _, q string, _, _ time.Time, _ int) ([]promquery.Series, error) {
		*queries = append(*queries, q)
		switch {
		case strings.Contains(q, "min_over_time(up{"):
			return []promquery.Series{
				series("instance", "10.0.0.1:5801", [2]float64{1000, 1}, [2]float64{2000, 1}),
				series("instance", "10.0.0.2:5801", [2]float64{1000, 0}, [2]float64{2000, 1}),
			}, nil
		case strings.Contains(q, "node_state"):
			return []promquery.Series{series("instance", "10.0.0.1:5801", [2]float64{1000, 1}, [2]float64{2000, 0})}, nil
		case strings.Contains(q, "job_count"):
			return []promquery.Series{series("type", "running", [2]float64{1000, 3})}, nil
		case strings.Contains(q, "jvm_gc_collection"):
			return nil, errors.New("boom")
		}
		return []promquery.Series{series("instance", "10.0.0.1:5801", [2]float64{1000, 42})}, nil
	}
	svc.instantQuery = func(_ context.Context, _, q string, _ time.Time) ([]promquery.Sample, error) {
		*queries = append(*queries, q)
		switch {
		case strings.Contains(q, "max by (cluster_id, instance) (up{"):
			return []promquery.Sample{sample(1, "instance", "10.0.0.1:5801", "cluster_id", "1"), sample(0, "instance", "10.0.0.2:5801", "cluster_id", "1"), sample(0, "instance", "10.0.1.1:5802", "cluster_id", "2")}, nil
		case strings.Contains(q, "cluster_info"):
			return []promquery.Sample{sample(1, "master", "10.0.0.1:5801")}, nil
		case strings.Contains(q, "isClusterSafe"):
			return []promquery.Sample{sample(1, "cluster_id", "1")}, nil
		case strings.Contains(q, "jvm_memory_bytes_used"):
			return []promquery.Sample{sample(71.5, "instance", "10.0.0.1:5801", "cluster_id", "1"), sample(40, "instance", "10.0.0.2:5801", "cluster_id", "1"), sample(55, "instance", "10.0.1.1:5802", "cluster_id", "2")}, nil
		case strings.Contains(q, "jvm_threads_deadlocked"):
			return []promquery.Sample{sample(0)}, nil
		}
		return nil, nil
	}
	return svc
}

func TestClusterDashboardCombinesInventoryAndMetrics(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newTestClusterService(&queries)
	got, err := svc.GetClusterDashboard(context.Background(), 0, "1h", nil)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if got.Source != "prometheus" || got.StepSeconds != 15 || got.CellSeconds != 75 {
		t.Fatalf("window = %s step %d cell %d, want prometheus / 15 / 75 (1h / 48)", got.Source, got.StepSeconds, got.CellSeconds)
	}
	if len(got.Clusters) != 2 || got.Clusters[0].ID != 1 || got.Clusters[0].RunningJobs != 3 || got.Clusters[1].HealthStatus != "unhealthy" {
		t.Fatalf("clusters = %+v", got.Clusters)
	}
	if got.Clusters[0].MaxHeapPercent == nil || *got.Clusters[0].MaxHeapPercent != 71.5 || *got.Clusters[0].EndpointsDown != 1 {
		t.Fatalf("card metrics = %+v", got.Clusters[0])
	}
	master, worker := got.Nodes[0], got.Nodes[1]
	if master.Instance != "10.0.0.1:5801" || master.Master == nil || !*master.Master || *master.ScrapeUp != true {
		t.Fatalf("master row = %+v", master)
	}
	if worker.Master == nil || *worker.Master || *worker.ScrapeUp != false || *worker.HeapPercent != 40 {
		t.Fatalf("worker row = %+v", worker)
	}
	// 抓取正常但 node_state=0 也算不可用；没有 node_state 时只看 up。
	// Up but node_state=0 still counts as unavailable; without node_state only up matters.
	avail := map[string][][2]float64{}
	for _, item := range got.Series {
		if item.Key == "availability" {
			avail[item.Label] = item.Points
		}
	}
	if avail["10.0.0.1:5801"][1][1] != 0 || avail["10.0.0.1:5801"][0][1] != 1 || avail["10.0.0.2:5801"][0][1] != 0 || avail["10.0.0.2:5801"][1][1] != 1 {
		t.Fatalf("availability = %v", avail)
	}
	if got.Signals == nil || got.Signals.ClustersSafe != 1 || got.Signals.EndpointsDown != 2 || *got.Signals.DeadlockedThreads != 0 {
		t.Fatalf("signals = %+v", got.Signals)
	}
	if len(got.PartialErrors) != 1 || got.PartialErrors[0] != "gc" {
		t.Fatalf("partial errors = %v, want only gc", got.PartialErrors)
	}
	for _, q := range queries {
		if strings.Contains(q, "seatunnel:") {
			t.Fatalf("query %q depends on a recording rule", q)
		}
	}
}

func TestClusterDashboardScopesToOneCluster(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	got, err := newTestClusterService(&queries).GetClusterDashboard(context.Background(), 2, "15m", nil)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	// 卡片仍是全部集群（可切换），节点与查询只含集群 2；卡片指标的两条即时查询不限范围。
	// Cards still list every cluster (switchable); nodes and queries cover only cluster 2, except the two unscoped card
	// instants.
	if len(got.Clusters) != 2 || len(got.Nodes) != 1 || got.Nodes[0].Instance != "10.0.1.1:5802" {
		t.Fatalf("scoped = %+v / %+v", got.Clusters, got.Nodes)
	}
	if got.Clusters[0].MaxHeapPercent == nil || *got.Clusters[0].MaxHeapPercent != 71.5 || *got.Clusters[1].MaxHeapPercent != 55 {
		t.Fatalf("card heap must cover every cluster: %+v", got.Clusters)
	}
	unscoped := 0
	for _, q := range queries {
		if !strings.Contains(q, `cluster_id="2"`) {
			unscoped++
		}
	}
	if unscoped != 2 {
		t.Fatalf("unscoped queries = %d, want 2 (up_now, heap_all)", unscoped)
	}
}

func TestClusterDashboardWithoutPrometheus(t *testing.T) {
	withPrometheus(t, false)
	var queries []string
	got, err := newTestClusterService(&queries).GetClusterDashboard(context.Background(), 0, "", nil)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if got.Source != "none" || got.Reason != "observability_disabled" || got.Signals != nil || len(got.Series) != 0 || len(queries) != 0 {
		t.Fatalf("no-prometheus dashboard = %+v (queries %d)", got, len(queries))
	}
	if len(got.Clusters) != 2 || len(got.Nodes) != 3 || got.Nodes[0].HeapPercent != nil {
		t.Fatalf("inventory must still load: %+v", got.Nodes)
	}
	if _, err := newTestClusterService(&queries).GetClusterDashboard(context.Background(), 0, "2h", nil); !errors.Is(err, ErrInvalidClusterDashboardQuery) {
		t.Fatalf("bad range err = %v", err)
	}
}
