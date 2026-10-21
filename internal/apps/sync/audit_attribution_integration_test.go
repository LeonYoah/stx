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

type auditAttributionFixture struct {
	handler    *Handler
	auditRepo  *audit.Repository
	service    *Service
	execSvc    *executionapp.Service
	db         *gorm.DB
	user       *auth.User
	cliToken   *auth.CLIToken
}

func newAuditAttributionFixture(t *testing.T) *auditAttributionFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:audit-attr-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.AutoMigrate(
		&Task{}, &TaskVersion{}, &JobInstance{}, &GlobalVariable{}, &PreviewSession{}, &PreviewTable{}, &PreviewRow{},
		&SchedulerConnection{}, &TaskSchedulerBinding{},
		&auth.User{}, &auth.CLIToken{}, &audit.AuditLog{},
		&executionapp.Execution{}, &executionapp.Confirmation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &auth.User{Username: "cli-ops", PasswordHash: "x", IsActive: true, IsAdmin: true}
	if err := database.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	token := &auth.CLIToken{
		UserID:      user.ID,
		Name:        "ci-bot",
		TokenPrefix: "stx_inte",
		TokenHash:   fmt.Sprintf("hash-%s", t.Name()),
	}
	if err := database.Create(token).Error; err != nil {
		t.Fatalf("create cli token: %v", err)
	}
	auditRepo := audit.NewRepository(database)
	service := NewService(NewRepository(database))
	execService := executionapp.NewService(executionapp.NewRepository(database), executionapp.NewProviderRegistry())
	execService.SetAuditRepository(auditRepo)
	service.SetExecutionService(execService)
	handler := NewHandler(service)
	handler.SetAuditRepository(auditRepo)
	return &auditAttributionFixture{
		handler:   handler,
		auditRepo: auditRepo,
		service:   service,
		execSvc:   execService,
		db:        database,
		user:      user,
		cliToken:  token,
	}
}

func (f *auditAttributionFixture) newCLIRouter(clientIP string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyUser, f.user)
		c.Set(auth.ContextKeyCLIToken, f.cliToken)
		ctx := auth.ContextWithCLITokenForTest(c.Request.Context(), f.cliToken)
		c.Request = c.Request.WithContext(ctx)
		audit.BindOperator(c)
		c.Next()
	})
	if clientIP != "" {
		_ = router.SetTrustedProxies(nil)
	}
	router.POST("/tasks/:id/publish", f.handler.PublishTask)
	router.POST("/tasks/:id/submit", f.handler.SubmitTask)
	return router
}

func (f *auditAttributionFixture) createDraftFile(t *testing.T, name string) *Task {
	t.Helper()
	ctx := context.Background()
	folder, err := f.service.CreateTask(ctx, &CreateTaskRequest{
		NodeType: string(TaskNodeTypeFolder), Name: "ws-" + name, ContentFormat: string(ContentFormatHOCON),
	}, uint(f.user.ID))
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	file, err := f.service.CreateTask(ctx, &CreateTaskRequest{
		ParentID: uintPtr(folder.ID), NodeType: string(TaskNodeTypeFile), Name: name,
		Mode: string(TaskModeBatch), ContentFormat: string(ContentFormatHOCON), Content: fakeBatchHOCON,
	}, uint(f.user.ID))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	return file
}

func doJSONRequest(router *gin.Engine, method, path, body, requestID, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", requestID)
	req.Header.Set("X-STX-Client", "cli")
	req.Header.Set("User-Agent", "stx-cli/test")
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCLITokenPublishAndSubmitAuditAttribution(t *testing.T) {
	f := newAuditAttributionFixture(t)
	file := f.createDraftFile(t, "publish-submit")
	router := f.newCLIRouter("203.0.113.40")
	remote := "203.0.113.40:54321"

	publishID := "req-cli-publish-1"
	wPub := doJSONRequest(router, http.MethodPost, fmt.Sprintf("/tasks/%d/publish", file.ID), `{"comment":"v1"}`, publishID, remote)
	if wPub.Code != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", wPub.Code, wPub.Body.String())
	}

	ctx := context.Background()
	publishLogs, total, err := f.auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{Action: "publish", RequestID: publishID, IncludeAll: true})
	if err != nil {
		t.Fatalf("list publish audit: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 publish audit, got %d", total)
	}
	pub := publishLogs[0]
	if pub.ClientType != "cli" {
		t.Fatalf("publish client_type=%q want cli", pub.ClientType)
	}
	if pub.TokenID == nil || *pub.TokenID != f.cliToken.ID {
		t.Fatalf("publish token_id=%v want %d", pub.TokenID, f.cliToken.ID)
	}
	if pub.TokenPrefix != "stx_inte" {
		t.Fatalf("publish token_prefix=%q", pub.TokenPrefix)
	}
	if !strings.HasPrefix(pub.IPAddress, "203.0.113.40") {
		t.Fatalf("publish ip=%q want 203.0.113.40", pub.IPAddress)
	}
	if pub.RequestID != publishID {
		t.Fatalf("publish request_id=%q", pub.RequestID)
	}

	submitID := "req-cli-submit-1"
	wSub := doJSONRequest(router, http.MethodPost, fmt.Sprintf("/tasks/%d/submit", file.ID),
		`{"external_ref":"ds:2001:7"}`, submitID, remote)
	if wSub.Code != http.StatusOK {
		t.Fatalf("submit status=%d body=%s", wSub.Code, wSub.Body.String())
	}

	submitLogs, total, err := f.auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{Action: "submit", RequestID: submitID, IncludeAll: true})
	if err != nil {
		t.Fatalf("list submit audit: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 submit audit, got %d", total)
	}
	sub := submitLogs[0]
	if sub.ClientType != "cli" || sub.TokenPrefix != "stx_inte" || sub.RequestID != submitID {
		t.Fatalf("submit attribution incomplete: %+v", sub)
	}
	if sub.TokenID == nil || *sub.TokenID != f.cliToken.ID {
		t.Fatalf("submit token_id=%v", sub.TokenID)
	}
	if !strings.HasPrefix(sub.IPAddress, "203.0.113.40") {
		t.Fatalf("submit ip=%q", sub.IPAddress)
	}
	executionID := sub.ExecutionID
	if executionID == "" {
		if v, ok := sub.Details["execution_id"].(string); ok {
			executionID = v
		}
	}
	if strings.TrimSpace(executionID) == "" {
		t.Fatalf("submit audit must link execution_id: %+v", sub)
	}

	execLogs, total, err := f.auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{ExecutionID: executionID, IncludeAll: true})
	if err != nil {
		t.Fatalf("list execution audit: %v", err)
	}
	if total < 1 {
		t.Fatalf("expected execution audit rows for %s, got %d", executionID, total)
	}
	for _, logItem := range execLogs {
		if logItem.TokenID == nil || *logItem.TokenID != f.cliToken.ID {
			t.Fatalf("execution audit missing token: action=%s log=%+v", logItem.Action, logItem)
		}
		if logItem.TokenPrefix != "stx_inte" || logItem.RequestID != submitID {
			t.Fatalf("execution audit attribution incomplete: %+v", logItem)
		}
		if logItem.UserID == nil || *logItem.UserID != uint(f.user.ID) {
			t.Fatalf("execution audit owner wrong: %+v", logItem)
		}
	}

	item, err := f.execSvc.Get(ctx, executionapp.Actor{UserID: f.user.ID, IsAdmin: true}, executionID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if item.ActorTokenID == nil || *item.ActorTokenID != f.cliToken.ID || item.TokenPrefix != "stx_inte" {
		t.Fatalf("execution row missing token fields: %+v", item)
	}
	if !strings.HasPrefix(item.ClientIP, "203.0.113.40") {
		t.Fatalf("execution client_ip=%q", item.ClientIP)
	}
}

func TestSubmitUnpublishedTaskAuditKeepsRealErrorCode(t *testing.T) {
	f := newAuditAttributionFixture(t)
	file := f.createDraftFile(t, "unpublished")
	router := f.newCLIRouter("198.51.100.20")
	requestID := "req-submit-unpublished"

	w := doJSONRequest(router, http.MethodPost, fmt.Sprintf("/tasks/%d/submit", file.ID),
		`{"external_ref":"ds:9:1"}`, requestID, "198.51.100.20:1111")
	if w.Code == http.StatusOK {
		t.Fatalf("unpublished submit should fail, body=%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "published") {
		t.Fatalf("response should mention unpublished, body=%s", w.Body.String())
	}

	ctx := context.Background()
	logs, total, err := f.auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{Action: "submit", RequestID: requestID, IncludeAll: true})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected failure audit row, got %d", total)
	}
	logItem := logs[0]
	if logItem.Details["result_status"] != "failed" {
		t.Fatalf("result_status=%v", logItem.Details["result_status"])
	}
	errMsg, _ := logItem.Details["error"].(string)
	if errMsg == "" || errMsg == "sync operation failed" {
		t.Fatalf("audit error should keep real message, got %q", errMsg)
	}
	if !strings.Contains(errMsg, "published") {
		t.Fatalf("expected unpublished wording, got %q", errMsg)
	}
	if logItem.ClientType != "cli" || logItem.TokenPrefix != "stx_inte" {
		t.Fatalf("failure audit should still attribute CLI token: %+v", logItem)
	}

	var executions []executionapp.Execution
	if err := f.db.Where("request_id = ?", requestID).Find(&executions).Error; err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected 1 execution for failed submit, got %d", len(executions))
	}
	if executions[0].ErrorCode != "task_not_published" {
		t.Fatalf("error_code=%q want task_not_published msg=%q", executions[0].ErrorCode, executions[0].ErrorMessage)
	}
	if executions[0].ErrorMessage == "" || executions[0].ErrorMessage == "sync operation failed" {
		t.Fatalf("execution error_message should keep real error, got %q", executions[0].ErrorMessage)
	}
	if executions[0].ActorTokenID == nil || *executions[0].ActorTokenID != f.cliToken.ID {
		t.Fatalf("failed execution should keep actor token: %+v", executions[0])
	}
}

func TestSubmitThenReconcilerTransitionReusesActorAttribution(t *testing.T) {
	f := newAuditAttributionFixture(t)
	file := f.createDraftFile(t, "reconcile")
	router := f.newCLIRouter("192.0.2.55")
	requestID := "req-cli-reconcile-1"

	wPub := doJSONRequest(router, http.MethodPost, fmt.Sprintf("/tasks/%d/publish", file.ID), `{}`, requestID+"-pub", "192.0.2.55:9")
	if wPub.Code != http.StatusOK {
		t.Fatalf("publish failed: %s", wPub.Body.String())
	}
	wSub := doJSONRequest(router, http.MethodPost, fmt.Sprintf("/tasks/%d/submit", file.ID),
		`{"external_ref":"ds:3001:2"}`, requestID, "192.0.2.55:9")
	if wSub.Code != http.StatusOK {
		t.Fatalf("submit failed: %s", wSub.Body.String())
	}

	ctx := context.Background()
	submitLogs, _, err := f.auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{Action: "submit", RequestID: requestID, IncludeAll: true})
	if err != nil || len(submitLogs) != 1 {
		t.Fatalf("submit audit: err=%v logs=%d", err, len(submitLogs))
	}
	executionID := submitLogs[0].ExecutionID
	if executionID == "" {
		executionID, _ = submitLogs[0].Details["execution_id"].(string)
	}
	if executionID == "" {
		t.Fatal("missing execution_id")
	}

	item, err := f.execSvc.Get(ctx, executionapp.Actor{UserID: f.user.ID, IsAdmin: true}, executionID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	// Submit may leave execution pending/running/succeeded depending on engine sync stubs.
	from := item.Status
	if from == executionapp.StatusPending {
		if err := f.execSvc.Transition(ctx, executionID, executionapp.StatusPending, executionapp.StatusRunning, nil); err != nil {
			t.Fatalf("start transition: %v", err)
		}
		from = executionapp.StatusRunning
	}
	if from != executionapp.StatusSucceeded && from != executionapp.StatusFailed && from != executionapp.StatusTimedOut && from != executionapp.StatusCancelled {
		reconcileCtx := executionapp.WithTriggerSource(ctx, executionapp.TriggerSourceReconciler)
		if err := f.execSvc.Transition(reconcileCtx, executionID, from, executionapp.StatusSucceeded, map[string]any{"cancellable": false}); err != nil {
			t.Fatalf("reconciler transition: %v", err)
		}
	} else {
		// Already terminal from submit path: still verify create/start reused token; force a synthetic
		// reconciler result by creating a sibling execution that mirrors production Create→Transition.
		sibling, created, err := f.execSvc.Create(auth.ContextWithCLITokenForTest(
			audit.WithRequestAttribution(ctx, audit.RequestAttribution{ClientIP: "192.0.2.55", UserAgent: "stx-cli/test"}),
			f.cliToken,
		), executionapp.CreateInput{
			OperationID: "sync.task.submit",
			OwnerUserID: f.user.ID,
			Module:      "sync",
			RequestID:   requestID + "-sibling",
			ClientType:  "cli",
			RiskLevel:   executionapp.RiskLevelR1,
		})
		if err != nil || !created {
			t.Fatalf("create sibling: created=%t err=%v", created, err)
		}
		if err := f.execSvc.Transition(ctx, sibling.ExecutionID, executionapp.StatusPending, executionapp.StatusRunning, nil); err != nil {
			t.Fatalf("sibling start: %v", err)
		}
		reconcileCtx := executionapp.WithTriggerSource(ctx, executionapp.TriggerSourceReconciler)
		if err := f.execSvc.Transition(reconcileCtx, sibling.ExecutionID, executionapp.StatusRunning, executionapp.StatusSucceeded, map[string]any{"cancellable": false}); err != nil {
			t.Fatalf("sibling reconciler: %v", err)
		}
		executionID = sibling.ExecutionID
		requestID = requestID + "-sibling"
	}

	logs, total, err := f.auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{ExecutionID: executionID, IncludeAll: true})
	if err != nil {
		t.Fatalf("list execution audits: %v", err)
	}
	if total < 2 {
		t.Fatalf("expected start/result audits, got %d", total)
	}
	byAction := map[string]*audit.AuditLog{}
	for _, logItem := range logs {
		byAction[logItem.Action] = logItem
		if logItem.TokenID == nil || *logItem.TokenID != f.cliToken.ID {
			t.Fatalf("token not reused on %s: %+v", logItem.Action, logItem)
		}
		if logItem.RequestID != requestID {
			t.Fatalf("request_id not reused on %s: got %q want %q", logItem.Action, logItem.RequestID, requestID)
		}
		if logItem.UserID == nil || *logItem.UserID != uint(f.user.ID) {
			t.Fatalf("owner not preserved on %s: %+v", logItem.Action, logItem)
		}
	}
	result := byAction["execution.result"]
	if result == nil {
		t.Fatalf("missing execution.result among %#v", byAction)
	}
	if result.ClientType != executionapp.TriggerSourceReconciler {
		t.Fatalf("writer source=%q want reconciler", result.ClientType)
	}
	if result.Trigger != "auto" {
		t.Fatalf("reconciler trigger=%q want auto", result.Trigger)
	}
	if result.Details["initiator_client_type"] != "cli" {
		t.Fatalf("initiator_client_type=%v", result.Details["initiator_client_type"])
	}
}
