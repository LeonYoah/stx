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

# 守护 release-on-tag.yml 的上传清单：发版目录里的每个文件（以及 SHA256SUMS 列出的每个文件）都必须恰好被
# softprops/action-gh-release 的 files 通配命中一次——漏命中会导致资产缺失（如 v1.2.0 的 stx-java-proxy.sh），
# 重复命中会让同一资产上传两次而 404。
# Guard the upload list in release-on-tag.yml: every file in the release dir (and every file listed in SHA256SUMS)
# must be matched by exactly one softprops/action-gh-release `files` glob. Unmatched files silently go missing from
# the release (e.g. stx-java-proxy.sh on v1.2.0); files matched twice are uploaded twice and 404.
#
# 用法 / Usage: scripts/check-release-asset-globs.sh <release-assets-dir> [workflow-file]
#   release-assets-dir: package-release.sh --layout split 的输出目录（dist/releases 或 CI 中的 release-assets）
#   workflow-file 默认 / default: .github/workflows/release-on-tag.yml

set -euo pipefail

assets_dir="${1:?usage: $0 <release-assets-dir> [workflow-file]}"
workflow="${2:-.github/workflows/release-on-tag.yml}"
[[ -d "$assets_dir" ]] || { echo "[ERROR] $assets_dir is not a directory" >&2; exit 2; }
[[ -f "$workflow" ]] || { echo "[ERROR] $workflow not found" >&2; exit 2; }

# 取出 action-gh-release 步骤 files: | 块中的通配（去掉目录前缀，只按文件名匹配）。
# Extract the globs of the action-gh-release step's `files: |` block (directory prefix stripped; match by basename).
mapfile -t globs < <(awk '
  /uses:[[:space:]]*softprops\/action-gh-release@/ { in_step = 1; next }
  in_step && /^[[:space:]]*-[[:space:]]+(name|uses|id):/ { in_step = 0 }
  in_step {
    if (collecting) {
      if ($0 ~ /^[[:space:]]*$/) next
      match($0, /^[[:space:]]*/); ind = RLENGTH
      if (ind <= key_ind) { collecting = 0 }
      else { line = substr($0, ind + 1); if (line !~ /^#/) print line; next }
    }
    if ($0 ~ /^[[:space:]]*files:[[:space:]]*[|>]/) { match($0, /^[[:space:]]*/); key_ind = RLENGTH; collecting = 1 }
  }
' "$workflow" | sed 's|.*/||')
[[ ${#globs[@]} -gt 0 ]] || { echo "[ERROR] no action-gh-release files globs found in $workflow" >&2; exit 2; }

fail=0
count_matches() {
  local name="$1" g n=0
  for g in "${globs[@]}"; do
    # shellcheck disable=SC2053
    [[ "$name" == $g ]] && n=$((n + 1))
  done
  echo "$n"
}

shopt -s nullglob
names=()
for f in "$assets_dir"/*; do
  [[ -f "$f" ]] && names+=("$(basename "$f")")
done
shopt -u nullglob
[[ ${#names[@]} -gt 0 ]] || { echo "[ERROR] $assets_dir is empty" >&2; exit 2; }

for name in "${names[@]}"; do
  n="$(count_matches "$name")"
  if [[ "$n" -eq 0 ]]; then
    echo "[FAIL] $name is not matched by any upload glob (it would be missing from the release)"
    fail=1
  elif [[ "$n" -gt 1 ]]; then
    echo "[FAIL] $name is matched by $n upload globs (duplicate upload 404s)"
    fail=1
  fi
done

# SHA256SUMS 里列出的文件必须存在且会被上传，否则用户无法按清单校验。
# Files listed in SHA256SUMS must exist and be uploaded, or users cannot verify against the list.
if [[ -f "$assets_dir/SHA256SUMS" ]]; then
  while read -r _ listed; do
    [[ -z "${listed:-}" ]] && continue
    listed="${listed#\*}"
    if [[ ! -f "$assets_dir/$listed" ]]; then
      echo "[FAIL] SHA256SUMS lists $listed but it is not in $assets_dir"
      fail=1
    elif [[ "$(count_matches "$listed")" -eq 0 ]]; then
      echo "[FAIL] SHA256SUMS lists $listed but no upload glob matches it"
      fail=1
    fi
    if [[ -f "$assets_dir/${listed}.sha256" && "$(count_matches "${listed}.sha256")" -eq 0 ]]; then
      echo "[FAIL] ${listed}.sha256 exists but no upload glob matches it"
      fail=1
    fi
  done <"$assets_dir/SHA256SUMS"
else
  echo "[FAIL] $assets_dir/SHA256SUMS missing"
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  echo "[ERROR] release upload globs in $workflow do not cover the packaged assets exactly once (see above)" >&2
  exit 1
fi
echo "[OK] ${#names[@]} release assets each matched by exactly one upload glob; SHA256SUMS entries all uploaded."
