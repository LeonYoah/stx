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
	"context"
	"strings"
	"testing"
	"time"

	agentapp "github.com/LeonYoah/stx/internal/apps/agent"
	"github.com/LeonYoah/stx/internal/apps/audit"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAgentCommandSenderAdapterRecordsOwnerExecutionAndRedactsSecrets(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&audit.CommandLog{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}
	repo := audit.NewRepository(database)
	adapter := &agentCommandSenderAdapter{auditRepo: repo}
	ctx := audit.WithCommandMetadata(context.Background(), audit.CommandMetadata{
		RequestID:     "request-1",
		ExecutionID:   "11111111-1111-1111-1111-111111111111",
		OwnerUserID:   7,
		OwnerUsername: "admin",
		ClientType:    "cli",
	})
	startedAt := time.Now().Add(-time.Second)
	adapter.recordCommandLog(ctx, "agent-1", "jvm_dump", map[string]string{
		"password": "command-secret",
		"path":     "/tmp/output.hprof",
	}, startedAt, &pb.CommandResponse{
		CommandId: "command-1",
		Status:    pb.CommandStatus_SUCCESS,
		Progress:  100,
		Output:    "token=output-secret",
	})

	commandLog, err := repo.GetCommandLogByCommandID(ctx, "command-1")
	if err != nil {
		t.Fatalf("get command log: %v", err)
	}
	if commandLog.CreatedBy == nil || *commandLog.CreatedBy != 7 || commandLog.ExecutionID != "11111111-1111-1111-1111-111111111111" || commandLog.RequestID != "request-1" {
		t.Fatalf("unexpected command linkage: %+v", commandLog)
	}
	if commandLog.Parameters["password"] != "******" || strings.Contains(commandLog.Output, "output-secret") {
		t.Fatalf("expected command secrets to be redacted, got parameters=%#v output=%q", commandLog.Parameters, commandLog.Output)
	}

	logs, total, err := repo.ListAuditLogs(ctx, &audit.AuditLogFilter{CommandID: "command-1", IncludeAll: true})
	if err != nil {
		t.Fatalf("list command audit logs: %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("agent command should stay in command logs, not audit rows, total=%d len=%d", total, len(logs))
	}

	linked, linkedTotal, err := repo.ListCommandLogs(ctx, &audit.CommandLogFilter{RequestID: "request-1", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list by request id: %v", err)
	}
	if linkedTotal != 1 || len(linked) != 1 || linked[0].CommandID != "command-1" {
		t.Fatalf("command should be queryable by request id: total=%d logs=%+v", linkedTotal, linked)
	}
}

func TestAgentCommandSenderAdapterOmitsCheckProcessFromAuditLogs(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&audit.CommandLog{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}
	repo := audit.NewRepository(database)
	adapter := &agentCommandSenderAdapter{auditRepo: repo}
	startedAt := time.Now().Add(-time.Second)
	adapter.recordCommandLog(context.Background(), "agent-1", "check_process", map[string]string{
		"role":        "hybrid",
		"sub_command": "check_process",
	}, startedAt, &pb.CommandResponse{
		CommandId: "check-process-command-1",
		Status:    pb.CommandStatus_SUCCESS,
		Progress:  100,
		Output:    `{"success":true,"details":{"pid":"92322"}}`,
	})

	if _, err := repo.GetCommandLogByCommandID(context.Background(), "check-process-command-1"); err != nil {
		t.Fatalf("check_process should remain in technical command logs: %v", err)
	}
	logs, total, err := repo.ListAuditLogs(context.Background(), &audit.AuditLogFilter{
		CommandID:  "check-process-command-1",
		IncludeAll: true,
	})
	if err != nil {
		t.Fatalf("list check_process audit logs: %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("check_process should not create audit logs, total=%d len=%d", total, len(logs))
	}
}

func TestAgentCommandWithoutOwnerIsRecordedAsAgent(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&audit.CommandLog{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}
	repo := audit.NewRepository(database)
	adapter := &agentCommandSenderAdapter{auditRepo: repo}
	adapter.recordCommandLog(context.Background(), "agent-1", "jvm_dump", nil, time.Now(), &pb.CommandResponse{
		CommandId: "command-agent",
		Status:    pb.CommandStatus_SUCCESS,
	})

	logs, _, err := repo.ListAuditLogs(context.Background(), &audit.AuditLogFilter{Username: "agent", IncludeAll: true})
	if err != nil {
		t.Fatalf("list agent audit logs: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("unowned agent command should not become an audit row: %+v", logs)
	}
	if _, err := repo.GetCommandLogByCommandID(context.Background(), "command-agent"); err != nil {
		t.Fatalf("command log should still exist: %v", err)
	}
}

func TestAgentCommandSendFailureUpdatesPendingAuditRecord(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&audit.CommandLog{}); err != nil {
		t.Fatalf("migrate command logs: %v", err)
	}
	repo := audit.NewRepository(database)
	adapter := &agentCommandSenderAdapter{manager: agentapp.NewManager(nil), auditRepo: repo}
	ctx := audit.WithCommandMetadata(context.Background(), audit.CommandMetadata{
		RequestID:   "request-send-failed",
		ExecutionID: "33333333-3333-3333-3333-333333333333",
		OwnerUserID: 9,
		ClientType:  "cli",
	})

	if _, _, err := adapter.SendCommand(ctx, "missing-agent", "thread_dump", map[string]string{"install_dir": "/tmp/st", "role": "master"}); err == nil {
		t.Fatal("missing agent should fail")
	}
	logs, total, err := repo.ListCommandLogs(ctx, &audit.CommandLogFilter{RequestID: "request-send-failed", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list failed command logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("send failure should keep one command log: total=%d logs=%+v", total, logs)
	}
	item := logs[0]
	if item.Status != audit.CommandStatusFailed || item.FinishedAt == nil || item.ClientType != "cli" || item.ExecutionID == "" || item.Error == "" {
		t.Fatalf("failed command audit record is incomplete: %+v", item)
	}
	if !strings.Contains(item.DisplayCommand, "agent:thread_dump") {
		t.Fatalf("failed command should retain a safe display command: %q", item.DisplayCommand)
	}
}

func TestBuildAgentDisplayCommandUsesActualJavaTool(t *testing.T) {
	threadCommand := buildAgentDisplayCommand("thread_dump", nil, `{"tool":"jstack","pid":321}`)
	if threadCommand != "jstack -l 321" {
		t.Fatalf("unexpected thread dump command: %q", threadCommand)
	}
	heapCommand := buildAgentDisplayCommand("jvm_dump", nil, `{"tool":"jcmd","pid":654,"output_path":"/tmp/heap dump.hprof"}`)
	if heapCommand != `jcmd 654 GC.heap_dump "/tmp/heap dump.hprof"` {
		t.Fatalf("unexpected JVM dump command: %q", heapCommand)
	}
}
