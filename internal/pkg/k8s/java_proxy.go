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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// 伴生 Proxy 相关常量：镜像内 SeaTunnel 安装根目录与代理 jar 挂载路径是启动命令契约的一部分。
// Companion proxy constants: the in-image SeaTunnel home and the proxy jar mount path are
// part of the launch command contract.
const (
	proxyJarMountPath  = "/opt/stx-proxy/stx-java-proxy.jar"
	proxyJarFileKey    = "stx-java-proxy.jar"
	proxyJarCMNameTmpl = "stx-java-proxy-jar-%s"
	proxySHAAnnotation = "stx.seatunnel.apache.org/jar-sha256"
	proxyHealthPath    = "/healthz"
	proxyMainClass     = "io.github.leonyoah.stx.proxy.StxJavaProxyApplication"
	proxyStartupCP     = "/opt/seatunnel/lib/*:/opt/seatunnel/starter/seatunnel-starter.jar:/opt/stx-proxy/stx-java-proxy.jar"
	seaTunnelHomeInPod = "/opt/seatunnel"
)

// ProxyJarLoader 按代际加载本地 stx-java-proxy jar 字节；由上层注入（读 lib/ 目录），
// 便于单测用假字节替换。
// ProxyJarLoader loads the local stx-java-proxy jar bytes by epoch; injected by upper layers
// (reading lib/) so tests can substitute fake bytes.
type ProxyJarLoader func(epoch string) ([]byte, error)

// JavaProxyManager 伴生 stx-java-proxy 托管器：jar ConfigMap + 同镜像伴生 Deployment/Service。
// JavaProxyManager hosts the companion stx-java-proxy: jar ConfigMap plus same-image companion
// Deployment/Service.
type JavaProxyManager struct {
	manager   *ClientManager
	jarLoader ProxyJarLoader
}

// NewJavaProxyManager 创建伴生 Proxy 管理器。
// NewJavaProxyManager creates a companion proxy manager.
func NewJavaProxyManager(manager *ClientManager, jarLoader ProxyJarLoader) *JavaProxyManager {
	return &JavaProxyManager{manager: manager, jarLoader: jarLoader}
}

// ProxyResourceNames 伴生 Proxy 的资源命名。
// ProxyResourceNames is the resource naming of the companion proxy.
func ProxyResourceNames(clusterCode string) (deployment, service, jarConfigMap string) {
	return "stx-java-proxy-" + clusterCode, "stx-java-proxy-" + clusterCode, fmt.Sprintf(proxyJarCMNameTmpl, clusterCode)
}

// EnsureProxy 幂等确保伴生 Proxy 就绪：写入 jar ConfigMap（binaryData + sha256 注解）、
// 克隆目标工作负载创建/更新伴生 Deployment 与 Service。
// EnsureProxy idempotently ensures the companion proxy: writes the jar ConfigMap
// (binaryData + sha256 annotation) and creates/updates the cloned companion
// Deployment and Service.
func (p *JavaProxyManager) EnsureProxy(ctx context.Context, cfg *SessionConfig, clusterCode string, clusterID uint, epoch string) (sha256Hex string, err error) {
	if p.jarLoader == nil {
		return "", fmt.Errorf("k8s: proxy jar loader is not configured")
	}
	jarBytes, err := p.jarLoader(epoch)
	if err != nil {
		return "", fmt.Errorf("k8s: load proxy jar failed: %w", err)
	}
	if len(jarBytes) == 0 || len(jarBytes) > 900*1024 {
		return "", fmt.Errorf("k8s: proxy jar size %d bytes exceeds the ConfigMap binaryData budget", len(jarBytes))
	}
	sum := sha256.Sum256(jarBytes)
	sha256Hex = hex.EncodeToString(sum[:])

	clientset, err := p.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return "", err
	}
	ns := cfg.Namespace
	deploymentName, serviceName, jarCMName := ProxyResourceNames(clusterCode)
	if cfg.JavaProxy.DeploymentName != "" {
		deploymentName = cfg.JavaProxy.DeploymentName
	}
	if cfg.JavaProxy.ServiceName != "" {
		serviceName = cfg.JavaProxy.ServiceName
	}
	labels := sessionLabels(clusterCode, clusterID, "stx-java-proxy")

	if err := p.ensureJarConfigMap(ctx, clientset, ns, jarCMName, jarBytes, sha256Hex, labels); err != nil {
		return "", err
	}

	template, err := p.resolveTargetPodTemplate(ctx, clientset, cfg)
	if err != nil {
		return "", err
	}
	if err := p.ensureProxyDeployment(ctx, clientset, cfg, deploymentName, jarCMName, template, labels); err != nil {
		return "", err
	}
	if err := p.ensureProxyService(ctx, clientset, cfg, serviceName, labels); err != nil {
		return "", err
	}
	return sha256Hex, nil
}

func (p *JavaProxyManager) ensureJarConfigMap(ctx context.Context, clientset kubernetes.Interface, ns, name string, jarBytes []byte, sha256Hex string, labels map[string]string) error {
	annotations := map[string]string{proxySHAAnnotation: sha256Hex}
	existing, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels, Annotations: annotations},
			BinaryData: map[string][]byte{proxyJarFileKey: jarBytes},
		}
		if _, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("k8s: create proxy jar configmap failed: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("k8s: get proxy jar configmap failed: %w", err)
	}
	// 内容指纹一致时跳过写入，避免无意义滚动。
	// Skip the write when the fingerprint matches to avoid pointless churn.
	if existing.Annotations[proxySHAAnnotation] == sha256Hex && len(existing.BinaryData[proxyJarFileKey]) == len(jarBytes) {
		return nil
	}
	if !isOwnedByCluster(existing.Labels, labels[LabelClusterCodeKey]) {
		return errResourceConflict("configmap", name)
	}
	if existing.BinaryData == nil {
		existing.BinaryData = map[string][]byte{}
	}
	existing.BinaryData[proxyJarFileKey] = jarBytes
	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}
	existing.Annotations[proxySHAAnnotation] = sha256Hex
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for k, v := range labels {
		existing.Labels[k] = v
	}
	if _, err := clientset.CoreV1().ConfigMaps(ns).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("k8s: update proxy jar configmap failed: %w", err)
	}
	return nil
}

// resolveTargetPodTemplate 读取目标 master（优先）或 worker 工作负载的 Pod 模板用于克隆。
// resolveTargetPodTemplate reads the target master (preferred) or worker workload pod
// template for cloning.
func (p *JavaProxyManager) resolveTargetPodTemplate(ctx context.Context, clientset kubernetes.Interface, cfg *SessionConfig) (*corev1.PodSpec, error) {
	for _, target := range []struct{ kind, name string }{
		{cfg.MasterWorkloadKind, cfg.MasterWorkloadName},
		{cfg.WorkerWorkloadKind, cfg.WorkerWorkloadName},
	} {
		if target.name == "" {
			continue
		}
		switch target.kind {
		case "StatefulSet", "":
			sts, err := clientset.AppsV1().StatefulSets(cfg.Namespace).Get(ctx, target.name, metav1.GetOptions{})
			if err == nil {
				spec := sts.Spec.Template.Spec.DeepCopy()
				return spec, nil
			}
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("k8s: get statefulset %q failed: %w", target.name, err)
			}
		case "Deployment":
			dep, err := clientset.AppsV1().Deployments(cfg.Namespace).Get(ctx, target.name, metav1.GetOptions{})
			if err == nil {
				spec := dep.Spec.Template.Spec.DeepCopy()
				return spec, nil
			}
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("k8s: get deployment %q failed: %w", target.name, err)
			}
		}
	}
	return nil, fmt.Errorf("k8s: no target workload found for proxy cloning in namespace %q", cfg.Namespace)
}

// 插件与配置类挂载路径白名单：只有这些路径下的卷才需要被伴生 Proxy 继承。
// Whitelisted mount path prefixes: only volumes under these paths are inherited by the
// companion proxy.
var proxyCloneMountPrefixes = []string{
	"/opt/seatunnel/connectors",
	"/opt/seatunnel/plugins",
	"/opt/seatunnel/lib",
	"/opt/seatunnel/config",
	"/opt/seatunnel/starter",
	"/tmp/seatunnel",
}

func isProxyCloneablePath(mountPath string) bool {
	for _, prefix := range proxyCloneMountPrefixes {
		if mountPath == prefix || strings.HasPrefix(mountPath, prefix+"/") {
			return true
		}
	}
	return false
}

// cloneProxyVolumes 按白名单+安全过滤克隆卷与挂载：
// 克隆 ConfigMap/Secret/emptyDir（含对应插件 initContainers）与 RWX/ROX PVC；
// 跳过 StatefulSet 单 Pod RWO 数据卷，避免跨节点挂卷死锁。
// cloneProxyVolumes clones volumes and mounts by whitelist plus safety filters:
// ConfigMap/Secret/emptyDir (with their plugin initContainers) and RWX/ROX PVCs are cloned;
// single-pod RWO data PVCs of StatefulSets are skipped to avoid cross-node mount deadlocks.
func cloneProxyVolumes(ctx context.Context, clientset kubernetes.Interface, ns string, template *corev1.PodSpec) (volumes []corev1.Volume, mounts []corev1.VolumeMount, initContainers []corev1.Container) {
	if template == nil || len(template.Containers) == 0 {
		return nil, nil, nil
	}
	primary := template.Containers[0]

	// 收集白名单挂载及其卷名。
	// Collect whitelisted mounts and their volume names.
	neededVolumes := map[string]bool{}
	for _, mount := range primary.VolumeMounts {
		if isProxyCloneablePath(mount.MountPath) {
			mounts = append(mounts, mount)
			neededVolumes[mount.Name] = true
		}
	}

	for _, vol := range template.Volumes {
		if !neededVolumes[vol.Name] {
			continue
		}
		switch {
		case vol.ConfigMap != nil || vol.Secret != nil || vol.EmptyDir != nil:
			volumes = append(volumes, vol)
		case vol.PersistentVolumeClaim != nil:
			if pvcSharable(ctx, clientset, ns, vol.PersistentVolumeClaim.ClaimName) {
				volumes = append(volumes, vol)
			}
		}
	}

	clonedVolumeNames := map[string]bool{}
	for _, vol := range volumes {
		clonedVolumeNames[vol.Name] = true
	}
	// 反向过滤悬空挂载：RWO PVC 被跳过或 volumeClaimTemplate 卷根本不在 template.Volumes 中时，
	// 其 mount 在 Proxy Pod 里无对应卷，必须剔除否则 Pod 创建即失败。
	// Back-filter dangling mounts: when an RWO PVC is skipped or a volumeClaimTemplate volume
	// never appears in template.Volumes, its mount has no backing volume in the proxy pod and
	// must be dropped or pod creation fails outright.
	keptMounts := mounts[:0]
	for _, mount := range mounts {
		if clonedVolumeNames[mount.Name] {
			keptMounts = append(keptMounts, mount)
		}
	}
	mounts = keptMounts
	// 只有引用了被克隆卷的 initContainers 才需要复制（典型场景：插件安装器把 jar 写进共享 emptyDir）。
	// Only initContainers referencing cloned volumes are copied (the typical case is a plugin
	// installer writing jars into a shared emptyDir).
	for _, init := range template.InitContainers {
		used := false
		for _, mount := range init.VolumeMounts {
			if clonedVolumeNames[mount.Name] {
				used = true
				break
			}
		}
		if used {
			initContainers = append(initContainers, init)
		}
	}
	return volumes, mounts, initContainers
}

// pvcSharable 判断 PVC 是否可跨 Pod 共享挂载（accessModes 含 RWX/ROX）；查询失败时保守跳过。
// pvcSharable reports whether a PVC can be shared across pods (access modes include RWX/ROX);
// lookup failures conservatively skip the volume.
func pvcSharable(ctx context.Context, clientset kubernetes.Interface, ns, claimName string) bool {
	pvc, err := clientset.CoreV1().PersistentVolumeClaims(ns).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		return false
	}
	for _, mode := range pvc.Spec.AccessModes {
		if mode == corev1.ReadWriteMany || mode == corev1.ReadOnlyMany {
			return true
		}
	}
	return false
}

func splitJVMOpts(opts string) []string {
	parts := []string{}
	for _, piece := range strings.Fields(opts) {
		parts = append(parts, piece)
	}
	return parts
}

// parseJVMMemoryFlagMi 提取 jvmOpts 中最后一个有效的 -Xmx/-Xms 堆内存（单位换算 MiB，大小写不敏感）。
// 取最后一个是因为 JVM 语义上后出现的参数覆盖先出现的。
// parseJVMMemoryFlagMi extracts the last valid -Xmx/-Xms heap value in MiB
// (case-insensitive); the last one wins because later JVM flags override earlier ones.
func parseJVMMemoryFlagMi(jvmOpts, flag string) int {
	value := 0
	lower := strings.ToLower(flag)
	for _, piece := range strings.Fields(jvmOpts) {
		p := strings.ToLower(strings.TrimSpace(piece))
		if !strings.HasPrefix(p, lower) {
			continue
		}
		raw := p[len(lower):]
		if raw == "" {
			continue
		}
		unit := float64(1)
		last := raw[len(raw)-1]
		num := raw
		switch last {
		case 'k':
			unit = 1.0 / 1024
			num = raw[:len(raw)-1]
		case 'm':
			num = raw[:len(raw)-1]
		case 'g':
			unit = 1024
			num = raw[:len(raw)-1]
		}
		n, err := strconv.ParseFloat(num, 64)
		if err != nil || n <= 0 {
			continue
		}
		value = int(math.Ceil(n * unit))
	}
	return value
}

func hasJVMFlag(jvmOpts, substr string) bool {
	lower := strings.ToLower(substr)
	for _, piece := range strings.Fields(jvmOpts) {
		if strings.Contains(strings.ToLower(piece), lower) {
			return true
		}
	}
	return false
}

func alignUpMi(v, align int) int {
	if v%align == 0 {
		return v
	}
	return (v/align + 1) * align
}

func formatMemoryMi(mi int) string {
	if mi%1024 == 0 {
		return fmt.Sprintf("%dGi", mi/1024)
	}
	return fmt.Sprintf("%dMi", mi)
}

// deriveProxyContainerResources 按 jvm_opts 推导 Proxy 容器内存配额：
// 未指定 -Xmx 以 512Mi 为基准堆；limits = xmx + max(512, xmx*0.25)（堆外/Metaspace/线程栈余量）向上 128Mi 对齐；
// requests = min(limit, max(256, xms+192))，xms 缺省 64Mi。CPU 固定 100m/1 保证 ResourceQuota 命名空间可调度。
// deriveProxyContainerResources derives the proxy container memory budget from jvm_opts:
// the heap baseline is 512Mi without -Xmx; limits = xmx + max(512, xmx*0.25) (off-heap,
// Metaspace and thread-stack headroom) aligned up to 128Mi; requests = min(limit,
// max(256, xms+192)) with xms defaulting to 64Mi. CPU is fixed at 100m/1 so namespaces
// with ResourceQuota can schedule the pod.
func deriveProxyContainerResources(jvmOpts string) corev1.ResourceRequirements {
	xmxMi := parseJVMMemoryFlagMi(jvmOpts, "-xmx")
	if xmxMi <= 0 {
		xmxMi = 512
	}
	xmsMi := parseJVMMemoryFlagMi(jvmOpts, "-xms")
	if xmsMi <= 0 {
		xmsMi = 64
	}
	if xmsMi > xmxMi {
		xmsMi = xmxMi
	}
	overheadMi := int(math.Ceil(float64(xmxMi) * 0.25))
	if overheadMi < 512 {
		overheadMi = 512
	}
	limitMi := alignUpMi(xmxMi+overheadMi, 128)
	reqMi := xmsMi + 192
	if reqMi < 256 {
		reqMi = 256
	}
	reqMi = alignUpMi(reqMi, 64)
	if reqMi > limitMi {
		reqMi = limitMi
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(formatMemoryMi(reqMi)),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(formatMemoryMi(limitMi)),
			corev1.ResourceCPU:    resource.MustParse("1"),
		},
	}
}

func (p *JavaProxyManager) ensureProxyDeployment(ctx context.Context, clientset kubernetes.Interface, cfg *SessionConfig, deploymentName, jarCMName string, template *corev1.PodSpec, labels map[string]string) error {
	volumes, mounts, initContainers := cloneProxyVolumes(ctx, clientset, cfg.Namespace, template)

	volumes = append(volumes, corev1.Volume{
		Name: "stx-java-proxy-jar",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: jarCMName},
				DefaultMode:          int32ptr(0644),
			},
		},
	})
	mounts = append(mounts, corev1.VolumeMount{
		Name:      "stx-java-proxy-jar",
		MountPath: proxyJarMountPath,
		SubPath:   proxyJarFileKey,
		ReadOnly:  true,
	})

	image := cfg.Image
	pullSecrets := []corev1.LocalObjectReference{}
	var env []corev1.EnvVar
	var envFrom []corev1.EnvFromSource
	serviceAccount := cfg.ServiceAccount
	if template != nil {
		if len(template.Containers) > 0 && template.Containers[0].Image != "" {
			image = template.Containers[0].Image
		}
	}
	// 纳管模式允许 cfg.Image 为空，完全依赖目标 Workload 模板提取；两边都空说明目标不可克隆。
	// Adopted mode permits an empty cfg.Image and relies on the target workload
	// template; when both are empty the target is not cloneable.
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("k8s: cannot resolve proxy image from config or target workload template")
	}
	if template != nil {
		pullSecrets = template.ImagePullSecrets
		if len(template.Containers) > 0 {
			env = template.Containers[0].Env
			envFrom = template.Containers[0].EnvFrom
		}
		if template.ServiceAccountName != "" {
			serviceAccount = template.ServiceAccountName
		}
	}

	javaArgs := splitJVMOpts(cfg.JavaProxy.JVMOpts)
	// 未显式指定堆上限时，用 MaxRAMPercentage 让 JVM 按容器 cgroup 限制自适应，
	// 防止 JVM 默认 1/4 物理内存（看到的是宿主机）撑爆容器。
	// Without an explicit heap cap, MaxRAMPercentage lets the JVM adapt to the container
	// cgroup limit instead of defaulting to a fraction of the host memory it observes.
	if parseJVMMemoryFlagMi(cfg.JavaProxy.JVMOpts, "-xmx") <= 0 && !hasJVMFlag(cfg.JavaProxy.JVMOpts, "maxrampercentage") {
		javaArgs = append(javaArgs, "-XX:MaxRAMPercentage=70.0")
	}
	javaArgs = append(javaArgs,
		fmt.Sprintf("-Dstx.java.proxy.port=%d", cfg.JavaProxy.Port),
		"-DSEATUNNEL_HOME="+seaTunnelHomeInPod,
		"-Dstx.java.proxy.seatunnel.home="+seaTunnelHomeInPod,
		"-cp", proxyStartupCP,
		proxyMainClass,
	)

	container := corev1.Container{
		Name:            "stx-java-proxy",
		Image:           image,
		ImagePullPolicy: corev1.PullPolicy(cfg.ImagePullPolicy),
		Command:         append([]string{"java"}, javaArgs...),
		Ports:           []corev1.ContainerPort{{Name: "http", ContainerPort: int32(cfg.JavaProxy.Port)}},
		VolumeMounts:    mounts,
		Env:             env,
		EnvFrom:         envFrom,
		Resources:       deriveProxyContainerResources(cfg.JavaProxy.JVMOpts),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: proxyHealthPath,
					Port: intstr.FromInt32(int32(cfg.JavaProxy.Port)),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    6,
		},
	}

	selectorLabels := map[string]string{
		LabelClusterCodeKey: labels[LabelClusterCodeKey],
		LabelComponentKey:   "stx-java-proxy",
	}
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccount,
					ImagePullSecrets:   pullSecrets,
					InitContainers:     initContainers,
					Containers:         []corev1.Container{container},
					Volumes:            volumes,
				},
			},
		},
	}

	existing, err := clientset.AppsV1().Deployments(cfg.Namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := clientset.AppsV1().Deployments(cfg.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("k8s: create proxy deployment failed: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("k8s: get proxy deployment failed: %w", err)
	}
	if !isOwnedByCluster(existing.Labels, labels[LabelClusterCodeKey]) {
		return errResourceConflict("deployment", deploymentName)
	}
	existing.Spec.Template = deployment.Spec.Template
	existing.Spec.Replicas = deployment.Spec.Replicas
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for k, v := range labels {
		existing.Labels[k] = v
	}
	if _, err := clientset.AppsV1().Deployments(cfg.Namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("k8s: update proxy deployment failed: %w", err)
	}
	return nil
}

func (p *JavaProxyManager) ensureProxyService(ctx context.Context, clientset kubernetes.Interface, cfg *SessionConfig, serviceName string, labels map[string]string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: cfg.Namespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				LabelClusterCodeKey: labels[LabelClusterCodeKey],
				LabelComponentKey:   "stx-java-proxy",
			},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: int32(cfg.JavaProxy.Port), TargetPort: intstr.FromInt32(int32(cfg.JavaProxy.Port))},
			},
		},
	}
	return upsertService(ctx, clientset, cfg.Namespace, svc)
}

// DeleteProxy 删除伴生 Proxy 的 Deployment/Service/jar ConfigMap；adopted 集群解绑时也只清这些。
// DeleteProxy removes the companion proxy Deployment/Service/jar ConfigMap; unbinding an
// adopted cluster cleans exactly these and nothing else.
func (p *JavaProxyManager) DeleteProxy(ctx context.Context, cfg *SessionConfig, clusterCode string) error {
	clientset, err := p.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	ns := cfg.Namespace
	deploymentName, serviceName, jarCMName := ProxyResourceNames(clusterCode)
	if cfg.JavaProxy.DeploymentName != "" {
		deploymentName = cfg.JavaProxy.DeploymentName
	}
	if cfg.JavaProxy.ServiceName != "" {
		serviceName = cfg.JavaProxy.ServiceName
	}
	var errs []string
	deleteOne := func(kind string, del func() error) {
		if err := del(); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Sprintf("%s: %v", kind, err))
		}
	}
	policy := metav1.DeletePropagationBackground
	deleteOne("deployment/"+deploymentName, func() error {
		obj, err := clientset.AppsV1().Deployments(ns).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.AppsV1().Deployments(ns).Delete(ctx, deploymentName, metav1.DeleteOptions{PropagationPolicy: &policy})
	})
	deleteOne("service/"+serviceName, func() error {
		obj, err := clientset.CoreV1().Services(ns).Get(ctx, serviceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().Services(ns).Delete(ctx, serviceName, metav1.DeleteOptions{})
	})
	deleteOne("configmap/"+jarCMName, func() error {
		obj, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx, jarCMName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().ConfigMaps(ns).Delete(ctx, jarCMName, metav1.DeleteOptions{})
	})
	if len(errs) > 0 {
		return fmt.Errorf("k8s: delete proxy partially failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ProxyStatus 伴生 Proxy 运行状态摘要。
// ProxyStatus summarizes the companion proxy runtime state.
type ProxyStatus struct {
	DeploymentName string   `json:"deployment_name"`
	ServiceName    string   `json:"service_name"`
	Exists         bool     `json:"exists"`
	Ready          bool     `json:"ready"`
	ReadyReplicas  int32    `json:"ready_replicas"`
	PodEvents      []string `json:"pod_events"`
}

// GetProxyStatus 读取伴生 Proxy 状态与 Pod 事件（失败时展示原因）。
// GetProxyStatus reads companion proxy state and pod events (surfacing causes on failure).
func (p *JavaProxyManager) GetProxyStatus(ctx context.Context, cfg *SessionConfig, clusterCode string) (*ProxyStatus, error) {
	clientset, err := p.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return nil, err
	}
	ns := cfg.Namespace
	deploymentName, serviceName, _ := ProxyResourceNames(clusterCode)
	if cfg.JavaProxy.DeploymentName != "" {
		deploymentName = cfg.JavaProxy.DeploymentName
	}
	if cfg.JavaProxy.ServiceName != "" {
		serviceName = cfg.JavaProxy.ServiceName
	}
	status := &ProxyStatus{DeploymentName: deploymentName, ServiceName: serviceName, PodEvents: []string{}}
	dep, err := clientset.AppsV1().Deployments(ns).Get(ctx, deploymentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return status, nil
	}
	if err != nil {
		return nil, fmt.Errorf("k8s: get proxy deployment failed: %w", err)
	}
	status.Exists = true
	status.ReadyReplicas = dep.Status.ReadyReplicas
	status.Ready = dep.Status.ReadyReplicas >= 1

	selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels).String()
	pods, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err == nil {
		for i := range pods.Items {
			pod := &pods.Items[i]
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue && cond.Reason != "" {
					status.PodEvents = append(status.PodEvents, fmt.Sprintf("%s: %s %s", pod.Name, cond.Reason, cond.Message))
				}
			}
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
					status.PodEvents = append(status.PodEvents, fmt.Sprintf("%s: %s %s", pod.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message))
				}
			}
		}
	}
	return status, nil
}

// resolveProxyDeploymentName 解析伴生 Proxy Deployment 名称（配置优先，默认确定性命名）。
// resolveProxyDeploymentName resolves the companion proxy deployment name (config first,
// deterministic default otherwise).
func resolveProxyDeploymentName(cfg *SessionConfig, clusterCode string) string {
	deploymentName, _, _ := ProxyResourceNames(clusterCode)
	if cfg.JavaProxy.DeploymentName != "" {
		deploymentName = cfg.JavaProxy.DeploymentName
	}
	return deploymentName
}

// StopProxy 将伴生 Proxy Deployment 副本缩为 0；保留 Deployment/Service/jar ConfigMap 以便快速拉起。
// StopProxy scales the companion proxy deployment to zero replicas while keeping the
// Deployment/Service/jar ConfigMap so a later start comes up quickly.
func (p *JavaProxyManager) StopProxy(ctx context.Context, cfg *SessionConfig, clusterCode string) error {
	clientset, err := p.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	deploymentName := resolveProxyDeploymentName(cfg, clusterCode)
	dep, err := clientset.AppsV1().Deployments(cfg.Namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("k8s: get proxy deployment failed: %w", err)
	}
	if !isOwnedByCluster(dep.Labels, clusterCode) {
		return errResourceConflict("deployment", deploymentName)
	}
	patch := []byte(`{"spec":{"replicas":0}}`)
	if _, err := clientset.AppsV1().Deployments(cfg.Namespace).Patch(ctx, deploymentName, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("k8s: stop proxy deployment failed: %w", err)
	}
	return nil
}

// RestartProxy 先 EnsureProxy 收敛 jar/配置/副本为 1，再打 restartedAt 注解强制滚动重建：
// 仅改 jvm_opts 时 PodSpec 可能无差异，无注解则不会触发重建。
// RestartProxy first converges jar/config/replicas via EnsureProxy, then stamps the
// restartedAt annotation to force a rollout: a jvm_opts-only change may leave the PodSpec
// unchanged, which would otherwise never restart the pod.
func (p *JavaProxyManager) RestartProxy(ctx context.Context, cfg *SessionConfig, clusterCode string, clusterID uint, epoch string) error {
	if _, err := p.EnsureProxy(ctx, cfg, clusterCode, clusterID, epoch); err != nil {
		return err
	}
	clientset, err := p.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	deploymentName := resolveProxyDeploymentName(cfg, clusterCode)
	restartedAt := time.Now().UTC().Format(time.RFC3339)
	patch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`, restartedAt))
	if _, err := clientset.AppsV1().Deployments(cfg.Namespace).Patch(ctx, deploymentName, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("k8s: restart proxy deployment failed: %w", err)
	}
	return nil
}

// GetProxyLogs 拉取活跃伴生 Proxy Pod 的最近日志；优先选 Running 中最新的 Pod。
// GetProxyLogs fetches recent logs of the active companion proxy pod, preferring the
// newest Running pod.
func (p *JavaProxyManager) GetProxyLogs(ctx context.Context, cfg *SessionConfig, clusterCode string, tailLines int64) (string, string, error) {
	clientset, err := p.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return "", "", err
	}
	deploymentName := resolveProxyDeploymentName(cfg, clusterCode)
	dep, err := clientset.AppsV1().Deployments(cfg.Namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("k8s: get proxy deployment failed: %w", err)
	}
	selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels).String()
	pods, err := clientset.CoreV1().Pods(cfg.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", "", fmt.Errorf("k8s: list proxy pods failed: %w", err)
	}
	if len(pods.Items) == 0 {
		return "", "", fmt.Errorf("k8s: no proxy pod found")
	}
	best := -1
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if best < 0 || pod.CreationTimestamp.After(pods.Items[best].CreationTimestamp.Time) {
			best = i
		}
	}
	if best < 0 {
		best = 0
	}
	podName := pods.Items[best].Name
	if tailLines <= 0 {
		tailLines = 200
	}
	raw, err := clientset.CoreV1().Pods(cfg.Namespace).GetLogs(podName, &corev1.PodLogOptions{TailLines: &tailLines}).Do(ctx).Raw()
	if err != nil {
		return "", "", fmt.Errorf("k8s: read proxy pod logs failed: %w", err)
	}
	return podName, string(raw), nil
}
