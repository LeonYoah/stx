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
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadSummary 一个工作负载的摘要信息。
// WorkloadSummary summarizes one workload.
type WorkloadSummary struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"ready_replicas"`
	Image         string `json:"image"`
}

// DiscoveredCluster 命名空间中发现的一套 SeaTunnel 常驻集群候选。
// DiscoveredCluster is one adoptable SeaTunnel session cluster candidate in a namespace.
type DiscoveredCluster struct {
	BaseName          string          `json:"base_name"`
	Namespace         string          `json:"namespace"`
	Master            *WorkloadSummary `json:"master,omitempty"`
	Worker            *WorkloadSummary `json:"worker,omitempty"`
	Services          []string        `json:"services"`
	ConfigMapName     string          `json:"configmap_name,omitempty"`
	LabelSelector     string          `json:"label_selector,omitempty"`
	MasterHTTPPort    int             `json:"master_http_port"`
	HazelcastPort     int             `json:"hazelcast_port"`
}

// Discoverer 在命名空间中扫描已有 SeaTunnel 集群（StatefulSet/Deployment/Service/ConfigMap）。
// Discoverer scans a namespace for existing SeaTunnel clusters
// (StatefulSet/Deployment/Service/ConfigMap).
type Discoverer struct {
	manager *ClientManager
}

// NewDiscoverer 创建发现器。
// NewDiscoverer creates a discoverer.
func NewDiscoverer(manager *ClientManager) *Discoverer {
	return &Discoverer{manager: manager}
}

func isSeaTunnelImage(image string) bool {
	return strings.Contains(strings.ToLower(image), "seatunnel")
}

func workloadRole(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, "-master") || strings.Contains(lower, "master") {
		return "master"
	}
	if strings.HasSuffix(lower, "-worker") || strings.Contains(lower, "worker") {
		return "worker"
	}
	return ""
}

func baseNameOf(name, role string) string {
	lower := strings.ToLower(name)
	switch role {
	case "master":
		if idx := strings.LastIndex(lower, "-master"); idx > 0 {
			return name[:idx]
		}
		if idx := strings.LastIndex(lower, "master"); idx > 0 {
			return strings.TrimRight(name[:idx], "-")
		}
	case "worker":
		if idx := strings.LastIndex(lower, "-worker"); idx > 0 {
			return name[:idx]
		}
		if idx := strings.LastIndex(lower, "worker"); idx > 0 {
			return strings.TrimRight(name[:idx], "-")
		}
	}
	return name
}

// Discover 扫描命名空间中的 SeaTunnel 工作负载并按 master/worker 命名约定配对成组。
// Discover scans SeaTunnel workloads in the namespace and pairs master/worker workloads
// by naming convention.
func (d *Discoverer) Discover(ctx context.Context, hostID uint, namespace string) ([]*DiscoveredCluster, error) {
	clientset, err := d.manager.Clientset(ctx, hostID)
	if err != nil {
		return nil, err
	}
	if namespace == "" {
		namespace = "default"
	}

	stsList, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s: list statefulsets failed: %w", err)
	}
	deployList, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s: list deployments failed: %w", err)
	}
	svcList, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s: list services failed: %w", err)
	}

	groups := map[string]*DiscoveredCluster{}
	addWorkload := func(kind string, summary *WorkloadSummary, labels map[string]string) {
		role := workloadRole(summary.Name)
		if role == "" {
			return
		}
		base := baseNameOf(summary.Name, role)
		group, ok := groups[base]
		if !ok {
			group = &DiscoveredCluster{
				BaseName:       base,
				Namespace:      namespace,
				MasterHTTPPort: 8080,
				HazelcastPort:  5801,
			}
			groups[base] = group
		}
		if role == "master" && group.Master == nil {
			group.Master = summary
		}
		if role == "worker" && group.Worker == nil {
			group.Worker = summary
		}
		if group.LabelSelector == "" {
			if instance, ok := labels["app.kubernetes.io/instance"]; ok && instance != "" {
				group.LabelSelector = "app.kubernetes.io/instance=" + instance
			}
		}
		_ = kind
	}

	for i := range stsList.Items {
		sts := &stsList.Items[i]
		image := firstContainerImage(&sts.Spec.Template.Spec)
		if !isSeaTunnelImage(image) {
			continue
		}
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		addWorkload("StatefulSet", &WorkloadSummary{
			Kind:          "StatefulSet",
			Name:          sts.Name,
			Replicas:      replicas,
			ReadyReplicas: sts.Status.ReadyReplicas,
			Image:         image,
		}, sts.Labels)
		if cm := configMapNameFromPodSpec(&sts.Spec.Template.Spec); cm != "" {
			base := baseNameOf(sts.Name, workloadRole(sts.Name))
			if group, ok := groups[base]; ok && group.ConfigMapName == "" {
				group.ConfigMapName = cm
			}
		}
	}
	for i := range deployList.Items {
		dep := &deployList.Items[i]
		image := firstContainerImage(&dep.Spec.Template.Spec)
		if !isSeaTunnelImage(image) {
			continue
		}
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		addWorkload("Deployment", &WorkloadSummary{
			Kind:          "Deployment",
			Name:          dep.Name,
			Replicas:      replicas,
			ReadyReplicas: dep.Status.ReadyReplicas,
			Image:         image,
		}, dep.Labels)
		if cm := configMapNameFromPodSpec(&dep.Spec.Template.Spec); cm != "" {
			base := baseNameOf(dep.Name, workloadRole(dep.Name))
			if group, ok := groups[base]; ok && group.ConfigMapName == "" {
				group.ConfigMapName = cm
			}
		}
	}

	// 关联服务：名字命中 baseName 的 Service 归入对应分组。
	// Associate services: services whose names contain the base name join the group.
	for i := range svcList.Items {
		svc := &svcList.Items[i]
		for base, group := range groups {
			if strings.Contains(svc.Name, base) {
				group.Services = appendUniqueString(group.Services, svc.Name)
			}
		}
	}

	result := make([]*DiscoveredCluster, 0, len(groups))
	for _, group := range groups {
		if group.Master == nil && group.Worker == nil {
			continue
		}
		sort.Strings(group.Services)
		if group.Services == nil {
			group.Services = []string{}
		}
		result = append(result, group)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].BaseName < result[j].BaseName })
	return result, nil
}

func firstContainerImage(spec *corev1.PodSpec) string {
	if spec == nil || len(spec.Containers) == 0 {
		return ""
	}
	return spec.Containers[0].Image
}

func configMapNameFromPodSpec(spec *corev1.PodSpec) string {
	if spec == nil {
		return ""
	}
	for _, vol := range spec.Volumes {
		if vol.ConfigMap != nil && strings.Contains(vol.ConfigMap.Name, "config") {
			return vol.ConfigMap.Name
		}
	}
	for _, vol := range spec.Volumes {
		if vol.ConfigMap != nil {
			return vol.ConfigMap.Name
		}
	}
	return ""
}

func appendUniqueString(list []string, value string) []string {
	for _, item := range list {
		if item == value {
			return list
		}
	}
	return append(list, value)
}

// 确保 appsv1 引用保留（后续 session 文件使用）。
// Keep the appsv1 import referenced (used by session files).
var _ = appsv1.SchemeGroupVersion
