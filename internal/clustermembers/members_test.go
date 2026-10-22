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
package clustermembers

import (
	"reflect"
	"testing"
)

func TestCompute(t *testing.T) {
	tests := []struct {
		name   string
		mode   string
		nodes  []Member
		add    []Member
		remove []Member
		want   []string
	}{
		{
			name:  "hybrid uses each node hazelcast port and sorts numerically",
			mode:  ModeHybrid,
			nodes: []Member{{HostID: 1, HostIP: "10.66.0.2", Role: RoleMasterWorker, Port: 5801}},
			add:   []Member{{HostID: 2, HostIP: "10.66.0.1", Role: RoleMasterWorker, Port: 5801}},
			want:  []string{"10.66.0.1:5801", "10.66.0.2:5801"},
		},
		{
			name: "hybrid zero port falls back to 5801",
			mode: ModeHybrid,
			nodes: []Member{
				{HostID: 1, HostIP: "10.0.0.10", Role: RoleHybrid},
				{HostID: 2, HostIP: "10.0.0.2", Role: RoleMasterWorker},
			},
			want: []string{"10.0.0.2:5801", "10.0.0.10:5801"},
		},
		{
			name: "separated renders master and worker ports",
			mode: ModeSeparated,
			nodes: []Member{
				{HostID: 1, HostIP: "10.0.0.1", Role: RoleMaster, Port: 5801},
				{HostID: 2, HostIP: "10.0.0.2", Role: RoleWorker, Port: 5802},
				{HostID: 3, HostIP: "10.0.0.3", Role: RoleWorker},
			},
			want: []string{"10.0.0.1:5801", "10.0.0.2:5802", "10.0.0.3:5802"},
		},
		{
			name: "separated master and worker on the same host keep both entries",
			mode: ModeSeparated,
			nodes: []Member{
				{HostID: 1, HostIP: "10.0.0.1", Role: RoleWorker, Port: 5802},
				{HostID: 1, HostIP: "10.0.0.1", Role: RoleMaster, Port: 5801},
			},
			want: []string{"10.0.0.1:5801", "10.0.0.1:5802"},
		},
		{
			name: "duplicates are removed",
			mode: ModeHybrid,
			nodes: []Member{
				{HostID: 1, HostIP: "10.0.0.1", Port: 5801},
				{HostID: 1, HostIP: "10.0.0.1", Port: 5801},
			},
			add:  []Member{{HostID: 1, HostIP: " 10.0.0.1 ", Port: 5801}},
			want: []string{"10.0.0.1:5801"},
		},
		{
			name: "remove matches by address",
			mode: ModeHybrid,
			nodes: []Member{
				{HostID: 1, HostIP: "10.66.0.2", Port: 5801},
				{HostID: 2, HostIP: "10.66.0.1", Port: 5801},
			},
			remove: []Member{{HostID: 2, HostIP: "10.66.0.1", Port: 5801}},
			want:   []string{"10.66.0.2:5801"},
		},
		{
			name:  "empty host ip is skipped and result is never nil",
			mode:  ModeHybrid,
			nodes: []Member{{HostID: 1, HostIP: ""}},
			want:  []string{},
		},
		{
			name: "host names sort after ips",
			mode: ModeHybrid,
			nodes: []Member{
				{HostID: 1, HostIP: "node-b", Port: 5801},
				{HostID: 2, HostIP: "node-a", Port: 5801},
				{HostID: 3, HostIP: "192.168.1.5", Port: 5801},
			},
			want: []string{"192.168.1.5:5801", "node-a:5801", "node-b:5801"},
		},
		{
			name:  "bare ipv6 is bracketed",
			mode:  ModeHybrid,
			nodes: []Member{{HostID: 1, HostIP: "fd00::1", Port: 5801}},
			want:  []string{"[fd00::1]:5801"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compute(tt.mode, tt.nodes, tt.add, tt.remove)
			if got == nil {
				t.Fatal("Compute must never return nil")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Compute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeIsOrderIndependent(t *testing.T) {
	nodes := []Member{
		{HostID: 3, HostIP: "10.0.0.3", Port: 5801},
		{HostID: 1, HostIP: "10.0.0.1", Port: 5801},
		{HostID: 2, HostIP: "10.0.0.2", Port: 5801},
	}
	reversed := []Member{nodes[2], nodes[1], nodes[0]}
	if a, b := Compute(ModeHybrid, nodes, nil, nil), Compute(ModeHybrid, reversed, nil, nil); !reflect.DeepEqual(a, b) {
		t.Fatalf("Compute should be stable regardless of input order: %v vs %v", a, b)
	}
}

func TestInstallerMemberList(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		masters    []string
		workers    []string
		masterPort int
		workerPort int
		want       []string
	}{
		{
			name:       "hybrid keeps input order and ignores workers",
			mode:       ModeHybrid,
			masters:    []string{"10.66.0.2", "10.66.0.1"},
			workers:    []string{"10.66.0.9"},
			masterPort: 5801,
			want:       []string{"10.66.0.2:5801", "10.66.0.1:5801"},
		},
		{
			name:    "hybrid default port",
			mode:    ModeHybrid,
			masters: []string{"a"},
			want:    []string{"a:5801"},
		},
		{
			name:       "separated masters before workers with default worker port",
			mode:       ModeSeparated,
			masters:    []string{"m2", "m1"},
			workers:    []string{"w1"},
			masterPort: 15801,
			want:       []string{"m2:15801", "m1:15801", "w1:5802"},
		},
		{
			name:    "duplicates are kept like the historical installer",
			mode:    ModeHybrid,
			masters: []string{"a", "a"},
			want:    []string{"a:5801", "a:5801"},
		},
		{
			name: "no address returns empty slice",
			mode: ModeSeparated,
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InstallerMemberList(tt.mode, tt.masters, tt.workers, tt.masterPort, tt.workerPort)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("InstallerMemberList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLoopback(t *testing.T) {
	tests := map[string]bool{
		"localhost":          true,
		"LOCALHOST:5801":     true,
		"127.0.0.1":          true,
		"127.0.0.1:5801":     true,
		"127.1.2.3:5802":     true,
		"::1":                true,
		"[::1]:5801":         true,
		"node.localhost":     true,
		"10.66.0.2":          false,
		"10.66.0.2:5801":     false,
		"172.16.0.2:5801":    false,
		"0.0.0.0:5801":       false,
		"seatunnel-master-1": false,
		"":                   false,
		"   ":                false,
	}
	for address, want := range tests {
		if got := IsLoopback(address); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", address, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		members []Member
		want    []IssueCode
	}{
		{
			name: "valid distinct hosts",
			members: []Member{
				{HostID: 1, HostIP: "10.66.0.2"},
				{HostID: 2, HostIP: "10.66.0.1"},
			},
			want: []IssueCode{},
		},
		{
			name: "same host with two roles sharing ip is fine",
			members: []Member{
				{HostID: 1, HostIP: "10.0.0.1", Role: RoleMaster},
				{HostID: 1, HostIP: "10.0.0.1", Role: RoleWorker},
			},
			want: []IssueCode{},
		},
		{
			name: "different hosts sharing ip",
			members: []Member{
				{HostID: 1, HostName: "t1", HostIP: "172.16.0.2"},
				{HostID: 2, HostName: "t2", HostIP: "172.16.0.2"},
			},
			want: []IssueCode{IssueDuplicateIP},
		},
		{
			name: "empty loopback and unspecified",
			members: []Member{
				{HostID: 1, HostIP: ""},
				{HostID: 2, HostIP: "127.0.0.1"},
				{HostID: 3, HostIP: "localhost"},
				{HostID: 4, HostIP: "0.0.0.0"},
			},
			want: []IssueCode{IssueEmptyIP, IssueLoopbackIP, IssueLoopbackIP, IssueUnspecifiedIP},
		},
		{
			name: "same host repeated loopback is reported once",
			members: []Member{
				{HostID: 2, HostIP: "127.0.0.1", Role: RoleMaster},
				{HostID: 2, HostIP: "127.0.0.1", Role: RoleWorker},
			},
			want: []IssueCode{IssueLoopbackIP},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := Validate(tt.members)
			if issues == nil {
				t.Fatal("Validate must never return nil")
			}
			got := make([]IssueCode, 0, len(issues))
			for _, issue := range issues {
				if issue.Message == "" {
					t.Fatalf("issue %s has empty message", issue.Code)
				}
				got = append(got, issue.Code)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Validate() codes = %v, want %v (issues=%+v)", got, tt.want, issues)
			}
		})
	}
}

func TestValidateDuplicateIssueListsHosts(t *testing.T) {
	issues := Validate([]Member{
		{HostID: 1, HostName: "t1", HostIP: "172.16.0.2"},
		{HostID: 2, HostName: "t2", HostIP: "172.16.0.2"},
	})
	if len(issues) != 1 {
		t.Fatalf("expected one issue, got %+v", issues)
	}
	if !reflect.DeepEqual(issues[0].HostIDs, []uint{1, 2}) || issues[0].HostIP != "172.16.0.2" {
		t.Fatalf("unexpected duplicate issue: %+v", issues[0])
	}
}
