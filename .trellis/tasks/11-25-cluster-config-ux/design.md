# 集群级配置文件双模态编辑与配置集 — 轻量技术设计

> 关联 PRD：[prd.md](./prd.md)

---

## 1. 后端设计

### 1.1 `yaml.Node` 结构化提取与局部 Patch 引擎（`internal/apps/config/structured.go`）
复用并泛化 [event_report.go](../../internal/apps/config/event_report.go) 已验证的 `gopkg.in/yaml.v3` `yaml.Node` 模式：
- `ExtractSeatunnelStructured(yamlText string) (*SeatunnelStructuredConfig, error)`：解析 `seatunnel.yaml` 提取 `http`、`slot_service`、`checkpoint`、`telemetry`、`history_job_expire_minutes`、`job_log_mode`、`event_report_http` 及检测到的 `${VAR}` 列表与潜在明文凭据键告警。
- `PatchSeatunnelStructured(baseYAML string, patch *SeatunnelStructuredPatch) (string, error)`：在原始 `yaml.Node` 树上原地增改目标路径，保留全部注释、空白行与未纳管的自定义键，写回前过 `validateConfigContent`。
- `ExtractJVMOptions(text string)` / `PatchJVMOptions(text string, patch *JVMOptionsPatch)`：解析并原地更新 `-Xms`、`-Xmx`、GC Flag（`-XX:+UseG1GC` / `-XX:+UseZGC`），保留其余行内注释与用户自定义参数。
- 提供无状态预览 API（供前端表单↔YAML 实时互转，不落库）：
  - `POST /api/v1/configs/structured/parse`
  - `POST /api/v1/configs/structured/patch`

### 1.2 伴生 Proxy 镜像默认基线读取
- `tools/stx-java-proxy` 新增只读端点 `GET /api/v1/config/defaults`：按白名单读取 `$SEATUNNEL_HOME/config/` 下存在的标准配置文件原文（单文件大小上限 256 KB，拒绝路径穿越）。
- STX 后端暴露 `GET /api/v1/clusters/:id/configs/defaults`：K8s 集群经 `K8sTransport` 调伴生 Proxy 获取；物理机集群可按需从节点或内置版本基线回退。

### 1.3 配置挂载包（`cluster_config_profiles` / Bundle，兼顾 SeaTunnel、Hadoop/Hive 与未来 YARN）
```go
type ConfigProfile struct {
    ID             uint          `gorm:"primaryKey;autoIncrement"`
    Name           string        `gorm:"size:120;uniqueIndex;not null"`
    Description    string        `gorm:"size:500"`
    BundleKind     string        `gorm:"size:32;default:'seatunnel'"` // seatunnel | hadoop | kerberos | custom
    DeploymentMode string        `gorm:"size:20;default:'separated'"` // hybrid | separated
    InfraScope     string        `gorm:"size:32;default:'all'"`       // all | bare_metal | kubernetes_session | kubernetes_application_template | yarn
    MountPath      string        `gorm:"size:255"`                    // 默认挂载目录，如 /etc/hadoop/conf
    EnvVarsJSON    db.ScriptText // 伴随环境变量，如 {"HADOOP_CONF_DIR":"/etc/hadoop/conf"}
    Sensitive      bool          `gorm:"default:false"`               // true 时内容经 secretbox AES-GCM 加密，K8s 落地为 0400 Secret
    Builtin        bool          `gorm:"default:false"`
    FilesJSON      db.ScriptText // map[fileName]string（普通文本或 base64/enc:v1: 密文）
    CreatedBy      uint
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```
- **伴生 `stx-java-proxy` 同步挂载契约**：集群或 App 模板绑定的所有非 RWO 配置挂载包（`hadoop` / `kerberos` / 外部 ConfigMap/Secret）及伴随环境变量，必须一并注入伴生 `stx-java-proxy` Deployment，确保工作台对 HDFS / Hive / Iceberg / Kerberos 的连通性测试与 Catalog/Checkpoint 探测与引擎视角完全一致。
- API & CLI 对齐：
  - `GET/POST/PUT/DELETE /api/v1/config-profiles`、`POST /api/v1/clusters/:id/configs/save-as-profile`、`POST /api/v1/clusters/:id/configs/apply-profile`
  - 对应 `stx config-profile list|get|create|update|delete|apply`

---

## 2. 前端交互设计（遵循 `.trellis/spec/frontend/`）

1. **复用已有组件**：将 [RuntimeAdvancedConfigCard.tsx](../../frontend/components/common/installer/RuntimeAdvancedConfigCard.tsx) 及 [ConfigStep.tsx](../../frontend/components/common/installer/ConfigStep.tsx) 中的 Checkpoint / HTTP / JVM 表单块下沉到 `frontend/components/common/config/structured/`，由安装向导、集群配置页、K8s 一键部署与 App 模板共享，零重复实现。
2. **`ClusterConfigs.tsx` 单层扁平画布升级**：
   - 顶栏右侧 `SegmentedControl` 切换 **`表单` / `源码`**（仅对支持结构化的 `seatunnel.yaml` / `jvm_*_options` 显示，其余文件直出源码）；
   - 工具栏轻量动作：「载入镜像默认」「从集群/配置集导入」「快捷片段」「与镜像默认 Diff」「版本历史」；
   - 源码模式保留行号与语法高亮，上方以 `ArcAlert` 提示检测到的明文密钥与未绑定 `${VAR}` 占位符；
   - 保存前在确认弹窗内直接展示变更 Diff，未保存离开触发 `DirtyCloseDialog`。

---

## 3. 与 K8s 路线图的衔接顺序

1. **PR #208（P1）**：保持不变，先交付 K8s 底座、常驻模式与伴生 Proxy；
2. **P2（`11-25-k8s-p2-app-mode`）**：先落地伴生 Proxy `GET /api/v1/config/defaults` 与 App 模板三选一（`image_default` / `stx_managed` / `external_ref`）；
3. **本任务（`11-25-cluster-config-ux`）**：作为独立 PR 交付跨物理机 + K8s 的表单↔YAML 双模态编辑器、快捷片段、配置集（Config Profile）与 `${VAR}` Secret 防呆校验。
