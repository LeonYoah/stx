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

import "time"

// Phase / Status enums.
const (
	StatusIdle       = "idle"
	StatusChecking   = "checking"
	StatusAvailable  = "available"
	StatusUpToDate   = "up_to_date"
	StatusStaging    = "staging"
	StatusStaged     = "staged"
	StatusUpgrading  = "upgrading"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"

	AssetPending  = "pending"
	AssetLoading  = "loading"
	AssetReady    = "ready"
	AssetFailed   = "failed"
	AssetSkipped  = "skipped"

	StepPending   = "pending"
	StepRunning   = "running"
	StepSucceeded = "succeeded"
	StepFailed    = "failed"
	StepCancelled = "cancelled"
	StepSkipped   = "skipped"

	StepControlPlane = "control_plane"
	StepAgent        = "agent"
	StepProxy        = "proxy"
	StepVerify       = "verify"

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
}

// StageState tracks asset staging. / 预置进度。
type StageState struct {
	Mode       string       `json:"mode,omitempty"`
	Status     string       `json:"status"`
	Progress   int          `json:"progress"`
	TargetVersion string    `json:"target_version,omitempty"`
	Assets     []AssetState `json:"assets"`
	Message    string       `json:"message,omitempty"`
	ErrorCode  string       `json:"error_code,omitempty"`
	Cancellable bool        `json:"cancellable"`
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
	Cancellable   bool        `json:"cancellable"`
	StartedAt     *time.Time  `json:"started_at,omitempty"`
	CompletedAt   *time.Time  `json:"completed_at,omitempty"`
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
	// Apply=true attempts real replace hooks; MVP defaults to dry-run apply.
	// Apply=true 尝试真实替换钩子；MVP 默认 dry-run。
	Apply bool `json:"apply,omitempty"`
}
