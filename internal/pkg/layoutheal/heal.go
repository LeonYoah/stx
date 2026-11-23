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

// Package layoutheal 负责一键升级后的安装目录布局自修复。
//
// 执行一键升级的是旧版本升级器，它只替换 stx 二进制、lib/、scripts/ 与 frontend/，
// 不会替换 bin/。旧 bin/start.sh 每次重启都会拉起旧外部监控三件套，因此修复必须
// 由升级后首次启动的新二进制完成：用内嵌脚本刷新 bin/、移走 bin/lib/observability.sh，
// 并在后台时间窗内按 pid 文件停掉旧进程。
//
// Package layoutheal self-heals the install directory layout after an in-app upgrade.
//
// The upgrade is executed by the OLD upgrader, which only replaces the stx binary, lib/,
// scripts/ and frontend/ — never bin/. The legacy bin/start.sh keeps launching the old
// external monitoring trio on every restart, so the fix must run inside the newly started
// binary: refresh bin/ from the embedded scripts, move bin/lib/observability.sh away and
// stop the legacy processes by pid file within a startup window.
package layoutheal

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/LeonYoah/stx/install"
	"github.com/LeonYoah/stx/internal/apps/selfupgrade"
)

// LayoutHealEnv 关闭自修复的环境变量名（值为 off 时跳过全部动作）。
// LayoutHealEnv disables self-healing when set to "off".
const LayoutHealEnv = "STX_LAYOUT_HEAL"

// binScriptNames 是自修复要刷新的脚本清单。/ binScriptNames lists the scripts refreshed by Heal.
var binScriptNames = []string{"start.sh", "stop.sh", "status.sh"}

// observabilityScript 是旧三件套脚本相对 bin/ 的路径（新 start.sh 不再 source 它）。
// observabilityScript is the legacy trio script path relative to bin/ (the new start.sh never sources it).
const observabilityScript = "lib/observability.sh"

// Options 描述一次自修复执行的可调参数；零值即可用于生产。
// Options tunes one heal run; the zero value is production-ready.
type Options struct {
	// InstallDir 覆盖安装目录探测（测试用）；为空时取当前可执行文件所在目录。
	// InstallDir overrides install-dir detection (tests); empty resolves the running executable's dir.
	InstallDir string
	// ScriptsFS 提供要写入的脚本内容，默认内嵌的 install.BinScripts。
	// ScriptsFS supplies the script content to install; defaults to the embedded install.BinScripts.
	ScriptsFS fs.FS
	// InDocker 报告是否运行在容器里，默认 selfupgrade.IsRunningInDocker。
	// InDocker reports container execution; defaults to selfupgrade.IsRunningInDocker.
	InDocker func() bool
	// Getenv 读取环境变量，默认 os.Getenv。/ Getenv reads env vars; defaults to os.Getenv.
	Getenv func(string) string
	// Executable 返回当前可执行文件路径，默认 os.Executable。
	// Executable returns the running binary path; defaults to os.Executable.
	Executable func() (string, error)
	// Now 返回当前时间，用于备份时间戳，默认 time.Now。/ Now supplies backup timestamps; defaults to time.Now.
	Now func() time.Time
	// GOOS 覆盖目标平台探测，默认 runtime.GOOS（测试在非 Linux 上跑用）。
	// GOOS overrides platform detection; defaults to runtime.GOOS (tests on non-Linux hosts).
	GOOS string
	// Logger 输出自修复日志，默认 log.Printf（level 为 INFO/WARN）。
	// Logger emits heal logs; defaults to log.Printf (level is INFO or WARN).
	Logger func(level, format string, args ...interface{})
}

func (o Options) withDefaults() Options {
	if o.ScriptsFS == nil {
		o.ScriptsFS = install.BinScripts
	}
	if o.InDocker == nil {
		o.InDocker = selfupgrade.IsRunningInDocker
	}
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	if o.Executable == nil {
		o.Executable = os.Executable
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Logger == nil {
		o.Logger = func(level, format string, args ...interface{}) {
			log.Printf("[LayoutHeal] "+level+" "+format+"\n", args...)
		}
	}
	return o
}

// Result 汇总 Heal 的动作；InstallDir 非空表示当前进程跑在安装目录布局里（无论是否有改动）。
// Result summarizes a heal run; a non-empty InstallDir means the process runs inside an
// install layout (whether or not anything changed).
type Result struct {
	InstallDir string
	Replaced   []string
	Backups    []string
	Moved      []string
}

// DetectInstallDir 判断当前进程是否运行在 STX 安装目录布局里，返回规范化目录；不满足返回空串。
// 判定条件全部满足才算安装目录：Linux、非容器、bin/start.sh 含 STX 标识、config.yaml 或 stx.env 存在。
// DetectInstallDir reports the normalized install dir when the process runs inside an STX
// install layout, else "". All gates must pass: Linux, not a container, bin/start.sh carries
// the STX marker, and config.yaml or stx.env exists.
func DetectInstallDir(o Options) string {
	if o.GOOS != "linux" {
		return ""
	}
	if o.InDocker() {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(o.Getenv(LayoutHealEnv)), "off") {
		return ""
	}
	dir := strings.TrimSpace(o.InstallDir)
	if dir == "" {
		exe, err := o.Executable()
		if err != nil || exe == "" {
			return ""
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		dir = filepath.Dir(exe)
	}
	start := filepath.Join(dir, "bin", "start.sh")
	raw, err := os.ReadFile(start)
	if err != nil || !strings.Contains(string(raw), "STX") {
		return ""
	}
	if !fileExists(filepath.Join(dir, "config.yaml")) && !fileExists(filepath.Join(dir, "stx.env")) {
		return ""
	}
	return filepath.Clean(dir)
}

// Heal 在安装目录布局下用内嵌脚本覆盖 bin/ 并移走旧三件套脚本；不满足条件时直接返回空 Result。
// 只在 start.sh 仍 source lib/observability.sh（即仍是 v1.4.0 旧布局）时执行替换，避免把
// 管理员对已是新版脚本的定制在每次重启时静默还原；跳过替换时仍返回 InstallDir 供收尾进程使用。
// 每个脚本先对比内容，不同才「改名备份 → 写临时文件 → chmod 0755 → 原子 rename」；任何一步失败
// 只记 WARN 继续，不阻止服务启动，也不回滚已替换的文件。没有任何变化时不打日志。
// Heal refreshes bin/ from the embedded scripts and moves the legacy trio script away under an
// install layout; it returns an empty Result when the gates do not hold. The replacement only runs
// while start.sh still sources lib/observability.sh (the v1.4.0 legacy layout), so admin edits to
// an already-new script are never silently reverted on restart; InstallDir is still returned for
// the reaper. Each script is compared first and only rewritten via "backup rename -> temp file ->
// chmod 0755 -> atomic rename" when it differs; any step failure logs a WARN and continues without
// blocking startup or rolling back. Nothing is logged when nothing changed.
func Heal(_ context.Context, opts Options) (Result, error) {
	o := opts.withDefaults()
	var res Result
	dir := DetectInstallDir(o)
	if dir == "" {
		return res, nil
	}
	res.InstallDir = dir
	binDir := filepath.Join(dir, "bin")

	// 只在仍是旧布局时替换脚本：v1.4.0 的 start.sh 会 source lib/observability.sh，
	// 新脚本不再引用它。start.sh 已无此引用说明布局已修复（或被管理员手工改过），
	// 继续每次覆盖会把用户的定制静默还原，因此直接跳过替换，只把 InstallDir 留给
	// 收尾进程用。
	// Only rewrite while the layout is still legacy: v1.4.0's start.sh sources
	// lib/observability.sh and the new scripts never reference it. A start.sh without
	// that reference means the layout is already healed (or hand-edited by the admin);
	// overwriting on every startup would silently revert customizations, so skip the
	// replacement and only hand InstallDir to the reaper.
	startRaw, err := os.ReadFile(filepath.Join(binDir, "start.sh"))
	if err != nil || !strings.Contains(string(startRaw), observabilityScript) {
		return res, nil
	}

	for _, name := range binScriptNames {
		content, err := fs.ReadFile(o.ScriptsFS, "bin/"+name)
		if err != nil {
			o.Logger("WARN", "failed to read embedded script %s, skipped: %v", name, err)
			continue
		}
		replaced, backup, err := install.InstallScriptIfChanged(binDir, name, content, o.Now())
		if err != nil {
			o.Logger("WARN", "refresh %s failed: %v", name, err)
			continue
		}
		if replaced {
			res.Replaced = append(res.Replaced, name)
			if backup != "" {
				res.Backups = append(res.Backups, backup)
			}
		}
	}

	// 新 start.sh 不再 source 旧三件套脚本，但旧 start.sh 会，所以要移走；
	// 备份文件名不以 .sh 结尾，防止被误 source。
	// The new start.sh never sources the legacy trio script, but the old one does, so move it
	// away; the backup name does not end in .sh so it cannot be sourced by accident.
	obs := filepath.Join(binDir, observabilityScript)
	if fileExists(obs) {
		backup := obs + ".bak-" + backupTimestamp(o.Now())
		if err := os.Rename(obs, backup); err != nil {
			o.Logger("WARN", "failed to move %s away: %v", observabilityScript, err)
		} else {
			res.Moved = append(res.Moved, observabilityScript)
			res.Backups = append(res.Backups, backup)
		}
	}

	if len(res.Replaced) > 0 || len(res.Moved) > 0 {
		o.Logger("INFO", "install layout healed: replaced %v, moved %v, backups %v", res.Replaced, res.Moved, res.Backups)
	}
	return res, nil
}

func backupTimestamp(now time.Time) string {
	return now.UTC().Format("20060102T150405Z")
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
