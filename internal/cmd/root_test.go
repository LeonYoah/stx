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
package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestRootCommandWithoutArgumentsShowsHelp(t *testing.T) {
	var serverCalls int
	command := newRootCommand(func() error {
		serverCalls++
		return nil
	})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stdout)
	command.SetArgs([]string{})

	if err := command.Execute(); err != nil {
		t.Fatalf("执行根命令失败 / executing root command failed: %v", err)
	}
	if serverCalls != 0 {
		t.Fatalf("显示帮助时不应启动服务 / help must not start the server: calls=%d", serverCalls)
	}
	if !strings.Contains(stdout.String(), "Usage:") || !strings.Contains(stdout.String(), "Start the STX API server") {
		t.Fatalf("根命令帮助内容不完整 / root help is incomplete: %s", stdout.String())
	}
}

func TestRootCommandRegistersOnlyPublicServerEntry(t *testing.T) {
	command := newRootCommand(func() error { return nil })
	children := make(map[string]bool)
	for _, child := range command.Commands() {
		children[child.Name()] = child.Hidden
	}

	if hidden, ok := children["server"]; !ok || hidden {
		t.Fatalf("server 命令应公开 / server command must be public: %#v", children)
	}
	if hidden, ok := children["api"]; !ok || !hidden {
		t.Fatalf("api 兼容命令应隐藏 / api compatibility command must be hidden: %#v", children)
	}
	if _, ok := children["scheduler"]; ok {
		t.Fatalf("scheduler 不应注册 / scheduler must not be registered")
	}
	if _, ok := children["worker"]; ok {
		t.Fatalf("worker 不应注册 / worker must not be registered")
	}
	for _, name := range []string{"auth", "admin", "dashboard", "host", "cluster", "config", "package", "plugin"} {
		if hidden, ok := children[name]; !ok || hidden {
			t.Fatalf("%s API 命令组应公开 / %s API command group must be public: %#v", name, name, children)
		}
	}
}

func TestRootCommandRegistersPackageInstallerAndPluginReadCommands(t *testing.T) {
	command := newRootCommand(func() error { return nil })
	paths := [][]string{
		{"package", "list"},
		{"package", "get"},
		{"package", "download", "list"},
		{"package", "download", "get"},
		{"host", "install", "status", "get"},
		{"plugin", "list"},
		{"plugin", "get"},
		{"plugin", "local", "list"},
		{"plugin", "download", "list"},
		{"plugin", "download", "status", "get"},
		{"plugin", "dependency", "list"},
		{"plugin", "official-dependency", "list"},
		{"cluster", "plugin", "list"},
		{"cluster", "plugin", "progress", "get"},
	}

	for _, path := range paths {
		found, remaining, err := command.Find(path)
		if err != nil {
			t.Fatalf("查找命令失败 / finding command failed: path=%v err=%v", path, err)
		}
		if len(remaining) != 0 || found.Name() != path[len(path)-1] {
			t.Fatalf("命令路径未完整注册 / command path is not fully registered: path=%v found=%s remaining=%v", path, found.CommandPath(), remaining)
		}
	}
}

func TestRootCommandRegistersAuthAdminAndDashboardReadCommands(t *testing.T) {
	command := newRootCommand(func() error { return nil })
	paths := [][]string{
		{"auth", "user-info", "get"},
		{"admin", "user", "list"},
		{"admin", "user", "get"},
		{"dashboard", "overview", "get"},
		{"dashboard", "stats", "get"},
		{"dashboard", "cluster", "list"},
		{"dashboard", "host", "list"},
		{"dashboard", "activity", "list"},
	}

	for _, path := range paths {
		found, remaining, err := command.Find(path)
		if err != nil {
			t.Fatalf("查找命令失败 / finding command failed: path=%v err=%v", path, err)
		}
		if len(remaining) != 0 || found.Name() != path[len(path)-1] {
			t.Fatalf("命令路径未完整注册 / command path is not fully registered: path=%v found=%s remaining=%v", path, found.CommandPath(), remaining)
		}
	}
}

func TestRootCommandRegistersClusterReadWriteAndProcessCommands(t *testing.T) {
	command := newRootCommand(func() error { return nil })
	paths := [][]string{
		{"cluster", "create"}, {"cluster", "update"},
		{"cluster", "node", "add"}, {"cluster", "node", "add-batch"}, {"cluster", "node", "update"},
		{"cluster", "node", "precheck"}, {"cluster", "node", "logs"},
		{"cluster", "start"}, {"cluster", "stop"}, {"cluster", "restart"},
		{"cluster", "node", "start"}, {"cluster", "node", "stop"}, {"cluster", "node", "restart"},
		{"cluster", "java-proxy", "status"}, {"cluster", "java-proxy", "logs"},
		{"cluster", "java-proxy", "start"}, {"cluster", "java-proxy", "stop"}, {"cluster", "java-proxy", "restart"},
	}
	for _, path := range paths {
		found, remaining, err := command.Find(path)
		if err != nil || len(remaining) != 0 || found.Name() != path[len(path)-1] {
			t.Fatalf("集群命令未完整注册: path=%v found=%s remaining=%v err=%v", path, found.CommandPath(), remaining, err)
		}
	}
}

func TestRootCommandDoesNotExposeDeleteOrRemoveCommands(t *testing.T) {
	root := newRootCommand(func() error { return nil })
	var visit func(command *cobra.Command)
	visit = func(command *cobra.Command) {
		for _, child := range command.Commands() {
			if child.Name() == "delete" || child.Name() == "remove" {
				t.Fatalf("CLI 不应暴露删除命令: %s / delete commands must not be exposed", child.CommandPath())
			}
			visit(child)
		}
	}
	visit(root)
}

func TestRootCommandRegistersHostDiscoveryProcessCommand(t *testing.T) {
	command := newRootCommand(func() error { return nil })
	path := []string{"host", "discovery", "process", "list"}
	found, remaining, err := command.Find(path)
	if err != nil {
		t.Fatalf("查找发现命令失败 / finding discovery command failed: %v", err)
	}
	if len(remaining) != 0 || found.Name() != "list" {
		t.Fatalf("发现命令路径未完整注册 / discovery command path is not fully registered: found=%s remaining=%v", found.CommandPath(), remaining)
	}
}

func TestServerAndAPICommandsUseSameRunner(t *testing.T) {
	var serverCalls int
	runner := func() error {
		serverCalls++
		return nil
	}

	serverCommand := newRootCommand(runner)
	serverCommand.SetArgs([]string{"server"})
	if err := serverCommand.Execute(); err != nil {
		t.Fatalf("执行 server 命令失败 / executing server command failed: %v", err)
	}

	apiCommand := newRootCommand(runner)
	apiCommand.SetOut(&bytes.Buffer{})
	apiCommand.SetErr(&bytes.Buffer{})
	apiCommand.SetArgs([]string{"api"})
	if err := apiCommand.Execute(); err != nil {
		t.Fatalf("执行 api 兼容命令失败 / executing api compatibility command failed: %v", err)
	}

	if serverCalls != 2 {
		t.Fatalf("server 和 api 应复用启动流程 / server and api must share the runner: calls=%d", serverCalls)
	}
}

func TestExecuteCommandWritesStructuredErrorAndStableExitCode(t *testing.T) {
	rootCommand := newRootCommand(func() error {
		return clioutput.NewError(clioutput.CodePermission, "admin required", clioutput.ExitPermission, false)
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := executeCommand(rootCommand, []string{"server"}, &stdout, &stderr)
	if exitCode != int(clioutput.ExitPermission) {
		t.Fatalf("权限错误退出码错误 / permission error exit code is incorrect: %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("失败命令的 stdout 必须为空 / stdout must be empty on failure: %s", stdout.String())
	}
	var event map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &event); err != nil {
		t.Fatalf("stderr 不是单个合法 JSON 事件 / stderr is not one valid JSON event: %v", err)
	}
	if event["event"] != "error" || event["code"] != clioutput.CodePermission || event["retryable"] != false {
		t.Fatalf("错误事件字段错误 / error event fields are incorrect: %#v", event)
	}
}

func TestExecuteCommandUnknownCommandReturnsUsageExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := executeCommand(newRootCommand(func() error { return nil }), []string{"missing-command"}, &stdout, &stderr)
	if exitCode != int(clioutput.ExitUsage) {
		t.Fatalf("未知命令应返回用法退出码 / unknown command must return usage exit code: %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("未知命令不应写 stdout / unknown command must not write stdout: %s", stdout.String())
	}
	var event map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &event); err != nil {
		t.Fatalf("未知命令 stderr 不是合法 JSON / unknown command stderr is not valid JSON: %v", err)
	}
	if event["code"] != clioutput.CodeUsage {
		t.Fatalf("未知命令错误分类错误 / unknown command classification is incorrect: %#v", event)
	}
}

func TestExecuteCommandNamespaceSupportsGlobalOutputFlags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := executeCommand(newRootCommand(func() error { return nil }), []string{"namespace", "list", "--output", "yaml"}, &stdout, &stderr)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("namespace yaml 执行失败 / namespace yaml execution failed: code=%d stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "operation_id: namespace.list") || !strings.Contains(stdout.String(), "complete: true") {
		t.Fatalf("namespace yaml 输出错误 / namespace yaml output is incorrect: %s", stdout.String())
	}
}
