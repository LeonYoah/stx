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

	"github.com/LeonYoah/stx/internal/pkg/infra"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newProxyK8sCluster(t *testing.T, repo *Repository, javaProxy map[string]interface{}) *Cluster {
	t.Helper()
	k8sCfg := map[string]interface{}{
		"host_id":           1,
		"namespace":         "seatunnel",
		"access_mode":       "apiserver_proxy",
		"management_source": "stx_deployed",
		"image":             "apache/seatunnel:3.1.0",
		"master_replicas":   1,
		"worker_replicas":   1,
	}
	if javaProxy != nil {
		k8sCfg["java_proxy"] = javaProxy
	}
	cluster := &Cluster{
		Name:        "proxy-k8s",
		ClusterCode: "demo",
		Version:     "3.1.0",
		Status:      ClusterStatusRunning,
		InfraType:   string(infra.TypeK8sSession),
		Config:      ClusterConfig{"kubernetes": k8sCfg},
	}
	if err := repo.Create(context.Background(), cluster); err != nil {
		t.Fatalf("create: %v", err)
	}
	return cluster
}

// TestK8sSTXJavaProxyStatusMapping 验证 K8s Proxy 状态映射的关键契约：
// Role 必须为空串（前端会翻译非空 role）、PID 为 0、HostName 为 namespace/deployment。
// TestK8sSTXJavaProxyStatusMapping verifies the key K8s proxy status contract:
// Role must be empty (the frontend translates non-empty roles), PID is zero and
// HostName is namespace/deployment.
func TestK8sSTXJavaProxyStatusMapping(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stx-java-proxy-demo",
			Namespace: "seatunnel",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":          "stx",
				"stx.seatunnel.apache.org/cluster-code": "demo",
				"stx.seatunnel.apache.org/component":    "stx-java-proxy",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"stx.seatunnel.apache.org/cluster-code": "demo",
				"stx.seatunnel.apache.org/component":    "stx-java-proxy",
			}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}
	if _, err := client.AppsV1().Deployments("seatunnel").Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	db, cleanup := setupServiceTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	svc := &Service{k8sManager: newFakeK8sManager(client), repo: repo}
	cluster := newProxyK8sCluster(t, repo, nil)

	status, err := svc.GetSTXJavaProxyStatus(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status == nil {
		t.Fatal("status nil")
	}
	if status.Role != "" {
		t.Fatalf("Role = %q, want empty (frontend translates non-empty roles)", status.Role)
	}
	if status.PID != 0 {
		t.Fatalf("PID = %d, want 0", status.PID)
	}
	if status.HostName != "seatunnel/stx-java-proxy-demo" {
		t.Fatalf("HostName = %q, want seatunnel/stx-java-proxy-demo", status.HostName)
	}
	if !status.Managed {
		t.Fatal("Managed should be true in managed mode")
	}
	if !status.Running {
		t.Fatal("Running should be true when deployment has replicas")
	}
}

// TestK8sSTXJavaProxyStopScalesZero 验证 Stop 将 Deployment 缩容到 0 且保留对象。
// TestK8sSTXJavaProxyStopScalesZero verifies Stop scales the deployment to zero while
// keeping the object around.
func TestK8sSTXJavaProxyStopScalesZero(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stx-java-proxy-demo",
			Namespace: "seatunnel",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":          "stx",
				"stx.seatunnel.apache.org/cluster-code": "demo",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
		},
	}
	if _, err := client.AppsV1().Deployments("seatunnel").Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	db, cleanup := setupServiceTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	svc := &Service{k8sManager: newFakeK8sManager(client), repo: repo}
	cluster := newProxyK8sCluster(t, repo, nil)

	if _, err := svc.StopSTXJavaProxy(ctx, cluster.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	got, err := client.AppsV1().Deployments("seatunnel").Get(ctx, "stx-java-proxy-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("deployment must be kept after stop: %v", err)
	}
	if *got.Spec.Replicas != 0 {
		t.Fatalf("replicas = %d, want 0", *got.Spec.Replicas)
	}
}

// TestK8sSTXJavaProxyDisabledMode 验证 disabled 模式的状态契约。
// TestK8sSTXJavaProxyDisabledMode verifies the disabled-mode status contract.
func TestK8sSTXJavaProxyDisabledMode(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	svc := &Service{k8sManager: newFakeK8sManager(client), repo: repo}
	cluster := newProxyK8sCluster(t, repo, map[string]interface{}{"mode": "disabled"})

	status, err := svc.GetSTXJavaProxyStatus(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Running || status.Healthy {
		t.Fatalf("disabled proxy must not be running/healthy: %+v", status)
	}
	if status.Managed {
		t.Fatal("disabled proxy must not report managed")
	}
}

// TestK8sProxyConfigWritesBothLocations 验证 UpdateSTXJavaProxyConfig 同时写两个配置位置。
// TestK8sProxyConfigWritesBothLocations verifies UpdateSTXJavaProxyConfig persists both
// config locations.
func TestK8sProxyConfigWritesBothLocations(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	svc := &Service{k8sManager: newFakeK8sManager(client), repo: repo}
	cluster := newProxyK8sCluster(t, repo, nil)

	jvm := "-Xmx1g"
	port := 18081
	restart := false
	if _, err := svc.UpdateSTXJavaProxyConfig(ctx, cluster.ID, &UpdateSTXJavaProxyConfigRequest{
		JvmOpts: &jvm,
		Port:    &port,
		Restart: &restart,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	reloaded, err := repo.GetByID(ctx, cluster.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	proxyRaw, ok := reloaded.Config["stx_java_proxy"].(map[string]interface{})
	if !ok {
		t.Fatal("stx_java_proxy config missing")
	}
	if proxyRaw["jvm_opts"] != "-Xmx1g" {
		t.Fatalf("stx_java_proxy.jvm_opts = %v", proxyRaw["jvm_opts"])
	}
	k8sRaw, ok := reloaded.Config["kubernetes"].(map[string]interface{})
	if !ok {
		t.Fatal("kubernetes config missing")
	}
	jpRaw, ok := k8sRaw["java_proxy"].(map[string]interface{})
	if !ok {
		t.Fatal("kubernetes.java_proxy missing")
	}
	if jpRaw["jvm_opts"] != "-Xmx1g" {
		t.Fatalf("kubernetes.java_proxy.jvm_opts = %v", jpRaw["jvm_opts"])
	}
	if jpRaw["port"] != 18081 && jpRaw["port"] != float64(18081) {
		t.Fatalf("kubernetes.java_proxy.port = %v", jpRaw["port"])
	}
}

var _ = k8s.JavaProxyModeManaged
