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

`stx-java-proxy` 是一个面向平台集成的轻量 sidecar 服务。它不需要二开
SeaTunnel，也不建议嵌进 SeaTunnel Master/Worker 进程内部，而是复用已经安装好的
SeaTunnel 运行时 jar 和 SPI。

## 1. 当前能力

当前版本支持：

- 基于 SeaTunnel `ConfigParserUtil` 的配置级 DAG 解析
- 基于 SeaTunnel `Catalog` SPI 的元数据探测
- 基于 SeaTunnel `CheckpointStorageFactory` 的远端 checkpoint 存储探测
- 基于 SeaTunnel `IMapStorageFactory` 的远端 imap 存储探测
- `source preview` 配置派生
- `transform preview` 配置派生

默认只做配置级 DAG，不默认进入完整执行规划链路。

联调与测试流程见：

- [TESTING_CN.md](./TESTING_CN.md)

接口文档见：

- [API_CN.md](./API_CN.md)

## 2. 项目结构

- Maven 坐标：`io.github.leonyoah.stx:stx-java-proxy`
- 应用入口：`src/main/java/io/github/leonyoah/stx/proxy/StxJavaProxyApplication.java`
- 数据模型：`src/main/java/io/github/leonyoah/stx/proxy/model/`
- 服务实现：`src/main/java/io/github/leonyoah/stx/proxy/service/`，按功能分为：
  - `catalog/`：Catalog 元数据探测
  - `config/`：配置解析、校验、预览和保存模式计算
  - `plugin/`：插件加载、选项描述、模板与枚举值
  - `storage/`：checkpoint、IMAP 和运行时存储操作
  - `support/`：请求读取、超时执行和公共异常
- 测试代码：`src/test/java/io/github/leonyoah/stx/proxy/`
- 本地包装脚本：`bin/stx-java-proxy.sh`，实际调用仓库根目录的统一脚本

`org.apache.seatunnel` 只用于 SeaTunnel 依赖及其 API；STX 自有源码统一放在 `io.github.leonyoah.stx` 下。

## 3. 推荐部署方式

最推荐的方式是：

- 部署在已经安装好 SeaTunnel 的节点上
- 作为独立 JVM 进程运行
- 直接复用 `${SEATUNNEL_HOME}/starter` 和 `${SEATUNNEL_HOME}/lib`

这和 SeaTunnel 官方 dist 的结构是一致的：

- `starter/` 放 starter jar
- `lib/` 放共享依赖
- `plugins/` 放连接器 单独依赖
- 启动脚本按 `lib/*:${APP_JAR}` 组装 classpath

参考：

- [assembly-bin.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-dist/src/main/assembly/assembly-bin.xml#L151)
- [assembly-bin.xml](/Users/mac/Documents/projects/seatunnel/seatunnel-dist/src/main/assembly/assembly-bin.xml#L170)
- [seatunnel.sh](/Users/mac/Documents/projects/seatunnel/seatunnel-core/seatunnel-starter/src/main/bin/seatunnel.sh#L94)
- [seatunnel-cluster.sh](/Users/mac/Documents/projects/seatunnel/seatunnel-core/seatunnel-starter/src/main/bin/seatunnel-cluster.sh#L189)

这样做的好处：

- 不重复打包一份很重的运行时
- 直接复用现有 `seatunnel-starter.jar`
- 直接复用 `${SEATUNNEL_HOME}/lib` 中已有依赖
- 和 SeaTunnel 现有 connector / plugin 依赖模型一致

## 4. JDK 与兼容性建议

建议使用：

- `JDK 8`
- `JDK 11`

原因很简单：proxy 会直连 SeaTunnel 现有的 Hadoop、Catalog、checkpoint、imap 相关实现，
这些依赖在较新的 JDK 上更容易遇到兼容性问题。

当前本地已完成的实测：

- `SeaTunnel 2.3.13 + JDK 8 + MySQL Catalog probe`：通过
- `SeaTunnel 2.3.13 + JDK 8 + MinIO(S3) checkpoint probe`：通过
- `SeaTunnel 2.3.13 + JDK 8 + MinIO(S3) imap probe`：通过

在 `JDK 26` 下，`imap probe` 可能报：

- `getSubject is not supported`

这不是 proxy 自己的业务逻辑问题，而是旧 Hadoop 路径与高版本 JDK 的兼容性问题。

同时建议在联调前做好远端资源预置，例如：

- MinIO 的 bucket 先创建好
- JDBC 驱动先放到 `${SEATUNNEL_HOME}/plugins/<ConnectorName>/`
- 远端 probe 默认带 `probeTimeoutMs = 15000` 的总超时保护

## 5. 打包方式

模块校验命令：

```bash
./mvnw -f tools/stx-java-proxy/pom.xml spotless:apply
./mvnw -f tools/stx-java-proxy/pom.xml test
./mvnw -f tools/stx-java-proxy/pom.xml -DskipTests verify
```

默认产物是薄 jar：

- `target/stx-java-proxy-2.3.13-2.12.15.jar`

这个 jar 很小，运行时依赖来自 `${SEATUNNEL_HOME}`。

如果确实要生成可执行 `bin` 包，可以用：

```bash
./mvnw -f tools/stx-java-proxy/pom.xml -Pstandalone-bin -DskipTests verify
```

但即使是 `-bin.jar`，也仍然不会把这些远端存储重依赖打进去：

- `checkpoint-storage-hdfs`
- `imap-storage-file`
- `seatunnel-hadoop3-3.1.4-uber`
- `seatunnel-hazelcast-shade`

这些依赖应该来自：

- 已安装好的 `${SEATUNNEL_HOME}/lib`
- 或请求里的 `pluginJars`

## 6. 启动方式

已经提供了辅助启动脚本：

- [stx-java-proxy.sh](./bin/stx-java-proxy.sh)

推荐部署步骤：

1. 把薄 jar 复制到已安装的 SeaTunnel 目录，例如：
   - `${SEATUNNEL_HOME}/tools/stx-java-proxy.jar`
2. 把启动脚本复制到：
   - `${SEATUNNEL_HOME}/bin/stx-java-proxy.sh`
3. 通过已安装 SeaTunnel 的运行时启动：

```bash
export SEATUNNEL_HOME=/opt/seatunnel
export STX_JAVA_PROXY_JAR=${SEATUNNEL_HOME}/tools/stx-java-proxy.jar
export JAVA_HOME=/path/to/jdk8-or-jdk11

sh ${SEATUNNEL_HOME}/bin/stx-java-proxy.sh \
  -Dstx.java.proxy.port=18080
```

脚本会自动复用：

- `${SEATUNNEL_HOME}/lib/*`
- `${SEATUNNEL_HOME}/starter/seatunnel-starter.jar`
- `${STX_JAVA_PROXY_JAR}`
- `${SEATUNNEL_HOME}/connectors/*`，如果目录存在
- `${SEATUNNEL_HOME}/plugins/<ConnectorName>/` 下所有 `.jar`，如果目录存在

如果启动时还要额外挂其他 jar，可以设置：

- `EXTRA_PROXY_CLASSPATH=/path/a.jar:/path/b.jar`

### JVM 内存与参数配置

`stx-java-proxy` 作为轻量级中继进程，默认强制限制最大堆内存上限为 **`512MB`**（默认参数 `-Xms64m -Xmx512m`），避免占用过多宿主机内存或触发 OOM Killer。

支持以下 4 种方式按需调整与修改 proxy 内存参数：

1. **环境变量覆盖（推荐）**：
   ```bash
   export STX_JAVA_PROXY_JVM_OPTS="-Xms128m -Xmx1024m -XX:+UseG1GC"
   sh bin/stx-java-proxy.sh
   ```
2. **命令行参数直接传参**：
   ```bash
   # 直接传入 -Xmx / -Xms 等 JVM 参数（脚本自动识别并注入 JVM，不污染应用参数）
   sh bin/stx-java-proxy.sh -Xmx1024m -Xms256m
   ```
3. **配置文件持久化配置**：
   复制模板并配置 `stx-java-proxy-env.sh`：
   ```bash
   cp conf/stx-java-proxy-env.sh.template conf/stx-java-proxy-env.sh
   echo 'export STX_JAVA_PROXY_JVM_OPTS="-Xms128m -Xmx1024m"' >> conf/stx-java-proxy-env.sh
   ```
   脚本启动时会依次自动探测并加载以下路径的配置文件：
   - `${STX_JAVA_PROXY_CONF_DIR}/stx-java-proxy-env.sh`
   - `${PROXY_HOME}/conf/stx-java-proxy-env.sh`
   - `${PROXY_HOME}/config/stx-java-proxy-env.sh`
   - `${SEATUNNEL_HOME}/config/stx-java-proxy-env.sh`
4. **快捷系统属性指定**：
   ```bash
   sh bin/stx-java-proxy.sh -Dstx.java.proxy.xmx=1024m
   ```
通过 `GET /healthz` 接口可即时观测当前生效的 JVM 内存指标（包括 `maxMemoryMb`、`totalMemoryBytes` 等）。

### 集群目录部署清单

如果你要把 proxy 部署到一套已经安装好的 SeaTunnel 集群目录，建议按这个顺序核对：

1. 核对 JDK
   - 建议节点使用 `JDK 8` 或 `JDK 11`
2. 核对基础目录
   - `${SEATUNNEL_HOME}/starter/seatunnel-starter.jar`
   - `${SEATUNNEL_HOME}/lib/`
   - `${SEATUNNEL_HOME}/connectors/`
   - `${SEATUNNEL_HOME}/plugins/`
3. 放置 proxy 文件
   - `${SEATUNNEL_HOME}/tools/stx-java-proxy.jar`
   - `${SEATUNNEL_HOME}/bin/stx-java-proxy.sh`
4. 核对 connector 主 jar
   - 例如 JDBC 需要 `${SEATUNNEL_HOME}/connectors/connector-jdbc-<version>.jar`
5. 核对第三方驱动
   - 例如 MySQL 驱动建议放 `${SEATUNNEL_HOME}/plugins/connector-jdbc`,或更深层子目录，proxy 现在也会一并加载
6. 核对远端存储依赖
   - S3 / MinIO 至少要确认 `${SEATUNNEL_HOME}/lib/` 下有 Hadoop/S3A/AWS 相关 jar
   - OSS 需要额外确认阿里云相关 jar 是否已放到 `${SEATUNNEL_HOME}/lib/` 或通过 `EXTRA_PROXY_CLASSPATH` 挂入
7. 启动 proxy
   - `sh ${SEATUNNEL_HOME}/bin/stx-java-proxy.sh -Dstx.java.proxy.port=18080`
8. 做最小自检
   - `GET /healthz`
   - 用一份最小 HOCON 调 `POST /api/v1/config/dag`
   - 再按需做 `catalog/probe` 或 `checkpoint/probe`

建议把 proxy 部署在“负责提交任务或最容易访问外部数据源”的节点上，并作为独立 JVM 进程运行，不要直接嵌进 SeaTunnel Master/Worker 进程。

## 7. 远端存储依赖说明

当前版本只支持远端存储探测。

### Checkpoint probe

支持：

- `plugin = "hdfs"`

支持的远端 `storage.type`：

- `hdfs`
- `s3`
- `oss`
- `cos`

会明确拒绝：

- `storage.type = local`
- `storage.type = localfile`

### IMap probe

支持：

- `plugin = "hdfs"`

支持的远端 `storage.type`：

- `hdfs`
- `s3`
- `oss`

本地模式同样会明确拒绝。

### OSS 说明

OSS 通常需要额外 jar，这些 jar 不会被 proxy 自己打进包里，这是刻意的。

建议沿用 SeaTunnel 自身的依赖管理方式：

- 放到 `${SEATUNNEL_HOME}/lib`
- 或通过 `pluginJars` 显式传入

相关文档：

- [engine-jar-storage-mode.md](/Users/mac/Documents/projects/seatunnel/docs/zh/engines/zeta/engine-jar-storage-mode.md#L28)
- [OssFile.md](/Users/mac/Documents/projects/seatunnel/docs/zh/connectors/sink/OssFile.md#L22)

## 8. 接口

当前已实现接口：

- `GET /healthz`
- `POST /api/v1/config/dag`
- `POST /api/v1/config/preview/source`
- `POST /api/v1/config/preview/transform`
- `POST /api/v1/catalog/probe`
- `POST /api/v1/storage/checkpoint/probe`
- `POST /api/v1/storage/imap/probe`

### 配置输入

支持：

- `content + contentFormat = hocon`
- `content + contentFormat = json`
- `filePath`

如果不传 `contentFormat`，默认按 `hocon` 解析。

### Preview 输出

preview 派生接口会返回：

- `content`
- `contentFormat`
- `config`
- `graph`

支持的 preview 输出格式：

- 默认 `json`
- `outputFormat = "hocon"`

当前默认仍然是 JSON，因为它更适合平台侧做稳定 round-trip；如果你们需要直接拿 SeaTunnel
原生文本配置，也可以显式指定 HOCON 输出。

## 9. 本地校验

当前模块已本地验证：

```bash
./mvnw -f tools/stx-java-proxy/pom.xml spotless:apply
./mvnw -f tools/stx-java-proxy/pom.xml test
./mvnw -f tools/stx-java-proxy/pom.xml -DskipTests verify
```

仓库根目录的 `./mvnw -q -DskipTests verify` 仍然会失败，但失败点是仓库里已有的其它模块编译问题，
不是这个 proxy 模块引入的。
