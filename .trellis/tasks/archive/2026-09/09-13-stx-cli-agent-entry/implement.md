# STX CLI AI Agent 入口实现计划

## 状态

- 日期：2026-09-18
- 当前阶段：实现中，P0-A、P0-B 已完成，P0-C 基础部分已完成
- 开始实现前要求：用户审阅并确认 `prd.md`、`design.md` 和本文件
- 本文件只列实施顺序，不表示已经授权开始修改业务代码

## 1. 实施原则

- 采用“工作包 + 验收关口”的实施方式，不把所有工作排成一条长串。
- 前置工作没有通过验收，依赖它的工作不得开始。
- CLI 公共基础和最小真实调用优先，扩展功能不得阻塞第一个可用版本。
- 只读命令先接入，写命令必须等待服务端安全规则完成。
- 服务端规则优先，不能只在 CLI 做权限、确认、脱敏或幂等保护。
- 每个工作包都需要独立测试，关键组合还要通过统一验收关口。
- 不使用 `git restore`。如需回撤，先说明影响并由用户确认。
- 修改 `.proto` 后必须重新生成并检查：
  - `internal/proto/agent/agent.pb.go`
  - `internal/proto/agent/agent_grpc.pb.go`
  - 仓库中 Agent 使用的对应生成文件
- 新增或修改的前后端代码注释使用中英双语，中文在前。

## 2. 优先级

| 优先级 | 含义 | 处理原则 |
| --- | --- | --- |
| P0 | CLI 能否成立的前置工作 | 未完成时，不批量接入业务命令 |
| P1 | 全接口 CLI 所需的安全、执行和覆盖能力 | 未完成时，不开放对应写操作或异步操作 |
| P2 | 诊断、源码、工作台和 Skill 等扩展能力 | 不阻塞基础 CLI，可在满足依赖后同时开发 |

## 3. 总体依赖

```text
G0 方案与公共契约确认
 ├─ P0-A CLI 入口与本地配置
 ├─ P0-B 输出协议、stderr 事件与退出码
 ├─ P0-C 操作登记模型与路由基线
 └─ P0-D CLI 令牌服务端能力
          ↓
G1 CLI 基础验收
 ├─ P0-E HTTP 客户端与登录命令
 ├─ P0-F capabilities 与命令生成框架
 └─ P0-G 首个真实只读命令
          ↓
G2 最小端到端验收
 ├─ P1-A 各模块只读命令接入
 ├─ P1-B 风险确认、幂等和资源版本
 ├─ P1-C 公共异步执行与真实取消
 ├─ P1-D 审计、用户归属和敏感信息处理
 └─ P1-E 路由、OpenAPI 和 CLI 覆盖检查
          ↓
G3 安全写操作验收
 ├─ P1-F 各模块写命令和异步命令接入
 ├─ P2-A 诊断资源和 Agent 产物
 ├─ P2-B 调试工作台用户权限
 ├─ P2-C 安装包源码
 └─ P2-D Skill 命令
          ↓
G4 全量路由与扩展功能验收
 └─ P2-E 文档、兼容和发布检查
          ↓
G5 发布验收
```

同一层的工作可以同时进行，箭头只表示验收依赖。

## 4. 验收关口

### G0：方案与公共契约确认

进入条件：无。

通过标准：

- `prd.md` 的首版范围、暂不纳入内容和验收条件没有未决问题。
- `design.md` 的命令结构、Header、输出、错误、风险级别、公共执行和产物规则已经确定。
- `OperationSpec` 最小字段、`operation_id` 命名和路由例外类型已经确定。
- 当前实际路由、已有 Swagger 操作和缺少注解的路由有可重复生成的基线报告。
- 用户确认三份规划文件，Trellis 校验通过。

### G1：CLI 基础验收

依赖：G0。

必须完成：P0-A、P0-B、P0-C、P0-D。

通过标准：

- `stx` 无参数只显示帮助，不启动服务，也不初始化数据库。
- `stx server` 正常启动现有 API 服务；隐藏的 `stx api` 仍可兼容一个版本。
- 多命名空间配置和环境变量覆盖通过测试，敏感配置文件权限为 `0600`。
- 默认 stdout 是一个合法 JSON 值；stderr 每行是合法 JSON 事件；退出码稳定。
- 操作登记能发现重复编号、命令冲突、非法修订号和无登记路由。
- CLI 令牌默认 7 天、最长 30 天，只保存哈希，并按用户当前权限鉴权。

### G2：最小端到端验收

依赖：G1。

必须完成：P0-E、P0-F、P0-G。

通过标准：

- 可以使用 `stx login` 登录真实测试服务，并通过本地命名空间复用凭据。
- CLI 请求携带约定的认证 Header、客户端 Header、版本、请求编号和超时。
- `stx capability list|get` 返回服务端版本、登记版本、操作权限和拒绝原因。
- 至少一个真实只读业务命令完成请求、输出、错误和审计的完整流程。
- 非交互环境可调用命令，不依赖 TTY 提示。
- CLI 模式不会初始化本地数据库。

### G3：安全写操作验收

依赖：G2。

必须完成：P1-B、P1-C、P1-D 的公共能力；P1-E 的强制检查已启用。

通过标准：

- R1、R2、R3 都由服务端判断和执行，CLI 不能绕过。
- 幂等键重复请求、请求摘要冲突和结束记录保留规则通过测试。
- `revision`、`ETag`、`If-Match` 和冲突返回通过测试。
- 公共执行支持查询、等待和取消；只有执行者确认停止后才能进入 `cancelled`。
- 不可取消或已经太晚的操作返回稳定原因，不能报告虚假成功。
- 执行、任务和审计按 `owner_user_id` 或实际发起用户归属，与令牌和客户端机器无关。
- 密码、密钥、令牌和配置密文不会出现在 API、CLI、日志、审计和诊断预览中。
- 在本关口通过前，所有写命令不能作为公开 CLI 命令验收。

### G4：全量路由与扩展功能验收

依赖：G3。

必须完成：P1-A、P1-E、P1-F，以及首版的 P2-A、P2-B、P2-C、P2-D。

通过标准：

- 所有实际 `/api/v1` 路由均有 `OperationSpec` 或明确的例外登记。
- 普通 API、SSE 和下载接口分别使用普通命令、`watch` 命令和 `download` 命令。
- 代理、Webhook、OAuth 回调等特殊路由不会机械生成普通 CLI 命令。
- 每个公开命令都通过 help、输出、stderr、退出码、权限和样例检查。
- 诊断资源、调试工作台权限、安装包源码和 Skill 通过各自专项验收。

### G5：发布验收

依赖：G4、P2-E。

通过标准：

- 完整 Go、Agent、前端、Swagger、许可证和构建检查通过。
- 文档、唯一 `SKILL.md`、能力接口和实际命令一致。
- 使用不允许直连 SeaTunnel 节点的测试环境，仅通过 STX CLI 完成一次典型故障信息收集。
- 发布说明写明兼容入口、最低 CLI 版本、暂不支持功能和回退方式。

## 5. 工作包与现有详细清单的对应关系

| 工作包 | 优先级 | 依赖 | 对应详细清单 | 完成条件 |
| --- | --- | --- | --- | --- |
| P0-A CLI 入口与本地配置 | P0 | G0 | 附录 A | 根命令、Server、命名空间和数据库隔离测试通过 |
| P0-B 输出协议 | P0 | G0 | 附录 D 的输出部分 | stdout、stderr、退出码和 `--pick` 契约测试通过 |
| P0-C 操作登记与路由基线 | P0 | G0 | 附录 C | 能阻止新增未登记路由和命令冲突 |
| P0-D CLI 令牌服务端 | P0 | G0 | 附录 B 的服务端部分 | 令牌有效期、哈希、撤销和实时权限测试通过 |
| P0-E HTTP 客户端与登录 | P0 | G1 | 附录 B 的客户端部分 | 真实服务登录和认证错误测试通过 |
| P0-F capabilities 与命令生成 | P0 | G1 | 附录 C、附录 D | 能力查询和通用只读请求构建测试通过 |
| P0-G 首个真实只读命令 | P0 | P0-E、P0-F | 附录 G 中选择一个稳定接口 | 达到 G2 |
| P1-A 各模块只读命令 | P1 | G2 | 附录 G | 对应只读路由覆盖缺口清零 |
| P1-B 风险、幂等、资源版本 | P1 | G2、P0-C | 附录 E | R1/R2/R3、冲突和重试测试通过 |
| P1-C 公共异步执行 | P1 | G2、P0-D | 附录 F | 状态迁移和真实取消测试通过 |
| P1-D 审计、用户归属、敏感信息 | P1 | G2、P0-D | 附录 I | 用户隔离、只追加审计和脱敏测试通过 |
| P1-E 覆盖强制检查 | P1 | G2、P0-C | 附录 K | 路由、OpenAPI、登记和 CLI 检查进入 CI |
| P1-F 写命令和异步命令 | P1 | G3、对应 P1-A | 附录 G | 对应模块安全规则和路由覆盖测试通过 |
| P2-A 诊断资源与 Agent 产物 | P2 | G3、P1-C、P1-D | 附录 H | 诊断、取消、产物中转和过期清理测试通过 |
| P2-B 调试工作台用户权限 | P2 | G3、P1-D | 附录 I | 私有、共享、运行归属和秘密变量测试通过 |
| P2-C 安装包源码 | P2 | G2；公开写命令依赖 G3 | 附录 J | 自动下载、导入、后补、校验和删除测试通过 |
| P2-D Skill 命令 | P2 | G2；最终内容依赖公开命令稳定 | 附录 J | 安装、更新、备份、恢复和示例测试通过 |
| P2-E 文档与发布 | P2 | G4 | 附录 K、附录 L | 达到 G5 |

## 6. 可同时进行的工作

### G0 之后

以下四条工作流可同时开始：

- P0-A：CLI 入口与配置。
- P0-B：输出协议。
- P0-C：操作登记与路由基线。
- P0-D：CLI 令牌服务端。

它们必须使用 G0 已确认的公共契约。公共文件推荐按“入口和包骨架、输出类型、操作登记类型、认证中间件”的次序合入，每次合入后运行对应单元测试。

### G1 之后

- P0-E 客户端和登录可以与 P0-F capabilities Handler 同时进行。
- 通用命令构建器必须复用 P0-B 的输出类型和 P0-C 的登记类型。
- P0-G 必须等待 P0-E 和 P0-F 完成，不能提前验收。

### G2 之后

- 各业务模块的只读命令可以按模块组同时接入。
- P1-B、P1-C、P1-D 可以分别开发，最后通过 G3 做联合验收。
- Swagger 注解可随模块接入同时补充，P1-E 负责统一检查。
- P2-C 的数据模型和下载部分可提前开发，但写命令不得在 G3 前公开。
- P2-D 的本地 Skill 安装器可提前开发，但 Skill 内容不得在公开命令稳定前定稿。

只读模块建议分为六组，分配互不重叠的代码范围：

1. auth、admin、dashboard、health。
2. host、cluster、config、discovery。
3. package、installer、plugin。
4. monitor、monitoring、diagnostics 查询。
5. sync 调试工作台查询、STX upgrade 查询。
6. audit 查询、Agent 命令记录、release bundle 元数据。

### G3 之后

- 不同模块的写命令可以同时接入。
- P2-A、P2-B、P2-C、P2-D 可以同时进行。
- 诊断产物涉及 Agent、控制端和 CLI 三端，可按协议、Agent 执行、STX 中转、CLI 下载安排独立工作，但协议必须先确认。

## 7. 必须串行的工作

- 根命令入口完成后，才能验收登录和远端命令不会初始化数据库。
- 输出类型和操作登记类型确定后，才能完成通用命令构建器。
- CLI 令牌服务端完成后，才能进行真实登录调用。
- HTTP 客户端与 capabilities 完成后，才能验收首个真实只读命令。
- 首个只读命令通过后，才能批量接入业务模块，避免重复修改命令协议。
- 风险确认、幂等、资源版本、公共执行、审计和敏感信息处理通过 G3 后，才能公开写命令和异步命令。
- Agent 文件协议确定后，才能修改 `.proto` 和实现大型产物中转。
- 公开命令稳定后，才能定稿 `SKILL.md` 和发布文档。

## 8. 当前不得开始的工作

- 不得批量生成全部 CLI 命令，先完成 P0-C 和 P0-G。
- 不得公开 create、update、delete、submit、upgrade、dump 等写命令，先通过 G3。
- 不得修改 Agent 文件协议，先确认诊断产物的请求、分块、校验、过期和错误契约。
- 不得实现火焰图、JFR、async-profiler、审计导出、任意 Shell 或任意文件读取。
- 不得新增独立 Scheduler、Worker 服务。
- 不得新增模拟升级。
- 不得为未稳定的命令维护多份 Skill 文件。
- 不得因为规划文件完成而自动执行 Trellis `start`。

## 9. 推荐交付批次

| 批次 | 范围 | 结束条件 | 可交付结果 |
| --- | --- | --- | --- |
| 1. CLI 基础 | P0-A、P0-B、P0-C、P0-D | G1 | 服务入口不受影响，CLI 本地基础、输出、登记和令牌可独立验收 |
| 2. 最小可用 CLI | P0-E、P0-F、P0-G | G2 | AI Agent 可以登录、发现能力并执行真实只读命令 |
| 3. 安全公共能力 | P1-B、P1-C、P1-D、P1-E 强制检查 | G3 | 写操作、异步操作、取消、审计和敏感信息处理使用统一规则 |
| 4. 现有 API 覆盖 | P1-A、P1-F | 普通业务路由都有命令或例外登记 | 首版全接口 CLI 主体完成 |
| 5. 扩展功能 | P2-A、P2-B、P2-C、P2-D | G4 | 诊断资源、工作台权限、源码包和 Skill 可用 |
| 6. 发布准备 | P2-E | G5 | 完整测试、文档和端到端演练通过 |

## 10. Trellis 子任务建议

当前任务保留为父任务。用户确认本计划后，再创建以下子任务；现在不创建，也不启动：

1. CLI 入口、配置和输出协议。
2. 操作登记、能力接口和路由覆盖检查。
3. CLI 令牌、HTTP 客户端和最小只读命令。
4. 风险确认、幂等、资源版本和公共执行。
5. 审计、用户归属和敏感信息处理。
6. 现有 API 的 CLI 接入，按模块组继续划分互不重叠的子任务。
7. 诊断资源和 Agent 产物。
8. 调试工作台用户权限。
9. 安装包源码。
10. Skill、文档和发布检查。

每个子任务开始前必须写明依赖的验收关口、允许修改的目录、不得修改的公共文件、验证命令和完成条件。

## 11. 详细工作清单

下面的清单用于说明具体工作和测试，不表示必须按附录字母串行执行。实际次序以工作包依赖和 G0 至 G5 为准。

## 附录 A：入口与本地 CLI 基础

- [x] 将根 Cobra 命令改为 `stx`，无参数显示帮助。
- [x] 新增公开 `stx server`。
- [x] 将数据库初始化和迁移移入 Server 启动路径。
- [x] 将 `stx api` 改为隐藏的废弃别名，行为等同于 `stx server`。
- [x] 移除公开的 `scheduler`、`worker` 命令注册。
- [ ] 建立 `internal/cli/config`、`client`、`output`、`command` 基础包。
  - [x] 完成 `internal/cli/config`。
  - [ ] 完成 `internal/cli/client`、`output`、`command`。
- [x] 实现多命名空间配置、环境变量覆盖和 `0600` 文件权限。
- [x] 实现 `namespace list|show|use|delete`。
- [x] 为根命令、Server 命令和本地配置补单元测试。

验证：

```bash
go test ./internal/cmd/... ./internal/cli/...
go run . --help
go run . server --help
```

2026-09-18 实际二进制验证：

- [x] 构建并压缩 `darwin/arm64` 的 `stx` 二进制。
- [x] 交叉构建并压缩生产常用的 `linux/amd64`、`linux/arm64` 静态二进制。
- [x] 从压缩包重新解压执行，不使用源码目录中的临时二进制。
- [x] 无服务端配置运行 `stx`、`stx --help`、`stx server --help` 和 `stx namespace list`，未创建数据库。
- [x] 使用独立 SQLite 配置启动 `stx server`，完成首次迁移并创建 46 张业务表。
- [x] `/api/v1/health`、默认管理员登录和带会话的 `/api/v1/auth/user-info` 均返回 HTTP 200。
- [x] 压缩包通过 `tar -tzf` 完整性检查并生成 SHA-256。
- [x] 本机 Docker Server 未运行，因此 Linux 包只完成交叉编译和包完整性检查，未伪装成已运行验证。

回退点：入口改造必须保持旧 `api` 别名可用，确认新 Server 启动正常后才能继续。

## 附录 B：CLI 登录和 HTTP 客户端

- [ ] 扩充 auth 模块，增加 CLI 令牌模型、登录、列表和撤销接口。
- [ ] 令牌只保存哈希，默认 7 天，最长 30 天。
- [ ] 实现 `stx login`、`logout`、`whoami`。
- [ ] TTY 隐藏输入密码，非 TTY 使用 `--password-stdin`。
- [ ] HTTP 客户端统一附加 Bearer Token、客户端 Header、请求编号和超时。
- [ ] 实现网络错误、认证错误、权限错误和服务端错误分类。
- [ ] 确认日志、错误和审计不包含令牌原文。

验证：

```bash
go test ./internal/apps/auth/... ./internal/cli/client/... ./internal/cli/config/...
```

## 附录 C：操作登记与能力接口

- [x] 新增共享 `OperationSpec` 和登记表。
- [x] 确认 `operation_id` 唯一、命令路径无冲突、修订号有效。
- [ ] 新增 `GET /api/v1/capabilities`。
- [ ] 实现 `stx capability list|get`。
- [x] 建立路由例外类型：`server_only`、`proxy`、`watch`、`download`。
- [x] 编写路由覆盖检查，先输出当前缺口，再逐模块清零。
- [x] 为登记表中的 help 示例和输出样例增加结构校验。

2026-09-18 基线结果：

- 实际 `/api/v1` 路由 239 条，Swagger 90 个路径、107 个操作。
- 当前登记 1 个普通操作，登记 19 个特殊路由例外，记录 219 个历史缺口。
- `go run ./internal/operation/cmd/contract-report --root .` 可重复生成报告。
- 使用 `--write` 更新基线时，默认拒绝新增未登记缺口；只有显式增加 `--allow-new-gaps` 才能接受新缺口。
- 当前阶段没有开始 capabilities 接口和 CLI 命令生成，它们仍按 P0-F 的依赖顺序处理。

验证：

```bash
go test ./internal/operation/... ./internal/router/...
```

检查点：普通业务路由没有登记或例外说明时，测试必须失败。

## 附录 D：输出协议和通用命令构建器

- [x] 实现默认 JSON 外层结构。
- [x] 实现 `--output`、`--format`、`-f`。
- [x] 实现 `--pick` 和 `pick_fallback`。
- [x] 实现 stderr NDJSON 事件写入器。
- [x] 实现稳定退出码映射。
- [ ] 实现普通 path/query/body 请求的通用 Cobra 命令构建器。
- [x] 对列表、单对象和空结果增加契约测试。
- [x] 保证 `raw` 只渲染已经进入安全结果结构的 `data`，不会读取另一份未处理响应，并继续应用 `--pick`；服务端权限与脱敏的端到端测试在 HTTP 客户端和真实业务命令接入后继续验证。

2026-09-18 实际二进制验证：

- [x] 从重新生成的 `darwin/arm64` 压缩包解压运行，不使用源码目录临时二进制。
- [x] 验证 `json`、`yaml`、`table`、`raw` 四种输出。
- [x] 验证有效 `--pick` 保留协议外层，缺少字段时 stdout 返回原安全结果且 stderr 输出 `pick_fallback`。
- [x] 验证失败时 stdout 为空，stderr 为单个 JSON 错误事件。
- [x] 验证命名空间不存在返回退出码 5，未知命令和非法格式返回退出码 2。
- [x] 使用同一压缩包重新启动 SQLite Server，健康接口返回 HTTP 200。
- [x] 刷新 `linux/amd64` 和 `linux/arm64` 静态包并重新生成 SHA-256。

验证：

```bash
go test ./internal/cli/output/... ./internal/cli/command/...
```

## 附录 E：确认、幂等和资源版本

- [ ] 新增服务端风险识别和影响返回结构。
- [ ] 实现 R1 `--confirm`。
- [ ] 实现 R2/R3 一次性确认编号和 `next_action`。
- [ ] 实现幂等记录、请求摘要和 24 小时结束记录保留。
- [ ] 实现 `idempotency_conflict`。
- [ ] 为可修改资源增加 `revision`、`ETag` 和 `If-Match` 处理。
- [ ] 实现 `revision_conflict`。
- [ ] 验证 TTY 自动二次请求与非 TTY 显式确认行为。

验证：

```bash
go test ./internal/operation/... ./internal/apps/...
```

检查点：任何 R2/R3 Handler 都不能在缺少有效服务端确认时产生实际操作。

## 附录 F：公共异步执行

- [ ] 新增公共执行模型、repository、service 和 handler。
- [ ] 实现公共状态迁移和终态保护。
- [ ] 实现 `stx execution get|wait|cancel`。
- [ ] 让各模块异步操作返回稳定 `execution_id`，并保存模块引用。
- [ ] 按用户保存 `owner_user_id`，系统执行使用明确 `actor_type`。
- [ ] 修正只改数据库状态的假取消。
- [ ] 为不可取消阶段返回稳定原因。
- [ ] 覆盖完成与取消同时发生、重复取消和迟到回调测试。

验证：

```bash
go test ./internal/apps/execution/... ./internal/apps/task/... ./internal/apps/installer/... ./internal/apps/sync/... ./internal/apps/stupgrade/...
```

## 附录 G：现有 API 的 CLI 覆盖

按以下顺序接入，先只读，再写入，再异步操作：

- [ ] auth、admin、dashboard、health。
- [ ] host、cluster、config、discovery。
- [ ] package、installer、plugin。
- [ ] monitor、monitoring。
- [ ] diagnostics。
- [ ] sync 调试工作台。
- [ ] STX upgrade。
- [ ] audit 与 Agent 命令记录。
- [ ] release bundle 和专用下载接口。
- [ ] 为 proxy、Webhook、OAuth 回调等补例外登记。

每个模块完成时检查：

- [ ] 命令帮助和样例。
- [ ] 权限、风险、审计。
- [ ] JSON/table/yaml/raw。
- [ ] 错误码和退出码。
- [ ] 路由覆盖测试。

## 附录 H：诊断资源和 Agent 产物

- [ ] 将诊断证据登记为可单独执行的资源。
- [ ] 诊断包改为选择资源组合，并复用单项执行器。
- [ ] 增加类直方图 Agent 命令和参数校验。
- [ ] 实现同一 JVM 诊断互斥和 `target_busy`。
- [ ] 实现 Heap Dump 十分钟限制。
- [ ] 实现线程快照和类直方图真实取消。
- [ ] Heap Dump 开始后返回不可取消。
- [ ] 文本结果执行脱敏和 1 MiB/256 KiB 处理。
- [ ] 新增 Agent 到 STX 的受限文件分块读取协议。
- [ ] 实现 `artifact_id` 鉴权、续传、长度和 SHA-256 校验。
- [ ] 增加 7 至 30 天产物清理。
- [ ] 自动策略默认不选择 JVM 诊断，Heap Dump 选项限制管理员。

涉及 `.proto` 时执行：

```bash
make proto
go test ./...
(cd agent && go test ./...)
```

回退点：新的 Agent 协议必须兼容尚未升级的 Agent，并通过 capability 返回不支持原因。

## 附录 I：用户归属、审计和秘密处理

- [ ] 调试工作台任务增加所有者和私有/共享范围。
- [ ] 运行历史按实际发起用户过滤。
- [ ] 全局变量增加所有者和私有/共享范围。
- [ ] 任务运行只加载有权引用的变量。
- [ ] 普通用户只能查询自己的执行和审计。
- [ ] 审计事件采用只追加方式。
- [ ] 建立共用秘密处理能力，并补齐任务正文、快照、日志和诊断结果的脱敏。
- [ ] 验证掩码回写不会覆盖原秘密。

验证：

```bash
go test ./internal/apps/sync/... ./internal/apps/audit/... ./internal/apps/diagnostics/...
```

## 附录 J：安装包源码和 Skill

- [ ] 安装包状态增加关联源码信息。
- [ ] 自动下载默认同时下载源码。
- [ ] 手工导入支持运行包和可选源码包。
- [ ] 实现源码后补、替换、校验和下载。
- [ ] 删除运行包时删除关联源码。
- [ ] 使用 `go:embed` 携带唯一 `SKILL.md`。
- [ ] 实现 Skill 的 show/status/install/update/backup/restore。
- [ ] 只管理 Claude 和 Agents 目录。

验证：

```bash
go test ./internal/apps/installer/... ./internal/cli/...
```

## 附录 K：Swagger、文档和发布检查

- [ ] 为缺失的普通业务接口补 Swagger 注解。
- [ ] 特殊路由在操作登记中说明原因。
- [ ] CI 运行 Swagger 生成并检查差异。
- [ ] 编写 CLI 指南、命令样例、AI Agent 使用说明和安全说明。
- [ ] 检查 `SKILL.md` 示例与真实命令一致。
- [ ] 检查所有公开命令的 stdout、stderr 和退出码。

验证：

```bash
scripts/swagger.sh
git diff --exit-code -- docs
go test ./...
(cd agent && go test ./...)
(cd frontend && pnpm test)
(cd frontend && pnpm build)
scripts/license.sh
```

## 附录 L：最终验收

- [ ] `stx` 无参数只显示帮助，不访问数据库。
- [ ] `stx server` 能正常启动现有平台。
- [ ] AI Agent 可通过非交互方式登录并调用 CLI。
- [ ] 全部实际路由进入操作登记或例外清单。
- [ ] 普通命令默认 stdout 是单个合法 JSON 值。
- [ ] stderr 每行是合法 JSON 事件。
- [ ] R1/R2/R3、幂等和资源版本冲突均经过测试。
- [ ] 所有异步操作可使用公共执行编号查询。
- [ ] 不支持真实取消的阶段不会报告取消成功。
- [ ] 秘密不会通过 API、CLI、日志、审计或诊断产物预览泄露。
- [ ] AI 无法 SSH 节点时，仍能通过 STX CLI 完成一次典型故障信息采集。
