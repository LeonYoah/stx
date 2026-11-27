# 全局部署模式领域模型与术语契约

> 本文档定义 SeaTunnelX 全站统一的集群部署模式、集群架构、基础设施判定单一真源与双身份（控制面 / 引擎运行）边界，适用于 Go 后端、Next.js 前端与 CLI。

---

## 1. 正交维度与标准术语

| 维度 | 领域含义 | 规范展示名 | 持久化字段（向后兼容） |
|---|---|---|---|
| **部署模式**（运行底座 × 生命周期） | 面向用户的一等产品概念，直接对齐 Apache Flink 与上游 SeaTunnel `MasterType` | `Standalone`、`Kubernetes Session`、`Kubernetes Application`、`YARN Session`、`YARN Application` | `clusters.infra_type`（空值按 `bare_metal` 处理；兼容历史过渡值 `kubernetes`） |
| **集群架构** | Master 与 Worker 是否同进程（仅 Session 集群生效） | 中文：`集群架构`（`混合架构` / `分离架构`）；英文：`Cluster Architecture`（`Hybrid` / `Separated`） | `clusters.deployment_mode`（`hybrid` / `separated`，接口与 CLI 参数名不变） |
| **管理来源** | Session 集群由 STX 部署还是纳管已有（仅 Session 生效） | 托管部署 / 纳管已有 | `config.kubernetes.management_source`（`stx_deployed` / `adopted`） |

### 术语红线
- **禁止在界面文案中使用「拓扑」一词**：原 `hybrid / separated` 统一称 **集群架构（混合架构 / 分离架构）**；作业执行 DAG 视图统一称 **流程图 / 执行流程图**（英文 `Execution Graph`）。
- **部署模式统一沿用行业通用英文专有名词**：`Standalone`、`Kubernetes Session`、`Kubernetes Application`、`YARN Session`、`YARN Application`，不另造中英斜杠混拼文案。
- **主机管理**暂保持原名称（`/hosts`、`/api/v1/hosts`、`stx host`），待引入 YARN 底座时再评估统一更名。

---

## 2. 单一真源与硬门禁

- **Go 后端**：`internal/pkg/infra`（`IsBareMetal` / `IsK8s` / `IsK8sSession` / `IsYarn` / `IsApplication` / `IsSession` / `PlatformOf` / `ExecutionModeOf` / `DeployModeOf`）。
- **前端**：`frontend/lib/infra.ts`（`isBareMetalInfra` / `isK8sInfra` / `isK8sSessionInfra` / `isYarnInfra` / `isApplicationInfra` / `isSessionInfra` / `getRuntimePlatform` / `getClusterExecutionMode` / `getClusterDeployMode` / `getClusterDeployModeLabel`）。
- **ESLint 硬门禁（`frontend/eslint.config.mjs`）**：禁止业务代码直接写 `infra_type === 'bare_metal'`、`infra_type !== 'bare_metal'` 或 `infra_type === 'kubernetes'`，防止引入 YARN 或 Application 模式时把非 `bare_metal` 全部误判为 Kubernetes。

---

## 3. 权限与双 SA 职责边界

- **控制面身份（STX 用，`stx-manager`）**：仅用于 STX 连接并操作目标命名空间。主机接入 Step 1–2 仅聚焦控制面账号，单命名空间最小权限 YAML 严禁混入引擎 SA 或 `ClusterRole`。
- **引擎运行身份（SeaTunnel Pod 用）**：
  - **Kubernetes Session**：默认采用 `service-dns` 发现，不调用 K8s API；引擎 SA（`seatunnel-runner`）为**可选**（主机保存完成后及详情页支持独立预备并一键检测绑定默认，后续部署自动复用；严禁复用 `stx-manager`）。
  - **Kubernetes Application（P2）**：作业无需提前注册常驻集群，Master Pod 需动态创建/回收 Worker Pod（上游 `seatunnel-application` Role：`pods` 增删查监控 + `batch/jobs` 读取），引擎 SA 为**必填项**（不可静默回退 `default`）。
