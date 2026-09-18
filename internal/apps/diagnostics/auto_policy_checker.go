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
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/pkg/schedulex"
)

const autoPolicyRequestedByPrefix = "auto-policy:"

// AutoPolicyChecker evaluates auto-inspection policies against incoming signals.
// AutoPolicyChecker 根据传入信号评估自动巡检策略。
type AutoPolicyChecker struct {
	repo    *Repository
	service *Service
}

// NewAutoPolicyChecker creates an auto-policy checker.
// NewAutoPolicyChecker 创建自动策略检查器。
func NewAutoPolicyChecker(repo *Repository, service *Service) *AutoPolicyChecker {
	return &AutoPolicyChecker{
		repo:    repo,
		service: service,
	}
}

// CheckJavaErrorTrigger checks whether a Java error event should trigger an auto-inspection.
// CheckJavaErrorTrigger 检查一条 Java 错误事件是否应触发自动巡检。
func (c *AutoPolicyChecker) CheckJavaErrorTrigger(ctx context.Context, clusterID uint, exceptionClass string, messageSnippet string) error {
	if c == nil || c.repo == nil || c.service == nil || clusterID == 0 {
		return nil
	}

	policies, err := c.repo.ListEnabledPoliciesForCluster(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("auto-policy: failed to list policies for cluster %d: %w", clusterID, err)
	}
	if len(policies) == 0 {
		return nil
	}

	exceptionClassLower := strings.ToLower(strings.TrimSpace(exceptionClass))
	messageSnippetLower := strings.ToLower(strings.TrimSpace(messageSnippet))

	for _, policy := range policies {
		if policy == nil || !policy.Enabled {
			continue
		}
		for _, condition := range policy.Conditions {
			if !condition.Enabled {
				continue
			}
			matched, templateCode := c.matchJavaErrorCondition(condition, exceptionClassLower, messageSnippetLower)
			if !matched {
				continue
			}

			triggerReason := fmt.Sprintf("%s|%s: %s", c.policyReasonPrefix(policy.ID), templateCode, strings.TrimSpace(exceptionClass))
			triggered, err := c.triggerAutoPolicyInspection(ctx, clusterID, policy, triggerReason)
			if err != nil {
				return err
			}
			if triggered {
				return nil
			}
		}
	}
	return nil
}

// CheckScheduledPolicies checks all enabled schedule-based policies for one cluster.
// CheckScheduledPolicies 检查单个集群的所有定时巡检策略。
func (c *AutoPolicyChecker) CheckScheduledPolicies(ctx context.Context, clusterID uint, now time.Time) error {
	if c == nil || c.repo == nil || c.service == nil || clusterID == 0 {
		return nil
	}

	policies, err := c.repo.ListEnabledPoliciesForCluster(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("auto-policy: failed to list scheduled policies for cluster %d: %w", clusterID, err)
	}

	now = now.UTC().Truncate(time.Minute)
	for _, policy := range policies {
		if policy == nil || !policy.Enabled {
			continue
		}
		for _, condition := range policy.Conditions {
			if !condition.Enabled || condition.TemplateCode != ConditionCodeScheduled {
				continue
			}
			matched, reason, err := c.matchScheduledCondition(policy, condition, now)
			if err != nil {
				log.Printf("[DiagnosticsAutoPolicy] schedule condition invalid: cluster_id=%d policy_id=%d err=%v", clusterID, policy.ID, err)
				continue
			}
			if !matched {
				continue
			}
			if _, err := c.triggerAutoPolicyInspection(ctx, clusterID, policy, reason); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

// CheckPrometheusPolicies evaluates metric-signal policies through unified alert instances.
// CheckPrometheusPolicies 通过统一告警实例评估指标信号类策略。
func (c *AutoPolicyChecker) CheckPrometheusPolicies(ctx context.Context, clusterID uint, now time.Time) error {
	if c == nil || c.repo == nil || c.service == nil || clusterID == 0 {
		return nil
	}
	if c.service.monitoringService == nil {
		return nil
	}

	policies, err := c.repo.ListEnabledPoliciesForCluster(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("auto-policy: failed to list metric-signal policies for cluster %d: %w", clusterID, err)
	}
	alerts, err := c.service.monitoringService.ListAlertInstances(ctx, &monitoringapp.AlertInstanceFilter{
		ClusterID: fmt.Sprintf("%d", clusterID),
		Page:      1,
		PageSize:  200,
		Status:    monitoringapp.AlertDisplayStatusFiring,
	})
	if err != nil {
		return fmt.Errorf("auto-policy: failed to list alert instances for cluster %d: %w", clusterID, err)
	}
	if alerts == nil || len(alerts.Alerts) == 0 {
		return nil
	}

	now = now.UTC()
	for _, policy := range policies {
		if policy == nil || !policy.Enabled {
			continue
		}
		for _, condition := range policy.Conditions {
			if !condition.Enabled {
				continue
			}
			var reason string
			switch condition.TemplateCode {
			case ConditionCodeNodeUnhealthy:
				distinctNodes, ok := matchNodeUnhealthyCondition(condition, alerts.Alerts, now)
				if !ok {
					continue
				}
				reason = fmt.Sprintf(
					"%s|%s nodes=%d window=%dm",
					c.policyReasonPrefix(policy.ID),
					condition.TemplateCode,
					distinctNodes,
					resolveConditionWindowMinutes(condition),
				)
			case ConditionCodeAlertFiring:
				matchedAlert, ok := matchAlertFiringCondition(condition, alerts.Alerts)
				if !ok {
					continue
				}
				reason = fmt.Sprintf(
					"%s|%s alert=%s",
					c.policyReasonPrefix(policy.ID),
					condition.TemplateCode,
					normalizeMetricAlertReasonKey(matchedAlert),
				)
			default:
				matchedAlert, ok := matchMetricAlertCondition(condition, alerts.Alerts)
				if !ok {
					continue
				}
				reason = fmt.Sprintf(
					"%s|%s alert=%s",
					c.policyReasonPrefix(policy.ID),
					condition.TemplateCode,
					normalizeMetricAlertReasonKey(matchedAlert),
				)
			}
			if _, err := c.triggerAutoPolicyInspection(ctx, clusterID, policy, reason); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

// CheckErrorSpikePolicies evaluates error-spike policies against recent diagnostics error events.
// CheckErrorSpikePolicies 根据近期诊断错误事件评估错误频率激增策略。
func (c *AutoPolicyChecker) CheckErrorSpikePolicies(ctx context.Context, clusterID uint, now time.Time) error {
	if c == nil || c.repo == nil || c.service == nil || clusterID == 0 {
		return nil
	}

	policies, err := c.repo.ListEnabledPoliciesForCluster(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("auto-policy: failed to list error-spike policies for cluster %d: %w", clusterID, err)
	}
	if len(policies) == 0 {
		return nil
	}

	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	for _, policy := range policies {
		if policy == nil || !policy.Enabled {
			continue
		}
		for _, condition := range policy.Conditions {
			if !condition.Enabled || condition.TemplateCode != ConditionCodeErrorSpike {
				continue
			}
			matchedBurst, ok, err := c.matchErrorSpikeCondition(ctx, clusterID, condition, now)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			reason := fmt.Sprintf(
				"%s|%s count=%d window=%dm group=%d",
				c.policyReasonPrefix(policy.ID),
				condition.TemplateCode,
				matchedBurst.RecentCount,
				resolveConditionWindowMinutes(condition),
				matchedBurst.ErrorGroupID,
			)
			if matchedBurst.Group != nil {
				groupLabel := strings.TrimSpace(firstNonEmptyString(
					matchedBurst.Group.Title,
					matchedBurst.Group.ExceptionClass,
					matchedBurst.Group.SampleMessage,
				))
				if groupLabel != "" {
					reason = fmt.Sprintf("%s title=%s", reason, truncateString(groupLabel, 80))
				}
			}
			if _, err := c.triggerAutoPolicyInspection(ctx, clusterID, policy, reason); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (c *AutoPolicyChecker) matchJavaErrorCondition(condition InspectionConditionItem, exceptionClassLower, messageSnippetLower string) (bool, InspectionConditionTemplateCode) {
	code := condition.TemplateCode

	switch code {
	case ConditionCodeJavaOOM:
		if strings.Contains(exceptionClassLower, "outofmemoryerror") {
			return true, code
		}
	case ConditionCodeJavaStackOverflow:
		if strings.Contains(exceptionClassLower, "stackoverflowerror") {
			return true, code
		}
	case ConditionCodeJavaMetaspace:
		if strings.Contains(exceptionClassLower, "outofmemoryerror") && strings.Contains(messageSnippetLower, "metaspace") {
			return true, code
		}
	default:
	}

	if len(condition.ExtraKeywords) > 0 {
		for _, keyword := range condition.ExtraKeywords {
			kw := strings.ToLower(strings.TrimSpace(keyword))
			if kw == "" {
				continue
			}
			if strings.Contains(messageSnippetLower, kw) || strings.Contains(exceptionClassLower, kw) {
				return true, code
			}
		}
	}

	return false, ""
}

func (c *AutoPolicyChecker) matchErrorSpikeCondition(
	ctx context.Context,
	clusterID uint,
	condition InspectionConditionItem,
	now time.Time,
) (*recentErrorGroupBurst, bool, error) {
	template, ok := findBuiltinConditionTemplate(condition.TemplateCode)
	if !ok {
		return nil, false, fmt.Errorf("unknown error spike template %s", condition.TemplateCode)
	}
	windowMinutes := resolveConditionWindowMinutes(condition)
	threshold := resolveConditionThreshold(condition, template.DefaultThreshold)
	if threshold <= 0 {
		threshold = 1
	}

	bursts, err := c.repo.ListRecentErrorGroupBursts(
		ctx,
		clusterID,
		now.Add(-time.Duration(windowMinutes)*time.Minute),
		50,
	)
	if err != nil {
		return nil, false, fmt.Errorf(
			"auto-policy: failed to list error bursts for cluster %d: %w",
			clusterID,
			err,
		)
	}
	for _, burst := range bursts {
		if burst == nil || burst.RecentCount < int64(threshold) {
			continue
		}
		if !errorSpikeBurstMatchesKeywords(condition, burst) {
			continue
		}
		return burst, true, nil
	}
	return nil, false, nil
}

func (c *AutoPolicyChecker) matchScheduledCondition(policy *InspectionAutoPolicy, condition InspectionConditionItem, now time.Time) (bool, string, error) {
	expr, err := resolveScheduledCronExpression(condition)
	if err != nil {
		return false, "", err
	}
	matched, _, _, err := schedulex.MatchMinuteWindow(expr, now.UTC(), schedulex.DefaultTimezone)
	if err != nil {
		return false, "", err
	}
	if !matched {
		return false, "", nil
	}

	return true, fmt.Sprintf("%s|%s cron=%s", c.policyReasonPrefix(policy.ID), condition.TemplateCode, expr), nil
}

func (c *AutoPolicyChecker) triggerAutoPolicyInspection(ctx context.Context, clusterID uint, policy *InspectionAutoPolicy, triggerReason string) (bool, error) {
	if policy == nil {
		return false, nil
	}

	if c.isInCooldown(ctx, clusterID, policy.ID, policy.CooldownMinutes) {
		return false, nil
	}

	report, err := c.service.StartInspection(ctx, &StartClusterInspectionRequest{
		ClusterID:     clusterID,
		TriggerSource: InspectionTriggerSourceAuto,
	}, 0, c.policyRequestedBy(policy.ID))
	if err != nil {
		return false, fmt.Errorf("auto-policy: failed to start inspection for cluster %d (reason: %s): %w", clusterID, triggerReason, err)
	}

	if report != nil && report.Report != nil && report.Report.ID > 0 {
		trimmedReason := truncateString(strings.TrimSpace(triggerReason), 200)
		if err := c.repo.UpdateInspectionReportAutoTriggerReason(ctx, report.Report.ID, trimmedReason); err != nil {
			log.Printf("[DiagnosticsAutoPolicy] update auto trigger reason failed: cluster_id=%d policy_id=%d report_id=%d err=%v", clusterID, policy.ID, report.Report.ID, err)
		}
	}

	if policy.AutoCreateTask && report != nil && report.Report != nil {
		options := policy.TaskOptions.Normalize()
		_, taskErr := c.service.CreateDiagnosticTask(ctx, &CreateDiagnosticTaskRequest{
			ClusterID:     report.Report.ClusterID,
			TriggerSource: DiagnosticTaskSourceInspectionFinding,
			SourceRef: DiagnosticTaskSourceRef{
				InspectionReportID: report.Report.ID,
			},
			Options:   options,
			AutoStart: policy.AutoStartTask,
		}, 0, "auto-policy")
		if taskErr != nil {
			log.Printf("[DiagnosticsAutoPolicy] auto create diagnostic task failed: cluster_id=%d policy_id=%d report_id=%d err=%v", clusterID, policy.ID, report.Report.ID, taskErr)
		}
	}

	return true, nil
}

func (c *AutoPolicyChecker) isInCooldown(ctx context.Context, clusterID uint, policyID uint, cooldownMinutes int) bool {
	if cooldownMinutes <= 0 {
		cooldownMinutes = 30
	}
	lastReport, err := c.repo.GetLastInspectionReportForRequester(ctx, clusterID, c.policyRequestedBy(policyID))
	if err != nil {
		if errors.Is(err, ErrInspectionReportNotFound) {
			return false
		}
		return true
	}
	cooldownDeadline := time.Now().UTC().Add(-time.Duration(cooldownMinutes) * time.Minute)
	return lastReport.CreatedAt.After(cooldownDeadline)
}

func (c *AutoPolicyChecker) policyRequestedBy(policyID uint) string {
	return fmt.Sprintf("%s%d", autoPolicyRequestedByPrefix, policyID)
}

func (c *AutoPolicyChecker) policyReasonPrefix(policyID uint) string {
	return fmt.Sprintf("policy:%d", policyID)
}

func resolveScheduledCronExpression(condition InspectionConditionItem) (string, error) {
	expr := strings.TrimSpace(condition.CronExprOverride)
	if expr != "" {
		return expr, nil
	}
	template, ok := findBuiltinConditionTemplate(condition.TemplateCode)
	if !ok {
		return "", fmt.Errorf("unknown schedule template %s", condition.TemplateCode)
	}
	expr = strings.TrimSpace(template.DefaultCronExpr)
	if expr == "" {
		return "", fmt.Errorf("default cron expression is empty for %s", condition.TemplateCode)
	}
	return expr, nil
}

func matchMetricAlertCondition(condition InspectionConditionItem, alerts []*monitoringapp.AlertInstance) (*monitoringapp.AlertInstance, bool) {
	for _, alert := range alerts {
		if alert == nil || alert.Status != monitoringapp.AlertDisplayStatusFiring {
			continue
		}
		if metricAlertMatchesCondition(condition, alert) {
			return alert, true
		}
	}
	return nil, false
}

func matchAlertFiringCondition(condition InspectionConditionItem, alerts []*monitoringapp.AlertInstance) (*monitoringapp.AlertInstance, bool) {
	for _, alert := range alerts {
		if alert == nil || alert.Status != monitoringapp.AlertDisplayStatusFiring {
			continue
		}
		if !alertMatchesKeywords(condition, alert) {
			continue
		}
		return alert, true
	}
	return nil, false
}

func matchNodeUnhealthyCondition(condition InspectionConditionItem, alerts []*monitoringapp.AlertInstance, now time.Time) (int, bool) {
	windowMinutes := resolveConditionWindowMinutes(condition)
	threshold := resolveConditionThreshold(condition, 1)
	if threshold <= 0 {
		threshold = 1
	}
	distinctNodes := map[string]struct{}{}
	for _, alert := range alerts {
		if alert == nil || alert.Status != monitoringapp.AlertDisplayStatusFiring {
			continue
		}
		if !isNodeUnhealthyAlert(alert) {
			continue
		}
		if !alertMatchesKeywords(condition, alert) {
			continue
		}
		firingAt := alert.FiringAt.UTC()
		if !firingAt.IsZero() && now.UTC().Sub(firingAt) < time.Duration(windowMinutes)*time.Minute {
			continue
		}
		key := normalizeAlertNodeKey(alert)
		if key == "" {
			key = strings.TrimSpace(alert.AlertID)
		}
		if key == "" {
			continue
		}
		distinctNodes[key] = struct{}{}
	}
	return len(distinctNodes), len(distinctNodes) >= threshold
}

func errorSpikeBurstMatchesKeywords(condition InspectionConditionItem, burst *recentErrorGroupBurst) bool {
	if burst == nil || len(condition.ExtraKeywords) == 0 {
		return true
	}
	haystacks := []string{}
	if burst.Group != nil {
		haystacks = append(
			haystacks,
			strings.ToLower(strings.TrimSpace(burst.Group.Title)),
			strings.ToLower(strings.TrimSpace(burst.Group.ExceptionClass)),
			strings.ToLower(strings.TrimSpace(burst.Group.SampleMessage)),
			strings.ToLower(strings.TrimSpace(burst.Group.Fingerprint)),
		)
	}
	for _, keyword := range condition.ExtraKeywords {
		needle := strings.ToLower(strings.TrimSpace(keyword))
		if needle == "" {
			continue
		}
		for _, haystack := range haystacks {
			if haystack != "" && strings.Contains(haystack, needle) {
				return true
			}
		}
	}
	return false
}

func isNodeUnhealthyAlert(alert *monitoringapp.AlertInstance) bool {
	if alert == nil {
		return false
	}
	ruleKey := strings.ToLower(strings.TrimSpace(alert.RuleKey))
	alertName := strings.ToLower(strings.TrimSpace(alert.AlertName))
	summary := strings.ToLower(strings.TrimSpace(alert.Summary))
	description := strings.ToLower(strings.TrimSpace(alert.Description))
	return containsAnyMetricAlertAlias(
		[]string{ruleKey, alertName, summary, description},
		"node_offline",
		"node offline",
		"node unavailable",
		"node unhealthy",
	)
}

func normalizeAlertNodeKey(alert *monitoringapp.AlertInstance) string {
	if alert == nil || alert.SourceRef == nil {
		return ""
	}
	for _, candidate := range []string{
		strings.TrimSpace(alert.SourceRef.Hostname),
		strings.TrimSpace(alert.SourceRef.ProcessName),
		strings.TrimSpace(alert.SourceRef.EventType),
		strings.TrimSpace(alert.SourceRef.Fingerprint),
	} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func alertMatchesKeywords(condition InspectionConditionItem, alert *monitoringapp.AlertInstance) bool {
	if alert == nil || len(condition.ExtraKeywords) == 0 {
		return true
	}
	haystacks := []string{
		strings.ToLower(strings.TrimSpace(alert.RuleKey)),
		strings.ToLower(strings.TrimSpace(alert.AlertName)),
		strings.ToLower(strings.TrimSpace(alert.Summary)),
		strings.ToLower(strings.TrimSpace(alert.Description)),
	}
	if alert.SourceRef != nil {
		haystacks = append(
			haystacks,
			strings.ToLower(strings.TrimSpace(alert.SourceRef.EventType)),
			strings.ToLower(strings.TrimSpace(alert.SourceRef.ProcessName)),
			strings.ToLower(strings.TrimSpace(alert.SourceRef.Hostname)),
			strings.ToLower(strings.TrimSpace(alert.SourceRef.Fingerprint)),
		)
	}
	for _, keyword := range condition.ExtraKeywords {
		needle := strings.ToLower(strings.TrimSpace(keyword))
		if needle == "" {
			continue
		}
		for _, haystack := range haystacks {
			if haystack != "" && strings.Contains(haystack, needle) {
				return true
			}
		}
	}
	return false
}

func resolveConditionWindowMinutes(condition InspectionConditionItem) int {
	template, ok := findBuiltinConditionTemplate(condition.TemplateCode)
	defaultMinutes := 5
	if ok && template.DefaultWindowMinutes > 0 {
		defaultMinutes = template.DefaultWindowMinutes
	}
	if condition.WindowMinutesOverride != nil && *condition.WindowMinutesOverride > 0 {
		return *condition.WindowMinutesOverride
	}
	return defaultMinutes
}

func resolveConditionThreshold(condition InspectionConditionItem, fallback int) int {
	if condition.ThresholdOverride != nil && *condition.ThresholdOverride > 0 {
		return *condition.ThresholdOverride
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func metricAlertMatchesCondition(condition InspectionConditionItem, alert *monitoringapp.AlertInstance) bool {
	if alert == nil {
		return false
	}
	haystacks := []string{
		strings.ToLower(strings.TrimSpace(alert.RuleKey)),
		strings.ToLower(strings.TrimSpace(alert.AlertName)),
		strings.ToLower(strings.TrimSpace(alert.Summary)),
		strings.ToLower(strings.TrimSpace(alert.Description)),
	}

	switch condition.TemplateCode {
	case ConditionCodePromCPUHigh:
		return containsAnyMetricAlertAlias(haystacks, "cpu_usage_high", "cpu load high", "cpu high")
	case ConditionCodePromHeapHigh:
		return containsAnyMetricAlertAlias(haystacks, "memory_usage_high", "heap memory usage high", "heap usage high", "memory high")
	case ConditionCodePromGCFrequent:
		return containsAnyMetricAlertAlias(haystacks, "gc_frequent", "gc frequent", "gc time ratio high", "gc high")
	case ConditionCodePromHeapRising:
		return containsAnyMetricAlertAlias(haystacks, "heap_rising", "heap rising", "heap keeps rising", "memory rising")
	default:
		return false
	}
}

func containsAnyMetricAlertAlias(haystacks []string, aliases ...string) bool {
	for _, haystack := range haystacks {
		if haystack == "" {
			continue
		}
		for _, alias := range aliases {
			if alias != "" && strings.Contains(haystack, alias) {
				return true
			}
		}
	}
	return false
}

func normalizeMetricAlertReasonKey(alert *monitoringapp.AlertInstance) string {
	if alert == nil {
		return ""
	}
	if value := strings.TrimSpace(alert.RuleKey); value != "" {
		return value
	}
	if value := strings.TrimSpace(alert.AlertName); value != "" {
		return value
	}
	return strings.TrimSpace(alert.Summary)
}
