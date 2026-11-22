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

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// PrecheckSubCommandCheckEngineHealth 探测 SeaTunnel 引擎业务就绪（而非仅进程存活）。
// PrecheckSubCommandCheckEngineHealth probes SeaTunnel engine business readiness (not mere process liveness).
const PrecheckSubCommandCheckEngineHealth PrecheckSubCommand = "check_engine_health"

// engineHealthReasonMaxLen 限制回传的引擎错误原因长度，避免超长堆栈淹没诊断输出。
// engineHealthReasonMaxLen bounds the returned engine error reason so huge stack traces do not flood diagnostics.
const engineHealthReasonMaxLen = 512

// engineHealthHTTPClient 用于访问本机 Hazelcast REST；短超时，测试可替换。
// engineHealthHTTPClient reaches the local Hazelcast REST API with a short timeout; tests may replace it.
var engineHealthHTTPClient = &http.Client{Timeout: 3 * time.Second}

// engineHealthResult 是一次引擎就绪探测的结论。
// engineHealthResult is the outcome of one engine readiness probe.
type engineHealthResult struct {
	// known 表示拿到了可判定的 HTTP 应答；网络不可达 / 404（旧版无 REST）时为 false，绝不误报。
	// known means a decidable HTTP answer was obtained; it is false on unreachable network or 404
	// (older versions without REST), never producing false alarms.
	known   bool
	healthy bool
	reason  string
}

// handleCheckEngineHealth 请求 http://<host>:<port>/hazelcast/rest/maps/overview：
// 200 且响应不含 "status":"fail" → 引擎就绪；明确 5xx 或 status=fail → 半初始化/僵尸 JVM（known=true 但不健康）；
// 网络错误或 404 → known=false（不可判定）。host 依次尝试 127.0.0.1 与参数 host。
// handleCheckEngineHealth queries http://<host>:<port>/hazelcast/rest/maps/overview: HTTP 200 without a
// "status":"fail" body means the engine is ready; an explicit 5xx or a status=fail payload means a
// half-initialized / zombie JVM (known=true but unhealthy); network errors or 404 mean unknown (known=false).
// It tries 127.0.0.1 first and then the "host" parameter.
func handleCheckEngineHealth(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	port, err := strconv.Atoi(strings.TrimSpace(params["port"]))
	if err != nil || port <= 0 || port > 65535 {
		return &PrecheckResult{Success: false, Message: fmt.Sprintf("invalid port: %q", params["port"])}, nil
	}
	hosts := []string{"127.0.0.1"}
	if host := strings.TrimSpace(params["host"]); host != "" && host != "127.0.0.1" {
		hosts = append(hosts, host)
	}

	result := engineHealthResult{}
	for _, host := range hosts {
		res := probeEngineReadiness(ctx, net.JoinHostPort(host, strconv.Itoa(port)))
		if res.known {
			result = res
			break
		}
		// 未判定（网络错误 / 404）时继续尝试下一个候选地址。
		// Undecidable answers (network error / 404) fall through to the next candidate address.
		result = res
	}

	details := map[string]string{
		"known":   strconv.FormatBool(result.known),
		"healthy": strconv.FormatBool(result.healthy),
	}
	if result.reason != "" {
		details["reason"] = result.reason
	}

	switch {
	case result.known && result.healthy:
		return &PrecheckResult{
			Success: true,
			Message: "SeaTunnel engine is ready / SeaTunnel 引擎已就绪",
			Details: details,
		}, nil
	case result.known:
		return &PrecheckResult{
			Success: false,
			Message: fmt.Sprintf("SeaTunnel engine is not ready: %s / SeaTunnel 引擎未就绪：%s", result.reason, result.reason),
			Details: details,
		}, nil
	default:
		return &PrecheckResult{
			Success: false,
			Message: fmt.Sprintf("engine readiness unknown: %s / 引擎就绪状态未知：%s", result.reason, result.reason),
			Details: details,
		}, nil
	}
}

// probeEngineReadiness 访问一次 /hazelcast/rest/maps/overview 并按响应分类。
// probeEngineReadiness hits /hazelcast/rest/maps/overview once and classifies the response.
func probeEngineReadiness(ctx context.Context, hostPort string) engineHealthResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+hostPort+"/hazelcast/rest/maps/overview", nil)
	if err != nil {
		return engineHealthResult{reason: err.Error()}
	}
	resp, err := engineHealthHTTPClient.Do(req)
	if err != nil {
		return engineHealthResult{reason: err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return engineHealthResult{reason: err.Error()}
	}

	failReason := engineFailReason(body)
	switch {
	case resp.StatusCode >= 500:
		// 半初始化引擎在 maps/overview 上明确返回 5xx（如 coordinatorService is null）。
		// A half-initialized engine answers 5xx on maps/overview (e.g. coordinatorService is null).
		return engineHealthResult{known: true, healthy: false, reason: firstNonEmptyEngineReason(failReason, fmt.Sprintf("HTTP %d", resp.StatusCode))}
	case resp.StatusCode == http.StatusOK:
		if failReason != "" {
			return engineHealthResult{known: true, healthy: false, reason: failReason}
		}
		return engineHealthResult{known: true, healthy: true}
	default:
		// 404（旧版无 REST 或端点缺失）等其他状态码一律按未知处理，不误报。
		// Any other status such as 404 (older versions without REST) is treated as unknown, never a false alarm.
		return engineHealthResult{known: false, reason: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}
}

// engineFailReason 提取响应体里的 Hazelcast 失败原因：{"status":"fail","message":"..."}。
// engineFailReason extracts the Hazelcast failure reason from a {"status":"fail","message":"..."} body.
func engineFailReason(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || !strings.Contains(trimmed, `"status"`) {
		return ""
	}
	var payload struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Status), "fail") {
		return ""
	}
	return truncateEngineReason(firstNonEmptyEngineReason(strings.TrimSpace(payload.Message), "status=fail"))
}

func firstNonEmptyEngineReason(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func truncateEngineReason(reason string) string {
	if len(reason) <= engineHealthReasonMaxLen {
		return reason
	}
	return reason[:engineHealthReasonMaxLen] + "..."
}
