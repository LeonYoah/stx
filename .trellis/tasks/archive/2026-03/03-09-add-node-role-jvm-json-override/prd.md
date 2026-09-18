# redesign add-node role selection and node-level JVM override

## Goal

修正 STX 当前“添加节点 / Add Node”在集群部署模型上的语义错误，并补齐节点级 JVM 自定义能力。

本任务要解决两个已确认的问题：

1. **混合模式（Hybrid）下角色选择错误**：当前 UI 仅允许选择 `master` / `worker`，但混合模式的真实语义应为固定的 `master/worker`；
2. **节点级 JVM 配置能力不足**：当前 JVM 配置偏集群默认值，无法表达“同一主机上的 master 与 worker 内存配置不同”或“某个节点覆写默认 JVM”的场景。

本任务已确认采用 **方案 B：JSON overrides**，即通过节点级 JSON 配置覆盖（override）来承载 JVM 等扩展配置，而不是为每类配置持续增加独立列。

---

## Requirements

### R1. 修正角色建模与交互语义

“添加节点”必须严格区分两种部署模式：

#### 混合模式（Hybrid）
- 不允许用户再手动选择 `master` 或 `worker`；
- 节点角色固定为 `master/worker`；
- UI 应显示为只读或不可切换状态，并明确说明“混合模式下每个节点同时承担 Master 与 Worker 角色”。

#### 分离模式（Separated）
- 允许用户选择 `master` 或 `worker`；
- 应支持**同一主机同时承担两个角色**；
- 这意味着前端交互不应再被设计成“单选角色 + 单次只能创建一个逻辑角色”的形式。

### R2. 同主机双角色能力

在分离模式下，添加节点应支持“同一主机添加两个角色实例”：

- 用户可一次性为同一主机勾选 `master` 与 `worker`；
- 系统应明确向用户表达：这将创建两个逻辑节点实例，而不是一个混合节点；
- 两个角色实例应允许分别配置端口与 JVM 覆盖值；
- 创建流程应尽量保持原子性，避免“一半成功、一半失败”的脏状态。

### R3. 节点级 JVM 覆盖采用 JSON overrides（方案 B）

节点模型应支持一个可扩展的 JSON 覆盖字段，例如：

```json
{
  "jvm": {
    "hybrid_heap_size": 8,
    "master_heap_size": 4,
    "worker_heap_size": 16
  }
}
```

要求：

- 覆盖字段应属于**节点级配置**，而不是替代现有集群默认 JVM 配置；
- 未配置 override 时，节点继承集群默认值；
- 配置了 override 时，节点安装/升级/配置生成链路应优先使用 override；
- JSON 结构需明确约束，避免出现任意脏 key。

### R4. Add Node Dialog 交互重构

“添加节点”对话框应按部署模式动态渲染：

#### 混合模式
- 角色区域显示固定值：`Master/Worker`；
- 端口区显示混合模式所需字段（如 Hazelcast / API / WorkerPort）；
- JVM 区只显示 `hybrid_heap_size` 覆盖项；
- 支持“使用集群默认 / 自定义覆盖”切换。

#### 分离模式
- 角色区域改为多选或复选：`Master`、`Worker`；
- 若只选 `Master`，仅展示 master 相关端口与 JVM；
- 若只选 `Worker`，仅展示 worker 相关端口与 JVM；
- 若同时选中两者，应分组展示：
  - Master 端口与 JVM
  - Worker 端口与 JVM
- UI 中应明确提示“将在同一主机创建两个节点实例”。

### R5. 后端 API / 数据结构调整

当前 `AddNodeRequest` 为单节点语义，不足以表达“同主机双角色 + 分别 override”的场景。

应评估并落地其中一种方式：

#### 推荐方案
新增批量/多实例语义接口，例如一次提交：
- 一个 host
- 多个 entries（master / worker）
- 每个 entry 带各自端口与 overrides

#### 兼容要求
- 如短期无法直接替换旧接口，应保证前端/后端有清晰的兼容策略；
- 若保留旧接口，需明确它在 hybrid / separated 下的语义边界，避免继续扩散错误建模。

### R6. 安装 / 升级 / 配置生成链路必须识别 overrides

节点级 override 不能只停留在 UI 与数据库。

要求至少明确以下链路的行为：

- 节点安装时如何读取 override JVM 值；
- 节点升级时如何继承/回填 override；
- 配置生成与下发时，override 与集群默认值的优先级；
- 集群详情 / 节点编辑 / 巡检展示时，如何区分“继承默认”与“已覆盖”。

---

## Acceptance Criteria

- [ ] 混合模式下，“添加节点”不再允许用户手选 `master` / `worker`，而是固定显示 `master/worker`
- [ ] 分离模式下，UI 支持同一主机同时选择 `master` 与 `worker`
- [ ] 同一主机双角色场景可以分别录入端口和 JVM 参数
- [ ] 节点模型新增 JSON overrides 能力，并明确约束 `jvm` 结构
- [ ] 节点未设置 override 时，安装/升级/配置生成链路继续使用集群默认 JVM
- [ ] 节点设置 override 时，安装/升级/配置生成链路优先使用节点级 override
- [ ] API 契约能表达“单主机多角色实例”的创建请求，且具有明确兼容策略
- [ ] Add Node Dialog 交互能根据 Hybrid / Separated 正确切换显示逻辑
- [ ] 集群详情或节点详情可识别并展示“继承默认 / 已自定义覆盖”状态
- [ ] 至少补充 1 组前后端测试，覆盖节点级 override 的解析或应用逻辑

---

## Technical Notes

### Current problems in existing code

当前实现存在以下已识别问题：

- 前端 `AddNodeDialog.tsx` 仅提供 `master` / `worker` 两个选项；
- 但前后端类型与模型实际上已存在 `master/worker`（`NodeRole.MASTER_WORKER`）；
- 当前 hybrid 的实现语义更接近“master + workerPort”，属于技术拼接，不是正确的产品建模；
- `cluster_nodes` 当前没有节点级 JVM override 字段；
- `AddNodeRequest` 仍是单角色、单实例心智，不适合承载同机双角色。

### Expected related files

可能涉及但不限于：

- `frontend/components/common/cluster/AddNodeDialog.tsx`
- `frontend/components/common/cluster/EditNodeDialog.tsx`
- `frontend/lib/services/cluster/types.ts`
- `frontend/lib/i18n/locales/zh.json`
- `frontend/lib/i18n/locales/en.json`
- `internal/apps/cluster/model.go`
- `internal/apps/cluster/service.go`
- `internal/apps/cluster/repository.go`
- `internal/apps/cluster/handler.go`
- `internal/db/migrator/migrator.go`
- `internal/apps/installer/types.go`
- `internal/apps/installer/service.go`

### Recommended implementation order

1. 明确 API / 数据模型：节点 overrides JSON 结构、创建请求结构
2. 数据库迁移：为节点增加 overrides 字段（或等价结构）
3. 后端落地：AddNode / UpdateNode / install / upgrade / config apply
4. 前端改造：Add Node Dialog 与节点编辑页
5. 展示与校验：详情页展示 override 状态，补充文案与表单校验
6. 联调验证：混合模式固定角色、分离模式双角色、override 生效链路

### Non-goals for this task

本任务暂不追求：

- 一次性重做全部部署向导与集群编辑页的所有交互；
- 把所有节点级配置都迁移到 override，仅先聚焦 JVM；
- 引入复杂的模板化配置系统。
