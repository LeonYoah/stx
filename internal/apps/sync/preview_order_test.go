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
	"reflect"
	"testing"
	"time"
)

func TestExtractOrderedKeysFromJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []string
	}{
		{
			name:     "single object preserves key order",
			input:    []byte(`{"id": 1, "name": "alice", "age": 20, "email": "a@b.com"}`),
			expected: []string{"id", "name", "age", "email"},
		},
		{
			name:     "array of objects uses first object keys",
			input:    []byte(`[{"order_no": "A001", "amount": 100, "remark": "ok"}, {"order_no": "A002"}]`),
			expected: []string{"order_no", "amount", "remark"},
		},
		{
			name:     "object with rows array",
			input:    []byte(`{"platform_job_id": "123", "rows": [{"col_a": 1, "col_b": 2, "col_c": 3}]}`),
			expected: []string{"col_a", "col_b", "col_c"},
		},
		{
			name:     "empty input",
			input:    []byte(``),
			expected: nil,
		},
		{
			name:     "invalid json",
			input:    []byte(`not a json`),
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := extractOrderedKeysFromJSON(tc.input)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

func TestExtractSchemaFieldNamesFromHocon(t *testing.T) {
	hoconContent := `
env {
  job.mode = "STREAMING"
}
source {
  FakeSource {
    schema = {
      fields {
        id = "bigint"
        name = "string"
        age = "int"
        create_time = "timestamp"
        email = "string"
        phone = "string"
        address = "string"
        score = "decimal(10,2)"
        is_vip = "boolean"
        register_date = "date"
        order_no = "string"
        amount = "bigint"
        remark = "string"
      }
    }
  }
}
`
	expected := []string{
		"id", "name", "age", "create_time", "email",
		"phone", "address", "score", "is_vip",
		"register_date", "order_no", "amount", "remark",
	}

	actual := extractSchemaFieldNamesFromHocon(hoconContent)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func TestAlignPreviewColumnsOrder(t *testing.T) {
	// 场景：数据库或无序 map 中乱序收集的列
	// Scenario: scrambled columns collected from unordered map
	scrambled := []string{
		"remark", "id", "email", "address", "amount", "create_time",
		"score", "age", "phone", "is_vip", "register_date", "name", "order_no", "RowKind",
	}

	schemaFields := []string{
		"id", "name", "age", "create_time", "email",
		"phone", "address", "score", "is_vip",
		"register_date", "order_no", "amount", "remark",
	}

	expected := []string{
		"RowKind", "id", "name", "age", "create_time", "email",
		"phone", "address", "score", "is_vip",
		"register_date", "order_no", "amount", "remark",
	}

	actual := alignPreviewColumnsOrder(scrambled, schemaFields, nil)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func TestAlignPreviewColumnsOrderWithExtraColumns(t *testing.T) {
	// 存在额外未声明在 schema 中的派生列
	// Scenario: derived columns not declared in schema
	columns := []string{"calculated_tax", "id", "name", "RowKind"}
	schemaFields := []string{"id", "name"}

	expected := []string{"RowKind", "id", "name", "calculated_tax"}
	actual := alignPreviewColumnsOrder(columns, schemaFields, nil)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func TestEqualStringSlices(t *testing.T) {
	if !equalStringSlices([]string{"a", "b"}, []string{"a", "b"}) {
		t.Fatalf("expected true")
	}
	if equalStringSlices([]string{"a", "b"}, []string{"b", "a"}) {
		t.Fatalf("expected false for different order")
	}
	if equalStringSlices([]string{"a"}, []string{"a", "b"}) {
		t.Fatalf("expected false for different length")
	}
}

func TestNormalizePreviewCollectRequestPreservesKeyOrder(t *testing.T) {
	// 验证 normalizePreviewCollectRequest 解析原始字节流时保持字段定义顺序
	// Verify normalizePreviewCollectRequest preserves field order from raw JSON bytes
	rawJSON := []byte(`{
		"platform_job_id": "test_job_1",
		"rows": [
			{
				"__st_debug_rowkind": "+I",
				"id": 1,
				"name": "stx",
				"age": 18,
				"create_time": "2026-10-28 12:00:00",
				"email": "test@example.com",
				"phone": "13800000000",
				"address": "Beijing",
				"score": 99.5,
				"is_vip": true,
				"register_date": "2026-10-28",
				"order_no": "ORD-001",
				"amount": 1000,
				"remark": "ok"
			}
		]
	}`)

	var req PreviewCollectRequest
	normalizePreviewCollectRequest(&req, rawJSON)

	expectedCols := []string{
		"RowKind", "id", "name", "age", "create_time", "email",
		"phone", "address", "score", "is_vip",
		"register_date", "order_no", "amount", "remark",
	}

	actualCols := interfaceSliceToStrings(req.Columns)
	if !reflect.DeepEqual(actualCols, expectedCols) {
		t.Fatalf("expected columns %v, got %v", expectedCols, actualCols)
	}
}

func TestGetPreviewSnapshotAlignsExistingColumnsWithSchema(t *testing.T) {
	// 验证 GetPreviewSnapshot 会自动把乱序存储的列按 Schema 声明顺序重排并更新数据库
	// Verify GetPreviewSnapshot automatically reorders scrambled columns by schema order and updates db
	service := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now()

	hoconContent := `
env { job.mode = "STREAMING" }
source {
  FakeSource {
    schema = {
      fields {
        id = "bigint"
        name = "string"
        age = "int"
        email = "string"
      }
    }
  }
}
`
	instance := &JobInstance{
		TaskID:        101,
		PlatformJobID: "test_preview_job",
		Status:        JobStatusRunning,
		RunType:       RunTypePreview,
		StartedAt:     &now,
		ResultPreview: JSONMap{
			"preview_content": hoconContent,
		},
	}
	if err := service.repo.CreateJobInstance(ctx, instance); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}

	session := &PreviewSession{
		JobInstanceID:  instance.ID,
		TaskID:         instance.TaskID,
		PlatformJobID:  instance.PlatformJobID,
		RowLimit:       10,
		TimeoutMinutes: 5,
		Status:         "collecting",
		StartedAt:      &now,
	}
	if err := service.repo.CreatePreviewSession(ctx, session); err != nil {
		t.Fatalf("create preview session failed: %v", err)
	}

	// 存入乱序的列 (email, RowKind, age, id, name)
	// Store scrambled columns (email, RowKind, age, id, name)
	table := &PreviewTable{
		SessionID:   session.ID,
		TablePath:   "users",
		DisplayName: "users",
		Columns:     JSONStringSlice{"email", "RowKind", "age", "id", "name"},
		RowCount:    1,
	}
	if err := service.repo.CreatePreviewTable(ctx, table); err != nil {
		t.Fatalf("create preview table failed: %v", err)
	}

	// 调用 GetPreviewSnapshot，应自动按 Schema 对齐为 [RowKind, id, name, age, email]
	// Call GetPreviewSnapshot, which should automatically align to [RowKind, id, name, age, email]
	snapshot, err := service.GetPreviewSnapshot(ctx, instance.ID, "users")
	if err != nil {
		t.Fatalf("get preview snapshot failed: %v", err)
	}

	expectedCols := []string{"RowKind", "id", "name", "age", "email"}
	if len(snapshot.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(snapshot.Tables))
	}
	if !reflect.DeepEqual(snapshot.Tables[0].Columns, expectedCols) {
		t.Fatalf("expected aligned columns %v, got %v", expectedCols, snapshot.Tables[0].Columns)
	}
	if snapshot.SelectedTable == nil || !reflect.DeepEqual(snapshot.SelectedTable.Columns, expectedCols) {
		t.Fatalf("expected selected table columns %v, got %v", expectedCols, snapshot.SelectedTable.Columns)
	}

	// 验证数据库中的记录也被写回对齐
	// Verify table in database was also updated with aligned order
	updatedTable, err := service.repo.GetPreviewTableByPath(ctx, session.ID, "users")
	if err != nil {
		t.Fatalf("get table from db failed: %v", err)
	}
	if !reflect.DeepEqual([]string(updatedTable.Columns), expectedCols) {
		t.Fatalf("expected db columns %v, got %v", expectedCols, updatedTable.Columns)
	}
}

func TestCollectPreviewAlignsColumnsWithSchema(t *testing.T) {
	// 验证上报预览数据时，自动根据 Schema 定义顺序持久化对齐后的列
	// Verify preview collection automatically aligns and persists columns per schema order
	service := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now()

	hoconContent := `
env { job.mode = "STREAMING" }
source {
  FakeSource {
    schema = {
      fields {
        id = "bigint"
        name = "string"
        age = "int"
        email = "string"
      }
    }
  }
}
`
	instance := &JobInstance{
		TaskID:        102,
		PlatformJobID: "test_collect_job",
		Status:        JobStatusRunning,
		RunType:       RunTypePreview,
		StartedAt:     &now,
		ResultPreview: JSONMap{
			"preview_content": hoconContent,
		},
	}
	if err := service.repo.CreateJobInstance(ctx, instance); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}

	rawJSON := []byte(`{
		"platform_job_id": "test_collect_job",
		"dataset": "orders",
		"rows": [
			{"email": "b@b.com", "__st_debug_rowkind": "+I", "age": 25, "id": 99, "name": "bob"}
		]
	}`)

	var req PreviewCollectRequest
	normalizePreviewCollectRequest(&req, rawJSON)

	if err := service.CollectPreview(ctx, &req); err != nil {
		t.Fatalf("collect preview failed: %v", err)
	}

	snapshot, err := service.GetPreviewSnapshot(ctx, instance.ID, "orders")
	if err != nil {
		t.Fatalf("get preview snapshot failed: %v", err)
	}

	expectedCols := []string{"RowKind", "id", "name", "age", "email"}
	if !reflect.DeepEqual(snapshot.Tables[0].Columns, expectedCols) {
		t.Fatalf("expected %v, got %v", expectedCols, snapshot.Tables[0].Columns)
	}
}
