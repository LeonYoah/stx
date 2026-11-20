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
package collector

import (
	"context"
	"math"
	"testing"
	"time"

	pb "github.com/LeonYoah/stx/agent"
)

// collectOnce 真采一轮（本机环境），返回 name → samples 索引。
// collectOnce runs one real collection round on this machine and indexes samples by name.
func collectOnce(t *testing.T) map[string][]string {
	t.Helper()
	c := NewMetricsCollector(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	samples := c.CollectHostMetrics(ctx)
	if len(samples) == 0 {
		t.Fatal("no host samples collected")
	}
	seen := map[string][]string{}
	for _, s := range samples {
		if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
			t.Fatalf("non-finite sample %s %v", s.Name, s.Labels)
		}
		seen[s.Name] = append(seen[s.Name], s.Labels["mode"]+s.Labels["kind"]+s.Labels["device"]+s.Labels["iface"]+s.Labels["mountpoint"])
	}
	return seen
}

func TestCollectHostMetricsEmitsCoreSeries(t *testing.T) {
	seen := collectOnce(t)
	for _, name := range []string{
		"stx_host_cpu_seconds_total",
		"stx_host_cpu_cores",
		"stx_host_load",
		"stx_host_memory_bytes",
		"stx_host_boot_time_seconds",
	} {
		if len(seen[name]) == 0 {
			t.Fatalf("missing core series %s", name)
		}
	}
	// CPU 至少应有 user+idle 两种 mode。/ CPU should carry at least user+idle modes.
	modes := map[string]bool{}
	for _, dim := range seen["stx_host_cpu_seconds_total"] {
		modes[dim] = true
	}
	if !modes["user"] || !modes["idle"] {
		t.Fatalf("cpu modes = %v", modes)
	}
	// 负载三维齐全。/ All three load kinds present.
	if len(seen["stx_host_load"]) != 3 {
		t.Fatalf("load kinds = %v", seen["stx_host_load"])
	}
}

// collectSamples 采一轮并按 name 索引完整样本（自指标断言需要读标签与值）。
// collectSamples runs one round and indexes full samples by name (self-metric
// assertions need labels and values).
func collectSamples(t *testing.T, c *MetricsCollector) map[string][]*pb.HostMetricSample {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	samples := c.CollectHostMetrics(ctx)
	if len(samples) == 0 {
		t.Fatal("no host samples collected")
	}
	seen := map[string][]*pb.HostMetricSample{}
	for _, s := range samples {
		seen[s.Name] = append(seen[s.Name], s)
	}
	return seen
}

// TestCollectAgentSelfEmitsSelfSeries 锁定 stx_agent_* 自指标随主机指标同轮上报。
// TestCollectAgentSelfEmitsSelfSeries locks stx_agent_* self-metrics riding the same
// round as host metrics.
func TestCollectAgentSelfEmitsSelfSeries(t *testing.T) {
	c := NewMetricsCollector(nil)
	c.SetSelfVersion("9.9.9-test")
	c.RecordHeartbeatFailure()
	c.RecordCommandResult("INSTALL", "success")
	c.RecordCommandResult("INSTALL", "failed")
	c.RecordCommandResult("PRECHECK", "success")

	seen := collectSamples(t, c)
	for _, name := range []string{
		"stx_agent_heap_inuse_bytes",
		"stx_agent_goroutines",
		"stx_agent_uptime_seconds",
		"stx_agent_build_info",
		"stx_agent_heartbeat_failures_total",
		"stx_agent_commands_total",
	} {
		if len(seen[name]) == 0 {
			t.Fatalf("missing self series %s", name)
		}
	}

	// build_info 必须带 version 标签且恒为 1。
	// build_info must carry the version label and stay constant 1.
	build := seen["stx_agent_build_info"][0]
	if build.Labels["version"] != "9.9.9-test" || build.Value != 1 {
		t.Fatalf("build_info = %v %+v", build.Value, build.Labels)
	}
	if seen["stx_agent_heartbeat_failures_total"][0].Value != 1 {
		t.Fatalf("heartbeat failures = %v", seen["stx_agent_heartbeat_failures_total"][0].Value)
	}
	if seen["stx_agent_goroutines"][0].Value <= 0 || seen["stx_agent_uptime_seconds"][0].Value < 0 {
		t.Fatalf("runtime series invalid: %+v", seen)
	}

	// 命令结果按 {type,result} 维度展开，累计值正确。
	// Command outcomes expand over {type,result} with the right totals.
	got := map[string]float64{}
	for _, s := range seen["stx_agent_commands_total"] {
		got[s.Labels["type"]+"/"+s.Labels["result"]] = s.Value
	}
	if got["INSTALL/success"] != 1 || got["INSTALL/failed"] != 1 || got["PRECHECK/success"] != 1 {
		t.Fatalf("command totals = %v", got)
	}
}

func TestCollectHostMetricsVirtualDevicesFiltered(t *testing.T) {
	seen := collectOnce(t)
	for _, name := range []string{"stx_host_net_receive_bytes_total", "stx_host_disk_read_bytes_total"} {
		for _, dim := range seen[name] {
			for _, bad := range []string{"lo", "docker", "veth", "br-", "loop", "ram", "dm-", "zram"} {
				if dim == bad {
					t.Fatalf("%s carried virtual device %q", name, bad)
				}
			}
		}
	}
}
