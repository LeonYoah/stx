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
