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
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListContainerImages GET /api/v1/packages/images
// ListContainerImages 列出已注册容器镜像与官方预设。
// @Summary 列出注册容器镜像 / List registered container images
// @Tags packages
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/images [get]
func (h *Handler) ListContainerImages(c *gin.Context) {
	result, err := h.service.ListContainerImages(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// CreateContainerImage POST /api/v1/packages/images
// CreateContainerImage 注册容器镜像。
// @Summary 注册容器镜像（管理员） / Register a container image (admin)
// @Tags packages
// @Produce json
// @Param body body map[string]interface{} true "镜像定义 / Image definition"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/images [post]
func (h *Handler) CreateContainerImage(c *gin.Context) {
	var req ContainerImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	image, err := h.service.CreateContainerImage(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, image)
}

// UpdateContainerImage PUT /api/v1/packages/images/:id
// UpdateContainerImage 更新容器镜像。
// @Summary 更新容器镜像（管理员） / Update a container image (admin)
// @Tags packages
// @Produce json
// @Param id path int true "镜像 ID / Image ID"
// @Param body body map[string]interface{} true "镜像定义 / Image definition"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/images/{id} [put]
func (h *Handler) UpdateContainerImage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image id"})
		return
	}
	var req ContainerImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	image, err := h.service.UpdateContainerImage(c.Request.Context(), uint(id), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, image)
}

// DeleteContainerImage DELETE /api/v1/packages/images/:id
// DeleteContainerImage 删除容器镜像。
// @Summary 删除容器镜像（管理员） / Delete a container image (admin)
// @Tags packages
// @Produce json
// @Param id path int true "镜像 ID / Image ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/packages/images/{id} [delete]
func (h *Handler) DeleteContainerImage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image id"})
		return
	}
	if err := h.service.DeleteContainerImage(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
