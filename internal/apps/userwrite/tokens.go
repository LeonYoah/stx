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

package userwrite

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/admin"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const adminTokenExecutionModule = "cli_token"

// CreateCLIToken 由管理员签发带名称的 CLI 令牌，原文只在首次成功时返回。
// CreateCLIToken issues a named CLI token for an administrator and returns plaintext only on first success.
// @Tags admin
// @Accept json
// @Produce json
// @Param request body admin.CreateCLITokenRequest true "签发 CLI 令牌请求"
// @Success 200 {object} admin.CreateCLITokenResponse
// @Router /api/v1/admin/cli-tokens [post]
func (h *Handler) CreateCLIToken(c *gin.Context) {
	var req admin.CreateCLITokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, admin.CreateCLITokenResponse{ErrorMsg: err.Error()})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.ExpiresIn = strings.TrimSpace(req.ExpiresIn)
	if req.ExpiresIn == "" {
		req.ExpiresIn = "never"
	}
	requestHash, err := executionapp.HashRequest(struct {
		UserID    uint64 `json:"user_id"`
		Name      string `json:"name"`
		ExpiresIn string `json:"expires_in"`
	}{req.UserID, req.Name, req.ExpiresIn})
	if err != nil {
		c.JSON(http.StatusInternalServerError, admin.CreateCLITokenResponse{ErrorMsg: err.Error()})
		return
	}
	item, existing, beginErr := h.begin(c, "admin.token.create", adminTokenExecutionModule, req.Name, executionapp.RiskLevelR1,
		"签发 CLI 令牌后，持有者可按该用户权限调用 STX API，原文只显示一次。", requestHash)
	if beginErr != nil {
		executionapp.WriteError(c, beginErr)
		return
	}
	if existing {
		h.writeExistingCLIToken(c, item)
		return
	}

	database := db.DB(c.Request.Context())
	user, err := auth.FindByID(database, req.UserID)
	if err != nil {
		runErr := fmt.Errorf("用户不存在")
		h.finish(c, item, "", runErr)
		c.JSON(http.StatusBadRequest, admin.CreateCLITokenResponse{ErrorMsg: runErr.Error()})
		return
	}
	if !user.IsActive {
		h.finish(c, item, "", auth.ErrUserInactive)
		c.JSON(http.StatusBadRequest, admin.CreateCLITokenResponse{ErrorMsg: auth.ErrUserInactive.Error()})
		return
	}

	now := time.Now()
	issue, err := auth.IssueNamedCLIToken(database, user, req.Name, req.ExpiresIn, now)
	if err != nil {
		h.finish(c, item, "", err)
		status := http.StatusBadRequest
		if !errors.Is(err, auth.ErrCLITokenNameRequired) && !errors.Is(err, auth.ErrCLITokenNameTooLong) && !errors.Is(err, auth.ErrInvalidCLITokenTTL) && !errors.Is(err, auth.ErrUserInactive) {
			status = http.StatusInternalServerError
		}
		c.JSON(status, admin.CreateCLITokenResponse{ErrorMsg: err.Error()})
		return
	}

	resultRef := strconv.FormatUint(issue.ID, 10)
	h.finish(c, item, resultRef, nil)
	h.recordAuditResource(c, "create", "cli_token", resultRef, issue.Name)
	logger.InfoF(c.Request.Context(), "[Admin] 签发 CLI 令牌成功: token_id=%d user_id=%d prefix=%s", issue.ID, user.ID, issue.Prefix)
	c.JSON(http.StatusOK, admin.CreateCLITokenResponse{Data: admin.NewCreateCLITokenResult(issue, user, now)})
}

// RevokeCLIToken 由管理员吊销指定 CLI 令牌。
// RevokeCLIToken lets an administrator revoke a CLI token.
// @Tags admin
// @Produce json
// @Param id path int true "Token ID"
// @Success 200 {object} admin.RevokeCLITokenResponse
// @Router /api/v1/admin/cli-tokens/{id}/revoke [post]
func (h *Handler) RevokeCLIToken(c *gin.Context) {
	tokenID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || tokenID == 0 {
		c.JSON(http.StatusBadRequest, admin.RevokeCLITokenResponse{ErrorMsg: "无效的令牌 ID"})
		return
	}
	requestHash, err := executionapp.HashRequest(struct {
		TokenID uint64 `json:"token_id"`
	}{tokenID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, admin.RevokeCLITokenResponse{ErrorMsg: err.Error()})
		return
	}
	item, existing, beginErr := h.begin(c, "admin.token.revoke", adminTokenExecutionModule, strconv.FormatUint(tokenID, 10), executionapp.RiskLevelR1,
		"吊销 CLI 令牌后，使用该令牌的登录和脚本会立即失效。", requestHash)
	if beginErr != nil {
		executionapp.WriteError(c, beginErr)
		return
	}
	if existing {
		h.writeExistingRevokedCLIToken(c, item)
		return
	}

	now := time.Now()
	token, err := auth.RevokeCLITokenByID(db.DB(c.Request.Context()), tokenID, now)
	if err != nil {
		h.finish(c, item, "", err)
		status := http.StatusInternalServerError
		if errors.Is(err, auth.ErrCLITokenNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, admin.RevokeCLITokenResponse{ErrorMsg: err.Error()})
		return
	}

	username := ""
	if owner, findErr := auth.FindByID(db.DB(c.Request.Context()), token.UserID); findErr == nil {
		username = owner.Username
	}
	info := token.ToInfo(username, now)
	resultRef := strconv.FormatUint(token.ID, 10)
	h.finish(c, item, resultRef, nil)
	h.recordAuditResource(c, "revoke", "cli_token", resultRef, token.Name)
	logger.InfoF(c.Request.Context(), "[Admin] 吊销 CLI 令牌成功: token_id=%d prefix=%s", token.ID, token.TokenPrefix)
	c.JSON(http.StatusOK, admin.RevokeCLITokenResponse{Data: &info})
}

func (h *Handler) writeExistingCLIToken(c *gin.Context, item *executionapp.Execution) {
	if item == nil || item.Status != executionapp.StatusSucceeded {
		status := executionapp.Status("")
		if item != nil {
			status = item.Status
		}
		executionapp.WriteError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
		return
	}
	tokenID, err := strconv.ParseUint(strings.TrimSpace(item.ResultRef), 10, 64)
	if err != nil {
		executionapp.WriteError(c, executionapp.ErrConcurrentUpdate)
		return
	}
	h.writeCLITokenWithoutPlaintext(c, tokenID)
}

func (h *Handler) writeExistingRevokedCLIToken(c *gin.Context, item *executionapp.Execution) {
	if item == nil || item.Status != executionapp.StatusSucceeded {
		status := executionapp.Status("")
		if item != nil {
			status = item.Status
		}
		executionapp.WriteError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
		return
	}
	tokenID, err := strconv.ParseUint(strings.TrimSpace(item.ResultRef), 10, 64)
	if err != nil {
		executionapp.WriteError(c, executionapp.ErrConcurrentUpdate)
		return
	}
	token, err := auth.FindCLITokenByID(db.DB(c.Request.Context()), tokenID)
	if err != nil {
		c.JSON(http.StatusNotFound, admin.RevokeCLITokenResponse{ErrorMsg: err.Error()})
		return
	}
	username := ""
	if owner, findErr := auth.FindByID(db.DB(c.Request.Context()), token.UserID); findErr == nil {
		username = owner.Username
	}
	info := token.ToInfo(username, time.Now())
	c.JSON(http.StatusOK, admin.RevokeCLITokenResponse{Data: &info})
}

func (h *Handler) writeCLITokenWithoutPlaintext(c *gin.Context, tokenID uint64) {
	token, err := auth.FindCLITokenByID(db.DB(c.Request.Context()), tokenID)
	if err != nil {
		c.JSON(http.StatusNotFound, admin.CreateCLITokenResponse{ErrorMsg: err.Error()})
		return
	}
	username := ""
	if owner, findErr := auth.FindByID(db.DB(c.Request.Context()), token.UserID); findErr == nil {
		username = owner.Username
	}
	info := token.ToInfo(username, time.Now())
	c.JSON(http.StatusOK, admin.CreateCLITokenResponse{Data: &admin.CreateCLITokenResult{CLITokenInfo: info}})
}
