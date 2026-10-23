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

# 安全发布 deps Release（release-deps.yml 使用）：绝不删除整个 Release / tag，只替换本次重建的资产。
#   1. 校验每个 tarball 都有匹配的 .sha256；
#   2. 下载 deps 上已有的 MANIFEST.json，与本次片段合并（未重建的条目原样保留）；
#   3. Release 已存在：先把新文件以 <name>.uploading 临时名全部上传（--clobber 只覆盖临时名的残留），
#      全部上传成功后再逐个“删旧资产 + 临时名改回正式名”，MANIFEST.json 最后切换；
#      上传中途失败时旧资产完好无损，deps 永远不会为空。
#   4. Release 不存在：gh release create（--latest=false）。
# Safe deps publisher (used by release-deps.yml). Never deletes the whole release/tag; replaces only rebuilt assets:
#   1. every tarball must have a matching .sha256;
#   2. download the existing MANIFEST.json and merge this run's fragments into it (untouched entries preserved);
#   3. release exists: upload all new files under a temporary <name>.uploading name first (--clobber only replaces
#      leftovers of that temp name), then swap each one (delete old asset + rename temp to the real name),
#      MANIFEST.json last. A failed upload leaves the old assets intact; deps is never empty;
#   4. release missing: gh release create (--latest=false).
#
# 用法 / Usage:
#   scripts/release-deps-publish.sh <tag> <assets-dir> <manifest-fragment>...
# 环境 / Env: GITHUB_REPOSITORY (owner/repo, required), TARGET_SHA (for create), GH (default: gh),
#             DEPS_RELEASE_NOTES (optional)

set -euo pipefail

tag="${1:?usage: $0 <tag> <assets-dir> <manifest-fragment>...}"
assets_dir="${2:?usage: $0 <tag> <assets-dir> <manifest-fragment>...}"
shift 2
[[ $# -gt 0 ]] || { echo "[ERROR] at least one MANIFEST fragment is required" >&2; exit 2; }
fragments=("$@")
repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY (owner/repo) is required}"
GH="${GH:-gh}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
notes="${DEPS_RELEASE_NOTES:-可复用依赖，不随版本 tag 重打。Reusable deps. Not rebuilt on version tags.}"
tmp_suffix=".uploading"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# 1. 收集并校验本次产物。/ Collect and verify this run's artifacts.
shopt -s nullglob
tarballs=("$assets_dir"/node-*.tar.gz "$assets_dir"/observability-*.tar.gz)
shopt -u nullglob
[[ ${#tarballs[@]} -gt 0 ]] || { echo "[ERROR] no node-*/observability-* tarballs in $assets_dir" >&2; exit 1; }
files=()
for t in "${tarballs[@]}"; do
  [[ -f "${t}.sha256" ]] || { echo "[ERROR] missing sidecar ${t}.sha256" >&2; exit 1; }
  want="$(awk 'NF{print $1; exit}' "${t}.sha256")"
  got="$(sha256_of "$t")"
  [[ "$want" == "$got" ]] || { echo "[ERROR] sha256 mismatch: $(basename "$t") sidecar=$want actual=$got" >&2; exit 1; }
  files+=("$t" "${t}.sha256")
done

release_exists=false
if "$GH" release view "$tag" -R "$repo" >/dev/null 2>&1; then
  release_exists=true
fi

# 2. 合并 MANIFEST。/ Merge MANIFEST.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
existing=""
if [[ "$release_exists" == "true" ]]; then
  mkdir -p "$work/old"
  if "$GH" release download "$tag" -R "$repo" -p MANIFEST.json -D "$work/old" --clobber >/dev/null 2>&1 \
    && [[ -s "$work/old/MANIFEST.json" ]]; then
    existing="$work/old/MANIFEST.json"
  else
    echo "[WARN] $tag has no readable MANIFEST.json; writing a fresh one" >&2
  fi
fi
python3 "$script_dir/merge-deps-manifest.py" --assets-dir "$assets_dir" --out "$assets_dir/MANIFEST.json" \
  ${existing:+--existing "$existing"} "${fragments[@]}"
echo "== merged MANIFEST.json"
cat "$assets_dir/MANIFEST.json"

# 4. Release 不存在：直接创建。/ Release missing: create it.
if [[ "$release_exists" != "true" ]]; then
  echo "[INFO] release $tag not found; creating it"
  "$GH" release create "$tag" -R "$repo" --target "${TARGET_SHA:?TARGET_SHA is required to create the release}" \
    --latest=false --title "$tag" --notes "$notes" "${files[@]}" "$assets_dir/MANIFEST.json"
  exit 0
fi

# 3. Release 已存在：临时名上传 → 逐个切换，MANIFEST 最后。/ Existing release: temp upload, then swap; MANIFEST last.
release_id="$("$GH" api "repos/$repo/releases/tags/$tag" --jq .id)"
[[ -n "$release_id" ]] || { echo "[ERROR] cannot resolve release id of $tag" >&2; exit 1; }

list_assets() {
  # 每行：id<TAB>name<TAB>size。/ One line per asset: id<TAB>name<TAB>size.
  "$GH" api --paginate "repos/$repo/releases/$release_id/assets?per_page=100" \
    --jq '.[] | "\(.id)\t\(.name)\t\(.size)"'
}

mkdir -p "$work/upload"
upload_list=()
for f in "${files[@]}" "$assets_dir/MANIFEST.json"; do
  staged="$work/upload/$(basename "$f")${tmp_suffix}"
  ln "$f" "$staged" 2>/dev/null || cp "$f" "$staged"
  upload_list+=("$staged")
done
echo "[INFO] uploading ${#upload_list[@]} files under temporary names (*${tmp_suffix})"
"$GH" release upload "$tag" -R "$repo" --clobber "${upload_list[@]}"

swap_asset() {
  local name="$1" listing old_id new_id
  listing="$(list_assets)"
  new_id="$(awk -F'\t' -v n="${name}${tmp_suffix}" '$2 == n {print $1; exit}' <<<"$listing")"
  old_id="$(awk -F'\t' -v n="$name" '$2 == n {print $1; exit}' <<<"$listing")"
  [[ -n "$new_id" ]] || { echo "[ERROR] uploaded temp asset ${name}${tmp_suffix} not found" >&2; return 1; }
  if [[ -n "$old_id" ]]; then
    "$GH" api -X DELETE "repos/$repo/releases/assets/$old_id" >/dev/null
  fi
  "$GH" api -X PATCH "repos/$repo/releases/assets/$new_id" -f name="$name" >/dev/null
  echo "[OK] replaced $name${old_id:+ (old asset $old_id)}"
}

for f in "${files[@]}"; do
  swap_asset "$(basename "$f")"
done
swap_asset "MANIFEST.json"

# 最终核对：正式名都在、大小一致、无临时名残留。/ Final check: names present, sizes match, no temp leftovers.
listing="$(list_assets)"
fail=0
for f in "${files[@]}" "$assets_dir/MANIFEST.json"; do
  name="$(basename "$f")"
  size="$(awk -F'\t' -v n="$name" '$2 == n {print $3; exit}' <<<"$listing")"
  local_size="$(wc -c <"$f" | tr -d ' ')"
  if [[ "$size" != "$local_size" ]]; then
    echo "[FAIL] $name on release has size '${size:-missing}', expected $local_size"
    fail=1
  fi
done
if cut -f2 <<<"$listing" | grep -q '\.uploading$'; then
  echo "[FAIL] temporary *${tmp_suffix} assets left on $tag"
  fail=1
fi
[[ "$fail" -eq 0 ]] || exit 1
echo "[OK] $tag updated in place: ${#files[@]} assets + MANIFEST.json replaced; other assets untouched"
