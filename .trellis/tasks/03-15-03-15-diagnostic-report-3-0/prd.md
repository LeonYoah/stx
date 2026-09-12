# 诊断报告 3.0 改造

## Goal

将当前“诊断报告”从偏产物索引/证据罗列页，升级为更接近 Allure Report 风格的“结论优先、证据后置、时间线串联”的 3.0 报告页。首阶段先完成 HTML 报告的 3.0 信息架构与视觉节奏改造，不新增 AI 推理依赖。

## Requirements

### R1. 报告首页先给结论，再给证据

- 顶部必须有 Executive Summary 区域；
- 明确展示：风险等级、核心现象、影响范围、时间窗、建议动作；
- 风险等级与建议动作不能写死，应基于当前已有证据做规则生成；
- 在样本不足场景下，结论表达应保守、可解释。

### R2. 报告结构升级为 3.0 信息架构

- 报告主体按以下顺序组织：
  1. Executive Summary
  2. Key Findings
  3. Root Cause Categories
  4. Timeline
  5. Key Signals
  6. Error Analysis
  7. Runtime Config
  8. Alerts & Process Signals
  9. Attachments / Task Metadata
- 不再让“采集步骤”或“产物列表”主导页面结构。

### R3. 关键问题要有归因视图

- 借鉴 Allure categories，增加根因分类视图；
- 首期至少支持：Dependency / Configuration / Resource / Process / Unknown；
- 分类由已有错误组、进程事件、指标信号等规则映射得到；
- 用户应能快速看出“当前更像哪类问题”。

### R4. 引入时间线，把多源信号串起来

- 将错误、告警、进程事件、关键指标峰值、巡检/诊断时间点按统一时区展示；
- 时间线优先服务人工阅读，不要求首期支持复杂交互；
- 多节点场景下，事件文案中需带节点/主机标识。

### R5. 指标区从表格升级为“卡片 + 图”

- 默认重点展示 CPU / Heap / Old Gen / GC 四类关键信号；
- 每张卡需包含：状态、峰值、峰值时间、趋势图、一句话解释；
- 其余信号可折叠或降级展示；
- 图表时间轴应与报告时区一致。

### R6. 原始证据后置，但保持可钻取

- Error Analysis 保留错误组摘要、日志切片、异常链；
- Runtime Config 保留关键配置摘要、变更轨迹、配置预览、目录清单；
- Alerts & Process Signals 后置为辅助证据；
- 原始产物仍保留下载/预览能力，供 AI 和人工深挖。

### R7. 保持双语与产品化表达

- 标题、分组名、按钮、说明文案统一双语；
- 避免工程内部术语直接暴露给用户；
- 报告整体视觉应减少后台卡片堆叠感，强调“报告感”。

## Acceptance Criteria

- [ ] HTML 报告出现 Executive Summary，能在首屏表达风险等级、核心现象、影响范围、建议动作
- [ ] 报告结构调整为 3.0 顺序，原“产物索引式布局”不再主导首页
- [ ] 至少支持 5 类根因分类，并能从现有证据规则生成
- [ ] 报告存在统一时区的时间线，能串起错误/进程/告警/指标峰值
- [ ] Key Signals 升级为重点卡片化展示，不再仅是指标表格
- [ ] Error / Config / Alert / Process 证据位置后置且层级清晰
- [ ] 双语文案与现有报告风格一致，不出现明显硬编码或术语混乱

## Technical Notes

### 主要涉及文件

- `internal/apps/diagnostics/task_execute.go`
- `internal/apps/diagnostics/task_models.go`
- `internal/apps/diagnostics/normalize.go`（如需补错误分类支撑）
- `frontend/components/common/diagnostics/DiagnosticReportMockV3.tsx`（计划新增，作为视觉参考）
- 可能补充诊断报告规则/分类辅助结构

### 实施分期建议

#### Phase 1：信息架构与规则摘要
- 报告模板重组为 3.0 顺序；
- 新增风险等级规则 v1；
- 新增建议动作模板 v1；
- 新增根因分类映射 v1；
- 新增 Timeline 数据聚合。

#### Phase 2：关键指标与证据重排
- Key Signals 卡片化；
- 错误/配置区重构；
- 告警与进程信号后移；
- 报告视觉节奏收敛。

#### Phase 3：产品化收口
- 双语细节与术语收口；
- 处理多节点名称展示；
- 评估是否补充前端预览页统一入口。

### 约束

- 本阶段优先复用现有诊断任务、报告、产物链路；
- 不引入额外前端重型图表依赖；
- 不要求本阶段接入 AI 自动总结，但要给后续增强留结构位置。

### 后续增强（暂缓）

- **诊断规则外置化 v2**：将根因分类规则、建议动作模板、关键词映射从 Go 代码中抽离，改为可配置规则源（如 YAML / DB 配置），以便适配更多用户现场与 connector 场景；
- 本阶段先保持规则 v1 内置实现，优先完成报告结构与交互体验收口。
