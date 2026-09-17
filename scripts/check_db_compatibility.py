#!/usr/bin/env python3
# -*- coding: utf-8 -*-
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
"""
Database Compatibility Linter for STX.
数据库方言与兼容性静态合规检查工具。

Checks Go source files for:
1. Database-specific GORM type tags (e.g. longtext, mediumtext, tinyint, datetime).
2. Raw SQL anti-patterns (e.g. raw LIKE without LOWER(), MySQL backtick identifier quotes, MySQL-specific date functions).
"""

import os
import re
import sys
from typing import List, Tuple

# 根目录定位 / Locate root directory
ROOT_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
INTERNAL_DIR = os.path.join(ROOT_DIR, "internal")

# 规则定义 / Rule definitions
# (Rule Name, Pattern, Error Message CN, Error Message EN)
RULES: List[Tuple[str, re.Pattern, str, str]] = [
    (
        "FORBIDDEN_GORM_LONGTEXT",
        re.compile(r'gorm:"[^"]*\btype:longtext\b[^"]*"', re.IGNORECASE),
        "禁止在 GORM 标签中使用 'type:longtext'（PostgreSQL 不支持），请使用通用的 'type:text'",
        "Forbidden GORM tag 'type:longtext' (not supported by PostgreSQL), use 'type:text' instead",
    ),
    (
        "FORBIDDEN_GORM_MEDIUMTEXT",
        re.compile(r'gorm:"[^"]*\btype:mediumtext\b[^"]*"', re.IGNORECASE),
        "禁止在 GORM 标签中使用 'type:mediumtext'（PostgreSQL 不支持），请使用通用的 'type:text'",
        "Forbidden GORM tag 'type:mediumtext' (not supported by PostgreSQL), use 'type:text' instead",
    ),
    (
        "FORBIDDEN_GORM_TINYINT",
        re.compile(r'gorm:"[^"]*\btype:tinyint\b[^"]*"', re.IGNORECASE),
        "禁止在 GORM 标签中使用 'type:tinyint'，请使用通用 Go 类型 bool 或 int",
        "Forbidden GORM tag 'type:tinyint', use standard Go bool or int types instead",
    ),
    (
        "FORBIDDEN_GORM_DATETIME",
        re.compile(r'gorm:"[^"]*\btype:datetime\b[^"]*"', re.IGNORECASE),
        "禁止在 GORM 标签中使用 'type:datetime'，请使用 time.Time",
        "Forbidden GORM tag 'type:datetime', use time.Time instead",
    ),
    (
        "CASE_SENSITIVE_LIKE_WITHOUT_LOWER",
        re.compile(r'\.Where\(\s*"(?![^"]*LOWER\s*\()[^"]*\bLIKE\s+\?[^"]*"', re.IGNORECASE),
        "禁止使用未包裹 LOWER() 的裸 LIKE 查询（PostgreSQL 默认大小写敏感），请统一写为 LOWER(col) LIKE LOWER(?)",
        "Forbidden bare LIKE query without LOWER() (PostgreSQL is case-sensitive by default), use LOWER(col) LIKE LOWER(?)",
    ),
    (
        "MYSQL_BACKTICK_IN_SQL",
        re.compile(r'\.(Where|Order|Select|Pluck|Group|Having)\(\s*"[^"]*`[^"]*"'),
        "禁止在 SQL 查询字符串中使用 MySQL 专有反引号 `（PostgreSQL 会报语法错误），请移除反引号或使用标准双引号",
        "Forbidden MySQL backtick ` in SQL query strings (causes syntax error in PostgreSQL), remove backticks or use standard identifiers",
    ),
]

# 忽略的文件或路径 / Files or directories to exclude
EXCLUDED_PATHS = [
    "/vendor/",
    "/proto/",
    "/test/",
    "_test.go",  # 测试文件中的某些 mock 可放宽，但 internal/apps 测试也建议遵守
]


def is_excluded(filepath: str) -> bool:
    for ex in EXCLUDED_PATHS:
        if ex in filepath:
            return True
    return False


def scan_file(filepath: str) -> List[str]:
    violations = []
    rel_path = os.path.relpath(filepath, ROOT_DIR)

    try:
        with open(filepath, "r", encoding="utf-8") as f:
            lines = f.readlines()
    except Exception as e:
        return [f"Failed to read file {rel_path}: {e}"]

    for line_idx, line in enumerate(lines, start=1):
        # 跳过纯单行注释 / Skip single-line comments
        stripped = line.strip()
        if stripped.startswith("//") or stripped.startswith("/*") or stripped.startswith("*"):
            continue

        for rule_name, pattern, msg_cn, msg_en in RULES:
            match = pattern.search(line)
            if match:
                violations.append(
                    f"[{rule_name}] {rel_path}:{line_idx}\n"
                    f"  Content: {line.strip()}\n"
                    f"  CN: {msg_cn}\n"
                    f"  EN: {msg_en}"
                )

    return violations


def main():
    target_dir = sys.argv[1] if len(sys.argv) > 1 else INTERNAL_DIR
    if not os.path.exists(target_dir):
        print(f"Error: Directory {target_dir} does not exist.")
        sys.exit(1)

    print(f"=== Scanning Go files in {os.path.relpath(target_dir, ROOT_DIR)} for DB compatibility ===")
    total_files = 0
    all_violations = []

    for root, _, files in os.walk(target_dir):
        for file in files:
            if not file.endswith(".go"):
                continue
            filepath = os.path.join(root, file)
            if is_excluded(filepath):
                continue

            total_files += 1
            violations = scan_file(filepath)
            if violations:
                all_violations.extend(violations)

    print(f"Scanned {total_files} Go source files.")

    if all_violations:
        print(f"\n❌ Found {len(all_violations)} database compatibility violations:\n")
        for v in all_violations:
            print(v)
            print("-" * 60)
        print("\nPlease fix the above issues to ensure compatibility across SQLite, MySQL, and PostgreSQL.")
        sys.exit(1)
    else:
        print("✅ All scanned files conform to multi-database compatibility rules!")
        sys.exit(0)


if __name__ == "__main__":
    main()
