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
	"strings"
	"testing"
)

// 未定义模板变量等提前返回路径不填 warnings，序列化后必须是 [] 而不是 null。
// Early-return paths such as undefined template variables leave warnings unset;
// they must serialize as [] rather than null.
func TestValidateResultMarshalJSONNeverEmitsNullLists(t *testing.T) {
	payload, err := json.Marshal(&ValidateResult{
		Valid:   false,
		Errors:  []string{ErrUndefinedTemplateVariable.Error()},
		Summary: "Connection test failed: undefined template variable.",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(payload)
	if strings.Contains(body, "null") {
		t.Fatalf("expected no null fields, got %s", body)
	}
	if !strings.Contains(body, `"warnings":[]`) {
		t.Fatalf("expected empty warnings array, got %s", body)
	}

	empty, err := json.Marshal(ValidateResult{})
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if !strings.Contains(string(empty), `"errors":[]`) || !strings.Contains(string(empty), `"warnings":[]`) {
		t.Fatalf("expected empty arrays for zero value, got %s", empty)
	}
}

// 已有内容与 omitempty 字段保持原样。
// Existing values and omitempty fields keep their original encoding.
func TestValidateResultMarshalJSONKeepsExistingFields(t *testing.T) {
	payload, err := json.Marshal(ValidateResult{
		Valid:        true,
		Warnings:     []string{"w1"},
		DetectedVars: []string{"a"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(payload)
	for _, want := range []string{`"valid":true`, `"warnings":["w1"]`, `"detected_vars":["a"]`, `"errors":[]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in %s", want, body)
		}
	}
	if strings.Contains(body, "checks") || strings.Contains(body, "resolved") {
		t.Fatalf("omitempty fields should stay omitted, got %s", body)
	}
}
