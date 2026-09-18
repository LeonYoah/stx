# 项目改名为 STX

## 目标

将仓库中由本项目拥有的 `SeaTunnelX` / `seatunnelx` 名称完整改为 `STX` / `stx`，覆盖源码、运行时标识、目录与文件名、构建产物、安装脚本、文档以及 CI。项目尚未对外部署，不提供旧名称兼容，也不增加迁移逻辑。

## 命名规则

- 产品展示名使用 `STX`。
- 文件名、命令、服务名、镜像名、包名、URL 段和其他技术标识使用 `stx`。
- Go 主模块使用 `github.com/LeonYoah/stx`，Agent 子模块使用 `github.com/LeonYoah/stx/agent`。
- Java 类型按 Java 命名习惯使用 `StxJavaProxy*`，构件和启动脚本使用 `stx-java-proxy`。
- Java Proxy 使用 STX 自有的 Maven 坐标和 Java 包 `io.github.leonyoah.stx.proxy`，不再放在 Apache SeaTunnel 的包名下。
- Java Proxy 的 `service/` 目录按 Catalog、配置、插件、存储和公共支持分为二级功能包，测试代码采用相同目录结构。
- 本项目环境变量使用 `STX_*`，系统属性使用 `stx.*`，内部服务值使用 `stx_*`。
- 安装路径使用 `/opt/stx`；Agent 配置、日志和库目录分别使用 `/etc/stx-agent`、`/var/log/stx-agent`、`/usr/local/lib/stx-agent`。
- 应用数据库、日志、会话 Cookie、状态目录和管理标记使用 `stx`，例如 `stx.db`、`stx.log`、`stx_session_id`、`.stx`、`.stx-managed`。

## 要求

- 修改根模块、Agent 模块以及仓库内全部 Go import，保证两个模块都只引用新的模块路径。
- 修改后端、Agent、Java Proxy、前端中的产品文案、代码标识、配置默认值、API 路径、服务标识和测试数据。
- 修改不含 `X` 但明确属于本项目的旧标识，例如 `seatunnel-agent`、`seatunnel-platform-login`、`SEATUNNEL_PROXY_JAR` 和 `seatunnel-managed-alert-policies`；不得修改同形的 Apache SeaTunnel 上游标识。
- 重命名带旧项目名的源码、脚本、Java 类型和 Java Proxy 目录，并同步所有调用处。
- 修改二进制、发布压缩包、Docker 镜像、Java 构件、PM2 进程及 systemd 服务名称。
- 修改安装、重启、诊断、可观测性及发布脚本中的路径、环境变量和文件名。
- 修改 `.github/workflows` 中的路径过滤、构建命令、产物名、镜像名和发布说明。
- 修改前端包元数据、页面文案、国际化资源、测试、GitHub 链接及静态资源中的可见旧名称。
- 修改当前生效的 README、开发文档、OpenSpec 规范和 Trellis 规范；保留归档任务、工作记录及归档 OpenSpec 的历史原文。
- 保留并纳入当前工作区中已有的规范调整，包括精简过时的 gRPC TLS 规范入口和 TypeScript 缓存说明，以及补充注释和 protobuf 生成约定。
- 检查当前 Trellis 规范；移除没有项目专有内容的重复模板，修正失效入口，并让目录与测试说明反映现有代码。
- 重新生成受模块路径或注解影响的 protobuf 与 Swagger 文件，不手工保留过时生成内容。
- 保留 Apache SeaTunnel 上游项目本身的名称和接口，包括 `SeaTunnel`、`SEATUNNEL_HOME`、`SEATUNNEL_VERSION`、`org.apache.seatunnel`、`seatunnel-engine-*` 和 protobuf wire package `seatunnel.agent.v1`。
- 保留 `NOTICE` 中说明项目来源的历史名称及原始项目地址；该文件当前产品标题改为 `STX`。
- 不修改被 Git 忽略的本机 `config.yaml`，只修改仓库跟踪的示例及测试配置。

## 不在本次范围内

- 旧二进制、旧环境变量、旧 API 路径、旧目录、旧数据文件或旧服务名的兼容。
- 已部署环境的数据迁移与进程迁移。
- GitHub 仓库远端改名、外部镜像仓库创建、域名变更和 Logo 重做。
- 归档记录中的历史文字改写。

## 验收条件

- [ ] 当前源码、配置、CI、脚本和有效文档中不再出现由本项目拥有的 `SeaTunnelX` / `seatunnelx` 标识；允许项仅限明确保留的历史归档和 `NOTICE` 来源说明。
- [ ] 根 Go 模块和 Agent 子模块能够格式化、编译并通过测试，生成代码中的 Go 包路径为新模块路径。
- [ ] 前端依赖锁文件与包名一致，类型检查、单元测试和生产构建通过。
- [ ] Java Proxy 在新目录下能够通过测试并生成 `stx-java-proxy-*.jar`。
- [ ] Swagger 能重新生成，生成文件不含旧项目模块路径或旧产品名。
- [ ] CI 工作流只使用新路径、新产物名和新镜像名，YAML 语法检查通过。
- [ ] 发布打包脚本生成 `stx-<version>-...` 包，包内使用 `stx`、`stx-agent` 和 `stx-java-proxy`。
- [ ] Git 状态可清楚显示必要的文件重命名，没有误改归档历史或 Apache SeaTunnel 上游标识。
- [ ] Trellis 规范索引无失效链接，任务上下文不再引用已删除规范，目录说明与当前代码一致。

## 说明

- 本次是直接替换，不设置弃用期。
- 仓库当前远端所有者是 `LeonYoah`，因此新 Go 模块使用 `github.com/LeonYoah/stx`；GitHub 仓库本身需在代码合并前后由维护者另行改名。
