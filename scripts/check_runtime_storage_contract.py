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


def require_params(path: Path, keys: list[str]) -> list[str]:
    text = path.read_text(encoding="utf-8")
    return [key for key in keys if f'params["{key}"]' not in text]


def main() -> int:
    errors = []
    for path in (CONTROL_PLANE, AGENT_PRECHECK):
        missing = require_params(path, PARAM_KEYS)
        if missing:
            errors.append(f"{path.relative_to(ROOT)} missing runtime-storage params: {', '.join(missing)}")

    control_text = CONTROL_PLANE.read_text(encoding="utf-8")
    spec_text = RUNTIME_SPEC.read_text(encoding="utf-8")
    direct_namenode_pattern = re.compile(r'rpc-address\."?\s*\+\s*nameServices\s*\+\s*"\.nn[12]')
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
