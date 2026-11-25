# P3：K8s 无 Agent 诊断与观测增强

> 父任务：[11-25-k8s-integration](../11-25-k8s-integration/prd.md)
> 完整架构契约：[../11-25-k8s-integration/design.md](../11-25-k8s-integration/design.md)
> 前置依赖：依赖 `11-25-k8s-p1-session-foundation`（建议在 `11-25-k8s-p2-app-mode` 后一并覆盖常驻与 App Pod）。

## 1. 目标

补齐 K8s 无 `stx agent` 环境下诊断中心与容器监控的最后 3 块增强能力，并完成全量 K8s 架构规范沉淀至 `.trellis/spec/`。

## 2. 核心需求

1. **控制面错误日志聚合（`K8sErrorLogCollector`）**
   - 替代物理机 Agent 的 `diagnostics/error_collector.go`：后台按 `SinceTime` 游标定时增量拉取受管 K8s Pod（常驻 Master/Worker 与活跃 App Master/Worker）的 `PodLogs`，复用现有 `ERROR/FATAL` 正则解析与错误归组逻辑写入诊断中心错误表。
2. **容器内 JVM Thread Dump（`pods/exec`）**
   - 通过 `client-go` `remotecommand`（`pods/exec`）在目标容器内执行 `jcmd 1 Thread.print`（回退 `jstack 1`），接入现有诊断任务与报告视图；对缺失 `pods/exec` RBAC 或精简 JRE 镜像给出明确原因码，不崩溃；容器内 `jmap` Heap Dump 显式标记受限不支持。
3. **可选 `metrics-server` Pod CPU/Mem 指标采集**
   - 探测目标 K8s 集群是否注册 `v1beta1.metrics.k8s.io`：可用时周期采集受管 SeaTunnel Pod 与关联 Node 的 CPU/Memory 使用量写入 `embedts`；未安装时静默降级展示 JVM 指标与 K8s 配额，不产生错误告警。
4. **规范归档与端到端回归**
   - 将 K8s 架构契约、凭据加密、伴生 Proxy 克隆规则、STIP-41 契约与隔离边界沉淀至 `.trellis/spec/backend/` 与 `.trellis/spec/frontend/`。

## 3. 验收标准

- [ ] **AC-1**：云端 Minikube 触发 SeaTunnel 错误日志后，诊断中心「错误日志」自动聚合出现对应 K8s Pod 错误组，且重启 STX 不重复归档。
- [ ] **AC-2**：在云端 `apache/seatunnel:3.1.0` Pod 上通过 STX 诊断中心成功采集 Thread Dump 并在前端/CLI 展示；模拟无 `pods/exec` 权限或无 `jcmd` 时返回清晰受限提示。
- [ ] **AC-3**：`metrics-server` 未开启时不报错降级；开启 `minikube addons enable metrics-server` 后可查询到 Pod CPU/Memory 指标。
- [ ] **AC-4**：全量后端/Agent/前端测试通过，`.trellis/spec/` 索引与文档更新完毕。
