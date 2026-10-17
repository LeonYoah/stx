# 集群作业纳管：工作台调试 vs 调度生产 PRD

## 1. 目标与用户价值

**真实产品形态（2026-10-15 对齐）**：

- STX 工作台 **终归只负责脚本编写、单次提交与调试**。
- **生产触发**交给外部 **调度引擎**（Airflow / DolphinScheduler / 内部调度等）按计划提交到 **同一 SeaTunnel 集群**。
- 调度提交后，作业 **不再经过** `POST /api/v1/sync/tasks/:id/submit`，因此 **不会** 产生 `JobInstance`。
- 现有「运行 / 停止 / Savepoint / Checkpoint / 恢复」UI 都挂在 **工作台当前任务的实例列表**（`listJobs?task_id=`）上；调度作业进不了这张表，这些能力看起来就「没有意义」。

**问题本质**：不是 Engine 做不了 stop / savepoint / CK，而是 STX **把运行面能力绑死在「自己提交时记账」上**。`EngineClient` 的 `StopJob` / `GetJobInfo` / Checkpoint 接口 **只认 `engine_job_id`**，与谁提交无关；缺的是 **集群级作业真相面** 和（可选的）**调度提交契约**。

**目标**：让 STX 成为纳管集群上 **所有 Engine 作业的控制面**，即使触发权在调度引擎：

1. **看见**：集群上正在跑 / 近期结束的作业（含调度提交、脚本直连、STX 调试）。
2. **管控**：对任意已知 `engine_job_id` 做停止、Savepoint 停止、CK 查询、日志/指标（权限 + Execution 审计）。
3. **可选溯源**：调度若走 STX 提交网关或约定 jobId，则能回挂 Task 版本，恢复/脚本才完整。

**非目标（本阶段）**：用 STX 内置 cron **替代** 企业调度引擎；把多任务运行史塞进「某一个脚本任务」的工作台列表。

---

## 2. 已确认事实（来自代码库）

| 事实 | 依据 |
|------|------|
| 工作台运行列表按 **当前 `task_id`** 拉 `JobInstance`，不是集群全量 | `DataSyncStudio.loadJobs` → `listJobs({ task_id })` |
| `JobInstance.task_id` **NOT NULL**，模型假设「每次运行都属于一个 STX 任务」 | `internal/apps/sync/model.go` |
| Engine 运行态操作（`GetJobInfo` / `StopJob` / CK / logs）只依赖 endpoint + `jobId` | `engine_client.go` |
| 提交时 STX 可指定 `jobId`（`platform_job_id` 写入 Engine） | `Submit` query `jobId` |
| STX 已有任务内 cron（`task_schedule.go`），这是 **工作台定时**，不是企业调度对接 | 与 Airflow 等无关 |
| `EngineClient` **无** running/finished 作业列表 | 接口未封装 |
| 集群已能代理 SeaTunnel WebUI（Engine 自己的作业页），但无 STX 权限/审计/与 Task 关联 | `cluster/webui_proxy_rewrite.go` |
| Grafana `job_count` 已是 Engine 维度，含调度作业 | observability dashboards |

**能力拆分（不要混为一谈）**：

| 能力 | 真正依赖 | 调度直连 Engine 后 |
|------|----------|-------------------|
| 停止 / Savepoint 停止 | `engine_job_id` + Engine REST | **可纳管**（需发现或传入 jobId） |
| 作业状态 / 指标 / job-info | 同上 | **可纳管** |
| CK overview / history（REST V2） | 同上 | **可纳管** |
| 作业日志 | Engine logs API 或集群日志模式 | **大部可纳管** |
| 工作台「本任务运行史」 | `JobInstance.task_id` | **不应混入** 调度作业 |
| 按 Task 版本恢复（rebuild 脚本再 submit） | Task + version + savepoint | **仅当能关联 Task** 或调度走网关 |
| DAG / 提交脚本回看 | `submit_spec` / `result_preview` | 无台账则 **弱**（Engine 或有 jobDag） |
| Execution 审计「谁点的运行」 | STX submit 当时的 actor | 调度直连则 **无**；STX 上的 stop 仍可审计 |

---

## 3. 产品分层（已收敛）

```
┌─────────────────────────────────────────────────────────┐
│  工作台（定义面）                                          │
│  写脚本 / 校验 / 预览 / 单次调试提交                         │
│  底部列表 = 本 Task 的 JobInstance 历史                     │
│  不展示、不合并「别的任务」或「调度作业」                      │
└─────────────────────────────────────────────────────────┘
                         │ 调试通过后，定义交给调度引用
                         ▼
┌─────────────────────────────────────────────────────────┐
│  调度引擎（触发面）                                        │
│  按 DAG/cron 决定「何时跑」                                 │
│  提交目标：Engine 直连  或  STX 提交网关（见 D0）            │
└─────────────────────────────────────────────────────────┘
                         ▼
┌─────────────────────────────────────────────────────────┐
│  集群作业控制面（运行面）← 本任务要建的                      │
│  入口：集群详情「运行作业」                                  │
│  数据：Engine 列表 ⊕ STX JobInstance 按 engine_job_id 关联 │
│  操作：停 / SP / CK / 日志 / 指标（对任意 Engine 作业）      │
└─────────────────────────────────────────────────────────┘
```

先前 Canvas「一张表混进工作台」**作废**；合并只发生在 **集群运行面**。

---

## 4. 纳管路径（D0，待确认）

### 路径 A — Engine 真相面（发现 + 管控）

调度 / 脚本仍 `submit-job` 到 Engine。STX **列出** 集群作业并代理运行态 API。

- **立刻保住**：停、Savepoint 停、CK、日志、指标。
- **保不住**：每次调度运行自动进某 Task 的历史、完整「按版本恢复」除非能匹配 jobName/jobId。
- **工作量**：Engine 列表适配 + 集群 UI + 按 `engine_job_id` 的写操作 + CLI。

### 路径 B — 提交网关（调度必须走 STX）

调度调 `POST /sync/tasks/:id/submit`（或专用 runtime submit），**禁止**（或弱约束）直连 Engine。

- **立刻保住**：每次生产运行都是 `JobInstance`，现有恢复/脚本/审计全套可用。
- **保不住**：无法强制已存在的「直连 Master」提交，除非再做网络拦截。
- **工作量**：调度侧改造 + 稳定 API/CLI 契约 + 幂等 jobId；对接每个调度产品。

### 路径 C — 分阶段（建议）

1. **先做 A**：运行面不依赖调度改代码，值班能管集群上所有作业。
2. **再提供 B 作为官方契约**：调度「发布到生产」时走 STX，换完整溯源；A 继续兜底漏网直连。

---

## 5. 范围边界

### 5.1 建议纳入（A / C 第一期）

- 集群维度 Engine 作业列表 + 与 STX 实例关联（`source=stx|external|scheduled_unlinked`）。
- 对 **任意** Engine 作业：详情、指标、日志、CK（V2）、停止、Savepoint 停止（R2 + Execution）。
- 工作台保持单任务列表；可选横幅：「本任务目标集群另有 N 个未登记作业 → 集群运行面」。
- CLI：`stx cluster jobs …`（或等价），登记 operation。
- 2.x / 3.x Engine API 适配（design 阶段实测 running-jobs / overview）。

### 5.2 建议二期（B）

- 对外稳定「调度提交」API：task 发布版本 + 变量覆盖 + 指定 jobId + 返回 engine_job_id。
- 文档：Airflow HTTP / 通用 curl 示例；不绑定单一调度品牌（除非产品指定）。
- 可选 jobId / jobName 约定，便于 A 面自动挂回 Task。

### 5.3 明确不做

- 把调度作业行塞进工作台「当前脚本」运行列表。
- 用 STX cron 替换企业调度。
- 首期在网关/防火墙层拦截 Engine REST（另立强管控项目）。
- 无 Task 关联时伪造完整版本恢复。

---

## 6. 非功能与规范约束

- `.trellis/spec/backend/index.md`、`frontend/index.md`、`cross-layer-thinking-guide.md`。
- 新路由登记 + CLI；写操作 R2、`--confirm`、幂等键。
- 注释中英双语，中文在前。
- Engine 列表 API 版本差异必须在 design 实测后再锁字段。

---

## 7. 开放决策

| # | 决策 | 状态 |
|---|------|------|
| **D0** | 纳管路径：**A** 发现面 / **B** 提交网关 / **C** 先 A 后 B | **待确认（当前最高优先级）** |
| D1 | 第一期运行面是否含 停止 + Savepoint 停止 + CK（建议：含，否则与「管控」不符） | 待 D0 |
| D2 | 运行面入口：集群详情 Tab（建议） | 倾向确认 |
| D3 | 未关联 Task 的作业：谁可停（建议：管理员 / 集群运维，R2） | 待确认 |
| D4 | 列表刷新：按需拉取 + 短 TTL（建议）；不做常驻全量落库 | 待确认 |
| D5 | UI：工作台不合并；集群页 Engine 视角表 | **已收敛** |

---

## 8. 验收标准（草案，随 D0 定稿）

### 8.1 运行面（路径 A / C-1）

- [ ] 集群详情可列出 Engine 当前运行（及约定范围内的结束）作业；STX 调试实例与未登记作业可区分。
- [ ] 对未登记作业可：查看 job-info、CK（V2 可用时）、日志、停止、Savepoint 停止；操作进 Execution/审计（cluster_id + engine_job_id）。
- [ ] 工作台 `JobRunsPanel` 仍仅 `task_id` 过滤，不出现其他任务或裸 Engine 行。
- [ ] Engine 列表 API 不可用时稳定错误，不拖垮集群页。
- [ ] CLI + operation 登记完整。

### 8.2 提交网关（路径 B / C-2）

- [ ] 调度可用文档化 HTTP/CLI 提交已发布 Task 版本，产生 `JobInstance` 且工作台该任务历史可见。
- [ ] 同一 jobId 幂等；失败原因可回传调度。

---

## 9. 风险

- Engine 列表路径在 2.3 / 3.x 不一致，需 adapter。
- 仅 jobName 关联会碰撞；优先 STX 写入的 `jobId`。
- 调度直连时「谁触发的」在 Engine 侧通常没有，STX 只能审计 **自己发出的管控动作**。
- WebUI 代理 ≠ 纳管：无权限模型、无与 Task 的官方契约。

---

## 10. 相关与记录

- `09-20-cli-workbench-sync`、`09-13-stx-cli-agent-entry`。
- 2026-10-13：按「外部作业可见性」立项。
- 2026-10-15：目标改为 **调度接管生产后的集群作业全面纳管**；否定工作台混表。
