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

# STX Java Proxy 测试流程

这份文档聚焦 3 件事：

- 用 `HOCON` 作为第一优先级验证配置解析和 preview 派生
- 说明 `SeaTunnel 2.3.13` 在 `starter / lib / connectors / plugins` 四层里的真实依赖边界
- 给出 `MySQL + MinIO` 的一套可复用联调步骤

## 1. 2.3.13 依赖边界

建议按下面这个边界理解 `SEATUNNEL_HOME`：

| 目录 | 典型内容 | 说明 |
| --- | --- | --- |
| `starter/` | `seatunnel-starter.jar` | 主启动 jar。`2.3.13` 的 starter 内已经包含 `checkpoint-storage-hdfs`、`imap-storage-file`、`checkpoint-storage-local-file` 这些存储实现类。 |
| `lib/` | `seatunnel-hadoop3-3.1.4-uber`、`seatunnel-hadoop-aws`、`aws-java-sdk-bundle` 等 | Hadoop 文件系统和远端对象存储相关依赖。`starter` 并没有把这部分全部打进去。 |
| `connectors/` | `connector-jdbc-2.3.13.jar` 等 | SeaTunnel connector 主 jar。通常通过 `bin/install-plugin.sh` 下载到这里。 |
| `plugins/<ConnectorName>/` | `mysql-connector-java`、`postgresql`、`ojdbc8` 等 | connector 隔离依赖目录。proxy 会递归加载该目录下所有 jar。推荐仍然放在 `lib/` 子目录，便于和 SeaTunnel 现有习惯保持一致。 |

这几个边界可以从官方代码直接对应出来：

- starter 自身依赖了 `seatunnel-engine-server`：[seatunnel-starter/pom.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-core/seatunnel-starter/pom.xml)
- starter shade 时明确排除了 `seatunnel-hadoop3-3.1.4-uber`：[seatunnel-starter/pom.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-core/seatunnel-starter/pom.xml#L108)
- dist 会把 `seatunnel-starter.jar` 放到 `starter/`：[assembly-bin.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-dist/src/main/assembly/assembly-bin.xml#L149)
- dist 会把 `seatunnel-hadoop3-3.1.4-uber` 和 `seatunnel-hadoop-aws` 放到 `lib/`：[assembly-bin.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-dist/src/main/assembly/assembly-bin.xml#L170)
- connector jar 放到 `connectors/`：[install-plugin.sh](/Users/mac/Documents/projects/seatunnel/bin/install-plugin.sh)
- connector 隔离依赖约定在 `plugins/<ConnectorName>/lib/`：[connector-isolated-dependency.md](/Users/mac/Documents/projects/seatunnel/docs/zh/connectors/connector-isolated-dependency.md)

一句话总结：

- `checkpoint-storage-hdfs-2.3.13.jar` 这类实现类从运行效果上看已经在 `seatunnel-starter.jar` 里
- 但它依赖的 Hadoop/S3A/AWS 运行时并不都在 starter 里，仍然要靠 `lib/`
- `connector-jdbc` 要放 `connectors/`
- `mysql-connector-java` 这类第三方驱动建议放 `plugins/Jdbc/lib/`

## 2. 建议的 JDK

建议优先使用：

- `JDK 8`
- `JDK 11`

在较新的 JDK 上，尤其是 `JDK 26`，`imap probe` 可能遇到：

- `getSubject is not supported`

这不是 proxy 逻辑本身的问题，而是旧 Hadoop 路径与高版本 JDK 的兼容性问题。

## 3. 启动前预置

### 3.1 MinIO bucket

`checkpoint probe` 和 `imap probe` 测 MinIO 时，建议先创建 bucket。否则即使最终会失败，也可能把失败路径拖到更深的位置。

```bash
docker run --rm --network host --entrypoint /bin/sh docker.m.daocloud.io/minio/mc:latest -c '
  mc alias set local http://127.0.0.1:9000 minioadmin minioadmin123 >/dev/null &&
  mc mb --ignore-existing local/stx-proxy >/dev/null &&
  mc ls local
'
```

### 3.2 Proxy 的快速失败机制

当前 proxy 已经做了两层快速失败：

- 前置参数校验：
  - checkpoint 至少要求 `storage.type`、`namespace`
  - `storage.type = s3` 时要求 `s3.bucket`
  - imap 额外要求 `businessName`、`clusterName`
- 总超时：
  - 所有 `catalog/checkpoint/imap probe` 默认 `probeTimeoutMs = 15000`
  - 超时会返回 `504`

`s3` 路径还会补几项更保守的默认值：

- `fs.s3a.connection.establish.timeout = 5000`
- `fs.s3a.connection.timeout = 10000`
- `fs.s3a.attempts.maximum = 1`
- `fs.s3a.retry.limit = 1`

## 4. 组装最小 2.3.13 环境

下面是一套最小目录示意：

```text
/tmp/st_proxy_home_2313/
├── starter/
│   └── seatunnel-starter.jar
├── lib/
│   ├── seatunnel-hadoop3-3.1.4-uber-2.3.13-optional.jar
│   ├── seatunnel-hadoop-aws-2.3.13-optional.jar
│   ├── hadoop-aws-3.1.4.jar
│   ├── aws-java-sdk-bundle-1.11.271.jar
│   └── disruptor-3.4.4.jar
├── connectors/
│   └── connector-jdbc-2.3.13.jar
├── plugins/
│   └── Jdbc/
│       └── lib/
│           └── mysql-connector-java-8.0.27.jar
├── tools/
│   └── stx-java-proxy.jar
└── bin/
    └── stx-java-proxy.sh
```

说明：

- `disruptor` 对 imap 路径是必需的
- `seatunnel-hadoop-aws` 和 `aws-java-sdk-bundle` 对 `s3/minio` 很关键
- JDBC 驱动不建议打进 proxy 包，放 `plugins/Jdbc/lib/` 即可

## 5. 启动命令

```bash
export SEATUNNEL_HOME=/tmp/st_proxy_home_2313
export STX_JAVA_PROXY_JAR=${SEATUNNEL_HOME}/tools/stx-java-proxy.jar
export JAVA_HOME=/path/to/jdk8-or-jdk11

sh ${SEATUNNEL_HOME}/bin/stx-java-proxy.sh \
  -Dstx.java.proxy.port=18080
```

当前脚本会自动加载：

- `${SEATUNNEL_HOME}/starter/seatunnel-starter.jar`
- `${SEATUNNEL_HOME}/lib/*`
- `${SEATUNNEL_HOME}/connectors/*`
- `${SEATUNNEL_HOME}/plugins/<ConnectorName>/` 下所有 `.jar`

### 5.1 部署到已安装集群目录

如果不是临时测试目录，而是直接部署到现成 SeaTunnel 节点，建议按下面操作：

```bash
export SEATUNNEL_HOME=/opt/seatunnel
export STX_JAVA_PROXY_JAR=${SEATUNNEL_HOME}/tools/stx-java-proxy.jar
export JAVA_HOME=/path/to/jdk8-or-jdk11

cp tools/stx-java-proxy/target/stx-java-proxy-2.3.13-2.12.15.jar \
  ${STX_JAVA_PROXY_JAR}

cp tools/stx-java-proxy/bin/stx-java-proxy.sh \
  ${SEATUNNEL_HOME}/bin/stx-java-proxy.sh

sh ${SEATUNNEL_HOME}/bin/stx-java-proxy.sh \
  -Dstx.java.proxy.port=18080
```

部署后建议至少核对这几项：

- `${SEATUNNEL_HOME}/starter/seatunnel-starter.jar` 是否存在
- `${SEATUNNEL_HOME}/connectors/connector-jdbc-<version>.jar` 是否存在
- `${SEATUNNEL_HOME}/plugins/Jdbc/` 目录树下是否已有对应 JDBC 驱动
- `${SEATUNNEL_HOME}/lib/` 下是否已有 S3/OSS 所需的 Hadoop 相关 jar
- `curl http://127.0.0.1:18080/healthz` 是否返回 `{"ok":true}`

## 6. HOCON 优先测试

### 6.1 配置 DAG

```bash
cat >/tmp/st_dag_hocon.json <<'EOF'
{
  "contentFormat": "hocon",
  "content": "source { Jdbc { plugin_output = \\\"src\\\" url = \\\"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\\\" user = \\\"root\\\" password = \\\"seatunnel\\\" driver = \\\"com.mysql.cj.jdbc.Driver\\\" query = \\\"select * from users\\\" } } sink { Console { plugin_input = [\\\"src\\\"] } }"
}
EOF

curl -sS -X POST http://127.0.0.1:18080/api/v1/config/dag \
  -H 'Content-Type: application/json' \
  --data @/tmp/st_dag_hocon.json
```

### 6.2 Source preview 派生 HOCON

```bash
cat >/tmp/st_source_preview_hocon.json <<'EOF'
{
  "contentFormat": "hocon",
  "outputFormat": "hocon",
  "content": "source { Jdbc { plugin_output = \\\"src\\\" url = \\\"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\\\" user = \\\"root\\\" password = \\\"seatunnel\\\" driver = \\\"com.mysql.cj.jdbc.Driver\\\" query = \\\"select * from users\\\" } } sink { Console { plugin_input = [\\\"src\\\"] } }",
  "httpSink": {
    "url": "https://platform.example.com/api/preview/callback",
    "array_mode": false
  }
}
EOF

curl -sS -X POST http://127.0.0.1:18080/api/v1/config/preview/source \
  -H 'Content-Type: application/json' \
  --data @/tmp/st_source_preview_hocon.json
```

### 6.3 Transform preview 派生 HOCON

```bash
cat >/tmp/st_transform_preview_hocon.json <<'EOF'
{
  "contentFormat": "hocon",
  "outputFormat": "hocon",
  "transformIndex": 0,
  "content": "source { SourceA { plugin_output = \\\"a\\\" } SourceB { plugin_output = \\\"b\\\" } } transform { Joiner { plugin_input = [\\\"a\\\",\\\"b\\\"] plugin_output = \\\"joined\\\" } Cleaner { plugin_input = [\\\"joined\\\"] plugin_output = \\\"cleaned\\\" } } sink { Console { plugin_input = [\\\"cleaned\\\"] } }",
  "httpSink": {
    "url": "https://platform.example.com/api/preview/callback",
    "array_mode": false
  }
}
EOF

curl -sS -X POST http://127.0.0.1:18080/api/v1/config/preview/transform \
  -H 'Content-Type: application/json' \
  --data @/tmp/st_transform_preview_hocon.json
```

## 7. MySQL Catalog probe

```bash
cat >/tmp/st_catalog_mysql_hocon.json <<'EOF'
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
EOF

curl -sS -X POST http://127.0.0.1:18080/api/v1/catalog/probe \
  -H 'Content-Type: application/json' \
  --data @/tmp/st_catalog_mysql_hocon.json
```

重点看这些返回字段：

- `databases`
- `tables`
- `table.schema.columns`
- `table.schema.primaryKey`

## 8. MinIO checkpoint / imap probe

### 8.1 checkpoint probe

```bash
cat >/tmp/st_checkpoint_probe_minio.json <<'EOF'
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
EOF

curl -sS -X POST http://127.0.0.1:18080/api/v1/storage/checkpoint/probe \
  -H 'Content-Type: application/json' \
  --data @/tmp/st_checkpoint_probe_minio.json
```

### 8.2 imap probe

```bash
cat >/tmp/st_imap_probe_minio.json <<'EOF'
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
EOF

curl -sS -X POST http://127.0.0.1:18080/api/v1/storage/imap/probe \
  -H 'Content-Type: application/json' \
  --data @/tmp/st_imap_probe_minio.json
```

## 9. 故障注入与预期

### 9.1 bucket 不存在

把 `s3.bucket` 改成一个不存在的 bucket，例如：

- `s3a://stx-proxy-not-exists`

预期：

- 服务应在 `probeTimeoutMs` 内返回
- 可能是底层 500，也可能是 504 timeout，但不应该无限挂住

### 9.2 JDBC 驱动缺失

把 `${SEATUNNEL_HOME}/plugins/Jdbc/` 目录树里的 MySQL 驱动移走。

预期：

- `catalog/probe` 会快速失败
- 返回中应明确体现 driver/类加载失败信息

### 9.3 connector jar 缺失

把 `${SEATUNNEL_HOME}/connectors/connector-jdbc-2.3.13.jar` 去掉。

预期：

- `catalog/probe` 会快速失败
- 返回中应体现 catalog factory not found 或类加载失败

## 10. 当前模块自动化测试

当前模块里，HOCON 是第一优先级，相关测试已经覆盖：

- HOCON DAG 解析
- HOCON source preview 派生
- HOCON transform preview 派生
- probe 超时
- probe 必填参数快速失败

执行：

```bash
./mvnw -f tools/stx-java-proxy/pom.xml spotless:apply
./mvnw -f tools/stx-java-proxy/pom.xml test
./mvnw -f tools/stx-java-proxy/pom.xml -DskipTests verify
```
