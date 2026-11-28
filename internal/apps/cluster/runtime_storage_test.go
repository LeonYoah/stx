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

package cluster

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
)

func TestParseCheckpointStorageFromYAMLLocalFile(t *testing.T) {
	content := `
seatunnel:
  engine:
    checkpoint:
      storage:
        plugin-config:
          namespace: /tmp/seatunnel/checkpoint_snapshot
          storage.type: hdfs
          fs.defaultFS: file:///tmp/
`

	spec := parseCheckpointStorageFromYAML(content)
	if spec == nil {
		t.Fatal("expected checkpoint storage spec")
	}
	if spec.StorageType != "LOCAL_FILE" {
		t.Fatalf("expected LOCAL_FILE storage type, got %s", spec.StorageType)
	}
	if spec.External {
		t.Fatalf("expected local checkpoint storage to be non-external")
	}
	if spec.Namespace != "/tmp/seatunnel/checkpoint_snapshot" {
		t.Fatalf("unexpected namespace: %s", spec.Namespace)
	}
}

func TestParseCheckpointValidationConfigFromYAMLLocalFile(t *testing.T) {
	content := `
seatunnel:
  engine:
    checkpoint:
      storage:
        plugin-config:
          namespace: /tmp/seatunnel/checkpoint_snapshot
          storage.type: hdfs
          fs.defaultFS: file:///tmp/
`

	cfg := parseCheckpointValidationConfigFromYAML(content)
	if cfg == nil {
		t.Fatal("expected checkpoint validation config")
	}
	if cfg.StorageType != "LOCAL_FILE" {
		t.Fatalf("expected LOCAL_FILE storage type, got %s", cfg.StorageType)
	}
	if cfg.Namespace != "/tmp/seatunnel/checkpoint_snapshot" {
		t.Fatalf("unexpected namespace: %s", cfg.Namespace)
	}
}

func TestParseCheckpointValidationConfigFromYAMLHDFSACustomNamenodes(t *testing.T) {
	content := `
seatunnel:
  engine:
    checkpoint:
      storage:
        plugin-config:
          namespace: /seatunnel/checkpoint/
          storage.type: hdfs
          fs.defaultFS: hdfs://hpcluster
          hdfs_site_path: /etc/hadoop/conf/hdfs-site.xml
          disable.cache: true
          kerberosPrincipal: hive/_HOST@EXAMPLE.COM
          kerberosKeytabFilePath: /etc/security/keytabs/hdfs.keytab
          seatunnel.hadoop.dfs.nameservices: hpcluster
          seatunnel.hadoop.dfs.ha.namenodes.hpcluster: namenode326,namenode129
          seatunnel.hadoop.dfs.namenode.rpc-address.hpcluster.namenode326: nn1.example.com:8020
          seatunnel.hadoop.dfs.namenode.rpc-address.hpcluster.namenode129: nn2.example.com:8020
          seatunnel.hadoop.dfs.client.failover.proxy.provider.hpcluster: org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider
`

	cfg := parseCheckpointValidationConfigFromYAML(content)
	if cfg == nil {
		t.Fatal("expected checkpoint validation config")
	}
	if !cfg.HDFSHAEnabled || cfg.HDFSNameServices != "hpcluster" || cfg.HDFSHANamenodes != "namenode326,namenode129" {
		t.Fatalf("HDFS HA config was not preserved: %#v", cfg)
	}
	if cfg.HDFSNamenodeRPCAddress1 != "nn1.example.com:8020" || cfg.HDFSNamenodeRPCAddress2 != "nn2.example.com:8020" {
		t.Fatalf("custom namenode RPC addresses parsed incorrectly: %#v", cfg)
	}
	if cfg.HdfsSitePath != "/etc/hadoop/conf/hdfs-site.xml" || cfg.KerberosPrincipal == "" || cfg.KerberosKeytabFilePath == "" {
		t.Fatalf("HDFS advanced config was not preserved: %#v", cfg)
	}
	if cfg.DisableCache == nil || !*cfg.DisableCache {
		t.Fatalf("disable.cache was not preserved: %#v", cfg)
	}
}

func TestParseCheckpointValidationConfigFromYAMLDefaultHDFSHAWithoutStorageType(t *testing.T) {
	content := `
seatunnel:
  engine:
    checkpoint:
      storage:
        plugin-config:
          namespace: /seatunnel/checkpoint/
          fs.defaultFS: hdfs://hpcluster
          seatunnel.hadoop.dfs.nameservices: hpcluster
          seatunnel.hadoop.dfs.ha.namenodes.hpcluster: namenode326,namenode129
          seatunnel.hadoop.dfs.namenode.rpc-address.hpcluster.namenode326: nn1.example.com:8020
          seatunnel.hadoop.dfs.namenode.rpc-address.hpcluster.namenode129: nn2.example.com:8020
`
	cfg := parseCheckpointValidationConfigFromYAML(content)
	if cfg == nil {
		t.Fatal("expected checkpoint validation config")
	}
	if cfg.StorageType != installerapp.CheckpointStorageHDFS || !cfg.HDFSHAEnabled {
		t.Fatalf("omitted storage.type should default to HDFS HA: %#v", cfg)
	}
	if cfg.HDFSNamenodeRPCAddress1 != "nn1.example.com:8020" || cfg.HDFSNamenodeRPCAddress2 != "nn2.example.com:8020" {
		t.Fatalf("custom namenode RPC addresses missing when storage.type omitted: %#v", cfg)
	}
}

func TestOptionalBoolPtrEmptyStringReturnsNil(t *testing.T) {
	if got := optionalBoolPtr(""); got != nil {
		t.Fatalf("expected nil for empty string, got %v", *got)
	}
	if got := optionalBoolPtr("   "); got != nil {
		t.Fatalf("expected nil for whitespace string, got %v", *got)
	}
	spec := runtimeSpecFromClusterConfig("checkpoint", map[string]interface{}{
		"storage_type":  "HDFS",
		"disable_cache": "",
	})
	if spec.DisableCache != nil {
		t.Fatalf("empty disable_cache must stay nil, got %v", *spec.DisableCache)
	}
}

func TestRuntimeSpecFromPluginConfigHDFSACustomNamenodes(t *testing.T) {
	spec := runtimeSpecFromPluginConfig("checkpoint", map[string]interface{}{
		"namespace":                         "/seatunnel/checkpoint/",
		"storage.type":                      "hdfs",
		"fs.defaultFS":                      "hdfs://hpcluster",
		"seatunnel.hadoop.dfs.nameservices": "hpcluster",
		"seatunnel.hadoop.dfs.ha.namenodes.hpcluster":                     "namenode326,namenode129",
		"seatunnel.hadoop.dfs.namenode.rpc-address.hpcluster.namenode326": "nn1.example.com:8020",
		"seatunnel.hadoop.dfs.namenode.rpc-address.hpcluster.namenode129": "nn2.example.com:8020",
	})
	if spec == nil {
		t.Fatal("expected runtime storage spec")
	}
	if spec.HDFSNamenodeRPCAddress1 != "nn1.example.com:8020" || spec.HDFSNamenodeRPCAddress2 != "nn2.example.com:8020" {
		t.Fatalf("custom namenode RPC addresses parsed incorrectly: %#v", spec)
	}
}

func TestRuntimeStorageProxyParamsPreserveHDFSHA(t *testing.T) {
	checkpoint := parseCheckpointValidationConfigFromYAML(`
seatunnel:
  engine:
    checkpoint:
      storage:
        plugin-config:
          namespace: /seatunnel/checkpoint/
          storage.type: hdfs
          fs.defaultFS: hdfs://hpcluster
          hdfs_site_path: /etc/hadoop/conf/hdfs-site.xml
          disable.cache: true
          seatunnel.hadoop.dfs.nameservices: hpcluster
          seatunnel.hadoop.dfs.ha.namenodes.hpcluster: namenode326,namenode129
          seatunnel.hadoop.dfs.namenode.rpc-address.hpcluster.namenode326: nn1.example.com:8020
          seatunnel.hadoop.dfs.namenode.rpc-address.hpcluster.namenode129: nn2.example.com:8020
          seatunnel.hadoop.dfs.client.failover.proxy.provider.hpcluster: org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider
`)
	params := runtimeStorageProxyParams("", "", installerapp.RuntimeStorageValidationCheckpoint, checkpoint, nil, nil)
	for key, want := range map[string]string{
		"hdfs_ha_enabled":              "true",
		"hdfs_name_services":           "hpcluster",
		"hdfs_ha_namenodes":            "namenode326,namenode129",
		"hdfs_namenode_rpc_address_1":  "nn1.example.com:8020",
		"hdfs_namenode_rpc_address_2":  "nn2.example.com:8020",
		"hdfs_failover_proxy_provider": "org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider",
		"hdfs_site_path":               "/etc/hadoop/conf/hdfs-site.xml",
		"disable_cache":                "true",
	} {
		if params[key] != want {
			t.Fatalf("%s mismatch: got %q want %q (params=%#v)", key, params[key], want, params)
		}
	}
}

func TestParseIMAPStorageFromYAMLDisabled(t *testing.T) {
	content := `
map:
  engine*:
    map-store:
      enabled: false
`

	spec := parseIMAPStorageFromYAML(content)
	if spec == nil {
		t.Fatal("expected imap storage spec")
	}
	if spec.Enabled {
		t.Fatalf("expected disabled IMAP")
	}
	if spec.StorageType != "DISABLED" {
		t.Fatalf("expected DISABLED storage type, got %s", spec.StorageType)
	}
}

func TestParseIMAPStorageFromYAMLS3(t *testing.T) {
	content := `
map:
  engine*:
    map-store:
      enabled: true
      properties:
        type: hdfs
        namespace: /seatunnel/engine
        storage.type: s3
        s3.bucket: s3a://seatunnel-bucket
        fs.s3a.endpoint: http://127.0.0.1:9000
`

	spec := parseIMAPStorageFromYAML(content)
	if spec == nil {
		t.Fatal("expected imap storage spec")
	}
	if !spec.Enabled {
		t.Fatalf("expected enabled IMAP")
	}
	if spec.StorageType != "S3" {
		t.Fatalf("expected S3 storage type, got %s", spec.StorageType)
	}
	if !spec.External {
		t.Fatalf("expected S3 IMAP to be external")
	}
	if spec.Bucket != "s3a://seatunnel-bucket" {
		t.Fatalf("unexpected bucket: %s", spec.Bucket)
	}
	if spec.Endpoint != "http://127.0.0.1:9000" {
		t.Fatalf("unexpected endpoint: %s", spec.Endpoint)
	}
}

func TestParseIMAPStorageFromYAMLHazelcastRoot(t *testing.T) {
	content := `
hazelcast:
  map:
    engine*:
      map-store:
        enabled: true
        properties:
          type: hdfs
          namespace: /seatunnel/imap/
          storage.type: s3
          s3.bucket: s3a://seatunnel-imap
          fs.defaultFS: s3a://seatunnel-imap
          fs.s3a.endpoint: http://127.0.0.1:19000
`

	spec := parseIMAPStorageFromYAML(content)
	if spec == nil {
		t.Fatal("expected imap storage spec")
	}
	if !spec.Enabled {
		t.Fatalf("expected enabled IMAP")
	}
	if spec.StorageType != "S3" {
		t.Fatalf("expected S3 storage type, got %s", spec.StorageType)
	}
	if spec.Namespace != "/seatunnel/imap/" {
		t.Fatalf("unexpected namespace: %s", spec.Namespace)
	}
	if spec.Endpoint != "http://127.0.0.1:19000" {
		t.Fatalf("unexpected endpoint: %s", spec.Endpoint)
	}
}

func TestRuntimeSpecFromClusterConfigDisabledIMAP(t *testing.T) {
	spec := runtimeSpecFromClusterConfig("imap", map[string]interface{}{
		"storage_type": "DISABLED",
		"namespace":    "/tmp/seatunnel/imap/",
	})
	if spec == nil {
		t.Fatal("expected runtime storage spec")
	}
	if spec.Enabled {
		t.Fatalf("expected disabled IMAP spec")
	}
	if spec.External {
		t.Fatalf("expected disabled IMAP to be non-external")
	}
}

func TestParseIMAPValidationConfigFromYAMLDisabled(t *testing.T) {
	cfg := parseIMAPValidationConfigFromYAML(`
hazelcast:
  map:
    engine*:
      map-store:
        enabled: false
`)
	if cfg == nil || cfg.StorageType != installerapp.IMAPStorageDisabled {
		t.Fatalf("关闭的 IMAP 配置解析错误 / disabled IMAP config parsed incorrectly: %#v", cfg)
	}
}

func TestIMAPValidationConfigFromClusterDefaultsDisabled(t *testing.T) {
	cfg := imapValidationConfigFromCluster(map[string]interface{}{"enabled": false})
	if cfg == nil || cfg.StorageType != installerapp.IMAPStorageDisabled {
		t.Fatalf("关闭的集群 IMAP 配置解析错误 / disabled cluster IMAP config parsed incorrectly: %#v", cfg)
	}
}

func TestRuntimeStorageListResultKeepsEmptyItemsArray(t *testing.T) {
	content, err := json.Marshal(&RuntimeStorageListResult{
		ClusterID: 6,
		Kind:      "imap",
		Items:     make([]RuntimeStorageListItem, 0),
	})
	if err != nil {
		t.Fatalf("编码运行时存储列表失败 / encoding runtime storage list failed: %v", err)
	}
	if !strings.Contains(string(content), `"items":[]`) {
		t.Fatalf("空列表必须保留 items 数组 / empty list must keep the items array: %s", content)
	}
}

func TestAsStringNilReturnsEmpty(t *testing.T) {
	if got := asString(nil); got != "" {
		t.Fatalf("expected empty string for nil, got %q", got)
	}
}

type mockRuntimeStorageAgentSender struct {
	responses map[string]string
	commands  []mockAgentCommand
}

func (m *mockRuntimeStorageAgentSender) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	m.commands = append(m.commands, mockAgentCommand{
		agentID:     agentID,
		commandType: commandType,
		params:      params,
	})
	return true, m.responses[agentID], nil
}

func TestFillLocalRuntimeStorageStatsDeduplicatesHosts(t *testing.T) {
	now := time.Now()
	hostProvider := NewMockHostProvider()
	hostProvider.AddHost(&HostInfo{ID: 1, Name: "host-1", AgentID: "agent-1", LastHeartbeat: &now})
	hostProvider.AddHost(&HostInfo{ID: 2, Name: "host-2", AgentID: "agent-2", LastHeartbeat: &now})

	payload1, err := json.Marshal(map[string]any{
		"success": true,
		"message": "ok",
		"details": map[string]string{
			"exists":     "true",
			"path":       "/tmp/runtime",
			"size_bytes": "100",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal payload1: %v", err)
	}
	payload2, err := json.Marshal(map[string]any{
		"success": true,
		"message": "ok",
		"details": map[string]string{
			"exists":     "true",
			"path":       "/tmp/runtime",
			"size_bytes": "200",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal payload2: %v", err)
	}

	agentSender := &mockRuntimeStorageAgentSender{
		responses: map[string]string{
			"agent-1": string(payload1),
			"agent-2": string(payload2),
		},
	}

	svc := &Service{
		hostProvider:     hostProvider,
		agentSender:      agentSender,
		heartbeatTimeout: time.Minute,
	}

	spec := &RuntimeStorageSpec{
		Kind:        "imap",
		Enabled:     true,
		StorageType: "LOCAL_FILE",
		Namespace:   "/tmp/runtime",
	}

	nodes := []*NodeInfo{
		{ID: 1, HostID: 1, HostName: "host-1", Role: NodeRoleMaster},
		{ID: 2, HostID: 1, HostName: "host-1", Role: NodeRoleWorker},
		{ID: 3, HostID: 2, HostName: "host-2", Role: NodeRoleWorker},
	}

	svc.fillLocalRuntimeStorageStats(context.Background(), nodes, spec)

	if len(spec.Nodes) != 2 {
		t.Fatalf("expected one runtime storage entry per host, got %d", len(spec.Nodes))
	}
	if spec.TotalSizeBytes != 300 {
		t.Fatalf("expected deduplicated total size 300, got %d", spec.TotalSizeBytes)
	}
	if len(agentSender.commands) != 2 {
		t.Fatalf("expected stat_path to run once per host, got %d commands", len(agentSender.commands))
	}
}

func TestRuntimeStorageListItemUnmarshalAcceptsCamelCaseFields(t *testing.T) {
	raw := []byte(`{
		"path":"file:/tmp/stx-local-imap/engine_checkpoint-id-map",
		"name":"engine_checkpoint-id-map",
		"directory":true,
		"sizeBytes":9750,
		"modifiedAt":"2026-03-24T00:30:41+08:00"
	}`)

	var item RuntimeStorageListItem
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatalf("failed to unmarshal runtime storage list item: %v", err)
	}
	if item.SizeBytes != 9750 {
		t.Fatalf("expected size 9750, got %d", item.SizeBytes)
	}
	if item.ModifiedAt != "2026-03-24T00:30:41+08:00" {
		t.Fatalf("unexpected modified_at: %s", item.ModifiedAt)
	}
}

func TestIMAPHazelcastConfigType(t *testing.T) {
	cases := []struct {
		name string
		role NodeRole
		want string
	}{
		{name: "master uses separated master config", role: NodeRoleMaster, want: "hazelcast-master.yaml"},
		{name: "worker also uses separated master config", role: NodeRoleWorker, want: "hazelcast-master.yaml"},
		{name: "hybrid uses default config", role: NodeRoleMasterWorker, want: "hazelcast.yaml"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := imapHazelcastConfigType(tc.role); got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}
