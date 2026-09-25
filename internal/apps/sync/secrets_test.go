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
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSanitizeTaskAndJobResponsesDoNotExposeSecrets(t *testing.T) {
	task := &Task{
		Content: `source { Jdbc { password = "task-secret" username = "root" } }`,
		Definition: JSONMap{
			"clientSecret": "definition-secret",
			"name":         "sample",
		},
	}
	safeTask := sanitizeTaskForResponse(task)
	if strings.Contains(safeTask.Content.String(), "task-secret") || safeTask.Definition["clientSecret"] != maskedSecretValue {
		t.Fatalf("expected task secrets to be masked, got content=%q definition=%#v", safeTask.Content, safeTask.Definition)
	}
	if safeTask.Definition["name"] != "sample" {
		t.Fatalf("expected ordinary task field to remain unchanged, got %#v", safeTask.Definition["name"])
	}
	if !strings.Contains(task.Content.String(), "task-secret") || task.Definition["clientSecret"] != "definition-secret" {
		t.Fatal("sanitizing a response must not mutate the stored task")
	}

	job := &JobInstance{
		SubmitSpec: JSONMap{
			"submitted_content": `{"password":"job-secret","name":"job"}`,
		},
		ErrorMessage: "token=error-secret",
	}
	safeJob := sanitizeJobForResponse(job)
	if strings.Contains(safeJob.SubmitSpec["submitted_content"].(string), "job-secret") || strings.Contains(safeJob.ErrorMessage, "error-secret") {
		t.Fatalf("expected job response secrets to be masked, got submit_spec=%#v error=%q", safeJob.SubmitSpec, safeJob.ErrorMessage)
	}
	if !strings.Contains(job.SubmitSpec["submitted_content"].(string), "job-secret") {
		t.Fatal("sanitizing a response must not mutate the stored job")
	}
}

func TestUpdateTaskPreservesMaskedHOCONAndDefinitionSecrets(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "root",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	task, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "masked-update",
		Mode:          string(TaskModeBatch),
		ContentFormat: string(ContentFormatHOCON),
		Content:       `source { Jdbc { password = "saved-password" username = "root" } }`,
		Definition: JSONMap{
			"accessToken": "saved-token",
			"parallelism": float64(1),
		},
	}, 1)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	updated, err := service.UpdateTask(ctx, task.ID, &UpdateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          task.Name,
		Description:   "changed",
		Mode:          string(TaskModeBatch),
		ContentFormat: string(ContentFormatHOCON),
		Content:       `source { Jdbc { password = "******" username = "admin" } }`,
		Definition: JSONMap{
			"accessToken": maskedSecretValue,
			"parallelism": float64(2),
		},
	})
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	if !strings.Contains(updated.Content.String(), "saved-password") || strings.Contains(updated.Content.String(), maskedSecretValue) {
		t.Fatalf("expected saved HOCON password to be preserved, got %q", updated.Content)
	}
	if !strings.Contains(updated.Content.String(), `username = "admin"`) {
		t.Fatalf("expected non-secret HOCON change to be saved, got %q", updated.Content)
	}
	if updated.Definition["accessToken"] != "saved-token" || updated.Definition["parallelism"] != float64(2) {
		t.Fatalf("expected masked definition secret to be preserved with ordinary updates, got %#v", updated.Definition)
	}
}

func TestRestoreMaskedJSONContentAndRejectUnknownMask(t *testing.T) {
	saved := `{"source":{"password":"saved-password","name":"old"}}`
	incoming := `{"source":{"password":"******","name":"new"}}`
	restored, err := restoreMaskedTaskContent(ContentFormatJSON, saved, incoming)
	if err != nil {
		t.Fatalf("restore JSON content: %v", err)
	}
	if !strings.Contains(restored, "saved-password") || !strings.Contains(restored, `"name":"new"`) {
		t.Fatalf("unexpected restored JSON content: %s", restored)
	}

	_, err = restoreMaskedTaskContent(ContentFormatHOCON, "source { username = root }", "source { password = ****** }")
	if !errors.Is(err, ErrMaskedSecretCannotBeRestored) {
		t.Fatalf("expected unresolved mask error, got %v", err)
	}
}

func TestSanitizeValidationDAGAndPreviewResults(t *testing.T) {
	validation := sanitizeValidateResultForResponse(&ValidateResult{
		Summary:  "password=summary-secret",
		Resolved: map[string]string{"db_password": "resolved-secret", "endpoint": "https://example.com"},
		Checks:   []ValidateCheck{{Target: "token=target-secret", Message: "ok"}},
	})
	if strings.Contains(validation.Summary, "summary-secret") || validation.Resolved["db_password"] != maskedSecretValue || strings.Contains(validation.Checks[0].Target, "target-secret") {
		t.Fatalf("validation result contains secrets: %+v", validation)
	}

	dag := sanitizeDAGResultForResponse(&DAGResult{Nodes: []JSONMap{{"password": "dag-secret", "name": "source"}}})
	if dag.Nodes[0]["password"] != maskedSecretValue || dag.Nodes[0]["name"] != "source" {
		t.Fatalf("unexpected sanitized DAG: %#v", dag.Nodes)
	}

	preview := sanitizePreviewSnapshotForResponse(&PreviewSnapshot{Tables: []*PreviewTableData{{Rows: []map[string]interface{}{{"accessToken": "row-secret", "name": "alice"}}}}})
	if preview.Tables[0].Rows[0]["accessToken"] != maskedSecretValue || preview.Tables[0].Rows[0]["name"] != "alice" {
		t.Fatalf("unexpected sanitized preview rows: %#v", preview.Tables[0].Rows)
	}
}

func TestSanitizeTaskPreservesVariableExpressions(t *testing.T) {
	task := &Task{
		Content: `source {
  Jdbc {
    url = "jdbc:mysql://127.0.0.1:3306/stx_e2e"
    username = "root"
    password = {{mysqlpas}}
  }
}
sink {
  Jdbc {
    password = "{{mysqlpas}}"
  }
}`,
	}
	safe := sanitizeTaskForResponse(task)
	if !strings.Contains(safe.Content.String(), "password = {{mysqlpas}}") {
		t.Fatalf("expected unquoted {{mysqlpas}} to be preserved, got %q", safe.Content)
	}
	if !strings.Contains(safe.Content.String(), `password = "{{mysqlpas}}"`) {
		t.Fatalf("expected quoted \"{{mysqlpas}}\" to be preserved, got %q", safe.Content)
	}
	if strings.Contains(safe.Content.String(), "******") {
		t.Fatalf("expected no ****** in task with variable placeholder, got %q", safe.Content)
	}
}

func TestRestoreMaskedHOCONCleansLegacyCorruptedBraces(t *testing.T) {
	saved := `source { Jdbc { password = {{mysqlpas}} } }`
	incoming := `source { Jdbc { password = ******}} } }`
	restored, err := restoreMaskedTaskContent(ContentFormatHOCON, saved, incoming)
	if err != nil {
		t.Fatalf("failed to restore legacy corrupted mask: %v", err)
	}
	if !strings.Contains(restored, "password = {{mysqlpas}}") {
		t.Fatalf("expected password to be restored to {{mysqlpas}}, got %q", restored)
	}
	if strings.Contains(restored, "******") {
		t.Fatalf("expected mask to be completely replaced, got %q", restored)
	}
}
