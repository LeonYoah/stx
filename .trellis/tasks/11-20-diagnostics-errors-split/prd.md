# 诊断中心错误面板改为列表与详情分栏（第 2 步 / 共 3 步）

## 背景

第 1 步（PR #171）已把诊断中心外壳对齐告警中心，并把巡检策略做成列表 + 常驻详情栏。本步处理「错误」面板（`DiagnosticsErrorCenter.tsx`，959 行）。

## 已确认问题（代码证据）

1. **交互形态不一致**：详情用侧滑 `Sheet`，注释仍写“告警中心同款”，实际告警中心已是列表 + `DetailHost` 常驻栏 + J/K。
2. **频次筛选只作用于当前页**：「高频 / 中频 / 低频」在前端过滤当前 20 条，`全部` 计数却是服务端总数，分页仍按全量——筛高频时第 2 页可能是空的，计数互相矛盾。后端 `/diagnostics/errors/groups` 没有频次参数。
3. **硬编码中文**（违反 i18n 规范）：`堆栈信息已复制到剪贴板`、四个筛选标签、`已有方案`、`{n} 次`、`详情`、`受影响节点与来源`、`所属集群`、`错误摘要信息`、`复制摘要`、`{n} 条记录`、`前往受影响集群`、`关闭`、`异常`。
4. **集群信息错**：详情“所属集群”取的是当前筛选集群，未筛选时显示“全局”；“前往受影响集群”只在筛了集群时出现。错误组自带 `last_cluster_id`，应按它显示集群名与跳转。
5. 复制走 `navigator.clipboard.writeText` 未处理失败，提示为硬编码中文。

## 需求

- R1 后端：错误组列表新增查询参数 `min_occurrences` / `max_occurrences`（非负整数，作用于 `occurrence_count`，闭区间；非法值 400，沿用现有参数解析错误风格）；CLI 走 operation registry，`diagnostics.error.group.list` 增加两个 `InputSpec`。
- R2 前端服务类型 `DiagnosticsErrorGroupListParams` 增加对应字段。
- R3 新错误面板（替换 `DiagnosticsErrorCenter`）：
  - 工具栏：频次分段（全部 / 高频 ≥10 / 中频 3–9 / 低频 <3，**服务端筛选，不显示分桶计数**）+ 关键字搜索（Enter 提交、可清除、`/` 聚焦）+ 刷新；切换筛选回第 1 页。
  - 列表：`TableCard`（骨架 / 错误重试 / 空态 / 分页，服务端分页 20 条）；行 = 标题+样本摘要、异常类、来源节点、频次软徽章（按热度着色）、最近出现时间、已有方案徽章；行可点击、方向键 / Enter 可达。
  - 详情：`DetailHost` 宽栏（窄屏抽屉），J/K 在当前页切换；内容 = 经验库卡片、元信息（频次 / 首次 / 最近出现 / 节点 / 所属集群名，按 `last_cluster_id` 解析）、样本摘要 + `CopyButton`、近期事件列表（可选中）、选中事件的堆栈（`ExpandableTextPanel`）；底部操作 = 前往集群（按 `last_cluster_id`）、沉淀 / 更新方案、关闭。
  - URL `group_id` 同步与深链打开保持；请求竞态保护保持。
- R4 所有可见文案走 i18n（中英）。
- R5 测试：Go（repo 频次过滤、handler 参数解析与 400）；vitest（筛选参数与回第 1 页、打开详情拉取、J/K、集群名解析）；E2E mock 模板（列表、频次筛选、打开详情、J/K 切换、一个加载失败重试样例）。

## 验收标准

- [ ] 选「高频」时请求带 `min_occurrences=10`，分页总数来自服务端，与列表一致。
- [ ] 未筛集群时详情显示错误组所属集群名，并能跳转。
- [ ] `DiagnosticsErrorCenter.tsx` 无硬编码中文可见文案（新面板同）。
- [ ] `stx diagnostics error group list --min_occurrences 10` 可用（registry 覆盖测试通过）。
- [ ] go test / tsc / vitest / eslint / translation-coverage / 新 E2E 通过。

## 不在范围

- 巡检面板（第 3 步）。
- 把 `DetailHost` 等提升到 `common/layout`：主工作区有另一会话正在改告警中心文件（AlertsPanel / monitoring.module.css），本步不动告警中心组件，继续带 `shortcut:` 复用；等那边落地后单独提一个小 PR 提升。
- 分桶计数接口。
