# installer real e2e

## Goal

为 STX 一键安装新增一套真实端到端（E2E）验证：能够联网拉取指定 SeaTunnel 版本，跑通完整安装流程，并验证最终生成的配置文件、插件目录与依赖目录是否符合预期；同时保证本地运行会清理临时资源，并支持在 GitHub CI 环境中稳定执行。

## What I already know

* 当前一键安装已具备真实在线安装链路：控制面可下载指定 SeaTunnel 安装包，再传输给 Agent。
* Agent 当前会真实改写 `seatunnel.yaml`、`hazelcast*.yaml`、`hazelcast-client.yaml`、`log4j2.properties`，并支持 checkpoint / IMAP / HTTP / Job Log Mode / 插件安装。
* 前端已有 `frontend/e2e/install-wizard-template.spec.ts` 与 `e2e-lab/install-wizard` 页面，但当前更多是基于路由夹具的模板，不是真实安装 E2E。
* 真实远端存储可用 MinIO 模拟 S3。
* 用户要求本地运行后及时清理临时资源，并确保 GitHub CI 也能跑通。

## Requirements

- 新增一套真实安装 E2E，可指定 SeaTunnel 版本（先覆盖 2.3.13）。
- E2E 需覆盖：在线下载、安装向导/API 发起安装、Agent 执行、最终文件断言。
- 至少覆盖以下配置结果：
  - `seatunnel.yaml` 中 checkpoint 与 HTTP 配置
  - `hazelcast.yaml` 中 IMAP map-store 配置
  - `hazelcast-client.yaml` 基础连通配置
  - `log4j2.properties` 日志模式
- 至少覆盖一个包含远端对象存储（MinIO）的场景。
- 本地运行需要自动创建并清理临时目录、测试 bucket、测试安装目录。
- 设计需兼容 GitHub CI，避免依赖手工环境。
- E2E 应优先复用当前真实安装链路，不额外造新流程。

## Acceptance Criteria

- [ ] 可通过脚本或测试命令拉起真实 E2E 环境并运行。
- [ ] E2E 可指定 SeaTunnel 版本并真实下载安装包。
- [ ] E2E 安装完成后能断言关键配置文件内容。
- [ ] E2E 运行结束后能清理本地产生的临时资源。
- [ ] GitHub CI 环境中可运行至少一套精简版真实安装 E2E。

## Out of Scope

- 不要求本期覆盖所有 SeaTunnel 版本。
- 不要求本期覆盖所有插件组合。
- 不要求本期实现真实对象存储的深度鉴权探针。

## Technical Notes

- 重点文件：
  - `frontend/e2e/*`
  - `frontend/app/(main)/e2e-lab/install-wizard/page.tsx`
  - `internal/apps/installer/service.go`
  - `agent/internal/installer/manager.go`
  - `.github/workflows/*`
- 需要重点考虑资源清理：安装目录、下载缓存、MinIO bucket、临时配置文件。
