# checkpoint source state 官方深度 inspect

## Goal

在 `stx-java-proxy` 中新增一个专用的 checkpoint source state 深度 inspect 能力，能够在现有 `PipelineState / CompletedCheckpoint / ActionState / TaskStatistics` 基础上，继续使用 SeaTunnel 官方 serializer / factory / state class 解码 source state，输出结构化的 offset / split / pending state 摘要，供平台展示 MySQL CDC binlog 位点、Kafka topic-partition offset、CDC enumerator pending split 等信息。

## What I already know

- 当前 proxy 已有基础 checkpoint inspect，位置：
  - `tools/stx-java-proxy/src/main/java/io/github/leonyoah/stx/proxy/service/storage/CheckpointDeserializeService.java`
- 现有能力只解析到：
  - `PipelineState`
  - `CompletedCheckpoint`
  - `actionStates`
  - `taskStatistics`
- 现有能力还没有继续解码 `ActionSubtaskState.state` 内部的 source payload。
- SeaTunnel checkpoint 结构本质上是两层：
  - 外层 engine checkpoint 对象
  - 内层 connector 自己的 state payload byte[]
- SeaTunnel Source API 自带官方 serializer 入口：
  - `getSplitSerializer()`
  - `getEnumeratorStateSerializer()`
- 当前仓库里大量 source 没有自定义 serializer，很多沿用 `DefaultSerializer`。
- 第一阶段希望严格跟随官方实现，不引入：
  - `ObjectInputStream + 反射 fallback`
- 第一阶段不做 sink / transform 深度 inspect。
- 第一阶段不改现有 `/api/v1/storage/checkpoint/inspect` 响应结构，避免破坏已有平台调用。

## Requirements

- proxy 新增专用接口：
  - `POST /api/v1/storage/checkpoint/inspect-source-state`
- 新接口必须复用现有 checkpoint 文件读取能力：
  - 支持现有 `config + path + pluginJars` 模式
- 新接口必须支持传入 `jobConfig`，用于将 source config index 与 checkpoint action name 精确匹配。
- 新接口必须支持只解析指定 source：
  - `sourceTargets`
- 新接口必须支持限制每个 subtask 返回的 split 数量：
  - `splitLimitPerSubtask`
- 新接口必须保持老 `checkpoint/inspect` 完全不变。
- source state 深度解码只允许使用：
  - 官方 serializer
  - 官方 factory helper
  - 官方 state class
- 第一阶段不允许引入：
  - `ObjectInputStream + 反射 fallback`
- 第一阶段不允许通过 `createSource()` 盲目实例化所有 source，以避免外部系统副作用。
- source 匹配逻辑必须复用官方 action name 规则：
  - `Source[i]-pluginName`
- job config 解析必须优先复用现有 `JobConfigSupportService`。
- 输出结果必须结构化，至少包含：
  - source configIndex
  - pluginName
  - actionName
  - decodeStrategy
  - enumeratorStateClass
  - subtaskStateClass
  - coordinator 摘要
  - subtask split 摘要
- 单个 source 解码失败不能拖垮整个接口；失败 source 进入结构化错误结果或 warnings。
- 未注册 descriptor 的 source 不能返回 500，应进入 `unsupportedSources`。

## Non-Goals

- 不修改老 `/api/v1/storage/checkpoint/inspect` 的响应结构。
- 不做 sink 深度 inspect。
- 不做 transform 深度 inspect。
- 不做未知 connector 的猜测式解码。
- 不做无 `jobConfig` 场景下的 source 精确匹配。
- 不在第一阶段通过 source 实例化去拿 serializer。
- 不在第一阶段加入 `ObjectInputStream + 反射 fallback`。

## Core Design

### 1. 新接口

新增：
- `POST /api/v1/storage/checkpoint/inspect-source-state`

建议注册到：
- `StxJavaProxyServer.java`

### 2. 请求结构

示例：

```json
{
  "config": {
    "storage.type": "s3",
    "namespace": "/seatunnel/checkpoint",
    "s3.bucket": "s3a://seatunnel-checkpoint",
    "fs.s3a.endpoint": "http://minio:9000",
    "fs.s3a.path.style.access": "true"
  },
  "path": "seatunnel/checkpoint/177475643448100000/1774757740578-433-1-131.ser",
  "pluginJars": [],
  "jobConfig": {
    "content": "env { ... } source { ... } sink { ... }",
    "contentFormat": "hocon",
    "variables": {}
  },
  "sourceTargets": [0, 1],
  "splitLimitPerSubtask": 20,
  "includeCoordinator": true,
  "includeSubtaskSplits": true
}
```

### 3. 响应结构

示例：

```json
{
  "ok": true,
  "pipelineState": {},
  "completedCheckpoint": {},
  "sources": [
    {
      "configIndex": 0,
      "pluginName": "MySQL-CDC",
      "actionName": "Source[0]-MySQL-CDC",
      "decodeStrategy": "CHANGE_STREAM_FACTORY",
      "enumeratorStateClass": "org.apache.seatunnel.connectors.cdc.base.source.enumerator.state.HybridPendingSplitsState",
      "subtaskStateClass": "org.apache.seatunnel.connectors.cdc.base.source.split.IncrementalSplit",
      "coordinator": {},
      "subtasks": [
        {
          "subtaskIndex": 0,
          "splitCount": 1,
          "splits": []
        }
      ]
    }
  ],
  "unsupportedSources": [],
  "warnings": []
}
```

### 4. 三种 decode strategy

#### CHANGE_STREAM_FACTORY
适用：
- MySQL-CDC
- Oracle-CDC

做法：
- 抽取 source 的 coordinator state + subtask split state
- 组装 `ChangeStreamTableSourceCheckpoint`
- 调 `ChangeStreamTableSourceFactory.deserializeTableSourceState(...)`

#### DEFAULT_SERIALIZER_TYPED
适用：
- 大部分 CDC source
- Kafka / Pulsar / RocketMQ / RabbitMQ / SLS / TableStore
- TiDB-CDC
- Paimon / Iceberg
- FakeSource

做法：
- registry 给出已知 split class / enumerator state class
- 用 `DefaultSerializer.deserialize(...)` 按 typed class 解码

#### SINGLE_SPLIT_EMPTY
适用：
- Http
- Prometheus
- GraphQL
- Socket
- Web3j
- OpenMldb

做法：
- split 解为 `SingleSplit`
- enumerator state 解为 `SingleSplitEnumeratorState`
- 若 payload 为空，直接输出“无可恢复 payload”

### 5. Registry

建议新增：
- `StreamingSourceDescriptorRegistry`

核心字段：
- `factoryIdentifier`
- `pluginName`
- `decodeStrategy`
- `splitClassName`
- `enumeratorStateClassName`
- `projectorId`

第一阶段注册：
- `MySQL-CDC` -> `CHANGE_STREAM_FACTORY`
- `Oracle-CDC` -> `CHANGE_STREAM_FACTORY`
- `Postgres-CDC` -> `DEFAULT_SERIALIZER_TYPED`
- `SQLServer-CDC` -> `DEFAULT_SERIALIZER_TYPED`
- `OpenGauss-CDC` -> `DEFAULT_SERIALIZER_TYPED`
- `MongoDB-CDC` -> `DEFAULT_SERIALIZER_TYPED`
- `TiDB-CDC` -> `DEFAULT_SERIALIZER_TYPED`
- `Kafka` -> `DEFAULT_SERIALIZER_TYPED`
- `Pulsar` -> `DEFAULT_SERIALIZER_TYPED`
- `Rocketmq` -> `DEFAULT_SERIALIZER_TYPED`
- `RabbitMQ` -> `DEFAULT_SERIALIZER_TYPED`
- `Sls` -> `DEFAULT_SERIALIZER_TYPED`
- `TableStore` -> `DEFAULT_SERIALIZER_TYPED`
- `Paimon` -> `DEFAULT_SERIALIZER_TYPED`
- `Iceberg` -> `DEFAULT_SERIALIZER_TYPED`
- `FakeSource` -> `DEFAULT_SERIALIZER_TYPED`
- `Http / Prometheus / GraphQL / Socket / Web3j / OpenMldb` -> `SINGLE_SPLIT_EMPTY`

### 6. Projector

建议解码和展示投影分层：

- 解码层：把 byte[] 还原为 typed object
- projector 层：把 typed object 投影成前端可读结构

第一批 projector：
- `cdc-projector`
- `tidb-projector`
- `kafka-projector`
- `pulsar-projector`
- `rocketmq-projector`
- `rabbitmq-projector`
- `sls-projector`
- `tablestore-projector`
- `paimon-projector`
- `iceberg-projector`
- `fake-projector`
- `single-split-projector`

projector 原则：
- 保留类名
- 保留数量信息
- 只提取稳定关键字段
- 不返回大对象全文
- 不返回无限递归结构

## Source Family Scope

### A. CDC 家族
覆盖：
- MySQL-CDC
- Oracle-CDC
- Postgres-CDC
- SQLServer-CDC
- OpenGauss-CDC
- MongoDB-CDC

### B. TiDB-CDC
覆盖：
- `TiDBSourceSplit`
- `TiDBSourceCheckpointState`

### C. MQ / Log 家族
覆盖：
- Kafka
- Pulsar
- RocketMQ
- RabbitMQ
- SLS
- TableStore

### D. Lakehouse Streaming 家族
覆盖：
- Paimon
- Iceberg

### E. SingleSplit 家族
覆盖：
- Http
- Prometheus
- GraphQL
- Socket
- Web3j
- OpenMldb

### F. 测试流式 source
覆盖：
- FakeSource

## Matching Rules

- 必须传 `jobConfig`
- action name 必须按官方规则生成：
  - `Source[i]-pluginName`
- source 匹配建议新增：
  - `CheckpointSourceActionMatcher`
- config 解析优先复用：
  - `JobConfigSupportService`

## Service Split

建议新增：
- `CheckpointSourceStateInspectService`
- `CheckpointSourceActionMatcher`
- `StreamingSourceDescriptorRegistry`
- `OfficialStateDecoder`
- `ChangeStreamFactoryDecoder`
- `DefaultSerializerTypedDecoder`
- `SingleSplitEmptyDecoder`
- `SourceStateProjectorRegistry`

建议复用：
- `CheckpointDeserializeService`
- `JobConfigSupportService`
- `PluginClassLoaderUtils`
- `ProbeExecutionUtils`

## Performance Requirements

- 接口必须支持按 source 精确解析：
  - `sourceTargets`
- 必须支持限制每个 subtask 返回的 split 数量：
  - `splitLimitPerSubtask`
- 必须支持可选关闭 coordinator / subtask split 详情，控制响应大小：
  - `includeCoordinator`
  - `includeSubtaskSplits`
- 不允许一次默认展开超大 split payload 全文。
- 单个 source 解码失败时，不能中断整个接口。
- 未匹配 descriptor 的 source 不报 500，放进 `unsupportedSources`。
- 必须沿用现有 `pluginJars` 动态 classloader 机制，避免要求 proxy 重启。

## Acceptance Criteria

- [ ] 新增 `POST /api/v1/storage/checkpoint/inspect-source-state`
- [ ] 老 `POST /api/v1/storage/checkpoint/inspect` 保持完全不变
- [ ] 可以读取现有 checkpoint 文件并继续解析 source state
- [ ] 能根据 `jobConfig` 将 checkpoint action 与 source config index 精确匹配
- [ ] `MySQL-CDC` 可输出结构化 binlog offset 摘要
- [ ] `Oracle-CDC` 可输出结构化 SCN 摘要
- [ ] `Kafka` 可输出 topic / partition / offset 摘要
- [ ] `CDC enumerator` 可输出 pending split / processed table 摘要
- [ ] `SingleSplit` 家族能输出无 payload / payload size 等轻量摘要
- [ ] 未覆盖 source 进入 `unsupportedSources`，不影响其他 source 成功返回
- [ ] 单个 source 解码失败只影响该 source，不影响整包响应
- [ ] 至少补齐一轮 proxy 单测 / 定向集成验证

## Risks

- registry 直接依赖当前 SeaTunnel 源码中的 state class 名称，后续 connector 升级后需要同步维护。
- 若未来某些 source 改为自定义 serializer，`DefaultSerializerTypedDecoder` 可能不再适用。
- 第一阶段不实例化 source，意味着部分 future connector 若依赖运行时 serializer 获取逻辑，可能需要二期补充。
- 第一阶段不做 `ObjectInputStream + 反射 fallback`，未知 connector 将明确返回 unsupported，而不会“猜测式成功”。

## Open Questions

- `jobConfig` 是否允许后续直接传 proxy 侧已解析 graph，而不是原始 content + format？
- `Paimon / Iceberg` 的 projector 第一版是否只做数量摘要，后续再补 table offset 细节？
- 平台侧是否需要新增一个 source-state inspect 专用展示面板，而不是复用现有 checkpoint inspect 对话框？

## Suggested Validation

- MySQL-CDC：验证 `file / pos / server_id / ts_sec`
- Oracle-CDC：验证 `scn / commit_scn / lcr_position`
- Kafka：验证 `topic / partition / startOffset / currentOffset / endOffset`
- SingleSplit 家族：验证无 payload 场景
- 未注册 source：验证进入 `unsupportedSources`
- 错误 source：验证单 source 失败不拖垮整体响应

## Recent Updates

- proxy 已新增 `inspect-source-state` 专用接口，并保持老 `checkpoint/inspect` 不变。
- projector 首轮已覆盖 CDC / Kafka / TiDB / Pulsar / RocketMQ / RabbitMQ / SLS / TableStore / Paimon / Iceberg / FakeSource / SingleSplit。
- 平台 cluster checkpoint inspect 已支持可选 `job_config`，内部自动补调 source state inspect，并以 `source_state_inspect` 形式合并回老详情响应。
- Sync 工作台 CK 详情页已改为“Completed Checkpoint + Pipeline State + Actions”结构，其中 Action 状态、Task 统计、Source State 融合展示，且 action 默认展开。
- 已完成端到端回归：通过平台接口校验 MySQL-CDC checkpoint 可拿到 `mysql-bin.000003 / pos=2840 / server_id=223344`。
