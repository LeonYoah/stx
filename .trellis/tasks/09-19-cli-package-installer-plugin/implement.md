# Package Installer Plugin CLI 只读覆盖实施计划

## 1. 前置检查

- [x] 读取 backend、CLI、安全和质量规范。
- [x] 确认 14 条路由的 Handler 参数和返回方式。
- [x] 记录并避开工作区内 4 个无关文档改动。

## 2. 公共查询参数能力

- [x] 为 `InputSpec` 增加重复查询参数标记。
- [x] 更新操作登记校验，限制该标记只能用于 query。
- [x] 更新生成式命令构建器，支持 string-array flag 和多个同名 URL 参数。
- [x] 增加单元测试，覆盖单值、重复值、空值和必填判断。

## 3. 操作登记和命令

- [x] 在登记表加入 14 个 GET 操作及帮助样例。
- [x] 更新操作登记集合和关键字段测试。
- [x] 更新根命令测试，验证 14 条命令路径。
- [x] 重新生成路由基线并检查统计变化。

## 4. 本地质量检查

- [x] `gofmt` 修改过的 Go 文件。
- [x] `go test ./internal/operation/... ./internal/cli/command/... ./internal/cmd/...`。
- [x] `go test ./...`。
- [x] `go vet ./...`。
- [x] `go run ./internal/operation/cmd/contract-report --root .`。
- [x] 仅对本任务文件运行 `git diff --check -- <files>`。

## 5. 真实服务验证

- [x] 构建 `dist/stx` 并确认软连接目标。
- [x] 重启本地 STX 服务，确认 17800 和 17890 端口可用。
- [x] 使用隔离 `XDG_CONFIG_HOME` 登录本地 STX。
- [x] 从真实 host、cluster、package 和 plugin 列表获取可用参数。
- [x] 调用 14 个新增命令并保存命令、退出码、stdout 和 stderr 摘要。
- [x] 验证 JSON、table、raw、`--pick`、重复 `profile_keys` 和稳定错误退出码。

## 6. 完成

- [x] 使用 `trellis-check` 检查规范、测试和变更范围。
- [x] 更新需要长期保留的 CLI 规范；已补充同名 query 参数规则。
- [ ] 中文提交本任务代码。
- [ ] 归档 Trellis 任务并记录开发日志。

## 风险点

- capability 来自运行中的服务端，修改登记后必须重启服务，否则新命令会在业务请求前被拒绝。
- 插件目录查询可能访问外部仓库，真实验证需要记录超时或网络失败，不能把网络条件误报为 CLI 失败。
- 本地可能没有安装或下载任务；此时空列表和未找到错误用于验证协议，不伪造生产数据。
- 当前全仓 `git diff --check` 会被既有文档行尾空格影响，必须使用本任务文件列表做定向检查。
