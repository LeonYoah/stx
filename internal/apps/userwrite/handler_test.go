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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	appdb "github.com/LeonYoah/stx/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCreateAndRevokeCLIToken(t *testing.T) {
	_, adminUser, handler := newUserWriteTestHandler(t)
	router := newUserWriteTestRouter(handler, adminUser)

	create := performUserWriteRequest(router, http.MethodPost, "/admin/cli-tokens",
		`{"user_id":1,"name":"worker","expires_in":"never"}`, "token-create", "", true)
	if create.Code != http.StatusOK {
		t.Fatalf("签发令牌失败: status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Data struct {
			ID        uint64 `json:"id"`
			Token     string `json:"token"`
			Permanent bool   `json:"permanent"`
			Name      string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil || created.Data.ID == 0 || created.Data.Token == "" || !created.Data.Permanent || created.Data.Name != "worker" {
		t.Fatalf("签发响应错误: err=%v body=%s", err, create.Body.String())
	}

	retry := performUserWriteRequest(router, http.MethodPost, "/admin/cli-tokens",
		`{"user_id":1,"name":"worker","expires_in":"never"}`, "token-create", "", true)
	if retry.Code != http.StatusOK {
		t.Fatalf("签发幂等重试失败: status=%d body=%s", retry.Code, retry.Body.String())
	}
	if strings.Contains(retry.Body.String(), created.Data.Token) {
		t.Fatalf("幂等重试不应再次返回令牌原文: %s", retry.Body.String())
	}

	custom := performUserWriteRequest(router, http.MethodPost, "/admin/cli-tokens",
		`{"user_id":1,"name":"ci","expires_in":"90d"}`, "token-create-90d", "", true)
	if custom.Code != http.StatusOK {
		t.Fatalf("签发 90 天令牌失败: status=%d body=%s", custom.Code, custom.Body.String())
	}
	var customCreated struct {
		Data struct {
			Permanent bool    `json:"permanent"`
			ExpiresAt *string `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(custom.Body.Bytes(), &customCreated); err != nil || customCreated.Data.Permanent || customCreated.Data.ExpiresAt == nil {
		t.Fatalf("90 天令牌响应错误: err=%v body=%s", err, custom.Body.String())
	}

	revokePath := fmt.Sprintf("/admin/cli-tokens/%d/revoke", created.Data.ID)
	revoked := performUserWriteRequest(router, http.MethodPost, revokePath, "", "token-revoke", "", true)
	if revoked.Code != http.StatusOK {
		t.Fatalf("吊销令牌失败: status=%d body=%s", revoked.Code, revoked.Body.String())
	}
}

func TestCreateAndUpdateUserUseSharedExecutionWithoutLeakingPassword(t *testing.T) {
	database, adminUser, handler := newUserWriteTestHandler(t)
	router := newUserWriteTestRouter(handler, adminUser)

	createBody := `{"username":"cli-user","password":"secret-value","nickname":"before","is_admin":false}`
	first := performUserWriteRequest(router, http.MethodPost, "/admin/users", createBody, "create-key", "", true)
	if first.Code != http.StatusOK {
		t.Fatalf("创建用户失败: status=%d body=%s", first.Code, first.Body.String())
	}
	var created struct {
		Data auth.UserInfo `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil || created.Data.ID == 0 {
		t.Fatalf("创建响应错误: err=%v body=%s", err, first.Body.String())
	}

	retry := performUserWriteRequest(router, http.MethodPost, "/admin/users", createBody, "create-key", "", true)
	if retry.Code != http.StatusOK {
		t.Fatalf("相同幂等请求重试失败: status=%d body=%s", retry.Code, retry.Body.String())
	}
	conflict := performUserWriteRequest(router, http.MethodPost, "/admin/users",
		`{"username":"cli-user-2","password":"different-value"}`, "create-key", "", true)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("不同请求复用幂等键应冲突: status=%d body=%s", conflict.Code, conflict.Body.String())
	}

	updatePath := fmt.Sprintf("/admin/users/%d", created.Data.ID)
	update := performUserWriteRequest(router, http.MethodPut, updatePath,
		`{"nickname":"after","password":"new-secret","is_active":false,"is_admin":false}`, "update-key", "", true)
	if update.Code != http.StatusOK {
		t.Fatalf("更新用户失败: status=%d body=%s", update.Code, update.Body.String())
	}
	stored, err := auth.FindByID(database, created.Data.ID)
	if err != nil || stored.IsActive || stored.IsAdmin || stored.Nickname != "after" || !stored.CheckPassword("new-secret") {
		t.Fatalf("更新结果错误: user=%+v err=%v", stored, err)
	}

	var executionCount int64
	if err := database.Model(&executionapp.Execution{}).Count(&executionCount).Error; err != nil || executionCount != 2 {
		t.Fatalf("公共执行记录数量错误: count=%d err=%v", executionCount, err)
	}
	var logs []audit.AuditLog
	if err := database.Find(&logs).Error; err != nil {
		t.Fatalf("读取审计记录失败: %v", err)
	}
	content, _ := json.Marshal(logs)
	for _, secret := range []string{"secret-value", "new-secret", executionapp.HashString("secret-value"), executionapp.HashString("new-secret")} {
		if strings.Contains(string(content), secret) {
			t.Fatalf("审计记录泄露密码信息: %s", content)
		}
	}
}

func TestDeleteUserRequiresOneTimeConfirmationAndSupportsRetry(t *testing.T) {
	database, adminUser, handler := newUserWriteTestHandler(t)
	target := &auth.User{Username: "delete-user", IsActive: true}
	if err := target.SetPassword("secret-value", 4); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(target).Error; err != nil {
		t.Fatal(err)
	}
	router := newUserWriteTestRouter(handler, adminUser)
	path := fmt.Sprintf("/admin/users/%d", target.ID)

	first := performUserWriteRequest(router, http.MethodDelete, path, "", "delete-key", "", true)
	if first.Code != http.StatusPreconditionRequired {
		t.Fatalf("首次删除应要求一次性确认: status=%d body=%s", first.Code, first.Body.String())
	}
	var confirmation struct {
		Data struct {
			ConfirmationID string `json:"confirmation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &confirmation); err != nil || confirmation.Data.ConfirmationID == "" {
		t.Fatalf("确认响应错误: err=%v body=%s", err, first.Body.String())
	}
	if _, err := auth.FindByID(database, target.ID); err != nil {
		t.Fatalf("确认前不应删除用户: %v", err)
	}

	second := performUserWriteRequest(router, http.MethodDelete, path, "", "delete-key", confirmation.Data.ConfirmationID, true)
	if second.Code != http.StatusOK {
		t.Fatalf("确认后删除失败: status=%d body=%s", second.Code, second.Body.String())
	}
	if _, err := auth.FindByID(database, target.ID); err == nil {
		t.Fatal("确认后用户仍然存在")
	}
	retry := performUserWriteRequest(router, http.MethodDelete, path, "", "delete-key", confirmation.Data.ConfirmationID, true)
	if retry.Code != http.StatusOK {
		t.Fatalf("删除幂等重试失败: status=%d body=%s", retry.Code, retry.Body.String())
	}
}

func TestUpdateProfileRequiresR1ConfirmationAndIsIdempotent(t *testing.T) {
	database, adminUser, handler := newUserWriteTestHandler(t)
	router := newUserWriteTestRouter(handler, adminUser)

	missingConfirm := performUserWriteRequest(router, http.MethodPut, "/auth/profile", `{"language":"en"}`, "profile-key", "", false)
	if missingConfirm.Code != http.StatusPreconditionRequired {
		t.Fatalf("个人资料更新缺少确认时应被拒绝: status=%d body=%s", missingConfirm.Code, missingConfirm.Body.String())
	}
	first := performUserWriteRequest(router, http.MethodPut, "/auth/profile", `{"language":"en"}`, "profile-key", "", true)
	if first.Code != http.StatusOK {
		t.Fatalf("个人资料更新失败: status=%d body=%s", first.Code, first.Body.String())
	}
	retry := performUserWriteRequest(router, http.MethodPut, "/auth/profile", `{"language":"en"}`, "profile-key", "", true)
	if retry.Code != http.StatusOK {
		t.Fatalf("个人资料幂等重试失败: status=%d body=%s", retry.Code, retry.Body.String())
	}
	stored, err := auth.FindByID(database, adminUser.ID)
	if err != nil || stored.Language != "en" {
		t.Fatalf("个人资料没有更新: user=%+v err=%v", stored, err)
	}
}

func TestUpdateProfileTimezone(t *testing.T) {
	database, adminUser, handler := newUserWriteTestHandler(t)
	router := newUserWriteTestRouter(handler, adminUser)

	invalid := performUserWriteRequest(router, http.MethodPut, "/auth/profile", `{"timezone":"Mars/Base"}`, "tz-bad", "", true)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("非法时区应被拒绝: status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	set := performUserWriteRequest(router, http.MethodPut, "/auth/profile", `{"timezone":"America/New_York"}`, "tz-set", "", true)
	if set.Code != http.StatusOK || !strings.Contains(set.Body.String(), `"timezone":"America/New_York"`) {
		t.Fatalf("设置时区失败: status=%d body=%s", set.Code, set.Body.String())
	}
	clear := performUserWriteRequest(router, http.MethodPut, "/auth/profile", `{"timezone":""}`, "tz-clear", "", true)
	if clear.Code != http.StatusOK {
		t.Fatalf("清空时区失败: status=%d body=%s", clear.Code, clear.Body.String())
	}
	stored, err := auth.FindByID(database, adminUser.ID)
	if err != nil || stored.Timezone != "" {
		t.Fatalf("时区应恢复为跟随浏览器: user=%+v err=%v", stored, err)
	}
}

func newUserWriteTestHandler(t *testing.T) (*gorm.DB, *auth.User, *Handler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:userwrite-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	if err := database.AutoMigrate(&auth.User{}, &auth.CLIToken{}, &executionapp.Execution{}, &executionapp.Confirmation{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	previous := appdb.GetGlobalDB()
	appdb.SetGlobalDB(database)
	t.Cleanup(func() { appdb.SetGlobalDB(previous) })
	adminUser := &auth.User{Username: "admin", IsActive: true, IsAdmin: true}
	if err := adminUser.SetPassword("admin-secret", 4); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(adminUser).Error; err != nil {
		t.Fatal(err)
	}
	auditRepo := audit.NewRepository(database)
	executionService := executionapp.NewService(executionapp.NewRepository(database), executionapp.NewProviderRegistry())
	executionService.SetAuditRepository(auditRepo)
	return database, adminUser, NewHandler(executionService, auditRepo)
}

func newUserWriteTestRouter(handler *Handler, currentUser *auth.User) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetUserToContext(c, currentUser)
		c.Next()
	})
	router.POST("/admin/users", handler.CreateUser)
	router.PUT("/admin/users/:id", handler.UpdateUser)
	router.DELETE("/admin/users/:id", handler.DeleteUser)
	router.POST("/admin/cli-tokens", handler.CreateCLIToken)
	router.POST("/admin/cli-tokens/:id/revoke", handler.RevokeCLIToken)
	router.PUT("/auth/profile", handler.UpdateProfile)
	return router
}

func performUserWriteRequest(router http.Handler, method, path, body, idempotencyKey, confirmationID string, confirmed bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-STX-Client", "cli")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	if confirmed {
		request.Header.Set("X-STX-Confirm", "true")
	}
	if confirmationID != "" {
		request.Header.Set("X-STX-Confirmation-ID", confirmationID)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
