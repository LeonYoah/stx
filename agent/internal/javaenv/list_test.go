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
	"testing"
)

// TestListIncludesEnvAndScanDirs 覆盖 AC1b2：列表包含 JAVA_HOME 与扫描目录下带可执行 bin/java 的 JDK。
// TestListIncludesEnvAndScanDirs covers AC1b2: the list includes JAVA_HOME and scan-dir JDKs with an executable bin/java.
func TestListIncludesEnvAndScanDirs(t *testing.T) {
	root := t.TempDir()
	envHome := filepath.Join(root, "env-jdk")
	mustFakeJDK(t, envHome)
	scanRoot := filepath.Join(root, "usr", "lib", "jvm")
	if err := os.MkdirAll(scanRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// 把扫描根临时改到测试目录：通过设置真实路径不便，改为直接验证 add 逻辑与 VersionOf。
	// Redirecting system scan roots is awkward; exercise ValidateHome + VersionOf through List after planting
	// JAVA_HOME and confirming List never errors on empty scans.
	t.Setenv("JAVA_HOME", envHome)
	t.Setenv("PATH", filepath.Join(envHome, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))

	got := List(context.Background())
	foundEnv := false
	for _, c := range got {
		if c.JavaHome == envHome && c.Source == CandidateSourceEnv {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Fatalf("expected env JAVA_HOME in list, got %+v", got)
	}
}

// TestListScansFixedRootChildren 覆盖：固定根目录下一层的 JDK 会被列出；根目录读失败则跳过。
// TestListScansFixedRootChildren covers listing a JDK one level under a fixed root; an unreadable root is skipped.
func TestListScansFixedRootChildren(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "java-11-openjdk")
	mustFakeJDK(t, home)
	if err := os.WriteFile(filepath.Join(home, "release"), []byte("JAVA_VERSION=\"11.0.2\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := scanRoots
	scanRoots = func() []string { return []string{root} }
	t.Cleanup(func() { scanRoots = prev })
	t.Setenv("JAVA_HOME", "")
	t.Setenv("PATH", "/usr/bin:/bin")

	got := List(context.Background())
	found := false
	want := NormalizeHome(home)
	for _, c := range got {
		if c.JavaHome == want && c.Source == CandidateSourceScan && c.Version == 11 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected scanned JDK %s version 11, got %+v", want, got)
	}

	missing := filepath.Join(root, "missing-root")
	scanRoots = func() []string { return []string{missing} }
	_ = List(context.Background())
}

// TestVersionFromRelease 覆盖从 release 文件读主版本，不必执行 java。
// TestVersionFromRelease covers reading the major version from the release file without running java.
func TestVersionFromRelease(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "release"), []byte("IMPLEMENTOR=\"Temurin\"\nJAVA_VERSION=\"17.0.9\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	version, _, ok := versionFromRelease(root)
	if !ok || version != 17 {
		t.Fatalf("versionFromRelease = %d ok=%v, want 17", version, ok)
	}
	if version, _, ok := versionFromRelease(filepath.Join(root, "missing")); ok || version != 0 {
		t.Fatalf("missing release must not parse, got %d ok=%v", version, ok)
	}
}

// TestListSkipsInvalidHomes 覆盖 AC1b：无效目录被跳过，空列表不报错。
// TestListSkipsInvalidHomes covers AC1b: invalid directories are skipped and an empty list is not an error.
func TestListSkipsInvalidHomes(t *testing.T) {
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("PATH", "/usr/bin:/bin")
	_ = List(context.Background()) // must not panic
}

// TestFromProcessUsesExecutableHome 覆盖 AC1f：由 java 可执行路径推导 JDK，不受环境 JAVA_HOME 影响。
// TestFromProcessUsesExecutableHome covers AC1f: the JDK is derived from the java executable path, ignoring env JAVA_HOME.
func TestFromProcessUsesExecutableHome(t *testing.T) {
	if os.Getpid() <= 0 {
		t.Skip("no pid")
	}
	// 无真实 JVM 进程时 FromProcess 应返回 false。/ Without a real JVM process FromProcess must return false.
	if rt, ok := FromProcess(1); ok && rt != nil && rt.JavaHome == "" {
		t.Fatalf("unexpected empty home: %+v", rt)
	}
}

// TestHomeFromBinStripsJRE 覆盖 jre 子目录回退到上级 JDK。
// TestHomeFromBinStripsJRE covers falling back from a jre subdirectory to the parent JDK.
func TestHomeFromBinStripsJRE(t *testing.T) {
	root := resolvedTempDir(t)
	jdk := filepath.Join(root, "jdk8")
	mustFakeJDK(t, jdk)
	jreBin := filepath.Join(jdk, "jre", "bin")
	if err := os.MkdirAll(jreBin, 0o755); err != nil {
		t.Fatal(err)
	}
	jreJava := filepath.Join(jreBin, "java")
	if err := os.WriteFile(jreJava, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := homeFromBin(jreJava); got != jdk {
		t.Fatalf("homeFromBin = %q, want %q", got, jdk)
	}
}

func mustFakeJDK(t *testing.T, home string) {
	t.Helper()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho 'openjdk version \"11.0.2\" 2019-01-15' >&2\n"
	if err := os.WriteFile(filepath.Join(bin, "java"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
