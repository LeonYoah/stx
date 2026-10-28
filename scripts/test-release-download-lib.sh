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

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=../install/download-lib.sh
source "$ROOT_DIR/install/download-lib.sh"
# shellcheck source=../install/install-core.sh
source "$ROOT_DIR/install/install-core.sh"

fail=0

url="$(stx_build_github_release_url "v1.2.0" "stx-linux-amd64" "")"
expected="https://github.com/LeonYoah/stx/releases/download/v1.2.0/stx-linux-amd64"
if [[ "$url" != "$expected" ]]; then
  echo "FAIL direct url: $url"
  fail=1
else
  echo "OK direct url"
fi

url="$(stx_build_github_release_url "v1.2.0" "stx-linux-amd64" "https://v4.gh-proxy.org")"
expected="https://v4.gh-proxy.org/https://github.com/LeonYoah/stx/releases/download/v1.2.0/stx-linux-amd64"
if [[ "$url" != "$expected" ]]; then
  echo "FAIL mirrored url: $url"
  fail=1
else
  echo "OK mirrored url"
fi

check_eq() {
  local name="$1" got="$2" want="$3"
  if [[ "$got" == "$want" ]]; then
    echo "OK $name"
  else
    echo "FAIL $name: got [$got] want [$want]"
    fail=1
  fi
}

# tag=latest 必须走 releases/latest/download（releases/download/latest 会 404）。
# tag=latest must use releases/latest/download (releases/download/latest 404s).
check_eq "latest url" "$(stx_build_github_release_url latest SHA256SUMS "")" \
  "https://github.com/LeonYoah/stx/releases/latest/download/SHA256SUMS"
check_eq "latest mirrored url" "$(stx_build_github_release_url latest SHA256SUMS "https://gh-proxy.org/")" \
  "https://gh-proxy.org/https://github.com/LeonYoah/stx/releases/latest/download/SHA256SUMS"
check_eq "deps url" "$(stx_build_github_release_url deps MANIFEST.json "")" \
  "https://github.com/LeonYoah/stx/releases/download/deps/MANIFEST.json"

# 无 python3 的 JSON 解析。/ python3-free JSON parsing.
api_json='{
  "url": "https://api.github.com/repos/LeonYoah/stx/releases/1",
  "id": 1,
  "tag_name" : "v1.2.0",
  "name": "v1.2.0",
  "body": "mentions \"tag_name\": \"vX\" later"
}'
check_eq "json tag_name" "$(printf '%s' "$api_json" | stx_json_tag_name)" "v1.2.0"
check_eq "json tag_name compact" "$(printf '{"id":1,"tag_name":"v9.9.9-rc.1","x":2}' | stx_json_tag_name)" "v9.9.9-rc.1"
check_eq "json tag_name missing" "$(printf '{"message":"Not Found"}' | stx_json_tag_name || true)" ""
manifest='{
  "node_major": "18",
  "observability": {
    "amd64": "observability-prom3.9.1-am0.31.1-gf12.3.3-linux-amd64.tar.gz",
    "arm64": "observability-prom3.9.1-am0.31.1-gf12.3.3-linux-arm64.tar.gz"
  },
  "build_time": "2026-09-19T05:04:30Z"
}'
check_eq "manifest obs amd64" "$(printf '%s' "$manifest" | stx_manifest_obs_asset amd64)" \
  "observability-prom3.9.1-am0.31.1-gf12.3.3-linux-amd64.tar.gz"
check_eq "manifest obs arm64" "$(printf '%s' "$manifest" | stx_manifest_obs_asset arm64)" \
  "observability-prom3.9.1-am0.31.1-gf12.3.3-linux-arm64.tar.gz"
check_eq "manifest obs missing arch" "$(printf '%s' "$manifest" | stx_manifest_obs_asset riscv64 || true)" ""

for v in v18.18.0 v18.20.8 v20.0.0 v22.11.0 v100.1.1; do
  if stx_node_version_ok "$v"; then echo "OK node ok $v"; else echo "FAIL node should be ok: $v"; fail=1; fi
done
# 无小版本号按 minor=0：v18 => 18.0 < 18.18 拒绝，v20/v22 接受。/ No minor => minor=0.
for v in v20 v22; do
  if stx_node_version_ok "$v"; then echo "OK node ok (no minor) $v"; else echo "FAIL node should be ok: $v"; fail=1; fi
done
for v in v18.17.1 v16.20.2 v8.9.0 v18 v16 garbage ""; do
  if stx_node_version_ok "$v"; then echo "FAIL node should be rejected: $v"; fail=1; else echo "OK node rejected [$v]"; fi
done

# Node 变体自动选择：glibc < 2.27 => glibc217（仅 amd64）；arm64 总是 official。
# Node variant auto-pick: glibc < 2.27 => glibc217 (amd64 only); arm64 always official.
check_variant() {
  local glibc="$1" arch="$2" want="$3"
  check_eq "node variant glibc=${glibc:-none} arch=${arch:-unset}" \
    "$(stx_detect_glibc_version() { echo "$glibc"; }; stx_recommend_node_variant "$arch")" "$want"
}
check_variant 2.17 amd64 glibc217
check_variant 2.26 amd64 glibc217
check_variant 2.27 amd64 official
check_variant 2.35 amd64 official
check_variant 2.17 "" glibc217
check_variant 2.17 arm64 official
check_variant 2.26 arm64 official
check_variant "" amd64 official

ms="$(stx_now_ms)"
if [[ "$ms" =~ ^[0-9]{13,}$ ]]; then echo "OK now_ms"; else echo "FAIL now_ms: $ms"; fail=1; fi

if stx_require_cmds bash tar 2>/dev/null; then echo "OK require_cmds present"; else echo "FAIL require_cmds present"; fail=1; fi
if stx_require_cmds stx-definitely-missing-cmd 2>/dev/null; then echo "FAIL require_cmds missing"; fail=1; else echo "OK require_cmds missing"; fi

# stx_yaml_set（纯 awk）在真实 config.example.yaml 上的行为。/ stx_yaml_set (pure awk) on the real config.example.yaml.
yaml_get() {
  # 打印 key 所在行（去掉缩进），用于断言。/ Print the matched key line (indent stripped) for assertions.
  awk -v path="$2" '
    BEGIN { n = split(path, k, "."); d = 1; ind = 0 }
    /^[[:space:]]*(#|$)/ { next }
    { match($0, /^ */); cur = RLENGTH
      if (d > 1 && cur < ind) exit
      if (cur == ind && index(substr($0, cur + 1), k[d] ":") == 1) {
        if (d == n) { print substr($0, cur + 1); exit }
        d++; ind += 2 } }' "$1"
}
ycfg="$(mktemp)"
cp "$ROOT_DIR/config.example.yaml" "$ycfg"
before_lines="$(wc -l <"$ycfg")"
stx_yaml_set "$ycfg" "app.addr" ":18000"
stx_yaml_set "$ycfg" "app.external_url" "http://10.0.0.5:18000"
stx_yaml_set "$ycfg" "grpc.port" "18890"
stx_yaml_set "$ycfg" "observability.enabled" "true"
stx_yaml_set "$ycfg" "observability.prometheus.url" "http://127.0.0.1:9090"
stx_yaml_set "$ycfg" "observability.alertmanager.url" "http://127.0.0.1:9093"
stx_yaml_set "$ycfg" "observability.grafana.url" "http://127.0.0.1:3000"
stx_yaml_set "$ycfg" "auth.default_admin_password" 'p@ss w/ord&\1'
stx_yaml_set "$ycfg" "oauth_providers.github.enabled" "false"
stx_yaml_set "$ycfg" "app.app_name" "'single'"
check_eq "yaml app.addr" "$(yaml_get "$ycfg" app.addr)" "addr: :18000"
check_eq "yaml app.external_url" "$(yaml_get "$ycfg" app.external_url)" 'external_url: "http://10.0.0.5:18000"'
check_eq "yaml grpc.port" "$(yaml_get "$ycfg" grpc.port)" "port: 18890"
check_eq "yaml observability.enabled" "$(yaml_get "$ycfg" observability.enabled)" "enabled: true"
check_eq "yaml observability.prometheus.url" "$(yaml_get "$ycfg" observability.prometheus.url)" 'url: "http://127.0.0.1:9090"'
check_eq "yaml observability.alertmanager.url" "$(yaml_get "$ycfg" observability.alertmanager.url)" 'url: "http://127.0.0.1:9093"'
check_eq "yaml observability.grafana.url" "$(yaml_get "$ycfg" observability.grafana.url)" 'url: "http://127.0.0.1:3000"'
check_eq "yaml auth.default_admin_password" "$(yaml_get "$ycfg" auth.default_admin_password)" 'default_admin_password: "p@ss w/ord&\1"'
check_eq "yaml nested 3-level" "$(yaml_get "$ycfg" oauth_providers.github.enabled)" "enabled: false"
check_eq "yaml pre-quoted value" "$(yaml_get "$ycfg" app.app_name)" "app_name: 'single'"
# 同名子键不得被误改（grpc.port 改动不影响 database.port）。/ Same-named keys elsewhere stay untouched.
check_eq "yaml database.port untouched" "$(yaml_get "$ycfg" database.port)" "port: 3306"
check_eq "yaml line count kept" "$(wc -l <"$ycfg")" "$before_lines"
# 逐行比较（行数不变），不依赖 diff/cmp（精简镜像可能没有 diffutils）。/ Line-by-line compare without diff/cmp.
changed="$(awk 'NR == FNR { a[FNR] = $0; next } a[FNR] != $0 { n++ } END { print n + 0 }' "$ROOT_DIR/config.example.yaml" "$ycfg")"
# 10 次写入中 4 次与原值相同（observability.* 默认值），仅 6 行实际变化（行尾注释按原实现被替换掉）。
# 4 of the 10 writes equal the existing values (observability.* defaults), so exactly 6 lines change.
check_eq "yaml only targeted lines changed" "$changed" "6"
cp "$ycfg" "$ycfg.bak"
if stx_yaml_set "$ycfg" "observability.prometheus.nope" "x" 2>/dev/null; then
  echo "FAIL yaml missing key should fail"; fail=1
else
  echo "OK yaml missing key fails"
fi
if stx_yaml_set "$ycfg" "nope.addr" "x" 2>/dev/null; then
  echo "FAIL yaml missing parent should fail"; fail=1
else
  echo "OK yaml missing parent fails"
fi
if [[ "$(cksum <"$ycfg")" == "$(cksum <"$ycfg.bak")" ]]; then echo "OK yaml untouched on failure"; else echo "FAIL yaml modified on failure"; fail=1; fi
printf 'a:\n  b: 1\nz: 2' >"$ycfg"
stx_yaml_set "$ycfg" "z" "3"
check_eq "yaml keeps missing trailing newline" "$(od -An -c "$ycfg" | tr -d ' \n')" 'a:\nb:1\nz:3'
rm -f "$ycfg" "$ycfg.bak"

tmpdir="$(mktemp -d)"
echo hello >"$tmpdir/f"
if command -v sha256sum >/dev/null 2>&1; then
  want="$(sha256sum "$tmpdir/f" | awk '{print $1}')"
else
  want="$(shasum -a 256 "$tmpdir/f" | awk '{print $1}')"
fi
echo "$want" >"$tmpdir/f.sha256"
if stx_verify_sha256 "$tmpdir/f" "$tmpdir/f.sha256"; then
  echo "OK sha256"
else
  echo "FAIL sha256"
  fail=1
fi
echo wrong >"$tmpdir/f.sha256"
if stx_verify_sha256 "$tmpdir/f" "$tmpdir/f.sha256" 2>/dev/null; then
  echo "FAIL sha256 should mismatch"
  fail=1
else
  echo "OK sha256 mismatch detected"
fi
rm -rf "$tmpdir"

# Port helper smoke (may be flaky if ports busy; only checks function exists).
if stx_find_free_port 65530 >/dev/null; then
  echo "OK port probe"
else
  echo "FAIL port probe"
  fail=1
fi

# ---------- 镜像线路解析 / mirror line resolution ----------
expect_eq() { # <name> <want> <got>
  if [[ "$2" == "$3" ]]; then echo "OK $1"; else echo "FAIL $1: want [$2] got [$3]"; fail=1; fi
}
expect_eq "mirror lines count" 6 "${#STX_MIRROR_LINES[@]}"
expect_eq "resolve number 2 -> v4" "$(stx_mirror_field 1 2)" "https://v4.gh-proxy.org"
expect_eq "resolve '2'" 1 "$(stx_mirror_resolve 2)"
expect_eq "resolve cdn" 3 "$(stx_mirror_resolve cdn)"
expect_eq "resolve prefix url" 4 "$(stx_mirror_resolve https://axisnow.gh-proxy.org/)"
expect_eq "resolve github" 5 "$(stx_mirror_resolve https://github.com)"
expect_eq "resolve direct" 5 "$(stx_mirror_resolve direct)"
expect_eq "resolve auto" "" "$(stx_mirror_resolve auto)"
expect_eq "resolve custom url" "custom:https://my.proxy" "$(stx_mirror_resolve https://my.proxy/)"
if stx_mirror_resolve 9 >/dev/null 2>&1; then echo "FAIL resolve 9 must fail"; fail=1; else echo "OK resolve 9 rejected"; fi
if stx_mirror_resolve bogus >/dev/null 2>&1; then echo "FAIL resolve bogus must fail"; fail=1; else echo "OK resolve bogus rejected"; fi

# ---------- 测速选择（桩）/ probe + choose (stubbed) ----------
export STX_NONINTERACTIVE=1
unset STX_MIRROR STX_DOWNLOAD_MIRROR_PREFIX
stx_mirror_probe_one() { # cdn fastest, v4 second, gh-proxy third, others down
  case "$1" in
    https://cdn.gh-proxy.org/*) echo "1 80 9000" ;;
    https://v4.gh-proxy.org/*) echo "1 50 5000" ;;
    https://gh-proxy.org/*) echo "1 90 100" ;;
    *) echo "0 0 0" ;;
  esac
}
stx_choose_mirror "https://github.com/LeonYoah/stx/releases/latest/download/x" >"$tmpdir.probe" 2>&1 || true
expect_eq "fastest line chosen (cdn)" "https://cdn.gh-proxy.org" "$STX_SELECTED_MIRROR_PREFIX"
expect_eq "order: chosen, then OK lines by speed, then failed" \
  "https://cdn.gh-proxy.org https://v4.gh-proxy.org https://gh-proxy.org https://v6.gh-proxy.org https://axisnow.gh-proxy.org  " \
  "$(printf '%s ' "${STX_MIRROR_ORDER[@]}")"
if grep -q "不可用 / unreachable" "$tmpdir.probe" && grep -q "9000 KB/s" "$tmpdir.probe"; then
  echo "OK probe table shows speed and unreachable lines"
else
  echo "FAIL probe table"; cat "$tmpdir.probe"; fail=1
fi
STX_MIRROR=github stx_choose_mirror "https://github.com/x" >/dev/null 2>&1
expect_eq "STX_MIRROR=github forces direct" "" "$STX_SELECTED_MIRROR_PREFIX"
stx_choose_mirror "https://github.com/x" 2 >/dev/null 2>&1
expect_eq "--mirror 2 forces v4" "https://v4.gh-proxy.org" "$STX_SELECTED_MIRROR_PREFIX"
stx_mirror_probe_one() { echo "0 0 0"; }
stx_choose_mirror "https://github.com/x" >/dev/null 2>&1
expect_eq "all lines down -> recommended v4 first" "https://v4.gh-proxy.org" "$STX_SELECTED_MIRROR_PREFIX"
STX_MIRROR=https://my.proxy stx_choose_mirror "https://github.com/x" >/dev/null 2>&1
expect_eq "custom STX_MIRROR first" "https://my.proxy" "${STX_MIRROR_ORDER[0]}"
expect_eq "custom STX_MIRROR then all lines" 7 "${#STX_MIRROR_ORDER[@]}"
rm -f "$tmpdir.probe"

# ---------- 下载失败自动换线路（桩 curl）/ download fallback (stubbed curl) ----------
mkdir -p "$tmpdir"
CURL_CALLS="$tmpdir/calls"
: >"$CURL_CALLS"
curl() { # only the third line works
  local out="" url="" a
  while [[ $# -gt 0 ]]; do
    a="$1"; shift
    case "$a" in -o) out="$1"; shift ;; http*) url="$a" ;; esac
  done
  echo "$url" >>"$CURL_CALLS"
  if [[ "$url" == https://gh-proxy.org/* ]]; then echo payload >"$out"; return 0; fi
  return 22
}
STX_MIRROR_ORDER=("https://cdn.gh-proxy.org" "https://v4.gh-proxy.org" "https://gh-proxy.org" "")
if stx_download_with_fallback "https://github.com/a/b" "$tmpdir/dl" >/dev/null 2>&1 && [[ "$(cat "$tmpdir/dl")" == payload ]]; then
  echo "OK download falls back to the next line"
else
  echo "FAIL download fallback"; fail=1
fi
expect_eq "fallback tried 3 lines" 3 "$(wc -l <"$CURL_CALLS" | tr -d ' ')"
expect_eq "successful line remembered" "https://gh-proxy.org" "$STX_SELECTED_MIRROR_PREFIX"
STX_MIRROR_ORDER=("https://cdn.gh-proxy.org" "")
if stx_download_with_fallback "https://github.com/a/b" "$tmpdir/dl2" q >/dev/null 2>&1; then
  echo "FAIL download must fail when all lines fail"; fail=1
else
  echo "OK all lines failing returns error"
fi
[[ ! -e "$tmpdir/dl2" ]] && echo "OK no partial file left" || { echo "FAIL partial file left"; fail=1; }
unset -f curl
rm -rf "$tmpdir"

# ---------- 离线包命名与变体 / bundle naming and variant ----------
expect_eq "glibc 2.17 < 2.28" yes "$(stx_glibc_lt_228 2.17 && echo yes || echo no)"
expect_eq "glibc 2.28 not < 2.28" no "$(stx_glibc_lt_228 2.28 && echo yes || echo no)"
expect_eq "glibc 2.35 not < 2.28" no "$(stx_glibc_lt_228 2.35 && echo yes || echo no)"
expect_eq "arm64 variant" arm64 "$(stx_detect_bundle_variant arm64)"
expect_eq "asset name centos7" "stx-v1.3.0-linux-amd64-centos7-offline.tar.gz" "$(stx_bundle_asset_name v1.3.0 centos7)"
expect_eq "alias name arm64" "stx-offline-linux-arm64.tar.gz" "$(stx_bundle_alias_name arm64)"
expect_eq "latest url uses alias" "https://github.com/LeonYoah/stx/releases/latest/download/stx-offline-linux-amd64.tar.gz" \
  "$(stx_bundle_release_url latest amd64)"
expect_eq "tag url uses versioned name" "https://github.com/LeonYoah/stx/releases/download/v1.3.0/stx-v1.3.0-linux-amd64-offline.tar.gz" \
  "$(stx_bundle_release_url v1.3.0 amd64)"

# ---------- 非交互判定 / non-interactive detection ----------
if (unset STX_ASSUME_YES STX_NONINTERACTIVE; CI=true stx_is_interactive); then
  echo "FAIL CI=true must be non-interactive"; fail=1
else
  echo "OK CI=true is non-interactive"
fi
: >"$(stx_tty_eof_flag)"
if (unset STX_ASSUME_YES STX_NONINTERACTIVE CI; stx_is_interactive); then
  echo "FAIL tty EOF marker must make it non-interactive"; fail=1
else
  echo "OK tty EOF marker => non-interactive"
fi
rm -f "$(stx_tty_eof_flag)"
expect_eq "stx_prompt non-interactive returns default" "dflt" "$(STX_NONINTERACTIVE=1 stx_prompt "x" dflt)"
forced="$(STX_NONINTERACTIVE=1 STX_PORT_PROBE_LIMIT=50 timeout 20 bash -c 'source "$0/install/download-lib.sh"; source "$0/install/install-core.sh"; stx_ask_port_forced p 40000 ""' "$ROOT_DIR" 2>/dev/null)"
if [[ "$forced" =~ ^[0-9]+$ && "$forced" -gt 40000 ]]; then echo "OK forced port without tty takes the suggestion (no loop)"; else echo "FAIL forced port without tty: [$forced]"; fail=1; fi
nosug="$(STX_NONINTERACTIVE=1 STX_PORT_PROBE_LIMIT=0 timeout 20 bash -c 'source "$0/install/download-lib.sh"; source "$0/install/install-core.sh"; stx_ask_port_forced p 40000 "" >/dev/null 2>&1; echo $?' "$ROOT_DIR")"
expect_eq "forced port without tty and no suggestion fails (no loop)" 1 "$nosug"
expect_eq "confirm port without tty keeps current" "17880" "$(STX_NONINTERACTIVE=1 stx_ask_port_confirm "p" 17880 "" x "" 2>/dev/null)"

# ---------- 管理员数据库状态 / admin database state ----------
kd="$(mktemp -d)"
printf 'database:\n  type: "sqlite"\n  sqlite_path: ./data/stx.db\n' >"$kd/config.yaml"
expect_eq "sqlite without db file => new" new "$(stx_admin_db_kind "$kd" "$kd/config.yaml")"
mkdir -p "$kd/data" && echo x >"$kd/data/stx.db"
expect_eq "sqlite with db file => existing" existing "$(stx_admin_db_kind "$kd" "$kd/config.yaml")"
printf 'database:\n  type: "mysql"\n  host: "db"\n' >"$kd/mysql.yaml"
expect_eq "preserved mysql config => external (not 'initialized')" external "$(stx_admin_db_kind "$kd" "$kd/mysql.yaml")"
expect_eq "chosen mysql overrides sqlite config" external "$(STX_DB_TYPE_OVERRIDE=mysql stx_admin_db_kind "$kd" "$kd/config.yaml")"
if stx_existing_admin_db "$kd" "$kd/mysql.yaml"; then
  echo "FAIL external DB must not be treated as initialized"; fail=1
else
  echo "OK external DB not treated as initialized"
fi
rm -rf "$kd"

exit "$fail"
