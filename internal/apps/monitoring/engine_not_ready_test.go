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
	"testing"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
)

func engineNotReadyTestNode() *clusterapp.NodeInfo {
	return &clusterapp.NodeInfo{
		ID:            9,
		ClusterID:     1,
		HostID:        2,
		HostName:      "host-9",
		HostIP:        "10.0.0.9",
		Role:          clusterapp.NodeRoleMasterWorker,
		Status:        clusterapp.NodeStatusRunning,
		IsOnline:      true,
		ProcessPID:    4321,
		HazelcastPort: 5801,
		UpdatedAt:     time.Now().UTC(),
	}
}

func engineNotReadySignals(up bool, jvm bool, nodeState bool) *availabilityMetricSignals {
	key := availabilitySignalKey(1, "10.0.0.9:5801")
	signals := &availabilityMetricSignals{
		endpointDown:     map[string]struct{}{},
		endpointUp:       map[string]struct{}{},
		nodeStateDown:    map[string]struct{}{},
		nodeStatePresent: map[string]struct{}{},
		jvmMetricsSeen:   map[string]struct{}{},
	}
	if up {
		signals.endpointUp[key] = struct{}{}
	}
	if jvm {
		signals.jvmMetricsSeen[key] = struct{}{}
	}
	if nodeState {
		signals.nodeStatePresent[key] = struct{}{}
	}
	return signals
}

func hasReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

// TestEngineNotReadyReasonRequiresUpJVMAndMissingNodeState up==1 + jvm_threads_current 样本 +
// node_state 完全缺失 → engine_not_ready；缺任一前置条件不触发（裸 up=1 绝不误报）。
// TestEngineNotReadyReasonRequiresUpJVMAndMissingNodeState: up==1 + a jvm_threads_current sample +
// entirely missing node_state yields engine_not_ready; missing any precondition prevents it
// (bare up=1 never false-alarms).
func TestEngineNotReadyReasonRequiresUpJVMAndMissingNodeState(t *testing.T) {
	node := engineNotReadyTestNode()

	// 半初始化：up=1 + JVM 指标在 + node_state 缺失 → 触发。
	// Half-initialized: up=1 + JVM metrics present + node_state missing -> fires.
	reasons := availabilityNodeOfflineReasons(node, engineNotReadySignals(true, true, false))
	if !hasReason(reasons, availabilityReasonEngineNotReady) {
		t.Fatalf("expected engine_not_ready, got %v", reasons)
	}

	cases := []struct {
		name           string
		up, jvm, state bool
		wantNotReady   bool
	}{
		{"node_state present", true, true, true, false},
		{"jvm metrics missing", true, false, false, false},
		{"endpoint down", false, true, false, false},
		{"all present", true, true, true, false},
		{"nothing", false, false, false, false},
	}
	for _, tc := range cases {
		reasons := availabilityNodeOfflineReasons(node, engineNotReadySignals(tc.up, tc.jvm, tc.state))
		if got := hasReason(reasons, availabilityReasonEngineNotReady); got != tc.wantNotReady {
			t.Fatalf("%s: engine_not_ready=%v, want %v (reasons=%v)", tc.name, got, tc.wantNotReady, reasons)
		}
	}

	// node_state==0（而非缺失）走既有 node_state_down 原因，不重复报 engine_not_ready。
	// node_state==0 (present but down) still reports node_state_down, not engine_not_ready.
	signals := engineNotReadySignals(true, true, true)
	signals.nodeStateDown[availabilitySignalKey(1, "10.0.0.9:5801")] = struct{}{}
	reasons = availabilityNodeOfflineReasons(node, signals)
	if !hasReason(reasons, availabilityReasonNodeStateDown) || hasReason(reasons, availabilityReasonEngineNotReady) {
		t.Fatalf("expected node_state_down only, got %v", reasons)
	}
}
