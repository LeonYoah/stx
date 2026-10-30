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

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeJDKTools 生成假 JDK，工具脚本会输出 "<label> <tool> <args>"，用于判断实际调用的是哪个 JDK。
// writeFakeJDKTools creates a fake JDK whose tool scripts print "<label> <tool> <args>" to show which JDK ran.
func writeFakeJDKTools(t *testing.T, root, label string, tools ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake JDK scripts need a POSIX shell / 假 JDK 脚本需要 POSIX shell")
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "java"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		script := "#!/bin/sh\necho " + label + " " + tool + " \"$@\"\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// AC4b：配置了节点 JDK 时 jcmd/jstack/jmap 取自 <java_home>/bin，而不是 PATH。
// AC4b: with a configured node JDK, jcmd/jstack/jmap come from <java_home>/bin, not PATH.
func TestDiagnosticsUseConfiguredJDKTools(t *testing.T) {
	dir := t.TempDir()
	jdk8 := writeFakeJDKTools(t, filepath.Join(dir, "jdk8"), "jdk8", "jcmd", "jstack", "jmap")
	jdk17 := writeFakeJDKTools(t, filepath.Join(dir, "jdk17"), "jdk17", "jcmd", "jstack", "jmap")
	t.Setenv("PATH", filepath.Join(jdk17, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("JAVA_HOME", "")

	output, tool, err := runThreadDumpCommand(context.Background(), 4242, jdk8)
	if err != nil || tool != "jcmd" || !strings.HasPrefix(string(output), "jdk8 jcmd 4242") {
		t.Fatalf("thread dump = %q, %q, %v", output, tool, err)
	}
	if tool, err := runJVMDumpCommand(context.Background(), 4242, filepath.Join(dir, "heap.hprof"), jdk8); err != nil || tool != "jcmd" {
		t.Fatalf("jvm dump = %q, %v", tool, err)
	}

	// AC6：未配置时沿用探测到的 JDK（此处为 PATH 上的 17）。/ AC6: no config keeps the detected JDK (17 on PATH).
	output, _, err = runThreadDumpCommand(context.Background(), 4242, "")
	if err != nil || !strings.HasPrefix(string(output), "jdk17 jcmd") {
		t.Fatalf("auto thread dump = %q, %v", output, err)
	}
}

// AC4b：配置的 JDK 缺少工具时不回退 PATH，并提示具体目录。
// AC4b: a configured JDK without tools never falls back to PATH and names the directory.
func TestDiagnosticsConfiguredJDKWithoutToolsDoesNotUsePath(t *testing.T) {
	dir := t.TempDir()
	jre := writeFakeJDKTools(t, filepath.Join(dir, "jre-only"), "jre")
	jdk17 := writeFakeJDKTools(t, filepath.Join(dir, "jdk17"), "jdk17", "jcmd", "jstack")
	t.Setenv("PATH", filepath.Join(jdk17, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, err := runThreadDumpCommand(context.Background(), 4242, jre)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(jre, "bin")) {
		t.Fatalf("expected hint naming %s, got %v", filepath.Join(jre, "bin"), err)
	}
}
