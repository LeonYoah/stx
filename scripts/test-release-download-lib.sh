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

exit "$fail"
