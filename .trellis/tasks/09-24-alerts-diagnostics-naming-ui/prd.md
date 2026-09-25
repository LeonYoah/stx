# 告警诊断命名与规则通知页收口

## Goal

统一「告警与诊断」用户可见命名，消除「中心 / 策略」叠床架屋；并把「规则与通知」页收口到 `ui-conventions` 的高信噪比、零塌陷加载与紧凑工具栏规范，让用户一眼分清「看告警」与「配规则/通知」。

## Confirmed Naming (用户已拍板)

```
告警与诊断                          ← Dock（保留）
├── 告警中心                        ← ModuleNav（保留「中心」）
│   ├── 告警事件                    ← 二级 Tab：告警实例列表
│   └── 规则与通知                  ← 二级 Tab
│         ├── 告警规则              ← 三级分段（默认）
│         ├── 通知通道              ← 三级分段：邮件 / Webhook / IM
│         └── 投递记录              ← 三级分段：发送历史列表
└── 诊断中心                        ← ModuleNav（保留「中心」）
    ├── 错误
    ├── 巡检
    └── 经验库
```

配套约定：

- 「中心」只出现在 ModuleNav（告警中心 / 诊断中心）与 Dock 语义；二级 Tab 与页内实体不再用「中心」。
- 废弃用户可见「策略中心 / 告警策略中心」；告警侧实体统一称「告警规则」。
- 诊断侧「自动巡检策略」继续叫「巡检策略」，不与告警「规则」混名。
- 路由：二级 `tab=policies`（及既有 alias）保留兼容；三级 `section=rules|channels|history`，缺省 `rules`；用户可见文案不暴露 query key。
- 英文侧同步：`Alert Events` / `Rules & Notifications` / `Alert Rules` / `Notification Channels` / `Delivery History`；诊断 Tab：`Errors` / `Inspections` / `Playbooks`（或既有 memories 语义对应的短名，实现时与现文案对齐）。

## Confirmed Scope

本任务按用户选择的方案 **2**，布局力度定为 **重**：

1. **命名文案落地**：i18n（zh/en）、WorkspaceHeader 标题/副标题、ModuleNav、二级 Tab、规则与通知页内标题与空态、去掉硬编码英文 badge。
2. **规则与通知页重收口**（对照 `ui-conventions`）：
   - 去掉重复页内大标题卡；工具栏 + 单主内容区。
   - 三级分段 **告警规则 | 通知通道 | 投递记录**（方案 A，已确认）；各段独立列表工作面。
   - 引入 `StatPillsBar`（随当前三级分段切换指标；具体 pill 在 design 定）。
   - 列表零塌陷加载（骨架 / `TableLoadingBar`）。
   - 通知通道、投递记录离开「巨型 Dialog 套多 Card」主工作面；编辑仍用分级 Dialog。
   - 规则/通道编辑 Dialog：边框、Header/Footer 分隔与尺寸分级收口；**不改字段业务模型**。
3. **Chrome 对齐**：告警 / 诊断两侧二级 Tab 放置与样式一致（优先统一到 Header `actions`）。

## Confirmed Facts (仓库已核实)

- Dock 入口 `alertsAndDiagnostics` → `/monitoring`，active 覆盖 `/monitoring` 与 `/diagnostics`。
- 告警壳：`MonitoringCenterWorkspace`；二级 Tab 在 Header `actions`：`alerts` | `policies`。
- 诊断壳：`DiagnosticsWorkspace`；二级 Tab 在内容区 body：`errors` | `inspections` | `memories`。
- 「规则与通知」实现体：`MonitoringPolicyCenter`（`policyCenterV2` 文案），页内含告警规则表 + 邮件/Webhook 通道弹窗 + 投递历史弹窗。
- 当前美学偏离点（对照规范）：
  - 顶部独立 Card 仅承载标题/副标题/操作按钮，与下方列表 Card 叠床；WorkspaceHeader 下再套一层「告警策略」标题。
  - 列表初次加载用整格 Spinner，未用 `TableSkeletonRows` / `TableLoadingBar`。
  - Workspace 硬编码英文 badge（`Alerts & Policies` / `Diagnostics Center`）。
  - i18n 仍残留旧 Tab 名（策略中心、错误中心、巡检中心等）与未挂载遗留 Panel/TaskCenter（清理范围见 Open Questions）。

## Requirements

1. 所有用户可见文案按上文 Confirmed Naming 替换；中英双语一致。
2. 「规则与通知」按「重」+ 方案 A：三级分段 **告警规则 | 通知通道 | 投递记录**；各段列表 + StatPillsBar + 零塌陷加载；`section` 同步 URL。
3. 告警与诊断二级 Tab chrome 对齐（位置、字重/尺寸、有无图标策略一致）。
4. 不改变告警规则 / 通知通道的后端契约与业务行为；本任务以 IA 命名与前端展示层为主。
5. 规则/通道编辑仍用分级 Dialog，字段模型保持，仅做视觉与布局规范对齐。
6. 删除未挂载死代码（TaskCenter / legacy Panels）并收敛无引用过期 i18n。

## Acceptance Criteria

- [ ] Dock / ModuleNav / 二级 Tab / 页内标题与空态均符合定稿命名树；用户可见主文案不再出现「策略中心」「错误中心」「巡检中心」（遗留兼容路由/内部 key 除外）。
- [ ] 「规则与通知」内可见三级分段：告警规则 / 通知通道 / 投递记录；默认落在告警规则。
- [ ] 三级分段同步 URL：`section=rules|channels|history`（缺省 rules）；刷新/分享可还原分段；二级仍兼容 `tab=policies`。
- [ ] 各段有与之匹配的 StatPillsBar；无重复页内大标题卡；筛选/工具栏左对齐紧凑。
- [ ] 三段列表初次加载为骨架行；刷新不塌陷；通道/投递不再以巨型 Dialog 为主工作面。
- [ ] 规则/通道编辑 Dialog 符合分级尺寸与边框/Header/Footer 分隔规范。
- [ ] 告警 / 诊断二级 Tab 放置与样式一致；硬编码英文 badge 移除或 i18n 化。
- [ ] 未挂载 `DiagnosticsTaskCenter` 与三个 legacy Monitoring Panel 已删除，barrel 无残留 export。
- [ ] 相关前端单测 / 文案断言更新并通过。

## Out of Scope

- 合并 `/monitoring` 与 `/diagnostics` 为单路由。
- 告警规则评估引擎、通知投递后端逻辑变更。
- 重做规则/通道表单字段模型或校验规则。
- 二级 query `tab=policies` 改名（保留兼容 alias 即可）。

## Legacy Cleanup (已确认)

- 过期用户可见 i18n 主文案必改；无引用的旧 key 删除或收敛。
- 删除/停用未挂载死代码：`DiagnosticsTaskCenter`、legacy `MonitoringRulesPanel` / `MonitoringIntegrationsPanel` / `MonitoringNotificationHistoryPanel`（实现前再 grep 确认无引用）。
- 不在本任务重写其业务能力——投递历史已由「规则与通知 → 投递记录」承接。

## Open Questions

1. ~~布局力度~~ → **重**
2. ~~三级分流~~ → **A：告警规则 | 通知通道 | 投递记录**
3. ~~三级 URL~~ → **同步** `section=`；二级保留 `tab=policies`
4. ~~遗留清理~~ → **文案 + 删死代码**

规划决策已齐；剩余实现细节（StatPills 具体指标、组件拆分）写入 `design.md`。
