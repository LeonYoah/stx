# Auth 与管理员用户写命令技术方案

## 1. 命令结构

本批使用专用命令处理 JSON 正文、布尔字段和密码输入，不扩展普通 GET 命令构建器。package 写命令中已有的命名空间、确认、幂等和 capability 检查抽成共用辅助方法，后续 host、cluster 和 config 写命令可以继续复用。

命令参数：

```text
stx auth profile update --email <email> --language <zh|en> --confirm
stx admin user create --username <name> [--nickname <name>] [--email <email>] [--admin] [--password-stdin] --confirm
stx admin user update <id> [--nickname <name>] [--email <email>] [--active=<bool>] [--admin=<bool>] [--password-stdin] --confirm
stx admin user delete <id> --confirm [--confirmation-id <id>]
```

## 2. 客户端安全输入

- 复用登录命令的终端密码读取能力，先关闭回显再显示 `Password:`。
- 非交互环境只有显式传入 `--password-stdin` 才读取 stdin。
- 创建用户必须读取密码；更新用户只有传入 `--password-stdin` 时才加入 `password` 字段。
- 布尔参数使用 Cobra `Changed` 判断是否加入正文，避免未传入时把 `false` 误写到服务端。

## 3. 共用写请求辅助方法

新增共用安全写选项和准备方法，负责：

1. 本地检查 `--confirm`。
2. 读取命名空间并创建客户端。
3. 查询 capability，检查权限、修订号和模式。
4. 生成或复用幂等键。
5. 发送 `Idempotency-Key`、`X-STX-Confirm` 和可选 `X-STX-Confirmation-ID`。
6. 向 stderr 写结构化 warning 和自动生成的幂等键。

package 写命令改用该辅助方法，行为保持不变。

## 4. 服务端同步执行

在公共执行服务增加同步写操作的开始和结束方法：

- 开始时先查幂等记录，再执行 R1/R2/R3 校验，并创建 `running` 执行记录。
- 业务成功后写入 `succeeded` 和安全的 `result_ref`；失败后写入 `failed` 和错误信息。
- 已成功的幂等请求由业务 Handler 根据 `result_ref` 重新读取安全响应。
- 创建用户保存用户 ID；更新和删除使用路径用户 ID；个人资料使用当前用户 ID。

该方法不保存请求正文。密码只在内存中的业务请求结构出现，请求摘要使用密码 SHA-256。

## 5. Handler 依赖

新增带依赖的写 Handler，由 `router.go` 注入公共执行服务和审计仓库：

- auth 个人资料写 Handler。
- admin 用户写 Handler。

现有只读 Handler 保持不变。为避免破坏网页，服务端安全校验先只对 `X-STX-Client: cli` 和直接 API 调用启用，网页继续执行原有业务审计；后续前端接入确认协议后再移除兼容分支。

## 6. 风险等级

| 操作 | 风险 | 原因 |
| --- | --- | --- |
| 修改个人资料 | R1 | 会改变当前用户邮箱或语言，但可再次修改 |
| 创建用户 | R1 | 新增账号，可由管理员删除 |
| 更新用户 | R1 | 可修改状态、角色或密码，需要显式确认 |
| 删除用户 | R2 | 删除后应用内没有恢复入口，需要一次性确认编号 |

## 7. 错误和重试

- 缺少显式确认：HTTP 428，CLI 退出码 6。
- 缺少幂等键：HTTP 400，CLI 用法或服务端校验错误。
- R2 第一次调用：HTTP 428，返回确认编号、风险和到期时间。
- 幂等键冲突：HTTP 409，CLI 退出码 6。
- 普通用户调用管理员接口：HTTP 403，CLI 退出码 4。
- 用户不存在：HTTP 404，CLI 退出码 5。
- 删除自己：HTTP 400，不创建删除结果。

## 8. 兼容和回退

- 不修改数据库结构和现有用户表。
- 不改变登录、退出和只读用户查询。
- CLI 命令可通过取消根命令注册回退，服务端安全封装可单独保留。
- 不自动回撤；需要回撤时由用户确认具体文件和提交。
