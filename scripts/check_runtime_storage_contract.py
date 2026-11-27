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

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

PARAM_KEYS = [
    "namespace",
    "hdfs_namenode_host",
    "hdfs_namenode_port",
    "kerberos_principal",
    "kerberos_keytab_file_path",
    "hdfs_ha_enabled",
    "hdfs_name_services",
    "hdfs_ha_namenodes",
    "hdfs_namenode_rpc_address_1",
    "hdfs_namenode_rpc_address_2",
    "hdfs_failover_proxy_provider",
    "hdfs_site_path",
    "disable_cache",
    "storage_endpoint",
    "s3_credentials_provider",
    "storage_access_key",
    "storage_secret_key",
    "storage_bucket",
]

CONTROL_PLANE = ROOT / "internal/apps/cluster/runtime_storage_external.go"
AGENT_PRECHECK = ROOT / "agent/internal/executor/precheck_handlers.go"
RUNTIME_SPEC = ROOT / "internal/apps/cluster/runtime_storage.go"


STRUCT_FIELDS = [
    "HDFSNameNodeHost",
    "HDFSNameNodePort",
    "KerberosPrincipal",
    "KerberosKeytabFilePath",
    "HDFSHAEnabled",
    "HDFSNameServices",
    "HDFSHANamenodes",
    "HDFSNamenodeRPCAddress1",
    "HDFSNamenodeRPCAddress2",
    "HDFSFailoverProxyProvider",
    "HdfsSitePath",
    "DisableCache",
    "S3CredentialsProvider",
]


def extract_go_func(text: str, func_name: str) -> str:
    match = re.search(rf"^func\s+{re.escape(func_name)}\b.*$", text, re.MULTILINE)
    if not match:
        return ""
    start = match.start()
    next_func = re.search(r"^func\s+", text[match.end():], re.MULTILINE)
    end = match.end() + next_func.start() if next_func else len(text)
    return text[start:end]


def require_params(text: str, keys: list[str]) -> list[str]:
    return [key for key in keys if f'params["{key}"]' not in text]


def require_fields(text: str, fields: list[str]) -> list[str]:
    return [field for field in fields if not re.search(rf"\b{re.escape(field)}\b", text)]


def main() -> int:
    errors = []
    control_text = CONTROL_PLANE.read_text(encoding="utf-8")
    agent_text = AGENT_PRECHECK.read_text(encoding="utf-8")
    spec_text = RUNTIME_SPEC.read_text(encoding="utf-8")

    # 逐函数检查控制面与 Agent 的参数透传，防止某一侧（如 IMAP）漏字段被另一侧掩盖。
    # Check param passthrough per function on both control plane and agent so one path cannot mask gaps in another.
    for path, text, func_name in (
        (CONTROL_PLANE, control_text, "fillRuntimeStorageProxyParams"),
        (AGENT_PRECHECK, agent_text, "checkpointConfigFromParams"),
        (AGENT_PRECHECK, agent_text, "imapConfigFromParams"),
    ):
        body = extract_go_func(text, func_name)
        if not body:
            errors.append(f"{path.relative_to(ROOT)} missing function {func_name}")
            continue
        missing = require_params(body, PARAM_KEYS)
        if missing:
            errors.append(f"{path.relative_to(ROOT)}:{func_name} missing runtime-storage params: {', '.join(missing)}")

    # 检查控制面 YAML/集群配置映射函数是否保留全部 HDFS HA / Kerberos / S3 高级字段。
    # Check control-plane YAML and cluster-config mappers preserve all HDFS HA / Kerberos / S3 fields.
    for path, text, func_name in (
        (CONTROL_PLANE, control_text, "parseCheckpointValidationConfigFromYAML"),
        (CONTROL_PLANE, control_text, "parseIMAPValidationConfigFromYAML"),
        (CONTROL_PLANE, control_text, "checkpointValidationConfigFromCluster"),
        (CONTROL_PLANE, control_text, "imapValidationConfigFromCluster"),
        (RUNTIME_SPEC, spec_text, "runtimeSpecFromClusterConfig"),
    ):
        body = extract_go_func(text, func_name)
        if not body:
            errors.append(f"{path.relative_to(ROOT)} missing function {func_name}")
            continue
        missing = require_fields(body, STRUCT_FIELDS)
        if missing:
            errors.append(f"{path.relative_to(ROOT)}:{func_name} missing runtime-storage fields: {', '.join(missing)}")

    direct_namenode_pattern = re.compile(r'rpc-address\."?\s*\+\s*\w+\s*\+\s*"\.nn[12]')
    for path, text in ((CONTROL_PLANE, control_text), (RUNTIME_SPEC, spec_text)):
        if direct_namenode_pattern.search(text):
            errors.append(f"{path.relative_to(ROOT)} hard-codes nn1/nn2 instead of resolving HDFS HA namenode aliases")

    if errors:
        for error in errors:
            print(f"[runtime-storage-contract] FAIL {error}", file=sys.stderr)
        return 1
    print("[runtime-storage-contract] OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
