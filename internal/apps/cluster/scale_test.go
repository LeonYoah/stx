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

package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	pluginapp "github.com/LeonYoah/stx/internal/apps/plugin"
	"gorm.io/gorm"
)

// ==================== 测试替身 / Test doubles ====================

// scaleFakeAgent 模拟各主机 Agent：进程运行状态、端口、连通性、Hazelcast 成员与 java-proxy 状态。
// scaleFakeAgent simulates host Agents: process state, ports, reachability, Hazelcast members and java-proxy state.
type scaleFakeAgent struct {
	mu           sync.Mutex
	javaFail     map[string]bool
	listening    map[string]bool // agent|port
	unreachable  map[string]bool // agent|host:port
	running      map[string]bool // agent|installDir
	members      map[string][]string
	stopFails    map[string]bool
	proxyHealthy map[string]bool
	commands     []mockAgentCommand
	onCommand    func(agentID, commandType string, params map[string]string)
}

func newScaleFakeAgent() *scaleFakeAgent {
	return &scaleFakeAgent{
		javaFail:     map[string]bool{},
		listening:    map[string]bool{},
		unreachable:  map[string]bool{},
		running:      map[string]bool{},
		members:      map[string][]string{},
		stopFails:    map[string]bool{},
		proxyHealthy: map[string]bool{},
	}
}

func precheckJSON(success bool, message string, details map[string]string) string {
	content, _ := json.Marshal(map[string]any{"success": success, "message": message, "details": details})
	return string(content)
}

func (a *scaleFakeAgent) SendCommand(_ context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	copied := make(map[string]string, len(params))
	for key, value := range params {
		copied[key] = value
	}
	a.mu.Lock()
	a.commands = append(a.commands, mockAgentCommand{agentID: agentID, commandType: commandType, params: copied})
	hook := a.onCommand
	success, message := a.handle(agentID, commandType, copied)
	a.mu.Unlock()
	if hook != nil {
		hook(agentID, commandType, copied)
	}
	return success, message, nil
}

func (a *scaleFakeAgent) handle(agentID, commandType string, params map[string]string) (bool, string) {
	isProxy := params["service"] == "stx_java_proxy"
	switch commandType {
	case "check_java":
		if a.javaFail[agentID] {
			return false, precheckJSON(false, "java not found", nil)
		}
		return true, precheckJSON(true, "java 11", nil)
	case "check_port":
		listening := a.listening[agentID+"|"+params["port"]]
		return listening, precheckJSON(listening, "port", nil)
	case "check_path_ready", "check_directory":
		return true, precheckJSON(true, "ok", nil)
	case "check_tcp":
		ok := !a.unreachable[agentID+"|"+params["host"]+":"+params["port"]]
		return ok, precheckJSON(ok, "tcp", nil)
	case "check_cluster_members":
		members := a.members[agentID]
		if len(members) == 0 {
			return false, precheckJSON(false, "no members", nil)
		}
		return true, precheckJSON(true, "members", map[string]string{
			"member_count": fmt.Sprintf("%d", len(members)),
			"members":      strings.Join(members, ","),
		})
	case "check_process":
		running := a.running[agentID+"|"+params["install_dir"]]
		if running {
			return true, precheckJSON(true, "found", map[string]string{"pid": "4321"})
		}
		return false, precheckJSON(false, "not found", nil)
	case "start":
		if isProxy {
			a.proxyHealthy[agentID] = true
			return true, `{"managed":true,"running":true,"healthy":true}`
		}
		a.running[agentID+"|"+params["install_dir"]] = true
		return true, "started"
	case "stop":
		if isProxy {
			a.proxyHealthy[agentID] = false
			return true, `{"managed":true,"running":false,"healthy":false}`
		}
		if a.stopFails[agentID] {
			return false, "stop failed: permission denied"
		}
		a.running[agentID+"|"+params["install_dir"]] = false
		return true, "stopped"
	case "restart":
		a.running[agentID+"|"+params["install_dir"]] = true
		return true, "restarted"
	case "status":
		healthy := a.proxyHealthy[agentID]
		return true, fmt.Sprintf(`{"managed":true,"running":%t,"healthy":%t}`, healthy, healthy)
	default:
		return true, "ok"
	}
}

func (a *scaleFakeAgent) count(agentID, commandType string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	total := 0
	for _, command := range a.commands {
		if (agentID == "" || command.agentID == agentID) && command.commandType == commandType {
			total++
		}
	}
	return total
}

func (a *scaleFakeAgent) setMembers(members []string, agentIDs ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, agentID := range agentIDs {
		a.members[agentID] = append([]string(nil), members...)
	}
}

// fakeScaleConfig 记录配置服务调用顺序与参数。
// fakeScaleConfig records config service call order and arguments.
type fakeScaleConfig struct {
	mu           sync.Mutex
	configs      []*appconfig.ConfigInfo
	calls        []string
	patchMembers [][]string
	applyMembers [][]string
	patchPartial bool
	applyPartial bool
	deleted      []uint
	registered   []uint
}

func (c *fakeScaleConfig) record(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, call)
}

func (c *fakeScaleConfig) GetByCluster(context.Context, uint) ([]*appconfig.ConfigInfo, error) {
	return c.configs, nil
}

func (c *fakeScaleConfig) PatchMemberList(_ context.Context, _ uint, _ string, members []string, _ string, _ uint) (*appconfig.MemberListPatchResult, error) {
	c.record("patch_members")
	c.mu.Lock()
	c.patchMembers = append(c.patchMembers, append([]string(nil), members...))
	partial := c.patchPartial
	c.mu.Unlock()
	result := &appconfig.MemberListPatchResult{Members: members, ConfigChangeResult: appconfig.ConfigChangeResult{Items: []*appconfig.ConfigChangeItem{}}}
	if partial {
		result.PendingCount = 1
	}
	return result, nil
}

func (c *fakeScaleConfig) ApplyTemplatesToHost(_ context.Context, _ uint, _ uint, _ string, members []string, _ string, _ uint) (*appconfig.ConfigChangeResult, error) {
	c.record("apply_templates")
	c.mu.Lock()
	c.applyMembers = append(c.applyMembers, append([]string(nil), members...))
	partial := c.applyPartial
	c.mu.Unlock()
	result := &appconfig.ConfigChangeResult{Items: []*appconfig.ConfigChangeItem{}}
	if partial {
		result.FailedCount = 1
	}
	return result, nil
}

func (c *fakeScaleConfig) RegisterHostConfigsFromNode(_ context.Context, _ uint, hostID uint, _ string, _ uint) (*appconfig.ConfigChangeResult, error) {
	c.record("register_configs")
	c.mu.Lock()
	c.registered = append(c.registered, hostID)
	c.mu.Unlock()
	return &appconfig.ConfigChangeResult{Items: []*appconfig.ConfigChangeItem{}}, nil
}

func (c *fakeScaleConfig) DeleteHostConfigs(_ context.Context, _ uint, hostID uint) (int, error) {
	c.record("delete_host_configs")
	c.mu.Lock()
	c.deleted = append(c.deleted, hostID)
	c.mu.Unlock()
	return 3, nil
}

// fakeScaleInstaller 模拟安装器；failuresLeft 次安装失败后成功。
// fakeScaleInstaller simulates the installer; installs fail failuresLeft times and then succeed.
type fakeScaleInstaller struct {
	requests     []*installerapp.InstallationRequest
	failuresLeft int
	lastStatus   *installerapp.InstallationStatus
}

func (i *fakeScaleInstaller) StartInstallation(_ context.Context, req *installerapp.InstallationRequest) (*installerapp.InstallationStatus, error) {
	i.requests = append(i.requests, req)
	status := &installerapp.InstallationStatus{ID: fmt.Sprintf("install-%d", len(i.requests)), Status: installerapp.StepStatusSuccess}
	if i.failuresLeft > 0 {
		i.failuresLeft--
		status.Status = installerapp.StepStatusFailed
		status.Error = "package download failed"
	}
	i.lastStatus = status
	return status, nil
}

func (i *fakeScaleInstaller) GetInstallationStatusSnapshot(context.Context, uint) (*installerapp.InstallationStatus, error) {
	if i.lastStatus == nil {
		return nil, installerapp.ErrInstallationNotFound
	}
	copied := *i.lastStatus
	return &copied, nil
}

type fakeScalePlugins struct {
	candidates []pluginapp.PluginReplayCandidate
	failed     int
	calls      int
}

func (p *fakeScalePlugins) ListReplayCandidates(context.Context, uint) ([]pluginapp.PluginReplayCandidate, error) {
	return p.candidates, nil
}

func (p *fakeScalePlugins) ReplayClusterPluginsToHost(_ context.Context, clusterID, hostID uint, installDir string) (*pluginapp.PluginReplayReport, error) {
	p.calls++
	return &pluginapp.PluginReplayReport{ClusterID: clusterID, HostID: hostID, InstallDir: installDir, Results: []pluginapp.PluginReplayResult{}, InstalledCount: len(p.candidates), FailedCount: p.failed}, nil
}

type fakeScaleJobs struct {
	jobs []ScaleRunningJob
	err  error
}

func (j *fakeScaleJobs) ListClusterRunningJobs(context.Context, uint) ([]ScaleRunningJob, error) {
	return j.jobs, j.err
}

type fakeScaleDiscoverer struct {
	processes map[uint][]ScaleDiscoveredProcess
}

func (d *fakeScaleDiscoverer) DiscoverSeaTunnelProcesses(_ context.Context, hostID uint) ([]ScaleDiscoveredProcess, error) {
	return d.processes[hostID], nil
}

type fakeScaleMonitor struct {
	pushes []string
	err    error
}

func (m *fakeScaleMonitor) PushClusterMonitorConfigToHost(_ context.Context, clusterID, hostID uint) error {
	m.pushes = append(m.pushes, fmt.Sprintf("%d/%d", clusterID, hostID))
	return m.err
}

// scaleTestEnv 汇总一个扩缩容测试环境。
// scaleTestEnv bundles one scaling test environment.
type scaleTestEnv struct {
	db        *gorm.DB
	svc       *Service
	scale     *ScaleService
	agent     *scaleFakeAgent
	config    *fakeScaleConfig
	installer *fakeScaleInstaller
	plugins   *fakeScalePlugins
	jobs      *fakeScaleJobs
	discover  *fakeScaleDiscoverer
	monitor   *fakeScaleMonitor
	cluster   *Cluster
	hosts     *MockHostProvider
}

const (
	scaleTestInstallDir = "/opt/seatunnel"
	scaleTestUser       = uint64(7)
)

// newScaleTestEnv 创建混合模式集群：主机 1..3（10.0.0.1..3），节点为 existingHosts 上运行中的 master/worker。
// newScaleTestEnv creates a hybrid cluster: hosts 1..3 (10.0.0.1..3) with running master/worker nodes on existingHosts.
func newScaleTestEnv(t *testing.T, mode DeploymentMode, existing map[uint]NodeRole) *scaleTestEnv {
	t.Helper()
	database, cleanup := setupServiceTestDB(t)
	t.Cleanup(cleanup)
	if err := database.AutoMigrate(&ClusterScaleOperation{}, &executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("migrate scale models: %v", err)
	}
	repo := NewRepository(database)
	hosts := NewMockHostProvider()
	now := time.Now()
	for id := uint(1); id <= 3; id++ {
		heartbeat := now
		hosts.AddHost(&HostInfo{ID: id, Name: fmt.Sprintf("t%d", id), IPAddress: fmt.Sprintf("10.0.0.%d", id), AgentID: fmt.Sprintf("agent-%d", id), AgentStatus: "installed", LastHeartbeat: &heartbeat})
	}
	svc := NewService(repo, hosts, &ServiceConfig{HeartbeatTimeout: time.Hour})
	agent := newScaleFakeAgent()
	svc.SetAgentCommandSender(agent)

	ctx := context.Background()
	cluster := &Cluster{Name: "scale-" + strings.ReplaceAll(t.Name(), "/", "_"), DeploymentMode: mode, Version: "2.3.12", InstallDir: scaleTestInstallDir, Status: ClusterStatusRunning}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	hostIDs := make([]uint, 0, len(existing))
	for hostID := range existing {
		hostIDs = append(hostIDs, hostID)
	}
	sort.Slice(hostIDs, func(i, j int) bool { return hostIDs[i] < hostIDs[j] })
	members := make([]string, 0)
	agents := make([]string, 0)
	for _, hostID := range hostIDs {
		role := existing[hostID]
		port := 5801
		if role == NodeRoleWorker {
			port = 5802
		}
		node := &ClusterNode{ClusterID: cluster.ID, HostID: hostID, Role: role, InstallDir: scaleTestInstallDir, HazelcastPort: port, APIPort: 8080, Status: NodeStatusRunning, ProcessPID: 100 + int(hostID)}
		if err := repo.AddNode(ctx, node); err != nil {
			t.Fatalf("add node: %v", err)
		}
		agentID := fmt.Sprintf("agent-%d", hostID)
		agent.running[agentID+"|"+scaleTestInstallDir] = true
		members = append(members, fmt.Sprintf("10.0.0.%d:%d", hostID, port))
		agents = append(agents, agentID)
	}
	agent.setMembers(members, agents...)

	config := &fakeScaleConfig{configs: []*appconfig.ConfigInfo{
		{ID: 1, ClusterID: cluster.ID, ConfigType: appconfig.ConfigTypeHazelcast, IsTemplate: true, Content: hazelcastMembers(members...)},
		{ID: 2, ClusterID: cluster.ID, ConfigType: appconfig.ConfigTypeSeatunnel, IsTemplate: true, Content: "seatunnel:\n  engine:\n    checkpoint:\n      storage:\n        type: hdfs\n        plugin-config:\n          storage.type: hdfs\n          fs.defaultFS: file:///tmp/\n"},
	}}
	scale := NewScaleService(svc, NewScaleRepository(database))
	scale.launch = func(fn func()) { fn() }
	scale.timing = scaleTiming{pollInterval: time.Millisecond, installTimeout: 30 * time.Millisecond, loopbackNodeTimeout: 30 * time.Millisecond, membershipTimeout: 30 * time.Millisecond, javaProxyTimeout: 30 * time.Millisecond, stopTimeout: 30 * time.Millisecond}
	env := &scaleTestEnv{
		db: database, svc: svc, scale: scale, agent: agent, config: config,
		installer: &fakeScaleInstaller{},
		plugins:   &fakeScalePlugins{candidates: []pluginapp.PluginReplayCandidate{{PluginName: "jdbc", ArtifactID: "connector-jdbc", Version: "2.3.12", Status: pluginapp.PluginStatusInstalled, ProfileKeys: []string{"postgresql"}}}},
		jobs:      &fakeScaleJobs{},
		discover:  &fakeScaleDiscoverer{processes: map[uint][]ScaleDiscoveredProcess{}},
		monitor:   &fakeScaleMonitor{},
		cluster:   cluster,
		hosts:     hosts,
	}
	scale.SetConfigManager(env.config)
	scale.SetInstaller(env.installer)
	scale.SetPluginReplayer(env.plugins)
	scale.SetRunningJobsProvider(env.jobs)
	scale.SetProcessDiscoverer(env.discover)
	scale.SetMonitorPusher(env.monitor)
	return env
}

func hazelcastMembers(members ...string) string {
	var builder strings.Builder
	builder.WriteString("hazelcast:\n  network:\n    join:\n      tcp-ip:\n        enabled: true\n        member-list:\n")
	for _, member := range members {
		builder.WriteString("          - " + member + "\n")
	}
	return builder.String()
}

func scaleActor() executionapp.Actor {
	return executionapp.Actor{UserID: scaleTestUser}
}

func findCheck(plan *ScalePlan, name string) []ScaleCheck {
	found := make([]ScaleCheck, 0)
	for _, check := range plan.Checks {
		if check.Name == name {
			found = append(found, check)
		}
	}
	return found
}

func (env *scaleTestEnv) mustPlanOut(t *testing.T, req ScaleOutRequest) *ScalePlan {
	t.Helper()
	plan, err := env.scale.PlanScaleOut(context.Background(), env.cluster.ID, &req, "en")
	if err != nil {
		t.Fatalf("PlanScaleOut: %v", err)
	}
	return plan
}

func (env *scaleTestEnv) loadOp(t *testing.T, id uint) *ClusterScaleOperation {
	t.Helper()
	op, err := env.scale.repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("load op: %v", err)
	}
	return op
}

func stepByName(op *ClusterScaleOperation, name ScaleStepName) ScaleStepState {
	for _, step := range op.steps() {
		if step.Name == name {
			return step
		}
	}
	return ScaleStepState{}
}

// joinOnStart 让新节点启动后所有 Agent 看到完整成员列表。
// joinOnStart makes every Agent see the complete member list once the new node starts.
func (env *scaleTestEnv) joinOnStart(newAgent string, members []string) {
	env.agent.onCommand = func(agentID, commandType string, params map[string]string) {
		if commandType == "start" && agentID == newAgent && params["service"] == "" {
			env.agent.setMembers(members, "agent-1", "agent-2", "agent-3")
		}
	}
}

// ==================== 计划 / Plan ====================

func TestPlanScaleOutInstallHappyPath(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})

	if plan.DetectedMode != ScaleModeInstall || plan.Mode != ScaleModeInstall {
		t.Fatalf("expected install mode, got detected=%s mode=%s", plan.DetectedMode, plan.Mode)
	}
	if !plan.CanSubmit {
		t.Fatalf("plan should be submittable, blocking=%v", plan.BlockingReasons)
	}
	if got := strings.Join(plan.MembersBefore, ","); got != "10.0.0.1:5801" {
		t.Fatalf("members_before = %s", got)
	}
	if got := strings.Join(plan.MembersAfter, ","); got != "10.0.0.1:5801,10.0.0.2:5801" {
		t.Fatalf("members_after = %s", got)
	}
	if plan.Target.InstallDir != scaleTestInstallDir || plan.Target.HazelcastPort != 5801 || plan.Target.APIPort != 8080 || plan.Target.Role != NodeRoleMasterWorker {
		t.Fatalf("unexpected target: %+v", plan.Target)
	}
	for _, name := range []string{"agent_online", "java", "port_free", "directory", "member_reachability", "member_address"} {
		checks := findCheck(plan, name)
		if len(checks) == 0 {
			t.Fatalf("missing check %s", name)
		}
		for _, check := range checks {
			if check.Status != ScaleCheckPassed {
				t.Fatalf("check %s should pass: %+v", name, check)
			}
		}
	}
	wantSteps := []ScaleStepName{ScaleStepValidatePlan, ScaleStepInstallFiles, ScaleStepRegisterNode, ScaleStepApplyConfigs, ScaleStepReplayPlugins, ScaleStepStartNode, ScaleStepPatchMembers, ScaleStepVerifyMembership}
	if fmt.Sprint(plan.Steps) != fmt.Sprint(wantSteps) {
		t.Fatalf("steps = %v", plan.Steps)
	}
	if len(plan.PluginsToReplay) != 1 || plan.PluginsToReplay[0].Name != "jdbc" {
		t.Fatalf("plugins_to_replay = %+v", plan.PluginsToReplay)
	}
	if plan.CheckpointStorage == nil || !plan.CheckpointStorage.Local || plan.CheckpointStorage.Location != "file:///tmp/" {
		t.Fatalf("checkpoint storage = %+v", plan.CheckpointStorage)
	}

	// 数组字段必须输出 [] 而不是 null（跨层契约）。
	// Array fields must render [] instead of null (cross-layer contract).
	content, _ := json.Marshal(plan)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(content, &raw)
	for _, key := range []string{"members_before", "members_after", "checks", "loopback_nodes", "plugins_to_replay", "running_jobs", "steps", "blocking_reasons", "warnings"} {
		if string(raw[key]) == "null" || len(raw[key]) == 0 {
			t.Fatalf("%s must be an array, got %s", key, raw[key])
		}
	}

	// 指纹稳定且与语言无关。
	// The fingerprint is stable and language independent.
	zhPlan, err := env.scale.PlanScaleOut(context.Background(), env.cluster.ID, &ScaleOutRequest{HostID: 2}, "zh")
	if err != nil {
		t.Fatalf("PlanScaleOut zh: %v", err)
	}
	if zhPlan.Fingerprint != plan.Fingerprint || !strings.HasPrefix(plan.Fingerprint, "sha256:") {
		t.Fatalf("fingerprint should be stable: %s vs %s", zhPlan.Fingerprint, plan.Fingerprint)
	}
}

func TestPlanScaleOutBlockingAndWarningBranches(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(env *scaleTestEnv)
		req     ScaleOutRequest
		check   string
		blocked bool
	}{
		{name: "java missing", setup: func(env *scaleTestEnv) { env.agent.javaFail["agent-2"] = true }, req: ScaleOutRequest{HostID: 2}, check: "java", blocked: true},
		{name: "port in use", setup: func(env *scaleTestEnv) { env.agent.listening["agent-2|8080"] = true }, req: ScaleOutRequest{HostID: 2, Mode: ScaleModeInstall}, check: "port_free", blocked: true},
		{name: "member unreachable", setup: func(env *scaleTestEnv) { env.agent.unreachable["agent-2|10.0.0.1:5801"] = true }, req: ScaleOutRequest{HostID: 2}, check: "member_reachability", blocked: true},
		{name: "duplicate ip", setup: func(env *scaleTestEnv) { env.hosts.hosts[2].IPAddress = "10.0.0.1" }, req: ScaleOutRequest{HostID: 2}, check: "member_address", blocked: true},
		{name: "agent offline", setup: func(env *scaleTestEnv) {
			old := time.Now().Add(-48 * time.Hour)
			env.hosts.hosts[2].LastHeartbeat = &old
		}, req: ScaleOutRequest{HostID: 2}, check: "agent_online", blocked: true},
		{name: "register without running seatunnel", req: ScaleOutRequest{HostID: 2, Mode: ScaleModeRegister}, check: "port_listening", blocked: true},
		{name: "node already exists", req: ScaleOutRequest{HostID: 1}, check: "node_exists", blocked: true},
		{name: "high disk usage", setup: func(env *scaleTestEnv) { env.hosts.hosts[2].DiskUsage = 95 }, req: ScaleOutRequest{HostID: 2}, check: "disk_space", blocked: false},
		{name: "running jobs unknown", setup: func(env *scaleTestEnv) { env.jobs.err = errors.New("engine down") }, req: ScaleOutRequest{HostID: 2}, check: "running_jobs", blocked: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
			if tc.setup != nil {
				tc.setup(env)
			}
			plan := env.mustPlanOut(t, tc.req)
			checks := findCheck(plan, tc.check)
			if len(checks) == 0 {
				t.Fatalf("missing check %s in %+v", tc.check, plan.Checks)
			}
			hasBad := false
			for _, check := range checks {
				if check.Status == ScaleCheckFailed || check.Status == ScaleCheckWarning {
					hasBad = true
				}
			}
			if !hasBad {
				t.Fatalf("check %s should be failed or warning: %+v", tc.check, checks)
			}
			if plan.CanSubmit == tc.blocked {
				t.Fatalf("can_submit=%v but blocked=%v (reasons %v)", plan.CanSubmit, tc.blocked, plan.BlockingReasons)
			}
			if tc.blocked && len(plan.BlockingReasons) == 0 {
				t.Fatal("blocked plan must list blocking reasons")
			}
		})
	}
}

func TestPlanScaleOutDetectsRegisterModeAndVersionMismatch(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.discover.processes[2] = []ScaleDiscoveredProcess{{PID: 9, Role: "hybrid", InstallDir: "/data/seatunnel", Version: "2.3.12", HazelcastPort: 5901, APIPort: 8090}}
	env.agent.listening["agent-2|5901"] = true

	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if plan.DetectedMode != ScaleModeRegister || plan.Mode != ScaleModeRegister {
		t.Fatalf("expected register mode, got %s/%s", plan.DetectedMode, plan.Mode)
	}
	if plan.Target.InstallDir != "/data/seatunnel" || plan.Target.HazelcastPort != 5901 || plan.Target.APIPort != 8090 {
		t.Fatalf("register target should come from discovery: %+v", plan.Target)
	}
	if !plan.CanSubmit {
		t.Fatalf("register plan should pass: %v", plan.BlockingReasons)
	}
	if fmt.Sprint(plan.Steps) != fmt.Sprint([]ScaleStepName{ScaleStepValidatePlan, ScaleStepRegisterNode, ScaleStepApplyConfigs, ScaleStepReplayPlugins, ScaleStepPatchMembers, ScaleStepVerifyMembership}) {
		t.Fatalf("register steps = %v", plan.Steps)
	}

	env.discover.processes[2][0].Version = "2.3.13"
	mismatch := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if mismatch.CanSubmit || findCheck(mismatch, "version")[0].Status != ScaleCheckFailed {
		t.Fatalf("version mismatch must block: %+v", findCheck(mismatch, "version"))
	}

	// 手动切换为安装模式时，已占用的端口会阻断。
	// Manually switching to install mode is blocked by the occupied port.
	forced := env.mustPlanOut(t, ScaleOutRequest{HostID: 2, Mode: ScaleModeInstall, InstallDir: "/data/seatunnel", Ports: ScaleNodePorts{HazelcastPort: 5901}})
	if forced.DetectedMode != ScaleModeRegister || forced.Mode != ScaleModeInstall || forced.CanSubmit {
		t.Fatalf("forced install over a running node must be blocked: %+v", forced.BlockingReasons)
	}
}

func TestPlanScaleOutDetectsLoopbackMembers(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	hostID := uint(1)
	env.config.configs = append(env.config.configs, &appconfig.ConfigInfo{ID: 10, ClusterID: env.cluster.ID, HostID: &hostID, ConfigType: appconfig.ConfigTypeHazelcast, Content: hazelcastMembers("127.0.0.1:5801")})

	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if len(plan.LoopbackNodes) != 1 || plan.LoopbackNodes[0].Source != "config" || plan.LoopbackNodes[0].Address != "127.0.0.1:5801" {
		t.Fatalf("loopback nodes = %+v", plan.LoopbackNodes)
	}
	if !plan.RequiresRestartAck || !plan.CanSubmit {
		t.Fatalf("loopback without jobs requires ack but can be submitted: ack=%v can=%v", plan.RequiresRestartAck, plan.CanSubmit)
	}
	if plan.Steps[1] != ScaleStepFixLoopback {
		t.Fatalf("fix_loopback must run right after validation: %v", plan.Steps)
	}

	env.jobs.jobs = []ScaleRunningJob{{JobID: "1", JobName: "orders"}}
	withJobs := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if withJobs.CanSubmit {
		t.Fatal("loopback fix with running jobs must be blocked")
	}
	if withJobs.Fingerprint == plan.Fingerprint {
		t.Fatal("running jobs must change the fingerprint when loopback nodes exist")
	}

	// 运行时成员为回环（节点配置看似正常）也要识别。
	// Loopback runtime members are detected even when node configs look fine.
	runtimeEnv := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	runtimeEnv.agent.setMembers([]string{"127.0.0.1:5801"}, "agent-1")
	runtimePlan := runtimeEnv.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if len(runtimePlan.LoopbackNodes) != 1 || runtimePlan.LoopbackNodes[0].Source != "runtime" {
		t.Fatalf("runtime loopback not detected: %+v", runtimePlan.LoopbackNodes)
	}
}

func TestPlanScaleInGuardrails(t *testing.T) {
	ctx := context.Background()
	t.Run("last node blocked", func(t *testing.T) {
		env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
		nodes, _ := env.svc.repo.GetNodesByClusterID(ctx, env.cluster.ID)
		plan, err := env.scale.PlanScaleIn(ctx, env.cluster.ID, nodes[0].ID, "en")
		if err != nil {
			t.Fatalf("PlanScaleIn: %v", err)
		}
		if plan.CanSubmit || findCheck(plan, "last_node")[0].Status != ScaleCheckFailed {
			t.Fatalf("removing the last node must be blocked: %+v", plan.Checks)
		}
	})
	t.Run("last master blocked in separated mode", func(t *testing.T) {
		env := newScaleTestEnv(t, DeploymentModeSeparated, map[uint]NodeRole{1: NodeRoleMaster, 2: NodeRoleWorker})
		master, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 1, string(NodeRoleMaster))
		plan, err := env.scale.PlanScaleIn(ctx, env.cluster.ID, master.ID, "en")
		if err != nil {
			t.Fatalf("PlanScaleIn: %v", err)
		}
		if plan.CanSubmit || findCheck(plan, "last_master")[0].Status != ScaleCheckFailed {
			t.Fatalf("removing the last master must be blocked: %+v", plan.Checks)
		}
		if strings.Join(plan.MembersAfter, ",") != "10.0.0.2:5802" {
			t.Fatalf("separated members_after = %v", plan.MembersAfter)
		}
	})
	t.Run("running jobs require ack and warn about local checkpoints", func(t *testing.T) {
		env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
		env.jobs.jobs = []ScaleRunningJob{{JobID: "77", JobName: "cdc"}}
		node, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
		plan, err := env.scale.PlanScaleIn(ctx, env.cluster.ID, node.ID, "en")
		if err != nil {
			t.Fatalf("PlanScaleIn: %v", err)
		}
		if !plan.CanSubmit || !plan.RequiresJobsAck || len(plan.RunningJobs) != 1 {
			t.Fatalf("running jobs should require ack without blocking: %+v", plan)
		}
		if len(findCheck(plan, "checkpoint_storage")) != 1 {
			t.Fatalf("local checkpoint warning missing: %+v", plan.Checks)
		}
		if plan.JavaProxyFollow != nil {
			t.Fatalf("removing a non-proxy node must not follow java-proxy: %+v", plan.JavaProxyFollow)
		}
		if fmt.Sprint(plan.Steps) != fmt.Sprint([]ScaleStepName{ScaleStepValidatePlan, ScaleStepStopNode, ScaleStepPatchMembers, ScaleStepCleanupRecords, ScaleStepRefreshMonitor, ScaleStepVerify}) {
			t.Fatalf("scale-in steps = %v", plan.Steps)
		}
	})
	t.Run("unknown jobs require ack and java-proxy follows", func(t *testing.T) {
		env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
		env.jobs.err = errors.New("engine unreachable")
		node, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 1, string(NodeRoleMasterWorker))
		plan, err := env.scale.PlanScaleIn(ctx, env.cluster.ID, node.ID, "en")
		if err != nil {
			t.Fatalf("PlanScaleIn: %v", err)
		}
		if !plan.RequiresJobsAck || plan.RunningJobsKnown {
			t.Fatalf("unknown jobs must require ack: %+v", plan)
		}
		if plan.JavaProxyFollow == nil || plan.JavaProxyFollow.FromNodeID != node.ID || plan.JavaProxyFollow.ToHostID != 2 {
			t.Fatalf("java-proxy follow = %+v", plan.JavaProxyFollow)
		}
	})
}

// ==================== 提交 / Submission ====================

func TestSubmitRejectsChangedFingerprintAndMissingAcks(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	env.jobs.jobs = []ScaleRunningJob{{JobID: "1"}}
	node, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
	plan, _ := env.scale.PlanScaleIn(ctx, env.cluster.ID, node.ID, "en")

	if _, err := env.scale.SubmitScaleIn(ctx, scaleActor(), env.cluster.ID, node.ID, &ScaleInSubmitRequest{PlanFingerprint: "sha256:stale", JobsRiskAck: true}, ScaleExecutionRequest{}); !errors.Is(err, ErrScalePlanChanged) {
		t.Fatalf("stale fingerprint must be rejected, got %v", err)
	}
	if _, err := env.scale.SubmitScaleIn(ctx, scaleActor(), env.cluster.ID, node.ID, &ScaleInSubmitRequest{PlanFingerprint: plan.Fingerprint}, ScaleExecutionRequest{}); !errors.Is(err, ErrScaleJobsRiskAckRequired) {
		t.Fatalf("missing jobs_risk_ack must be rejected, got %v", err)
	}

	loopEnv := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	loopEnv.agent.setMembers([]string{"127.0.0.1:5801"}, "agent-1")
	loopPlan := loopEnv.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	_, err := loopEnv.scale.SubmitScaleOut(ctx, scaleActor(), loopEnv.cluster.ID, &ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: loopPlan.Fingerprint}, ScaleExecutionRequest{})
	if !errors.Is(err, ErrScaleRestartAckRequired) {
		t.Fatalf("missing loopback_restart_ack must be rejected, got %v", err)
	}
	var count int64
	loopEnv.db.Model(&ClusterScaleOperation{}).Count(&count)
	if count != 0 {
		t.Fatalf("rejected submissions must not create operations, got %d", count)
	}
	if status, code := scaleErrorStatus(ErrScaleRestartAckRequired); status != http.StatusBadRequest || code != ErrorCodeScaleRestartAckRequired {
		t.Fatalf("ack error mapping = %d %s", status, code)
	}
}

func TestSubmitIsMutuallyExclusivePerCluster(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	running := &ClusterScaleOperation{ClusterID: env.cluster.ID, Kind: ScaleKindOut, Status: ScaleOperationRunning, CreatedBy: 99}
	if err := env.scale.repo.Create(ctx, running); err != nil {
		t.Fatalf("seed running op: %v", err)
	}
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	_, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID, &ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan.Fingerprint}, ScaleExecutionRequest{})
	if !errors.Is(err, ErrClusterScaleInProgress) {
		t.Fatalf("expected ErrClusterScaleInProgress, got %v", err)
	}
	if status, code := scaleErrorStatus(err); status != http.StatusConflict || code != ErrorCodeClusterScaleInProgress {
		t.Fatalf("expected 409 cluster_scale_in_progress, got %d %s", status, code)
	}
}

func TestSubmitScaleInRequiresR2ConfirmationAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	executionService := executionapp.NewService(executionapp.NewRepository(env.db), nil)
	env.scale.SetExecutionService(executionService)
	env.agent.setMembers([]string{"10.0.0.1:5801"}, "agent-1")
	launched := 0
	env.scale.launch = func(func()) { launched++ }

	node, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
	plan, _ := env.scale.PlanScaleIn(ctx, env.cluster.ID, node.ID, "en")
	req := &ScaleInSubmitRequest{PlanFingerprint: plan.Fingerprint}
	meta := ScaleExecutionRequest{RequestID: "req-1", IdempotencyKey: "key-1", RequestHash: "hash-1", ClientType: "web"}

	_, err := env.scale.SubmitScaleIn(ctx, scaleActor(), env.cluster.ID, node.ID, req, meta)
	var confirmation *executionapp.ConfirmationRequiredError
	if !errors.As(err, &confirmation) || confirmation.ConfirmationID == "" {
		t.Fatalf("first R2 request must return a confirmation id, got %v", err)
	}
	if launched != 0 {
		t.Fatal("nothing may run before confirmation")
	}

	meta.ConfirmationID = confirmation.ConfirmationID
	result, err := env.scale.SubmitScaleIn(ctx, scaleActor(), env.cluster.ID, node.ID, req, meta)
	if err != nil {
		t.Fatalf("confirmed submission: %v", err)
	}
	if result.ExecutionID == "" || result.OperationID == 0 || launched != 1 {
		t.Fatalf("unexpected submit result: %+v launched=%d", result, launched)
	}
	item, err := executionService.Get(ctx, scaleActor(), result.ExecutionID)
	if err != nil {
		t.Fatalf("execution lookup: %v", err)
	}
	if item.Status != executionapp.StatusRunning || item.ModuleRef != scaleModuleRef(result.OperationID) || item.RiskLevel != executionapp.RiskLevelR2 || item.RequestID != "req-1" {
		t.Fatalf("unexpected execution: %+v", item)
	}

	replay, err := env.scale.SubmitScaleIn(ctx, scaleActor(), env.cluster.ID, node.ID, req, ScaleExecutionRequest{IdempotencyKey: "key-1", RequestHash: "hash-1"})
	if err != nil || replay.OperationID != result.OperationID || launched != 1 {
		t.Fatalf("idempotent replay must return the same operation without relaunching: %+v %v launched=%d", replay, err, launched)
	}
}

// ==================== 执行 / Execution ====================

func TestScaleOutInstallRunsStepsInOrder(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	executionService := executionapp.NewService(executionapp.NewRepository(env.db), nil)
	env.scale.SetExecutionService(executionService)
	env.joinOnStart("agent-2", []string{"10.0.0.1:5801", "10.0.0.2:5801"})

	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	req := &ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan.Fingerprint}
	meta := ScaleExecutionRequest{RequestID: "req-out", IdempotencyKey: "key-out", RequestHash: "hash-out"}
	_, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID, req, meta)
	var confirmation *executionapp.ConfirmationRequiredError
	if !errors.As(err, &confirmation) {
		t.Fatalf("expected confirmation, got %v", err)
	}
	meta.ConfirmationID = confirmation.ConfirmationID
	result, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID, req, meta)
	if err != nil {
		t.Fatalf("SubmitScaleOut: %v", err)
	}

	op := env.loadOp(t, result.OperationID)
	if op.Status != ScaleOperationSucceeded {
		t.Fatalf("operation should succeed, got %s: %s (steps %s)", op.Status, op.Error, op.StepsJSON)
	}
	for _, step := range op.steps() {
		if step.Status != ScaleStepSucceeded || step.Attempts != 1 {
			t.Fatalf("step %s = %s attempts=%d err=%s", step.Name, step.Status, step.Attempts, step.Error)
		}
	}
	if len(env.installer.requests) != 1 {
		t.Fatalf("expected one installation, got %d", len(env.installer.requests))
	}
	install := env.installer.requests[0]
	if !install.DeferNodeStart || install.Version != "2.3.12" || install.ClusterID != fmt.Sprint(env.cluster.ID) || strings.Join(install.MasterAddresses, ",") != "10.0.0.1,10.0.0.2" {
		t.Fatalf("unexpected installation request: %+v", install)
	}
	if got := strings.Join(env.config.calls, ","); got != "apply_templates,register_configs,patch_members" {
		t.Fatalf("config call order = %s", got)
	}
	if got := strings.Join(env.config.applyMembers[0], ","); got != "10.0.0.1:5801,10.0.0.2:5801" {
		t.Fatalf("templates applied with members %s", got)
	}
	// 新节点必须在应用模板之后、修补其他节点成员之前启动。
	// The new node must start after templates are applied and before other nodes' members are patched.
	if env.agent.count("agent-2", "start") != 1 || env.plugins.calls != 1 {
		t.Fatalf("start=%d replay=%d", env.agent.count("agent-2", "start"), env.plugins.calls)
	}
	nodes, _ := env.svc.repo.GetNodesByClusterID(ctx, env.cluster.ID)
	if len(nodes) != 2 || len(op.nodeIDs()) != 1 {
		t.Fatalf("expected the new node registered: nodes=%d op nodes=%v", len(nodes), op.nodeIDs())
	}
	if len(env.monitor.pushes) != 1 || env.monitor.pushes[0] != fmt.Sprintf("%d/2", env.cluster.ID) {
		t.Fatalf("monitor pushes = %v", env.monitor.pushes)
	}
	item, _ := executionService.Get(ctx, scaleActor(), result.ExecutionID)
	if item.Status != executionapp.StatusSucceeded || item.ResultRef != scaleResultRef(env.cluster.ID, op.ID) {
		t.Fatalf("execution should succeed with result ref: %+v", item)
	}
}

func TestScaleOutFailureRetriesFromFailedStep(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.installer.failuresLeft = 1
	env.joinOnStart("agent-2", []string{"10.0.0.1:5801", "10.0.0.2:5801"})

	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	result, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID, &ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan.Fingerprint}, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("SubmitScaleOut: %v", err)
	}
	op := env.loadOp(t, result.OperationID)
	if op.Status != ScaleOperationFailed || op.CurrentStep != ScaleStepInstallFiles || !strings.Contains(op.Error, "package download failed") {
		t.Fatalf("expected failure at install_files, got %s/%s: %s", op.Status, op.CurrentStep, op.Error)
	}
	if stepByName(op, ScaleStepRegisterNode).Status != ScaleStepPending {
		t.Fatal("steps after the failure must stay pending")
	}
	if info := op.toInfo(); !info.Retryable || info.Warnings == nil || info.NodeIDs == nil {
		t.Fatalf("failed operation info: %+v", info)
	}

	// 非创建者且非管理员不能重试。
	// Neither the creator nor an admin: retry is denied.
	if _, err := env.scale.RetryOperation(ctx, executionapp.Actor{UserID: 99}, env.cluster.ID, op.ID, ScaleExecutionRequest{}); !errors.Is(err, ErrScaleOperationNotFound) {
		t.Fatalf("other users must not retry, got %v", err)
	}
	retried, err := env.scale.RetryOperation(ctx, scaleActor(), env.cluster.ID, op.ID, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("RetryOperation: %v", err)
	}
	op = env.loadOp(t, retried.OperationID)
	if op.Status != ScaleOperationSucceeded {
		t.Fatalf("retry should succeed, got %s: %s", op.Status, op.Error)
	}
	if attempts := stepByName(op, ScaleStepValidatePlan).Attempts; attempts != 1 {
		t.Fatalf("completed steps must not re-run on retry, validate_plan attempts=%d", attempts)
	}
	if attempts := stepByName(op, ScaleStepInstallFiles).Attempts; attempts != 2 {
		t.Fatalf("failed step must re-run, install_files attempts=%d", attempts)
	}
	if _, err := env.scale.RetryOperation(ctx, scaleActor(), env.cluster.ID, op.ID, ScaleExecutionRequest{}); !errors.Is(err, ErrScaleOperationNotRetryable) {
		t.Fatalf("succeeded operations are not retryable, got %v", err)
	}
}

func TestScaleOutFixLoopbackRestartsLoopbackNodes(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.agent.setMembers([]string{"127.0.0.1:5801"}, "agent-1")
	env.agent.onCommand = func(agentID, commandType string, params map[string]string) {
		if commandType == "restart" && agentID == "agent-1" {
			env.agent.setMembers([]string{"10.0.0.1:5801"}, "agent-1")
		}
		if commandType == "start" && agentID == "agent-2" && params["service"] == "" {
			env.agent.setMembers([]string{"10.0.0.1:5801", "10.0.0.2:5801"}, "agent-1", "agent-2")
		}
	}
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if !plan.RequiresRestartAck {
		t.Fatal("plan should require restart ack")
	}
	result, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID, &ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan.Fingerprint, LoopbackRestartAck: true}, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("SubmitScaleOut: %v", err)
	}
	op := env.loadOp(t, result.OperationID)
	if op.Status != ScaleOperationSucceeded {
		t.Fatalf("operation should succeed, got %s: %s", op.Status, op.Error)
	}
	if env.agent.count("agent-1", "restart") != 1 {
		t.Fatalf("loopback node must be restarted once, got %d", env.agent.count("agent-1", "restart"))
	}
	if env.config.calls[0] != "register_configs" || env.config.calls[1] != "patch_members" || strings.Join(env.config.patchMembers[0], ",") != "10.0.0.1:5801" {
		t.Fatalf("loopback fix must register missing node configs and patch routable members first: calls=%v members=%v", env.config.calls, env.config.patchMembers)
	}

	// 幂等：节点已以可路由地址运行时重跑不再重启。
	// Idempotent: re-running when the node already runs with a routable address does not restart it again.
	outcome := env.scale.stepFixLoopback(ctx, op, plan, "en")
	if outcome.status != ScaleStepSucceeded || env.agent.count("agent-1", "restart") != 1 {
		t.Fatalf("re-run should skip restart: %+v restarts=%d", outcome, env.agent.count("agent-1", "restart"))
	}
}

func TestScaleStepsAreIdempotentOnRerun(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	node2, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
	op := &ClusterScaleOperation{ID: 1, ClusterID: env.cluster.ID, CreatedBy: uint(scaleTestUser)}
	op.setNodeIDs([]uint{node2.ID})
	outPlan := &ScalePlan{Kind: ScaleKindOut, Mode: ScaleModeInstall, Target: ScaleTarget{HostID: 2, Role: NodeRoleMasterWorker, InstallDir: scaleTestInstallDir, HazelcastPort: 5801}}

	// start_node：进程已运行时不再下发 start。
	// start_node: no start command when the process already runs.
	for i := 0; i < 2; i++ {
		if outcome := env.scale.stepStartNode(ctx, op, outPlan, "en"); outcome.status != ScaleStepSucceeded {
			t.Fatalf("start_node rerun %d: %+v", i, outcome)
		}
	}
	if env.agent.count("agent-2", "start") != 0 {
		t.Fatal("start must not be sent to an already running node")
	}

	// register_node：重复登记不新增节点。
	// register_node: repeated registration never adds a node.
	for i := 0; i < 2; i++ {
		if outcome := env.scale.stepRegisterNode(ctx, op, outPlan, "en"); outcome.status != ScaleStepSucceeded {
			t.Fatalf("register_node rerun %d: %+v", i, outcome)
		}
	}
	if count, _ := env.svc.repo.CountNodesByClusterID(ctx, env.cluster.ID); count != 2 {
		t.Fatalf("expected 2 nodes after repeated registration, got %d", count)
	}

	// patch_members：重复修补使用同一成员列表。
	// patch_members: repeated patches use the same member list.
	for i := 0; i < 2; i++ {
		if outcome := env.scale.stepPatchMembers(ctx, op, outPlan, "en"); outcome.status != ScaleStepSucceeded {
			t.Fatalf("patch_members rerun %d: %+v", i, outcome)
		}
	}
	if fmt.Sprint(env.config.patchMembers[0]) != fmt.Sprint(env.config.patchMembers[1]) {
		t.Fatalf("patch members differ between reruns: %v", env.config.patchMembers)
	}

	// stop_node / cleanup_records：已停止、已删除时重跑仍成功。
	// stop_node / cleanup_records: re-runs succeed when already stopped / deleted.
	inPlan := &ScalePlan{Kind: ScaleKindIn, Target: ScaleTarget{NodeID: node2.ID, HostID: 2, Role: NodeRoleMasterWorker, InstallDir: scaleTestInstallDir}}
	for i := 0; i < 2; i++ {
		if outcome := env.scale.stepStopNode(ctx, op, inPlan, "en"); outcome.status != ScaleStepSucceeded {
			t.Fatalf("stop_node rerun %d: %+v", i, outcome)
		}
	}
	if env.agent.count("agent-2", "stop") != 1 {
		t.Fatalf("stop must be sent once, got %d", env.agent.count("agent-2", "stop"))
	}
	for i := 0; i < 2; i++ {
		if outcome := env.scale.stepCleanupRecords(ctx, op, inPlan, "en"); outcome.status != ScaleStepSucceeded {
			t.Fatalf("cleanup_records rerun %d: %+v", i, outcome)
		}
	}
	if outcome := env.scale.stepStopNode(ctx, op, inPlan, "en"); outcome.status != ScaleStepSkipped {
		t.Fatalf("stop_node after cleanup should be skipped: %+v", outcome)
	}
}

func TestScaleInStopFailureKeepsNodeRecord(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	env.agent.stopFails["agent-2"] = true
	node, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
	plan, _ := env.scale.PlanScaleIn(ctx, env.cluster.ID, node.ID, "en")

	result, err := env.scale.SubmitScaleIn(ctx, scaleActor(), env.cluster.ID, node.ID, &ScaleInSubmitRequest{PlanFingerprint: plan.Fingerprint}, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("SubmitScaleIn: %v", err)
	}
	op := env.loadOp(t, result.OperationID)
	if op.Status != ScaleOperationFailed || op.CurrentStep != ScaleStepStopNode {
		t.Fatalf("expected failure at stop_node, got %s/%s", op.Status, op.CurrentStep)
	}
	if _, err := env.svc.repo.GetNodeByID(ctx, node.ID); err != nil {
		t.Fatalf("node record must be kept when stop fails: %v", err)
	}
	if len(env.config.deleted) != 0 || len(env.config.patchMembers) != 0 {
		t.Fatalf("no cleanup or patch after a failed stop: deleted=%v patches=%v", env.config.deleted, env.config.patchMembers)
	}
}

func TestScaleInRemovesNodeAndFollowsJavaProxy(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	env.agent.proxyHealthy["agent-1"] = true
	env.agent.onCommand = func(agentID, commandType string, params map[string]string) {
		if commandType == "stop" && agentID == "agent-1" && params["service"] == "" {
			env.agent.setMembers([]string{"10.0.0.2:5801"}, "agent-2")
		}
	}
	node, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 1, string(NodeRoleMasterWorker))
	plan, _ := env.scale.PlanScaleIn(ctx, env.cluster.ID, node.ID, "en")
	if plan.JavaProxyFollow == nil {
		t.Fatal("removing the first master must plan a java-proxy follow")
	}
	result, err := env.scale.SubmitScaleIn(ctx, scaleActor(), env.cluster.ID, node.ID, &ScaleInSubmitRequest{PlanFingerprint: plan.Fingerprint}, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("SubmitScaleIn: %v", err)
	}
	op := env.loadOp(t, result.OperationID)
	if op.Status != ScaleOperationSucceeded {
		t.Fatalf("scale-in should succeed, got %s: %s (%s)", op.Status, op.Error, op.StepsJSON)
	}
	if _, err := env.svc.repo.GetNodeByID(ctx, node.ID); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("node record should be deleted, got %v", err)
	}
	if strings.Join(env.config.patchMembers[0], ",") != "10.0.0.2:5801" {
		t.Fatalf("remaining members = %v", env.config.patchMembers)
	}
	if len(env.config.deleted) != 1 || env.config.deleted[0] != 1 {
		t.Fatalf("host configs of host 1 should be deleted: %v", env.config.deleted)
	}
	if len(env.monitor.pushes) != 1 || env.monitor.pushes[0] != fmt.Sprintf("%d/1", env.cluster.ID) {
		t.Fatalf("monitor must be refreshed for host 1: %v", env.monitor.pushes)
	}
	if !env.agent.proxyHealthy["agent-2"] || env.agent.proxyHealthy["agent-1"] {
		t.Fatalf("java-proxy should move from agent-1 to agent-2: %v", env.agent.proxyHealthy)
	}
	if order := strings.Join(env.config.calls, ","); order != "patch_members,delete_host_configs" {
		t.Fatalf("config call order = %s", order)
	}
}

func TestRecoverInterruptedOperationsMarksFailed(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	executionService := executionapp.NewService(executionapp.NewRepository(env.db), nil)
	env.scale.SetExecutionService(executionService)
	item, _, err := executionService.Create(ctx, executionapp.CreateInput{OperationID: scaleOutOperationID, OwnerUserID: scaleTestUser, Module: scaleExecutionModule, RiskLevel: executionapp.RiskLevelR2, Status: executionapp.StatusRunning})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	op := &ClusterScaleOperation{ClusterID: env.cluster.ID, Kind: ScaleKindOut, Status: ScaleOperationRunning, ExecutionID: item.ExecutionID, CreatedBy: uint(scaleTestUser)}
	op.setSteps([]ScaleStepState{{Name: ScaleStepValidatePlan, Status: ScaleStepSucceeded}, {Name: ScaleStepInstallFiles, Status: ScaleStepRunning, Attempts: 1}})
	if err := env.scale.repo.Create(ctx, op); err != nil {
		t.Fatalf("create op: %v", err)
	}

	recovered, err := env.scale.RecoverInterruptedOperations(ctx)
	if err != nil || recovered != 1 {
		t.Fatalf("RecoverInterruptedOperations = %d, %v", recovered, err)
	}
	op = env.loadOp(t, op.ID)
	if op.Status != ScaleOperationFailed || op.Error != scaleErrorInterruptedByRestart || op.FinishedAt == nil {
		t.Fatalf("interrupted op = %+v", op)
	}
	if step := stepByName(op, ScaleStepInstallFiles); step.Status != ScaleStepFailed || step.Error != scaleErrorInterruptedByRestart {
		t.Fatalf("running step should be failed: %+v", step)
	}
	if step := stepByName(op, ScaleStepValidatePlan); step.Status != ScaleStepSucceeded {
		t.Fatalf("finished steps keep their status: %+v", step)
	}
	execution, _ := executionService.Get(ctx, scaleActor(), item.ExecutionID)
	if execution.Status != executionapp.StatusFailed {
		t.Fatalf("execution should be failed, got %s", execution.Status)
	}
}

func TestScaleOperationListIsOwnerScoped(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	for _, owner := range []uint{uint(scaleTestUser), 99} {
		op := &ClusterScaleOperation{ClusterID: env.cluster.ID, Kind: ScaleKindOut, Status: ScaleOperationFailed, CreatedBy: owner}
		if err := env.scale.repo.Create(ctx, op); err != nil {
			t.Fatalf("create op: %v", err)
		}
	}
	mine, err := env.scale.ListOperations(ctx, scaleActor(), env.cluster.ID, "", 0)
	if err != nil || len(mine) != 1 {
		t.Fatalf("user should only see own operations: %d %v", len(mine), err)
	}
	all, err := env.scale.ListOperations(ctx, executionapp.Actor{UserID: 1, IsAdmin: true}, env.cluster.ID, ScaleOperationFailed, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("admin should see all operations: %d %v", len(all), err)
	}
	if all[0].Steps == nil || all[0].Warnings == nil || all[0].NodeIDs == nil {
		t.Fatalf("info arrays must not be nil: %+v", all[0])
	}
}

func TestParseCheckpointStorageSummary(t *testing.T) {
	local := parseCheckpointStorageSummary("seatunnel:\n  engine:\n    checkpoint:\n      storage:\n        type: hdfs\n        plugin-config:\n          fs.defaultFS: file:///tmp/\n")
	if local == nil || !local.Local || local.Type != "hdfs" {
		t.Fatalf("file:// storage should be local: %+v", local)
	}
	remote := parseCheckpointStorageSummary("seatunnel:\n  engine:\n    checkpoint:\n      storage:\n        type: hdfs\n        plugin-config:\n          fs.defaultFS: hdfs://nn:8020\n")
	if remote == nil || remote.Local {
		t.Fatalf("hdfs storage should not be local: %+v", remote)
	}
	if parseCheckpointStorageSummary("seatunnel: {}\n") != nil {
		t.Fatal("missing storage should return nil")
	}
}

func TestBuildScaleInstallRequestSeparatedWorkerPorts(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeSeparated, map[uint]NodeRole{1: NodeRoleMaster, 2: NodeRoleWorker})
	view, err := env.scale.loadScaleView(context.Background(), env.cluster.ID, false)
	if err != nil {
		t.Fatalf("loadScaleView: %v", err)
	}
	plan := &ScalePlan{Target: ScaleTarget{HostID: 3, HostIP: "10.0.0.3", Role: NodeRoleWorker, InstallDir: scaleTestInstallDir, HazelcastPort: 5902}}
	req := buildScaleInstallRequest(view, plan)
	if req.ClusterPort != 5801 || req.WorkerPort != 5902 || req.NodeRole != installerapp.NodeRoleWorker || !req.DeferNodeStart {
		t.Fatalf("separated worker ports: %+v", req)
	}
	if strings.Join(req.MasterAddresses, ",") != "10.0.0.1" || strings.Join(req.WorkerAddresses, ",") != "10.0.0.2,10.0.0.3" {
		t.Fatalf("separated addresses: masters=%v workers=%v", req.MasterAddresses, req.WorkerAddresses)
	}
}

func TestScaleInCleanupKeepsHostConfigsWhenHostStillUsed(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeSeparated, map[uint]NodeRole{1: NodeRoleMaster, 2: NodeRoleWorker})
	worker := &ClusterNode{ClusterID: env.cluster.ID, HostID: 1, Role: NodeRoleWorker, InstallDir: scaleTestInstallDir, HazelcastPort: 5802, Status: NodeStatusRunning}
	if err := env.svc.repo.AddNode(ctx, worker); err != nil {
		t.Fatalf("add worker: %v", err)
	}
	op := &ClusterScaleOperation{ID: 1, ClusterID: env.cluster.ID, CreatedBy: uint(scaleTestUser)}
	plan := &ScalePlan{Kind: ScaleKindIn, Target: ScaleTarget{NodeID: worker.ID, HostID: 1, Role: NodeRoleWorker}}
	outcome := env.scale.stepCleanupRecords(ctx, op, plan, "en")
	if outcome.status != ScaleStepSucceeded || len(env.config.deleted) != 0 {
		t.Fatalf("host configs must stay while the host still runs the master: %+v deleted=%v", outcome, env.config.deleted)
	}
}
