# 后端开发规范

> 适用于根 Go 模块、`agent/` Go 模块和 `tools/stx-java-proxy/`。

---

## 概述

本目录包含后端开发相关规范。各文件内填写的是本项目实际采用的约定。

---

## 规范索引

| 文档 | 说明 | 状态 |
|------|------|------|
| [目录结构](./directory-structure.md) | 模块划分与文件布局 | 已填写 |
| [数据库规范](./database-guidelines.md) | ORM、查询、迁移 | 已填写 |
| [错误处理](./error-handling.md) | 错误类型与处理策略 | 已填写 |
| [质量规范](./quality-guidelines.md) | 代码标准、注释规范、License 头、语言输出约定、禁止模式 | 已填写 |
| [日志规范](./logging-guidelines.md) | 结构化日志与日志级别 | 已填写 |

---

## 开发前检查

- 先确认改动属于根 Go 模块、Agent 还是 Java Proxy，并阅读对应的[目录结构](./directory-structure.md)。
- 涉及数据库、HTTP 错误或日志时，分别阅读数据库、错误处理和日志规范。
- 新增源文件、方法或用户可见文案时，按[质量规范](./quality-guidelines.md)检查注释、License 头和语言输出。

## 质量检查

- 根 Go 模块运行 `go test ./...`，Agent 模块运行 `(cd agent && go test ./...)`。
- Java Proxy 运行 `mvn -f tools/stx-java-proxy/pom.xml test`；若测试依赖外部夹具，需同时记录已通过的独立测试与缺失夹具。
- 提交前运行与改动对应的格式化、静态检查和 License 检查。

---

**语言**：本目录下所有文档均使用**中文**。
