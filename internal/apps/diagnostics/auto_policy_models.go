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
	// ConditionCodePromGCFrequent triggers on frequent GC pauses.
	// ConditionCodePromGCFrequent 在 GC 频繁时触发。
	ConditionCodePromGCFrequent InspectionConditionTemplateCode = "PROM_GC_FREQUENT"
	// ConditionCodePromHeapRising triggers on monotonically rising heap usage.
	// ConditionCodePromHeapRising 在堆内存持续上涨时触发。
	ConditionCodePromHeapRising InspectionConditionTemplateCode = "PROM_HEAP_RISING"
	// ConditionCodePromHeapHigh triggers on high heap utilization.
	// ConditionCodePromHeapHigh 在堆内存使用率高时触发。
	ConditionCodePromHeapHigh InspectionConditionTemplateCode = "PROM_HEAP_HIGH"
	// ConditionCodePromCPUHigh triggers on sustained high CPU usage.
	// ConditionCodePromCPUHigh 在 CPU 持续高负载时触发。
	ConditionCodePromCPUHigh InspectionConditionTemplateCode = "PROM_CPU_HIGH"
	// ConditionCodeErrorSpike triggers on error frequency spike.
	// ConditionCodeErrorSpike 在错误频率激增时触发。
	ConditionCodeErrorSpike InspectionConditionTemplateCode = "ERROR_SPIKE"
	// ConditionCodeNodeUnhealthy triggers on sustained node unhealthy state.
	// ConditionCodeNodeUnhealthy 在节点持续异常时触发。
	ConditionCodeNodeUnhealthy InspectionConditionTemplateCode = "NODE_UNHEALTHY"
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
	// ConditionCategoryPrometheus groups Prometheus metric conditions.
	// ConditionCategoryPrometheus 分组 Prometheus 指标条件。
	ConditionCategoryPrometheus InspectionConditionCategory = "prometheus"
	// ConditionCategoryErrorRate groups error rate conditions.
	// ConditionCategoryErrorRate 分组错误频率条件。
	ConditionCategoryErrorRate InspectionConditionCategory = "error_rate"
	// ConditionCategoryNodeUnhealthy groups node health conditions.
	// ConditionCategoryNodeUnhealthy 分组节点健康条件。
	ConditionCategoryNodeUnhealthy InspectionConditionCategory = "node_unhealthy"
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
		Code:                 ConditionCodePromGCFrequent,
		Category:             ConditionCategoryPrometheus,
		Name:                 bilingualText("GC 频繁", "Frequent GC"),
		Description:          bilingualText("当 GC 频繁相关指标对应的告警持续触发时，自动发起巡检。", "Trigger inspection when the alert for frequent GC keeps firing."),
		DefaultThreshold:     10,
		DefaultWindowMinutes: 5,
	},
	{
		Code:                 ConditionCodePromHeapRising,
		Category:             ConditionCategoryPrometheus,
		Name:                 bilingualText("堆内存持续上涨", "Heap Keeps Rising"),
		Description:          bilingualText("当堆内存持续上涨相关告警触发时，自动发起巡检。", "Trigger inspection when the heap-rising alert is firing."),
		DefaultWindowMinutes: 15,
	},
	{
		Code:                 ConditionCodePromHeapHigh,
		Category:             ConditionCategoryPrometheus,
		Name:                 bilingualText("堆内存使用率高", "High Heap Usage"),
		Description:          bilingualText("当堆内存使用率高相关告警触发时，自动发起巡检。", "Trigger inspection when the high heap usage alert is firing."),
		DefaultThreshold:     85,
		DefaultWindowMinutes: 10,
	},
	{
		Code:                 ConditionCodePromCPUHigh,
		Category:             ConditionCategoryPrometheus,
		Name:                 bilingualText("CPU 持续高负载", "Sustained High CPU"),
		Description:          bilingualText("当 CPU 持续高负载相关告警触发时，自动发起巡检。", "Trigger inspection when the sustained high CPU alert is firing."),
		DefaultThreshold:     90,
		DefaultWindowMinutes: 10,
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
		Code:                 ConditionCodeNodeUnhealthy,
		Category:             ConditionCategoryNodeUnhealthy,
		Name:                 bilingualText("节点持续异常", "Persistent Node Anomaly"),
		Description:          bilingualText("N 个节点异常持续 M 分钟。", "Trigger when N nodes remain abnormal for M minutes."),
		DefaultThreshold:     1,
		DefaultWindowMinutes: 10,
	},
	{
		Code:        ConditionCodeAlertFiring,
		Category:    ConditionCategoryAlertFiring,
		Name:        bilingualText("告警规则触发", "Alert Firing"),
		Description: bilingualText("指定告警规则 firing 时触发巡检。", "Trigger inspection when the specified alert rule is firing."),
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
	ConditionCodePromGCFrequent:    {},
	ConditionCodePromHeapRising:    {},
	ConditionCodePromHeapHigh:      {},
	ConditionCodePromCPUHigh:       {},
	ConditionCodeErrorSpike:        {},
	ConditionCodeNodeUnhealthy:     {},
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
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
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
