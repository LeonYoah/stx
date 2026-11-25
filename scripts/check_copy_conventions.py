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

"""复制规范硬门禁：违规即失败，无基线。
Copy conventions hard gate: any violation fails the build; no baseline.

规则 Rules:
1. 禁止裸调 navigator.clipboard / execCommand 复制，唯一实现点在 use-copy-feedback.ts。
   Raw navigator.clipboard / execCommand copy is forbidden outside use-copy-feedback.ts.
2. 禁止 JSX 传 copiedLabel= / failedLabel=：反馈文案由 CopyButton 内部 i18n 统一。
   JSX copiedLabel=/failedLabel= props are forbidden; CopyButton owns feedback labels via i18n.
"""
import re
import sys
from pathlib import Path

FRONTEND = Path("frontend")
CLIPBOARD_LIB = "components/arc/lib/use-copy-feedback.ts"
IGNORE_MARK = "copy-lint:ignore"

RAW_CLIPBOARD_RE = re.compile(r"navigator\.clipboard|execCommand\(\s*['\"]copy['\"]")
JSX_LABEL_RE = re.compile(r"\b(?:copiedLabel|failedLabel)=\{")

EXCLUDE_DIRS = {"node_modules", ".next", "prototype", "e2e"}


def iter_frontend_files():
    for suffix in ("*.ts", "*.tsx"):
        for path in sorted(FRONTEND.rglob(suffix)):
            if any(part in EXCLUDE_DIRS for part in path.parts):
                continue
            yield path


def scan_file(path):
    rel = path.relative_to(FRONTEND).as_posix()
    is_clipboard_lib = rel == CLIPBOARD_LIB
    violations = []
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except (UnicodeDecodeError, OSError):
        return violations
    for lineno, line in enumerate(lines, 1):
        if IGNORE_MARK in line:
            continue
        if not is_clipboard_lib and RAW_CLIPBOARD_RE.search(line):
            violations.append((rel, lineno, "RAW_CLIPBOARD", line.strip()))
        if JSX_LABEL_RE.search(line):
            violations.append((rel, lineno, "JSX_FEEDBACK_LABEL", line.strip()))
    return violations


def main():
    all_violations = []
    for path in iter_frontend_files():
        all_violations.extend(scan_file(path))
    if not all_violations:
        print("copy conventions: OK, 0 violations")
        return 0
    print(f"copy conventions: {len(all_violations)} violation(s):")
    for rel, lineno, rule, snippet in all_violations:
        print(f"  {rel}:{lineno} [{rule}] {snippet[:100]}")
    print("修复方式：复制统一走 writeClipboard / Arc CopyButton；反馈文案由组件内部 i18n 控制，不要传 copiedLabel/failedLabel。")
    return 1


if __name__ == "__main__":
    sys.exit(main())
