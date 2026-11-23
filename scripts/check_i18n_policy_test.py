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

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import check_i18n_policy as pol


class TestBilingualRule(unittest.TestCase):
    def assertHit(self, literal: str):
        self.assertTrue(
            pol.bilingual_slash_positions(literal),
            f"expected hit: {literal}",
        )

    def assertPass(self, literal: str):
        self.assertFalse(
            pol.bilingual_slash_positions(literal),
            f"expected pass: {literal}",
        )

    def test_zh_then_en(self):
        self.assertHit("分片上传未完成，请重试 / Chunk upload did not complete")
        self.assertHit("下载卸载包 / Download package")

    def test_en_then_zh(self):
        self.assertHit("Download package / 下载卸载包")

    def test_tech_acronym_pair(self):
        self.assertPass("增量日志流 (CDC / Log Stream)")

    def test_tech_identifier_pair(self):
        self.assertPass("JDK 24+ 移除 Subject.getSubject / JEP 486")

    def test_same_language_enum(self):
        self.assertPass("严重 %d / 告警 %d / 信息 %d")
        self.assertPass("关联集群 / 告警上下文")

    def test_template_interpolation(self):
        self.assertPass(" 分，基线 ${x / 60} 秒")

    def test_progress_counter(self):
        self.assertPass("已完成 {done} / {total}")

    def test_single_word_en_side(self):
        self.assertHit("主机名 / IP")
        self.assertHit("Downloaded / 已下载")

    def test_version_required_sentence(self):
        self.assertHit("Java 17 required / 需要 Java 17")

    def test_path_list_inside_zh(self):
        self.assertPass("插件位于 connector / lib / plugins 目录下")

    def test_literal_brace_block_not_placeholder(self):
        self.assertHit('{"error": "Invalid version / 版本无效"}')

    def test_go_template_blocks_stripped(self):
        self.assertPass('{{ $labels.cluster_name }} / {{ $labels.instance }} 触发指标告警')
        self.assertPass('{{pair "打印 / 导出 PDF" "Print / Export PDF"}}')

    def test_lowercase_translation_not_path(self):
        self.assertHit("已连接 localhost / connected")

    def test_lead_word_version_sentence(self):
        self.assertHit("Requires Java 17 / 需要 Java 17")

    def test_two_path_list_passes(self):
        self.assertPass("可选目录 /opt/seatunnel / /opt/stx")

    def test_versioned_name_list_passes(self):
        self.assertPass("支持的运行时：OpenJDK 17 / Oracle JDK 21")

    def test_adjacent_line_ignore_does_not_leak(self):
        src = 'x := "ok" // i18n-lint:ignore\ny := "中文 / English"\n'
        hits = pol.scan_go_source("internal/apps/x/a.go", src)
        self.assertEqual(len(hits), 1)

    def test_standalone_comment_ignore_exempts_next_literal(self):
        src = '// i18n-lint:ignore\nconst s = `#!/bin/bash\nSTX Agent / 启动包装\n`\n'
        hits = pol.scan_go_source("internal/apps/x/a.go", src)
        self.assertEqual(len(hits), 0)


class TestIterLiterals(unittest.TestCase):
    def test_skips_comments(self):
        src = '// "中文注释 / English comment"\ns := "clean"\n/* "注释两块 / Block two" */'
        lits = [lit for _, lit, _, _ in pol.iter_literals(src, allow_single_quote=False)]
        self.assertEqual(lits, ["clean"])

    def test_template_strips_holes(self):
        src = "const s = `${Math.floor(a / 60)} 分 ${b} 秒`;"
        lits = [lit for _, lit, _, _ in pol.iter_literals(src, allow_single_quote=True)]
        self.assertEqual(lits, ["  分   秒"])

    def test_go_raw_and_basic(self):
        src = 'a := "x / y"\nb := `raw / str`\nc := \'r\''
        lits = [lit for _, lit, _, _ in pol.iter_literals(src, allow_single_quote=False)]
        self.assertEqual(lits, ["x / y", "raw / str"])


class TestSourceScan(unittest.TestCase):
    def test_go_hit_and_ignore(self):
        src = (
            'package x\n'
            'var a = "上传失败 / Upload failed"\n'
            'var b = "上传失败 / Upload failed" // i18n-lint:ignore test\n'
        )
        vs = pol.scan_go_source("internal/foo/x.go", src)
        self.assertEqual(len(vs), 1)
        self.assertEqual(vs[0].line, 2)
        self.assertEqual(vs[0].rule, pol.RULE_BILINGUAL)

    def test_rule_b_in_apps(self):
        src = (
            'package x\n'
            'lang := c.Query("lang")\n'
            'h := c.GetHeader("Accept-Language")\n'
            'ok := locale.FromContext(c.Request.Context())\n'
        )
        vs = pol.scan_go_source("internal/apps/foo/x.go", src)
        self.assertEqual(len(vs), 2)
        self.assertTrue(all(v.rule == pol.RULE_RAW_LOCALE for v in vs))
        self.assertEqual(pol.scan_go_source("internal/pkg/foo/x.go", src), [])

    def test_rule_b_ignored(self):
        src = 'l := c.Query("lang") // i18n-lint:ignore compat\n'
        self.assertEqual(
            pol.scan_go_source("internal/apps/foo/x.go", src), []
        )

    def test_ts_scan(self):
        src = "throw new Error('上传为空 / Empty response');\n"
        vs = pol.scan_ts_source("frontend/lib/x.ts", src)
        self.assertEqual(len(vs), 1)
        self.assertEqual(vs[0].rule, pol.RULE_BILINGUAL)

    def test_ts_nested_template_expression(self):
        src = 'const m = `x ${ok ? "中文 / English" : "y"}`;\n'
        vs = pol.scan_ts_source("frontend/lib/x.ts", src)
        self.assertEqual(len(vs), 1)

    def test_multiline_interpolation_span(self):
        src = 'const m = `a ${one +\n two +\n three} b 中文 / English`;\n'
        vs = pol.scan_ts_source("frontend/lib/x.ts", src)
        self.assertEqual(len(vs), 1)
        self.assertEqual(vs[0].end, 3)


class TestScanLine(unittest.TestCase):
    def test_added_line_hit(self):
        vs = pol.scan_line(
            "internal/apps/foo/x.go",
            '    return errors.New("上传失败 / Upload failed")',
            10,
        )
        self.assertEqual(len(vs), 1)

    def test_added_line_rule_b(self):
        vs = pol.scan_line(
            "internal/apps/foo/x.go", 'lang := c.Query("lang")', 3
        )
        self.assertEqual(len(vs), 1)
        self.assertEqual(vs[0].rule, pol.RULE_RAW_LOCALE)

    def test_unscanned_path(self):
        self.assertEqual(
            pol.scan_line("docs/readme.md", '失败 / Failed', 1), []
        )
        self.assertEqual(
            pol.scan_line(
                "internal/foo/x_test.go", '失败 / Failed', 1
            ),
            [],
        )


class TestBaseline(unittest.TestCase):
    def _scan(self, path: str, n: int):
        return {
            path: [
                pol.Violation(path, i + 1, pol.RULE_BILINGUAL, "x")
                for i in range(n)
            ]
        }

    def test_increase_blocks(self):
        errors, hints = pol.check_baseline(
            self._scan("a.go", 3), {"a.go": 2}
        )
        self.assertEqual(len(errors), 1)
        self.assertEqual(hints, [])

    def test_decrease_hints_only(self):
        errors, hints = pol.check_baseline(
            self._scan("a.go", 1), {"a.go": 2}
        )
        self.assertEqual(errors, [])
        self.assertEqual(len(hints), 1)

    def test_new_file_blocks(self):
        errors, _ = pol.check_baseline(
            self._scan("new.go", 1), {"a.go": 2}
        )
        self.assertEqual(len(errors), 1)

    def test_equal_passes(self):
        errors, hints = pol.check_baseline(
            self._scan("a.go", 2), {"a.go": 2}
        )
        self.assertEqual(errors, [])
        self.assertEqual(hints, [])


class TestDiffMode(unittest.TestCase):
    def test_scan_diff_end_to_end(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            subprocess.run(["git", "init", "-q"], cwd=root, check=True)
            subprocess.run(
                ["git", "config", "user.email", "t@t"], cwd=root, check=True
            )
            subprocess.run(
                ["git", "config", "user.name", "t"], cwd=root, check=True
            )
            d = root / "internal" / "apps" / "x"
            d.mkdir(parents=True)
            (d / "x.go").write_text("package x\n", encoding="utf-8")
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            subprocess.run(
                ["git", "commit", "-qm", "init"], cwd=root, check=True
            )
            (d / "x.go").write_text(
                'package x\nvar a = "上传失败 / Upload failed"\n'
                'var lang = c.Query("lang")\n',
                encoding="utf-8",
            )
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            subprocess.run(
                ["git", "commit", "-qm", "bad"], cwd=root, check=True
            )
            vs = pol.scan_diff(root, "HEAD~1", "HEAD")
            rules = sorted(v.rule for v in vs)
            self.assertEqual(
                rules, [pol.RULE_BILINGUAL, pol.RULE_RAW_LOCALE]
            )

    def test_scan_diff_reads_head_not_worktree(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            subprocess.run(["git", "init", "-q"], cwd=root, check=True)
            subprocess.run(
                ["git", "config", "user.email", "t@t"], cwd=root, check=True
            )
            subprocess.run(
                ["git", "config", "user.name", "t"], cwd=root, check=True
            )
            d = root / "internal" / "apps" / "x"
            d.mkdir(parents=True)
            (d / "x.go").write_text("package x\n", encoding="utf-8")
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            subprocess.run(
                ["git", "commit", "-qm", "init"], cwd=root, check=True
            )
            (d / "x.go").write_text(
                'package x\nvar a = "上传失败 / Upload failed"\n',
                encoding="utf-8",
            )
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            subprocess.run(
                ["git", "commit", "-qm", "bad"], cwd=root, check=True
            )
            (d / "x.go").write_text(
                'package x\nvar a = "Upload failed"\n',
                encoding="utf-8",
            )
            vs = pol.scan_diff(root, "HEAD~1", "HEAD")
            self.assertEqual(
                [v.rule for v in vs], [pol.RULE_BILINGUAL]
            )


if __name__ == "__main__":
    unittest.main()
