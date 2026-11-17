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

	if err := validateConfig(c); err == nil {
		t.Fatalf("expected validation error when app.external_url is empty")
	}
}

// fillEmbeddedObservabilityDefaults 补齐内嵌 tsdb 的必填字段（与 setDefaults 的产物一致），
// 让直造 configModel 的校验测试不用逐个抄默认值。
// fillEmbeddedObservabilityDefaults fills the embedded-tsdb required fields (mirroring what
// setDefaults produces) so validation tests that build configModel directly need not repeat them.
func fillEmbeddedObservabilityDefaults(c *configModel) {
	c.Observability.Storage.Dir = "data/tsdb"
	c.Observability.Storage.Retention = "31d"
	c.Observability.ScrapeInterval = "15s"
	c.Observability.EvalInterval = "15s"
	c.Observability.QueryTimeout = "8s"
	c.Observability.MaxSamples = 50000000
}

func TestValidateConfig_RemoteObservabilityHappyPath(t *testing.T) {
	c := &configModel{}
	c.App.ExternalURL = "https://stx.example.com"
	c.Observability.Enabled = true
	fillEmbeddedObservabilityDefaults(c)

	if err := validateConfig(c); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

// TestValidateConfig_EmbeddedObservabilityInvalid 覆盖内嵌存储与查询参数的校验失败路径。
// TestValidateConfig_EmbeddedObservabilityInvalid covers the embedded storage/query validation failures.
func TestValidateConfig_EmbeddedObservabilityInvalid(t *testing.T) {
	newValid := func() *configModel {
		c := &configModel{}
		c.App.ExternalURL = "https://stx.example.com"
		c.Observability.Enabled = true
		fillEmbeddedObservabilityDefaults(c)
		return c
	}

	cases := []struct {
		name   string
		mutate func(c *configModel)
	}{
		{"empty storage dir", func(c *configModel) { c.Observability.Storage.Dir = " " }},
		{"bad retention", func(c *configModel) { c.Observability.Storage.Retention = "abc" }},
		{"zero retention", func(c *configModel) { c.Observability.Storage.Retention = "0" }},
		{"negative max_size", func(c *configModel) { c.Observability.Storage.MaxSize = -1 }},
		{"bad scrape interval", func(c *configModel) { c.Observability.ScrapeInterval = "x" }},
		{"zero eval interval", func(c *configModel) { c.Observability.EvalInterval = "0s" }},
		{"bad query timeout", func(c *configModel) { c.Observability.QueryTimeout = "soon" }},
		{"negative max_samples", func(c *configModel) { c.Observability.MaxSamples = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newValid()
			tc.mutate(c)
			if err := validateConfig(c); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}
