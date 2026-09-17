#!/usr/bin/env bash
#
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
#

# 中文：本地多数据库兼容性一键快速验证脚本
# English: One-click local multi-database compatibility fast verification script

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

echo "============================================================"
echo " [STX] Multi-Database Compatibility Fast Verification Gate "
echo "============================================================"

# Step 1: 静态方言与反模式扫描 / Step 1: Static dialect and anti-pattern scan
echo ""
echo ">>> [1/3] Running Static Code & Dialect Compatibility Linter..."
python3 scripts/check_db_compatibility.py

# Step 2: 数据库迁移与幂等性测试 / Step 2: Schema migration & idempotency test
echo ""
echo ">>> [2/3] Running Multi-Database Schema Migration & Seed Tests..."
go test -v ./internal/db/migrator/... -run TestMultiDatabaseMigration -count=1

# Step 3: 核心行为一致性测试 / Step 3: Core behavioral compatibility suite
echo ""
echo ">>> [3/3] Running Cross-Database Behavioral Compatibility Suite..."
go test -v ./internal/db/... -run TestDatabaseCompatibilitySuite -count=1

echo ""
echo "============================================================"
echo " ✅ All Multi-Database Compatibility Verification Passed!  "
echo "============================================================"
