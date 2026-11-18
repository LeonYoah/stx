# 从最新正式版平滑升级 · 技术设计

需求与已确认事实见 `prd.md`。本文给出各需求的落点、契约与取舍。所有新增或修改的注释都写中英双语，中文在前。

---

## §1 一键升级后安装布局自修复（R1）

### 1.1 为什么放在新二进制里

执行一键升级的是**旧**升级器。v1.4.0 只复制 `stx`、`lib/agent`、java-proxy、`scripts/stx-java-proxy.sh`、`frontend/`，
包里加 `bin/` 对旧升级器无效。所以修复必须由**升级后首次启动的新二进制**完成，同时也给以后的新升级器补上 `bin/` 替换。

### 1.2 内嵌脚本

- 新增 `install/assets.go`（package `install`）：

  ```go
  //go:embed bin/start.sh bin/stop.sh bin/status.sh
  var BinScripts embed.FS
  ```

  `go:embed` 不能引用上级目录，所以放在 `install/` 包内。仓库根模块直接 import `github.com/LeonYoah/stx/install`。
- 脚本内容是通用的：`BASE_DIR` 按脚本位置推算，没有安装期占位符，可以原样写回。

### 1.3 `layoutheal` 包（建议 `internal/pkg/layoutheal`）

```go
// Heal 在安装目录布局下用内嵌脚本覆盖 bin/ 并移走旧三件套脚本；不满足条件时直接返回。
// Heal refreshes bin/ from the embedded scripts and moves the legacy stack script away under an install layout.
func Heal(ctx context.Context, opts Options) (Result, error)
```

**启用条件（全部满足才执行）：**
- 只在 Linux 上执行；
- 不在 Docker 中：复用 `selfupgrade.IsRunningInDocker()`；
- 安装目录取 `filepath.Dir(os.Executable())` 解析软链接后的路径，目录下同时存在：
  - `bin/start.sh`，且内容含 `STX` 标识，避免误伤开发环境；
  - `config.yaml` 或 `stx.env`；
- 环境变量 `STX_LAYOUT_HEAL` 不为 `off`。

**动作：**
1. 对 `start.sh` / `stop.sh` / `status.sh` 逐个比较 sha256，内容不同时才替换：
   1. 原文件改名为 `bin/<name>.bak-<UTC 时间戳>`；
   2. 新内容写到临时文件 `bin/.<name>.tmp`；
   3. `chmod 0755`；
   4. `rename` 原子替换。
2. 若存在 `bin/lib/observability.sh`，改名为 `bin/lib/observability.sh.bak-<UTC 时间戳>`。
   - 新 `start.sh` 不会 source 它，但旧 `start.sh` 会，所以要移走。
   - 备份文件名不以 `.sh` 结尾，防止被误 source。
3. 汇总记录一条 INFO 日志（中英双语）：列出替换了哪些文件、备份路径。没有变化时不打日志。

**失败处理：** 任何一步失败只记 WARN，不阻止服务启动，也不回滚已替换的文件。各脚本彼此独立。

**调用点：** `internal/cmd/api.go` `runServer()` 中 `migrator.Migrate()` 之后、`router.Serve()` 之前，同步执行。
只做本地文件操作，耗时在毫秒级。

### 1.4 旧三件套进程收尾

**为什么要异步执行：** 旧 `start.sh` 先启动 backend，**之后**才启动三件套。新二进制启动时三件套可能还没起来，
所以需要一段时间窗口持续检查。

```go
// ReapLegacyObservability 在启动后的时间窗内按 pid 文件停掉旧版外部监控进程。
// ReapLegacyObservability stops legacy external monitoring processes by pid file within a startup window.
func ReapLegacyObservability(ctx context.Context, installDir string, window, interval time.Duration)
```

- 默认窗口 3 分钟，间隔 5 秒（v1.4.0 单元的 `TimeoutStartSec=120`，窗口要更长），在 goroutine 中运行，进程退出时随 ctx 结束。
- pid 文件路径（相对安装目录）与允许的进程名：

  | pid 文件 | 允许的进程名 |
  |---|---|
  | `observability/prometheus/prometheus.pid` | `prometheus` |
  | `observability/alertmanager/alertmanager.pid` | `alertmanager` |
  | `observability/grafana/grafana.pid` | `grafana` / `grafana-server` |

- 读出 pid 后，用 `/proc/<pid>/exe` 的 basename（读不到时退回 `/proc/<pid>/cmdline` 的第一个字段）核对进程名，
  不匹配就视为 pid 被复用：只删除 pid 文件，不发信号。
- 匹配时先发 `SIGTERM`，最多等 10 秒，仍存活再发 `SIGKILL`，然后删除 pid 文件；每个进程记一条 INFO 日志。
- `observability/` 目录与数据**不删**。
- 与 §1.3 使用同样的启用条件；pid 文件都不存在时空转到窗口结束。

### 1.5 新升级包与新升级器

- `install/package-release.sh` 的 upgrade stage（约 689 行）增加：
  - `mkdir -p "$upgrade_stage/bin"`；
  - 复制 `install/bin/{start,stop,status}.sh`。
- `internal/apps/selfupgrade/service.go` `applyStep` 增加 `bin/` 替换：
  - 包里有 `bin/` 时，逐个脚本按 §1.3 的方式"备份 → 临时文件 → rename"；
  - 包里没有 `bin/` 时跳过；
  - 替换失败要暴露成步骤错误，与 `lib/agent` 的处理一致。
- 两处同时存在是为了：未来由新升级器升级时，在重启**之前**就换好脚本，不用依赖启动后的自修复。

---

## §2 遗留告警一次性收尾（R2）

- 位置：`internal/db/migrator/` 新增 `legacy_alerts_migration.go`。在 `Migrate()` 中紧跟 `MigrateTimeColumnsToUTC` 调用。
- 一次性标记：复用 `stx_schema_flags`（`SchemaFlag`），标记名 `legacy_prometheus_alerts_resolved_v1`。
  并发写入的处理方式与 `time_columns_utc_v1` 相同：先占位写标记，主键冲突视为其他实例已完成。
- 判定条件：

  ```sql
  UPDATE monitoring_remote_alerts
     SET status = 'resolved', resolved_at = :now, ends_at = :now_unix, updated_at = :now
   WHERE LOWER(status) <> 'resolved'
     AND generator_url IS NOT NULL AND generator_url <> ''
  ```

  - 内置评估器写入的 `generator_url` 恒为空（`metric_evaluator.go:455`）；Prometheus 产生的告警由 v1.4.0 webhook
    写入 `generatorURL`（`remote_integration.go:247`@v1.4.0）。
  - 已经跑过 main 预发构建的环境，评估器写入的告警不受影响。
- **直接执行 SQL，不走 `ingestRemoteAlertRecord`**，所以不会触发投递，也不会产生恢复通知。
- 记一条 INFO 日志，包含处理条数。
- 表不存在时（全新安装）只写标记。
- 单测：
  - SQLite 下插入三条记录：带 `generator_url` 的 firing、不带 `generator_url` 的 firing、已 resolved；
    只有第一条被改；
  - 重复调用为空操作；
  - 没有新增 `monitoring_notification_deliveries` 记录。

---

## §3 失效配置显式化（R3）

### 3.1 判定函数（后端唯一来源）

| 对象 | 失效条件 | `invalid_reason`（英文码，前端映射文案） |
|---|---|---|
| 通知路由 `NotificationRoute` | `source_type` 非空，且不属于 `platform_health` / `metric` / `job` | `legacy_source_type` |
| 巡检自动策略 `InspectionAutoPolicy` | 任一条件的 `template_code` 不在内置目录中 | `legacy_condition_code` |
| 同上 | `ALERT_FIRING` 条件启用了，但 `rule_keys` 为空 | `alert_firing_without_rule_keys` |

- 函数分别放在 `monitoring`（路由）和 `diagnostics`（策略）包内，复用已有的校验逻辑：
  - 路由：model `Validate` 里对 `source_type` 的判断；
  - 策略：`auto_policy_service.go:223` 一带的条件校验。
- 抽成纯函数，供"返回 DTO"和"启动扫描"两处共用。
- 一个对象有多个失效原因时，用 `,` 连接，顺序固定。

### 3.2 API / CLI

- 路由与策略的列表、详情 DTO 增加 `invalid_reason string \`json:"invalid_reason,omitempty"\``。
- 不新增接口，不改请求体。
- 更新时仍然由原有校验返回 400，保持 #146 的契约。
- CLI 的 `monitoring notification-route list` 和 `diagnostics auto-policy list` 走 JSON 输出，会自动带出该字段。
  如果有表格输出，增加一列"失效原因"。
- 按 AGENTS.md 的 CLI 约定，检查对应操作的 `OutputExample` 并补上该字段。

### 3.3 启动扫描

- `router.Serve()` 完成服务初始化后，异步执行一次扫描。
- 每类各记一条 WARN（中英双语），列出 ID，最多 20 个，超出部分写"等 N 条"。
- 示例：`[Upgrade] 3 条通知路由的来源类别已失效，通知不会再发送，请重新选择：#4,#7,#9 / ...`
- 只扫描一次，不周期执行。

### 3.4 前端

- 通知路由目前没有前端页面（只有 `monitoring.service.ts` 的 service 方法），这次**不新建页面**，靠 API / CLI 和日志提示。
- `frontend/components/common/diagnostics/AutoPolicyConfigPanel.tsx`：
  - 策略带 `invalid_reason` 时，在列表行或卡片上显示警告徽标，悬浮显示原因文案；
  - 打开编辑时：
    - 删掉失效的条件码；
    - `ALERT_FIRING` 的规则键多选框标为必填；
    - 不自动保存。
- 先查 `.trellis/spec/frontend/index.md` 与 `ui-conventions.md`。
- i18n 在 `zh.json` / `en.json` 增加三个原因码对应的文案。

---

## §4 CI：最新正式版 → 当前代码 的一键升级测试（R4）

### 4.1 文件

- `scripts/ci-upgrade-from-latest.sh`：主体脚本，`set -euo pipefail`。只能在一次性虚拟机或 CI 上运行，
  **不要**在共享机器（如 t1 的 `/opt/stx`）上跑。
- `.github/workflows/upgrade-from-latest.yml`：编排。

### 4.2 触发方式

```yaml
on:
  pull_request:
    paths:
      - 'internal/apps/selfupgrade/**'
      - 'install/**'
      - 'internal/db/**'
      - 'internal/config/**'
      - 'internal/pkg/layoutheal/**'
      - 'internal/apps/monitoring/**'
      - 'internal/apps/diagnostics/auto_policy*'
      - 'scripts/ci-upgrade-from-latest.sh'
      - '.github/workflows/upgrade-from-latest.yml'
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      from_version: { description: '起始版本，留空 = releases/latest', required: false }
  workflow_call:
    inputs:
      target_version: { type: string, required: true }
```

- `release-on-tag.yml` 新增 job `upgrade-gate`：
  - 通过 `uses: ./.github/workflows/upgrade-from-latest.yml` 调用，`with.target_version` 取 tag 去掉 `v` 前缀；
  - `publish-github-release` 的 `needs` 增加 `upgrade-gate`。
- 打 tag 时 `releases/latest` 正好是上一个正式版，测的就是用户将要走的那条升级路径。

### 4.3 步骤

**1. 确定起始版本 FROM 与目标版本 TARGET**
- FROM：`inputs.from_version`，或 `gh release view --repo "$GITHUB_REPOSITORY" --json tagName -q .tagName`。
  `releases/latest` 本身不含预发版。
- TARGET：
  - 有 `target_version` 时直接用；
  - 否则取 FROM 的 `major.minor.(patch+1)-ci.<run_number>`，例如 `1.4.1-ci.812`。
  - 这个格式符合 FROM 的 `releaseVersionPattern`（`^[vV]?\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`），并且大于 FROM。

**2. 用当前代码构建一键升级包**
- 环境：Go、Node、pnpm、Java 11 的安装步骤照抄 `release-on-tag.yml` 的 `package-release` job。
- 构建：`bash install/package-release.sh --arch amd64 --version "$TARGET" --build-frontend --output-dir dist`
  （以脚本实际参数为准）。
- 产物：`dist/stx-upgrade-$TARGET-linux-amd64.tar.gz`。需要确认二进制的 ldflags 版本等于 TARGET。

**3. 安装 FROM**
- 从 `https://github.com/$GITHUB_REPOSITORY/releases/download/$FROM/install-online.sh` 下载安装脚本，执行
  `sudo bash -s -- --version "$FROM" --yes --admin-password "$PW"`。
- 用 FROM 的 `install.sh --help` 探测是否支持 `--with-observability`，支持就加上，以复现带三件套的旧环境。
- 安装方式为 systemd（默认）。轮询 `GET /api/v1/version`，直到返回 FROM。

**4. 在 FROM 上植入数据**
- 先用 FROM 的 CLI 登录（见 §4.6）：`printf '%s' "$PW" | $STX auth login --server http://127.0.0.1:17800 --username admin --password-stdin --namespace ci`。
  **只登录这一次**，升级后不重新登录，用来验证 CLI 凭据跨升级仍然有效。
- 每项植入前先探测能力，不支持就打 `::notice::` 后跳过，并记录到 `seeded_*` 变量。
- 植入优先走 FROM 的 CLI（`--request-file` 传请求体，写操作加 `--confirm`）。FROM 的 CLI 没有对应命令时（例如
  Alertmanager webhook 本来就不是 CLI 操作），才退回 curl。
- 已核对 v1.4.0 的 CLI 命令：
  - 通知渠道：`monitoring notification-channel create`；
  - 通知路由：`monitoring notification-route create`；
  - 巡检策略：`diagnostics auto-policy create`；
  - 工作台目录：`sync task create`（`node_type=folder`）；
  - 遗留告警只能 curl webhook。

  | 植入项 | 方式 | 断言用途 |
  |---|---|---|
  | 通用数据 | 新建一个通知渠道（webhook 指向本机一个只记录请求的端口）和一个工作台目录 | 数据保留 |
  | 遗留告警 | FROM 存在 `/monitoring/alertmanager/webhook` 时，POST 一条 AM v4 格式的 firing 告警，`generatorURL` 非空 | §2 |
  | 旧路由 | FROM 接受时，创建 `source_type=remote_alertmanager` 的通知路由，绑定上面的渠道 | §3 |
  | 旧巡检策略 | FROM 接受时，创建含 `PROM_CPU_HIGH` 条件的自动策略 | §3 |
  | 三件套 | 记录 `observability/*/*.pid` 是否存在、对应进程是否在跑 → `legacy_stack=true/false` | §1.4 |

**5. 一键升级到 TARGET（用的是 FROM 自带的升级器和 CLI）**

`$STX` 指安装目录里的 `stx` 二进制。升级前它是 FROM 的 CLI；升级后同一路径被替换成 TARGET，正好用来做断言。

| 步骤 | 首选（FROM 的 CLI） | 退回（FROM 的 CLI 没有该命令时） |
|---|---|---|
| 1. 指定目标版本 | `$STX self-upgrade check --request-file check.json`（`{"force_latest":"$TARGET"}`） | — |
| 2. 导入本地包 | `$STX self-upgrade import <tar.gz> --confirm`（§8 新增） | `POST /self-upgrade/stage/import`，带 CLI 的 token |
| 3. 执行 | `$STX self-upgrade execute --request-file exec.json --confirm`（`{"mode":"apply"}`） | — |
| 4. 等待完成 | `$STX self-upgrade task get <id> --wait`（§8 新增） | 轮询 `$STX self-upgrade history` 或 `GET /self-upgrade/tasks/:id` |
| 5. 等新版本就绪 | `$STX self-upgrade status` 返回 TARGET，整体超时 6 分钟 | `GET /api/v1/version` |

- 用 `$STX self-upgrade --help` 判断 FROM 有没有 `import` 和 `task get`，**按命令逐个探测**，不要按版本号判断。
- v1.4.0 两个命令都没有，所以第 2 步和第 4 步会走退回路径。等本版本发布成为 latest 后，整条链路自然变成纯 CLI，
  不需要回头改脚本。
- `execute` 是 R3 操作。服务端如果返回 `confirmation_required`，要读出 `confirmation_id`，再带
  `--confirmation-id` 重试一次。脚本里要处理这个分支，不能直接判失败。
- 退回 curl 时也用 CLI 登录得到的 token（`Authorization: Bearer`，加 `X-STX-Client: cli`），不再单独维护 cookie 会话。

**6. 断言**

- 升级后的接口类断言**全部用 TARGET 的 CLI**（同一个 `$STX`，沿用升级前的登录配置），例如：
  - `$STX monitoring notification-route list`、`$STX diagnostics auto-policy list`：取 `invalid_reason`；
  - `$STX monitoring notification-channel list`：检查数据保留；
  - 告警实例列表命令：遗留告警的断言。
- 这样同时覆盖了 R3 里"CLI 输出带出失效原因"这一条验收。
- 只有进程、端口、文件这类系统层断言才用 shell。
- 新增断言：升级后第一次调用 CLI **不重新登录**也能成功（CLI 凭据跨升级有效）。二次重启之后同样检查一次。

| 类别 | 断言 | 条件 |
|---|---|---|
| 通用 | 版本 = TARGET；能登录；植入的渠道和目录还在；`systemctl is-active stx`；前端首页返回 200 | 总是 |
| 通用 | 安装目录下的 `bin/start.sh` 与仓库 `install/bin/start.sh` 字节一致（`cmp`）；`bin/lib/observability.sh` 不存在；有 `*.bak-*` 备份 | 总是 |
| 通用 | `logs/backend.log` 中没有 `panic` / `fatal error` | 总是 |
| 三件套 | 升级完成后 3 分 30 秒内，安装目录下的 `prometheus`、`alertmanager`、`grafana` 进程全部退出；9090 / 9093 / 3000 不再监听；`observability/` 目录仍在 | `legacy_stack=true` |
| 遗留告警 | `GET /monitoring/alert-instances?status=firing` 中没有植入的那条；渠道接收端**没有**收到 resolved 请求 | 植入成功时 |
| 失效配置 | 路由和策略的 `invalid_reason` 非空；`backend.log` 中有对应的 WARN | 植入成功时 |
| 二次重启 | `sudo systemctl restart stx` 后，重复"通用"断言和"三件套进程不存在"断言 | 总是 / `legacy_stack=true` |

**7. 收集现场**
- 无论成败都执行（`if: always()`）。
- 打包上传：`logs/`、`journalctl -u stx --no-pager`、自升级 history、`ps aux`、`ss -ltnp`。

### 4.4 能力探测的意义

下一个正式版发布后，`releases/latest` 本身就不含三件套。这时三件套、遗留告警、旧路由的断言会因为植入被跳过而自动不执行，
通用断言继续保证"最新正式版 → 当前代码"能升级成功。不需要回头修改工作流。
如果要对 v1.4.0 做回归，可以手动触发 `workflow_dispatch`，设置 `from_version=v1.4.0`。

### 4.5 资源与时长

- `runs-on: ubuntu-latest`（自带 systemd，可以 sudo）。不需要 SeaTunnel 集群。
- 预计耗时：构建约 8 分钟，安装、升级、断言约 6 分钟。
- 构建产物不缓存，保证测的就是当前代码。

### 4.6 为什么 CLI 优先

- CLI 是用户真实使用的升级入口之一。直接调接口绕开了 CLI 的请求体、`--confirm`、R3 二次确认和 token 存储，
  这些环节坏了，CI 也发现不了。
- 升级前后用的是同一路径下的 `stx`：前半段测"旧 CLI 驱动旧升级器"，后半段测"新 CLI 读新服务端"，
  中间还验证了 CLI 凭据跨升级有效。
- 能力探测按命令逐个进行。缺命令时只在那一步退回 curl，并打 `::notice::` 写明原因，方便以后删除退回分支。

---

## §5 发布说明（R5）

- 新增目录 `docs/release-notes/`：
  - `TEMPLATE.md`：固定章节为「升级须知」「破坏性变更」「新功能」「修复」「已知问题」；
  - `unreleased.md`：这次先按 §7 写好下一版的草稿。打 tag 前把它改名为 `v<x.y.z>.md`，再新建一个空的 `unreleased.md`。
- `release-on-tag.yml` 的 `publish-github-release`：
  - 新增一步：当 tag 不含 `-`（即正式版）时，要求 `src/docs/release-notes/${GITHUB_REF_NAME}.md` 存在，否则失败；
  - `softprops/action-gh-release` 增加 `body_path`（预发版允许缺失时置空），保留 `generate_release_notes: true`，
    让自动生成的 PR 列表附在正文之后。
- `docs/打包发布说明.md` 补一节"打 tag 前检查清单"：
  1. 发布说明已改名；
  2. `upgrade-from-latest` 在 main 上为绿；
  3. 版本号是否需要升主版本。

---

## §6 文档（R6）

`docs/内置监控说明.md`「从旧版本升级」一节：
1. 区分"`install.sh` 重装"和"应用内一键升级"两条路径，说明后者由新版本启动时自动刷新 `bin/` 脚本并停掉旧三件套。
2. 补全删除的接口：HTTP SD 两个、webhook、Grafana 代理两个。
3. 内存与磁盘：默认开启、31 天 / 30GiB、小内存机器的建议（降低保留时长，或 `--without-observability`）。
4. 时区：Agent 升级前回退到控制面时区；容器建议设置 `TZ` 或 `STX_SEATUNNEL_TIMEZONE`。
5. Agent：主机指标和节点时区需要升级 Agent，老 Agent 兼容。
6. Docker Compose：`docker compose up -d --remove-orphans`。
7. 失效配置：如何在 API / CLI / 界面识别 `invalid_reason`，以及如何修复。

---

## §7 下一版发布说明草稿要点（写入 `docs/release-notes/unreleased.md`）

**升级须知：**
- 监控内置，不再需要也不再启动 Prometheus / Alertmanager / Grafana；历史监控数据不迁移。
- 一键升级后会自动刷新 `bin/` 脚本并停掉旧组件，旧数据目录保留。
- 内存与磁盘占用会变化，需要提前评估。
- 建议同步升级 Agent。

**破坏性变更：**
- 告警类别取值改为 `platform_health` / `metric` / `job`，旧值返回 400。
- 通知路由和巡检策略中的旧值会失效，需要重新选择（见 `invalid_reason`）。
- 删除 5 个接口：HTTP SD 两个、webhook、Grafana 代理两个。
- 删除安装参数 `--with-observability`、`--prometheus-port`、`--alertmanager-port`、`--grafana-port`。
- Webhook 通知负载中的 `source_type` 取值改变。
- 引擎时间的回退时区改为控制面所在机器的时区。

**新功能：**
- 首页集群 / 作业 / 主机大盘。
- 内置 tsdb 自观测与容量护栏。
- 管理员可用的进程剖析（pprof，默认关闭）。
- 告警规则目录。

---

## §8 CLI 补齐：自升级导入本地包与任务查询（R7）

现状：`self_upgrade.stage.import` 和 `self_upgrade.task.get` 在 `internal/operation/self_upgrade.go` 里的
命令路径是 `nil`，CLI 没有对应命令（v1.4.0 和当前 main 都一样）。内网、离线用户无法用 CLI 完成一键升级，
这也违反了 AGENTS.md「新增接口要补 CLI 能力」的约定。

### 8.1 `stx self-upgrade import <file> --confirm`

- 手写命令，参照 `internal/cmd/plugin.go` / `internal/cmd/package.go`，用 `client.RequestMultipart` 流式上传，
  字段名 `file`。上传的文件不要整个读进内存，升级包最大 500MB。
- 在 `self_upgrade.go` 里给 `self_upgrade.stage.import` 补上命令路径 `[]string{"self-upgrade", "import"}`、
  `Example`、`OutputExample`。operation 契约测试与 `route_baseline.json` 同步更新。
- 风险等级沿用 R1，要求 `--confirm`。
- 输出：导入后的暂存快照，包括目标版本和资产列表，与接口返回一致。

### 8.2 `stx self-upgrade task get <id> [--wait] [--timeout 6m]`

- `self_upgrade.task.get` 补命令路径 `[]string{"self-upgrade", "task", "get"}`。
- `--wait`：轮询直到任务进入终态（succeeded / failed / cancelled），终态非 succeeded 时返回非零退出码。
  - 控制面重启期间连接失败属于预期，要重试，不能当作失败；
  - 超时也返回非零退出码，并在输出里带上最后一次看到的状态。
- 不加 `--wait` 时行为与普通读命令一致。
- 顺带给 `self_upgrade.task.cancel` 补上 `self-upgrade task cancel <id> --confirm`。

### 8.3 文档

- CLI 文档（`docs/` 下 CLI 相关章节，以及 `internal/cli/skill/assets/*/SKILL.md` 中如有自升级说明）补充离线升级流程：
  `check` → `import` → `execute` → `task get --wait`。
- 写进发布说明的「新功能」：CLI 支持导入本地升级包。

## 已知风险

| 风险 | 缓解 |
|---|---|
| 自修复误判开发目录为安装目录 | 启用条件要求 `bin/start.sh` 含 STX 标识，且同时存在 `config.yaml` 或 `stx.env`；可用 `STX_LAYOUT_HEAL=off` 关闭 |
| pid 被复用导致误杀 | 发信号前核对 `/proc/<pid>/exe` 的进程名 |
| 用户手工改过 `bin/start.sh` | 替换前备份，备份路径写进日志和发布说明 |
| CI 依赖 GitHub Release 网络 | 下载失败重试 3 次；仍失败时整个 job 失败，不静默跳过 |
| FROM 的自升级接口将来变化 | 脚本先做接口存在性检查，报错信息指向本设计文档 |
