# STX

**SeaTunnel 一站式运维平台** — 让 SeaTunnel 运维不再黑箱。

[English](./README.md) · [二次开发](./docs/二次开发.md) · [快速开始](./docs/00-快速开始.md)

## 为什么做 STX

线上跑 SeaTunnel，很少只是「提交一个任务」就结束。团队还要管主机监控、集群重启、集群安装升级、连接器分发、配置变更、运行诊断、Checkpoint 排查，以及任务故障后的恢复。

STX 就是围绕这些事的控制面：把主机、集群、安装包、插件、监控、诊断和任务操作收敛到一个统一入口。并原生提供 AI Agent 智能运维入口（CLI + Skill）。

## 能力一览

- **主机与 Agent** — 主机接入、Agent 安装、心跳与容量
- **集群生命周期** — 创建、部署、启停、升级 SeaTunnel 集群
- **安装包与插件** — 包管理、连接器市场安装
- **可观测与诊断** — 监控中心、巡检、异常线索、恢复动作
- **任务工作台** — HOCON/DAG 流程、Checkpoint 可视化、可调试运行
- **AI Agent 入口** — 面向 CLI + Skill 的智能运维入口（建设中）
- **认证与审计** — 账密登录、可选 GitHub/Google OAuth、审计日志、用户管理



## 界面截图


| 页面   | 预览                                           |
| ---- | -------------------------------------------- |
| 登录   | ![登录](docs/screenshots/00-login.png)         |
| 控制台  | ![控制台](docs/screenshots/01-dashboard.png)    |
| 工作台  | ![工作台](docs/screenshots/02-workbench.png)    |
| 主机管理 | ![主机管理](docs/screenshots/03-hosts.png)       |
| 集群管理 | ![集群管理](docs/screenshots/04-clusters.png)    |
| 监控中心 | ![监控中心](docs/screenshots/05-monitoring.png)  |
| 诊断中心 | ![诊断中心](docs/screenshots/06-diagnostics.png) |
| 安装包  | ![安装包](docs/screenshots/07-packages.png)     |
| 插件市场 | ![插件市场](docs/screenshots/08-plugins.png)     |
| 数据同步 | ![数据同步](docs/screenshots/10-sync.png)        |
| 用户中心 | ![用户中心](docs/screenshots/11-user-center.png) |




## 架构

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  前端           │     │  后端           │     │  数据库         │
│  Next.js        │◄───►│  Go (Gin)       │◄───►│  SQLite/MySQL/  │
│  React 19       │     │  GORM + gRPC    │     │  PostgreSQL     │
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  STX Agent      │
                        │  部署在目标主机 │
                        └─────────────────┘
```



## 快速开始



### 环境要求

- Go >= 1.24
- Node.js >= 18
- pnpm >= 8（推荐）



### 本地启动

```bash
git clone https://github.com/LeonYoah/stx.git
cd stx
cp config.example.yaml config.yaml

go mod tidy
go run main.go api
```

另开终端：

```bash
cd frontend
pnpm install
pnpm dev
```

浏览器打开 `http://localhost:3000`，默认账号 `admin` / `admin123`（或 `config.yaml` 中的配置）。

### 离线安装包

```bash
scripts/package-release.sh \
  --arch amd64 \
  --bundle-observability without \
  --node-major 18 \
  --node-variant glibc217

tar -xzf stx-<version>-linux-amd64-node18-glibc217-without-observability.tar.gz
cd stx-<version>-linux-amd64-node18-glibc217-without-observability
sudo ./install.sh
```

更多部署说明见 [docs/00-快速开始.md](./docs/00-快速开始.md)、[docs/打包发布说明.md](./docs/打包发布说明.md)。

## 关键配置


| 配置项                           | 说明            | 默认值                     |
| ----------------------------- | ------------- | ----------------------- |
| `auth.default_admin_username` | 初始管理员用户名      | `admin`                 |
| `auth.default_admin_password` | 初始管理员密码       | `admin123`              |
| `database.type`               | 数据库类型         | `sqlite`                |
| `app.addr`                    | HTTP 监听地址     | 见 `config.example.yaml` |
| `grpc.port`                   | Agent gRPC 端口 | 见 `config.example.yaml` |


可选在 `oauth_providers` 下启用 GitHub / Google OAuth，回调地址一般为 `http://localhost:3000/callback`。

## 文档

- [用户快速开始](./docs/00-快速开始.md)
- [二次开发](./docs/二次开发.md)
- [打包发布](./docs/打包发布说明.md)
- [可观测性接入](./docs/可观测性三件套一键接入说明.md)



## 许可证

Apache License 2.0，见 [LICENSE](./LICENSE)。

本项目最初基于 [linux-do/cdk](https://github.com/linux-do/cdk)（MIT）改造。

## 相关链接

- [Apache SeaTunnel](https://seatunnel.apache.org/)
- [STX GitHub](https://github.com/LeonYoah/stx)

