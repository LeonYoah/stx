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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logger"
	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
)

const (
	// stxJavaProxyAssetBaseEnvVar is an optional Control Plane asset base
	// (…/api/v1/agent/assets) used to pull script/jar when local copies are missing.
	// stxJavaProxyAssetBaseEnvVar 是可选的控制面资产基址（…/api/v1/agent/assets），
	// 本地缺少脚本/jar 时用于拉取。
	stxJavaProxyAssetBaseEnvVar = "STX_JAVA_PROXY_ASSET_BASE"

	// stxJavaProxyControlPlaneLibDir is the default Control Plane lib dir on co-located hosts.
	// stxJavaProxyControlPlaneLibDir 是同机部署时控制面默认 lib 目录。
	stxJavaProxyControlPlaneLibDir = "/opt/stx/lib"

	// stxJavaProxyControlPlaneScriptsDir is the default Control Plane scripts dir on co-located hosts.
	// stxJavaProxyControlPlaneScriptsDir 是同机部署时控制面默认 scripts 目录。
	stxJavaProxyControlPlaneScriptsDir = "/opt/stx/scripts"
)

var stxJavaProxyAssetHTTPClient = &http.Client{Timeout: 60 * time.Second}

// EnsureSTXJavaProxySupportAssets makes sure the Agent home has the launcher script and the
// epoch jar for seatunnelVersion. It first copies from local Control Plane install paths when
// present, then downloads from assetBaseURL (or STX_JAVA_PROXY_ASSET_BASE) when still missing.
// EnsureSTXJavaProxySupportAssets 确保 Agent 主目录具备启动脚本与 seatunnelVersion 对应代际 jar。
// 优先从同机控制面安装目录拷贝；仍缺时再从 assetBaseURL（或环境变量 STX_JAVA_PROXY_ASSET_BASE）下载。
func EnsureSTXJavaProxySupportAssets(ctx context.Context, installDir, seatunnelVersion, assetBaseURL string) error {
	home := resolveSTXJavaProxyHome(installDir)
	scriptName := seatunnelmeta.STXJavaProxyScriptFileName
	jarName := seatunnelmeta.STXJavaProxyJarFileName(seatunnelVersion)
	scriptDest := filepath.Join(home, "scripts", scriptName)
	jarDest := filepath.Join(home, "lib", jarName)

	if err := os.MkdirAll(filepath.Dir(scriptDest), 0o755); err != nil {
		return fmt.Errorf("create stx-java-proxy scripts dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(jarDest), 0o755); err != nil {
		return fmt.Errorf("create stx-java-proxy lib dir: %w", err)
	}

	base := strings.TrimRight(strings.TrimSpace(assetBaseURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv(stxJavaProxyAssetBaseEnvVar)), "/")
	}

	// 脚本是可选资产：默认 Go 直启不再使用它；有本地副本或资产基址时顺带保留（兼容旧 Agent 与人工使用），
	// 缺失或下载失败只记告警，不阻塞启动。
	// The script is an optional asset: the default direct-Java launch no longer uses it; when a local copy
	// or asset base exists it is still staged (legacy agents / manual use), but a missing script or a failed
	// download only logs a warning and never blocks startup.
	if !fileExists(scriptDest) {
		if copied := copyFirstExistingFile([]string{
			filepath.Join(stxJavaProxyControlPlaneScriptsDir, scriptName),
			filepath.Join(installDir, "scripts", scriptName),
			filepath.Join(installDir, "bin", scriptName),
		}, scriptDest); copied {
			_ = os.Chmod(scriptDest, 0o755)
			logger.InfoF(ctx, "[stx-java-proxy] staged launcher script from local path to %s / 已从本机路径暂存启动脚本到 %s", scriptDest, scriptDest)
		} else if base != "" {
			if err := downloadSTXJavaProxyAsset(ctx, base+"/"+scriptName, scriptDest); err != nil {
				logger.WarnF(ctx, "[stx-java-proxy] download optional launcher script failed (direct Java launch unaffected): %v / 下载可选启动脚本失败（直启不受影响）: %v", err, err)
			} else {
				_ = os.Chmod(scriptDest, 0o755)
				logger.InfoF(ctx, "[stx-java-proxy] downloaded launcher script to %s / 已下载启动脚本到 %s", scriptDest, scriptDest)
			}
		}
	}

	if !fileExists(jarDest) {
		localJarCandidates := []string{
			filepath.Join(stxJavaProxyControlPlaneLibDir, jarName),
			filepath.Join("/opt/stx/default-lib", jarName),
			filepath.Join(installDir, "lib", jarName),
		}
		if copied := copyFirstExistingFile(localJarCandidates, jarDest); copied {
			logger.InfoF(ctx, "[stx-java-proxy] staged jar from local path to %s / 已从本机路径暂存 jar 到 %s", jarDest, jarDest)
		} else if base != "" {
			// Prefer filename-style URL (same shape as .sh); fall back to ?version= for older control planes.
			// 优先文件名路径（与 .sh 一致）；旧控制面再回退 ?version=。
			epoch := seatunnelmeta.ProxyEpochForVersion(seatunnelVersion)
			urls := []string{
				base + "/" + jarName,
				base + "/stx-java-proxy.jar?version=" + epoch,
			}
			var lastErr error
			for _, u := range urls {
				if err := downloadSTXJavaProxyAsset(ctx, u, jarDest); err != nil {
					lastErr = err
					continue
				}
				lastErr = nil
				break
			}
			if lastErr != nil {
				return fmt.Errorf("download stx-java-proxy jar: %w", lastErr)
			}
			logger.InfoF(ctx, "[stx-java-proxy] downloaded jar to %s / 已下载 jar 到 %s", jarDest, jarDest)
		}
	}

	// jar 是直启的核心必需项：所有来源都拿不到时必须报错（调用方会降级为告警，真正的阻塞在解析阶段）。
	// The jar is the core requirement for direct launch: when every source misses it this is an error
	// (callers degrade it to a warning; the real block happens at jar resolution).
	if !fileExists(jarDest) {
		return fmt.Errorf("stx-java-proxy jar %s is unavailable under %s", jarName, filepath.Dir(jarDest))
	}
	return nil
}

func copyFirstExistingFile(candidates []string, dest string) bool {
	for _, src := range candidates {
		if !fileExists(src) {
			continue
		}
		if err := copyFileContents(src, dest); err != nil {
			continue
		}
		return true
	}
	return false
}

func copyFileContents(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func downloadSTXJavaProxyAsset(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := stxJavaProxyAssetHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s -> %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
