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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/rules"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/LeonYoah/stx/internal/pkg/embedts"
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
	policy := &AlertPolicy{
		Name:                       "CPU 高负载",
		PolicyType:                 AlertPolicyBuilderKindMetricsTemplate,
		TemplateKey:                "cpu_usage_high",
		ClusterID:                  "all",
		Severity:                   AlertSeverityWarning,
		Enabled:                    true,
		SendRecovery:               true,
		ConditionsJSON:             "[]",
		NotificationChannelIDsJSON: "[]",
		ReceiverUserIDsJSON:        "[]",
		LastExecutionStatus:        AlertPolicyExecutionStatusIdle,
	}
	require.NoError(t, repo.db.WithContext(ctx).Create(policy).Error)

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
				"managed_by":  "stx",
				"policy_id":   fmt.Sprintf("%d", policy.ID),
				"policy_name": policy.Name,
				"severity":    "warning",
			},
			Annotations: map[string]string{
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

	// AlertName 按统一告警记录语义取 policy_name → alertname。
	// AlertName resolves policy_name → alertname per the unified alert-record semantics.
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
	require.Equal(t, fmt.Sprintf("%d", policy.ID), firingLabels["policy_id"])

	// managed 投递缝已认领该告警并累计策略匹配数（无渠道时不发通知但记录状态）。
	// The managed delivery seam claimed the alert and bumped the policy match counter
	// (no channels configured, so nothing is sent but execution state is recorded).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var reloaded AlertPolicy
		require.NoError(t, repo.db.WithContext(ctx).First(&reloaded, policy.ID).Error)
		if reloaded.MatchCount >= 1 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	var reloaded AlertPolicy
	require.NoError(t, repo.db.WithContext(ctx).First(&reloaded, policy.ID).Error)
	require.Equal(t, 1, reloaded.MatchCount)
	require.NotNil(t, reloaded.LastMatchedAt)

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

// TestBuildManagedMetricPolicyRule 覆盖策略 → rulefmt.Rule 适配：标签/注解/表达式/for 与原
// 写 Prometheus 规则文件的字段一一对应；禁用/非指标模板/未知模板返回 ok=false。
// TestBuildManagedMetricPolicyRule covers the policy → rulefmt.Rule adapter: labels,
// annotations, expression and 'for' match the fields the old Prometheus rule file carried;
// disabled / non-metric / unknown-template policies return ok=false.
func TestBuildManagedMetricPolicyRule(t *testing.T) {
	clusterNames := map[string]string{"all": "全部集群", "7": "prod-cluster"}

	t.Run("metrics template", func(t *testing.T) {
		policy := &AlertPolicy{
			ID:             5,
			Name:           "CPU 告警",
			PolicyType:     AlertPolicyBuilderKindMetricsTemplate,
			TemplateKey:    "cpu_usage_high",
			ClusterID:      "all",
			Severity:       AlertSeverityCritical,
			Enabled:        true,
			ConditionsJSON: `[{"metric_key":"cpu","operator":">","threshold":"0.8","window_minutes":3}]`,
		}
		rule, ok, err := buildManagedMetricPolicyRule(policy, clusterNames)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "STXManagedPolicy_5", rule.Alert)
		require.Equal(t, model.Duration(3*time.Minute), rule.For)
		require.Contains(t, rule.Expr, `process_cpu_seconds_total{job="seatunnel_engine_http"}[3m]`)
		require.Contains(t, rule.Expr, `> 0.8`)
		require.Equal(t, "stx", rule.Labels["managed_by"])
		require.Equal(t, "5", rule.Labels["policy_id"])
		require.Equal(t, "critical", rule.Labels["severity"])
		require.Equal(t, "cpu_usage_high", rule.Labels["template_key"])
		require.Contains(t, rule.Annotations["summary"], "CPU 告警")
		require.Contains(t, rule.Annotations["condition"], "3")
		require.NotEmpty(t, rule.Annotations["current_value"])
	})

	t.Run("cluster scoped", func(t *testing.T) {
		policy := &AlertPolicy{
			ID:          9,
			Name:        "队列积压",
			PolicyType:  AlertPolicyBuilderKindMetricsTemplate,
			TemplateKey: "job_thread_pool_queue_backlog_high",
			ClusterID:   "7",
			Enabled:     true,
		}
		rule, ok, err := buildManagedMetricPolicyRule(policy, clusterNames)
		require.NoError(t, err)
		require.True(t, ok)
		require.Contains(t, rule.Expr, `cluster_id="7"`)
		require.Equal(t, "prod-cluster", rule.Annotations["cluster_scope"])
	})

	t.Run("disabled / unknown / non-metric", func(t *testing.T) {
		disabled := &AlertPolicy{ID: 1, PolicyType: AlertPolicyBuilderKindMetricsTemplate, TemplateKey: "cpu_usage_high", Enabled: false}
		_, ok, err := buildManagedMetricPolicyRule(disabled, clusterNames)
		require.NoError(t, err)
		require.False(t, ok)

		unknown := &AlertPolicy{ID: 2, PolicyType: AlertPolicyBuilderKindMetricsTemplate, TemplateKey: "no_such_template", Enabled: true}
		_, ok, err = buildManagedMetricPolicyRule(unknown, clusterNames)
		require.NoError(t, err)
		require.False(t, ok)

		syncJob := &AlertPolicy{ID: 3, PolicyType: AlertPolicyBuilderKindSyncJob, Enabled: true}
		_, ok, err = buildManagedMetricPolicyRule(syncJob, clusterNames)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

// TestBuiltinMetricRuleGroups 校验内嵌默认规则文件解析成功且规模正确（10 录制 + 11 告警）。
// TestBuiltinMetricRuleGroups asserts the embedded default rule file parses and has the
// expected shape (10 recording + 11 alerting rules).
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
	require.Equal(t, 11, alerting)
}
