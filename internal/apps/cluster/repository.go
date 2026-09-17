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
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository provides data access operations for Cluster and ClusterNode entities.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new Repository instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction executes fn inside one database transaction.
// Transaction 在一个数据库事务内执行 fn。
func (r *Repository) Transaction(ctx context.Context, fn func(tx *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

// Create creates a new cluster record in the database.
// Returns ErrClusterNameDuplicate if a cluster with the same name already exists.
// Returns ErrClusterNameEmpty if the cluster name is empty.
func (r *Repository) Create(ctx context.Context, cluster *Cluster) error {
	// Validate cluster name is not empty
	if cluster.Name == "" {
		return ErrClusterNameEmpty
	}

	// Check for duplicate name
	var count int64
	if err := r.db.WithContext(ctx).Model(&Cluster{}).Where("name = ?", cluster.Name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrClusterNameDuplicate
	}

	return r.db.WithContext(ctx).Create(cluster).Error
}

// GetByID retrieves a cluster by its ID with optional node preloading.
// Returns ErrClusterNotFound if the cluster does not exist.
func (r *Repository) GetByID(ctx context.Context, id uint, preloadNodes bool) (*Cluster, error) {
	var cluster Cluster
	query := r.db.WithContext(ctx)
	if preloadNodes {
		query = query.Preload("Nodes")
	}
	if err := query.First(&cluster, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrClusterNotFound
		}
		return nil, err
	}
	return &cluster, nil
}

// GetByName retrieves a cluster by its name.
// Returns ErrClusterNotFound if no cluster with the given name exists.
func (r *Repository) GetByName(ctx context.Context, name string) (*Cluster, error) {
	var cluster Cluster
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&cluster).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrClusterNotFound
		}
		return nil, err
	}
	return &cluster, nil
}

// List retrieves clusters based on filter criteria with pagination.
// Returns the list of clusters and total count.
func (r *Repository) List(ctx context.Context, filter *ClusterFilter) ([]*Cluster, int64, error) {
	query := r.db.WithContext(ctx).Model(&Cluster{})

	// Apply filters
	if filter != nil {
		if filter.Name != "" {
			// 按集群名称模糊过滤（使用 LOWER 忽略大小写，兼容多数据库）
			// Filter by cluster name (case-insensitive using LOWER for multi-database compatibility)
			query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+filter.Name+"%")
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.DeploymentMode != "" {
			query = query.Where("deployment_mode = ?", filter.DeploymentMode)
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

	// Execute query with nodes preloaded
	var clusters []*Cluster
	if err := query.Preload("Nodes").Order("created_at DESC").Find(&clusters).Error; err != nil {
		return nil, 0, err
	}

	return clusters, total, nil
}

// Update updates an existing cluster record.
// Returns ErrClusterNotFound if the cluster does not exist.
// Returns ErrClusterNameDuplicate if updating to a name that already exists.
func (r *Repository) Update(ctx context.Context, cluster *Cluster) error {
	// Check if cluster exists
	var existing Cluster
	if err := r.db.WithContext(ctx).First(&existing, cluster.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrClusterNotFound
		}
		return err
	}

	// Check for duplicate name if name is being changed
	if cluster.Name != "" && cluster.Name != existing.Name {
		var count int64
		if err := r.db.WithContext(ctx).Model(&Cluster{}).Where("name = ? AND id != ?", cluster.Name, cluster.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrClusterNameDuplicate
		}
	}

	return r.db.WithContext(ctx).Save(cluster).Error
}

// Delete removes a cluster record from the database.
// Returns ErrClusterNotFound if the cluster does not exist.
// Note: This also deletes all associated cluster nodes due to foreign key cascade.
func (r *Repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete associated nodes first
		if err := tx.Where("cluster_id = ?", id).Delete(&ClusterNode{}).Error; err != nil {
			return err
		}

		// Delete the cluster
		result := tx.Delete(&Cluster{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrClusterNotFound
		}
		return nil
	})
}

// UpdateStatus updates the status of a cluster.
func (r *Repository) UpdateStatus(ctx context.Context, id uint, status ClusterStatus) error {
	result := r.db.WithContext(ctx).Model(&Cluster{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrClusterNotFound
	}
	return nil
}

// ExistsByName checks if a cluster with the given name exists.
func (r *Repository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Cluster{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Node operations

// AddNode adds a node to a cluster.
// Returns ErrClusterNotFound if the cluster does not exist.
// Returns ErrNodeAlreadyExists if the host already has a node with the same role in the cluster.
// 如果集群不存在返回 ErrClusterNotFound。
// 如果主机在集群中已有相同角色的节点返回 ErrNodeAlreadyExists。
func (r *Repository) AddNode(ctx context.Context, node *ClusterNode) error {
	// Check if cluster exists
	// 检查集群是否存在
	var cluster Cluster
	if err := r.db.WithContext(ctx).First(&cluster, node.ClusterID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrClusterNotFound
		}
		return err
	}

	// Check if node with same role already exists for this host in this cluster
	// 检查此主机在此集群中是否已有相同角色的节点
	// In separated mode, a host can have both master and worker roles
	// 在分离模式下，一个主机可以同时有 master 和 worker 角色
	var count int64
	if err := r.db.WithContext(ctx).Model(&ClusterNode{}).
		Where("cluster_id = ? AND host_id = ? AND role = ?", node.ClusterID, node.HostID, node.Role).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrNodeAlreadyExists
	}

	return r.db.WithContext(ctx).Create(node).Error
}

// GetNodeByID retrieves a cluster node by its ID.
// Returns ErrNodeNotFound if the node does not exist.
func (r *Repository) GetNodeByID(ctx context.Context, id uint) (*ClusterNode, error) {
	var node ClusterNode
	if err := r.db.WithContext(ctx).First(&node, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}
	return &node, nil
}

// GetNodeByHostID retrieves the first cluster node for a specific host.
// GetNodeByHostID 获取特定主机的第一个集群节点。
func (r *Repository) GetNodeByHostID(ctx context.Context, hostID uint) (*ClusterNode, error) {
	var node ClusterNode
	if err := r.db.WithContext(ctx).Where("host_id = ?", hostID).First(&node).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &node, nil
}

// GetNodesByClusterID retrieves all nodes for a cluster.
func (r *Repository) GetNodesByClusterID(ctx context.Context, clusterID uint) ([]*ClusterNode, error) {
	var nodes []*ClusterNode
	if err := r.db.WithContext(ctx).Where("cluster_id = ?", clusterID).Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

// GetNodeByClusterAndHost retrieves a cluster node by cluster ID and host ID.
// GetNodeByClusterAndHost 根据集群 ID 和主机 ID 获取集群节点。
// When a host has multiple nodes (e.g. master + worker in separated mode), use GetNodeByClusterAndHostAndRole instead.
// 当同一主机有多个节点（如分离模式下 master+worker）时，请使用 GetNodeByClusterAndHostAndRole。
func (r *Repository) GetNodeByClusterAndHost(ctx context.Context, clusterID uint, hostID uint) (*ClusterNode, error) {
	var node ClusterNode
	if err := r.db.WithContext(ctx).Where("cluster_id = ? AND host_id = ?", clusterID, hostID).First(&node).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &node, nil
}

// GetNodeByClusterAndHostAndRole retrieves a cluster node by cluster ID, host ID and role.
// GetNodeByClusterAndHostAndRole 根据集群 ID、主机 ID 和角色获取集群节点。
// Used when one host has both master and worker nodes (separated mode).
// 用于同一主机同时有 master 和 worker 节点（分离模式）时。
func (r *Repository) GetNodeByClusterAndHostAndRole(ctx context.Context, clusterID uint, hostID uint, role string) (*ClusterNode, error) {
	var node ClusterNode
	if err := r.db.WithContext(ctx).Where("cluster_id = ? AND host_id = ? AND role = ?", clusterID, hostID, role).First(&node).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &node, nil
}

// GetNodesByHostID retrieves all cluster nodes for a specific host.
// This is useful for checking if a host is associated with any clusters.
func (r *Repository) GetNodesByHostID(ctx context.Context, hostID uint) ([]*ClusterNode, error) {
	var nodes []*ClusterNode
	if err := r.db.WithContext(ctx).Where("host_id = ?", hostID).Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

// RemoveNode removes a node from a cluster.
// Returns ErrNodeNotFound if the node does not exist.
func (r *Repository) RemoveNode(ctx context.Context, nodeID uint) error {
	result := r.db.WithContext(ctx).Delete(&ClusterNode{}, nodeID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// RemoveNodeByClusterAndHost removes a node by cluster ID and host ID.
// Returns ErrNodeNotFound if the node does not exist.
func (r *Repository) RemoveNodeByClusterAndHost(ctx context.Context, clusterID, hostID uint) error {
	result := r.db.WithContext(ctx).Where("cluster_id = ? AND host_id = ?", clusterID, hostID).Delete(&ClusterNode{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// UpdateNodeStatus updates the status of a cluster node.
func (r *Repository) UpdateNodeStatus(ctx context.Context, nodeID uint, status NodeStatus) error {
	result := r.db.WithContext(ctx).Model(&ClusterNode{}).Where("id = ?", nodeID).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// UpdateNodeProcess updates the process information for a cluster node.
func (r *Repository) UpdateNodeProcess(ctx context.Context, nodeID uint, pid int, processStatus string) error {
	return r.UpdateNodeProcessStatus(ctx, nodeID, pid, processStatus)
}

// UpdateNode updates a cluster node's configuration.
// UpdateNode 更新集群节点的配置。
func (r *Repository) UpdateNode(ctx context.Context, node *ClusterNode) error {
	result := r.db.WithContext(ctx).Save(node)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// CountNodesByClusterID returns the number of nodes in a cluster.
func (r *Repository) CountNodesByClusterID(ctx context.Context, clusterID uint) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&ClusterNode{}).Where("cluster_id = ?", clusterID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetClustersWithHostID retrieves all clusters that have a specific host as a node.
// This is useful for checking cluster associations before deleting a host.
func (r *Repository) GetClustersWithHostID(ctx context.Context, hostID uint) ([]*Cluster, error) {
	var clusters []*Cluster
	if err := r.db.WithContext(ctx).
		Joins("JOIN cluster_nodes ON cluster_nodes.cluster_id = clusters.id").
		Where("cluster_nodes.host_id = ?", hostID).
		Find(&clusters).Error; err != nil {
		return nil, err
	}
	return clusters, nil
}

// GetNodeByHostAndInstallDirAndRole retrieves a cluster node by host ID, install directory and role.
// GetNodeByHostAndInstallDirAndRole 根据主机 ID、安装目录和角色获取集群节点。
// Returns clusterID, nodeID, found, error
func (r *Repository) GetNodeByHostAndInstallDirAndRole(ctx context.Context, hostID uint, installDir, role string) (uint, uint, bool, error) {
	var node ClusterNode
	if err := r.db.WithContext(ctx).Where("host_id = ? AND install_dir = ? AND role = ?", hostID, installDir, role).First(&node).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	return node.ClusterID, node.ID, true, nil
}

// UpdateNodeProcessStatus updates the process PID, status, and last event time for a cluster node.
// UpdateNodeProcessStatus 更新集群节点的进程 PID、状态和最后事件时间。
// Uses the unified 'status' field (not 'process_status') for node state.
// 使用统一的 'status' 字段（而非 'process_status'）表示节点状态。
// Also updates 'last_event_at' to track when the last process event occurred.
// 同时更新 'last_event_at' 以跟踪最后一次进程事件发生的时间。
func (r *Repository) UpdateNodeProcessStatus(ctx context.Context, nodeID uint, pid int, processStatus string) error {
	// Map process status to node status / 将进程状态映射到节点状态
	var nodeStatus NodeStatus
	switch processStatus {
	case "running":
		nodeStatus = NodeStatusRunning
	case "stopped":
		nodeStatus = NodeStatusStopped
	case "crashed":
		nodeStatus = NodeStatusError
	default:
		nodeStatus = NodeStatusStopped
	}

	now := time.Now()
	updates := map[string]interface{}{
		"process_pid":   pid,
		"status":        nodeStatus,
		"last_event_at": now, // Update last event time / 更新最后事件时间
	}

	result := r.db.WithContext(ctx).Model(&ClusterNode{}).Where("id = ?", nodeID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// GetNodesByClusterIDWithHost retrieves all nodes for a cluster with host information.
// GetNodesByClusterIDWithHost 获取集群的所有节点及其主机信息。
func (r *Repository) GetNodesByClusterIDWithHost(ctx context.Context, clusterID uint) ([]*ClusterNode, error) {
	var nodes []*ClusterNode
	if err := r.db.WithContext(ctx).Where("cluster_id = ?", clusterID).Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}
