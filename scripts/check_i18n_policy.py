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
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

RULE_BILINGUAL = "FORBIDDEN_BILINGUAL_LITERAL"
RULE_RAW_LOCALE = "FORBIDDEN_RAW_LOCALE_PARSE"
IGNORE_MARK = "i18n-lint:ignore"

CJK_RE = re.compile(r"[一-鿿]")
EN_WORD_RE = re.compile(r"[A-Za-z]{2,}")
RULE_B_RE = re.compile(
    r'\.Query\(\s*"(?:lang|language|locale)"\s*\)'
    r'|\.GetHeader\(\s*"Accept-Language"\s*\)'
)

GO_EXCLUDE_SUFFIXES = ("_test.go", ".pb.go")
GO_EXCLUDE_DIRS = {"vendor", "proto"}
FRONTEND_EXCLUDE_SUFFIXES = (".test.ts", ".test.tsx", ".d.ts")
FRONTEND_EXCLUDE_DIRS = {
    "__tests__",
    "e2e",
    "locales",
    "prototype",
    ".next",
    "dist-standalone",
    "node_modules",
}


@dataclass
class Violation:
    path: str
    line: int
    rule: str
    detail: str

    def render(self) -> str:
        return f"{self.path}:{self.line}: {self.rule}: {self.detail}"


def _strip_interpolation(text: str) -> str:
    out = []
    i, n = 0, len(text)
    while i < n:
        if text[i] == "$" and i + 1 < n and text[i + 1] == "{":
            depth, j = 0, i + 1
            while j < n:
                if text[j] == "{":
                    depth += 1
                elif text[j] == "}":
                    depth -= 1
                    if depth == 0:
                        j += 1
                        break
                j += 1
            out.append(" ")
            i = j
            continue
        out.append(text[i])
        i += 1
    return "".join(out)


def iter_literals(text: str, allow_single_quote: bool):
    i, n, line = 0, len(text), 1
    while i < n:
        ch = text[i]
        if ch == "\n":
            line += 1
            i += 1
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "/":
            j = text.find("\n", i)
            i = n if j < 0 else j
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "*":
            j = text.find("*/", i + 2)
            end = n if j < 0 else j + 2
            line += text.count("\n", i, end)
            i = end
            continue
        if ch in ('"', "`") or (allow_single_quote and ch == "'"):
            quote = ch
            start_line = line
            i += 1
            buf = []
            while i < n:
                c = text[i]
                if c == "\\" and quote != "`":
                    buf.append(" ")
                    i += 1
                    if i < n:
                        buf.append(" ")
                        i += 1
                    continue
                if quote == "`" and c == "$" and i + 1 < n and text[i + 1] == "{":
                    depth, j = 0, i + 1
                    while j < n:
                        if text[j] == "{":
                            depth += 1
                        elif text[j] == "}":
                            depth -= 1
                            if depth == 0:
                                j += 1
                                break
                        j += 1
                    line += text.count("\n", i, min(j, n))
                    buf.append(" ")
                    i = j
                    continue
                if c == quote:
                    i += 1
                    break
                if c == "\n":
                    if quote == "`":
                        line += 1
                        buf.append(c)
                        i += 1
                        continue
                    break
                buf.append(c)
                i += 1
            yield start_line, "".join(buf)
            continue
        i += 1


def _is_zh_segment(segment: str) -> bool:
    return (
        len(CJK_RE.findall(segment)) >= 2
        and len(EN_WORD_RE.findall(segment)) <= 1
    )


def _is_en_segment(segment: str) -> bool:
    return CJK_RE.search(segment) is None and len(EN_WORD_RE.findall(segment)) >= 2


def _paren_depth(text: str, pos: int) -> int:
    depth = 0
    for ch in text[:pos]:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth = max(0, depth - 1)
    return depth


def bilingual_slash_positions(literal: str):
    cleaned = _strip_interpolation(literal)
    if " / " not in cleaned:
        return []
    parts = cleaned.split(" / ")
    hits = []
    offset = 0
    for idx in range(len(parts) - 1):
        left_seg = parts[idx]
        right_seg = parts[idx + 1]
        slash_pos = offset + len(left_seg)
        offset = slash_pos + 3
        if _paren_depth(cleaned, slash_pos) > 0:
            continue
        if (_is_zh_segment(left_seg) and _is_en_segment(right_seg)) or (
            _is_en_segment(left_seg) and _is_zh_segment(right_seg)
        ):
            hits.append(slash_pos)
    return hits


def line_has_ignore(source_lines, line_no: int) -> bool:
    if 1 <= line_no <= len(source_lines):
        return IGNORE_MARK in source_lines[line_no - 1]
    return False


def scan_go_source(path: str, text: str) -> list[Violation]:
    violations = []
    source_lines = text.splitlines()
    in_apps = path.startswith("internal/apps/")
    for line_no, raw_line in enumerate(source_lines, start=1):
        if in_apps and not line_has_ignore(source_lines, line_no):
            for m in RULE_B_RE.finditer(raw_line):
                violations.append(
                    Violation(path, line_no, RULE_RAW_LOCALE, m.group(0))
                )
    for start_line, literal in iter_literals(text, allow_single_quote=False):
        for pos in bilingual_slash_positions(literal):
            line_no = start_line + literal.count("\n", 0, pos)
            if line_has_ignore(source_lines, line_no) or line_has_ignore(
                source_lines, start_line
            ):
                continue
            violations.append(
                Violation(
                    path,
                    line_no,
                    RULE_BILINGUAL,
                    literal.splitlines()[0][:80],
                )
            )
            break
    return violations


def scan_ts_source(path: str, text: str) -> list[Violation]:
    violations = []
    source_lines = text.splitlines()
    for start_line, literal in iter_literals(text, allow_single_quote=True):
        for pos in bilingual_slash_positions(literal):
            line_no = start_line + literal.count("\n", 0, pos)
            if line_has_ignore(source_lines, line_no) or line_has_ignore(
                source_lines, start_line
            ):
                continue
            violations.append(
                Violation(
                    path,
                    line_no,
                    RULE_BILINGUAL,
                    literal.splitlines()[0][:80],
                )
            )
            break
    return violations


def _is_go_file(path: str) -> bool:
    if not path.endswith(".go") or path.endswith(GO_EXCLUDE_SUFFIXES):
        return False
    parts = path.split("/")
    return path.startswith(("internal/", "agent/")) and not (
        GO_EXCLUDE_DIRS & set(parts)
    )


def _is_frontend_file(path: str) -> bool:
    if not path.endswith((".ts", ".tsx")) or path.endswith(
        FRONTEND_EXCLUDE_SUFFIXES
    ):
        return False
    if not path.startswith("frontend/"):
        return False
    return not (FRONTEND_EXCLUDE_DIRS & set(path.split("/")))


def scan_file(repo_root: Path, rel_path: str) -> list[Violation]:
    text = (repo_root / rel_path).read_text(encoding="utf-8", errors="replace")
    if _is_go_file(rel_path):
        return scan_go_source(rel_path, text)
    if _is_frontend_file(rel_path):
        return scan_ts_source(rel_path, text)
    return []


def scan_line(rel_path: str, line_text: str, line_no: int) -> list[Violation]:
    if IGNORE_MARK in line_text:
        return []
    violations = []
    if _is_go_file(rel_path) and rel_path.startswith("internal/apps/"):
        for m in RULE_B_RE.finditer(line_text):
            violations.append(
                Violation(rel_path, line_no, RULE_RAW_LOCALE, m.group(0))
            )
    if _is_go_file(rel_path) or _is_frontend_file(rel_path):
        for _, literal in iter_literals(
            line_text, allow_single_quote=_is_frontend_file(rel_path)
        ):
            if bilingual_slash_positions(literal):
                violations.append(
                    Violation(
                        rel_path,
                        line_no,
                        RULE_BILINGUAL,
                        literal[:80],
                    )
                )
                break
    return violations


def scan_diff(repo_root: Path, base_ref: str, head_ref: str) -> list[Violation]:
    proc = subprocess.run(
        ["git", "diff", "--unified=0", "--no-color", base_ref, head_ref],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=True,
    )
    violations = []
    current_path = None
    new_line_no = 0
    for raw in proc.stdout.splitlines():
        if raw.startswith("+++ b/"):
            current_path = raw[len("+++ b/") :]
            continue
        if raw.startswith("+++"):
            current_path = None
            continue
        m = re.match(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@", raw)
        if m:
            new_line_no = int(m.group(1)) - 1
            continue
        if current_path is None:
            continue
        if raw.startswith("+"):
            new_line_no += 1
            violations.extend(
                scan_line(current_path, raw[1:], new_line_no)
            )
        elif raw.startswith("-"):
            continue
        else:
            new_line_no += 1
    return violations


def iter_scannable_files(repo_root: Path):
    for base in ("internal", "agent", "frontend"):
        root = repo_root / base
        if not root.is_dir():
            continue
        for dirpath, dirnames, filenames in os.walk(root):
            rel_dir = os.path.relpath(dirpath, repo_root)
            parts = set(Path(rel_dir).parts)
            dirnames[:] = [
                d
                for d in dirnames
                if d not in GO_EXCLUDE_DIRS | FRONTEND_EXCLUDE_DIRS
            ]
            for name in filenames:
                rel = str(Path(rel_dir) / name).replace(os.sep, "/")
                if _is_go_file(rel) or _is_frontend_file(rel):
                    yield rel


def full_scan(repo_root: Path) -> dict[str, list[Violation]]:
    results = {}
    for rel in sorted(iter_scannable_files(repo_root)):
        violations = scan_file(repo_root, rel)
        if violations:
            results[rel] = violations
    return results


def load_baseline(path: Path) -> dict[str, int]:
    data = json.loads(path.read_text(encoding="utf-8"))
    return {str(k): int(v) for k, v in data.get("files", {}).items()}


def check_baseline(
    scan: dict[str, list[Violation]], baseline: dict[str, int]
) -> tuple[list[str], list[str]]:
    errors, hints = [], []
    files = sorted(set(scan) | set(baseline))
    for rel in files:
        actual = len(scan.get(rel, []))
        expected = baseline.get(rel, 0)
        if actual > expected:
            first = scan[rel][0].render()
            errors.append(
                f"{rel}: 违规数 {actual} > 基线 {expected}（首个：{first}）"
            )
        elif actual < expected:
            hints.append(
                f"{rel}: 违规数 {actual} < 基线 {expected}，可运行 --update-baseline 收紧"
            )
    return errors, hints


def write_baseline(path: Path, scan: dict[str, list[Violation]]) -> None:
    payload = {
        "version": 1,
        "files": {rel: len(v) for rel, v in sorted(scan.items())},
    }
    path.write_text(
        json.dumps(payload, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )


def repo_root() -> Path:
    proc = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        capture_output=True,
        text=True,
        check=True,
    )
    return Path(proc.stdout.strip())


def main() -> int:
    parser = argparse.ArgumentParser(
        description="i18n 治理硬规则检查：规则 A 禁止双语拼接字面量，规则 B 禁止 handler 手写语言解析。"
    )
    parser.add_argument("--base-ref", help="diff 模式基线 ref（如 origin/main）")
    parser.add_argument("--head-ref", default="HEAD", help="diff 模式头部 ref")
    parser.add_argument("--baseline", help="基线棘轮校验：scripts/i18n_baseline.json")
    parser.add_argument("--update-baseline", help="按当前扫描结果重写基线 JSON")
    args = parser.parse_args()

    if not (args.base_ref or args.baseline or args.update_baseline):
        parser.error("需指定 --base-ref/--head-ref、--baseline 或 --update-baseline")

    root = repo_root()
    failures = 0

    if args.base_ref:
        violations = scan_diff(root, args.base_ref, args.head_ref)
        for v in violations:
            print(v.render())
        if violations:
            print(f"diff 门禁：发现 {len(violations)} 处违规")
            failures += len(violations)

    if args.update_baseline:
        scan = full_scan(root)
        write_baseline(Path(args.update_baseline), scan)
        print(f"已写入基线 {args.update_baseline}（{len(scan)} 个文件）")
        return 0

    if args.baseline:
        scan = full_scan(root)
        baseline = load_baseline(Path(args.baseline))
        errors, hints = check_baseline(scan, baseline)
        for hint in hints:
            print(f"提示：{hint}")
        for err in errors:
            print(f"违规：{err}")
        if errors:
            failures += len(errors)
        else:
            print("baseline 校验通过")

    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
