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
	"strings"
	"testing"
	"time"
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
