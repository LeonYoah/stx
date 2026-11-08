#!/usr/bin/env bash
#
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
#
# 生成开屏中文欢迎语专用的极小字体（Noto Serif SC Light 子集）。
# Build the tiny splash greeting font (a Noto Serif SC Light subset).
#
# 为什么：欢迎语只有几个字，整套 Noto Serif SC 有约 6MB 分片和约 100KB 的
# @font-face 声明，而且会进入每个页面的全局 CSS。这里只裁出用到的字形，产物仅几 KB。
# Why: the greeting is a handful of glyphs; the full Noto Serif SC package ships
# ~6MB of subsets plus ~100KB of @font-face rules that land in every page's CSS.
# We keep only the glyphs in use, producing a file of a few KB.
#
# 何时运行：修改 lib/i18n/locales/zh.json 中的 splash.welcomeBack 后必须重跑并提交产物。
# When to run: re-run and commit the output whenever splash.welcomeBack in
# lib/i18n/locales/zh.json changes.
#
# 依赖：curl、node、uv（uvx 临时拉起 fonttools，不进入项目依赖）。
# Requires: curl, node, uv (uvx fetches fonttools ad hoc; not a project dep).
#
# 用法 / Usage: cd frontend && bash scripts/fonts/build-splash-greeting-font.sh

set -euo pipefail

# 源字体固定到 google/fonts 的具体提交，保证产物可复现。
# Source font pinned to a google/fonts commit for reproducible output.
SRC_COMMIT="2e61f4355afd22b801791b0df176065082423b87"
SRC_BASE="https://raw.githubusercontent.com/google/fonts/${SRC_COMMIT}/ofl/notoserifsc"
# 与 welcome-splash.css 中的 font-weight 保持一致 / Must match font-weight in welcome-splash.css.
WEIGHT=300

FRONTEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${FRONTEND_DIR}/public/fonts"
OUT_FONT="${OUT_DIR}/stx-splash-greeting.woff2"
OUT_LICENSE="${OUT_DIR}/stx-splash-greeting.OFL.txt"
# 记录产物对应的文案与源字体，单测据此校验文案未漂移。
# Records the text and source behind the output; a unit test checks for drift.
OUT_MANIFEST="${OUT_DIR}/stx-splash-greeting.json"

# 从语言包读取欢迎语，避免文案与字体不一致 / Read the greeting from the locale file so glyphs never drift.
TEXT="$(node -e "process.stdout.write(require('${FRONTEND_DIR}/lib/i18n/locales/zh.json').splash.welcomeBack)")"
if [[ -z "${TEXT}" ]]; then
  echo "splash.welcomeBack is empty in zh.json" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

echo "→ downloading Noto Serif SC (${SRC_COMMIT:0:7})"
curl -fsSL "${SRC_BASE}/NotoSerifSC%5Bwght%5D.ttf" -o "${TMP_DIR}/source.ttf"
curl -fsSL "${SRC_BASE}/OFL.txt" -o "${OUT_LICENSE}"

echo "→ instancing wght=${WEIGHT}"
uvx --from 'fonttools[woff]' fonttools varLib.instancer \
  "${TMP_DIR}/source.ttf" "wght=${WEIGHT}" --update-name-table -o "${TMP_DIR}/instance.ttf" -q

echo "→ subsetting: ${TEXT}"
# 保留全部 name 记录（含版权与 OFL 许可声明）/ Keep all name records (copyright and OFL notice).
uvx --from 'fonttools[woff]' pyftsubset "${TMP_DIR}/instance.ttf" \
  --text="${TEXT}" \
  --flavor=woff2 \
  --layout-features='*' \
  --name-IDs='*' \
  --no-hinting \
  --output-file="${OUT_FONT}"

TEXT="${TEXT}" SRC_COMMIT="${SRC_COMMIT}" WEIGHT="${WEIGHT}" node -e "
const m = {text: process.env.TEXT, source: 'google/fonts@' + process.env.SRC_COMMIT + ':ofl/notoserifsc/NotoSerifSC[wght].ttf', weight: Number(process.env.WEIGHT)};
require('fs').writeFileSync(process.argv[1], JSON.stringify(m, null, 2) + '\\n');
" "${OUT_MANIFEST}"

echo "✓ ${OUT_FONT} ($(wc -c < "${OUT_FONT}") bytes)"
