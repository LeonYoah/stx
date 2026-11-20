# 实施清单（交接）

> 状态：后端 / 前端 / 单测 / spec 已完成；tsc 与相关 vitest、`go test ./internal/apps/sync/...` 均通过。
> 未做真实浏览器验证（需要真实引擎缺连接器的集群），见「待完成」说明。

## 已完成

### A 后端
- [x] `internal/apps/sync/connector_missing.go`：`ConnectorMissingError` + `classifyEngineSubmitError` + `syncErrorCode`。
  - 匹配 `Unable to create a (source|sink|transform) for identifier 'X'`（SeaTunnel API-06）。
  - `EngineCallUnknownError` 不归类（结果未知要登记待确认作业）。
  - `Error()` 前面是可读说明，冒号后保留原始引擎消息。
- [x] `service.go` 两个 `engineClient.Submit` 调用点（预览 / 正式提交，恢复也走正式提交）各归类一次。
- [x] `handler.go`：`JobResponse.ErrorCode`；预览 / 提交 / 恢复失败带 `error_code=connector_missing`；
  `getStatusCodeForError` 映射 409（CLI 归为冲突、不可重试）。
- [x] 单测 `connector_missing_test.go`：三种插件类型、大小写、非匹配原样返回、结果未知不归类、提交链路 → 409。
  `go test ./internal/apps/sync/ -run 'ConnectorMissing|ClassifyEngine|SubmitDefinite|SubmitUnknown'` 通过。
- [x] CLI：`stx sync task preview|submit` 直接打印 `error_msg`，409 走 `CodeConflict`，无需改代码。

### A 前端
- [x] `api-client.ts` 把 `error_code` 挂到抛出的 Error 上（`errorCode`），`types.ts` 补字段。
- [x] `sync-studio-utils.ts`：`ErrorCategory` 加 `connector`；`UserFacingErrorState.action`；
  `parseConnectorMissingError`；`formatSyncUserFacingError(..., {preview})` 最先识别缺连接器，
  预览 + Http sink 用专门文案，`action` 跳 `/plugins?q=<identifier 小写>`。
- [x] `DataSyncStudio.tsx` 预览失败传 `{preview: true}`。
- [x] `StudioDialogs.tsx`：`connector` 徽标；`action` 渲染为新标签页打开的按钮（避免离开工作台丢草稿）。
- [x] `PluginMain.tsx`：`?q=` 预填搜索词。
- [x] i18n（zh / en）：`connectorMissing*`、`connectorType.*`。

### B 前端
- [x] vendored beUI Table 到 `frontend/components/motion/table/` + `components/motion/checkbox.tsx`，
  MIT 头、`@/lib/i18n` 改指 `@/components/charts/beui-i18n`，`lib/utils|ease|touch` 复用仓库版本；
  `licenses/third_party_mit_files.txt` 已登记。
- [x] `PreviewDataTable.tsx`：ResizeObserver 量高（beUI 虚拟滚动要像素高度）、行高 32、列默认 160px、
  可拖列宽 / 列重排 / 排序；NULL 与空串区分；单元格悬停复制；`RowKind` 徽标；labels 用 `t.raw` 传
  （`{column}` 由 beUI 自己替换，不能让 next-intl 格式化）。
- [x] `CheckpointPanels.tsx` `PreviewWorkspacePanel`：表格换成 `PreviewDataTable`，去掉客户端分页与排序状态，
  底栏显示行数 + 截断提示（`previewSnapshot.truncated` / `row_limit`）；删掉 `previewPage` / `onChangePage` props。
- [x] `DataSyncStudio.tsx` 删掉 `previewPage` 状态。
- [x] i18n：`previewRowCount`、`previewRowLimitReached`、`previewEmptyString`、`copyCellValue`、
  `previewResizeColumn`、`previewReorderColumn`。

### A 前端（续，第二轮）

- [x] 新增 `ConnectorMissingHint`：作业详情 `JobDetailDialog` 与首页 `JobPreviewDrawer` 的已存失败消息
  按引擎原文归类为「缺少连接器」并给出 `/plugins?q=` 安装链接（不依赖结构化 `error_code`，历史作业同样生效）。

## 收尾记录（第二轮）

1. [x] **类型检查**：`pnpm exec tsc --noEmit` 通过（8G 机器要 10 分钟以上，用 `NODE_OPTIONS=--max-old-space-size=6144` 后台跑）。
2. [x] **改旧单测** `PreviewWorkspacePanel.test.tsx`：去掉 `previewPage` / `onChangePage`；mock
   `@tanstack/react-virtual` 展开全部行（jsdom 量不到虚拟列表）；底栏行数断言；排序点列名按钮；
   loading 用例给非空 columns 让骨架占位条（`.animate-pulse`，不是 `data-slot="skeleton"`）出现；
   加 NULL / 空串与 `truncated` 上限提示用例。
3. [x] **新单测**：`formatSyncUserFacingError` 缺连接器五个分支（引擎原文 / 仅 error_code / 预览 Http 专属文案 /
   预览非 Http 走通用文案 / 普通错误不误判）；`StudioActionErrorDialog` 补 connector 徽标 + 新标签页 action 用例。
4. [x] **vitest 配置**：beUI 不用 CSS Module，无 PostCSS 影响。
5. [ ] **浏览器验证**（`pnpm dev` + API）：需要真实引擎且集群缺连接器才能覆盖 AC1；分类文案、弹窗 action、
   底栏行数、骨架、NULL / 空串已有单测兜底。留给有真实环境的验证：
   - 没装 HTTP 连接器的集群点预览 → 弹窗标题「集群缺少连接器」、预览专用说明、「去插件市场安装」新标签打开 `/plugins?q=http` 且搜索框已填。
   - 预览 1 万行滚动流畅；拖列宽 / 列序、排序；悬停复制；深浅色主题；ResizeObserver 跟随控制台高度。
   - 切数据集后列宽 / 排序复位（`key={activeDataset.name}`）。
6. [x] **视觉对齐**：行高 32px 对齐 `StudioDataTable` h-8、外层 `text-xs`；未改 vendored 源码。
7. [x] **E2E 评估**：不补夹具用例。缺连接器要引擎真实返回，夹具拦 `preview` 接口只能重演
   `formatSyncUserFacingError` 已被单测覆盖的路径；预览表格是 jsdom 可测的纯展示交互。
8. [x] **spec 更新**：`ui-conventions.md` §14 改为任务表 `StudioDataTable` / 预览表 beUI `PreviewDataTable`
  + vendored 约定与 checklist；`error-handling.md` 记 `error_code` 契约（`connector_missing` → 409、归类与识别规则）。
9. [x] `go test ./internal/apps/sync/...` 全量通过。

## 设计取舍（已和需求方确认）

- 不做提交前插件检查：插件列表有缓存，自带连接器探测失败与「没有」不可区分，会误拦；引擎拒绝是同步返回的，归类后体验等价。
- 只换预览表，不动工作台任务表 / 首页作业表（数据浏览和管理列表是两类场景）。
- 快照接口没有列类型，本次不做列头类型展示。
