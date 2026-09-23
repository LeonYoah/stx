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

# Render observability configs from install/observability templates.
# profile: bare | docker
# 从 install/observability 模板渲染可观测性配置。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE="${1:-}"
OUT_DIR="${2:-}"

usage() {
  cat <<'USAGE'
Usage:
  install/observability/render.sh <bare|docker> <output-dir>

Examples:
  install/observability/render.sh bare /opt/stx/observability
  install/observability/render.sh docker deploy/docker/observability
USAGE
}

if [[ -z "$PROFILE" || -z "$OUT_DIR" ]]; then
  usage
  exit 1
fi

case "$PROFILE" in
  bare)
    ALERTMANAGER_TARGET="${ALERTMANAGER_TARGET:-127.0.0.1:9093}"
    PROMETHEUS_TARGET="${PROMETHEUS_TARGET:-127.0.0.1:9090}"
    STX_SD_URL="${STX_SD_URL:-http://127.0.0.1:17800/api/v1/monitoring/prometheus/discovery}"
    STX_WEBHOOK_URL="${STX_WEBHOOK_URL:-http://127.0.0.1:17800/api/v1/monitoring/alertmanager/webhook}"
    RULE_FILES="${RULE_FILES:-rules/*.yml}"
    PROMETHEUS_URL="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
    GRAFANA_DASHBOARDS_PATH="${GRAFANA_DASHBOARDS_PATH:-__GRAFANA_DASHBOARDS_PATH__}"
    ;;
  docker)
    ALERTMANAGER_TARGET="${ALERTMANAGER_TARGET:-alertmanager:9093}"
    PROMETHEUS_TARGET="${PROMETHEUS_TARGET:-localhost:9090}"
    STX_SD_URL="${STX_SD_URL:-http://stx-backend:17800/api/v1/monitoring/prometheus/discovery}"
    STX_WEBHOOK_URL="${STX_WEBHOOK_URL:-http://stx-backend:17800/api/v1/monitoring/alertmanager/webhook}"
    RULE_FILES="${RULE_FILES:-/etc/prometheus/rules/*.yml}"
    PROMETHEUS_URL="${PROMETHEUS_URL:-http://prometheus:9090}"
    GRAFANA_DASHBOARDS_PATH="${GRAFANA_DASHBOARDS_PATH:-/var/lib/grafana/dashboards}"
    ;;
  *)
    echo "unknown profile: $PROFILE (expected bare|docker)" >&2
    usage
    exit 1
    ;;
esac

mkdir -p \
  "$OUT_DIR/prometheus/rules" \
  "$OUT_DIR/alertmanager" \
  "$OUT_DIR/grafana/provisioning/datasources" \
  "$OUT_DIR/grafana/provisioning/dashboards" \
  "$OUT_DIR/grafana/dashboards"

render_file() {
  local src="$1"
  local dst="$2"
  sed \
    -e "s#__ALERTMANAGER_TARGET__#${ALERTMANAGER_TARGET}#g" \
    -e "s#__PROMETHEUS_TARGET__#${PROMETHEUS_TARGET}#g" \
    -e "s#__STX_SD_URL__#${STX_SD_URL}#g" \
    -e "s#__STX_WEBHOOK_URL__#${STX_WEBHOOK_URL}#g" \
    -e "s#__RULE_FILES__#${RULE_FILES}#g" \
    -e "s#__PROMETHEUS_URL__#${PROMETHEUS_URL}#g" \
    -e "s#__GRAFANA_DASHBOARDS_PATH__#${GRAFANA_DASHBOARDS_PATH}#g" \
    "$src" >"$dst"
}

render_file \
  "$SCRIPT_DIR/prometheus_config/prometheus.yml.tpl" \
  "$OUT_DIR/prometheus/prometheus.yml"

render_file \
  "$SCRIPT_DIR/alertmanager_config/alertmanager.yml.tpl" \
  "$OUT_DIR/alertmanager/alertmanager.yml"

if [[ -f "$SCRIPT_DIR/grafana_config/provisioning/datasources/prometheus.yml.tpl" ]]; then
  render_file \
    "$SCRIPT_DIR/grafana_config/provisioning/datasources/prometheus.yml.tpl" \
    "$OUT_DIR/grafana/provisioning/datasources/prometheus.yml"
fi

if [[ -f "$SCRIPT_DIR/grafana_config/provisioning/dashboards/default.yml" ]]; then
  render_file \
    "$SCRIPT_DIR/grafana_config/provisioning/dashboards/default.yml" \
    "$OUT_DIR/grafana/provisioning/dashboards/default.yml"
fi

if compgen -G "$SCRIPT_DIR/prometheus_config/rules/*.yml" > /dev/null; then
  cp "$SCRIPT_DIR/prometheus_config/rules/"*.yml "$OUT_DIR/prometheus/rules/"
fi

if compgen -G "$SCRIPT_DIR/grafana_config/dashboards/*.json" > /dev/null; then
  cp "$SCRIPT_DIR/grafana_config/dashboards/"*.json "$OUT_DIR/grafana/dashboards/"
fi

# Marker so humans know not to hand-edit.
cat >"$OUT_DIR/README.md" <<EOF
# Generated observability configs ($PROFILE)

Source templates: \`install/observability/\`
Regenerate:

\`\`\`bash
./install/observability/render.sh $PROFILE $OUT_DIR
\`\`\`

Do not edit these files by hand.
EOF

echo "rendered $PROFILE observability configs -> $OUT_DIR"
