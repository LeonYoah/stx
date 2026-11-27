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

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/pkg/locale"
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
	AccessMode         string `json:"access_mode"`
	ClusterName        string `json:"cluster_name"`
	ClusterCode        string `json:"cluster_code"`
	DeploymentMode     string `json:"deployment_mode"`
	Image              string `json:"image"`
	Version            string `json:"version"`
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
		add("api_connectivity", "error", false, fmt.Sprintf(locale.Pick(ctx, "Kubernetes API 不可达: %v", "Kubernetes API unreachable: %v"), err))
		return result, nil
	}
	// 连通性：读 namespace 即证明 API 与基本权限可用；单命名空间最小权限返回 403 时退化为命名空间内 ConfigMap 只读列表探测。
	// Connectivity: reading the namespace proves the API and basic permissions; on 403 under single-namespace minimal RBAC,
	// probe namespace-scoped ConfigMaps list instead.
	if _, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{}); err != nil {
		if apierrors.IsForbidden(err) {
			if _, nsErr := clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{Limit: 1}); nsErr != nil {
				add("api_connectivity", "error", false, fmt.Sprintf(locale.Pick(ctx, "无法访问命名空间 %q: %v", "cannot access namespace %q: %v"), namespace, nsErr))
			} else {
				add("api_connectivity", "info", true, fmt.Sprintf(locale.Pick(ctx, "API 可达（单命名空间授权模式，已验证目标命名空间 %q 可访问）", "API reachable (single-namespace mode, verified access to namespace %q)"), namespace))
			}
		} else if apierrors.IsNotFound(err) {
			add("api_connectivity", "warning", true, fmt.Sprintf(locale.Pick(ctx, "API 可达，但 namespace %q 不存在，部署时将自动创建", "API reachable, but namespace %q does not exist and will be created at deploy time"), namespace))
		} else {
			add("api_connectivity", "error", false, fmt.Sprintf(locale.Pick(ctx, "读取 namespace 失败: %v", "failed to read namespace: %v"), err))
		}
	} else {
		add("api_connectivity", "info", true, fmt.Sprintf(locale.Pick(ctx, "API 可达，namespace %q 存在", "API reachable, namespace %q exists"), namespace))
	}

	// RBAC SelfSubjectAccessReview：区分核心运行时权限（required）、按需能力（optional）与全面权限（extended）。
	// RBAC via SelfSubjectAccessReview: separate core required rules from optional capability-gated
	// rules and extended multi-namespace/RBAC provisioning rules.
	rbacItems, err := s.k8sManager.PrecheckRBAC(ctx, req.HostID, namespace)
	if err != nil {
		add("rbac", "error", false, fmt.Sprintf(locale.Pick(ctx, "RBAC 预检失败: %v", "RBAC precheck failed: %v"), err))
	} else {
		deniedRequired := []string{}
		for _, item := range rbacItems {
			if item.Scope != "" && item.Scope != k8s.RBACScopeRequired {
				continue
			}
			if !item.Allowed {
				deniedRequired = append(deniedRequired, fmt.Sprintf("%s(%s)", item.Resource, item.Verb))
			}
		}
		if len(deniedRequired) > 0 {
			add("rbac", "error", false, fmt.Sprintf(locale.Pick(ctx, "缺少必需权限: %s", "missing required permissions: %s"), strings.Join(deniedRequired, ", ")))
		} else {
			permMode := k8s.EvaluatePermissionMode(rbacItems)
			canManageRBAC := k8s.CanManageRBAC(rbacItems)
			switch {
			case permMode == k8s.PermissionModeFull:
				add("rbac", "info", true, locale.Pick(ctx, "RBAC 权限齐备（全面模式：支持跨命名空间与自动创建集群专属 RBAC）", "RBAC permissions complete (full mode: multi-namespace and auto per-cluster RBAC)"))
			case canManageRBAC:
				add("rbac", "info", true, locale.Pick(ctx, "RBAC 核心权限齐备（单命名空间模式，可自动创建集群专属 RBAC）", "Core RBAC permissions complete (single-namespace mode, auto per-cluster RBAC available)"))
			default:
				effectiveSA, _ := s.resolveK8sServiceAccountWithRBAC(ctx, req.HostID, namespace, req.ServiceAccount, rbacItems)
				if effectiveSA != "" && effectiveSA != "default" {
					add("rbac", "info", true, fmt.Sprintf(locale.Pick(ctx, "RBAC 核心权限齐备（最小权限模式，将使用预建 ServiceAccount %q）", "Core RBAC permissions complete (minimal mode, using pre-created ServiceAccount %q)"), effectiveSA))
				} else {
					add("rbac", "warning", true, locale.Pick(ctx, "RBAC 核心权限齐备（最小权限模式，无自动创建 RBAC 权限；建议指定预建的 SeaTunnel ServiceAccount，未指定时将回退命名空间 default）", "Core RBAC permissions complete (minimal mode without RBAC creation; recommend specifying a pre-created SeaTunnel ServiceAccount, otherwise namespace default is used)"))
				}
			}
		}

		// 访问通道校验：仅支持 in_cluster（集群内直连）或 external（外部入口）。
		// Access-mode validation: only in_cluster and external are supported.
		accessMode := strings.TrimSpace(req.AccessMode)
		if accessMode == "" {
			accessMode = string(k8s.AccessModeInCluster)
		}
		if accessMode != string(k8s.AccessModeInCluster) && accessMode != string(k8s.AccessModeExternal) {
			add("access_mode", "error", false, locale.Pick(ctx, "不支持的连接方式，仅支持集群内直连（in_cluster）或集群外入口（external）", "unsupported access_mode; only in_cluster or external are supported"))
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
		add("resource_conflict", "error", false, fmt.Sprintf(locale.Pick(ctx, "已存在同名资源: %s", "conflicting resources already exist: %s"), strings.Join(conflicts, ", ")))
	} else {
		add("resource_conflict", "info", true, locale.Pick(ctx, "无同名资源冲突", "no conflicting resources"))
	}

	// pvc / secrets / serviceaccount 存在性：引用了（含主机默认 ServiceAccount）就必须在；
	// 最小权限禁掉 secrets get 时遇到 403 不误报「不存在」，转为 warning 提示运维自行确认。
	// pvc/secret/serviceaccount existence: referenced objects must exist; 403 Forbidden on
	// Secrets.Get under minimal RBAC becomes a warning instead of false-reporting NotFound.
	missing := []string{}
	unverifiedSecrets := []string{}
	manualSecrets := []string{}
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
		manualSecrets = append(manualSecrets, secret)
		if _, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secret, metav1.GetOptions{}); err != nil {
			if apierrors.IsForbidden(err) {
				unverifiedSecrets = append(unverifiedSecrets, secret)
			} else {
				missing = append(missing, "Secret/"+secret)
			}
		}
	}
	saToCheck := strings.TrimSpace(req.ServiceAccount)
	if saToCheck == "" && s.hostProvider != nil {
		if hostInfo, hostErr := s.hostProvider.GetHostByID(ctx, req.HostID); hostErr == nil && hostInfo != nil {
			saToCheck = strings.TrimSpace(hostInfo.K8sDefaultServiceAccount)
		}
	}
	if saToCheck != "" {
		if _, err := clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, saToCheck, metav1.GetOptions{}); err != nil {
			missing = append(missing, "ServiceAccount/"+saToCheck)
		}
	}
	if len(missing) > 0 {
		add("referenced_objects", "error", false, fmt.Sprintf(locale.Pick(ctx, "引用对象不存在: %s", "referenced objects do not exist: %s"), strings.Join(missing, ", ")))
	} else if len(unverifiedSecrets) > 0 {
		add("referenced_objects", "warning", true, fmt.Sprintf(locale.Pick(ctx, "未授予 secrets 读取权限，无法校验 Secret %s 是否存在，请确认运维已在命名空间预建", "secrets read permission absent, cannot verify Secret %s; confirm it is pre-created in the namespace"), strings.Join(unverifiedSecrets, ", ")))
	} else {
		add("referenced_objects", "info", true, locale.Pick(ctx, "引用的 PVC/Secret/ServiceAccount 均存在或未引用", "referenced PVC/Secret/ServiceAccount all exist or are unused"))
	}

	// 镜像仓库与拉取凭据校验（路线 A）：
	// 1) 用户已显式指定 imagePullSecrets → 尊重已有 Secret，不代建；
	// 2) 命中带凭据的已登记仓库（含认证 docker.io）→ 校验是否具备 secrets 写权限，缺失则精确阻断并给二选一指引；
	// 3) 匿名仓库 / 未登记仓库 → 放行（未登记私仓无手动 Secret 时仅给 warning 提醒）。
	// Registry and pull-credential precheck (Route A): user-supplied secrets skip auto-creation;
	// credentialed registries require namespace secrets write permission or an explicit manual secret;
	// unregistered private registries warn without hard blocking.
	imageHost := installerapp.RegistryImageHost(req.Image)
	if len(manualSecrets) > 0 {
		add("registry", "info", true, fmt.Sprintf(locale.Pick(ctx, "将使用用户指定的 imagePullSecrets（%s），不自动代建 Secret", "using user-specified imagePullSecrets (%s); skipping auto pull-secret creation"), strings.Join(manualSecrets, ", ")))
	} else if imageHost != "" {
		row, _ := s.findContainerRegistryByHost(ctx, imageHost)
		switch {
		case row != nil && row.HasCredentials():
			if len(rbacItems) > 0 && !k8s.CanManageSecrets(rbacItems) {
				add("registry", "error", false, fmt.Sprintf(locale.Pick(ctx, "仓库 %s 已登记凭据，但当前 K8s 凭据缺少命名空间 secrets(get/create/update) 权限，无法自动代建拉取 Secret；请授予 secrets 权限，或在高级选项指定运维预建的 imagePullSecrets", "registry %s has credentials, but the K8s token lacks namespace secrets(get/create/update) permission to auto-create the pull secret; grant secrets permissions or specify a pre-created imagePullSecret in advanced options"), imageHost))
			} else {
				add("registry", "info", true, fmt.Sprintf(locale.Pick(ctx, "仓库 %s 已登记（含凭据），拉取 Secret 将自动代建", "registry %s is registered with credentials; the pull secret will be materialized automatically"), imageHost))
			}
		case row != nil:
			if row.IsSystem {
				add("registry", "info", true, locale.Pick(ctx, "docker.io 官方仓库（匿名），无需拉取凭据", "docker.io official registry (anonymous), no pull credentials needed"))
			} else {
				add("registry", "info", true, fmt.Sprintf(locale.Pick(ctx, "仓库 %s 已登记（匿名）", "registry %s is registered (anonymous)"), imageHost))
			}
		case imageHost == installerapp.DockerHubRegistryHost:
			add("registry", "info", true, locale.Pick(ctx, "docker.io 公共镜像，无需凭据", "docker.io public image, no credentials needed"))
		default:
			add("registry", "warning", true, fmt.Sprintf(locale.Pick(ctx, "镜像仓库 %s 未在 STX 登记且未指定 imagePullSecrets；若集群未配置节点免密或 ServiceAccount 拉取凭据，拉取可能失败", "registry %s is unregistered and no imagePullSecrets were provided; ensure nodes have pull access or ServiceAccount credentials"), imageHost))
		}
	} else {
		add("registry", "info", true, locale.Pick(ctx, "镜像仓库校验已跳过（镜像尚未填写）", "registry check skipped (image not specified)"))
	}
	if !semverLikeTag(strings.TrimSpace(req.Version)) {
		add("version", "warning", true, fmt.Sprintf(locale.Pick(ctx, "版本 %q 不是确定性语义版本（如 3.1.0），配置渲染将按保守默认生成", "version %q is not a deterministic semver (e.g. 3.1.0); config rendering will fall back to conservative defaults"), req.Version))
	} else {
		add("version", "info", true, fmt.Sprintf(locale.Pick(ctx, "引擎版本 %s", "engine version %s"), req.Version))
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
			sizingProblems = append(sizingProblems, fmt.Sprintf(locale.Pick(ctx, "%s 堆 %dMi 超过容器内存 %dMi 的 80%%（%dMi）", "%s heap %dMi exceeds 80%% of container memory %dMi (%dMi)"), rh.role, rh.heapMi, containerMi, containerMi*4/5))
		}
	}
	// 无显式堆时堆按容器内存 1/2 自动推导：内存必须可解析且 >= 512Mi，
	// 否则推导返回 0 回落保守默认 -Xmx2g，小容器必然 OOM。
	// Without explicit heaps the heap derives as half the container memory, so memory
	// must parse and be at least 512Mi — otherwise derivation yields 0 and the
	// conservative -Xmx2g fallback OOMs small containers.
	noExplicitHeap := req.MasterHeapMB <= 0 && req.WorkerHeapMB <= 0 && req.HybridHeapMB <= 0 && strings.TrimSpace(req.JvmXmx) == ""
	if noExplicitHeap {
		memCheck := []struct {
			role string
			raw  string
		}{}
		if req.DeploymentMode == "hybrid" {
			memCheck = append(memCheck, struct {
				role string
				raw  string
			}{"hybrid", req.HybridMemory})
		} else {
			memCheck = append(memCheck,
				struct {
					role string
					raw  string
				}{"master", req.MasterMemory},
				struct {
					role string
					raw  string
				}{"worker", req.WorkerMemory})
		}
		for _, mem := range memCheck {
			if problem := validateContainerMemoryFloor(ctx, mem.role, mem.raw); problem != "" {
				sizingProblems = append(sizingProblems, problem)
			}
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
				sizingProblems = append(sizingProblems, fmt.Sprintf(locale.Pick(ctx, "%s 内存 %q 无法解析", "%s memory %q is unparseable"), mem.role, raw))
				continue
			}
			containerMi := int(quantity.Value() / (1024 * 1024))
			if containerMi > 0 && xmxMi*5 > containerMi*4 {
				sizingProblems = append(sizingProblems, fmt.Sprintf(locale.Pick(ctx, "JVM 堆 %dMi 超过 %s 容器内存 %dMi 的 80%%（%dMi）", "JVM heap %dMi exceeds 80%% of %s container memory %dMi (%dMi)"), xmxMi, mem.role, containerMi, containerMi*4/5))
			}
		}
	}
	eventChannelWarning := ""
	if req.EnableEventChannel {
		// 预检口径必须与真实生效路径一致：override 为空时实际生效的是控制面
		// app.external_url，回环或为空只给 warning（部署后可在事件通道卡片覆写），
		// 绝不谎报"可达性预检通过"。
		// The precheck must mirror the effective path: with no override the live base is
		// the control-plane app.external_url; loopback or empty yields a warning (the
		// event channel card offers post-deploy override), never a false "reachable" pass.
		effective := strings.TrimSpace(req.STXCallbackURL)
		if effective == "" {
			effective = strings.TrimSpace(config.GetExternalURL())
		}
		if effective == "" {
			eventChannelWarning = locale.Pick(ctx, "事件通道已开启但控制面 app.external_url 未配置，引擎事件将无法回传；部署后可在集群详情的事件通道卡片覆写回调地址", "event channel is on but control-plane app.external_url is unset, so engine events cannot report back; override the callback in the cluster detail event channel card after deploy")
		} else if parsed, parseErr := urlpkg.Parse(effective); parseErr != nil || parsed == nil || IsLoopbackCallbackHost(parsed.Hostname()) {
			if strings.TrimSpace(req.STXCallbackURL) != "" {
				sizingProblems = append(sizingProblems, locale.Pick(ctx, "事件通道回调地址为回环地址，引擎 Pod 内无法到达控制面", "event channel callback address is loopback; engine pods cannot reach the control plane"))
			} else {
				eventChannelWarning = locale.Pick(ctx, "事件通道回调将继承控制面 app.external_url（回环地址），引擎 Pod 内无法到达；部署后可在集群详情的事件通道卡片覆写为 Pod 可达地址", "event channel callback inherits control-plane app.external_url (loopback), unreachable inside engine pods; override it with a pod-reachable address in the cluster detail event channel card after deploy")
			}
		}
	}
	if len(sizingProblems) > 0 {
		add("sizing", "error", false, strings.Join(sizingProblems, "；"))
	} else {
		if eventChannelWarning != "" {
			add("sizing", "warning", true, locale.Pick(ctx, "容器内存与堆配比合规；", "container memory and heap sizing compliant; ")+eventChannelWarning)
		} else {
			add("sizing", "info", true, locale.Pick(ctx, "容器内存与堆配比合规（堆按容器内存 1/2 自动推导）", "container memory and heap sizing compliant (heap auto-derives as half the container memory)"))
		}
	}
	return result, nil
}

// K8sPrecheckHandler handles POST /api/v1/clusters/k8s/precheck.
// K8sPrecheckHandler 处理创建向导 K8s 预检请求。
// @Summary K8s 创建预检 / Kubernetes creation precheck
// @Tags clusters
// @Produce json
// @Param body body map[string]interface{} true "预检请求 / Precheck request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/clusters/k8s/precheck [post]
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

// validateContainerMemoryFloor 校验容器内存可解析且不低于 512Mi（堆自动推导下限）。
// validateContainerMemoryFloor validates container memory parses and meets the 512Mi
// floor required by heap auto-derivation; empty problem string means pass.
func validateContainerMemoryFloor(ctx context.Context, role string, raw string) string {
	trimmed := strings.TrimSpace(raw)
	quantity, parseErr := resource.ParseQuantity(trimmed)
	if parseErr != nil {
		return fmt.Sprintf(locale.Pick(ctx, "%s 容器内存 %q 无法解析", "%s container memory %q is unparseable"), role, trimmed)
	}
	containerMi := int(quantity.Value() / (1024 * 1024))
	if containerMi < 512 {
		return fmt.Sprintf(locale.Pick(ctx, "%s 容器内存 %dMi 低于下限 512Mi（堆自动推导要求）", "%s container memory %dMi is below the 512Mi minimum (required by heap auto-derivation)"), role, containerMi)
	}
	return ""
}
