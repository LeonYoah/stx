# SeaTunnel 3.0 运行时适配与多版本兼容架构设计

> 本文针对 STX 中间件层 `stx-java-proxy` 与边缘探针 `stx-agent`，系统阐述面向 Apache SeaTunnel 3.0 大版本的适配方案、Provider / Adapter 多版本分发机制，以及后续小版本在 API 无破坏性改动时的“一次适配，配置零代码复用”架构。

---

## 1. 架构演进与设计目标

随着 Apache SeaTunnel 演进至 3.0 大版本，运行时在 Checkpoint 存储格式、类目组织结构、CDC 连接器 SPI 等方面进行了重构：
1. **Checkpoint 存储轻量化**：支持增量 Checkpoint（`incrementalStateFiles`），拆分元数据与小文件状态；
2. **连接器类加载结构扁平与目录树分离**：连接器不仅位于根 `connectors/`，还分布于 `connectors/seatunnel/`、`starter/`、`seatunnel-dist/` 等新型目录；
3. **SPI 协议演进**：CDC 连接器逐步扩展为 `ChangeStreamTableSourceFactory` 并与 `TableSourceFactory` 双轨并行；
4. **跨版本长期演进诉求**：要求建立统一的适配层，使得后续 3.1、3.2 等版本若无破坏性 API 变动，**无需修改 Proxy 核心代码，直接通过规则或配置映射实现版本复用**。

```mermaid
flowchart TD
    subgraph STX Go Agent
        AgentCmd[CLI / Agent Storage Probe]
        ScanJars[递归深度扫描 connectors/starter/plugins]
        AgentCmd --> ScanJars
    end

    subgraph stx-java-proxy 运行时分发中心
        Registry[SeaTunnelEngineAdapterRegistry]
        Policy[EngineAdapterVersionPolicy 前向策略引擎]
        V23Adapter[SeaTunnel23EngineAdapter (2.3.x)]
        V30Adapter[SeaTunnel30EngineAdapter (3.0.x / 3.x)]
        FutureAdapter[自定义扩展 / 外部配置映射]
        
        Registry --> Policy
        Policy -->|匹配 2.3.* / 2.*| V23Adapter
        Policy -->|匹配 3.0.* / 3.*| V30Adapter
        Policy -->|stx.adapter.version.mapping| FutureAdapter
    end

    ScanJars -->|传递 version, jars, payload| Registry
    V23Adapter --> DynamicLoader[Dual ClassLoader 租约隔离加载]
    V30Adapter --> DynamicLoader
    DynamicLoader --> Result[归一化 CompletedCheckpointData]
```

---

## 2. Provider / Adapter 多版本分发架构

为了彻底解耦 proxy 核心逻辑与底层特定引擎版本，引入统一的 SPI 抽象层：

### 2.1 契约规范：`SeaTunnelEngineAdapter`

位于 `io.github.leonyoah.stx.proxy.adapter`：
- **`deserializeCheckpoint(byte[] rawBytes, ClassLoader classLoader)`**：负责将二进制 Checkpoint/Savepoint 文件反序列化为统一模型 `CompletedCheckpointData`；
- **`inspectSources(...)` / `inspectSinks(...)`**：提取该引擎版本下 Source 位点与 Sink 提交状态；
- **`inspectIMapWal(...)`**：解析 Zeta 集群底层 IMap WAL 物理预写日志；
- **`getEngineCapabilities()`**：声明当前适配器特性集（增量 Checkpoint、表级状态、ChangeStream SPI、Savepoint 挂起恢复）。

### 2.2 统一数据载荷：`CompletedCheckpointData`

屏蔽 2.3 与 3.0 底层对象差异，承载：
- `completedCheckpoint`: 原始反序列化根对象；
- `pipelineState`: PipelineState 实例；
- `actionStates`: 包含所有算子状态的 Map；
- `incremental`: 是否为 3.0 增量快照；
- `savepoint`: 是否为 Savepoint 挂起快照；
- `engineVersion`: 实际解析适配版本标识；
- `extraMetadata`: 包含 3.0 表级状态（Table-level state）统计等扩展元数据。

### 2.3 适配基类：`AbstractSeaTunnelEngineAdapter`

封装防御性编程范式：
- **ClassLoader 切换上下文闭包**：在独立 ClassLoader 执行期间自动切换 `Thread.currentThread().setContextClassLoader`，并在 `finally` 块中绝对还原；
- **反射静默容错**：在不同版本属性名可能微调（如 `incremental` vs `incrementalStateFiles`）时，通过 `readPropertyQuietly` 与 `invokeMethodQuietly` 规避 `NoSuchMethodException` 或 `LinkageError`。

---

## 3. “一次适配，零代码复用”配置与前向兼容策略

业务现场通常无法在 SeaTunnel 发布微版本（如 3.0.1、3.1.0）时立即重新打包 STX。为此实现了 **三层自适应前向路由策略 (`EngineAdapterVersionPolicy`)**：

```mermaid
flowchart TD
    Input[请求携带 version 字符串 / 运行时探测版本] --> Step1{1. 匹配自定义配置映射?}
    Step1 -->|命中 -Dstx.adapter.version.mapping| ReturnTarget[获取对应已注册 Adapter]
    Step1 -->|未配置| Step2{2. 命中内置精确/通配规则?}
    
    Step2 -->|2.3.* / 2.3.x| Use23[SeaTunnel23EngineAdapter]
    Step2 -->|3.0.* / 3.0.x| Use30[SeaTunnel30EngineAdapter]
    Step2 -->|未精确命中| Step3{3. 主版本前向兼容自动回退?}
    
    Step3 -->|3.x.x 如 3.1.0 / 3.2.0| Use30Forward[自动复用 3.0 适配器 (零修改)]
    Step3 -->|2.x.x 如 2.4.0| Use23Forward[自动复用 2.3 适配器]
    Step3 -->|未知或无法解析| GlobalFallback[全局兜底适配器 (默认 2.3)]
```

### 3.1 三层路由机制

1. **第一层：外部动态覆盖配置（Dynamic Override）**：
   - 支持 JVM 启动参数 `-Dstx.adapter.version.mapping="3.1.*:3.0;3.2.*:3.0"`；
   - 支持操作系统环境变量 `STX_ADAPTER_VERSION_MAPPING`；
   - 运维人员无需修改任何代码，配置一行映射即可指定目标 Adapter。
2. **第二层：内置规则集（Built-in Rules）**：
   - 规则 `3.0.*` $\rightarrow$ `3.0`
   - 规则 `2.3.*` $\rightarrow$ `2.3`
3. **第三层：主版本前向兼容自动复用（Forward-Compatible Auto-Reuse）**：
   - 当检测到任意 `3.x.x` 版本（如 3.1.0、3.2.5），策略引擎自动解析主版本号为 3，并**自动回退复用 3.0 适配器**；
   - 同理，`2.x.x` 自动复用 2.3 适配器。
4. **第四层：默认兜底适配（Global Fallback）**：
   - 若遇到无法识别的非语义化版本，根据系统配置 `stx.adapter.version.fallback` 回退到最稳健的已注册适配器。

---

## 4. 全目录深度穿透与类加载健壮性

针对前文分析中由于未穿透连接器子目录导致的 `DECODE_FAILED (MySqlIncrementalSourceFactory)` 问题，以及 SeaTunnel 3.0 目录结构的变更，实施了双端全自动穿透：

### 4.1 Go Agent 深度递归穿透 (`agent/internal/installer/runtime_storage_probe.go`)
- 原实现只扫描 `connectors/` 和 `plugins/` 的第一层；
- 改造后：使用 `filepath.WalkDir`，扫描目录扩展至：
  - `connectors/`（自动下钻其子目录 `connectors/seatunnel/` 等，深度控制在 3 层内防软链循环）；
  - `plugins/`（自动下钻）；
  - `lib/`（Zeta 核心依赖）；
  - `starter/`（3.0 新引入的启动模块）；
  - `seatunnel-dist/`（3.0 新发版依赖库）。
- 所有扫描到的 Jar 经去重后随探针请求传递给 Java Proxy。

### 4.2 Java 端目录穿透与 ClassLoader 租约回收 (`PluginClassLoaderUtils.java`)
- `PluginClassLoaderUtils.collectJarPaths` 同样自动收集 `starter`、`seatunnel-dist` 以及环境变量 `STX_CONNECTOR_PATHS` / `-Dstx.connector.paths` 指定的附加目录；
- 当探针请求未显式携带 `pluginJars` 列表时，自动触发 `createClassLoaderFromSeatunnelHome`，确保绝不由于缺少环境参数而导致连接器解码失败；
- 维持基于文件大小与修改时间的指纹缓存（Fingerprint Cache），避免每次请求高频创建 URLClassLoader 造成的元空间泄漏。

---

## 5. SeaTunnel 3.0 Checkpoint 协议兼容与增量解析

在 `SeaTunnel30EngineAdapter` 中，针对 3.0 的新协议特性提供了专有解析支持：

### 5.1 增量快照元数据识别
3.0 针对流作业采用增量 Checkpoint，状态文件中可能不直接存储全部算子字节，而是记录增量文件索引：
- 适配器自动探测 `incremental` 布尔标记；
- 扫描 `incrementalStateFiles` 字典，若存在外部文件引用，将其标识为增量 Checkpoint，并在响应体标记 `isIncremental = true`。

### 5.2 挂起式 Savepoint 感知
- 3.0 引入了带优雅停机状态的 Savepoint（挂起任务）；
- 适配器提取 `checkpointType`（如 `SAVEPOINT_TYPE`、`SUSPEND_SAVEPOINT_TYPE`），并在响应中向上层提供 `isSavepoint = true`。

### 5.3 CDC SPI 双轨探测
- 2.3 时代的 CDC 依赖 `TableSourceFactory`；
- 3.0 的 CDC 引入了 `ChangeStreamTableSourceFactory` 与轻量化位点协议；
- Proxy 在执行连接器反射探测时，优先探测 3.0 的 `ChangeStreamTableSourceFactory`，若不存在则平滑回退至通用 `TableSourceFactory`。

---

## 6. 端到端验证与测试覆盖

本次重构落地并验证了以下单元测试套件：

| 测试类 | 验证范围 | 状态 |
| :--- | :--- | :--- |
| `EngineVersionTest` | 验证语义化版本解析、前导 `v` 去除、通配符 `matches("3.0.*")` 与跨版本比对 | **PASSED (100%)** |
| `EngineAdapterVersionPolicyTest` | 验证 2.3/3.0 精确路由、`3.1.0` 零代码前向兼容复用 `3.0` 适配器、自定义参数映射覆盖 | **PASSED (100%)** |
| `SeaTunnelEngineAdapterRegistryTest` | 验证多版本适配器单例注册表、动态根据版本分发、3.0 特性集（增量/表级状态）暴露 | **PASSED (100%)** |
| `CheckpointProbeServiceTest` | 验证 Checkpoint 探测服务与端到端链路 | **PASSED (100%)** |
| `CheckpointSourceActionMatcherTest` | 验证 Source 算子自动匹配与作业配置解析 | **PASSED (100%)** |
| `PluginClassLoaderUtilsTest` | 验证双 ClassLoader 穿透、文件指纹缓存与安全租约回收 | **PASSED (100%)** |
| `IMapProbeServiceTest` | 验证 IMap WAL 解析与底层状态探测 | **PASSED (100%)** |
| Go Agent Compile | 验证 Go 模块 `agent` 与根工程编译无任何语法或签名冲突 | **PASSED (100%)** |
