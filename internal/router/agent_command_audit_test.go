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
		RequestID:   "request-1",
		ExecutionID: "11111111-1111-1111-1111-111111111111",
		OwnerUserID: 7,
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
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected one command audit log, total=%d len=%d", total, len(logs))
	}
	if logs[0].UserID == nil || *logs[0].UserID != 7 || logs[0].ExecutionID != commandLog.ExecutionID || logs[0].ResultStatus != string(audit.CommandStatusSuccess) {
		t.Fatalf("unexpected command audit linkage: %+v", logs[0])
	}
}
