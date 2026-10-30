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

package installer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeonYoah/stx/agent/internal/javaenv"
)

// BackupInstallDir 备份安装目录，并跳过 logs 目录。
// BackupInstallDir backs up the install directory while skipping the logs directory.
func (m *InstallerManager) BackupInstallDir(ctx context.Context, installDir, backupDir string) (string, error) {
	if installDir == "" {
		return "", fmt.Errorf("install_dir is required")
	}
	if backupDir == "" {
		backupDir = fmt.Sprintf("%s.backup-%s", installDir, time.Now().Format("20060102150405"))
	}
	if err := copyDirFiltered(installDir, backupDir, map[string]struct{}{"logs": {}}); err != nil {
		return "", err
	}
	return backupDir, nil
}

// ExtractPackageToDir 校验并解压安装包到目标目录。
// ExtractPackageToDir verifies and extracts a package to the target directory.
func (m *InstallerManager) ExtractPackageToDir(ctx context.Context, packagePath, targetDir, expectedChecksum string, reporter ProgressReporter) error {
	if packagePath == "" {
		return fmt.Errorf("package_path is required")
	}
	if targetDir == "" {
		return fmt.Errorf("target_dir is required")
	}
	if expectedChecksum == "" {
		return fmt.Errorf("expected_checksum is required")
	}
	if err := m.VerifyChecksum(packagePath, expectedChecksum); err != nil {
		return err
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("failed to clean target dir: %w", err)
	}
	return m.extractPackage(ctx, packagePath, targetDir, reporter)
}

// SyncConnectorsManifest 根据 manifest 准备 connectors 目录，但保留安装包自带内容。
// SyncConnectorsManifest prepares the connectors directory while preserving package-bundled assets.
func (m *InstallerManager) SyncConnectorsManifest(installDir string, keepFiles []string) error {
	return syncManifestDir(filepath.Join(installDir, "connectors"), keepFiles)
}

// SyncLibManifest 根据 manifest 准备 lib 目录，但保留安装包自带内容。
// SyncLibManifest prepares the lib directory while preserving package-bundled assets.
func (m *InstallerManager) SyncLibManifest(installDir string, keepFiles []string) error {
	return syncManifestDir(filepath.Join(installDir, "lib"), keepFiles)
}

// SyncPluginsManifest 根据 manifest 准备 plugins 目录，但保留安装包自带内容。
// SyncPluginsManifest prepares the plugins directory while preserving package-bundled assets.
func (m *InstallerManager) SyncPluginsManifest(installDir string, keepFiles []string) error {
	return syncManifestDir(filepath.Join(installDir, "plugins"), keepFiles)
}

// RestoreInstallDir 从备份恢复安装目录。
// RestoreInstallDir restores the install directory from a backup snapshot.
func (m *InstallerManager) RestoreInstallDir(backupDir, restoreDir string) error {
	if backupDir == "" || restoreDir == "" {
		return fmt.Errorf("backup_dir and restore_dir are required")
	}
	if err := os.RemoveAll(restoreDir); err != nil {
		return fmt.Errorf("failed to clean restore dir: %w", err)
	}
	return copyDirFiltered(backupDir, restoreDir, nil)
}

// SwitchInstallDir 切换 current 软链到目标目录；若未提供软链，仅验证目录存在。
// SwitchInstallDir switches the current symlink to the target directory; if no symlink is provided, it only validates the target directory.
func (m *InstallerManager) SwitchInstallDir(targetDir, currentLink string) (string, error) {
	if targetDir == "" {
		return "", fmt.Errorf("target_dir is required")
	}
	if _, err := os.Stat(targetDir); err != nil {
		return "", fmt.Errorf("target_dir is not ready: %w", err)
	}
	if currentLink == "" {
		return targetDir, nil
	}
	if err := os.RemoveAll(currentLink); err != nil {
		return "", fmt.Errorf("failed to remove current link: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(currentLink), 0755); err != nil {
		return "", fmt.Errorf("failed to prepare current link dir: %w", err)
	}
	if err := os.Symlink(targetDir, currentLink); err != nil {
		return "", fmt.Errorf("failed to create current link: %w", err)
	}
	return currentLink, nil
}

// VerifyInstallLayout 校验升级后的安装目录关键文件是否存在。
// VerifyInstallLayout verifies that the critical files exist in the upgraded install directory.
func (m *InstallerManager) VerifyInstallLayout(installDir string) error {
	if installDir == "" {
		return fmt.Errorf("install_dir is required")
	}
	requiredPaths := []string{
		filepath.Join(installDir, "bin", "seatunnel-cluster.sh"),
		filepath.Join(installDir, "config"),
	}
	for _, requiredPath := range requiredPaths {
		if _, err := os.Stat(requiredPath); err != nil {
			return fmt.Errorf("required path %s is missing: %w", requiredPath, err)
		}
	}
	return nil
}

// RunSmokeTestTemplate 在升级后的安装目录中执行模板任务，验证集群可用性。
// RunSmokeTestTemplate executes the bundled template job under the upgraded install directory.
func (m *InstallerManager) RunSmokeTestTemplate(ctx context.Context, installDir string) (string, error) {
	if installDir == "" {
		return "", fmt.Errorf("install_dir is required")
	}
	if err := m.VerifyInstallLayout(installDir); err != nil {
		return "", err
	}

	scriptPath := filepath.Join(installDir, "bin", "seatunnel.sh")
	configPath := filepath.Join(installDir, "config", "v2.batch.config.template")
	if _, err := os.Stat(scriptPath); err != nil {
		return "", fmt.Errorf("smoke test script is missing: %w", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		return "", fmt.Errorf("smoke test config is missing: %w", err)
	}

	smokeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// 节点配置了 JDK 时在 profile 之后导出；未配置时命令不变。
	// A configured node JDK is exported after the profiles; without one the command is unchanged.
	cmd := exec.CommandContext(smokeCtx, "bash", "-lc", javaenv.ShellExportPrefix(javaenv.HomeFromContext(ctx))+"./bin/seatunnel.sh -c config/v2.batch.config.template")
	cmd.Dir = installDir
	output, err := cmd.CombinedOutput()
	summary := summarizeCommandOutput(string(output))
	if err != nil {
		if summary != "" {
			return "", fmt.Errorf("模板任务执行失败: %w; 输出: %s", err, summary)
		}
		return "", fmt.Errorf("模板任务执行失败: %w", err)
	}
	return summary, nil
}

func syncManifestDir(dir string, keepFiles []string) error {
	if dir == "" {
		return fmt.Errorf("target dir is required")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to prepare manifest dir: %w", err)
	}
	// keepFiles currently represents the managed overlay files prepared by Control Plane.
	// During upgrade we must preserve the target package's built-in connectors/lib assets,
	// so this primitive is intentionally non-destructive.
	// keepFiles 当前仅表示控制面准备的“受管叠加文件”集合。
	// 升级时必须保留目标安装包自带的 connectors/lib 资产，因此这里明确禁止做破坏式清理。
	_ = keepFiles
	return nil
}

func summarizeCommandOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	const maxLen = 1200
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[len(trimmed)-maxLen:]
}

func copyDirFiltered(src, dst string, skipTopLevel map[string]struct{}) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("source dir is not ready: %w", err)
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, info.Mode())
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) > 0 {
			if _, skip := skipTopLevel[parts[0]]; skip {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		targetPath := filepath.Join(dst, rel)
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			dstFile.Close()
			return err
		}
		return dstFile.Close()
	})
}
