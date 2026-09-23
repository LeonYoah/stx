# PRD: 在线 / 离线安装与发布依赖拆分

## 背景

STX 控制面需要可复制的安装体验：在线一条命令、离线「有网机打包 → 离线机安装」。当前发布物把 Node、三件套打进每个 stx 包，体积大、重复多；Next.js standalone 仍需 Node，CentOS 7 需 glibc217 变体。监控采用「可选内置三件套」，产品壳自研、时序/告警引擎不自研（对齐 Doris Manager 打包思路，但不塞 JDK）。

## 目标

1. **发布瘦身**：日常只构建/上传 `stx` 与 `stx-agent` 二进制（**不打成 tar.gz**）；Node / 可观测三件套作为独立 deps 资产，GitHub 上各版本只存一份。
2. **在线一键**：单脚本完成探测、选源、下载、安装、systemd、默认配置与收尾提示。
3. **离线一键**：有网机下载完整 bundle（含所需 deps），拷贝到离线机后本地安装，不访问外网。
4. **国内加速**：按地区/连通性判断；国内经 gh-proxy 多节点测速后下载。
5. **安装体验**：端口占用自动 +1 探测；默认配置写好；告知配置路径与重启方式；systemd 对齐 agent。
6. **Docker**：`stx-all-in-one` **不含**监控，文档写清；Compose 可选监控另议。

## 非目标（本期）

- 在 STX 内自研时序库 / PromQL / 告警引擎
- all-in-one 镜像内置 Prometheus/Grafana/Alertmanager
- 强制纯静态前端（继续 Next standalone + Node）
- K8s/Helm 一键（可后续子任务）

## 用户故事

1. 作为国内用户，我执行一条 curl|bash，脚本自动选加速节点并装好控制面。
2. 作为离线机房用户，我在跳板机下好 bundle，U 盘拷到内网后 `./install.sh` 即可。
3. 作为运维，我只关心改 `/opt/stx/config.yaml` 后 `systemctl restart stx`。
4. 作为发布者，发版只需上传 stx/agent 二进制；Node/三件套版本不变则不必重传。

## 验收标准

- [ ] Release 资产：`stx`、`stx-agent` 为**裸二进制**（可附 `.sha256`），不强制整包 tar.gz
- [ ] deps 资产：`node-*`、`observability-*` 独立发布，版本不变可复用
- [ ] `install-online.sh` 一条命令完成在线安装（含 region + 可选测速）
- [ ] `download-bundle.sh` 产出可拷贝的离线目录；离线 `install.sh` 零外网
- [ ] 端口冲突自动顺延；安装结束打印配置路径与重启命令
- [ ] systemd（system/user）行为与 stx-agent 安装脚本同级
- [ ] 文档明确 all-in-one 不含监控
- [ ] 设计与契约落入 `.trellis/spec/infra/`，供后续实现对照
