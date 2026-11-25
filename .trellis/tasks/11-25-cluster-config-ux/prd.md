# 集群级配置文件双模态编辑、镜像基线与配置集复用

> 类型：跨形态配置体验升级（同时覆盖物理机 `bare_metal`、K8s 常驻集群 `kubernetes_session` 与 K8s App 模板 `kubernetes_application_template`）
> 关联规划：[11-25-k8s-integration](../11-25-k8s-integration/design.md) | [11-25-k8s-p2-app-mode](../11-25-k8s-p2-app-mode/prd.md)
> 详细技术草案：[design.md](./design.md)

---

## 1. 背景与痛点

当前 STX 已具备集群配置文件的版本管理与 Diff 同步（`internal/apps/config/`）、安装期结构化表单（`ConfigStep.tsx` / `RuntimeAdvancedConfigCard.tsx`）以及基于 `yaml.Node` 的无损局部 Patch 能力（`event_report.go`），但存在三处明显断层：

1. **安装后只剩裸文本框**：集群详情「配置管理」页（`ClusterConfigs.tsx` / `ConfigEditor.tsx`）仅提供纯 `<Textarea>`，调整 `seatunnel.yaml`（Checkpoint 存储、HTTP 端口与 Basic Auth、Slot 策略、Metrics、Event Report）或 `jvm_*_options` 全靠手写 YAML 缩进，容易出错且缺少即时校验提示。
2. **跨集群与多模板无法复用**：多套物理机集群、K8s Session 集群或 K8s App 模板需要共用同一套生产配置（如 S3/OSS Checkpoint、统一 `log4j2.properties`、JVM 参数）时，只能逐个重复手工修改。
3. **K8s 镜像基线与凭据安全体验缺失**：参考 Apache StreamPark（`FlinkEnv` 同步基线、模板抽屉、快捷片段、变量管理脱敏）与 Dinky（表单 + 自定义配置），SeaTunnel 引擎配置忠于 Pod 内挂载的 `seatunnel.yaml`（与 `LeonYoah/seatunnel#6` 原则一致），需要「零黑盒单一真相源 + 真实镜像默认基线 + `${VAR}`/Secret 零明文」的原生方案。

---

## 2. 核心需求

### 2.1 「表单视图 ↔ YAML 源码」双向 AST 同步（单一真相源，零黑盒）
- 在集群配置工作台提供 **表单视图** 与 **YAML 源码视图** 无缝切换，底层始终操作同一份 `seatunnel.yaml` / `hazelcast-*.yaml` / `jvm_*_options` 文件内容（不做 Dinky/Flink 式隐式覆盖层）：
  - **`seatunnel.yaml` 结构化表单**（分 4 组渐进披露）：
    1. **HTTP 与鉴权**：`enable-http`、`port`（K8s 模式明确区分容器监听端口与 Service 发布端口）、Basic Auth 开关与账密；
    2. **Slot 与调度**：动态/静态 Slot、`slot-num`、分配策略、历史作业过期、`job-log-mode`；
    3. **Checkpoint 存储**：Local/HDFS/OSS/S3/COS 表单 + **「测试存储连通性」**（复用 `stx-java-proxy` `/api/v1/storage/checkpoint/probe`）；
    4. **Telemetry 与事件通道**：Prometheus `/metrics` 开关、`event-report-http` 开关。
  - **`jvm_*_options` 结构化表单**：`-Xms` / `-Xmx`、GC 算法（G1GC / ZGC）与附加参数。
  - **无损局部 Patch**：泛化 `internal/apps/config/event_report.go` 的 `yaml.Node` AST 读写能力，表单修改仅更新目标节点，**100% 保留用户手写的注释、字段顺序与其他高级自定义项**；保存前展示与上一版本的 Diff 预览并写入 `config_versions`。

### 2.2 真实镜像默认基线拉取与 K8s `subPath` 增量覆盖
- 伴生 `stx-java-proxy` 提供只读接口 `GET /api/v1/config/defaults`，直接读取目标容器镜像内 `$SEATUNNEL_HOME/config/*`（`seatunnel.yaml`、`hazelcast-*.yaml`、`jvm_*_options`、`log4j2.properties`）。
- 用户在 K8s 集群或 App 模板中一键「载入镜像默认配置」即可拿到当前镜像真实基线（天然适配任意 SeaTunnel 版本与企业定制镜像，STX 无需硬编码维护多版本模板）。
- 支持 **「与镜像默认值对比 Diff」**；K8s 落地时仅将发生变更的文件写入 ConfigMap 并按 `subPath` 挂载，未改动文件继续沿用镜像内置版本。

### 2.3 快捷片段、从已有集群导入与「配置集（Config Profile）」复用
- **快捷片段（Snippet）**：源码编辑器内置一键插入常用配置块（S3/OSS/HDFS Checkpoint、Basic Auth、Metrics、K8s Hazelcast DNS 发现）。
- **从已有集群复制**：支持选择任意同类已有集群，一键拉取其配置文件进行对比导入（借鉴 StreamPark 历史模板抽屉）。
- **配置集（Config Profile / Preset）**：
  - 新增 `cluster_config_profiles` 表，支持保存一组标准化配置文件（可只包含部分覆盖文件，如仅 `seatunnel.yaml` + `log4j2.properties`）；
  - 内置 3 套官方只读 Preset（开发快速上手 / 生产 K8s 标准 / App 模式轻量化）；
  - 任意已调优集群支持「另存为配置集」；新建物理机集群、一键部署 K8s Session 集群、创建 K8s App 模板时支持直接套用或派生。

### 2.4 `${VAR}` 占位符识别、明文凭据防呆与 K8s Secret 绑定
- 编辑器自动扫描配置文件中的 `${ENV_VAR}` 占位符并展示绑定状态（已绑定 K8s Secret / 环境变量 / 未定义）。
- 当检测到 K8s 集群/App 模板的 `seatunnel.yaml` 中出现明文密钥字段（如 `secret.key`、`basic-auth-password`、`access.key`）时，给出明确风险提示并引导改用 `${VAR}` + `secret:<name>/<key>` 注入（由 SeaTunnel 容器启动时解析，确保 ConfigMap 零明文凭据）。

---

## 3. 验收标准

- [ ] **AC-1（双向无损 AST 同步）**：`seatunnel.yaml` 表单修改 HTTP / Slot / Checkpoint / Telemetry / EventReport 后写回 YAML，单测验证原有注释、无关自定义字段与顺序 100% 保留；切源码视图所见即所得，保存前展示 Diff。
- [ ] **AC-2（镜像基线与增量挂载）**：K8s 集群/App 模板可通过伴生 Proxy 读取目标镜像真实默认配置文件并支持与镜像默认做 Diff；未改动文件不写入托管 ConfigMap 也不遮蔽镜像文件。
- [ ] **AC-3（配置集与跨集群复用）**：支持内置 Preset、从已有集群导入、以及 CRUD 自定义配置集（含 CLI 对齐），并在物理机、K8s Session 一键部署、K8s App 模板中均可一键套用。
- [ ] **AC-4（凭据防呆与 `${VAR}` 绑定）**：明文敏感键触发提醒；`${VAR}` 占位符可识别并校验绑定状态，K8s 渲染自动通过 `secretKeyRef` 注入，ConfigMap 不含明文密钥。
- [ ] **AC-5（质量门禁）**：前端严格遵循 `.trellis/spec/frontend/`（`.arc-compact`、单层画布 `0.5px` 细线、无嵌套 `<Card>`、单语言 i18n、脏态拦截）；全量 Go / Frontend / i18n / CLI 测试通过。

---

## 4. 明确非目标（Out of Scope）

- 不引入 Dinky 式“STX 宿主机本地 `/opt/seatunnel/conf` 文件路径依赖”。
- 不做隐藏式 `-D` 参数覆盖层（避免与 `seatunnel.yaml` 形成两套真相源，保持与 `LeonYoah/seatunnel#6` 职责解耦一致）。
- 不在本任务中实现跨团队 RBAC 级独立“全局密钥中心”（K8s 优先直接对接原生 Secret `secretKeyRef`，物理机沿用现有集群环境变量/加密配置机制）。
