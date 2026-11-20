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

package migrator

import (
	"strconv"
	"strings"
	"testing"
	"time"

	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newAlertCatalogMigrationDB 建一张只含本迁移相关表的内存库。
// newAlertCatalogMigrationDB builds an in-memory database holding only the tables this
// migration touches.
func newAlertCatalogMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 事务会换连接，file::memory: 每条连接各是一张空库；用共享缓存的具名内存库并把
	// 连接池限到 1，保证迁移读到测试写入的数据。
	// Transactions may hop connections and every file::memory: connection is an empty database;
	// a shared-cache named in-memory database plus a single-connection pool keeps the test data
	// visible to the migration.
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(
		&monitoringapp.AlertPolicy{},
		&monitoringapp.AlertRule{},
		&monitoringapp.NotificationRoute{},
		&monitoringapp.NotificationChannel{},
		&monitoringapp.ClusterRuleConfig{},
		&monitoringapp.ClusterRuleOverride{},
		&monitoringapp.AlertNotificationDefaults{},
	))
	return database
}

func createChannel(t *testing.T, db *gorm.DB, enabled bool) monitoringapp.NotificationChannel {
	t.Helper()
	channel := monitoringapp.NotificationChannel{
		Name:    "ch",
		Type:    "email",
		Enabled: enabled,
	}
	require.NoError(t, db.Create(&channel).Error)
	return channel
}

// TestMigrateAlertRuleCatalogGlobalPolicy 覆盖：全局 metrics_template 策略 → 全局配置；
// ratio 阈值 → percent；window_minutes → for_seconds；启用渠道 → critical + custom。
// TestMigrateAlertRuleCatalogGlobalPolicy covers: a global metrics_template policy becomes the
// global config; ratio thresholds become percent; window_minutes become for_seconds; an
// enabled channel yields critical + custom.
func TestMigrateAlertRuleCatalogGlobalPolicy(t *testing.T) {
	db := newAlertCatalogMigrationDB(t)
	channel := createChannel(t, db, true)

	require.NoError(t, db.Create(&monitoringapp.AlertPolicy{
		Name:                       "cpu",
		PolicyType:                 monitoringapp.AlertPolicyBuilderKindMetricsTemplate,
		TemplateKey:                "cpu_usage_high",
		ClusterID:                  "all",
		Severity:                   monitoringapp.AlertSeverityCritical,
		Enabled:                    true,
		ConditionsJSON:             `[{"metric_key":"cpu","operator":">","threshold":"0.8","window_minutes":15}]`,
		NotificationChannelIDsJSON: `[` + itoa(channel.ID) + `]`,
	}).Error)

	require.NoError(t, MigrateAlertRuleCatalog(db))

	var cfg monitoringapp.ClusterRuleConfig
	require.NoError(t, db.Where("rule_key = ?", "cpu_usage_high").First(&cfg).Error)
	require.NotNil(t, cfg.Enabled)
	require.True(t, *cfg.Enabled)
	require.NotNil(t, cfg.Threshold)
	require.InDelta(t, 80, *cfg.Threshold, 0.001)
	require.NotNil(t, cfg.ForSeconds)
	require.Equal(t, 900, *cfg.ForSeconds)
	require.NotNil(t, cfg.Severity)
	require.Equal(t, "critical", *cfg.Severity)
	require.Equal(t, "custom", cfg.NotifyMode)
	require.JSONEq(t, `[`+itoa(channel.ID)+`]`, cfg.ChannelIDsJSON)

	// 迁移后旧策略行保留但置为停用 / Migrated policy rows stay but are disabled.
	var policy monitoringapp.AlertPolicy
	require.NoError(t, db.First(&policy).Error)
	require.False(t, policy.Enabled)
}

// TestMigrateAlertRuleCatalogClusterPolicyAndDowngrade 覆盖：指定集群策略 → 集群例外；
// 无启用渠道的策略降为 warning 并保留 custom 通知。
// TestMigrateAlertRuleCatalogClusterPolicyAndDowngrade covers: a cluster-scoped policy becomes
// an override; a policy without enabled channels downgrades to warning while keeping custom
// notification.
func TestMigrateAlertRuleCatalogClusterPolicyAndDowngrade(t *testing.T) {
	db := newAlertCatalogMigrationDB(t)
	disabled := createChannel(t, db, false)

	require.NoError(t, db.Create(&monitoringapp.AlertPolicy{
		Name:                       "node",
		PolicyType:                 monitoringapp.AlertPolicyBuilderKindPlatformHealth,
		LegacyRuleKey:              monitoringapp.AlertRuleKeyNodeOffline,
		ClusterID:                  "6",
		Severity:                   monitoringapp.AlertSeverityCritical,
		Enabled:                    true,
		ConditionsJSON:             `[{"metric_key":"","operator":"","threshold":"","window_minutes":5}]`,
		NotificationChannelIDsJSON: `[` + itoa(disabled.ID) + `]`,
	}).Error)

	require.NoError(t, MigrateAlertRuleCatalog(db))

	var override monitoringapp.ClusterRuleOverride
	require.NoError(t, db.Where("rule_key = ? AND cluster_id = ?", monitoringapp.AlertRuleKeyNodeOffline, 6).First(&override).Error)
	require.NotNil(t, override.Severity)
	require.Equal(t, "warning", *override.Severity)
	require.Equal(t, "custom", override.NotifyMode)
	require.NotNil(t, override.ForSeconds)
	require.Equal(t, 300, *override.ForSeconds)
}

// TestMigrateAlertRuleCatalogConflictResolution 覆盖：同 (rule, cluster) 多策略取最近更新的
// 启用策略；组内全停用时取最近更新的一条。
// TestMigrateAlertRuleCatalogConflictResolution covers: multiple policies for one
// (rule, cluster) resolve to the most recently updated enabled policy; when all are disabled
// the most recently updated one wins.
func TestMigrateAlertRuleCatalogConflictResolution(t *testing.T) {
	db := newAlertCatalogMigrationDB(t)
	channel := createChannel(t, db, true)
	old := time.Now().Add(-time.Hour)
	recent := time.Now()

	// enabled 有 gorm default:true、updated_at 是 autoUpdateTime，Create 会覆盖显式值；
	// 先按启用创建再用 UpdateColumn 落定测试语义。
	// enabled carries gorm default:true and updated_at is autoUpdateTime, so Create overwrites
	// explicit values; create enabled then pin the test semantics with UpdateColumn.
	oldPolicy := monitoringapp.AlertPolicy{
		Name:           "old-enabled",
		PolicyType:     monitoringapp.AlertPolicyBuilderKindMetricsTemplate,
		TemplateKey:    "memory_usage_high",
		ClusterID:      "",
		Enabled:        true,
		ConditionsJSON: `[{"threshold":"0.5","window_minutes":5}]`,
	}
	require.NoError(t, db.Create(&oldPolicy).Error)
	require.NoError(t, db.Model(&oldPolicy).UpdateColumn("updated_at", recent).Error)
	require.NoError(t, db.Create(&monitoringapp.AlertPolicy{
		Name:                       "newer-enabled",
		PolicyType:                 monitoringapp.AlertPolicyBuilderKindMetricsTemplate,
		TemplateKey:                "memory_usage_high",
		ClusterID:                  "",
		Enabled:                    true,
		ConditionsJSON:             `[{"threshold":"0.9","window_minutes":20}]`,
		NotificationChannelIDsJSON: `[` + itoa(channel.ID) + `]`,
	}).Error)
	disabledPolicy := monitoringapp.AlertPolicy{
		Name:          "disabled-policy",
		PolicyType:    monitoringapp.AlertPolicyBuilderKindPlatformHealth,
		LegacyRuleKey: "agent_offline",
		ClusterID:     "7",
		Enabled:       true,
	}
	require.NoError(t, db.Create(&disabledPolicy).Error)
	require.NoError(t, db.Model(&disabledPolicy).UpdateColumns(map[string]any{"enabled": false, "updated_at": old}).Error)

	require.NoError(t, MigrateAlertRuleCatalog(db))

	var cfg monitoringapp.ClusterRuleConfig
	require.NoError(t, db.Where("rule_key = ?", "memory_usage_high").First(&cfg).Error)
	require.NotNil(t, cfg.Threshold)
	require.InDelta(t, 90, *cfg.Threshold, 0.001)
	require.NotNil(t, cfg.ForSeconds)
	require.Equal(t, 1200, *cfg.ForSeconds)
	require.NotNil(t, cfg.Severity)
	require.Equal(t, "critical", *cfg.Severity)

	var override monitoringapp.ClusterRuleOverride
	require.NoError(t, db.Where("rule_key = ? AND cluster_id = ?", "agent_offline", 7).First(&override).Error)
	require.NotNil(t, override.Enabled)
	require.False(t, *override.Enabled)
}

// TestMigrateAlertRuleCatalogLegacyRuleRows 覆盖：monitoring_alert_rules 中 Enabled /
// WindowSeconds 偏离种子默认的行转为集群例外；与种子一致的行不迁移；未知规则键跳过；
// 已被策略写过的同 (rule, cluster) 例外不被覆盖。
// TestMigrateAlertRuleCatalogLegacyRuleRows covers: monitoring_alert_rules rows deviating
// from seed defaults become cluster overrides; seed-equal rows are skipped; unknown keys are
// skipped; an override already written by the policy migration is not overwritten.
func TestMigrateAlertRuleCatalogLegacyRuleRows(t *testing.T) {
	db := newAlertCatalogMigrationDB(t)

	offlineRow := monitoringapp.AlertRule{
		ClusterID: 6, RuleKey: monitoringapp.AlertRuleKeyNodeOffline,
		RuleName: "n", Severity: "critical", Enabled: true, Threshold: 1, WindowSeconds: 60,
	}
	require.NoError(t, db.Create(&offlineRow).Error)
	// enabled 的 gorm 默认是 true，停用要 UpdateColumn 落定。
	// enabled defaults to true via gorm, so disabling must go through UpdateColumn.
	require.NoError(t, db.Model(&offlineRow).UpdateColumn("enabled", false).Error)
	// 与种子一致：不迁移 / identical to the seed: skipped.
	require.NoError(t, db.Create(&monitoringapp.AlertRule{
		ClusterID: 6, RuleKey: monitoringapp.AlertRuleKeyProcessCrashed,
		RuleName: "p", Severity: "critical", Enabled: true, Threshold: 1, WindowSeconds: 300,
	}).Error)
	// 未知键：跳过 / unknown key: skipped.
	unknownRow := monitoringapp.AlertRule{
		ClusterID: 6, RuleKey: "removed_rule",
		RuleName: "x", Severity: "warning", Enabled: true, Threshold: 1, WindowSeconds: 60,
	}
	require.NoError(t, db.Create(&unknownRow).Error)
	require.NoError(t, db.Model(&unknownRow).UpdateColumn("enabled", false).Error)

	require.NoError(t, MigrateAlertRuleCatalog(db))

	var overrides []monitoringapp.ClusterRuleOverride
	require.NoError(t, db.Find(&overrides).Error)
	require.Len(t, overrides, 1)
	require.Equal(t, monitoringapp.AlertRuleKeyNodeOffline, overrides[0].RuleKey)
	require.Equal(t, uint(6), overrides[0].ClusterID)
	require.NotNil(t, overrides[0].Enabled)
	require.False(t, *overrides[0].Enabled)
}

// TestMigrateAlertRuleCatalogNotificationRoutes 覆盖 design §10 的三种路由情形：
// 无过滤 / 仅 source_type / 仅 cluster / severity 过滤 → 默认通知；
// rule_key（可带 cluster_id）→ 规则层 custom 渠道；停用路由与停用渠道跳过。
// TestMigrateAlertRuleCatalogNotificationRoutes covers the three route cases of design §10:
// unfiltered / source_type-only / cluster-only / severity-filtered routes land in the default
// notification; rule_key routes (optionally with cluster_id) land as custom channels on the
// rule layer; disabled routes and channels are skipped.
func TestMigrateAlertRuleCatalogNotificationRoutes(t *testing.T) {
	db := newAlertCatalogMigrationDB(t)
	chA := createChannel(t, db, true)
	chB := createChannel(t, db, true)
	chC := createChannel(t, db, true)
	chDisabled := createChannel(t, db, false)

	routes := []monitoringapp.NotificationRoute{
		{Name: "all", Enabled: true, ChannelID: chA.ID},
		{Name: "source-only", Enabled: true, SourceType: "platform_health", ChannelID: chB.ID},
		{Name: "cluster-only", Enabled: true, ClusterID: "9", ChannelID: chB.ID},
		{Name: "severity", Enabled: true, Severity: "critical", ChannelID: chB.ID},
		{Name: "rule", Enabled: true, RuleKey: "node_offline", ChannelID: chC.ID},
		{Name: "rule-cluster", Enabled: true, RuleKey: "node_offline", ClusterID: "6", ChannelID: chC.ID},
		{Name: "disabled-route", Enabled: false, ChannelID: chA.ID},
		{Name: "disabled-channel", Enabled: true, ChannelID: chDisabled.ID},
	}
	for _, route := range routes {
		require.NoError(t, db.Create(&route).Error)
	}

	require.NoError(t, MigrateAlertRuleCatalog(db))

	var defaults monitoringapp.AlertNotificationDefaults
	require.NoError(t, db.First(&defaults).Error)
	require.JSONEq(t, `[`+itoa(chA.ID)+`,`+itoa(chB.ID)+`]`, defaults.ChannelIDsJSON)

	var global monitoringapp.ClusterRuleConfig
	require.NoError(t, db.Where("rule_key = ?", "node_offline").First(&global).Error)
	require.Equal(t, "custom", global.NotifyMode)
	require.JSONEq(t, `[`+itoa(chC.ID)+`]`, global.ChannelIDsJSON)

	var override monitoringapp.ClusterRuleOverride
	require.NoError(t, db.Where("rule_key = ? AND cluster_id = ?", "node_offline", 6).First(&override).Error)
	require.Equal(t, "custom", override.NotifyMode)
	require.JSONEq(t, `[`+itoa(chC.ID)+`]`, override.ChannelIDsJSON)
}

// TestMigrateAlertRuleCatalogDefaultsFallback 覆盖：路由没有产生默认通知时，取全部迁移
// 策略渠道 / 接收人的并集兜底。
// TestMigrateAlertRuleCatalogDefaultsFallback covers: when no route produced a default
// notification, the union of all migrated policy channels / receivers is the fallback.
func TestMigrateAlertRuleCatalogDefaultsFallback(t *testing.T) {
	db := newAlertCatalogMigrationDB(t)
	chA := createChannel(t, db, true)
	chB := createChannel(t, db, true)

	require.NoError(t, db.Create(&monitoringapp.AlertPolicy{
		Name:                       "a",
		PolicyType:                 monitoringapp.AlertPolicyBuilderKindMetricsTemplate,
		TemplateKey:                "cpu_usage_high",
		Enabled:                    true,
		CooldownMinutes:            30,
		SendRecovery:               true,
		NotificationChannelIDsJSON: `[` + itoa(chA.ID) + `]`,
		ReceiverUserIDsJSON:        `[7]`,
	}).Error)
	require.NoError(t, db.Create(&monitoringapp.AlertPolicy{
		Name:                       "b",
		PolicyType:                 monitoringapp.AlertPolicyBuilderKindPlatformHealth,
		LegacyRuleKey:              "node_offline",
		Enabled:                    true,
		CooldownMinutes:            10,
		SendRecovery:               false,
		NotificationChannelIDsJSON: `[` + itoa(chB.ID) + `]`,
		ReceiverUserIDsJSON:        `[9]`,
	}).Error)

	require.NoError(t, MigrateAlertRuleCatalog(db))

	var defaults monitoringapp.AlertNotificationDefaults
	require.NoError(t, db.First(&defaults).Error)
	require.JSONEq(t, `[`+itoa(chA.ID)+`,`+itoa(chB.ID)+`]`, defaults.ChannelIDsJSON)
	require.JSONEq(t, `[7,9]`, defaults.ReceiverUserIDsJSON)
	require.Equal(t, 30, defaults.CooldownMinutes)
	// 任一启用策略开恢复 → 默认通知保持开恢复。
	// Any enabled policy with recovery on keeps the default recovery switch on.
	require.True(t, defaults.SendRecovery)
}

// TestMigrateAlertRuleCatalogSyncJobUntouchedAndIdempotent 覆盖：sync_job 策略不动；
// 重复执行为空操作；旧行保留。
// TestMigrateAlertRuleCatalogSyncJobUntouchedAndIdempotent covers: sync_job policies stay
// untouched; a repeated run is a no-op; legacy rows are preserved.
func TestMigrateAlertRuleCatalogSyncJobUntouchedAndIdempotent(t *testing.T) {
	db := newAlertCatalogMigrationDB(t)
	channel := createChannel(t, db, true)

	require.NoError(t, db.Create(&monitoringapp.AlertPolicy{
		Name:       "sync",
		PolicyType: monitoringapp.AlertPolicyBuilderKindSyncJob,
		Enabled:    true,
		TaskID:     42,
	}).Error)
	require.NoError(t, db.Create(&monitoringapp.AlertPolicy{
		Name:                       "legacy",
		PolicyType:                 monitoringapp.AlertPolicyBuilderKindPlatformHealth,
		LegacyRuleKey:              "master_unavailable",
		Enabled:                    true,
		NotificationChannelIDsJSON: `[` + itoa(channel.ID) + `]`,
	}).Error)

	require.NoError(t, MigrateAlertRuleCatalog(db))

	var syncPolicy, legacyPolicy monitoringapp.AlertPolicy
	require.NoError(t, db.Where("policy_type = ?", monitoringapp.AlertPolicyBuilderKindSyncJob).First(&syncPolicy).Error)
	require.True(t, syncPolicy.Enabled)
	require.NoError(t, db.Where("policy_type = ?", monitoringapp.AlertPolicyBuilderKindPlatformHealth).First(&legacyPolicy).Error)
	require.False(t, legacyPolicy.Enabled)

	var cfgCount, policyCount int64
	require.NoError(t, db.Model(&monitoringapp.ClusterRuleConfig{}).Count(&cfgCount).Error)
	require.NoError(t, db.Model(&monitoringapp.AlertPolicy{}).Count(&policyCount).Error)
	require.Equal(t, int64(2), policyCount)

	// 重复执行：标记短路，行数不变 / second run: flag short-circuits, row counts unchanged.
	require.NoError(t, MigrateAlertRuleCatalog(db))
	var cfgCount2, policyCount2 int64
	require.NoError(t, db.Model(&monitoringapp.ClusterRuleConfig{}).Count(&cfgCount2).Error)
	require.NoError(t, db.Model(&monitoringapp.AlertPolicy{}).Count(&policyCount2).Error)
	require.Equal(t, cfgCount, cfgCount2)
	require.Equal(t, policyCount, policyCount2)
}

// TestMigrateAlertRuleCatalogEmptyDatabase 覆盖：无遗留数据时迁移只写标记，不产配置行。
// TestMigrateAlertRuleCatalogEmptyDatabase covers: with no legacy data the migration only
// writes the flag and produces no config rows.
func TestMigrateAlertRuleCatalogEmptyDatabase(t *testing.T) {
	db := newAlertCatalogMigrationDB(t)
	require.NoError(t, MigrateAlertRuleCatalog(db))

	var flagged int64
	require.NoError(t, db.Model(&SchemaFlag{}).Where("name = ?", AlertRuleCatalogMigrationName).Count(&flagged).Error)
	require.Equal(t, int64(1), flagged)

	var count int64
	require.NoError(t, db.Model(&monitoringapp.ClusterRuleConfig{}).Count(&count).Error)
	require.Zero(t, count)
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
