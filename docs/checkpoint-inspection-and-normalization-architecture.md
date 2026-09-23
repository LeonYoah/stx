# Checkpoint 解码与业务位点归一化系统架构设计

## 1. 架构目标与设计原则

在分布式数据同步与 CDC（Change Data Capture）链路中，Checkpoint / Savepoint 是保障端到端 Exactly-Once 语义与状态恢复的核心基石。
然而，SeaTunnel 引擎底层 Checkpoint 存储格式具有以下特征：
- 强二进制序列化（ProtoStuff / Java Serialization）；
- 算子状态深层嵌套（`PipelineState` -> `CompletedCheckpoint` -> `ActionState` -> `SubtaskState` -> 状态 byte[] 切片）；
- 序列化数据反序列化强依赖对应 Connector 的类定义（如 MySQL CDC 的 `BinlogOffset`、`SnapshotSplitState` 等）。

本架构致力于解决**二进制状态深层穿透解码**、**连接器隔离加载**以及**异构连接器业务位点（Lag / Offset / Partition / 2PC）的多维归一化**，为上层提供纯粹面向业务排障与进度监控的可视化模型。

```
+------------------------------------------------------------------------------------+
|                         STX Web Console (Vue/React Frontend)                      |
|  [Tab 1: 概览与业务位点 (Overview & Progress)]   [Tab 2: 算子与分片拓扑 (Action Topology)]|
+------------------------------------------------------------------------------------+
                                      ▲ REST JSON
                                      │
+------------------------------------------------------------------------------------+
|                           STX Server (Go Cluster App)                              |
|   - Checkpoint 元数据聚合与 API 路由                                                |
|   - 跨层数据透传 (Sinks, Sources, Config, Normalization)                            |
+------------------------------------------------------------------------------------+
                                      ▲ gRPC / Agent Probe
                                      │
+------------------------------------------------------------------------------------+
|                         STX Agent (Host Executor Daemon)                           |
|   - 本地/远端集群路径解析与 Connector Jar 收集扫描                                 |
|   - 守护进程或 Managed CLI 驱动 stx-java-proxy                                     |
+------------------------------------------------------------------------------------+
                                      ▲ HTTP Loopback
                                      │
+------------------------------------------------------------------------------------+
|                        stx-java-proxy (Engine Intermediate)                        |
|                                                                                    |
|  +------------------------------------------------------------------------------+  |
|  |           Isolated PluginClassLoader (递归扫描 connectors/plugins/lib)       |  |
|  +------------------------------------------------------------------------------+  |
|                                     │                                              |
|  +------------------------------------------------------------------------------+  |
|  |                    CompletedCheckpoint 解码与状态解构管道                    |  |
|  |  1. PipelineState.class (ProtoStuff)                                         |  |
|  |  2. CompletedCheckpoint.class (Target ClassLoader 反射实例化)                |  |
|  |  3. ActionState / SubtaskState 二进制切片解包                                |  |
|  +------------------------------------------------------------------------------+  |
|                                     │                                              |
|  +------------------------------------------------------------------------------+  |
|  |                      四大位点范式与 Sink 2PC 归一化引擎                      |  |
|  |  - BINLOG_OFFSET (MySQL CDC, TiDB CDC, Postgres CDC)                         |  |
|  |  - PARTITION_OFFSET (Kafka, Pulsar, RocketMQ)                                |  |
|  |  - TIME_CURSOR (增量时间戳 / Watermark / 轮询游标)                           |  |
|  |  - FILE_CHUNK_PROGRESS (分块文件, 对象存储分片)                              |  |
|  |  - SINK_2PC_TRANSACTION (Jdbc, ClickHouse, Iceberg 预提交事务状态)           |  |
|  +------------------------------------------------------------------------------+  |
+------------------------------------------------------------------------------------+
```

---

## 2. 分层架构与核心数据流

### 2.1 存储探测与数据流转 (Probe & Serialization Flow)

1. **Agent 定位与 Jar 提取**：
   - STX Agent 探测节点通过被测集群的 `${SEATUNNEL_HOME}` 自动扫描收集当前环境下的所有 Connector Jars（支持穿透 `connectors/`、`plugins/` 以及 3.0 的统一插件树子目录）。
   - Agent 构建包含快照存储协议配置、Checkpoint 文件路径、作业 HOCON 配置和插件 Jar 列表的探测载荷。

2. **Isolated ClassLoader 双重隔离与回退 (ClassLoader Fallback)**：
   - 优先通过 `PluginClassLoaderUtils.createClassLoader(pluginJars, parent)` 加载指定的外部 Connector Jars。
   - 若未显式传入 Jars，回退至 `createClassLoaderFromSeatunnelHome(parent)`，动态穿透宿主环境的插件与类库。
   - 切换当前工作线程的 ContextClassLoader（TCCL）至运行时隔离 ClassLoader，使 ProtoStuff 与 Java 原生反序列化管道均可安全获取特定 Connector 类。

3. **两阶段二进制解析 (Two-Phase Binary Extraction)**：
   - **Phase 1: 引擎管线元数据解构**：
     反序列化外层 `org.apache.seatunnel.engine.checkpoint.storage.PipelineState`，读取状态元数据（Checkpoint ID, Job ID, Pipeline ID, Checkpoint Type, State Size）。
   - **Phase 2: 运行时 CompletedCheckpoint 动态解构**：
     通过 `Class.forName("org.apache.seatunnel.engine.server.checkpoint.CompletedCheckpoint", true, runtimeClassLoader)` 加载并反序列化出各算子 `ActionState` 与 `SubtaskState` 状态集。

---

## 3. 四大业务位点范式归一化机制

为了彻底消除不同连接器私有状态结构的异构性，系统引入状态归一化模型（`NormalizedProgress`），将任意流批 Source 的底层状态抽象为 4 大标准物理范式：

| 归一化范式 (`paradigm`) | 典型代表连接器 | 核心属性与物理语义 | 计算与展示重点 |
| :--- | :--- | :--- | :--- |
| **`BINLOG_OFFSET`** | MySQL-CDC, Postgres-CDC, Oracle-CDC, TiDB | `binlogFile`, `binlogPosition`, `serverUuid`, `gtid`, `assignedSplit` | Binlog 文件与偏移量进度、Event Time、实时消费延迟 Lag |
| **`PARTITION_OFFSET`** | Kafka, Pulsar, RocketMQ | `topic`, `partition`, `committedOffset`, `latestOffset`, `lag` | 分区偏移量明细、总消费进度、各 Partition 积压状态 |
| **`TIME_CURSOR`** | 轮询数据库, 日志流, 时序数据 | `currentTimestamp`, `cursorColumn`, `watermark` | 游标推进时间、业务时间差延迟监控 |
| **`FILE_CHUNK_PROGRESS`** | LocalFile, S3, OSS, HDFS, Iceberg Batch | `filePath`, `currentByteOffset`, `totalBytes`, `chunkIndex` | 分片读取百分比、已处理分片数/总分片数 |

### 3.1 归一化状态模型结构

```json
{
  "paradigm": "BINLOG_OFFSET",
  "summary": "mysql-bin.000003 : 12584",
  "target": "stx_e2e.users_src",
  "lagMs": 1054,
  "eventTimestamp": 1726930245000,
  "subtaskCount": 1,
  "subtasks": [
    {
      "subtaskIndex": 0,
      "summary": "mysql-bin.000003:12584 (offset: 12584)",
      "target": "stx_e2e.users_src",
      "lagMs": 1054,
      "eventTimestamp": 1726930245000,
      "stateSizeBytes": 156
    }
  ],
  "rawDetails": {
    "file": "mysql-bin.000003",
    "pos": 12584,
    "splitId": "stx_e2e.users_src:0"
  }
}
```

### 3.2 Sink 端两阶段提交（2PC）事务状态感知

除 Source 消费进度外，Sink 端的状态直接决定数据是否已持久化或处于预提交状态：
- 识别 `Sink[...].state` 二进制切片；
- 提取并呈现 2PC 预提交事务（Prepared Transactions）的事务 ID 集合、写入表目标及等待协调器提交的事务数；
- 协助运维人员即时排查 2PC 悬挂事务或超时回滚隐患。

---

## 4. 前端 UI 极简双看板设计

原有的底层状态树（AST / 反序列化内部字段反射树）由于过于晦涩且存在反序列化异常噪音，已被彻底剔除。前台精简为双看板视图：

```
+------------------------------------------------------------------------------------+
| Checkpoint 详情 - #20 [SAVEPOINT_TYPE]               [复制路径] [复制完整快照 JSON] |
| 集群: prod-cluster | Pipeline: 1 | Job ID: 179000218632900000 | 大小: 7.6 KB       |
+------------------------------------------------------------------------------------+
|  [ 概览与业务位点 (Overview) ]  |  [ 算子与分片拓扑 (Action Topology) ]             |
+------------------------------------------------------------------------------------+
|                                                                                    |
|  [Source 业务位点卡片]                                                             |
|  +------------------------------------------------------------------------------+  |
|  | MySQL-CDC (Source[0]-MySQL-CDC)                     状态: 正常消费            |  |
|  | 范式: BINLOG_OFFSET | 目标表: stx_e2e.users_src                               |  |
|  | 当前位点: mysql-bin.000003 : 12584                   延迟 (Lag): 1.05 秒       |  |
|  | +--------------------------------------------------------------------------+ |  |
|  | | Subtask #0 | mysql-bin.000003:12584 | Lag: 1.05s | 状态体积: 156 B        | |  |
|  | +--------------------------------------------------------------------------+ |  |
|  +------------------------------------------------------------------------------+  |
|                                                                                    |
|  [Sink 2PC 事务状态卡片]                                                           |
|  +------------------------------------------------------------------------------+  |
|  | Jdbc (Sink[0]-Jdbc)                                 2PC 状态: 1 个已预提交    |  |
|  | 事务提交状态体积: 381 B | 目标表: stx_e2e.users_sink                              |  |
|  +------------------------------------------------------------------------------+  |
+------------------------------------------------------------------------------------+
```

1. **顶栏操作区**：保留「复制路径」并新增「复制完整快照 JSON」，满足高阶专家离线审查需求，同时保持主界面零视觉负担。
2. **看板 1: 概览与业务位点 (`overview`)**：
   - 核心指标条：Pipeline、触发耗时、快照体积、触发类型。
   - Source 业务位点卡片：以业务视角高亮展示当前消费文件、Offset、目标表及实时 Lag；展开可查看并行 Subtask 明细。
   - Sink 2PC 状态卡片：呈现待提交事务数与写入进度。
3. **看板 2: 算子与分片拓扑 (`actions`)**：
   - 拓扑并行度总览、Subtask 资源消耗统计、Coordinator 分片分配列表。

---

## 5. 总结

本架构实现了从底层**强依赖类库的二进制快照**到**业务可用位点指标**的平滑解耦与跨层投递，具备隔离容错度高、指标直观、算子拓扑清晰的特点，彻底解决由于插件类加载缺失导致的 `DECODE_FAILED` 问题。
