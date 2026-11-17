# 打包与安装契约

> 适用于发布资产、在线/离线安装脚本、控制面 systemd 与 Node deps。监控内置在 stx 中（见 `docs/内置监控说明.md`）。  
> 详细产品背景见任务 `.trellis/tasks/09-18-online-offline-install-packaging/`。

---

## 1. Scope / Trigger

在以下改动时必须遵守本文：

- 修改 `install/package-release.sh`、`install/bin/*`、`install/install*.sh`
- 新增/修改在线安装、离线 bundle、下载测速逻辑
- 变更 GitHub Release 资产命名或 deps 复用策略
- 控制面 systemd 与 `stx-agent` 安装脚本对齐

---

## 2. Signatures（资产与脚本）

### 2.1 版本 Release 资产（裸二进制）

| 资产 | 形态 | 说明 |
|------|------|------|
| `stx-linux-<arch>` | 可执行文件 | **禁止**再要求唯一交付为整包 tar.gz |
| `stx-linux-<arch>.sha256` | 文本 | 与二进制配对 |
| `stx-agent-linux-<arch>` | 可执行文件 | 同上 |
| `stx-agent-linux-<arch>.sha256` | 文本 | 同上 |
| `frontend-standalone-<ver>-linux-<arch>.tar.gz` | tar.gz | 多文件唯一例外（Next standalone） |

`<arch>` ∈ `amd64` \| `arm64`。

控制面一键安装目标 OS：Ubuntu / Debian / Rocky / Alma / CentOS Stream / RHEL 8+（amd64、arm64），以及 CentOS 7 **仅 amd64**（glibc217 Node）。**CentOS 7 arm64 不支持**（无对应 Node 构建）。macOS / Windows 不是一键安装目标。

### 2.2 deps Release 资产（独立、可复用）

| 资产模式 | 示例 |
|----------|------|
| Node | `node-<major>-<official\|glibc217>-linux-<arch>.tar.gz` |
| 清单 | `MANIFEST.json`（Node 版本、最低 node、适用 glibc） |

存放位置是 GitHub Release tag **`deps`**，不是每个 `v*`。版本工作流 `release-on-tag.yml` 不得传 `--emit-deps`。只有 Node 版本变化时，手动跑 `release-deps.yml`。该 Release 必须 `--latest=false`。
deps 上旧版本留下的 `observability-*` 资产与 MANIFEST 条目保持原样（老版本安装仍会用到），发布脚本只替换本次重建的资产，不删除它们。

### 2.3 用户入口脚本

```bash
# 在线
curl -fsSL <raw-or-mirrored-url>/install-online.sh | bash -s -- [options]

# 离线打包（有网机）
./download-bundle.sh --version vX.Y.Z --arch amd64 [--node-variant glibc217]

# 安装（在线解压后或离线 bundle 内）
./install.sh [--install-dir /opt/stx] [--offline] [--without-observability] [--no-start]
```

### 2.4 SeaTunnel 离线资产包（控制面本地仓导出）

有网机在 STX 控制面将本地 `packages_dir` + `plugins_dir/{version}` 打成可带走的 tar.gz：

| API | 说明 |
|------|------|
| `POST /api/v1/packages/offline-bundles` | 按版本 create（要求本地 bin 已存在） |
| `GET /api/v1/packages/offline-bundles` | 列出已生成包 |
| `GET /api/v1/packages/offline-bundles/:name/download` | 下载 tar.gz |
| `POST /api/v1/packages/offline-bundles/import` | 无网机导入回本地仓 |
| `DELETE /api/v1/packages/offline-bundles/:name` | 删除已生成包 |

包内布局：`MANIFEST.json` + `packages/` + `plugins/{version}/`。UI 入口：安装包管理页「离线导入 / 导出」（导入按文件名分流资产包 vs 官方 bin；导出一键打包）。

---

## 3. Contracts

### 3.1 环境变量

| 变量 | 默认 | 含义 |
|------|------|------|
| `STX_DOWNLOAD_REGION` | 自动 | `cn` \| `global` |
| `STX_DOWNLOAD_MIRROR_PREFIX` | 空 | 非空则直接前缀 GitHub URL，跳过公共测速 |
| `STX_GH_PROXY_NODES` | 见下表 | 逗号分隔 |
| `STX_INSTALL_DIR` | `/opt/stx` | 安装根目录 |
| `STX_IMAGE_REGISTRY` | `ghcr.io/leonyoah` | Docker Compose 镜像前缀；中国华为云用 `swr.cn-east-3.myhuaweicloud.com/stx` |
| `STX_IMAGE_TAG` | `latest` | Docker Compose / 单镜像 tag |
| `STX_CONFIG_FILE` | 仓库 `../../config.example.yaml`；发版 tar `./config.yaml` | Compose 挂载到容器的服务端配置；勿另维护 Docker schema |

### 3.2 gh-proxy 节点（中国默认）

```text
https://gh-proxy.org
https://v4.gh-proxy.org
https://v6.gh-proxy.org
https://cdn.gh-proxy.org
https://axisnow.gh-proxy.org
```

URL 拼装：

```text
${MIRROR_PREFIX}/https://github.com/${OWNER}/${REPO}/releases/download/${TAG}/${ASSET}
```

### 3.3 运行时目录契约

```text
$INSTALL_DIR/stx
$INSTALL_DIR/frontend/server.js
$INSTALL_DIR/config.yaml
$INSTALL_DIR/bin/{start,stop,status}.sh
$INSTALL_DIR/scripts/stx-java-proxy.sh  # required; served via GET /api/v1/agent/assets/stx-java-proxy.sh
$INSTALL_DIR/lib/stx-java-proxy-v2.jar  # required; GET .../assets/stx-java-proxy-v2.jar and ?version=v2
$INSTALL_DIR/lib/stx-java-proxy-v3.jar  # required; GET .../assets/stx-java-proxy-v3.jar and ?version=v3
$INSTALL_DIR/runtime/node/bin/node      # optional
$INSTALL_DIR/data/tsdb                  # 内置监控数据（observability.storage.dir 默认值）
$INSTALL_DIR/observability/             # 仅旧版本升级残留：安装脚本只提示、不删除；stop.sh 停掉其中有 pid 文件的旧进程
```

离线 `packages/` 布局必须含 `stx-java-proxy-v2.jar` / `stx-java-proxy-v3.jar`；`install.sh` 缺 jar 或脚本时必须失败，不得静默跳过（DEF-R2-1）。

`bin/start.sh` / `stop.sh` 只启停 STX 前后端；旧的 `--observability <mode>` 参数接受并忽略（旧 systemd 单元可能还带着）。
`install.sh` / `install-online.sh` 不再接受旧的 `--with-observability`、`--prometheus-port`、`--alertmanager-port`、`--grafana-port`（按 unknown option 报错退出；属破坏性升级，旧命令需删参）。
### 3.4 配置键（安装器写入）

- 默认端口：前端 `17880`、HTTP API `17800`、gRPC `17890`（避开常见 80/8000/9000）
- `app.addr` / gRPC 端口：端口探测结果  
- `app.external_url`：本机可达 URL  
- `observability.enabled`：默认 `true`（内置监控）；`--without-observability` 写 `false`  
- **不要**写入 `observability.prometheus` / `alertmanager` / `grafana` 段（已删除；旧文件残留会被忽略）
- Docker Compose 包：`config.yaml` 从仓库根目录 `config.example.yaml` 拷出；Compose 用 `STX_DATABASE_*` 覆盖容器 DNS，不另维护一份 Docker schema  

### 3.5 systemd（对齐 agent）

| 身份 | unit 路径 |
|------|-----------|
| root | `/etc/systemd/system/stx.service` |
| 非 root | `${HOME}/.config/systemd/user/stx.service` |

`ExecStart` 推荐指向 `$INSTALL_DIR/bin/start.sh`（或等价 wrapper）。改配置后：`systemctl restart stx`（user 则 `systemctl --user restart stx`）。

---

## 4. Validation & Error Matrix

| 场景 | 期望 |
|------|------|
| sha256 不匹配 | 安装中止，保留已下文件到缓存目录并提示 |
| `--offline` 仍缺 packages 内文件 | 中止，列出缺失资产名 |
| `--offline` 尝试访问外网 | 禁止；报错 |
| 本机 node &lt; 18.18 | 不得当作合格运行时；继续解析 deps/包管理 |
| 官方 Node 22 跑在 glibc 2.17 | 安装前按 glibc 选择 `glibc217` 资产；错包 fail-fast |
| 端口连续 +20 仍占用 | 中止并提示手动指定端口 |
| 中国测速五节点全失败 | 回退直连；再失败则中止并给出手动下载说明 |
| 升级时存在旧 `observability/` 目录 | 只提示一次，保留目录与数据 |

---

## 5. Good / Base / Bad Cases

### Good

- 海外机器：`REGION=global`，直连下载 `stx-linux-amd64` + frontend tarball，systemd enable  
- 中国机器：测速选中 `v4.gh-proxy.org`，前缀下载成功  
- 离线：bundle 含 stx + frontend + node-glibc217，内网机 `--offline` 安装成功  

### Base

- 本机已有 Node 20：跳过 node deps，仍下载 stx + frontend  
- `--without-observability`：config 中 `observability.enabled: false`，其余照常  

### Bad

- 把 stx 再次打成唯一巨型 tar.gz 且内嵌每次重复的 Node deps 作为**唯一**发布形态  
- CentOS 7 使用 official Node 22 且无探测  
- 离线安装静默访问 GitHub  

---

## 6. Tests Required

| 测试点 | 断言 |
|--------|------|
| URL 拼装单测/脚本测 | prefix + github URL 格式正确 |
| 测速选择 | mock 延迟后选最快 2xx 节点 |
| sha256 校验 | 错码拒绝安装 |
| 端口探测 | 占用端口被跳过，写入 config 为新端口 |
| offline 禁网 | `http_proxy`/curl 到外网被拦截或未调用 |
| 资产命名 | CI/package 脚本产出裸 `stx-linux-*`；离线包不含 `observability-*` 与 `bin/lib/observability.sh` |

---

## 7. Wrong vs Correct

| Wrong | Correct |
|-------|---------|
| 每次发版上传完整聚合 tar 作为唯一产物 | 发版上传裸 `stx`/`stx-agent` + frontend tarball；deps 独立复用 |
| 每个 `v*` 重打并上传 node | 只在 tag `deps`；版本工作流不传 `--emit-deps` |
| 假定纯静态 `webserver/static` | Next standalone + Node；deps/node 或系统 node |
| 中国写死单一代理域名 | 五节点测速 + 可覆盖 `STX_DOWNLOAD_MIRROR_PREFIX` |
| 为了「更全的监控」重新打包外部监控组件 | 监控内置在 stx；需要的指标在内置采集里补 |
| Compose 写死仅 GHCR、中国无法拉 | `STX_IMAGE_REGISTRY` 可切到 `swr.cn-east-3.myhuaweicloud.com/stx` |
| 假定用户必须跑脚本才能装 | README 提供手动下载资产表 + `packages/` + `install.sh --offline` |

---

## Pre-Development Checklist（本规范）

- [ ] 确认改的是「版本资产」还是「deps 资产」
- [ ] stx/agent 是否仍为裸二进制
- [ ] 在线/离线是否共用 install-core
- [ ] 中国下载是否走可测速/可覆盖镜像
- [ ] systemd 路径是否与 agent 一致
- [ ] 没有重新引入外部监控组件的下载 / 启停 / 端口
