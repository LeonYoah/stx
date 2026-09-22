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

package diagnostics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newAutoPolicyTestService(t *testing.T, alertReader alertInstanceReader) (*Service, *Repository) {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(
		&SeatunnelErrorGroup{},
		&SeatunnelErrorEvent{},
		&ClusterInspectionReport{},
		&ClusterInspectionFinding{},
		&InspectionAutoPolicy{},
	); err != nil {
		t.Fatalf("auto migrate diagnostics models: %v", err)
	}

	repo := NewRepository(database)
	service := NewServiceWithRepository(repo, &fakeInspectionClusterReader{
		cluster: &cluster.Cluster{ID: 7, Name: "demo-cluster"},
		status: &cluster.ClusterStatusInfo{
			ClusterID:    7,
			ClusterName:  "demo-cluster",
			Status:       cluster.ClusterStatusRunning,
			HealthStatus: cluster.HealthStatusHealthy,
		},
	}, nil, alertReader)
	return service, repo
}

func intPtr(value int) *int {
	return &value
}

func TestAutoPolicyCheckerMatchJavaErrorCondition_matchesOOMAndMetaspace(t *testing.T) {
	checker := &AutoPolicyChecker{}

	matched, code := checker.matchJavaErrorCondition(
		InspectionConditionItem{
			Enabled:      true,
			TemplateCode: ConditionCodeJavaOOM,
		},
		"java.lang.outofmemoryerror",
		"java heap space",
	)
	if !matched || code != ConditionCodeJavaOOM {
		t.Fatalf("expected JAVA_OOM to match, got matched=%v code=%s", matched, code)
	}

	matched, code = checker.matchJavaErrorCondition(
		InspectionConditionItem{
			Enabled:      true,
			TemplateCode: ConditionCodeJavaMetaspace,
		},
		"java.lang.outofmemoryerror",
		"metaspace",
	)
	if !matched || code != ConditionCodeJavaMetaspace {
		t.Fatalf("expected JAVA_METASPACE to match, got matched=%v code=%s", matched, code)
	}
}

func TestServiceListBuiltinConditionTemplates_hidesUnsupportedTemplates(t *testing.T) {
	service, _ := newAutoPolicyTestService(t, nil)

	templates := service.ListBuiltinConditionTemplates()
	if len(templates) == 0 {
		t.Fatal("expected supported templates")
	}
	for _, template := range templates {
		if template == nil {
			continue
		}
		if !isConditionTemplateSupported(template.Code) {
			t.Fatalf("unexpected unsupported template in list: %s", template.Code)
		}
	}
}

func TestServiceCreateAutoPolicy_rejectsUnsupportedConditionTemplate(t *testing.T) {
	service, _ := newAutoPolicyTestService(t, nil)

	_, err := service.CreateAutoPolicy(context.Background(), 1, &CreateInspectionAutoPolicyRequest{
		Name:      "unsupported",
		ClusterID: 7,
		Conditions: InspectionConditionItems{
			{TemplateCode: InspectionConditionTemplateCode("NOT_SUPPORTED"), Enabled: true},
		},
	})
	if err == nil {
		t.Fatal("expected unsupported template to be rejected")
	}
}

func TestServiceCreateAutoPolicyUsesDefaultDiagnosticResourcesWithoutStarting(t *testing.T) {
	service, repo := newAutoPolicyTestService(t, nil)

	policy, err := service.CreateAutoPolicy(context.Background(), 1, &CreateInspectionAutoPolicyRequest{
		Name:      "scheduled-default-resources",
		ClusterID: 7,
		Conditions: InspectionConditionItems{
			{TemplateCode: ConditionCodeScheduled, Enabled: true},
		},
		AutoCreateTask: true,
	})
	if err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}
	if len(policy.TaskOptions.SelectedResources) != 5 || policy.TaskOptions.IncludeThreadDump || policy.TaskOptions.IncludeJVMDump {
		t.Fatalf("unexpected default auto-policy resources: %+v", policy.TaskOptions)
	}
	if containsDiagnosticResource(policy.TaskOptions.SelectedResources, DiagnosticResourceThreadDump) {
		t.Fatalf("thread dump should require explicit selection: %+v", policy.TaskOptions)
	}
	if policy.AutoStartTask {
		t.Fatal("new auto-policy should create the diagnostics task without starting it by default")
	}
	persisted, err := repo.GetAutoPolicyByID(context.Background(), policy.ID)
	if err != nil {
		t.Fatalf("GetAutoPolicyByID returned error: %v", err)
	}
	if persisted.AutoStartTask {
		t.Fatal("auto_start_task should remain false after persistence")
	}
}

func TestValidateAutoPolicyTaskOptionsForActorRequiresAdminForJVMDump(t *testing.T) {
	options := &DiagnosticTaskOptions{SelectedResources: []DiagnosticResourceCode{DiagnosticResourceJVMDump}}
	if err := validateAutoPolicyTaskOptionsForActor(executionapp.Actor{UserID: 8}, options); err == nil {
		t.Fatal("普通用户不应配置 JVM Dump / non-admin users must not configure JVM dumps")
	}
	if err := validateAutoPolicyTaskOptionsForActor(executionapp.Actor{UserID: 1, IsAdmin: true}, options); err != nil {
		t.Fatalf("管理员应允许配置 JVM Dump / admins should be allowed to configure JVM dumps: %v", err)
	}
}

func TestAutoPolicyCheckerCheckErrorSpikePolicies_triggersInspectionWhenThresholdBreached(t *testing.T) {
	service, repo := newAutoPolicyTestService(t, nil)
	policy := &InspectionAutoPolicy{
		ClusterID:       7,
		Name:            "error-spike",
		Enabled:         true,
		CooldownMinutes: 10,
		Conditions: InspectionConditionItems{
			{
				TemplateCode:          ConditionCodeErrorSpike,
				Enabled:               true,
				ThresholdOverride:     intPtr(3),
				WindowMinutesOverride: intPtr(5),
			},
		},
	}
	if err := repo.CreateAutoPolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}

	group := &SeatunnelErrorGroup{
		Fingerprint:        "oom-users",
		FingerprintVersion: DefaultFingerprintVersion,
		Title:              "users table write failed",
		ExceptionClass:     "java.lang.RuntimeException",
		SampleMessage:      "users batch failed",
		OccurrenceCount:    3,
		FirstSeenAt:        time.Date(2026, 3, 26, 2, 18, 0, 0, time.UTC),
		LastSeenAt:         time.Date(2026, 3, 26, 2, 20, 0, 0, time.UTC),
		LastClusterID:      7,
	}
	if err := repo.CreateErrorGroup(context.Background(), group); err != nil {
		t.Fatalf("CreateErrorGroup returned error: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := repo.CreateErrorEvent(context.Background(), &SeatunnelErrorEvent{
			ClusterID:      7,
			ErrorGroupID:   group.ID,
			Fingerprint:    group.Fingerprint,
			AgentID:        "agent-1",
			ExceptionClass: group.ExceptionClass,
			Message:        group.SampleMessage,
			OccurredAt:     time.Date(2026, 3, 26, 2, 18+i, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("CreateErrorEvent returned error: %v", err)
		}
	}

	checker := NewAutoPolicyChecker(repo, service)
	if err := checker.CheckErrorSpikePolicies(context.Background(), 7, time.Date(2026, 3, 26, 2, 20, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CheckErrorSpikePolicies returned error: %v", err)
	}

	reports, err := service.ListInspectionReports(context.Background(), &ClusterInspectionReportFilter{
		ClusterID: 7,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListInspectionReports returned error: %v", err)
	}
	if reports.Total != 1 || len(reports.Items) != 1 {
		t.Fatalf("expected 1 inspection report, got total=%d len=%d", reports.Total, len(reports.Items))
	}
	if !strings.Contains(reports.Items[0].AutoTriggerReason, string(ConditionCodeErrorSpike)) {
		t.Fatalf("expected auto trigger reason to mention %s, got %q", ConditionCodeErrorSpike, reports.Items[0].AutoTriggerReason)
	}
}

func TestAutoPolicyCheckerCheckScheduledPolicies_triggersInspectionOncePerCooldownWindow(t *testing.T) {
	service, repo := newAutoPolicyTestService(t, nil)
	policy := &InspectionAutoPolicy{
		ClusterID:       7,
		Name:            "scheduled",
		Enabled:         true,
		CooldownMinutes: 10,
		Conditions: InspectionConditionItems{
			{
				TemplateCode:     ConditionCodeScheduled,
				Enabled:          true,
				CronExprOverride: "* * * * *",
			},
		},
	}
	if err := repo.CreateAutoPolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}

	checker := NewAutoPolicyChecker(repo, service)
	now := time.Date(2026, 3, 26, 2, 15, 0, 0, time.UTC)

	if err := checker.CheckScheduledPolicies(context.Background(), 7, now); err != nil {
		t.Fatalf("CheckScheduledPolicies returned error: %v", err)
	}
	if err := checker.CheckScheduledPolicies(context.Background(), 7, now); err != nil {
		t.Fatalf("CheckScheduledPolicies second run returned error: %v", err)
	}

	reports, total, err := repo.ListInspectionReports(context.Background(), &ClusterInspectionReportFilter{
		ClusterID: 7,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListInspectionReports returned error: %v", err)
	}
	if total != 1 || len(reports) != 1 {
		t.Fatalf("expected exactly one inspection report, total=%d len=%d", total, len(reports))
	}
	if reports[0].TriggerSource != InspectionTriggerSourceAuto {
		t.Fatalf("expected auto trigger source, got %s", reports[0].TriggerSource)
	}
	if reports[0].RequestedBy != "auto-policy:1" {
		t.Fatalf("expected requester auto-policy:1, got %s", reports[0].RequestedBy)
	}
	if reports[0].AutoTriggerReason == "" || !strings.Contains(reports[0].AutoTriggerReason, "SCHEDULED") {
		t.Fatalf("expected schedule trigger reason, got %q", reports[0].AutoTriggerReason)
	}
}

func TestAutoPolicyCheckerCheckPrometheusPolicies_triggersInspectionWhenThresholdBreached(t *testing.T) {
	service, repo := newAutoPolicyTestService(t, &fakeInspectionAlertReader{
		data: &monitoringapp.AlertInstanceListData{
			Alerts: []*monitoringapp.AlertInstance{
				{
					AlertID:     "a-1",
					ClusterID:   "7",
					RuleKey:     "memory_usage_high",
					AlertName:   "Heap memory usage high",
					Status:      monitoringapp.AlertDisplayStatusFiring,
					LastSeenAt:  time.Now().UTC(),
					CreatedAt:   time.Now().UTC(),
					FiringAt:    time.Now().UTC(),
					ClusterName: "demo-cluster",
				},
			},
		},
	})
	policy := &InspectionAutoPolicy{
		ClusterID:       7,
		Name:            "heap-high",
		Enabled:         true,
		CooldownMinutes: 5,
		Conditions: InspectionConditionItems{
			{
				TemplateCode: ConditionCodePromHeapHigh,
				Enabled:      true,
			},
		},
	}
	if err := repo.CreateAutoPolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}

	checker := NewAutoPolicyChecker(repo, service)
	if err := checker.CheckPrometheusPolicies(context.Background(), 7, time.Date(2026, 3, 26, 2, 20, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CheckPrometheusPolicies returned error: %v", err)
	}

	reports, total, err := repo.ListInspectionReports(context.Background(), &ClusterInspectionReportFilter{
		ClusterID: 7,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListInspectionReports returned error: %v", err)
	}
	if total != 1 || len(reports) != 1 {
		t.Fatalf("expected one prometheus-triggered inspection report, total=%d len=%d", total, len(reports))
	}
	if reports[0].AutoTriggerReason == "" || !strings.Contains(reports[0].AutoTriggerReason, "PROM_HEAP_HIGH") {
		t.Fatalf("expected PROM_HEAP_HIGH trigger reason, got %q", reports[0].AutoTriggerReason)
	}
}

func TestAutoPolicyCheckerCheckPrometheusPolicies_triggersInspectionForNodeUnhealthy(t *testing.T) {
	service, repo := newAutoPolicyTestService(t, &fakeInspectionAlertReader{
		data: &monitoringapp.AlertInstanceListData{
			Alerts: []*monitoringapp.AlertInstance{
				{
					AlertID:     "node-a",
					ClusterID:   "7",
					RuleKey:     "node_offline",
					AlertName:   "Node offline",
					Status:      monitoringapp.AlertDisplayStatusFiring,
					FiringAt:    time.Date(2026, 3, 26, 2, 0, 0, 0, time.UTC),
					SourceRef:   &monitoringapp.AlertInstanceSourceRef{Hostname: "host-a"},
					ClusterName: "demo-cluster",
				},
				{
					AlertID:     "node-b",
					ClusterID:   "7",
					RuleKey:     "node_offline",
					AlertName:   "Node offline",
					Status:      monitoringapp.AlertDisplayStatusFiring,
					FiringAt:    time.Date(2026, 3, 26, 2, 1, 0, 0, time.UTC),
					SourceRef:   &monitoringapp.AlertInstanceSourceRef{Hostname: "host-b"},
					ClusterName: "demo-cluster",
				},
			},
		},
	})
	policy := &InspectionAutoPolicy{
		ClusterID:       7,
		Name:            "node-unhealthy",
		Enabled:         true,
		CooldownMinutes: 5,
		Conditions: InspectionConditionItems{
			{
				TemplateCode:          ConditionCodeNodeUnhealthy,
				Enabled:               true,
				ThresholdOverride:     intPtr(2),
				WindowMinutesOverride: intPtr(10),
			},
		},
	}
	if err := repo.CreateAutoPolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}

	checker := NewAutoPolicyChecker(repo, service)
	if err := checker.CheckPrometheusPolicies(context.Background(), 7, time.Date(2026, 3, 26, 2, 20, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CheckPrometheusPolicies returned error: %v", err)
	}

	reports, total, err := repo.ListInspectionReports(context.Background(), &ClusterInspectionReportFilter{
		ClusterID: 7,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListInspectionReports returned error: %v", err)
	}
	if total != 1 || len(reports) != 1 {
		t.Fatalf("expected one node-unhealthy inspection report, total=%d len=%d", total, len(reports))
	}
	if !strings.Contains(reports[0].AutoTriggerReason, string(ConditionCodeNodeUnhealthy)) {
		t.Fatalf("expected trigger reason to mention %s, got %q", ConditionCodeNodeUnhealthy, reports[0].AutoTriggerReason)
	}
}

func TestAutoPolicyCheckerCheckPrometheusPolicies_triggersInspectionForAlertFiring(t *testing.T) {
	service, repo := newAutoPolicyTestService(t, &fakeInspectionAlertReader{
		data: &monitoringapp.AlertInstanceListData{
			Alerts: []*monitoringapp.AlertInstance{
				{
					AlertID:     "alert-1",
					ClusterID:   "7",
					RuleKey:     "custom_rule_users_delay",
					AlertName:   "Users delay high",
					Summary:     "Users delay high",
					Status:      monitoringapp.AlertDisplayStatusFiring,
					FiringAt:    time.Date(2026, 3, 26, 2, 19, 0, 0, time.UTC),
					ClusterName: "demo-cluster",
				},
			},
		},
	})
	policy := &InspectionAutoPolicy{
		ClusterID:       7,
		Name:            "alert-firing",
		Enabled:         true,
		CooldownMinutes: 5,
		Conditions: InspectionConditionItems{
			{
				TemplateCode:  ConditionCodeAlertFiring,
				Enabled:       true,
				ExtraKeywords: []string{"users_delay"},
			},
		},
	}
	if err := repo.CreateAutoPolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}

	checker := NewAutoPolicyChecker(repo, service)
	if err := checker.CheckPrometheusPolicies(context.Background(), 7, time.Date(2026, 3, 26, 2, 20, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CheckPrometheusPolicies returned error: %v", err)
	}

	reports, total, err := repo.ListInspectionReports(context.Background(), &ClusterInspectionReportFilter{
		ClusterID: 7,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListInspectionReports returned error: %v", err)
	}
	if total != 1 || len(reports) != 1 {
		t.Fatalf("expected one alert-firing inspection report, total=%d len=%d", total, len(reports))
	}
	if !strings.Contains(reports[0].AutoTriggerReason, string(ConditionCodeAlertFiring)) {
		t.Fatalf("expected trigger reason to mention %s, got %q", ConditionCodeAlertFiring, reports[0].AutoTriggerReason)
	}
}
