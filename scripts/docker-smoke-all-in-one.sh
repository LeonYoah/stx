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

# all-in-one 镜像冒烟：启动容器，校验 /api/v1/health 返回 200，前端首页跟随跳转（/ -> /login）后返回 200。
# all-in-one smoke: start the container, expect /api/v1/health == 200 and the frontend home page
# (/ -> /login, redirects followed) == 200.
#
# 用法 / Usage: scripts/docker-smoke-all-in-one.sh <image> [--platform linux/arm64] [--timeout 180]
# 环境变量 / Env: DOCKER (default: docker), SMOKE_BACKEND_PORT (17800), SMOKE_FRONTEND_PORT (17880)

set -euo pipefail

DOCKER="${DOCKER:-docker}"
BACKEND_PORT="${SMOKE_BACKEND_PORT:-17800}"
FRONTEND_PORT="${SMOKE_FRONTEND_PORT:-17880}"
TIMEOUT=180
PLATFORM=""
IMAGE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --platform) PLATFORM="${2:-}"; shift 2 ;;
    --timeout) TIMEOUT="${2:-}"; shift 2 ;;
    -h|--help)
      sed -n '17,22p' "$0"
      exit 0
      ;;
    -*) echo "unknown option: $1" >&2; exit 2 ;;
    *) IMAGE="$1"; shift ;;
  esac
done

if [[ -z "$IMAGE" ]]; then
  echo "usage: $0 <image> [--platform linux/arm64] [--timeout 180]" >&2
  exit 2
fi

container="stx-smoke-$$"
platform_args=()
if [[ -n "$PLATFORM" ]]; then
  platform_args=(--platform "$PLATFORM")
fi

# shellcheck disable=SC2086
cleanup() {
  local rc=$?
  if [[ $rc -ne 0 ]]; then
    echo "::group::container logs ($container)"
    $DOCKER logs "$container" 2>&1 | tail -n 200 || true
    echo "--- /opt/stx/logs/backend.log"
    $DOCKER exec "$container" tail -n 200 /opt/stx/logs/backend.log 2>/dev/null || true
    echo "--- /opt/stx/logs/frontend.log"
    $DOCKER exec "$container" tail -n 200 /opt/stx/logs/frontend.log 2>/dev/null || true
    echo "::endgroup::"
  fi
  $DOCKER rm -f "$container" >/dev/null 2>&1 || true
  exit $rc
}
trap cleanup EXIT

echo "[smoke] starting $IMAGE ${PLATFORM:+($PLATFORM)} as $container"
# shellcheck disable=SC2086
$DOCKER run -d --name "$container" "${platform_args[@]}" \
  -p "127.0.0.1:${BACKEND_PORT}:17800" \
  -p "127.0.0.1:${FRONTEND_PORT}:17880" \
  "$IMAGE" >/dev/null

# 轮询直到返回 200 或超时。/ Poll until HTTP 200 or timeout.
wait_for_200() {
  local name="$1" url="$2"
  shift 2
  local deadline=$((SECONDS + TIMEOUT))
  local code=""
  while (( SECONDS < deadline )); do
    if ! $DOCKER inspect -f '{{.State.Running}}' "$container" 2>/dev/null | grep -q true; then
      echo "[smoke] container exited before $name became ready" >&2
      return 1
    fi
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$@" "$url" 2>/dev/null || true)"
    if [[ "$code" == "200" ]]; then
      echo "[smoke] $name OK ($url -> 200 after $((SECONDS))s)"
      return 0
    fi
    sleep 2
  done
  echo "[smoke] $name FAILED: $url last status=${code:-none} after ${TIMEOUT}s" >&2
  return 1
}

SECONDS=0
wait_for_200 "backend health" "http://127.0.0.1:${BACKEND_PORT}/api/v1/health"
wait_for_200 "frontend home" "http://127.0.0.1:${FRONTEND_PORT}/" -L

final_url="$(curl -sS -o /dev/null -L -w '%{url_effective}' --max-time 10 "http://127.0.0.1:${FRONTEND_PORT}/")"
first_hop="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' --max-time 10 "http://127.0.0.1:${FRONTEND_PORT}/")"
echo "[smoke] frontend / first hop: ${first_hop}; final: ${final_url}"
# sharp 必须在当前架构可加载，且 next/image 返回经过优化（明显小于原图）的图片。
# sharp must load on this arch and next/image must return an optimized (clearly smaller) image.
sharp_info="$($DOCKER exec -w /opt/stx/frontend "$container" node -e "
const sharp = require(require.resolve('sharp', {paths: [require.resolve('next/package.json')]}));
console.log(process.arch + ' sharp=' + sharp.versions.sharp + ' libvips=' + sharp.versions.vips);
")" || { echo "[smoke] sharp failed to load inside the image" >&2; exit 1; }
echo "[smoke] $sharp_info"

logo="/brand/stx-logo.png"
original_bytes="$(curl -sS -o /dev/null -w '%{size_download}' --max-time 10 "http://127.0.0.1:${FRONTEND_PORT}${logo}")"
read -r image_code image_bytes image_type < <(curl -sS -o /dev/null -H 'Accept: image/webp,image/png' \
  -w '%{http_code} %{size_download} %{content_type}\n' --max-time 60 \
  "http://127.0.0.1:${FRONTEND_PORT}/_next/image?url=%2Fbrand%2Fstx-logo.png&w=256&q=75")
echo "[smoke] next/image ${logo} w=256: status=${image_code} bytes=${image_bytes} type=${image_type} (original ${original_bytes} bytes)"
if [[ "$image_code" != "200" ]] || (( image_bytes <= 0 || image_bytes * 2 > original_bytes )); then
  echo "[smoke] next/image did not return an optimized image (sharp not working?)" >&2
  exit 1
fi

echo "[smoke] all-in-one smoke passed"
