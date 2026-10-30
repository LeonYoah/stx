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

package executor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// writeFakeJDKForPrecheck 生成 bin/java 会打印指定版本的假 JDK。
// writeFakeJDKForPrecheck creates a fake JDK whose bin/java prints the given version.
func writeFakeJDKForPrecheck(t *testing.T, root, version string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake JDK scripts need a POSIX shell / 假 JDK 脚本需要 POSIX shell")
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho 'openjdk version \"" + version + "\" 2024-01-16' >&2\n"
	if err := os.WriteFile(filepath.Join(bin, "java"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// AC9：同时装了 JDK 8 与 17、PATH 上是 17、指定 8 时，预检检查并报告的是 8。
// AC9: with JDK 8 and 17 installed, 17 on PATH and 8 specified, precheck checks and reports 8.
func TestHandleCheckJavaCustomHomeWinsOverPath(t *testing.T) {
	dir := t.TempDir()
	jdk8 := writeFakeJDKForPrecheck(t, filepath.Join(dir, "jdk8"), "1.8.0_392")
	jdk17 := writeFakeJDKForPrecheck(t, filepath.Join(dir, "jdk17"), "17.0.9")
	t.Setenv("PATH", filepath.Join(jdk17, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := handleCheckJava(context.Background(), map[string]string{"java_home": jdk8})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	d := result.Details
	if d["java_home_source"] != "custom" || d["java_home"] != jdk8 || d["java_bin"] != filepath.Join(jdk8, "bin", "java") {
		t.Fatalf("unexpected details: %+v", d)
	}
	if d["installed_version"] != "8" || d["check_status"] != "passed" {
		t.Fatalf("expected version 8 passed, got %+v", d)
	}
}

// AC2：指定的 JDK 是 17 时返回警告（Success 仍为 true，check_status=warning）。
// AC2: a specified JDK 17 is a warning (Success stays true, check_status=warning).
func TestHandleCheckJavaCustomHomeWarnsOnUnrecommendedVersion(t *testing.T) {
	jdk17 := writeFakeJDKForPrecheck(t, filepath.Join(t.TempDir(), "jdk17"), "17.0.9")
	result, err := handleCheckJava(context.Background(), map[string]string{"java_home": jdk17})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.Details["check_status"] != "warning" || result.Details["installed_version"] != "17" {
		t.Fatalf("expected warning for 17, got %+v", result)
	}
}

// AC2/AC9：路径不存在时失败，并在信息中带上路径与原因。
// AC2/AC9: a missing path fails with the path and reason in the message.
func TestHandleCheckJavaCustomHomeMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-jdk")
	result, err := handleCheckJava(context.Background(), map[string]string{"java_home": missing})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatalf("expected failure, got %+v", result)
	}
	if !strings.Contains(result.Message, missing) || result.Details["error_reason"] != "not_found" {
		t.Fatalf("message/details should carry path and reason: %+v", result)
	}
	if result.Details["java_home_source"] != "custom" {
		t.Fatalf("source should be custom: %+v", result.Details)
	}
}

// AC6/AC9：java_home 为空时沿用 PATH 探测，来源为 detected。
// AC6/AC9: an empty java_home keeps PATH detection with source detected.
func TestHandleCheckJavaEmptyKeepsDetection(t *testing.T) {
	jdk11 := writeFakeJDKForPrecheck(t, filepath.Join(t.TempDir(), "jdk11"), "11.0.21")
	t.Setenv("PATH", filepath.Join(jdk11, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("JAVA_HOME", "")

	result, err := handleCheckJava(context.Background(), map[string]string{"java_home": "  "})
	if err != nil {
		t.Fatal(err)
	}
	d := result.Details
	if d["java_home_source"] != "detected" || d["java_bin"] != filepath.Join(jdk11, "bin", "java") || d["java_home"] != jdk11 {
		t.Fatalf("unexpected details: %+v", d)
	}
}

// TestHandleListJavaHomes 覆盖 AC1b：list_java_homes 成功返回 jdks JSON，空列表也不失败。
// TestHandleListJavaHomes covers AC1b: list_java_homes succeeds with jdks JSON and does not fail on an empty list.
func TestHandleListJavaHomes(t *testing.T) {
	result, err := handleListJavaHomes(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !result.Success || result.Details["jdks"] == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := strconv.Atoi(result.Details["count"]); err != nil {
		t.Fatalf("count: %v", err)
	}
}
