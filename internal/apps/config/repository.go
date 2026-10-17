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

package config

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

var (
	ErrConfigNotFound  = errors.New("config not found")
	ErrVersionNotFound = errors.New("config version not found")
)

// Repository 配置数据仓库
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建配置仓库实例
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 创建配置
func (r *Repository) Create(ctx context.Context, config *Config) error {
	return r.db.WithContext(ctx).Create(config).Error
}

// GetByID 根据 ID 获取配置
func (r *Repository) GetByID(ctx context.Context, id uint) (*Config, error) {
	var config Config
	err := r.db.WithContext(ctx).First(&config, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConfigNotFound
	}
	return &config, err
}

// GetTemplate 获取集群模板配置
func (r *Repository) GetTemplate(ctx context.Context, clusterID uint, configType ConfigType) (*Config, error) {
	var config Config
	err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND host_id IS NULL AND config_type = ?", clusterID, configType).
		First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConfigNotFound
	}
	return &config, err
}

// GetNodeConfig 获取节点配置
func (r *Repository) GetNodeConfig(ctx context.Context, clusterID uint, hostID uint, configType ConfigType) (*Config, error) {
	var config Config
	err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND host_id = ? AND config_type = ?", clusterID, hostID, configType).
		First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConfigNotFound
	}
	return &config, err
}

// List 获取配置列表
func (r *Repository) List(ctx context.Context, filter *ConfigFilter) ([]*Config, error) {
	var configs []*Config
	query := r.db.WithContext(ctx).Model(&Config{})

	if filter.ClusterID > 0 {
		query = query.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.HostID != nil {
		query = query.Where("host_id = ?", *filter.HostID)
	}
	if filter.ConfigType != "" {
		query = query.Where("config_type = ?", filter.ConfigType)
	}
	if filter.OnlyTemplate {
		query = query.Where("host_id IS NULL")
	}

	err := query.Order("config_type, host_id").Find(&configs).Error
	return configs, err
}

// ListByCluster 获取集群所有配置（包括模板和节点配置）
func (r *Repository) ListByCluster(ctx context.Context, clusterID uint) ([]*Config, error) {
	var configs []*Config
	err := r.db.WithContext(ctx).
		Where("cluster_id = ?", clusterID).
		Order("config_type, host_id").
		Find(&configs).Error
	return configs, err
}

// ListUpdatedByClusterBetween returns configs updated within the given time window.
// ListUpdatedByClusterBetween 返回指定时间窗口内更新过的配置记录。
func (r *Repository) ListUpdatedByClusterBetween(ctx context.Context, clusterID uint, start, end time.Time) ([]*Config, error) {
	var configs []*Config
	query := r.db.WithContext(ctx).Model(&Config{}).Where("cluster_id = ?", clusterID)
	if !start.IsZero() {
		query = query.Where("updated_at >= ?", timex.Bound(start))
	}
	if !end.IsZero() {
		query = query.Where("updated_at <= ?", timex.Bound(end))
	}
	err := query.Order("updated_at DESC").Order("config_type").Find(&configs).Error
	return configs, err
}

// ListNodeConfigs 获取集群某类型的所有节点配置
func (r *Repository) ListNodeConfigs(ctx context.Context, clusterID uint, configType ConfigType) ([]*Config, error) {
	var configs []*Config
	err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND config_type = ? AND host_id IS NOT NULL", clusterID, configType).
		Find(&configs).Error
	return configs, err
}

// Update 更新配置
func (r *Repository) Update(ctx context.Context, config *Config) error {
	return r.db.WithContext(ctx).Save(config).Error
}

// Delete 删除配置
func (r *Repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&Config{}, id).Error
}

// DeleteByCluster 删除集群所有配置
func (r *Repository) DeleteByCluster(ctx context.Context, clusterID uint) error {
	return r.db.WithContext(ctx).Where("cluster_id = ?", clusterID).Delete(&Config{}).Error
}

// CreateVersion 创建配置版本
func (r *Repository) CreateVersion(ctx context.Context, version *ConfigVersion) error {
	return r.db.WithContext(ctx).Create(version).Error
}

// GetVersionByID 根据 ID 获取版本
func (r *Repository) GetVersionByID(ctx context.Context, id uint) (*ConfigVersion, error) {
	var version ConfigVersion
	err := r.db.WithContext(ctx).First(&version, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrVersionNotFound
	}
	return &version, err
}

// GetVersion 获取指定版本
func (r *Repository) GetVersion(ctx context.Context, configID uint, version int) (*ConfigVersion, error) {
	var v ConfigVersion
	err := r.db.WithContext(ctx).
		Where("config_id = ? AND version = ?", configID, version).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrVersionNotFound
	}
	return &v, err
}

// ListVersions 获取配置的版本历史
func (r *Repository) ListVersions(ctx context.Context, configID uint) ([]*ConfigVersion, error) {
	var versions []*ConfigVersion
	err := r.db.WithContext(ctx).
		Where("config_id = ?", configID).
		Order("version DESC").
		Find(&versions).Error
	return versions, err
}

// DeleteVersionsByConfig 删除配置的所有版本
func (r *Repository) DeleteVersionsByConfig(ctx context.Context, configID uint) error {
	return r.db.WithContext(ctx).Where("config_id = ?", configID).Delete(&ConfigVersion{}).Error
}

// Transaction 执行事务
func (r *Repository) Transaction(ctx context.Context, fn func(tx *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}
