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

# 校验镜像内的 stx / agent / java-proxy / 前端文件与 dist/releases/ 中的发版产物逐字节一致（sha256）。
# Verify that stx / agent / java-proxy / frontend files inside an image are byte-identical (sha256)
# to the release artifacts in dist/releases/.
#
# 用法 / Usage:
#   scripts/docker-verify-release-artifacts.sh <image> <backend|frontend|all-in-one> <amd64|arm64> [releases-dir]
# 环境变量 / Env: DOCKER (default: docker)

set -euo pipefail

DOCKER="${DOCKER:-docker}"
IMAGE="${1:-}"
KIND="${2:-}"
ARCH="${3:-}"
RELEASES_DIR="${4:-dist/releases}"

if [[ -z "$IMAGE" || -z "$KIND" || -z "$ARCH" ]]; then
  echo "usage: $0 <image> <backend|frontend|all-in-one> <amd64|arm64> [releases-dir]" >&2
  exit 2
fi

sha_of() { sha256sum "$1" | awk '{print $1}'; }

# 期望值：发版产物路径 => 镜像内路径。/ Expected: release artifact => path inside the image.
pairs=()
frontend_dir=""
case "$KIND" in
  backend|all-in-one)
    pairs+=(
      "stx-linux-${ARCH}=/opt/stx/stx"
      "stx-agent-linux-amd64=/opt/stx/lib/agent/stx-agent-linux-amd64"
      "stx-agent-linux-arm64=/opt/stx/lib/agent/stx-agent-linux-arm64"
      "stx-agent-linux-amd64=/opt/stx/default-lib/agent/stx-agent-linux-amd64"
      "stx-agent-linux-arm64=/opt/stx/default-lib/agent/stx-agent-linux-arm64"
      "stx-java-proxy-v2.jar=/opt/stx/lib/stx-java-proxy-v2.jar"
      "stx-java-proxy-v3.jar=/opt/stx/lib/stx-java-proxy-v3.jar"
    )
    [[ "$KIND" == "all-in-one" ]] && frontend_dir="/opt/stx/frontend"
    ;;
  frontend)
    frontend_dir="/opt/stx/frontend"
    ;;
  *)
    echo "unknown image kind: $KIND" >&2
    exit 2
    ;;
esac

paths=()
for pair in "${pairs[@]}"; do
  paths+=("${pair#*=}")
done

# 一次 docker run 取回镜像内全部 sha256。/ Fetch every in-image sha256 with a single docker run.
# shellcheck disable=SC2016 # expanded inside the container
script='set -e
for p in "$@"; do sha256sum "$p"; done
if [ -n "$FRONTEND_DIR" ]; then
  cd "$FRONTEND_DIR" && find . -type f | LC_ALL=C sort | xargs -d "\n" sha256sum | sha256sum | sed "s/ .*/ FRONTEND_TREE/"
fi'
in_image="$($DOCKER run --rm --platform "linux/${ARCH}" --entrypoint sh -e FRONTEND_DIR="$frontend_dir" "$IMAGE" -c "$script" sh "${paths[@]}")"

failed=0
for pair in "${pairs[@]}"; do
  artifact="${pair%%=*}"
  target="${pair#*=}"
  want="$(sha_of "$RELEASES_DIR/$artifact")"
  got="$(awk -v p="$target" '$2 == p {print $1}' <<<"$in_image")"
  if [[ "$want" == "$got" ]]; then
    echo "[verify] OK   $target == dist/releases/$artifact ($want)"
  else
    echo "[verify] FAIL $target (${got:-missing}) != dist/releases/$artifact ($want)" >&2
    failed=1
  fi
done

if [[ -n "$frontend_dir" ]]; then
  tarball="$RELEASES_DIR/frontend-standalone-linux-${ARCH}.tar.gz"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  tar -xzf "$tarball" -C "$tmp"
  want="$(cd "$tmp" && find . -type f | LC_ALL=C sort | xargs -d '\n' sha256sum | sha256sum | awk '{print $1}')"
  got="$(awk '$2 == "FRONTEND_TREE" {print $1}' <<<"$in_image")"
  if [[ "$want" == "$got" ]]; then
    echo "[verify] OK   $frontend_dir tree == $(basename "$tarball") ($want)"
  else
    echo "[verify] FAIL $frontend_dir tree (${got:-missing}) != $(basename "$tarball") ($want)" >&2
    failed=1
  fi
fi

if [[ $failed -ne 0 ]]; then
  echo "[verify] image $IMAGE does not match dist/releases" >&2
  exit 1
fi
echo "[verify] image $IMAGE matches dist/releases"
