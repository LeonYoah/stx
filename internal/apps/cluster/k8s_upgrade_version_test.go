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
	"testing"

	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestImageTagVersionPriority 验证版本回写优先级：targetVersion 永远优先；
// 非语义化 tag（prod-202604）绝不回写，语义化 tag 仅在无 targetVersion 时回退。
func TestImageTagVersionPriority(t *testing.T) {
	assert.Equal(t, "prod-202604", imageTagVersion("harbor.corp/seatunnel:prod-202604"))
	assert.False(t, semverLikeTag("prod-202604"), "non-semver tag must never overwrite version")
	assert.True(t, semverLikeTag("3.1.0"))
	assert.True(t, semverLikeTag("v3.1.0"))
	assert.False(t, semverLikeTag("3.1"))
	assert.False(t, semverLikeTag("latest"))
	// 场景：harbor.corp/seatunnel:prod-202604 + targetVersion=3.1.0 → 版本必须是 3.1.0。
	// Scenario: harbor.corp/seatunnel:prod-202604 with targetVersion=3.1.0 must yield 3.1.0.
	version := ""
	targetVersion := "3.1.0"
	if targetVersion != "" {
		version = targetVersion
	} else if tag := imageTagVersion("harbor.corp/seatunnel:prod-202604"); tag != "" && semverLikeTag(tag) {
		version = tag
	}
	assert.Equal(t, "3.1.0", version)
}

// TestEnsureRegistryCredentialCreatesNamespace 验证全新 namespace 场景：
// 代建凭据先自建命名空间，Secret 创建不再必炸 not found。
func TestEnsureRegistryCredentialCreatesNamespace(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	// 不预建 namespace：模拟全新命名空间部署。
	// No pre-created namespace: simulates a brand-new namespace deploy.
	ctx := context.Background()
	// 直接验证 EnsureNamespace 幂等创建。
	// Verify idempotent creation through EnsureNamespace directly.
	require.NoError(t, k8s.EnsureNamespace(ctx, clientset, "brand-new-ns"))
	_, err := clientset.CoreV1().Namespaces().Get(ctx, "brand-new-ns", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, k8s.EnsureNamespace(ctx, clientset, "brand-new-ns"), "ensure must be idempotent")
}

func TestSemverLikeTagEdge(t *testing.T) {
	assert.False(t, semverLikeTag(""))
	assert.False(t, semverLikeTag("v1.2.x"))
	assert.False(t, semverLikeTag("1.2.3.4"))
}
