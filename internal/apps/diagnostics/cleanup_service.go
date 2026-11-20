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
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// inspectionReconcileStaleWindow 是本进程内「理论上还在跑但其实挂死」的兜底窗口：
// 巡检评估同步执行、任务步骤分钟级，超过一天还非终态只能视为僵尸。
// inspectionReconcileStaleWindow is the fallback stale window for rows stuck non-terminal
// inside the current process: evaluation runs inline, so anything non-terminal for more
// than a day can only be a zombie.
const inspectionReconcileStaleWindow = 24 * time.Hour

// inspectionSweepBatchLimit 每轮自动清理的删除上限，避免一次扫出几千条时长事务。
// inspectionSweepBatchLimit caps how many records one sweep deletes per round.
const inspectionSweepBatchLimit = 200

// interruptedInspectionMessage / interruptedDiagnosticTaskMessage 是 reconcile 写回的
// 双语终态原因，前端按既有 localize 通道直接展示。
// interruptedInspectionMessage / interruptedDiagnosticTaskMessage are the bilingual
// terminal reasons written by reconcile; the frontend renders them through the existing
// localize channel.
func interruptedInspectionMessage() string {
	return bilingualText("进程重启，巡检被中断。", "The inspection was interrupted by a restart.")
}
func interruptedDiagnosticTaskMessage() string {
	return bilingualText("进程重启，诊断任务被中断。", "The diagnostic task was interrupted by a restart.")
}

// GetInspectionCleanupSettings 返回全局巡检清理设置（无行时返回 0/0 默认不清理）。
// GetInspectionCleanupSettings returns the global inspection cleanup settings
// (0/0 "no cleanup" when the row was never written).
func (s *Service) GetInspectionCleanupSettings(ctx context.Context) (*InspectionCleanupSettingsInfo, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	row, err := s.repo.GetInspectionCleanupSettings(ctx)
	if err != nil {
		return nil, err
	}
	return row.ToInfo(), nil
}

// SaveInspectionCleanupSettings 校验并保存全局清理设置；两个字段都必须显式提供
// （完整替换语义），nil 视为非法请求。
// SaveInspectionCleanupSettings validates and persists the global cleanup settings; both
// fields must be provided explicitly (full-replace semantics); nil is rejected.
func (s *Service) SaveInspectionCleanupSettings(ctx context.Context, req *UpdateInspectionCleanupSettingsRequest) (*InspectionCleanupSettingsInfo, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if req == nil || req.RetentionDays == nil || req.MaxReports == nil {
		return nil, fmt.Errorf("%w: retention_days and max_reports are required", ErrInvalidInspectionCleanupRequest)
	}
	if *req.RetentionDays < 0 || *req.RetentionDays > maxInspectionRetentionDays {
		return nil, fmt.Errorf("%w: retention_days must be between 0 and %d", ErrInvalidInspectionCleanupRequest, maxInspectionRetentionDays)
	}
	if *req.MaxReports < 0 || *req.MaxReports > maxInspectionReportKeep {
		return nil, fmt.Errorf("%w: max_reports must be between 0 and %d", ErrInvalidInspectionCleanupRequest, maxInspectionReportKeep)
	}
	row, err := s.repo.SaveInspectionCleanupSettings(ctx, *req.RetentionDays, *req.MaxReports)
	if err != nil {
		return nil, err
	}
	return row.ToInfo(), nil
}

// DeleteInspectionReports 批量删除巡检记录并级联清理绑定物：
// 发现项、source_ref.inspection_report_id 命中的诊断任务（含子表）及其磁盘 bundle 目录。
// 逐条判定结果：pending/running 记录拒删，绑定任务非终态的记录本次跳过。
// DeleteInspectionReports batch-deletes inspection records and cascades their bound
// objects: findings, diagnostic tasks whose source_ref.inspection_report_id matches
// (with child rows) and their on-disk bundle dirs. Each id gets its own outcome: reports
// still pending/running are refused, records whose bound tasks are not terminal are
// skipped this round.
func (s *Service) DeleteInspectionReports(ctx context.Context, ids []uint) ([]*InspectionDeleteResult, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	unique := dedupeUintIDs(ids)
	results := make([]*InspectionDeleteResult, 0, len(unique))
	if len(unique) == 0 {
		return results, nil
	}
	if len(unique) > inspectionDeleteBatchLimit {
		return nil, fmt.Errorf("%w: at most %d ids per request", ErrInvalidInspectionCleanupRequest, inspectionDeleteBatchLimit)
	}

	reports := make(map[uint]*ClusterInspectionReport, len(unique))
	for _, id := range unique {
		report, err := s.repo.GetInspectionReportByID(ctx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrInspectionReportNotFound) {
				results = append(results, &InspectionDeleteResult{ID: id, Result: InspectionDeleteResultNotFound})
				continue
			}
			return nil, err
		}
		reports[id] = report
	}

	candidateIDs := make([]uint, 0, len(unique))
	for _, id := range unique {
		report := reports[id]
		if report == nil {
			continue
		}
		if report.Status == InspectionReportStatusPending || report.Status == InspectionReportStatusRunning {
			results = append(results, &InspectionDeleteResult{
				ID:     id,
				Result: InspectionDeleteResultReportRunning,
				Reason: "inspection report is not finished yet",
			})
			continue
		}
		candidateIDs = append(candidateIDs, id)
	}

	boundTasks, err := s.repo.ListDiagnosticTasksByInspectionReportIDs(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}
	blocked := map[uint]*DiagnosticTask{}
	terminalTasksByReport := map[uint][]uint{}
	for _, task := range boundTasks {
		if task == nil {
			continue
		}
		reportID := task.SourceRef.InspectionReportID
		if !isTerminalDiagnosticTaskStatus(task.Status) {
			if _, seen := blocked[reportID]; !seen {
				blocked[reportID] = task
			}
			continue
		}
		terminalTasksByReport[reportID] = append(terminalTasksByReport[reportID], task.ID)
	}

	deletableReportIDs := make([]uint, 0, len(candidateIDs))
	deletableTaskIDs := make([]uint, 0, len(boundTasks))
	for _, id := range candidateIDs {
		if task, blockedBy := blocked[id]; blockedBy {
			results = append(results, &InspectionDeleteResult{
				ID:     id,
				Result: InspectionDeleteResultTaskRunning,
				Reason: fmt.Sprintf("bound diagnostic task #%d is %s", task.ID, task.Status),
				TaskID: task.ID,
			})
			continue
		}
		deletableReportIDs = append(deletableReportIDs, id)
		// 只收编归属可删报告的绑定任务：报告被阻塞跳过时它的任务必须一并留下，
		// 否则活报告会留下指向已删任务的悬空绑定。
		// Only sweep up tasks bound to deletable reports: a skipped report must
		// keep all its tasks, otherwise the live report ends up with a dangling
		// binding to a deleted task.
		deletableTaskIDs = append(deletableTaskIDs, terminalTasksByReport[id]...)
	}

	var bundleDirs []string
	if len(deletableReportIDs) > 0 {
		err = s.repo.Transaction(ctx, func(tx *Repository) error {
			var cascadeErr error
			bundleDirs, cascadeErr = tx.DeleteInspectionReportsCascade(ctx, deletableReportIDs, deletableTaskIDs)
			return cascadeErr
		})
		if err != nil {
			return nil, err
		}
		for _, dir := range bundleDirs {
			if removeErr := removeDiagnosticBundleDir(dir); removeErr != nil {
				logger.WarnF(ctx, "[Diagnostics] 删除诊断 bundle 目录失败: dir=%s err=%v", dir, removeErr)
			}
		}
		for _, id := range deletableReportIDs {
			results = append(results, &InspectionDeleteResult{ID: id, Result: InspectionDeleteResultDeleted})
		}
	}

	// 结果按请求 id 原顺序返回，前端按 id 回显更直观。
	// Keep results in request order so the panel can echo outcomes per row.
	byID := make(map[uint]*InspectionDeleteResult, len(results))
	for _, item := range results {
		byID[item.ID] = item
	}
	ordered := make([]*InspectionDeleteResult, 0, len(unique))
	for _, id := range unique {
		if item := byID[id]; item != nil {
			ordered = append(ordered, item)
		}
	}
	return ordered, nil
}

// removeDiagnosticBundleDir 只删 storage 诊断任务目录内的路径：
// bundle_dir 来自 DB，删除前强制落在 tasks 根目录之下，防止脏数据误删其他目录。
// removeDiagnosticBundleDir only removes paths inside the storage diagnostics task root:
// bundle_dir comes from the DB, so deletion is gated on the tasks root prefix to keep
// polluted values from wiping unrelated directories.
func removeDiagnosticBundleDir(dir string) error {
	clean := filepath.Clean(strings.TrimSpace(dir))
	if clean == "." || clean == "" {
		return nil
	}
	base := strings.TrimSpace(config.GetStorageConfig().BaseDir)
	if base == "" {
		base = "./data/storage"
	}
	tasksRoot := filepath.Clean(filepath.Join(base, "diagnostics", "tasks"))
	// 严格要求落在 tasks 根目录之内（不含根本身），脏数据不能把整棵任务树清掉。
	// The path must sit strictly inside the tasks root (never the root itself) so a
	// polluted bundle_dir cannot wipe the whole task tree.
	if !strings.HasPrefix(clean, tasksRoot+string(filepath.Separator)) {
		return fmt.Errorf("refuse to remove path outside diagnostics task root: %s", clean)
	}
	return os.RemoveAll(clean)
}

// sweepExpiredInspectionReports 按全局设置清一轮过期巡检记录；设置为 0/0 时直接返回。
// sweepExpiredInspectionReports deletes expired inspection records per the global
// settings; a 0/0 settings short-circuits to a no-op.
func (s *Service) sweepExpiredInspectionReports(ctx context.Context, now time.Time) {
	if s == nil || s.repo == nil {
		return
	}
	settings, err := s.repo.GetInspectionCleanupSettings(ctx)
	if err != nil {
		logger.WarnF(ctx, "[DiagnosticsCleanup] 读取清理设置失败: %v", err)
		return
	}
	if settings.RetentionDays <= 0 && settings.MaxReports <= 0 {
		return
	}
	ids, err := s.repo.ListExpiredInspectionReportIDs(ctx, settings.RetentionDays, settings.MaxReports, now, inspectionSweepBatchLimit)
	if err != nil {
		logger.WarnF(ctx, "[DiagnosticsCleanup] 计算待清理巡检记录失败: %v", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	results, err := s.DeleteInspectionReports(ctx, ids)
	if err != nil {
		logger.WarnF(ctx, "[DiagnosticsCleanup] 自动清理巡检记录失败: %v", err)
		return
	}
	var deleted, skipped int
	for _, item := range results {
		if item != nil && item.Result == InspectionDeleteResultDeleted {
			deleted++
		} else {
			skipped++
		}
	}
	if deleted > 0 || skipped > 0 {
		logger.InfoF(ctx, "[DiagnosticsCleanup] 巡检记录清理完成: deleted=%d skipped=%d", deleted, skipped)
	}
}

// ReconcileInterruptedDiagnostics 在进程启动时收尾重启遗留的非终态对象：
// 巡检报告 pending/running → failed，诊断任务按原意图 → cancelled / failed，
// 非终态子行同步置终态。判定带宽限期（updated_at 早于进程启动或超过僵死窗口），
// 不会误收尾启动后新创建的在途对象。
// ReconcileInterruptedDiagnostics closes non-terminal objects orphaned by a restart at
// process start: pending/running inspection reports become failed, diagnostic tasks are
// closed along their intent (cancelled / failed) and their non-terminal child rows get the
// same terminal status. The zombie predicate is grace-bounded (updated_at before process
// start or beyond the stale window) so in-flight rows created after startup are never
// touched.
func (s *Service) ReconcileInterruptedDiagnostics(ctx context.Context, processStartedAt time.Time) {
	if s == nil || s.repo == nil {
		return
	}
	staleBefore := timex.Now().Add(-inspectionReconcileStaleWindow)

	reportIDs, err := s.repo.ReconcileInterruptedInspectionReports(ctx, processStartedAt, staleBefore, interruptedInspectionMessage())
	if err != nil {
		logger.WarnF(ctx, "[Diagnostics] 收尾中断巡检记录失败: %v", err)
	} else if len(reportIDs) > 0 {
		logger.InfoF(ctx, "[Diagnostics] 已收尾 %d 条重启中断的巡检记录", len(reportIDs))
	}

	taskIDs, err := s.repo.ReconcileInterruptedDiagnosticTasks(ctx, processStartedAt, staleBefore, interruptedDiagnosticTaskMessage())
	if err != nil {
		logger.WarnF(ctx, "[Diagnostics] 收尾中断诊断任务失败: %v", err)
		return
	}
	if len(taskIDs) == 0 {
		return
	}
	logger.InfoF(ctx, "[Diagnostics] 已收尾 %d 个重启中断的诊断任务", len(taskIDs))
	// 同步公共执行记录并广播任务终态，让执行中心和订阅方不再显示 running。
	// Sync shared execution rows and publish the terminal state so the execution center
	// and subscribers stop showing these tasks as running.
	for _, taskID := range taskIDs {
		task, loadErr := s.repo.GetDiagnosticTaskByID(ctx, taskID)
		if loadErr != nil || task == nil {
			continue
		}
		if syncErr := s.syncExecutionFromDiagnosticTask(ctx, task); syncErr != nil {
			logger.WarnF(ctx, "[Diagnostics] 同步执行终态失败: task_id=%d err=%v", taskID, syncErr)
		}
		s.publishDiagnosticTaskEvent(newDiagnosticTaskUpdatedEvent(task))
	}
}

func dedupeUintIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	// 顺序保持稳定（按请求原序去重，不排序）。
	// Order stays stable (deduped in request order, not sorted).
	return out
}
