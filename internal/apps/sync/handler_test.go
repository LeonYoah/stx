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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGetJobLogsRejectsLegacyLinesQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newTestSyncService(t)
	handler := NewHandler(service)
	router := gin.New()
	router.GET("/api/v1/sync/jobs/:id/logs", handler.GetJobLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/jobs/33/logs?lines=400", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestGetJobLogsRejectsLegacyAllQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newTestSyncService(t)
	handler := NewHandler(service)
	router := gin.New()
	router.GET("/api/v1/sync/jobs/:id/logs", handler.GetJobLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/jobs/33/logs?all=true", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestTaskPermissionHandlersKeepContentAndPermissionChangesSeparate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := database.AutoMigrate(&Task{}, &TaskVersion{}, &JobInstance{}, &GlobalVariable{}, &PreviewSession{}, &PreviewTable{}, &PreviewRow{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("migrate test models failed: %v", err)
	}

	service := NewService(NewRepository(database))
	auditRepo := audit.NewRepository(database)
	handler := NewHandler(service)
	handler.SetAuditRepository(auditRepo)

	folder, err := service.CreateTask(context.Background(), &CreateTaskRequest{
		NodeType: string(TaskNodeTypeFolder),
		Name:     "handler_permissions",
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	task, err := service.CreateTask(context.Background(), &CreateTaskRequest{
		ParentID:      &folder.ID,
		NodeType:      string(TaskNodeTypeFile),
		Name:          "handler_permissions.env",
		Mode:          string(TaskModeBatch),
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { parallelism = 1 }",
		Definition:    JSONMap{"is_public": true},
	}, 1)
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	owner := &auth.User{ID: 1, Username: "owner", IsActive: true}
	regular := &auth.User{ID: 4, Username: "regular", IsActive: true}
	currentUser := owner
	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetUserToContext(c, currentUser)
		c.Next()
	})
	router.GET("/api/v1/sync/tasks/:id/permissions", handler.GetTaskPermissions)
	router.PUT("/api/v1/sync/tasks/:id/permissions", handler.UpdateTaskPermissions)

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/v1/sync/tasks/"+strconv.Itoa(int(task.ID))+"/permissions", nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get permissions returned %d: %s", getResponse.Code, getResponse.Body.String())
	}
	var permissionsResponse TaskPermissionsResponse
	if err := json.Unmarshal(getResponse.Body.Bytes(), &permissionsResponse); err != nil {
		t.Fatalf("decode permissions response failed: %v", err)
	}
	if permissionsResponse.Data == nil || !permissionsResponse.Data.IsOwner || !permissionsResponse.Data.CanManage || !permissionsResponse.Data.IsPublic {
		t.Fatalf("unexpected owner permissions: %+v", permissionsResponse.Data)
	}

	currentUser = regular
	deniedResponse := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodPut, "/api/v1/sync/tasks/"+strconv.Itoa(int(task.ID))+"/permissions", bytes.NewBufferString(`{"is_public":false}`))
	deniedRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(deniedResponse, deniedRequest)
	if deniedResponse.Code != http.StatusForbidden {
		t.Fatalf("regular user update returned %d: %s", deniedResponse.Code, deniedResponse.Body.String())
	}

	currentUser = owner
	updateResponse := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/v1/sync/tasks/"+strconv.Itoa(int(task.ID))+"/permissions", bytes.NewBufferString(`{"is_public":false,"collaborator_ids":[4]}`))
	updateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("owner update returned %d: %s", updateResponse.Code, updateResponse.Body.String())
	}

	storedTask, err := service.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if storedTask.Content != "env { parallelism = 1 }" {
		t.Fatalf("permission update changed task content: %q", storedTask.Content)
	}

	logs, total, err := auditRepo.ListAuditLogs(context.Background(), &audit.AuditLogFilter{
		ResourceType: "sync_task_permissions",
		ResourceID:   strconv.Itoa(int(task.ID)),
		IncludeAll:   true,
		Page:         1,
		PageSize:     10,
	})
	if err != nil {
		t.Fatalf("list permission audit logs failed: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].Details["operation_id"] != "sync.task.permissions.update" {
		t.Fatalf("permission audit record is incomplete: total=%d logs=%+v", total, logs)
	}
	if _, ok := logs[0].Details["before"]; !ok {
		t.Fatalf("permission audit record is missing before state: %+v", logs[0].Details)
	}
	if _, ok := logs[0].Details["after"]; !ok {
		t.Fatalf("permission audit record is missing after state: %+v", logs[0].Details)
	}
}
