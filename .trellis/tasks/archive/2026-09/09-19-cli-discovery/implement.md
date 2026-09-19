# 实施步骤

## 1. 准备

- [x] 读取本任务文件、backend CLI 规范、公共执行安全规范和通用指南。
- [x] 确认 discovery 三条路由、Agent 返回和前端实际使用情况。
- [x] 确认本机 STX 与 Agent 当前状态。

## 2. 扩充普通命令构建器

- [x] 允许无请求体的 R0 POST 普通命令。
- [x] 保持对高风险 POST 和 body/header/file 输入的拒绝。
- [x] 在帮助中展示已登记的影响说明。
- [x] 增加请求方法、拒绝条件和离线帮助测试。

## 3. 登记 discovery

- [x] 新增 `host.discovery.process.list`。
- [x] 为 `/discover` 和 `/discover/confirm` 增加带真实原因的例外登记。
- [x] 扩充操作登记和根命令测试。
- [x] 更新路由基线。

## 4. 自动验证

- [x] 执行 `gofmt`。
- [x] 运行目标包测试和路由契约检查。
- [x] 运行 `go test ./...`、`go vet ./...` 和 `git diff --check`。
- [x] 构建 `dist/stx` 并记录架构与 SHA-256。

## 5. 真实验证

- [x] 用最新二进制重启本机 STX，确认 Agent 在线。
- [x] 使用隔离配置登录。
- [x] 真实执行进程发现并验证 Agent 返回。
- [x] 验证 JSON、table、raw、`--pick`、404、Agent 未安装和帮助内容。
- [x] 验证完成后退出登录。

## 6. 完成

- [x] 使用 `trellis-check` 检查实现和验证记录。
- [x] 判断项目规范是否需要补充。
- [x] 只提交本任务文件，使用中文提交信息。
- [x] 归档任务并记录开发日志。

## 验证记录

- 真实服务：HTTP `17800`，gRPC `17890`。
- 真实主机：ID `10`，在线 Agent 返回 1 个 SeaTunnel `2.3.13` hybrid 进程。
- 正常输出：JSON、table、raw 和 `--pick success,processes` 均通过。
- 不存在主机：退出码 `5`，stdout 为空，错误类型为 `not_found`。
- Agent 未安装：临时主机返回退出码 `9`，错误消息为 `agent not installed / Agent 未安装`；临时主机已删除，无残留。
- 路由报告：`route_count=246`、`swagger_operation_count=114`、`registered_operations=26`、`route_exceptions=21`、`historical_gaps=199`。
- 二进制：`dist/stx`，`darwin/arm64`，SHA-256 `52fbf8dc3ed758c7892fc5362ba78cf58698804ab9673e460884b98db71b0b06`。
- 最新二进制重启服务后再次验证 JSON、table、raw 和 `--pick`，主机 `10` 仍返回同一 SeaTunnel 进程；验证结束后已退出登录。
