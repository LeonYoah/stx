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
	"fmt"
	"time"
)

// Phase / Status enums.
const (
	StatusIdle      = "idle"
	StatusChecking  = "checking"
	StatusAvailable = "available"
	StatusUpToDate  = "up_to_date"
	StatusStaging   = "staging"
	StatusStaged    = "staged"
	StatusUpgrading = "upgrading"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	AssetPending = "pending"
	AssetLoading = "loading"
	AssetReady   = "ready"
	AssetFailed  = "failed"
	AssetSkipped = "skipped"

	StepPending   = "pending"
	StepRunning   = "running"
	StepSucceeded = "succeeded"
	StepFailed    = "failed"
	StepCancelled = "cancelled"
	StepSkipped   = "skipped"
	// StepPartial 表示步骤完成但部分节点未成功（见 agent_nodes）。
	// StepPartial means the step finished but some nodes did not succeed (see agent_nodes).
	StepPartial = "partial"

	StepControlPlane = "control_plane"
	StepAgent        = "agent"
	StepProxy        = "proxy"
	StepVerify       = "verify"
	// StepRestart 重启 STX 自身进程（控制面、随包前端）；不触碰 SeaTunnel。
	// StepRestart restarts STX's own processes (control plane, bundled frontend); SeaTunnel is never touched.
	StepRestart = "restart"

	// ExecModeApply 真实应用升级；ExecModeDryRun 仅演练、不做任何变更。
	// ExecModeApply really applies the upgrade; ExecModeDryRun only rehearses and changes nothing.
	ExecModeApply  = "apply"
	ExecModeDryRun = "dry_run"

	// 进程重启状态 / process restart status
	RestartPending     = "pending"
	RestartRunning     = "restarting"
	RestartSucceeded   = "restarted"
	RestartRolledBack  = "rolled_back"
	RestartFailed      = "failed"
	RestartSkipped     = "skipped"
	RestartNotRunning  = "not_running"
	RestartWouldDoThis = "planned"

	ModeOnline  = "online"
	ModeOffline = "offline"
)

// AssetSpec describes one staged release asset. / 描述一项预置资产。
type AssetSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Glob        bool   `json:"glob"`
}

// AssetState is the runtime state of one asset. / 单项资产运行时状态。
type AssetState struct {
	AssetSpec
	Status   string `json:"status"`
	Size     int64  `json:"size,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message,omitempty"`
	Progress int    `json:"progress,omitempty"`
}

// StepState is one upgrade execution step. / 升级执行步骤。
type StepState struct {
	Code      string     `json:"code"`
	Label     string     `json:"label"`
	Status    string     `json:"status"`
	Message   string     `json:"message,omitempty"`
	Error     string     `json:"error,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// CheckResult is the outcome of a version check. / 版本检查结果。
type CheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
	CheckedAt       string `json:"checked_at"`
	Message         string `json:"message,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
	// StagedVersion 是检查后仍保留的已预置版本（可能与 LatestVersion 不同）。
	// StagedVersion is the staged version kept after the check (may differ from LatestVersion).
	StagedVersion string `json:"staged_version,omitempty"`
	// StagePreserved 表示本次检查保留了已预置包，可直接执行升级。
	// StagePreserved reports that this check kept the staged package, so execution can proceed.
	StagePreserved bool `json:"stage_preserved,omitempty"`
}

// StageState tracks asset staging. / 预置进度。
type StageState struct {
	Mode          string       `json:"mode,omitempty"`
	Status        string       `json:"status"`
	Progress      int          `json:"progress"`
	TargetVersion string       `json:"target_version,omitempty"`
	Assets        []AssetState `json:"assets"`
	Message       string       `json:"message,omitempty"`
	ErrorCode     string       `json:"error_code,omitempty"`
	Cancellable   bool         `json:"cancellable"`
}

// TaskState is an execute/stage task snapshot. / 执行或预置任务快照。
type TaskState struct {
	ID            string      `json:"id"`
	Kind          string      `json:"kind"` // stage | execute
	Status        string      `json:"status"`
	TargetVersion string      `json:"target_version,omitempty"`
	CurrentStep   string      `json:"current_step,omitempty"`
	FailureStep   string      `json:"failure_step,omitempty"`
	FailureReason string      `json:"failure_reason,omitempty"`
	ErrorCode     string      `json:"error_code,omitempty"`
	Steps         []StepState `json:"steps,omitempty"`
	// AgentNodes 是各 Worker 节点 Agent/Proxy 的升级状态。
	// AgentNodes is the per-node Agent/Proxy upgrade status.
	AgentNodes []AgentNodeUpgrade `json:"agent_nodes,omitempty"`
	// ExecutionMode 是 apply 或 dry_run；DryRun 便于直接判断。
	// ExecutionMode is apply or dry_run; DryRun is a convenience flag.
	ExecutionMode string `json:"execution_mode,omitempty"`
	DryRun        bool   `json:"dry_run"`
	// Restarts 是控制面与随包前端的重启结果。/ Restarts holds control plane and bundled frontend restart results.
	Restarts    []ProcessRestart `json:"restarts,omitempty"`
	Cancellable bool             `json:"cancellable"`
	StartedAt   *time.Time       `json:"started_at,omitempty"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
}

// UpgradeHistoryRecord 记录一次完成或失败的自升级历史。
// UpgradeHistoryRecord records a completed, failed, or rolled back upgrade attempt.
type UpgradeHistoryRecord struct {
	ID             string      `json:"id"`
	StartedAt      time.Time   `json:"started_at"`
	CompletedAt    time.Time   `json:"completed_at"`
	DurationMs     int64       `json:"duration_ms"`
	FromVersion    string      `json:"from_version"`
	TargetVersion  string      `json:"target_version"`
	Mode           string      `json:"mode"`            // online | offline
	DeploymentType string      `json:"deployment_type"` // host | docker
	Status         string      `json:"status"`          // succeeded | failed | rolled_back
	FailureStep    string      `json:"failure_step,omitempty"`
	FailureReason  string      `json:"failure_reason,omitempty"`
	ErrorCode      string      `json:"error_code,omitempty"`
	Steps          []StepState `json:"steps,omitempty"`
	// AgentNodes 是各节点 Agent/Proxy 的升级结果（离线节点重连后补升级会回写）。
	// AgentNodes is the per-node Agent/Proxy result (updated when offline nodes catch up after reconnecting).
	AgentNodes []AgentNodeUpgrade `json:"agent_nodes,omitempty"`
	// ExecutionMode 是 apply 或 dry_run（旧记录为空）。/ ExecutionMode is apply or dry_run (empty for older records).
	ExecutionMode string           `json:"execution_mode,omitempty"`
	DryRun        bool             `json:"dry_run"`
	Restarts      []ProcessRestart `json:"restarts,omitempty"`
}

// StatusResponse is the aggregate console status. / 控制台聚合状态。
type StatusResponse struct {
	CurrentVersion  string       `json:"current_version"`
	GitCommit       string       `json:"git_commit,omitempty"`
	BuildTime       string       `json:"build_time,omitempty"`
	LatestVersion   string       `json:"latest_version,omitempty"`
	UpdateAvailable bool         `json:"update_available"`
	Checked         bool         `json:"checked"`
	OverallStatus   string       `json:"overall_status"`
	Stage           StageState   `json:"stage"`
	ActiveTask      *TaskState   `json:"active_task,omitempty"`
	LastCheck       *CheckResult `json:"last_check,omitempty"`
	MockMode        bool         `json:"mock_mode"`
	// DeploymentType distinguishes container vs host deployments ("docker" | "host").
	// DeploymentType 区分容器化与物理宿主机部署 ("docker" | "host")。
	DeploymentType string `json:"deployment_type"`
	// DockerComposeUpgradeCmd provides the suggested container upgrade command.
	// DockerComposeUpgradeCmd 提供推荐的容器升级命令。
	DockerComposeUpgradeCmd string `json:"docker_compose_upgrade_cmd,omitempty"`
	// Architecture reflects the host operating system and CPU architecture.
	// Architecture 反映当前宿主机操作系统与架构 (如 linux/arm64)。
	Architecture string `json:"architecture"`
	// Agents 列出各节点当前 Agent 版本及是否与控制面一致。
	// Agents lists each node's current Agent version and whether it matches the control plane.
	Agents []AgentVersionInfo `json:"agents,omitempty"`
	// AgentsOutdated 是版本与控制面不一致的 Agent 数量。
	// AgentsOutdated counts Agents whose version differs from the control plane.
	AgentsOutdated int `json:"agents_outdated"`
	// RestartPlan 描述真实应用时会重启的 STX 进程（SeaTunnel 不受影响）。
	// RestartPlan describes the STX processes an apply restarts (SeaTunnel is unaffected).
	RestartPlan *RestartPlanInfo `json:"restart_plan,omitempty"`
}

// CheckRequest optionally injects a failure for local smoke tests.
// CheckRequest 可注入失败码，便于本地冒烟。
type CheckRequest struct {
	SimulateFailure string `json:"simulate_failure,omitempty"`
	ForceLatest     string `json:"force_latest,omitempty"`
}

// StageDownloadRequest starts online staging. / 启动联网预置。
type StageDownloadRequest struct {
	TargetVersion   string `json:"target_version,omitempty"`
	SimulateFailure string `json:"simulate_failure,omitempty"`
}

// ExecuteRequest starts the upgrade from local packages only.
// ExecuteRequest 仅从本地预置包开升。
type ExecuteRequest struct {
	SimulateFailure string `json:"simulate_failure,omitempty"`
	// Mode 显式选择 "apply"（真实应用：替换并重启控制面、Agent、proxy、随包前端）或 "dry_run"（演练，不做任何变更）。
	// 省略时回退到旧字段 apply；两者都省略时为 dry_run（安全默认）。
	// Mode explicitly selects "apply" (really replace and restart the control plane, Agents, proxies and bundled
	// frontend) or "dry_run" (rehearse, change nothing). When omitted the legacy apply flag decides; when both are
	// omitted the request is a dry run (safe default).
	Mode string `json:"mode,omitempty"`
	// Apply 是旧字段，等价于 mode=apply；与 mode 冲突时报错。
	// Apply is the legacy flag, equivalent to mode=apply; conflicting with mode is rejected.
	Apply bool `json:"apply,omitempty"`
}

// ResolveMode 返回本次执行的显式模式。/ ResolveMode returns the explicit execution mode of the request.
func (r ExecuteRequest) ResolveMode() (string, error) {
	switch r.Mode {
	case ExecModeApply:
		return ExecModeApply, nil
	case ExecModeDryRun:
		if r.Apply {
			return "", fmt.Errorf("mode=dry_run conflicts with apply=true / mode=dry_run 与 apply=true 冲突")
		}
		return ExecModeDryRun, nil
	case "":
		if r.Apply {
			return ExecModeApply, nil
		}
		return ExecModeDryRun, nil
	default:
		return "", fmt.Errorf("invalid mode %q: must be %q or %q / mode 只能是 %q 或 %q", r.Mode, ExecModeApply, ExecModeDryRun, ExecModeApply, ExecModeDryRun)
	}
}

// ProcessRestart 是一个 STX 自身进程的重启结果。/ ProcessRestart is the restart result of one STX-owned process.
type ProcessRestart struct {
	// Component: control_plane | frontend
	Component string `json:"component"`
	// Strategy: standalone | systemd | supervisor | pm2 | hook | none
	Strategy string     `json:"strategy,omitempty"`
	Status   string     `json:"status"`
	Message  string     `json:"message,omitempty"`
	OldPID   int        `json:"old_pid,omitempty"`
	NewPID   int        `json:"new_pid,omitempty"`
	At       *time.Time `json:"at,omitempty"`
}

// RestartPlanInfo 告诉确认框真实应用时会重启哪些进程。/ RestartPlanInfo tells the confirm dialog which processes an apply restarts.
type RestartPlanInfo struct {
	ControlPlaneStrategy string `json:"control_plane_strategy"`
	ControlPlaneDetail   string `json:"control_plane_detail,omitempty"`
	// FrontendDetected 表示检测到随包的 Next standalone 前端进程，会一并重启。
	// FrontendDetected reports a bundled Next standalone frontend process that is restarted too.
	FrontendDetected bool   `json:"frontend_detected"`
	FrontendStrategy string `json:"frontend_strategy,omitempty"`
	// RestartsAgents / RestartsProxies 恒为 true：Agent 换二进制后原地重启，运行中的 stx-java-proxy 换 jar 后重启。
	// RestartsAgents / RestartsProxies are always true: Agents restart in place after the swap, running proxies after the jar swap.
	RestartsAgents  bool `json:"restarts_agents"`
	RestartsProxies bool `json:"restarts_proxies"`
	// SeaTunnelAffected 恒为 false：自升级从不停止、重启或改动 SeaTunnel/Hazelcast 进程与作业。
	// SeaTunnelAffected is always false: self-upgrade never stops, restarts or modifies SeaTunnel/Hazelcast processes or jobs.
	SeaTunnelAffected bool `json:"seatunnel_affected"`
}
