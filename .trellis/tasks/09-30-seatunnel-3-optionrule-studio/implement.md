# 实现记录：OptionRule Studio 一期

## 已完成

1. **权威校验**：`OptionRuleValidationService` + `ConfigValidationService`
   - `/api/v1/config/validate` 默认跑引擎 `ConfigValidator`（含 3.0 VC/CR）
   - 请求字段 `validateOptionRules=false` 可关闭
   - 草稿保存仍不拦；Studio「校验 / 提交前 preflight」走此路径

2. **Schema 抽取**：`OptionRuleConstraintCodec`（反射，双代际同源）
   - `PluginOptionSchemaResult.valueConstraints` / `conditionRules`
   - Go / 前端 DTO 透传 `value_constraints` / `condition_rules`

3. **Studio hover**：展示条件表达式摘要 + 匹配字段的值约束提示

4. **行内 Monaco 红线**：`sync-option-rule-assist` + `setModelMarkers`
   - 内容变更 debounce 350ms 刷新
   - valueConstraints 失败 → error；条件已满足缺 CONDITIONAL 字段 → warning
   - `{{var}}` 跳过值校验

5. **conditionRules 收敛补全**：插件块内 option key 补全
   - 命中子规则的 key 排前；仅属于未命中子规则的 key 降权
   - 顺带补上「写 key」场景的 Property 补全（原先只有枚举值补全）

## 验证

- `vitest` sync-option-rule-assist / sync-studio-utils
- proxy / Go 一期测例仍适用
