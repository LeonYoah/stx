# PRD: 集群升级界面 UI 优化

## 背景 / Background

集群升级流程包含三个阶段的界面:
1. **ClusterUpgradePrepare** - 升级准备(选择目标版本、运行预检查)
2. **ClusterUpgradeConfig** - 配置文件合并(解决配置冲突)
3. **ClusterUpgradeExecute** - 执行升级(监控步骤、查看日志)

当前实现存在多处违反 `.trellis/spec/frontend/ui-conventions.md` 的问题,影响用户体验。

## 问题分析 / Problems

### 1. 违反"零塌陷加载"原则
所有三个组件初始 `loading` 状态无骨架屏,页面直接空白,布局不稳定。

### 2. 违反"高信噪比与渐进式披露"原则
- **ClusterUpgradePrepare**: 所有字段平铺,高级选项(package_checksum、connector_names)无法折叠
- **ClusterUpgradeExecute**: 顶部 StatusCard 信息冗余(title 和 badge 重复显示相同值)
- **ClusterUpgradeConfig**: 所有配置文件平铺,无冲突文件未标识优先级

### 3. 违反"原子信息防被迫换行"原则
- **ClusterUpgradeExecute**: 节点表格的 `message` 列用 `max-w-[340px] truncate`,长文本被截断无法查看全文

### 4. 交互反馈不足
- **ClusterUpgradePrepare**: "运行预检查"按钮点击后无进度指示器
- **ClusterUpgradeExecute**: 日志自动翻页逻辑复杂,用户不知道是否还有历史日志

### 5. 状态机可视化缺失
用户无法直观看到"准备 → 配置 → 执行"三阶段流程的当前位置。

## 优化目标 / Goals

遵循 `ui-conventions.md` 规范,提升集群升级界面的视觉稳定性、信息密度控制和交互反馈质量。

## 优化需求 / Requirements

### P0: 核心体验修复(必须实现)

#### R1. 添加骨架屏加载状态
- **位置**: ClusterUpgradePrepare、ClusterUpgradeConfig、ClusterUpgradeExecute
- **实现**: 使用 `Skeleton` 组件替代空白 loading 状态
- **验收**: 页面加载时显示占位符,无布局跳动

#### R2. StatusCard 去重复信息
- **位置**: ClusterUpgradeExecute 顶部 4 个状态卡片
- **实现**: 移除 CardTitle 中的重复 Badge,仅保留文本值
- **验收**: 状态卡片信息密度降低,无冗余展示

#### R3. 消息列完整展示
- **位置**: ClusterUpgradeExecute 节点表格的 message 列
- **实现**: 添加 Tooltip 悬浮显示完整信息
- **验收**: 鼠标悬停可查看被截断的完整消息

### P1: 体验提升(建议实现)

#### R4. 三阶段流程导航
- **位置**: 三个页面顶部 WorkspaceHeader 下方
- **实现**: 添加 Steps 组件标识"准备 → 配置 → 执行"当前阶段
- **验收**: 用户可直观看到当前所处流程位置

#### R5. 高级选项折叠
- **位置**: ClusterUpgradePrepare 的 package_checksum 和 connector_names 字段
- **实现**: 使用 Collapsible 组件包裹,默认折叠
- **验收**: 界面默认简洁,需要时可展开高级选项

#### R6. 配置文件冲突标识
- **位置**: ClusterUpgradeConfig 的文件 Tabs
- **实现**: 冲突文件 Tab 添加红色角标或数字徽章
- **验收**: 用户可快速识别需要处理的冲突文件

#### R7. 预检查进度反馈
- **位置**: ClusterUpgradePrepare 的"运行预检查"按钮
- **实现**: 按钮添加 Loader 图标 + 文案变化
- **验收**: 点击后按钮显示"预检查中..."和加载动画

### P2: 高级优化(可选实现)

#### R8. 日志加载交互优化
- **位置**: ClusterUpgradeExecute 日志区域
- **实现**: 添加"加载更早日志"按钮,替代隐式分页
- **验收**: 用户可主动加载历史日志,明确是否还有更多

#### R9. 步骤/节点视图切换
- **位置**: ClusterUpgradeExecute 步骤表格和节点表格
- **实现**: 使用 Tabs 切换两个视图,释放横向空间
- **验收**: 表格有更多横向空间展示信息

## 约束 / Constraints

1. **不破坏现有功能**: 所有优化仅改进 UI 展示,不修改业务逻辑
2. **遵循 shadcn/ui 组件**: 使用项目现有的 UI 组件库
3. **保持国际化**: 新增的文案需添加到 `messages/` 国际化文件
4. **符合 spec 规范**: 严格遵循 `.trellis/spec/frontend/ui-conventions.md`

## 验收标准 / Acceptance Criteria

### AC1: 零塌陷加载
- [ ] 所有三个页面加载时显示 Skeleton 占位符
- [ ] 加载完成后无布局跳动或闪烁

### AC2: 信息密度优化
- [ ] StatusCard 无重复 Badge 展示
- [ ] 高级选项默认折叠(如实现 R5)
- [ ] 配置文件冲突明确标识(如实现 R6)

### AC3: 原子信息完整性
- [ ] 节点消息列可通过 Tooltip 查看完整内容
- [ ] 不存在被强制截断无法查看的关键信息

### AC4: 交互反馈清晰
- [ ] 预检查按钮有加载状态反馈
- [ ] 异步操作均有明确的进行中状态

### AC5: 规范自检通过
- [ ] 对照 `ui-conventions.md` 自检无违规项
- [ ] 代码审查通过,无 lint 错误

## 非目标 / Non-Goals

- 不重构配置合并的底层逻辑
- 不修改升级任务的执行流程
- 不添加新的业务功能(如回滚、暂停等)
- 不改变现有的路由结构

## 参考 / References

- `.trellis/spec/frontend/ui-conventions.md` - UI 美学规范
- `frontend/components/common/cluster/upgrade/` - 当前实现
- `frontend/components/ui/` - shadcn/ui 组件库
