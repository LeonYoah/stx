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

# STX Java Proxy API

This document describes the current HTTP API exposed by `stx-java-proxy`.

## 1. Base Rules

- Base URL example: `http://127.0.0.1:18080`
- `GET /healthz` is the only `GET` endpoint
- all `/api/v1/**` endpoints accept `POST`
- request and response bodies are JSON
- config content inside the JSON body can be either `hocon` or `json`

If a `POST` endpoint is called with another HTTP method, the proxy returns:

```json
{
  "ok": false,
  "message": "Method not allowed"
}
```

## 2. Common Response Model

### Success

Successful responses return `HTTP 200` and a JSON body defined by the endpoint.

### Failure

All failures use this structure:

```json
{
  "ok": false,
  "message": "..."
}
```

Common status codes:

- `400`: invalid request, missing field, unsupported mode, invalid config
- `405`: method not allowed
- `500`: runtime failure, missing dependency, remote initialization failure
- `504`: probe timeout

## 3. Common Config Input Fields

`/api/v1/config/dag`, `/api/v1/config/preview/source`, and
`/api/v1/config/preview/transform` share the same config input rules.

Supported fields:

- `content`
  - inline SeaTunnel config text
- `contentFormat`
  - `hocon` or `json`
  - default: `hocon`
- `filePath`
  - absolute or reachable file path on the proxy host
- `variables`
  - array of `key=value`

Rules:

- exactly one of `content` or `filePath` must be provided
- if `contentFormat` is omitted, `hocon` is used
- invalid `variables` entries return `400`

Example:

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

## 4. Health

### `GET /healthz`

Response:

```json
{
  "ok": true
}
```

## 5. Config DAG

### `POST /api/v1/config/dag`

Parses the job config with SeaTunnel config logic and returns a lightweight config-level DAG.

#### Request

Uses the common config input fields.

#### Response

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

#### Response fields

- `simpleGraph`
  - `true` when the config is `1 source + 1 sink + optional 1 transform`
- `warnings`
  - compatibility hints for simple-graph dataset linking
- `graph.nodes[].nodeId`
  - stable proxy node id such as `source-0`, `transform-1`, `sink-0`
- `graph.nodes[].configIndex`
  - index inside the original `source` / `transform` / `sink` array

## 6. Source Preview Derivation

### `POST /api/v1/config/preview/source`

Derives a preview-only job config:

- keeps one selected source
- removes user sinks
- appends `Metadata`
- appends `Http`

#### Request

Uses the common config input fields plus:

- `sourceNodeId`
  - optional, example: `source-0`
- `sourceIndex`
  - optional numeric index
- `outputFormat`
  - `json` or `hocon`
  - default: `json`
- `metadataOutputDataset`
  - optional, default `__st_preview_rows`
- `metadataFields`
  - optional object
- `envOverrides`
  - optional object merged into root `env`
- `httpSink`
  - required object
  - must include `url`

Selection rules:

- if there is only one source, selection can be omitted
- if there are multiple sources, either `sourceNodeId` or `sourceIndex` must be provided
- if both are provided, they must refer to the same source

Default metadata fields:

```json
{
  "Database": "__st_debug_db",
  "Table": "__st_debug_table",
  "RowKind": "__st_debug_rowkind"
}
```

Minimal request:

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

#### Response

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

Key response fields:

- `content`
  - rendered preview config
- `contentFormat`
  - actual format of `content`
- `config`
  - structured config object
- `graph`
  - DAG of the derived preview config

## 7. Transform Preview Derivation

### `POST /api/v1/config/preview/transform`

Derives a preview-only config that keeps the upstream path of one selected transform and appends
`Metadata` and `Http` at the end.

#### Request

Uses the common config input fields plus:

- `transformNodeId`
  - optional, example: `transform-0`
- `transformIndex`
  - optional numeric index
- `outputFormat`
  - `json` or `hocon`
- `metadataOutputDataset`
  - optional
- `metadataFields`
  - optional object
- `envOverrides`
  - optional object merged into root `env`
- `httpSink`
  - required object with `url`

Selection rules:

- at least one transform must exist
- if there is only one transform, selection can be omitted
- if there are multiple transforms, either `transformNodeId` or `transformIndex` must be provided

Minimal request:

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

Response fields are the same as source preview, except:

- `mode = "transform_preview"`
- `selectedNodeId` points to the selected transform

## 8. Catalog Probe

### `POST /api/v1/catalog/probe`

Creates a SeaTunnel `Catalog` by `factoryIdentifier`, optionally lists databases and tables, and
optionally fetches a single table schema.

#### Request

- `factoryIdentifier`
  - required
  - examples: `MySQL`, `Oracle`, `Dameng`
- `options`
  - required object
  - passed to SeaTunnel catalog factory
- `catalogName`
  - optional
  - default: `stx_java_proxy_catalog`
- `pluginJars`
  - optional array of jar paths
- `includeDatabases`
  - optional boolean
  - default: `true`
- `databaseName`
  - optional
  - if provided, `tables` is returned
- `tablePath`
  - optional
  - either a string such as `db.schema.table`
  - or an object with `databaseName`, `schemaName`, `tableName`

Example:

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

#### Response

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

## 9. Checkpoint Storage Probe

### `POST /api/v1/storage/checkpoint/probe`

Creates a SeaTunnel checkpoint storage and optionally performs a write-read-delete cycle.

#### Request

- `plugin`
  - required
  - current value: `hdfs`
- `mode`
  - optional
  - `read_write` or `init_only`
  - default: `read_write`
- `probeTimeoutMs`
  - optional
  - default: `15000`
- `timeoutMs`
  - optional alias of `probeTimeoutMs`
- `pluginJars`
  - optional array of jar paths
- `config`
  - required object

Remote-only policy:

- local modes are rejected
- `storage.type` must not be `local` or `localfile`

Required `config` fields:

- `storage.type`
- `namespace`
- `s3.bucket` when `storage.type = s3`

Example:

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

Default fast-fail values for `s3` are injected when absent:

- `fs.s3a.connection.establish.timeout = 5000`
- `fs.s3a.connection.timeout = 10000`
- `fs.s3a.attempts.maximum = 1`
- `fs.s3a.retry.limit = 1`

#### Response

`init_only`:

```json
{
  "ok": true,
  "plugin": "hdfs",
  "mode": "init_only",
  "storageType": "s3",
  "initialized": true
}
```

`read_write`:

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

## 10. IMap Storage Probe

### `POST /api/v1/storage/imap/probe`

Creates a SeaTunnel imap storage and optionally performs a write-read-delete cycle.

#### Request

- `plugin`
  - optional
  - default: `hdfs`
- `mode`
  - optional
  - `read_write` or `init_only`
  - default: `read_write`
- `probeTimeoutMs`
  - optional
  - default: `15000`
- `timeoutMs`
  - optional alias of `probeTimeoutMs`
- `deleteAllOnDestroy`
  - optional boolean
  - default: `false`
- `pluginJars`
  - optional array of jar paths
- `config`
  - required object

Required `config` fields:

- `storage.type`
- `namespace`
- `businessName`
- `clusterName`
- `s3.bucket` when `storage.type = s3`

Remote-only policy:

- local modes are rejected
- `storage.type` must not be `local` or `localfile`

The proxy also injects:

- the same `s3` fast-fail defaults as checkpoint probe
- `writeDataTimeoutMilliseconds`
  - defaults to `max(1000, probeTimeoutMs)`

Example:

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

#### Response

`init_only`:

```json
{
  "ok": true,
  "plugin": "hdfs",
  "mode": "init_only",
  "storageType": "s3",
  "initialized": true
}
```

`read_write`:

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
