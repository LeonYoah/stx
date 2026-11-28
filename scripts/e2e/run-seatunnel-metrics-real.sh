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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/seatunnel-metrics-real" && pwd)"
OUT_DIR="${STX_METRICS_REAL_OUT_DIR:-${ROOT_DIR}/out}"
REST_PORT="${STX_METRICS_REAL_REST_PORT:-28180}"
BASE_URL="http://127.0.0.1:${REST_PORT}"
JOB_FILE="${ROOT_DIR}/jobs/kafka-jsonpath-sql-iceberg.conf"
COMPOSE=(docker compose -f "${ROOT_DIR}/docker-compose.yml")
export STX_METRICS_REAL_REST_PORT="${REST_PORT}"

mkdir -p "${OUT_DIR}"
cleanup() {
  "${COMPOSE[@]}" logs --no-color seatunnel kafka >"${OUT_DIR}/containers.log" 2>&1 || true
  "${COMPOSE[@]}" down -v --remove-orphans >"${OUT_DIR}/compose-down.log" 2>&1 || true
}
trap cleanup EXIT

"${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
"${COMPOSE[@]}" up -d

python3 - "$BASE_URL" <<'PY'
import json
import sys
import time
import urllib.request

base = sys.argv[1]
deadline = time.time() + 120
while time.time() < deadline:
    try:
        with urllib.request.urlopen(base + "/overview", timeout=3) as response:
            if response.status == 200:
                sys.exit(0)
    except Exception:
        pass
    time.sleep(2)
raise SystemExit("SeaTunnel REST did not become ready within 120 seconds")
PY

docker exec stx-metrics-real-kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 \
  --create --if-not-exists --topic orders --partitions 1 --replication-factor 1 >/dev/null

curl -fsS --form "config_file=@${JOB_FILE}" "${BASE_URL}/submit-job/upload?jobName=stx-metrics-real" >"${OUT_DIR}/submit.json"
JOB_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["jobId"])' "${OUT_DIR}/submit.json")"
echo "submitted job ${JOB_ID}"

python3 - <<'PY' >"${OUT_DIR}/messages.txt"
for index in range(1, 301):
    print('{"id":%d,"customer_id":%d,"total_amount":%s,"event_date":"2026-11-27"}' % (index, 1000 + index % 7, format(10 + index / 10, ".2f")))
PY
docker exec -i stx-metrics-real-kafka /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server localhost:9092 --topic orders <"${OUT_DIR}/messages.txt" >/dev/null

python3 - "$BASE_URL" "$JOB_ID" "${OUT_DIR}/job-info.json" <<'PY'
import json
import sys
import time
import urllib.request

base, job_id, output = sys.argv[1:4]
url = f"{base}/job-info/{job_id}"
deadline = time.time() + 150
last = None
while time.time() < deadline:
    try:
        with urllib.request.urlopen(url, timeout=5) as response:
            last = json.load(response)
    except Exception as exc:
        time.sleep(2)
        continue
    metrics = last.get("metrics") or {}
    status = last.get("jobStatus")
    received = int(float(metrics.get("SourceReceivedCount") or 0))
    written = int(float(metrics.get("SinkWriteCount") or 0))
    if received >= 300 and written >= 300:
        with open(output, "w", encoding="utf-8") as fh:
            json.dump(last, fh, ensure_ascii=False, indent=2)
        print(f"captured metrics: received={received}, written={written}")
        sys.exit(0)
    if status in {"FAILED", "CANCELED"}:
        with open(output, "w", encoding="utf-8") as fh:
            json.dump(last, fh, ensure_ascii=False, indent=2)
        raise SystemExit(f"job entered terminal state {status}: {last.get('diagnostics')}")
    time.sleep(2)
if last is not None:
    with open(output, "w", encoding="utf-8") as fh:
        json.dump(last, fh, ensure_ascii=False, indent=2)
raise SystemExit("timed out waiting for SeaTunnel aggregate metrics")
PY

echo "metrics fixture written to ${OUT_DIR}/job-info.json"
