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
	"path/filepath"
	"strings"
	"testing"

	cliconfig "github.com/LeonYoah/stx/internal/cli/config"
)

func TestNamespaceCommandsDoNotExposeToken(t *testing.T) {
	store := cliconfig.NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	if err := store.Upsert("production", cliconfig.Namespace{
		Server: "https://stx.example.com",
		Token:  "super-secret-token",
		Output: "json",
	}, true); err != nil {
		t.Fatalf("准备命名空间失败 / preparing namespace failed: %v", err)
	}
	provider := func() (*cliconfig.Store, error) { return store, nil }

	for _, args := range [][]string{{"list"}, {"show", "production"}} {
		command := newNamespaceCommandWithStore(provider)
		var stdout bytes.Buffer
		command.SetOut(&stdout)
		command.SetErr(&stdout)
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("执行 namespace %v 失败 / executing namespace %v failed: %v", args, args, err)
		}
		if strings.Contains(stdout.String(), "super-secret-token") {
			t.Fatalf("命令输出泄露令牌 / command output exposed token: %s", stdout.String())
		}
	}
}

func TestNamespaceUseAndDeleteCommands(t *testing.T) {
	store := cliconfig.NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	if err := store.Upsert("one", cliconfig.Namespace{Server: "https://one.example.com"}, true); err != nil {
		t.Fatalf("准备 one 失败 / preparing one failed: %v", err)
	}
	if err := store.Upsert("two", cliconfig.Namespace{Server: "https://two.example.com"}, false); err != nil {
		t.Fatalf("准备 two 失败 / preparing two failed: %v", err)
	}
	provider := func() (*cliconfig.Store, error) { return store, nil }

	useCommand := newNamespaceCommandWithStore(provider)
	useCommand.SetOut(&bytes.Buffer{})
	useCommand.SetErr(&bytes.Buffer{})
	useCommand.SetArgs([]string{"use", "two"})
	if err := useCommand.Execute(); err != nil {
		t.Fatalf("切换命名空间失败 / selecting namespace failed: %v", err)
	}

	deleteCommand := newNamespaceCommandWithStore(provider)
	deleteCommand.SetOut(&bytes.Buffer{})
	deleteCommand.SetErr(&bytes.Buffer{})
	deleteCommand.SetArgs([]string{"delete", "two"})
	if err := deleteCommand.Execute(); err != nil {
		t.Fatalf("删除命名空间失败 / deleting namespace failed: %v", err)
	}

	file, err := store.Load()
	if err != nil {
		t.Fatalf("读取最终配置失败 / loading final config failed: %v", err)
	}
	if file.CurrentNamespace != "one" {
		t.Fatalf("删除当前项后应选择 one / one must become current after deletion: %s", file.CurrentNamespace)
	}
}
