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

package config

import "testing"

func TestValidateConfig_ObservabilityDisabled(t *testing.T) {
	c := &configModel{}
	c.Observability.Enabled = false
	if err := validateConfig(c); err != nil {
		t.Fatalf("expected nil error when observability is disabled, got: %v", err)
	}
}

func TestValidateConfig_MissingExternalURL(t *testing.T) {
	c := &configModel{}
	c.Observability.Enabled = true
	c.Observability.Prometheus.URL = "http://127.0.0.1:9090"
	c.Observability.Alertmanager.URL = "http://127.0.0.1:9093"
	c.Observability.Grafana.URL = "http://127.0.0.1:3000"
	c.Observability.Prometheus.HTTPSDPath = "/api/v1/monitoring/prometheus/discovery"
	c.Observability.Alertmanager.WebhookPath = "/api/v1/monitoring/alertmanager/webhook"

	if err := validateConfig(c); err == nil {
		t.Fatalf("expected validation error when app.external_url is empty")
	}
}

func TestValidateConfig_RemoteObservabilityHappyPath(t *testing.T) {
	c := &configModel{}
	c.App.ExternalURL = "https://stx.example.com"
	c.Observability.Enabled = true
	c.Observability.Prometheus.URL = "http://127.0.0.1:9090"
	c.Observability.Alertmanager.URL = "http://127.0.0.1:9093"
	c.Observability.Grafana.URL = "http://127.0.0.1:3000"
	c.Observability.Prometheus.HTTPSDPath = "/api/v1/monitoring/prometheus/discovery"
	c.Observability.Alertmanager.WebhookPath = "/api/v1/monitoring/alertmanager/webhook"

	if err := validateConfig(c); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}
