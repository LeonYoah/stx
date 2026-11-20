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
package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// stubPlatformHostLister 是测试用的 hosts 表替身。
// stubPlatformHostLister is a hosts-table stub for tests.
type stubPlatformHostLister struct {
	hosts   []*host.Host
	timeout time.Duration
	since   time.Time
}

func (s *stubPlatformHostLister) List(ctx context.Context, filter *host.HostFilter) ([]*host.Host, int64, error) {
	return s.hosts, int64(len(s.hosts)), nil
}

func (s *stubPlatformHostLister) GetHeartbeatTimeout() time.Duration { return s.timeout }
func (s *stubPlatformHostLister) GetProcessStartedAt() time.Time     { return s.since }

// TestGetPlatformSnapshot_Unavailable 锁定 observability 关闭时的降级契约：
// available=false + reason，不报错、agents 为空数组。
// TestGetPlatformSnapshot_Unavailable locks the degradation contract while
// observability is off: available=false + reason, no error, empty agents array.
func TestGetPlatformSnapshot_Unavailable(t *testing.T) {
	original := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = original })
	config.Config.Observability.Enabled = false

	service := NewService(nil, nil, nil)
	snap, err := service.GetPlatformSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetPlatformSnapshot: %v", err)
	}
	if snap.Available {
		t.Fatal("expected available=false")
	}
	if snap.Reason == "" {
		t.Fatal("expected reason")
	}
	if snap.Agents == nil || len(snap.Agents) != 0 {
		t.Fatalf("agents should be an empty array, got %#v", snap.Agents)
	}
}

// TestGetPlatformSnapshot_MergesHostsAndMetrics 验证以 hosts 表为主合并指标：
// 有指标的 Agent 字段齐全，旧版/无指标 Agent 字段为 nil 且仍出现在列表中。
// TestGetPlatformSnapshot_MergesHostsAndMetrics merges metrics onto the hosts table:
// instrumented agents carry values while legacy/uninstrumented agents keep nil
// fields but still show up.
func TestGetPlatformSnapshot_MergesHostsAndMetrics(t *testing.T) {
	original := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = original })
	config.Config.Observability.Enabled = true

	ctx := context.Background()
	if err := embedts.Start(ctx, embedts.Options{Dir: t.TempDir()}); err != nil {
		t.Fatalf("start embedded tsdb: %v", err)
	}
	t.Cleanup(func() { _ = embedts.Close() })

	recent := time.Now().Add(-5 * time.Second)
	stale := time.Now().Add(-time.Hour)
	service := NewService(nil, nil, nil)
	service.SetPlatformHostLister(&stubPlatformHostLister{
		timeout: 30 * time.Second,
		hosts: []*host.Host{
			{ID: 7, Name: "agent-a", HostType: host.HostTypeBareMetal, AgentVersion: "1.0.0", LastHeartbeat: &recent},
			{ID: 8, Name: "agent-old", HostType: host.HostTypeBareMetal, AgentVersion: "0.9.0", LastHeartbeat: &stale},
		},
	})

	// 写入一轮 agent 自指标（与 agent 端上报通道相同的写入路径）。
	// Write one round of agent self-metrics through the same write path the agent
	// report channel uses.
	if _, err := host.WriteHostMetrics(ctx, 7, "agent-a", "127.0.0.1:8001", []host.HostMetricSample{
		{Name: "stx_agent_process_resident_memory_bytes", Value: 4096},
		{Name: "stx_agent_goroutines", Value: 12},
		{Name: "stx_agent_uptime_seconds", Value: 60},
		{Name: "stx_agent_build_info", Labels: map[string]string{"version": "1.0.0"}, Value: 1},
		{Name: "stx_agent_commands_total", Labels: map[string]string{"type": "INSTALL", "result": "failed"}, Value: 2},
	}); err != nil {
		t.Fatalf("write agent metrics: %v", err)
	}
	// 自抓取一轮，让 stx_server_/go_/process_ 指标落盘。
	// Run one self-scrape so stx_server_/go_/process_ metrics land on disk.
	if err := embedts.SelfScrapeOnce(ctx); err != nil {
		t.Fatalf("self scrape: %v", err)
	}

	snap, err := service.GetPlatformSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetPlatformSnapshot: %v", err)
	}
	if !snap.Available {
		t.Fatalf("expected available, got reason %q", snap.Reason)
	}
	if snap.Server == nil {
		t.Fatal("missing server snapshot")
	}
	if snap.Server.RSSBytes <= 0 || snap.Server.Goroutines <= 0 || snap.Server.UptimeSeconds <= 0 {
		t.Fatalf("server process metrics missing: %+v", snap.Server)
	}
	if len(snap.Agents) != 2 {
		t.Fatalf("agents = %+v", snap.Agents)
	}

	online := snap.Agents[0]
	if online.HostID != 7 {
		t.Fatalf("agents not sorted by host_id: %+v", snap.Agents)
	}
	if !online.Online || online.Version != "1.0.0" {
		t.Fatalf("online agent = %+v", online)
	}
	if online.RSSBytes == nil || *online.RSSBytes != 4096 {
		t.Fatalf("rss = %+v", online.RSSBytes)
	}
	if online.Goroutines == nil || *online.Goroutines != 12 {
		t.Fatalf("goroutines = %+v", online.Goroutines)
	}
	if online.UptimeSeconds == nil || *online.UptimeSeconds != 60 {
		t.Fatalf("uptime = %+v", online.UptimeSeconds)
	}
	if online.LastReportAt == nil {
		t.Fatal("expected last_report_at")
	}

	legacy := snap.Agents[1]
	if legacy.HostID != 8 || legacy.Online {
		t.Fatalf("legacy agent = %+v", legacy)
	}
	if legacy.RSSBytes != nil || legacy.UptimeSeconds != nil || legacy.LastReportAt != nil {
		t.Fatalf("legacy agent should keep nil metrics: %+v", legacy)
	}
}
