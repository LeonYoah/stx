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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

// 控制面重启策略 / control plane restart strategies
const (
	RestartStrategyStandalone = "standalone"
	RestartStrategySystemd    = "systemd"
	RestartStrategySupervisor = "supervisor"
	RestartStrategyPM2        = "pm2"
	RestartStrategyHook       = "hook"
	RestartStrategyDocker     = "docker"

	// RestartHookEnv 指定自定义重启命令（经 sh -c 执行），优先于自动检测。
	// RestartHookEnv names a custom restart command (run via sh -c) that wins over auto-detection.
	RestartHookEnv = "STX_SELF_UPGRADE_RESTART_CMD"
	// RestartTimeoutEnv 覆盖新进程就绪等待时间（Go duration）。/ RestartTimeoutEnv overrides the readiness timeout.
	RestartTimeoutEnv = "STX_SELF_UPGRADE_RESTART_TIMEOUT"
	// FrontendPIDFileEnv 显式指定前端进程 pid 文件。/ FrontendPIDFileEnv points at the frontend pid file explicitly.
	FrontendPIDFileEnv = "STX_FRONTEND_PID_FILE"

	// WatchdogCommand 是隐藏的 CLI 子命令名，由旧二进制以独立会话运行，负责拉起/校验/回滚。
	// WatchdogCommand is the hidden CLI sub-command the old binary runs detached to start/verify/roll back.
	WatchdogCommand = "__self-upgrade-restart"

	restartPlanFile   = "restart-plan.json"
	restartReadyFile  = "restart-ready.json"
	restartResultFile = "restart-result.json"
	activeTaskFile    = "active-task.json"
	restartLogFile    = "restart.log"

	restartOutcomeSucceeded  = "succeeded"
	restartOutcomeRolledBack = "rolled_back"
	restartOutcomeFailed     = "failed"
)

// restartStrategy 描述控制面如何被重启。/ restartStrategy describes how the control plane gets restarted.
type restartStrategy struct {
	Kind    string   `json:"kind"`
	Command []string `json:"command,omitempty"`
	Detail  string   `json:"detail,omitempty"`
}

// frontendPlan 描述随包 Next standalone 前端进程的重启方式。/ frontendPlan describes how the bundled frontend restarts.
type frontendPlan struct {
	PID     int      `json:"pid"`
	Argv    []string `json:"argv"`
	Env     []string `json:"env"`
	Dir     string   `json:"dir"`
	LogPath string   `json:"log_path,omitempty"`
	PIDFile string   `json:"pid_file,omitempty"`
	Port    int      `json:"port,omitempty"`
	// AppDir 是含 server.js 的前端目录；NewDir 是预先准备好的新版本目录（为空表示只重启不替换）。
	// AppDir is the directory holding server.js; NewDir is the prepared new version (empty: restart only, no swap).
	AppDir string `json:"app_dir,omitempty"`
	NewDir string `json:"new_dir,omitempty"`
	// ManagedBy 非空表示前端由进程管理器随控制面一起重启，watchdog 只做校验。
	// ManagedBy non-empty means the process manager restarts it with the control plane; the watchdog only verifies.
	ManagedBy string `json:"managed_by,omitempty"`
	// Swapped 表示进程管理器重启前已把 NewDir 换入 AppDir（旧目录在 AppDir.bak）。
	// Swapped means NewDir was moved into AppDir before the manager restart (the old dir is AppDir.bak).
	Swapped bool `json:"swapped,omitempty"`
}

// restartPlan 是旧进程交给 watchdog 的完整重启计划。/ restartPlan is the full plan the old process hands to the watchdog.
type restartPlan struct {
	TaskID         string          `json:"task_id"`
	TargetVersion  string          `json:"target_version"`
	FromVersion    string          `json:"from_version"`
	Strategy       restartStrategy `json:"strategy"`
	Executable     string          `json:"executable"`
	Backup         string          `json:"backup"`
	Argv           []string        `json:"argv"`
	Env            []string        `json:"env"`
	Dir            string          `json:"dir"`
	StdoutPath     string          `json:"stdout_path,omitempty"`
	StderrPath     string          `json:"stderr_path,omitempty"`
	OldPID         int             `json:"old_pid"`
	BackendPIDFile string          `json:"backend_pid_file,omitempty"`
	HealthURL      string          `json:"health_url"`
	ReadyPath      string          `json:"ready_path"`
	ResultPath     string          `json:"result_path"`
	TimeoutSeconds int             `json:"timeout_seconds"`
	Frontend       *frontendPlan   `json:"frontend,omitempty"`
}

type restartResult struct {
	Status       string          `json:"status"`
	Reason       string          `json:"reason,omitempty"`
	ControlPlane ProcessRestart  `json:"control_plane"`
	Frontend     *ProcessRestart `json:"frontend,omitempty"`
	At           time.Time       `json:"at"`
}

type readyMarker struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
	TaskID  string `json:"task_id,omitempty"`
}

// 可替换的系统交互（测试用）/ swappable system interactions (tests)
var (
	restartGetenv   = os.Getenv
	restartReadFile = os.ReadFile
	restartLookPath = exec.LookPath
	// restartExit 让旧进程退出，交由 watchdog 以新二进制拉起。/ restartExit ends the old process so the watchdog can start the new binary.
	restartExit = func() { os.Exit(0) }
	// spawnWatchdogFn 以独立会话启动 watchdog。/ spawnWatchdogFn starts the watchdog in its own session.
	spawnWatchdogFn = spawnWatchdog
	// runManagerCommandFn 发出进程管理器重启命令。/ runManagerCommandFn issues the process manager restart command.
	runManagerCommandFn = runDetached
	// frontendReadyFn 检查前端是否可用（默认 TCP 探测端口）。/ frontendReadyFn checks frontend readiness (TCP probe by default).
	frontendReadyFn = probeTCPPort
	// healthCheckFn 检查控制面健康接口。/ healthCheckFn checks the control plane health endpoint.
	healthCheckFn = probeHTTPHealth
	// watchdogPoll 是 watchdog 轮询间隔。/ watchdogPoll is the watchdog poll interval.
	watchdogPoll = 500 * time.Millisecond
	// oldProcessExitTimeout 是等待旧进程退出的上限。/ oldProcessExitTimeout bounds the wait for the old process to exit.
	oldProcessExitTimeout = 60 * time.Second
)

// detectRestartStrategy 依次检测：自定义钩子 → 容器 → systemd → supervisor → pm2 → 独立进程（脱离会话的 re-exec）。
// detectRestartStrategy checks, in order: custom hook → container → systemd → supervisor → pm2 → standalone
// (detached re-exec).
func detectRestartStrategy() restartStrategy {
	if hook := strings.TrimSpace(restartGetenv(RestartHookEnv)); hook != "" {
		return restartStrategy{Kind: RestartStrategyHook, Command: []string{"/bin/sh", "-c", hook}, Detail: hook}
	}
	if IsRunningInDocker() {
		return restartStrategy{Kind: RestartStrategyDocker, Detail: "container image upgrade"}
	}
	if restartGetenv("INVOCATION_ID") != "" {
		if unit, user := systemdUnitFromCgroup(); unit != "" {
			cmd := []string{"systemctl"}
			if user {
				cmd = append(cmd, "--user")
			}
			cmd = append(cmd, "--no-block", "restart", unit)
			return restartStrategy{Kind: RestartStrategySystemd, Command: cmd, Detail: unit}
		}
	}
	if restartGetenv("SUPERVISOR_ENABLED") != "" && restartGetenv("SUPERVISOR_PROCESS_NAME") != "" {
		name := restartGetenv("SUPERVISOR_PROCESS_NAME")
		if group := restartGetenv("SUPERVISOR_GROUP_NAME"); group != "" && group != name {
			name = group + ":" + name
		}
		cmd := []string{"supervisorctl"}
		if url := restartGetenv("SUPERVISOR_SERVER_URL"); url != "" && !strings.HasPrefix(url, "unix://") {
			cmd = append(cmd, "-s", url)
		}
		cmd = append(cmd, "restart", name)
		return restartStrategy{Kind: RestartStrategySupervisor, Command: cmd, Detail: name}
	}
	if id := restartGetenv("pm_id"); id != "" {
		name := restartGetenv("name")
		if name == "" {
			name = id
		}
		return restartStrategy{Kind: RestartStrategyPM2, Command: []string{"pm2", "restart", name, "--update-env"}, Detail: name}
	}
	return restartStrategy{Kind: RestartStrategyStandalone, Detail: "detached re-exec with the same arguments and environment"}
}

// systemdUnitFromCgroup 从 /proc/self/cgroup 解析所属 .service 单元。/ systemdUnitFromCgroup parses the owning .service unit.
func systemdUnitFromCgroup() (unit string, user bool) {
	data, err := restartReadFile("/proc/self/cgroup")
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 {
			continue
		}
		segs := strings.Split(parts[2], "/")
		for i := len(segs) - 1; i >= 0; i-- {
			if strings.HasSuffix(segs[i], ".service") && !strings.HasPrefix(segs[i], "user@") {
				return segs[i], strings.Contains(parts[2], "/user@")
			}
		}
	}
	return "", false
}

// restartTimeout 是等待新进程就绪的时间。/ restartTimeout is how long to wait for the new process to become ready.
func restartTimeout() time.Duration {
	if v := strings.TrimSpace(restartGetenv(RestartTimeoutEnv)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 90 * time.Second
}

// healthURLFromAddr 把 app.addr 转成本机健康检查地址。/ healthURLFromAddr turns app.addr into a local health URL.
func healthURLFromAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/api/v1/health"
}

func fdTarget(fd int) string {
	target, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil || !filepath.IsAbs(target) || strings.HasPrefix(target, "/dev/") {
		return ""
	}
	if fi, err := os.Stat(target); err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	return target
}

// restartExecutable 返回控制面可执行文件路径（测试可替换）。/ restartExecutable returns the control plane executable (tests swap it).
var restartExecutable = func() string { return startupExecutable }

// startupExecutable 在进程启动时解析：替换二进制时 exe 会被改名为 .bak，/proc/self/exe 随之指向 .bak，
// 因此不能在升级过程中再解析。
// startupExecutable is resolved at process start: replacing the binary renames exe to .bak and /proc/self/exe
// follows it, so it must not be resolved again mid-upgrade.
var startupExecutable = currentExecutable()

func currentExecutable() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "./stx"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

// detectFrontend 查找随包的 Next standalone 前端进程（start.sh 的 run/frontend.pid 布局，或 STX_FRONTEND_PID_FILE）。
// detectFrontend finds the bundled Next standalone frontend process (start.sh run/frontend.pid layout, or
// STX_FRONTEND_PID_FILE).
func detectFrontend(exe, cwd string) *frontendPlan {
	candidates := []string{}
	if v := strings.TrimSpace(restartGetenv(FrontendPIDFileEnv)); v != "" {
		candidates = append(candidates, v)
	}
	candidates = append(candidates,
		filepath.Join(filepath.Dir(exe), "run", "frontend.pid"),
		filepath.Join(cwd, "run", "frontend.pid"),
	)
	for _, pidFile := range candidates {
		raw, err := restartReadFile(pidFile)
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil || pid <= 0 || !processAlive(pid) {
			continue
		}
		argv, env, dir, logPath, err := snapshotProcess(pid)
		if err != nil || !isFrontendCommandLine(argv) {
			continue
		}
		fp := &frontendPlan{PID: pid, Argv: argv, Env: env, Dir: dir, LogPath: logPath, PIDFile: pidFile}
		for _, kv := range env {
			if strings.HasPrefix(kv, "PORT=") {
				fp.Port, _ = strconv.Atoi(strings.TrimPrefix(kv, "PORT="))
			}
		}
		for _, a := range argv[1:] {
			if filepath.Base(a) == "server.js" {
				p := a
				if !filepath.IsAbs(p) {
					p = filepath.Join(dir, p)
				}
				fp.AppDir = filepath.Dir(p)
			}
		}
		return fp
	}
	return nil
}

// isFrontendCommandLine 只接受 node server.js 形态的前端进程。/ isFrontendCommandLine only accepts `node server.js` frontends.
func isFrontendCommandLine(argv []string) bool {
	if len(argv) < 2 || !strings.Contains(filepath.Base(argv[0]), "node") {
		return false
	}
	for _, a := range argv[1:] {
		if filepath.Base(a) == "server.js" {
			return true
		}
	}
	return false
}

func snapshotProcess(pid int) (argv, env []string, dir, logPath string, err error) {
	base := filepath.Join("/proc", strconv.Itoa(pid))
	raw, err := os.ReadFile(filepath.Join(base, "cmdline"))
	if err != nil {
		return nil, nil, "", "", err
	}
	argv = splitNul(raw)
	if len(argv) == 0 {
		return nil, nil, "", "", fmt.Errorf("empty cmdline")
	}
	rawEnv, err := os.ReadFile(filepath.Join(base, "environ"))
	if err != nil {
		return nil, nil, "", "", err
	}
	env = splitNul(rawEnv)
	dir, _ = os.Readlink(filepath.Join(base, "cwd"))
	if out, err := os.Readlink(filepath.Join(base, "fd", "1")); err == nil && filepath.IsAbs(out) && !strings.HasPrefix(out, "/dev/") {
		if fi, err := os.Stat(out); err == nil && fi.Mode().IsRegular() {
			logPath = out
		}
	}
	return argv, env, dir, logPath, nil
}

func splitNul(raw []byte) []string {
	var out []string
	for _, p := range strings.Split(string(raw), "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		// 字段 3 是状态；僵尸进程视为已退出。/ field 3 is the state; zombies count as exited.
		if i := strings.LastIndexByte(string(raw), ')'); i > 0 && i+2 < len(raw) {
			if st := raw[i+2]; st == 'Z' || st == 'X' {
				return false
			}
		}
	}
	return true
}

// RestartPlanPreview 返回确认框展示的重启计划（只读）。/ RestartPlanPreview returns the read-only restart plan for the confirm dialog.
func (s *Service) RestartPlanPreview() *RestartPlanInfo {
	st := detectRestartStrategy()
	info := &RestartPlanInfo{
		ControlPlaneStrategy: st.Kind,
		ControlPlaneDetail:   st.Detail,
		RestartsAgents:       true,
		RestartsProxies:      true,
		SeaTunnelAffected:    false,
	}
	cwd, _ := os.Getwd()
	if fp := detectFrontend(restartExecutable(), cwd); fp != nil {
		info.FrontendDetected = true
		info.FrontendStrategy = frontendStrategyFor(st)
	} else if st.Kind == RestartStrategyHook {
		info.FrontendStrategy = RestartStrategyHook
	}
	return info
}

func frontendStrategyFor(st restartStrategy) string {
	if st.Kind == RestartStrategySystemd {
		return RestartStrategySystemd
	}
	return RestartStrategyStandalone
}

// buildRestartPlan 收集重启所需信息：可执行文件与备份、原 argv/env/cwd/输出、策略、健康地址、前端。
// buildRestartPlan gathers what the restart needs: executable and backup, original argv/env/cwd/output, strategy,
// health URL and frontend.
func (s *Service) buildRestartPlan(taskID, target, from string) (*restartPlan, error) {
	st := detectRestartStrategy()
	if st.Kind == RestartStrategyDocker {
		return nil, fmt.Errorf("container deployments restart via a new image / 容器部署通过新镜像重启")
	}
	exe := restartExecutable()
	backup := exe + ".bak"
	if fi, err := os.Stat(backup); err != nil || fi.Size() == 0 {
		return nil, fmt.Errorf("no backup of the previous control plane binary at %s; refusing to restart without a rollback path / 缺少控制面旧二进制备份，拒绝在无回滚路径时重启", backup)
	}
	cwd, _ := os.Getwd()
	staging := absPath(s.stagingDir)
	plan := &restartPlan{
		TaskID:         taskID,
		TargetVersion:  target,
		FromVersion:    from,
		Strategy:       st,
		Executable:     absPath(exe),
		Backup:         absPath(backup),
		Argv:           append([]string(nil), os.Args...),
		Env:            os.Environ(),
		Dir:            absPath(cwd),
		StdoutPath:     fdTarget(1),
		StderrPath:     fdTarget(2),
		OldPID:         os.Getpid(),
		HealthURL:      healthURLFromAddr(config.Config.App.Addr),
		ReadyPath:      filepath.Join(staging, restartReadyFile),
		ResultPath:     filepath.Join(staging, restartResultFile),
		TimeoutSeconds: int(restartTimeout().Seconds()),
	}
	if plan.TimeoutSeconds <= 0 {
		plan.TimeoutSeconds = 1
	}
	// 原输出不是普通文件（终端/管道）时，新进程输出写到自升级目录，避免丢日志。
	// When the original output is not a regular file (tty/pipe), write the new process output under the staging dir.
	if plan.StdoutPath == "" {
		plan.StdoutPath = filepath.Join(staging, "control-plane.out")
	} else {
		plan.StdoutPath = absPath(plan.StdoutPath)
	}
	if plan.StderrPath == "" {
		plan.StderrPath = plan.StdoutPath
	} else {
		plan.StderrPath = absPath(plan.StderrPath)
	}
	if pidFile := filepath.Join(filepath.Dir(exe), "run", "backend.pid"); fileHasPID(pidFile, plan.OldPID) {
		plan.BackendPIDFile = pidFile
	}
	if fp := detectFrontend(exe, cwd); fp != nil {
		if st.Kind == RestartStrategySystemd {
			fp.ManagedBy = RestartStrategySystemd
		}
		if st.Kind != RestartStrategyHook {
			if dir, err := s.versionStagingDir(target); err == nil && fp.AppDir != "" {
				if src := filepath.Join(dir, "frontend"); fileExists(filepath.Join(src, "server.js")) {
					newDir := fp.AppDir + ".new"
					_ = os.RemoveAll(newDir)
					if err := copyDir(src, newDir); err != nil {
						return nil, fmt.Errorf("prepare new frontend: %w / 准备新前端失败", err)
					}
					fp.NewDir = newDir
				}
			}
			plan.Frontend = fp
		}
	}
	return plan, nil
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func fileHasPID(path string, pid int) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	return err == nil && v == pid
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// absPath 将路径转为绝对路径；空串保持为空。/ absPath makes a path absolute; empty stays empty.
func absPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}

// absolutizeRestartPlan 确保 plan 内路径均为绝对路径，避免 systemd-run 默认 cwd=/ 时读错文件（DEF-UPG-1）。
// absolutizeRestartPlan makes plan paths absolute so systemd-run's default cwd=/ cannot miss files (DEF-UPG-1).
func absolutizeRestartPlan(plan *restartPlan) {
	if plan == nil {
		return
	}
	plan.Dir = absPath(plan.Dir)
	plan.Executable = absPath(plan.Executable)
	plan.Backup = absPath(plan.Backup)
	plan.ReadyPath = absPath(plan.ReadyPath)
	plan.ResultPath = absPath(plan.ResultPath)
	plan.StdoutPath = absPath(plan.StdoutPath)
	plan.StderrPath = absPath(plan.StderrPath)
	plan.BackendPIDFile = absPath(plan.BackendPIDFile)
	if fp := plan.Frontend; fp != nil {
		fp.Dir = absPath(fp.Dir)
		fp.LogPath = absPath(fp.LogPath)
		fp.PIDFile = absPath(fp.PIDFile)
		fp.AppDir = absPath(fp.AppDir)
		fp.NewDir = absPath(fp.NewDir)
	}
}

// buildWatchdogArgv 构造 watchdog 启动参数；systemd 下经 systemd-run 拉起，并固定工作目录与绝对 --plan。
// buildWatchdogArgv builds watchdog argv; under systemd it uses systemd-run with a fixed working directory and absolute --plan.
func buildWatchdogArgv(planPath string, plan *restartPlan) []string {
	absolutizeRestartPlan(plan)
	planPath = absPath(planPath)
	args := []string{plan.Backup, WatchdogCommand, "--plan", planPath}
	if plan.Strategy.Kind != RestartStrategySystemd {
		return args
	}
	run, err := restartLookPath("systemd-run")
	if err != nil {
		return args
	}
	unitArgs := []string{run}
	if strings.Contains(strings.Join(plan.Strategy.Command, " "), "--user") {
		unitArgs = append(unitArgs, "--user")
	}
	// DEF-UPG-1：systemd-run 默认 WorkingDirectory=/，相对 --plan / result 会落到错误位置。
	// DEF-UPG-1: systemd-run defaults WorkingDirectory=/; relative --plan/result resolve in the wrong place.
	if wd := plan.Dir; wd != "" {
		unitArgs = append(unitArgs, "--working-directory="+wd)
	}
	unitArgs = append(unitArgs, "--collect", "--unit", "stx-self-upgrade-"+shortID(plan.TaskID))
	return append(unitArgs, args...)
}

// spawnWatchdog 以旧二进制（即 .bak，当前运行的代码）启动 watchdog，并让它脱离当前进程树与会话。
// systemd 下优先用 systemd-run 把它放到独立单元，避免随服务 cgroup 一起被停止。
// spawnWatchdog starts the watchdog from the old binary (the .bak, i.e. the code running now) detached from this
// process tree and session. Under systemd it prefers systemd-run so the watchdog lives outside the service cgroup.
func spawnWatchdog(planPath string, plan *restartPlan) error {
	absolutizeRestartPlan(plan)
	planPath = absPath(planPath)
	logPath := filepath.Join(filepath.Dir(planPath), restartLogFile)
	args := buildWatchdogArgv(planPath, plan)
	return runDetachedWithLog(args, plan.Env, plan.Dir, logPath)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// runDetached 经 sh 后台化运行命令（被 init 收养，不受当前进程退出影响）。
// runDetached runs a command backgrounded via sh (adopted by init, unaffected by this process exiting).
func runDetached(argv, env []string, dir string) error {
	return runDetachedWithLog(argv, env, dir, os.DevNull)
}

func runDetachedWithLog(argv, env []string, dir, logPath string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty command")
	}
	args := append([]string{"-c", `log="$1"; shift; "$@" >>"$log" 2>&1 </dev/null &`, "sh", logPath}, argv...)
	cmd := exec.Command("/bin/sh", args...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Run()
}

func probeHTTPHealth(ctx context.Context, url string) error {
	if url == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health status %d", resp.StatusCode)
	}
	return nil
}

func probeTCPPort(ctx context.Context, port int) error {
	if port <= 0 {
		return nil
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return conn.Close()
}

// exitWatchers 记录 watchdog 直接拉起的子进程的退出通知。/ exitWatchers tracks exit notifications of children the watchdog started.
type exitWatchers struct {
	mu sync.Mutex
	m  map[int]chan struct{}
}

var watchedExits = &exitWatchers{m: map[int]chan struct{}{}}

func (w *exitWatchers) store(pid int, ch chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.m[pid] = ch
}

func (w *exitWatchers) load(pid int) chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.m[pid]
}
