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

package diagnostics

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ListTroubleshootingMemories handles GET /api/v1/diagnostics/troubleshooting-memories
// ListTroubleshootingMemories 处理 GET /api/v1/diagnostics/troubleshooting-memories
func (h *Handler) ListTroubleshootingMemories(c *gin.Context) {
	lang := strings.TrimSpace(c.Query("language"))
	if lang == "" {
		lang = strings.TrimSpace(c.Query("locale"))
	}
	if lang == "" {
		acceptLang := c.GetHeader("Accept-Language")
		if strings.HasPrefix(strings.ToLower(acceptLang), "en") {
			lang = "en"
		}
	}

	query := &TroubleshootingMemoryQuery{
		TargetType:  strings.TrimSpace(c.Query("target_type")),
		Fingerprint: strings.TrimSpace(c.Query("fingerprint")),
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		Language:    lang,
	}

	if clusterIDStr := strings.TrimSpace(c.Query("cluster_id")); clusterIDStr != "" {
		if id, err := strconv.ParseUint(clusterIDStr, 10, 32); err == nil {
			v := uint(id)
			query.ClusterID = &v
		}
	}

	if pageStr := strings.TrimSpace(c.Query("page")); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil {
			query.Page = p
		}
	}

	if pageSizeStr := strings.TrimSpace(c.Query("page_size")); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil {
			query.PageSize = ps
		}
	}

	items, total, err := h.service.ListTroubleshootingMemories(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list troubleshooting memories: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{
		Data: gin.H{
			"items": items,
			"total": total,
		},
	})
}

// GetTroubleshootingMemory handles GET /api/v1/diagnostics/troubleshooting-memories/:id
// GetTroubleshootingMemory 处理 GET /api/v1/diagnostics/troubleshooting-memories/:id
func (h *Handler) GetTroubleshootingMemory(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "Invalid memory id"})
		return
	}

	item, err := h.service.GetTroubleshootingMemory(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{ErrorMsg: "Troubleshooting memory not found: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: item})
}

// CreateTroubleshootingMemory handles POST /api/v1/diagnostics/troubleshooting-memories
// CreateTroubleshootingMemory 处理 POST /api/v1/diagnostics/troubleshooting-memories
func (h *Handler) CreateTroubleshootingMemory(c *gin.Context) {
	var req CreateTroubleshootingMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "Invalid request payload: " + err.Error()})
		return
	}

	item, err := h.service.CreateTroubleshootingMemory(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: item})
}

// UpdateTroubleshootingMemory handles PUT /api/v1/diagnostics/troubleshooting-memories/:id
// UpdateTroubleshootingMemory 处理 PUT /api/v1/diagnostics/troubleshooting-memories/:id
func (h *Handler) UpdateTroubleshootingMemory(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "Invalid memory id"})
		return
	}

	var req UpdateTroubleshootingMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "Invalid request payload: " + err.Error()})
		return
	}

	item, err := h.service.UpdateTroubleshootingMemory(c.Request.Context(), uint(id), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: item})
}

// DeleteTroubleshootingMemory handles DELETE /api/v1/diagnostics/troubleshooting-memories/:id
// DeleteTroubleshootingMemory 处理 DELETE /api/v1/diagnostics/troubleshooting-memories/:id
func (h *Handler) DeleteTroubleshootingMemory(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "Invalid memory id"})
		return
	}

	if err := h.service.DeleteTroubleshootingMemory(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to delete troubleshooting memory: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: gin.H{"success": true}})
}
