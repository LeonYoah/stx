# 右侧高级属性分栏

## 边界

只改工作台右侧设置页的排版，不改保存、发布、运行的数据流。

- `StudioSidebarShell` 在设置页使用 `fill`，并增加 `flush`，让子面板自己占满高度、自己留白。模板管理页仍用带内边距的 `fill`。
- `SettingsSidebarPanel` 根节点改为纵向两段，各 `min-h-0 flex-1 overflow-y-auto`。
- 模板块从默认流里拿出来，放进下段一个默认关闭的 `Collapsible`。展开后沿用现在的精选 / 原始两个 Tab 和 `TemplatePluginSelect`。

## 布局

```
┌ 编辑器（焦点）          ┐ ┌ 任务高级属性
│                         │ │  执行模式、集群、任务变量
├ 底部控制台              ┤ ├ 运行高级属性
│                         │ │  可观测性、超时、告警
│                         │ │  模板（默认折叠）
└                         ┘ └
```

右侧外壳仍 `row-span-2`，所以下段自然落在控制台旁边。两段都是属性，不是第二块编辑器。

## 兼容

- 回调签名不变。`DataSyncStudio` 继续把集群、变量、模板、告警传进来。
- 超时折叠、localStorage 和 `forceMount` 保持原样，测试仍能找到输入框。
- 不把模板放进 `ScrollArea` 的 table 布局里，避免长名称撑宽侧栏。两段用普通 `overflow-y-auto`。

## 回滚

还原 `SettingsSidebarPanel` 的设置页结构和 `StudioSidebarShell` 的 `flush`。没有接口和存储迁移。
