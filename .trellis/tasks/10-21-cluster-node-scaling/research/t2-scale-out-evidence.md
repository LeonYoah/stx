# 研究：2026-10-21 t2 扩容实录与缺口证据

> 现场：t1 `10.66.0.2`（控制面 + SeaTunnel 3.0.0 hybrid 节点）、t2 `10.66.0.1`（新主机），WireGuard 组网；集群 id=1 `seatunnel-3-single`。

## 实际完成扩容所需的手工步骤（即本任务要消除的人工环节）

1. 控制面 `app.external_url` 172.16.0.2 → 10.66.0.2；两台 agent 显式 `agent.ip`（两机 eth0 同为 172.16.0.2）。
2. `stx host update 1 --ip-address 10.66.0.2`；`stx host create` 登记 t2；在 t2 执行 agent 安装命令（`--ip=10.66.0.1`）。
3. t2 手工安装 JDK 11（STX 不负责，见 PRD Out of Scope）。
4. 集群模板 `hazelcast.yaml` member-list 127.0.0.1 → `10.66.0.2:5801, 10.66.0.1:5801`，`sync-all` 后重启 t1（t1 原通告 `127.0.0.1:5801`，t2 无法入群）。
5. CLI 调安装器：`host install start 2 --request-file`（`cluster_id=1`, `master_addresses=[10.66.0.2,10.66.0.1]`）→ 安装 + 登记 + 启动成功。
6. 安装器 `InitClusterConfigs` 覆盖模板：`hazelcast.yaml`→v3、`jvm_options`→v3、`log4j2.properties`→v3（routingAppender→fileAppender，per_job 日志丢失）、`hazelcast-client.yaml`→v2。手工恢复 log4j2 模板（v4）并 `sync-all`，重启 t2。
7. 集群已装 jdbc 插件（profile `postgresql`：`postgresql-42.4.3.jar`、`postgis-jdbc-2.5.1.jar`）未下发到 t2；`cluster plugin install` 报「插件已安装」；手工从 t1 拷贝 `connectors/connector-jdbc-3.0.0.jar` 与 `plugins/connector-jdbc/`。
8. `stx cluster plugin list 1` 返回的记录无 `selected_profile_keys`（profile 不入库，依赖本地缓存元数据）。

## 关键代码证据

| 结论 | 位置 |
|---|---|
| 添加节点无安装分支，预检=纳管已运行节点 | `frontend/components/common/cluster/AddNodeDialog.tsx`；`internal/apps/cluster/service.go:1991` |
| 提交不校验预检结果 | `AddNodeDialog.tsx:461` |
| AddNodes 只写库 | `internal/apps/cluster/service.go:1229` |
| 拓扑钩子无人注册 | `internal/apps/cluster/service.go:235` |
| RemoveNode 只删库 | `internal/apps/cluster/service.go:1291` |
| 模板被新节点覆盖 | `internal/apps/config/service.go:393–446` |
| 安装器只写新节点 member-list | `agent/internal/installer/manager.go:2781` |
| 监控下发在主机无节点时跳过 | `internal/grpc/handlers.go:1137` |
| agent 按 cluster_id 替换监控项 | `agent/cmd/main.go:2339` |
| java-proxy 承载节点动态选首个在线 master | `internal/apps/cluster/stx_java_proxy.go:241` |
| 运行中作业（集群维度） | `internal/apps/sync/engine_client.go:240` |
| 可复用预检子命令 check_tcp / check_cluster_members / check_java | `agent/internal/executor/precheck_handlers.go`、`cluster_members_check.go` |
| Prometheus HTTP SD 实时计算目标 | `internal/apps/monitoring/service.go:700` |
| SeaTunnel 发行包默认 member-list 为 localhost | `apache-seatunnel-3.0.0/config/hazelcast.yaml` |
| 前端 R2 两步确认先例 | `frontend/lib/services/core/execution-headers.ts`、`st-upgrade.service.ts` |
| YAML 字段修补 + 同步先例 | `internal/apps/cluster/runtime_storage_apply.go` |
