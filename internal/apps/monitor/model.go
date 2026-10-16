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

// Package monitor provides process monitoring and auto-restart functionality.
// Package monitor 提供进程监控和自动重启功能。
package monitor

import (
	"time"
)

// MonitorConfig represents the monitoring configuration for a cluster.
// MonitorConfig 表示集群的监控配置。
// Requirements: 5.2 - Monitor configuration model
type MonitorConfig struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID       uint       `json:"cluster_id" gorm:"uniqueIndex;not null"` // 关联的集群 ID / Associated cluster ID
	AutoMonitor     bool       `json:"auto_monitor" gorm:"default:true"`       // 是否启用自动监控 / Whether auto monitoring is enabled
	AutoRestart     bool       `json:"auto_restart" gorm:"default:true"`       // 是否启用自动拉起 / Whether auto restart is enabled
	MonitorInterval int        `json:"monitor_interval" gorm:"default:5"`      // 监控间隔（秒）/ Monitor interval (seconds)
	RestartDelay    int        `json:"restart_delay" gorm:"default:10"`        // 重启延迟（秒）/ Restart delay (seconds)
	MaxRestarts     int        `json:"max_restarts" gorm:"default:3"`          // 最大重启次数 / Max restart count
	TimeWindow      int        `json:"time_window" gorm:"default:300"`         // 时间窗口（秒，5分钟）/ Time window (seconds, 5 minutes)
	CooldownPeriod  int        `json:"cooldown_period" gorm:"default:1800"`    // 冷却时间（秒，30分钟）/ Cooldown period (seconds, 30 minutes)
	ConfigVersion   int        `json:"config_version" gorm:"default:1"`        // 配置版本号 / Config version number
	LastSyncAt      *time.Time `json:"last_sync_at"`                           // 最后同步时间 / Last sync time
	CreatedAt       time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for MonitorConfig.
// TableName 指定 MonitorConfig 的表名。
func (MonitorConfig) TableName() string {
	return "monitor_configs"
}

// ProcessEventType represents the type of process event.
// ProcessEventType 表示进程事件类型。
type ProcessEventType string

const (
	// EventTypeStarted indicates the process has started.
	// EventTypeStarted 表示进程已启动。
	EventTypeStarted ProcessEventType = "started"

	// EventTypeStopped indicates the process has stopped normally.
	// EventTypeStopped 表示进程已正常停止。
	EventTypeStopped ProcessEventType = "stopped"

	// EventTypeCrashed indicates the process has crashed unexpectedly.
	// EventTypeCrashed 表示进程意外崩溃。
	EventTypeCrashed ProcessEventType = "crashed"

	// EventTypeRestarted indicates the process has been auto-restarted.
	// EventTypeRestarted 表示进程已被自动重启。
	EventTypeRestarted ProcessEventType = "restarted"

	// EventTypeRestartFailed indicates the auto-restart attempt failed.
	// EventTypeRestartFailed 表示自动重启尝试失败。
	EventTypeRestartFailed ProcessEventType = "restart_failed"

	// EventTypeRestartLimitReached indicates the restart limit has been reached.
	// EventTypeRestartLimitReached 表示已达到重启次数上限。
	EventTypeRestartLimitReached ProcessEventType = "restart_limit_reached"

	// EventTypeClusterRestartRequested indicates a manual cluster restart request was issued.
	// EventTypeClusterRestartRequested 表示触发了一次手动重启请求（集群或节点）。
	EventTypeClusterRestartRequested ProcessEventType = "cluster_restart_requested"

	// EventTypeNodeRestartRequested indicates a manual node restart request was issued.
	// EventTypeNodeRestartRequested 表示触发了一次手动节点重启请求。
	EventTypeNodeRestartRequested ProcessEventType = "node_restart_requested"

	// EventTypeNodeStopRequested indicates a manual node stop request was issued.
	// EventTypeNodeStopRequested 表示触发了一次手动节点停止请求。
	EventTypeNodeStopRequested ProcessEventType = "node_stop_requested"

	// EventTypeNodeOffline indicates one managed node has remained unavailable beyond the grace window.
	// EventTypeNodeOffline 表示某个受管节点在宽限窗口后仍处于不可用状态。
	EventTypeNodeOffline ProcessEventType = "node_offline"

	// EventTypeNodeRecovered indicates one node-offline episode has ended and the node is healthy again.
	// EventTypeNodeRecovered 表示某个节点离线告警阶段已经结束，节点重新恢复健康。
	EventTypeNodeRecovered ProcessEventType = "node_recovered"
)

// ProcessEvent represents a process lifecycle event.
// ProcessEvent 表示进程生命周期事件。
// Requirements: 6.1, 6.2 - Process event model
type ProcessEvent struct {
	ID          uint             `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID   uint             `json:"cluster_id" gorm:"index"`                // 关联的集群 ID / Associated cluster ID
	NodeID      uint             `json:"node_id" gorm:"index;index:idx_process_events_node_type_created,priority:1"`                   // 关联的节点 ID / Associated node ID
	HostID      uint             `json:"host_id" gorm:"index"`                                                                          // 关联的主机 ID / Associated host ID
	EventType   ProcessEventType `json:"event_type" gorm:"size:30;index;index:idx_process_events_node_type_created,priority:2"`        // 事件类型 / Event type
	PID         int              `json:"pid"`                                                                                           // 进程 PID / Process PID
	ProcessName string           `json:"process_name" gorm:"size:100"`                                                                  // 进程名称 / Process name
	InstallDir  string           `json:"install_dir" gorm:"size:255"`                                                                   // 安装目录 / Install directory
	Role        string           `json:"role" gorm:"size:20"`                                                                           // 节点角色 / Node role
	Details     string           `json:"details" gorm:"type:text"`                                                                      // 事件详情（JSON）/ Event details (JSON)
	CreatedAt   time.Time        `json:"created_at" gorm:"autoCreateTime;index;index:idx_process_events_node_type_created,priority:3"`  // 事件时间 / Event time
}

// TableName specifies the table name for ProcessEvent.
// TableName 指定 ProcessEvent 的表名。
func (ProcessEvent) TableName() string {
	return "process_events"
}

// UpdateMonitorConfigRequest represents a request to update monitor config.
// UpdateMonitorConfigRequest 表示更新监控配置的请求。
// Requirements: 5.3 - Update monitor config request
type UpdateMonitorConfigRequest struct {
	AutoMonitor     *bool `json:"auto_monitor"`
	AutoRestart     *bool `json:"auto_restart"`
	MonitorInterval *int  `json:"monitor_interval"`
	RestartDelay    *int  `json:"restart_delay"`
	MaxRestarts     *int  `json:"max_restarts"`
	TimeWindow      *int  `json:"time_window"`
	CooldownPeriod  *int  `json:"cooldown_period"`
}

// Validate validates the update request.
// Validate 验证更新请求。
// Requirements: 5.7 - Config validation
// **Feature: seatunnel-process-monitor, Property 12: 监控配置验证**
// **Validates: Requirements 5.7**
func (r *UpdateMonitorConfigRequest) Validate() error {
	if r.MonitorInterval != nil {
		if *r.MonitorInterval < 1 || *r.MonitorInterval > 60 {
			return ErrInvalidMonitorInterval
		}
	}
	if r.RestartDelay != nil {
		if *r.RestartDelay < 1 || *r.RestartDelay > 300 {
			return ErrInvalidRestartDelay
		}
	}
	if r.MaxRestarts != nil {
		if *r.MaxRestarts < 1 || *r.MaxRestarts > 10 {
			return ErrInvalidMaxRestarts
		}
	}
	if r.TimeWindow != nil {
		if *r.TimeWindow < 60 || *r.TimeWindow > 3600 {
			return ErrInvalidTimeWindow
		}
	}
	if r.CooldownPeriod != nil {
		if *r.CooldownPeriod < 60 || *r.CooldownPeriod > 86400 {
			return ErrInvalidCooldownPeriod
		}
	}
	return nil
}

// ProcessEventFilter represents filter criteria for querying process events.
// ProcessEventFilter 表示查询进程事件的过滤条件。
type ProcessEventFilter struct {
	ClusterID uint             `json:"cluster_id"`
	NodeID    uint             `json:"node_id"`
	HostID    uint             `json:"host_id"`
	EventType ProcessEventType `json:"event_type"`
	StartTime *time.Time       `json:"start_time"`
	EndTime   *time.Time       `json:"end_time"`
	Page      int              `json:"page"`
	PageSize  int              `json:"page_size"`
}

// ProcessEventWithHost represents a process event with host information.
// ProcessEventWithHost 表示带有主机信息的进程事件。
type ProcessEventWithHost struct {
	ProcessEvent
	Hostname string `json:"hostname"` // 主机名 / Hostname
	IP       string `json:"ip"`       // 主机 IP / Host IP
}

// DefaultMonitorConfig returns the default monitor configuration.
// DefaultMonitorConfig 返回默认的监控配置。
// Requirements: 5.2 - Default config for new clusters
// **Feature: seatunnel-process-monitor, Property 14: 新集群默认配置**
// **Validates: Requirements 5.2**
func DefaultMonitorConfig(clusterID uint) *MonitorConfig {
	return &MonitorConfig{
		ClusterID:       clusterID,
		AutoMonitor:     true,
		AutoRestart:     true,
		MonitorInterval: 5,
		RestartDelay:    10,
		MaxRestarts:     3,
		TimeWindow:      300,
		CooldownPeriod:  1800,
		ConfigVersion:   1,
	}
}
