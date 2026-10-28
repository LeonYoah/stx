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

# 离线 bundle 与本地三件套启动的无网回归测试（桩掉 Release 下载）。
# Network-free regression tests for the offline bundle and local observability start (release downloads stubbed).
#   1. build-offline-bundle.sh 组装 amd64 / arm64 / centos7 三个离线包（布局、sha256、latest 别名、Node 变体）；
#      缺 Node 包、sha256 不符、动态链接二进制时失败。
#      build-offline-bundle.sh assembles the amd64 / arm64 / centos7 bundles (layout, sha256, latest alias, Node
#      variant); missing Node, bad sha256 or dynamically linked binaries are fatal.
#   2. install.sh 优先使用包内 Node；--without-node 时只接受本机 Node ≥ 18.18；glibc < 2.28 拒绝官方 Node 包；
#      install-online.sh --bundle-file 与 download-bundle.sh --print-urls。
#      install.sh prefers the bundled Node; --without-node only accepts a host Node >= 18.18; glibc < 2.28 refuses
#      the official-Node bundle; install-online.sh --bundle-file and download-bundle.sh --print-urls.
#   3. alertmanager 启动参数含 --cluster.listen-address=（单机关闭 gossip）。
#      alertmanager is launched with --cluster.listen-address= (gossip disabled for single node).
#   4. stx_obs_init_defaults 不再输出 cp 'are the same file' 告警。
#      stx_obs_init_defaults no longer emits cp 'are the same file' warnings.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT

fail=0
ok() { echo "OK $*"; }
bad() { echo "FAIL $*"; fail=1; }
assert_file() { if [[ -e "$2" ]]; then ok "$1"; else bad "$1: missing $2"; fi; }
assert_no_file() { if [[ ! -e "$2" ]]; then ok "$1"; else bad "$1: unexpected $2"; fi; }
assert_grep() { if grep -qF -- "$2" "$3" 2>/dev/null; then ok "$1"; else bad "$1: [$2] not in $3"; sed 's/^/    | /' "$3" 2>/dev/null || true; fi; }
assert_no_grep() { if ! grep -qF -- "$2" "$3" 2>/dev/null; then ok "$1"; else bad "$1: [$2] unexpectedly in $3"; sed 's/^/    | /' "$3" 2>/dev/null || true; fi; }

# 一个不含 node 的 PATH：把现有 PATH 中除 node 以外的可执行文件软链到 $T/nonode。
# A PATH without node: symlink every executable from the current PATH except node into $T/nonode.
NONODE="$T/nonode"
mkdir -p "$NONODE"
IFS=: read -r -a _path_dirs <<<"$PATH"
for d in "${_path_dirs[@]}"; do
  [[ -d "$d" ]] || continue
  for f in "$d"/*; do
    n="$(basename "$f")"
    [[ "$n" == node || "$n" == nodejs || -e "$NONODE/$n" || ! -x "$f" || -d "$f" ]] && continue
    ln -s "$f" "$NONODE/$n"
  done
done

# 伪 node：打印给定版本。/ Fake node printing the given version.
make_fake_node() {
  local dir="$1" ver="$2"
  mkdir -p "$dir"
  printf '#!/bin/sh\nif [ "$1" = "-v" ]; then echo %s; exit 0; fi\nexit 0\n' "$ver" >"$dir/node"
  chmod +x "$dir/node"
}

# ---------- fake build outputs + deps / 伪构建产物与 deps ----------
REL="$T/releases"
DEPS="$T/deps"
VER=v9.9.9
mkdir -p "$REL" "$DEPS" "$T/build/frontend" "$T/build/node-v22.0.0-linux-x64" "$T/build/node-v18.20.8-linux-x64-glibc217"
printf '#!/bin/sh\necho stx\n' >"$REL/stx-linux-amd64"
echo 'console.log("fe")' >"$T/build/frontend/server.js"
tar -C "$T/build/frontend" -czf "$REL/frontend-standalone-${VER}-linux-amd64.tar.gz" .
cp "$REL/stx-linux-amd64" "$REL/stx-linux-arm64"
cp "$REL/stx-linux-amd64" "$REL/stx-agent-linux-amd64"
cp "$REL/stx-linux-amd64" "$REL/stx-agent-linux-arm64"
cp "$REL/frontend-standalone-${VER}-linux-amd64.tar.gz" "$REL/frontend-standalone-${VER}-linux-arm64.tar.gz"
echo jar >"$REL/stx-java-proxy-v2.jar"
echo jar >"$REL/stx-java-proxy-v3.jar"
make_fake_node "$T/build/node-v22.0.0-linux-x64/bin" v22.0.0
make_fake_node "$T/build/node-v18.20.8-linux-x64-glibc217/bin" v18.20.8
tar -C "$T/build" -czf "$DEPS/node-22-official-linux-amd64.tar.gz" node-v22.0.0-linux-x64
tar -C "$T/build" -czf "$DEPS/node-22-official-linux-arm64.tar.gz" node-v22.0.0-linux-x64
tar -C "$T/build" -czf "$DEPS/node-18-glibc217-linux-amd64.tar.gz" node-v18.20.8-linux-x64-glibc217

run_build() {
  bash "$ROOT_DIR/install/build-offline-bundle.sh" --version "$VER" --from-dir "$REL" --deps-dir "$DEPS" \
    --without-observability --output-dir "$T/out" --cache-dir "$T/cache" --keep-dir "$@"
}

# ---------- 1a. build all three variants from local outputs ----------
if run_build --variant all --alias >"$T/build.log" 2>&1; then
  ok "build-offline-bundle all variants exits 0"
else
  bad "build-offline-bundle failed"; sed 's/^/    | /' "$T/build.log"
fi
for plat in linux-amd64 linux-arm64 linux-amd64-centos7; do
  assert_file "bundle $plat tarball" "$T/out/stx-${VER}-${plat}-offline.tar.gz"
  assert_file "bundle $plat sha256" "$T/out/stx-${VER}-${plat}-offline.tar.gz.sha256"
  assert_file "bundle $plat latest alias" "$T/out/stx-offline-${plat}.tar.gz"
  assert_grep "alias $plat sha256 names the alias file" "stx-offline-${plat}.tar.gz" "$T/out/stx-offline-${plat}.tar.gz.sha256"
  (cd "$T/out" && sha256sum -c "stx-${VER}-${plat}-offline.tar.gz.sha256" >/dev/null 2>&1) \
    && ok "bundle $plat sha256 verifies" || bad "bundle $plat sha256 does not verify"
  d="$T/out/stx-${VER}-${plat}-offline"
  for f in install.sh install-core.sh download-lib.sh config.example.yaml MANIFEST.json README-OFFLINE.md SHA256SUMS \
    bin/start.sh bin/stop.sh bin/status.sh bin/lib/observability.sh scripts/stx-java-proxy.sh; do
    assert_file "bundle $plat has $f" "$d/$f"
  done
done
C7="$T/out/stx-${VER}-linux-amd64-centos7-offline"
A64="$T/out/stx-${VER}-linux-amd64-offline"
assert_file "centos7 bundle uses node-18-glibc217" "$C7/packages/node-18-glibc217-linux-amd64.tar.gz"
assert_grep "centos7 MANIFEST min_glibc 2.17" '"min_glibc": "2.17"' "$C7/MANIFEST.json"
assert_grep "centos7 MANIFEST node_version" '"node_version": "v18.20.8"' "$C7/MANIFEST.json"
assert_file "amd64 bundle uses node-22-official" "$A64/packages/node-22-official-linux-amd64.tar.gz"
assert_grep "amd64 MANIFEST min_glibc 2.28" '"min_glibc": "2.28"' "$A64/MANIFEST.json"
assert_file "arm64 bundle uses node-22-official arm64" \
  "$T/out/stx-${VER}-linux-arm64-offline/packages/node-22-official-linux-arm64.tar.gz"

# ---------- 1b. missing node package is fatal ----------
mv "$DEPS/node-18-glibc217-linux-amd64.tar.gz" "$T/node18.bak"
if run_build --variant centos7 >"$T/build-missing.log" 2>&1; then
  bad "build must fail when the node package is missing"
else
  ok "build fails when the node package is missing"
fi
assert_grep "missing node error names the asset" "node-18-glibc217-linux-amd64.tar.gz" "$T/build-missing.log"
mv "$T/node18.bak" "$DEPS/node-18-glibc217-linux-amd64.tar.gz"

# ---------- 1c. node sha256 mismatch in deps dir is fatal ----------
echo "0000000000000000000000000000000000000000000000000000000000000000  node-22-official-linux-amd64.tar.gz" \
  >"$DEPS/node-22-official-linux-amd64.tar.gz.sha256"
if run_build --variant amd64 >"$T/build-badsha.log" 2>&1; then
  bad "build must fail on node sha256 mismatch"
else
  ok "build fails on node sha256 mismatch"
fi
rm -f "$DEPS/node-22-official-linux-amd64.tar.gz.sha256"

# ---------- 1d. dynamically linked binary is rejected ----------
cp "$REL/stx-linux-amd64" "$T/stx.bak"
printf '#!/bin/sh\n# /lib64/ld-linux-x86-64.so.2\n' >"$REL/stx-linux-amd64"
if PATH="$NONODE" bash -c 'command -v readelf >/dev/null' 2>/dev/null; then
  ok "readelf present; dynamic check uses INTERP (skip grep heuristic test)"
else
  if run_build --variant centos7 >"$T/build-dyn.log" 2>&1; then
    bad "build must reject a dynamically linked stx binary"
  else
    ok "build rejects a dynamically linked stx binary"
  fi
fi
cp "$T/stx.bak" "$REL/stx-linux-amd64"

# ---------- 1e. help documents variants ----------
bash "$ROOT_DIR/install/build-offline-bundle.sh" --help >"$T/help.log" 2>&1 || true
assert_grep "--help documents centos7" "centos7" "$T/help.log"

# ---------- 2a. install bundled node on a host WITHOUT node ----------
inst="$T/inst-bundled"
if PATH="$NONODE" bash "$A64/install.sh" -y --install-dir "$inst" \
  --no-start --no-systemd --without-observability >"$T/install-bundled.log" 2>&1; then
  ok "install bundled-node offline exits 0"
else
  bad "install bundled-node offline failed"; sed 's/^/    | /' "$T/install-bundled.log"
fi
if [[ -x "$inst/runtime/node/bin/node" && ! -L "$inst/runtime/node/bin/node" ]] \
  && [[ "$("$inst/runtime/node/bin/node" -v)" == "v22.0.0" ]]; then
  ok "install uses bundled node runtime"
else
  bad "install did not stage bundled node runtime"
fi
assert_grep "install logs numbered steps" "[1/7]" "$T/install-bundled.log"
assert_grep "install prints summary" "STX 安装完成" "$T/install-bundled.log"
assert_grep "install prints default password" "admin123" "$T/install-bundled.log"
assert_grep "install prints next steps" "后续步骤" "$T/install-bundled.log"
assert_file "install writes stx.env" "$inst/stx.env"

# ---------- 2b. bundled node is preferred even if host has node ----------
make_fake_node "$T/hostnode" v20.11.1
inst="$T/inst-prefer"
PATH="$T/hostnode:$NONODE" bash "$A64/install.sh" -y --install-dir "$inst" \
  --no-start --no-systemd --without-observability >"$T/install-prefer.log" 2>&1 || true
if [[ -x "$inst/runtime/node/bin/node" && "$("$inst/runtime/node/bin/node" -v)" == "v22.0.0" ]]; then
  ok "install prefers bundled node over host node"
else
  bad "install did not prefer bundled node"; sed 's/^/    | /' "$T/install-prefer.log"
fi

# ---------- 2c. --without-node + host without node => clear error ----------
if PATH="$NONODE" bash "$A64/install.sh" -y --install-dir "$T/inst-nonode" --without-node \
  --no-start --no-systemd --without-observability >"$T/install-nonode.log" 2>&1; then
  bad "install --without-node on node-less host must fail"
else
  ok "install --without-node on node-less host fails"
fi
assert_grep "install --without-node error explains Node >= 18.18" "need >= 18.18" "$T/install-nonode.log"
assert_grep "failure names the failed step" "安装失败：第 3/7 步" "$T/install-nonode.log"

# ---------- 2d. --without-node + host node too old => clear error ----------
make_fake_node "$T/oldnode" v16.20.2
if PATH="$T/oldnode:$NONODE" bash "$A64/install.sh" -y --install-dir "$T/inst-old" --without-node \
  --no-start --no-systemd --without-observability >"$T/install-old.log" 2>&1; then
  bad "install with old host node must fail"
else
  ok "install with old host node fails"
fi
assert_grep "install error shows found version" "found: v16.20.2" "$T/install-old.log"

# ---------- 2e. --without-node + host node >= 18.18 => symlinked ----------
inst="$T/inst-host"
if PATH="$T/hostnode:$NONODE" bash "$A64/install.sh" -y --install-dir "$inst" --without-node \
  --no-start --no-systemd --without-observability >"$T/install-host.log" 2>&1; then
  ok "install --without-node with host node v20 exits 0"
else
  bad "install with host node v20 failed"; sed 's/^/    | /' "$T/install-host.log"
fi
if [[ -L "$inst/runtime/node/bin/node" && "$(readlink "$inst/runtime/node/bin/node")" == "$T/hostnode/node" ]]; then
  ok "install links host node with --without-node"
else
  bad "install did not link host node"
fi

# ---------- 2f. glibc < 2.28 host refuses the official-Node bundle ----------
mkdir -p "$T/oldglibc"
printf '#!/bin/sh\necho "ldd (GNU libc) 2.17"\n' >"$T/oldglibc/ldd"; chmod +x "$T/oldglibc/ldd"
if PATH="$T/oldglibc:$NONODE" bash "$A64/install.sh" -y --install-dir "$T/inst-glibc" \
  --no-start --no-systemd --without-observability >"$T/install-glibc.log" 2>&1; then
  bad "official-Node bundle must be refused on glibc 2.17"
else
  ok "official-Node bundle refused on glibc 2.17"
fi
assert_grep "glibc error points to the centos7 bundle" "centos7-offline" "$T/install-glibc.log"
if PATH="$T/oldglibc:$NONODE" bash "$C7/install.sh" -y --install-dir "$T/inst-c7" \
  --no-start --no-systemd --without-observability >"$T/install-c7.log" 2>&1; then
  ok "centos7 bundle installs on glibc 2.17"
else
  bad "centos7 bundle failed on glibc 2.17"; sed 's/^/    | /' "$T/install-c7.log"
fi

# ---------- 2g. install-online.sh --bundle-file (no network) ----------
if PATH="$NONODE" bash "$ROOT_DIR/install/install-online.sh" -y --bundle-file "$T/out/stx-${VER}-linux-amd64-offline.tar.gz" \
  --install-dir "$T/online-inst" --no-start --no-systemd --without-observability --work-dir "$T" >"$T/online.log" 2>&1; then
  ok "install-online --bundle-file exits 0"
else
  bad "install-online --bundle-file failed"; sed 's/^/    | /' "$T/online.log"
fi
assert_file "install-online installed stx" "$T/online-inst/stx"
assert_grep "install-online logs download steps" "下载 [1/2]" "$T/online.log"

# ---------- 2h. download-bundle.sh --print-urls lists 6 lines with latest alias ----------
bash "$ROOT_DIR/install/download-bundle.sh" --variant centos7 --print-urls >"$T/urls.log" 2>&1 || true
assert_grep "print-urls v4 line" 'wget "https://v4.gh-proxy.org/https://github.com/LeonYoah/stx/releases/latest/download/stx-offline-linux-amd64-centos7.tar.gz"' "$T/urls.log"
assert_grep "print-urls direct line" 'wget "https://github.com/LeonYoah/stx/releases/latest/download/stx-offline-linux-amd64-centos7.tar.gz"' "$T/urls.log"
if [[ "$(grep -c '^wget ' "$T/urls.log")" -eq 6 ]]; then ok "print-urls prints 6 lines"; else bad "print-urls line count"; fi
bash "$ROOT_DIR/install/download-bundle.sh" --variant arm64 --version v1.3.0 --print-urls >"$T/urls2.log" 2>&1 || true
assert_grep "print-urls versioned name" "releases/download/v1.3.0/stx-v1.3.0-linux-arm64-offline.tar.gz" "$T/urls2.log"

# ---------- 2i. curl | bash：不信任当前目录，按版本拉取 download-lib.sh ----------
# curl | bash: never trust cwd; fetch download-lib.sh matching --version, then the baked release tag, then main.
mkdir -p "$T/pipe/bin" "$T/pipe/cwd"
printf 'echo EVIL-CWD-LIB; exit 9\n' >"$T/pipe/cwd/download-lib.sh"
cat >"$T/pipe/bin/curl" <<CURL
#!/usr/bin/env bash
out=""; url=""
while [[ \$# -gt 0 ]]; do
  case "\$1" in -o) out="\$2"; shift 2 ;; -w) shift 2 ;; http*) url="\$1"; shift ;; *) shift ;; esac
done
echo "\$url" >>"$T/pipe/calls"
case "\$url" in
  https://raw.githubusercontent.com/*/v7.7.7/install/download-lib.sh) cp "$ROOT_DIR/install/download-lib.sh" "\$out"; printf 200 ;;
  https://raw.githubusercontent.com/*/v6.6.6/install/download-lib.sh) echo old >"\$out"; printf 200 ;;
  *) printf 404 ;;
esac
CURL
chmod +x "$T/pipe/bin/curl"
sed 's/^STX_SCRIPT_RELEASE_TAG=""$/STX_SCRIPT_RELEASE_TAG="v7.7.7"/' "$ROOT_DIR/install/install-online.sh" >"$T/pipe/baked.sh"
(cd "$T/pipe/cwd" && PATH="$T/pipe/bin:$PATH" bash -s -- --version v6.6.6 --help <"$T/pipe/baked.sh" >"$T/pipe/out.log" 2>&1) || true
if grep -q EVIL-CWD-LIB "$T/pipe/out.log"; then bad "curl|bash must not source download-lib.sh from cwd"; else ok "curl|bash ignores download-lib.sh in cwd"; fi
assert_grep "curl|bash usage printed via fetched lib" "STX 一键安装" "$T/pipe/out.log"
if [[ "$(sed -n 1p "$T/pipe/calls")" == *"/v6.6.6/install/download-lib.sh" && "$(sed -n 2p "$T/pipe/calls")" == *"/v7.7.7/install/download-lib.sh" ]]; then
  ok "lib fetch order: --version tag, then baked release tag"
else
  bad "lib fetch order wrong"; sed 's/^/    | /' "$T/pipe/calls"
fi
assert_grep "fallback warns about the version mismatch" "using v7.7.7" "$T/pipe/out.log"
grep -q 'STX_SCRIPT_RELEASE_TAG=""' "$ROOT_DIR/install/install-online.sh" && grep -q 'STX_SCRIPT_RELEASE_TAG=""' "$ROOT_DIR/install/download-bundle.sh" \
  && ok "release-tag placeholder present for package-release.sh" || bad "release-tag placeholder missing"

# ---------- 3. alertmanager launched with --cluster.listen-address= ----------
# 目录含空格，验证参数已正确加引号。/ Path with spaces verifies the args are quoted.
OBS_BASE="$T/obs dir"
mkdir -p "$OBS_BASE/observability/"{prometheus,alertmanager/data,grafana/bin,grafana/conf}
for b in prometheus/prometheus alertmanager/alertmanager grafana/bin/grafana; do
  printf '#!/bin/sh\nfor a in "$@"; do echo "$a"; done > "%s.args"\nexit 0\n' "$OBS_BASE/observability/$b" \
    >"$OBS_BASE/observability/$b"
  chmod +x "$OBS_BASE/observability/$b"
done
touch "$OBS_BASE/observability/prometheus/prometheus.yml" "$OBS_BASE/observability/alertmanager/alertmanager.yml" \
  "$OBS_BASE/observability/grafana/conf/grafana.ini"
(
  # shellcheck disable=SC2034  # read by sourced observability.sh
  BASE_DIR="$OBS_BASE"
  # shellcheck source=../install/bin/lib/observability.sh
  source "$ROOT_DIR/install/bin/lib/observability.sh"
  stx_obs_start_stack >/dev/null
)
am_args="$OBS_BASE/observability/alertmanager/alertmanager.args"
for _ in 1 2 3 4 5; do [[ -s "$am_args" ]] && break; sleep 1; done
if [[ -f "$am_args" ]] && grep -qx -- "--cluster.listen-address=" "$am_args"; then
  ok "alertmanager started with --cluster.listen-address="
else
  bad "alertmanager args lack --cluster.listen-address="; [[ -f "$am_args" ]] && sed 's/^/    | /' "$am_args"
fi
if [[ -f "$am_args" ]] && grep -qx -- "--web.listen-address=:9093" "$am_args"; then
  ok "alertmanager keeps --web.listen-address=:9093"
else
  bad "alertmanager lost --web.listen-address"
fi
if [[ -f "$am_args" ]] && grep -qxF -- "--config.file=$OBS_BASE/observability/alertmanager/alertmanager.yml" "$am_args"; then
  ok "alertmanager --config.file survives a path with spaces"
else
  bad "alertmanager --config.file broken by spaces"
fi
for comp in prometheus/prometheus grafana/bin/grafana; do
  for _ in 1 2 3 4 5; do [[ -s "$OBS_BASE/observability/$comp.args" ]] && break; sleep 1; done
  if grep -qF -- "$OBS_BASE/observability/" "$OBS_BASE/observability/$comp.args" 2>/dev/null; then
    ok "$comp started with quoted paths"
  else
    bad "$comp not started with quoted paths"
  fi
done
if grep -qF -- "--cluster.listen-address=" "$ROOT_DIR/scripts/manage-deps-obs.sh"; then
  ok "manage-deps-obs.sh alertmanager has --cluster.listen-address="
else
  bad "manage-deps-obs.sh alertmanager lacks --cluster.listen-address="
fi
if grep -qF -- "- --cluster.listen-address=" "$ROOT_DIR/deploy/docker/compose.base.yml"; then
  ok "compose alertmanager has --cluster.listen-address="
else
  bad "compose alertmanager lacks --cluster.listen-address="
fi

# ---------- 4. init defaults: no cp 'are the same file' warnings ----------
INIT_BASE="$T/init"
mkdir -p "$INIT_BASE/observability/"{prometheus,alertmanager,grafana}
cp -a "$ROOT_DIR/install/observability/." "$INIT_BASE/observability/"
chmod +x "$INIT_BASE/observability/render.sh"
init_rc=0
(
  # shellcheck source=../install/bin/lib/observability.sh
  source "$ROOT_DIR/install/bin/lib/observability.sh"
  stx_obs_init_defaults "$INIT_BASE/observability"
) >"$T/init.log" 2>&1 || init_rc=$?
if [[ "$init_rc" == 0 ]]; then ok "stx_obs_init_defaults exits 0"; else bad "stx_obs_init_defaults rc=$init_rc"; fi
assert_no_grep "stx_obs_init_defaults has no 'same file' warnings" "are the same file" "$T/init.log"
assert_file "grafana dashboards rendered" "$INIT_BASE/observability/grafana/provisioning/dashboards/default.yml"
assert_file "grafana.ini rendered" "$INIT_BASE/observability/grafana/conf/grafana.ini"

# stx_obs_copy_tree：不同目录仍会复制。/ stx_obs_copy_tree still copies between distinct dirs.
mkdir -p "$T/ct/src/sub" && echo x >"$T/ct/src/sub/f"
(
  # shellcheck source=../install/bin/lib/observability.sh
  source "$ROOT_DIR/install/bin/lib/observability.sh"
  stx_obs_copy_tree "$T/ct/src" "$T/ct/dst"
  stx_obs_copy_tree "$T/ct/src" "$T/ct/src"
  stx_obs_copy_tree "$T/ct/missing" "$T/ct/dst2"
) >"$T/ct.log" 2>&1 || bad "stx_obs_copy_tree failed"
assert_file "stx_obs_copy_tree copies distinct dirs" "$T/ct/dst/sub/f"
assert_no_grep "stx_obs_copy_tree same dir is silent" "same file" "$T/ct.log"
assert_no_file "stx_obs_copy_tree skips missing src" "$T/ct/dst2"

exit "$fail"
