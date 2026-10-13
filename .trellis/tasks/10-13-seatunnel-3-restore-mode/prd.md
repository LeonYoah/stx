# PRD: SeaTunnel 3.0 restoreMode（CHECKPOINT / SAVEPOINT）接入

## 背景与目标

SeaTunnel 3.0 REST `/submit-job` 新增 `restoreMode`（`CHECKPOINT` | `SAVEPOINT`）与 `restoreSourceJobId`，与旧参数 `isStartWithSavePoint` 并存。失败任务通常没有 SAVEPOINT，但可有最近完成的 CHECKPOINT；CHECKPOINT 恢复会产生**新** runtime jobId。

**用户价值**：失败任务能按官方语义从 CHECKPOINT 恢复；Savepoint 停机恢复仍可用；Studio/CLI 文案与引擎行为对齐。

## 已确认事实（仓库内）

- STX 恢复：`POST /api/v1/sync/jobs/:id/recover` → 新 JobInstance → 现网 **复用** `platform_job_id` + `isStartWithSavePoint=true`
- 未传 `restoreMode` / `restoreSourceJobId`
- java-proxy 不参与提交/恢复

## 需求

1. `engine_version >= 3.0.0`：按源状态自动选 `SAVEPOINT` 或 `CHECKPOINT` 并传官方参数
2. `< 3.0`：保持 `isStartWithSavePoint` + 复用 jobId
3. CHECKPOINT：`restoreSourceJobId=源 platform_job_id`，`jobId=STX nextJobID()`，**响应回填** `EngineJobID`
4. SAVEPOINT（3.0）：`restoreMode=SAVEPOINT`，复用源 `jobId`
5. Studio：按钮/确认文案反映自动模式；MVP **无**手动覆盖
6. CLI：更新 recover 说明，**无**新模式 flag
7. 本地模式继续 `ErrLocalSavepointUnsupported`

## 验收标准

- [ ] 2.x 行为与现网一致
- [ ] 3.0 Savepoint 停止 → SAVEPOINT 恢复成功，platform_job_id 复用
- [ ] 3.0 FAILED（有 completed checkpoint）→ CHECKPOINT 恢复成功，新 platform_job_id + `recovered_from`
- [ ] 无状态时可读 400/错误信息
- [ ] 文案区分 Savepoint / Checkpoint 恢复
- [ ] 单测覆盖 restore plan 与 submit 参数

## 非目标

- 指定历史 checkpoint 文件 ID
- java-proxy / 存储 inspect 改动
- 改集群 `checkpoint.retain-after-job-cancelled` 默认值

## 已拍板决策

1. **自动按源作业状态选模式**（不手选）
   - `SAVEPOINT_DONE` 等 → `SAVEPOINT`
   - `FAILED` / `CANCELED` / `FINISHED` 等 → `CHECKPOINT`（3.0+）
2. **MVP 无手动覆盖**
3. **版本门控** `>= 3.0.0`，不回退重试
4. **CHECKPOINT 新 jobId**：STX `nextJobID()` 预分配 + submit 响应回填

## 关联文档

- `design.md` — 技术方案
- `implement.md` — 实施顺序与验证
