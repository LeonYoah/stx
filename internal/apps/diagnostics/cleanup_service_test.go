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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/config"
)

func newCleanupTestRepository(t *testing.T) *Repository {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(
		&ClusterInspectionReport{},
		&ClusterInspectionFinding{},
		&DiagnosticTask{},
		&DiagnosticTaskStep{},
		&DiagnosticNodeExecution{},
		&DiagnosticStepLog{},
		&InspectionCleanupSettings{},
	); err != nil {
		t.Fatalf("auto migrate cleanup models: %v", err)
	}
	return NewRepository(database)
}

func seedCleanupReport(t *testing.T, repo *Repository, status InspectionReportStatus) *ClusterInspectionReport {
	t.Helper()
	report := &ClusterInspectionReport{
		ClusterID:       7,
		Status:          status,
		TriggerSource:   InspectionTriggerSourceDiagnosticsWorkspace,
		LookbackMinutes: 30,
	}
	if err := repo.CreateInspectionReport(t.Context(), report); err != nil {
		t.Fatalf("create report: %v", err)
	}
	return report
}

func seedCleanupTask(t *testing.T, repo *Repository, reportID uint, status DiagnosticTaskStatus) *DiagnosticTask {
	t.Helper()
	task := &DiagnosticTask{
		ClusterID:     7,
		TriggerSource: DiagnosticTaskSourceInspectionFinding,
		SourceRef:     DiagnosticTaskSourceRef{InspectionReportID: reportID},
		Options:       DiagnosticTaskOptions{},
		SelectedNodes: DiagnosticTaskNodeTargets{},
		Status:        status,
	}
	if err := repo.db.WithContext(t.Context()).Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

// backdateColumns 绕过 gorm 自动时间戳直接改写 created_at / updated_at。
// backdateColumns writes created_at / updated_at directly past gorm autotime.
func backdateColumns(t *testing.T, repo *Repository, table string, id uint, createdAt, updatedAt time.Time) {
	t.Helper()
	if err := repo.db.Exec(
		"UPDATE "+table+" SET created_at = ?, updated_at = ? WHERE id = ?",
		createdAt, updatedAt, id,
	).Error; err != nil {
		t.Fatalf("backdate %s #%d: %v", table, id, err)
	}
}

func countRows(t *testing.T, repo *Repository, table string, where string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := repo.db.Table(table).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestInspectionCleanupSettingsRoundTrip(t *testing.T) {
	repo := newCleanupTestRepository(t)
	ctx := context.Background()

	settings, err := repo.GetInspectionCleanupSettings(ctx)
	if err != nil {
		t.Fatalf("get default settings: %v", err)
	}
	if settings.RetentionDays != 0 || settings.MaxReports != 0 {
		t.Fatalf("expected default 0/0, got %+v", settings)
	}

	saved, err := repo.SaveInspectionCleanupSettings(ctx, 30, 200)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if saved.RetentionDays != 30 || saved.MaxReports != 200 || saved.ID != inspectionCleanupSettingsRowID {
		t.Fatalf("unexpected saved settings: %+v", saved)
	}
	// 再存一次必须还是同一行（单行 upsert）。
	// A second save must still be the same single row (upsert).
	if _, err := repo.SaveInspectionCleanupSettings(ctx, 0, 0); err != nil {
		t.Fatalf("re-save settings: %v", err)
	}
	if count := countRows(t, repo, "diagnostics_cleanup_settings", "1=1"); count != 1 {
		t.Fatalf("expected 1 settings row, got %d", count)
	}
}

func TestSaveInspectionCleanupSettingsValidation(t *testing.T) {
	repo := newCleanupTestRepository(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	if _, err := svc.SaveInspectionCleanupSettings(ctx, nil); err == nil {
		t.Fatal("expected error for nil request")
	}
	if _, err := svc.SaveInspectionCleanupSettings(ctx, &UpdateInspectionCleanupSettingsRequest{}); err == nil {
		t.Fatal("expected error for missing fields")
	}
	negative := -1
	if _, err := svc.SaveInspectionCleanupSettings(ctx, &UpdateInspectionCleanupSettingsRequest{RetentionDays: &negative, MaxReports: &negative}); err == nil {
		t.Fatal("expected error for negative values")
	}
	over := maxInspectionRetentionDays + 1
	zero := 0
	if _, err := svc.SaveInspectionCleanupSettings(ctx, &UpdateInspectionCleanupSettingsRequest{RetentionDays: &over, MaxReports: &zero}); err == nil {
		t.Fatal("expected error for retention_days over cap")
	}
	// 显式 0/0 合法：表示「不自动清理」。
	// Explicit 0/0 is valid: it means "no automatic cleanup".
	if _, err := svc.SaveInspectionCleanupSettings(ctx, &UpdateInspectionCleanupSettingsRequest{RetentionDays: &zero, MaxReports: &zero}); err != nil {
		t.Fatalf("expected 0/0 to be valid, got %v", err)
	}
}

func TestListDiagnosticTasksByInspectionReportIDs(t *testing.T) {
	repo := newCleanupTestRepository(t)
	ctx := context.Background()

	bound := seedCleanupTask(t, repo, 42, DiagnosticTaskStatusSucceeded)
	bound2 := seedCleanupTask(t, repo, 42, DiagnosticTaskStatusFailed)
	// 绑定到其它巡检记录的任务不应被选中。
	// A task bound to a different report must not be selected.
	seedCleanupTask(t, repo, 43, DiagnosticTaskStatusSucceeded)
	// 非巡检来源的任务即使 source_ref 结构类似也不应命中。
	// Non-inspection tasks must never match even with a similar source_ref.
	unrelated := &DiagnosticTask{
		ClusterID:     7,
		TriggerSource: DiagnosticTaskSourceErrorGroup,
		SourceRef:     DiagnosticTaskSourceRef{InspectionReportID: 42, ErrorGroupID: 9},
		Options:       DiagnosticTaskOptions{},
		SelectedNodes: DiagnosticTaskNodeTargets{},
		Status:        DiagnosticTaskStatusSucceeded,
	}
	if err := repo.db.WithContext(ctx).Create(unrelated).Error; err != nil {
		t.Fatalf("create unrelated task: %v", err)
	}

	tasks, err := repo.ListDiagnosticTasksByInspectionReportIDs(ctx, []uint{42})
	if err != nil {
		t.Fatalf("list bound tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 bound tasks, got %d", len(tasks))
	}
	seen := map[uint]bool{}
	for _, task := range tasks {
		seen[task.ID] = true
	}
	if !seen[bound.ID] || !seen[bound2.ID] {
		t.Fatalf("missing expected task ids in %+v", seen)
	}
}

func TestDeleteInspectionReportsCascade(t *testing.T) {
	repo := newCleanupTestRepository(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	baseDir := t.TempDir()
	oldBase := config.Config.Storage.BaseDir
	config.Config.Storage.BaseDir = baseDir
	defer func() { config.Config.Storage.BaseDir = oldBase }()

	report := seedCleanupReport(t, repo, InspectionReportStatusCompleted)
	if err := repo.CreateInspectionFindings(ctx, []*ClusterInspectionFinding{
		{ReportID: report.ID, ClusterID: 7, Severity: InspectionFindingSeverityWarning, CheckCode: "x", CheckName: "x"},
	}); err != nil {
		t.Fatalf("create findings: %v", err)
	}
	task := seedCleanupTask(t, repo, report.ID, DiagnosticTaskStatusSucceeded)
	bundleDir := filepath.Join(baseDir, "diagnostics", "tasks", "bundle-1")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	if err := repo.db.Model(task).Update("bundle_dir", bundleDir).Error; err != nil {
		t.Fatalf("set bundle dir: %v", err)
	}
	step := &DiagnosticTaskStep{TaskID: task.ID, Code: "collect", Status: DiagnosticTaskStatusSucceeded}
	node := &DiagnosticNodeExecution{TaskID: task.ID, NodeID: 1, Status: DiagnosticTaskStatusSucceeded}
	log := &DiagnosticStepLog{TaskID: task.ID, StepCode: "collect", EventType: DiagnosticLogEventTypeSuccess, Message: "done"}
	for _, row := range []any{step, node, log} {
		if err := repo.db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatalf("create child row: %v", err)
		}
	}

	results, err := svc.DeleteInspectionReports(ctx, []uint{report.ID})
	if err != nil {
		t.Fatalf("delete inspection reports: %v", err)
	}
	if len(results) != 1 || results[0].Result != InspectionDeleteResultDeleted {
		t.Fatalf("expected deleted result, got %+v", results)
	}
	for _, check := range []struct {
		table string
		where string
		args  []any
	}{
		{"diagnostics_inspection_reports", "id = ?", []any{report.ID}},
		{"diagnostics_inspection_findings", "report_id = ?", []any{report.ID}},
		{"diagnostics_tasks", "id = ?", []any{task.ID}},
		{"diagnostics_task_steps", "task_id = ?", []any{task.ID}},
		{"diagnostics_task_nodes", "task_id = ?", []any{task.ID}},
		{"diagnostics_task_logs", "task_id = ?", []any{task.ID}},
	} {
		if count := countRows(t, repo, check.table, check.where, check.args...); count != 0 {
			t.Fatalf("expected %s cleaned, still has %d rows", check.table, count)
		}
	}
	if _, statErr := os.Stat(bundleDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected bundle dir removed, stat err=%v", statErr)
	}
}

func TestDeleteInspectionReportsGuards(t *testing.T) {
	repo := newCleanupTestRepository(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	running := seedCleanupReport(t, repo, InspectionReportStatusRunning)
	pending := seedCleanupReport(t, repo, InspectionReportStatusPending)
	doneWithRunningTask := seedCleanupReport(t, repo, InspectionReportStatusCompleted)
	boundTask := seedCleanupTask(t, repo, doneWithRunningTask.ID, DiagnosticTaskStatusRunning)
	// 终态报告但绑定任务还在跑：整条跳过，记录保留。
	// Terminal report whose bound task still runs: skipped entirely, kept.

	results, err := svc.DeleteInspectionReports(ctx, []uint{running.ID, pending.ID, doneWithRunningTask.ID, 9999})
	if err != nil {
		t.Fatalf("delete with guards: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %+v", results)
	}
	byID := map[uint]InspectionDeleteResultCode{}
	for _, item := range results {
		byID[item.ID] = item.Result
	}
	if byID[running.ID] != InspectionDeleteResultReportRunning {
		t.Fatalf("running report: got %s", byID[running.ID])
	}
	if byID[pending.ID] != InspectionDeleteResultReportRunning {
		t.Fatalf("pending report: got %s", byID[pending.ID])
	}
	if byID[doneWithRunningTask.ID] != InspectionDeleteResultTaskRunning {
		t.Fatalf("task-running report: got %s", byID[doneWithRunningTask.ID])
	}
	if byID[9999] != InspectionDeleteResultNotFound {
		t.Fatalf("missing report: got %s", byID[9999])
	}
	for _, id := range []uint{running.ID, pending.ID, doneWithRunningTask.ID} {
		if count := countRows(t, repo, "diagnostics_inspection_reports", "id = ?", id); count != 1 {
			t.Fatalf("report #%d should still exist", id)
		}
	}
	if count := countRows(t, repo, "diagnostics_tasks", "id = ?", boundTask.ID); count != 1 {
		t.Fatal("running bound task should still exist")
	}
}

func TestDeleteInspectionReportsKeepsUnrelatedTasks(t *testing.T) {
	repo := newCleanupTestRepository(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	report := seedCleanupReport(t, repo, InspectionReportStatusCompleted)
	// 从错误组直接发起的诊断任务与巡检记录无绑定，删除记录不得动它。
	// A task created directly from an error group is unbound; deleting the
	// report must leave it alone.
	unrelated := &DiagnosticTask{
		ClusterID:     7,
		TriggerSource: DiagnosticTaskSourceErrorGroup,
		SourceRef:     DiagnosticTaskSourceRef{ErrorGroupID: 5},
		Options:       DiagnosticTaskOptions{},
		SelectedNodes: DiagnosticTaskNodeTargets{},
		Status:        DiagnosticTaskStatusSucceeded,
	}
	if err := repo.db.WithContext(ctx).Create(unrelated).Error; err != nil {
		t.Fatalf("create unrelated task: %v", err)
	}

	if _, err := svc.DeleteInspectionReports(ctx, []uint{report.ID}); err != nil {
		t.Fatalf("delete report: %v", err)
	}
	if count := countRows(t, repo, "diagnostics_tasks", "id = ?", unrelated.ID); count != 1 {
		t.Fatal("unrelated task must survive report deletion")
	}
}

func TestListExpiredInspectionReportIDs(t *testing.T) {
	repo := newCleanupTestRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-40 * 24 * time.Hour)

	oldDone := seedCleanupReport(t, repo, InspectionReportStatusCompleted)
	oldFailed := seedCleanupReport(t, repo, InspectionReportStatusFailed)
	newDone := seedCleanupReport(t, repo, InspectionReportStatusCompleted)
	// 非终态的旧记录永不进入清理候选（在跑的东西不是残留物）。
	// Non-terminal old reports never become cleanup candidates (live work is not residue).
	oldRunning := seedCleanupReport(t, repo, InspectionReportStatusRunning)
	for _, id := range []uint{oldDone.ID, oldFailed.ID, oldRunning.ID} {
		backdateColumns(t, repo, "diagnostics_inspection_reports", id, old, old)
	}

	// 天数维度：只有到期终态记录。
	// Age dimension: only expired terminal records.
	ids, err := repo.ListExpiredInspectionReportIDs(ctx, 30, 0, now, 0)
	if err != nil {
		t.Fatalf("expired by days: %v", err)
	}
	if len(ids) != 2 || (ids[0] != oldDone.ID && ids[1] != oldDone.ID) {
		t.Fatalf("expected 2 expired ids incl oldDone, got %v", ids)
	}
	for _, id := range ids {
		if id == newDone.ID || id == oldRunning.ID {
			t.Fatalf("unexpected id %d in expired set", id)
		}
	}

	// 数量维度：4 条终态保留 1 条 → 最旧的 2 条到期（newDone 留下，oldRunning 不数）。
	// Count dimension: keep 1 of 3 terminal -> the oldest 2 expire (newDone stays,
	// oldRunning not counted).
	ids, err = repo.ListExpiredInspectionReportIDs(ctx, 0, 1, now, 0)
	if err != nil {
		t.Fatalf("expired by count: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 overflow ids, got %v", ids)
	}
	for _, id := range ids {
		if id == newDone.ID {
			t.Fatalf("newest report must be kept, got %v", ids)
		}
	}

	// 双维度并集 + 0/0 什么都不清。
	// Combined union, and 0/0 cleans nothing.
	ids, err = repo.ListExpiredInspectionReportIDs(ctx, 30, 2, now, 0)
	if err != nil {
		t.Fatalf("expired union: %v", err)
	}
	if len(ids) < 2 {
		t.Fatalf("expected union ids, got %v", ids)
	}
	ids, err = repo.ListExpiredInspectionReportIDs(ctx, 0, 0, now, 0)
	if err != nil {
		t.Fatalf("expired 0/0: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("0/0 must return nothing, got %v", ids)
	}
}

func TestReconcileInterruptedDiagnostics(t *testing.T) {
	repo := newCleanupTestRepository(t)
	svc := &Service{repo: repo}
	ctx := context.Background()
	now := time.Now().UTC()
	processStartedAt := now.Add(-time.Minute)
	old := processStartedAt.Add(-time.Hour)

	zombieReport := seedCleanupReport(t, repo, InspectionReportStatusRunning)
	freshReport := seedCleanupReport(t, repo, InspectionReportStatusRunning)
	zombieTask := seedCleanupTask(t, repo, 42, DiagnosticTaskStatusRunning)
	cancellingTask := seedCleanupTask(t, repo, 43, DiagnosticTaskStatusCancelling)
	freshTask := seedCleanupTask(t, repo, 44, DiagnosticTaskStatusRunning)
	step := &DiagnosticTaskStep{TaskID: zombieTask.ID, Code: "collect", Status: DiagnosticTaskStatusRunning}
	node := &DiagnosticNodeExecution{TaskID: zombieTask.ID, NodeID: 1, Status: DiagnosticTaskStatusRunning}
	if err := repo.db.WithContext(ctx).Create(step).Error; err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := repo.db.WithContext(ctx).Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	// 僵尸 = updated_at 早于进程启动；新鲜 = 晚于进程启动，绝不能动。
	// Zombie = updated_at before process start; fresh = after it, must stay.
	backdateColumns(t, repo, "diagnostics_inspection_reports", zombieReport.ID, old, old)
	backdateColumns(t, repo, "diagnostics_tasks", zombieTask.ID, old, old)
	backdateColumns(t, repo, "diagnostics_tasks", cancellingTask.ID, old, old)

	svc.ReconcileInterruptedDiagnostics(ctx, processStartedAt)

	reload := func(table string, id uint) (status, reason string) {
		var row struct {
			Status  string `gorm:"column:status"`
			Message string `gorm:"column:error_message"`
			Reason  string `gorm:"column:failure_reason"`
		}
		if err := repo.db.Table(table).Select("status, '' as error_message, '' as failure_reason").Where("id = ?", id).Scan(&row).Error; err != nil {
			t.Fatalf("reload %s #%d: %v", table, id, err)
		}
		switch table {
		case "diagnostics_inspection_reports":
			var report ClusterInspectionReport
			if err := repo.db.First(&report, id).Error; err != nil {
				t.Fatalf("reload report #%d: %v", id, err)
			}
			return string(report.Status), report.ErrorMessage
		default:
			var task DiagnosticTask
			if err := repo.db.First(&task, id).Error; err != nil {
				t.Fatalf("reload task #%d: %v", id, err)
			}
			return string(task.Status), task.FailureReason
		}
	}

	status, msg := reload("diagnostics_inspection_reports", zombieReport.ID)
	if status != string(InspectionReportStatusFailed) || msg == "" {
		t.Fatalf("zombie report should be failed with message, got %s / %q", status, msg)
	}
	status, _ = reload("diagnostics_inspection_reports", freshReport.ID)
	if status != string(InspectionReportStatusRunning) {
		t.Fatalf("fresh report must stay running, got %s", status)
	}
	status, reason := reload("diagnostics_tasks", zombieTask.ID)
	if status != string(DiagnosticTaskStatusFailed) || reason == "" {
		t.Fatalf("zombie task should be failed with reason, got %s / %q", status, reason)
	}
	status, _ = reload("diagnostics_tasks", cancellingTask.ID)
	if status != string(DiagnosticTaskStatusCancelled) {
		t.Fatalf("cancelling task should land on cancelled, got %s", status)
	}
	status, _ = reload("diagnostics_tasks", freshTask.ID)
	if status != string(DiagnosticTaskStatusRunning) {
		t.Fatalf("fresh task must stay running, got %s", status)
	}
	// 僵尸任务的非终态子行同步收尾。
	// Non-terminal children of the zombie task are closed too.
	var closedStep DiagnosticTaskStep
	if err := repo.db.First(&closedStep, step.ID).Error; err != nil {
		t.Fatalf("reload step: %v", err)
	}
	if closedStep.Status != DiagnosticTaskStatusFailed {
		t.Fatalf("zombie step should be failed, got %s", closedStep.Status)
	}
	var closedNode DiagnosticNodeExecution
	if err := repo.db.First(&closedNode, node.ID).Error; err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if closedNode.Status != DiagnosticTaskStatusFailed {
		t.Fatalf("zombie node should be failed, got %s", closedNode.Status)
	}
}

func TestSweepExpiredInspectionReports(t *testing.T) {
	repo := newCleanupTestRepository(t)
	svc := &Service{repo: repo}
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-40 * 24 * time.Hour)

	oldReport := seedCleanupReport(t, repo, InspectionReportStatusCompleted)
	newReport := seedCleanupReport(t, repo, InspectionReportStatusCompleted)
	boundTask := seedCleanupTask(t, repo, oldReport.ID, DiagnosticTaskStatusSucceeded)
	backdateColumns(t, repo, "diagnostics_inspection_reports", oldReport.ID, old, old)

	// 未配置时不动。
	// With no settings nothing moves.
	svc.sweepExpiredInspectionReports(ctx, now)
	if count := countRows(t, repo, "diagnostics_inspection_reports", "id = ?", oldReport.ID); count != 1 {
		t.Fatal("no settings -> report must stay")
	}

	if _, err := svc.SaveInspectionCleanupSettings(ctx, &UpdateInspectionCleanupSettingsRequest{
		RetentionDays: intPtr(30), MaxReports: intPtr(0),
	}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	svc.sweepExpiredInspectionReports(ctx, now)
	if count := countRows(t, repo, "diagnostics_inspection_reports", "id = ?", oldReport.ID); count != 0 {
		t.Fatal("expired report should be swept")
	}
	if count := countRows(t, repo, "diagnostics_inspection_reports", "id = ?", newReport.ID); count != 1 {
		t.Fatal("fresh report must stay")
	}
	if count := countRows(t, repo, "diagnostics_tasks", "id = ?", boundTask.ID); count != 0 {
		t.Fatal("bound task should be swept with the report")
	}
}
