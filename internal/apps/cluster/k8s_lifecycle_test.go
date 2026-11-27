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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"context"
	"testing"

	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestVerifyAdoptedObjectsMissing 纳管对象缺失时列出全部缺失项：
// workload/service/configmap 任一不在 K8s 中都应返回非空列表。
// TestVerifyAdoptedObjectsMissing lists every absent object when adoption targets are
// missing: any absent workload/service/configmap yields a non-empty list.

// newLifecycleTestManager 注入 fake clientset 的测试管理器。
// newLifecycleTestManager builds a manager backed by the injected fake clientset.
func newLifecycleTestManager(clientset *fake.Clientset) *k8s.ClientManager {
	loader := func(ctx context.Context, hostID uint) (*k8s.Credentials, error) {
		return &k8s.Credentials{AuthMode: k8s.AuthModeToken, APIURL: "https://127.0.0.1:8443", Token: "test"}, nil
	}
	return k8s.NewClientManagerWithFactory(loader, func(cfg *rest.Config) (kubernetes.Interface, error) {
		return clientset, nil
	})
}

func TestVerifyAdoptedObjectsMissing(t *testing.T) {
	clientset := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "seatunnel"}})
	mgr := newLifecycleTestManager(clientset)
	cfg := &k8s.SessionConfig{
		HostID:             1,
		Namespace:          "seatunnel",
		MasterWorkloadName: "demo-master",
		WorkerWorkloadName: "demo-worker",
		MasterServiceName:  "demo-master",
		ConfigMapName:      "demo-config",
	}
	missing := verifyAdoptedK8sObjectsExist(context.Background(), mgr, cfg)
	require.NotEmpty(t, missing)
	joined := ""
	for _, item := range missing {
		joined += item + "|"
	}
	assert.Contains(t, joined, "statefulset/demo-master")
	assert.Contains(t, joined, "statefulset/demo-worker")
	assert.Contains(t, joined, "service/demo-master")
	assert.Contains(t, joined, "configmap/demo-config")
}

// TestVerifyAdoptedObjectsPresent 全部对象存在时返回空缺失列表。
// TestVerifyAdoptedObjectsPresent returns an empty list when every object exists.
func TestVerifyAdoptedObjectsPresent(t *testing.T) {
	objects := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "seatunnel"}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "demo-master", Namespace: "seatunnel"}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "demo-worker", Namespace: "seatunnel"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "demo-master", Namespace: "seatunnel"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo-config", Namespace: "seatunnel"}},
	}
	clientset := fake.NewSimpleClientset(objects...)
	mgr := newLifecycleTestManager(clientset)
	cfg := &k8s.SessionConfig{
		HostID:             1,
		Namespace:          "seatunnel",
		MasterWorkloadName: "demo-master",
		WorkerWorkloadName: "demo-worker",
		MasterServiceName:  "demo-master",
		ConfigMapName:      "demo-config",
	}
	missing := verifyAdoptedK8sObjectsExist(context.Background(), mgr, cfg)
	assert.Empty(t, missing)
}

// TestVerifyAdoptedObjectsSingleNamespaceForbidden 验证单命名空间最小权限下
// Namespaces().Get 返回 403 Forbidden 时，仍可继续验证并纳管命名空间内的现有对象。
// TestVerifyAdoptedObjectsSingleNamespaceForbidden verifies adoption succeeds when
// Namespaces().Get returns 403 Forbidden under single-namespace minimal RBAC.
func TestVerifyAdoptedObjectsSingleNamespaceForbidden(t *testing.T) {
	objects := []runtime.Object{
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "demo-master", Namespace: "seatunnel"}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "demo-worker", Namespace: "seatunnel"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "demo-master", Namespace: "seatunnel"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo-config", Namespace: "seatunnel"}},
	}
	clientset := fake.NewSimpleClientset(objects...)
	clientset.PrependReactor("get", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "seatunnel", nil)
	})
	mgr := newLifecycleTestManager(clientset)
	cfg := &k8s.SessionConfig{
		HostID:             1,
		Namespace:          "seatunnel",
		MasterWorkloadName: "demo-master",
		WorkerWorkloadName: "demo-worker",
		MasterServiceName:  "demo-master",
		ConfigMapName:      "demo-config",
	}
	missing := verifyAdoptedK8sObjectsExist(context.Background(), mgr, cfg)
	assert.Empty(t, missing)
}

// TestVerifyAdoptedNamespaceMissing 整个 namespace 不存在时直接报 namespace 缺失。
// TestVerifyAdoptedNamespaceMissing reports the missing namespace directly when it is absent.
func TestVerifyAdoptedNamespaceMissing(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	mgr := newLifecycleTestManager(clientset)
	cfg := &k8s.SessionConfig{HostID: 1, Namespace: "ghost-ns", MasterWorkloadName: "demo-master"}
	missing := verifyAdoptedK8sObjectsExist(context.Background(), mgr, cfg)
	require.Len(t, missing, 1)
	assert.Contains(t, missing[0], "namespace/ghost-ns")
}

// TestCleanupFailedDeployOnlyOwnedResources 回收只删带归属标签的 stx 资源，
// 用户自建同名前缀资源无标签保留。
// TestCleanupFailedDeployOnlyOwnedResources reclaims only stx-labeled objects;
// user resources without ownership labels survive.
func TestCleanupFailedDeployOnlyOwnedResources(t *testing.T) {
	ownedLabels := map[string]string{"app.kubernetes.io/managed-by": "stx", k8s.LabelClusterCodeKey: "demo"}
	objects := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "seatunnel"}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "demo-master", Namespace: "seatunnel", Labels: ownedLabels}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "demo-master", Namespace: "seatunnel", Labels: ownedLabels}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo-config", Namespace: "seatunnel", Labels: ownedLabels}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "stx-regcred-demo", Namespace: "seatunnel", Labels: ownedLabels}},
		// 用户自建资源：无归属标签，回收后必须仍在。
		// User-owned resources: no ownership labels, must survive reclamation.
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "user-master", Namespace: "seatunnel"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "user-config", Namespace: "seatunnel"}},
	}
	clientset := fake.NewSimpleClientset(objects...)
	mgr := newLifecycleTestManager(clientset)
	cfg := &k8s.SessionConfig{HostID: 1, Namespace: "seatunnel"}
	err := cleanupFailedK8sDeploy(context.Background(), mgr, cfg, "demo")
	require.NoError(t, err)

	_, err = clientset.AppsV1().StatefulSets("seatunnel").Get(context.Background(), "demo-master", metav1.GetOptions{})
	assert.Error(t, err, "owned sts must be deleted")
	_, err = clientset.CoreV1().ConfigMaps("seatunnel").Get(context.Background(), "demo-config", metav1.GetOptions{})
	assert.Error(t, err, "owned cm must be deleted")
	_, err = clientset.CoreV1().Secrets("seatunnel").Get(context.Background(), "stx-regcred-demo", metav1.GetOptions{})
	assert.Error(t, err, "owned secret must be deleted")

	_, err = clientset.AppsV1().StatefulSets("seatunnel").Get(context.Background(), "user-master", metav1.GetOptions{})
	assert.NoError(t, err, "user sts must survive")
	_, err = clientset.CoreV1().ConfigMaps("seatunnel").Get(context.Background(), "user-config", metav1.GetOptions{})
	assert.NoError(t, err, "user cm must survive")
}
