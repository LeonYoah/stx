# 预览缺连接器提示 + 预览数据表格换 BeUI table

## 背景

1. 预览会把任务 sink 换成 SeaTunnel `Http` sink，让引擎把样本回传 STX。集群没装 HTTP 连接器时，引擎返回
   `FactoryException ... Unable to create a sink for identifier 'Http'`，前端原样显示 `submit job failed: status=500 body=...`，
   用户看不出要装连接器，更看不出这个 Http sink 是预览自己注入的。
2. 预览数据表是自研 `StudioDataTable`（排序 + 客户端分页）。预览列是动态的、可能很宽，行数最多 10000，
   没有虚拟滚动、列宽不可调，长值只能靠 tooltip 看。

## 需求

### A. 缺连接器提示

- 后端识别引擎错误 `Unable to create a (source|sink|transform) for identifier 'X'`，归类为「集群缺连接器」，
  在提交的共用位置（预览与正式运行两个 `engineClient.Submit` 调用点）处理一次。
- 预览 / 运行 / 恢复接口遇到该错误时返回 409 + `error_code=connector_missing`，`error_msg` 为可读说明并保留原始引擎消息
  （前端从原文解析插件类型与标识；是否预览注入的 Http sink 由前端调用场景判断）。
- 前端错误弹窗新增「连接器缺失」分类：标题、说明、建议；预览注入场景说明「预览需要 HTTP 连接器」；
  提供「去安装」按钮跳到插件市场并预填搜索词。原始错误折叠在详情里。
- 作业列表 / 详情里已存的失败消息走同一分类（按消息文本识别），不依赖结构化 `data`。
- 不做提交前检查：插件列表有缓存、自带连接器探测失败与「没有」不可区分，会误拦；引擎拒绝是同步返回的，
  归类后体验等价。

### B. 预览表格

- 预览数据表换成 BeUI `table`（虚拟滚动、列宽拖动、排序、骨架屏）。只换预览；工作台任务表、首页作业表仍用
  `StudioDataTable`（数据浏览与管理列表是两类场景）。
- 交互：
  - 全部行一次渲染（虚拟滚动），去掉客户端分页，底栏显示行数。
  - 单元格：NULL 与空字符串区分显示；长值截断 + 悬停看全文；单元格可一键复制。
  - `RowKind` 列保持徽标样式。
  - 首次加载骨架屏、刷新顶部进度条，不塌陷。
  - 空态区分「没有预览」「预览准备中」「没有数据」。
- 文案全部走 next-intl；BeUI 内置文案经 `labels` 传入。

## 验收标准

- AC1：集群缺 HTTP 连接器时点预览，弹窗标题为连接器缺失，说明预览需要 HTTP 连接器，有「去安装」按钮跳到
  `/plugins?q=http`，插件市场搜索框已填 `http`；原始引擎错误可展开查看。
- AC2：正式运行缺 source/sink 连接器时同样归类，说明里带连接器类型和标识。
- AC3：后端单测覆盖错误归类（source/sink/transform、大小写、非匹配错误原样返回、结果未知错误不归类）。
- AC4：预览表 1 万行滚动流畅，列宽可拖，排序可用，NULL / 空串可区分，单元格可复制。
- AC5：`go test ./internal/apps/sync/...`、前端相关单测、`tsc --noEmit` 通过；spec 同步更新。
