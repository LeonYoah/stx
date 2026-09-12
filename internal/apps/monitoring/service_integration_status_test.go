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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/config"
)

func TestService_decorateManagedMetricsTargetsFromPrometheus_marksHealthyTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/targets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"activeTargets":[{"scrapeUrl":"http://38.55.133.202:5801/hazelcast/rest/instance/metrics","health":"up","lastError":"","labels":{"job":"seatunnel_engine_http","instance":"38.55.133.202:5801"},"discoveredLabels":{"__address__":"38.55.133.202:5801","__metrics_path__":"/hazelcast/rest/instance/metrics","job":"seatunnel_engine_http"}}]}}`))
	}))
	defer server.Close()

	originalPrometheusURL := config.Config.Observability.Prometheus.URL
	originalMetricsPath := config.Config.Observability.SeatunnelMetric.Path
	t.Cleanup(func() {
		config.Config.Observability.Prometheus.URL = originalPrometheusURL
		config.Config.Observability.SeatunnelMetric.Path = originalMetricsPath
	})

	config.Config.Observability.Prometheus.URL = server.URL
	config.Config.Observability.SeatunnelMetric.Path = "/hazelcast/rest/instance/metrics"

	service := &Service{}
	targets := []*managedMetricsTarget{{
		Target:   "38.55.133.202:5801",
		ProbeURL: "http://38.55.133.202:5801/hazelcast/rest/instance/metrics",
	}}

	if err := service.decorateManagedMetricsTargetsFromPrometheus(context.Background(), targets); err != nil {
		t.Fatalf("decorateManagedMetricsTargetsFromPrometheus returned error: %v", err)
	}

	if !targets[0].Healthy {
		t.Fatal("expected managed target to be marked healthy")
	}
	if targets[0].StatusCode != http.StatusOK {
		t.Fatalf("expected http 200 status code, got %d", targets[0].StatusCode)
	}
	if targets[0].ProbeError != "" {
		t.Fatalf("expected empty probe error, got %q", targets[0].ProbeError)
	}
}

func TestService_decorateManagedMetricsTargetsFromPrometheus_reportsUndiscoveredTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"activeTargets":[{"scrapeUrl":"http://38.55.133.202:5801/hazelcast/rest/instance/metrics","health":"up","lastError":"","labels":{"job":"seatunnel_engine_http","instance":"38.55.133.202:5801"},"discoveredLabels":{"__address__":"38.55.133.202:5801","__metrics_path__":"/hazelcast/rest/instance/metrics","job":"seatunnel_engine_http"}}]}}`))
	}))
	defer server.Close()

	originalPrometheusURL := config.Config.Observability.Prometheus.URL
	originalMetricsPath := config.Config.Observability.SeatunnelMetric.Path
	t.Cleanup(func() {
		config.Config.Observability.Prometheus.URL = originalPrometheusURL
		config.Config.Observability.SeatunnelMetric.Path = originalMetricsPath
	})

	config.Config.Observability.Prometheus.URL = server.URL
	config.Config.Observability.SeatunnelMetric.Path = "/hazelcast/rest/instance/metrics"

	service := &Service{}
	targets := []*managedMetricsTarget{{
		Target:   "38.55.133.202:5802",
		ProbeURL: "http://38.55.133.202:5802/hazelcast/rest/instance/metrics",
	}}

	if err := service.decorateManagedMetricsTargetsFromPrometheus(context.Background(), targets); err != nil {
		t.Fatalf("decorateManagedMetricsTargetsFromPrometheus returned error: %v", err)
	}

	if targets[0].Healthy {
		t.Fatal("expected undiscovered target to remain unhealthy")
	}
	if targets[0].ProbeError == "" {
		t.Fatal("expected undiscovered target to expose a probe error")
	}
}
