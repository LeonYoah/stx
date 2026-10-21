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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeFleet 模拟 Agent 行为：按节点配置成功/拒绝/试运行回滚/失联。
// fakeFleet simulates Agent behaviour per node: succeed / reject / trial rollback / silent.
type fakeFleet struct {
	mu        sync.Mutex
	nodes     map[uint]*FleetNode
	behavior  map[uint]string // ok | reject | rollback | silent | transfer_fail
	transfers []string
	commands  []map[string]string
	proxyOut  map[uint]string
}

func newFakeFleet(nodes ...FleetNode) *fakeFleet {
	f := &fakeFleet{nodes: map[uint]*FleetNode{}, behavior: map[uint]string{}, proxyOut: map[uint]string{}}
	for i := range nodes {
		n := nodes[i]
		f.nodes[n.HostID] = &n
	}
	return f
}

func (f *fakeFleet) ListNodes(ctx context.Context) ([]FleetNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []FleetNode{}
	for _, n := range f.nodes {
		out = append(out, *n)
	}
	return out, nil
}

func (f *fakeFleet) Node(ctx context.Context, hostID uint) (FleetNode, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[hostID]
	if !ok {
		return FleetNode{}, false
	}
	return *n, true
}

func (f *fakeFleet) byAgent(agentID string) *FleetNode {
	for _, n := range f.nodes {
		if n.AgentID == agentID {
			return n
		}
	}
	return nil
}

func (f *fakeFleet) TransferFile(ctx context.Context, agentID, key, localPath, checksum string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.byAgent(agentID)
	if n == nil || !n.Online {
		return "", errors.New("agent not connected")
	}
	if f.behavior[n.HostID] == "transfer_fail" {
		return "", errors.New("stream broken")
	}
	f.transfers = append(f.transfers, agentID+":"+filepath.Base(localPath))
	return "/var/lib/stx-agent/packages/" + filepath.Base(localPath), nil
}

func (f *fakeFleet) SendUpgradeCommand(ctx context.Context, agentID string, params map[string]string, timeout time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, params)
	n := f.byAgent(agentID)
	if n == nil || !n.Online {
		return "", errors.New("agent not connected")
	}
	switch params["sub_command"] {
	case subCommandAgentSelfUpdate:
		switch f.behavior[n.HostID] {
		case "reject":
			return "", errors.New("checksum mismatch")
		case "silent":
			n.Online = false
			return `{"to_version":"x"}`, nil
		case "rollback":
			n.ConnectedAt = time.Now().Add(10 * time.Millisecond)
			return `{"to_version":"x"}`, nil
		default:
			n.Version = params["target_version"]
			n.ConnectedAt = time.Now().Add(10 * time.Millisecond)
			return `{"to_version":"` + params["target_version"] + `"}`, nil
		}
	case subCommandAgentSelfRollback:
		n.Version = "1.0.0"
		n.ConnectedAt = time.Now().Add(10 * time.Millisecond)
		return `{"rolling_back":true}`, nil
	case subCommandAgentRollbackProxyJars:
		return `{"restored":["stx-java-proxy-v3.jar"],"restarts":[{"status":"restarted","old_pid":20,"new_pid":21,"port":18081}]}`, nil
	case subCommandAgentUpdateProxyJars:
		if out, ok := f.proxyOut[n.HostID]; ok {
			return out, nil
		}
		return `{"target_dir":"/usr/local/lib/stx-agent/lib","updated":["a"]}`, nil
	}
	return "", errors.New("unknown sub_command")
}

func newRolloutService(t *testing.T, fleet AgentFleet) (*Service, string) {
	t.Helper()
	s := newTestService(t)
	s.SetAgentFleet(fleet)
	s.SetRolloutOptions(RolloutOptions{Concurrency: 2, CommandTimeout: time.Second, ReconnectTimeout: 300 * time.Millisecond, PollInterval: 5 * time.Millisecond})
	s.agentLibDir = filepath.Join(t.TempDir(), "lib", "agent")
	dir, err := s.versionStagingDir("1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "lib", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "lib", "agent", "stx-agent-linux-amd64"), []byte("agent-1.0.1"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "lib", "stx-java-proxy-v3.jar"), []byte("jar"), 0o644)
	return s, dir
}

func startTask(s *Service, id string) {
	s.mu.Lock()
	s.task = &TaskState{ID: id, Kind: "execute", Status: StatusUpgrading, TargetVersion: "1.0.1"}
	s.mu.Unlock()
}

func nodeByHost(nodes []AgentNodeUpgrade, id uint) AgentNodeUpgrade {
	for _, n := range nodes {
		if n.HostID == id {
			return n
		}
	}
	return AgentNodeUpgrade{}
}

func TestRolloutAgentsPerNodeStatus(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	fleet := newFakeFleet(
		FleetNode{HostID: 1, HostName: "ok", AgentID: "a1", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 2, HostName: "reject", AgentID: "a2", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 3, HostName: "trial-rollback", AgentID: "a3", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 4, HostName: "offline", AgentID: "a4", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: false},
		FleetNode{HostID: 5, HostName: "arm", AgentID: "a5", Version: "1.0.0", OS: "linux", Arch: "arm64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 6, HostName: "current", AgentID: "a6", Version: "v1.0.1", OS: "linux", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 7, HostName: "silent", AgentID: "a7", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 8, HostName: "transfer-fail", AgentID: "a8", Version: "1.0.0", OS: "linux", Arch: "amd64", Online: true, ConnectedAt: old},
	)
	fleet.behavior[2] = "reject"
	fleet.behavior[3] = "rollback"
	fleet.behavior[7] = "silent"
	fleet.behavior[8] = "transfer_fail"
	s, _ := newRolloutService(t, fleet)
	startTask(s, "t1")
	_, opts := s.fleetAndOptions()

	summary, partial, err := s.rolloutAgents(context.Background(), "t1", "1.0.1", fleet, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !partial {
		t.Fatal("expected partial result")
	}
	nodes := s.taskAgentNodes("t1")
	want := map[uint]string{1: NodeUpgradeSucceeded, 2: NodeUpgradeFailed, 3: NodeUpgradeRolledBack, 4: NodeUpgradeOffline, 5: NodeUpgradeFailed, 6: NodeUpgradeSucceeded, 7: NodeUpgradeFailed, 8: NodeUpgradeFailed}
	for id, st := range want {
		got := nodeByHost(nodes, id)
		if got.Status != st {
			t.Errorf("host %d status = %s (%s), want %s", id, got.Status, got.Message, st)
		}
	}
	if n := nodeByHost(nodes, 1); n.CurrentVersion != "1.0.1" || n.FromVersion != "1.0.0" {
		t.Errorf("host 1 versions = %+v", n)
	}
	if !strings.Contains(nodeByHost(nodes, 5).Message, "linux/arm64") {
		t.Errorf("arm node message = %s", nodeByHost(nodes, 5).Message)
	}
	if !strings.Contains(summary, "2/8 agents on target version") {
		t.Errorf("summary = %s", summary)
	}
	// 已是目标版本的节点不应被传输 / up-to-date node must not receive a transfer
	for _, tr := range fleet.transfers {
		if strings.HasPrefix(tr, "a6:") || strings.HasPrefix(tr, "a4:") {
			t.Errorf("unexpected transfer %s", tr)
		}
	}
	// 离线节点进入待补升级 / offline node queued for catch-up
	s.mu.Lock()
	pending := s.loadPendingLocked()
	s.mu.Unlock()
	if len(pending) != 1 || pending[0].HostID != 4 || pending[0].TaskID != "t1" {
		t.Fatalf("pending = %+v", pending)
	}
	// 自升级命令带校验和与目标版本 / self-update command carries checksum and target version
	for _, c := range fleet.commands {
		if c["sub_command"] == subCommandAgentSelfUpdate && (len(c["checksum"]) != 64 || c["target_version"] != "1.0.1" || c["package_path"] == "") {
			t.Errorf("bad self-update params %+v", c)
		}
	}
}

func TestRolloutProxyJarsOnlyForUpgradedAgents(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	fleet := newFakeFleet(
		FleetNode{HostID: 1, AgentID: "a1", Version: "1.0.0", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 2, AgentID: "a2", Version: "1.0.0", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 3, AgentID: "a3", Version: "1.0.0", Arch: "amd64", Online: true, ConnectedAt: old},
	)
	fleet.behavior[2] = "reject"
	fleet.proxyOut[3] = `{"skipped":true,"reason":"no stx-java-proxy installation found on this node"}`
	s, _ := newRolloutService(t, fleet)
	startTask(s, "t1")
	_, opts := s.fleetAndOptions()
	if _, _, err := s.rolloutAgents(context.Background(), "t1", "1.0.1", fleet, opts); err != nil {
		t.Fatal(err)
	}
	summary, failed := s.rolloutProxyJars(context.Background(), "t1", "1.0.1", fleet, opts)
	if failed {
		t.Fatalf("unexpected proxy failure: %s", summary)
	}
	nodes := s.taskAgentNodes("t1")
	if got := nodeByHost(nodes, 1); got.ProxyStatus != ProxyUpdated {
		t.Errorf("host1 proxy = %s %s", got.ProxyStatus, got.ProxyMessage)
	}
	if got := nodeByHost(nodes, 2); got.ProxyStatus != ProxySkipped || !strings.Contains(got.ProxyMessage, "agent not upgraded") {
		t.Errorf("host2 proxy = %s %s", got.ProxyStatus, got.ProxyMessage)
	}
	if got := nodeByHost(nodes, 3); got.ProxyStatus != ProxySkipped || !strings.Contains(got.ProxyMessage, "no stx-java-proxy") {
		t.Errorf("host3 proxy = %s %s", got.ProxyStatus, got.ProxyMessage)
	}
	for _, c := range fleet.commands {
		if c["sub_command"] == subCommandAgentUpdateProxyJars && !strings.HasSuffix(c["jar_paths"], "stx-java-proxy-v3.jar") {
			t.Errorf("jar_paths = %s", c["jar_paths"])
		}
	}
}

func TestRollbackUpgradedAgentsKeepsStepWithControlPlane(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	fleet := newFakeFleet(
		FleetNode{HostID: 1, AgentID: "a1", Version: "1.0.0", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 2, AgentID: "a2", Version: "1.0.1", Arch: "amd64", Online: true, ConnectedAt: old},
		FleetNode{HostID: 3, AgentID: "a3", Version: "1.0.0", Arch: "amd64", Online: false},
	)
	s, _ := newRolloutService(t, fleet)
	startTask(s, "t1")
	_, opts := s.fleetAndOptions()
	if _, _, err := s.rolloutAgents(context.Background(), "t1", "1.0.1", fleet, opts); err != nil {
		t.Fatal(err)
	}
	s.rollbackUpgradedAgents(context.Background(), "t1", "1.0.1")
	nodes := s.taskAgentNodes("t1")
	if got := nodeByHost(nodes, 1); got.Status != NodeUpgradeRolledBack || got.CurrentVersion != "1.0.0" {
		t.Errorf("host1 = %+v", got)
	}
	// 本来就是目标版本的节点不回滚 / node that was already on target is left alone
	if got := nodeByHost(nodes, 2); got.Status != NodeUpgradeSucceeded {
		t.Errorf("host2 = %+v", got)
	}
	rollbacks := 0
	for _, c := range fleet.commands {
		if c["sub_command"] == subCommandAgentSelfRollback {
			rollbacks++
		}
	}
	if rollbacks != 1 {
		t.Errorf("rollback commands = %d", rollbacks)
	}
	// 控制面回滚后不再补升级离线节点 / offline catch-up is cancelled with the rollback
	s.mu.Lock()
	pending := s.loadPendingLocked()
	s.mu.Unlock()
	if len(pending) != 0 {
		t.Errorf("pending should be cleared, got %+v", pending)
	}
}

func TestHandleAgentReadyCatchesUpOfflineNode(t *testing.T) {
	fleet := newFakeFleet(FleetNode{HostID: 4, HostName: "late", AgentID: "a4", Version: "1.0.0", Arch: "amd64", Online: false})
	s, _ := newRolloutService(t, fleet)
	startTask(s, "t1")
	_, opts := s.fleetAndOptions()
	if _, _, err := s.rolloutAgents(context.Background(), "t1", "1.0.1", fleet, opts); err != nil {
		t.Fatal(err)
	}
	// 任务结束并写入历史 / finish the task into history
	s.mu.Lock()
	s.recordHistoryLocked(UpgradeHistoryRecord{ID: "t1", TargetVersion: "1.0.1", Status: StatusSucceeded, AgentNodes: s.task.AgentNodes})
	s.task = nil
	s.mu.Unlock()

	// 非待补节点不处理 / non-pending host is ignored
	s.HandleAgentReady(context.Background(), 99)

	fleet.mu.Lock()
	fleet.nodes[4].Online = true
	fleet.nodes[4].ConnectedAt = time.Now().Add(-time.Minute)
	fleet.mu.Unlock()
	s.HandleAgentReady(context.Background(), 4)

	deadline := time.Now().Add(3 * time.Second)
	for {
		hist := s.ListHistory()
		got := nodeByHost(hist[0].AgentNodes, 4)
		if got.Status == NodeUpgradeSucceeded && got.ProxyStatus == ProxyUpdated {
			if got.CurrentVersion != "1.0.1" {
				t.Fatalf("current version = %s", got.CurrentVersion)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("catch-up did not finish: %+v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.mu.Lock()
	pending := s.loadPendingLocked()
	s.mu.Unlock()
	if len(pending) != 0 {
		t.Fatalf("pending not cleared: %+v", pending)
	}
}

func TestDryRunListsAgentPlanWithoutChanges(t *testing.T) {
	fleet := newFakeFleet(
		FleetNode{HostID: 1, AgentID: "a1", Version: "1.0.0", Online: true},
		FleetNode{HostID: 2, AgentID: "a2", Version: "1.0.0", Online: false},
	)
	s := newTestService(t)
	s.SetAgentFleet(fleet)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportOffline("bundle.tar.gz", strings.NewReader("x"), ""); err != nil {
		t.Fatal(err)
	}
	task, err := s.Execute(context.Background(), ExecuteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := s.GetTask(task.ID)
		if got.Status == StatusSucceeded {
			var agentStep StepState
			for _, st := range got.Steps {
				if st.Code == StepAgent {
					agentStep = st
				}
			}
			if !strings.Contains(agentStep.Message, "1/2 agent(s) would be upgraded") {
				t.Fatalf("agent step message = %s", agentStep.Message)
			}
			if len(got.AgentNodes) != 2 || got.AgentNodes[0].Status != NodeUpgradeSkipped {
				t.Fatalf("agent nodes = %+v", got.AgentNodes)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not finish: %+v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(fleet.transfers) != 0 || len(fleet.commands) != 0 {
		t.Fatal("dry-run must not contact agents")
	}
}

func TestStatusReportsAgentVersions(t *testing.T) {
	fleet := newFakeFleet(
		FleetNode{HostID: 1, HostName: "w1", Version: "0.0.1", Online: true},
		FleetNode{HostID: 2, HostName: "w2", Version: "", Online: false},
	)
	s := newTestService(t)
	s.SetAgentFleet(fleet)
	st := s.Status()
	if len(st.Agents) != 2 || st.AgentsOutdated != 2 {
		t.Fatalf("agents = %+v outdated=%d", st.Agents, st.AgentsOutdated)
	}
	fleet.nodes[1].Version = "v" + st.CurrentVersion
	fleet.nodes[2].Version = st.CurrentVersion
	st = s.Status()
	if st.AgentsOutdated != 0 {
		t.Fatalf("outdated = %d, agents = %+v", st.AgentsOutdated, st.Agents)
	}
}

func TestSummarizeAgentNodes(t *testing.T) {
	msg, partial := summarizeAgentNodes(nil)
	if partial || !strings.Contains(msg, "no agent nodes") {
		t.Fatalf("empty: %s %v", msg, partial)
	}
	msg, partial = summarizeAgentNodes([]AgentNodeUpgrade{{Status: NodeUpgradeSucceeded}, {Status: NodeUpgradeOffline}})
	if !partial || msg != "1/2 agents on target version, 1 offline" {
		t.Fatalf("got %q %v", msg, partial)
	}
}
