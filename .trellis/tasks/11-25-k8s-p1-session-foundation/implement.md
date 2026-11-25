# P1 执行清单：K8s 底座、常驻容器模式与伴生 Proxy

> 开发前必读：`trellis-before-dev` + `.trellis/spec/backend/index.md` + `.trellis/spec/frontend/index.md` + `.trellis/spec/guides/cross-layer-thinking-guide.md`

## 1. 有序实施步骤

- [ ] **Step 1 · 依赖与凭据加密**
  - `go.mod` 引入 `k8s.io/client-go`、`k8s.io/api`、`k8s.io/apimachinery`
  - 实现 `internal/utils/secretbox` 及单测（加密/解密/明文兼容/空串处理）
- [ ] **Step 2 · 统一 K8s 适配层（`internal/pkg/k8s`）**
  - 实现 `K8sClientManager`（kubeconfig/token/in_cluster + 缓存失效 + `SelfSubjectAccessReview` RBAC 预检）
  - 实现 `K8sTransport`（`apiserver_proxy` / `in_cluster` / `external`）与单元测试（`httptest` 模拟 API Server Proxy）
  - 实现 `K8sJavaProxyManager`（272 KB ConfigMap `binaryData` + 智能卷克隆 + Deployment/Service 托管 + `/healthz` 探活）与 Fake Clientset 单测
  - 实现 `K8sSessionManager`（纳管发现 + 一键部署标准 StatefulSet/Headless Svc/ConfigMap/RBAC + Pod 投影 + Scale/Rollout/UpgradeImage + ConfigMap 读写 + PodLogs + 按 `management_source` 安全卸载）与 Fake Clientset 单测
- [ ] **Step 3 · `host` 与 `cluster` 后端域接入**
  - 完善 `internal/apps/host` 加密落库、脱敏回显、更新留空保留、连通测试/Namespace/发现接口
  - 扩展 `internal/apps/cluster` 模型列（含 P2 预留字段）与数据库迁移、`scope` 列表过滤、K8s 集群创建（纳管/一键部署）自动触发伴生 Proxy、双校验健康检查、WebUI 代理与 `stx_java_proxy` 分发、扩缩容/滚动重启/镜像升级接口
- [ ] **Step 4 · `sync` / `monitoring` / `config` / `plugin` 跨模块接入**
  - `sync`：`K8sRuntimeResolver` + 支持 `K8sTransport` 的 `EngineClient` / `ConfigToolClient`、`GetJobLogs` K8s PodLogs 分支、拦截 K8s 集群走 `local` 模式
  - `monitoring`：K8s 集群 8080 `/metrics` 抓取目标与 `K8sTransport`、`availability_evaluator` 跳过 `agent_offline`
  - `config`：K8s ConfigMap 拉取/更新/版本管理与可选滚动生效
  - `plugin`：K8s 集群只读镜像连接器视图（调用伴生 Proxy），拒绝 K8s 写装/卸请求
  - `seatunnel/capabilities.go`：补充 K8s 能力位
  - `internal/router/router.go` & `internal/operation/registry.go`：注册全部新路由与操作元数据
- [ ] **Step 5 · CLI 与前端对齐**
  - `internal/cmd/`：补齐 `stx host`（kubernetes 参数/测试/发现）与 `stx cluster`（K8s 创建/纳管/部署/scale/rollout-restart/upgrade-image）及单测
  - `frontend/`（严格 `.arc-compact` + 单层画布 `0.5px` 细线 + 中英双语）：
    - 主机管理：K8s 主机表单（三类认证 + 测试连接 + RBAC 检查清单 + 掩码留空不覆盖）
    - 集群管理：新增 K8s 常驻集群纳管/一键部署向导（保存自动拉起并探活伴生 Proxy）、集群详情适配 Pod 节点表/扩缩容/镜像滚动升级/ConfigMap/只读镜像插件页
    - 工作台：K8s 集群隐藏 `local` 模式，无缝走伴生 Proxy 校验/测试连接/预览/DAG
- [ ] **Step 6 · 云端真实 Minikube 端到端验证**
  - 在云端环境验证 AC-2、AC-3（纳管现有 `seatunnel` 集群）、AC-4（新建 `seatunnel-stx-e2e` 一键部署→运行作业→改镜像滚动→卸载）、AC-5（插件只读边界）

## 2. 验证命令

```bash
go test ./...
go test -race ./internal/pkg/k8s/... ./internal/apps/host/... ./internal/apps/cluster/... ./internal/apps/sync/... ./internal/apps/monitoring/...
(cd agent && go test ./...)
(cd frontend && pnpm exec tsc --noEmit && pnpm test && pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts)
```
