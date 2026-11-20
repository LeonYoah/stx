# 实施清单
1. `inspection-utils.ts` + 单测。
2. `StartInspectionDialog.tsx`、`InspectionReportDetail.tsx`、`InspectionsPanel.tsx`；workspace 接入；删旧组件。
3. CSS 列模板（媒体查询内）。
4. i18n 中英。
5. vitest + E2E（`e2e/helpers/diagnostics-inspections-template.ts`、`e2e/diagnostics-inspections-template.spec.ts`）。

## 验证
```bash
cd frontend && pnpm exec tsc --noEmit
cd frontend && pnpm exec vitest run components/common/diagnostics components/common/monitoring lib/i18n/__tests__/translation-coverage.test.ts
cd frontend && pnpm exec eslint <changed files>
cd frontend && E2E_FRONTEND_PORT=3140 pnpm exec playwright test e2e/diagnostics-inspections-template.spec.ts e2e/diagnostics-errors-template.spec.ts e2e/diagnostics-policies-template.spec.ts
```
