# 数据同步工作台 CLI 全面覆盖与端到端真实作业验收 PRD

## 1. 目标与背景

根据 `09-13-stx-cli-agent-entry` 核心架构思想，SeaTunnelX CLI（`stx`）不仅是控制台的字符外壳，更是面向自动化运维与自主 AI Agent 的核心交互契约。
此前 STX 已覆盖集群、主机、配置、诊断、包管理、监控等模块的 CLI，但平台最核心的业务模块——**数据同步工作台（Workbench / DataSyncStudio）** 尚未接入 `stx` CLI 命令树，其对应的 25 个业务路由在操作登记表中均处于历史缺口状态。

本任务目标：
1. **全面设计并接入数据同步工作台的所有能力到 `stx` CLI**：
   - **工作区与任务编排**：目录树（`tree`）、任务列表与详情（`list`/`get`）、创建（`create`）、更新（`update`）、删除（`delete`）、发布（`publish`）、版本历史与回滚（`versions`/`rollback`）。
   - **DAG、校验与连接测试**：拓扑流向与节点解析（`dag`）、配置合法性校验（`validate`）、多数据源连通性测试（`test-connections`）。
   - **预览与评估**：数据抽样预览（`preview`）、Sink 端 SaveMode DDL 预演（`preview-savemode`）、预览快照获取。
   - **作业提交与全生命周期管理**：作业提交（`submit`）、作业实例查询（`list`/`get`）、实时运行日志（`logs`）、作业停止（`cancel`，支持 `--savepoint` 触发保存点）。
   - **Checkpoint 与 Savepoint 容灾恢复**：Checkpoint 状态与指标快照查询（`checkpoint`）、基于 Savepoint / Checkpoint 的断点恢复重跑（`recover`）。
   - **元数据与辅助系统**：全局变量管理（`variable`）、插件工厂与 Schema 模版（`plugin`）。
2. **严格遵循 `09-13-stx-cli-agent-entry` 安全与架构规范**：
   - 风险分级（R0 纯只读免确认，R1 低风险写操作 `--confirm` + 自动生成幂等键，R2/R3 高风险运行与删除二次确认）。
   - 统一异步任务执行抽象（返回全局 `execution_id`，打通 `stx execution wait/get/cancel`）。
   - 审计自动绑定（每个操作记录调用源、用户、关联 Execution 与操作轨迹）。
   - 输出约定：标准纯净单个 JSON 写入 stdout，结构化 NDJSON 事件（进度、提示）写入 stderr。
   - 离线帮助：`stx sync --help` 及所有子命令在离线/无网络环境下正常工作。
3. **真实端到端环境验收**：
   - 启动本地 MySQL 容器与测试数据库/表（开启 binlog）。
   - 配置并提交真实 **MySQL CDC -> Console/MySQL** 以及 **JDBC -> MySQL/Console** 任务。
   - 验证：从创建任务、生成 DAG、连通性检查、预览、提交作业到获取运行日志、查询 Checkpoint、触发 Savepoint 停止、最后从 Savepoint 成功恢复的全链路真实验收。

---

## 2. 核心功能契约设计

### 2.1 命令架构划分

所有数据同步工作台能力收敛在 `stx sync` 命令组下：

```text
stx sync
  ├── task                      # 任务管理与编排
  │   ├── tree                  # 工作区目录树
  │   ├── list                  # 任务列表查询
  │   ├── get <id>              # 任务详情
  │   ├── create                # 创建同步任务/目录
  │   ├── update <id>           # 更新任务元数据或配置
  │   ├── delete <id>           # 删除任务 (R2)
  │   ├── publish <id>          # 发布为正式版本 (R1)
  │   ├── validate <id>         # 校验任务配置
  │   ├── test-connections <id> # 测试数据源连通性 (R0)
  │   ├── dag <id>              # 生成/解析任务 DAG 拓扑 (R0)
  │   ├── preview <id>          # 启动数据预览采样 (R1/Async)
  │   ├── preview-savemode <id> # 预览 Sink 端 SaveMode (R0)
  │   ├── submit <id>           # 提交任务运行 (R2/Async)
  │   └── version               # 任务版本控制
  │       ├── list <task_id>    # 历史版本列表
  │       ├── rollback <task_id> <version_id> # 回滚版本 (R1)
  │       └── delete <task_id> <version_id>   # 删除版本 (R2)
  ├── job                       # SeaTunnel 作业运行与监控
  │   ├── list                  # 作业列表 (支持 task_id/status 过滤)
  │   ├── get <id>              # 作业运行详情
  │   ├── logs <id>             # 获取作业运行日志 (--lines, --all)
  │   ├── checkpoint <id>       # 查询 Checkpoint 快照与指标
  │   ├── preview <id>          # 查询关联预览快照数据
  │   ├── cancel <id>           # 取消作业 (R2, --savepoint 支持保存点停止)
  │   └── recover <id>          # 从 Savepoint/Checkpoint 恢复重跑 (R2/Async)
  ├── variable                  # 全局变量管理
  │   ├── list                  # 变量列表
  │   ├── create                # 新增全局变量
  │   ├── update <id>           # 修改全局变量
  │   └── delete <id>           # 删除全局变量
  └── plugin                    # 插件工厂与模版元数据
      ├── list                  # 支持的插件列表 (source/transform/sink)
      ├── options <name>        # 插件 Option 配置 Schema
      ├── template <name>       # 插件模版配置
      ├── enum-values           # 获取动态枚举值
      └── enum-catalog          # 获取枚举目录
```

---

## 3. 风险与安全分级 (Aligned with 09-13 Contract)

| 操作 | 风险等级 | 说明与安全约束 |
| :--- | :---: | :--- |
| `sync task tree/list/get` | **R0** | 纯读操作，直接输出 JSON |
| `sync task dag/validate/preview-savemode` | **R0** | 逻辑静态解析与规则校验，不产生数据变更 |
| `sync task test-connections` | **R0** | 探测远端连通性，轻量只读检查 |
| `sync task create/update/publish` | **R1** | 写入任务配置或产生新版本，需 `--confirm` 与幂等键 |
| `sync task preview` | **R1** | 启动轻量数据采样会话，关联 Execution，超时自动清理 |
| `sync task delete` | **R2** | 删除任务配置，需强确认 `--confirm` |
| `sync task submit` | **R2** | 提交分布式计算作业，占用集群资源，关联 Execution，支持 `--wait` |
| `sync job list/get/logs/checkpoint/preview` | **R0** | 查询作业运行状态、日志、Checkpoint 快照 |
| `sync job cancel` | **R2** | 终止正在运行的分布式作业，支持 `--savepoint` 保存状态 |
| `sync job recover` | **R2** | 从 Checkpoint/Savepoint 恢复启动作业，关联 Execution |
| `sync variable create/update/delete` | **R1** | 修改作业全局变量 |

---

## 4. 验收标准

1. **契约与命令完整性**：
   - 所有 25 个 `sync` 业务 API 均在 `internal/operation/registry.go` 中完成正式登记。
   - `route_baseline.json` 中的历史缺口全部清空归零，`Validate(Registry(), RouteExceptions())` 单元测试通过。
   - `stx sync --help` 及其所有子命令均可离线执行并提供中英文帮助。
2. **安全与执行模型对齐**：
   - 写操作（`create`, `update`, `delete`, `submit`, `cancel`, `recover`, `publish` 等）均实现安全保护机制（`--confirm` 与 `Idempotency-Key`）。
   - 异步写操作（`submit`, `recover`, `preview`）均返回全局 `execution_id`，可通过 `stx execution get/wait` 跟踪。
3. **端到端真实验收**：
   - 本地拉起 MySQL 容器，创建源库表 `source_cdc` 与目标库表 `sink_mysql`。
   - 通过 CLI 创建同步任务并配置 DAG（支持 MySQL CDC -> MySQL 以及 JDBC -> MySQL/Console）。
   - 运行 `stx sync task validate` 与 `stx sync task dag` 验证拓扑。
   - 运行 `stx sync task preview` 验证数据采样。
   - 运行 `stx sync task submit` 提交到本地 SeaTunnel 集群运行，并通过 `stx execution wait` 等待就绪。
   - 通过 `stx sync job logs` 查看作业运行日志。
   - 通过 `stx sync job checkpoint` 查看 Checkpoint 成功指标。
   - 通过 `stx sync job cancel --savepoint` 触发保存点并停止作业。
   - 通过 `stx sync job recover` 从保存点恢复作业并继续同步，验证数据一致性。
