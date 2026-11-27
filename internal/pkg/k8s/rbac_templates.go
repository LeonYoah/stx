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

// RenderMinimalRBACYAML 生成单命名空间最小权限 RBAC YAML（控制面 managerSA 与引擎运行账号 runnerSA 严格隔离）。
// RenderMinimalRBACYAML renders a single-namespace minimal RBAC YAML separating the STX control-plane
// identity (managerSA, defaulting to stx-manager) from the read-only SeaTunnel runner ServiceAccount.
func RenderMinimalRBACYAML(namespace, runnerSA string, managerSA ...string) string {
	ns := normalizeTemplateNamespace(namespace)
	runner := normalizeRunnerSA(runnerSA)
	manager := normalizeManagerSA(managerSA...)
	return fmt.Sprintf(`# STX 控制面单命名空间最小权限模板 / STX single-namespace minimal RBAC template
# 1) %s: STX 控制面管理本命名空间工作负载与代理 / manages workloads and proxy in namespace %q
# 2) %s: SeaTunnel 引擎 Pod 只读身份（仅 Hazelcast 节点发现）/ read-only identity for engine pods (Hazelcast discovery only)
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
    resources: ["pods"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["pods/proxy"]
    verbs: ["get", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "create", "update", "delete"]
  - apiGroups: [""]
    resources: ["services/proxy"]
    verbs: ["get", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "create", "update", "delete"]
  - apiGroups: [""]
    resources: ["serviceaccounts", "persistentvolumeclaims"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["statefulsets", "deployments"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apps"]
    resources: ["replicasets"]
    verbs: ["get", "list"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["get", "list", "create", "delete"]
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
---
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
`,
		manager, ns,
		runner,
		manager, ns,
		manager, ns,
		manager, ns, manager, ns, manager,
		runner, ns,
		runner, ns,
		runner, ns, runner, ns, runner,
	)
}

// RenderFullRBACYAML 生成跨命名空间全面权限 RBAC YAML（支持多命名空间选择与按集群自动创建专属 RBAC）。
// RenderFullRBACYAML renders a multi-namespace full RBAC YAML enabling cross-namespace discovery
// and automatic per-cluster SeaTunnel RBAC provisioning.
func RenderFullRBACYAML(namespace string, managerSA ...string) string {
	ns := normalizeTemplateNamespace(namespace)
	manager := normalizeManagerSA(managerSA...)
	return fmt.Sprintf(`# STX 控制面跨命名空间全面权限模板 / STX multi-namespace full RBAC template
# 1) %s: 跨命名空间调度 SeaTunnel 集群，并按集群自动创建隔离的 ServiceAccount/Role/RoleBinding
# 1) %s: manages SeaTunnel across namespaces and auto-provisions isolated per-cluster RBAC
apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s-cluster-role
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "create"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["pods/proxy"]
    verbs: ["get", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "create", "update", "delete"]
  - apiGroups: [""]
    resources: ["services/proxy"]
    verbs: ["get", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "create", "update", "delete"]
  - apiGroups: [""]
    resources: ["serviceaccounts"]
    verbs: ["get", "list", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["statefulsets", "deployments"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apps"]
    resources: ["replicasets"]
    verbs: ["get", "list"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["get", "list", "create", "delete"]
  - apiGroups: ["rbac.authorization.k8s.io"]
    resources: ["roles", "rolebindings"]
    verbs: ["get", "list", "create", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s-cluster-binding
subjects:
  - kind: ServiceAccount
    name: %s
    namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %s-cluster-role
`,
		manager,
		manager,
		manager, ns,
		manager,
		manager,
		manager, ns,
		manager,
	)
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
