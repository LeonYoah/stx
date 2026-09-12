# Design: 安装流自动 TLS

## Approach

1. **Go bootstrap（优先）**：API 启动、`initGRPCServer` 之前调用 `EnsureGRPCTLS`：
   - `exec.LookPath("openssl")` 检测
   - 无 openssl → 保持配置中的 TLS 关闭（若用户已显式开启且缺证书则打错误日志）
   - 有 openssl → 确保 `certs/` 下 CA/server 存在；缺则生成；回写内存配置（及可选写回 config 文件字段路径）
2. **不覆盖**：若 `server.crt`/`server.key`/`ca.crt` 已存在则跳过生成，仅启用 TLS 并指向它们；日志提示可替换后重启
3. **Agent**：新增 `GET /api/v1/agent/ca.crt`；install.sh 在 CP TLS 开启时下载到 `/etc/stx-agent/certs/ca.crt` 并写配置
4. **Validate**：Agent TLS 开启时要求 `ca_file`（或允许系统信任，本期要求 ca_file）；client cert 可选
5. **Docker**：`Dockerfile.backend` / `Dockerfile.all-in-one` 增加 `openssl`

## Cert layout

```
{workdir}/certs/
  ca.crt
  ca.key      # 仅 CP 本地，不下发
  server.crt
  server.key
```

## openssl 生成命令（示意）

- 自签 CA
- 用 CA 签 server，SAN=DNS:localhost,IP:127.0.0.1,+external_url host

## Out of scope

- mTLS 强制
- 证书自动轮换
- HTTP API 全面 HTTPS（仅 gRPC TLS）
