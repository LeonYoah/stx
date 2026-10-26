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
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fixedClusterVersionProvider 是测试用集群版本桩。
// fixedClusterVersionProvider is a test stub returning a fixed cluster version.
type fixedClusterVersionProvider struct {
	version string
	err     error
}

func (p fixedClusterVersionProvider) GetClusterVersion(context.Context, uint) (string, error) {
	return p.version, p.err
}

func newInjectionService(version string) *Service {
	service := &Service{}
	service.SetClusterVersionProvider(fixedClusterVersionProvider{version: version})
	return service
}

const streamingHOCON = `env {
  parallelism = 1
  job.mode = "STREAMING"
}
source { FakeSource { plugin_output = "fake" } }
sink { Console { plugin_input = "fake" } }
`

const batchHOCON = `env {
  job.mode = "BATCH"
}
source { FakeSource {} }
sink { Console {} }
`

func TestApplyObservabilityInjection_HOCON(t *testing.T) {
	cases := []struct {
		name         string
		version      string
		definition   JSONMap
		taskMode     TaskMode
		content      string
		wantInjected bool
		wantReason   string
		wantChanged  bool
	}{
		{name: "3.0.0 streaming is injected", version: "3.0.0", content: streamingHOCON, wantInjected: true, wantReason: observabilityReasonInjected, wantChanged: true},
		{name: "2.3.12 is untouched", version: "2.3.12", content: streamingHOCON, wantReason: observabilityReasonVersionUnsupported},
		{name: "unknown version is untouched", version: "", content: streamingHOCON, wantReason: observabilityReasonVersionUnknown},
		{name: "batch job under auto is untouched", version: "3.0.0", content: batchHOCON, wantReason: observabilityReasonBatchJob},
		{name: "on forces batch job injection", version: "3.0.0", definition: JSONMap{"observability": "on"}, content: batchHOCON, wantInjected: true, wantReason: observabilityReasonInjected, wantChanged: true},
		{name: "off disables streaming injection", version: "3.0.0", definition: JSONMap{"observability": "off"}, content: streamingHOCON, wantReason: observabilityReasonDisabledByTask},
		{name: "task mode fallback when content lacks job.mode", version: "3.0.0", taskMode: TaskModeStreaming, content: "source { FakeSource {} }\nsink { Console {} }\n", wantInjected: true, wantReason: observabilityReasonInjected, wantChanged: true},
		{name: "user observability wins", version: "3.0.0", content: streamingHOCON + "env { engine { observability { enabled = false } } }\n", wantReason: observabilityReasonUserConfigured},
		{name: "already injected content is not injected twice", version: "3.0.0", content: observabilityHOCONBlock() + streamingHOCON, wantInjected: true, wantReason: observabilityReasonAlreadyInjected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newInjectionService(tc.version)
			mode := tc.taskMode
			if mode == "" {
				mode = TaskModeBatch
			}
			task := &Task{ClusterID: 1, Mode: mode, Definition: tc.definition}
			body, result := service.applyObservabilityInjection(context.Background(), task, []byte(tc.content), "hocon")
			if result.Injected != tc.wantInjected || result.Reason != tc.wantReason {
				t.Fatalf("got injected=%v reason=%q, want injected=%v reason=%q", result.Injected, result.Reason, tc.wantInjected, tc.wantReason)
			}
			changed := string(body) != tc.content
			if changed != tc.wantChanged {
				t.Fatalf("content changed=%v, want %v; body=%s", changed, tc.wantChanged, body)
			}
			if tc.wantChanged {
				if !strings.HasPrefix(string(body), observabilityInjectMarker) {
					t.Fatalf("expected injected block at the beginning, got %s", body)
				}
				if !strings.HasSuffix(string(body), tc.content) {
					t.Fatalf("expected original content preserved after injected block")
				}
			}
		})
	}
}

func TestApplyObservabilityInjection_versionProviderErrorFallsBackToTaskVersion(t *testing.T) {
	service := &Service{}
	service.SetClusterVersionProvider(fixedClusterVersionProvider{err: errors.New("cluster down")})
	task := &Task{ClusterID: 1, Mode: TaskModeStreaming, EngineVersion: "3.0.0"}
	_, result := service.applyObservabilityInjection(context.Background(), task, []byte(streamingHOCON), "hocon")
	if !result.Injected || result.Version != "3.0.0" {
		t.Fatalf("expected task engine version fallback to inject, got %+v", result)
	}
}

func TestInjectObservabilityHOCON_bracedRoot(t *testing.T) {
	content := "# comment\n// another\n{\n  env { job.mode = STREAMING }\n}\n"
	got := injectObservabilityHOCON(content)
	want := "# comment\n// another\n{\n" + observabilityHOCONBlock() + "\n  env { job.mode = STREAMING }\n}\n"
	if got != want {
		t.Fatalf("braced root injection mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestApplyObservabilityInjection_JSON(t *testing.T) {
	service := newInjectionService("3.0.1-SNAPSHOT")
	task := &Task{ClusterID: 1, Mode: TaskModeStreaming}

	t.Run("adds defaults and keeps user env", func(t *testing.T) {
		content := `{"env":{"job.mode":"STREAMING","parallelism":2,"engine":{"tag":"x"}},"source":[{"plugin_name":"FakeSource","row.num":10000000000}]}`
		body, result := service.applyObservabilityInjection(context.Background(), task, []byte(content), "json")
		if !result.Injected || result.Reason != observabilityReasonInjected {
			t.Fatalf("expected injection, got %+v", result)
		}
		var root map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.UseNumber()
		if err := decoder.Decode(&root); err != nil {
			t.Fatalf("decode injected json: %v", err)
		}
		env := root["env"].(map[string]any)
		engine := env["engine"].(map[string]any)
		if engine["tag"] != "x" || env["parallelism"].(json.Number).String() != "2" {
			t.Fatalf("user env lost: %v", env)
		}
		if !isDefaultObservabilityJSON(engine["observability"]) {
			t.Fatalf("expected default observability, got %v", engine["observability"])
		}
		// 大整数不能被转成浮点。 / Large integers must not be converted to floats.
		if !strings.Contains(string(body), "10000000000") {
			t.Fatalf("large integer was not preserved: %s", body)
		}
		// 再次注入应识别为已注入。 / Re-injecting must be recognized as already injected.
		again, second := service.applyObservabilityInjection(context.Background(), task, body, "json")
		if second.Reason != observabilityReasonAlreadyInjected || string(again) != string(body) {
			t.Fatalf("expected already_injected, got %+v", second)
		}
	})

	t.Run("user observability wins", func(t *testing.T) {
		content := `{"env":{"job.mode":"STREAMING","engine":{"observability":{"enabled":false}}}}`
		body, result := service.applyObservabilityInjection(context.Background(), task, []byte(content), "json")
		if result.Reason != observabilityReasonUserConfigured || string(body) != content {
			t.Fatalf("expected user_configured untouched, got %+v %s", result, body)
		}
	})

	t.Run("flat user key wins", func(t *testing.T) {
		content := `{"env":{"job.mode":"STREAMING","engine.observability.enabled":false}}`
		_, result := service.applyObservabilityInjection(context.Background(), task, []byte(content), "json")
		if result.Reason != observabilityReasonUserConfigured {
			t.Fatalf("expected user_configured, got %+v", result)
		}
	})

	t.Run("invalid json is submitted unchanged", func(t *testing.T) {
		content := `{"env": [1,2]}`
		body, result := service.applyObservabilityInjection(context.Background(), task, []byte(content), "json")
		if result.Reason != observabilityReasonInvalidContent || string(body) != content {
			t.Fatalf("expected invalid_content untouched, got %+v %s", result, body)
		}
	})
}

func TestApplyObservabilityInjection_SQLUnsupported(t *testing.T) {
	service := newInjectionService("3.0.0")
	task := &Task{ClusterID: 1, Mode: TaskModeStreaming}
	content := "CREATE TABLE a WITH ('connector'='FakeSource');"
	body, result := service.applyObservabilityInjection(context.Background(), task, []byte(content), "sql")
	if result.Reason != observabilityReasonFormatUnsupported || string(body) != content {
		t.Fatalf("expected format_unsupported, got %+v", result)
	}
}
