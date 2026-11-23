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

package command

import (
	"net/http"
	"strings"
	"testing"
	"time"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
)

func TestWithLifecycleTimeoutExtendsRestartClient(t *testing.T) {
	t.Parallel()
	base, err := cliClient.New(cliConfig.Resolved{Server: "http://127.0.0.1:17800", Timeout: 30 * time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	adjusted := withLifecycleTimeout(base, operation.OperationSpec{ID: "cluster.restart"})
	if adjusted == base {
		t.Fatal("lifecycle ops must receive a timeout-extended client copy")
	}
	concrete, ok := adjusted.(*cliClient.Client)
	if !ok {
		t.Fatalf("expected *cliClient.Client, got %T", adjusted)
	}
	// WithTimeout 返回独立副本；再包一层证明链式可用，且原客户端保持 30s。
	// WithTimeout returns an isolated copy; re-wrapping proves chaining, base stays at 30s.
	if concrete.WithTimeout(time.Minute) == concrete {
		t.Fatal("WithTimeout must return a distinct client copy")
	}
	unchanged := withLifecycleTimeout(base, operation.OperationSpec{ID: "cluster.get"})
	if unchanged != base {
		t.Fatal("non-lifecycle ops must keep the original client")
	}
}

func TestLifecycleBusinessFailureExitsNonZeroWithNodeSummary(t *testing.T) {
	t.Parallel()
	spec := operation.OperationSpec{
		ID:           "cluster.restart",
		CommandPath:  []string{"cluster", "restart"},
		Summary:      "Restart a cluster",
		GeneratedCLI: true,
		Method:       http.MethodPost,
		Route:        "/api/v1/clusters/:id/restart",
		Mode:         operation.ModeNormal,
		Risk:         operation.RiskR2,
		Revision:     1,
		Impact:       &operation.ImpactSpec{Level: operation.RiskR2, Message: "Restarts all nodes."},
		Input: []operation.InputSpec{
			{Name: "id", Location: operation.InputPath, Required: true, Description: "Cluster ID"},
		},
		Example:       "stx cluster restart 1 --confirm",
		OutputExample: `{"api_version":"v1","operation_id":"cluster.restart","data":{}}`,
	}
	client := &fakeClient{
		capabilities: cliClient.CapabilityData{Operations: []cliClient.CapabilityOperation{{
			OperationID: spec.ID, Revision: spec.Revision, Allowed: true, Mode: string(spec.Mode),
		}}},
		requestID: "req_restart",
		response: map[string]any{
			"success": false,
			"message": "Partial failure: 1/2 nodes succeeded",
			"node_results": []any{
				map[string]any{"node_id": float64(1), "host_name": "n1", "success": true, "message": "ok"},
				map[string]any{"node_id": float64(2), "host_name": "n2", "success": false, "message": "agent not connected"},
			},
		},
	}
	commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return client, nil })
	if err != nil {
		t.Fatal(err)
	}
	root, stdout, _ := testRoot(commands)
	root.SetArgs([]string{"cluster", "restart", "1", "--confirm"})
	classified := clioutput.ClassifyError(root.Execute())
	if classified.Code != clioutput.CodeExecution || classified.ExitCode != clioutput.ExitExecution {
		t.Fatalf("expected execution failure exit, got %#v", classified)
	}
	if !strings.Contains(classified.Message, "n2") || !strings.Contains(classified.Message, "agent not connected") {
		t.Fatalf("failed node summary missing: %s", classified.Message)
	}
	if !strings.Contains(stdout.String(), `"success": false`) && !strings.Contains(stdout.String(), `"success":false`) {
		t.Fatalf("stdout must still contain the operation payload: %s", stdout.String())
	}
}

func TestLifecycleBusinessSuccessStaysZero(t *testing.T) {
	t.Parallel()
	err := lifecycleBusinessFailure(operation.OperationSpec{ID: "cluster.restart"}, map[string]any{
		"success": true,
		"message": "ok",
	}, "req")
	if err != nil {
		t.Fatalf("success must not fail CLI: %v", err)
	}
}
