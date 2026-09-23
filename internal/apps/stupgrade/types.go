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

package stupgrade

import "time"

// AssetSource 表示升级资产的来源。
// AssetSource represents the source of an upgrade asset.
type AssetSource string

const (
	// AssetSourceLocalPackage 表示来自控制面本地包仓库。
	// AssetSourceLocalPackage means the asset comes from the control-plane package store.
	AssetSourceLocalPackage AssetSource = "local_package"
	// AssetSourceUploadedPackage 表示来自离线上传。
	// AssetSourceUploadedPackage means the asset comes from an offline upload.
	AssetSourceUploadedPackage AssetSource = "uploaded_package"
	// AssetSourceMirrorDownload 表示来自镜像在线下载。
	// AssetSourceMirrorDownload means the asset comes from an online mirror download.
	AssetSourceMirrorDownload AssetSource = "mirror_download"
	// AssetSourceLocalPlugin 表示来自控制面本地插件仓。
	// AssetSourceLocalPlugin means the asset comes from the control-plane plugin store.
	AssetSourceLocalPlugin AssetSource = "local_plugin"
	// AssetSourceManualImport 表示来自手工导入。
	// AssetSourceManualImport means the asset comes from a manual import.
	AssetSourceManualImport AssetSource = "manual_import"
)

// PlanStatus 表示升级计划状态。
// PlanStatus represents the status of an upgrade plan.
type PlanStatus string

const (
	PlanStatusDraft      PlanStatus = "draft"
	PlanStatusReady      PlanStatus = "ready"
	PlanStatusBlocked    PlanStatus = "blocked"
	PlanStatusApplied    PlanStatus = "applied"
	PlanStatusSuperseded PlanStatus = "superseded"
)

// ExecutionStatus 表示升级任务、步骤和节点的通用状态。
// ExecutionStatus represents the common status for tasks, steps, and nodes.
type ExecutionStatus string

const (
	ExecutionStatusPending           ExecutionStatus = "pending"
	ExecutionStatusReady             ExecutionStatus = "ready"
	ExecutionStatusRunning           ExecutionStatus = "running"
	ExecutionStatusCancelRequested   ExecutionStatus = "cancel_requested"
	ExecutionStatusCancelling        ExecutionStatus = "cancelling"
	ExecutionStatusSucceeded         ExecutionStatus = "succeeded"
	ExecutionStatusFailed            ExecutionStatus = "failed"
	ExecutionStatusBlocked           ExecutionStatus = "blocked"
	ExecutionStatusSkipped           ExecutionStatus = "skipped"
	ExecutionStatusRollbackRunning   ExecutionStatus = "rollback_running"
	ExecutionStatusRollbackSucceeded ExecutionStatus = "rollback_succeeded"
	ExecutionStatusRollbackFailed    ExecutionStatus = "rollback_failed"
	ExecutionStatusCancelled         ExecutionStatus = "cancelled"
)

// StepCode 表示升级编排中的固定步骤编码。
// StepCode represents the fixed step codes in the upgrade orchestration.
type StepCode string

const (
	StepCodePrecheckPackage   StepCode = "PRECHECK_PACKAGE"
	StepCodePrecheckConnector StepCode = "PRECHECK_CONNECTOR"
	StepCodeFreezeJobs        StepCode = "FREEZE_JOBS"
	StepCodeSavepoint         StepCode = "SAVEPOINT"
	StepCodeBackup            StepCode = "BACKUP"
	StepCodeDistributePackage StepCode = "DISTRIBUTE_PACKAGE"
	StepCodeSyncLib           StepCode = "SYNC_LIB"
	StepCodeSyncConnectors    StepCode = "SYNC_CONNECTORS"
	StepCodeSyncPlugins       StepCode = "SYNC_PLUGINS"
	StepCodeMergeConfig       StepCode = "MERGE_CONFIG"
	StepCodeStopCluster       StepCode = "STOP_CLUSTER"
	StepCodeSwitchVersion     StepCode = "SWITCH_VERSION"
	StepCodeStartCluster      StepCode = "START_CLUSTER"
	StepCodeHealthCheck       StepCode = "HEALTH_CHECK"
	StepCodeSmokeTest         StepCode = "SMOKE_TEST"
	StepCodeComplete          StepCode = "COMPLETE"
	StepCodeRollbackPrepare   StepCode = "ROLLBACK_PREPARE"
	StepCodeRollbackRestore   StepCode = "ROLLBACK_RESTORE"
	StepCodeRollbackRestart   StepCode = "ROLLBACK_RESTART"
	StepCodeRollbackVerify    StepCode = "ROLLBACK_VERIFY"
	StepCodeFailed            StepCode = "FAILED"
)

// LogLevel 表示升级日志级别。
// LogLevel represents the level of upgrade logs.
type LogLevel string

const (
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

// LogEventType 表示步骤日志事件类型。
// LogEventType represents the event type of a step log.
type LogEventType string

const (
	LogEventTypeStarted  LogEventType = "started"
	LogEventTypeProgress LogEventType = "progress"
	LogEventTypeSuccess  LogEventType = "success"
	LogEventTypeFailed   LogEventType = "failed"
	LogEventTypeRollback LogEventType = "rollback"
	LogEventTypeNote     LogEventType = "note"
)

// CheckCategory 表示预检查问题分类。
// CheckCategory represents the category of a precheck issue.
type CheckCategory string

const (
	CheckCategoryPackage   CheckCategory = "package"
	CheckCategoryConnector CheckCategory = "connector"
	CheckCategoryNode      CheckCategory = "node"
	CheckCategoryConfig    CheckCategory = "config"
)

// ConfigConflictStatus 表示配置冲突状态。
// ConfigConflictStatus represents the status of a config conflict.
type ConfigConflictStatus string

const (
	ConfigConflictPending  ConfigConflictStatus = "pending"
	ConfigConflictResolved ConfigConflictStatus = "resolved"
)

// PackageManifest 描述升级目标安装包。
// PackageManifest describes the target package for an upgrade.
type PackageManifest struct {
	Version   string      `json:"version"`
	FileName  string      `json:"file_name"`
	LocalPath string      `json:"local_path"`
	Checksum  string      `json:"checksum"`
	Arch      string      `json:"arch"`
	SizeBytes int64       `json:"size_bytes"`
	Source    AssetSource `json:"source"`
	Mirror    string      `json:"mirror,omitempty"`
}

// ConnectorArtifact 描述单个 connector 资产。
// ConnectorArtifact describes a single connector asset.
type ConnectorArtifact struct {
	PluginName string      `json:"plugin_name"`
	ArtifactID string      `json:"artifact_id"`
	Version    string      `json:"version"`
	Category   string      `json:"category,omitempty"`
	FileName   string      `json:"file_name"`
	LocalPath  string      `json:"local_path"`
	Checksum   string      `json:"checksum,omitempty"`
	Source     AssetSource `json:"source"`
	Required   bool        `json:"required"`
}

// LibraryArtifact 描述升级中需同步的 lib 依赖。
// LibraryArtifact describes a lib dependency that should be synchronized during upgrade.
type LibraryArtifact struct {
	GroupID    string      `json:"group_id"`
	ArtifactID string      `json:"artifact_id"`
	Version    string      `json:"version"`
	FileName   string      `json:"file_name"`
	LocalPath  string      `json:"local_path"`
	Checksum   string      `json:"checksum,omitempty"`
	Source     AssetSource `json:"source"`
	Scope      string      `json:"scope,omitempty"`
}

// PluginDependencyArtifact 描述升级中需同步的 plugins 隔离依赖。
// PluginDependencyArtifact describes an isolated dependency under plugins/ that should be synchronized during upgrade.
type PluginDependencyArtifact struct {
	GroupID      string      `json:"group_id"`
	ArtifactID   string      `json:"artifact_id"`
	Version      string      `json:"version"`
	FileName     string      `json:"file_name"`
	LocalPath    string      `json:"local_path"`
	Checksum     string      `json:"checksum,omitempty"`
	Source       AssetSource `json:"source"`
	TargetDir    string      `json:"target_dir"`
	RelativePath string      `json:"relative_path"`
}

// ConnectorManifest 描述目标版本的 connector / lib / plugins 叠加清单。
// ConnectorManifest describes the managed connector / lib / isolated dependency overlay manifest of the target version.
type ConnectorManifest struct {
	Version         string                     `json:"version"`
	ReplacementMode string                     `json:"replacement_mode"`
	Connectors      []ConnectorArtifact        `json:"connectors"`
	Libraries       []LibraryArtifact          `json:"libraries"`
	PluginDeps      []PluginDependencyArtifact `json:"plugin_dependencies"`
}

// ConfigConflict 描述单个三方合并冲突。
// ConfigConflict describes a single three-way merge conflict.
type ConfigConflict struct {
	ID            string               `json:"id"`
	ConfigType    string               `json:"config_type"`
	Path          string               `json:"path"`
	BaseValue     string               `json:"base_value,omitempty"`
	LocalValue    string               `json:"local_value,omitempty"`
	TargetValue   string               `json:"target_value,omitempty"`
	ResolvedValue string               `json:"resolved_value,omitempty"`
	Status        ConfigConflictStatus `json:"status"`
}

// ConfigMergeFile 描述单个配置文件的三方合并计划。
// ConfigMergeFile describes the three-way merge plan of a single config file.
type ConfigMergeFile struct {
	ConfigType    string           `json:"config_type"`
	TargetPath    string           `json:"target_path"`
	BaseContent   string           `json:"base_content"`
	LocalContent  string           `json:"local_content"`
	TargetContent string           `json:"target_content"`
	MergedContent string           `json:"merged_content"`
	ConflictCount int              `json:"conflict_count"`
	Resolved      bool             `json:"resolved"`
	Conflicts     []ConfigConflict `json:"conflicts"`
}

// ConfigMergePlan 描述 base/local/target 三方合并计划。
// ConfigMergePlan describes the base/local/target merge plan.
type ConfigMergePlan struct {
	Ready         bool              `json:"ready"`
	HasConflicts  bool              `json:"has_conflicts"`
	ConflictCount int               `json:"conflict_count"`
	Files         []ConfigMergeFile `json:"files"`
	GeneratedAt   time.Time         `json:"generated_at"`
}

// NodeTarget 描述本次升级的节点目标。
// NodeTarget describes a node target in the current upgrade.
type NodeTarget struct {
	ClusterNodeID    uint   `json:"cluster_node_id"`
	HostID           uint   `json:"host_id"`
	HostName         string `json:"host_name"`
	HostIP           string `json:"host_ip"`
	Role             string `json:"role"`
	Arch             string `json:"arch"`
	SourceVersion    string `json:"source_version"`
	TargetVersion    string `json:"target_version"`
	SourceInstallDir string `json:"source_install_dir"`
	TargetInstallDir string `json:"target_install_dir"`
}

// PlanStep 描述升级计划中的有序步骤。
// PlanStep describes an ordered step in an upgrade plan.
type PlanStep struct {
	Sequence    int      `json:"sequence"`
	Code        StepCode `json:"code"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
}

// UpgradePlanSnapshot 是升级资产与步骤的统一快照。
// UpgradePlanSnapshot is the unified snapshot of upgrade assets and steps.
type UpgradePlanSnapshot struct {
	ClusterID         uint              `json:"cluster_id"`
	DeploymentMode    string            `json:"deployment_mode,omitempty"`
	SourceVersion     string            `json:"source_version"`
	TargetVersion     string            `json:"target_version"`
	PackageManifest   PackageManifest   `json:"package_manifest"`
	ConnectorManifest ConnectorManifest `json:"connector_manifest"`
	ConfigMergePlan   ConfigMergePlan   `json:"config_merge_plan"`
	NodeTargets       []NodeTarget      `json:"node_targets"`
	Steps             []PlanStep        `json:"steps"`
	GeneratedAt       time.Time         `json:"generated_at"`
}

// BlockingIssue 描述阻断型预检查问题。
// BlockingIssue describes a blocking precheck issue.
type BlockingIssue struct {
	Category CheckCategory     `json:"category"`
	Code     string            `json:"code"`
	Message  string            `json:"message"`
	Blocking bool              `json:"blocking"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PrecheckResult 描述升级预检查结果。
// PrecheckResult describes the result of an upgrade precheck.
type PrecheckResult struct {
	ClusterID         uint               `json:"cluster_id"`
	TargetVersion     string             `json:"target_version"`
	Ready             bool               `json:"ready"`
	Issues            []BlockingIssue    `json:"issues"`
	PackageManifest   *PackageManifest   `json:"package_manifest,omitempty"`
	ConnectorManifest *ConnectorManifest `json:"connector_manifest,omitempty"`
	ConfigMergePlan   *ConfigMergePlan   `json:"config_merge_plan,omitempty"`
	NodeTargets       []NodeTarget       `json:"node_targets"`
	GeneratedAt       time.Time          `json:"generated_at"`
}

// PrecheckRequest 描述升级预检查输入。
// PrecheckRequest describes the input of an upgrade precheck.
type PrecheckRequest struct {
	ClusterID        uint     `json:"cluster_id" binding:"required"`
	TargetVersion    string   `json:"target_version" binding:"required"`
	PackageChecksum  string   `json:"package_checksum,omitempty"`
	PackageArch      string   `json:"package_arch,omitempty"`
	ConnectorNames   []string `json:"connector_names,omitempty"`
	NodeIDs          []uint   `json:"node_ids,omitempty"`
	TargetInstallDir string   `json:"target_install_dir,omitempty"`
}

// CreatePlanRequest 描述升级计划生成输入。
// CreatePlanRequest describes the input of upgrade plan creation.
type CreatePlanRequest struct {
	PrecheckRequest
	ConfigMergePlan *ConfigMergePlan `json:"config_merge_plan,omitempty"`
}

// ExecutePlanRequest 描述升级计划执行输入。
// ExecutePlanRequest describes the input of upgrade plan execution.
type ExecutePlanRequest struct {
	PlanID uint `json:"plan_id" binding:"required"`
}

// CreatePlanResult 描述升级计划生成输出。
// CreatePlanResult describes the result of upgrade plan creation.
type CreatePlanResult struct {
	Precheck *PrecheckResult    `json:"precheck"`
	Plan     *UpgradePlanRecord `json:"plan,omitempty"`
}

// TaskListFilter 描述升级任务列表查询条件。
// TaskListFilter describes the filters used when querying upgrade tasks.
type TaskListFilter struct {
	ClusterID   uint
	Page        int
	PageSize    int
	OwnerUserID uint
	IncludeAll  bool
}

// UpgradeTaskSummary 描述升级任务列表中的摘要信息。
// UpgradeTaskSummary describes the summary fields returned by the upgrade task list.
type UpgradeTaskSummary struct {
	ID             uint            `json:"id"`
	ExecutionID    string          `json:"execution_id,omitempty"`
	ClusterID      uint            `json:"cluster_id"`
	PlanID         uint            `json:"plan_id"`
	SourceVersion  string          `json:"source_version"`
	TargetVersion  string          `json:"target_version"`
	Status         ExecutionStatus `json:"status"`
	CurrentStep    StepCode        `json:"current_step"`
	FailureStep    StepCode        `json:"failure_step"`
	FailureReason  string          `json:"failure_reason"`
	RollbackStatus ExecutionStatus `json:"rollback_status"`
	RollbackReason string          `json:"rollback_reason"`
	StartedAt      *time.Time      `json:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at"`
	CreatedBy      uint            `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// StepLogFilter 描述步骤日志查询条件。
// StepLogFilter describes the filters used when querying step logs.
type StepLogFilter struct {
	TaskID          uint
	StepCode        StepCode
	NodeExecutionID *uint
	Level           LogLevel
	Page            int
	PageSize        int
	OwnerUserID     uint
	IncludeAll      bool
}

// DefaultExecutionSteps 返回 MVP 升级执行的固定步骤顺序。
// DefaultExecutionSteps returns the fixed step order for the MVP upgrade execution.
func DefaultExecutionSteps() []PlanStep {
	return []PlanStep{
		{Sequence: 1, Code: StepCodePrecheckPackage, Title: "安装包预检查", Description: "检查目标安装包、checksum 与架构匹配。", Required: true},
		{Sequence: 2, Code: StepCodePrecheckConnector, Title: "插件预检查", Description: "检查 connector / lib / plugins 清单是否齐全。", Required: true},
		{Sequence: 3, Code: StepCodeBackup, Title: "确认回滚基线", Description: "确认旧版本目录仍被保留，可作为双目录升级的回滚基线。", Required: true},
		{Sequence: 4, Code: StepCodeDistributePackage, Title: "分发安装包", Description: "将目标安装包准备到所有目标节点。", Required: true},
		{Sequence: 5, Code: StepCodeSyncLib, Title: "同步 Lib", Description: "保留目标安装包自带 lib，并叠加平台管理依赖。", Required: true},
		{Sequence: 6, Code: StepCodeSyncConnectors, Title: "同步 Connector", Description: "保留目标安装包自带 connector，并叠加平台管理 connector。", Required: true},
		{Sequence: 7, Code: StepCodeSyncPlugins, Title: "同步隔离依赖", Description: "保留目标安装包自带资产，并叠加 plugins 目录中的隔离依赖。", Required: true},
		{Sequence: 8, Code: StepCodeMergeConfig, Title: "应用配置", Description: "应用已确认的三方合并配置。", Required: true},
		{Sequence: 9, Code: StepCodeStopCluster, Title: "停止集群", Description: "停止当前集群进程并进入切换窗口。", Required: true},
		{Sequence: 10, Code: StepCodeSwitchVersion, Title: "切换版本", Description: "切换到目标版本目录或 current 指针。", Required: true},
		{Sequence: 11, Code: StepCodeStartCluster, Title: "启动集群", Description: "启动切换后的目标版本。", Required: true},
		{Sequence: 12, Code: StepCodeHealthCheck, Title: "健康检查", Description: "校验节点与集群服务健康状态。", Required: true},
		{Sequence: 13, Code: StepCodeSmokeTest, Title: "可用性验证", Description: "执行升级后的模板任务验证；失败只记录告警，不阻塞升级。", Required: true},
		{Sequence: 14, Code: StepCodeComplete, Title: "完成", Description: "标记升级成功并收尾清理。", Required: true},
	}
}

// DefaultRollbackSteps 返回固定回滚步骤顺序。
// DefaultRollbackSteps returns the fixed rollback step order.
func DefaultRollbackSteps(startSequence int) []PlanStep {
	sequence := startSequence
	steps := []PlanStep{
		{Sequence: sequence, Code: StepCodeRollbackPrepare, Title: "回滚准备", Description: "冻结失败现场并准备回滚。", Required: true},
		{Sequence: sequence + 1, Code: StepCodeRollbackRestore, Title: "恢复快照", Description: "恢复旧版本安装、配置与插件资产快照。", Required: true},
		{Sequence: sequence + 2, Code: StepCodeRollbackRestart, Title: "回滚重启", Description: "重新拉起旧版本集群。", Required: true},
		{Sequence: sequence + 3, Code: StepCodeRollbackVerify, Title: "回滚校验", Description: "校验旧版本恢复后的健康状态。", Required: true},
		{Sequence: sequence + 4, Code: StepCodeFailed, Title: "失败收口", Description: "记录原始失败根因与回滚结果。", Required: true},
	}
	return steps
}

// IsRollbackStep 判断步骤是否属于回滚分支。
// IsRollbackStep reports whether a step belongs to the rollback branch.
func IsRollbackStep(code StepCode) bool {
	switch code {
	case StepCodeRollbackPrepare, StepCodeRollbackRestore, StepCodeRollbackRestart, StepCodeRollbackVerify, StepCodeFailed:
		return true
	default:
		return false
	}
}
