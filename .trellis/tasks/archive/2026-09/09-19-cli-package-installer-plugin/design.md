# Package Installer Plugin CLI 只读覆盖技术设计

## 1. 边界

本任务沿用现有生成式 CLI：`internal/operation` 保存操作登记，`internal/cli/command` 根据登记创建 Cobra 命令，`internal/apps/capability` 把服务端登记和当前用户权限返回给客户端。业务 Handler 和服务层保持不变。

14 个 GET 都使用 `normal` 模式和 `R0` 风险等级。它们只读取 JSON，不使用文件流、SSE、请求体、确认头或幂等键。

## 2. 命令与操作编号

| operation_id | 命令 | 路由 |
| --- | --- | --- |
| `package.list` | `stx package list` | `GET /api/v1/packages` |
| `package.get` | `stx package get <version>` | `GET /api/v1/packages/:version` |
| `package.download.list` | `stx package download list` | `GET /api/v1/packages/downloads` |
| `package.download.get` | `stx package download get <version>` | `GET /api/v1/packages/download/:version` |
| `host.install.status.get` | `stx host install status get <id>` | `GET /api/v1/hosts/:id/install/status` |
| `plugin.list` | `stx plugin list [--version] [--mirror]` | `GET /api/v1/plugins` |
| `plugin.local.list` | `stx plugin local list` | `GET /api/v1/plugins/local` |
| `plugin.download.list` | `stx plugin download list` | `GET /api/v1/plugins/downloads` |
| `plugin.get` | `stx plugin get <name> [--version]` | `GET /api/v1/plugins/:name` |
| `plugin.download.status.get` | `stx plugin download status get <name> --version <version> [--profile_keys value...]` | `GET /api/v1/plugins/:name/download/status` |
| `plugin.dependency.list` | `stx plugin dependency list <name> [--version]` | `GET /api/v1/plugins/:name/dependencies` |
| `plugin.official-dependency.list` | `stx plugin official-dependency list <name> [--version] [--profile_key]` | `GET /api/v1/plugins/:name/official-dependencies` |
| `cluster.plugin.list` | `stx cluster plugin list <id>` | `GET /api/v1/clusters/:id/plugins` |
| `cluster.plugin.progress.get` | `stx cluster plugin progress get <id> <name>` | `GET /api/v1/clusters/:id/plugins/:name/progress` |

命令使用现有 API 参数名，便于从服务端文档直接判断传值方式。`id` 在命令帮助中结合父命令说明其含义。

插件路径参数使用业务名，例如 `jdbc`；`connector-jdbc` 是对应 jar 的 artifact 名，不能代替业务名写入命令样例。

## 3. 重复查询参数

当前 `InputSpec` 只描述名称、位置、必填和说明，命令构建器统一创建 string flag，并用 `url.Values.Set` 写入一个值。`profile_keys` 在 Handler 中使用 `QueryArray`，需要新增 `Repeated bool`：

- 普通查询参数继续使用 Cobra string flag。
- `Repeated=true` 的查询参数使用 Cobra string-slice flag。
- URL 构建时对每个非空值调用 `url.Values.Add`，保留多个同名参数。
- 必填重复参数至少需要一个非空值；本批的 `profile_keys` 是可选项。
- 操作登记校验拒绝 path、header、body 和 file 上的 `Repeated=true`，避免含义不明确。

该字段会进入操作登记摘要。capability 当前不返回输入定义，只返回操作编号、修订号、权限、模式、风险和影响信息。`RegistryRevision` 增加一版，单个操作的 `Revision` 仍为 1，因为这些操作首次公开。

## 4. 输出与错误

沿用通用远端客户端：Handler 响应中的 `data` 进入 CLI 统一结果信封，`error_msg` 由客户端现有错误处理转换为 stderr JSON 事件和稳定退出码。

命令不为列表结果编写专用 table 模板。现有通用渲染器负责 JSON、YAML、table、raw 和 `--pick`，从而保持各模块行为相同。

## 5. 测试

- `internal/operation/validate_test.go` 检查操作总数、操作集合、关键参数和重复参数约束。
- `internal/cli/command/command_test.go` 检查重复 flag 生成、URL 编码、必填判断及现有单值参数不回归。
- `internal/cmd/root_test.go` 检查 14 条命令路径可查找。
- 合约报告重新生成基线，确认 14 条路由变为 `operation`。
- 构建并重启本地服务后，通过真实登录调用全部命令。

## 6. 兼容与回退

新增操作不会改变已有命令。`Repeated` 默认为 false，旧登记项行为不变。

若重复参数实现出现问题，可以在取得用户确认后回退 `Repeated` 字段和对应登记；其余 13 个不依赖重复参数的命令可独立保留。禁止使用 `git restore`。
