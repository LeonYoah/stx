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
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed assets/*/SKILL.md
var embeddedSources embed.FS

// Language 表示内置 Skill 的语言。
// Language identifies one embedded Skill language.
type Language string

const (
	LanguageAuto    Language = "auto"
	LanguageChinese Language = "zh-CN"
	LanguageEnglish Language = "en"
)

// Target 表示 STX 可以管理的 Skill 安装目标。
// Target identifies one Skill installation target managed by STX.
type Target string

const (
	TargetClaude Target = "claude"
	TargetAgents Target = "agents"
)

var (
	ErrInvalidLanguage = errors.New("invalid skill language")
	ErrInvalidTarget   = errors.New("invalid skill target")
	ErrBackupNotFound  = errors.New("skill backup not found")
)

// TargetStatus 描述一个安装目标当前的文件状态。
// TargetStatus describes the current file state of one installation target.
type TargetStatus struct {
	Target         Target   `json:"target" yaml:"target"`
	Path           string   `json:"path" yaml:"path"`
	Installed      bool     `json:"installed" yaml:"installed"`
	Current        bool     `json:"current" yaml:"current"`
	Language       Language `json:"language,omitempty" yaml:"language,omitempty"`
	Checksum       string   `json:"checksum,omitempty" yaml:"checksum,omitempty"`
	SourceChecksum string   `json:"source_checksum" yaml:"source_checksum"`
	BackupCount    int      `json:"backup_count" yaml:"backup_count"`
	LatestBackup   string   `json:"latest_backup,omitempty" yaml:"latest_backup,omitempty"`
}

// Change 描述安装、更新、备份或恢复对一个目标产生的结果。
// Change describes the result of install, update, backup, or restore for one target.
type Change struct {
	Target         Target   `json:"target" yaml:"target"`
	Path           string   `json:"path" yaml:"path"`
	Status         string   `json:"status" yaml:"status"`
	Language       Language `json:"language,omitempty" yaml:"language,omitempty"`
	Checksum       string   `json:"checksum,omitempty" yaml:"checksum,omitempty"`
	BackupID       string   `json:"backup_id,omitempty" yaml:"backup_id,omitempty"`
	SafetyBackupID string   `json:"safety_backup_id,omitempty" yaml:"safety_backup_id,omitempty"`
}

// Manager 管理 Claude 与 Agents 目录中的 STX Skill 文件。
// Manager manages STX Skill files in the Claude and Agents directories.
type Manager struct {
	home        string
	backupRoot  string
	newBackupID func() string
}

// NewManager 创建只操作指定用户目录的 Skill 管理器。
// NewManager creates a Skill manager scoped to the supplied home directory.
func NewManager(home string) *Manager {
	return &Manager{
		home:       home,
		backupRoot: filepath.Join(home, ".stx", "skill-backups"),
		newBackupID: func() string {
			return time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + uuid.NewString()[:8]
		},
	}
}

// ResolveLanguage 解析显式语言，auto 会读取常见系统语言环境变量。
// ResolveLanguage resolves an explicit language; auto reads common system locale variables.
func ResolveLanguage(value string, getenv func(string) string) (Language, error) {
	normalized := normalizeLocale(value)
	switch {
	case normalized == "", normalized == string(LanguageAuto):
		return DetectLanguage(getenv), nil
	case normalized == "zh", strings.HasPrefix(normalized, "zh-"):
		return LanguageChinese, nil
	case normalized == "en", strings.HasPrefix(normalized, "en-"):
		return LanguageEnglish, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidLanguage, value)
	}
}

// DetectLanguage 只在系统明确使用英语时选择英文，其他情况默认中文。
// DetectLanguage selects English only for an explicit English locale and defaults to Chinese otherwise.
func DetectLanguage(getenv func(string) string) Language {
	if getenv == nil {
		getenv = os.Getenv
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		value := strings.TrimSpace(getenv(key))
		if value == "" {
			continue
		}
		normalized := normalizeLocale(strings.Split(value, ":")[0])
		if normalized == "en" || strings.HasPrefix(normalized, "en-") {
			return LanguageEnglish
		}
		return LanguageChinese
	}
	return LanguageChinese
}

// ParseTargets 解析 all、claude 或 agents 安装目标。
// ParseTargets parses the all, claude, or agents installation selector.
func ParseTargets(value string) ([]Target, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return []Target{TargetClaude, TargetAgents}, nil
	case string(TargetClaude):
		return []Target{TargetClaude}, nil
	case string(TargetAgents):
		return []Target{TargetAgents}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidTarget, value)
	}
}

// Source 返回指定语言的内置 SKILL.md。
// Source returns the embedded SKILL.md for the selected language.
func Source(language Language) ([]byte, error) {
	path := ""
	switch language {
	case LanguageChinese:
		path = "assets/zh-CN/SKILL.md"
	case LanguageEnglish:
		path = "assets/en/SKILL.md"
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidLanguage, language)
	}
	content, err := embeddedSources.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// Status 查询目标文件是否安装、是否与所选语言版本一致以及可用备份。
// Status reports installation, selected-language freshness, and available backups.
func (m *Manager) Status(language Language, targets []Target) ([]TargetStatus, error) {
	source, err := Source(language)
	if err != nil {
		return nil, err
	}
	sourceChecksum := checksum(source)
	result := make([]TargetStatus, 0, len(targets))
	for _, target := range targets {
		path, err := m.skillPath(target)
		if err != nil {
			return nil, err
		}
		item := TargetStatus{Target: target, Path: path, SourceChecksum: sourceChecksum}
		content, err := os.ReadFile(path)
		switch {
		case err == nil:
			item.Installed = true
			item.Current = bytes.Equal(content, source)
			item.Language = languageForContent(content)
			item.Checksum = checksum(content)
		case errors.Is(err, os.ErrNotExist):
		default:
			return nil, err
		}
		backups, err := m.listBackups(target)
		if err != nil {
			return nil, err
		}
		item.BackupCount = len(backups)
		if len(backups) > 0 {
			item.LatestBackup = backups[0]
		}
		result = append(result, item)
	}
	return result, nil
}

// Install 只写入尚未存在的 SKILL.md，不覆盖已有文件。
// Install writes only missing SKILL.md files and never overwrites existing files.
func (m *Manager) Install(language Language, targets []Target) ([]Change, error) {
	source, err := Source(language)
	if err != nil {
		return nil, err
	}
	result := make([]Change, 0, len(targets))
	for _, target := range targets {
		path, err := m.skillPath(target)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err == nil {
			result = append(result, Change{Target: target, Path: path, Status: "skipped_existing", Language: languageForFile(path)})
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := writeFileAtomic(path, source, 0o644); err != nil {
			return nil, err
		}
		result = append(result, Change{Target: target, Path: path, Status: "installed", Language: language, Checksum: checksum(source)})
	}
	return result, nil
}

// Update 写入缺失或内容不同的 SKILL.md，并在覆盖前保存备份。
// Update writes missing or changed SKILL.md files and backs up content before replacement.
func (m *Manager) Update(language Language, targets []Target) ([]Change, error) {
	source, err := Source(language)
	if err != nil {
		return nil, err
	}
	backupID := m.newBackupID()
	result := make([]Change, 0, len(targets))
	for _, target := range targets {
		path, err := m.skillPath(target)
		if err != nil {
			return nil, err
		}
		current, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Equal(current, source) {
			result = append(result, Change{Target: target, Path: path, Status: "unchanged", Language: language, Checksum: checksum(source)})
			continue
		}
		change := Change{Target: target, Path: path, Language: language, Checksum: checksum(source)}
		switch {
		case readErr == nil:
			if err := m.saveBackup(target, backupID, current); err != nil {
				return nil, err
			}
			change.Status = "updated"
			change.BackupID = backupID
		case errors.Is(readErr, os.ErrNotExist):
			change.Status = "installed"
		default:
			return nil, readErr
		}
		if err := writeFileAtomic(path, source, 0o644); err != nil {
			return nil, err
		}
		result = append(result, change)
	}
	return result, nil
}

// Backup 备份当前存在的 SKILL.md，缺失目标返回 skipped_missing。
// Backup saves existing SKILL.md files and reports skipped_missing for absent targets.
func (m *Manager) Backup(targets []Target) ([]Change, error) {
	backupID := m.newBackupID()
	result := make([]Change, 0, len(targets))
	for _, target := range targets {
		path, err := m.skillPath(target)
		if err != nil {
			return nil, err
		}
		content, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			result = append(result, Change{Target: target, Path: path, Status: "skipped_missing"})
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := m.saveBackup(target, backupID, content); err != nil {
			return nil, err
		}
		result = append(result, Change{
			Target: target, Path: path, Status: "backed_up", Language: languageForContent(content), Checksum: checksum(content), BackupID: backupID,
		})
	}
	return result, nil
}

// Restore 从指定备份恢复；backupID 为空时使用每个目标的最新备份。
// Restore restores a selected backup; an empty backupID selects the latest backup for each target.
func (m *Manager) Restore(targets []Target, backupID string) ([]Change, error) {
	if backupID != "" && !validBackupID(backupID) {
		return nil, fmt.Errorf("%w: %s", ErrBackupNotFound, backupID)
	}
	type restoreItem struct {
		target   Target
		path     string
		backupID string
		content  []byte
	}
	items := make([]restoreItem, 0, len(targets))
	for _, target := range targets {
		selectedID := backupID
		if selectedID == "" {
			backups, err := m.listBackups(target)
			if err != nil {
				return nil, err
			}
			if len(backups) == 0 {
				return nil, fmt.Errorf("%w for target %s", ErrBackupNotFound, target)
			}
			selectedID = backups[0]
		}
		backupPath, err := m.backupSkillPath(target, selectedID)
		if err != nil {
			return nil, err
		}
		content, err := os.ReadFile(backupPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w for target %s: %s", ErrBackupNotFound, target, selectedID)
		}
		if err != nil {
			return nil, err
		}
		path, err := m.skillPath(target)
		if err != nil {
			return nil, err
		}
		items = append(items, restoreItem{target: target, path: path, backupID: selectedID, content: content})
	}

	safetyBackupID := m.newBackupID()
	result := make([]Change, 0, len(items))
	for _, item := range items {
		change := Change{
			Target: item.target, Path: item.path, Status: "restored", Language: languageForContent(item.content), Checksum: checksum(item.content), BackupID: item.backupID,
		}
		current, err := os.ReadFile(item.path)
		if err == nil {
			if err := m.saveBackup(item.target, safetyBackupID, current); err != nil {
				return nil, err
			}
			change.SafetyBackupID = safetyBackupID
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := writeFileAtomic(item.path, item.content, 0o644); err != nil {
			return nil, err
		}
		result = append(result, change)
	}
	return result, nil
}

func (m *Manager) skillPath(target Target) (string, error) {
	switch target {
	case TargetClaude:
		return filepath.Join(m.home, ".claude", "skills", "stx", "SKILL.md"), nil
	case TargetAgents:
		return filepath.Join(m.home, ".agents", "skills", "stx", "SKILL.md"), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidTarget, target)
	}
}

func (m *Manager) backupSkillPath(target Target, backupID string) (string, error) {
	if !validBackupID(backupID) {
		return "", fmt.Errorf("%w: %s", ErrBackupNotFound, backupID)
	}
	if _, err := m.skillPath(target); err != nil {
		return "", err
	}
	return filepath.Join(m.backupRoot, string(target), backupID, "SKILL.md"), nil
}

func (m *Manager) saveBackup(target Target, backupID string, content []byte) error {
	path, err := m.backupSkillPath(target, backupID)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, content, 0o600)
}

func (m *Manager) listBackups(target Target) ([]string, error) {
	if _, err := m.skillPath(target); err != nil {
		return nil, err
	}
	directory := filepath.Join(m.backupRoot, string(target))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && validBackupID(entry.Name()) {
			if _, err := os.Stat(filepath.Join(directory, entry.Name(), "SKILL.md")); err == nil {
				ids = append(ids, entry.Name())
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids, nil
}

func languageForFile(path string) Language {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return languageForContent(content)
}

func languageForContent(content []byte) Language {
	for _, language := range []Language{LanguageChinese, LanguageEnglish} {
		source, err := Source(language)
		if err == nil && bytes.Equal(content, source) {
			return language
		}
	}
	return "custom"
}

func writeFileAtomic(path string, content []byte, mode fs.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".SKILL.md.tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func normalizeLocale(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if index := strings.IndexAny(value, ".@"); index >= 0 {
		value = value[:index]
	}
	return strings.ReplaceAll(value, "_", "-")
}

func validBackupID(value string) bool {
	if value == "" || filepath.Base(value) != value || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}
