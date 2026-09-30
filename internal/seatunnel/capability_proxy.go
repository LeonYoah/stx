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

package seatunnel

import (
	"fmt"
	"strings"
)

const (
	// DefaultSTXJavaProxyVersion is the default packaged proxy epoch for scripts
	// that do not yet know the target SeaTunnel cluster version.
	// DefaultSTXJavaProxyVersion 是尚不知道目标 SeaTunnel 集群版本时脚本使用的默认代际。
	DefaultSTXJavaProxyVersion = "v2"

	// STXJavaProxyEpochV2 is the jar epoch compiled against SeaTunnel 2.3.x.
	// STXJavaProxyEpochV2 是按 SeaTunnel 2.3.x 编译的 jar 代际。
	STXJavaProxyEpochV2 = "v2"

	// STXJavaProxyEpochV3 is the jar epoch compiled against SeaTunnel 3.0.x.
	// STXJavaProxyEpochV3 是按 SeaTunnel 3.0.x 编译的 jar 代际。
	STXJavaProxyEpochV3 = "v3"

	// STXJavaProxyJarFileNamePattern defines the packaged jar naming convention.
	// STXJavaProxyJarFileNamePattern 定义 stx-java-proxy jar 的统一命名规则。
	STXJavaProxyJarFileNamePattern = "stx-java-proxy-%s.jar"

	// STXJavaProxyScriptFileName is the shared launcher script name.
	// STXJavaProxyScriptFileName 是统一的 stx-java-proxy 启动脚本名。
	STXJavaProxyScriptFileName = "stx-java-proxy.sh"
)

// ProxyEpochForVersion maps a SeaTunnel cluster version string (or an epoch
// label such as "v2"/"v3") to its stx-java-proxy compatibility epoch.
//
// Epoch labels are the single source of truth for jar naming:
// stx-java-proxy-v2.jar, stx-java-proxy-v3.jar.
//
// ProxyEpochForVersion 将 SeaTunnel 集群版本字符串（或已是 "v2"/"v3" 的代际标签）
// 映射到 stx-java-proxy 兼容代际。代际标签是 jar 命名的唯一真相来源。
func ProxyEpochForVersion(seatunnelVersion string) string {
	trimmed := strings.TrimSpace(seatunnelVersion)
	if trimmed == "" {
		return STXJavaProxyEpochV2
	}
	// 已是代际标签则直接返回，供下载接口 ?version=v3 使用。
	// Pass through epoch labels so asset download ?version=v3 resolves correctly.
	switch trimmed {
	case STXJavaProxyEpochV2, STXJavaProxyEpochV3:
		return trimmed
	}

	// 取主版本号（第一个 "." 之前的部分）。
	// Extract major version number (everything before the first ".").
	major := trimmed
	if idx := strings.Index(trimmed, "."); idx >= 0 {
		major = trimmed[:idx]
	}
	// 去掉可选的 "v" 前缀（如 v3.0.0）。
	// Strip optional leading "v" (e.g. v3.0.0).
	major = strings.TrimPrefix(major, "v")
	switch major {
	case "3":
		return STXJavaProxyEpochV3
	default:
		return STXJavaProxyEpochV2
	}
}

// ResolveSTXJavaProxyVersion returns the proxy epoch label for the given
// SeaTunnel version, falling back to the default epoch when version is blank.
// ResolveSTXJavaProxyVersion 返回对应 SeaTunnel 版本的代际标签，版本为空时回退到默认代际。
func ResolveSTXJavaProxyVersion(seatunnelVersion string) string {
	return ProxyEpochForVersion(seatunnelVersion)
}

// STXJavaProxyJarFileName returns the packaged stx-java-proxy jar file name
// for the given SeaTunnel cluster version or epoch label.
// STXJavaProxyJarFileName 返回与指定 SeaTunnel 集群版本或代际标签对应的 jar 文件名。
func STXJavaProxyJarFileName(seatunnelVersion string) string {
	return fmt.Sprintf(STXJavaProxyJarFileNamePattern, ProxyEpochForVersion(seatunnelVersion))
}
