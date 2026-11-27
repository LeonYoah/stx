# STX Kubernetes 模式集成（常驻容器与 App 模式）

> 状态：Planning Ready（待评审确认后启动子任务 1 实施）
> 类型：父任务（协调 3 个可独立验收子任务）
> 关联调研：[research/seatunnel-k8s-and-stx-gaps.md](./research/seatunnel-k8s-and-stx-gaps.md)
> 完整架构设计：[design.md](./design.md)
> 总体执行计划：[implement.md](./implement.md)

## 1. 背景与目标

STX 当前完整支持物理机/VM（`bare_metal` + Agent）形态下的 SeaTunnel 集群管理、数据同步工作台、作业对账、日志查看、监控告警与 Web UI 代理。随着 Apache SeaTunnel 3.x（特别是 3.1.0+）在 Kubernetes 上成熟出两类官方运行形态，STX 需要以统一架构支持：

1. **常驻容器模式（`kubernetes_session`）**：涵盖 **纳管已有集群（`adopted`）** 与 **STX 一键部署标准 StatefulSet 集群（`stx_deployed`）** 两种来源，基于 Master/Worker StatefulSet + Headless Service + ConfigMap 运行，多作业共享资源，经 Master REST API v2（`:8080`）提交与运维。
2. **App 模式（STIP-41 Native Zeta Application Mode）**：基于 **App 模板（`kubernetes_application_template`）** 与 **自动登记临时集群实例（`kubernetes_application_instance`, `auto_registered=true`）** 双层模型，按作业动态拉起 `batch/v1 Job`（Master Pod）+ 固定数量 Worker Pods，资源按作业强隔离、结束自动 GC，并支持长跑实时作业直接在集群管理页统一治理。

**总目标**：在不破坏存量 `bare_metal` 架构与接口契约的前提下，从一开始拉通全部 K8s 架构边界与数据模型契约（避免分期返工），分 3 个独立可验收的子任务落地并在云端真实 Minikube + SeaTunnel 3.1.0 环境完成端到端实测验证。

---

## 2. 已确认的核心产品与架构决策（全部收敛）

| 决策项 | 最终结论 | 关键约束 |
| --- | --- | --- |
| **Q1 · 分期与任务树** | 父任务 + 3 个顺序子任务（架构与数据模型一次性定稿防返工） | P1 底座+常驻全流程+伴生 Proxy → P2 App 模式全流程 → P3 无 Agent 诊断/监控增强 |
| **Q2 · 伴生 `stx-java-proxy`** | Phase 1 内置「272 KB `ConfigMap.binaryData` + 同镜像独立伴生 Deployment」 | 「测试连接」纯只读 + `SelfSubjectAccessReview` RBAC 预检；「保存注册」即自动拉起常驻 Proxy 并 `/healthz` 探活；App 模板及派生的临时实例统一共享该 Proxy，亦支持同 Host/Namespace 多模板复用 |
| **Q3 · App 提交与临时集群** | 纯 Go `client-go` 原生 STIP-41 + `clusters` 自动登记临时集群（`auto_registered=true`） | `/clusters` 视图隔离（默认只看受管集群与模板，独立分段看 App 实例）；工作台下拉框严禁选临时实例；`sync_job_instances.cluster_id` 永锚父模板 ID 防清理断链；K8s TTL + `EphemeralClusterCleaner` 双层定期清理 + 一键回收 |
| **Q4 · 常驻一键部署与插件** | Phase 1 同时涵盖「纳管已有 (`adopted`)」与「一键部署标准 StatefulSet (`stx_deployed`)」；**砍掉 K8s 动态装插件** | 坚定遵循容器不可变镜像原则：不侵入镜像、不支持向运行中容器动态装/卸插件；插件页提供由伴生 Proxy 扫描的**只读镜像连接器视图**，增减插件通过修改镜像 Tag 滚动升级完成 |

---

## 3. 子任务拆解与验收映射

| 顺序 | 子任务目录 | 核心交付 | 前置依赖 |
| --- | --- | --- | --- |
| 1 | `11-25-k8s-p1-session-foundation` | K8s 基础设施底座（`client-go`、`K8sTransport`、Host 凭据加密/连通/RBAC 预检）+ 常驻容器全流程（纳管 + 一键部署标准 StatefulSet + 镜像滚动升级 + 扩缩容/ConfigMap/作业/日志/8080 指标/WebUI 代理/只读插件视图）+ 伴生 `stx-java-proxy` 托管 + 前端 & CLI | 无（首个交付） |
| 2 | `11-25-k8s-p2-app-mode` | App 模板（`kubernetes_application_template`，保存即起常驻 Proxy）+ 纯 Go STIP-41 提交器（Secret 挂载 `job.conf` + 支持 Master HTTP）+ 自动登记临时集群（`kubernetes_application_instance`）及 4 条隔离边界 + 双层融合对账/Events/Pod Logs/Savepoint & `restore-job-id` 恢复 + 双层定期清理与一键回收 + 前端 & CLI | 依赖 P1 提供的 `internal/pkg/k8s`、伴生 Proxy 与 `Cluster.InfraType` 底座 |
| 3 | `11-25-k8s-p3-diagnostics-obs` | 无 Agent 诊断与观测增强：控制面 `K8sErrorLogCollector` 错误日志聚合、`pods/exec` JVM Thread Dump（`jcmd 1 Thread.print`）、可选 `metrics-server` Pod CPU/Mem 指标采集、全量 E2E 回归与 `.trellis/spec/` 规范归档 | 依赖 P1（P2 完成后同步覆盖 App Pod） |

---

## 4. 跨子任务总验收标准（Cross-Child AC）

- [ ] **AC-1（存量零回归与安全）**：存量 `bare_metal` 集群、节点、作业、监控、告警、CLI 行为 100% 无回归；K8s `kubeconfig`/`token` 加密落库、接口脱敏（永不回显明文）、更新留空保留原值。
- [ ] **AC-2（K8s 底座与伴生 Proxy）**：Web UI 与 CLI 支持创建/测试 `kubernetes` Host；注册 K8s 常驻集群或 App 模板保存时自动以 272 KB ConfigMap 拉起常驻伴生 `stx-java-proxy` 并探活通过；在 App 模板 0 作业状态下工作台测试连接、Catalog 探测、插件表单、校验、预览 DAG 全部可用。
- [ ] **AC-3（常驻容器端到端，含纳管 + 一键部署）**：
  - 在云端 Minikube 成功纳管现有 `seatunnel` 集群，跑通作业提交/对账/Pod 日志/Savepoint/恢复/扩缩容/ConfigMap/8080 指标/WebUI 代理/只读插件视图；
  - 在云端 Minikube 独立 Namespace 成功执行「一键部署标准 StatefulSet 集群 → 改镜像 Tag 滚动升级 → 解绑/卸载清理」全流程。
- [ ] **AC-4（App 模式端到端，含长跑实时集群治理）**：
  - 在云端 Minikube 创建 `seatunnel-apps` App 模板，提交批与流作业，自动生成临时集群记录并在 `/clusters`「App 运行实例」视角可见、主视图与工作台目标选择器严格隔离；
  - 验证双层对账（Job/Pod + Master HTTP `/job-info` + K8s Events）、Master/Worker Pod Logs、Savepoint 停止与 `restore-job-id` 新 jobId 恢复、以及过期临时集群自动清理后 `sync_job_instances` 历史零断链。
- [ ] **AC-5（无 Agent 诊断补齐与质量门禁）**：诊断中心可聚合 K8s Pod 错误日志并执行 `pods/exec` Thread Dump；全量 `go test ./...`、`(cd agent && go test ./...)`、前端 `tsc --noEmit` / `pnpm test` / i18n 覆盖测试 / CLI 单测通过，且 `.trellis/spec/` 完成 K8s 规范沉淀。

---

## 5. 明确非目标（Out of Scope）

- **不在 K8s Pod 内植入 STX Host Agent**（彻底无 Agent 架构）。
- **不做 K8s 容器内动态安装/卸载插件或 InitContainer 插件下载 Overlay**（坚定不可变镜像原则，避免侵入镜像与版本漂移）。
- **不自研 Kubernetes CRD / SeaTunnel Operator**（直接使用原生 `StatefulSet`、`Deployment`、`batch/v1 Job`、`Service`、`ConfigMap`、`Secret`）。
- **不覆盖 Docker Compose / `HostTypeDocker`**。
- **不开放容器内 `jmap` Heap Dump**（容器 CGroup 内存与临时盘空间受限，高 OOMKill 风险）。
- **不覆盖宿主机级硬件指标**（磁盘 IOPS、网卡吞吐、宿主机挂载点，无 Agent/node-exporter 边界明确）。
