# 实施清单

1. 抽 `policy-form-utils.ts`，迁移既有单测导入。
2. 抽 `PolicyFormBody.tsx`（Arc 控件 + 单列布局 + 脏态 + ConfirmMorph 删除），保留 #170 的规则选择器行为。
3. `PoliciesPanel.tsx` + `monitoring.module.css` 列模板。
4. `DiagnosticsWorkspace.tsx` 外壳 + 顶层标签脏态拦截；删除 `AutoPolicyConfigPanel.tsx`，更新 `index.ts`。
5. i18n 中英；删死键。
6. vitest：`PoliciesPanel` 列表/筛选/脏态拦截/删除确认；picker 测试迁到 `PolicyFormBody`。
7. E2E mock 模板：`e2e/helpers/diagnostics-policies-template.ts` + `e2e/diagnostics-policies-template.spec.ts`（参照 `plugin-dependency-template`）。
8. `.trellis/spec/frontend/directory-structure.md` 如列有诊断组件则同步。

## 验证

```bash
cd frontend && pnpm exec tsc --noEmit
cd frontend && pnpm exec vitest run components/common/diagnostics components/common/monitoring lib/i18n/__tests__/translation-coverage.test.ts
cd frontend && pnpm exec eslint <changed files>
cd frontend && pnpm exec playwright test e2e/diagnostics-policies-template.spec.ts
```
