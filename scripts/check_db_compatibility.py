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


def check_composite_indexes(target_dir: str) -> List[str]:
    """
    检查复合索引总字符长度，防止超出 MySQL InnoDB 3072 字节（utf8mb4 下约 768 字符）上限。
    Checks composite index key lengths to prevent exceeding MySQL InnoDB 3072-byte limit (768 chars in utf8mb4).
    """
    from collections import defaultdict

    indexes = defaultdict(lambda: {"cols": [], "total_size": 0, "file": ""})
    gorm_tag_re = re.compile(r'gorm:"([^"]+)"')
    size_re = re.compile(r'size:(\d+)')
    index_re = re.compile(r'(?:uniqueIndex|index):([a-zA-Z0-9_]+)')

    for root, _, files in os.walk(target_dir):
        for file in files:
            if not file.endswith(".go") or file.endswith("_test.go"):
                continue
            filepath = os.path.join(root, file)
            if is_excluded(filepath):
                continue

            with open(filepath, "r", encoding="utf-8") as f:
                for line_idx, line in enumerate(f, start=1):
                    stripped = line.strip()
                    if stripped.startswith("//") or stripped.startswith("/*") or stripped.startswith("*"):
                        continue
                    m_tag = gorm_tag_re.search(line)
                    if not m_tag:
                        continue
                    tag = m_tag.group(1)
                    m_idx = index_re.search(tag)
                    if m_idx:
                        idx_name = m_idx.group(1)
                        m_sz = size_re.search(tag)
                        sz = int(m_sz.group(1)) if m_sz else 0
                        col = stripped.split()[0]
                        indexes[idx_name]["cols"].append((col, sz, line_idx))
                        indexes[idx_name]["total_size"] += sz
                        indexes[idx_name]["file"] = filepath

    violations = []
    # 750 字符 * 4 字节/字符 = 3000 字节，安全预警阈值（MySQL InnoDB 上限 3072 字节）
    # 750 chars * 4 bytes/char = 3000 bytes, safe threshold (MySQL InnoDB limit 3072 bytes)
    for idx_name, data in indexes.items():
        if len(data["cols"]) > 1 and data["total_size"] > 700:
            rel_file = os.path.relpath(data["file"], ROOT_DIR)
            col_details = ", ".join(f"{c}(size:{s})" for c, s, _ in data["cols"])
            violations.append(
                f"[COMPOSITE_INDEX_TOO_LONG] {rel_file}\n"
                f"  Index: {idx_name}\n"
                f"  Total VARCHAR size: {data['total_size']} chars (~{data['total_size']*4} bytes in utf8mb4)\n"
                f"  Columns: {col_details}\n"
                f"  CN: 复合索引总键长超出安全限制（MySQL 3072 字节上限），请缩减字段 size 或移除不必要的索引列\n"
                f"  EN: Composite index key length exceeds MySQL 3072-byte limit; shrink column sizes or remove unneeded columns"
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

    # 检查复合索引长度 / Check composite index lengths
    index_violations = check_composite_indexes(target_dir)
    if index_violations:
        all_violations.extend(index_violations)

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
