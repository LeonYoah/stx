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

# 种子资产补齐：若 /opt/stx/lib 缺少内置代理或 Agent 资产（例如被宿主机空卷挂载覆盖），自动从种子目录补齐
# Seed assets auto-completion: if /opt/stx/lib is missing builtin proxy or Agent assets (e.g. masked by host bind mount), seed from defaults
if [[ -d /opt/stx/default-lib ]]; then
  mkdir -p /opt/stx/lib
  cp -rn /opt/stx/default-lib/* /opt/stx/lib/ 2>/dev/null || true
fi

# 如果没有传入参数，默认执行 server 命令
# If no arguments provided, default to server subcommand
if [[ "$#" -eq 0 ]]; then
  exec /opt/stx/stx server
elif [[ "$1" == "server" || "$1" == "version" || "$1" == "help" || "$1" == -* ]]; then
  exec /opt/stx/stx "$@"
else
  exec "$@"
fi
