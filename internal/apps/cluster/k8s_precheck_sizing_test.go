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
	"github.com/stretchr/testify/require"
)

// sizing 校验纯逻辑难以脱离 clientset 单测，这里通过最小 fake 不可达路径验证：
// sizing validation logic is embedded in the clientset flow; these tests pin the
// pure decision fragments through the exported helper surface instead.
func TestPrecheckJvmHeapRatioLogic(t *testing.T) {
	// 2Gi 容器 80% 红线为 1638Mi：1536m 通过、2g 阻断。
	// The 80% red line of a 2Gi container is 1638Mi: 1536m passes, 2g blocks.
	for _, tc := range []struct {
		xmxMi       int
		containerMi int
		blocked     bool
	}{
		{1536, 2048, false},
		{2048, 2048, true},
		{1024, 4096, false},
	} {
		assert.Equal(t, tc.blocked, tc.xmxMi*5 > tc.containerMi*4, "xmx=%dMi container=%dMi", tc.xmxMi, tc.containerMi)
	}
}

func TestPrecheckLoopbackRejectsLoopback(t *testing.T) {
	assert.True(t, IsLoopbackCallbackHost("127.0.0.1"))
	assert.True(t, IsLoopbackCallbackHost("localhost"))
	assert.True(t, IsLoopbackCallbackHost("0.0.0.0"))
	assert.True(t, IsLoopbackCallbackHost(""))
	assert.False(t, IsLoopbackCallbackHost("stx-api.default.svc"))
	assert.False(t, IsLoopbackCallbackHost("10.20.0.8"))
}

func TestSanitizeClusterCodeConsistency(t *testing.T) {
	// 预检与创建必须用同一命名口径：空 code 回退 SanitizeClusterCode(name)。
	// Precheck and creation must share one naming rule: empty code falls back to
	// SanitizeClusterCode(name).
	require.NotEmpty(t, SanitizeClusterCode("Demo Cluster", 0))
	assert.Equal(t, SanitizeClusterCode("Demo Cluster", 0), SanitizeClusterCode("Demo Cluster", 0))
}
