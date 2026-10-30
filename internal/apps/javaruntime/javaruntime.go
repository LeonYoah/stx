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

// Package javaruntime 是控制面解释节点 JDK 的唯一入口：校验用户填写的 java_home、生成 check_java 参数，
// 并把 Agent 的 check_java 结果统一解释为通过 / 警告 / 失败。安装预检、扩容预检与节点详情都复用它。
// Package javaruntime is the Control Plane's single entry for node JDKs: it validates the user-entered java_home,
// builds check_java parameters and interprets the Agent's check_java output as passed / warning / failed. Install
// precheck, scale-out precheck and node detail all reuse it.
package javaruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"
)

// ParamJavaHome 是下发给 Agent 的节点 JDK 参数名，与 Agent 侧 javaenv.ParamJavaHome 一致。
// ParamJavaHome is the node JDK parameter sent to the Agent, matching the Agent's javaenv.ParamJavaHome.
const ParamJavaHome = "java_home"

// MaxJavaHomeLength 与 cluster_nodes.java_home 的列宽一致。
// MaxJavaHomeLength matches the cluster_nodes.java_home column size.
const MaxJavaHomeLength = 512

// 检查状态。/ Check statuses.
const (
	StatusPassed  = "passed"
	StatusWarning = "warning"
	StatusFailed  = "failed"
)

// 来源。/ Sources.
const (
	SourceCustom   = "custom"
	SourceDetected = "detected"
)

// ReasonAgentUnsupported 表示 Agent 版本过旧，不支持自定义 JDK。
// ReasonAgentUnsupported means the Agent is too old to support a custom JDK.
const ReasonAgentUnsupported = "agent_unsupported"

// ErrInvalidJavaHome 表示用户填写的 JDK 路径格式不合法。
// ErrInvalidJavaHome means the user-entered JDK path is malformed.
var ErrInvalidJavaHome = errors.New("invalid java_home / JDK 路径不合法")

// Normalize 去掉首尾空白并清理路径（去掉末尾斜杠等）；空字符串表示未配置，沿用自动探测。
// Normalize trims whitespace and cleans the path (dropping trailing slashes and such); empty means not configured,
// keeping auto detection.
func Normalize(home string) string {
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return path.Clean(home)
}

// Validate 校验并规范化用户填写的 JDK 路径：为空合法（自动探测）；非空时必须是绝对路径、不含控制字符且不超过列宽。
// 路径是否存在、bin/java 是否可执行由 Agent 在预检中检查。
// Validate checks and normalizes a user-entered JDK path: empty is valid (auto detection); a non-empty path must be
// absolute, contain no control characters and fit the column. Existence and an executable bin/java are checked by
// the Agent during precheck.
func Validate(home string) (string, error) {
	normalized := Normalize(home)
	if normalized == "" {
		return "", nil
	}
	if len(normalized) > MaxJavaHomeLength {
		return "", fmt.Errorf("%w: 长度超过 %d / longer than %d", ErrInvalidJavaHome, MaxJavaHomeLength, MaxJavaHomeLength)
	}
	if !strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("%w: 必须是绝对路径 %s / must be an absolute path %s", ErrInvalidJavaHome, normalized, normalized)
	}
	for _, r := range normalized {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: 包含控制字符 / contains control characters", ErrInvalidJavaHome)
		}
	}
	return normalized, nil
}

// CheckParams 生成 check_java 的 Agent 参数。始终带上 java_home 键（可能为空），表示调用方已显式决定，
// 中心化注入不会再按节点记录覆盖。
// CheckParams builds the Agent parameters for check_java. The java_home key is always present (possibly empty) to
// mark an explicit decision, so the central injection never overrides it from node records.
func CheckParams(home string) map[string]string {
	return map[string]string{
		"sub_command": "check_java",
		ParamJavaHome: Normalize(home),
	}
}

// Result 是一次 check_java 的解释结果，记录实际检查的路径与版本。
// Result is an interpreted check_java run, recording the path and version actually checked.
type Result struct {
	Status        string `json:"status"`
	Message       string `json:"message"`
	JavaHome      string `json:"java_home"`
	JavaBin       string `json:"java_bin"`
	Source        string `json:"source"`
	Version       int    `json:"version"`
	VersionString string `json:"version_string,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type agentResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Details map[string]string `json:"details"`
}

// Interpret 解释 Agent 的 check_java 输出。configured 为调用方下发的 java_home（为空表示自动探测）。
// 规则（AC2）：8 或 11 通过；其他可识别版本警告；Agent 报告失败（路径缺失、不可执行、未安装或版本过低）为失败。
// Interpret interprets the Agent's check_java output. configured is the java_home the caller sent (empty means
// auto detection). Rules (AC2): 8 or 11 pass; other recognized versions warn; an Agent failure (missing path,
// non-executable, not installed or too old) fails.
func Interpret(configured string, success bool, output string) *Result {
	configured = Normalize(configured)
	result := &Result{Source: SourceDetected, JavaHome: configured}
	if configured != "" {
		result.Source = SourceCustom
	}

	var resp agentResponse
	parsed := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp) == nil
	if parsed {
		if v := strings.TrimSpace(resp.Details["java_home_source"]); v != "" {
			result.Source = v
		}
		if v := strings.TrimSpace(resp.Details["java_home"]); v != "" {
			result.JavaHome = v
		}
		result.JavaBin = strings.TrimSpace(resp.Details["java_bin"])
		result.Reason = strings.TrimSpace(resp.Details["error_reason"])
		result.VersionString = strings.TrimSpace(resp.Details["version_string"])
		if v, err := strconv.Atoi(strings.TrimSpace(resp.Details["installed_version"])); err == nil {
			result.Version = v
		}
		result.Message = strings.TrimSpace(resp.Message)
	} else {
		result.Message = strings.TrimSpace(output)
	}

	// 旧版 Agent 不认识 java_home，会静默按 PATH 探测；指定了路径时必须视为失败，避免用错 JDK。
	// An old Agent ignores java_home and silently detects from PATH; with a configured path that must fail so the
	// wrong JDK is never used.
	if configured != "" && (!parsed || strings.TrimSpace(resp.Details["java_home_source"]) != SourceCustom) {
		result.Source = SourceCustom
		result.JavaHome = configured
		result.Status = StatusFailed
		result.Reason = ReasonAgentUnsupported
		result.Message = "Agent 版本不支持自定义 JDK 路径，请先升级 Agent / The Agent version does not support a custom JDK path; upgrade the Agent first"
		return result
	}

	switch {
	case !success:
		result.Status = StatusFailed
	case result.Version == 8 || result.Version == 11:
		result.Status = StatusPassed
	default:
		result.Status = StatusWarning
	}
	return result
}

// FailureMessage 返回带节点、路径与原因的双语失败信息，供预检阻断时展示（AC1c / AC2）。
// FailureMessage returns a bilingual failure message naming the node, path and reason for blocked prechecks
// (AC1c / AC2).
func (r *Result) FailureMessage(node string) string {
	target := firstNonBlank(r.JavaBin, r.JavaHome, "java")
	reason := firstNonBlank(r.Message, r.Reason, "unknown")
	return fmt.Sprintf("节点 %s 的 JDK 不可用（%s）：%s / JDK on node %s is not usable (%s): %s", node, target, reason, node, target, reason)
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
