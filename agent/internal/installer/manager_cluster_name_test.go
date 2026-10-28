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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestModifyHazelcastConfigSetsClusterName 验证在配置集群时将 ClusterName 写入 hazelcast.yaml 与客户端配置。
// TestModifyHazelcastConfigSetsClusterName verifies ClusterName is written to hazelcast.yaml and client config during configuration.
func TestModifyHazelcastConfigSetsClusterName(t *testing.T) {
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "hazelcast.yaml")
	clientPath := filepath.Join(dir, "hazelcast-client.yaml")

	serverContent := `hazelcast:
  cluster-name: seatunnel
  network:
    port:
      port: 5801
    join:
      tcp-ip:
        enabled: true
        member-list:
          - 127.0.0.1:5801
`
	clientContent := `hazelcast-client:
  cluster-name: seatunnel
  network:
    cluster-members:
      - 127.0.0.1:5801
`
	if err := os.WriteFile(serverPath, []byte(serverContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clientPath, []byte(clientContent), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewInstallerManager()
	params := &InstallParams{
		DeploymentMode: DeploymentModeHybrid,
		ClusterName:    "prodzeta01",
	}

	if err := mgr.modifyHazelcastConfig(serverPath, params); err != nil {
		t.Fatalf("modifyHazelcastConfig: %v", err)
	}
	if err := mgr.modifyHazelcastClientConfig(clientPath, params); err != nil {
		t.Fatalf("modifyHazelcastClientConfig: %v", err)
	}

	serverOut, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(serverOut), "cluster-name: prodzeta01") {
		t.Fatalf("expected cluster-name: prodzeta01 in server config, got:\n%s", string(serverOut))
	}

	clientOut, err := os.ReadFile(clientPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(clientOut), "cluster-name: prodzeta01") {
		t.Fatalf("expected cluster-name: prodzeta01 in client config, got:\n%s", string(clientOut))
	}
}
