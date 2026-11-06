# STX Docker 全量（无需克隆仓库）

发版产物：`stx-docker-compose.tar.gz`。

```bash
mkdir -p stx-docker && cd stx-docker
# 中国
curl -fsSL https://v4.gh-proxy.org/https://github.com/LeonYoah/stx/releases/latest/download/stx-docker-compose.tar.gz | tar -xz
# GitHub 直连：把上面 URL 里的 https://v4.gh-proxy.org/ 去掉
cd docker
# SeaTunnel 不在本机时：改 config.yaml 的 app.external_url / app.console_url 为其他机器能访问的安装机 IP
mkdir -p data
docker compose up -d
```

中国拉应用镜像（华为云 SWR）：

```bash
cp .env.cn.example .env
docker compose up -d
```

| 数据库 | 启动 |
| --- | --- |
| MySQL（默认） | `docker compose up -d` |
| SQLite | `docker compose -f docker-compose.sqlite.yml up -d` |
| PostgreSQL | `docker compose -f docker-compose.postgres.yml up -d` |

库类型由对应 compose 注入，不必改 `config.yaml` 的 database 段。

宿主机只暴露：

| 服务 | 地址 |
| --- | --- |
| Web UI | http://127.0.0.1:17880 |
| API | http://127.0.0.1:17800 |
| gRPC | 17890 |

监控在 Web UI 内，不单独映射 3000/9090/9093。

`data/` 已 gitignore，备份拷整个 `data/` 即可。
