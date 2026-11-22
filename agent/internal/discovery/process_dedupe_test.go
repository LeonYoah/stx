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

package discovery

import (
	"sort"
	"testing"
)

func TestDedupeDiscoveredProcesses_DropsShellParentKeepsJavaChild(t *testing.T) {
	// 复现：PID 3089160=/bin/sh -c …SeaTunnelServer -d 父进程；PID 3089162=Java 子进程；仅 Java 监听。
	// Reproduction: shell parent wraps SeaTunnelServer; Java child is the real listener.
	input := []*discoveredProcessCandidate{
		{
			PID:        3089160,
			PPID:       1,
			InstallDir: "/tmp/fixture-register/seatunnel-3.0.0",
			Role:       "hybrid",
			CommandLine: "/bin/sh -c /tmp/fixture-register/seatunnel-3.0.0/bin/seatunnel-cluster.sh -d " +
				"-Dhazelcast.config=/tmp/fixture-register/seatunnel-3.0.0/config/hazelcast.yaml " +
				SeaTunnelMainClass + " -d",
		},
		{
			PID:        3089162,
			PPID:       3089160,
			InstallDir: "/tmp/fixture-register/seatunnel-3.0.0",
			Role:       "hybrid",
			CommandLine: "java -Dhazelcast.config=/tmp/fixture-register/seatunnel-3.0.0/config/hazelcast.yaml " +
				"-cp /tmp/fixture-register/seatunnel-3.0.0/lib/* " + SeaTunnelMainClass + " -d",
		},
	}

	got := dedupeDiscoveredProcesses(input)
	if len(got) != 1 {
		t.Fatalf("got %d processes after dedupe, want 1 (java child only)", len(got))
	}
	if got[0].PID != 3089162 {
		t.Fatalf("kept PID=%d, want java child 3089162", got[0].PID)
	}
}

func TestDedupeDiscoveredProcesses_KeepsDistinctRealInstances(t *testing.T) {
	// 两个真实实例（不同安装目录 / 不同实际端口）不得被合并。
	// Two real instances (different install dirs / ports) must not be merged.
	input := []*discoveredProcessCandidate{
		{
			PID:         100,
			PPID:        1,
			InstallDir:  "/tmp/fixture-a/seatunnel-3.0.0",
			Role:        "hybrid",
			CommandLine: "java -Dhazelcast.config=/tmp/fixture-a/seatunnel-3.0.0/config/hazelcast.yaml " + SeaTunnelMainClass + " -d",
		},
		{
			PID:         200,
			PPID:        1,
			InstallDir:  "/tmp/fixture-b/seatunnel-3.0.0",
			Role:        "hybrid",
			CommandLine: "java -Dhazelcast.config=/tmp/fixture-b/seatunnel-3.0.0/config/hazelcast.yaml " + SeaTunnelMainClass + " -d",
		},
	}

	got := dedupeDiscoveredProcesses(input)
	if len(got) != 2 {
		t.Fatalf("got %d processes, want 2 distinct instances", len(got))
	}
	pids := []int{got[0].PID, got[1].PID}
	sort.Ints(pids)
	if pids[0] != 100 || pids[1] != 200 {
		t.Fatalf("kept PIDs=%v, want [100 200]", pids)
	}
}

func TestDedupeDiscoveredProcesses_ShellWithoutChildStillDroppedWhenJavaSiblingExists(t *testing.T) {
	// 同目录下即使 PPID 链不完整，shell 包装也不应与 Java 并存为两个节点。
	// Even without a complete PPID chain, a shell wrapper must not coexist with the Java peer.
	input := []*discoveredProcessCandidate{
		{
			PID:         10,
			PPID:        1,
			InstallDir:  "/opt/seatunnel",
			Role:        "hybrid",
			CommandLine: "/bin/bash -c java " + SeaTunnelMainClass + " -d",
		},
		{
			PID:         11,
			PPID:        2,
			InstallDir:  "/opt/seatunnel",
			Role:        "hybrid",
			CommandLine: "java -DSEATUNNEL_HOME=/opt/seatunnel " + SeaTunnelMainClass + " -d",
		},
	}
	got := dedupeDiscoveredProcesses(input)
	if len(got) != 1 || got[0].PID != 11 {
		t.Fatalf("got %+v, want only java PID 11", summarizeCandidates(got))
	}
}

func summarizeCandidates(items []*discoveredProcessCandidate) []int {
	pids := make([]int, 0, len(items))
	for _, item := range items {
		pids = append(pids, item.PID)
	}
	return pids
}
