# Design: 在线 / 离线安装与发布依赖拆分

## 1. 决策摘要

| 决策 | 结论 |
|------|------|
| stx / stx-agent 形态 | **裸二进制** + 同名 `.sha256`，不打成 tar.gz |
| frontend | 仍属产品代码，随 stx **版本**发布；可为 `frontend-standalone-$ver-$os-$arch.tar.gz`（多文件必须打包）或随安装器从固定路径展开 |
| Node / 三件套 | **独立 deps**，GitHub 只存一份；安装时解析获取 |
| 在线 vs 离线 | 同一 `install-core`；在线多下载层，离线用本地 `packages/` |
| 国内下载 | region 判定 + gh-proxy **五节点测速** |
| systemd | 对齐 agent（root system / 非 root user） |
| all-in-one | **不带监控**，文档说明 |

## 2. Release 资产模型

### 2.1 每版本上传（`vX.Y.Z`）

```text
stx-linux-amd64
stx-linux-amd64.sha256
stx-linux-arm64
stx-linux-arm64.sha256
stx-agent-linux-amd64
stx-agent-linux-amd64.sha256
stx-agent-linux-arm64
stx-agent-linux-arm64.sha256
frontend-standalone-vX.Y.Z-linux-amd64.tar.gz    # 可选拆分；或由 CI 附带
frontend-standalone-vX.Y.Z-linux-amd64.tar.gz.sha256
install-online.sh                                 # 或指向 raw main + 版本 pin
SHA256SUMS
```

说明：

- **不要**再发「巨型 `stx-…-with-observability.tar.gz`」作为唯一形态。
- 若需兼容旧文档，可保留「聚合 offline bundle」由 `download-bundle.sh` 现场组装，而不是 CI 每次打全量。

### 2.2 deps 独立 Release（建议 tag：`deps` 或 `deps-YYYY.MM`）

```text
node-22-official-linux-amd64.tar.gz
node-22-glibc217-linux-amd64.tar.gz
node-18-glibc217-linux-amd64.tar.gz   # CentOS 7 推荐
observability-prom3.9.1-am0.31.1-gf12.3.3-linux-amd64.tar.gz
… arm64 对称 …
*.sha256
MANIFEST.json   # 组件版本、适用 glibc、最低 node
```

版本不变则 **不重新上传**；stx 发版 MANIFEST 只引用 deps 的 tag/文件名。

### 2.3 为何 frontend 仍可能是 tar.gz

Next standalone 是目录树（`server.js` + `.next` + …），不是单文件；**只有单文件二进制才坚持裸传**。安装后落到 `/opt/stx/frontend/`。

## 3. 安装后目录（运行时）

```text
/opt/stx/
  stx                          # 裸二进制
  bin/
    start.sh stop.sh status.sh
    install-core 相关辅助可内嵌
  frontend/                    # standalone 展开
  config.yaml
  data/ logs/ run/ certs/
  runtime/node/                # 可选；本机 node 足够则可缺省或 symlink
  deps/                        # 可选；仅 bundled 监控时存在
  lib/agent/                   # 可选下发用 agent 二进制缓存
```

相对 Doris：无 `static/`、无 `jdk/`；前端是 Node 进程，不是静态目录。

## 4. 在线 / 离线流程

```text
                    ┌─────────────────────┐
                    │  region + mirror    │
                    │  (仅在线)            │
                    └─────────┬───────────┘
                              ▼
┌──────────────┐     ┌────────────────┐     ┌─────────────────┐
│ online.sh    │────▶│ fetch assets   │────▶│ install-core    │
│ curl \| bash  │     │ stx+frontend   │     │ 端口/配置/systemd│
└──────────────┘     │ + optional deps│     └─────────────────┘
                     └────────────────┘              ▲
┌──────────────┐     ┌────────────────┐              │
│ download-    │────▶│ offline bundle │──拷贝────────┘
│ bundle.sh    │     │ packages/*     │  离线机 install.sh
└──────────────┘     └────────────────┘
```

### 4.1 在线

1. 探测 `arch`、glibc（→ node variant）、端口空闲集  
2. `STX_DOWNLOAD_REGION` 或连通性判定 `cn|global`  
3. `cn`：对五节点测速，选定 prefix  
4. 下载 stx、frontend、（缺则）node、（若 `--with-observability`）observability  
5. 本机已有合格 `node` / 包管理可装且版本 ≥ 18.18 → 可跳过 node 包  
6. `install-core`

### 4.2 离线

**有网机：**

```bash
./download-bundle.sh --version vX.Y.Z --arch amd64 --node-variant glibc217 --with-observability
```

产出：

```text
stx-offline-bundle-vX.Y.Z-linux-amd64/
  MANIFEST.json
  SHA256SUMS
  packages/
    stx-linux-amd64
    stx-agent-linux-amd64          # 可选
    frontend-standalone-….tar.gz
    node-….tar.gz                  # 可选
    observability-….tar.gz         # 可选
  install.sh                       # 强制 --offline，拒绝外网
  README-OFFLINE.md
```

**离线机：** `./install.sh` → 只读 `packages/` + checksum → `install-core`。

## 5. 国内加速（gh-proxy）

前缀：`https://<node>/https://github.com/...`

节点（可配置覆盖）：

```text
https://gh-proxy.org
https://v4.gh-proxy.org
https://v6.gh-proxy.org
https://cdn.gh-proxy.org
https://axisnow.gh-proxy.org
```

测速：对各节点 `HEAD`/`GET` 小文件（如 `SHA256SUMS`），超时 3～5s，选 TTFB 最低且 2xx；下载失败换次优。

环境变量：

| 变量 | 含义 |
|------|------|
| `STX_DOWNLOAD_REGION=cn\|global` | 强制地区 |
| `STX_DOWNLOAD_MIRROR_PREFIX` | 自定义前缀（自建镜像），跳过公共测速 |
| `STX_GH_PROXY_NODES` | 逗号分隔节点列表 |

## 6. 依赖解析顺序

**Node：**

1. `PATH` 上 `node` 且 `>= 18.18`  
2. `$INSTALL_DIR/runtime/node`  
3. 离线 `packages/node-*.tar.gz` 或在线拉 deps  
4. `yum|dnf|apt|apk` 安装后再次验版本；失败则回退 3  

**Observability：**

1. 用户已有外部栈 → 写 `observability.mode=remote` + URL  
2. 本地/下载 bundled deps → `mode=bundled` + 启停脚本  
3. 明确 `--without-observability` → 关闭  

## 7. install-core 行为

- 端口：默认 frontend `17880`、HTTP `17800`、gRPC `17890`（避开 80/8000/9000）；占用则 `+1` 直至上限（如 +20）  
- 写 `config.yaml`（从 example + 探测结果）；保留已有配置除非 `--force`  
- 若 bundled：跑 `init-observability-defaults`，写入本地三件套 URL，`enabled=true`  
- systemd：`stx.service`（可 ExecStart=`bin/start.sh`）；对齐 agent 的 root/user 路径  
- 收尾打印：访问 URL、配置路径、`systemctl restart stx`、`bin/status.sh`  

## 8. 脚本边界（少而清晰）

| 脚本 | 职责 |
|------|------|
| `install-online.sh` | 在线入口：region、测速、下载、调 core |
| `download-bundle.sh` | 有网机组离线包 |
| `install.sh` | 包内/离线入口 → core（`--offline` 禁网） |
| `lib/download.sh` | URL 拼装、测速、校验（被 source） |
| `bin/start\|stop\|status.sh` | 进程/三件套启停（已有，增强即可） |

## 9. Docker

- `stx-all-in-one`：仅 backend + frontend + 镜像内 Node；**无** Prom/GF/AM  
- 文档：`docker run` 试用；监控请用外部栈或二进制 bundled/Compose profile  

## 10. 与现有代码关系

- 演进自：`scripts/package-release.sh`、`support-files/release/*`、`deps/*-observability.sh`  
- 目标：package-release **默认只产二进制 + frontend tarball**；obs/node 改为独立 job/手动 deps release  
- agent 安装脚本中的 systemd 模式作为 CP systemd 参考实现  

## 11. 实现分期（建议）

1. **Spec 落地**（本任务）：prd + design + `.trellis/spec/infra/packaging-and-install.md`  
2. **P0**：资产命名/CI 改裸二进制；`lib/download.sh` + online/offline 骨架；install-core 端口与配置  
3. **P1**：systemd；deps 独立 release；瘦身 package-release  
4. **P2**：Compose 可选监控文档；旧 tar.gz 聚合形态弃用说明  
