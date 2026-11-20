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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestClusterRuleCatalogKeys 断言目录恰好 21 条规则且键唯一。
// TestClusterRuleCatalogKeys asserts the catalog holds exactly 21 unique rule keys.
func TestClusterRuleCatalogKeys(t *testing.T) {
	require.Len(t, clusterRuleCatalog, 21)
	seen := map[string]struct{}{}
	for _, def := range clusterRuleCatalog {
		require.NotNil(t, def)
		require.NotEmpty(t, def.Key)
		_, dup := seen[def.Key]
		require.False(t, dup, "duplicate catalog key %q", def.Key)
		seen[def.Key] = struct{}{}
		require.NotNil(t, lookupClusterRuleDefinition(def.Key))
	}
}

// TestClusterRuleCatalogDefaultEnabled 断言默认开关清单与 PRD D4 一致（13 开 8 关）。
// TestClusterRuleCatalogDefaultEnabled asserts the default-enabled set matches PRD D4
// (13 on / 8 off).
func TestClusterRuleCatalogDefaultEnabled(t *testing.T) {
	enabled := map[string]bool{}
	for _, def := range clusterRuleCatalog {
		enabled[def.Key] = def.DefaultEnabled
	}
	wantOn := []string{
		AlertRuleKeyNodeOffline, "master_unavailable", "worker_insufficient", "agent_offline",
		AlertRuleKeyProcessCrashed, AlertRuleKeyProcessRestartFailed, AlertRuleKeyProcessRestartLimitReached,
		"sync_status_unavailable", "split_brain_risk", "deadlocked_threads_detected",
		"job_thread_pool_rejection_high", "memory_usage_high", "fd_usage_high",
	}
	wantOff := []string{
		"cpu_usage_high", "gc_time_high", "job_thread_pool_queue_backlog_high",
		"coordinator_backlog_growing", "failed_jobs_high", "sync_event_channel_degraded",
		AlertRuleKeyClusterRestartRequested, AlertRuleKeyNodeStopRequested,
	}
	for _, key := range wantOn {
		require.True(t, enabled[key], "%s should default on per D4", key)
	}
	for _, key := range wantOff {
		require.False(t, enabled[key], "%s should default off per D4", key)
	}
}

// TestClusterRuleCatalogThresholdsAndDurations 抽查 D6 的默认阈值与持续时间（重复内置规则沿用原值）。
// TestClusterRuleCatalogThresholdsAndDurations spot-checks the D6 default thresholds and
// durations (duplicated builtin rules keep their original values).
func TestClusterRuleCatalogThresholdsAndDurations(t *testing.T) {
	cases := []struct {
		key       string
		operator  string
		unit      string
		threshold float64
		forDur    time.Duration
	}{
		{"memory_usage_high", ">", clusterRuleThresholdUnitPercent, 85, 10 * time.Minute},
		{"fd_usage_high", ">", clusterRuleThresholdUnitPercent, 80, 10 * time.Minute},
		{"cpu_usage_high", ">", clusterRuleThresholdUnitPercent, 80, 10 * time.Minute},
		{"gc_time_high", ">", clusterRuleThresholdUnitPercent, 20, 10 * time.Minute},
		{"job_thread_pool_queue_backlog_high", ">", clusterRuleThresholdUnitCount, 100, 5 * time.Minute},
		{"coordinator_backlog_growing", ">", clusterRuleThresholdUnitCount, 50, 10 * time.Minute},
		{"deadlocked_threads_detected", ">", clusterRuleThresholdUnitCount, 0, time.Minute},
		{"job_thread_pool_rejection_high", ">", clusterRuleThresholdUnitCount, 0, 5 * time.Minute},
		{"failed_jobs_high", ">", clusterRuleThresholdUnitCount, 0, time.Minute},
	}
	for _, c := range cases {
		def := lookupClusterRuleDefinition(c.key)
		require.NotNil(t, def, "missing catalog key %q", c.key)
		require.NotNil(t, def.Threshold, "%s should define a threshold", c.key)
		require.Equal(t, c.operator, def.Threshold.Operator, c.key)
		require.Equal(t, c.unit, def.Threshold.Unit, c.key)
		require.InDelta(t, c.threshold, def.Threshold.Default, 1e-9, c.key)
		require.Equal(t, c.forDur, def.DefaultFor, c.key)
	}
	// 无阈值的规则保持 nil。
	// Threshold-less rules stay nil.
	require.Nil(t, lookupClusterRuleDefinition("split_brain_risk").Threshold)
	require.Nil(t, lookupClusterRuleDefinition("sync_status_unavailable").Threshold)
}

// TestClusterRuleCatalogSeverityDefaultsWarning 断言全部规则默认 warning（D9）。
// TestClusterRuleCatalogSeverityDefaultsWarning asserts every rule defaults to warning (D9).
func TestClusterRuleCatalogSeverityDefaultsWarning(t *testing.T) {
	for _, def := range clusterRuleCatalog {
		require.Equal(t, AlertSeverityWarning, def.Severity, "%s should default to warning", def.Key)
	}
}

// TestClusterRuleCatalogTSDBExprsParse 断言每条 tsdb 规则在全局与集群匹配器下的表达式都可被
// promql 解析，且集群匹配器被注入。
// TestClusterRuleCatalogTSDBExprsParse asserts every tsdb rule's expression parses under both
// the global and a cluster matcher, with the matcher injected.
func TestClusterRuleCatalogTSDBExprsParse(t *testing.T) {
	for _, def := range clusterRuleCatalog {
		if def.Engine != clusterRuleEngineTSDB {
			continue
		}
		require.NotNil(t, def.Expr, "%s must define an expr builder", def.Key)
		cfg := &resolvedRuleConfig{RuleKey: def.Key, Definition: def, Enabled: true}
		for _, matcher := range []string{"", `cluster_id="9"`} {
			expr := def.Expr(cfg, matcher)
			require.NotEmpty(t, expr, "%s produced an empty expr", def.Key)
			_, err := metricRuleExprParser.ParseExpr(expr)
			require.NoError(t, err, "%s expr must parse: %s", def.Key, expr)
			if matcher != "" {
				require.Contains(t, expr, `cluster_id="9"`, "%s expr must carry the cluster matcher", def.Key)
			}
		}
	}
}

// TestResolveClusterRule_fieldInheritance 断言目录默认 → 全局配置 → 集群例外逐字段合并。
// TestResolveClusterRule_fieldInheritance asserts per-field layering of catalog default →
// global config → cluster override.
func TestResolveClusterRule_fieldInheritance(t *testing.T) {
	service, _, _, ctx := setupMonitoringAlertPolicyService(t)

	base, err := service.ResolveClusterRule(ctx, "cpu_usage_high", 0)
	require.NoError(t, err)
	require.NotNil(t, base)
	require.False(t, base.Enabled, "cpu_usage_high defaults off")
	require.InDelta(t, 80.0, *base.Threshold, 1e-9)
	require.Equal(t, 600, base.ForSeconds)

	// 全局层改阈值与持续时间，例外层只改持续时间：集群解析应拿到全局阈值 + 例外时长。
	// The global layer changes threshold and duration, the override only duration: cluster
	// resolution must see the global threshold plus the override duration.
	threshold := 95.0
	forSeconds := 300
	_, err = service.SaveClusterRuleConfig(ctx, "cpu_usage_high", &clusterRuleFields{
		Threshold:  &threshold,
		ForSeconds: &forSeconds,
	})
	require.NoError(t, err)
	_, err = service.SaveClusterRuleOverride(ctx, "cpu_usage_high", 7, &clusterRuleFields{
		ForSeconds: ptrInt(60),
	})
	require.NoError(t, err)

	global, err := service.ResolveClusterRule(ctx, "cpu_usage_high", 0)
	require.NoError(t, err)
	require.InDelta(t, 95.0, *global.Threshold, 1e-9)
	require.Equal(t, 300, global.ForSeconds)

	overridden, err := service.ResolveClusterRule(ctx, "cpu_usage_high", 7)
	require.NoError(t, err)
	require.InDelta(t, 95.0, *overridden.Threshold, 1e-9, "override inherits the global threshold")
	require.Equal(t, 60, overridden.ForSeconds, "override field wins")

	other, err := service.ResolveClusterRule(ctx, "cpu_usage_high", 8)
	require.NoError(t, err)
	require.Equal(t, 300, other.ForSeconds, "other clusters stay on the global config")
}

// TestResolveClusterRule_nilReset 断言字段写回 nil / 删除例外后回落到上一层。
// TestResolveClusterRule_nilReset asserts fields written back to nil (or the override deleted)
// fall back to the layer above.
func TestResolveClusterRule_nilReset(t *testing.T) {
	service, _, _, ctx := setupMonitoringAlertPolicyService(t)

	enabled := true
	_, err := service.SaveClusterRuleOverride(ctx, "cpu_usage_high", 9, &clusterRuleFields{
		Enabled: &enabled,
	})
	require.NoError(t, err)
	resolved, err := service.ResolveClusterRule(ctx, "cpu_usage_high", 9)
	require.NoError(t, err)
	require.True(t, resolved.Enabled)

	// 例外行整行覆盖保存：Enabled 写回 nil 即恢复继承目录默认（关闭）。
	// Overrides upsert whole rows: writing Enabled back to nil restores inheritance (off).
	_, err = service.SaveClusterRuleOverride(ctx, "cpu_usage_high", 9, &clusterRuleFields{})
	require.NoError(t, err)
	resolved, err = service.ResolveClusterRule(ctx, "cpu_usage_high", 9)
	require.NoError(t, err)
	require.False(t, resolved.Enabled)

	require.NoError(t, service.DeleteClusterRuleOverride(ctx, "cpu_usage_high", 9))
	resolved, err = service.ResolveClusterRule(ctx, "cpu_usage_high", 9)
	require.NoError(t, err)
	require.False(t, resolved.Enabled)
}

// TestResolveClusterRule_notifyModes 覆盖 default / custom / none 与空默认通知的生效渠道解析。
// TestResolveClusterRule_notifyModes covers the effective channels of the default / custom /
// none modes plus an empty default notification.
func TestResolveClusterRule_notifyModes(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)

	channel := &NotificationChannel{Name: "ops", Type: NotificationChannelTypeWebhook, Enabled: true, Endpoint: "http://example.com"}
	require.NoError(t, repo.CreateNotificationChannel(ctx, channel))

	// 无默认通知、未自定义：生效渠道为空。
	// No default notification and nothing custom: effective channels are empty.
	resolved, err := service.ResolveClusterRule(ctx, AlertRuleKeyNodeOffline, 0)
	require.NoError(t, err)
	require.Empty(t, resolved.ChannelIDs)
	require.False(t, resolved.deliversExternally())

	// 默认通知配了渠道：default 模式规则沿用。
	// With a default notification configured, default-mode rules inherit its channels.
	_, err = service.SaveAlertNotificationDefaults(ctx, []uint{channel.ID}, nil, 10, true)
	require.NoError(t, err)
	resolved, err = service.ResolveClusterRule(ctx, AlertRuleKeyNodeOffline, 0)
	require.NoError(t, err)
	require.Equal(t, []uint{channel.ID}, resolved.ChannelIDs)
	require.Equal(t, 10, resolved.CooldownMinutes)
	require.True(t, resolved.SendRecovery)

	// none：显式关闭通知，渠道清空。
	// none: notification explicitly off, channels cleared.
	_, err = service.SaveClusterRuleConfig(ctx, AlertRuleKeyNodeOffline, &clusterRuleFields{
		NotifyMode: clusterRuleNotifyModeNone,
	})
	require.NoError(t, err)
	resolved, err = service.ResolveClusterRule(ctx, AlertRuleKeyNodeOffline, 0)
	require.NoError(t, err)
	require.Empty(t, resolved.ChannelIDs)

	// custom：规则自己的渠道接管，不再读默认通知。
	// custom: the rule's own channels take over instead of the default notification.
	other := &NotificationChannel{Name: "oncall", Type: NotificationChannelTypeWebhook, Enabled: true, Endpoint: "http://oncall.example.com"}
	require.NoError(t, repo.CreateNotificationChannel(ctx, other))
	idsJSON, err := marshalAlertPolicyChannelIDs([]uint{other.ID})
	require.NoError(t, err)
	critical := string(AlertSeverityCritical)
	_, err = service.SaveClusterRuleConfig(ctx, AlertRuleKeyNodeOffline, &clusterRuleFields{
		Severity:       &critical,
		NotifyMode:     clusterRuleNotifyModeCustom,
		ChannelIDsJSON: idsJSON,
	})
	require.NoError(t, err)
	resolved, err = service.ResolveClusterRule(ctx, AlertRuleKeyNodeOffline, 0)
	require.NoError(t, err)
	require.Equal(t, []uint{other.ID}, resolved.ChannelIDs)
	require.True(t, resolved.deliversExternally())

	// 例外层通知方式成组接管：集群例外用 none 时渠道清空，其余字段仍继承全局层
	// （critical+none 会被保存校验拒绝，这里用默认 warning 的 fd_usage_high 验证接管语义）。
	// The override's notify mode takes over as a unit: a none override clears channels while
	// other fields still inherit the global layer (critical+none is rejected at save time, so
	// the takeover semantics are checked on the default-warning fd_usage_high).
	fdIDs, err := marshalAlertPolicyChannelIDs([]uint{channel.ID})
	require.NoError(t, err)
	_, err = service.SaveClusterRuleConfig(ctx, "fd_usage_high", &clusterRuleFields{
		NotifyMode:     clusterRuleNotifyModeCustom,
		ChannelIDsJSON: fdIDs,
	})
	require.NoError(t, err)
	_, err = service.SaveClusterRuleOverride(ctx, "fd_usage_high", 3, &clusterRuleFields{
		NotifyMode: clusterRuleNotifyModeNone,
	})
	require.NoError(t, err)
	resolved, err = service.ResolveClusterRule(ctx, "fd_usage_high", 3)
	require.NoError(t, err)
	require.Empty(t, resolved.ChannelIDs)
	resolved, err = service.ResolveClusterRule(ctx, "fd_usage_high", 4)
	require.NoError(t, err)
	require.Equal(t, []uint{channel.ID}, resolved.ChannelIDs, "clusters without an override keep the global custom channels")
}

// TestSaveClusterRuleConfig_criticalRequiresChannel 断言 critical 规则无启用渠道时保存被拒
// （含例外层）。
// TestSaveClusterRuleConfig_criticalRequiresChannel asserts saving a critical rule without an
// enabled channel is rejected (the override layer included).
func TestSaveClusterRuleConfig_criticalRequiresChannel(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)

	critical := string(AlertSeverityCritical)
	_, err := service.SaveClusterRuleConfig(ctx, AlertRuleKeyNodeOffline, &clusterRuleFields{
		Severity: &critical,
	})
	require.ErrorIs(t, err, ErrClusterRuleCriticalChannelRequired, "global layer critical without channels must be rejected")

	_, err = service.SaveClusterRuleOverride(ctx, AlertRuleKeyNodeOffline, 5, &clusterRuleFields{
		Severity: &critical,
	})
	require.ErrorIs(t, err, ErrClusterRuleCriticalChannelRequired, "override layer critical without channels must be rejected")

	// 生效通知来自例外层渠道时同样校验：选了停用渠道即拒。
	// Effective notification from the override's channels is validated the same way: a disabled
	// channel is rejected.
	disabled := &NotificationChannel{Name: "off", Type: NotificationChannelTypeWebhook, Enabled: false, Endpoint: "http://off.example.com"}
	require.NoError(t, repo.CreateNotificationChannel(ctx, disabled))
	idsJSON, err := marshalAlertPolicyChannelIDs([]uint{disabled.ID})
	require.NoError(t, err)
	_, err = service.SaveClusterRuleOverride(ctx, AlertRuleKeyNodeOffline, 5, &clusterRuleFields{
		Severity:       &critical,
		NotifyMode:     clusterRuleNotifyModeCustom,
		ChannelIDsJSON: idsJSON,
	})
	require.ErrorIs(t, err, ErrClusterRuleChannelDisabled)
}

// TestSaveClusterRuleConfig_globalCriticalChecksOverrides 断言保存全局配置为 critical 时，
// 每个已有集群例外都按叠加后的生效结果复算 critical 不变式，任一不满足即拒绝且报错带集群 ID。
// TestSaveClusterRuleConfig_globalCriticalChecksOverrides asserts a global save to critical
// re-resolves every stored override's effective config against the critical invariant; any
// violation rejects the whole write with the cluster id in the error.
func TestSaveClusterRuleConfig_globalCriticalChecksOverrides(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)

	channel := &NotificationChannel{Name: "ops", Type: NotificationChannelTypeWebhook, Enabled: true, Endpoint: "http://example.com"}
	require.NoError(t, repo.CreateNotificationChannel(ctx, channel))
	idsJSON, err := marshalAlertPolicyChannelIDs([]uint{channel.ID})
	require.NoError(t, err)

	enabled := true
	// 集群 11 的例外固定 none 通知、severity 继承全局层：全局改 critical 后它也变 critical，
	// 而生效通知为 none → 无渠道，违反 critical 不变式。
	// The cluster-11 override pins notify mode none and inherits severity: a global flip to
	// critical makes it critical too, with no channels — a critical-invariant violation.
	_, err = service.SaveClusterRuleOverride(ctx, AlertRuleKeyNodeOffline, 11, &clusterRuleFields{
		Enabled:    &enabled,
		NotifyMode: clusterRuleNotifyModeNone,
	})
	require.NoError(t, err)

	critical := string(AlertSeverityCritical)
	warning := string(AlertSeverityWarning)
	_, err = service.SaveClusterRuleConfig(ctx, AlertRuleKeyNodeOffline, &clusterRuleFields{
		Severity:       &critical,
		NotifyMode:     clusterRuleNotifyModeCustom,
		ChannelIDsJSON: idsJSON,
	})
	require.ErrorIs(t, err, ErrClusterRuleCriticalChannelRequired)
	require.Contains(t, err.Error(), "cluster_id=11", "error must name the violating override cluster")

	// 例外固定 warning 后不再继承 critical，全局保存放行。
	// Once the override pins warning it no longer inherits critical, and the global save passes.
	_, err = service.SaveClusterRuleOverride(ctx, AlertRuleKeyNodeOffline, 11, &clusterRuleFields{
		Enabled:  &enabled,
		Severity: &warning,
	})
	require.NoError(t, err)
	_, err = service.SaveClusterRuleConfig(ctx, AlertRuleKeyNodeOffline, &clusterRuleFields{
		Severity:       &critical,
		NotifyMode:     clusterRuleNotifyModeCustom,
		ChannelIDsJSON: idsJSON,
	})
	require.NoError(t, err)
}

// TestBuildManagedRuleGroups_clusterOverride 断言 tsdb 规则组的全局 rule 用
// cluster_id!~"…" 排掉例外集群，例外集群按 cluster_id="…" 单独生成。
// TestBuildManagedRuleGroups_clusterOverride asserts the managed group's global rule excludes
// override clusters via cluster_id!~"…" while each override cluster gets its own cluster_id="…"
// rule.
func TestBuildManagedRuleGroups_clusterOverride(t *testing.T) {
	service, _, _, ctx := setupMonitoringAlertPolicyService(t)

	enabled := true
	_, err := service.SaveClusterRuleConfig(ctx, "memory_usage_high", &clusterRuleFields{Enabled: &enabled})
	require.NoError(t, err)
	_, err = service.SaveClusterRuleOverride(ctx, "memory_usage_high", 9, &clusterRuleFields{Enabled: &enabled})
	require.NoError(t, err)

	evaluator, err := NewMetricEvaluator(service, MetricEvaluatorOptions{Interval: time.Second})
	require.NoError(t, err)
	groups, err := evaluator.buildManagedRuleGroups(ctx)
	require.NoError(t, err)
	require.Len(t, groups.Groups, 1)

	var globals, exceptions []string
	for _, rule := range groups.Groups[0].Rules {
		require.Equal(t, "stx", rule.Labels["managed_by"])
		require.NotEmpty(t, rule.Labels["rule_key"])
		_, err := metricRuleExprParser.ParseExpr(rule.Expr)
		require.NoError(t, err, "generated expr must parse: %s", rule.Expr)
		switch {
		case strings.Contains(rule.Expr, `cluster_id!~`):
			globals = append(globals, rule.Expr)
		case strings.Contains(rule.Expr, `cluster_id="9"`):
			exceptions = append(exceptions, rule.Expr)
		}
	}
	require.NotEmpty(t, globals, "expected a global rule for memory_usage_high")
	require.True(t, strings.Contains(globals[0], `cluster_id!~"9"`), "global rule must exclude override clusters: %s", globals[0])
	require.NotEmpty(t, exceptions, "expected a per-cluster rule for the override")
	require.True(t, strings.Contains(exceptions[0], `cluster_id="9"`), "override rule must pin cluster_id: %s", exceptions[0])

	// 默认关闭的规则不产出任何 rule。
	// Rules disabled by default produce no rules.
	for _, rule := range groups.Groups[0].Rules {
		require.NotEqual(t, "cpu_usage_high", rule.Labels["rule_key"])
	}
}

func ptrInt(value int) *int {
	return &value
}
