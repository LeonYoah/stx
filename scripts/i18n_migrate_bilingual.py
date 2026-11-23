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
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import check_i18n_policy as pol

LOCALE_IMPORT = "github.com/LeonYoah/stx/internal/pkg/locale"
FMT_IMPORT = "fmt"

LOG_CALL_RE = re.compile(
    r"\b(?:s?log|logger|zap|sugar)\w*\.\w*(?:Debug|Info|Warn|Error|Fatal|Print|Panic|Log)\w*\s*\("
    r"|\.\w*(?:Debugf|Infof|Warnf|Errorf|Fatalf|Printf|Println|Panicf|Logf)\s*\("
    r"|\bfmt\.Print\w*\s*\("
    r"|\.Logger\s*\("
)

FUNC_SIG_RE = re.compile(r"\nfunc\s+(?:\([^)]*\)\s*)?(\w+)\s*\(")
GIN_CTX_RE = re.compile(r"(\w+)\s*\*?gin\.Context")
STD_CTX_RE = re.compile(r"(\w+)\s+context\.Context")
LOCAL_CTX_RE = re.compile(
    r"(?:^|\n)\s*(\w+)\s*:=\s*(?:\w+\.Request\.Context\(\)|context\.Background\(\)|context\.TODO\(\))"
)
VERB_RE = re.compile(r"%[-+#0 .]*(?:\*|\d+)?(?:\.\d+)?[bcdoOqxXUeEfFgGsqvtxTwWp]")


def verb_count(segment: str) -> int:
    return len(VERB_RE.findall(segment))


def split_literal(raw: str):
    parts = raw.split(" / ")
    zh = [p for p in parts if pol.CJK_RE.search(p)]
    en = [p for p in parts if not pol.CJK_RE.search(p)]
    if not zh or not en:
        return None
    return " / ".join(zh), " / ".join(en)


def paren_pairs(text: str) -> dict:
    stack = []
    pairs = {}
    i, n = 0, len(text)
    while i < n:
        c = text[i]
        if c == "/" and i + 1 < n and text[i + 1] == "/":
            j = text.find("\n", i)
            i = n if j < 0 else j
            continue
        if c == "/" and i + 1 < n and text[i + 1] == "*":
            j = text.find("*/", i + 2)
            i = n if j < 0 else j + 2
            continue
        if c in '"`':
            q = c
            i += 1
            while i < n and text[i] != q:
                if text[i] == "\\" and q != "`":
                    i += 1
                i += 1
            i += 1
            continue
        if c == "'":
            i += 1
            while i < n and text[i] != "'":
                if text[i] == "\\":
                    i += 1
                i += 1
            i += 1
            continue
        if c in "([{":
            stack.append((c, i))
        elif c in ")]}":
            want = {')': '(', ']': '[', '}': '{'}[c]
            if stack and stack[-1][0] == want:
                op, oi = stack.pop()
                if c == ")":
                    pairs[oi] = i
            else:
                stack.clear()
        i += 1
    return pairs


def split_top_level_args(text: str, start: int, end: int):
    args = []
    depth = 0
    seg_start = start
    i = start
    while i < end:
        c = text[i]
        if c in '"`':
            q = c
            i += 1
            while i < end and text[i] != q:
                if text[i] == "\\" and q != "`":
                    i += 1
                i += 1
            i += 1
            continue
        if c == "'":
            i += 1
            while i < end and text[i] != "'":
                if text[i] == "\\":
                    i += 1
                i += 1
            i += 1
            continue
        if c in "([{":
            depth += 1
        elif c in ")]}":
            depth -= 1
        elif c == "," and depth == 0:
            args.append((seg_start, i))
            seg_start = i + 1
        i += 1
    if text[seg_start:end].strip():
        args.append((seg_start, end))
    return args


def find_ctx_expr(text: str, pos: int):
    func_match = None
    for m in FUNC_SIG_RE.finditer(text):
        if m.start() < pos:
            func_match = m
        else:
            break
    if func_match is None:
        return None
    brace = text.find("{", func_match.end())
    sig_end = brace if 0 < brace <= pos else pos
    sig = text[func_match.start() : sig_end]
    m = GIN_CTX_RE.search(sig)
    if m and m.group(1) != "_":
        return f"{m.group(1)}.Request.Context()"
    m = STD_CTX_RE.search(sig)
    if m and m.group(1) != "_":
        return m.group(1)
    body = text[sig_end:pos]
    for m in LOCAL_CTX_RE.finditer(body):
        if m.group(1) != "_":
            return m.group(1)
    return None


def is_log_context(text: str, pos: int) -> bool:
    line_start = text.rfind("\n", 0, pos) + 1
    window_start = line_start
    for _ in range(4):
        prev = text.rfind("\n", 0, max(0, window_start - 1)) + 1
        if prev == window_start:
            break
        window_start = prev
    return bool(LOG_CALL_RE.search(text[window_start:pos]))


def quote_like(original: str, interior: str) -> str:
    if original.startswith("`"):
        return "`" + interior + "`"
    return '"' + interior.replace('"', '\\"') + '"'


def ensure_import(text: str, path: str) -> str:
    quoted = f'"{path}"'
    if quoted in text:
        return text
    block = re.search(r"import\s*\((.*?)\)", text, re.S)
    if block:
        lines = block.group(1).splitlines()
        entry = f"\t{quoted}"
        insert_at = len(lines)
        for i, line in enumerate(lines):
            stripped = line.strip()
            if stripped.startswith('"') and stripped.strip('"') > path:
                insert_at = i
                break
        lines.insert(insert_at, entry)
        return text[: block.start(1)] + "\n".join(lines) + text[block.end(1) :]
    m = re.search(r'import\s+"([^"]+)"', text)
    if m:
        existing = m.group(1)
        pair = sorted([existing, path])
        block = 'import (\n\t"%s"\n\t"%s"\n)' % (pair[0], pair[1])
        return text[: m.start()] + block + text[m.end() :]
    pkg = re.search(r"^package \w+\n", text, re.M)
    if pkg:
        return text[: pkg.end()] + f"\nimport {quoted}\n" + text[pkg.end() :]
    return text


def migrate_file(repo_root: Path, rel: str, use_locale_pick: bool, dry_run: bool):
    path = repo_root / rel
    text = path.read_text(encoding="utf-8")
    source_lines = text.splitlines()
    pairs = paren_pairs(text)
    edits = []
    used_pick = False
    used_sprintf = False
    for start_line, literal, s, e in pol.iter_literals(
        text, allow_single_quote=False
    ):
        if not pol.bilingual_slash_positions(literal):
            continue
        if pol.line_has_ignore(source_lines, start_line):
            continue
        raw = text[s + 1 : e - 1]
        pair = split_literal(raw)
        if pair is None:
            print(f"SKIP 无法拆分 {rel}:{start_line} {raw[:60]}")
            continue
        zh_raw, en_raw = pair
        original = text[s:e]
        parts = raw.split(" / ")
        zh_verbs = sum(verb_count(p) for p in parts if pol.CJK_RE.search(p))
        en_verbs = sum(verb_count(p) for p in parts if not pol.CJK_RE.search(p))
        has_w = "%w" in raw
        ctx_expr = None
        if use_locale_pick and not is_log_context(text, s) and not has_w:
            ctx_expr = find_ctx_expr(text, s)

        if zh_verbs == 0 and en_verbs == 0:
            if ctx_expr:
                replacement = (
                    f"locale.Pick({ctx_expr}, "
                    f"{quote_like(original, zh_raw)}, {quote_like(original, en_raw)})"
                )
                used_pick = True
            else:
                replacement = quote_like(original, en_raw)
            edits.append((s, e, replacement))
            continue

        open_paren = max(
            (o for o, c in pairs.items() if o < s < c), default=None
        )
        if open_paren is None:
            print(f"SKIP 动词字面量不在调用内 {rel}:{start_line} {raw[:60]}")
            continue
        close_paren = pairs[open_paren]
        args = split_top_level_args(text, open_paren + 1, close_paren)
        arg_idx = next(
            (i for i, (a, b) in enumerate(args) if a <= s < b), None
        )
        if arg_idx is None:
            print(f"SKIP 无法定位实参 {rel}:{start_line} {raw[:60]}")
            continue
        trailing = args[arg_idx + 1 :]
        cursor = 0
        zh_args, en_args = [], []
        wrap_arg = None
        for p in parts:
            cnt = verb_count(p)
            seg_args = trailing[cursor : cursor + cnt]
            if "%w" in p and seg_args:
                verbs = VERB_RE.findall(p)
                wpos = verbs.index("%w")
                if wpos < len(seg_args):
                    wrap_arg = seg_args[wpos]
            if pol.CJK_RE.search(p):
                zh_args += seg_args
            else:
                en_args += seg_args
            cursor += cnt
        if cursor != len(trailing):
            print(
                f"SKIP 实参数不匹配 {rel}:{start_line} {raw[:60]}"
                f"（需要 {cursor}，实际 {len(trailing)}）"
            )
            continue

        lit_arg_start, _ = args[arg_idx]
        if has_w or ctx_expr is None:
            final_en = en_raw
            final_en_args = list(en_args)
            if has_w and "%w" not in final_en:
                final_en = final_en + ": %w"
                if wrap_arg is not None:
                    final_en_args.append(wrap_arg)
            en_arg_text = "".join(
                ", " + text[a:b].strip() for a, b in final_en_args
            )
            new_inside = (
                text[open_paren + 1 : lit_arg_start]
                + quote_like(original, final_en)
                + en_arg_text
            )
            edits.append((open_paren + 1, close_paren, new_inside))
        else:
            def side_expr(side_raw, side_args):
                if not side_args:
                    return quote_like(original, side_raw)
                return (
                    f"fmt.Sprintf({quote_like(original, side_raw)}, "
                    + ", ".join(text[a:b].strip() for a, b in side_args)
                    + ")"
                )

            zh_expr = side_expr(zh_raw, zh_args)
            en_expr = side_expr(en_raw, en_args)
            replacement = f"locale.Pick({ctx_expr}, {zh_expr}, {en_expr})"
            new_inside = (
                text[open_paren + 1 : lit_arg_start] + replacement
            )
            edits.append((open_paren + 1, close_paren, new_inside))
            used_pick = True
            if zh_args or en_args:
                used_sprintf = True

    if not edits:
        return 0
    for s, e, replacement in reversed(edits):
        text = text[:s] + replacement + text[e:]
    if used_pick:
        text = ensure_import(text, LOCALE_IMPORT)
    if used_sprintf:
        text = ensure_import(text, FMT_IMPORT)
    if not dry_run:
        path.write_text(text, encoding="utf-8")
    return len(edits)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="按规则 A 扫描结果机械改写双语拼接字面量：agent/ 与日志调用取英文，ctx 可达处改 locale.Pick，%w 保留英文。"
    )
    parser.add_argument("prefixes", nargs="+", help="相对仓库根的文件或目录前缀")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument(
        "--english-only",
        action="store_true",
        help="全部改写为英文单语（agent/ 固定如此）",
    )
    args = parser.parse_args()

    root = pol.repo_root()
    total = 0
    for prefix in args.prefixes:
        targets = [
            rel
            for rel in pol.iter_scannable_files(root)
            if rel == prefix or rel.startswith(prefix.rstrip("/") + "/")
        ]
        if (root / prefix).is_file():
            targets = [prefix]
        for rel in targets:
            pick_ok = not args.english_only and not rel.startswith("agent/")
            n = migrate_file(root, rel, pick_ok, args.dry_run)
            if n:
                total += n
                print(f"{rel}: 改写 {n} 处")
    print(f"共改写 {total} 处")
    return 0


if __name__ == "__main__":
    sys.exit(main())
