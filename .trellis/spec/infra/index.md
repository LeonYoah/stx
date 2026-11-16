# 基础设施 / 发布安装规范

> 适用于发布资产、安装脚本、deps 复用与控制面部署约定。

---

## 概述

本目录约束 **如何打包与安装 STX**，避免把 Node 与每次发版绑死，并统一在线/离线体验。监控内置在 stx 中，不再打包任何外部监控组件。

---

## 规范索引

| 文档 | 说明 | 状态 |
|------|------|------|
| [打包与安装契约](./packaging-and-install.md) | 资产命名、在线/离线、gh-proxy、systemd、校验矩阵 | 已填写 |

---

## 开发前检查

- 改动 `install/package-release.sh`、`install/bin/*` 或安装相关文档前，必读 [打包与安装契约](./packaging-and-install.md)。
- 区分：**版本资产**（stx / agent / frontend）与 **deps 资产**（node）。
- stx 与 stx-agent 以**裸二进制**发布；不要只提供巨型聚合 tar.gz。
- 不要重新引入外部监控组件（Prometheus / Alertmanager / Grafana）的下载、启停或端口；监控由 stx 进程内置提供。

## 质量检查

- 安装脚本变更：覆盖 checksum、offline 禁网、端口探测中至少一项自动化或手工清单。
- 发布脚本变更：确认 CI 产物文件名符合契约表。
- 国内下载逻辑变更：保留 `STX_DOWNLOAD_MIRROR_PREFIX` 覆盖能力。
