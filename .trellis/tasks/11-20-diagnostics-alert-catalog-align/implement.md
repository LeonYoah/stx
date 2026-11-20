# 实施清单

1. 后端目录重建 + 单测（monitoring 包：键集合等于集群目录∪TaskRules、无 stx_*、英文名取 NameEN、scope/category 派生）。修复因目录变化失败的既有测试（只调整断言到新契约，不放宽语义）。
2. diagnostics `unknown_alert_rule_key` + 单测（存量含 stx_* → 失效；gc_time_high 创建通过校验）。
3. 前端 types / AutoPolicyConfigPanel / i18n + vitest 单测（两组分组、未知键徽章可移除、白名单）。
4. 规范：`cluster-alert-rules.md`、`platform-self-monitoring.md` §4。
5. 检查 CLI（`internal/cmd/diagnostics.go` 帮助、`internal/operation/registry.go` catalog 描述）是否需要措辞调整。

## 验证命令

```bash
go test ./internal/apps/monitoring/... ./internal/apps/diagnostics/... ./internal/cmd/... ./internal/operation/...
cd frontend && pnpm exec tsc --noEmit
cd frontend && pnpm exec vitest run components/common/diagnostics lib/i18n/__tests__/translation-coverage.test.ts
```

## 风险

- monitoring 包内可能有测试断言旧目录含 `stx_*`（`platform_alert_rules_test.go`）——按新契约改断言。
