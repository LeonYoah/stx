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
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Iterable

SUPPORTED_EXTENSIONS = {
    ".go",
    ".ts",
    ".tsx",
    ".js",
    ".jsx",
    ".mjs",
    ".cjs",
    ".py",
    ".sh",
    ".yml",
    ".yaml",
}

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
HEADER_SCAN_BYTES = 4096
DEFAULT_ALLOWLIST = "licenses/legacy_mit_files.txt"


@dataclass(frozen=True)
class Change:
    status: str
    path: str
    old_path: str | None = None


class LicenseCheckError(RuntimeError):
    pass


def run_git(*args: str) -> str:
    proc = subprocess.run(
        ["git", *args],
        check=False,
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        stderr = proc.stderr.strip()
        raise LicenseCheckError(f"git {' '.join(args)} failed: {stderr or proc.stdout.strip()}")
    return proc.stdout


def parse_changes(output: str) -> list[Change]:
    changes: list[Change] = []
    for raw_line in output.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        parts = line.split("\t")
        status = parts[0]
        kind = status[:1]
        if kind == "R" and len(parts) >= 3:
            changes.append(Change(status="R", old_path=parts[1], path=parts[2]))
            continue
        if kind in {"A", "M"} and len(parts) >= 2:
            changes.append(Change(status=kind, path=parts[1]))
    return changes


def iter_untracked_files() -> Iterable[Change]:
    output = run_git("ls-files", "--others", "--exclude-standard")
    for raw_line in output.splitlines():
        path = raw_line.strip()
        if path:
            yield Change(status="A", path=path)


def collect_changes(args: argparse.Namespace) -> list[Change]:
    if args.base_ref:
        diff_range = f"{args.base_ref}...{args.head_ref}"
        return parse_changes(
            run_git(
                "diff",
                "--name-status",
                "--find-renames",
                "--diff-filter=AMR",
                diff_range,
            )
        )

    if args.working_tree:
        changes = parse_changes(
            run_git(
                "diff",
                "--name-status",
                "--find-renames",
                "--diff-filter=AMR",
                "HEAD",
            )
        )
        changes.extend(iter_untracked_files())
        return dedupe_changes(changes)

    # Default local mode: staged changes only, which keeps random local artifacts
    # from breaking the check.
    return parse_changes(
        run_git(
            "diff",
            "--cached",
            "--name-status",
            "--find-renames",
            "--diff-filter=AMR",
        )
    )


def dedupe_changes(changes: Iterable[Change]) -> list[Change]:
    deduped: dict[str, Change] = {}
    for change in changes:
        deduped[change.path] = change
    return list(deduped.values())


def load_allowlist(path: Path) -> set[str]:
    if not path.exists():
        raise LicenseCheckError(f"legacy MIT allowlist not found: {path}")

    allowlist: set[str] = set()
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        allowlist.add(line)
    return allowlist


def validate_allowlist_entries(allowlist: set[str]) -> list[str]:
    missing: list[str] = []
    for entry in sorted(allowlist):
        if not Path(entry).exists():
            missing.append(entry)
    return missing


def is_supported_source(path: str) -> bool:
    posix = PurePosixPath(path)
    if posix.suffix.lower() not in SUPPORTED_EXTENSIONS:
        return False
    return not any(part in EXCLUDED_DIRS for part in posix.parts)


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
        return text[: end + 2] if end != -1 else text[:HEADER_SCAN_BYTES]

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


def detect_license(path: Path) -> str:
    if not path.exists() or not path.is_file():
        return "missing"
    try:
        text = path.read_text(encoding="utf-8-sig", errors="ignore")
    except OSError as exc:
        raise LicenseCheckError(f"failed to read {path}: {exc}") from exc

    header = extract_leading_comment(text, path.suffix.lower())
    if any(marker in header for marker in MIT_MARKERS):
        return "mit"
    if any(marker in header for marker in APACHE_MARKERS):
        return "apache"
    return "none"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Check STX mixed license policy: legacy allowlisted files keep MIT, "
            "new source files must use Apache 2.0."
        )
    )
    parser.add_argument(
        "--allowlist",
        default=DEFAULT_ALLOWLIST,
        help=f"Path to legacy MIT allowlist (default: {DEFAULT_ALLOWLIST})",
    )
    parser.add_argument(
        "--base-ref",
        help="Base git ref/SHA for diff mode, e.g. origin/main or github.event.before",
    )
    parser.add_argument(
        "--head-ref",
        default="HEAD",
        help="Head git ref/SHA for diff mode (default: HEAD)",
    )
    parser.add_argument(
        "--working-tree",
        action="store_true",
        help="Check working tree changes against HEAD, including untracked files",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Print ignored modified files that are outside current policy enforcement",
    )
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    try:
        allowlist_path = Path(args.allowlist)
        allowlist = load_allowlist(allowlist_path)
        missing_allowlist_entries = validate_allowlist_entries(allowlist)

        changes = collect_changes(args)
        relevant_changes = [change for change in changes if is_supported_source(change.path)]
    except LicenseCheckError as exc:
        print(f"License policy check failed: {exc}", file=sys.stderr)
        return 1

    errors: list[str] = []
    notices: list[str] = []
    ignored_modified: list[str] = []

    if missing_allowlist_entries:
        errors.append(
            "legacy MIT allowlist contains missing paths; update "
            f"{allowlist_path}:\n  - " + "\n  - ".join(missing_allowlist_entries)
        )

    for change in relevant_changes:
        path = Path(change.path)
        license_kind = detect_license(path)

        if change.status == "A":
            if license_kind != "apache":
                errors.append(
                    f"new file must carry an Apache 2.0 header: {change.path} "
                    f"(detected: {license_kind})"
                )
            else:
                notices.append(f"Apache OK (new): {change.path}")
            continue

        is_legacy_mit = change.path in allowlist or (change.old_path in allowlist if change.old_path else False)
        if is_legacy_mit:
            if license_kind != "mit":
                errors.append(
                    f"legacy MIT file must keep an MIT header: {change.path} "
                    f"(detected: {license_kind})"
                )
            else:
                notices.append(f"MIT OK (legacy): {change.path}")
            continue

        if args.verbose:
            ignored_modified.append(change.path)

    if not relevant_changes:
        print("No relevant source-file changes to check.")
    else:
        for line in notices:
            print(line)
        if ignored_modified:
            print("Ignored existing non-legacy files (policy is diff-aware for debt control):")
            for path in sorted(ignored_modified):
                print(f"  - {path}")

    if errors:
        print("\nLicense policy check failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        print(
            "\nHint: add Apache 2.0 headers to new files with "
            "`python3 scripts/add_apache_license.py --git-added --staged`.",
            file=sys.stderr,
        )
        return 1

    print("License policy check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
