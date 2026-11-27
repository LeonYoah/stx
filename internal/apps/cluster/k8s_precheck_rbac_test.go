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

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/utils/secretbox"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestK8sPrecheck_ZeroSecretsAndZeroProxy_InClusterAndCredentialChecks(t *testing.T) {
	secretbox.SetSecret("test-precheck-secret")
	t.Cleanup(func() { secretbox.SetSecret("") })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Cluster{}, &ClusterNode{}, &installerapp.ContainerRegistry{}))
	encPass, err := secretbox.Encrypt("robot-secret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&installerapp.ContainerRegistry{
		Host: "harbor.corp.com", Username: "robot", Password: encPass, ProbeStatus: "ok",
	}).Error)

	// 构造零 secrets + 零 */proxy 的大厂最小权限环境。
	// Build a strict enterprise minimal RBAC mock with zero secrets and zero */proxy rules.
	client := fake.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: k8s.DefaultSeaTunnelRunnerServiceAccount, Namespace: "seatunnel-prod"},
	})
	attachMinimalRBACReactors(client, "seatunnel-prod", false)
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authv1.SelfSubjectAccessReview)
		attr := review.Spec.ResourceAttributes
		allowed := true
		if attr != nil {
			switch attr.Resource {
			case "namespaces", "roles", "rolebindings", "endpoints", "secrets":
				allowed = false
			case "serviceaccounts":
				if attr.Verb == "create" || attr.Verb == "delete" {
					allowed = false
				}
			case "pods":
				if attr.Subresource == "proxy" || attr.Verb == "delete" {
					allowed = false
				}
			case "services":
				if attr.Subresource == "proxy" {
					allowed = false
				}
			}
		}
		out := review.DeepCopy()
		out.Status.Allowed = allowed
		return true, out, nil
	})
	client.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "forbidden", nil)
	})
	client.PrependReactor("create", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "forbidden", nil)
	})

	mgr := newLifecycleTestManager(client)
	hosts := NewMockHostProvider()
	hosts.AddHost(&HostInfo{ID: 1, Name: "k8s-strict", HostType: "kubernetes", K8sNamespace: "seatunnel-prod"})
	svc := NewService(NewRepository(db), hosts, nil)
	svc.SetK8sDependencies(mgr, nil, nil)

	// 1) 已下线的 apiserver_proxy 模式必须明确阻断。
	resProxyUnsupported, err := svc.K8sPrecheck(context.Background(), &K8sPrecheckRequest{
		HostID: 1, Namespace: "seatunnel-prod", AccessMode: "apiserver_proxy",
		ClusterName: "c1", DeploymentMode: "hybrid", HybridMemory: "1Gi",
		Image: "apache/seatunnel:3.1.1", Version: "3.1.1",
	})
	require.NoError(t, err)
	assert.False(t, resProxyUnsupported.Passed)

	// 2) in_cluster 模式下，公开镜像（零 secrets + 零 proxy）全部通过。
	resInClusterPublic, err := svc.K8sPrecheck(context.Background(), &K8sPrecheckRequest{
		HostID: 1, Namespace: "seatunnel-prod", AccessMode: "in_cluster",
		ClusterName: "c1", DeploymentMode: "hybrid", HybridMemory: "1Gi",
		Image: "apache/seatunnel:3.1.1", Version: "3.1.1",
	})
	require.NoError(t, err)
	assert.True(t, resInClusterPublic.Passed)

	// 3) 未填 image_pull_secrets 但选中带凭据的 harbor.corp.com 时，因缺 secrets 权限精确阻断并给二选一指引。
	resNeedSecretPerm, err := svc.K8sPrecheck(context.Background(), &K8sPrecheckRequest{
		HostID: 1, Namespace: "seatunnel-prod", AccessMode: "in_cluster",
		ClusterName: "c1", DeploymentMode: "hybrid", HybridMemory: "1Gi",
		Image: "harbor.corp.com/team/seatunnel:3.1.1", Version: "3.1.1",
	})
	require.NoError(t, err)
	assert.False(t, resNeedSecretPerm.Passed)

	// 4) 手动指定已有 image_pull_secrets 后，不代建 Secret；Secrets.Get 403 降级为 warning，预检通过！
	resManualSecret, err := svc.K8sPrecheck(context.Background(), &K8sPrecheckRequest{
		HostID: 1, Namespace: "seatunnel-prod", AccessMode: "in_cluster",
		ClusterName: "c1", DeploymentMode: "hybrid", HybridMemory: "1Gi",
		Image: "harbor.corp.com/team/seatunnel:3.1.1", Version: "3.1.1",
		ImagePullSecrets: "precreated-harbor-secret",
	})
	require.NoError(t, err)
	assert.True(t, resManualSecret.Passed)

	// 5) 未登记私仓（路线 A）不阻断，只给 warning。
	resUnregistered, err := svc.K8sPrecheck(context.Background(), &K8sPrecheckRequest{
		HostID: 1, Namespace: "seatunnel-prod", AccessMode: "in_cluster",
		ClusterName: "c1", DeploymentMode: "hybrid", HybridMemory: "1Gi",
		Image: "ghcr.io/custom/seatunnel:3.1.1", Version: "3.1.1",
	})
	require.NoError(t, err)
	assert.True(t, resUnregistered.Passed)

	// 6) ensureK8sRegistryCredential：手填 Secret 优先跳过代建；无手填且遇到 403 返回 ErrRegistrySecretForbidden。
	cfgSkip := &k8s.SessionConfig{
		HostID: 1, Namespace: "seatunnel-prod", Image: "harbor.corp.com/team/seatunnel:3.1.1",
		ImagePullSecrets: []string{"precreated-harbor-secret"},
	}
	require.NoError(t, svc.ensureK8sRegistryCredential(context.Background(), cfgSkip, &Cluster{ClusterCode: "c1"}))
	assert.Equal(t, []string{"precreated-harbor-secret"}, cfgSkip.ImagePullSecrets)

	cfgForbidden := &k8s.SessionConfig{
		HostID: 1, Namespace: "seatunnel-prod", Image: "harbor.corp.com/team/seatunnel:3.1.1",
	}
	credErr := svc.ensureK8sRegistryCredential(context.Background(), cfgForbidden, &Cluster{ClusterCode: "c1"})
	require.ErrorIs(t, credErr, ErrRegistrySecretForbidden)
}
