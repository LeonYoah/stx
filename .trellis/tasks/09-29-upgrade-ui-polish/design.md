# Design: 集群升级界面 UI 优化

## 技术边界 / Technical Boundaries

### 改动范围
- **前端组件**: `frontend/components/common/cluster/upgrade/` 下的三个核心组件
- **UI 组件库**: 使用现有 `frontend/components/ui/` 的 shadcn/ui 组件
- **国际化**: 新增文案需添加到 `frontend/messages/zh-CN.json` 和 `frontend/messages/en.json`

### 不改动范围
- 后端 API 接口(`internal/apps/stupgrade/`)
- 数据流和状态管理逻辑
- 路由结构(`frontend/app/(main)/clusters/[id]/upgrade/`)

## 架构设计 / Architecture Design

### 组件职责分离

#### 现有职责(保持不变)
- **ClusterUpgradePrepare**: 表单输入 + 预检查调用 + session 存储
- **ClusterUpgradeConfig**: 配置对比 + 冲突解决 + 合并计划编辑
- **ClusterUpgradeExecute**: 任务启动 + 步骤监控 + 日志查看

#### 新增 UI 层职责(纯展示)
- **LoadingSkeleton**: 各组件的骨架屏布局
- **UpgradeSteps**: 三阶段流程导航组件(可选,P1)
- **MessageTooltip**: 消息列的 Tooltip 封装(P0)

### 组件依赖关系

```
ClusterUpgradePrepare
  ├─ LoadingSkeletonPrepare (new)
  ├─ AdvancedOptionsCollapsible (new, P1)
  └─ existing logic (unchanged)

ClusterUpgradeConfig
  ├─ LoadingSkeletonConfig (new)
  ├─ ConflictBadgeTabs (modified, P1)
  └─ existing merge editor (unchanged)

ClusterUpgradeExecute
  ├─ LoadingSkeletonExecute (new)
  ├─ StatusCard (modified - remove duplicate badge)
  ├─ MessageTooltipCell (new)
  └─ existing polling logic (unchanged)

UpgradeSteps (new, shared, P1)
  └─ used by all three pages
```

## 数据流设计 / Data Flow

### 无数据流变更
所有优化均为**纯 UI 层改动**,不涉及:
- API 请求/响应格式
- LocalStorage session 结构
- 状态管理逻辑(useState/useEffect)
- 轮询机制

### UI 状态扩展(局部)

#### ClusterUpgradePrepare
```typescript
// 新增状态(P1: 高级选项折叠)
const [advancedExpanded, setAdvancedExpanded] = useState(false);
```

#### ClusterUpgradeExecute
```typescript
// 无新增状态,仅修改 JSX 渲染逻辑
// StatusCard 移除 Badge 组件
// TableCell 包裹 Tooltip 组件
```

#### ClusterUpgradeConfig
```typescript
// 无新增状态,仅修改 Tab 渲染逻辑
// 冲突文件添加徽章标识
```

## 实现策略 / Implementation Strategy

### P0: 核心体验修复

#### 1. 骨架屏实现方案
**原则**: 镜像真实布局,避免过度精细化

##### ClusterUpgradePrepare
```tsx
// 骨架屏结构 / Skeleton structure
<div className="space-y-6">
  <Skeleton className="h-10 w-64" /> {/* 标题 */}
  <Card>
    <CardHeader>
      <Skeleton className="h-6 w-48" />
    </CardHeader>
    <CardContent className="space-y-4">
      <Skeleton className="h-10 w-full" /> {/* 输入框 */}
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-32 w-full" /> {/* 表格区域 */}
    </CardContent>
  </Card>
  <Skeleton className="h-10 w-32" /> {/* 按钮 */}
</div>
```

##### ClusterUpgradeExecute
```tsx
// 执行页骨架屏 / Execute page skeleton
<div className="space-y-6">
  <div className="grid gap-4 md:grid-cols-4">
    <Skeleton className="h-24 w-full" /> {/* StatusCard × 4 */}
    <Skeleton className="h-24 w-full" />
    <Skeleton className="h-24 w-full" />
    <Skeleton className="h-24 w-full" />
  </div>
  <Skeleton className="h-96 w-full" /> {/* 表格区域 */}
  <Skeleton className="h-96 w-full" /> {/* 日志区域 */}
</div>
```

#### 2. StatusCard 去重复方案
**修改点**: `ClusterUpgradeExecute.tsx` 的 `StatusCard` 组件

```tsx
// 修改前 / Before
<CardTitle className='flex items-center justify-between gap-3 text-base'>
  <span>{value}</span>
  <Badge variant={badgeVariant}>{value}</Badge> {/* 冗余 */}
</CardTitle>

// 修改后 / After
<CardTitle className='text-base'>
  {value}
</CardTitle>
```

**影响**: 仅视觉变化,逻辑零影响

#### 3. 消息列 Tooltip 方案
**修改点**: `ClusterUpgradeExecute.tsx` 节点表格的 message 列

```tsx
// 修改前 / Before
<TableCell className='max-w-[340px] truncate'>
  {node.message || node.error || '-'}
</TableCell>

// 修改后 / After - 引入 Tooltip / Add Tooltip
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';

<TableCell>
  <TooltipProvider>
    <Tooltip>
      <TooltipTrigger className='max-w-[340px] truncate text-left'>
        {node.message || node.error || '-'}
      </TooltipTrigger>
      <TooltipContent className='max-w-md'>
        <p>{node.message || node.error || '-'}</p>
      </TooltipContent>
    </Tooltip>
  </TooltipProvider>
</TableCell>
```

**注意**: 空值("-")也需包裹,避免条件渲染复杂度

### P1: 体验提升

#### 4. 三阶段流程导航(可选)
**实现**: 创建共享组件 `UpgradeSteps.tsx`

```tsx
// 新组件 / New component
// frontend/components/common/cluster/upgrade/UpgradeSteps.tsx

interface UpgradeStepsProps {
  current: 'prepare' | 'config' | 'execute';
  clusterId: number;
}

export function UpgradeSteps({current, clusterId}: UpgradeStepsProps) {
  const steps = [
    {key: 'prepare', labelKey: 'prepareStage', href: `/clusters/${clusterId}/upgrade/prepare`},
    {key: 'config', labelKey: 'configStage', href: `/clusters/${clusterId}/upgrade/config`},
    {key: 'execute', labelKey: 'executeStage', href: `/clusters/${clusterId}/upgrade/execute`},
  ];
  
  // 使用 shadcn/ui 的 Steps 组件或自定义实现
  // 渲染为横向进度条
}
```

**集成点**: 在三个页面的 `WorkspaceHeader` 下方添加

#### 5. 高级选项折叠(可选)
**修改点**: `ClusterUpgradePrepare.tsx` 的高级字段区域

```tsx
// 引入 Collapsible / Import Collapsible
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';

// 包裹高级选项 / Wrap advanced options
<Collapsible open={advancedExpanded} onOpenChange={setAdvancedExpanded}>
  <CollapsibleTrigger asChild>
    <Button variant="ghost" size="sm">
      {t('advancedOptions')}
      <ChevronDown className={cn('ml-2 h-4 w-4 transition-transform', advancedExpanded && 'rotate-180')} />
    </Button>
  </CollapsibleTrigger>
  <CollapsibleContent className="space-y-4">
    {/* package_checksum 和 connector_names 字段 */}
  </CollapsibleContent>
</Collapsible>
```

#### 6. 配置文件冲突标识(可选)
**修改点**: `ClusterUpgradeConfig.tsx` 的 TabsList 渲染

```tsx
// 冲突文件添加角标 / Add badge for conflict files
{mergePlan.files.map((file) => (
  <TabsTrigger key={file.config_type} value={file.config_type}>
    {file.config_type}
    {file.conflict_count > 0 && (
      <Badge variant="destructive" className="ml-2 h-5 w-5 rounded-full p-0 text-xs">
        {file.conflict_count}
      </Badge>
    )}
  </TabsTrigger>
))}
```

#### 7. 预检查进度反馈(可选)
**修改点**: `ClusterUpgradePrepare.tsx` 的按钮状态

```tsx
// 修改前 / Before
<Button onClick={handleRunPrecheck} disabled={runningPrecheck}>
  {t('runPrecheck')}
</Button>

// 修改后 / After
<Button onClick={handleRunPrecheck} disabled={runningPrecheck}>
  {runningPrecheck && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
  {runningPrecheck ? t('runningPrecheck') : t('runPrecheck')}
</Button>
```

**国际化**: 需添加 `runningPrecheck` 翻译键

## 兼容性与权衡 / Compatibility & Tradeoffs

### 兼容性
- **无破坏性变更**: 所有改动向后兼容
- **组件 API 不变**: 三个组件的 props 接口保持不变
- **样式隔离**: 仅修改组件内部 JSX,不影响全局样式

### 权衡分析

#### 权衡1: Tooltip vs 展开行
**选择**: Tooltip
**理由**:
- 实现成本低(shadcn/ui 现成组件)
- 不改变表格布局(避免高度跳动)
- 符合"原子信息"原则(悬停即可查看)

#### 权衡2: 骨架屏精细度
**选择**: 简化骨架屏(不完全镜像)
**理由**:
- 加载时间通常 < 1s,用户感知有限
- 过度精细化增加维护成本
- 关键是"有占位"而非"完美镜像"

#### 权衡3: P1/P2 功能是否实现
**建议**: 优先 P0,P1 选择性实现
**理由**:
- P0 是规范违规修复,必须做
- P1 是体验提升,ROI 较高(如三阶段导航、冲突标识)
- P2 是高级优化,收益递减(如日志交互、视图切换)

## 测试策略 / Testing Strategy

### 视觉回归测试
- **工具**: 浏览器开发者工具 + 手动验证
- **检查点**:
  - 加载状态无布局跳动
  - Tooltip 正确显示完整内容
  - 冲突徽章位置和颜色正确

### 功能回归测试
- **范围**: 升级流程端到端
- **验证**:
  - 准备 → 配置 → 执行流程正常
  - 预检查、配置合并、任务执行逻辑不受影响
  - 国际化切换正常

### 规范自检
- **对照**: `.trellis/spec/frontend/ui-conventions.md`
- **检查项**:
  - ✅ 零塌陷加载
  - ✅ 高信噪比与渐进式披露
  - ✅ 原子信息防被迫换行
  - ✅ 交互反馈清晰

## 回滚方案 / Rollback Plan

### Git 提交策略
按优先级分 commit:
1. `feat(ui): 集群升级界面添加骨架屏加载状态` (P0-R1)
2. `refactor(ui): 优化 StatusCard 信息密度` (P0-R2)
3. `feat(ui): 节点消息列添加 Tooltip 完整展示` (P0-R3)
4. `feat(ui): 集群升级添加三阶段流程导航` (P1-R4, 可选)
5. ... 其他 P1/P2

### 回滚粒度
- **完全回滚**: `git revert <commit-hash>` 单个 commit
- **部分回滚**: 手动移除对应代码块(UI 改动互不耦合)

### 风险评估
- **低风险**: P0 改动均为纯 UI 展示,无业务逻辑变更
- **零数据风险**: 不涉及 API、数据库、状态存储
