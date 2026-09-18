# STX CLI AI Agent 入口技术方案

## 文档状态

- 日期：2026-09-17
- 阶段：技术方案，尚未开始实现
- 需求来源：`prd.md`
- 重点：先完成可被 AI Agent 稳定调用的 STX CLI，再补齐它依赖的服务端公共能力

## 1. 方案目标

首版提供一个同时支持服务端和远端客户端的 `stx` Go 二进制：

```text
stx server
stx login
stx cluster list
stx diagnostics resource run thread-dump ...
stx execution wait <execution-id>
```

AI Agent 不需要登录 SeaTunnel 节点，也不需要解析网页。它通过 STX API 获取日志、指标、任务记录、部署信息、配置、JAR 清单、源码包和诊断产物，并通过受控命令执行测试任务或诊断操作。

首版坚持以下约束：

- 不提供任意 Shell、SSH 或任意节点文件路径访问。
- 普通命令默认输出版本化 JSON。
- stdout 只写最终业务结果，stderr 只写一行一个 JSON 事件。
- 每条实际 HTTP 路由都必须登记为 CLI 操作或明确例外。
- 风险确认、幂等、资源版本、异步执行和审计在服务端生效，不能只依赖 CLI 自觉遵守。
- `stx-agent` 保持独立轻量，不加入控制端 CLI 或 API Server 代码。

## 2. 当前代码基础

仓库已经使用 Cobra 和 Viper，可以继续沿用：

- 根入口：`main.go`
- Cobra 命令：`internal/cmd/`
- API 服务：`internal/router/router.go`
- 功能模块：`internal/apps/<name>/`
- Agent 通信：`internal/apps/agent/`、`internal/grpc/`、`internal/proto/agent/`

当前根命令存在两个需要先修正的问题：

1. 根命令名仍是 `linux-do-cdk`，需要改为 `stx`。
2. 根命令 `PreRun` 会执行数据库迁移，导致任何客户端命令都可能初始化本地数据库。迁移必须移到 `stx server` 的启动路径。

`stx scheduler` 和 `stx worker` 不再作为公开命令。旧的 `stx api` 只保留一个版本的隐藏兼容别名，实际行为等同于 `stx server`。

## 3. 总体结构

```text
┌──────────────────────────────────────────────────────────────┐
│ stx CLI                                                      │
│ Cobra 命令 → 本地配置 → 操作定义 → HTTP 客户端 → 输出渲染   │
└──────────────────────────────┬───────────────────────────────┘
                               │ HTTPS / Bearer Token
┌──────────────────────────────▼───────────────────────────────┐
│ STX API Server                                               │
│ 认证 → 操作识别 → 权限/风险/幂等/版本检查 → 现有业务 Handler │
│            → 公共执行记录 → 审计                             │
└──────────────────────────────┬───────────────────────────────┘
                               │ 受控 gRPC 命令 / 文件分块
┌──────────────────────────────▼───────────────────────────────┐
│ stx-agent                                                    │
│ 注册命令、参数校验、超时、取消、产物保存和分块读取           │
└──────────────────────────────────────────────────────────────┘
```

### 3.1 建议的代码边界

在现有目录规范内新增以下职责：

```text
internal/
├── cli/                       # 只在本地客户端运行
│   ├── client/                # HTTP、Header、重试、流式下载
│   ├── config/                # 多命名空间配置与环境变量
│   ├── output/                # JSON/table/yaml/raw 与 stderr 事件
│   └── command/               # 通用操作命令构建器
├── operation/                 # 客户端和服务端共用的操作登记
└── apps/
    ├── auth/                  # 扩充 CLI 令牌
    ├── execution/             # 新增公共执行记录
    └── audit/                 # 扩充请求和执行审计
```

具体文件名可在实现阶段按现有包大小调整，但职责不能混在 `internal/cmd` 或 `internal/router/router.go` 中。`internal/cmd` 只负责组装 Cobra 命令和启动对应流程。

## 4. 命令结构

根命令无参数时显示帮助并返回成功，不启动服务，也不访问数据库。

```text
stx
├── server
├── login
├── logout
├── whoami
├── namespace
│   ├── list
│   ├── show
│   ├── use
│   └── delete
├── capability
│   ├── list
│   └── get
├── execution
│   ├── get
│   ├── wait
│   └── cancel
├── cluster ...
├── host ...
├── package ...
├── plugin ...
├── monitor ...
├── monitoring ...
├── diagnostics
│   ├── resource
│   │   ├── list
│   │   ├── get
│   │   └── run
│   ├── bundle ...
│   └── artifact
│       ├── get
│       └── download
├── sync ...
├── upgrade ...
├── audit
│   ├── list
│   └── get
└── skill
    ├── show
    ├── status
    ├── install
    ├── update
    ├── backup list
    └── restore
```

命令名以业务含义为准，不机械照搬 URL。普通 CRUD 和查询命令使用公共构建器，SSE、文件下载、登录、Skill 等命令使用专用实现。

首版不提供以下入口：

- 任意 URL、HTTP 方法和请求体转发命令。
- `stx shell` 或远程命令执行。
- `stx audit export`。
- `stx scheduler`、`stx worker`。

## 5. 操作登记表

### 5.1 目的

操作登记表是 CLI 命令、服务端能力查询、风险规则、帮助信息和路由覆盖检查共同使用的清单。它不替换现有业务 Handler，也不引入新的接口描述语言，首版使用普通 Go 结构体和手工登记。

建议的核心字段：

```go
type OperationSpec struct {
    ID              string
    CommandPath     []string
    Method          string
    Route           string
    Mode            OperationMode
    AuthRequired    bool
    Risk            RiskLevel
    AdminOnly       bool
    Revision        int
    UsesAgent       bool
    Async           bool
    SupportsPick    bool
    SupportsRevision bool
    Impact          *ImpactSpec
    Input           []InputSpec
    Example         string
    OutputExample   string
}
```

`OperationMode` 首版只需要：

- `normal`：普通 JSON 请求。
- `watch`：SSE 或持续状态读取。
- `download`：文件流式下载。
- `server_only`：Webhook、OAuth 回调等只供服务端接收的接口。
- `proxy`：WebUI、Grafana 等代理入口，不生成任意转发命令。

### 5.2 稳定编号

`operation_id` 使用稳定的业务名称，例如：

```text
cluster.list
cluster.stop
diagnostics.resource.execute
diagnostics.artifact.download
sync.task.run
package.source.upload
```

URL 或 Handler 改名时，只要业务语义没有改变，`operation_id` 就保持不变。协议发生不兼容变化时增加 `revision`，而不是创建难以追踪的新命令别名。

### 5.3 命令生成方式

CLI 命令树在编译时由本地登记表构建，不能依赖远端服务在线才能显示帮助。执行远端命令前读取 `/api/v1/capabilities`，确认：

- 服务端是否支持该 `operation_id`。
- 服务端操作修订号是否兼容。
- 当前用户是否具备所需权限。
- 当前服务端给出的风险和影响提示。

登录、本地命名空间、Skill 和帮助命令不依赖能力接口。

### 5.4 路由覆盖

当前约有 239 条 `/api/v1` 路由注册，Swagger 只有 107 个操作。首版增加覆盖检查：

- 每条路由必须有 `operation_id`，或进入明确的例外清单。
- 例外项必须说明是 `server_only`、`proxy`、`watch` 还是 `download`。
- 普通业务接口缺少操作登记时 CI 失败。
- 普通业务接口缺少 Swagger 注解或生成结果过期时 CI 失败。

首版可以使用 Go AST 或测试期路由清单完成检查，不要求立即重写现有 Gin 路由注册方式。

## 6. 本地配置和登录

### 6.1 配置文件

默认配置文件：

```text
~/.config/stx/config.yaml
```

目录权限为 `0700`，文件权限为 `0600`。配置支持多个命名空间：

```yaml
current_namespace: production
namespaces:
  production:
    server: https://stx.example.com
    token: "..."
    token_expires_at: "2026-09-24T12:00:00+08:00"
    output: json
```

读取优先级：

1. 命令参数。
2. 环境变量。
3. 当前命名空间配置。
4. 内置默认值。

首版支持的环境变量至少包括：

```text
STX_NAMESPACE
STX_SERVER
STX_TOKEN
STX_OUTPUT
STX_TIMEOUT
```

### 6.2 登录

`stx login` 使用用户名和密码向远端 STX 换取随机 CLI 令牌：

- TTY 中隐藏输入密码。
- 非 TTY 使用 `--password-stdin`，不提供会出现在进程参数中的明文 `--password`。
- 令牌默认有效 7 天，服务端限制最长 30 天。
- 服务端只保存令牌哈希和元数据。
- 令牌只证明用户身份，权限始终取用户当前权限。

后续请求固定携带：

```text
Authorization: Bearer <token>
X-STX-Client: cli
User-Agent: stx-cli/<version>
```

令牌原文不能进入日志或审计。`logout` 优先在服务端撤销当前令牌，随后删除本地副本。

## 7. 普通请求流程

```text
命令参数
  ↓
解析当前命名空间和环境变量
  ↓
查找 OperationSpec
  ↓
查询服务端 capabilities
  ↓
构建 path/query/body/file 请求
  ↓
附加认证、请求编号、幂等键、资源版本和确认信息
  ↓
调用现有业务 API
  ↓
将现有 {data,error_msg} 响应转换为 CLI 统一结果
  ↓
执行脱敏后的 --pick 和格式渲染
  ↓
stdout 最终结果 / stderr 状态事件
```

CLI 不重新实现服务端业务规则。参数语义、权限、风险、脱敏和状态变更都以服务端为准。

## 8. 输出和错误

### 8.1 默认结果

普通命令默认输出 JSON，不因 TTY 改变：

```json
{
  "api_version": "v1",
  "operation_id": "cluster.list",
  "request_id": "req_example",
  "data": [],
  "result_meta": {
    "complete": true
  }
}
```

`result_meta` 只包含：

- `complete`
- 可选 `reason`
- 可选 `next_command`

分页数量等信息放在具体命令的 `data` 中。

### 8.2 输出选项

```text
--output json|table|yaml|raw
--format json|table|yaml|raw
-f json|table|yaml|raw
--pick field1,field2
```

- `raw` 只跳过 CLI 的外层转换，不能绕过服务端脱敏、权限和大小限制。
- `--pick` 只处理 `data` 顶层字段。
- 字段不存在时返回未裁剪的安全结果，并在 stderr 写 `pick_fallback`。

### 8.3 stderr 事件

stderr 每行一个 JSON 对象，例如：

```json
{"event":"progress","execution_id":"exec_1","progress":35}
{"event":"impact","level":"high","message":"可能暂停目标 JVM"}
{"event":"pick_fallback","missing_fields":["foo"]}
```

失败时 stdout 为空，stderr 最后一行是结构化错误：

```json
{
  "event": "error",
  "code": "revision_conflict",
  "message": "资源已经变化，请重新查询",
  "retryable": false,
  "request_id": "req_example"
}
```

### 8.4 退出码

首版使用稳定分类，不直接把 HTTP 状态码当进程退出码：

| 退出码 | 含义 |
|---|---|
| 0 | 成功，结果可能通过 `complete:false` 表示仍有后续读取 |
| 2 | CLI 用法或输入错误 |
| 3 | 未登录、令牌无效或过期 |
| 4 | 无权限 |
| 5 | 资源不存在 |
| 6 | 冲突、确认缺失、资源版本冲突或幂等冲突 |
| 7 | 网络错误或服务不可用 |
| 8 | 等待或请求超时 |
| 9 | 服务端内部错误 |
| 10 | 异步执行失败或取消 |
| 11 | 文件传输或校验失败 |

## 9. 风险确认、幂等和资源版本

### 9.1 请求 Header

公共请求可使用：

```text
Idempotency-Key
If-Match
X-STX-Confirm
X-STX-Confirmation-ID
X-Request-ID
```

### 9.2 风险处理

- R0：直接执行。
- R1：需要 `--confirm`，CLI 发送 `X-STX-Confirm: true`。
- R2：服务端先返回影响说明和一次性确认编号，第二次请求携带 `--confirmation-id`。
- R3：与 R2 相同，同时要求管理员权限。

TTY 可以显示影响说明并询问用户，确认后在同一进程内自动重试。非 TTY 不提问，AI Agent 必须读取 `next_action` 后显式提交确认编号。

确认编号不写入可复制的 `next_command`。`next_action` 单独返回确认编号、有效期、绑定的幂等键和所需参数。

### 9.3 幂等

R1、R2、R3 请求都带幂等键：

- CLI 默认生成 UUID。
- `--idempotency-key` 允许脚本显式指定。
- 网络重试和 R2/R3 两次请求必须复用同一键。
- 相同键和相同请求复用原执行或结果。
- 相同键和不同请求返回 `idempotency_conflict`。

### 9.4 资源版本

可修改资源返回 `revision` 和 `ETag`。CLI 使用 `--revision` 发送 `If-Match`。资源已经变化时服务端返回 `revision_conflict`，不执行目标操作。

## 10. 公共异步执行

各模块保留自己的任务表和详细状态，新增公共执行记录：

```text
stx execution get <execution-id>
stx execution wait <execution-id>
stx execution cancel <execution-id>
```

公共状态：

```text
pending
running
cancel_requested
cancelling
cancelled
succeeded
failed
timed_out
```

核心字段：

```text
execution_id
operation_id
owner_user_id
actor_type
status
cancellable
cancellable_reason
progress
module_ref
result_ref
created_at / started_at / finished_at
```

执行归用户所有，与令牌、命名空间和客户端机器无关。普通用户只能管理自己的执行，管理员按权限管理全部执行。

`wait` 可以使用长轮询或模块事件流，进度写 stderr，完成后的最终结果写 stdout。取消只有在实际执行停止并得到确认后才能进入 `cancelled`。

## 11. 诊断资源和产物

诊断中心提供资源登记，单项执行和诊断包共用同一执行器：

```text
stx diagnostics resource list
stx diagnostics resource get <code>
stx diagnostics resource run <code> ...
```

资源登记至少包含：

```text
code
parameters
risk
impact
timeout
execution_location
result_type
bundle_allowed
```

### 11.1 JVM 诊断

- 自动策略默认不执行线程快照、类直方图和 Heap Dump，但配置时可以勾选。
- 自动策略中的 Heap Dump 只有管理员可以启用。
- 同一 JVM 同时只能运行一个 JVM 诊断命令，不排队。
- Heap Dump 同一 JVM 十分钟内最多实际开始一次。
- Heap Dump 开始后不可取消。

### 11.2 文本结果

- 脱敏后不超过 1 MiB：直接返回完整正文。
- 超过 1 MiB：Agent 保存完整脱敏文件，响应返回最多 256 KiB 预览和 `artifact_id`。
- 预览结果返回 `complete:false` 和下载命令。

### 11.3 大型产物

Heap Dump、大型日志等保存在 Agent：

- STX 只保存产物元数据。
- 下载必须使用 `artifact_id`，不能提交绝对路径。
- Agent 分块读取，STX 实时中转。
- 支持续传、长度检查和 SHA-256。
- 默认保留 7 天，可配置为 7 至 30 天。
- 首版不提供下载限速配置。

## 12. 权限、审计和秘密处理

### 12.1 审计

操作登记中需要审计的请求由公共中间件记录：

- 用户和认证方式。
- CLI 令牌编号与客户端版本，不保存令牌原文。
- `operation_id`、目标、风险、幂等键摘要、确认记录。
- 公共执行编号、Agent 命令编号、结果和耗时。
- 文件下载字节数和校验结果。

普通用户只能查询自己的记录，管理员按权限查询全部记录。审计事件只追加，首版不提供审计导出。

### 12.2 敏感信息

建立共用的秘密处理能力，并由业务 DTO 明确标记敏感字段。不能只依赖通用字符串替换。

- 参考 SeaTunnel 的敏感键列表。
- API、CLI、stderr、审计、日志和诊断包均不能返回秘密原文。
- `raw` 不绕过脱敏。
- 管理员也不能通过产品接口找回秘密。
- 更新带掩码对象时默认保留原秘密，只有显式新值才替换。

### 12.3 调试工作台

- 任务默认私有，可显式共享。
- 执行归实际发起用户所有。
- 全局变量默认私有，可显式共享。
- 秘密变量始终只写不读。

服务尚未部署，因此不设计旧任务或旧变量迁移。

## 13. 安装包源码

本地安装包列表以运行包为主记录，并增加源码状态：

- 自动下载默认同时下载同版本源码。
- 手工导入要求运行包，源码可选。
- 后续可按版本单独补充源码。
- 源码只提供整包下载，不在 STX 服务端解压或搜索。
- 校验版本、文件名、tar.gz 可读性和 SHA-256。
- 删除运行包时删除关联源码。

这部分通过普通 `package` CLI 命令暴露，不建立另一套客户端协议。

## 14. Skill 命令

使用 Go `embed` 随 `stx` 二进制携带一份 `SKILL.md`。首版只管理：

```text
~/.claude/skills/stx
~/.agents/skills/stx
```

安装和更新前备份已有内容，使用临时目录和原子替换。Skill 中只描述公开命令和安全规则，不保存服务地址、令牌或用户数据。

## 15. 兼容和发布

### 15.1 服务启动兼容

- 新入口：`stx server`。
- 旧入口：`stx api` 隐藏并标记废弃一个版本。
- 根命令不再根据第一个位置参数手工分发。
- 数据库迁移只在 Server 启动路径执行。

### 15.2 客户端与服务端版本

- 新 CLI 调用旧服务端时，如果没有 capabilities，返回明确的 `capabilities_unavailable`，提示升级服务端。
- 服务端不支持某操作时返回 `operation_not_supported`。
- 操作修订不兼容时返回 `operation_revision_mismatch`。
- 旧 CLI 不会自动获得新命令，这是正常的客户端升级边界。

### 15.3 回退

实现期间保留现有网页 API 行为。CLI 公共中间件出现问题时，可以停止发布 CLI 命令，但不能通过关闭中间件绕过服务端权限和审计。

数据库新增表和字段在服务尚未部署的前提下直接采用新结构，不增加历史数据转换代码。

## 16. 测试方案

### 16.1 单元测试

- 配置优先级、文件权限和多命名空间。
- Header、请求构建、重试和幂等键复用。
- JSON/table/yaml/raw、`--pick` 和 stderr 事件。
- 操作登记唯一性、命令冲突和样例合法性。
- 风险确认、资源版本和退出码映射。
- 公共执行状态迁移和取消竞争。
- 敏感字段脱敏。

### 16.2 集成测试

- `stx login` 后调用真实测试服务器。
- 普通查询、R1、R2、R3 完整流程。
- 相同幂等键重试不会重复执行。
- 用户只能查看自己的执行和审计。
- Agent 诊断、产物续传和校验。
- 全部路由均被操作登记或例外清单覆盖。

### 16.3 命令契约测试

每个公开命令至少验证：

- `--help` 中的风险、影响和样例。
- 默认 stdout 是合法 JSON。
- stderr 每行是合法 JSON。
- 失败时 stdout 为空且退出码稳定。
- 输出样例通过当前结构校验。

## 17. 首版不做

- 任意 Shell、SSH 或任意路径文件读取。
- 火焰图、JFR 和 async-profiler。
- 审计导出。
- STX 服务端源码索引和搜索。
- 独立 Scheduler、Worker 服务。
- 自动策略的长期授权版本管理。
- 节点产物下载限速配置。
- 模拟升级命令。
