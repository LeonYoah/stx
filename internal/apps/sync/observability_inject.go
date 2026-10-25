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

package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/LeonYoah/stx/internal/seatunnel"
)

const (
	// observabilitySettingKey 是任务定义中实时可观测性开关的键（auto/on/off）。
	// observabilitySettingKey is the task definition key for the realtime observability switch (auto/on/off).
	observabilitySettingKey = "observability"

	// ObservabilityModeAuto 表示流式作业开启、批作业关闭。
	// ObservabilityModeAuto enables observability for streaming jobs and disables it for batch jobs.
	ObservabilityModeAuto = "auto"
	// ObservabilityModeOn 表示无论作业模式都开启。
	// ObservabilityModeOn always enables observability.
	ObservabilityModeOn = "on"
	// ObservabilityModeOff 表示不注入。
	// ObservabilityModeOff never injects observability.
	ObservabilityModeOff = "off"

	// 注入的默认值：桶长与引擎采集间隔对齐，保留窗口取引擎允许的最大值。
	// Injected defaults: bucket aligned with the engine poll interval, retention at the engine maximum.
	defaultObservabilityBucketMs         = 5000
	defaultObservabilityRetentionMinutes = 10

	// observabilityInjectMarker 标记 STX 注入的 HOCON 片段，恢复作业复用提交内容时据此识别，避免重复注入。
	// observabilityInjectMarker tags the STX-injected HOCON block so recovered submits that reuse content are not injected twice.
	observabilityInjectMarker = "# STX: injected realtime observability defaults; settings in this job take precedence."
)

// 注入结果原因，写入 submit_spec.observability.reason，供实时接口解释 not_enabled。
// Injection reasons stored in submit_spec.observability.reason so the realtime API can explain not_enabled.
const (
	observabilityReasonInjected           = "injected"
	observabilityReasonAlreadyInjected    = "already_injected"
	observabilityReasonUserConfigured     = "user_configured"
	observabilityReasonDisabledByTask     = "disabled_by_task"
	observabilityReasonBatchJob           = "batch_job"
	observabilityReasonVersionUnsupported = "version_unsupported"
	observabilityReasonVersionUnknown     = "version_unknown"
	observabilityReasonFormatUnsupported  = "format_unsupported"
	observabilityReasonInvalidContent     = "invalid_content"
	observabilityReasonEmptyContent       = "empty_content"
)

var (
	// jobModePattern 轻量匹配 HOCON/JSON 中的 job.mode，覆盖 job.mode = "STREAMING"、"job.mode": "BATCH" 等写法。
	// jobModePattern lightly matches job.mode in HOCON/JSON, e.g. job.mode = "STREAMING" or "job.mode": "BATCH".
	jobModePattern = regexp.MustCompile(`(?i)["']?job\.mode["']?\s*[:=]\s*["']?(STREAMING|BATCH)\b`)
	// observabilityKeyPattern 判断用户内容里是否已出现 observability 配置。
	// observabilityKeyPattern detects whether user content already mentions observability settings.
	observabilityKeyPattern = regexp.MustCompile(`(?i)\bobservability\b`)
)

// observabilityInjection 描述一次提交的注入判定结果。
// observabilityInjection describes the injection decision for one submit.
type observabilityInjection struct {
	Injected  bool
	Mode      string
	Reason    string
	Streaming bool
	Version   string
	// Supported 表示目标集群版本支持实时可观测性，前端据此决定是否展示入口（不在前端比较版本）。
	// Supported reports whether the target cluster version supports realtime observability so the frontend can
	// decide whether to show entry points without comparing versions itself.
	Supported bool
}

// toMap 转成写入 submit_spec 的结构。
// toMap converts the decision into the submit_spec structure.
func (o observabilityInjection) toMap() JSONMap {
	return JSONMap{
		"injected":  o.Injected,
		"mode":      o.Mode,
		"reason":    o.Reason,
		"streaming": o.Streaming,
		"version":   o.Version,
		"supported": o.Supported,
	}
}

// resolveObservabilityMode 读取任务设置，缺省或非法值按 auto 处理。
// resolveObservabilityMode reads the task setting; missing or invalid values fall back to auto.
func resolveObservabilityMode(definition JSONMap) string {
	switch strings.ToLower(strings.TrimSpace(stringValue(definition, observabilitySettingKey))) {
	case ObservabilityModeOn:
		return ObservabilityModeOn
	case ObservabilityModeOff:
		return ObservabilityModeOff
	default:
		return ObservabilityModeAuto
	}
}

// detectStreamingJob 优先看内容中的 job.mode，内容里找不到时回落到任务的 Mode 字段。
// detectStreamingJob prefers job.mode in the content and falls back to the task Mode field.
func detectStreamingJob(content string, taskMode TaskMode) bool {
	if match := jobModePattern.FindStringSubmatch(content); len(match) == 2 {
		return strings.EqualFold(match[1], "STREAMING")
	}
	return taskMode == TaskModeStreaming
}

// resolveTaskClusterVersion 返回任务目标集群的 SeaTunnel 版本，取不到时回落到任务声明的引擎版本。
// resolveTaskClusterVersion returns the target cluster SeaTunnel version, falling back to the task engine version.
func (s *Service) resolveTaskClusterVersion(ctx context.Context, task *Task) string {
	if task == nil {
		return ""
	}
	if s != nil && s.clusterVersionProvider != nil && task.ClusterID > 0 {
		if version, err := s.clusterVersionProvider.GetClusterVersion(ctx, task.ClusterID); err == nil && strings.TrimSpace(version) != "" {
			return strings.TrimSpace(version)
		}
	}
	return strings.TrimSpace(task.EngineVersion)
}

// applyObservabilityInjection 按集群版本与任务设置决定是否在提交内容中注入 env.engine.observability。
// 用户自己写的 observability 配置始终优先；任何解析失败都原样提交，不阻断作业。
// applyObservabilityInjection decides whether to inject env.engine.observability into the submit body based on
// the cluster version and task setting. User-provided observability settings always win; any parse failure
// submits the original body unchanged so the job is never blocked.
func (s *Service) applyObservabilityInjection(ctx context.Context, task *Task, body []byte, format string) ([]byte, observabilityInjection) {
	result := observabilityInjection{Mode: ObservabilityModeAuto}
	if task == nil {
		result.Reason = observabilityReasonEmptyContent
		return body, result
	}
	result.Mode = resolveObservabilityMode(task.Definition)
	content := string(body)
	result.Streaming = detectStreamingJob(content, task.Mode)
	result.Version = s.resolveTaskClusterVersion(ctx, task)
	result.Supported = seatunnel.SupportsRealtimeObservability(result.Version)

	switch {
	case strings.TrimSpace(content) == "":
		result.Reason = observabilityReasonEmptyContent
		return body, result
	case strings.Contains(content, observabilityInjectMarker):
		result.Injected = true
		result.Reason = observabilityReasonAlreadyInjected
		return body, result
	case result.Mode == ObservabilityModeOff:
		result.Reason = observabilityReasonDisabledByTask
		return body, result
	case result.Mode == ObservabilityModeAuto && !result.Streaming:
		result.Reason = observabilityReasonBatchJob
		return body, result
	case result.Version == "":
		result.Reason = observabilityReasonVersionUnknown
		return body, result
	case !result.Supported:
		result.Reason = observabilityReasonVersionUnsupported
		return body, result
	}

	switch normalizeSubmitFormat(format) {
	case "hocon":
		if observabilityKeyPattern.MatchString(content) {
			result.Reason = observabilityReasonUserConfigured
			return body, result
		}
		result.Injected = true
		result.Reason = observabilityReasonInjected
		return []byte(injectObservabilityHOCON(content)), result
	case "json":
		injected, reason := injectObservabilityJSON(body)
		result.Reason = reason
		result.Injected = reason == observabilityReasonInjected || reason == observabilityReasonAlreadyInjected
		return injected, result
	default:
		result.Reason = observabilityReasonFormatUnsupported
		return body, result
	}
}

// observabilityHOCONBlock 返回注入的 HOCON 片段（含标记行）。
// observabilityHOCONBlock returns the injected HOCON block including the marker line.
func observabilityHOCONBlock() string {
	return observabilityInjectMarker + "\n" +
		"env { engine { observability { enabled = true, bucket_ms = 5000, retention_minutes = 10 } } }\n"
}

// injectObservabilityHOCON 把默认片段放在用户内容之前；HOCON 同名对象合并且后写覆盖先写，所以用户配置优先。
// 根对象带花括号时插入到第一个 '{' 之后，避免出现非法的"顶层字段 + 花括号根对象"混写。
// injectObservabilityHOCON places the defaults before user content; HOCON merges same-named objects and later
// values win, so user settings take precedence. With a braced root object the block goes right after the first
// '{' to avoid mixing top-level fields with a braced root, which HOCON rejects.
func injectObservabilityHOCON(content string) string {
	block := observabilityHOCONBlock()
	if idx := firstSignificantHOCONIndex(content); idx >= 0 && content[idx] == '{' {
		return content[:idx+1] + "\n" + block + content[idx+1:]
	}
	return block + content
}

// firstSignificantHOCONIndex 返回跳过空白与注释行后第一个有效字符的位置，找不到时返回 -1。
// firstSignificantHOCONIndex returns the index of the first significant character after whitespace and comment lines, or -1.
func firstSignificantHOCONIndex(content string) int {
	i := 0
	for i < len(content) {
		switch c := content[i]; {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '#' || (c == '/' && i+1 < len(content) && content[i+1] == '/'):
			next := strings.IndexByte(content[i:], '\n')
			if next < 0 {
				return -1
			}
			i += next + 1
		default:
			return i
		}
	}
	return -1
}

// injectObservabilityJSON 只在 env.engine.observability 缺失时补默认值；env 不是对象或 JSON 无法解析时原样返回。
// injectObservabilityJSON adds defaults only when env.engine.observability is absent; it returns the body unchanged
// when env is not an object or the JSON cannot be parsed.
func injectObservabilityJSON(body []byte) ([]byte, string) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil || root == nil {
		return body, observabilityReasonInvalidContent
	}
	env := map[string]any{}
	if raw, exists := root["env"]; exists {
		existing, ok := raw.(map[string]any)
		if !ok {
			return body, observabilityReasonInvalidContent
		}
		env = existing
	}
	for key := range env {
		if strings.HasPrefix(strings.ToLower(key), "engine.observability") {
			return body, observabilityReasonUserConfigured
		}
	}
	engine := map[string]any{}
	if raw, exists := env["engine"]; exists {
		existing, ok := raw.(map[string]any)
		if !ok {
			return body, observabilityReasonInvalidContent
		}
		engine = existing
	}
	if existing, exists := engine["observability"]; exists {
		if isDefaultObservabilityJSON(existing) {
			return body, observabilityReasonAlreadyInjected
		}
		return body, observabilityReasonUserConfigured
	}
	engine["observability"] = map[string]any{
		"enabled":           true,
		"bucket_ms":         defaultObservabilityBucketMs,
		"retention_minutes": defaultObservabilityRetentionMinutes,
	}
	env["engine"] = engine
	root["env"] = env
	encoded, err := json.Marshal(root)
	if err != nil {
		return body, observabilityReasonInvalidContent
	}
	return encoded, observabilityReasonInjected
}

// isDefaultObservabilityJSON 判断 JSON 里的 observability 是否正是 STX 注入的默认值（恢复作业复用提交内容时出现）。
// isDefaultObservabilityJSON reports whether the JSON observability object equals the STX defaults (seen when recovering).
func isDefaultObservabilityJSON(value any) bool {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 3 {
		return false
	}
	enabled, _ := object["enabled"].(bool)
	return enabled &&
		jsonNumberEquals(object["bucket_ms"], defaultObservabilityBucketMs) &&
		jsonNumberEquals(object["retention_minutes"], defaultObservabilityRetentionMinutes)
}

// jsonNumberEquals 兼容 json.Number 与 float64 两种解码结果比较整数。
// jsonNumberEquals compares an integer against json.Number or float64 decode results.
func jsonNumberEquals(value any, want int64) bool {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		return err == nil && parsed == want
	case float64:
		return int64(typed) == want && typed == float64(int64(typed))
	default:
		return false
	}
}
