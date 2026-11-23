# 国际化治理与硬规则检测

## 背景

STX 后端与前端长期存在大量硬编码的 `中文 / English` 同屏双语拼接字面量，与仓库既定的「双语支持 = 按当前语言单语切换」规范（`.trellis/spec/backend/quality-guidelines.md`、`.trellis/spec/frontend/index.md`）相违背：

- 扫描 `internal/` 与 `agent/` 非测试 Go 代码，存在约 **1,350+ 处** `中文 / English` 拼接字面量（分布于 128 个文件，集中在 `agent/`、`installer`、`cluster`、`plugin`、`stupgrade` 等模块）；
- 扫描 `frontend/` 非测试生产代码，仍存在 **9 处** `中文 / English` 拼接字面量（分布于 `installer.service.ts`、`plugin.service.ts`、`ModuleNavTabs.tsx`、`CheckpointPanels.tsx`），且 `frontend/lib/services/core/api-client.ts` 存在 5 条硬编码中文兜底报错；
- 前后端尚未建立全局统一的语言透传链路：`api-client.ts` 未注入 `Accept-Language`，仅约 10 处调用点手工拼接 `?lang=`；后端仅 4 处 handler 各自解析 `lang` / `language` / `locale` / `Accept-Language`，缺少统一 middleware 与 `context.Context` 存取入口；
- 为兜底历史双语拼接，前后端已长出多套补偿解析逻辑（如 `frontend/lib/i18n/localize-text.ts` 的 `splitLocalizedPair`、`internal/apps/diagnostics/display_text.go` 的 `findDiagnosticLocalizedPair`），维护成本持续上升。

本任务目标是在不一次性重构全部 1,350+ 处存量的前提下，先收口全局语言链路、确立分层国际化契约，并用 CI 硬规则检测锁死新增与存量基线，再按暴露面分阶段清零历史债务。

## 范围

### P0（首期落地）
1. **全局语言透传与 Context**：
   - 前端 `api-client.ts` 统一注入 `Accept-Language: getCurrentLocale()`，并本地化拦截器内的 5 条兜底网络/状态报错；
   - 后端新增 `internal/pkg/locale` 与 Gin `localeMiddleware`，归一化 `?lang=`、`?language=`、`?locale=`、`Accept-Language` 并写入 `context.Context`；
   - 迁移现有 4 处手写语言解析 handler（`diagnostics/handler.go`、`diagnostics/troubleshooting_handler.go`、`cluster/scale_handler.go`、`monitoring/alert_policy_handler.go`）统一从 context 读取。
2. **前端存量清零**：清理 `frontend/` 中 9 处双语拼接字面量，使前端双语拼接基线归零。
3. **硬规则检测与 CI 门禁**：
   - 新增 `scripts/check_i18n_policy.py` 及单测 `scripts/check_i18n_policy_test.py`；
   - 建立增量 Diff 阻断 + `scripts/i18n_baseline.json` 存量棘轮（只减不增）；
   - 在 `frontend/lib/i18n/__tests__/translation-coverage.test.ts` 增加语言包单语校验；
   - 接入 `.github/workflows/ci-main.yml`（纳入 `ci-gate`）与 `Makefile`（`check_i18n` 纳入 `pre_commit`）；
   - 同步更新 `.trellis/spec/backend/quality-guidelines.md` 与 `.trellis/spec/backend/error-handling.md`。

### P1 / P2（后续增量治理，拆分子任务或随迭代推进）
- **P1**：高频直接进入 Web UI 的后端模块（`installer`、`plugin`、`cluster`、`stupgrade`）的错误与提示迁移为 `error_code` 或按 locale 单语输出；
- **P2**：其余后端模块与 `agent/` 运行日志逐步收敛为纯英文单语，持续收紧 `scripts/i18n_baseline.json`。

## 需求

- **R1 统一语言协商与向后兼容**：
  - 后端按 `?lang=` → `?language=` → `?locale=` → `Accept-Language` 优先级归一化为 `zh` / `en`，未指定时默认 `zh`；
  - 所有现有带 `?lang=` / `?language=` / `?locale=` 的 API、SSE 与下载链接保持完全兼容。
- **R2 分层文案契约**：
  - 面向 UI 的 API 业务错误与状态原因：后端返回稳定 `error_code`（复用既有 `error_code` 协议）+ 可选结构化参数 + 单语兜底 `error_msg`，前端按 `apiErrors.<code>` 翻译，未知码白名单回退 `error_msg`；
  - 后端生成的整句内容（诊断报告、扩缩容计划、告警内容、SSE 进度）：在生成时按 `locale.FromContext(ctx)` 输出单语；
  - 内部运行日志与 `agent/` 日志：统一纯英文单语；
  - CLI 输出：优先消费 `error_code` 与 `error_msg`，不依赖解析双语斜杠文本。
- **R3 硬规则检测（低误报、可执行、可渐进收敛）**：
  - **规则 A**：禁止在生产代码字符串字面量中新增 `中文 / English` 双语拼接；必须避开技术斜杠与同语种枚举斜杠（如 `CDC / Log Stream`、`Subject.getSubject / JEP 486`、`严重 %d / 告警 %d`），并支持显式注释豁免（`// i18n-lint:ignore`）；
  - **规则 B**：禁止在 `internal/apps/` 新增手写 `c.Query("lang"|"language"|"locale")` 或 `c.GetHeader("Accept-Language")`，强制走 `internal/pkg/locale`；
  - **规则 C**：禁止 `frontend/lib/i18n/locales/{zh,en}.json` 的翻译值包含双语拼接。
- **R4 增量阻断 + 存量棘轮**：
  - PR / Push Diff 中任何新增违规行直接阻断 CI；
  - 全量基线检查中，任何文件违规计数超过 `scripts/i18n_baseline.json` 记录值直接阻断 CI；计数下降时提示可同步收紧基线。

## 待确认项（评审阶段）

1. **后端 & `agent/` 的 `logger.*` 日志是否同步纳入规则 A 的新增阻断**（默认建议：纳入，新增日志统一纯英文单语；若需放宽可单独豁免日志调用）。
2. **未传任何语言参数与 `Accept-Language` 时的后端默认语言**（默认建议：`zh`，与现有 `diagnostics`、`frontend/lib/i18n/config.ts` 的 `defaultLocale = 'zh'` 保持一致）。

## 验收标准

- [ ] `api-client.ts` 所有请求均携带 `Accept-Language` 头，且 5 条拦截器兜底错误在中英文切换下均展示对应单语。
- [ ] `localeMiddleware` 在单测中覆盖 `?lang=`、`?language=`、`?locale=`、`Accept-Language`（含 `zh-CN,zh;q=0.9,en;q=0.8`、`en-US`）及缺省兜底，4 处现有 handler 迁移后行为与既有单测全部通过。
- [ ] `frontend/` 生产代码中双语拼接字面量清零；`translation-coverage.test.ts` 新增单语断言并通过。
- [ ] `scripts/check_i18n_policy.py` 通过配套单测（覆盖正例、技术斜杠反例、豁免标记、diff 模式与 baseline 棘轮模式），并在当前仓库全量运行返回 0。
- [ ] `.github/workflows/ci-main.yml` 接入 `check_i18n_policy.py` 并纳入 `ci-gate`，`Makefile` 新增 `check_i18n` 且挂入 `pre_commit`。
- [ ] `go test ./...`、`(cd agent && go test ./...)`、`cd frontend && pnpm test`、`cd frontend && pnpm exec tsc --noEmit`、`python3 scripts/check_license.py` 全部通过。

## 不在范围

- 本期（P0）不一次性改写 `internal/` 与 `agent/` 下的 1,350+ 处历史双语字面量（纳入 P1/P2 逐步收紧）。
- 不移除现有前端 `localizeBackendText` 与 `diagnostics/display_text.go` 的存量兜底解析（待 P1/P2 存量清零后再下线）。
- 不引入第三语言或外部翻译管理平台。
