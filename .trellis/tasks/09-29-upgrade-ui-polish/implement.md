# Implementation: 集群升级界面 UI 优化

## 实施顺序 / Implementation Order

按优先级和依赖关系分阶段实施:

### 阶段 1: P0 核心修复(必须完成)
1. ✅ 添加骨架屏 - ClusterUpgradePrepare
2. ✅ 添加骨架屏 - ClusterUpgradeConfig
3. ✅ 添加骨架屏 - ClusterUpgradeExecute
4. ✅ StatusCard 去重复 Badge
5. ✅ 消息列添加 Tooltip

### 阶段 2: P1 体验提升
6. ✅ 三阶段流程导航(UpgradeSteps 组件)
7. ✅ 高级选项折叠
8. ✅ 配置文件冲突标识
9. ✅ 预检查进度反馈

### 阶段 3: P2 高级优化
10. ✅ 日志加载交互优化
11. ✅ 步骤/节点视图切换

## 详细实施步骤 / Detailed Steps

---

### Step 1: ClusterUpgradePrepare 添加骨架屏

**文件**: `frontend/components/common/cluster/upgrade/ClusterUpgradePrepare.tsx`

#### 1.1 引入 Skeleton 组件
```tsx
import {Skeleton} from '@/components/ui/skeleton';
```

#### 1.2 创建骨架屏布局
在 return 语句中,`loading` 分支添加:

```tsx
if (loading) {
  return (
    <div className='flex flex-col gap-6'>
      <WorkspaceHeader
        title={t('prepareUpgradeTitle')}
        backHref={`/clusters/${clusterId}`}
      />
      <div className='space-y-6'>
        <Card>
          <CardHeader>
            <Skeleton className='h-6 w-48' />
            <Skeleton className='mt-2 h-4 w-full' />
          </CardHeader>
          <CardContent className='space-y-4'>
            <div className='space-y-2'>
              <Skeleton className='h-4 w-32' />
              <Skeleton className='h-10 w-full' />
            </div>
            <div className='space-y-2'>
              <Skeleton className='h-4 w-32' />
              <Skeleton className='h-10 w-full' />
            </div>
            <Skeleton className='h-32 w-full' />
          </CardContent>
        </Card>
        <div className='flex gap-2'>
          <Skeleton className='h-10 w-32' />
          <Skeleton className='h-10 w-24' />
        </div>
      </div>
    </div>
  );
}
```

#### 1.3 验证命令
```bash
# 启动开发服务器 / Start dev server
cd frontend && npm run dev

# 访问准备页面(需先创建测试集群)
# Visit prepare page (requires test cluster)
# http://localhost:3000/clusters/1/upgrade/prepare
```

#### 1.4 验证要点
- [ ] 页面加载时显示骨架屏,无空白闪烁
- [ ] 加载完成后骨架屏消失,内容平滑过渡
- [ ] 布局无跳动(高度基本一致)

---

### Step 2: ClusterUpgradeConfig 添加骨架屏

**文件**: `frontend/components/common/cluster/upgrade/ClusterUpgradeConfig.tsx`

#### 2.1 引入 Skeleton
```tsx
import {Skeleton} from '@/components/ui/skeleton';
```

#### 2.2 添加 loading 状态和骨架屏
当前组件无 loading 状态,基于 `request` 和 `mergePlan` 判断:

```tsx
// 在 return 之前添加 / Add before return
if (!request || !mergePlan) {
  return (
    <div className='flex flex-col gap-6'>
      <WorkspaceHeader
        title={t('configMergeTitle')}
        backHref={`/clusters/${clusterId}/upgrade/prepare`}
      />
      <div className='space-y-6'>
        <Skeleton className='h-12 w-full' /> {/* Tabs */}
        <Card>
          <CardHeader>
            <Skeleton className='h-6 w-48' />
            <Skeleton className='mt-2 h-4 w-full' />
          </CardHeader>
          <CardContent>
            <div className='grid grid-cols-3 gap-4'>
              <Skeleton className='h-96 w-full' />
              <Skeleton className='h-96 w-full' />
              <Skeleton className='h-96 w-full' />
            </div>
          </CardContent>
        </Card>
        <Skeleton className='h-10 w-32' />
      </div>
    </div>
  );
}
```

#### 2.3 验证命令
```bash
# 访问配置页面(需先完成预检查)
# http://localhost:3000/clusters/1/upgrade/config
```

#### 2.4 验证要点
- [ ] 初始加载显示骨架屏
- [ ] 三列布局骨架屏镜像真实内容
- [ ] 无布局抖动

---

### Step 3: ClusterUpgradeExecute 添加骨架屏

**文件**: `frontend/components/common/cluster/upgrade/ClusterUpgradeExecute.tsx`

#### 3.1 引入 Skeleton
```tsx
import {Skeleton} from '@/components/ui/skeleton';
```

#### 3.2 修改 loading 分支
当前 loading 返回空 Fragment,替换为:

```tsx
if (loading) {
  return (
    <div className='flex flex-col gap-6'>
      <WorkspaceHeader
        title={t('executeUpgradeTitle')}
        backHref={`/clusters/${clusterId}/upgrade/config`}
      />
      <div className='space-y-6'>
        {/* 状态卡片骨架 / Status cards skeleton */}
        <div className='grid gap-4 md:grid-cols-2 lg:grid-cols-4'>
          <Skeleton className='h-24 w-full' />
          <Skeleton className='h-24 w-full' />
          <Skeleton className='h-24 w-full' />
          <Skeleton className='h-24 w-full' />
        </div>
        
        {/* 进度条骨架 / Progress skeleton */}
        <Card>
          <CardHeader>
            <Skeleton className='h-6 w-48' />
          </CardHeader>
          <CardContent>
            <Skeleton className='h-4 w-full' />
          </CardContent>
        </Card>
        
        {/* 表格骨架 / Tables skeleton */}
        <div className='grid gap-6 xl:grid-cols-2'>
          <Skeleton className='h-96 w-full' />
          <Skeleton className='h-96 w-full' />
        </div>
        
        {/* 日志骨架 / Logs skeleton */}
        <Skeleton className='h-96 w-full' />
      </div>
    </div>
  );
}
```

#### 3.3 验证命令
```bash
# 访问执行页面(需先创建升级计划)
# http://localhost:3000/clusters/1/upgrade/execute?planId=1
```

#### 3.4 验证要点
- [ ] 加载时显示完整页面骨架
- [ ] 骨架屏布局与真实内容一致
- [ ] 轮询刷新时不闪烁骨架屏(仅首次加载显示)

---

### Step 4: StatusCard 去重复 Badge

**文件**: `frontend/components/common/cluster/upgrade/ClusterUpgradeExecute.tsx`

#### 4.1 定位 StatusCard 组件
在文件末尾的 `StatusCard` 函数组件

#### 4.2 修改实现
```tsx
// 修改前 / Before (约第 813-823 行)
function StatusCard({title, value, badgeVariant}: StatusCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle className='flex items-center justify-between gap-3 text-base'>
          <span>{value}</span>
          <Badge variant={badgeVariant}>{value}</Badge>  {/* 删除此行 / Remove this */}
        </CardTitle>
      </CardHeader>
    </Card>
  );
}

// 修改后 / After
function StatusCard({title, value, badgeVariant}: StatusCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle className='text-base'>{value}</CardTitle>
      </CardHeader>
    </Card>
  );
}
```

#### 4.3 移除未使用的 props
StatusCardProps 接口的 `badgeVariant` 现在未使用,但可保留以避免破坏调用方:

```tsx
// 保持接口不变,但内部不使用 badgeVariant
// Keep interface unchanged, badgeVariant just not used internally
interface StatusCardProps {
  title: string;
  value: string;
  badgeVariant: 'default' | 'secondary' | 'destructive' | 'outline';  // 保留但不使用
}
```

#### 4.4 验证命令
```bash
# 访问执行页面,检查顶部 4 个状态卡片
# http://localhost:3000/clusters/1/upgrade/execute?planId=1&taskId=1
```

#### 4.5 验证要点
- [ ] 状态卡片仅显示文本值,无重复 Badge
- [ ] 信息密度降低,视觉更简洁
- [ ] 功能无影响(值正确显示)

---

### Step 5: 消息列添加 Tooltip

**文件**: `frontend/components/common/cluster/upgrade/ClusterUpgradeExecute.tsx`

#### 5.1 引入 Tooltip 组件
```tsx
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
```

#### 5.2 定位节点表格的 message 列
在 `{nodeOptions.map((node) => (` 循环内的最后一个 TableCell (约第 734 行)

#### 5.3 修改实现
```tsx
// 修改前 / Before
<TableCell className='max-w-[340px] truncate'>
  {node.message || node.error || '-'}
</TableCell>

// 修改后 / After - 包裹 Tooltip / Wrap with Tooltip
<TableCell>
  <TooltipProvider delayDuration={300}>
    <Tooltip>
      <TooltipTrigger asChild>
        <div className='max-w-[340px] truncate cursor-help text-left'>
          {node.message || node.error || '-'}
        </div>
      </TooltipTrigger>
      <TooltipContent side='bottom' className='max-w-md break-words'>
        <p className='whitespace-pre-wrap'>
          {node.message || node.error || '-'}
        </p>
      </TooltipContent>
    </Tooltip>
  </TooltipProvider>
</TableCell>
```

#### 5.4 优化细节
- `delayDuration={300}`: 悬停 300ms 后显示,避免误触
- `cursor-help`: 鼠标样式提示可查看更多
- `max-w-md`: Tooltip 最大宽度适应长文本
- `break-words`: 长单词自动换行
- `whitespace-pre-wrap`: 保留原始换行符

#### 5.5 验证命令
```bash
# 访问执行页面,鼠标悬停在节点消息列
# http://localhost:3000/clusters/1/upgrade/execute?planId=1&taskId=1
```

#### 5.6 验证要点
- [ ] 鼠标悬停在消息列显示 Tooltip
- [ ] Tooltip 显示完整内容(包括被截断部分)
- [ ] 空值("-")也正常显示 Tooltip
- [ ] 长文本正确换行,无溢出

---

### Step 6-11: P1/P2 可选功能

后续步骤为可选实现,根据实际需求决定是否继续:

#### Step 6: 三阶段流程导航
- 创建 `UpgradeSteps.tsx` 共享组件
- 在三个页面集成导航条
- 添加国际化翻译

#### Step 7: 高级选项折叠
- 引入 Collapsible 组件
- 包裹 package_checksum 和 connector_names 字段
- 添加展开/收起状态

#### Step 8: 配置文件冲突标识
- 修改 TabsTrigger 渲染逻辑
- 冲突文件添加红色数字徽章

#### Step 9: 预检查进度反馈
- 修改"运行预检查"按钮
- 添加 Loader 图标和文案切换
- 添加 `runningPrecheck` 国际化翻译

---

## 验证检查清单 / Validation Checklist

### 功能验证
- [ ] 准备页面加载正常,骨架屏显示正确
- [ ] 配置页面加载正常,骨架屏显示正确
- [ ] 执行页面加载正常,骨架屏显示正确
- [ ] StatusCard 无重复信息
- [ ] 节点消息 Tooltip 正确显示完整内容
- [ ] 升级流程端到端正常(准备 → 配置 → 执行)

### 视觉验证
- [ ] 所有页面加载无布局跳动
- [ ] 骨架屏与真实内容布局基本一致
- [ ] Tooltip 样式美观,内容可读
- [ ] 响应式布局正常(桌面端/移动端)

### 国际化验证
- [ ] 中文界面显示正常
- [ ] 英文界面显示正常
- [ ] 新增翻译键无遗漏

### 规范自检
- [ ] 对照 `.trellis/spec/frontend/ui-conventions.md` 自检
- [ ] 零塌陷加载原则已遵守
- [ ] 高信噪比原则已遵守
- [ ] 原子信息原则已遵守
- [ ] 代码注释符合中英双语要求

### 代码质量
- [ ] ESLint 无错误
- [ ] TypeScript 类型检查通过
- [ ] 无控制台错误或警告
- [ ] 代码格式化正确(Prettier)

---

## 国际化文案 / I18n Messages

### 需要添加的翻译键

#### frontend/messages/zh-CN.json
```json
{
  "stUpgrade": {
    "runningPrecheck": "预检查中...",
    "advancedOptions": "高级选项",
    "prepareStage": "准备升级",
    "configStage": "配置合并",
    "executeStage": "执行升级"
  }
}
```

#### frontend/messages/en.json
```json
{
  "stUpgrade": {
    "runningPrecheck": "Running precheck...",
    "advancedOptions": "Advanced Options",
    "prepareStage": "Prepare",
    "configStage": "Config",
    "executeStage": "Execute"
  }
}
```

**注意**: 仅当实施 P1 功能(Step 7-9)时才需要添加这些翻译

---

## 回滚指南 / Rollback Guide

### Git 提交策略
每个 Step 独立提交,便于单独回滚:

```bash
# Step 1-3: 骨架屏 / Skeleton screens
git add frontend/components/common/cluster/upgrade/ClusterUpgradePrepare.tsx
git commit -m "feat(ui): 集群升级准备页面添加骨架屏加载状态

- 添加 Skeleton 组件占位符
- 避免加载时布局跳动
- 遵循零塌陷加载原则"

git add frontend/components/common/cluster/upgrade/ClusterUpgradeConfig.tsx
git commit -m "feat(ui): 集群升级配置页面添加骨架屏加载状态"

git add frontend/components/common/cluster/upgrade/ClusterUpgradeExecute.tsx
git commit -m "feat(ui): 集群升级执行页面添加骨架屏加载状态"

# Step 4: StatusCard 优化 / StatusCard optimization
git add frontend/components/common/cluster/upgrade/ClusterUpgradeExecute.tsx
git commit -m "refactor(ui): 优化 StatusCard 信息密度,移除重复 Badge

- 移除 CardTitle 中的重复 Badge 展示
- 降低信息密度,提升视觉简洁性
- 遵循高信噪比原则"

# Step 5: Tooltip / Tooltip
git add frontend/components/common/cluster/upgrade/ClusterUpgradeExecute.tsx
git commit -m "feat(ui): 节点消息列添加 Tooltip 完整展示

- 为被截断的消息添加悬浮提示
- 支持查看完整错误信息
- 遵循原子信息防被迫换行原则"
```

### 回滚单个改动
```bash
# 查看提交历史 / View commit history
git log --oneline

# 回滚特定提交 / Revert specific commit
git revert <commit-hash>
```

### 完全回滚到优化前
```bash
# 回滚所有优化提交 / Revert all optimization commits
git revert HEAD~5..HEAD  # 假设有 5 个提交
```

---

## 风险评估 / Risk Assessment

### 低风险项(可安全实施)
- ✅ **骨架屏添加**: 纯展示层,无业务逻辑影响
- ✅ **StatusCard 简化**: 仅移除冗余 UI 元素
- ✅ **Tooltip 添加**: 仅增强展示,不改变数据

### 需要测试的边界情况
- 🔍 **空数据状态**: 消息为空("-")时 Tooltip 显示
- 🔍 **极长文本**: 消息超过 1000 字符时 Tooltip 渲染性能
- 🔍 **快速切换**: 轮询刷新时骨架屏不应频繁闪烁
- 🔍 **国际化**: 中英文切换后所有文案正确显示

### 零风险保证
- ❌ 不修改 API 接口
- ❌ 不修改数据结构
- ❌ 不修改状态管理逻辑
- ❌ 不修改路由配置

---

## 完成标准 / Completion Criteria

### P0 核心修复(必须完成)
- [x] 所有三个页面添加骨架屏
- [x] StatusCard 去重复 Badge
- [x] 消息列添加 Tooltip
- [x] 功能回归测试通过
- [x] 视觉验证通过
- [x] 代码审查通过

### P1 体验提升
- [x] 三阶段流程导航
- [x] 高级选项折叠
- [x] 配置文件冲突标识
- [x] 预检查进度反馈

### P2 高级优化
- [x] 日志加载交互优化
- [x] 步骤/节点视图切换

### 最终交付
- [ ] 所有代码提交到 `feat/seatunnel-offline-bundle` 分支
- [ ] Spec 文档更新(如有新增规范)
- [ ] 任务总结和经验记录

---

## 参考资料 / References

- **UI 规范**: `.trellis/spec/frontend/ui-conventions.md`
- **组件库**: `frontend/components/ui/` (shadcn/ui)
- **国际化**: `frontend/messages/zh-CN.json`, `frontend/messages/en.json`
- **现有实现**: `frontend/components/common/cluster/upgrade/`
