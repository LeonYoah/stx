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

import "testing"

func TestCheckpointConfigValidateAcceptsHDFSHA(t *testing.T) {
	cfg := &CheckpointConfig{
		StorageType:             CheckpointStorageHDFS,
		Namespace:               "/seatunnel/checkpoint/",
		HDFSHAEnabled:           true,
		HDFSNameServices:        "cluster-a",
		HDFSHANamenodes:         "nn1,nn2",
		HDFSNamenodeRPCAddress1: "nn1.example.com:8020",
		HDFSNamenodeRPCAddress2: "nn2.example.com:8020",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected HA checkpoint config to pass validation, got: %v", err)
	}
}

func TestCheckpointConfigValidateRejectsIncompleteHDFSHA(t *testing.T) {
	cfg := &CheckpointConfig{
		StorageType:             CheckpointStorageHDFS,
		Namespace:               "/seatunnel/checkpoint/",
		HDFSHAEnabled:           true,
		HDFSNameServices:        "cluster-a",
		HDFSHANamenodes:         "nn1,nn2",
		HDFSNamenodeRPCAddress1: "nn1.example.com:8020",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected incomplete HA checkpoint config to be rejected")
	}
}

func TestIMAPConfigValidateRejectsIncompleteHDFSHA(t *testing.T) {
	cfg := &IMAPConfig{
		StorageType:             IMAPStorageHDFS,
		Namespace:               "/seatunnel/imap/",
		HDFSHAEnabled:           true,
		HDFSNameServices:        "cluster-a",
		HDFSHANamenodes:         "nn1,nn2",
		HDFSNamenodeRPCAddress1: "nn1.example.com:8020",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected incomplete HA IMAP config to be rejected")
	}
}

func TestBuildCheckpointPluginConfigUsesCustomHDFSHANamenodeAliases(t *testing.T) {
	disableCache := true
	cfg := &CheckpointConfig{
		StorageType:               CheckpointStorageHDFS,
		Namespace:                 "/seatunnel/checkpoint/",
		HDFSHAEnabled:             true,
		HDFSNameServices:          "ns2",
		HDFSHANamenodes:           "namenode326,namenode129",
		HDFSNamenodeRPCAddress1:   "nn-a.example:8020",
		HDFSNamenodeRPCAddress2:   "nn-b.example:8020",
		HDFSFailoverProxyProvider: "org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider",
		HdfsSitePath:              "/etc/hadoop/conf/hdfs-site.xml",
		DisableCache:              &disableCache,
	}

	pluginConfig, err := buildCheckpointPluginConfig(cfg)
	if err != nil {
		t.Fatalf("buildCheckpointPluginConfig returned error: %v", err)
	}
	if pluginConfig["seatunnel.hadoop.dfs.namenode.rpc-address.ns2.namenode326"] != "nn-a.example:8020" {
		t.Fatalf("first custom namenode RPC address missing: %#v", pluginConfig)
	}
	if pluginConfig["seatunnel.hadoop.dfs.namenode.rpc-address.ns2.namenode129"] != "nn-b.example:8020" {
		t.Fatalf("second custom namenode RPC address missing: %#v", pluginConfig)
	}
	if pluginConfig["hdfs_site_path"] != "/etc/hadoop/conf/hdfs-site.xml" || pluginConfig["disable.cache"] != "true" {
		t.Fatalf("HDFS advanced options missing: %#v", pluginConfig)
	}
}
