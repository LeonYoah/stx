# API 与 Services

> 前端如何调用后端以及如何统一处理错误。

---

## 概述

所有后端请求通过 `lib/services/core/api-client.ts` 的 **axios** 客户端发出。客户端使用 `baseURL: '/api/v1'`、`withCredentials: true` 和 60 秒超时。响应格式为 **ApiResponse&lt;T&gt;**，包含 `error_msg` 与 `data`。`lib/services/core/base.service.ts` 中的 **BaseService** 封装 get/post/put/delete，成功时仅返回 `data`；错误由响应拦截器处理，以 `Error` 形式 reject，消息为后端返回或通用提示。

---

## API 响应结构

- **成功**：后端返回 `{ "error_msg": "", "data": <载荷> }`。Services 返回类型为 `T` 的 `data`。
- **错误**：后端返回 HTTP 4xx/5xx，通常带 `{ "error_msg": "消息" }`。拦截器用 `Error` reject，若存在 `error_msg` 则 `message` 为该值。

来自 `lib/services/core/types.ts`：

```ts
export interface ApiResponse<T = unknown> {
  error_msg: string;
  data: T;
}

export interface ApiError {
  error_msg: string;
  data?: unknown;
}
```

---

## 服务层

- **BaseService**：抽象类，提供静态方法 `get<T>`、`post<T>`、`put<T>`、`delete<T>`，以及按需透传查询参数的 `postWithParams<T>`、`putWithParams<T>`。各方法调用 `apiClient.*`，期望 `ApiResponse<T>`，并返回 `response.data.data`。子类设置 `protected static basePath`，通过 `getFullPath(path)` 拼 URL。
- **约定**：
  - 每个领域一个 service 类（如 `ClusterService`、`HostService`）。
  - 方法对应后端接口（如 `list(params)`、`getById(id)`、`create(body)`）。
  - 返回类型为 `data` 的类型（如 `ClusterInfo`、`PaginatedResponse<ClusterInfo>`）；不向调用方暴露原始 `ApiResponse`。
  - 登录页这类**能力开关由后端决定**的界面，不要硬编码展示第三方登录入口。应先调用服务（如 `auth.getEnabledOAuthProviders()` → `GET /api/v1/oauth/providers`），再按返回结果渲染 GitHub / Google 按钮。
  - 安装向导 / 创建集群这类**版本敏感配置**，不要在前端硬编码版本比较逻辑来猜哪些字段可用。应优先消费后端返回的能力矩阵（如 `installer.listPackages()` 返回的 `version_capabilities`），再决定是否展示 `history_job_expire_minutes`、`scheduled_deletion_enable`、`job_schedule_strategy` 等高级配置。
  - 若后端来自 Go / GORM 等实现，**不要假定数组字段一定是 `[]`**；对于会被 UI 直接 `.map()` / `.filter()` 的字段，优先在 service 或 session 恢复边界统一做 normalize，把 `null` 兜底成 `[]`。
  - 若接口返回**后端本地化文案**（如 diagnostics 摘要、诊断报告、SSE 事件、模板说明），service 应显式附带当前语言参数 `lang`，避免后端回落到默认语言或双语拼接结果。
  - 后端写接口接入公共执行协议后，Web service 必须同步接入 `createWebExecutionHeaders`。不能只验证 CLI，也不能在全局 axios 拦截器里给所有 POST/PUT/DELETE 自动加执行请求头，因为登录、查询型 POST 和未接入公共执行协议的接口不需要这些请求头。
  - R1 请求必须发送稳定的 `Idempotency-Key` 和 `X-STX-Confirm: true`；R2/R3 首次请求取得 `confirmation_id` 后，必须复用同一个幂等键，并通过 `X-STX-Confirmation-ID` 只重试一次。上传分片应为每个分片生成稳定且互不相同的幂等键，使单个分片可以安全重试。
  - 在线补充 SeaTunnel 源码属于长耗时写请求，service 要为该请求设置独立的 10 分钟超时，不能沿用普通 API 客户端的 60 秒超时。
  - 同一业务动作的 CLI 与 Web 调用方必须一起检查。后端新增或提高风险等级时，至少验证 Web 正常请求、幂等重试、一次性确认和错误结构透传。
- **使用**：组件或 hooks 从 `lib/services/<domain>` 导入 service，调用静态方法；错误用 try/catch 捕获，消息在 UI 中展示（toast 或行内错误状态）。

---

## 错误处理

- **401**：拦截器重定向到 `/login`（或 `/login?redirect=...`），`/auth/login` 自身除外，以便登录失败能展示后端错误。
- **403**：以「权限不足」消息 reject。
- **5xx**：以「服务器内部错误，请稍后重试」reject。
- **超时 (ECONNABORTED)**：以「请求超时，请检查网络连接」reject。
- **后端 error_msg**：若存在 `error.response?.data?.error_msg`，用该消息 reject；否则使用通用提示。
- **结构化确认数据**：响应拦截器应把后端 `data` 保留在抛出的 `ApiError.data` 中，R2/R3 service 据此读取 `confirmation_required` 和 `confirmation_id`。不能只保留错误文案，否则 Web 无法完成第二次确认请求。
- **在组件中**：对 service 调用包一层 try/catch；用 toast（如 sonner）或行内错误状态展示 `error.message`。不要向用户暴露堆栈或打日志。

---

## 新增一个 Service

1. 在 `lib/services/<domain>/` 下创建 `<domain>.service.ts`，可选 `types.ts` 和 `index.ts`。
2. 继承 `BaseService`，设置 `basePath` 为后端前缀（如 `'/clusters'`）。
3. 添加方法，调用 `this.get/post/put/delete` 并指定路径与返回类型 `T`（即 `data` 类型）。
4. 从 `lib/services/<domain>/index.ts` 导出，如需再从 `lib/services/index.ts` 导出。
5. 与后端契约一致：列表接口常返回 `{ data: { total, clusters } }` 等；类型 `T` 与之对应。

---

## 常见错误

- 在共享 `apiClient` 之外使用 `fetch` 或裸 axios（会绕过 401 重定向与错误统一处理）。
- 假定成功时一定存在 `data`；拦截器只透传响应，BaseService 假定为 `ApiResponse<T>` 并返回 `data` — 需保证后端 2xx 时始终返回 `data`。
- 基于会话的认证忘记 `withCredentials: true`；客户端与请求拦截器中已配置。
- 在调用处定义包含 `error_msg` 的响应类型；应把成功视为 `T`，错误视为抛出的 `Error`。
- 直接信任后端数组字段永远不是 `null`；尤其是 Go 的 nil slice 可能在 JSON 中变成 `null`，会让 UI 在 `.map()` / `.filter()` / `.length` 处崩溃。对这类字段要在边界统一 normalize。
- 后端给某个写接口加上 `Authorize` 后，只修改 CLI，不检查 Web service。这样浏览器请求会缺少 `Idempotency-Key` 或确认请求头，并直接收到 4xx。
- 在全局请求拦截器里无条件给全部写请求添加确认头。这样会把公共执行语义错误地扩散到登录、查询型 POST 和普通配置接口。
- R2/R3 第二次确认请求重新生成幂等键。确认编号与原幂等键、请求摘要绑定，换键会导致确认失败。

## Scenario: Sync 工作台 Config DAG 兼容 WebUI `jobDag`

### 1. Scope / Trigger
- Trigger：Data Sync Studio 需要展示配置预览 DAG，但不希望长期维护一套与 SeaTunnel Web 完全分叉的 DAG 数据模型。

### 2. Signatures
- 前端 service：
  - `POST /api/v1/sync/tasks/:id/dag`
- config tool（由后端调用，不直接暴露给浏览器）：
  - `POST /api/v1/config/webui-dag`

### 3. Contracts
- `/api/v1/sync/tasks/:id/dag` 返回仍保持 `data` 包裹，但 `data` 至少包含：
  - `nodes`
  - `edges`
  - `webui_job`
  - `warnings`
  - `simple_graph`
- `webui_job` 必须是 **WebUI-compatible pseudo job detail**，最少包含：
  - `jobId`
  - `jobName`
  - `jobStatus`
  - `jobDag.pipelineEdges`
  - `jobDag.vertexInfoMap`
  - `metrics`
  - `pluginJarsUrls`
- `metrics` 即使没有 runtime 指标也不能省；必须提供空壳对象，避免前端 DAG 详情区再做特殊分支。

### 4. Validation & Error Matrix
- `config tool` 支持 `/api/v1/config/webui-dag`：
  - 返回 `webui_job`，前端用兼容 DAG 视图渲染。
- `config tool` 不支持 `/api/v1/config/webui-dag` 或返回非 2xx：
  - `/sync` DAG 预览直接报错，提示后端/agent/proxy 版本不满足要求。
- 只有在用户显式查看调试数据时，才允许展示原始 JSON；它不是主链路 fallback。

### 5. Good/Base/Bad Cases
- Good：
  - `webui_job.jobDag.vertexInfoMap` 与 `pipelineEdges` 完整，前端直接渲染兼容 DAG。
- Base：
  - 返回 `webui_job`，但 `metrics` 是空壳对象；前端仍可稳定渲染。
- Bad：
  - 后端只返回旧 `nodes/edges` 而没有 `webui_job`，前端被迫长期维护两套 DAG 语义。

### 6. Tests Required
- Backend：
  - java-proxy `webui-dag` 映射测试：断言 `vertexInfoMap`、`pipelineEdges`、`metrics` 空壳存在。
  - Go service 测试：当 `InspectWebUIDAG` 可用时，`/sync` DAG 结果必须带 `webui_job`。
  - Go service 测试：当 `InspectWebUIDAG` 失败时，错误应直接向上返回，不再偷偷回退旧接口。
- Frontend：
  - TypeScript 类型检查，确保 `SyncDagResult.webui_job` 与 DAG 视图组件契约一致。

### 7. Wrong vs Correct
#### Wrong
- 直接把 config DAG 当成一份自定义节点列表，前端单独维护新的展示模型。

#### Correct
- 后端统一适配成 WebUI-compatible `jobDag`，前端只消费该兼容结构；旧 config DAG 不再作为 DAG 主链路 fallback 契约。
