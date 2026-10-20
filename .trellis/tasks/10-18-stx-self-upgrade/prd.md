# PRD: STX 控制台自升级

## 背景 / Background

STX 目前具备 `stx-self-version`、安装包/Release 资产与 Agent/Proxy 同版本对齐能力，但**控制台内没有面向管理员的「升级 STX 自身」产品入口**。运维需要靠发版配合脚本完成控制面与 Agent/Proxy 替换。

本需求与既有 **SeaTunnel 集群升级（stupgrade）** 界面（见 `.trellis/tasks/09-29-upgrade-ui-polish`）**不是同一功能**：集群升级面向工作负载集群；本 PRD 面向 **STX 控制面 + Agent + Proxy 自身版本**。

产品决策（2026-10-18）：
1. 入口放在 **个人信息卡片**：「检查更新」（业界常见做法）。
2. 提供管理员专属 **系统升级** 页面（非仅小弹窗）。
3. 统一 **预置资产 → 再开升**：联网自动下载到本地仓，离线手动导入；正式升级只消费本地已预置包。
4. Proxy 资产按 `stx-java-proxy-*.jar` 全量匹配（不硬编码仅 v2/v3）。

## 目标 / Goals

- 管理员可在控制台检查新版本、预置资产、执行并验证 STX 自升级。
- UI 严格遵循 `.trellis/spec/frontend/ui-conventions.md`（Dock、`WorkspaceHeader`、渐进式披露、零塌陷加载、shadcn/i18n）。
- 与 GitHub Release / tag 发版流水线解耦呈现：产品内不把「打 tag / CI」做成运维流程图，但依赖其产出的包。

## 需求 / Requirements

### P0

#### R1. 个人信息卡片「检查更新」
- 位置：`ManagementBar` 既有 Profile 入口（已展示 `STX v…`）。
- 行为：管理员点击「检查更新」→ 进入系统升级页（或先轻量检查结果再跳转）。
- 非管理员：按钮禁用，Tooltip「仅管理员可升级」。

#### R2. 系统升级页（Admin only）
- 路由建议：`app/(main)/…` 下独立页；侧栏/Dock 可不新增入口，或以管理员可见「系统升级」补充。
- `WorkspaceHeader`：标题「系统升级」、短副标题、刷新。
- 区块：版本状态（当前/最新/状态胶囊）→ 预置资产 → 执行升级 → 验证结果。

#### R3. 预置资产（联网 | 离线）
- Tab：**联网自动下载** | **离线导入**。
- 下载/导入写入本地包缓存；列表需覆盖控制面、Agent、`stx-java-proxy-*.jar` 等本版本资产。
- 预置未完成时，「开始升级」禁用。

#### R4. 执行升级与验证
- 顺序：控制面 → Agent → Proxy（glob）→ 版本校验（`stx version` / API / `stx-self-version` 等以仓库事实为准）。
- 零塌陷加载：Skeleton / 顶置进度条；步骤状态可视化。

### P1

#### R5. 权限与审计
- 仅管理员可触发检查、预置、开升；写审计日志。

#### R6. i18n
- 全部用户可见文案进 `messages/`（zh-CN / en）。

### P2

#### R7. 失败回滚提示 / 重试单步（若后端能力具备再做）

## 约束 / Constraints

1. 不与 stupgrade 集群升级页面混用同一套路由与文案。
2. 升级过程禁止「边升边下」；只吃本地预置包。
3. 遵循前端 UI 约定；禁止左侧主导航替代 Dock。
4. Proxy 全量 jar，不写死 v2/v3。

## 验收标准 / Acceptance Criteria

- [ ] 管理员可从个人信息卡进入系统升级并完成「检查 → 预置 → 开升 → 验证」演示/联调路径
- [ ] 普通用户看不到可用的升级动作（禁用+说明）
- [ ] 联网与离线仅预置方式不同，开升路径一致
- [ ] 对照 `ui-conventions.md` 自检通过
- [ ] 与 `docs/stx-self-version.md`、安装/打包脚本中的资产约定一致

## 非目标 / Non-Goals

- 不在产品内做「打 GitHub tag / 改 CI」向导
- 不替代 stupgrade（SeaTunnel 集群升级）
- 本阶段可不改 install-online 脚本的 epoch 发现逻辑（可另开任务）

## 参考

- 交互原型（本地）：`/workspace/stx-self-upgrade-prototype/index.html`（需按 UI 规范重画）
- `docs/stx-self-version.md`
- `.trellis/spec/frontend/ui-conventions.md`

## 失败场景 / Failure Scenarios（P0，API + UI 必须处理）

| Code | 场景 | API 行为 | UI 行为 |
|------|------|----------|---------|
| `check_fail` | 检查更新失败（网络/GitHub API） | 返回 502/503 + `error_code=check_fail` | 状态胶囊「检查失败」，可重试 |
| `already_latest` | 已是最新 | 200，`update_available=false` | 胶囊「最新」，禁用预置/开升 |
| `download_partial` | 联网下载中断或部分资产失败 | stage 任务 `failed`，已下资产保留，`error_code=download_partial` | 列表标失败项，可重试/取消 |
| `bad_offline_import` | 离线包校验失败（缺资产/坏校验和） | 400，`error_code=bad_offline_import` | 导入区错误条，不标记 staged |
| `forbidden_non_admin` | 非管理员 | 403 | Profile「检查更新」禁用 + Tooltip |
| `step_fail_control_plane` | 控制面替换失败 | 任务 `failed`，`failure_step=control_plane` | 步骤标红，提示勿关页面/可查看原因 |
| `step_fail_agent` | Agent 替换失败 | 同上 `failure_step=agent` | 同上 |
| `step_fail_proxy` | Proxy jar 替换失败 | 同上 `failure_step=proxy` | 同上 |
| `verify_fail` | 升级后版本校验失败 | 任务 `failed`，`failure_step=verify` | 验证步骤失败说明 |
| `concurrent_upgrade` | 已有进行中的预置/升级 | 409，`error_code=concurrent_upgrade` | Toast/横幅「已有升级在进行」 |
| `disk_full` | 磁盘空间不足 | 507/400，`error_code=disk_full` | 预置失败提示清理磁盘 |
| `cancelled` | 用户取消（预置中可取消；开升中仅未进入不可逆步骤时可取消） | 任务 `cancelled` | 状态「已取消」 |

本地联调可用请求体字段 `simulate_failure`（仅非 production）注入上表 code，便于冒烟。

## API 草案（MVP）

前缀：`/api/v1/self-upgrade`（`LoginRequired` + `LoginAdminRequired`）

- `GET /status` — 当前版本、检查结果、预置状态、活跃任务
- `POST /check` — 检查 GitHub latest（或 mock）
- `POST /stage/download` — 联网预置到本地仓
- `POST /stage/import` — multipart 离线导入
- `GET /stage` — 预置进度与资产列表
- `POST /stage/cancel` — 取消预置
- `POST /execute` — 仅消费本地已预置包；顺序 CP → Agent → Proxy(glob) → Verify
- `GET /tasks/:id` — 任务与步骤
- `POST /tasks/:id/cancel` — 条件取消

## 资产约定

与 `install/package-release.sh` 对齐的本地仓清单（按目标版本）：

- `stx`（控制面二进制，按 GOOS/GOARCH）
- `stx-agent`
- `frontend-standalone-*.tar.gz`（可选，MVP 可标记）
- `stx-java-proxy-*.jar`（全量 glob，不硬编码 v2/v3）
- `stx-java-proxy.sh`
- `SHA256SUMS`

正式升级**禁止边升边下**；缺本地资产则拒绝 `execute`。
