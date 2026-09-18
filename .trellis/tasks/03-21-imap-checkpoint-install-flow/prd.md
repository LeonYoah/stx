# brainstorm: 一键安装支持 IMAP 与 Checkpoint 配置

## Goal

让 STX 的一键安装 / 创建集群流程完整支持 SeaTunnel Engine 的 IMAP 与 checkpoint 配置、校验与安装后可观测性；同时在已安装或注册进来的集群详情页中支持查看 IMAP / checkpoint 目录、大小、清理状态与相关操作，降低错误配置带来的恢复失败或启动过慢风险。

## What I already know

* 用户希望 STX 一键安装支持 IMAP 配置和 checkpoint 配置。
* checkpoint 在当前产品里已经有部分实现，但未充分测试，交互也存在问题。
* IMAP 当前在 STX 安装流程里基本未实现，至少未发现前端/后端安装表单与参数透传。
* 用户要求对远程地址做联通性校验。
* 用户明确给出了产品侧认知：
  * IMAP 类似 Flink 的 RocksDB，用于保存集群恢复所需的 Hazelcast/IMap 元数据，而不是表数据。
  * 未配置外部 IMAP 时，元数据在内存里，集群重启后丢失。
  * 配置外部 IMAP 后，集群重启 / master 切换时会依赖 IMAP 数据恢复任务，但外部 IMAP 数据量过大也会拖慢恢复，甚至导致长时间不可用。
  * 只有批处理任务时，不建议开启外部 IMAP；实时任务场景更适合配外部 IMAP。
  * checkpoint 与 IMAP 是分开配置的。
* 用户希望对安装好的集群或注册进来的集群，在详情页支持查看 checkpoint / IMAP 路径、占用大小、IMAP 定期清理与手动清理。

## Repo findings

* 当前安装/创建集群流程已经支持 checkpoint：
  * 前端：`frontend/components/common/installer/ConfigStep.tsx`、`frontend/components/common/cluster/ClusterDeployWizard.tsx`
  * Hook：`frontend/hooks/use-installer.ts`
  * 后端：`internal/apps/installer/service.go`
  * Agent：`agent/cmd/main.go`、`agent/internal/installer/manager.go`
* 当前 checkpoint 支持的安装类型是：`LOCAL_FILE / HDFS / OSS / S3`。
* Agent 端已有 checkpoint 配置写入 `seatunnel.yaml` 的实现，并支持 HDFS HA、Kerberos、OSS/S3 参数。
* 当前默认 checkpoint 配置是 `LOCAL_FILE + /tmp/seatunnel/checkpoint/`。
* 当前代码中未发现安装流程对 IMAP 的表单、参数透传或 agent 配置落盘支持。
* 当前代码中未发现对 checkpoint / IMAP 远端存储地址的联通性测试接口或安装前校验步骤。
* 当前代码中未发现集群详情页对 checkpoint / IMAP 路径占用、定期清理、手动清理的支持。

## Official SeaTunnel references

* IMAP 配置在 `hazelcast*.yaml` 的 `map.engine*.map-store` 下。
* IMAP 官方样例支持：
  * HDFS
  * 本地文件（通过 `storage.type: hdfs` + `fs.defaultFS: file:///`）
  * OSS
  * S3/MinIO
* checkpoint 配置分两层：
  * 作业 `env { checkpoint.interval / checkpoint.timeout }`
  * 引擎 `seatunnel.yaml -> seatunnel.engine.checkpoint.storage`
* checkpoint 官方存储支持：
  * `type: hdfs` + `storage.type: hdfs / oss / cos / s3 / local(file:///)`
  * `type: localfile`（deprecated）

## Assumptions (temporary)

* STX 当前安装向导对 IMAP 是缺失能力，不是已有隐藏实现。
* IMAP / checkpoint 联通性校验应当由控制面或 agent 主动发起，而不是依赖用户手工验证。
* 集群详情页的 checkpoint / IMAP 目录查看与清理，需要通过 agent 执行远程文件系统检查或命令。
* 注册进来的集群可能存在配置偏差，需要兼容“只读识别”与“后续治理”。

## Open Questions

* 用户已确认本期范围选 A：安装/创建集群流程 + 详情页查看路径/大小/手动清理；IMAP 定时清理先搁置，依赖 SeaTunnel 自身自动清理机制。
* 用户已确认 IMAP 安装交互采用完整模式 A：暴露 `DISABLED / LOCAL_FILE / HDFS / OSS / S3`。

## Requirements (evolving)

* 安装流程支持配置 IMAP。
* 安装流程支持 checkpoint，并修复现有交互问题。
* 单节点场景默认/推荐 checkpoint 使用 local；分布式集群应警告用户不要用 local。
* 对 IMAP 给出清晰产品提示，说明适用场景、风险与恢复影响。
* 对远程地址/存储做联通性校验。
* 安装后的集群详情页支持查看 IMAP / checkpoint 路径、大小、清理状态与操作。
* 已安装集群与注册集群都要联动支持。
* 对已注册集群，详情页应优先从节点上的真实 `seatunnel.yaml` / `hazelcast*.yaml` 反向解析 checkpoint 与 IMAP 配置；仅在无法读取 live config 时，才回退到控制面保存的 `cluster_config`。
* 本期存储校验仅覆盖“本地路径可用性 / 远端端点 TCP 可达性”，不做与 SeaTunnel 运行时完全一致的深度鉴权与真实写入探针。

## Acceptance Criteria (evolving)

* [ ] 一键安装/创建集群流程可配置 checkpoint，且校验逻辑与 SeaTunnel 官方语义一致。
* [ ] 一键安装/创建集群流程可配置 IMAP，并能生成正确的 `hazelcast*.yaml` map-store 配置。
* [ ] 当集群为多节点但 checkpoint 使用 local 时，UI 给出显著警告。
* [ ] 当用户配置远程存储（如 HDFS/OSS/S3）时，支持联通性测试或可执行校验。
* [ ] 集群详情页可查看 IMAP / checkpoint 路径与占用情况。
* [ ] 集群详情页支持 IMAP 手动清理；定期清理本期不实现。
* [ ] 对注册进来的集群，详情页优先展示从 live config 反向解析出的 checkpoint / IMAP 配置，并明确标注来源为 `live` 或 `cluster_config`。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Out of Scope (explicit)

* 不在本任务中实现 SeaTunnel 源表/目标表级别的数据恢复机制。
* 不在本任务中修改 SeaTunnel 官方恢复语义。
* 不在本任务中重构所有安装向导页面，只做与 IMAP/checkpoint 强相关的交互收口。
* 本期不实现 IMAP 定期清理任务，仅支持查看与手动清理。
* 本期不实现基于目标版本 SeaTunnel Java/Hadoop classpath 的深度鉴权探针，仅保留快速联通性校验。

## Technical Notes

* 相关代码：
  * `frontend/components/common/installer/ConfigStep.tsx`
  * `frontend/components/common/cluster/ClusterDeployWizard.tsx`
  * `frontend/hooks/use-installer.ts`
  * `frontend/lib/services/installer/types.ts`
  * `internal/apps/installer/service.go`
  * `internal/apps/installer/types.go`
  * `agent/cmd/main.go`
  * `agent/internal/installer/manager.go`
* Apache SeaTunnel 上游仓库参考路径（实施时按目标版本核对）：
  * `docs/en/engines/zeta/separated-cluster-deployment.md`
  * `docs/en/engines/zeta/hybrid-cluster-deployment.md`
  * `docs/en/engines/zeta/checkpoint-storage.md`
  * `docs/en/introduction/configuration/JobEnvConfig.md`
* 这是一个明显的 cross-layer/infrastructure task：前端表单、installer API、agent 配置落盘、远端探测、集群详情联动都需要统一设计。
