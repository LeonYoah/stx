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

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
)

func TestAuditHandlersEnforceOwnerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	handler := NewHandler(repo)
	ctx := context.Background()
	ownerOne := uint(1)
	ownerTwo := uint(2)

	auditLogs := []*AuditLog{
		{UserID: &ownerOne, Username: "owner-one", Action: "create", ResourceType: "execution", ResourceID: "execution-1"},
		{UserID: &ownerTwo, Username: "owner-two", Action: "create", ResourceType: "execution", ResourceID: "execution-2"},
		{UserID: nil, Username: "system", Action: "create", ResourceType: "execution", ResourceID: "execution-system"},
	}
	for _, log := range auditLogs {
		if err := repo.CreateAuditLog(ctx, log); err != nil {
			t.Fatalf("create audit log: %v", err)
		}
	}
	commandLogs := []*CommandLog{
		{CommandID: "command-owner-one", AgentID: "agent-1", CommandType: "diagnostic", Status: CommandStatusSuccess, CreatedBy: &ownerOne},
		{CommandID: "command-owner-two", AgentID: "agent-1", CommandType: "diagnostic", Status: CommandStatusSuccess, CreatedBy: &ownerTwo},
		{CommandID: "command-system", AgentID: "agent-1", CommandType: "diagnostic", Status: CommandStatusSuccess},
	}
	for _, log := range commandLogs {
		if err := repo.CreateCommandLog(ctx, log); err != nil {
			t.Fatalf("create command log: %v", err)
		}
	}

	ownerUser := &auth.User{ID: uint64(ownerOne), Username: "owner-one", IsActive: true}
	adminUser := &auth.User{ID: 99, Username: "admin", IsActive: true, IsAdmin: true}

	t.Run("普通用户只能列出自己的记录", func(t *testing.T) {
		auditRecorder := serveAuditHandler(t, ownerUser, http.MethodGet, "/api/v1/audit-logs?current=1&size=20", "", handler.ListAuditLogs)
		var auditResponse struct {
			Data struct {
				Total int64           `json:"total"`
				Logs  []*AuditLogInfo `json:"logs"`
			} `json:"data"`
		}
		decodeAuditResponse(t, auditRecorder, http.StatusOK, &auditResponse)
		if auditResponse.Data.Total != 1 || len(auditResponse.Data.Logs) != 1 || auditResponse.Data.Logs[0].ResourceID != "execution-1" {
			t.Fatalf("unexpected owner audit response: %+v", auditResponse.Data)
		}

		commandRecorder := serveAuditHandler(t, ownerUser, http.MethodGet, "/api/v1/commands?current=1&size=20", "", handler.ListCommandLogs)
		var commandResponse struct {
			Data struct {
				Total    int64             `json:"total"`
				Commands []*CommandLogInfo `json:"commands"`
			} `json:"data"`
		}
		decodeAuditResponse(t, commandRecorder, http.StatusOK, &commandResponse)
		if commandResponse.Data.Total != 1 || len(commandResponse.Data.Commands) != 1 || commandResponse.Data.Commands[0].CommandID != "command-owner-one" {
			t.Fatalf("unexpected owner command response: %+v", commandResponse.Data)
		}
	})

	t.Run("普通用户不能读取他人或系统记录", func(t *testing.T) {
		for _, test := range []struct {
			name    string
			path    string
			id      uint
			handler gin.HandlerFunc
		}{
			{name: "other audit", path: fmt.Sprintf("/api/v1/audit-logs/%d", auditLogs[1].ID), id: auditLogs[1].ID, handler: handler.GetAuditLog},
			{name: "system audit", path: fmt.Sprintf("/api/v1/audit-logs/%d", auditLogs[2].ID), id: auditLogs[2].ID, handler: handler.GetAuditLog},
			{name: "other command", path: fmt.Sprintf("/api/v1/commands/%d", commandLogs[1].ID), id: commandLogs[1].ID, handler: handler.GetCommandLog},
			{name: "system command", path: fmt.Sprintf("/api/v1/commands/%d", commandLogs[2].ID), id: commandLogs[2].ID, handler: handler.GetCommandLog},
		} {
			t.Run(test.name, func(t *testing.T) {
				recorder := serveAuditHandler(t, ownerUser, http.MethodGet, test.path, fmt.Sprintf("%d", test.id), test.handler)
				if recorder.Code != http.StatusNotFound {
					t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
				}
			})
		}
	})

	t.Run("管理员可以列出和读取全部记录", func(t *testing.T) {
		auditRecorder := serveAuditHandler(t, adminUser, http.MethodGet, "/api/v1/audit-logs?current=1&size=20", "", handler.ListAuditLogs)
		var auditResponse struct {
			Data struct {
				Total int64 `json:"total"`
			} `json:"data"`
		}
		decodeAuditResponse(t, auditRecorder, http.StatusOK, &auditResponse)
		if auditResponse.Data.Total != 3 {
			t.Fatalf("expected three audit logs, got %d", auditResponse.Data.Total)
		}

		commandRecorder := serveAuditHandler(t, adminUser, http.MethodGet, "/api/v1/commands?current=1&size=20", "", handler.ListCommandLogs)
		var commandResponse struct {
			Data struct {
				Total int64 `json:"total"`
			} `json:"data"`
		}
		decodeAuditResponse(t, commandRecorder, http.StatusOK, &commandResponse)
		if commandResponse.Data.Total != 3 {
			t.Fatalf("expected three command logs, got %d", commandResponse.Data.Total)
		}

		systemAudit := serveAuditHandler(t, adminUser, http.MethodGet, fmt.Sprintf("/api/v1/audit-logs/%d", auditLogs[2].ID), fmt.Sprintf("%d", auditLogs[2].ID), handler.GetAuditLog)
		if systemAudit.Code != http.StatusOK {
			t.Fatalf("expected admin to read system audit log, got %d: %s", systemAudit.Code, systemAudit.Body.String())
		}
		systemCommand := serveAuditHandler(t, adminUser, http.MethodGet, fmt.Sprintf("/api/v1/commands/%d", commandLogs[2].ID), fmt.Sprintf("%d", commandLogs[2].ID), handler.GetCommandLog)
		if systemCommand.Code != http.StatusOK {
			t.Fatalf("expected admin to read system command log, got %d: %s", systemCommand.Code, systemCommand.Body.String())
		}
	})
}

func serveAuditHandler(t *testing.T, user *auth.User, method, target, id string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	if id != "" {
		c.Params = gin.Params{{Key: "id", Value: id}}
	}
	auth.SetUserToContext(c, user)
	handler(c)
	return recorder
}

func decodeAuditResponse(t *testing.T, recorder *httptest.ResponseRecorder, expectedStatus int, target any) {
	t.Helper()
	if recorder.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d: %s", expectedStatus, recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
