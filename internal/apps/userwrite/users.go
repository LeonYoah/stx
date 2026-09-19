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
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/admin"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

// CreateUser 创建用户，密码只用于业务写入和不可逆请求摘要。
// CreateUser creates a user and uses the password only for the business write and irreversible request digest.
func (h *Handler) CreateUser(c *gin.Context) {
	var req admin.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, admin.CreateUserResponse{ErrorMsg: err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Nickname = strings.TrimSpace(req.Nickname)
	req.Email = strings.TrimSpace(req.Email)
	requestHash, err := executionapp.HashRequest(struct {
		Username       string `json:"username"`
		Nickname       string `json:"nickname,omitempty"`
		Email          string `json:"email,omitempty"`
		IsAdmin        bool   `json:"is_admin"`
		PasswordSHA256 string `json:"password_sha256"`
	}{req.Username, req.Nickname, req.Email, req.IsAdmin, executionapp.HashString(req.Password)})
	if err != nil {
		c.JSON(http.StatusInternalServerError, admin.CreateUserResponse{ErrorMsg: err.Error()})
		return
	}
	item, existing, beginErr := h.begin(c, "admin.user.create", adminUserExecutionModule, req.Username, executionapp.RiskLevelR1,
		"创建用户会新增一个可登录 STX 的账号，管理员可以后续停用或删除。", requestHash)
	if beginErr != nil {
		executionapp.WriteError(c, beginErr)
		return
	}
	if existing {
		h.writeExistingUser(c, item, true)
		return
	}
	if _, err := auth.FindByUsername(db.DB(c.Request.Context()), req.Username); err == nil {
		runErr := fmt.Errorf("用户名已存在")
		h.finish(c, item, "", runErr)
		c.JSON(http.StatusBadRequest, admin.CreateUserResponse{ErrorMsg: runErr.Error()})
		return
	}
	user := &auth.User{Username: req.Username, Nickname: req.Nickname, Email: req.Email, IsActive: true, IsAdmin: req.IsAdmin}
	if err := user.SetPassword(req.Password, config.GetAuthConfig().BcryptCost); err != nil {
		h.finish(c, item, "", err)
		c.JSON(http.StatusBadRequest, admin.CreateUserResponse{ErrorMsg: err.Error()})
		return
	}
	if err := user.Create(db.DB(c.Request.Context())); err != nil {
		h.finish(c, item, "", err)
		c.JSON(http.StatusInternalServerError, admin.CreateUserResponse{ErrorMsg: err.Error()})
		return
	}
	resultRef := strconv.FormatUint(user.ID, 10)
	h.finish(c, item, resultRef, nil)
	h.recordAudit(c, "create", resultRef, user.Username)
	logger.InfoF(c.Request.Context(), "[Admin] 创建用户成功: %s", user.Username)
	c.JSON(http.StatusOK, admin.CreateUserResponse{Data: user.ToUserInfo()})
}

// UpdateUser 更新用户字段，布尔字段通过指针保留“未提供”和 false 的差异。
// UpdateUser updates user fields while pointer booleans preserve the difference between omitted and false.
func (h *Handler) UpdateUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, admin.UpdateUserResponse{ErrorMsg: "无效的用户 ID"})
		return
	}
	var req admin.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, admin.UpdateUserResponse{ErrorMsg: err.Error()})
		return
	}
	passwordSHA256 := ""
	if req.Password != "" {
		passwordSHA256 = executionapp.HashString(req.Password)
	}
	requestHash, err := executionapp.HashRequest(struct {
		UserID         uint64  `json:"user_id"`
		Nickname       string  `json:"nickname,omitempty"`
		Email          *string `json:"email,omitempty"`
		IsActive       *bool   `json:"is_active,omitempty"`
		IsAdmin        *bool   `json:"is_admin,omitempty"`
		PasswordSHA256 string  `json:"password_sha256,omitempty"`
	}{userID, strings.TrimSpace(req.Nickname), req.Email, req.IsActive, req.IsAdmin, passwordSHA256})
	if err != nil {
		c.JSON(http.StatusInternalServerError, admin.UpdateUserResponse{ErrorMsg: err.Error()})
		return
	}
	item, existing, beginErr := h.begin(c, "admin.user.update", adminUserExecutionModule, strconv.FormatUint(userID, 10), executionapp.RiskLevelR1,
		"更新用户可能改变账号状态、管理员权限或登录密码。", requestHash)
	if beginErr != nil {
		executionapp.WriteError(c, beginErr)
		return
	}
	if existing {
		h.writeExistingUser(c, item, false)
		return
	}
	user, err := auth.FindByID(db.DB(c.Request.Context()), userID)
	if err != nil {
		h.finish(c, item, "", err)
		c.JSON(http.StatusNotFound, admin.UpdateUserResponse{ErrorMsg: "用户不存在"})
		return
	}
	updates := make(map[string]interface{})
	if req.Nickname != "" {
		updates["nickname"] = strings.TrimSpace(req.Nickname)
	}
	if req.Email != nil {
		updates["email"] = strings.TrimSpace(*req.Email)
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if req.IsAdmin != nil {
		updates["is_admin"] = *req.IsAdmin
	}
	if req.Password != "" {
		if err := user.SetPassword(req.Password, config.GetAuthConfig().BcryptCost); err != nil {
			h.finish(c, item, "", err)
			c.JSON(http.StatusBadRequest, admin.UpdateUserResponse{ErrorMsg: err.Error()})
			return
		}
		updates["password_hash"] = user.PasswordHash
	}
	if len(updates) > 0 {
		if err := db.DB(c.Request.Context()).Model(user).Updates(updates).Error; err != nil {
			h.finish(c, item, "", err)
			c.JSON(http.StatusInternalServerError, admin.UpdateUserResponse{ErrorMsg: err.Error()})
			return
		}
	}
	user, err = auth.FindByID(db.DB(c.Request.Context()), userID)
	if err != nil {
		h.finish(c, item, "", err)
		c.JSON(http.StatusInternalServerError, admin.UpdateUserResponse{ErrorMsg: err.Error()})
		return
	}
	resultRef := strconv.FormatUint(userID, 10)
	h.finish(c, item, resultRef, nil)
	h.recordAudit(c, "update", resultRef, user.Username)
	logger.InfoF(c.Request.Context(), "[Admin] 更新用户成功: %s", user.Username)
	c.JSON(http.StatusOK, admin.UpdateUserResponse{Data: user.ToUserInfo()})
}

// DeleteUser 删除非当前用户，CLI/API 调用必须先取得一次性确认编号。
// DeleteUser deletes a user other than the current actor and requires one-time confirmation for CLI/API calls.
func (h *Handler) DeleteUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, admin.DeleteUserResponse{ErrorMsg: "无效的用户 ID"})
		return
	}
	if userID == auth.GetUserIDFromContext(c) {
		c.JSON(http.StatusBadRequest, admin.DeleteUserResponse{ErrorMsg: "不能删除当前登录用户"})
		return
	}
	requestHash, err := executionapp.HashRequest(struct {
		UserID uint64 `json:"user_id"`
	}{UserID: userID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, admin.DeleteUserResponse{ErrorMsg: err.Error()})
		return
	}
	item, existing, beginErr := h.begin(c, "admin.user.delete", adminUserExecutionModule, strconv.FormatUint(userID, 10), executionapp.RiskLevelR2,
		"删除用户后无法通过 STX 恢复，该用户的登录令牌和应用内关联记录可能失效。", requestHash)
	if beginErr != nil {
		executionapp.WriteError(c, beginErr)
		return
	}
	if existing {
		if item == nil || item.Status != executionapp.StatusSucceeded {
			status := executionapp.Status("")
			if item != nil {
				status = item.Status
			}
			executionapp.WriteError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
			return
		}
		c.JSON(http.StatusOK, admin.DeleteUserResponse{})
		return
	}
	user, err := auth.FindByID(db.DB(c.Request.Context()), userID)
	if err != nil {
		h.finish(c, item, "", err)
		c.JSON(http.StatusNotFound, admin.DeleteUserResponse{ErrorMsg: "用户不存在"})
		return
	}
	if err := db.DB(c.Request.Context()).Delete(user).Error; err != nil {
		h.finish(c, item, "", err)
		c.JSON(http.StatusInternalServerError, admin.DeleteUserResponse{ErrorMsg: err.Error()})
		return
	}
	resultRef := strconv.FormatUint(userID, 10)
	h.finish(c, item, resultRef, nil)
	h.recordAudit(c, "delete", resultRef, user.Username)
	logger.InfoF(c.Request.Context(), "[Admin] 删除用户成功: %s", user.Username)
	c.JSON(http.StatusOK, admin.DeleteUserResponse{})
}
