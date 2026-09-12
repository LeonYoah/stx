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

# STX Java Proxy Test Workflow

This document focuses on three things:

- validating config parsing and preview derivation with `HOCON` as the first priority
- clarifying the real dependency boundaries of SeaTunnel `2.3.13` across `starter / lib / connectors / plugins`
- providing a reusable `MySQL + MinIO` verification flow

## 1. Dependency Boundaries in 2.3.13

Use the following model when preparing `SEATUNNEL_HOME`:

| Directory | Typical content | Notes |
| --- | --- | --- |
| `starter/` | `seatunnel-starter.jar` | Main bootstrap jar. In `2.3.13`, starter already contains storage implementation classes such as `checkpoint-storage-hdfs`, `imap-storage-file`, and `checkpoint-storage-local-file`. |
| `lib/` | `seatunnel-hadoop3-3.1.4-uber`, `seatunnel-hadoop-aws`, `aws-java-sdk-bundle` | Hadoop filesystem and remote object-storage runtime jars. These are not fully bundled into starter. |
| `connectors/` | `connector-jdbc-2.3.13.jar` | Main SeaTunnel connector jars. Usually populated by `bin/install-plugin.sh`. |
| `plugins/<ConnectorName>/` | `mysql-connector-java`, `postgresql`, `ojdbc8` | Connector-isolated dependencies. The proxy recursively loads all jars under each connector directory. Keeping them under `lib/` subdirectories is still the recommended layout. |

Relevant code and packaging references:

- starter depends on `seatunnel-engine-server`: [seatunnel-starter/pom.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-core/seatunnel-starter/pom.xml)
- starter shade explicitly excludes `seatunnel-hadoop3-3.1.4-uber`: [seatunnel-starter/pom.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-core/seatunnel-starter/pom.xml#L108)
- dist puts `seatunnel-starter.jar` under `starter/`: [assembly-bin.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-dist/src/main/assembly/assembly-bin.xml#L149)
- dist puts `seatunnel-hadoop3-3.1.4-uber` and `seatunnel-hadoop-aws` under `lib/`: [assembly-bin.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-dist/src/main/assembly/assembly-bin.xml#L170)
- connector jars go into `connectors/`: [install-plugin.sh](/Users/mac/Documents/projects/seatunnel/bin/install-plugin.sh)
- connector isolated dependency convention is documented here: [connector-isolated-dependency.md](/Users/mac/Documents/projects/seatunnel/docs/en/connectors/connector-isolated-dependency.md)

In short:

- implementation classes such as `checkpoint-storage-hdfs` are effectively available from `seatunnel-starter.jar`
- Hadoop, S3A, and AWS runtime jars still need to come from `lib/`
- `connector-jdbc` still belongs in `connectors/`
- third-party drivers such as MySQL are still best placed under `plugins/Jdbc/lib/`

## 2. Recommended JDK

Prefer:

- `JDK 8`
- `JDK 11`

On newer JDKs, especially `JDK 26`, `imap probe` may fail with:

- `getSubject is not supported`

That is a Hadoop compatibility issue rather than a proxy-specific bug.

## 3. Pre-Provisioning

### 3.1 MinIO bucket

Create the bucket before running MinIO-based probes:

```bash
docker run --rm --network host --entrypoint /bin/sh docker.m.daocloud.io/minio/mc:latest -c '
  mc alias set local http://127.0.0.1:9000 minioadmin minioadmin123 >/dev/null &&
  mc mb --ignore-existing local/stx-proxy >/dev/null &&
  mc ls local
'
```

### 3.2 Fast-fail behavior

The proxy includes two layers of fast-fail protection:

- upfront validation:
  - checkpoint requires at least `storage.type` and `namespace`
  - `storage.type = s3` additionally requires `s3.bucket`
  - imap additionally requires `businessName` and `clusterName`
- global timeout:
  - all `catalog/checkpoint/imap probe` requests default to `probeTimeoutMs = 15000`
  - timeouts return `504`

For `s3`, the proxy also applies conservative defaults:

- `fs.s3a.connection.establish.timeout = 5000`
- `fs.s3a.connection.timeout = 10000`
- `fs.s3a.attempts.maximum = 1`
- `fs.s3a.retry.limit = 1`

## 4. Minimal 2.3.13 Layout

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

Notes:

- `disruptor` is required by the imap path
- `seatunnel-hadoop-aws` and `aws-java-sdk-bundle` are important for `s3/minio`
- JDBC drivers should stay outside the proxy jar and live under `plugins/Jdbc/lib/`

## 5. Startup

```bash
export SEATUNNEL_HOME=/tmp/st_proxy_home_2313
export STX_JAVA_PROXY_JAR=${SEATUNNEL_HOME}/tools/stx-java-proxy.jar
export JAVA_HOME=/path/to/jdk8-or-jdk11

sh ${SEATUNNEL_HOME}/bin/stx-java-proxy.sh \
  -Dstx.java.proxy.port=18080
```

The script automatically loads:

- `${SEATUNNEL_HOME}/starter/seatunnel-starter.jar`
- `${SEATUNNEL_HOME}/lib/*`
- `${SEATUNNEL_HOME}/connectors/*`
- every `.jar` under `${SEATUNNEL_HOME}/plugins/<ConnectorName>/`

### 5.1 Deploy into an existing cluster directory

If you want to deploy into an existing SeaTunnel installation instead of a temporary test home:

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

After deployment, verify at least:

- `${SEATUNNEL_HOME}/starter/seatunnel-starter.jar`
- `${SEATUNNEL_HOME}/connectors/connector-jdbc-<version>.jar`
- the JDBC driver exists somewhere under `${SEATUNNEL_HOME}/plugins/Jdbc/`
- S3 or OSS related Hadoop jars exist under `${SEATUNNEL_HOME}/lib/`
- `curl http://127.0.0.1:18080/healthz` returns `{"ok":true}`

## 6. HOCON-First Verification

### 6.1 Config DAG

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

### 6.2 Source preview derivation in HOCON

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

### 6.3 Transform preview derivation in HOCON

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

## 7. MySQL Catalog Probe

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

Focus on these response fields:

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
