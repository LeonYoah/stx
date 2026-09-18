# UI 自动化与 CI 框架设计（chunk upload）

## Goal

围绕刚落地的 **chunk upload 主链路**，为 STX 控制台建立一套可持续演进的 **UI 自动化 + CI 执行框架**。

本任务的重点不是一次性把所有 E2E 用例都写完，而是先把后续能稳定扩展的“测试底座”设计清楚，并明确 MVP 的首批落地点。

本任务确认采用以下总体方向：

- **保留 Vitest + Testing Library**：负责函数、hooks、组件逻辑测试；
- **新增 Playwright 作为唯一 UI 自动化主框架**：负责页面级交互、上传、登录、弹窗、告警提示等真实浏览器链路；
- **CI 分层执行**：PR 跑快速 lane，nightly / main 跑完整 lane；
- **先以 chunk upload 为切入口**，后续扩展到日志查看、过滤交互、集群操作等关键页面。

---

## Requirements

### R1. 明确测试分层与框架边界

项目需要形成统一的测试分层，避免“单测、组件测、页面测、联调测”职责混乱。

要求：

- Vitest + Testing Library 继续承担：
  - 工具函数测试
  - service / hook 逻辑测试
  - 组件级交互测试（不依赖真实浏览器）
- Playwright 承担：
  - 页面级 UI 自动化
  - 真实文件上传交互
  - 弹窗、进度条、toast、错误态
  - 登录态与关键业务链路验证
- MVP 阶段**不同时引入 Cypress**，避免双框架并存导致维护成本上升；
- 需要明确每类问题应优先落在哪一层测试，避免滥用 E2E 替代单测。

### R2. 设计项目级 Playwright 目录结构与运行方式

需要为仓库定义一套能直接落地的 Playwright 组织方式。

至少明确：

- 配置文件位置（如 `frontend/playwright.config.ts`）；
- 用例目录（如 `frontend/e2e/`）；
- 辅助目录（如 `fixtures/`、`helpers/`、`test-data/`）；
- package.json 中的脚本命名；
- 本地执行方式；
- CI 执行方式；
- 报告、trace、截图、视频产物的保存策略。

要求该结构既适用于：

- **mock API 的快速 UI 测试**
- **真实后端启动的全链路测试**

### R3. 设计 chunk upload 的首批自动化场景矩阵

围绕本次新增的 `/api/v1/packages/upload/chunk` 主链路，需要明确首批自动化覆盖范围。

MVP 必须至少定义以下用例：

1. **正常多分片上传成功**
   - 选择一个大于 8MB 的 tar.gz 文件；
   - 看到进度递增；
   - 上传成功后 UI 正确提示；
   - 包列表中可看到目标版本。

2. **最后一片完成后的状态收敛**
   - 进度条最终到 100%；
   - 上传按钮状态恢复；
   - 弹窗关闭或成功态符合设计预期。

3. **后端返回 409**
   - UI 展示正确冲突/顺序错误提示；
   - 不允许出现“假成功”。

4. **后端返回 413**
   - UI 正确提示文件过大或被拒绝；
   - 上传流程中断。

5. **中途失败**
   - 第 N 片失败；
   - UI 进入失败态；
   - 用户可以重新上传或重试（若产品支持）。

同时要明确哪些用例适合：

- Playwright mock API 层
- Playwright 真后端层
- Vitest service / 组件层

### R4. 设计服务启动与测试环境 harness

UI 自动化不能依赖人工手动先启动服务。

必须定义统一的测试 harness，至少说明：

- 前端如何由 Playwright 自动拉起（如 `webServer`）；
- 后端如何拉起：
  - 最小 real backend 模式
  - 或 mock / fake backend 模式
- 测试环境需要哪些基础配置；
- 测试数据如何准备；
- 测试结束如何清理；
- 如何避免测试相互污染。

MVP 建议至少支持两种模式：

#### 模式 A：快速 UI 模式
- 前端真实启动；
- API 由 Playwright route mock 或最小 fake server 响应；
- 用于 PR 快速检查。

#### 模式 B：真实链路模式
- 前端真实启动；
- 后端真实启动；
- 验证 chunk upload 端到端链路；
- 用于 nightly / main 增强检查。

### R5. 设计 CI 分层执行方案

需要在现有 `.github/workflows/ci-main.yml` 基础上，定义 UI 自动化如何进入 CI。

要求至少明确两条执行 lane：

#### PR 快速 lane
- 保持当前 backend tests
- 保持当前 frontend vitest
- 新增 Playwright smoke / mock UI 用例
- 控制时长，避免显著拖慢 PR

#### Nightly / Main 完整 lane
- 启动真实服务
- 执行完整 Playwright E2E
- 覆盖 chunk upload 异常矩阵
- 上传 report / trace / screenshot artifact

还需明确：

- 失败时保留哪些 artifact；
- 是否分 job 执行；
- 如何按路径变更触发；
- 如何避免对普通前端改动引入过高 CI 成本。

### R6. 测试稳定性与可维护性约束

需要在设计中明确测试稳定性原则，避免后续 E2E 大量 flaky。

至少约定：

- 优先使用可访问性选择器 / 语义角色；
- 仅在必要时补 `data-testid`；
- 避免过度依赖文案全文匹配；
- 避免固定 sleep，优先等待真实状态；
- 登录态、上传结果、toast、列表刷新要有稳定断言点；
- 对 chunk upload 这类长链路，需要给出合理超时与失败诊断方案。

### R7. 明确本任务的 1 / 2 / 3 交付物

本任务至少要产出以下三项明确结果：

#### 1. 项目专用 Playwright 结构与分层方案
- 目录结构
- 脚本命名
- 测试分层原则
- 环境启动方式

#### 2. chunk upload 首个 UI/E2E 用例方案
- 用例路径
- 测试步骤
- 断言点
- mock / real backend 两种执行建议

#### 3. GitHub Actions 接入草稿
- job 拆分方式
- 触发条件
- 产物上传
- PR 与 nightly 差异

---

## Acceptance Criteria

- [ ] 已明确推荐框架组合：Vitest 保留，Playwright 作为唯一 UI 自动化主框架
- [ ] 已给出适用于本项目的 Playwright 目录结构与脚本约定
- [ ] 已定义 chunk upload 首批 UI 自动化场景矩阵
- [ ] 已明确 mock API 模式与真实后端模式的职责边界
- [ ] 已明确 PR 快速 lane 与 nightly/full lane 的 CI 设计
- [ ] 已明确报告、trace、截图等失败诊断产物策略
- [ ] 已明确测试稳定性原则（selector、等待、超时、数据隔离）
- [ ] 已产出“1 / 2 / 3”三项交付物说明，便于后续直接实施

---

## Technical Notes

### Current code / workflow baseline

当前仓库已有以下基础，可直接复用：

- 前端已有 `Vitest + Testing Library`
  - `frontend/vitest.config.ts`
  - `frontend/vitest.setup.ts`
- 前端已有 chunk upload 主链路
  - `frontend/lib/services/installer/installer.service.ts`
  - `frontend/components/common/installer/UploadPackageDialog.tsx`
  - `frontend/components/common/installer/PackageMain.tsx`
- CI 已有 frontend / backend 基础检查
  - `.github/workflows/ci-main.yml`
- 前端交付构建统一为：
  - `cd frontend && pnpm run pack:standalone`

### Suggested file impact

后续实施时，可能涉及但不限于：

- `frontend/package.json`
- `frontend/playwright.config.ts`
- `frontend/e2e/**/*.spec.ts`
- `frontend/e2e/fixtures/*`
- `frontend/e2e/helpers/*`
- `.github/workflows/ci-main.yml`
- `frontend/scripts/e2e/*.sh` 与 `frontend/scripts/e2e/select-e2e-specs.mjs`
- 必要时的测试专用 mock / fake server 文件

### Suggested implementation order

建议按以下顺序推进：

1. 明确 Playwright 目录结构、脚本与本地执行方式
2. 先补一个最小 smoke：登录 / 打开包管理 / 上传文件 / 成功提示
3. 再补 chunk upload 异常矩阵
4. 接入 PR 快速 lane
5. 最后补 nightly / full e2e lane 与 artifact 收集

### Code-spec depth warning

本任务虽然以“测试”为主，但实际涉及以下跨层契约，不能只停留在原则描述：

- Playwright 如何启动前端 / 后端；
- mock API 与真实 API 的职责边界；
- CI job 触发条件与执行命令；
- chunk upload 各类错误码在 UI 层的断言方式；
- 报告 / trace / artifact 的保留契约。

因此后续若进入实施阶段，需要同步补充相关 code-spec，而不只是 PRD。

### Non-goals for MVP

本任务 MVP 暂不追求：

- 一次性覆盖所有控制台页面；
- 同时支持 Playwright 与 Cypress 双框架；
- 首阶段就做完整跨浏览器矩阵；
- 引入过重的容器化全量测试平台。
