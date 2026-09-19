# Auth Admin Dashboard CLI 覆盖

## 目标

继续原 STX CLI PRD 的现有 API 覆盖，在已经完成的普通 GET 命令构建器上接入 auth、admin 和 dashboard 的 8 个只读接口，并用本机 STX 服务做真实验证。

## 范围

本批新增以下命令：

- `stx auth user-info get`
- `stx admin user list`
- `stx admin user get <id>`
- `stx dashboard overview get`
- `stx dashboard stats get`
- `stx dashboard cluster list`
- `stx dashboard host list`
- `stx dashboard activity list`

本批不处理 OAuth、用户新增修改删除、Dashboard 后端未实际读取的 `limit` 参数，也不修改已有配置接口的敏感信息问题。

## 要求

- 复用 `internal/cli/command` 的普通 GET 命令生成方式，不为每个接口手写 Cobra 命令。
- 操作登记必须与现有路由和 handler 的真实行为一致。
- 两个 admin 操作必须标记为仅管理员可用。
- 用户信息输出不得包含密码、令牌或密码哈希。
- 所有命令继续支持命名空间、统一输出和 `--pick`。
- 离线查看帮助时不能访问服务端。
- 不修改与本任务无关的发布文件。

## 验收标准

- [x] 操作登记新增 8 项，登记总数从 17 增至 25，路由历史缺口相应减少 8。
- [x] `stx --help` 和各级子命令帮助中能找到 8 个命令。
- [x] Admin 操作的 capability 对管理员为允许，对普通用户遵守服务端权限结果。
- [x] 列表参数 `current`、`size`、`username`、`is_active`、`is_admin` 能正确生成 query。
- [x] JSON 输出、至少一种非 JSON 输出和 `--pick` 经过真实服务验证。
- [x] 查询不存在的用户返回稳定的未找到退出码。
- [x] 目标包测试、全量测试、静态检查、路由契约检查和最终构建通过。
- [x] 最新 `dist/stx` 连接本机服务完成真实调用验证。
