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
#   1. download-bundle.sh 默认总是组装 Node（即便打包机已有 Node ≥ 18.18）；--without-node 才跳过；deps 缺 Node 包时失败。
#      download-bundle.sh always bundles Node by default (even when the packaging host has Node >= 18.18);
#      only --without-node skips it; a missing node package on deps is fatal.
#   2. install.sh --offline 优先使用包内 Node；--without-node 包在目标机无合格 Node 时明确报错。
#      install.sh --offline prefers the bundled Node; a --without-node bundle fails clearly without a usable host Node.
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

# ---------- fake release assets / 伪 Release 资产 ----------
REL="$T/releases"
VER=v9.9.9
mkdir -p "$REL/$VER" "$REL/deps" "$T/build/frontend" "$T/build/node-v22.0.0-linux-x64"
printf '#!/bin/sh\necho stx\n' >"$REL/$VER/stx-linux-amd64"
echo 'console.log("fe")' >"$T/build/frontend/server.js"
tar -C "$T/build/frontend" -czf "$REL/$VER/frontend-standalone-${VER}-linux-amd64.tar.gz" .
make_fake_node "$T/build/node-v22.0.0-linux-x64/bin" v22.0.0
tar -C "$T/build" -czf "$REL/deps/node-22-official-linux-amd64.tar.gz" node-v22.0.0-linux-x64
cp "$REL/$VER/stx-linux-amd64" "$REL/$VER/stx-linux-arm64"
cp "$REL/$VER/frontend-standalone-${VER}-linux-amd64.tar.gz" "$REL/$VER/frontend-standalone-${VER}-linux-arm64.tar.gz"
cp "$REL/deps/node-22-official-linux-amd64.tar.gz" "$REL/deps/node-22-official-linux-arm64.tar.gz"

# ---------- fake repo root with stubbed download-lib / 桩化 download-lib 的伪仓库 ----------
FAKE="$T/root"
mkdir -p "$FAKE/install/bin/lib"
cp "$ROOT_DIR/install/download-bundle.sh" "$ROOT_DIR/install/install-online.sh" "$ROOT_DIR/install/install.sh" "$ROOT_DIR/install/install-core.sh" "$FAKE/install/"
cp "$ROOT_DIR/install/download-lib.sh" "$FAKE/install/download-lib.real.sh"
cp "$ROOT_DIR/install/bin/start.sh" "$ROOT_DIR/install/bin/stop.sh" "$ROOT_DIR/install/bin/status.sh" "$FAKE/install/bin/"
cp "$ROOT_DIR/install/bin/lib/observability.sh" "$FAKE/install/bin/lib/"
cp "$ROOT_DIR/config.example.yaml" "$FAKE/config.example.yaml"
cat >"$FAKE/install/download-lib.sh" <<STUB
# shellcheck shell=bash
source "$FAKE/install/download-lib.real.sh"  # absolute: the stub also gets copied into work dirs
stx_detect_glibc_version() { echo "\${FAKE_GLIBC:-2.35}"; }
stx_detect_download_region() { echo global; }
stx_select_mirror_prefix() { STX_SELECTED_MIRROR_PREFIX=""; }
stx_stage_java_proxy_script() { return 0; }
stx_download_release_asset() {
  local src="$REL/\$1/\$2"
  [[ -f "\$src" ]] || { echo "[stub] 404 \$1/\$2" >&2; return 1; }
  mkdir -p "\$(dirname "\$3")" && cp "\$src" "\$3"
}
STUB

run_bundle() {
  local out="$1"; shift
  OUTPUT_DIR="$out" bash "$FAKE/install/download-bundle.sh" --version "$VER" --arch amd64 \
    --node-variant official --without-observability --deps-tag deps "$@"
}
BUNDLE_NAME="stx-offline-bundle-${VER}-linux-amd64"

# ---------- 1a. default: packaging host HAS node >= 18.18, node is still bundled ----------
make_fake_node "$T/hostnode" v20.11.1
out="$T/out-default"
if PATH="$T/hostnode:$NONODE" run_bundle "$out" >"$T/bundle-default.log" 2>&1; then
  ok "bundle default exits 0"
else
  bad "bundle default failed"; sed 's/^/    | /' "$T/bundle-default.log"
fi
assert_file "bundle default includes node even when host has node v20" \
  "$out/$BUNDLE_NAME/packages/node-22-official-linux-amd64.tar.gz"
assert_grep "bundle MANIFEST with_node=true" '"with_node": true' "$out/$BUNDLE_NAME/MANIFEST.json"
assert_no_grep "bundle default does not skip node" "skip node package" "$T/bundle-default.log"

# ---------- 1b. --without-node skips node ----------
out="$T/out-nonode"
if PATH="$T/hostnode:$NONODE" run_bundle "$out" --without-node >"$T/bundle-nonode.log" 2>&1; then
  ok "bundle --without-node exits 0"
else
  bad "bundle --without-node failed"; sed 's/^/    | /' "$T/bundle-nonode.log"
fi
if compgen -G "$out/$BUNDLE_NAME/packages/node-*" >/dev/null; then
  bad "bundle --without-node must not include node"
else
  ok "bundle --without-node has no node package"
fi
assert_grep "bundle MANIFEST with_node=false" '"with_node": false' "$out/$BUNDLE_NAME/MANIFEST.json"

# ---------- 1c. node asset missing on deps is fatal (no silent node-less bundle) ----------
if PATH="$NONODE" run_bundle "$T/out-missing" --node-variant glibc217 >"$T/bundle-missing.log" 2>&1; then
  bad "bundle must fail when node package is missing on deps"
else
  ok "bundle fails when node package is missing"
fi
assert_grep "bundle missing-node error mentions --without-node" "--without-node" "$T/bundle-missing.log"
assert_grep "bundle missing glibc217 hints --node-variant official" "--node-variant official" "$T/bundle-missing.log"

# ---------- 1c2. node sha256 mismatch is fatal ----------
echo "0000000000000000000000000000000000000000000000000000000000000000  node-22-official-linux-amd64.tar.gz" \
  >"$REL/deps/node-22-official-linux-amd64.tar.gz.sha256"
if PATH="$NONODE" run_bundle "$T/out-badsha" >"$T/bundle-badsha.log" 2>&1; then
  bad "bundle must fail on node sha256 mismatch"
else
  ok "bundle fails on node sha256 mismatch"
fi
# 正确的 sha256 通过校验。/ A correct sha256 passes.
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$REL/deps" && sha256sum node-22-official-linux-amd64.tar.gz >node-22-official-linux-amd64.tar.gz.sha256)
else
  (cd "$REL/deps" && shasum -a 256 node-22-official-linux-amd64.tar.gz >node-22-official-linux-amd64.tar.gz.sha256)
fi
if PATH="$NONODE" run_bundle "$T/out-goodsha" >"$T/bundle-goodsha.log" 2>&1; then
  ok "bundle passes with correct node sha256"
else
  bad "bundle failed with correct node sha256"; sed 's/^/    | /' "$T/bundle-goodsha.log"
fi
rm -f "$REL/deps/node-22-official-linux-amd64.tar.gz.sha256"

# ---------- 1c3. arm64 + old glibc auto-picks official (no glibc217 for arm64) ----------
out="$T/out-arm64"
if PATH="$NONODE" FAKE_GLIBC=2.17 OUTPUT_DIR="$out" bash "$FAKE/install/download-bundle.sh" --version "$VER" \
  --arch arm64 --without-observability --deps-tag deps >"$T/bundle-arm64.log" 2>&1; then
  ok "bundle arm64 + glibc 2.17 exits 0"
else
  bad "bundle arm64 + glibc 2.17 failed"; sed 's/^/    | /' "$T/bundle-arm64.log"
fi
assert_file "bundle arm64 uses node-22-official" "$out/stx-offline-bundle-${VER}-linux-arm64/packages/node-22-official-linux-arm64.tar.gz"
assert_grep "bundle arm64 MANIFEST node_variant=official" '"node_variant": "official"' \
  "$out/stx-offline-bundle-${VER}-linux-arm64/MANIFEST.json"

# ---------- 1d. help documents --without-node ----------
bash "$FAKE/install/download-bundle.sh" --help >"$T/help.log" 2>&1 || true
assert_grep "--help documents --without-node" "--without-node" "$T/help.log"
assert_grep "--help says node is bundled by default" "default: always bundled" "$T/help.log"

# 真实 bundle 携带的是未桩化的 download-lib.sh。/ A real bundle ships the un-stubbed download-lib.sh.
for b in "$T/out-default/$BUNDLE_NAME" "$T/out-nonode/$BUNDLE_NAME"; do
  cp "$ROOT_DIR/install/download-lib.sh" "$b/download-lib.sh"
done

# ---------- 2a. install bundled node on a host WITHOUT node ----------
inst="$T/inst-bundled"
if PATH="$NONODE" bash "$T/out-default/$BUNDLE_NAME/install.sh" --offline --install-dir "$inst" \
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

# ---------- 2b. bundled node is preferred even if host has node ----------
inst="$T/inst-prefer"
PATH="$T/hostnode:$NONODE" bash "$T/out-default/$BUNDLE_NAME/install.sh" --offline --install-dir "$inst" \
  --no-start --no-systemd --without-observability >"$T/install-prefer.log" 2>&1 || true
if [[ -x "$inst/runtime/node/bin/node" && "$("$inst/runtime/node/bin/node" -v)" == "v22.0.0" ]]; then
  ok "install prefers bundled node over host node"
else
  bad "install did not prefer bundled node"; sed 's/^/    | /' "$T/install-prefer.log"
fi

# ---------- 2c. --without-node bundle + host without node => clear error ----------
inst="$T/inst-nonode"
if PATH="$NONODE" bash "$T/out-nonode/$BUNDLE_NAME/install.sh" --offline --install-dir "$inst" \
  --no-start --no-systemd --without-observability >"$T/install-nonode.log" 2>&1; then
  bad "install --without-node bundle on node-less host must fail"
else
  ok "install --without-node bundle on node-less host fails"
fi
assert_grep "install error explains --without-node" "built with --without-node" "$T/install-nonode.log"

# ---------- 2d. --without-node bundle + host node too old => clear error ----------
make_fake_node "$T/oldnode" v16.20.2
if PATH="$T/oldnode:$NONODE" bash "$T/out-nonode/$BUNDLE_NAME/install.sh" --offline --install-dir "$T/inst-old" \
  --no-start --no-systemd --without-observability >"$T/install-old.log" 2>&1; then
  bad "install with old host node must fail"
else
  ok "install with old host node fails"
fi
assert_grep "install error shows found version" "found: v16.20.2" "$T/install-old.log"

# ---------- 2e. --without-node bundle + host node >= 18.18 => symlinked ----------
inst="$T/inst-host"
if PATH="$T/hostnode:$NONODE" bash "$T/out-nonode/$BUNDLE_NAME/install.sh" --offline --install-dir "$inst" \
  --no-start --no-systemd --without-observability >"$T/install-host.log" 2>&1; then
  ok "install --without-node bundle with host node v20 exits 0"
else
  bad "install with host node v20 failed"; sed 's/^/    | /' "$T/install-host.log"
fi
if [[ -L "$inst/runtime/node/bin/node" && "$(readlink "$inst/runtime/node/bin/node")" == "$T/hostnode/node" ]]; then
  ok "install links host node when bundle has none"
else
  bad "install did not link host node"
fi

# ---------- 2f. install-online.sh: consistent Node errors ----------
run_online() {
  STX_DOWNLOAD_WORK_DIR="$T/online-work-$1" bash "$FAKE/install/install-online.sh" --version "$VER" --arch amd64 \
    --install-dir "$T/online-$1" --no-start --no-systemd --without-observability --deps-tag deps "${@:2}"
}
if PATH="$NONODE" run_online nonode --node-variant official --without-node >"$T/online-nonode.log" 2>&1; then
  bad "install-online --without-node on node-less host must fail"
else
  ok "install-online --without-node on node-less host fails"
fi
assert_grep "install-online --without-node error text" "no usable system Node (need >= 18.18, found: none)" "$T/online-nonode.log"
assert_no_grep "install-online --without-node fails before downloading" "using release tag" "$T/online-nonode.log"
if PATH="$T/oldnode:$NONODE" run_online old --node-variant official --without-node >"$T/online-old.log" 2>&1; then
  bad "install-online --without-node with node v16 must fail"
else
  ok "install-online --without-node with node v16 fails"
fi
assert_grep "install-online shows found v16" "found: v16.20.2" "$T/online-old.log"
if PATH="$NONODE" run_online missing --node-variant glibc217 >"$T/online-missing.log" 2>&1; then
  bad "install-online must fail when node package is missing"
else
  ok "install-online fails when node package is missing"
fi
assert_grep "install-online missing node hints --node-variant official" "--node-variant official" "$T/online-missing.log"
assert_no_grep "install-online no longer just warns" "install may require system node" "$T/online-missing.log"
if PATH="$NONODE" run_online ok --node-variant official >"$T/online-ok.log" 2>&1; then
  ok "install-online downloads node on node-less host"
else
  bad "install-online on node-less host failed"; sed 's/^/    | /' "$T/online-ok.log"
fi
if [[ -x "$T/online-ok/runtime/node/bin/node" && "$("$T/online-ok/runtime/node/bin/node" -v)" == "v22.0.0" ]]; then
  ok "install-online staged downloaded node"
else
  bad "install-online did not stage downloaded node"
fi
if PATH="$T/hostnode:$NONODE" run_online host --node-variant official --without-node >"$T/online-host.log" 2>&1; then
  ok "install-online --without-node with host node v20 exits 0"
else
  bad "install-online --without-node with host node v20 failed"; sed 's/^/    | /' "$T/online-host.log"
fi

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
