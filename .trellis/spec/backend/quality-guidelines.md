# 质量规范

> 适用于根 Go 模块、Agent 和 Java Proxy 的代码质量标准。

---

## 概述

根模块和 Agent 使用 **Go**，根模块采用 handler、service、repository 分层，代码按功能组织在 `internal/apps/<name>/` 下。Java Proxy 使用 **Java / Maven**，服务代码按功能放在 `service/` 的二级包中。Go 测试使用标准 `testing` 包，Java 测试使用 JUnit；格式化和静态检查分别使用项目现有的 Go 工具与 Maven Spotless 配置。

---

## 注释、文件头与语言输出

### 注释语言

- **自己新增或修改的注释，要求中英双语。** 适用于你本次提交中新写的函数注释、复杂逻辑注释、接口说明注释等。
- **第三方 / 生成 / 历史镜像代码不强制回填双语。** 对于 vendor、自动生成文件或明显继承的历史文件，不要求为了补齐双语而大面积重写旧注释；但只要你新增了注释，就应按双语写法补齐。
- **双语注释以“中文在前，英文在后”为默认顺序。** 重点是两种语言表达同一语义，而不是写两套不同内容。
- **不要为了形式上的双语重复显而易见的代码。** 仍然只在必要处写注释，重点保持清晰、简洁、可维护。

### 注释密度

- **注释不宜过密。** 仅在有必要处添加注释：非常规逻辑、业务规则、复杂算法或对外 API 约定。避免逐行注释或重复代码已表达的含义。

### 新建文件须带 License 头

- **每个新建的后端源文件必须在文件顶部添加 Apache 2.0 License 头。** 紧接在 `package` 声明之前，`*/` 与 `package` 之间不要多空行。完整内容如下：

```
/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
```

### 用户可见语言输出

- **后端不应默认返回“中文 / English”拼接文案。** 面向 UI、HTML 报告、SSE 事件、任务日志等用户可见内容，应根据前端显式传入的 `lang`（如 `zh` / `en`）输出单语言结果。
- **语言切换由前端驱动。** 语言偏好来源于个人中心 / 用户资料，前端在请求 diagnostics 等需要本地化输出的接口时负责透传 `lang`，后端负责按该语言返回结果。
- **内部标识与用户文案分离。** 状态码、step code、template code 等内部字段保持稳定常量；展示名称、说明、推荐动作等用户文案再做本地化转换。
- **“支持中英文”不等于“同屏双语拼接”。** 例如 diagnostics / 巡检中心 / 诊断报告这类模块，要求是支持 `zh` / `en` 自由切换，并按当前语言返回单语言结果；不要回退到 `中文 / English` 并排或斜杠拼接输出。
- **版本敏感安装配置由后端统一裁决。** 对一键安装 / 创建集群这类会下发 `seatunnel.yaml` / `hazelcast*.yaml` 的流程，Control Plane 应返回显式 capability（如 `version_capabilities`），Agent 在最终写配置时也必须二次 gate，避免把旧版本不支持的 key（如 `history-job-expire-minutes`、`scheduled-deletion-enable`、`job-schedule-strategy`）误写进去。

### 敏感凭证与密码/Secret 字段安全契约

- **禁止在列表与详情接口回显明文敏感信息**：对于密码（password）、凭据（token / secret）、全局变量中的 Secret 类型等，查询或列表接口一律进行脱敏（如输出掩码 `******` 或置空并附加布尔标志 `is_set: true`），严禁以明文形式返回给前端。
- **更新接口遵循“默认保留原值”（Preserve-by-Default）**：在更新此类资源的接口中，若前端提交空字符串或未变更标识，后端必须保留已有密文，禁止将其误覆盖为空字符串；仅当客户端显式传入非空的新密文时才执行更新与加密持久化。
- **落库安全与日志隔离**：敏感凭据在数据库中优先使用加密存储，禁止明文落库；审计日志和运行日志中严禁打印任何密文或凭证内容。

### 部署会话 Cookie 约定

- **私有化部署默认应将 `app.session_domain` 留空。** 这样浏览器会按当前访问 host 绑定 `stx_session_id`，最适合 IP、内网域名和自定义域名混用场景。
- **不要在示例配置里把 `app.session_domain` 固定写成 `localhost`。** 若实际通过 `10.x.x.x`、`192.168.x.x` 或企业域名访问，但 Cookie domain 仍是 `localhost`，常见现象是：
  - `POST /api/v1/auth/login` 返回 200
  - 浏览器未正确回传会话 Cookie
  - 后续 `GET /api/v1/auth/user-info` 及其他受保护接口全部返回 `401`
- **只有在访问入口固定为某个域名时才显式设置 `app.session_domain`**，例如 `stx.company.com`。
- **`app.session_secure` 仅在浏览器始终通过 HTTPS 访问时设为 `true`。** 若仍通过 HTTP 调试或内网明文访问，设为 `true` 会导致浏览器不发送 Cookie。

#### Good / Base / Bad

- **Good**
  - `app.session_domain: ""`
  - 部署后通过 `http://10.0.0.5:17800` 或 `https://stx.company.com` 访问
  - 登录成功后 `/api/v1/auth/user-info` 返回 200
- **Base**
  - `app.session_domain: "stx.company.com"`
  - 确认所有用户都只通过该域名访问
- **Bad**
  - `app.session_domain: "localhost"`
  - 实际通过 IP / 企业域名访问
  - 现象：登录成功但后续接口持续 `401`

---

## 禁止模式

- **全局可变状态**：不得用全局变量持有 service 或 DB，应通过构造函数注入（如 `NewHandler(service, auditRepo)`）。仅允许的全局实例为 `internal/db` 的 DB 与 `internal/logger` 的 logger。
- **省略 context**：不得新增省略首参 `context.Context` 的 repository 或 service 方法，否则会破坏链路与取消传播。
- **在 API 中直接暴露 gorm.ErrRecordNotFound**：不得向 handler 返回 `gorm.ErrRecordNotFound`，应在 repository 中映射为领域错误（如 `ErrClusterNotFound`），再在 handler 中映射为 HTTP 状态码。
- **记录敏感信息**：不得在日志中输出密码、令牌、API 密钥或任何用于认证的配置，也不得记录可能包含凭据的请求/响应体。
- **忽略错误**：不得对影响正确性的错误使用 `_ = ...`，要么处理错误，要么在注释中说明为何可以忽略（如尽力而为的清理）。

---

## 必须遵循的模式

- **Context 传递**：从 HTTP handler 经 service 到 repository 一路传递 `ctx`；每次 DB 调用使用 `r.db.WithContext(ctx)`（或事务）。
- **领域失败用哨兵错误**：将包级哨兵错误集中定义在 `err.go`、`errors.go` 或所属领域文件中；handler 使用状态码辅助函数或等价判断返回一致的 HTTP 状态码。
- **响应结构**：统一使用 `ErrorMsg` 表示错误、`Data` 表示成功载荷，并显式设置 HTTP 状态码（400、404、409、500）。
- **新功能代码**：新功能放在 `internal/apps/<name>/` 下，按需包含 handler、service、repository、model、err；在 `internal/router/router.go` 中注册路由与依赖。
- **迁移**：新增持久化实体时，将对应 model 加入 `internal/db/migrator/migrator.go` 的 `AutoMigrate` 列表。

---

## 测试要求

- **单元测试**：使用同包下的 `*_test.go`。多场景优先用表驱动测试。在可行时 mock 外部依赖（如 DB）；部分测试使用真实 SQLite（如 `cluster/repository_test.go`、`audit/repository_test.go`）。
- **覆盖重点**：以 repository 和 service 逻辑为主；handler 常通过间接方式覆盖。关键路径（如重名、未找到）应有测试。
- **命名**：测试函数名为 `TestXxx`；子测试用 `t.Run("描述", ...)`。示例：`TestRepository_Create_clusterNameDuplicate_returnsErrClusterNameDuplicate`。
- **DB 测试**：使用真实 DB 时，避免依赖共享全局状态；尽量每个测试使用独立 DB 或事务（参见现有 `repository_test.go` 写法）。
- **Java Proxy**：测试目录必须采用与主代码相同的功能包结构；提交前运行 Maven Spotless 和相关 JUnit 测试。若完整测试依赖本机未安装的 SeaTunnel 连接器，须记录缺失夹具，并单独运行不依赖这些夹具的测试。

---

## 代码评审检查项

- 新接口使用统一的请求/响应与错误模式（`ErrorMsg`、`Data`、`getStatusCodeForError`）。
- 所有新 DB 访问均使用 `WithContext(ctx)`。
- 新领域错误已加入集中定义位置，并在 handler 的状态码映射中处理。
- 日志中无敏感信息与 PII。
- 新表/新 model 已加入 migrator 的 `AutoMigrate` 列表。
- 在可行处为新 repository 与 service 行为补充测试。
- 不为 service 或 repository 使用全局变量；依赖在 `router.go` 或 `cmd` 中组装。
