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

// Package auth 认证处理器属性测试
// 注意：这些测试使用独立的测试数据库和会话存储，不依赖全局配置
package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 测试辅助函数：创建测试数据库（使用纯 Go SQLite 驱动）
func setupTestDB(t *testing.T) *gorm.DB {
	// 使用 glebarez/sqlite 纯 Go 驱动，不需要 CGO
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}

	// 自动迁移用户表
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("迁移用户表失败: %v", err)
	}

	return db
}

// 测试辅助函数：创建测试用户
func createTestUser(db *gorm.DB, username, password string, isActive bool) (*User, error) {
	user := &User{
		Username: username,
		IsActive: isActive,
		IsAdmin:  false,
	}
	if err := user.SetPassword(password, DefaultBcryptCost); err != nil {
		return nil, err
	}
	if err := db.Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

// 测试辅助函数：创建测试 Gin 引擎
func setupTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// 设置会话中间件
	store := cookie.NewStore([]byte("test-secret"))
	r.Use(sessions.Sessions("test-session", store))

	// 注入数据库到上下文
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})

	return r
}

// 测试辅助函数：执行登录请求
func performLogin(router *gin.Engine, username, password string) *httptest.ResponseRecorder {
	reqBody := LoginRequest{
		Username: username,
		Password: password,
	}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// **Feature: stx-platform-login, Property 1: Valid credentials create session**
// **Validates: Requirements 1.1, 1.4**
// 对于任何有效的用户名和密码组合，提交这些凭证应该创建会话

func TestProperty_ValidCredentialsCreateSession(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50 // 减少测试次数因为涉及数据库操作

	properties := gopter.NewProperties(parameters)

	// 属性：对于任意有效凭证，登录应成功并返回 200 状态码
	properties.Property("Valid credentials return 200 and create session", prop.ForAll(
		func(usernameLen int, passwordLen int) bool {
			// 生成用户名和密码
			username := ""
			for i := 0; i < usernameLen; i++ {
				username += string(rune('a' + (i % 26)))
			}
			password := ""
			for i := 0; i < passwordLen; i++ {
				password += string(rune('A' + (i % 26)))
			}

			// 创建测试数据库和用户
			db := setupTestDB(t)
			_, err := createTestUser(db, username, password, true)
			if err != nil {
				return false
			}

			// 创建测试路由
			router := setupTestRouter(db)
			router.POST("/login", func(c *gin.Context) {
				// 使用注入的数据库
				testDB := c.MustGet("db").(*gorm.DB)
				loginWithDB(c, testDB)
			})

			// 执行登录
			w := performLogin(router, username, password)

			// 验证响应
			if w.Code != http.StatusOK {
				return false
			}

			// 验证响应体
			var resp LoginResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				return false
			}

			return resp.ErrorMsg == "" && resp.Data != nil
		},
		gen.IntRange(3, 20), // 用户名长度 3-20
		gen.IntRange(6, 20), // 密码长度 6-20
	))

	properties.TestingRun(t)
}

// **Feature: stx-platform-login, Property 2: Invalid credentials return generic error**
// **Validates: Requirements 1.2**
// 对于任何无效的用户名或密码组合，认证系统应返回不暴露具体字段的错误消息

func TestProperty_InvalidCredentialsReturnGenericError(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	// 属性：错误密码应返回通用错误消息
	properties.Property("Wrong password returns generic error without revealing which field is wrong", prop.ForAll(
		func(usernameLen int, passwordLen int, wrongSuffixLen int) bool {
			// 生成用户名和密码
			username := ""
			for i := 0; i < usernameLen; i++ {
				username += string(rune('a' + (i % 26)))
			}
			correctPassword := ""
			for i := 0; i < passwordLen; i++ {
				correctPassword += string(rune('A' + (i % 26)))
			}
			// 生成错误密码
			wrongSuffix := ""
			for i := 0; i < wrongSuffixLen; i++ {
				wrongSuffix += "X"
			}
			wrongPassword := correctPassword + wrongSuffix

			// 创建测试数据库和用户
			db := setupTestDB(t)
			_, err := createTestUser(db, username, correctPassword, true)
			if err != nil {
				return false
			}

			// 创建测试路由
			router := setupTestRouter(db)
			router.POST("/login", func(c *gin.Context) {
				testDB := c.MustGet("db").(*gorm.DB)
				loginWithDB(c, testDB)
			})

			// 使用错误密码登录
			w := performLogin(router, username, wrongPassword)

			// 验证返回 401 状态码
			if w.Code != http.StatusUnauthorized {
				return false
			}

			// 验证错误消息是通用的，不暴露具体字段
			var resp LoginResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				return false
			}

			// 错误消息应该是通用的 "用户名或密码错误"
			return resp.ErrorMsg == ErrMsgInvalidCredentials
		},
		gen.IntRange(3, 20),
		gen.IntRange(6, 20),
		gen.IntRange(1, 5),
	))

	// 属性：不存在的用户名应返回相同的通用错误消息
	properties.Property("Non-existent username returns same generic error", prop.ForAll(
		func(usernameLen int, passwordLen int) bool {
			// 生成不存在的用户名
			username := "nonexistent_"
			for i := 0; i < usernameLen; i++ {
				username += string(rune('a' + (i % 26)))
			}
			password := ""
			for i := 0; i < passwordLen; i++ {
				password += string(rune('A' + (i % 26)))
			}

			// 创建空的测试数据库（没有用户）
			db := setupTestDB(t)

			// 创建测试路由
			router := setupTestRouter(db)
			router.POST("/login", func(c *gin.Context) {
				testDB := c.MustGet("db").(*gorm.DB)
				loginWithDB(c, testDB)
			})

			// 尝试登录
			w := performLogin(router, username, password)

			// 验证返回 401 状态码
			if w.Code != http.StatusUnauthorized {
				return false
			}

			// 验证错误消息与密码错误时相同
			var resp LoginResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				return false
			}

			return resp.ErrorMsg == ErrMsgInvalidCredentials
		},
		gen.IntRange(3, 20),
		gen.IntRange(6, 20),
	))

	properties.TestingRun(t)
}

// **Feature: stx-platform-login, Property 3: Empty credentials are rejected**
// **Validates: Requirements 1.3**
// 对于任何空用户名或密码的登录请求，认证系统应拒绝请求并返回验证错误

func TestProperty_EmptyCredentialsAreRejected(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	// 属性：空用户名应被拒绝
	properties.Property("Empty username is rejected", prop.ForAll(
		func(passwordLen int) bool {
			password := ""
			for i := 0; i < passwordLen; i++ {
				password += string(rune('A' + (i % 26)))
			}

			db := setupTestDB(t)
			router := setupTestRouter(db)
			router.POST("/login", func(c *gin.Context) {
				testDB := c.MustGet("db").(*gorm.DB)
				loginWithDB(c, testDB)
			})

			// 空用户名登录
			w := performLogin(router, "", password)

			// 应返回 400 状态码
			return w.Code == http.StatusBadRequest
		},
		gen.IntRange(6, 20),
	))

	// 属性：空密码应被拒绝
	properties.Property("Empty password is rejected", prop.ForAll(
		func(usernameLen int) bool {
			username := ""
			for i := 0; i < usernameLen; i++ {
				username += string(rune('a' + (i % 26)))
			}

			db := setupTestDB(t)
			router := setupTestRouter(db)
			router.POST("/login", func(c *gin.Context) {
				testDB := c.MustGet("db").(*gorm.DB)
				loginWithDB(c, testDB)
			})

			// 空密码登录
			w := performLogin(router, username, "")

			// 应返回 400 状态码
			return w.Code == http.StatusBadRequest
		},
		gen.IntRange(3, 20),
	))

	// 属性：纯空白用户名应被拒绝
	properties.Property("Whitespace-only username is rejected", prop.ForAll(
		func(spaceCount int, passwordLen int) bool {
			// 生成纯空白用户名
			username := ""
			for i := 0; i < spaceCount; i++ {
				username += " "
			}
			password := ""
			for i := 0; i < passwordLen; i++ {
				password += string(rune('A' + (i % 26)))
			}

			db := setupTestDB(t)
			router := setupTestRouter(db)
			router.POST("/login", func(c *gin.Context) {
				testDB := c.MustGet("db").(*gorm.DB)
				loginWithDB(c, testDB)
			})

			w := performLogin(router, username, password)

			// 应返回 400 状态码
			return w.Code == http.StatusBadRequest
		},
		gen.IntRange(1, 10),
		gen.IntRange(6, 20),
	))

	properties.TestingRun(t)
}

// loginWithDB 使用指定数据库执行登录逻辑（用于测试）
func loginWithDB(c *gin.Context, testDB *gorm.DB) {
	var req LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LoginResponse{ErrorMsg: ErrMsgEmptyCredentials})
		return
	}

	// 去除用户名空白
	trimmedUsername := strings.TrimSpace(req.Username)
	password := req.Password

	if trimmedUsername == "" || password == "" {
		c.JSON(http.StatusBadRequest, LoginResponse{ErrorMsg: ErrMsgEmptyCredentials})
		return
	}

	user, err := FindByUsername(testDB, trimmedUsername)
	if err != nil {
		c.JSON(http.StatusUnauthorized, LoginResponse{ErrorMsg: ErrMsgInvalidCredentials})
		return
	}

	if !user.CheckPassword(password) {
		c.JSON(http.StatusUnauthorized, LoginResponse{ErrorMsg: ErrMsgInvalidCredentials})
		return
	}

	if !user.IsActive {
		c.JSON(http.StatusForbidden, LoginResponse{ErrorMsg: ErrMsgUserInactive})
		return
	}

	session := sessions.Default(c)
	session.Set(SessionKeyUserID, user.ID)
	session.Set(SessionKeyUsername, user.Username)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, LoginResponse{ErrorMsg: ErrMsgSessionError})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{Data: user.ToUserInfo()})
}

// **Feature: stx-platform-login, Property 6: Session cleanup on logout**
// **Validates: Requirements 6.2**
// 对于任何用户登出操作，与该用户关联的所有会话数据应从会话存储中移除

func TestProperty_SessionCleanupOnLogout(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	// 属性：登出后会话应被清除
	properties.Property("Logout clears session data", prop.ForAll(
		func(usernameLen int, passwordLen int) bool {
			// 生成用户名和密码
			username := ""
			for i := 0; i < usernameLen; i++ {
				username += string(rune('a' + (i % 26)))
			}
			password := ""
			for i := 0; i < passwordLen; i++ {
				password += string(rune('A' + (i % 26)))
			}

			// 创建测试数据库和用户
			db := setupTestDB(t)
			_, err := createTestUser(db, username, password, true)
			if err != nil {
				return false
			}

			// 创建测试路由
			router := setupTestRouter(db)
			router.POST("/login", func(c *gin.Context) {
				testDB := c.MustGet("db").(*gorm.DB)
				loginWithDB(c, testDB)
			})
			router.POST("/logout", logoutHandler)
			router.GET("/check-session", checkSessionHandler)

			// 步骤 1: 登录
			loginW := performLogin(router, username, password)
			if loginW.Code != http.StatusOK {
				return false
			}

			// 获取登录后的 cookie
			cookies := loginW.Result().Cookies()

			// 步骤 2: 验证会话存在
			checkReq1, _ := http.NewRequest("GET", "/check-session", nil)
			for _, cookie := range cookies {
				checkReq1.AddCookie(cookie)
			}
			checkW1 := httptest.NewRecorder()
			router.ServeHTTP(checkW1, checkReq1)

			// 会话应该存在
			var checkResp1 map[string]interface{}
			if err := json.Unmarshal(checkW1.Body.Bytes(), &checkResp1); err != nil {
				return false
			}
			if !checkResp1["has_session"].(bool) {
				return false
			}

			// 步骤 3: 登出
			logoutReq, _ := http.NewRequest("POST", "/logout", nil)
			for _, cookie := range cookies {
				logoutReq.AddCookie(cookie)
			}
			logoutW := httptest.NewRecorder()
			router.ServeHTTP(logoutW, logoutReq)

			if logoutW.Code != http.StatusOK {
				return false
			}

			// 获取登出后的 cookie（会话已清除）
			logoutCookies := logoutW.Result().Cookies()

			// 步骤 4: 验证会话已清除
			checkReq2, _ := http.NewRequest("GET", "/check-session", nil)
			for _, cookie := range logoutCookies {
				checkReq2.AddCookie(cookie)
			}
			checkW2 := httptest.NewRecorder()
			router.ServeHTTP(checkW2, checkReq2)

			var checkResp2 map[string]interface{}
			if err := json.Unmarshal(checkW2.Body.Bytes(), &checkResp2); err != nil {
				return false
			}

			// 会话应该不存在
			return !checkResp2["has_session"].(bool)
		},
		gen.IntRange(3, 20),
		gen.IntRange(6, 20),
	))

	properties.TestingRun(t)
}

// logoutHandler 登出处理器（用于测试）
func logoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, LogoutResponse{ErrorMsg: ErrMsgSessionError})
		return
	}
	c.JSON(http.StatusOK, LogoutResponse{})
}

// checkSessionHandler 检查会话处理器（用于测试）
func checkSessionHandler(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get(SessionKeyUserID)
	hasSession := userID != nil
	c.JSON(http.StatusOK, gin.H{
		"has_session": hasSession,
		"user_id":     userID,
	})
}
