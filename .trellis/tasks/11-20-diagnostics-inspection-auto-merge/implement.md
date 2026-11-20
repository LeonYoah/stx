# 实施清单
1. 后端：字段/过滤/统计/回填/registry + Go 测试。
2. `DiagnosticsClusterFilter` + Workspace 清理 + 各面板挂接。
3. InspectionsPanel 视图切换 + records 行自动徽章 + auto_policy 过滤徽章。
4. PoliciesPanel 行重排 + `policy-describe.ts` + 详情底部跳转。
5. i18n。
6. vitest 更新/新增；E2E 三个 spec 更新。
7. `.trellis/spec/frontend/directory-structure.md` 同步。

## 验证
```bash
go test ./internal/apps/diagnostics/... ./internal/operation/... ./internal/cmd/... ./internal/db/...
cd frontend && pnpm exec tsc --noEmit
cd frontend && pnpm exec vitest run components/common/diagnostics components/common/monitoring components/common/troubleshooting lib/i18n/__tests__/translation-coverage.test.ts
cd frontend && pnpm exec eslint <changed files>
cd frontend && E2E_FRONTEND_PORT=3160 pnpm exec playwright test e2e/diagnostics-inspections-template.spec.ts e2e/diagnostics-errors-template.spec.ts e2e/diagnostics-policies-template.spec.ts
```
