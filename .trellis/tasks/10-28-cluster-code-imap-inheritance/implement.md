# 实施规划与步骤: 集群标识(cluster_code)生命周期与IMAP clusterName自动继承

## 任务拆解与进展

- [x] **Phase 1: 故障根因验证与 IMAP clusterName 修复**
  - [x] 排查 SeaTunnel 3.0.0 IMAP 初始化抛出 `IllegalArgumentException: init file storage properties is not contains clusterName`。
  - [x] 在 `internal/apps/cluster/runtime_storage_apply.go` 中实现 `resolveIMAPClusterNameFromYAML` 并自动注入 `properties.clusterName`。
  - [x] 单元测试 `TestPatchIMAPYAMLInheritsClusterName` 验证通过。

- [x] **Phase 2: 后端 ClusterCode 领域与生命周期实现**
  - [x] 新增 `internal/apps/cluster/clustercode.go` 工具库（拼音清洗、正则校验、兜底生成、本地配置提取）。
  - [x] 在 `internal/apps/cluster/model.go` 中增加 `ClusterCode` 字段与 `EffectiveClusterCode()` 方法。
  - [x] 在 `internal/apps/cluster/service.go` 中完善 `Create`（清洗/校验/兜底）与 `Update`（不可变性保持）。
  - [x] 单元测试 `TestClusterCode_LifecycleAndRenameImmutability` 验证通过。

- [x] **Phase 3: 安装与扩容通道参数传递**
  - [x] `internal/apps/installer/types.go` 与 `service.go` 传递 `ClusterName`。
  - [x] `internal/apps/cluster/scale_executor.go` 传递 `ClusterName: view.cluster.EffectiveClusterCode()`。
  - [x] `agent/cmd/main.go` 接收 `ClusterName` 参数。
  - [x] `agent/internal/installer/manager.go` 在 `modifyHazelcastConfig`、`modifyHazelcastClientConfig` 与 `configureIMAPStorage` 中设置 `cluster-name`。
  - [x] Agent 单元测试 `TestModifyHazelcastConfigSetsClusterName` 验证通过。

- [x] **Phase 4: 纳管外部集群导入 (Import/Discovery)**
  - [x] Agent 发现增强：`agent/internal/discovery/config_parser.go` 读取并上报 `ClusterName`。
  - [x] 控制面发现透传：`discovery.DiscoveredProcess`、`ScaleDiscoveredProcess` 与 `CreateNodeFromDiscovery` 均支持 `ClusterName`。
  - [x] 导入自适应：当未显式指定 `cluster_code` 时，自动提取发现节点或本地目录 `hazelcast.yaml` 的真实 `cluster-name` 作为 `cluster_code`。
  - [x] 扩容预检：在纳管模式扩容节点时比对 `cluster-name`，若不一致提供告警提示。
  - [x] 单元测试 `TestService_CreateFromDiscovery_InheritsDiscoveredClusterNameAsClusterCode` 验证通过。

- [x] **Phase 5: CLI 约定检查与更新**
  - [x] `cluster.get` 与 `cluster.list` 自动透出新增的 `cluster_code` 属性。
  - [x] `internal/operation` 单测通过。

- [x] **Phase 6: 前端向导与配置编辑集成**
  - [x] `frontend/lib/cluster-code.ts`：实现 `isValidClusterCode` 与 `recommendClusterCode`，配套单测 `cluster-code.test.ts` 通过。
  - [x] `frontend/components/common/cluster/wizard/ClusterBasicStep.tsx`：提供集群标识输入框，随集群名称自动推荐，支持手动编辑并提供格式校验提示与不可变说明。
  - [x] `frontend/components/common/cluster/ClusterDeployWizard.tsx`：创建集群时传递 `cluster_code`。
  - [x] `frontend/components/common/cluster/CreateClusterDialog.tsx`：纳管已有节点时传递发现的 `cluster_name`。
  - [x] `frontend/components/common/cluster/EditClusterDialog.tsx`：展示只读 `cluster_code`，说明创建后不可修改。
  - [x] `frontend/components/common/cluster/ClusterDetail.tsx`：头部徽章展示 `cluster_code`。
  - [x] `frontend/components/common/cluster/ClusterConfigs.tsx`：在线配置编辑保存 `hazelcast.yaml` / `hazelcast-client.yaml` 时，若检测到 `cluster-name` 修改，弹出级联风险警示对话框（不强行阻断，确认后保存）。
  - [x] `frontend/lib/i18n/locales/zh.json` 与 `en.json`：补充完整双语文案，覆盖率测试通过。

- [x] **Phase 7: 质量门禁与验证**
  - [x] 后端单元测试覆盖并通过（`go test ./internal/apps/cluster/... ./internal/apps/installer/...`）。
  - [x] Agent 单元测试并通过（`cd agent && go test ./...`）。
  - [x] 前端 TypeScript 编译无错误（`cd frontend && pnpm exec tsc --noEmit`）。
  - [x] 前端单元测试与文案覆盖率测试通过（`cd frontend && pnpm exec vitest run lib/__tests__/cluster-code.test.ts && pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts`）。
  - [x] 双语注释规范自检（中文在前，英文在后）。
