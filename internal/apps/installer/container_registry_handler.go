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
	"errors"
	"net/http"
	"strconv"

	"github.com/LeonYoah/stx/internal/pkg/locale"
	"github.com/gin-gonic/gin"
)

func registryErrorStatus(err error) int {
	switch {
	case errors.Is(err, errRegistryNotFound):
		return http.StatusNotFound
	case errors.Is(err, errRegistryAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, errRegistryValidation), errors.Is(err, errSystemRegistryProtected):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// ListContainerRegistries GET /api/v1/packages/registries
// ListContainerRegistries 列出登记的镜像仓库。
// @Summary 列出镜像仓库 / List container registries
// @Tags packages
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/registries [get]
func (h *Handler) ListContainerRegistries(c *gin.Context) {
	rows, err := h.service.ListContainerRegistries(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}
	if rows == nil {
		rows = []*ContainerRegistry{}
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"items": rows}})
}

// CreateContainerRegistry POST /api/v1/packages/registries
// CreateContainerRegistry 登记镜像仓库。
// @Summary 登记镜像仓库（管理员） / Register a container registry (admin)
// @Tags packages
// @Produce json
// @Param body body ContainerRegistryRequest true "仓库定义 / Registry definition"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/registries [post]
func (h *Handler) CreateContainerRegistry(c *gin.Context) {
	var req ContainerRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	row, err := h.service.CreateContainerRegistry(c.Request.Context(), &req)
	if err != nil {
		c.JSON(registryErrorStatus(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": row})
}

// UpdateContainerRegistry PUT /api/v1/packages/registries/:id
// UpdateContainerRegistry 更新镜像仓库。
// @Summary 更新镜像仓库（管理员） / Update a container registry (admin)
// @Tags packages
// @Produce json
// @Param id path int true "仓库 ID / Registry ID"
// @Param body body ContainerRegistryRequest true "仓库定义 / Registry definition"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/registries/{id} [put]
func (h *Handler) UpdateContainerRegistry(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": locale.Pick(c.Request.Context(), "无效的仓库 ID", "invalid registry id")})
		return
	}
	var req ContainerRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	row, err := h.service.UpdateContainerRegistry(c.Request.Context(), uint(id), &req)
	if err != nil {
		c.JSON(registryErrorStatus(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": row})
}

// DeleteContainerRegistry DELETE /api/v1/packages/registries/:id
// DeleteContainerRegistry 删除镜像仓库。
// @Summary 删除镜像仓库（管理员） / Delete a container registry (admin)
// @Tags packages
// @Produce json
// @Param id path int true "仓库 ID / Registry ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/registries/{id} [delete]
func (h *Handler) DeleteContainerRegistry(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": locale.Pick(c.Request.Context(), "无效的仓库 ID", "invalid registry id")})
		return
	}
	if err := h.service.DeleteContainerRegistry(c.Request.Context(), uint(id)); err != nil {
		c.JSON(registryErrorStatus(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"success": true}})
}

// TestContainerRegistry POST /api/v1/packages/registries/:id/test
// TestContainerRegistry 测试仓库连通性并刷新 catalog 缓存。
// @Summary 测试镜像仓库连通性 / Test registry connectivity and refresh catalog
// @Tags packages
// @Produce json
// @Param id path int true "仓库 ID / Registry ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/registries/{id}/test [post]
func (h *Handler) TestContainerRegistry(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": locale.Pick(c.Request.Context(), "无效的仓库 ID", "invalid registry id")})
		return
	}
	row, err := h.service.GetContainerRegistry(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(registryErrorStatus(err), gin.H{"error_msg": err.Error()})
		return
	}
	h.service.ProbeContainerRegistry(c.Request.Context(), row)
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": row})
}

// ListContainerRegistryTags GET /api/v1/packages/registries/:id/tags?repo=x
// ListContainerRegistryTags 实时探测仓库中指定镜像的 tag 列表。
// @Summary 探测镜像 tag 列表 / Probe tags of a repository
// @Tags packages
// @Produce json
// @Param id path int true "仓库 ID / Registry ID"
// @Param repo query string true "镜像名 / Repository"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/registries/{id}/tags [get]
func (h *Handler) ListContainerRegistryTags(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": locale.Pick(c.Request.Context(), "无效的仓库 ID", "invalid registry id")})
		return
	}
	repo := c.Query("repo")
	tags, err := h.service.ProbeRegistryTags(c.Request.Context(), uint(id), repo)
	if err != nil {
		status := registryErrorStatus(err)
		if status == http.StatusInternalServerError {
			status = http.StatusBadGateway
		}
		c.JSON(status, gin.H{"error_msg": err.Error()})
		return
	}
	if tags == nil {
		tags = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": gin.H{"items": tags}})
}

// ProbeRegistryTransient POST /api/v1/packages/registries/probe
// ProbeRegistryTransient 注册/编辑前不落库测试连通性（管理员）。
// @Summary 注册或编辑前测试仓库连通性（管理员） / Probe a registry before save (admin)
// @Tags packages
// @Accept json
// @Produce json
// @Param payload body ContainerRegistryProbeRequest true "仓库信息 / Registry payload"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/registries/probe [post]
func (h *Handler) ProbeRegistryTransient(c *gin.Context) {
	var req ContainerRegistryProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	res, err := h.service.ProbeRegistryTransient(c.Request.Context(), &req)
	if err != nil {
		c.JSON(registryErrorStatus(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": res})
}
