# STX Docker 全量（无需克隆仓库）

发版产物：`stx-docker-compose.tar.gz`（`package-release` + `release-on-tag` 已接入；**下次打版本 tag 后** `latest/download` 才有。当前若 404，用仓库里的 `deploy/docker/` 或等新 tag）。

```bash
mkdir -p stx-docker && cd stx-docker
curl -fsSL https://github.com/LeonYoah/stx/releases/latest/download/stx-docker-compose.tar.gz | tar -xz
cd docker
cp config.example.yaml config.yaml
# 按所选 Compose 改 database 段
docker compose up -d
```

国内：把下载链接贴到 https://gh-proxy.com/ ，或：

```bash
curl -fsSL https://v4.gh-proxy.org/https://github.com/LeonYoah/stx/releases/latest/download/stx-docker-compose.tar.gz | tar -xz
```

| 数据库 | 启动 |
| --- | --- |
| MySQL（默认） | `docker compose up -d`（`config.yaml` 里 `type: mysql` / `host: mysql`） |
| SQLite | `docker compose -f docker-compose.sqlite.yml up -d`（`type: sqlite`） |
| PostgreSQL | `docker compose -f docker-compose.postgres.yml up -d`（`type: postgres` / `host: postgres`） |

| 服务 | 地址 |
| --- | --- |
| 控制台 | http://127.0.0.1:17880 |
| API | http://127.0.0.1:17800 |
| Grafana | http://127.0.0.1:3000 |
