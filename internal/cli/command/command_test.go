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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

type fakeClient struct {
	capabilityRequestID string
	capabilities        cliClient.CapabilityData
	capabilityErr       error
	requestID           string
	response            any
	requestErr          error
	requestCalls        int
	method              string
	path                string
}

func (f *fakeClient) Capabilities(context.Context) (string, cliClient.CapabilityData, error) {
	return f.capabilityRequestID, f.capabilities, f.capabilityErr
}

func (f *fakeClient) Request(_ context.Context, method, path string, _ any, result any) (string, error) {
	f.requestCalls++
	f.method = method
	f.path = path
	if f.requestErr != nil {
		return "", f.requestErr
	}
	target, ok := result.(*any)
	if !ok {
		return "", errors.New("unexpected result type")
	}
	*target = f.response
	return f.requestID, nil
}

func TestBuildExecutesGETWithPathQueryNamespaceAndPick(t *testing.T) {
	spec := testSpec()
	client := &fakeClient{
		capabilityRequestID: "req_capability",
		capabilities: cliClient.CapabilityData{Operations: []cliClient.CapabilityOperation{{
			OperationID: spec.ID,
			Revision:    spec.Revision,
			Allowed:     true,
			Mode:        string(spec.Mode),
		}}},
		requestID: "req_business",
		response:  map[string]any{"name": "node-1", "secret": "masked"},
	}
	var receivedNamespace string
	commands, err := Build([]operation.OperationSpec{spec}, func(namespace string) (Client, error) {
		receivedNamespace = namespace
		return client, nil
	})
	if err != nil {
		t.Fatalf("构建命令失败 / building commands failed: %v", err)
	}

	root, stdout, stderr := testRoot(commands)
	root.SetArgs([]string{"sample", "get", "node/value", "--filter", "ready state", "--namespace", "prod", "--pick", "name"})
	if err := root.Execute(); err != nil {
		t.Fatalf("执行命令失败 / executing command failed: %v", err)
	}
	if receivedNamespace != "prod" {
		t.Fatalf("命名空间错误 / namespace is incorrect: %q", receivedNamespace)
	}
	if client.method != "GET" || client.path != "/api/v1/samples/node%2Fvalue?filter=ready+state" {
		t.Fatalf("请求错误 / request is incorrect: method=%s path=%s", client.method, client.path)
	}
	if stderr.Len() != 0 {
		t.Fatalf("成功命令不应写 stderr / successful command must not write stderr: %s", stderr.String())
	}
	var result clioutput.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout 不是合法 JSON / stdout is not valid JSON: %v", err)
	}
	data, ok := result.Data.(map[string]any)
	if !ok || data["name"] != "node-1" || len(data) != 1 {
		t.Fatalf("字段选择结果错误 / picked data is incorrect: %#v", result.Data)
	}
}

func TestBuildHelpDoesNotCreateClient(t *testing.T) {
	var factoryCalls int
	commands, err := Build([]operation.OperationSpec{testSpec()}, func(string) (Client, error) {
		factoryCalls++
		return &fakeClient{}, nil
	})
	if err != nil {
		t.Fatalf("构建命令失败 / building commands failed: %v", err)
	}
	root, stdout, _ := testRoot(commands)
	root.SetArgs([]string{"sample", "get", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("显示帮助失败 / showing help failed: %v", err)
	}
	if factoryCalls != 0 {
		t.Fatalf("显示帮助不应创建客户端 / help must not create a client: calls=%d", factoryCalls)
	}
	if !strings.Contains(stdout.String(), "stx sample get example") || !strings.Contains(stdout.String(), "Output example") {
		t.Fatalf("帮助缺少登记样例 / help is missing registry examples: %s", stdout.String())
	}
}

func TestBuildStopsBeforeBusinessRequestWhenCapabilityRejects(t *testing.T) {
	spec := testSpec()
	tests := []struct {
		name      string
		remote    *cliClient.CapabilityOperation
		expected  string
		exitCode  clioutput.ExitCode
		requestID string
	}{
		{name: "missing", expected: clioutput.CodeNotFound, exitCode: clioutput.ExitNotFound, requestID: "req_missing"},
		{name: "denied", remote: &cliClient.CapabilityOperation{OperationID: spec.ID, Revision: 1, Allowed: false, DenialCode: "admin_required", Mode: string(spec.Mode)}, expected: clioutput.CodePermission, exitCode: clioutput.ExitPermission, requestID: "req_denied"},
		{name: "old revision", remote: &cliClient.CapabilityOperation{OperationID: spec.ID, Revision: 0, Allowed: true, Mode: string(spec.Mode)}, expected: clioutput.CodeConflict, exitCode: clioutput.ExitConflict, requestID: "req_revision"},
		{name: "different mode", remote: &cliClient.CapabilityOperation{OperationID: spec.ID, Revision: 1, Allowed: true, Mode: "watch"}, expected: clioutput.CodeConflict, exitCode: clioutput.ExitConflict, requestID: "req_mode"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operations := []cliClient.CapabilityOperation(nil)
			if test.remote != nil {
				operations = append(operations, *test.remote)
			}
			client := &fakeClient{capabilityRequestID: test.requestID, capabilities: cliClient.CapabilityData{Operations: operations}}
			commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return client, nil })
			if err != nil {
				t.Fatalf("构建命令失败 / building commands failed: %v", err)
			}
			root, _, _ := testRoot(commands)
			root.SetArgs([]string{"sample", "get", "one"})
			err = root.Execute()
			classified := clioutput.ClassifyError(err)
			if classified.Code != test.expected || classified.ExitCode != test.exitCode || classified.RequestID != test.requestID {
				t.Fatalf("能力错误错误 / capability error is incorrect: %#v", classified)
			}
			if client.requestCalls != 0 {
				t.Fatalf("能力检查失败后不应调用业务接口 / business API must not run after capability rejection: %d", client.requestCalls)
			}
		})
	}
}

func TestBuildRejectsUnsupportedOrMismatchedSpecs(t *testing.T) {
	tests := []struct {
		name string
		spec operation.OperationSpec
	}{
		{name: "post", spec: func() operation.OperationSpec { item := testSpec(); item.Method = "POST"; return item }()},
		{name: "watch", spec: func() operation.OperationSpec { item := testSpec(); item.Mode = operation.ModeWatch; return item }()},
		{name: "missing summary", spec: func() operation.OperationSpec { item := testSpec(); item.Summary = ""; return item }()},
		{name: "missing path input", spec: func() operation.OperationSpec { item := testSpec(); item.Input = item.Input[1:]; return item }()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build([]operation.OperationSpec{test.spec}, func(string) (Client, error) { return &fakeClient{}, nil })
			if err == nil {
				t.Fatal("非法登记项应该被拒绝 / invalid registry entry must be rejected")
			}
		})
	}
}

func testSpec() operation.OperationSpec {
	return operation.OperationSpec{
		ID:           "sample.get",
		CommandPath:  []string{"sample", "get"},
		Summary:      "Get one sample",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/samples/:id",
		Mode:         operation.ModeNormal,
		AuthRequired: true,
		Risk:         operation.RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []operation.InputSpec{
			{Name: "id", Location: operation.InputPath, Required: true, Description: "Sample ID"},
			{Name: "filter", Location: operation.InputQuery, Required: false, Description: "Sample filter"},
		},
		Example:       "stx sample get example",
		OutputExample: `{"api_version":"v1","operation_id":"sample.get","request_id":"req_example","data":{},"result_meta":{"complete":true}}`,
	}
}

func testRoot(commands []*cobra.Command) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(commands...)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root, stdout, stderr
}
