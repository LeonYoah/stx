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
	"k8s.io/client-go/kubernetes/fake"
)

func fakeJarLoader(epoch string) ([]byte, error) {
	return []byte(strings.Repeat("j", 1024)), nil
}

func buildTargetWorkloadSTS() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-master", Namespace: "e2e-ns"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ImagePullSecrets:   []corev1.LocalObjectReference{{Name: "registry-secret"}},
					ServiceAccountName: "e2e",
					InitContainers: []corev1.Container{{
						Name:  "plugin-installer",
						Image: "apache/seatunnel:3.1.0",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "plugins-shared",
							MountPath: "/opt/seatunnel/connectors",
						}},
					}},
					Containers: []corev1.Container{{
						Name:  "e2e-master",
						Image: "apache/seatunnel:3.1.0",
						Env:   []corev1.EnvVar{{Name: "TZ", Value: "Asia/Shanghai"}},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "seatunnel-configs", MountPath: "/opt/seatunnel/config/seatunnel.yaml", SubPath: "seatunnel.yaml"},
							{Name: "plugins-shared", MountPath: "/opt/seatunnel/connectors"},
							{Name: "rwo-data", MountPath: "/tmp/seatunnel"},
							{Name: "rwx-shared", MountPath: "/opt/seatunnel/lib"},
							{Name: "random-volume", MountPath: "/var/other"},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "seatunnel-configs", VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "e2e-configs"}},
						}},
						{Name: "plugins-shared", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "rwo-data", VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data-e2e-master-0"},
						}},
						{Name: "rwx-shared", VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "shared-lib"},
						}},
						{Name: "random-volume", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
				},
			},
		},
	}
}

func TestEnsureProxy_clonesWorkloadWithSafetyFilters(t *testing.T) {
	rwoPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data-e2e-master-0", Namespace: "e2e-ns"},
		Spec:       corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
	}
	rwxPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-lib", Namespace: "e2e-ns"},
		Spec:       corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}},
	}
	client := fake.NewSimpleClientset(buildTargetWorkloadSTS(), rwoPVC, rwxPVC)
	mgr := newTestManager(client)
	pm := NewJavaProxyManager(mgr, fakeJarLoader)
	cfg := testSessionConfig()

	sha, err := pm.EnsureProxy(context.Background(), cfg, "e2e", 42, "v3")
	if err != nil {
		t.Fatalf("EnsureProxy error: %v", err)
	}
	if sha == "" {
		t.Fatal("expected sha256")
	}
	ctx := context.Background()

	jarCM, err := client.CoreV1().ConfigMaps("e2e-ns").Get(ctx, "stx-java-proxy-jar-e2e", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("jar configmap missing: %v", err)
	}
	if len(jarCM.BinaryData["stx-java-proxy.jar"]) != 1024 {
		t.Fatal("jar bytes mismatch")
	}
	if jarCM.Annotations["stx.seatunnel.apache.org/jar-sha256"] != sha {
		t.Fatal("sha256 annotation mismatch")
	}

	dep, err := client.AppsV1().Deployments("e2e-ns").Get(ctx, "stx-java-proxy-e2e", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("proxy deployment missing: %v", err)
	}
	if *dep.Spec.Replicas != 1 {
		t.Fatal("expected 1 replica")
	}
	container := dep.Spec.Template.Spec.Containers[0]
	if container.Image != "apache/seatunnel:3.1.0" {
		t.Fatalf("expected cloned image, got %q", container.Image)
	}
	if container.Command[0] != "java" {
		t.Fatalf("expected java command, got %v", container.Command)
	}
	joined := strings.Join(container.Command, " ")
	if !strings.Contains(joined, "-Dstx.java.proxy.port=18080") {
		t.Fatal("missing proxy port system property")
	}
	if !strings.Contains(joined, "io.github.leonyoah.stx.proxy.StxJavaProxyApplication") {
		t.Fatal("missing main class")
	}
	if !strings.Contains(joined, "/opt/stx-proxy/stx-java-proxy.jar") {
		t.Fatal("missing jar in classpath")
	}
	if len(dep.Spec.Template.Spec.ImagePullSecrets) != 1 || dep.Spec.Template.Spec.ImagePullSecrets[0].Name != "registry-secret" {
		t.Fatal("imagePullSecrets not cloned")
	}
	if dep.Spec.Template.Spec.ServiceAccountName != "e2e" {
		t.Fatal("service account not cloned")
	}
	if len(container.Env) != 1 || container.Env[0].Name != "TZ" {
		t.Fatal("env not cloned")
	}

	volumeNames := map[string]bool{}
	for _, vol := range dep.Spec.Template.Spec.Volumes {
		volumeNames[vol.Name] = true
	}
	if !volumeNames["seatunnel-configs"] {
		t.Fatal("config configmap volume should be cloned")
	}
	if !volumeNames["plugins-shared"] {
		t.Fatal("plugin emptyDir volume should be cloned")
	}
	if !volumeNames["rwx-shared"] {
		t.Fatal("RWX pvc should be cloned")
	}
	if volumeNames["rwo-data"] {
		t.Fatal("RWO pvc must be skipped")
	}
	if volumeNames["random-volume"] {
		t.Fatal("non-whitelisted volume must be skipped")
	}
	if !volumeNames["stx-java-proxy-jar"] {
		t.Fatal("jar configmap volume missing")
	}
	if len(dep.Spec.Template.Spec.InitContainers) != 1 || dep.Spec.Template.Spec.InitContainers[0].Name != "plugin-installer" {
		t.Fatal("plugin init container should be cloned")
	}

	jarMountFound := false
	for _, mount := range container.VolumeMounts {
		if mount.MountPath == "/opt/stx-proxy/stx-java-proxy.jar" && mount.SubPath == "stx-java-proxy.jar" && mount.ReadOnly {
			jarMountFound = true
		}
	}
	if !jarMountFound {
		t.Fatal("jar mount missing or wrong")
	}

	svc, err := client.CoreV1().Services("e2e-ns").Get(ctx, "stx-java-proxy-e2e", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("proxy service missing: %v", err)
	}
	if svc.Spec.Ports[0].Port != 18080 {
		t.Fatalf("expected proxy service port 18080, got %d", svc.Spec.Ports[0].Port)
	}
}

func TestEnsureProxy_idempotentSameSHA(t *testing.T) {
	client := fake.NewSimpleClientset(buildTargetWorkloadSTS())
	mgr := newTestManager(client)
	pm := NewJavaProxyManager(mgr, fakeJarLoader)
	cfg := testSessionConfig()
	if _, err := pm.EnsureProxy(context.Background(), cfg, "e2e", 42, "v3"); err != nil {
		t.Fatalf("first ensure error: %v", err)
	}
	if _, err := pm.EnsureProxy(context.Background(), cfg, "e2e", 42, "v3"); err != nil {
		t.Fatalf("second ensure error: %v", err)
	}
}

func TestEnsureProxy_rejectsOversizeJar(t *testing.T) {
	client := fake.NewSimpleClientset(buildTargetWorkloadSTS())
	mgr := newTestManager(client)
	pm := NewJavaProxyManager(mgr, func(epoch string) ([]byte, error) {
		return make([]byte, 1024*1024), nil
	})
	cfg := testSessionConfig()
	if _, err := pm.EnsureProxy(context.Background(), cfg, "e2e", 42, "v3"); err == nil {
		t.Fatal("expected error for oversize jar")
	}
}

func TestDeleteProxy_removesExactlyProxyResources(t *testing.T) {
	client := fake.NewSimpleClientset(buildTargetWorkloadSTS())
	mgr := newTestManager(client)
	pm := NewJavaProxyManager(mgr, fakeJarLoader)
	cfg := testSessionConfig()
	ctx := context.Background()
	if _, err := pm.EnsureProxy(ctx, cfg, "e2e", 42, "v3"); err != nil {
		t.Fatalf("ensure error: %v", err)
	}
	if err := pm.DeleteProxy(ctx, cfg, "e2e"); err != nil {
		t.Fatalf("delete error: %v", err)
	}
	if _, err := client.AppsV1().Deployments("e2e-ns").Get(ctx, "stx-java-proxy-e2e", metav1.GetOptions{}); err == nil {
		t.Fatal("proxy deployment should be deleted")
	}
	if _, err := client.CoreV1().Services("e2e-ns").Get(ctx, "stx-java-proxy-e2e", metav1.GetOptions{}); err == nil {
		t.Fatal("proxy service should be deleted")
	}
	if _, err := client.CoreV1().ConfigMaps("e2e-ns").Get(ctx, "stx-java-proxy-jar-e2e", metav1.GetOptions{}); err == nil {
		t.Fatal("jar configmap should be deleted")
	}
	if _, err := client.AppsV1().StatefulSets("e2e-ns").Get(ctx, "e2e-master", metav1.GetOptions{}); err != nil {
		t.Fatal("user workload must stay untouched")
	}
}

func TestSplitJVMOpts(t *testing.T) {
	parts := splitJVMOpts("-Xms64m -Xmx512m   -XX:+UseG1GC")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %v", parts)
	}
	if len(splitJVMOpts("")) != 0 {
		t.Fatal("empty opts should yield no parts")
	}
}

// TestCloneProxyVolumesDropsDanglingMounts 覆盖 RWO PVC 跳过与 volumeClaimTemplate 场景：
// 对应 mount 必须被反向过滤，不得悬空。
// TestCloneProxyVolumesDropsDanglingMounts covers RWO PVC skipping and volumeClaimTemplate
// cases: their mounts must be back-filtered instead of dangling.
func TestCloneProxyVolumesDropsDanglingMounts(t *testing.T) {
	clientset := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data-rwo", Namespace: "ns"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	})
	template := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "seatunnel",
			VolumeMounts: []corev1.VolumeMount{
				{Name: "plugins", MountPath: "/opt/seatunnel/plugins/custom"},
				{Name: "data", MountPath: "/opt/seatunnel/data"},
				{Name: "checkpoint", MountPath: "/tmp/seatunnel"},
			},
		}},
		Volumes: []corev1.Volume{
			{Name: "plugins", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data-rwo"}}},
			// checkpoint 对应 volumeClaimTemplate，Volumes 中无此卷（StatefulSet 场景）。
			// checkpoint belongs to a volumeClaimTemplate and never appears in Volumes.
		},
	}
	volumes, mounts, _ := cloneProxyVolumes(context.Background(), clientset, "ns", template)
	volNames := map[string]bool{}
	for _, v := range volumes {
		volNames[v.Name] = true
	}
	if !volNames["plugins"] {
		t.Fatal("emptyDir plugins volume must be cloned")
	}
	if volNames["data"] {
		t.Fatal("RWO pvc volume must be skipped")
	}
	for _, m := range mounts {
		if !volNames[m.Name] {
			t.Fatalf("dangling mount %q survived filtering", m.Name)
		}
	}
	if len(mounts) != 1 || mounts[0].Name != "plugins" {
		t.Fatalf("expected only plugins mount to survive, got %+v", mounts)
	}
}

// TestDeriveProxyContainerResources 验证规格示例：Xmx 推导 limits/requests 换算与对齐。
// TestDeriveProxyContainerResources verifies the spec examples of the Xmx-derived
// limits/requests conversion and alignment.
func TestDeriveProxyContainerResources(t *testing.T) {
	cases := []struct {
		jvmOpts   string
		wantLimit string
		wantReq   string
	}{
		{"-Xmx512m", "1Gi", "256Mi"},
		{"-Xmx1024m", "1536Mi", "256Mi"},
		{"-Xmx1024m -Xms128m", "1536Mi", "320Mi"},
		{"-Xmx2048m -Xms256m", "2560Mi", "448Mi"},
		{"-Xmx4096m", "5Gi", "256Mi"},
		{"-Xmx6g", "7680Mi", "256Mi"},
		{"", "1Gi", "256Mi"},                      // 未指定堆：512Mi 基准 / unspecified heap: 512Mi baseline
		{"-Xmx1G", "1536Mi", "256Mi"},             // 大写单位 / uppercase unit
		{"-Xmx512m -Xmx1024m", "1536Mi", "256Mi"}, // 后者覆盖 / last wins
		{"-Xmxabc", "1Gi", "256Mi"},               // 非法值回退基准 / invalid falls back to baseline
	}
	for _, tc := range cases {
		res := deriveProxyContainerResources(tc.jvmOpts)
		limit := res.Limits.Memory().String()
		req := res.Requests.Memory().String()
		if limit != tc.wantLimit || req != tc.wantReq {
			t.Errorf("jvmOpts %q: limit=%s req=%s, want limit=%s req=%s", tc.jvmOpts, limit, req, tc.wantLimit, tc.wantReq)
		}
		if res.Requests.Cpu().String() != "100m" || res.Limits.Cpu().String() != "1" {
			t.Errorf("jvmOpts %q: unexpected cpu %+v", tc.jvmOpts, res)
		}
	}
}
