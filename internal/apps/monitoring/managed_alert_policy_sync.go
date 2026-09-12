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

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/config"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

const (
	managedPrometheusRuleFileName       = "stx-managed-alert-policies.yml"
	managedPrometheusRuleGroupName      = "stx-managed-alert-policies"
	managedAlertmanagerDefaultReceiver  = "stx-webhook"
	defaultAlertReminderIntervalMinutes = 10
)

type managedObservabilityRuntimePaths struct {
	PrometheusDir      string
	PrometheusRuleFile string
	PromtoolPath       string
	AlertmanagerDir    string
	AlertmanagerConfig string
	AlertmanagerPID    string
}

type managedPrometheusRuleFile struct {
	Groups []managedPrometheusRuleGroup `yaml:"groups"`
}

type managedPrometheusRuleGroup struct {
	Name  string                       `yaml:"name"`
	Rules []managedPrometheusAlertRule `yaml:"rules"`
}

type managedPrometheusAlertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type resolvedMetricPolicyCondition struct {
	Operator      string
	Threshold     string
	WindowMinutes int
}

// SyncManagedAlertingArtifacts keeps local Prometheus/Alertmanager demo runtime in sync with unified metric policies.
// SyncManagedAlertingArtifacts 将统一指标策略同步到本地 Prometheus / Alertmanager demo 运行时。
func (s *Service) SyncManagedAlertingArtifacts(ctx context.Context) error {
	if s == nil || s.repo == nil || !config.Config.Observability.Enabled {
		return nil
	}
	return s.syncManagedAlertingArtifactsWithRepo(ctx, s.repo)
}

func (s *Service) syncManagedAlertingArtifactsWithRepo(ctx context.Context, repo *Repository) error {
	if repo == nil || !config.Config.Observability.Enabled {
		return nil
	}

	paths, err := discoverManagedObservabilityRuntimePaths()
	if err != nil {
		return nil
	}

	if err := s.syncManagedPrometheusRuleFile(ctx, repo, paths); err != nil {
		return err
	}
	if err := writeManagedAlertmanagerConfig(paths.AlertmanagerConfig); err != nil {
		return err
	}
	if err := reloadManagedPrometheus(ctx); err != nil {
		return err
	}
	if err := reloadManagedAlertmanager(paths.AlertmanagerPID); err != nil {
		return err
	}
	return nil
}

func (s *Service) syncManagedPrometheusRuleFile(ctx context.Context, repo *Repository, paths *managedObservabilityRuntimePaths) error {
	if repo == nil || paths == nil {
		return nil
	}

	policies, err := repo.ListEnabledManagedMetricAlertPolicies(ctx)
	if err != nil {
		return err
	}
	clusterNames, err := s.listManagedAlertPolicyClusterNames(ctx)
	if err != nil {
		return err
	}
	payload, err := s.buildManagedPrometheusRulePayload(policies, clusterNames)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(paths.PrometheusRuleFile, payload, 0o644); err != nil {
		return err
	}
	if err := validateManagedPrometheusRuleFile(paths.PromtoolPath, paths.PrometheusRuleFile); err != nil {
		return err
	}
	return nil
}

func discoverManagedObservabilityRuntimePaths() (*managedObservabilityRuntimePaths, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	prometheusDir, err := resolveSingleGlobDir(filepath.Join(cwd, "deps", "prometheus-*"))
	if err != nil {
		return nil, err
	}
	alertmanagerDir, err := resolveSingleGlobDir(filepath.Join(cwd, "deps", "alertmanager-*"))
	if err != nil {
		return nil, err
	}

	return &managedObservabilityRuntimePaths{
		PrometheusDir:      prometheusDir,
		PrometheusRuleFile: filepath.Join(prometheusDir, "rules", managedPrometheusRuleFileName),
		PromtoolPath:       filepath.Join(prometheusDir, "promtool"),
		AlertmanagerDir:    alertmanagerDir,
		AlertmanagerConfig: filepath.Join(alertmanagerDir, "alertmanager.yml"),
		AlertmanagerPID:    filepath.Join(alertmanagerDir, "alertmanager.pid"),
	}, nil
}

func resolveSingleGlobDir(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no runtime directory matched %s", pattern)
	}
	sort.Strings(matches)
	for _, candidate := range matches {
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no runtime directory matched %s", pattern)
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, content, mode); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func validateManagedPrometheusRuleFile(promtoolPath, ruleFile string) error {
	if strings.TrimSpace(promtoolPath) == "" {
		return nil
	}
	if _, err := os.Stat(promtoolPath); err != nil {
		return nil
	}

	cmd := exec.Command(promtoolPath, "check", "rules", ruleFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("managed metric rules are invalid: %s", message)
	}
	return nil
}

func reloadManagedPrometheus(ctx context.Context) error {
	apiURL := joinURL(config.Config.Observability.Prometheus.URL, "/-/reload")
	if strings.TrimSpace(apiURL) == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("prometheus reload returned http %d", resp.StatusCode)
	}
	return nil
}

func reloadManagedAlertmanager(pidFile string) error {
	trimmed := strings.TrimSpace(pidFile)
	if trimmed == "" {
		return nil
	}
	raw, err := os.ReadFile(trimmed)
	if err != nil {
		return nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGHUP)
}

func writeManagedAlertmanagerConfig(path string) error {
	webhookURL := joinURL(config.GetExternalURL(), config.Config.Observability.Alertmanager.WebhookPath)
	if strings.TrimSpace(webhookURL) == "" {
		return fmt.Errorf("app.external_url is empty, cannot build alertmanager webhook url")
	}

	content := fmt.Sprintf(`global:
  resolve_timeout: 5m

route:
  receiver: %s
  group_by: [alertname, cluster, instance, policy_id]
  group_wait: 30s
  group_interval: 5m
  repeat_interval: %dm

receivers:
  - name: %s
    webhook_configs:
      - url: '%s'
        send_resolved: true
`, managedAlertmanagerDefaultReceiver, defaultAlertReminderIntervalMinutes, managedAlertmanagerDefaultReceiver, webhookURL)

	return atomicWriteFile(path, []byte(content), 0o644)
}

func (r *Repository) ListEnabledManagedMetricAlertPolicies(ctx context.Context) ([]*AlertPolicy, error) {
	var policies []*AlertPolicy
	if err := r.db.WithContext(ctx).
		Where("enabled = ? AND policy_type IN ?", true, []AlertPolicyBuilderKind{AlertPolicyBuilderKindMetricsTemplate, AlertPolicyBuilderKindCustomPromQL}).
		Order("id ASC").
		Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}

func (s *Service) listManagedAlertPolicyClusterNames(ctx context.Context) (map[string]string, error) {
	result := map[string]string{"all": "全部集群"}
	if s == nil || s.clusterService == nil {
		return result, nil
	}

	clusters, _, err := s.clusterService.List(ctx, &clusterapp.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return nil, err
	}
	for _, item := range clusters {
		if item == nil || item.ID == 0 {
			continue
		}
		result[strconv.FormatUint(uint64(item.ID), 10)] = strings.TrimSpace(item.Name)
	}
	return result, nil
}

func (s *Service) buildManagedPrometheusRulePayload(policies []*AlertPolicy, clusterNames map[string]string) ([]byte, error) {
	rules := make([]managedPrometheusAlertRule, 0, len(policies))
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		rule, ok, err := s.buildManagedPrometheusAlertRule(policy, clusterNames)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		rules = append(rules, *rule)
	}

	payload := managedPrometheusRuleFile{Groups: []managedPrometheusRuleGroup{}}
	if len(rules) > 0 {
		payload.Groups = append(payload.Groups, managedPrometheusRuleGroup{
			Name:  managedPrometheusRuleGroupName,
			Rules: rules,
		})
	}

	raw, err := yaml.Marshal(&payload)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *Service) buildManagedPrometheusAlertRule(policy *AlertPolicy, clusterNames map[string]string) (*managedPrometheusAlertRule, bool, error) {
	if policy == nil || !policy.Enabled {
		return nil, false, nil
	}

	clusterScopeName := strings.TrimSpace(firstNonEmpty(clusterNames[strings.TrimSpace(policy.ClusterID)], policy.ClusterID, "全部集群"))
	forDuration := ""
	labels := map[string]string{
		"managed_by":  "stx",
		"policy_id":   strconv.FormatUint(uint64(policy.ID), 10),
		"policy_name": strings.TrimSpace(firstNonEmpty(policy.Name, "未命名策略")),
		"policy_type": string(policy.PolicyType),
		"severity":    strings.TrimSpace(firstNonEmpty(string(normalizeAlertPolicySeverity(policy.Severity)), string(AlertSeverityWarning))),
	}
	annotations := map[string]string{
		"policy_id":     strconv.FormatUint(uint64(policy.ID), 10),
		"policy_name":   strings.TrimSpace(firstNonEmpty(policy.Name, "未命名策略")),
		"cluster_scope": clusterScopeName,
	}
	if strings.TrimSpace(policy.TemplateKey) != "" {
		labels["template_key"] = strings.TrimSpace(policy.TemplateKey)
		annotations["template_key"] = strings.TrimSpace(policy.TemplateKey)
		annotations["template_name"] = alertPolicyTemplateDisplayNameZH(strings.TrimSpace(policy.TemplateKey))
	}

	var expr string
	switch policy.PolicyType {
	case AlertPolicyBuilderKindMetricsTemplate:
		template, ok := findAlertPolicyTemplateSummary(strings.TrimSpace(policy.TemplateKey))
		if !ok {
			return nil, false, nil
		}
		condition := resolveMetricPolicyCondition(policy, template)
		annotations["condition"] = buildMetricConditionDescriptionZH(condition)
		annotations["summary"] = buildManagedMetricAlertSummaryAnnotation(policy, template)
		annotations["description"] = buildManagedMetricAlertDescriptionAnnotation(policy, template, condition)
		annotations["current_value"] = `{{ printf "%.6f" $value }}`
		forDuration = formatPrometheusAlertForDuration(condition.WindowMinutes)

		builtExpr, err := buildMetricTemplatePromQL(policy, template, condition)
		if err != nil {
			return nil, false, err
		}
		expr = builtExpr
	case AlertPolicyBuilderKindCustomPromQL:
		expr = strings.TrimSpace(policy.PromQL)
		if expr == "" {
			return nil, false, nil
		}
		condition := resolveCustomPromQLWindow(policy)
		forDuration = formatPrometheusAlertForDuration(condition.WindowMinutes)
		annotations["summary"] = fmt.Sprintf(`{{ $labels.cluster_name }} / {{ $labels.instance }} 指标策略“%s”已触发`, escapePrometheusTemplateText(strings.TrimSpace(firstNonEmpty(policy.Name, "未命名策略"))))
		annotations["description"] = fmt.Sprintf("自定义 PromQL 条件命中，当前值：{{ printf \"%%.6f\" $value }}。")
	default:
		return nil, false, nil
	}

	rule := &managedPrometheusAlertRule{
		Alert:       sanitizeManagedPrometheusAlertName(policy.ID),
		Expr:        expr,
		For:         forDuration,
		Labels:      labels,
		Annotations: annotations,
	}
	return rule, true, nil
}

func sanitizeManagedPrometheusAlertName(policyID uint) string {
	return fmt.Sprintf("STXManagedPolicy_%d", policyID)
}

func findAlertPolicyTemplateSummary(templateKey string) (*AlertPolicyTemplateSummaryDTO, bool) {
	for _, item := range allAlertPolicyTemplateSummaries() {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.Key) == strings.TrimSpace(templateKey) {
			return item, true
		}
	}
	return nil, false
}

func resolveMetricPolicyCondition(policy *AlertPolicy, template *AlertPolicyTemplateSummaryDTO) resolvedMetricPolicyCondition {
	condition := resolvedMetricPolicyCondition{
		Operator:      strings.TrimSpace(firstNonEmpty(template.DefaultOperator, ">")),
		Threshold:     strings.TrimSpace(firstNonEmpty(template.DefaultThreshold, "0")),
		WindowMinutes: template.DefaultWindowMins,
	}
	conditions := unmarshalAlertPolicyConditions(policy.ConditionsJSON)
	if len(conditions) > 0 && conditions[0] != nil {
		if value := strings.TrimSpace(conditions[0].Operator); value != "" {
			condition.Operator = value
		}
		if value := strings.TrimSpace(conditions[0].Threshold); value != "" {
			condition.Threshold = value
		}
		if conditions[0].WindowMinutes > 0 {
			condition.WindowMinutes = conditions[0].WindowMinutes
		}
	}
	if condition.WindowMinutes <= 0 {
		condition.WindowMinutes = 1
	}
	if condition.Operator == "" {
		condition.Operator = ">"
	}
	if condition.Threshold == "" {
		condition.Threshold = "0"
	}
	return condition
}

func resolveCustomPromQLWindow(policy *AlertPolicy) resolvedMetricPolicyCondition {
	condition := resolvedMetricPolicyCondition{WindowMinutes: 1}
	conditions := unmarshalAlertPolicyConditions(policy.ConditionsJSON)
	if len(conditions) > 0 && conditions[0] != nil && conditions[0].WindowMinutes > 0 {
		condition.WindowMinutes = conditions[0].WindowMinutes
	}
	return condition
}

func formatPrometheusAlertForDuration(windowMinutes int) string {
	if windowMinutes <= 0 {
		return ""
	}
	return fmt.Sprintf("%dm", windowMinutes)
}

func buildMetricTemplatePromQL(policy *AlertPolicy, template *AlertPolicyTemplateSummaryDTO, condition resolvedMetricPolicyCondition) (string, error) {
	if template == nil {
		return "", fmt.Errorf("template is required")
	}

	matchers := []string{`job="seatunnel_engine_http"`}
	clusterID := strings.TrimSpace(policy.ClusterID)
	if clusterID != "" && !strings.EqualFold(clusterID, "all") {
		matchers = append(matchers, fmt.Sprintf(`cluster_id=%q`, clusterID))
	}
	window := fmt.Sprintf("%dm", maxInt(condition.WindowMinutes, 1))
	op := strings.TrimSpace(condition.Operator)
	threshold := strings.TrimSpace(condition.Threshold)
	if threshold == "" {
		threshold = "0"
	}

	switch strings.TrimSpace(template.Key) {
	case "cpu_usage_high":
		return fmt.Sprintf(`sum by (cluster_id, cluster_name, cluster, instance) (rate(%s[%s])) %s %s`, buildPrometheusSelector("process_cpu_seconds_total", matchers...), window, op, threshold), nil
	case "memory_usage_high":
		used := buildPrometheusSelector("jvm_memory_bytes_used", append(matchers, `area="heap"`)...)
		maxv := buildPrometheusSelector("jvm_memory_bytes_max", append(matchers, `area="heap"`)...)
		return fmt.Sprintf(`max by (cluster_id, cluster_name, cluster, instance) ((%s) / clamp_min((%s), 1)) %s %s`, used, maxv, op, threshold), nil
	case "fd_usage_high":
		openFds := buildPrometheusSelector("process_open_fds", matchers...)
		maxFds := buildPrometheusSelector("process_max_fds", matchers...)
		return fmt.Sprintf(`max by (cluster_id, cluster_name, cluster, instance) ((%s) / clamp_min((%s), 1)) %s %s`, openFds, maxFds, op, threshold), nil
	case "failed_jobs_high":
		return fmt.Sprintf(`sum by (cluster_id, cluster_name, cluster, instance) (%s) %s %s`, buildPrometheusSelector("job_count", append(matchers, `state="FAILED"`)...), op, threshold), nil
	case "job_thread_pool_queue_backlog_high":
		return fmt.Sprintf(`max by (cluster_id, cluster_name, cluster, instance) (%s) %s %s`, buildPrometheusSelector("job_thread_pool_queueTaskCount", matchers...), op, threshold), nil
	case "job_thread_pool_rejection_high":
		return fmt.Sprintf(`sum by (cluster_id, cluster_name, cluster, instance) (increase(%s[%s])) %s %s`, buildPrometheusSelector("job_thread_pool_rejection_total", matchers...), window, op, threshold), nil
	case "deadlocked_threads_detected":
		return fmt.Sprintf(`max by (cluster_id, cluster_name, cluster, instance) (%s) %s %s`, buildPrometheusSelector("jvm_threads_deadlocked", matchers...), op, threshold), nil
	case "split_brain_risk":
		return fmt.Sprintf(`min by (cluster_id, cluster_name, cluster, instance) (%s) %s %s`, buildPrometheusSelector("hazelcast_partition_isClusterSafe", matchers...), op, threshold), nil
	default:
		return "", fmt.Errorf("unsupported metrics template: %s", template.Key)
	}
}

func buildPrometheusSelector(metric string, matchers ...string) string {
	trimmedMetric := strings.TrimSpace(metric)
	items := make([]string, 0, len(matchers))
	for _, matcher := range matchers {
		matcher = strings.TrimSpace(matcher)
		if matcher == "" {
			continue
		}
		items = append(items, matcher)
	}
	if len(items) == 0 {
		return trimmedMetric
	}
	return fmt.Sprintf("%s{%s}", trimmedMetric, strings.Join(items, ","))
}

func buildMetricConditionDescriptionZH(condition resolvedMetricPolicyCondition) string {
	return fmt.Sprintf("持续 %d 分钟，条件：%s %s", maxInt(condition.WindowMinutes, 1), strings.TrimSpace(firstNonEmpty(condition.Operator, ">")), strings.TrimSpace(firstNonEmpty(condition.Threshold, "0")))
}

func buildManagedMetricAlertSummaryAnnotation(policy *AlertPolicy, template *AlertPolicyTemplateSummaryDTO) string {
	policyName := escapePrometheusTemplateText(strings.TrimSpace(firstNonEmpty(policy.Name, "未命名策略")))
	templateName := escapePrometheusTemplateText(alertPolicyTemplateDisplayNameZH(strings.TrimSpace(template.Key)))
	return fmt.Sprintf(`{{ $labels.cluster_name }} / {{ $labels.instance }} 触发指标告警：%s（%s）`, policyName, templateName)
}

func buildManagedMetricAlertDescriptionAnnotation(policy *AlertPolicy, template *AlertPolicyTemplateSummaryDTO, condition resolvedMetricPolicyCondition) string {
	description := escapePrometheusTemplateText(strings.TrimSpace(firstNonEmpty(
		policy.Description,
		alertPolicyTemplateDescriptionZH(template.Key),
		alertPolicyTemplateDisplayNameZH(template.Key),
	)))
	conditionText := escapePrometheusTemplateText(buildMetricConditionDescriptionZH(condition))
	return fmt.Sprintf(`%s；当前值：{{ printf "%%.6f" $value }}；%s。`, description, conditionText)
}

func escapePrometheusTemplateText(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(strings.TrimSpace(value))
}

func alertPolicyTemplateDisplayNameZH(templateKey string) string {
	switch strings.TrimSpace(templateKey) {
	case "master_unavailable":
		return "Master 不可用"
	case "worker_insufficient":
		return "可用 Worker 不足"
	case AlertRuleKeyNodeOffline:
		return "节点离线"
	case "agent_offline":
		return "Agent 离线"
	case AlertRuleKeyProcessCrashed:
		return "进程崩溃"
	case AlertRuleKeyProcessRestartFailed:
		return "进程重启失败"
	case AlertRuleKeyProcessRestartLimitReached:
		return "达到重启上限"
	case AlertRuleKeyClusterRestartRequested:
		return "重启已触发"
	case AlertRuleKeyNodeStopRequested:
		return "节点停止已触发"
	case "cpu_usage_high":
		return "CPU 异常"
	case "memory_usage_high":
		return "内存异常"
	case "fd_usage_high":
		return "文件句柄异常"
	case "failed_jobs_high":
		return "失败作业异常"
	case "job_thread_pool_queue_backlog_high":
		return "作业线程池积压"
	case "job_thread_pool_rejection_high":
		return "作业线程池拒绝"
	case "deadlocked_threads_detected":
		return "死锁线程异常"
	case "split_brain_risk":
		return "集群安全性异常"
	default:
		return strings.TrimSpace(firstNonEmpty(templateKey, "指标告警"))
	}
}

func alertPolicyTemplateDescriptionZH(templateKey string) string {
	switch strings.TrimSpace(templateKey) {
	case "master_unavailable":
		return "检测受管 SeaTunnel 集群是否已经没有健康的 Master / Coordinator 节点。"
	case "worker_insufficient":
		return "检测单个集群中健康 Worker 数量是否持续低于预期基线。"
	case AlertRuleKeyNodeOffline:
		return "检测节点心跳或运行态可见性是否持续丢失。"
	case "agent_offline":
		return "检测控制面是否失去了集群运维所需的 Agent 连接。"
	case AlertRuleKeyProcessCrashed:
		return "跟踪受管 SeaTunnel 进程是否发生重复崩溃。"
	case AlertRuleKeyProcessRestartFailed:
		return "检测自动重启已经无法恢复故障进程的情况。"
	case AlertRuleKeyProcessRestartLimitReached:
		return "检测运行时是否进入类似 crash-loop 的重启上限异常。"
	case AlertRuleKeyClusterRestartRequested:
		return "当用户在控制面触发集群或节点重启时发送通知。"
	case AlertRuleKeyNodeStopRequested:
		return "当用户在控制面手动停止节点时立即发送通知。"
	case "cpu_usage_high":
		return "基于 process_cpu_seconds_total 持续检测 SeaTunnel 进程 CPU 负载，表示评估窗口内的平均 CPU 核使用量，而不是宿主机总 CPU 百分比。"
	case "memory_usage_high":
		return "基于 jvm_memory_bytes_used 和 jvm_memory_bytes_max 持续检测 SeaTunnel JVM 堆内存使用率是否超过阈值。"
	case "fd_usage_high":
		return "基于 process_open_fds 和 process_max_fds 提前发现文件句柄耗尽风险。"
	case "failed_jobs_high":
		return "基于带 FAILED 状态标签的 job_count 持续检测失败作业数量是否异常。"
	case "job_thread_pool_queue_backlog_high":
		return "基于 job_thread_pool_queueTaskCount 检测作业线程池队列积压，避免演变为高延迟或饥饿。"
	case "job_thread_pool_rejection_high":
		return "当 job_thread_pool_rejection_total 持续增长时告警，表示任务已被拒绝而不是被排队或执行。"
	case "deadlocked_threads_detected":
		return "基于 jvm_threads_deadlocked 检测 JVM 死锁线程，避免任务执行与恢复流程被卡死。"
	case "split_brain_risk":
		return "基于 hazelcast_partition_isClusterSafe 检测集群分区安全性异常，提前发现脑裂风险。"
	default:
		return strings.TrimSpace(firstNonEmpty(alertPolicyTemplateDisplayNameZH(templateKey), "指标告警"))
	}
}

func parseManagedRemotePolicyID(record *RemoteAlertRecord) (uint, bool) {
	if record == nil {
		return 0, false
	}
	labels := parseRemoteAlertLabels(record)
	if !strings.EqualFold(strings.TrimSpace(labels["managed_by"]), "stx") {
		return 0, false
	}
	parsedID, err := strconv.ParseUint(strings.TrimSpace(labels["policy_id"]), 10, 32)
	if err != nil || parsedID == 0 {
		return 0, true
	}
	return uint(parsedID), true
}

func parseRemoteAlertLabels(record *RemoteAlertRecord) map[string]string {
	if record == nil {
		return map[string]string{}
	}
	result := map[string]string{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(record.LabelsJSON)), &result); err == nil && len(result) > 0 {
		return result
	}
	return map[string]string{}
}

func parseRemoteAlertAnnotations(record *RemoteAlertRecord) map[string]string {
	if record == nil {
		return map[string]string{}
	}
	result := map[string]string{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(record.AnnotationsJSON)), &result); err == nil && len(result) > 0 {
		return result
	}
	return map[string]string{}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func isGormRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
