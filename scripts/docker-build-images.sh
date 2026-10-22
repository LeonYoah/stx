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

# 本地构建 STX 镜像：先用 install/package-release.sh 打包发版产物，再让 Dockerfile 只消费 dist/releases/。
# Build STX images locally: package release artifacts with install/package-release.sh first,
# then build images that only consume dist/releases/ (same flow as CI and the release pipeline).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

DOCKER="${DOCKER:-docker}"
PLATFORMS="linux/$(case "$(uname -m)" in aarch64|arm64) echo arm64 ;; *) echo amd64 ;; esac)"
IMAGES="backend,frontend,all-in-one"
TAG="local"
SKIP_PACKAGE=false
SMOKE=false
VERIFY=false
APP_VERSION="${APP_VERSION:-}"

usage() {
  cat <<'USAGE'
Usage: scripts/docker-build-images.sh [options]

Options:
  --platforms <list>    Comma-separated platforms (default: host, e.g. linux/amd64).
                        Non-host platforms need QEMU binfmt (docker run --privileged --rm tonistiigi/binfmt --install arm64).
  --images <list>       Comma-separated subset of backend,frontend,all-in-one (default: all three)
  --tag <tag>           Image tag prefix; images are tagged stx-<image>:<tag>-<arch> (default: local)
  --version <string>    Version label passed to package-release.sh and image labels
                        (default: git describe --tags --always --dirty)
  --skip-package        Reuse existing dist/releases/ instead of running install/package-release.sh
  --verify              After build, check stx/agent/java-proxy/frontend sha256 in the image == dist/releases
  --smoke               After build, run scripts/docker-smoke-all-in-one.sh against the all-in-one image
  -h, --help            Show this help

Env:
  DOCKER                Docker CLI command (default: docker; e.g. DOCKER="sudo docker")

Examples:
  scripts/docker-build-images.sh                                   # package + build all images for host arch
  scripts/docker-build-images.sh --skip-package --verify --smoke   # rebuild images from existing artifacts
  scripts/docker-build-images.sh --platforms linux/amd64,linux/arm64 --images all-in-one
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --platforms) PLATFORMS="${2:-}"; shift 2 ;;
    --images) IMAGES="${2:-}"; shift 2 ;;
    --tag) TAG="${2:-}"; shift 2 ;;
    --version) APP_VERSION="${2:-}"; shift 2 ;;
    --skip-package) SKIP_PACKAGE=true; shift ;;
    --verify) VERIFY=true; shift ;;
    --smoke) SMOKE=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

cd "$ROOT_DIR"

if [[ -z "$APP_VERSION" ]]; then
  APP_VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
fi
GIT_COMMIT="$(git rev-parse HEAD 2>/dev/null || echo unknown)"

IFS=',' read -r -a platform_list <<<"$PLATFORMS"
IFS=',' read -r -a image_list <<<"$IMAGES"

for image in "${image_list[@]}"; do
  case "$image" in
    backend|frontend|all-in-one) ;;
    *) echo "unknown image: $image (supported: backend, frontend, all-in-one)" >&2; exit 1 ;;
  esac
done

if [[ "$SKIP_PACKAGE" != "true" ]]; then
  # 与发版 package-release job 相同的参数，保证本地镜像和 Release 资产同源。
  # Same flags as the release package-release job, so local images match Release assets.
  echo "==> packaging release artifacts (version=$APP_VERSION)"
  install/package-release.sh --arch all --layout split --build-frontend --version "$APP_VERSION"
fi

built=()
for platform in "${platform_list[@]}"; do
  arch="${platform#linux/}"
  for image in "${image_list[@]}"; do
    ref="stx-${image}:${TAG}-${arch}"
    echo "==> building $ref ($platform) from dist/releases"
    # shellcheck disable=SC2086
    $DOCKER buildx build \
      --platform "$platform" \
      --file ".github/docker/Dockerfile.${image}" \
      --build-arg "APP_VERSION=${APP_VERSION}" \
      --build-arg "GIT_COMMIT=${GIT_COMMIT}" \
      --tag "$ref" \
      --load \
      .
    built+=("$ref")

    if [[ "$VERIFY" == "true" ]]; then
      DOCKER="$DOCKER" scripts/docker-verify-release-artifacts.sh "$ref" "$image" "$arch"
    fi
    if [[ "$SMOKE" == "true" && "$image" == "all-in-one" ]]; then
      DOCKER="$DOCKER" scripts/docker-smoke-all-in-one.sh "$ref" --platform "$platform"
    fi
  done
done

echo
echo "built images:"
for ref in "${built[@]}"; do
  echo "  $ref"
done
