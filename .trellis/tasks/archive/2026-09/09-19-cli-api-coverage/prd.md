# 本地异步验证与现有 API CLI 覆盖

## 目标

先使用本机正在运行的 STX、Agent 和测试集群验证 CLI 的真实异步任务行为，再开始把现有 STX API 接入 `stx` CLI。

本任务只完成第一批只读接口和可复用的普通 GET 命令构建器，不在同一批次处理写操作、下载、流式输出或诊断资源新增。

## 已确认事实

- 本地 STX HTTP 地址为 `http://127.0.0.1:17800`，gRPC 端口为 `17890`。
- 本地 Agent 和 Java Proxy 正在运行，测试集群 ID 为 `6`，SeaTunnel 版本为 `2.3.13`。
- 当前 CLI 已有登录、退出、身份查询、健康检查、能力查询和公共执行查询、等待、取消命令。
- 当前操作登记包含 8 个操作；仓库内 API 路由为 246 条，Swagger 操作为 114 个，历史未登记缺口为 219 条。
- `internal/cli/command` 尚不存在，普通业务 API 尚未通过登记项批量构建 CLI 命令。
- 用户已确认继续原 PRD，并要求先完成本地真实异步任务验证，然后开始现有 API 的 CLI 覆盖。

## 本地真实异步任务验证结果

### 取消路径

- 创建普通诊断任务，关闭线程栈和 JVM Dump。
- 公共执行 ID：`4c3a5c36-eb80-43f9-80b7-5201b922b3d1`。
- 初始状态为 `pending`。
- 执行 `stx execution cancel --confirm` 后服务端返回 `cancelled`。
- 执行 `stx execution wait` 后确认最终状态为 `cancelled`。
- 任务尚未实际启动，因此服务端能够立即确认停止。

### 成功路径

- 创建 `auto_start: true` 的普通诊断任务。
- 公共执行 ID：`83679faf-8033-43a9-8e69-62d13a33e9b3`。
- 执行 `stx execution wait` 的退出码为 0，最终状态为 `succeeded`。
- 执行 `stx execution get` 返回 `succeeded`，进度为 100。

### 基础 CLI 验证

- `stx login --password-stdin`、`health`、`whoami`、`capability list` 和 `logout` 已连接真实服务验证。
- CLI 配置文件权限为 `0600`，登录 stdout 不包含令牌。
- 不存在的 execution 返回 `not_found`，退出码为 5。
- 退出登录后执行 `whoami` 返回认证错误，退出码为 3。

## 本批需求

### 通用 GET 命令构建器

- 新增 `internal/cli/command` 包，从本地 `operation.OperationSpec` 构建 Cobra 命令。
- 命令帮助必须来自本地登记项，离线执行 `--help` 时不得访问远端服务。
- 第一版只接受 `ModeNormal` 且 HTTP 方法为 GET 的操作。
- path 输入按登记顺序映射为位置参数，并进行 URL 转义。
- query 输入映射为同名命令参数；未传入的可选参数不得加入请求。
- 每个普通命令支持 `--namespace`，输出继续使用已有全局 `--output`、`--format`、`-f` 和 `--pick`。
- 请求业务接口前查询服务端能力，检查操作是否存在、操作修订号是否兼容、当前用户是否允许执行。
- 请求使用现有 CLI HTTP 客户端和统一结果格式，不复制登录、能力查询、execution 等专用命令的实现。

### 第一批只读命令

登记并接入以下操作：

| operation_id | CLI | API |
| --- | --- | --- |
| `host.list` | `stx host list` | `GET /api/v1/hosts` |
| `host.get` | `stx host get <id>` | `GET /api/v1/hosts/:id` |
| `cluster.list` | `stx cluster list` | `GET /api/v1/clusters` |
| `cluster.get` | `stx cluster get <id>` | `GET /api/v1/clusters/:id` |
| `cluster.node.list` | `stx cluster node list <id>` | `GET /api/v1/clusters/:id/nodes` |
| `cluster.status.get` | `stx cluster status get <id>` | `GET /api/v1/clusters/:id/status` |
| `config.cluster.list` | `stx config cluster list <id>` | `GET /api/v1/clusters/:id/configs` |
| `config.get` | `stx config get <id>` | `GET /api/v1/configs/:id` |
| `config.version.list` | `stx config version list <id>` | `GET /api/v1/configs/:id/versions` |

命令名称使用现有 operation ID 的层次，集群 ID 和配置 ID 的位置参数名称在 help 中明确显示。

## 验收标准

- [x] 本地真实异步任务的取消路径和成功路径均完成验证，并记录执行 ID 与终态。
- [x] `internal/cli/command` 可以根据登记项构建普通 GET 命令，且有单元测试覆盖 path、query、能力检查、输出和错误情况。
- [x] 第一批 9 个只读操作加入操作登记，并通过登记表校验。
- [x] `stx --help` 能显示 `host`、`cluster` 和 `config` 命令组；各级 `--help` 不访问网络。
- [x] 命令在请求业务接口前完成服务端能力和权限检查。
- [x] `--namespace`、全局输出格式和 `--pick` 在新命令中可用。
- [x] 路由覆盖报告中的历史缺口减少 9 条，且未新增未说明缺口。
- [x] Go 单元测试、操作契约检查和构建通过。
- [x] 已重新构建 `dist/stx`，并连接本地 `127.0.0.1:17800` 对第一批命令进行真实调用。

## 不在本批范围

- POST、PUT、PATCH、DELETE 的通用命令构建和二次确认。
- 下载、watch、代理和 server-only 路由。
- 诊断资源、安装包源码下载和调试工作台功能变更。
- 把全部 219 个历史缺口一次性接入 CLI。
- 修改现有 API 的响应结构。

## 约束

- 不修改或暂存当前工作区中与本任务无关的发布脚本、工作流和打包文档改动。
- 不使用 `git restore`；需要回撤时先由用户确认。
- 新增或修改的代码注释使用中英双语，中文在前。
- 新增 Go 文件带 Apache 2.0 许可证头。

## 后续安全项

- 当前配置详情、集群配置列表和配置版本接口会返回完整 `content`。本轮 CLI 真实验证使用 `--pick` 排除了正文，但普通 API 和 CLI 仍可取得原内容。
- 在生产环境允许 AI Agent 调用配置接口前，需要在服务端复用统一敏感字段处理，对 SeaTunnel、HOCON、YAML、properties 和 JVM 参数中的密码、令牌、密钥做屏蔽；不能依赖客户端主动 `--pick`。
- 该项涉及配置查看、编辑时掩码回填和版本历史，单独安排任务，不放进本轮只读 CLI 接入。
