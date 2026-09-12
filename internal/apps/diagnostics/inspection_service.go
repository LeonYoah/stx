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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
)

const (
	inspectionCheckOfflineNode        = "offline_node"
	inspectionCheckRestartFailure     = "restart_failure"
	inspectionCheckRecentErrorBurst   = "recent_error_burst"
	inspectionCheckActiveAlert        = "active_alert"
	defaultInspectionRecentEventLimit = 200
)

// InspectionEvaluationResult contains the current inspection findings before persistence.
// InspectionEvaluationResult 表示巡检发现项在落库前的评估结果。
type InspectionEvaluationResult struct {
	ClusterID     uint                        `json:"cluster_id"`
	GeneratedAt   time.Time                   `json:"generated_at"`
	Summary       string                      `json:"summary"`
	FindingTotal  int                         `json:"finding_total"`
	CriticalCount int                         `json:"critical_count"`
	WarningCount  int                         `json:"warning_count"`
	InfoCount     int                         `json:"info_count"`
	Findings      []*ClusterInspectionFinding `json:"findings"`
}

// EvaluateInspectionFindings evaluates first-batch inspection checks using
// managed runtime, process events, alerts, and recent error groups.
// EvaluateInspectionFindings 基于受管运行时、进程事件、告警与近期错误组评估首批巡检发现项。
func (s *Service) EvaluateInspectionFindings(ctx context.Context, clusterID uint, lookbackWindow time.Duration, errorThreshold int) (*InspectionEvaluationResult, error) {
	if s == nil || s.clusterService == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if lookbackWindow <= 0 {
		lookbackWindow = time.Duration(defaultInspectionLookbackMinutes) * time.Minute
	}
	if errorThreshold <= 0 {
		errorThreshold = defaultInspectionErrorThreshold
	}

	statusInfo, err := s.clusterService.GetStatus(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	findings := make([]*ClusterInspectionFinding, 0, 8)
	findings = append(findings, s.evaluateOfflineNodeFindings(statusInfo)...)

	if s.monitorService != nil {
		eventFindings, err := s.evaluateProcessEventFindings(ctx, clusterID, lookbackWindow)
		if err != nil {
			return nil, err
		}
		findings = append(findings, eventFindings...)
	}

	if s.repo != nil {
		errorFindings, err := s.evaluateRecentErrorFindings(ctx, clusterID, lookbackWindow, errorThreshold)
		if err != nil {
			return nil, err
		}
		findings = append(findings, errorFindings...)
	}

	if s.monitoringService != nil {
		alertFindings, err := s.evaluateActiveAlertFindings(ctx, clusterID)
		if err != nil {
			return nil, err
		}
		findings = append(findings, alertFindings...)
	}

	sort.SliceStable(findings, func(i, j int) bool {
		left := inspectionSeverityRank(findings[i].Severity)
		right := inspectionSeverityRank(findings[j].Severity)
		if left == right {
			return findings[i].ID < findings[j].ID
		}
		return left > right
	})

	result := &InspectionEvaluationResult{
		ClusterID:   clusterID,
		GeneratedAt: time.Now().UTC(),
		Findings:    findings,
	}
	for _, finding := range findings {
		switch finding.Severity {
		case InspectionFindingSeverityCritical:
			result.CriticalCount++
		case InspectionFindingSeverityWarning:
			result.WarningCount++
		default:
			result.InfoCount++
		}
	}
	result.FindingTotal = len(findings)
	result.Summary = buildInspectionSummary(statusInfo.ClusterName, result, lookbackWindow)
	return result, nil
}

func (s *Service) evaluateOfflineNodeFindings(statusInfo *cluster.ClusterStatusInfo) []*ClusterInspectionFinding {
	if statusInfo == nil {
		return nil
	}

	findings := make([]*ClusterInspectionFinding, 0)
	for _, node := range statusInfo.Nodes {
		if node == nil || node.IsOnline {
			continue
		}
		findings = append(findings, &ClusterInspectionFinding{
			ClusterID:       statusInfo.ClusterID,
			Severity:        InspectionFindingSeverityCritical,
			Category:        "node_health",
			CheckCode:       inspectionCheckOfflineNode,
			CheckName:       bilingualText("离线节点", "Offline Node"),
			Summary:         bilingualText(fmt.Sprintf("节点 %s（%s）离线", resolveInspectionNodeName(node), strings.TrimSpace(string(node.Role))), fmt.Sprintf("Node %s (%s) is offline", resolveInspectionNodeName(node), strings.TrimSpace(string(node.Role)))),
			Recommendation:  bilingualText("在重试操作前，先检查主机心跳、Agent 状态与 SeaTunnel 进程状态。", "Check host heartbeat, agent status, and SeaTunnel process state before retrying operations."),
			EvidenceSummary: fmt.Sprintf("host=%s, ip=%s, status=%s, pid=%d", strings.TrimSpace(node.HostName), strings.TrimSpace(node.HostIP), strings.TrimSpace(string(node.Status)), node.ProcessPID),
			RelatedNodeID:   node.NodeID,
			RelatedHostID:   node.HostID,
		})
	}
	return findings
}

func (s *Service) evaluateProcessEventFindings(ctx context.Context, clusterID uint, lookbackWindow time.Duration) ([]*ClusterInspectionFinding, error) {
	events, err := s.monitorService.ListClusterEvents(ctx, clusterID, defaultInspectionRecentEventLimit)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().UTC().Add(-lookbackWindow)
	findings := make([]*ClusterInspectionFinding, 0)
	seen := make(map[string]struct{})
	for _, event := range events {
		if event == nil || event.CreatedAt.Before(cutoff) {
			continue
		}
		switch event.EventType {
		case monitor.EventTypeRestartFailed, monitor.EventTypeRestartLimitReached:
			key := fmt.Sprintf("%s:%d:%s", event.EventType, event.NodeID, strings.TrimSpace(event.ProcessName))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			findings = append(findings, &ClusterInspectionFinding{
				ClusterID:       clusterID,
				Severity:        InspectionFindingSeverityCritical,
				Category:        "process_event",
				CheckCode:       inspectionCheckRestartFailure,
				CheckName:       bilingualText("重启失败", "Restart Failure"),
				Summary:         bilingualText(fmt.Sprintf("近期检测到进程 %s 出现 %s", strings.TrimSpace(event.ProcessName), strings.TrimSpace(string(event.EventType))), fmt.Sprintf("Recent %s detected for process %s", strings.TrimSpace(string(event.EventType)), strings.TrimSpace(event.ProcessName))),
				Recommendation:  bilingualText("查看节点事件历史与运行日志，并核对启动命令和依赖是否正常。", "Review node event history and runtime logs, then verify startup command and dependencies."),
				EvidenceSummary: truncateString(strings.TrimSpace(event.Details), 1024),
				RelatedNodeID:   event.NodeID,
				RelatedHostID:   event.HostID,
			})
		}
	}
	return findings, nil
}

func (s *Service) evaluateRecentErrorFindings(ctx context.Context, clusterID uint, lookbackWindow time.Duration, errorThreshold int) ([]*ClusterInspectionFinding, error) {
	since := time.Now().UTC().Add(-lookbackWindow)
	bursts, err := s.repo.ListRecentErrorGroupBursts(ctx, clusterID, since, 20)
	if err != nil {
		return nil, err
	}
	if errorThreshold <= 0 {
		errorThreshold = defaultInspectionErrorThreshold
	}

	findings := make([]*ClusterInspectionFinding, 0)
	for _, burst := range bursts {
		if burst == nil || burst.Group == nil || burst.RecentCount < int64(errorThreshold) {
			continue
		}
		group := burst.Group
		severity := InspectionFindingSeverityWarning
		if burst.RecentCount >= 10 {
			severity = InspectionFindingSeverityCritical
		}
		findings = append(findings, &ClusterInspectionFinding{
			ClusterID:           clusterID,
			Severity:            severity,
			Category:            "error_group",
			CheckCode:           inspectionCheckRecentErrorBurst,
			CheckName:           bilingualText("近期错误突增", "Recent Error Burst"),
			Summary:             bilingualText(fmt.Sprintf("近期错误组“%s”在最近 %d 分钟内出现 %d 次", group.Title, int(lookbackWindow/time.Minute), burst.RecentCount), fmt.Sprintf("Recent error group \"%s\" occurred %d times in the last %d minutes", group.Title, burst.RecentCount, int(lookbackWindow/time.Minute))),
			Recommendation:      bilingualText("打开错误中心查看分组证据，定位底层依赖或运行时问题。", "Open the error center to inspect grouped evidence and identify the underlying dependency or runtime issue."),
			EvidenceSummary:     truncateString(strings.TrimSpace(group.SampleMessage), 1024),
			RelatedErrorGroupID: group.ID,
			RelatedNodeID:       group.LastNodeID,
			RelatedHostID:       group.LastHostID,
		})
	}
	return findings, nil
}

func (s *Service) evaluateActiveAlertFindings(ctx context.Context, clusterID uint) ([]*ClusterInspectionFinding, error) {
	alerts, err := s.monitoringService.ListAlertInstances(ctx, &monitoringapp.AlertInstanceFilter{
		ClusterID: strconv.FormatUint(uint64(clusterID), 10),
		Status:    monitoringapp.AlertDisplayStatusFiring,
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		return nil, err
	}
	if alerts == nil || len(alerts.Alerts) == 0 {
		return nil, nil
	}

	findings := make([]*ClusterInspectionFinding, 0, len(alerts.Alerts))
	for _, alert := range alerts.Alerts {
		if alert == nil {
			continue
		}
		severity := InspectionFindingSeverityWarning
		if alert.Severity == monitoringapp.AlertSeverityCritical {
			severity = InspectionFindingSeverityCritical
		}
		finding := &ClusterInspectionFinding{
			ClusterID:       clusterID,
			Severity:        severity,
			Category:        "alert",
			CheckCode:       inspectionCheckActiveAlert,
			CheckName:       bilingualText("活动告警", "Active Alert"),
			Summary:         bilingualText(fmt.Sprintf("活动 %s 告警：%s", strings.TrimSpace(string(alert.Severity)), strings.TrimSpace(alert.AlertName)), fmt.Sprintf("Active %s alert: %s", strings.TrimSpace(string(alert.Severity)), strings.TrimSpace(alert.AlertName))),
			Recommendation:  bilingualText("在执行重启或扩缩容前，先查看告警详情与关联运行时证据。", "Review the alert detail and linked runtime evidence before performing restart or scale actions."),
			EvidenceSummary: truncateString(firstNonEmptyString(alert.Summary, alert.Description), 1024),
			RelatedAlertID:  strings.TrimSpace(alert.AlertID),
		}
		if alert.SourceRef != nil {
			finding.EvidenceSummary = truncateString(firstNonEmptyString(finding.EvidenceSummary, alert.SourceRef.EventType, alert.SourceRef.ProcessName), 1024)
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func buildInspectionSummary(clusterName string, result *InspectionEvaluationResult, lookbackWindow time.Duration) string {
	if result == nil {
		return ""
	}
	name := strings.TrimSpace(clusterName)
	if name == "" {
		name = fmt.Sprintf("cluster-%d", result.ClusterID)
	}
	lookbackMinutes := int(lookbackWindow / time.Minute)
	if lookbackMinutes <= 0 {
		lookbackMinutes = defaultInspectionLookbackMinutes
	}
	return bilingualText(
		fmt.Sprintf("%s 在最近 %d 分钟巡检中生成 %d 条发现（严重 %d / 告警 %d / 信息 %d）",
			name,
			lookbackMinutes,
			result.FindingTotal,
			result.CriticalCount,
			result.WarningCount,
			result.InfoCount,
		),
		fmt.Sprintf("%s inspection for the last %d minutes generated %d findings (%d critical / %d warning / %d info)",
			name,
			lookbackMinutes,
			result.FindingTotal,
			result.CriticalCount,
			result.WarningCount,
			result.InfoCount,
		),
	)
}

func inspectionSeverityRank(severity InspectionFindingSeverity) int {
	switch severity {
	case InspectionFindingSeverityCritical:
		return 3
	case InspectionFindingSeverityWarning:
		return 2
	default:
		return 1
	}
}

func resolveInspectionNodeName(node *cluster.NodeStatusInfo) string {
	if node == nil {
		return "-"
	}
	if name := strings.TrimSpace(node.HostName); name != "" {
		return name
	}
	if ip := strings.TrimSpace(node.HostIP); ip != "" {
		return ip
	}
	return fmt.Sprintf("node-%d", node.NodeID)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
