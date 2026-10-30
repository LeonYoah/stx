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

# 安装冒烟（CI 与本地通用）：先用 install/build-offline-bundle.sh 把真实 Release（默认 v1.2.0）的产物组装成 amd64 离线整包，
# 再在不含 python3 的 debian:bookworm 容器中用 install-online.sh --bundle-file 安装，
# 校验 python3 不存在、安装成功、/opt/stx/scripts/stx-java-proxy.sh 与 lib/stx-java-proxy-v{2,3}.jar 存在且与 Release SHA256SUMS 一致、
# Agent 资产接口 GET /api/v1/agent/assets/stx-java-proxy.sh 与 stx-java-proxy-v{2,3}.jar 返回 200。
#   mode=plain   : --no-systemd，start.sh 直接启动
#   mode=systemd : systemd 作为 PID1（--privileged），安装 → stop.sh 停进程 → 重装，验证 systemctl restart 真正拉起进程
#   mode=mysql   : 外部 MySQL 8 容器；首次安装设置密码 → 清空库后保留配置重装并设置新密码（必须生效）→ 库里已有管理员时
#                  再设密码（必须不生效且提示沿用）
#   mode=postgres: 外部 PostgreSQL 容器；首次安装在伪终端里走交互菜单选 3) PostgreSQL 并设置密码（必须生效）→
#                  用 --db-type pg 参数重装并设新密码（库里已有管理员，必须不生效）；全程日志不含任何密码
# Install smoke (CI and local): assemble an amd64 offline bundle from a real release (default v1.2.0) with
# install/build-offline-bundle.sh, then install it with install-online.sh --bundle-file inside a
# python3-free debian:bookworm container; assert no python3, install OK, scripts/stx-java-proxy.sh and
# lib/stx-java-proxy-v{2,3}.jar present and matching the release SHA256SUMS, and agent asset endpoints return 200.
#   mode=plain   : --no-systemd, started by start.sh
#   mode=systemd : systemd as PID1 (--privileged); install -> stop.sh -> reinstall, asserting restart really runs it
#   mode=mysql   : external MySQL 8 container; first install sets a password -> empty the database and reinstall over
#                  the preserved config with a new password (must apply) -> reinstall while the admin exists (must not)
#   mode=postgres: external PostgreSQL container; first install answers the interactive menu with 3) PostgreSQL in a
#                  pseudo-terminal and sets a password (must apply) -> reinstall via --db-type pg flags with a new
#                  password while the admin exists (must not apply); no password ever shows up in logs
#
# 用法 / Usage: scripts/ci-install-smoke.sh <plain|systemd|mysql|postgres> [release-tag]
#   DOCKER (default: docker), STX_SMOKE_IMAGE (default: debian:bookworm)

set -euo pipefail

mode="${1:?usage: $0 <plain|systemd|mysql|postgres> [release-tag]}"
tag="${2:-v1.2.0}"
DOCKER="${DOCKER:-docker}"
image="${STX_SMOKE_IMAGE:-debian:bookworm}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
name="stx-install-smoke-${mode}-$$"
work="$(mktemp -d)"
db="stx-smoke-db-$$"
cleanup() {
  $DOCKER rm -f "$name" "$db" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

want_sha="$(curl -fsSL --retry 3 "https://github.com/LeonYoah/stx/releases/download/${tag}/SHA256SUMS" \
  | awk '$2 == "stx-java-proxy.sh" {print $1}')"
echo "[smoke] release $tag SHA256SUMS stx-java-proxy.sh = ${want_sha:-<not listed>}"

# 用仓库脚本从 Release 组装离线整包（与发版 CI 同一脚本）。/ Assemble the bundle with the same script as release CI.
bash "$ROOT_DIR/install/build-offline-bundle.sh" --variant amd64 --from-release "$tag" --version "$tag" \
  --without-observability --output-dir "$work/out" --cache-dir "$work/cache"
bundle="stx-${tag}-linux-amd64-offline.tar.gz"
[[ -f "$work/out/$bundle" ]] || { echo "[smoke] bundle $bundle was not built" >&2; exit 1; }

# 容器内执行的检查脚本。/ Checks executed inside the container.
# shellcheck disable=SC2016  # expanded inside the container
inner='
set -uo pipefail
fail=0
check() { if eval "$2"; then echo "[smoke] OK   $1"; else echo "[smoke] FAIL $1"; fail=1; fi; }
api_port() { awk "/^app:/{a=1} a&&/^  addr:/{gsub(/[^0-9]/,\"\",\$2);print \$2;exit}" /opt/stx/config.yaml; }
verify() {
  local port code got
  check "/opt/stx/scripts/stx-java-proxy.sh exists" "[[ -x /opt/stx/scripts/stx-java-proxy.sh ]]"
  check "/opt/stx/lib/stx-java-proxy-v2.jar exists" "[[ -s /opt/stx/lib/stx-java-proxy-v2.jar ]]"
  check "/opt/stx/lib/stx-java-proxy-v3.jar exists" "[[ -s /opt/stx/lib/stx-java-proxy-v3.jar ]]"
  got="$(sha256sum /opt/stx/scripts/stx-java-proxy.sh 2>/dev/null | cut -d\  -f1)"
  [[ -n "$WANT_SHA" ]] && check "stx-java-proxy.sh sha256 matches release SHA256SUMS ($got)" "[[ \"$got\" == \"$WANT_SHA\" ]]"
  port="$(api_port)"
  for _ in $(seq 1 30); do
    code="$(curl -s -o /tmp/asset.sh -w "%{http_code}" "http://127.0.0.1:${port}/api/v1/agent/assets/stx-java-proxy.sh" || true)"
    [[ "$code" == 200 ]] && break; sleep 2
  done
  check "GET /api/v1/agent/assets/stx-java-proxy.sh -> $code" "[[ \"$code\" == 200 ]]"
  code="$(curl -s -o /tmp/asset-v2.jar -w "%{http_code}" "http://127.0.0.1:${port}/api/v1/agent/assets/stx-java-proxy-v2.jar" || true)"
  check "GET /api/v1/agent/assets/stx-java-proxy-v2.jar -> $code" "[[ \"$code\" == 200 ]]"
  code="$(curl -s -o /tmp/asset-v3.jar -w "%{http_code}" "http://127.0.0.1:${port}/api/v1/agent/assets/stx-java-proxy-v3.jar" || true)"
  check "GET /api/v1/agent/assets/stx-java-proxy-v3.jar -> $code" "[[ \"$code\" == 200 ]]"
  code="$(curl -s -o /tmp/asset-q.jar -w "%{http_code}" "http://127.0.0.1:${port}/api/v1/agent/assets/stx-java-proxy.jar?version=v2" || true)"
  check "GET /api/v1/agent/assets/stx-java-proxy.jar?version=v2 -> $code" "[[ \"$code\" == 200 ]]"
  check "/opt/stx/bin/status.sh reports backend running" "grep -q \"backend: running\" <<<\"\$(/opt/stx/bin/status.sh)\""
}
'

install_cmd="bash /src/install/install-online.sh -y --bundle-file /bundle/$bundle --without-observability"

extra_pkgs=""
if [[ "$mode" == systemd ]]; then
  extra_pkgs="systemd systemd-sysv dbus"
fi
prep='export DEBIAN_FRONTEND=noninteractive
for i in 1 2 3 4 5; do apt-get update -qq >/dev/null 2>&1 && break; sleep 5; done
apt-get install -y -qq --no-install-recommends curl ca-certificates iproute2 procps '"$extra_pkgs"' >/dev/null 2>&1'

case "$mode" in
  plain)
    $DOCKER run --rm --name "$name" -e TAG="$tag" -e WANT_SHA="$want_sha" -v "$ROOT_DIR:/src:ro" -v "$work/out:/bundle:ro" "$image" bash -c "
      $prep
      $inner
      check 'python3 is NOT installed' '! command -v python3 >/dev/null'
      $install_cmd --no-systemd 2>&1 | grep -vE '^ *[0-9]+ |% Total|Dload'; rc=\${PIPESTATUS[0]}
      check \"install-online.sh exit 0 (rc=\$rc)\" '[[ \$rc -eq 0 ]]'
      verify
      exit \$fail"
    ;;
  systemd)
    $DOCKER run -d --name "$name" --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
      -e TAG="$tag" -e WANT_SHA="$want_sha" -v "$ROOT_DIR:/src:ro" -v "$work/out:/bundle:ro" "$image" \
      bash -c "$prep; touch /tmp/prep-done; exec /sbin/init" >/dev/null
    for _ in $(seq 1 100); do
      $DOCKER exec "$name" test -d /run/systemd/system 2>/dev/null && break; sleep 3
    done
    $DOCKER exec -e TAG="$tag" -e WANT_SHA="$want_sha" "$name" bash -c "
      $inner
      check 'python3 is NOT installed' '! command -v python3 >/dev/null'
      check 'systemd is PID1' '[[ -d /run/systemd/system ]]'
      $install_cmd 2>&1 | grep -vE '^ *[0-9]+ |% Total|Dload'; rc=\${PIPESTATUS[0]}
      check \"install #1 exit 0 (rc=\$rc)\" '[[ \$rc -eq 0 ]]'
      check 'stx.service enabled' '[[ \$(systemctl is-enabled stx) == enabled ]]'
      check 'stx.service active' '[[ \$(systemctl is-active stx) == active ]]'
      check 'processes run inside the unit cgroup' 'grep -q \"stx server\" <<<\"\$(systemctl status stx --no-pager)\"'
      verify
      # 进程被 stop.sh 停掉而 unit 仍 active(exited)，重装必须真正拉起进程。
      # Processes stopped via stop.sh while the unit stays active(exited): reinstall must really start them.
      /opt/stx/bin/stop.sh >/dev/null 2>&1
      check 'after stop.sh unit still active(exited)' '[[ \$(systemctl is-active stx) == active ]]'
      check 'after stop.sh backend stopped' 'grep -q \"backend: stopped\" <<<\"\$(/opt/stx/bin/status.sh)\"'
      $install_cmd 2>&1 | grep -E '\\[(OK|ERROR)\\] started|auto starting|ERROR'; rc=\${PIPESTATUS[0]}
      check \"install #2 (reinstall) exit 0 (rc=\$rc)\" '[[ \$rc -eq 0 ]]'
      verify
      systemctl restart stx; sleep 3
      check 'systemctl restart stx keeps it running' 'grep -q \"backend: running\" <<<\"\$(/opt/stx/bin/status.sh)\"'
      systemctl stop stx; sleep 2
      check 'systemctl stop stx stops backend' 'grep -q \"backend: stopped\" <<<\"\$(/opt/stx/bin/status.sh)\"'
      exit \$fail"
    ;;
  mysql)
    # 默认 bridge + 容器 IP（不建自定义网络，避免某些环境里自定义网络的 DNS 不能解析外网）。
    # Default bridge + container IP (no custom network; its embedded DNS cannot reach the Internet on some hosts).
    $DOCKER run -d --name "$db" -e MYSQL_ROOT_PASSWORD=rootpw -e MYSQL_DATABASE=stx \
      "${STX_SMOKE_MYSQL_IMAGE:-mysql:8.0}" >/dev/null
    for _ in $(seq 1 60); do
      $DOCKER exec "$db" mysqladmin -uroot -prootpw --silent ping >/dev/null 2>&1 \
        && $DOCKER exec "$db" mysql -uroot -prootpw -e "SELECT 1" stx >/dev/null 2>&1 && break
      sleep 3
    done
    db_ip="$($DOCKER inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$db")"
    echo "[smoke] MySQL at $db_ip:3306"
    $DOCKER run -d --name "$name" -v "$ROOT_DIR:/src:ro" -v "$work/out:/bundle:ro" "$image" \
      bash -c "$prep; touch /tmp/prep-done; sleep infinity" >/dev/null
    for _ in $(seq 1 100); do $DOCKER exec "$name" test -f /tmp/prep-done 2>/dev/null && break; sleep 3; done
    # shellcheck disable=SC2016  # expanded inside the container
    mysql_inner='
      login_ok() { curl -s -m 10 -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"$1\"}" \
        "http://127.0.0.1:$(api_port)/api/v1/auth/login" | grep -q "\"username\":\"admin\""; }
      db_args="--db-type mysql --db-host '"$db_ip"' --db-port 3306 --db-user root --db-password rootpw --db-name stx"
      # 1) 首次安装（空库）设置密码。/ First install on an empty database with a password.
      '"$install_cmd"' --no-systemd $db_args --admin-password Pw-One-111 >/tmp/i1.log 2>&1; rc=$?
      check "install #1 on empty MySQL exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
      check "install #1: config database.type mysql" "grep -q \"type: \\\"mysql\\\"\" /opt/stx/config.yaml"
      check "install #1: login with Pw-One-111" "login_ok Pw-One-111"
      check "install #1: summary says password changed" "grep -q 安装时已修改 /tmp/i1.log"
      /opt/stx/bin/stop.sh >/dev/null 2>&1
      touch /tmp/need-wipe
      while [[ -e /tmp/need-wipe ]]; do sleep 1; done
      # 2) 库被清空、保留旧配置重装并设置新密码：必须生效。/ Emptied DB + preserved config + new password: must apply.
      '"$install_cmd"' --no-systemd --admin-password Pw-Two-222 >/tmp/i2.log 2>&1; rc=$?
      check "install #2 (preserved config, empty DB) exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
      check "install #2: kept external MySQL config" "grep -q \"type: \\\"mysql\\\"\" /opt/stx/config.yaml"
      check "install #2: login with Pw-Two-222 (new password applied)" "login_ok Pw-Two-222"
      check "install #2: summary says password changed" "grep -q 安装时已修改 /tmp/i2.log"
      # 3) 库里已有管理员：新密码不生效并提示沿用。/ Admin exists: the new password must not apply.
      '"$install_cmd"' --no-systemd --admin-password Pw-Three-333 >/tmp/i3.log 2>&1; rc=$?
      check "install #3 (admin exists) exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
      check "install #3: Pw-Two-222 still works" "login_ok Pw-Two-222"
      check "install #3: Pw-Three-333 rejected" "! login_ok Pw-Three-333"
      check "install #3: summary says existing password kept" "grep -q 沿用已有数据库中的密码 /tmp/i3.log"
      # 4) 不传密码重装：摘要不能谎称默认密码。/ Reinstall without a password must not claim the default one.
      '"$install_cmd"' --no-systemd >/tmp/i4.log 2>&1; rc=$?
      check "install #4 exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
      check "install #4: summary says existing password kept" "grep -q 沿用已有数据库中的密码 /tmp/i4.log"
      # 密码不得出现在摘要、安装日志或配置以外的任何日志里。/ Passwords must never show up in summaries or logs.
      check "install #3: unapplied Pw-Three-333 removed from config" "! grep -qF Pw-Three-333 /opt/stx/config.yaml"
      check "install #3: config restored to the applied password" "grep -qF Pw-Two-222 /opt/stx/config.yaml"
      check "install #4: summary/log does not contain Pw-Two-222" "! grep -qF Pw-Two-222 /tmp/i4.log"
      check "logs/install.log does not contain any password" "! grep -qE \"Pw-(One-111|Two-222|Three-333)|rootpw\" /opt/stx/logs/install.log"
      check "no /tmp/stx-install-*.log contains a password" "! grep -lE \"Pw-(One-111|Two-222|Three-333)\" /tmp/stx-install-*.log"
      # 5) --no-start 且不传密码：unknown 摘要也不能显示原配置里的密码。/ --no-start + no password must not print it either.
      '"$install_cmd"' --no-systemd --no-start >/tmp/i5.log 2>&1; rc=$?
      check "install #5 (--no-start) exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
      check "install #5: summary does not contain Pw-Two-222" "! grep -qF Pw-Two-222 /tmp/i5.log"
      check "install #5: summary says the password is not shown" "grep -q 出于安全不在此显示 /tmp/i5.log"
      exit $fail'
    $DOCKER exec "$name" bash -c "$inner $mysql_inner" &
    inner_pid=$!
    for _ in $(seq 1 200); do
      $DOCKER exec "$name" test -e /tmp/need-wipe 2>/dev/null && break
      kill -0 "$inner_pid" 2>/dev/null || break
      sleep 3
    done
    if $DOCKER exec "$name" test -e /tmp/need-wipe 2>/dev/null; then
      $DOCKER exec "$db" mysql -uroot -prootpw -e "DROP DATABASE stx; CREATE DATABASE stx DEFAULT CHARACTER SET utf8mb4;"
      echo "[smoke] MySQL database stx emptied"
      $DOCKER exec "$name" rm -f /tmp/need-wipe
    fi
    rc=0; wait "$inner_pid" || rc=$?
    [[ $rc -eq 0 ]] || { $DOCKER exec "$name" bash -c "tail -40 /tmp/i*.log" || true; exit "$rc"; }
    ;;
  postgres)
    $DOCKER run -d --name "$db" -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=rootpw -e POSTGRES_DB=stx \
      "${STX_SMOKE_POSTGRES_IMAGE:-postgres:16-alpine}" >/dev/null
    for _ in $(seq 1 60); do
      $DOCKER exec "$db" pg_isready -U postgres -d stx >/dev/null 2>&1 \
        && $DOCKER exec "$db" psql -U postgres -d stx -c "SELECT 1" >/dev/null 2>&1 && break
      sleep 3
    done
    db_ip="$($DOCKER inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$db")"
    echo "[smoke] PostgreSQL at $db_ip:5432"
    $DOCKER run -d --name "$name" -v "$ROOT_DIR:/src:ro" -v "$work/out:/bundle:ro" "$image" \
      bash -c "$prep; touch /tmp/prep-done; sleep infinity" >/dev/null
    for _ in $(seq 1 100); do $DOCKER exec "$name" test -f /tmp/prep-done 2>/dev/null && break; sleep 3; done
    # shellcheck disable=SC2016  # expanded inside the container
    pg_inner='
      login_ok() { curl -s -m 10 -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"$1\"}" \
        "http://127.0.0.1:$(api_port)/api/v1/auth/login" | grep -q "\"username\":\"admin\""; }
      check "util-linux script (pseudo-terminal) available" "command -v script >/dev/null"
      # 1) 交互安装（不带 -y）：数据库菜单选 3，依次回答 地址/端口(回车=5432)/用户/密码/库名(回车=stx)/管理员密码×2。
      #    Interactive install (no -y): menu choice 3, then host / port (Enter=5432) / user / password / db (Enter=stx)
      #    / admin password twice.
      printf "3\n%s\n\npostgres\nrootpw\n\nPg-One-111\nPg-One-111\n" "'"$db_ip"'" >/tmp/pg-menu.in
      script -qec "bash /src/install/install-online.sh --bundle-file /bundle/'"$bundle"' --without-observability --no-systemd \
        --frontend-port 17880 --http-port 17800 --grpc-port 17890 --external-ip 127.0.0.1" /dev/null </tmp/pg-menu.in >/tmp/i1.log 2>&1; rc=$?
      check "install #1 (interactive menu 3) exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
      check "install #1: menu offered 3) PostgreSQL" "grep -q \"3) PostgreSQL\" /tmp/i1.log"
      check "install #1: postgres create-database hint" "grep -q \"CREATE DATABASE stx;\" /tmp/i1.log"
      check "install #1: no MySQL utf8mb4 hint" "! grep -q utf8mb4 /tmp/i1.log"
      check "install #1: config database.type postgres" "grep -q \"type: \\\"postgres\\\"\" /opt/stx/config.yaml"
      check "install #1: config database.port 5432" "grep -q \"port: 5432\" /opt/stx/config.yaml"
      check "install #1: login with Pg-One-111" "login_ok Pg-One-111"
      check "install #1: summary says password changed" "grep -q 安装时已修改 /tmp/i1.log"
      verify
      # 2) 参数安装（-y，--db-type 别名 pg）：库里已有管理员，新密码不生效。/ Flags (-y, alias pg): admin exists, must not apply.
      '"$install_cmd"' --no-systemd --db-type pg --db-host '"$db_ip"' --db-user postgres --db-password rootpw \
        --admin-password Pg-Two-222 >/tmp/i2.log 2>&1; rc=$?
      check "install #2 (--db-type pg) exit 0 (rc=$rc)" "[[ $rc -eq 0 ]]"
      check "install #2: config database.type postgres" "grep -q \"type: \\\"postgres\\\"\" /opt/stx/config.yaml"
      check "install #2: Pg-One-111 still works" "login_ok Pg-One-111"
      check "install #2: Pg-Two-222 rejected" "! login_ok Pg-Two-222"
      check "install #2: summary says existing password kept" "grep -q 沿用已有数据库中的密码 /tmp/i2.log"
      check "install #2: unapplied Pg-Two-222 removed from config" "! grep -qF Pg-Two-222 /opt/stx/config.yaml"
      verify
      check "install #2: log has no password" "! grep -qE \"Pg-(One-111|Two-222)|rootpw\" /tmp/i2.log"
      check "logs/install.log does not contain any password" "! grep -qE \"Pg-(One-111|Two-222)|rootpw\" /opt/stx/logs/install.log"
      check "no /tmp/stx-install-*.log contains a password" "! grep -lE \"Pg-(One-111|Two-222)|rootpw\" /tmp/stx-install-*.log"
      exit $fail'
    rc=0; $DOCKER exec -e WANT_SHA="$want_sha" "$name" bash -c "$inner $pg_inner" || rc=$?
    [[ $rc -eq 0 ]] || { $DOCKER exec "$name" bash -c "tail -60 /tmp/i*.log; tail -40 /opt/stx/logs/backend.log" || true; exit "$rc"; }
    ;;
  *)
    echo "unknown mode: $mode" >&2
    exit 2
    ;;
esac
echo "[smoke] mode=$mode tag=$tag passed"
