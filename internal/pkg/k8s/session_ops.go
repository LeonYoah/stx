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
	"io"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// PodInfo 一个受管 Pod 的投影信息，用于节点表与健康判定。
// PodInfo is the projection of one managed pod for node tables and health checks.
type PodInfo struct {
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Ready        bool      `json:"ready"`
	Phase        string    `json:"phase"`
	PodIP        string    `json:"pod_ip"`
	NodeName     string    `json:"node_name"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	Restarts     int32     `json:"restarts"`
	WorkloadKind string    `json:"workload_kind"`
	WorkloadName string    `json:"workload_name"`
}

// ListPodsByLabel 按标签选择器列出 Pod 投影。
// ListPodsByLabel lists pod projections by label selector.
func (s *SessionManager) ListPodsByLabel(ctx context.Context, cfg *SessionConfig, labelSelector string) ([]*PodInfo, error) {
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return nil, err
	}
	list, err := clientset.CoreV1().Pods(cfg.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("k8s: list pods failed: %w", err)
	}
	infos := make([]*PodInfo, 0, len(list.Items))
	for i := range list.Items {
		infos = append(infos, podToInfo(&list.Items[i]))
	}
	return infos, nil
}

// workloadTargetForRole 解析角色对应的工作负载 kind/name；hybrid 形态只有一个混合
// 工作负载，role "master"/"worker" 与 "hybrid" 都归一到同一目标，避免误查不存在的
// 分离 StatefulSet。
// workloadTargetForRole resolves a role to its workload kind/name; hybrid topology has a
// single mixed workload, so "master"/"worker"/"hybrid" all resolve to the same target
// instead of nonexistent separated StatefulSets.
func workloadTargetForRole(cfg *SessionConfig, role string) (string, string) {
	if cfg.IsHybrid() {
		return "StatefulSet", cfg.HybridWorkloadName
	}
	if role == "master" {
		return cfg.MasterWorkloadKind, cfg.MasterWorkloadName
	}
	return cfg.WorkerWorkloadKind, cfg.WorkerWorkloadName
}

func podToInfo(pod *corev1.Pod) *PodInfo {
	info := &PodInfo{
		Name:     pod.Name,
		Phase:    string(pod.Status.Phase),
		PodIP:    pod.Status.PodIP,
		NodeName: pod.Spec.NodeName,
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			info.Ready = true
		}
	}
	if pod.Status.StartTime != nil {
		info.StartedAt = pod.Status.StartTime.Time
	}
	for _, cs := range pod.Status.ContainerStatuses {
		info.Restarts += cs.RestartCount
	}
	for _, ref := range pod.OwnerReferences {
		info.WorkloadKind = ref.Kind
		info.WorkloadName = ref.Name
		if ref.Kind == "StatefulSet" || ref.Kind == "ReplicaSet" {
			break
		}
	}
	// hybrid 必须先于 -master 判定：<code>-master-hybrid 含 "-master" 子串会被误判成 master。
	// Hybrid must be checked before -master: <code>-master-hybrid contains "-master"
	// and would be misclassified as master.
	if strings.Contains(info.WorkloadName, "-hybrid") || pod.Labels[LabelComponentKey] == "hybrid" {
		info.Role = "hybrid"
	} else if strings.Contains(info.WorkloadName, "-master") {
		info.Role = "master"
	} else if strings.Contains(info.WorkloadName, "-worker") {
		info.Role = "worker"
	}
	return info
}

// SessionPods 按集群配置同时返回 master / worker 两组 Pod 投影。
// SessionPods returns master and worker pod projections per the cluster config.
func (s *SessionManager) SessionPods(ctx context.Context, cfg *SessionConfig, clusterCode string) ([]*PodInfo, error) {
	selector := cfg.LabelSelector
	if strings.TrimSpace(selector) == "" {
		selector = LabelClusterCodeKey + "=" + clusterCode
	}
	pods, err := s.ListPodsByLabel(ctx, cfg, selector)
	if err != nil {
		return nil, err
	}
	// 引擎节点面板只应呈现引擎工作负载 Pod：STX Java Proxy 等基础设施 Pod 也带
	// cluster-code 标签，必须从选择器结果中剔除（Proxy 管控统一在运行时 Tab）。
	// The engine nodes panel must only show engine workload pods: infrastructure pods
	// like the STX Java Proxy also carry the cluster-code label and must be filtered
	// out of the selector results (proxy management lives in the runtime tab).
	engineOnly := pods[:0]
	for _, pod := range pods {
		if pod.WorkloadKind == "Deployment" && strings.HasPrefix(pod.WorkloadName, "stx-java-proxy-") {
			continue
		}
		if strings.HasPrefix(pod.Name, "stx-java-proxy-") {
			continue
		}
		engineOnly = append(engineOnly, pod)
	}
	pods = engineOnly
	// 工作负载归属必须恒为增量合并而非空列表兜底：选择器一旦命中任何 Pod（例如 proxy），
	// 纯 fallback 会把未带标签的引擎 Pod 全部掩盖。
	// Workload attribution must always merge additively instead of acting as an
	// empty-list fallback: once the selector hits any pod (e.g. the proxy), a pure
	// fallback would mask every unlabeled engine pod.
	if cfg.MasterWorkloadName != "" || cfg.WorkerWorkloadName != "" || cfg.HybridWorkloadName != "" {
		clientset, err := s.manager.Clientset(ctx, cfg.HostID)
		if err != nil {
			return nil, err
		}
		list, err := clientset.CoreV1().Pods(cfg.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("k8s: list pods failed: %w", err)
		}
		// Deployment 集群中 Pod 的直接 owner 是 ReplicaSet（<deploy>-<hash>），
		// 必须先建立 ReplicaSet→Deployment 映射才能正确归属。
		// In Deployment clusters the pod's direct owner is a ReplicaSet (<deploy>-<hash>),
		// so a ReplicaSet→Deployment map is required for correct attribution.
		rsToDeploy := map[string]string{}
		rsList, rsErr := clientset.AppsV1().ReplicaSets(cfg.Namespace).List(ctx, metav1.ListOptions{})
		if rsErr == nil {
			for i := range rsList.Items {
				rs := &rsList.Items[i]
				for _, ref := range rs.OwnerReferences {
					if ref.Kind == "Deployment" {
						rsToDeploy[rs.Name] = ref.Name
						break
					}
				}
			}
		}
		seen := map[string]bool{}
		for _, pod := range pods {
			seen[pod.Name] = true
		}
		for i := range list.Items {
			pod := &list.Items[i]
			info := podToInfo(pod)
			if info.WorkloadKind == "ReplicaSet" {
				if deployName, ok := rsToDeploy[info.WorkloadName]; ok {
					info.WorkloadKind = "Deployment"
					info.WorkloadName = deployName
				}
			}
			if (info.WorkloadName == cfg.MasterWorkloadName || info.WorkloadName == cfg.WorkerWorkloadName || (cfg.HybridWorkloadName != "" && info.WorkloadName == cfg.HybridWorkloadName)) && !seen[info.Name] {
				pods = append(pods, info)
				seen[info.Name] = true
			}
		}
	}
	for _, pod := range pods {
		if cfg.HybridWorkloadName != "" && pod.WorkloadName == cfg.HybridWorkloadName {
			pod.Role = "hybrid"
		}
		if pod.WorkloadName == cfg.MasterWorkloadName {
			pod.Role = "master"
		}
		if pod.WorkloadName == cfg.WorkerWorkloadName {
			pod.Role = "worker"
		}
	}
	sort.Slice(pods, func(i, j int) bool { return pods[i].Name < pods[j].Name })
	return pods, nil
}

// Scale 调整 master/worker 工作负载副本数（仅 StatefulSet/Deployment）。
// Scale adjusts master/worker workload replicas (StatefulSet/Deployment only).
func (s *SessionManager) Scale(ctx context.Context, cfg *SessionConfig, role string, replicas int) error {
	if replicas < 0 {
		return fmt.Errorf("k8s: replicas must be >= 0, got %d", replicas)
	}
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	kind, name := workloadTargetForRole(cfg, role)
	if name == "" {
		return fmt.Errorf("k8s: %s workload name is empty", role)
	}
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
	switch kind {
	case "StatefulSet", "":
		if _, err := clientset.AppsV1().StatefulSets(cfg.Namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("k8s: scale statefulset %q failed: %w", name, err)
		}
	case "Deployment":
		if _, err := clientset.AppsV1().Deployments(cfg.Namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("k8s: scale deployment %q failed: %w", name, err)
		}
	default:
		return fmt.Errorf("k8s: unsupported workload kind %q for scale", kind)
	}
	return nil
}

// GetReplicas 读取工作负载当前期望副本数，用于停止/启动对称恢复。
// GetReplicas reads the current desired replicas so stop/start can restore symmetrically.
func (s *SessionManager) GetReplicas(ctx context.Context, cfg *SessionConfig, role string) (int, error) {
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return 0, err
	}
	kind, name := workloadTargetForRole(cfg, role)
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("k8s: %s workload name is empty", role)
	}
	switch kind {
	case "StatefulSet", "":
		sts, err := clientset.AppsV1().StatefulSets(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		if sts.Spec.Replicas == nil {
			return 1, nil
		}
		return int(*sts.Spec.Replicas), nil
	case "Deployment":
		dep, err := clientset.AppsV1().Deployments(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		if dep.Spec.Replicas == nil {
			return 1, nil
		}
		return int(*dep.Spec.Replicas), nil
	default:
		return 0, fmt.Errorf("k8s: unsupported workload kind %q", kind)
	}
}

// RolloutRestart 通过重启注解触发工作负载滚动重启；role 为空时同时重启 master 与 worker。
// RolloutRestart triggers a rolling restart via the restart annotation; an empty role
// restarts both master and worker.
func (s *SessionManager) RolloutRestart(ctx context.Context, cfg *SessionConfig, role string) error {
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	targets := []struct{ kind, name string }{}
	if cfg.IsHybrid() {
		// hybrid 形态只有单一工作负载：任意角色（含空）都收敛到它，避免误查不存在的分离 STS。
		// Hybrid topology has a single workload: any role (including empty) converges on
		// it instead of nonexistent separated StatefulSets.
		targets = append(targets, struct{ kind, name string }{"StatefulSet", cfg.HybridWorkloadName})
	} else {
		if role == "" || role == "master" {
			targets = append(targets, struct{ kind, name string }{cfg.MasterWorkloadKind, cfg.MasterWorkloadName})
		}
		if role == "" || role == "worker" {
			targets = append(targets, struct{ kind, name string }{cfg.WorkerWorkloadKind, cfg.WorkerWorkloadName})
		}
	}
	restartedAt := time.Now().UTC().Format(time.RFC3339)
	patch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`, restartedAt))
	for _, target := range targets {
		if target.name == "" {
			continue
		}
		switch target.kind {
		case "StatefulSet", "":
			if _, err := clientset.AppsV1().StatefulSets(cfg.Namespace).Patch(ctx, target.name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
				return fmt.Errorf("k8s: rollout restart statefulset %q failed: %w", target.name, err)
			}
		case "Deployment":
			if _, err := clientset.AppsV1().Deployments(cfg.Namespace).Patch(ctx, target.name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
				return fmt.Errorf("k8s: rollout restart deployment %q failed: %w", target.name, err)
			}
		default:
			return fmt.Errorf("k8s: unsupported workload kind %q for rollout restart", target.kind)
		}
	}
	return nil
}

// UpgradeImage 不可变镜像升级：同时 Patch master/worker 容器镜像，K8s 原生执行 RollingUpdate；
// 伴生 Proxy 的同步滚动由调用方追加 PatchProxyImage 完成。
// UpgradeImage patches master/worker container images for immutable-tag upgrades and lets
// K8s perform the RollingUpdate; the companion proxy rollout is done separately via
// PatchProxyImage by the caller.
func (s *SessionManager) UpgradeImage(ctx context.Context, cfg *SessionConfig, image string) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("k8s: image is required")
	}
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	upgradeTargets := []struct{ kind, name string }{
		{cfg.MasterWorkloadKind, cfg.MasterWorkloadName},
		{cfg.WorkerWorkloadKind, cfg.WorkerWorkloadName},
	}
	if cfg.IsHybrid() {
		upgradeTargets = []struct{ kind, name string }{{"StatefulSet", cfg.HybridWorkloadName}}
	}
	for _, target := range upgradeTargets {
		if target.name == "" {
			continue
		}
		if err := patchWorkloadImage(ctx, clientset, cfg.Namespace, target.kind, target.name, image); err != nil {
			return err
		}
	}
	return nil
}

// PatchProxyImage 同步滚动伴生 Proxy Deployment 的镜像。
// PatchProxyImage rolls the companion proxy Deployment image in lockstep.
func (s *SessionManager) PatchProxyImage(ctx context.Context, cfg *SessionConfig, deploymentName, image string) error {
	if deploymentName == "" {
		return nil
	}
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	if _, err := clientset.AppsV1().Deployments(cfg.Namespace).Get(ctx, deploymentName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	return patchWorkloadImage(ctx, clientset, cfg.Namespace, "Deployment", deploymentName, image)
}

func patchWorkloadImage(ctx context.Context, clientset kubernetes.Interface, ns, kind, name, image string) error {
	patch := []byte(fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":%q,"image":%q}]}}}}`, name, image))
	// 容器名与工作负载名一致（STX 部署契约）；adopted 场景容器名可能不同，退化为按第一容器 patch。
	// Container name matches workload name per the STX deploy contract; for adopted clusters
	// the container name may differ, so fall back to patching the first container.
	switch kind {
	case "StatefulSet", "":
		sts, err := clientset.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("k8s: get statefulset %q failed: %w", name, err)
		}
		if len(sts.Spec.Template.Spec.Containers) > 0 {
			patch = []byte(fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":%q,"image":%q}]}}}}`,
				sts.Spec.Template.Spec.Containers[0].Name, image))
		}
		if _, err := clientset.AppsV1().StatefulSets(ns).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("k8s: patch statefulset %q image failed: %w", name, err)
		}
	case "Deployment":
		dep, err := clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("k8s: get deployment %q failed: %w", name, err)
		}
		if len(dep.Spec.Template.Spec.Containers) > 0 {
			patch = []byte(fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":%q,"image":%q}]}}}}`,
				dep.Spec.Template.Spec.Containers[0].Name, image))
		}
		if _, err := clientset.AppsV1().Deployments(ns).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("k8s: patch deployment %q image failed: %w", name, err)
		}
	default:
		return fmt.Errorf("k8s: unsupported workload kind %q for image upgrade", kind)
	}
	return nil
}

// ReadConfigMap 读取集群 ConfigMap 全部键值。
// ReadConfigMap reads all keys of the cluster ConfigMap.
func (s *SessionManager) ReadConfigMap(ctx context.Context, cfg *SessionConfig) (map[string]string, error) {
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return nil, err
	}
	name := cfg.ConfigMapName
	if name == "" {
		return nil, fmt.Errorf("k8s: configmap_name is empty")
	}
	cm, err := clientset.CoreV1().ConfigMaps(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s: get configmap %q failed: %w", name, err)
	}
	data := map[string]string{}
	for k, v := range cm.Data {
		data[k] = v
	}
	return data, nil
}

// WriteConfigMapFile 更新 ConfigMap 单个键；可选随后滚动重启集群使配置生效。
// WriteConfigMapFile updates one ConfigMap key; optionally rolling-restarts the cluster
// afterwards to apply it.
func (s *SessionManager) WriteConfigMapFile(ctx context.Context, cfg *SessionConfig, key, content string, restart bool) error {
	if key == "" {
		return fmt.Errorf("k8s: config key is required")
	}
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	cm, err := clientset.CoreV1().ConfigMaps(cfg.Namespace).Get(ctx, cfg.ConfigMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("k8s: get configmap %q failed: %w", cfg.ConfigMapName, err)
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[key] = content
	if _, err := clientset.CoreV1().ConfigMaps(cfg.Namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("k8s: update configmap %q failed: %w", cfg.ConfigMapName, err)
	}
	if restart {
		return s.RolloutRestart(ctx, cfg, "")
	}
	return nil
}

// PodLogs 读取 Pod 日志；tailLines<=0 取默认 500，sinceTime 可选增量游标。
// PodLogs reads pod logs; non-positive tailLines falls back to 500, and sinceTime is an
// optional incremental cursor.
func (s *SessionManager) PodLogs(ctx context.Context, cfg *SessionConfig, podName string, tailLines int64, sinceTime *time.Time) (string, error) {
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return "", err
	}
	if tailLines <= 0 {
		tailLines = 500
	}
	opts := &corev1.PodLogOptions{TailLines: &tailLines}
	if sinceTime != nil {
		since := metav1.NewTime(*sinceTime)
		opts.SinceTime = &since
	}
	stream, err := clientset.CoreV1().Pods(cfg.Namespace).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("k8s: read pod %q logs failed: %w", podName, err)
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("k8s: read pod %q logs stream failed: %w", podName, err)
	}
	return string(data), nil
}

// DeletePod 删除指定 Pod；工作负载控制器会自动重建，用于单 Pod 卡死时的强制轮换。
// DeletePod deletes the given pod; the owning workload controller recreates it,
// which is the Kubernetes-native way to force-rotate a single stuck pod.
func (s *SessionManager) DeletePod(ctx context.Context, cfg *SessionConfig, podName string) error {
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	if err := clientset.CoreV1().Pods(cfg.Namespace).Delete(ctx, podName, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("k8s: delete pod %q failed: %w", podName, err)
	}
	return nil
}

// WorkloadStatus 汇总 master/worker 工作负载的期望与就绪副本数，用于双校验健康检查第一段。
// WorkloadStatus aggregates desired and ready replicas of master/worker workloads for the
// first stage of the double health check.
type WorkloadStatus struct {
	Role         string `json:"role"`
	WorkloadKind string `json:"workload_kind"`
	WorkloadName string `json:"workload_name"`
	Desired      int    `json:"desired"`
	Ready        int    `json:"ready"`
	Updated      int    `json:"updated"`
	CurrentImage string `json:"current_image"`
	Exists       bool   `json:"exists"`
}

// GetWorkloadStatus 读取 master/worker 工作负载状态；资源不存在时 Exists=false 而非报错，
// 便于健康检查输出明确原因。
// GetWorkloadStatus reads master/worker workload status; missing resources yield Exists=false
// instead of an error so health checks can surface explicit causes.
func (s *SessionManager) GetWorkloadStatus(ctx context.Context, cfg *SessionConfig) ([]*WorkloadStatus, error) {
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return nil, err
	}
	result := []*WorkloadStatus{}
	collect := func(role, kind, name string) {
		status := &WorkloadStatus{Role: role, WorkloadKind: kind, WorkloadName: name}
		if name == "" {
			result = append(result, status)
			return
		}
		switch kind {
		case "StatefulSet", "":
			sts, err := clientset.AppsV1().StatefulSets(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				result = append(result, status)
				return
			}
			status.Exists = true
			status.Ready = int(sts.Status.ReadyReplicas)
			status.Updated = int(sts.Status.UpdatedReplicas)
			if sts.Spec.Replicas != nil {
				status.Desired = int(*sts.Spec.Replicas)
			}
			if len(sts.Spec.Template.Spec.Containers) > 0 {
				status.CurrentImage = sts.Spec.Template.Spec.Containers[0].Image
			}
		case "Deployment":
			dep, err := clientset.AppsV1().Deployments(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				result = append(result, status)
				return
			}
			status.Exists = true
			status.Ready = int(dep.Status.ReadyReplicas)
			status.Updated = int(dep.Status.UpdatedReplicas)
			if dep.Spec.Replicas != nil {
				status.Desired = int(*dep.Spec.Replicas)
			}
			if len(dep.Spec.Template.Spec.Containers) > 0 {
				status.CurrentImage = dep.Spec.Template.Spec.Containers[0].Image
			}
		}
		result = append(result, status)
	}
	if cfg.IsHybrid() {
		collect("hybrid", "StatefulSet", cfg.HybridWorkloadName)
	} else {
		collect("master", cfg.MasterWorkloadKind, cfg.MasterWorkloadName)
		collect("worker", cfg.WorkerWorkloadKind, cfg.WorkerWorkloadName)
	}
	return result, nil
}
