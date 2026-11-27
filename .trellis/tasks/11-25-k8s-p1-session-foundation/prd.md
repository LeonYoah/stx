# P1：K8s 底座、常驻容器模式（纳管+一键部署）与伴生 Proxy

> 父任务：[11-25-k8s-integration](../11-25-k8s-integration/prd.md)
> 完整架构契约：[../11-25-k8s-integration/design.md](../11-25-k8s-integration/design.md)
> 本子任务详细设计与清单：[design.md](./design.md) | [implement.md](./implement.md)
> 前置依赖：无（首个可独立验收交付）

## 1. 目标

打通 STX 接入 Kubernetes 的公共底座，并交付完整的 **K8s 常驻容器模式（`kubernetes_session`）**（同时支持 **纳管已有集群 `adopted`** 与 **一键部署标准 StatefulSet 集群 `stx_deployed`**）以及 **272 KB ConfigMap 伴生 `stx-java-proxy` 托管**，在云端 Minikube + SeaTunnel 3.1.0 环境跑通全链路闭环。

## 2. 需求范围

1. **K8s 基础设施底座（`internal/pkg/k8s` + `host`）**
   - `Host(host_type=kubernetes)` 支持 `kubeconfig` / `token` / `in_cluster` 三种认证模式；凭据使用 AES-256-GCM 加密存储（兼容历史明文平滑升级），接口永不回显明文，编辑留空保留原值。
   - 新增连通性测试与只读 RBAC 预检（`SelfSubjectAccessReview` 检查 Pods/StatefulSets/Deployments/Services/ConfigMaps/Secrets/Jobs/`pods/proxy`/`pods/log`），不产生任何残留 K8s 对象；提供 Namespace 列举与已有 SeaTunnel 集群发现。
   - 提供统一 `K8sClientManager` 与 `K8sTransport`（默认 `apiserver_proxy`，支持 `in_cluster` / `external`）。
2. **Cluster 模型一次性扩展（兼容 P2 App 模式防返工）**
   - `clusters` 表新增 `infra_type`（默认 `bare_metal`）、`k8s_host_id`、`auto_registered`、`parent_cluster_id`、`source_job_instance_id`。
   - 存量 `bare_metal` 路径 100% 兼容无回归。
3. **常驻伴生 `stx-java-proxy` 托管（`K8sJavaProxyManager`）**
   - 保存/注册 K8s 集群时默认自动将 `lib/stx-java-proxy-v{2,3}.jar`（~272 KB）写入目标 Namespace `ConfigMap.binaryData`（带 SHA256 annotation），拉起 1 副本同镜像独立伴生 `Deployment` + `Service(:18080)` 并完成 `/healthz` 探活。
   - 自动克隆目标 Workload 的 `image`、`imagePullSecrets`、`env`/`envFrom` 及非 `RWO` 插件/配置/Checkpoint 卷挂载；支持 `managed` / `reuse_cluster` / `external` / `disabled`。
4. **常驻容器模式全流程（`adopted` + `stx_deployed` 统一模型）**
   - **纳管已有集群（`adopted`）**：发现已有 StatefulSet/Deployment、Headless Service、ConfigMap；删除 STX 集群只清理 STX 伴生 Proxy，绝不删用户原集群。
   - **一键部署标准 StatefulSet 集群（`stx_deployed`）**：按上游生产规范创建可选 RBAC、ConfigMap、Headless Services、Master/Worker StatefulSet（PID 1 直接 `exec`、探针、`terminationGracePeriodSeconds: 120`）与伴生 Proxy；卸载时可显式选择清理 STX 创建的 K8s 资源（绝不删外部 PVC）。
   - **全链路运维、同步与观测**：Pod 动态投影 `NodeInfo`、Pod Ready + `/overview` 双重健康检查、扩缩容、滚动重启、**不可变镜像 Tag 滚动升级**（同步滚动伴生 Proxy）、ConfigMap 读写与滚动生效、同步作业提交/对账/停止/Savepoint/恢复（复用 REST v2 + `K8sTransport`）、PodLogs 日志读取、8080 `/metrics` 指标抓取、K8s 可用性告警（跳过 `agent_offline`）、Active Master WebUI 代理、插件页只读镜像连接器视图。
5. **前端与 CLI 对齐**
   - 前端严格遵循 `.trellis/spec/frontend/`（`.arc-compact`、单层画布 `0.5px` 细线、中英双语、脏态拦截）；工作台对 K8s 集群隐藏 `local` 模式。
   - CLI 补齐 K8s Host 创建/测试/发现、K8s Session 集群创建（纳管/部署）/扩缩容/滚动重启/镜像升级。

## 3. 验收标准

- [ ] **AC-1**：`bare_metal` 存量单测与核心路径零回归；K8s 凭据密文落库、查询脱敏、更新留空保留原值均有单测覆盖。
- [ ] **AC-2**：`POST /api/v1/hosts/:id/test-connection` 对云端 Minikube 返回真实 K8s 版本与 RBAC 检查项，且不创建任何 K8s 资源。
- [ ] **AC-3（纳管闭环）**：纳管云端 `seatunnel` 集群，保存时自动拉起伴生 `stx-java-proxy` 且 `/healthz` 通过；工作台可完成插件列表/OptionRule Schema/配置校验/FakeSource 作业提交/对账/Pod 日志/Savepoint/停止；可完成扩缩容、ConfigMap 查看、8080 指标抓取、WebUI 代理；解绑删除不删用户 `seatunnel` StatefulSet。
- [ ] **AC-4（一键部署闭环）**：在云端新建 `seatunnel-stx-e2e` Namespace，经 STX 一键部署 1 Master + 1 Worker StatefulSet + 伴生 Proxy → 跑通作业 → 执行镜像滚动更新 → 勾选清理 K8s 资源完成卸载。
- [ ] **AC-5（插件只读边界）**：K8s 集群插件页仅展示镜像内连接器只读清单与镜像升级引导，不显示上传/安装/卸载写操作。
- [ ] **AC-6（质量与 CLI）**：对应 `stx` CLI 命令可用且有单测；`go test ./...`、`(cd agent && go test ./...)`、前端 `tsc --noEmit` / `pnpm test` / `translation-coverage.test.ts` 全绿。
