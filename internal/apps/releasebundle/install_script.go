/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package releasebundle provides temporary download and install endpoints for STX bundles.
// releasebundle 包提供 STX 离线发布包的临时下载与安装端点。
package releasebundle

import (
	"bytes"
	"fmt"
	"text/template"
)

// InstallScriptData holds the values injected into the release install script template.
// InstallScriptData 保存渲染发布包安装脚本模板所需的数据。
type InstallScriptData struct {
	// DownloadURL points to the package download endpoint.
	// DownloadURL 指向发布包下载端点。
	DownloadURL string
	// ExampleCommand is shown when the script is not executed as root.
	// ExampleCommand 用于在非 root 执行时展示示例命令。
	ExampleCommand string
}

// GenerateInstallScript renders the STX bundle install script.
// GenerateInstallScript 渲染 STX 离线包安装脚本。
func GenerateInstallScript(data InstallScriptData) (string, error) {
	if data.DownloadURL == "" {
		return "", fmt.Errorf("download url is required")
	}
	if data.ExampleCommand == "" {
		data.ExampleCommand = "curl -fsSL <install-script-url> | sudo bash"
	}

	tmpl, err := template.New("release_bundle_install_script").Parse(releaseBundleInstallScriptTemplate)
	if err != nil {
		return "", fmt.Errorf("parse release bundle install script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render release bundle install script template: %w", err)
	}

	return buf.String(), nil
}

const releaseBundleInstallScriptTemplate = `#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/stx}"
FORCE_INSTALL="${FORCE_INSTALL:-0}"
PRESERVE_CONFIG="${PRESERVE_CONFIG:-1}"
AUTO_START="${AUTO_START:-1}"
TMP_ROOT="${TMP_ROOT:-/tmp}"
LOGIN_USERNAME="${STX_USERNAME:-}"
LOGIN_PASSWORD="${STX_PASSWORD:-}"
DOWNLOAD_URL="{{.DownloadURL}}"

log() {
  printf '[STX] %s\n' "$*"
}

fail() {
  printf '[STX][ERROR] %s\n' "$*" >&2
  exit 1
}

cleanup() {
  if [[ -n "${WORK_DIR:-}" && -d "${WORK_DIR:-}" ]]; then
    rm -rf "${WORK_DIR}"
  fi
}

download_file() {
  local url="$1"
  local output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -u "${LOGIN_USERNAME}:${LOGIN_PASSWORD}" -o "$output" "$url"
    return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -q --user="${LOGIN_USERNAME}" --password="${LOGIN_PASSWORD}" -O "$output" "$url"
    return 0
  fi
  fail "curl or wget is required"
}

prompt_credentials() {
  if [[ -n "${LOGIN_USERNAME}" && -n "${LOGIN_PASSWORD}" ]]; then
    return 0
  fi

  if [[ ! -r /dev/tty ]]; then
    fail "set STX_USERNAME and STX_PASSWORD when running non-interactively"
  fi

  if [[ -z "${LOGIN_USERNAME}" ]]; then
    printf 'STX username: ' > /dev/tty
    read -r LOGIN_USERNAME < /dev/tty
  fi
  if [[ -z "${LOGIN_PASSWORD}" ]]; then
    printf 'STX password: ' > /dev/tty
    stty -echo < /dev/tty
    read -r LOGIN_PASSWORD < /dev/tty
    stty echo < /dev/tty
    printf '\n' > /dev/tty
  fi

  if [[ -z "${LOGIN_USERNAME}" || -z "${LOGIN_PASSWORD}" ]]; then
    fail "username and password are required"
  fi
}

if [[ "${EUID}" -ne 0 ]]; then
  fail "please run this script with sudo or as root. Example: {{.ExampleCommand}}"
fi

WORK_DIR="$(mktemp -d "${TMP_ROOT%/}/stx-install.XXXXXX")"
trap cleanup EXIT

ARCHIVE_PATH="${WORK_DIR}/stx.tar.gz"
prompt_credentials
log "downloading STX bundle ..."
download_file "${DOWNLOAD_URL}" "${ARCHIVE_PATH}"

log "extracting bundle ..."
tar -xzf "${ARCHIVE_PATH}" -C "${WORK_DIR}"

PKG_DIR="$(find "${WORK_DIR}" -mindepth 1 -maxdepth 1 -type d | head -n1)"
if [[ -z "${PKG_DIR}" ]]; then
  fail "failed to locate extracted package directory"
fi

INSTALL_ARGS=(--install-dir "${INSTALL_DIR}")
if [[ "${FORCE_INSTALL}" == "1" ]]; then
  INSTALL_ARGS+=(--force)
fi
if [[ "${PRESERVE_CONFIG}" != "1" ]]; then
  INSTALL_ARGS+=(--no-preserve-config)
fi
if [[ "${AUTO_START}" != "1" ]]; then
  INSTALL_ARGS+=(--no-start)
fi

log "running bundled installer ..."
bash "${PKG_DIR}/install.sh" "${INSTALL_ARGS[@]}"

log "done."
log "install dir: ${INSTALL_DIR}"
`
