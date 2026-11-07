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

"""离线正/负例：MIT 完整性与白名单策略，不联网。
Offline positive/negative cases for MIT completeness and allowlist policy (no network).
"""

from __future__ import annotations

import os
import tempfile
import unittest
from contextlib import contextmanager
from pathlib import Path

from check_license import (
    Change,
    evaluate_changes,
    header_license_kind,
    is_complete_mit_header,
    load_allowlist,
    validate_allowlist_entries,
)

COMPLETE_MIT = """\
/*
 * MIT License
 *
 * Copyright (c) 2026 Elia Kuratli
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */
"""

TITLE_ONLY_MIT = "/*\n * MIT License\n */\n"

NO_LICENSE = "export const x = 1;\n"

FAKE_APACHE = """\
/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.
 */
"""


class MitHeaderDetectionTest(unittest.TestCase):
    def test_complete_mit_allowed(self) -> None:
        self.assertTrue(is_complete_mit_header(COMPLETE_MIT))
        self.assertEqual(header_license_kind(COMPLETE_MIT), "mit")

    def test_title_only_mit_rejected(self) -> None:
        self.assertFalse(is_complete_mit_header(TITLE_ONLY_MIT))
        self.assertEqual(header_license_kind(TITLE_ONLY_MIT), "none")

    def test_missing_license_rejected(self) -> None:
        self.assertFalse(is_complete_mit_header(NO_LICENSE))
        self.assertEqual(header_license_kind(NO_LICENSE), "none")

    def test_copyright_without_permission_rejected(self) -> None:
        header = "/*\n * MIT License\n * Copyright (c) 2026 Someone\n */\n"
        self.assertFalse(is_complete_mit_header(header))
        self.assertEqual(header_license_kind(header), "none")


class AllowlistPolicyTest(unittest.TestCase):
    def setUp(self) -> None:
        self._tmpdir = tempfile.TemporaryDirectory()
        self.root = Path(self._tmpdir.name)
        self.addCleanup(self._tmpdir.cleanup)

    @contextmanager
    def _chdir(self, path: Path):
        prev = Path.cwd()
        os.chdir(path)
        try:
            yield
        finally:
            os.chdir(prev)

    def _write(self, rel: str, content: str) -> str:
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        return str(path)

    def test_allowlisted_file_with_fake_apache_rejected(self) -> None:
        # 清单内 MIT 文件若挂伪 Apache 头必须拒绝。
        # Allowlisted MIT files with a forged Apache header must fail.
        # 使用 cwd 相对路径，避免绝对路径/排除目录干扰策略匹配。
        # Use cwd-relative paths so policy matching mirrors real git diffs.
        rel = "frontend/components/arc/widget.tsx"
        self._write(rel, FAKE_APACHE + "export const w = 1;\n")
        with self._chdir(self.root):
            errors, notices, _ = evaluate_changes(
                [Change(status="A", path=rel)],
                allowlist=set(),
                third_party_mit={rel},
            )
        self.assertTrue(any("must carry an MIT header" in e for e in errors), errors)
        self.assertFalse(notices)

    def test_unlisted_new_file_still_requires_apache(self) -> None:
        # 未列名新增源文件仍要求 Apache，即使带完整 MIT。
        # Unlisted new sources still require Apache even with a complete MIT header.
        rel = "frontend/components/new-thing/index.ts"
        self._write(rel, COMPLETE_MIT + "export const n = 1;\n")
        with self._chdir(self.root):
            errors, notices, _ = evaluate_changes(
                [Change(status="A", path=rel)],
                allowlist=set(),
                third_party_mit=set(),
            )
        self.assertTrue(any("must carry an Apache 2.0 header" in e for e in errors), errors)
        self.assertFalse(notices)

    def test_allowlisted_complete_mit_passes(self) -> None:
        rel = "frontend/components/arc/ok.tsx"
        self._write(rel, COMPLETE_MIT + "export const ok = 1;\n")
        with self._chdir(self.root):
            errors, notices, _ = evaluate_changes(
                [Change(status="A", path=rel)],
                allowlist=set(),
                third_party_mit={rel},
            )
        self.assertEqual(errors, [])
        self.assertTrue(any("MIT OK (third-party new)" in n for n in notices), notices)

    def test_allowlisted_title_only_mit_rejected(self) -> None:
        # 白名单文件仅有 “MIT License” 标题时仍拒绝。
        # Allowlisted files with only an "MIT License" title still fail.
        rel = "frontend/components/arc/title-only.tsx"
        self._write(rel, TITLE_ONLY_MIT + "export const t = 1;\n")
        with self._chdir(self.root):
            errors, _, _ = evaluate_changes(
                [Change(status="A", path=rel)],
                allowlist=set(),
                third_party_mit={rel},
            )
        self.assertTrue(any("must carry an MIT header" in e and "none" in e for e in errors), errors)

    def test_missing_allowlist_path_rejected(self) -> None:
        # 清单条目对应路径不存在时按设计明确拒绝。
        # Missing allowlist paths must fail the check by design.
        missing = str(self.root / "does-not-exist.tsx")
        missing_entries = validate_allowlist_entries({missing})
        self.assertEqual(missing_entries, [missing])

        errors, _, _ = evaluate_changes(
            [],
            allowlist=set(),
            third_party_mit={missing},
            third_party_path=Path("licenses/third_party_mit_files.txt"),
        )
        self.assertTrue(
            any("third-party MIT allowlist contains missing paths" in e for e in errors),
            errors,
        )

    def test_missing_allowlist_file_raises(self) -> None:
        # 清单文件本身不存在时 load_allowlist 明确失败。
        # load_allowlist must fail clearly when the allowlist file is absent.
        with self.assertRaises(Exception) as ctx:
            load_allowlist(self.root / "no-such-allowlist.txt")
        self.assertIn("allowlist not found", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
