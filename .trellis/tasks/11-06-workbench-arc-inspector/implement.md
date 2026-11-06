# 执行顺序

## 这一刀

- [x] 设置页拆成上下两段高级属性，模板默认折叠
- [x] `StudioSidebarShell` 增加 `flush`，设置页自管高度
- [x] 中英文标题
- [x] 超时测试补上两段标题，并确认默认看不到精选模板列表
- [x] `cd frontend && pnpm exec vitest --run components/common/sync/__tests__/TimeoutSettingsSection.test.tsx`

## 主线：第一批 Arc 控件

- [x] 安装 `arc-foundation` 与第一批免费组件，根布局引入 `foundation.css`，同步 `data-theme`
- [x] 顶栏：Action button 保存，Split button 发布，Expanding button group 测试 / DAG / 预览，Expanding search
- [x] 左侧过滤用 Search field，范围用 Segmented control
- [x] 复制（作业详情与节点名称/当前表）、日志筛选、执行模式、可观测性换对应 Arc 控件
- [ ] 树和 File dropzone 单独立项，不在第一批
- [ ] 变量列表里的复制按钮仍走原来的回调。一行一个 CopyButton 会在长列表上挂很多 ResizeObserver，现有单测也直接断言回调。

## 风险

- 设置页改为 `fill` 后，如果外壳仍包一层 `ScrollArea`，两段不会各自滚动。设置页必须走 `flush`。
- 模板折叠不要 `forceMount`，否则默认视图仍会挂上整块模板列表。
