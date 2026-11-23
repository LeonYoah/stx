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

package discovery

import (
	"github.com/LeonYoah/stx/internal/pkg/locale"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for cluster discovery.
// Handler 处理集群发现的 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler creates a new discovery handler.
// NewHandler 创建新的发现处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// DiscoverProcesses handles POST /api/v1/hosts/:id/discover-processes
// DiscoverProcesses 处理 POST /api/v1/hosts/:id/discover-processes
// @Summary Discover SeaTunnel processes on a host (simplified)
// @Description Scan for running SeaTunnel processes, returns PID, role, install_dir only
// @Tags Discovery
// @Accept json
// @Produce json
// @Param id path int true "Host ID"
// @Success 200 {object} ProcessDiscoveryResult
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/hosts/{id}/discover-processes [post]
func (h *Handler) DiscoverProcesses(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": locale.Pick(c.Request.Context(), "无效的主机 ID", "invalid host id")})
		return
	}

	result, err := h.service.DiscoverProcesses(c.Request.Context(), uint(hostID))
	if err != nil {
		switch err {
		case ErrHostNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case ErrAgentNotInstalled, ErrAgentOffline:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, result)
}

// TriggerDiscovery handles POST /api/v1/hosts/:id/discover
// TriggerDiscovery 处理 POST /api/v1/hosts/:id/discover
// @Summary Trigger cluster discovery on a host
// @Description Trigger SeaTunnel cluster discovery on a specific host via Agent
// @Tags Discovery
// @Accept json
// @Produce json
// @Param id path int true "Host ID"
// @Success 200 {object} DiscoveryResult
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/hosts/{id}/discover [post]
func (h *Handler) TriggerDiscovery(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": locale.Pick(c.Request.Context(), "无效的主机 ID", "invalid host id")})
		return
	}

	result, err := h.service.TriggerDiscovery(c.Request.Context(), uint(hostID))
	if err != nil {
		switch err {
		case ErrHostNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case ErrAgentNotInstalled, ErrAgentOffline:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, result)
}

// ConfirmDiscovery handles POST /api/v1/hosts/:id/discover/confirm
// ConfirmDiscovery 处理 POST /api/v1/hosts/:id/discover/confirm
// @Summary Confirm and import discovered clusters
// @Description Confirm discovered clusters and import them into the system
// @Tags Discovery
// @Accept json
// @Produce json
// @Param id path int true "Host ID"
// @Param request body ConfirmDiscoveryRequest true "Confirm request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/hosts/{id}/discover/confirm [post]
func (h *Handler) ConfirmDiscovery(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": locale.Pick(c.Request.Context(), "无效的主机 ID", "invalid host id")})
		return
	}

	var req ConfirmDiscoveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.HostID = uint(hostID)

	createdIDs, err := h.service.ConfirmDiscovery(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case ErrHostNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case ErrAgentNotInstalled, ErrAgentOffline:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     locale.Pick(c.Request.Context(), "集群导入成功", "clusters imported successfully"),
		"cluster_ids": createdIDs,
		"count":       len(createdIDs),
	})
}
