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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func newFakeK8sManager(client kubernetes.Interface) *k8s.ClientManager {
	return k8s.NewClientManagerWithFactory(
		func(_ context.Context, _ uint) (*k8s.Credentials, error) {
			return &k8s.Credentials{AuthMode: k8s.AuthModeToken, APIURL: "https://unused", Token: "t"}, nil
		},
		func(_ *rest.Config) (kubernetes.Interface, error) {
			return client, nil
		},
	)
}

func newLifecycleCluster() *Cluster {
	return &Cluster{
		Name:        "lifecycle",
		ClusterCode: "demo",
		Status:      ClusterStatusRunning,
		InfraType:   string(infra.TypeK8sSession),
		Config: ClusterConfig{
			"kubernetes": map[string]interface{}{
				"host_id":              1,
				"namespace":            "seatunnel",
				"access_mode":          "apiserver_proxy",
				"management_source":    "stx_deployed",
				"image":                "apache/seatunnel:3.1.0",
				"master_replicas":      1,
				"worker_replicas":      2,
				"master_workload_name": "demo-master",
				"worker_workload_name": "demo-worker",
			},
		},
	}
}

// TestK8sLifecycleStopSnapshotsAndScalesZero 验证 Stop 快照实时副本后缩 0、Start 恢复快照。
// TestK8sLifecycleStopSnapshotsAndScalesZero verifies Stop snapshots live replicas before
// scaling to zero and Start restores the snapshot.
func TestK8sLifecycleStopSnapshotsAndScalesZero(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	// worker 实时 3 与持久化 2 不同：模拟外部 kubectl scale / live worker 3 differs from the
	// persisted 2: simulates an external kubectl scale.
	for _, w := range []struct {
		name string
		n    int32
	}{{"demo-master", 1}, {"demo-worker", 3}} {
		n := w.n
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: w.name, Namespace: "seatunnel"},
			Spec:       appsv1.StatefulSetSpec{Replicas: &n},
		}
		if _, err := client.AppsV1().StatefulSets("seatunnel").Create(ctx, sts, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	db, cleanup := setupServiceTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	svc := &Service{k8sManager: newFakeK8sManager(client), repo: repo}

	cluster := newLifecycleCluster()
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create: %v", err)
	}
	cluster, err := repo.GetByID(ctx, cluster.ID, true)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	// Stop：快照实时副本 1+3 并缩 0 / Stop: snapshot live 1+3 then scale to zero
	res, err := svc.executeK8sLifecycleOperation(ctx, cluster, OperationStop)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !res.Success {
		t.Fatalf("stop result not success: %+v", res)
	}
	for _, w := range []string{"demo-master", "demo-worker"} {
		sts, err := client.AppsV1().StatefulSets("seatunnel").Get(ctx, w, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if *sts.Spec.Replicas != 0 {
			t.Fatalf("%s replicas after stop = %d, want 0", w, *sts.Spec.Replicas)
		}
	}
	reloaded, err := repo.GetByID(ctx, cluster.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := reloaded.K8sSessionConfig()
	if cfg == nil {
		t.Fatal("k8s cfg missing after stop")
	}
	if cfg.WorkerReplicas != 3 {
		t.Fatalf("snapshot worker replicas = %d, want 3 (live value)", cfg.WorkerReplicas)
	}
	if reloaded.Status != ClusterStatusStopped {
		t.Fatalf("status = %s, want stopped", reloaded.Status)
	}

	// Start：恢复实时快照 3 而非持久化旧值 2 / Start: restore live snapshot 3, not the stale persisted 2
	reloadedNodes, err := repo.GetByID(ctx, cluster.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.executeK8sLifecycleOperation(ctx, reloadedNodes, OperationStart); err != nil {
		t.Fatalf("start: %v", err)
	}
	sts, err := client.AppsV1().StatefulSets("seatunnel").Get(ctx, "demo-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if *sts.Spec.Replicas != 3 {
		t.Fatalf("worker replicas after start = %d, want 3", *sts.Spec.Replicas)
	}
	reloaded2, _ := repo.GetByID(ctx, cluster.ID, false)
	if reloaded2.Status != ClusterStatusRunning {
		t.Fatalf("status = %s, want running", reloaded2.Status)
	}
}

// TestK8sLifecycleRestartRollsWorkloads 验证 Restart 触发滚动重启注解且集群回 Running。
// TestK8sLifecycleRestartRollsWorkloads verifies Restart patches the rollout-restart
// annotation and the cluster returns to Running.
func TestK8sLifecycleRestartRollsWorkloads(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	for _, w := range []string{"demo-master", "demo-worker"} {
		n := int32(1)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: w, Namespace: "seatunnel"},
			Spec:       appsv1.StatefulSetSpec{Replicas: &n},
		}
		if _, err := client.AppsV1().StatefulSets("seatunnel").Create(ctx, sts, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	svc := &Service{k8sManager: newFakeK8sManager(client), repo: repo}
	cluster := newLifecycleCluster()
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	cluster, _ = repo.GetByID(ctx, cluster.ID, true)
	if _, err := svc.executeK8sLifecycleOperation(ctx, cluster, OperationRestart); err != nil {
		t.Fatalf("restart: %v", err)
	}
	found := false
	for _, w := range []string{"demo-master", "demo-worker"} {
		sts, err := client.AppsV1().StatefulSets("seatunnel").Get(ctx, w, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if sts.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("no workload carries the restartedAt annotation after restart")
	}
}

// TestImageTagVersion 验证镜像 Tag 解析：digest/latest 不算具体版本。
// TestImageTagVersion verifies image tag parsing: digest/latest do not count as versions.
func TestImageTagVersion(t *testing.T) {
	cases := map[string]string{
		"apache/seatunnel:3.1.0":                "3.1.0",
		"apache/seatunnel:latest":               "",
		"apache/seatunnel@sha256:abc":           "",
		"registry.example.com:5000/seatunnel:2": "2",
		"apache/seatunnel":                      "",
	}
	for image, want := range cases {
		if got := imageTagVersion(image); got != want {
			t.Fatalf("imageTagVersion(%q) = %q, want %q", image, got, want)
		}
	}
}

// TestDeleteK8sPodOwnershipGuard 验证仅可删除本集群引擎 Pod：成员 Pod 删除成功，
// 同命名空间的无关 Pod 被拒绝且仍然存在。
// TestDeleteK8sPodOwnershipGuard verifies only engine pods of this cluster can be
// deleted: member pods are deleted while foreign pods in the namespace are rejected
// and left intact.
func TestDeleteK8sPodOwnershipGuard(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	mkPod := func(name string, labels map[string]string) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "seatunnel", Labels: labels},
		}
		if _, err := client.CoreV1().Pods("seatunnel").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	mkPod("demo-master-0", map[string]string{
		"stx.seatunnel.apache.org/cluster-code": "demo",
		"stx.seatunnel.apache.org/component":    "master",
	})
	mkPod("foreign-pod", map[string]string{"app": "other"})

	db, cleanup := setupServiceTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	svc := &Service{k8sManager: newFakeK8sManager(client), repo: repo}

	cluster := newLifecycleCluster()
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create: %v", err)
	}
	cluster, err := repo.GetByID(ctx, cluster.ID, true)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	// 成员 Pod：删除成功 / member pod: deletion succeeds
	if err := svc.DeleteK8sPod(ctx, cluster, "demo-master-0"); err != nil {
		t.Fatalf("delete member pod: %v", err)
	}
	if _, err := client.CoreV1().Pods("seatunnel").Get(ctx, "demo-master-0", metav1.GetOptions{}); err == nil {
		t.Fatal("member pod still exists after delete")
	}

	// 外部 Pod：拒绝删除且仍存在 / foreign pod: rejected and intact
	if err := svc.DeleteK8sPod(ctx, cluster, "foreign-pod"); err == nil {
		t.Fatal("expected ownership rejection for foreign pod")
	}
	if _, err := client.CoreV1().Pods("seatunnel").Get(ctx, "foreign-pod", metav1.GetOptions{}); err != nil {
		t.Fatal("foreign pod must not be deleted")
	}
}
