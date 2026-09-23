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

import "time"

const (
	// DefaultFingerprintVersion is the fingerprint algorithm version for Seatunnel error grouping.
	// DefaultFingerprintVersion 表示 Seatunnel 错误分组当前使用的指纹算法版本。
	DefaultFingerprintVersion = "v1"
)

// SeatunnelErrorGroup stores one aggregated error fingerprint group.
// SeatunnelErrorGroup 存储一个聚合后的错误指纹分组。
type SeatunnelErrorGroup struct {
	ID                 uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Fingerprint        string    `json:"fingerprint" gorm:"size:64;uniqueIndex;not null"`
	FingerprintVersion string    `json:"fingerprint_version" gorm:"size:20;not null"`
	Title              string    `json:"title" gorm:"type:text"`
	ExceptionClass     string    `json:"exception_class" gorm:"size:255;index"`
	NormalizedText     string    `json:"normalized_text" gorm:"type:text"`
	SampleMessage      string    `json:"sample_message" gorm:"type:text"`
	SampleEvidence     string    `json:"sample_evidence" gorm:"type:text"`
	OccurrenceCount    int64     `json:"occurrence_count" gorm:"default:0"`
	FirstSeenAt        time.Time `json:"first_seen_at" gorm:"index"`
	LastSeenAt         time.Time `json:"last_seen_at" gorm:"index"`
	LastClusterID      uint      `json:"last_cluster_id" gorm:"index"`
	LastNodeID         uint      `json:"last_node_id" gorm:"index"`
	LastHostID         uint      `json:"last_host_id" gorm:"index"`
	CreatedAt          time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for SeatunnelErrorGroup.
// TableName 指定 SeatunnelErrorGroup 的表名。
func (SeatunnelErrorGroup) TableName() string {
	return "diagnostics_error_groups"
}

// SeatunnelErrorEvent stores one structured Seatunnel ERROR occurrence.
// SeatunnelErrorEvent 存储一条结构化 Seatunnel ERROR 事件。
type SeatunnelErrorEvent struct {
	ID             uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	ErrorGroupID   uint      `json:"error_group_id" gorm:"index;not null"`
	Fingerprint    string    `json:"fingerprint" gorm:"size:64;index;not null"`
	ClusterID      uint      `json:"cluster_id" gorm:"index"`
	NodeID         uint      `json:"node_id" gorm:"index"`
	HostID         uint      `json:"host_id" gorm:"index"`
	AgentID        string    `json:"agent_id" gorm:"size:100;index;not null"`
	Role           string    `json:"role" gorm:"size:20;index"`
	InstallDir     string    `json:"install_dir" gorm:"size:255"`
	SourceFile     string    `json:"source_file" gorm:"size:500;index"`
	SourceKind     string    `json:"source_kind" gorm:"size:20;index"`
	JobID          string    `json:"job_id" gorm:"size:100;index"`
	OccurredAt     time.Time `json:"occurred_at" gorm:"index"`
	Message        string    `json:"message" gorm:"type:text"`
	ExceptionClass string    `json:"exception_class" gorm:"size:255;index"`
	NormalizedText string    `json:"normalized_text" gorm:"type:text"`
	Evidence       string    `json:"evidence" gorm:"type:text"`
	CursorStart    int64     `json:"cursor_start"`
	CursorEnd      int64     `json:"cursor_end"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

// TableName specifies the table name for SeatunnelErrorEvent.
// TableName 指定 SeatunnelErrorEvent 的表名。
func (SeatunnelErrorEvent) TableName() string {
	return "diagnostics_error_events"
}

// SeatunnelLogCursor stores the latest processed file offset per source file.
// SeatunnelLogCursor 存储每个来源文件的最新已处理偏移量。
type SeatunnelLogCursor struct {
	ID             uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	AgentID        string     `json:"agent_id" gorm:"size:80;not null;uniqueIndex:idx_diag_log_cursor"`
	HostID         uint       `json:"host_id" gorm:"index"`
	ClusterID      uint       `json:"cluster_id" gorm:"index"`
	NodeID         uint       `json:"node_id" gorm:"index"`
	InstallDir     string     `json:"install_dir" gorm:"size:128;not null;uniqueIndex:idx_diag_log_cursor"`
	Role           string     `json:"role" gorm:"size:20;not null;uniqueIndex:idx_diag_log_cursor"`
	SourceFile     string     `json:"source_file" gorm:"size:255;not null;uniqueIndex:idx_diag_log_cursor"`
	CursorOffset   int64      `json:"cursor_offset"`
	LastOccurredAt *time.Time `json:"last_occurred_at"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for SeatunnelLogCursor.
// TableName 指定 SeatunnelLogCursor 的表名。
func (SeatunnelLogCursor) TableName() string {
	return "diagnostics_log_cursors"
}

// IngestSeatunnelErrorRequest represents one structured Seatunnel error ingestion request.
// IngestSeatunnelErrorRequest 表示一条结构化 Seatunnel 错误入库请求。
type IngestSeatunnelErrorRequest struct {
	ClusterID   uint      `json:"cluster_id"`
	NodeID      uint      `json:"node_id"`
	HostID      uint      `json:"host_id"`
	AgentID     string    `json:"agent_id"`
	Role        string    `json:"role"`
	InstallDir  string    `json:"install_dir"`
	SourceFile  string    `json:"source_file"`
	SourceKind  string    `json:"source_kind"`
	JobID       string    `json:"job_id"`
	OccurredAt  time.Time `json:"occurred_at"`
	Message     string    `json:"message"`
	Evidence    string    `json:"evidence"`
	CursorStart int64     `json:"cursor_start"`
	CursorEnd   int64     `json:"cursor_end"`
}

// SeatunnelErrorGroupFilter defines query filters for error groups.
// SeatunnelErrorGroupFilter 定义错误组查询过滤条件。
type SeatunnelErrorGroupFilter struct {
	ClusterID      uint       `json:"cluster_id"`
	NodeID         uint       `json:"node_id"`
	HostID         uint       `json:"host_id"`
	Role           string     `json:"role"`
	JobID          string     `json:"job_id"`
	Keyword        string     `json:"keyword"`
	ExceptionClass string     `json:"exception_class"`
	StartTime      *time.Time `json:"start_time"`
	EndTime        *time.Time `json:"end_time"`
	Page           int        `json:"page"`
	PageSize       int        `json:"page_size"`
}

// SeatunnelErrorEventFilter defines query filters for error events.
// SeatunnelErrorEventFilter 定义错误事件查询过滤条件。
type SeatunnelErrorEventFilter struct {
	ErrorGroupID   uint       `json:"error_group_id"`
	ClusterID      uint       `json:"cluster_id"`
	NodeID         uint       `json:"node_id"`
	HostID         uint       `json:"host_id"`
	Role           string     `json:"role"`
	JobID          string     `json:"job_id"`
	Keyword        string     `json:"keyword"`
	ExceptionClass string     `json:"exception_class"`
	StartTime      *time.Time `json:"start_time"`
	EndTime        *time.Time `json:"end_time"`
	Page           int        `json:"page"`
	PageSize       int        `json:"page_size"`
}

// SeatunnelErrorGroupInfo is the API view model for an error group.
// SeatunnelErrorGroupInfo 是错误组的 API 视图模型。
type SeatunnelErrorGroupInfo struct {
	ID                 uint      `json:"id"`
	Fingerprint        string    `json:"fingerprint"`
	FingerprintVersion string    `json:"fingerprint_version"`
	Title              string    `json:"title"`
	ExceptionClass     string    `json:"exception_class"`
	SampleMessage      string    `json:"sample_message"`
	OccurrenceCount    int64     `json:"occurrence_count"`
	FirstSeenAt        time.Time `json:"first_seen_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	LastClusterID      uint      `json:"last_cluster_id"`
	LastNodeID         uint      `json:"last_node_id"`
	LastHostID         uint      `json:"last_host_id"`
	LastHostName       string    `json:"last_host_name"`
	LastHostIP         string    `json:"last_host_ip"`
}

// ToInfo converts one group model to response info.
// ToInfo 将错误组模型转换为响应视图。
func (g *SeatunnelErrorGroup) ToInfo(display *DiagnosticHostDisplayContext) *SeatunnelErrorGroupInfo {
	if g == nil {
		return nil
	}
	info := &SeatunnelErrorGroupInfo{
		ID:                 g.ID,
		Fingerprint:        g.Fingerprint,
		FingerprintVersion: g.FingerprintVersion,
		Title:              g.Title,
		ExceptionClass:     g.ExceptionClass,
		SampleMessage:      g.SampleMessage,
		OccurrenceCount:    g.OccurrenceCount,
		FirstSeenAt:        g.FirstSeenAt,
		LastSeenAt:         g.LastSeenAt,
		LastClusterID:      g.LastClusterID,
		LastNodeID:         g.LastNodeID,
		LastHostID:         g.LastHostID,
	}
	if display != nil {
		info.LastHostName = display.HostName
		info.LastHostIP = display.HostIP
	}
	return info
}

// SeatunnelErrorEventInfo is the API view model for an error event.
// SeatunnelErrorEventInfo 是错误事件的 API 视图模型。
type SeatunnelErrorEventInfo struct {
	ID             uint      `json:"id"`
	ErrorGroupID   uint      `json:"error_group_id"`
	Fingerprint    string    `json:"fingerprint"`
	ClusterID      uint      `json:"cluster_id"`
	NodeID         uint      `json:"node_id"`
	HostID         uint      `json:"host_id"`
	HostName       string    `json:"host_name"`
	HostIP         string    `json:"host_ip"`
	AgentID        string    `json:"agent_id"`
	Role           string    `json:"role"`
	InstallDir     string    `json:"install_dir"`
	SourceFile     string    `json:"source_file"`
	SourceKind     string    `json:"source_kind"`
	JobID          string    `json:"job_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	Message        string    `json:"message"`
	ExceptionClass string    `json:"exception_class"`
	Evidence       string    `json:"evidence"`
	CursorStart    int64     `json:"cursor_start"`
	CursorEnd      int64     `json:"cursor_end"`
}

// ToInfo converts one event model to response info.
// ToInfo 将错误事件模型转换为响应视图。
func (e *SeatunnelErrorEvent) ToInfo(display *DiagnosticHostDisplayContext) *SeatunnelErrorEventInfo {
	if e == nil {
		return nil
	}
	info := &SeatunnelErrorEventInfo{
		ID:             e.ID,
		ErrorGroupID:   e.ErrorGroupID,
		Fingerprint:    e.Fingerprint,
		ClusterID:      e.ClusterID,
		NodeID:         e.NodeID,
		HostID:         e.HostID,
		AgentID:        e.AgentID,
		Role:           e.Role,
		InstallDir:     e.InstallDir,
		SourceFile:     e.SourceFile,
		SourceKind:     e.SourceKind,
		JobID:          e.JobID,
		OccurredAt:     e.OccurredAt,
		Message:        e.Message,
		ExceptionClass: e.ExceptionClass,
		Evidence:       e.Evidence,
		CursorStart:    e.CursorStart,
		CursorEnd:      e.CursorEnd,
	}
	if display != nil {
		info.HostName = display.HostName
		info.HostIP = display.HostIP
	}
	return info
}

// SeatunnelErrorGroupsData is the paginated error group payload.
// SeatunnelErrorGroupsData 是分页错误组载荷。
type SeatunnelErrorGroupsData struct {
	Items    []*SeatunnelErrorGroupInfo `json:"items"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}

// SeatunnelErrorEventsData is the paginated error event payload.
// SeatunnelErrorEventsData 是分页错误事件载荷。
type SeatunnelErrorEventsData struct {
	Items    []*SeatunnelErrorEventInfo `json:"items"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}

// SeatunnelErrorGroupDetailData is the detail payload for one error group.
// SeatunnelErrorGroupDetailData 是单个错误组的详情载荷。
type SeatunnelErrorGroupDetailData struct {
	Group  *SeatunnelErrorGroupInfo   `json:"group"`
	Events []*SeatunnelErrorEventInfo `json:"events"`
}

// DiagnosticHostDisplayContext stores host display labels for diagnostics API views.
// DiagnosticHostDisplayContext 存储 diagnostics API 视图的主机展示信息。
type DiagnosticHostDisplayContext struct {
	HostName string
	HostIP   string
}
