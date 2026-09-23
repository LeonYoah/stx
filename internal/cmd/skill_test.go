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
	"os"
	"path/filepath"
	"strings"
	"testing"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	cliskill "github.com/LeonYoah/stx/internal/cli/skill"
)

func TestSkillShowSupportsEnglishAndDefaultsToChinese(t *testing.T) {
	options := skillCommandOptions{
		homeProvider: func() (string, error) { return t.TempDir(), nil },
		getenv:       func(string) string { return "" },
	}
	for _, test := range []struct {
		args     []string
		contains string
	}{
		{args: []string{"show"}, contains: "通过 `stx` 与远端 STX 服务交互"},
		{args: []string{"show", "--language", "en"}, contains: "Use `stx` to interact with a remote STX service"},
	} {
		command := newSkillCommandWithOptions(options)
		var stdout bytes.Buffer
		command.SetOut(&stdout)
		command.SetErr(&bytes.Buffer{})
		command.SetArgs(test.args)
		if err := command.Execute(); err != nil {
			t.Fatalf("执行 skill show 失败 / executing skill show failed: %v", err)
		}
		if !strings.Contains(stdout.String(), test.contains) || strings.Contains(stdout.String(), `"api_version"`) {
			t.Fatalf("skill show 应原样输出 Skill / skill show must print raw Skill content: %s", stdout.String())
		}
	}
}

func TestSkillInstallUsesMachineEnglishLocale(t *testing.T) {
	home := t.TempDir()
	options := skillCommandOptions{
		homeProvider: func() (string, error) { return home, nil },
		getenv: func(key string) string {
			if key == "LANG" {
				return "en_US.UTF-8"
			}
			return ""
		},
	}
	command := newSkillCommandWithOptions(options)
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"install"})
	if err := command.Execute(); err != nil {
		t.Fatalf("安装 Skill 失败 / installing Skill failed: %v", err)
	}
	var result clioutput.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("安装输出不是 JSON / install output is not JSON: %v\n%s", err, stdout.String())
	}
	encodedData, _ := json.Marshal(result.Data)
	if !strings.Contains(string(encodedData), `"language":"en"`) {
		t.Fatalf("安装结果未选择英文 / install result did not select English: %s", encodedData)
	}
	english, _ := cliskill.Source(cliskill.LanguageEnglish)
	for _, path := range []string{
		filepath.Join(home, ".claude", "skills", "stx", "SKILL.md"),
		filepath.Join(home, ".agents", "skills", "stx", "SKILL.md"),
	} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != string(english) {
			t.Fatalf("英文 Skill 未安装 / English Skill was not installed: path=%s err=%v", path, err)
		}
	}
}

func TestSkillCommandRejectsUnsupportedTarget(t *testing.T) {
	command := newSkillCommandWithOptions(skillCommandOptions{
		homeProvider: func() (string, error) { return t.TempDir(), nil },
		getenv:       func(string) string { return "" },
	})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"install", "--target", "custom"})
	err := command.Execute()
	classified := clioutput.ClassifyError(err)
	if classified.ExitCode != clioutput.ExitUsage {
		t.Fatalf("无效目标应返回用法错误 / invalid target must return usage error: %#v", classified)
	}
}
