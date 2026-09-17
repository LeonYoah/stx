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

// Package plugin provides SeaTunnel plugin marketplace management.
// plugin 包提供 SeaTunnel 插件市场管理功能。
package plugin

import (
	"time"
)

// PluginCategory represents the category of a plugin.
// PluginCategory 表示插件的分类。
type PluginCategory string

const (
	PluginCategorySource    PluginCategory = "source"    // Source / Data source (legacy, for compatibility / 遗留，用于兼容)
	PluginCategorySink      PluginCategory = "sink"      // Sink / Data sink (legacy, for compatibility / 遗留，用于兼容)
	PluginCategoryConnector PluginCategory = "connector" // Connector / 连接器 (primary category, can be used as source or sink / 主要分类，可作为 source 或 sink)
	PluginCategoryTransform PluginCategory = "transform" // Transform / 数据转换 (deprecated, not fetched from Maven / 已弃用，不从 Maven 获取)
)

// PluginStatus represents the status of a plugin.
// PluginStatus 表示插件的状态。
type PluginStatus string

const (
	PluginStatusAvailable PluginStatus = "available" // 可用 / Available
	PluginStatusInstalled PluginStatus = "installed" // 已安装 / Installed
	PluginStatusEnabled   PluginStatus = "enabled"   // 已启用 / Enabled
	PluginStatusDisabled  PluginStatus = "disabled"  // 已禁用 / Disabled
)

// MirrorSource represents the Maven repository mirror source.
// MirrorSource 表示 Maven 仓库镜像源。
type MirrorSource string

const (
	MirrorSourceApache      MirrorSource = "apache"      // Apache 官方仓库
	MirrorSourceAliyun      MirrorSource = "aliyun"      // 阿里云镜像
	MirrorSourceHuaweiCloud MirrorSource = "huaweicloud" // 华为云镜像
)

// MirrorURLs maps mirror sources to their Maven repository base URLs.
// MirrorURLs 将镜像源映射到其 Maven 仓库基础 URL。
var MirrorURLs = map[MirrorSource]string{
	MirrorSourceApache:      "https://repo.maven.apache.org/maven2",
	MirrorSourceAliyun:      "https://maven.aliyun.com/repository/public",
	MirrorSourceHuaweiCloud: "https://repo.huaweicloud.com/repository/maven",
}

// PluginDependency represents a dependency of a plugin.
// PluginDependency 表示插件的依赖项。
type PluginDependency struct {
	GroupID          string                 `json:"group_id"`                     // Maven groupId
	ArtifactID       string                 `json:"artifact_id"`                  // Maven artifactId
	Version          string                 `json:"version"`                      // 版本号 / Version
	TargetDir        string                 `json:"target_dir"`                   // 目标目录 (connectors/、lib/ 或 plugins/<mapping>) / Target directory
	SourceType       PluginDependencySource `json:"source_type,omitempty"`        // 依赖来源 / Dependency source
	OriginalFileName string                 `json:"original_file_name,omitempty"` // 原始上传文件名 / Original uploaded file name
	StoredPath       string                 `json:"-"`                            // 控制面存储路径（仅服务端内部使用）/ Stored path (server-side only)
}

// PluginDependencySource represents the source type of an effective dependency.
// PluginDependencySource 表示依赖来源类型。
type PluginDependencySource string

const (
	PluginDependencySourceMaven    PluginDependencySource = "maven"
	PluginDependencySourceUpload   PluginDependencySource = "upload"
	PluginDependencySourceOfficial PluginDependencySource = "official"
)

// PluginDependencyConfig represents a user-configured dependency for a plugin (GORM model).
// PluginDependencyConfig 表示用户为插件配置的依赖项（GORM 模型）。
type PluginDependencyConfig struct {
	ID               uint                   `gorm:"primaryKey" json:"id"`
	PluginName       string                 `gorm:"size:80;not null;index:idx_plugin_dep,unique" json:"plugin_name"`                 // 插件名称 / Plugin name
	SeatunnelVersion string                 `gorm:"size:50;not null;default:'';index:idx_plugin_dep,unique" json:"seatunnel_version"` // SeaTunnel 版本 / SeaTunnel version
	GroupID          string                 `gorm:"size:128;not null;index:idx_plugin_dep,unique" json:"group_id"`                    // Maven groupId / 分组标识
	ArtifactID       string                 `gorm:"size:128;not null;index:idx_plugin_dep,unique" json:"artifact_id"`                 // Maven artifactId
	Version          string                 `gorm:"size:64;not null;index:idx_plugin_dep,unique" json:"version"`                      // 版本号 / Version
	TargetDir        string                 `gorm:"size:64;not null;default:lib;index:idx_plugin_dep,unique" json:"target_dir"`      // 目标目录 / Target directory
	SourceType       PluginDependencySource `gorm:"size:20;not null;default:maven;index:idx_plugin_dep,unique" json:"source_type"`    // 来源类型 / Source type
	OriginalFileName string                 `gorm:"size:255" json:"original_file_name,omitempty"`                                     // 原始文件名 / Original uploaded file name
	StoredPath       string                 `gorm:"size:1024" json:"-"`                                                               // 控制面存储路径 / Stored path
	FileSize         int64                  `gorm:"not null;default:0" json:"file_size,omitempty"`                                    // 文件大小 / File size
	Checksum         string                 `gorm:"size:128" json:"checksum,omitempty"`                                               // 文件摘要 / File checksum
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// TableName returns the table name for PluginDependencyConfig.
// TableName 返回 PluginDependencyConfig 的表名。
func (PluginDependencyConfig) TableName() string {
	return "plugin_dependency_configs"
}

// PluginDependencyDisable represents one disabled official dependency entry for a plugin/version.
// PluginDependencyDisable 表示插件某个版本下被用户禁用的一条官方依赖。
type PluginDependencyDisable struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	PluginName       string    `gorm:"size:80;not null;index:idx_plugin_dep_disable,unique" json:"plugin_name"`
	SeatunnelVersion string    `gorm:"size:50;not null;default:'';index:idx_plugin_dep_disable,unique" json:"seatunnel_version"`
	GroupID          string    `gorm:"size:128;not null;index:idx_plugin_dep_disable,unique" json:"group_id"`
	ArtifactID       string    `gorm:"size:128;not null;index:idx_plugin_dep_disable,unique" json:"artifact_id"`
	Version          string    `gorm:"size:64;not null;index:idx_plugin_dep_disable,unique" json:"version"`
	TargetDir        string    `gorm:"size:64;not null;index:idx_plugin_dep_disable,unique" json:"target_dir"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName returns the table name for PluginDependencyDisable.
// TableName 返回 PluginDependencyDisable 的表名。
func (PluginDependencyDisable) TableName() string {
	return "plugin_dependency_disables"
}

// Plugin represents a SeaTunnel plugin.
// Plugin 表示一个 SeaTunnel 插件。
type Plugin struct {
	Name                      string                   `json:"name"`                                  // 插件名称 / Plugin name
	DisplayName               string                   `json:"display_name"`                          // 显示名称 / Display name
	Category                  PluginCategory           `json:"category"`                              // 分类 / Category
	Version                   string                   `json:"version"`                               // 版本号（与 SeaTunnel 主版本一致）/ Version
	Description               string                   `json:"description"`                           // 描述 / Description
	GroupID                   string                   `json:"group_id"`                              // Maven groupId
	ArtifactID                string                   `json:"artifact_id"`                           // Maven artifactId
	Dependencies              []PluginDependency       `json:"dependencies,omitempty"`                // 依赖库列表 / Dependencies
	Icon                      string                   `json:"icon,omitempty"`                        // 图标 URL / Icon URL
	DocURL                    string                   `json:"doc_url,omitempty"`                     // 文档链接 / Documentation URL
	DependencyStatus          PluginDependencyStatus   `json:"dependency_status,omitempty"`           // 依赖状态 / Dependency status
	DependencyCount           int                      `json:"dependency_count,omitempty"`            // 生效依赖数量 / Effective dependency count
	DependencyBaselineVersion string                   `json:"dependency_baseline_version,omitempty"` // 依赖基线版本 / Dependency baseline version
	DependencyResolutionMode  DependencyResolutionMode `json:"dependency_resolution_mode,omitempty"`  // 依赖解析模式 / Dependency resolution mode
}

// InstalledPlugin represents a plugin installed on a cluster (GORM model).
// InstalledPlugin 表示安装在集群上的插件（GORM 模型）。
// Note: Plugins are managed at cluster level, not host level.
// 注意：插件在集群级别管理，而非主机级别。
type InstalledPlugin struct {
	ID                  uint               `gorm:"primaryKey" json:"id"`
	ClusterID           uint               `gorm:"index;not null" json:"cluster_id"`                 // 集群 ID / Cluster ID
	PluginName          string             `gorm:"size:100;not null;index" json:"plugin_name"`       // 插件名称 / Plugin name
	ArtifactID          string             `gorm:"size:100" json:"artifact_id"`                      // Maven artifact ID (e.g., connector-cdc-mysql)
	Category            PluginCategory     `gorm:"size:20;not null" json:"category"`                 // 分类 / Category
	Version             string             `gorm:"size:20;not null" json:"version"`                  // 版本号 / Version
	Status              PluginStatus       `gorm:"size:20;not null;default:installed" json:"status"` // 状态 / Status
	InstallPath         string             `gorm:"size:255" json:"install_path"`                     // 安装路径 / Install path
	InstalledAt         time.Time          `gorm:"not null" json:"installed_at"`                     // 安装时间 / Installed at
	UpdatedAt           time.Time          `json:"updated_at"`                                       // 更新时间 / Updated at
	InstalledBy         uint               `json:"installed_by,omitempty"`                           // 安装者 ID / Installed by
	SelectedProfileKeys []string           `gorm:"-" json:"selected_profile_keys,omitempty"`         // 选中的画像 / Selected profiles
	AttachedConnectors  []string           `gorm:"-" json:"attached_connectors,omitempty"`           // 自动附带的连接器 / Attached connectors
	Dependencies        []PluginDependency `gorm:"-" json:"dependencies,omitempty"`                  // 自动附带的依赖 / Attached dependencies
}

// TableName returns the table name for InstalledPlugin.
// TableName 返回 InstalledPlugin 的表名。
func (InstalledPlugin) TableName() string {
	return "installed_plugins"
}

// PluginFilter represents filter options for querying plugins.
// PluginFilter 表示查询插件的过滤选项。
type PluginFilter struct {
	ClusterID uint           `json:"cluster_id,omitempty"` // 集群 ID / Cluster ID
	Category  PluginCategory `json:"category,omitempty"`   // 分类 / Category
	Status    PluginStatus   `json:"status,omitempty"`     // 状态 / Status
	Keyword   string         `json:"keyword,omitempty"`    // 搜索关键词 / Search keyword
	Page      int            `json:"page,omitempty"`       // 页码 / Page number
	PageSize  int            `json:"page_size,omitempty"`  // 每页数量 / Page size
}

// InstallPluginRequest represents a request to install a plugin.
// InstallPluginRequest 表示安装插件的请求。
type InstallPluginRequest struct {
	PluginName  string       `json:"plugin_name" binding:"required"` // 插件名称 / Plugin name
	Version     string       `json:"version" binding:"required"`     // 版本号 / Version
	Mirror      MirrorSource `json:"mirror,omitempty"`               // 镜像源 / Mirror source
	ProfileKeys []string     `json:"profile_keys,omitempty"`         // 选中的依赖画像 / Selected dependency profiles
}

// PluginInstallStatus represents the installation status of a plugin.
// PluginInstallStatus 表示插件的安装状态。
type PluginInstallStatus struct {
	PluginName string `json:"plugin_name"`       // 插件名称 / Plugin name
	Status     string `json:"status"`            // 状态 / Status
	Progress   int    `json:"progress"`          // 进度 (0-100) / Progress
	Message    string `json:"message,omitempty"` // 消息 / Message
	Error      string `json:"error,omitempty"`   // 错误信息 / Error message
}

// AvailablePluginsResponse represents the response for listing available plugins.
// AvailablePluginsResponse 表示获取可用插件列表的响应。
type PluginListSource string

const (
	PluginListSourceDatabase PluginListSource = "database"
	PluginListSourceRemote   PluginListSource = "remote"
	PluginListSourceSeed     PluginListSource = "seed"
)

type AvailablePluginsResponse struct {
	Plugins             []Plugin         `json:"plugins"`                         // 插件列表 / Plugin list
	Total               int              `json:"total"`                           // 总数 / Total count
	Version             string           `json:"version"`                         // SeaTunnel 版本 / SeaTunnel version
	Mirror              string           `json:"mirror"`                          // 当前请求镜像源 / Requested mirror
	Source              PluginListSource `json:"source"`                          // 数据来源 / Data source
	CacheHit            bool             `json:"cache_hit"`                       // 是否命中缓存 / Whether cache was hit
	CatalogSourceMirror string           `json:"catalog_source_mirror,omitempty"` // 目录实际来源镜像 / Catalog source mirror
	CatalogRefreshedAt  *time.Time       `json:"catalog_refreshed_at,omitempty"`  // 最近刷新时间 / Catalog refreshed at
}

// ==================== Plugin Download Types 插件下载类型 ====================

// PluginDownloadStatus represents the status of a plugin download task.
// PluginDownloadStatus 表示插件下载任务的状态。
type PluginDownloadStatus string

const (
	PluginDownloadPending     PluginDownloadStatus = "pending"     // 等待中 / Pending
	PluginDownloadDownloading PluginDownloadStatus = "downloading" // 下载中 / Downloading
	PluginDownloadCompleted   PluginDownloadStatus = "completed"   // 已完成 / Completed
	PluginDownloadFailed      PluginDownloadStatus = "failed"      // 失败 / Failed
	PluginDownloadCancelled   PluginDownloadStatus = "cancelled"   // 已取消 / Cancelled
)

// PluginDownloadTask represents a plugin download task.
// PluginDownloadTask 表示插件下载任务。
type PluginDownloadTask struct {
	ID              string               `json:"id"`                         // 任务 ID / Task ID
	ClusterID       uint                 `json:"cluster_id"`                 // 集群 ID / Cluster ID
	PluginName      string               `json:"plugin_name"`                // 插件名称 / Plugin name
	Version         string               `json:"version"`                    // 版本号 / Version
	Mirror          MirrorSource         `json:"mirror"`                     // 镜像源 / Mirror source
	Status          PluginDownloadStatus `json:"status"`                     // 状态 / Status
	Progress        int                  `json:"progress"`                   // 进度 (0-100) / Progress
	CurrentStep     string               `json:"current_step,omitempty"`     // 当前步骤 / Current step
	DownloadedBytes int64                `json:"downloaded_bytes,omitempty"` // 已下载字节 / Downloaded bytes
	TotalBytes      int64                `json:"total_bytes,omitempty"`      // 总字节 / Total bytes
	Speed           int64                `json:"speed,omitempty"`            // 下载速度 (bytes/s) / Download speed
	Message         string               `json:"message,omitempty"`          // 消息 / Message
	Error           string               `json:"error,omitempty"`            // 错误信息 / Error message
	StartTime       time.Time            `json:"start_time"`                 // 开始时间 / Start time
	EndTime         *time.Time           `json:"end_time,omitempty"`         // 结束时间 / End time
}

// PluginDownloadRequest represents a request to download/install a plugin.
// PluginDownloadRequest 表示下载/安装插件的请求。
type PluginDownloadRequest struct {
	PluginName  string       `json:"plugin_name" binding:"required"` // 插件名称 / Plugin name
	Version     string       `json:"version" binding:"required"`     // 版本号 / Version
	Mirror      MirrorSource `json:"mirror,omitempty"`               // 镜像源 / Mirror source
	ProfileKeys []string     `json:"profile_keys,omitempty"`         // 选中的依赖画像 / Selected dependency profiles
}

// AvailableVersionsResponse represents the response for listing available versions.
// AvailableVersionsResponse 表示获取可用版本列表的响应。
type AvailableVersionsResponse struct {
	Versions           []string `json:"versions"`            // 版本列表 / Version list
	RecommendedVersion string   `json:"recommended_version"` // 推荐版本 / Recommended version
	Warning            string   `json:"warning,omitempty"`   // 警告信息 / Warning message
}

// ==================== Plugin Dependency Config Types 插件依赖配置类型 ====================

// AddDependencyRequest represents a request to add a dependency to a plugin.
// AddDependencyRequest 表示为插件添加依赖的请求。
type AddDependencyRequest struct {
	PluginName       string `json:"plugin_name"`                    // 插件名称（从 URL 获取）/ Plugin name (from URL)
	SeatunnelVersion string `json:"seatunnel_version,omitempty"`    // SeaTunnel 版本（用于推导默认 target_dir）/ SeaTunnel version
	GroupID          string `json:"group_id" binding:"required"`    // Maven groupId
	ArtifactID       string `json:"artifact_id" binding:"required"` // Maven artifactId
	Version          string `json:"version" binding:"required"`     // 版本号（必填）/ Version (required)
	TargetDir        string `json:"target_dir,omitempty"`           // 目标目录（可选）/ Target directory
}

// UploadDependencyRequest represents a request to upload a custom dependency jar.
// UploadDependencyRequest 表示上传自定义依赖 Jar 的请求。
type UploadDependencyRequest struct {
	PluginName       string `json:"plugin_name"`                 // 插件名称（从 URL 获取）/ Plugin name (from URL)
	SeatunnelVersion string `json:"seatunnel_version,omitempty"` // SeaTunnel 版本 / SeaTunnel version
	GroupID          string `json:"group_id,omitempty"`          // 分组（可选）/ Group ID (optional)
	ArtifactID       string `json:"artifact_id"`                 // 依赖名 / Artifact ID
	Version          string `json:"version"`                     // 依赖版本 / Version
	TargetDir        string `json:"target_dir,omitempty"`        // 目标目录 / Target directory
}

// DisableDependencyRequest represents a request to disable one official dependency item.
// DisableDependencyRequest 表示禁用一条官方依赖的请求。
type DisableDependencyRequest struct {
	PluginName       string `json:"plugin_name"`                 // 插件名称（从 URL 获取）/ Plugin name (from URL)
	SeatunnelVersion string `json:"seatunnel_version,omitempty"` // SeaTunnel 版本 / SeaTunnel version
	GroupID          string `json:"group_id" binding:"required"`
	ArtifactID       string `json:"artifact_id" binding:"required"`
	Version          string `json:"version" binding:"required"`
	TargetDir        string `json:"target_dir" binding:"required"`
}

// UpdateDependencyRequest represents a request to update a dependency.
// UpdateDependencyRequest 表示更新依赖的请求。
type UpdateDependencyRequest struct {
	GroupID    string `json:"group_id"`    // Maven groupId
	ArtifactID string `json:"artifact_id"` // Maven artifactId
	Version    string `json:"version"`     // 版本号 / Version
}

// PluginDependencyResponse represents the response for plugin dependencies.
// PluginDependencyResponse 表示插件依赖的响应。
type PluginDependencyResponse struct {
	PluginName   string                   `json:"plugin_name"`  // 插件名称 / Plugin name
	Dependencies []PluginDependencyConfig `json:"dependencies"` // 依赖列表 / Dependencies
}

// PluginDependencyStatus represents lightweight dependency readiness shown in plugin marketplace.
// PluginDependencyStatus 表示插件市场展示的轻量依赖状态。
type PluginDependencyStatus string

const (
	PluginDependencyStatusReadyExact      PluginDependencyStatus = "ready_exact"
	PluginDependencyStatusReadyFallback   PluginDependencyStatus = "ready_fallback"
	PluginDependencyStatusRuntimeAnalyzed PluginDependencyStatus = "runtime_analyzed"
	PluginDependencyStatusNotRequired     PluginDependencyStatus = "not_required"
	PluginDependencyStatusUnknown         PluginDependencyStatus = "unknown"
)

// DependencyResolutionMode represents how effective dependencies are resolved.
// DependencyResolutionMode 表示生效依赖的解析模式。
type DependencyResolutionMode string

const (
	DependencyResolutionModeExact    DependencyResolutionMode = "exact"
	DependencyResolutionModeFallback DependencyResolutionMode = "fallback"
	DependencyResolutionModeRuntime  DependencyResolutionMode = "runtime"
	DependencyResolutionModeNone     DependencyResolutionMode = "none"
)

// PluginCatalogSource represents the source of catalog entries.
// PluginCatalogSource 表示插件目录项来源。
type PluginCatalogSource string

const (
	PluginCatalogSourceSeed   PluginCatalogSource = "seed"
	PluginCatalogSourceRemote PluginCatalogSource = "remote"
)

// PluginCatalogEntry stores discovered connector metadata in DB.
// PluginCatalogEntry 持久化存储发现到的连接器元数据。
type PluginCatalogEntry struct {
	ID               uint                `gorm:"primaryKey" json:"id"`
	SeatunnelVersion string              `gorm:"size:50;not null;index:idx_plugin_catalog_version_name,unique" json:"seatunnel_version"`
	PluginName       string              `gorm:"size:120;not null;index:idx_plugin_catalog_version_name,unique" json:"plugin_name"`
	DisplayName      string              `gorm:"size:200;not null" json:"display_name"`
	ArtifactID       string              `gorm:"size:200;not null" json:"artifact_id"`
	GroupID          string              `gorm:"size:200;not null" json:"group_id"`
	Category         PluginCategory      `gorm:"size:50;not null" json:"category"`
	Description      string              `gorm:"type:text" json:"description"`
	DocURL           string              `gorm:"size:500" json:"doc_url"`
	Source           PluginCatalogSource `gorm:"size:20;not null;default:remote" json:"source"`
	SourceMirror     string              `gorm:"size:30" json:"source_mirror"`
	RefreshedAt      *time.Time          `json:"refreshed_at"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

func (PluginCatalogEntry) TableName() string { return "plugin_catalog_entries" }

// PluginDependencyProfileSource represents the source kind of official dependency profile.
// PluginDependencyProfileSource 表示官方依赖画像来源类型。
type PluginDependencyProfileSource string

const (
	PluginDependencyProfileSourceOfficialSeed    PluginDependencyProfileSource = "official_seed"
	PluginDependencyProfileSourceRuntimeAnalyzed PluginDependencyProfileSource = "runtime_analyzed"
)

// PluginDependencyProfile stores one official dependency profile for a plugin/version.
// PluginDependencyProfile 存储插件某个版本的一份官方依赖画像。
type PluginDependencyProfile struct {
	ID                       uint                          `gorm:"primaryKey" json:"id"`
	SeatunnelVersion         string                        `gorm:"size:50;not null;index:idx_plugin_dep_profile_unique,unique" json:"seatunnel_version"`
	PluginName               string                        `gorm:"size:120;not null;index:idx_plugin_dep_profile_unique,unique;index" json:"plugin_name"`
	ArtifactID               string                        `gorm:"size:200;not null" json:"artifact_id"`
	ProfileKey               string                        `gorm:"size:120;not null;default:default;index:idx_plugin_dep_profile_unique,unique" json:"profile_key"`
	ProfileName              string                        `gorm:"size:200" json:"profile_name"`
	EngineScope              string                        `gorm:"size:50;not null;default:zeta;index:idx_plugin_dep_profile_unique,unique" json:"engine_scope"`
	SourceKind               PluginDependencyProfileSource `gorm:"size:30;not null;index:idx_plugin_dep_profile_unique,unique" json:"source_kind"`
	BaselineVersionUsed      string                        `gorm:"size:50" json:"baseline_version_used"`
	ResolutionMode           DependencyResolutionMode      `gorm:"size:20;not null;default:exact" json:"resolution_mode"`
	TargetDir                string                        `gorm:"size:120;not null;default:lib" json:"target_dir"`
	AppliesTo                string                        `gorm:"size:255;not null;default:*" json:"applies_to"`
	IncludeVersions          string                        `gorm:"size:500" json:"include_versions"`
	ExcludedVersions         string                        `gorm:"size:500" json:"excluded_versions"`
	DocSlug                  string                        `gorm:"size:255" json:"doc_slug"`
	DocSourceURL             string                        `gorm:"size:500" json:"doc_source_url"`
	Confidence               string                        `gorm:"size:20;not null;default:medium" json:"confidence"`
	IsDefault                bool                          `gorm:"not null" json:"is_default"`
	NoAdditionalDependencies bool                          `gorm:"not null;default:false" json:"no_additional_dependencies"`
	ContentHash              string                        `gorm:"size:128" json:"content_hash"`
	CreatedAt                time.Time                     `json:"created_at"`
	UpdatedAt                time.Time                     `json:"updated_at"`
	Items                    []PluginDependencyProfileItem `gorm:"foreignKey:ProfileID;constraint:OnDelete:CASCADE" json:"items,omitempty"`
}

func (PluginDependencyProfile) TableName() string { return "plugin_dependency_profiles" }

// PluginDependencyProfileItem stores one dependency item under a profile.
// PluginDependencyProfileItem 存储画像中的单条依赖项。
type PluginDependencyProfileItem struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProfileID  uint      `gorm:"not null;index:idx_plugin_dep_profile_item_unique,unique" json:"profile_id"`
	GroupID    string    `gorm:"size:128;not null;index:idx_plugin_dep_profile_item_unique,unique" json:"group_id"`
	ArtifactID string    `gorm:"size:128;not null;index:idx_plugin_dep_profile_item_unique,unique" json:"artifact_id"`
	Version    string    `gorm:"size:64;not null;index:idx_plugin_dep_profile_item_unique,unique" json:"version"`
	TargetDir  string    `gorm:"size:64;not null;index:idx_plugin_dep_profile_item_unique,unique" json:"target_dir"`
	Required   bool      `gorm:"not null;default:true" json:"required"`
	SourceURL  string    `gorm:"size:500" json:"source_url"`
	Note       string    `gorm:"type:text" json:"note"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Disabled   bool      `gorm:"-" json:"disabled"`
	DisableID  *uint     `gorm:"-" json:"disable_id,omitempty"`
}

func (PluginDependencyProfileItem) TableName() string { return "plugin_dependency_profile_items" }

// OfficialDependenciesResponse contains resolved official dependency information for one plugin.
// OfficialDependenciesResponse 表示单个插件解析后的官方依赖信息。
type OfficialDependenciesResponse struct {
	PluginName               string                    `json:"plugin_name"`
	SeatunnelVersion         string                    `json:"seatunnel_version"`
	DependencyStatus         PluginDependencyStatus    `json:"dependency_status"`
	DependencyCount          int                       `json:"dependency_count"`
	BaselineVersionUsed      string                    `json:"baseline_version_used,omitempty"`
	DependencyResolutionMode DependencyResolutionMode  `json:"dependency_resolution_mode"`
	Profiles                 []PluginDependencyProfile `json:"profiles"`
	EffectiveDependencies    []PluginDependency        `json:"effective_dependencies"`
	DisabledDependencies     []PluginDependencyDisable `json:"disabled_dependencies,omitempty"`
}
