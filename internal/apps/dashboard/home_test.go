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
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/apps/monitoring"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

type fakeHomeJobs struct {
	summary *syncapp.HomeJobSummary
	err     error
	cluster uint
}

func (f *fakeHomeJobs) GetHomeJobSummary(_ context.Context, _ executionapp.Actor, clusterID uint, _ time.Time) (*syncapp.HomeJobSummary, error) {
	f.cluster = clusterID
	return f.summary, f.err
}

type fakeHomeMonitoring struct {
	health       *monitoring.ClusterHealthData
	alerts       *monitoring.AlertInstanceListData
	closedAlerts *monitoring.AlertInstanceListData
	alertFilter  *monitoring.AlertInstanceFilter
	alertErr     error
	closedErr    error
}

func (f *fakeHomeMonitoring) GetClustersHealthFor(context.Context, uint) (*monitoring.ClusterHealthData, error) {
	return f.health, nil
}

func (f *fakeHomeMonitoring) ListAlertInstances(_ context.Context, filter *monitoring.AlertInstanceFilter) (*monitoring.AlertInstanceListData, error) {
	f.alertFilter = filter
	// 「需要处理」抑制逻辑单独查已关闭告警，用独立返回便于区分两种查询。
	// The needs-attention suppression queries closed alerts separately; a dedicated fake return keeps them apart.
	if filter.Status == monitoring.AlertDisplayStatusClosed {
		return f.closedAlerts, f.closedErr
	}
	return f.alerts, f.alertErr
}

func TestHomeServiceCombinesBlocksAndSortsAttention(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	jobs := &fakeHomeJobs{summary: &syncapp.HomeJobSummary{Running: 3, Attention: []syncapp.HomeAttentionItem{
		{Kind: "cdc_delay", Severity: "warning", At: now.Add(-time.Minute)},
		{Kind: "job_failed", Severity: "critical", At: now.Add(-time.Hour)},
	}}}
	mon := &fakeHomeMonitoring{
		health: &monitoring.ClusterHealthData{Clusters: []*monitoring.ClusterHealthItem{
			{ClusterID: 1, HealthStatus: "healthy", TotalNodes: 3, OnlineNodes: 3},
			{ClusterID: 2, HealthStatus: "degraded", TotalNodes: 1, OnlineNodes: 1, ActiveAlerts: 2, CriticalAlerts: 1},
		}},
		alerts: &monitoring.AlertInstanceListData{Alerts: []*monitoring.AlertInstance{
			{AlertID: "a1", AlertName: "EndpointDown", Severity: monitoring.AlertSeverityCritical, FiringAt: now.Add(-2 * time.Minute)},
		}},
	}
	service := NewHomeService(jobs, mon)
	service.now = func() time.Time { return now }

	all := service.GetHomeSummary(context.Background(), executionapp.Actor{UserID: 1}, 0)
	if all.Clusters.Total != 2 || all.Clusters.Healthy != 1 || all.Clusters.Degraded != 1 || all.Clusters.NodesOnline != 4 {
		t.Fatalf("clusters = %+v", all.Clusters)
	}
	if all.Alerts.Active != 2 || all.Alerts.Critical != 1 || all.Jobs.Running != 3 {
		t.Fatalf("alerts = %+v jobs = %+v", all.Alerts, all.Jobs)
	}
	kinds := []string{}
	for _, item := range all.Attention {
		kinds = append(kinds, item.Kind)
	}
	if len(kinds) != 3 || kinds[0] != "alert" || kinds[1] != "job_failed" || kinds[2] != "cdc_delay" {
		t.Fatalf("attention order = %v, want critical newest first then warnings", kinds)
	}

	one := service.GetHomeSummary(context.Background(), executionapp.Actor{UserID: 1}, 2)
	if one.Clusters.Total != 1 || one.Clusters.NodesTotal != 1 || jobs.cluster != 2 || mon.alertFilter.ClusterID != "2" {
		t.Fatalf("cluster scope not applied: clusters=%+v jobs=%d alerts=%+v", one.Clusters, jobs.cluster, mon.alertFilter)
	}
}

// 「作业失败」告警被人工关闭后，同任务关闭时间点之前的失败条目不再出现在「需要处理」；
// 关闭后的新失败、别的任务、以及别的规则（如 CDC 延迟）关闭都不影响。
// Once a job-failed alert is closed, that task's earlier failures leave the strip; failures after the
// close, other tasks and other rules (like CDC delay) are unaffected.
func TestHomeServiceSuppressesJobFailuresCoveredByClosedAlert(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	closedAt := now.Add(-time.Hour)
	jobs := &fakeHomeJobs{summary: &syncapp.HomeJobSummary{Attention: []syncapp.HomeAttentionItem{
		{Kind: "job_failed", Severity: "critical", Title: "old-fail", TaskID: 15, JobID: 10, At: now.Add(-2 * time.Hour)},
		{Kind: "job_failed", Severity: "critical", Title: "new-fail", TaskID: 15, JobID: 11, At: now.Add(-30 * time.Minute)},
		{Kind: "job_failed", Severity: "critical", Title: "other-task", TaskID: 16, JobID: 20, At: now.Add(-2 * time.Hour)},
		{Kind: "cdc_delay", Severity: "warning", Title: "cdc", TaskID: 15, At: now.Add(-2 * time.Hour)},
	}}}
	mon := &fakeHomeMonitoring{
		health: &monitoring.ClusterHealthData{},
		alerts: &monitoring.AlertInstanceListData{},
		closedAlerts: &monitoring.AlertInstanceListData{Alerts: []*monitoring.AlertInstance{
			{
				AlertID: "sync:episode:1", RuleKey: syncalert.RuleJobFailed,
				Status: monitoring.AlertDisplayStatusClosed, ClosedAt: &closedAt,
				SourceRef: &monitoring.AlertInstanceSourceRef{TaskID: 15},
			},
			{
				AlertID: "sync:episode:2", RuleKey: "sync_cdc_delay_high",
				Status: monitoring.AlertDisplayStatusClosed, ClosedAt: &closedAt,
				SourceRef: &monitoring.AlertInstanceSourceRef{TaskID: 16},
			},
		}},
	}
	service := NewHomeService(jobs, mon)
	service.now = func() time.Time { return now }

	got := service.GetHomeSummary(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0)
	failed := map[string]bool{}
	for _, item := range got.Attention {
		if item.Kind == "job_failed" {
			failed[item.Title] = true
		}
	}
	if failed["old-fail"] || !failed["new-fail"] || !failed["other-task"] || len(failed) != 2 {
		t.Fatalf("suppression wrong, remaining job_failed = %v", failed)
	}
}

// 已关闭告警查询失败时条目原样保留，宁可多显示也不能吞掉真实失败。
// When the closed-alert lookup fails the items pass through: over-showing beats swallowing a real failure.
func TestHomeServiceKeepsJobFailuresWhenClosedLookupFails(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	jobs := &fakeHomeJobs{summary: &syncapp.HomeJobSummary{Attention: []syncapp.HomeAttentionItem{
		{Kind: "job_failed", Severity: "critical", Title: "old-fail", TaskID: 15, JobID: 10, At: now.Add(-2 * time.Hour)},
	}}}
	mon := &fakeHomeMonitoring{
		health:    &monitoring.ClusterHealthData{},
		alerts:    &monitoring.AlertInstanceListData{},
		closedErr: errors.New("boom"),
	}
	service := NewHomeService(jobs, mon)
	service.now = func() time.Time { return now }

	got := service.GetHomeSummary(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, 0)
	if len(got.Attention) != 1 || got.Attention[0].Kind != "job_failed" {
		t.Fatalf("job_failed item must survive a failed closed lookup: %+v", got.Attention)
	}
}

func TestHomeServiceReportsPartialErrors(t *testing.T) {
	service := NewHomeService(&fakeHomeJobs{err: errors.New("db down")},
		&fakeHomeMonitoring{health: &monitoring.ClusterHealthData{}, alertErr: errors.New("boom")})
	got := service.GetHomeSummary(context.Background(), executionapp.Actor{}, 0)
	if got.Jobs != nil || len(got.PartialErrors) != 2 || got.PartialErrors[0] != "jobs" || got.PartialErrors[1] != "alerts" {
		t.Fatalf("partial errors = %v jobs = %+v", got.PartialErrors, got.Jobs)
	}
	if got.Attention == nil {
		t.Fatal("attention must be an empty array, not null")
	}
}
