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

package diagnostics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestAutoPolicyCreateIsIdempotentForCLI 验证 CLI 重试不会重复创建自动巡检策略。
// TestAutoPolicyCreateIsIdempotentForCLI verifies that CLI retries do not duplicate automatic inspection policies.
func TestAutoPolicyCreateIsIdempotentForCLI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("打开测试数据库失败 / opening test database failed: %v", err)
	}
	if err := database.AutoMigrate(&InspectionAutoPolicy{}, &executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("迁移测试表失败 / migrating test tables failed: %v", err)
	}

	service := NewServiceWithRepository(NewRepository(database), nil, nil, nil)
	service.SetExecutionService(executionapp.NewService(executionapp.NewRepository(database), executionapp.NewProviderRegistry()))
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetUserToContext(c, &auth.User{ID: 9, Username: "cli-user", IsActive: true})
		c.Next()
	})
	router.POST("/api/v1/diagnostics/auto-policies", handler.CreateAutoPolicy)

	body := []byte(`{"cluster_id":6,"name":"same-policy","enabled":true,"conditions":[{"template_code":"SCHEDULED","enabled":true}],"cooldown_minutes":30,"auto_create_task":false,"auto_start_task":false}`)
	first := performDiagnosticsWrite(t, router, http.MethodPost, "/api/v1/diagnostics/auto-policies", body, "same-auto-policy-key")
	second := performDiagnosticsWrite(t, router, http.MethodPost, "/api/v1/diagnostics/auto-policies", body, "same-auto-policy-key")
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("重复请求应返回同一成功结果 / repeated requests should return the same success: first=%d second=%d first_body=%s second_body=%s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}

	var firstResponse, secondResponse struct {
		Data InspectionAutoPolicyInfo `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstResponse); err != nil {
		t.Fatalf("解析首次响应失败 / decoding first response failed: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResponse); err != nil {
		t.Fatalf("解析重复响应失败 / decoding repeated response failed: %v", err)
	}
	if firstResponse.Data.ID == 0 || firstResponse.Data.ID != secondResponse.Data.ID {
		t.Fatalf("幂等请求返回了不同策略 / idempotent requests returned different policies: first=%d second=%d", firstResponse.Data.ID, secondResponse.Data.ID)
	}

	var policyCount, executionCount int64
	if err := database.Model(&InspectionAutoPolicy{}).Count(&policyCount).Error; err != nil || policyCount != 1 {
		t.Fatalf("幂等请求产生了重复策略 / idempotent requests created duplicate policies: count=%d err=%v", policyCount, err)
	}
	if err := database.Model(&executionapp.Execution{}).Where("operation_id = ? AND owner_user_id = ?", "diagnostics.auto-policy.create", 9).Count(&executionCount).Error; err != nil || executionCount != 1 {
		t.Fatalf("公共执行记录数量错误 / shared execution count is incorrect: count=%d err=%v", executionCount, err)
	}
	var execution executionapp.Execution
	if err := database.Where("operation_id = ? AND owner_user_id = ?", "diagnostics.auto-policy.create", 9).First(&execution).Error; err != nil {
		t.Fatalf("读取公共执行记录失败 / loading shared execution failed: %v", err)
	}
	if execution.ModuleRef != fmt.Sprintf("%d", firstResponse.Data.ID) || execution.ResultRef != execution.ModuleRef {
		t.Fatalf("执行记录未绑定真实策略编号 / execution was not bound to the policy ID: module_ref=%s result_ref=%s", execution.ModuleRef, execution.ResultRef)
	}
}

func performDiagnosticsWrite(t *testing.T, router http.Handler, method, path string, body []byte, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-STX-Client", "cli")
	request.Header.Set("X-STX-Confirm", "true")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request.Header.Set("X-Request-ID", "req-"+idempotencyKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
