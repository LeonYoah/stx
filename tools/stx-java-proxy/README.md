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

# STX Java Proxy

`stx-java-proxy` is a lightweight sidecar service for platform integration. It does
not fork SeaTunnel and it does not run inside the SeaTunnel master or worker JVM. Instead, it
reuses SeaTunnel jars and SPI from an existing SeaTunnel installation.

## 1. Scope

The proxy currently provides:

- config-level DAG parsing via SeaTunnel `ConfigParserUtil`
- catalog probing via SeaTunnel `Catalog` SPI
- remote checkpoint storage probing via SeaTunnel `CheckpointStorageFactory`
- remote imap storage probing via SeaTunnel `IMapStorageFactory`
- source preview config derivation
- transform preview config derivation

The proxy intentionally defaults to config-level DAG parsing. It does not generate the full
runtime execution DAG by default.

Detailed documents:

- [API.md](./API.md)
- [TESTING.md](./TESTING.md)

## 2. Project Layout

- Maven coordinates: `io.github.leonyoah.stx:stx-java-proxy`
- application entry point: `src/main/java/io/github/leonyoah/stx/proxy/StxJavaProxyApplication.java`
- data models: `src/main/java/io/github/leonyoah/stx/proxy/model/`
- service implementations: `src/main/java/io/github/leonyoah/stx/proxy/service/`, grouped by function:
  - `catalog/`: Catalog metadata probes
  - `config/`: config parsing, validation, previews, and save-mode planning
  - `plugin/`: plugin loading, option descriptors, templates, and enum values
  - `storage/`: checkpoint, IMAP, and runtime-storage operations
  - `support/`: request parsing, timeout execution, and shared exceptions
- tests: `src/test/java/io/github/leonyoah/stx/proxy/`
- source-tree wrapper: `bin/stx-java-proxy.sh`, which delegates to the shared root script

`org.apache.seatunnel` is reserved for SeaTunnel dependencies and APIs. STX-owned source code lives under `io.github.leonyoah.stx`.

## 3. Recommended Deployment

The recommended deployment model is:

- deploy the proxy on a node that already has SeaTunnel installed
- run it as a separate JVM process
- reuse `${SEATUNNEL_HOME}/starter` and `${SEATUNNEL_HOME}/lib`

This matches how SeaTunnel distribution itself is assembled:

- starter jars are placed under `starter/`
- shared dependencies are placed under `lib/`
- startup scripts use `lib/*:${APP_JAR}` as classpath

References:

- [assembly-bin.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-dist/src/main/assembly/assembly-bin.xml#L151)
- [assembly-bin.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-dist/src/main/assembly/assembly-bin.xml#L170)
- [seatunnel.sh](/Users/mac/Documents/projects/seatunnel/seatunnel-core/seatunnel-starter/src/main/bin/seatunnel.sh#L94)
- [seatunnel-cluster.sh](/Users/mac/Documents/projects/seatunnel/seatunnel-core/seatunnel-starter/src/main/bin/seatunnel-cluster.sh#L189)

### Why this deployment model

- no duplicate heavyweight packaging
- can reuse cluster-provided `seatunnel-starter.jar`
- can reuse existing shared jars in `${SEATUNNEL_HOME}/lib`
- can reuse connector and plugin dependency conventions already used by SeaTunnel

## 4. JDK Recommendation and Compatibility

Recommended runtimes:

- `JDK 8`
- `JDK 11`

The proxy directly reuses SeaTunnel Hadoop, catalog, checkpoint, and imap implementations, so
older ecosystem dependencies are noticeably more reliable on LTS JDKs than on very new runtimes.

Locally verified combinations:

- `SeaTunnel 2.3.13 + JDK 8 + MySQL catalog probe`
- `SeaTunnel 2.3.13 + JDK 8 + MinIO(S3) checkpoint probe`
- `SeaTunnel 2.3.13 + JDK 8 + MinIO(S3) imap probe`

On `JDK 26`, the imap probe may fail with:

- `getSubject is not supported`

That comes from the Hadoop compatibility layer rather than proxy-specific business logic.

Recommended prechecks before integration:

- create remote buckets such as MinIO buckets ahead of time
- place JDBC drivers under `${SEATUNNEL_HOME}/plugins/<ConnectorName>/`
- keep the default `probeTimeoutMs = 15000` unless you have a clear reason to change it

## 5. Packaging

Default build:

```bash
./mvnw -f tools/stx-java-proxy/pom.xml spotless:apply
./mvnw -f tools/stx-java-proxy/pom.xml test
./mvnw -f tools/stx-java-proxy/pom.xml -DskipTests verify
```

Default output is a thin jar:

- `target/stx-java-proxy-2.3.13-2.12.15.jar`

This jar is intentionally small and expects SeaTunnel runtime jars from `${SEATUNNEL_HOME}`.

Optional standalone packaging:

```bash
./mvnw -f tools/stx-java-proxy/pom.xml -Pstandalone-bin -DskipTests verify
```

This attaches an additional `-bin.jar`, but it still excludes remote-storage heavyweight jars such
as:

- `checkpoint-storage-hdfs`
- `imap-storage-file`
- `seatunnel-hadoop3-3.1.4-uber`
- `seatunnel-hazelcast-shade`

Those dependencies are expected to come from the installed SeaTunnel cluster, or from explicit
`pluginJars` in API requests.

## 6. Startup

A helper script is provided at:

- [stx-java-proxy.sh](./bin/stx-java-proxy.sh)

Typical deployment:

1. Copy the thin proxy jar to the installed SeaTunnel home, for example:
   - `${SEATUNNEL_HOME}/tools/stx-java-proxy.jar`
2. Copy the startup script to:
   - `${SEATUNNEL_HOME}/bin/stx-java-proxy.sh`
3. Start the proxy with the installed SeaTunnel runtime:

```bash
export SEATUNNEL_HOME=/opt/seatunnel
export STX_JAVA_PROXY_JAR=${SEATUNNEL_HOME}/tools/stx-java-proxy.jar
export JAVA_HOME=/path/to/jdk8-or-jdk11

sh ${SEATUNNEL_HOME}/bin/stx-java-proxy.sh \
  -Dstx.java.proxy.port=18080
```

The script builds classpath from:

- `${SEATUNNEL_HOME}/lib/*`
- `${SEATUNNEL_HOME}/starter/seatunnel-starter.jar`
- `${STX_JAVA_PROXY_JAR}`
- `${SEATUNNEL_HOME}/connectors/*` if present
- every `.jar` under `${SEATUNNEL_HOME}/plugins/<ConnectorName>/` if present

If extra jars are needed at process startup, use:

- `EXTRA_PROXY_CLASSPATH=/path/a.jar:/path/b.jar`

### Cluster Directory Deployment Checklist

If you deploy the proxy into an existing SeaTunnel cluster directory, use this checklist:

1. Verify JDK
   - prefer `JDK 8` or `JDK 11`
2. Verify base directories
   - `${SEATUNNEL_HOME}/starter/seatunnel-starter.jar`
   - `${SEATUNNEL_HOME}/lib/`
   - `${SEATUNNEL_HOME}/connectors/`
   - `${SEATUNNEL_HOME}/plugins/`
3. Place proxy files
   - `${SEATUNNEL_HOME}/tools/stx-java-proxy.jar`
   - `${SEATUNNEL_HOME}/bin/stx-java-proxy.sh`
4. Verify connector jars
   - for JDBC, ensure `${SEATUNNEL_HOME}/connectors/connector-jdbc-<version>.jar`
5. Verify third-party drivers
   - MySQL drivers are still recommended under `${SEATUNNEL_HOME}/plugins/Jdbc/lib/`
   - if jars already live directly under `${SEATUNNEL_HOME}/plugins/Jdbc/` or deeper subdirectories, the proxy now loads them as well
6. Verify remote storage dependencies
   - for S3 or MinIO, confirm Hadoop/S3A/AWS jars exist under `${SEATUNNEL_HOME}/lib/`
   - for OSS, confirm Aliyun-related jars are available under `${SEATUNNEL_HOME}/lib/` or injected through `EXTRA_PROXY_CLASSPATH`
7. Start the proxy
   - `sh ${SEATUNNEL_HOME}/bin/stx-java-proxy.sh -Dstx.java.proxy.port=18080`
8. Run smoke checks
   - `GET /healthz`
   - a minimal HOCON request to `POST /api/v1/config/dag`
   - then `catalog/probe` or `checkpoint/probe` as needed

Deploy the proxy on the node that is best positioned to reach external systems or submit jobs, and keep it as a separate JVM process rather than embedding it into the SeaTunnel master or worker JVM.

## 7. Remote Storage Dependencies

The proxy only supports remote storage probing in V1.

### Checkpoint probe

Supported plugin:

- `plugin = "hdfs"`

Supported remote `storage.type`:

- `hdfs`
- `s3`
- `oss`
- `cos`

`storage.type = local` and `localfile` are rejected on purpose.

### IMap probe

Supported plugin:

- `plugin = "hdfs"`

Supported remote `storage.type`:

- `hdfs`
- `s3`
- `oss`

Local mode is rejected on purpose.

### OSS note

OSS usually needs extra jars that are not bundled by this proxy. That is intentional.

Follow the same dependency strategy as SeaTunnel itself: place required jars into
`${SEATUNNEL_HOME}/lib/`, or pass them through `pluginJars`.

Relevant SeaTunnel docs already follow this convention:

- [engine-jar-storage-mode.md](/Users/mac/Documents/projects/seatunnel/docs/en/engines/zeta/engine-jar-storage-mode.md#L28)
- [OssFile.md](/Users/mac/Documents/projects/seatunnel/docs/en/connectors/source/OssFile.md#L22)

## 8. API

Implemented endpoints:

- `GET /healthz`
- `POST /api/v1/config/dag`
- `POST /api/v1/config/preview/source`
- `POST /api/v1/config/preview/transform`
- `POST /api/v1/catalog/probe`
- `POST /api/v1/storage/checkpoint/probe`
- `POST /api/v1/storage/imap/probe`

### Config input

The proxy accepts:

- `content` with `contentFormat = hocon`
- `content` with `contentFormat = json`
- `filePath`

If `contentFormat` is omitted, `hocon` is assumed.

### Preview config output

Preview derivation endpoints return:

- `content`
- `contentFormat`
- `config`
- `graph`

Supported preview output formats:

- default `json`
- `outputFormat = "hocon"`

The proxy keeps JSON as the default because it is safer for platform-side round-trip handling, but
HOCON output is supported when callers need SeaTunnel-native text configuration.

## 9. Verification

Verified locally for this module:

```bash
./mvnw -f tools/stx-java-proxy/pom.xml spotless:apply
./mvnw -f tools/stx-java-proxy/pom.xml test
./mvnw -f tools/stx-java-proxy/pom.xml -DskipTests verify
```

Repository root `./mvnw -q -DskipTests verify` still fails because of existing unrelated compile
errors outside this module.
