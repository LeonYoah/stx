#!/usr/bin/env bash
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# 在干净容器里用离线包做端到端安装冒烟（CI 与本地通用；安装本身不联网）。镜像里没有 curl 时（如 ubuntu）先用 apt 装上
# curl，以便做 HTTP 检查；STX_SMOKE_NO_APT=1 可跳过（此时 HTTP 检查会失败而不是被跳过）。
# End-to-end install smoke of an offline bundle in a clean container (CI and local; the install itself is offline).
# Images without curl (e.g. ubuntu) get curl via apt first so the HTTP checks run; STX_SMOKE_NO_APT=1 skips that
# (the HTTP checks then fail instead of being skipped).
#   1. 解压离线包，非交互执行 ./install.sh -y --no-systemd（自定义管理员密码与端口）
#   2. 校验内置 node --version、/api/v1/health=200、Web 首页 2xx/3xx、用新密码登录成功
#   3. 安装摘要包含访问地址 / 账号 / 后续步骤，且不回显自定义密码
#   4. 重装（保留配置与数据库）：端口沿用，提示管理员密码沿用
#   Extract, run ./install.sh -y --no-systemd with a custom password and ports; check bundled node, API health,
#   web root, login with the new password, the summary text, then a reinstall that keeps config and DB.
#
# 用法 / Usage: scripts/smoke-offline-bundle.sh <bundle.tar.gz> [image]
#   image 默认按包名：*-centos7-* => centos:7，否则 ubuntu:22.04。DOCKER 默认 docker。

set -euo pipefail

bundle="${1:?usage: $0 <bundle.tar.gz> [image]}"
bundle="$(cd "$(dirname "$bundle")" && pwd)/$(basename "$bundle")"
image="${2:-}"
if [[ -z "$image" ]]; then
  case "$bundle" in
    *centos7*) image="centos:7" ;;
    *) image="ubuntu:22.04" ;;
  esac
fi
DOCKER="${DOCKER:-docker}"
name="stx-bundle-smoke-$$"
cleanup() { $DOCKER rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "[smoke] bundle=$(basename "$bundle") image=$image"

# shellcheck disable=SC2016  # expanded inside the container
inner='
set -uo pipefail
fail=0
check() { if eval "$2"; then echo "[smoke] OK   $1"; else echo "[smoke] FAIL $1"; fail=1; fi; }
code() { curl -s -o /dev/null -m 5 -w "%{http_code}" "$1" 2>/dev/null || true; }
if ! command -v curl >/dev/null 2>&1 && [[ "${STX_SMOKE_NO_APT:-0}" != 1 ]] && command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  for i in 1 2 3; do apt-get update -qq >/dev/null 2>&1 && break; sleep 3; done
  apt-get install -y -qq --no-install-recommends curl ca-certificates >/dev/null 2>&1 || true
fi
check "curl available for HTTP checks" "command -v curl >/dev/null"
mkdir -p /work && cd /work
tar -xzf /bundle/b.tar.gz
cd stx-*-offline
echo "[smoke] glibc: $(ldd --version 2>&1 | head -n1)"
./install.sh -y --no-systemd --admin-password "S3cret-pw!" --frontend-port 18080 --http-port 18000 --grpc-port 18090 \
  --external-ip 10.1.2.3 --prometheus-port 19090 --alertmanager-port 19093 --grafana-port 13000 \
  >/tmp/install1.log 2>&1; rc=$?
cat /tmp/install1.log
check "install.sh exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
check "numbered steps [1/7]..[7/7] logged" "grep -q \"\\[1/7\\]\" /tmp/install1.log && grep -q \"\\[7/7\\]\" /tmp/install1.log"
check "bundled node runs: $(/opt/stx/runtime/node/bin/node --version 2>&1)" "/opt/stx/runtime/node/bin/node --version"
check "summary shows URL with web port" "grep -q \"http://.*:18080\" /tmp/install1.log"
check "summary shows account admin" "grep -q \"登录账号 / user: .*admin\" /tmp/install1.log"
check "summary does not echo the custom password" "! grep -q \"S3cret-pw!\" /tmp/install1.log"
check "summary says password was changed" "grep -q \"已修改为你设置的密码\" /tmp/install1.log"
check "next steps mention 安装包管理 / 插件市场 / 主机管理 / 集群管理" "grep -q 安装包管理 /tmp/install1.log && grep -q 插件市场 /tmp/install1.log && grep -q 主机管理 /tmp/install1.log && grep -q 集群管理 /tmp/install1.log"
check "next steps mention JDK" "grep -q JDK /tmp/install1.log"
check "config app.addr :18000" "grep -q \"addr: \\\":18000\\\"\" /opt/stx/config.yaml || grep -q \"addr: :18000\" /opt/stx/config.yaml"
check "config grpc.port 18090" "grep -q \"port: 18090\" /opt/stx/config.yaml"
check "stx.env FRONTEND_PORT=18080" "grep -q FRONTEND_PORT=18080 /opt/stx/stx.env"
check "install.log copied" "[[ -s /opt/stx/logs/install.log ]]"
check "pre-flight result list printed" "grep -q \"安装前检查\" /tmp/install1.log && grep -q \"磁盘 / disk\" /tmp/install1.log"
check "external_url uses --external-ip" "grep -q \"external_url: \\\"http://10.1.2.3:18000\\\"\" /opt/stx/config.yaml"
check "console_url set" "grep -q \"console_url: \\\"http://10.1.2.3:18080\\\"\" /opt/stx/config.yaml"
check "summary URL uses external ip" "grep -q \"http://10.1.2.3:18080\" /tmp/install1.log"
check "stx.env has observability ports" "grep -q STX_GRAFANA_PORT=13000 /opt/stx/stx.env && grep -q STX_PROMETHEUS_PORT=19090 /opt/stx/stx.env"
check "prometheus.yml targets alertmanager :19093 and stx :18000" "grep -q 127.0.0.1:19093 /opt/stx/observability/prometheus/prometheus.yml && grep -q 127.0.0.1:18000 /opt/stx/observability/prometheus/prometheus.yml"
check "summary mentions observability disk usage" "grep -q \"占用磁盘\" /tmp/install1.log"
check "GET /api/v1/health -> 200" "[[ \$(code http://127.0.0.1:18000/api/v1/health) == 200 ]]"
c="$(code http://127.0.0.1:18080/)"
check "GET web / -> $c" "[[ $c =~ ^[23] ]]"
login="$(curl -s -m 10 -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"S3cret-pw!\"}" http://127.0.0.1:18000/api/v1/auth/login || true)"
echo "[smoke] login response: ${login:0:200}"
check "login with the new password succeeds" "grep -q \"\\\"username\\\":\\\"admin\\\"\" <<<\"\$login\""
bad="$(curl -s -m 10 -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" http://127.0.0.1:18000/api/v1/auth/login || true)"
check "login with admin123 is rejected" "! grep -q \"\\\"username\\\":\\\"admin\\\"\" <<<\"\$bad\""
sleep 5
check "prometheus ready on :19090" "[[ \$(code http://127.0.0.1:19090/-/ready) == 200 ]]"
check "grafana on :13000" "[[ \$(code http://127.0.0.1:13000/api/health) == 200 ]]"
check "alertmanager on :19093" "[[ \$(code http://127.0.0.1:19093/-/ready) == 200 ]]"
# 端口冲突（非交互）必须明确失败。/ Port conflict in non-interactive mode must fail clearly.
./install.sh -y --no-systemd --install-dir /opt/stx-conflict --http-port 18000 --without-observability >/tmp/conflict.log 2>&1; rc=$?
check "non-interactive port conflict fails (rc=$rc)" "[[ $rc -ne 0 ]]"
check "conflict message names the port" "grep -q \"端口冲突\" /tmp/conflict.log && grep -q \"18000\" /tmp/conflict.log"
# MySQL 只写配置（不启动）。/ MySQL is config-only (no start).
./install.sh -y --no-systemd --no-start --install-dir /opt/stx-mysql --without-observability --db-type mysql \
  --db-host 10.9.9.9 --db-port 3307 --db-user stx --db-password "p\"w\\x" --db-name stxdb >/tmp/mysql.log 2>&1; rc=$?
check "mysql config install exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
check "database.type mysql" "grep -q \"type: \\\"mysql\\\"\" /opt/stx-mysql/config.yaml || grep -q \"type: mysql\" /opt/stx-mysql/config.yaml"
check "database host/port/user/db written" "grep -q \"host: \\\"10.9.9.9\\\"\" /opt/stx-mysql/config.yaml && grep -q \"port: 3307\" /opt/stx-mysql/config.yaml && grep -q \"database: \\\"stxdb\\\"\" /opt/stx-mysql/config.yaml"
check "database password escaped" "grep -qF \"password: \\\"p\\\\\\\"w\\\\\\\\x\\\"\" /opt/stx-mysql/config.yaml"
grep -n "^database:" -A12 /opt/stx-mysql/config.yaml | grep -E "type|host|port|username|password|database:" | head -8
./install.sh -y --no-systemd >/tmp/install2.log 2>&1; rc=$?
tail -n 40 /tmp/install2.log
check "reinstall exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
check "reinstall keeps web port 18080" "grep -q \"http://.*:18080\" /tmp/install2.log"
check "reinstall keeps api port 18000" "grep -q \"api=18000\" /tmp/install2.log"
check "reinstall: admin password unchanged notice" "grep -q 沿用已有数据库中的密码 /tmp/install2.log"
check "reinstall: summary/log never contains the custom password" "! grep -qF \"S3cret-pw!\" /tmp/install2.log && ! grep -qF \"S3cret-pw!\" /opt/stx/logs/install.log"
check "first install log never contains the custom password" "! grep -qF \"S3cret-pw!\" /tmp/install1.log"
/opt/stx/bin/stop.sh >/dev/null 2>&1 || true
exit $fail
'

$DOCKER run --rm --name "$name" -e STX_SMOKE_NO_APT="${STX_SMOKE_NO_APT:-0}" -v "$bundle:/bundle/b.tar.gz:ro" "$image" bash -c "$inner"
