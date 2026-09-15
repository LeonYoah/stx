# STX

**One-stop SeaTunnel ops platform** — make SeaTunnel ops no longer a black box.

[中文文档](./README_CN.md) · [Secondary development](./docs/secondary-development.md) · [Quick start (CN)](./docs/00-快速开始.md)

## Why STX

Running SeaTunnel in production is rarely “just submit a job”. Teams also need host monitoring, cluster restart, install and upgrade, connector distribution, config changes, runtime diagnosis, Checkpoint inspection, and recovery after job failures.

STX is the control plane for that work: one place to manage hosts, clusters, packages, plugins, monitoring, diagnostics, and job operations — with a native AI Agent ops entry (CLI + Skill).

## Highlights

- **Host & Agent** — host onboarding, Agent install, heartbeat and capacity
- **Cluster lifecycle** — create, deploy, start/stop, and upgrade SeaTunnel clusters
- **Packages & plugins** — package management and connector marketplace installs
- **Observability & diagnosis** — monitoring center, inspections, error clues, recovery actions
- **Job workbench** — HOCON/DAG workflows, Checkpoint visualization, debuggable runs
- **AI Agent entry** — CLI + Skill oriented intelligent ops entry (in progress)
- **Auth & audit** — password login, optional GitHub/Google OAuth, audit logs, user admin

## Screenshots

| Page | Preview |
| --- | --- |
| Login | ![Login](docs/screenshots/00-login.png) |
| Dashboard | ![Dashboard](docs/screenshots/01-dashboard.png) |
| Workbench | ![Workbench](docs/screenshots/02-workbench.png) |
| Hosts | ![Hosts](docs/screenshots/03-hosts.png) |
| Clusters | ![Clusters](docs/screenshots/04-clusters.png) |
| Monitoring | ![Monitoring](docs/screenshots/05-monitoring.png) |
| Diagnostics | ![Diagnostics](docs/screenshots/06-diagnostics.png) |
| Packages | ![Packages](docs/screenshots/07-packages.png) |
| Plugins | ![Plugins](docs/screenshots/08-plugins.png) |
| Data Sync | ![Data Sync](docs/screenshots/10-sync.png) |
| User Center | ![User Center](docs/screenshots/11-user-center.png) |

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Frontend       │     │  Backend        │     │  Database       │
│  Next.js        │◄───►│  Go (Gin)       │◄───►│  SQLite/MySQL/  │
│  React 19       │     │  GORM + gRPC    │     │  PostgreSQL     │
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  STX Agent      │
                        │  on each host   │
                        └─────────────────┘
```

## Quick start

### Requirements

- Go >= 1.24
- Node.js >= 18
- pnpm >= 8 (recommended)

### Run locally

```bash
git clone https://github.com/LeonYoah/stx.git
cd stx
cp config.example.yaml config.yaml

go mod tidy
go run main.go api
```

In another terminal:

```bash
cd frontend
pnpm install
pnpm dev
```

Open `http://localhost:3000` and sign in with `admin` / `admin123` (or the values in `config.yaml`).

### Offline install (release bundle)

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

See [docs/00-快速开始.md](./docs/00-快速开始.md) and [docs/打包发布说明.md](./docs/打包发布说明.md) for deployment details.

## Configuration essentials

| Key | Purpose | Default |
| --- | --- | --- |
| `auth.default_admin_username` | Initial admin username | `admin` |
| `auth.default_admin_password` | Initial admin password | `admin123` |
| `database.type` | Database type | `sqlite` |
| `app.addr` | HTTP listen address | see `config.example.yaml` |
| `grpc.port` | Agent gRPC port | see `config.example.yaml` |

Optional OAuth providers (`github` / `google`) can be enabled under `oauth_providers` in `config.yaml`. Callback URL is typically `http://localhost:3000/callback`.

## Docs

- [Quick start (users)](./docs/00-快速开始.md)
- [Secondary development](./docs/secondary-development.md)
- [Packaging & release](./docs/打包发布说明.md)
- [Observability integration](./docs/可观测性三件套一键接入说明.md)

## License

Apache License 2.0. See [LICENSE](./LICENSE).

This project was originally adapted from [linux-do/cdk](https://github.com/linux-do/cdk) (MIT).

## Links

- [Apache SeaTunnel](https://seatunnel.apache.org/)
- [STX on GitHub](https://github.com/LeonYoah/stx)
