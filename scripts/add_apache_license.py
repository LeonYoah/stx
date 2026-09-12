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

from __future__ import annotations

import argparse
import codecs
import subprocess
import sys
from pathlib import Path, PurePosixPath

BLOCK_COMMENT_EXTENSIONS = {".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}
HASH_COMMENT_EXTENSIONS = {".py", ".sh", ".yml", ".yaml"}
SUPPORTED_EXTENSIONS = BLOCK_COMMENT_EXTENSIONS | HASH_COMMENT_EXTENSIONS
EXCLUDED_DIRS = {
    ".git",
    ".next",
    "node_modules",
    "dist",
    "dist-standalone",
    "build",
    "coverage",
    "deps",
    "deps_bak",
    "vendor",
}
APACHE_MARKERS = (
    "Licensed to the Apache Software Foundation (ASF) under one or more",
    "Licensed under the Apache License, Version 2.0",
    "The ASF licenses this file to You under the Apache License, Version 2.0",
    "Apache License, Version 2.0",
)
MIT_MARKERS = ("MIT License",)

BLOCK_HEADER = """/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the \"License\"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an \"AS IS\" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */"""

HASH_HEADER = """# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the \"License\"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License."""


def run_git(*args: str) -> str:
    proc = subprocess.run(
        ["git", *args],
        check=False,
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        raise SystemExit(proc.stderr.strip() or proc.stdout.strip())
    return proc.stdout


def is_supported_path(path: str) -> bool:
    posix = PurePosixPath(path)
    if posix.suffix.lower() not in SUPPORTED_EXTENSIONS:
        return False
    return not any(part in EXCLUDED_DIRS for part in posix.parts)


def collect_git_added_paths(staged: bool) -> list[str]:
    paths: set[str] = set()
    if staged:
        output = run_git("diff", "--cached", "--name-only", "--diff-filter=A")
        paths.update(line.strip() for line in output.splitlines() if line.strip())
        return sorted(path for path in paths if is_supported_path(path))

    # Working tree mode: include staged adds plus untracked files.
    output = run_git("diff", "HEAD", "--name-only", "--diff-filter=A")
    paths.update(line.strip() for line in output.splitlines() if line.strip())
    output = run_git("ls-files", "--others", "--exclude-standard")
    paths.update(line.strip() for line in output.splitlines() if line.strip())
    return sorted(path for path in paths if is_supported_path(path))


def split_go_build_constraint_prefix(text: str) -> tuple[str, str]:
    lines = text.splitlines(keepends=True)
    if not lines:
        return "", text

    idx = 0
    saw_build_constraint = False
    while idx < len(lines):
        stripped = lines[idx].strip()
        if not stripped:
            idx += 1
            if saw_build_constraint:
                break
            continue
        if stripped.startswith("//go:build") or stripped.startswith("// +build"):
            saw_build_constraint = True
            idx += 1
            continue
        break

    if not saw_build_constraint:
        return "", text
    return "".join(lines[:idx]), "".join(lines[idx:])


def extract_leading_comment(text: str, suffix: str) -> str:
    text = text.lstrip("\ufeff")
    if text.startswith("#!"):
        _, _, text = text.partition("\n")
    text = text.lstrip("\n")

    if suffix == ".go":
        _, text = split_go_build_constraint_prefix(text)
        text = text.lstrip("\n")

    if text.startswith("/*"):
        end = text.find("*/")
        return text[: end + 2] if end != -1 else text[:4096]

    comment_lines: list[str] = []
    for line in text.splitlines():
        stripped = line.lstrip()
        if not stripped:
            if comment_lines:
                comment_lines.append(line)
            continue
        if stripped.startswith("#") or stripped.startswith("//"):
            comment_lines.append(line)
            continue
        break
    return "\n".join(comment_lines)


def detect_existing_license(text: str, suffix: str) -> str:
    header = extract_leading_comment(text, suffix)
    if any(marker in header for marker in MIT_MARKERS):
        return "mit"
    if any(marker in header for marker in APACHE_MARKERS):
        return "apache"
    return "none"


def read_text_preserve_bom(path: Path) -> tuple[bool, str]:
    data = path.read_bytes()
    has_bom = data.startswith(codecs.BOM_UTF8)
    if has_bom:
        data = data[len(codecs.BOM_UTF8):]
    return has_bom, data.decode("utf-8")


def write_text_preserve_bom(path: Path, has_bom: bool, text: str) -> None:
    encoded = text.encode("utf-8")
    if has_bom:
        encoded = codecs.BOM_UTF8 + encoded
    path.write_bytes(encoded)


def build_header_for_path(path: Path) -> str:
    suffix = path.suffix.lower()
    if suffix in BLOCK_COMMENT_EXTENSIONS:
        return BLOCK_HEADER
    if suffix in HASH_COMMENT_EXTENSIONS:
        return HASH_HEADER
    raise SystemExit(f"unsupported file type for Apache header insertion: {path}")


def insert_header(text: str, header: str, suffix: str) -> str:
    if text.startswith("#!"):
        first_line, _, remainder = text.partition("\n")
        remainder = remainder.lstrip("\n")
        return f"{first_line}\n{header}\n\n{remainder}" if remainder else f"{first_line}\n{header}\n"

    if suffix == ".go":
        prefix, remainder = split_go_build_constraint_prefix(text)
        if prefix:
            remainder = remainder.lstrip("\n")
            return f"{prefix}{header}\n\n{remainder}" if remainder else f"{prefix}{header}\n"

    stripped = text.lstrip("\n")
    return f"{header}\n\n{stripped}" if stripped else f"{header}\n"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Batch-add Apache 2.0 headers to new STX source/config files."
    )
    parser.add_argument("paths", nargs="*", help="Explicit file paths to update")
    parser.add_argument(
        "--git-added",
        action="store_true",
        help="Auto-select git-added files instead of passing paths manually",
    )
    parser.add_argument(
        "--staged",
        action="store_true",
        help="When used with --git-added, only inspect staged added files",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print files that would be updated without modifying them",
    )
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    if not args.paths and not args.git_added:
        parser.error("provide file paths or use --git-added")

    paths = list(args.paths)
    if args.git_added:
        paths.extend(collect_git_added_paths(staged=args.staged))

    unique_paths = []
    seen = set()
    for raw_path in paths:
        path = raw_path.strip()
        if not path or path in seen:
            continue
        seen.add(path)
        if is_supported_path(path):
            unique_paths.append(path)

    if not unique_paths:
        print("No supported files to update.")
        return 0

    updated = 0
    skipped = 0

    for raw_path in unique_paths:
        path = Path(raw_path)
        if not path.exists() or not path.is_file():
            print(f"skip missing file: {raw_path}", file=sys.stderr)
            skipped += 1
            continue

        has_bom, text = read_text_preserve_bom(path)
        license_kind = detect_existing_license(text, path.suffix.lower())
        if license_kind == "apache":
            print(f"skip already Apache: {raw_path}")
            skipped += 1
            continue
        if license_kind == "mit":
            print(f"skip MIT legacy file: {raw_path}", file=sys.stderr)
            skipped += 1
            continue

        header = build_header_for_path(path)
        new_text = insert_header(text, header, path.suffix.lower())
        if args.dry_run:
            print(f"would update: {raw_path}")
            updated += 1
            continue

        write_text_preserve_bom(path, has_bom, new_text)
        print(f"updated: {raw_path}")
        updated += 1

    print(f"Done. updated={updated} skipped={skipped}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
