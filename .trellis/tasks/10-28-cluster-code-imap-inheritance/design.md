# 技术设计: 集群标识(cluster_code)生命周期与IMAP clusterName自动继承

## 架构与跨层数据流

```
[Web 前端向导 / CLI]
    │
    │ cluster_code (推荐或手填)
    ▼
[STX Backend: clusterService.Create] ─── Sanitize / Validate
    │                                ─── 若为空则从 name 拼音/英数兜底
    │                                ─── 若纳管已有节点则从 hazelcast.yaml 提取
    ▼
[Database: clusters 表] ── 保存 cluster_code (只读不可变)
    │
    │ 安装 / 扩容 / 配置下发
    ▼
[STX Backend: installerService / scaleExecutor]
    │
    │ params.ClusterName = cluster.EffectiveClusterCode()
    ▼
[STX Agent / InstallerManager]
    ├── modifyHazelcastConfig: 设置 hazelcast.cluster-name
    ├── modifyHazelcastClientConfig: 设置 hazelcast-client.cluster-name
    └── configureIMAPStorage: 继承 ClusterName 注入 map-store.properties
```

## 关键模块细节

### 1. `clustercode.go` 工具库
- `SanitizeClusterCode(name string) string`：
  - 处理中文（拼音音节转换），去除非字母数字字符，转为小写。
  - 截断长度限制在 2 ~ 32 字符之间。
  - 若处理后为空，提供带哈希或短随机的合法默认值（如 `cluster` + 4位hex）。
- `IsValidClusterCode(code string) bool`：
  - 正则表达式 `^[a-z0-9]{2,32}$`。

### 2. 数据库与领域模型
- `internal/apps/cluster/model.go`：
  - `Cluster` 结构体增加 `ClusterCode string json:"cluster_code" gorm:"column:cluster_code;size:64;index"`。
  - `CreateClusterRequest` 增加 `ClusterCode string json:"cluster_code"`。
  - `ClusterInfo` 增加 `ClusterCode string json:"cluster_code"`。
  - 提供 `EffectiveClusterCode() string`：若 `ClusterCode != ""` 则返回 `ClusterCode`；否则返回由 `SanitizeClusterCode(c.Name)` 生成的值。
  - `UpdateClusterRequest` 明确不包含 `ClusterCode`，`clusterService.Update` 忽略任何 `cluster_code` 的写入，确保不可变契约。

### 3. IMAP 存储补丁与静默继承
- `internal/apps/cluster/runtime_storage_apply.go`：
  - `patchIMAPYAML` 在组装 `map-store.properties` 时，调用 `resolveIMAPClusterNameFromYAML(yamlContent)` 或使用集群的 `EffectiveClusterCode()`。
  - 保证 `properties` 中始终存在 `clusterName: <cluster_name>`。
- `agent/internal/installer/manager.go`：
  - `configureIMAPStorage` 读取 `params.ClusterName`，若为空再退回到 `hazelcast.yaml` 的 `cluster-name`，保证 `clusterName` 写入 `map-store.properties`。

### 4. 纳管外部集群导入
- 在 `clusterService.Create` 中，如果用户导入已有节点且未指定 `cluster_code`：
  - 读取节点对应安装目录下的 `config/hazelcast.yaml`。
  - 解析其中的 `hazelcast.cluster-name` 并赋值给 `cluster.ClusterCode`。

### 5. 在线配置编辑高危变更提示
- `frontend/components/common/config/FileConfigTab.tsx`：
  - 监听保存操作。若当前保存的文件是 `hazelcast.yaml` 或 `hazelcast-client.yaml`，解析/对比其 `cluster-name` 与原有配置中的 `cluster-name`。
  - 若 `cluster-name` 发生了修改，弹出非阻塞确认对话框：
    - 风险文案警告修改需要所有节点及客户端同步修改并冷重启，可能导致 STX 存储路径不同步。
    - 用户确认后执行保存。
