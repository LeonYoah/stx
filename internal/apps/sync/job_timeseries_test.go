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
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

func TestGetJobTimeseries_observabilityDisabled(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = false

	svc := &Service{}
	job := &JobInstance{ID: 9, TaskID: 1, Status: JobStatusRunning, SubmitSpec: JSONMap{"cluster_id": float64(1)}}
	out, err := svc.getJobTimeseries(context.Background(), job, "1h", nil, "throughput", 0)
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

func TestTimeseriesPromQL_checkpointIgnoresEngineObs(t *testing.T) {
	t.Parallel()
	// Checkpoint PromQL must stay available even when engine observability is off.
	// Checkpoint PromQL 在引擎可观测性关闭时仍应可用。
	if got := timeseriesPromQL("checkpoint", "1", false); len(got) == 0 {
		t.Fatalf("expected checkpoint templates when engine obs off")
	}
	if got := timeseriesPromQL("backpressure", "1", false); got != nil {
		t.Fatalf("expected nil backpressure when engine obs off, got %#v", got)
	}
	if got := timeseriesPromQL("backpressure", "1", true); len(got) == 0 {
		t.Fatalf("expected backpressure templates when engine obs on")
	}
}

// TestParseJobTimeseriesWindow 覆盖 start/end 的解析与校验：RFC3339、秒/毫秒时间戳、缺省 end、非法区间。
// TestParseJobTimeseriesWindow covers start/end parsing and validation: RFC3339, second/millisecond timestamps, default
// end, and invalid windows.
func TestParseJobTimeseriesWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 14, 13, 0, 0, 0, time.UTC)

	if w, err := ParseJobTimeseriesWindow("", "", now); err != nil || w != nil {
		t.Fatalf("empty: w=%v err=%v", w, err)
	}

	w, err := ParseJobTimeseriesWindow("2026-11-14T20:13:00+08:00", "1794658708000", now)
	if err != nil {
		t.Fatalf("rfc3339 + ms: %v", err)
	}
	if !w.Start.Equal(time.Date(2026, 11, 14, 12, 13, 0, 0, time.UTC)) || w.End.UnixMilli() != 1794658708000 {
		t.Fatalf("got %+v", w)
	}

	w, err = ParseJobTimeseriesWindow("1794657000", "", now)
	if err != nil || w.Start.Unix() != 1794657000 || !w.End.Equal(now) {
		t.Fatalf("seconds + default end: w=%+v err=%v", w, err)
	}

	for name, tc := range map[string][2]string{
		"end only":     {"", "2026-11-14T12:00:00Z"},
		"bad start":    {"yesterday", ""},
		"end <= start": {"2026-11-14T12:00:00Z", "2026-11-14T12:00:00Z"},
		"too long":     {"2026-09-01T00:00:00Z", "2026-11-14T00:00:00Z"},
	} {
		if _, err := ParseJobTimeseriesWindow(tc[0], tc[1], now); err == nil {
			t.Fatalf("%s: want error", name)
		}
	}
}

// TestAutoTimeseriesStep 确认自定义时间窗的 step 让点数落在预算内，且不会细于 10 秒。
// TestAutoTimeseriesStep checks custom-window steps keep the point count within budget and never go below 10s.
func TestAutoTimeseriesStep(t *testing.T) {
	t.Parallel()
	cases := map[time.Duration]int{
		25 * time.Minute:    10,
		2 * time.Hour:       30,
		24 * time.Hour:      600,
		31 * 24 * time.Hour: 14400,
	}
	for span, want := range cases {
		if got := autoTimeseriesStep(span); got != want {
			t.Fatalf("span %s: step %d, want %d", span, got, want)
		}
	}
}

// TestGetJobTimeseries_customWindow 确认传入时间窗时响应带 custom range、起止毫秒与自动 step。
// TestGetJobTimeseries_customWindow checks a window yields range=custom, start/end millis, and an auto step.
func TestGetJobTimeseries_customWindow(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = false

	svc := &Service{}
	job := &JobInstance{ID: 9, TaskID: 1, Status: JobStatusFailed, SubmitSpec: JSONMap{"cluster_id": float64(1)}}
	start := time.Date(2026, 11, 14, 12, 13, 0, 0, time.UTC)
	end := start.Add(25 * time.Minute)
	out, err := svc.getJobTimeseries(context.Background(), job, "1h", &JobTimeseriesWindow{Start: start, End: end}, "throughput", 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.Range != "custom" || out.StartMs != start.UnixMilli() || out.EndMs != end.UnixMilli() || out.StepSeconds != 10 {
		t.Fatalf("got %+v", out)
	}
}
