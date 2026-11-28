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
	"strings"
	"testing"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
)

func TestPatchCheckpointYAMLUpdatesPluginConfig(t *testing.T) {
	content := `
seatunnel:
  engine:
    checkpoint:
      storage:
        plugin-config:
          namespace: /tmp/seatunnel/checkpoint_snapshot
          storage.type: hdfs
          fs.defaultFS: file:///
`
	next, err := patchRuntimeStorageYAML(content, installerapp.RuntimeStorageValidationCheckpoint, &ApplyRuntimeStorageRequest{
		Enabled:     true,
		StorageType: "S3",
		Namespace:   "s3a://bucket/checkpoint/",
		Endpoint:    "http://127.0.0.1:9000",
		Bucket:      "bucket",
		AccessKey:   "ak",
		SecretKey:   "sk",
	}, "2.3.13")
	if err != nil {
		t.Fatal(err)
	}
	spec := parseCheckpointStorageFromYAML(next)
	if spec == nil || spec.StorageType != "S3" || spec.Bucket != "bucket" || spec.Endpoint != "http://127.0.0.1:9000" {
		t.Fatalf("unexpected checkpoint spec: %+v\n%s", spec, next)
	}
	if !strings.Contains(next, "fs.s3a.aws.credentials.provider") {
		t.Fatalf("missing s3 credentials provider:\n%s", next)
	}
}

func TestPatchCheckpointYAMLWritesHDFSHAAndKerberos(t *testing.T) {
	content := `
seatunnel:
  engine:
    checkpoint:
      storage:
        plugin-config:
          namespace: /tmp/seatunnel/checkpoint_snapshot
          storage.type: hdfs
          fs.defaultFS: file:///
`
	cacheEnabled := false
	next, err := patchRuntimeStorageYAML(content, installerapp.RuntimeStorageValidationCheckpoint, &ApplyRuntimeStorageRequest{
		Enabled:                 true,
		StorageType:             "HDFS",
		Namespace:               "/seatunnel/checkpoint/",
		HDFSHAEnabled:           true,
		HDFSNameServices:        "usdp-bing",
		HDFSHANamenodes:         "nn1,nn2",
		HDFSNamenodeRPCAddress1: "usdp-bing-nn1:8020",
		HDFSNamenodeRPCAddress2: "usdp-bing-nn2:8020",
		KerberosPrincipal:       "hdfs/nn@EXAMPLE.COM",
		KerberosKeytabFilePath:  "/etc/security/hdfs.keytab",
		HdfsSitePath:            "/etc/hadoop/hdfs-site.xml",
		DisableCache:            &cacheEnabled,
	}, "2.3.13")
	if err != nil {
		t.Fatal(err)
	}
	spec := parseCheckpointStorageFromYAML(next)
	if spec == nil || spec.StorageType != "HDFS" || !spec.HDFSHAEnabled || spec.HDFSNameServices != "usdp-bing" {
		t.Fatalf("unexpected hdfs spec: %+v\n%s", spec, next)
	}
	if spec.KerberosPrincipal != "hdfs/nn@EXAMPLE.COM" || spec.HdfsSitePath != "/etc/hadoop/hdfs-site.xml" {
		t.Fatalf("missing kerberos or hdfs-site: %+v", spec)
	}
	if spec.DisableCache == nil || *spec.DisableCache {
		t.Fatalf("disable.cache not parsed: %+v", spec.DisableCache)
	}
	if strings.Contains(next, "fs.oss.") || strings.Contains(next, "fs.s3a.") {
		t.Fatalf("hdfs yaml leaked object-store keys:\n%s", next)
	}
}

func TestPatchIMAPYAMLCanDisable(t *testing.T) {
	content := `
hazelcast:
  map:
    engine*:
      map-store:
        enabled: true
        properties:
          storage.type: hdfs
          namespace: /tmp/imap
          fs.defaultFS: file:///
    engine_checkpoint_monitor:
      map-store:
        enabled: false
`
	next, err := patchRuntimeStorageYAML(content, installerapp.RuntimeStorageValidationIMAP, &ApplyRuntimeStorageRequest{
		Enabled:     false,
		StorageType: "DISABLED",
	}, "2.3.13")
	if err != nil {
		t.Fatal(err)
	}
	spec := parseIMAPStorageFromYAML(next)
	if spec == nil || spec.Enabled || !strings.EqualFold(spec.StorageType, "DISABLED") {
		t.Fatalf("unexpected imap spec: %+v\n%s", spec, next)
	}
	if strings.Contains(next, "factory-class-name") || strings.Contains(next, "initial-mode") || strings.Contains(next, "properties:") || strings.Contains(next, "engine_checkpoint_monitor") {
		t.Fatalf("disabled imap yaml leaked map-store attributes:\n%s", next)
	}
}

// TestPatchIMAPYAMLEnablesFactoryClassName 验证启用 IMAP 时自动注入 SeaTunnel FileMapStoreFactory 与 initial-mode，
// 且仅对 SeaTunnel 2.3.13 临时排除 engine_checkpoint_monitor 的 EAGER MapStore。
// TestPatchIMAPYAMLEnablesFactoryClassName verifies SeaTunnel FileMapStoreFactory and initial-mode are injected
// and engine_checkpoint_monitor MapStore is temporarily disabled only for SeaTunnel 2.3.13.
func TestPatchIMAPYAMLEnablesFactoryClassName(t *testing.T) {
	content := `
hazelcast:
  map:
    engine*:
      map-store:
        enabled: false
`
	next, err := patchRuntimeStorageYAML(content, installerapp.RuntimeStorageValidationIMAP, &ApplyRuntimeStorageRequest{
		Enabled:     true,
		StorageType: "HDFS",
		Namespace:   "/tmp/imap/",
	}, "2.3.13")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "factory-class-name: org.apache.seatunnel.engine.server.persistence.FileMapStoreFactory") {
		t.Fatalf("expected factory-class-name in output:\n%s", next)
	}
	if !strings.Contains(next, "initial-mode: EAGER") {
		t.Fatalf("expected initial-mode in output:\n%s", next)
	}
	if !strings.Contains(next, "enabled: true") {
		t.Fatalf("expected enabled: true in output:\n%s", next)
	}
	if !strings.Contains(next, "engine_checkpoint_monitor:") || !strings.Contains(next, "enabled: false") {
		t.Fatalf("expected engine_checkpoint_monitor map-store disabled override on 2.3.13:\n%s", next)
	}
	if !strings.Contains(next, "type: hdfs") {
		t.Fatalf("expected type: hdfs in output:\n%s", next)
	}
	if !strings.Contains(next, "clusterName: seatunnel") {
		t.Fatalf("expected fallback clusterName: seatunnel in output:\n%s", next)
	}

	// 升级或切换到 3.0.0 时不得写入且应清理旧的 2.3.13 临时规避覆盖项。
	// On 3.0.0, the temporary 2.3.13 workaround must not be written and any stale override must be removed.
	nextV3, err := patchRuntimeStorageYAML(next, installerapp.RuntimeStorageValidationIMAP, &ApplyRuntimeStorageRequest{
		Enabled:     true,
		StorageType: "HDFS",
		Namespace:   "/tmp/imap/",
	}, "3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(nextV3, "engine_checkpoint_monitor") {
		t.Fatalf("expected engine_checkpoint_monitor override to be omitted on 3.0.0:\n%s", nextV3)
	}
}

// TestPatchIMAPYAMLInheritsClusterName 验证启用 IMAP 时自动静默继承当前 Hazelcast 的 cluster-name。
// TestPatchIMAPYAMLInheritsClusterName verifies IMAP silently inherits cluster-name from current Hazelcast config.
func TestPatchIMAPYAMLInheritsClusterName(t *testing.T) {
	content := `
hazelcast:
  cluster-name: production-stx-01
  map:
    engine*:
      map-store:
        enabled: false
`
	next, err := patchRuntimeStorageYAML(content, installerapp.RuntimeStorageValidationIMAP, &ApplyRuntimeStorageRequest{
		Enabled:     true,
		StorageType: "HDFS",
		Namespace:   "/tmp/imap/",
	}, "3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "clusterName: production-stx-01") {
		t.Fatalf("expected inherited clusterName: production-stx-01 in output:\n%s", next)
	}

	// 再次应用，验证已有自定义 clusterName 时优先保留原有值
	// Re-apply to verify existing custom clusterName is preserved
	reapply, err := patchRuntimeStorageYAML(next, installerapp.RuntimeStorageValidationIMAP, &ApplyRuntimeStorageRequest{
		Enabled:     true,
		StorageType: "HDFS",
		Namespace:   "/tmp/imap-v2/",
	}, "3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reapply, "clusterName: production-stx-01") {
		t.Fatalf("expected preserved clusterName: production-stx-01 in output:\n%s", reapply)
	}
	if !strings.Contains(reapply, "namespace: /tmp/imap-v2/") {
		t.Fatalf("expected updated namespace in output:\n%s", reapply)
	}
}

func TestRuntimeStorageConfigTargetsRespectsDeploymentMode(t *testing.T) {
	configs := []*appconfig.ConfigInfo{
		{ID: 1, ConfigType: appconfig.ConfigTypeHazelcastMaster, IsTemplate: true},
		{ID: 2, ConfigType: appconfig.ConfigTypeHazelcast, IsTemplate: true},
		{ID: 3, ConfigType: appconfig.ConfigTypeSeatunnel, IsTemplate: true},
	}

	hybridTargets := runtimeStorageConfigTargets(configs, installerapp.RuntimeStorageValidationIMAP, DeploymentModeHybrid)
	if len(hybridTargets) != 1 || hybridTargets[0].ConfigType != appconfig.ConfigTypeHazelcast {
		t.Fatalf("expected hybrid IMAP target hazelcast.yaml, got %+v", hybridTargets)
	}

	separatedTargets := runtimeStorageConfigTargets(configs, installerapp.RuntimeStorageValidationIMAP, DeploymentModeSeparated)
	if len(separatedTargets) != 1 || separatedTargets[0].ConfigType != appconfig.ConfigTypeHazelcastMaster {
		t.Fatalf("expected separated IMAP target hazelcast-master.yaml, got %+v", separatedTargets)
	}

	fallbackTargets := runtimeStorageConfigTargets(configs[:1], installerapp.RuntimeStorageValidationIMAP, DeploymentModeHybrid)
	if len(fallbackTargets) != 1 || fallbackTargets[0].ConfigType != appconfig.ConfigTypeHazelcastMaster {
		t.Fatalf("expected fallback to hazelcast-master.yaml when hazelcast.yaml is absent, got %+v", fallbackTargets)
	}
}
