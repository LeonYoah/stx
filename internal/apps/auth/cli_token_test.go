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

package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appdb "github.com/LeonYoah/stx/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newCLITokenTestDB 创建只包含认证表的内存数据库。
// newCLITokenTestDB creates an in-memory database containing authentication tables.
func newCLITokenTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	if err := database.AutoMigrate(&User{}, &CLIToken{}); err != nil {
		t.Fatalf("迁移认证表失败: %v", err)
	}
	return database
}

// newCLITokenTestUser 创建可用于 CLI 登录的测试用户。
// newCLITokenTestUser creates a test user that can log in through the CLI endpoint.
func newCLITokenTestUser(t *testing.T, database *gorm.DB, active bool) *User {
	t.Helper()
	user := &User{Username: "cli-user", IsActive: active}
	if err := user.SetPassword("secret123", DefaultBcryptCost); err != nil {
		t.Fatalf("设置测试密码失败: %v", err)
	}
	if err := database.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return user
}

func TestParseCLITokenTTL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "default", input: "", want: DefaultCLITokenTTL},
		{name: "minimum", input: "7d", want: 7 * 24 * time.Hour},
		{name: "maximum", input: "30D", want: 30 * 24 * time.Hour},
		{name: "too short", input: "6d", wantErr: true},
		{name: "too long", input: "31d", wantErr: true},
		{name: "duration is not accepted", input: "168h", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCLITokenTTL(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidCLITokenTTL) {
					t.Fatalf("错误 = %v, want %v", err, ErrInvalidCLITokenTTL)
				}
				return
			}
			if err != nil {
				t.Fatalf("解析有效期失败: %v", err)
			}
			if got != tt.want {
				t.Fatalf("有效期 = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestIssueAndAuthenticateCLITokenStoresOnlyHash(t *testing.T) {
	database := newCLITokenTestDB(t)
	user := newCLITokenTestUser(t, database, true)
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)

	issue, err := IssueCLIToken(database, user, "7d", now)
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	if issue.Token == "" || issue.ExpiresAt.Equal(now) {
		t.Fatalf("签发结果不完整: %+v", issue)
	}

	var stored CLIToken
	if err := database.First(&stored).Error; err != nil {
		t.Fatalf("读取令牌失败: %v", err)
	}
	if stored.TokenHash == issue.Token {
		t.Fatal("数据库不应保存令牌原文")
	}
	if stored.TokenPrefix == "" || len(stored.TokenPrefix) >= len(issue.Token) {
		t.Fatalf("令牌前缀不正确: prefix=%q token=%q", stored.TokenPrefix, issue.Token)
	}

	gotToken, gotUser, err := AuthenticateCLIToken(database, issue.Token, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("校验令牌失败: %v", err)
	}
	if gotToken.ID != stored.ID || gotUser.ID != user.ID {
		t.Fatalf("校验结果不正确: token=%+v user=%+v", gotToken, gotUser)
	}
	if gotToken.LastUsedAt == nil || !gotToken.LastUsedAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("最近使用时间未更新: %v", gotToken.LastUsedAt)
	}
}

func TestAuthenticateCLITokenRejectsExpiredRevokedAndInactive(t *testing.T) {
	database := newCLITokenTestDB(t)
	user := newCLITokenTestUser(t, database, true)
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)

	expired, err := IssueCLIToken(database, user, "7d", now.Add(-8*24*time.Hour))
	if err != nil {
		t.Fatalf("签发过期测试令牌失败: %v", err)
	}
	if _, _, err := AuthenticateCLIToken(database, expired.Token, now); !errors.Is(err, ErrCLITokenExpired) {
		t.Fatalf("过期令牌错误 = %v", err)
	}

	active, err := IssueCLIToken(database, user, "7d", now)
	if err != nil {
		t.Fatalf("签发撤销测试令牌失败: %v", err)
	}
	if err := RevokeCLIToken(database, active.Token, now); err != nil {
		t.Fatalf("撤销令牌失败: %v", err)
	}
	if err := RevokeCLIToken(database, active.Token, now); err != nil {
		t.Fatalf("重复撤销不应失败: %v", err)
	}
	if _, _, err := AuthenticateCLIToken(database, active.Token, now); !errors.Is(err, ErrCLITokenRevoked) {
		t.Fatalf("已撤销令牌错误 = %v", err)
	}

	user.IsActive = false
	if err := database.Save(user).Error; err != nil {
		t.Fatalf("禁用测试用户失败: %v", err)
	}
	inactive, err := IssueCLIToken(database, user, "7d", now)
	if err == nil || !errors.Is(err, ErrUserInactive) {
		t.Fatalf("禁用用户签发结果 = %v", err)
	}
	_ = inactive
}

func TestCLILoginRequiresCLIHeaderAndLogoutRevokesToken(t *testing.T) {
	database := newCLITokenTestDB(t)
	newCLITokenTestUser(t, database, true)
	appdb.SetGlobalDB(database)
	t.Cleanup(func() { appdb.SetGlobalDB(nil) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/login", CLIClientRequired(), CLILogin)
	router.POST("/logout", CLIAuthRequired(), CLILogout)
	router.GET("/whoami", CLIAuthRequired(), CLIWhoAmI)

	loginBody, _ := json.Marshal(CLILoginRequest{Username: "cli-user", Password: "secret123", ExpiresIn: "7d"})
	withoutHeader := httptest.NewRecorder()
	router.ServeHTTP(withoutHeader, httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(loginBody)))
	if withoutHeader.Code != http.StatusBadRequest {
		t.Fatalf("缺少 CLI 请求头的状态码 = %d", withoutHeader.Code)
	}

	loginRequest := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(loginBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("X-STX-Client", "cli")
	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("登录状态码 = %d, body=%s", loginResponse.Code, loginResponse.Body.String())
	}

	var login CLILoginResponse
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &login); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	if login.Data == nil || login.Data.Token == "" || login.Data.TokenType != "Bearer" {
		t.Fatalf("登录响应缺少令牌: %+v", login)
	}

	whoamiRequest := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	whoamiRequest.Header.Set("X-STX-Client", "cli")
	whoamiRequest.Header.Set("Authorization", "Bearer "+login.Data.Token)
	whoamiResponse := httptest.NewRecorder()
	router.ServeHTTP(whoamiResponse, whoamiRequest)
	if whoamiResponse.Code != http.StatusOK {
		t.Fatalf("whoami 状态码 = %d, body=%s", whoamiResponse.Code, whoamiResponse.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutRequest.Header.Set("X-STX-Client", "cli")
	logoutRequest.Header.Set("Authorization", "Bearer "+login.Data.Token)
	logoutResponse := httptest.NewRecorder()
	router.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusOK {
		t.Fatalf("登出状态码 = %d, body=%s", logoutResponse.Code, logoutResponse.Body.String())
	}

	afterLogout := httptest.NewRecorder()
	router.ServeHTTP(afterLogout, whoamiRequest)
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("登出后的 whoami 状态码 = %d", afterLogout.Code)
	}
}
