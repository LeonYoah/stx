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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

const maxInspectionErrorMessageLength = 2048

// StartInspection creates one persisted inspection report, evaluates current signals,
// and stores the resulting findings.
// StartInspection 创建一条巡检报告、评估当前信号并保存巡检发现项。
func (s *Service) StartInspection(ctx context.Context, req *StartClusterInspectionRequest, requestedByUserID uint, requestedBy string) (*ClusterInspectionReportDetailData, error) {
	if s == nil || s.repo == nil || s.clusterService == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if req == nil || req.ClusterID == 0 {
		return nil, fmt.Errorf("%w: cluster_id is required", ErrInvalidInspectionRequest)
	}

	triggerSource, ok := normalizeInspectionTriggerSource(req.TriggerSource)
	if !ok {
		return nil, fmt.Errorf("%w: invalid trigger_source", ErrInvalidInspectionRequest)
	}
	lookbackMinutes, ok := normalizeInspectionLookbackMinutes(req.LookbackMinutes)
	if !ok {
		return nil, fmt.Errorf("%w: lookback_minutes must be between %d and %d", ErrInvalidInspectionRequest, minInspectionLookbackMinutes, maxInspectionLookbackMinutes)
	}
	errorThreshold, ok := normalizeInspectionErrorThreshold(req.ErrorThreshold)
	if !ok {
		return nil, fmt.Errorf("%w: error_threshold must be between %d and %d", ErrInvalidInspectionRequest, minInspectionErrorThreshold, maxInspectionErrorThreshold)
	}
	if _, err := s.clusterService.Get(ctx, req.ClusterID); err != nil {
		return nil, err
	}

	startedAt := timex.Now()
	report := &ClusterInspectionReport{
		ClusterID:         req.ClusterID,
		Status:            InspectionReportStatusRunning,
		TriggerSource:     triggerSource,
		LookbackMinutes:   lookbackMinutes,
		ErrorThreshold:    errorThreshold,
		RequestedByUserID: requestedByUserID,
		RequestedBy:       truncateString(strings.TrimSpace(requestedBy), 120),
		StartedAt:         &startedAt,
	}
	if err := s.repo.CreateInspectionReport(ctx, report); err != nil {
		return nil, err
	}

	result, err := s.EvaluateInspectionFindings(ctx, req.ClusterID, time.Duration(lookbackMinutes)*time.Minute, errorThreshold)
	if err != nil {
		return s.failInspectionReport(ctx, report, err)
	}

	findings := make([]*ClusterInspectionFinding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		if finding == nil {
			continue
		}
		findings = append(findings, &ClusterInspectionFinding{
			ReportID:            report.ID,
			ClusterID:           req.ClusterID,
			Severity:            finding.Severity,
			Category:            finding.Category,
			CheckCode:           finding.CheckCode,
			CheckName:           finding.CheckName,
			Summary:             finding.Summary,
			Recommendation:      finding.Recommendation,
			EvidenceSummary:     finding.EvidenceSummary,
			RelatedNodeID:       finding.RelatedNodeID,
			RelatedHostID:       finding.RelatedHostID,
			RelatedErrorGroupID: finding.RelatedErrorGroupID,
			RelatedAlertID:      finding.RelatedAlertID,
		})
	}

	finishedAt := timex.Now()
	report.Status = InspectionReportStatusCompleted
	report.Summary = result.Summary
	report.ErrorMessage = ""
	report.FindingTotal = result.FindingTotal
	report.CriticalCount = result.CriticalCount
	report.WarningCount = result.WarningCount
	report.InfoCount = result.InfoCount
	report.FinishedAt = &finishedAt

	if err := s.repo.Transaction(ctx, func(tx *Repository) error {
		if err := tx.CreateInspectionFindings(ctx, findings); err != nil {
			return err
		}
		return tx.UpdateInspectionReport(ctx, report)
	}); err != nil {
		return nil, err
	}

	return s.GetInspectionReportDetail(ctx, report.ID)
}

// ListInspectionReports returns paginated inspection reports for diagnostics workspace.
// ListInspectionReports 返回诊断中心的分页巡检报告列表。
func (s *Service) ListInspectionReports(ctx context.Context, filter *ClusterInspectionReportFilter) (*ClusterInspectionReportsData, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if filter == nil {
		filter = &ClusterInspectionReportFilter{}
	}

	status, ok := normalizeInspectionReportStatus(filter.Status)
	if !ok {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInspectionRequest)
	}
	filter.Status = status

	triggerSource, ok := normalizeInspectionTriggerSource(filter.TriggerSource)
	if !ok {
		return nil, fmt.Errorf("%w: invalid trigger_source", ErrInvalidInspectionRequest)
	}
	if strings.TrimSpace(string(filter.TriggerSource)) != "" {
		filter.TriggerSource = triggerSource
	} else {
		filter.TriggerSource = ""
	}

	severity, ok := normalizeInspectionFindingSeverity(filter.Severity)
	if !ok {
		return nil, fmt.Errorf("%w: invalid severity", ErrInvalidInspectionRequest)
	}
	filter.Severity = severity

	reports, total, err := s.repo.ListInspectionReports(ctx, filter)
	if err != nil {
		return nil, err
	}

	page, pageSize := normalizePagination(filter.Page, filter.PageSize)
	items := make([]*ClusterInspectionReportInfo, 0, len(reports))
	for _, report := range reports {
		info := report.ToInfo()
		if info != nil && s.clusterService != nil {
			if clusterInfo, err := s.clusterService.Get(ctx, report.ClusterID); err == nil && clusterInfo != nil {
				info.ClusterName = clusterInfo.Name
			}
		}
		items = append(items, info)
	}
	return &ClusterInspectionReportsData{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetInspectionReportDetail returns one inspection report and all findings.
// GetInspectionReportDetail 返回单个巡检报告及其全部发现项。
func (s *Service) GetInspectionReportDetail(ctx context.Context, reportID uint) (*ClusterInspectionReportDetailData, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if reportID == 0 {
		return nil, fmt.Errorf("%w: report_id is required", ErrInvalidInspectionRequest)
	}

	report, err := s.repo.GetInspectionReportByID(ctx, reportID)
	if err != nil {
		return nil, err
	}
	findings, err := s.repo.ListInspectionFindingsByReportID(ctx, reportID)
	if err != nil {
		return nil, err
	}

	items := make([]*ClusterInspectionFindingInfo, 0, len(findings))
	for _, finding := range findings {
		items = append(items, finding.ToInfo(s.resolveDiagnosticHostDisplayContext(ctx, finding.RelatedHostID)))
	}

	var relatedTask *DiagnosticTask
	if linked, linkErr := s.repo.GetLatestDiagnosticTaskByInspectionReportID(ctx, reportID); linkErr == nil && linked != nil {
		relatedTask = linked
	}

	reportInfo := report.ToInfo()
	if reportInfo != nil && s.clusterService != nil {
		if clusterInfo, err := s.clusterService.Get(ctx, report.ClusterID); err == nil && clusterInfo != nil {
			reportInfo.ClusterName = clusterInfo.Name
		}
	}

	return &ClusterInspectionReportDetailData{
		Report:                reportInfo,
		Findings:              items,
		RelatedDiagnosticTask: relatedTask,
	}, nil
}

func (s *Service) failInspectionReport(ctx context.Context, report *ClusterInspectionReport, cause error) (*ClusterInspectionReportDetailData, error) {
	if report == nil {
		return nil, cause
	}
	finishedAt := timex.Now()
	report.Status = InspectionReportStatusFailed
	report.ErrorMessage = truncateString(strings.TrimSpace(cause.Error()), maxInspectionErrorMessageLength)
	report.Summary = bilingualText(
		fmt.Sprintf("集群 %d 巡检失败", report.ClusterID),
		fmt.Sprintf("Cluster %d inspection failed", report.ClusterID),
	)
	report.FinishedAt = &finishedAt
	report.FindingTotal = 0
	report.CriticalCount = 0
	report.WarningCount = 0
	report.InfoCount = 0
	if err := s.repo.UpdateInspectionReport(ctx, report); err != nil {
		return nil, err
	}
	return s.GetInspectionReportDetail(ctx, report.ID)
}
