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
	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func attachMinimalRBACReactors(client *fake.Clientset, ns string, denyRequired bool) {
	client.PrependReactor("get", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, ns, nil)
	})
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		review, ok := createAction.GetObject().(*authv1.SelfSubjectAccessReview)
		if !ok {
			return false, nil, nil
		}
		attr := review.Spec.ResourceAttributes
		allowed := true
		if attr != nil {
			switch attr.Resource {
			case "namespaces":
				allowed = false
			case "serviceaccounts":
				if attr.Verb == "create" || attr.Verb == "delete" {
					allowed = false
				}
			case "roles", "rolebindings", "endpoints":
				allowed = false
			case "statefulsets":
				if denyRequired && attr.Verb == "create" {
					allowed = false
				}
			}
		}
		out := review.DeepCopy()
		out.Status.Allowed = allowed
		return true, out, nil
	})
}

// TestK8sPrecheck_SingleNamespaceMinimalRBAC 验证单命名空间最小权限下的 precheck 与 SA 自动推导：
// 1) namespaces Get 403 且扩展 RBAC 均 denied 时，api_connectivity 与 rbac 仍判定通过；
// 2) 核心权限（如 statefulsets/create）缺失时精确阻断；
// 3) resolveK8sServiceAccountWithRBAC 按 host default -> seatunnel-runner 优先级推导。
// TestK8sPrecheck_SingleNamespaceMinimalRBAC validates precheck and SA inference under single-NS minimal RBAC:
// 1) api_connectivity and rbac pass when Namespaces().Get returns 403 and extended rules are denied;
// 2) missing required rules block deployment;
// 3) resolveK8sServiceAccountWithRBAC falls back host default -> seatunnel-runner -> default.
func TestK8sPrecheck_SingleNamespaceMinimalRBAC(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: k8s.DefaultSeaTunnelRunnerServiceAccount, Namespace: "seatunnel-prod"},
	})
	attachMinimalRBACReactors(client, "seatunnel-prod", false)

	mgr := newLifecycleTestManager(client)
	hosts := NewMockHostProvider()
	hosts.AddHost(&HostInfo{ID: 1, Name: "k8s-min", HostType: "kubernetes", K8sNamespace: "seatunnel-prod"})
	svc := NewService(nil, hosts, nil)
	svc.SetK8sDependencies(mgr, nil, nil)

	res, err := svc.K8sPrecheck(context.Background(), &K8sPrecheckRequest{
		HostID:         1,
		Namespace:      "seatunnel-prod",
		ClusterName:    "min-cluster",
		DeploymentMode: "hybrid",
		HybridMemory:   "1Gi",
	})
	require.NoError(t, err)
	assert.True(t, res.Passed)

	itemsByName := map[string]*K8sPrecheckItem{}
	for _, it := range res.Items {
		itemsByName[it.Name] = it
	}
	require.NotNil(t, itemsByName["api_connectivity"])
	assert.True(t, itemsByName["api_connectivity"].Passed)
	require.NotNil(t, itemsByName["rbac"])
	assert.True(t, itemsByName["rbac"].Passed)
	assert.Contains(t, itemsByName["rbac"].Message, k8s.DefaultSeaTunnelRunnerServiceAccount)

	sa, createRBAC := svc.resolveK8sServiceAccountWithRBAC(context.Background(), 1, "seatunnel-prod", "", nil)
	assert.False(t, createRBAC)
	assert.Equal(t, k8s.DefaultSeaTunnelRunnerServiceAccount, sa)

	// Host 指定了自定义默认 SA 时，主机默认优先于自动探测。
	// Host-configured default SA takes precedence over auto-detected seatunnel-runner.
	hosts.AddHost(&HostInfo{
		ID:                       2,
		Name:                     "k8s-custom-sa",
		HostType:                 "kubernetes",
		K8sNamespace:             "seatunnel-prod",
		K8sDefaultServiceAccount: "custom-runner",
	})
	sa2, createRBAC2 := svc.resolveK8sServiceAccountWithRBAC(context.Background(), 2, "seatunnel-prod", "", nil)
	assert.False(t, createRBAC2)
	assert.Equal(t, "custom-runner", sa2)

	// referenced_objects 校验 custom-runner 缺失时阻断。
	// Missing host-configured default SA blocks precheck at referenced_objects.
	resMissing, err := svc.K8sPrecheck(context.Background(), &K8sPrecheckRequest{
		HostID:         2,
		Namespace:      "seatunnel-prod",
		ClusterName:    "min-cluster",
		DeploymentMode: "hybrid",
		HybridMemory:   "1Gi",
	})
	require.NoError(t, err)
	assert.False(t, resMissing.Passed)

	// 缺少核心 required 规则时必须阻断。
	// Missing a required rule must block deployment.
	clientDeny := fake.NewSimpleClientset()
	attachMinimalRBACReactors(clientDeny, "seatunnel-prod", true)
	mgrDeny := newLifecycleTestManager(clientDeny)
	svcDeny := NewService(nil, hosts, nil)
	svcDeny.SetK8sDependencies(mgrDeny, nil, nil)
	resDeny, err := svcDeny.K8sPrecheck(context.Background(), &K8sPrecheckRequest{
		HostID:         1,
		Namespace:      "seatunnel-prod",
		ClusterName:    "min-cluster",
		DeploymentMode: "hybrid",
		HybridMemory:   "1Gi",
	})
	require.NoError(t, err)
	assert.False(t, resDeny.Passed)
}
