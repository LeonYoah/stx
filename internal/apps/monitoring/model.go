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

// Package monitoring provides cluster monitoring overview APIs for STX.
// monitoring 包为 STX 提供集群监控总览 API。
package monitoring

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/monitor"
)

// Response is the standard API response for monitoring endpoints.
// Response 是监控接口的标准响应结构。
type Response struct {
	ErrorMsg string      `json:"error_msg"`
	Data     interface{} `json:"data"`
}

// EventStats summarizes process events.
// EventStats 汇总进程事件统计。
type EventStats struct {
	Started             int64 `json:"started"`
	Stopped             int64 `json:"stopped"`
	Crashed             int64 `json:"crashed"`
	Restarted           int64 `json:"restarted"`
	RestartFailed       int64 `json:"restart_failed"`
	RestartLimitReached int64 `json:"restart_limit_reached"`
	NodeOffline         int64 `json:"node_offline"`
	NodeRecovered       int64 `json:"node_recovered"`
}

// CriticalCount returns the number of critical events.
// CriticalCount 返回关键告警事件数量。
func (e *EventStats) CriticalCount() int64 {
	if e == nil {
		return 0
	}
	return e.Crashed + e.RestartFailed + e.RestartLimitReached + e.NodeOffline
}

// OverviewStats represents global monitoring statistics.
// OverviewStats 表示全局监控统计。
type OverviewStats struct {
	TotalClusters     int   `json:"total_clusters"`
	HealthyClusters   int   `json:"healthy_clusters"`
	UnhealthyClusters int   `json:"unhealthy_clusters"`
	UnknownClusters   int   `json:"unknown_clusters"`
	TotalNodes        int   `json:"total_nodes"`
	OnlineNodes       int   `json:"online_nodes"`
	OfflineNodes      int   `json:"offline_nodes"`
	CrashedEvents24h  int64 `json:"crashed_events_24h"`
	RestartFailed24h  int64 `json:"restart_failed_events_24h"`
	ActiveAlerts1h    int64 `json:"active_alerts_1h"`
}

// ClusterMonitoringSummary represents one cluster in monitoring overview.
// ClusterMonitoringSummary 表示监控总览中的单个集群摘要。
type ClusterMonitoringSummary struct {
	ClusterID              uint       `json:"cluster_id"`
	ClusterName            string     `json:"cluster_name"`
	Status                 string     `json:"status"`
	HealthStatus           string     `json:"health_status"`
	TotalNodes             int        `json:"total_nodes"`
	OnlineNodes            int        `json:"online_nodes"`
	OfflineNodes           int        `json:"offline_nodes"`
	CrashedEvents24h       int64      `json:"crashed_events_24h"`
	RestartFailedEvents24h int64      `json:"restart_failed_events_24h"`
	ActiveAlerts1h         int64      `json:"active_alerts_1h"`
	LastEventAt            *time.Time `json:"last_event_at"`
}

// OverviewData represents the monitoring overview payload.
// OverviewData 表示监控总览数据。
type OverviewData struct {
	GeneratedAt   time.Time                   `json:"generated_at"`
	Stats         *OverviewStats              `json:"stats"`
	EventStats24h *EventStats                 `json:"event_stats_24h"`
	Clusters      []*ClusterMonitoringSummary `json:"clusters"`
}

// ClusterBaseInfo represents basic cluster status in detail API.
// ClusterBaseInfo 表示集群详情中的基础状态。
type ClusterBaseInfo struct {
	ClusterID    uint   `json:"cluster_id"`
	ClusterName  string `json:"cluster_name"`
	Status       string `json:"status"`
	HealthStatus string `json:"health_status"`
}

// ClusterDetailStats represents monitoring stats for one cluster.
// ClusterDetailStats 表示单集群监控统计。
type ClusterDetailStats struct {
	TotalNodes             int   `json:"total_nodes"`
	OnlineNodes            int   `json:"online_nodes"`
	OfflineNodes           int   `json:"offline_nodes"`
	CrashedEvents24h       int64 `json:"crashed_events_24h"`
	RestartFailedEvents24h int64 `json:"restart_failed_events_24h"`
	ActiveAlerts1h         int64 `json:"active_alerts_1h"`
}

// NodeSnapshot represents node runtime snapshot in monitoring detail.
// NodeSnapshot 表示监控详情中的节点运行快照。
type NodeSnapshot struct {
	NodeID     uint   `json:"node_id"`
	HostID     uint   `json:"host_id"`
	HostName   string `json:"host_name"`
	HostIP     string `json:"host_ip"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	IsOnline   bool   `json:"is_online"`
	ProcessPID int    `json:"process_pid"`
}

// ClusterOverviewData represents one cluster monitoring detail payload.
// ClusterOverviewData 表示单集群监控详情数据。
type ClusterOverviewData struct {
	GeneratedAt   time.Time                       `json:"generated_at"`
	Cluster       *ClusterBaseInfo                `json:"cluster"`
	Stats         *ClusterDetailStats             `json:"stats"`
	EventStats24h *EventStats                     `json:"event_stats_24h"`
	EventStats1h  *EventStats                     `json:"event_stats_1h"`
	MonitorConfig *monitor.MonitorConfig          `json:"monitor_config"`
	Nodes         []*NodeSnapshot                 `json:"nodes"`
	RecentEvents  []*monitor.ProcessEventWithHost `json:"recent_events"`
}

// AlertSeverity represents alert severity level.
// AlertSeverity 表示告警严重级别。
type AlertSeverity string

const (
	// AlertSeverityWarning indicates warning-level alerts.
	// AlertSeverityWarning 表示告警级别为 warning。
	AlertSeverityWarning AlertSeverity = "warning"
	// AlertSeverityCritical indicates critical-level alerts.
	// AlertSeverityCritical 表示告警级别为 critical。
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertStatus represents alert lifecycle state.
// AlertStatus 表示告警生命周期状态。
type AlertStatus string

const (
	// AlertStatusFiring indicates the alert is currently active.
	// AlertStatusFiring 表示告警正在触发。
	AlertStatusFiring AlertStatus = "firing"
	// AlertStatusAcknowledged indicates the alert has been acknowledged.
	// AlertStatusAcknowledged 表示告警已确认。
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	// AlertStatusSilenced indicates the alert is temporarily silenced.
	// AlertStatusSilenced 表示告警处于静默状态。
	AlertStatusSilenced AlertStatus = "silenced"
)

// AlertSourceType represents where one unified alert comes from.
// AlertSourceType 表示统一告警的来源类型。
type AlertSourceType string

const (
	// AlertSourceTypeLocalProcessEvent indicates a locally detected process event.
	// AlertSourceTypeLocalProcessEvent 表示本地进程事件告警。
	AlertSourceTypeLocalProcessEvent AlertSourceType = "local_process_event"
	// AlertSourceTypeRemoteAlertmanager indicates a remote alert ingested from Alertmanager.
	// AlertSourceTypeRemoteAlertmanager 表示从 Alertmanager 回流的远程告警。
	AlertSourceTypeRemoteAlertmanager AlertSourceType = "remote_alertmanager"
)

// AlertLifecycleStatus represents the source alert lifecycle.
// AlertLifecycleStatus 表示源告警生命周期状态。
type AlertLifecycleStatus string

const (
	// AlertLifecycleStatusFiring indicates alert is currently firing.
	// AlertLifecycleStatusFiring 表示告警正在触发。
	AlertLifecycleStatusFiring AlertLifecycleStatus = "firing"
	// AlertLifecycleStatusResolved indicates alert is resolved.
	// AlertLifecycleStatusResolved 表示告警已恢复。
	AlertLifecycleStatusResolved AlertLifecycleStatus = "resolved"
)

// AlertDisplayStatus represents the user-facing alert state in alert center.
// AlertDisplayStatus 表示告警中心面向用户展示的主状态。
type AlertDisplayStatus string

const (
	// AlertDisplayStatusFiring indicates the problem still exists.
	// AlertDisplayStatusFiring 表示问题仍然存在。
	AlertDisplayStatusFiring AlertDisplayStatus = "firing"
	// AlertDisplayStatusResolved indicates the problem has recovered.
	// AlertDisplayStatusResolved 表示问题已经恢复。
	AlertDisplayStatusResolved AlertDisplayStatus = "resolved"
	// AlertDisplayStatusClosed indicates the alert incident is manually closed.
	// AlertDisplayStatusClosed 表示该次告警事件已被人工关闭。
	AlertDisplayStatusClosed AlertDisplayStatus = "closed"
)

// AlertHandlingStatus represents manual handling state over a unified alert.
// AlertHandlingStatus 表示统一告警上的人工处理状态。
type AlertHandlingStatus string

const (
	// AlertHandlingStatusPending indicates no manual handling is applied.
	// AlertHandlingStatusPending 表示尚未进行人工处理。
	AlertHandlingStatusPending AlertHandlingStatus = "pending"
	// AlertHandlingStatusAcknowledged indicates alert has been acknowledged.
	// AlertHandlingStatusAcknowledged 表示告警已确认。
	AlertHandlingStatusAcknowledged AlertHandlingStatus = "acknowledged"
	// AlertHandlingStatusSilenced indicates alert is temporarily silenced.
	// AlertHandlingStatusSilenced 表示告警已静默。
	AlertHandlingStatusSilenced AlertHandlingStatus = "silenced"
	// AlertHandlingStatusClosed indicates the alert incident is manually closed.
	// AlertHandlingStatusClosed 表示告警事件已被人工关闭。
	AlertHandlingStatusClosed AlertHandlingStatus = "closed"
)

const (
	// AlertRuleKeyProcessCrashed is the rule key for process crashed events.
	// AlertRuleKeyProcessCrashed 是进程崩溃规则键。
	AlertRuleKeyProcessCrashed = "process_crashed"
	// AlertRuleKeyProcessRestartFailed is the rule key for process restart failed events.
	// AlertRuleKeyProcessRestartFailed 是进程重启失败规则键。
	AlertRuleKeyProcessRestartFailed = "process_restart_failed"
	// AlertRuleKeyProcessRestartLimitReached is the rule key for process restart limit reached events.
	// AlertRuleKeyProcessRestartLimitReached 是达到重启上限规则键。
	AlertRuleKeyProcessRestartLimitReached = "process_restart_limit_reached"
	// AlertRuleKeyClusterRestartRequested is the rule key for manual restart events.
	// AlertRuleKeyClusterRestartRequested 表示手动重启事件（集群或节点）的规则键。
	AlertRuleKeyClusterRestartRequested = "cluster_restart_requested"
	// AlertRuleKeyNodeStopRequested is the rule key for manual node stop events.
	// AlertRuleKeyNodeStopRequested 表示手动节点停止事件的规则键。
	AlertRuleKeyNodeStopRequested = "node_stop_requested"
	// AlertRuleKeyNodeOffline is the rule key for sustained node-unavailable episodes.
	// AlertRuleKeyNodeOffline 表示节点持续不可用阶段的规则键。
	AlertRuleKeyNodeOffline = "node_offline"
)

// AlertFilter represents query filters for alert list.
// AlertFilter 表示告警列表查询过滤条件。
type AlertFilter struct {
	ClusterID uint        `json:"cluster_id"`
	Status    AlertStatus `json:"status"`
	StartTime *time.Time  `json:"start_time"`
	EndTime   *time.Time  `json:"end_time"`
	Page      int         `json:"page"`
	PageSize  int         `json:"page_size"`
}

// AlertStats summarizes alert status counts in current query result.
// AlertStats 汇总当前查询结果的告警状态数量。
type AlertStats struct {
	Firing       int64 `json:"firing"`
	Acknowledged int64 `json:"acknowledged"`
	Silenced     int64 `json:"silenced"`
}

// AlertInstanceFilter represents filters for unified alert instances.
// AlertInstanceFilter 表示统一告警实例查询过滤条件。
type AlertInstanceFilter struct {
	SourceType      AlertSourceType      `json:"source_type"`
	ClusterID       string               `json:"cluster_id"`
	Severity        AlertSeverity        `json:"severity"`
	Status          AlertDisplayStatus   `json:"status"`
	LifecycleStatus AlertLifecycleStatus `json:"lifecycle_status"`
	HandlingStatus  AlertHandlingStatus  `json:"handling_status"`
	StartTime       *time.Time           `json:"start_time"`
	EndTime         *time.Time           `json:"end_time"`
	Page            int                  `json:"page"`
	PageSize        int                  `json:"page_size"`
}

// AlertInstanceStats summarizes unified alert counts.
// AlertInstanceStats 汇总统一告警数量统计。
type AlertInstanceStats struct {
	Firing   int64 `json:"firing"`
	Resolved int64 `json:"resolved"`
	Closed   int64 `json:"closed"`
}

// AlertInstance represents one normalized alert item for frontend.
// AlertInstance 表示返回给前端的统一告警项。
type AlertInstance struct {
	AlertID         string                  `json:"alert_id"`
	SourceType      AlertSourceType         `json:"source_type"`
	ClusterID       string                  `json:"cluster_id"`
	ClusterName     string                  `json:"cluster_name"`
	Severity        AlertSeverity           `json:"severity"`
	AlertName       string                  `json:"alert_name"`
	RuleKey         string                  `json:"rule_key"`
	Summary         string                  `json:"summary"`
	Description     string                  `json:"description"`
	Status          AlertDisplayStatus      `json:"status"`
	LifecycleStatus AlertLifecycleStatus    `json:"lifecycle_status"`
	HandlingStatus  AlertHandlingStatus     `json:"handling_status"`
	CreatedAt       time.Time               `json:"created_at"`
	FiringAt        time.Time               `json:"firing_at"`
	ResolvedAt      *time.Time              `json:"resolved_at,omitempty"`
	LastSeenAt      time.Time               `json:"last_seen_at"`
	AcknowledgedBy  string                  `json:"acknowledged_by,omitempty"`
	AcknowledgedAt  *time.Time              `json:"acknowledged_at,omitempty"`
	SilencedBy      string                  `json:"silenced_by,omitempty"`
	SilencedUntil   *time.Time              `json:"silenced_until,omitempty"`
	ClosedBy        string                  `json:"closed_by,omitempty"`
	ClosedAt        *time.Time              `json:"closed_at,omitempty"`
	LatestNote      string                  `json:"latest_note,omitempty"`
	SourceRef       *AlertInstanceSourceRef `json:"source_ref,omitempty"`
}

// AlertInstanceSourceRef contains original-source reference info for one alert.
// AlertInstanceSourceRef 表示统一告警关联的原始来源信息。
type AlertInstanceSourceRef struct {
	EventID     uint   `json:"event_id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	EventType   string `json:"event_type,omitempty"`
	ProcessName string `json:"process_name,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	Receiver    string `json:"receiver,omitempty"`
	Env         string `json:"env,omitempty"`
}

// AlertInstanceListData represents unified alert list response payload.
// AlertInstanceListData 表示统一告警列表响应数据。
type AlertInstanceListData struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Page        int                 `json:"page"`
	PageSize    int                 `json:"page_size"`
	Total       int64               `json:"total"`
	Stats       *AlertInstanceStats `json:"stats"`
	Alerts      []*AlertInstance    `json:"alerts"`
}

// AlertEvent represents one alert item in alert center.
// AlertEvent 表示告警中心中的单条告警记录。
type AlertEvent struct {
	AlertID          string                   `json:"alert_id"`
	EventID          uint                     `json:"event_id"`
	ClusterID        uint                     `json:"cluster_id"`
	ClusterName      string                   `json:"cluster_name"`
	NodeID           uint                     `json:"node_id"`
	HostID           uint                     `json:"host_id"`
	Hostname         string                   `json:"hostname"`
	IP               string                   `json:"ip"`
	EventType        monitor.ProcessEventType `json:"event_type"`
	Severity         AlertSeverity            `json:"severity"`
	Status           AlertStatus              `json:"status"`
	RuleKey          string                   `json:"rule_key"`
	RuleName         string                   `json:"rule_name"`
	ProcessName      string                   `json:"process_name"`
	PID              int                      `json:"pid"`
	Role             string                   `json:"role"`
	Details          string                   `json:"details"`
	CreatedAt        time.Time                `json:"created_at"`
	AcknowledgedBy   string                   `json:"acknowledged_by,omitempty"`
	AcknowledgedAt   *time.Time               `json:"acknowledged_at,omitempty"`
	SilencedBy       string                   `json:"silenced_by,omitempty"`
	SilencedUntil    *time.Time               `json:"silenced_until,omitempty"`
	LatestActionNote string                   `json:"latest_action_note,omitempty"`
}

// AlertListData represents alert center list payload.
// AlertListData 表示告警中心列表响应数据。
type AlertListData struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Page        int           `json:"page"`
	PageSize    int           `json:"page_size"`
	Total       int64         `json:"total"`
	Stats       *AlertStats   `json:"stats"`
	Alerts      []*AlertEvent `json:"alerts"`
}

// AlertRuleDTO represents a monitoring alert rule for frontend.
// AlertRuleDTO 表示前端可消费的告警规则。
type AlertRuleDTO struct {
	ID            uint          `json:"id"`
	ClusterID     uint          `json:"cluster_id"`
	RuleKey       string        `json:"rule_key"`
	RuleName      string        `json:"rule_name"`
	Description   string        `json:"description"`
	Severity      AlertSeverity `json:"severity"`
	Enabled       bool          `json:"enabled"`
	Threshold     int           `json:"threshold"`
	WindowSeconds int           `json:"window_seconds"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// AlertRuleListData represents rule list payload.
// AlertRuleListData 表示规则列表响应数据。
type AlertRuleListData struct {
	GeneratedAt time.Time       `json:"generated_at"`
	ClusterID   uint            `json:"cluster_id"`
	Rules       []*AlertRuleDTO `json:"rules"`
}

// UpdateAlertRuleRequest represents patch fields for one alert rule.
// UpdateAlertRuleRequest 表示告警规则更新请求。
type UpdateAlertRuleRequest struct {
	RuleName      *string        `json:"rule_name"`
	Description   *string        `json:"description"`
	Severity      *AlertSeverity `json:"severity"`
	Enabled       *bool          `json:"enabled"`
	Threshold     *int           `json:"threshold"`
	WindowSeconds *int           `json:"window_seconds"`
}

// Validate validates update request values.
// Validate 验证规则更新请求参数。
func (r *UpdateAlertRuleRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("empty request")
	}
	if r.Severity != nil {
		if *r.Severity != AlertSeverityWarning && *r.Severity != AlertSeverityCritical {
			return fmt.Errorf("invalid severity")
		}
	}
	if r.Threshold != nil && (*r.Threshold < 1 || *r.Threshold > 9999) {
		return fmt.Errorf("threshold must be between 1 and 9999")
	}
	if r.WindowSeconds != nil && (*r.WindowSeconds < 10 || *r.WindowSeconds > 86400) {
		return fmt.Errorf("window_seconds must be between 10 and 86400")
	}
	return nil
}

// AcknowledgeAlertRequest represents acknowledge action payload.
// AcknowledgeAlertRequest 表示确认告警请求。
type AcknowledgeAlertRequest struct {
	Note string `json:"note"`
}

// SilenceAlertRequest represents silence action payload.
// SilenceAlertRequest 表示静默告警请求。
type SilenceAlertRequest struct {
	DurationMinutes int    `json:"duration_minutes"`
	Note            string `json:"note"`
}

// Validate validates silence request.
// Validate 验证静默请求参数。
func (r *SilenceAlertRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("empty request")
	}
	if r.DurationMinutes < 1 || r.DurationMinutes > 7*24*60 {
		return fmt.Errorf("duration_minutes must be between 1 and 10080")
	}
	return nil
}

// AlertActionResult represents action result on one alert.
// AlertActionResult 表示单条告警动作执行结果。
type AlertActionResult struct {
	EventID          uint        `json:"event_id"`
	Status           AlertStatus `json:"status"`
	AcknowledgedBy   string      `json:"acknowledged_by,omitempty"`
	AcknowledgedAt   *time.Time  `json:"acknowledged_at,omitempty"`
	SilencedBy       string      `json:"silenced_by,omitempty"`
	SilencedUntil    *time.Time  `json:"silenced_until,omitempty"`
	LatestActionNote string      `json:"latest_action_note,omitempty"`
}

// AlertInstanceActionResult represents action result on one unified alert instance.
// AlertInstanceActionResult 表示统一告警实例上的动作结果。
type AlertInstanceActionResult struct {
	AlertID        string              `json:"alert_id"`
	Status         AlertDisplayStatus  `json:"status,omitempty"`
	HandlingStatus AlertHandlingStatus `json:"handling_status"`
	AcknowledgedBy string              `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time          `json:"acknowledged_at,omitempty"`
	SilencedBy     string              `json:"silenced_by,omitempty"`
	SilencedUntil  *time.Time          `json:"silenced_until,omitempty"`
	ClosedBy       string              `json:"closed_by,omitempty"`
	ClosedAt       *time.Time          `json:"closed_at,omitempty"`
	LatestNote     string              `json:"latest_note,omitempty"`
}

// IntegrationComponentStatus represents one monitoring stack component status.
// IntegrationComponentStatus 表示监控栈组件状态。
type IntegrationComponentStatus struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Healthy    bool   `json:"healthy"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// IntegrationStatusData represents full integration status payload.
// IntegrationStatusData 表示集成状态响应数据。
type IntegrationStatusData struct {
	GeneratedAt time.Time                     `json:"generated_at"`
	Components  []*IntegrationComponentStatus `json:"components"`
}

// AlertPolicyCapabilityStatus represents one capability state in policy center.
// AlertPolicyCapabilityStatus 表示策略中心中的一项能力状态。
type AlertPolicyCapabilityStatus string

const (
	// AlertPolicyCapabilityStatusAvailable indicates capability is ready to use.
	// AlertPolicyCapabilityStatusAvailable 表示能力已可用。
	AlertPolicyCapabilityStatusAvailable AlertPolicyCapabilityStatus = "available"
	// AlertPolicyCapabilityStatusUnavailable indicates capability is currently unavailable.
	// AlertPolicyCapabilityStatusUnavailable 表示能力当前不可用。
	AlertPolicyCapabilityStatusUnavailable AlertPolicyCapabilityStatus = "unavailable"
	// AlertPolicyCapabilityStatusPlanned indicates capability is planned but not implemented yet.
	// AlertPolicyCapabilityStatusPlanned 表示能力已规划但尚未实现。
	AlertPolicyCapabilityStatusPlanned AlertPolicyCapabilityStatus = "planned"
)

const (
	// AlertPolicyCapabilityKeyPlatformHealth indicates platform-health based alert policies.
	// AlertPolicyCapabilityKeyPlatformHealth 表示平台健康类告警策略能力。
	AlertPolicyCapabilityKeyPlatformHealth = "platform_health"
	// AlertPolicyCapabilityKeyMetricsTemplates indicates metrics-template based policies.
	// AlertPolicyCapabilityKeyMetricsTemplates 表示指标模板类告警策略能力。
	AlertPolicyCapabilityKeyMetricsTemplates = "metrics_templates"
	// AlertPolicyCapabilityKeyCustomPromQL indicates custom PromQL policy capability.
	// AlertPolicyCapabilityKeyCustomPromQL 表示自定义 PromQL 策略能力。
	AlertPolicyCapabilityKeyCustomPromQL = "custom_promql"
	// AlertPolicyCapabilityKeyRemoteIngest indicates remote Alertmanager ingest capability.
	// AlertPolicyCapabilityKeyRemoteIngest 表示远程 Alertmanager 回流能力。
	AlertPolicyCapabilityKeyRemoteIngest = "remote_ingest"
	// AlertPolicyCapabilityKeyWebhookNotification indicates webhook/IM notification delivery capability.
	// AlertPolicyCapabilityKeyWebhookNotification 表示 Webhook/IM 通知投递能力。
	AlertPolicyCapabilityKeyWebhookNotification = "webhook_notification"
	// AlertPolicyCapabilityKeyInAppNotification indicates in-app notification center capability.
	// AlertPolicyCapabilityKeyInAppNotification 表示站内通知中心能力。
	AlertPolicyCapabilityKeyInAppNotification = "in_app_notification"
)

// AlertPolicyBuilderKind represents one policy-builder experience in UI.
// AlertPolicyBuilderKind 表示前端策略构建器类型。
type AlertPolicyBuilderKind string

const (
	// AlertPolicyBuilderKindPlatformHealth represents platform-health policy builder.
	// AlertPolicyBuilderKindPlatformHealth 表示平台健康策略构建器。
	AlertPolicyBuilderKindPlatformHealth AlertPolicyBuilderKind = "platform_health"
	// AlertPolicyBuilderKindMetricsTemplate represents metrics-template policy builder.
	// AlertPolicyBuilderKindMetricsTemplate 表示指标模板策略构建器。
	AlertPolicyBuilderKindMetricsTemplate AlertPolicyBuilderKind = "metrics_template"
	// AlertPolicyBuilderKindCustomPromQL represents custom PromQL policy builder.
	// AlertPolicyBuilderKindCustomPromQL 表示自定义 PromQL 策略构建器。
	AlertPolicyBuilderKindCustomPromQL AlertPolicyBuilderKind = "custom_promql"
)

// AlertPolicyExecutionStatus represents the latest execution result of one unified alert policy.
// AlertPolicyExecutionStatus 表示统一告警策略最近一次执行结果。
type AlertPolicyExecutionStatus string

const (
	// AlertPolicyExecutionStatusIdle indicates the policy has not matched any event yet.
	// AlertPolicyExecutionStatusIdle 表示策略尚未匹配到任何事件。
	AlertPolicyExecutionStatusIdle AlertPolicyExecutionStatus = "idle"
	// AlertPolicyExecutionStatusMatched indicates the policy matched but no delivery was executed.
	// AlertPolicyExecutionStatusMatched 表示策略已匹配但尚未执行通知投递。
	AlertPolicyExecutionStatusMatched AlertPolicyExecutionStatus = "matched"
	// AlertPolicyExecutionStatusSent indicates every attempted delivery succeeded or was already deduplicated as sent.
	// AlertPolicyExecutionStatusSent 表示本次执行的通知已成功发送或已被去重识别为已发送。
	AlertPolicyExecutionStatusSent AlertPolicyExecutionStatus = "sent"
	// AlertPolicyExecutionStatusFailed indicates every attempted delivery failed.
	// AlertPolicyExecutionStatusFailed 表示本次执行的通知全部失败。
	AlertPolicyExecutionStatusFailed AlertPolicyExecutionStatus = "failed"
	// AlertPolicyExecutionStatusPartial indicates some deliveries succeeded while others failed.
	// AlertPolicyExecutionStatusPartial 表示本次执行的通知部分成功、部分失败。
	AlertPolicyExecutionStatusPartial AlertPolicyExecutionStatus = "partial"
)

// AlertPolicyCapabilityDTO represents one capability card in the unified policy center.
// AlertPolicyCapabilityDTO 表示统一策略中心中的能力卡片。
type AlertPolicyCapabilityDTO struct {
	Key       string                      `json:"key"`
	Title     string                      `json:"title"`
	Summary   string                      `json:"summary"`
	Status    AlertPolicyCapabilityStatus `json:"status"`
	Reason    string                      `json:"reason,omitempty"`
	DependsOn []string                    `json:"depends_on,omitempty"`
}

// AlertPolicyBuilderDTO represents one builder entry in the policy center.
// AlertPolicyBuilderDTO 表示策略中心中的构建器入口。
type AlertPolicyBuilderDTO struct {
	Key           AlertPolicyBuilderKind      `json:"key"`
	Title         string                      `json:"title"`
	Description   string                      `json:"description"`
	Status        AlertPolicyCapabilityStatus `json:"status"`
	CapabilityKey string                      `json:"capability_key"`
	Recommended   bool                        `json:"recommended"`
}

// AlertPolicyTemplateSummaryDTO represents one recommended policy template summary.
// AlertPolicyTemplateSummaryDTO 表示一条推荐策略模板摘要。
type AlertPolicyTemplateSummaryDTO struct {
	Key               string   `json:"key"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Category          string   `json:"category"`
	SourceKind        string   `json:"source_kind"`
	CapabilityKey     string   `json:"capability_key"`
	LegacyRuleKey     string   `json:"legacy_rule_key,omitempty"`
	MetricSource      string   `json:"metric_source,omitempty"`
	RequiredSignals   []string `json:"required_signals,omitempty"`
	SuggestedPromQL   string   `json:"suggested_promql,omitempty"`
	DefaultOperator   string   `json:"default_operator,omitempty"`
	DefaultThreshold  string   `json:"default_threshold,omitempty"`
	DefaultWindowMins int      `json:"default_window_minutes,omitempty"`
	Recommended       bool     `json:"recommended"`
}

// NotificationRecipientUserDTO represents one selectable notification recipient user.
// NotificationRecipientUserDTO 表示一条可选通知接收用户记录。
type NotificationRecipientUserDTO struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	IsAdmin  bool   `json:"is_admin"`
}

// AlertPolicyCenterBootstrapData represents bootstrap payload for unified policy center.
// AlertPolicyCenterBootstrapData 表示统一策略中心的初始化数据。
type AlertPolicyCenterBootstrapData struct {
	GeneratedAt            time.Time                        `json:"generated_at"`
	CapabilityMode         string                           `json:"capability_mode"`
	Capabilities           []*AlertPolicyCapabilityDTO      `json:"capabilities"`
	Builders               []*AlertPolicyBuilderDTO         `json:"builders"`
	Templates              []*AlertPolicyTemplateSummaryDTO `json:"templates"`
	Components             []*IntegrationComponentStatus    `json:"components"`
	NotifiableUsers        []*NotificationRecipientUserDTO  `json:"notifiable_users"`
	DefaultReceiverUserIDs []uint64                         `json:"default_receiver_user_ids"`
}

// AlertPolicyConditionDTO represents one condition item in a persisted alert policy.
// AlertPolicyConditionDTO 表示持久化告警策略中的一条条件项。
type AlertPolicyConditionDTO struct {
	MetricKey     string `json:"metric_key"`
	Operator      string `json:"operator"`
	Threshold     string `json:"threshold"`
	WindowMinutes int    `json:"window_minutes"`
}

// NotificationEmailSecurity represents SMTP transport security mode.
// NotificationEmailSecurity 表示 SMTP 传输安全模式。
type NotificationEmailSecurity string

const (
	// NotificationEmailSecurityNone indicates plain SMTP without TLS upgrade.
	// NotificationEmailSecurityNone 表示不使用 TLS 升级的明文 SMTP。
	NotificationEmailSecurityNone NotificationEmailSecurity = "none"
	// NotificationEmailSecurityStartTLS indicates STARTTLS upgrade after connect.
	// NotificationEmailSecurityStartTLS 表示连接后使用 STARTTLS 升级。
	NotificationEmailSecurityStartTLS NotificationEmailSecurity = "starttls"
	// NotificationEmailSecuritySSL indicates implicit TLS / SMTPS.
	// NotificationEmailSecuritySSL 表示隐式 TLS / SMTPS。
	NotificationEmailSecuritySSL NotificationEmailSecurity = "ssl"
)

// NotificationChannelEmailConfig represents one SMTP-backed email channel config.
// NotificationChannelEmailConfig 表示一份基于 SMTP 的邮件渠道配置。
type NotificationChannelEmailConfig struct {
	Protocol   string                    `json:"protocol"`
	Security   NotificationEmailSecurity `json:"security"`
	Host       string                    `json:"host"`
	Port       int                       `json:"port"`
	Username   string                    `json:"username,omitempty"`
	Password   string                    `json:"password,omitempty"`
	From       string                    `json:"from"`
	FromName   string                    `json:"from_name,omitempty"`
	Recipients []string                  `json:"recipients"`
}

// Validate validates email channel config values.
// Validate 校验邮件渠道配置参数。
func (c *NotificationChannelEmailConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("email config is required")
	}
	protocol := strings.ToLower(strings.TrimSpace(c.Protocol))
	if protocol == "" {
		protocol = "smtp"
	}
	if protocol != "smtp" {
		return fmt.Errorf("email protocol must be smtp")
	}
	switch NotificationEmailSecurity(strings.ToLower(strings.TrimSpace(string(c.Security)))) {
	case "", NotificationEmailSecurityNone, NotificationEmailSecurityStartTLS, NotificationEmailSecuritySSL:
	default:
		return fmt.Errorf("invalid email security mode")
	}
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("email host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("email port must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Username) != "" && strings.TrimSpace(c.Password) == "" {
		return fmt.Errorf("email password is required when username is set")
	}
	if strings.TrimSpace(c.From) == "" {
		return fmt.Errorf("email from is required")
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(c.From)); err != nil {
		return fmt.Errorf("invalid email from address")
	}
	for _, recipient := range c.Recipients {
		if strings.TrimSpace(recipient) == "" {
			return fmt.Errorf("email recipient must not be empty")
		}
		if _, err := mail.ParseAddress(strings.TrimSpace(recipient)); err != nil {
			return fmt.Errorf("invalid email recipient address")
		}
	}
	return nil
}

// Normalize applies default values to email config.
// Normalize 为邮件配置补齐默认值。
func (c *NotificationChannelEmailConfig) Normalize() *NotificationChannelEmailConfig {
	if c == nil {
		return nil
	}
	normalized := *c
	normalized.Protocol = strings.ToLower(strings.TrimSpace(normalized.Protocol))
	if normalized.Protocol == "" {
		normalized.Protocol = "smtp"
	}
	normalized.Security = NotificationEmailSecurity(strings.ToLower(strings.TrimSpace(string(normalized.Security))))
	if normalized.Security == "" {
		normalized.Security = NotificationEmailSecuritySSL
	}
	normalized.Host = strings.TrimSpace(normalized.Host)
	normalized.Username = strings.TrimSpace(normalized.Username)
	normalized.Password = strings.TrimSpace(normalized.Password)
	normalized.From = strings.TrimSpace(normalized.From)
	normalized.FromName = strings.TrimSpace(normalized.FromName)
	normalized.Recipients = normalizeEmailRecipients(normalized.Recipients)
	return &normalized
}

// NotificationChannelConfig represents channel-type-specific structured config.
// NotificationChannelConfig 表示按渠道类型区分的结构化配置。
type NotificationChannelConfig struct {
	Email *NotificationChannelEmailConfig `json:"email,omitempty"`
}

func normalizeEmailRecipients(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

// AlertPolicyDTO represents one unified alert policy resource for frontend.
// AlertPolicyDTO 表示前端使用的统一告警策略资源。
type AlertPolicyDTO struct {
	ID                     uint                       `json:"id"`
	Name                   string                     `json:"name"`
	Description            string                     `json:"description"`
	PolicyType             AlertPolicyBuilderKind     `json:"policy_type"`
	TemplateKey            string                     `json:"template_key,omitempty"`
	LegacyRuleKey          string                     `json:"legacy_rule_key,omitempty"`
	ClusterID              string                     `json:"cluster_id,omitempty"`
	Severity               AlertSeverity              `json:"severity"`
	Enabled                bool                       `json:"enabled"`
	CooldownMinutes        int                        `json:"cooldown_minutes"`
	SendRecovery           bool                       `json:"send_recovery"`
	PromQL                 string                     `json:"promql,omitempty"`
	Conditions             []*AlertPolicyConditionDTO `json:"conditions"`
	NotificationChannelIDs []uint                     `json:"notification_channel_ids"`
	ReceiverUserIDs        []uint64                   `json:"receiver_user_ids"`
	MatchCount             int                        `json:"match_count"`
	DeliveryCount          int                        `json:"delivery_count"`
	LastMatchedAt          *time.Time                 `json:"last_matched_at,omitempty"`
	LastDeliveredAt        *time.Time                 `json:"last_delivered_at,omitempty"`
	LastExecutionStatus    AlertPolicyExecutionStatus `json:"last_execution_status"`
	LastExecutionError     string                     `json:"last_execution_error,omitempty"`
	CreatedAt              time.Time                  `json:"created_at"`
	UpdatedAt              time.Time                  `json:"updated_at"`
}

// AlertPolicyListData represents alert policy list payload.
// AlertPolicyListData 表示告警策略列表响应数据。
type AlertPolicyListData struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Total       int               `json:"total"`
	Policies    []*AlertPolicyDTO `json:"policies"`
}

// UpsertAlertPolicyRequest represents create/update payload for one alert policy resource.
// UpsertAlertPolicyRequest 表示新增/更新统一告警策略的请求体。
type UpsertAlertPolicyRequest struct {
	Name                   string                     `json:"name"`
	Description            string                     `json:"description"`
	PolicyType             AlertPolicyBuilderKind     `json:"policy_type"`
	TemplateKey            string                     `json:"template_key"`
	LegacyRuleKey          string                     `json:"legacy_rule_key"`
	ClusterID              string                     `json:"cluster_id"`
	Severity               AlertSeverity              `json:"severity"`
	Enabled                *bool                      `json:"enabled"`
	CooldownMinutes        *int                       `json:"cooldown_minutes"`
	SendRecovery           *bool                      `json:"send_recovery"`
	PromQL                 string                     `json:"promql"`
	Conditions             []*AlertPolicyConditionDTO `json:"conditions"`
	NotificationChannelIDs []uint                     `json:"notification_channel_ids"`
	ReceiverUserIDs        []uint64                   `json:"receiver_user_ids"`
}

// Validate validates alert policy request values.
// Validate 校验统一告警策略请求参数。
func (r *UpsertAlertPolicyRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("empty request")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch r.PolicyType {
	case AlertPolicyBuilderKindPlatformHealth, AlertPolicyBuilderKindMetricsTemplate, AlertPolicyBuilderKindCustomPromQL:
	default:
		return fmt.Errorf("invalid policy_type")
	}
	switch r.Severity {
	case AlertSeverityWarning, AlertSeverityCritical:
	default:
		return fmt.Errorf("invalid severity")
	}
	if r.CooldownMinutes != nil && (*r.CooldownMinutes < 0 || *r.CooldownMinutes > 7*24*60) {
		return fmt.Errorf("cooldown_minutes must be between 0 and 10080")
	}
	switch r.PolicyType {
	case AlertPolicyBuilderKindPlatformHealth, AlertPolicyBuilderKindMetricsTemplate:
		if strings.TrimSpace(r.TemplateKey) == "" {
			return fmt.Errorf("template_key is required")
		}
	case AlertPolicyBuilderKindCustomPromQL:
		if strings.TrimSpace(r.PromQL) == "" {
			return fmt.Errorf("promql is required")
		}
	}
	for _, condition := range r.Conditions {
		if condition == nil {
			return fmt.Errorf("conditions must not contain null")
		}
		if strings.TrimSpace(condition.Operator) == "" {
			return fmt.Errorf("condition operator is required")
		}
		switch strings.TrimSpace(condition.Operator) {
		case ">", ">=", "<", "<=", "==", "!=":
		default:
			return fmt.Errorf("invalid condition operator")
		}
		if condition.WindowMinutes < 0 || condition.WindowMinutes > 7*24*60 {
			return fmt.Errorf("condition window_minutes must be between 0 and 10080")
		}
	}
	for _, receiverUserID := range r.ReceiverUserIDs {
		if receiverUserID == 0 {
			return fmt.Errorf("receiver_user_ids must not contain zero")
		}
	}
	return nil
}

// NotifiableUserListData represents selectable notification recipient users.
// NotifiableUserListData 表示可选通知接收用户列表数据。
type NotifiableUserListData struct {
	GeneratedAt            time.Time                       `json:"generated_at"`
	Users                  []*NotificationRecipientUserDTO `json:"users"`
	DefaultReceiverUserIDs []uint64                        `json:"default_receiver_user_ids"`
}

// NotificationChannelDTO is frontend DTO for notification channels.
// NotificationChannelDTO 是前端使用的通知渠道 DTO。
type NotificationChannelDTO struct {
	ID          uint                       `json:"id"`
	Name        string                     `json:"name"`
	Type        NotificationChannelType    `json:"type"`
	Enabled     bool                       `json:"enabled"`
	Endpoint    string                     `json:"endpoint"`
	Secret      string                     `json:"secret"`
	Config      *NotificationChannelConfig `json:"config,omitempty"`
	Description string                     `json:"description"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
}

// NotificationChannelListData represents channel list payload.
// NotificationChannelListData 表示通知渠道列表响应。
type NotificationChannelListData struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Total       int                       `json:"total"`
	Channels    []*NotificationChannelDTO `json:"channels"`
}

// NotificationChannelTestResult represents one test-send result for a channel.
// NotificationChannelTestResult 表示通知渠道一次测试发送结果。
type NotificationChannelTestResult struct {
	ChannelID    uint       `json:"channel_id"`
	DeliveryID   uint       `json:"delivery_id"`
	Status       string     `json:"status"`
	Receiver     string     `json:"receiver,omitempty"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	StatusCode   int        `json:"status_code,omitempty"`
	ResponseBody string     `json:"response_body,omitempty"`
}

// NotificationChannelConnectionTestResult represents one SMTP connection test result.
// NotificationChannelConnectionTestResult 表示一次 SMTP 连接测试结果。
type NotificationChannelConnectionTestResult struct {
	Status    string     `json:"status"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	LastError string     `json:"last_error,omitempty"`
}

// NotificationChannelTestRequest represents manual test-send options for one notification channel.
// NotificationChannelTestRequest 表示通知渠道手动测试发送的请求参数。
type NotificationChannelTestRequest struct {
	ReceiverUserID uint64 `json:"receiver_user_id"`
}

// NotificationChannelDraftTestRequest represents manual test-send options for one unsaved notification channel draft.
// NotificationChannelDraftTestRequest 表示未保存通知渠道草稿的手动测试发送请求参数。
type NotificationChannelDraftTestRequest struct {
	Channel        *UpsertNotificationChannelRequest `json:"channel"`
	ReceiverUserID uint64                            `json:"receiver_user_id"`
}

// Validate checks whether the draft test request contains a valid channel payload.
// Validate 校验草稿测试发送请求是否包含合法的渠道配置。
func (r *NotificationChannelDraftTestRequest) Validate() error {
	if r == nil || r.Channel == nil {
		return fmt.Errorf("channel is required")
	}
	return r.Channel.Validate()
}

// NotificationDeliveryEventType represents delivery lifecycle event type.
// NotificationDeliveryEventType 表示通知投递事件类型。
type NotificationDeliveryEventType string

const (
	// NotificationDeliveryEventTypeFiring indicates a firing alert notification.
	// NotificationDeliveryEventTypeFiring 表示触发通知。
	NotificationDeliveryEventTypeFiring NotificationDeliveryEventType = "firing"
	// NotificationDeliveryEventTypeResolved indicates a resolved alert notification.
	// NotificationDeliveryEventTypeResolved 表示恢复通知。
	NotificationDeliveryEventTypeResolved NotificationDeliveryEventType = "resolved"
	// NotificationDeliveryEventTypeTest indicates a manual test notification.
	// NotificationDeliveryEventTypeTest 表示手动测试通知。
	NotificationDeliveryEventTypeTest NotificationDeliveryEventType = "test"
)

// NotificationDeliveryStatus represents current delivery execution state.
// NotificationDeliveryStatus 表示当前通知投递状态。
type NotificationDeliveryStatus string

const (
	// NotificationDeliveryStatusPending indicates delivery is pending.
	// NotificationDeliveryStatusPending 表示投递待执行。
	NotificationDeliveryStatusPending NotificationDeliveryStatus = "pending"
	// NotificationDeliveryStatusSending indicates delivery is being sent.
	// NotificationDeliveryStatusSending 表示投递发送中。
	NotificationDeliveryStatusSending NotificationDeliveryStatus = "sending"
	// NotificationDeliveryStatusSent indicates delivery succeeded.
	// NotificationDeliveryStatusSent 表示投递成功。
	NotificationDeliveryStatusSent NotificationDeliveryStatus = "sent"
	// NotificationDeliveryStatusFailed indicates delivery failed.
	// NotificationDeliveryStatusFailed 表示投递失败。
	NotificationDeliveryStatusFailed NotificationDeliveryStatus = "failed"
	// NotificationDeliveryStatusRetrying indicates delivery is retrying.
	// NotificationDeliveryStatusRetrying 表示投递重试中。
	NotificationDeliveryStatusRetrying NotificationDeliveryStatus = "retrying"
	// NotificationDeliveryStatusCanceled indicates delivery is canceled.
	// NotificationDeliveryStatusCanceled 表示投递已取消。
	NotificationDeliveryStatusCanceled NotificationDeliveryStatus = "canceled"
)

// NotificationRouteDTO is frontend DTO for notification routes.
// NotificationRouteDTO 是前端使用的通知路由 DTO。
type NotificationRouteDTO struct {
	ID                 uint      `json:"id"`
	Name               string    `json:"name"`
	Enabled            bool      `json:"enabled"`
	SourceType         string    `json:"source_type,omitempty"`
	ClusterID          string    `json:"cluster_id,omitempty"`
	Severity           string    `json:"severity,omitempty"`
	RuleKey            string    `json:"rule_key,omitempty"`
	ChannelID          uint      `json:"channel_id"`
	SendResolved       bool      `json:"send_resolved"`
	MuteIfAcknowledged bool      `json:"mute_if_acknowledged"`
	MuteIfSilenced     bool      `json:"mute_if_silenced"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// NotificationRouteListData represents notification route list payload.
// NotificationRouteListData 表示通知路由列表响应。
type NotificationRouteListData struct {
	GeneratedAt time.Time               `json:"generated_at"`
	Total       int                     `json:"total"`
	Routes      []*NotificationRouteDTO `json:"routes"`
}

// NotificationDeliveryFilter represents notification history query filters.
// NotificationDeliveryFilter 表示通知历史查询过滤条件。
type NotificationDeliveryFilter struct {
	ChannelID uint                          `json:"channel_id"`
	PolicyID  uint                          `json:"policy_id"`
	Status    NotificationDeliveryStatus    `json:"status"`
	EventType NotificationDeliveryEventType `json:"event_type"`
	ClusterID string                        `json:"cluster_id"`
	StartTime *time.Time                    `json:"start_time"`
	EndTime   *time.Time                    `json:"end_time"`
	Page      int                           `json:"page"`
	PageSize  int                           `json:"page_size"`
}

// NotificationDeliveryDTO is frontend DTO for notification history entries.
// NotificationDeliveryDTO 是前端使用的通知历史 DTO。
type NotificationDeliveryDTO struct {
	ID                 uint       `json:"id"`
	AlertID            string     `json:"alert_id"`
	SourceType         string     `json:"source_type"`
	SourceKey          string     `json:"source_key"`
	PolicyID           uint       `json:"policy_id"`
	ClusterID          string     `json:"cluster_id,omitempty"`
	ClusterName        string     `json:"cluster_name,omitempty"`
	AlertName          string     `json:"alert_name,omitempty"`
	ChannelID          uint       `json:"channel_id"`
	ChannelName        string     `json:"channel_name,omitempty"`
	EventType          string     `json:"event_type"`
	Status             string     `json:"status"`
	AttemptCount       int        `json:"attempt_count"`
	LastError          string     `json:"last_error,omitempty"`
	ResponseStatusCode int        `json:"response_status_code,omitempty"`
	SentAt             *time.Time `json:"sent_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// NotificationDeliveryListData represents notification history list payload.
// NotificationDeliveryListData 表示通知历史列表响应数据。
type NotificationDeliveryListData struct {
	GeneratedAt time.Time                  `json:"generated_at"`
	Page        int                        `json:"page"`
	PageSize    int                        `json:"page_size"`
	Total       int64                      `json:"total"`
	Deliveries  []*NotificationDeliveryDTO `json:"deliveries"`
}

// UpsertNotificationRouteRequest represents create/update route payload.
// UpsertNotificationRouteRequest 表示新增/更新通知路由请求。
type UpsertNotificationRouteRequest struct {
	Name               string `json:"name"`
	Enabled            *bool  `json:"enabled"`
	SourceType         string `json:"source_type"`
	ClusterID          string `json:"cluster_id"`
	Severity           string `json:"severity"`
	RuleKey            string `json:"rule_key"`
	ChannelID          uint   `json:"channel_id"`
	SendResolved       *bool  `json:"send_resolved"`
	MuteIfAcknowledged *bool  `json:"mute_if_acknowledged"`
	MuteIfSilenced     *bool  `json:"mute_if_silenced"`
}

// Validate validates notification route request.
// Validate 验证通知路由请求参数。
func (r *UpsertNotificationRouteRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("empty request")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if r.ChannelID == 0 {
		return fmt.Errorf("channel_id is required")
	}
	if r.SourceType != "" {
		switch AlertSourceType(strings.TrimSpace(r.SourceType)) {
		case AlertSourceTypeLocalProcessEvent, AlertSourceTypeRemoteAlertmanager:
		default:
			return fmt.Errorf("invalid source_type")
		}
	}
	if r.Severity != "" {
		switch AlertSeverity(strings.TrimSpace(r.Severity)) {
		case AlertSeverityWarning, AlertSeverityCritical:
		default:
			return fmt.Errorf("invalid severity")
		}
	}
	return nil
}

// UpsertNotificationChannelRequest represents create/update channel payload.
// UpsertNotificationChannelRequest 表示新增/更新通知渠道请求。
type UpsertNotificationChannelRequest struct {
	Name        string                     `json:"name"`
	Type        NotificationChannelType    `json:"type"`
	Enabled     *bool                      `json:"enabled"`
	Endpoint    string                     `json:"endpoint"`
	Secret      string                     `json:"secret"`
	Config      *NotificationChannelConfig `json:"config,omitempty"`
	Description string                     `json:"description"`
}

// Validate validates request values.
// Validate 校验通知渠道请求参数。
func (r *UpsertNotificationChannelRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("empty request")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch r.Type {
	case NotificationChannelTypeWebhook, NotificationChannelTypeEmail, NotificationChannelTypeWeCom, NotificationChannelTypeDingTalk, NotificationChannelTypeFeishu:
	default:
		return fmt.Errorf("invalid channel type")
	}
	if r.Type == NotificationChannelTypeEmail {
		if r.Config == nil || r.Config.Email == nil {
			return fmt.Errorf("email config is required")
		}
		if err := r.Config.Email.Validate(); err != nil {
			return err
		}
		return nil
	}
	if strings.TrimSpace(r.Endpoint) == "" {
		return fmt.Errorf("endpoint is required")
	}
	return nil
}

// PrometheusSDTargetGroup is one target group in Prometheus HTTP SD response.
// PrometheusSDTargetGroup 是 Prometheus HTTP SD 响应中的目标组。
type PrometheusSDTargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// AlertmanagerWebhookPayload represents Alertmanager webhook body.
// AlertmanagerWebhookPayload 表示 Alertmanager webhook 请求体。
type AlertmanagerWebhookPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []*WebhookAlert   `json:"alerts"`
}

// WebhookAlert represents one alert item in webhook payload.
// WebhookAlert 表示 webhook 中的单条告警。
type WebhookAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// AlertmanagerWebhookResult represents processing result for one webhook request.
// AlertmanagerWebhookResult 表示单次 webhook 处理结果。
type AlertmanagerWebhookResult struct {
	Received int      `json:"received"`
	Stored   int      `json:"stored"`
	Errors   []string `json:"errors,omitempty"`
}

// RemoteAlertFilter represents query filters for remote alert records.
// RemoteAlertFilter 表示远程告警记录查询过滤条件。
type RemoteAlertFilter struct {
	ClusterID string     `json:"cluster_id"`
	Status    string     `json:"status"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	Page      int        `json:"page"`
	PageSize  int        `json:"page_size"`
}

// RemoteAlertItem represents one remote alert record returned to frontend.
// RemoteAlertItem 表示返回给前端的单条远程告警记录。
type RemoteAlertItem struct {
	ID             uint       `json:"id"`
	Fingerprint    string     `json:"fingerprint"`
	Status         string     `json:"status"`
	Receiver       string     `json:"receiver"`
	AlertName      string     `json:"alert_name"`
	Severity       string     `json:"severity"`
	ClusterID      string     `json:"cluster_id"`
	ClusterName    string     `json:"cluster_name"`
	Env            string     `json:"env"`
	Summary        string     `json:"summary"`
	Description    string     `json:"description"`
	StartsAt       int64      `json:"starts_at"`
	EndsAt         int64      `json:"ends_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	LastReceivedAt time.Time  `json:"last_received_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// RemoteAlertListData represents remote alert list response payload.
// RemoteAlertListData 表示远程告警列表响应数据。
type RemoteAlertListData struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Page        int                `json:"page"`
	PageSize    int                `json:"page_size"`
	Total       int64              `json:"total"`
	Alerts      []*RemoteAlertItem `json:"alerts"`
}

// ClusterHealthItem represents one cluster health summary for monitoring center.
// ClusterHealthItem 表示监控中心中的单集群健康摘要。
type ClusterHealthItem struct {
	ClusterID      uint   `json:"cluster_id"`
	ClusterName    string `json:"cluster_name"`
	Status         string `json:"status"`
	HealthStatus   string `json:"health_status"`
	TotalNodes     int    `json:"total_nodes"`
	OnlineNodes    int    `json:"online_nodes"`
	OfflineNodes   int    `json:"offline_nodes"`
	ActiveAlerts   int64  `json:"active_alerts"`
	CriticalAlerts int64  `json:"critical_alerts"`
}

// ClusterHealthData represents all clusters health payload.
// ClusterHealthData 表示全部集群健康响应数据。
type ClusterHealthData struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Total       int                  `json:"total"`
	Clusters    []*ClusterHealthItem `json:"clusters"`
}

// PlatformHealthData represents platform-level health summary.
// PlatformHealthData 表示平台级健康汇总。
type PlatformHealthData struct {
	GeneratedAt       time.Time `json:"generated_at"`
	HealthStatus      string    `json:"health_status"`
	TotalClusters     int       `json:"total_clusters"`
	HealthyClusters   int       `json:"healthy_clusters"`
	DegradedClusters  int       `json:"degraded_clusters"`
	UnhealthyClusters int       `json:"unhealthy_clusters"`
	UnknownClusters   int       `json:"unknown_clusters"`
	ActiveAlerts      int64     `json:"active_alerts"`
	CriticalAlerts    int64     `json:"critical_alerts"`
}
