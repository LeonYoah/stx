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
	"strings"
	"testing"
)

func TestTextEventWriterEmitsPlainLines(t *testing.T) {
	var buf bytes.Buffer
	w := NewTextEventWriter(&buf)
	if err := w.Emit(Event{Event: "log_line", Message: "hello sea"}); err != nil {
		t.Fatalf("emit log_line: %v", err)
	}
	if err := w.Emit(Event{Event: "job_status", Status: "success"}); err != nil {
		t.Fatalf("emit job_status: %v", err)
	}
	if err := w.Emit(Event{Event: "job_metrics", Message: "should-skip"}); err != nil {
		t.Fatalf("emit job_metrics: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, `"event"`) {
		t.Fatalf("text mode must not emit JSON: %q", out)
	}
	if !strings.Contains(out, "hello sea\n") {
		t.Fatalf("missing plain log line: %q", out)
	}
	if !strings.Contains(out, "[stx] job_status=success\n") {
		t.Fatalf("missing status line: %q", out)
	}
	if strings.Contains(out, "should-skip") {
		t.Fatalf("job_metrics envelope should be silent in text mode: %q", out)
	}
}

func TestJSONEventWriterStillEmitsNDJSON(t *testing.T) {
	var buf bytes.Buffer
	w := NewEventWriter(&buf)
	if err := w.Emit(Event{Event: "log_line", Message: "hello"}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if !strings.Contains(buf.String(), `"event":"log_line"`) {
		t.Fatalf("json mode should emit NDJSON: %q", buf.String())
	}
}
