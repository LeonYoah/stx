# 打包与安装契约

> 适用于发布资产、在线/离线安装脚本、控制面 systemd 与可选可观测 deps。  
> 详细产品背景见任务 `.trellis/tasks/09-18-online-offline-install-packaging/`。

---

## 1. Scope / Trigger

在以下改动时必须遵守本文：

- 修改 `scripts/package-release.sh`、`support-files/release/*`、`deps/*observability*`
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
| 三件套 | `observability-prom<ver>-am<ver>-gf<ver>-linux-<arch>.tar.gz` |
| 清单 | `MANIFEST.json`（组件版本、最低 node、适用 glibc） |

存放位置是 GitHub Release tag **`deps`**，不是每个 `v*`。版本工作流 `release-on-tag.yml` 不得传 `--emit-deps`。只有 Node 或三件套版本变化时，手动跑 `release-deps.yml`。该 Release 必须 `--latest=false`。

### 2.3 用户入口脚本

```bash
# 在线
curl -fsSL <raw-or-mirrored-url>/install-online.sh | bash -s -- [options]

# 离线打包（有网机）
./download-bundle.sh --version vX.Y.Z --arch amd64 [--node-variant glibc217] [--with-observability]

# 安装（在线解压后或离线 bundle 内）
./install.sh [--install-dir /opt/stx] [--offline] [--with-observability] [--no-start]
```

---

## 3. Contracts

### 3.1 环境变量

| 变量 | 默认 | 含义 |
|------|------|------|
| `STX_DOWNLOAD_REGION` | 自动 | `cn` \| `global` |
| `STX_DOWNLOAD_MIRROR_PREFIX` | 空 | 非空则直接前缀 GitHub URL，跳过公共测速 |
| `STX_GH_PROXY_NODES` | 见下表 | 逗号分隔 |
| `STX_INSTALL_DIR` | `/opt/stx` | 安装根目录 |
| `START_OBSERVABILITY` | `auto` | `auto` \| `true` \| `false`（start.sh） |

### 3.2 gh-proxy 节点（国内默认）

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
$INSTALL_DIR/runtime/node/bin/node    # 可选
$INSTALL_DIR/deps/start-observability.sh  # 可选 bundled
```

### 3.4 配置键（安装器写入）

- 默认端口：前端 `17880`、HTTP API `17800`、gRPC `17890`（避开常见 80/8000/9000）
- `app.addr` / gRPC 端口：端口探测结果  
- `app.external_url`：本机可达 URL  
- `observability.enabled` / `observability.*.url`：bundled 或 remote  
- **不要**在 all-in-one 镜像默认启用 bundled 三件套进程  

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
| 国内测速五节点全失败 | 回退直连；再失败则中止并给出手动下载说明 |
| all-in-one 文档 | 必须写明「不含监控三件套」 |

---

## 5. Good / Base / Bad Cases

### Good

- 海外机器：`REGION=global`，直连下载 `stx-linux-amd64` + frontend tarball，systemd enable  
- 国内机器：测速选中 `v4.gh-proxy.org`，前缀下载成功  
- 离线：bundle 含 stx + frontend + node-glibc217，内网机 `--offline` 安装成功  

### Base

- 本机已有 Node 20：跳过 node deps，仍下载 stx + frontend  
- 不带 observability：不拉三件套，config 中 enabled=false 或保持 remote 空闲  

### Bad

- 把 stx 再次打成唯一巨型 tar.gz 且内嵌每次重复的 Node+三件套作为**唯一**发布形态  
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
| 资产命名 | CI/package 脚本产出裸 `stx-linux-*` 而非仅旧式 with-observability tar |

---

## 7. Wrong vs Correct

| Wrong | Correct |
|-------|---------|
| 每次发版上传完整 `…-with-observability.tar.gz` 作为唯一产物 | 发版上传裸 `stx`/`stx-agent` + frontend tarball；deps 独立复用 |
| 每个 `v*` 重打并上传 node / 三件套 | 这两类只在 tag `deps`；版本工作流不传 `--emit-deps` |
| 假定纯静态 `webserver/static` | Next standalone + Node；deps/node 或系统 node |
| 国内写死单一代理域名 | 五节点测速 + 可覆盖 `STX_DOWNLOAD_MIRROR_PREFIX` |
| all-in-one 塞入三件套「图省事」 | all-in-one 仅管控面；全量监控用 `deploy/docker/docker-compose.yml` |
| 假定用户必须跑脚本才能装 | README 提供手动下载资产表 + `packages/` + `install.sh --offline` |

---

## Pre-Development Checklist（本规范）

- [ ] 确认改的是「版本资产」还是「deps 资产」
- [ ] stx/agent 是否仍为裸二进制
- [ ] 在线/离线是否共用 install-core
- [ ] 国内下载是否走可测速/可覆盖镜像
- [ ] systemd 路径是否与 agent 一致
- [ ] 文档是否声明 all-in-one 无监控
