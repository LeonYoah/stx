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
	"strings"
	"testing"
)

func TestRedactDetailsHandlesNestedValuesAndCaseVariants(t *testing.T) {
	details := AuditDetails{
		"Password": "database-password",
		"nested": map[string]any{
			"apiKey": "api-secret",
			"items": []any{
				map[string]any{"ACCESS_TOKEN": "access-token", "name": "source"},
			},
		},
	}

	redacted := RedactDetails(details)
	if redacted["Password"] != redactedValue {
		t.Fatalf("expected Password to be redacted, got %#v", redacted["Password"])
	}
	nested := redacted["nested"].(map[string]any)
	if nested["apiKey"] != redactedValue {
		t.Fatalf("expected apiKey to be redacted, got %#v", nested["apiKey"])
	}
	items := nested["items"].([]any)
	item := items[0].(map[string]any)
	if item["ACCESS_TOKEN"] != redactedValue {
		t.Fatalf("expected ACCESS_TOKEN to be redacted, got %#v", item["ACCESS_TOKEN"])
	}
	if item["name"] != "source" {
		t.Fatalf("expected ordinary field to remain unchanged, got %#v", item["name"])
	}
}

func TestRedactTextHandlesJSONAndHOCONWithoutChangingOrdinaryFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "json", input: `{"password":"json-secret","secretary":"alice","token_expiry":3600}`},
		{name: "hocon", input: "source.password = hocon-secret\nsource.name = mysql\npassword_hint = unchanged"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			redacted := RedactText(test.input)
			if strings.Contains(redacted, "json-secret") || strings.Contains(redacted, "hocon-secret") {
				t.Fatalf("expected secret text to be removed, got %q", redacted)
			}
			if test.name == "json" && (!strings.Contains(redacted, `"secretary":"alice"`) || !strings.Contains(redacted, `"token_expiry":3600`)) {
				t.Fatalf("expected ordinary JSON fields to remain unchanged, got %q", redacted)
			}
			if test.name == "hocon" && (!strings.Contains(redacted, "source.name = mysql") || !strings.Contains(redacted, "password_hint = unchanged")) {
				t.Fatalf("expected ordinary HOCON fields to remain unchanged, got %q", redacted)
			}
		})
	}
}

func TestRepositoryRedactsSecretsBeforePersistence(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	ownerUserID := uint(42)

	auditLog := &AuditLog{
		UserID:       &ownerUserID,
		Action:       "execution.create",
		ResourceType: "execution",
		Details: AuditDetails{
			"password": "audit-password",
			"config":   `token = audit-token`,
		},
	}
	if err := repo.CreateAuditLog(ctx, auditLog); err != nil {
		t.Fatalf("create audit log: %v", err)
	}
	storedAudit, err := repo.GetAuditLogByID(ctx, auditLog.ID)
	if err != nil {
		t.Fatalf("get audit log: %v", err)
	}
	if serialized := storedAudit.Details["password"]; serialized != redactedValue {
		t.Fatalf("expected stored audit password to be redacted, got %#v", serialized)
	}
	if config, _ := storedAudit.Details["config"].(string); strings.Contains(config, "audit-token") {
		t.Fatalf("expected stored audit config to be redacted, got %q", config)
	}

	commandLog := &CommandLog{
		CommandID:   "command-redaction-test",
		AgentID:     "agent-1",
		CommandType: "diagnostic",
		Parameters: CommandParameters{
			"clientSecret": "command-secret",
		},
		Status:    CommandStatusSuccess,
		Output:    `{"access_key":"output-secret"}`,
		Error:     "password=error-secret",
		CreatedBy: &ownerUserID,
	}
	if err := repo.CreateCommandLog(ctx, commandLog); err != nil {
		t.Fatalf("create command log: %v", err)
	}
	storedCommand, err := repo.GetCommandLogByID(ctx, commandLog.ID)
	if err != nil {
		t.Fatalf("get command log: %v", err)
	}
	if storedCommand.Parameters["clientSecret"] != redactedValue {
		t.Fatalf("expected stored command parameter to be redacted, got %#v", storedCommand.Parameters["clientSecret"])
	}
	for _, value := range []string{storedCommand.Output, storedCommand.Error} {
		if strings.Contains(value, "output-secret") || strings.Contains(value, "error-secret") {
			t.Fatalf("expected stored command text to be redacted, got %q", value)
		}
	}
}
