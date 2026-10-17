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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

const (
	dolphinFakeProcess  = "process"  // 3.1 / 3.2
	dolphinFakeWorkflow = "workflow" // 3.3+
	dolphinFakeV2       = "v2"
)

type dolphinFake struct {
	family            string
	paths             []string
	lastForm          url.Values
	lastScheduleQuery url.Values
	nextTask          int64
	nextDef           int64
	definition        map[string]any
}

func newDolphinFakeServer(t *testing.T, family string) (*httptest.Server, *dolphinFake) {
	t.Helper()
	fake := &dolphinFake{
		family:   family,
		nextTask: 88001,
		nextDef:  90001,
		definition: map[string]any{
			"code":         int64(9),
			"name":         "orders",
			"version":      3,
			"releaseState": "OFFLINE",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(server.Close)
	return server, fake
}

func (f *dolphinFake) serve(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	if parsed, err := url.ParseQuery(string(body)); err == nil && len(parsed) > 0 {
		f.lastForm = parsed
	}
	f.paths = append(f.paths, request.Method+" "+request.URL.Path)
	if f.family == dolphinFakeV2 {
		f.serveV2(writer, request)
		return
	}
	path := request.URL.Path
	if strings.Contains(path, "workflow-definition") && f.family != dolphinFakeWorkflow {
		// 3.1/3.2 真实单机对未知路径返回 200 HTML 控制台，而不是 JSON 业务错误。
		// Real 3.1/3.2 standalone serves the 200 HTML console for unknown paths, not a JSON biz error.
		writeDolphinHTMLFallback(writer)
		return
	}
	if strings.Contains(path, "process-definition") && f.family != dolphinFakeProcess {
		writeDolphinBizError(writer, "process-definition was renamed in 3.3")
		return
	}
	switch {
	case request.Method == http.MethodGet && strings.HasSuffix(path, "/projects"):
		writeDolphinOK(writer, map[string]any{"totalList": []any{map[string]any{"code": 7, "name": "stg"}}})
	case strings.Contains(path, "gen-task-codes"):
		writeDolphinOK(writer, []int64{f.nextTask})
	case strings.Contains(path, "/schedules"):
		f.serveSchedules(writer, request)
	case request.Method == http.MethodGet && f.isDefinitionCollection(path):
		writeDolphinOK(writer, map[string]any{
			"totalList": []any{map[string]any{"code": 9, "name": "orders", "releaseState": "OFFLINE", "version": 3}},
		})
	case request.Method == http.MethodGet && f.isDefinitionItem(path):
		writeDolphinOK(writer, f.definitionPayload())
	case request.Method == http.MethodPost && f.isDefinitionCollection(path):
		f.definition["code"] = f.nextDef
		writeDolphinOK(writer, f.definitionPayload())
	case request.Method == http.MethodPut && f.isDefinitionItem(path):
		writeDolphinOK(writer, f.definitionPayload())
	case request.Method == http.MethodPost && strings.HasSuffix(path, "/release"):
		writeDolphinOK(writer, true)
	default:
		writeDolphinOK(writer, map[string]any{})
	}
}

func (f *dolphinFake) serveV2(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	if request.Method == http.MethodGet && (path == "/projects" || strings.HasSuffix(path, "/projects")) && !strings.Contains(path, "definition") {
		writeDolphinOK(writer, []map[string]any{{"id": 1, "name": "default"}})
		return
	}
	writeDolphinBizError(writer, "DolphinScheduler 2.x API")
}

func (f *dolphinFake) serveSchedules(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		// 记下 3.1/3.2 的 processDefinitionCode 与 3.3+ 的 workflowDefinitionCode。
		// Record processDefinitionCode (3.1/3.2) vs workflowDefinitionCode (3.3+).
		f.lastScheduleQuery = request.URL.Query()
		writeDolphinOK(writer, map[string]any{"totalList": []any{map[string]any{"id": 11}}})
		return
	}
	writeDolphinOK(writer, map[string]any{"id": 11})
}

func (f *dolphinFake) isDefinitionCollection(path string) bool {
	return strings.HasSuffix(path, "/process-definition") || strings.HasSuffix(path, "/workflow-definition")
}

func (f *dolphinFake) isDefinitionItem(path string) bool {
	return strings.Contains(path, "/process-definition/") || strings.Contains(path, "/workflow-definition/")
}

func (f *dolphinFake) definitionPayload() map[string]any {
	tasks := []map[string]any{
		{"code": 1, "name": "up", "taskType": "SHELL", "version": 1, "taskParams": map[string]any{"rawScript": "echo up"}},
		{"code": 88, "name": "stx", "taskType": "SHELL", "version": 1, "taskParams": map[string]any{"rawScript": "old"}},
	}
	relations := []map[string]any{{"preTaskCode": 1, "postTaskCode": 88}}
	if f.family == dolphinFakeWorkflow {
		return map[string]any{
			"workflowDefinition":       f.definition,
			"workflowTaskRelationList": relations,
			"taskDefinitionList":       tasks,
		}
	}
	return map[string]any{
		"processDefinition":       f.definition,
		"processTaskRelationList": relations,
		"taskDefinitionList":      tasks,
	}
}

func (f *dolphinFake) saw(substr string) bool {
	for _, path := range f.paths {
		if strings.Contains(path, substr) {
			return true
		}
	}
	return false
}

func (f *dolphinFake) count(substr string) int {
	n := 0
	for _, path := range f.paths {
		if strings.Contains(path, substr) {
			n++
		}
	}
	return n
}

func writeDolphinHTMLFallback(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/html")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("<!DOCTYPE html><html><body>DolphinScheduler UI</body></html>"))
}

func writeDolphinBizError(writer http.ResponseWriter, msg string) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"code": 10007, "msg": msg, "data": nil})
}

func parseFormInt(form url.Values, key string) int64 {
	value, _ := strconv.ParseInt(form.Get(key), 10, 64)
	return value
}
