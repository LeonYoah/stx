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
	// DefaultSTXJavaProxyVersion is kept for backward compatibility in scripts
	// that reference the old per-SeaTunnel-version jar name.
	// DefaultSTXJavaProxyVersion 保留用于向后兼容仍引用旧精确版本 jar 名的脚本。
	DefaultSTXJavaProxyVersion = "v2"

	// STXJavaProxyJarFileNamePattern defines the packaged jar naming convention.
	// STXJavaProxyJarFileNamePattern 定义 stx-java-proxy jar 的统一命名规则。
	STXJavaProxyJarFileNamePattern = "stx-java-proxy-%s.jar"

	// STXJavaProxyScriptFileName is the shared launcher script name.
	// STXJavaProxyScriptFileName 是统一的 stx-java-proxy 启动脚本名。
	STXJavaProxyScriptFileName = "stx-java-proxy.sh"
)

// ProxyEpochForVersion maps a SeaTunnel cluster version string to its
// stx-java-proxy compatibility epoch label (e.g. "2.3.5" → "v2").
//
// A new epoch label is only introduced when a genuinely breaking API change
// requires a separate jar build; otherwise all versions within the same major
// share the same epoch and the same jar. The epoch label is the single source
// of truth for jar naming: stx-java-proxy-v2.jar, stx-java-proxy-v3.jar, etc.
//
// ProxyEpochForVersion 将 SeaTunnel 集群版本字符串映射到 stx-java-proxy 兼容代际标签
// （例如 "2.3.5" → "v2"）。仅在真正出现 breaking API 变更、需要单独构建 jar 时才引入
// 新的代际标签；同一主版本下的所有小版本共享同一代际和同一 jar。代际标签是 jar 命名的
// 唯一真相来源：stx-java-proxy-v2.jar、stx-java-proxy-v3.jar，以此类推。
func ProxyEpochForVersion(seatunnelVersion string) string {
	trimmed := strings.TrimSpace(seatunnelVersion)
	if trimmed == "" {
		return "v2"
	}
	// 取主版本号（第一个 "." 之前的部分）。
	// Extract major version number (everything before the first ".").
	major := trimmed
	if idx := strings.Index(trimmed, "."); idx >= 0 {
		major = trimmed[:idx]
	}
	switch major {
	case "3":
		// 3.x 目前与 v2 jar 的核心存储接口完全兼容，共用 v2 代际。
		// 真正出现 breaking change 时在此处返回 "v3"，并发布 stx-java-proxy-v3.jar。
		// 3.x is currently API-compatible with the v2 jar at the core storage level.
		// Return "v3" here (and ship stx-java-proxy-v3.jar) only when a genuine
		// breaking change is introduced in a future 3.x release.
		return "v2"
	default:
		// 2.x 及更早版本、未知版本均使用 v2 代际。
		// 2.x and earlier (or unrecognised) versions use the v2 epoch.
		return "v2"
	}
}

// ResolveSTXJavaProxyVersion returns the proxy epoch label for the given
// SeaTunnel version, falling back to the default epoch when version is blank.
// ResolveSTXJavaProxyVersion 返回对应 SeaTunnel 版本的代际标签，版本为空时回退到默认代际。
func ResolveSTXJavaProxyVersion(seatunnelVersion string) string {
	return ProxyEpochForVersion(seatunnelVersion)
}

// STXJavaProxyJarFileName returns the packaged stx-java-proxy jar file name
// for the given SeaTunnel cluster version.
// STXJavaProxyJarFileName 返回与指定 SeaTunnel 集群版本对应的 stx-java-proxy jar 文件名。
func STXJavaProxyJarFileName(seatunnelVersion string) string {
	return fmt.Sprintf(STXJavaProxyJarFileNamePattern, ProxyEpochForVersion(seatunnelVersion))
}
