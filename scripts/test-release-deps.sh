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

# release-deps 安全发布的离线测试（无网络、无 token）：
#   - release-deps.yml 不得出现 `gh release delete` / --cleanup-tag，且提供 components 输入；
#   - merge-deps-manifest.py：只重打三件套时保留 Node 条目，反之亦然；
#   - release-deps-publish.sh（用桩 gh 模拟 Release 状态）：先以临时名上传、再切换，MANIFEST 最后，
#     未重建的资产（Node 包）id 不变，Release 不存在时才 create。
# Offline tests for the safe deps publisher (no network, no token): workflow never deletes the release,
# manifest merge preserves untouched entries, publisher uploads under temp names first, swaps, MANIFEST last,
# leaves non-rebuilt assets untouched and only creates the release when it does not exist.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WF="$ROOT_DIR/.github/workflows/release-deps.yml"
fail=0
ok() { echo "OK $*"; }
bad() { echo "FAIL $*"; fail=1; }
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# ---- workflow 静态检查 / workflow static checks ----
if grep -nE 'gh[[:space:]]+release[[:space:]]+delete|--cleanup-tag' "$WF" "$ROOT_DIR/scripts/release-deps-publish.sh"; then
  bad "release-deps must never delete the deps release/tag"
else
  ok "no gh release delete / --cleanup-tag"
fi
grep -q 'components:' "$WF" && grep -q -- '- observability' "$WF" && ok "components input with observability option" \
  || bad "components input missing"
grep -q 'release-deps-publish.sh' "$WF" && ok "workflow publishes via release-deps-publish.sh" || bad "publisher not used"

# ---- manifest merge ----
OBS_A="observability-prom3.9.1-am0.31.1-gf12.3.3-linux-amd64.tar.gz"
OBS_R="observability-prom3.9.1-am0.31.1-gf12.3.3-linux-arm64.tar.gz"
old_manifest() {
  cat <<JSON
{
  "node_major": "18",
  "node_variant": "glibc217",
  "node_version": "18.20.8",
  "prometheus": "3.9.0",
  "alertmanager": "0.31.0",
  "grafana": "12.3.2",
  "observability": {
    "amd64": "observability-old-linux-amd64.tar.gz",
    "arm64": "observability-old-linux-arm64.tar.gz"
  },
  "custom": "kept",
  "build_time": "2026-09-19T05:04:30Z"
}
JSON
}
frag() { # $1 node_major $2 variant $3 version
  cat <<JSON
{
  "node_major": "$1", "node_variant": "$2", "node_version": "$3",
  "prometheus": "3.9.1", "alertmanager": "0.31.1", "grafana": "12.3.3",
  "observability": {"amd64": "$OBS_A", "arm64": "$OBS_R"},
  "build_time": "x"
}
JSON
}
jget() { python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); 
for k in sys.argv[2].split("."):
    d = d.get(k) if isinstance(d, dict) else None
print("" if d is None else (json.dumps(d) if isinstance(d,(dict,list)) else d))' "$1" "$2"; }

# 只重打三件套：Node 旧字段原样保留。/ observability only: legacy node keys untouched.
m="$tmp/m1"; mkdir -p "$m/assets"
old_manifest >"$m/old.json"; frag 18 official "" >"$m/f1.json"
touch "$m/assets/$OBS_A" "$m/assets/$OBS_R"
python3 "$ROOT_DIR/scripts/merge-deps-manifest.py" --assets-dir "$m/assets" --out "$m/out.json" \
  --existing "$m/old.json" --now 2026-10-23T00:00:00Z "$m/f1.json" 2>/dev/null
[[ "$(jget "$m/out.json" node_version)" == "18.20.8" && "$(jget "$m/out.json" node_variant)" == "glibc217" ]] \
  && ok "obs-only keeps node fields" || bad "obs-only changed node fields"
[[ "$(jget "$m/out.json" observability.arm64)" == "$OBS_R" && "$(jget "$m/out.json" prometheus)" == "3.9.1" ]] \
  && ok "obs-only updates observability" || bad "obs-only did not update observability"
[[ "$(jget "$m/out.json" custom)" == "kept" && "$(jget "$m/out.json" build_time)" == "2026-10-23T00:00:00Z" ]] \
  && ok "unknown keys kept, build_time refreshed" || bad "unknown keys / build_time"
[[ -z "$(jget "$m/out.json" node)" ]] && ok "obs-only adds no node map" || bad "obs-only touched node map"

# 只重打 Node：三件套条目保留；最后一次 Node 构建生效于旧字段。/ node only: observability kept; last node wins.
m="$tmp/m2"; mkdir -p "$m/assets"
old_manifest >"$m/old.json"; frag 22 official 22.23.4 >"$m/f1.json"; frag 18 glibc217 18.20.9 >"$m/f2.json"
touch "$m/assets/node-22-official-linux-amd64.tar.gz" "$m/assets/node-22-official-linux-arm64.tar.gz" \
  "$m/assets/node-18-glibc217-linux-amd64.tar.gz"
python3 "$ROOT_DIR/scripts/merge-deps-manifest.py" --assets-dir "$m/assets" --out "$m/out.json" \
  --existing "$m/old.json" "$m/f1.json" "$m/f2.json" 2>/dev/null
[[ "$(jget "$m/out.json" observability.amd64)" == "observability-old-linux-amd64.tar.gz" && "$(jget "$m/out.json" grafana)" == "12.3.2" ]] \
  && ok "node-only keeps observability" || bad "node-only changed observability"
[[ "$(jget "$m/out.json" node_version)" == "18.20.9" ]] && ok "legacy node keys follow last build" || bad "legacy node keys"
python3 - "$m/out.json" <<'PY' && ok "node map records each built package" || bad "node map"
import json, sys
n = json.load(open(sys.argv[1]))["node"]
assert n["node-22-official-linux-arm64.tar.gz"]["version"] == "22.23.4", n
assert n["node-18-glibc217-linux-amd64.tar.gz"]["version"] == "18.20.9", n
assert "node-18-glibc217-linux-arm64.tar.gz" not in n, n
PY
# 片段里的文件一个都不存在：报错且不写输出。/ Nothing built: error, no output.
m="$tmp/m3"; mkdir -p "$m/assets"; frag 22 official 22.1.0 >"$m/f1.json"
if python3 "$ROOT_DIR/scripts/merge-deps-manifest.py" --assets-dir "$m/assets" --out "$m/out.json" "$m/f1.json" 2>/dev/null; then
  bad "merge with nothing built should fail"
else
  [[ ! -e "$m/out.json" ]] && ok "merge with nothing built fails" || bad "merge wrote output despite failure"
fi

# ---- publisher with a stub gh ----
# 桩 gh：用 state 文件（id<TAB>name<TAB>size）模拟 Release 资产，并把每次调用记入 log。
# Stub gh: simulate release assets in a state file (id<TAB>name<TAB>size) and log every call.
stub="$tmp/gh"
cat >"$stub" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
state="$STUB_DIR/state"; log="$STUB_DIR/log"
echo "$*" >>"$log"
next_id() { local n; n=$(cat "$STUB_DIR/seq" 2>/dev/null || echo 100); echo $((n + 1)) >"$STUB_DIR/seq"; echo $((n + 1)); }
add_file() { local f="$1" name; name="$(basename "$f")"
  awk -F'\t' -v n="$name" '$2 != n' "$state" >"$state.t"; mv "$state.t" "$state"
  printf '%s\t%s\t%s\n' "$(next_id)" "$name" "$(wc -c <"$f" | tr -d ' ')" >>"$state"; }
case "$1 $2" in
  "release view") [[ -f "$state" ]] ;;
  "release download")
    dir=""; while [[ $# -gt 0 ]]; do [[ "$1" == "-D" ]] && dir="$2"; shift; done
    [[ -f "$STUB_DIR/MANIFEST.json" ]] && cp "$STUB_DIR/MANIFEST.json" "$dir/MANIFEST.json" ;;
  "release delete") echo "stub: release delete is forbidden" >&2; exit 9 ;;
  "release create") : >"$state"; for a in "$@"; do [[ -f "$a" ]] && add_file "$a"; done ;;
  "release upload")
    [[ -n "${STUB_FAIL_UPLOAD:-}" ]] && { echo "stub: upload failed" >&2; exit 1; }
    for a in "$@"; do [[ -f "$a" ]] && add_file "$a"; done ;;
  *)
    if [[ "$1" == "api" && "$2" == repos/*/releases/tags/* ]]; then echo 1
    elif [[ "$1" == "api" && "$2" == "--paginate" ]]; then cat "$state"
    elif [[ "$1 $2" == "api -X" && "$3" == "DELETE" ]]; then id="${4##*/}"
      awk -F'\t' -v i="$id" '$1 != i' "$state" >"$state.t"; mv "$state.t" "$state"
    elif [[ "$1 $2" == "api -X" && "$3" == "PATCH" ]]; then id="${4##*/}"; name="${6#name=}"
      awk -F'\t' -v OFS='\t' -v i="$id" -v n="$name" '$1 == i {$2 = n} {print}' "$state" >"$state.t"; mv "$state.t" "$state"
    else echo "stub: unexpected: $*" >&2; exit 3; fi ;;
esac
STUB
chmod +x "$stub"

mk_assets() { # $1 dir: rebuilt observability tarballs + sidecars + fragment
  mkdir -p "$1/assets"
  for n in "$OBS_A" "$OBS_R"; do
    head -c 2048 /dev/urandom >"$1/assets/$n"
    sha256sum "$1/assets/$n" | awk '{print $1}' >"$1/assets/$n.sha256"
  done
  frag 18 official "" >"$1/frag.json"
}

# 场景 A：deps 已存在（含 Node 包与旧三件套），只重打三件套。/ Scenario A: existing release, obs-only rebuild.
d="$tmp/pa"; mkdir -p "$d"; mk_assets "$d"
old_manifest >"$d/MANIFEST.json"
printf '1\tnode-22-official-linux-amd64.tar.gz\t111\n2\tnode-22-official-linux-amd64.tar.gz.sha256\t65\n3\tnode-18-glibc217-linux-amd64.tar.gz\t333\n4\t%s\t9\n5\t%s.sha256\t65\n6\tMANIFEST.json\t10\n' "$OBS_A" "$OBS_A" >"$d/state"
if STUB_DIR="$d" GH="$stub" GITHUB_REPOSITORY=o/r bash "$ROOT_DIR/scripts/release-deps-publish.sh" deps "$d/assets" "$d/frag.json" >"$d/out" 2>&1; then
  ok "publish (existing release) succeeded"
else
  bad "publish (existing release) failed"; sed 's/^/    /' "$d/out"
fi
grep -q '^release delete' "$d/log" && bad "publisher called release delete" || ok "publisher never deletes the release"
! grep -q '^release create' "$d/log" && ok "no create when release exists" || bad "create called although release exists"
first_upload="$(grep -n '^release upload' "$d/log" | head -1 | cut -d: -f1)"
first_delete="$(grep -n 'api -X DELETE' "$d/log" | head -1 | cut -d: -f1)"
[[ -n "$first_upload" && -n "$first_delete" && "$first_upload" -lt "$first_delete" ]] \
  && ok "all uploads happen before any asset delete" || bad "upload/delete order"
grep '^release upload' "$d/log" | grep -q -- '--clobber' && ok "temp upload uses --clobber" || bad "upload without --clobber"
last_patch="$(grep 'api -X PATCH' "$d/log" | tail -1)"
[[ "$last_patch" == *"name=MANIFEST.json" ]] && ok "MANIFEST.json switched last" || bad "MANIFEST not last: $last_patch"
for keep in "1	node-22-official-linux-amd64.tar.gz	111" "3	node-18-glibc217-linux-amd64.tar.gz	333"; do
  grep -qxF "$keep" "$d/state" && ok "untouched asset kept: ${keep#*	}" || bad "node asset changed: $keep"
done
for n in "$OBS_A" "$OBS_R" "$OBS_A.sha256" "$OBS_R.sha256" MANIFEST.json; do
  [[ "$(awk -F'\t' -v n="$n" '$2 == n' "$d/state" | wc -l)" -eq 1 ]] && ok "exactly one $n" || bad "asset count for $n"
done
[[ "$(awk -F'\t' -v n="$OBS_A" '$2 == n {print $3}' "$d/state")" == "2048" ]] && ok "obs amd64 replaced with new file" || bad "obs amd64 not replaced"
grep -q 'uploading' "$d/state" && bad "temp names left" || ok "no temp names left"
grep -q '"node_version": "18.20.8"' "$d/assets/MANIFEST.json" && grep -q "\"arm64\": \"$OBS_R\"" "$d/assets/MANIFEST.json" \
  && ok "published MANIFEST merged (node kept, obs updated)" || bad "published MANIFEST not merged"

# 场景 B：上传失败——旧资产一个不少。/ Scenario B: upload fails, old assets intact.
d="$tmp/pb"; mkdir -p "$d"; mk_assets "$d"; old_manifest >"$d/MANIFEST.json"
printf '1\tnode-22-official-linux-amd64.tar.gz\t111\n4\t%s\t9\n6\tMANIFEST.json\t10\n' "$OBS_A" >"$d/state"
cp "$d/state" "$d/state.before"
if STUB_FAIL_UPLOAD=1 STUB_DIR="$d" GH="$stub" GITHUB_REPOSITORY=o/r bash "$ROOT_DIR/scripts/release-deps-publish.sh" deps "$d/assets" "$d/frag.json" >"$d/out" 2>&1; then
  bad "publish should fail when upload fails"
else
  [[ "$(cat "$d/state")" == "$(cat "$d/state.before")" ]] && ok "failed upload leaves deps untouched" || bad "deps changed after failed upload"
fi

# 场景 C：sidecar 不匹配——拒绝发布。/ Scenario C: sidecar mismatch is rejected before any call that mutates.
d="$tmp/pc"; mkdir -p "$d"; mk_assets "$d"; echo deadbeef >"$d/assets/$OBS_A.sha256"
if STUB_DIR="$d" GH="$stub" GITHUB_REPOSITORY=o/r bash "$ROOT_DIR/scripts/release-deps-publish.sh" deps "$d/assets" "$d/frag.json" >"$d/out" 2>&1; then
  bad "sha256 mismatch should be rejected"
else
  [[ ! -s "$d/log" ]] && ok "sha256 mismatch rejected before touching the release" || bad "release touched despite mismatch"
fi

# 场景 D：deps 不存在——create。/ Scenario D: release missing -> create.
d="$tmp/pd"; mkdir -p "$d"; mk_assets "$d"
if STUB_DIR="$d" GH="$stub" GITHUB_REPOSITORY=o/r TARGET_SHA=abc bash "$ROOT_DIR/scripts/release-deps-publish.sh" deps "$d/assets" "$d/frag.json" >"$d/out" 2>&1; then
  grep -q '^release create deps -R o/r --target abc --latest=false' "$d/log" && ok "creates release when missing (latest=false)" || bad "create args"
  [[ "$(wc -l <"$d/state")" -eq 5 ]] && ok "create uploads 4 files + MANIFEST" || bad "create asset count"
else
  bad "publish (missing release) failed"; sed 's/^/    /' "$d/out"
fi

exit "$fail"
