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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	stxversion "github.com/LeonYoah/stx/internal/version"
)

// 节点升级状态 / per-node upgrade status
const (
	NodeUpgradePending    = "pending"
	NodeUpgradeRunning    = "upgrading"
	NodeUpgradeSucceeded  = "succeeded"
	NodeUpgradeFailed     = "failed"
	NodeUpgradeRolledBack = "rolled_back"
	NodeUpgradeOffline    = "offline"
	NodeUpgradeSkipped    = "skipped"

	ProxyUpdated = "updated"
	ProxySkipped = "skipped"
	ProxyFailed  = "failed"
	// ProxyRolledBack 表示 jar 已随控制面回滚恢复为备份。/ ProxyRolledBack means jars were restored with the control plane rollback.
	ProxyRolledBack = "rolled_back"

	ProxyRestartRestarted  = "restarted"
	ProxyRestartNotRunning = "not_running"
	ProxyRestartRolledBack = "rolled_back"
	ProxyRestartFailed     = "failed"
	ProxyRestartSkipped    = "skipped"

	subCommandAgentSelfUpdate      = "agent_self_update"
	subCommandAgentSelfRollback    = "agent_self_rollback"
	subCommandAgentUpdateProxyJars = "agent_update_proxy_jars"
	// subCommandAgentRollbackProxyJars 用 .bak 恢复 proxy jar 并重启运行中的 proxy（仅 STX 自身进程）。
	// subCommandAgentRollbackProxyJars restores proxy jars from .bak and restarts running proxies (STX processes only).
	subCommandAgentRollbackProxyJars = "agent_rollback_proxy_jars"

	agentPendingFile = "agent-pending.json"
)

// FleetNode 是一个已安装 Agent 的节点。/ FleetNode is a host with an installed Agent.
type FleetNode struct {
	HostID      uint
	HostName    string
	IPAddress   string
	AgentID     string
	Version     string
	OS          string
	Arch        string
	Online      bool
	ConnectedAt time.Time
	// InstallDirs 是该节点上 SeaTunnel 集群节点的安装目录，用于定位运行中的 stx-java-proxy 状态目录。
	// InstallDirs are the SeaTunnel install dirs of cluster nodes on this host, used to find running stx-java-proxy state.
	InstallDirs []string
}

// AgentFleet 抽象 CP 对已连接 Agent 的访问（由 router 适配 agent.Manager 与 host.Service）。
// AgentFleet abstracts the control plane's access to Agents (adapted from agent.Manager and host.Service by the router).
type AgentFleet interface {
	// ListNodes 返回所有已安装 Agent 的节点（含离线）。/ ListNodes returns every host with an installed Agent, including offline ones.
	ListNodes(ctx context.Context) ([]FleetNode, error)
	// Node 返回单个节点的最新连接信息。/ Node returns the latest connection info of one host.
	Node(ctx context.Context, hostID uint) (FleetNode, bool)
	// TransferFile 让 Agent 通过现有 DownloadPackage 流拉取文件，返回 Agent 上的路径。
	// TransferFile has the Agent pull a file over the existing DownloadPackage stream and returns its path on the Agent.
	TransferFile(ctx context.Context, agentID, key, localPath, checksum string) (string, error)
	// SendUpgradeCommand 发送 UPGRADE 子命令并返回成功输出。/ SendUpgradeCommand sends an UPGRADE sub-command and returns its output.
	SendUpgradeCommand(ctx context.Context, agentID string, params map[string]string, timeout time.Duration) (string, error)
}

// AgentNodeUpgrade 是单节点 Agent/Proxy 升级状态。/ AgentNodeUpgrade is the Agent/Proxy upgrade status of one node.
type AgentNodeUpgrade struct {
	HostID         uint   `json:"host_id"`
	HostName       string `json:"host_name"`
	IPAddress      string `json:"ip_address,omitempty"`
	AgentID        string `json:"agent_id,omitempty"`
	FromVersion    string `json:"from_version,omitempty"`
	TargetVersion  string `json:"target_version"`
	CurrentVersion string `json:"current_version,omitempty"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	ProxyStatus    string `json:"proxy_status,omitempty"`
	ProxyMessage   string `json:"proxy_message,omitempty"`
	// ProxyRestart 是该节点运行中 stx-java-proxy 的重启结果：restarted | not_running | rolled_back | failed | skipped。
	// ProxyRestart is the restart result of the node's running stx-java-proxy.
	ProxyRestart        string     `json:"proxy_restart,omitempty"`
	ProxyRestartMessage string     `json:"proxy_restart_message,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

// AgentVersionInfo 是状态页展示的节点 Agent 版本。/ AgentVersionInfo is a node's Agent version shown on the status page.
type AgentVersionInfo struct {
	HostID    uint   `json:"host_id"`
	HostName  string `json:"host_name"`
	IPAddress string `json:"ip_address,omitempty"`
	Version   string `json:"version"`
	Online    bool   `json:"online"`
	UpToDate  bool   `json:"up_to_date"`
}

// RolloutOptions 控制 Agent 下发节奏。/ RolloutOptions tunes the Agent rollout.
type RolloutOptions struct {
	Concurrency      int
	CommandTimeout   time.Duration
	ReconnectTimeout time.Duration
	PollInterval     time.Duration
}

func defaultRolloutOptions() RolloutOptions {
	return RolloutOptions{
		Concurrency:      4,
		CommandTimeout:   2 * time.Minute,
		ReconnectTimeout: 150 * time.Second,
		PollInterval:     2 * time.Second,
	}
}

type agentPendingEntry struct {
	HostID        uint   `json:"host_id"`
	TargetVersion string `json:"target_version"`
	TaskID        string `json:"task_id"`
}

// SetAgentFleet 注入 Agent 访问能力；未注入时 Agent 步骤只能更新控制面本地 lib 并如实报告。
// SetAgentFleet injects Agent access; without it the Agent step can only refresh the control plane's local lib and says so.
func (s *Service) SetAgentFleet(f AgentFleet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fleet = f
}

// SetRolloutOptions 覆盖下发参数（测试用）。/ SetRolloutOptions overrides rollout tuning (tests).
func (s *Service) SetRolloutOptions(o RolloutOptions) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rollout = o
}

func (s *Service) fleetAndOptions() (AgentFleet, RolloutOptions) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.rollout
	d := defaultRolloutOptions()
	if o.Concurrency <= 0 {
		o.Concurrency = d.Concurrency
	}
	if o.CommandTimeout <= 0 {
		o.CommandTimeout = d.CommandTimeout
	}
	if o.ReconnectTimeout <= 0 {
		o.ReconnectTimeout = d.ReconnectTimeout
	}
	if o.PollInterval <= 0 {
		o.PollInterval = d.PollInterval
	}
	return s.fleet, o
}

// agentVersions 汇总各节点 Agent 版本（不持锁调用）。/ agentVersions summarises node Agent versions (call without the lock).
func (s *Service) agentVersions(ctx context.Context, cpVersion string) ([]AgentVersionInfo, int) {
	fleet, _ := s.fleetAndOptions()
	if fleet == nil {
		return nil, 0
	}
	nodes, err := fleet.ListNodes(ctx)
	if err != nil {
		return nil, 0
	}
	out := make([]AgentVersionInfo, 0, len(nodes))
	outdated := 0
	for _, n := range nodes {
		up := sameVersion(n.Version, cpVersion)
		if !up {
			outdated++
		}
		out = append(out, AgentVersionInfo{HostID: n.HostID, HostName: n.HostName, IPAddress: n.IPAddress, Version: n.Version, Online: n.Online, UpToDate: up})
	}
	return out, outdated
}

func sameVersion(a, b string) bool {
	a, b = stxversion.Normalize(a), stxversion.Normalize(b)
	return a != "" && a == b
}

// agentBinaryFor 选择与节点 OS/架构匹配的 Agent 二进制：优先预置目录，其次控制面 lib/agent。
// agentBinaryFor picks the Agent binary matching the node OS/arch: the staged package first, then the control plane's lib/agent.
func (s *Service) agentBinaryFor(target, goos, goarch string) (string, error) {
	if goos == "" {
		goos = "linux"
	}
	if goarch == "" {
		goarch = runtimeGOARCH()
	}
	name := fmt.Sprintf("stx-agent-%s-%s", strings.ToLower(goos), strings.ToLower(goarch))
	candidates := []string{}
	if dir, err := s.versionStagingDir(target); err == nil {
		candidates = append(candidates, filepath.Join(dir, "lib", "agent", name), filepath.Join(dir, name))
	}
	candidates = append(candidates, filepath.Join(s.cpAgentLibDir(), name))
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
			return c, nil
		}
	}
	return "", fmt.Errorf("upgrade package has no agent binary for %s/%s (%s) / 升级包中没有 %s/%s 的 Agent 二进制", goos, goarch, name, goos, goarch)
}

func (s *Service) cpAgentLibDir() string {
	if s.agentLibDir != "" {
		return s.agentLibDir
	}
	return filepath.Join(".", "lib", "agent")
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// updateAgentNode 更新活动任务或历史记录中的节点状态。/ updateAgentNode updates a node in the active task or in history.
func (s *Service) updateAgentNode(taskID string, hostID uint, fn func(n *AgentNodeUpgrade)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if s.task != nil && s.task.ID == taskID {
		for i := range s.task.AgentNodes {
			if s.task.AgentNodes[i].HostID == hostID {
				fn(&s.task.AgentNodes[i])
				s.task.AgentNodes[i].UpdatedAt = &now
			}
		}
	}
	// 已结束任务：回写历史 / finished task: write back into history
	list := s.loadHistoryLocked()
	changed := false
	for i := range list {
		if list[i].ID != taskID {
			continue
		}
		for j := range list[i].AgentNodes {
			if list[i].AgentNodes[j].HostID == hostID {
				fn(&list[i].AgentNodes[j])
				list[i].AgentNodes[j].UpdatedAt = &now
				changed = true
			}
		}
	}
	if changed {
		s.saveHistoryLocked(list)
	}
}

func (s *Service) taskAgentNodes(taskID string) []AgentNodeUpgrade {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.ID != taskID {
		return nil
	}
	return append([]AgentNodeUpgrade(nil), s.task.AgentNodes...)
}

// rolloutAgents 把新 Agent 下发到所有在线节点：传输 → 节点校验/备份/替换/重启 → 等待以新版本重连。
// 离线节点记入待补升级列表，重连时自动补升级。返回步骤摘要以及是否存在未成功节点。
// rolloutAgents pushes the new Agent to every online node: transfer → node verifies/backs up/replaces/restarts →
// wait for it to reconnect on the new version. Offline nodes are queued and upgraded when they reconnect.
// Returns the step summary and whether any node did not succeed.
func (s *Service) rolloutAgents(ctx context.Context, taskID, target string, fleet AgentFleet, opts RolloutOptions) (string, bool, error) {
	nodes, err := fleet.ListNodes(ctx)
	if err != nil {
		return "", false, fmt.Errorf("%w: list agent nodes: %v", ErrStepFailAgent, err)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].HostID < nodes[j].HostID })
	states := make([]AgentNodeUpgrade, 0, len(nodes))
	var todo []FleetNode
	var offline []uint
	now := time.Now().UTC()
	for _, n := range nodes {
		st := AgentNodeUpgrade{HostID: n.HostID, HostName: n.HostName, IPAddress: n.IPAddress, AgentID: n.AgentID,
			FromVersion: n.Version, CurrentVersion: n.Version, TargetVersion: target, UpdatedAt: &now}
		switch {
		case !n.Online:
			st.Status = NodeUpgradeOffline
			st.Message = "agent offline; it will be upgraded automatically when it reconnects / Agent 离线，重连后自动补升级"
			offline = append(offline, n.HostID)
		case sameVersion(n.Version, target):
			st.Status = NodeUpgradeSucceeded
			st.Message = "already on target version / 已是目标版本"
		default:
			st.Status = NodeUpgradePending
			todo = append(todo, n)
		}
		states = append(states, st)
	}
	s.mu.Lock()
	if s.task != nil && s.task.ID == taskID {
		s.task.AgentNodes = states
	}
	s.mu.Unlock()
	s.addPendingAgents(taskID, target, offline)

	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup
	for _, n := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func(n FleetNode) {
			defer wg.Done()
			defer func() { <-sem }()
			s.upgradeAgentNode(ctx, taskID, target, n, fleet, opts)
		}(n)
	}
	wg.Wait()
	summary, partial := summarizeAgentNodes(s.taskAgentNodes(taskID))
	return summary, partial, nil
}

func summarizeAgentNodes(nodes []AgentNodeUpgrade) (string, bool) {
	counts := map[string]int{}
	for _, n := range nodes {
		counts[n.Status]++
	}
	total := len(nodes)
	if total == 0 {
		return "no agent nodes registered / 未发现已安装 Agent 的节点", false
	}
	partial := counts[NodeUpgradeSucceeded] != total
	msg := fmt.Sprintf("%d/%d agents on target version", counts[NodeUpgradeSucceeded], total)
	for _, k := range []string{NodeUpgradeFailed, NodeUpgradeRolledBack, NodeUpgradeOffline} {
		if counts[k] > 0 {
			msg += fmt.Sprintf(", %d %s", counts[k], k)
		}
	}
	return msg, partial
}

// upgradeAgentNode 升级单个节点，结果写入任务/历史。/ upgradeAgentNode upgrades one node and records the result.
func (s *Service) upgradeAgentNode(ctx context.Context, taskID, target string, n FleetNode, fleet AgentFleet, opts RolloutOptions) string {
	set := func(status, msg string) {
		s.updateAgentNode(taskID, n.HostID, func(st *AgentNodeUpgrade) {
			st.Status, st.Message = status, msg
		})
	}
	set(NodeUpgradeRunning, "transferring agent binary / 正在传输 Agent 二进制")
	bin, err := s.agentBinaryFor(target, n.OS, n.Arch)
	if err != nil {
		set(NodeUpgradeFailed, err.Error())
		return NodeUpgradeFailed
	}
	sum, err := sha256File(bin)
	if err != nil {
		set(NodeUpgradeFailed, "hash agent binary: "+err.Error())
		return NodeUpgradeFailed
	}
	remote, err := fleet.TransferFile(ctx, n.AgentID, fmt.Sprintf("stx-agent-%s-%s", stxversion.Normalize(target), sum[:12]), bin, sum)
	if err != nil {
		set(NodeUpgradeFailed, "transfer failed, node unchanged: "+err.Error()+" / 传输失败，节点未变更")
		return NodeUpgradeFailed
	}
	set(NodeUpgradeRunning, "replacing and restarting agent / 正在替换并重启 Agent")
	sentAt := time.Now()
	out, err := fleet.SendUpgradeCommand(ctx, n.AgentID, map[string]string{
		"sub_command":    subCommandAgentSelfUpdate,
		"package_path":   remote,
		"checksum":       sum,
		"target_version": stxversion.Normalize(target),
	}, opts.CommandTimeout)
	if err != nil {
		set(NodeUpgradeFailed, "agent rejected the new binary, node unchanged: "+err.Error()+" / Agent 拒绝新二进制，节点未变更")
		return NodeUpgradeFailed
	}
	if strings.Contains(out, `"already_current":true`) {
		s.updateAgentNode(taskID, n.HostID, func(st *AgentNodeUpgrade) {
			st.Status, st.Message, st.CurrentVersion = NodeUpgradeSucceeded, "already on target version / 已是目标版本", target
		})
		return NodeUpgradeSucceeded
	}
	status, cur, msg := waitAgentReconnect(ctx, fleet, n.HostID, target, sentAt, opts)
	s.updateAgentNode(taskID, n.HostID, func(st *AgentNodeUpgrade) {
		st.Status, st.Message = status, msg
		if cur != "" {
			st.CurrentVersion = cur
		}
	})
	return status
}

// waitAgentReconnect 等待节点重启后重连：新版本 → succeeded；重连但仍是旧版本 → rolled_back（试运行自动回滚）；超时 → failed。
// waitAgentReconnect waits for the node to reconnect after restarting: target version → succeeded; reconnected on the
// old version → rolled_back (trial self-rollback); timeout → failed.
func waitAgentReconnect(ctx context.Context, fleet AgentFleet, hostID uint, target string, sentAt time.Time, opts RolloutOptions) (string, string, string) {
	deadline := time.Now().Add(opts.ReconnectTimeout)
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()
	for {
		if n, ok := fleet.Node(ctx, hostID); ok && n.Online && n.ConnectedAt.After(sentAt) {
			if sameVersion(n.Version, target) {
				return NodeUpgradeSucceeded, n.Version, "agent restarted on the new version / Agent 已以新版本重连"
			}
			// 新进程在注册前回滚会以旧版本重连；给新版本一点时间避免误判中间态。
			// A trial rollback reconnects on the old version; only conclude after a grace period to avoid racing.
			if time.Since(n.ConnectedAt) > 3*opts.PollInterval {
				return NodeUpgradeRolledBack, n.Version, fmt.Sprintf("new agent failed its trial run and restored version %s / 新 Agent 试运行失败，已自动恢复 %s", n.Version, n.Version)
			}
		}
		if time.Now().After(deadline) {
			cur := ""
			if n, ok := fleet.Node(ctx, hostID); ok {
				cur = n.Version
			}
			return NodeUpgradeFailed, cur, fmt.Sprintf("agent did not reconnect on %s within %s; the node restores its backup automatically if the new binary cannot register / Agent 未在 %s 内以新版本重连，新二进制无法注册时节点会自动恢复备份", target, opts.ReconnectTimeout, opts.ReconnectTimeout)
		}
		select {
		case <-ctx.Done():
			return NodeUpgradeFailed, "", "cancelled while waiting for agent / 等待 Agent 时被取消"
		case <-ticker.C:
		}
	}
}

// rollbackUpgradedAgents 在控制面回滚时把已升级的 Agent 一并恢复到备份版本，保持与控制面一致。
// rollbackUpgradedAgents restores upgraded Agents to their backups when the control plane rolls back, keeping them in step.
func (s *Service) rollbackUpgradedAgents(ctx context.Context, taskID, target string) {
	fleet, opts := s.fleetAndOptions()
	if fleet == nil {
		return
	}
	var wg sync.WaitGroup
	for _, st := range s.taskAgentNodes(taskID) {
		if st.Status != NodeUpgradeSucceeded || sameVersion(st.FromVersion, target) || st.FromVersion == "" {
			continue
		}
		wg.Add(1)
		go func(st AgentNodeUpgrade) {
			defer wg.Done()
			// 控制面可能刚重启，先等 Agent 重新连上。/ the control plane may have just restarted; wait for the Agent first.
			if fn, ok := waitNodeOnline(ctx, fleet, st.HostID, opts); ok {
				st.AgentID = fn.AgentID
			}
			// 先由新 Agent 恢复 proxy jar 并重启运行中的 proxy（旧 Agent 不认识该子命令）。
			// Restore proxy jars first while the new Agent still runs (the old Agent does not know this sub-command).
			if st.ProxyStatus == ProxyUpdated {
				s.rollbackProxyJarsOnNode(ctx, taskID, st, fleet, opts)
			}
			sentAt := time.Now()
			if _, err := fleet.SendUpgradeCommand(ctx, st.AgentID, map[string]string{"sub_command": subCommandAgentSelfRollback}, opts.CommandTimeout); err != nil {
				s.updateAgentNode(taskID, st.HostID, func(n *AgentNodeUpgrade) {
					n.Message = "control plane rolled back but agent rollback failed: " + err.Error() + " / 控制面已回滚，Agent 回滚失败"
					n.Status = NodeUpgradeFailed
				})
				return
			}
			status, cur, _ := waitAgentReconnect(ctx, fleet, st.HostID, st.FromVersion, sentAt, opts)
			s.updateAgentNode(taskID, st.HostID, func(n *AgentNodeUpgrade) {
				if cur != "" {
					n.CurrentVersion = cur
				}
				if status == NodeUpgradeSucceeded {
					n.Status = NodeUpgradeRolledBack
					n.Message = "rolled back together with the control plane / 已随控制面回滚"
				} else {
					n.Status = NodeUpgradeFailed
					n.Message = "control plane rolled back but agent did not come back on " + st.FromVersion + " / 控制面已回滚，Agent 未恢复到旧版本"
				}
			})
		}(st)
	}
	wg.Wait()
	s.removePendingAgentsForTask(taskID)
}

// waitNodeOnline 等待节点在线（最多 ReconnectTimeout）。/ waitNodeOnline waits until the node is online (up to ReconnectTimeout).
func waitNodeOnline(ctx context.Context, fleet AgentFleet, hostID uint, opts RolloutOptions) (FleetNode, bool) {
	deadline := time.Now().Add(opts.ReconnectTimeout)
	for {
		if n, ok := fleet.Node(ctx, hostID); ok && n.Online {
			return n, true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return FleetNode{}, false
		}
		time.Sleep(opts.PollInterval)
	}
}

// rollbackProxyJarsOnNode 恢复节点上的 proxy jar 备份并重启运行中的 proxy，结果写入节点状态。
// rollbackProxyJarsOnNode restores the node's proxy jar backups, restarts running proxies and records the result.
func (s *Service) rollbackProxyJarsOnNode(ctx context.Context, taskID string, st AgentNodeUpgrade, fleet AgentFleet, opts RolloutOptions) {
	params := map[string]string{"sub_command": subCommandAgentRollbackProxyJars}
	if fn, ok := fleet.Node(ctx, st.HostID); ok && len(fn.InstallDirs) > 0 {
		params["install_dirs"] = strings.Join(fn.InstallDirs, ",")
	}
	out, err := fleet.SendUpgradeCommand(ctx, st.AgentID, params, opts.CommandTimeout)
	s.updateAgentNode(taskID, st.HostID, func(n *AgentNodeUpgrade) {
		if err != nil {
			n.ProxyStatus = ProxyFailed
			n.ProxyMessage = "proxy jar rollback failed: " + err.Error() + " / proxy jar 回滚失败"
			return
		}
		n.ProxyStatus = ProxyRolledBack
		n.ProxyMessage = "proxy jars restored from backup / proxy jar 已从备份恢复"
		n.ProxyRestart, n.ProxyRestartMessage = summarizeProxyRestarts(parseProxyRestarts(out))
	})
}

// proxyRestartEntry 对应 Agent 返回的单个 proxy 重启结果。/ proxyRestartEntry mirrors one proxy restart result from the Agent.
type proxyRestartEntry struct {
	StateDir string `json:"state_dir"`
	Status   string `json:"status"`
	OldPID   int    `json:"old_pid"`
	NewPID   int    `json:"new_pid"`
	Port     int    `json:"port"`
	Message  string `json:"message"`
}

func parseProxyRestarts(out string) []proxyRestartEntry {
	var res struct {
		Restarts []proxyRestartEntry `json:"restarts"`
	}
	_ = json.Unmarshal([]byte(out), &res)
	return res.Restarts
}

// summarizeProxyRestarts 把节点上多个 proxy 的重启结果汇总为一个状态（最差优先）。
// summarizeProxyRestarts folds several proxy restart results of a node into one status (worst wins).
func summarizeProxyRestarts(list []proxyRestartEntry) (string, string) {
	if len(list) == 0 {
		return ProxyRestartNotRunning, "no running stx-java-proxy on this node / 该节点没有运行中的 stx-java-proxy"
	}
	rank := map[string]int{ProxyRestartFailed: 4, ProxyRestartRolledBack: 3, ProxyRestartRestarted: 2, ProxyRestartSkipped: 1, ProxyRestartNotRunning: 0}
	worst := ""
	parts := make([]string, 0, len(list))
	for _, r := range list {
		if worst == "" || rank[r.Status] > rank[worst] {
			worst = r.Status
		}
		part := r.Status
		if r.Port > 0 {
			part = fmt.Sprintf("port %d: %s", r.Port, r.Status)
		}
		if r.OldPID > 0 || r.NewPID > 0 {
			part += fmt.Sprintf(" (pid %d → %d)", r.OldPID, r.NewPID)
		}
		if r.Message != "" {
			part += " " + r.Message
		}
		parts = append(parts, part)
	}
	return worst, strings.Join(parts, "; ")
}

// rolloutProxyJars 把 stx-java-proxy jar 下发到已升级的节点（替换并备份），并重启该节点上运行中的 proxy；
// 重启失败时 Agent 会恢复备份 jar 再拉起。只涉及 STX 自身的 proxy 进程，不触碰 SeaTunnel。
// rolloutProxyJars pushes stx-java-proxy jars to upgraded nodes (replace with backup) and restarts the node's running
// proxies; if a restart fails the Agent restores the backup jars and starts it again. Only STX's own proxy processes
// are touched, never SeaTunnel.
func (s *Service) rolloutProxyJars(ctx context.Context, taskID, target string, fleet AgentFleet, opts RolloutOptions) (string, bool) {
	dir, err := s.versionStagingDir(target)
	if err != nil {
		return err.Error(), true
	}
	jars, _ := filepath.Glob(filepath.Join(dir, "lib", "stx-java-proxy-*.jar"))
	if len(jars) == 0 {
		jars, _ = filepath.Glob(filepath.Join(dir, "stx-java-proxy-*.jar"))
	}
	nodes := s.taskAgentNodes(taskID)
	if len(jars) == 0 {
		for _, n := range nodes {
			s.updateAgentNode(taskID, n.HostID, func(st *AgentNodeUpgrade) {
				st.ProxyStatus, st.ProxyMessage = ProxySkipped, "no proxy jar in upgrade package / 升级包中没有 proxy jar"
			})
		}
		return "no proxy jar in upgrade package", false
	}
	updated, failed, skipped, restarted, restartIssues := 0, 0, 0, 0, 0
	for _, n := range nodes {
		if n.Status != NodeUpgradeSucceeded {
			skipped++
			s.updateAgentNode(taskID, n.HostID, func(st *AgentNodeUpgrade) {
				st.ProxyStatus, st.ProxyMessage = ProxySkipped, "agent not upgraded / Agent 未升级"
			})
			continue
		}
		r := s.pushProxyJars(ctx, n, target, jars, fleet, opts)
		switch r.status {
		case ProxyUpdated:
			updated++
		case ProxyFailed:
			failed++
		default:
			skipped++
		}
		switch r.restart {
		case ProxyRestartRestarted:
			restarted++
		case ProxyRestartFailed, ProxyRestartRolledBack:
			restartIssues++
		}
		s.updateAgentNode(taskID, n.HostID, func(st *AgentNodeUpgrade) { r.apply(st) })
	}
	return fmt.Sprintf("proxy jars: %d node(s) updated, %d skipped, %d failed; running proxies: %d node(s) restarted, %d with restart issues",
		updated, skipped, failed, restarted, restartIssues), failed > 0 || restartIssues > 0
}

type proxyPushResult struct {
	status, msg, restart, restartMsg string
}

func (r proxyPushResult) apply(st *AgentNodeUpgrade) {
	st.ProxyStatus, st.ProxyMessage = r.status, r.msg
	st.ProxyRestart, st.ProxyRestartMessage = r.restart, r.restartMsg
}

func (s *Service) pushProxyJars(ctx context.Context, n AgentNodeUpgrade, target string, jars []string, fleet AgentFleet, opts RolloutOptions) proxyPushResult {
	fail := func(msg string) proxyPushResult {
		return proxyPushResult{status: ProxyFailed, msg: msg, restart: ProxyRestartSkipped, restartMsg: "jars not updated / jar 未更新"}
	}
	remotes := make([]string, 0, len(jars))
	for _, jar := range jars {
		sum, err := sha256File(jar)
		if err != nil {
			return fail(err.Error())
		}
		remote, err := fleet.TransferFile(ctx, n.AgentID, fmt.Sprintf("%s-%s-%s", strings.TrimSuffix(filepath.Base(jar), ".jar"), stxversion.Normalize(target), sum[:12]), jar, sum)
		if err != nil {
			return fail("transfer failed: " + err.Error())
		}
		// Agent 按文件名识别 jar；拉取结果沿用源文件名。/ the Agent identifies jars by file name, which the pull keeps.
		remotes = append(remotes, remote)
	}
	params := map[string]string{
		"sub_command": subCommandAgentUpdateProxyJars,
		"jar_paths":   strings.Join(remotes, ","),
		// 让 Agent 重启该节点上运行中的 stx-java-proxy（失败时恢复备份 jar）。
		// Ask the Agent to restart running stx-java-proxy processes on the node (restoring backup jars on failure).
		"restart_running": "true",
	}
	if fn, ok := fleet.Node(ctx, n.HostID); ok && len(fn.InstallDirs) > 0 {
		params["install_dirs"] = strings.Join(fn.InstallDirs, ",")
	}
	out, err := fleet.SendUpgradeCommand(ctx, n.AgentID, params, opts.CommandTimeout)
	if err != nil {
		return fail(err.Error())
	}
	var res struct {
		TargetDir string   `json:"target_dir"`
		Updated   []string `json:"updated"`
		Skipped   bool     `json:"skipped"`
		Reason    string   `json:"reason"`
	}
	_ = json.Unmarshal([]byte(out), &res)
	restart, restartMsg := summarizeProxyRestarts(parseProxyRestarts(out))
	if res.Skipped {
		return proxyPushResult{status: ProxySkipped, msg: res.Reason, restart: restart, restartMsg: restartMsg}
	}
	msg := fmt.Sprintf("%d jar(s) updated in %s (backups kept as .bak) / 已更新 %d 个 jar（备份为 .bak）", len(res.Updated), res.TargetDir, len(res.Updated))
	return proxyPushResult{status: ProxyUpdated, msg: msg, restart: restart, restartMsg: restartMsg}
}

// ---- 离线节点补升级 / catch-up for offline nodes ----

func (s *Service) pendingPath() string { return filepath.Join(s.stagingDir, agentPendingFile) }

func (s *Service) loadPendingLocked() []agentPendingEntry {
	data, err := os.ReadFile(s.pendingPath())
	if err != nil {
		return nil
	}
	var list []agentPendingEntry
	_ = json.Unmarshal(data, &list)
	return list
}

func (s *Service) savePendingLocked(list []agentPendingEntry) {
	if len(list) == 0 {
		_ = os.Remove(s.pendingPath())
		return
	}
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(s.pendingPath(), data, 0o644)
	}
}

func (s *Service) addPendingAgents(taskID, target string, hostIDs []uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.loadPendingLocked()
	// 新任务取代旧的待补升级 / a new task supersedes older pending entries
	kept := list[:0]
	for _, e := range list {
		if e.TaskID == taskID {
			kept = append(kept, e)
		}
	}
	for _, id := range hostIDs {
		kept = append(kept, agentPendingEntry{HostID: id, TargetVersion: target, TaskID: taskID})
	}
	s.savePendingLocked(kept)
}

func (s *Service) removePendingAgent(hostID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.loadPendingLocked()
	kept := list[:0]
	for _, e := range list {
		if e.HostID != hostID {
			kept = append(kept, e)
		}
	}
	s.savePendingLocked(kept)
}

func (s *Service) removePendingAgentsForTask(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.loadPendingLocked()
	kept := list[:0]
	for _, e := range list {
		if e.TaskID != taskID {
			kept = append(kept, e)
		}
	}
	s.savePendingLocked(kept)
}

// HandleAgentReady 在 Agent 重连就绪后调用：若该节点在待补升级列表中，则在后台补升级（含 proxy jar）。
// HandleAgentReady runs after an Agent reconnects: if the node is queued for catch-up, upgrade it in the background (including proxy jars).
func (s *Service) HandleAgentReady(ctx context.Context, hostID uint) {
	fleet, opts := s.fleetAndOptions()
	if fleet == nil {
		return
	}
	s.mu.Lock()
	var entry *agentPendingEntry
	for _, e := range s.loadPendingLocked() {
		if e.HostID == hostID {
			e := e
			entry = &e
			break
		}
	}
	if entry == nil || s.catchingUp[hostID] {
		s.mu.Unlock()
		return
	}
	if s.catchingUp == nil {
		s.catchingUp = map[uint]bool{}
	}
	s.catchingUp[hostID] = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.catchingUp, hostID)
			s.mu.Unlock()
		}()
		runCtx := context.Background()
		_ = ctx
		n, ok := fleet.Node(runCtx, hostID)
		if !ok || !n.Online {
			return
		}
		s.removePendingAgent(hostID)
		if sameVersion(n.Version, entry.TargetVersion) {
			s.updateAgentNode(entry.TaskID, hostID, func(st *AgentNodeUpgrade) {
				st.Status, st.CurrentVersion, st.Message = NodeUpgradeSucceeded, n.Version, "already on target version after reconnect / 重连时已是目标版本"
			})
			return
		}
		s.updateAgentNode(entry.TaskID, hostID, func(st *AgentNodeUpgrade) {
			st.AgentID, st.FromVersion, st.CurrentVersion = n.AgentID, n.Version, n.Version
		})
		if s.upgradeAgentNode(runCtx, entry.TaskID, entry.TargetVersion, n, fleet, opts) != NodeUpgradeSucceeded {
			return
		}
		dir, err := s.versionStagingDir(entry.TargetVersion)
		if err != nil {
			return
		}
		jars, _ := filepath.Glob(filepath.Join(dir, "lib", "stx-java-proxy-*.jar"))
		if len(jars) == 0 {
			return
		}
		st := AgentNodeUpgrade{HostID: hostID, AgentID: n.AgentID}
		r := s.pushProxyJars(runCtx, st, entry.TargetVersion, jars, fleet, opts)
		s.updateAgentNode(entry.TaskID, hostID, func(st *AgentNodeUpgrade) { r.apply(st) })
	}()
}

// planAgentsDryRun 在演练模式下列出各节点将会如何处理，不做任何变更。
// planAgentsDryRun lists what would happen to each node in dry-run mode without changing anything.
func (s *Service) planAgentsDryRun(ctx context.Context, taskID, target string) string {
	fleet, _ := s.fleetAndOptions()
	if fleet == nil {
		return "dry-run: nothing applied / 演练模式：未实际应用"
	}
	nodes, err := fleet.ListNodes(ctx)
	if err != nil {
		return "dry-run: nothing applied / 演练模式：未实际应用"
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].HostID < nodes[j].HostID })
	now := time.Now().UTC()
	states := make([]AgentNodeUpgrade, 0, len(nodes))
	would := 0
	for _, n := range nodes {
		st := AgentNodeUpgrade{HostID: n.HostID, HostName: n.HostName, IPAddress: n.IPAddress, AgentID: n.AgentID,
			FromVersion: n.Version, CurrentVersion: n.Version, TargetVersion: target, Status: NodeUpgradeSkipped, UpdatedAt: &now}
		switch {
		case sameVersion(n.Version, target):
			st.Message = "dry-run: already on target version / 演练：已是目标版本"
		case !n.Online:
			st.Message = "dry-run: offline, would be upgraded on reconnect / 演练：离线，重连后才会升级"
		default:
			would++
			st.Message = "dry-run: would be upgraded / 演练：将会升级"
		}
		states = append(states, st)
	}
	s.mu.Lock()
	if s.task != nil && s.task.ID == taskID {
		s.task.AgentNodes = states
	}
	s.mu.Unlock()
	return fmt.Sprintf("dry-run: nothing applied; %d/%d agent(s) would be upgraded / 演练模式：未实际应用，%d/%d 个 Agent 将会升级", would, len(nodes), would, len(nodes))
}
