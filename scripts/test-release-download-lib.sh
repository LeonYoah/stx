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
