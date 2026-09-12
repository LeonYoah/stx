# 日志规范

> 本项目中的日志使用方式。

---

## 概述

**主后端**（根模块 `internal/`）通过 **otelzap**（`internal/logger`）使用 **Zap**，使日志可与 **OpenTelemetry** 的 trace/span ID 关联。**Agent**（`agent/` 下独立模块）使用另一套 logger，见下文 [Agent 日志](#agent-日志)。对外 API 为一组接收 `context.Context` 和格式字符串的函数：`DebugF`、`InfoF`、`WarnF`、`ErrorF`。**context 通过入参传递**：新写的方法、主类入口若会打日志或调下层，首参应为 `ctx context.Context`，从 handler/gRPC 一路向下传，打日志时统一用该 `ctx`。Logger 在启动时初始化一次；日志级别与输出（标准输出、文件或两者）在 `config.Config.Log` 中配置。高频或噪音较大的 HTTP 路径（如 Grafana 代理的 Prometheus 查询、Live WebSocket）在路由中间件中排除请求日志，以控制日志量。

---

## 日志级别

- **Debug**：详细诊断（如 `LoggerMiddleware` 中 Grafana 静态资源请求）。生产环境尽量少用，保持可读性。
- **Info**：正常操作（如 `LoggerMiddleware` 中的 HTTP 请求/响应、「集群已创建」、「Agent 已连接」）。请求日志默认级别。
- **Warn**：可恢复或意外但已处理的情况（如配置回退、重试）。
- **Error**：需要关注的失败（如命令执行失败、关键路径 DB 错误）。用于后台任务入口或关键路径。

使用 `logger.DebugF(ctx, "format", args...)`、`logger.InfoF(ctx, "format", args...)`，以及 `WarnF` / `ErrorF`。context 用于将 trace ID、span ID 附加到日志（见 `logger/utils.go`）。

---

## 结构化日志

- **格式**：通过 `config.Config.Log.Format` 配置：`json` 或 console。JSON 包含 `time`、`level`、`msg`、`caller`，以及 context 中的 `traceID`/`spanID`（若有）。
- **调用方**：Zap 配置了 `AddCaller()` 和 `AddCallerSkip(1)`，记录的是调用 `InfoF` 等的调用方，而非 logger 自身。
- **请求日志**：路由的 `loggerMiddleware()` 记录方法、路径、查询、开始/结束时间、耗时、客户端 IP、响应状态与大小。普通请求用 `logger.InfoF`，部分代理静态路径用 `logger.DebugF`；Grafana Live WS 与 Grafana Prometheus 数据源查询路径完全不记录。

---

## 应记录的内容

- **HTTP 请求**：方法、路径、查询、耗时、状态、响应大小（排除上述高频路径）。
- **重要业务事件**：如集群创建、节点添加、安装开始、Agent 注册。
- **错误**：返回 5xx 或操作失败且从响应无法明显看出原因时（用 `ErrorF` 记录并包含错误信息；避免在 service 和 handler 重复记录同一错误）。
- **启动**：数据库已连接、gRPC 已启动、配置已加载（在 `cmd`/`router` 中常用标准 `log`，因请求路径尚未使用 logger）。

---

## 不应记录的内容

- **敏感信息**：密码、令牌、API 密钥或任何用于认证的配置。不得记录可能包含凭据的请求/响应体。
- **生产环境 Info 中的 PII**：如非运维必需，避免在 Info 级别记录完整用户标识或 IP；遵循项目隐私策略。
- **高频代理流量**：Grafana Prometheus 数据源查询与 Grafana Live WebSocket 不在 HTTP 中间件中记录，以减少噪音（见 `router/middlewares.go`：`isGrafanaPrometheusQueryPath`、`isGrafanaLiveWSPath`）。
- **生产环境过多 Debug**：Debug 用于开发与排障；生产默认日志级别应为 Info 或 Warn。

---

## 使用方式

- **请求路径中**：使用 `logger.InfoF(c.Request.Context(), "消息 %s", arg)`（或从已有 `ctx` 的 handler/service 传入）。
- **新方法/主类**：会打日志或调 repository、其他 service 的，**首参定为 `ctx context.Context`**，从 handler/gRPC 一路传下去，打日志用该 `ctx`；不要在中途改用 `context.Background()`（除非已脱离请求，如后台任务入口）。
- **后台任务/legacy 命令壳**：传入带可选 trace 的 context，同样使用 `logger.*F(ctx, ...)`。
- **初始化**：部分代码仍使用标准 `log` 或 `log.Printf`（如配置、DB 初始化）；在请求链路使用 Zap 之前的启动阶段可以接受。

---

## Agent 日志

**STX Runtime Agent** 为根目录下独立 Go 模块 `agent/`，拥有自己的 logger 包 `agent/internal/logger`，但**用法与主后端一致**：统一使用 `logger.InfoF(ctx, format, args...)` 等 API。

- **导入**：`"github.com/LeonYoah/stx/agent/internal/logger"`，包名为 `logger`（与 backend 一致，不再使用 `agentlogger` 别名）。
- **调用方式**：与 backend 相同：`logger.DebugF(ctx, ...)`、`logger.InfoF(ctx, ...)`、`logger.WarnF(ctx, ...)`、`logger.ErrorF(ctx, ...)`，首参均为 `context.Context`。
- **上下文传递**：**Agent 新写方法也建议将 `ctx` 作为首参并向下传递**，便于与主后端一致、后续若接 trace 可直接复用；方法内用 `a.ctx`、已有 `ctx` 的 handler 直接传入、闭包或无上下文处用 `context.Background()`。
- **初始化**：须在 `agent/cmd/main.go` 中调用 `logger.Init(cfg)`，再使用 logger。
- **与主后端区别**：Agent 不接 OpenTelemetry，ctx 仅用于 API 一致；输出固定为控制台 + 文件、JSON 编码。

**提示**：后续开发或扩展 Agent 时，请按上述约定——新方法首参传 `ctx` 并一路向下传递，与 internal 保持一致。
