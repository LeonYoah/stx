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
package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// legacyHazelcastMemberListAndPort 是改用 clustermembers 之前的原始实现副本，用于证明输出完全一致。
// legacyHazelcastMemberListAndPort is a verbatim copy of the pre-clustermembers logic, used to prove the
// output is unchanged.
func legacyHazelcastMemberListAndPort(params *InstallParams, fileName string) ([]string, int) {
	var memberList []string
	var port int
	if params.DeploymentMode == DeploymentModeHybrid {
		port = params.ClusterPort
		if port == 0 {
			port = 5801
		}
		for _, addr := range params.MasterAddresses {
			memberList = append(memberList, fmt.Sprintf("%s:%d", addr, port))
		}
	} else {
		masterPort := params.ClusterPort
		if masterPort == 0 {
			masterPort = 5801
		}
		workerPort := params.WorkerPort
		if workerPort == 0 {
			workerPort = 5802
		}
		for _, addr := range params.MasterAddresses {
			memberList = append(memberList, fmt.Sprintf("%s:%d", addr, masterPort))
		}
		for _, addr := range params.WorkerAddresses {
			memberList = append(memberList, fmt.Sprintf("%s:%d", addr, workerPort))
		}
		if strings.Contains(fileName, "master") {
			port = masterPort
		} else if strings.Contains(fileName, "worker") {
			port = workerPort
		} else {
			port = masterPort
		}
	}
	if len(memberList) == 0 {
		memberList = append(memberList, fmt.Sprintf("127.0.0.1:%d", port))
	}
	return memberList, port
}

// legacyHazelcastClientMembers 是 hazelcast-client.yaml 成员生成的原始实现副本。
// legacyHazelcastClientMembers is a verbatim copy of the original hazelcast-client.yaml member logic.
func legacyHazelcastClientMembers(params *InstallParams) []string {
	var memberList []string
	port := params.ClusterPort
	if port == 0 {
		port = 5801
	}
	for _, addr := range params.MasterAddresses {
		memberList = append(memberList, fmt.Sprintf("%s:%d", addr, port))
	}
	if len(memberList) == 0 {
		memberList = append(memberList, fmt.Sprintf("127.0.0.1:%d", port))
	}
	return memberList
}

func TestHazelcastMemberListMatchesLegacyInstaller(t *testing.T) {
	paramsCases := []InstallParams{
		{DeploymentMode: DeploymentModeHybrid},
		{DeploymentMode: DeploymentModeHybrid, ClusterPort: 15801},
		{DeploymentMode: DeploymentModeHybrid, MasterAddresses: []string{"10.66.0.2", "10.66.0.1"}},
		{DeploymentMode: DeploymentModeHybrid, MasterAddresses: []string{"b", "a", "b"}, WorkerAddresses: []string{"w"}, ClusterPort: 5901},
		{DeploymentMode: DeploymentModeSeparated},
		{DeploymentMode: DeploymentModeSeparated, ClusterPort: 15801, WorkerPort: 15802},
		{DeploymentMode: DeploymentModeSeparated, MasterAddresses: []string{"m2", "m1"}, WorkerAddresses: []string{"w1", "w2"}},
		{DeploymentMode: DeploymentModeSeparated, WorkerAddresses: []string{"w1"}, WorkerPort: 6802},
		{DeploymentMode: "", MasterAddresses: []string{"m1"}, WorkerAddresses: []string{"w1"}},
	}
	fileNames := []string{"hazelcast.yaml", "hazelcast-master.yaml", "hazelcast-worker.yaml"}

	for i := range paramsCases {
		params := paramsCases[i]
		for _, fileName := range fileNames {
			t.Run(fmt.Sprintf("%d/%s", i, fileName), func(t *testing.T) {
				wantMembers, wantPort := legacyHazelcastMemberListAndPort(&params, fileName)
				gotMembers, gotPort := hazelcastMemberListAndPort(&params, fileName)
				if !reflect.DeepEqual(gotMembers, wantMembers) || gotPort != wantPort {
					t.Fatalf("got (%v, %d), legacy (%v, %d)", gotMembers, gotPort, wantMembers, wantPort)
				}
			})
		}
		t.Run(fmt.Sprintf("%d/hazelcast-client.yaml", i), func(t *testing.T) {
			want := legacyHazelcastClientMembers(&params)
			if got := hazelcastClientMembers(&params); !reflect.DeepEqual(got, want) {
				t.Fatalf("client members got %v, legacy %v", got, want)
			}
		})
	}
}

func TestModifyHazelcastConfigWritesSharedMemberList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hazelcast-worker.yaml")
	original := "hazelcast:\n  network:\n    join:\n      tcp-ip:\n        enabled: true\n        member-list:\n          - localhost\n    port:\n      port: 5801\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	params := &InstallParams{
		DeploymentMode:  DeploymentModeSeparated,
		MasterAddresses: []string{"10.0.0.1"},
		WorkerAddresses: []string{"10.0.0.2"},
	}
	if err := NewInstallerManager().modifyHazelcastConfig(path, params); err != nil {
		t.Fatalf("modifyHazelcastConfig: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Hazelcast struct {
			Network struct {
				Join struct {
					TCPIP struct {
						MemberList []string `yaml:"member-list"`
					} `yaml:"tcp-ip"`
				} `yaml:"join"`
				Port struct {
					Port int `yaml:"port"`
				} `yaml:"port"`
			} `yaml:"network"`
		} `yaml:"hazelcast"`
	}
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if got, want := parsed.Hazelcast.Network.Join.TCPIP.MemberList, []string{"10.0.0.1:5801", "10.0.0.2:5802"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("member-list = %v, want %v", got, want)
	}
	if parsed.Hazelcast.Network.Port.Port != 5802 {
		t.Fatalf("worker file port = %d, want 5802", parsed.Hazelcast.Network.Port.Port)
	}
}
