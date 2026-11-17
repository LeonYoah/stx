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
 * See the License for the LICENSE file for more details.
 */

package monitoring

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

func TestService_QueryInstant_pointLookup(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() {
		config.Config.Observability.Enabled = originalEnabled
		_ = embedts.Close()
	})
	config.Config.Observability.Enabled = true
	_ = embedts.Close()

	dir := t.TempDir()
	ctx := context.Background()
	if err := embedts.Start(ctx, embedts.Options{Dir: dir, MaxSamples: 10000, QueryTimeout: 5 * time.Second}); err != nil {
		t.Fatalf("Start embedts: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	app := embedts.Appender(ctx)
	if app == nil {
		t.Fatal("nil appender")
	}
	lset := labels.FromStrings(labels.MetricName, embedts.LoadtestMetric, "job", "loadtest", "series", "0")
	if _, err := app.Append(0, lset, now.UnixMilli(), 1.5); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := app.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	service := &Service{}
	data, err := service.QueryInstant(ctx, `stx_loadtest_gauge{series="0"}`, now)
	if err != nil {
		t.Fatalf("QueryInstant: %v", err)
	}
	if data.SampleCount != 1 {
		t.Fatalf("sample_count=%d want 1 elapsed=%.2f", data.SampleCount, data.ElapsedMs)
	}
	if data.Samples[0].Value != 1.5 {
		t.Fatalf("value=%v want 1.5", data.Samples[0].Value)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"ts_ms":`, `"value":`, `"elapsed_ms":`, `"sample_count":`} {
		if !strings.Contains(text, want) {
			t.Fatalf("json missing %s: %s", want, text)
		}
	}

	if _, err := service.QueryInstant(ctx, "", now); err == nil {
		t.Fatal("empty query should fail")
	}
}

func TestService_QueryRange_emptyExpr(t *testing.T) {
	service := &Service{}
	if _, err := service.QueryRange(context.Background(), "  ", "", "", 0); err == nil {
		t.Fatal("empty query should fail")
	}
}
