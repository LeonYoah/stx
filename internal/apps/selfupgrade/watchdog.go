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
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	stxversion "github.com/LeonYoah/stx/internal/version"
)

// RunRestartWatchdog 是隐藏子命令 __self-upgrade-restart 的入口，由旧二进制在独立会话中运行：
//  1. 等待旧控制面退出（进程管理器策略下由管理器负责停止并重启）；
//  2. 独立部署时以相同 argv/env/cwd/输出拉起新二进制；
//  3. 等待新进程写入就绪标记（版本 = 目标版本）且健康接口返回 200；
//  4. 失败时恢复备份二进制并以同样方式拉起旧版本，结果记为 rolled_back；
//  5. 成功后重启随包前端（失败则恢复旧前端目录）；
//  6. 把结果写入 restart-result.json，由存活的控制面收尾任务与历史。
//
// 它只操作 STX 自身的控制面与前端进程，从不触碰 SeaTunnel/Hazelcast。
//
// RunRestartWatchdog is the entry of the hidden __self-upgrade-restart sub-command, run detached by the old binary:
//  1. wait for the old control plane to exit (process managers stop and restart it themselves);
//  2. in standalone deployments start the new binary with the same argv/env/cwd/output;
//  3. wait for the ready marker (version = target) and a 200 from the health endpoint;
//  4. on failure restore the backup binary, start the old version the same way and report rolled_back;
//  5. after success restart the bundled frontend (restoring the old frontend dir if it fails);
//  6. write restart-result.json for the surviving control plane to finish the task and history.
//
// It only operates on STX's own control plane and frontend processes, never on SeaTunnel/Hazelcast.
func RunRestartWatchdog(planPath string) error {
	// 绝对化 --plan：systemd-run 单元默认 cwd=/，相对路径会读不到交棒计划（DEF-UPG-1）。
	// Absolutize --plan: systemd-run units default to cwd=/ and miss a relative handover plan (DEF-UPG-1).
	planPath = absPath(planPath)
	var plan restartPlan
	if err := readJSONFile(planPath, &plan); err != nil {
		return fmt.Errorf("read restart plan: %w", err)
	}
	absolutizeRestartPlan(&plan)
	res := watchdogRun(context.Background(), &plan)
	res.At = time.Now().UTC()
	log.Printf("[self-upgrade watchdog] task %s finished: %s %s", plan.TaskID, res.Status, res.Reason)
	return writeJSONFile(plan.ResultPath, res)
}

func watchdogRun(ctx context.Context, plan *restartPlan) restartResult {
	cp := ProcessRestart{Component: "control_plane", Strategy: plan.Strategy.Kind, OldPID: plan.OldPID, Status: RestartRunning}
	standalone := plan.Strategy.Kind == RestartStrategyStandalone
	timeout := time.Duration(plan.TimeoutSeconds) * time.Second

	if !waitProcessExit(plan.OldPID, oldProcessExitTimeout) {
		if !standalone {
			cp.Status = RestartFailed
			cp.Message = fmt.Sprintf("%s did not restart the control plane within %s / 进程管理器未在规定时间内重启控制面", plan.Strategy.Kind, oldProcessExitTimeout)
			return restartResult{Status: restartOutcomeFailed, Reason: cp.Message, ControlPlane: cp}
		}
		// 旧进程未按计划退出：先温和再强制，仅针对控制面自身 PID。/ old process did not exit: TERM then KILL, own PID only.
		_ = syscall.Kill(plan.OldPID, syscall.SIGTERM)
		if !waitProcessExit(plan.OldPID, 10*time.Second) {
			_ = syscall.Kill(plan.OldPID, syscall.SIGKILL)
			waitProcessExit(plan.OldPID, 5*time.Second)
		}
	}

	newPID, err := startControlPlane(plan, false)
	if err == nil {
		newPID, err = waitControlPlaneReady(ctx, plan, newPID, plan.TargetVersion, timeout)
	}
	if err == nil {
		cp.Status, cp.NewPID = RestartSucceeded, newPID
		cp.Message = fmt.Sprintf("control plane restarted on %s (pid %d → %d) / 控制面已以 %s 重启", plan.TargetVersion, plan.OldPID, newPID, plan.TargetVersion)
		res := restartResult{Status: restartOutcomeSucceeded, ControlPlane: cp}
		if plan.Frontend != nil {
			fr := restartFrontend(ctx, plan)
			res.Frontend = &fr
		}
		return res
	}

	// 新版本未能就绪：停止它、恢复备份并拉起旧版本。/ new version not ready: stop it, restore the backup, start the old one.
	reason := err.Error()
	log.Printf("[self-upgrade watchdog] new control plane not ready: %s; rolling back", reason)
	if standalone && newPID > 0 && processAlive(newPID) {
		_ = syscall.Kill(newPID, syscall.SIGTERM)
		if !waitProcessExit(newPID, 10*time.Second) {
			_ = syscall.Kill(newPID, syscall.SIGKILL)
			waitProcessExit(newPID, 5*time.Second)
		}
	}
	if fp := plan.Frontend; fp != nil && fp.Swapped {
		// 进程管理器重启前换入的新前端随之恢复。/ restore the frontend dir swapped in before the manager restart.
		_ = os.RemoveAll(fp.AppDir + ".failed")
		_ = os.Rename(fp.AppDir, fp.AppDir+".failed")
		_ = os.Rename(fp.AppDir+".bak", fp.AppDir)
	}
	if rerr := restoreBinary(plan.Backup, plan.Executable); rerr != nil {
		cp.Status = RestartFailed
		cp.Message = fmt.Sprintf("new control plane failed (%s) and restoring the backup failed: %v / 新控制面失败且恢复备份失败", reason, rerr)
		return restartResult{Status: restartOutcomeFailed, Reason: cp.Message, ControlPlane: cp}
	}
	_ = os.Remove(plan.ReadyPath)
	oldPID, err := startControlPlane(plan, true)
	if err == nil {
		oldPID, err = waitControlPlaneReady(ctx, plan, oldPID, plan.FromVersion, timeout)
	}
	if err != nil {
		cp.Status = RestartFailed
		cp.Message = fmt.Sprintf("new control plane failed (%s); restored %s but it did not come back: %v / 新控制面失败，已恢复旧版本但未能启动", reason, plan.FromVersion, err)
		return restartResult{Status: restartOutcomeFailed, Reason: cp.Message, ControlPlane: cp}
	}
	cp.Status, cp.NewPID = RestartRolledBack, oldPID
	cp.Message = fmt.Sprintf("new control plane failed (%s); restored %s and restarted it (pid %d) / 新控制面启动失败，已恢复 %s 并重启", reason, plan.FromVersion, oldPID, plan.FromVersion)
	return restartResult{Status: restartOutcomeRolledBack, Reason: reason, ControlPlane: cp}
}

// startControlPlane 独立部署时直接拉起；进程管理器部署时发出（或依赖已发出的）重启命令，返回 0 表示 PID 由就绪标记给出。
// startControlPlane starts the binary directly in standalone mode; with a process manager it relies on (or issues)
// the restart command and returns 0, meaning the PID comes from the ready marker.
func startControlPlane(plan *restartPlan, rollback bool) (int, error) {
	if plan.Strategy.Kind != RestartStrategyStandalone {
		// 首次重启命令由旧进程发出；回滚时需要管理器用恢复后的二进制再重启一次。
		// The first restart command is issued by the old process; a rollback needs the manager to restart again
		// with the restored binary.
		if rollback && len(plan.Strategy.Command) > 0 {
			return 0, runManagerCommandFn(plan.Strategy.Command, plan.Env, plan.Dir)
		}
		return 0, nil
	}
	argv := plan.Argv
	if len(argv) == 0 {
		argv = []string{plan.Executable, "server"}
	}
	cmd := exec.Command(plan.Executable)
	// 保留原始 argv（含 argv[0]），与原进程的命令行完全一致。/ keep the original argv (incl. argv[0]) verbatim.
	cmd.Args = append([]string(nil), argv...)
	cmd.Env = plan.Env
	cmd.Dir = plan.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var files []*os.File
	open := func(path string) *os.File {
		if path == "" {
			return nil
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil
		}
		files = append(files, f)
		return f
	}
	if f := open(plan.StdoutPath); f != nil {
		cmd.Stdout = f
	}
	if plan.StderrPath == plan.StdoutPath && cmd.Stdout != nil {
		cmd.Stderr = cmd.Stdout
	} else if f := open(plan.StderrPath); f != nil {
		cmd.Stderr = f
	}
	defer func() {
		for _, f := range files {
			_ = f.Close()
		}
	}()
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start %s: %w", plan.Executable, err)
	}
	pid := cmd.Process.Pid
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	watchedExits.store(pid, exited)
	if plan.BackendPIDFile != "" {
		_ = os.WriteFile(plan.BackendPIDFile, []byte(strconv.Itoa(pid)+"\n"), 0o644)
	}
	return pid, nil
}

// waitControlPlaneReady 等待就绪标记（版本匹配、进程存活）与健康接口。/ waitControlPlaneReady waits for the ready marker and health.
func waitControlPlaneReady(ctx context.Context, plan *restartPlan, pid int, wantVersion string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	exited := watchedExits.load(pid)
	for {
		if exited != nil {
			select {
			case <-exited:
				return pid, fmt.Errorf("control plane %s exited during startup / 控制面 %s 启动过程中退出", wantVersion, wantVersion)
			default:
			}
		}
		var m readyMarker
		if err := readJSONFile(plan.ReadyPath, &m); err == nil && m.PID > 0 && m.PID != plan.OldPID && processAlive(m.PID) &&
			(pid == 0 || m.PID == pid) && stxversion.Normalize(m.Version) == stxversion.Normalize(wantVersion) {
			if err := healthCheckFn(ctx, plan.HealthURL); err == nil {
				return m.PID, nil
			}
		}
		if time.Now().After(deadline) {
			return pid, fmt.Errorf("control plane %s was not ready within %s / 控制面 %s 未在 %s 内就绪", wantVersion, timeout, wantVersion, timeout)
		}
		time.Sleep(watchdogPoll)
	}
}

func waitProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(watchdogPoll)
	}
	return true
}

// restoreBinary 用备份覆盖可执行文件，失败的新二进制保留为 .failed 便于排查。
// restoreBinary overwrites the executable with the backup; the failed new binary is kept as .failed for diagnosis.
func restoreBinary(backup, exe string) error {
	if fi, err := os.Stat(backup); err != nil || fi.Size() == 0 {
		return fmt.Errorf("backup %s missing", backup)
	}
	_ = os.Remove(exe + ".failed")
	_ = os.Rename(exe, exe+".failed")
	tmp := exe + ".restore"
	if err := copyFile(backup, tmp); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o755)
	return os.Rename(tmp, exe)
}

// restartFrontend 替换（若有新版本）并重启随包前端；失败时恢复旧目录并重启。进程管理器托管时只校验。
// restartFrontend swaps in the new frontend (when present) and restarts it; on failure it restores the old dir and
// restarts again. When a process manager owns it, only verification is done.
func restartFrontend(ctx context.Context, plan *restartPlan) ProcessRestart {
	fp := plan.Frontend
	pr := ProcessRestart{Component: "frontend", Strategy: RestartStrategyStandalone, OldPID: fp.PID}
	if fp.ManagedBy != "" {
		pr.Strategy = fp.ManagedBy
		newPID := 0
		deadline := time.Now().Add(time.Duration(plan.TimeoutSeconds) * time.Second)
		for time.Now().Before(deadline) {
			if raw, err := os.ReadFile(fp.PIDFile); err == nil {
				if p, err := strconv.Atoi(stringsTrim(raw)); err == nil && p != fp.PID && processAlive(p) {
					newPID = p
					break
				}
			}
			time.Sleep(watchdogPoll)
		}
		if newPID == 0 {
			pr.Status, pr.Message = RestartFailed, "frontend did not come back after the "+fp.ManagedBy+" restart / 前端未随服务重启恢复"
			return pr
		}
		pr.Status, pr.NewPID = RestartSucceeded, newPID
		pr.Message = "frontend restarted by " + fp.ManagedBy + " / 前端已随服务重启"
		return pr
	}

	swapped := false
	if fp.NewDir != "" && fp.AppDir != "" {
		bak := fp.AppDir + ".bak"
		_ = os.RemoveAll(bak)
		if err := os.Rename(fp.AppDir, bak); err == nil {
			if err := os.Rename(fp.NewDir, fp.AppDir); err != nil {
				_ = os.Rename(bak, fp.AppDir)
			} else {
				swapped = true
			}
		}
	}
	stopFrontend(fp.PID)
	pid, err := startFrontend(fp)
	if err == nil {
		err = waitFrontendReady(ctx, fp, pid, time.Duration(plan.TimeoutSeconds)*time.Second)
	}
	if err == nil {
		pr.Status, pr.NewPID = RestartSucceeded, pid
		pr.Message = fmt.Sprintf("frontend restarted (pid %d → %d) / 前端已重启", fp.PID, pid)
		if swapped {
			pr.Message = fmt.Sprintf("frontend replaced and restarted (pid %d → %d) / 前端已替换并重启", fp.PID, pid)
		}
		return pr
	}
	firstErr := err
	stopFrontend(pid)
	if swapped {
		_ = os.RemoveAll(fp.AppDir + ".failed")
		_ = os.Rename(fp.AppDir, fp.AppDir+".failed")
		_ = os.Rename(fp.AppDir+".bak", fp.AppDir)
	}
	pid, err = startFrontend(fp)
	if err == nil {
		err = waitFrontendReady(ctx, fp, pid, time.Duration(plan.TimeoutSeconds)*time.Second)
	}
	if err != nil {
		pr.Status = RestartFailed
		pr.Message = fmt.Sprintf("frontend failed to start (%v) and the previous frontend did not come back (%v) / 前端启动失败且未能恢复", firstErr, err)
		return pr
	}
	pr.Status, pr.NewPID = RestartRolledBack, pid
	pr.Message = fmt.Sprintf("new frontend failed (%v); previous frontend restored and restarted (pid %d) / 新前端启动失败，已恢复旧前端", firstErr, pid)
	return pr
}

func stringsTrim(b []byte) string {
	s := string(b)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// stopFrontend 只在命令行确认是 node server.js 时才发送信号。/ stopFrontend only signals a verified `node server.js` process.
func stopFrontend(pid int) {
	if pid <= 0 || !processAlive(pid) {
		return
	}
	if argv, _, _, _, err := snapshotProcess(pid); err != nil || !isFrontendCommandLine(argv) {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	if !waitProcessExit(pid, 15*time.Second) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		waitProcessExit(pid, 5*time.Second)
	}
}

func startFrontend(fp *frontendPlan) (int, error) {
	cmd := exec.Command(fp.Argv[0], fp.Argv[1:]...)
	cmd.Env = fp.Env
	cmd.Dir = fp.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if fp.LogPath != "" {
		if f, err := os.OpenFile(fp.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			defer f.Close()
			cmd.Stdout, cmd.Stderr = f, f
		}
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	watchedExits.store(pid, exited)
	if fp.PIDFile != "" {
		_ = os.MkdirAll(filepath.Dir(fp.PIDFile), 0o755)
		_ = os.WriteFile(fp.PIDFile, []byte(strconv.Itoa(pid)+"\n"), 0o644)
	}
	return pid, nil
}

func waitFrontendReady(ctx context.Context, fp *frontendPlan, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	exited := watchedExits.load(pid)
	for {
		if exited != nil {
			select {
			case <-exited:
				return fmt.Errorf("frontend exited during startup")
			default:
			}
		}
		if err := frontendReadyFn(ctx, fp.Port); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("frontend not ready within %s", timeout)
		}
		time.Sleep(watchdogPoll)
	}
}
