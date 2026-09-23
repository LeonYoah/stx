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

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestTroubleshootingMemoryCreateIsIdempotentForCLI 验证 CLI 重试不会重复创建经验记录，并且成功写入审计。
// TestTroubleshootingMemoryCreateIsIdempotentForCLI verifies that CLI retries do not duplicate records and successful writes are audited.
func TestTroubleshootingMemoryCreateIsIdempotentForCLI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("打开测试数据库失败 / opening test database failed: %v", err)
	}
	if err := database.AutoMigrate(&TroubleshootingMemory{}, &executionapp.Execution{}, &executionapp.Confirmation{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("迁移测试表失败 / migrating test tables failed: %v", err)
	}

	service := NewServiceWithRepository(NewRepository(database), nil, nil, nil)
	executionService := executionapp.NewService(executionapp.NewRepository(database), executionapp.NewProviderRegistry())
	service.SetExecutionService(executionService)
	handler := NewHandler(service)
	handler.SetAuditRepository(audit.NewRepository(database))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetUserToContext(c, &auth.User{ID: 7, Username: "cli-user", IsActive: true})
		c.Next()
	})
	router.POST("/api/v1/diagnostics/troubleshooting-memories", handler.CreateTroubleshootingMemory)

	body := []byte(`{"target_type":"error","fingerprint":"same","title":"same","solution":"same"}`)
	first := performTroubleshootingMemoryCreate(t, router, body, "same-key")
	second := performTroubleshootingMemoryCreate(t, router, body, "same-key")
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("重复请求应成功返回同一结果 / repeated requests should return the same success: first=%d second=%d", first.Code, second.Code)
	}

	var firstResponse, secondResponse struct {
		Data TroubleshootingMemoryItem `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstResponse); err != nil {
		t.Fatalf("解析首次响应失败 / decoding first response failed: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResponse); err != nil {
		t.Fatalf("解析重复响应失败 / decoding repeated response failed: %v", err)
	}
	if firstResponse.Data.ID == "" || firstResponse.Data.ID != secondResponse.Data.ID {
		t.Fatalf("幂等请求返回了不同记录 / idempotent requests returned different records: first=%s second=%s", firstResponse.Data.ID, secondResponse.Data.ID)
	}

	var memoryCount int64
	if err := database.Model(&TroubleshootingMemory{}).Count(&memoryCount).Error; err != nil || memoryCount != 1 {
		t.Fatalf("幂等请求产生了重复记录 / idempotent requests created duplicates: count=%d err=%v", memoryCount, err)
	}
	var auditCount int64
	if err := database.Model(&audit.AuditLog{}).Where("action = ? AND resource_type = ?", "create", "troubleshooting_memory").Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("审计记录数量错误 / audit record count is incorrect: count=%d err=%v", auditCount, err)
	}

	conflict := performTroubleshootingMemoryCreate(t, router, []byte(`{"target_type":"error","fingerprint":"changed","title":"changed","solution":"changed"}`), "same-key")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("不同请求复用幂等键应冲突 / reusing an idempotency key with a different request should conflict: code=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func performTroubleshootingMemoryCreate(t *testing.T, router http.Handler, body []byte, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/diagnostics/troubleshooting-memories", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-STX-Client", "cli")
	request.Header.Set("X-STX-Confirm", "true")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
