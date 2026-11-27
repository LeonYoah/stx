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

// Kubernetes 两级 RBAC 授权模板生成器：与后端 internal/pkg/k8s/rbac_templates.go 保持逐条对齐。
// Kubernetes two-tier RBAC YAML generator aligned 1:1 with backend internal/pkg/k8s/rbac_templates.go.

export const DEFAULT_STX_MANAGER_SA = 'stx-manager';
export const DEFAULT_SEATUNNEL_RUNNER_SA = 'seatunnel-runner';

function normalizeNamespace(namespace?: string): string {
  const trimmed = (namespace || '').trim();
  return trimmed || 'seatunnel';
}

function normalizeRunnerSA(runnerServiceAccount?: string): string {
  const trimmed = (runnerServiceAccount || '').trim();
  return trimmed || DEFAULT_SEATUNNEL_RUNNER_SA;
}

function normalizeManagerSA(managerServiceAccount?: string): string {
  const trimmed = (managerServiceAccount || '').trim();
  return trimmed || DEFAULT_STX_MANAGER_SA;
}

// sanitizeK8sToken 去除从终端粘贴时带入的换行与首尾空白，避免整段 JWT 被换行切碎导致 401。
// sanitizeK8sToken strips embedded line breaks and outer whitespace introduced by terminal copy/paste.
export function sanitizeK8sToken(raw?: string): string {
  return (raw || '').replace(/[\r\n]+/g, '').trim();
}

// looksTruncatedK8sToken 温和判断令牌是否疑似在终端复制时被截断（如仅复制了 JWT 其中一段）。
// looksTruncatedK8sToken heuristically flags likely truncated JWT tokens (non-blocking UX warning only).
export function looksTruncatedK8sToken(raw?: string): boolean {
  const cleaned = sanitizeK8sToken(raw);
  if (!cleaned) {
    return false;
  }
  const parts = cleaned.split('.');
  if (parts.length !== 3 || parts.some((p) => p.length === 0)) {
    return true;
  }
  return cleaned.length < 100;
}

// renderMinimalK8sRbacYaml 生成单命名空间最小权限模板（控制面 SA 与引擎运行身份严格隔离）。
// renderMinimalK8sRbacYaml renders the single-namespace minimal RBAC YAML with strict SA isolation.
export function renderMinimalK8sRbacYaml(
  namespace?: string,
  runnerServiceAccount?: string,
  managerServiceAccount?: string,
): string {
  const ns = normalizeNamespace(namespace);
  const runner = normalizeRunnerSA(runnerServiceAccount);
  const manager = normalizeManagerSA(managerServiceAccount);
  return `# STX 控制面单命名空间最小权限模板 / STX single-namespace minimal RBAC template
# 1) ${manager}: STX 控制面管理本命名空间工作负载与代理 / manages workloads and proxy in namespace "${ns}"
# 2) ${runner}: SeaTunnel 引擎 Pod 只读身份（仅 Hazelcast 节点发现）/ read-only identity for engine pods (Hazelcast discovery only)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${manager}
  namespace: ${ns}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: ${manager}-role
  namespace: ${ns}
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
  name: ${manager}-binding
  namespace: ${ns}
subjects:
  - kind: ServiceAccount
    name: ${manager}
    namespace: ${ns}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: ${manager}-role
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${runner}
  namespace: ${ns}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: ${runner}-role
  namespace: ${ns}
rules:
  - apiGroups: [""]
    resources: ["endpoints", "pods", "services"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ${runner}-binding
  namespace: ${ns}
subjects:
  - kind: ServiceAccount
    name: ${runner}
    namespace: ${ns}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: ${runner}-role`;
}

// renderFullK8sRbacYaml 生成跨命名空间全面权限模板（支持跨命名空间选择与按集群自动创建 RBAC）。
// renderFullK8sRbacYaml renders the multi-namespace full RBAC YAML enabling multi-NS and auto per-cluster RBAC.
export function renderFullK8sRbacYaml(
  namespace?: string,
  managerServiceAccount?: string,
): string {
  const ns = normalizeNamespace(namespace);
  const manager = normalizeManagerSA(managerServiceAccount);
  return `# STX 控制面跨命名空间全面权限模板 / STX multi-namespace full RBAC template
# 1) ${manager}: 跨命名空间调度 SeaTunnel 集群，并按集群自动创建隔离的 ServiceAccount/Role/RoleBinding
# 1) ${manager}: manages SeaTunnel across namespaces and auto-provisions isolated per-cluster RBAC
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${manager}
  namespace: ${ns}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${manager}-cluster-role
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
  name: ${manager}-cluster-binding
subjects:
  - kind: ServiceAccount
    name: ${manager}
    namespace: ${ns}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ${manager}-cluster-role`;
}

// renderK8sTokenCommand 生成签发控制面 SA Token 的 kubectl 命令。
// renderK8sTokenCommand builds the kubectl create token command for the control-plane ServiceAccount.
export function renderK8sTokenCommand(
  namespace?: string,
  managerServiceAccount?: string,
): string {
  const ns = normalizeNamespace(namespace);
  const manager = normalizeManagerSA(managerServiceAccount);
  return `kubectl -n ${ns} create token ${manager} --duration=8760h`;
}

// renderK8sExportKubeconfigCommand 生成导出当前上下文并内嵌证书的 kubeconfig 命令。
// renderK8sExportKubeconfigCommand returns the kubectl command that flattens certificates into a standalone kubeconfig.
export function renderK8sExportKubeconfigCommand(): string {
  return 'kubectl config view --raw --flatten --minify';
}

// renderK8sGenerateKubeconfigCommand 将新建控制面 SA 的 Token 与当前集群 API/CA 封装为独立 kubeconfig。
// renderK8sGenerateKubeconfigCommand generates a shell snippet packaging the minted SA token and cluster CA into a standalone kubeconfig.
export function renderK8sGenerateKubeconfigCommand(
  namespace?: string,
  managerServiceAccount?: string,
): string {
  const ns = normalizeNamespace(namespace);
  const manager = normalizeManagerSA(managerServiceAccount);
  return `SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CA=$(kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
TOKEN=$(kubectl -n ${ns} create token ${manager} --duration=8760h)
cat <<EOF
apiVersion: v1
kind: Config
clusters:
- name: stx-cluster
  cluster:
    server: \${SERVER}
    certificate-authority-data: \${CA}
contexts:
- name: stx-context
  context:
    cluster: stx-cluster
    namespace: ${ns}
    user: ${manager}
current-context: stx-context
users:
- name: ${manager}
  user:
    token: \${TOKEN}
EOF`;
}
