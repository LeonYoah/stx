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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	"github.com/spf13/cobra"
)

func TestLoginCommandStoresTokenAndDoesNotPrintIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/auth/cli/login" {
			t.Fatalf("登录路径错误: %s", request.URL.Path)
		}
		if request.Header.Get("X-STX-Client") != "cli" {
			t.Fatalf("缺少 CLI 请求头: %q", request.Header.Get("X-STX-Client"))
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取登录请求失败: %v", err)
		}
		if body["password"] != "secret" || body["expires_in"] != "7d" {
			t.Fatalf("登录请求内容错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"token":      "stx_secret_token",
			"token_type": "Bearer",
			"expires_at": "2026-09-25T10:00:00Z",
			"user":       map[string]any{"id": 1, "username": "admin", "is_active": true},
		}})
	}))
	defer server.Close()

	store := cliConfig.NewStore(t.TempDir() + "/config.yaml")
	command := newLoginCommand(authCommandOptions{storeProvider: func() (*cliConfig.Store, error) { return store, nil }, stdin: strings.NewReader("secret\n")})
	command.SetArgs([]string{"--server", server.URL, "--username", "admin", "--password-stdin"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.Root().SetArgs(command.Flags().Args())
	// Use a root command so persistent output flags are available to the login command.
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(command)
	root.SetArgs([]string{"login", "--server", server.URL, "--username", "admin", "--password-stdin"})
	root.PersistentFlags().String("output", "json", "output")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("登录命令失败: %v", err)
	}

	resolved, err := store.Resolve(cliConfig.Overrides{Namespace: "default"})
	if err != nil {
		t.Fatalf("读取登录配置失败: %v", err)
	}
	if resolved.Token != "stx_secret_token" {
		t.Fatalf("配置没有保存令牌: %q", resolved.Token)
	}
	if strings.Contains(root.OutOrStdout().(*bytes.Buffer).String(), "stx_secret_token") {
		t.Fatal("登录结果不应把令牌原文写入 stdout")
	}
}

func TestReadLoginPasswordRequiresStdinInNonTTY(t *testing.T) {
	command := &cobra.Command{}
	_, err := readLoginPassword(command, strings.NewReader("secret\n"), false)
	if err == nil || !strings.Contains(err.Error(), "password-stdin") {
		t.Fatalf("非 TTY 登录错误错误: %v", err)
	}
}
