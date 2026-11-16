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

package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
)

func TestParseCLIWaitTimeout(t *testing.T) {
	got, err := parseCLIWaitTimeout("")
	if err != nil || got != 0 {
		t.Fatalf("empty timeout should wait forever: duration=%s err=%v", got, err)
	}
	got, err = parseCLIWaitTimeout("   ")
	if err != nil || got != 0 {
		t.Fatalf("blank timeout should wait forever: duration=%s err=%v", got, err)
	}
	got, err = parseCLIWaitTimeout("6h")
	if err != nil || got != 6*time.Hour {
		t.Fatalf("6h = %s err=%v", got, err)
	}
	if _, err := parseCLIWaitTimeout("0"); err == nil {
		t.Fatal("zero duration must be rejected")
	}
	if _, err := parseCLIWaitTimeout("nope"); err == nil {
		t.Fatal("invalid duration must be rejected")
	}
}

func TestStaleObservationExceededUsesSyncLagMs(t *testing.T) {
	lag := int64(4 * 60 * 1000)
	job := syncJobWaitResult{SyncState: "stale", SyncLagMs: &lag}
	if !staleObservationExceeded(job, 3*time.Minute, nil) {
		t.Fatal("expected stale observation to exceed 3m when sync_lag_ms is 4m")
	}
}

func TestStaleObservationExceededResetsWhenHealthy(t *testing.T) {
	since := time.Now().Add(-10 * time.Minute)
	job := syncJobWaitResult{SyncState: "ok"}
	if staleObservationExceeded(job, 3*time.Minute, &since) {
		t.Fatal("ok sync_state should not trigger observation timeout")
	}
	if !since.IsZero() {
		t.Fatal("staleSince should reset when sync_state is not stale")
	}
}

func TestNormalizeCLILogNewlines(t *testing.T) {
	got := normalizeCLILogNewlines(`a\nb`)
	if got != "a\nb" {
		t.Fatalf("got %q", got)
	}
	lines := splitCLILogLines("a\nb\n")
	if len(lines) != 2 || lines[0] != "a" || lines[1] != "b" {
		t.Fatalf("lines=%v", lines)
	}
}

func TestFormatCLIJobMetricsBlock(t *testing.T) {
	block := formatCLIJobMetricsBlock(map[string]interface{}{
		"metrics": map[string]interface{}{
			"SourceReceivedCount": float64(9),
			"SinkWriteCount":      float64(9),
		},
	}, time.Unix(0, 0).UTC(), true)
	if !strings.Contains(block, "STX metrics final") {
		t.Fatalf("missing final title: %s", block)
	}
	if !strings.Contains(block, "\n") {
		t.Fatal("metrics block must include real newlines")
	}
}

func TestMaybeEmitJobMetricsLogSkipsDuplicateBlock(t *testing.T) {
	var buf bytes.Buffer
	events := clioutput.NewEventWriter(&buf)
	job := syncJobWaitResult{
		ID:     1,
		Status: "success",
		ResultPreview: map[string]interface{}{
			"metrics": map[string]interface{}{
				"SourceReceivedCount": float64(9),
				"SinkWriteCount":      float64(9),
			},
		},
	}
	first, block := maybeEmitJobMetricsLog(events, "op", "req", job, time.Time{}, "", true)
	if first.IsZero() || strings.TrimSpace(block) == "" {
		t.Fatal("expected the first metrics block to emit")
	}
	_, _ = maybeEmitJobMetricsLog(events, "op", "req", job, first, block, true)
	if strings.Count(buf.String(), `"event":"job_metrics"`) != 1 {
		t.Fatalf("duplicate metrics emitted: %s", buf.String())
	}
}

func TestPreviewCollectURL(t *testing.T) {
	t.Setenv("STX_SERVER", "")
	got := previewCollectURL("https://stx.example:8443/")
	if got != "https://stx.example:8443/api/v1/sync/preview/collect" {
		t.Fatalf("got %q", got)
	}
	got = previewCollectURL("")
	if got != "http://127.0.0.1:17800/api/v1/sync/preview/collect" {
		t.Fatalf("default got %q", got)
	}
	t.Setenv("STX_SERVER", "https://from-env.example/")
	got = previewCollectURL("")
	if got != "https://from-env.example/api/v1/sync/preview/collect" {
		t.Fatalf("env got %q", got)
	}
}

func TestApplyPreviewCollectURLUsesClientServerAndKeepsCustom(t *testing.T) {
	t.Setenv("STX_SERVER", "https://should-not-win.example")
	definition := map[string]any{
		"preview_http_sink": map[string]any{"array_mode": false},
	}
	applyPreviewCollectURL(definition, "https://saved-ns.example:8443")
	sink := definition["preview_http_sink"].(map[string]any)
	if sink["url"] != "https://saved-ns.example:8443/api/v1/sync/preview/collect" {
		t.Fatalf("namespace server got %v", sink["url"])
	}

	custom := map[string]any{
		"preview_http_sink": map[string]any{"url": "https://custom.example/collect", "array_mode": true},
	}
	applyPreviewCollectURL(custom, "https://saved-ns.example:8443")
	customSink := custom["preview_http_sink"].(map[string]any)
	if customSink["url"] != "https://custom.example/collect" {
		t.Fatalf("custom url overwritten: %v", customSink["url"])
	}
}
