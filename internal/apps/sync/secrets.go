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
	"encoding/json"
	"regexp"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/db"
)

const maskedSecretValue = "******"

var syncAssignmentPattern = regexp.MustCompile(`(?m)(["']?)([A-Za-z_][A-Za-z0-9_.-]*)(["']?)(\s*[:=]\s*)("[^"\\]*(?:\\.[^"\\]*)*"|'[^'\\]*(?:\\.[^'\\]*)*'|\{\{[^{}]*\}\}|\$\{[^{}]*\}|\*{6}\}{0,2}|[^\s,;}\]]+)`)

// sanitizeTaskForResponse 返回可安全展示的任务副本，不修改数据库实体。
// sanitizeTaskForResponse returns a display-safe task copy without mutating the database entity.
func sanitizeTaskForResponse(task *Task) *Task {
	if task == nil {
		return nil
	}
	clone := *task
	clone.Content = db.ScriptText(audit.RedactText(task.Content.String()))
	clone.Definition = redactSyncJSONMap(task.Definition)
	return &clone
}

func sanitizeTasksForResponse(tasks []*Task) []*Task {
	result := make([]*Task, len(tasks))
	for index, task := range tasks {
		result[index] = sanitizeTaskForResponse(task)
	}
	return result
}

func sanitizeTaskTreeForResponse(nodes []*TaskTreeNode) []*TaskTreeNode {
	result := make([]*TaskTreeNode, len(nodes))
	for index, node := range nodes {
		if node == nil {
			continue
		}
		clone := *node
		clone.Content = audit.RedactText(node.Content)
		clone.Definition = redactSyncJSONMap(node.Definition)
		clone.Children = sanitizeTaskTreeForResponse(node.Children)
		result[index] = &clone
	}
	return result
}

func sanitizeTaskVersionForResponse(version *TaskVersion) *TaskVersion {
	if version == nil {
		return nil
	}
	clone := *version
	clone.ContentSnapshot = db.ScriptText(audit.RedactText(version.ContentSnapshot.String()))
	clone.DefinitionSnapshot = redactSyncJSONMap(version.DefinitionSnapshot)
	return &clone
}

func sanitizeTaskVersionsForResponse(versions []*TaskVersion) []*TaskVersion {
	result := make([]*TaskVersion, len(versions))
	for index, version := range versions {
		result[index] = sanitizeTaskVersionForResponse(version)
	}
	return result
}

func sanitizeJobForResponse(job *JobInstance) *JobInstance {
	if job == nil {
		return nil
	}
	clone := *job
	clone.SubmitSpec = redactSyncJSONMap(job.SubmitSpec)
	clone.ResultPreview = redactSyncJSONMap(job.ResultPreview)
	clone.ErrorMessage = audit.RedactText(job.ErrorMessage)
	return &clone
}

func sanitizeJobsForResponse(jobs []*JobInstance) []*JobInstance {
	result := make([]*JobInstance, len(jobs))
	for index, job := range jobs {
		result[index] = sanitizeJobForResponse(job)
	}
	return result
}

func sanitizeJobLogsForResponse(result *JobLogsResult) *JobLogsResult {
	if result == nil {
		return nil
	}
	clone := *result
	clone.Logs = audit.RedactText(result.Logs)
	clone.EmptyReason = audit.RedactText(result.EmptyReason)
	return &clone
}

func sanitizeValidateResultForResponse(result *ValidateResult) *ValidateResult {
	if result == nil {
		return nil
	}
	clone := *result
	clone.Summary = audit.RedactText(result.Summary)
	clone.Errors = redactTextSlice(result.Errors)
	clone.Warnings = redactTextSlice(result.Warnings)
	clone.Resolved = make(map[string]string, len(result.Resolved))
	for key, value := range result.Resolved {
		if audit.IsSensitiveKey(key) {
			clone.Resolved[key] = maskedSecretValue
			continue
		}
		clone.Resolved[key] = audit.RedactText(value)
	}
	clone.Checks = make([]ValidateCheck, len(result.Checks))
	copy(clone.Checks, result.Checks)
	for index := range clone.Checks {
		clone.Checks[index].Target = audit.RedactText(clone.Checks[index].Target)
		clone.Checks[index].Message = audit.RedactText(clone.Checks[index].Message)
	}
	return &clone
}

func sanitizeDAGResultForResponse(result *DAGResult) *DAGResult {
	if result == nil {
		return nil
	}
	clone := *result
	clone.Nodes = make([]JSONMap, len(result.Nodes))
	for index, node := range result.Nodes {
		clone.Nodes[index] = redactSyncJSONMap(node)
	}
	clone.Edges = make([]JSONMap, len(result.Edges))
	for index, edge := range result.Edges {
		clone.Edges[index] = redactSyncJSONMap(edge)
	}
	clone.WebUIJob = redactSyncJSONMap(result.WebUIJob)
	clone.Warnings = redactTextSlice(result.Warnings)
	return &clone
}

func sanitizePreviewSnapshotForResponse(snapshot *PreviewSnapshot) *PreviewSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	clone.EmptyReason = audit.RedactText(snapshot.EmptyReason)
	clone.InjectedScript = audit.RedactText(snapshot.InjectedScript)
	clone.Warnings = redactTextSlice(snapshot.Warnings)
	clone.Tables = make([]*PreviewTableData, len(snapshot.Tables))
	for index, table := range snapshot.Tables {
		clone.Tables[index] = sanitizePreviewTableForResponse(table)
	}
	clone.SelectedTable = sanitizePreviewTableForResponse(snapshot.SelectedTable)
	return &clone
}

func sanitizePreviewTableForResponse(table *PreviewTableData) *PreviewTableData {
	if table == nil {
		return nil
	}
	clone := *table
	clone.Rows = make([]map[string]interface{}, len(table.Rows))
	for index, row := range table.Rows {
		clone.Rows[index] = map[string]interface{}(audit.RedactDetails(audit.AuditDetails(row)))
	}
	return &clone
}

func redactTextSlice(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = audit.RedactText(value)
	}
	return result
}

func redactSyncJSONMap(value JSONMap) JSONMap {
	if value == nil {
		return nil
	}
	redacted := audit.RedactDetails(audit.AuditDetails(value))
	return JSONMap(redacted)
}

// restoreMaskedTaskContent 把客户端回传的掩码替换为已保存值，避免把掩码写入任务正文。
// restoreMaskedTaskContent replaces client-returned masks with saved values so masks are never persisted as task content.
func restoreMaskedTaskContent(format ContentFormat, saved, incoming string) (string, error) {
	if !strings.Contains(incoming, maskedSecretValue) {
		return incoming, nil
	}
	if format == ContentFormatJSON {
		return restoreMaskedJSONContent(saved, incoming)
	}
	return restoreMaskedHOCONContent(saved, incoming)
}

func restoreMaskedJSONContent(saved, incoming string) (string, error) {
	var savedValue any
	if err := json.Unmarshal([]byte(saved), &savedValue); err != nil {
		return "", ErrMaskedSecretCannotBeRestored
	}
	var incomingValue any
	if err := json.Unmarshal([]byte(incoming), &incomingValue); err != nil {
		return "", ErrMaskedSecretCannotBeRestored
	}
	restored, err := restoreMaskedJSONValue(savedValue, incomingValue, "")
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(restored)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func restoreMaskedJSONValue(saved, incoming any, key string) (any, error) {
	if audit.IsSensitiveKey(key) && incoming == maskedSecretValue {
		if saved == nil {
			return nil, ErrMaskedSecretCannotBeRestored
		}
		return saved, nil
	}
	switch current := incoming.(type) {
	case map[string]any:
		savedMap, _ := saved.(map[string]any)
		result := make(map[string]any, len(current))
		for childKey, childValue := range current {
			var savedChild any
			if savedMap != nil {
				savedChild = savedMap[childKey]
			}
			restored, err := restoreMaskedJSONValue(savedChild, childValue, childKey)
			if err != nil {
				return nil, err
			}
			result[childKey] = restored
		}
		return result, nil
	case []any:
		savedItems, _ := saved.([]any)
		result := make([]any, len(current))
		for index, childValue := range current {
			var savedChild any
			if index < len(savedItems) {
				savedChild = savedItems[index]
			}
			restored, err := restoreMaskedJSONValue(savedChild, childValue, key)
			if err != nil {
				return nil, err
			}
			result[index] = restored
		}
		return result, nil
	default:
		return incoming, nil
	}
}

func restoreMaskedHOCONContent(saved, incoming string) (string, error) {
	savedValues := make(map[string][]string)
	for _, match := range syncAssignmentPattern.FindAllStringSubmatch(saved, -1) {
		if len(match) < 6 || !audit.IsSensitiveKey(match[2]) {
			continue
		}
		key := normalizeSecretAssignmentKey(match[2])
		savedValues[key] = append(savedValues[key], match[5])
	}
	used := make(map[string]int)
	failed := false
	restored := syncAssignmentPattern.ReplaceAllStringFunc(incoming, func(matchText string) string {
		match := syncAssignmentPattern.FindStringSubmatch(matchText)
		if len(match) < 6 || !audit.IsSensitiveKey(match[2]) || unquoteSecretValue(match[5]) != maskedSecretValue {
			return matchText
		}
		key := normalizeSecretAssignmentKey(match[2])
		index := used[key]
		used[key] = index + 1
		values := savedValues[key]
		if index >= len(values) {
			failed = true
			return matchText
		}
		return match[1] + match[2] + match[3] + match[4] + values[index]
	})
	if failed || strings.Contains(restored, maskedSecretValue) {
		return "", ErrMaskedSecretCannotBeRestored
	}
	return restored, nil
}

func restoreMaskedJSONMap(saved, incoming JSONMap) (JSONMap, error) {
	if incoming == nil {
		return JSONMap{}, nil
	}
	restored, err := restoreMaskedJSONValue(map[string]any(saved), map[string]any(incoming), "")
	if err != nil {
		return nil, err
	}
	result, ok := restored.(map[string]any)
	if !ok {
		return nil, ErrMaskedSecretCannotBeRestored
	}
	return JSONMap(result), nil
}

func normalizeSecretAssignmentKey(key string) string {
	return strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(strings.TrimSpace(key)))
}

func unquoteSecretValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'')) {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	trimmed = strings.TrimSuffix(trimmed, "}}")
	return strings.TrimSpace(trimmed)
}
