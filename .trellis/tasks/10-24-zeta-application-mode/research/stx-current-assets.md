# STX 现状：作业链路与可复用资产

## 现有作业链路（全部依赖常驻集群和 REST）

- `internal/apps/sync/engine_client.go`：`EngineClient` 通过 REST V2（并兼容旧的 `/hazelcast/rest/maps/*`）完成以下操作：submit、job-info、running/finished 列表、checkpoint 概览/历史、stop、logs。
- 作业实例（`sync_job_instances`）挂在 `cluster_id` 下，用 `engine_job_id` 关联引擎侧作业。
- 对账：`.trellis/spec/backend/job-status-reconciler.md`。按 `cluster_id` 分组调用 `ListRunningJobs`/`ListFinishedJobs`，必要时再调 `/job-info`。规则包括“集群可达但作业已消失 → failed”“集群不可用 → degraded/stale，不擅自判失败”。
- 日志：由 Agent 读取主机上的日志文件（`GET .../logs`），不经过 SeaTunnel REST。
- CLI：`stx sync job ... --wait`，依赖读库路径和 `--observation-timeout`；Dolphin shell 走同一条路径。

## 可复用资产

| 资产 | 位置 | 对本任务的价值 |
| --- | --- | --- |
| stx-java-proxy | `tools/stx-java-proxy`、`internal/apps/cluster/stx_java_proxy.go` | 已有的 Java sidecar，复用 SeaTunnel 运行时 jar；有 2.3 / 3.0 两套引擎适配器；能用 `ConfigParserUtil` 解析 DAG、做 Catalog 探测，用 `CheckpointStorageFactory` / `IMapStorageFactory` 探测存储；打包了 hadoop3 uber。→ **最合适的 gateway 承载位置**（相当于 Dinky 的 dinky-gateway） |
| 插件管理 | `internal/apps/plugin` | 已按 SeaTunnel 版本维护 connector 及其 Maven/上传依赖（`target_dir`：`connectors/`、`lib/`、`plugins/<mapping>`），在控制面存储并带校验和 → **精简制品的依赖来源** |
| K8s 凭据 | `internal/apps/host`（`HostTypeKubernetes`：API URL、namespace、kubeconfig/token） | 可以作为 K8s 执行目标的连接信息 |
| runtime storage | `internal/apps/cluster/runtime_storage*.go` | HDFS（HA/Kerberos/hdfs-site）、S3 等 checkpoint 存储配置 → application 的 checkpoint 和恢复可以复用 |
| 版本能力矩阵 | `internal/seatunnel/capabilities.go` `CapabilitiesForVersion` | 可以加一项 `SupportsApplicationMode`（>= 3.0.1 / 3.1.0，待上游发版后确认） |
| Agent | `internal/proto/agent`（CommandStream、DownloadPackage） | 兜底方案：在装有 Hadoop 客户端配置的“网关主机”上执行 CLI |

## 需要改变的假设

1. 作业不一定属于某个 `cluster_id`，可能属于某个执行目标下的一个 application。
2. 观察手段不一定是 REST，在 application 下由平台 API 加 proxy 完成。
3. `UNKNOWN` 不能套用“作业从引擎消失 → failed”的规则：STX 自己发起的 cancel 应记为 canceled，并且必须在保留期到期前把终态写入数据库。
4. 日志不在 Agent 管理的主机上，要从 YARN 的 NM/聚合日志或 K8s 的 pods/log 获取。
