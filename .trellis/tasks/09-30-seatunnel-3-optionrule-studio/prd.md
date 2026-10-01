# SeaTunnel 3.0 OptionRule → Sync Studio 编辑区交互

## Goal

把 SeaTunnel 3.0 `OptionRule` 新增的 **valueConstraints（值约束）** 与 **conditionRules（条件子规则树）** 接到 STX 现有插件 schema 链路，让 Sync Studio HOCON 编辑区从「只会提示字段名」升级为「能按官方规则当场纠错、按模式收敛补全」。

## 背景（一句话）

3.0 把校验规则写进连接器的 `optionRule()`，并经 REST `/option-rules` 暴露；我们 java-proxy 已能抽旧式必填/互斥/条件必填，但还没吃透这两项新能力。

## Scope

**一期（设计对齐 + 契约落地优先）**

- 解释清楚 3.0 新特性与编辑区交互映射（见 `design.md`）
- v3 proxy：从 `OptionRule` 抽出 `valueConstraints` / `conditionRules`（含 `expressionTree`）并进入 `PluginOptionDescriptor`（或并列顶层字段）
- Go / 前端 DTO 透传
- Studio：hover 展示约束；保存前本地校验；按条件过滤补全（最小可用）

**明确非目标（一期不做）**

- 把 HOCON 编辑器改成表单向导 / 条件显隐表单
- 直连引擎 REST `/option-rules` 作为主数据源（可作对照探针，不替代 proxy）
- 2.x 集群伪实现 valueConstraints（无数据则空列表降级）
- 完整复刻 `ConditionExtension` 自定义回调（扩展类无法跨 JVM 序列化执行时，仅透传 description）

## Requirements

1. 3.x 集群上，schema 响应能带回结构化 `value_constraints` 与 `condition_rules`（字段名最终以 design 契约为准）。
2. 2.x 集群行为不变：新字段为空/缺省，旧 `required_mode` / `condition_expression` 仍可用。
3. 编辑区至少消费：
   - 值约束 → 行内诊断或保存前错误列表
   - 条件规则 / 条件必填 → 补全候选收敛 + hover 说明
4. 校验失败文案可读（中文优先），指向字段 key，不抛原始 Java 堆栈。

## Acceptance Criteria

- [x] `design.md` 用大白话 + 图说明三类规则（presence / value / nested）及 Studio 交互映射
- [x] DTO / API 契约在 design 中定稿（proxy → Go → frontend）
- [x] 一期交互优先级落地：① 值约束诊断（proxy ConfigValidator） ② schema 透传 ③ hover 增强
- [x] 2.x 无回归；3.x 无约束时前端静默降级
- [x] 相关单测覆盖：extractor/codec 样例 + 前端 formatValueConstraintHints
- [x] 二期：行内 Monaco diagnostics / 条件补全过滤

## Notes

- 数据主路径继续走 **java-proxy + SEATUNNEL_HOME**，与双代际 v3 jar 对齐。
- 引擎 REST `/option-rules` 仅作探索/对照，不作为 Studio 主依赖。
