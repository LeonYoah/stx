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

package k8s

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func newTestManager(fakeClient *fake.Clientset) *ClientManager {
	loader := func(ctx context.Context, hostID uint) (*Credentials, error) {
		return &Credentials{
			AuthMode: AuthModeToken,
			APIURL:   "https://127.0.0.1:8443",
			Token:    "test-token",
		}, nil
	}
	return NewClientManagerWithFactory(loader, func(cfg *rest.Config) (kubernetes.Interface, error) {
		return fakeClient, nil
	})
}

func testSessionConfig() *SessionConfig {
	cfg := &SessionConfig{
		HostID:             1,
		Namespace:          "e2e-ns",
		ManagementSource:   ManagementSourceSTXDeployed,
		AccessMode:         AccessModeAPIServerProxy,
		Image:              "apache/seatunnel:3.1.0",
		MasterWorkloadName: "e2e-master",
		WorkerWorkloadName: "e2e-worker",
		ConfigMapName:      "e2e-configs",
	}
	cfg.ApplyDefaults()
	cfg.Namespace = "e2e-ns"
	cfg.MasterWorkloadName = "e2e-master"
	cfg.WorkerWorkloadName = "e2e-worker"
	cfg.ConfigMapName = "e2e-configs"
	cfg.ManagementSource = ManagementSourceSTXDeployed
	return cfg
}

func TestDeploy_createsFullResourceSet(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()

	result, err := sm.Deploy(context.Background(), &DeployOptions{
		Config:      cfg,
		ClusterCode: "e2e",
		ClusterID:   42,
		CreateRBAC:  true,
	})
	if err != nil {
		t.Fatalf("Deploy error: %v", err)
	}
	if len(result.CreatedResources) == 0 {
		t.Fatal("expected created resources")
	}

	ctx := context.Background()
	if _, err := client.CoreV1().Namespaces().Get(ctx, "e2e-ns", metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
	sa, err := client.CoreV1().ServiceAccounts("e2e-ns").Get(ctx, "e2e", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("serviceaccount not created: %v", err)
	}
	if sa.Labels[LabelManagedByKey] != LabelManagedByValue {
		t.Fatal("missing managed-by label on serviceaccount")
	}

	cm, err := client.CoreV1().ConfigMaps("e2e-ns").Get(ctx, "e2e-configs", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("configmap not created: %v", err)
	}
	for _, key := range sessionConfigMountKeys() {
		if _, ok := cm.Data[key]; !ok {
			t.Fatalf("configmap missing key %q", key)
		}
	}
	if !strings.Contains(cm.Data["seatunnel.yaml"], "port: 8080") {
		t.Fatal("seatunnel.yaml missing http port")
	}
	if !strings.Contains(cm.Data["hazelcast-master.yaml"], "service-dns: e2e.e2e-ns.svc.cluster.local") {
		t.Fatal("hazelcast master missing service-dns")
	}
	if strings.Contains(cm.Data["hazelcast-master.yaml"], "rest-api:\n      enabled: true") {
		t.Fatal("rest-api v1 must stay disabled")
	}

	clusterSvc, err := client.CoreV1().Services("e2e-ns").Get(ctx, "e2e", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("cluster service missing: %v", err)
	}
	if clusterSvc.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Fatal("cluster service must be headless")
	}
	masterSvc, err := client.CoreV1().Services("e2e-ns").Get(ctx, "e2e-master", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("master service missing: %v", err)
	}
	if masterSvc.Spec.Ports[0].Port != 8080 {
		t.Fatalf("master service port expected 8080, got %d", masterSvc.Spec.Ports[0].Port)
	}

	masterSTS, err := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-master", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("master statefulset missing: %v", err)
	}
	if *masterSTS.Spec.Replicas != 1 {
		t.Fatalf("expected 1 master replica, got %d", *masterSTS.Spec.Replicas)
	}
	if *masterSTS.Spec.Template.Spec.TerminationGracePeriodSeconds != 120 {
		t.Fatal("expected terminationGracePeriodSeconds 120")
	}
	masterContainer := masterSTS.Spec.Template.Spec.Containers[0]
	if masterContainer.Command[0] != "/opt/seatunnel/bin/seatunnel-cluster.sh" {
		t.Fatalf("unexpected master command: %v", masterContainer.Command)
	}
	if len(masterContainer.Command) != 3 {
		t.Fatal("command must exec jvm directly without shell wrapper")
	}
	if masterContainer.ReadinessProbe.HTTPGet == nil || masterContainer.ReadinessProbe.HTTPGet.Path != "/overview" {
		t.Fatal("master readiness should probe /overview")
	}

	workerSTS, err := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("worker statefulset missing: %v", err)
	}
	if *workerSTS.Spec.Replicas != 2 {
		t.Fatalf("expected 2 worker replicas, got %d", *workerSTS.Spec.Replicas)
	}
	if workerSTS.Spec.Template.Spec.Containers[0].ReadinessProbe.TCPSocket == nil {
		t.Fatal("worker readiness should use tcp probe")
	}
}

func TestDeploy_idempotentUpdate(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	opts := &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 42, CreateRBAC: true}
	if _, err := sm.Deploy(context.Background(), opts); err != nil {
		t.Fatalf("first deploy error: %v", err)
	}
	cfg.MasterReplicas = 3
	if _, err := sm.Deploy(context.Background(), opts); err != nil {
		t.Fatalf("second deploy error: %v", err)
	}
	sts, err := client.AppsV1().StatefulSets("e2e-ns").Get(context.Background(), "e2e-master", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get master sts error: %v", err)
	}
	if *sts.Spec.Replicas != 3 {
		t.Fatalf("expected updated replicas 3, got %d", *sts.Spec.Replicas)
	}
}

func TestUninstall_removesSTXResourcesButNeverPVC(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	if _, err := sm.Deploy(context.Background(), &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 42, CreateRBAC: true}); err != nil {
		t.Fatalf("deploy error: %v", err)
	}
	// 模拟外部 Checkpoint PVC（STX 从不创建也不删除）。
	// Simulate an external checkpoint PVC (STX never creates nor deletes it).
	if _, err := client.CoreV1().PersistentVolumeClaims("e2e-ns").Create(context.Background(), &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "external-checkpoint", Namespace: "e2e-ns"},
		Spec:       corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pvc error: %v", err)
	}

	if err := sm.Uninstall(context.Background(), cfg, "e2e"); err != nil {
		t.Fatalf("uninstall error: %v", err)
	}
	ctx := context.Background()
	if _, err := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-master", metav1.GetOptions{}); err == nil {
		t.Fatal("master statefulset should be deleted")
	}
	if _, err := client.CoreV1().ConfigMaps("e2e-ns").Get(ctx, "e2e-configs", metav1.GetOptions{}); err == nil {
		t.Fatal("configmap should be deleted")
	}
	if _, err := client.CoreV1().Services("e2e-ns").Get(ctx, "e2e", metav1.GetOptions{}); err == nil {
		t.Fatal("cluster service should be deleted")
	}
	if _, err := client.CoreV1().ServiceAccounts("e2e-ns").Get(ctx, "e2e", metav1.GetOptions{}); err == nil {
		t.Fatal("serviceaccount should be deleted")
	}
	if _, err := client.CoreV1().PersistentVolumeClaims("e2e-ns").Get(ctx, "external-checkpoint", metav1.GetOptions{}); err != nil {
		t.Fatal("external pvc must never be deleted")
	}
}

func TestScale_rolloutRestart_upgradeImage(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	if _, err := sm.Deploy(context.Background(), &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 1}); err != nil {
		t.Fatalf("deploy error: %v", err)
	}
	ctx := context.Background()

	if err := sm.Scale(ctx, cfg, "worker", 5); err != nil {
		t.Fatalf("scale error: %v", err)
	}
	worker, _ := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-worker", metav1.GetOptions{})
	if *worker.Spec.Replicas != 5 {
		t.Fatalf("expected 5 replicas, got %d", *worker.Spec.Replicas)
	}
	if err := sm.Scale(ctx, cfg, "worker", -1); err == nil {
		t.Fatal("expected error for negative replicas")
	}

	if err := sm.RolloutRestart(ctx, cfg, ""); err != nil {
		t.Fatalf("rollout restart error: %v", err)
	}
	master, _ := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-master", metav1.GetOptions{})
	if master.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatal("expected restart annotation on master")
	}
	worker, _ = client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-worker", metav1.GetOptions{})
	if worker.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatal("expected restart annotation on worker")
	}

	if err := sm.UpgradeImage(ctx, cfg, "apache/seatunnel:3.1.1"); err != nil {
		t.Fatalf("upgrade image error: %v", err)
	}
	master, _ = client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-master", metav1.GetOptions{})
	if master.Spec.Template.Spec.Containers[0].Image != "apache/seatunnel:3.1.1" {
		t.Fatalf("expected upgraded master image, got %q", master.Spec.Template.Spec.Containers[0].Image)
	}
	worker, _ = client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-worker", metav1.GetOptions{})
	if worker.Spec.Template.Spec.Containers[0].Image != "apache/seatunnel:3.1.1" {
		t.Fatalf("expected upgraded worker image, got %q", worker.Spec.Template.Spec.Containers[0].Image)
	}
	if err := sm.UpgradeImage(ctx, cfg, ""); err == nil {
		t.Fatal("expected error for empty image")
	}
}

func TestConfigMapReadWrite(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	if _, err := sm.Deploy(context.Background(), &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 1}); err != nil {
		t.Fatalf("deploy error: %v", err)
	}
	ctx := context.Background()

	data, err := sm.ReadConfigMap(ctx, cfg)
	if err != nil {
		t.Fatalf("read configmap error: %v", err)
	}
	if _, ok := data["seatunnel.yaml"]; !ok {
		t.Fatal("expected seatunnel.yaml in configmap")
	}

	if err := sm.WriteConfigMapFile(ctx, cfg, "seatunnel.yaml", "custom: true", false); err != nil {
		t.Fatalf("write configmap error: %v", err)
	}
	data, _ = sm.ReadConfigMap(ctx, cfg)
	if data["seatunnel.yaml"] != "custom: true" {
		t.Fatal("expected updated content")
	}
	if err := sm.WriteConfigMapFile(ctx, cfg, "", "x", false); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestWorkloadStatus_missingIsExistsFalse(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	statuses, err := sm.GetWorkloadStatus(context.Background(), cfg)
	if err != nil {
		t.Fatalf("workload status error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.Exists {
			t.Fatalf("%s should not exist", s.Role)
		}
	}
}

func TestDiscover_pairsMasterWorker(t *testing.T) {
	stsMaster := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "seatunnel-master", Namespace: "seatunnel", Labels: map[string]string{"app.kubernetes.io/instance": "seatunnel"}},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "m", Image: "apache/seatunnel:3.1.0"}},
					Volumes: []corev1.Volume{{
						Name:         "cfg",
						VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "seatunnel-configs"}}},
					}},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}
	stsWorker := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "seatunnel-worker", Namespace: "seatunnel", Labels: map[string]string{"app.kubernetes.io/instance": "seatunnel"}},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32ptr(2),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "w", Image: "apache/seatunnel:3.1.0"}}},
			},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 2},
	}
	other := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-master", Namespace: "seatunnel"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "db", Image: "mysql:8"}}},
			},
		},
	}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "seatunnel-master", Namespace: "seatunnel"}}

	client := fake.NewSimpleClientset(stsMaster, stsWorker, other, svc)
	mgr := newTestManager(client)
	discoverer := NewDiscoverer(mgr)
	found, err := discoverer.Discover(context.Background(), 1, "seatunnel")
	if err != nil {
		t.Fatalf("discover error: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 discovered cluster, got %d", len(found))
	}
	group := found[0]
	if group.BaseName != "seatunnel" {
		t.Fatalf("expected base seatunnel, got %q", group.BaseName)
	}
	if group.Master == nil || group.Master.Name != "seatunnel-master" {
		t.Fatal("master not paired")
	}
	if group.Worker == nil || group.Worker.Name != "seatunnel-worker" {
		t.Fatal("worker not paired")
	}
	if group.ConfigMapName != "seatunnel-configs" {
		t.Fatalf("expected configmap seatunnel-configs, got %q", group.ConfigMapName)
	}
	if group.LabelSelector != "app.kubernetes.io/instance=seatunnel" {
		t.Fatalf("unexpected label selector %q", group.LabelSelector)
	}
	if len(group.Services) == 0 {
		t.Fatal("expected associated services")
	}
}

func TestBuildRESTConfig_modes(t *testing.T) {
	t.Run("token mode", func(t *testing.T) {
		cfg, err := BuildRESTConfig(&Credentials{
			AuthMode: AuthModeToken,
			APIURL:   "https://api.example:6443/",
			Token:    "abc",
		})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if cfg.Host != "https://api.example:6443" {
			t.Fatalf("trailing slash not trimmed: %q", cfg.Host)
		}
		if cfg.BearerToken != "abc" {
			t.Fatal("token mismatch")
		}
	})
	t.Run("token mode requires fields", func(t *testing.T) {
		if _, err := BuildRESTConfig(&Credentials{AuthMode: AuthModeToken}); err == nil {
			t.Fatal("expected error for missing api url")
		}
		if _, err := BuildRESTConfig(&Credentials{AuthMode: AuthModeToken, APIURL: "https://x"}); err == nil {
			t.Fatal("expected error for missing token")
		}
	})
	t.Run("kubeconfig mode requires content", func(t *testing.T) {
		if _, err := BuildRESTConfig(&Credentials{AuthMode: AuthModeKubeconfig}); err == nil {
			t.Fatal("expected error for empty kubeconfig")
		}
	})
	t.Run("kubeconfig mode parses content", func(t *testing.T) {
		kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://k8s.example:6443
    insecure-skip-tls-verify: true
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    token: abc
`
		cfg, err := BuildRESTConfig(&Credentials{AuthMode: AuthModeKubeconfig, Kubeconfig: kubeconfig})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if cfg.Host != "https://k8s.example:6443" {
			t.Fatalf("unexpected host %q", cfg.Host)
		}
	})
	t.Run("unsupported mode", func(t *testing.T) {
		if _, err := BuildRESTConfig(&Credentials{AuthMode: "bogus"}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestTransport_baseURLs(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	ctx := context.Background()

	transport := NewTransport(mgr, 1, "seatunnel", AccessModeAPIServerProxy, "")
	svcURL, err := transport.ServiceBaseURL(ctx, "seatunnel-master", 8080)
	if err != nil {
		t.Fatalf("service url error: %v", err)
	}
	expected := "https://127.0.0.1:8443/api/v1/namespaces/seatunnel/services/http:seatunnel-master:8080/proxy"
	if svcURL != expected {
		t.Fatalf("unexpected service proxy url:\n got %q\nwant %q", svcURL, expected)
	}
	podURL, err := transport.PodBaseURL(ctx, "seatunnel-master-0", "10.244.0.3", 8080)
	if err != nil {
		t.Fatalf("pod url error: %v", err)
	}
	expectedPod := "https://127.0.0.1:8443/api/v1/namespaces/seatunnel/pods/http:seatunnel-master-0:8080/proxy"
	if podURL != expectedPod {
		t.Fatalf("unexpected pod proxy url:\n got %q\nwant %q", podURL, expectedPod)
	}

	inCluster := NewTransport(mgr, 1, "seatunnel", AccessModeInCluster, "")
	svcInCluster, err := inCluster.ServiceBaseURL(ctx, "seatunnel-master", 8080)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if svcInCluster != "http://seatunnel-master.seatunnel.svc.cluster.local:8080" {
		t.Fatalf("unexpected in-cluster url %q", svcInCluster)
	}
	podInCluster, err := inCluster.PodBaseURL(ctx, "p", "10.244.0.3", 8080)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if podInCluster != "http://10.244.0.3:8080" {
		t.Fatalf("unexpected in-cluster pod url %q", podInCluster)
	}

	external := NewTransport(mgr, 1, "seatunnel", AccessModeExternal, "http://192.168.49.2:30080/")
	svcExternal, err := external.ServiceBaseURL(ctx, "any", 8080)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if svcExternal != "http://192.168.49.2:30080" {
		t.Fatalf("unexpected external url %q", svcExternal)
	}
}
