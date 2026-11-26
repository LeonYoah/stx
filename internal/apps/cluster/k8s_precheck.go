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
	"fmt"
	"net/http"
	urlpkg "net/url"
	"strings"

	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// K8sPrecheckRequest 创建向导预检输入：主机 + namespace + 部署规格。
// K8sPrecheckRequest is the deploy-wizard precheck input: host, namespace, and spec.
type K8sPrecheckRequest struct {
	HostID             uint   `json:"host_id" binding:"required,min=1"`
	Namespace          string `json:"namespace"`
	ClusterName        string `json:"cluster_name"`
	ClusterCode        string `json:"cluster_code"`
	DeploymentMode     string `json:"deployment_mode"`
	Image              string `json:"image"`
	ImagePullSecrets   string `json:"image_pull_secrets"`
	ServiceAccount     string `json:"service_account"`
	CheckpointPVC      string `json:"checkpoint_pvc"`
	MasterMemory       string `json:"master_memory"`
	WorkerMemory       string `json:"worker_memory"`
	HybridMemory       string `json:"hybrid_memory"`
	JvmXmx             string `json:"jvm_xmx"`
	MasterHeapMB       int    `json:"master_heap_mb"`
	WorkerHeapMB       int    `json:"worker_heap_mb"`
	HybridHeapMB       int    `json:"hybrid_heap_mb"`
	EnableEventChannel bool   `json:"enable_event_channel"`
	STXCallbackURL     string `json:"stx_callback_url"`
}

// K8sPrecheckItem 单项预检结果。
// K8sPrecheckItem is one precheck outcome.
type K8sPrecheckItem struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Level   string `json:"level"` // error / warning / info
	Message string `json:"message"`
}

// K8sPrecheckResult 预检汇总：passed=false 时部署按钮必须禁用。
// K8sPrecheckResult summarizes the precheck; the deploy button must stay disabled
// while passed is false.
type K8sPrecheckResult struct {
	Passed bool               `json:"passed"`
	Items  []*K8sPrecheckItem `json:"items"`
}

// K8sPrecheck 创建集群预检：API 连通 / RBAC / 资源冲突 / pvc+secrets+sa / 配比。
// K8sPrecheck runs pre-deploy checks: API connectivity, RBAC, resource conflicts,
// pvc+secrets+sa existence, and sizing validation.
func (s *Service) K8sPrecheck(ctx context.Context, req *K8sPrecheckRequest) (*K8sPrecheckResult, error) {
	if s.k8sManager == nil {
		return nil, fmt.Errorf("cluster: k8s dependencies not configured")
	}
	namespace := strings.TrimSpace(req.Namespace)
	if namespace == "" {
		namespace = "default"
	}
	result := &K8sPrecheckResult{Passed: true, Items: []*K8sPrecheckItem{}}
	add := func(name, level string, passed bool, message string) {
		result.Items = append(result.Items, &K8sPrecheckItem{Name: name, Level: level, Passed: passed, Message: message})
		if !passed && level == "error" {
			result.Passed = false
		}
	}

	clientset, err := s.k8sManager.Clientset(ctx, req.HostID)
	if err != nil {
		add("api_connectivity", "error", false, fmt.Sprintf("Kubernetes API 不可达: %v", err))
		return result, nil
	}
	// 连通性：读 namespace 即证明 API 与基本权限可用。
	// Connectivity: reading the namespace proves the API and basic permissions.
	if _, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			add("api_connectivity", "warning", true, fmt.Sprintf("API 可达，但 namespace %q 不存在，部署时将自动创建", namespace))
		} else {
			add("api_connectivity", "error", false, fmt.Sprintf("读取 namespace 失败: %v", err))
		}
	} else {
		add("api_connectivity", "info", true, fmt.Sprintf("API 可达，namespace %q 存在", namespace))
	}

	// RBAC SelfSubjectAccessReview。
	// RBAC via SelfSubjectAccessReview.
	rbacItems, err := s.k8sManager.PrecheckRBAC(ctx, req.HostID, namespace)
	if err != nil {
		add("rbac", "error", false, fmt.Sprintf("RBAC 预检失败: %v", err))
	} else {
		denied := []string{}
		for _, item := range rbacItems {
			if !item.Allowed {
				denied = append(denied, fmt.Sprintf("%s(%s)", item.Resource, item.Verb))
			}
		}
		if len(denied) > 0 {
			add("rbac", "error", false, fmt.Sprintf("缺少权限: %s", strings.Join(denied, ", ")))
		} else {
			add("rbac", "info", true, "RBAC 权限齐备")
		}
	}

	// 资源冲突：同名 StatefulSet/Service/ConfigMap 已存在时阻塞部署。
	// Resource conflicts: existing same-name workloads/services/configmaps block deploy.
	conflicts := []string{}
	// 命名口径必须与 CreateKubernetes 完全一致：显式 cluster_code 优先，
	// 否则回退 SanitizeClusterCode(cluster_name)，否则预检与实部署检查的就不是同一批资源。
	// The naming must match CreateKubernetes exactly: an explicit cluster_code wins,
	// otherwise fall back to SanitizeClusterCode(cluster_name); otherwise precheck and
	// the real deploy would examine different resources.
	code := strings.TrimSpace(req.ClusterCode)
	if code == "" {
		code = SanitizeClusterCode(req.ClusterName, 0)
	}
	if code != "" {
		names := k8s.ResourceNamesFor(code)
		for _, name := range []string{names.MasterSTS, names.WorkerSTS, names.MasterSTS + "-hybrid"} {
			if _, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
				conflicts = append(conflicts, "StatefulSet/"+name)
			}
		}
		for _, name := range []string{names.ClusterSvc, names.MasterSvc} {
			if _, err := clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
				conflicts = append(conflicts, "Service/"+name)
			}
		}
		if _, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, names.ConfigMap, metav1.GetOptions{}); err == nil {
			conflicts = append(conflicts, "ConfigMap/"+names.ConfigMap)
		}
	}
	if len(conflicts) > 0 {
		add("resource_conflict", "error", false, fmt.Sprintf("已存在同名资源: %s", strings.Join(conflicts, ", ")))
	} else {
		add("resource_conflict", "info", true, "无同名资源冲突")
	}

	// pvc / secrets / serviceaccount 存在性：引用了就必须在。
	// pvc/secret/serviceaccount existence: referenced objects must exist.
	missing := []string{}
	if pvc := strings.TrimSpace(req.CheckpointPVC); pvc != "" {
		if _, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvc, metav1.GetOptions{}); err != nil {
			missing = append(missing, "PersistentVolumeClaim/"+pvc)
		}
	}
	for _, secret := range strings.Split(req.ImagePullSecrets, ",") {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		if _, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secret, metav1.GetOptions{}); err != nil {
			missing = append(missing, "Secret/"+secret)
		}
	}
	if sa := strings.TrimSpace(req.ServiceAccount); sa != "" {
		if _, err := clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, sa, metav1.GetOptions{}); err != nil {
			missing = append(missing, "ServiceAccount/"+sa)
		}
	}
	if len(missing) > 0 {
		add("referenced_objects", "error", false, fmt.Sprintf("引用对象不存在: %s", strings.Join(missing, ", ")))
	} else {
		add("referenced_objects", "info", true, "引用的 PVC/Secret/ServiceAccount 均存在或未引用")
	}

	// 配比真校验：JVM 堆不得超过容器内存 80%（与创建向导红线一致）；
	// 事件通道开启时回调地址不得回环。两项失败均为 error 级阻断。
	// Real sizing validation: the JVM heap must not exceed 80% of container memory
	// (same red line as the deploy wizard); with the event channel on, the callback
	// address must not be loopback. Both failures are error-level blockers.
	sizingProblems := []string{}
	// 分角色堆优先于统一 Xmx：向导按角色输入堆上限后，统一 Xmx 仅作历史兜底。
	// Per-role heaps win over the shared Xmx: once the wizard carries per-role heap
	// ceilings, the shared Xmx only serves as a legacy fallback.
	roleHeaps := []struct {
		role   string
		heapMi int
		memory string
	}{}
	if req.DeploymentMode == "hybrid" {
		roleHeaps = append(roleHeaps, struct {
			role   string
			heapMi int
			memory string
		}{"hybrid", req.HybridHeapMB, req.HybridMemory})
	} else {
		roleHeaps = append(roleHeaps,
			struct {
				role   string
				heapMi int
				memory string
			}{"master", req.MasterHeapMB, req.MasterMemory},
			struct {
				role   string
				heapMi int
				memory string
			}{"worker", req.WorkerHeapMB, req.WorkerMemory})
	}
	for _, rh := range roleHeaps {
		if rh.heapMi <= 0 {
			continue
		}
		raw := strings.TrimSpace(rh.memory)
		if raw == "" {
			continue
		}
		quantity, parseErr := resource.ParseQuantity(raw)
		if parseErr != nil {
			continue
		}
		containerMi := int(quantity.Value() / (1024 * 1024))
		if containerMi > 0 && rh.heapMi*5 > containerMi*4 {
			sizingProblems = append(sizingProblems, fmt.Sprintf("%s 堆 %dMi 超过容器内存 %dMi 的 80%%（%dMi）", rh.role, rh.heapMi, containerMi, containerMi*4/5))
		}
	}
	if xmxMi := k8s.JVMHeapMaxMi("-Xmx" + strings.TrimSpace(req.JvmXmx)); xmxMi > 0 && req.MasterHeapMB <= 0 && req.WorkerHeapMB <= 0 && req.HybridHeapMB <= 0 {
		memories := []struct {
			role string
			raw  string
		}{}
		if req.DeploymentMode == "hybrid" {
			memories = append(memories, struct {
				role string
				raw  string
			}{"hybrid", req.HybridMemory})
		} else {
			memories = append(memories,
				struct {
					role string
					raw  string
				}{"master", req.MasterMemory},
				struct {
					role string
					raw  string
				}{"worker", req.WorkerMemory})
		}
		for _, mem := range memories {
			raw := strings.TrimSpace(mem.raw)
			if raw == "" {
				continue
			}
			quantity, parseErr := resource.ParseQuantity(raw)
			if parseErr != nil {
				sizingProblems = append(sizingProblems, fmt.Sprintf("%s 内存 %q 无法解析", mem.role, raw))
				continue
			}
			containerMi := int(quantity.Value() / (1024 * 1024))
			if containerMi > 0 && xmxMi*5 > containerMi*4 {
				sizingProblems = append(sizingProblems, fmt.Sprintf("JVM 堆 %dMi 超过 %s 容器内存 %dMi 的 80%%（%dMi）", xmxMi, mem.role, containerMi, containerMi*4/5))
			}
		}
	}
	if req.EnableEventChannel {
		if override := strings.TrimSpace(req.STXCallbackURL); override != "" {
			if parsed, parseErr := urlpkg.Parse(override); parseErr != nil || parsed == nil || IsLoopbackCallbackHost(parsed.Hostname()) {
				sizingProblems = append(sizingProblems, "事件通道回调地址为回环地址，引擎 Pod 内无法到达控制面")
			}
		}
	}
	if len(sizingProblems) > 0 {
		add("sizing", "error", false, strings.Join(sizingProblems, "；"))
	} else {
		add("sizing", "info", true, "JVM 堆与容器内存配比合规"+func() string {
			if req.EnableEventChannel {
				return "，事件通道回调地址可达性预检通过"
			}
			return ""
		}())
	}
	return result, nil
}

// K8sPrecheckHandler handles POST /api/v1/clusters/k8s/precheck.
// K8sPrecheckHandler 处理创建向导 K8s 预检请求。
func (h *Handler) K8sPrecheckHandler(c *gin.Context) {
	var req K8sPrecheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	result, err := h.service.K8sPrecheck(c.Request.Context(), &req)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), gin.H{"error_msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
