# 架构设计: 基于 SeaTunnel SPI 与集群拓扑感知的全量连接器测试连接体系

## 1. 架构总览

本设计构建了一个从 **Web 控制台 -> 后端调度路由 -> 集群节点 Agent -> 引擎 Java Proxy -> SeaTunnel SPI 探测** 的端到端闭环体系：

```mermaid
flowchart TD
    subgraph Frontend["前端展示层 (DataSyncStudio)"]
        UI_Btn["点击【测试连接】"] --> UI_Modal["ValidationDialog 结构化结果展示"]
    end

    subgraph ControlPlane["STX 管控层 (Go Backend)"]
        API["POST /api/v1/sync/tasks/:id/validate<br/>or /api/v1/sync/configs/validate"]
        Router{"集群拓扑路由判断<br/>(Cluster/Node Binding)"}
        API --> Router
    end

    subgraph DataPlane["目标计算集群节点 (SeaTunnel Worker/Master)"]
        Agent["STX Agent (gRPC Server)"]
        Proxy["stx-java-proxy (Port: 18055)"]
        Agent -->|HTTP Proxy| Proxy
    end

    subgraph LocalPlane["管控本地执行 (草稿态兜底)"]
        LocalProxy["本地 stx-java-proxy"]
    end

    Router -->|"已绑定集群 (真实计算拓扑)"| Agent
    Router -->|"未指定集群 (草稿态)"| LocalProxy

    subgraph EngineSPI["SeaTunnel 运行时 SPI 探测链 (stx-java-proxy)"]
        Proxy --> Chain["ProbeHandler 责任链"]
        LocalProxy --> Chain

        Chain --> H1["1. SupportDryRunValidation SPI<br/>(Kafka, Jdbc, S3File, FakeSource)"]
        Chain --> H2["2. CatalogFactory SPI<br/>(40+ Catalogs: MySQL, PG, ClickHouse, Doris...)"]
        Chain --> H3["3. CDC 映射探针<br/>(MySQL-CDC -> MySqlCatalog 等)"]
        Chain --> H4["4. 专用轻量探针 (Redis, HTTP)"]
        Chain --> H5["5. Generic TCP Socket 3次握手兜底"]
        Chain --> H6["6. Native Local 免检白名单"]
    end

    subgraph ExternalDB["目标外部系统 (内网)"]
        H1 -->|"真实协议连接"| Ext1["Kafka / S3 / 数据库"]
        H2 -->|"Catalog.open()"| Ext2["MySQL / Doris / ES / Mongo..."]
        H3 -->|"Catalog 握手"| Ext3["MySQL-CDC / PG-CDC..."]
        H4 -->|"PING / HEAD"| Ext4["Redis / Webhook..."]
        H5 -->|"TCP 握手"| Ext5["任意 Host:Port 外部服务"]
    end
```

---

## 2. Java Proxy 核心设计：SPI 探测责任链 (Probe Chain)

在 `stx-java-proxy` 中，摒弃过去的单个 `if (!"Jdbc".equalsIgnoreCase(pluginName))` 硬编码，改为标准的**可插拔责任链（Chain of Responsibility）**设计：

```mermaid
flowchart TD
    Start["收到连接器配置 (PluginName, Config)"] --> Step1{"是否属于本地免检白名单?<br/>(FakeSource, Console, Assert)"}
    Step1 -->|是| Res_Skip["返回 status: skipped, severity: info<br/>(提示无需外部连接，不阻断 valid)"]
    
    Step1 -->|否| Step2{"Connector Factory 是否实现<br/>SupportDryRunValidation SPI?"}
    Step2 -->|是| Res_DryRun["调用 factory.validateConnectionForDryRun()<br/>(Kafka AdminClient / S3 Bucket / JDBC 驱动)"]
    
    Step2 -->|否| Step3{"是否存在对应或映射的<br/>CatalogFactory?"}
    Step3 -->|是| Res_Catalog["FactoryUtil.discoverFactory(CatalogFactory)<br/>catalog.open() 建立物理连接"]
    
    Step3 -->|否| Step4{"是否为 CDC 连接器?<br/>(MySQL-CDC, Postgres-CDC...)"}
    Step4 -->|是| Res_CDC["剥离 -CDC 后缀路由至底层 Catalog<br/>或提取 JDBC URL 执行驱动连接"]
    
    Step4 -->|否| Step5{"是否支持专用协议?<br/>(Redis, HTTP REST)"}
    Step5 -->|是| Res_Special["执行专有轻量探针 (PING / HEAD)"]
    
    Step5 -->|否| Step6{"配置中是否包含 Host/Port/URL?"}
    Step6 -->|是| Res_TCP["执行 Generic TCP Socket 探针 (三次握手超时 2s)"]
    Step6 -->|否| Res_Unsupported["返回 status: skipped, severity: info<br/>(提示暂无对应网络探针，不阻断 valid)"]
```

### 2.1 SeaTunnel 源码契约映射表

根据我们对 SeaTunnel v3.0.0 源码 80+ 个连接器模块的实际扫描，探测机制映射如下：

| 连接器类别 | 包含插件 | 探测技术方案 |
| :--- | :--- | :--- |
| **官方 DryRun SPI** | `Kafka`, `Jdbc`, `S3File`, `FakeSource` | `SupportSourceDryRunValidation.validateConnectionForDryRun()`<br/>`SupportSinkDryRunValidation.validateConnectionForDryRun()` |
| **关系型与分析型 (Catalog 覆盖)** | `MySQL`, `PostgreSQL`, `Oracle`, `SQLServer`, `ClickHouse`, `Doris`, `StarRocks`, `Databend`, `DB2`, `Dameng`, `Kingbase`, `OpenGauss`, `TiDB`, `OceanBase`, `Redshift`, `SapHana`, `DuckDB`, `HighGo`, `Iris`, `Xugu`, `YashanDb` (共 21 种) | `FactoryUtil.discoverOptionalFactory(classLoader, CatalogFactory.class, identifier)`<br/>调用 `catalogFactory.createCatalog().open()` |
| **CDC 族 (官方 Catalog 映射)** | `MySQL-CDC`, `Postgres-CDC`, `Oracle-CDC`, `SqlServer-CDC`, `TiDB-CDC` | 遵循 SeaTunnel `CatalogTableUtil.getCatalogTables` 官方规范：<br/>`factoryId = pluginName.replace("-CDC", "")`<br/>调用底层对应数据库 Catalog 建立连接并探测 |
| **数据湖与对象存储 (Catalog 覆盖)** | `Iceberg`, `Hudi`, `Paimon`, `Kudu`, `MaxCompute`, `S3File`, `OssFile`, `HdfsFile`, `GcsFile` | 动态加载对应 `CatalogFactory` 执行探测 |
| **搜索引擎与 NoSQL (Catalog 覆盖)** | `Elasticsearch`, `Easysearch`, `MongoDB`, `Milvus`, `Typesense`, `Hbase` | 动态加载对应 `CatalogFactory` 执行探测 |
| **专用网络探针** | `Redis` (RESP PING), `Http` (HTTP GET/HEAD) | 专用轻量探针 |
| **通用 TCP 兜底** | 任何包含 `host` / `hostname` / `port` 或 `url` 的第三方外部连接器 | `Socket.connect(new InetSocketAddress(host, port), 2000)` |
| **本地免检组件** | `FakeSource`, `Console`, `Assert`, `LocalFile` | 声明为无需外部连接，返回 `skipped` + `severity: info` |

---

## 3. 集群拓扑路由与调度设计 (STX Agent Router)

### 3.1 为什么必须经过 Agent？
若 STX 仅在自己所在的服务器上执行探测，无法真实反映计算集群的网络环境。

### 3.2 路由分发逻辑 (Go 后端)

```go
// internal/apps/sync/service.go
func (s *SyncService) ValidateTaskConnection(ctx context.Context, taskID uint, req *ValidateRequest) (*ValidationResult, error) {
    task, err := s.getTask(taskID)
    if err != nil {
        return nil, err
    }
    
    // 1. 获取任务绑定的目标集群与节点
    clusterID := task.TargetClusterID
    if clusterID > 0 {
        activeNode, err := s.clusterService.GetActiveExecutionNode(clusterID)
        if err == nil && activeNode != nil {
            // 2. 路由到集群节点的 Agent 执行
            return s.agentClient.ValidateConfigOnNode(ctx, activeNode.AgentID, req)
        }
    }
    
    // 3. 兜底：未绑定集群或 Agent 离线时，使用本地 Java Proxy 执行
    return s.javaProxyClient.ValidateConfig(ctx, req)
}
```

---

## 4. 三态判定模型与 API 响应协议

### 4.1 Java Proxy 响应体契约升级

```json
{
  "ok": true,
  "valid": true,
  "probed_by": {
    "execution_target": "agent-worker-01",
    "host": "10.0.0.211",
    "engine_version": "3.0.0"
  },
  "checks": [
    {
      "nodeId": "source-0",
      "kind": "source",
      "connectorType": "Source[0]-MySQL-CDC",
      "target": "127.0.0.1:3306",
      "status": "success",
      "probeMethod": "catalog-mysql",
      "latencyMs": 15,
      "message": "Connection and metadata probed successfully via MySqlCatalog."
    },
    {
      "nodeId": "sink-0",
      "kind": "sink",
      "connectorType": "Sink[0]-Console",
      "target": "",
      "status": "skipped",
      "severity": "info",
      "probeMethod": "native-local",
      "latencyMs": 0,
      "message": "该连接器不依赖外部服务，无需连接测试。"
    }
  ],
  "errors": [],
  "warnings": [],
  "infos": [
    "Sink[0]-Console 无需连接测试: 该连接器不依赖外部服务，无需连接测试。"
  ],
  "summary": "Config validation and connection test passed."
}
```

### 4.2 核心规则判定代码修正
在 `ConfigValidationService.java` 中：
```java
// 严禁将 skipped 计入 allChecksSucceeded 失败！
if ("failed".equals(status)) {
    errors.add(String.format("%s 连接失败: %s", check.get("connectorType"), check.get("message")));
    allChecksSucceeded = false;
} else if ("skipped".equals(status)) {
    // 无论是本地免检还是平台能力兜底，均为 info，绝不阻断验证
    infos.add(String.format("%s 连接状态: %s", check.get("connectorType"), check.get("message")));
}
// 最终判断：只有真正发生错误时 valid 才为 false
boolean passed = errors.isEmpty();
```

---

## 5. 安全与稳健性保障

1. **类加载器安全**：必须使用 SeaTunnel 专用的 `PluginClassLoader`，并在探测结束后的 `finally` 块中严格还原 `Thread.currentThread().setContextClassLoader(originalClassLoader)`。
2. **连接超时熔断**：每个探测分支设置严格的 Socket / JDBC Connect Timeout（默认 2000ms），总探测设有 `Future.get(5000, TimeUnit.MILLISECONDS)` 超时熔断保护，防止目标 IP 无响应卡死线程。
3. **敏感凭证脱敏**：返回给前端的报错信息统一经过 `DryRunConnectFailureMessageSanitizer` 脱敏，确保不会出现明文数据库密码。
