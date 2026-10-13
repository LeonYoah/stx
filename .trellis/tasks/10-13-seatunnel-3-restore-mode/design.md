# Design: SeaTunnel 3.0 restoreMode 接入

## 架构边界

```
Studio / CLI
  POST /api/v1/sync/jobs/:id/recover  (无 body 模式字段，MVP)
    → sync.Service.RecoverJob
         → resolveRestorePlan(source status, engine_version)
         → engineClient.Submit(restoreMode | legacy savepoint)
    ← 新 JobInstance (run_type=recover, recovered_from, platform_job_id)
```

- **不改** java-proxy、checkpoint 文件 inspect
- **改** `engine_client.Submit`、`RecoverJob` / `submitTaskInstanceWithPayload`、Studio 恢复文案与确认提示

## 恢复计划（RestorePlan）

| 条件 | 提交参数 | STX `platform_job_id` |
|------|----------|------------------------|
| `engine_version < 3.0` 或无法解析 | `jobId=源`, `isStartWithSavePoint=true` | 复用源 |
| 3.0+ & 源状态 ∈ {`SAVEPOINT_DONE`, …savepoint 停} | `restoreMode=SAVEPOINT`, `restoreSourceJobId=源`, `jobId=源` | 复用源 |
| 3.0+ & 源状态 ∈ {`FAILED`, `CANCELED`, `FINISHED`, …} | `restoreMode=CHECKPOINT`, `restoreSourceJobId=源`, `jobId=nextJobID()` | **预分配新 ID，响应回填** |

状态 → 模式映射函数放在 `internal/apps/sync`（纯 Go，可单测），前端仅 **展示** 后端返回或镜像同一规则（用于确认 toast / 按钮 tooltip，避免双源漂移时以后端为准）。

**SAVEPOINT_DONE 判定**：`result_preview.job_status` / `status` 为 `SAVEPOINT_DONE`（与现有 `getDisplayJobLifecycleStatus` 一致）。`DOING_SAVEPOINT` 不可 recover。

## EngineSubmitRequest 扩展

```go
type EngineSubmitRequest struct {
    // ...
    StartWithSavepoint bool   // legacy 2.x / 回退
    RestoreMode        string // CHECKPOINT | SAVEPOINT | ""
    RestoreSourceJobID string
}
```

`SeaTunnelEngineClient.Submit` V2 query：

- legacy：`isStartWithSavePoint=true` + `jobId`
- 3.0 SAVEPOINT：`restoreMode=SAVEPOINT` + `restoreSourceJobId` + `jobId`（同源）
- 3.0 CHECKPOINT：`restoreMode=CHECKPOINT` + `restoreSourceJobId`（源）+ `jobId`（新）

**互斥**：设 `restoreMode` 时不传 `isStartWithSavePoint`（避免引擎歧义）。

落库：`PlatformJobID` = 本次 submit 使用的 `jobId`；`EngineJobID` = 响应 `jobId`（与现网 submit 一致）。

## 版本门控

- 使用任务 `EngineVersion`（`applyTaskDefaults` 后）；空则集群默认版本（若 resolver 已有则复用，否则 `< 3.0` 走 legacy）
- 复用 `seatunnel.CompareVersions(version, "3.0.0") >= 0`

## API / CLI

- **不新增** recover 请求字段（MVP 无手动覆盖）
- 可选：recover 响应 / 新实例 `submit_spec` 写入 `restore_mode` 便于审计与 UI 展示
- CLI `stx sync job recover`：帮助文案改为「按作业状态自动从 Savepoint 或 Checkpoint 恢复」；**不加** `--restore-mode`

## Studio UX

- 按钮文案：由一律「Savepoint 恢复」改为上下文文案，例如：
  - Savepoint 停：`从 Savepoint 恢复`
  - 失败等：`从 Checkpoint 恢复`
- 确认前 toast / 简短说明展示将使用的模式与源 `platform_job_id`（CHECKPOINT 时提示将产生新引擎作业 ID）
- 引擎 400（无 checkpoint/savepoint）映射友好错误（透传或关键字解析）

## 错误与兼容

- REST V1 fallback：仍不支持 restoreMode；3.0 集群应走 V2。若误落 V1，保持现有行为并记录 submit_spec
- `ErrLocalSavepointUnsupported` 不变

## 风险

- 集群未开 `checkpoint.retain-after-job-cancelled` 时 CANCELED → CHECKPOINT 仍可能 400：文案提示配置项，非 STX bug
- FINISHED 默认 CHECKPOINT：若用户只想重跑无状态，可能误恢复；MVP 接受（与「恢复」语义一致）
