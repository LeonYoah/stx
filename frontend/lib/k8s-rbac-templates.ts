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

// Kubernetes 独立 RBAC 授权模板生成器：严格拆分 STX 控制面账号与 SeaTunnel 引擎运行账号，
// 并与后端 internal/pkg/k8s/rbac_templates.go 保持逐条对齐。
// Split Kubernetes RBAC YAML generator isolating STX control-plane SA from SeaTunnel engine runner SA,
// aligned 1:1 with backend internal/pkg/k8s/rbac_templates.go.

export const DEFAULT_STX_MANAGER_SA = 'stx-manager';
export const DEFAULT_SEATUNNEL_RUNNER_SA = 'seatunnel-runner';

export interface ControlPlaneK8sRbacOptions {
  includeDeleteRules?: boolean;
  includeSecretRules?: boolean;
}

export interface K8sCleanupCommandOptions {
  includeClusterRole?: boolean;
  runnerServiceAccount?: string;
  includeRunner?: boolean;
}

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

// renderControlPlaneK8sRbacYaml 生成单命名空间 STX 控制面权限模板（不包含引擎运行账号）。
// renderControlPlaneK8sRbacYaml renders pure single-namespace STX control-plane RBAC YAML (no engine runner SA).
export function renderControlPlaneK8sRbacYaml(
  namespace?: string,
  managerServiceAccount?: string,
  options?: ControlPlaneK8sRbacOptions,
): string {
  const ns = normalizeNamespace(namespace);
  const manager = normalizeManagerSA(managerServiceAccount);
  const {
    includeDeleteRules = false,
    includeSecretRules = false,
  } = options || {};

  const optionalBlocks: string[] = [];
  if (includeDeleteRules) {
    optionalBlocks.push(`  # --- 可选块：STX 页面一键卸载、失败自动回收与单 Pod 重建；不勾选时缩容至 0 并提示按标签清理命令 ---
  # --- Optional: UI uninstall, failed deploy reclamation & single-pod recreate ---
  - apiGroups: ["apps"]
    resources: ["statefulsets", "deployments"]
    verbs: ["delete"]
  - apiGroups: [""]
    resources: ["services", "configmaps", "pods"]
    verbs: ["delete"]`);
  }
  if (includeSecretRules) {
    optionalBlocks.push(`  # --- 可选块：STX 自动代建私仓拉取 Secret；由运维预建 imagePullSecrets 时无需勾选 ---
  # --- Optional: STX auto pull-secret provisioning; skip when admins pre-create imagePullSecrets ---
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "create", "update", "delete"]`);
  }

  const optionalYaml = optionalBlocks.length > 0 ? `\n${optionalBlocks.join('\n')}` : '';

  return `# STX 控制面单命名空间权限模板 / STX single-namespace control-plane RBAC template
# 仅授权 STX 控制面在当前命名空间内部署、更新与读取日志 / Scoped strictly to control-plane deploy, update, and logs in one namespace
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
    verbs: ["list"]${optionalYaml}
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
  name: ${manager}-role`;
}

// renderSeaTunnelRunnerK8sRbacYaml 生成独立的 SeaTunnel 引擎运行账号模板（仅含命名空间只读发现权限）。
// renderSeaTunnelRunnerK8sRbacYaml renders standalone read-only RBAC YAML for SeaTunnel engine Pods.
export function renderSeaTunnelRunnerK8sRbacYaml(
  namespace?: string,
  runnerServiceAccount?: string,
): string {
  const ns = normalizeNamespace(namespace);
  const runner = normalizeRunnerSA(runnerServiceAccount);
  return `# SeaTunnel 引擎运行账号权限模板（与 STX 控制面账号完全独立）
# Standalone SeaTunnel engine runner RBAC template (isolated from STX control-plane SA)
# 仅在启用 K8s API 节点发现或需要绑定专属 Pod 身份时应用；默认 DNS 服务发现无需任何 K8s API 权限
# Apply only when using K8s API discovery or dedicated pod identity; default DNS discovery needs zero K8s API permissions
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

// renderK8sCleanupCommand 生成从目标 Kubernetes 集群手动清理 STX 控制面授权对象的命令；
// 仅在明确勾选 includeRunner 时才附带引擎运行账号清理，防止误删存量集群使用的 ServiceAccount。
// renderK8sCleanupCommand builds kubectl cleanup commands for the control-plane SA;
// engine runner SA cleanup is only emitted when includeRunner=true to avoid breaking running clusters.
export function renderK8sCleanupCommand(
  namespace?: string,
  managerServiceAccount?: string,
  options?: K8sCleanupCommandOptions,
): string {
  const ns = normalizeNamespace(namespace);
  const manager = normalizeManagerSA(managerServiceAccount);
  const lines = [
    `kubectl -n ${ns} delete rolebinding ${manager}-binding --ignore-not-found`,
    `kubectl -n ${ns} delete role ${manager}-role --ignore-not-found`,
    `kubectl -n ${ns} delete serviceaccount ${manager} --ignore-not-found`,
  ];
  if (options?.includeClusterRole) {
    lines.push(
      `kubectl delete clusterrolebinding ${manager}-cluster-binding --ignore-not-found`,
      `kubectl delete clusterrole ${manager}-cluster-role --ignore-not-found`,
    );
  }
  if (options?.includeRunner) {
    const runner = normalizeRunnerSA(options.runnerServiceAccount);
    lines.push(
      `kubectl -n ${ns} delete rolebinding ${runner}-binding --ignore-not-found`,
      `kubectl -n ${ns} delete role ${runner}-role --ignore-not-found`,
      `kubectl -n ${ns} delete serviceaccount ${runner} --ignore-not-found`,
    );
  }
  return lines.join('\n');
}
