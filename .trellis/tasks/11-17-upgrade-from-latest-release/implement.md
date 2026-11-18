# 从最新正式版平滑升级 · 执行计划

分支 `chore/upgrade-from-latest-release`。按下面的顺序提交，每步一个 commit，提交信息用中文。
每步开始前按 AGENTS.md 查阅对应的 spec，新增或修改的注释写中英双语，中文在前。

**先写测试再修**：Step 1 的工作流在没有修复前应该失败。这能证明测试真的能抓到问题，在 PR 描述里贴出失败那次运行的链接。

## Step 1 · CI 跨版本升级测试（design §4）

- [ ] `scripts/ci-upgrade-from-latest.sh`：
  - 确定 FROM / TARGET；
  - 构建一键升级包；
  - 安装 FROM，植入数据；
  - 一键升级；
  - 断言；
  - 收集现场。
- [ ] `.github/workflows/upgrade-from-latest.yml`：配置 `pull_request` / `push` / `workflow_dispatch` / `workflow_call`
  四种触发方式，`if: always()` 上传现场。
- 验证：在 PR 上手动触发一次。预期在"三件套进程已退出 / `bin/start.sh` 一致 / 遗留告警 / `invalid_reason`"这几条断言上失败。
- 注意：脚本不要在共享机器上运行（t1 的 `/opt/stx` 是共享的）。

## Step 2 · 安装布局自修复与旧三件套收尾（design §1）

- [ ] `install/assets.go`：内嵌 `bin/{start,stop,status}.sh`。
- [ ] `internal/pkg/layoutheal`：实现 `Heal` 与 `ReapLegacyObservability`，并写单测：
  - 用临时目录模拟安装布局，验证替换、备份、幂等，以及开发目录被跳过；
  - 用假的 `/proc` 读取函数验证 pid 被复用时不发信号。
- [ ] 在 `internal/cmd/api.go` `runServer()` 中接入：先同步执行 `Heal`，再异步启动收尾 goroutine。
- [ ] `install/package-release.sh` 的升级包加入 `bin/`；`selfupgrade.applyStep` 增加 `bin/` 替换和对应单测。
- 验证：`go test ./internal/pkg/layoutheal/... ./internal/apps/selfupgrade/...`。

## Step 3 · 遗留告警一次性收尾（design §2）

- [ ] `internal/db/migrator/legacy_alerts_migration.go`，接入 `Migrate()`。
- [ ] 单测覆盖三类记录、幂等、无投递记录。
- 验证：`go test ./internal/db/migrator/...`。

## Step 4 · 失效配置显式化（design §3）

- [ ] 后端：
  - 判定函数；
  - DTO 增加 `invalid_reason`；
  - 启动扫描与 WARN；
  - CLI 的 `OutputExample` 补字段。
- [ ] 前端：
  - `AutoPolicyConfigPanel` 增加徽标与编辑引导；
  - i18n 中英文案。
- 验证：
  - `go test ./internal/apps/monitoring/... ./internal/apps/diagnostics/... ./internal/operation/...`；
  - `cd frontend && pnpm exec tsc --noEmit && pnpm test`；
  - 不要在 8G 机器上跑 `next dev` / `next build`。

## Step 5 · 发布说明与文档（design §5 §6 §7）

- [ ] `docs/release-notes/TEMPLATE.md` 和 `unreleased.md`（按 §7 填写草稿）。
- [ ] `release-on-tag.yml`：
  - 正式版必须有发布说明文件；
  - 配置 `body_path`；
  - 新增 `upgrade-gate` job，并加入 `publish-github-release` 的 `needs`。
- [ ] 更新 `docs/内置监控说明.md` 的升级章节，以及 `docs/打包发布说明.md` 的打 tag 检查清单。
- [ ] spec：在 `.trellis/spec/infra/`（或 backend 中对应的发布 / 升级文档）记录"一键升级由旧升级器执行，
  布局修复必须放在新二进制里"这条约束。
- 验证：工作流 YAML 用 `actionlint` 检查（如可用）；`scripts/check-release-asset-globs.sh` 仍然通过。

## Step 6 · CLI 补齐与 CI 改为 CLI 优先（design §4.3 §4.6 §8，PR 审查后追加）

- [ ] 新增 `stx self-upgrade import <file> --confirm`：
  - 参照 `internal/cmd/plugin.go`，用 `RequestMultipart` 流式上传；
  - 给 `self_upgrade.stage.import` 补命令路径、`Example`、`OutputExample`。
- [ ] 新增 `stx self-upgrade task get <id> [--wait] [--timeout]` 和 `task cancel <id> --confirm`：
  - `--wait` 期间控制面重启导致的连接失败要重试；
  - 终态失败或超时返回非零退出码。
- [ ] 单测：
  - multipart 字段名与流式上传；
  - `--wait` 的三种终态、重启期间连接失败后恢复、超时。
- [ ] 同步更新 operation 契约测试 / `route_baseline.json`、CLI 文档与 `SKILL.md`，以及 `unreleased.md` 的「新功能」。
- [ ] `scripts/ci-upgrade-from-latest.sh` 改为 CLI 优先：
  - `$STX` = 安装目录里的 `stx`，只用 `auth login --password-stdin --namespace ci` 登录一次；
  - 植入：渠道、路由、巡检策略、目录走 FROM 的 CLI，遗留告警仍用 curl 调 webhook；
  - 升级：`check` / `execute` 走 CLI；`import` / `task get` 先用 `--help` 探测，缺命令时退回接口并打 `::notice::`；
  - 处理 `execute` 返回 `confirmation_required` 时带 `--confirmation-id` 重试；
  - 断言：接口类断言全部改用 TARGET 的 CLI；新增"升级后不重新登录也能调用 CLI"，二次重启后再检查一次；
  - 删除 cookie 会话相关代码；退回 curl 时使用 CLI 登录得到的 token。
- 验证：
  - `go test ./internal/cmd/... ./internal/cli/... ./internal/operation/...`；
  - PR 上的 upgrade 工作流为绿，日志里能看到 CLI 调用，以及 v1.4.0 下导入和任务查询两处的退回提示。

## 收尾

- [ ] 在 PR 上重新跑 Step 1 的工作流，要求全绿；在 PR 描述里贴出前后两次运行的链接。
- [ ] 手动触发一次 `workflow_dispatch`，设置 `from_version=v1.4.0`，要求全绿。

## 审查门槛

- Step 1 合入前需确认：没有修复时，工作流确实在预期的断言上失败。
- Step 2 合入前需确认：开发目录（仓库根目录下 `go run main.go server`）启动时不会触发自修复。
- Step 3 合入前需确认：没有产生恢复通知。
- 整体合入前需确认：最新正式版不含三件套时（可以临时把 FROM 指向一个不带三件套的预发版来模拟），
  跳过逻辑正确，测试仍然通过。
