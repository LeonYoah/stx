# 连接器与作业配置模板

## Goal

在数据同步工作台提供统一的**模板**能力，并明确两类来源的关系：

- **精选模板**：高度定制、贴近生产实践的推荐配置（平台内置精选 ∪ 用户自定义）。
- **原始默认参数模板**：连接器官方/SPI 给出的全部参数清单，带默认值或空值，用于对照与起步填空。

产品叙事是「两类模板、一条调优闭环」；实现上仍区分「可持久化内容」与「按集群即时生成」，但不对用户强调「库 vs 工具」行话。

## Confirmed Facts（仓库已证实）

- 现有能力（`03-28-sync-plugin-template-schema`）：侧栏「配置模板」三下拉 → `POST /api/v1/sync/plugins/template` → SPI 全量选项骨架（空值/官方默认，禁止示例伪值）→ `buildInsertedTemplateContent` 插入。此即「原始默认参数模板」的技术底座，**保留能力、改名并纳入统一模板体系**。
- 工作台已有树右键、Monaco、`{{key}}` 变量、官方文档入口。
- Dinky：Document/`fill_value` 可复用片段 + CRUD；偏「精选/片段」一侧。
- 官方全集 ≠ 生产最优；多表/CDC/批流差异大。
- 持久化：GORM `AutoMigrate`（`internal/db/migrator`）；插件类种子先例：`internal/apps/plugin/seed/seatunnel-plugins.json`。

## Requirements

### R0 关系模型

```
原始默认参数模板（全集 + 默认值，随集群插件版本即时生成）
        │  删减、改值、加 {{变量}}、组批/流/多表场景
        ▼
精选模板（高度定制 + 生产实践推荐）
        │  平台内置精选  ∪  我的自定义
        ▼
复用精选；需要更多参数时切到「原始默认参数」Tab 插入后继续调优 → 另存回精选
```

| | 精选模板 | 原始默认参数模板 |
|---|---|---|
| 对用户 | 一类模板（推荐直接用） | 一类模板（对照/填空） |
| 内容 | 少而准，有场景语义 | 全量 key，默认值/空值 |
| 来源 | 种子 + 用户持久化 | 集群 + java-proxy，**默认不落库** |
| 实现角色 | 可持久化「内容」 | 即时生成「生成器」 |

### R1 精选模板粒度（已调整：块级自由组合优先）

精选以内置 **env / source / transform / sink 片段** 为主，用户自由组合；插入行为对齐各节（env 整段替换或合并；source/transform/sink 复用 `buildInsertedTemplateContent`）。

| section | 含义 |
|---|---|
| `env` | 运行环境（BATCH / STREAMING） |
| `source` | 单插件源端精选 |
| `transform` | 单插件转换精选（可多段拼接） |
| `sink` | 单插件目标端精选 |

**一键生成组合模板**（另存 / 从当前编辑器制作）：解析当前 HOCON 时须覆盖 **env、source、transform、sink** 四节——可整份存为「组合精选」，或按节拆成多条片段；缺节不报错，有节必纳入，禁止只抽 source/sink 而丢掉 env/transform。

可选：后续再加「组合菜谱」元数据（推荐哪几个片段拼在一起）；一期以块种子 + 一键组合为主。

**变量策略**：仅密码/密钥类凭证使用 `{{...}}`（如 `jdbc_password`、`cdc_password`、`ftp_password`、`s3_access_key`、`s3_secret_key`）；主机、库表、路径、用户名等写死可改示例值。

### R2 内置精选范围（一期，块级）

**env**：batch、streaming  

**source**：FakeSource；Jdbc 单表/多表；MySQL-CDC 单表/多表；Kafka；LocalFile；**S3File**；**HdfsFile**；**FtpFile**  

**transform**：至少 1 条常用示例（如 Copy / FieldMapper），保证一键组合与 Tab 分类有 transform 位  

**sink**：Console；Jdbc 单表；Jdbc 多表路由；Kafka；Hive **基础 / Kerberos / S3**  

覆盖原场景组合（冒烟、JDBC↔JDBC、CDC→JDBC/Kafka、Kafka→JDBC、文件→JDBC、JDBC→Hive、JDBC→Console）均由上述块拼出。

### R3 精选：自定义、改内置与存储（已拍板 A）

- **存储**：仓库**种子文件**发版内置；**DB** 存「我的」及对内置的 copy-on-write 副本。升级刷新种子，不覆盖用户 DB。
- 内置只读；编辑内置 → DB 副本（可关联 `source_builtin_id`）。
- 维度：`mode`、`pattern`、`connectors`、`enabled`、可选版本字段。

### R4 沉淀为精选 / 一键组合

统一弹窗（名称 / section 或「整份组合」/ mode / 描述 / 正文可编辑）：

- 选区、右键文件、当前编辑器 →「另存为精选模板」
- **一键生成组合模板**：从当前全文识别并纳入 **env + source + transform + sink**（有则带上，无则跳过）；默认建议存为整份组合，也允许拆成按节多条
- 从原始默认参数改完后同一另存；**禁止**原始默认自动入库

### R5 统一模板交互（已拍板 Tab）

```
模板
├── Tab「精选模板」           ← 默认（内置精选 | 我的）
└── Tab「原始默认参数模板」   ← 原三下拉迁入并改名
```

一期**不做**块内「对照补全」（不在已有插件块上智能追加官方参数）。需要更多参数时：切到「原始默认参数」Tab 选插件插入。

### R6 命名（已拍板）

| 角色 | 中文名 |
|---|---|
| 主入口 | 模板 |
| 实践推荐 | 精选模板 |
| SPI 全集 | 原始默认参数模板 |
| 另存 | 另存为精选模板 |

### R7 权限与安全

- 精选 CRUD 走 sync 权限；共享禁用明文密钥，引导 `{{var}}`。

## Acceptance Criteria

- [ ] 精选：种子文件 + DB（我的/副本）；升级不覆盖用户数据。
- [ ] 「模板」两 Tab：精选（默认）、原始默认参数；原「配置模板」文案废弃并迁入后者。
- [ ] 精选以 env/source/**transform**/sink 块级片段为主，可自由组合；仅凭证类走 `{{}}`。
- [ ] 一键生成组合模板时覆盖当前作业中存在的 env、source、transform、sink 四节。
- [ ] 内置覆盖：文件 Local/S3/Hdfs/Ftp；Hive 基础/Kerberos/S3；Jdbc/CDC/Kafka/Fake/Console；至少 1 条 transform 示例。
- [ ] 用户 CRUD 我的精选；改内置仅副本。
- [ ] 选区 / 右键 / 当前内容可另存为精选，弹窗可编辑。
- [ ] 原始默认参数 API 行为保留，不自动入库；一期无块内对照补全。
- [ ] 盲测：生产实践 → 精选；全参数/默认值 → 原始默认参数。

## Out of Scope（一期）

- **块内对照补全**（已有插件块上追加/合并官方参数）——二期再做。
- 官方 vs 精选逐 key diff；插件升级提示新增参数。
- 模板市场；拖拽编排。
- Postgres-CDC / Oracle / Paimon / ClickHouse 等扩展种子。

## Decisions Log

| 项 | 结论 |
|---|---|
| 产品关系 | 两类模板 + 调优闭环（精选 = 实践结果；原始默认 = 全集起点） |
| 叫法 | 精选模板 / 原始默认参数模板 / 主入口「模板」 |
| UI | 同一入口 **Tab**，默认精选 |
| 存储 | **A**：仓库种子 + DB 用户/副本 |
| 对照补全 | **一期不做** |
| 精选形态 | **env/source/transform/sink 分块可组合** |
| 一键组合 | 解析并纳入四节（有则必带） |
| 变量策略 | **仅密码/密钥凭证**走 `{{}}`，其余示例字面量 |
| Hive | 基础 + Kerberos + S3 变体 |
| 文件 Source | LocalFile + S3File + HdfsFile + FtpFile |
| 精选草稿 | **v2 已审定**（`curated-draft/`） |
| plugin_IO | **≥2.3.9** 用 `plugin_*`；`<2.3.9` 插入时改写为 `source_table_name` / `result_table_name` |

## Notes

- 分支：`features/connector-template`；PR 目标：`main`。
- 规划通过后写 `design.md` + `implement.md`，再 `task.py start`。
