# 实施计划：国际化治理与硬规则检测

开工前先读 `.trellis/spec/backend/quality-guidelines.md`、`.trellis/spec/backend/error-handling.md` 与 `.trellis/spec/frontend/index.md`，并确认 PRD 中 2 个待确认项（日志是否纳入规则 A、默认语言是否为 `zh`）。

## Phase 1 · 后端统一 Locale Context 与 Middleware

- [ ] 新增 `internal/pkg/locale/locale.go` 与 `internal/pkg/locale/locale_test.go`：
  - 实现 `Normalize`、`ParseAcceptLanguage`（含 `q=` 权重排序）、`WithLocale`、`FromContext`、`Pick`；
  - 表驱动单测覆盖 `zh`、`zh-CN`、`en`、`en-US`、`Accept-Language` 多值权重、非法值及空 context 兜底。
- [ ] 在 `internal/router/middlewares.go` 新增 `localeMiddleware()`，并在 `internal/router/router.go` 全局中间件链注册；补中间件单测覆盖优先级 `?lang=` > `?language=` > `?locale=` > `Accept-Language` > 默认值。
- [ ] 迁移 4 处现有手写语言解析为 `locale.FromContext(c.Request.Context())`：
  - `internal/apps/diagnostics/handler.go`
  - `internal/apps/diagnostics/troubleshooting_handler.go`
  - `internal/apps/cluster/scale_handler.go`
  - `internal/apps/monitoring/alert_policy_handler.go`
- [ ] 验证：`go test ./internal/pkg/locale/... ./internal/router/... ./internal/apps/diagnostics/... ./internal/apps/cluster/... ./internal/apps/monitoring/...`

## Phase 2 · 前端统一透传与存量双语清零

- [ ] 改造 `frontend/lib/services/core/api-client.ts`：
  - 请求拦截器统一注入 `Accept-Language: getCurrentLocale()`（尊重调用方显式覆写）；
  - 响应拦截器中 5 条硬编码中文兜底错误（超时、403、413、5xx、网络失败）按 `getCurrentLocale()` 输出单语，并补单测。
- [ ] 清理前端 9 处存量双语字面量并补齐 `zh.json` / `en.json` 对应词条：
  - `frontend/lib/services/plugin/plugin.service.ts`（1 处）
  - `frontend/lib/services/installer/installer.service.ts`（5 处）
  - `frontend/components/common/layout/ModuleNavTabs.tsx`（2 处）
  - `frontend/components/common/sync/CheckpointPanels.tsx`（1 处）
- [ ] 在 `frontend/lib/i18n/__tests__/translation-coverage.test.ts` 新增断言：禁止 `zh.json` / `en.json` 值内出现双语拼接。
- [ ] 验证：`cd frontend && pnpm test` 与 `cd frontend && pnpm exec tsc --noEmit`。

## Phase 3 · 硬规则检测与 CI / Makefile / Spec 接入

- [ ] 新增 `scripts/check_i18n_policy.py`（带 Apache 2.0 License 头）：
  - 实现规则 A（`FORBIDDEN_BILINGUAL_LITERAL`，含技术斜杠防误报与 `// i18n-lint:ignore` 豁免）与规则 B（`FORBIDDEN_RAW_LOCALE_PARSE`）；
  - 支持 `--base-ref` / `--head-ref` diff 模式、`--baseline` 棘轮校验模式、`--update-baseline` 生成模式。
- [ ] 新增 `scripts/check_i18n_policy_test.py`，覆盖正例、真实反例（`CDC / Log Stream`、`Subject.getSubject / JEP 486`、`严重 %d / 告警 %d`）、豁免标记、diff 模式与 baseline 增减场景。
- [ ] 生成初始 `scripts/i18n_baseline.json`（确认 `frontend/` 基线为 0）。
- [ ] 接入门禁与开发规范：
  - `.github/workflows/ci-main.yml`：在静态检查 job 加入 `check_i18n_policy_test.py`、diff 门禁、baseline 校验；
  - `Makefile`：新增 `check_i18n` 并加入 `pre_commit`；
  - `.trellis/spec/backend/quality-guidelines.md` 与 `.trellis/spec/backend/error-handling.md`：补充 `internal/pkg/locale`、`error_code` 与 `check_i18n` 门禁约定。

## Phase 4 · 全量验证与后续治理跟踪

- [ ] 全量回归：
  - `python3 -m unittest scripts/check_i18n_policy_test.py`
  - `python3 scripts/check_i18n_policy.py --baseline scripts/i18n_baseline.json`
  - `python3 scripts/check_license.py --base-ref origin/main --head-ref HEAD`
  - `go test ./...`
  - `(cd agent && go test ./...)`
  - `cd frontend && pnpm test && pnpm exec tsc --noEmit`
- [ ] P1/P2 后续清债（另开子任务或随业务迭代推进）：按 `installer` → `plugin` → `cluster` → `stupgrade` → 其余模块与 `agent/` 顺序收紧 `scripts/i18n_baseline.json`。

## 回滚

- `localeMiddleware` 仅向 `context.Context` 注入值且兼容原有 query 参数，回滚可直接移除中间件注册与 `internal/pkg/locale` 引用；
- `check_i18n_policy.py` 为独立 CI 静态检查脚本，若出现误报可通过 `// i18n-lint:ignore` 临时豁免或从 `ci-main.yml` 移除该步骤，不影响运行时行为。
