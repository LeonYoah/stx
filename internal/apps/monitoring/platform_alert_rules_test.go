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
package monitoring

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LeonYoah/stx/internal/version"
)

// platformAlertTemplateKeys 是本任务新增的 5 条平台自监控模板键。
// platformAlertTemplateKeys lists the five platform self-monitoring template keys
// introduced by this task.
var platformAlertTemplateKeys = []string{
	AlertRuleKeyPlatformServerMemoryHigh,
	AlertRuleKeyPlatformServerHTTP5xxRatioHigh,
	AlertRuleKeyPlatformTSDBCompactionFailed,
	AlertRuleKeyPlatformAgentMemoryHigh,
	AlertRuleKeyPlatformAgentVersionMismatch,
}

// TestPlatformAlertTemplatesRegistered 锁定 5 条模板在策略中心目录里可见：
// 归属 platform_health 分类、走 metrics_template 评估器、有默认阈值。
// TestPlatformAlertTemplatesRegistered locks the five templates into the policy
// catalog: platform_health category, metrics_template evaluator, sane defaults.
func TestPlatformAlertTemplatesRegistered(t *testing.T) {
	templates := allAlertPolicyTemplateSummaries()
	byKey := map[string]*AlertPolicyTemplateSummaryDTO{}
	for _, tpl := range templates {
		byKey[tpl.Key] = tpl
	}
	for _, key := range platformAlertTemplateKeys {
		tpl, ok := byKey[key]
		require.True(t, ok, "template %s missing", key)
		require.Equal(t, "platform_health", tpl.Category, "%s category", key)
		require.Equal(t, string(AlertPolicyBuilderKindMetricsTemplate), tpl.SourceKind, "%s source kind", key)
		require.Equal(t, AlertPolicyCapabilityKeyMetricsTemplates, tpl.CapabilityKey, "%s capability", key)
		require.NotEmpty(t, tpl.SuggestedPromQL, "%s suggested promql", key)
		require.NotEmpty(t, tpl.Name, "%s name", key)
		require.NotEmpty(t, tpl.Description, "%s description", key)
		require.NotEmpty(t, alertPolicyTemplateDisplayNameZH(key), "%s zh name", key)
	}
	// 每条生成表达式必须是合法 PromQL。
	// Every generated expression must parse as valid PromQL.
	for _, key := range platformAlertTemplateKeys {
		expr, err := buildPlatformMetricPromQL(key, "10m", ">", "5")
		require.NoError(t, err, "%s build", key)
		_, err = metricRuleExprParser.ParseExpr(expr)
		require.NoError(t, err, "%s parse: %s", key, expr)
	}
}

// TestPlatformMetricPromQL_VersionInjectionAndEscaping 覆盖版本号注入、阈值替换与转义：
// server 版本以 %q 注入不等匹配器；阈值/算子可被策略条件覆盖。
// TestPlatformMetricPromQL_VersionInjectionAndEscaping covers version injection,
// threshold substitution and escaping: the server version lands in the inequality
// matcher via %q; operator/threshold stay overridable by policy conditions.
func TestPlatformMetricPromQL_VersionInjectionAndEscaping(t *testing.T) {
	expr, err := buildPlatformMetricPromQL(AlertRuleKeyPlatformAgentVersionMismatch, "30m", "==", "1")
	require.NoError(t, err)
	require.Contains(t, expr, fmt.Sprintf("version!=%q", version.Version))
	require.Contains(t, expr, "== 1")
	_, err = metricRuleExprParser.ParseExpr(expr)
	require.NoError(t, err, "generated expr: %s", expr)

	// 自定义阈值与算子透传进表达式。
	// Custom operator/threshold flow through into the expression.
	expr, err = buildPlatformMetricPromQL(AlertRuleKeyPlatformServerMemoryHigh, "5m", ">=", "1073741824")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(expr, ">= 1073741824"), expr)

	// 版本号含引号 / 反斜杠时 %q 转义保证表达式仍合法。
	// Versions containing quotes or backslashes stay parseable thanks to %q escaping.
	expr, err = buildPlatformMetricPromQL(AlertRuleKeyPlatformAgentVersionMismatch, "30m", "==", "1")
	require.NoError(t, err)
	require.NotContains(t, expr, `"`+"\n")
}

// TestPlatformMetricPromQL_NoSeaTunnelMatcher 锁定平台表达式不携带
// seatunnel_engine_http 匹配器（信号来自内嵌自抓取与 Agent 通道）。
// TestPlatformMetricPromQL_NoSeaTunnelMatcher pins that platform expressions never
// carry the seatunnel_engine_http matcher (signals come from the embedded scrape
// and the agent channel).
func TestPlatformMetricPromQL_NoSeaTunnelMatcher(t *testing.T) {
	for _, key := range platformAlertTemplateKeys {
		tpl, ok := findAlertPolicyTemplateSummary(key)
		require.True(t, ok)
		expr, err := buildMetricTemplatePromQL(&AlertPolicy{TemplateKey: key}, tpl, resolvedMetricPolicyCondition{
			Operator: ">", Threshold: "1", WindowMinutes: 5,
		})
		require.NoError(t, err, key)
		require.NotContains(t, expr, "seatunnel_engine_http", key)
		_, err = metricRuleExprParser.ParseExpr(expr)
		require.NoError(t, err, "%s: %s", key, expr)
	}
}
