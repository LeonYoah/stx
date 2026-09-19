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

package installer

import (
	"context"
	"time"

	"github.com/LeonYoah/stx/internal/seatunnel"
)

// MirrorSource represents the download mirror source
// MirrorSource 表示下载镜像源
type MirrorSource string

const (
	MirrorAliyun      MirrorSource = "aliyun"
	MirrorApache      MirrorSource = "apache"
	MirrorHuaweiCloud MirrorSource = "huaweicloud"
)

// InstallMode represents the installation mode
// InstallMode 表示安装模式
type InstallMode string

const (
	InstallModeOnline  InstallMode = "online"
	InstallModeOffline InstallMode = "offline"
)

// DeploymentMode represents the deployment mode
// DeploymentMode 表示部署模式
type DeploymentMode string

const (
	DeploymentModeHybrid    DeploymentMode = "hybrid"
	DeploymentModeSeparated DeploymentMode = "separated"
)

// NodeRole represents the node role
// NodeRole 表示节点角色
type NodeRole string

const (
	NodeRoleMaster       NodeRole = "master"
	NodeRoleWorker       NodeRole = "worker"
	NodeRoleMasterWorker NodeRole = "master/worker"
)

// StepStatus represents the status of an installation step
// StepStatus 表示安装步骤的状态
type StepStatus string

const (
	StepStatusPending StepStatus = "pending"
	StepStatusRunning StepStatus = "running"
	StepStatusSuccess StepStatus = "success"
	StepStatusFailed  StepStatus = "failed"
	StepStatusSkipped StepStatus = "skipped"
)

// InstallStep represents an installation step
// InstallStep 表示安装步骤
type InstallStep string

const (
	InstallStepDownload            InstallStep = "download"
	InstallStepVerify              InstallStep = "verify"
	InstallStepExtract             InstallStep = "extract"
	InstallStepConfigureCluster    InstallStep = "configure_cluster"
	InstallStepConfigureCheckpoint InstallStep = "configure_checkpoint"
	InstallStepConfigureIMAP       InstallStep = "configure_imap"
	InstallStepConfigureJVM        InstallStep = "configure_jvm"
	InstallStepInstallPlugins      InstallStep = "install_plugins"
	InstallStepRegisterCluster     InstallStep = "register_cluster"
	InstallStepComplete            InstallStep = "complete"
)

// CheckStatus represents the status of a precheck item
// CheckStatus 表示预检查项的状态
type CheckStatus string

const (
	CheckStatusPassed  CheckStatus = "passed"
	CheckStatusFailed  CheckStatus = "failed"
	CheckStatusWarning CheckStatus = "warning"
)

// PackageInfo contains information about a SeaTunnel package
// PackageInfo 包含 SeaTunnel 安装包信息
type PackageInfo struct {
	Version      string                  `json:"version"`
	FileName     string                  `json:"file_name"`
	FileSize     int64                   `json:"file_size"`
	Checksum     string                  `json:"checksum,omitempty"`
	DownloadURLs map[MirrorSource]string `json:"download_urls"`
	IsLocal      bool                    `json:"is_local"`
	LocalPath    string                  `json:"local_path,omitempty"`
	UploadedAt   *time.Time              `json:"uploaded_at,omitempty"`
}

// AvailableVersions contains available SeaTunnel versions
// AvailableVersions 包含可用的 SeaTunnel 版本
type AvailableVersions struct {
	Versions            []string                                 `json:"versions"`
	RecommendedVersion  string                                   `json:"recommended_version"`
	LocalPackages       []PackageInfo                            `json:"local_packages"`
	VersionCapabilities map[string]seatunnel.VersionCapabilities `json:"version_capabilities"`
}

// JobScheduleStrategy represents the SeaTunnel job schedule strategy.
// JobScheduleStrategy 表示 SeaTunnel 作业调度策略。
type JobScheduleStrategy string

const (
	JobScheduleStrategyWait   JobScheduleStrategy = "WAIT"
	JobScheduleStrategyReject JobScheduleStrategy = "REJECT"
)

// SlotAllocationStrategy represents the SeaTunnel slot allocation strategy.
// SlotAllocationStrategy 表示 SeaTunnel slot 分配策略。
type SlotAllocationStrategy string

const (
	SlotAllocationStrategyRandom     SlotAllocationStrategy = "RANDOM"
	SlotAllocationStrategySystemLoad SlotAllocationStrategy = "SYSTEM_LOAD"
	SlotAllocationStrategySlotRatio  SlotAllocationStrategy = "SLOT_RATIO"
)

// JobLogMode represents the SeaTunnel log output mode.
// JobLogMode 表示 SeaTunnel 日志输出模式。
type JobLogMode string

const (
	JobLogModeMixed  JobLogMode = "mixed"
	JobLogModePerJob JobLogMode = "per_job"
)

// JVMConfig contains JVM memory configuration
// JVMConfig 包含 JVM 内存配置
type JVMConfig struct {
	HybridHeapSize int `json:"hybrid_heap_size"`
	MasterHeapSize int `json:"master_heap_size"`
	WorkerHeapSize int `json:"worker_heap_size"`
}

// CheckpointStorageType represents the checkpoint storage type
// CheckpointStorageType 表示检查点存储类型
type CheckpointStorageType string

const (
	CheckpointStorageLocalFile CheckpointStorageType = "LOCAL_FILE"
	CheckpointStorageHDFS      CheckpointStorageType = "HDFS"
	CheckpointStorageOSS       CheckpointStorageType = "OSS"
	CheckpointStorageS3        CheckpointStorageType = "S3"
)

// IMAPStorageType represents the IMAP persistence storage type.
// IMAPStorageType 表示 IMAP 持久化存储类型。
type IMAPStorageType string

const (
	IMAPStorageDisabled  IMAPStorageType = "DISABLED"
	IMAPStorageLocalFile IMAPStorageType = "LOCAL_FILE"
	IMAPStorageHDFS      IMAPStorageType = "HDFS"
	IMAPStorageOSS       IMAPStorageType = "OSS"
	IMAPStorageS3        IMAPStorageType = "S3"
)

// CheckpointConfig contains checkpoint storage configuration
// CheckpointConfig 包含检查点存储配置
type CheckpointConfig struct {
	StorageType      CheckpointStorageType `json:"storage_type"`
	Namespace        string                `json:"namespace"`
	HDFSNameNodeHost string                `json:"hdfs_namenode_host,omitempty"`
	HDFSNameNodePort int                   `json:"hdfs_namenode_port,omitempty"`
	// HDFS Kerberos authentication / HDFS Kerberos 认证
	KerberosPrincipal      string `json:"kerberos_principal,omitempty"`
	KerberosKeytabFilePath string `json:"kerberos_keytab_file_path,omitempty"`
	// HDFS HA mode configuration / HDFS HA 模式配置
	HDFSHAEnabled             bool   `json:"hdfs_ha_enabled,omitempty"`
	HDFSNameServices          string `json:"hdfs_name_services,omitempty"`
	HDFSHANamenodes           string `json:"hdfs_ha_namenodes,omitempty"`
	HDFSNamenodeRPCAddress1   string `json:"hdfs_namenode_rpc_address_1,omitempty"`
	HDFSNamenodeRPCAddress2   string `json:"hdfs_namenode_rpc_address_2,omitempty"`
	HDFSFailoverProxyProvider string `json:"hdfs_failover_proxy_provider,omitempty"`
	// HdfsSitePath 是可选的 hdfs-site.xml 路径，对应 SeaTunnel hdfs_site_path。
	// HdfsSitePath is the optional hdfs-site.xml path, mapped to SeaTunnel hdfs_site_path.
	HdfsSitePath string `json:"hdfs_site_path,omitempty"`
	// DisableCache 对应 plugin-config.disable.cache；nil 表示沿用默认（hdfs 类型默认关闭缓存）。
	// DisableCache maps to plugin-config.disable.cache; nil keeps the default (cache off for hdfs type).
	DisableCache *bool `json:"disable_cache,omitempty"`
	// OSS/S3 configuration / OSS/S3 配置
	StorageEndpoint  string `json:"storage_endpoint,omitempty"`
	StorageAccessKey string `json:"storage_access_key,omitempty"`
	StorageSecretKey string `json:"storage_secret_key,omitempty"`
	StorageBucket    string `json:"storage_bucket,omitempty"`
	// S3CredentialsProvider 对应 fs.s3a.aws.credentials.provider；空则使用 SimpleAWSCredentialsProvider。
	// S3CredentialsProvider maps to fs.s3a.aws.credentials.provider; empty uses SimpleAWSCredentialsProvider.
	S3CredentialsProvider string `json:"s3_credentials_provider,omitempty"`
}

// IMAPConfig contains IMAP persistence configuration.
// IMAPConfig 包含 IMAP 持久化配置。
type IMAPConfig struct {
	StorageType IMAPStorageType `json:"storage_type"`
	Namespace   string          `json:"namespace"`
	// HDFS configuration / HDFS 配置
	HDFSNameNodeHost string `json:"hdfs_namenode_host,omitempty"`
	HDFSNameNodePort int    `json:"hdfs_namenode_port,omitempty"`
	// HDFS Kerberos authentication / HDFS Kerberos 认证
	KerberosPrincipal      string `json:"kerberos_principal,omitempty"`
	KerberosKeytabFilePath string `json:"kerberos_keytab_file_path,omitempty"`
	// HDFS HA mode configuration / HDFS HA 模式配置
	HDFSHAEnabled             bool   `json:"hdfs_ha_enabled,omitempty"`
	HDFSNameServices          string `json:"hdfs_name_services,omitempty"`
	HDFSHANamenodes           string `json:"hdfs_ha_namenodes,omitempty"`
	HDFSNamenodeRPCAddress1   string `json:"hdfs_namenode_rpc_address_1,omitempty"`
	HDFSNamenodeRPCAddress2   string `json:"hdfs_namenode_rpc_address_2,omitempty"`
	HDFSFailoverProxyProvider string `json:"hdfs_failover_proxy_provider,omitempty"`
	// HdfsSitePath 是可选的 hdfs-site.xml 路径，对应 SeaTunnel hdfs_site_path。
	// HdfsSitePath is the optional hdfs-site.xml path, mapped to SeaTunnel hdfs_site_path.
	HdfsSitePath string `json:"hdfs_site_path,omitempty"`
	// DisableCache 对应 plugin-config.disable.cache；nil 表示沿用默认。
	// DisableCache maps to plugin-config.disable.cache; nil keeps the default.
	DisableCache *bool `json:"disable_cache,omitempty"`
	// OSS/S3 configuration / OSS/S3 配置
	StorageEndpoint  string `json:"storage_endpoint,omitempty"`
	StorageAccessKey string `json:"storage_access_key,omitempty"`
	StorageSecretKey string `json:"storage_secret_key,omitempty"`
	StorageBucket    string `json:"storage_bucket,omitempty"`
	// S3CredentialsProvider 对应 fs.s3a.aws.credentials.provider；空则使用 SimpleAWSCredentialsProvider。
	// S3CredentialsProvider maps to fs.s3a.aws.credentials.provider; empty uses SimpleAWSCredentialsProvider.
	S3CredentialsProvider string `json:"s3_credentials_provider,omitempty"`
}

// ConnectorConfig contains connector installation configuration
// ConnectorConfig 包含连接器安装配置
type ConnectorConfig struct {
	InstallConnectors bool         `json:"install_connectors"`
	Connectors        []string     `json:"connectors,omitempty"`
	PluginRepo        MirrorSource `json:"plugin_repo,omitempty"`
	// SelectedPlugins is the list of plugin names to install during SeaTunnel setup
	// SelectedPlugins 是 SeaTunnel 安装过程中要安装的插件名称列表
	SelectedPlugins []string `json:"selected_plugins,omitempty"`
	// SelectedPluginProfiles stores selected profile keys for profile-aware plugins
	// SelectedPluginProfiles 保存带画像插件选择的画像键
	SelectedPluginProfiles map[string][]string `json:"selected_plugin_profiles,omitempty"`
}

// PluginInstallInfo contains information about a plugin installation during SeaTunnel setup
// PluginInstallInfo 包含 SeaTunnel 安装过程中插件安装的信息
type PluginInstallInfo struct {
	// Name is the plugin name / Name 是插件名称
	Name string `json:"name"`
	// Category is the plugin category (source/sink/transform) / Category 是插件分类
	Category string `json:"category"`
	// Version is the plugin version / Version 是插件版本
	Version string `json:"version"`
	// Status is the installation status / Status 是安装状态
	Status PluginInstallStatus `json:"status"`
	// Progress is the installation progress (0-100) / Progress 是安装进度
	Progress int `json:"progress"`
	// Message is the status message / Message 是状态消息
	Message string `json:"message,omitempty"`
	// Error is the error message if failed / Error 是失败时的错误信息
	Error string `json:"error,omitempty"`
}

// PluginInstallStatus represents the status of a plugin installation
// PluginInstallStatus 表示插件安装的状态
type PluginInstallStatus string

const (
	// PluginInstallStatusPending indicates the plugin is waiting to be installed
	// PluginInstallStatusPending 表示插件等待安装
	PluginInstallStatusPending PluginInstallStatus = "pending"
	// PluginInstallStatusDownloading indicates the plugin is being downloaded
	// PluginInstallStatusDownloading 表示插件正在下载
	PluginInstallStatusDownloading PluginInstallStatus = "downloading"
	// PluginInstallStatusInstalling indicates the plugin is being installed
	// PluginInstallStatusInstalling 表示插件正在安装
	PluginInstallStatusInstalling PluginInstallStatus = "installing"
	// PluginInstallStatusCompleted indicates the plugin installation is completed
	// PluginInstallStatusCompleted 表示插件安装完成
	PluginInstallStatusCompleted PluginInstallStatus = "completed"
	// PluginInstallStatusFailed indicates the plugin installation failed
	// PluginInstallStatusFailed 表示插件安装失败
	PluginInstallStatusFailed PluginInstallStatus = "failed"
)

// InstallationRequest is the request for one-click installation
// InstallationRequest 是一键安装的请求
type InstallationRequest struct {
	HostID                  string                 `json:"host_id"`
	ClusterID               string                 `json:"cluster_id"`
	Version                 string                 `json:"version" binding:"required"`
	InstallDir              string                 `json:"install_dir"`
	InstallMode             InstallMode            `json:"install_mode"`
	Mirror                  MirrorSource           `json:"mirror,omitempty"`
	PackagePath             string                 `json:"package_path,omitempty"`
	DeploymentMode          DeploymentMode         `json:"deployment_mode"`
	NodeRole                NodeRole               `json:"node_role"`
	MasterAddresses         []string               `json:"master_addresses,omitempty"`
	WorkerAddresses         []string               `json:"worker_addresses,omitempty"` // Worker addresses for separated mode / 分离模式的 worker 地址
	ClusterPort             int                    `json:"cluster_port,omitempty"`     // Master hazelcast port / Master Hazelcast 端口
	WorkerPort              int                    `json:"worker_port,omitempty"`      // Worker hazelcast port / Worker Hazelcast 端口
	HTTPPort                int                    `json:"http_port,omitempty"`        // SeaTunnel HTTP API 端口
	JavaProxyPort           int                    `json:"java_proxy_port,omitempty"`  // Managed stx-java-proxy listen port / 托管 stx-java-proxy 监听端口
	EnableHTTP              *bool                  `json:"enable_http,omitempty"`      // 是否开启 SeaTunnel HTTP / Web UI
	DynamicSlot             *bool                  `json:"dynamic_slot,omitempty"`
	SlotNum                 *int                   `json:"slot_num,omitempty"`
	SlotAllocationStrategy  SlotAllocationStrategy `json:"slot_allocation_strategy,omitempty"`
	JobScheduleStrategy     JobScheduleStrategy    `json:"job_schedule_strategy,omitempty"`
	HistoryJobExpireMinutes *int                   `json:"history_job_expire_minutes,omitempty"`
	ScheduledDeletionEnable *bool                  `json:"scheduled_deletion_enable,omitempty"`
	JobLogMode              JobLogMode             `json:"job_log_mode,omitempty"`
	JVM                     *JVMConfig             `json:"jvm,omitempty"`
	Checkpoint              *CheckpointConfig      `json:"checkpoint,omitempty"`
	IMAP                    *IMAPConfig            `json:"imap,omitempty"`
	Connector               *ConnectorConfig       `json:"connector,omitempty"`
}

// StepInfo contains information about an installation step
// StepInfo 包含安装步骤的信息
type StepInfo struct {
	Step        InstallStep `json:"step"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Status      StepStatus  `json:"status"`
	Progress    int         `json:"progress"`
	Message     string      `json:"message,omitempty"`
	Error       string      `json:"error,omitempty"`
	StartTime   *time.Time  `json:"start_time,omitempty"`
	EndTime     *time.Time  `json:"end_time,omitempty"`
	Retryable   bool        `json:"retryable"`
}

// InstallationStatus represents the current installation status
// InstallationStatus 表示当前安装状态
type InstallationStatus struct {
	ID          string      `json:"id"`
	HostID      string      `json:"host_id"`
	ClusterID   string      `json:"cluster_id,omitempty"`
	Status      StepStatus  `json:"status"`
	CurrentStep InstallStep `json:"current_step"`
	Steps       []StepInfo  `json:"steps"`
	Progress    int         `json:"progress"`
	Message     string      `json:"message,omitempty"`
	Error       string      `json:"error,omitempty"`
	Warnings    []string    `json:"warnings,omitempty"`
	StartTime   time.Time   `json:"start_time"`
	EndTime     *time.Time  `json:"end_time,omitempty"`
}

// PrecheckItem represents a single precheck result item
// PrecheckItem 表示单个预检查结果项
type PrecheckItem struct {
	Name    string                 `json:"name"`
	Status  CheckStatus            `json:"status"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// PrecheckResult contains all precheck results
// PrecheckResult 包含所有预检查结果
type PrecheckResult struct {
	Items         []PrecheckItem `json:"items"`
	OverallStatus CheckStatus    `json:"overall_status"`
	Summary       string         `json:"summary"`
}

// DownloadStatus represents the status of a download task
// DownloadStatus 表示下载任务的状态
type DownloadStatus string

const (
	DownloadStatusPending     DownloadStatus = "pending"
	DownloadStatusDownloading DownloadStatus = "downloading"
	DownloadStatusCancelling  DownloadStatus = "cancelling"
	DownloadStatusCompleted   DownloadStatus = "completed"
	DownloadStatusFailed      DownloadStatus = "failed"
	DownloadStatusCancelled   DownloadStatus = "cancelled"
)

// DownloadTask represents a package download task
// DownloadTask 表示安装包下载任务
type DownloadTask struct {
	ID              string         `json:"id"`
	Version         string         `json:"version"`
	Mirror          MirrorSource   `json:"mirror"`
	DownloadURL     string         `json:"download_url"`
	Status          DownloadStatus `json:"status"`
	Progress        int            `json:"progress"` // 0-100
	DownloadedBytes int64          `json:"downloaded_bytes"`
	TotalBytes      int64          `json:"total_bytes"`
	Speed           int64          `json:"speed"` // bytes per second
	Message         string         `json:"message,omitempty"`
	Error           string         `json:"error,omitempty"`
	StartTime       time.Time      `json:"start_time"`
	EndTime         *time.Time     `json:"end_time,omitempty"`
	ExecutionID     string         `json:"execution_id,omitempty"`
	OwnerUserID     uint64         `json:"-"`
	// cancel stops the active HTTP request and is not exposed in API responses.
	// cancel 用于停止正在执行的 HTTP 请求，不对外暴露。
	cancel context.CancelFunc `json:"-"`
	// done closes only after the background downloader has stopped.
	// done 仅在后台下载协程真正停止后关闭。
	done chan struct{} `json:"-"`
}

// DownloadRequest represents a request to download a package
// DownloadRequest 表示下载安装包的请求
type DownloadRequest struct {
	Version string       `json:"version" binding:"required"`
	Mirror  MirrorSource `json:"mirror"`
}

// RuntimeStorageValidationKind indicates which runtime storage config to validate.
// RuntimeStorageValidationKind 表示需要校验的运行时存储配置类型。
type RuntimeStorageValidationKind string

const (
	RuntimeStorageValidationCheckpoint RuntimeStorageValidationKind = "checkpoint"
	RuntimeStorageValidationIMAP       RuntimeStorageValidationKind = "imap"
)

// RuntimeStorageValidationRequest validates checkpoint or IMAP storage reachability from hosts.
// RuntimeStorageValidationRequest 用于校验主机到 checkpoint 或 IMAP 存储的可达性。
type RuntimeStorageValidationRequest struct {
	HostIDs    []uint                       `json:"host_ids" binding:"required,min=1"`
	Kind       RuntimeStorageValidationKind `json:"kind" binding:"required"`
	Checkpoint *CheckpointConfig            `json:"checkpoint,omitempty"`
	IMAP       *IMAPConfig                  `json:"imap,omitempty"`
}

// RuntimeStorageValidationHostResult describes one host validation result.
// RuntimeStorageValidationHostResult 描述单个主机的校验结果。
type RuntimeStorageValidationHostResult struct {
	HostID   uint              `json:"host_id"`
	HostName string            `json:"host_name,omitempty"`
	Success  bool              `json:"success"`
	Message  string            `json:"message"`
	Details  map[string]string `json:"details,omitempty"`
}

// RuntimeStorageValidationResult is the aggregated validation result.
// RuntimeStorageValidationResult 是聚合后的校验结果。
type RuntimeStorageValidationResult struct {
	Success bool                                  `json:"success"`
	Kind    RuntimeStorageValidationKind          `json:"kind"`
	Warning string                                `json:"warning,omitempty"`
	Hosts   []*RuntimeStorageValidationHostResult `json:"hosts"`
}
