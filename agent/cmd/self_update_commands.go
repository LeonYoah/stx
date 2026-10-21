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


package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/executor"
	"github.com/LeonYoah/stx/agent/internal/installer"
	"github.com/LeonYoah/stx/agent/internal/logger"
	"github.com/LeonYoah/stx/agent/internal/selfupdate"
	stxversion "github.com/LeonYoah/stx/internal/version"
)

const (
	// subCommandAgentSelfUpdate 用已传输到本机的新二进制替换自身并重启。
	// subCommandAgentSelfUpdate replaces the agent with an already transferred binary and restarts.
	subCommandAgentSelfUpdate = "agent_self_update"
	// subCommandAgentSelfRollback 恢复上一次自升级留下的备份并重启。
	// subCommandAgentSelfRollback restores the backup left by the last self-update and restarts.
	subCommandAgentSelfRollback = "agent_self_rollback"
	// subCommandAgentUpdateProxyJars 更新本机 stx-java-proxy jar（带备份，不重启运行中的 proxy）。
	// subCommandAgentUpdateProxyJars updates local stx-java-proxy jars (with backup, running proxies untouched).
	subCommandAgentUpdateProxyJars = "agent_update_proxy_jars"
)

var (
	// selfUpdateRestartDelay 给命令响应留出回传时间后再 exec。
	// selfUpdateRestartDelay leaves time for the command response to reach the Control Plane before exec.
	selfUpdateRestartDelay = 2 * time.Second

	// 测试钩子 / test hooks
	selfUpdateApply    = selfupdate.Apply
	selfUpdateRestart  = selfupdate.Restart
	selfUpdateRollback = selfupdate.RollbackAndRestart
	selfUpdateRestore  = selfupdate.RestoreFile
	selfUpdateHasBak   = selfupdate.HasBackup
	proxyLibDirs       = installer.STXJavaProxyLibDirs
	scheduleAfter      = func(d time.Duration, fn func()) { time.AfterFunc(d, fn) }
)

// markRegistered 在注册成功后调用一次，确认自升级试运行。
// markRegistered is called after a successful registration and confirms a self-upgrade trial.
func (a *Agent) markRegistered() {
	if a.registeredCh == nil {
		return
	}
	a.registeredOnce.Do(func() { close(a.registeredCh) })
}

func (a *Agent) handleAgentSelfUpgradeCommand(ctx context.Context, subCommand string, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	switch subCommand {
	case subCommandAgentSelfUpdate:
		return a.handleAgentSelfUpdate(ctx, cmd, reporter)
	case subCommandAgentSelfRollback:
		return a.handleAgentSelfRollback(ctx, cmd)
	default:
		return a.handleAgentUpdateProxyJars(ctx, cmd)
	}
}

func (a *Agent) handleAgentSelfUpdate(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	if !a.selfUpdateMu.TryLock() {
		err := fmt.Errorf("another agent self-update is in progress / 另一个 Agent 自升级正在进行")
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}
	target := selfupdate.NormalizeVersion(cmd.Parameters["target_version"])
	if target != "" && target == selfupdate.NormalizeVersion(stxversion.Version) {
		a.selfUpdateMu.Unlock()
		out, _ := json.Marshal(map[string]any{"already_current": true, "from_version": stxversion.Version, "to_version": target})
		return executor.CreateSuccessResponse(cmd.CommandId, string(out)), nil
	}
	if reporter != nil {
		reporter.Report(30, "Verifying new agent binary / 校验新 Agent 二进制")
	}
	res, err := selfUpdateApply(ctx, selfupdate.ApplyRequest{
		PackagePath:   cmd.Parameters["package_path"],
		Checksum:      cmd.Parameters["checksum"],
		TargetVersion: target,
	}, stxversion.Version)
	if err != nil {
		a.selfUpdateMu.Unlock()
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}
	logger.InfoF(ctx, "[SelfUpdate] Binary replaced %s -> %s (backup %s), restarting in %s / 二进制已替换，即将重启",
		res.FromVersion, res.ToVersion, res.Backup, selfUpdateRestartDelay)
	scheduleAfter(selfUpdateRestartDelay, func() {
		defer a.selfUpdateMu.Unlock()
		if err := selfUpdateRestart(res); err != nil {
			// exec 失败：恢复旧二进制文件，继续以旧版本运行，CP 会看到版本未变化。
			// exec failed: restore the old binary file and keep running as the old version; the CP sees no version change.
			logger.ErrorF(context.Background(), "[SelfUpdate] exec new binary failed: %v; restoring backup / exec 新二进制失败，恢复备份", err)
			if _, rerr := selfUpdateRestore(res.Backup); rerr != nil {
				logger.ErrorF(context.Background(), "[SelfUpdate] restore backup failed: %v / 恢复备份失败", rerr)
			}
		}
	})
	out, _ := json.Marshal(res)
	return executor.CreateSuccessResponse(cmd.CommandId, string(out)), nil
}

func (a *Agent) handleAgentSelfRollback(ctx context.Context, cmd *pb.CommandRequest) (*pb.CommandResponse, error) {
	if !selfUpdateHasBak() {
		err := selfupdate.ErrNoBackup
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}
	if !a.selfUpdateMu.TryLock() {
		err := fmt.Errorf("another agent self-update is in progress / 另一个 Agent 自升级正在进行")
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}
	logger.InfoF(ctx, "[SelfUpdate] Rolling back to backup binary in %s / 即将回滚到备份二进制", selfUpdateRestartDelay)
	scheduleAfter(selfUpdateRestartDelay, func() {
		defer a.selfUpdateMu.Unlock()
		if err := selfUpdateRollback(""); err != nil {
			logger.ErrorF(context.Background(), "[SelfUpdate] rollback failed: %v / 回滚失败", err)
		}
	})
	out, _ := json.Marshal(map[string]any{"from_version": stxversion.Version, "rolling_back": true})
	return executor.CreateSuccessResponse(cmd.CommandId, string(out)), nil
}

// proxyJarUpdateResult 是 stx-java-proxy jar 更新结果。/ proxyJarUpdateResult reports a proxy jar update.
type proxyJarUpdateResult struct {
	TargetDir string   `json:"target_dir,omitempty"`
	Updated   []string `json:"updated,omitempty"`
	BackedUp  []string `json:"backed_up,omitempty"`
	Skipped   bool     `json:"skipped,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

// resolveProxyJarDir 返回本机已存在 stx-java-proxy jar 的目录；没有安装 proxy 时返回空。
// resolveProxyJarDir returns the directory that already holds stx-java-proxy jars; empty when no proxy is installed.
func resolveProxyJarDir() string {
	for _, dir := range proxyLibDirs() {
		matches, _ := filepath.Glob(filepath.Join(dir, "stx-java-proxy-*.jar"))
		if len(matches) > 0 {
			if abs, err := filepath.Abs(dir); err == nil {
				return abs
			}
			return dir
		}
	}
	return ""
}

func (a *Agent) handleAgentUpdateProxyJars(ctx context.Context, cmd *pb.CommandRequest) (*pb.CommandResponse, error) {
	jars := splitCSV(cmd.Parameters["jar_paths"])
	if len(jars) == 0 {
		err := fmt.Errorf("jar_paths is required / jar_paths 必填")
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}
	result := proxyJarUpdateResult{TargetDir: resolveProxyJarDir()}
	if result.TargetDir == "" {
		result.Skipped = true
		result.Reason = "no stx-java-proxy installation found on this node / 本节点未安装 stx-java-proxy"
		out, _ := json.Marshal(result)
		return executor.CreateSuccessResponse(cmd.CommandId, string(out)), nil
	}
	for _, src := range jars {
		base := filepath.Base(src)
		if !strings.HasPrefix(base, "stx-java-proxy-") || !strings.HasSuffix(base, ".jar") {
			err := fmt.Errorf("unexpected proxy jar name %q / 非法 proxy jar 文件名", base)
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		if _, err := os.Stat(src); err != nil {
			err = fmt.Errorf("proxy jar not found: %s / 未找到 proxy jar", src)
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		dst := filepath.Join(result.TargetDir, base)
		backedUp, err := selfupdate.ReplaceFileWithBackup(src, dst, 0o644)
		if err != nil {
			err = fmt.Errorf("update %s: %v / 更新 proxy jar 失败", dst, err)
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		result.Updated = append(result.Updated, dst)
		if backedUp {
			result.BackedUp = append(result.BackedUp, dst+".bak")
		}
	}
	logger.InfoF(ctx, "[SelfUpdate] Updated %d stx-java-proxy jar(s) in %s / 已更新 proxy jar", len(result.Updated), result.TargetDir)
	out, _ := json.Marshal(result)
	return executor.CreateSuccessResponse(cmd.CommandId, string(out)), nil
}

func stxversionVersion() string { return stxversion.Version }
