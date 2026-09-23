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

// Package userwrite 为个人资料和管理员用户写接口接入公共执行协议。
// Package userwrite connects profile and administrator user writes to the shared execution contract.
package userwrite

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/admin"
	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const adminUserExecutionModule = "admin_user"

// Handler 保存用户写接口需要的公共执行和审计依赖。
// Handler stores shared execution and audit dependencies for user write endpoints.
type Handler struct {
	executionService *executionapp.Service
	auditRepo        *audit.Repository
}

// NewHandler 创建用户写处理器。
// NewHandler creates a user write handler.
func NewHandler(executionService *executionapp.Service, auditRepo *audit.Repository) *Handler {
	return &Handler{executionService: executionService, auditRepo: auditRepo}
}

func (h *Handler) begin(c *gin.Context, operationID, module, moduleRef string, risk executionapp.RiskLevel, impact, requestHash string) (*executionapp.Execution, bool, error) {
	metadata := executionapp.MetadataFromGin(c)
	if h.executionService == nil || metadata.ClientType == "web" {
		return nil, false, nil
	}
	user := auth.GetUserFromContext(c)
	actor := executionapp.Actor{UserID: auth.GetUserIDFromContext(c)}
	if user != nil {
		actor.IsAdmin = user.IsAdmin
	}
	return h.executionService.BeginSynchronous(c.Request.Context(), actor, executionapp.SynchronousInput{
		OperationID: operationID, Module: module, ModuleRef: moduleRef,
		RequestID: metadata.RequestID, IdempotencyKey: metadata.IdempotencyKey, RequestHash: requestHash,
		RiskLevel: risk, Impact: impact, Confirmed: metadata.Confirmed,
		ConfirmationID: metadata.ConfirmationID, ClientType: metadata.ClientType,
	})
}

func (h *Handler) finish(c *gin.Context, item *executionapp.Execution, resultRef string, runErr error) {
	if h.executionService == nil || item == nil {
		return
	}
	if err := h.executionService.FinishSynchronous(c.Request.Context(), item, resultRef, runErr); err != nil {
		logger.WarnF(c.Request.Context(), "[UserWrite] 更新公共执行记录失败: execution_id=%s err=%v", item.ExecutionID, err)
	}
}

func (h *Handler) writeExistingUser(c *gin.Context, item *executionapp.Execution, create bool) {
	if item == nil || item.Status != executionapp.StatusSucceeded {
		status := executionapp.Status("")
		if item != nil {
			status = item.Status
		}
		executionapp.WriteError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
		return
	}
	userID, err := strconv.ParseUint(strings.TrimSpace(item.ResultRef), 10, 64)
	if err != nil {
		executionapp.WriteError(c, executionapp.ErrConcurrentUpdate)
		return
	}
	user, err := auth.FindByID(db.DB(c.Request.Context()), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error_msg": "用户不存在", "data": nil})
		return
	}
	if create {
		c.JSON(http.StatusOK, admin.CreateUserResponse{Data: user.ToUserInfo()})
		return
	}
	c.JSON(http.StatusOK, admin.UpdateUserResponse{Data: user.ToUserInfo()})
}

func (h *Handler) recordAudit(c *gin.Context, action, resourceID, resourceName string) {
	if h.auditRepo == nil {
		return
	}
	_ = audit.RecordFromGin(c, h.auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		action, "user", resourceID, resourceName, audit.AuditDetails{"trigger": "manual"})
}
