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
	"strings"
	"testing"

	"github.com/LeonYoah/stx/internal/config"
)

func TestResolveMetricsPolicyCapability(t *testing.T) {
	original := config.Config.Observability.Enabled
	t.Cleanup(func() {
		config.Config.Observability.Enabled = original
	})

	t.Run("disabled when observability is off", func(t *testing.T) {
		config.Config.Observability.Enabled = false

		status, reason := resolveMetricsPolicyCapability(map[string]*IntegrationComponentStatus{})
		if status != AlertPolicyCapabilityStatusUnavailable {
			t.Fatalf("expected unavailable, got %s", status)
		}
		if !strings.Contains(reason, "Observability is disabled") {
			t.Fatalf("unexpected reason: %s", reason)
		}
	})

	t.Run("available when prometheus and metrics are healthy", func(t *testing.T) {
		config.Config.Observability.Enabled = true

		status, reason := resolveMetricsPolicyCapability(map[string]*IntegrationComponentStatus{
			"prometheus":        {Name: "prometheus", Healthy: true},
			"seatunnel_metrics": {Name: "seatunnel_metrics", Healthy: true},
		})
		if status != AlertPolicyCapabilityStatusAvailable {
			t.Fatalf("expected available, got %s", status)
		}
		if reason != "" {
			t.Fatalf("expected empty reason, got %s", reason)
		}
	})

	t.Run("unavailable when metrics target is unhealthy", func(t *testing.T) {
		config.Config.Observability.Enabled = true

		status, reason := resolveMetricsPolicyCapability(map[string]*IntegrationComponentStatus{
			"prometheus":        {Name: "prometheus", Healthy: true},
			"seatunnel_metrics": {Name: "seatunnel_metrics", Healthy: false, Error: "partial healthy: 1/3 targets expose metrics"},
		})
		if status != AlertPolicyCapabilityStatusUnavailable {
			t.Fatalf("expected unavailable, got %s", status)
		}
		if !strings.Contains(reason, "SeaTunnel metrics targets is not ready") {
			t.Fatalf("unexpected reason: %s", reason)
		}
	})
}

func TestResolveRemoteIngestCapability(t *testing.T) {
	original := config.Config.Observability.Enabled
	t.Cleanup(func() {
		config.Config.Observability.Enabled = original
	})

	config.Config.Observability.Enabled = true
	status, reason := resolveRemoteIngestCapability(map[string]*IntegrationComponentStatus{
		"alertmanager": {Name: "alertmanager", Healthy: false, StatusCode: 503},
	})
	if status != AlertPolicyCapabilityStatusUnavailable {
		t.Fatalf("expected unavailable, got %s", status)
	}
	if !strings.Contains(reason, "Alertmanager is not ready") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestBuildVisibleAlertPolicyTemplateSummaries_filtersUnavailableMetricTemplates(t *testing.T) {
	templates := buildVisibleAlertPolicyTemplateSummaries([]*AlertPolicyCapabilityDTO{
		{Key: AlertPolicyCapabilityKeyPlatformHealth, Status: AlertPolicyCapabilityStatusAvailable},
		{Key: AlertPolicyCapabilityKeyMetricsTemplates, Status: AlertPolicyCapabilityStatusUnavailable},
	})

	for _, template := range templates {
		if template == nil {
			continue
		}
		if template.SourceKind == string(AlertPolicyBuilderKindMetricsTemplate) {
			t.Fatalf("expected unavailable metric template to be filtered, got %+v", template)
		}
	}
}

func TestBuildVisibleAlertPolicyTemplateSummaries_includesTelemetryTemplatesWhenMetricsAvailable(t *testing.T) {
	templates := buildVisibleAlertPolicyTemplateSummaries([]*AlertPolicyCapabilityDTO{
		{Key: AlertPolicyCapabilityKeyPlatformHealth, Status: AlertPolicyCapabilityStatusAvailable},
		{Key: AlertPolicyCapabilityKeyMetricsTemplates, Status: AlertPolicyCapabilityStatusAvailable},
	})

	found := map[string]bool{}
	for _, template := range templates {
		if template == nil {
			continue
		}
		found[template.Key] = true
	}

	expectedKeys := []string{
		"cpu_usage_high",
		"memory_usage_high",
		"fd_usage_high",
		"failed_jobs_high",
		"job_thread_pool_queue_backlog_high",
		"job_thread_pool_rejection_high",
		"deadlocked_threads_detected",
		"split_brain_risk",
	}
	for _, key := range expectedKeys {
		if !found[key] {
			t.Fatalf("expected template %s to be visible", key)
		}
	}
}
