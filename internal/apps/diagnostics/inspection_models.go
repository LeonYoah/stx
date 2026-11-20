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
	"strings"
	"time"
)

const (
	defaultInspectionLookbackMinutes = 30
	minInspectionLookbackMinutes     = 5
	maxInspectionLookbackMinutes     = 24 * 60
	defaultInspectionErrorThreshold  = 1
	minInspectionErrorThreshold      = 1
	maxInspectionErrorThreshold      = 1000
)

// InspectionReportStatus represents the execution status of one inspection report.
// InspectionReportStatus 表示一条巡检报告的执行状态。
type InspectionReportStatus string

const (
	// InspectionReportStatusPending means the inspection has been created but not started.
	// InspectionReportStatusPending 表示巡检已创建但尚未开始。
	InspectionReportStatusPending InspectionReportStatus = "pending"
	// InspectionReportStatusRunning means the inspection is currently evaluating signals.
	// InspectionReportStatusRunning 表示巡检正在执行信号评估。
	InspectionReportStatusRunning InspectionReportStatus = "running"
	// InspectionReportStatusCompleted means the inspection finished successfully.
	// InspectionReportStatusCompleted 表示巡检已成功完成。
	InspectionReportStatusCompleted InspectionReportStatus = "completed"
	// InspectionReportStatusFailed means the inspection execution failed.
	// InspectionReportStatusFailed 表示巡检执行失败。
	InspectionReportStatusFailed InspectionReportStatus = "failed"
)

// InspectionTriggerSource represents where an inspection was initiated from.
// InspectionTriggerSource 表示巡检的触发来源。
type InspectionTriggerSource string

const (
	// InspectionTriggerSourceManual indicates a user manually created the inspection.
	// InspectionTriggerSourceManual 表示用户手动发起巡检。
	InspectionTriggerSourceManual InspectionTriggerSource = "manual"
	// InspectionTriggerSourceClusterDetail indicates the inspection was started from cluster detail page.
	// InspectionTriggerSourceClusterDetail 表示巡检来自集群详情页。
	InspectionTriggerSourceClusterDetail InspectionTriggerSource = "cluster_detail"
	// InspectionTriggerSourceDiagnosticsWorkspace indicates the inspection was started from diagnostics workspace.
	// InspectionTriggerSourceDiagnosticsWorkspace 表示巡检来自诊断中心工作台。
	InspectionTriggerSourceDiagnosticsWorkspace InspectionTriggerSource = "diagnostics_workspace"
	// InspectionTriggerSourceAuto indicates the inspection was triggered automatically by an auto-policy.
	// InspectionTriggerSourceAuto 表示巡检由自动策略自动触发。
	InspectionTriggerSourceAuto InspectionTriggerSource = "auto"
)

// InspectionFindingSeverity represents finding severity in an inspection report.
// InspectionFindingSeverity 表示巡检发现项的严重级别。
type InspectionFindingSeverity string

const (
	// InspectionFindingSeverityInfo indicates informational findings.
	// InspectionFindingSeverityInfo 表示信息级发现项。
	InspectionFindingSeverityInfo InspectionFindingSeverity = "info"
	// InspectionFindingSeverityWarning indicates warning findings.
	// InspectionFindingSeverityWarning 表示告警级发现项。
	InspectionFindingSeverityWarning InspectionFindingSeverity = "warning"
	// InspectionFindingSeverityCritical indicates critical findings.
	// InspectionFindingSeverityCritical 表示严重发现项。
	InspectionFindingSeverityCritical InspectionFindingSeverity = "critical"
)

// ClusterInspectionReport stores one persisted inspection execution.
// ClusterInspectionReport 存储一条持久化的集群巡检执行记录。
type ClusterInspectionReport struct {
	ID                uint                    `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID         uint                    `json:"cluster_id" gorm:"index;not null"`
	Status            InspectionReportStatus  `json:"status" gorm:"size:20;index;not null"`
	TriggerSource     InspectionTriggerSource `json:"trigger_source" gorm:"size:32;index;not null"`
	LookbackMinutes   int                     `json:"lookback_minutes" gorm:"default:30"`
	ErrorThreshold    int                     `json:"error_threshold" gorm:"default:1"`
	RequestedByUserID uint                    `json:"requested_by_user_id" gorm:"index"`
	RequestedBy       string                  `json:"requested_by" gorm:"size:120;index"`
	Summary           string                  `json:"summary" gorm:"type:text"`
	ErrorMessage      string                  `json:"error_message" gorm:"type:text"`
	FindingTotal      int                     `json:"finding_total" gorm:"default:0"`
	CriticalCount     int                     `json:"critical_count" gorm:"default:0"`
	WarningCount      int                     `json:"warning_count" gorm:"default:0"`
	InfoCount         int                     `json:"info_count" gorm:"default:0"`
	AutoTriggerReason string                  `json:"auto_trigger_reason" gorm:"size:200"`
	// AutoPolicyID 记录触发本报告的自动巡检策略 ID；手动巡检为 0。存量数据由迁移回填。
	// AutoPolicyID identifies the auto-inspection policy that triggered this report;
	// manual inspections keep 0. Historical rows are backfilled by the migrator.
	AutoPolicyID uint       `json:"auto_policy_id" gorm:"index;not null;default:0"`
	StartedAt    *time.Time `json:"started_at" gorm:"index"`
	FinishedAt   *time.Time `json:"finished_at" gorm:"index"`
	CreatedAt    time.Time  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt    time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for ClusterInspectionReport.
// TableName 指定 ClusterInspectionReport 的表名。
func (ClusterInspectionReport) TableName() string {
	return "diagnostics_inspection_reports"
}

// ClusterInspectionFinding stores one structured finding within an inspection report.
// ClusterInspectionFinding 存储巡检报告中的一条结构化发现项。
type ClusterInspectionFinding struct {
	ID                  uint                      `json:"id" gorm:"primaryKey;autoIncrement"`
	ReportID            uint                      `json:"report_id" gorm:"index;not null"`
	ClusterID           uint                      `json:"cluster_id" gorm:"index;not null"`
	Severity            InspectionFindingSeverity `json:"severity" gorm:"size:20;index;not null"`
	Category            string                    `json:"category" gorm:"size:64;index;not null"`
	CheckCode           string                    `json:"check_code" gorm:"size:100;index;not null"`
	CheckName           string                    `json:"check_name" gorm:"size:200"`
	Summary             string                    `json:"summary" gorm:"type:text"`
	Recommendation      string                    `json:"recommendation" gorm:"type:text"`
	EvidenceSummary     string                    `json:"evidence_summary" gorm:"type:text"`
	RelatedNodeID       uint                      `json:"related_node_id" gorm:"index"`
	RelatedHostID       uint                      `json:"related_host_id" gorm:"index"`
	RelatedErrorGroupID uint                      `json:"related_error_group_id" gorm:"index"`
	RelatedAlertID      string                    `json:"related_alert_id" gorm:"size:120;index"`
	CreatedAt           time.Time                 `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt           time.Time                 `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for ClusterInspectionFinding.
// TableName 指定 ClusterInspectionFinding 的表名。
func (ClusterInspectionFinding) TableName() string {
	return "diagnostics_inspection_findings"
}

// ClusterInspectionReportFilter defines list filters for inspection reports.
// ClusterInspectionReportFilter 定义巡检报告列表过滤条件。
type ClusterInspectionReportFilter struct {
	ClusterID     uint                    `json:"cluster_id"`
	Status        InspectionReportStatus  `json:"status"`
	TriggerSource InspectionTriggerSource `json:"trigger_source"`
	// AutoPolicyID 非零时只列出该自动巡检策略触发的报告。
	// AutoPolicyID limits the list to reports triggered by that policy when non-zero.
	AutoPolicyID uint                      `json:"auto_policy_id"`
	Severity     InspectionFindingSeverity `json:"severity"`
	// Keyword 模糊匹配报告摘要、错误信息、触发原因与自动策略名；纯数字额外精确命中报告 ID。
	// Keyword fuzzy-matches report summary, error message, trigger reason, and
	// auto-policy name; pure digits additionally match the report ID exactly.
	Keyword   string     `json:"keyword"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	Page      int        `json:"page"`
	PageSize  int        `json:"page_size"`
}

// ClusterInspectionFindingFilter defines list filters for inspection findings.
// ClusterInspectionFindingFilter 定义巡检发现项列表过滤条件。
type ClusterInspectionFindingFilter struct {
	ReportID            uint                      `json:"report_id"`
	ClusterID           uint                      `json:"cluster_id"`
	Severity            InspectionFindingSeverity `json:"severity"`
	Category            string                    `json:"category"`
	RelatedNodeID       uint                      `json:"related_node_id"`
	RelatedHostID       uint                      `json:"related_host_id"`
	RelatedErrorGroupID uint                      `json:"related_error_group_id"`
	Page                int                       `json:"page"`
	PageSize            int                       `json:"page_size"`
}

// StartClusterInspectionRequest describes one manual inspection trigger request.
// StartClusterInspectionRequest 描述一次手动发起巡检的请求。
type StartClusterInspectionRequest struct {
	ClusterID       uint                    `json:"cluster_id" binding:"required"`
	TriggerSource   InspectionTriggerSource `json:"trigger_source"`
	LookbackMinutes int                     `json:"lookback_minutes,omitempty"`
	ErrorThreshold  int                     `json:"error_threshold,omitempty"`
}

// ClusterInspectionReportInfo is the API view model for one inspection report.
// ClusterInspectionReportInfo 是巡检报告的 API 视图模型。
type ClusterInspectionReportInfo struct {
	ID                uint                    `json:"id"`
	ClusterID         uint                    `json:"cluster_id"`
	ClusterName       string                  `json:"cluster_name,omitempty"`
	Status            InspectionReportStatus  `json:"status"`
	TriggerSource     InspectionTriggerSource `json:"trigger_source"`
	LookbackMinutes   int                     `json:"lookback_minutes"`
	ErrorThreshold    int                     `json:"error_threshold"`
	RequestedByUserID uint                    `json:"requested_by_user_id"`
	RequestedBy       string                  `json:"requested_by"`
	Summary           string                  `json:"summary"`
	ErrorMessage      string                  `json:"error_message"`
	FindingTotal      int                     `json:"finding_total"`
	CriticalCount     int                     `json:"critical_count"`
	WarningCount      int                     `json:"warning_count"`
	InfoCount         int                     `json:"info_count"`
	AutoTriggerReason string                  `json:"auto_trigger_reason,omitempty"`
	AutoPolicyID      uint                    `json:"auto_policy_id,omitempty"`
	StartedAt         *time.Time              `json:"started_at,omitempty"`
	FinishedAt        *time.Time              `json:"finished_at,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}

// ClusterInspectionFindingInfo is the API view model for one inspection finding.
// ClusterInspectionFindingInfo 是巡检发现项的 API 视图模型。
type ClusterInspectionFindingInfo struct {
	ID                  uint                      `json:"id"`
	ReportID            uint                      `json:"report_id"`
	ClusterID           uint                      `json:"cluster_id"`
	Severity            InspectionFindingSeverity `json:"severity"`
	Category            string                    `json:"category"`
	CheckCode           string                    `json:"check_code"`
	CheckName           string                    `json:"check_name"`
	Summary             string                    `json:"summary"`
	Recommendation      string                    `json:"recommendation"`
	EvidenceSummary     string                    `json:"evidence_summary"`
	RelatedNodeID       uint                      `json:"related_node_id"`
	RelatedHostID       uint                      `json:"related_host_id"`
	RelatedHostName     string                    `json:"related_host_name"`
	RelatedHostIP       string                    `json:"related_host_ip"`
	RelatedErrorGroupID uint                      `json:"related_error_group_id"`
	RelatedAlertID      string                    `json:"related_alert_id"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

// ClusterInspectionReportsData is the paginated inspection report payload.
// ClusterInspectionReportsData 是分页巡检报告载荷。
type ClusterInspectionReportsData struct {
	Items    []*ClusterInspectionReportInfo `json:"items"`
	Total    int64                          `json:"total"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
}

// ClusterInspectionReportDetailData is the inspection report detail payload.
// ClusterInspectionReportDetailData 是巡检报告详情载荷。
type ClusterInspectionReportDetailData struct {
	Report                *ClusterInspectionReportInfo    `json:"report"`
	Findings              []*ClusterInspectionFindingInfo `json:"findings"`
	RelatedDiagnosticTask *DiagnosticTask                 `json:"related_diagnostic_task,omitempty"`
}

// ToInfo converts a persisted inspection report into API view model.
// ToInfo 将巡检报告转换为 API 视图模型。
func (r *ClusterInspectionReport) ToInfo() *ClusterInspectionReportInfo {
	if r == nil {
		return nil
	}
	return &ClusterInspectionReportInfo{
		ID:                r.ID,
		ClusterID:         r.ClusterID,
		Status:            r.Status,
		TriggerSource:     r.TriggerSource,
		LookbackMinutes:   firstNonZeroInt(r.LookbackMinutes, defaultInspectionLookbackMinutes),
		ErrorThreshold:    firstNonZeroInt(r.ErrorThreshold, defaultInspectionErrorThreshold),
		RequestedByUserID: r.RequestedByUserID,
		RequestedBy:       r.RequestedBy,
		Summary:           r.Summary,
		ErrorMessage:      r.ErrorMessage,
		FindingTotal:      r.FindingTotal,
		CriticalCount:     r.CriticalCount,
		WarningCount:      r.WarningCount,
		InfoCount:         r.InfoCount,
		AutoTriggerReason: r.AutoTriggerReason,
		AutoPolicyID:      r.AutoPolicyID,
		StartedAt:         r.StartedAt,
		FinishedAt:        r.FinishedAt,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}
}

// ToInfo converts one persisted inspection finding into API view model.
// ToInfo 将巡检发现项转换为 API 视图模型。
func (f *ClusterInspectionFinding) ToInfo(display *DiagnosticHostDisplayContext) *ClusterInspectionFindingInfo {
	if f == nil {
		return nil
	}
	info := &ClusterInspectionFindingInfo{
		ID:                  f.ID,
		ReportID:            f.ReportID,
		ClusterID:           f.ClusterID,
		Severity:            f.Severity,
		Category:            f.Category,
		CheckCode:           f.CheckCode,
		CheckName:           f.CheckName,
		Summary:             f.Summary,
		Recommendation:      f.Recommendation,
		EvidenceSummary:     f.EvidenceSummary,
		RelatedNodeID:       f.RelatedNodeID,
		RelatedHostID:       f.RelatedHostID,
		RelatedErrorGroupID: f.RelatedErrorGroupID,
		RelatedAlertID:      f.RelatedAlertID,
		CreatedAt:           f.CreatedAt,
		UpdatedAt:           f.UpdatedAt,
	}
	if display != nil {
		info.RelatedHostName = display.HostName
		info.RelatedHostIP = display.HostIP
	}
	return info
}

func normalizeInspectionTriggerSource(value InspectionTriggerSource) (InspectionTriggerSource, bool) {
	switch InspectionTriggerSource(strings.TrimSpace(string(value))) {
	case "":
		return InspectionTriggerSourceManual, true
	case InspectionTriggerSourceManual:
		return InspectionTriggerSourceManual, true
	case InspectionTriggerSourceClusterDetail:
		return InspectionTriggerSourceClusterDetail, true
	case InspectionTriggerSourceDiagnosticsWorkspace:
		return InspectionTriggerSourceDiagnosticsWorkspace, true
	case InspectionTriggerSourceAuto:
		return InspectionTriggerSourceAuto, true
	default:
		return "", false
	}
}

func normalizeInspectionReportStatus(value InspectionReportStatus) (InspectionReportStatus, bool) {
	switch InspectionReportStatus(strings.TrimSpace(string(value))) {
	case "":
		return "", true
	case InspectionReportStatusPending:
		return InspectionReportStatusPending, true
	case InspectionReportStatusRunning:
		return InspectionReportStatusRunning, true
	case InspectionReportStatusCompleted:
		return InspectionReportStatusCompleted, true
	case InspectionReportStatusFailed:
		return InspectionReportStatusFailed, true
	default:
		return "", false
	}
}

func normalizeInspectionFindingSeverity(value InspectionFindingSeverity) (InspectionFindingSeverity, bool) {
	switch InspectionFindingSeverity(strings.TrimSpace(string(value))) {
	case "":
		return "", true
	case InspectionFindingSeverityInfo:
		return InspectionFindingSeverityInfo, true
	case InspectionFindingSeverityWarning:
		return InspectionFindingSeverityWarning, true
	case InspectionFindingSeverityCritical:
		return InspectionFindingSeverityCritical, true
	default:
		return "", false
	}
}

func normalizeInspectionLookbackMinutes(value int) (int, bool) {
	switch {
	case value == 0:
		return defaultInspectionLookbackMinutes, true
	case value < minInspectionLookbackMinutes || value > maxInspectionLookbackMinutes:
		return 0, false
	default:
		return value, true
	}
}

func normalizeInspectionErrorThreshold(value int) (int, bool) {
	switch {
	case value == 0:
		return defaultInspectionErrorThreshold, true
	case value < minInspectionErrorThreshold || value > maxInspectionErrorThreshold:
		return 0, false
	default:
		return value, true
	}
}
