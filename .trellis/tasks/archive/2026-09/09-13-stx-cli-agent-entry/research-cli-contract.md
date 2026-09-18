# STX CLI 设计参考：dp-cli、Multica 与 CLI 指南

## 研究日期

2026-09-16

## 参考材料

- 本机 `dp-cli` Skill：`/Users/mac/Documents/projects/youzan/dp-skills/dp-cli/SKILL.md`
- 本机 dp-cli 源码：`/Users/mac/Documents/projects/youzan/yz-bigdata-cli`
- Multica 研究：`research-multica.md`
- 命令行界面设计指南：用户提供的中文指南

## 可以借鉴的做法

### dp-cli

- 每个命令同时登记用途、访问类型、参数、使用示例、输出样例和列定义。帮助信息不只列参数，也告诉使用者结果长什么样。
- 全局 `--pick` 只保留结果行的指定顶层字段；结果元数据始终保留。字段不存在时不悄悄丢数据，而是返回原始结果并向 stderr 写出结构化的回退事件。
- 列表结果带有统一的结果元数据：返回模式、是否完整、返回数量、总数量、分页信息、是否还有后续结果、原因和下一条命令。
- `nextCommand` 由当前参数和有效配置生成，并进行 Shell 转义。分页、查看详情、获取完整结果等后续动作可以直接复制执行。
- JSON 主结果只写 stdout，结果处理标记写 stderr。调用者可以分别保存结果和处理状态，不需要从混合文本中猜 JSON 边界。
- Skill 管理有 `status`、`install`、`update`、备份和恢复；替换前校验摘要，使用临时目录、同目录原子替换和本地操作锁，失败时可以恢复旧内容。

### Multica

- 使用 Go/Cobra 组织资源命令，认证、配置、HTTP 请求、错误和文件传输由公共组件处理，业务命令只描述自身参数和调用过程。
- 使用 profile 保存不同服务地址和环境，命令参数、环境变量、配置文件和默认值有固定优先顺序。
- CLI 使用 Bearer 令牌，客户端标识 Header 只用于来源识别、日志和兼容判断，不作为授权依据。
- 错误按认证失败、无权限、资源不存在、输入错误、网络失败和服务端错误分类，并映射到稳定退出码。
- 大文件采用流式传输，不把安装包、源码包、诊断包或 Dump 一次性读入内存。
- STX 不照搬 Multica 的 Agent 临时令牌。STX 已确认 CLI 令牌与用户绑定，并且每次请求读取用户当前权限。

### 命令行指南

- 帮助需要让用户能发现功能；命令示例和输出样例比只列参数更有用。
- 输出要足够说明当前状态，但不能把大量日志和无关字段混在主要结果中。
- 错误需要说明发生了什么、是否已经产生远端影响，以及下一步可以执行什么。
- 有风险的操作先展示影响和当前状态，再进入确认或执行阶段；中途状态要能被查询。

## STX 统一方案

### 1. CLI 请求 Header

CLI 对远端 STX API 的请求固定携带：

```http
Authorization: Bearer <token>
X-STX-Client: cli
User-Agent: stx-cli/<version>
```

- `Authorization` 是 CLI 身份凭据，服务端根据令牌所属用户的当前状态和权限处理请求。
- `X-STX-Client` 固定为 `cli`，只用于区分调用来源。
- `User-Agent` 携带实际 CLI 版本，用于日志、审计、故障定位和兼容提示。
- 不增加协议版本 Header。API 主版本由 `/api/v1` 表示，具体操作兼容性由能力查询接口和 `operation_id` 判断。
- `X-STX-Client` 与 `User-Agent` 都可能被调用方伪造，不能参与认证或权限判断。审计应分别保存认证方式、客户端类型和客户端版本。

### 2. 能力查询

登录成功后，CLI 调用：

```text
GET /api/v1/capabilities
```

响应的核心结构如下：

```json
{
  "api_version": "v1",
  "server_version": "0.1.0",
  "min_cli_version": "0.1.0",
  "registry_revision": "sha256:example",
  "operations": [
    {
      "operation_id": "diagnostics.resource.list",
      "revision": 1,
      "allowed": true
    },
    {
      "operation_id": "cluster.delete",
      "revision": 1,
      "allowed": false,
      "denial_code": "admin_required"
    }
  ]
}
```

- 接口要求登录，以便按照当前用户权限计算 `allowed`。
- 操作出现在 `operations` 中表示当前服务端支持；CLI 本地存在但响应中没有的操作视为服务端不支持。
- `revision` 表示单项操作协议修订号。CLI 只在自身支持该修订号时发送业务请求。
- `allowed` 表示查询时当前用户是否可以使用；`denial_code` 只返回稳定的拒绝类型，不泄露内部权限配置。
- 服务端执行业务请求时必须重新检查用户状态和权限。能力响应只能用于提前提示，不能代替鉴权。
- `registry_revision` 用于识别操作登记表是否变化和协助排查问题，不要求 CLI 仅凭摘要决定兼容性。
- `min_cli_version` 用于阻止已知无法安全调用当前服务端的旧 CLI；普通新增操作仍以 `operation_id` 和 `revision` 判断。

### 3. 风险确认

风险操作按操作登记表中的等级处理，不根据 HTTP 方法推断风险。

#### R1

- R1 使用布尔型 `--confirm`，并携带幂等键。
- TTY 中未提供 `--confirm` 时，CLI 可以显示变更预览并询问用户；确认后继续执行。
- 非 TTY 中未提供 `--confirm` 时，只返回预览或结构化确认要求，不改变远端状态。

```text
stx task update 123 --name example --confirm
```

#### R2、R3

- 第一次请求只返回 `impact` 和 `next_action`，其中包含单次使用、短期有效的确认编号，不执行目标操作。
- 第二次请求通过 `--confirmation-id <id>` 携带确认编号。
- TTY 中可以在用户确认后由 CLI 自动发起第二次请求；非 TTY 永不弹出交互提示。
- R3 还要求管理员权限，不能通过 `--yes` 或布尔型 `--confirm` 跳过服务端确认。

```text
stx cluster stop 12 --confirmation-id <id>
```

确认编号绑定用户、CLI 令牌、`operation_id`、目标和规范化后的请求参数。任一内容变化、编号过期或已经使用时，服务端拒绝执行并要求重新申请。确认编号放在结构化 `next_action` 中，不写入 `next_command`。

### 4. 幂等与资源版本条件

#### 幂等

- R1、R2、R3 请求都携带 `Idempotency-Key`。CLI 默认生成，也允许通过 `--idempotency-key <key>` 指定。
- 服务端以“用户、`operation_id`、幂等键”查找记录，并比较规范化请求摘要。
- 相同键和相同请求在执行中时返回原执行编号及当前状态，执行结束后返回原结果或结果引用，不重复执行。
- 相同键对应不同请求时返回 HTTP 409 和 `idempotency_conflict`。
- R2、R3 从申请确认编号开始就复用同一个幂等键，确认编号也绑定该键；CLI 自动重试不得生成新键。
- 运行中的记录不清理，结束记录保留 24 小时。幂等记录过期不影响审计记录的长期保存。

#### 资源版本条件

- 可修改资源在查询结果中返回资源 `revision`，并可通过响应 `ETag` 表达同一版本。
- CLI 对更新、删除以及依赖资源当前状态的执行操作提供 `--revision <n>`，通过请求 `If-Match` 发送给服务端。基于刚读取对象生成修改时，CLI 可以自动带上该版本。
- 服务端发现版本已经变化时返回 HTTP 409 和 `revision_conflict`，不执行目标操作。调用者需要重新查询后再修改；R2、R3 还需要重新申请确认编号。
- R2、R3 的确认编号绑定资源版本，避免确认预览完成后仍用旧状态执行。
- 能力响应中的 `operations[].revision` 是操作协议修订号，资源对象的 `revision` 是资源版本，不能混用。
- 首版不要求现有网页调用全部改用 `If-Match`。没有可靠资源版本的旧接口需要在操作登记表中说明限制，并明确 CLI 是否允许调用。

幂等键用于防止同一请求因重试而重复执行；资源版本条件用于防止基于旧数据覆盖或操作已经变化的资源，两者都需要保留。

### 5. 异步执行与真实取消

各模块保留自己的任务表、步骤和结果结构，另提供公共执行视图。启动异步操作后统一返回不透明的 `execution_id`，CLI 提供：

```text
stx execution get <execution-id>
stx execution wait <execution-id>
stx execution cancel <execution-id>
```

公共执行数据建议包含：

```json
{
  "execution_id": "exec_example",
  "operation_id": "diagnostics.resource.execute",
  "status": "running",
  "cancellable": true,
  "progress": 35,
  "module_ref": {
    "type": "diagnostic_resource",
    "id": "123"
  }
}
```

`progress` 和 `module_ref` 属于执行数据，不放入通用 `result_meta`。`module_ref` 只暴露允许当前用户访问的业务引用。

执行归属使用用户编号：

- 公共执行记录保存 `owner_user_id` 和 `actor_type`，其中 `actor_type` 至少区分 `user` 与 `system`。
- CLI 令牌编号、命名空间、机器标识和客户端版本只进入审计，不作为所有权字段。
- 同一用户换用自己的另一枚有效令牌或另一台机器后，仍能查询、等待和取消自己的执行。
- 每次请求仍检查用户当前状态和权限。用户被禁用或失去相关权限后，旧令牌不能继续操作执行。
- 普通用户不能管理其他用户的执行；管理员按权限查看和管理全部执行。系统执行不伪装为普通用户执行。

公共状态为：

- `pending`：已经创建，尚未开始实际执行。
- `running`：实际执行正在进行。
- `cancel_requested`：取消请求已经持久保存，尚未确认实际执行者收到。
- `cancelling`：实际执行者已经收到取消请求，正在停止、清理或恢复。
- `cancelled`：实际执行已经停止，必要的取消收尾已经达到允许结束的状态。
- `succeeded`、`failed`、`timed_out`：其他终态。

取消规则：

- 取消是单独登记的操作，具备权限、风险、确认、幂等和审计规则。
- 服务端先以条件更新写入 `cancel_requested`，再向后台任务、SeaTunnel、Agent 或本地进程发送停止请求。
- 只有收到实际停止确认后才能写入 `cancelled`；停止过程中使用 `cancelling`。
- 当前执行阶段不能停止时返回 `cancellable: false` 和稳定原因。执行已经结束或进入不可逆阶段时返回 `too_late_to_cancel` 及当前真实状态。
- 多次取消复用原取消结果，不重复发送停止命令。完成与取消同时发生时，通过合法状态迁移决定唯一终态，迟到回调不能覆盖终态。
- 安装、升级、Dump 等操作需要在执行数据中返回清理或恢复状态。相关步骤尚未结束时保持 `cancelling`，不能先返回 `cancelled`。
- 暂时没有真实停止能力的模块不提供可成功执行的取消命令，公共数据返回 `cancellable: false`。

取消风险按取消动作自身的影响登记：

- `execution get`、`execution wait`：R0。
- 取消普通文件下载、停止不会改变目标服务状态的证据采集：R1，使用 `--confirm`。
- 停止 SeaTunnel 作业或会影响目标服务的普通 Agent 操作：R2，使用服务端确认编号。
- 中断安装、升级、回滚等可能留下不完整状态的操作：R3，要求管理员权限和服务端确认编号。
- 当前阶段不能安全停止时返回 `cancellable: false`，不签发确认编号。

取消动作不直接继承原始操作的风险等级。例如启动下载和取消下载可以是不同等级；中断升级则需要按照中断本身可能产生的状态处理。

`wait` 可以在内部使用模块事件流或轮询，但 stdout 仍只写最终结果，进度事件写 stderr。公共执行视图不替换模块自己的日志、步骤和结果接口。

#### 调试工作台任务与执行

调试工作台使用与公共执行相同的用户归属原则，并补充任务定义的访问规则：

- 任务定义保存稳定的 `owner_user_id` 和访问范围。新建任务默认私有，所有者可以显式改为共享。
- 私有任务只有所有者和管理员可以查看、编辑、删除、发布和运行。
- 共享任务允许具备工作台使用权限的用户查看和运行；编辑、删除、发布、修改共享范围和修改调度配置仍只允许所有者和管理员执行。
- 任务版本继承任务定义的访问规则，版本接口不能形成绕过入口。
- 预览、提交和恢复产生的执行使用实际操作用户作为 `owner_user_id`。共享任务只共享任务定义，不自动共享执行记录和运行历史。
- 普通用户只能查询和管理自己的执行；管理员按权限查询和管理全部执行。CLI 令牌、机器和命名空间不参与所有权判断。
- 服务尚未部署，因此首版直接使用新的数据库字段和权限检查，不实现旧任务迁移、默认所有者推断或兼容旧行为的临时分支。

#### 工作台全局变量

全局变量不能继续作为所有登录用户都能使用的公共凭据池。当前代码虽然保存 `CreatedBy`，但变量键全局唯一，列表、修改和删除没有所有者检查，任务运行时还会加载全部变量原值。首版采用以下规则：

- 变量保存 `owner_user_id` 和访问范围，默认范围为私有；所有者可以显式改为共享。
- 私有变量只能被所有者的任务和管理员任务引用；共享变量允许有工作台使用权限的其他用户任务引用。
- 变量列表和详情只返回当前用户有权查看的变量。秘密变量无论调用者是谁都只返回掩码，管理员也不能通过 API、CLI、日志或任务结果取回明文。
- 修改、删除和修改共享范围只允许变量所有者或管理员执行。共享变量代表允许其他用户任务使用该凭据，修改共享范围时应显示影响说明并按有状态操作的确认规则处理。
- 任务运行时按任务所有者和变量访问范围筛选变量，不能先加载全部原值再在输出阶段脱敏。
- 变量引用、任务、实际发起用户和结果状态进入审计；审计只保存变量标识和脱敏信息，不保存变量原值或可逆密文。
- 服务尚未部署，因此直接使用新的所有者和访问范围字段，不设计旧变量迁移或把历史变量自动判为共享的兼容分支。

#### 审计记录访问

当前审计日志和 Agent 命令日志接口只使用登录检查，任意登录用户都能查询全部列表和详情。CLI 与诊断能力扩大后，审计中会包含更多目标、参数、结果引用和执行关系，不能继续采用这一范围。

- 普通用户只能查看自己发起或归属于自己的 API、CLI、Agent、诊断和任务执行记录；管理员按权限查看全部记录。
- 系统自动执行默认只对管理员可见。如果自动执行明确关联某个用户拥有的任务或公共执行，该用户可以查看该关联范围内的记录。
- 审计事件只追加，不提供修改或按条删除的产品接口。Agent 命令记录可以更新真实执行状态，但重要状态变化需要另外写入不可修改的审计事件，避免最终状态覆盖过程证据。
- 审计过期数据只允许服务端保留策略批量清理，清理动作本身需要留下可查询记录。
- 首版不提供审计导出 API、文件下载入口或 `stx audit export` 命令，只提供分页列表和单条详情查询。
- 审计查询不得返回认证令牌、Session Cookie、密码、密钥、变量原值、完整任务密文或未受限制的 Agent 原始输出。

### 6. Skill 命令

只维护一份源文件 `SKILL.md`，由 `stx` 二进制随包提供。建议命令如下：

```text
stx skill show
stx skill status [--target all|claude|agents]
stx skill install [--target all|claude|agents]
stx skill update [--target all|claude|agents]
stx skill backup list
stx skill restore <backup-id> [--target all|claude|agents]
```

- 首版只管理 `~/.claude/skills/stx` 和 `~/.agents/skills/stx` 两个安装目录，不支持自定义安装目录。其他工具需要使用时，可以通过 `show` 查看内容后手工复制。
- `show` 将随包的 Skill 内容原样写到 stdout，不在内容前后添加说明文字。
- `status` 显示源文件版本、内容摘要，以及每个目标的缺失、当前、过期或无效状态。
- `install` 只安装缺失目标；`update` 安装缺失目标并替换内容不同的目标，内容相同时不改动。
- 更新已有目标前先备份；写入使用临时目录和原子替换，并用本地锁避免两个 `stx` 进程同时修改同一目录。
- `restore` 根据备份编号恢复，恢复前再次备份当前状态。
- 目标目录、备份清单和状态收据中都不保存令牌或其他敏感信息。

这组命令只维护一份源文件。安装到多个 AI 工具目录时产生的是受摘要校验的副本，不是多份需要分别编辑的 Skill。

### 7. 全局输出参数

- `--output json|table|yaml|raw` 是正式写法。
- `--format` 和 `-f` 作为兼容别名，行为与 `--output` 完全相同，方便熟悉 dp-cli 的用户迁移。
- stdout 只承载最终业务结果。AI Agent 和脚本使用 `--output json` 时，stdout 必须是一个完整 JSON 值，不能夹杂进度、警告或说明文字。
- stderr 承载进度、警告、影响提示、结果处理事件和错误；成功命令可以产生警告事件，但退出码仍为 `0`。失败命令不向 stdout 写业务结果，在 stderr 写出最终错误并返回非零退出码。
- 人工终端可以使用 `table` 或 `yaml`；`raw` 只用于查看服务端原始响应，不绕过脱敏、权限和审计。

### 8. `--pick`

- 所有返回列表或对象数据的普通命令都支持全局 `--pick field1,field2`。
- 对列表结果，字段选择作用于每一行的顶层字段；`result_meta`、影响说明、请求编号等协议字段必须保留。
- 对单对象结果，字段选择作用于 `data` 的顶层字段；不能通过 `--pick` 读取被脱敏或被权限隐藏的字段。
- 字段不存在时返回未裁剪的安全结果，并在 stderr 输出 `pick_fallback` 事件，事件中列出缺失字段。这样不会因为字段名拼错而得到看似完整但实际为空的结果。
- `--pick` 不支持任意 JSONPath，避免命令协议被复杂表达式拖慢，也避免把内部结构暴露为稳定接口。

### 9. 输出样例和结果元数据

操作登记表中的每个可调用命令都维护脱敏的 `example` 和 `output_example`。`help` 展示它们，CI 校验样例仍符合命令的输出结构。

普通 JSON 结果采用统一外层结构，字段名遵循 STX 现有的下划线命名约定：

```json
{
  "api_version": "v1",
  "operation_id": "diagnostics.resource.list",
  "request_id": "req_example",
  "data": [
    {
      "code": "metrics",
      "risk": "R0"
    }
  ],
  "result_meta": {
    "complete": false,
    "reason": "当前只返回第一页",
    "next_command": "stx diagnostics resource list --page 2 --page-size 20 --output json"
  }
}
```

`result_meta` 只统一定义以下三个字段：

- `complete`：必填，表示 stdout 是否已经包含当前查询条件对应的完整结果集合，而不只是当前页或当前截取范围。
- `reason`：仅在结果不完整且需要解释原因时返回。
- `next_command`：仅在存在安全、明确的后续读取动作时返回。

完整结果只需要返回 `{"complete": true}`。`mode`、`returned_count`、`total_count`、`has_more`、`page`、`page_size` 和 `limit` 不进入通用元数据；具体命令确实需要数量或分页信息时，将它们放到该命令的 `data` 中。

文本诊断结果以脱敏后的 UTF-8 字节数为准：不超过 1 MiB 时可以在 `data` 中返回完整正文；超过 1 MiB 时只返回最多 256 KiB 的脱敏预览和产物元数据，并设置 `complete: false`、说明原因及提供下载产物的 `next_command`。Heap Dump 等二进制大型产物只返回元数据和下载命令，不生成正文预览。`raw` 输出同样不能绕过这些限制。

### 10. `next_command` 的安全边界

- 只对安全的后续查询、分页、查看详情、等待任务和下载产物生成可直接执行的 `next_command`。
- 命令由 CLI 根据当前命名空间、非敏感参数和服务端返回的资源编号生成，并进行 Shell 转义。
- 不把 Bearer 令牌、密码、密钥、一次性确认编号或配置密文写进 `next_command`。
- R2/R3 操作需要确认时，结果同时给出结构化 `next_action`，说明下一步动作、确认要求和有效期；确认编号由调用者显式传入，不通过可复制命令泄露。
- `next_command` 只表示建议的下一步，不代表该动作已经授权或一定能成功。

### 11. stdout 与 stderr

stdout 与 stderr 不互相混用，AI 可以分别保存和解析两条流：

- stdout 只输出最终业务结果；`stx skill show` 直接输出 `SKILL.md`，文件或流式命令遵守各自的专用协议。
- stderr 使用一行一个 JSON 事件。结果被截断、`--pick` 回退、出现影响提示、进度变化或命令失败时，写出稳定事件名和必要字段。
- stderr 事件只说明处理状态，不重复完整业务数据，也不改变 stdout 的格式。

建议事件包含：`progress`、`result_processed`、`pick_fallback`、`impact_warning`、`next_action` 和 `error`。事件不携带令牌、密码、密钥、配置密文和未限制大小的原始 Dump 内容。

### 12. 与操作登记表的关系

每个操作登记项增加 `output_example`、`pick_fields`、`completion_rule`、`next_command_rule` 和 `skill_topic` 等描述。CLI 命令、帮助、补全、能力查询和 `SKILL.md` 中的命令引用都从登记项校验，但 `SKILL.md` 不复制全部操作列表。

## 不直接照搬的部分

- 不把 STX 做成浏览器自动化 CLI；STX 直接调用远端 API，并通过 `stx-agent` 执行受限命令。
- 不允许任意代理任意 HTTP 方法、任意路径或任意 Shell。
- 不让客户端标识 Header 代替认证和权限判断。
- 不把用户令牌下发给 Agent；Agent 继续使用 STX 与任务绑定的受控通道。
- 不照搬 dp-cli 列表结果中的模式、数量和分页字段；STX 各命令按自身数据结构返回这些信息，通用 `result_meta` 只表达结果完整性和安全的后续读取动作。
