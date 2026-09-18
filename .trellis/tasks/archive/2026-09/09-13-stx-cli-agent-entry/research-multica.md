# Multica CLI 与 API 研究

研究日期：2026-09-13

## 结论

Multica 与 STX 的目标接近，但二进制组织并不完全相同：

- `multica` 是一个 Go/Cobra 二进制，既是远端 API 客户端，也能启动和管理本机 Agent daemon。
- 中心 API Server 使用另一个 Go 入口并构建为 `server` 二进制，不是 `multica server` 子命令。
- CLI 与 Web 主要复用同一组资源 API；CLI 没有单独复制一套业务接口。
- Multica 的普通 App API 仍以 `/api/...` 为主，后来增加的 Public API 使用 `/v1`，并建立了更严格的协议文件和测试。

STX 当前只需要一个控制端 API 进程，调度与后台维护都在该进程内运行。仓库中的 `scheduler`、`worker` 已是遗留兼容壳。STX 可以让控制端二进制同时提供 Server 与远端 CLI，但 `stx-agent` 继续独立发布，不需要照搬 Multica 的中心 Server/CLI 双二进制结构，也不需要把 Agent 合入控制端程序。

## 二进制与命令结构

Multica 的构建脚本分别构建：

- `./cmd/server` → `bin/server`
- `./cmd/multica` → `bin/multica`

`multica` 根命令使用 Cobra，业务命令按资源分组，例如：

- `multica issue list|get|create|update`
- `multica project ...`
- `multica agent ...`
- `multica attachment download|upload`
- `multica daemon start|stop|status|logs`
- `multica auth status|logout`
- `multica login`、`multica setup`、`multica config`

参考：

- [CLI 入口](https://github.com/multica-ai/multica/blob/main/server/cmd/multica/main.go)
- [中心 Server 入口](https://github.com/multica-ai/multica/blob/main/server/cmd/server/main.go)
- [构建目标](https://github.com/multica-ai/multica/blob/main/Makefile)
- [CLI 使用文档](https://github.com/multica-ai/multica/blob/main/apps/docs/content/docs/cli.mdx)

Multica 的 CLI 命令是手工编写的 Cobra 命令，每个命令显式调用一个 REST 路径；没有从 OpenAPI 自动生成全部命令。公共 HTTP 客户端统一封装 GET、POST、PUT、PATCH、DELETE、上传和下载。

## 地址、配置与输出

Multica 的全局参数包括 `--server-url`、`--workspace-id`、`--profile` 和 `--debug`。它支持多份 profile，用于分别连接生产、测试或自托管环境。

配置优先级是：

1. 命令参数
2. 环境变量
3. profile 配置文件
4. 程序默认值

默认配置位于 `~/.multica/config.json`，其他 profile 位于 `~/.multica/profiles/<name>/config.json`。配置文件采用临时文件加原子重命名保存，并设置为 `0600`。

列表命令通常默认表格，读取单项和写入命令通常默认 JSON；文档明确要求脚本和 Agent 使用 `--output json`。错误会按网络、认证、无权限、不存在、请求非法和服务端故障分类，并映射到稳定退出码；原始响应只在 `--debug` 下显示。

参考：

- [CLI 配置](https://github.com/multica-ai/multica/blob/main/server/internal/cli/config.go)
- [HTTP 客户端](https://github.com/multica-ai/multica/blob/main/server/internal/cli/client.go)
- [错误与退出码](https://github.com/multica-ai/multica/blob/main/server/internal/cli/errors.go)
- [输出工具](https://github.com/multica-ai/multica/blob/main/server/internal/cli/output.go)

## 认证与客户端标识

Multica 的同一组 App API 支持两类常用认证：

- Web/Desktop 使用 HttpOnly Cookie 中的 JWT；写请求还要通过 CSRF 校验。
- CLI、daemon、脚本和直接 API 调用使用 `Authorization: Bearer <PAT>`。

CLI 浏览器登录先获得 JWT，再调用 `POST /api/tokens` 创建默认 90 天的个人令牌。无浏览器环境可以在 Web 中创建令牌，再通过交互提示交给 CLI。服务端只保存令牌哈希、前缀、名称、过期时间和最近使用时间。

每个 CLI 请求还会发送：

- `X-Client-Platform: cli`
- `X-Client-Version`
- `X-Client-OS`
- `X-Client-Capabilities`
- 有工作区时发送 `X-Workspace-ID`

服务端明确把客户端标识视为可伪造信息，只用于日志、指标和兼容判断，不用于认证或提高权限。这与 STX 已确认的 Header 用法一致。

参考：

- [认证与令牌文档](https://multica.ai/docs/auth-tokens)
- [认证中间件](https://github.com/multica-ai/multica/blob/main/server/internal/middleware/auth.go)
- [客户端标识中间件](https://github.com/multica-ai/multica/blob/main/server/internal/middleware/client.go)

## Agent 使用临时身份

Multica 不把用户个人令牌直接交给受管 Agent。daemon 领取任务后，服务端生成 `mat_` 临时令牌，绑定用户、工作区、Agent 和任务，最长有效 24 小时。服务端从令牌记录恢复 Agent 与任务身份，并删除客户端伪造的 `X-Agent-ID`、`X-Task-ID` 和行为来源 Header。

受管 Agent 运行时，CLI 禁止退回读取用户 profile 中的个人令牌；部分只允许真人执行的接口也会拒绝任务令牌。这一点对 STX 的生产操作和审计有直接参考价值。

## API 分区

Multica 按调用者和信任范围划分路由：

- 普通 App API：`/api/...`
- daemon 协议：`/api/daemon/...`，使用单独认证中间件和 daemon 令牌
- Plugin Bridge：`/api/plugin-bridge/v1/...`
- Public Plugin API：独立域名下的 `/v1/...`
- Webhook、OAuth 回调和公开下载入口：在认证路由组之外单独登记

不同入口可以使用不同凭据和限流规则，但最终调用同一 Go service/handler，不通过内部 HTTP 再调用一次业务 API。

## Public API v1 协议

Multica 后来新增的 Public API v1 将以下内容定义为协议来源：

- `openapi.yaml`：OpenAPI 3.1 请求与响应协议
- `routes.go`：服务端操作登记表
- `types.go`：不依赖数据库模型和 App API 返回结构的 DTO
- `problem.go`：统一错误格式
- `foundation.go`：身份、凭据、分页、幂等、版本修订、风险、审计和限流的公共约定

操作登记表为每个接口记录：

- HTTP 方法和路径
- 接口类型
- 可用凭据类型
- 权限 scope
- 风险等级
- 审计状态
- 限流方案

测试会比较操作登记表与 OpenAPI，确保路径、方法、接口类型和 scope 一致。OpenAPI 中每个接口有稳定 `operationId`，并使用扩展字段记录接口类型和 scope。

公共协议还规定：

- 列表采用不透明 cursor，默认 50 条，最多 200 条，并返回 `next_cursor`。
- create、trigger、replay、retry 等操作应持久保存 `Idempotency-Key` 的执行结果；只接收 Header 不算完成幂等处理。
- 可修改资源提供 revision 和 ETag；条件更新使用 `If-Match`，版本过期返回 `revision_conflict`。
- 错误使用 `application/problem+json`，包括稳定 `code`、可读 `detail`、HTTP 状态和 `request_id`。
- v1 只允许增加字段和接口；破坏兼容性的变化需要新主版本和迁移时间。

参考：

- [Public API v1 说明](https://github.com/multica-ai/multica/blob/main/server/pkg/publicapi/v1/README.md)
- [操作登记表](https://github.com/multica-ai/multica/blob/main/server/pkg/publicapi/v1/routes.go)
- [公共约定](https://github.com/multica-ai/multica/blob/main/server/pkg/publicapi/v1/foundation.go)
- [OpenAPI 3.1](https://github.com/multica-ai/multica/blob/main/server/pkg/publicapi/v1/openapi.yaml)
- [协议一致性测试](https://github.com/multica-ai/multica/blob/main/server/pkg/publicapi/v1/contract_test.go)
- [统一错误格式](https://github.com/multica-ai/multica/blob/main/server/pkg/publicapi/v1/problem.go)

## 文件资源

Multica 的附件下载分两步：

1. 通过已认证的元数据接口读取文件名、大小和 `download_url`。
2. 下载地址是相对路径时继续携带 API 认证；是对象存储绝对签名地址时不携带 API 令牌。

这与 STX 首版资源“元数据查询 + 文件下载”很接近。不过 Multica 当前 CLI 会把整个附件读入内存，并限制为 100 MB，不适合直接照搬到 SeaTunnel 源码包、诊断包和 JVM Dump。STX CLI 应直接流式写文件，支持进度、临时文件、校验和失败续传。

参考：

- [附件命令](https://github.com/multica-ai/multica/blob/main/server/cmd/multica/cmd_attachment.go)
- [文件下载客户端](https://github.com/multica-ai/multica/blob/main/server/internal/cli/client.go)

## 对 STX 的建议

### 可以采用

- 同一套资源 API 同时服务 Web 与 CLI，认证方式不同，不复制 `/cli/...` 业务接口。
- Bearer 令牌与客户端标识 Header 分离；Header 只用于审计和兼容判断。
- 支持多个 profile，允许一台机器分别连接多个 STX 环境。
- CLI 命令按业务资源组织，显式映射稳定 `operation_id`。
- 建立服务端操作登记表，并用测试保证路由、OpenAPI、风险、权限和审计说明一致。
- 使用统一 JSON 错误结构、请求编号和稳定退出码。
- 大文件先查元数据，再下载到本地；不要读入内存。
- 为受管 AI 会话签发短期、范围受限的令牌，不把人的长期令牌交给 Agent。

### 不应直接照搬

- Multica 的 App API 与 CLI 是长期逐项手工增加的，官方 Skill 也明确说明 CLI 尚未覆盖每个页面能力。STX 已确定首版覆盖全部现有接口，需要自动检查遗漏。
- Multica 对部分有影响的写操作主要依赖 Agent Skill 要求先征得用户同意。STX 面向生产 SeaTunnel，需要保留已经确定的服务端 R2/R3 确认编号，不能只依赖客户端提示。
- Multica 的附件下载会一次读入内存。STX 的源码包、日志归档和 Dump 更大，需要流式下载、断点续传和校验。
