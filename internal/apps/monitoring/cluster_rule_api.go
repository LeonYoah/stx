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
	"strings"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
)

// clusterRuleScope 是集群级规则固定的 scope 取值。
// clusterRuleScope is the fixed scope value of every cluster-level rule.
const clusterRuleScope = "cluster"

// ClusterRuleThresholdView 是集群级规则阈值的可编辑元数据：value 为当前生效值（null 表示
// 等于部署数，目前只有 worker_insufficient），default 为目录默认值，unit/operator/min/max
// 供前端渲染输入控件。
// ClusterRuleThresholdView is the editable-threshold metadata of one cluster rule: value is the
// effective value (null means "equal to the deployed count", currently only worker_insufficient),
// default the catalog default, and unit/operator/min/max drive the input control.
type ClusterRuleThresholdView struct {
	Value    *float64 `json:"value"`
	Default  *float64 `json:"default"`
	Unit     string   `json:"unit"`
	Operator string   `json:"operator"`
	Min      float64  `json:"min"`
	Max      float64  `json:"max"`
}

// ClusterRuleLayerView 是一层配置（全局或例外）的用户原值：nil 字段表示继承上一层。
// ClusterRuleLayerView is the raw stored value of one layer (global or override): nil fields
// inherit the layer below.
type ClusterRuleLayerView struct {
	Enabled         *bool    `json:"enabled"`
	Threshold       *float64 `json:"threshold"`
	ForSeconds      *int     `json:"for_seconds"`
	Severity        *string  `json:"severity"`
	NotifyMode      *string  `json:"notify_mode"`
	ChannelIDs      []uint   `json:"channel_ids"`
	ReceiverUserIDs []uint64 `json:"receiver_user_ids"`
}

// ClusterRuleOverrideEffective 是单条例外叠加目录与全局层后的生效结果；external_channel_missing
// 为 true 时前端显示警告标识（critical 但生效通知已无启用外部渠道）。
// ClusterRuleOverrideEffective is the resolved result of one override over catalog and global
// layers; external_channel_missing flags a critical rule whose effective notification has no
// enabled external channel left.
type ClusterRuleOverrideEffective struct {
	Enabled                bool          `json:"enabled"`
	Severity               AlertSeverity `json:"severity"`
	Threshold              *float64      `json:"threshold"`
	ForSeconds             int           `json:"for_seconds"`
	ExternalChannelMissing bool          `json:"external_channel_missing"`
}

// ClusterRuleOverrideView 是 API 返回的一条集群例外：原值 + 生效结果。
// ClusterRuleOverrideView is one cluster override in the API: raw layer values plus the
// resolved effective result.
type ClusterRuleOverrideView struct {
	ClusterID       uint                          `json:"cluster_id"`
	ClusterName     string                        `json:"cluster_name"`
	Enabled         *bool                         `json:"enabled"`
	Threshold       *float64                      `json:"threshold"`
	ForSeconds      *int                          `json:"for_seconds"`
	Severity        *string                       `json:"severity"`
	NotifyMode      *string                       `json:"notify_mode"`
	ChannelIDs      []uint                        `json:"channel_ids"`
	ReceiverUserIDs []uint64                      `json:"receiver_user_ids"`
	Effective       *ClusterRuleOverrideEffective `json:"effective"`
}

// ClusterRuleView 是 GET /cluster-rules 的单条响应：顶层为生效值，global 为全局层原值。
// ClusterRuleView is one GET /cluster-rules item: top-level fields carry effective values and
// global carries the raw global-layer values.
type ClusterRuleView struct {
	Key                    string                     `json:"key"`
	Name                   string                     `json:"name"`
	NameEN                 string                     `json:"name_en"`
	Description            string                     `json:"description"`
	DescriptionEN          string                     `json:"description_en"`
	Scope                  string                     `json:"scope"`
	DefaultEnabled         bool                       `json:"default_enabled"`
	Enabled                bool                       `json:"enabled"`
	Severity               AlertSeverity              `json:"severity"`
	Threshold              *ClusterRuleThresholdView  `json:"threshold"`
	ForSeconds             int                        `json:"for_seconds"`
	DefaultForSeconds      int                        `json:"default_for_seconds"`
	ForEditable            bool                       `json:"for_editable"`
	NotifyMode             string                     `json:"notify_mode"`
	ChannelIDs             []uint                     `json:"channel_ids"`
	ReceiverUserIDs        []uint64                   `json:"receiver_user_ids"`
	HasGlobalConfig        bool                       `json:"has_global_config"`
	Global                 *ClusterRuleLayerView      `json:"global"`
	Overrides              []*ClusterRuleOverrideView `json:"overrides"`
	OverrideCount          int                        `json:"override_count"`
	ExternalChannelMissing bool                       `json:"external_channel_missing"`
}

// AlertNotificationDefaultsView 是默认通知的 API 形态；configured=false 表示尚未写入行。
// AlertNotificationDefaultsView is the API shape of the default notification; configured=false
// means no row has been written yet.
type AlertNotificationDefaultsView struct {
	ChannelIDs      []uint   `json:"channel_ids"`
	ReceiverUserIDs []uint64 `json:"receiver_user_ids"`
	CooldownMinutes int      `json:"cooldown_minutes"`
	SendRecovery    bool     `json:"send_recovery"`
	Configured      bool     `json:"configured"`
}

// clusterRuleLayerRequest 是 PUT 请求体：整层替换语义，缺省或 null 的字段继承上一层。
// clusterRuleLayerRequest is the PUT body: whole-layer replacement semantics where omitted or
// null fields inherit the layer below.
type clusterRuleLayerRequest struct {
	Enabled         *bool     `json:"enabled"`
	Threshold       *float64  `json:"threshold"`
	ForSeconds      *int      `json:"for_seconds"`
	Severity        *string   `json:"severity"`
	NotifyMode      *string   `json:"notify_mode"`
	ChannelIDs      *[]uint   `json:"channel_ids"`
	ReceiverUserIDs *[]uint64 `json:"receiver_user_ids"`
}

// alertNotificationDefaultsRequest 是默认通知的 PUT 请求体。
// alertNotificationDefaultsRequest is the PUT body of the default notification.
type alertNotificationDefaultsRequest struct {
	ChannelIDs      *[]uint   `json:"channel_ids"`
	ReceiverUserIDs *[]uint64 `json:"receiver_user_ids"`
	CooldownMinutes *int      `json:"cooldown_minutes"`
	SendRecovery    *bool     `json:"send_recovery"`
}

// toFields 把请求体转换为一层配置的存储字段。
// toFields converts the request body into the stored fields of one layer.
func (r *clusterRuleLayerRequest) toFields() *clusterRuleFields {
	fields := &clusterRuleFields{}
	if r == nil {
		return fields
	}
	fields.Enabled = r.Enabled
	fields.Threshold = r.Threshold
	fields.ForSeconds = r.ForSeconds
	fields.Severity = r.Severity
	if r.NotifyMode != nil {
		fields.NotifyMode = strings.TrimSpace(*r.NotifyMode)
	}
	if r.ChannelIDs != nil {
		if data, err := json.Marshal(normalizeAlertPolicyChannelIDs(*r.ChannelIDs)); err == nil {
			fields.ChannelIDsJSON = string(data)
		}
	}
	if r.ReceiverUserIDs != nil {
		if data, err := json.Marshal(*r.ReceiverUserIDs); err == nil {
			fields.ReceiverUserIDsJSON = string(data)
		}
	}
	return fields
}

// ListClusterRuleViews 返回全部 21 条集群级规则的 API 视图。
// ListClusterRuleViews returns the API views of all 21 cluster-level rules.
func (s *Service) ListClusterRuleViews(ctx context.Context) ([]*ClusterRuleView, error) {
	keys := make([]string, 0, len(clusterRuleCatalog))
	for _, def := range clusterRuleCatalog {
		keys = append(keys, def.Key)
	}
	return s.buildClusterRuleViews(ctx, keys)
}

// GetClusterRuleView 返回单条集群级规则的 API 视图。
// GetClusterRuleView returns the API view of one cluster-level rule.
func (s *Service) GetClusterRuleView(ctx context.Context, ruleKey string) (*ClusterRuleView, error) {
	views, err := s.buildClusterRuleViews(ctx, []string{strings.TrimSpace(ruleKey)})
	if err != nil {
		return nil, err
	}
	if len(views) == 0 {
		return nil, ErrClusterRuleNotFound
	}
	return views[0], nil
}

// buildClusterRuleViews 一次装载三层配置、启用渠道与集群名，批量组装规则视图。
// buildClusterRuleViews loads the three config layers, enabled channels and cluster names once,
// then assembles every rule view.
func (s *Service) buildClusterRuleViews(ctx context.Context, keys []string) ([]*ClusterRuleView, error) {
	layers, err := s.loadClusterRuleLayers(ctx)
	if err != nil {
		return nil, err
	}
	enabledChannels, err := s.listEnabledChannelIDs(ctx)
	if err != nil {
		return nil, err
	}
	clusterNames := s.listClusterNames(ctx)

	views := make([]*ClusterRuleView, 0, len(keys))
	for _, key := range keys {
		def := lookupClusterRuleDefinition(key)
		if def == nil {
			continue
		}
		effective := resolveClusterRuleFromLayers(def, layers, 0)
		view := &ClusterRuleView{
			Key:                    def.Key,
			Name:                   def.Name,
			NameEN:                 def.NameEN,
			Description:            def.Description,
			DescriptionEN:          def.DescriptionEN,
			Scope:                  clusterRuleScope,
			DefaultEnabled:         def.DefaultEnabled,
			Enabled:                effective.Enabled,
			Severity:               resolveRuleSeverity(effective),
			ForSeconds:             effective.ForSeconds,
			DefaultForSeconds:      int(def.DefaultFor / 1000000000),
			ForEditable:            def.ForEditable,
			NotifyMode:             effective.NotifyMode,
			ChannelIDs:             effective.ChannelIDs,
			ReceiverUserIDs:        effective.ReceiverUserIDs,
			ExternalChannelMissing: clusterRuleExternalChannelMissing(effective, enabledChannels),
		}
		if def.Threshold != nil {
			view.Threshold = &ClusterRuleThresholdView{
				Value:    effective.Threshold,
				Unit:     def.Threshold.Unit,
				Operator: def.Threshold.Operator,
				Min:      def.Threshold.Min,
				Max:      def.Threshold.Max,
			}
			if def.Threshold.Unit != clusterRuleThresholdUnitWorkers {
				// workers 单位的默认值是"部署数"而非具体数字，default 留给 null。
				// The workers unit's default is the deployed count, not a number, so default
				// stays null.
				defaultValue := def.Threshold.Default
				view.Threshold.Default = &defaultValue
			}
		}
		if global := layers.global(def.Key); global != nil {
			view.HasGlobalConfig = true
			view.Global = clusterRuleLayerToView(global)
		} else {
			view.Global = &ClusterRuleLayerView{}
		}
		for _, clusterID := range layers.overrideClusterIDs(def.Key) {
			override := layers.override(def.Key, clusterID)
			overrideEffective := resolveClusterRuleFromLayers(def, layers, clusterID)
			overrideView := &ClusterRuleOverrideView{
				ClusterID:       clusterID,
				ClusterName:     clusterNames[clusterID],
				Enabled:         override.Enabled,
				Threshold:       override.Threshold,
				ForSeconds:      override.ForSeconds,
				Severity:        override.Severity,
				ChannelIDs:      unmarshalAlertPolicyChannelIDs(override.ChannelIDsJSON),
				ReceiverUserIDs: unmarshalAlertPolicyReceiverUserIDs(override.ReceiverUserIDsJSON),
				Effective: &ClusterRuleOverrideEffective{
					Enabled:                overrideEffective.Enabled,
					Severity:               resolveRuleSeverity(overrideEffective),
					Threshold:              overrideEffective.Threshold,
					ForSeconds:             overrideEffective.ForSeconds,
					ExternalChannelMissing: clusterRuleExternalChannelMissing(overrideEffective, enabledChannels),
				},
			}
			if mode := strings.TrimSpace(override.NotifyMode); mode != "" {
				overrideView.NotifyMode = &mode
			}
			view.Overrides = append(view.Overrides, overrideView)
		}
		if view.Overrides == nil {
			view.Overrides = []*ClusterRuleOverrideView{}
		}
		view.OverrideCount = len(view.Overrides)
		views = append(views, view)
	}
	return views, nil
}

// clusterRuleLayerToView 把一层存储字段转成 API 原值视图。
// clusterRuleLayerToView converts stored layer fields into the raw-value API view.
func clusterRuleLayerToView(fields *clusterRuleFields) *ClusterRuleLayerView {
	view := &ClusterRuleLayerView{}
	if fields == nil {
		return view
	}
	view.Enabled = fields.Enabled
	view.Threshold = fields.Threshold
	view.ForSeconds = fields.ForSeconds
	view.Severity = fields.Severity
	if mode := strings.TrimSpace(fields.NotifyMode); mode != "" {
		view.NotifyMode = &mode
	}
	view.ChannelIDs = unmarshalAlertPolicyChannelIDs(fields.ChannelIDsJSON)
	view.ReceiverUserIDs = unmarshalAlertPolicyReceiverUserIDs(fields.ReceiverUserIDsJSON)
	return view
}

// clusterRuleExternalChannelMissing 判定 critical 生效结果的渠道里是否已无启用的外部渠道。
// clusterRuleExternalChannelMissing reports whether a critical effective config has no enabled
// external channel left.
func clusterRuleExternalChannelMissing(cfg *resolvedRuleConfig, enabledChannels map[uint]bool) bool {
	if cfg == nil || resolveRuleSeverity(cfg) != AlertSeverityCritical {
		return false
	}
	for _, channelID := range cfg.ChannelIDs {
		if enabledChannels[channelID] {
			return false
		}
	}
	return true
}

// listEnabledChannelIDs 取全部启用渠道 ID；仓储不可用时返回空集（视图降级为 missing=true，
// 由调用方决定是否提示）。
// listEnabledChannelIDs returns every enabled channel id; an unavailable repository yields an
// empty set (views degrade to missing=true, up to the caller to surface it).
func (s *Service) listEnabledChannelIDs(ctx context.Context) (map[uint]bool, error) {
	enabled := map[uint]bool{}
	if s.repo == nil {
		return enabled, nil
	}
	channels, err := s.repo.ListNotificationChannels(ctx)
	if err != nil {
		return nil, err
	}
	for _, channel := range channels {
		if channel != nil && channel.Enabled {
			enabled[channel.ID] = true
		}
	}
	return enabled, nil
}

// listClusterNames 取集群 ID → 名称映射给例外展示；集群服务不可用时返回空表（名称留空）。
// listClusterNames maps cluster IDs to names for override display; an unavailable cluster
// service yields an empty map (names stay empty).
func (s *Service) listClusterNames(ctx context.Context) map[uint]string {
	names := map[uint]string{}
	if s.clusterService == nil {
		return names
	}
	clusters, _, err := s.clusterService.List(ctx, &clusterapp.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return names
	}
	for _, item := range clusters {
		if item != nil {
			names[item.ID] = item.Name
		}
	}
	return names
}

// GetAlertNotificationDefaultsView 返回默认通知视图；未配置时 configured=false，值域为空。
// GetAlertNotificationDefaultsView returns the default notification view; unconfigured rows
// yield configured=false with empty values.
func (s *Service) GetAlertNotificationDefaultsView(ctx context.Context) (*AlertNotificationDefaultsView, error) {
	view := &AlertNotificationDefaultsView{ChannelIDs: []uint{}, ReceiverUserIDs: []uint64{}, SendRecovery: true}
	if s.repo == nil {
		return view, nil
	}
	row, err := s.repo.GetAlertNotificationDefaults(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return view, nil
	}
	view.Configured = true
	view.ChannelIDs = unmarshalAlertPolicyChannelIDs(row.ChannelIDsJSON)
	view.ReceiverUserIDs = unmarshalAlertPolicyReceiverUserIDs(row.ReceiverUserIDsJSON)
	view.CooldownMinutes = row.CooldownMinutes
	view.SendRecovery = row.SendRecovery
	return view, nil
}
