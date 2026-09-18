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
	// DefaultSTXJavaProxyVersion is the fallback stx-java-proxy implementation
	// shipped with STX until newer SeaTunnel-specific probe jars are added.
	// DefaultSTXJavaProxyVersion 是 STX 当前内置的 stx-java-proxy 回退版本。
	DefaultSTXJavaProxyVersion = "2.3.13"

	// STXJavaProxyJarFileNamePattern defines the packaged jar naming convention.
	// STXJavaProxyJarFileNamePattern 定义 stx-java-proxy jar 的统一命名规则。
	STXJavaProxyJarFileNamePattern = "stx-java-proxy-%s.jar"

	// STXJavaProxyScriptFileName is the shared launcher script name.
	// STXJavaProxyScriptFileName 是统一的 stx-java-proxy 启动脚本名。
	STXJavaProxyScriptFileName = "stx-java-proxy.sh"
)

// ResolveSTXJavaProxyVersion falls back to the packaged default when no
// SeaTunnel-specific stx-java-proxy jar version is provided.
// ResolveSTXJavaProxyVersion 在未指定版本时回退到内置默认 stx-java-proxy 版本。
func ResolveSTXJavaProxyVersion(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed != "" {
		return trimmed
	}
	return DefaultSTXJavaProxyVersion
}

// STXJavaProxyJarFileName returns the packaged stx-java-proxy jar file name
// for a SeaTunnel version.
// STXJavaProxyJarFileName 返回指定 SeaTunnel 版本对应的 stx-java-proxy jar 文件名。
func STXJavaProxyJarFileName(version string) string {
	return fmt.Sprintf(STXJavaProxyJarFileNamePattern, ResolveSTXJavaProxyVersion(version))
}
