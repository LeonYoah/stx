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
	"strings"

	"github.com/LeonYoah/stx/internal/seatunnel"
)

// EngineStorageProbeFields 构建引擎原生探活配置所需的全部连接字段。
// 键位语义必须与 agent/internal/installer/runtime_storage_probe.go 的
// buildCheckpointPluginConfig 完全一致（同一契约的两端：裸金属在 Agent 侧构建，
// K8s 在控制面直连伴生 Proxy 构建），任何漂移都会让引擎用错存储。
// EngineStorageProbeFields carries every connection field needed to build an
// engine-native probe config. Key semantics must stay identical to
// buildCheckpointPluginConfig in agent/internal/installer/runtime_storage_probe.go
// (same contract on both ends: bare-metal builds agent-side, Kubernetes builds
// control-plane-side against the companion proxy); any drift makes the engine
// use the wrong storage.
type EngineStorageProbeFields struct {
	StorageType               string
	Namespace                 string
	Endpoint                  string
	Bucket                    string
	AccessKey                 string
	SecretKey                 string
	HDFSNameNodeHost          string
	HDFSNameNodePort          int
	HDFSHAEnabled             bool
	HDFSNameServices          string
	HDFSHANamenodes           string
	HDFSNamenodeRPCAddress1   string
	HDFSNamenodeRPCAddress2   string
	HDFSFailoverProxyProvider string
	KerberosPrincipal         string
	KerberosKeytabFilePath    string
	HdfsSitePath              string
	DisableCache              *bool
	S3CredentialsProvider     string
}

// BuildEngineStorageProbeConfig 生成引擎原生小写键的探活 plugin-config。
// 返回的 map 只包含非空键；HDFS HA 缺 nameservices 时返回错误。
// BuildEngineStorageProbeConfig produces the engine-native lowercase-key probe
// plugin-config. Only non-empty keys are returned; HDFS HA without nameservices
// is an error.
func BuildEngineStorageProbeConfig(f *EngineStorageProbeFields) (map[string]string, error) {
	if f == nil {
		return nil, fmt.Errorf("probe fields required")
	}
	namespace := strings.TrimSpace(f.Namespace)
	if namespace != "" && !strings.HasSuffix(namespace, "/") {
		namespace += "/"
	}
	cfg := make(map[string]string)
	set := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			cfg[key] = value
		}
	}

	switch strings.ToUpper(strings.TrimSpace(f.StorageType)) {
	case "HDFS":
		cfg["storage.type"] = "hdfs"
		set("namespace", namespace)
		if f.HDFSHAEnabled {
			if strings.TrimSpace(f.HDFSNameServices) == "" {
				return nil, fmt.Errorf("hdfs_name_services is required for HDFS HA storage")
			}
			cfg["fs.defaultFS"] = fmt.Sprintf("hdfs://%s", f.HDFSNameServices)
			cfg["seatunnel.hadoop.dfs.nameservices"] = f.HDFSNameServices
			set(fmt.Sprintf("seatunnel.hadoop.dfs.ha.namenodes.%s", f.HDFSNameServices), f.HDFSHANamenodes)
			endpoints, err := seatunnel.ResolveHDFSHARPCAddresses(f.HDFSHANamenodes, f.HDFSNamenodeRPCAddress1, f.HDFSNamenodeRPCAddress2)
			if err != nil {
				return nil, err
			}
			for _, endpoint := range endpoints {
				cfg[fmt.Sprintf("seatunnel.hadoop.dfs.namenode.rpc-address.%s.%s", f.HDFSNameServices, endpoint.Name)] = endpoint.Address
			}
			provider := f.HDFSFailoverProxyProvider
			if provider == "" {
				provider = "org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider"
			}
			cfg[fmt.Sprintf("seatunnel.hadoop.dfs.client.failover.proxy.provider.%s", f.HDFSNameServices)] = provider
		} else {
			port := f.HDFSNameNodePort
			if port <= 0 {
				port = 8020
			}
			if host := strings.TrimSpace(f.HDFSNameNodeHost); host != "" {
				cfg["fs.defaultFS"] = fmt.Sprintf("hdfs://%s:%d", host, port)
			}
		}
		set("kerberosPrincipal", f.KerberosPrincipal)
		set("kerberosKeytabFilePath", f.KerberosKeytabFilePath)
		set("hdfs_site_path", strings.TrimSpace(f.HdfsSitePath))
		if f.DisableCache != nil {
			cfg["disable.cache"] = fmt.Sprintf("%t", *f.DisableCache)
		}
	case "OSS":
		cfg["storage.type"] = "oss"
		set("namespace", namespace)
		set("oss.bucket", f.Bucket)
		set("fs.oss.endpoint", f.Endpoint)
		set("fs.oss.accessKeyId", f.AccessKey)
		set("fs.oss.accessKeySecret", f.SecretKey)
	default: // S3 及未知类型按 s3 契约下发（引擎侧不支持会给出明确错误）
		cfg["storage.type"] = "s3"
		set("namespace", namespace)
		set("s3.bucket", f.Bucket)
		set("fs.s3a.endpoint", f.Endpoint)
		set("fs.s3a.access.key", f.AccessKey)
		set("fs.s3a.secret.key", f.SecretKey)
		provider := strings.TrimSpace(f.S3CredentialsProvider)
		if provider == "" {
			provider = "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider"
		}
		cfg["fs.s3a.aws.credentials.provider"] = provider
	}
	return cfg, nil
}
