# 技术设计

## 范围

本批只处理普通只读 HTTP GET 操作。登录、退出、能力查询、健康检查和公共执行命令继续使用已有专用实现。

## 代码边界

### 操作登记

在 `internal/operation/registry.go` 增加第一批 9 个操作。每项登记包含：

- 稳定的 `operation_id` 和 `command_path`。
- HTTP 方法、API 路由、认证要求、风险等级和修订号。
- path/query 输入说明。
- 可直接执行的命令示例和合法输出样例。

登记项仍是能力接口、CLI 帮助和路由覆盖检查的共同来源。

### 通用命令构建器

新增 `internal/cli/command` 包，职责是：

1. 校验登记项属于第一版支持的 `ModeNormal + GET`。
2. 根据 `CommandPath` 组成 Cobra 命令树。
3. 把 path 输入映射为位置参数，把 query 输入映射为 flag。
4. 在执行阶段取得指定命名空间的 CLI 客户端。
5. 查询 `/api/v1/capabilities` 并检查服务端操作。
6. 生成安全的请求路径，调用已有 `Client.Request`。
7. 使用已有 renderer 输出统一结果。

`internal/cmd` 负责把登记项筛选后挂到根命令，不让 `internal/cli/command` 依赖 `internal/cmd`。

## 命令树构建

第一批命令路径存在共享父节点，例如 `cluster node list` 和 `cluster status get`。构建器按命令段逐级查找或创建父节点，只在叶子节点设置执行函数。

父节点只显示帮助，不访问网络。叶子节点的 `Use` 包含位置参数占位符，例如 `get <id>`。

若两个登记项产生相同叶子路径，构建时返回错误，避免后注册项覆盖已有命令。

## 输入处理

### path 参数

- 只接受登记为 `InputPath` 的输入。
- 按 `Input` 中出现的顺序消费位置参数。
- 必填 path 参数决定 Cobra 的精确参数数量。
- 替换 `:name` 时使用 `url.PathEscape`。
- 如果登记中的 path 输入与路由占位符不一致，构建失败。

### query 参数

- `InputQuery` 使用输入名称作为长 flag。
- 本批按字符串传入，避免为每个业务参数复制类型规则。
- 必填 query 在执行前检查；可选 query 只在用户显式传入后加入 URL。
- query 使用 `url.Values` 编码。

第一批接口以 path 参数为主。若列表接口现有 query 参数已在登记中声明，则由同一机制处理。

## 能力检查

业务请求前调用 `Client.Capabilities`：

- 找不到 operation ID：返回 `not_found`，不请求业务 API。
- `allowed=false`：返回权限错误，并保留服务端 `denial_code`。
- 服务端 revision 小于本地登记项 revision：返回冲突或兼容性错误，不请求业务 API。
- mode 与本地登记不一致：返回兼容性错误。

本批不要求服务端登记摘要与本地摘要完全相同，因为服务端可能比 CLI 新；只校验当前操作。

## 输出

业务响应的 `data` 使用 `json.RawMessage` 解码为安全 JSON 值，再交给 `cli/output`：

- 外层继续包含 `api_version`、`operation_id`、`request_id`、`data` 和精简的 `result_meta`。
- `--pick`、JSON、YAML、table 和 raw 继续走已有 renderer。
- 普通成功结果不写 stderr。
- HTTP、认证、权限、未找到和兼容性错误继续使用稳定退出码。

## 兼容与风险

- 现有专用命令路径不变。
- 本批不改变服务端 API，也不改数据库。
- 如果通用构建器存在问题，可以只取消在根命令中的注册，已有 CLI 不受影响。
- 操作登记增加后，能力接口会立即返回新操作；因此登记项和 CLI 命令必须在同一批变更中提交。

## 验证重点

- 离线 help 不调用能力接口。
- capability 允许后才调用业务 API。
- capability 缺失、拒绝和 revision 不兼容时业务 API 调用次数为 0。
- path 参数经过转义，query 只包含用户显式传入项。
- 命令输出可被 JSON 解析，`--pick` 不会绕过统一结果处理。
