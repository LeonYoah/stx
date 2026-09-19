# Discovery CLI 与遗留路由登记

## 目标

继续现有 API 的 CLI 覆盖，为当前真实可用的主机进程发现接口提供命令，并明确登记另外两条尚不可用的 discovery 路由，避免 AI 调用后得到误导结果。

## 范围

本批公开：

- `stx host discovery process list <host-id>`
- 对应 `POST /api/v1/hosts/:id/discover-processes`

本批不公开：

- `POST /api/v1/hosts/:id/discover`：当前执行 Agent 命令后丢弃结果，固定返回空集群。
- `POST /api/v1/hosts/:id/discover/confirm`：当前没有注入 `ClusterMatcher`，确认导入必然失败。

以上两条路由必须加入带原因的例外登记，后续修复服务端流程后再改为正式操作。

## 要求

- 普通命令构建器只增加无请求体的 R0 POST 支持，不允许任意写操作通过该入口生成。
- discovery 进程发现操作必须标记 `UsesAgent: true`，并说明它只读取目标主机进程信息，不停止或修改进程。
- 命令继续执行 capability、mode 和 revision 检查。
- 命令支持命名空间、JSON、YAML、table、raw 和 `--pick`。
- 帮助阶段不得访问服务端。
- 真实验证必须使用本机在线 Agent 和主机 ID `10`，确认能返回 SeaTunnel 进程信息。

## 验收标准

- [x] 操作登记增加 `host.discovery.process.list`，登记操作从 25 增至 26。
- [x] 两条不可用 discovery 路由加入例外登记，例外从 19 增至 21。
- [x] 历史缺口从 202 减少至 199。
- [x] 通用命令构建器拒绝 GET 和无请求体 R0 POST 之外的普通命令。
- [x] 离线帮助能显示命令、输出样例和影响说明。
- [x] 最新 `dist/stx` 连接本机服务，真实返回至少一个 SeaTunnel 进程。
- [x] JSON、table 和 `--pick` 经过真实验证。
- [x] 不存在主机和 Agent 未安装等错误保持稳定退出行为。
- [x] 目标包测试、全量测试、静态检查、路由契约检查和构建通过。
