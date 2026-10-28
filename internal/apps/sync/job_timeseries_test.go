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
	"context"
	"testing"

	"github.com/LeonYoah/stx/internal/config"
)

func TestGetJobTimeseries_observabilityDisabled(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = false

	svc := &Service{}
	job := &JobInstance{ID: 9, TaskID: 1, Status: JobStatusRunning, SubmitSpec: JSONMap{"cluster_id": float64(1)}}
	out, err := svc.getJobTimeseries(context.Background(), job, "1h", "throughput", 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.Source != "none" || out.Reason != "observability_disabled" {
		t.Fatalf("got %+v", out)
	}
	if out.StepSeconds != 15 {
		t.Fatalf("step=%d", out.StepSeconds)
	}
	if out.Series == nil || out.Annotations == nil {
		t.Fatalf("series/annotations must be non-nil empty slices")
	}
}

func TestParseTimeseriesMetrics(t *testing.T) {
	t.Parallel()
	got := parseTimeseriesMetrics("throughput, rows, throughput")
	if len(got) != 2 || got[0] != "throughput" || got[1] != "rows" {
		t.Fatalf("got %#v", got)
	}
	def := parseTimeseriesMetrics("")
	if len(def) < 4 {
		t.Fatalf("default metrics too short: %#v", def)
	}
}

func TestTimeseriesPromQL_busyGated(t *testing.T) {
	t.Parallel()
	if got := timeseriesPromQL("busy", "1", false); got != nil {
		t.Fatalf("expected nil when engine obs off, got %#v", got)
	}
	if got := timeseriesPromQL("busy", "1", true); len(got) == 0 {
		t.Fatalf("expected templates when engine obs on")
	}
}
