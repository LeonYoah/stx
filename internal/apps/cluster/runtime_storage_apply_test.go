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
	})
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
	})
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
`
	next, err := patchRuntimeStorageYAML(content, installerapp.RuntimeStorageValidationIMAP, &ApplyRuntimeStorageRequest{
		Enabled:     false,
		StorageType: "DISABLED",
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := parseIMAPStorageFromYAML(next)
	if spec == nil || spec.Enabled || !strings.EqualFold(spec.StorageType, "DISABLED") {
		t.Fatalf("unexpected imap spec: %+v\n%s", spec, next)
	}
}
