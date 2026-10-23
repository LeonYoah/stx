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
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

// ScaleHandler 提供集群扩缩容的 HTTP 接口。
// ScaleHandler serves the cluster scaling HTTP endpoints.
type ScaleHandler struct {
	service   *ScaleService
	auditRepo *audit.Repository
}

// NewScaleHandler 创建扩缩容处理器；auditRepo 为 nil 时跳过业务审计（公共执行审计仍会记录）。
// NewScaleHandler creates the scaling handler; with a nil auditRepo the business audit is skipped (shared execution
// audits are still recorded).
func NewScaleHandler(service *ScaleService, auditRepo *audit.Repository) *ScaleHandler {
	return &ScaleHandler{service: service, auditRepo: auditRepo}
}

// ScalePlanResponse 是伸缩计划响应。
// ScalePlanResponse is the scaling plan response.
type ScalePlanResponse struct {
	ErrorCode string     `json:"error_code,omitempty"`
	ErrorMsg  string     `json:"error_msg"`
	Data      *ScalePlan `json:"data"`
}

// ScaleSubmitResponse 是提交或重试伸缩操作的响应。
// ScaleSubmitResponse is the response of submitting or retrying a scaling operation.
type ScaleSubmitResponse struct {
	ErrorCode string             `json:"error_code,omitempty"`
	ErrorMsg  string             `json:"error_msg"`
	Data      *ScaleSubmitResult `json:"data"`
}

// ScaleOperationResponse 是伸缩操作详情响应。
// ScaleOperationResponse is the scaling operation detail response.
type ScaleOperationResponse struct {
	ErrorCode string              `json:"error_code,omitempty"`
	ErrorMsg  string              `json:"error_msg"`
	Data      *ScaleOperationInfo `json:"data"`
}

// ScaleOperationListData 是伸缩操作列表数据，Items 永不为 null。
// ScaleOperationListData is the scaling operation list payload; Items is never null.
type ScaleOperationListData struct {
	Items []*ScaleOperationInfo `json:"items"`
}

// ScaleOperationListResponse 是伸缩操作列表响应。
// ScaleOperationListResponse is the scaling operation list response.
type ScaleOperationListResponse struct {
	ErrorCode string                  `json:"error_code,omitempty"`
	ErrorMsg  string                  `json:"error_msg"`
	Data      *ScaleOperationListData `json:"data"`
}

// scaleErrorResponse 是伸缩接口的错误响应。
// scaleErrorResponse is the error response of scaling endpoints.
type scaleErrorResponse struct {
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg"`
	Data      any    `json:"data"`
}

// PlanScaleOut 处理 POST /api/v1/clusters/:id/scale-out/plan，返回扩容计划（R0，只读）。
// PlanScaleOut handles POST /api/v1/clusters/:id/scale-out/plan and returns a scale-out plan (R0, read-only).
// @Summary 计算扩容计划 / Compute a scale-out plan
// @Tags clusters
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param lang query string false "zh / en"
// @Param request body ScaleOutRequest true "扩容计划请求"
// @Success 200 {object} ScalePlanResponse
// @Router /api/v1/clusters/{id}/scale-out/plan [post]
func (h *ScaleHandler) PlanScaleOut(c *gin.Context) {
	clusterID, ok := scaleUintParam(c, "id")
	if !ok {
		return
	}
	var req ScaleOutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeScaleError(c, errors.Join(ErrScaleInvalidRequest, err))
		return
	}
	plan, err := h.service.PlanScaleOut(c.Request.Context(), clusterID, &req, c.Query("lang"))
	if err != nil {
		writeScaleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ScalePlanResponse{Data: plan})
}

// ScaleOut 处理 POST /api/v1/clusters/:id/scale-out，提交扩容（R2，两步确认，异步执行）。
// ScaleOut handles POST /api/v1/clusters/:id/scale-out and submits a scale-out (R2, two-step confirmation, async).
// @Summary 提交扩容 / Submit a scale-out
// @Tags clusters
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param lang query string false "zh / en"
// @Param Idempotency-Key header string true "幂等键"
// @Param X-STX-Confirmation-ID header string false "一次性确认编号"
// @Param request body ScaleOutSubmitRequest true "扩容提交请求"
// @Success 200 {object} ScaleSubmitResponse
// @Router /api/v1/clusters/{id}/scale-out [post]
func (h *ScaleHandler) ScaleOut(c *gin.Context) {
	clusterID, ok := scaleUintParam(c, "id")
	if !ok {
		return
	}
	var req ScaleOutSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeScaleError(c, errors.Join(ErrScaleInvalidRequest, err))
		return
	}
	meta, err := scaleExecutionMeta(c, struct {
		ClusterID uint                  `json:"cluster_id"`
		Request   ScaleOutSubmitRequest `json:"request"`
	}{ClusterID: clusterID, Request: req})
	if err != nil {
		writeScaleError(c, err)
		return
	}
	result, err := h.service.SubmitScaleOut(c.Request.Context(), scaleActorFromGin(c), clusterID, &req, meta)
	if err != nil {
		writeScaleError(c, err)
		return
	}
	h.recordAudit(c, "scale_out", clusterID, result, audit.AuditDetails{"host_id": req.HostID, "mode": string(result.Operation.Mode)})
	c.JSON(http.StatusOK, ScaleSubmitResponse{Data: result})
}

// PlanScaleIn 处理 POST /api/v1/clusters/:id/nodes/:nodeId/scale-in/plan，返回缩容计划（R0，只读）。
// PlanScaleIn handles POST /api/v1/clusters/:id/nodes/:nodeId/scale-in/plan and returns a scale-in plan (R0).
// @Summary 计算缩容计划 / Compute a scale-in plan
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Param nodeId path int true "节点ID"
// @Param lang query string false "zh / en"
// @Success 200 {object} ScalePlanResponse
// @Router /api/v1/clusters/{id}/nodes/{nodeId}/scale-in/plan [post]
func (h *ScaleHandler) PlanScaleIn(c *gin.Context) {
	clusterID, ok := scaleUintParam(c, "id")
	if !ok {
		return
	}
	nodeID, ok := scaleUintParam(c, "nodeId")
	if !ok {
		return
	}
	plan, err := h.service.PlanScaleIn(c.Request.Context(), clusterID, nodeID, c.Query("lang"))
	if err != nil {
		writeScaleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ScalePlanResponse{Data: plan})
}

// ScaleIn 处理 POST /api/v1/clusters/:id/nodes/:nodeId/scale-in，提交缩容（R2，两步确认，异步执行）。
// ScaleIn handles POST /api/v1/clusters/:id/nodes/:nodeId/scale-in and submits a scale-in (R2, two-step, async).
// @Summary 提交缩容 / Submit a scale-in
// @Tags clusters
// @Accept json
// @Produce json
// @Param id path int true "集群ID"
// @Param nodeId path int true "节点ID"
// @Param lang query string false "zh / en"
// @Param Idempotency-Key header string true "幂等键"
// @Param X-STX-Confirmation-ID header string false "一次性确认编号"
// @Param request body ScaleInSubmitRequest true "缩容提交请求"
// @Success 200 {object} ScaleSubmitResponse
// @Router /api/v1/clusters/{id}/nodes/{nodeId}/scale-in [post]
func (h *ScaleHandler) ScaleIn(c *gin.Context) {
	clusterID, ok := scaleUintParam(c, "id")
	if !ok {
		return
	}
	nodeID, ok := scaleUintParam(c, "nodeId")
	if !ok {
		return
	}
	var req ScaleInSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeScaleError(c, errors.Join(ErrScaleInvalidRequest, err))
		return
	}
	meta, err := scaleExecutionMeta(c, struct {
		ClusterID uint                 `json:"cluster_id"`
		NodeID    uint                 `json:"node_id"`
		Request   ScaleInSubmitRequest `json:"request"`
	}{ClusterID: clusterID, NodeID: nodeID, Request: req})
	if err != nil {
		writeScaleError(c, err)
		return
	}
	result, err := h.service.SubmitScaleIn(c.Request.Context(), scaleActorFromGin(c), clusterID, nodeID, &req, meta)
	if err != nil {
		writeScaleError(c, err)
		return
	}
	h.recordAudit(c, "scale_in", clusterID, result, audit.AuditDetails{"node_id": nodeID})
	c.JSON(http.StatusOK, ScaleSubmitResponse{Data: result})
}

// GetScaleOperation 处理 GET /api/v1/clusters/:id/scale-operations/:opId，返回操作详情与步骤。
// GetScaleOperation handles GET /api/v1/clusters/:id/scale-operations/:opId and returns details and steps.
// @Summary 查询伸缩操作 / Get a scaling operation
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Param opId path int true "伸缩操作ID"
// @Success 200 {object} ScaleOperationResponse
// @Router /api/v1/clusters/{id}/scale-operations/{opId} [get]
func (h *ScaleHandler) GetScaleOperation(c *gin.Context) {
	clusterID, ok := scaleUintParam(c, "id")
	if !ok {
		return
	}
	operationID, ok := scaleUintParam(c, "opId")
	if !ok {
		return
	}
	info, err := h.service.GetOperation(c.Request.Context(), scaleActorFromGin(c), clusterID, operationID)
	if err != nil {
		writeScaleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ScaleOperationResponse{Data: info})
}

// ListScaleOperations 处理 GET /api/v1/clusters/:id/scale-operations，可按 status 过滤（页面刷新后恢复进行中操作）。
// ListScaleOperations handles GET /api/v1/clusters/:id/scale-operations, optionally filtered by status (used to
// resume running operations after a page refresh).
// @Summary 列出伸缩操作 / List scaling operations
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Param status query string false "running / succeeded / failed"
// @Param limit query int false "最多返回条数（默认 20，最大 100）"
// @Success 200 {object} ScaleOperationListResponse
// @Router /api/v1/clusters/{id}/scale-operations [get]
func (h *ScaleHandler) ListScaleOperations(c *gin.Context) {
	clusterID, ok := scaleUintParam(c, "id")
	if !ok {
		return
	}
	status := ScaleOperationStatus(strings.TrimSpace(c.Query("status")))
	switch status {
	case "", ScaleOperationRunning, ScaleOperationSucceeded, ScaleOperationFailed:
	default:
		writeScaleError(c, errors.Join(ErrScaleInvalidRequest, errors.New("status must be running, succeeded or failed")))
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	items, err := h.service.ListOperations(c.Request.Context(), scaleActorFromGin(c), clusterID, status, limit)
	if err != nil {
		writeScaleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ScaleOperationListResponse{Data: &ScaleOperationListData{Items: items}})
}

// RetryScaleOperation 处理 POST /api/v1/clusters/:id/scale-operations/:opId/retry，从失败步骤重试（R1）。
// RetryScaleOperation handles POST /api/v1/clusters/:id/scale-operations/:opId/retry and retries from the failed
// step (R1).
// @Summary 重试伸缩操作 / Retry a scaling operation
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Param opId path int true "伸缩操作ID"
// @Param lang query string false "zh / en"
// @Param Idempotency-Key header string true "幂等键"
// @Param X-STX-Confirm header string true "显式确认（true）"
// @Success 200 {object} ScaleSubmitResponse
// @Router /api/v1/clusters/{id}/scale-operations/{opId}/retry [post]
func (h *ScaleHandler) RetryScaleOperation(c *gin.Context) {
	clusterID, ok := scaleUintParam(c, "id")
	if !ok {
		return
	}
	operationID, ok := scaleUintParam(c, "opId")
	if !ok {
		return
	}
	meta, err := scaleExecutionMeta(c, struct {
		ClusterID   uint `json:"cluster_id"`
		OperationID uint `json:"operation_id"`
	}{ClusterID: clusterID, OperationID: operationID})
	if err != nil {
		writeScaleError(c, err)
		return
	}
	result, err := h.service.RetryOperation(c.Request.Context(), scaleActorFromGin(c), clusterID, operationID, meta)
	if err != nil {
		writeScaleError(c, err)
		return
	}
	h.recordAudit(c, "scale_retry", clusterID, result, nil)
	c.JSON(http.StatusOK, ScaleSubmitResponse{Data: result})
}

// recordAudit 记录业务审计（集群名、执行编号、伸缩操作编号）。
// recordAudit records the business audit (cluster name, execution ID, scaling operation ID).
func (h *ScaleHandler) recordAudit(c *gin.Context, action string, clusterID uint, result *ScaleSubmitResult, extra audit.AuditDetails) {
	if h.auditRepo == nil || result == nil {
		return
	}
	details := audit.AuditDetails{"trigger": "manual", "execution_id": result.ExecutionID, "scale_operation_id": result.OperationID}
	for key, value := range extra {
		details[key] = value
	}
	clusterName := ""
	if cluster, err := h.service.svc.Get(c.Request.Context(), clusterID); err == nil && cluster != nil {
		clusterName = cluster.Name
	}
	if err := audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		action, "cluster", audit.UintID(clusterID), clusterName, details); err != nil {
		logger.WarnF(c.Request.Context(), "[ClusterScale] 记录审计失败 / Failed to record audit: action=%s, err=%v", action, err)
	}
}

// scaleExecutionMeta 从请求头读取公共执行元数据并计算请求摘要。
// scaleExecutionMeta reads shared-execution metadata from headers and hashes the request.
func scaleExecutionMeta(c *gin.Context, request any) (ScaleExecutionRequest, error) {
	metadata := executionapp.MetadataFromGin(c)
	hash, err := executionapp.HashRequest(request)
	if err != nil {
		return ScaleExecutionRequest{}, err
	}
	return ScaleExecutionRequest{
		RequestID:      metadata.RequestID,
		IdempotencyKey: metadata.IdempotencyKey,
		RequestHash:    hash,
		Confirmed:      metadata.Confirmed,
		ConfirmationID: metadata.ConfirmationID,
		ClientType:     metadata.ClientType,
		Lang:           c.Query("lang"),
		Username:       auth.GetUsernameFromContext(c),
	}, nil
}

// scaleActorFromGin 读取登录用户作为执行归属。
// scaleActorFromGin reads the logged-in user as the execution owner.
func scaleActorFromGin(c *gin.Context) executionapp.Actor {
	user := auth.GetUserFromContext(c)
	if user == nil {
		return executionapp.Actor{}
	}
	return executionapp.Actor{UserID: user.ID, IsAdmin: user.IsAdmin}
}

// scaleUintParam 解析路径中的正整数参数，失败时直接写入 400。
// scaleUintParam parses a positive integer path parameter and writes 400 on failure.
func scaleUintParam(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, scaleErrorResponse{ErrorCode: ErrorCodeScaleInvalidRequest, ErrorMsg: "invalid " + name})
		return 0, false
	}
	return uint(value), true
}

// writeScaleError 统一写出伸缩接口错误：公共执行错误（确认、幂等、权限）沿用 execution.WriteError，其余按领域错误映射。
// writeScaleError writes scaling endpoint errors: shared execution errors (confirmation, idempotency, permission)
// reuse execution.WriteError and the rest are mapped from domain errors.
func writeScaleError(c *gin.Context, err error) {
	var confirmationErr *executionapp.ConfirmationRequiredError
	if errors.As(err, &confirmationErr) || isScaleExecutionError(err) {
		executionapp.WriteError(c, err)
		return
	}
	status, code := scaleErrorStatus(err)
	response := scaleErrorResponse{ErrorCode: code, ErrorMsg: err.Error()}
	// 并发冲突时把进行中操作的编号、类型与发起人放进 data，看不到该操作的用户也能知道原因。
	// On a conflict, put the running operation's ID, kind and initiator in data so users who cannot see it still
	// learn why.
	var inProgress *ScaleInProgressError
	if errors.As(err, &inProgress) {
		response.Data = inProgress
	}
	c.JSON(status, response)
}

func isScaleExecutionError(err error) bool {
	for _, target := range []error{
		executionapp.ErrExecutionNotFound,
		executionapp.ErrPermissionDenied,
		executionapp.ErrAdminRequired,
		executionapp.ErrIdempotencyConflict,
		executionapp.ErrIdempotencyKeyMissing,
		executionapp.ErrExplicitConfirmNeeded,
		executionapp.ErrConfirmationInvalid,
		executionapp.ErrConcurrentUpdate,
		executionapp.ErrInvalidTransition,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// scaleErrorStatus 把伸缩领域错误映射为 HTTP 状态码与稳定错误编码。
// scaleErrorStatus maps scaling domain errors to HTTP status codes and stable error codes.
func scaleErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrClusterScaleInProgress):
		return http.StatusConflict, ErrorCodeClusterScaleInProgress
	case errors.Is(err, ErrScaleOperationNotFound):
		return http.StatusNotFound, ErrorCodeScaleOperationNotFound
	case errors.Is(err, ErrScaleOperationNotRetryable):
		return http.StatusConflict, ErrorCodeScaleNotRetryable
	case errors.Is(err, ErrScalePlanChanged):
		return http.StatusConflict, ErrorCodeScalePlanChanged
	case errors.Is(err, ErrScalePlanBlocked):
		return http.StatusConflict, ErrorCodeScalePlanBlocked
	case errors.Is(err, ErrScaleRestartAckRequired):
		return http.StatusBadRequest, ErrorCodeScaleRestartAckRequired
	case errors.Is(err, ErrScaleJobsRiskAckRequired):
		return http.StatusBadRequest, ErrorCodeScaleJobsAckRequired
	case errors.Is(err, ErrScaleRollingRestartUnavailable):
		return http.StatusBadRequest, ErrorCodeScaleRollingRestartUnavailable
	case errors.Is(err, ErrScaleInvalidRequest),
		errors.Is(err, ErrInvalidNodeRole),
		errors.Is(err, ErrInvalidHazelcastPort),
		errors.Is(err, ErrInvalidAPIPort),
		errors.Is(err, ErrInvalidWorkerPort):
		return http.StatusBadRequest, ErrorCodeScaleInvalidRequest
	case errors.Is(err, ErrClusterNotFound):
		return http.StatusNotFound, "cluster_not_found"
	case errors.Is(err, ErrNodeNotFound):
		return http.StatusNotFound, "node_not_found"
	case errors.Is(err, ErrScaleNotConfigured):
		return http.StatusServiceUnavailable, "scale_not_configured"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
