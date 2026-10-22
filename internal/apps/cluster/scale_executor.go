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
	"sort"
	"strconv"
	"strings"
	"time"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/clustermembers"
	"github.com/LeonYoah/stx/internal/logger"
)

// scaleStepOutcome 是单个步骤的执行结果。
// scaleStepOutcome is the result of executing one step.
type scaleStepOutcome struct {
	status  ScaleStepStatus
	message string
	err     error
	result  any
}

func stepSucceeded(message string, result any) scaleStepOutcome {
	return scaleStepOutcome{status: ScaleStepSucceeded, message: message, result: result}
}

func stepPartial(message string, result any) scaleStepOutcome {
	return scaleStepOutcome{status: ScaleStepPartial, message: message, result: result}
}

func stepSkipped(message string) scaleStepOutcome {
	return scaleStepOutcome{status: ScaleStepSkipped, message: message}
}

func stepFailed(err error, result any) scaleStepOutcome {
	return scaleStepOutcome{status: ScaleStepFailed, err: err, result: result}
}

// runOperation 按计划顺序执行尚未完成的步骤，每步前后持久化状态；某步失败即停止，操作进入 failed 等待重试。
// runOperation executes the not-yet-finished steps in plan order, persisting state before and after each; the first
// failure stops the run and leaves the operation failed for retry.
func (s *ScaleService) runOperation(ctx context.Context, operationID uint) {
	op, err := s.repo.GetByID(ctx, operationID)
	if err != nil {
		logger.ErrorF(ctx, "[ClusterScale] 读取伸缩操作失败 / Failed to load scaling operation: op=%d, err=%v", operationID, err)
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.ErrorF(ctx, "[ClusterScale] 伸缩操作执行异常 / Scaling operation panicked: op=%d, panic=%v", op.ID, recovered)
			s.finishOperation(ctx, op, fmt.Errorf("internal error: %v", recovered))
		}
	}()

	plan, err := op.plan()
	if err != nil {
		s.finishOperation(ctx, op, err)
		return
	}
	lang := normalizeScaleLang(op.Lang)
	steps := op.steps()
	var runErr error
	for i := range steps {
		if steps[i].Status.isDone() {
			continue
		}
		started := s.now()
		steps[i].Status = ScaleStepRunning
		steps[i].Attempts++
		steps[i].StartedAt = &started
		steps[i].FinishedAt = nil
		steps[i].Error = ""
		steps[i].Message = ""
		steps[i].Result = nil
		op.CurrentStep = steps[i].Name
		op.setSteps(steps)
		if err := s.repo.Save(ctx, op); err != nil {
			runErr = err
			break
		}
		logger.InfoF(ctx, "[ClusterScale] 开始步骤 / Step started: op=%d, step=%s, attempt=%d", op.ID, steps[i].Name, steps[i].Attempts)

		outcome := s.runStep(ctx, op, plan, steps[i].Name, lang)

		finished := s.now()
		steps[i].Status = outcome.status
		steps[i].FinishedAt = &finished
		steps[i].Message = truncateScaleText(outcome.message, 2000)
		if outcome.err != nil {
			steps[i].Error = truncateScaleText(outcome.err.Error(), 2000)
		}
		if outcome.result != nil {
			if content, marshalErr := json.Marshal(outcome.result); marshalErr == nil {
				steps[i].Result = content
			}
		}
		op.setSteps(steps)
		if err := s.repo.Save(ctx, op); err != nil {
			runErr = err
			break
		}
		logger.InfoF(ctx, "[ClusterScale] 步骤结束 / Step finished: op=%d, step=%s, status=%s, message=%s, err=%v",
			op.ID, steps[i].Name, outcome.status, steps[i].Message, outcome.err)
		if outcome.status == ScaleStepFailed {
			runErr = outcome.err
			if runErr == nil {
				runErr = errors.New(firstNonEmpty(outcome.message, string(steps[i].Name)+" failed"))
			}
			runErr = fmt.Errorf("%s: %w", steps[i].Name, runErr)
			break
		}
	}
	s.finishOperation(ctx, op, runErr)
}

// finishOperation 写入操作终态并同步公共执行记录。
// finishOperation writes the operation's terminal state and syncs the shared execution.
func (s *ScaleService) finishOperation(ctx context.Context, op *ClusterScaleOperation, runErr error) {
	now := s.now()
	op.FinishedAt = &now
	if runErr != nil {
		op.Status = ScaleOperationFailed
		op.Error = truncateScaleText(runErr.Error(), 2000)
	} else {
		op.Status = ScaleOperationSucceeded
		op.Error = ""
	}
	if err := s.repo.Save(ctx, op); err != nil {
		logger.ErrorF(ctx, "[ClusterScale] 保存伸缩操作终态失败 / Failed to save scaling operation result: op=%d, err=%v", op.ID, err)
	}
	s.finishExecution(ctx, op, runErr)
	if runErr != nil {
		logger.WarnF(ctx, "[ClusterScale] 伸缩操作失败，可从失败步骤重试 / Scaling operation failed and can be retried: op=%d, err=%v", op.ID, runErr)
		return
	}
	logger.InfoF(ctx, "[ClusterScale] 伸缩操作完成 / Scaling operation succeeded: cluster=%d, op=%d, kind=%s", op.ClusterID, op.ID, op.Kind)
}

// runStep 分派单个步骤。每个步骤都按“可重复执行、结果一致”的原则实现。
// runStep dispatches one step. Every step is implemented to be safely re-runnable with the same result.
func (s *ScaleService) runStep(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, name ScaleStepName, lang string) scaleStepOutcome {
	switch name {
	case ScaleStepValidatePlan:
		return s.stepValidatePlan(ctx, op, plan, lang)
	case ScaleStepFixLoopback:
		return s.stepFixLoopback(ctx, op, plan, lang)
	case ScaleStepInstallFiles:
		return s.stepInstallFiles(ctx, op, plan, lang)
	case ScaleStepRegisterNode:
		return s.stepRegisterNode(ctx, op, plan, lang)
	case ScaleStepApplyConfigs:
		return s.stepApplyConfigs(ctx, op, plan, lang)
	case ScaleStepReplayPlugins:
		return s.stepReplayPlugins(ctx, op, plan, lang)
	case ScaleStepStartNode:
		return s.stepStartNode(ctx, op, plan, lang)
	case ScaleStepPatchMembers:
		return s.stepPatchMembers(ctx, op, plan, lang)
	case ScaleStepVerifyMembership:
		return s.stepVerifyMembership(ctx, op, plan, lang)
	case ScaleStepStopNode:
		return s.stepStopNode(ctx, op, plan, lang)
	case ScaleStepCleanupRecords:
		return s.stepCleanupRecords(ctx, op, plan, lang)
	case ScaleStepRefreshMonitor:
		return s.stepRefreshMonitor(ctx, op, plan, lang)
	case ScaleStepFollowJavaProxy:
		return s.stepFollowJavaProxy(ctx, op, plan, lang)
	case ScaleStepVerify:
		return s.stepVerifyScaleIn(ctx, op, plan, lang)
	default:
		return stepFailed(fmt.Errorf("unknown step %s", name), nil)
	}
}

// scaleOpRef 是写入配置版本说明的伸缩操作引用。
// scaleOpRef is the scaling operation reference written into config version comments.
func scaleOpRef(op *ClusterScaleOperation) string {
	return fmt.Sprintf("scale-op-%d", op.ID)
}

// sleep 等待一个轮询间隔，上下文取消时提前返回。
// sleep waits one poll interval and returns early when the context is cancelled.
func (s *ScaleService) sleep(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// stepValidatePlan 重算计划并与提交时的指纹比对；变化、出现阻断项或确认缺失时失败，要求重新预检。
// stepValidatePlan recomputes the plan and compares it with the submitted fingerprint; changes, blocking items or a
// missing acknowledgement fail the step and require a new precheck.
func (s *ScaleService) stepValidatePlan(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	var (
		fresh *ScalePlan
		err   error
	)
	switch plan.Kind {
	case ScaleKindOut:
		fresh, err = s.PlanScaleOut(ctx, op.ClusterID, &ScaleOutRequest{
			HostID:     plan.Target.HostID,
			Mode:       plan.Mode,
			Role:       plan.Target.Role,
			InstallDir: plan.Target.InstallDir,
			Ports: ScaleNodePorts{
				HazelcastPort: plan.Target.HazelcastPort,
				APIPort:       plan.Target.APIPort,
				WorkerPort:    plan.Target.WorkerPort,
			},
		}, lang)
	case ScaleKindIn:
		fresh, err = s.PlanScaleIn(ctx, op.ClusterID, plan.Target.NodeID, lang)
	default:
		err = ErrScaleOperationCorrupted
	}
	if err != nil {
		return stepFailed(err, nil)
	}
	if err := validateSubmittedPlan(fresh, plan.Fingerprint, op.LoopbackRestartAck, op.JobsRiskAck); err != nil {
		return stepFailed(err, map[string]any{"fingerprint": fresh.Fingerprint, "blocking_reasons": fresh.BlockingReasons})
	}
	result := map[string]any{"fingerprint": fresh.Fingerprint}
	if plan.Kind == ScaleKindOut {
		// 记录操作开始时（任何重启之前）正在运行的现有节点，作为入群校验基线。
		// Record the existing nodes running when the operation starts (before any restart) as the join baseline.
		view, viewErr := s.refreshNodeStatuses(ctx, op.ClusterID)
		if viewErr != nil {
			return stepFailed(viewErr, nil)
		}
		result[scaleBaselineMembersKey] = baselineRunningAddresses(view)
	}
	return stepSucceeded(scaleText(lang, "计划校验通过", "plan validated"), result)
}

// scaleBaselineMembersKey 是 validate_plan 步骤结果中入群校验基线的键。
// scaleBaselineMembersKey is the key of the join baseline in the validate_plan step result.
const scaleBaselineMembersKey = "baseline_members"

// baselineRunningAddresses 返回进程运行中、主机在线且地址为 IP 的节点成员地址（主机名无法与 Hazelcast 上报的 IP 比对）。
// baselineRunningAddresses returns member addresses of running nodes on online hosts with IP addresses (host names
// cannot be compared with the IPs Hazelcast reports).
func baselineRunningAddresses(view *scaleView) []string {
	addresses := make([]string, 0, len(view.nodes))
	for _, node := range view.nodes {
		host := view.hosts[node.HostID]
		if node.Status != NodeStatusRunning || host == nil || !isIPAddress(host.IPAddress) {
			continue
		}
		if address := nodeAddress(node, host); address != "" && !clustermembers.IsLoopback(address) {
			addresses = append(addresses, address)
		}
	}
	sort.Strings(addresses)
	return addresses
}

// joinBaseline 读取 validate_plan 记录的入群基线；缺失（升级前创建的操作）时返回 false。
// joinBaseline reads the join baseline recorded by validate_plan; returns false when missing (operations created
// before the upgrade).
func joinBaseline(op *ClusterScaleOperation) ([]string, bool) {
	for _, step := range op.steps() {
		if step.Name != ScaleStepValidatePlan || len(step.Result) == 0 {
			continue
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(step.Result, &payload); err != nil {
			return nil, false
		}
		raw, ok := payload[scaleBaselineMembersKey]
		if !ok {
			return nil, false
		}
		var members []string
		if err := json.Unmarshal(raw, &members); err != nil {
			return nil, false
		}
		return members, true
	}
	return nil, false
}

// allAddressesIn 判断 expected 中的每个地址都出现在成员列表中。
// allAddressesIn reports whether every expected address appears in the member list.
func allAddressesIn(members []string, expected []string) bool {
	for _, address := range expected {
		if !containsAddress(members, address) {
			return false
		}
	}
	return true
}

// stepFixLoopback 修正回环成员：补登记缺失的节点配置 → 修补模板与节点成员列表 → 逐个重启回环节点并等待其以可路由地址恢复。
// stepFixLoopback fixes loopback members: register missing node configs → patch template and node member lists →
// restart loopback nodes one by one and wait until each recovers with a routable address.
func (s *ScaleService) stepFixLoopback(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	if len(plan.LoopbackNodes) == 0 {
		return stepSkipped(scaleText(lang, "没有回环成员", "no loopback members"))
	}
	view, err := s.loadScaleView(ctx, op.ClusterID, true)
	if err != nil {
		return stepFailed(err, nil)
	}
	nodesByID := make(map[uint]*ClusterNode, len(view.nodes))
	for _, node := range view.nodes {
		nodesByID[node.ID] = node
	}
	memberTypes := make(map[appconfig.ConfigType]struct{})
	for _, configType := range appconfig.MemberListConfigTypes(string(view.cluster.DeploymentMode)) {
		memberTypes[configType] = struct{}{}
	}
	hostHasMemberConfig := make(map[uint]bool)
	for _, config := range view.configs {
		if config == nil || config.IsTemplate || config.HostID == nil {
			continue
		}
		if _, ok := memberTypes[config.ConfigType]; ok {
			hostHasMemberConfig[*config.HostID] = true
		}
	}

	// 回环节点没有节点配置记录时先导入其现有文件，否则修补不会写到它的磁盘。
	// Import the existing files of loopback nodes without config records first; otherwise patching never reaches them.
	loopbackHosts := make(map[uint]bool)
	for _, loopback := range plan.LoopbackNodes {
		node := nodesByID[loopback.NodeID]
		if node == nil {
			return stepFailed(fmt.Errorf("loopback node %d no longer exists", loopback.NodeID), nil)
		}
		loopbackHosts[node.HostID] = true
		if !hostHasMemberConfig[node.HostID] {
			if _, err := s.config.RegisterHostConfigsFromNode(ctx, op.ClusterID, node.HostID, node.InstallDir, op.CreatedBy); err != nil {
				return stepFailed(fmt.Errorf("register configs of node %d: %w", node.ID, err), nil)
			}
		}
	}

	members := clusterMemberList(view, 0)
	patch, err := s.config.PatchMemberList(ctx, op.ClusterID, string(view.cluster.DeploymentMode), members, scaleOpRef(op), op.CreatedBy)
	if err != nil {
		return stepFailed(err, patch)
	}
	for _, item := range patch.Items {
		if item.HostID != nil && loopbackHosts[*item.HostID] && (item.Status == appconfig.ConfigChangePendingPush || item.Status == appconfig.ConfigChangeFailed) {
			return stepFailed(fmt.Errorf("member list of loopback host %d was not written (%s): %s", *item.HostID, item.Status, item.Message), patch)
		}
	}

	restarted := make([]map[string]any, 0, len(plan.LoopbackNodes))
	for _, loopback := range plan.LoopbackNodes {
		node := nodesByID[loopback.NodeID]
		host := view.hosts[node.HostID]
		address := nodeAddress(node, host)
		if s.memberHealthy(s.probeMembers(ctx, node, host), address, host) {
			restarted = append(restarted, map[string]any{"node_id": node.ID, "restarted": false})
			continue
		}
		result, err := s.svc.RestartNode(ctx, op.ClusterID, node.ID)
		if err != nil {
			return stepFailed(fmt.Errorf("restart node %d: %w", node.ID, err), map[string]any{"patch": patch, "restarted": restarted})
		}
		if result != nil && !result.Success {
			return stepFailed(fmt.Errorf("restart node %d: %s", node.ID, result.Message), map[string]any{"patch": patch, "restarted": restarted})
		}
		probe, ok := s.waitForMember(ctx, node, host, address, s.timing.loopbackNodeTimeout)
		if !ok {
			return stepFailed(fmt.Errorf("node %d did not recover with a routable member address within %s (members: %s)", node.ID, s.timing.loopbackNodeTimeout, strings.Join(probe.members, ",")),
				map[string]any{"patch": patch, "restarted": restarted})
		}
		restarted = append(restarted, map[string]any{"node_id": node.ID, "restarted": true, "members": probe.members})
	}
	return stepSucceeded(scaleText(lang,
		fmt.Sprintf("已修正 %d 个回环成员节点并恢复", len(plan.LoopbackNodes)),
		fmt.Sprintf("fixed and recovered %d loopback member node(s)", len(plan.LoopbackNodes))),
		map[string]any{"patch": patch, "restarted": restarted})
}

// memberHealthy 判断节点运行时成员不含回环地址且包含自身可路由地址（主机名无法与 IP 比较时只看前者）。
// memberHealthy reports whether a node's runtime members contain no loopback address and include its own routable
// address (only the former when a host name cannot be compared with IPs).
func (s *ScaleService) memberHealthy(probe memberProbe, address string, host *HostInfo) bool {
	if !probe.known {
		return false
	}
	for _, member := range probe.members {
		if clustermembers.IsLoopback(member) {
			return false
		}
	}
	if host != nil && !isIPAddress(host.IPAddress) {
		return true
	}
	return containsAddress(probe.members, address)
}

// waitForMember 轮询节点运行时成员直到健康或超时。
// waitForMember polls a node's runtime members until healthy or timed out.
func (s *ScaleService) waitForMember(ctx context.Context, node *ClusterNode, host *HostInfo, address string, timeout time.Duration) (memberProbe, bool) {
	deadline := s.now().Add(timeout)
	var probe memberProbe
	for {
		probe = s.probeMembers(ctx, node, host)
		if s.memberHealthy(probe, address, host) {
			return probe, true
		}
		if !s.now().Before(deadline) {
			return probe, false
		}
		if err := s.sleep(ctx, s.timing.pollInterval); err != nil {
			return probe, false
		}
	}
}

// clusterMemberList 计算当前集群节点（可排除一个节点）的完整成员列表。
// clusterMemberList computes the full member list of current cluster nodes, optionally excluding one node.
func clusterMemberList(view *scaleView, excludeNodeID uint) []string {
	members := make([]clustermembers.Member, 0, len(view.nodes))
	for _, node := range view.nodes {
		if excludeNodeID != 0 && node.ID == excludeNodeID {
			continue
		}
		members = append(members, scaleMember(node, view.hosts[node.HostID]))
	}
	return clustermembers.Compute(string(view.cluster.DeploymentMode), members, nil, nil)
}

// stepInstallFiles 调用安装器（DeferNodeStart）安装 SeaTunnel 并登记节点，不启动；轮询安装状态直到结束。
// stepInstallFiles installs SeaTunnel through the installer (DeferNodeStart) and registers the node without starting
// it, polling the installation status until it ends.
func (s *ScaleService) stepInstallFiles(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	if s.installer == nil {
		return stepFailed(fmt.Errorf("%w: installer is missing", ErrScaleNotConfigured), nil)
	}
	view, err := s.loadScaleView(ctx, op.ClusterID, false)
	if err != nil {
		return stepFailed(err, nil)
	}
	req := buildScaleInstallRequest(view, plan)
	started, err := s.installer.StartInstallation(ctx, req)
	if err != nil {
		return stepFailed(fmt.Errorf("start installation: %w", err), nil)
	}
	installID := ""
	if started != nil {
		installID = started.ID
	}
	deadline := s.now().Add(s.timing.installTimeout)
	for {
		snapshot, err := s.installer.GetInstallationStatusSnapshot(ctx, plan.Target.HostID)
		if err != nil {
			return stepFailed(fmt.Errorf("read installation status: %w", err), map[string]any{"installation_id": installID})
		}
		if installID != "" && snapshot.ID != installID {
			return stepFailed(fmt.Errorf("installation %s was superseded by %s", installID, snapshot.ID), map[string]any{"installation_id": installID})
		}
		result := map[string]any{
			"installation_id": snapshot.ID,
			"current_step":    snapshot.CurrentStep,
			"progress":        snapshot.Progress,
			"warnings":        nonNilStrings(snapshot.Warnings),
		}
		switch snapshot.Status {
		case installerapp.StepStatusSuccess:
			return stepSucceeded(scaleText(lang, "SeaTunnel 已安装并登记节点", "SeaTunnel installed and node registered"), result)
		case installerapp.StepStatusFailed:
			return stepFailed(errors.New(firstNonEmpty(snapshot.Error, snapshot.Message, "installation failed")), result)
		}
		if !s.now().Before(deadline) {
			return stepFailed(fmt.Errorf("installation did not finish within %s", s.timing.installTimeout), result)
		}
		if err := s.sleep(ctx, s.timing.pollInterval); err != nil {
			return stepFailed(err, result)
		}
	}
}

// buildScaleInstallRequest 构建扩容安装请求：版本固定为集群版本，成员地址为加入后的完整 IP 列表，DeferNodeStart=true。
// buildScaleInstallRequest builds the scale-out installation request: the version is the cluster version, member
// addresses are the complete IP list after joining and DeferNodeStart is true.
func buildScaleInstallRequest(view *scaleView, plan *ScalePlan) *installerapp.InstallationRequest {
	target := plan.Target
	masters := make([]string, 0, len(view.nodes)+1)
	workers := make([]string, 0, len(view.nodes)+1)
	seenMasters := make(map[string]struct{})
	seenWorkers := make(map[string]struct{})
	masterPort, workerPort := 0, 0
	add := func(role NodeRole, ip string, hazelcastPort int) {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		if view.cluster.DeploymentMode == DeploymentModeSeparated && role == NodeRoleWorker {
			if _, ok := seenWorkers[ip]; !ok {
				seenWorkers[ip] = struct{}{}
				workers = append(workers, ip)
			}
			workerPort = firstPositive(workerPort, hazelcastPort)
			return
		}
		if _, ok := seenMasters[ip]; !ok {
			seenMasters[ip] = struct{}{}
			masters = append(masters, ip)
		}
		masterPort = firstPositive(masterPort, hazelcastPort)
	}
	// 目标节点端口优先，其次是现有节点。
	// The target node's port wins, then existing nodes.
	add(target.Role, target.HostIP, target.HazelcastPort)
	for _, node := range view.nodes {
		if host := view.hosts[node.HostID]; host != nil {
			add(node.Role, host.IPAddress, node.HazelcastPort)
		}
	}
	if view.cluster.DeploymentMode == DeploymentModeHybrid {
		workerPort = firstPositive(target.WorkerPort, DefaultPorts.WorkerHazelcast)
	}
	// 地址稳定排序，与成员列表计算规则一致。
	// Sort addresses stably, consistent with the member-list computation.
	clustermembers.SortAddresses(masters)
	clustermembers.SortAddresses(workers)
	return &installerapp.InstallationRequest{
		HostID:          strconv.FormatUint(uint64(target.HostID), 10),
		ClusterID:       strconv.FormatUint(uint64(view.cluster.ID), 10),
		Version:         view.cluster.Version,
		InstallDir:      target.InstallDir,
		InstallMode:     installerapp.InstallModeOnline,
		DeploymentMode:  installerapp.DeploymentMode(view.cluster.DeploymentMode),
		NodeRole:        installerapp.NodeRole(target.Role),
		MasterAddresses: masters,
		WorkerAddresses: workers,
		ClusterPort:     firstPositive(masterPort, DefaultPorts.MasterHazelcast),
		WorkerPort:      firstPositive(workerPort, DefaultPorts.WorkerHazelcast),
		HTTPPort:        target.APIPort,
		DeferNodeStart:  true,
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// stepRegisterNode 按计划目标登记或刷新节点记录（安装模式校验安装器的登记并修正端口；纳管模式创建记录并探测进程）。
// stepRegisterNode registers or refreshes the node record from the plan target (install mode verifies the
// installer's registration and corrects ports; register mode creates the record and probes the process).
func (s *ScaleService) stepRegisterNode(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	target := plan.Target
	if err := s.svc.EnsureNodeForInstallation(ctx, op.ClusterID, target.HostID, string(target.Role), target.InstallDir, target.HazelcastPort, target.APIPort, target.WorkerPort); err != nil {
		return stepFailed(err, nil)
	}
	node, err := s.svc.repo.GetNodeByClusterAndHostAndRole(ctx, op.ClusterID, target.HostID, string(target.Role))
	if err != nil {
		return stepFailed(err, nil)
	}
	if node == nil {
		return stepFailed(fmt.Errorf("node for host %d role %s was not registered", target.HostID, target.Role), nil)
	}
	op.setNodeIDs([]uint{node.ID})
	if plan.Mode == ScaleModeRegister {
		s.svc.detectAndUpdateNodeProcess(ctx, node, node.HostID)
		s.svc.updateClusterStatusFromNodes(ctx, op.ClusterID)
	}
	return stepSucceeded(scaleText(lang, fmt.Sprintf("节点已登记（ID %d）", node.ID), fmt.Sprintf("node registered (ID %d)", node.ID)), map[string]any{"node_id": node.ID})
}

// targetNode 返回扩容操作登记的新节点。
// targetNode returns the new node registered by a scale-out operation.
func (s *ScaleService) targetNode(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan) (*ClusterNode, *HostInfo, error) {
	var node *ClusterNode
	if ids := op.nodeIDs(); len(ids) > 0 {
		found, err := s.svc.repo.GetNodeByID(ctx, ids[0])
		if err != nil {
			return nil, nil, err
		}
		node = found
	} else {
		found, err := s.svc.repo.GetNodeByClusterAndHostAndRole(ctx, op.ClusterID, plan.Target.HostID, string(plan.Target.Role))
		if err != nil {
			return nil, nil, err
		}
		if found == nil {
			return nil, nil, ErrNodeNotFound
		}
		node = found
	}
	host, err := s.svc.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil {
		return node, nil, err
	}
	return node, host, nil
}

// stepApplyConfigs 安装模式：以模板（成员列表为完整列表）生成该主机配置并写盘，再补登记其余文件；纳管模式：只登记节点现有配置。
// stepApplyConfigs: install mode renders this host's configs from templates (with the complete member list) and
// writes them, then registers the remaining files; register mode only registers the node's existing configs.
func (s *ScaleService) stepApplyConfigs(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	node, _, err := s.targetNode(ctx, op, plan)
	if err != nil {
		return stepFailed(err, nil)
	}
	if plan.Mode == ScaleModeRegister {
		registered, err := s.config.RegisterHostConfigsFromNode(ctx, op.ClusterID, node.HostID, node.InstallDir, op.CreatedBy)
		if err != nil {
			return stepFailed(err, registered)
		}
		return stepSucceeded(scaleText(lang,
			fmt.Sprintf("已登记节点现有配置 %d 项（不覆盖模板与节点文件）", len(registered.Items)),
			fmt.Sprintf("registered %d existing node config(s) (templates and node files untouched)", len(registered.Items))), registered)
	}

	view, err := s.loadScaleView(ctx, op.ClusterID, false)
	if err != nil {
		return stepFailed(err, nil)
	}
	members := clusterMemberList(view, 0)
	applied, err := s.config.ApplyTemplatesToHost(ctx, op.ClusterID, node.HostID, string(view.cluster.DeploymentMode), members, scaleOpRef(op), op.CreatedBy)
	if err != nil {
		return stepFailed(err, applied)
	}
	if applied.Partial() {
		return stepFailed(fmt.Errorf("%d config(s) were not written to the node before startup", applied.PendingCount+applied.FailedCount), applied)
	}
	registered, err := s.config.RegisterHostConfigsFromNode(ctx, op.ClusterID, node.HostID, node.InstallDir, op.CreatedBy)
	result := map[string]any{"applied": applied, "registered": registered}
	if err != nil {
		return stepPartial(scaleText(lang, "模板已下发，但登记其余配置失败："+err.Error(), "templates applied but registering other configs failed: "+err.Error()), result)
	}
	return stepSucceeded(scaleText(lang,
		fmt.Sprintf("已按集群模板写入 %d 项配置", len(applied.Items)),
		fmt.Sprintf("wrote %d config(s) from cluster templates", len(applied.Items))), result)
}

// stepReplayPlugins 把集群插件（含画像依赖）补齐到新节点；单个插件失败记为 partial，不阻断入群。
// stepReplayPlugins replays cluster plugins (with profile dependencies) to the new node; single plugin failures make
// the step partial without blocking the join.
func (s *ScaleService) stepReplayPlugins(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	if s.plugins == nil {
		return stepSkipped(scaleText(lang, "未配置插件服务", "plugin service is not configured"))
	}
	node, _, err := s.targetNode(ctx, op, plan)
	if err != nil {
		return stepFailed(err, nil)
	}
	report, err := s.plugins.ReplayClusterPluginsToHost(ctx, op.ClusterID, node.HostID, node.InstallDir)
	if err != nil {
		return stepFailed(err, report)
	}
	if report.Partial() {
		return stepPartial(scaleText(lang,
			fmt.Sprintf("%d 个插件补齐失败，可重试", report.FailedCount),
			fmt.Sprintf("%d plugin(s) failed to replay and can be retried", report.FailedCount)), report)
	}
	return stepSucceeded(scaleText(lang,
		fmt.Sprintf("已补齐 %d 个插件，%d 个已存在", report.InstalledCount, report.PresentCount),
		fmt.Sprintf("replayed %d plugin(s), %d already present", report.InstalledCount, report.PresentCount)), report)
}

// checkNodeProcess 通过 check_process 判断节点进程是否运行；known=false 表示无法判断。
// checkNodeProcess uses check_process to tell whether the node process runs; known=false means undetermined.
func (s *ScaleService) checkNodeProcess(ctx context.Context, node *ClusterNode, host *HostInfo) (running bool, known bool) {
	if node == nil || !s.hostReady(host) {
		return false, false
	}
	role := "hybrid"
	switch node.Role {
	case NodeRoleMaster:
		role = "master"
	case NodeRoleWorker:
		role = "worker"
	}
	success, _, err := s.svc.agentSender.SendCommand(ctx, host.AgentID, "check_process", map[string]string{
		"role":        role,
		"install_dir": node.InstallDir,
	})
	if err != nil {
		return false, false
	}
	return success, true
}

// stepStartNode 启动新节点；进程已运行时直接成功（重试幂等）。
// stepStartNode starts the new node; succeeds immediately when the process already runs (idempotent retry).
func (s *ScaleService) stepStartNode(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	node, host, err := s.targetNode(ctx, op, plan)
	if err != nil {
		return stepFailed(err, nil)
	}
	if running, known := s.checkNodeProcess(ctx, node, host); known && running {
		s.svc.detectAndUpdateNodeProcess(ctx, node, node.HostID)
		return stepSucceeded(scaleText(lang, "节点进程已在运行", "the node process is already running"), map[string]any{"node_id": node.ID, "started": false})
	}
	result, err := s.svc.StartNode(ctx, op.ClusterID, node.ID)
	if err != nil {
		return stepFailed(err, nil)
	}
	if result != nil && !result.Success {
		return stepFailed(errors.New(firstNonEmpty(result.Message, "start node failed")), result)
	}
	return stepSucceeded(scaleText(lang, "节点已启动", "node started"), map[string]any{"node_id": node.ID, "started": true})
}

// stepPatchMembers 以当前节点（缩容排除目标节点）计算完整成员列表，只修补模板与各节点的 member-list 并写盘，不重启。
// stepPatchMembers computes the complete member list from current nodes (excluding the target on scale-in), patches
// only the member-list of templates and node configs and writes them without restarting.
func (s *ScaleService) stepPatchMembers(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	view, err := s.loadScaleView(ctx, op.ClusterID, false)
	if err != nil {
		return stepFailed(err, nil)
	}
	exclude := uint(0)
	if plan.Kind == ScaleKindIn {
		exclude = plan.Target.NodeID
	}
	members := clusterMemberList(view, exclude)
	if len(members) == 0 {
		return stepFailed(errors.New("no routable member address left"), nil)
	}
	patched, err := s.config.PatchMemberList(ctx, op.ClusterID, string(view.cluster.DeploymentMode), members, scaleOpRef(op), op.CreatedBy)
	if err != nil {
		return stepFailed(err, patched)
	}
	if patched.Partial() {
		return stepPartial(scaleText(lang,
			fmt.Sprintf("成员列表已更新，%d 项配置推送失败（Agent 重连后自动重推）", patched.PendingCount+patched.FailedCount),
			fmt.Sprintf("member lists updated; %d config(s) failed to push (re-pushed when the Agent reconnects)", patched.PendingCount+patched.FailedCount)), patched)
	}
	return stepSucceeded(scaleText(lang,
		fmt.Sprintf("成员列表已更新为 %d 个地址（更新 %d 项，未变 %d 项）", len(members), patched.UpdatedCount, patched.UnchangedCount),
		fmt.Sprintf("member lists set to %d address(es) (%d updated, %d unchanged)", len(members), patched.UpdatedCount, patched.UnchangedCount)), patched)
}

// refreshNodeStatuses 刷新所有节点的进程状态并返回最新节点列表。
// refreshNodeStatuses refreshes every node's process status and returns the latest view.
func (s *ScaleService) refreshNodeStatuses(ctx context.Context, clusterID uint) (*scaleView, error) {
	view, err := s.loadScaleView(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	for _, node := range view.nodes {
		s.svc.detectAndUpdateNodeProcess(ctx, node, node.HostID)
	}
	return s.loadScaleView(ctx, clusterID, false)
}

// runningMemberCount 统计进程运行中且有地址的节点数（期望的 Hazelcast 成员数）。
// runningMemberCount counts running nodes with an address (the expected Hazelcast member count).
func runningMemberCount(view *scaleView, excludeNodeID uint) int {
	count := 0
	for _, node := range view.nodes {
		if node.ID == excludeNodeID || node.Status != NodeStatusRunning {
			continue
		}
		if nodeAddress(node, view.hosts[node.HostID]) != "" {
			count++
		}
	}
	return count
}

// stepVerifyMembership 校验新节点入群：其运行时成员须包含自身地址与操作开始时全部运行中的现有节点（validate_plan 记录的
// 基线；扩容不会停止现有节点，回环修正的重启也只是暂时离群），成员数不少于「基线 + 1」；随后刷新集群状态并下发监控跟踪列表。
// 不以校验时的数据库状态为准：刚重启的节点状态可能滞后，按其计数会把「新节点自成集群」误判为入群成功。
// stepVerifyMembership verifies the join: the new node's runtime members must include its own address and every
// existing node that was running when the operation started (the validate_plan baseline; scaling out never stops
// existing nodes and the loopback-fix restart is only temporary), with at least "baseline + 1" members; then the
// cluster status is refreshed and the monitor tracking list is pushed. The DB status at verification time is not
// used: a just-restarted node may lag, and counting by it once accepted a new node that formed its own cluster.
func (s *ScaleService) stepVerifyMembership(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	node, host, err := s.targetNode(ctx, op, plan)
	if err != nil {
		return stepFailed(err, nil)
	}
	view, err := s.refreshNodeStatuses(ctx, op.ClusterID)
	if err != nil {
		return stepFailed(err, nil)
	}
	address := nodeAddress(node, host)
	others, ok := joinBaseline(op)
	if !ok {
		// 兼容升级前创建的操作：退回到当前运行中的其他节点。
		// Operations created before the upgrade: fall back to the other nodes running now.
		others = make([]string, 0)
		for _, candidate := range baselineRunningAddresses(view) {
			if normalizeScaleAddress(candidate) != normalizeScaleAddress(address) {
				others = append(others, candidate)
			}
		}
	}
	expected := len(others) + 1
	deadline := s.now().Add(s.timing.membershipTimeout)
	var probe memberProbe
	for {
		probe = s.probeMembers(ctx, node, host)
		addressOK := containsAddress(probe.members, address) || (host != nil && !isIPAddress(host.IPAddress))
		// 只看到自己的新节点自成一个集群，不算入群。/ A new node that only sees itself formed its own cluster.
		joinedOK := allAddressesIn(probe.members, others)
		if probe.known && probe.count >= expected && addressOK && joinedOK {
			break
		}
		if !s.now().Before(deadline) {
			s.svc.updateClusterStatusFromNodes(ctx, op.ClusterID)
			hint := ""
			if plan.Mode == ScaleModeRegister {
				hint = scaleText(lang, "；纳管节点不会被重启，若其仍在独立集群中，请手动重启该节点以使新成员列表生效",
					"; registered nodes are not restarted, so if it still runs as a separate cluster, restart it manually to apply the new member list")
			}
			return stepFailed(fmt.Errorf("new node sees %d/%d members (%s)%s", probe.count, expected, strings.Join(probe.members, ","), hint),
				map[string]any{"expected": expected, "members": nonNilStrings(probe.members)})
		}
		if err := s.sleep(ctx, s.timing.pollInterval); err != nil {
			return stepFailed(err, nil)
		}
	}
	s.svc.updateClusterStatusFromNodes(ctx, op.ClusterID)
	s.svc.notifyClusterTopologyChanged(ctx, op.ClusterID)
	result := map[string]any{"expected": expected, "member_count": probe.count, "members": nonNilStrings(probe.members)}
	if s.monitor != nil {
		if err := s.monitor.PushClusterMonitorConfigToHost(ctx, op.ClusterID, node.HostID); err != nil {
			result["monitor_push_error"] = err.Error()
			return stepPartial(scaleText(lang,
				fmt.Sprintf("新节点已入群（%d 个成员），但下发监控配置失败：%v", probe.count, err),
				fmt.Sprintf("the new node joined (%d members) but pushing the monitor config failed: %v", probe.count, err)), result)
		}
	}
	return stepSucceeded(scaleText(lang,
		fmt.Sprintf("新节点已入群，集群共 %d 个成员", probe.count),
		fmt.Sprintf("the new node joined; the cluster has %d members", probe.count)), result)
}

// stepStopNode 停止待移除节点（Agent 在停止前标记手动停止，失败时清除，停止后取消跟踪）并确认进程已退出；失败时不删除记录。
// stepStopNode stops the node being removed (the Agent marks it manually stopped before stopping, clears the flag on
// failure and untracks it afterwards) and confirms the process exited; on failure the record is kept.
func (s *ScaleService) stepStopNode(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	node, err := s.svc.repo.GetNodeByID(ctx, plan.Target.NodeID)
	if err != nil {
		if errors.Is(err, ErrNodeNotFound) {
			return stepSkipped(scaleText(lang, "节点记录已不存在", "the node record no longer exists"))
		}
		return stepFailed(err, nil)
	}
	host, err := s.svc.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil {
		return stepFailed(err, nil)
	}
	if !s.hostReady(host) {
		return stepFailed(errors.New(scaleText(lang,
			"目标主机 Agent 离线，无法停止进程；可使用“仅移除记录（强制）”",
			"the target host Agent is offline and cannot stop the process; use \"remove record only (force)\" instead")), nil)
	}
	if running, known := s.checkNodeProcess(ctx, node, host); known && !running {
		_ = s.svc.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusStopped)
		_ = s.svc.repo.UpdateNodeProcess(ctx, node.ID, 0, "stopped")
		return stepSucceeded(scaleText(lang, "节点进程已停止", "the node process is already stopped"), map[string]any{"node_id": node.ID, "stopped": false})
	}
	result, err := s.svc.StopNode(ctx, op.ClusterID, node.ID)
	if err != nil {
		return stepFailed(err, nil)
	}
	if result != nil && !result.Success {
		return stepFailed(errors.New(firstNonEmpty(result.Message, "stop node failed")), result)
	}
	deadline := s.now().Add(s.timing.stopTimeout)
	for {
		running, known := s.checkNodeProcess(ctx, node, host)
		if known && !running {
			break
		}
		if !s.now().Before(deadline) {
			return stepFailed(errors.New(scaleText(lang, "停止命令已执行，但进程仍在运行", "the stop command ran but the process is still running")), nil)
		}
		if err := s.sleep(ctx, s.timing.pollInterval); err != nil {
			return stepFailed(err, nil)
		}
	}
	return stepSucceeded(scaleText(lang, "节点进程已停止", "node process stopped"), map[string]any{"node_id": node.ID, "stopped": true})
}

// stepCleanupRecords 删除节点记录；该主机在本集群已无节点时删除其主机配置记录。
// stepCleanupRecords deletes the node record and, when the host has no node of this cluster left, its host configs.
func (s *ScaleService) stepCleanupRecords(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	if err := s.svc.repo.RemoveNode(ctx, plan.Target.NodeID); err != nil && !errors.Is(err, ErrNodeNotFound) {
		return stepFailed(err, nil)
	}
	nodes, err := s.svc.repo.GetNodesByClusterID(ctx, op.ClusterID)
	if err != nil {
		return stepFailed(err, nil)
	}
	hostStillUsed := false
	for _, node := range nodes {
		if node.HostID == plan.Target.HostID {
			hostStillUsed = true
			break
		}
	}
	deletedConfigs := 0
	if !hostStillUsed {
		deletedConfigs, err = s.config.DeleteHostConfigs(ctx, op.ClusterID, plan.Target.HostID)
		if err != nil {
			return stepFailed(err, nil)
		}
	}
	s.svc.updateClusterStatusFromNodes(ctx, op.ClusterID)
	s.svc.notifyClusterTopologyChanged(ctx, op.ClusterID)
	return stepSucceeded(scaleText(lang,
		fmt.Sprintf("已删除节点记录与 %d 项主机配置", deletedConfigs),
		fmt.Sprintf("deleted the node record and %d host config(s)", deletedConfigs)),
		map[string]any{"node_id": plan.Target.NodeID, "deleted_configs": deletedConfigs, "host_still_used": hostStillUsed})
}

// stepRefreshMonitor 向该主机 Agent 下发按集群限定的监控配置（可能为空列表）以移除跟踪项；失败记为 partial（停止时 Agent 已取消跟踪）。
// stepRefreshMonitor pushes the cluster-scoped monitor config (possibly empty) to the host Agent to drop tracked
// entries; failures are partial since the Agent already untracked the process on stop.
func (s *ScaleService) stepRefreshMonitor(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	if s.monitor == nil {
		return stepSkipped(scaleText(lang, "未配置监控下发能力", "monitor push is not configured"))
	}
	if err := s.monitor.PushClusterMonitorConfigToHost(ctx, op.ClusterID, plan.Target.HostID); err != nil {
		return stepPartial(scaleText(lang, "下发监控配置失败："+err.Error(), "failed to push the monitor config: "+err.Error()), nil)
	}
	return stepSucceeded(scaleText(lang, "已刷新主机监控跟踪列表", "host monitor tracking list refreshed"), nil)
}

// stepFollowJavaProxy 被移除节点承载 java-proxy 时：尽力停止旧实例，在新选出的 master 上启动并等待健康。
// stepFollowJavaProxy, when the removed node hosted the java-proxy: stop the old instance best-effort, start it on the
// newly picked master and wait until healthy.
func (s *ScaleService) stepFollowJavaProxy(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	follow := plan.JavaProxyFollow
	if follow == nil {
		return stepSkipped(scaleText(lang, "被移除节点不承载 java-proxy", "the removed node does not host the java-proxy"))
	}
	s.stopJavaProxyOnHost(ctx, op.ClusterID, follow.FromHostID, follow.FromNodeID, follow.FromInstallDir)
	if status, err := s.svc.GetSTXJavaProxyStatus(ctx, op.ClusterID); err == nil && status != nil && status.Healthy && status.NodeID != follow.FromNodeID {
		return stepSucceeded(scaleText(lang, "java-proxy 已在 "+status.HostName+" 上健康运行", "the java-proxy is healthy on "+status.HostName), status)
	}
	started, err := s.svc.StartSTXJavaProxy(ctx, op.ClusterID)
	if err != nil {
		return stepFailed(fmt.Errorf("start java-proxy: %w", err), started)
	}
	deadline := s.now().Add(s.timing.javaProxyTimeout)
	var status *STXJavaProxyStatus
	for {
		status, err = s.svc.GetSTXJavaProxyStatus(ctx, op.ClusterID)
		if err == nil && status != nil && status.Healthy {
			break
		}
		if !s.now().Before(deadline) {
			return stepFailed(fmt.Errorf("java-proxy did not become healthy within %s: %v", s.timing.javaProxyTimeout, err), status)
		}
		if err := s.sleep(ctx, s.timing.pollInterval); err != nil {
			return stepFailed(err, status)
		}
	}
	return stepSucceeded(scaleText(lang, "java-proxy 已在 "+status.HostName+" 上启动并健康", "the java-proxy started and is healthy on "+status.HostName), status)
}

// stopJavaProxyOnHost 尽力停止旧承载主机上的 java-proxy（节点记录已删除，使用计划快照中的目录）。
// stopJavaProxyOnHost stops the java-proxy on the old host best-effort (the node record is gone, so the plan
// snapshot's directory is used).
func (s *ScaleService) stopJavaProxyOnHost(ctx context.Context, clusterID, hostID, nodeID uint, installDir string) {
	host, err := s.svc.hostProvider.GetHostByID(ctx, hostID)
	if err != nil || !s.hostReady(host) {
		return
	}
	cluster, err := s.svc.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return
	}
	params := map[string]string{
		"service":     "stx_java_proxy",
		"cluster_id":  strconv.FormatUint(uint64(clusterID), 10),
		"node_id":     strconv.FormatUint(uint64(nodeID), 10),
		"install_dir": firstNonEmpty(installDir, cluster.InstallDir),
		"version":     cluster.Version,
	}
	if proxyCfg := cluster.Config.GetSTXJavaProxyConfig(); proxyCfg != nil && proxyCfg.Port > 0 {
		params["port"] = strconv.Itoa(proxyCfg.Port)
	} else if ports := cluster.Config.GetPortConfig(); ports != nil && ports.JavaProxyPort > 0 {
		params["port"] = strconv.Itoa(ports.JavaProxyPort)
	}
	if _, _, err := s.svc.agentSender.SendCommand(ctx, host.AgentID, string(OperationStop), params); err != nil {
		logger.WarnF(ctx, "[ClusterScale] 停止旧 java-proxy 失败（忽略）/ Failed to stop old java-proxy (ignored): host=%d, err=%v", hostID, err)
	}
}

// stepVerifyScaleIn 校验缩容结果：剩余运行节点看到的成员数等于运行节点数且不含被移除地址，然后刷新集群状态。
// stepVerifyScaleIn verifies scale-in: a remaining running node sees exactly the running node count without the
// removed address, then the cluster status is refreshed.
func (s *ScaleService) stepVerifyScaleIn(ctx context.Context, op *ClusterScaleOperation, plan *ScalePlan, lang string) scaleStepOutcome {
	view, err := s.refreshNodeStatuses(ctx, op.ClusterID)
	if err != nil {
		return stepFailed(err, nil)
	}
	expected := runningMemberCount(view, 0)
	var probeNode *ClusterNode
	for _, node := range view.nodes {
		if node.Status != NodeStatusRunning || !s.hostReady(view.hosts[node.HostID]) {
			continue
		}
		if probeNode == nil || (isMasterCapable(node.Role) && !isMasterCapable(probeNode.Role)) {
			probeNode = node
		}
	}
	s.svc.updateClusterStatusFromNodes(ctx, op.ClusterID)
	if expected == 0 || probeNode == nil {
		return stepSkipped(scaleText(lang, "剩余节点均未运行，跳过成员校验", "no remaining node is running; membership check skipped"))
	}
	host := view.hosts[probeNode.HostID]
	deadline := s.now().Add(s.timing.membershipTimeout)
	var probe memberProbe
	for {
		probe = s.probeMembers(ctx, probeNode, host)
		if probe.known && probe.count == expected && !containsAddress(probe.members, plan.Target.Address) {
			break
		}
		if !s.now().Before(deadline) {
			return stepFailed(fmt.Errorf("cluster sees %d members (expected %d): %s", probe.count, expected, strings.Join(probe.members, ",")),
				map[string]any{"expected": expected, "members": nonNilStrings(probe.members)})
		}
		if err := s.sleep(ctx, s.timing.pollInterval); err != nil {
			return stepFailed(err, nil)
		}
	}
	s.svc.updateClusterStatusFromNodes(ctx, op.ClusterID)
	return stepSucceeded(scaleText(lang,
		fmt.Sprintf("缩容完成，集群共 %d 个成员", probe.count),
		fmt.Sprintf("scale-in finished; the cluster has %d members", probe.count)),
		map[string]any{"expected": expected, "member_count": probe.count, "members": nonNilStrings(probe.members)})
}
