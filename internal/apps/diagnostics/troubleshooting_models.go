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

// TroubleshootingTargetType 表示排障经验的目标类型（错误日志或监控告警）
// TroubleshootingTargetType represents target category of troubleshooting memory (error log or alert)
type TroubleshootingTargetType string

const (
	// TargetTypeError 针对错误日志
	// TargetTypeError targets error log events
	TargetTypeError TroubleshootingTargetType = "error"

	// TargetTypeAlert 针对监控告警指标
	// TargetTypeAlert targets monitoring alerts
	TargetTypeAlert TroubleshootingTargetType = "alert"
)

// TroubleshootingMemory stores one verified troubleshooting solution and knowledge entry.
// TroubleshootingMemory 存储一条已验证的排障解决方案与经验沉淀条目。
type TroubleshootingMemory struct {
	ID             uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	TargetType     string    `json:"target_type" gorm:"size:32;index;not null;default:'error'"`
	Fingerprint    string    `json:"fingerprint" gorm:"size:255;index;not null"`
	Title          string    `json:"title" gorm:"size:255;not null"`
	ErrorSummary   string    `json:"error_summary" gorm:"type:text"`
	RootCause      string    `json:"root_cause" gorm:"type:text"`
	Solution       string    `json:"solution" gorm:"type:text;not null"`
	ActionsTaken   string    `json:"actions_taken" gorm:"type:text"`
	PreventiveTips string    `json:"preventive_tips" gorm:"type:text"`
	ClusterID      uint      `json:"cluster_id" gorm:"index;default:0"`
	ClusterName    string    `json:"cluster_name" gorm:"size:100"`
	Tags           string    `json:"tags" gorm:"size:500"`
	Author         string    `json:"author" gorm:"size:100;not null;default:'运维工程师'"`
	IsPreset       bool      `json:"is_preset" gorm:"default:false;index"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定 TroubleshootingMemory 的数据库表名
// TableName specifies table name for TroubleshootingMemory
func (TroubleshootingMemory) TableName() string {
	return "diagnostics_troubleshooting_memories"
}

// TroubleshootingMemoryItem 是返回给前端的完整数据视图
// TroubleshootingMemoryItem is the full view payload returned to client
type TroubleshootingMemoryItem struct {
	ID             string    `json:"id"`
	TargetType     string    `json:"target_type"`
	Fingerprint    string    `json:"fingerprint"`
	Title          string    `json:"title"`
	ErrorSummary   string    `json:"error_summary"`
	RootCause      string    `json:"root_cause,omitempty"`
	Solution       string    `json:"solution"`
	ActionsTaken   []string  `json:"actions_taken,omitempty"`
	PreventiveTips string    `json:"preventive_tips,omitempty"`
	ClusterID      uint      `json:"cluster_id,omitempty"`
	ClusterName    string    `json:"cluster_name,omitempty"`
	Tags           []string  `json:"tags"`
	Author         string    `json:"author"`
	IsPreset       bool      `json:"is_preset"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateTroubleshootingMemoryRequest 创建排障经验的请求载荷
// CreateTroubleshootingMemoryRequest is the payload to create troubleshooting memory
type CreateTroubleshootingMemoryRequest struct {
	TargetType     string   `json:"target_type" binding:"required"`
	Fingerprint    string   `json:"fingerprint" binding:"required"`
	Title          string   `json:"title" binding:"required"`
	ErrorSummary   string   `json:"error_summary"`
	RootCause      string   `json:"root_cause"`
	Solution       string   `json:"solution" binding:"required"`
	ActionsTaken   []string `json:"actions_taken"`
	PreventiveTips string   `json:"preventive_tips"`
	ClusterID      *uint    `json:"cluster_id"`
	ClusterName    string   `json:"cluster_name"`
	Tags           []string `json:"tags"`
	Author         string   `json:"author"`
}

// UpdateTroubleshootingMemoryRequest 更新排障经验的请求载荷
// UpdateTroubleshootingMemoryRequest is the payload to update troubleshooting memory
type UpdateTroubleshootingMemoryRequest struct {
	Title          string   `json:"title"`
	ErrorSummary   string   `json:"error_summary"`
	RootCause      string   `json:"root_cause"`
	Solution       string   `json:"solution"`
	ActionsTaken   []string `json:"actions_taken"`
	PreventiveTips string   `json:"preventive_tips"`
	Tags           []string `json:"tags"`
	Author         string   `json:"author"`
}

// TroubleshootingMemoryQuery 查询排障经验的筛选参数
// TroubleshootingMemoryQuery represents filter query for troubleshooting memories
type TroubleshootingMemoryQuery struct {
	TargetType  string
	Fingerprint string
	ClusterID   *uint
	Keyword     string
	Page        int
	PageSize    int
}
