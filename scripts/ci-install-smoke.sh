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
# 校验 python3 不存在、安装成功、/opt/stx/scripts/stx-java-proxy.sh 存在且与 Release SHA256SUMS 一致、
# Agent 资产接口 GET /api/v1/agent/assets/stx-java-proxy.sh 返回 200。
#   mode=plain   : --no-systemd，start.sh 直接启动
#   mode=systemd : systemd 作为 PID1（--privileged），安装 → stop.sh 停进程 → 重装，验证 systemctl restart 真正拉起进程
# Install smoke (CI and local): assemble an amd64 offline bundle from a real release (default v1.2.0) with
# install/build-offline-bundle.sh, then install it with install-online.sh --bundle-file inside a
# python3-free debian:bookworm container; assert no python3, install OK, scripts/stx-java-proxy.sh present and
# matching the release SHA256SUMS, and the agent asset endpoint returns 200.
#   mode=plain   : --no-systemd, started by start.sh
#   mode=systemd : systemd as PID1 (--privileged); install -> stop.sh -> reinstall, asserting restart really runs it
#
# 用法 / Usage: scripts/ci-install-smoke.sh <plain|systemd> [release-tag]
#   DOCKER (default: docker), STX_SMOKE_IMAGE (default: debian:bookworm)

set -euo pipefail

mode="${1:?usage: $0 <plain|systemd> [release-tag]}"
tag="${2:-v1.2.0}"
DOCKER="${DOCKER:-docker}"
image="${STX_SMOKE_IMAGE:-debian:bookworm}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
name="stx-install-smoke-${mode}-$$"
work="$(mktemp -d)"
cleanup() { $DOCKER rm -f "$name" >/dev/null 2>&1 || true; rm -rf "$work"; }
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
  got="$(sha256sum /opt/stx/scripts/stx-java-proxy.sh 2>/dev/null | cut -d\  -f1)"
  [[ -n "$WANT_SHA" ]] && check "stx-java-proxy.sh sha256 matches release SHA256SUMS ($got)" "[[ \"$got\" == \"$WANT_SHA\" ]]"
  port="$(api_port)"
  for _ in $(seq 1 30); do
    code="$(curl -s -o /tmp/asset.sh -w "%{http_code}" "http://127.0.0.1:${port}/api/v1/agent/assets/stx-java-proxy.sh" || true)"
    [[ "$code" == 200 ]] && break; sleep 2
  done
  check "GET /api/v1/agent/assets/stx-java-proxy.sh -> $code" "[[ \"$code\" == 200 ]]"
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
  *)
    echo "unknown mode: $mode" >&2
    exit 2
    ;;
esac
echo "[smoke] mode=$mode tag=$tag passed"
