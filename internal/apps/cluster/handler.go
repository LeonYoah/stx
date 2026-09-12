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

// Package cluster provides cluster management functionality for the STX Agent system.
// cluster 包提供 STX Agent 系统的集群管理功能。
package cluster

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP handlers for cluster management operations.
// Handler 提供集群管理操作的 HTTP 处理器。
type Handler struct {
	service             *Service
	auditRepo           *audit.Repository
	onOperationExecuted func(context.Context, *OperationEvent) error
}

// OperationEvent represents one cluster operation notification hook payload.
// OperationEvent 表示一条集群操作通知钩子载荷。
type OperationEvent struct {
	ClusterID   uint
	ClusterName string
	NodeID      uint
	HostID      uint
	HostName    string
	HostIP      string
	Role        string
	Operation   OperationType
	Success     bool
	Message     string
	Operator    string
	Trigger     string
}

// NewHandler creates a new Handler instance.
// NewHandler 创建一个新的 Handler 实例。
// auditRepo may be nil; audit logging is skipped when nil.
func NewHandler(service *Service, auditRepo *audit.Repository) *Handler {
	return &Handler{service: service, auditRepo: auditRepo}
}

// SetOnOperationExecuted sets an optional hook invoked after a cluster operation HTTP request succeeds.
// SetOnOperationExecuted 设置集群操作 HTTP 请求成功后的可选回调。
func (h *Handler) SetOnOperationExecuted(fn func(context.Context, *OperationEvent) error) {
	h.onOperationExecuted = fn
}

func (h *Handler) notifyOperationExecuted(ctx context.Context, event *OperationEvent) {
	if h == nil || h.onOperationExecuted == nil || event == nil {
		return
	}
	if err := h.onOperationExecuted(ctx, event); err != nil {
		logger.WarnF(ctx, "[Cluster] operation event hook failed: cluster_id=%d, operation=%s, err=%v", event.ClusterID, event.Operation, err)
	}
}

func (h *Handler) buildNodeOperationEvent(ctx context.Context, clusterID uint, nodeID uint, operation OperationType, result *OperationResult, operator string) *OperationEvent {
	if h == nil || h.service == nil {
		return nil
	}

	clusterName, _ := h.service.GetClusterNodeDisplayInfo(ctx, clusterID, nodeID)
	event := &OperationEvent{
		ClusterID:   clusterID,
		ClusterName: clusterName,
		NodeID:      nodeID,
		Operation:   operation,
		Success:     result != nil && result.Success,
		Operator:    strings.TrimSpace(operator),
		Trigger:     "manual_api",
	}
	if result != nil {
		event.Message = strings.TrimSpace(result.Message)
	}

	nodes, err := h.service.GetNodes(ctx, clusterID)
	if err != nil {
		return event
	}
	for _, node := range nodes {
		if node == nil || node.ID != nodeID {
			continue
		}
		event.HostID = node.HostID
		event.HostName = strings.TrimSpace(node.HostName)
		event.HostIP = strings.TrimSpace(node.HostIP)
		event.Role = strings.TrimSpace(string(node.Role))
		break
	}
	return event
}

// ==================== Request/Response Types 请求/响应类型 ====================

// ListClustersRequest represents the request for listing clusters.
// ListClustersRequest 表示获取集群列表的请求。
type ListClustersRequest struct {
	Current        int            `json:"current" form:"current" binding:"min=1"`
	Size           int            `json:"size" form:"size" binding:"min=1,max=100"`
	Name           string         `json:"name" form:"name"`
	Status         ClusterStatus  `json:"status" form:"status"`
	DeploymentMode DeploymentMode `json:"deployment_mode" form:"deployment_mode"`
}

// ListClustersResponse represents the response for listing clusters.
// ListClustersResponse 表示获取集群列表的响应。
type ListClustersResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *struct {
		Total    int64          `json:"total"`
		Clusters []*ClusterInfo `json:"clusters"`
	} `json:"data"`
}

// CreateClusterResponse represents the response for creating a cluster.
// CreateClusterResponse 表示创建集群的响应。
type CreateClusterResponse struct {
	ErrorMsg string       `json:"error_msg"`
	Data     *ClusterInfo `json:"data"`
}

// GetClusterResponse represents the response for getting a cluster.
// GetClusterResponse 表示获取集群详情的响应。
type GetClusterResponse struct {
	ErrorMsg string       `json:"error_msg"`
	Data     *ClusterInfo `json:"data"`
}

// UpdateClusterResponse represents the response for updating a cluster.
// UpdateClusterResponse 表示更新集群的响应。
type UpdateClusterResponse struct {
	ErrorMsg string       `json:"error_msg"`
	Data     *ClusterInfo `json:"data"`
}

// DeleteClusterResponse represents the response for deleting a cluster.
// DeleteClusterResponse 表示删除集群的响应。
type DeleteClusterResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     any    `json:"data"`
}

// AddNodeResponse represents the response for adding a node to a cluster.
// AddNodeResponse 表示向集群添加节点的响应。
type AddNodeResponse struct {
	ErrorMsg string    `json:"error_msg"`
	Data     *NodeInfo `json:"data"`
}

// AddNodesResponse represents the response for batch-adding nodes to a cluster.
// AddNodesResponse 表示批量向集群添加节点的响应。
type AddNodesResponse struct {
	ErrorMsg string      `json:"error_msg"`
	Data     []*NodeInfo `json:"data"`
}

// RemoveNodeResponse represents the response for removing a node from a cluster.
// RemoveNodeResponse 表示从集群移除节点的响应。
type RemoveNodeResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     any    `json:"data"`
}

// GetNodesResponse represents the response for getting cluster nodes.
// GetNodesResponse 表示获取集群节点列表的响应。
type GetNodesResponse struct {
	ErrorMsg string      `json:"error_msg"`
	Data     []*NodeInfo `json:"data"`
}

// ClusterOperationResponse represents the response for cluster operations (start/stop/restart).
// ClusterOperationResponse 表示集群操作（启动/停止/重启）的响应。
type ClusterOperationResponse struct {
	ErrorMsg string           `json:"error_msg"`
	Data     *OperationResult `json:"data"`
}

// GetClusterStatusResponse represents the response for getting cluster status.
// GetClusterStatusResponse 表示获取集群状态的响应。
type GetClusterStatusResponse struct {
	ErrorMsg string             `json:"error_msg"`
	Data     *ClusterStatusInfo `json:"data"`
}

// GetRuntimeStorageResponse represents runtime storage details response.
// GetRuntimeStorageResponse 表示运行时存储详情响应。
type GetRuntimeStorageResponse struct {
	ErrorMsg string                 `json:"error_msg"`
	Data     *RuntimeStorageDetails `json:"data"`
}

// CleanupRuntimeStorageResponse represents runtime storage cleanup response.
// CleanupRuntimeStorageResponse 表示运行时存储清理响应。
type CleanupRuntimeStorageResponse struct {
	ErrorMsg string                       `json:"error_msg"`
	Data     *RuntimeStorageCleanupResult `json:"data"`
}

// ValidateRuntimeStorageResponse represents runtime storage connectivity validation response.
// ValidateRuntimeStorageResponse 表示运行时存储连通性校验响应。
type ValidateRuntimeStorageResponse struct {
	ErrorMsg string                                       `json:"error_msg"`
	Data     *installerapp.RuntimeStorageValidationResult `json:"data"`
}

type ListRuntimeStorageResponse struct {
	ErrorMsg string                    `json:"error_msg"`
	Data     *RuntimeStorageListResult `json:"data"`
}

type PreviewRuntimeStorageResponse struct {
	ErrorMsg string                       `json:"error_msg"`
	Data     *RuntimeStoragePreviewResult `json:"data"`
}

type InspectCheckpointRuntimeStorageResponse struct {
	ErrorMsg string                                 `json:"error_msg"`
	Data     *RuntimeStorageCheckpointInspectResult `json:"data"`
}

type InspectIMAPRuntimeStorageResponse struct {
	ErrorMsg string                           `json:"error_msg"`
	Data     *RuntimeStorageIMAPInspectResult `json:"data"`
}

// STXJavaProxyResponse represents a managed stx-java-proxy response.
type STXJavaProxyResponse struct {
	ErrorMsg string              `json:"error_msg"`
	Data     *STXJavaProxyStatus `json:"data"`
}

type STXJavaProxyLogPreviewResponse struct {
	ErrorMsg string                        `json:"error_msg"`
	Data     *STXJavaProxyLogPreviewResult `json:"data"`
}

// PrecheckNodeResponse represents the response for node precheck.
// PrecheckNodeResponse 表示节点预检查的响应。
type PrecheckNodeResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     *PrecheckResult `json:"data"`
}

// ==================== Cluster CRUD Handlers 集群 CRUD 处理器 ====================

// CreateCluster handles POST /api/v1/clusters - creates a new cluster.
// CreateCluster 处理 POST /api/v1/clusters - 创建新集群。
// @Tags clusters
// @Accept json
// @Produce json
// @Param request body CreateClusterRequest true "创建集群请求"
// @Success 200 {object} CreateClusterResponse
// @Router /api/v1/clusters [post]
func (h *Handler) CreateCluster(c *gin.Context) {
	var req CreateClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CreateClusterResponse{ErrorMsg: err.Error()})
		return
	}

	cluster, err := h.service.Create(c.Request.Context(), &req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, CreateClusterResponse{ErrorMsg: err.Error()})
		return
	}

	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"create", "cluster", audit.UintID(cluster.ID), cluster.Name, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 创建集群成功: %s (mode: %s)", cluster.Name, cluster.DeploymentMode)
	c.JSON(http.StatusOK, CreateClusterResponse{Data: cluster.ToClusterInfo()})
}

// GetSTXJavaProxyStatus handles GET /api/v1/clusters/:id/stx-java-proxy/status.
func (h *Handler) GetSTXJavaProxyStatus(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, STXJavaProxyResponse{ErrorMsg: "invalid cluster id"})
		return
	}
	status, err := h.service.GetSTXJavaProxyStatus(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), STXJavaProxyResponse{ErrorMsg: err.Error(), Data: status})
		return
	}
	c.JSON(http.StatusOK, STXJavaProxyResponse{Data: status})
}

// StartSTXJavaProxy handles POST /api/v1/clusters/:id/stx-java-proxy/start.
func (h *Handler) StartSTXJavaProxy(c *gin.Context) {
	h.handleSTXJavaProxyOperation(c, func(ctx context.Context, clusterID uint) (*STXJavaProxyStatus, error) {
		return h.service.StartSTXJavaProxy(ctx, clusterID)
	})
}

// StopSTXJavaProxy handles POST /api/v1/clusters/:id/stx-java-proxy/stop.
func (h *Handler) StopSTXJavaProxy(c *gin.Context) {
	h.handleSTXJavaProxyOperation(c, func(ctx context.Context, clusterID uint) (*STXJavaProxyStatus, error) {
		return h.service.StopSTXJavaProxy(ctx, clusterID)
	})
}

// RestartSTXJavaProxy handles POST /api/v1/clusters/:id/stx-java-proxy/restart.
func (h *Handler) RestartSTXJavaProxy(c *gin.Context) {
	h.handleSTXJavaProxyOperation(c, func(ctx context.Context, clusterID uint) (*STXJavaProxyStatus, error) {
		return h.service.RestartSTXJavaProxy(ctx, clusterID)
	})
}

// PreviewSTXJavaProxyServiceLog handles GET /api/v1/clusters/:id/stx-java-proxy/logs.
func (h *Handler) PreviewSTXJavaProxyServiceLog(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, STXJavaProxyLogPreviewResponse{ErrorMsg: "invalid cluster id"})
		return
	}
	lines := 200
	if linesValue := strings.TrimSpace(c.Query("lines")); linesValue != "" {
		parsed, parseErr := strconv.Atoi(linesValue)
		if parseErr != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, STXJavaProxyLogPreviewResponse{ErrorMsg: "invalid lines"})
			return
		}
		lines = parsed
	}
	result, err := h.service.GetSTXJavaProxyServiceLog(c.Request.Context(), uint(clusterID), lines)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), STXJavaProxyLogPreviewResponse{ErrorMsg: err.Error(), Data: result})
		return
	}
	c.JSON(http.StatusOK, STXJavaProxyLogPreviewResponse{Data: result})
}

func (h *Handler) handleSTXJavaProxyOperation(c *gin.Context, fn func(context.Context, uint) (*STXJavaProxyStatus, error)) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, STXJavaProxyResponse{ErrorMsg: "invalid cluster id"})
		return
	}
	status, err := fn(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), STXJavaProxyResponse{ErrorMsg: err.Error(), Data: status})
		return
	}
	c.JSON(http.StatusOK, STXJavaProxyResponse{Data: status})
}

// ListClusters handles GET /api/v1/clusters - lists clusters with filtering and pagination.
// ListClusters 处理 GET /api/v1/clusters - 获取集群列表（支持过滤和分页）。
// @Tags clusters
// @Param request query ListClustersRequest true "查询参数"
// @Produce json
// @Success 200 {object} ListClustersResponse
// @Router /api/v1/clusters [get]
func (h *Handler) ListClusters(c *gin.Context) {
	req := &ListClustersRequest{Current: 1, Size: 20}
	if err := c.ShouldBindQuery(req); err != nil {
		c.JSON(http.StatusBadRequest, ListClustersResponse{ErrorMsg: err.Error()})
		return
	}

	// Build filter from request
	// 从请求构建过滤条件
	filter := &ClusterFilter{
		Name:           req.Name,
		Status:         req.Status,
		DeploymentMode: req.DeploymentMode,
		Page:           req.Current,
		PageSize:       req.Size,
	}

	clusters, total, err := h.service.ListWithInfo(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ListClustersResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, ListClustersResponse{
		Data: &struct {
			Total    int64          `json:"total"`
			Clusters []*ClusterInfo `json:"clusters"`
		}{
			Total:    total,
			Clusters: clusters,
		},
	})
}

// GetCluster handles GET /api/v1/clusters/:id - gets a cluster by ID.
// GetCluster 处理 GET /api/v1/clusters/:id - 根据 ID 获取集群详情。
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} GetClusterResponse
// @Router /api/v1/clusters/{id} [get]
func (h *Handler) GetCluster(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetClusterResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	cluster, err := h.service.Get(c.Request.Context(), uint(clusterID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, GetClusterResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetClusterResponse{Data: cluster.ToClusterInfo()})
}

// UpdateCluster handles PUT /api/v1/clusters/:id - updates an existing cluster.
// UpdateCluster 处理 PUT /api/v1/clusters/:id - 更新现有集群。
// @Tags clusters
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param request body UpdateClusterRequest true "更新集群请求"
// @Success 200 {object} UpdateClusterResponse
// @Router /api/v1/clusters/{id} [put]
func (h *Handler) UpdateCluster(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, UpdateClusterResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	var req UpdateClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, UpdateClusterResponse{ErrorMsg: err.Error()})
		return
	}

	cluster, err := h.service.Update(c.Request.Context(), uint(clusterID), &req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, UpdateClusterResponse{ErrorMsg: err.Error()})
		return
	}

	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"update", "cluster", audit.UintID(cluster.ID), cluster.Name, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 更新集群成功: %s", cluster.Name)
	c.JSON(http.StatusOK, UpdateClusterResponse{Data: cluster.ToClusterInfo()})
}

// DeleteCluster handles DELETE /api/v1/clusters/:id - deletes a cluster.
// DeleteCluster 处理 DELETE /api/v1/clusters/:id - 删除集群。
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} DeleteClusterResponse
// @Router /api/v1/clusters/{id} [delete]
func (h *Handler) DeleteCluster(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, DeleteClusterResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	// Get cluster name for logging before deletion
	// 在删除前获取集群名用于日志记录
	cluster, err := h.service.Get(c.Request.Context(), uint(clusterID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, DeleteClusterResponse{ErrorMsg: err.Error()})
		return
	}

	forceDelete := c.Query("force_delete") == "1" || c.Query("force_delete") == "true"
	if err := h.service.Delete(c.Request.Context(), uint(clusterID), forceDelete); err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, DeleteClusterResponse{ErrorMsg: err.Error()})
		return
	}

	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"delete", "cluster", audit.UintID(uint(clusterID)), cluster.Name, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 删除集群成功: %s", cluster.Name)
	c.JSON(http.StatusOK, DeleteClusterResponse{})
}

// ==================== Node Management Handlers 节点管理处理器 ====================

// AddNode handles POST /api/v1/clusters/:id/nodes - adds a node to a cluster.
// AddNode 处理 POST /api/v1/clusters/:id/nodes - 向集群添加节点。
// @Tags clusters
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param request body AddNodeRequest true "添加节点请求"
// @Success 200 {object} AddNodeResponse
// @Router /api/v1/clusters/{id}/nodes [post]
func (h *Handler) AddNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, AddNodeResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	var req AddNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AddNodeResponse{ErrorMsg: err.Error()})
		return
	}

	node, err := h.service.AddNode(c.Request.Context(), uint(clusterID), &req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, AddNodeResponse{ErrorMsg: err.Error()})
		return
	}

	resourceName := h.getClusterNodeResourceName(c, uint(clusterID), node.ID)
	resID := audit.UintID(uint(clusterID)) + "/" + audit.UintID(node.ID)
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"add_node", "cluster_node", resID, resourceName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 添加节点成功: cluster_id=%d, host_id=%d, role=%s, hazelcast_port=%d", clusterID, req.HostID, node.Role, node.HazelcastPort)
	c.JSON(http.StatusOK, AddNodeResponse{Data: buildNodeInfo(node)})
}

// AddNodes handles POST /api/v1/clusters/:id/nodes/batch - adds multiple logical nodes for one host atomically.
// AddNodes 处理 POST /api/v1/clusters/:id/nodes/batch - 为同一主机原子添加多个逻辑节点。
func (h *Handler) AddNodes(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, AddNodesResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	var req AddNodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AddNodesResponse{ErrorMsg: err.Error()})
		return
	}

	nodes, err := h.service.AddNodes(c.Request.Context(), uint(clusterID), &req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, AddNodesResponse{ErrorMsg: err.Error()})
		return
	}

	nodeInfos := make([]*NodeInfo, 0, len(nodes))
	roleLabels := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		nodeInfos = append(nodeInfos, buildNodeInfo(node))
		roleLabels = append(roleLabels, string(node.Role))
	}

	clusterName := h.getClusterNameForAudit(c.Request.Context(), uint(clusterID))
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"add_nodes", "cluster", audit.UintID(uint(clusterID)), clusterName, audit.AuditDetails{
			"trigger": "manual",
			"count":   len(nodeInfos),
			"roles":   strings.Join(roleLabels, ","),
		})
	logger.InfoF(c.Request.Context(), "[Cluster] 批量添加节点成功: cluster_id=%d, host_id=%d, roles=%s", clusterID, req.HostID, strings.Join(roleLabels, ","))
	c.JSON(http.StatusOK, AddNodesResponse{Data: nodeInfos})
}

// RemoveNode handles DELETE /api/v1/clusters/:id/nodes/:nodeId - removes a node from a cluster.
// RemoveNode 处理 DELETE /api/v1/clusters/:id/nodes/:nodeId - 从集群移除节点。
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Param nodeId path int true "节点ID"
// @Success 200 {object} RemoveNodeResponse
// @Router /api/v1/clusters/{id}/nodes/{nodeId} [delete]
func (h *Handler) RemoveNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, RemoveNodeResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	nodeID, err := strconv.ParseUint(c.Param("nodeId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, RemoveNodeResponse{ErrorMsg: "无效的节点 ID / Invalid node ID"})
		return
	}

	resourceName := h.getClusterNodeResourceName(c, uint(clusterID), uint(nodeID))

	if err := h.service.RemoveNode(c.Request.Context(), uint(clusterID), uint(nodeID)); err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, RemoveNodeResponse{ErrorMsg: err.Error()})
		return
	}

	resID := audit.UintID(uint(clusterID)) + "/" + audit.UintID(uint(nodeID))
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"remove_node", "cluster_node", resID, resourceName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 移除节点成功: cluster_id=%d, node_id=%d", clusterID, nodeID)
	c.JSON(http.StatusOK, RemoveNodeResponse{})
}

// UpdateNode handles PUT /api/v1/clusters/:id/nodes/:nodeId - updates a node in a cluster.
// UpdateNode 处理 PUT /api/v1/clusters/:id/nodes/:nodeId - 更新集群中的节点。
// @Tags clusters
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param nodeId path int true "节点ID"
// @Param request body UpdateNodeRequest true "更新节点请求"
// @Success 200 {object} AddNodeResponse
// @Router /api/v1/clusters/{id}/nodes/{nodeId} [put]
func (h *Handler) UpdateNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, AddNodeResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	nodeID, err := strconv.ParseUint(c.Param("nodeId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, AddNodeResponse{ErrorMsg: "无效的节点 ID / Invalid node ID"})
		return
	}

	var req UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AddNodeResponse{ErrorMsg: err.Error()})
		return
	}

	node, err := h.service.UpdateNode(c.Request.Context(), uint(clusterID), uint(nodeID), &req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, AddNodeResponse{ErrorMsg: err.Error()})
		return
	}

	resourceName := h.getClusterNodeResourceName(c, uint(clusterID), node.ID)
	resID := audit.UintID(uint(clusterID)) + "/" + audit.UintID(node.ID)
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"update_node", "cluster_node", resID, resourceName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 更新节点成功: cluster_id=%d, node_id=%d", clusterID, nodeID)
	c.JSON(http.StatusOK, AddNodeResponse{Data: buildNodeInfo(node)})
}

// GetNodes handles GET /api/v1/clusters/:id/nodes - gets all nodes for a cluster.
// GetNodes 处理 GET /api/v1/clusters/:id/nodes - 获取集群的所有节点。
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} GetNodesResponse
// @Router /api/v1/clusters/{id}/nodes [get]
func (h *Handler) GetNodes(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetNodesResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	nodes, err := h.service.GetNodes(c.Request.Context(), uint(clusterID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, GetNodesResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetNodesResponse{Data: nodes})
}

// ==================== Cluster Operation Handlers 集群操作处理器 ====================

// StartCluster handles POST /api/v1/clusters/:id/start - starts a cluster.
// StartCluster 处理 POST /api/v1/clusters/:id/start - 启动集群。
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} ClusterOperationResponse
// @Router /api/v1/clusters/{id}/start [post]
func (h *Handler) StartCluster(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	result, err := h.service.Start(c.Request.Context(), uint(clusterID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, ClusterOperationResponse{ErrorMsg: err.Error()})
		return
	}
	clusterName := h.getClusterNameForAudit(c.Request.Context(), uint(clusterID))
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"start", "cluster", audit.UintID(uint(clusterID)), clusterName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 启动集群: cluster_id=%d, success=%v", clusterID, result.Success)
	c.JSON(http.StatusOK, ClusterOperationResponse{Data: result})
}

// StopCluster handles POST /api/v1/clusters/:id/stop - stops a cluster.
// StopCluster 处理 POST /api/v1/clusters/:id/stop - 停止集群。
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} ClusterOperationResponse
// @Router /api/v1/clusters/{id}/stop [post]
func (h *Handler) StopCluster(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	result, err := h.service.Stop(c.Request.Context(), uint(clusterID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, ClusterOperationResponse{ErrorMsg: err.Error()})
		return
	}
	clusterName := h.getClusterNameForAudit(c.Request.Context(), uint(clusterID))
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"stop", "cluster", audit.UintID(uint(clusterID)), clusterName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 停止集群: cluster_id=%d, success=%v", clusterID, result.Success)
	c.JSON(http.StatusOK, ClusterOperationResponse{Data: result})
}

// RestartCluster handles POST /api/v1/clusters/:id/restart - restarts a cluster.
// RestartCluster 处理 POST /api/v1/clusters/:id/restart - 重启集群。
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} ClusterOperationResponse
// @Router /api/v1/clusters/{id}/restart [post]
func (h *Handler) RestartCluster(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	result, err := h.service.Restart(c.Request.Context(), uint(clusterID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, ClusterOperationResponse{ErrorMsg: err.Error()})
		return
	}
	clusterName := h.getClusterNameForAudit(c.Request.Context(), uint(clusterID))
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"restart", "cluster", audit.UintID(uint(clusterID)), clusterName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 重启集群: cluster_id=%d, success=%v", clusterID, result.Success)
	h.notifyOperationExecuted(c.Request.Context(), &OperationEvent{
		ClusterID:   uint(clusterID),
		ClusterName: clusterName,
		Operation:   OperationRestart,
		Success:     result.Success,
		Message:     result.Message,
		Operator:    auth.GetUsernameFromContext(c),
		Trigger:     "manual_api",
	})
	c.JSON(http.StatusOK, ClusterOperationResponse{Data: result})
}

// GetClusterStatus handles GET /api/v1/clusters/:id/status - gets the status of a cluster.
// GetClusterStatus 处理 GET /api/v1/clusters/:id/status - 获取集群状态。
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} GetClusterStatusResponse
// @Router /api/v1/clusters/{id}/status [get]
func (h *Handler) GetClusterStatus(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetClusterStatusResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	status, err := h.service.GetStatus(c.Request.Context(), uint(clusterID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, GetClusterStatusResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetClusterStatusResponse{Data: status})
}

// GetRuntimeStorage handles GET /api/v1/clusters/:id/runtime-storage.
// GetRuntimeStorage 处理 GET /api/v1/clusters/:id/runtime-storage。
func (h *Handler) GetRuntimeStorage(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetRuntimeStorageResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	result, err := h.service.GetRuntimeStorageDetails(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, GetRuntimeStorageResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, GetRuntimeStorageResponse{Data: result})
}

// ValidateRuntimeStorage handles POST /api/v1/clusters/:id/runtime-storage/:kind/validate.
// ValidateRuntimeStorage 处理 POST /api/v1/clusters/:id/runtime-storage/:kind/validate。
func (h *Handler) ValidateRuntimeStorage(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ValidateRuntimeStorageResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	kind := installerapp.RuntimeStorageValidationKind(strings.ToLower(strings.TrimSpace(c.Param("kind"))))
	result, err := h.service.ValidateRuntimeStorage(c.Request.Context(), uint(clusterID), kind)
	if err != nil {
		c.JSON(http.StatusBadRequest, ValidateRuntimeStorageResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ValidateRuntimeStorageResponse{Data: result})
}

// ListRuntimeStorage handles POST /api/v1/clusters/:id/runtime-storage/:kind/list.
func (h *Handler) ListRuntimeStorage(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ListRuntimeStorageResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	kind := installerapp.RuntimeStorageValidationKind(strings.ToLower(strings.TrimSpace(c.Param("kind"))))
	var req struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
		Limit     int    `json:"limit"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	if req.Limit <= 0 {
		req.Limit = 200
	}
	result, err := h.service.ListRuntimeStorage(c.Request.Context(), uint(clusterID), kind, req.Path, req.Recursive, req.Limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, ListRuntimeStorageResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ListRuntimeStorageResponse{Data: result})
}

// PreviewRuntimeStorage handles POST /api/v1/clusters/:id/runtime-storage/:kind/preview.
func (h *Handler) PreviewRuntimeStorage(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, PreviewRuntimeStorageResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	kind := installerapp.RuntimeStorageValidationKind(strings.ToLower(strings.TrimSpace(c.Param("kind"))))
	var req struct {
		Path     string `json:"path"`
		MaxBytes int    `json:"max_bytes"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	result, err := h.service.PreviewRuntimeStorage(c.Request.Context(), uint(clusterID), kind, req.Path, req.MaxBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, PreviewRuntimeStorageResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, PreviewRuntimeStorageResponse{Data: result})
}

// InspectCheckpointRuntimeStorage handles POST /api/v1/clusters/:id/runtime-storage/checkpoint/inspect.
func (h *Handler) InspectCheckpointRuntimeStorage(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, InspectCheckpointRuntimeStorageResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	var req struct {
		Path      string                                    `json:"path"`
		JobConfig *RuntimeStorageCheckpointInspectJobConfig `json:"job_config"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	result, err := h.service.InspectCheckpointRuntimeStorage(
		c.Request.Context(),
		uint(clusterID),
		req.Path,
		req.JobConfig,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, InspectCheckpointRuntimeStorageResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, InspectCheckpointRuntimeStorageResponse{Data: result})
}

// InspectIMAPRuntimeStorage handles POST /api/v1/clusters/:id/runtime-storage/imap/inspect.
func (h *Handler) InspectIMAPRuntimeStorage(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, InspectIMAPRuntimeStorageResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	result, err := h.service.InspectIMAPRuntimeStorage(c.Request.Context(), uint(clusterID), req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, InspectIMAPRuntimeStorageResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, InspectIMAPRuntimeStorageResponse{Data: result})
}

// ProxyWebUI handles proxying the SeaTunnel Web UI through the control plane.
// ProxyWebUI 处理通过控制平面反向代理 SeaTunnel Web UI。
func (h *Handler) ProxyWebUI(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "无效的集群 ID / Invalid cluster ID"})
		return
	}

	nodes, err := h.service.GetNodes(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}

	var targetNode *NodeInfo
	for _, node := range nodes {
		if node == nil || node.APIPort <= 0 {
			continue
		}
		if node.Role != NodeRoleMaster && node.Role != NodeRoleMasterWorker {
			continue
		}
		if targetNode == nil || node.IsOnline {
			targetNode = node
		}
		if node.IsOnline {
			break
		}
	}

	if targetNode == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "当前集群未找到可用的 SeaTunnel Web UI 节点 / No available SeaTunnel Web UI node found"})
		return
	}

	upstreamHost := strings.TrimSpace(targetNode.HostIP)
	if upstreamHost == "" {
		upstreamHost = strings.TrimSpace(targetNode.HostName)
	}
	if upstreamHost == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "目标节点缺少主机地址，无法代理 Web UI / Target node has no host address for Web UI proxy"})
		return
	}

	target, err := url.Parse("http://" + upstreamHost + ":" + strconv.Itoa(targetNode.APIPort))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}

	prefix := "/api/v1/clusters/" + strconv.FormatUint(clusterID, 10) + "/webui"
	proxyPath := c.Param("proxyPath")
	if proxyPath == "" {
		proxyPath = "/"
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		if proxyPath == "/" {
			req.URL.Path = "/"
		} else {
			req.URL.Path = proxyPath
		}
		req.URL.RawPath = req.URL.EscapedPath()
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Prefix", prefix)
		req.Header.Set("X-Forwarded-Host", c.Request.Host)
		if req.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		contentType := strings.ToLower(resp.Header.Get("Content-Type"))
		shouldRewrite := strings.Contains(contentType, "text/html") ||
			strings.Contains(contentType, "javascript")
		if !shouldRewrite {
			return nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()

		content := string(body)
		if strings.Contains(contentType, "text/html") && !strings.Contains(strings.ToLower(content), "<base ") {
			content = strings.Replace(content, "<head>", "<head><base href=\""+prefix+"/\">", 1)
		}
		content = strings.ReplaceAll(content, "href=\"/", "href=\""+prefix+"/")
		content = strings.ReplaceAll(content, "src=\"/", "src=\""+prefix+"/")
		content = strings.ReplaceAll(content, "action=\"/", "action=\""+prefix+"/")
		content = strings.ReplaceAll(content, "fetch('/api/", "fetch('"+prefix+"/api/")
		content = strings.ReplaceAll(content, "fetch(\"/api/", "fetch(\""+prefix+"/api/")
		content = strings.ReplaceAll(content, "baseURL:\"/api\"", "baseURL:\""+prefix+"/api\"")
		content = strings.ReplaceAll(content, "baseURL: '/api'", "baseURL: '"+prefix+"/api'")
		content = strings.ReplaceAll(content, "baseURL:'/api'", "baseURL:'"+prefix+"/api'")
		content = strings.ReplaceAll(content, "axios.defaults.baseURL=\"/api\"", "axios.defaults.baseURL=\""+prefix+"/api\"")

		resp.Body = io.NopCloser(strings.NewReader(content))
		resp.ContentLength = int64(len(content))
		resp.Header.Set("Content-Length", strconv.Itoa(len(content)))
		return nil
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		logger.WarnF(c.Request.Context(), "[Cluster] Web UI proxy failed: cluster=%d, target=%s, err=%v", clusterID, target.String(), proxyErr)
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte(`{"error_msg":"SeaTunnel Web UI 代理失败 / Failed to proxy SeaTunnel Web UI"}`))
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

// CleanupIMAPStorage handles POST /api/v1/clusters/:id/runtime-storage/imap/cleanup.
// CleanupIMAPStorage 处理 POST /api/v1/clusters/:id/runtime-storage/imap/cleanup。
func (h *Handler) CleanupIMAPStorage(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, CleanupRuntimeStorageResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	result, err := h.service.CleanupIMAPStorage(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(http.StatusBadRequest, CleanupRuntimeStorageResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, CleanupRuntimeStorageResponse{Data: result})
}

// ==================== Node Precheck Handlers 节点预检查处理器 ====================

// PrecheckNode handles POST /api/v1/clusters/:id/nodes/precheck - prechecks a node before adding.
// PrecheckNode 处理 POST /api/v1/clusters/:id/nodes/precheck - 添加节点前的预检查。
// @Tags clusters
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param request body PrecheckRequest true "预检查请求"
// @Success 200 {object} PrecheckNodeResponse
// @Router /api/v1/clusters/{id}/nodes/precheck [post]
func (h *Handler) PrecheckNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, PrecheckNodeResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	var req PrecheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, PrecheckNodeResponse{ErrorMsg: err.Error()})
		return
	}

	result, err := h.service.PrecheckNode(c.Request.Context(), uint(clusterID), &req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, PrecheckNodeResponse{ErrorMsg: err.Error()})
		return
	}

	logger.InfoF(c.Request.Context(), "[Cluster] 节点预检查完成: cluster_id=%d, host_id=%d, success=%v", clusterID, req.HostID, result.Success)
	c.JSON(http.StatusOK, PrecheckNodeResponse{Data: result})
}

// ==================== Helper Methods 辅助方法 ====================

// getClusterNodeResourceName returns display name for cluster_node audit: "集群名（主机名 - 角色）" or "集群名".
// getClusterNodeResourceName 返回集群节点审计展示名：集群名（主机名 - 角色）或仅集群名。
func (h *Handler) getClusterNodeResourceName(c *gin.Context, clusterID, nodeID uint) string {
	clusterName, nodeDisplay := h.service.GetClusterNodeDisplayInfo(c.Request.Context(), clusterID, nodeID)
	if clusterName == "" {
		return ""
	}
	if nodeDisplay != "" {
		return clusterName + "（" + nodeDisplay + "）"
	}
	return clusterName
}

// getClusterNameForAudit returns cluster name by ID for audit log resource_name; empty string on error.
// getClusterNameForAudit 按 ID 取集群名称用于审计资源名称，失败时返回空字符串。
func (h *Handler) getClusterNameForAudit(ctx context.Context, clusterID uint) string {
	cluster, err := h.service.Get(ctx, clusterID)
	if err != nil || cluster == nil {
		return ""
	}
	return cluster.Name
}

// getStatusCodeForError returns the appropriate HTTP status code for an error.
// getStatusCodeForError 根据错误返回适当的 HTTP 状态码。
func (h *Handler) getStatusCodeForError(err error) int {
	switch {
	case errors.Is(err, ErrClusterNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrClusterNameDuplicate):
		return http.StatusConflict
	case errors.Is(err, ErrClusterNameEmpty),
		errors.Is(err, ErrInvalidDeploymentMode),
		errors.Is(err, ErrInvalidNodeRole):
		return http.StatusBadRequest
	case errors.Is(err, ErrClusterHasRunningTask):
		return http.StatusConflict
	case errors.Is(err, ErrNodeNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrNodeAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, ErrNodeAgentNotInstalled),
		errors.Is(err, ErrInvalidHazelcastPort),
		errors.Is(err, ErrInvalidAPIPort),
		errors.Is(err, ErrInvalidWorkerPort),
		errors.Is(err, ErrNodeBatchEntriesRequired),
		errors.Is(err, ErrInvalidNodeJVMOverride),
		errors.Is(err, ErrPrecheckFailed):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// ==================== Node Operation Handlers 节点操作处理器 ====================

// StartNode handles POST /api/v1/clusters/:id/nodes/:nodeId/start - starts a node.
// StartNode 处理 POST /api/v1/clusters/:id/nodes/:nodeId/start - 启动节点。
func (h *Handler) StartNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	nodeID, err := strconv.ParseUint(c.Param("nodeId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的节点 ID / Invalid node ID"})
		return
	}

	result, err := h.service.StartNode(c.Request.Context(), uint(clusterID), uint(nodeID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, ClusterOperationResponse{ErrorMsg: err.Error()})
		return
	}

	clusterName, nodeDisplay := h.service.GetClusterNodeDisplayInfo(c.Request.Context(), uint(clusterID), uint(nodeID))
	resourceName := clusterName
	if nodeDisplay != "" {
		resourceName = clusterName + "（" + nodeDisplay + "）"
	}
	resID := audit.UintID(uint(clusterID)) + "/" + audit.UintID(uint(nodeID))
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"start_node", "cluster_node", resID, resourceName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 启动节点: cluster_id=%d, node_id=%d, success=%v", clusterID, nodeID, result.Success)
	c.JSON(http.StatusOK, ClusterOperationResponse{Data: result})
}

// StopNode handles POST /api/v1/clusters/:id/nodes/:nodeId/stop - stops a node.
// StopNode 处理 POST /api/v1/clusters/:id/nodes/:nodeId/stop - 停止节点。
func (h *Handler) StopNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	nodeID, err := strconv.ParseUint(c.Param("nodeId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的节点 ID / Invalid node ID"})
		return
	}

	result, err := h.service.StopNode(c.Request.Context(), uint(clusterID), uint(nodeID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, ClusterOperationResponse{ErrorMsg: err.Error()})
		return
	}

	clusterName, nodeDisplay := h.service.GetClusterNodeDisplayInfo(c.Request.Context(), uint(clusterID), uint(nodeID))
	resourceName := clusterName
	if nodeDisplay != "" {
		resourceName = clusterName + "（" + nodeDisplay + "）"
	}
	resID := audit.UintID(uint(clusterID)) + "/" + audit.UintID(uint(nodeID))
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"stop_node", "cluster_node", resID, resourceName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 停止节点: cluster_id=%d, node_id=%d, success=%v", clusterID, nodeID, result.Success)
	h.notifyOperationExecuted(c.Request.Context(), h.buildNodeOperationEvent(
		c.Request.Context(),
		uint(clusterID),
		uint(nodeID),
		OperationStop,
		result,
		auth.GetUsernameFromContext(c),
	))
	c.JSON(http.StatusOK, ClusterOperationResponse{Data: result})
}

// RestartNode handles POST /api/v1/clusters/:id/nodes/:nodeId/restart - restarts a node.
// RestartNode 处理 POST /api/v1/clusters/:id/nodes/:nodeId/restart - 重启节点。
func (h *Handler) RestartNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	nodeID, err := strconv.ParseUint(c.Param("nodeId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClusterOperationResponse{ErrorMsg: "无效的节点 ID / Invalid node ID"})
		return
	}

	result, err := h.service.RestartNode(c.Request.Context(), uint(clusterID), uint(nodeID))
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, ClusterOperationResponse{ErrorMsg: err.Error()})
		return
	}

	clusterName, nodeDisplay := h.service.GetClusterNodeDisplayInfo(c.Request.Context(), uint(clusterID), uint(nodeID))
	resourceName := clusterName
	if nodeDisplay != "" {
		resourceName = clusterName + "（" + nodeDisplay + "）"
	}
	resID := audit.UintID(uint(clusterID)) + "/" + audit.UintID(uint(nodeID))
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"restart_node", "cluster_node", resID, resourceName, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Cluster] 重启节点: cluster_id=%d, node_id=%d, success=%v", clusterID, nodeID, result.Success)
	h.notifyOperationExecuted(c.Request.Context(), h.buildNodeOperationEvent(
		c.Request.Context(),
		uint(clusterID),
		uint(nodeID),
		OperationRestart,
		result,
		auth.GetUsernameFromContext(c),
	))
	c.JSON(http.StatusOK, ClusterOperationResponse{Data: result})
}

// GetNodeLogsResponse represents the response for getting node logs.
// GetNodeLogsResponse 表示获取节点日志的响应。
type GetNodeLogsResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *struct {
		Logs string `json:"logs"`
	} `json:"data"`
}

// GetNodeLogs handles GET /api/v1/clusters/:id/nodes/:nodeId/logs - gets node logs.
// GetNodeLogs 处理 GET /api/v1/clusters/:id/nodes/:nodeId/logs - 获取节点日志。
// Query parameters:
// - lines: number of lines (default: 100) / 行数（默认：100）
// - mode: "tail" (default), "head", "all" / 模式
// - filter: grep pattern / 过滤模式
// - date: date for rolling logs (e.g., "2025-11-12-1") / 滚动日志日期
func (h *Handler) GetNodeLogs(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetNodeLogsResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}

	nodeID, err := strconv.ParseUint(c.Param("nodeId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetNodeLogsResponse{ErrorMsg: "无效的节点 ID / Invalid node ID"})
		return
	}

	// Parse query parameters / 解析查询参数
	req := &GetNodeLogsRequest{}
	if linesStr := c.Query("lines"); linesStr != "" {
		if lines, err := strconv.Atoi(linesStr); err == nil {
			req.Lines = lines
		}
	}
	req.Mode = c.Query("mode")
	req.Filter = c.Query("filter")
	req.Date = c.Query("date")

	logs, err := h.service.GetNodeLogs(c.Request.Context(), uint(clusterID), uint(nodeID), req)
	if err != nil {
		statusCode := h.getStatusCodeForError(err)
		c.JSON(statusCode, GetNodeLogsResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetNodeLogsResponse{Data: &struct {
		Logs string `json:"logs"`
	}{Logs: logs}})
}
