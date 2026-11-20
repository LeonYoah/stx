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

// TestServiceCreateAutoPolicy_validatesAlertFiringCondition 覆盖 ALERT_FIRING 的全部 400 分支：
// 缺规则键、非法规则键、非法阈值 / 窗口，以及正常创建路径。
// TestServiceCreateAutoPolicy_validatesAlertFiringCondition covers every 400 branch of
// ALERT_FIRING: missing rule keys, unknown rule keys, invalid threshold/window and the happy path.
func TestServiceCreateAutoPolicy_validatesAlertFiringCondition(t *testing.T) {
	service, _ := newAutoPolicyTestService(t, nil)
	base := &CreateInspectionAutoPolicyRequest{
		Name:      "alert-firing",
		ClusterID: 7,
		Conditions: InspectionConditionItems{
			{TemplateCode: ConditionCodeAlertFiring, Enabled: true},
		},
	}
	cases := []struct {
		name      string
		condition InspectionConditionItem
		wantErr   string
	}{
		{name: "missing rule keys", condition: InspectionConditionItem{TemplateCode: ConditionCodeAlertFiring, Enabled: true}, wantErr: "at least one rule key"},
		{name: "blank rule keys", condition: InspectionConditionItem{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{" ", ""}}, wantErr: "at least one rule key"},
		{name: "unknown rule key", condition: InspectionConditionItem{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"not_a_rule"}}, wantErr: "unknown alert rule key"},
		{name: "retired stx_ template key", condition: InspectionConditionItem{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"stx_server_memory_high"}}, wantErr: "unknown alert rule key"},
		{name: "zero threshold", condition: InspectionConditionItem{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"node_offline"}, ThresholdOverride: intPtr(0)}, wantErr: "threshold"},
		{name: "negative window", condition: InspectionConditionItem{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"node_offline"}, WindowMinutesOverride: intPtr(-1)}, wantErr: "window"},
		{name: "window too large", condition: InspectionConditionItem{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"node_offline"}, WindowMinutesOverride: intPtr(1441)}, wantErr: "window"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := *base
			req.Conditions = InspectionConditionItems{tc.condition}
			_, err := service.CreateAutoPolicy(context.Background(), 1, &req)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}

	// 合法规则键去重后应创建成功。
	// Valid rule keys (with duplicates) must create successfully.
	policy, err := service.CreateAutoPolicy(context.Background(), 1, &CreateInspectionAutoPolicyRequest{
		Name:      "alert-firing-ok",
		ClusterID: 7,
		Conditions: InspectionConditionItems{
			{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"node_offline", " node_offline ", "cpu_usage_high", "gc_time_high"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}
	if got := policy.Conditions[0].RuleKeys; len(got) != 3 || got[0] != "node_offline" || got[1] != "cpu_usage_high" || got[2] != "gc_time_high" {
		t.Fatalf("expected deduped rule keys [node_offline cpu_usage_high gc_time_high], got %v", got)
	}

	// 已删除的旧条件码按未知模板 400。
	// Deleted legacy condition codes are rejected as unknown templates.
	_, err = service.CreateAutoPolicy(context.Background(), 1, &CreateInspectionAutoPolicyRequest{
		Name:      "legacy-code",
		ClusterID: 7,
		Conditions: InspectionConditionItems{
			{TemplateCode: InspectionConditionTemplateCode("PROM_CPU_HIGH"), Enabled: true},
		},
	})
	if err == nil {
		t.Fatal("expected deleted legacy code to be rejected")
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

// alertFiringReportTotal 返回巡检报告总数；测试用它判断告警触发条件是否命中。
// alertFiringReportTotal returns the inspection report total; tests use it to check whether
// an alert-firing condition matched.
func alertFiringReportTotal(t *testing.T, repo *Repository, clusterID uint) int64 {
	t.Helper()
	_, total, err := repo.ListInspectionReports(context.Background(), &ClusterInspectionReportFilter{
		ClusterID: clusterID,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListInspectionReports returned error: %v", err)
	}
	return total
}

func TestAutoPolicyCheckerCheckAlertFiringPolicies_triggersWhenTargetThresholdMet(t *testing.T) {
	now := time.Date(2026, 3, 26, 2, 20, 0, 0, time.UTC)
	service, repo := newAutoPolicyTestService(t, &fakeInspectionAlertReader{
		data: &monitoringapp.AlertInstanceListData{
			Alerts: []*monitoringapp.AlertInstance{
				{
					AlertID:   "node-a",
					ClusterID: "7",
					RuleKey:   "node_offline",
					Status:    monitoringapp.AlertDisplayStatusFiring,
					FiringAt:  now.Add(-20 * time.Minute),
					SourceRef: &monitoringapp.AlertInstanceSourceRef{Hostname: "host-a"},
				},
				{
					AlertID:   "node-b",
					ClusterID: "7",
					RuleKey:   "node_offline",
					Status:    monitoringapp.AlertDisplayStatusFiring,
					FiringAt:  now.Add(-19 * time.Minute),
					SourceRef: &monitoringapp.AlertInstanceSourceRef{Hostname: "host-b"},
				},
				// 规则键不在条件列表里的 firing 告警不参与对象计数。
				// A firing alert whose rule key is not selected must not count.
				{
					AlertID:   "node-c",
					ClusterID: "7",
					RuleKey:   "cpu_usage_high",
					Status:    monitoringapp.AlertDisplayStatusFiring,
					FiringAt:  now.Add(-19 * time.Minute),
					SourceRef: &monitoringapp.AlertInstanceSourceRef{Hostname: "host-c"},
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
				TemplateCode:          ConditionCodeAlertFiring,
				Enabled:               true,
				RuleKeys:              []string{"node_offline"},
				ThresholdOverride:     intPtr(2),
				WindowMinutesOverride: intPtr(10),
			},
		},
	}
	if err := repo.CreateAutoPolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}

	checker := NewAutoPolicyChecker(repo, service)
	if err := checker.CheckAlertFiringPolicies(context.Background(), 7, now); err != nil {
		t.Fatalf("CheckAlertFiringPolicies returned error: %v", err)
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
	if !strings.Contains(reports[0].AutoTriggerReason, "ALERT_FIRING") || !strings.Contains(reports[0].AutoTriggerReason, "targets=2") {
		t.Fatalf("expected ALERT_FIRING targets=2 trigger reason, got %q", reports[0].AutoTriggerReason)
	}
}

func TestAutoPolicyCheckerCheckAlertFiringPolicies_skipsAlertInsideWindowAndLegacyCodes(t *testing.T) {
	now := time.Date(2026, 3, 26, 2, 20, 0, 0, time.UTC)
	service, repo := newAutoPolicyTestService(t, &fakeInspectionAlertReader{
		data: &monitoringapp.AlertInstanceListData{
			Alerts: []*monitoringapp.AlertInstance{
				{
					AlertID:   "alert-1",
					ClusterID: "7",
					RuleKey:   "node_offline",
					Status:    monitoringapp.AlertDisplayStatusFiring,
					// 距窗口要求还差 1 秒，不满足持续时长。
					// One second short of the required window, so it must not trigger.
					FiringAt: now.Add(-10*time.Minute + time.Second),
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
				TemplateCode:          ConditionCodeAlertFiring,
				Enabled:               true,
				RuleKeys:              []string{"node_offline"},
				WindowMinutesOverride: intPtr(10),
			},
			// 已删除的旧条件码：检查器直接跳过，不报错也不触发。
			// A deleted legacy condition code: the checker skips it silently.
			{
				TemplateCode: InspectionConditionTemplateCode("PROM_CPU_HIGH"),
				Enabled:      true,
			},
		},
	}
	if err := repo.CreateAutoPolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreateAutoPolicy returned error: %v", err)
	}

	checker := NewAutoPolicyChecker(repo, service)
	if err := checker.CheckAlertFiringPolicies(context.Background(), 7, now); err != nil {
		t.Fatalf("CheckAlertFiringPolicies returned error: %v", err)
	}
	if total := alertFiringReportTotal(t, repo, 7); total != 0 {
		t.Fatalf("expected no inspection report before the window elapses, total=%d", total)
	}

	// 快进 2 秒后持续时长满足，同一告警应触发巡检。
	// Two seconds later the window elapses and the same alert must trigger.
	if err := checker.CheckAlertFiringPolicies(context.Background(), 7, now.Add(2*time.Second)); err != nil {
		t.Fatalf("CheckAlertFiringPolicies returned error: %v", err)
	}
	if total := alertFiringReportTotal(t, repo, 7); total != 1 {
		t.Fatalf("expected one inspection report once the window elapses, total=%d", total)
	}
}
