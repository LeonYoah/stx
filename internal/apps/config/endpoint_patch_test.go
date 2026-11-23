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

package config

import (
	"strings"
	"testing"
)

const endpointSample = `hazelcast:
  cluster-name: stx
  network:
    port:
      auto-increment: true
      port-count: 100
      port: 5811
    join:
      tcp-ip:
        enabled: true
        member-list:
          - 10.88.0.11:5811
    rest-api:
      enabled: true
`

func TestPatchHazelcastEndpointRewritesPortAndAddress(t *testing.T) {
	patched, changed, err := PatchHazelcastEndpoint(endpointSample, "10.88.0.12:5833", 5833)
	if err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changed")
	}
	if !strings.Contains(patched, "port: 5833") {
		t.Fatalf("port not rewritten: %s", patched)
	}
	if !strings.Contains(patched, "public-address: 10.88.0.12:5833") {
		t.Fatalf("public-address missing: %s", patched)
	}
	if !strings.Contains(patched, "10.88.0.11:5811") || !strings.Contains(patched, "port-count: 100") {
		t.Fatalf("unrelated content lost: %s", patched)
	}
}

func TestPatchHazelcastEndpointNoop(t *testing.T) {
	patched, changed, err := PatchHazelcastEndpoint(endpointSample, "", 0)
	if err != nil || changed || patched != endpointSample {
		t.Fatalf("expected noop, changed=%v err=%v", changed, err)
	}
	patched, changed, err = PatchHazelcastEndpoint(endpointSample, "10.88.0.12:5833", 0)
	if err != nil {
		t.Fatalf("address-only patch failed: %v", err)
	}
	if !changed || !strings.Contains(patched, "public-address") {
		t.Fatalf("expected address written, changed=%v", changed)
	}
	if !strings.Contains(patched, "port: 5811") {
		t.Fatalf("port should stay: %s", patched)
	}
}

func TestHazelcastEndpointPort(t *testing.T) {
	ports := []NodePortInfo{
		{Role: "worker", HazelcastPort: 5800, WorkerPort: 5822},
		{Role: "master", HazelcastPort: 5833},
	}
	if got := HazelcastEndpointPort(ConfigTypeHazelcastMaster, "separated", ports); got != 5833 {
		t.Fatalf("master port = %d", got)
	}
	if got := HazelcastEndpointPort(ConfigTypeHazelcastWorker, "separated", ports); got != 5822 {
		t.Fatalf("worker port = %d", got)
	}
	if got := HazelcastEndpointPort(ConfigTypeHazelcast, "separated", ports); got != 0 {
		t.Fatalf("generic yaml must be skipped in separated mode, got %d", got)
	}
	if got := HazelcastEndpointPort(ConfigTypeHazelcast, "hybrid", ports); got != 5800 {
		t.Fatalf("hybrid port = %d", got)
	}
}
