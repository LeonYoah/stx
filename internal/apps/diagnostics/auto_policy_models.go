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

package diagnostics

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// InspectionConditionTemplateCode identifies one built-in auto-inspection condition template.
// InspectionConditionTemplateCode 标识一个内置的自动巡检条件模板。
type InspectionConditionTemplateCode string

const (
	// ConditionCodeJavaOOM triggers on java.lang.OutOfMemoryError.
	// ConditionCodeJavaOOM 在 java.lang.OutOfMemoryError 时触发。
	ConditionCodeJavaOOM InspectionConditionTemplateCode = "JAVA_OOM"
	// ConditionCodeJavaStackOverflow triggers on java.lang.StackOverflowError.
	// ConditionCodeJavaStackOverflow 在 java.lang.StackOverflowError 时触发。
	ConditionCodeJavaStackOverflow InspectionConditionTemplateCode = "JAVA_STACKOVERFLOW"
	// ConditionCodeJavaMetaspace triggers on Metaspace exhaustion.
	// ConditionCodeJavaMetaspace 在 Metaspace 耗尽时触发。
	ConditionCodeJavaMetaspace InspectionConditionTemplateCode = "JAVA_METASPACE"
	// ConditionCodeErrorSpike triggers on error frequency spike.
	// ConditionCodeErrorSpike 在错误频率激增时触发。
	ConditionCodeErrorSpike InspectionConditionTemplateCode = "ERROR_SPIKE"
	// ConditionCodeAlertFiring triggers on a specific alert rule firing.
	// ConditionCodeAlertFiring 在指定告警规则触发时触发。
	ConditionCodeAlertFiring InspectionConditionTemplateCode = "ALERT_FIRING"
	// ConditionCodeScheduled triggers on a cron schedule.
	// ConditionCodeScheduled 按 Cron 表达式定时触发。
	ConditionCodeScheduled InspectionConditionTemplateCode = "SCHEDULED"
)

// InspectionConditionCategory groups condition templates by domain.
// InspectionConditionCategory 按领域对条件模板分类。
type InspectionConditionCategory string

const (
	// ConditionCategoryJavaError groups Java fatal error conditions.
	// ConditionCategoryJavaError 分组 Java 致命错误条件。
	ConditionCategoryJavaError InspectionConditionCategory = "java_error"
	// ConditionCategoryErrorRate groups error rate conditions.
	// ConditionCategoryErrorRate 分组错误频率条件。
	ConditionCategoryErrorRate InspectionConditionCategory = "error_rate"
	// ConditionCategoryAlertFiring groups alert rule conditions.
	// ConditionCategoryAlertFiring 分组告警规则条件。
	ConditionCategoryAlertFiring InspectionConditionCategory = "alert_firing"
	// ConditionCategorySchedule groups schedule-based conditions.
	// ConditionCategorySchedule 分组定时巡检条件。
	ConditionCategorySchedule InspectionConditionCategory = "schedule"
)

// InspectionConditionTemplate describes one built-in condition template.
// InspectionConditionTemplate 描述一个内置条件模板。
type InspectionConditionTemplate struct {
	Code                 InspectionConditionTemplateCode `json:"code"`
	Category             InspectionConditionCategory     `json:"category"`
	Name                 string                          `json:"name"`
	Description          string                          `json:"description"`
	DefaultThreshold     int                             `json:"default_threshold"`
	DefaultWindowMinutes int                             `json:"default_window_minutes"`
	DefaultCronExpr      string                          `json:"default_cron_expr,omitempty"`
	ImmediateOnMatch     bool                            `json:"immediate_on_match"`
}

// BuiltinConditionTemplates is the complete list of built-in auto-inspection condition templates.
// BuiltinConditionTemplates 是内置自动巡检条件模板完整列表。
var BuiltinConditionTemplates = []InspectionConditionTemplate{
	{
		Code:             ConditionCodeJavaOOM,
		Category:         ConditionCategoryJavaError,
		Name:             bilingualText("Java 内存溢出 (OOM)", "Java Out Of Memory (OOM)"),
		Description:      bilingualText("exception_class 含 OutOfMemoryError 时立即触发巡检。", "Trigger inspection immediately when exception_class contains OutOfMemoryError."),
		ImmediateOnMatch: true,
	},
	{
		Code:             ConditionCodeJavaStackOverflow,
		Category:         ConditionCategoryJavaError,
		Name:             bilingualText("Java 栈溢出", "Java Stack Overflow"),
		Description:      bilingualText("exception_class 含 StackOverflowError 时立即触发巡检。", "Trigger inspection immediately when exception_class contains StackOverflowError."),
		ImmediateOnMatch: true,
	},
	{
		Code:             ConditionCodeJavaMetaspace,
		Category:         ConditionCategoryJavaError,
		Name:             bilingualText("Metaspace 耗尽", "Metaspace Exhausted"),
		Description:      bilingualText("exception_class 含 OutOfMemoryError 且 message 含 Metaspace 时立即触发巡检。", "Trigger inspection immediately when exception_class contains OutOfMemoryError and message contains Metaspace."),
		ImmediateOnMatch: true,
	},
	{
		Code:                 ConditionCodeErrorSpike,
		Category:             ConditionCategoryErrorRate,
		Name:                 bilingualText("错误频率激增", "Error Spike"),
		Description:          bilingualText("M 分钟内错误数超过 N 条。", "Trigger when the error count exceeds N within M minutes."),
		DefaultThreshold:     50,
		DefaultWindowMinutes: 5,
	},
	{
		Code:                 ConditionCodeAlertFiring,
		Category:             ConditionCategoryAlertFiring,
		Name:                 bilingualText("告警触发", "Alert Firing"),
		Description:          bilingualText("所选告警规则触发时发起巡检；可要求持续时长与同时触发的对象数。", "Trigger inspection when the selected alert rules fire; may require a minimum duration and a minimum number of firing objects."),
		DefaultThreshold:     1,
		DefaultWindowMinutes: 0,
	},
	{
		Code:            ConditionCodeScheduled,
		Category:        ConditionCategorySchedule,
		Name:            bilingualText("定时巡检", "Scheduled Inspection"),
		Description:     bilingualText("按 Cron 表达式定时触发巡检。", "Trigger inspection on a schedule defined by a cron expression."),
		DefaultCronExpr: "0 0 * * *",
	},
}

var supportedConditionTemplateCodes = map[InspectionConditionTemplateCode]struct{}{
	ConditionCodeJavaOOM:           {},
	ConditionCodeJavaStackOverflow: {},
	ConditionCodeJavaMetaspace:     {},
	ConditionCodeErrorSpike:        {},
	ConditionCodeAlertFiring:       {},
	ConditionCodeScheduled:         {},
}

func isConditionTemplateSupported(code InspectionConditionTemplateCode) bool {
	_, ok := supportedConditionTemplateCodes[code]
	return ok
}

func findBuiltinConditionTemplate(code InspectionConditionTemplateCode) (*InspectionConditionTemplate, bool) {
	for i := range BuiltinConditionTemplates {
		if BuiltinConditionTemplates[i].Code == code {
			return &BuiltinConditionTemplates[i], true
		}
	}
	return nil, false
}

// InspectionConditionItem stores one user-configured condition within an auto-policy.
// InspectionConditionItem 存储自动策略中的一条用户配置的条件项。
type InspectionConditionItem struct {
	TemplateCode          InspectionConditionTemplateCode `json:"template_code"`
	Enabled               bool                            `json:"enabled"`
	ThresholdOverride     *int                            `json:"threshold_override,omitempty"`
	WindowMinutesOverride *int                            `json:"window_minutes_override,omitempty"`
	CronExprOverride      string                          `json:"cron_expr_override,omitempty"`
	ExtraKeywords         []string                        `json:"extra_keywords,omitempty"`
	// RuleKeys 是告警触发条件要监听的规则键，取自告警规则目录，按精确相等匹配。
	// RuleKeys lists the rule keys an alert-firing condition watches, taken from the alert rule catalog; exact match.
	RuleKeys []string `json:"rule_keys,omitempty"`
}

// InspectionConditionItems is a slice of InspectionConditionItem for JSON column storage.
// InspectionConditionItems 是 InspectionConditionItem 切片，用于 JSON 列存储。
type InspectionConditionItems []InspectionConditionItem

// Value implements driver.Valuer for JSON storage.
// Value 实现 driver.Valuer，用于 JSON 存储。
func (items InspectionConditionItems) Value() (driver.Value, error) {
	if items == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(items)
}

// Scan implements sql.Scanner for JSON retrieval.
// Scan 实现 sql.Scanner，用于 JSON 读取。
func (items *InspectionConditionItems) Scan(value interface{}) error {
	bytes, err := normalizeJSONScanBytes(value)
	if err != nil {
		return err
	}
	if bytes == nil {
		*items = InspectionConditionItems{}
		return nil
	}
	return json.Unmarshal(bytes, items)
}

// InspectionAutoPolicy persists one auto-inspection trigger policy.
// InspectionAutoPolicy 存储一条自动巡检触发策略。
type InspectionAutoPolicy struct {
	ID              uint                     `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID       uint                     `json:"cluster_id" gorm:"index;not null;default:0"`
	Name            string                   `json:"name" gorm:"size:200;not null"`
	Enabled         bool                     `json:"enabled" gorm:"not null"`
	Conditions      InspectionConditionItems `json:"conditions" gorm:"type:json;not null"`
	CooldownMinutes int                      `json:"cooldown_minutes" gorm:"not null;default:30"`
	// AutoCreateTask controls whether a diagnostics bundle task should be created automatically after an inspection is triggered.
	// AutoCreateTask 控制巡检触发后是否自动创建诊断包任务。
	AutoCreateTask bool `json:"auto_create_task" gorm:"not null;default:false"`
	// AutoStartTask controls whether the auto-created diagnostics task should start immediately.
	// AutoStartTask 控制自动创建的诊断任务是否立即开始执行。
	AutoStartTask bool `json:"auto_start_task" gorm:"not null;default:false"`
	// TaskOptions configures the diagnostics task bundle options (thread dump / JVM dump / log sample, etc.).
	// TaskOptions 配置诊断任务的采集选项（线程栈 / JVM Dump / 日志采样等）。
	TaskOptions DiagnosticTaskOptions `json:"task_options" gorm:"type:json;not null"`
	CreatedAt   time.Time             `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt   time.Time             `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the auto-policy table name.
// TableName 指定自动策略表名。
func (InspectionAutoPolicy) TableName() string {
	return "diagnostics_inspection_auto_policies"
}

// InspectionAutoPolicyInfo is the API view model for one auto-policy.
// InspectionAutoPolicyInfo 是自动策略的 API 视图模型。
type InspectionAutoPolicyInfo struct {
	ID              uint                     `json:"id"`
	ClusterID       uint                     `json:"cluster_id"`
	Name            string                   `json:"name"`
	Enabled         bool                     `json:"enabled"`
	Conditions      InspectionConditionItems `json:"conditions"`
	CooldownMinutes int                      `json:"cooldown_minutes"`
	AutoCreateTask  bool                     `json:"auto_create_task"`
	AutoStartTask   bool                     `json:"auto_start_task"`
	TaskOptions     DiagnosticTaskOptions    `json:"task_options"`
	// InvalidReason 非空表示策略带了已失效的条件（旧 PROM_* / NODE_UNHEALTHY 模板、
	// 未选规则的 ALERT_FIRING 或引用了目录外规则键的 ALERT_FIRING），在当前内置监控语义下
	// 不会触发；存量数据不自动改写，只显式提示，引导编辑重选。取值是稳定的原因码
	// （见 AutoPolicyInvalid*），由前端按 i18n 翻译展示。
	// InvalidReason is non-empty when the policy carries a dead condition (a retired PROM_* /
	// NODE_UNHEALTHY template, an ALERT_FIRING condition without rule keys, or one referencing
	// rule keys outside the catalog) that never fires under the embedded monitor; stored rows
	// are not rewritten, only surfaced for editing. The value is a stable reason code
	// (see AutoPolicyInvalid*) translated via i18n.
	InvalidReason string `json:"invalid_reason"`
	// InvalidDetail 携带失效的具体取值（旧条件模板码等），供界面翻译时插值展示。
	// InvalidDetail carries the offending value (e.g. the retired condition template code)
	// so the UI can interpolate it into the translated message.
	InvalidDetail string `json:"invalid_detail,omitempty"`
	// LastTriggeredAt 是该策略最近一次触发巡检的时间；从未触发为 null。
	// 仅在列表响应中由服务层聚合填充，详情接口不保证携带。
	// LastTriggeredAt is the policy's latest inspection trigger time; null when it never
	// fired. It is only populated by the list endpoint via service-side aggregation.
	LastTriggeredAt *time.Time `json:"last_triggered_at"`
	// TriggerCount7d 是近 7 天该策略触发巡检的次数，同 LastTriggeredAt 仅在列表响应填充。
	// TriggerCount7d counts inspections this policy triggered in the last 7 days; like
	// LastTriggeredAt it is only populated by the list endpoint.
	TriggerCount7d int64     `json:"trigger_count_7d"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// 自动巡检策略失效原因码。/ Invalid-reason codes for auto policies.
const (
	// AutoPolicyInvalidLegacyCondition 表示条件模板已不在内置目录（外部 Prometheus
	// 时代的 PROM_* / NODE_UNHEALTHY），内置监控下永不触发。
	// AutoPolicyInvalidLegacyCondition marks a condition template missing from the builtin
	// catalog (the external-Prometheus era PROM_* / NODE_UNHEALTHY codes).
	AutoPolicyInvalidLegacyCondition = "legacy_condition_code"
	// AutoPolicyInvalidAlertFiringNoRules 表示 ALERT_FIRING 条件没有选任何告警规则键。
	// AutoPolicyInvalidAlertFiringNoRules marks an ALERT_FIRING condition with no rule keys.
	AutoPolicyInvalidAlertFiringNoRules = "alert_firing_without_rule_keys"
	// AutoPolicyInvalidUnknownAlertRuleKey 表示 ALERT_FIRING 条件引用了已不在告警规则目录的
	// 规则键（如告警模型收拢后停用的 stx_* 平台自监控模板），内置监控下永不触发。
	// AutoPolicyInvalidUnknownAlertRuleKey marks an ALERT_FIRING condition referencing a rule
	// key that is no longer in the alert rule catalog (e.g. the stx_* platform self-monitoring
	// templates retired by the alert-model consolidation); it never fires under the embedded
	// monitor.
	AutoPolicyInvalidUnknownAlertRuleKey = "unknown_alert_rule_key"
)

// InvalidReason 返回策略失效原因码（空串表示有效），与读取视图共用：
//   - 条件模板已不在内置目录（外部 Prometheus 时代的 PROM_* / NODE_UNHEALTHY）→ legacy_condition_code；
//   - ALERT_FIRING 条件没有选任何告警规则键 → alert_firing_without_rule_keys；
//   - ALERT_FIRING 条件引用了目录外规则键 → unknown_alert_rule_key。
//
// 返回的是稳定原因码而不是整句文案，文案由前端按 i18n 渲染；注意判定独立于写入校验：
// 历史数据不经过 validateAutoPolicyConditions，必须单独判读。
// InvalidReason returns the policy's invalid-reason code ("" when valid), shared by read views:
//   - a condition template missing from the builtin catalog (the PROM_* / NODE_UNHEALTHY
//     codes from the external-Prometheus era) -> legacy_condition_code;
//   - an ALERT_FIRING condition with no alert rule keys selected -> alert_firing_without_rule_keys;
//   - an ALERT_FIRING condition referencing a rule key outside the catalog ->
//     unknown_alert_rule_key.
//
// It returns a stable code rather than a full sentence; the UI renders the message via i18n.
// The check is deliberately separate from write validation: stored rows never pass through
// validateAutoPolicyConditions and must be judged on their own.
func (p *InspectionAutoPolicy) InvalidReason() string {
	reason, _ := p.invalidInfo()
	return reason
}

// InvalidDetail 返回失效原因对应的具体取值（旧条件模板码 / ALERT_FIRING / 目录外规则键），
// 供前端插值。
// InvalidDetail returns the offending value for the invalid reason (the retired template
// code / ALERT_FIRING / an out-of-catalog rule key) for frontend interpolation.
func (p *InspectionAutoPolicy) InvalidDetail() string {
	_, detail := p.invalidInfo()
	return detail
}

// invalidInfo 返回 (原因码, 失效取值)；空串表示策略有效。
// invalidInfo returns (reason code, offending value); empty strings mean the policy is valid.
func (p *InspectionAutoPolicy) invalidInfo() (reason, detail string) {
	if p == nil {
		return "", ""
	}
	for _, condition := range p.Conditions {
		if !isConditionTemplateSupported(condition.TemplateCode) {
			return AutoPolicyInvalidLegacyCondition, string(condition.TemplateCode)
		}
		if condition.TemplateCode != ConditionCodeAlertFiring {
			continue
		}
		ruleKeys := normalizeAlertRuleKeys(condition.RuleKeys)
		if len(ruleKeys) == 0 {
			return AutoPolicyInvalidAlertFiringNoRules, string(ConditionCodeAlertFiring)
		}
		catalogKeys := alertRuleCatalogKeySet()
		for _, ruleKey := range ruleKeys {
			if _, ok := catalogKeys[ruleKey]; !ok {
				return AutoPolicyInvalidUnknownAlertRuleKey, ruleKey
			}
		}
	}
	return "", ""
}

// ToInfo converts a persisted auto-policy into API view model.
// ToInfo 将自动策略转换为 API 视图模型。
func (p *InspectionAutoPolicy) ToInfo() *InspectionAutoPolicyInfo {
	if p == nil {
		return nil
	}
	conditions := p.Conditions
	if conditions == nil {
		conditions = InspectionConditionItems{}
	}
	return &InspectionAutoPolicyInfo{
		ID:              p.ID,
		ClusterID:       p.ClusterID,
		Name:            p.Name,
		Enabled:         p.Enabled,
		Conditions:      conditions,
		CooldownMinutes: firstNonZeroInt(p.CooldownMinutes, 30),
		AutoCreateTask:  p.AutoCreateTask,
		AutoStartTask:   p.AutoStartTask,
		TaskOptions:     p.TaskOptions,
		InvalidReason:   p.InvalidReason(),
		InvalidDetail:   p.InvalidDetail(),
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

// CreateInspectionAutoPolicyRequest describes a create auto-policy request.
// CreateInspectionAutoPolicyRequest 描述创建自动策略的请求。
type CreateInspectionAutoPolicyRequest struct {
	ClusterID       uint                     `json:"cluster_id"`
	Name            string                   `json:"name" binding:"required"`
	Enabled         *bool                    `json:"enabled,omitempty"`
	Conditions      InspectionConditionItems `json:"conditions" binding:"required"`
	CooldownMinutes int                      `json:"cooldown_minutes"`
	AutoCreateTask  bool                     `json:"auto_create_task"`
	AutoStartTask   bool                     `json:"auto_start_task"`
	TaskOptions     *DiagnosticTaskOptions   `json:"task_options,omitempty"`
}

// UpdateInspectionAutoPolicyRequest describes an update auto-policy request.
// UpdateInspectionAutoPolicyRequest 描述更新自动策略的请求。
type UpdateInspectionAutoPolicyRequest struct {
	Name            *string                   `json:"name,omitempty"`
	Enabled         *bool                     `json:"enabled,omitempty"`
	Conditions      *InspectionConditionItems `json:"conditions,omitempty"`
	CooldownMinutes *int                      `json:"cooldown_minutes,omitempty"`
	AutoCreateTask  *bool                     `json:"auto_create_task,omitempty"`
	AutoStartTask   *bool                     `json:"auto_start_task,omitempty"`
	TaskOptions     *DiagnosticTaskOptions    `json:"task_options,omitempty"`
}

// InspectionAutoPolicyListData is the paginated auto-policy list payload.
// InspectionAutoPolicyListData 是分页自动策略列表载荷。
type InspectionAutoPolicyListData struct {
	Items    []*InspectionAutoPolicyInfo `json:"items"`
	Total    int64                       `json:"total"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
}
