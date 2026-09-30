# stx-java-proxy 双代际设计

## 1. 当前实现（已落地）

### 目标
同一套源码按 SeaTunnel 主版本分别编译，产出编译依赖对齐的 thin jar，供 2.x / 3.x 集群选型。

### 结构

```
tools/stx-java-proxy/          ← 单一 Maven 工程、一份业务源码
  ├─ -Pepoch-v2                → SeaTunnel 2.3.13 + 旧 optional shade → stx-java-proxy-v2.jar
  └─ -Pepoch-v3,!epoch-v2      → SeaTunnel 3.0.0 + seatunnel-shade-* → stx-java-proxy-v3.jar
```

运行时仍是 thin jar：`classpath = SEATUNNEL_HOME/lib/* + starter + proxy jar`。  
版本行为差异放在 `SeaTunnel23EngineAdapter` / `SeaTunnel30EngineAdapter`（反射 + 能力探测），不拆模块。

### 选型

| 输入 | 代际 | jar |
|------|------|-----|
| `2.x` / 空 / `v2` | `v2` | `stx-java-proxy-v2.jar` |
| `3.x` / `v3` | `v3` | `stx-java-proxy-v3.jar` |

3.x 若本地尚无 v3 jar，Agent 侧可回退 v2（滚动升级）。

### 构建注意
- 默认激活 `epoch-v2`；打 v3 必须 `-Pepoch-v3,!epoch-v2`，否则会同时解析 2.3 optional 与 3.0 shade 坐标。
- 3.0 shade 模块坐标与 2.3 不同（如 `seatunnel-shade-jackson:2.15.4-3.0.0`），已按 profile 分别声明。

---

## 2. 预留：多模块拆分（未实施）

当「一套源码 + 双 profile」不够用时，再拆成下列模块。**在此之前不要提前拆。**

### 目标布局

```
tools/stx-java-proxy/
  proxy-api/            纯接口与 DTO，无 SeaTunnel 依赖
  proxy-adapter-v23/    仅对 SeaTunnel 2.3.x 编译的适配实现
  proxy-adapter-v30/    仅对 SeaTunnel 3.0.x 编译的适配实现
  proxy-app/            HTTP/业务组装；依赖 proxy-api + 运行时选中的 adapter
```

### 职责边界

| 模块 | 依赖 SeaTunnel？ | 职责 |
|------|------------------|------|
| `proxy-api` | 否 | `SeaTunnelEngineAdapter` 接口、能力枚举、跨版本 DTO |
| `proxy-adapter-v23` | 仅 2.3.x | 2.3 类型安全实现；可直接 `import` 2.3 API |
| `proxy-adapter-v30` | 仅 3.0.x | 3.0 类型安全实现；可直接 `import` 3.0 新类型/SPI |
| `proxy-app` | 通过 adapter | 现有 Service / Server / CLI；禁止直接依赖「仅单边存在」的 ST 类型 |

### 产物与现有代际对齐

拆分后仍对外发布：

- `stx-java-proxy-v2.jar` ← `proxy-app` + `proxy-api` + `proxy-adapter-v23`（shade 或薄组装）
- `stx-java-proxy-v3.jar` ← `proxy-app` + `proxy-api` + `proxy-adapter-v30`

`ProxyEpochForVersion` / 安装下载链路**不必改命名约定**。

### 何时触发拆分（满足任一即可评估）

1. 3.0 新功能必须大量使用「仅 3.0 存在」的类型/SPI，反射成本过高或易错。
2. 同名类在 2.3 / 3.0 语义分叉，反射也无法安全统一。
3. 双 profile 下公共代码需大量 `#ifdef` 式分支，可读性崩溃。
4. CI 中 `epoch-v2` 与 `epoch-v3` 频繁出现「一边能编、一边不能编」且无法用 adapter 收敛。

### 明确不触发拆分的情况

- 仅新增连接器依赖基线（`seatunnel-plugins.json`）— 与 proxy 模块无关。
- 仅 checkpoint / DAG 等可用反射探测的字段差异 — 继续放在现有 `*EngineAdapter`。
- 仅打包脚本或代际映射调整。

### 迁移步骤草案（触发后再做）

1. 抽出 `proxy-api`（现有 `adapter` 接口 + 模型）。
2. 将 `SeaTunnel23EngineAdapter` / `SeaTunnel30EngineAdapter` 分别迁入 adapter 子模块，并改为**编译期依赖**对应 SeaTunnel 版本（可逐步去掉反射）。
3. `proxy-app` 只依赖 `proxy-api`；组装时用 Maven profile 或 classifier 绑定对应 adapter。
4. 保持对外 jar 名 `stx-java-proxy-v2.jar` / `v3.jar` 与 Go 侧 epoch 逻辑不变。
5. 双 profile CI 改为「每个 epoch 编对应 adapter + app」。

### 兼容性约束

- 业务主流程仍应一份（`proxy-app`），禁止复制整套 Service。
- 新能力优先：`proxy-api` 声明能力位 → 单边 adapter 实现 → 另一边降级/返回 unsupported。
- 禁止在 `proxy-app` 中直接 `import` 仅某一代际存在的 SeaTunnel 类型。

---

## 3. 决策摘要

| 阶段 | 形态 | 状态 |
|------|------|------|
| 现在 | 单工程 + `epoch-v2` / `epoch-v3` 双编译 | **已落地** |
| 预留 | `proxy-api` + `adapter-v23/v30` + `proxy-app` | **设计预留，未实施** |

默认路径：继续在当前单工程内用 adapter 承接 3.x 新功能；仅当「触发拆分」条件成立时再迁到多模块。
