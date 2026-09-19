# 技术设计

## 操作登记

新增操作：

```text
operation_id: host.discovery.process.list
command:      stx host discovery process list <host-id>
method:       POST
route:        /api/v1/hosts/:id/discover-processes
mode:         normal
risk:         R0
uses_agent:   true
```

该接口只执行进程扫描，返回 PID、角色、安装目录、版本和端口，不修改主机或 SeaTunnel 进程。

另外两条 discovery 路由作为 `server_only` 例外登记，并在原因中写清当前服务端缺陷。例外登记只是让覆盖报告如实分类，不代表接口已经可供 CLI 使用。

## 普通命令构建器

现有构建器只接受 GET。本批增加严格限制的 POST：

- `ModeNormal`。
- `Method` 为 POST。
- `Risk` 必须是 R0。
- 不得包含 body、header 或 file 输入。
- 请求体固定为 `nil`。

因此后续带修改效果、确认、幂等、请求体或文件的操作仍需专用命令或后续安全构建器，不能因为支持 POST 而自动公开。

命令帮助在已有摘要和输出样例之间增加影响说明。当登记项有 `Impact` 时，显示风险等级、影响内容和性能说明；查看帮助仍不创建客户端。

## 错误处理

业务 API 当前使用 `{"error":"..."}`，CLI 客户端已经可以从错误响应中提取消息。HTTP 404 继续映射退出码 5，其他服务端校验错误沿用公共错误映射。

## 验证方式

- 单元测试覆盖 R0 POST 成功、R1 POST 被拒绝、有 body 的 POST 被拒绝、GET 行为不变以及帮助中的影响说明。
- 路由契约检查确认一条路由变为 operation，两条路由变为 exception。
- 构建最新二进制并重启本机 STX，使用主机 `10` 和在线 Agent 真实执行命令。

## 回退说明

不自动回退。需要回退时先说明影响并取得用户确认，禁止执行 `git restore`。
