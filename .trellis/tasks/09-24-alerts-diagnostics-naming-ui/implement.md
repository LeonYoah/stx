# 告警诊断命名与规则通知页 — 执行计划

## Checklist

### 0. Spec 自检（动手前）

- [ ] 重读 `.trellis/spec/frontend/ui-conventions.md`（StatPillsBar、零塌陷加载、筛选左对齐、Dialog 分级、高信噪比）
- [ ] 扫一眼 `.trellis/spec/guides/cross-layer-thinking-guide.md`（空值兜底；本任务无契约变更）

### 1. 命名文案（可先合，低风险）

- [ ] `zh.json` / `en.json`：二级 Tab、诊断 Tab、规则与通知页内标题/空态/按钮；「策略→规则」用户可见串
- [ ] `MonitoringCenterWorkspace` / `DiagnosticsWorkspace`：副标题、去硬编码 badge
- [ ] 更新依赖旧文案的前端测试断言

### 2. Chrome 对齐

- [ ] 诊断二级 Tab 移入 `WorkspaceHeader` `actions`，样式与告警侧一致（无图标或两侧统一）
- [ ] 确认 ModuleNav「告警中心 / 诊断中心」不变

### 3. 规则与通知重结构

- [ ] Workspace 解析/写入 `section=rules|channels|history`；与 `tab=policies` 联动；非 policies 清理 section
- [ ] 三级分段 UI + 按段 StatPillsBar
- [ ] `AlertRulesSection`：单列表卡、左对齐筛选、骨架/`TableLoadingBar`、规则 Dialog 视觉收口
- [ ] `NotificationChannelsSection`：页内通道列表；编辑 Dialog 不再内嵌双栏应用壳
- [ ] `DeliveryHistorySection`：页内投递列表 + 筛选；替换原 history 主 Dialog
- [ ] 从 `MonitoringPolicyCenter` 搬移逻辑时保持 service 调用与保存/测试通道行为不变

### 4. 死代码与 i18n 收敛

- [ ] 全仓确认无引用后删除 TaskCenter / 三个 legacy Panel 及 barrel export
- [ ] 删除确认无引用的过期 i18n key（旧 tabs.rules 展示名等按需保留 key 映射或删）

### 5. 验证

```bash
# 文案/组件相关单测（路径按改动调整）
cd frontend && npm test -- --run components/common/monitoring components/common/diagnostics

# 类型（若仓库惯例）
cd frontend && npx tsc --noEmit
```

- [ ] 手工：`/monitoring` 二级 Tab 文案；`?tab=policies&section=channels|history` 刷新还原
- [ ] 手工：诊断三 Tab 在 header；文案为 错误/巡检/经验库
- [ ] 手工：规则 CRUD、邮件/Webhook 通道保存与测试、投递列表可见
- [ ] grep 用户可见串：策略中心、错误中心、巡检中心 不应再作为主 Tab/标题

### 6. Review gate（`task.py start` 前）

- [ ] 用户确认 `prd.md` + `design.md` + `implement.md`
- [ ] `python3 ./.trellis/scripts/task.py start`

## Risky files

- `MonitoringPolicyCenter.tsx`（大拆分）
- `MonitoringCenterWorkspace.tsx` / `DiagnosticsWorkspace.tsx`
- `frontend/lib/i18n/locales/{zh,en}.json`
- barrel `monitoring/index.ts` / `diagnostics/index.ts`

## Rollback

- 单 commit 或清晰 commit 序列；出问题 `git revert`；无 DB/API 迁移。

### Checklist progress (session)

- [x] 命名文案：二级 Tab、诊断 Tab、副标题、legacy policyCenter 可见串
- [x] 去硬编码英文 badge；诊断二级 Tab 移入 Header actions
- [x] 规则与通知三级分段 + URL `section=` + StatPills + 骨架加载
- [x] 通道/投递页内列表；通道编辑 Dialog 表单化
- [x] 删除 TaskCenter / 三个 legacy Panel
- [x] `tsc --noEmit` 通过；前端单测 163 passed

剩余建议：浏览器手点深链 `?tab=policies&section=channels|history`；通道保存/测试回归。
