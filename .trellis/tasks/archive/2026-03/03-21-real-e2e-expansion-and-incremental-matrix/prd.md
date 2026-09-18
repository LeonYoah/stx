# real e2e expansion and incremental matrix

## Goal

在现有真实安装 E2E 的基础上，把 STX 的真实环境端到端测试扩展成一套可持续维护的能力矩阵：覆盖升级、配置管理、插件市场、运行时存储、Web UI 等核心真实链路，并统一纳入当前 `E2E` workflow 中，通过增量选择器按改动范围只触发必要的真实套件，避免每次 PR 都全量跑所有重型场景。

## What I already know

* 当前已经有一套真实安装 E2E：能够在 GitHub CI 中真实启动 backend/frontend/agent，联网拉取指定 SeaTunnel 版本，完成单节点安装，并断言 `seatunnel.yaml`、`hazelcast*.yaml`、`hazelcast-client.yaml`、`log4j2.properties` 等产物。
* 真实安装 E2E 已支持 MinIO，能够验证 checkpoint / IMAP 的 S3 配置，并且 CI 环境已经修复了清理权限问题：CI 默认只停 MinIO 容器，不再强制删除本地临时目录。
* 当前 `E2E` workflow 已统一承接 smoke 与 installer-real，不再拆成两个 workflow。
* 当前增量选择器 `frontend/scripts/e2e/select-e2e-specs.mjs` 只负责 smoke 套件的增量选择；真实安装类 suite 目前仍是按 job 级开关触发。
* 近期改动较多的高价值真实链路包括：升级、配置管理、插件下载与安装、运行时存储详情、HTTP/WebUI 代理。

## Problems to Solve

1. 当前真实环境 E2E 只有 installer-real，一旦产品核心链路继续增加，容易回到“新增一个功能就补一个零散 workflow / 脚本”的维护方式。
2. 真实环境 E2E 还没有形成统一的 suite/profile 命名与触发规则，后续新增测试难以稳定扩展。
3. 当前没有把“哪些代码改动应该触发哪些真实套件”收口成显式矩阵，无法继续扩展增量跑。
4. 还缺少对升级、配置管理、插件市场、运行时存储、WebUI 等真实关键链路的 CI 级回归保护。

## Requirements

### A. 总体结构
- 继续沿用统一的 `E2E` workflow，不再拆新的 workflow。
- 在 `E2E` workflow 内，真实环境测试按 suite/job 扩展，而不是按页面散落。
- suite 命名要体现能力，而不是页面名。

### B. 下一批真实环境 E2E 覆盖范围
至少规划以下 suite：

1. `installer-real`
   - 已有，保留并继续作为安装主链路基线。

2. `upgrade-real`
   - 覆盖集群从旧版本升级到目标版本的真实流程。
   - 需要验证升级前目标版本包准备、升级步骤、插件资产迁移、配置迁移、升级后版本与文件结果。

3. `config-real`
   - 覆盖配置管理真实流程。
   - 需要验证全局集群配置编辑、智能修复、保存并同步到所有节点、从节点同步、历史版本预览/对比/回滚。

4. `plugin-real`
   - 覆盖插件市场/本地插件真实流程。
   - 需要验证下载插件、profile 选择、依赖落盘、metadata、安装到集群、集群详情查看插件详情。

5. `storage-real`
   - 覆盖运行时存储详情与治理。
   - 需要验证 checkpoint / IMAP 的 live 反向解析、配置来源、占用大小、IMAP 手动清理等。

6. `webui-real`
   - 覆盖 HTTP/WebUI 配置与代理访问。
   - 需要验证开启 HTTP 后 WebUI 代理路由、详情页嵌入、新窗口打开，以及未开启时的提示。

### C. 增量选择
- 除 smoke 外，真实环境 suite 也要支持增量触发。
- 选择器或等价逻辑需要输出 suite 级布尔开关，例如：
  - `run_installer_real`
  - `run_upgrade_real`
  - `run_config_real`
  - `run_plugin_real`
  - `run_storage_real`
  - `run_webui_real`
- 这些开关必须由仓库改动路径驱动，而不是靠手工记忆。

### D. 资源与环境原则
- CI 中默认只清理容器/进程，不强制 `rm -rf` 本地临时目录。
- 本地开发环境仍保留自动清理能力。
- 每个真实 suite 使用独立临时目录前缀，避免环境相互污染。
- 重型真实 suite 先只跑 x64；smoke 继续保持 x64 / arm64。

### E. 验证要求
- 每个真实 suite 都必须验证“最终产物”，不能只看 UI 成功提示。
- 最终产物验证优先包括：
  - 配置文件内容
  - 插件/依赖目录结果
  - 已安装记录 / runtime-storage 解析结果
  - 代理/HTTP 可达性
- 能复用已有 harness 的，不重复造新的启动链路。

## Proposed Suite Matrix

| suite | 目标 | 依赖能力 | 核心断言 | 建议触发改动 |
| --- | --- | --- | --- | --- |
| `installer-real` | 真实安装 | backend + frontend + agent (+ 可选 minio) | 安装成功、配置文件正确 | `internal/apps/installer/**`, `agent/internal/installer/**`, `frontend/components/common/installer/**`, `frontend/hooks/use-installer.ts`, `config.e2e.installer-real.yaml`, `config.e2e.agent-real.yaml` |
| `upgrade-real` | 真实升级 | backend + frontend + agent + 本地已安装集群 | 升级步骤、版本切换、connectors/lib/plugins 结果、升级后 smoke | `internal/apps/stupgrade/**`, `frontend/components/common/cluster/upgrade/**`, `frontend/lib/services/st-upgrade/**`, `internal/apps/plugin/**`, `agent/internal/installer/**` |
| `config-real` | 配置管理 | backend + frontend + agent + live config 文件 | 智能修复、同步到节点、从节点同步、历史版本预览/对比/回滚 | `internal/apps/config/**`, `frontend/components/common/cluster/ClusterConfigs.tsx`, `frontend/lib/services/config/**` |
| `plugin-real` | 插件市场/本地插件 | backend + frontend + agent + 本地插件缓存 | connectors/plugins/lib/metadata、profile、集群插件详情 | `internal/apps/plugin/**`, `frontend/components/common/plugin/**`, `frontend/components/common/cluster/ClusterPlugins.tsx`, `frontend/lib/services/plugin/**` |
| `storage-real` | 运行时存储详情 | backend + frontend + agent (+ 可选 minio) | runtime-storage live 反向解析、占用、IMAP cleanup | `internal/apps/cluster/runtime_storage*`, `frontend/components/common/cluster/ClusterDetail.tsx`, `frontend/lib/services/cluster/**`, `agent/internal/installer/**` |
| `webui-real` | HTTP / WebUI | backend + frontend + agent + SeaTunnel HTTP upstream | WebUI 代理、嵌入访问、新窗口打开、未开启提示 | `internal/apps/cluster/**webui*`, `internal/seatunnel/**`, `frontend/components/common/cluster/ClusterDetail.tsx`, `agent/internal/installer/**` |

## Incremental Strategy

### Smoke
- 保持当前 `select-e2e-specs.mjs` 的增量 spec 选择逻辑。

### Real Suites
- 在统一选择器或统一 scope job 中新增 suite 级输出：
  - `run_installer_real`
  - `run_upgrade_real`
  - `run_config_real`
  - `run_plugin_real`
  - `run_storage_real`
  - `run_webui_real`
- PR 场景：根据文件改动自动决定是否跑对应 suite。
- `workflow_dispatch` / `schedule`：支持 `all` 或按 suite 手动选择。

### Trigger Principles
- 只要改动命中某个真实 suite 的关键目录，就自动触发该 suite。
- 如果改动的是共享底层，例如：
  - `agent/internal/installer/**`
  - `internal/apps/installer/**`
  - `frontend/playwright.config.ts`
  - `frontend/scripts/e2e/**`
  则允许同时触发多个真实 suite。

## Acceptance Criteria

- [ ] 形成统一的真实环境 E2E suite 矩阵（installer / upgrade / config / plugin / storage / webui）。
- [ ] 形成 suite 级增量触发规则，并能清晰映射到代码目录。
- [ ] 新增真实 suite 时，不需要再创建新的 workflow，只需要扩展统一的 `E2E` workflow。
- [ ] 本地与 CI 的资源清理策略明确：本地可清理，CI 默认不做破坏性目录删除。
- [ ] 未来每个真实 suite 都有明确的“最终产物”断言要求。

## Out of Scope

- 本期不实现所有 suite，只完成设计与触发矩阵。
- 本期不做多节点真实集群 E2E。
- 本期不做深度 Java probe / 对象存储真实鉴权 E2E。
- 本期不要求 arm64 运行重型真实 suite。

## Recommended Execution Order

1. `upgrade-real`
2. `config-real`
3. `plugin-real`
4. `storage-real`
5. `webui-real`

## Technical Notes

- 现有真实 installer harness：`frontend/scripts/e2e/run-real-installer.sh`
- 现有真实安装 spec：`frontend/e2e/install-wizard-real.spec.ts`
- 现有统一 workflow：`.github/workflows/e2e.yml`
- 现有增量选择器：`frontend/scripts/e2e/select-e2e-specs.mjs`
- 后续建议把“suite scope 决策”也统一收口到脚本，而不是散落在 workflow shell 里。
