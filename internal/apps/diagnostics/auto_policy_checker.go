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

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/pkg/schedulex"
	"github.com/LeonYoah/stx/internal/pkg/timex"
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

// CheckAlertFiringPolicies evaluates alert-firing policies through unified alert instances.
// CheckAlertFiringPolicies 通过统一告警实例评估告警触发类策略。
func (c *AutoPolicyChecker) CheckAlertFiringPolicies(ctx context.Context, clusterID uint, now time.Time) error {
	if c == nil || c.repo == nil || c.service == nil || clusterID == 0 {
		return nil
	}
	if c.service.monitoringService == nil {
		return nil
	}

	policies, err := c.repo.ListEnabledPoliciesForCluster(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("auto-policy: failed to list alert-firing policies for cluster %d: %w", clusterID, err)
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
			if condition.TemplateCode != ConditionCodeAlertFiring {
				continue
			}
			_, targetCount, ok := matchAlertFiringCondition(condition, alerts.Alerts, now)
			if !ok {
				continue
			}
			reason := fmt.Sprintf(
				"%s|%s rules=%s targets=%d window=%dm",
				c.policyReasonPrefix(policy.ID),
				condition.TemplateCode,
				strings.Join(condition.RuleKeys, ","),
				targetCount,
				resolveConditionWindowMinutes(condition),
			)
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
		now = timex.Now()
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
	matched, _, _, err := schedulex.MatchMinuteWindow(expr, now.UTC(), schedulex.NormalizeTimezone(""))
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
		if err := c.repo.SetInspectionReportAutoTrigger(ctx, report.Report.ID, policy.ID, trimmedReason); err != nil {
			log.Printf("[DiagnosticsAutoPolicy] update auto trigger reason failed: cluster_id=%d policy_id=%d report_id=%d err=%v", clusterID, policy.ID, report.Report.ID, err)
		}
	}

	if policy.AutoCreateTask && report != nil && report.Report != nil {
		options := policy.TaskOptions.Normalize()
		request := &CreateDiagnosticTaskRequest{
			ClusterID:     report.Report.ClusterID,
			TriggerSource: DiagnosticTaskSourceInspectionFinding,
			SourceRef: DiagnosticTaskSourceRef{
				InspectionReportID: report.Report.ID,
			},
			Options:   options,
			AutoStart: policy.AutoStartTask,
		}
		requestHash, hashErr := executionapp.HashRequest(struct {
			PolicyID uint                  `json:"policy_id"`
			ReportID uint                  `json:"report_id"`
			Options  DiagnosticTaskOptions `json:"options"`
		}{PolicyID: policy.ID, ReportID: report.Report.ID, Options: options})
		if hashErr != nil {
			requestHash = executionapp.HashString(fmt.Sprintf("diagnostics-auto-task:%d:%d", policy.ID, report.Report.ID))
		}
		_, taskErr := c.service.CreateDiagnosticTaskWithExecution(ctx, request, 0, "auto-policy", DiagnosticExecutionRequest{
			RequestID:      fmt.Sprintf("auto-policy-%d-report-%d", policy.ID, report.Report.ID),
			IdempotencyKey: fmt.Sprintf("diagnostics-auto-task:%d:%d", policy.ID, report.Report.ID),
			RequestHash:    requestHash,
			IsAdmin:        true,
			ClientType:     "system",
		})
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
	cooldownDeadline := timex.Now().Add(-time.Duration(cooldownMinutes) * time.Minute)
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

// matchAlertFiringCondition 只认规则键精确相等；持续时长按 FiringAt 计算，对象数按 alertTargetKey 去重，
// 返回首个命中的告警、去重后的对象数以及是否达到阈值。
// matchAlertFiringCondition matches rule keys exactly; duration is measured from FiringAt and targets
// are deduped by alertTargetKey. It returns the first matching alert, the deduped target count and
// whether the threshold was reached.
func matchAlertFiringCondition(condition InspectionConditionItem, alerts []*monitoringapp.AlertInstance, now time.Time) (*monitoringapp.AlertInstance, int, bool) {
	wanted := make(map[string]struct{}, len(condition.RuleKeys))
	for _, key := range condition.RuleKeys {
		if key = strings.TrimSpace(key); key != "" {
			wanted[key] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, 0, false
	}
	windowMinutes := resolveConditionWindowMinutes(condition)
	threshold := resolveConditionThreshold(condition, 1)
	var matched *monitoringapp.AlertInstance
	targets := map[string]struct{}{}
	for _, alert := range alerts {
		if alert == nil || alert.Status != monitoringapp.AlertDisplayStatusFiring {
			continue
		}
		if _, ok := wanted[strings.TrimSpace(alert.RuleKey)]; !ok {
			continue
		}
		if firingAt := alert.FiringAt.UTC(); !firingAt.IsZero() && now.UTC().Sub(firingAt) < time.Duration(windowMinutes)*time.Minute {
			continue
		}
		if key := alertTargetKey(alert); key != "" {
			targets[key] = struct{}{}
		}
		if matched == nil {
			matched = alert
		}
	}
	return matched, len(targets), len(targets) >= threshold
}

// alertTargetKey 返回告警对象的去重键：主机名 → 任务 ID → 指纹 → 告警实例 ID，取第一个非空值；
// 指标告警每个对象一条序列，指纹已按对象区分。
// alertTargetKey returns the dedupe key of the alerting object: hostname, task ID, fingerprint,
// then the alert instance ID — the first non-empty wins. Metric alerts produce one series per
// object, so the fingerprint already distinguishes objects.
func alertTargetKey(alert *monitoringapp.AlertInstance) string {
	if alert == nil {
		return ""
	}
	if ref := alert.SourceRef; ref != nil {
		if value := strings.TrimSpace(ref.Hostname); value != "" {
			return "host:" + value
		}
		if ref.TaskID > 0 {
			return fmt.Sprintf("task:%d", ref.TaskID)
		}
		if value := strings.TrimSpace(ref.Fingerprint); value != "" {
			return "fp:" + value
		}
	}
	return strings.TrimSpace(alert.AlertID)
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

func resolveConditionWindowMinutes(condition InspectionConditionItem) int {
	// 模板默认窗口优先：ALERT_FIRING 默认 0 表示立即触发，ERROR_SPIKE 默认 5 分钟。
	// The template default wins: ALERT_FIRING defaults to 0 (immediate), ERROR_SPIKE to 5 minutes.
	defaultMinutes := 5
	if template, ok := findBuiltinConditionTemplate(condition.TemplateCode); ok {
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
