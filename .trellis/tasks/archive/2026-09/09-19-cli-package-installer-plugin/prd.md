# Package Installer Plugin CLI 只读覆盖

## Goal

为 package、installer、plugin 的 14 个普通 GET 接口补充生成式 CLI，让用户和 AI Agent 可以通过稳定命令查询安装包、下载任务、主机安装状态、插件目录、插件依赖和集群插件状态，并使用本地真实 STX 服务完成验证。

## Requirements

### 接口范围

- `GET /api/v1/packages`
- `GET /api/v1/packages/:version`
- `GET /api/v1/packages/downloads`
- `GET /api/v1/packages/download/:version`
- `GET /api/v1/hosts/:id/install/status`
- `GET /api/v1/plugins`
- `GET /api/v1/plugins/local`
- `GET /api/v1/plugins/downloads`
- `GET /api/v1/plugins/:name`
- `GET /api/v1/plugins/:name/download/status`
- `GET /api/v1/plugins/:name/dependencies`
- `GET /api/v1/plugins/:name/official-dependencies`
- `GET /api/v1/clusters/:id/plugins`
- `GET /api/v1/clusters/:id/plugins/:name/progress`

### 命令要求

- 所有命令由操作登记表生成，不新增与登记表重复的手写 Cobra 命令。
- 操作编号和命令路径使用稳定的业务名称，路径参数使用位置参数，查询参数使用同名长选项。
- `profile_keys` 必须支持重复传入，例如 `--profile_keys jdbc --profile_keys cdc`，请求中保留多个同名查询参数。
- 每个操作支持既有的 JSON、YAML、table、raw 和 `--pick` 输出能力。
- 每个操作的离线帮助包含用途、调用样例和输出样例，查看帮助时不连接服务端。
- 客户端在业务请求前继续检查远端 capability，远端未登记、权限不足或修订号不兼容时不得调用业务接口。
- 路由覆盖基线必须把这 14 条路由从历史缺口改为已登记操作。

### 真实验证要求

- 使用仓库构建出的 `dist/stx`，并通过已有软连接执行命令。
- 重启本地 STX 服务，使 capability 返回本次新增的操作登记。
- 使用隔离的 CLI 配置目录登录本地服务，避免覆盖用户日常配置。
- 对 14 个命令逐条发起真实 HTTP 请求；有现成数据时验证成功结果，没有现成任务或记录时，空列表或稳定的未找到错误也属于有效验证。
- 至少验证 JSON、table、raw、`--pick`、重复 `profile_keys` 和一个稳定错误退出码。
- 汇报时明确区分：真实成功、有数据的成功、空数据成功、预期错误以及因本地数据条件无法触发的场景。

### 约束

- 本任务只处理 GET，不加入 POST、PUT、DELETE，也不开始安装、下载或插件变更。
- 不修改现有业务 Handler 的含义。
- 不修改或暂存当前工作区内 4 个安装发布文档的既有改动。
- 禁止使用 `git restore`；如需回撤，先说明并取得用户确认。
- 新增或修改的 Go 注释使用中英双语，中文在前。

## Acceptance Criteria

- [x] 操作登记表包含上述 14 个操作，且操作编号、命令路径、路由和参数经过单元测试。
- [x] 生成式命令支持单值和重复值查询参数，重复值按多个同名 URL 查询参数发送。
- [x] 根命令帮助和命令查找测试能发现全部新增命令。
- [x] 操作登记校验、路由基线测试、CLI 命令测试、`go test ./...` 和 `go vet ./...` 通过。
- [x] 合约报告显示登记操作从 26 增加到 40，历史缺口从 199 降为 185，路由总数和 Swagger 操作数没有意外变化。
- [x] 最新 `dist/stx` 完成构建，软连接仍指向该文件。
- [x] 本地 STX 服务返回新增 capability，14 个命令完成真实调用记录。
- [x] 本任务文件和代码通过定向 `git diff --check`，没有包含 4 个无关文档改动。

## Notes

- 本任务是 STX CLI AI Agent 入口总计划中“现有 API 的 CLI 覆盖”的第三批。
- 用户已确认 GET 可以一次处理较多；POST、PUT、DELETE 后续按业务操作分批验证。
- 下载状态接口虽然路径含 `download`，但响应是 JSON 状态，不属于文件下载命令。

## 验证结果

- 日期：2026-09-19。
- 最终二进制：`dist/stx`，darwin/arm64，SHA-256 `b2c36683d8972dc19104ddbf4b5b4e0b7f18d2c8562ab973050312e4ab022ddf`。
- 本地服务：PID `80307`，HTTP `17800`，gRPC `17890`；Agent 已重新注册到主机 `10`。
- capability 返回 `40` 个操作，本批 `14` 个操作全部存在且允许管理员调用。
- 真实数据：主机 `10`，集群 `6`，SeaTunnel `2.3.13`，插件目录 `85` 项，本地插件 `cdc-mysql` 和 `jdbc`。
- `package.list`、`package.get`、`plugin.list`、`plugin.get`、`plugin.local.list`、`plugin.download.status.get`、`plugin.dependency.list`、`plugin.official-dependency.list`、`cluster.plugin.list` 和 `cluster.plugin.progress.get` 均完成真实请求。
- `package.download.list` 和 `plugin.download.list` 真实返回空列表；`host.install.status.get` 与 `package.download.get` 因没有对应任务返回 `not_found` 和退出码 `5`。
- 重复参数真实调用返回 `selected_profile_keys=["mysql","postgresql"]`，证明两个同名 query 值都到达服务端。
- JSON、table、raw、`--pick`、帮助样例和稳定错误退出码均已验证。
