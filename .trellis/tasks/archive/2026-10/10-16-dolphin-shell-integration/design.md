# STX × DolphinScheduler Shell 集成设计

## 1. 边界

```
Dolphin SHELL 节点
  → stx CLI（Worker 上）
    → STX Server
      → SeaTunnel

STX 控制台推送
  → STX Server
    → Dolphin API（建/改工作流定义）
```

STX 不实现 Dolphin 插件，不回调 Dolphin 的任务状态接口。节点成败完全由 CLI 退出码决定。

## 2. 提交与等待

### 2.1 命令

```text
stx sync task submit <id> --confirm
  [--wait]
  [--timeout 6h]
  [--close-job | --close-job=false]
  [--external-ref ds:${wf_inst}:${task_code}]
  [--schedule-time "${system.datetime}"]
  [--var k=v]...
```

默认异步，保持现状。`--wait` 时 `--close-job` 默认 true。实时任务忽略 `--close-job`，`--wait` 在 `running` 结束。

### 2.2 `--wait` 修复

不要再调用 `GET /api/v1/executions/:id/wait`。CLI 循环：

1. `GET /api/v1/sync/jobs/{id}`（服务端 `GetJob` 会 `refreshJobInstance`）
2. 批任务：status ∈ {success, failed, canceled} 则退出
3. 实时任务：status = running 则退出 0；终态按结果退出
4. `--timeout` 到期：`POST /api/v1/sync/jobs/{id}/cancel`，退出 8
5. 每次间隔约 5 秒

`preview --wait` / `recover --wait` 同样改为分段等待真实业务终态，不能把等待接口超时当成成功。

### 2.3 提交正文

`POST /api/v1/sync/tasks/:id/submit` 增加可选字段（空则行为与现在一致）：

| 字段 | 作用 |
| --- | --- |
| `variables` | 覆盖本次 `custom_variables`，不写回任务 |
| `external_ref` | 写入作业；用于接管和按 ref 停止 |
| `schedule_time` | RFC3339 或 `yyyyMMddHHmmss`，作为 `ReferenceTime` |
| `close_job` | 仅文档/审计；真正的 close 在 CLI 侧 |

值匹配 `^\$\{.+\}$` 时返回 400，CLI 映射退出码 2。

`{{system.*}}`：有绑定则项目/工作流/节点取绑定；工作流实例 ID 取 `external_ref` 中的实例段；时间取 `schedule_time`。

## 3. 停作业

### 3.1 三条入口

| 入口 | 处理 |
| --- | --- |
| SIGINT / SIGTERM（Dolphin 3.3+、Ctrl-C） | CLI 捕获，`POST cancel`（带接管序号），写 done，退出 10 |
| SIGKILL（3.1/3.2、OOM） | 守护进程：管道 EOF 且无 done → 按 ref 停 |
| STX 作业页 / `stx sync job cancel` | Server 停 SeaTunnel；CLI `--wait` 看到 canceled，写 done，退出 10；不调 Dolphin |

### 3.2 守护进程

CLI 在提交前：

1. 建匿名管道
2. 两次 fork + `setsid`，子进程不在 Dolphin `pstree` 里
3. 守护进程持有读端和 `external_ref`、server、token
4. CLI 持有写端；正常终态写 `done\n` 后关闭
5. 守护进程读到 `done` 则退出；读到 EOF 且无 done 则 `POST /api/v1/sync/jobs/cancel-by-ref`，失败退避重试，上限 30 分钟，写本地日志后放弃

守护进程必须在提交请求发出前就绪，这样提交中途 CLI 被杀也能按 ref 停（作业可能尚未创建：cancel-by-ref 对不存在的 ref 视为已结束）。

### 3.3 external_ref 与接管序号

作业表增加：

- `external_ref` 字符串，索引
- `lease_generation` 整数，每次接管 +1（名字避免与已放弃的续租混淆，实现用 `attach_generation`）

规则：

- 运行中：返回该作业，`attach_generation++`，新 CLI 继续 `--wait`
- cancel_requested / cancelling：等到终态，再提交新作业
- 终态：提交新作业
- 停止请求必须带当前 `attach_generation`；不匹配则 409 忽略

Dolphin 容错重跑时工作流实例 ID 和节点 code 不变，ref 不变，因此接管原作业。失败重试是新任务实例，上一作业已终态，会提交新作业。

新增：

```text
POST /api/v1/sync/jobs/cancel-by-ref
{ "external_ref": "...", "attach_generation": N }
```

调度令牌允许调用此接口。

## 4. 认证

### 4.1 环境变量

CLI 读 `STX_SERVER`、`STX_TOKEN`，优先于配置文件。现有 `STX_USERNAME` 不变。

Dolphin 环境 `stx-cli-prod`：

```text
export PATH=/opt/stx/bin:$PATH
export STX_SERVER=https://stx.internal
export STX_TOKEN=stxs_••••
```

### 4.2 调度令牌

新表或在 `auth_cli_tokens` 增加 `kind=scheduler`、`expires_at` 可空、`bound_task_id`。

能力：`sync.task.submit`、`sync.job.get`、`sync.job.cancel`、`sync.jobs.cancel-by-ref`，且 `task_id` 必须是已绑定该 Dolphin 节点的任务。

吊销后 401。未绑定或解绑后 403，CLI 退出 4。只在页面发放/吊销，CLI 不提供删除。

## 5. 推送

### 5.1 Dolphin 连接

STX 增加调度器连接（API base URL、token、可选 project code 默认值）。推送对话框选连接。密钥脱敏，符合 UI 凭证规范。

Dolphin 客户端按版本切换：

- 3.1 / 3.2：`/dolphinscheduler/projects/{projectCode}/process-definition/...`
- 3.3+：`workflow-definitions`

写入步骤：`gen-task-codes` → 若已上线则 release OFFLINE → PUT 整图 → ONLINE → 若有 schedule 则重新 online。

PUT 前重新 GET，version 对不上则 409。

### 5.2 绑定

表 `sync_task_scheduler_bindings`：

- task_id（unique）
- kind = dolphin
- connection_id
- project_code / workflow_code / task_code
- pushed_at / pushed_by

不存上下游。解绑删行，不开内置定时（用户自己打开）。

推送成功后关闭该任务 `schedule.enabled`。

### 5.3 节点脚本

```text
# 由 STX 生成（任务 #42 orders_sync），重新推送会覆盖
exec stx sync task submit 42 --wait --confirm \
  --external-ref "ds:${system.workflow.instance.id}:${system.task.definition.code}" \
  --schedule-time "${system.datetime}" \
  --timeout 6h
```

有覆盖变量时追加 `--var`。`exec` 让 stx 直接收信号。

Worker 分组 `stx-workers`，环境 `stx-cli-prod`。Dolphin 节点超时 = CLI timeout + 10 分钟。失败重试 1 次 / 5 分钟，由用户可改节点设置。

### 5.4 推送 API 与 CLI

```text
POST /api/v1/sync/tasks/:id/dolphin/push
GET  /api/v1/sync/tasks/:id/dolphin/script
DELETE 绑定仅页面：POST /api/v1/sync/tasks/:id/dolphin/unbind
```

push body：

- `mode`: `new` | `reuse` | `update`
- `workflow_name`（new）
- `workflow_code`、`upstreams[]`、`downstreams[]`（reuse）
- `use_builtin_schedule`（new）
- `timeout`、`variables`、`worker_group`、`environment`
- `dry_run`

update 模式忽略上下游。reuse 在服务端做环检测。

CLI：`stx sync task dolphin push <id> --confirm` 以及 `dolphin script <id>`（只复制脚本，R0）。

`dolphin unbind` 不进 CLI。

## 6. 前端

- 任务设置/调度侧边栏：「外部调度」区块。未绑定：复制脚本、推送到 Dolphin。已绑定：绑定信息、更新节点、解除绑定、打开 Dolphin、内置定时禁用。
- 推送对话框：`sm:max-w-4xl`，边框 + muted footer。两种方式 pill。复用时上下游 checkbox。成环 Callout danger，主按钮 disabled。覆盖变量放折叠区。
- 再次推送：DiffView 看脚本差异，连线只读。
- 文案走 i18n zh/en 双写。

## 7. 兼容与回滚

- 不传新字段的 submit 行为与现在一致（修 `--wait` 除外：修的是明确的错误成功）。
- 无绑定则 `{{system.*}}` 仍用 STX 任务编号（现状）。
- 回滚：关掉推送入口；已推节点仍可用一期 CLI（脚本已在 Dolphin）。守护进程与 `--wait` 修复应随 CLI 二进制一起回滚。

## 8. 取舍

- 不用服务端续租：心跳不稳定，且 STX 停作业、CLI `--wait` 已经能覆盖失联以外的路径。
- 不插入连线：改图复杂度高，复用时勾上下游足够。
- 不调 Dolphin 停止 API：CLI 退出码就能让节点失败；少一条跨系统权限。
- 守护进程兜底 SIGKILL，而不是假装能捕获 `kill -9`。
