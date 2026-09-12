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
	"fmt"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

// GetAlertPolicyCenterBootstrap returns unified policy-center bootstrap payload.
// GetAlertPolicyCenterBootstrap 返回统一策略中心初始化数据。
func (s *Service) GetAlertPolicyCenterBootstrap(ctx context.Context) (*AlertPolicyCenterBootstrapData, error) {
	integrationStatus, err := s.GetIntegrationStatus(ctx)
	if err != nil {
		return nil, err
	}
	notifiableUsers, defaultReceiverUserIDs, err := s.resolveNotificationRecipientDirectory(ctx)
	if err != nil {
		return nil, err
	}

	componentMap := indexIntegrationComponents(integrationStatus.Components)
	metricsStatus, metricsReason := resolveMetricsPolicyCapability(componentMap)
	customPromQLStatus, customPromQLReason := resolveCustomPromQLCapability(componentMap)
	remoteIngestStatus, remoteIngestReason := resolveRemoteIngestCapability(componentMap)

	capabilities := []*AlertPolicyCapabilityDTO{
		{
			Key:     AlertPolicyCapabilityKeyPlatformHealth,
			Title:   "Platform Health Policies",
			Summary: "Use STX-managed runtime and cluster signals to detect health issues even without Prometheus.",
			Status:  AlertPolicyCapabilityStatusAvailable,
		},
		{
			Key:       AlertPolicyCapabilityKeyMetricsTemplates,
			Title:     "Metrics Templates",
			Summary:   "Enable CPU, memory, FD, failed-job, and other metrics-driven alert policies through Prometheus.",
			Status:    metricsStatus,
			Reason:    metricsReason,
			DependsOn: []string{"prometheus", "seatunnel_metrics"},
		},
		{
			Key:       AlertPolicyCapabilityKeyCustomPromQL,
			Title:     "Custom PromQL",
			Summary:   "Create productized metric policies while still allowing advanced teams to write custom PromQL rules.",
			Status:    customPromQLStatus,
			Reason:    customPromQLReason,
			DependsOn: []string{"prometheus", "seatunnel_metrics"},
		},
		{
			Key:       AlertPolicyCapabilityKeyRemoteIngest,
			Title:     "Remote Alert Ingest",
			Summary:   "Ingest Alertmanager webhook alerts into the unified alert center and notification pipeline.",
			Status:    remoteIngestStatus,
			Reason:    remoteIngestReason,
			DependsOn: []string{"alertmanager"},
		},
		{
			Key:     AlertPolicyCapabilityKeyWebhookNotification,
			Title:   "Webhook / IM Notifications",
			Summary: "Deliver alert notifications through webhook-compatible channels such as Webhook, WeCom, DingTalk, and Feishu.",
			Status:  AlertPolicyCapabilityStatusAvailable,
		},
		{
			Key:     AlertPolicyCapabilityKeyInAppNotification,
			Title:   "In-App Notification Center",
			Summary: "Provide a built-in notification inbox, receiver experience, and recovery follow-up inside STX.",
			Status:  AlertPolicyCapabilityStatusPlanned,
			Reason:  "Planned next step after the unified policy domain lands.",
		},
	}

	builders := []*AlertPolicyBuilderDTO{
		{
			Key:           AlertPolicyBuilderKindPlatformHealth,
			Title:         "Platform Health Policies",
			Description:   "Create unified policies for cluster availability, node liveness, process stability, and operation failures.",
			Status:        AlertPolicyCapabilityStatusAvailable,
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			Recommended:   true,
		},
		{
			Key:           AlertPolicyBuilderKindMetricsTemplate,
			Title:         "Metrics Templates",
			Description:   "Start from curated metric templates for CPU, memory, FD, failed jobs, and other Prometheus-backed signals.",
			Status:        metricsStatus,
			CapabilityKey: AlertPolicyCapabilityKeyMetricsTemplates,
			Recommended:   true,
		},
		{
			Key:           AlertPolicyBuilderKindCustomPromQL,
			Title:         "Custom PromQL",
			Description:   "Unlock advanced policies when Prometheus is healthy and metrics targets are reachable.",
			Status:        customPromQLStatus,
			CapabilityKey: AlertPolicyCapabilityKeyCustomPromQL,
			Recommended:   false,
		},
	}

	return &AlertPolicyCenterBootstrapData{
		GeneratedAt:            time.Now().UTC(),
		CapabilityMode:         "unified_capability_aware",
		Capabilities:           capabilities,
		Builders:               builders,
		Templates:              buildVisibleAlertPolicyTemplateSummaries(capabilities),
		Components:             cloneIntegrationComponents(integrationStatus.Components),
		NotifiableUsers:        notifiableUsers,
		DefaultReceiverUserIDs: defaultReceiverUserIDs,
	}, nil
}

func indexIntegrationComponents(components []*IntegrationComponentStatus) map[string]*IntegrationComponentStatus {
	result := make(map[string]*IntegrationComponentStatus, len(components))
	for _, component := range components {
		if component == nil {
			continue
		}
		result[strings.TrimSpace(strings.ToLower(component.Name))] = component
	}
	return result
}

func cloneIntegrationComponents(components []*IntegrationComponentStatus) []*IntegrationComponentStatus {
	if len(components) == 0 {
		return []*IntegrationComponentStatus{}
	}
	result := make([]*IntegrationComponentStatus, 0, len(components))
	for _, component := range components {
		if component == nil {
			continue
		}
		cloned := *component
		result = append(result, &cloned)
	}
	return result
}

func resolveMetricsPolicyCapability(componentMap map[string]*IntegrationComponentStatus) (AlertPolicyCapabilityStatus, string) {
	if !config.Config.Observability.Enabled {
		return AlertPolicyCapabilityStatusUnavailable, "Observability is disabled. Enable the Prometheus stack to unlock metric policies. Local platform-health alerts still work."
	}
	if reason := explainUnhealthyComponent(componentMap["prometheus"], "Prometheus"); reason != "" {
		return AlertPolicyCapabilityStatusUnavailable, reason
	}
	if reason := explainUnhealthyComponent(componentMap["seatunnel_metrics"], "SeaTunnel metrics targets"); reason != "" {
		return AlertPolicyCapabilityStatusUnavailable, reason
	}
	return AlertPolicyCapabilityStatusAvailable, ""
}

func resolveCustomPromQLCapability(componentMap map[string]*IntegrationComponentStatus) (AlertPolicyCapabilityStatus, string) {
	status, reason := resolveMetricsPolicyCapability(componentMap)
	if status != AlertPolicyCapabilityStatusAvailable {
		return status, reason
	}
	return AlertPolicyCapabilityStatusAvailable, ""
}

func resolveRemoteIngestCapability(componentMap map[string]*IntegrationComponentStatus) (AlertPolicyCapabilityStatus, string) {
	if !config.Config.Observability.Enabled {
		return AlertPolicyCapabilityStatusUnavailable, "Observability is disabled. Configure Alertmanager to enable remote alert ingest. Local platform-health alerts and direct notifications remain available."
	}
	if reason := explainUnhealthyComponent(componentMap["alertmanager"], "Alertmanager"); reason != "" {
		return AlertPolicyCapabilityStatusUnavailable, reason + " Local platform-health alerts and direct email/webhook notifications remain available."
	}
	return AlertPolicyCapabilityStatusAvailable, ""
}

func explainUnhealthyComponent(component *IntegrationComponentStatus, displayName string) string {
	if component == nil {
		return fmt.Sprintf("%s status is unavailable.", displayName)
	}
	if component.Healthy {
		return ""
	}
	if strings.TrimSpace(component.Error) != "" {
		return fmt.Sprintf("%s is not ready: %s", displayName, strings.TrimSpace(component.Error))
	}
	if component.StatusCode > 0 {
		return fmt.Sprintf("%s is not ready: HTTP %d.", displayName, component.StatusCode)
	}
	return fmt.Sprintf("%s is not ready.", displayName)
}

func buildVisibleAlertPolicyTemplateSummaries(capabilities []*AlertPolicyCapabilityDTO) []*AlertPolicyTemplateSummaryDTO {
	statusByCapability := make(map[string]AlertPolicyCapabilityStatus, len(capabilities))
	for _, capability := range capabilities {
		if capability == nil {
			continue
		}
		statusByCapability[strings.TrimSpace(capability.Key)] = capability.Status
	}

	result := make([]*AlertPolicyTemplateSummaryDTO, 0)
	for _, template := range allAlertPolicyTemplateSummaries() {
		if template == nil {
			continue
		}
		status, exists := statusByCapability[strings.TrimSpace(template.CapabilityKey)]
		if exists && status != AlertPolicyCapabilityStatusAvailable {
			continue
		}
		cloned := *template
		if len(template.RequiredSignals) > 0 {
			cloned.RequiredSignals = append([]string{}, template.RequiredSignals...)
		}
		result = append(result, &cloned)
	}
	return result
}

func allAlertPolicyTemplateSummaries() []*AlertPolicyTemplateSummaryDTO {
	return []*AlertPolicyTemplateSummaryDTO{
		{
			Key:           "master_unavailable",
			Name:          "Master unavailable",
			Description:   "Detect when the managed SeaTunnel cluster has no healthy master / coordinator left.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			Recommended:   true,
		},
		{
			Key:           "worker_insufficient",
			Name:          "Healthy workers below threshold",
			Description:   "Alert when the healthy worker count drops below the configured baseline for one cluster.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			Recommended:   true,
		},
		{
			Key:           AlertRuleKeyNodeOffline,
			Name:          "Node offline",
			Description:   "Detect node heartbeat or runtime visibility loss for a sustained duration.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			LegacyRuleKey: AlertRuleKeyNodeOffline,
			Recommended:   true,
		},
		{
			Key:           "agent_offline",
			Name:          "Agent offline",
			Description:   "Alert when the management plane loses the agent connection required for cluster operations.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			Recommended:   true,
		},
		{
			Key:           AlertRuleKeyProcessCrashed,
			Name:          "Process crashed",
			Description:   "Track repeated process crashes in the managed SeaTunnel runtime.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			LegacyRuleKey: AlertRuleKeyProcessCrashed,
			Recommended:   false,
		},
		{
			Key:           AlertRuleKeyProcessRestartFailed,
			Name:          "Process restart failed",
			Description:   "Alert when automatic restart can no longer recover a failed process.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			LegacyRuleKey: AlertRuleKeyProcessRestartFailed,
			Recommended:   true,
		},
		{
			Key:           AlertRuleKeyProcessRestartLimitReached,
			Name:          "Restart limit reached",
			Description:   "Detect crash-loop style behavior when the managed runtime exceeds restart limits.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			LegacyRuleKey: AlertRuleKeyProcessRestartLimitReached,
			Recommended:   true,
		},
		{
			Key:           AlertRuleKeyClusterRestartRequested,
			Name:          "Restart requested",
			Description:   "Send a notification when a managed cluster restart or node restart is triggered from the control plane.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			LegacyRuleKey: AlertRuleKeyClusterRestartRequested,
			Recommended:   false,
		},
		{
			Key:           AlertRuleKeyNodeStopRequested,
			Name:          "Node stop requested",
			Description:   "Send a notification immediately when one managed node is manually stopped from the control plane.",
			Category:      "platform_health",
			SourceKind:    string(AlertPolicyBuilderKindPlatformHealth),
			CapabilityKey: AlertPolicyCapabilityKeyPlatformHealth,
			LegacyRuleKey: AlertRuleKeyNodeStopRequested,
			Recommended:   false,
		},
		{
			Key:               "cpu_usage_high",
			Name:              "Process CPU load high",
			Description:       "Track sustained SeaTunnel process CPU load through process_cpu_seconds_total. This reflects average CPU core usage over the evaluation window, not host CPU percentage.",
			Category:          "metrics",
			SourceKind:        string(AlertPolicyBuilderKindMetricsTemplate),
			CapabilityKey:     AlertPolicyCapabilityKeyMetricsTemplates,
			MetricSource:      "seatunnel_telemetry",
			RequiredSignals:   []string{"process_cpu_seconds_total"},
			SuggestedPromQL:   "sum by (instance) (rate(process_cpu_seconds_total[5m])) > 0.8",
			DefaultOperator:   ">",
			DefaultThreshold:  "0.8",
			DefaultWindowMins: 5,
			Recommended:       true,
		},
		{
			Key:               "memory_usage_high",
			Name:              "Heap memory usage high",
			Description:       "Alert when the SeaTunnel JVM heap usage ratio stays above the threshold using jvm_memory_bytes_used and jvm_memory_bytes_max.",
			Category:          "metrics",
			SourceKind:        string(AlertPolicyBuilderKindMetricsTemplate),
			CapabilityKey:     AlertPolicyCapabilityKeyMetricsTemplates,
			MetricSource:      "seatunnel_telemetry",
			RequiredSignals:   []string{"jvm_memory_bytes_used", "jvm_memory_bytes_max"},
			SuggestedPromQL:   "max by (instance) (jvm_memory_bytes_used{area=\"heap\"} / clamp_min(jvm_memory_bytes_max{area=\"heap\"}, 1)) > 0.8",
			DefaultOperator:   ">",
			DefaultThreshold:  "0.8",
			DefaultWindowMins: 5,
			Recommended:       true,
		},
		{
			Key:               "fd_usage_high",
			Name:              "FD usage high",
			Description:       "Detect file descriptor exhaustion risk from process_open_fds and process_max_fds before the engine loses the ability to open files or sockets.",
			Category:          "metrics",
			SourceKind:        string(AlertPolicyBuilderKindMetricsTemplate),
			CapabilityKey:     AlertPolicyCapabilityKeyMetricsTemplates,
			MetricSource:      "seatunnel_telemetry",
			RequiredSignals:   []string{"process_open_fds", "process_max_fds"},
			SuggestedPromQL:   "max by (instance) (process_open_fds / clamp_min(process_max_fds, 1)) > 0.8",
			DefaultOperator:   ">",
			DefaultThreshold:  "0.8",
			DefaultWindowMins: 5,
			Recommended:       true,
		},
		{
			Key:               "failed_jobs_high",
			Name:              "Failed jobs high",
			Description:       "Track failed job volume using the job_count metric labelled with FAILED state.",
			Category:          "metrics",
			SourceKind:        string(AlertPolicyBuilderKindMetricsTemplate),
			CapabilityKey:     AlertPolicyCapabilityKeyMetricsTemplates,
			MetricSource:      "seatunnel_telemetry",
			RequiredSignals:   []string{"job_count"},
			SuggestedPromQL:   "sum by (instance) (job_count{state=\"FAILED\"}) > 0",
			DefaultOperator:   ">",
			DefaultThreshold:  "0",
			DefaultWindowMins: 1,
			Recommended:       true,
		},
		{
			Key:               "job_thread_pool_queue_backlog_high",
			Name:              "Job thread pool queue backlog high",
			Description:       "Detect job execution backlog using job_thread_pool_queueTaskCount before queue growth turns into latency or starvation.",
			Category:          "metrics",
			SourceKind:        string(AlertPolicyBuilderKindMetricsTemplate),
			CapabilityKey:     AlertPolicyCapabilityKeyMetricsTemplates,
			MetricSource:      "seatunnel_telemetry",
			RequiredSignals:   []string{"job_thread_pool_queueTaskCount"},
			SuggestedPromQL:   "max by (instance) (job_thread_pool_queueTaskCount) > 100",
			DefaultOperator:   ">",
			DefaultThreshold:  "100",
			DefaultWindowMins: 5,
			Recommended:       true,
		},
		{
			Key:               "job_thread_pool_rejection_high",
			Name:              "Job thread pool rejections detected",
			Description:       "Alert when job_thread_pool_rejection_total increases, indicating work is being rejected instead of queued or executed.",
			Category:          "metrics",
			SourceKind:        string(AlertPolicyBuilderKindMetricsTemplate),
			CapabilityKey:     AlertPolicyCapabilityKeyMetricsTemplates,
			MetricSource:      "seatunnel_telemetry",
			RequiredSignals:   []string{"job_thread_pool_rejection_total"},
			SuggestedPromQL:   "sum by (instance) (increase(job_thread_pool_rejection_total[5m])) > 0",
			DefaultOperator:   ">",
			DefaultThreshold:  "0",
			DefaultWindowMins: 5,
			Recommended:       true,
		},
		{
			Key:               "deadlocked_threads_detected",
			Name:              "Deadlocked threads detected",
			Description:       "Use jvm_threads_deadlocked to detect JVM deadlocks that can stall task execution and recovery.",
			Category:          "metrics",
			SourceKind:        string(AlertPolicyBuilderKindMetricsTemplate),
			CapabilityKey:     AlertPolicyCapabilityKeyMetricsTemplates,
			MetricSource:      "seatunnel_telemetry",
			RequiredSignals:   []string{"jvm_threads_deadlocked"},
			SuggestedPromQL:   "max by (instance) (jvm_threads_deadlocked) > 0",
			DefaultOperator:   ">",
			DefaultThreshold:  "0",
			DefaultWindowMins: 1,
			Recommended:       true,
		},
		{
			Key:               "split_brain_risk",
			Name:              "Cluster not safe / split-brain risk",
			Description:       "Detect Hazelcast partition safety issues through hazelcast_partition_isClusterSafe and raise an alert before split-brain risk expands.",
			Category:          "metrics",
			SourceKind:        string(AlertPolicyBuilderKindMetricsTemplate),
			CapabilityKey:     AlertPolicyCapabilityKeyMetricsTemplates,
			MetricSource:      "seatunnel_telemetry",
			RequiredSignals:   []string{"hazelcast_partition_isClusterSafe"},
			SuggestedPromQL:   "min by (instance) (hazelcast_partition_isClusterSafe) < 1",
			DefaultOperator:   "<",
			DefaultThreshold:  "1",
			DefaultWindowMins: 1,
			Recommended:       true,
		},
	}
}
