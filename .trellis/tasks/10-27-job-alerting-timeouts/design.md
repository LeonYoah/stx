# 总体设计：作业层级告警与引擎超时治理

## 1. 分层

```
                ┌──────────────────────────── STX ────────────────────────────┐
用户/调度 ──提交/停止──▶ sync 写路径 ──HTTP──▶ SeaTunnel
                │   子任务1：结果未知 → 登记作业（pending / cancelling）+ 截止时间        │
                │                                                                       │
                │  sync 对账（读路径） ◀──HTTP── SeaTunnel                                │
                │   子任务1：按 jobId 确认；过期 → FAILED / 停止超时标记                     │
                │   子任务3：逐集群并行、止损、退避、超时可配                                 │
                │        │ 信号（非阻塞）                                                 │
                │        ▼                                                               │
                │  monitoring：SyncAlertHub → 告警阶段 → 策略匹配 → 投递                   │
                │   子任务2：任务级策略（任务设置里配）+ 集群级默认规则                     │
                └───────────────────────────────────────────────────────────────────────┘
```

## 2. 跨子任务契约

| 契约 | 提供方 | 使用方 | 内容 |
|---|---|---|---|
| 结果未知标记 | 1 | 2、前端、CLI | `submit_spec.submit_outcome ∈ {unknown, confirmed, expired}`、`submit_deadline`；`result_preview.cancel_outcome`、`cancel_deadline`、`cancel_overdue_at` |
| 过期接入点 | 1 | 2 | `onSubmitOutcomeExpired(job)`：提交过期判为 FAILED 时调用一次 |
| 停止超时接入点 | 1 | 2 | `onCancelOverdue(job)`：首次越过 `cancel_deadline` 时调用一次 |
| 错误分类 | 1 | 3 | 判断错误是否属于传输层、是否可能已经送达，两边用同一套底层判断 |
| D1 信号 | 3（或 2 先放在对账里） | 2 | `Observe(Since=首次失败时间)`、`Resolve(cluster_rule)`；如果 3 后合并，迁到 `scheduler.finish` |
| HTTP 超时 | 3 | 1 | `engine_request_timeout_seconds` 同时作用于读请求和写请求 |

## 3. 配置总览（`config.yaml` → `sync:`）

| 键 | 默认 | 子任务 |
|---|---|---|
| `submit_timeout_seconds` | 300 | 1 |
| `stop_timeout_seconds` | 300 | 1 |
| `job_reconciler_cluster_concurrency` | 4 | 3 |
| `job_reconciler_max_backoff_seconds` | 120 | 3 |
| `engine_request_timeout_seconds` | 15 | 3 |

任务级覆盖写在 `definition.submit_timeout_minutes` 和 `definition.stop_timeout_minutes` 里，取值 1 到 120，跟随任务版本。告警配置单独存在任务范围的告警策略里，**不跟随任务版本**。

## 4. 设计原则
- **没有证据就不下结论**：引擎不可达时只更新 `sync_state`，不判失败；提交过期的前提是引擎可达，并且明确返回"没有这个作业"。
- **写操作宁可多等，也不重复提交**：结果未知期间拦截重复提交，截止时间可配置。
- **告警只用现有信号**：第一期不新增任何引擎调用。
- **sync 不依赖 monitoring**：两边通过接口和非阻塞信号连接，monitoring 未启用时 sync 照常工作。
