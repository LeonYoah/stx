# sync 工作台插件模板与配置 schema 联动

## Goal

在 `/sync` 数据同步工作台中，基于当前选中集群的已安装插件，为用户提供 source / transform / sink 模板选择器；选中插件后，由 `stx-java-proxy` 返回带注释的 HOCON 模板片段并追加到编辑器底部；编辑器再结合插件 schema 与新枚举值接口，为命中的配置 key 提供候选值联动，降低手写配置门槛并提升配置正确率。

## What I already know

- 当前 `/sync` 已支持任务编辑、validate、DAG、preview 等能力。
- `stx-java-proxy` 已支持 config DAG、preview、catalog、checkpoint、imap 等接口。
- Source / Sink 插件与集群安装状态强相关，应优先基于当前集群已安装插件联动。
- Transform 插件不依赖集群额外安装，主要来自 `seatunnel-transforms-v2.jar`，更适合由 proxy 运行时直接发现。
- proxy 已具备请求级 `pluginJars` 动态加载能力，但 config 相关能力更多依赖 `SEATUNNEL_HOME` 扫描。
- 当前已有“已知插件枚举值”能力，但本次要求编辑器联动改走新接口，而不是继续复用旧接口。
- 仅依赖 `Factory.optionRule()` 不够，需要两层方案：
  - 第一层：`optionRule`
  - 第二层：`Option` 常量补扫
- 不做第三层源码 `config.get(...)` 扫描。

## Requirements

- `/sync` 页面右侧在“集群选择”下新增“配置模板”区域。
- 配置模板区域包含三个下拉框：
  - Source
  - Transform
  - Sink
- Source / Sink 下拉框内容需基于当前选中集群的已安装插件联动。
- Transform 下拉框内容需由 proxy 从运行时可见的 transform factory 动态发现，不要求平台维护静态清单。
- 用户选中某个 source / transform / sink 后，可将对应插件的 HOCON 模板片段追加到当前编辑器底部。
- 模板片段需支持 HOCON 注释，并尽量输出配置项描述，帮助用户理解字段含义。
- 模板值只输出：
  - 空值
  - 官方默认值
- 模板不内置额外“示例伪值”或平台自定义默认值，避免误导用户。
- 模板默认优先输出基础配置；高级补充项可后续通过 schema 扩展支持，但第一期至少要区分“主规则项”和“补扫项”。
- 前端需提供“高级模式”开关：
  - 默认模式只展示平台记录的已安装 Source / Sink 插件
  - 高级模式允许同时展示 proxy 运行时发现但平台未建档的插件
- 编辑器需要与插件 schema / 枚举值能力联动：
  - 当命中已知配置 key 时，可展示枚举值候选
  - 候选值需与当前 block 所属插件相匹配
  - 枚举联动改走新接口，不复用旧接口
- 新增 proxy 插件 schema 能力时，需支持：
  - source
  - sink
  - transform
  - catalog
- 第一层主数据源使用官方 `optionRule`
- 第二层仅做 `Option` 常量类补扫，不做源码扫描
- 对于 `catalog` / `optionRule()` 为空或不完整的场景，系统仍需返回可用 schema，并在 warnings 中提示
- 第二层补扫失败时，不能影响第一层主结果返回
- 模板追加成功后，编辑器需要自动滚动、聚焦并直接选中新插入 block，方便用户继续编辑

## Performance Requirements

- 该能力只允许用于 Studio 管理 / 编辑辅助路径，不能引入到作业提交热路径。
- 插件 schema 抽取必须加缓存，避免每次打开下拉框或每次编辑器联动都重新扫 jar。
- 至少要有两级缓存：
  - `schemaCache`
  - `fieldScanCache`
- 缓存 key 需包含 classpath / code source fingerprint，而不是只按插件名缓存，避免插件升级后拿到陈旧 schema。
- fingerprint 不要求做 SHA，允许使用：
  - 绝对路径
  - 文件大小
  - 最后修改时间
- Transform 列表应支持缓存，避免每次切换集群或切换高级模式都重新完整扫描 transforms jar。
- Source / Sink 下拉框优先使用平台已有“集群安装插件”数据，不依赖每次都让 proxy 全量扫描运行时。
- 只有在高级模式下，才补充展示 proxy 运行时发现但平台未建档的插件；这一补充结果也必须走缓存。
- 编辑器枚举值联动必须是轻量查询：
  - 优先走已缓存 schema
  - 不允许在每次光标移动时触发完整 schema 扫描
  - 不允许在每次输入时重复发起重型网络请求
- 冷启动单次 schema 解析允许为“几十到几百毫秒”量级，但命中缓存后应接近毫秒级返回。
- 前端模板追加和枚举联动不能造成编辑器明显卡顿。
- 自动滚动到新插入 block 的实现不能触发整份文档的重建或重复格式化，避免大文本场景下编辑器闪烁或卡顿。

## Acceptance Criteria

- [ ] `/sync` 页面右侧新增“配置模板”区域，位于集群选择下方。
- [ ] Source / Sink 下拉框会随当前集群已安装插件联动变化。
- [ ] Transform 下拉框可列出 proxy 运行时发现到的 transform 插件。
- [ ] 前端存在“高级模式”开关；开启后可额外展示 proxy 运行时发现但平台未建档的 Source / Sink 插件。
- [ ] 用户选择 source / transform / sink 后，可以将对应带注释的 HOCON 模板片段追加到编辑器底部，而不是覆盖当前内容。
- [ ] 追加完成后，编辑器会自动滚动、聚焦并直接选中新插入 block。
- [ ] 模板值仅输出空值或官方默认值，不输出平台杜撰示例值。
- [ ] proxy 新增插件 schema 接口，能返回统一的 `PluginOptionDescriptor` 列表。
- [ ] proxy 新增插件模板接口，能返回带注释的 HOCON 模板片段。
- [ ] proxy 新增插件枚举接口，编辑器命中已知 key 时走新接口获取候选值。
- [ ] schema 结果至少包含：
  - key
  - type
  - defaultValue
  - description
  - fallbackKeys
  - enumValues
  - requiredMode
  - origins
  - declaredClasses
- [ ] 第一层 `optionRule` 与第二层 `Option` 常量补扫可以正确合并。
- [ ] `catalog` / `optionRule()` 为空时仍能返回可用结果，并附带 warnings。
- [ ] schema 抽取具备缓存，重复请求不会反复全量扫 jar。
- [ ] 在至少一轮本地验证中，连续切换插件下拉、切换高级模式或重复命中同一插件 schema 时，响应时间明显优于首次冷启动请求。
- [ ] 至少完成一轮后端单测 / 定向验证，以及前端类型检查与手工联动验证。

## Out of Scope

- 不做第三层源码 `config.get(...)` 扫描。
- 不保证 100% 覆盖插件运行时会读取的所有配置项。
- 不还原 connector 私有运行时全部校验规则。
- 不在第一期实现复杂表达式的前端结构化渲染，只需先返回字符串形式的 `conditionExpression`。
- 不在第一期重做完整 Monaco/HOCON 语言服务，只做与现有编辑器能力兼容的模板追加和枚举联动。
- 不要求第一期自动去重或重排用户已有 block，只做“追加到末尾”。

## API / Contract Notes

### Proxy APIs

建议新增：

- `POST /api/v1/plugin/list`
- `POST /api/v1/plugin/options`
- `POST /api/v1/plugin/template`
- `POST /api/v1/plugin/enum-values`

#### `POST /api/v1/plugin/list`

用途：
- 返回某类型插件列表
- transform 下拉框直接使用
- source / sink 也可用于高级模式补充运行时发现结果

请求示例：

```json
{
  "pluginType": "transform",
  "pluginJars": []
}
```

响应示例：

```json
{
  "ok": true,
  "pluginType": "transform",
  "plugins": [
    {
      "factoryIdentifier": "Sql"
    }
  ],
  "warnings": []
}
```

#### `POST /api/v1/plugin/options`

用途：
- 返回统一插件配置 schema

请求示例：

```json
{
  "pluginType": "sink",
  "factoryIdentifier": "Doris",
  "pluginJars": [],
  "includeSupplement": true
}
```

#### `POST /api/v1/plugin/template`

用途：
- 返回带注释的 HOCON 模板片段，减少前端拼装逻辑

请求示例：

```json
{
  "pluginType": "source",
  "factoryIdentifier": "Jdbc",
  "pluginJars": [],
  "includeSupplement": true,
  "includeComments": true,
  "includeAdvanced": false
}
```

响应示例：

```json
{
  "ok": true,
  "pluginType": "source",
  "factoryIdentifier": "Jdbc",
  "contentFormat": "hocon",
  "template": "source { ... }",
  "warnings": []
}
```

#### `POST /api/v1/plugin/enum-values`

用途：
- 针对当前插件和配置 key 返回枚举候选值
- 编辑器命中 key 时统一走这个新接口

请求示例：

```json
{
  "pluginType": "sink",
  "factoryIdentifier": "Doris",
  "optionKey": "save_mode",
  "pluginJars": []
}
```

响应示例：

```json
{
  "ok": true,
  "pluginType": "sink",
  "factoryIdentifier": "Doris",
  "optionKey": "save_mode",
  "enumValues": ["APPEND", "OVERWRITE"],
  "warnings": []
}
```

### Platform APIs

平台侧建议补一层 sync studio 聚合接口，避免前端自行拼多个请求：

- 获取当前 cluster 可用 source / sink 插件列表
- 获取 transform 列表（可转调 proxy）
- 高级模式下获取 proxy 运行时发现但平台未建档的插件列表
- 获取指定插件模板
- 获取指定插件 schema
- 获取指定插件枚举候选值

## Schema Design Notes

建议模型：

- `PluginOptionSchemaResult`
- `PluginOptionDescriptor`
- `RequiredMode`
- `OptionOrigin`

### `RequiredMode`

- `REQUIRED`
- `OPTIONAL`
- `EXCLUSIVE`
- `CONDITIONAL`
- `BUNDLED`
- `UNKNOWN_NO_DEFAULT`
- `SUPPLEMENTAL_OPTIONAL`

### `OptionOrigin`

- `OPTION_RULE`
- `FIELD_SCAN`

### 合并规则

- 统一以 `Option.key()` 为主键
- 先第一层，后第二层
- 第一层优先保留 `requiredMode`
- 第二层只补：
  - description
  - defaultValue
  - fallbackKeys
  - enumValues
  - declaredClasses
- 仅第二层命中的项：
  - `advanced = true`
  - `defaultValue == null -> UNKNOWN_NO_DEFAULT`
  - `defaultValue != null -> SUPPLEMENTAL_OPTIONAL`

## Frontend Notes

- 配置模板区域建议放在 `/sync` 右侧辅助面板，而不是主编辑区上方，避免抢占编辑空间。
- 插件模板插入行为必须是 append，不做 replace。
- 追加时建议自动补两个换行，避免与已有 HOCON 黏连。
- 追加完成后需自动滚动、聚焦并直接选中新插入 block。
- 前端默认展示基础配置；高级配置可作为后续迭代能力。
- Source / Sink 默认只显示平台已建档插件；高级模式下再合并显示 proxy 运行时发现但未建档的插件。
- 编辑器枚举值联动时，前端需先识别当前 block 所属：
  - source / transform / sink
  - factoryIdentifier
  - option key
- 命中 key 后优先从本地缓存 schema 中提取 `enumValues`，仅在缺失时再请求新枚举接口。

## Technical Notes

- 相关前端文件大概率涉及：
  - `frontend/components/common/sync/*`
  - `frontend/lib/services/sync/*`
- 相关后端文件大概率涉及：
  - `internal/apps/sync/*`
- 相关 proxy 文件建议新增或修改：
  - `tools/stx-java-proxy/src/main/java/.../service/plugin/PluginOptionSchemaService.java`
  - `.../FactoryOptionRuleExtractor.java`
  - `.../OptionFieldScanService.java`
  - `.../TemplateRenderService.java`
  - `.../PluginEnumValueService.java`
  - `.../model/PluginOptionSchemaResult.java`
  - `.../model/PluginOptionDescriptor.java`
  - `.../model/RequiredMode.java`
  - `.../model/OptionOrigin.java`
- `FactoryUtil.sourceFullOptionRule(...)` / `sinkFullOptionRule(...)` 优先作为 source / sink 第一层主规则来源。
- transform / catalog 可先使用 `factory.optionRule()`。
- 第二层补扫只扫描：
  - 请求中的 `pluginJars`
  - 或 factory 所在 code source
- 候选类名限制在：
  - `*Options`
  - `*BaseOptions`
  - `*CommonOptions`
  - `*SourceOptions`
  - `*SinkOptions`
  - `*CatalogOptions`
  - `*ConfigOptions`
- 候选包名前缀限制在 `org.apache.seatunnel.`，避免扫第三方 driver 包。
- transform 列表发现也应复用相同 classloader / cache 体系，避免实现两套扫描逻辑。
- 高级模式的“运行时发现但未建档插件”只需在响应中显式标注来源为 proxy 运行时发现，不做额外分析。

## Suggested Implementation Slices

### Slice 1: Proxy schema / template / enum MVP
- 新增 `/api/v1/plugin/options`
- 新增 `/api/v1/plugin/template`
- 新增 `/api/v1/plugin/enum-values`
- 先打通 source / sink / transform
- 加入基础缓存

### Slice 2: Proxy plugin list
- 新增 `/api/v1/plugin/list`
- transform 下拉框接入
- source / sink 支持运行时发现兜底

### Slice 3: Platform sync 聚合接口
- 结合 cluster 已安装插件
- 为前端输出可直接消费的 source / sink / transform 选项
- 高级模式下补充 proxy 运行时发现但未建档的插件

### Slice 4: Frontend 模板追加
- 右侧三个下拉框
- 选中后追加模板到编辑器
- 自动滚动到新 block
- 基础 loading / warning 展示

### Slice 5: 编辑器枚举联动
- 基于 schema 缓存做 key 命中
- 缺失时走新枚举接口
- 枚举候选展示

## Risks

- 某些 factory 的 `optionRule()` 不完整或返回空，需要确保第二层补扫能兜底。
- 某些插件 Option 定义分散在父类 / common options 中，若类扫描范围过窄会漏项。
- 若缓存 key 设计不合理，插件升级后可能出现旧 schema 污染。
- 若前端把每次输入都绑定到远程查询，编辑器性能会明显恶化，必须避免。
- transform 发现机制若与运行时 classpath 不一致，可能出现 UI 可见但提交时不可用的错觉，需要清楚区分“发现来源”。

## Open Questions

- 高级模式中，运行时发现但未建档的插件不做进一步分析或差异解释，只标注其来源为 proxy 运行时发现。
- 模板追加后需直接选中新插入区域，而不是仅滚动到可见区域。
```

## Recent Updates

- 已补编辑器自定义 HOCON 高亮：`env` / `source` / `transform` / `sink` 仅在完整命中且作为 block 关键字时做差异化高亮；字符串转义（如 `quote_char = "\""`）显示也做了修正。
- 已补插件模板与枚举语义修正：proxy 不再把 Java enum name 直接当配置值，改为区分展示名与真实配置值，修复了 `datetime_format` 一类 legacy formatter 选项误生成枚举名的问题。
- 已补低版本兼容：当目标集群 SeaTunnel 版本低于 `2.3.9` 时，模板返回与预览 derive / 注入脚本会把 `plugin_input` / `plugin_output` 重写为 `source_table_name` / `result_table_name`。
- 已补编辑器 hover 信息：支持展示字段描述、默认值、requiredMode、enumValues，并兼容对注释掉的 key（如 `# key` / `#key` / `##key`）做识别。
- 已补全局枚举目录预加载与编辑器本地缓存联动，避免每次输入单个 key 都请求接口；接口失败仅降级，不阻塞工作台使用。
- 已持续优化枚举补全交互：支持 `=` / 空格 / `"` / 光标进入 value 区域触发；点击枚举后按整段 value 区域替换，避免只做局部插入。
- 已补模板输出分区与紧凑布局：区分 `required` / `conditional` / `optional` / `input` / `output` 区域；对 optional / conditional 的空值默认注释掉；模板字段间不再插空行。
