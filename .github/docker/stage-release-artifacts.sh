#!/bin/sh
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

# 在 Docker 构建阶段内从 dist/releases/ 暂存发版产物，镜像不再自行编译。
# Stage release artifacts from dist/releases/ inside a Docker build stage; images never compile.
#
# 用法 / Usage: stage-release-artifacts.sh <releases-dir> <out-dir> <target-arch> <component>...
#   component: backend  -> stx-linux-<arch>, stx-agent-linux-{amd64,arm64}, stx-java-proxy-{v2,v3}.jar
#              frontend -> frontend-standalone-linux-<arch>.tar.gz
#
# 输出布局 / Output layout:
#   <out-dir>/stx
#   <out-dir>/lib/agent/stx-agent-linux-{amd64,arm64}
#   <out-dir>/lib/stx-java-proxy-{v2,v3}.jar
#   <out-dir>/frontend/            (unpacked Next.js standalone, contains server.js)
#
# 缺任何产物都会列出全部缺失文件并以非零退出。
# Any missing artifact lists every missing file and exits non-zero.

set -eu

if [ "$#" -lt 4 ]; then
  echo "usage: $0 <releases-dir> <out-dir> <target-arch> <component>..." >&2
  exit 2
fi

releases_dir="$1"
out_dir="$2"
arch="$3"
shift 3

case "$arch" in
  amd64|arm64) ;;
  *)
    echo "[stage-release-artifacts] unsupported target arch: '$arch' (supported: amd64, arm64)" >&2
    exit 2
    ;;
esac

want_backend=false
want_frontend=false
for component in "$@"; do
  case "$component" in
    backend) want_backend=true ;;
    frontend) want_frontend=true ;;
    *)
      echo "[stage-release-artifacts] unknown component: $component" >&2
      exit 2
      ;;
  esac
done

stx_bin="stx-linux-${arch}"
agent_bins="stx-agent-linux-amd64 stx-agent-linux-arm64"
proxy_jars="stx-java-proxy-v2.jar stx-java-proxy-v3.jar"
frontend_tar="frontend-standalone-linux-${arch}.tar.gz"

required=""
if [ "$want_backend" = "true" ]; then
  required="$required $stx_bin $agent_bins $proxy_jars"
fi
if [ "$want_frontend" = "true" ]; then
  required="$required $frontend_tar"
fi

missing=""
for name in $required; do
  if [ ! -s "$releases_dir/$name" ]; then
    missing="$missing $name"
  fi
done

if [ -n "$missing" ]; then
  {
    echo "[stage-release-artifacts] ERROR: missing release artifacts in dist/releases/ (target arch: $arch):"
    for name in $missing; do
      echo "  - dist/releases/$name"
    done
    echo "[stage-release-artifacts] Images only consume release artifacts and never compile from source."
    echo "[stage-release-artifacts] Build them first, e.g.:"
    echo "  install/package-release.sh --arch all --layout split --build-frontend"
    echo "  (or: make docker-images, see docs/打包发布说明.md)"
  } >&2
  exit 1
fi

mkdir -p "$out_dir"

if [ "$want_backend" = "true" ]; then
  mkdir -p "$out_dir/lib/agent"
  cp "$releases_dir/$stx_bin" "$out_dir/stx"
  for name in $agent_bins; do
    cp "$releases_dir/$name" "$out_dir/lib/agent/$name"
  done
  for name in $proxy_jars; do
    cp "$releases_dir/$name" "$out_dir/lib/$name"
  done
  # actions/upload-artifact 不保留可执行位，这里统一补齐。
  # actions/upload-artifact drops the executable bit; restore it here.
  chmod 0755 "$out_dir/stx" "$out_dir"/lib/agent/*
  chmod 0644 "$out_dir"/lib/*.jar
fi

if [ "$want_frontend" = "true" ]; then
  mkdir -p "$out_dir/frontend"
  tar -xzf "$releases_dir/$frontend_tar" -C "$out_dir/frontend"
  if [ ! -f "$out_dir/frontend/server.js" ]; then
    echo "[stage-release-artifacts] ERROR: dist/releases/$frontend_tar does not contain server.js at its root" >&2
    exit 1
  fi
fi

echo "[stage-release-artifacts] staged (arch=$arch, components=$*):"
(cd "$out_dir" && find . -maxdepth 3 -type f ! -path './frontend/*' -exec sha256sum {} + | sort -k2)
