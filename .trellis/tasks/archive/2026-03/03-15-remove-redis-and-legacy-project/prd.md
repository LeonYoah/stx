# 移除 Redis 与 legacy project 默认链路

## Goal

将 STX 的默认产品形态从当前遗留的 project/redis/clickhouse/async 兼容模式，收敛为更符合现有主链路的部署形态：默认不再暴露 legacy project 业务，不再依赖 Redis 作为默认会话后端，而是使用内存会话；同时清理默认配置、默认路由与隐式依赖，避免“配置上不用，代码里仍默认依赖”的不一致。

## What I already know

* `internal/session/manager.go` 已支持：`redis.enabled=false` 时自动走内存会话。
* `config.example.yaml` 仍包含 `projectApp`、`redis`、`clickhouse`、`schedule`、`worker`、`legacy`、`oauth2` 等 legacy/兼容配置块。
* 用户倾向于 **保留 `oauth2` 配置块**，不希望这次把它一起移除。
* `internal/router/router.go` 当前仍默认注册 legacy project 相关路由：`/projects`、`/tags`、`/admin/projects`。
* `internal/apps/project/middlewares.go`、`models.go`、`routers.go` 等直接依赖 `db.Redis`。
* `internal/apps/oauth/tasks.go`、`utils.go` 依赖 `db.Redis` 与 `internal/task/schedule`（Asynq）。
* `internal/apps/dashboard/logic.go` 也直接使用 `db.Redis` 做缓存。
* `internal/apps/project/clickhouses.go` 依赖 `config.Config.ClickHouse`。

## Assumptions (temporary)

* 这次目标已收敛为：彻底移除 Redis / ClickHouse 功能代码，并物理删除 legacy project / projectApp 相关代码。
* `scheduler` / `worker` 命令代码暂时保留，但从默认配置示例中移除对应配置块。
* 会话层目标仍是内存会话，但 Redis 相关实现本身也进入删除范围。

## Open Questions

* 无。范围已确认：旧 dashboard stats/all 与 oauth badge/积分异步链路一起删除。

## Requirements (evolving)

* 默认配置示例应体现 STX 当前主链路能力。
* 默认会话应使用内存实现，不要求 Redis。
* 不应保留会误导部署者的无效/无用默认配置块。
* 若 legacy project 不再属于默认产品能力，则默认 API 不应继续注册其路由。
* 默认配置示例中移除 `clickhouse`、`redis`、`schedule`、`worker`、`projectApp`、`legacy`。
* `oauth2` 兼容配置本次保留，不纳入默认能力清退范围。
* 默认产品关闭并物理删除 legacy project 路由与模块。
* 删除 Redis 与 ClickHouse 相关功能代码，而不仅是配置示例。
* 删除旧 dashboard `stats/all` 接口与 oauth badge/积分异步链路。

## Acceptance Criteria (evolving)

* [x] `config.example.yaml` 不再包含 `redis`、`projectApp`、`legacy`。
* [x] 默认启动路径不依赖 Redis 也能正常会话登录。
* [x] 默认路由不再暴露 `/projects`、`/tags`、`/admin/projects`。
* [x] `config.example.yaml` 不再包含 `clickhouse`、`schedule`、`worker`。
* [x] Redis 与 ClickHouse 功能代码已从默认产品实现中删除。
* [x] legacy project / projectApp 相关代码与路由已物理删除。
* [x] 旧 dashboard `stats/all` 接口与 oauth badge/积分异步链路已删除。
* [x] 与该默认路径冲突的 Redis / ClickHouse / async 依赖已被同步收敛或隔离。

## Definition of Done (team quality bar)

* 相关后端测试更新并通过
* 受影响配置/路由/依赖已同步梳理
* 文档/规范更新，说明默认部署不再依赖 Redis 与 legacy project

## Out of Scope (explicit)

* 重构新的分布式任务系统来替代 badge/积分异步链路
* 引入新的分布式会话后端
* 移除 `oauth2` 兼容配置

## Technical Notes

* 关键文件：
  * `config.example.yaml`
  * `internal/session/manager.go`
  * `internal/router/router.go`
  * `internal/apps/project/*`
  * `internal/apps/oauth/tasks.go`
  * `internal/apps/oauth/utils.go`
  * `internal/apps/dashboard/logic.go`
  * `internal/task/schedule/*`
  * `internal/task/worker/*`
* 当前 repo 事实表明：仅删配置不够，至少还要处理 legacy project 路由与若干 Redis 直接调用。


## Validation

- `go test ./... -run '^$'`
- `go test ./internal/session/...`
- `cd frontend && pnpm exec tsc --noEmit`
- `cd frontend && pnpm lint --file "components/common/auth/CallbackHandler.tsx"`
- `git diff --check`
- `$(go env GOPATH)/bin/swag init -o docs --parseDependency --parseInternal`
