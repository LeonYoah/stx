/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package locale

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestFmtSprintfLocalePickVerbsMatch 静态扫描 internal/ 下全部 fmt.Sprintf(locale.Pick(...), args...)
// 调用，确保中英模板占位符数量一致且与实际传参个数匹配，防止 `%!s(MISSING)` 运行时错位。
// TestFmtSprintfLocalePickVerbsMatch scans all fmt.Sprintf(locale.Pick(...), args...) calls
// under internal/ to ensure Chinese/English format verbs match each other and argument count.
func TestFmtSprintfLocalePickVerbsMatch(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	internalDir := filepath.Join(repoRoot, "internal")

	fset := token.NewFileSet()
	checked := 0
	var failures []string

	err := filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		fileAST, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		rel, _ := filepath.Rel(repoRoot, path)
		ast.Inspect(fileAST, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall || len(call.Args) < 1 || !isSelectorNamed(call.Fun, "fmt", "Sprintf") {
				return true
			}
			pickCall, isPick := call.Args[0].(*ast.CallExpr)
			if !isPick || !isSelectorNamed(pickCall.Fun, "locale", "Pick") || len(pickCall.Args) < 3 {
				return true
			}
			zh, okZh := extractStringLiteral(pickCall.Args[1])
			en, okEn := extractStringLiteral(pickCall.Args[2])
			if !okZh || !okEn {
				return true
			}
			checked++
			zhCount, zhIndexed := countFormatArgs(zh)
			enCount, enIndexed := countFormatArgs(en)
			argCount := len(call.Args) - 1
			pos := fset.Position(call.Pos())
			if zhIndexed || enIndexed {
				if zhCount != enCount || zhCount > argCount {
					failures = append(failures, fmt.Sprintf("%s:%d: indexed verb mismatch zh=%d en=%d args=%d", rel, pos.Line, zhCount, enCount, argCount))
				}
				return true
			}
			if zhCount != enCount || zhCount != argCount {
				failures = append(failures, fmt.Sprintf("%s:%d: verb mismatch zh=%d en=%d args=%d (zh=%q en=%q)", rel, pos.Line, zhCount, enCount, argCount, zh, en))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
	if checked == 0 {
		t.Fatal("expected at least one fmt.Sprintf(locale.Pick(...)) call in internal/")
	}
	if len(failures) > 0 {
		t.Fatalf("found %d fmt.Sprintf(locale.Pick(...)) mismatches:\n%s", len(failures), strings.Join(failures, "\n"))
	}
}

func isSelectorNamed(expr ast.Expr, pkg, fn string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != fn {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

func extractStringLiteral(expr ast.Expr) (string, bool) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		unquoted, err := strconv.Unquote(v.Value)
		return unquoted, err == nil
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		left, okL := extractStringLiteral(v.X)
		right, okR := extractStringLiteral(v.Y)
		if okL && okR {
			return left + right, true
		}
		return "", false
	default:
		return "", false
	}
}

// countFormatArgs 解析 fmt 格式化字符串所需参数数量（统计 %% 转义外的所有占位符与 *）。
// countFormatArgs counts consumed arguments in a fmt format string (ignoring %%).
func countFormatArgs(format string) (count int, indexed bool) {
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		// flags: + - # 0 space
		for j < len(format) && strings.ContainsRune("+-#0 ", rune(format[j])) {
			j++
		}
		// optional explicit argument index [n]
		if j < len(format) && format[j] == '[' {
			indexed = true
		}
		for j < len(format) {
			ch := format[j]
			if ch == '*' {
				count++
				j++
				continue
			}
			if (ch >= '0' && ch <= '9') || ch == '.' || ch == '[' || ch == ']' {
				j++
				continue
			}
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				count++
				i = j
				break
			}
			break
		}
	}
	return count, indexed
}
