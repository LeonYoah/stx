# STX CLI 约定

> 适用于根 Go 模块中的 `stx` 命令、远端 API 客户端和服务端 CLI 认证接口。

## 1. 范围

STX 使用同一个二进制提供两种入口：`stx server` 启动 API 服务，其他公开命令访问远端 STX API。根命令和本地命名空间命令不能因为服务端配置缺失而退出，也不能初始化数据库。

## 2. 命令与 API 签名

| 用途 | 命令或接口 | 约束 |
| --- | --- | --- |
| 启动服务 | `stx server` | 只在此路径加载服务端配置、迁移数据库并监听 HTTP |
| 兼容入口 | `stx api` | 隐藏命令，暂时等同于 `stx server` |
| 登录 | `POST /api/v1/auth/cli/login` | 请求带 `X-STX-Client: cli`，响应不能返回令牌原文 |
| 当前用户 | `GET /api/v1/auth/cli/whoami` | 需要 Bearer 令牌和 CLI Header |
| 退出登录 | `POST /api/v1/auth/cli/logout` | 撤销令牌后本地删除令牌，重复撤销保持幂等 |
| 能力查询 | `GET /api/v1/capabilities` | 返回服务端版本、登记表摘要、操作权限和拒绝原因 |

## 3. 请求、响应和环境变量

远端 CLI 请求固定携带：

```text
Authorization: Bearer <cli-token>
X-STX-Client: cli
User-Agent: stx-cli/<version>
X-Request-ID: <uuid>
Accept: application/json
```

`X-STX-Client` 只用于区分调用来源，不能替代身份认证或权限判断。令牌属于用户，服务端每次请求都根据用户当前状态和权限判断，不把长期令牌转交给 Agent。

服务端配置路径使用 `CONFIG_PATH`，数据库测试路径可使用 `STX_DATABASE_TYPE` 和 `STX_DATABASE_SQLITE_PATH` 覆盖。CLI 本地配置使用 `XDG_CONFIG_HOME`，文件权限必须为 `0600`。

普通命令成功时，完整结果只写 stdout，且为单个 JSON 值。stderr 只写结构化事件，每行必须是一个 JSON 值。登录结果只能返回 `token_set: true`、令牌类型、过期时间和用户信息，不能返回令牌正文。

## 4. 校验与错误矩阵

| 情况 | HTTP/API 行为 | CLI 行为 |
| --- | --- | --- |
| 缺少 CLI Header | 返回 `400` | 输出 `usage` 或协议错误事件 |
| 缺少令牌 | 返回 `401` | stderr 输出 `authentication_required`，使用认证退出码 |
| 令牌所属用户已禁用 | 返回 `403` | stderr 输出认证失败事件，不继续调用业务接口 |
| 服务端不可达 | 无 HTTP 响应 | stderr 输出 `network_error`，标记可重试 |
| 请求超时 | 无 HTTP 响应 | stderr 输出 `timeout`，标记可重试 |
| 命名空间未配置 | 不发起请求 | stderr 输出本地配置错误 |
| `--password-stdin` 为空 | 不发起请求 | stderr 输出用法错误，不能回退到明文参数 |

退出码必须通过统一错误类型映射，不能让 Cobra 或底层 HTTP 错误直接决定进程退出码。错误消息、请求编号和重试标记不能包含令牌或密码。

## 5. 正常、边界和错误示例

正常的登录结果：

```json
{"operation_id":"auth.cli.login","data":{"token_set":true,"token_type":"Bearer"},"result_meta":{"complete":true}}
```

正常的错误事件：

```json
{"event":"error","code":"authentication_required","message":"CLI token is not configured","retryable":false}
```

错误示例包括：把令牌正文打印到登录结果、在本地命令启动时迁移数据库、把错误文本混入 stdout、只修改任务状态却宣称实际操作已经取消。

## 6. 必须有的测试

- 根命令、帮助和本地命名空间命令在没有服务端配置时可运行，且不创建数据库。
- 服务端只在 `server` 命令路径读取配置并启动迁移。
- 客户端请求检查四个请求 Header，且每次请求都有请求编号。
- 登录读取 stdin，非交互环境拒绝直接读取密码，stdout 和 stderr 都没有令牌正文。
- 令牌有效期默认为 7 天，接受 7 至 30 天，超过范围被拒绝，用户禁用后立即失效。
- 成功 stdout 是合法 JSON，错误 stderr 是合法 NDJSON，退出码稳定。
- 退出登录后本地令牌被清除，重复退出和已失效令牌行为可预测。
- 能力查询返回登记表修订摘要，普通用户对受限操作得到稳定拒绝原因。
- 真实二进制连接独立测试服务，完成登录、当前用户、能力查询、健康检查和退出登录。

## 7. 错误做法与正确做法

错误：在根命令构造时执行数据库迁移，导致执行 `stx namespace list` 也要求服务端配置。

正确：根命令只注册命令；`stx server` 的运行函数内部才校验服务端配置、执行迁移并启动路由。

错误：登录接口返回完整 Bearer 令牌，或把令牌写入日志。

正确：服务端只保存令牌哈希，CLI 配置文件保存令牌但权限设为 `0600`，登录输出只返回 `token_set`，日志和错误中不出现令牌正文。

错误：用 `X-STX-Client: cli` 作为授权依据。

正确：使用 Bearer 令牌确定用户身份，再读取用户当前权限；客户端 Header 只作为来源标识和兼容检查。

## 8. 场景：从操作登记生成普通 GET 命令

### 8.1 范围 / 触发条件

当现有只读 API 需要提供给 CLI 和 AI Agent 时，优先在 `internal/operation/registry.go` 登记，并由 `internal/cli/command` 构建普通命令。登录、能力查询、公共执行、下载和流式命令仍保留专用实现。

### 8.2 签名

登记项至少包含：

```go
operation.OperationSpec{
    ID:           "host.get",
    CommandPath:  []string{"host", "get"},
    Summary:      "Get one host",
    GeneratedCLI: true,
    Method:       "GET",
    Route:        "/api/v1/hosts/:id",
    Mode:         operation.ModeNormal,
    Revision:     1,
}
```

构建入口：

```go
command.Build(specs []operation.OperationSpec, factory command.ClientFactory) ([]*cobra.Command, error)
```

### 8.3 契约

- 本地登记项负责命令路径、帮助摘要、输入、示例和输出样例；执行 `--help` 不访问网络。
- `InputPath` 按登记顺序变成位置参数并使用 `url.PathEscape`。
- `InputQuery` 变成同名长参数，只有用户显式传入时才加入 URL。
- 每个生成命令支持 `--namespace`，结果继续走公共 `--output`、`--format`、`-f` 和 `--pick`。
- 业务请求前必须调用 `/api/v1/capabilities`，检查 operation ID、权限、mode 和 revision。
- 普通结果仍只向 stdout 写一个值；错误和 `pick_fallback` 事件写 stderr NDJSON。

### 8.4 校验与错误对应表

| 情况 | 行为 |
| --- | --- |
| operation ID 不存在 | 不调用业务 API，返回 `not_found` / 退出码 5 |
| `allowed=false` | 不调用业务 API，返回 `permission_denied` / 退出码 4 |
| 服务端 revision 小于本地 revision | 不调用业务 API，返回 `conflict` / 退出码 6 |
| 服务端 mode 与本地登记不同 | 不调用业务 API，返回 `conflict` / 退出码 6 |
| path 输入与路由占位符不一致 | 构建命令失败，测试阶段发现 |
| 必填 query 未传 | 不发请求，返回 `usage_error` / 退出码 2 |

### 8.5 Good / Base / Bad

- Good：登记 `host.get` 后，由同一登记项提供 capability、help、请求路由和覆盖报告信息。
- Base：特殊 watch 或 download 命令继续独立实现，不强行交给普通 GET 构建器。
- Bad：只在 Cobra 中手写命令却不登记操作，导致能力查询和路由覆盖报告仍显示缺口。

### 8.6 必须有的测试

- path 转义、可选 query、命名空间和字段选择。
- 离线 help 不创建客户端，不调用 capability。
- capability 缺失、拒绝、旧 revision 和 mode 不匹配时，业务 API 调用次数为 0。
- 根命令显示新增命令组，路由基线中的对应项从 `historical_gap` 变为 `operation`。
- 构建真实 `stx` 二进制，连接本地服务完成登录、能力检查和业务查询。

### 8.7 错误与正确示例

错误：

```go
root.AddCommand(&cobra.Command{Use: "host-get", RunE: callHostAPI})
```

这会复制路由和帮助信息，也无法自动进入能力查询与覆盖检查。

正确：

```go
spec.GeneratedCLI = true
commands, err := command.Build([]operation.OperationSpec{spec}, clientFactory)
```

登记表是命令定义来源，构建器只负责输入映射、能力检查、请求和统一输出。
