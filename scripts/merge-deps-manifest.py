#!/usr/bin/env python3
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
"""合并 deps Release 的 MANIFEST.json（仅 CI 使用）。

把本次 package-release.sh --deps-only 产出的若干 MANIFEST 片段合并进 deps 上已有的 MANIFEST.json：
只更新本次真正产出了文件（在 assets 目录中存在）的条目，其余条目（例如只重打三件套时的 Node 条目）原样保留。

Merge the MANIFEST fragments produced by this run's package-release.sh --deps-only invocations into the
MANIFEST.json already on the deps release. Only entries whose files were actually produced (present in the
assets dir) are updated; everything else (e.g. Node entries when only observability is rebuilt) is preserved.

Usage:
  merge-deps-manifest.py --assets-dir dist/releases --out dist/releases/MANIFEST.json \
      [--existing old/MANIFEST.json] fragment1.json [fragment2.json ...]
"""

import argparse
import datetime
import json
import os
import sys

ARCHES = ("amd64", "arm64")


def load_json(path):
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, dict):
        raise SystemExit(f"{path}: MANIFEST must be a JSON object")
    return data


def merge(existing, fragments, assets_dir, now):
    merged = dict(existing)
    updated = []
    for frag in fragments:
        # Node：片段带 node_version 且对应包确实产出时才更新。/ Node: only when built in this run.
        version = str(frag.get("node_version") or "")
        major = str(frag.get("node_major") or "")
        variant = str(frag.get("node_variant") or "")
        if version:
            built = False
            node_map = dict(merged.get("node") or {})
            for arch in ARCHES:
                asset = f"node-{major}-{variant}-linux-{arch}.tar.gz"
                if os.path.isfile(os.path.join(assets_dir, asset)):
                    node_map[asset] = {"major": major, "variant": variant, "version": version, "arch": arch}
                    updated.append(asset)
                    built = True
            if built:
                merged["node"] = node_map
                # 兼容旧字段：与原工作流一致，最后一次构建的 Node 生效。/ Legacy keys: last Node build wins.
                merged["node_major"] = major
                merged["node_variant"] = variant
                merged["node_version"] = version

        # 三件套：按架构，只登记本次产出的 tarball。/ Observability: per arch, only tarballs built now.
        obs = frag.get("observability") or {}
        built_obs = {
            arch: name
            for arch, name in obs.items()
            if isinstance(name, str) and name and os.path.isfile(os.path.join(assets_dir, name))
        }
        if built_obs:
            obs_map = dict(merged.get("observability") or {})
            obs_map.update(built_obs)
            merged["observability"] = obs_map
            for key in ("prometheus", "alertmanager", "grafana"):
                if frag.get(key):
                    merged[key] = frag[key]
            updated.extend(built_obs.values())

    if not updated:
        raise SystemExit("no deps assets from the fragments were found in the assets dir; nothing to merge")
    merged["build_time"] = now
    return merged, updated


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--assets-dir", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--existing", default="")
    ap.add_argument("--now", default="", help="override build_time (tests)")
    ap.add_argument("fragments", nargs="+")
    args = ap.parse_args(argv)

    existing = {}
    if args.existing and os.path.isfile(args.existing):
        existing = load_json(args.existing)
    fragments = [load_json(p) for p in args.fragments]
    now = args.now or datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    merged, updated = merge(existing, fragments, args.assets_dir, now)

    tmp = args.out + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(merged, f, indent=2, ensure_ascii=False)
        f.write("\n")
    os.replace(tmp, args.out)
    for name in updated:
        print(f"[manifest] updated: {name}", file=sys.stderr)
    kept = sorted(set((existing.get("node") or {}).keys()) - set(updated))
    for name in kept:
        print(f"[manifest] kept   : {name}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
