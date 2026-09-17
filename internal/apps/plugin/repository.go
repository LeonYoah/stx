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

package plugin

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Common errors / 常见错误
var (
	ErrPluginNotFound = errors.New("plugin not found / 插件未找到")
)

// Repository provides data access for installed plugins.
// Repository 提供已安装插件的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new Repository instance.
// NewRepository 创建一个新的 Repository 实例。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new installed plugin record.
// Create 创建一个新的已安装插件记录。
func (r *Repository) Create(ctx context.Context, plugin *InstalledPlugin) error {
	return r.db.WithContext(ctx).Create(plugin).Error
}

// GetByID retrieves an installed plugin by ID.
// GetByID 通过 ID 获取已安装插件。
func (r *Repository) GetByID(ctx context.Context, id uint) (*InstalledPlugin, error) {
	var plugin InstalledPlugin
	if err := r.db.WithContext(ctx).First(&plugin, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginNotFound
		}
		return nil, err
	}
	return &plugin, nil
}

// GetByClusterAndName retrieves an installed plugin by cluster ID and plugin name.
// GetByClusterAndName 通过集群 ID 和插件名称获取已安装插件。
func (r *Repository) GetByClusterAndName(ctx context.Context, clusterID uint, pluginName string) (*InstalledPlugin, error) {
	var plugin InstalledPlugin
	if err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND plugin_name = ?", clusterID, pluginName).
		First(&plugin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginNotFound
		}
		return nil, err
	}
	return &plugin, nil
}

// List retrieves installed plugins with optional filters.
// List 获取已安装插件列表，支持可选过滤条件。
func (r *Repository) List(ctx context.Context, filter *PluginFilter) ([]InstalledPlugin, int64, error) {
	var plugins []InstalledPlugin
	var total int64

	query := r.db.WithContext(ctx).Model(&InstalledPlugin{})

	// Apply filters / 应用过滤条件
	if filter != nil {
		if filter.ClusterID > 0 {
			query = query.Where("cluster_id = ?", filter.ClusterID)
		}
		if filter.Category != "" {
			query = query.Where("category = ?", filter.Category)
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.Keyword != "" {
			// 按插件名称模糊过滤（使用 LOWER 忽略大小写，兼容多数据库）
			// Filter by plugin name (case-insensitive using LOWER for multi-database compatibility)
			query = query.Where("LOWER(plugin_name) LIKE LOWER(?)", "%"+filter.Keyword+"%")
		}
	}

	// Count total / 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination / 应用分页
	if filter != nil && filter.PageSize > 0 {
		offset := 0
		if filter.Page > 1 {
			offset = (filter.Page - 1) * filter.PageSize
		}
		query = query.Offset(offset).Limit(filter.PageSize)
	}

	// Execute query / 执行查询
	if err := query.Order("installed_at DESC").Find(&plugins).Error; err != nil {
		return nil, 0, err
	}

	return plugins, total, nil
}

// ListByCluster retrieves all installed plugins for a specific cluster.
// ListByCluster 获取指定集群的所有已安装插件。
func (r *Repository) ListByCluster(ctx context.Context, clusterID uint) ([]InstalledPlugin, error) {
	var plugins []InstalledPlugin
	if err := r.db.WithContext(ctx).
		Where("cluster_id = ?", clusterID).
		Order("installed_at DESC").
		Find(&plugins).Error; err != nil {
		return nil, err
	}
	return plugins, nil
}

// Update updates an installed plugin record.
// Update 更新已安装插件记录。
func (r *Repository) Update(ctx context.Context, plugin *InstalledPlugin) error {
	return r.db.WithContext(ctx).Save(plugin).Error
}

// UpdateStatus updates the status of an installed plugin.
// UpdateStatus 更新已安装插件的状态。
func (r *Repository) UpdateStatus(ctx context.Context, id uint, status PluginStatus) error {
	return r.db.WithContext(ctx).
		Model(&InstalledPlugin{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// Delete deletes an installed plugin record.
// Delete 删除已安装插件记录。
func (r *Repository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&InstalledPlugin{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// DeleteByClusterAndName deletes an installed plugin by cluster ID and plugin name.
// DeleteByClusterAndName 通过集群 ID 和插件名称删除已安装插件。
func (r *Repository) DeleteByClusterAndName(ctx context.Context, clusterID uint, pluginName string) error {
	result := r.db.WithContext(ctx).
		Where("cluster_id = ? AND plugin_name = ?", clusterID, pluginName).
		Delete(&InstalledPlugin{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// ExistsByClusterAndName checks if a plugin is installed on a cluster.
// ExistsByClusterAndName 检查插件是否已安装在集群上。
func (r *Repository) ExistsByClusterAndName(ctx context.Context, clusterID uint, pluginName string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&InstalledPlugin{}).
		Where("cluster_id = ? AND plugin_name = ?", clusterID, pluginName).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ==================== Plugin Dependency Config Methods 插件依赖配置方法 ====================

// ListDependencies retrieves all user-added dependencies for a plugin/version.
// ListDependencies 获取插件指定版本的所有用户新增依赖。
func (r *Repository) ListDependencies(ctx context.Context, pluginName, seatunnelVersion string) ([]PluginDependencyConfig, error) {
	var deps []PluginDependencyConfig
	query := r.db.WithContext(ctx).Where("plugin_name = ?", pluginName)
	if seatunnelVersion != "" {
		query = query.Where("(seatunnel_version = ? OR seatunnel_version = '')", seatunnelVersion)
	}
	if err := query.
		Order("created_at ASC").
		Find(&deps).Error; err != nil {
		return nil, err
	}
	return deps, nil
}

// UpsertDependency creates or updates one dependency configuration.
// UpsertDependency 创建或更新一条依赖配置。
func (r *Repository) UpsertDependency(ctx context.Context, dep *PluginDependencyConfig) error {
	if dep == nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Select("*").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "plugin_name"},
				{Name: "seatunnel_version"},
				{Name: "group_id"},
				{Name: "artifact_id"},
				{Name: "version"},
				{Name: "target_dir"},
				{Name: "source_type"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"original_file_name", "stored_path", "file_size", "checksum", "updated_at"}),
		}).
		Create(dep).Error
}

// FindDependencyByNaturalKey finds one dependency config by its natural key.
// FindDependencyByNaturalKey 按自然键查找依赖配置。
func (r *Repository) FindDependencyByNaturalKey(ctx context.Context, pluginName, seatunnelVersion, groupID, artifactID, version, targetDir string, sourceType PluginDependencySource) (*PluginDependencyConfig, error) {
	var dep PluginDependencyConfig
	if err := r.db.WithContext(ctx).
		Where("plugin_name = ? AND seatunnel_version = ? AND group_id = ? AND artifact_id = ? AND version = ? AND target_dir = ? AND source_type = ?",
			pluginName, seatunnelVersion, groupID, artifactID, version, targetDir, sourceType).
		First(&dep).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &dep, nil
}

// DeleteDependency deletes a dependency configuration by ID.
// DeleteDependency 通过 ID 删除依赖配置。
func (r *Repository) DeleteDependency(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&PluginDependencyConfig{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("dependency not found / 依赖未找到")
	}
	return nil
}

// GetDependencyByID retrieves a dependency by ID.
// GetDependencyByID 通过 ID 获取依赖。
func (r *Repository) GetDependencyByID(ctx context.Context, id uint) (*PluginDependencyConfig, error) {
	var dep PluginDependencyConfig
	if err := r.db.WithContext(ctx).First(&dep, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("dependency not found / 依赖未找到")
		}
		return nil, err
	}
	return &dep, nil
}

// ListDependencyDisables lists disabled official dependencies for a plugin/version.
// ListDependencyDisables 获取插件指定版本的官方依赖禁用记录。
func (r *Repository) ListDependencyDisables(ctx context.Context, pluginName, seatunnelVersion string) ([]PluginDependencyDisable, error) {
	var items []PluginDependencyDisable
	query := r.db.WithContext(ctx).Where("plugin_name = ?", pluginName)
	if seatunnelVersion != "" {
		query = query.Where("seatunnel_version = ?", seatunnelVersion)
	}
	if err := query.Order("created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// UpsertDependencyDisable creates or updates one disabled official dependency record.
// UpsertDependencyDisable 创建或更新一条官方依赖禁用记录。
func (r *Repository) UpsertDependencyDisable(ctx context.Context, item *PluginDependencyDisable) error {
	if item == nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Select("*").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "plugin_name"},
				{Name: "seatunnel_version"},
				{Name: "group_id"},
				{Name: "artifact_id"},
				{Name: "version"},
				{Name: "target_dir"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"updated_at"}),
		}).
		Create(item).Error
}

// FindDependencyDisableByNaturalKey finds one disable record by natural key.
// FindDependencyDisableByNaturalKey 按自然键查找禁用记录。
func (r *Repository) FindDependencyDisableByNaturalKey(ctx context.Context, pluginName, seatunnelVersion, groupID, artifactID, version, targetDir string) (*PluginDependencyDisable, error) {
	var item PluginDependencyDisable
	if err := r.db.WithContext(ctx).
		Where("plugin_name = ? AND seatunnel_version = ? AND group_id = ? AND artifact_id = ? AND version = ? AND target_dir = ?",
			pluginName, seatunnelVersion, groupID, artifactID, version, targetDir).
		First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// DeleteDependencyDisable deletes a disable record by ID.
// DeleteDependencyDisable 通过 ID 删除禁用记录。
func (r *Repository) DeleteDependencyDisable(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&PluginDependencyDisable{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("disabled dependency not found / 禁用依赖未找到")
	}
	return nil
}

// GetDependencyDisableByID retrieves one disable record by ID.
// GetDependencyDisableByID 通过 ID 获取禁用记录。
func (r *Repository) GetDependencyDisableByID(ctx context.Context, id uint) (*PluginDependencyDisable, error) {
	var item PluginDependencyDisable
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("disabled dependency not found / 禁用依赖未找到")
		}
		return nil, err
	}
	return &item, nil
}

// Transaction runs fn within a DB transaction.
// Transaction 在数据库事务中执行 fn。
func (r *Repository) Transaction(ctx context.Context, fn func(tx *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

// ListCatalogEntriesByVersion returns persisted plugin catalog entries for a SeaTunnel version.
// ListCatalogEntriesByVersion 返回指定 SeaTunnel 版本的插件目录项。
func (r *Repository) ListCatalogEntriesByVersion(ctx context.Context, version string) ([]PluginCatalogEntry, error) {
	var entries []PluginCatalogEntry
	if err := r.db.WithContext(ctx).
		Where("seatunnel_version = ?", version).
		Order("plugin_name ASC").
		Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// UpsertCatalogEntries upserts plugin catalog entries by version + plugin name.
// UpsertCatalogEntries 按版本 + 插件名 upsert 插件目录项。
func (r *Repository) UpsertCatalogEntries(ctx context.Context, entries []PluginCatalogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Select("*").
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "seatunnel_version"}, {Name: "plugin_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"display_name", "artifact_id", "group_id", "category", "description", "doc_url", "source", "source_mirror", "refreshed_at", "updated_at"}),
		}).
		Create(&entries).Error
}

// ReplaceCatalogEntriesByVersion replaces all catalog entries for one version.
// ReplaceCatalogEntriesByVersion 替换指定版本的全部插件目录项。
func (r *Repository) ReplaceCatalogEntriesByVersion(ctx context.Context, version string, entries []PluginCatalogEntry) error {
	return r.Transaction(ctx, func(tx *Repository) error {
		if err := tx.db.WithContext(ctx).Where("seatunnel_version = ?", version).Delete(&PluginCatalogEntry{}).Error; err != nil {
			return err
		}
		if len(entries) == 0 {
			return nil
		}
		return tx.db.WithContext(ctx).Create(&entries).Error
	})
}

// ListDependencyProfilesByPlugin returns all dependency profiles for a plugin.
// ListDependencyProfilesByPlugin 返回插件的所有依赖画像。
func (r *Repository) ListDependencyProfilesByPlugin(ctx context.Context, pluginName string) ([]PluginDependencyProfile, error) {
	var profiles []PluginDependencyProfile
	if err := r.db.WithContext(ctx).
		Preload("Items").
		Where("plugin_name = ?", pluginName).
		Order("seatunnel_version DESC, source_kind ASC, profile_key ASC").
		Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// UpsertDependencyProfile upserts one profile and replaces its items.
// UpsertDependencyProfile upsert 一份依赖画像并替换其子项。
func (r *Repository) UpsertDependencyProfile(ctx context.Context, profile *PluginDependencyProfile) error {
	if profile == nil {
		return nil
	}
	return r.Transaction(ctx, func(tx *Repository) error {
		base := *profile
		base.Items = nil
		if err := tx.db.WithContext(ctx).
			Select("*").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "seatunnel_version"}, {Name: "plugin_name"}, {Name: "profile_key"}, {Name: "engine_scope"}, {Name: "source_kind"}},
				DoUpdates: clause.AssignmentColumns([]string{"artifact_id", "profile_name", "baseline_version_used", "resolution_mode", "target_dir", "applies_to", "include_versions", "excluded_versions", "doc_slug", "doc_source_url", "confidence", "is_default", "no_additional_dependencies", "content_hash", "updated_at"}),
			}).
			Create(&base).Error; err != nil {
			return err
		}

		var persisted PluginDependencyProfile
		if err := tx.db.WithContext(ctx).
			Where("seatunnel_version = ? AND plugin_name = ? AND profile_key = ? AND engine_scope = ? AND source_kind = ?", profile.SeatunnelVersion, profile.PluginName, profile.ProfileKey, profile.EngineScope, profile.SourceKind).
			First(&persisted).Error; err != nil {
			return err
		}

		if err := tx.db.WithContext(ctx).Where("profile_id = ?", persisted.ID).Delete(&PluginDependencyProfileItem{}).Error; err != nil {
			return err
		}

		if len(profile.Items) == 0 {
			return nil
		}
		items := make([]PluginDependencyProfileItem, 0, len(profile.Items))
		for _, item := range profile.Items {
			item.ProfileID = persisted.ID
			items = append(items, item)
		}
		return tx.db.WithContext(ctx).Create(&items).Error
	})
}

// DeleteStaleDependencyProfiles removes official profiles that are no longer present in the latest seed for one version.
// DeleteStaleDependencyProfiles 删除某个版本中最新 seed 已不存在的官方依赖画像。
func (r *Repository) DeleteStaleDependencyProfiles(ctx context.Context, seatunnelVersion string, sourceKind PluginDependencyProfileSource, keepKeys map[string]struct{}) error {
	return r.Transaction(ctx, func(tx *Repository) error {
		var profiles []PluginDependencyProfile
		if err := tx.db.WithContext(ctx).
			Where("seatunnel_version = ? AND source_kind = ?", seatunnelVersion, sourceKind).
			Find(&profiles).Error; err != nil {
			return err
		}
		for _, profile := range profiles {
			key := profile.PluginName + ":" + profile.ProfileKey + ":" + profile.EngineScope
			if _, ok := keepKeys[key]; ok {
				continue
			}
			if err := tx.db.WithContext(ctx).Where("profile_id = ?", profile.ID).Delete(&PluginDependencyProfileItem{}).Error; err != nil {
				return err
			}
			if err := tx.db.WithContext(ctx).Delete(&PluginDependencyProfile{}, profile.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
