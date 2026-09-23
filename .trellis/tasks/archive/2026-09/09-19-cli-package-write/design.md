# 安装包管理 CLI 写操作覆盖技术方案

## 1. 设计边界

本批把普通 JSON 请求与文件上传分开处理。版本刷新和主机安装命令查询可以复用现有操作登记与生成命令；上传、分片上传、下载、取消和删除需要在同一个 `package` 命令树下增加专用叶子，以处理 multipart、文件流和确认重试。

不为本批重新设计任务表。现有安装包下载任务继续由安装包服务维护，但取消流程必须补上真实停止下载的信号传递，并在状态确认后再返回取消结果。

## 2. 命令结构

建议使用以下命令路径：

```text
stx host agent install-command get <host-id>
stx package version refresh
stx package upload <file>
stx package upload chunk <file>
stx package delete <version>
stx package download start <version>
stx package download cancel <version>
```

具体名称以现有操作登记命名和根命令树为准，不能与已经存在的 `package` 根命令冲突。每个写命令应支持 `--namespace`、`--idempotency-key`、`--confirm`，必要时支持 `--confirmation-id` 以便重试确认流程。

## 3. 客户端请求层

在 `internal/cli/client` 增加流式 multipart 请求能力：

- 使用 `io.Pipe` 或 multipart writer 将文件逐步写入请求体。
- 请求结束后始终关闭文件和请求体，取消上下文时停止写入。
- 复用现有响应 envelope 解析、HTTP 错误分类、超时和 `X-Request-ID` 返回。
- 只允许透传已登记的安全 Header，包括 `Idempotency-Key`、`X-STX-Confirm` 和 `X-STX-Confirmation-ID`。
- 上传文件过大时不能先 `io.ReadAll`；测试应使用带计数的 reader 或临时文件验证流式行为。

## 4. 服务端确认与幂等

安装包 Handler 接入现有 execution service 的最小能力：

1. 根据操作 ID、风险等级、影响说明和请求摘要调用授权逻辑。
2. 缺少确认时返回统一的确认错误；CLI 在 `--confirm` 下带上确认 Header，并沿用同一个请求摘要和幂等键重试。
3. 相同幂等键和请求摘要返回原执行结果或已有下载任务；摘要不一致返回冲突。
4. 审计记录包含用户、操作 ID、目标版本、请求 ID、幂等键、结果和失败原因。

上传分片的底层接口允许高层上传命令逐片调用，但每一片必须带上同一上传 ID和幂等上下文；服务端已有顺序限制不在 CLI 中伪装成可重放。

## 5. 下载取消

检查并修正下载 goroutine 与任务状态之间的连接：

- 下载任务持有 `context.CancelFunc` 或等效取消信号。
- `CancelDownload` 设置取消请求后触发实际网络请求和文件写入停止。
- 后台下载收到取消后只能进入 `cancelled`，并删除未完成临时文件。
- 如果下载已完成或已进入不可取消阶段，返回明确状态，不覆盖已完成结果。
- 取消接口重复调用保持幂等，返回当前最终状态。

## 6. 输出和错误

普通 JSON 接口沿用通用输出渲染。写命令的成功结果包含请求 ID、幂等键、任务 ID或版本信息；影响较大的操作在 stderr 发送结构化 warning，不能把 warning 混进机器可读 stdout。

服务端错误统一映射为已有 CLI 错误码。确认缺失、幂等冲突、未找到、不可取消、超时和网络失败必须能区分，便于 Agent 根据退出码决定是否重试。

## 7. 兼容与回退

- 默认不改变已有 GET 命令。
- 只新增客户端方法和 package 专用命令，不改动前端 Header 约定。
- 若某个写接口的服务端安全接入需要扩大范围，先保持命令不可执行并报告缺口，不通过 CLI 绕过保护。
- 测试文件和临时安装包使用独立版本号，验证结束后删除，不触碰现有版本。
