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
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrorCodeConnectorMissing 是集群缺连接器时响应里的 error_code；CLI 与前端据此给出安装指引。
// ErrorCodeConnectorMissing is the error_code returned when the cluster lacks a connector; CLI and UI use it to guide installation.
const ErrorCodeConnectorMissing = "connector_missing"

// ErrConnectorMissing 表示引擎找不到任务需要的连接器（集群未安装对应插件）。
// ErrConnectorMissing reports that the engine cannot find a connector the job needs (plugin not installed on the cluster).
var ErrConnectorMissing = errors.New("sync: cluster is missing a required connector")

// engineConnectorMissingPattern 匹配 SeaTunnel FactoryUtil 在插件缺失时的固定措辞（API-06），2.3.x 与 3.0 一致。
// engineConnectorMissingPattern matches SeaTunnel FactoryUtil's fixed wording when a plugin is absent (API-06), same in 2.3.x and 3.0.
var engineConnectorMissingPattern = regexp.MustCompile(`(?i)Unable to create a (source|sink|transform) for identifier '([^']+)'`)

// engineCausedBySplit 把压平的异常文本按 "caused by:" 切分层段：java-proxy 用 " | caused by: " 拼接，
// 引擎作业失败原因里则是 Java 原生 "Caused by: " 堆栈。
// engineCausedBySplit splits a flattened exception chain on "caused by:": the java-proxy joins layers
// with " | caused by: ", while engine job failure reasons use Java's native "Caused by: " stack lines.
var engineCausedBySplit = regexp.MustCompile(`(?i)\s*\|?\s*caused by:`)

// enginePluginAbsentSignals 是「确实没找到插件」的确定信号，逐层段匹配，
// 对齐 java-proxy isPluginNotFound 对 Throwable 链的判定（FactoryUtil 工厂发现失败 / PluginIdentifier not found）。
// enginePluginAbsentSignals are the definitive "plugin genuinely absent" signals, matched per chain
// segment — mirroring the java-proxy isPluginNotFound check over the Throwable chain (FactoryUtil
// discovery failure / PluginIdentifier not found).
var enginePluginAbsentSignals = []*regexp.Regexp{
	regexp.MustCompile(`(?i)could not find any factor`),
	regexp.MustCompile(`(?i)no factory found for identifier`),
	regexp.MustCompile(`(?i)pluginidentifier[^\n]*not found`),
	regexp.MustCompile(`(?i)not found[^\n]*(?:factor|plugin)|(?:factor|plugin)[^\n]*not found`),
	regexp.MustCompile(`连接器未安装`),
}

// engineMessageIndicatesMissingPlugin 只在「确实没找到插件」时返回 true。
// API-06 外层措辞把所有工厂初始化失败包成同一句：有 caused-by 链时逐段找确定信号，
// 找不到就透传原始错误（如 JDBC 连不通被包成同一措辞）；裸外层措辞（引擎 REST 只回外层
// message，无 caused-by）维持既有归类，缺插件是该路径最常见的形态。
// engineMessageIndicatesMissingPlugin reports true only when the plugin is genuinely absent.
// The API-06 wrapper folds every factory-init failure into one sentence: when a caused-by chain
// exists, look for a definitive absent signal per segment — absent that, the raw error passes
// through (e.g. a JDBC connect failure wrapped in the same wording). A bare wrapper message
// (engine REST returns only the outer message, no caused-by) keeps the existing classification
// since a missing plugin is the most common shape on that path.
func engineMessageIndicatesMissingPlugin(message string) bool {
	segments := engineCausedBySplit.Split(message, -1)
	if len(segments) == 1 {
		return true
	}
	for _, segment := range segments {
		for _, signal := range enginePluginAbsentSignals {
			if signal.MatchString(segment) {
				return true
			}
		}
	}
	return false
}

// ConnectorMissingError 携带缺失连接器的类型与标识，原始引擎错误保留在 Cause 里供排查。
// ConnectorMissingError carries the missing connector's type and identifier; the raw engine error stays in Cause.
type ConnectorMissingError struct {
	// PluginType 是 source / sink / transform。 / PluginType is source / sink / transform.
	PluginType string
	// Identifier 是任务里写的插件名（如 Http、Jdbc）。 / Identifier is the plugin name used in the job (e.g. Http, Jdbc).
	Identifier string
	Cause      error
}

// Error 把可读说明放在前面，原始消息放在冒号后；前端与已存作业消息都按这段英文前缀识别。
// Error leads with a readable summary and keeps the raw message after the colon; UI and stored job messages match this English prefix.
func (e *ConnectorMissingError) Error() string {
	return fmt.Sprintf("sync: cluster is missing the %s connector %q, install the plugin on the cluster first: %v", e.PluginType, e.Identifier, e.Cause)
}

// Is 让 errors.Is(err, ErrConnectorMissing) 命中，handler 据此映射状态码与 error_code。
// Is makes errors.Is(err, ErrConnectorMissing) match so the handler can map status and error_code.
func (e *ConnectorMissingError) Is(target error) bool {
	return target == ErrConnectorMissing
}

// Unwrap 保留原始引擎错误链。 / Unwrap keeps the raw engine error chain.
func (e *ConnectorMissingError) Unwrap() error {
	return e.Cause
}

// classifyEngineSubmitError 把引擎「找不到连接器」的提交失败归类为 ConnectorMissingError，其余错误原样返回。
// 结果未知（EngineCallUnknownError）不归类：调用方要按未知结果登记待确认作业。
// classifyEngineSubmitError turns the engine's "connector not found" submit failure into ConnectorMissingError and
// returns every other error unchanged. Unknown outcomes (EngineCallUnknownError) are left alone because callers
// must record them as pending jobs.
func classifyEngineSubmitError(err error) error {
	if err == nil {
		return nil
	}
	var unknownErr *EngineCallUnknownError
	if errors.As(err, &unknownErr) || errors.Is(err, ErrConnectorMissing) {
		return err
	}
	match := engineConnectorMissingPattern.FindStringSubmatch(err.Error())
	if match == nil || !engineMessageIndicatesMissingPlugin(err.Error()) {
		return err
	}
	return &ConnectorMissingError{
		PluginType: strings.ToLower(match[1]),
		Identifier: match[2],
		Cause:      err,
	}
}

// syncErrorCode 返回需要随响应下发的 error_code；没有专门错误码时为空。
// syncErrorCode returns the error_code to send with the response; empty when no dedicated code applies.
func syncErrorCode(err error) string {
	if errors.Is(err, ErrConnectorMissing) {
		return ErrorCodeConnectorMissing
	}
	return ""
}
