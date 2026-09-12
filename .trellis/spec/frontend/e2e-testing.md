# E2E 测试规范

> 面向 `frontend/e2e/` 的 Playwright 端到端测试约定。目标不是把所有接口都跑成“真外网”，而是让主用户流在本地与 CI 中都稳定、可维护、可诊断。

---

## 场景：Playwright E2E 基线

### 1. Scope / Trigger

- 触发条件：新增或修改跨层主用户流，例如登录、仪表盘、插件依赖配置、安装向导、升级准备。
- 适用范围：需要同时验证页面结构、交互动作、接口回调与用户可见结果的场景。
- 不适用：纯展示组件、纯 Hook、纯算法逻辑，这些优先放在 Vitest 单测里。

### 2. Signatures

- 目录：
    - `frontend/e2e/*.spec.ts`：业务场景测试
    - `frontend/e2e/auth.setup.ts`：登录态准备
    - `frontend/e2e/helpers/*.ts`：E2E 夹具、路由桩、共享动作
    - `frontend/app/(main)/e2e-lab/*`：仅在缺少自然页面入口时使用的内部挂载页
- 脚本：
    - `cd frontend && pnpm run test:e2e`
    - `cd frontend && pnpm run test:e2e:mock`
    - `cd frontend && pnpm run test:e2e:plugin-template`
    - `cd frontend && pnpm run test:e2e:workbench-real`
- 现成样例：
    - `frontend/e2e/login-ui.spec.ts`
    - `frontend/e2e/dashboard.spec.ts`
    - `frontend/e2e/plugin-dependency-template.spec.ts`
    - `frontend/e2e/install-wizard-template.spec.ts`
    - `frontend/e2e/install-wizard-negative.spec.ts`
    - `frontend/e2e/upgrade-prepare-template.spec.ts`
    - `frontend/e2e/upgrade-prepare-negative.spec.ts`
    - `frontend/e2e/install-wizard-real.spec.ts`
    - `frontend/e2e/config-real.spec.ts`
    - `frontend/e2e/upgrade-real.spec.ts`
    - `frontend/e2e/plugin-real.spec.ts`
    - `frontend/e2e/workbench-real.spec.ts`

### 3. Contracts

- 功能代码契约：
    - 新增或修改主用户流、跨层交互、多步骤向导、核心增删改动作时，必须同步补一份 E2E 参考
    - 这份参考至少包含：场景 spec、必要锚点、夹具或真实入口页
    - 如果本次形成新的复用模式，还要把样例入口补到 `.trellis/spec/frontend/index.md` 可索引的位置
- 反例契约：
    - 主流程模板除了“成功路径”，还应至少保留一个失败或阻断样例
- 安装类流程优先覆盖“失败后停留在当前步骤并给出重试/回滚提示”
- 升级类流程优先覆盖“存在阻断问题时禁止继续，并给出明确处置入口”
- 真实环境 E2E（real suites）允许“**UI 触发关键用户动作 + API/文件系统验证最终收敛**”的混合模式，不要求所有等待条件都绑死在页面提示或横幅上
- 登录契约：
    - `auth.setup.ts` 负责生成 `frontend/.playwright/auth/admin.json`
    - 业务 spec 默认复用登录态，不要每条用例都从登录页重新走一遍
- 环境变量：
    - `E2E_API_MODE`：`real` 或 `mock`
    - `E2E_FRONTEND_BASE_URL`
    - `E2E_BACKEND_BASE_URL`
    - `E2E_USERNAME`
    - `E2E_PASSWORD`
    - `GO_BIN`
- 选择器契约：
    - 第一优先级：`getByRole`、`getByLabel`、`getByText`
    - 第二优先级：为图标按钮、重复表格行、动态弹窗补 `aria-label` 或 `data-testid`
    - 禁止把 `nth()`、长 CSS 选择器、过深 DOM 路径当成主选择器
- 夹具契约：
    - 优先“真实登录 + 业务接口定向夹具”的混合模式
- 只拦截不稳定、昂贵、依赖外部仓库或数据准备过重的接口
- 登录、路由守卫、主布局等公共链路尽量保持真实
- 如果组件只有弹窗、没有稳定页面入口，可增加一个**不进导航**的 `e2e-lab` 内部挂载页承载真实组件
- 对 real suites，优先把“创建资源 / 触发同步 / 回滚”等**易抖的收尾动作**沉到 API helper，再用页面去验证用户可见结果，避免因为弹窗按钮、toast 消失时机或 banner 轮询导致死循环

### 4. Validation & Error Matrix

| 场景                          | 推荐做法                              | 不推荐做法                               |
| ----------------------------- | ------------------------------------- | ---------------------------------------- |
| 图标按钮无可访问名称          | 补 `aria-label` 或 `data-testid`      | 在测试里点“第 3 个按钮”                  |
| 外部仓库 / Maven / 下载源波动 | 用 `page.route()` 定向夹具            | 直接依赖公网并把波动带进 CI              |
| 需要复现上传流程              | 用测试运行时生成临时 `.jar` 占位文件  | 提交大型二进制测试文件                   |
| 需要验证用户已登录            | 复用 `auth.setup.ts` 的 storage state | 每条测试都从 `/login` 开始               |
| 页面依赖太多接口              | 只拦截当前主链路必需接口              | 一次性把整个 `/api/v1/**` 全部自建假后端 |

### 5. Good / Base / Bad Cases

- Good：
    - 真实后端负责登录、会话、主布局
    - 当前业务链路的重接口用固定夹具
    - 用例断言“用户看见了什么变化”
    - real suites 里先用 UI 触发关键路径，再用 API 与真实文件落盘确认最终状态
- Base：
    - `test:e2e:mock` 用于最小 smoke，确保无 Go 环境时也能起一条基础校验
- Bad：
    - 依赖共享测试环境中的历史数据
    - 用固定等待时间代替业务完成信号
    - 把接口细节断言到实现噪音级别，导致一改文案或布局就全碎
    - 把“保存成功 toast 消失”“banner 自动消失”“dialog 焦点变化”当成 real suite 的唯一完成条件

### 6. Tests Required

- 新增 E2E 场景时，至少覆盖：
    - 入口页能打开
    - 核心动作可执行
    - 动作后的用户可见结果发生变化
    - 失败时能通过 trace / screenshot / video 回放
- 变更包含增删改动作时，优先成组覆盖：
    - 加载
    - 变更
    - 回滚或清理
- 插件依赖这类跨层场景，至少要有：
    - 画像加载
    - 官方依赖禁用 / 恢复
    - 自定义依赖上传 / 删除

### 7. Wrong vs Correct

#### Wrong

- 在表格里直接写：`page.locator('button').nth(5).click()`
- 让插件依赖测试直接请求远端 Maven，再等待真实依赖分析完成
- 上传测试把真实驱动 JAR 提交进仓库

#### Correct

- 为图标按钮补稳定锚点，例如 `data-testid="plugin-disable-official-mysql-connector-j"`
- 保留真实登录链路，只把插件依赖接口改为固定夹具
- 在测试输出目录临时生成占位 `.jar` 文件，再通过上传接口夹具完成闭环

---

## 模板：插件依赖 E2E

- 场景文件：`frontend/e2e/plugin-dependency-template.spec.ts`
- 夹具文件：`frontend/e2e/helpers/plugin-dependency-template.ts`
- 适用模式：
    - 需要验证“打开详情 -> 选择画像 -> 变更依赖 -> 看见结果”的完整链路
    - 真实后端可登录，但业务数据不适合依赖真实仓库或共享数据库

## 模板：安装 / 升级主流程

- 安装向导：
    - 场景文件：`frontend/e2e/install-wizard-template.spec.ts`
    - 反例文件：`frontend/e2e/install-wizard-negative.spec.ts`
    - 夹具文件：`frontend/e2e/helpers/install-wizard-template.ts`
    - 内部挂载页：`frontend/app/(main)/e2e-lab/install-wizard/page.tsx`
- 升级准备：
    - 场景文件：`frontend/e2e/upgrade-prepare-template.spec.ts`
    - 反例文件：`frontend/e2e/upgrade-prepare-negative.spec.ts`
    - 夹具文件：`frontend/e2e/helpers/upgrade-prepare-template.ts`
- 适用模式：
    - 需要覆盖“预检查 -> 参数确认 -> 下一页”这类多步骤主流程
    - 页面依赖真实登录和真实路由，但不适合依赖真实资产仓库或共享集群数据

## CI 选择策略

- PR 门禁默认只跑“命中的功能域 E2E smoke”，不全量跑。
- 当前主门禁运行在：
    - `ubuntu-24.04`（x64）
    - `ubuntu-24.04-arm`（arm64）
- 增量选择脚本：`frontend/scripts/e2e/select-e2e-specs.mjs`
- 选择计算先由单独 job 执行一次，再把结果复用给 x64 / arm64 两个 smoke job，避免重复算 diff。
- 同一 PR 或分支有新提交时，旧的 CI run 会自动取消，避免占用 runner。
- 当前映射域：
    - 插件依赖
    - 安装向导（正向 + 失败态）
    - 升级准备（正向 + 阻断态）
    - 登录 / Dashboard
- 真实环境 E2E（例如 installer-real / config-real / upgrade-real / plugin-real）不进入 smoke 选择器，改由统一 `E2E` workflow 的 suite 级 path filter 决定是否运行
- 工作台真实套件使用 `frontend/e2e/workbench-real.spec.ts` 与 `frontend/e2e/helpers/workbench-real.ts`，通过 `pnpm run test:e2e:workbench-real` 启动。
- 下列共享变更会退化为全量 E2E smoke：
    - `frontend/playwright.config.ts`
    - `frontend/package.json`
    - `frontend/pnpm-lock.yaml`
    - `frontend/components/ui/**`
    - `frontend/lib/i18n/**`
    - `config.e2e.yaml`
- 选择器新增了新功能域后，要同步更新：
    - `frontend/scripts/e2e/select-e2e-specs.mjs`
    - `.trellis/spec/frontend/index.md`
    - 对应模板样例入口

## 落地清单

- 新写 E2E 前，先判断它属于：
    - 真后端 smoke
    - 真实登录 + 定向夹具
    - 全 mock smoke
- 先补稳定锚点，再写用例，不要把脆弱选择器写进 spec。
- spec 命名优先按“用户流”命名，而不是按组件命名。
- 每个新增业务流都应该提供一个“最小可运行样例”，供后续场景复制扩展。

## Real E2E 注意事项

- **优先验证“最终状态”，不要死等中间 UI 态。**
    - 例如：
        - 配置同步后优先看 API 返回或真实文件落盘
        - 插件下载后优先看 metadata / 本地文件
        - 升级后优先看任务步骤与目标目录内容
- **关键用户入口仍然要走 UI。**
    - 例如：
        - 打开详情
        - 选择 profile
        - 点击智能修复
        - 打开版本对比
    - 但保存、同步、回滚、从节点回灌这类动作，如果页面交互不稳定，可以沉到 API helper。
- **避免形成等待死循环。**
    - 不要把这些当成唯一收敛条件：
        - toast 自动消失
        - tooltip / banner 自动隐藏
        - modal 焦点切换
        - 页面某个按钮临时可点
    - 应优先使用：
        - API 轮询
        - 真实文件内容轮询
        - 明确的数据版本变化
- **real suites 要主动输出阶段日志。**
    - 例如：
        - `cluster configs initialized from node`
        - `template synced to node and file updated`
        - `plugin metadata downloaded`
    - 这样 GitHub CI 卡住时能马上知道卡在哪一步。
- **本地和 CI 的资源策略不同。**
    - 本地：
        - 可以清理临时目录
        - 真实 E2E 运行前后要注意停掉残留 SeaTunnel / MinIO / agent 进程
        - 8G 开发机上优先把 SeaTunnel JVM 调小后再跑
    - CI：
        - runner 是一次性的
        - 默认不做破坏性本地目录删除
        - 仍然要停掉容器和子进程，避免影响同 job 后续步骤
