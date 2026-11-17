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
package host

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql"

	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// instantVector 对内嵌 tsdb 跑一次即时查询，返回命中的瞬时向量。
// instantVector runs one instant query against the embedded tsdb and returns the vector.
func instantVector(t *testing.T, qs string, ts time.Time) promql.Vector {
	t.Helper()
	engine, queryable := embedts.QueryEngine()
	if engine == nil {
		t.Fatal("embedded tsdb not ready")
	}
	qry, err := engine.NewInstantQuery(context.Background(), queryable, nil, qs, ts)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}
	res := qry.Exec(context.Background())
	if res.Err != nil {
		t.Fatalf("query %s: %v", qs, res.Err)
	}
	vec, ok := res.Value.(promql.Vector)
	if !ok {
		t.Fatalf("query %s: not a vector", qs)
	}
	return vec
}

func TestWriteHostMetricsRoundTripAndStale(t *testing.T) {
	if err := embedts.Start(context.Background(), embedts.Options{Dir: t.TempDir()}); err != nil {
		t.Fatalf("start embedded tsdb: %v", err)
	}
	t.Cleanup(func() {
		hostTrackers.Delete(uint(7))
		_ = embedts.Close()
	})
	ctx := context.Background()

	written, err := WriteHostMetrics(ctx, 7, "t1", "10.0.0.1:7001", []HostMetricSample{
		{Name: "stx_host_cpu_seconds_total", Labels: map[string]string{"mode": "user"}, Value: 1.5},
		{Name: "stx_host_net_receive_bytes_total", Labels: map[string]string{"iface": "eth0"}, Value: 100},
		{Name: "not_host_metric", Value: 1},                                                // 名字不对 → 丢弃 / bad name → dropped
		{Name: "stx_host_mem_bytes", Value: math.NaN()},                                    // NaN → 丢弃 / NaN → dropped
		{Name: "stx_host_load1", Labels: map[string]string{"host_id": "999"}, Value: 0.42}, // 身份标签注入被忽略 / identity-label injection ignored
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if written != 3 {
		t.Fatalf("written = %d want 3", written)
	}

	at := time.Now().Add(30 * time.Second)
	vec := instantVector(t, `stx_host_cpu_seconds_total{host_id="7",mode="user"}`, at)
	if len(vec) != 1 || vec[0].F != 1.5 {
		t.Fatalf("cpu = %+v", vec)
	}
	vec = instantVector(t, `stx_host_load1`, at)
	if len(vec) != 1 || vec[0].Metric.Get("host_id") != "7" || vec[0].Metric.Get("job") != "stx_host" {
		t.Fatalf("load1 labels = %+v", vec)
	}

	// 第二轮只报 cpu → net 序列消失应被补 stale marker。
	// Round two reports cpu only — the vanished net series gets a stale marker.
	if _, err := WriteHostMetrics(ctx, 7, "t1", "10.0.0.1:7001", []HostMetricSample{
		{Name: "stx_host_cpu_seconds_total", Labels: map[string]string{"mode": "user"}, Value: 2.0},
	}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	at = time.Now().Add(30 * time.Second)
	if vec := instantVector(t, `stx_host_net_receive_bytes_total{host_id="7"}`, at); len(vec) != 0 {
		t.Fatalf("vanished series still visible: %+v", vec)
	}
	if vec := instantVector(t, `stx_host_cpu_seconds_total{host_id="7"}`, at); len(vec) != 1 || vec[0].F != 2.0 {
		t.Fatalf("cpu after round 2 = %+v", vec)
	}

	// 主机离线 → 全部序列 stale。
	// Host offline → every series goes stale.
	MarkHostMetricsStale(ctx, 7)
	at = time.Now().Add(30 * time.Second)
	if vec := instantVector(t, `stx_host_cpu_seconds_total{host_id="7"}`, at); len(vec) != 0 {
		t.Fatalf("offline host series still visible: %+v", vec)
	}
}

func TestWriteHostMetricsNoStore(t *testing.T) {
	_ = embedts.Close()
	written, err := WriteHostMetrics(context.Background(), 1, "h", "h:1", []HostMetricSample{{Name: "stx_host_load1", Value: 1}})
	if err != nil || written != 0 {
		t.Fatalf("no-store write = %d, %v", written, err)
	}
}
