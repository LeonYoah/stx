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
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/pkg/timex"
	"gorm.io/gorm"
)

// AlertRuleCatalogMigrationName 是告警模型收拢迁移的标记名。
// AlertRuleCatalogMigrationName is the flag name of the alert-model consolidation migration.
const AlertRuleCatalogMigrationName = "alert_rule_catalog_v1"

// legacyRuleSeedDefault 记录被删除的 monitoring_alert_rules 种子值。数据迁移的语义须冻结
// 在切换时点，不能随目录后续演进漂移，所以种子表与规则键集合都内联在本文件里。
// legacyRuleSeedDefault freezes the removed monitoring_alert_rules seed values. Migration
// semantics must be pinned at cutover time and must not drift with later catalog evolution, so
// the seed table and the rule-key set are inlined here.
type legacyRuleSeedDefault struct {
	Enabled       bool
	WindowSeconds int
}

// legacyAlertRuleSeeds 是已删除的 defaultClusterRules() 种下的 8 条 per-cluster 默认行。
// legacyAlertRuleSeeds are the 8 per-cluster default rows the removed defaultClusterRules() seeded.
var legacyAlertRuleSeeds = map[string]legacyRuleSeedDefault{
	monitoringapp.AlertRuleKeyProcessCrashed:             {Enabled: true, WindowSeconds: 300},
	monitoringapp.AlertRuleKeyProcessRestartFailed:       {Enabled: true, WindowSeconds: 300},
	monitoringapp.AlertRuleKeyProcessRestartLimitReached: {Enabled: true, WindowSeconds: 300},
	monitoringapp.AlertRuleKeyClusterRestartRequested:    {Enabled: true, WindowSeconds: 60},
	monitoringapp.AlertRuleKeyNodeStopRequested:          {Enabled: true, WindowSeconds: 60},
	monitoringapp.AlertRuleKeyNodeOffline:                {Enabled: true, WindowSeconds: 60},
	"sync_status_unavailable":                            {Enabled: true, WindowSeconds: 300},
	"sync_event_channel_degraded":                        {Enabled: true, WindowSeconds: 60},
}

// legacyCatalogRuleKeys 是切换时点目录中 21 条规则的键集合；映射不进目录的旧数据只记摘要。
// legacyCatalogRuleKeys is the set of the 21 catalog keys at cutover; legacy rows that cannot be
// mapped are only counted in the summary.
var legacyCatalogRuleKeys = map[string]struct{}{
	monitoringapp.AlertRuleKeyNodeOffline:                {},
	"master_unavailable":                                 {},
	"worker_insufficient":                                {},
	"agent_offline":                                      {},
	monitoringapp.AlertRuleKeyProcessCrashed:             {},
	monitoringapp.AlertRuleKeyProcessRestartFailed:       {},
	monitoringapp.AlertRuleKeyProcessRestartLimitReached: {},
	"sync_status_unavailable":                            {},
	"sync_event_channel_degraded":                        {},
	"split_brain_risk":                                   {},
	"deadlocked_threads_detected":                        {},
	"job_thread_pool_rejection_high":                     {},
	"memory_usage_high":                                  {},
	"fd_usage_high":                                      {},
	"cpu_usage_high":                                     {},
	"gc_time_high":                                       {},
	"job_thread_pool_queue_backlog_high":                 {},
	"coordinator_backlog_growing":                        {},
	"failed_jobs_high":                                   {},
	monitoringapp.AlertRuleKeyClusterRestartRequested:    {},
	monitoringapp.AlertRuleKeyNodeStopRequested:          {},
}

// legacyPercentUnitRules 是目录中阈值单位为 percent 的规则：旧指标模板把阈值存成
// ratio（0.8 表示 80%），迁移时 ×100；值已大于 1 的视为已是 percent，不再换算。
// legacyPercentUnitRules are catalog rules whose threshold unit is percent: legacy metric
// templates stored thresholds as ratios (0.8 means 80%), so the migration scales them by 100;
// values already above 1 are treated as percent and kept as-is.
var legacyPercentUnitRules = map[string]struct{}{
	"cpu_usage_high":    {},
	"memory_usage_high": {},
	"fd_usage_high":     {},
	"gc_time_high":      {},
}

// legacyThresholdRules 是目录中带可编辑阈值的规则；其余规则没有阈值概念，忽略旧条件里的阈值。
// legacyThresholdRules are catalog rules carrying an editable threshold; other rules have no
// threshold concept, so legacy condition thresholds for them are ignored.
var legacyThresholdRules = map[string]struct{}{
	"worker_insufficient":                {},
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

// legacyDeferredRuleKeys 是迁移前没有判定引擎的三条模板规则：配置照常迁移，摘要里注明
// “升级后开始生效”。
// legacyDeferredRuleKeys are the three template rules that had no evaluator before the cutover:
// their config still migrates, but the summary notes they "take effect after upgrade".
var legacyDeferredRuleKeys = map[string]struct{}{
	"master_unavailable":  {},
	"worker_insufficient": {},
	"agent_offline":       {},
}

// legacyPolicyCondition 与 AlertPolicyConditionDTO 同构；迁移包不复用内部类型以保持冻结语义。
// legacyPolicyCondition mirrors AlertPolicyConditionDTO; the migration re-declares it to keep
// cutover semantics frozen.
type legacyPolicyCondition struct {
	MetricKey     string `json:"metric_key"`
	Operator      string `json:"operator"`
	Threshold     string `json:"threshold"`
	WindowMinutes int    `json:"window_minutes"`
}

// legacyRuleLayer 是把一条胜出策略（或种子偏离行）落成配置层前的中间形态。
// legacyRuleLayer is the intermediate form of one winning policy (or a seed-deviation row)
// before it lands as a config layer.
type legacyRuleLayer struct {
	enabled             *bool
	threshold           *float64
	forSeconds          *int
	severity            *string
	notifyMode          string
	channelIDsJSON      string
	receiverUserIDsJSON string
}

// legacyMigrationStats 汇总迁移计数，用于双语日志摘要。
// legacyMigrationStats collects counters for the bilingual log summary.
type legacyMigrationStats struct {
	policiesMigrated       int
	policyConflictsSkipped int
	policiesUnknownKey     int
	policiesDisabled       int
	severityDowngraded     int
	deferredRulePolicies   int
	ruleRowsMigrated       int
	ruleRowsSkipped        int
	routesToDefaults       int
	routesToRules          int
	routesSkipped          int
	defaultsFromFallback   bool
	defaultChannels        []uint
	defaultReceiverIDs     []uint64
}

// MigrateAlertRuleCatalog 把遗留非作业告警配置一次性收拢进集群规则目录的三张配置表：
//
//   - metrics_template / platform_health 策略按 (ruleKey, cluster) 分组，cluster 为空 / "all"
//     的写全局配置行，指定集群的写集群例外行；同组冲突取最近更新的启用策略（组内全部停用
//     时取最近更新的一条以保留停用意图），其余记摘要。启用状态、阈值（percent 规则按
//     ratio→percent 换算）、持续时间（window_minutes→for_seconds）、渠道 / 接收人
//     （非空 → notify_mode=custom）逐字段迁移；带启用渠道 → critical，否则 → warning，
//     因无启用渠道被降级的记入摘要。三条无判定模板照常迁移并记“升级后开始生效”。
//   - monitoring_alert_rules 中 Enabled / WindowSeconds 偏离种子默认的行写为集群例外；
//     若策略迁移已写过同 (rule, cluster) 例外，种子行只补策略未写的字段，不覆盖。
//   - 启用的 NotificationRoute：无过滤或仅 source_type 过滤的渠道并入默认通知；带
//     rule_key 过滤的写入对应规则的全局层或集群例外（custom 渠道去重合并）；带
//     severity 过滤的无法表达，渠道并入默认通知并记摘要；仅 cluster 过滤没有 rule_key 的
//     无法归属单条规则，同样并入默认通知并记摘要。停用渠道与停用路由跳过并记数。
//   - 默认通知仍为空且存在迁移策略时，取全部迁移策略渠道 / 接收人的并集兜底；
//     cooldown 取迁移启用策略的最大值，send_recovery 仅当全部迁移启用策略都关闭时才关。
//   - sync_job 策略不动。迁移完成后把全部 metrics_template / platform_health 策略置
//     Enabled=false（旧行不删，便于回滚与排查）。
//
// 幂等：stx_schema_flags 标记存在即跳过；标记在事务内先占位，并发启动时后到实例主键冲突
// 整体回滚（与 MigrateLegacyRemoteAlerts 同一约定）。
//
// MigrateAlertRuleCatalog folds legacy non-job alert configuration into the three cluster-rule
// config tables, once:
//
//   - metrics_template / platform_health policies group by (ruleKey, cluster); an empty /
//     "all" cluster writes a global config row, a specific cluster writes an override row.
//     Conflicts resolve to the most recently updated enabled policy (or the most recently
//     updated one when all are disabled, keeping the disabled intent); the rest are logged.
//     Enabled, threshold (ratio→percent for percent-unit rules), duration
//     (window_minutes→for_seconds) and channels / receivers (non-empty → notify_mode=custom)
//     migrate field-wise; policies with an enabled channel become critical, the rest warning,
//     and channel-less downgrades are logged. The three evaluator-less templates still
//     migrate and are logged as "takes effect after upgrade".
//   - monitoring_alert_rules rows whose Enabled / WindowSeconds deviate from the removed seed
//     defaults become cluster overrides; when the policy migration already wrote that
//     (rule, cluster) override, the seed row only fills fields the policy left unset.
//   - Enabled NotificationRoutes: unfiltered or source_type-only routes merge their channel
//     into the default notification; rule_key-filtered routes merge into that rule's global
//     layer or cluster override as a deduped custom channel; severity-filtered routes cannot
//     be expressed and merge into the default notification with a summary entry; cluster-only
//     routes without a rule_key cannot be attributed to one rule and also merge into the
//     default notification with a summary entry. Disabled channels and routes are skipped
//     and counted.
//   - When the default notification would stay empty while migrated policies exist, the union
//     of all migrated policy channels / receivers becomes the fallback; cooldown takes the
//     max across migrated enabled policies and send_recovery stays on unless every migrated
//     enabled policy disabled it.
//   - sync_job policies are untouched. Afterwards every metrics_template / platform_health
//     policy is set Enabled=false (rows are kept for rollback and diagnosis).
//
// Idempotent: an existing stx_schema_flags flag short-circuits; the flag is claimed inside the
// transaction first so a racing instance rolls back on the primary key (same convention as
// MigrateLegacyRemoteAlerts).
func MigrateAlertRuleCatalog(database *gorm.DB) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}
	if err := database.AutoMigrate(&SchemaFlag{}); err != nil {
		return err
	}
	var flagged int64
	if err := database.Model(&SchemaFlag{}).Where("name = ?", AlertRuleCatalogMigrationName).Count(&flagged).Error; err != nil {
		return err
	}
	if flagged > 0 {
		return nil
	}

	stats := &legacyMigrationStats{}
	err := database.Transaction(func(tx *gorm.DB) error {
		// 先占位写标记：并发启动时后到实例主键冲突并整体回滚。
		// Claim the flag first: a concurrent instance hits the primary key and rolls back.
		if err := tx.Create(&SchemaFlag{
			Name:      AlertRuleCatalogMigrationName,
			Detail:    "fold legacy non-job alert config into cluster rule catalog tables",
			AppliedAt: timex.Now(),
		}).Error; err != nil {
			return err
		}
		if !tx.Migrator().HasTable(&monitoringapp.AlertPolicy{}) {
			return nil
		}
		return runAlertRuleCatalogMigration(tx, stats)
	})
	if err != nil {
		return err
	}

	log.Printf("[Database] 告警模型收拢迁移完成：迁移配置层 %d 层（冲突丢弃策略 %d 条、无法映射 %d 条、降级 warning %d 条、升级后生效 %d 条），旧规则行转例外 %d 条，路由并入默认通知 %d 条 / 写入规则 %d 条，停用旧策略 %d 条"+
		" / alert-model consolidation done: %d config layers written (%d conflicting policies dropped, %d unmapped policies, %d downgraded to warning, %d effective after upgrade), %d legacy rule rows became overrides, %d routes merged into default notification / %d into rules, %d legacy policies disabled\n",
		stats.policiesMigrated, stats.policyConflictsSkipped, stats.policiesUnknownKey, stats.severityDowngraded, stats.deferredRulePolicies,
		stats.ruleRowsMigrated, stats.routesToDefaults, stats.routesToRules, stats.policiesDisabled,
		stats.policiesMigrated, stats.policyConflictsSkipped, stats.policiesUnknownKey, stats.severityDowngraded, stats.deferredRulePolicies,
		stats.ruleRowsMigrated, stats.routesToDefaults, stats.routesToRules, stats.policiesDisabled)
	return nil
}

func runAlertRuleCatalogMigration(tx *gorm.DB, stats *legacyMigrationStats) error {
	enabledChannels, err := loadEnabledAlertChannelIDs(tx)
	if err != nil {
		return err
	}

	var policies []monitoringapp.AlertPolicy
	if err := tx.Where("policy_type IN ?", []string{
		string(monitoringapp.AlertPolicyBuilderKindPlatformHealth),
		string(monitoringapp.AlertPolicyBuilderKindMetricsTemplate),
	}).Find(&policies).Error; err != nil {
		return err
	}

	globalLayers := map[string]*legacyRuleLayer{}
	overrideLayers := map[string]*legacyRuleLayer{}
	overrideClusterIDs := map[string]uint{}
	var migratedEnabledPolicies []*monitoringapp.AlertPolicy
	var allMigratedPolicies []*monitoringapp.AlertPolicy

	// 先按 (ruleKey, cluster) 分组挑出胜出策略，再只把胜出者落成配置层。
	// Group policies by (ruleKey, cluster) to pick a winner first, then materialize only the
	// winner as a config layer.
	groups := map[string][]*monitoringapp.AlertPolicy{}
	for i := range policies {
		policy := &policies[i]
		ruleKey := legacyPolicyRuleKey(policy)
		if _, ok := legacyCatalogRuleKeys[ruleKey]; !ok {
			stats.policiesUnknownKey++
			continue
		}
		clusterID, ok := legacyPolicyClusterID(policy)
		if !ok {
			stats.policiesUnknownKey++
			continue
		}
		allMigratedPolicies = append(allMigratedPolicies, policy)
		if policy.Enabled {
			migratedEnabledPolicies = append(migratedEnabledPolicies, policy)
		}
		if _, ok := legacyDeferredRuleKeys[ruleKey]; ok {
			stats.deferredRulePolicies++
		}
		groupKey := ruleKey + "|" + strconv.FormatUint(uint64(clusterID), 10)
		groups[groupKey] = append(groups[groupKey], policy)
	}

	for groupKey, group := range groups {
		winner := pickLegacyPolicyWinner(group)
		stats.policyConflictsSkipped += len(group) - 1
		ruleKey, _, _ := strings.Cut(groupKey, "|")
		layer := legacyPolicyToRuleLayer(winner, ruleKey, enabledChannels, stats)
		clusterID, _ := strconv.ParseUint(groupKey[strings.Index(groupKey, "|")+1:], 10, 32)
		if clusterID == 0 {
			globalLayers[ruleKey] = layer
		} else {
			overrideLayers[groupKey] = layer
			overrideClusterIDs[groupKey] = uint(clusterID)
		}
	}
	stats.policiesMigrated = len(globalLayers) + len(overrideLayers)

	if err := migrateLegacyAlertRuleRows(tx, overrideLayers, overrideClusterIDs, stats); err != nil {
		return err
	}
	if err := migrateLegacyNotificationRoutes(tx, enabledChannels, globalLayers, overrideLayers, overrideClusterIDs, stats); err != nil {
		return err
	}
	if err := writeAlertRuleCatalogLayers(tx, globalLayers, overrideLayers, overrideClusterIDs); err != nil {
		return err
	}
	if err := writeAlertNotificationDefaultsRow(tx, migratedEnabledPolicies, allMigratedPolicies, stats); err != nil {
		return err
	}

	res := tx.Model(&monitoringapp.AlertPolicy{}).
		Where("policy_type IN ? AND enabled = ?", []string{
			string(monitoringapp.AlertPolicyBuilderKindPlatformHealth),
			string(monitoringapp.AlertPolicyBuilderKindMetricsTemplate),
		}, true).
		Update("enabled", false)
	if res.Error != nil {
		return res.Error
	}
	stats.policiesDisabled = int(res.RowsAffected)
	return nil
}

// pickLegacyPolicyWinner 取组内最近更新的启用策略；全组停用时取最近更新的一条。
// pickLegacyPolicyWinner returns the most recently updated enabled policy of a group, or the
// most recently updated one when the whole group is disabled.
func pickLegacyPolicyWinner(group []*monitoringapp.AlertPolicy) *monitoringapp.AlertPolicy {
	winner := group[0]
	for _, policy := range group[1:] {
		switch {
		case policy.Enabled && !winner.Enabled:
			winner = policy
		case policy.Enabled == winner.Enabled && policy.UpdatedAt.After(winner.UpdatedAt):
			winner = policy
		}
	}
	return winner
}

// loadEnabledAlertChannelIDs 返回启用中的通知渠道集合，用于级别判定与停用路由跳过。
// loadEnabledAlertChannelIDs returns the set of enabled notification channels, used for
// severity decisions and for skipping routes on disabled channels.
func loadEnabledAlertChannelIDs(tx *gorm.DB) (map[uint]bool, error) {
	enabled := map[uint]bool{}
	if !tx.Migrator().HasTable(&monitoringapp.NotificationChannel{}) {
		return enabled, nil
	}
	var channels []monitoringapp.NotificationChannel
	if err := tx.Where("enabled = ?", true).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		enabled[channel.ID] = true
	}
	return enabled, nil
}

// legacyPolicyRuleKey 解析策略对应的目录规则键：platform_health 策略记 legacy_rule_key，
// metrics_template 策略记 template_key，两者都落到目录键上。
// legacyPolicyRuleKey resolves the catalog key of a policy: platform_health policies carry
// legacy_rule_key and metrics_template policies carry template_key, both landing on catalog keys.
func legacyPolicyRuleKey(policy *monitoringapp.AlertPolicy) string {
	if key := strings.TrimSpace(policy.LegacyRuleKey); key != "" {
		return key
	}
	return strings.TrimSpace(policy.TemplateKey)
}

// legacyPolicyClusterID 解析策略的集群范围：空串与 "all" 表示全局（返回 0），数字串为指定
// 集群；无法解析的值返回 ok=false，调用方记摘要后跳过。
// legacyPolicyClusterID parses the policy cluster scope: "" and "all" mean global (returns 0),
// a numeric string is one cluster; unparseable values return ok=false and the caller logs a skip.
func legacyPolicyClusterID(policy *monitoringapp.AlertPolicy) (uint, bool) {
	raw := strings.TrimSpace(policy.ClusterID)
	if raw == "" || strings.EqualFold(raw, "all") {
		return 0, true
	}
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || parsed == 0 {
		return 0, false
	}
	return uint(parsed), true
}

// legacyPolicyToRuleLayer 把胜出策略转成配置层：阈值按规则单位换算（percent 规则 ratio≤1
// → ×100），持续时间取第一条条件的 window_minutes；旧模型里启用 + 渠道即外投，所以有
// 启用渠道 → critical、否则 → warning，因无启用渠道被降为 warning 的记摘要。
// legacyPolicyToRuleLayer converts the winning policy into a config layer: the threshold is
// scaled per rule unit (percent rules ratio≤1 → ×100) and the duration takes the first
// condition's window_minutes; in the legacy model enabled+channels meant external delivery, so
// an enabled channel maps to critical, anything else to warning with a downgrade logged.
func legacyPolicyToRuleLayer(policy *monitoringapp.AlertPolicy, ruleKey string, enabledChannels map[uint]bool, stats *legacyMigrationStats) *legacyRuleLayer {
	layer := &legacyRuleLayer{
		enabled: boolPtr(policy.Enabled),
	}
	channelIDs := unmarshalLegacyUintIDs(policy.NotificationChannelIDsJSON)
	receiverIDs := unmarshalLegacyUint64IDs(policy.ReceiverUserIDsJSON)
	if len(channelIDs) > 0 || len(receiverIDs) > 0 {
		layer.notifyMode = "custom"
		layer.channelIDsJSON = marshalLegacyUintIDs(channelIDs)
		layer.receiverUserIDsJSON = marshalLegacyUint64IDs(receiverIDs)
	}
	hasEnabledChannel := false
	for _, id := range channelIDs {
		if enabledChannels[id] {
			hasEnabledChannel = true
			break
		}
	}
	severity := string(monitoringapp.AlertSeverityWarning)
	if hasEnabledChannel {
		severity = string(monitoringapp.AlertSeverityCritical)
	} else if policy.Severity == monitoringapp.AlertSeverityCritical || len(channelIDs) > 0 {
		stats.severityDowngraded++
	}
	layer.severity = &severity

	for _, condition := range unmarshalLegacyConditions(policy.ConditionsJSON) {
		if layer.threshold == nil {
			if _, ok := legacyThresholdRules[ruleKey]; ok {
				if value, err := strconv.ParseFloat(strings.TrimSpace(condition.Threshold), 64); err == nil {
					if _, percent := legacyPercentUnitRules[ruleKey]; percent && value <= 1 {
						value *= 100
					}
					layer.threshold = float64Ptr(value)
				}
			}
		}
		if layer.forSeconds == nil && condition.WindowMinutes > 0 {
			layer.forSeconds = intPtr(condition.WindowMinutes * 60)
		}
	}
	return layer
}

// migrateLegacyAlertRuleRows 把 monitoring_alert_rules 中偏离种子默认的行转成集群例外：
// 只补策略迁移未写的字段（策略是更新的配置面，冲突时策略优先）。
// migrateLegacyAlertRuleRows turns monitoring_alert_rules rows deviating from the seed
// defaults into cluster overrides: they only fill fields the policy migration left unset
// (policies were the newer configuration surface and win on conflict).
func migrateLegacyAlertRuleRows(tx *gorm.DB, overrideLayers map[string]*legacyRuleLayer, overrideClusterIDs map[string]uint, stats *legacyMigrationStats) error {
	if !tx.Migrator().HasTable(&monitoringapp.AlertRule{}) {
		return nil
	}
	var rows []monitoringapp.AlertRule
	if err := tx.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		seed, ok := legacyAlertRuleSeeds[row.RuleKey]
		if !ok {
			stats.ruleRowsSkipped++
			continue
		}
		if row.Enabled == seed.Enabled && row.WindowSeconds == seed.WindowSeconds {
			continue
		}
		groupKey := legacyOverrideKey(row.RuleKey, row.ClusterID)
		layer, exists := overrideLayers[groupKey]
		if !exists {
			layer = &legacyRuleLayer{}
			overrideLayers[groupKey] = layer
			overrideClusterIDs[groupKey] = row.ClusterID
		}
		if layer.enabled == nil {
			layer.enabled = boolPtr(row.Enabled)
		}
		if layer.forSeconds == nil {
			layer.forSeconds = intPtr(row.WindowSeconds)
		}
		stats.ruleRowsMigrated++
	}
	return nil
}

// migrateLegacyNotificationRoutes 按 design §10 处理启用路由：rule_key 过滤写规则层
// custom 渠道（带 cluster_id 时写集群例外），severity 过滤与仅 cluster 过滤无法归属单条
// 规则的渠道并入默认通知；无过滤或仅 source_type 过滤的也并入默认通知。
// migrateLegacyNotificationRoutes handles enabled routes per design §10: a rule_key filter
// writes the channel into that rule's layer as a custom channel (a cluster_id additionally
// targets the cluster override); severity-filtered and cluster-only routes cannot be
// attributed to one rule so their channels merge into the default notification, as do
// unfiltered or source_type-only routes.
func migrateLegacyNotificationRoutes(tx *gorm.DB, enabledChannels map[uint]bool, globalLayers map[string]*legacyRuleLayer, overrideLayers map[string]*legacyRuleLayer, overrideClusterIDs map[string]uint, stats *legacyMigrationStats) error {
	if !tx.Migrator().HasTable(&monitoringapp.NotificationRoute{}) {
		return nil
	}
	var routes []monitoringapp.NotificationRoute
	if err := tx.Find(&routes).Error; err != nil {
		return err
	}
	for _, route := range routes {
		if !route.Enabled {
			stats.routesSkipped++
			continue
		}
		if !enabledChannels[route.ChannelID] {
			stats.routesSkipped++
			continue
		}
		ruleKey := strings.TrimSpace(route.RuleKey)
		clusterRaw := strings.TrimSpace(route.ClusterID)
		severity := strings.TrimSpace(route.Severity)
		clusterID := uint(0)
		clusterOK := true
		if clusterRaw != "" && !strings.EqualFold(clusterRaw, "all") {
			parsed, err := strconv.ParseUint(clusterRaw, 10, 32)
			if err != nil || parsed == 0 {
				clusterOK = false
			} else {
				clusterID = uint(parsed)
			}
		}
		switch {
		case ruleKey != "" && severity == "" && clusterOK:
			if _, ok := legacyCatalogRuleKeys[ruleKey]; !ok {
				// rule_key 不在目录里的路由没有归属，渠道仍并入默认通知保住投递。
				// A route whose rule_key is outside the catalog cannot be attributed; its
				// channel still merges into the default notification to keep delivery.
				stats.defaultChannels = append(stats.defaultChannels, route.ChannelID)
				stats.routesToDefaults++
				continue
			}
			layer := legacyRuleLayerForRoute(ruleKey, clusterID, globalLayers, overrideLayers, overrideClusterIDs)
			mergeLegacyRouteChannel(layer, route.ChannelID)
			stats.routesToRules++
		default:
			// severity 过滤、仅 cluster 过滤与无过滤 / 仅 source_type 过滤都落默认通知。
			// Severity-filtered, cluster-only, unfiltered and source_type-only routes all
			// land in the default notification.
			stats.defaultChannels = append(stats.defaultChannels, route.ChannelID)
			stats.routesToDefaults++
		}
	}
	return nil
}

// legacyRuleLayerForRoute 取路由归属的配置层：rule_key + cluster → 集群例外层（不存在则建
// 空层），rule_key only → 全局层；并在层上标 custom 通知。
// legacyRuleLayerForRoute resolves the config layer a route belongs to: rule_key + cluster
// targets the cluster override (an empty layer is created when missing), rule_key alone targets
// the global layer; the layer is marked as custom notification.
func legacyRuleLayerForRoute(ruleKey string, clusterID uint, globalLayers map[string]*legacyRuleLayer, overrideLayers map[string]*legacyRuleLayer, overrideClusterIDs map[string]uint) *legacyRuleLayer {
	if clusterID == 0 {
		layer, ok := globalLayers[ruleKey]
		if !ok {
			layer = &legacyRuleLayer{}
			globalLayers[ruleKey] = layer
		}
		return layer
	}
	groupKey := legacyOverrideKey(ruleKey, clusterID)
	layer, ok := overrideLayers[groupKey]
	if !ok {
		layer = &legacyRuleLayer{}
		overrideLayers[groupKey] = layer
		overrideClusterIDs[groupKey] = clusterID
	}
	return layer
}

// mergeLegacyRouteChannel 把路由渠道并入配置层：层上已有 custom 渠道则去重合并，
// 否则把层切成 custom 并写入该渠道。
// mergeLegacyRouteChannel merges a route channel into a config layer: existing custom channels
// dedupe-merge, otherwise the layer switches to custom with this channel.
func mergeLegacyRouteChannel(layer *legacyRuleLayer, channelID uint) {
	channelIDs := unmarshalLegacyUintIDs(layer.channelIDsJSON)
	channelIDs = append(channelIDs, channelID)
	layer.notifyMode = "custom"
	layer.channelIDsJSON = marshalLegacyUintIDs(channelIDs)
}

// writeAlertRuleCatalogLayers 把全局层与例外层落库：已有行按字段更新（nil 保留原值），
// 没有则插入；幂等由外层 flag 保证，这里再做幂等写防御重复执行的部分提交。
// writeAlertRuleCatalogLayers persists global and override layers: existing rows update
// field-wise (nil keeps the stored value), missing rows insert; the outer flag provides
// idempotence, and the field-wise writes additionally defend against partial re-application.
func writeAlertRuleCatalogLayers(tx *gorm.DB, globalLayers map[string]*legacyRuleLayer, overrideLayers map[string]*legacyRuleLayer, overrideClusterIDs map[string]uint) error {
	for ruleKey, layer := range globalLayers {
		var existing monitoringapp.ClusterRuleConfig
		err := tx.Where("rule_key = ?", ruleKey).First(&existing).Error
		switch {
		case err == nil:
			applyLegacyRuleLayerConfig(&existing, layer)
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
		case err == gorm.ErrRecordNotFound:
			row := monitoringapp.ClusterRuleConfig{RuleKey: ruleKey}
			applyLegacyRuleLayerConfig(&row, layer)
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}
	for groupKey, layer := range overrideLayers {
		ruleKey, _, _ := strings.Cut(groupKey, "|")
		clusterID := overrideClusterIDs[groupKey]
		var existing monitoringapp.ClusterRuleOverride
		err := tx.Where("rule_key = ? AND cluster_id = ?", ruleKey, clusterID).First(&existing).Error
		switch {
		case err == nil:
			applyLegacyRuleLayerOverride(&existing, layer)
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
		case err == gorm.ErrRecordNotFound:
			row := monitoringapp.ClusterRuleOverride{RuleKey: ruleKey, ClusterID: clusterID}
			applyLegacyRuleLayerOverride(&row, layer)
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

func applyLegacyRuleLayerConfig(row *monitoringapp.ClusterRuleConfig, layer *legacyRuleLayer) {
	row.Enabled = layer.enabled
	row.Threshold = layer.threshold
	row.ForSeconds = layer.forSeconds
	row.Severity = layer.severity
	row.NotifyMode = layer.notifyMode
	row.ChannelIDsJSON = layer.channelIDsJSON
	row.ReceiverUserIDsJSON = layer.receiverUserIDsJSON
}

func applyLegacyRuleLayerOverride(row *monitoringapp.ClusterRuleOverride, layer *legacyRuleLayer) {
	row.Enabled = layer.enabled
	row.Threshold = layer.threshold
	row.ForSeconds = layer.forSeconds
	row.Severity = layer.severity
	row.NotifyMode = layer.notifyMode
	row.ChannelIDsJSON = layer.channelIDsJSON
	row.ReceiverUserIDsJSON = layer.receiverUserIDsJSON
}

// writeAlertNotificationDefaultsRow 写单行默认通知：路由并入的渠道 + 仍为空时的策略并集
// 兜底；cooldown 取迁移启用策略最大值，send_recovery 仅当全部启用迁移策略关闭时才关。
// writeAlertNotificationDefaultsRow writes the single default-notification row: channels merged
// from routes plus the all-policy union fallback when still empty; cooldown takes the max of
// migrated enabled policies and send_recovery turns off only when every migrated enabled
// policy disabled it.
func writeAlertNotificationDefaultsRow(tx *gorm.DB, migratedEnabledPolicies, allMigratedPolicies []*monitoringapp.AlertPolicy, stats *legacyMigrationStats) error {
	channelIDs := stats.defaultChannels
	receiverIDs := stats.defaultReceiverIDs
	if len(channelIDs) == 0 && len(receiverIDs) == 0 && len(allMigratedPolicies) > 0 {
		for _, policy := range allMigratedPolicies {
			channelIDs = append(channelIDs, unmarshalLegacyUintIDs(policy.NotificationChannelIDsJSON)...)
			receiverIDs = append(receiverIDs, unmarshalLegacyUint64IDs(policy.ReceiverUserIDsJSON)...)
		}
		stats.defaultsFromFallback = len(channelIDs) > 0 || len(receiverIDs) > 0
	}
	if len(channelIDs) == 0 && len(receiverIDs) == 0 {
		return nil
	}

	cooldown := 0
	for _, policy := range migratedEnabledPolicies {
		if policy.CooldownMinutes > cooldown {
			cooldown = policy.CooldownMinutes
		}
	}
	sendRecovery := true
	if len(migratedEnabledPolicies) > 0 {
		for _, policy := range migratedEnabledPolicies {
			if !policy.SendRecovery {
				sendRecovery = false
			}
		}
	}

	row := monitoringapp.AlertNotificationDefaults{
		ChannelIDsJSON:      marshalLegacyUintIDs(channelIDs),
		ReceiverUserIDsJSON: marshalLegacyUint64IDs(receiverIDs),
		CooldownMinutes:     cooldown,
		SendRecovery:        sendRecovery,
	}
	var existing monitoringapp.AlertNotificationDefaults
	err := tx.Order("id").First(&existing).Error
	switch {
	case err == nil:
		existing.ChannelIDsJSON = row.ChannelIDsJSON
		existing.ReceiverUserIDsJSON = row.ReceiverUserIDsJSON
		existing.CooldownMinutes = row.CooldownMinutes
		existing.SendRecovery = row.SendRecovery
		return tx.Save(&existing).Error
	case err == gorm.ErrRecordNotFound:
		return tx.Create(&row).Error
	default:
		return err
	}
}

func legacyOverrideKey(ruleKey string, clusterID uint) string {
	return ruleKey + "|" + strconv.FormatUint(uint64(clusterID), 10)
}

func unmarshalLegacyConditions(raw string) []legacyPolicyCondition {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var conditions []legacyPolicyCondition
	if err := json.Unmarshal([]byte(trimmed), &conditions); err != nil {
		return nil
	}
	return conditions
}

func unmarshalLegacyUintIDs(raw string) []uint {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return nil
	}
	return ids
}

func unmarshalLegacyUint64IDs(raw string) []uint64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var ids []uint64
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return nil
	}
	return ids
}

func marshalLegacyUintIDs(ids []uint) string {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	seen := map[uint]bool{}
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	payload, _ := json.Marshal(unique)
	return string(payload)
}

func marshalLegacyUint64IDs(ids []uint64) string {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	seen := map[uint64]bool{}
	unique := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	payload, _ := json.Marshal(unique)
	return string(payload)
}

func boolPtr(v bool) *bool          { return &v }
func intPtr(v int) *int             { return &v }
func float64Ptr(v float64) *float64 { return &v }
