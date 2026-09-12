# STX 改名设计

## 范围边界

本次改名只处理本项目拥有的名称。Apache SeaTunnel 是被管理的上游系统，它的产品名、环境变量、Java 包和协议名称继续使用原值。判断一个名称是否修改时，以“它是在指本管理平台，还是在指 Apache SeaTunnel”为准。

历史内容分成两类：当前会参与构建、运行或用户阅读的内容需要修改；`.trellis/tasks/archive`、`.trellis/workspace`、`openspec/changes/archive` 等历史记录保持原文。`NOTICE` 的当前产品标题修改，来源说明保持原文。

## 统一映射

| 类别 | 原值 | 新值 |
| --- | --- | --- |
| 产品展示名 | `SeaTunnelX`、`Seatunnel X` | `STX` |
| 技术标识 | `seatunnelx` | `stx` |
| Go 模块 | `github.com/seatunnel/seatunnelX` | `github.com/LeonYoah/stx` |
| Agent | `seatunnelx-agent` | `stx-agent` |
| Java Proxy | `seatunnelx-java-proxy` | `stx-java-proxy` |
| Java 类型前缀 | `SeatunnelXJavaProxy` | `StxJavaProxy` |
| 前端包 | `seatunnel-platform` | `stx-frontend` |
| 环境变量前缀 | `SEATUNNELX_` | `STX_` |
| 系统属性前缀 | `seatunnelx.` | `stx.` |
| 默认安装目录 | `/opt/seatunnelx` | `/opt/stx` |
| Agent 目录 | `*/seatunnelx-agent` | `*/stx-agent` |
| 应用状态目录 | `.seatunnelx` | `.stx` |
| 管理标记 | `.seatunnelx-managed` | `.stx-managed` |
| PM2 进程 | `seatunnelx-api/ui` | `stx-api/ui` |
| 应用数据文件 | `seatunnelx.db/log` | `stx.db/log` |
| 会话 Cookie | `seatunnel_session_id` | `stx_session_id` |
| 告警规则文件与组 | `seatunnel-managed-alert-policies` | `stx-managed-alert-policies` |
| Java Proxy API | `/seatunnelx-java-proxy` | `/stx-java-proxy` |
| Agent API | `/seatunnelx` | `/stx` |

大小写变体按所在语言的惯例修改，例如 Go 导出标识使用 `STX`，Java 类型使用 `Stx`，普通英文句子使用 `STX`。不含 `X` 但明确属于本项目的旧标识也要修改，例如测试 Feature 标签 `seatunnel-agent` / `seatunnel-platform-login` 和 `SEATUNNEL_PROXY_JAR`；不得借此修改 Apache SeaTunnel 自身的 API、包名或协议名。

## 实施方式

### Go 与 protobuf

先修改两个 `go.mod` 和 `.proto` 的 `go_package`，再统一修改 import。protobuf 生成文件使用项目已有生成方式更新；如果本机生成器不可用，必须说明原因，并确保两份生成文件中的路径与源码一致。Go 文件名中的旧技术标识同步重命名。

### Java Proxy

整个 Maven 工程目录改为 `tools/stx-java-proxy`，启动脚本改为 `scripts/stx-java-proxy.sh`。修改 Maven `artifactId`、主类及测试类名称，并将项目源码从 Apache 命名空间迁到 `io.github.leonyoah.stx.proxy`；SeaTunnel 依赖仍保留 `org.apache.seatunnel` 坐标。`service/` 再按 `catalog`、`config`、`plugin`、`storage`、`support` 五类组织，测试目录采用相同结构。

### 前端

修改 npm 包名、锁文件根包名、界面文案、国际化资源、服务常量及 E2E 固定值。用户界面展示 `STX`，机器可读值使用 `stx`。不改与 Apache SeaTunnel 集群和引擎有关的名称。

### CI、发布和运行脚本

所有工作流中的目录过滤必须跟随 Java Proxy 新目录。构建产物、缓存路径、Docker 镜像与 release 附件统一使用新名。安装包内的二进制、脚本、服务与目录保持一致，避免出现“构建名已改而启动脚本仍找旧文件”的情况。

### 文档与生成文件

先修改源码注解和当前文档，再运行 Swagger 生成脚本。普通文档按统一映射更新，但引用 Apache SeaTunnel 官方项目或历史来源时保留原名。图片中若仍含旧字样且没有可编辑源文件，在验收结果中单独列出。

### Trellis 规范

只保留项目实际使用的长期约定。初始化附带且从未加入项目专有内容的通用模板可以移除；包含真实问题记录的规范继续保留。各层 `index.md` 作为入口，提供适用范围、开发前检查、质量检查和有效文档链接。删除规范后同步修改当前任务的 JSONL 上下文，并验证所有相对链接。

## 风险控制

- 全局替换可能误伤 Apache SeaTunnel 名称，因此只替换明确属于本项目的完整标识，并在最后逐项检查剩余结果。
- 多个脚本共同约定二进制、目录和服务名，修改后通过发布包内容检查和 shell 语法检查验证。
- Go、Java、TypeScript 均有生成或锁定文件，修改源码后必须更新对应文件并运行各自检查。
- 并行修改按目录划分，公共任务文档与最终检查由主任务负责，参与者不得回退他人修改。

## 验证顺序

1. 搜索旧名称并按允许列表审查。
2. 运行格式化、静态检查和各语言测试。
3. 重新生成 protobuf 与 Swagger，并确认再次生成不会产生差异。
4. 执行前端生产构建、Java Maven 测试和 Go 双模块测试。
5. 检查 CI YAML、shell 脚本与发布包内文件名。
6. 检查 Trellis 规范链接、目录说明与任务上下文。
7. 查看最终 Git 差异，确认历史内容和 Apache SeaTunnel 标识未被误改。
