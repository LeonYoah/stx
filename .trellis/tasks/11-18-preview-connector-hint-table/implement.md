# 实施清单（交接）

> 状态：后端 A 完成并有单测；前端 A / B 代码已写，**未跑 tsc / vitest / 浏览器验证**（开发机 8G 内存，tsc 被 OOM 杀掉）。
> 接手人按「待完成」逐项推进。

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

## 待完成

1. [ ] **跑类型检查**：`cd frontend && pnpm exec tsc --noEmit`，修报错。重点看：
   - vendored beUI 文件在本仓库 tsconfig（strict / noUncheckedIndexedAccess 等）下是否报错；
   - `t.raw(...)` 的类型；`Table<PreviewRow>` 泛型；`useSearchParams` 在 `PluginMain`（页面已包 Suspense）。
2. [ ] **改旧单测** `components/common/sync/__tests__/PreviewWorkspacePanel.test.tsx`：
   - 去掉 `previewPage` / `onChangePage`；
   - 「分页在表格底栏」用例改为「底栏显示 2 行」；排序用例改为点列头按钮（beUI 的排序按钮可访问名是列名，不再是 `sortColumn` 文案）；
   - 加用例：NULL / 空串显示、`truncated=true` 显示上限提示。
   - jsdom 没有布局，`PreviewDataTable` 会用 320px 兜底高度；若虚拟列表在 jsdom 下不渲染行，mock `@tanstack/react-virtual` 或给 scroll 容器打桩 `clientHeight`。
3. [ ] **新单测**：`formatSyncUserFacingError` 缺连接器分支（引擎原文 / 只有 error_code / 预览 Http 文案 / 正式运行文案 / action href）。
4. [ ] **vitest 配置**：beUI 不用 CSS Module，应无影响；若引入后测试报 PostCSS 错，按 spec「`css.postcss.plugins` 必须是空数组」处理。
5. [ ] **浏览器验证**（`pnpm dev` + API）：
   - 找一个没装 HTTP 连接器的集群点预览 → 弹窗标题「集群缺少连接器」、预览专用说明、「去插件市场安装」新标签打开 `/plugins?q=http` 且搜索框已填；原始错误可展开。
   - 预览 1 万行（`row_limit=10000`）滚动流畅；拖列宽、拖列序、排序；NULL / `(空串)` 可区分；悬停复制；深浅色主题。
   - 拖控制台高度，表格视口跟着变（ResizeObserver）。
   - 切数据集后列宽 / 排序复位（`key={activeDataset.name}`）。
6. [ ] **视觉打磨**：beUI 表头是 `bg-muted`、单元格 `px-4`；和控制台其他表（`studio-sidebar-chrome` 行高、Arc token）对一下，
   必要时只在 `PreviewDataTable` 外层用 className / CSS 变量覆盖，**不要改 vendored 源码**（要改就在文件头注明本地改动）。
7. [ ] **E2E**：按 `.trellis/spec/frontend/e2e-testing.md` 评估是否补「预览缺连接器弹窗」夹具用例。
8. [ ] **spec 更新**（Phase 3.3）：
   - `frontend/ui-conventions.md` §14：「任务表和预览表用 `StudioDataTable`」改为「任务表用 `StudioDataTable`；预览数据表用 beUI Table（`PreviewDataTable`），不分页、虚拟滚动」，并写 beUI Table 的引入约定（同 file-tree：不装 `lib/i18n.tsx`）。
   - `backend/error-handling.md` 或 `sync` 相关 spec：记 `error_code=connector_missing` + 409 契约与识别规则。
9. [ ] `go test ./internal/apps/sync/...` 全量跑一遍（只跑过相关用例）。

## 设计取舍（已和需求方确认）

- 不做提交前插件检查：插件列表有缓存，自带连接器探测失败与「没有」不可区分，会误拦；引擎拒绝是同步返回的，归类后体验等价。
- 只换预览表，不动工作台任务表 / 首页作业表（数据浏览和管理列表是两类场景）。
- 快照接口没有列类型，本次不做列头类型展示。
