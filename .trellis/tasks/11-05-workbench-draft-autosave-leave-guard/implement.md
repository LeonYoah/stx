# 实现清单

## 顺序

1. 抽出 `buildTaskPayloadFromEditor` 纯函数（供当前保存与 Save&Leave 多草稿落盘）
2. `DraftSaveStatusBadge` + i18n（zh/en）
3. `WorkbenchLeaveDialog`（Stay / Save&Leave / Discard）
4. `useWorkbenchLeaveGuard`：beforeunload + 站内 `<a>` 捕获
5. 改造 `DataSyncStudio`：`draftSaveStatus`、防抖自动保存、leave 集成、关页签不误清未保存草稿
6. 单测：payload 构建、leave risk 判定辅助；`tsc` / 相关 vitest

## 验证

```bash
cd frontend && pnpm exec tsc --noEmit
cd frontend && pnpm test -- run components/common/sync/__tests__/
```

## 自检对照

- [x] AC1 防抖 1.5s，只 `updateTask`/`persistCurrentFile(false)`
- [x] AC2 共享状态，无双重成功 toast
- [x] AC3 无 `publishTask` 于自动保存
- [x] AC4/AC5 离开与 beforeunload
- [x] AC6 只读不自动保存不拦截
- [x] AC7 dirty 标记一致
- [x] `pnpm exec tsc --noEmit` + 相关 vitest / i18n coverage 通过
