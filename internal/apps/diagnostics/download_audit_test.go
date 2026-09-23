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
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRecordDiagnosticResourceAccessLinksUserExecutionAndFileType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&audit.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	repo := audit.NewRepository(database)
	handler := &Handler{auditRepo: repo}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/api/v1/diagnostics/tasks/8/files/logs/seatunnel.log", nil)
	c.Request.Header.Set("X-Request-ID", "request-download-1")
	c.Request.Header.Set("X-STX-Client", "cli")
	auth.SetUserToContext(c, &auth.User{ID: 17, Username: "operator", IsActive: true})
	task := &DiagnosticTask{ID: 8, ExecutionID: "22222222-2222-2222-2222-222222222222", CreatedBy: 17}

	if err := handler.recordDiagnosticResourceAccess(c, task, "diagnostics.file.read", "log", "logs/seatunnel.log"); err != nil {
		t.Fatalf("record diagnostic resource access: %v", err)
	}
	logs, total, err := repo.ListAuditLogs(c.Request.Context(), &audit.AuditLogFilter{ExecutionID: task.ExecutionID, IncludeAll: true})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected one audit log, total=%d len=%d", total, len(logs))
	}
	log := logs[0]
	if log.UserID == nil || *log.UserID != 17 || log.RequestID != "request-download-1" || log.ClientType != "cli" || log.ExecutionID != task.ExecutionID {
		t.Fatalf("unexpected audit linkage: %+v", log)
	}
	if log.Details["file_type"] != "log" || log.Details["relative_path"] != "logs/seatunnel.log" {
		t.Fatalf("unexpected audit details: %#v", log.Details)
	}
	if _, exists := log.Details["content"]; exists {
		t.Fatalf("audit details must not contain file content: %#v", log.Details)
	}
}
