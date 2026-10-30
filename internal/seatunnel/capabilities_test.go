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

package seatunnel

import "testing"

func TestCapabilitiesForVersion(t *testing.T) {
	t.Run("2.3.2 has no history or schedule cleanup settings", func(t *testing.T) {
		capabilities := CapabilitiesForVersion("2.3.2")
		if !capabilities.SupportsDynamicSlot || !capabilities.SupportsSlotNum {
			t.Fatalf("expected 2.3.2 to support slot settings, got %+v", capabilities)
		}
		if capabilities.SupportsHistoryJobExpireMinutes {
			t.Fatalf("expected 2.3.2 not to support history_job_expire_minutes, got %+v", capabilities)
		}
		if capabilities.SupportsScheduledDeletionEnable || capabilities.SupportsJobScheduleStrategy {
			t.Fatalf("expected 2.3.2 not to support scheduled deletion or job schedule strategy, got %+v", capabilities)
		}
		if capabilities.SupportsSlotAllocationStrategy {
			t.Fatalf("expected 2.3.2 not to support slot allocation strategy, got %+v", capabilities)
		}
		if capabilities.SupportsHTTPService || capabilities.SupportsJobLogMode {
			t.Fatalf("expected 2.3.2 not to support http service or job log mode, got %+v", capabilities)
		}
	})

	t.Run("2.3.3 supports historical retention only", func(t *testing.T) {
		capabilities := CapabilitiesForVersion("2.3.3")
		if !capabilities.SupportsHistoryJobExpireMinutes {
			t.Fatalf("expected 2.3.3 to support history_job_expire_minutes, got %+v", capabilities)
		}
		if capabilities.SupportsScheduledDeletionEnable || capabilities.SupportsJobScheduleStrategy {
			t.Fatalf("expected 2.3.3 not to support scheduled deletion or job schedule strategy, got %+v", capabilities)
		}
		if capabilities.SupportsSlotAllocationStrategy {
			t.Fatalf("expected 2.3.3 not to support slot allocation strategy, got %+v", capabilities)
		}
	})

	t.Run("2.3.9 supports retention cleanup and job schedule strategy", func(t *testing.T) {
		capabilities := CapabilitiesForVersion("2.3.9")
		if !capabilities.SupportsHistoryJobExpireMinutes || !capabilities.SupportsScheduledDeletionEnable || !capabilities.SupportsJobScheduleStrategy {
			t.Fatalf("expected 2.3.9 to support advanced retention settings, got %+v", capabilities)
		}
		if !capabilities.SupportsHTTPService || !capabilities.SupportsJobLogMode {
			t.Fatalf("expected 2.3.9 to support http service and job log mode, got %+v", capabilities)
		}
		if capabilities.DefaultJobLogMode != "per_job" {
			t.Fatalf("expected 2.3.9 default job log mode per_job, got %+v", capabilities)
		}
		if capabilities.SupportsSlotAllocationStrategy {
			t.Fatalf("expected 2.3.9 not to support slot allocation strategy yet, got %+v", capabilities)
		}
	})

	t.Run("2.3.8 supports per-job log mode but not http service", func(t *testing.T) {
		capabilities := CapabilitiesForVersion("2.3.8")
		if !capabilities.SupportsJobLogMode {
			t.Fatalf("expected 2.3.8 to support job log mode, got %+v", capabilities)
		}
		if capabilities.DefaultJobLogMode != "per_job" {
			t.Fatalf("expected 2.3.8 default job log mode per_job, got %+v", capabilities)
		}
		if capabilities.SupportsHTTPService {
			t.Fatalf("expected 2.3.8 not to support http service, got %+v", capabilities)
		}
	})

	t.Run("below 2.3.8 keeps mixed default job log mode", func(t *testing.T) {
		capabilities := CapabilitiesForVersion("2.3.7")
		if capabilities.SupportsJobLogMode {
			t.Fatalf("expected 2.3.7 not to support job log mode, got %+v", capabilities)
		}
		if capabilities.DefaultJobLogMode != "mixed" {
			t.Fatalf("expected 2.3.7 default job log mode mixed, got %+v", capabilities)
		}
	})

	t.Run("2.3.10 supports slot allocation strategy", func(t *testing.T) {
		capabilities := CapabilitiesForVersion("2.3.10")
		if !capabilities.SupportsSlotAllocationStrategy {
			t.Fatalf("expected 2.3.10 to support slot allocation strategy, got %+v", capabilities)
		}
		if capabilities.DefaultSlotAllocationStrategy != "RANDOM" {
			t.Fatalf("expected default slot allocation strategy RANDOM, got %+v", capabilities)
		}
	})
}

func TestCompareVersions(t *testing.T) {
	testCases := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{name: "equal", v1: "2.3.9", v2: "2.3.9", want: 0},
		{name: "greater", v1: "2.3.10", v2: "2.3.9", want: 1},
		{name: "less", v1: "2.3.8", v2: "2.3.9", want: -1},
		{name: "suffix", v1: "2.2.0", v2: "2.2.0-beta", want: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := CompareVersions(testCase.v1, testCase.v2)
			if testCase.want == 0 && got != 0 {
				t.Fatalf("CompareVersions(%q, %q) = %d, want 0", testCase.v1, testCase.v2, got)
			}
			if testCase.want > 0 && got <= 0 {
				t.Fatalf("CompareVersions(%q, %q) = %d, want > 0", testCase.v1, testCase.v2, got)
			}
			if testCase.want < 0 && got >= 0 {
				t.Fatalf("CompareVersions(%q, %q) = %d, want < 0", testCase.v1, testCase.v2, got)
			}
		})
	}
}

// TestRealtimeAndEventCapabilities 校验实时可观测性与作业状态事件的版本门槛。
// TestRealtimeAndEventCapabilities verifies version gates for realtime observability and job state events.
func TestRealtimeAndEventCapabilities(t *testing.T) {
	cases := []struct {
		version     string
		realtime    bool
		stateEvents bool
		nonTerminal bool
	}{
		{version: "", realtime: false, stateEvents: false, nonTerminal: false},
		{version: "2.3.11", realtime: false, stateEvents: false, nonTerminal: false},
		{version: "2.3.12", realtime: false, stateEvents: true, nonTerminal: false},
		{version: "3.0.0-SNAPSHOT", realtime: false, stateEvents: true, nonTerminal: false},
		{version: "3.0.0", realtime: true, stateEvents: true, nonTerminal: true},
		{version: "3.0.1-SNAPSHOT", realtime: true, stateEvents: true, nonTerminal: true},
		{version: "3.1.0", realtime: true, stateEvents: true, nonTerminal: true},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			if got := SupportsRealtimeObservability(tc.version); got != tc.realtime {
				t.Fatalf("SupportsRealtimeObservability(%q)=%v, want %v", tc.version, got, tc.realtime)
			}
			if got := SupportsJobStateEvents(tc.version); got != tc.stateEvents {
				t.Fatalf("SupportsJobStateEvents(%q)=%v, want %v", tc.version, got, tc.stateEvents)
			}
			// B4 与事件通道同一门槛（2.3.12）。 / B4 shares the event channel gate (2.3.12).
			if got := SupportsCDCDelayEvents(tc.version); got != tc.stateEvents {
				t.Fatalf("SupportsCDCDelayEvents(%q)=%v, want %v", tc.version, got, tc.stateEvents)
			}
			if got := SupportsNonTerminalJobStateEvents(tc.version); got != tc.nonTerminal {
				t.Fatalf("SupportsNonTerminalJobStateEvents(%q)=%v, want %v", tc.version, got, tc.nonTerminal)
			}
		})
	}
}
