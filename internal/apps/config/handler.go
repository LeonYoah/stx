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
	"errors"
	"net/http"
	"strconv"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/gin-gonic/gin"
)

// Handler 配置管理 HTTP 处理器
type Handler struct {
	service          *Service
	executionService *executionapp.Service
}

// NewHandler 创建处理器实例
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// SetExecutionService 设置 CLI 和直接 API 写请求使用的公共执行服务。
// SetExecutionService sets the shared execution service used by CLI and direct API writes.
func (h *Handler) SetExecutionService(service *executionapp.Service) { h.executionService = service }

// Response 标准响应格式
type Response struct {
	ErrorMsg string      `json:"error_msg"`
	Data     interface{} `json:"data"`
}

// GetClusterConfigs 获取集群所有配置
// @Summary 获取集群配置列表
// @Tags Config
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} Response
// @Router /api/v1/clusters/{id}/configs [get]
func (h *Handler) GetClusterConfigs(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster id", Data: nil})
		return
	}

	configs, err := h.service.GetByCluster(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: configs})
}

// GetConfig 获取配置详情
// @Summary 获取配置详情
// @Tags Config
// @Produce json
// @Param id path int true "配置ID"
// @Success 200 {object} Response
// @Router /api/v1/configs/{id} [get]
func (h *Handler) GetConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid config id", Data: nil})
		return
	}

	config, err := h.service.Get(c.Request.Context(), uint(id))
	if err != nil {
		if err == ErrConfigNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "config not found", Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: config})
}

// NormalizeConfig 智能修复（规范化）配置内容
// @Summary 规范化配置内容
// @Tags Config
// @Accept json
// @Produce json
// @Param body body NormalizeConfigRequest true "规范化请求"
// @Success 200 {object} Response
// @Router /api/v1/configs/normalize [post]
func (h *Handler) NormalizeConfig(c *gin.Context) {
	var req NormalizeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	content, err := h.service.NormalizeContent(c.Request.Context(), &req)
	if err != nil {
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: validationErr.Error(), Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: map[string]string{"content": content}})
}

// UpdateConfig 更新配置
// @Summary 更新配置
// @Tags Config
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Param body body UpdateConfigRequest true "更新内容"
// @Success 200 {object} Response
// @Router /api/v1/configs/{id} [put]
func (h *Handler) UpdateConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid config id", Data: nil})
		return
	}

	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}
	executionItem, existing, err := h.beginConfigWrite(c, "config.update", strconv.FormatUint(id, 10), executionapp.RiskLevelR1, configUpdateImpact, struct {
		ID      uint64              `json:"id"`
		Request UpdateConfigRequest `json:"request"`
	}{id, req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if existing {
		h.writeExistingConfigWrite(c, executionItem)
		return
	}

	userID := getUserID(c)
	config, err := h.service.Update(c.Request.Context(), uint(id), &req, userID)
	resultRef := ""
	if config != nil {
		resultRef = strconv.FormatUint(uint64(config.ID), 10)
	}
	h.finishConfigWrite(c, executionItem, resultRef, err)
	if err != nil {
		if err == ErrConfigNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "config not found", Data: nil})
			return
		}
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: validationErr.Error(), Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: config})
}

// GetConfigVersions 获取配置版本历史
// @Summary 获取配置版本历史
// @Tags Config
// @Produce json
// @Param id path int true "配置ID"
// @Success 200 {object} Response
// @Router /api/v1/configs/{id}/versions [get]
func (h *Handler) GetConfigVersions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid config id", Data: nil})
		return
	}

	versions, err := h.service.GetVersions(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: versions})
}

// RollbackConfig 回滚配置到指定版本
// @Summary 回滚配置
// @Tags Config
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Param body body RollbackConfigRequest true "回滚请求"
// @Success 200 {object} Response
// @Router /api/v1/configs/{id}/rollback [post]
func (h *Handler) RollbackConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid config id", Data: nil})
		return
	}

	var req RollbackConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}
	executionItem, existing, err := h.beginConfigWrite(c, "config.rollback", strconv.FormatUint(id, 10), executionapp.RiskLevelR1, configRollbackImpact, struct {
		ID      uint64                `json:"id"`
		Request RollbackConfigRequest `json:"request"`
	}{id, req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if existing {
		h.writeExistingConfigWrite(c, executionItem)
		return
	}

	userID := getUserID(c)
	config, err := h.service.Rollback(c.Request.Context(), uint(id), &req, userID)
	resultRef := ""
	if config != nil {
		resultRef = strconv.FormatUint(uint64(config.ID), 10)
	}
	h.finishConfigWrite(c, executionItem, resultRef, err)
	if err != nil {
		if err == ErrConfigNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "config not found", Data: nil})
			return
		}
		if err == ErrVersionNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "version not found", Data: nil})
			return
		}
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: validationErr.Error(), Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: config})
}

// PromoteConfig 推广配置到集群
// @Summary 推广配置到集群
// @Tags Config
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Param body body PromoteConfigRequest true "推广请求"
// @Success 200 {object} Response
// @Router /api/v1/configs/{id}/promote [post]
func (h *Handler) PromoteConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid config id", Data: nil})
		return
	}

	var req PromoteConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 允许空 body
		req = PromoteConfigRequest{}
	}
	executionItem, existing, err := h.beginConfigWrite(c, "config.promote", strconv.FormatUint(id, 10), executionapp.RiskLevelR2, configPromoteImpact, struct {
		ID      uint64               `json:"id"`
		Request PromoteConfigRequest `json:"request"`
	}{id, req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if existing {
		h.writeExistingConfigWrite(c, executionItem)
		return
	}

	userID := getUserID(c)
	err = h.service.Promote(c.Request.Context(), uint(id), &req, userID)
	h.finishConfigWrite(c, executionItem, strconv.FormatUint(id, 10), err)
	if err != nil {
		if err == ErrConfigNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "config not found", Data: nil})
			return
		}
		if err == ErrCannotPromoteTemplate {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "cannot promote template config", Data: nil})
			return
		}
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: validationErr.Error(), Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: map[string]string{"message": "config promoted to cluster successfully"}})
}

// SyncFromTemplate 从集群模板同步
// @Summary 从集群模板同步配置
// @Tags Config
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Param body body SyncConfigRequest true "同步请求"
// @Success 200 {object} Response
// @Router /api/v1/configs/{id}/sync [post]
func (h *Handler) SyncFromTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid config id", Data: nil})
		return
	}

	var req SyncConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 允许空 body
		req = SyncConfigRequest{}
	}
	executionItem, existing, err := h.beginConfigWrite(c, "config.sync", strconv.FormatUint(id, 10), executionapp.RiskLevelR1, configSyncImpact, struct {
		ID      uint64            `json:"id"`
		Request SyncConfigRequest `json:"request"`
	}{id, req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if existing {
		h.writeExistingConfigWrite(c, executionItem)
		return
	}

	userID := getUserID(c)
	config, err := h.service.SyncFromTemplate(c.Request.Context(), uint(id), &req, userID)
	resultRef := ""
	if config != nil {
		resultRef = strconv.FormatUint(uint64(config.ID), 10)
	}
	h.finishConfigWrite(c, executionItem, resultRef, err)
	if err != nil {
		if err == ErrConfigNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "config not found", Data: nil})
			return
		}
		if err == ErrTemplateNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "configuration template not found", Data: nil})
			return
		}
		if err == ErrCannotSyncTemplate {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "cannot sync template config", Data: nil})
			return
		}
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: validationErr.Error(), Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: config})
}

// InitClusterConfigsRequest 初始化集群配置请求
type InitClusterConfigsRequest struct {
	HostID     uint   `json:"host_id" binding:"required"`
	InstallDir string `json:"install_dir" binding:"required"`
}

// InitClusterConfigs 初始化集群配置（从节点拉取）
// @Summary 初始化集群配置
// @Tags Config
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param body body InitClusterConfigsRequest true "初始化请求"
// @Success 200 {object} Response
// @Router /api/v1/clusters/{id}/configs/init [post]
func (h *Handler) InitClusterConfigs(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster id", Data: nil})
		return
	}

	var req InitClusterConfigsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}
	executionItem, existing, err := h.beginConfigWrite(c, "config.cluster.init", strconv.FormatUint(clusterID, 10), executionapp.RiskLevelR1, configClusterInitImpact, struct {
		ClusterID uint64                    `json:"cluster_id"`
		Request   InitClusterConfigsRequest `json:"request"`
	}{clusterID, req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if existing {
		h.writeExistingConfigWrite(c, executionItem)
		return
	}

	userID := getUserID(c)
	err = h.service.InitClusterConfigs(c.Request.Context(), uint(clusterID), req.HostID, req.InstallDir, userID)
	h.finishConfigWrite(c, executionItem, strconv.FormatUint(clusterID, 10), err)
	if err != nil {
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: validationErr.Error(), Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: map[string]string{"message": "cluster configs initialized successfully"}})
}

// PushConfigRequest 推送配置请求
type PushConfigRequest struct {
	InstallDir string `json:"install_dir" binding:"required"`
}

// PushConfigToNode 推送配置到节点
// @Summary 推送配置到节点
// @Tags Config
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Param body body PushConfigRequest true "推送请求"
// @Success 200 {object} Response
// @Router /api/v1/configs/{id}/push [post]
func (h *Handler) PushConfigToNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid config id", Data: nil})
		return
	}

	var req PushConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}
	executionItem, existing, err := h.beginConfigWrite(c, "config.push", strconv.FormatUint(id, 10), executionapp.RiskLevelR2, configPushImpact, struct {
		ID      uint64            `json:"id"`
		Request PushConfigRequest `json:"request"`
	}{id, req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if existing {
		h.writeExistingConfigWrite(c, executionItem)
		return
	}

	err = h.service.PushConfigToNode(c.Request.Context(), uint(id), req.InstallDir)
	h.finishConfigWrite(c, executionItem, strconv.FormatUint(id, 10), err)
	if err != nil {
		if err == ErrConfigNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "config not found", Data: nil})
			return
		}
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: validationErr.Error(), Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: map[string]string{"message": "config pushed to node successfully"}})
}

// SyncTemplateToAllNodesRequest 同步模板到所有节点请求
type SyncTemplateToAllNodesRequest struct {
	ConfigType string `json:"config_type" binding:"required"`
}

// SyncTemplateToAllNodes 将集群模板同步到所有节点
// @Summary 同步模板到所有节点
// @Tags Config
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param body body SyncTemplateToAllNodesRequest true "同步请求"
// @Success 200 {object} Response
// @Router /api/v1/clusters/{id}/configs/sync-all [post]
func (h *Handler) SyncTemplateToAllNodes(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster id", Data: nil})
		return
	}

	var req SyncTemplateToAllNodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}
	executionItem, existing, err := h.beginConfigWrite(c, "config.cluster.sync-all", strconv.FormatUint(clusterID, 10), executionapp.RiskLevelR2, configClusterSyncAllImpact, struct {
		ClusterID uint64                        `json:"cluster_id"`
		Request   SyncTemplateToAllNodesRequest `json:"request"`
	}{clusterID, req})
	if err != nil {
		executionapp.WriteError(c, err)
		return
	}
	if existing {
		h.writeExistingConfigWrite(c, executionItem)
		return
	}

	userID := getUserID(c)
	result, err := h.service.SyncTemplateToAllNodes(c.Request.Context(), uint(clusterID), ConfigType(req.ConfigType), userID)
	resultRef := strconv.FormatUint(clusterID, 10) + ":0"
	if result != nil {
		resultRef = strconv.FormatUint(clusterID, 10) + ":" + strconv.Itoa(result.SyncedCount)
	}
	h.finishConfigWrite(c, executionItem, resultRef, err)
	if err != nil {
		if err == ErrTemplateNotFound {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: "configuration template not found", Data: nil})
			return
		}
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: validationErr.Error(), Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{ErrorMsg: "", Data: map[string]interface{}{
		"message":      "template synced to all nodes",
		"synced_count": result.SyncedCount,
		"push_errors":  result.PushErrors,
	}})
}

// getUserID 从统一认证上下文获取用户 ID。
// getUserID reads the user ID from the shared authentication context.
func getUserID(c *gin.Context) uint {
	return uint(auth.GetUserIDFromContext(c))
}
