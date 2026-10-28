# PRD: 集群标识(cluster_code)生命周期与IMAP clusterName自动继承

## 背景与问题

1. **SeaTunnel IMAP 存储异常中断**：
   在 SeaTunnel 3.0.0 启用 IMap 存储（如 HDFS / 本地文件持久化）后，Master 选主时抛出：
   `java.lang.IllegalArgumentException: init file storage properties is not contains clusterName`。
   经排查，SeaTunnel 引擎端 `IMapFileStorage.checkInitStorageProperties` 强制要求在 `properties` 中包含 `businessName`（由 `FileMapStoreFactory` 注入为 mapName）和 `clusterName`。此前 STX 补丁仅注入了 `fs.defaultFS` / `path` 等，缺少 `clusterName`。
2. **集群名称与物理 Hazelcast 集群标识割裂**：
   STX 中的集群拥有用户可读名称（例如中文或任意字符），但底层的 Hazelcast 实例通信、多节点互联、客户端连接、以及 IMAP 存储在文件系统中的持久化目录结构（`${namespace}/${clusterName}/${businessName}/`）都需要一个稳定的字符标识 `cluster-name`。
3. **改名级联破坏风险**：
   若用户后续修改集群展示名称，物理 `cluster-name` 决不能随之改动，否则会导致：
   - 既有 IMAP 历史持久化数据孤立断链。
   - 节点间与客户端 Hazelcast 握手失败。
   - 需要停止全部节点、修改所有配置文件并重新冷启动。

## 需求目标与方案契约

### 1. 集群标识 `cluster_code` 生命周期
- **格式规范**：仅允许小写字母与数字（`^[a-z0-9]{2,32}$`）。
- **创建阶段**：
  - 前端向导在用户输入集群名称（例如“数仓实时计算”）时，根据名称拼音或字母数字自动推荐 `cluster_code`（如 `shucangshishi` 或拼音首字母/截断）。
  - 用户可自由手动编辑调整该值。
  - 后端在接收到请求时进行清洗校验（`SanitizeClusterCode` / `IsValidClusterCode`），若为空则自动根据名称兜底生成合规的唯一标识。
- **创建后永久不可变**：
  - 集群重命名（`clusterService.Update`）只修改 `name` 和 `description`，决不修改 `cluster_code`。
  - 前端集群详情与设置界面中，`cluster_code` 必须作为只读标识展示，明确提示“集群标识作为物理节点通信与存储隔离标识，创建后不可修改”。

### 2. 纳管已有集群（Import/Discovery）
- 当导入已有外部节点（未指定 `cluster_code`）时，系统读取主节点的 `hazelcast.yaml`，提取已有的 `hazelcast.cluster-name` 作为本集群的 `cluster_code`，保障与物理现状完全一致。

### 3. 安装与扩容自动配置
- 在集群新节点安装、扩容时，Installer Manager 自动将 `cluster_code` 写入：
  - `hazelcast.yaml`（或 master/worker 对应配置文件）中的 `hazelcast.cluster-name`。
  - `hazelcast-client.yaml` 中的 `hazelcast-client.cluster-name`。
  - IMAP `map-store.properties` 中的 `clusterName`。

### 4. IMAP 运行时配置静默继承
- 在通过 STX 启用/切换 IMAP 存储时，前端无需向用户额外暴露 `clusterName` 输入框。
- 后端及 Agent 静默读取当前集群的 `EffectiveClusterCode()` 或从当前节点的 `hazelcast.yaml` 中解析 `cluster-name`，自动注入 `properties` 中的 `clusterName`。

### 5. 在线配置编辑风险预警（Non-blocking Risk Warning）
- 用户在 Web 在线配置文件编辑器（`FileConfigTab`）修改 `hazelcast.yaml` 或 `hazelcast-client.yaml` 时：
  - 若检测到 `cluster-name` 发生变更，不进行强行拦截阻断，但必须弹出风险确认对话框：
    - 明确告知：“修改集群标识（cluster-name）属于高危操作：需同步手动修改所有节点与客户端配置文件并重启整个集群；且可能导致控制台元数据与存储路径不同步。是否确认保存并知晓风险？”
  - 用户点击确认后方可提交保存。
