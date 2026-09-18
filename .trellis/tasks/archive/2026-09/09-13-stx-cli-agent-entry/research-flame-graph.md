# 火焰图采集方案研究

## 状态

- 2026-09-15：已决定不纳入 STX CLI 首版。本文件只保留前期调查结果，后续重新讨论火焰图时再确定具体实现和发布方式。

## 研究日期

2026-09-14

## 结论

- JDK 8 和 JDK 11 都没有内置的火焰图生成器。JFR 负责记录 JVM 事件，火焰图是对采样数据的另一种展示，两者不能当成同一种产物。
- JDK 11 内置 JFR，可以通过 `jcmd` 控制录制，并通过 `jfr` 命令或 JFR API 读取 `.jfr` 文件；这些内置能力不会直接生成交互式 HTML 或 SVG 火焰图。
- 不能假定所有 JDK 8 都能使用 JFR。Oracle JDK 8 的旧版文档把 JFR 记为商业功能，8u40 前还要求在 JVM 启动时传入启用参数；OpenJDK 8 后来移植了 JFR，但各供应商采用的更新时间不同。
- STX 面向 JDK 8 和 JDK 11 时，不能只看主版本号。每次执行前都应查询目标 JVM 的实际命令，并检查工具、附加权限、操作系统和 CPU 架构。

## 仓库现状

- 当前代码没有 JFR、async-profiler 或火焰图实现。
- Agent protobuf 没有性能采样命令，目前只有日志、线程栈和 JVM Dump 等诊断命令。
- STX 安装预检查把 Java 8 作为最低支持版本，并推荐 Java 8 或 Java 11。
- Agent 已经能够定位受管 SeaTunnel 进程并在节点上生成诊断文件，可以沿用这部分目标识别和产物登记方式。

## 官方资料

- Oracle JDK 8 JFR 说明：<https://docs.oracle.com/javase/8/docs/technotes/guides/troubleshoot/tooldescr003.html>
- Oracle JDK 8 工具更新说明：<https://docs.oracle.com/javase/8/docs/technotes/tools/enhancements-8.html>
- Oracle JDK 8 `jcmd` 文档：<https://docs.oracle.com/javase/8/docs/technotes/tools/unix/jcmd.html>
- Oracle JDK 11 `jdk.jfr` API：<https://docs.oracle.com/en/java/javase/11/docs/api/jdk.jfr/jdk/jfr/package-summary.html>
- Oracle JDK 11 诊断工具说明：<https://docs.oracle.com/en/java/javase/11/troubleshoot/diagnostic-tools.html>
- Oracle JDK 11 `jcmd` 文档：<https://docs.oracle.com/en/java/javase/11/tools/jcmd.html>
- async-profiler 项目：<https://github.com/async-profiler/async-profiler>
- async-profiler 输出格式：<https://github.com/async-profiler/async-profiler/blob/master/docs/OutputFormats.md>
- async-profiler 接入说明：<https://github.com/async-profiler/async-profiler/blob/master/docs/IntegratingAsyncProfiler.md>

## JDK 8 与 JDK 11 的能力

### JDK 11

- JFR 属于 JDK 的 `jdk.jfr` 模块，目标 JVM 支持时可通过 `jcmd <pid> JFR.start`、`JFR.check`、`JFR.dump` 和 `JFR.stop` 控制录制。
- JFR 原始产物是 `.jfr`。JDK 11 的 `jfr` 命令可以打印和汇总事件，JDK Mission Control 可以查看录制内容，但仍不是 STX 所需的可下载 HTML 火焰图。
- 如果 STX 要把 JFR 数据转成火焰图，仍需引入并管理转换工具。

### JDK 8

- Oracle JDK 8 提供过 JFR，但旧版生产使用受商业许可约束。8u40 以前需要在启动 JVM 时传入 `-XX:+UnlockCommercialFeatures` 和 `-XX:+FlightRecorder`；8u40 开始可以在运行时通过 `jcmd` 或 JMC 启用。
- OpenJDK 8 的 JFR 是后续移植能力，生产节点所用供应商和更新版本未必相同，不能把“Java 8”直接等同于“JFR 可用”。
- STX 应以 `jcmd <pid> help JFR.start` 的实际结果为准；命令不存在、不能启用或许可条件不明确时，不执行 JFR 录制。

## 外部采样工具

### async-profiler

- async-profiler 面向 OpenJDK 及其他基于 HotSpot 的 JVM，可以采集 CPU、分配、锁、Wall Clock、原生栈和内核栈等信息，并直接生成交互式 HTML 火焰图或 JFR 文件。
- 官方当前提供 Linux x64、Linux arm64 和 macOS 构建。项目仍包含 JDK 8 的兼容处理，但 STX 必须锁定并测试允许使用的版本，不能自动追随最新版。
- 它需要与操作系统和 CPU 架构匹配的本地文件，并具备附加到目标 JVM 所需的权限；部分 CPU 与内核事件还受 Linux `perf_event` 权限限制。
- Agent 不把 async-profiler 编进自身二进制。STX 将它作为独立诊断工具管理，由管理员预置或手工导入，校验版本、平台、架构和文件摘要；Agent 不自行联网下载。

## 运行文件与环境依赖

### 仅放入 `async-profiler.jar` 不够

- GitHub Release 中单独发布的 `async-profiler.jar` 只包含 Java API 和 Java Agent 类，不包含 `libasyncProfiler.so`。它只有约 10 KiB，不能独自完成采样。
- Maven Central 的无分类器 JAR 会同时放入 Linux x64、Linux arm64 和 macOS 的 native 库，平台分类 JAR 则只放入对应平台的 native 库。这类 JAR 可以在目标 Java 进程内提取并加载 native 库，但需要在启动时使用 `-javaagent`，或另写一个基于 Attach API 的装载程序。
- STX 的目标是按需附加到已经运行的 SeaTunnel JVM，不应要求修改 SeaTunnel 启动参数，也不应为了装载一个 JAR 再维护 JDK 8 和 JDK 11 两套 Attach API 启动方式。因此不采用“只内置一个 JAR”的路径。

### STX 首版真正需要的文件

- `bin/asprof`：外部命令入口，当前版本已经包含 jattach 和容器辅助逻辑。
- `lib/libasyncProfiler.so`：实际加载进目标 JVM 的 native profiler，必须与操作系统和 CPU 架构匹配。
- `LICENSE`、版本和文件摘要：用于许可证说明、版本识别和完整性校验。
- `bin/jfrconv` 或 `jfr-converter.jar` 不是直接生成 HTML 火焰图的必需文件。只有后续需要把已有 `.jfr` 转成其他展示格式时才需要分发。

STX 可以直接调用：

```text
asprof -e cpu -d <seconds> -f <output.html> <pid>
```

使用官方预编译文件时，运行节点不需要 GCC、Clang、Make 或 Maven。目标机器仍需满足：

- 运行的是受支持的 HotSpot 系 JVM，首版验证 JDK 8 和 JDK 11。
- Agent 对目标 JVM 有附加权限，通常要求同一系统用户，或者 Agent 具备受控的特权；目标 JVM 不能启用 `-XX:+DisableAttachMechanism`。
- `asprof` 所在文件系统允许执行，native 库和结果目录可读写，结果路径受 STX 限制。
- Linux CPU 采样受 `perf_event_open`、`kernel.perf_event_paranoid`、`kernel.kptr_restrict`、容器 seccomp 和能力配置影响。权限不足时可以使用经过验证的 `ctimer` 模式，但结果会缺少部分内核信息。
- 如果 SeaTunnel 位于容器内，Agent 需要使用宿主机 PID，并确保目标容器能通过相同绝对路径读取 native 库，或使用 `--libpath` 指定容器内路径。

## 与 STX 当前发布方式的关系

- STX 当前为 Agent 提供 Linux amd64、Linux arm64、macOS amd64 和 macOS arm64 二进制，正式发布包主要面向 Linux amd64 和 Linux arm64。
- 当前 Agent 安装接口只下载一个 Agent 二进制。采用 async-profiler 后，需要把安装单元改成“Agent 二进制 + 对应平台的诊断工具目录”，或者让 Agent 从 STX 控制端按版本下载并安装工具目录。
- 当前 Linux systemd 安装脚本以 root 用户运行 Agent，通常具备附加到受管 SeaTunnel 进程的系统权限。不过仍必须只允许服务端登记并验证过的 SeaTunnel PID，不能让调用方传入任意 PID；容器和系统安全策略仍可能拒绝附加。
- 建议把 async-profiler 作为 Agent 旁边的可替换工具目录，而不是使用 Go `embed` 放入 Agent 二进制。这样可以单独升级或停用 profiler，也不会让所有 Agent 更新都携带新的第三方 native 文件。

## 后续候选方案

- 保留诊断资源 `flame-graph`，但它的含义固定为“生成可直接查看的火焰图”，不能只返回 `.jfr` 后把操作标记为成功。
- 为保证 JDK 8 和 JDK 11 上的命令语义一致，建议 `flame-graph` 使用经过 STX 验证并管理的 async-profiler。首版先支持 CPU 采样，后续再根据生产验证增加 Wall Clock、分配和锁采样。
- JFR 如需提供，应登记为另一个候选资源 `jfr-recording`。它只负责生成和下载 `.jfr`，执行前查询目标 JVM 是否支持，不承诺生成火焰图。
- `flame-graph` 执行前检查目标 JVM、async-profiler 版本、平台、架构、附加权限和系统限制。能力不足时返回明确原因，不改用 JFR，也不临时下载工具。
- STX 发布并校验每个平台的 `asprof` 与 `libasyncProfiler.so`，Agent 只调用登记过的参数模板。直接输出 HTML 时不分发转换器 JAR。
- 产物至少包含采样元数据 JSON、交互式 HTML 火焰图和可用时的原始采样文件，并进入诊断执行记录与一键诊断包。
- 采样方式、类型、持续时间、间隔、目标 PID、工具版本、产物大小和校验值写入审计，不记录采样文件正文。

## 取舍

- 统一使用 async-profiler 会增加一个本地原生工具的分发、验证和版本维护工作，但可以让 JDK 8 与 JDK 11 的 `flame-graph` 返回相同类型的结果。
- 以 JFR 为首选可以减少部分 JDK 11 节点上的外部采样依赖，但 JDK 8 节点上的可用性不稳定，并且仍要维护一个火焰图转换器。
- 把 `flame-graph` 和 `jfr-recording` 分成两个资源，会多一个资源编码和权限规则，但名称、产物和失败原因更清楚，也不会让 AI 把 `.jfr` 文件误认为火焰图。
