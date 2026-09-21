# STX Docker 全量（无需克隆仓库）

发版产物：`stx-docker-compose.tar.gz`（`package-release` + `release-on-tag` 已接入；**下次打版本 tag 后** `latest/download` 才有。当前若 404，用仓库里的 `deploy/docker/` 或等新 tag）。

```bash
mkdir -p stx-docker && cd stx-docker
curl -fsSL https://github.com/LeonYoah/stx/releases/latest/download/stx-docker-compose.tar.gz | tar -xz
cd docker
cp config.example.yaml config.yaml
# 按所选 Compose 改 database 段
mkdir -p data
docker compose up -d
```

中国：把下载链接贴到 https://gh-proxy.com/ ，或：

```bash
curl -fsSL https://v4.gh-proxy.org/https://github.com/LeonYoah/stx/releases/latest/download/stx-docker-compose.tar.gz | tar -xz
```

中国拉应用镜像（华为云 SWR，组织 `stx`）：

```bash
cp .env.cn.example .env
# 或编辑 .env：STX_IMAGE_REGISTRY=swr.cn-east-3.myhuaweicloud.com/stx
docker compose up -d
```

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `STX_IMAGE_REGISTRY` | `ghcr.io/leonyoah` | 镜像前缀；中国用 `swr.cn-east-3.myhuaweicloud.com/stx` |
| `STX_IMAGE_TAG` | `latest` | 镜像 tag |

| 数据库 | 启动 |
| --- | --- |
| MySQL（默认） | `mkdir -p data && docker compose up -d`（`config.yaml` 里 `type: mysql` / `host: mysql`） |
| SQLite | `mkdir -p data && docker compose -f docker-compose.sqlite.yml up -d`（`type: sqlite`） |
| PostgreSQL | `mkdir -p data && docker compose -f docker-compose.postgres.yml up -d`（`type: postgres` / `host: postgres`） |

## 数据目录

持久化一律用相对路径绑定到 `./data/`（不用 Docker named volume）：

```text
./data/stx/           # 控制面数据
./data/stx-lib/       # 控制面 lib
./data/prometheus/
./data/alertmanager/
./data/grafana/
./data/mysql/         # 仅 MySQL compose
./data/postgres/      # 仅 Postgres compose
```

`data/` 已 gitignore，备份/迁移直接拷整个 `data/` 目录即可。

| 服务 | 地址 |
| --- | --- |
| 控制台 | http://127.0.0.1:17880 |
| API | http://127.0.0.1:17800 |
| Grafana | http://127.0.0.1:3000 |
| Prometheus | http://127.0.0.1:9090 |
| Alertmanager | http://127.0.0.1:9093 |

## 监控配置（单源）

`observability/` 下 YAML **由模板生成**，不要手改。唯一真相：`install/observability/`。

重新生成：

```bash
# 在仓库根目录
./install/observability/render.sh docker deploy/docker/observability
```

`package-release` 打 `stx-docker-compose.tar.gz` 时会自动执行上述渲染。
