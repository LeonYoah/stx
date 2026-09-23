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

登录用户名按 `--username`、`STX_USERNAME`、TTY 交互输入的顺序读取。真实终端缺少用户名时，在 stderr 显示 `Username: ` 并读取一行；非交互环境不能等待输入。密码交互必须先关闭终端回显，再在 stderr 显示 `Password: `；脚本仍使用 `--password-stdin`。交互提示是 stderr NDJSON 约定的特例，最终成功结果仍只能写 stdout。

## 4. 校验与错误矩阵

| 情况 | HTTP/API 行为 | CLI 行为 |
| --- | --- | --- |
| 缺少 CLI Header | 返回 `400` | 输出 `usage` 或协议错误事件 |
| 缺少令牌 | 返回 `401` | stderr 输出 `authentication_required`，使用认证退出码 |
| 令牌所属用户已禁用 | 返回 `403` | stderr 输出认证失败事件，不继续调用业务接口 |
| 服务端不可达 | 无 HTTP 响应 | stderr 输出 `network_error`，标记可重试 |
| 请求超时 | 无 HTTP 响应 | stderr 输出 `timeout`，标记可重试 |
| 命名空间未配置 | 不发起请求 | stderr 输出本地配置错误 |
| 非交互登录缺少用户名 | 不发起请求 | stderr 输出 `usage_error`，要求 `--username` 或 `STX_USERNAME` |
| TTY 登录缺少用户名 | 不发起请求 | 显示 `Username: ` 并读取输入 |
| `--password-stdin` 为空 | 不发起请求 | stderr 输出用法错误，不能回退到明文参数 |
| 非交互登录未使用 `--password-stdin` | 不发起请求 | stderr 输出 `usage_error`，不能等待输入 |
| TTY 密码输入取消 | 不发起请求 | 恢复终端状态并返回稳定用法错误 |

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

正常的交互登录：

```text
$ stx login --server http://127.0.0.1:17800
Username: admin
Password:
{"api_version":"v1","operation_id":"auth.cli.login","data":{"token_set":true}}
```

密码输入不能回显。脚本模式使用 `--username` 和 `--password-stdin`，不能尝试读取 TTY。

错误示例包括：把令牌正文打印到登录结果、在本地命令启动时迁移数据库、把错误文本混入 stdout、只修改任务状态却宣称实际操作已经取消。

## 6. 必须有的测试

- 根命令、帮助和本地命名空间命令在没有服务端配置时可运行，且不创建数据库。
- 服务端只在 `server` 命令路径读取配置并启动迁移。
- 客户端请求检查四个请求 Header，且每次请求都有请求编号。
- 登录读取 stdin，非交互环境拒绝直接读取密码，stdout 和 stderr 都没有令牌正文。
- TTY 登录在缺少用户名时依次显示 `Username:`、`Password:`，用户名可见、密码不回显，最终 stdout 只有一个 JSON 值。
- 用户名来源优先级为 `--username`、`STX_USERNAME`、TTY 输入；非 TTY 缺少用户名时立即返回退出码 2。
- 密码输入覆盖退格、回车、取消和终端状态恢复；真实伪终端测试必须确认提示出现后立即写入密码也不会回显。
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

错误：`stx login --server <url>` 在真实终端中因为缺少 `--username` 直接退出，或者先显示密码提示再关闭终端回显。

正确：真实终端依次询问用户名和密码，并在显示密码提示前关闭回显；非交互调用继续明确要求用户名来源和 `--password-stdin`。

## 8. 场景：从操作登记生成普通命令

### 8.1 范围 / 触发条件

当现有 API 需要提供给 CLI 和 AI Agent，且输入只包含 path、query、统一安全 Header 或完整 JSON 正文时，优先在 `internal/operation/registry.go` 登记，并由 `internal/cli/command` 构建普通命令。通用构建器支持 GET、POST、PUT 和 PATCH；包含 `InputBody` 时统一生成 `--request-file`。登录、能力查询、公共执行、下载、流式命令、文件上传和需要特殊交互的命令仍保留专用实现。

### 8.2 签名

登记项至少包含：

```go
operation.OperationSpec{
    ID:           "host.discovery.process.list",
    CommandPath:  []string{"host", "discovery", "process", "list"},
    Summary:      "List SeaTunnel processes discovered on a host",
    GeneratedCLI: true,
    Method:       "POST",
    Route:        "/api/v1/hosts/:id/discover-processes",
    Mode:         operation.ModeNormal,
    Risk:         operation.RiskR0,
    Revision:     1,
    UsesAgent:    true,
    Impact: &operation.ImpactSpec{
        Level:       operation.RiskR0,
        Message:     "Runs a read-only SeaTunnel process scan on the target host through STX Agent.",
        Performance: "Reads local process metadata; it does not stop or modify SeaTunnel processes.",
    },
}
```

构建入口：

```go
command.Build(specs []operation.OperationSpec, factory command.ClientFactory) ([]*cobra.Command, error)
```

### 8.3 规则

- 本地登记项负责命令路径、帮助摘要、输入、示例和输出样例；执行 `--help` 不访问网络。
- 实际 HTTP 方法必须取自 `OperationSpec.Method`，不能在构建器中写死为 GET。
- GET、POST、PUT 和 PATCH 在 `ModeNormal` 下可以由通用构建器生成；DELETE 不能公开为 CLI。
- `InputPath` 按登记顺序变成位置参数并使用 `url.PathEscape`。
- `InputQuery` 变成同名长参数，只有用户显式传入时才加入 URL。
- 存在 `InputBody` 时生成 `--request-file`，文件内容作为完整 JSON 正文发送；任一 body 输入必填时，该参数也必填。
- 请求文件缺失、为空或 JSON 无效时，在创建客户端和查询 capability 前返回 `usage_error`。
- 通用构建器不接受任意 Header 和文件上传输入；`Idempotency-Key`、`X-STX-Confirm`、`X-STX-Confirmation-ID` 由写操作参数统一处理。
- R1 至 R3 命令要求 `--confirm`，并支持幂等键和一次性确认编号。
- 每个生成命令支持 `--namespace`，结果继续走公共 `--output`、`--format`、`-f` 和 `--pick`。
- 业务请求前必须调用 `/api/v1/capabilities`，检查 operation ID、权限、mode 和 revision。
- 登记项包含 `ImpactSpec` 时，帮助必须显示风险等级、影响说明和性能说明；显示帮助不能创建客户端或访问服务端。
- CLI 客户端优先读取通用响应外层中的 `data`。若成功响应没有 `data`、`error_code` 和 `error_msg`，则把整个合法 JSON 当作业务结果，以兼容遗留裸 JSON 接口。
- 遗留错误响应 `{"error":"..."}` 必须保留真实错误消息，并继续按 HTTP 状态映射统一错误类型和退出码。
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
| 必填 body 未提供 `--request-file` | 不发请求，返回 `usage_error` / 退出码 2 |
| 请求文件为空或 JSON 无效 | 不发请求，返回 `usage_error` / 退出码 2 |
| GET 或 DELETE 登记 `InputBody` | 构建命令失败，不注册命令 |
| 输入包含任意 Header 或上传文件 | 构建命令失败，不注册命令 |
| DELETE 设置 `GeneratedCLI=true` | 操作登记校验失败 |
| 非 normal 模式 | 构建命令失败，不注册命令 |
| 成功响应是非法 JSON | 返回 `server_error`，不能输出部分结果 |
| 遗留接口返回 `404` 和 `{"error":"host not found"}` | 保留服务端消息，返回 `not_found` / 退出码 5 |

### 8.5 Good / Base / Bad

- Good：登记 Java Proxy 配置 PUT，并声明一个必填 `InputBody`；生成命令通过 `--request-file` 发送完整 JSON，同时保留确认和幂等处理。
- Base：普通 GET 和无正文 POST 继续使用通用构建器；特殊 watch、download、文件上传或密码交互继续独立实现。
- Bad：为了绕过构建器的方法检查把 PUT 改成 POST，却仍发送空正文。

### 8.6 必须有的测试

- path 转义、可选 query、命名空间和字段选择。
- GET 使用 GET；无请求体 POST 使用登记的方法且请求体为 `nil`。
- PUT 和 PATCH 能读取 `--request-file`，原样发送有效 JSON，并携带确认和幂等 Header。
- 必填请求文件缺失、文件为空、JSON 无效时，能力查询和业务请求次数均为 0。
- GET/DELETE 带 body、任意 Header、上传文件和非 normal 模式必须构建失败。
- 离线 help 不创建客户端，不调用 capability。
- `ImpactSpec` 的风险等级、影响内容和性能说明出现在帮助中。
- capability 缺失、拒绝、旧 revision 和 mode 不匹配时，业务 API 调用次数为 0。
- 通用响应外层、成功裸 JSON、遗留 `{"error":"..."}` 和非法成功 JSON 都有客户端测试。
- 根命令显示新增命令组，路由基线中的对应项从 `historical_gap` 变为 `operation`。
- 构建真实 `stx` 二进制，连接本地服务完成登录、能力检查和业务查询。

### 8.7 错误与正确示例

错误：

```go
spec.Method = "PUT"
spec.Risk = operation.RiskR2
spec.Input = append(spec.Input, operation.InputSpec{Location: operation.InputBody, Required: true})
spec.GeneratedCLI = true
```

如果构建器仍固定发送空正文，这个命令虽然能出现在帮助中，但实际调用必然失败。

正确：

```go
spec.Method = "PUT"
spec.Mode = operation.ModeNormal
spec.Risk = operation.RiskR2
spec.Input = []operation.InputSpec{
    {Name: "id", Location: operation.InputPath, Required: true},
    {Name: "request", Location: operation.InputBody, Required: true},
    {Name: "Idempotency-Key", Location: operation.InputHeader, Required: true},
    {Name: "X-STX-Confirm", Location: operation.InputHeader, Required: true},
}
spec.GeneratedCLI = true
commands, err := command.Build([]operation.OperationSpec{spec}, clientFactory)
```

登记表是命令定义来源。构建器按 `OperationSpec.Method` 发起请求，`InputBody` 由 `--request-file` 提供，并继续执行能力检查和统一输出。

## 9. 同名查询参数

### 9.1 适用范围

当 Handler 使用 `QueryArray` 或同类方式读取一个 query 名称的多个值时，生成式 CLI 必须明确登记为重复参数，不能把多个值拼成一个普通字符串。

### 9.2 签名

登记方式：

```go
operation.InputSpec{
    Name:        "profile_keys",
    Location:    operation.InputQuery,
    Required:    false,
    Repeated:    true,
    Description: "Dependency profile key; may be specified more than once",
}
```

CLI 调用：

```text
stx plugin download status get jdbc \
  --version 2.3.13 \
  --profile_keys mysql \
  --profile_keys postgresql
```

HTTP 请求：

```text
GET /api/v1/plugins/jdbc/download/status?profile_keys=mysql&profile_keys=postgresql&version=2.3.13
```

### 9.3 契约

- `Repeated=true` 只允许用于 `InputQuery`。
- 普通 query 使用 Cobra string flag 和 `url.Values.Set`。
- 重复 query 使用 Cobra string-array flag，并对每个有效值调用 `url.Values.Add`。
- 构建 URL 前清除值两端的空白，忽略空值，保留有效值的输入顺序。
- 必填重复参数至少要有一个非空值，否则返回 `usage_error`，且不能创建客户端或发送请求。
- `Repeated` 会进入操作登记摘要；capability 当前不返回输入定义。

### 9.4 校验与错误对应表

| 情况 | 行为 |
| --- | --- |
| query 未设置 `Repeated` | 继续按单值参数处理 |
| query 设置 `Repeated=true` | 生成可多次传入的 string-array flag |
| path/header/body/file 设置 `Repeated=true` | 登记校验和命令构建都失败 |
| 可选重复参数全部为空 | 不写入 URL |
| 必填重复参数未传或全部为空 | `usage_error` / 退出码 2，不发请求 |
| 多次传入有效值 | URL 中保留多个同名参数 |

### 9.5 Good / Base / Bad

- Good：`--profile_keys mysql --profile_keys postgresql` 生成两个 `profile_keys`，服务端 `QueryArray` 读取两个值。
- Base：只有一个值时仍使用同一个重复参数入口，生成一个 query 值。
- Bad：把值拼成 `mysql,postgresql` 后使用 `Set`；服务端会收到一个值，且逗号可能是业务值的一部分。

### 9.6 必须有的测试

- 命令构建测试断言重复 flag 生成两个同名 URL 参数，顺序与输入一致。
- 空值测试断言空字符串被忽略。
- 必填测试断言没有有效值时返回退出码 2，业务请求次数为 0。
- 登记校验和命令构建测试都要拒绝非 query 的 `Repeated=true`。
- 真实二进制测试使用两个重复值调用本地服务，并断言响应或服务端结果保留两个值。

### 9.7 错误与正确示例

错误：

```go
query.Set("profile_keys", strings.Join(values, ","))
```

正确：

```go
for _, value := range values {
    query.Add("profile_keys", value)
}
```

同名 query 是 HTTP 协议中的多个值，不应在 CLI 内自行发明分隔符。

## 10. 场景：CLI 写命令的确认、幂等与敏感输入

### 10.1 范围 / 触发条件

- 触发条件：命令通过远端 API 修改个人资料、用户、安装包或其他服务端资源。
- 目标：让脚本和 AI Agent 能稳定重试，同时避免密码进入参数、日志和输出。

### 10.2 签名

写命令统一支持：

```text
--namespace <name>
--confirm
--idempotency-key <key>
--confirmation-id <id>
```

密码字段只能通过隐藏终端输入或 `--password-stdin` 提供，禁止增加 `--password <value>`。

### 10.3 契约

- CLI 请求携带 `X-STX-Client: cli`、`Idempotency-Key` 和 `X-STX-Confirm: true`。
- R2 第一次请求把一次性 `confirmation_id` 写入 stderr；第二次请求必须继续使用同一个幂等键并附带该编号。
- 成功业务结果只写 stdout，warning、幂等键、确认编号和错误事件写 stderr，每行一个 JSON 值。
- CLI 不保存请求正文；服务端执行结果只保留安全引用，例如用户 ID。
- 布尔更新字段使用 Cobra 的 `Changed` 判断，未传入时不能把默认值写进请求正文。

### 10.4 校验与错误对应表

| 情况 | CLI 行为 |
| --- | --- |
| 缺少 `--confirm` | 不发起业务请求，返回冲突退出码 |
| 缺少幂等键 | 自动生成并把幂等键事件写入 stderr |
| R2 首次请求 | 返回确认编号事件和最终错误事件，退出码为冲突类 |
| 相同幂等键、请求内容相同 | 输出原业务结果 |
| 相同幂等键、请求内容不同 | 输出幂等冲突，不能复用旧结果 |
| 非交互环境未使用 `--password-stdin` | 不等待输入，返回用法错误 |

### 10.5 Good / Base / Bad

- Good：密码从 stdin 读取，stdout 和 stderr 均不出现密码，重试使用稳定幂等键。
- Base：无密码的 R1 写命令也要求显式确认和幂等键。
- Bad：把密码放入命令参数、帮助样例、审计详情或错误消息。

### 10.6 必须有的测试

- 断言缺少确认时没有业务请求。
- 断言 R1 的相同幂等键可以复用结果，变更正文会返回冲突。
- 断言 R2 需要一次性确认编号，重复成功请求仍保持幂等。
- 断言显式 `false` 的布尔字段会进入请求正文，未传入字段不会进入正文。
- 真实二进制测试需检查 stdout 为单个 JSON、stderr 为 NDJSON，并确认密码和密码摘要均未出现。

### 10.7 错误与正确示例

错误：

```text
stx admin user create --username demo --password plain-text
```

正确：

```text
printf '%s\n' "$STX_TEST_PASSWORD" | stx admin user create \
  --username demo --password-stdin --confirm --idempotency-key create-demo
```

## 11. 场景：由登记表生成写命令，并禁止 CLI 删除资源

### 11.1 范围

无正文的 `POST` 以及带完整 JSON 正文的 `POST`、`PUT`、`PATCH` 都可以由操作登记表生成。CLI 不提供任何资源删除入口；服务端和网页可以继续保留原有 `DELETE` API。文件上传、密码交互和其他特殊输入仍使用专用实现。

### 11.2 登记和命令约定

- `GeneratedCLI=true` 时，方法可以是 `GET`、`POST`、`PUT` 或 `PATCH`；`DELETE` 必须保持 `GeneratedCLI=false`。
- 存在 `InputBody` 时统一增加 `--request-file`，将文件中的完整 JSON 作为请求正文。
- 删除类操作不设置 `CommandPath` 和 CLI `Example`，避免能力信息和帮助文案推荐不可执行命令。
- 手写 Cobra 命令也不能使用 `delete` 或表示资源删除的 `remove` 命令名。
- R1 至 R3 必须提供 `ImpactSpec`，并在登记输入中声明 `Idempotency-Key` 和 `X-STX-Confirm`。
- 生成命令自动增加 `--confirm`、`--idempotency-key` 和 `--confirmation-id`。
- 缺少 `--confirm` 时不能创建客户端、查询能力或调用业务接口。
- 执行前先查询 capability；服务端允许后才生成或使用幂等键并调用业务接口。
- warning、自动生成的幂等键和二次确认编号写 stderr，业务结果写 stdout。
- 客户端确认只负责避免误操作，不能代替服务端的确认、幂等和公共执行记录。

### 11.3 必须有的测试

- 无正文 R0 POST 能正常生成和调用。
- 带正文的 POST、PUT 和 PATCH 能从请求文件读取 JSON 并发送。
- 必填请求文件缺失或无效时，不查询能力，也不调用业务接口。
- R1/R2 POST 缺少确认时网络调用次数为 0。
- 遍历根命令树，断言不存在 `delete` 或资源删除语义的 `remove` 命令。
- 遍历操作登记，断言所有 `DELETE` 操作均为 `GeneratedCLI=false` 且没有命令路径。
- 带确认的命令发送统一安全请求头，并输出影响提示。
- 服务端返回 `confirmation_required` 时，stderr 包含一次性确认编号。
- 任意 Header 和上传文件输入仍被生成器拒绝；`InputBody` 只允许 POST、PUT 和 PATCH。

### 11.4 错误与正确示例

错误：

```go
OperationSpec{Method: http.MethodDelete, GeneratedCLI: true, CommandPath: []string{"package", "delete"}}
```

正确：

```go
OperationSpec{ID: "package.delete", Method: http.MethodDelete, GeneratedCLI: false}
```

服务端操作编号继续用于路由登记、网页调用和审计，但 CLI 命令树中没有对应入口。

### 11.5 校验与错误对应表

| 情况 | 行为 |
| --- | --- |
| `DELETE` 设置 `GeneratedCLI=true` | 登记校验失败，构建和测试不能通过 |
| 非 CLI 删除操作设置命令路径或样例 | 规范测试失败，要求清除命令提示 |
| 用户执行旧的 `stx ... delete` | Cobra 返回未知命令或多余参数，不发起网络请求 |

### 11.6 Good / Base / Bad

- Good：服务端保留删除 API，网页按现有权限调用，CLI 只提供查询、新增、修改和受控执行命令。
- Base：取消异步任务继续使用 `cancel`，因为它停止一次执行，不是删除业务资源。
- Bad：把删除操作改名为 `remove` 后继续暴露给 CLI。

### 11.7 范围边界

本约定只限制 `stx` CLI。服务端 Handler、Repository 和网页删除按钮是否保留，由对应业务需求决定；不能为了移除 CLI 命令而删除已有 API。

### 11.8 安装流程资源接口

- Agent 安装脚本、卸载脚本、CA、Agent 二进制和 Java Proxy 文件只供主机安装流程使用，登记为 `ModeDownload` 路由例外，不生成用户 CLI。
- 不得仅因为某个路由存在，就为它增加 CLI 命令；先确认它是否属于用户或 AI Agent 的实际使用场景。
- 已停止使用的临时发布接口应删除路由、实现、测试和 Swagger 内容，不能继续留在操作登记中。

## 12. 场景：带 JSON 正文的 R0 查询与运行时存储

### 12.1 范围 / 触发条件

- API 使用 `POST`，但只执行查询、解析或连通性检查，不修改服务端和集群状态。
- CLI 需要发送路径、递归选项、读取上限或存储配置等 JSON 正文。
- 运行时存储可能处于 `DISABLED`，此时不能继续调用 Agent 或 Java Proxy 假装存在外部文件。

### 12.2 命令与 API

```text
stx cluster runtime-storage validate <cluster-id> <checkpoint|imap>
stx cluster runtime-storage list <cluster-id> <checkpoint|imap> [--path <path>] [--recursive] [--limit <n>]
stx cluster runtime-storage preview <cluster-id> <checkpoint|imap> --path <path> [--max-bytes <n>]
stx cluster runtime-storage checkpoint inspect <cluster-id> --path <path> [--job-config-file <json>]
stx cluster runtime-storage imap inspect <cluster-id> --path <path>
stx installer runtime-storage validate --request-file <json>
```

对应接口使用 `POST` 和 JSON 正文，但风险等级是 R0。操作登记设置 `GeneratedCLI=false`，由专用 Cobra 命令读取正文并先查询 capability。

### 12.3 请求与响应规则

- `kind` 只允许 `checkpoint` 或 `imap`。
- `preview` 和 `inspect` 的 `--path` 应使用 `list` 返回的完整 `path`；本地存储也可使用绝对路径。
- `--request-file` 表示完整正文。提供该参数时，不再用其他字段参数覆盖文件内容。
- 列表响应的 `items` 必须始终是数组；没有文件时返回 `[]`，不能省略，也不能返回 `null`。
- IMAP 为 `DISABLED` 时：`validate` 返回成功并说明无需外部存储；`list` 返回 `items: []`；`preview` 和 `inspect` 返回明确的关闭状态错误。
- Agent 命令日志保留实际命令参数，但 access key、secret key 等敏感字段必须显示为掩码。

### 12.4 校验与错误对应表

| 情况 | 行为 |
| --- | --- |
| `kind` 不是 `checkpoint` 或 `imap` | CLI 在发起网络请求前返回用法错误 |
| `preview` / `inspect` 缺少路径 | CLI 在发起网络请求前返回用法错误 |
| IMAP 已关闭且执行校验 | 返回成功，`details.mode=disabled` |
| IMAP 已关闭且执行列表 | 返回成功，`items=[]` |
| IMAP 已关闭且执行预览或检查 | 返回 `imap runtime storage is disabled` |
| 文件不存在 | 返回稳定服务端错误，并保留请求编号 |
| checkpoint 文件格式无效 | Java Proxy 返回解析失败，CLI 使用服务端错误退出码 |

### 12.5 Good / Base / Bad

- Good：先执行 `list`，再把返回的完整路径交给 `preview` 或 `inspect`。
- Base：安装前使用 `--request-file` 一次传入主机编号、类型和存储配置。
- Bad：看到接口是 `POST` 就强制要求 `--confirm`，或在 IMAP 已关闭时仍调用 Java Proxy。

### 12.6 必须有的测试

- CLI 单元测试检查六条命令的路径、正文、能力查询和输出。
- 缺少路径、非法 `kind` 时，断言网络请求数为 0。
- 服务测试覆盖 IMAP `DISABLED` 的校验、空列表和明确错误。
- 列表为空时断言 JSON 中存在 `"items":[]`。
- 真实测试连接本机 STX 和 Agent，至少完成 checkpoint 校验、列表、文本预览和安装前校验；存在合法 checkpoint/WAL 样本时再验证解析成功。
- 审计检查确认 `request_id`、`client_type=cli`、`created_by` 和脱敏参数正确。

### 12.7 错误与正确示例

错误：

```go
if kind == "imap" {
    sendJavaProxyCommand()
}
```

正确：

```go
if runtimeStorageValidationDisabled(kind, cfg) {
    return &RuntimeStorageListResult{Items: []RuntimeStorageListItem{}}, nil
}
```

## 13. 场景：内置 STX Skill 的语言选择与安装

### 13.1 范围 / 触发条件

- `stx` 二进制需要为 AI Agent 安装 STX 使用说明。
- 发布物同时携带中文版和英文版，但每个安装目标最终只保存一个 `SKILL.md`。
- 首版只管理 Claude 与 Agents 目录，不允许传入任意安装路径。

### 13.2 命令签名

```text
stx skill show [--language auto|zh-CN|en]
stx skill status [--target all|claude|agents] [--language auto|zh-CN|en]
stx skill install [--target all|claude|agents] [--language auto|zh-CN|en]
stx skill update [--target all|claude|agents] [--language auto|zh-CN|en]
stx skill backup [--target all|claude|agents]
stx skill restore [--target all|claude|agents] [--backup <id>]
```

固定安装位置：

```text
claude -> ~/.claude/skills/stx/SKILL.md
agents -> ~/.agents/skills/stx/SKILL.md
```

### 13.3 语言与文件规则

- `--language auto` 依次读取 `LC_ALL`、`LC_MESSAGES`、`LANGUAGE`、`LANG`。
- 明确识别到 `en`、`en_US`、`en-US` 等英语区域时使用英文版。
- 中文、空值、`C`、无法识别的语言都使用中文版；中文版是默认版本。
- `--language zh-CN` 与 `--language en` 可以覆盖自动选择；其他显式值返回用法错误。
- `show` 只把选中的 `SKILL.md` 原样写到 stdout，不加普通结果外壳。
- `status`、`install`、`update`、`backup`、`restore` 使用普通机器可读结果外壳。
- `install` 只补缺失文件，不覆盖已有文件。
- `update` 对缺失文件执行安装；内容变化时先备份，再用同目录临时文件原子替换；内容相同则返回 `unchanged`。
- 备份存放在 `~/.stx/skill-backups/<target>/<backup-id>/SKILL.md`，文件权限为 `0600`。
- `restore` 默认选择每个目标最新的备份。覆盖当前文件前再次保存安全备份。
- 备份编号只能包含字母、数字、点、短横线和下划线，不能包含路径分隔符。

### 13.4 校验与错误对应表

| 情况 | 行为 |
| --- | --- |
| 系统语言为英语 | `auto` 选择 `en` |
| 系统语言缺失或无法识别 | `auto` 选择 `zh-CN` |
| 显式语言不是 `auto|zh-CN|en` | 用法错误，不写文件 |
| target 不是 `all|claude|agents` | 用法错误，不写文件 |
| `install` 遇到已有文件 | 返回 `skipped_existing`，不覆盖、不备份 |
| `update` 遇到不同内容 | 先备份，随后原子替换 |
| `restore` 没有可用备份 | 未找到错误，不改变目标文件 |
| 指定的备份只存在于部分目标 | 在写入前完成全部目标校验；任一目标缺失则不恢复 |

### 13.5 Good / Base / Bad

- Good：英语机器自动安装英文版，中文或没有语言信息的机器安装中文版；用户可用 `--language` 明确覆盖。
- Base：已有自定义 Skill 时，`install` 保留原文件，用户确认后再运行 `update`。
- Bad：仓库维护两套安装目录并让 Agent 同时加载中英文，或根据服务端用户语言改写本机 Skill。

### 13.6 必须有的测试

- 内置中英文文件都通过 Skill 格式校验，且核心命令示例能在当前 Cobra 命令树中找到。
- 语言检测覆盖英语、中文、空值、`C` 和其他语言。
- `show` 断言 stdout 是原始 Markdown，不是 JSON。
- `install` 覆盖双目标、已有文件不覆盖和默认中文。
- `update` 断言内容变化前生成备份，内容相同时不重复写入。
- `restore` 断言最新备份、指定备份、安全备份和非法备份编号。
- 构建真实 `dist/stx` 后检查 `skill --help`、两种语言 `show` 和 `status`。

### 13.7 错误与正确示例

错误：

```go
language := os.Getenv("LANG")
if language == "" {
    language = "en"
}
```

这会在没有语言信息时错误地安装英文版。

正确：

```go
language := DetectLanguage(os.Getenv)
// 只有明确的英语区域返回 en，其他情况返回 zh-CN。
// Only an explicit English locale returns en; all other cases return zh-CN.
```

错误：

```go
os.WriteFile(target, source, 0o644)
```

正确：更新时先保存当前文件，再在目标目录写临时文件并通过 `os.Rename` 替换。
