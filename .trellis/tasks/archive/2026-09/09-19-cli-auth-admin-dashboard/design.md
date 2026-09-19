# 技术设计

## 实现范围

沿用上一批的登记驱动方式。`internal/operation/registry.go` 增加 8 个 `ModeNormal + GET` 操作并设置 `GeneratedCLI: true`，`internal/cmd/generated.go` 会自动把它们注册到根命令。

本批不修改通用命令构建器，除非测试发现它无法表达真实接口参数。

## 操作定义

| operation_id | 命令 | 路由 | 说明 |
| --- | --- | --- | --- |
| `auth.user-info.get` | `stx auth user-info get` | `GET /api/v1/auth/user-info` | 当前用户信息 |
| `admin.user.list` | `stx admin user list` | `GET /api/v1/admin/users` | 管理员用户列表 |
| `admin.user.get` | `stx admin user get <id>` | `GET /api/v1/admin/users/:id` | 管理员查询单个用户 |
| `dashboard.overview.get` | `stx dashboard overview get` | `GET /api/v1/dashboard/overview` | 完整概览 |
| `dashboard.stats.get` | `stx dashboard stats get` | `GET /api/v1/dashboard/overview/stats` | 概览统计 |
| `dashboard.cluster.list` | `stx dashboard cluster list` | `GET /api/v1/dashboard/overview/clusters` | 集群摘要 |
| `dashboard.host.list` | `stx dashboard host list` | `GET /api/v1/dashboard/overview/hosts` | 主机摘要 |
| `dashboard.activity.list` | `stx dashboard activity list` | `GET /api/v1/dashboard/overview/activities` | 最近活动 |

两个 admin 操作设置 `AdminOnly: true`。其余操作要求登录，风险等级为 R0，修订号为 1，并支持 `--pick`。

## 参数和输出

`admin.user.list` 登记现有 handler 接受的 5 个 query 参数：`current`、`size`、`username`、`is_active`、`is_admin`。布尔参数仍由通用构建器按字符串传递，服务端负责校验。

Dashboard handler 当前固定返回 5 条集群、5 条主机和 10 条活动，没有读取 Swagger 中写出的 `limit`。因此本批不登记 `limit`，避免 CLI 显示一个实际无效的参数。

用户响应使用 `auth.UserInfo`，只包含公开用户资料和权限状态，不包含密码字段或密码哈希。

## 兼容性

- 不改变服务端路由、数据库和认证中间件。
- 不改变已有专用 auth 命令。
- 新操作会出现在 capability 接口中，CLI 在调用业务接口前继续检查权限、mode 和 revision。
- 若登记出现问题，可只移除本批登记；回退前需用户确认，且不使用 `git restore`。

## 验证方式

- 单元测试检查 8 个 operation ID、命令路径、管理员限制和参数登记。
- 路由契约检查确认只减少对应 8 条历史缺口。
- 构建最新 `dist/stx`，重启本地 STX 后使用隔离配置登录。
- 真实执行全部 8 个命令，并检查输出格式、`--pick`、管理员能力和未找到退出码。
