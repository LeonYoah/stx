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

# 内嵌 tsdb 实时长时间运行基准（#150 第 2 步）：按墙钟 scrape_interval 抓取，
# 默认 48 小时 / 2 万序列 / 15s。不要用突发写入档代替这一档。
# Real-time embedded tsdb soak (#150 step 2): wall-clock scrape_interval,
# default 48h / 20k series / 15s. The burst capacity profile does not replace this.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

STX_TSDB_SOAK="${STX_TSDB_SOAK:-48h}"
STX_TSDB_SOAK_SERIES="${STX_TSDB_SOAK_SERIES:-20000}"
STX_TSDB_SOAK_TARGETS="${STX_TSDB_SOAK_TARGETS:-4}"
STX_TSDB_SOAK_INTERVAL="${STX_TSDB_SOAK_INTERVAL:-15s}"
STX_TSDB_SOAK_SAMPLE="${STX_TSDB_SOAK_SAMPLE:-1m}"
STX_TSDB_SOAK_OUT="${STX_TSDB_SOAK_OUT:-$ROOT_DIR/tmp/tsdb-soak-${STX_TSDB_SOAK}.jsonl}"

case "$STX_TSDB_SOAK" in
  48h|2d) TIMEOUT="${STX_TSDB_SOAK_TIMEOUT:-50h}" ;;
  smoke) TIMEOUT="${STX_TSDB_SOAK_TIMEOUT:-1m}" ;;
  *) TIMEOUT="${STX_TSDB_SOAK_TIMEOUT:-1h}" ;;
esac

mkdir -p "$(dirname "$STX_TSDB_SOAK_OUT")"

export STX_TSDB_SOAK STX_TSDB_SOAK_SERIES STX_TSDB_SOAK_TARGETS
export STX_TSDB_SOAK_INTERVAL STX_TSDB_SOAK_SAMPLE STX_TSDB_SOAK_OUT
if [[ -n "${STX_TSDB_SOAK_BLOCK:-}" ]]; then
  export STX_TSDB_SOAK_BLOCK
fi

echo "tsdb soak duration=$STX_TSDB_SOAK series=$STX_TSDB_SOAK_SERIES targets=$STX_TSDB_SOAK_TARGETS interval=$STX_TSDB_SOAK_INTERVAL sample=$STX_TSDB_SOAK_SAMPLE timeout=$TIMEOUT out=$STX_TSDB_SOAK_OUT" >&2

exec go test ./internal/pkg/embedts -count=1 -timeout "$TIMEOUT" -run '^TestCapacity_Soak$'
