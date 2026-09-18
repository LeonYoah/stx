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

package host

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository provides data access operations for Host entities.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new Repository instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new host record in the database.
// Create 在数据库中创建新的主机记录。
// Returns ErrHostNameDuplicate if a host with the same name already exists.
// Returns ErrHostIPDuplicate if a host with the same IP already exists.
// Returns ErrHostIPInvalid if the IP address format is invalid (for bare_metal).
func (r *Repository) Create(ctx context.Context, host *Host) error {
	// Validate host name is not empty
	// 验证主机名不为空
	if host.Name == "" {
		return ErrHostNameEmpty
	}

	// Validate IP address format for bare_metal hosts
	// 验证物理机/VM 的 IP 地址格式
	if host.HostType == HostTypeBareMetal || host.HostType == "" {
		if !ValidateIPAddress(host.IPAddress) {
			return ErrHostIPInvalid
		}
	}

	// Check for duplicate name
	// 检查名称是否重复
	var count int64
	if err := r.db.WithContext(ctx).Model(&Host{}).Where("name = ?", host.Name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrHostNameDuplicate
	}

	// Check for duplicate IP (bare_metal)
	// 检查 IP 是否重复（物理机/VM）
	if (host.HostType == HostTypeBareMetal || host.HostType == "") && host.IPAddress != "" {
		count = 0
		if err := r.db.WithContext(ctx).Model(&Host{}).Where("ip_address = ?", host.IPAddress).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrHostIPDuplicate
		}
	}

	return r.db.WithContext(ctx).Create(host).Error
}

// GetByID retrieves a host by its ID.
// Returns ErrHostNotFound if the host does not exist.
func (r *Repository) GetByID(ctx context.Context, id uint) (*Host, error) {
	var host Host
	if err := r.db.WithContext(ctx).First(&host, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHostNotFound
		}
		return nil, err
	}
	return &host, nil
}

// GetByIP retrieves a host by its IP address.
// Returns ErrHostNotFound if no host with the given IP exists.
func (r *Repository) GetByIP(ctx context.Context, ip string) (*Host, error) {
	var host Host
	if err := r.db.WithContext(ctx).Where("ip_address = ?", ip).First(&host).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHostNotFound
		}
		return nil, err
	}
	return &host, nil
}

// GetByName retrieves a host by its name.
// Returns ErrHostNotFound if no host with the given name exists.
func (r *Repository) GetByName(ctx context.Context, name string) (*Host, error) {
	var host Host
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&host).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHostNotFound
		}
		return nil, err
	}
	return &host, nil
}

// GetByAgentID retrieves a host by its Agent ID.
// Returns ErrHostNotFound if no host with the given Agent ID exists.
func (r *Repository) GetByAgentID(ctx context.Context, agentID string) (*Host, error) {
	var host Host
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&host).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHostNotFound
		}
		return nil, err
	}
	return &host, nil
}

// List retrieves hosts based on filter criteria with pagination.
// List 根据过滤条件和分页获取主机列表。
// since: when non-zero, IsOnline filter uses heartbeat after since (e.g. process start).
// Returns the list of hosts and total count.
func (r *Repository) List(ctx context.Context, filter *HostFilter, heartbeatTimeout time.Duration, since time.Time) ([]*Host, int64, error) {
	query := r.db.WithContext(ctx).Model(&Host{})

	// Apply filters / 应用过滤条件
	if filter != nil {
		if filter.Name != "" {
			// 按主机名称模糊过滤（使用 LOWER 忽略大小写，兼容多数据库）
			// Filter by host name (case-insensitive using LOWER for multi-database compatibility)
			query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+filter.Name+"%")
		}
		// Filter by host type / 按主机类型过滤
		if filter.HostType != "" {
			query = query.Where("host_type = ?", filter.HostType)
		}
		if filter.IPAddress != "" {
			// 按 IP 地址模糊过滤（使用 LOWER 忽略大小写，兼容多数据库）
			// Filter by IP address (case-insensitive using LOWER for multi-database compatibility)
			query = query.Where("LOWER(ip_address) LIKE LOWER(?)", "%"+filter.IPAddress+"%")
		}
		// Filter by host status / 按主机状态过滤
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.AgentStatus != "" {
			query = query.Where("agent_status = ?", filter.AgentStatus)
		}
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if filter != nil && filter.PageSize > 0 {
		offset := 0
		if filter.Page > 0 {
			offset = (filter.Page - 1) * filter.PageSize
		}
		query = query.Offset(offset).Limit(filter.PageSize)
	}

	// Execute query
	var hosts []*Host
	if err := query.Order("created_at DESC").Find(&hosts).Error; err != nil {
		return nil, 0, err
	}

	// Filter by online status if specified (use since for consistency with display)
	if filter != nil && filter.IsOnline != nil {
		filteredHosts := make([]*Host, 0)
		for _, h := range hosts {
			if h.IsOnlineWithSince(heartbeatTimeout, since) == *filter.IsOnline {
				filteredHosts = append(filteredHosts, h)
			}
		}
		hosts = filteredHosts
	}

	return hosts, total, nil
}

// Update updates an existing host record.
// Returns ErrHostNotFound if the host does not exist.
// Returns ErrHostNameDuplicate if updating to a name that already exists.
// Returns ErrHostIPDuplicate if updating to an IP that already exists.
// Returns ErrHostIPInvalid if the IP address format is invalid.
func (r *Repository) Update(ctx context.Context, host *Host) error {
	// Check if host exists
	var existing Host
	if err := r.db.WithContext(ctx).First(&existing, host.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrHostNotFound
		}
		return err
	}

	// Validate IP address if being updated
	if host.IPAddress != "" && !ValidateIPAddress(host.IPAddress) {
		return ErrHostIPInvalid
	}

	// Check for duplicate name if name is being changed
	if host.Name != "" && host.Name != existing.Name {
		var count int64
		if err := r.db.WithContext(ctx).Model(&Host{}).Where("name = ? AND id != ?", host.Name, host.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrHostNameDuplicate
		}
	}

	// Check for duplicate IP if changed
	// 如果 IP 发生变化，检查是否与其他主机重复
	if host.IPAddress != "" && host.IPAddress != existing.IPAddress {
		var count int64
		if err := r.db.WithContext(ctx).Model(&Host{}).Where("ip_address = ? AND id != ?", host.IPAddress, host.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrHostIPDuplicate
		}
	}

	return r.db.WithContext(ctx).Save(host).Error
}

// Delete removes a host record from the database.
// Returns ErrHostNotFound if the host does not exist.
// Note: Cluster association check should be done at the service layer.
func (r *Repository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&Host{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrHostNotFound
	}
	return nil
}

// UpdateAgentStatus updates the agent status and related fields for a host.
// Also updates the host status based on agent status:
// - AgentStatusInstalled -> HostStatusConnected
// - AgentStatusOffline -> HostStatusOffline
func (r *Repository) UpdateAgentStatus(ctx context.Context, id uint, status AgentStatus, agentID string, version string) error {
	updates := map[string]interface{}{
		"agent_status":  status,
		"agent_id":      agentID,
		"agent_version": version,
	}

	// Update host status based on agent status
	// 根据 Agent 状态更新主机状态
	switch status {
	case AgentStatusInstalled:
		updates["status"] = HostStatusConnected
	case AgentStatusOffline:
		updates["status"] = HostStatusOffline
	}

	result := r.db.WithContext(ctx).Model(&Host{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrHostNotFound
	}
	return nil
}

// UpdateHeartbeat updates the heartbeat timestamp and resource usage for a host.
func (r *Repository) UpdateHeartbeat(ctx context.Context, id uint, cpuUsage, memoryUsage, diskUsage float64) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&Host{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_heartbeat": now,
		"cpu_usage":      cpuUsage,
		"memory_usage":   memoryUsage,
		"disk_usage":     diskUsage,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrHostNotFound
	}
	return nil
}

// UpdateSystemInfo updates the system information for a host.
func (r *Repository) UpdateSystemInfo(ctx context.Context, id uint, osType, arch string, cpuCores int, totalMemory, totalDisk int64) error {
	result := r.db.WithContext(ctx).Model(&Host{}).Where("id = ?", id).Updates(map[string]interface{}{
		"os_type":      osType,
		"arch":         arch,
		"cpu_cores":    cpuCores,
		"total_memory": totalMemory,
		"total_disk":   totalDisk,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrHostNotFound
	}
	return nil
}

// MarkOfflineHosts marks all hosts as offline if their last heartbeat exceeds the timeout.
func (r *Repository) MarkOfflineHosts(ctx context.Context, timeout time.Duration) (int64, error) {
	cutoff := time.Now().Add(-timeout)
	result := r.db.WithContext(ctx).Model(&Host{}).
		Where("agent_status = ? AND last_heartbeat < ?", AgentStatusInstalled, cutoff).
		Update("agent_status", AgentStatusOffline)
	return result.RowsAffected, result.Error
}

// ExistsByName checks if a host with the given name exists.
func (r *Repository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Host{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
