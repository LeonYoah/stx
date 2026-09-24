# 告警诊断命名与规则通知页 — 技术设计

## Boundaries

| 层 | 改动 | 不改 |
|----|------|------|
| 前端展示 / i18n | 主战场 | — |
| 路由 query | 二级保留 `tab=policies`（及既有 alias）；新增 `section` | 不改 path `/monitoring` `/diagnostics` |
| 后端 API / 契约 | 无 | 规则评估、通道 CRUD、投递查询语义不变 |
| 死代码 | 删除未挂载 Panel / TaskCenter 及 barrel export | 不迁移其业务到新页以外的路径 |

## Information Architecture

```
Dock: 告警与诊断
  ModuleNav: 告警中心 | 诊断中心
    /monitoring
      tab=alerts (缺省)     → 告警事件
      tab=policies          → 规则与通知
        section=rules (缺省)
        section=channels
        section=history
    /diagnostics
      tab=errors|inspections|memories  → 错误 | 巡检 | 经验库
```

兼容：`tab=rules|integrations|notifications|history` 仍映射到 `policies`（现有逻辑保留）；进入后若来自 `history` alias 可默认 `section=history`（可选增强，实现时写入 checklist）。

## Component Structure

### Shell

- `MonitoringCenterWorkspace`：二级 Tab 文案 →「告警事件 / 规则与通知」；去掉硬编码英文 badge；副标题按 PRD 缩短。
- `DiagnosticsWorkspace`：二级 Tab →「错误 / 巡检 / 经验库」；Tab 移入 Header `actions`（与告警侧对齐）；去掉彩色图标或两侧统一无图标；去掉硬编码 badge。

### 规则与通知（重构重心）

现状：`MonitoringPolicyCenter.tsx` 单体过大（规则表 + 双通道巨型 Dialog + 投递 Dialog）。

目标拆分（文件名可微调，职责如下）：

| 组件 | 职责 |
|------|------|
| `MonitoringRulesNotificationsWorkspace`（或保留文件名、内部分段） | 读 `section`、三级分段 UI、StatPills 切换、把数据下发给子视图 |
| `AlertRulesSection` | 规则列表 + 筛选 + 新建/编辑 Dialog（从现有表单抽出） |
| `NotificationChannelsSection` | 通道列表（邮件/Webhook 合并或 type 筛选）+ 编辑 Dialog |
| `DeliveryHistorySection` | 全局投递列表（复用 `listNotificationDeliveries`，支持按规则/通道筛选） |

数据加载：继续用现有 `getAlertPolicyCenterBootstrapSafe` / `listAlertPoliciesSafe` / `listNotificationChannelsSafe` / `listNotificationDeliveriesSafe`；可在 workspace 层并行拉取，子段按需刷新。

### Dialog 分级

- 规则编辑：Medium～Large（保持现有字段，收口 `border` / header-footer / 内部滚动）。
- 通道编辑：从「Dialog 内嵌左右栏应用」改为：**列表在 section 页内**；编辑用 Large Dialog 单表单滚动（左侧通道清单不再塞进 Dialog）。
- 投递：不再弹 Dialog 当主面；细节可用 Medium Dialog 或行展开（实现选更轻的一种）。

## StatPills（按 section）

指标均由已加载列表 **客户端聚合**（无新 API）：

| section | pills（可点筛选时与列表联动） |
|---------|-------------------------------|
| rules | 全部 / 已启用 / 已停用 |
| channels | 全部 / 邮件 / Webhook / 已启用（若实现成本高可砍「已启用」） |
| history | 全部 / 成功 / 失败（映射现有 delivery status） |

对齐 `MonitoringAlertsCenter` 的 `StatPillsBar` 用法。

## URL Contract

| param | values | default |
|-------|--------|---------|
| `tab` | `alerts` \| `policies` (+ legacy aliases) | alerts（删 tab） |
| `section` | `rules` \| `channels` \| `history` | 仅当 `tab=policies` 时有效；缺省 rules（可 omit） |

`section` 在非 policies tab 时忽略并清理（replace，避免脏 query）。

## i18n

- 主改：`monitoringCenter.tabs.*`、`diagnosticsCenter.tabs.*`、`policyCenterV2` 标题/空态/列名中的「策略→规则」用户可见串。
- Dock / Module：`dock.monitoringCenter` / `diagnosticsCenter` 保持「告警中心 / 诊断中心」。
- 删除或停用无引用的旧 `policyCenter` 大段说明文案（若仍被代码引用则只改可见串，死 key 再删）。
- en.json 同步。

## Dead Code Removal

已确认仅 barrel export、无页面引用：

- `DiagnosticsTaskCenter.tsx` + `diagnostics/index.ts` export
- `MonitoringRulesPanel.tsx` / `MonitoringIntegrationsPanel.tsx` / `MonitoringNotificationHistoryPanel.tsx` + `monitoring/index.ts` exports

删除前再全仓 grep（含测试）。投递能力由 `DeliveryHistorySection` 承接。

## Compatibility & Rollback

- 深链 `/monitoring?tab=policies` 仍进入规则与通知，默认 rules section。
- 回滚：git revert；无数据迁移。
- 风险：`MonitoringPolicyCenter` 拆分易回归通道测试/保存路径——保留现有 service 调用与表单 state 机，优先搬移 UI 壳。

## Trade-offs

- **保留 `tab=policies`**：避免破坏书签/集群深链；展示层已不叫「策略」。
- **客户端聚合 StatPills**：实现快；历史 pill 在分页场景下仅反映当前已载入集——若 API 无 totals，pill 标注为当前结果集或首屏拉取足够 size（design 实现时选：history 请求较大 page size 或仅展示本页统计并在 UI 标明）。**推荐**：history 段请求时用现有 list API 的合理 page size，pill 表示「当前筛选结果」与告警事件页一致，不假装全局精确总数（除非 API 已返回 total 且可按 status 过滤——有则用服务端 filter）。
