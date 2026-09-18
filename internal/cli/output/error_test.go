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
	"context"
	"encoding/json"
	"testing"
)

func TestEventWriterErrorIncludesFalseRetryable(t *testing.T) {
	var stderr bytes.Buffer
	err := NewError(CodeConflict, "resource changed", ExitConflict, false)
	err.RequestID = "req_conflict"
	if writeErr := NewEventWriter(&stderr).EmitError(err); writeErr != nil {
		t.Fatalf("写入错误事件失败 / writing error event failed: %v", writeErr)
	}
	var event map[string]any
	if decodeErr := json.Unmarshal(stderr.Bytes(), &event); decodeErr != nil {
		t.Fatalf("错误事件不是合法 JSON / error event is not valid JSON: %v", decodeErr)
	}
	if event["event"] != "error" || event["code"] != CodeConflict || event["retryable"] != false || event["request_id"] != "req_conflict" {
		t.Fatalf("错误事件字段错误 / error event fields are incorrect: %#v", event)
	}
}

func TestClassifyErrorTimeout(t *testing.T) {
	classified := ClassifyError(context.DeadlineExceeded)
	if classified.ExitCode != ExitTimeout || classified.Code != CodeTimeout || !classified.Retryable {
		t.Fatalf("超时分类错误 / timeout classification is incorrect: %#v", classified)
	}
}
