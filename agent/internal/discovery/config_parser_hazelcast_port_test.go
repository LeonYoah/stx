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
	"os"
	"path/filepath"
	"testing"
)

func writeHazelcastFixture(t *testing.T, installDir, fileName, body string) string {
	t.Helper()
	configDir := filepath.Join(installDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	path := filepath.Join(configDir, fileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", fileName, err)
	}
	return path
}

func TestReadHazelcastPort_NonDefaultFromActiveConfig(t *testing.T) {
	installDir := t.TempDir()
	// 复现证据：活动配置由 -Dhazelcast.config 指向 hazelcast.yaml，network.port.port=17601。
	// Reproduction: active config via -Dhazelcast.config uses network.port.port=17601.
	writeHazelcastFixture(t, installDir, "hazelcast.yaml", `
hazelcast:
  cluster-name: fixture-register
  network:
    port:
      auto-increment: false
      port: 17601
`)

	reader := NewConfigReader()
	got := reader.ReadHazelcastPort(installDir, "hybrid")
	if got != 17601 {
		t.Fatalf("HazelcastPort=%d, want 17601 from active hazelcast.yaml", got)
	}
}

func TestReadHazelcastPort_PreferExplicitConfigPath(t *testing.T) {
	installDir := t.TempDir()
	// 默认 hazelcast.yaml 仍是 5801；显式 -Dhazelcast.config 指向 17501。
	// Default hazelcast.yaml stays 5801; explicit -Dhazelcast.config points at 17501.
	writeHazelcastFixture(t, installDir, "hazelcast.yaml", `
hazelcast:
  network:
    port:
      port: 5801
`)
	explicit := writeHazelcastFixture(t, installDir, "hazelcast-runtime.yaml", `
network:
  port:
    auto-increment: false
    port: 17501
`)

	reader := NewConfigReader()
	got := reader.ReadHazelcastPortPrefer(installDir, "hybrid", explicit)
	if got != 17501 {
		t.Fatalf("HazelcastPort=%d, want 17501 from -Dhazelcast.config path", got)
	}
}

func TestReadHazelcastPort_DefaultWhenMissing(t *testing.T) {
	reader := NewConfigReader()
	if got := reader.ReadHazelcastPort(t.TempDir(), "hybrid"); got != 5801 {
		t.Fatalf("missing hybrid config: got %d, want default 5801", got)
	}
	if got := reader.ReadHazelcastPort(t.TempDir(), "worker"); got != 5802 {
		t.Fatalf("missing worker config: got %d, want default 5802", got)
	}
}

func TestReadHazelcastPort_DefaultWhenPortSectionAbsent(t *testing.T) {
	installDir := t.TempDir()
	writeHazelcastFixture(t, installDir, "hazelcast.yaml", `
hazelcast:
  cluster-name: seatunnel
  network:
    join:
      multicast:
        enabled: false
`)
	reader := NewConfigReader()
	if got := reader.ReadHazelcastPort(installDir, "hybrid"); got != 5801 {
		t.Fatalf("absent port section: got %d, want default 5801", got)
	}
}
