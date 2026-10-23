# DolphinScheduler SHELL 接入契约

> 适用于根 Go 模块中的同步任务提交、CLI `--wait`/`--close-job`、管理员签发的 CLI 登录令牌，以及向 DolphinScheduler 推送 SHELL 节点。

---

## Scenario: STX 通过 SHELL 节点接入 DolphinScheduler

### 1. Scope / Trigger

- Trigger: 同步任务需要被 DolphinScheduler 调度，且停作业必须可靠；不实现 Dolphin 插件。
- 涉及：`POST /api/v1/sync/tasks/:id/submit`、作业查询/停止、Dolphin 推送/绑定、CLI `stx sync task submit|dolphin|job cancel-by-ref`，以及管理员签发的 `stx_` 登录令牌。
- 不在范围：调用 Dolphin 停止工作流实例、lease/heartbeat、CLI 删除/解绑、单独的调度令牌。

### 2. Signatures

```text
POST /api/v1/sync/tasks/:id/submit
GET  /api/v1/sync/jobs/:id
POST /api/v1/sync/jobs/:id/cancel
POST /api/v1/sync/jobs/cancel-by-ref
GET  /api/v1/sync/tasks/:id/dolphin
GET  /api/v1/sync/tasks/:id/dolphin/script
POST /api/v1/sync/tasks/:id/dolphin/push
POST /api/v1/sync/tasks/:id/dolphin/unbind          # 仅页面
```

```text
stx sync task submit <id> --confirm [--wait] [--timeout <duration>]
  [--close-job | --close-job=false] [--external-ref ds:${wf_inst}:${task_code}]
  [--schedule-time "${system.datetime}"] [--var k=v]...
stx sync task dolphin script <id>
stx sync task dolphin push <id> --confirm [--new-workflow|--workflow] [--dry-run]
stx sync job cancel-by-ref --external-ref <ref> --attach-generation <n> --confirm
```

数据库：

- `sync_job_instances.external_ref`、`attach_generation`
- `JobInstance.Mode` 是 JSON 虚拟字段（`gorm:"-"`），从 `submit_spec.mode` 回填，不落库
- `scheduler_connections`、`task_scheduler_bindings`
- Worker 使用 `auth_cli_tokens` 里管理员签发的登录令牌（`stx_`，`ExpiresAt` 可空表示永久）

### 3. Contracts

提交正文（空则行为与旧版一致）：

| 字段 | 约束 |
| --- | --- |
| `variables` | 只覆盖本次 `custom_variables`，不写回任务 |
| `external_ref` | 写入作业；同一 ref 运行中则接管并递增 `attach_generation` |
| `schedule_time` | RFC3339 或 `yyyyMMddHHmmss`，作为内置时间变量 `ReferenceTime` |
| `close_job` | 仅文档/审计；真正 close 在 CLI |

CLI 环境变量优先级：`--server`/`--token` → `STX_SERVER`/`STX_TOKEN` → 本地 namespace 文件。

Worker 令牌：

- 使用管理员在「用户管理 → 令牌管理」签发的 `stx_` 登录令牌，不要再签发 `stxs_` 调度令牌
- 预设有效期 7d / 30d / 90d / never（永久），也可自定义 1–3650 天
- 原文只在签发时返回一次；Worker 环境变量是 `STX_SERVER` + `STX_TOKEN`
- 权限与令牌所属用户一致；解绑 Dolphin 节点不吊销该登录令牌
- CLI：`stx admin token create` 签发，`stx login --token` 绑定已有令牌
- 推送不签发、不绑定、不把 `STX_TOKEN` 写入 SHELL 节点；`env_notes` 只提醒用户自行绑定。任务侧栏不得放令牌管理入口

`--wait`：

- 必须轮询 `GET /api/v1/sync/jobs/{id}`（默认读库，禁止 `fresh=1`；引擎状态由作业对账器刷新）
- 禁止把 `GET /api/v1/executions/:id/wait` 的超时当成成功
- 批任务等 `success|failed|canceled`；实时任务等 `running` 后退出 0
- 成功 0，参数 2，认证 3，无权限 4，显式 `--timeout` 到期先 cancel 再 8，失败/被停 10；`sync_state=stale` 超过 `--observation-timeout`（默认 3m）时同样以 timeout 退出并发出 `job_observation_lost`
- 不传 `--timeout` 则一直等到作业终态；时长由作业本身和 Dolphin 节点超时决定，STX 不定默认上限
- 生成的 Dolphin SHELL 脚本不得写入 `--timeout`
- 默认跟随作业日志（`--no-follow-logs` 可关）
- 作业 JSON 的 `mode` 必须能从 `submit_spec` 回填，否则 CLI `--wait` 会把实时任务当成批任务死等

`--close-job`：

- 与 `--wait` 联用时默认 true；实时任务必须关闭（脚本生成 `--close-job=false`）
- SIGINT/SIGTERM：发 cancel 后写 `done`
- SIGKILL：提交前 unix `setsid` 守护进程（不要同时 `Setpgid`，否则 `Start` 返回 operation not permitted，任务在提交前失败）；管道 `generation N` / `done`；EOF 且无 `done` 则 `cancel-by-ref` 重试最多 30 分钟
- 守护进程遇到 409（旧 generation）或 404 必须停止重试

连接：

- 创建调度器连接必须提交 Dolphin Open API Token（请求头 `token`）；列表/详情只返回 `token_set`，不回显原文。
- 页面无连接时必须直接展开带标签的 Token 表单；已有连接时 Token 仍可从「新建连接」填写。不要把它和 Worker `STX_TOKEN`（管理员签发的 `stx_` 登录令牌）混成一个字段。

推送：

- `new`：单 SHELL 节点工作流；可选把 STX cron 转成 Dolphin 定时并上线
- `reuse`：挂到勾选上下游；成环 400 且不改图；`from==0` 的起点边不参与成环
- `update`：已绑定任务强制走此模式；可省略 `connection_id`/`project_code`，从绑定回填；只改该节点脚本/设置，不覆盖已有 `timeoutFlag`/`timeout`
- 新建 SHELL 节点 `timeoutFlag=CLOSE`，不设分钟数；用户在 Dolphin 里打开的超时，再推送不得改掉
- 已上线工作流：OFFLINE → PUT → ONLINE；PUT 后上线失败必须带 “workflow is OFFLINE”
- 定时是独立资源（`/projects/{code}/schedules`，按工作流 code 关联，有自己的上下线状态），Dolphin 下线工作流会连带下线其定时：
  - reuse / update 在**下线前**读取定时并记下原本在线的集合；读取失败时中止推送（不下线、不改图），返回 “nothing was changed”
  - 重新上线后**只恢复原本在线的定时**，推送前已暂停的定时保持暂停；禁止对已有工作流调用「全部上线」（`OnlineSchedules` 只给 STX 新建的工作流用）
  - 原本下线的工作流：推送后保持下线，不上线、不碰定时（读取失败只记警告）
  - 推送结果与工作流详情返回 `schedules`（推送前状态），结果另含 `schedules_restored`；数组为 `[]` 不为 `null`
- 定时属于整个工作流：`use_builtin_schedule` 只对 `new` 生效；reuse / update 不创建、不覆盖已有定时，带该参数时返回 warning。推送弹窗必须告知上下线行为、工作流定时（含暂停）、STX cron 不沿用，以及工作流无定时时任务不会自动运行
- `expected_version` 变化 → 409，不覆盖
- 推送成功关闭 STX 内置定时；绑定期间不能再打开
- 解绑只删 STX 记录，不动 Dolphin 节点，也不吊销管理员签发的登录令牌
- 推送与内置定时一样只绑已发布版本：未发布 → 400 `ErrTaskNotPublished`；脚本取当前发布快照，不用编辑器草稿
- 同一 STX 发布版本不能再更新推送。绑定记录 `task_version`；`CurrentVersion <= binding.TaskVersion`（且已记录过版本）→ 409 `ErrDolphinPushNoNewVersion`
- 旧绑定 `task_version=0` 允许再推一次，把当前发布版本写回绑定

Dolphin 触发的提交：

- `external_ref` 形如 `ds:{wf_inst}:{task_code}` 时 `run_type=schedule`、`submit_spec.trigger_source=dolphin`
- 必须跑当前已发布快照；未发布 → 400 `ErrTaskNotPublished`
- 作业记录带 `task_version`、`external_ref`、`trigger_source=dolphin`，与手动运行、内置定时区分
- 没有 SeaTunnel 集群或引擎提交失败时，仍要留下一条 `failed` 作业，不能假装调度没调过

Dolphin 定义接口是**整图覆盖**（`taskDefinitionJson` + `taskRelationJson` + `locations` 一次 PUT 全部节点/边/坐标）。回写契约：

- GET 后任务/边必须按原始 JSON 数组保存；未改动的节点（SPARK/SQL/CONDITION 等）字节级带回，禁止类型化结构丢掉 `taskGroupId` 等未知字段。
- `locations` 同时读根字段和 `processDefinition` / `workflowDefinition` 内嵌字段；3.2 数组 `[{taskCode,x,y,...}]` 与旧版 map 都要认。
- 上线工作流 OFFLINE 后再 GET，3.2 可能清空 `locations`；update 与 reuse 都必须用下线前的副本回填，禁止把 `{}` / `[]` 写回去把画布抹掉。
- 合并新节点坐标时保留已有节点的 extra 字段（如 `targetarr`、`name`、浮点 x/y）；解析失败时保持原串。
- 绑定 SHELL 升 version 时，必须同步该 code 在边上的 `preTaskVersion` / `postTaskVersion`。
- 绑定节点若已不是 SHELL，拒绝 update，避免把 `rawScript` 写进 SPARK/SQL 参数。

数组字段（`items`/`tasks`/`relations`/`recent_jobs`）在 API 与前端 service 边界都必须是 `[]`，不能是 `null`。

DolphinScheduler **只支持 3.x**：

| 版本 | API 族 | 定义路径 | 定义 JSON | 关系 JSON | 定时字段 |
| --- | --- | --- | --- | --- | --- |
| 3.0 / 3.1 / 3.2 | `process` | `/projects/{code}/process-definition` | `processDefinition` | `processTaskRelationList` | `processDefinitionCode` |
| 3.3+ | `workflow` | `/projects/{code}/workflow-definition` | `workflowDefinition` | `workflowTaskRelationList` | `workflowDefinitionCode` |

连接可填 `dolphin_version`（如 `3.2.2` / `3.3.1`）直接选定 API 族；留空则先探 3.3 路径再回退 3.1/3.2。探测必须要求响应是 JSON；3.2 单机对 `workflow-definition` 常返回 200 HTML 控制台页，不能当成 3.3。项目列表若只有 `id` 没有 `code`（2.x）返回 `ErrDolphinUnsupportedVersion`。解码必须同时识别两套 JSON 字段，否则 3.3 复用/更新会把连线读成空。

SHELL 节点 JSON 必须带 `isCache=NO`、`taskExecuteType=BATCH`；3.2.x 缺 `isCache` 时 `transformTask` NPE，接口只回 `request parameter {0} is not valid`。默认 worker group 是 Dolphin 自带的 `default`，不要默认 `stx-workers`（真实单机没有这个组）。

控制台链接：3.0/3.1 用 `#/projects/{code}/process-definition/list/{wf}`；3.2 起 UI 已是 `/projects/{code}/workflow-definition/{wf}`（3.2.2 的 hash process-definition 会落到首页）。API 族仍按 3.2=`process`、3.3+=`workflow` 划分。

### 4. Validation & Error Matrix

| 条件 | HTTP | CLI 退出码 |
| --- | --- | --- |
| `--var` / `variables` 值匹配 `^\$\{.+\}$` | 400 `ErrUnreplacedVariable` | 2 |
| `schedule_time` 非法 | 400 | 2 |
| CLI 令牌已吊销 | 401 | 3 |
| `attach_generation` 不匹配 | 409（CLI 守护进程忽略） | — |
| 复用成环 | 400 `ErrDolphinCycle` | 2 |
| Dolphin 版本变化 | 409 | 6 |
| 已绑定节点被删 | 400 `ErrDolphinNodeMissing` | 2 |
| `--wait` 超时并已请求 cancel | — | 8 |
| 作业 `failed` / `canceled` | — | 10 |
| 绑定期间打开内置定时 | 400 `ErrScheduleLockedByDolphin` | 2 |
| Dolphin 不是 3.x / 2.x 项目列表 / 高低版本路径都不存在 | 400 `ErrDolphinUnsupportedVersion` | 2 |
| 未发布就推送或 Dolphin/CLI 提交 | 400 `ErrTaskNotPublished` | 2 |
| 同一已发布版本再次更新推送 | 409 `ErrDolphinPushNoNewVersion` | 6 |

### 5. Good / Base / Bad

- Good：Dolphin SHELL 使用生成脚本，`--wait` 轮询作业接口；Worker 环境只有用户自行绑定的 `STX_SERVER` 与管理员签发的 `stx_` 登录令牌；推送不写 `STX_TOKEN`。
- Base：未绑定任务仍可复制脚本，ref 里保留 Dolphin `${system.*}` 占位符，由 Worker 替换。
- Bad：`--wait` 打公共执行 wait；把 `JobInstance.Mode` 当成库字段；update 推送还强制前端传 `project_code`；守护进程把 409 重试 30 分钟；推送侧栏放「打开令牌管理」。

### 6. Tests Required

- `go test ./internal/cmd/ ./internal/cli/... ./internal/apps/sync/ ./internal/apps/auth/ ./internal/operation/`
- 断言：变量未替换拒绝；同一 `external_ref` 接管；旧 generation 停止被忽略；update 不改边；成环不写 Dolphin；version 冲突 409；流式脚本含 `--close-job=false`；生成脚本不含 `--timeout`；新建节点 `timeoutFlag=CLOSE`；update 保留原超时。
- 未发布推送失败；同一发布版本再次 update 409；发布新版本后才允许更新。Dolphin `external_ref` 提交使用发布快照，`trigger_source=dolphin`；引擎不可用时仍写入 failed 作业。
- Dolphin httptest：3.1/3.2 只打 `process-definition` 且定时带 `processDefinitionCode`；3.3+ 只打 `workflow-definition` 且解码 `workflowTaskRelationList`；更新 3.3 节点必须把原 `workflowTaskRelationList` 写回 `taskRelationJson`；2.x / 非 3.x 版本被拒绝。
- 整图回写：update 在 OFFLINE 后 GET 丢掉 locations 时仍要把兄弟 SPARK/CONDITION 节点、边 extra 字段和坐标 extra 写回；reuse 不得改写已有节点 JSON。OFFLINE 后再 GET 若只返回部分节点或空数组，必须用下线前副本回写，禁止把残缺图 PUT 回去。
- 定时恢复：在线工作流含「在线 + 暂停」两条定时时，update / reuse 推送后只有原在线的被上线、暂停的保持暂停；下线工作流不触发任何上线；在线工作流读不到定时则不下线（有状态 fake：下线工作流连带下线定时）。
- CLI：`stx sync scheduler connection create --dolphin-version 3.2.2` 把版本写入连接请求体。
- 前端：中英文 i18n 键齐全；service 对 `tasks`/`relations`/`recent_jobs` 做 `?? []`。任务侧栏在已绑定状态下不得出现令牌管理入口。
- 脚本：`rawScript` 不含 `STX_TOKEN`；`env_notes` 不含 `export STX_TOKEN=`，并说明推送不签发登录令牌。

### 7. Wrong vs Correct

#### Wrong

```go
_, err := client.Request(ctx, "GET", "/api/v1/executions/"+executionID+"/wait", ...)
// 把 30 秒上限超时当成提交成功
```

```go
type JobInstance struct {
    Mode string `json:"mode"` // 落库或查询时为空，CLI --wait 无法识别 streaming
}
```

```go
if req.ProjectCode == 0 {
    return nil, fmt.Errorf("%w: project_code is required", ErrDolphinPushInvalid)
}
```

#### Correct

```go
_, err := client.Request(ctx, "GET", "/api/v1/sync/jobs/"+strconv.FormatUint(uint64(job.ID), 10), ...)
```

```go
Mode string `json:"mode,omitempty" gorm:"-"`
clone.Mode = strings.ToLower(strings.TrimSpace(stringValue(job.SubmitSpec, "mode")))
```

```go
if projectCode == 0 && binding != nil {
    projectCode = binding.ProjectCode
}
if projectCode == 0 && mode != "update" {
    return nil, fmt.Errorf("%w: project_code is required", ErrDolphinPushInvalid)
}
```

---

## Design Decision: 守护进程 + attach_generation，而不是 lease

**Context**：Dolphin 3.1/3.2 对进程树 `kill -9`，JVM `-cj` 钩子不会跑。

**Options Considered**：

1. 作业 lease/heartbeat，超时由 Server 杀作业
2. CLI 提交前 `setsid` 守护进程，管道感知 CLI 死亡后再 `cancel-by-ref`

**Decision**：选 2。避免 Server 侧租约复杂度；取消以 STX 作业为第一公民，不回调 Dolphin。接管序号保证新 CLI 启动后旧守护进程的停止请求被 409 忽略。

---

## Common Mistake: GetJob 响应没有 mode

**Symptom**：实时任务 `--wait` 一直等到超时。

**Cause**：`JobInstance` 没有持久化 `mode`，调度令牌路径只返回作业行。

**Fix**：`gorm:"-"` 的 `Mode` + `sanitizeJobForResponse` 从 `SubmitSpec` 回填；CLI 提交成功后优先用响应里的 `mode`。
