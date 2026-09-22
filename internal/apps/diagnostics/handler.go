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
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/gin-gonic/gin"
)

// Handler handles diagnostics workspace HTTP requests.
// Handler 处理诊断中心工作台 HTTP 请求。
type Handler struct {
	service   *Service
	auditRepo *audit.Repository
}

// NewHandler creates a diagnostics handler.
// NewHandler 创建诊断中心处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// SetAuditRepository 设置诊断资源访问使用的审计仓库。
// SetAuditRepository sets the audit repository used for diagnostic resource access.
func (h *Handler) SetAuditRepository(repo *audit.Repository) {
	h.auditRepo = repo
}

// GetWorkspaceBootstrap handles GET /api/v1/diagnostics/bootstrap
// GetWorkspaceBootstrap 处理 GET /api/v1/diagnostics/bootstrap
func (h *Handler) GetWorkspaceBootstrap(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	req := &WorkspaceBootstrapRequest{
		Source:  strings.TrimSpace(c.Query("source")),
		AlertID: strings.TrimSpace(c.Query("alert_id")),
	}

	if clusterIDStr := strings.TrimSpace(c.Query("cluster_id")); clusterIDStr != "" {
		clusterID, err := strconv.ParseUint(clusterIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid cluster_id"})
			return
		}
		value := uint(clusterID)
		req.ClusterID = &value
	}

	data, err := h.service.GetWorkspaceBootstrap(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get diagnostics workspace bootstrap: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, Response{Data: localizeWorkspaceBootstrapData(data, lang)})
}

// ListDiagnosticResources handles GET /api/v1/diagnostics/resources.
// ListDiagnosticResources 处理 GET /api/v1/diagnostics/resources。
// @Tags diagnostics
// @Produce json
// @Success 200 {object} Response
// @Router /api/v1/diagnostics/resources [get]
func (h *Handler) ListDiagnosticResources(c *gin.Context) {
	c.JSON(http.StatusOK, Response{Data: ListDiagnosticResources()})
}

// GetDiagnosticResource handles GET /api/v1/diagnostics/resources/:code.
// GetDiagnosticResource 处理 GET /api/v1/diagnostics/resources/:code。
// @Tags diagnostics
// @Produce json
// @Param code path string true "诊断资源编码"
// @Success 200 {object} Response
// @Failure 404 {object} Response
// @Router /api/v1/diagnostics/resources/{code} [get]
func (h *Handler) GetDiagnosticResource(c *gin.Context) {
	resource, err := GetDiagnosticResource(DiagnosticResourceCode(c.Param("code")))
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: resource})
}

// RunDiagnosticResource handles POST /api/v1/diagnostics/resources/:code/run.
// RunDiagnosticResource 处理 POST /api/v1/diagnostics/resources/:code/run。
// @Tags diagnostics
// @Accept json
// @Produce json
// @Param code path string true "诊断资源编码"
// @Param request body RunDiagnosticResourceRequest true "单项诊断资源请求"
// @Success 201 {object} Response
// @Failure 400 {object} Response
// @Failure 403 {object} Response
// @Router /api/v1/diagnostics/resources/{code}/run [post]
func (h *Handler) RunDiagnosticResource(c *gin.Context) {
	code := DiagnosticResourceCode(strings.TrimSpace(c.Param("code")))
	var req RunDiagnosticResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	requestHash, err := executionapp.HashRequest(struct {
		Code    DiagnosticResourceCode       `json:"code"`
		Request RunDiagnosticResourceRequest `json:"request"`
	}{Code: code, Request: req})
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	actor := currentDiagnosticActor(c)
	metadata := executionapp.MetadataFromGin(c)
	task, err := h.service.CreateDiagnosticResourceTask(
		c.Request.Context(),
		code,
		&req,
		uint(actor.UserID),
		auth.GetUsernameFromContext(c),
		DiagnosticExecutionRequest{
			RequestID:      metadata.RequestID,
			IdempotencyKey: metadata.IdempotencyKey,
			RequestHash:    requestHash,
			Confirmed:      metadata.Confirmed,
			ConfirmationID: metadata.ConfirmationID,
			IsAdmin:        actor.IsAdmin,
			ClientType:     metadata.ClientType,
		},
	)
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	c.JSON(http.StatusCreated, Response{Data: localizeDiagnosticTask(task, diagnosticsLanguageFromRequest(c))})
}

// ListSeatunnelErrorGroups handles GET /api/v1/diagnostics/errors/groups.
// ListSeatunnelErrorGroups 处理 GET /api/v1/diagnostics/errors/groups。
func (h *Handler) ListSeatunnelErrorGroups(c *gin.Context) {
	filter, err := parseErrorGroupFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.ListSeatunnelErrorGroups(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list diagnostics error groups: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// ListSeatunnelErrorEvents handles GET /api/v1/diagnostics/errors/events.
// ListSeatunnelErrorEvents 处理 GET /api/v1/diagnostics/errors/events。
func (h *Handler) ListSeatunnelErrorEvents(c *gin.Context) {
	filter, err := parseErrorEventFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.ListSeatunnelErrorEvents(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to list diagnostics error events: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// GetSeatunnelErrorGroupDetail handles GET /api/v1/diagnostics/errors/groups/:id.
// GetSeatunnelErrorGroupDetail 处理 GET /api/v1/diagnostics/errors/groups/:id。
func (h *Handler) GetSeatunnelErrorGroupDetail(c *gin.Context) {
	groupID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	eventLimit, err := parseOptionalInt(c.Query("event_limit"), 20)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	filter, err := parseErrorEventFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	filter.ErrorGroupID = groupID

	data, err := h.service.GetSeatunnelErrorGroupDetail(c.Request.Context(), filter, eventLimit)
	if err != nil {
		if errors.Is(err, ErrSeatunnelErrorGroupNotFound) {
			c.JSON(http.StatusNotFound, Response{ErrorMsg: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "Failed to get diagnostics error group detail: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// StartInspection handles POST /api/v1/diagnostics/inspections.
// StartInspection 处理 POST /api/v1/diagnostics/inspections。
func (h *Handler) StartInspection(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	var req StartClusterInspectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	executionItem, existing, err := h.beginDiagnosticsSynchronousWrite(c, "diagnostics.inspection.run", diagnosticsInspectionExecutionModule, "", executionapp.RiskLevelR1, diagnosticsInspectionRunImpact, req)
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	if existing {
		h.writeExistingDiagnosticsSynchronousWrite(c, executionItem)
		return
	}

	data, err := h.service.StartInspection(
		c.Request.Context(),
		&req,
		uint(auth.GetUserIDFromContext(c)),
		auth.GetUsernameFromContext(c),
	)
	resultRef := ""
	if data != nil && data.Report != nil {
		resultRef = strconv.FormatUint(uint64(data.Report.ID), 10)
	}
	h.finishDiagnosticsSynchronousWrite(c, executionItem, resultRef, err)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, Response{Data: localizeInspectionReportDetailData(data, lang)})
}

// ListInspectionReports handles GET /api/v1/diagnostics/inspections.
// ListInspectionReports 处理 GET /api/v1/diagnostics/inspections。
func (h *Handler) ListInspectionReports(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	filter, err := parseInspectionReportFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.ListInspectionReports(c.Request.Context(), filter)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: localizeInspectionReportsData(data, lang)})
}

// GetInspectionReportDetail handles GET /api/v1/diagnostics/inspections/:id.
// GetInspectionReportDetail 处理 GET /api/v1/diagnostics/inspections/:id。
func (h *Handler) GetInspectionReportDetail(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	reportID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.GetInspectionReportDetail(c.Request.Context(), reportID)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: localizeInspectionReportDetailData(data, lang)})
}

// CreateDiagnosticTask handles POST /api/v1/diagnostics/tasks.
// CreateDiagnosticTask 处理 POST /api/v1/diagnostics/tasks。
func (h *Handler) CreateDiagnosticTask(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	var req CreateDiagnosticTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	actor := currentDiagnosticActor(c)
	metadata := executionapp.MetadataFromGin(c)
	requestHash, err := executionapp.HashRequest(struct {
		ClusterID       uint                     `json:"cluster_id"`
		TriggerSource   DiagnosticTaskSourceType `json:"trigger_source"`
		SourceRef       DiagnosticTaskSourceRef  `json:"source_ref"`
		NodeScope       DiagnosticTaskNodeScope  `json:"node_scope"`
		SelectedNodeIDs []uint                   `json:"selected_node_ids"`
		Options         DiagnosticTaskOptions    `json:"options"`
		LookbackMinutes int                      `json:"lookback_minutes"`
		AutoStart       bool                     `json:"auto_start"`
	}{
		ClusterID:       req.ClusterID,
		TriggerSource:   req.TriggerSource,
		SourceRef:       req.SourceRef,
		NodeScope:       req.NodeScope,
		SelectedNodeIDs: req.SelectedNodeIDs,
		Options:         req.Options,
		LookbackMinutes: req.LookbackMinutes,
		AutoStart:       req.AutoStart,
	})
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}

	data, err := h.service.CreateDiagnosticTaskWithExecution(
		c.Request.Context(),
		&req,
		uint(actor.UserID),
		auth.GetUsernameFromContext(c),
		DiagnosticExecutionRequest{
			RequestID:      metadata.RequestID,
			IdempotencyKey: metadata.IdempotencyKey,
			RequestHash:    requestHash,
			Confirmed:      metadata.Confirmed,
			ConfirmationID: metadata.ConfirmationID,
			IsAdmin:        actor.IsAdmin,
			ClientType:     metadata.ClientType,
		},
	)
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	c.JSON(http.StatusCreated, Response{Data: localizeDiagnosticTask(data, lang)})
}

// ListDiagnosticTasks handles GET /api/v1/diagnostics/tasks.
// ListDiagnosticTasks 处理 GET /api/v1/diagnostics/tasks。
func (h *Handler) ListDiagnosticTasks(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	filter, err := parseDiagnosticTaskFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	items, total, err := h.service.ListDiagnosticTasksForActor(c.Request.Context(), currentDiagnosticActor(c), filter)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: localizeDiagnosticTaskListData(&DiagnosticTaskListData{
		Items:    items,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, lang)})
}

// GetDiagnosticTaskDetail handles GET /api/v1/diagnostics/tasks/:id.
// GetDiagnosticTaskDetail 处理 GET /api/v1/diagnostics/tasks/:id。
func (h *Handler) GetDiagnosticTask(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	taskID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.GetDiagnosticTaskForActor(c.Request.Context(), currentDiagnosticActor(c), taskID)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: localizeDiagnosticTask(data, lang)})
}

// ListDiagnosticTaskSteps handles GET /api/v1/diagnostics/tasks/:id/steps.
// ListDiagnosticTaskSteps 处理 GET /api/v1/diagnostics/tasks/:id/steps。
func (h *Handler) ListDiagnosticTaskSteps(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	taskID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.ListDiagnosticTaskStepsForActor(c.Request.Context(), currentDiagnosticActor(c), taskID)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: localizeDiagnosticTaskSteps(data, lang)})
}

// ListDiagnosticTaskArtifacts handles GET /api/v1/diagnostics/tasks/:id/artifacts.
// ListDiagnosticTaskArtifacts 处理 GET /api/v1/diagnostics/tasks/:id/artifacts。
// @Tags diagnostics
// @Produce json
// @Param id path int true "诊断任务 ID"
// @Success 200 {object} Response
// @Failure 404 {object} Response
// @Router /api/v1/diagnostics/tasks/{id}/artifacts [get]
func (h *Handler) ListDiagnosticTaskArtifacts(c *gin.Context) {
	taskID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	items, err := h.service.ListDiagnosticTaskArtifacts(c.Request.Context(), currentDiagnosticActor(c), taskID)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: map[string]any{"items": items, "total": len(items)}})
}

// ListDiagnosticTaskLogs handles GET /api/v1/diagnostics/tasks/:id/logs.
// ListDiagnosticTaskLogs 处理 GET /api/v1/diagnostics/tasks/:id/logs。
func (h *Handler) ListDiagnosticTaskLogs(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	taskID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	filter, err := parseDiagnosticTaskLogFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	filter.TaskID = taskID

	items, total, err := h.service.ListDiagnosticStepLogsForActor(c.Request.Context(), currentDiagnosticActor(c), filter)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: localizeDiagnosticTaskLogListData(&DiagnosticTaskLogListData{
		Items:    items,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, lang)})
}

// StartDiagnosticTask handles POST /api/v1/diagnostics/tasks/:id/start.
// StartDiagnosticTask 处理 POST /api/v1/diagnostics/tasks/:id/start。
func (h *Handler) StartDiagnosticTask(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	taskID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	actor := currentDiagnosticActor(c)
	executionItem, existing, err := h.beginDiagnosticsSynchronousWrite(c, "diagnostics.task.start", diagnosticsTaskStartExecutionModule, strconv.FormatUint(uint64(taskID), 10), executionapp.RiskLevelR1, diagnosticsTaskStartImpact, struct {
		TaskID uint `json:"task_id"`
	}{TaskID: taskID})
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	if existing {
		h.writeExistingDiagnosticsSynchronousWrite(c, executionItem)
		return
	}
	if err := h.service.StartDiagnosticTaskForActor(c.Request.Context(), actor, taskID); err != nil {
		h.finishDiagnosticsSynchronousWrite(c, executionItem, strconv.FormatUint(uint64(taskID), 10), err)
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	data, err := h.service.GetDiagnosticTaskForActor(c.Request.Context(), actor, taskID)
	h.finishDiagnosticsSynchronousWrite(c, executionItem, strconv.FormatUint(uint64(taskID), 10), err)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: localizeDiagnosticTask(data, lang)})
}

// StreamDiagnosticTaskEvents handles GET /api/v1/diagnostics/tasks/:id/events/stream.
// StreamDiagnosticTaskEvents 处理 GET /api/v1/diagnostics/tasks/:id/events/stream。
func (h *Handler) StreamDiagnosticTaskEvents(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	taskID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "streaming is not supported"})
		return
	}

	task, err := h.service.GetDiagnosticTaskForActor(c.Request.Context(), currentDiagnosticActor(c), taskID)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}

	events, unsubscribe := h.service.SubscribeDiagnosticTaskEvents(taskID)
	defer unsubscribe()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeEvent := func(event DiagnosticTaskEvent) bool {
		event = localizeDiagnosticTaskEvent(event, lang)
		payload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: %q\n\n", marshalErr.Error())
			flusher.Flush()
			return false
		}
		_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.EventType, payload)
		flusher.Flush()
		return true
	}

	if !writeEvent(newDiagnosticTaskSnapshotEvent(task)) {
		return
	}

	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if !writeEvent(event) {
				return
			}
		case <-keepAliveTicker.C:
			_, _ = fmt.Fprint(c.Writer, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

// PreviewDiagnosticTaskHTML handles GET /api/v1/diagnostics/tasks/:id/html.
// PreviewDiagnosticTaskHTML 处理 GET /api/v1/diagnostics/tasks/:id/html。
func (h *Handler) PreviewDiagnosticTaskHTML(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	task, path, err := h.resolveDiagnosticTaskFile(c, func(task *DiagnosticTask) string {
		return resolveDiagnosticLocalizedHTMLPath(task.IndexPath, lang)
	})
	if err != nil {
		h.writeDiagnosticTaskFileError(c, err)
		return
	}
	if err := h.recordDiagnosticResourceAccess(c, task, "diagnostics.file.preview", "html", filepath.Base(path)); err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "failed to record diagnostic resource access"})
		return
	}

	if c.Query("download") == "1" {
		c.FileAttachment(path, fmt.Sprintf("diagnostic-task-%d-summary.html", task.ID))
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.File(path)
}

// PreviewDiagnosticTaskFile handles GET /api/v1/diagnostics/tasks/:id/files/*path.
// PreviewDiagnosticTaskFile 处理 GET /api/v1/diagnostics/tasks/:id/files/*path。
func (h *Handler) PreviewDiagnosticTaskFile(c *gin.Context) {
	relativePath := strings.TrimPrefix(strings.TrimSpace(c.Param("path")), "/")
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "diagnostic bundle file path is required"})
		return
	}

	task, path, err := h.resolveDiagnosticTaskFile(c, func(task *DiagnosticTask) string {
		bundleDir := strings.TrimSpace(task.BundleDir)
		if bundleDir == "" {
			bundleDir = diagnosticTaskBundleDir(task.ID)
		}
		return filepath.Join(bundleDir, filepath.Clean(filepath.FromSlash(relativePath)))
	})
	if err != nil {
		h.writeDiagnosticTaskFileError(c, err)
		return
	}
	if err := h.recordDiagnosticResourceAccess(c, task, "diagnostics.file.read", strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), relativePath); err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "failed to record diagnostic resource access"})
		return
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".html":
		c.Header("Content-Type", "text/html; charset=utf-8")
	case ".log", ".txt", ".json", ".yaml", ".yml", ".properties":
		c.Header("Content-Type", "text/plain; charset=utf-8")
	}
	c.File(path)
}

// DownloadDiagnosticTaskBundle handles GET /api/v1/diagnostics/tasks/:id/bundle.
// DownloadDiagnosticTaskBundle 处理 GET /api/v1/diagnostics/tasks/:id/bundle。
func (h *Handler) DownloadDiagnosticTaskBundle(c *gin.Context) {
	task, bundleDir, err := h.resolveDiagnosticTaskFile(c, func(task *DiagnosticTask) string {
		return task.BundleDir
	})
	if err != nil {
		h.writeDiagnosticTaskFileError(c, err)
		return
	}
	if err := h.recordDiagnosticResourceAccess(c, task, "diagnostics.bundle.download", "zip", ""); err != nil {
		c.JSON(http.StatusInternalServerError, Response{ErrorMsg: "failed to record diagnostic resource access"})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fmt.Sprintf("diagnostic-task-%d.zip", task.ID)))

	zipWriter := zip.NewWriter(c.Writer)
	defer func() {
		_ = zipWriter.Close()
	}()

	walkErr := filepath.Walk(bundleDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil || info.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(bundleDir, path)
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relativePath)
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr != nil {
		c.Error(walkErr)
	}
}

func (h *Handler) recordDiagnosticResourceAccess(c *gin.Context, task *DiagnosticTask, action, fileType, relativePath string) error {
	if task == nil {
		return nil
	}
	details := audit.AuditDetails{
		"task_id":       task.ID,
		"execution_id":  task.ExecutionID,
		"file_type":     fileType,
		"relative_path": relativePath,
		"result_status": "succeeded",
		"trigger":       "manual",
	}
	return audit.RecordFromGin(
		c,
		h.auditRepo,
		auth.GetUserIDFromContext(c),
		auth.GetUsernameFromContext(c),
		action,
		"diagnostic_resource",
		audit.UintID(task.ID),
		fileType,
		details,
	)
}

func (h *Handler) resolveDiagnosticTaskFile(c *gin.Context, selector func(task *DiagnosticTask) string) (*DiagnosticTask, string, error) {
	taskID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		return nil, "", err
	}

	task, err := h.service.GetDiagnosticTaskForActor(c.Request.Context(), currentDiagnosticActor(c), taskID)
	if err != nil {
		return nil, "", err
	}

	path := strings.TrimSpace(selector(task))
	if path == "" {
		return nil, "", os.ErrNotExist
	}

	bundleDir := strings.TrimSpace(task.BundleDir)
	if bundleDir == "" {
		bundleDir = diagnosticTaskBundleDir(task.ID)
	}

	absBundleDir, err := filepath.Abs(bundleDir)
	if err != nil {
		return nil, "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	if absPath != absBundleDir && !strings.HasPrefix(absPath, absBundleDir+string(os.PathSeparator)) {
		return nil, "", fmt.Errorf("diagnostics: invalid task file path")
	}
	if _, err := os.Stat(absPath); err != nil {
		return nil, "", err
	}
	return task, absPath, nil
}

func (h *Handler) writeDiagnosticTaskFileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, Response{ErrorMsg: "diagnostic bundle file not found"})
	case errors.Is(err, ErrDiagnosticTaskNotFound):
		c.JSON(http.StatusNotFound, Response{ErrorMsg: err.Error()})
	case strings.Contains(err.Error(), "invalid task file path"):
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
	default:
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
	}
}

func diagnosticsLanguageFromRequest(c *gin.Context) DiagnosticLanguage {
	if c == nil {
		return DiagnosticLanguageZH
	}
	return normalizeDiagnosticLanguage(c.Query("lang"))
}

func parseErrorGroupFilter(c *gin.Context) (*SeatunnelErrorGroupFilter, error) {
	clusterID, err := parseOptionalUint(c.Query("cluster_id"))
	if err != nil {
		return nil, err
	}
	nodeID, err := parseOptionalUint(c.Query("node_id"))
	if err != nil {
		return nil, err
	}
	hostID, err := parseOptionalUint(c.Query("host_id"))
	if err != nil {
		return nil, err
	}
	page, err := parseOptionalInt(c.Query("page"), 1)
	if err != nil {
		return nil, err
	}
	pageSize, err := parseOptionalInt(c.Query("page_size"), 20)
	if err != nil {
		return nil, err
	}
	startTime, err := parseOptionalTime(c.Query("start_time"))
	if err != nil {
		return nil, err
	}
	endTime, err := parseOptionalTime(c.Query("end_time"))
	if err != nil {
		return nil, err
	}

	return &SeatunnelErrorGroupFilter{
		ClusterID:      clusterID,
		NodeID:         nodeID,
		HostID:         hostID,
		Role:           strings.TrimSpace(c.Query("role")),
		JobID:          strings.TrimSpace(c.Query("job_id")),
		Keyword:        strings.TrimSpace(c.Query("keyword")),
		ExceptionClass: strings.TrimSpace(c.Query("exception_class")),
		StartTime:      startTime,
		EndTime:        endTime,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}

func parseErrorEventFilter(c *gin.Context) (*SeatunnelErrorEventFilter, error) {
	groupID, err := parseOptionalUint(c.Query("group_id"))
	if err != nil {
		return nil, err
	}
	clusterID, err := parseOptionalUint(c.Query("cluster_id"))
	if err != nil {
		return nil, err
	}
	nodeID, err := parseOptionalUint(c.Query("node_id"))
	if err != nil {
		return nil, err
	}
	hostID, err := parseOptionalUint(c.Query("host_id"))
	if err != nil {
		return nil, err
	}
	page, err := parseOptionalInt(c.Query("page"), 1)
	if err != nil {
		return nil, err
	}
	pageSize, err := parseOptionalInt(c.Query("page_size"), 20)
	if err != nil {
		return nil, err
	}
	startTime, err := parseOptionalTime(c.Query("start_time"))
	if err != nil {
		return nil, err
	}
	endTime, err := parseOptionalTime(c.Query("end_time"))
	if err != nil {
		return nil, err
	}

	return &SeatunnelErrorEventFilter{
		ErrorGroupID:   groupID,
		ClusterID:      clusterID,
		NodeID:         nodeID,
		HostID:         hostID,
		Role:           strings.TrimSpace(c.Query("role")),
		JobID:          strings.TrimSpace(c.Query("job_id")),
		Keyword:        strings.TrimSpace(c.Query("keyword")),
		ExceptionClass: strings.TrimSpace(c.Query("exception_class")),
		StartTime:      startTime,
		EndTime:        endTime,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}

func parseInspectionReportFilter(c *gin.Context) (*ClusterInspectionReportFilter, error) {
	clusterID, err := parseOptionalUint(c.Query("cluster_id"))
	if err != nil {
		return nil, err
	}
	page, err := parseOptionalInt(c.Query("page"), 1)
	if err != nil {
		return nil, err
	}
	pageSize, err := parseOptionalInt(c.Query("page_size"), 20)
	if err != nil {
		return nil, err
	}
	startTime, err := parseOptionalTime(c.Query("start_time"))
	if err != nil {
		return nil, err
	}
	endTime, err := parseOptionalTime(c.Query("end_time"))
	if err != nil {
		return nil, err
	}

	return &ClusterInspectionReportFilter{
		ClusterID:     clusterID,
		Status:        InspectionReportStatus(strings.TrimSpace(c.Query("status"))),
		TriggerSource: InspectionTriggerSource(strings.TrimSpace(c.Query("trigger_source"))),
		Severity:      InspectionFindingSeverity(strings.TrimSpace(c.Query("severity"))),
		StartTime:     startTime,
		EndTime:       endTime,
		Page:          page,
		PageSize:      pageSize,
	}, nil
}

func parseDiagnosticTaskFilter(c *gin.Context) (*DiagnosticTaskListFilter, error) {
	clusterID, err := parseOptionalUint(c.Query("cluster_id"))
	if err != nil {
		return nil, err
	}
	page, err := parseOptionalInt(c.Query("page"), 1)
	if err != nil {
		return nil, err
	}
	pageSize, err := parseOptionalInt(c.Query("page_size"), 20)
	if err != nil {
		return nil, err
	}
	return &DiagnosticTaskListFilter{
		ClusterID:     clusterID,
		TriggerSource: DiagnosticTaskSourceType(strings.TrimSpace(c.Query("trigger_source"))),
		Status:        DiagnosticTaskStatus(strings.TrimSpace(c.Query("status"))),
		Page:          page,
		PageSize:      pageSize,
	}, nil
}

func parseDiagnosticTaskLogFilter(c *gin.Context) (*DiagnosticTaskLogFilter, error) {
	page, err := parseOptionalInt(c.Query("page"), 1)
	if err != nil {
		return nil, err
	}
	pageSize, err := parseOptionalInt(c.Query("page_size"), 50)
	if err != nil {
		return nil, err
	}
	nodeExecutionID, err := parseOptionalUintPointer(c.Query("node_execution_id"))
	if err != nil {
		return nil, err
	}
	return &DiagnosticTaskLogFilter{
		StepCode:        DiagnosticStepCode(strings.TrimSpace(c.Query("step_code"))),
		NodeExecutionID: nodeExecutionID,
		Level:           DiagnosticLogLevel(strings.TrimSpace(c.Query("level"))),
		Page:            page,
		PageSize:        pageSize,
	}, nil
}

func parseOptionalUint(raw string) (uint, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	return parseUintQueryValue(value, value)
}

func parseUintQueryValue(raw string, field string) (uint, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return 0, errors.New("invalid " + field)
	}
	return uint(parsed), nil
}

func parseOptionalUintPointer(raw string) (*uint, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := parseUintQueryValue(value, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseOptionalInt(raw string, defaultValue int) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New("invalid integer query value")
	}
	return parsed, nil
}

func parseOptionalTime(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if ts, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return &ts, nil
		}
	}
	return nil, errors.New("invalid time query value")
}

func getDiagnosticsStatusCode(err error) int {
	switch {
	case errors.Is(err, ErrInvalidSeatunnelErrorRequest), errors.Is(err, ErrInvalidInspectionRequest), errors.Is(err, ErrInvalidDiagnosticTaskRequest), errors.Is(err, ErrInvalidAutoPolicyRequest):
		return http.StatusBadRequest
	case errors.Is(err, executionapp.ErrIdempotencyKeyMissing):
		return http.StatusBadRequest
	case errors.Is(err, ErrSeatunnelErrorGroupNotFound), errors.Is(err, ErrInspectionReportNotFound), errors.Is(err, ErrInspectionFindingNotFound), errors.Is(err, ErrDiagnosticTaskNotFound), errors.Is(err, ErrDiagnosticResourceNotFound), errors.Is(err, clusterapp.ErrClusterNotFound), errors.Is(err, ErrAutoPolicyNotFound):
		return http.StatusNotFound
	case errors.Is(err, executionapp.ErrPermissionDenied), errors.Is(err, executionapp.ErrAdminRequired):
		return http.StatusForbidden
	case errors.Is(err, ErrDiagnosticTaskAlreadyFinished), errors.Is(err, executionapp.ErrIdempotencyConflict), errors.Is(err, executionapp.ErrConcurrentUpdate), errors.Is(err, executionapp.ErrInvalidTransition), errors.Is(err, executionapp.ErrConfirmationInvalid):
		return http.StatusConflict
	case errors.Is(err, executionapp.ErrExplicitConfirmNeeded), errors.Is(err, executionapp.ErrConfirmationRequired):
		return http.StatusPreconditionRequired
	default:
		return http.StatusInternalServerError
	}
}

// writeDiagnosticsError 输出诊断接口错误，并保留公共执行确认信息。
// writeDiagnosticsError writes a diagnostic API error while preserving shared-execution confirmation details.
func (h *Handler) writeDiagnosticsError(c *gin.Context, err error) {
	var confirmationErr *executionapp.ConfirmationRequiredError
	if errors.As(err, &confirmationErr) {
		c.JSON(http.StatusPreconditionRequired, Response{
			ErrorMsg: confirmationErr.Error(),
			Data: map[string]any{
				"confirmation_required": true,
				"confirmation_id":       confirmationErr.ConfirmationID,
				"risk_level":            confirmationErr.RiskLevel,
				"impact":                confirmationErr.Impact,
				"expires_at":            confirmationErr.ExpiresAt,
			},
		})
		return
	}
	c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
}

func currentDiagnosticActor(c *gin.Context) executionapp.Actor {
	user := auth.GetUserFromContext(c)
	if user == nil {
		return executionapp.Actor{}
	}
	return executionapp.Actor{UserID: uint64(user.ID), IsAdmin: user.IsAdmin}
}

// validateAutoPolicyTaskOptionsForActor 阻止普通用户通过自动策略绕过 JVM Dump 管理员限制。
// validateAutoPolicyTaskOptionsForActor prevents non-admin users from bypassing JVM dump restrictions through auto policies.
func validateAutoPolicyTaskOptionsForActor(actor executionapp.Actor, options *DiagnosticTaskOptions) error {
	if options == nil {
		return nil
	}
	if options.Normalize().IncludeJVMDump && !actor.IsAdmin {
		return executionapp.ErrAdminRequired
	}
	return nil
}

// ListBuiltinConditionTemplates handles GET /api/v1/diagnostics/auto-policies/templates.
// ListBuiltinConditionTemplates 处理 GET /api/v1/diagnostics/auto-policies/templates。
func (h *Handler) ListBuiltinConditionTemplates(c *gin.Context) {
	lang := diagnosticsLanguageFromRequest(c)
	templates := h.service.ListBuiltinConditionTemplates()
	c.JSON(http.StatusOK, Response{Data: localizeBuiltinConditionTemplates(templates, lang)})
}

// ListAutoPolicies handles GET /api/v1/diagnostics/auto-policies.
// ListAutoPolicies 处理 GET /api/v1/diagnostics/auto-policies。
func (h *Handler) ListAutoPolicies(c *gin.Context) {
	var clusterID *uint
	if raw := strings.TrimSpace(c.Query("cluster_id")); raw != "" {
		parsed, err := parseUintQueryValue(raw, "cluster_id")
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
			return
		}
		clusterID = &parsed
	}
	page, err := parseOptionalInt(c.Query("page"), 1)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	pageSize, err := parseOptionalInt(c.Query("page_size"), 20)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.ListAutoPolicies(c.Request.Context(), clusterID, page, pageSize)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// CreateAutoPolicy handles POST /api/v1/diagnostics/auto-policies.
// CreateAutoPolicy 处理 POST /api/v1/diagnostics/auto-policies。
func (h *Handler) CreateAutoPolicy(c *gin.Context) {
	var req CreateInspectionAutoPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	if err := validateAutoPolicyTaskOptionsForActor(currentDiagnosticActor(c), req.TaskOptions); err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	executionItem, existing, err := h.beginDiagnosticsSynchronousWrite(c, "diagnostics.auto-policy.create", diagnosticsAutoPolicyExecutionModule, "", executionapp.RiskLevelR1, diagnosticsAutoPolicyCreateImpact, req)
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	if existing {
		h.writeExistingDiagnosticsSynchronousWrite(c, executionItem)
		return
	}

	data, err := h.service.CreateAutoPolicy(
		c.Request.Context(),
		uint(auth.GetUserIDFromContext(c)),
		&req,
	)
	resultRef := ""
	if data != nil {
		resultRef = strconv.FormatUint(uint64(data.ID), 10)
	}
	h.finishDiagnosticsSynchronousWrite(c, executionItem, resultRef, err)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, Response{Data: data})
}

// GetAutoPolicy handles GET /api/v1/diagnostics/auto-policies/:id.
// GetAutoPolicy 处理 GET /api/v1/diagnostics/auto-policies/:id。
func (h *Handler) GetAutoPolicy(c *gin.Context) {
	policyID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	data, err := h.service.GetAutoPolicy(c.Request.Context(), policyID)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// UpdateAutoPolicy handles PUT /api/v1/diagnostics/auto-policies/:id.
// UpdateAutoPolicy 处理 PUT /api/v1/diagnostics/auto-policies/:id。
func (h *Handler) UpdateAutoPolicy(c *gin.Context) {
	policyID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	var req UpdateInspectionAutoPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}
	if err := validateAutoPolicyTaskOptionsForActor(currentDiagnosticActor(c), req.TaskOptions); err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	executionItem, existing, err := h.beginDiagnosticsSynchronousWrite(c, "diagnostics.auto-policy.update", diagnosticsAutoPolicyExecutionModule, strconv.FormatUint(uint64(policyID), 10), executionapp.RiskLevelR1, diagnosticsAutoPolicyUpdateImpact, struct {
		ID      uint                              `json:"id"`
		Request UpdateInspectionAutoPolicyRequest `json:"request"`
	}{ID: policyID, Request: req})
	if err != nil {
		h.writeDiagnosticsError(c, err)
		return
	}
	if existing {
		h.writeExistingDiagnosticsSynchronousWrite(c, executionItem)
		return
	}

	data, err := h.service.UpdateAutoPolicy(c.Request.Context(), policyID, &req)
	resultRef := ""
	if data != nil {
		resultRef = strconv.FormatUint(uint64(data.ID), 10)
	}
	h.finishDiagnosticsSynchronousWrite(c, executionItem, resultRef, err)
	if err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// DeleteAutoPolicy handles DELETE /api/v1/diagnostics/auto-policies/:id.
// DeleteAutoPolicy 处理 DELETE /api/v1/diagnostics/auto-policies/:id。
func (h *Handler) DeleteAutoPolicy(c *gin.Context) {
	policyID, err := parseUintQueryValue(c.Param("id"), "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error()})
		return
	}

	if err := h.service.DeleteAutoPolicy(c.Request.Context(), policyID); err != nil {
		c.JSON(getDiagnosticsStatusCode(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: nil})
}
