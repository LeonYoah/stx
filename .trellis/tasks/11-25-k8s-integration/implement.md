# STX Kubernetes 模式集成 — 总体执行与协作计划

> 状态：Planning Ready（评审确认后再逐个 `task.py start <child-dir>`）
> 协作模式：本地设计/审查/文档归档 + 云端 Devin 会话（`devin-01566d0d871141b9841e24249d7b6701`，Minikube v1.37 + SeaTunnel 3.1.0 + `~/repos/stx`）开发与真实 K8s 端到端联调

---

## 1. 分阶段子任务执行顺序与门禁

### Stage 1：`11-25-k8s-p1-session-foundation`（底座 + 常驻容器全流程 + 伴生 Proxy）
1. **依赖与安全底座**：
   - `go.mod` 引入 `k8s.io/client-go`、`k8s.io/api`、`k8s.io/apimachinery`；
   - 新增 `internal/utils/secretbox`（AES-256-GCM 凭据加解密，兼容旧明文回退）；
   - 完善 `internal/apps/host`（`k8s_auth_mode`、加密/脱敏、留空保留原值、`TestConnection` + `SelfSubjectAccessReview` RBAC 预检、Namespace 列举、K8s 集群自动发现）。
2. **统一 K8s 适配层（`internal/pkg/k8s`）**：
   - `K8sClientManager`（按 Host 缓存 Clientset / RESTConfig，凭据变更自动失效）；
   - `K8sTransport`（支持 `apiserver_proxy` / `in_cluster` / `external`，提供 Pod 与 Service HTTP 代理 `RoundTripper`）；
   - `K8sJavaProxyManager`（272 KB `ConfigMap.binaryData` + 同镜像独立 Deployment + 智能卷克隆跳过 RWO + `/healthz` 探活 + 复用模式）；
   - `K8sSessionManager`（纳管已有集群发现、一键部署标准 StatefulSet/Headless Service/ConfigMap/RBAC、Pod 投影 `NodeInfo`、Scale、Rollout Restart、镜像 Tag 滚动升级、ConfigMap 读写、PodLogs 读取、安全解绑/卸载）。
3. **跨模块接入**：
   - `cluster`：新增 `InfraType` / `K8sHostID` / `AutoRegistered` / `ParentClusterID` / `SourceJobInstanceID` 列（一次性建全供 P2 复用）；`health.go` 双校验（Pod Ready + `/overview`）；`webui_master.go` 走 `K8sTransport`；`stx_java_proxy.go` 分发至 `K8sJavaProxyManager`；
   - `sync`：`K8sRuntimeResolver`（无 Agent 依赖）、`GetJobLogs` K8s PodLogs 分支、隐藏 K8s `local` 执行模式；
   - `monitoring`：`collectManagedMetricsTargets` 支持 K8s 集群 `8080/metrics` + `K8sTransport`；`availability_evaluator.go` 对 K8s 跳过 `agent_offline`；
   - `config` / `plugin`：ConfigMap 读写与滚动生效；K8s 插件页只读镜像连接器视图；
   - `seatunnel/capabilities.go`：补充 K8s 能力位；
   - CLI & 前端：遵循 `.trellis/spec/frontend/`（`.arc-compact`、单层画布 `0.5px` 细线、中英双语、脏态拦截）与 `AGENTS.md` CLI 约定补齐交互与命令。
4. **Stage 1 云端实测门禁**：
   - 纳管云端已有 `seatunnel` 集群 → 伴生 Proxy 自动拉起就绪 → 工作台校验/插件表单/提交 FakeSource 作业/对账/Pod 日志/Savepoint/停止/扩缩容/ConfigMap/指标/WebUI 代理全通；
   - 云端新建 `seatunnel-stx-e2e` Namespace → 一键部署标准 StatefulSet 集群 → 提交作业 → 滚动改镜像 Tag → 一键卸载验证干净清理。

### Stage 2：`11-25-k8s-p2-app-mode`（App 模板 + STIP-41 纯 Go + 临时集群治理）
- **前置门禁**：Stage 1 合并并通过全量单测与云端实测。
- 核心交付：`K8sApplicationRunner`（纯 Go STIP-41 + OwnerRef Secret 挂载 `job.conf` + 支持 `kubernetes.master.enable-http`/`http-port`）、App 模板注册即起常驻 Proxy、提交自动登记临时集群（`auto_registered=true`）与 4 条隔离边界、批量双层融合对账 + K8s Events 故障归因、Savepoint 停止与 `restore-job-id` 恢复、`EphemeralClusterCleaner` 定期清理 + 一键回收、工作台任务级覆盖表单、`/clusters` 视图隔离与 CLI 对齐。
- **Stage 2 云端实测门禁**：在云端 `seatunnel-apps` Namespace 跑通批作业 + 长跑流作业提交、`/clusters` 临时集群视角运维、Master HTTP 观测、Savepoint 停止与 `restore-job-id` 恢复、自动/手动清理后历史作业零断链。

### Stage 3：`11-25-k8s-p3-diagnostics-obs`（无 Agent 诊断与观测增强）
- **前置门禁**：Stage 1 完成（推荐 Stage 2 后一并覆盖 App Pod）。
- 核心交付：`K8sErrorLogCollector`（诊断中心错误日志聚合）、`pods/exec` JVM Thread Dump（`jcmd 1 Thread.print`）、可选 `metrics-server` Pod CPU/Mem 采集、全量回归与 `.trellis/spec/` 规范归档。

---

## 2. 统一验证命令

```bash
# 后端根模块（含 -race 并发测试）
go test ./...
go test -race ./internal/pkg/k8s/... ./internal/apps/sync/... ./internal/apps/cluster/...

# Agent 模块回归（确认零破坏）
(cd agent && go test ./...)

# 前端类型、单测与国际化覆盖率
(cd frontend && pnpm exec tsc --noEmit)
(cd frontend && pnpm test)
(cd frontend && pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts)
```

---

## 3. 高风险改动点与回滚策略

| 风险文件 / 域 | 风险点 | 防护与回滚点 |
| --- | --- | --- |
| `internal/apps/host/service.go` | 凭据加密引入后旧明文兼容与空值覆盖 | 密文统一带 `enc:v1:` 前缀，解密遇无前缀按明文平滑兼容；单测锁住更新留空不覆盖 |
| `internal/apps/sync/engine_client.go` & `job_reconciler.go` | K8s Transport / App 批量对账影响物理机退避与止损语义 | 按 `infra_type` 明确路由分支，保留原 `bare_metal` 路径零改动；`-race` 全量回归 |
| `internal/apps/cluster/` & `/clusters` 前端 | 临时集群污染主列表或删除集群误删用户 K8s 资源 | `management_source=adopted` 强制禁止删用户 Workload；列表默认过滤 `auto_registered=false` |
| `internal/apps/monitoring/` | K8s 8080 指标抓取或临时集群误触发 `agent_offline` / `master_unavailable` | 按 `infra_type` 显式门控告警规则，单测覆盖 4 种 `infra_type` 矩阵 |
