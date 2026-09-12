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

// Package admin 提供管理员后台功能
package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

// ==================== 用户列表 ====================

// ListUsersRequest 用户列表请求
type ListUsersRequest struct {
	Current  int    `json:"current" form:"current" binding:"min=1"`
	Size     int    `json:"size" form:"size" binding:"min=1,max=100"`
	Username string `json:"username" form:"username"`
	IsActive *bool  `json:"is_active" form:"is_active"`
	IsAdmin  *bool  `json:"is_admin" form:"is_admin"`
}

// ListUsersResponse 用户列表响应
type ListUsersResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *struct {
		Total int64            `json:"total"`
		Users []*auth.UserInfo `json:"users"`
	} `json:"data"`
}

// ListUsersHandler 获取用户列表
// @Tags admin
// @Param request query ListUsersRequest true "request query"
// @Produce json
// @Success 200 {object} ListUsersResponse
// @Router /api/v1/admin/users [get]
func ListUsersHandler(c *gin.Context) {
	req := &ListUsersRequest{}
	if err := c.ShouldBindQuery(req); err != nil {
		c.JSON(http.StatusBadRequest, ListUsersResponse{ErrorMsg: err.Error()})
		return
	}

	offset := (req.Current - 1) * req.Size
	query := db.DB(c.Request.Context()).Model(&auth.User{})

	if req.Username != "" {
		query = query.Where("username LIKE ?", req.Username+"%")
	}
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}
	if req.IsAdmin != nil {
		query = query.Where("is_admin = ?", *req.IsAdmin)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ListUsersResponse{ErrorMsg: err.Error()})
		return
	}

	var users []auth.User
	if err := query.Order("created_at DESC").Offset(offset).Limit(req.Size).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ListUsersResponse{ErrorMsg: err.Error()})
		return
	}

	// 转换为 UserInfo
	userInfos := make([]*auth.UserInfo, len(users))
	for i, u := range users {
		userInfos[i] = u.ToUserInfo()
	}

	c.JSON(http.StatusOK, ListUsersResponse{
		Data: &struct {
			Total int64            `json:"total"`
			Users []*auth.UserInfo `json:"users"`
		}{
			Total: total,
			Users: userInfos,
		},
	})
}

// ==================== 创建用户 ====================

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6,max=100"`
	Nickname string `json:"nickname" binding:"max=100"`
	Email    string `json:"email" binding:"omitempty,max=255,email"`
	IsAdmin  bool   `json:"is_admin"`
}

// CreateUserResponse 创建用户响应
type CreateUserResponse struct {
	ErrorMsg string         `json:"error_msg"`
	Data     *auth.UserInfo `json:"data"`
}

// CreateUserHandler 创建用户
// @Tags admin
// @Accept json
// @Produce json
// @Param request body CreateUserRequest true "创建用户请求"
// @Success 200 {object} CreateUserResponse
// @Router /api/v1/admin/users [post]
func CreateUserHandler(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CreateUserResponse{ErrorMsg: err.Error()})
		return
	}

	// 检查用户名是否已存在
	_, err := auth.FindByUsername(db.DB(c.Request.Context()), req.Username)
	if err == nil {
		c.JSON(http.StatusBadRequest, CreateUserResponse{ErrorMsg: "用户名已存在"})
		return
	}

	// 创建用户
	user := &auth.User{
		Username: req.Username,
		Nickname: strings.TrimSpace(req.Nickname),
		Email:    strings.TrimSpace(req.Email),
		IsActive: true,
		IsAdmin:  req.IsAdmin,
	}

	// 设置密码
	if err := user.SetPassword(req.Password, config.GetAuthConfig().BcryptCost); err != nil {
		c.JSON(http.StatusBadRequest, CreateUserResponse{ErrorMsg: err.Error()})
		return
	}

	// 保存到数据库
	if err := user.Create(db.DB(c.Request.Context())); err != nil {
		c.JSON(http.StatusInternalServerError, CreateUserResponse{ErrorMsg: err.Error()})
		return
	}

	auditRepo := audit.NewRepository(db.DB(c.Request.Context()))
	_ = audit.RecordFromGin(c, auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"create", "user", strconv.FormatUint(uint64(user.ID), 10), user.Username, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Admin] 创建用户成功: %s", user.Username)
	c.JSON(http.StatusOK, CreateUserResponse{Data: user.ToUserInfo()})
}

// ==================== 更新用户 ====================

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Nickname string  `json:"nickname" binding:"max=100"`
	Password string  `json:"password" binding:"omitempty,min=6,max=100"`
	Email    *string `json:"email" binding:"omitempty,max=255,email"`
	IsActive *bool   `json:"is_active"`
	IsAdmin  *bool   `json:"is_admin"`
}

// UpdateUserResponse 更新用户响应
type UpdateUserResponse struct {
	ErrorMsg string         `json:"error_msg"`
	Data     *auth.UserInfo `json:"data"`
}

// UpdateUserHandler 更新用户
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "用户ID"
// @Param request body UpdateUserRequest true "更新用户请求"
// @Success 200 {object} UpdateUserResponse
// @Router /api/v1/admin/users/{id} [put]
func UpdateUserHandler(c *gin.Context) {
	// 解析用户 ID
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, UpdateUserResponse{ErrorMsg: "无效的用户 ID"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, UpdateUserResponse{ErrorMsg: err.Error()})
		return
	}

	// 查找用户
	user, err := auth.FindByID(db.DB(c.Request.Context()), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, UpdateUserResponse{ErrorMsg: "用户不存在"})
		return
	}

	// 更新字段
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

	// 更新密码
	if req.Password != "" {
		if err := user.SetPassword(req.Password, config.GetAuthConfig().BcryptCost); err != nil {
			c.JSON(http.StatusBadRequest, UpdateUserResponse{ErrorMsg: err.Error()})
			return
		}
		updates["password_hash"] = user.PasswordHash
	}

	// 保存更新
	if len(updates) > 0 {
		if err := db.DB(c.Request.Context()).Model(user).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, UpdateUserResponse{ErrorMsg: err.Error()})
			return
		}
	}

	// 重新查询用户信息
	user, _ = auth.FindByID(db.DB(c.Request.Context()), userID)

	auditRepo := audit.NewRepository(db.DB(c.Request.Context()))
	_ = audit.RecordFromGin(c, auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"update", "user", strconv.FormatUint(userID, 10), user.Username, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Admin] 更新用户成功: %s", user.Username)
	c.JSON(http.StatusOK, UpdateUserResponse{Data: user.ToUserInfo()})
}

// ==================== 删除用户 ====================

// DeleteUserResponse 删除用户响应
type DeleteUserResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     any    `json:"data"`
}

// DeleteUserHandler 删除用户
// @Tags admin
// @Produce json
// @Param id path int true "用户ID"
// @Success 200 {object} DeleteUserResponse
// @Router /api/v1/admin/users/{id} [delete]
func DeleteUserHandler(c *gin.Context) {
	// 解析用户 ID
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, DeleteUserResponse{ErrorMsg: "无效的用户 ID"})
		return
	}

	// 查找用户
	user, err := auth.FindByID(db.DB(c.Request.Context()), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, DeleteUserResponse{ErrorMsg: "用户不存在"})
		return
	}

	// 不允许删除自己
	currentUserID := auth.GetUserIDFromContext(c)
	if userID == currentUserID {
		c.JSON(http.StatusBadRequest, DeleteUserResponse{ErrorMsg: "不能删除当前登录用户"})
		return
	}

	// 删除用户
	if err := db.DB(c.Request.Context()).Delete(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, DeleteUserResponse{ErrorMsg: err.Error()})
		return
	}

	auditRepo := audit.NewRepository(db.DB(c.Request.Context()))
	_ = audit.RecordFromGin(c, auditRepo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
		"delete", "user", strconv.FormatUint(userID, 10), user.Username, audit.AuditDetails{"trigger": "manual"})
	logger.InfoF(c.Request.Context(), "[Admin] 删除用户成功: %s", user.Username)
	c.JSON(http.StatusOK, DeleteUserResponse{})
}

// ==================== 获取单个用户 ====================

// GetUserResponse 获取用户响应
type GetUserResponse struct {
	ErrorMsg string         `json:"error_msg"`
	Data     *auth.UserInfo `json:"data"`
}

// GetUserHandler 获取单个用户
// @Tags admin
// @Produce json
// @Param id path int true "用户ID"
// @Success 200 {object} GetUserResponse
// @Router /api/v1/admin/users/{id} [get]
func GetUserHandler(c *gin.Context) {
	// 解析用户 ID
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GetUserResponse{ErrorMsg: "无效的用户 ID"})
		return
	}

	// 查找用户
	user, err := auth.FindByID(db.DB(c.Request.Context()), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, GetUserResponse{ErrorMsg: "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, GetUserResponse{Data: user.ToUserInfo()})
}
