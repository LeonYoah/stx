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

# 用真实的 docker/metadata-action 代码（dist/index.js）模拟 tag 推送，校验 release-on-tag.yml 生成的镜像 tag：
#   - 预发 tag（含 '-'，如 v1.2.0-beta.1）不得生成 latest，也不得生成 {{major}}.{{minor}}；
#   - 正式 tag（如 v1.2.0）必须生成 latest。
# 不登录、不推送任何镜像。
# Run the real docker/metadata-action code (dist/index.js) against simulated tag refs and assert the image tags
# that release-on-tag.yml would publish: pre-release tags (containing '-') never get `latest` (nor major.minor),
# stable tags always do. Nothing is logged in to or pushed.
#
# 用法 / Usage: scripts/check-release-image-tags.sh <metadata-action-checkout> [workflow-file]
#   metadata-action-checkout: docker/metadata-action 的检出目录（与 release-on-tag.yml 使用同一主版本，需含 dist/）
#   workflow-file 默认 / default: .github/workflows/release-on-tag.yml

set -euo pipefail

action_dir="${1:?usage: $0 <metadata-action-checkout> [workflow-file]}"
workflow="${2:-.github/workflows/release-on-tag.yml}"
[[ -f "$action_dir/dist/index.js" ]] || { echo "[ERROR] $action_dir/dist/index.js not found" >&2; exit 2; }
[[ -f "$workflow" ]] || { echo "[ERROR] $workflow not found" >&2; exit 2; }

tmp="$(mktemp -d)"
mock_pid=""
cleanup() { [[ -n "$mock_pid" ]] && kill "$mock_pid" 2>/dev/null; rm -rf "$tmp"; }
trap cleanup EXIT

# metadata-action 启动时会调用 GET /repos/{owner}/{repo}（仅用于 OCI labels）。用本地桩服务应答，
# 使检查不依赖网络、token 或 API 限流；tag 计算与仓库元数据无关。
# metadata-action calls GET /repos/{owner}/{repo} (used only for OCI labels). Answer it from a local stub so the
# check needs no network, token or API quota; tag computation does not depend on repo metadata.
node -e '
  const srv = require("http").createServer((req, res) => {
    res.setHeader("content-type", "application/json");
    res.end(JSON.stringify({ name: "stx", full_name: "LeonYoah/stx", description: "", license: null,
      html_url: "https://github.com/LeonYoah/stx", default_branch: "main" }));
  });
  srv.listen(0, "127.0.0.1", () => require("fs").writeFileSync(process.argv[1], String(srv.address().port)));
' "$tmp/port" &
mock_pid=$!
for _ in $(seq 1 50); do [[ -s "$tmp/port" ]] && break; sleep 0.1; done
[[ -s "$tmp/port" ]] || { echo "[ERROR] stub GitHub API did not start" >&2; exit 2; }
api_url="http://127.0.0.1:$(cat "$tmp/port")"

# 从工作流中取出 metadata-action 步骤的 flavor / tags 块（YAML 块标量或单行值）。
# Extract the metadata-action step's flavor / tags blocks (block scalar or inline value) from the workflow.
extract_input() {
  local key="$1"
  awk -v key="$key" '
    /uses:[[:space:]]*docker\/metadata-action@/ { in_step = 1; next }
    in_step && /^[[:space:]]*-[[:space:]]+(name|uses|id):/ { in_step = 0 }
    in_step {
      if (collecting) {
        if ($0 ~ /^[[:space:]]*$/) next
        match($0, /^[[:space:]]*/); ind = RLENGTH
        if (ind <= key_ind) { collecting = 0 }
        else { line = substr($0, ind + 1); if (line !~ /^#/) print line; next }
      }
      if ($0 ~ "^[[:space:]]*" key ":") {
        match($0, /^[[:space:]]*/); key_ind = RLENGTH
        rest = $0; sub("^[[:space:]]*" key ":[[:space:]]*", "", rest)
        if (rest ~ /^[|>]/) collecting = 1; else if (rest != "") print rest
      }
    }
  ' "$workflow"
}

tags_tpl="$(extract_input tags)"
flavor_tpl="$(extract_input flavor)"
[[ -n "$tags_tpl" ]] || { echo "[ERROR] metadata-action tags not found in $workflow" >&2; exit 2; }

# 只支持工作流里实际用到的表达式 contains(github.ref_name, '<s>')，遇到其它 ${{ }} 直接失败，避免静默误判。
# Only the expression form actually used (contains(github.ref_name, '<s>')) is evaluated; anything else fails loudly.
render() {
  REF_NAME="$1" node -e '
    const ref = process.env.REF_NAME;
    let s = require("fs").readFileSync(0, "utf8");
    s = s.replace(/\$\{\{\s*(!?)\s*contains\(\s*github\.ref_name\s*,\s*\x27([^\x27]*)\x27\s*\)\s*\}\}/g,
      (_, neg, needle) => String(neg ? !ref.includes(needle) : ref.includes(needle)));
    s = s.replace(/\$\{\{\s*contains\(\s*github\.ref_name\s*,\s*\x27([^\x27]*)\x27\s*\)\s*&&\s*\x27([^\x27]*)\x27\s*\|\|\s*\x27([^\x27]*)\x27\s*\}\}/g,
      (_, needle, a, b) => (ref.includes(needle) ? a : b));
    if (/\$\{\{/.test(s)) { console.error("[ERROR] unsupported expression in metadata-action inputs:\n" + s); process.exit(2); }
    process.stdout.write(s);
  '
}

# 运行 metadata-action，输出生成的 tag 列表（每行一个）。
# Run metadata-action for one simulated tag ref and print the generated tags, one per line.
run_meta() {
  local ref_name="$1" out="$tmp/out-$1" event="$tmp/event-$1.json" sha
  sha="0123456789abcdef0123456789abcdef01234567"
  printf '{"ref":"refs/tags/%s","head_commit":{"id":"%s","timestamp":"2026-01-01T00:00:00Z"}}\n' "$ref_name" "$sha" > "$event"
  : > "$out"
  env -i PATH="$PATH" HOME="$tmp" RUNNER_TEMP="$tmp" \
    GITHUB_ACTIONS=true GITHUB_EVENT_NAME=push GITHUB_EVENT_PATH="$event" \
    GITHUB_REF="refs/tags/$ref_name" GITHUB_REF_NAME="$ref_name" GITHUB_REF_TYPE=tag GITHUB_SHA="$sha" \
    GITHUB_REPOSITORY=LeonYoah/stx GITHUB_REPOSITORY_OWNER=LeonYoah GITHUB_SERVER_URL=https://github.com \
    GITHUB_API_URL="$api_url" GITHUB_RUN_ID=1 GITHUB_RUN_NUMBER=1 GITHUB_WORKFLOW=check GITHUB_OUTPUT="$out" \
    INPUT_CONTEXT=workflow \
    INPUT_IMAGES=$'ghcr.io/leonyoah/stx-backend\nswr.cn-east-3.myhuaweicloud.com/stx/stx-backend' \
    INPUT_TAGS="$(render "$ref_name" <<<"$tags_tpl")" \
    INPUT_FLAVOR="$(render "$ref_name" <<<"$flavor_tpl")" \
    INPUT_SEP-TAGS=$'\n' INPUT_SEP-LABELS=$'\n' INPUT_SEP-ANNOTATIONS=$'\n' \
    INPUT_BAKE-TARGET=docker-metadata-action INPUT_GITHUB-TOKEN=stub-token \
    node "$action_dir/dist/index.js" > "$tmp/log-$ref_name" 2>&1 || { cat "$tmp/log-$ref_name" >&2; return 1; }
  # GITHUB_OUTPUT 中 tags 为 heredoc 格式：tags<<DELIM ... DELIM
  awk '/^tags<</ { d = substr($0, 7); f = 1; next } f && $0 == d { f = 0 } f' "$out"
}

fail=0
check() {
  local ref_name="$1" want_latest="$2" tags
  tags="$(run_meta "$ref_name")"
  echo "== refs/tags/$ref_name =="
  while IFS= read -r t; do echo "  $t"; done <<<"$tags"
  for reg in ghcr.io/leonyoah swr.cn-east-3.myhuaweicloud.com/stx; do
    local img="$reg/stx-backend"
    grep -qxF "$img:$ref_name" <<<"$tags" || { echo "[FAIL] $img:$ref_name missing"; fail=1; }
    if [[ "$want_latest" == yes ]]; then
      grep -qxF "$img:latest" <<<"$tags" || { echo "[FAIL] stable tag $ref_name must publish $img:latest"; fail=1; }
    else
      if grep -qxF "$img:latest" <<<"$tags"; then echo "[FAIL] pre-release tag $ref_name must NOT publish $img:latest"; fail=1; fi
      if grep -qE "^$img:[0-9]+\.[0-9]+$" <<<"$tags"; then echo "[FAIL] pre-release tag $ref_name must NOT publish $img:<major>.<minor>"; fail=1; fi
    fi
  done
}

check v1.2.0-beta.1 no
check v1.2.0-rc.1 no
# 非 semver 的预发 tag：semver 规则不生效时，旧的 latest=auto 会经 type=ref,event=tag 补出 latest。
# Non-semver pre-release tag: without semver rules matching, the old latest=auto added latest via type=ref,event=tag.
check v1.2-beta.1 no
check v1.2.0 yes

if [[ "$fail" -ne 0 ]]; then
  echo "[ERROR] release image tag policy violated (see above)" >&2
  exit 1
fi
echo "[OK] release image tags: pre-release tags never publish latest; stable tags do (GHCR + SWR)."
