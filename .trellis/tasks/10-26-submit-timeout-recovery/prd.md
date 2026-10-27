# 提交与停止超时：结果未知时交给对账确认，超时可配置

> 父任务：`10-27-job-alerting-timeouts`。本任务给父任务的场景 A2（提交超时）和 C1（停不下来）提供状态和截止时间；告警本身由子任务"作业告警"负责。

## Goal

SeaTunnel 集群压力大时，`submit-job` 和 `stop-job` 可能超过 STX 的 15 秒 HTTP 超时，引擎通常仍在继续处理。现在 STX 的处理有两个问题：

- **提交超时**：当作普通失败，不留作业记录（只有 Dolphin 触发的会留一条 FAILED），引擎里多出一个 STX 不知道、也停不掉的作业。重试时会分配新的 jobId，**同一份数据会被两个作业同时写**。内置定时靠"窗口内是否已有记录"去重，没有记录就会在下一分钟再提交一次。
- **停止超时**：状态恢复成运行中，并提示"停止请求失败"。但引擎可能其实已经在停了。停止中的作业也没有时间上限，会一直卡着。

目标：
- 结果不确定时，先登记作业，标成"确认中"，由对账按 jobId 去引擎核实。确认之前，不接受同一任务的新提交。
- 提交超时、停止超时都有明确的截止时间，**默认 5 分钟，全局可配，任务可以单独覆盖**。

## Decisions

- 2026-10-26 用户：先单独修提交超时；逐集群并行对账、退避、超时可配置这几项另做（已拆为父任务下的子任务"引擎查询韧性"）。
- 2026-10-27 用户：提交超时**默认 5 分钟，可自定义**，采用**全局默认 + 任务覆盖**；停止超时同样处理。
- 采用"结果不明再登记"：只在引擎调用结果拿不准时写待确认记录，不做"先登记再提交"（取舍见 `design.md` §2）。
- 不新增表或列，标记写在 `submit_spec` / `result_preview` 里。
- 截止时间在登记时就算好并写入记录。之后改配置或改任务，只影响新的提交和停止，不影响已经在确认中的作业。

## Requirements

### R1 区分"提交失败"和"结果未知"
- **结果未知**：请求可能已经到达引擎，但没拿到回复。
  - HTTP 客户端超时，或 ctx 到期、被取消。
  - 连接建立后断开（EOF、connection reset、broken pipe）。
  - HTTP 504。
  - 读响应体中途出错。
- **确定失败**，行为和现在一致：
  - 连接被拒绝、DNS 解析失败、TLS 握手失败。
  - 除 504 以外的非 2xx 响应。
- 结果未知时**不改走 REST v1 再提交一次**；确定失败时，v1 回退保持不变。
- 2xx 但响应体解析失败时，视为提交成功，jobId 用请求里的那个。

### R2 提交结果未知时登记待确认作业
- 写一条作业记录：
  - `status=pending`，`engine_job_id` 等于本次提交用的 jobId。
  - 引擎地址字段按实际请求的那个端点写，保证对账能按 jobId 查到。
  - `submit_spec` 里写这几项：
    - `submit_outcome=unknown`
    - `submit_outcome_since`：UTC RFC3339
    - `submit_deadline`：since 加上实际生效的提交超时
    - `submit_timeout_seconds`
    - `submit_error`：脱敏后的原始错误
- 接口返回 200，返回体就是这条作业。
- 不登记事件通道的"45 秒内应收到状态事件"预期。
- 手动运行、CLI、Dolphin、内置定时、savepoint 恢复都走这条路径。本地执行不涉及引擎提交，不受影响。

### R3 提交不受调用方断开影响
- 浏览器关页面或 CLI 超时断开，不影响提交继续执行：调用引擎、写作业记录、绑定公共执行记录都会照常做完。
- 引擎调用本身仍受 HTTP 超时约束。

### R4 对账确认提交结果
- 引擎里出现这个 jobId，就按现有逻辑更新状态。"出现"指 running 或 finished 列表里有它，或者 job-info 返回了真实状态。同时写 `submit_outcome=confirmed` 和 `submit_outcome_at`。
- 截止时间前，job-info 只返回空壳：保持 `pending`，不按孤儿作业处理。
- 过了截止时间仍是空壳：判为 FAILED，写 `submit_outcome=expired`，`error_message` 用提交超时专用文案（中英双语，写明超时时长，并建议查看引擎日志）。
- 引擎一直不可达时只更新 `sync_state`，不判失败。
- savepoint 恢复会沿用原来的 jobId。截止时间前**不采信终态**，避免把上一轮运行的结果当成这一次的。
- 事件刷新、`?fresh=1` 和对账使用同一套确认规则。

### R5 确认前不重复提交
- 同一任务有一条在截止时间内、`submit_outcome=unknown` 的 pending 作业时，新的提交和恢复请求一律返回 **409**。提示带上那条作业的 ID 和截止时间。
- Dolphin 用同一个 `external_ref` 重试时，接管那条作业。
- 内置定时被拦截时只记日志。
- 过了截止时间不再拦截。

### R6 停止结果未知
- `stop-job` 结果未知（分类同 R1）时：
  - **不恢复成运行中**，状态保持 `cancelling`。
  - 在 `result_preview` 里写 `cancel_outcome=unknown` 和 `cancel_error`。
  - 接口返回 200 加作业，前端提示"停止请求超时，正在确认"。
- 确定失败时，行为和现在一致：恢复原状态，并提示"停止请求失败"。
- 每次进入 `cancel_requested` 时写 `cancel_requested_at`，并按实际生效的停止超时算出 `cancel_deadline`，写入 `result_preview`。这条对结果未知和正常停止都适用。
- 过了 `cancel_deadline` 作业还没到终态：状态不变，作业上显示"停止超时"。告警由子任务"作业告警"负责。

### R7 超时可配置
- 全局配置，写在 `config.yaml` 的 `sync:` 段：
  - `submit_timeout_seconds`：默认 300。
  - `stop_timeout_seconds`：默认 300。
  - 填 0 或不填都用默认值。
- 任务覆盖，写在 `task.Definition`：
  - `submit_timeout_minutes`、`stop_timeout_minutes`。
  - 为空表示用全局值，取值范围 1 到 120。
  - 跟着任务版本走，改了要重新发布才生效。
- 实际生效的值：先看本次运行用的任务快照里有没有覆盖值，有就用它，没有就用全局值。
- 前端：在任务设置侧栏加"超时"一项，包含"提交超时"和"停止超时"，占位提示显示全局默认值，遵守 `ui-conventions.md`。

### R8 前端提示
- 提交结果未知时，用警告提示代替成功提示。
- 作业行显示"提交确认中"，悬停时显示截止时间。
- 停止结果未知时，显示警告提示"停止请求超时，正在确认"。
- 过了 `cancel_deadline` 还没停下的作业，显示"停止超时"。
- 收到 409 时，直接显示后端返回的提示。
- 文案中英双语。

### R9 CLI
- `stx sync task submit` 和恢复命令收到结果未知的作业时，在 stderr 输出 warning，`--wait` 继续等。
- 停止命令收到 `cancel_outcome=unknown` 时同样输出 warning。
- 收到 409 时按冲突退出码退出。
- 确认任务的超时字段能通过现有的任务更新 CLI 写入。如果现有 CLI 不支持，就补上参数。

## Acceptance Criteria

- [ ] 模拟 `submit-job` 超时：
  - 接口返回 200，生成一条 pending 作业，带 `unknown` 和 `submit_deadline`。
  - `engine_job_id` 等于提交时用的 jobId。
  - 没有调用 v1。
- [ ] 连接被拒绝或者引擎返回 500：和现在一致；配置了 v1 时照常回退。
- [ ] 请求 ctx 被取消后，作业记录照样写入。
- [ ] 对账在 running 列表里找到这个作业：状态变成 running，标记为 confirmed。
- [ ] 截止时间内收到空壳，作业保持 pending；超过截止时间，状态变成 failed，标记为 expired，并使用提交超时专用文案。
- [ ] 任务覆盖值为 2 分钟时，截止时间按 2 分钟算；任务没有覆盖值时，按全局配置算。
- [ ] 恢复提交时结果未知，且 finished 列表里还有上一轮的终态：截止时间内状态不变。
- [ ] 截止时间内，同一任务再次提交或恢复返回 409；Dolphin 用同一个 ref 重试时接管原作业；过了截止时间放行。
- [ ] 结果未知时不登记事件预期。
- [ ] `stop-job` 超时：
  - 状态保持 cancelling，写入 `cancel_outcome=unknown`。
  - 引擎随后返回 CANCELED 时，状态变成 canceled。
  - 引擎返回确定的错误时，和现在一致。
- [ ] 每次停止都写 `cancel_deadline`。过了截止时间还没停下，作业上显示"停止超时"。
- [ ] 任务设置里可以填两个超时，留空时显示全局默认值，发布后生效。
- [ ] 前端提示和 CLI warning 符合 R8、R9。
- [ ] 以下检查全部通过：
  - `go test ./internal/...`（已知的 installer 本机失败除外）
  - 前端 `tsc --noEmit` 和 `pnpm test`
  - `check_license.py`、`check_db_compatibility.py`
- [ ] 更新 spec：`job-status-reconciler.md`、`dolphin-scheduler-shell.md`、`engine-events-and-realtime.md`。
- [ ] 在 t1 上实测：
  - 用延迟代理制造提交超时和停止超时。
  - 确认"确认中"能正常转为运行中。
  - 确认重复提交会被拦截。
  - 确认"停止超时"能正常显示。

## Out of Scope

- 告警发送（子任务"作业告警"）。
- 逐集群并行对账、退避、引擎 HTTP 超时可配置（子任务"引擎查询韧性"）。
- STX 进程恰好在提交过程中崩溃，可能留不下记录（见 `design.md` §2）。
- 超过截止时间之后才被引擎启动的作业：STX 已经判定失败，不会再跟踪。这种情况会由提交超时告警通知到人。
