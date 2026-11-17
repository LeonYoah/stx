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
# See the License for the LICENSE file for more details.

# 把约 25GiB 的 stx_loadtest_gauge 历史块写到 staging，再导入 data/tsdb。
# 导入会短暂停 API；SeaTunnel CDC 作业在引擎侧，不会跟着停。
# 中断后可重跑 write：已有 ULID 块按窗口跳过，从缺口续写。
# Write ~25GiB of stx_loadtest_gauge history into staging, then import into data/tsdb.
# Import bounces the API briefly; the SeaTunnel CDC job stays on the engine.
# A retry of write skips existing ULID windows and continues from the gaps.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

STX_TSDB_BACKFILL="${STX_TSDB_BACKFILL:-25g}"
STX_TSDB_BACKFILL_DIR="${STX_TSDB_BACKFILL_DIR:-$ROOT_DIR/data/tsdb-staging}"
LIVE_DIR="${STX_TSDB_LIVE_DIR:-$ROOT_DIR/data/tsdb}"
TIMEOUT="${STX_TSDB_BACKFILL_TIMEOUT:-12h}"
PHASE="${1:-write}"

mkdir -p "$STX_TSDB_BACKFILL_DIR" "$LIVE_DIR"

write_blocks() {
  echo "tsdb backfill write size=$STX_TSDB_BACKFILL dir=$STX_TSDB_BACKFILL_DIR timeout=$TIMEOUT" >&2
  STX_TSDB_BACKFILL="$STX_TSDB_BACKFILL" \
    STX_TSDB_BACKFILL_DIR="$STX_TSDB_BACKFILL_DIR" \
    go test ./internal/pkg/embedts -count=1 -timeout "$TIMEOUT" -run '^TestCapacity_Backfill$'
}

import_blocks() {
  echo "tsdb backfill import $STX_TSDB_BACKFILL_DIR -> $LIVE_DIR" >&2
  local n=0
  for d in "$STX_TSDB_BACKFILL_DIR"/*; do
    [[ -d "$d" ]] || continue
    local name
    name="$(basename "$d")"
    [[ ${#name} -eq 26 ]] || continue
    if [[ -e "$LIVE_DIR/$name" ]]; then
      echo "skip existing $name" >&2
      continue
    fi
    mv "$d" "$LIVE_DIR/$name"
    n=$((n + 1))
  done
  echo "imported $n blocks; live=$(du -sh "$LIVE_DIR" | awk '{print $1}')" >&2
}

case "$PHASE" in
  write) write_blocks ;;
  import) import_blocks ;;
  all)
    write_blocks
    import_blocks
    ;;
  *)
    echo "usage: $0 [write|import|all]" >&2
    exit 2
    ;;
esac
