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

package sync

import "time"

// EngineEventChannel 记录一个集群的 SeaTunnel 事件上报通道（event-report-http → STX）的开关与健康状态。
// token 不落库：每次用 SessionSecret 与 TokenNonce 做 HMAC 派生，轮换 token 只需更换 nonce。
// EngineEventChannel records the on/off state and health of one cluster's SeaTunnel event-report-http channel.
// The token is never stored: it is derived via HMAC from SessionSecret and TokenNonce, so rotation only swaps the nonce.
type EngineEventChannel struct {
	ClusterID         uint       `json:"cluster_id" gorm:"primaryKey;autoIncrement:false"`
	Enabled           bool       `json:"enabled" gorm:"not null"`
	TokenNonce        string     `json:"-" gorm:"size:64"`
	ReportNonTerminal bool       `json:"report_non_terminal" gorm:"not null"`
	ConfigAppliedAt   *time.Time `json:"config_applied_at,omitempty"`
	ConfigApplyError  string     `json:"config_apply_error,omitempty" gorm:"type:text"`
	LastEventAt       *time.Time `json:"last_event_at,omitempty"`
	LastBatchAt       *time.Time `json:"last_batch_at,omitempty"`
	ReceivedCount     int64      `json:"received_count" gorm:"not null;default:0"`
	DuplicateCount    int64      `json:"duplicate_count" gorm:"not null;default:0"`
	DroppedCount      int64      `json:"dropped_count" gorm:"not null;default:0"`
	ParseErrorCount   int64      `json:"parse_error_count" gorm:"not null;default:0"`
	AuthFailCount     int64      `json:"auth_fail_count" gorm:"not null;default:0"`
	ExpectMissedAt    *time.Time `json:"expect_missed_at,omitempty"`
	ExpectSatisfiedAt *time.Time `json:"expect_satisfied_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回事件通道表名。
// TableName returns the event channel table name.
func (EngineEventChannel) TableName() string {
	return "sync_engine_event_channels"
}

// EngineEvent 是一条清洗、去重后的引擎事件（JOB_STATUS、schema change、生命周期、CDC 延迟分钟桶）。
// EngineEvent is one sanitized, de-duplicated engine event (JOB_STATUS, schema change, lifecycle, CDC delay minute bucket).
type EngineEvent struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID     uint      `json:"cluster_id" gorm:"not null;index:idx_sync_engine_events_cluster_time,priority:1"`
	EngineJobID   string    `json:"engine_job_id" gorm:"size:64;index"`
	JobInstanceID *uint     `json:"job_instance_id,omitempty" gorm:"index"`
	EventType     string    `json:"event_type" gorm:"size:64;not null;index"`
	JobStatus     string    `json:"job_status,omitempty" gorm:"size:32"`
	EventTimeMs   int64     `json:"event_time_ms" gorm:"not null;index:idx_sync_engine_events_cluster_time,priority:2"`
	Payload       JSONMap   `json:"payload" gorm:"type:json"`
	DedupeKey     string    `json:"-" gorm:"size:64;not null;uniqueIndex"`
	ReceivedAt    time.Time `json:"received_at" gorm:"not null;index"`
}

// TableName 返回引擎事件表名。
// TableName returns the engine event table name.
func (EngineEvent) TableName() string {
	return "sync_engine_events"
}
