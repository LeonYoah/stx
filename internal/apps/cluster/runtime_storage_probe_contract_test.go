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
	"net/http"
	"net/http/httptest"
	"testing"

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildEngineStorageProbeConfigS3Keys 验证 S3 键位为引擎原生小写契约。
func TestBuildEngineStorageProbeConfigS3Keys(t *testing.T) {
	cfg, err := installerapp.BuildEngineStorageProbeConfig(&installerapp.EngineStorageProbeFields{
		StorageType: "S3",
		Namespace:   "/checkpoint-probe/",
		Bucket:      "s3a://stx-proxy",
		Endpoint:    "http://127.0.0.1:9000",
		AccessKey:   "ak",
		SecretKey:   "sk",
	})
	require.NoError(t, err)
	assert.Equal(t, "s3", cfg["storage.type"])
	assert.Equal(t, "s3a://stx-proxy", cfg["s3.bucket"])
	assert.Equal(t, "http://127.0.0.1:9000", cfg["fs.s3a.endpoint"])
	assert.Equal(t, "ak", cfg["fs.s3a.access.key"])
	assert.Equal(t, "sk", cfg["fs.s3a.secret.key"])
	assert.Equal(t, "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider", cfg["fs.s3a.aws.credentials.provider"])
	for _, wrong := range []string{"endpoint", "bucket", "access.key", "secret.key"} {
		_, exists := cfg[wrong]
		assert.False(t, exists, "wrong key %s must not exist", wrong)
	}
}

// TestBuildEngineStorageProbeConfigHDFSHA 验证 HDFS HA 键位与 namenode 别名配对。
func TestBuildEngineStorageProbeConfigHDFSHA(t *testing.T) {
	cfg, err := installerapp.BuildEngineStorageProbeConfig(&installerapp.EngineStorageProbeFields{
		StorageType:             "HDFS",
		Namespace:               "/cp",
		HDFSHAEnabled:           true,
		HDFSNameServices:        "stx-ha",
		HDFSHANamenodes:         "nn1,nn2",
		HDFSNamenodeRPCAddress1: "10.0.0.1:8020",
		HDFSNamenodeRPCAddress2: "10.0.0.2:8020",
	})
	require.NoError(t, err)
	assert.Equal(t, "hdfs", cfg["storage.type"])
	assert.Equal(t, "hdfs://stx-ha", cfg["fs.defaultFS"])
	assert.Equal(t, "10.0.0.1:8020", cfg["seatunnel.hadoop.dfs.namenode.rpc-address.stx-ha.nn1"])
	assert.Equal(t, "10.0.0.2:8020", cfg["seatunnel.hadoop.dfs.namenode.rpc-address.stx-ha.nn2"])
}

// TestK8sProxyProbeBodyContract 用 external 模式直连 httptest，断言请求体精确契约
// （顶层 plugin=hdfs、无遗留 probe 标志、config 引擎原生键）并验证 ok 解析。
func TestK8sProxyProbeBodyContract(t *testing.T) {
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/storage/checkpoint/probe", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true, "writable": true, "readable": true}`))
	}))
	defer server.Close()

	svc := &Service{k8sManager: k8s.NewClientManager(nil)}
	clusterObj := &Cluster{
		InfraType:   "kubernetes",
		ClusterCode: "probe-e2e",
		Config: map[string]interface{}{
			"kubernetes": map[string]interface{}{
				"host_id":    1,
				"namespace":  "seatunnel",
				"java_proxy": map[string]interface{}{"mode": "external", "external_url": server.URL},
			},
		},
	}
	probeConfig, err := installerapp.BuildEngineStorageProbeConfig(&installerapp.EngineStorageProbeFields{
		StorageType: "S3",
		Namespace:   "/probe/",
		Bucket:      "s3a://bkt",
		Endpoint:    "http://minio:9000",
		AccessKey:   "ak",
		SecretKey:   "sk",
	})
	require.NoError(t, err)
	ok, _, probeErr := svc.k8sProxyHTTPProbe(context.Background(), clusterObj, "checkpoint", probeConfig)
	require.NoError(t, probeErr)
	assert.True(t, ok, "ok field must be honored")

	require.NotNil(t, captured)
	assert.Equal(t, "hdfs", captured["plugin"], "top-level plugin must be hdfs")
	_, hasProbeFlag := captured["probe"]
	assert.False(t, hasProbeFlag, "legacy probe flag must not be sent")
	configMap, isMap := captured["config"].(map[string]interface{})
	require.True(t, isMap, "config must be an object")
	assert.Equal(t, "s3", configMap["storage.type"])
	assert.Equal(t, "s3a://bkt", configMap["s3.bucket"])
	assert.Equal(t, "http://minio:9000", configMap["fs.s3a.endpoint"])
	_, wrongKey := configMap["endpoint"]
	assert.False(t, wrongKey, "wrong key endpoint must not exist")
}
