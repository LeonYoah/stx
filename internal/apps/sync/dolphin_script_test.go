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

func TestBuildDolphinShellScriptOmitsVarWithoutOverrides(t *testing.T) {
	script := BuildDolphinShellScript(&Task{ID: 42, Name: "orders_sync"}, "6h", nil, "", "")
	if !strings.Contains(script.Script, "exec stx sync task submit 42 --wait --confirm") {
		t.Fatalf("missing exec submit: %s", script.Script)
	}
	if strings.Contains(script.Script, "--close-job=false") {
		t.Fatalf("batch script should keep default close-job: %s", script.Script)
	}
	if !strings.Contains(script.Script, `--external-ref "ds:${system.workflow.instance.id}:${system.task.definition.code}"`) {
		t.Fatalf("missing external-ref placeholder: %s", script.Script)
	}
	if !strings.Contains(script.Script, `--schedule-time "${system.datetime}"`) {
		t.Fatalf("missing schedule-time placeholder: %s", script.Script)
	}
	if strings.Contains(script.Script, "--var ") {
		t.Fatalf("script should omit --var when there are no overrides: %s", script.Script)
	}
}

func TestBuildDolphinShellScriptAppendsOverrides(t *testing.T) {
	script := BuildDolphinShellScript(&Task{ID: 7, Name: "demo"}, "", map[string]string{"dt": "20260101", "skip": "${unreplaced}"}, "workers", "prod")
	if !strings.Contains(script.Script, "--var dt=20260101") {
		t.Fatalf("missing override: %s", script.Script)
	}
	if strings.Contains(script.Script, "--var skip=") {
		t.Fatalf("unreplaced override should be skipped: %s", script.Script)
	}
	if script.WorkerGroup != "workers" || script.Environment != "prod" {
		t.Fatalf("unexpected worker/env: %+v", script)
	}
}

func TestBuildDolphinShellScriptDisablesCloseJobForStreaming(t *testing.T) {
	script := BuildDolphinShellScript(&Task{ID: 9, Name: "stream", Mode: TaskModeStreaming}, "6h", nil, "", "")
	if !strings.Contains(script.Script, "--wait --close-job=false --confirm") {
		t.Fatalf("streaming script should disable close-job: %s", script.Script)
	}
}

func TestParseScheduleTimeAndUnreplacedVariables(t *testing.T) {
	opts, err := parseSubmitOptions(TaskActionRequest{ScheduleTime: "20260102150405", Variables: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if opts.ScheduleTime.IsZero() {
		t.Fatal("expected schedule time")
	}
	if _, err := parseSubmitOptions(TaskActionRequest{Variables: map[string]string{"k": "${dt}"}}); err == nil {
		t.Fatal("expected unreplaced variable error")
	}
	if _, err := parseSubmitOptions(TaskActionRequest{ScheduleTime: "not-a-time"}); err == nil {
		t.Fatal("expected invalid schedule time")
	}
}

func TestDolphinHasCycle(t *testing.T) {
	relations := []json.RawMessage{
		marshalRaw(dolphinRelation{PreTaskCode: 1, PostTaskCode: 2}),
		marshalRaw(dolphinRelation{PreTaskCode: 2, PostTaskCode: 3}),
	}
	if dolphinHasCycle(relations, nil) {
		t.Fatal("acyclic graph reported a cycle")
	}
	if !dolphinHasCycle(relations, [][2]int64{{3, 1}}) {
		t.Fatal("expected cycle when adding 3 -> 1")
	}
}
