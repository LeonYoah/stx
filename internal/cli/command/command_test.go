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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

type fakeClient struct {
	capabilityCalls     int
	capabilityRequestID string
	capabilities        cliClient.CapabilityData
	capabilityErr       error
	requestID           string
	response            any
	requestErr          error
	requestCalls        int
	method              string
	path                string
	body                any
	headers             map[string]string
}

func (f *fakeClient) Capabilities(context.Context) (string, cliClient.CapabilityData, error) {
	f.capabilityCalls++
	return f.capabilityRequestID, f.capabilities, f.capabilityErr
}

func (f *fakeClient) Request(_ context.Context, method, path string, body any, result any) (string, error) {
	return f.request(method, path, body, nil, result)
}

func (f *fakeClient) RequestWithHeaders(_ context.Context, method, path string, body any, headers map[string]string, result any) (string, error) {
	return f.request(method, path, body, headers, result)
}

func (f *fakeClient) request(method, path string, body any, headers map[string]string, result any) (string, error) {
	f.requestCalls++
	f.method = method
	f.path = path
	f.body = body
	f.headers = headers
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

func TestBuildExecutesConfirmedBodylessDelete(t *testing.T) {
	spec := testSpec()
	spec.ID = "sample.delete"
	spec.CommandPath = []string{"sample", "delete"}
	spec.Summary = "Delete a sample"
	spec.Method = http.MethodDelete
	spec.Risk = operation.RiskR2
	spec.Impact = &operation.ImpactSpec{Level: operation.RiskR2, Message: "Deleting the sample cannot be undone."}
	spec.Input = append(spec.Input,
		operation.InputSpec{Name: "Idempotency-Key", Location: operation.InputHeader, Required: true, Description: "Stable retry key"},
		operation.InputSpec{Name: "X-STX-Confirm", Location: operation.InputHeader, Required: true, Description: "Explicit confirmation"},
	)
	spec.Example = "stx sample delete one --confirm"
	client := &fakeClient{
		capabilities: cliClient.CapabilityData{Operations: []cliClient.CapabilityOperation{{
			OperationID: spec.ID, Revision: spec.Revision, Allowed: true, Mode: string(spec.Mode),
		}}},
		requestID: "req_delete", response: map[string]any{"deleted": true},
	}
	commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return client, nil })
	if err != nil {
		t.Fatalf("构建 DELETE 命令失败 / building DELETE command failed: %v", err)
	}
	root, _, stderr := testRoot(commands)
	root.SetArgs([]string{"sample", "delete", "one", "--confirm", "--idempotency-key", "delete-one"})
	if err := root.Execute(); err != nil {
		t.Fatalf("执行 DELETE 命令失败 / executing DELETE command failed: %v", err)
	}
	if client.method != http.MethodDelete || client.headers["Idempotency-Key"] != "delete-one" || client.headers["X-STX-Confirm"] != "true" {
		t.Fatalf("DELETE 安全请求错误 / DELETE safety request is incorrect: method=%s headers=%#v", client.method, client.headers)
	}
	if !strings.Contains(stderr.String(), "Deleting the sample cannot be undone") {
		t.Fatalf("缺少影响提示 / impact warning is missing: %s", stderr.String())
	}
}

func TestBuildRejectsRiskWriteWithoutConfirm(t *testing.T) {
	spec := testSpec()
	spec.ID = "sample.stop"
	spec.CommandPath = []string{"sample", "stop"}
	spec.Summary = "Stop a sample"
	spec.Method = http.MethodPost
	spec.Risk = operation.RiskR1
	spec.Impact = &operation.ImpactSpec{Level: operation.RiskR1, Message: "Stops the sample."}
	spec.Example = "stx sample stop one --confirm"
	client := &fakeClient{capabilities: cliClient.CapabilityData{Operations: []cliClient.CapabilityOperation{{
		OperationID: spec.ID, Revision: spec.Revision, Allowed: true, Mode: string(spec.Mode),
	}}}}
	commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return client, nil })
	if err != nil {
		t.Fatalf("构建 POST 命令失败 / building POST command failed: %v", err)
	}
	root, _, _ := testRoot(commands)
	root.SetArgs([]string{"sample", "stop", "one"})
	classified := clioutput.ClassifyError(root.Execute())
	if classified.Code != clioutput.CodeConflict || client.capabilityCalls != 0 || client.requestCalls != 0 {
		t.Fatalf("缺少确认时仍执行请求 / request executed without confirmation: error=%#v capability_calls=%d calls=%d", classified, client.capabilityCalls, client.requestCalls)
	}
}

func TestBuildExecutesBodylessR0POST(t *testing.T) {
	spec := testSpec()
	spec.ID = "sample.scan"
	spec.CommandPath = []string{"sample", "scan"}
	spec.Summary = "Scan samples"
	spec.Method = http.MethodPost
	spec.Example = "stx sample scan example"
	spec.OutputExample = `{"api_version":"v1","operation_id":"sample.scan","request_id":"req_example","data":{},"result_meta":{"complete":true}}`
	client := &fakeClient{
		capabilities: cliClient.CapabilityData{Operations: []cliClient.CapabilityOperation{{
			OperationID: spec.ID,
			Revision:    spec.Revision,
			Allowed:     true,
			Mode:        string(spec.Mode),
		}}},
		requestID: "req_business",
		response:  map[string]any{"success": true},
	}
	commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return client, nil })
	if err != nil {
		t.Fatalf("构建 POST 命令失败 / building POST command failed: %v", err)
	}
	root, _, _ := testRoot(commands)
	root.SetArgs([]string{"sample", "scan", "one"})
	if err := root.Execute(); err != nil {
		t.Fatalf("执行 POST 命令失败 / executing POST command failed: %v", err)
	}
	if client.method != http.MethodPost || client.path != "/api/v1/samples/one" || client.body != nil {
		t.Fatalf("POST 请求错误 / POST request is incorrect: method=%s path=%s body=%#v", client.method, client.path, client.body)
	}
}

func TestBuildExecutesPUTWithJSONRequestFile(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(requestFile, []byte(`{"port":18082,"restart":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := testSpec()
	spec.ID = "sample.update"
	spec.CommandPath = []string{"sample", "update"}
	spec.Summary = "Update a sample"
	spec.Method = http.MethodPut
	spec.Risk = operation.RiskR1
	spec.Impact = &operation.ImpactSpec{Level: operation.RiskR1, Message: "Updates the sample."}
	spec.Input = append(spec.Input,
		operation.InputSpec{Name: "request", Location: operation.InputBody, Required: true, Description: "Complete update request"},
		operation.InputSpec{Name: "Idempotency-Key", Location: operation.InputHeader, Required: true, Description: "Stable retry key"},
		operation.InputSpec{Name: "X-STX-Confirm", Location: operation.InputHeader, Required: true, Description: "Explicit confirmation"},
	)
	spec.Example = "stx sample update one --request-file request.json --confirm"
	client := &fakeClient{
		capabilities: cliClient.CapabilityData{Operations: []cliClient.CapabilityOperation{{
			OperationID: spec.ID, Revision: spec.Revision, Allowed: true, Mode: string(spec.Mode),
		}}},
		requestID: "req_update", response: map[string]any{"updated": true},
	}
	commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return client, nil })
	if err != nil {
		t.Fatalf("构建 PUT 命令失败 / building PUT command failed: %v", err)
	}
	root, _, _ := testRoot(commands)
	root.SetArgs([]string{"sample", "update", "one", "--filter", "active", "--request-file", requestFile, "--confirm", "--idempotency-key", "update-one"})
	if err := root.Execute(); err != nil {
		t.Fatalf("执行 PUT 命令失败 / executing PUT command failed: %v", err)
	}
	if client.method != http.MethodPut || client.path != "/api/v1/samples/one?filter=active" {
		t.Fatalf("PUT 请求错误 / PUT request is incorrect: method=%s path=%s", client.method, client.path)
	}
	if client.headers["Idempotency-Key"] != "update-one" || client.headers["X-STX-Confirm"] != "true" {
		t.Fatalf("PUT 安全请求头错误 / PUT safety headers are incorrect: %#v", client.headers)
	}
	var body map[string]any
	raw, ok := client.body.(json.RawMessage)
	if !ok || json.Unmarshal(raw, &body) != nil || body["port"] != float64(18082) || body["restart"] != false {
		t.Fatalf("PUT 请求正文错误 / PUT request body is incorrect: %#v", client.body)
	}
}

func TestBuildRequiresValidRequestFileBeforeNetwork(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "missing"},
		{name: "invalid", file: filepath.Join(t.TempDir(), "invalid.json")},
	}
	if err := os.WriteFile(tests[1].file, []byte(`{"port":`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := testSpec()
			spec.ID = "sample.update"
			spec.CommandPath = []string{"sample", "update"}
			spec.Summary = "Update a sample"
			spec.Method = http.MethodPatch
			spec.Risk = operation.RiskR1
			spec.Impact = &operation.ImpactSpec{Level: operation.RiskR1, Message: "Updates the sample."}
			spec.Input = append(spec.Input, operation.InputSpec{Name: "request", Location: operation.InputBody, Required: true, Description: "Complete update request"})
			client := &fakeClient{}
			commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return client, nil })
			if err != nil {
				t.Fatal(err)
			}
			root, _, _ := testRoot(commands)
			args := []string{"sample", "update", "one", "--confirm"}
			if test.file != "" {
				args = append(args, "--request-file", test.file)
			}
			root.SetArgs(args)
			classified := clioutput.ClassifyError(root.Execute())
			if classified.Code != clioutput.CodeUsage || client.capabilityCalls != 0 || client.requestCalls != 0 {
				t.Fatalf("请求文件错误处理不正确 / request file error is incorrect: error=%#v capability_calls=%d request_calls=%d", classified, client.capabilityCalls, client.requestCalls)
			}
		})
	}
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

func TestBuildExecutesGETWithRepeatedQueryValues(t *testing.T) {
	spec := testSpec()
	spec.Input = append(spec.Input, operation.InputSpec{
		Name:        "profile_keys",
		Location:    operation.InputQuery,
		Repeated:    true,
		Description: "Dependency profile keys",
	})
	client := &fakeClient{
		capabilities: cliClient.CapabilityData{Operations: []cliClient.CapabilityOperation{{
			OperationID: spec.ID,
			Revision:    spec.Revision,
			Allowed:     true,
			Mode:        string(spec.Mode),
		}}},
		response: map[string]any{"status": "ready"},
	}
	commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return client, nil })
	if err != nil {
		t.Fatalf("构建重复查询参数命令失败 / building command with repeated query values failed: %v", err)
	}

	root, _, _ := testRoot(commands)
	root.SetArgs([]string{
		"sample", "get", "one",
		"--profile_keys", " jdbc ",
		"--profile_keys", "cdc",
		"--profile_keys", " ",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("执行重复查询参数命令失败 / executing command with repeated query values failed: %v", err)
	}
	if client.path != "/api/v1/samples/one?profile_keys=jdbc&profile_keys=cdc" {
		t.Fatalf("重复查询参数错误 / repeated query values are incorrect: %s", client.path)
	}
}

func TestBuildRequiresOneNonEmptyRepeatedQueryValue(t *testing.T) {
	spec := testSpec()
	spec.Input = append(spec.Input, operation.InputSpec{
		Name:        "profile_keys",
		Location:    operation.InputQuery,
		Required:    true,
		Repeated:    true,
		Description: "Dependency profile keys",
	})
	commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) { return &fakeClient{}, nil })
	if err != nil {
		t.Fatalf("构建必填重复查询参数命令失败 / building command with required repeated query values failed: %v", err)
	}

	root, _, _ := testRoot(commands)
	root.SetArgs([]string{"sample", "get", "one", "--profile_keys", " "})
	err = root.Execute()
	classified := clioutput.ClassifyError(err)
	if classified.Code != clioutput.CodeUsage || classified.ExitCode != clioutput.ExitUsage {
		t.Fatalf("必填重复查询参数错误分类不正确 / required repeated query error is incorrect: %#v", classified)
	}
}

func TestBuildHelpDoesNotCreateClient(t *testing.T) {
	var factoryCalls int
	spec := testSpec()
	spec.Impact = &operation.ImpactSpec{
		Level:       operation.RiskR0,
		Message:     "Reads process metadata without changing the target process.",
		Performance: "Runs one short process scan.",
	}
	commands, err := Build([]operation.OperationSpec{spec}, func(string) (Client, error) {
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
	if !strings.Contains(stdout.String(), "stx sample get example") || !strings.Contains(stdout.String(), "Output example") ||
		!strings.Contains(stdout.String(), "Risk level: R0") || !strings.Contains(stdout.String(), "Runs one short process scan.") {
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
		{name: "get body", spec: func() operation.OperationSpec {
			item := testSpec()
			item.Input = append(item.Input, operation.InputSpec{Name: "request", Location: operation.InputBody, Required: true, Description: "Request body"})
			return item
		}()},
		{name: "post header", spec: func() operation.OperationSpec {
			item := testSpec()
			item.Method = "POST"
			item.Input = append(item.Input, operation.InputSpec{Name: "X-Test", Location: operation.InputHeader, Required: true, Description: "Request header"})
			return item
		}()},
		{name: "post file", spec: func() operation.OperationSpec {
			item := testSpec()
			item.Method = "POST"
			item.Input = append(item.Input, operation.InputSpec{Name: "file", Location: operation.InputFile, Required: true, Description: "Request file"})
			return item
		}()},
		{name: "watch", spec: func() operation.OperationSpec { item := testSpec(); item.Mode = operation.ModeWatch; return item }()},
		{name: "missing summary", spec: func() operation.OperationSpec { item := testSpec(); item.Summary = ""; return item }()},
		{name: "missing path input", spec: func() operation.OperationSpec { item := testSpec(); item.Input = item.Input[1:]; return item }()},
		{name: "repeated path input", spec: func() operation.OperationSpec {
			item := testSpec()
			item.Input[0].Repeated = true
			return item
		}()},
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
