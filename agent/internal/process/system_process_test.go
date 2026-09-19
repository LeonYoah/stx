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
package process

import "testing"

func TestMatchesManagedSeaTunnelProcessSeparatesInstallDirectories(t *testing.T) {
	processes := parseUnixProcessList(`
92322 java -Dseatunnel.logs.path=/tmp/seatunnel-2.3.13/logs -Dseatunnel.config=/tmp/seatunnel-2.3.13/config/seatunnel.yaml -cp /tmp/seatunnel-2.3.13/lib/* org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer -d
92444 java -Dseatunnel.logs.path=/tmp/seatunnel-2.3.12/logs -Dseatunnel.config=/tmp/seatunnel-2.3.12/config/seatunnel.yaml -cp /tmp/seatunnel-2.3.12/lib/* org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer -d
`)

	matched := matchingProcessIDs(processes, "/tmp/seatunnel-2.3.12", "master/worker")
	if len(matched) != 1 || matched[0] != 92444 {
		t.Fatalf("匹配到的 PID 为 %v，期望仅包含 92444", matched)
	}
}

func TestMatchesManagedSeaTunnelProcessSeparatesRoles(t *testing.T) {
	processes := parseUnixProcessList(`
1001 java -Dseatunnel.config=/opt/seatunnel/config/seatunnel.yaml -cp /opt/seatunnel/lib/* org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer -r master
1002 java -Dseatunnel.config=/opt/seatunnel/config/seatunnel.yaml -cp /opt/seatunnel/lib/* org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer -r worker
1003 java -Dseatunnel.config=/opt/seatunnel/config/seatunnel.yaml -cp /opt/seatunnel/lib/* org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer -d
`)

	tests := []struct {
		role string
		pid  int
	}{
		{role: "master", pid: 1001},
		{role: "worker", pid: 1002},
		{role: "master/worker", pid: 1003},
	}
	for _, test := range tests {
		matched := matchingProcessIDs(processes, "/opt/seatunnel", test.role)
		if len(matched) != 1 || matched[0] != test.pid {
			t.Fatalf("角色 %s 匹配到的 PID 为 %v，期望仅包含 %d", test.role, matched, test.pid)
		}
	}
}

func matchingProcessIDs(processes []systemProcess, installDir, role string) []int {
	pids := make([]int, 0)
	for _, proc := range processes {
		if matchesManagedSeaTunnelProcess(proc.CommandLine, installDir, role) {
			pids = append(pids, proc.PID)
		}
	}
	return pids
}
