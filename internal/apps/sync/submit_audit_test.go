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

package sync

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newSubmitAuditFixture(t *testing.T) (*Handler, *audit.Repository, *Service, uint64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:submit-audit-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.AutoMigrate(
		&Task{}, &TaskVersion{}, &JobInstance{},
		&auth.User{}, &audit.AuditLog{},
		&executionapp.Execution{}, &executionapp.Confirmation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &auth.User{Username: "alice", PasswordHash: "x", IsActive: true}
	if err := database.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	auditRepo := audit.NewRepository(database)
	service := NewService(NewRepository(database))
	execService := executionapp.NewService(executionapp.NewRepository(database), executionapp.NewProviderRegistry())
	execService.SetAuditRepository(auditRepo)
	service.SetExecutionService(execService)
	handler := NewHandler(service)
	handler.SetAuditRepository(auditRepo)
	return handler, auditRepo, service, user.ID
}

func newSubmitAuditRouter(handler *Handler, userID uint64, requestID string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyUser, &auth.User{ID: userID, Username: "alice", IsActive: true})
		c.Request.Header.Set("X-Request-ID", requestID)
		c.Request = c.Request.WithContext(audit.WithCommandMetadata(c.Request.Context(), audit.CommandMetadata{
			RequestID:   requestID,
			OwnerUserID: uint(userID),
			ClientType:  "web",
		}))
		c.Next()
	})
	router.POST("/tasks/:id/submit", handler.SubmitTask)
	return router
}

func TestSubmitTaskWritesAuditOnSuccessAndFailure(t *testing.T) {
	handler, auditRepo, service, userID := newSubmitAuditFixture(t)
	ctx := context.Background()

	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType: string(TaskNodeTypeFolder), Name: "workspace", ContentFormat: string(ContentFormatHOCON),
	}, uint(userID))
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	draft, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID: uintPtr(folder.ID), NodeType: string(TaskNodeTypeFile), Name: "draft",
		Mode: string(TaskModeBatch), ContentFormat: string(ContentFormatHOCON), Content: fakeBatchHOCON,
	}, uint(userID))
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	router := newSubmitAuditRouter(handler, userID, "req-submit-fail")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tasks/%d/submit", draft.ID), bytes.NewBufferString(`{"external_ref":"ds:1:2"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-submit-fail")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Fatalf("unpublished submit should fail, status=%d body=%s", w.Code, w.Body.String())
	}

	logs, total, err := auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{Action: "submit", IncludeAll: true})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 failure audit, got %d", total)
	}
	if logs[0].Details["result_status"] != "failed" {
		t.Fatalf("expected failed result_status, got %+v", logs[0].Details)
	}
	errMsg, _ := logs[0].Details["error"].(string)
	if errMsg == "" || errMsg == "sync operation failed" {
		t.Fatalf("failure audit should keep real error, got %q", errMsg)
	}
	if !strings.Contains(errMsg, "published") {
		t.Fatalf("expected unpublished error text, got %q", errMsg)
	}
	if logs[0].RequestID != "req-submit-fail" || logs[0].ResourceType != "sync_task" {
		t.Fatalf("failure audit linkage incomplete: %+v", logs[0])
	}

	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID: uintPtr(folder.ID), NodeType: string(TaskNodeTypeFile), Name: "demo",
		Mode: string(TaskModeBatch), ContentFormat: string(ContentFormatHOCON), Content: fakeBatchHOCON,
	}, uint(userID))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if _, _, err := service.PublishTask(ctx, file.ID, "v1", uint(userID)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	routerOK := newSubmitAuditRouter(handler, userID, "req-submit-ok")
	body := `{"external_ref":"ds:1001:88"}`
	reqOK := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tasks/%d/submit", file.ID), bytes.NewBufferString(body))
	reqOK.Header.Set("Content-Type", "application/json")
	reqOK.Header.Set("X-Request-ID", "req-submit-ok")
	wOK := httptest.NewRecorder()
	routerOK.ServeHTTP(wOK, reqOK)
	if wOK.Code != http.StatusOK {
		t.Fatalf("published submit should succeed, status=%d body=%s", wOK.Code, wOK.Body.String())
	}

	logs, total, err = auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{Action: "submit", RequestID: "req-submit-ok", IncludeAll: true})
	if err != nil {
		t.Fatalf("list success audit: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 success audit, got %d", total)
	}
	if logs[0].Details["result_status"] != "success" {
		t.Fatalf("expected success result_status, got %+v", logs[0].Details)
	}
	executionID, _ := logs[0].Details["execution_id"].(string)
	if strings.TrimSpace(executionID) == "" && logs[0].ExecutionID == "" {
		t.Fatalf("success audit should link execution_id: %+v", logs[0])
	}
}

func TestRunWithExecutionKeepsRealErrorCode(t *testing.T) {
	_, _, service, userID := newSubmitAuditFixture(t)
	ctx := context.Background()

	_, err := service.runWithExecution(ctx, uint(userID), "sync.task.submit", executionapp.RiskLevelR1, ExecutionRequest{
		RequestID:  "req-err-code",
		ClientType: "cli",
	}, func(context.Context) (*JobInstance, error) {
		return nil, ErrTaskNotPublished
	})
	if err == nil {
		t.Fatal("expected ErrTaskNotPublished")
	}

	var executions []executionapp.Execution
	if err := service.repo.db.Find(&executions).Error; err != nil {
		t.Fatalf("list executions: %v", err)
	}
	found := false
	for i := range executions {
		item := executions[i]
		if item.RequestID != "req-err-code" {
			continue
		}
		found = true
		if item.ErrorCode != "task_not_published" {
			t.Fatalf("error_code=%q want task_not_published msg=%q", item.ErrorCode, item.ErrorMessage)
		}
		if item.ErrorMessage == "" || item.ErrorMessage == "sync operation failed" {
			t.Fatalf("error_message should keep real error, got %q", item.ErrorMessage)
		}
	}
	if !found {
		t.Fatal("execution for req-err-code not found")
	}
}
