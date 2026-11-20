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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/rules"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/LeonYoah/stx/internal/pkg/embedts"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// setupMetricEvaluatorTestDB 建立覆盖远程告警/策略/投递全链表的 sqlite 测试库。
// setupMetricEvaluatorTestDB builds a sqlite test DB covering remote alerts, policies and the
// delivery tables end to end.
func setupMetricEvaluatorTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "metric_evaluator_test_*")
	require.NoError(t, err)

	database, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, database.AutoMigrate(
		&AlertPolicy{},
		&AlertState{},
		&AlertRule{},
		&NotificationChannel{},
		&NotificationDelivery{},
		&NotificationRoute{},
		&ClusterRuleConfig{},
		&ClusterRuleOverride{},
		&AlertNotificationDefaults{},
		&RemoteAlertRecord{},
	))

	cleanup := func() {
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.RemoveAll(tempDir)
	}
	return database, cleanup
}

// startMetricEvaluatorTestEmbedTS 启动指向 t.TempDir() 的内嵌 tsdb 单例。
// startMetricEvaluatorTestEmbedTS boots the embedded tsdb singleton rooted at t.TempDir().
func startMetricEvaluatorTestEmbedTS(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, embedts.Start(ctx, embedts.Options{
		Dir:               t.TempDir(),
		RetentionDuration: 24 * time.Hour,
		QueryTimeout:      5 * time.Second,
		MaxSamples:        1000,
	}))
	// t.Cleanup 后进先出：先 Close 再让 TempDir 删目录。
	// Cleanups run LIFO: Close happens before the TempDir removal.
	t.Cleanup(func() { _ = embedts.Close() })
	require.True(t, embedts.Ready())
}

// appendEvalSample 向内嵌 tsdb 追加一个样本。
// appendEvalSample appends one sample into the embedded tsdb.
func appendEvalSample(t *testing.T, ctx context.Context, metric string, extra map[string]string, ts time.Time, v float64) {
	t.Helper()
	app := embedts.Appender(ctx)
	require.NotNil(t, app)
	pairs := []string{labels.MetricName, metric}
	for k, val := range extra {
		pairs = append(pairs, k, val)
	}
	_, err := app.Append(0, labels.FromStrings(pairs...), ts.UnixMilli(), v)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
}

// waitForRemoteAlertRecord 轮询直到满足条件的远程告警记录出现或超时。
// waitForRemoteAlertRecord polls until a remote alert record matching want appears or times out.
func waitForRemoteAlertRecord(t *testing.T, ctx context.Context, repo *Repository, want func(*RemoteAlertRecord) bool) *RemoteAlertRecord {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var records []RemoteAlertRecord
		require.NoError(t, repo.db.WithContext(ctx).Order("id ASC").Find(&records).Error)
		for i := range records {
			if want(&records[i]) {
				return &records[i]
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	return nil
}

// TestMetricEvaluatorFiringThenResolved 端到端跑通「内存 tsdb → rules.Manager 评估 →
// for 门控 → NotifyFunc → 既有投递缝」：短 for 先验证 pending 期间不出 firing 记录，
// 触发后断言 managed 策略告警走 deliverManagedRemoteAlertPolicyNotifications（MatchCount+1），
// 解除后断言同指纹同 starts_at 的记录翻转为 resolved。
// TestMetricEvaluatorFiringThenResolved drives the whole "in-memory tsdb → rules.Manager eval →
// 'for' gate → NotifyFunc → existing delivery seam" path: a short 'for' first proves no firing
// record appears while pending, the firing transition then asserts managed policy alerts take
// deliverManagedRemoteAlertPolicyNotifications (MatchCount+1), and after recovery the record
// with the same fingerprint + starts_at flips to resolved.
func TestMetricEvaluatorFiringThenResolved(t *testing.T) {
	ctx := context.Background()
	database, cleanup := setupMetricEvaluatorTestDB(t)
	t.Cleanup(cleanup)
	startMetricEvaluatorTestEmbedTS(t, ctx)

	repo := NewRepository(database)
	service := NewService(nil, nil, repo)

	// 准备一条启用中的指标策略：NotifyFunc 生成的记录带 managed_by/policy_id 标签时应被
	// managed 投递缝认领并累计执行状态。
	// Seed an enabled metric policy: records produced by NotifyFunc with managed_by/policy_id
	// labels must be claimed by the managed delivery seam and accumulate execution state.
	severity := string(AlertSeverityCritical)
	require.NoError(t, repo.UpsertClusterRuleConfig(ctx, &ClusterRuleConfig{
		RuleKey:  "cpu_usage_high",
		Enabled:  ptrBool(true),
		Severity: &severity,
	}))

	const testSource = "stx-test"
	evaluator, err := NewMetricEvaluator(service, MetricEvaluatorOptions{
		Interval:    20 * time.Millisecond,
		ResendDelay: time.Hour, // 拉长重推间隔，保证测试里 firing/resolved 各只推一次 / keep resends out of the test window
	})
	require.NoError(t, err)

	// 注入自定义规则组（for=120ms 短门控），sources 只留测试 source，避免 managed 组重建覆盖。
	// Inject a custom rule group (short for=120ms gate) and leave only the test source so the
	// managed rebuild does not overwrite it.
	evaluator.sources = []string{testSource}
	evaluator.loader.setGroups(testSource, &rulefmt.RuleGroups{Groups: []rulefmt.RuleGroup{{
		Name: "evaluator-test",
		Rules: []rulefmt.Rule{{
			Alert: "STXEvalTestAlert",
			Expr:  `stx_eval_test_metric > 5`,
			For:   model.Duration(120 * time.Millisecond),
			Labels: map[string]string{
				"managed_by": "stx",
				"rule_key":   "cpu_usage_high",
				"severity":   "warning",
			},
			Annotations: map[string]string{
				"rule_name":   "CPU 高负载",
				"summary":     "evaluator test firing",
				"description": "evaluator test description",
			},
		}},
	}}})

	appendEvalSample(t, ctx, "stx_eval_test_metric", map[string]string{
		"cluster_id": "1", "cluster_name": "prod", "env": "prod", "instance": "i1",
	}, time.Now(), 10)

	startedAt := time.Now()
	require.NoError(t, evaluator.Start(ctx))
	t.Cleanup(evaluator.Stop)

	firing := waitForRemoteAlertRecord(t, ctx, repo, func(r *RemoteAlertRecord) bool {
		return r.Status == "firing"
	})
	require.NotNil(t, firing, "firing alert record should appear after the for gate")
	// for=120ms 门控：firing 记录不早于 ~100ms 出现（给 CI 留出余量）。
	// for=120ms gate: the firing record cannot appear earlier than ~100ms (CI margin included).
	require.GreaterOrEqual(t, time.Since(startedAt), 80*time.Millisecond, "alert fired before the for gate elapsed")

	// AlertName 按统一告警记录语义取 rule_name → alertname。
	// AlertName resolves rule_name → alertname per the unified alert-record semantics.
	require.Equal(t, "CPU 高负载", firing.AlertName)
	require.Equal(t, "warning", firing.Severity)
	require.Equal(t, "1", firing.ClusterID)
	require.Equal(t, "prod", firing.ClusterName)
	require.Equal(t, "prod", firing.Env)
	require.Equal(t, "evaluator test firing", firing.Summary)
	require.Equal(t, managedAlertmanagerDefaultReceiver, firing.Receiver)
	require.Len(t, firing.Fingerprint, 16)
	require.Nil(t, firing.ResolvedAt)
	var firingLabels map[string]string
	require.NoError(t, json.Unmarshal([]byte(firing.LabelsJSON), &firingLabels))
	require.Equal(t, "stx", firingLabels["managed_by"])
	require.Equal(t, "cpu_usage_high", firingLabels["rule_key"])

	// 标签 severity=warning 只影响展示；外投判定看解析配置——此规则配了 critical 但未配渠道，
	// managed 投递缝认领后不外投，不应产生投递记录。
	// The severity label is display-only; external delivery follows the resolved config — this
	// rule is critical but has no channels, so the managed seam claims it without sending and
	// no delivery row may exist.
	var deliveryCount int64
	require.NoError(t, repo.db.WithContext(ctx).Model(&NotificationDelivery{}).Count(&deliveryCount).Error)
	require.Zero(t, deliveryCount)

	// 指标回落 → 同一告警进入 resolved：同指纹同 starts_at 的记录翻转状态并落 ResolvedAt。
	// Metric recovers → the same alert resolves: the record sharing fingerprint + starts_at
	// flips status and records ResolvedAt.
	appendEvalSample(t, ctx, "stx_eval_test_metric", map[string]string{
		"cluster_id": "1", "cluster_name": "prod", "env": "prod", "instance": "i1",
	}, time.Now(), 0)

	resolved := waitForRemoteAlertRecord(t, ctx, repo, func(r *RemoteAlertRecord) bool {
		return r.Fingerprint == firing.Fingerprint && r.StartsAt == firing.StartsAt && r.Status == "resolved"
	})
	require.NotNil(t, resolved, "resolved update should land on the same episode record")
	require.NotNil(t, resolved.ResolvedAt)
	require.Equal(t, firing.ID, resolved.ID, "resolved event must reuse the firing episode row")
}

// TestRemoteAlertRecordFromRuleAlert 覆盖 rules.Alert → RemoteAlertRecord 的字段映射：
// firing/resolved/pending 三种状态与统一告警记录的字段语义对齐。
// TestRemoteAlertRecordFromRuleAlert covers the rules.Alert → RemoteAlertRecord field mapping:
// firing/resolved/pending states matching the unified alert-record field semantics.
func TestRemoteAlertRecordFromRuleAlert(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	firedAt := now.Add(-time.Minute)
	validUntil := now.Add(4 * time.Minute)

	mkAlert := func(state rules.AlertState, resolvedAt time.Time) *rules.Alert {
		return &rules.Alert{
			State: state,
			Labels: labels.FromMap(map[string]string{
				"alertname":    "STXManagedPolicy_7",
				"managed_by":   "stx",
				"policy_id":    "7",
				"policy_name":  "CPU 告警",
				"severity":     "critical",
				"cluster_id":   "1",
				"cluster_name": "prod",
				"instance":     "10.0.0.1:5801",
				"env":          "prod",
			}),
			Annotations: labels.FromMap(map[string]string{
				"summary":     "s",
				"description": "d",
				"policy_name": "CPU 告警",
			}),
			ActiveAt:   firedAt.Add(-time.Minute),
			FiredAt:    firedAt,
			ValidUntil: validUntil,
			ResolvedAt: resolvedAt,
		}
	}

	t.Run("firing", func(t *testing.T) {
		record := remoteAlertRecordFromRuleAlert(mkAlert(rules.StateFiring, time.Time{}), now)
		require.NotNil(t, record)
		require.Equal(t, "firing", record.Status)
		require.Equal(t, firedAt.Unix(), record.StartsAt)
		require.Equal(t, validUntil.Unix(), record.EndsAt)
		require.Equal(t, "CPU 告警", record.AlertName)
		require.Equal(t, "critical", record.Severity)
		require.Equal(t, "1", record.ClusterID)
		require.Equal(t, "prod", record.ClusterName)
		require.Equal(t, "prod", record.Env)
		require.Equal(t, "s", record.Summary)
		require.Equal(t, "d", record.Description)
		require.Nil(t, record.ResolvedAt)
		require.Equal(t, now, record.LastReceivedAt)
		var parsed map[string]string
		require.NoError(t, json.Unmarshal([]byte(record.LabelsJSON), &parsed))
		require.Equal(t, "7", parsed["policy_id"])
	})

	t.Run("resolved", func(t *testing.T) {
		resolvedAt := now.Add(-10 * time.Second)
		record := remoteAlertRecordFromRuleAlert(mkAlert(rules.StateInactive, resolvedAt), now)
		require.NotNil(t, record)
		require.Equal(t, "resolved", record.Status)
		require.Equal(t, resolvedAt.Unix(), record.EndsAt)
		require.NotNil(t, record.ResolvedAt)
		require.Equal(t, resolvedAt, *record.ResolvedAt)
	})

	t.Run("pending dropped", func(t *testing.T) {
		require.Nil(t, remoteAlertRecordFromRuleAlert(mkAlert(rules.StatePending, time.Time{}), now))
	})

	t.Run("fingerprint stable", func(t *testing.T) {
		a := mkAlert(rules.StateFiring, time.Time{})
		r1 := remoteAlertRecordFromRuleAlert(a, now)
		r2 := remoteAlertRecordFromRuleAlert(a, now)
		require.Equal(t, r1.Fingerprint, r2.Fingerprint)
	})
}

// TestBuildManagedClusterRule 覆盖解析配置 → rulefmt.Rule 适配：alert 名取稳定 rule_key，
// labels 带 managed_by / rule_key / severity，for 取生效持续时间，表达式按集群匹配器注入
// label 约束且可被 promql 解析。
// TestBuildManagedClusterRule covers the resolved-config → rulefmt.Rule adapter: the alert
// name is the stable rule_key, labels carry managed_by / rule_key / severity, 'for' is the
// effective duration, and the cluster matcher is injected into a promql-parseable expression.
func TestBuildManagedClusterRule(t *testing.T) {
	def := lookupClusterRuleDefinition("cpu_usage_high")
	require.NotNil(t, def)

	cfg := &resolvedRuleConfig{
		RuleKey:    def.Key,
		Definition: def,
		Enabled:    true,
		Severity:   AlertSeverityCritical,
		ForSeconds: 180,
	}
	rule := buildManagedClusterRule(cfg, `cluster_id="7"`, "prod-cluster")
	require.NotNil(t, rule)
	require.Equal(t, "cpu_usage_high", rule.Alert)
	require.Equal(t, model.Duration(180*time.Second), rule.For)
	require.Equal(t, "stx", rule.Labels["managed_by"])
	require.Equal(t, "cpu_usage_high", rule.Labels["rule_key"])
	require.Equal(t, "critical", rule.Labels["severity"])
	require.Equal(t, "cpu_usage_high", rule.Annotations["rule_key"])
	require.Equal(t, "prod-cluster", rule.Annotations["cluster_scope"])
	require.Contains(t, rule.Expr, `cluster_id="7"`)
	_, err := metricRuleExprParser.ParseExpr(rule.Expr)
	require.NoError(t, err, "generated expression must parse: %s", rule.Expr)
}

// TestBuildManagedClusterRule_thresholdPercentToRatio 验证用户态百分比阈值在 tsdb 表达式里
// 按设计换算为 ratio（80 → 0.8）。
// TestBuildManagedClusterRule_thresholdPercentToRatio verifies a user-facing percent threshold
// converts to a ratio inside the tsdb expression (80 → 0.8).
func TestBuildManagedClusterRule_thresholdPercentToRatio(t *testing.T) {
	def := lookupClusterRuleDefinition("cpu_usage_high")
	require.NotNil(t, def)
	require.NotNil(t, def.Threshold)

	threshold := 80.0
	cfg := &resolvedRuleConfig{
		RuleKey:    def.Key,
		Definition: def,
		Enabled:    true,
		Threshold:  &threshold,
		ForSeconds: 600,
	}
	rule := buildManagedClusterRule(cfg, "", "全部集群")
	require.Contains(t, rule.Expr, "> 0.8", "percent 80 must render as ratio 0.8: %s", rule.Expr)
	_, err := metricRuleExprParser.ParseExpr(rule.Expr)
	require.NoError(t, err)
}

// TestBuiltinMetricRuleGroups 校验内嵌默认规则文件解析成功且规模正确（10 录制 + 0 告警：
// 告警组已收敛到集群规则目录，录制规则保留）。
// TestBuiltinMetricRuleGroups asserts the embedded default rule file parses and has the
// expected shape (10 recording + 0 alerting: the alert group moved into the cluster rule
// catalog, recording rules stay).
func TestBuiltinMetricRuleGroups(t *testing.T) {
	groups, err := builtinMetricRuleGroupsOnce()
	require.NoError(t, err)
	require.NotNil(t, groups)

	var recording, alerting int
	for _, g := range groups.Groups {
		for _, r := range g.Rules {
			switch {
			case r.Record != "":
				recording++
				require.Contains(t, r.Record, "seatunnel:", "recording rules must materialise seatunnel: series")
			case r.Alert != "":
				alerting++
			}
		}
	}
	require.Equal(t, 10, recording)
	require.Equal(t, 0, alerting)
}

// TestMetricEvaluator_RestoresForStateAcrossRestart 验证 firing 告警跨重启的 for 状态恢复：
// e1 里 for=800ms 的告警触发 firing 后停机（ALERTS_FOR_STATE 随每次评估落盘、停止时不会被
// 写 stale），e2 同目录重启后 RestoreForState 应找回原 ActiveAt，告警在远小于一个完整 for
// 的时间内重新 firing——未恢复时最早也要等满一个 for 周期，时序上可判别。
// TestMetricEvaluator_RestoresForStateAcrossRestart verifies 'for' state restoration across a
// restart: after the for=800ms alert fires in boot e1 and the process stops (ALERTS_FOR_STATE
// is persisted on every eval and never stale-marked on stop), boot e2 on the same dir should
// recover the original ActiveAt via RestoreForState and re-fire well under one 'for' period—
// without restore the earliest possible firing is a full 'for' after restart, which is
// distinguishable on timing alone.
func TestMetricEvaluator_RestoresForStateAcrossRestart(t *testing.T) {
	ctx := context.Background()
	database, cleanup := setupMetricEvaluatorTestDB(t)
	t.Cleanup(cleanup)

	dir := t.TempDir()
	embedOpts := embedts.Options{
		Dir:               dir,
		RetentionDuration: 24 * time.Hour,
		QueryTimeout:      5 * time.Second,
		MaxSamples:        1000,
	}
	require.NoError(t, embedts.Start(ctx, embedOpts))
	t.Cleanup(func() { _ = embedts.Close() })

	repo := NewRepository(database)
	service := NewService(nil, nil, repo)
	policy := &AlertPolicy{
		Name:                       "重启恢复测试",
		PolicyType:                 AlertPolicyBuilderKindMetricsTemplate,
		TemplateKey:                "cpu_usage_high",
		ClusterID:                  "all",
		Severity:                   AlertSeverityWarning,
		Enabled:                    true,
		ConditionsJSON:             "[]",
		NotificationChannelIDsJSON: "[]",
		ReceiverUserIDsJSON:        "[]",
	}
	require.NoError(t, repo.db.WithContext(ctx).Create(policy).Error)

	const testSource = "stx-test-restart"
	newGroups := func() *rulefmt.RuleGroups {
		return &rulefmt.RuleGroups{Groups: []rulefmt.RuleGroup{{
			Name: "evaluator-restart-test",
			Rules: []rulefmt.Rule{{
				Alert: "STXEvalRestartAlert",
				Expr:  `stx_eval_restart_metric > 5`,
				For:   model.Duration(800 * time.Millisecond),
				Labels: map[string]string{
					"managed_by":  "stx",
					"policy_id":   fmt.Sprintf("%d", policy.ID),
					"policy_name": policy.Name,
					"severity":    "warning",
				},
				Annotations: map[string]string{"summary": "restart restore test"},
			}},
		}}}
	}
	startEvaluator := func() *MetricEvaluator {
		e, err := NewMetricEvaluator(service, MetricEvaluatorOptions{
			Interval:    20 * time.Millisecond,
			ResendDelay: time.Hour,
		})
		require.NoError(t, err)
		e.sources = []string{testSource}
		e.loader.setGroups(testSource, newGroups())
		require.NoError(t, e.Start(ctx))
		return e
	}

	appendEvalSample(t, ctx, "stx_eval_restart_metric", map[string]string{
		"cluster_id": "1", "cluster_name": "prod", "env": "prod", "instance": "i1",
	}, time.Now(), 10)

	// 第一轮：告警等满 for 触发 firing，ALERTS_FOR_STATE 样本随之落盘。
	// First boot: the alert fires after the 'for' gate and ALERTS_FOR_STATE lands on disk.
	e1 := startEvaluator()
	first := waitForRemoteAlertRecord(t, ctx, repo, func(r *RemoteAlertRecord) bool {
		return r.Status == "firing"
	})
	require.NotNil(t, first, "alert should fire within the first boot")
	// 多等一段再停机：上游恢复只对「for-state 样本里累计 pending 时长 ≥ for」的告警走
	// firing-retain 分支；刚触发就停会让累计值贴着 for 边界，落到 pending 重计分支。
	// Wait a bit before stopping: upstream restores into the firing-retain branch only when
	// the accrued pending time in the for-state sample already exceeds 'for'; stopping right
	// at the firing edge lands on the pending-restart branch instead.
	time.Sleep(300 * time.Millisecond)
	e1.Stop()
	require.NoError(t, embedts.Close())

	// 第二轮同目录重启：恢复后的告警应立刻重新 firing，而不是重新累计 for。
	// Second boot on the same dir: the restored alert must re-fire at once instead of
	// re-accruing the 'for' duration.
	require.NoError(t, embedts.Start(ctx, embedOpts))
	restartAt := time.Now()
	e2 := startEvaluator()
	t.Cleanup(e2.Stop)

	// 第二次 firing 的 FiredAt 与第一次只差零点几秒：starts_at 是秒精度，落在同一 unix 秒
	// 时会被 fingerprint+starts_at 唯一键 upsert 合并成同一条记录，此时 LastReceivedAt 仍会
	// 刷新——两种落库形态都算「重启后重新 firing」的证据。
	// The second firing's FiredAt is only fractions of a second after the first: starts_at has
	// second granularity, so the record can be upsert-merged by the fingerprint+starts_at unique
	// key—in that case LastReceivedAt still refreshes. Either persistence shape counts as proof
	// the alert re-fired after restart.
	second := waitForRemoteAlertRecord(t, ctx, repo, func(r *RemoteAlertRecord) bool {
		if r.Status != "firing" || r.Fingerprint != first.Fingerprint {
			return false
		}
		return r.StartsAt != first.StartsAt || r.LastReceivedAt.After(restartAt)
	})
	require.NotNil(t, second, "firing alert should re-emit quickly after restart via ALERTS_FOR_STATE restore")
	require.Less(t, time.Since(restartAt), 600*time.Millisecond, "restored alert must not wait a full for period")
}

// TestMetricEvaluator_BuiltinRulesEvaluateEndToEnd 用真实 rules.Manager 端到端跑内嵌规则组：
// 喂入覆盖全部录制规则输入面的原始指标，断言 10 条 seatunnel: 录制序列的计算结果，并让
// 11 条内置告警全部进入 pending——表达式语义、聚合标签与录制写回（AppendableV2）一并钉住。
// 内置规则文件自带 group interval（30s/15s）不适合单测，改用 interval 清零的同构副本提速。
// TestMetricEvaluator_BuiltinRulesEvaluateEndToEnd drives the embedded rule groups through the
// real rules.Manager end to end: raw inputs covering every recording rule are fed in, all 10
// seatunnel:* recording series are asserted, and all 11 builtin alerts reach pending—pinning
// expression semantics, aggregation labels and the AppendableV2 write-back at once. The rule
// file's own group intervals (30s/15s) are too slow for a unit test, so an identical copy
// with cleared intervals is used instead.
func TestMetricEvaluator_BuiltinRulesEvaluateEndToEnd(t *testing.T) {
	ctx := context.Background()
	database, cleanup := setupMetricEvaluatorTestDB(t)
	t.Cleanup(cleanup)
	startMetricEvaluatorTestEmbedTS(t, ctx)

	repo := NewRepository(database)
	evaluator, err := NewMetricEvaluator(NewService(nil, nil, repo), MetricEvaluatorOptions{
		Interval:    20 * time.Millisecond,
		ResendDelay: time.Hour,
	})
	require.NoError(t, err)

	// 重新解析一份内置规则组并把 group interval 清零，使其按测试的 20ms 评估间隔运行；
	// 只替换本实例 loader 里的 builtin 组，不改全局 once 缓存的共享对象。
	// Re-parse the builtin groups and clear the group intervals so they run on the test's
	// 20ms eval interval; replace only this instance's builtin groups in the loader without
	// mutating the shared once-cached object.
	groups, parseErrs := rulefmt.Parse(builtinSeaTunnelRulesYAML, false, model.UTF8Validation, metricRuleExprParser, promslog.NewNopLogger())
	require.Empty(t, parseErrs)
	for i := range groups.Groups {
		groups.Groups[i].Interval = 0
	}
	evaluator.sources = []string{builtinMetricRulesSource, managedMetricRulesSource}
	evaluator.loader.setGroups(builtinMetricRulesSource, groups)

	// 目录默认关闭的 tsdb 规则也全部打开，让受测表达式覆盖整个目录告警面。
	// Enable the catalog tsdb rules that are off by default so every catalog alert expression
	// is exercised.
	for _, key := range []string{
		"cpu_usage_high",
		"gc_time_high",
		"job_thread_pool_queue_backlog_high",
		"coordinator_backlog_growing",
		"failed_jobs_high",
	} {
		require.NoError(t, repo.UpsertClusterRuleConfig(ctx, &ClusterRuleConfig{
			RuleKey: key,
			Enabled: ptrBool(true),
		}))
	}

	baseLabels := map[string]string{
		"job":          "seatunnel_engine_http",
		"instance":     "10.0.0.9:5801",
		"cluster":      "c9",
		"cluster_id":   "9",
		"cluster_name": "c9",
		"env":          "prod",
	}
	merged := func(extra map[string]string) map[string]string {
		out := make(map[string]string, len(baseLabels)+len(extra))
		for k, v := range baseLabels {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	// 计数器按固定斜率铺满最近 6 分钟（30s 一个点）：5m 速率窗口两端都有采样，
	// rate() 外推余量小，斜率本身即可被断言。
	// Counters ramp at a fixed slope across the last 6 minutes (one point per 30s): the 5m
	// rate window is sampled at both edges so rate() extrapolation stays marginal and the
	// slope itself can be asserted.
	appendCounterRamp := func(metric string, slopePerSec float64) {
		for i := 12; i >= 0; i-- {
			ts := time.Now().Add(-time.Duration(i) * 30 * time.Second)
			appendEvalSample(t, ctx, metric, baseLabels, ts, slopePerSec*float64((12-i)*30))
		}
	}
	appendCounterRamp("job_thread_pool_task_total", 1.0)
	appendCounterRamp("job_thread_pool_completedTask_total", 0.2)
	appendCounterRamp("job_thread_pool_rejection_total", 0.1)
	appendCounterRamp("jvm_gc_collection_seconds_sum", 0.3)
	appendCounterRamp("process_cpu_seconds_total", 1.0)

	// 其余输入按即时值写入；组合刻意让全部 11 条内置告警都进入 pending。
	// Remaining inputs are written as instant values; the combination is chosen so all 11
	// builtin alerts end up pending.
	instantSamples := []struct {
		metric string
		extra  map[string]string
		value  float64
	}{
		{"job_thread_pool_queueTaskCount", map[string]string{"pool": "a"}, 150},
		{"job_thread_pool_queueTaskCount", map[string]string{"pool": "b"}, 40},
		{"jvm_memory_bytes_used", map[string]string{"area": "heap"}, 900},
		{"jvm_memory_bytes_max", map[string]string{"area": "heap"}, 1000},
		{"jvm_memory_bytes_used", map[string]string{"area": "nonheap"}, 100},
		{"jvm_memory_bytes_max", map[string]string{"area": "nonheap"}, 400},
		{"process_open_fds", nil, 900},
		{"process_max_fds", nil, 1000},
		{"hazelcast_executor_queueSize", nil, 30},
		{"hazelcast_executor_queueRemainingCapacity", nil, 70},
		{"up", nil, 0},
		{"node_state", nil, 0},
		{"hazelcast_partition_isClusterSafe", nil, 1},
		{"hazelcast_partition_isLocalMemberSafe", nil, 0},
		{"jvm_threads_deadlocked", nil, 2},
		{"job_count", map[string]string{"state": "FAILED"}, 3},
	}
	for _, sample := range instantSamples {
		appendEvalSample(t, ctx, sample.metric, merged(sample.extra), time.Now(), sample.value)
	}

	require.NoError(t, evaluator.Start(ctx))
	t.Cleanup(evaluator.Stop)

	// 录制规则先产出 seatunnel: 序列，依赖它们的告警要再过一个评估周期才转 pending——
	// 轮询直到全部目录 tsdb 告警（alertname=rule_key）凑齐。
	// Recording rules must emit the seatunnel:* series before dependent alerts turn pending,
	// which takes a couple of eval cycles—poll until every catalog tsdb alert (alertname is the
	// rule_key) shows up.
	expectedAlertnames := map[string]struct{}{
		"split_brain_risk":                   {},
		"deadlocked_threads_detected":        {},
		"job_thread_pool_rejection_high":     {},
		"memory_usage_high":                  {},
		"fd_usage_high":                      {},
		"cpu_usage_high":                     {},
		"gc_time_high":                       {},
		"job_thread_pool_queue_backlog_high": {},
		"coordinator_backlog_growing":        {},
		"failed_jobs_high":                   {},
	}
	seen := map[string]struct{}{}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		samples, err := promquery.Instant(ctx, `ALERTS{alertstate="pending"}`, time.Now())
		require.NoError(t, err)
		for _, s := range samples {
			seen[s.Metric["alertname"]] = struct{}{}
		}
		if len(seen) == len(expectedAlertnames) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for name := range expectedAlertnames {
		require.Contains(t, seen, name, "alert %s never went pending", name)
	}
	require.Len(t, seen, len(expectedAlertnames))

	// pending 不该触达投递缝（没有 firing/resolved 事件就没有告警记录）。
	// Pending alerts must not reach the delivery seam (no firing/resolved events, no records).
	var records []RemoteAlertRecord
	require.NoError(t, repo.db.WithContext(ctx).Find(&records).Error)
	require.Empty(t, records)

	assertSeriesValue := func(expr string, want, delta float64) []promquery.Sample {
		samples, err := promquery.Instant(ctx, expr, time.Now())
		require.NoError(t, err)
		require.Len(t, samples, 1, "expected exactly one series for %q, got %d", expr, len(samples))
		require.InDelta(t, want, samples[0].Value, delta, "unexpected value for %q", expr)
		return samples
	}

	// 队列深度聚合：max by (cluster, instance) 输出只保留分组标签（含 __name__ 共 3 个）。
	// Queue depth aggregation: max by (cluster, instance) keeps only the grouping labels
	// (3 including __name__).
	depth := assertSeriesValue(`seatunnel:job_thread_pool_queue_depth:max`, 150, 0.001)
	require.Equal(t, "c9", depth[0].Metric["cluster"])
	require.Equal(t, "10.0.0.9:5801", depth[0].Metric["instance"])
	require.Len(t, depth[0].Metric, 3)

	assertSeriesValue(`seatunnel:job_thread_pool_submit_rate5m`, 1.0, 0.2)
	assertSeriesValue(`seatunnel:job_thread_pool_complete_rate5m`, 0.2, 0.15)
	assertSeriesValue(`seatunnel:job_thread_pool_reject_rate5m`, 0.1, 0.1)
	assertSeriesValue(`seatunnel:job_thread_pool_backlog_gap_rate5m`, 0.8, 0.3)
	assertSeriesValue(`seatunnel:jvm_heap_usage_percent`, 90, 0.5)
	assertSeriesValue(`seatunnel:jvm_nonheap_usage_percent`, 25, 0.5)
	assertSeriesValue(`seatunnel:gc_time_ratio_percent5m`, 30, 4)
	assertSeriesValue(`seatunnel:fd_usage_percent`, 90, 0.5)
	assertSeriesValue(`seatunnel:hazelcast_executor_queue_util_percent`, 30, 0.5)
}
