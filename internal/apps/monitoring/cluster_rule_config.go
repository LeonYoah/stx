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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ClusterRuleConfig 是某条集群级规则的全局配置行（monitoring_cluster_rule_configs），
// 每规则至多一行；指针字段为 nil 表示沿用目录默认值。
// ClusterRuleConfig is the global config row of one cluster rule
// (monitoring_cluster_rule_configs), at most one row per rule; nil pointer fields inherit the
// catalog default.
type ClusterRuleConfig struct {
	ID                  uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	RuleKey             string    `json:"rule_key" gorm:"size:120;not null;uniqueIndex"`
	Enabled             *bool     `json:"enabled"`
	Threshold           *float64  `json:"threshold"`
	ForSeconds          *int      `json:"for_seconds"`
	Severity            *string   `json:"severity" gorm:"size:20"`
	NotifyMode          string    `json:"notify_mode" gorm:"size:20"`
	ChannelIDsJSON      string    `json:"channel_ids_json" gorm:"type:text"`
	ReceiverUserIDsJSON string    `json:"receiver_user_ids_json" gorm:"type:text"`
	CreatedAt           time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt           time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for ClusterRuleConfig.
// TableName 指定 ClusterRuleConfig 表名。
func (ClusterRuleConfig) TableName() string {
	return "monitoring_cluster_rule_configs"
}

// ClusterRuleOverride 是某条集群级规则在单个集群上的例外配置
// （monitoring_cluster_rule_overrides）；指针字段为 nil 表示继承上一层（全局配置或目录默认）。
// ClusterRuleOverride is one cluster exception of a cluster rule
// (monitoring_cluster_rule_overrides); nil pointer fields inherit the layer above (global
// config or catalog default).
type ClusterRuleOverride struct {
	ID                  uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	RuleKey             string    `json:"rule_key" gorm:"size:120;not null;uniqueIndex:ux_monitoring_cluster_rule_override"`
	ClusterID           uint      `json:"cluster_id" gorm:"not null;uniqueIndex:ux_monitoring_cluster_rule_override"`
	Enabled             *bool     `json:"enabled"`
	Threshold           *float64  `json:"threshold"`
	ForSeconds          *int      `json:"for_seconds"`
	Severity            *string   `json:"severity" gorm:"size:20"`
	NotifyMode          string    `json:"notify_mode" gorm:"size:20"`
	ChannelIDsJSON      string    `json:"channel_ids_json" gorm:"type:text"`
	ReceiverUserIDsJSON string    `json:"receiver_user_ids_json" gorm:"type:text"`
	CreatedAt           time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt           time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for ClusterRuleOverride.
// TableName 指定 ClusterRuleOverride 表名。
func (ClusterRuleOverride) TableName() string {
	return "monitoring_cluster_rule_overrides"
}

// AlertNotificationDefaults 是单行默认通知（monitoring_alert_notification_defaults）：
// 通知方式为 "" / "default" 的集群级规则共用这份渠道、接收人、冷却与恢复开关。
// AlertNotificationDefaults is the single-row default notification
// (monitoring_alert_notification_defaults): cluster rules whose notify mode is "" / "default"
// share these channels, receivers, cooldown and recovery switch.
type AlertNotificationDefaults struct {
	ID                  uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	ChannelIDsJSON      string    `json:"channel_ids_json" gorm:"type:text"`
	ReceiverUserIDsJSON string    `json:"receiver_user_ids_json" gorm:"type:text"`
	CooldownMinutes     int       `json:"cooldown_minutes"`
	SendRecovery        bool      `json:"send_recovery"`
	UpdatedAt           time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for AlertNotificationDefaults.
// TableName 指定 AlertNotificationDefaults 表名。
func (AlertNotificationDefaults) TableName() string {
	return "monitoring_alert_notification_defaults"
}

// clusterRuleFields 是一层规则配置（全局行或集群例外行都可归约到它），指针字段为 nil 表示继承。
// clusterRuleFields is one configuration layer (both the global row and a cluster override row
// reduce to it); nil pointer fields mean inherit.
type clusterRuleFields struct {
	Enabled             *bool
	Threshold           *float64
	ForSeconds          *int
	Severity            *string
	NotifyMode          string
	ChannelIDsJSON      string
	ReceiverUserIDsJSON string
}

func (c *ClusterRuleConfig) fields() *clusterRuleFields {
	if c == nil {
		return nil
	}
	return &clusterRuleFields{
		Enabled:             c.Enabled,
		Threshold:           c.Threshold,
		ForSeconds:          c.ForSeconds,
		Severity:            c.Severity,
		NotifyMode:          strings.TrimSpace(c.NotifyMode),
		ChannelIDsJSON:      c.ChannelIDsJSON,
		ReceiverUserIDsJSON: c.ReceiverUserIDsJSON,
	}
}

func (o *ClusterRuleOverride) fields() *clusterRuleFields {
	if o == nil {
		return nil
	}
	return &clusterRuleFields{
		Enabled:             o.Enabled,
		Threshold:           o.Threshold,
		ForSeconds:          o.ForSeconds,
		Severity:            o.Severity,
		NotifyMode:          strings.TrimSpace(o.NotifyMode),
		ChannelIDsJSON:      o.ChannelIDsJSON,
		ReceiverUserIDsJSON: o.ReceiverUserIDsJSON,
	}
}

// resolvedRuleConfig 是目录默认 → 全局配置 → 集群例外逐字段覆盖后的生效配置，
// 所有判定与投递只读这个结构。
// resolvedRuleConfig is the effective config after per-field layering of catalog default →
// global config → cluster override; every evaluation and delivery path reads only this shape.
type resolvedRuleConfig struct {
	RuleKey         string
	ClusterID       uint
	Definition      *ClusterRuleDefinition
	Enabled         bool
	Threshold       *float64
	ForSeconds      int
	Severity        AlertSeverity
	NotifyMode      string
	ChannelIDs      []uint
	ReceiverUserIDs []uint64
	CooldownMinutes int
	SendRecovery    bool
}

// clusterRuleCacheTTL 是解析结果的进程内缓存时长；测试可设为 0 关闭。
// clusterRuleCacheTTL is the in-process cache TTL of resolved configs; tests may set it to 0.
var clusterRuleCacheTTL = 3 * time.Second

type clusterRuleCacheEntry struct {
	cfg       *resolvedRuleConfig
	expiresAt time.Time
}

// clusterRuleLayers 一次装载三层配置，供单点解析与规则组构建共用。
// clusterRuleLayers loads the three configuration layers at once, shared by single-point
// resolution and managed rule-group building.
type clusterRuleLayers struct {
	globals   map[string]*clusterRuleFields
	overrides map[string]map[uint]*clusterRuleFields
	defaults  *AlertNotificationDefaults
}

func (l *clusterRuleLayers) global(key string) *clusterRuleFields {
	if l == nil {
		return nil
	}
	return l.globals[key]
}

func (l *clusterRuleLayers) override(key string, clusterID uint) *clusterRuleFields {
	if l == nil || clusterID == 0 {
		return nil
	}
	if byCluster, ok := l.overrides[key]; ok {
		return byCluster[clusterID]
	}
	return nil
}

func (l *clusterRuleLayers) overrideClusterIDs(key string) []uint {
	if l == nil {
		return nil
	}
	byCluster, ok := l.overrides[key]
	if !ok || len(byCluster) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(byCluster))
	for clusterID := range byCluster {
		ids = append(ids, clusterID)
	}
	sortUintSlice(ids)
	return ids
}

func sortUintSlice(values []uint) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// GetClusterRuleConfig 返回一条规则的全局配置；不存在时返回 (nil, nil)。
// GetClusterRuleConfig returns the global config row of one rule; (nil, nil) when absent.
func (r *Repository) GetClusterRuleConfig(ctx context.Context, ruleKey string) (*ClusterRuleConfig, error) {
	var row ClusterRuleConfig
	err := r.db.WithContext(ctx).Where("rule_key = ?", strings.TrimSpace(ruleKey)).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ListClusterRuleConfigs 返回全部规则全局配置行。
// ListClusterRuleConfigs returns every global rule config row.
func (r *Repository) ListClusterRuleConfigs(ctx context.Context) ([]*ClusterRuleConfig, error) {
	var rows []*ClusterRuleConfig
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpsertClusterRuleConfig 按 rule_key 幂等写入全局配置。
// UpsertClusterRuleConfig upserts the global config row keyed by rule_key.
func (r *Repository) UpsertClusterRuleConfig(ctx context.Context, row *ClusterRuleConfig) error {
	if row == nil {
		return fmt.Errorf("cluster rule config is required")
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "rule_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"enabled", "threshold", "for_seconds", "severity",
			"notify_mode", "channel_ids_json", "receiver_user_ids_json", "updated_at",
		}),
	}).Create(row).Error
}

// DeleteClusterRuleConfig 删除一条规则的全局配置（恢复目录默认）。
// DeleteClusterRuleConfig removes the global config row of one rule (back to catalog defaults).
func (r *Repository) DeleteClusterRuleConfig(ctx context.Context, ruleKey string) error {
	return r.db.WithContext(ctx).Where("rule_key = ?", strings.TrimSpace(ruleKey)).Delete(&ClusterRuleConfig{}).Error
}

// GetClusterRuleOverride 返回某集群对一条规则的例外；不存在时返回 (nil, nil)。
// GetClusterRuleOverride returns one cluster's override of a rule; (nil, nil) when absent.
func (r *Repository) GetClusterRuleOverride(ctx context.Context, ruleKey string, clusterID uint) (*ClusterRuleOverride, error) {
	var row ClusterRuleOverride
	err := r.db.WithContext(ctx).
		Where("rule_key = ? AND cluster_id = ?", strings.TrimSpace(ruleKey), clusterID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ListClusterRuleOverrides 返回一条规则的全部集群例外。
// ListClusterRuleOverrides returns every cluster override of one rule.
func (r *Repository) ListClusterRuleOverrides(ctx context.Context, ruleKey string) ([]*ClusterRuleOverride, error) {
	var rows []*ClusterRuleOverride
	if err := r.db.WithContext(ctx).
		Where("rule_key = ?", strings.TrimSpace(ruleKey)).
		Order("cluster_id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListAllClusterRuleOverrides 返回全部集群例外行（托管 TSDB 规则组构建用）。
// ListAllClusterRuleOverrides returns every cluster override row (used by managed tsdb
// rule-group builds).
func (r *Repository) ListAllClusterRuleOverrides(ctx context.Context) ([]*ClusterRuleOverride, error) {
	var rows []*ClusterRuleOverride
	if err := r.db.WithContext(ctx).Order("rule_key ASC, cluster_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpsertClusterRuleOverride 按 (rule_key, cluster_id) 幂等写入集群例外。
// UpsertClusterRuleOverride upserts the override row keyed by (rule_key, cluster_id).
func (r *Repository) UpsertClusterRuleOverride(ctx context.Context, row *ClusterRuleOverride) error {
	if row == nil {
		return fmt.Errorf("cluster rule override is required")
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "rule_key"}, {Name: "cluster_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"enabled", "threshold", "for_seconds", "severity",
			"notify_mode", "channel_ids_json", "receiver_user_ids_json", "updated_at",
		}),
	}).Create(row).Error
}

// DeleteClusterRuleOverride 删除某集群对一条规则的例外。
// DeleteClusterRuleOverride removes one cluster's override of a rule.
func (r *Repository) DeleteClusterRuleOverride(ctx context.Context, ruleKey string, clusterID uint) error {
	return r.db.WithContext(ctx).
		Where("rule_key = ? AND cluster_id = ?", strings.TrimSpace(ruleKey), clusterID).
		Delete(&ClusterRuleOverride{}).Error
}

// GetAlertNotificationDefaults 返回单行默认通知；未配置时返回 (nil, nil)。
// GetAlertNotificationDefaults returns the single default-notification row; (nil, nil) when
// unconfigured.
func (r *Repository) GetAlertNotificationDefaults(ctx context.Context) (*AlertNotificationDefaults, error) {
	var row AlertNotificationDefaults
	err := r.db.WithContext(ctx).Order("id ASC").First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// SaveAlertNotificationDefaults 写入单行默认通知（首行更新，否则创建）。
// SaveAlertNotificationDefaults writes the single default-notification row (updates the first
// row or creates one).
func (r *Repository) SaveAlertNotificationDefaults(ctx context.Context, row *AlertNotificationDefaults) error {
	if row == nil {
		return fmt.Errorf("alert notification defaults are required")
	}
	if row.ID == 0 {
		existing, err := r.GetAlertNotificationDefaults(ctx)
		if err != nil {
			return err
		}
		if existing != nil {
			row.ID = existing.ID
		}
	}
	return r.db.WithContext(ctx).Save(row).Error
}

// loadClusterRuleLayers 一次读出三张表的当前内容；规则组构建必须直读（不走缓存），保证重建用的是最新写入。
// loadClusterRuleLayers reads the three tables at once; rule-group builds must read directly
// (bypassing the cache) so a rebuild sees the freshest writes.
func (s *Service) loadClusterRuleLayers(ctx context.Context) (*clusterRuleLayers, error) {
	layers := &clusterRuleLayers{
		globals:   map[string]*clusterRuleFields{},
		overrides: map[string]map[uint]*clusterRuleFields{},
	}
	configs, err := s.repo.ListClusterRuleConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range configs {
		if row == nil {
			continue
		}
		layers.globals[strings.TrimSpace(row.RuleKey)] = row.fields()
	}
	overrides, err := s.repo.ListAllClusterRuleOverrides(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range overrides {
		if row == nil {
			continue
		}
		key := strings.TrimSpace(row.RuleKey)
		if _, ok := layers.overrides[key]; !ok {
			layers.overrides[key] = map[uint]*clusterRuleFields{}
		}
		layers.overrides[key][row.ClusterID] = row.fields()
	}
	defaults, err := s.repo.GetAlertNotificationDefaults(ctx)
	if err != nil {
		return nil, err
	}
	layers.defaults = defaults
	return layers, nil
}

// resolveClusterRuleFromLayers 在已装载的三层配置上解析单条规则（不含缓存与 DB 读取）。
// resolveClusterRuleFromLayers resolves one rule over already-loaded layers (no cache, no DB).
func resolveClusterRuleFromLayers(def *ClusterRuleDefinition, layers *clusterRuleLayers, clusterID uint) *resolvedRuleConfig {
	if def == nil {
		return nil
	}
	cfg := &resolvedRuleConfig{
		RuleKey:    def.Key,
		ClusterID:  clusterID,
		Definition: def,
		Enabled:    def.DefaultEnabled,
		ForSeconds: int(def.DefaultFor / time.Second),
		Severity:   def.Severity,
	}
	if def.Threshold != nil && def.Threshold.Unit != clusterRuleThresholdUnitWorkers {
		threshold := def.Threshold.Default
		cfg.Threshold = &threshold
	}
	apply := func(fields *clusterRuleFields) {
		if fields == nil {
			return
		}
		if fields.Enabled != nil {
			cfg.Enabled = *fields.Enabled
		}
		if fields.Threshold != nil {
			threshold := *fields.Threshold
			cfg.Threshold = &threshold
		}
		if fields.ForSeconds != nil {
			cfg.ForSeconds = *fields.ForSeconds
		}
		if fields.Severity != nil && strings.TrimSpace(*fields.Severity) != "" {
			cfg.Severity = normalizeAlertPolicySeverity(AlertSeverity(strings.TrimSpace(*fields.Severity)))
		}
	}
	apply(layers.global(def.Key))
	apply(layers.override(def.Key, clusterID))

	// 通知方式整体成组覆盖：提供非空 notify_mode 的那一层同时接管渠道与接收人；
	// 某层只写渠道不写 mode 不会改变生效方式。
	// The notify mode takes over as a unit: the layer that provides a non-empty notify_mode also
	// owns channels and receivers; a layer that only stores channel ids without a mode never
	// changes the effective mode.
	notifySource := layers.global(def.Key)
	if override := layers.override(def.Key, clusterID); override != nil && override.NotifyMode != "" {
		notifySource = override
	}
	cfg.NotifyMode = strings.TrimSpace(notifySourceMode(notifySource))
	switch cfg.NotifyMode {
	case clusterRuleNotifyModeCustom:
		cfg.ChannelIDs = unmarshalAlertPolicyChannelIDs(notifySource.ChannelIDsJSON)
		cfg.ReceiverUserIDs = unmarshalAlertPolicyReceiverUserIDs(notifySource.ReceiverUserIDsJSON)
	case clusterRuleNotifyModeNone:
	default:
		cfg.NotifyMode = clusterRuleNotifyModeDefault
	}
	resolveRuleNotification(cfg, layers.defaults)
	return cfg
}

func notifySourceMode(fields *clusterRuleFields) string {
	if fields == nil {
		return ""
	}
	return fields.NotifyMode
}

// resolveRuleNotification 按生效通知方式解析实际投递的渠道与接收人，并套用默认通知的冷却 / 恢复开关。
// resolveRuleNotification resolves the actual delivery channels and receivers from the effective
// notify mode, and applies the cooldown / recovery switches from the default notification.
func resolveRuleNotification(cfg *resolvedRuleConfig, defaults *AlertNotificationDefaults) {
	if cfg == nil {
		return
	}
	switch cfg.NotifyMode {
	case clusterRuleNotifyModeCustom:
		// 渠道与接收人已在逐字段覆盖时取自提供 custom 的那一层，这里不动。
		// Channels and receivers were already taken from the layer that provided custom; nothing
		// to do here.
	case clusterRuleNotifyModeNone:
		cfg.ChannelIDs = nil
		cfg.ReceiverUserIDs = nil
	default:
		if defaults != nil {
			cfg.ChannelIDs = unmarshalAlertPolicyChannelIDs(defaults.ChannelIDsJSON)
			cfg.ReceiverUserIDs = unmarshalAlertPolicyReceiverUserIDs(defaults.ReceiverUserIDsJSON)
		} else {
			cfg.ChannelIDs = nil
			cfg.ReceiverUserIDs = nil
		}
	}
	if defaults != nil {
		cfg.CooldownMinutes = defaults.CooldownMinutes
		cfg.SendRecovery = defaults.SendRecovery
	} else {
		cfg.CooldownMinutes = 0
		cfg.SendRecovery = true
	}
}

// resolveRuleSeverity 归一化规则的生效级别（缺省 / 未知一律 warning）。
// resolveRuleSeverity normalizes the effective rule severity (missing / unknown always warning).
func resolveRuleSeverity(cfg *resolvedRuleConfig) AlertSeverity {
	if cfg == nil {
		return AlertSeverityWarning
	}
	return normalizeAlertPolicySeverity(cfg.Severity)
}

// deliversExternally 判断规则当前是否具备外投条件：启用 + critical + 生效通知里至少一条渠道。
// deliversExternally reports whether the rule can deliver externally right now: enabled +
// critical + at least one channel in the effective notification.
func (c *resolvedRuleConfig) deliversExternally() bool {
	if c == nil || !c.Enabled {
		return false
	}
	return resolveRuleSeverity(c) == AlertSeverityCritical && len(c.ChannelIDs) > 0
}

// deliveryPolicy 把解析结果包装成 AlertPolicy 载体，复用既有投递缝（去重键、冷却提醒、
// 静默 / 关闭、恢复前置、失败重试）。ID 恒为 0：NotificationDelivery.PolicyID 不再写入，
// 策略执行统计的 ApplyAlertPolicyExecutionState 对 0 也是 no-op。
// deliveryPolicy wraps the resolved config as an AlertPolicy carrier so the existing delivery
// seam (dedupe keys, cooldown reminders, silence / close, recovery precondition, backoff
// retry) is reused. ID is always 0: NotificationDelivery.PolicyID is no longer written and the
// execution-stats ApplyAlertPolicyExecutionState is a no-op for 0.
func (c *resolvedRuleConfig) deliveryPolicy() *AlertPolicy {
	if c == nil {
		return nil
	}
	channelIDsJSON, _ := marshalAlertPolicyChannelIDs(c.ChannelIDs)
	receiverUserIDsJSON, _ := marshalAlertPolicyReceiverUserIDs(c.ReceiverUserIDs)
	name := c.RuleKey
	if c.Definition != nil && strings.TrimSpace(c.Definition.Name) != "" {
		name = strings.TrimSpace(c.Definition.Name)
	}
	return &AlertPolicy{
		Name:                       name,
		PolicyType:                 AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:                c.RuleKey,
		LegacyRuleKey:              c.RuleKey,
		ClusterID:                  clusterRuleScopeClusterID(c.ClusterID),
		Severity:                   resolveRuleSeverity(c),
		Enabled:                    c.Enabled,
		CooldownMinutes:            c.CooldownMinutes,
		SendRecovery:               c.SendRecovery,
		NotificationChannelIDsJSON: channelIDsJSON,
		ReceiverUserIDsJSON:        receiverUserIDsJSON,
	}
}

// deliveryPolicies 把解析结果包装成投递策略切片；不外投（未启用 / warning / 无渠道）时为空。
// deliveryPolicies wraps the resolved config as a delivery policy slice; empty when the rule
// must not deliver externally (disabled / warning / no channel).
func (c *resolvedRuleConfig) deliveryPolicies() []*AlertPolicy {
	if !c.deliversExternally() {
		return nil
	}
	return []*AlertPolicy{c.deliveryPolicy()}
}

func clusterRuleScopeClusterID(clusterID uint) string {
	if clusterID == 0 {
		return "all"
	}
	return strconv.FormatUint(uint64(clusterID), 10)
}

// ResolveClusterRule 是规则生效配置的唯一读取入口：目录默认 → 全局配置 → 集群例外逐字段覆盖，
// 带短 TTL 进程内缓存；未知规则键返回 (nil, nil)。
// ResolveClusterRule is the single read entry point of effective rule config: catalog default →
// global config → cluster override merged field by field, backed by a short-TTL in-process
// cache; unknown rule keys return (nil, nil).
func (s *Service) ResolveClusterRule(ctx context.Context, ruleKey string, clusterID uint) (*resolvedRuleConfig, error) {
	def := lookupClusterRuleDefinition(ruleKey)
	if def == nil || s == nil || s.repo == nil {
		return nil, nil
	}
	cacheKey := def.Key + "|" + strconv.FormatUint(uint64(clusterID), 10)
	if cached := s.readClusterRuleCache(cacheKey); cached != nil {
		return cached, nil
	}
	layers, err := s.loadClusterRuleLayers(ctx)
	if err != nil {
		return nil, err
	}
	cfg := resolveClusterRuleFromLayers(def, layers, clusterID)
	s.storeClusterRuleCache(cacheKey, cfg)
	return cfg, nil
}

func (s *Service) readClusterRuleCache(cacheKey string) *resolvedRuleConfig {
	if clusterRuleCacheTTL <= 0 {
		return nil
	}
	s.clusterRuleCacheMu.Lock()
	defer s.clusterRuleCacheMu.Unlock()
	entry, ok := s.clusterRuleCache[cacheKey]
	if !ok || entry == nil {
		return nil
	}
	if !timeNowUTC().Before(entry.expiresAt) {
		delete(s.clusterRuleCache, cacheKey)
		return nil
	}
	return entry.cfg
}

func (s *Service) storeClusterRuleCache(cacheKey string, cfg *resolvedRuleConfig) {
	if clusterRuleCacheTTL <= 0 || cfg == nil {
		return
	}
	s.clusterRuleCacheMu.Lock()
	defer s.clusterRuleCacheMu.Unlock()
	if s.clusterRuleCache == nil {
		s.clusterRuleCache = map[string]*clusterRuleCacheEntry{}
	}
	s.clusterRuleCache[cacheKey] = &clusterRuleCacheEntry{
		cfg:       cfg,
		expiresAt: timeNowUTC().Add(clusterRuleCacheTTL),
	}
}

// invalidateClusterRuleResolution 在配置写入后调用：清掉解析缓存，并触发 TSDB 托管规则组
// 立即重建（复用评估器的 reloadCh，未启动时是空操作）。
// invalidateClusterRuleResolution runs after config writes: it clears the resolution cache and
// asks the managed tsdb rule group to rebuild immediately (reusing the evaluator reloadCh; a
// no-op when the evaluator is not running).
func (s *Service) invalidateClusterRuleResolution() {
	if s == nil {
		return
	}
	s.clusterRuleCacheMu.Lock()
	s.clusterRuleCache = map[string]*clusterRuleCacheEntry{}
	s.clusterRuleCacheMu.Unlock()
	if evaluator := s.metricEvaluator.Load(); evaluator != nil {
		evaluator.RequestReload()
	}
}

// ensureClusterRuleCache 给 Service 初始化解析缓存（NewService 调用）。
// ensureClusterRuleCache initialises the resolution cache on Service (called by NewService).
func (s *Service) ensureClusterRuleCache() {
	s.clusterRuleCacheMu.Lock()
	defer s.clusterRuleCacheMu.Unlock()
	if s.clusterRuleCache == nil {
		s.clusterRuleCache = map[string]*clusterRuleCacheEntry{}
	}
}

// SaveClusterRuleConfig 写入一条规则的全局配置（保存前校验 + 失效缓存 + 触发规则组重建）。
// SaveClusterRuleConfig writes the global config of one rule (validated first, then the cache
// is invalidated and the rule-group rebuild triggered).
func (s *Service) SaveClusterRuleConfig(ctx context.Context, ruleKey string, fields *clusterRuleFields) (*ClusterRuleConfig, error) {
	if err := s.validateClusterRuleSave(ctx, ruleKey, 0, fields); err != nil {
		return nil, err
	}
	row := &ClusterRuleConfig{RuleKey: strings.TrimSpace(ruleKey)}
	applyClusterRuleFieldsToConfig(row, fields)
	if err := s.repo.UpsertClusterRuleConfig(ctx, row); err != nil {
		return nil, err
	}
	s.invalidateClusterRuleResolution()
	return s.repo.GetClusterRuleConfig(ctx, row.RuleKey)
}

// DeleteClusterRuleConfig 删除一条规则的全局配置，恢复目录默认。
// DeleteClusterRuleConfig removes the global config of one rule, restoring catalog defaults.
func (s *Service) DeleteClusterRuleConfig(ctx context.Context, ruleKey string) error {
	if lookupClusterRuleDefinition(ruleKey) == nil {
		return ErrClusterRuleNotFound
	}
	if err := s.repo.DeleteClusterRuleConfig(ctx, ruleKey); err != nil {
		return err
	}
	s.invalidateClusterRuleResolution()
	return nil
}

// SaveClusterRuleOverride 写入某集群对一条规则的例外配置。
// SaveClusterRuleOverride writes one cluster's override of a rule.
func (s *Service) SaveClusterRuleOverride(ctx context.Context, ruleKey string, clusterID uint, fields *clusterRuleFields) (*ClusterRuleOverride, error) {
	if clusterID == 0 {
		return nil, fmt.Errorf("cluster id is required")
	}
	if err := s.validateClusterRuleSave(ctx, ruleKey, clusterID, fields); err != nil {
		return nil, err
	}
	row := &ClusterRuleOverride{RuleKey: strings.TrimSpace(ruleKey), ClusterID: clusterID}
	applyClusterRuleFieldsToOverride(row, fields)
	if err := s.repo.UpsertClusterRuleOverride(ctx, row); err != nil {
		return nil, err
	}
	s.invalidateClusterRuleResolution()
	return s.repo.GetClusterRuleOverride(ctx, row.RuleKey, row.ClusterID)
}

// DeleteClusterRuleOverride 删除某集群对一条规则的例外。
// DeleteClusterRuleOverride removes one cluster's override of a rule.
func (s *Service) DeleteClusterRuleOverride(ctx context.Context, ruleKey string, clusterID uint) error {
	if lookupClusterRuleDefinition(ruleKey) == nil {
		return ErrClusterRuleNotFound
	}
	if err := s.repo.DeleteClusterRuleOverride(ctx, ruleKey, clusterID); err != nil {
		return err
	}
	s.invalidateClusterRuleResolution()
	return nil
}

// SaveAlertNotificationDefaults 写入默认通知（渠道严格校验：存在、启用、邮件有可达接收人）。
// SaveAlertNotificationDefaults writes the default notification (channels are strictly
// validated: exist, enabled, email channels reachable).
func (s *Service) SaveAlertNotificationDefaults(ctx context.Context, channelIDs []uint, receiverUserIDs []uint64, cooldownMinutes int, sendRecovery bool) (*AlertNotificationDefaults, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if _, err := validateAlertChannelSelection(ctx, s.repo, channelIDs, len(receiverUserIDs) > 0,
		ErrAlertPolicyNotificationChannelNotFound, ErrAlertPolicySyncJobChannelDisabled, ErrAlertPolicySyncJobEmailRecipientMissing); err != nil {
		return nil, err
	}
	if err := s.validateAlertPolicyReceiverUsers(ctx, receiverUserIDs); err != nil {
		return nil, err
	}
	channelIDsJSON, err := marshalAlertPolicyChannelIDs(channelIDs)
	if err != nil {
		return nil, err
	}
	receiverUserIDsJSON, err := marshalAlertPolicyReceiverUserIDs(receiverUserIDs)
	if err != nil {
		return nil, err
	}
	row := &AlertNotificationDefaults{
		ChannelIDsJSON:      channelIDsJSON,
		ReceiverUserIDsJSON: receiverUserIDsJSON,
		CooldownMinutes:     cooldownMinutes,
		SendRecovery:        sendRecovery,
	}
	if err := s.repo.SaveAlertNotificationDefaults(ctx, row); err != nil {
		return nil, err
	}
	s.invalidateClusterRuleResolution()
	return s.repo.GetAlertNotificationDefaults(ctx)
}

func applyClusterRuleFieldsToConfig(row *ClusterRuleConfig, fields *clusterRuleFields) {
	if row == nil || fields == nil {
		return
	}
	row.Enabled = fields.Enabled
	row.Threshold = fields.Threshold
	row.ForSeconds = fields.ForSeconds
	row.Severity = fields.Severity
	row.NotifyMode = strings.TrimSpace(fields.NotifyMode)
	row.ChannelIDsJSON = fields.ChannelIDsJSON
	row.ReceiverUserIDsJSON = fields.ReceiverUserIDsJSON
}

func applyClusterRuleFieldsToOverride(row *ClusterRuleOverride, fields *clusterRuleFields) {
	if row == nil || fields == nil {
		return
	}
	row.Enabled = fields.Enabled
	row.Threshold = fields.Threshold
	row.ForSeconds = fields.ForSeconds
	row.Severity = fields.Severity
	row.NotifyMode = strings.TrimSpace(fields.NotifyMode)
	row.ChannelIDsJSON = fields.ChannelIDsJSON
	row.ReceiverUserIDsJSON = fields.ReceiverUserIDsJSON
}

// validateClusterRuleSave 保存前校验：字段值域 + critical 时生效通知必须至少有一个启用的
// 外部渠道且邮件渠道有可达接收人（含例外层）。candidate 按所在层（clusterID==0 为全局）
// 叠在现有配置上模拟生效结果。
// validateClusterRuleSave validates a pending write: field value ranges plus, for a critical
// effective severity, the effective notification must hold at least one enabled external
// channel with reachable email recipients (exception layer included). The candidate is layered
// at its own level (clusterID==0 is global) over the stored config to simulate the outcome.
func (s *Service) validateClusterRuleSave(ctx context.Context, ruleKey string, clusterID uint, candidate *clusterRuleFields) error {
	def := lookupClusterRuleDefinition(ruleKey)
	if def == nil {
		return ErrClusterRuleNotFound
	}
	if candidate == nil {
		candidate = &clusterRuleFields{}
	}
	if candidate.Severity != nil {
		switch AlertSeverity(strings.TrimSpace(*candidate.Severity)) {
		case AlertSeverityWarning, AlertSeverityCritical:
		default:
			return ErrClusterRuleInvalidSeverity
		}
	}
	switch strings.TrimSpace(candidate.NotifyMode) {
	case "", clusterRuleNotifyModeDefault, clusterRuleNotifyModeCustom, clusterRuleNotifyModeNone:
	default:
		return ErrClusterRuleInvalidNotifyMode
	}
	if candidate.Threshold != nil {
		if def.Threshold == nil {
			return ErrClusterRuleUnsupportedField
		}
		if *candidate.Threshold < def.Threshold.Min || *candidate.Threshold > def.Threshold.Max {
			return ErrClusterRuleInvalidThreshold
		}
	}
	if candidate.ForSeconds != nil {
		if !def.ForEditable {
			return ErrClusterRuleUnsupportedField
		}
		if *candidate.ForSeconds <= 0 {
			return ErrClusterRuleInvalidFor
		}
	}

	candidateChannelIDs := unmarshalAlertPolicyChannelIDs(candidate.ChannelIDsJSON)
	candidateReceiverIDs := unmarshalAlertPolicyReceiverUserIDs(candidate.ReceiverUserIDsJSON)
	if strings.TrimSpace(candidate.NotifyMode) == clusterRuleNotifyModeCustom {
		// 自定义通知：所选渠道逐条校验存在且启用，邮件渠道要有可达接收人（规则接收人或渠道收件人）。
		// Custom notification: every selected channel must exist and be enabled, and each email
		// channel must reach somebody (rule receivers or channel recipients).
		if _, err := validateAlertChannelSelection(ctx, s.repo, candidateChannelIDs, len(candidateReceiverIDs) > 0,
			ErrAlertPolicyNotificationChannelNotFound, ErrClusterRuleChannelDisabled, ErrClusterRuleEmailRecipientMissing); err != nil {
			return err
		}
		if err := s.validateAlertPolicyReceiverUsers(ctx, candidateReceiverIDs); err != nil {
			return err
		}
	}

	// 把候选层叠加到现有配置上，模拟保存后的生效结果再做 critical 不变式校验。
	// Layer the candidate over the stored config to simulate the post-save outcome before the
	// critical invariant check.
	layers, err := s.loadClusterRuleLayers(ctx)
	if err != nil {
		return err
	}
	if clusterID == 0 {
		layers.globals[def.Key] = candidate
	} else {
		if _, ok := layers.overrides[def.Key]; !ok {
			layers.overrides[def.Key] = map[uint]*clusterRuleFields{}
		}
		layers.overrides[def.Key][clusterID] = candidate
	}
	effective := resolveClusterRuleFromLayers(def, layers, clusterID)
	if resolveRuleSeverity(effective) == AlertSeverityCritical {
		if err := s.validateClusterRuleEffectiveNotification(ctx, effective.ChannelIDs, effective.ReceiverUserIDs); err != nil {
			return err
		}
	}
	if clusterID != 0 {
		return nil
	}
	// 全局保存会把未覆盖 severity 的例外一起推成 critical；逐条复算例外的生效结果，
	// 任何一个不满足 critical 不变式都拒绝整次写入，避免保存后遗留不可外投的 critical 规则。
	// A global save also pushes overrides that don't pin severity into critical; re-resolve
	// every stored override and reject the write when any of them violates the critical
	// invariant, so no unreachable critical rule is left behind.
	for _, overrideClusterID := range layers.overrideClusterIDs(def.Key) {
		overrideEffective := resolveClusterRuleFromLayers(def, layers, overrideClusterID)
		if resolveRuleSeverity(overrideEffective) != AlertSeverityCritical {
			continue
		}
		if err := s.validateClusterRuleEffectiveNotification(ctx, overrideEffective.ChannelIDs, overrideEffective.ReceiverUserIDs); err != nil {
			return fmt.Errorf("cluster_id=%d: %w", overrideClusterID, err)
		}
	}
	return nil
}

// validateClusterRuleEffectiveNotification 校验 critical 规则的生效通知：至少有一个启用的
// 外部渠道，且每个启用的邮件渠道有可达接收人（配置接收人或渠道自带收件人）。
// validateClusterRuleEffectiveNotification checks the effective notification of a critical
// rule: at least one enabled external channel, and every enabled email channel reachable
// (configured receivers or channel-level recipients).
func (s *Service) validateClusterRuleEffectiveNotification(ctx context.Context, channelIDs []uint, receiverUserIDs []uint64) error {
	hasExternal := false
	for _, channelID := range normalizeAlertPolicyChannelIDs(channelIDs) {
		channel, err := s.repo.GetNotificationChannelByID(ctx, channelID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return err
		}
		if channel == nil || !channel.Enabled {
			continue
		}
		hasExternal = true
		if channel.Type != NotificationChannelTypeEmail || len(receiverUserIDs) > 0 {
			continue
		}
		channelConfig := unmarshalNotificationChannelConfig(channel.Type, channel.ConfigJSON)
		if channelConfig == nil || channelConfig.Email == nil || len(channelConfig.Email.Recipients) == 0 {
			return ErrClusterRuleEmailRecipientMissing
		}
	}
	if !hasExternal {
		return ErrClusterRuleCriticalChannelRequired
	}
	return nil
}
