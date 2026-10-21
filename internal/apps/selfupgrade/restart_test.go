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
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	stxversion "github.com/LeonYoah/stx/internal/version"
	"github.com/gin-gonic/gin"
)

func TestResolveMode(t *testing.T) {
	cases := []struct {
		req     ExecuteRequest
		want    string
		wantErr bool
	}{
		{ExecuteRequest{}, ExecModeDryRun, false},
		{ExecuteRequest{Apply: true}, ExecModeApply, false},
		{ExecuteRequest{Mode: "apply"}, ExecModeApply, false},
		{ExecuteRequest{Mode: "dry_run"}, ExecModeDryRun, false},
		{ExecuteRequest{Mode: "apply", Apply: true}, ExecModeApply, false},
		{ExecuteRequest{Mode: "dry_run", Apply: true}, "", true},
		{ExecuteRequest{Mode: "yolo"}, "", true},
	}
	for _, c := range cases {
		got, err := c.req.ResolveMode()
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("ResolveMode(%+v) = %q, %v; want %q, err=%v", c.req, got, err, c.want, c.wantErr)
		}
	}
}

func stagedTestService(t *testing.T) *Service {
	t.Helper()
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportOffline("bundle.tar.gz", strings.NewReader("x"), ""); err != nil {
		t.Fatal(err)
	}
	return s
}

func waitTaskDone(t *testing.T, s *Service, id string) *TaskState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, err := s.GetTask(id)
		if err == nil && got.Status != StatusUpgrading {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %s did not finish: %+v", id, got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func stepByCode(steps []StepState, code string) StepState {
	for _, st := range steps {
		if st.Code == code {
			return st
		}
	}
	return StepState{}
}

func TestExecuteRejectsInvalidModeAsBadRequest(t *testing.T) {
	s := stagedTestService(t)
	for _, req := range []ExecuteRequest{{Mode: "yolo"}, {Mode: ExecModeDryRun, Apply: true}} {
		_, err := s.Execute(context.Background(), req)
		if !errors.Is(err, ErrInvalidRequest) || statusFor(err) != http.StatusBadRequest {
			t.Fatalf("Execute(%+v) err = %v (status %d)", req, err, statusFor(err))
		}
	}
	// 被拒绝的请求不能留下任务 / rejected requests must not leave a task behind
	if st := s.Status(); (st.ActiveTask != nil && st.ActiveTask.Kind == "execute") || st.OverallStatus != StatusStaged {
		t.Fatalf("status after rejected execute = %s %+v", st.OverallStatus, st.ActiveTask)
	}
}

func TestHandlerExecuteMapsModeField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := stagedTestService(t)
	h := NewHandler(s)
	post := func(body string) (*httptest.ResponseRecorder, Response) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/self-upgrade/execute", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		h.Execute(c)
		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}
	if w, resp := post(`{"mode":"bogus"}`); w.Code != http.StatusBadRequest || resp.ErrorCode != CodeInvalidRequest {
		t.Fatalf("bogus mode: %d %s", w.Code, w.Body.String())
	}
	w, _ := post(`{"mode":"dry_run"}`)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"execution_mode":"dry_run"`) || !strings.Contains(w.Body.String(), `"dry_run":true`) {
		t.Fatalf("dry_run: %d %s", w.Code, w.Body.String())
	}
	var task struct {
		Data TaskState `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	waitTaskDone(t, s, task.Data.ID)
	w, _ = post(`{"apply":true}`)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"execution_mode":"apply"`) || strings.Contains(w.Body.String(), `"dry_run":true`) {
		t.Fatalf("legacy apply: %d %s", w.Code, w.Body.String())
	}
}

// 演练不改变任何状态：仍是已预置、仍可升级，历史记录标记为 dry_run，重启只是计划。
// A dry run changes nothing: still staged, update still available, history marked dry_run, restarts only planned.
func TestDryRunChangesNothingAndCanBeFollowedByApply(t *testing.T) {
	s := stagedTestService(t)
	before := s.Status()
	task, err := s.Execute(context.Background(), ExecuteRequest{Mode: ExecModeDryRun})
	if err != nil {
		t.Fatal(err)
	}
	if !task.DryRun || task.ExecutionMode != ExecModeDryRun {
		t.Fatalf("task mode = %s dry=%v", task.ExecutionMode, task.DryRun)
	}
	got := waitTaskDone(t, s, task.ID)
	if got.Status != StatusSucceeded {
		t.Fatalf("dry-run status = %s %s", got.Status, got.FailureReason)
	}
	if msg := stepByCode(got.Steps, StepRestart).Message; !strings.Contains(msg, "nothing restarted") || !strings.Contains(msg, "SeaTunnel is not touched") {
		t.Fatalf("restart step message = %q", msg)
	}
	if len(got.Restarts) == 0 || got.Restarts[0].Status != RestartWouldDoThis {
		t.Fatalf("restarts = %+v", got.Restarts)
	}
	after := s.Status()
	if after.OverallStatus != StatusStaged || after.UpdateAvailable != before.UpdateAvailable || after.LatestVersion != before.LatestVersion {
		t.Fatalf("dry-run changed state: before=%s/%v/%s after=%s/%v/%s", before.OverallStatus, before.UpdateAvailable, before.LatestVersion,
			after.OverallStatus, after.UpdateAvailable, after.LatestVersion)
	}
	if after.ActiveTask == nil || !after.ActiveTask.DryRun {
		t.Fatal("dry-run task should stay visible")
	}
	if !strings.Contains(after.Stage.Message, "nothing was applied") {
		t.Fatalf("stage message = %q", after.Stage.Message)
	}
	hist := s.ListHistory()
	if len(hist) != 1 || hist[0].ExecutionMode != ExecModeDryRun || !hist[0].DryRun {
		t.Fatalf("history = %+v", hist)
	}

	applyTask, err := s.Execute(context.Background(), ExecuteRequest{Mode: ExecModeApply})
	if err != nil {
		t.Fatalf("apply after dry-run must be allowed: %v", err)
	}
	waitTaskDone(t, s, applyTask.ID)
	hist = s.ListHistory()
	if len(hist) != 2 || hist[0].ExecutionMode != ExecModeApply || hist[0].DryRun {
		t.Fatalf("history after apply = %+v", hist[0])
	}
	if s.Status().RestartPlan == nil || s.Status().RestartPlan.SeaTunnelAffected {
		t.Fatal("status must expose a restart plan that leaves SeaTunnel alone")
	}
}

func TestDetectRestartStrategy(t *testing.T) {
	oldGetenv, oldRead := restartGetenv, restartReadFile
	t.Cleanup(func() { restartGetenv, restartReadFile = oldGetenv, oldRead })
	cgroup := "0::/system.slice/stx.service\n"
	restartReadFile = func(name string) ([]byte, error) {
		if name == "/proc/self/cgroup" {
			return []byte(cgroup), nil
		}
		return nil, os.ErrNotExist
	}
	cases := []struct {
		env  map[string]string
		cg   string
		kind string
		cmd  string
	}{
		{map[string]string{RestartHookEnv: "systemctl restart stx-all"}, cgroup, RestartStrategyHook, "/bin/sh -c systemctl restart stx-all"},
		{map[string]string{"INVOCATION_ID": "abc"}, cgroup, RestartStrategySystemd, "systemctl --no-block restart stx.service"},
		{map[string]string{"INVOCATION_ID": "abc"}, "0::/user.slice/user-1000.slice/user@1000.service/app.slice/stx.service\n", RestartStrategySystemd, "systemctl --user --no-block restart stx.service"},
		{map[string]string{"SUPERVISOR_ENABLED": "1", "SUPERVISOR_PROCESS_NAME": "stx"}, "", RestartStrategySupervisor, "supervisorctl restart stx"},
		{map[string]string{"pm_id": "3", "name": "stx-api"}, "", RestartStrategyPM2, "pm2 restart stx-api --update-env"},
		{map[string]string{"INVOCATION_ID": "abc"}, "0::/user.slice/session-1.scope\n", RestartStrategyStandalone, ""},
		{map[string]string{}, "", RestartStrategyStandalone, ""},
	}
	for _, c := range cases {
		env := c.env
		cgroup = c.cg
		restartGetenv = func(k string) string { return env[k] }
		st := detectRestartStrategy()
		if IsRunningInDocker() {
			t.Skip("running inside a container")
		}
		if st.Kind != c.kind || strings.Join(st.Command, " ") != c.cmd {
			t.Errorf("env %v: got %s %v, want %s %q", c.env, st.Kind, st.Command, c.kind, c.cmd)
		}
	}
}

// ---- watchdog ----

type procs struct {
	mu   sync.Mutex
	pids []int
}

func (p *procs) add(pid int) {
	p.mu.Lock()
	p.pids = append(p.pids, pid)
	p.mu.Unlock()
}

func (p *procs) killAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pid := range p.pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// fakeControlPlane 写就绪标记（与真实控制面在 NewService 中的行为一致）后常驻。
// fakeControlPlane writes the ready marker (like the real control plane in NewService) and stays up.
func fakeControlPlane(version string) string {
	return fmt.Sprintf(`printf '{"pid":%%d,"version":"%s"}' $$ > "$STX_TEST_READY"
exec sleep 300`, version)
}

// startProcess 启动一个后台进程并在测试结束时清理。/ startProcess starts a background process cleaned up after the test.
func startProcess(t *testing.T, p *procs, name string, args ...string) int {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	p.add(cmd.Process.Pid)
	return cmd.Process.Pid
}

// fakeSeaTunnel 启动一个命令行形如 SeaTunnel 引擎的进程，用来断言重启流程从不触碰它。
// fakeSeaTunnel starts a process whose command line looks like the SeaTunnel engine, to assert restarts never touch it.
func fakeSeaTunnel(t *testing.T, p *procs) int {
	t.Helper()
	return startProcess(t, p, "/bin/sh", "-c", "while :; do sleep 1; done", "org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer")
}

func installWatchdogHooks(t *testing.T) {
	t.Helper()
	oldHealth, oldPoll, oldExit, oldFront, oldMgr := healthCheckFn, watchdogPoll, oldProcessExitTimeout, frontendReadyFn, runManagerCommandFn
	t.Cleanup(func() {
		healthCheckFn, watchdogPoll, oldProcessExitTimeout, frontendReadyFn, runManagerCommandFn = oldHealth, oldPoll, oldExit, oldFront, oldMgr
	})
	healthCheckFn = func(context.Context, string) error { return nil }
	watchdogPoll = 20 * time.Millisecond
	oldProcessExitTimeout = 3 * time.Second
	frontendReadyFn = func(context.Context, int) error { return nil }
}

type watchdogFixture struct {
	dir   string
	exe   string
	ready string
	plan  *restartPlan
	procs *procs
}

func newWatchdogFixture(t *testing.T, newBody string) *watchdogFixture {
	t.Helper()
	installWatchdogHooks(t)
	dir := t.TempDir()
	f := &watchdogFixture{dir: dir, exe: filepath.Join(dir, "stx"), ready: filepath.Join(dir, "ready.json"), procs: &procs{}}
	t.Cleanup(f.procs.killAll)
	writeScript(t, f.exe, newBody)
	writeScript(t, f.exe+".bak", fakeControlPlane("1.0.0"))
	// 旧控制面：很快退出（模拟交棒后 os.Exit）。/ old control plane: exits shortly (the handover os.Exit).
	oldPID := startProcess(t, f.procs, "sleep", "0.2")
	f.plan = &restartPlan{
		TaskID: "task-1", TargetVersion: "1.0.1", FromVersion: "1.0.0",
		Strategy:   restartStrategy{Kind: RestartStrategyStandalone},
		Executable: f.exe, Backup: f.exe + ".bak",
		Argv: []string{f.exe, "server"},
		Env:  append(os.Environ(), "STX_TEST_READY="+f.ready),
		Dir:  dir, StdoutPath: filepath.Join(dir, "cp.log"), StderrPath: filepath.Join(dir, "cp.log"),
		OldPID: oldPID, BackendPIDFile: filepath.Join(dir, "backend.pid"),
		ReadyPath: f.ready, ResultPath: filepath.Join(dir, "result.json"), TimeoutSeconds: 3,
	}
	return f
}

func TestWatchdogStandaloneRestartsOntoNewBinary(t *testing.T) {
	f := newWatchdogFixture(t, fakeControlPlane("1.0.1"))
	st := fakeSeaTunnel(t, f.procs)
	res := watchdogRun(context.Background(), f.plan)
	if res.Status != restartOutcomeSucceeded || res.ControlPlane.Status != RestartSucceeded {
		t.Fatalf("result = %+v", res)
	}
	f.procs.add(res.ControlPlane.NewPID)
	if res.ControlPlane.NewPID == 0 || res.ControlPlane.NewPID == f.plan.OldPID || !processAlive(res.ControlPlane.NewPID) {
		t.Fatalf("new pid = %d", res.ControlPlane.NewPID)
	}
	if raw, _ := os.ReadFile(f.plan.BackendPIDFile); strings.TrimSpace(string(raw)) != fmt.Sprint(res.ControlPlane.NewPID) {
		t.Fatalf("backend pidfile = %q", raw)
	}
	if !processAlive(st) {
		t.Fatal("SeaTunnel process must not be touched by the restart")
	}
	if _, err := os.Stat(f.exe + ".failed"); err == nil {
		t.Fatal("successful restart must not restore the backup")
	}
}

func TestWatchdogRollsBackWhenNewBinaryFails(t *testing.T) {
	f := newWatchdogFixture(t, "echo boom >&2; exit 3")
	st := fakeSeaTunnel(t, f.procs)
	res := watchdogRun(context.Background(), f.plan)
	if res.Status != restartOutcomeRolledBack || res.ControlPlane.Status != RestartRolledBack {
		t.Fatalf("result = %+v", res)
	}
	f.procs.add(res.ControlPlane.NewPID)
	if !processAlive(res.ControlPlane.NewPID) {
		t.Fatal("old version should be running again")
	}
	exe, _ := os.ReadFile(f.exe)
	bak, _ := os.ReadFile(f.exe + ".bak")
	if string(exe) != string(bak) {
		t.Fatal("executable should be restored from the backup")
	}
	if failed, _ := os.ReadFile(f.exe + ".failed"); !strings.Contains(string(failed), "boom") {
		t.Fatal("the failed binary should be kept as .failed")
	}
	var m readyMarker
	if err := readJSONFile(f.ready, &m); err != nil || m.Version != "1.0.0" {
		t.Fatalf("ready marker = %+v %v", m, err)
	}
	if !processAlive(st) {
		t.Fatal("SeaTunnel process must not be touched by the rollback")
	}
}

func TestWatchdogRollsBackWhenHealthNeverPasses(t *testing.T) {
	f := newWatchdogFixture(t, fakeControlPlane("1.0.1"))
	healthCheckFn = func(context.Context, string) error {
		var m readyMarker
		if err := readJSONFile(f.ready, &m); err == nil && m.Version == "1.0.1" {
			return errors.New("503")
		}
		return nil
	}
	res := watchdogRun(context.Background(), f.plan)
	f.procs.add(res.ControlPlane.NewPID)
	if res.Status != restartOutcomeRolledBack {
		t.Fatalf("result = %+v", res)
	}
}

func TestWatchdogManagerThatNeverRestartsReportsFailure(t *testing.T) {
	f := newWatchdogFixture(t, fakeControlPlane("1.0.1"))
	oldProcessExitTimeout = 200 * time.Millisecond
	f.plan.Strategy = restartStrategy{Kind: RestartStrategySystemd, Command: []string{"systemctl", "--no-block", "restart", "stx.service"}}
	f.plan.OldPID = startProcess(t, f.procs, "sleep", "30")
	called := false
	runManagerCommandFn = func([]string, []string, string) error { called = true; return nil }
	res := watchdogRun(context.Background(), f.plan)
	if res.Status != restartOutcomeFailed || !processAlive(f.plan.OldPID) || called {
		t.Fatalf("result = %+v, old alive=%v, manager called=%v", res, processAlive(f.plan.OldPID), called)
	}
}

// 前端：换入新目录并以相同命令行重启；新前端起不来时恢复旧目录。只会对 node server.js 发信号。
// Frontend: swap the new dir in and restart with the same command line; restore the old dir when the new one fails.
// Only `node server.js` processes are ever signalled.
func TestWatchdogRestartsBundledFrontend(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprintf("new_frontend_bad=%v", bad), func(t *testing.T) {
			f := newWatchdogFixture(t, fakeControlPlane("1.0.1"))
			st := fakeSeaTunnel(t, f.procs)
			node := filepath.Join(f.dir, "node")
			sh, _ := exec.LookPath("sh")
			raw, _ := os.ReadFile(sh)
			if err := os.WriteFile(node, raw, 0o755); err != nil {
				t.Fatal(err)
			}
			app := filepath.Join(f.dir, "frontend")
			_ = os.MkdirAll(app, 0o755)
			_ = os.WriteFile(filepath.Join(app, "server.js"), []byte("old"), 0o644)
			newDir := app + ".new"
			_ = os.MkdirAll(newDir, 0o755)
			content := "new"
			if bad {
				content = "bad"
			}
			_ = os.WriteFile(filepath.Join(newDir, "server.js"), []byte(content), 0o644)
			argv := []string{node, "-c", "while :; do sleep 1; done", filepath.Join(app, "server.js")}
			oldFront := startProcess(t, f.procs, argv[0], argv[1:]...)
			frontendReadyFn = func(context.Context, int) error {
				if b, _ := os.ReadFile(filepath.Join(app, "server.js")); string(b) == "bad" {
					return errors.New("connection refused")
				}
				return nil
			}
			f.plan.Frontend = &frontendPlan{PID: oldFront, Argv: argv, Env: os.Environ(), Dir: f.dir, PIDFile: filepath.Join(f.dir, "frontend.pid"),
				Port: 1, AppDir: app, NewDir: newDir}
			res := watchdogRun(context.Background(), f.plan)
			f.procs.add(res.ControlPlane.NewPID)
			if res.Status != restartOutcomeSucceeded || res.Frontend == nil {
				t.Fatalf("result = %+v", res)
			}
			f.procs.add(res.Frontend.NewPID)
			want, wantContent := RestartSucceeded, "new"
			if bad {
				want, wantContent = RestartRolledBack, "old"
			}
			if res.Frontend.Status != want || processAlive(oldFront) || !processAlive(res.Frontend.NewPID) {
				t.Fatalf("frontend = %+v old alive=%v", res.Frontend, processAlive(oldFront))
			}
			if b, _ := os.ReadFile(filepath.Join(app, "server.js")); string(b) != wantContent {
				t.Fatalf("frontend dir content = %q", b)
			}
			if !processAlive(st) {
				t.Fatal("SeaTunnel process must not be touched")
			}
		})
	}
}

func TestStopFrontendRefusesNonFrontendProcesses(t *testing.T) {
	p := &procs{}
	t.Cleanup(p.killAll)
	st := fakeSeaTunnel(t, p)
	stopFrontend(st)
	time.Sleep(50 * time.Millisecond)
	if !processAlive(st) {
		t.Fatal("stopFrontend must only signal node server.js processes")
	}
}

// ---- 端到端：真实应用交棒与重启后收尾 / end to end: apply handover and post-restart finish ----

type applyHarness struct {
	s       *Service
	fleet   *fakeFleet
	dataDir string
	exe     string
	exited  chan struct{}
	plan    *restartPlan
}

func newApplyHarness(t *testing.T) *applyHarness {
	t.Helper()
	if IsRunningInDocker() {
		t.Skip("real apply is refused inside containers")
	}
	t.Chdir(t.TempDir())
	old := time.Now().Add(-time.Hour)
	fleet := newFakeFleet(
		FleetNode{HostID: 1, HostName: "w1", AgentID: "a1", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: true, ConnectedAt: old, InstallDirs: []string{"/opt/seatunnel"}},
		FleetNode{HostID: 2, HostName: "w2", AgentID: "a2", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 3, HostName: "w3", AgentID: "a3", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: false},
	)
	fleet.proxyOut[1] = `{"target_dir":"/opt/stx-agent/lib","updated":["stx-java-proxy-v3.jar"],"restarts":[{"state_dir":"/opt/seatunnel/logs/stx-java-proxy","status":"restarted","old_pid":100,"new_pid":101,"port":18081}]}`
	fleet.proxyOut[2] = `{"target_dir":"/opt/stx-agent/lib","updated":["stx-java-proxy-v3.jar"]}`
	s, stage := newRolloutService(t, fleet)
	s.mockMode = false
	dataDir := filepath.Dir(s.stagingDir)
	_ = os.WriteFile(filepath.Join(stage, "stx"), []byte("new-binary-1.0.1"), 0o755)
	s.mu.Lock()
	s.stage = StageState{Status: StatusStaged, TargetVersion: "1.0.1", Mode: ModeOffline}
	s.mu.Unlock()

	h := &applyHarness{s: s, fleet: fleet, dataDir: dataDir, exe: filepath.Join(t.TempDir(), "stx"), exited: make(chan struct{})}
	_ = os.WriteFile(h.exe, []byte("old-binary-1.0.0"), 0o755)

	oldExe, oldSpawn, oldExit, oldDelay, oldGetenv, oldPoll, oldGrace := restartExecutable, spawnWatchdogFn, restartExit, restartHandoverDelay, restartGetenv, watchdogPoll, restartResultGrace
	oldVersion := stxversion.Version
	t.Cleanup(func() {
		restartExecutable, spawnWatchdogFn, restartExit, restartHandoverDelay, restartGetenv, watchdogPoll, restartResultGrace = oldExe, oldSpawn, oldExit, oldDelay, oldGetenv, oldPoll, oldGrace
		stxversion.Version = oldVersion
	})
	stxversion.Version = "1.0.0"
	restartExecutable = func() string { return h.exe }
	restartGetenv = func(string) string { return "" }
	restartHandoverDelay = 10 * time.Millisecond
	watchdogPoll = 10 * time.Millisecond
	var once sync.Once
	restartExit = func() { once.Do(func() { close(h.exited) }) }
	spawnWatchdogFn = func(_ string, p *restartPlan) error { h.plan = p; return nil }
	return h
}

func (h *applyHarness) runApply(t *testing.T) *TaskState {
	t.Helper()
	task, err := h.s.Execute(context.Background(), ExecuteRequest{Mode: ExecModeApply})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.exited:
	case <-time.After(15 * time.Second):
		got, _ := h.s.GetTask(task.ID)
		t.Fatalf("control plane never handed over to the watchdog: %+v", got)
	}
	return task
}

// 允许下发给 Agent 的子命令：只涉及 Agent 自身与 stx-java-proxy，从不包含 SeaTunnel 进程/作业操作。
// Sub-commands allowed to reach Agents: only the Agent itself and stx-java-proxy, never SeaTunnel processes or jobs.
var seaTunnelSafeSubCommands = map[string]bool{
	subCommandAgentSelfUpdate:        true,
	subCommandAgentSelfRollback:      true,
	subCommandAgentUpdateProxyJars:   true,
	subCommandAgentRollbackProxyJars: true,
}

func assertSeaTunnelSafe(t *testing.T, f *fakeFleet) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.commands {
		if !seaTunnelSafeSubCommands[c["sub_command"]] {
			t.Errorf("self-upgrade sent a non-STX command to an agent: %+v", c)
		}
		for k, v := range c {
			if strings.Contains(strings.ToLower(k+"="+v), "seatunnel") && k != "install_dirs" {
				t.Errorf("self-upgrade command references SeaTunnel: %+v", c)
			}
		}
	}
}

func TestApplyReplacesBinaryRestartsProxiesAndHandsOverRestart(t *testing.T) {
	h := newApplyHarness(t)
	task := h.runApply(t)

	if b, _ := os.ReadFile(h.exe); string(b) != "new-binary-1.0.1" {
		t.Fatalf("executable = %q", b)
	}
	if b, _ := os.ReadFile(h.exe + ".bak"); string(b) != "old-binary-1.0.0" {
		t.Fatalf("backup = %q", b)
	}
	if h.plan == nil || h.plan.Strategy.Kind != RestartStrategyStandalone || h.plan.Executable != h.exe || h.plan.Backup != h.exe+".bak" ||
		h.plan.FromVersion != "1.0.0" || h.plan.TargetVersion != "1.0.1" || h.plan.OldPID != os.Getpid() {
		t.Fatalf("restart plan = %+v", h.plan)
	}
	got, _ := h.s.GetTask(task.ID)
	if got.Status != StatusUpgrading || got.CurrentStep != StepRestart || stepByCode(got.Steps, StepRestart).Status != StepRunning {
		t.Fatalf("task during handover = %s/%s", got.Status, got.CurrentStep)
	}
	if n := nodeByHost(got.AgentNodes, 1); n.ProxyStatus != ProxyUpdated || n.ProxyRestart != ProxyRestartRestarted || !strings.Contains(n.ProxyRestartMessage, "101") {
		t.Fatalf("node1 proxy = %+v", n)
	}
	if n := nodeByHost(got.AgentNodes, 2); n.ProxyRestart != ProxyRestartNotRunning {
		t.Fatalf("node2 proxy restart = %s", n.ProxyRestart)
	}
	var persisted TaskState
	if err := readJSONFile(filepath.Join(h.s.stagingDir, activeTaskFile), &persisted); err != nil || persisted.ID != task.ID || persisted.ExecutionMode != ExecModeApply {
		t.Fatalf("persisted task = %+v %v", persisted, err)
	}
	h.fleet.mu.Lock()
	sawProxy := false
	for _, c := range h.fleet.commands {
		if c["sub_command"] == subCommandAgentUpdateProxyJars && c["restart_running"] == "true" {
			sawProxy = true
			if c["agent_id"] == "a1" || (strings.Contains(c["jar_paths"], "v3") && c["install_dirs"] != "" && c["install_dirs"] != "/opt/seatunnel") {
				t.Errorf("proxy params = %+v", c)
			}
		}
	}
	h.fleet.mu.Unlock()
	if !sawProxy {
		t.Fatal("proxy rollout must ask agents to restart running proxies")
	}
	assertSeaTunnelSafe(t, h.fleet)

	// 新进程（1.0.1）启动：写就绪标记，watchdog 报告成功后收尾。
	// The new process (1.0.1) starts: writes the ready marker and finishes once the watchdog reports success.
	stxversion.Version = "1.0.1"
	s2 := NewService(h.dataDir)
	s2.SetAgentFleet(h.fleet)
	var m readyMarker
	if err := readJSONFile(h.plan.ReadyPath, &m); err != nil || m.PID != os.Getpid() || m.Version != "1.0.1" || m.TaskID != task.ID {
		t.Fatalf("ready marker = %+v %v", m, err)
	}
	if st := s2.Status(); st.ActiveTask == nil || st.ActiveTask.Status != StatusUpgrading {
		t.Fatal("restarted control plane should report the in-flight task")
	}
	_ = writeJSONFile(h.plan.ResultPath, restartResult{Status: restartOutcomeSucceeded,
		ControlPlane: ProcessRestart{Component: "control_plane", Strategy: RestartStrategyStandalone, Status: RestartSucceeded, OldPID: h.plan.OldPID, NewPID: 4242}})
	done := waitTaskDone(t, s2, task.ID)
	if done.Status != StatusSucceeded || stepByCode(done.Steps, StepVerify).Status != StepSucceeded || stepByCode(done.Steps, StepRestart).Status != StepSucceeded {
		t.Fatalf("finished task = %+v", done)
	}
	if len(done.Restarts) == 0 || done.Restarts[0].NewPID != 4242 {
		t.Fatalf("restarts = %+v", done.Restarts)
	}
	hist := s2.ListHistory()
	if len(hist) == 0 || hist[0].Status != StatusSucceeded || hist[0].ExecutionMode != ExecModeApply || hist[0].FromVersion != "1.0.0" || hist[0].DryRun {
		t.Fatalf("history = %+v", hist)
	}
	for _, f := range []string{activeTaskFile, restartPlanFile, restartReadyFile, restartResultFile} {
		if _, err := os.Stat(filepath.Join(h.s.stagingDir, f)); err == nil {
			t.Errorf("%s should be cleaned up", f)
		}
	}
	// 收尾只发生一次 / finishing happens once
	if s3 := NewService(h.dataDir); s3.Status().ActiveTask != nil {
		t.Fatal("a later start must not resume a finished task")
	}
}

func TestApplyRollbackAfterFailedRestartRestoresAgentsAndProxies(t *testing.T) {
	h := newApplyHarness(t)
	task := h.runApply(t)

	// watchdog 回滚后旧版本（1.0.0）重新启动。/ after the watchdog rollback the old version (1.0.0) starts again.
	stxversion.Version = "1.0.0"
	s2 := NewService(h.dataDir)
	s2.SetAgentFleet(h.fleet)
	s2.SetRolloutOptions(RolloutOptions{Concurrency: 2, CommandTimeout: time.Second, ReconnectTimeout: 300 * time.Millisecond, PollInterval: 5 * time.Millisecond})
	_ = writeJSONFile(h.plan.ResultPath, restartResult{Status: restartOutcomeRolledBack, Reason: "new control plane exited",
		ControlPlane: ProcessRestart{Component: "control_plane", Status: RestartRolledBack}})
	done := waitTaskDone(t, s2, task.ID)
	if done.Status != StatusFailed || done.FailureStep != StepRestart || !strings.Contains(done.FailureReason, "exited") {
		t.Fatalf("task = %+v", done)
	}
	if b, _ := os.ReadFile(h.exe); string(b) != "old-binary-1.0.0" {
		t.Fatalf("executable after rollback = %q", b)
	}
	if hist := s2.ListHistory(); len(hist) == 0 || hist[0].Status != "rolled_back" || hist[0].ExecutionMode != ExecModeApply {
		t.Fatalf("history = %+v", hist)
	}
	// Agent 与 proxy 随控制面回滚：先回滚 proxy jar（新 Agent 才认识），再回滚 Agent。
	// Agents and proxies follow the control plane: proxy jars first (only the new Agent knows it), then the Agent.
	deadline := time.Now().Add(5 * time.Second)
	for {
		n := nodeByHost(s2.taskAgentNodes(task.ID), 1)
		if n.Status == NodeUpgradeRolledBack && n.ProxyStatus == ProxyRolledBack {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("node1 not rolled back: %+v", n)
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.fleet.mu.Lock()
	proxyIdx, agentIdx := -1, -1
	for i, c := range h.fleet.commands {
		if c["sub_command"] == subCommandAgentRollbackProxyJars && proxyIdx < 0 {
			proxyIdx = i
		}
		if c["sub_command"] == subCommandAgentSelfRollback && agentIdx < 0 {
			agentIdx = i
		}
	}
	h.fleet.mu.Unlock()
	if proxyIdx < 0 || agentIdx < 0 || proxyIdx > agentIdx {
		t.Fatalf("rollback order proxy=%d agent=%d", proxyIdx, agentIdx)
	}
	assertSeaTunnelSafe(t, h.fleet)
}

func TestApplyRefusesRestartWithoutBackup(t *testing.T) {
	h := newApplyHarness(t)
	h.s.mu.Lock()
	h.s.task = &TaskState{ID: "t", Status: StatusUpgrading, Steps: []StepState{{Code: StepRestart}}}
	h.s.mu.Unlock()
	if err := h.s.beginControlPlaneRestart("t", "1.0.1"); err == nil || !strings.Contains(err.Error(), "backup") {
		t.Fatalf("err = %v", err)
	}
	if h.plan != nil {
		t.Fatal("watchdog must not start without a rollback path")
	}
}

func TestResumeMarksTaskWithoutPlanAsInterrupted(t *testing.T) {
	dir := t.TempDir()
	staging := filepath.Join(dir, "self-upgrade")
	_ = os.MkdirAll(staging, 0o755)
	_ = writeJSONFile(filepath.Join(staging, activeTaskFile), TaskState{ID: "x", Status: StatusUpgrading, TargetVersion: "1.0.1", ExecutionMode: ExecModeApply,
		Steps: []StepState{{Code: StepRestart, Status: StepRunning}}})
	s := NewService(dir)
	got, err := s.GetTask("x")
	if err != nil || got.Status != StatusFailed || !strings.Contains(got.FailureReason, "interrupted") && !strings.Contains(got.FailureReason, "stopped") {
		t.Fatalf("task = %+v %v", got, err)
	}
}
