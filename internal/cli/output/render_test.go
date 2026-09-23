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
package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRendererJSONProducesStableEnvelope(t *testing.T) {
	result := NewResult("cluster.list", "req_test", []map[string]any{})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	renderer := NewRenderer(&stdout, NewEventWriter(&stderr))

	if err := renderer.Render(result, Options{Format: FormatJSON}); err != nil {
		t.Fatalf("渲染 JSON 失败 / rendering JSON failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout 不是合法 JSON / stdout is not valid JSON: %v", err)
	}
	if decoded["api_version"] != "v1" || decoded["operation_id"] != "cluster.list" || decoded["request_id"] != "req_test" {
		t.Fatalf("协议字段错误 / protocol fields are incorrect: %#v", decoded)
	}
	data, ok := decoded["data"].([]any)
	if !ok || data == nil || len(data) != 0 {
		t.Fatalf("空列表必须输出 [] / empty list must be []: %#v", decoded["data"])
	}
	meta, ok := decoded["result_meta"].(map[string]any)
	if !ok || len(meta) != 1 || meta["complete"] != true {
		t.Fatalf("完整结果元数据只能包含 complete / complete result metadata is invalid: %#v", meta)
	}
	if stderr.Len() != 0 {
		t.Fatalf("普通成功结果不应产生 stderr / regular success must not write stderr: %s", stderr.String())
	}
}

func TestRendererPickObjectAndList(t *testing.T) {
	tests := []struct {
		name string
		data any
	}{
		{name: "object", data: map[string]any{"name": "one", "secret": "masked"}},
		{name: "list", data: []map[string]any{{"name": "one", "secret": "masked"}, {"name": "two", "secret": "masked"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := NewResult("sample.get", "req_pick", test.data)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			renderer := NewRenderer(&stdout, NewEventWriter(&stderr))
			if err := renderer.Render(result, Options{Format: FormatJSON, Pick: []string{"name"}}); err != nil {
				t.Fatalf("执行字段选择失败 / applying field selection failed: %v", err)
			}
			if strings.Contains(stdout.String(), "secret") || strings.Contains(stdout.String(), "masked") {
				t.Fatalf("字段选择后仍包含未选字段 / output still contains an unselected field: %s", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("有效字段选择不应回退 / valid pick must not fall back: %s", stderr.String())
			}
		})
	}
}

func TestRendererPickFallbackReturnsOriginalSafeResult(t *testing.T) {
	result := NewResult("sample.get", "req_fallback", map[string]any{"name": "one", "status": "ready"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	renderer := NewRenderer(&stdout, NewEventWriter(&stderr))
	if err := renderer.Render(result, Options{Format: FormatJSON, Pick: []string{"missing"}}); err != nil {
		t.Fatalf("执行字段选择失败 / applying field selection failed: %v", err)
	}
	if !strings.Contains(stdout.String(), `"name":"one"`) || !strings.Contains(stdout.String(), `"status":"ready"`) {
		t.Fatalf("字段不存在时应返回原安全结果 / missing field must return original safe result: %s", stdout.String())
	}
	var event Event
	if err := json.Unmarshal(stderr.Bytes(), &event); err != nil {
		t.Fatalf("fallback 事件不是合法 JSON / fallback event is not valid JSON: %v", err)
	}
	if event.Event != "pick_fallback" || len(event.MissingFields) != 1 || event.MissingFields[0] != "missing" {
		t.Fatalf("fallback 事件错误 / fallback event is incorrect: %#v", event)
	}
}

func TestRendererSupportsYAMLTableAndRaw(t *testing.T) {
	result := NewResult("sample.get", "req_formats", map[string]any{"name": "one", "status": "ready"})
	tests := []struct {
		format   Format
		contains []string
	}{
		{format: FormatYAML, contains: []string{"api_version: v1", "operation_id: sample.get", "complete: true"}},
		{format: FormatTable, contains: []string{"FIELD", "name", "OPERATION_ID", "sample.get", "COMPLETE", "true"}},
		{format: FormatRaw, contains: []string{`"name":"one"`, `"status":"ready"`}},
	}
	for _, test := range tests {
		t.Run(string(test.format), func(t *testing.T) {
			var stdout bytes.Buffer
			renderer := NewRenderer(&stdout, NewEventWriter(&bytes.Buffer{}))
			if err := renderer.Render(result, Options{Format: test.format}); err != nil {
				t.Fatalf("渲染 %s 失败 / rendering %s failed: %v", test.format, test.format, err)
			}
			for _, expected := range test.contains {
				if !strings.Contains(stdout.String(), expected) {
					t.Fatalf("%s 输出缺少 %q / %s output is missing %q: %s", test.format, expected, test.format, expected, stdout.String())
				}
			}
		})
	}
}

func TestRendererRawStillAppliesPick(t *testing.T) {
	result := NewResult("sample.get", "req_raw_pick", map[string]any{"name": "one", "secret": "must-not-appear"})
	var stdout bytes.Buffer
	renderer := NewRenderer(&stdout, NewEventWriter(&bytes.Buffer{}))
	if err := renderer.Render(result, Options{Format: FormatRaw, Pick: []string{"name"}}); err != nil {
		t.Fatalf("渲染 raw 失败 / rendering raw failed: %v", err)
	}
	if strings.Contains(stdout.String(), "secret") || strings.Contains(stdout.String(), "must-not-appear") {
		t.Fatalf("raw 不得绕过字段选择 / raw must not bypass field selection: %s", stdout.String())
	}
}
