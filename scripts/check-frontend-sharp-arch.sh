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

# 校验前端 standalone 包只携带目标架构 glibc 版 sharp（@img/sharp-linux-<cpu> + @img/sharp-libvips-linux-<cpu>），
# 不含其它架构、musl 或非 linux 的 sharp 原生包。
# Assert a frontend standalone tarball ships only the target arch glibc sharp
# (@img/sharp-linux-<cpu> + @img/sharp-libvips-linux-<cpu>) and no other-arch / musl / non-linux sharp packages.
#
# 用法 / Usage: scripts/check-frontend-sharp-arch.sh <amd64|arm64> [tarball]
#   tarball 默认 / default: dist/releases/frontend-standalone-linux-<arch>.tar.gz

set -euo pipefail

ARCH="${1:-}"
case "$ARCH" in
  amd64) CPU="x64" ;;
  arm64) CPU="arm64" ;;
  *)
    echo "usage: $0 <amd64|arm64> [tarball]" >&2
    exit 2
    ;;
esac
TARBALL="${2:-dist/releases/frontend-standalone-linux-${ARCH}.tar.gz}"

if [[ ! -s "$TARBALL" ]]; then
  echo "[sharp-check] missing tarball: $TARBALL" >&2
  exit 1
fi

want_sharp="sharp-linux-${CPU}"
want_vips="sharp-libvips-linux-${CPU}"

listing="$(tar -tzf "$TARBALL")"

# 从 .pnpm/@img+<name>@<ver> 与 .../@img/<name> 两种路径形态提取 sharp 包名。
# Extract sharp package names from both .pnpm/@img+<name>@<ver> and .../@img/<name> path forms.
found="$(
  {
    grep -oE '/@img\+sharp-[^/@]+@' <<<"$listing" | sed -E 's#^/@img\+##; s#@$##' || true
    grep -oE '/@img/sharp-[^/]+' <<<"$listing" | sed -E 's#^/@img/##' || true
  } | sort -u
)"

failed=0
unexpected="$(grep -vxF -e "$want_sharp" -e "$want_vips" <<<"$found" || true)"
if [[ -n "$unexpected" ]]; then
  echo "[sharp-check] FAIL $(basename "$TARBALL"): unexpected sharp packages for linux/${ARCH}:" >&2
  sed 's/^/  - @img\//' <<<"$unexpected" >&2
  failed=1
fi

if ! grep -qE "/@img/${want_sharp}/lib/${want_sharp}\.node$" <<<"$listing"; then
  echo "[sharp-check] FAIL $(basename "$TARBALL"): missing @img/${want_sharp}/lib/${want_sharp}.node" >&2
  failed=1
fi
if ! grep -qE "/@img/${want_vips}/lib/libvips-cpp\.so" <<<"$listing"; then
  echo "[sharp-check] FAIL $(basename "$TARBALL"): missing @img/${want_vips}/lib/libvips-cpp.so*" >&2
  failed=1
fi

if [[ $failed -ne 0 ]]; then
  exit 1
fi
echo "[sharp-check] OK   $(basename "$TARBALL"): only @img/${want_sharp} + @img/${want_vips}"
