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

// Package auth 提供用户认证相关的中间件
package auth

import (
	"net/http"
	"strings"

	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/otel_trace"
	"github.com/gin-gonic/gin"
)

// 上下文键常量
const (
	ContextKeyUser = "auth_user"
)

// 错误响应
type ErrorResponse struct {
	ErrorMsg string      `json:"error_msg"`
	Data     interface{} `json:"data"`
}

// LoginRequired 登录验证中间件
// 验证用户是否已登录，如果未登录则返回 401 错误
func LoginRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 初始化追踪
		ctx, span := otel_trace.Start(c.Request.Context(), "LoginRequired")
		defer span.End()

		// 从会话获取用户 ID
		userID := GetUserIDFromContext(c)
		if userID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				ErrorMsg: "未登录",
				Data:     nil,
			})
			return
		}

		// 从数据库加载用户，确保用户存在且激活
		user, err := FindByID(db.GetDB(ctx), userID)
		if err != nil {
			logger.ErrorF(ctx, "[LoginRequired] 用户不存在: %d, %v", userID, err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				ErrorMsg: "用户不存在",
				Data:     nil,
			})
			return
		}

		// 检查用户是否激活
		if !user.IsActive {
			logger.InfoF(ctx, "[LoginRequired] 用户已禁用: %d %s", user.ID, user.Username)
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{
				ErrorMsg: ErrMsgUserInactive,
				Data:     nil,
			})
			return
		}

		logger.DebugF(ctx, "[LoginRequired] 验证通过: %d %s", user.ID, user.Username)

		// 将用户信息存入上下文
		SetUserToContext(c, user)

		// 继续处理请求
		c.Next()
	}
}

// LoginRequiredSessionOnly 轻量会话登录验证中间件（不查数据库）
// 适用于高频静态资源代理场景，避免每个请求触发数据库读取。
func LoginRequiredSessionOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, span := otel_trace.Start(c.Request.Context(), "LoginRequiredSessionOnly")
		defer span.End()

		userID := GetUserIDFromContext(c)
		if userID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				ErrorMsg: "未登录",
				Data:     nil,
			})
			return
		}

		username := strings.TrimSpace(GetUsernameFromContext(c))
		if username == "" {
			username = "unknown"
		}

		// 仅注入最小可用用户上下文，避免后续处理器空指针。
		SetUserToContext(c, &User{
			ID:       userID,
			Username: username,
			IsActive: true,
		})

		logger.DebugF(ctx, "[LoginRequiredSessionOnly] 验证通过: %d %s", userID, username)
		c.Next()
	}
}

// AdminRequired 管理员权限验证中间件
// 验证用户是否为管理员，如果不是则返回 403 错误
// 注意：此中间件应在 LoginRequired 之后使用
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 初始化追踪
		ctx, span := otel_trace.Start(c.Request.Context(), "AdminRequired")
		defer span.End()

		// 从上下文获取用户
		user := GetUserFromContext(c)
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				ErrorMsg: "未登录",
				Data:     nil,
			})
			return
		}

		// 检查是否为管理员
		if !user.IsAdmin {
			logger.InfoF(ctx, "[AdminRequired] 非管理员访问: %d %s", user.ID, user.Username)
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{
				ErrorMsg: "需要管理员权限",
				Data:     nil,
			})
			return
		}

		logger.DebugF(ctx, "[AdminRequired] 管理员验证通过: %d %s", user.ID, user.Username)

		// 继续处理请求
		c.Next()
	}
}

// SetUserToContext 将用户信息存入 Gin 上下文
func SetUserToContext(c *gin.Context, user *User) {
	c.Set(ContextKeyUser, user)
}

// GetUserFromContext 从 Gin 上下文获取用户信息
func GetUserFromContext(c *gin.Context) *User {
	value, exists := c.Get(ContextKeyUser)
	if !exists {
		return nil
	}
	user, ok := value.(*User)
	if !ok {
		return nil
	}
	return user
}
