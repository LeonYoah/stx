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

// Package selfupdate 实现 Agent 二进制的原地自升级：校验、备份、替换、重启与失败回滚。
// Package selfupdate implements in-place Agent binary self-upgrade: verify, back up,
// replace, restart and roll back on failure.
package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	// EnvBackup 携带上一版本二进制的备份路径；新进程据此在试运行失败时回滚。
	// EnvBackup carries the backup path of the previous binary so the new process can roll back.
	EnvBackup = "STX_AGENT_SELF_UPDATE_BACKUP"
	// EnvFromVersion 记录升级前的版本，仅用于日志。
	// EnvFromVersion records the pre-upgrade version (logging only).
	EnvFromVersion = "STX_AGENT_SELF_UPDATE_FROM"
	// EnvTrialTimeout 覆盖试运行确认超时（Go duration 格式）。
	// EnvTrialTimeout overrides the trial confirmation timeout (Go duration format).
	EnvTrialTimeout = "STX_AGENT_SELF_UPDATE_TRIAL_TIMEOUT"

	// DefaultTrialTimeout 是新二进制必须完成注册的时限，超时即回滚。
	// DefaultTrialTimeout is how long the new binary has to register before it rolls back.
	DefaultTrialTimeout = 90 * time.Second

	backupSuffix  = ".bak"
	stagingSuffix = ".new"
)

var (
	// ErrNoBackup 表示没有可回滚的备份二进制。
	// ErrNoBackup means there is no backup binary to roll back to.
	ErrNoBackup = errors.New("agent backup binary not found / 未找到 Agent 备份二进制")

	versionLine = regexp.MustCompile(`(?m)Version:\s*v?(\S+)`)

	// 测试钩子 / test hooks
	execFn         = syscall.Exec
	executablePath = defaultExecutablePath
	versionProbe   = defaultVersionProbe
)

func defaultExecutablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return p, nil
}

// defaultVersionProbe 运行候选二进制的 version 子命令，同时验证其可执行（架构/格式正确）。
// defaultVersionProbe runs the candidate's version subcommand, which also proves it can execute on this host.
func defaultVersionProbe(ctx context.Context, binary string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(probeCtx, binary, "version")
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("new agent binary failed to run: %v (%s) / 新 Agent 二进制无法运行: %v", err, strings.TrimSpace(out.String()), err)
	}
	m := versionLine.FindStringSubmatch(out.String())
	if len(m) < 2 {
		return "", fmt.Errorf("cannot parse version output of new agent binary / 无法解析新 Agent 二进制的版本输出")
	}
	return m[1], nil
}

// NormalizeVersion 去掉前缀 v 与空白。/ NormalizeVersion trims spaces and a leading "v".
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// ApplyRequest 描述一次自升级。/ ApplyRequest describes one self-upgrade.
type ApplyRequest struct {
	PackagePath   string
	Checksum      string
	TargetVersion string
}

// ApplyResult 是替换完成后的结果。/ ApplyResult is the outcome after the binary was swapped.
type ApplyResult struct {
	Executable  string `json:"executable"`
	Backup      string `json:"backup"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

// Apply 校验新二进制（SHA256 + 可运行 + 版本一致），备份当前二进制，再原子替换。
// 替换前任一检查失败都不会改动当前二进制。
// Apply verifies the new binary (SHA256, runnable, matching version), backs up the current
// binary and atomically swaps it in. Any check failing before the swap leaves the current binary untouched.
func Apply(ctx context.Context, req ApplyRequest, currentVersion string) (*ApplyResult, error) {
	target := NormalizeVersion(req.TargetVersion)
	if req.PackagePath == "" || target == "" {
		return nil, fmt.Errorf("package_path and target_version are required / package_path 与 target_version 必填")
	}
	if strings.TrimSpace(req.Checksum) == "" {
		return nil, fmt.Errorf("checksum is required / checksum 必填")
	}
	sum, err := fileSHA256(req.PackagePath)
	if err != nil {
		return nil, fmt.Errorf("read new agent binary: %w / 读取新 Agent 二进制失败: %w", err, err)
	}
	if !strings.EqualFold(sum, strings.TrimSpace(req.Checksum)) {
		return nil, fmt.Errorf("checksum mismatch: got %s want %s / 校验和不一致", sum, req.Checksum)
	}
	exe, err := executablePath()
	if err != nil {
		return nil, fmt.Errorf("resolve agent executable: %w / 无法定位 Agent 可执行文件: %w", err, err)
	}

	staging := exe + stagingSuffix
	if err := copyFile(req.PackagePath, staging, 0o755); err != nil {
		return nil, fmt.Errorf("stage new agent binary next to %s: %w / 预置新 Agent 二进制失败: %w", exe, err, err)
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.Remove(staging)
		}
	}()

	got, err := versionProbe(ctx, staging)
	if err != nil {
		return nil, err
	}
	if NormalizeVersion(got) != target {
		return nil, fmt.Errorf("new agent binary reports version %s, expected %s / 新 Agent 二进制版本为 %s，期望 %s", got, target, got, target)
	}

	backup := exe + backupSuffix
	if err := copyFile(exe, backup+".tmp", 0o755); err != nil {
		return nil, fmt.Errorf("back up current agent binary: %w / 备份当前 Agent 二进制失败: %w", err, err)
	}
	if err := os.Rename(backup+".tmp", backup); err != nil {
		_ = os.Remove(backup + ".tmp")
		return nil, fmt.Errorf("back up current agent binary: %w / 备份当前 Agent 二进制失败: %w", err, err)
	}
	if err := os.Rename(staging, exe); err != nil {
		return nil, fmt.Errorf("replace agent binary: %w / 替换 Agent 二进制失败: %w", err, err)
	}
	cleanupStaging = false
	return &ApplyResult{Executable: exe, Backup: backup, FromVersion: NormalizeVersion(currentVersion), ToVersion: target}, nil
}

// Restart 以新二进制原地 exec（保持 PID，systemd/nohup 均适用），并把备份路径交给新进程用于试运行回滚。
// Restart execs the new binary in place (same PID, so systemd/nohup supervision keeps working)
// and hands the backup path to the new process for trial rollback.
func Restart(res *ApplyResult) error {
	env := append(withoutTrialEnv(os.Environ()), EnvBackup+"="+res.Backup, EnvFromVersion+"="+res.FromVersion)
	return execFn(res.Executable, os.Args, env)
}

// RestoreFile 用备份覆盖当前二进制（不重启）。/ RestoreFile copies the backup over the current binary without restarting.
func RestoreFile(backup string) (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", err
	}
	if backup == "" {
		backup = exe + backupSuffix
	}
	fi, err := os.Stat(backup)
	if err != nil || fi.Size() == 0 {
		return "", ErrNoBackup
	}
	staging := exe + stagingSuffix
	if err := copyFile(backup, staging, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(staging, exe); err != nil {
		_ = os.Remove(staging)
		return "", err
	}
	return exe, nil
}

// HasBackup 报告是否存在可回滚的备份。/ HasBackup reports whether a rollback backup exists.
func HasBackup() bool {
	exe, err := executablePath()
	if err != nil {
		return false
	}
	fi, err := os.Stat(exe + backupSuffix)
	return err == nil && fi.Size() > 0
}

// RollbackAndRestart 恢复备份并以旧二进制 exec（不再携带试运行变量）。
// RollbackAndRestart restores the backup and execs the previous binary (without trial variables).
func RollbackAndRestart(backup string) error {
	exe, err := RestoreFile(backup)
	if err != nil {
		return err
	}
	return execFn(exe, os.Args, withoutTrialEnv(os.Environ()))
}

// Trial 表示刚完成自升级、尚未确认可用的新进程。
// Trial describes a freshly self-upgraded process that has not yet proven it works.
type Trial struct {
	Backup      string
	FromVersion string
	Timeout     time.Duration
}

// TrialFromEnv 从环境变量读取试运行信息；非自升级启动时返回 nil。
// TrialFromEnv reads trial info from the environment; nil when not started by a self-upgrade.
func TrialFromEnv() *Trial {
	backup := strings.TrimSpace(os.Getenv(EnvBackup))
	if backup == "" {
		return nil
	}
	timeout := DefaultTrialTimeout
	if raw := strings.TrimSpace(os.Getenv(EnvTrialTimeout)); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			timeout = d
		}
	}
	return &Trial{Backup: backup, FromVersion: os.Getenv(EnvFromVersion), Timeout: timeout}
}

// Confirm 在新进程注册成功后调用，清除试运行变量，避免后续 exec 误回滚。
// Confirm is called once the new process registered; it clears the trial variables so later execs never roll back.
func (t *Trial) Confirm() {
	_ = os.Unsetenv(EnvBackup)
	_ = os.Unsetenv(EnvFromVersion)
}

// Rollback 恢复备份并重启为旧版本。/ Rollback restores the backup and restarts as the previous version.
func (t *Trial) Rollback() error {
	return RollbackAndRestart(t.Backup)
}

// Watch 等待 confirmed 关闭；超时或 failed 关闭时回滚。返回 true 表示已确认。
// Watch waits for confirmed to close; on timeout or when failed closes it rolls back. Returns true when confirmed.
func (t *Trial) Watch(confirmed, failed <-chan struct{}, onRollback func(reason string, err error)) bool {
	timer := time.NewTimer(t.Timeout)
	defer timer.Stop()
	var reason string
	select {
	case <-confirmed:
		t.Confirm()
		return true
	case <-failed:
		reason = "agent failed to start / Agent 启动失败"
	case <-timer.C:
		reason = fmt.Sprintf("agent did not register within %s / Agent 未在 %s 内完成注册", t.Timeout, t.Timeout)
	}
	err := t.Rollback()
	if onRollback != nil {
		onRollback(reason, err)
	}
	return false
}

func withoutTrialEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, EnvBackup+"=") || strings.HasPrefix(kv, EnvFromVersion+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// ReplaceFileWithBackup 把 src 原子写入 dst；若 dst 已存在先备份为 dst.bak。返回是否产生了备份。
// ReplaceFileWithBackup atomically writes src to dst, backing up an existing dst to dst.bak first.
func ReplaceFileWithBackup(src, dst string, mode os.FileMode) (bool, error) {
	backedUp := false
	if fi, err := os.Stat(dst); err == nil && fi.Mode().IsRegular() {
		if err := copyFile(dst, dst+backupSuffix, fi.Mode().Perm()); err != nil {
			return false, fmt.Errorf("back up %s: %w", dst, err)
		}
		backedUp = true
	}
	tmp := dst + stagingSuffix
	if err := copyFile(src, tmp, mode); err != nil {
		return backedUp, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return backedUp, err
	}
	return backedUp, nil
}

// OverwriteFile 原子地用 src 覆盖 dst，不生成新的备份（用于从 .bak 恢复）。
// OverwriteFile atomically overwrites dst with src without creating a new backup (used to restore from .bak).
func OverwriteFile(src, dst string, mode os.FileMode) error {
	tmp := dst + stagingSuffix
	if err := copyFile(src, tmp, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
