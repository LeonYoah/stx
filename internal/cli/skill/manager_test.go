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

package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveLanguageUsesEnglishOnlyForEnglishLocale(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		locale   string
		expected Language
	}{
		{name: "explicit Chinese", value: "zh-CN", expected: LanguageChinese},
		{name: "explicit English", value: "en", expected: LanguageEnglish},
		{name: "English locale", value: "auto", locale: "en_US.UTF-8", expected: LanguageEnglish},
		{name: "Chinese locale", value: "auto", locale: "zh_CN.UTF-8", expected: LanguageChinese},
		{name: "unsupported locale defaults Chinese", value: "auto", locale: "fr_FR.UTF-8", expected: LanguageChinese},
		{name: "missing locale defaults Chinese", value: "auto", expected: LanguageChinese},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			language, err := ResolveLanguage(test.value, func(key string) string {
				if key == "LANG" {
					return test.locale
				}
				return ""
			})
			if err != nil {
				t.Fatalf("解析语言失败 / resolving language failed: %v", err)
			}
			if language != test.expected {
				t.Fatalf("语言不匹配 / language mismatch: got=%s want=%s", language, test.expected)
			}
		})
	}
	if _, err := ResolveLanguage("ja", nil); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("无效显式语言应被拒绝 / invalid explicit language must be rejected: %v", err)
	}
}

func TestEmbeddedSourcesAreValidDistinctSkills(t *testing.T) {
	chinese, err := Source(LanguageChinese)
	if err != nil {
		t.Fatalf("读取中文 Skill 失败 / reading Chinese Skill failed: %v", err)
	}
	english, err := Source(LanguageEnglish)
	if err != nil {
		t.Fatalf("读取英文 Skill 失败 / reading English Skill failed: %v", err)
	}
	for language, content := range map[Language][]byte{LanguageChinese: chinese, LanguageEnglish: english} {
		text := string(content)
		if !strings.HasPrefix(text, "---\nname: stx\n") || !strings.Contains(text, "stx capability list") {
			t.Fatalf("Skill 内容不完整 / Skill content is incomplete: language=%s", language)
		}
	}
	if string(chinese) == string(english) {
		t.Fatal("中英文 Skill 不应相同 / Chinese and English Skills must differ")
	}
}

func TestManagerInstallUpdateBackupAndRestore(t *testing.T) {
	home := t.TempDir()
	manager := NewManager(home)
	backupIDs := []string{"update-backup", "manual-backup", "restore-safety"}
	manager.newBackupID = func() string {
		id := backupIDs[0]
		backupIDs = backupIDs[1:]
		return id
	}
	targets := []Target{TargetClaude, TargetAgents}

	changes, err := manager.Install(LanguageChinese, targets)
	if err != nil {
		t.Fatalf("安装 Skill 失败 / installing Skill failed: %v", err)
	}
	if len(changes) != 2 || changes[0].Status != "installed" || changes[1].Status != "installed" {
		t.Fatalf("安装结果不正确 / unexpected install result: %#v", changes)
	}
	chinese, _ := Source(LanguageChinese)
	claudePath := filepath.Join(home, ".claude", "skills", "stx", "SKILL.md")
	if content, err := os.ReadFile(claudePath); err != nil || string(content) != string(chinese) {
		t.Fatalf("中文 Skill 未正确写入 / Chinese Skill was not written: err=%v", err)
	}

	changes, err = manager.Install(LanguageEnglish, targets)
	if err != nil {
		t.Fatalf("重复安装失败 / repeated install failed: %v", err)
	}
	if changes[0].Status != "skipped_existing" || changes[1].Status != "skipped_existing" {
		t.Fatalf("install 不应覆盖已有文件 / install must not overwrite existing files: %#v", changes)
	}

	changes, err = manager.Update(LanguageEnglish, []Target{TargetClaude})
	if err != nil {
		t.Fatalf("更新 Skill 失败 / updating Skill failed: %v", err)
	}
	if changes[0].Status != "updated" || changes[0].BackupID != "update-backup" {
		t.Fatalf("更新未生成备份 / update did not create a backup: %#v", changes)
	}
	backupPath := filepath.Join(home, ".stx", "skill-backups", "claude", "update-backup", "SKILL.md")
	if content, err := os.ReadFile(backupPath); err != nil || string(content) != string(chinese) {
		t.Fatalf("更新备份不正确 / update backup is incorrect: err=%v", err)
	}

	changes, err = manager.Backup([]Target{TargetClaude})
	if err != nil {
		t.Fatalf("手工备份失败 / manual backup failed: %v", err)
	}
	if changes[0].BackupID != "manual-backup" || changes[0].Language != LanguageEnglish {
		t.Fatalf("手工备份结果不正确 / unexpected manual backup result: %#v", changes)
	}

	changes, err = manager.Restore([]Target{TargetClaude}, "update-backup")
	if err != nil {
		t.Fatalf("恢复 Skill 失败 / restoring Skill failed: %v", err)
	}
	if changes[0].Status != "restored" || changes[0].SafetyBackupID != "restore-safety" || changes[0].Language != LanguageChinese {
		t.Fatalf("恢复结果不正确 / unexpected restore result: %#v", changes)
	}
	if content, err := os.ReadFile(claudePath); err != nil || string(content) != string(chinese) {
		t.Fatalf("中文备份未恢复 / Chinese backup was not restored: err=%v", err)
	}

	statuses, err := manager.Status(LanguageChinese, targets)
	if err != nil {
		t.Fatalf("查询状态失败 / reading status failed: %v", err)
	}
	if !statuses[0].Current || statuses[0].BackupCount != 3 || statuses[1].Current != true {
		t.Fatalf("状态不正确 / unexpected status: %#v", statuses)
	}
}

func TestRestoreRejectsUnsafeOrMissingBackupID(t *testing.T) {
	manager := NewManager(t.TempDir())
	if _, err := manager.Restore([]Target{TargetClaude}, "../outside"); !errors.Is(err, ErrBackupNotFound) {
		t.Fatalf("路径型备份编号应被拒绝 / path-like backup ID must be rejected: %v", err)
	}
	if _, err := manager.Restore([]Target{TargetClaude}, ""); !errors.Is(err, ErrBackupNotFound) {
		t.Fatalf("缺少备份应返回未找到 / missing backup must return not found: %v", err)
	}
}
