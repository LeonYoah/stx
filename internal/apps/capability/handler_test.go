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

package capability

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	appdb "github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newCapabilityTestDB 创建能力接口所需的独立内存数据库。
// newCapabilityTestDB creates an isolated in-memory database for capability tests.
func newCapabilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("创建能力测试数据库失败: %v", err)
	}
	if err := database.AutoMigrate(&auth.User{}, &auth.CLIToken{}); err != nil {
		t.Fatalf("迁移能力测试表失败: %v", err)
	}
	return database
}

// createCapabilityTestUser 创建一个可签发 CLI 令牌的用户。
// createCapabilityTestUser creates a user that can receive a CLI token.
func createCapabilityTestUser(t *testing.T, database *gorm.DB, active, admin bool) *auth.User {
	t.Helper()
	user := &auth.User{Username: "capability-user", IsActive: active, IsAdmin: admin}
	if err := user.SetPassword("secret123", auth.DefaultBcryptCost); err != nil {
		t.Fatalf("设置能力测试密码失败: %v", err)
	}
	if err := database.Create(user).Error; err != nil {
		t.Fatalf("创建能力测试用户失败: %v", err)
	}
	return user
}

// capabilityRouter 创建只包含能力路由的 Gin 测试路由。
// capabilityRouter creates a Gin router containing only the capability endpoint.
func capabilityRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/capabilities", auth.CLIAuthRequired(), List)
	return router
}

func capabilityRequest(t *testing.T, router http.Handler, token string, cliHeader bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	if cliHeader {
		req.Header.Set("X-STX-Client", "cli")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestListRequiresCLIHeader(t *testing.T) {
	database := newCapabilityTestDB(t)
	previous := appdb.GetGlobalDB()
	appdb.SetGlobalDB(database)
	t.Cleanup(func() { appdb.SetGlobalDB(previous) })

	response := capabilityRequest(t, capabilityRouter(), "", false)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("缺少 CLI 请求头的状态码 = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestListRequiresCLIToken(t *testing.T) {
	database := newCapabilityTestDB(t)
	previous := appdb.GetGlobalDB()
	appdb.SetGlobalDB(database)
	t.Cleanup(func() { appdb.SetGlobalDB(previous) })

	response := capabilityRequest(t, capabilityRouter(), "", true)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("缺少 CLI 令牌的状态码 = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestListReturnsRegistryDigestForActiveUser(t *testing.T) {
	database := newCapabilityTestDB(t)
	previous := appdb.GetGlobalDB()
	appdb.SetGlobalDB(database)
	t.Cleanup(func() { appdb.SetGlobalDB(previous) })

	user := createCapabilityTestUser(t, database, true, false)
	issued, err := auth.IssueCLIToken(database, user, "7d", testNow())
	if err != nil {
		t.Fatalf("签发能力测试令牌失败: %v", err)
	}

	response := capabilityRequest(t, capabilityRouter(), issued.Token, true)
	if response.Code != http.StatusOK {
		t.Fatalf("能力查询状态码 = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body Response
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析能力响应失败: %v", err)
	}
	if body.Data.RegistryRevision != operation.RegistryDigest() {
		t.Fatalf("登记表摘要 = %q, want %q", body.Data.RegistryRevision, operation.RegistryDigest())
	}
	if len(body.Data.Operations) == 0 {
		t.Fatal("能力响应不应为空")
	}
}

func TestListRejectsTokenAfterUserDisabled(t *testing.T) {
	database := newCapabilityTestDB(t)
	previous := appdb.GetGlobalDB()
	appdb.SetGlobalDB(database)
	t.Cleanup(func() { appdb.SetGlobalDB(previous) })

	user := createCapabilityTestUser(t, database, true, false)
	issued, err := auth.IssueCLIToken(database, user, "7d", testNow())
	if err != nil {
		t.Fatalf("签发禁用用户测试令牌失败: %v", err)
	}
	if err := database.Model(&auth.User{}).Where("id = ?", user.ID).Update("is_active", false).Error; err != nil {
		t.Fatalf("禁用测试用户失败: %v", err)
	}

	response := capabilityRequest(t, capabilityRouter(), issued.Token, true)
	if response.Code != http.StatusForbidden {
		t.Fatalf("禁用用户能力查询状态码 = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestVersionEndpointReturnsProductVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/version", Version)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("version status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body VersionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode version response: %v", err)
	}
	if body.Data.Version == "" {
		t.Fatal("expected non-empty version")
	}
	if body.Data.MinCLIVersion == "" {
		t.Fatal("expected non-empty min_cli_version")
	}
}

func TestPermissionForOperationRequiresAdmin(t *testing.T) {
	allowed, denialCode := permissionForOperation(operation.OperationSpec{AdminOnly: true}, &auth.User{IsAdmin: false})
	if allowed || denialCode != "admin_required" {
		t.Fatalf("普通用户管理员操作权限 = (%v, %q)", allowed, denialCode)
	}

	allowed, denialCode = permissionForOperation(operation.OperationSpec{AdminOnly: true}, &auth.User{IsAdmin: true})
	if !allowed || denialCode != "" {
		t.Fatalf("管理员操作权限 = (%v, %q)", allowed, denialCode)
	}
}

func testNow() time.Time {
	// 用接近当前时间签发，避免固定日期在 7d 过期后令 CI 误红。
	// Use near-current time so a 7d TTL does not expire and flake CI.
	return time.Now().UTC()
}
