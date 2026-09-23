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
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const (
	troubleshootingMemoryExecutionModule = "diagnostics_troubleshooting_memory"
	troubleshootingMemoryCreateImpact    = "新增一条排障经验记录，后续诊断和人工排查可以读取该内容。"
	troubleshootingMemoryUpdateImpact    = "修改排障经验记录会改变后续诊断和人工排查读取到的内容。"
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
	requestHash, err := executionapp.HashRequest(req)
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	executionItem, existing, err := h.beginTroubleshootingMemoryWrite(c, "diagnostics.troubleshooting-memory.create", "", troubleshootingMemoryCreateImpact, requestHash)
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	if existing {
		h.writeExistingTroubleshootingMemory(c, executionItem)
		return
	}

	item, err := h.service.CreateTroubleshootingMemory(c.Request.Context(), &req)
	resultRef := ""
	if item != nil {
		resultRef = item.ID
	}
	h.finishTroubleshootingMemoryWrite(c, executionItem, resultRef, err)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	h.recordTroubleshootingMemoryAudit(c, "create", item)

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
	requestHash, err := executionapp.HashRequest(struct {
		ID      uint                               `json:"id"`
		Request UpdateTroubleshootingMemoryRequest `json:"request"`
	}{ID: uint(id), Request: req})
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	executionItem, existing, err := h.beginTroubleshootingMemoryWrite(c, "diagnostics.troubleshooting-memory.update", idStr, troubleshootingMemoryUpdateImpact, requestHash)
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	if existing {
		h.writeExistingTroubleshootingMemory(c, executionItem)
		return
	}

	item, err := h.service.UpdateTroubleshootingMemory(c.Request.Context(), uint(id), &req)
	resultRef := ""
	if item != nil {
		resultRef = item.ID
	}
	h.finishTroubleshootingMemoryWrite(c, executionItem, resultRef, err)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	h.recordTroubleshootingMemoryAudit(c, "update", item)

	c.JSON(http.StatusOK, Response{Data: item})
}

// beginTroubleshootingMemoryWrite 为 CLI/API 写请求执行确认和幂等校验，网页请求保持现有行为。
// beginTroubleshootingMemoryWrite applies confirmation and idempotency checks to CLI/API writes while preserving web behavior.
func (h *Handler) beginTroubleshootingMemoryWrite(c *gin.Context, operationID, moduleRef, impact, requestHash string) (*executionapp.Execution, bool, error) {
	metadata := executionapp.MetadataFromGin(c)
	if h == nil || h.service == nil || h.service.executionService == nil || metadata.ClientType == "web" {
		return nil, false, nil
	}
	return h.service.executionService.BeginSynchronous(c.Request.Context(), currentDiagnosticActor(c), executionapp.SynchronousInput{
		OperationID: operationID, Module: troubleshootingMemoryExecutionModule, ModuleRef: moduleRef,
		RequestID: metadata.RequestID, IdempotencyKey: metadata.IdempotencyKey, RequestHash: requestHash,
		RiskLevel: executionapp.RiskLevelR1, Impact: impact, Confirmed: metadata.Confirmed,
		ConfirmationID: metadata.ConfirmationID, ClientType: metadata.ClientType,
	})
}

// finishTroubleshootingMemoryWrite 在客户端断开后仍完成公共执行状态记录。
// finishTroubleshootingMemoryWrite completes the shared execution record even after client cancellation.
func (h *Handler) finishTroubleshootingMemoryWrite(c *gin.Context, item *executionapp.Execution, resultRef string, runErr error) {
	if h == nil || h.service == nil || h.service.executionService == nil || item == nil {
		return
	}
	finishContext := context.WithoutCancel(c.Request.Context())
	if err := h.service.executionService.FinishSynchronous(finishContext, item, resultRef, runErr); err != nil {
		logger.WarnF(finishContext, "[Diagnostics] 更新排障经验公共执行记录失败: execution_id=%s err=%v", item.ExecutionID, err)
	}
}

// writeExistingTroubleshootingMemory 返回幂等请求第一次执行产生的记录。
// writeExistingTroubleshootingMemory returns the record produced by the first idempotent request.
func (h *Handler) writeExistingTroubleshootingMemory(c *gin.Context, item *executionapp.Execution) {
	if item == nil || item.Status != executionapp.StatusSucceeded {
		status := executionapp.Status("")
		if item != nil {
			status = item.Status
		}
		h.writeDiagnosticsError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(item.ResultRef), 10, 32)
	if err != nil {
		h.writeDiagnosticsError(c, executionapp.ErrConcurrentUpdate)
		return
	}
	memory, err := h.service.GetTroubleshootingMemory(c.Request.Context(), uint(id))
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	c.JSON(http.StatusOK, Response{Data: memory})
}

// recordTroubleshootingMemoryAudit 记录排障经验写操作的发起用户和客户端来源。
// recordTroubleshootingMemoryAudit records the actor and client source of a troubleshooting-memory write.
func (h *Handler) recordTroubleshootingMemoryAudit(c *gin.Context, action string, item *TroubleshootingMemoryItem) {
	if h == nil || h.auditRepo == nil || item == nil {
		return
	}
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		action, "troubleshooting_memory", item.ID, item.Title,
		audit.AuditDetails{"trigger": "manual", "risk_level": "R1", "result_status": "succeeded"})
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
