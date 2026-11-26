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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyK8sLocalFileBoundary(t *testing.T) {
	spec := &RuntimeStorageSpec{Kind: "checkpoint", Enabled: true, StorageType: "LOCAL_FILE", SizeAvailable: true, CleanupSupported: true, TotalSizeBytes: 100, Nodes: []*RuntimeStorageNodeStat{{}}}
	applyK8sLocalFileBoundary(spec)
	assert.False(t, spec.SizeAvailable)
	assert.False(t, spec.CleanupSupported)
	assert.Nil(t, spec.Nodes)
	assert.Zero(t, spec.TotalSizeBytes)
	assert.Contains(t, spec.Warning, "Pod 重建后数据会丢失")
}

func TestApplyK8sLocalFileBoundarySkipsExternal(t *testing.T) {
	spec := &RuntimeStorageSpec{Kind: "checkpoint", Enabled: true, StorageType: "S3", SizeAvailable: true}
	applyK8sLocalFileBoundary(spec)
	assert.True(t, spec.SizeAvailable)
	assert.Empty(t, spec.Warning)
}

func TestK8sRuntimeStorageBoundaryError(t *testing.T) {
	assert.Nil(t, k8sRuntimeStorageBoundaryError("S3"))
	assert.Nil(t, k8sRuntimeStorageBoundaryError("s3"))
	err := k8sRuntimeStorageBoundaryError("LOCAL_FILE")
	assert.ErrorContains(t, err, "不支持在控制面浏览")
	err = k8sRuntimeStorageBoundaryError("")
	assert.ErrorContains(t, err, "不支持在控制面浏览")
	err = k8sRuntimeStorageBoundaryError("HDFS")
	assert.ErrorContains(t, err, "暂不支持通过 HTTP 代理")
	err = k8sRuntimeStorageBoundaryError("OSS")
	assert.ErrorContains(t, err, "暂不支持通过 HTTP 代理")
}
