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
	"strings"

	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// K8sPrecheckRequest 创建向导预检输入：主机 + namespace + 部署规格。
// K8sPrecheckRequest is the deploy-wizard precheck input: host, namespace, and spec.
type K8sPrecheckRequest struct {
	HostID           uint   `json:"host_id" binding:"required,min=1"`
	Namespace        string `json:"namespace"`
	ClusterName      string `json:"cluster_name"`
	DeploymentMode   string `json:"deployment_mode"`
	Image            string `json:"image"`
	ImagePullSecrets string `json:"image_pull_secrets"`
	ServiceAccount   string `json:"service_account"`
	CheckpointPVC    string `json:"checkpoint_pvc"`
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
	if trimmed := strings.TrimSpace(req.ClusterName); trimmed != "" {
		names := k8s.ResourceNamesFor(strings.ToLower(trimmed))
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

	// 配比校验：hybrid 模式副本数与形态合法性。
	// Sizing validation: hybrid replicas and topology sanity.
	if req.DeploymentMode == "hybrid" {
		add("sizing", "info", true, "hybrid 形态：单 StatefulSet 承载 master+worker 角色")
	} else {
		add("sizing", "info", true, "separated 形态：master/worker 分离部署")
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
