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

package sync

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LeonYoah/stx/internal/config"
	dto "github.com/prometheus/client_model/go"
)

func TestJobHasEngineObservability(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spec JSONMap
		want bool
	}{
		{name: "nil", want: false},
		{name: "injected", spec: JSONMap{"observability": map[string]any{"injected": true, "reason": "injected"}}, want: true},
		{name: "user configured", spec: JSONMap{"observability": map[string]any{"injected": false, "reason": "user_configured"}}, want: true},
		{name: "disabled", spec: JSONMap{"observability": map[string]any{"injected": false, "reason": "disabled_by_task"}}, want: false},
		{name: "batch", spec: JSONMap{"observability": map[string]any{"reason": "batch_job"}}, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			job := &JobInstance{ID: 1, SubmitSpec: tc.spec}
			if got := jobHasEngineObservability(job); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestJobPrometheusExporter_observeAndDelete(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	// Use a fresh exporter to avoid cross-test pollution of the process-wide singleton.
	// 使用全新 exporter，避免污染进程级单例。
	exp := newJobPrometheusExporter()

	job := &JobInstance{
		ID:         42,
		TaskID:     7,
		Status:     JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": float64(3)},
	}
	exp.observeJobMetrics(job, map[string]interface{}{
		"SourceReceivedCount": float64(100),
		"SinkWriteCount":      float64(90),
		"SinkCommittedCount":  float64(80),
		"TableSourceReceivedCount": map[string]interface{}{
			"a.t1": float64(60),
			"a.t2": float64(40),
		},
	})
	exp.observeCDCDelay(job, 1.5)

	mfs, err := exp.reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	foundSource := false
	for _, mf := range mfs {
		if mf.GetName() == "stx_job_source_received_rows_total" {
			foundSource = true
			if sumCounter(mf) < 100 {
				t.Fatalf("expected source counter >= 100, got %v", sumCounter(mf))
			}
		}
	}
	if !foundSource {
		t.Fatalf("missing source counter metric")
	}

	exp.deleteJob(job)
	mfs, err = exp.reg.Gather()
	if err != nil {
		t.Fatalf("gather after delete: %v", err)
	}
	for _, mf := range mfs {
		if strings.HasPrefix(mf.GetName(), "stx_job_") && sumCounter(mf)+sumGauge(mf) != 0 && len(mf.GetMetric()) > 0 {
			// After delete, series for this job should be gone; other jobs may remain (none here).
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "job_id" && lp.GetValue() == "42" {
						t.Fatalf("job series still present on %s", mf.GetName())
					}
				}
			}
		}
	}
}

func TestServeJobPrometheusMetrics_disabled(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = false

	req := httptest.NewRequest(http.MethodGet, jobPrometheusMetricsPath, nil)
	rr := httptest.NewRecorder()
	ServeJobPrometheusMetrics(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), "observability is disabled") {
		t.Fatalf("unexpected body %q", body)
	}
}

func sumCounter(mf *dto.MetricFamily) float64 {
	var sum float64
	for _, m := range mf.GetMetric() {
		if m.Counter != nil {
			sum += m.Counter.GetValue()
		}
	}
	return sum
}

func sumGauge(mf *dto.MetricFamily) float64 {
	var sum float64
	for _, m := range mf.GetMetric() {
		if m.Gauge != nil {
			sum += m.Gauge.GetValue()
		}
	}
	return sum
}
