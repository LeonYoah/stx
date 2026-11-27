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

// Package seatunnel 提供 SeaTunnel 版本元数据等跨层共享信息。
// Package seatunnel provides shared SeaTunnel metadata used across layers.
package seatunnel

import (
	"fmt"
	"strings"
)

const (
	// ArchiveVersionsURL 是 SeaTunnel 官方历史版本索引入口。
	// ArchiveVersionsURL is the official SeaTunnel archive index.
	ArchiveVersionsURL = "https://archive.apache.org/dist/seatunnel/"

	// DefaultInstallDirTemplate 是默认版本化安装目录模板。
	// DefaultInstallDirTemplate is the default versioned install directory template.
	DefaultInstallDirTemplate = "/opt/seatunnel-{version}"
)

var defaultVersionMetadata = VersionMetadata{
	RecommendedVersion: "3.1.0",
	FallbackVersions: []string{
		"3.1.0",
		"3.0.0",
		"2.3.13",
		"2.3.12",
		"2.3.11",
		"2.3.10",
		"2.3.9",
		"2.3.8",
		"2.3.7",
		"2.3.6",
		"2.3.5",
		"2.3.4",
		"2.3.3",
		"2.3.2",
		"2.3.1",
		"2.3.0",
		"2.2.0-beta",
		"2.1.3",
		"2.1.2",
		"2.1.1",
		"2.1.0",
	},
	VersionsSource: ArchiveVersionsURL,
}

// VersionMetadata 描述 SeaTunnel 版本元数据入口。
// VersionMetadata describes the central SeaTunnel version metadata entry point.
type VersionMetadata struct {
	RecommendedVersion string   `json:"recommended_version"`
	FallbackVersions   []string `json:"fallback_versions"`
	VersionsSource     string   `json:"versions_source"`
}

// Metadata 返回版本元数据快照。
// Metadata returns a snapshot of the version metadata.
func Metadata() VersionMetadata {
	return VersionMetadata{
		RecommendedVersion: defaultVersionMetadata.RecommendedVersion,
		FallbackVersions:   append([]string(nil), defaultVersionMetadata.FallbackVersions...),
		VersionsSource:     defaultVersionMetadata.VersionsSource,
	}
}

// DefaultVersion 返回统一默认版本。
// DefaultVersion returns the centralized default version.
func DefaultVersion() string {
	return defaultVersionMetadata.RecommendedVersion
}

// RecommendedVersion 返回推荐版本。
// RecommendedVersion returns the recommended version.
func RecommendedVersion() string {
	return defaultVersionMetadata.RecommendedVersion
}

// FallbackVersions 返回备用版本列表副本。
// FallbackVersions returns a copy of the fallback version list.
func FallbackVersions() []string {
	return append([]string(nil), defaultVersionMetadata.FallbackVersions...)
}

// ResolveVersion 在调用方未指定版本时回退到统一默认值。
// ResolveVersion falls back to the centralized default when callers omit the version.
func ResolveVersion(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed != "" {
		return trimmed
	}
	return DefaultVersion()
}

// DefaultInstallDir 根据版本构造版本化安装目录。
// DefaultInstallDir builds the versioned install directory for a version.
func DefaultInstallDir(version string) string {
	resolvedVersion := ResolveVersion(version)
	return fmt.Sprintf("/opt/seatunnel-%s", resolvedVersion)
}
