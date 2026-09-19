# 实施步骤

## 前置验收

- [x] 完成本地 CLI 登录、健康检查、身份和能力查询验证。
- [x] 完成真实异步任务取消路径验证。
- [x] 完成真实异步任务成功路径验证。
- [x] 用户已要求继续实施现有 API CLI 覆盖。

## 实施顺序

### 1. 读取规范并确认现状

- [x] 执行 `trellis-before-dev`，读取 backend、测试和通用指南中适用的规范。
- [x] 检查操作登记、能力接口、CLI client/output 和根命令注册方式。
- [x] 确认工作区中与本任务无关的文件，后续不暂存这些文件。

### 2. 增加第一批操作登记

- [x] 在 `internal/operation/registry.go` 登记 9 个只读操作。
- [x] 为 path/query 输入、命令示例和输出样例补齐数据。
- [x] 扩充登记表校验测试和路由覆盖基线。

### 3. 实现普通 GET 命令构建器

- [x] 新增 `internal/cli/command` 包及 Apache 2.0 许可证头。
- [x] 实现命令树创建、path 替换、query flag、命名空间和统一输出。
- [x] 实现 capability 存在性、权限、mode 和 revision 检查。
- [x] 为正常请求、离线 help、输入错误和能力拒绝增加单元测试。

### 4. 接入根命令

- [x] 从登记表筛选首批普通 GET 操作并挂到 `stx` 根命令。
- [x] 保持 login、logout、whoami、health、capability 和 execution 的专用命令不变。
- [x] 增加根命令帮助和命令路径测试。

### 5. 自动验证

- [x] `gofmt` 新增或修改的 Go 文件。
- [x] `go test ./internal/operation/... ./internal/cli/... ./internal/cmd/...`
- [x] `go run ./internal/operation/cmd/contract-report --root .`
- [x] `go test ./...`。
- [x] `go build -o dist/stx .`

### 6. 本地真实服务验证

- [x] 使用隔离的 CLI 配置登录 `http://127.0.0.1:17800`。
- [x] 检查 `stx host --help`、`cluster --help`、`config --help` 在不依赖网络时可显示。
- [x] 对第一批 9 个命令进行真实调用；使用了主机 ID `10`、集群 ID `6` 和配置 ID `95`。
- [x] 验证 JSON、YAML、table、raw、`--pick` 和未找到退出码 5。
- [x] 验证结束后退出登录，CLI 配置文件权限为 `0600`，stdout 未输出令牌。

### 7. 检查和提交

- [x] 使用 `trellis-check` 检查需求、代码、测试和真实验证记录。
- [x] 已将操作登记生成普通 GET CLI 的约定写入 `.trellis/spec/backend/cli-guidelines.md`。
- [x] 只暂存本任务文件，使用中文提交信息提交。

## 验证结果

- 日期：2026-09-19。
- 路由：246；Swagger 操作：114；登记操作：17；特殊例外：19；历史缺口：210。
- `go test ./...`、目标包测试、`go vet` 和 `git diff --check` 全部通过。
- 最终二进制：`dist/stx`，darwin/arm64，SHA-256 为 `c0e4b4298b22c6e4cb7438f81cf7a040dea143a03c8d908a2e76ba48a588ac7a`。
- 最新二进制连接本地服务完成 9 个新命令的真实调用，Agent 在服务重启后重新注册成功。
- 已记录配置接口返回完整正文的已有安全问题，本轮验证使用 `--pick` 排除正文。

## 可并行部分

- 操作登记与命令构建器测试设计可以并行检查，但实现时登记结构是命令构建器测试的输入，应先完成登记字段确认。
- 路由覆盖报告和 CLI 单元测试可在实现后并行运行。
- 真实服务验证必须在构建和单元测试通过后执行。

## 风险文件

- `internal/operation/registry.go`：能力接口会直接暴露新增登记项。
- `internal/cmd/root.go`：错误注册可能影响所有根命令。
- `internal/cli/command/*`：必须保证 help 阶段不访问网络。
- `internal/operation/testdata/route_baseline.json`：只允许减少本次已登记的历史缺口，不接受无关变化。

## 回退方式

不执行自动回撤。需要回撤时先向用户说明范围并取得确认，且禁止使用 `git restore`。
