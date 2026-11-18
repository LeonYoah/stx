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
	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// fakeHostInventory 回两台主机：t1 绑了 agent（有指标），t2 是 docker（不支持）。
// fakeHostInventory returns two hosts: t1 with a bound agent (has metrics) and t2 docker
// (unsupported).
type fakeHostInventory struct{}

func (fakeHostInventory) List(context.Context, *host.HostFilter) ([]*host.Host, int64, error) {
	hb := time.Date(2026, 11, 15, 11, 59, 0, 0, time.UTC)
	return []*host.Host{
		{ID: 1, Name: "t1", IPAddress: "10.66.0.2", HostType: host.HostTypeBareMetal, AgentID: "agent-1", AgentStatus: host.AgentStatusInstalled, CPUCores: 4, TotalMemory: 8 << 30, LastHeartbeat: &hb},
		{ID: 2, Name: "dock", IPAddress: "10.66.0.9", HostType: host.HostTypeDocker},
	}, 2, nil
}

// fakeHostNodes 只在集群 1 下放 host 1 的节点。
// fakeHostNodes puts host 1's node into cluster 1 only.
type fakeHostNodes struct{}

func (fakeHostNodes) GetNodes(_ context.Context, clusterID uint) ([]*cluster.NodeInfo, error) {
	if clusterID == 1 {
		return []*cluster.NodeInfo{{HostID: 1}}, nil
	}
	return []*cluster.NodeInfo{}, nil
}

func newHostDashboardService(queries *[]string) *HostDashboardService {
	svc := NewHostDashboardService(fakeHostInventory{}, fakeHostNodes{})
	svc.now = func() time.Time { return time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC) }
	svc.rangeQuery = func(_ context.Context, q string, _, _ time.Time, _ int) ([]promquery.Series, error) {
		if queries != nil {
			queryMu.Lock()
			*queries = append(*queries, q)
			queryMu.Unlock()
		}
		switch {
		case strings.Contains(q, "fds_allocated"):
			return nil, errors.New("fds query boom")
		case strings.Contains(q, "stx_host_cpu_seconds_total"):
			return []promquery.Series{
				{Metric: map[string]string{"host_id": "1", "mode": "user"}, Points: [][2]float64{{1000, 20}}},
				{Metric: map[string]string{"host_id": "1", "mode": "system"}, Points: [][2]float64{{1000, 10}}},
			}, nil
		case strings.Contains(q, "stx_host_net_"):
			return []promquery.Series{
				{Metric: map[string]string{"host_id": "1", "iface": "eth0"}, Points: [][2]float64{{1000, 1024}}},
			}, nil
		}
		return []promquery.Series{{Metric: map[string]string{"host_id": "1"}, Points: [][2]float64{{1000, 1}}}}, nil
	}
	svc.instantQuery = func(_ context.Context, _ string, _ time.Time) ([]promquery.Sample, error) {
		// 开机时间即时查询：host 1 返回 2026-11-01，host 2 无序列。
		// Boot-time instant query: host 1 reports 2026-11-01, host 2 has no series.
		return []promquery.Sample{
			{Metric: map[string]string{"host_id": "1"}, Value: float64(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).Unix())},
		}, nil
	}
	return svc
}

func TestHostDashboardPanelsAndSupport(t *testing.T) {
	withPrometheus(t, true)
	var queries []string
	svc := newHostDashboardService(&queries)
	got, err := svc.GetHostDashboard(context.Background(), 0, "1h", nil)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if got.Source != "prometheus" || len(got.Hosts) != 2 {
		t.Fatalf("meta = %s hosts %d", got.Source, len(got.Hosts))
	}
	supported, unsupported := got.Hosts[0], got.Hosts[1]
	if !supported.MetricsSupported || unsupported.MetricsSupported {
		t.Fatalf("support flags = %v / %v", supported.MetricsSupported, unsupported.MetricsSupported)
	}
	// 不支持的主机面板全空，支持的主机有真实面板内容。
	// The unsupported host's panels stay empty while the supported one carries real series.
	if len(unsupported.Panels) != 0 {
		t.Fatalf("unsupported panels = %d", len(unsupported.Panels))
	}
	if len(supported.Panels) != len(hostPanels) {
		t.Fatalf("panels = %d want %d", len(supported.Panels), len(hostPanels))
	}
	var cpu, net *DetailPanel
	for i := range supported.Panels {
		switch supported.Panels[i].Key {
		case "cpu":
			cpu = &supported.Panels[i]
		case "net":
			net = &supported.Panels[i]
		}
	}
	// CPU 面板已归一成百分比（非 idle 模式）。/ The cpu panel is normalized to percent
	// (non-idle modes only).
	if cpu == nil || cpu.Unit != "percent" || len(cpu.Series) != 2 || cpu.Series[0].Label != "system" && cpu.Series[0].Label != "user" {
		t.Fatalf("cpu panel = %+v", cpu)
	}
	// rx/tx 双查询共用 iface 维度 → 图例须带方向词防撞名。
	// rx/tx queries share the iface dimension — legends must carry the direction word.
	if net == nil || len(net.Series) != 2 || net.Series[0].Label != "rx eth0" || net.Series[1].Label != "tx eth0" {
		t.Fatalf("net panel = %+v", net)
	}
	// selector 用 host_id=~"…" 一次圈住可测主机（docker 主机不进选择器）。
	// The selector uses host_id=~"…" covering supported hosts only (the docker host is
	// never in it).
	joined := strings.Join(queries, "\n")
	if !strings.Contains(joined, `host_id=~"1"`) || strings.Contains(joined, `|"2"`) || strings.Contains(joined, `2|"`) || strings.Contains(joined, `~"2"`) {
		t.Fatalf("bad selectors:\n%s", joined)
	}
	// 扇出收敛：每台主机不再各发 13 条，全部主机共用 13 条查询。
	// Fan-out collapsed: the fixed ~13 queries are shared by all hosts instead of 13 each.
	wantTasks := 0
	for _, p := range hostPanels {
		wantTasks += len(p.queries)
	}
	if len(queries) != wantTasks {
		t.Fatalf("queries = %d want %d", len(queries), wantTasks)
	}
	// 开机时间进卡片头字段。/ Boot time lands on the entry for the header uptime line.
	if supported.BootTimeMs == nil || *supported.BootTimeMs != time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("boot_time_ms = %v", supported.BootTimeMs)
	}
	// fds 查询失败 → partial_errors 记一次。
	// The fds query failure lands once in partial_errors.
	if len(got.PartialErrors) != 1 || got.PartialErrors[0] != "fds" {
		t.Fatalf("partial_errors = %v", got.PartialErrors)
	}
}

func TestHostDashboardClusterFilter(t *testing.T) {
	withPrometheus(t, true)
	svc := newHostDashboardService(nil)
	got, err := svc.GetHostDashboard(context.Background(), 1, "1h", nil)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	// 集群 1 只有 host 1 的节点 → docker 主机被过滤掉。
	// Cluster 1 only has a node on host 1 — the docker host is filtered out.
	if len(got.Hosts) != 1 || got.Hosts[0].ID != 1 {
		t.Fatalf("hosts = %+v", got.Hosts)
	}
	got, err = svc.GetHostDashboard(context.Background(), 9, "1h", nil)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if len(got.Hosts) != 0 {
		t.Fatalf("empty cluster hosts = %+v", got.Hosts)
	}
}

func TestHostDashboardWithoutPrometheus(t *testing.T) {
	withPrometheus(t, false)
	svc := newHostDashboardService(nil)
	got, err := svc.GetHostDashboard(context.Background(), 0, "1h", nil)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	// 可观测关闭：清单照回、支持标记照算、面板空。
	// Observability off: inventory and support flags still come back, panels stay empty.
	if got.Source != "none" || got.Reason == "" || len(got.Hosts) != 2 || !got.Hosts[0].MetricsSupported || len(got.Hosts[0].Panels) != 0 {
		t.Fatalf("unavailable payload = %+v", got)
	}
}
