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
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// inspectionCleanupSettingsRowID 单行设置固定主键，upsert 只写这一行。
// inspectionCleanupSettingsRowID is the fixed primary key of the single settings row.
const inspectionCleanupSettingsRowID = 1

// GetInspectionCleanupSettings returns the stored cleanup settings, or zero values when the
// row has never been written (feature defaults to "no cleanup").
// GetInspectionCleanupSettings 返回已保存的清理设置；从未写入时返回零值（默认不清理）。
func (r *Repository) GetInspectionCleanupSettings(ctx context.Context) (*InspectionCleanupSettings, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var row InspectionCleanupSettings
	err := r.db.WithContext(ctx).First(&row, inspectionCleanupSettingsRowID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &InspectionCleanupSettings{}, nil
		}
		return nil, fmt.Errorf("get inspection cleanup settings: %w", err)
	}
	return &row, nil
}

// SaveInspectionCleanupSettings upserts the single cleanup settings row.
// SaveInspectionCleanupSettings 更新或写入唯一一行清理设置。
func (r *Repository) SaveInspectionCleanupSettings(ctx context.Context, retentionDays, maxReports int) (*InspectionCleanupSettings, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	row := &InspectionCleanupSettings{
		ID:            inspectionCleanupSettingsRowID,
		RetentionDays: retentionDays,
		MaxReports:    maxReports,
	}
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return nil, fmt.Errorf("save inspection cleanup settings: %w", err)
	}
	return r.GetInspectionCleanupSettings(ctx)
}

// zombieUpdatedAtScope 构造「updated_at 早于进程启动或早于僵死窗口」的 OR 条件，
// sqlite 下走 julianday 与 applyOccurredAt* 同一套时间比较约定。
// zombieUpdatedAtScope builds the "updated_at before process start or before the stale
// window" OR predicate, sharing the julianday convention used by applyOccurredAt* on sqlite.
func zombieUpdatedAtScope(dialector string, processStartedAt, staleBefore time.Time) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if strings.EqualFold(dialector, "sqlite") {
			return db.Where(
				"(julianday(updated_at) < julianday(?) OR julianday(updated_at) < julianday(?))",
				processStartedAt.UTC().Format(time.RFC3339Nano), staleBefore.UTC().Format(time.RFC3339Nano),
			)
		}
		return db.Where("(updated_at < ? OR updated_at < ?)", processStartedAt, staleBefore)
	}
}

// ListDiagnosticTasksByInspectionReportIDs returns every diagnostic task bound to one of the
// given inspection reports (source_ref.inspection_report_id match), newest first.
// Dialect handling mirrors GetLatestDiagnosticTaskByInspectionReportID.
// ListDiagnosticTasksByInspectionReportIDs 返回绑定到指定巡检记录的全部诊断任务
// （source_ref.inspection_report_id 命中），按创建时间倒序；方言处理与
// GetLatestDiagnosticTaskByInspectionReportID 一致。
func (r *Repository) ListDiagnosticTasksByInspectionReportIDs(ctx context.Context, reportIDs []uint) ([]*DiagnosticTask, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if len(reportIDs) == 0 {
		return []*DiagnosticTask{}, nil
	}
	dialector := r.db.Dialector.Name()
	var query *gorm.DB
	switch dialector {
	case "sqlite":
		query = r.db.WithContext(ctx).Where(
			"trigger_source = ? AND json_extract(source_ref, '$.inspection_report_id') IN ?",
			DiagnosticTaskSourceInspectionFinding, reportIDs,
		)
	case "mysql":
		query = r.db.WithContext(ctx).Where(
			"trigger_source = ? AND CAST(JSON_UNQUOTE(JSON_EXTRACT(source_ref, '$.inspection_report_id')) AS UNSIGNED) IN ?",
			DiagnosticTaskSourceInspectionFinding, reportIDs,
		)
	default:
		query = r.db.WithContext(ctx).Where(
			"trigger_source = ? AND (source_ref->>'inspection_report_id')::bigint IN ?",
			DiagnosticTaskSourceInspectionFinding, reportIDs,
		)
	}
	tasks := make([]*DiagnosticTask, 0, len(reportIDs))
	if err := query.Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list diagnostic tasks by inspection_report_id: %w", err)
	}
	return tasks, nil
}

// ListExpiredInspectionReportIDs 返回满足清理条件的终态巡检记录 id：
// retentionDays>0 时收 created_at 早于阈值的记录；maxReports>0 时收总量超过上限后
// 最旧的记录。两个条件取并集，按 id 升序（即创建顺序）返回，limit>0 时截断。
// ListExpiredInspectionReportIDs returns ids of terminal inspection reports matching the
// cleanup policy: when retentionDays>0, reports created before the cutoff; when
// maxReports>0, the oldest reports beyond the cap. Union of both, oldest first, truncated
// to limit when positive.
func (r *Repository) ListExpiredInspectionReportIDs(ctx context.Context, retentionDays, maxReports int, now time.Time, limit int) ([]uint, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	terminal := []InspectionReportStatus{InspectionReportStatusCompleted, InspectionReportStatusFailed}
	idSet := map[uint]struct{}{}
	if retentionDays > 0 {
		cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
		var ids []uint
		query := r.db.WithContext(ctx).Model(&ClusterInspectionReport{}).
			Where("status IN ?", terminal)
		query = applyOccurredAtUpperBound(query, "created_at", cutoff)
		if err := query.Pluck("id", &ids).Error; err != nil {
			return nil, fmt.Errorf("list expired inspection reports by retention: %w", err)
		}
		for _, id := range ids {
			idSet[id] = struct{}{}
		}
	}
	if maxReports > 0 {
		var ids []uint
		if err := r.db.WithContext(ctx).Model(&ClusterInspectionReport{}).
			Where("status IN ?", terminal).
			Order("created_at ASC").
			Pluck("id", &ids).Error; err != nil {
			return nil, fmt.Errorf("list inspection reports for overflow: %w", err)
		}
		for i, id := range ids {
			if len(ids)-i <= maxReports {
				break
			}
			idSet[id] = struct{}{}
		}
	}
	result := make([]uint, 0, len(idSet))
	for id := range idSet {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// DeleteInspectionReportsCascade deletes, inside the caller-managed transaction repository,
// everything bound to the given inspection reports: findings, bound diagnostic tasks and
// their steps / node executions / logs, and finally the report rows themselves.
// It returns the bundle dirs of the deleted tasks so the caller can remove them after commit.
// DeleteInspectionReportsCascade 在调用方管理的事务仓储内删除指定巡检记录的全部绑定物：
// 发现项、绑定诊断任务及其步骤 / 节点执行 / 日志，最后删记录行本身；
// 返回被删任务的 bundle 目录供提交后移除。
func (r *Repository) DeleteInspectionReportsCascade(ctx context.Context, reportIDs, taskIDs []uint) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if len(reportIDs) == 0 {
		return nil, nil
	}
	bundleDirs := make([]string, 0, len(taskIDs))
	if len(taskIDs) > 0 {
		var dirs []string
		if err := r.db.WithContext(ctx).Model(&DiagnosticTask{}).
			Where("id IN ?", taskIDs).
			Where("bundle_dir <> ''").
			Pluck("bundle_dir", &dirs).Error; err != nil {
			return nil, fmt.Errorf("collect bundle dirs: %w", err)
		}
		bundleDirs = dirs
		if err := r.db.WithContext(ctx).
			Where("task_id IN ?", taskIDs).
			Delete(&DiagnosticStepLog{}).Error; err != nil {
			return nil, fmt.Errorf("delete diagnostics task logs: %w", err)
		}
		if err := r.db.WithContext(ctx).
			Where("task_id IN ?", taskIDs).
			Delete(&DiagnosticNodeExecution{}).Error; err != nil {
			return nil, fmt.Errorf("delete diagnostics task nodes: %w", err)
		}
		if err := r.db.WithContext(ctx).
			Where("task_id IN ?", taskIDs).
			Delete(&DiagnosticTaskStep{}).Error; err != nil {
			return nil, fmt.Errorf("delete diagnostics task steps: %w", err)
		}
		if err := r.db.WithContext(ctx).
			Where("id IN ?", taskIDs).
			Delete(&DiagnosticTask{}).Error; err != nil {
			return nil, fmt.Errorf("delete diagnostics tasks: %w", err)
		}
	}
	if err := r.db.WithContext(ctx).
		Where("report_id IN ?", reportIDs).
		Delete(&ClusterInspectionFinding{}).Error; err != nil {
		return nil, fmt.Errorf("delete inspection findings: %w", err)
	}
	if err := r.db.WithContext(ctx).
		Where("id IN ?", reportIDs).
		Delete(&ClusterInspectionReport{}).Error; err != nil {
		return nil, fmt.Errorf("delete inspection reports: %w", err)
	}
	return bundleDirs, nil
}

// ReconcileInterruptedInspectionReports marks non-terminal inspection reports whose
// updated_at predates the process start (or is older than the stale window) as failed.
// Returns the reconciled report ids.
// ReconcileInterruptedInspectionReports 把 updated_at 早于进程启动（或超过僵死窗口）的
// 非终态巡检记录置为 failed，返回被收尾的记录 id。
func (r *Repository) ReconcileInterruptedInspectionReports(ctx context.Context, processStartedAt, staleBefore time.Time, message string) ([]uint, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	var ids []uint
	zombie := zombieUpdatedAtScope(r.db.Dialector.Name(), processStartedAt, staleBefore)
	if err := zombie(r.db.WithContext(ctx).Model(&ClusterInspectionReport{}).
		Where("status IN ?", []InspectionReportStatus{InspectionReportStatusPending, InspectionReportStatusRunning})).
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list interrupted inspection reports: %w", err)
	}
	if len(ids) == 0 {
		return ids, nil
	}
	now := timex.Now()
	result := r.db.WithContext(ctx).Model(&ClusterInspectionReport{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"status":        InspectionReportStatusFailed,
			"error_message": message,
			"finished_at":   now,
			"updated_at":    now,
		})
	if result.Error != nil {
		return nil, fmt.Errorf("reconcile interrupted inspection reports: %w", result.Error)
	}
	return ids, nil
}

// ReconcileInterruptedDiagnosticTasks closes non-terminal diagnostic tasks whose updated_at
// predates the process start (or is older than the stale window): cancel-requested and
// cancelling tasks become cancelled, everything else becomes failed. Non-terminal steps and
// node executions belonging to reconciled tasks are closed with the same terminal status.
// Returns the reconciled task ids.
// ReconcileInterruptedDiagnosticTasks 把 updated_at 早于进程启动（或超过僵死窗口）的
// 非终态诊断任务收尾：cancel_requested / cancelling 置 cancelled，其余置 failed；
// 属于被收尾任务的非终态步骤与节点执行同步置为同一终态。返回被收尾的任务 id。
func (r *Repository) ReconcileInterruptedDiagnosticTasks(ctx context.Context, processStartedAt, staleBefore time.Time, failureMessage string) ([]uint, error) {
	if r == nil || r.db == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	now := timex.Now()
	zombie := zombieUpdatedAtScope(r.db.Dialector.Name(), processStartedAt, staleBefore)

	pluckZombie := func(statuses []DiagnosticTaskStatus) ([]uint, error) {
		var ids []uint
		err := zombie(r.db.WithContext(ctx).Model(&DiagnosticTask{}).
			Where("status IN ?", statuses)).
			Pluck("id", &ids).Error
		return ids, err
	}
	cancelIDs, err := pluckZombie([]DiagnosticTaskStatus{DiagnosticTaskStatusCancelRequested, DiagnosticTaskStatusCancelling})
	if err != nil {
		return nil, fmt.Errorf("list cancelling zombie tasks: %w", err)
	}
	failIDs, err := pluckZombie([]DiagnosticTaskStatus{DiagnosticTaskStatusPending, DiagnosticTaskStatusReady, DiagnosticTaskStatusRunning})
	if err != nil {
		return nil, fmt.Errorf("list interrupted zombie tasks: %w", err)
	}
	reconciled := append(append([]uint{}, cancelIDs...), failIDs...)

	if len(cancelIDs) > 0 {
		if err := r.db.WithContext(ctx).Model(&DiagnosticTask{}).
			Where("id IN ?", cancelIDs).
			Updates(map[string]any{
				"status":       DiagnosticTaskStatusCancelled,
				"completed_at": now,
				"updated_at":   now,
			}).Error; err != nil {
			return nil, fmt.Errorf("reconcile cancelling diagnostic tasks: %w", err)
		}
	}
	if len(failIDs) > 0 {
		if err := r.db.WithContext(ctx).Model(&DiagnosticTask{}).
			Where("id IN ?", failIDs).
			Updates(map[string]any{
				"status":         DiagnosticTaskStatusFailed,
				"failure_reason": failureMessage,
				"completed_at":   now,
				"updated_at":     now,
			}).Error; err != nil {
			return nil, fmt.Errorf("reconcile interrupted diagnostic tasks: %w", err)
		}
	}

	// 收尾掉的任务可能有子行还停在非终态；按任务 id 把这些子行同步置终态，
	// 避免详情页永久转圈。skipped 只出现在步骤/节点执行上，任务本身不写 skipped，
	// 这里把任务表可能出现的 skipped 也视为可收尾的非终态。
	// Steps / node executions of reconciled tasks can remain non-terminal; close them with
	// the same terminal status so detail views stop spinning. "skipped" only appears on
	// child rows in practice, but any non-terminal status on a task is treated as closable.
	nonTerminal := []DiagnosticTaskStatus{
		DiagnosticTaskStatusPending, DiagnosticTaskStatusReady, DiagnosticTaskStatusRunning,
		DiagnosticTaskStatusCancelRequested, DiagnosticTaskStatusCancelling, DiagnosticTaskStatusSkipped,
	}
	closeChildren := func(taskIDs []uint, status DiagnosticTaskStatus) error {
		if len(taskIDs) == 0 {
			return nil
		}
		if err := r.db.WithContext(ctx).Model(&DiagnosticTaskStep{}).
			Where("task_id IN ? AND status IN ?", taskIDs, nonTerminal).
			Updates(map[string]any{"status": status, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := r.db.WithContext(ctx).Model(&DiagnosticNodeExecution{}).
			Where("task_id IN ? AND status IN ?", taskIDs, nonTerminal).
			Updates(map[string]any{"status": status, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return nil
	}
	if err := closeChildren(cancelIDs, DiagnosticTaskStatusCancelled); err != nil {
		return reconciled, fmt.Errorf("close cancelled task children: %w", err)
	}
	if err := closeChildren(failIDs, DiagnosticTaskStatusFailed); err != nil {
		return reconciled, fmt.Errorf("close failed task children: %w", err)
	}
	return reconciled, nil
}
