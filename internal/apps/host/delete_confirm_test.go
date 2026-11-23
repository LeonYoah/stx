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

package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/apps/cluster"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newHostDeleteConfirmEnv(t *testing.T) (*gorm.DB, *Service, *Handler, *auth.User, *Host, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:host-delete-confirm-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.AutoMigrate(
		&Host{}, &cluster.Cluster{}, &cluster.ClusterNode{}, &auth.User{},
		&executionapp.Execution{}, &executionapp.Confirmation{}, &audit.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &auth.User{Username: "admin", IsActive: true, IsAdmin: true}
	if err := user.SetPassword("secret", 4); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	clusterRepo := cluster.NewRepository(database)
	svc := NewService(NewRepository(database), clusterRepo, nil)
	execSvc := executionapp.NewService(executionapp.NewRepository(database), executionapp.NewProviderRegistry())
	execSvc.SetAuditRepository(audit.NewRepository(database))
	handler := NewHandler(svc, audit.NewRepository(database))
	handler.SetExecutionService(execSvc)

	host, err := svc.Create(t.Context(), &CreateHostRequest{Name: "idle-host", IPAddress: "10.0.0.50"})
	if err != nil {
		t.Fatalf("create host: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetUserToContext(c, user)
		c.Next()
	})
	router.DELETE("/hosts/:id", handler.DeleteHost)
	return database, svc, handler, user, host, router
}

func performHostDeleteRequest(router http.Handler, path, idempotencyKey, confirmationID string, confirmed bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, path, bytes.NewBuffer(nil))
	request.Header.Set("X-STX-Client", "web")
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

func TestDeleteHostRequiresR2Confirmation(t *testing.T) {
	_, svc, _, _, host, router := newHostDeleteConfirmEnv(t)
	path := fmt.Sprintf("/hosts/%d", host.ID)

	first := performHostDeleteRequest(router, path, "host-del-1", "", true)
	if first.Code != http.StatusPreconditionRequired {
		t.Fatalf("first delete should require confirmation: status=%d body=%s", first.Code, first.Body.String())
	}
	var payload struct {
		Data struct {
			ConfirmationID string `json:"confirmation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &payload); err != nil || payload.Data.ConfirmationID == "" {
		t.Fatalf("confirmation payload: err=%v body=%s", err, first.Body.String())
	}
	if _, err := svc.Get(t.Context(), host.ID); err != nil {
		t.Fatalf("host must remain after challenge: %v", err)
	}

	second := performHostDeleteRequest(router, path, "host-del-1", payload.Data.ConfirmationID, true)
	if second.Code != http.StatusOK {
		t.Fatalf("confirmed delete failed: status=%d body=%s", second.Code, second.Body.String())
	}
	if _, err := svc.Get(t.Context(), host.ID); err == nil {
		t.Fatal("host should be deleted after confirmed request")
	}
}

func TestDeleteHostPropagatesClusterAssociationAfterConfirm(t *testing.T) {
	database, svc, _, _, host, router := newHostDeleteConfirmEnv(t)
	clusterRepo := cluster.NewRepository(database)
	clusterObj := &cluster.Cluster{Name: "bound", DeploymentMode: cluster.DeploymentModeHybrid}
	if err := clusterRepo.Create(t.Context(), clusterObj); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := clusterRepo.AddNode(t.Context(), &cluster.ClusterNode{
		ClusterID: clusterObj.ID, HostID: host.ID, Role: cluster.NodeRoleMaster, Status: cluster.NodeStatusStopped,
	}); err != nil {
		t.Fatalf("add node: %v", err)
	}

	path := fmt.Sprintf("/hosts/%d", host.ID)
	first := performHostDeleteRequest(router, path, "host-del-bound", "", true)
	var payload struct {
		Data struct {
			ConfirmationID string `json:"confirmation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &payload); err != nil || payload.Data.ConfirmationID == "" {
		t.Fatalf("confirmation payload: err=%v body=%s", err, first.Body.String())
	}
	second := performHostDeleteRequest(router, path, "host-del-bound", payload.Data.ConfirmationID, true)
	if second.Code != http.StatusConflict {
		t.Fatalf("associated host must stay conflict after confirm: status=%d body=%s", second.Code, second.Body.String())
	}
	if _, err := svc.Get(t.Context(), host.ID); err != nil {
		t.Fatalf("host must remain: %v", err)
	}
}
