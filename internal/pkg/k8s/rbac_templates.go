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
	"fmt"
	"strings"
)

const (
	// DefaultSTXManagerServiceAccount STX 控制面使用的默认 ServiceAccount。
	// DefaultSTXManagerServiceAccount is the default ServiceAccount used by the STX control plane.
	DefaultSTXManagerServiceAccount = "stx-manager"
	// DefaultSeaTunnelRunnerServiceAccount 最小权限模式下 SeaTunnel 引擎 Pod 默认使用的低权限只读 ServiceAccount。
	// DefaultSeaTunnelRunnerServiceAccount is the default read-only ServiceAccount for SeaTunnel engine pods in minimal RBAC mode.
	DefaultSeaTunnelRunnerServiceAccount = "seatunnel-runner"
)

// MinimalRBACOptions 控制单命名空间控制面权限模板的可选模块拼装。
// MinimalRBACOptions controls optional capability blocks included in the single-namespace control-plane YAML.
type MinimalRBACOptions struct {
	IncludeDeleteRules bool
	IncludeSecretRules bool
}

func normalizeTemplateNamespace(namespace string) string {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		return "seatunnel"
	}
	return ns
}

func normalizeRunnerSA(runnerSA string) string {
	sa := strings.TrimSpace(runnerSA)
	if sa == "" {
		return DefaultSeaTunnelRunnerServiceAccount
	}
	return sa
}

func normalizeManagerSA(managerSA ...string) string {
	if len(managerSA) > 0 {
		if sa := strings.TrimSpace(managerSA[0]); sa != "" {
			return sa
		}
	}
	return DefaultSTXManagerServiceAccount
}

// RenderMinimalRBACYAML 生成单命名空间 STX 控制面最小权限 RBAC YAML（仅包含控制面自身所需权限，不包含引擎运行账号）。
// RenderMinimalRBACYAML renders pure single-namespace STX control-plane RBAC YAML (never bundles engine runner SA).
func RenderMinimalRBACYAML(namespace string, managerSA ...string) string {
	return RenderMinimalRBACYAMLWithOptions(
		namespace,
		normalizeManagerSA(managerSA...),
		MinimalRBACOptions{
			IncludeDeleteRules: false,
			IncludeSecretRules: false,
		},
	)
}

// RenderMinimalRBACYAMLWithOptions 按按需模块拼装单命名空间 STX 控制面最小权限 RBAC YAML：
// 1) 严格仅授予 STX 控制面 ServiceAccount 在当前命名空间内的部署、更新与日志读取权限；
// 2) 仅页面卸载删资源或自动代建 Secret 需要时才附加对应可选权限块；
// 3) 与 SeaTunnel 引擎运行账号完全解耦。
// RenderMinimalRBACYAMLWithOptions renders modular single-namespace STX control-plane RBAC YAML
// with scoped base rules and opt-in capability blocks, strictly decoupled from SeaTunnel engine runner SAs.
func RenderMinimalRBACYAMLWithOptions(namespace, managerSA string, opts MinimalRBACOptions) string {
	ns := normalizeTemplateNamespace(namespace)
	manager := normalizeManagerSA(managerSA)

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`# STX 控制面单命名空间权限模板 / STX single-namespace control-plane RBAC template
# 仅授权 STX 控制面在当前命名空间内部署、更新与读取日志 / Scoped strictly to control-plane deploy, update, and logs in one namespace
apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s-role
  namespace: %s
rules:
  # --- 核心运行时与部署必选权限 / Core runtime & deploy permissions ---
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["services", "configmaps"]
    verbs: ["get", "list", "create", "update"]
  - apiGroups: [""]
    resources: ["serviceaccounts", "persistentvolumeclaims"]
    verbs: ["get", "list"]
  - apiGroups: ["apps"]
    resources: ["statefulsets", "deployments"]
    verbs: ["get", "list", "create", "update", "patch"]
  - apiGroups: ["apps"]
    resources: ["replicasets"]
    verbs: ["list"]
`, manager, ns, manager, ns))

	if opts.IncludeDeleteRules {
		b.WriteString(`  # --- 可选块：STX 页面一键卸载、失败自动回收与单 Pod 重建；不勾选时缩容至 0 并提示按标签清理命令 ---
  # --- Optional: UI uninstall, failed deploy reclamation & single-pod recreate ---
  - apiGroups: ["apps"]
    resources: ["statefulsets", "deployments"]
    verbs: ["delete"]
  - apiGroups: [""]
    resources: ["services", "configmaps", "pods"]
    verbs: ["delete"]
`)
	}
	if opts.IncludeSecretRules {
		b.WriteString(`  # --- 可选块：STX 自动代建私仓拉取 Secret；由运维预建 imagePullSecrets 时无需勾选 ---
  # --- Optional: STX auto pull-secret provisioning; skip when admins pre-create imagePullSecrets ---
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "create", "update", "delete"]
`)
	}

	b.WriteString(fmt.Sprintf(`---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s-binding
  namespace: %s
subjects:
  - kind: ServiceAccount
    name: %s
    namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s-role
`, manager, ns, manager, ns, manager))

	return b.String()
}

// RenderSeaTunnelRunnerRBACYAML 生成独立的 SeaTunnel 引擎运行账号 RBAC YAML（仅包含命名空间内只读发现权限）。
// RenderSeaTunnelRunnerRBACYAML renders standalone read-only RBAC YAML for SeaTunnel engine Pods in a single namespace.
func RenderSeaTunnelRunnerRBACYAML(namespace, runnerSA string) string {
	ns := normalizeTemplateNamespace(namespace)
	runner := normalizeRunnerSA(runnerSA)
	return fmt.Sprintf(`# SeaTunnel 引擎运行账号权限模板（与 STX 控制面账号完全独立）
# Standalone SeaTunnel engine runner RBAC template (isolated from STX control-plane SA)
# 仅在启用 K8s API 节点发现或需要绑定专属 Pod 身份时应用；默认 DNS 服务发现无需任何 K8s API 权限
# Apply only when using K8s API discovery or dedicated pod identity; default DNS discovery needs zero K8s API permissions
apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s-role
  namespace: %s
rules:
  - apiGroups: [""]
    resources: ["endpoints", "pods", "services"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s-binding
  namespace: %s
subjects:
  - kind: ServiceAccount
    name: %s
    namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s-role
`, runner, ns, runner, ns, runner, ns, runner, ns, runner)
}

// RenderTokenCommand 生成签发控制面 SA 访问令牌的 kubectl 命令。
// RenderTokenCommand renders the kubectl command to mint a token for the control-plane ServiceAccount.
func RenderTokenCommand(namespace string, managerSA ...string) string {
	ns := normalizeTemplateNamespace(namespace)
	manager := normalizeManagerSA(managerSA...)
	return fmt.Sprintf("kubectl -n %s create token %s --duration=8760h", ns, manager)
}

// RenderKubeconfigCommand 生成将控制面 SA 令牌与当前集群 CA/API 封装为独立 kubeconfig 的脚本。
// RenderKubeconfigCommand renders a shell command packaging the control-plane SA token and cluster CA into a standalone kubeconfig.
func RenderKubeconfigCommand(namespace string, managerSA ...string) string {
	ns := normalizeTemplateNamespace(namespace)
	manager := normalizeManagerSA(managerSA...)
	return fmt.Sprintf(`SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CA=$(kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
TOKEN=$(kubectl -n %s create token %s --duration=8760h)
cat <<EOF
apiVersion: v1
kind: Config
clusters:
- name: stx-cluster
  cluster:
    server: ${SERVER}
    certificate-authority-data: ${CA}
contexts:
- name: stx-context
  context:
    cluster: stx-cluster
    namespace: %s
    user: %s
current-context: stx-context
users:
- name: %s
  user:
    token: ${TOKEN}
EOF`, ns, manager, ns, manager, manager)
}
