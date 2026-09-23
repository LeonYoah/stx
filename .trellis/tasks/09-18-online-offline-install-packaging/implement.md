# Implement: 在线离线安装与发布依赖拆分

## Goal

落地 P0 + systemd（P1 一部分）：裸二进制发布默认路径、下载库（地区/gh-proxy/校验）、install-core（端口/配置/systemd）、在线一键与离线 bundle 骨架。

## Checklist

1. [x] Spec/PRD/design 已有
2. [x] `support-files/release/download-lib.sh` — region、五节点测速、URL 拼装、sha256（不放 `lib/`，避免被 gitignore）
3. [x] `support-files/release/install-core.sh` — 端口探测、写 config、systemd、收尾提示
4. [x] `support-files/release/install.sh` — `--offline` / `--with-observability` / 调 core
5. [x] `scripts/install-online.sh` — 在线下载 + 调 install
6. [x] `scripts/download-bundle.sh` — 组离线包
7. [x] `scripts/package-release.sh` — 默认 `--layout split`；保留 `--layout legacy`
8. [x] 文档：`docs/打包发布说明.md`；CI release workflow；all-in-one 不含监控写清
9. [x] 轻量自测：`bash -n` + `scripts/test-release-download-lib.sh`

## Validation

```bash
bash -n support-files/release/download-lib.sh
bash -n support-files/release/install-core.sh
bash -n support-files/release/install.sh
bash -n scripts/install-online.sh
bash -n scripts/download-bundle.sh
bash -n scripts/package-release.sh
# 可选：STX_DOWNLOAD_MIRROR_PREFIX=... 干跑拼装
```

## Rollback

保留 `--layout legacy`；旧聚合 tar.gz 安装路径仍可用现有 `install.sh` 拷贝逻辑（兼容 SOURCE_DIR 含 stx 的形态）。
