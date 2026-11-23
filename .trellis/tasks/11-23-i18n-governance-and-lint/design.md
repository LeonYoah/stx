# 设计：国际化治理与硬规则检测

## 1. 结论

- **语言透传收口到一处**：前端 `api-client.ts` 请求拦截器统一注入 `Accept-Language`，后端通过全局 Gin `localeMiddleware` 将语言归一化到 `context.Context`（`internal/pkg/locale`），消灭各 handler 手工读取 query/header 的分叉。
- **错误与文案按归属分层**：进 UI 的业务错误复用现有 `error_code` + `error_msg` 契约，由前端按 `error_code` 本地化；后端必须生成整句的场景（报告、计划、告警、SSE）按 `locale.FromContext(ctx)` 单语生成；内部日志与 `agent/` 日志统一纯英文单语。
- **硬规则门禁采用「Diff 拦新增 + Baseline 锁存量」**：新增零外部依赖脚本 `scripts/check_i18n_policy.py`，复用仓库 `check_license.py` / `check_db_compatibility.py` 的 CI 接入模式，几秒内完成检查且不阻塞日常开发。

## 2. 现状（代码事实）

### 2.1 语言参数链路现状
- 前端 `frontend/lib/i18n/config.ts` 提供 `getCurrentLocale(): 'zh' | 'en'`（`localStorage.locale` → `navigator.language` → `'zh'`），但 `frontend/lib/services/core/api-client.ts` 未注入 `Accept-Language` 或 `lang`，仅在 `diagnostics.service.ts`、`scale.service.ts`、`monitoring.service.ts` 等少量调用点手工拼 `?lang=`。
- `api-client.ts` 响应拦截器（98–122 行）包含 5 条硬编码中文兜底文案（请求超时、403、413、5xx、网络请求失败）。
- 后端仅有 4 个文件手工读取语言参数且字段名不统一：
  - `internal/apps/diagnostics/handler.go:882`：`c.Query("lang")`
  - `internal/apps/diagnostics/troubleshooting_handler.go:43-52`：`language` → `locale` → `Accept-Language`
  - `internal/apps/cluster/scale_handler.go:114,181,367`：`c.Query("lang")`
  - `internal/apps/monitoring/alert_policy_handler.go:33`：`c.Query("lang")`
- 后端全局中间件注册位于 `internal/router/router.go:191-211`，自定义中间件定义在 `internal/router/middlewares.go`。

### 2.2 双语拼接存量分布（2026-11-23 静态扫描）
- **Go 生产代码（`internal/` + `agent/`，排除 `*_test.go`、`vendor/`、`proto/`）**：约 **1,350+ 处**，涉及 **128 个文件**：
  - 高频区域：`agent/internal`（220）、`internal/apps/installer`（201）、`agent/cmd`（182）、`internal/apps/cluster`（146）、`internal/apps/plugin`（136）、`internal/apps/stupgrade`（102）、`internal/apps/selfupgrade`（58）、`internal/apps/agent`（33）、`internal/grpc`（41）、`internal/router`（28）、`internal/apps/monitoring`（25）、`internal/apps/monitor`（24）、`internal/apps/discovery`（24）。
  - 按上下文分类：日志调用约 398 处、`errors.New/fmt.Errorf` 约 313 处、HTTP 响应直接写入约 122 处、其余字面量/结构体字段约 618 处。
- **前端生产代码（`frontend/`，排除测试、`locales/`、`prototype/`）**：共 **9 处**真实双语拼接：
  - `frontend/lib/services/plugin/plugin.service.ts:410`
  - `frontend/lib/services/installer/installer.service.ts:98,140,155,208,228`
  - `frontend/components/common/layout/ModuleNavTabs.tsx:225,226`
  - `frontend/components/common/sync/CheckpointPanels.tsx:707`（注：`1260` 行 `增量日志流 (CDC / Log Stream)` 与 `1433` 行 `${Math.floor(norm.lagSeconds / 60)} 分 ...` 分别为技术缩略语斜杠与模板表达式除法，属于反例）

### 2.3 现有可复用契约
- `ApiError`（`frontend/lib/services/core/types.ts:43-50`）与后端 `sync` / `cluster` / `cli/client` 均已支持 `error_code` 字段，且 `api-client.ts` 已将 `error_code` 透传到 `apiError.errorCode`。
- `diagnostics` 的 `AutoPolicyInvalid` 已建立前端白名单映射模式（`INVALID_REASON_CODES` + `invalidReasons.unknown` 回退）。
- `frontend/lib/i18n/__tests__/translation-coverage.test.ts` 已在 `pnpm test` 中校验 `zh.json`/`en.json` 键集合一致、ICU 占位符一致与语法合法性。

## 3. 统一语言透传与 Context 设计

### 3.1 后端 `internal/pkg/locale`

新建轻量包 `internal/pkg/locale`（无外部依赖，纯标准库）：

```go
package locale

type Locale string

const (
	ZH Locale = "zh"
	EN Locale = "en"
)

func Normalize(raw string) (Locale, bool)
func ParseAcceptLanguage(header string) (Locale, bool)
func WithLocale(ctx context.Context, loc Locale) context.Context
func FromContext(ctx context.Context) Locale
func Pick(ctx context.Context, zh, en string) string
```

归一化规则：
- 去空白转小写后，前缀为 `en`（如 `en`、`en-us`、`en_gb`）→ `EN`；前缀为 `zh`（如 `zh`、`zh-cn`、`zh-hans`）→ `ZH`；其余返回 `("", false)`。
- `ParseAcceptLanguage` 按逗号拆分区段，解析可选 `;q=` 权重，按权重降序取第一个能识别为 `zh` 或 `en` 的语言标签。
- `FromContext(ctx)` 未设置时返回默认 `ZH`。

### 3.2 Gin `localeMiddleware`

在 `internal/router/middlewares.go` 新增 `localeMiddleware()`，挂载到 `internal/router/router.go` 的全局链（`r.Use(otelgin.Middleware(...), loggerMiddleware(), localeMiddleware(), self.GinMiddleware())`）：

1. 按顺序尝试解析：`c.Query("lang")` → `c.Query("language")` → `c.Query("locale")` → `c.GetHeader("Accept-Language")`；
2. 取首个合法结果（均缺失或无法识别时兜底 `locale.ZH`）；
3. 通过 `c.Request = c.Request.WithContext(locale.WithLocale(c.Request.Context(), loc))` 注入标准 `context.Context`，确保 handler → service → repository 全链路可用。

现有 4 处 handler 改造：
- `diagnostics/handler.go`：`diagnosticsLanguageFromRequest(c)` 改为读取 `locale.FromContext(c.Request.Context())`；
- `diagnostics/troubleshooting_handler.go`：删除手工三级兜底，改为 `string(locale.FromContext(c.Request.Context()))`；
- `cluster/scale_handler.go`：3 处 `c.Query("lang")` 改为 `string(locale.FromContext(c.Request.Context()))`；
- `monitoring/alert_policy_handler.go`：`AlertRuleCatalog(string(locale.FromContext(c.Request.Context())))`。

### 3.3 前端 `api-client.ts`

1. 请求拦截器统一注入 `Accept-Language`：
   - 若调用方未显式覆写该 header，则设置 `config.headers['Accept-Language'] = getCurrentLocale()`；
   - `EventSource`（SSE）与直接文件下载 URL 无法自定义请求头，继续保留 `?lang=${getCurrentLocale()}`。
2. 响应拦截器中 5 条兜底错误（超时、403、413、5xx、网络失败）改为通过小型 locale 映射或 `zh.json`/`en.json` 的 `apiErrors.network.*` 键按 `getCurrentLocale()` 输出单语。

## 4. 分层国际化契约（写进 Spec）

| 层级 / 场景 | 后端职责 | 前端 / CLI 职责 |
|---|---|---|
| **UI 业务错误 / 校验失败 / 状态原因** | 返回稳定 `error_code`（snake_case）+ 可选结构化 `data` 参数 + 单语兜底 `error_msg` | 前端在 `apiErrors.<code>` 翻译（支持白名单回退 `error_msg`）；CLI 消费 `error_code` + `error_msg` |
| **后端生成整句（报告 / 计划 / 告警 / SSE）** | 使用 `locale.Pick(ctx, zh, en)` 或按 `locale.FromContext(ctx)` 选择模板，仅输出单一语言 | 直接展示，不再做 `splitLocalizedPair` 切割 |
| **内部运行日志 & Agent 日志** | 统一纯英文单语，禁止拼双语 | 不直接透传给普通 UI toast |
| **持久化字段** | 存稳定枚举码与结构化参数（参考 `AutoPolicyInvalid`），不存双语拼接串 | 读取时按当前语言渲染 |

## 5. 硬规则检测设计

### 5.1 脚本 `scripts/check_i18n_policy.py`

零第三方依赖（仅 Python 3 标准库），支持两种运行模式（可组合）：
- **Diff 模式**：`python3 scripts/check_i18n_policy.py --base-ref origin/main --head-ref HEAD` — 仅检查 `git diff --unified=0` 新增行，发现任意违规即返回非 0；
- **Baseline 模式**：`python3 scripts/check_i18n_policy.py --baseline scripts/i18n_baseline.json` — 全量扫描目标目录，统计每文件违规数并与基线比对：
  - 任意文件 `actual > baseline`（含新增未登记文件）→ 报错阻断；
  - 任意文件 `actual < baseline` → 输出提示建议运行 `--update-baseline` 收紧基线（不报错，避免阻塞正常减债 PR）；
- **更新基线**：`python3 scripts/check_i18n_policy.py --update-baseline scripts/i18n_baseline.json` — 重新生成排序稳定的 JSON 基线。

### 5.2 检测规则与防误报设计

**扫描范围**：
- Go：`internal/**/*.go`、`agent/**/*.go`（排除 `*_test.go`、`**/vendor/**`、`**/proto/**`、`*.pb.go`）；
- Frontend：`frontend/**/*.{ts,tsx}`（排除 `*.test.*`、`**/__tests__/**`、`**/e2e/**`、`**/locales/**`、`**/prototype/**`、`.next/`、`dist-standalone/`、`node_modules/`）。

**规则 A：`FORBIDDEN_BILINGUAL_LITERAL`（禁止双语拼接字面量）**
1. 词法预处理：忽略注释行（`//`、`/* ... */`）及行尾注释，仅提取代码中的字符串字面量（Go `"..."` / `` `...` ``，TS `'...'` / `"..."` / `` `...` ``）；
2. 若字面量包含 ` / `，对其按 ` / ` 分割检查相邻段 `(left, right)`；
3. 命中条件（满足其一即判定为双语拼接）：
   - `left` 含 ≥2 个汉字且英文词数 ≤1（或汉字数显著高于英文词数），且 `right` 不含汉字且含 ≥2 个英文单词（`[A-Za-z]{2,}`）；
   - 或对称情况：`left` 不含汉字且含 ≥2 个英文单词，`right` 含 ≥2 个汉字；
4. 反例自动豁免（不触发误报）：
   - TS/JS 模板字面量 `${...}` 内的表达式（如 `${Math.floor(norm.lagSeconds / 60)} 分 ...`），扫描前先剥离 `${...}` 插值块；
   - 括号内纯技术术语对，如 `增量日志流 (CDC / Log Stream)`、`Subject.getSubject / JEP 486`（左侧或右侧并非完整英文短语，或处于技术标识符上下文）；
   - 同语种枚举斜杠，如 `严重 %d / 告警 %d / 信息 %d`、`关联集群 / 告警上下文`；
   - 带显式豁免注释 `// i18n-lint:ignore` 的行（需在注释后附简短原因，如解析器测试或兼容常量）。

**规则 B：`FORBIDDEN_RAW_LOCALE_PARSE`（禁止在业务 handler 手写语言解析）**
- 在 `internal/apps/**/*.go`（非测试）禁止出现：
  - `c.Query("lang")`、`c.Query("language")`、`c.Query("locale")`
  - `c.GetHeader("Accept-Language")`
- 统一引导改用 `locale.FromContext(c.Request.Context())`。

**规则 C：前端语言包单语断言**
- 在 `frontend/lib/i18n/__tests__/translation-coverage.test.ts` 新增测试用例：遍历 `zh.json` 与 `en.json` 所有叶子字符串，禁止出现双语拼接模式（复用与规则 A 一致的判定逻辑）。

### 5.3 CI 与本地门禁接入

- **`.github/workflows/ci-main.yml`**：
  - 在 `license-check`（轻量静态策略 job）中增加：
    1. PR / Push Diff 门禁：`python3 scripts/check_i18n_policy.py --base-ref ... --head-ref ...`
    2. 全量基线棘轮：`python3 scripts/check_i18n_policy.py --baseline scripts/i18n_baseline.json`
    3. 脚本单测：`python3 -m unittest scripts/check_i18n_policy_test.py`
  - 同步将 `license-check` 纳入 `ci-gate`（或确保其失败直接阻断合并）。
- **`Makefile`**：新增 `check_i18n` target，并加入 `pre_commit`。

## 6. 兼容性与风险

- **向后兼容**：`localeMiddleware` 优先读取 `?lang=` / `?language=` / `?locale=` 再回退 `Accept-Language`，所有既有 Web、CLI、SSE、第三方脚本调用无需修改即可保持行为一致。
- **误报风险**：通过 `scripts/check_i18n_policy_test.py` 纳入仓库真实反例（`JDK 24+ 移除 Subject.getSubject / JEP 486`、`增量日志流 (CDC / Log Stream)`、`严重 %d / 告警 %d`）持续回归验证。
- **性能开销**：`check_i18n_policy.py` 全量扫描约 500 个源文件耗时 <1 秒，不增加 CI 排队或构建负担。
