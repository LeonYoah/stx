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

package selfupgrade

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	stxversion "github.com/LeonYoah/stx/internal/version"
)

// restartHandoverDelay 是旧进程在交棒后退出前的等待，让正在进行的状态轮询拿到"重启中"。
// restartHandoverDelay is how long the old process waits before exiting so in-flight status polls see "restarting".
var restartHandoverDelay = 1200 * time.Millisecond

// restartResultGrace 是在 watchdog 超时之外再等待结果的余量。/ restartResultGrace is extra time to wait for the result.
var restartResultGrace = 30 * time.Second

func (s *Service) activeTaskPath() string  { return filepath.Join(s.stagingDir, activeTaskFile) }
func (s *Service) restartPlanPath() string { return filepath.Join(s.stagingDir, restartPlanFile) }

// planRestartDryRun 在演练中描述会重启哪些进程，不做任何变更。/ planRestartDryRun describes the restarts without changing anything.
func (s *Service) planRestartDryRun(taskID string) string {
	info := s.RestartPlanPreview()
	restarts := []ProcessRestart{{Component: "control_plane", Strategy: info.ControlPlaneStrategy, Status: RestartWouldDoThis,
		Message: "dry-run: would restart via " + info.ControlPlaneStrategy + " / 演练：将通过 " + info.ControlPlaneStrategy + " 重启"}}
	if info.FrontendDetected {
		restarts = append(restarts, ProcessRestart{Component: "frontend", Strategy: info.FrontendStrategy, Status: RestartWouldDoThis,
			Message: "dry-run: would restart the bundled frontend / 演练：将重启随包前端"})
	}
	s.mu.Lock()
	if s.task != nil && s.task.ID == taskID {
		s.task.Restarts = restarts
	}
	s.mu.Unlock()
	msg := fmt.Sprintf("dry-run: nothing restarted; an apply would restart the control plane (%s)", info.ControlPlaneStrategy)
	if info.FrontendDetected {
		msg += " and the bundled frontend"
	}
	return msg + ", Agents and running stx-java-proxy; SeaTunnel is not touched / 演练模式：未重启任何进程；真实应用会重启控制面、前端、Agent 与运行中的 stx-java-proxy，SeaTunnel 不受影响"
}

// beginControlPlaneRestart 持久化任务与重启计划、启动 watchdog，然后让控制面重启到新二进制。
// 返回 nil 表示已交棒（独立部署时本进程稍后退出），之后由新进程（或回滚后的旧进程）在 resumeAfterRestart 中收尾。
// beginControlPlaneRestart persists the task and restart plan, starts the watchdog and then restarts the control
// plane onto the new binary. nil means the handover happened (standalone: this process exits shortly); the new
// process (or the rolled-back old one) finishes in resumeAfterRestart.
func (s *Service) beginControlPlaneRestart(taskID, target string) error {
	from := stxversion.Current().Version
	plan, err := s.buildRestartPlan(taskID, target, from)
	if err != nil {
		return err
	}
	_ = os.Remove(plan.ReadyPath)
	_ = os.Remove(plan.ResultPath)

	now := time.Now().UTC()
	restarts := []ProcessRestart{{Component: "control_plane", Strategy: plan.Strategy.Kind, Status: RestartRunning, OldPID: plan.OldPID, At: &now,
		Message: "restarting onto " + target + " via " + plan.Strategy.Kind + " / 正在通过 " + plan.Strategy.Kind + " 重启到 " + target}}
	if plan.Frontend != nil {
		restarts = append(restarts, ProcessRestart{Component: "frontend", Strategy: frontendStrategyFor(plan.Strategy), Status: RestartPending, OldPID: plan.Frontend.PID, At: &now})
	}

	s.mu.Lock()
	if s.task == nil || s.task.ID != taskID {
		s.mu.Unlock()
		return fmt.Errorf("task %s is no longer active", taskID)
	}
	s.task.Restarts = restarts
	s.setStepLocked(StepRestart, StepRunning, "restarting STX processes; SeaTunnel is not touched / 正在重启 STX 进程，SeaTunnel 不受影响")
	snapshot := s.snapshotTaskLocked()
	s.mu.Unlock()

	if err := writeJSONFile(s.activeTaskPath(), snapshot); err != nil {
		return fmt.Errorf("persist task: %w", err)
	}
	// 进程管理器会连同前端一起重启：提前换入新前端目录（失败时 watchdog 会换回）。
	// A process manager restarts the frontend too: swap the new frontend dir in now (the watchdog swaps it back on failure).
	if fp := plan.Frontend; fp != nil && fp.ManagedBy != "" && fp.NewDir != "" && fp.AppDir != "" {
		_ = os.RemoveAll(fp.AppDir + ".bak")
		if err := os.Rename(fp.AppDir, fp.AppDir+".bak"); err == nil {
			if err := os.Rename(fp.NewDir, fp.AppDir); err == nil {
				fp.Swapped = true
			} else {
				_ = os.Rename(fp.AppDir+".bak", fp.AppDir)
			}
		}
	}
	if err := writeJSONFile(s.restartPlanPath(), plan); err != nil {
		_ = os.Remove(s.activeTaskPath())
		return fmt.Errorf("persist restart plan: %w", err)
	}
	if err := spawnWatchdogFn(s.restartPlanPath(), plan); err != nil {
		_ = os.Remove(s.activeTaskPath())
		_ = os.Remove(s.restartPlanPath())
		return fmt.Errorf("start restart watchdog: %w", err)
	}
	log.Printf("[self-upgrade] task %s: restarting control plane onto %s via %s (watchdog started)", taskID, target, plan.Strategy.Kind)

	if plan.Strategy.Kind == RestartStrategyStandalone {
		go func() {
			time.Sleep(restartHandoverDelay)
			restartExit()
		}()
		return nil
	}
	// 进程管理器可能没有重启本进程：此时 watchdog 会报告失败，由仍存活的本进程收尾。
	// The process manager may not restart this process; the watchdog then reports failure and this surviving
	// process finishes the task.
	go s.awaitRestartOutcome(taskID, plan, true)
	go func() {
		time.Sleep(restartHandoverDelay)
		if err := runManagerCommandFn(plan.Strategy.Command, plan.Env, plan.Dir); err != nil {
			log.Printf("[self-upgrade] restart command %v failed: %v", plan.Strategy.Command, err)
		}
	}()
	return nil
}

func (s *Service) setStepLocked(code, status, msg string) {
	if s.task == nil {
		return
	}
	now := time.Now().UTC()
	s.task.CurrentStep = code
	for i := range s.task.Steps {
		if s.task.Steps[i].Code != code {
			continue
		}
		s.task.Steps[i].Status = status
		s.task.Steps[i].Message = msg
		if status == StepRunning && s.task.Steps[i].StartedAt == nil {
			s.task.Steps[i].StartedAt = &now
		}
		if status != StepRunning && status != StepPending {
			s.task.Steps[i].EndedAt = &now
		}
	}
}

func (s *Service) snapshotTaskLocked() *TaskState {
	if s.task == nil {
		return nil
	}
	cp := *s.task
	cp.Steps = append([]StepState(nil), s.task.Steps...)
	cp.Restarts = append([]ProcessRestart(nil), s.task.Restarts...)
	cp.AgentNodes = append([]AgentNodeUpgrade(nil), s.task.AgentNodes...)
	return &cp
}

// resumeAfterRestart 在服务启动时检查是否有因自升级重启而中断的任务：写就绪标记供 watchdog 判断，再等待结果收尾。
// resumeAfterRestart runs at service start: if a task was handed over by a self-upgrade restart, write the ready
// marker for the watchdog and finish the task once the result is in.
func (s *Service) resumeAfterRestart() {
	var task TaskState
	if err := readJSONFile(s.activeTaskPath(), &task); err != nil || task.ID == "" {
		return
	}
	var plan restartPlan
	if err := readJSONFile(s.restartPlanPath(), &plan); err != nil || plan.TaskID != task.ID {
		// 没有计划却有任务：重启前被中断。/ a task without a plan: interrupted before the restart.
		s.mu.Lock()
		s.task = &task
		s.mu.Unlock()
		s.finishRestart(task.ID, &restartPlan{TaskID: task.ID, TargetVersion: task.TargetVersion}, &restartResult{
			Status: restartOutcomeFailed, Reason: "the control plane stopped during the upgrade before the restart handover / 控制面在重启交棒前中断",
		})
		return
	}
	s.mu.Lock()
	s.task = &task
	s.mu.Unlock()
	_ = writeJSONFile(plan.ReadyPath, readyMarker{PID: os.Getpid(), Version: stxversion.Current().Version, TaskID: task.ID})
	log.Printf("[self-upgrade] resumed task %s after restart (running %s, target %s)", task.ID, stxversion.Current().Version, plan.TargetVersion)
	go s.awaitRestartOutcome(task.ID, &plan, false)
}

// awaitRestartOutcome 等待 watchdog 结果并收尾；只有一个进程会真正收尾（任务文件被删除后其余调用无效）。
// awaitRestartOutcome waits for the watchdog result and finishes; only one process finishes (later calls are no-ops
// once the task file is gone).
//
// handover 为 true 表示调用方是交棒的旧进程：成功结果属于新进程，旧进程不处理。
// handover is true for the old, handing-over process: a success result belongs to the new process.
func (s *Service) awaitRestartOutcome(taskID string, plan *restartPlan, handover bool) {
	deadline := time.Now().Add(time.Duration(plan.TimeoutSeconds)*time.Second*2 + oldProcessExitTimeout + restartResultGrace)
	for time.Now().Before(deadline) {
		var res restartResult
		if err := readJSONFile(plan.ResultPath, &res); err == nil && res.Status != "" {
			// 结果为成功但本进程是旧版本：说明本进程即将被替换，交给新进程。
			// A success result while this process is the old version means this process is about to be replaced.
			if res.Status == restartOutcomeSucceeded && (handover || !sameVersion(stxversion.Current().Version, plan.TargetVersion)) {
				return
			}
			s.finishRestart(taskID, plan, &res)
			return
		}
		time.Sleep(watchdogPoll)
	}
	s.finishRestart(taskID, plan, nil)
}

// finishRestart 根据 watchdog 结果与本进程版本完成任务、写历史；失败时回滚 Agent 与 proxy。
// finishRestart completes the task and history from the watchdog result and this process's version; on failure it
// rolls back Agents and proxies.
func (s *Service) finishRestart(taskID string, plan *restartPlan, res *restartResult) {
	s.mu.Lock()
	if s.task == nil || s.task.ID != taskID || s.task.Status != StatusUpgrading {
		s.mu.Unlock()
		return
	}
	if _, err := os.Stat(s.activeTaskPath()); err != nil {
		s.mu.Unlock()
		return
	}
	cur := stxversion.Current().Version
	onTarget := sameVersion(cur, plan.TargetVersion)
	// 合并重启结果 / merge restart results
	if res != nil {
		merged := []ProcessRestart{res.ControlPlane}
		if res.ControlPlane.Component == "" {
			merged = merged[:0]
		}
		if res.Frontend != nil {
			merged = append(merged, *res.Frontend)
		}
		if len(merged) > 0 {
			s.task.Restarts = merged
		}
	}
	succeeded := onTarget && (res == nil || res.Status == restartOutcomeSucceeded)
	if onTarget && res != nil && res.Status != restartOutcomeSucceeded {
		// 理论上不会发生：已在目标版本却报告失败，以实际运行版本为准。/ should not happen; trust the running version.
		succeeded = true
	}
	_ = os.Remove(s.activeTaskPath())
	_ = os.Remove(s.restartPlanPath())
	_ = os.Remove(plan.ReadyPath)
	_ = os.Remove(plan.ResultPath)

	if succeeded {
		msg := fmt.Sprintf("control plane restarted on %s / 控制面已以 %s 重启", cur, cur)
		if res == nil {
			msg += " (watchdog did not report; verified by the running version / watchdog 未上报，按实际运行版本确认)"
		}
		stepStatus := StepSucceeded
		for _, r := range s.task.Restarts {
			if r.Component == "frontend" && r.Status != RestartSucceeded {
				stepStatus = StepPartial
				msg += "; frontend: " + r.Message
			}
		}
		s.setStepLocked(StepRestart, stepStatus, msg)
		s.setStepLocked(StepVerify, StepSucceeded, "control plane is running "+cur+" / 控制面正在运行 "+cur)
		now := time.Now().UTC()
		s.task.Status = StatusSucceeded
		s.task.CurrentStep = ""
		s.task.Cancellable = false
		s.task.CompletedAt = &now
		s.updateAvailable = false
		s.latestVersion = plan.TargetVersion
		s.stage.Message = "upgrade completed successfully"
		s.recordHistoryLocked(s.historyRecordLocked(plan.FromVersion, StatusSucceeded, "", "", ""))
		s.mu.Unlock()
		return
	}

	// 失败：本进程运行的是旧版本（watchdog 已回滚或重启未发生）。确保二进制恢复为备份。
	// Failure: this process runs the old version (watchdog rolled back, or the restart never happened). Make sure
	// the binary is the backup again.
	reason := "control plane did not come back on " + plan.TargetVersion
	status := StatusFailed
	if res != nil {
		if res.Reason != "" {
			reason = res.Reason
		}
		if res.Status == restartOutcomeRolledBack {
			status = "rolled_back"
		}
	}
	if binaryAlreadyRestored(restartExecutable()) {
		// watchdog 已把备份恢复为可执行文件并以其启动本进程；不要再覆盖正在运行的二进制。
		// The watchdog already restored the backup and started this process from it; do not replace the running binary again.
		status = "rolled_back"
	} else if s.rollbackLocked(plan.TargetVersion, StepRestart) {
		status = "rolled_back"
	}
	now := time.Now().UTC()
	s.setStepLocked(StepRestart, StepFailed, reason)
	for i := range s.task.Steps {
		if s.task.Steps[i].Code == StepRestart {
			s.task.Steps[i].Error = reason
		}
	}
	s.task.Status = StatusFailed
	s.task.FailureStep = StepRestart
	s.task.FailureReason = reason
	s.task.ErrorCode = CodeVerifyFail
	s.task.Cancellable = false
	s.task.CompletedAt = &now
	s.stage.Message = "upgrade failed and was rolled back / 升级失败已回滚"
	s.recordHistoryLocked(s.historyRecordLocked(plan.FromVersion, status, StepRestart, reason, CodeVerifyFail))
	target := plan.TargetVersion
	s.mu.Unlock()

	// 控制面已回到旧版本：把 Agent 与 proxy 也恢复到备份，保持一致。
	// The control plane is back on the old version: restore Agents and proxies to their backups as well.
	go func() {
		ctx := context.Background()
		deadline := time.Now().Add(time.Minute)
		for {
			if f, _ := s.fleetAndOptions(); f != nil || time.Now().After(deadline) {
				break
			}
			time.Sleep(watchdogPoll)
		}
		s.rollbackUpgradedAgents(ctx, taskID, target)
		s.mu.Lock()
		if s.task != nil && s.task.ID == taskID {
			// 用回滚后的节点状态刷新历史。/ refresh the history with the rolled-back node states.
			list := s.loadHistoryLocked()
			for i := range list {
				if list[i].ID == taskID {
					list[i].AgentNodes = append([]AgentNodeUpgrade(nil), s.task.AgentNodes...)
				}
			}
			s.saveHistoryLocked(list)
		}
		s.mu.Unlock()
	}()
}

// historyRecordLocked 由当前任务生成历史记录。/ historyRecordLocked builds a history record from the current task.
func (s *Service) historyRecordLocked(from, status, failureStep, reason, code string) UpgradeHistoryRecord {
	now := time.Now().UTC()
	started := now
	if s.task.StartedAt != nil {
		started = *s.task.StartedAt
	}
	depType := "host"
	if IsRunningInDocker() {
		depType = "docker"
	}
	mode := s.stage.Mode
	if mode == "" {
		mode = ModeOnline
	}
	if from == "" {
		from = stxversion.Current().Version
	}
	return UpgradeHistoryRecord{
		ID:             s.task.ID,
		StartedAt:      started,
		CompletedAt:    now,
		DurationMs:     now.Sub(started).Milliseconds(),
		FromVersion:    from,
		TargetVersion:  s.task.TargetVersion,
		Mode:           mode,
		DeploymentType: depType,
		Status:         status,
		FailureStep:    failureStep,
		FailureReason:  reason,
		ErrorCode:      code,
		ExecutionMode:  s.task.ExecutionMode,
		DryRun:         s.task.DryRun,
		Restarts:       append([]ProcessRestart(nil), s.task.Restarts...),
		Steps:          append([]StepState(nil), s.task.Steps...),
		AgentNodes:     append([]AgentNodeUpgrade(nil), s.task.AgentNodes...),
	}
}

// binaryAlreadyRestored 判断可执行文件是否已与其 .bak 内容一致（即 watchdog 已完成恢复）。
// binaryAlreadyRestored reports whether the executable already matches its .bak (the watchdog restored it).
func binaryAlreadyRestored(exe string) bool {
	a, err := sha256File(exe)
	if err != nil {
		return false
	}
	b, err := sha256File(exe + ".bak")
	return err == nil && a == b
}
