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
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
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
		AccessMode:         AccessModeInCluster,
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
	// 端口名对齐上游 application 模式约定（apache/seatunnel#13277），支持按名寻址 services/http:<svc>:<port>/proxy。
	// The port name follows the upstream application-mode convention (apache/seatunnel#13277),
	// enabling name-addressed access like services/http:<svc>:<port>/proxy.
	if masterSvc.Spec.Ports[0].Name != "http" {
		t.Fatalf("master service port name expected http, got %q", masterSvc.Spec.Ports[0].Name)
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
		serverURL, err := ParseKubeconfigServerURL(kubeconfig)
		if err != nil {
			t.Fatalf("ParseKubeconfigServerURL error: %v", err)
		}
		if serverURL != "https://k8s.example:6443" {
			t.Fatalf("unexpected extracted server url %q", serverURL)
		}
	})
	t.Run("unsupported mode including removed in_cluster", func(t *testing.T) {
		if _, err := BuildRESTConfig(&Credentials{AuthMode: "in_cluster"}); err == nil {
			t.Fatal("expected in_cluster auth mode to be rejected")
		}
		if _, err := BuildRESTConfig(&Credentials{AuthMode: "bogus"}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestTransport_baseURLs(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	ctx := context.Background()

	unsupported := NewTransport(mgr, 1, "seatunnel", AccessMode("apiserver_proxy"), "")
	if _, err := unsupported.ServiceBaseURL(ctx, "seatunnel-master", 8080); err == nil {
		t.Fatal("expected error for unsupported apiserver_proxy service url")
	}
	if _, err := unsupported.PodBaseURL(ctx, "seatunnel-master-0", "10.244.0.3", 8080); err == nil {
		t.Fatal("expected error for unsupported apiserver_proxy pod url")
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

// TestRBACRequiredRulesFollowRealModel 逐条断言核心与按需 RBAC 规则遵循真实 K8s 模型：
// 1) 核心 RBACRequiredRules 不包含 secrets、*/proxy、pods(create/watch/delete)、batch/jobs、events；
// 2) 高敏/场景化能力拆入 RBACOptionalRules（secrets 与 workload/pods delete），全表不包含 services/proxy 与 pods/proxy；
// 3) services 不含 patch。
// TestRBACRequiredRulesFollowRealModel asserts lean required rules and capability-gated
// optional rules follow the real K8s RBAC model without proxy subresources or dead permissions.
func TestRBACRequiredRulesFollowRealModel(t *testing.T) {
	expectRequired := map[string]struct {
		group       string
		resource    string
		subresource string
	}{
		"pods":                   {"", "pods", ""},
		"statefulsets":           {"apps", "statefulsets", ""},
		"deployments":            {"apps", "deployments", ""},
		"replicasets":            {"apps", "replicasets", ""},
		"services":               {"", "services", ""},
		"configmaps":             {"", "configmaps", ""},
		"serviceaccounts":        {"", "serviceaccounts", ""},
		"persistentvolumeclaims": {"", "persistentvolumeclaims", ""},
	}
	seenRequired := map[string]bool{}
	for _, rule := range RBACRequiredRules {
		key := rule.Resource
		if rule.Subresource != "" {
			key = rule.Resource + "/" + rule.Subresource
		}
		seenRequired[key] = true
		if rule.Subresource == "" {
			want, ok := expectRequired[rule.Resource]
			if !ok {
				t.Fatalf("unexpected resource %q in RBACRequiredRules", rule.Resource)
			}
			if rule.Group != want.group {
				t.Fatalf("resource %q: group=%q want %q", rule.Resource, rule.Group, want.group)
			}
			if rule.Resource == "pods" && (rule.Verb == "create" || rule.Verb == "watch" || rule.Verb == "delete") {
				t.Fatalf("RBACRequiredRules must not require pods %s", rule.Verb)
			}
		} else {
			if rule.Group != "" || key != "pods/log" {
				t.Fatalf("RBACRequiredRules unexpected subresource rule group=%q key=%q", rule.Group, key)
			}
		}
		if rule.Resource == "services" && rule.Subresource == "" && rule.Verb == "patch" {
			t.Fatal("services patch must not be required for session mode")
		}
		if rule.Verb == "delete" {
			t.Fatalf("RBACRequiredRules must be zero-delete; found %s delete", rule.Resource)
		}
	}
	if !seenRequired["pods/log"] {
		t.Fatal("missing required subresource rule pods/log")
	}

	// secrets 与全部 delete 权限留在 RBACOptionalRules 按需判定，且全表不请求 services/proxy 或 pods/proxy。
	// secrets and delete verbs live in RBACOptionalRules; proxy subresources are never requested.
	seenOptionalDelete := map[string]bool{}
	seenOptional := map[string]bool{}
	for _, rule := range RBACOptionalRules {
		key := rule.Resource
		if rule.Subresource != "" {
			key = rule.Resource + "/" + rule.Subresource
		}
		seenOptional[key] = true
		if key == "pods/proxy" || key == "services/proxy" {
			t.Fatalf("RBACOptionalRules must not include proxy subresource %q", key)
		}
		if rule.Verb == "delete" {
			seenOptionalDelete[key] = true
		}
	}
	for _, key := range []string{"statefulsets", "deployments", "services", "configmaps", "pods", "secrets"} {
		if !seenOptionalDelete[key] {
			t.Fatalf("RBACOptionalRules must include delete verb for %s", key)
		}
	}
	if !seenOptional["secrets"] {
		t.Fatal("RBACOptionalRules must include secrets")
	}
}

// TestPermissionModeZeroDeleteAndUninstallFallback 验证零 delete 最小权限可直接获判 minimal，
// 且 Uninstall 遇 403 时将 StatefulSet/Deployment 缩容至 0 并返回按标签精确清理命令。
// TestPermissionModeZeroDeleteAndUninstallFallback verifies zero-delete tokens evaluate to
// PermissionModeMinimal and Uninstall on 403 scales workloads to 0 with ManualCleanupCommand.
func TestPermissionModeZeroDeleteAndUninstallFallback(t *testing.T) {
	items := make([]RBACCheckItem, 0, len(RBACRequiredRules)+len(RBACOptionalRules))
	for _, rule := range RBACRequiredRules {
		res := rule.Resource
		if rule.Subresource != "" {
			res += "/" + rule.Subresource
		}
		items = append(items, RBACCheckItem{Resource: res, Verb: rule.Verb, Allowed: true, Scope: RBACScopeRequired})
	}
	for _, rule := range RBACOptionalRules {
		res := rule.Resource
		if rule.Subresource != "" {
			res += "/" + rule.Subresource
		}
		items = append(items, RBACCheckItem{Resource: res, Verb: rule.Verb, Allowed: false, Scope: RBACScopeOptional})
	}
	if mode := EvaluatePermissionMode(items); mode != PermissionModeMinimal {
		t.Fatalf("zero-delete required rules mode=%q, want %q", mode, PermissionModeMinimal)
	}
	if CanDeleteWorkloads(items) {
		t.Fatal("CanDeleteWorkloads must be false when delete verbs are denied")
	}

	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	ctx := context.Background()
	if _, err := sm.Deploy(ctx, &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 42}); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	for _, res := range []string{"statefulsets", "deployments", "services", "configmaps"} {
		resourceName := res
		client.PrependReactor("delete", resourceName, func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: resourceName}, "forbidden", nil)
		})
	}
	err := sm.Uninstall(ctx, cfg, "e2e")
	if err == nil || !strings.Contains(err.Error(), ManualCleanupCommand("e2e-ns", "e2e")) {
		t.Fatalf("expected uninstall error with ManualCleanupCommand, got: %v", err)
	}
	sts, getErr := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, ResourceNamesFor("e2e").MasterSTS, metav1.GetOptions{})
	if getErr != nil || sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Fatalf("expected master statefulset scaled to 0 on 403 delete, got sts=%v err=%v", sts, getErr)
	}
}

func TestDeploy_refusesForeignResources(t *testing.T) {
	// 同名但非本集群拥有的资源（无标签或 cluster-code 不匹配）必须拒绝覆盖且保持原样。
	// Same-named resources not owned by this cluster (no labels or a different
	// cluster-code) must be rejected and left untouched.
	foreignLabels := []map[string]string{
		nil,
		{LabelManagedByKey: LabelManagedByValue, LabelClusterCodeKey: "other-cluster"},
		{LabelManagedByKey: "helm"},
	}
	for idx, labels := range foreignLabels {
		t.Run(fmt.Sprintf("case-%d", idx), func(t *testing.T) {
			client := fake.NewSimpleClientset()
			mgr := newTestManager(client)
			sm := NewSessionManager(mgr)
			cfg := testSessionConfig()
			ctx := context.Background()

			foreignCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "e2e-configs", Namespace: "e2e-ns", Labels: labels},
				Data:       map[string]string{"user-key": "user-data"},
			}
			if _, err := client.CoreV1().ConfigMaps("e2e-ns").Create(ctx, foreignCM, metav1.CreateOptions{}); err != nil {
				t.Fatalf("seed foreign configmap: %v", err)
			}

			_, err := sm.Deploy(ctx, &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 42})
			if err == nil || !strings.Contains(err.Error(), "not owned by this cluster") {
				t.Fatalf("expected ownership conflict, got: %v", err)
			}
			after, getErr := client.CoreV1().ConfigMaps("e2e-ns").Get(ctx, "e2e-configs", metav1.GetOptions{})
			if getErr != nil {
				t.Fatalf("foreign configmap missing after refused deploy: %v", getErr)
			}
			if after.Data["user-key"] != "user-data" {
				t.Fatal("foreign configmap content was modified")
			}
		})
	}

	t.Run("foreign service rejected", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		mgr := newTestManager(client)
		sm := NewSessionManager(mgr)
		cfg := testSessionConfig()
		ctx := context.Background()
		foreignSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "e2e", Namespace: "e2e-ns"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9999}}},
		}
		if _, err := client.CoreV1().Services("e2e-ns").Create(ctx, foreignSvc, metav1.CreateOptions{}); err != nil {
			t.Fatalf("seed foreign service: %v", err)
		}
		_, err := sm.Deploy(ctx, &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 42})
		if err == nil || !strings.Contains(err.Error(), "not owned by this cluster") {
			t.Fatalf("expected ownership conflict, got: %v", err)
		}
		after, _ := client.CoreV1().Services("e2e-ns").Get(ctx, "e2e", metav1.GetOptions{})
		if after.Spec.Ports[0].Port != 9999 {
			t.Fatal("foreign service was modified")
		}
	})

	t.Run("foreign statefulset rejected", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		mgr := newTestManager(client)
		sm := NewSessionManager(mgr)
		cfg := testSessionConfig()
		ctx := context.Background()
		replicas := int32(7)
		foreignSTS := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "e2e-master", Namespace: "e2e-ns"},
			Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
		}
		if _, err := client.AppsV1().StatefulSets("e2e-ns").Create(ctx, foreignSTS, metav1.CreateOptions{}); err != nil {
			t.Fatalf("seed foreign statefulset: %v", err)
		}
		_, err := sm.Deploy(ctx, &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 42})
		if err == nil || !strings.Contains(err.Error(), "not owned by this cluster") {
			t.Fatalf("expected ownership conflict, got: %v", err)
		}
		after, _ := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-master", metav1.GetOptions{})
		if *after.Spec.Replicas != 7 {
			t.Fatal("foreign statefulset was modified")
		}
	})

	t.Run("foreign rbac rejected", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		mgr := newTestManager(client)
		sm := NewSessionManager(mgr)
		cfg := testSessionConfig()
		ctx := context.Background()
		foreignSA := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "e2e", Namespace: "e2e-ns"}}
		if _, err := client.CoreV1().ServiceAccounts("e2e-ns").Create(ctx, foreignSA, metav1.CreateOptions{}); err != nil {
			t.Fatalf("seed foreign sa: %v", err)
		}
		_, err := sm.Deploy(ctx, &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 42, CreateRBAC: true})
		if err == nil || !strings.Contains(err.Error(), "not owned by this cluster") {
			t.Fatalf("expected ownership conflict, got: %v", err)
		}
	})
}

func TestUninstall_skipsForeignResources(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	ctx := context.Background()

	// 先正常部署一套本集群资源，再植入同名异集群资源验证跳过语义。
	// Deploy own resources first, then plant same-named foreign resources to verify
	// the skip semantics.
	if _, err := sm.Deploy(ctx, &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 42, CreateRBAC: true}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	foreignSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "unrelated-svc", Namespace: "e2e-ns"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 1234}}},
	}
	if _, err := client.CoreV1().Services("e2e-ns").Create(ctx, foreignSvc, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed foreign service: %v", err)
	}

	if err := sm.Uninstall(ctx, cfg, "e2e"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	names := ResourceNamesFor("e2e")
	if _, err := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, names.MasterSTS, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("owned master statefulset should be deleted")
	}
	if _, err := client.CoreV1().ConfigMaps("e2e-ns").Get(ctx, names.ConfigMap, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("owned configmap should be deleted")
	}
	if _, err := client.CoreV1().Services("e2e-ns").Get(ctx, "unrelated-svc", metav1.GetOptions{}); err != nil {
		t.Fatal("foreign service must survive uninstall")
	}
	if _, err := client.CoreV1().Namespaces().Get(ctx, "e2e-ns", metav1.GetOptions{}); err != nil {
		t.Fatal("namespace must never be deleted")
	}
}

func TestUninstall_neverDeletesForeignSameNamedResources(t *testing.T) {
	// 构造恶意场景：同名资源存在但属于其他 cluster-code，Uninstall 必须跳过。
	// Adversarial case: same-named resources owned by a different cluster-code must
	// be skipped by Uninstall.
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	ctx := context.Background()
	names := ResourceNamesFor("e2e")
	foreignLabels := map[string]string{LabelManagedByKey: LabelManagedByValue, LabelClusterCodeKey: "another"}
	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: names.MasterSTS, Namespace: "e2e-ns", Labels: foreignLabels},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: names.ConfigMap, Namespace: "e2e-ns", Labels: foreignLabels},
		Data:       map[string]string{"k": "v"},
	}
	if _, err := client.AppsV1().StatefulSets("e2e-ns").Create(ctx, sts, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().ConfigMaps("e2e-ns").Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := sm.Uninstall(ctx, cfg, "e2e"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, names.MasterSTS, metav1.GetOptions{}); err != nil {
		t.Fatal("foreign statefulset of another cluster must survive")
	}
	if _, err := client.CoreV1().ConfigMaps("e2e-ns").Get(ctx, names.ConfigMap, metav1.GetOptions{}); err != nil {
		t.Fatal("foreign configmap of another cluster must survive")
	}
}

func TestSessionPods_tracesDeploymentThroughReplicaSet(t *testing.T) {
	// 纳管 Deployment 且 LabelSelector 为空时，Pod 的直接 owner 是 ReplicaSet；
	// 必须追溯到上层 Deployment 才能正确列出并标记 master/worker 角色。
	// When adopting Deployments with an empty LabelSelector, the pod's direct owner is
	// a ReplicaSet; tracing up to the owning Deployment is required for correct listing
	// and master/worker role attribution.
	client := fake.NewSimpleClientset()
	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	ctx := context.Background()

	trueVal := true
	mkChain := func(deployName, rsName, podName string) {
		deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: "e2e-ns"}}
		if _, err := client.AppsV1().Deployments("e2e-ns").Create(ctx, deploy, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rsName,
				Namespace: "e2e-ns",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: deployName, Controller: &trueVal},
				},
			},
		}
		if _, err := client.AppsV1().ReplicaSets("e2e-ns").Create(ctx, rs, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: "e2e-ns",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: rsName, Controller: &trueVal},
				},
			},
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			},
		}
		if _, err := client.CoreV1().Pods("e2e-ns").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	mkChain("e2e-master", "e2e-master-5d9c8f7b6", "e2e-master-5d9c8f7b6-abcde")
	mkChain("e2e-worker", "e2e-worker-7c4d6f8b9d", "e2e-worker-7c4d6f8b9d-fghij")

	cfg := testSessionConfig()
	cfg.ManagementSource = ManagementSourceAdopted
	cfg.LabelSelector = ""
	cfg.MasterWorkloadKind = "Deployment"
	cfg.WorkerWorkloadKind = "Deployment"
	cfg.MasterWorkloadName = "e2e-master"
	cfg.WorkerWorkloadName = "e2e-worker"

	pods, err := sm.SessionPods(ctx, cfg, "nonexistent-code")
	if err != nil {
		t.Fatalf("SessionPods: %v", err)
	}
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(pods))
	}
	roles := map[string]string{}
	for _, p := range pods {
		if p.WorkloadKind != "Deployment" {
			t.Fatalf("pod %q workload kind=%q, want Deployment", p.Name, p.WorkloadKind)
		}
		roles[p.WorkloadName] = p.Role
	}
	if roles["e2e-master"] != "master" {
		t.Fatalf("master role mismatch: %v", roles)
	}
	if roles["e2e-worker"] != "worker" {
		t.Fatalf("worker role mismatch: %v", roles)
	}
}
