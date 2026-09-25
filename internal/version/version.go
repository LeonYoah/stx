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

// Package version 保存 STX 服务端、CLI 与 Agent 共用的产品版本信息。
// Package version stores product version information shared by the STX server, CLI, and Agent.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version 是当前 STX 发布版本，可在发布构建中通过 ldflags 覆盖。
// Version is the current STX release version and can be overridden with ldflags.
var Version = "1.0.0"

// GitCommit 是构建时的短提交哈希，可在发布构建中通过 ldflags 覆盖。
// GitCommit is the short commit hash at build time and can be overridden with ldflags.
var GitCommit = "unknown"

// BuildTime 是构建时间（UTC），可在发布构建中通过 ldflags 覆盖。
// BuildTime is the UTC build timestamp and can be overridden with ldflags.
var BuildTime = "unknown"

// MinCLIVersion 是当前服务端接受的最低 CLI 版本；破坏兼容时手动抬高。
// MinCLIVersion is the minimum CLI version accepted by the current server; raise manually on breaking changes.
var MinCLIVersion = "1.0.0"

// Info 是对外暴露的构建信息快照。
// Info is the public build-information snapshot.
type Info struct {
	Version   string `json:"version" yaml:"version"`
	GitCommit string `json:"git_commit" yaml:"git_commit"`
	BuildTime string `json:"build_time" yaml:"build_time"`
}

// Current 返回当前进程的构建信息。
// Current returns build information for the current process.
func Current() Info {
	return Info{
		Version:   Normalize(Version),
		GitCommit: strings.TrimSpace(GitCommit),
		BuildTime: strings.TrimSpace(BuildTime),
	}
}

// Normalize 去掉可选的 v 前缀并修剪空白。
// Normalize strips an optional leading v and trims whitespace.
func Normalize(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return ""
	}
	if (trimmed[0] == 'v' || trimmed[0] == 'V') && len(trimmed) > 1 && trimmed[1] >= '0' && trimmed[1] <= '9' {
		return trimmed[1:]
	}
	return trimmed
}

// Compare 比较两个 SemVer 风格版本（可带 v 前缀），返回 -1/0/1。
// Compare compares two SemVer-like versions (optional v prefix) and returns -1/0/1.
func Compare(left, right string) int {
	leftParts := splitVersion(Normalize(left))
	rightParts := splitVersion(Normalize(right))
	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}
	for i := 0; i < maxLen; i++ {
		var leftRaw, rightRaw string
		if i < len(leftParts) {
			leftRaw = leftParts[i]
		}
		if i < len(rightParts) {
			rightRaw = rightParts[i]
		}
		leftNum, leftSuffix := parsePart(leftRaw)
		rightNum, rightSuffix := parsePart(rightRaw)
		if leftNum != rightNum {
			if leftNum < rightNum {
				return -1
			}
			return 1
		}
		if leftSuffix != rightSuffix {
			if leftSuffix == "" {
				return 1
			}
			if rightSuffix == "" {
				return -1
			}
			return strings.Compare(leftSuffix, rightSuffix)
		}
	}
	return 0
}

// IsBelowMin 判断当前 CLI 版本是否低于服务端要求的最低版本。
// IsBelowMin reports whether the CLI version is below the server minimum.
func IsBelowMin(cliVersion, minVersion string) bool {
	cli := Normalize(cliVersion)
	min := Normalize(minVersion)
	if cli == "" || min == "" {
		return false
	}
	// 开发构建不强制拦截 / Development builds skip enforcement
	if cli == "dev" || strings.HasSuffix(cli, "-dev") {
		return false
	}
	return Compare(cli, min) < 0
}

// CompatibilityWarning 生成 CLI 过旧时的提示文案。
// CompatibilityWarning builds the message shown when the CLI is below the minimum.
func CompatibilityWarning(cliVersion, minVersion, serverVersion string) string {
	return fmt.Sprintf(
		"CLI version %s is below server minimum %s (server %s); upgrade the CLI to avoid protocol mismatches",
		Normalize(cliVersion),
		Normalize(minVersion),
		Normalize(serverVersion),
	)
}

func splitVersion(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ".")
}

func parsePart(part string) (int, string) {
	if part == "" {
		return 0, ""
	}
	idx := strings.Index(part, "-")
	numeric := part
	suffix := ""
	if idx >= 0 {
		numeric = part[:idx]
		suffix = part[idx:]
	}
	num, err := strconv.Atoi(numeric)
	if err != nil {
		return 0, part
	}
	return num, suffix
}
