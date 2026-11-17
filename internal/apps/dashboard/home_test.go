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
	health      *monitoring.ClusterHealthData
	alerts      *monitoring.AlertInstanceListData
	alertFilter *monitoring.AlertInstanceFilter
	alertErr    error
}

func (f *fakeHomeMonitoring) GetClustersHealthFor(context.Context, uint) (*monitoring.ClusterHealthData, error) {
	return f.health, nil
}

func (f *fakeHomeMonitoring) ListAlertInstances(_ context.Context, filter *monitoring.AlertInstanceFilter) (*monitoring.AlertInstanceListData, error) {
	f.alertFilter = filter
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
