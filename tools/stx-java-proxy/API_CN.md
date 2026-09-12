<!--
  Licensed to the Apache Software Foundation (ASF) under one or more
  contributor license agreements.  See the NOTICE file distributed with
  this work for additional information regarding copyright ownership.
  The ASF licenses this file to You under the Apache License, Version 2.0
  (the "License"); you may not use this file except in compliance with
  the License.  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
-->

# STX Java Proxy 接口文档

本文档描述 `stx-java-proxy` 当前对外暴露的 HTTP 接口。

## 1. 基础约定

- Base URL 示例：`http://127.0.0.1:18080`
- 只有 `GET /healthz` 是 `GET` 接口
- 所有 `/api/v1/**` 接口都使用 `POST`
- 请求体和响应体都是 JSON
- JSON 里的配置内容可以是 `hocon` 或 `json`

如果对 `POST` 接口使用了其它 HTTP 方法，返回：

```json
{
  "ok": false,
  "message": "Method not allowed"
}
```

## 2. 通用响应结构

### 成功

成功请求返回 `HTTP 200`，响应体由各接口定义。

### 失败

失败统一使用：

```json
{
  "ok": false,
  "message": "..."
}
```

常见状态码：

- `400`：请求字段缺失、模式不支持、参数非法、配置非法
- `405`：请求方法不支持
- `500`：运行期失败、依赖缺失、远端初始化失败
- `504`：probe 超时

## 3. 通用配置输入

`/api/v1/config/dag`、`/api/v1/config/preview/source` 和
`/api/v1/config/preview/transform` 共享同一套配置输入规则。

支持字段：

- `content`
  - 内联 SeaTunnel 配置文本
- `contentFormat`
  - `hocon` 或 `json`
  - 默认：`hocon`
- `filePath`
  - proxy 所在机器可访问的配置文件路径
- `variables`
  - `key=value` 形式的数组

规则：

- `content` 和 `filePath` 二选一，不能同时传
- `contentFormat` 不传时默认按 `hocon`
- `variables` 中出现非法项时返回 `400`

示例：

```json
{
  "contentFormat": "hocon",
  "variables": [
    "db_host=127.0.0.1",
    "db_port=3307"
  ],
  "content": "source { Jdbc { plugin_output = \"src\" url = \"jdbc:mysql://${db_host}:${db_port}/seatunnel_demo\" user = \"root\" password = \"seatunnel\" driver = \"com.mysql.cj.jdbc.Driver\" query = \"select * from users\" } } sink { Console { plugin_input = [\"src\"] } }"
}
```

## 4. 健康检查

### `GET /healthz`

返回：

```json
{
  "ok": true
}
```

## 5. 配置级 DAG 解析

### `POST /api/v1/config/dag`

使用 SeaTunnel 原生配置解析逻辑读取任务配置，并返回一张轻量的配置级 DAG。

#### 请求

使用通用配置输入。

#### 响应

```json
{
  "ok": true,
  "simpleGraph": true,
  "sourceCount": 1,
  "transformCount": 0,
  "sinkCount": 1,
  "warnings": [],
  "graph": {
    "nodes": [
      {
        "nodeId": "source-0",
        "kind": "SOURCE",
        "pluginName": "Jdbc",
        "configIndex": 0,
        "inputDatasets": [],
        "outputDataset": "src"
      },
      {
        "nodeId": "sink-0",
        "kind": "SINK",
        "pluginName": "Console",
        "configIndex": 0,
        "inputDatasets": [
          "src"
        ],
        "outputDataset": null
      }
    ],
    "edges": [
      {
        "fromDataset": "src",
        "fromNodeId": "source-0",
        "toNodeId": "sink-0"
      }
    ]
  }
}
```

#### 关键字段

- `simpleGraph`
  - 当配置是 `1 source + 1 sink + 可选 1 transform` 时为 `true`
- `warnings`
  - 简单图兼容连线时的提示信息
- `graph.nodes[].nodeId`
  - proxy 内部稳定节点 ID，例如 `source-0`、`transform-1`
- `graph.nodes[].configIndex`
  - 节点在原始 `source` / `transform` / `sink` 数组中的索引

## 6. Source Preview 配置派生

### `POST /api/v1/config/preview/source`

生成一份只用于预览的任务配置：

- 保留一个选中的 source
- 删除用户原始 sink
- 追加 `Metadata`
- 追加 `Http`

#### 请求

使用通用配置输入，并额外支持：

- `sourceNodeId`
  - 可选，例如 `source-0`
- `sourceIndex`
  - 可选，数值索引
- `outputFormat`
  - `json` 或 `hocon`
  - 默认：`json`
- `metadataOutputDataset`
  - 可选，默认 `__st_preview_rows`
- `metadataFields`
  - 可选，对象
- `envOverrides`
  - 可选，会 merge 到根级 `env`
- `httpSink`
  - 必填对象
  - 必须包含 `url`

选择规则：

- 如果只有一个 source，可以不传选择字段
- 如果有多个 source，必须传 `sourceNodeId` 或 `sourceIndex`
- 如果两个都传，必须指向同一个 source

默认元数据字段：

```json
{
  "Database": "__st_debug_db",
  "Table": "__st_debug_table",
  "RowKind": "__st_debug_rowkind"
}
```

最小请求示例：

```json
{
  "contentFormat": "hocon",
  "outputFormat": "hocon",
  "content": "source { Jdbc { plugin_output = \"src\" url = \"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\" user = \"root\" password = \"seatunnel\" driver = \"com.mysql.cj.jdbc.Driver\" query = \"select * from users\" } } sink { Console { plugin_input = [\"src\"] } }",
  "httpSink": {
    "url": "https://platform.example.com/api/preview/callback",
    "array_mode": false
  }
}
```

#### 响应

```json
{
  "ok": true,
  "mode": "source_preview",
  "selectedNodeId": "source-0",
  "selectedIndex": 0,
  "warnings": [],
  "content": "source { ... } transform { ... } sink { ... }",
  "contentFormat": "hocon",
  "config": {
    "source": [
      {
        "plugin_name": "Jdbc"
      }
    ],
    "transform": [
      {
        "plugin_name": "Metadata"
      }
    ],
    "sink": [
      {
        "plugin_name": "Http"
      }
    ]
  },
  "graph": {
    "nodes": [],
    "edges": []
  },
  "simpleGraph": true
}
```

关键字段：

- `content`
  - 渲染后的 preview 配置文本
- `contentFormat`
  - `content` 的实际格式
- `config`
  - 结构化配置对象
- `graph`
  - 派生后 preview 配置对应的 DAG

## 7. Transform Preview 配置派生

### `POST /api/v1/config/preview/transform`

生成一份只用于预览的配置，保留某个 transform 的上游链路，并在末尾追加
`Metadata` 和 `Http`。

#### 请求

使用通用配置输入，并额外支持：

- `transformNodeId`
  - 可选，例如 `transform-0`
- `transformIndex`
  - 可选，数值索引
- `outputFormat`
  - `json` 或 `hocon`
- `metadataOutputDataset`
  - 可选
- `metadataFields`
  - 可选，对象
- `envOverrides`
  - 可选，会 merge 到根级 `env`
- `httpSink`
  - 必填对象，必须包含 `url`

选择规则：

- 至少要有一个 transform
- 如果只有一个 transform，可以不传选择字段
- 如果有多个 transform，必须传 `transformNodeId` 或 `transformIndex`

最小请求示例：

```json
{
  "contentFormat": "hocon",
  "outputFormat": "hocon",
  "transformIndex": 0,
  "content": "source { SourceA { plugin_output = \"a\" } SourceB { plugin_output = \"b\" } } transform { Joiner { plugin_input = [\"a\",\"b\"] plugin_output = \"joined\" } Cleaner { plugin_input = [\"joined\"] plugin_output = \"cleaned\" } } sink { Console { plugin_input = [\"cleaned\"] } }",
  "httpSink": {
    "url": "https://platform.example.com/api/preview/callback",
    "array_mode": false
  }
}
```

响应字段与 source preview 基本一致，只是：

- `mode = "transform_preview"`
- `selectedNodeId` 指向选中的 transform

## 8. Catalog Probe

### `POST /api/v1/catalog/probe`

通过 `factoryIdentifier` 创建 SeaTunnel `Catalog`，可选地列出数据库、表，以及获取一张表的 schema。

#### 请求

- `factoryIdentifier`
  - 必填
  - 例如：`MySQL`、`Oracle`、`Dameng`
- `options`
  - 必填对象
  - 直接传给 SeaTunnel Catalog 工厂
- `catalogName`
  - 可选
  - 默认：`stx_java_proxy_catalog`
- `pluginJars`
  - 可选，jar 路径数组
- `includeDatabases`
  - 可选布尔值
  - 默认：`true`
- `databaseName`
  - 可选
  - 传了就会返回 `tables`
- `tablePath`
  - 可选
  - 可以是字符串，例如 `db.schema.table`
  - 也可以是对象：`databaseName`、`schemaName`、`tableName`

示例：

```json
{
  "factoryIdentifier": "MySQL",
  "databaseName": "seatunnel_demo",
  "tablePath": {
    "databaseName": "seatunnel_demo",
    "tableName": "users"
  },
  "options": {
    "url": "jdbc:mysql://127.0.0.1:3307/seatunnel_demo",
    "username": "root",
    "password": "seatunnel",
    "driver": "com.mysql.cj.jdbc.Driver"
  }
}
```

#### 响应

```json
{
  "ok": true,
  "factoryIdentifier": "MySQL",
  "catalogName": "stx_java_proxy_catalog",
  "defaultDatabase": "seatunnel_demo",
  "databases": [
    "seatunnel_demo"
  ],
  "tables": [
    "users"
  ],
  "table": {
    "tablePath": "seatunnel_demo.users",
    "catalogName": "stx_java_proxy_catalog",
    "comment": null,
    "partitionKeys": [],
    "options": {},
    "schema": {
      "columns": [
        {
          "name": "id",
          "dataType": "BIGINT",
          "columnLength": null,
          "scale": null,
          "nullable": false,
          "defaultValue": null,
          "comment": null,
          "sourceType": "BIGINT",
          "sinkType": null,
          "options": {}
        }
      ],
      "primaryKey": {
        "name": "PRIMARY",
        "columnNames": [
          "id"
        ],
        "enableAutoId": false
      },
      "constraintKeys": []
    }
  }
}
```

## 9. Checkpoint 存储 Probe

### `POST /api/v1/storage/checkpoint/probe`

创建 SeaTunnel checkpoint storage，并可选执行一次写入、读取、删除的验证流程。

#### 请求

- `plugin`
  - 必填
  - 当前只支持：`hdfs`
- `mode`
  - 可选
  - `read_write` 或 `init_only`
  - 默认：`read_write`
- `probeTimeoutMs`
  - 可选
  - 默认：`15000`
- `timeoutMs`
  - 可选，`probeTimeoutMs` 的别名
- `pluginJars`
  - 可选，jar 路径数组
- `config`
  - 必填对象

只支持远端模式：

- 本地模式会直接拒绝
- `storage.type` 不能是 `local` 或 `localfile`

`config` 必填项：

- `storage.type`
- `namespace`
- `s3.bucket`，当 `storage.type = s3`

示例：

```json
{
  "plugin": "hdfs",
  "probeTimeoutMs": 15000,
  "config": {
    "storage.type": "s3",
    "namespace": "/checkpoint-probe/",
    "s3.bucket": "s3a://stx-proxy",
    "fs.s3a.endpoint": "http://127.0.0.1:9000",
    "fs.s3a.access.key": "minioadmin",
    "fs.s3a.secret.key": "minioadmin123",
    "fs.s3a.path.style.access": "true",
    "fs.s3a.connection.ssl.enabled": "false",
    "fs.s3a.aws.credentials.provider": "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider"
  }
}
```

当 `storage.type = s3` 且这些参数缺失时，proxy 会自动补：

- `fs.s3a.connection.establish.timeout = 5000`
- `fs.s3a.connection.timeout = 10000`
- `fs.s3a.attempts.maximum = 1`
- `fs.s3a.retry.limit = 1`

#### 响应

`init_only`：

```json
{
  "ok": true,
  "plugin": "hdfs",
  "mode": "init_only",
  "storageType": "s3",
  "initialized": true
}
```

`read_write`：

```json
{
  "ok": true,
  "plugin": "hdfs",
  "mode": "read_write",
  "storageType": "s3",
  "writable": true,
  "readable": true,
  "storedCheckpoint": "..."
}
```

## 10. IMap 存储 Probe

### `POST /api/v1/storage/imap/probe`

创建 SeaTunnel imap storage，并可选执行一次写入、读取、删除的验证流程。

#### 请求

- `plugin`
  - 可选
  - 默认：`hdfs`
- `mode`
  - 可选
  - `read_write` 或 `init_only`
  - 默认：`read_write`
- `probeTimeoutMs`
  - 可选
  - 默认：`15000`
- `timeoutMs`
  - 可选，`probeTimeoutMs` 的别名
- `deleteAllOnDestroy`
  - 可选布尔值
  - 默认：`false`
- `pluginJars`
  - 可选，jar 路径数组
- `config`
  - 必填对象

`config` 必填项：

- `storage.type`
- `namespace`
- `businessName`
- `clusterName`
- `s3.bucket`，当 `storage.type = s3`

只支持远端模式：

- 本地模式会直接拒绝
- `storage.type` 不能是 `local` 或 `localfile`

proxy 还会自动补：

- 与 checkpoint probe 相同的 `s3` 快速失败默认值
- `writeDataTimeoutMilliseconds`
  - 默认 `max(1000, probeTimeoutMs)`

示例：

```json
{
  "plugin": "hdfs",
  "probeTimeoutMs": 15000,
  "deleteAllOnDestroy": false,
  "config": {
    "storage.type": "s3",
    "namespace": "/seatunnel-imap-probe",
    "businessName": "stx-java-proxy",
    "clusterName": "local-minio-test",
    "s3.bucket": "s3a://stx-proxy",
    "fs.s3a.endpoint": "http://127.0.0.1:9000",
    "fs.s3a.access.key": "minioadmin",
    "fs.s3a.secret.key": "minioadmin123",
    "fs.s3a.path.style.access": "true",
    "fs.s3a.connection.ssl.enabled": "false",
    "fs.s3a.aws.credentials.provider": "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider"
  }
}
```

#### 响应

`init_only`：

```json
{
  "ok": true,
  "plugin": "hdfs",
  "mode": "init_only",
  "storageType": "s3",
  "initialized": true
}
```

`read_write`：

```json
{
  "ok": true,
  "plugin": "hdfs",
  "mode": "read_write",
  "storageType": "s3",
  "writable": true,
  "readable": true
}
```
