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

package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMonitoringAuditMiddlewareRecordsSuccessfulCLIWrite(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&audit.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	repo := audit.NewRepository(database)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetUserToContext(c, &auth.User{ID: 7, Username: "admin"})
		c.Next()
	})
	router.Use(monitoringAuditMiddleware(repo))
	router.PUT("/api/v1/monitoring/notification-routes/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": 9}})
	})

	request := httptest.NewRequest(http.MethodPut, "/api/v1/monitoring/notification-routes/9", nil)
	request.Header.Set("X-STX-Client", "cli")
	request.Header.Set("X-Request-ID", "request-monitoring-audit")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected response status: %d", response.Code)
	}

	logs, total, err := repo.ListAuditLogs(request.Context(), &audit.AuditLogFilter{RequestID: "request-monitoring-audit", IncludeAll: true})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected one monitoring audit row, total=%d logs=%+v", total, logs)
	}
	item := logs[0]
	if item.Action != "update" || item.ResourceType != "monitoring_notification_route" || item.ResourceID != "9" || item.ClientType != "cli" || item.UserID == nil || *item.UserID != 7 {
		t.Fatalf("unexpected monitoring audit row: %+v", item)
	}
	if item.Details["operation_id"] != "monitoring.notification-route.update" || item.ResultStatus != "succeeded" || item.RiskLevel != "R1" {
		t.Fatalf("monitoring audit links are incomplete: %+v", item)
	}
}

func TestMonitoringAuditMiddlewareSkipsFailedWrite(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&audit.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	repo := audit.NewRepository(database)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(monitoringAuditMiddleware(repo))
	router.POST("/api/v1/monitoring/alert-policies", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "invalid"})
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/alert-policies", nil))
	logs, total, err := repo.ListAuditLogs(t.Context(), &audit.AuditLogFilter{IncludeAll: true})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("failed monitoring write must not be recorded as success: total=%d logs=%+v", total, logs)
	}
}
