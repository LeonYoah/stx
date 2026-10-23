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

# stx_yaml_set 差分测试：纯 awk 新实现 vs v1.2.0 的 python3 参考实现（原样内嵌，函数名改为 stx_yaml_set_ref）。
# 在 config.example.yaml 的全部 key 路径 + 不存在的路径 × 多种取值上，比较输出文件、stderr 与退出码，要求逐字节一致。
# 需要 python3（仅测试用，安装链路本身不需要）。
# Differential test: the pure-awk stx_yaml_set vs the v1.2.0 python3 reference (embedded verbatim, renamed to
# stx_yaml_set_ref). Over every key path in config.example.yaml plus missing paths x many values, output file,
# stderr and exit code must be byte-identical. Needs python3 (test only; the installer itself does not).

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=../install/install-core.sh
source "$ROOT_DIR/install/install-core.sh"

# ---- v1.2.0 reference implementation (verbatim) ----
stx_yaml_set_ref() {
  local file="$1"
  local key_path="$2" # e.g. app.addr or grpc.port
  local value="$3"
  python3 - "$file" "$key_path" "$value" <<'PY'
import pathlib
import re
import sys

path = pathlib.Path(sys.argv[1])
key_path = sys.argv[2].split(".")
value = sys.argv[3]
text = path.read_text(encoding="utf-8")
lines = text.splitlines(keepends=True)

def matches_key(line, key, indent):
    return re.match(rf"^{re.escape(' ' * indent)}{re.escape(key)}\s*:", line) is not None

idx = 0
indent = 0
for depth, key in enumerate(key_path):
    found = False
    while idx < len(lines):
        line = lines[idx]
        if line.strip() and not line.strip().startswith("#"):
            cur_indent = len(line) - len(line.lstrip(" "))
            if cur_indent < indent and depth > 0:
                break
            if matches_key(line, key, indent):
                found = True
                if depth == len(key_path) - 1:
                    nl = "\n" if line.endswith("\n") else ""
                    quote = '"' if not (value.startswith('"') or value.startswith("'") or value.startswith(":") or value.isdigit() or value in ("true", "false")) else ""
                    if value.startswith(":") or value.isdigit() or value in ("true", "false"):
                        lines[idx] = f"{' ' * indent}{key}: {value}{nl}"
                    else:
                        lines[idx] = f"{' ' * indent}{key}: {quote}{value}{quote}{nl}"
                    path.write_text("".join(lines), encoding="utf-8")
                    sys.exit(0)
                indent += 2
                idx += 1
                break
        idx += 1
    if not found:
        sys.exit(f"yaml key not found: {'.'.join(key_path)}")
sys.exit(f"yaml key not found: {'.'.join(key_path)}")
PY
}
# ---- end reference ----

command -v python3 >/dev/null 2>&1 || { echo "[SKIP] python3 not available for the reference implementation"; exit 0; }

cfg="$ROOT_DIR/config.example.yaml"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mapfile -t keys < <(awk '
  /^[ ]*#/ || /^[ \t]*$/ { next }
  { match($0, /^ */); ind = RLENGTH; line = substr($0, ind + 1)
    if (line !~ /^[A-Za-z0-9_]+[ \t]*:/) next
    k = line; sub(/[ \t]*:.*/, "", k); lvl = ind / 2; path[lvl] = k
    p = path[0]; for (i = 1; i <= lvl; i++) p = p "." path[i]; print p }' "$cfg" | sort -u)
keys+=(nope.x app.nope observability.prometheus.nope auth "app..addr")
vals=(':18000' '8080' 'true' 'false' 'http://1.2.3.4:17800' '"quoted"' "'sq'" 'a b c' 'a\b\\n&x/y|z' '' '0' '-1' 'x"y' '$HOME')

# 额外的边界文件：末行无换行、tab 缩进、空文件。/ Extra edge files: no trailing newline, tab indent, empty.
printf 'a:\n  b: 1\n  c:\n    d: x # c\nz: 2' >"$tmp/noeol.yaml"
printf 'a:\n\tb: 1\n  b: 3\nempty:\n' >"$tmp/tab.yaml"
: >"$tmp/empty.yaml"

n=0; diffs=0
run_case() {
  local src="$1" k="$2" v="$3" ra rb
  n=$((n + 1))
  cp "$src" "$tmp/a"; cp "$src" "$tmp/b"
  ( stx_yaml_set_ref "$tmp/a" "$k" "$v" ) 2>"$tmp/ea"; ra=$?
  ( stx_yaml_set "$tmp/b" "$k" "$v" ) 2>"$tmp/eb"; rb=$?
  if [[ "$ra" -ne "$rb" ]] || ! cmp -s "$tmp/a" "$tmp/b" || ! cmp -s "$tmp/ea" "$tmp/eb"; then
    diffs=$((diffs + 1))
    echo "DIFF file=$(basename "$src") key=$k val=$v rc=$ra/$rb"
    diff "$tmp/a" "$tmp/b" | head -5
  fi
}
for k in "${keys[@]}"; do
  for v in "${vals[@]}"; do run_case "$cfg" "$k" "$v"; done
done
for f in noeol tab empty; do
  for k in a.b a.c.d z empty a.b.x; do
    for v in 9 'x y' true; do run_case "$tmp/$f.yaml" "$k" "$v"; done
  done
done
echo "stx_yaml_set differential: cases=$n keys=${#keys[@]} diffs=$diffs"
[[ "$diffs" -eq 0 ]]
