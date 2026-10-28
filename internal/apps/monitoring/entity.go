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

import "time"

// AlertRule represents persistent alert rule configuration.
// AlertRule 表示持久化的告警规则配置。
type AlertRule struct {
	ID            uint          `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID     uint          `json:"cluster_id" gorm:"not null;index;uniqueIndex:ux_monitoring_cluster_rule"`
	RuleKey       string        `json:"rule_key" gorm:"size:80;not null;uniqueIndex:ux_monitoring_cluster_rule"`
	RuleName      string        `json:"rule_name" gorm:"size:120;not null"`
	Description   string        `json:"description" gorm:"type:text"`
	Severity      AlertSeverity `json:"severity" gorm:"size:20;default:warning"`
	Enabled       bool          `json:"enabled" gorm:"default:true"`
	Threshold     int           `json:"threshold" gorm:"default:1"`
	WindowSeconds int           `json:"window_seconds" gorm:"default:300"`
	CreatedAt     time.Time     `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time     `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for AlertRule.
// TableName 指定 AlertRule 表名。
func (AlertRule) TableName() string {
	return "monitoring_alert_rules"
}

// AlertPolicy represents one unified alert policy resource skeleton.
// AlertPolicy 表示统一告警策略资源骨架。
type AlertPolicy struct {
	ID                         uint                       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name                       string                     `json:"name" gorm:"size:160;not null;index"`
	Description                string                     `json:"description" gorm:"type:text"`
	PolicyType                 AlertPolicyBuilderKind     `json:"policy_type" gorm:"size:40;not null;index"`
	TemplateKey                string                     `json:"template_key" gorm:"size:120;index"`
	LegacyRuleKey              string                     `json:"legacy_rule_key" gorm:"size:120;index"`
	ClusterID                  string                     `json:"cluster_id" gorm:"size:64;index"`
	Severity                   AlertSeverity              `json:"severity" gorm:"size:20;default:warning"`
	Enabled                    bool                       `json:"enabled" gorm:"default:true"`
	CooldownMinutes            int                        `json:"cooldown_minutes" gorm:"default:0"`
	SendRecovery               bool                       `json:"send_recovery" gorm:"default:true"`
	PromQL                     string                     `json:"promql" gorm:"type:text"`
	ConditionsJSON             string                     `json:"conditions_json" gorm:"type:text"`
	NotificationChannelIDsJSON string                     `json:"notification_channel_ids_json" gorm:"type:text"`
	ReceiverUserIDsJSON        string                     `json:"receiver_user_ids_json" gorm:"type:text"`
	MatchCount                 int                        `json:"match_count" gorm:"default:0"`
	DeliveryCount              int                        `json:"delivery_count" gorm:"default:0"`
	LastMatchedAt              *time.Time                 `json:"last_matched_at"`
	LastDeliveredAt            *time.Time                 `json:"last_delivered_at"`
	LastExecutionStatus        AlertPolicyExecutionStatus `json:"last_execution_status" gorm:"size:20;default:idle"`
	LastExecutionError         string                     `json:"last_execution_error" gorm:"type:text"`
	// TaskID 只用于 sync_job 策略：所属同步任务，0 表示不是任务策略。
	// TaskID is only used by sync_job policies: the owning sync task; 0 means not a task policy.
	TaskID uint `json:"task_id" gorm:"not null;default:0;index"`
	// NotifyTaskOwner 只用于 sync_job 策略：邮件渠道是否同时发给任务创建人。
	// NotifyTaskOwner is only used by sync_job policies: whether email channels also reach the task creator.
	NotifyTaskOwner bool      `json:"notify_task_owner" gorm:"not null;default:false"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for AlertPolicy.
// TableName 指定 AlertPolicy 表名。
func (AlertPolicy) TableName() string {
	return "monitoring_alert_policies"
}

// AlertEventState records manual state operations for one process event.
// AlertEventState 记录针对单条进程事件的人工操作状态。
type AlertEventState struct {
	ID             uint        `json:"id" gorm:"primaryKey;autoIncrement"`
	EventID        uint        `json:"event_id" gorm:"not null;uniqueIndex"`
	ClusterID      uint        `json:"cluster_id" gorm:"not null;index"`
	Status         AlertStatus `json:"status" gorm:"size:20;not null;index"`
	AcknowledgedBy string      `json:"acknowledged_by" gorm:"size:100"`
	AcknowledgedAt *time.Time  `json:"acknowledged_at"`
	SilencedBy     string      `json:"silenced_by" gorm:"size:100"`
	SilencedUntil  *time.Time  `json:"silenced_until" gorm:"index"`
	Note           string      `json:"note" gorm:"type:text"`
	CreatedAt      time.Time   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time   `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for AlertEventState.
// TableName 指定 AlertEventState 表名。
func (AlertEventState) TableName() string {
	return "monitoring_alert_event_states"
}

// AlertState records unified manual handling state for one alert instance.
// AlertState 记录统一告警实例的人工处理状态。
type AlertState struct {
	ID             uint                `json:"id" gorm:"primaryKey;autoIncrement"`
	SourceType     AlertSourceType     `json:"source_type" gorm:"size:40;not null;index"`
	SourceKey      string              `json:"source_key" gorm:"size:255;not null;uniqueIndex"`
	ClusterID      string              `json:"cluster_id" gorm:"size:64;index"`
	HandlingStatus AlertHandlingStatus `json:"handling_status" gorm:"size:20;not null;index"`
	AcknowledgedBy string              `json:"acknowledged_by" gorm:"size:100"`
	AcknowledgedAt *time.Time          `json:"acknowledged_at"`
	SilencedBy     string              `json:"silenced_by" gorm:"size:100"`
	SilencedUntil  *time.Time          `json:"silenced_until" gorm:"index"`
	ClosedBy       string              `json:"closed_by" gorm:"size:100"`
	ClosedAt       *time.Time          `json:"closed_at" gorm:"index"`
	Note           string              `json:"note" gorm:"type:text"`
	CreatedAt      time.Time           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time           `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for AlertState.
// TableName 指定 AlertState 表名。
func (AlertState) TableName() string {
	return "monitoring_alert_states"
}

// NotificationChannelType represents notification channel type.
// NotificationChannelType 表示通知渠道类型。
type NotificationChannelType string

const (
	// NotificationChannelTypeWebhook indicates generic webhook.
	// NotificationChannelTypeWebhook 表示通用 Webhook。
	NotificationChannelTypeWebhook NotificationChannelType = "webhook"
	// NotificationChannelTypeEmail indicates email notification channel.
	// NotificationChannelTypeEmail 表示邮件通知渠道。
	NotificationChannelTypeEmail NotificationChannelType = "email"
	// NotificationChannelTypeWeCom indicates WeCom (企业微信) webhook channel.
	// NotificationChannelTypeWeCom 表示企业微信渠道。
	NotificationChannelTypeWeCom NotificationChannelType = "wecom"
	// NotificationChannelTypeDingTalk indicates DingTalk webhook channel.
	// NotificationChannelTypeDingTalk 表示钉钉渠道。
	NotificationChannelTypeDingTalk NotificationChannelType = "dingtalk"
	// NotificationChannelTypeFeishu indicates Feishu webhook channel.
	// NotificationChannelTypeFeishu 表示飞书渠道。
	NotificationChannelTypeFeishu NotificationChannelType = "feishu"
)

// NotificationChannel represents one alert notification channel.
// NotificationChannel 表示一条告警通知渠道配置。
type NotificationChannel struct {
	ID          uint                    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string                  `json:"name" gorm:"size:120;not null;index"`
	Type        NotificationChannelType `json:"type" gorm:"size:30;not null;index"`
	Enabled     bool                    `json:"enabled"`
	Endpoint    string                  `json:"endpoint" gorm:"size:500"`
	Secret      string                  `json:"secret" gorm:"size:500"`
	ConfigJSON  string                  `json:"config_json" gorm:"type:text"`
	Description string                  `json:"description" gorm:"type:text"`
	CreatedAt   time.Time               `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time               `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for NotificationChannel.
// TableName 指定 NotificationChannel 表名。
func (NotificationChannel) TableName() string {
	return "monitoring_notification_channels"
}

// NotificationRoute represents one route from alert match condition to channel.
// NotificationRoute 表示一条从告警匹配条件到通知渠道的路由规则。
type NotificationRoute struct {
	ID                 uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name               string    `json:"name" gorm:"size:120;not null;index"`
	Enabled            bool      `json:"enabled"`
	SourceType         string    `json:"source_type" gorm:"size:40;index"`
	ClusterID          string    `json:"cluster_id" gorm:"size:64;index"`
	Severity           string    `json:"severity" gorm:"size:20;index"`
	RuleKey            string    `json:"rule_key" gorm:"size:80;index"`
	ChannelID          uint      `json:"channel_id" gorm:"not null;index"`
	SendResolved       bool      `json:"send_resolved"`
	MuteIfAcknowledged bool      `json:"mute_if_acknowledged"`
	MuteIfSilenced     bool      `json:"mute_if_silenced"`
	CreatedAt          time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for NotificationRoute.
// TableName 指定 NotificationRoute 表名。
func (NotificationRoute) TableName() string {
	return "monitoring_notification_routes"
}

// NotificationDelivery represents one notification delivery record.
// NotificationDelivery 表示一条通知投递记录。
type NotificationDelivery struct {
	ID                  uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	AlertID             string     `json:"alert_id" gorm:"size:255;index"`
	SourceType          string     `json:"source_type" gorm:"size:40;index"`
	SourceKey           string     `json:"source_key" gorm:"size:255;index;uniqueIndex:ux_monitoring_delivery_source_channel_event,priority:1"`
	PolicyID            uint       `json:"policy_id" gorm:"index"`
	ClusterID           string     `json:"cluster_id" gorm:"size:64;index"`
	ClusterName         string     `json:"cluster_name" gorm:"size:255;index"`
	AlertName           string     `json:"alert_name" gorm:"size:255;index"`
	ChannelID           uint       `json:"channel_id" gorm:"not null;index;uniqueIndex:ux_monitoring_delivery_source_channel_event,priority:2"`
	ChannelName         string     `json:"channel_name" gorm:"size:120"`
	EventType           string     `json:"event_type" gorm:"size:20;not null;index;uniqueIndex:ux_monitoring_delivery_source_channel_event,priority:3"`
	Status              string     `json:"status" gorm:"size:20;not null;index"`
	AttemptCount        int        `json:"attempt_count" gorm:"default:0"`
	LastError           string     `json:"last_error" gorm:"type:text"`
	RequestPayload      string     `json:"request_payload" gorm:"type:text"`
	ResponseStatusCode  int        `json:"response_status_code"`
	ResponseBodyExcerpt string     `json:"response_body_excerpt" gorm:"type:text"`
	SentAt              *time.Time `json:"sent_at"`
	CreatedAt           time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt           time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for NotificationDelivery.
// TableName 指定 NotificationDelivery 表名。
func (NotificationDelivery) TableName() string {
	return "monitoring_notification_deliveries"
}

// RemoteAlertRecord stores one Alertmanager webhook alert after normalization.
// RemoteAlertRecord 保存标准化后的 Alertmanager webhook 告警记录。
type RemoteAlertRecord struct {
	ID uint `json:"id" gorm:"primaryKey;autoIncrement"`

	Fingerprint string `json:"fingerprint" gorm:"size:255;not null;index;uniqueIndex:ux_monitoring_remote_alert"`
	StartsAt    int64  `json:"starts_at" gorm:"not null;index;uniqueIndex:ux_monitoring_remote_alert"`

	Status      string `json:"status" gorm:"size:32;index"`
	Receiver    string `json:"receiver" gorm:"size:200"`
	AlertName   string `json:"alert_name" gorm:"size:255;index"`
	Severity    string `json:"severity" gorm:"size:64;index"`
	ClusterID   string `json:"cluster_id" gorm:"size:64;index"`
	ClusterName string `json:"cluster_name" gorm:"size:255;index"`
	Env         string `json:"env" gorm:"size:64;index"`

	GeneratorURL string `json:"generator_url" gorm:"size:1024"`
	Summary      string `json:"summary" gorm:"type:text"`
	Description  string `json:"description" gorm:"type:text"`

	LabelsJSON      string `json:"labels_json" gorm:"type:text"`
	AnnotationsJSON string `json:"annotations_json" gorm:"type:text"`

	EndsAt         int64      `json:"ends_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	LastReceivedAt time.Time  `json:"last_received_at" gorm:"index"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies table name for RemoteAlertRecord.
// TableName 指定 RemoteAlertRecord 表名。
func (RemoteAlertRecord) TableName() string {
	return "monitoring_remote_alerts"
}
