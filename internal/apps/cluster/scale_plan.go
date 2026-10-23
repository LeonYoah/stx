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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	"github.com/LeonYoah/stx/internal/clustermembers"
	"github.com/LeonYoah/stx/internal/processidentity"
	"gopkg.in/yaml.v3"
)

// ScaleNodePorts 是新节点的端口；为 0 时依次取发现结果、集群端口配置、同角色节点与角色默认值。
// ScaleNodePorts are the new node's ports; zero values fall back to discovery, cluster port config, a same-role
// node and the role default, in that order.
type ScaleNodePorts struct {
	HazelcastPort int `json:"hazelcast_port"`
	APIPort       int `json:"api_port"`
	WorkerPort    int `json:"worker_port"`
}

// ScaleOutRequest 是扩容计划请求。
// ScaleOutRequest is a scale-out plan request.
type ScaleOutRequest struct {
	HostID uint `json:"host_id" binding:"required"`
	// Mode 为 install / register；为空时使用自动识别结果。
	// Mode is install / register; empty uses the detected mode.
	Mode ScaleMode `json:"mode"`
	// Role 仅分离模式需要（master / worker）；混合模式固定为 master/worker。
	// Role is only required in separated mode (master / worker); hybrid always uses master/worker.
	Role       NodeRole       `json:"role"`
	InstallDir string         `json:"install_dir"`
	Ports      ScaleNodePorts `json:"ports"`
}

// ScaleOutSubmitRequest 是扩容提交请求：计划参数 + 计划指纹 + 回环重启确认。
// ScaleOutSubmitRequest is a scale-out submission: plan parameters + plan fingerprint + loopback restart ack.
type ScaleOutSubmitRequest struct {
	ScaleOutRequest
	PlanFingerprint    string `json:"plan_fingerprint" binding:"required"`
	LoopbackRestartAck bool   `json:"loopback_restart_ack"`
	// RollingRestart 入群后滚动重启现有节点（可选，见 ScalePlan.RollingRestart）。
	// RollingRestart restarts existing nodes one by one after the join (optional, see ScalePlan.RollingRestart).
	RollingRestart bool `json:"rolling_restart"`
}

// ScaleInSubmitRequest 是缩容提交请求：计划指纹 + 运行中作业风险确认。
// ScaleInSubmitRequest is a scale-in submission: plan fingerprint + running jobs risk ack.
type ScaleInSubmitRequest struct {
	PlanFingerprint string `json:"plan_fingerprint" binding:"required"`
	JobsRiskAck     bool   `json:"jobs_risk_ack"`
}

// ScaleCheckStatus 是预检项结果。
// ScaleCheckStatus is the result of one precheck item.
type ScaleCheckStatus string

const (
	ScaleCheckPassed  ScaleCheckStatus = "passed"
	ScaleCheckFailed  ScaleCheckStatus = "failed"
	ScaleCheckWarning ScaleCheckStatus = "warning"
	ScaleCheckSkipped ScaleCheckStatus = "skipped"
)

// ScaleCheck 是一个预检项；Name 为稳定编码，Blocking 表示该项失败时阻断提交。
// ScaleCheck is one precheck item; Name is a stable code and Blocking means a failure blocks submission.
type ScaleCheck struct {
	Name     string           `json:"name"`
	Status   ScaleCheckStatus `json:"status"`
	Blocking bool             `json:"blocking"`
	Message  string           `json:"message"`
	Target   string           `json:"target,omitempty"`
}

// ScaleTarget 是本次伸缩的目标节点（扩容为待加入节点，缩容为待移除节点）。
// ScaleTarget is the node being scaled (the joining node for scale-out, the removed node for scale-in).
type ScaleTarget struct {
	NodeID        uint     `json:"node_id,omitempty"`
	HostID        uint     `json:"host_id"`
	HostName      string   `json:"host_name"`
	HostIP        string   `json:"host_ip"`
	Role          NodeRole `json:"role"`
	InstallDir    string   `json:"install_dir"`
	HazelcastPort int      `json:"hazelcast_port"`
	APIPort       int      `json:"api_port"`
	WorkerPort    int      `json:"worker_port"`
	Address       string   `json:"address"`
}

// ScaleLoopbackNode 是 member-list 或运行时成员中含回环地址的现有节点。
// ScaleLoopbackNode is an existing node whose member-list or runtime members contain a loopback address.
type ScaleLoopbackNode struct {
	NodeID   uint   `json:"node_id"`
	HostID   uint   `json:"host_id"`
	HostName string `json:"host_name"`
	Address  string `json:"address"`
	// Source 为 config（节点配置）或 runtime（Hazelcast 运行时成员）。
	// Source is config (node config) or runtime (Hazelcast runtime members).
	Source string `json:"source"`
}

// ScaleRestartNode 是滚动重启的候选节点。/ ScaleRestartNode is a rolling-restart candidate node.
type ScaleRestartNode struct {
	NodeID   uint   `json:"node_id"`
	HostID   uint   `json:"host_id"`
	HostName string `json:"host_name"`
	Address  string `json:"address"`
}

// ScaleRollingRestart 描述扩容后可选的滚动重启。Hazelcast 只在启动时读取 tcp-ip member-list，且分裂后的合并只探测
// 启动时的列表；扩容只改写现有节点的文件、不重启，因此这些节点直到下次重启前都无法在分裂后主动找回新节点。
// 回环修正已重启的节点会直接写入扩容后的完整列表，不在候选之内。
// ScaleRollingRestart describes the optional rolling restart after scaling out. Hazelcast reads the tcp-ip member
// list only at startup, and split-brain merging only probes that list; scaling out rewrites existing nodes' files
// without restarting them, so until their next restart they cannot find the new node again after a split.
// Nodes already restarted by the loopback fix get the full post-scale list and are not candidates.
type ScaleRollingRestart struct {
	Nodes       []ScaleRestartNode `json:"nodes"`
	Available   bool               `json:"available"`
	Recommended bool               `json:"recommended"`
	// BlockedReason 不可选的原因；Note 是可选但需留意的说明（如作业状态未知）。
	// BlockedReason explains why it cannot be chosen; Note is a caveat when it can (e.g. jobs unknown).
	BlockedReason string `json:"blocked_reason,omitempty"`
	Note          string `json:"note,omitempty"`
}

// ScalePluginItem 是新节点需要补齐的集群插件。
// ScalePluginItem is a cluster plugin the joining node must receive.
type ScalePluginItem struct {
	Name           string   `json:"name"`
	ArtifactID     string   `json:"artifact_id"`
	Version        string   `json:"version"`
	Status         string   `json:"status"`
	ProfileKeys    []string `json:"profile_keys"`
	ProfileUnknown bool     `json:"profile_unknown"`
	// ProfileInferred 画像由控制面缓存中的依赖反推 / profile inferred from dependencies cached on the Control Plane
	ProfileInferred bool `json:"profile_inferred"`
}

// ScaleCheckpointStorage 描述集群 checkpoint 存储；Local 表示本地文件存储，作业 failover 后无法从 checkpoint 恢复。
// ScaleCheckpointStorage describes the cluster checkpoint storage; Local means local file storage, so failed-over
// jobs cannot restore from checkpoints.
type ScaleCheckpointStorage struct {
	Type     string `json:"type"`
	Location string `json:"location"`
	Local    bool   `json:"local"`
}

// ScaleJavaProxyFollow 描述缩容后 java-proxy 承载节点的迁移。
// ScaleJavaProxyFollow describes moving the java-proxy host after scale-in.
type ScaleJavaProxyFollow struct {
	FromNodeID     uint   `json:"from_node_id"`
	FromHostID     uint   `json:"from_host_id"`
	FromHostName   string `json:"from_host_name"`
	FromInstallDir string `json:"from_install_dir"`
	ToNodeID       uint   `json:"to_node_id"`
	ToHostID       uint   `json:"to_host_id"`
	ToHostName     string `json:"to_host_name"`
}

// ScalePlan 是伸缩计划：预检结果、成员变化、护栏与执行步骤。所有数组字段输出 []，不输出 null。
// ScalePlan is a scaling plan: precheck results, member changes, guardrails and execution steps. Every array field
// is rendered as [] instead of null.
type ScalePlan struct {
	Kind               ScaleKind               `json:"kind"`
	Mode               ScaleMode               `json:"mode,omitempty"`
	DetectedMode       ScaleMode               `json:"detected_mode,omitempty"`
	ClusterID          uint                    `json:"cluster_id"`
	DeploymentMode     DeploymentMode          `json:"deployment_mode"`
	ClusterVersion     string                  `json:"cluster_version"`
	Target             ScaleTarget             `json:"target"`
	Fingerprint        string                  `json:"fingerprint"`
	MembersBefore      []string                `json:"members_before"`
	MembersAfter       []string                `json:"members_after"`
	Checks             []ScaleCheck            `json:"checks"`
	LoopbackNodes      []ScaleLoopbackNode     `json:"loopback_nodes"`
	RequiresRestartAck bool                    `json:"requires_restart_ack"`
	PluginsToReplay    []ScalePluginItem       `json:"plugins_to_replay"`
	RunningJobs        []ScaleRunningJob       `json:"running_jobs"`
	RunningJobsKnown   bool                    `json:"running_jobs_known"`
	RequiresJobsAck    bool                    `json:"requires_jobs_ack"`
	CheckpointStorage  *ScaleCheckpointStorage `json:"checkpoint_storage"`
	JavaProxyFollow    *ScaleJavaProxyFollow   `json:"java_proxy_follow"`
	// RollingRestart 只用于扩容：入群后可选的滚动重启。/ Scale-out only: the optional rolling restart after the join.
	RollingRestart  *ScaleRollingRestart `json:"rolling_restart"`
	Steps           []ScaleStepName      `json:"steps"`
	CanSubmit       bool                 `json:"can_submit"`
	BlockingReasons []string             `json:"blocking_reasons"`
	Warnings        []string             `json:"warnings"`
	// Lang 是生成本计划文案所用的语言，不参与序列化。
	// Lang is the language used for this plan's texts and is not serialized.
	Lang string `json:"-"`
}

// normalize 把 nil 数组归一为空数组，保证 JSON 输出 [] 而不是 null。
// normalize turns nil slices into empty ones so JSON renders [] instead of null.
func (p *ScalePlan) normalize() {
	if p.MembersBefore == nil {
		p.MembersBefore = []string{}
	}
	if p.MembersAfter == nil {
		p.MembersAfter = []string{}
	}
	if p.Checks == nil {
		p.Checks = []ScaleCheck{}
	}
	if p.LoopbackNodes == nil {
		p.LoopbackNodes = []ScaleLoopbackNode{}
	}
	if p.PluginsToReplay == nil {
		p.PluginsToReplay = []ScalePluginItem{}
	}
	for i := range p.PluginsToReplay {
		if p.PluginsToReplay[i].ProfileKeys == nil {
			p.PluginsToReplay[i].ProfileKeys = []string{}
		}
	}
	if p.RunningJobs == nil {
		p.RunningJobs = []ScaleRunningJob{}
	}
	if p.Steps == nil {
		p.Steps = []ScaleStepName{}
	}
	if p.BlockingReasons == nil {
		p.BlockingReasons = []string{}
	}
	if p.Warnings == nil {
		p.Warnings = []string{}
	}
	if p.RollingRestart != nil && p.RollingRestart.Nodes == nil {
		p.RollingRestart.Nodes = []ScaleRestartNode{}
	}
}

// addCheck 追加预检项，并同步阻断原因与警告列表。
// addCheck appends a precheck item and keeps blocking reasons and warnings in sync.
func (p *ScalePlan) addCheck(check ScaleCheck) {
	p.Checks = append(p.Checks, check)
	switch {
	case check.Status == ScaleCheckFailed && check.Blocking:
		p.BlockingReasons = append(p.BlockingReasons, check.Message)
	case check.Status == ScaleCheckFailed, check.Status == ScaleCheckWarning:
		p.Warnings = append(p.Warnings, check.Message)
	}
}

// scaleView 是计划与执行共用的集群快照：节点按 ID 排序，主机信息按 ID 索引（查询失败时为 nil）。
// scaleView is the cluster snapshot shared by planning and execution: nodes sorted by ID and hosts indexed by ID
// (nil when the lookup failed).
type scaleView struct {
	cluster *Cluster
	nodes   []*ClusterNode
	hosts   map[uint]*HostInfo
	configs []*appconfig.ConfigInfo
}

func (s *ScaleService) loadScaleView(ctx context.Context, clusterID uint, withConfigs bool) (*scaleView, error) {
	cluster, err := s.svc.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	nodes, err := s.svc.repo.GetNodesByClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	view := &scaleView{cluster: cluster, nodes: nodes, hosts: make(map[uint]*HostInfo)}
	for _, node := range nodes {
		view.host(ctx, s.svc, node.HostID)
	}
	if withConfigs && s.config != nil {
		if configs, err := s.config.GetByCluster(ctx, clusterID); err == nil {
			view.configs = configs
		}
	}
	return view, nil
}

// host 返回缓存的主机信息，首次访问时查询。
// host returns the cached host info, querying it on first access.
func (v *scaleView) host(ctx context.Context, svc *Service, hostID uint) *HostInfo {
	if host, ok := v.hosts[hostID]; ok {
		return host
	}
	var host *HostInfo
	if svc != nil && svc.hostProvider != nil {
		if found, err := svc.hostProvider.GetHostByID(ctx, hostID); err == nil {
			host = found
		}
	}
	v.hosts[hostID] = host
	return host
}

// scaleMember 把节点转换为成员地址计算输入。
// scaleMember converts a node into a member-list computation input.
func scaleMember(node *ClusterNode, host *HostInfo) clustermembers.Member {
	member := clustermembers.Member{HostID: node.HostID, Role: string(node.Role), Port: node.HazelcastPort}
	if host != nil {
		member.HostIP = strings.TrimSpace(host.IPAddress)
		member.HostName = host.Name
	}
	return member
}

// nodeAddress 返回节点的 member-list 地址；主机没有地址时为空。
// nodeAddress returns the node's member-list address, empty when the host has no address.
func nodeAddress(node *ClusterNode, host *HostInfo) string {
	if node == nil || host == nil || strings.TrimSpace(host.IPAddress) == "" {
		return ""
	}
	port := node.HazelcastPort
	if port <= 0 {
		port = node.WorkerPort
	}
	return clustermembers.FormatAddress(host.IPAddress, port)
}

// hostReady 判断主机 Agent 已安装、在线且可下发命令。
// hostReady reports whether the host Agent is installed, online and able to receive commands.
func (s *ScaleService) hostReady(host *HostInfo) bool {
	return host != nil && strings.TrimSpace(host.AgentID) != "" && host.IsOnline(s.svc.heartbeatTimeout)
}

func hostLabel(host *HostInfo, hostID uint) string {
	if host != nil {
		if name := strings.TrimSpace(host.Name); name != "" {
			return name
		}
		if ip := strings.TrimSpace(host.IPAddress); ip != "" {
			return ip
		}
	}
	return fmt.Sprintf("host#%d", hostID)
}

func isMasterCapable(role NodeRole) bool {
	return role == NodeRoleMaster || role == NodeRoleMasterWorker
}

// runAgentCheck 下发一个 Agent 预检子命令，返回是否成功与可读说明。
// runAgentCheck sends one Agent precheck sub-command and returns success plus a readable message.
func (s *ScaleService) runAgentCheck(ctx context.Context, agentID, command string, params map[string]string) (bool, string, error) {
	success, message, err := s.svc.agentSender.SendCommand(ctx, agentID, command, params)
	if err != nil {
		return false, err.Error(), err
	}
	return success, parseCommandMessage(message), nil
}

// memberProbe 是 check_cluster_members 的解析结果。
// memberProbe is the parsed result of check_cluster_members.
type memberProbe struct {
	known   bool
	count   int
	members []string
	message string
}

// probeMembers 在节点所在主机读取 Hazelcast 运行时成员。
// probeMembers reads the Hazelcast runtime members on the node's host.
func (s *ScaleService) probeMembers(ctx context.Context, node *ClusterNode, host *HostInfo) memberProbe {
	if node == nil || !s.hostReady(host) {
		return memberProbe{}
	}
	port := node.HazelcastPort
	if port <= 0 {
		port = node.WorkerPort
	}
	success, message, err := s.svc.agentSender.SendCommand(ctx, host.AgentID, "check_cluster_members", map[string]string{
		"port": strconv.Itoa(port),
		"host": strings.TrimSpace(host.IPAddress),
	})
	if err != nil || !success {
		return memberProbe{message: firstNonEmpty(parseCommandMessage(message), errorString(err))}
	}
	return parseMemberProbe(message)
}

// parseMemberProbe 解析 check_cluster_members 输出中的成员数与成员地址。
// parseMemberProbe parses the member count and addresses from check_cluster_members output.
func parseMemberProbe(message string) memberProbe {
	var payload struct {
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(message)), &payload); err != nil {
		return memberProbe{message: strings.TrimSpace(message)}
	}
	probe := memberProbe{message: payload.Message, members: []string{}}
	if raw := strings.TrimSpace(payload.Details["members"]); raw != "" {
		for _, member := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(member); trimmed != "" {
				probe.members = append(probe.members, trimmed)
			}
		}
	}
	count, err := strconv.Atoi(strings.TrimSpace(payload.Details["member_count"]))
	if err != nil {
		count = len(probe.members)
	}
	probe.count = count
	probe.known = count > 0
	return probe
}

// containsAddress 判断成员列表是否包含地址（忽略方括号与大小写）。
// containsAddress reports whether the member list contains the address (ignoring brackets and case).
func containsAddress(members []string, address string) bool {
	normalized := normalizeScaleAddress(address)
	for _, member := range members {
		if normalizeScaleAddress(member) == normalized {
			return true
		}
	}
	return false
}

func normalizeScaleAddress(address string) string {
	return strings.ToLower(strings.NewReplacer("[", "", "]", "").Replace(strings.TrimSpace(address)))
}

// PlanScaleOut 计算扩容计划（只读，不修改任何状态）。
// PlanScaleOut computes a scale-out plan (read-only; nothing is modified).
func (s *ScaleService) PlanScaleOut(ctx context.Context, clusterID uint, req *ScaleOutRequest, lang string) (*ScalePlan, error) {
	if err := s.ensureRunnable(); err != nil {
		return nil, err
	}
	if req == nil || req.HostID == 0 {
		return nil, fmt.Errorf("%w: host_id is required", ErrScaleInvalidRequest)
	}
	lang = normalizeScaleLang(lang)
	view, err := s.loadScaleView(ctx, clusterID, true)
	if err != nil {
		return nil, err
	}
	cluster := view.cluster

	role, err := normalizeNodeRoleForDeployment(cluster.DeploymentMode, scaleOutRole(cluster.DeploymentMode, req.Role))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScaleInvalidRequest, err)
	}
	if req.Mode != "" && req.Mode != ScaleModeInstall && req.Mode != ScaleModeRegister {
		return nil, fmt.Errorf("%w: mode must be install or register", ErrScaleInvalidRequest)
	}
	host, err := s.svc.hostProvider.GetHostByID(ctx, req.HostID)
	if err != nil || host == nil {
		return nil, fmt.Errorf("%w: host %d not found: %v", ErrScaleInvalidRequest, req.HostID, err)
	}
	view.hosts[req.HostID] = host
	ready := s.hostReady(host) && host.AgentStatus == "installed"

	plan := &ScalePlan{
		Kind:           ScaleKindOut,
		ClusterID:      clusterID,
		DeploymentMode: cluster.DeploymentMode,
		ClusterVersion: cluster.Version,
		Lang:           lang,
	}

	// 识别主机上是否已有可纳管的 SeaTunnel 进程。
	// Detect whether the host already runs a SeaTunnel process that can be registered.
	process := s.detectRegistrableProcess(ctx, host, ready, req.InstallDir, cluster.DeploymentMode, role)
	plan.DetectedMode = ScaleModeInstall
	if process != nil {
		plan.DetectedMode = ScaleModeRegister
	}
	plan.Mode = req.Mode
	if plan.Mode == "" {
		plan.Mode = plan.DetectedMode
	}

	desired, err := s.resolveScaleOutNode(cluster, view.nodes, req, role, plan.Mode, process)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScaleInvalidRequest, err)
	}
	plan.Target = ScaleTarget{
		HostID:        req.HostID,
		HostName:      hostLabel(host, req.HostID),
		HostIP:        strings.TrimSpace(host.IPAddress),
		Role:          desired.Role,
		InstallDir:    desired.InstallDir,
		HazelcastPort: desired.HazelcastPort,
		APIPort:       desired.APIPort,
		WorkerPort:    desired.WorkerPort,
		Address:       nodeAddress(desired, host),
	}

	// 成员地址：现有节点 → 加入新节点后的完整列表。
	// Member addresses: existing nodes → the complete list after the new node joins.
	existing := make([]clustermembers.Member, 0, len(view.nodes))
	for _, node := range view.nodes {
		existing = append(existing, scaleMember(node, view.hosts[node.HostID]))
	}
	newMember := scaleMember(desired, host)
	plan.MembersBefore = clustermembers.Compute(string(cluster.DeploymentMode), existing, nil, nil)
	plan.MembersAfter = clustermembers.Compute(string(cluster.DeploymentMode), existing, []clustermembers.Member{newMember}, nil)

	s.addAgentCheck(plan, host, ready, lang)
	s.addNodeConflictChecks(ctx, plan, view, host, desired, lang)
	if plan.Mode == ScaleModeInstall {
		s.addInstallChecks(ctx, plan, cluster, host, ready, desired, lang)
	} else {
		s.addRegisterChecks(ctx, plan, cluster, host, ready, desired, process, lang)
		s.addTargetJoinedCheck(ctx, plan, view, host, ready, desired, lang)
	}
	addMemberAddressChecks(plan, append(existing, newMember), true, lang)
	s.addReachabilityChecks(ctx, plan, view, host, ready, lang)
	s.addConfigTemplateCheck(plan, view, lang)

	plan.LoopbackNodes = s.detectLoopbackNodes(ctx, view)
	s.fillRunningJobs(ctx, plan, lang)
	plan.CheckpointStorage = checkpointStorageFromConfigs(view.configs)
	if len(plan.LoopbackNodes) > 0 {
		plan.RequiresRestartAck = true
		labels := make([]string, 0, len(plan.LoopbackNodes))
		for _, node := range plan.LoopbackNodes {
			labels = append(labels, node.HostName)
		}
		check := ScaleCheck{
			Name:     "loopback_members",
			Status:   ScaleCheckWarning,
			Blocking: false,
			Target:   strings.Join(labels, ","),
			Message: scaleText(lang,
				fmt.Sprintf("%d 个现有节点的成员地址为回环地址（%s），需要先修正成员列表并逐个重启这些节点，新节点才能入群", len(plan.LoopbackNodes), strings.Join(labels, ", ")),
				fmt.Sprintf("%d existing node(s) use loopback member addresses (%s); their member list must be fixed and they must be restarted one by one before the new node can join", len(plan.LoopbackNodes), strings.Join(labels, ", "))),
		}
		if len(plan.RunningJobs) > 0 {
			check.Status = ScaleCheckFailed
			check.Blocking = true
			check.Message = scaleText(lang,
				fmt.Sprintf("集群有 %d 个运行中作业，不能重启回环成员节点（%s）；请先停止作业后再扩容", len(plan.RunningJobs), strings.Join(labels, ", ")),
				fmt.Sprintf("the cluster has %d running job(s), so loopback member nodes (%s) cannot be restarted; stop the jobs before scaling out", len(plan.RunningJobs), strings.Join(labels, ", ")))
		}
		plan.addCheck(check)
		// 作业状态未知时不阻断（引擎 HTTP 关闭时永远无法确认），但必须醒目提示重启风险；用户仍需勾选重启确认。
		// Unknown jobs do not block (they can never be confirmed when the engine HTTP API is off), but the restart
		// risk must be shown prominently; the user still has to tick the restart acknowledgement.
		if !plan.RunningJobsKnown {
			plan.addCheck(ScaleCheck{
				Name:     "loopback_restart_jobs_unknown",
				Status:   ScaleCheckWarning,
				Blocking: false,
				Target:   strings.Join(labels, ","),
				Message: scaleText(lang,
					fmt.Sprintf("无法确认是否有运行中作业，修正成员地址将重启节点（%s），运行中的作业可能中断", strings.Join(labels, ", ")),
					fmt.Sprintf("running jobs cannot be confirmed; fixing member addresses restarts node(s) (%s) and running jobs may be interrupted", strings.Join(labels, ", "))),
			})
		}
	}
	s.fillPluginsToReplay(ctx, plan, lang)
	plan.RollingRestart = s.planRollingRestart(plan, view, lang)

	plan.Steps = scaleOutSteps(plan.Mode, len(plan.LoopbackNodes) > 0)
	plan.CanSubmit = len(plan.BlockingReasons) == 0
	plan.normalize()
	plan.Fingerprint = scalePlanFingerprint(plan)
	return plan, nil
}

// planRollingRestart 计算滚动重启候选：主机在线、进程运行中、且不会被回环修正重启的现有节点。集群有运行中作业时不可选；
// 作业状态未知时可选但给出说明。
// planRollingRestart computes rolling-restart candidates: existing nodes on online hosts with running processes that
// the loopback fix will not restart. It cannot be chosen while jobs are running; with unknown jobs it can, with a note.
func (s *ScaleService) planRollingRestart(plan *ScalePlan, view *scaleView, lang string) *ScaleRollingRestart {
	loopback := make(map[uint]struct{}, len(plan.LoopbackNodes))
	for _, node := range plan.LoopbackNodes {
		loopback[node.NodeID] = struct{}{}
	}
	restart := &ScaleRollingRestart{Nodes: []ScaleRestartNode{}}
	for _, node := range view.nodes {
		if _, fixed := loopback[node.ID]; fixed || node.Status != NodeStatusRunning {
			continue
		}
		host := view.hosts[node.HostID]
		if !s.hostReady(host) {
			continue
		}
		restart.Nodes = append(restart.Nodes, ScaleRestartNode{
			NodeID:   node.ID,
			HostID:   node.HostID,
			HostName: hostLabel(host, node.HostID),
			Address:  nodeAddress(node, host),
		})
	}
	switch {
	case len(restart.Nodes) == 0:
		restart.BlockedReason = scaleText(lang, "没有需要重启的现有节点", "no existing node needs a restart")
	case len(plan.RunningJobs) > 0:
		restart.BlockedReason = scaleText(lang,
			fmt.Sprintf("集群有 %d 个运行中作业，不能滚动重启；可在空闲时手动重启这些节点", len(plan.RunningJobs)),
			fmt.Sprintf("the cluster has %d running job(s), so a rolling restart is not allowed; restart these nodes manually when idle", len(plan.RunningJobs)))
	default:
		restart.Available = true
		restart.Recommended = true
		if !plan.RunningJobsKnown {
			restart.Note = scaleText(lang, "无法确认是否有运行中作业，重启期间作业可能中断", "running jobs cannot be confirmed; jobs may be interrupted during the restart")
		}
	}
	return restart
}

// scaleOutRole 混合模式忽略请求角色，分离模式必须显式选择 master 或 worker。
// scaleOutRole ignores the requested role in hybrid mode; separated mode requires master or worker explicitly.
func scaleOutRole(mode DeploymentMode, requested NodeRole) NodeRole {
	if mode == DeploymentModeHybrid {
		return NodeRoleMasterWorker
	}
	return NodeRole(strings.ToLower(strings.TrimSpace(string(requested))))
}

// detectRegistrableProcess 在主机上寻找与角色匹配、且未被任何集群登记的 SeaTunnel 进程；指定安装目录时按目录匹配。
// detectRegistrableProcess finds a SeaTunnel process on the host that matches the role and is not registered by any
// cluster; when an install directory is given it must match.
func (s *ScaleService) detectRegistrableProcess(ctx context.Context, host *HostInfo, ready bool, installDir string, mode DeploymentMode, role NodeRole) *ScaleDiscoveredProcess {
	if s.discoverer == nil || host == nil || !ready {
		return nil
	}
	processes, err := s.discoverer.DiscoverSeaTunnelProcesses(ctx, host.ID)
	if err != nil {
		return nil
	}
	registered := make(map[string]struct{})
	if nodes, err := s.svc.repo.GetNodesByHostID(ctx, host.ID); err == nil {
		for _, node := range nodes {
			registered[cleanInstallDir(node.InstallDir)+"|"+string(node.Role)] = struct{}{}
		}
	}
	wantDir := cleanInstallDir(installDir)
	for i := range processes {
		process := processes[i]
		processDir := cleanInstallDir(process.InstallDir)
		if wantDir != "" && processDir != wantDir {
			continue
		}
		if !discoveredRoleMatches(mode, role, process.Role) {
			continue
		}
		if _, taken := registered[processDir+"|"+string(role)]; taken {
			continue
		}
		return &process
	}
	return nil
}

func cleanInstallDir(dir string) string {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return ""
	}
	return strings.TrimRight(trimmed, "/")
}

// discoveredRoleMatches 判断发现的进程角色是否与目标角色兼容。
// discoveredRoleMatches reports whether a discovered process role is compatible with the target role.
func discoveredRoleMatches(mode DeploymentMode, role NodeRole, discovered string) bool {
	normalized := strings.ToLower(strings.TrimSpace(discovered))
	if mode == DeploymentModeHybrid {
		return normalized == "" || normalized == "hybrid" || normalized == string(NodeRoleMasterWorker) || normalized == "master" || normalized == "worker"
	}
	return normalized == string(role)
}

// resolveScaleOutNode 计算新节点的安装目录与端口：请求值 → 发现结果（纳管）→ 集群配置 → 同角色节点 → 默认值。
// resolveScaleOutNode resolves the new node's install directory and ports: request → discovery (register) →
// cluster config → a same-role node → defaults.
func (s *ScaleService) resolveScaleOutNode(cluster *Cluster, nodes []*ClusterNode, req *ScaleOutRequest, role NodeRole, mode ScaleMode, process *ScaleDiscoveredProcess) (*ClusterNode, error) {
	hazelcastPort, apiPort, workerPort := req.Ports.HazelcastPort, req.Ports.APIPort, req.Ports.WorkerPort
	installDir := strings.TrimSpace(req.InstallDir)
	if mode == ScaleModeRegister && process != nil {
		if installDir == "" {
			installDir = strings.TrimSpace(process.InstallDir)
		}
		if hazelcastPort == 0 {
			hazelcastPort = process.HazelcastPort
		}
		if apiPort == 0 && isMasterCapable(role) {
			apiPort = process.APIPort
		}
	}
	if ports := cluster.Config.GetPortConfig(); ports != nil {
		switch role {
		case NodeRoleMaster:
			hazelcastPort = firstPositive(hazelcastPort, ports.MasterHazelcastPort)
			apiPort = firstPositive(apiPort, ports.MasterAPIPort)
		case NodeRoleWorker:
			hazelcastPort = firstPositive(hazelcastPort, ports.WorkerPort)
		case NodeRoleMasterWorker:
			hazelcastPort = firstPositive(hazelcastPort, ports.MasterHazelcastPort)
			apiPort = firstPositive(apiPort, ports.MasterAPIPort)
			workerPort = firstPositive(workerPort, ports.WorkerPort)
		}
	}
	var peer *ClusterNode
	for _, node := range nodes {
		if node.Role == role {
			peer = node
			break
		}
	}
	if peer != nil {
		hazelcastPort = firstPositive(hazelcastPort, peer.HazelcastPort)
		apiPort = firstPositive(apiPort, peer.APIPort)
		workerPort = firstPositive(workerPort, peer.WorkerPort)
	}
	if installDir == "" {
		installDir = strings.TrimSpace(cluster.InstallDir)
	}
	if installDir == "" && peer != nil {
		installDir = strings.TrimSpace(peer.InstallDir)
	}
	return buildNodeForCreate(cluster.ID, req.HostID, cluster, role, installDir, hazelcastPort, apiPort, workerPort, nil)
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (s *ScaleService) addAgentCheck(plan *ScalePlan, host *HostInfo, ready bool, lang string) {
	check := ScaleCheck{Name: "agent_online", Blocking: true, Target: hostLabel(host, plan.Target.HostID)}
	switch {
	case host == nil || host.AgentStatus != "installed" || strings.TrimSpace(host.AgentID) == "":
		check.Status = ScaleCheckFailed
		check.Message = scaleText(lang, "目标主机未安装 Agent", "the target host has no Agent installed")
	case !ready:
		check.Status = ScaleCheckFailed
		check.Message = scaleText(lang, "目标主机 Agent 离线", "the target host Agent is offline")
	default:
		check.Status = ScaleCheckPassed
		check.Message = scaleText(lang, "Agent 已安装且在线", "the Agent is installed and online")
	}
	plan.addCheck(check)
}

// addNodeConflictChecks 阻断重复节点与安装目录冲突：同主机同角色已登记、安装模式下主机已有本集群节点、目录被其他集群占用。
// addNodeConflictChecks blocks duplicates and install directory conflicts: the same host+role already registered, an
// install-mode host that already has a node of this cluster, or a directory used by another cluster.
func (s *ScaleService) addNodeConflictChecks(ctx context.Context, plan *ScalePlan, view *scaleView, host *HostInfo, desired *ClusterNode, lang string) {
	hostNodes, err := s.svc.repo.GetNodesByHostID(ctx, plan.Target.HostID)
	if err != nil {
		hostNodes = nil
	}
	targetDir := cleanInstallDir(desired.InstallDir)
	for _, node := range hostNodes {
		sameCluster := node.ClusterID == view.cluster.ID
		switch {
		case sameCluster && node.Role == desired.Role:
			plan.addCheck(ScaleCheck{Name: "node_exists", Status: ScaleCheckFailed, Blocking: true, Target: hostLabel(host, node.HostID),
				Message: scaleText(lang, "该主机已是本集群的同角色节点", "the host is already a node of this cluster with the same role")})
			return
		case sameCluster && plan.Mode == ScaleModeInstall:
			plan.addCheck(ScaleCheck{Name: "host_has_cluster_node", Status: ScaleCheckFailed, Blocking: true, Target: node.InstallDir,
				Message: scaleText(lang,
					fmt.Sprintf("该主机已在 %s 运行本集群的其他角色节点，请使用纳管模式添加", node.InstallDir),
					fmt.Sprintf("the host already runs another role of this cluster in %s; use register mode", node.InstallDir))})
			return
		case !sameCluster && cleanInstallDir(node.InstallDir) == targetDir:
			plan.addCheck(ScaleCheck{Name: "install_dir_conflict", Status: ScaleCheckFailed, Blocking: true, Target: node.InstallDir,
				Message: scaleText(lang,
					fmt.Sprintf("安装目录 %s 已被集群 %d 的节点使用", node.InstallDir, node.ClusterID),
					fmt.Sprintf("install directory %s is used by a node of cluster %d", node.InstallDir, node.ClusterID))})
			return
		}
	}
}

// addInstallChecks 安装模式预检：版本、Java、端口空闲、目录可创建、磁盘空间（警告）。
// addInstallChecks runs install-mode prechecks: version, Java, free ports, creatable directory, disk space (warning).
func (s *ScaleService) addInstallChecks(ctx context.Context, plan *ScalePlan, cluster *Cluster, host *HostInfo, ready bool, desired *ClusterNode, lang string) {
	if strings.TrimSpace(cluster.Version) == "" {
		plan.addCheck(ScaleCheck{Name: "cluster_version", Status: ScaleCheckFailed, Blocking: true,
			Message: scaleText(lang, "集群未记录 SeaTunnel 版本，无法确定安装包", "the cluster has no SeaTunnel version, so the package cannot be determined")})
	}
	if !ready {
		for _, name := range []string{"java", "port_free", "directory"} {
			plan.addCheck(ScaleCheck{Name: name, Status: ScaleCheckSkipped, Blocking: true,
				Message: scaleText(lang, "Agent 不可用，已跳过", "skipped because the Agent is unavailable")})
		}
		return
	}

	javaCheck := ScaleCheck{Name: "java", Blocking: true}
	if ok, message, err := s.runAgentCheck(ctx, host.AgentID, "check_java", map[string]string{}); err != nil || !ok {
		javaCheck.Status = ScaleCheckFailed
		javaCheck.Message = scaleText(lang, "未检测到可用的 Java（STX 不负责安装 JDK）："+message, "no usable Java found (STX does not install a JDK): "+message)
	} else {
		javaCheck.Status = ScaleCheckPassed
		javaCheck.Message = message
	}
	plan.addCheck(javaCheck)

	for _, port := range scaleListenPorts(desired) {
		check := ScaleCheck{Name: "port_free", Blocking: true, Target: strconv.Itoa(port)}
		listening, _, err := s.runAgentCheck(ctx, host.AgentID, "check_port", map[string]string{"port": strconv.Itoa(port)})
		switch {
		case err != nil:
			check.Status = ScaleCheckFailed
			check.Message = scaleText(lang, fmt.Sprintf("检查端口 %d 失败：%v", port, err), fmt.Sprintf("failed to check port %d: %v", port, err))
		case listening:
			check.Status = ScaleCheckFailed
			check.Message = scaleText(lang, fmt.Sprintf("端口 %d 已被占用", port), fmt.Sprintf("port %d is already in use", port))
		default:
			check.Status = ScaleCheckPassed
			check.Message = scaleText(lang, fmt.Sprintf("端口 %d 空闲", port), fmt.Sprintf("port %d is free", port))
		}
		plan.addCheck(check)
	}

	dirCheck := ScaleCheck{Name: "directory", Blocking: true, Target: desired.InstallDir}
	if ok, message, err := s.runAgentCheck(ctx, host.AgentID, "check_path_ready", map[string]string{"path": desired.InstallDir}); err != nil || !ok {
		dirCheck.Status = ScaleCheckFailed
		dirCheck.Message = scaleText(lang, "安装目录不可写也无法创建："+message, "the install directory is neither writable nor creatable: "+message)
	} else {
		dirCheck.Status = ScaleCheckPassed
		dirCheck.Message = scaleText(lang, "安装目录可写或可创建", "the install directory is writable or creatable")
	}
	plan.addCheck(dirCheck)

	diskCheck := ScaleCheck{Name: "disk_space", Blocking: false}
	switch {
	case host.DiskUsage <= 0:
		diskCheck.Status = ScaleCheckSkipped
		diskCheck.Message = scaleText(lang, "暂无磁盘使用率数据", "no disk usage data yet")
	case host.DiskUsage >= 90:
		diskCheck.Status = ScaleCheckWarning
		diskCheck.Message = scaleText(lang, fmt.Sprintf("磁盘使用率 %.0f%%，安装包与日志可能写满磁盘", host.DiskUsage), fmt.Sprintf("disk usage is %.0f%%; the package and logs may fill the disk", host.DiskUsage))
	default:
		diskCheck.Status = ScaleCheckPassed
		diskCheck.Message = scaleText(lang, fmt.Sprintf("磁盘使用率 %.0f%%", host.DiskUsage), fmt.Sprintf("disk usage is %.0f%%", host.DiskUsage))
	}
	plan.addCheck(diskCheck)
}

// scaleListenPorts 返回新节点会监听的端口（Hazelcast 端口与 REST API 端口）。
// scaleListenPorts returns the ports the new node will listen on (Hazelcast and REST API ports).
func scaleListenPorts(node *ClusterNode) []int {
	ports := make([]int, 0, 2)
	seen := make(map[int]struct{})
	for _, port := range []int{node.HazelcastPort, node.APIPort} {
		if port <= 0 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	return ports
}

// addRegisterChecks 纳管模式预检：端口监听、目录存在可写、版本一致。
// addRegisterChecks runs register-mode prechecks: port listening, directory exists and is writable, version match.
func (s *ScaleService) addRegisterChecks(ctx context.Context, plan *ScalePlan, cluster *Cluster, host *HostInfo, ready bool, desired *ClusterNode, process *ScaleDiscoveredProcess, lang string) {
	if !ready {
		for _, name := range []string{"port_listening", "directory", "version"} {
			plan.addCheck(ScaleCheck{Name: name, Status: ScaleCheckSkipped, Blocking: true,
				Message: scaleText(lang, "Agent 不可用，已跳过", "skipped because the Agent is unavailable")})
		}
		return
	}
	portCheck := ScaleCheck{Name: "port_listening", Blocking: true, Target: strconv.Itoa(desired.HazelcastPort)}
	if listening, _, err := s.runAgentCheck(ctx, host.AgentID, "check_port", map[string]string{"port": strconv.Itoa(desired.HazelcastPort)}); err != nil || !listening {
		portCheck.Status = ScaleCheckFailed
		portCheck.Message = scaleText(lang,
			fmt.Sprintf("端口 %d 未监听，SeaTunnel 未在该主机运行，请改用安装模式", desired.HazelcastPort),
			fmt.Sprintf("port %d is not listening; SeaTunnel is not running on the host, use install mode", desired.HazelcastPort))
	} else {
		portCheck.Status = ScaleCheckPassed
		portCheck.Message = scaleText(lang, fmt.Sprintf("端口 %d 正在监听", desired.HazelcastPort), fmt.Sprintf("port %d is listening", desired.HazelcastPort))
	}
	plan.addCheck(portCheck)

	dirCheck := ScaleCheck{Name: "directory", Blocking: true, Target: desired.InstallDir}
	if ok, message, err := s.runAgentCheck(ctx, host.AgentID, "check_directory", map[string]string{"path": desired.InstallDir}); err != nil || !ok {
		dirCheck.Status = ScaleCheckFailed
		dirCheck.Message = scaleText(lang, "安装目录不存在或不可写："+message, "the install directory does not exist or is not writable: "+message)
	} else {
		dirCheck.Status = ScaleCheckPassed
		dirCheck.Message = scaleText(lang, "安装目录存在且可写", "the install directory exists and is writable")
	}
	plan.addCheck(dirCheck)

	versionCheck := ScaleCheck{Name: "version", Blocking: true, Target: cluster.Version}
	discovered := ""
	if process != nil {
		discovered = strings.TrimSpace(process.Version)
	}
	switch {
	case discovered == "" || strings.TrimSpace(cluster.Version) == "":
		versionCheck.Status = ScaleCheckWarning
		versionCheck.Blocking = false
		versionCheck.Message = scaleText(lang, "无法确认节点 SeaTunnel 版本，请确认与集群版本一致", "the node's SeaTunnel version is unknown; make sure it matches the cluster version")
	case !strings.EqualFold(discovered, strings.TrimSpace(cluster.Version)):
		versionCheck.Status = ScaleCheckFailed
		versionCheck.Message = scaleText(lang,
			fmt.Sprintf("节点版本 %s 与集群版本 %s 不一致", discovered, cluster.Version),
			fmt.Sprintf("node version %s differs from cluster version %s", discovered, cluster.Version))
	default:
		versionCheck.Status = ScaleCheckPassed
		versionCheck.Message = scaleText(lang, "节点版本与集群一致", "the node version matches the cluster")
	}
	plan.addCheck(versionCheck)
}

// addTargetJoinedCheck 纳管模式下读取目标节点的 Hazelcast 运行时成员，判断其是否已与现有集群组网。
// 纳管「只登记不覆盖」不会重启节点；若节点自成一个集群，提前给出警告（不阻断），避免到最后校验才失败。
// addTargetJoinedCheck reads the target node's Hazelcast runtime members in register mode to tell whether it has
// already joined the existing cluster. Registering never restarts the node ("register, don't overwrite"), so a node
// that forms its own cluster gets an early warning (non-blocking) instead of failing only at final verification.
func (s *ScaleService) addTargetJoinedCheck(ctx context.Context, plan *ScalePlan, view *scaleView, host *HostInfo, ready bool, desired *ClusterNode, lang string) {
	existing := make([]string, 0, len(view.nodes))
	for _, node := range view.nodes {
		address := nodeAddress(node, view.hosts[node.HostID])
		if address == "" || clustermembers.IsLoopback(address) {
			continue
		}
		existing = append(existing, address)
	}
	// 集群尚无节点或现有节点只有回环地址时无从比对，跳过。
	// Nothing to compare against when the cluster has no node or only loopback addresses; skip.
	if len(existing) == 0 || !ready {
		return
	}
	check := ScaleCheck{Name: "target_joined_cluster", Status: ScaleCheckPassed, Blocking: false, Target: plan.Target.Address}
	probe := s.probeMembers(ctx, desired, host)
	switch {
	case !probe.known:
		check.Status = ScaleCheckWarning
		check.Message = scaleText(lang,
			"无法读取该节点的集群成员，无法确认其是否已与集群组网；若未组网，纳管后需手动重启该节点以加入",
			"cannot read the node's cluster members, so whether it has joined the cluster is unknown; if not, restart it manually after registering")
	case !anyAddressIn(probe.members, existing):
		check.Status = ScaleCheckWarning
		check.Message = scaleText(lang,
			fmt.Sprintf("该节点尚未与集群组网（其成员：%s），纳管后需手动重启以加入", strings.Join(probe.members, ", ")),
			fmt.Sprintf("the node has not joined the cluster yet (its members: %s); restart it manually after registering", strings.Join(probe.members, ", ")))
	default:
		check.Message = scaleText(lang, "该节点已与集群组网", "the node has already joined the cluster")
	}
	plan.addCheck(check)
}

// anyAddressIn 判断 candidates 中是否至少有一个地址出现在成员列表中。
// anyAddressIn reports whether at least one candidate address appears in the member list.
func anyAddressIn(members []string, candidates []string) bool {
	for _, candidate := range candidates {
		if containsAddress(members, candidate) {
			return true
		}
	}
	return false
}

// addMemberAddressChecks 把成员地址问题（空、回环、重复 IP）转换为预检项；blocking=false 时只作警告。
// addMemberAddressChecks turns member address issues (empty, loopback, duplicate IP) into precheck items; with
// blocking=false they are only warnings.
func addMemberAddressChecks(plan *ScalePlan, members []clustermembers.Member, blocking bool, lang string) {
	issues := clustermembers.Validate(members)
	if len(issues) == 0 {
		plan.addCheck(ScaleCheck{Name: "member_address", Status: ScaleCheckPassed, Blocking: blocking,
			Message: scaleText(lang, "节点地址可路由且不重复", "node addresses are routable and unique")})
		return
	}
	for _, issue := range issues {
		status := ScaleCheckFailed
		if !blocking {
			status = ScaleCheckWarning
		}
		plan.addCheck(ScaleCheck{Name: "member_address", Status: status, Blocking: blocking, Target: firstNonEmpty(issue.HostIP, issue.HostName),
			Message: scaleText(lang,
				fmt.Sprintf("节点地址不可路由或重复（%s %s），请在主机管理修正 IP（必要时在 Agent 配置 agent.ip）", issue.Code, firstNonEmpty(issue.HostIP, issue.HostName)),
				fmt.Sprintf("node address is not routable or duplicated (%s %s); fix the host IP in host management (pin agent.ip if needed)", issue.Code, firstNonEmpty(issue.HostIP, issue.HostName)))})
	}
}

// addReachabilityChecks 由新主机 Agent 探测到每个现有节点 Hazelcast 端口的 TCP 连通性。
// addReachabilityChecks probes TCP reachability from the new host's Agent to every existing node's Hazelcast port.
func (s *ScaleService) addReachabilityChecks(ctx context.Context, plan *ScalePlan, view *scaleView, host *HostInfo, ready bool, lang string) {
	if len(view.nodes) == 0 {
		return
	}
	if !ready {
		plan.addCheck(ScaleCheck{Name: "member_reachability", Status: ScaleCheckSkipped, Blocking: true,
			Message: scaleText(lang, "Agent 不可用，已跳过", "skipped because the Agent is unavailable")})
		return
	}
	seen := make(map[string]struct{})
	for _, node := range view.nodes {
		peerHost := view.hosts[node.HostID]
		address := nodeAddress(node, peerHost)
		if address == "" || clustermembers.IsLoopback(address) {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		port := node.HazelcastPort
		if port <= 0 {
			port = node.WorkerPort
		}
		check := ScaleCheck{Name: "member_reachability", Blocking: true, Target: address}
		ok, message, err := s.runAgentCheck(ctx, host.AgentID, "check_tcp", map[string]string{
			"host":            strings.TrimSpace(peerHost.IPAddress),
			"port":            strconv.Itoa(port),
			"timeout_seconds": "3",
		})
		if err != nil || !ok {
			check.Status = ScaleCheckFailed
			check.Message = scaleText(lang,
				fmt.Sprintf("新主机无法连接现有节点 %s（常见原因：节点 IP 不可路由、多网卡、防火墙）：%s", address, message),
				fmt.Sprintf("the new host cannot reach existing node %s (typical causes: unroutable IP, multiple NICs, firewall): %s", address, message))
		} else {
			check.Status = ScaleCheckPassed
			check.Message = scaleText(lang, fmt.Sprintf("可连接 %s", address), fmt.Sprintf("%s is reachable", address))
		}
		plan.addCheck(check)
	}
}

// addConfigTemplateCheck 集群没有任何配置模板时提示新节点将使用默认配置。
// addConfigTemplateCheck warns that the new node keeps default configs when the cluster has no config template.
func (s *ScaleService) addConfigTemplateCheck(plan *ScalePlan, view *scaleView, lang string) {
	for _, config := range view.configs {
		if config != nil && config.IsTemplate {
			return
		}
	}
	plan.addCheck(ScaleCheck{Name: "config_templates", Status: ScaleCheckWarning,
		Message: scaleText(lang, "集群没有配置模板，新节点将保留安装默认配置（成员列表仍会修补）", "the cluster has no config templates; the new node keeps its default configs (member lists are still patched)")})
}

// detectLoopbackNodes 识别成员地址含回环的现有节点：静态检查节点配置（无节点配置时看模板），运行时检查 Hazelcast 成员。
// detectLoopbackNodes finds existing nodes with loopback member addresses: statically from node configs (templates
// when a node has none) and at runtime from Hazelcast members.
func (s *ScaleService) detectLoopbackNodes(ctx context.Context, view *scaleView) []ScaleLoopbackNode {
	result := make([]ScaleLoopbackNode, 0)
	memberTypes := make(map[appconfig.ConfigType]struct{})
	for _, configType := range appconfig.MemberListConfigTypes(string(view.cluster.DeploymentMode)) {
		memberTypes[configType] = struct{}{}
	}
	templates := make([]string, 0)
	byHost := make(map[uint][]string)
	for _, config := range view.configs {
		if config == nil {
			continue
		}
		if _, ok := memberTypes[config.ConfigType]; !ok {
			continue
		}
		if config.IsTemplate {
			templates = append(templates, config.Content)
		} else if config.HostID != nil {
			byHost[*config.HostID] = append(byHost[*config.HostID], config.Content)
		}
	}

	for _, node := range view.nodes {
		host := view.hosts[node.HostID]
		contents := byHost[node.HostID]
		if len(contents) == 0 {
			contents = templates
		}
		var found *ScaleLoopbackNode
		for _, content := range contents {
			members, err := appconfig.ReadMemberList(content)
			if err != nil {
				continue
			}
			for _, member := range members {
				if clustermembers.IsLoopback(member) {
					found = &ScaleLoopbackNode{NodeID: node.ID, HostID: node.HostID, HostName: hostLabel(host, node.HostID), Address: member, Source: "config"}
					break
				}
			}
			if found != nil {
				break
			}
		}
		if found == nil && node.Status == NodeStatusRunning {
			probe := s.probeMembers(ctx, node, host)
			for _, member := range probe.members {
				if clustermembers.IsLoopback(member) {
					found = &ScaleLoopbackNode{NodeID: node.ID, HostID: node.HostID, HostName: hostLabel(host, node.HostID), Address: member, Source: "runtime"}
					break
				}
			}
		}
		if found != nil {
			result = append(result, *found)
		}
	}
	return result
}

// fillRunningJobs 读取集群运行中作业；无法读取时记为未知并给出警告。
// fillRunningJobs reads the cluster's running jobs; when unavailable they are marked unknown with a warning.
func (s *ScaleService) fillRunningJobs(ctx context.Context, plan *ScalePlan, lang string) {
	plan.RunningJobs = []ScaleRunningJob{}
	if s.jobs == nil {
		plan.addCheck(ScaleCheck{Name: "running_jobs", Status: ScaleCheckWarning,
			Message: scaleText(lang, "无法获取运行中作业（未配置作业服务）", "running jobs are unknown (job service not configured)")})
		return
	}
	jobs, err := s.jobs.ListClusterRunningJobs(ctx, plan.ClusterID)
	if err != nil {
		plan.addCheck(ScaleCheck{Name: "running_jobs", Status: ScaleCheckWarning,
			Message: scaleText(lang, "无法获取运行中作业："+err.Error(), "running jobs are unknown: "+err.Error())})
		return
	}
	plan.RunningJobsKnown = true
	if jobs != nil {
		plan.RunningJobs = jobs
	}
	sort.SliceStable(plan.RunningJobs, func(i, j int) bool { return plan.RunningJobs[i].JobID < plan.RunningJobs[j].JobID })
	status := ScaleCheckPassed
	message := scaleText(lang, "集群没有运行中作业", "no running jobs on the cluster")
	if len(plan.RunningJobs) > 0 {
		status = ScaleCheckWarning
		message = scaleText(lang, fmt.Sprintf("集群有 %d 个运行中作业", len(plan.RunningJobs)), fmt.Sprintf("the cluster has %d running job(s)", len(plan.RunningJobs)))
	}
	plan.addCheck(ScaleCheck{Name: "running_jobs", Status: status, Message: message})
}

// fillPluginsToReplay 列出新节点需要补齐的集群插件。
// fillPluginsToReplay lists the cluster plugins the new node must receive.
func (s *ScaleService) fillPluginsToReplay(ctx context.Context, plan *ScalePlan, lang string) {
	plan.PluginsToReplay = []ScalePluginItem{}
	if s.plugins == nil {
		return
	}
	candidates, err := s.plugins.ListReplayCandidates(ctx, plan.ClusterID)
	if err != nil {
		plan.addCheck(ScaleCheck{Name: "plugins", Status: ScaleCheckWarning,
			Message: scaleText(lang, "无法读取集群插件清单："+err.Error(), "failed to read cluster plugins: "+err.Error())})
		return
	}
	for _, candidate := range candidates {
		plan.PluginsToReplay = append(plan.PluginsToReplay, ScalePluginItem{
			Name:            candidate.PluginName,
			ArtifactID:      candidate.ArtifactID,
			Version:         candidate.Version,
			Status:          string(candidate.Status),
			ProfileKeys:     append([]string{}, candidate.ProfileKeys...),
			ProfileUnknown:  candidate.ProfileUnknown,
			ProfileInferred: candidate.ProfileInferred,
		})
	}
}

// checkpointStorageFromConfigs 从 seatunnel.yaml 模板（无模板时取任一节点配置）解析 checkpoint 存储类型。
// checkpointStorageFromConfigs parses the checkpoint storage from the seatunnel.yaml template (any node config when
// no template exists).
func checkpointStorageFromConfigs(configs []*appconfig.ConfigInfo) *ScaleCheckpointStorage {
	content := ""
	for _, config := range configs {
		if config == nil || config.ConfigType != appconfig.ConfigTypeSeatunnel {
			continue
		}
		if config.IsTemplate {
			content = config.Content
			break
		}
		if content == "" {
			content = config.Content
		}
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return parseCheckpointStorageSummary(content)
}

// parseCheckpointStorageSummary 解析 checkpoint.storage 的 type 与 fs.defaultFS；file:// 或本地路径判定为本地存储。
// parseCheckpointStorageSummary parses checkpoint.storage type and fs.defaultFS; file:// or a local path is local.
func parseCheckpointStorageSummary(content string) *ScaleCheckpointStorage {
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return nil
	}
	storage := nestedMap(root, "seatunnel", "engine", "checkpoint", "storage")
	if len(storage) == 0 {
		return nil
	}
	pluginConfig := asMap(storage["plugin-config"])
	summary := &ScaleCheckpointStorage{
		Type:     strings.ToLower(runtimeStorageFirstNonEmpty(asString(storage["type"]), asString(pluginConfig["storage.type"]))),
		Location: asString(pluginConfig["fs.defaultFS"]),
	}
	location := strings.ToLower(strings.TrimSpace(summary.Location))
	switch {
	case strings.HasPrefix(location, "file:"), strings.HasPrefix(location, "/"):
		summary.Local = true
	case summary.Type == "localfile":
		summary.Local = true
	}
	if summary.Type == "" {
		summary.Type = "unknown"
	}
	return summary
}

func scaleOutSteps(mode ScaleMode, loopback bool) []ScaleStepName {
	steps := []ScaleStepName{ScaleStepValidatePlan}
	if loopback {
		steps = append(steps, ScaleStepFixLoopback)
	}
	if mode == ScaleModeInstall {
		return append(steps, ScaleStepInstallFiles, ScaleStepRegisterNode, ScaleStepApplyConfigs, ScaleStepReplayPlugins,
			ScaleStepStartNode, ScaleStepPatchMembers, ScaleStepVerifyMembership)
	}
	return append(steps, ScaleStepRegisterNode, ScaleStepApplyConfigs, ScaleStepReplayPlugins, ScaleStepPatchMembers, ScaleStepVerifyMembership)
}

// PlanScaleIn 计算缩容计划（只读）：最后节点 / 最后在线 master 阻断，运行中作业需强确认，java-proxy 承载节点被移除时规划跟随。
// PlanScaleIn computes a scale-in plan (read-only): removing the last node or the last online master is blocked,
// running jobs require a strong acknowledgement and the java-proxy follows when its host node is removed.
func (s *ScaleService) PlanScaleIn(ctx context.Context, clusterID uint, nodeID uint, lang string) (*ScalePlan, error) {
	if err := s.ensureRunnable(); err != nil {
		return nil, err
	}
	lang = normalizeScaleLang(lang)
	node, err := s.svc.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node.ClusterID != clusterID {
		return nil, ErrNodeNotFound
	}
	view, err := s.loadScaleView(ctx, clusterID, true)
	if err != nil {
		return nil, err
	}
	host := view.hosts[node.HostID]
	plan := &ScalePlan{
		Kind:           ScaleKindIn,
		ClusterID:      clusterID,
		DeploymentMode: view.cluster.DeploymentMode,
		ClusterVersion: view.cluster.Version,
		Lang:           lang,
		Target: ScaleTarget{
			NodeID:        node.ID,
			HostID:        node.HostID,
			HostName:      hostLabel(host, node.HostID),
			Role:          node.Role,
			InstallDir:    node.InstallDir,
			HazelcastPort: node.HazelcastPort,
			APIPort:       node.APIPort,
			WorkerPort:    node.WorkerPort,
			Address:       nodeAddress(node, host),
		},
	}
	if host != nil {
		plan.Target.HostIP = strings.TrimSpace(host.IPAddress)
	}

	all := make([]clustermembers.Member, 0, len(view.nodes))
	remaining := make([]clustermembers.Member, 0, len(view.nodes))
	onlineMasters := 0
	masters := 0
	for _, other := range view.nodes {
		member := scaleMember(other, view.hosts[other.HostID])
		all = append(all, member)
		if other.ID == node.ID {
			continue
		}
		remaining = append(remaining, member)
		if isMasterCapable(other.Role) {
			masters++
			if s.hostReady(view.hosts[other.HostID]) {
				onlineMasters++
			}
		}
	}
	plan.MembersBefore = clustermembers.Compute(string(view.cluster.DeploymentMode), all, nil, nil)
	plan.MembersAfter = clustermembers.Compute(string(view.cluster.DeploymentMode), remaining, nil, nil)

	if len(view.nodes) <= 1 {
		plan.addCheck(ScaleCheck{Name: "last_node", Status: ScaleCheckFailed, Blocking: true,
			Message: scaleText(lang, "不能移除集群最后一个节点，请改为删除集群", "the last node of a cluster cannot be removed; delete the cluster instead")})
	} else {
		plan.addCheck(ScaleCheck{Name: "last_node", Status: ScaleCheckPassed, Blocking: true,
			Message: scaleText(lang, fmt.Sprintf("移除后剩余 %d 个节点", len(view.nodes)-1), fmt.Sprintf("%d node(s) remain after removal", len(view.nodes)-1))})
	}
	masterCheck := ScaleCheck{Name: "last_master", Blocking: true}
	switch {
	case masters == 0:
		masterCheck.Status = ScaleCheckFailed
		masterCheck.Message = scaleText(lang, "移除后集群将没有 master 节点", "the cluster would have no master node after removal")
	case onlineMasters == 0:
		masterCheck.Status = ScaleCheckFailed
		masterCheck.Message = scaleText(lang, "移除后集群将没有在线的 master 节点", "the cluster would have no online master node after removal")
	default:
		masterCheck.Status = ScaleCheckPassed
		masterCheck.Message = scaleText(lang, fmt.Sprintf("移除后仍有 %d 个在线 master", onlineMasters), fmt.Sprintf("%d online master(s) remain after removal", onlineMasters))
	}
	plan.addCheck(masterCheck)

	agentCheck := ScaleCheck{Name: "target_agent_online", Blocking: true, Target: hostLabel(host, node.HostID)}
	if s.hostReady(host) {
		agentCheck.Status = ScaleCheckPassed
		agentCheck.Message = scaleText(lang, "目标主机 Agent 在线，可停止节点进程", "the target host Agent is online and can stop the node process")
	} else {
		agentCheck.Status = ScaleCheckFailed
		agentCheck.Message = scaleText(lang, "目标主机 Agent 离线，无法停止节点进程；可使用“仅移除记录（强制）”", "the target host Agent is offline and cannot stop the node process; use \"remove record only (force)\" instead")
	}
	plan.addCheck(agentCheck)
	s.addSharedProcessCheck(ctx, plan, node, lang)

	addMemberAddressChecks(plan, remaining, false, lang)
	s.fillRunningJobs(ctx, plan, lang)
	plan.CheckpointStorage = checkpointStorageFromConfigs(view.configs)
	if !plan.RunningJobsKnown || len(plan.RunningJobs) > 0 {
		plan.RequiresJobsAck = true
		if plan.CheckpointStorage != nil && plan.CheckpointStorage.Local {
			plan.addCheck(ScaleCheck{Name: "checkpoint_storage", Status: ScaleCheckWarning, Target: plan.CheckpointStorage.Location,
				Message: scaleText(lang,
					"checkpoint 使用本地文件存储：作业 failover 到其他节点后无法从 checkpoint 恢复，可能从头运行或失败",
					"checkpoints use local file storage: jobs failed over to other nodes cannot restore from checkpoints and may restart from scratch or fail")})
		}
	}

	if follow := s.planJavaProxyFollow(ctx, clusterID, node, view); follow != nil {
		plan.JavaProxyFollow = follow
		plan.addCheck(ScaleCheck{Name: "java_proxy_follow", Status: ScaleCheckPassed, Target: follow.ToHostName,
			Message: scaleText(lang,
				fmt.Sprintf("该节点承载 java-proxy，移除后将在 %s 上启动并校验健康", follow.ToHostName),
				fmt.Sprintf("this node hosts the java-proxy; it will be started on %s and health-checked after removal", follow.ToHostName))})
	}

	plan.Steps = []ScaleStepName{ScaleStepValidatePlan, ScaleStepStopNode, ScaleStepPatchMembers, ScaleStepCleanupRecords, ScaleStepRefreshMonitor}
	if plan.JavaProxyFollow != nil {
		plan.Steps = append(plan.Steps, ScaleStepFollowJavaProxy)
	}
	plan.Steps = append(plan.Steps, ScaleStepVerify)
	plan.CanSubmit = len(plan.BlockingReasons) == 0
	plan.normalize()
	plan.Fingerprint = scalePlanFingerprint(plan)
	return plan, nil
}

// addSharedProcessCheck 检查被移除节点的进程是否同时登记在其他集群：同一主机上「安装目录 + 角色」相同即为同一受管进程
// （processidentity.ManagedName）。停止它会让其他集群失去该节点，因此阻断缩容；只想从本集群注销时应使用「仅移除记录」。
// addSharedProcessCheck checks whether the removed node's process is also registered in another cluster: on the same
// host, the same install directory + role is the same managed process (processidentity.ManagedName). Stopping it
// would take the node away from the other cluster, so scale-in is blocked; to only unregister it from this cluster,
// use "remove record only".
func (s *ScaleService) addSharedProcessCheck(ctx context.Context, plan *ScalePlan, node *ClusterNode, lang string) {
	shared := s.sharedProcessClusters(ctx, node)
	if len(shared) == 0 {
		return
	}
	plan.addCheck(ScaleCheck{
		Name:     "shared_process",
		Status:   ScaleCheckFailed,
		Blocking: true,
		Target:   strings.Join(shared, ","),
		Message: scaleText(lang,
			fmt.Sprintf("该节点进程（%s）同时登记在集群 %s，缩容会停止该进程并影响这些集群；若只想从本集群注销，请使用“仅移除记录”", node.InstallDir, strings.Join(shared, "、")),
			fmt.Sprintf("this node's process (%s) is also registered in cluster(s) %s; scaling in would stop it and affect them. To only unregister it from this cluster, use \"remove record only\"", node.InstallDir, strings.Join(shared, ", "))),
	})
}

// sharedProcessClusters 返回同一主机上与该节点为同一受管进程（cleanInstallDir + 角色 → ManagedName）的其他集群名，已排序。
// sharedProcessClusters returns the names of other clusters on the same host that register the same managed process as
// the node (cleanInstallDir + role → ManagedName), sorted.
func (s *ScaleService) sharedProcessClusters(ctx context.Context, node *ClusterNode) []string {
	shared := make([]string, 0)
	if node == nil {
		return shared
	}
	hostNodes, err := s.svc.repo.GetNodesByHostID(ctx, node.HostID)
	if err != nil {
		return shared
	}
	// 目录先规范化（去掉结尾斜杠等），同一物理目录的不同写法视为同一进程。
	// Normalize directories first (trailing slashes etc.) so different spellings of one directory are one process.
	identity := processidentity.ManagedName(cleanInstallDir(node.InstallDir), string(node.Role))
	for _, other := range hostNodes {
		if other == nil || other.ID == node.ID || other.ClusterID == node.ClusterID {
			continue
		}
		if processidentity.ManagedName(cleanInstallDir(other.InstallDir), string(other.Role)) != identity {
			continue
		}
		label := fmt.Sprintf("#%d", other.ClusterID)
		if otherCluster, getErr := s.svc.repo.GetByID(ctx, other.ClusterID, false); getErr == nil && otherCluster != nil && strings.TrimSpace(otherCluster.Name) != "" {
			label = otherCluster.Name
		}
		shared = append(shared, label)
	}
	sort.Strings(shared)
	return shared
}

// planJavaProxyFollow 判断被移除节点是否为当前 java-proxy 承载节点（与 pickSTXJavaProxyNode 规则一致），是则选出下一个承载节点。
// planJavaProxyFollow checks whether the removed node currently hosts the java-proxy (same rule as
// pickSTXJavaProxyNode) and, if so, picks the next host node.
func (s *ScaleService) planJavaProxyFollow(ctx context.Context, clusterID uint, removed *ClusterNode, view *scaleView) *ScaleJavaProxyFollow {
	current, _, err := s.svc.pickSTXJavaProxyNode(ctx, clusterID)
	if err != nil || current == nil || current.ID != removed.ID {
		return nil
	}
	next, nextHost := s.pickJavaProxyNodeExcluding(ctx, clusterID, removed.ID)
	if next == nil {
		return nil
	}
	return &ScaleJavaProxyFollow{
		FromNodeID:     removed.ID,
		FromHostID:     removed.HostID,
		FromHostName:   hostLabel(view.hosts[removed.HostID], removed.HostID),
		FromInstallDir: removed.InstallDir,
		ToNodeID:       next.ID,
		ToHostID:       next.HostID,
		ToHostName:     hostLabel(nextHost, next.HostID),
	}
}

// pickJavaProxyNodeExcluding 按 pickSTXJavaProxyNode 的顺序选出排除指定节点后的首个在线 master。
// pickJavaProxyNodeExcluding picks the first online master in pickSTXJavaProxyNode order, excluding one node.
func (s *ScaleService) pickJavaProxyNodeExcluding(ctx context.Context, clusterID uint, excludedNodeID uint) (*NodeInfo, *HostInfo) {
	nodes, err := s.svc.GetNodes(ctx, clusterID)
	if err != nil {
		return nil, nil
	}
	for _, node := range nodes {
		if node == nil || node.ID == excludedNodeID || !node.IsOnline || !isMasterCapable(node.Role) {
			continue
		}
		host, err := s.svc.hostProvider.GetHostByID(ctx, node.HostID)
		if err != nil || !s.hostReady(host) {
			continue
		}
		return node, host
	}
	return nil, nil
}

// scaleFingerprintInput 是计划指纹的规范输入：只包含决定执行行为的字段，与语言和检查文案无关。
// scaleFingerprintInput is the canonical plan fingerprint input: only fields that drive execution, independent of
// language and check texts.
type scaleFingerprintInput struct {
	Kind            ScaleKind `json:"kind"`
	Mode            ScaleMode `json:"mode"`
	ClusterID       uint      `json:"cluster_id"`
	NodeID          uint      `json:"node_id"`
	HostID          uint      `json:"host_id"`
	Role            NodeRole  `json:"role"`
	InstallDir      string    `json:"install_dir"`
	HazelcastPort   int       `json:"hazelcast_port"`
	APIPort         int       `json:"api_port"`
	WorkerPort      int       `json:"worker_port"`
	MembersBefore   []string  `json:"members_before"`
	MembersAfter    []string  `json:"members_after"`
	LoopbackNodeIDs []uint    `json:"loopback_node_ids"`
	Plugins         []string  `json:"plugins"`
	HasRunningJobs  bool      `json:"has_running_jobs"`
	JobsKnown       bool      `json:"jobs_known"`
	JavaProxyFrom   uint      `json:"java_proxy_from"`
	JavaProxyTo     uint      `json:"java_proxy_to"`
}

// scalePlanFingerprint 计算计划指纹；扩容只有存在回环节点时才把运行中作业纳入（作业只影响回环重启护栏）。
// 作业只取「是否存在」与「是否可确认」，不取作业 ID：批处理作业在预检与提交之间结束不应迫使用户重新预检，
// 而从无到有会改变指纹，要求用户重新确认风险。
// scalePlanFingerprint computes the plan fingerprint; scale-out includes running jobs only when loopback nodes
// exist (jobs only affect the loopback restart guardrail). Jobs contribute only "any running" and "known", not
// job IDs: a batch job finishing between precheck and submit must not force a new precheck, while going from
// none to some changes the fingerprint and requires the risk to be acknowledged again.
func scalePlanFingerprint(plan *ScalePlan) string {
	input := scaleFingerprintInput{
		Kind:            plan.Kind,
		Mode:            plan.Mode,
		ClusterID:       plan.ClusterID,
		NodeID:          plan.Target.NodeID,
		HostID:          plan.Target.HostID,
		Role:            plan.Target.Role,
		InstallDir:      plan.Target.InstallDir,
		HazelcastPort:   plan.Target.HazelcastPort,
		APIPort:         plan.Target.APIPort,
		WorkerPort:      plan.Target.WorkerPort,
		MembersBefore:   append([]string{}, plan.MembersBefore...),
		MembersAfter:    append([]string{}, plan.MembersAfter...),
		LoopbackNodeIDs: []uint{},
		Plugins:         []string{},
	}
	for _, node := range plan.LoopbackNodes {
		input.LoopbackNodeIDs = append(input.LoopbackNodeIDs, node.NodeID)
	}
	for _, item := range plan.PluginsToReplay {
		input.Plugins = append(input.Plugins, item.Name+"@"+item.Version+"#"+strings.Join(item.ProfileKeys, "+"))
	}
	if plan.Kind == ScaleKindIn || len(plan.LoopbackNodes) > 0 {
		input.JobsKnown = plan.RunningJobsKnown
		input.HasRunningJobs = len(plan.RunningJobs) > 0
	}
	if plan.JavaProxyFollow != nil {
		input.JavaProxyFrom = plan.JavaProxyFollow.FromNodeID
		input.JavaProxyTo = plan.JavaProxyFollow.ToNodeID
	}
	sort.Slice(input.LoopbackNodeIDs, func(i, j int) bool { return input.LoopbackNodeIDs[i] < input.LoopbackNodeIDs[j] })
	sort.Strings(input.Plugins)
	content, _ := json.Marshal(input)
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// isIPAddress 判断主机字符串是否为 IP（成员核对时主机名无法与 Hazelcast 上报的 IP 直接比较）。
// isIPAddress reports whether a host string is an IP (host names cannot be compared with Hazelcast-reported IPs).
func isIPAddress(host string) bool {
	return net.ParseIP(strings.Trim(strings.TrimSpace(host), "[]")) != nil
}
