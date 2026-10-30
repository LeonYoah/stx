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

package javaenv

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeJDK 在临时目录下生成假 JDK：bin/java 为打印指定版本的脚本，并附带指定的工具。
// writeFakeJDK creates a fake JDK in a temp dir: bin/java is a script printing the given version, plus the given tools.
func writeFakeJDK(t *testing.T, root, version string, tools ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake JDK scripts need a POSIX shell / 假 JDK 脚本需要 POSIX shell")
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho 'openjdk version \"" + version + "\"' >&2\n"
	if err := os.WriteFile(filepath.Join(bin, "java"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestValidateHome(t *testing.T) {
	dir := t.TempDir()
	good := writeFakeJDK(t, filepath.Join(dir, "jdk8"), "1.8.0_392")

	rt, err := ValidateHome(good + "/")
	if err != nil {
		t.Fatalf("ValidateHome(good) error = %v", err)
	}
	if rt.JavaHome != good || rt.JavaBin != filepath.Join(good, "bin", "java") || rt.Source != SourceCustom {
		t.Fatalf("ValidateHome(good) = %+v", rt)
	}

	noExec := filepath.Join(dir, "noexec")
	if err := os.MkdirAll(filepath.Join(noExec, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noExec, "bin", "java"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "plain-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	emptyDir := filepath.Join(dir, "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		home   string
		reason string
		path   string
	}{
		{"relative", "opt/jdk", ReasonNotAbsolute, "opt/jdk"},
		{"missing", filepath.Join(dir, "missing"), ReasonNotFound, filepath.Join(dir, "missing")},
		{"file", file, ReasonNotDirectory, file},
		{"no java", emptyDir, ReasonJavaMissing, filepath.Join(emptyDir, "bin", "java")},
		{"not executable", noExec, ReasonJavaNotExec, filepath.Join(noExec, "bin", "java")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateHome(tc.home)
			if err == nil {
				t.Fatalf("ValidateHome(%q) expected error", tc.home)
			}
			if got := ReasonOf(err); got != tc.reason {
				t.Fatalf("ReasonOf = %q, want %q", got, tc.reason)
			}
			// 错误信息必须带上具体路径（中英文两段都带）。/ The message must name the path in both languages.
			if strings.Count(err.Error(), tc.path) < 2 {
				t.Fatalf("error %q should mention %q twice", err.Error(), tc.path)
			}
		})
	}
}

func TestResolveEmptyUsesDetection(t *testing.T) {
	dir := t.TempDir()
	jdk := writeFakeJDK(t, filepath.Join(dir, "jdk17"), "17.0.9")
	t.Setenv("PATH", filepath.Join(jdk, "bin"))
	t.Setenv("JAVA_HOME", "")

	rt, err := Resolve("  ")
	if err != nil {
		t.Fatalf("Resolve(empty) error = %v", err)
	}
	if rt.Source != SourceDetected || rt.JavaBin != filepath.Join(jdk, "bin", "java") || rt.JavaHome != jdk {
		t.Fatalf("Resolve(empty) = %+v", rt)
	}

	t.Setenv("PATH", filepath.Join(dir, "nothing"))
	if _, err := Resolve(""); ReasonOf(err) != ReasonJavaNotOnPath {
		t.Fatalf("Resolve(empty) without java: err = %v", err)
	}
}

func TestDetectStripsJRE(t *testing.T) {
	dir := t.TempDir()
	jdk := writeFakeJDK(t, filepath.Join(dir, "jdk8"), "1.8.0_392", "jstack")
	writeFakeJDK(t, filepath.Join(jdk, "jre"), "1.8.0_392")
	t.Setenv("PATH", filepath.Join(jdk, "jre", "bin"))
	t.Setenv("JAVA_HOME", "")

	rt, err := Detect()
	if err != nil {
		t.Fatal(err)
	}
	if rt.JavaHome != jdk {
		t.Fatalf("Detect().JavaHome = %q, want %q", rt.JavaHome, jdk)
	}
}

func TestApplyEnv(t *testing.T) {
	env := []string{"A=1", "JAVA_HOME=/old", "PATH=/usr/bin:/bin"}

	// AC6：未配置时环境原样返回。/ AC6: no configured home leaves the env untouched.
	if got := ApplyEnv(env, ""); strings.Join(got, ";") != strings.Join(env, ";") {
		t.Fatalf("ApplyEnv(empty) = %v", got)
	}

	got := ApplyEnv(env, "/opt/jdk8/")
	want := []string{"A=1", "JAVA_HOME=/opt/jdk8", "PATH=/opt/jdk8/bin:/usr/bin:/bin"}
	if strings.Join(got, ";") != strings.Join(want, ";") {
		t.Fatalf("ApplyEnv = %v, want %v", got, want)
	}
	// AC5：不修改传入的切片（全局/Agent 环境不变）。/ AC5: the input slice (Agent env) is not modified.
	if env[1] != "JAVA_HOME=/old" {
		t.Fatalf("input env was modified: %v", env)
	}
}

func TestLookupTool(t *testing.T) {
	dir := t.TempDir()
	jdk8 := writeFakeJDK(t, filepath.Join(dir, "jdk8"), "1.8.0_392", "jstack", "jcmd")
	jdk17 := writeFakeJDK(t, filepath.Join(dir, "jdk17"), "17.0.9", "jstack", "jcmd", "jmap")
	t.Setenv("PATH", filepath.Join(jdk17, "bin"))
	t.Setenv("JAVA_HOME", "")

	// 配置了 JDK 8：只用 JDK 8 的工具，即使 PATH 上是 17。/ Configured JDK 8 wins over 17 on PATH.
	path, err := LookupTool(jdk8, "jstack")
	if err != nil || path != filepath.Join(jdk8, "bin", "jstack") {
		t.Fatalf("LookupTool(jdk8, jstack) = %q, %v", path, err)
	}
	// 配置的 JDK 缺少工具时报错，不回退 PATH。/ A missing tool in the configured JDK never falls back to PATH.
	if _, err := LookupTool(jdk8, "jmap"); ReasonOf(err) != ReasonToolNotAvailable {
		t.Fatalf("LookupTool(jdk8, jmap) err = %v", err)
	}
	// 未配置：使用探测到的 JDK。/ Not configured: the detected JDK is used.
	path, err = LookupTool("", "jmap")
	if err != nil || path != filepath.Join(jdk17, "bin", "jmap") {
		t.Fatalf("LookupTool(empty, jmap) = %q, %v", path, err)
	}
}

func TestContextAndShellPrefix(t *testing.T) {
	ctx := context.Background()
	if WithHome(ctx, " ") != ctx || HomeFromContext(ctx) != "" {
		t.Fatal("empty home must not be stored")
	}
	if got := HomeFromContext(WithHome(ctx, "/opt/jdk8/")); got != "/opt/jdk8" {
		t.Fatalf("HomeFromContext = %q", got)
	}
	if ShellExportPrefix("") != "" {
		t.Fatal("empty home must give an empty prefix")
	}
	if got := ShellExportPrefix("/opt/it's jdk"); got != `export JAVA_HOME='/opt/it'\''s jdk'; export PATH="$JAVA_HOME/bin:$PATH"; ` {
		t.Fatalf("ShellExportPrefix = %q", got)
	}
}
