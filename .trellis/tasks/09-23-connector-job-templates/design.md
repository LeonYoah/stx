# 设计：连接器与作业配置模板

## 1. 边界

| 层 | 职责 |
|---|---|
| 前端 Studio | 「模板」侧栏两 Tab；精选浏览/插入/另存；原始默认参数沿用现有插入；右键/选区另存弹窗 |
| Go `internal/apps/sync` | 精选模板 CRUD、列表合并（种子 ∪ DB）、copy-on-write、权限与审计挂钩 |
| 种子资源 | 仓库内 JSON/YAML（建议 `internal/apps/sync/seed/curated-templates/`）只读内置 |
| java-proxy | **不改** `/api/v1/plugin/template` 语义；仍服务「原始默认参数模板」 |
| 既有插件模板 API | `POST /api/v1/sync/plugins/template` 保留；前端改文案与 Tab 归属 |

**非目标（一期）**：块内对照补全、跨租户市场、按 ST 小版本多套种子矩阵。

## 2. 概念与数据流

精选种子以 **env / source / transform / sink 块** 存储与展示；用户自由组合。`plugin_input` 需与上游 `plugin_output` 对齐（模板注释提示）。

```
[种子: env|source|transform|sink] ──只读──┐
                                          ├──► ListCurated ──► UI 精选 Tab（按 section 分组）
[DB curated_templates]────────────────────┘
                                          │ 分别插入对应节
                                          ▼
                                     Monaco 编辑器
                                          │ 一键组合另存（扫描 env/source/transform/sink）
                                          ▼
                                     Create/Update DB 行

[集群插件] ──► 原始默认参数 Tab ──► RenderPluginTemplate ──► 插入
```

**一键组合**：从编辑器全文解析四个顶层节；存在的节全部进入另存内容（或拆条）；不得只保留 source/sink。

**变量策略**：种子正文仅对 password / access_key / secret_key 等凭证使用 `{{...}}`；其余为可改字面量示例。

**版本兼容（硬性）**：`plugin_input` / `plugin_output` 自 SeaTunnel **2.3.9**（#8072）起替代 `source_table_name` / `result_table_name`。精选种子默认写 `plugin_*`；**插入到 `< 2.3.9` 集群时必须改写为旧键**（复用 `rewritePluginIOKeysForLegacy` 同类逻辑）。低版本集群不得原样下发 `plugin_*`。

## 3. 数据模型（精选）

建议表名：`sync_curated_templates`（GORM model 如 `CuratedTemplate`，落入 sync AutoMigrate）。

| 字段 | 说明 |
|---|---|
| id | PK |
| builtin_id | 可空；用户副本指向种子 id（如 `jdbc-mysql-to-hive-batch`） |
| owner_user_id | 空=系统不可写；非空=我的/副本 |
| name / description | 展示 |
| grain | `job` \| `fragment` |
| plugin_type | fragment 时：`source`/`transform`/`sink`；job 可空 |
| mode | `BATCH` \| `STREAMING` \| `ANY` |
| pattern | `single` \| `multi` \| `cdc` \| `other` |
| connectors | JSON 字符串数组（如 `["Jdbc","Hive"]`） |
| content | HOCON 正文 |
| enabled | bool |
| source | `builtin` 仅存在于种子视图，不入库；DB 行为 `user` \| `override` |
| created_at / updated_at | |

**列表合并规则**：

1. 加载全部种子为「内置」。
2. 加载当前用户 DB 行。
3. 若存在 `builtin_id=X` 的 override，则内置列表中 X 标记为「已有副本」，「编辑内置」打开的是副本；删除副本可恢复见内置（可选一期：仅隐藏删除、保留行）。

种子文件示例结构：

```json
{
  "id": "jdbc-to-hive-batch",
  "name": "JDBC → Hive（批）",
  "grain": "job",
  "mode": "BATCH",
  "pattern": "single",
  "connectors": ["Jdbc", "Hive"],
  "content": "env { ... }\nsource { ... }\nsink { ... }\n"
}
```

## 4. API 契约（精选，草案）

均挂在 sync 路由、走现有 auth / operation 注册风格：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/api/v1/sync/curated-templates/list` | 合并内置+我的；支持 grain/mode/q 筛选 |
| POST | `/api/v1/sync/curated-templates` | 新建我的（含另存） |
| PUT | `/api/v1/sync/curated-templates/:id` | 更新我的/副本 |
| DELETE | `/api/v1/sync/curated-templates/:id` | 删我的/副本 |
| POST | `/api/v1/sync/curated-templates/fork` | 从 builtin_id fork 副本（编辑内置入口） |

原始默认参数：继续 `POST /api/v1/sync/plugins/template`，无新表。

响应包络保持 `{ error_msg, data }`。

## 5. 前端信息架构

- `SettingsSidebarPanel`：原「配置模板」区块改为「模板」容器 + Tabs。
  - Tab1 精选：列表（内置/我的分段或过滤）、插入、管理入口、「另存当前」。
  - Tab2 原始默认参数：现有 Source/Transform/Sink 三下拉 + 改名后的 hint。
- i18n：`pluginTemplates` → 拆成 `templates` / `curatedTemplates` / `rawDefaultParamTemplates`（中英同步）。
- `StudioTreeContextMenu` + 编辑器命令：另存为精选（打开共用 Dialog）。
- 另存 Dialog：预填 content（选区优先，否则全文）、grain 推断（有完整 env+source/sink 偏 job，否则 fragment）、允许改。

## 6. 兼容与迁移

- 旧文案「配置模板」仅 i18n/UI 变更，后端 plugins/template 路径不变，CLI `stx sync plugin template` 保留（帮助文案可后续改「原始默认参数」）。
- 新表 AutoMigrate 注册；无破坏性变更。
- 种子随发版覆盖文件；运行时不写回种子。

## 7. 安全

- content 不在列表接口强制脱敏；另存 UI 提示密钥用 `{{var}}`。
- 仅 owner 可改删自己的行；内置不可 DELETE。
- 写入走既有 sync 写权限 / operation risk 登记。

## 8. 回滚

- 功能开关（可选）：配置项关闭精选 Tab，仅显示原始默认参数（降级到改名后的旧体验）。
- 或发版回退：保留 DB 表无害；前端隐藏精选 Tab。

## 9. 风险与权衡

| 风险 | 缓解 |
|---|---|
| 精选与原始默认仍被当成两套互斥功能 | Tab 同入口 + 文案强调闭环 |
| 种子 HOCON 与真实 ST 版本漂移 | 种子注明适用版本；文档链到官方；一期不强制校验 |
| job 误替换编辑器 | 强制确认弹窗 |
| fragment 缺 plugin_type | 另存时必填或从 HOCON 启发式推断失败则要求手选 |
