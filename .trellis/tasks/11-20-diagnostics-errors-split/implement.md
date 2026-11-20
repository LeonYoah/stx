# 实施清单
1. 后端过滤参数 + registry + Go 测试。
2. 前端类型与 `error-utils.ts`（+单测）。
3. `ErrorGroupDetail.tsx`、`ErrorsPanel.tsx`，workspace 接入，删除旧组件。
4. i18n 中英。
5. vitest + E2E mock 模板（`e2e/helpers/diagnostics-errors-template.ts` + `e2e/diagnostics-errors-template.spec.ts`，参照 policies 模板）。

## 验证
```bash
go test ./internal/apps/diagnostics/... ./internal/operation/... ./internal/cmd/...
cd frontend && pnpm exec tsc --noEmit
cd frontend && pnpm exec vitest run components/common/diagnostics components/common/monitoring lib/i18n/__tests__/translation-coverage.test.ts
cd frontend && pnpm exec eslint <changed files>
cd frontend && E2E_FRONTEND_PORT=3100 pnpm exec playwright test e2e/diagnostics-errors-template.spec.ts e2e/diagnostics-policies-template.spec.ts
```
