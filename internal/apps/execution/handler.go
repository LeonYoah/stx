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

package execution

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
)

const maxWaitSeconds = 30

// Handler 提供公共执行查询、等待和取消接口。
// Handler exposes shared execution get, wait, and cancel endpoints.
type Handler struct {
	service *Service
}

// NewHandler 创建公共执行 HTTP 处理器。
// NewHandler creates the shared execution HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Response 是公共执行接口的统一响应。
// Response is the common response envelope for execution endpoints.
type Response struct {
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`
	Data      any    `json:"data,omitempty"`
}

// WaitData 返回最新执行记录和本次等待是否超时。
// WaitData returns the latest execution record and whether this wait timed out.
type WaitData struct {
	Execution *Execution `json:"execution"`
	TimedOut  bool       `json:"wait_timed_out"`
}

// ConfirmationData 返回危险操作的影响说明和一次性确认编号。
// ConfirmationData returns impact information and a one-time confirmation ID for a risky operation.
type ConfirmationData struct {
	ConfirmationRequired bool      `json:"confirmation_required"`
	ConfirmationID       string    `json:"confirmation_id"`
	RiskLevel            RiskLevel `json:"risk_level"`
	Impact               string    `json:"impact"`
	ExpiresAt            time.Time `json:"expires_at"`
}

// Get 读取当前用户有权查看的执行记录。
// Get reads an execution visible to the current user.
// @Tags execution
// @Produce json
// @Param id path string true "公共执行编号"
// @Success 200 {object} Response
// @Router /api/v1/executions/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	item, err := h.service.Get(c.Request.Context(), actorFromGin(c), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, Response{Data: item})
}

// Wait 等待执行结束，最长等待三十秒。
// Wait waits for completion for at most thirty seconds.
// @Tags execution
// @Produce json
// @Param id path string true "公共执行编号"
// @Param timeout_seconds query int false "服务端单次等待秒数，范围为 1 到 30"
// @Success 200 {object} Response
// @Router /api/v1/executions/{id}/wait [get]
func (h *Handler) Wait(c *gin.Context) {
	seconds := maxWaitSeconds
	if raw := strings.TrimSpace(c.Query("timeout_seconds")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxWaitSeconds {
			c.JSON(http.StatusBadRequest, Response{ErrorCode: "invalid_wait_timeout", ErrorMsg: "timeout_seconds must be between 1 and 30"})
			return
		}
		seconds = value
	}
	item, timedOut, err := h.service.Wait(c.Request.Context(), actorFromGin(c), c.Param("id"), time.Duration(seconds)*time.Second)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, Response{Data: WaitData{Execution: item, TimedOut: timedOut}})
}

// Cancel 请求取消执行；接口只在业务模块确认停止后返回 cancelled。
// Cancel requests cancellation and returns cancelled only after the business module confirms the stop.
// @Tags execution
// @Produce json
// @Param id path string true "公共执行编号"
// @Param Idempotency-Key header string true "幂等键"
// @Param X-STX-Confirm header bool true "确认取消影响"
// @Success 200 {object} Response
// @Router /api/v1/executions/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	metadata := MetadataFromGin(c)
	requestHash := HashString(strings.TrimSpace(c.Param("id")))
	if err := h.service.Authorize(c.Request.Context(), actorFromGin(c), AuthorizationInput{
		OperationID:    "execution.cancel",
		RiskLevel:      RiskLevelR1,
		Impact:         "取消会阻止任务继续执行；已经开始的安全步骤可能需要等待当前操作返回。",
		IdempotencyKey: metadata.IdempotencyKey,
		RequestHash:    requestHash,
		Confirmed:      metadata.Confirmed,
	}); err != nil {
		h.writeError(c, err)
		return
	}
	item, err := h.service.Cancel(c.Request.Context(), actorFromGin(c), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, Response{Data: item})
}

func (h *Handler) writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	switch {
	case errors.Is(err, ErrExecutionNotFound):
		status, code = http.StatusNotFound, ErrorCodeNotFound
	case errors.Is(err, ErrPermissionDenied), errors.Is(err, ErrAdminRequired):
		status, code = http.StatusForbidden, ErrorCodePermissionDenied
	case errors.Is(err, ErrIdempotencyConflict), errors.Is(err, ErrConcurrentUpdate), errors.Is(err, ErrInvalidTransition):
		status, code = http.StatusConflict, ErrorCodeIdempotencyConflict
	case errors.Is(err, ErrIdempotencyKeyMissing):
		status, code = http.StatusBadRequest, ErrorCodeIdempotencyRequired
	case errors.Is(err, ErrExplicitConfirmNeeded):
		status, code = http.StatusPreconditionRequired, ErrorCodeConfirmationRequired
	case errors.Is(err, ErrConfirmationInvalid):
		status, code = http.StatusConflict, ErrorCodeConfirmationInvalid
	case errors.Is(err, ErrNotCancellable), errors.Is(err, ErrProviderNotRegistered):
		status, code = http.StatusConflict, ErrorCodeNotCancellable
	}

	var confirmationErr *ConfirmationRequiredError
	if errors.As(err, &confirmationErr) {
		c.JSON(http.StatusPreconditionRequired, Response{
			ErrorCode: ErrorCodeConfirmationRequired,
			ErrorMsg:  confirmationErr.Error(),
			Data: ConfirmationData{
				ConfirmationRequired: true,
				ConfirmationID:       confirmationErr.ConfirmationID,
				RiskLevel:            confirmationErr.RiskLevel,
				Impact:               confirmationErr.Impact,
				ExpiresAt:            confirmationErr.ExpiresAt,
			},
		})
		return
	}
	c.JSON(status, Response{ErrorCode: code, ErrorMsg: err.Error()})
}

func actorFromGin(c *gin.Context) Actor {
	user := auth.GetUserFromContext(c)
	if user == nil {
		return Actor{}
	}
	return Actor{UserID: user.ID, IsAdmin: user.IsAdmin}
}
