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

package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type deleteConfirmEnv struct {
	db      *gorm.DB
	svc     *Service
	handler *Handler
	user    *auth.User
	execSvc *executionapp.Service
	cluster *Cluster
	node    *ClusterNode
	router  *gin.Engine
}

func newDeleteConfirmEnv(t *testing.T) *deleteConfirmEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:cluster-delete-confirm-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.AutoMigrate(
		&Cluster{}, &ClusterNode{}, &auth.User{},
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
	repo := NewRepository(database)
	svc := NewService(repo, NewMockHostProvider(), nil)
	execSvc := executionapp.NewService(executionapp.NewRepository(database), executionapp.NewProviderRegistry())
	execSvc.SetAuditRepository(audit.NewRepository(database))
	handler := NewHandler(svc, audit.NewRepository(database))
	handler.SetExecutionService(execSvc)

	cluster, err := svc.Create(t.Context(), &CreateClusterRequest{
		Name: "delete-confirm-" + t.Name(), DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := svc.UpdateStatus(t.Context(), cluster.ID, ClusterStatusStopped); err != nil {
		t.Fatalf("stop cluster: %v", err)
	}
	node := &ClusterNode{
		ClusterID: cluster.ID, HostID: 1, Role: NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel", HazelcastPort: 5801, Status: NodeStatusStopped,
	}
	if err := repo.AddNode(t.Context(), node); err != nil {
		t.Fatalf("add node: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetUserToContext(c, user)
		c.Next()
	})
	router.DELETE("/clusters/:id", handler.DeleteCluster)
	router.DELETE("/clusters/:id/nodes/:nodeId", handler.RemoveNode)

	return &deleteConfirmEnv{
		db: database, svc: svc, handler: handler, user: user, execSvc: execSvc,
		cluster: cluster, node: node, router: router,
	}
}

func performClusterDeleteRequest(router http.Handler, method, path, idempotencyKey, confirmationID string, confirmed bool, clientType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBuffer(nil))
	if clientType == "" {
		clientType = "web"
	}
	request.Header.Set("X-STX-Client", clientType)
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

func parseConfirmationID(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Data struct {
			ConfirmationID string `json:"confirmation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Data.ConfirmationID == "" {
		t.Fatalf("confirmation payload: err=%v body=%s", err, string(body))
	}
	return payload.Data.ConfirmationID
}

func TestDeleteClusterRequiresR2ConfirmationForWeb(t *testing.T) {
	env := newDeleteConfirmEnv(t)
	path := fmt.Sprintf("/clusters/%d", env.cluster.ID)

	missingKey := performClusterDeleteRequest(env.router, http.MethodDelete, path, "", "", true, "web")
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key: status=%d body=%s", missingKey.Code, missingKey.Body.String())
	}

	first := performClusterDeleteRequest(env.router, http.MethodDelete, path, "cluster-del-1", "", true, "web")
	if first.Code != http.StatusPreconditionRequired {
		t.Fatalf("first delete should require confirmation: status=%d body=%s", first.Code, first.Body.String())
	}
	confirmationID := parseConfirmationID(t, first.Body.Bytes())
	if _, err := env.svc.Get(t.Context(), env.cluster.ID); err != nil {
		t.Fatalf("cluster must remain after confirmation challenge: %v", err)
	}

	// 取消：拿到 confirmation_id 后不发第二次请求，资源仍在。
	// Cancel: after receiving confirmation_id, skipping the second request leaves the resource.
	if _, err := env.svc.Get(t.Context(), env.cluster.ID); err != nil {
		t.Fatalf("cancel must not delete: %v", err)
	}

	wrongTarget := performClusterDeleteRequest(env.router, http.MethodDelete,
		fmt.Sprintf("/clusters/%d", env.cluster.ID+999), "cluster-del-1", confirmationID, true, "web")
	if wrongTarget.Code != http.StatusConflict {
		t.Fatalf("confirmation bound to request hash must reject mismatched target: status=%d body=%s", wrongTarget.Code, wrongTarget.Body.String())
	}

	second := performClusterDeleteRequest(env.router, http.MethodDelete, path, "cluster-del-1", confirmationID, true, "web")
	if second.Code != http.StatusOK {
		t.Fatalf("confirmed delete failed: status=%d body=%s", second.Code, second.Body.String())
	}
	if _, err := env.svc.Get(t.Context(), env.cluster.ID); err == nil {
		t.Fatal("cluster should be deleted after confirmed request")
	}

	retry := performClusterDeleteRequest(env.router, http.MethodDelete, path, "cluster-del-1", confirmationID, true, "web")
	if retry.Code != http.StatusOK {
		t.Fatalf("idempotent retry of successful delete: status=%d body=%s", retry.Code, retry.Body.String())
	}
}

func TestDeleteClusterRejectsExpiredOrReusedConfirmation(t *testing.T) {
	env := newDeleteConfirmEnv(t)
	path := fmt.Sprintf("/clusters/%d", env.cluster.ID)

	first := performClusterDeleteRequest(env.router, http.MethodDelete, path, "cluster-del-expire", "", true, "cli")
	if first.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected confirmation: status=%d body=%s", first.Code, first.Body.String())
	}
	confirmationID := parseConfirmationID(t, first.Body.Bytes())

	var row executionapp.Confirmation
	if err := env.db.Where("owner_user_id = ?", env.user.ID).Order("id desc").First(&row).Error; err != nil {
		t.Fatalf("load confirmation: %v", err)
	}
	expired := time.Now().Add(-time.Minute)
	if err := env.db.Model(&row).Update("expires_at", expired).Error; err != nil {
		t.Fatalf("expire confirmation: %v", err)
	}

	expiredResp := performClusterDeleteRequest(env.router, http.MethodDelete, path, "cluster-del-expire", confirmationID, true, "cli")
	if expiredResp.Code != http.StatusConflict {
		t.Fatalf("expired confirmation must be rejected: status=%d body=%s", expiredResp.Code, expiredResp.Body.String())
	}
	if _, err := env.svc.Get(t.Context(), env.cluster.ID); err != nil {
		t.Fatalf("expired confirmation must not delete: %v", err)
	}
}

func TestRemoveNodeRequiresR2Confirmation(t *testing.T) {
	env := newDeleteConfirmEnv(t)
	path := fmt.Sprintf("/clusters/%d/nodes/%d", env.cluster.ID, env.node.ID)

	first := performClusterDeleteRequest(env.router, http.MethodDelete, path, "node-rm-1", "", true, "web")
	if first.Code != http.StatusPreconditionRequired {
		t.Fatalf("first remove should require confirmation: status=%d body=%s", first.Code, first.Body.String())
	}
	confirmationID := parseConfirmationID(t, first.Body.Bytes())
	if _, err := env.svc.repo.GetNodeByID(t.Context(), env.node.ID); err != nil {
		t.Fatalf("node must remain after challenge: %v", err)
	}

	second := performClusterDeleteRequest(env.router, http.MethodDelete, path, "node-rm-1", confirmationID, true, "web")
	if second.Code != http.StatusOK {
		t.Fatalf("confirmed remove failed: status=%d body=%s", second.Code, second.Body.String())
	}
	if _, err := env.svc.repo.GetNodeByID(t.Context(), env.node.ID); err == nil {
		t.Fatal("node should be removed after confirmed request")
	}
}

func TestDeleteClusterPropagatesRunningConflictAfterConfirm(t *testing.T) {
	env := newDeleteConfirmEnv(t)
	// 去掉节点，避免拓扑钩子按节点状态把集群写回 stopped。
	// Drop nodes so topology hooks cannot rewrite the cluster back to stopped from node status.
	if err := env.svc.repo.RemoveNode(t.Context(), env.node.ID); err != nil {
		t.Fatalf("remove fixture node: %v", err)
	}
	if err := env.svc.repo.UpdateStatus(t.Context(), env.cluster.ID, ClusterStatusRunning); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	path := fmt.Sprintf("/clusters/%d", env.cluster.ID)

	first := performClusterDeleteRequest(env.router, http.MethodDelete, path, "cluster-del-running", "", true, "web")
	confirmationID := parseConfirmationID(t, first.Body.Bytes())
	second := performClusterDeleteRequest(env.router, http.MethodDelete, path, "cluster-del-running", confirmationID, true, "web")
	if second.Code != http.StatusConflict {
		t.Fatalf("running cluster delete must stay conflict after confirm: status=%d body=%s", second.Code, second.Body.String())
	}
	if _, err := env.svc.Get(t.Context(), env.cluster.ID); err != nil {
		t.Fatalf("running cluster must remain: %v", err)
	}
}
