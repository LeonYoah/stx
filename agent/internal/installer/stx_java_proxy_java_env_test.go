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

package installer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/LeonYoah/stx/agent/internal/javaenv"
)

func writeFakeProxyJDK(t *testing.T, root string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake JDK scripts need a POSIX shell / 假 JDK 脚本需要 POSIX shell")
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "java"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func proxyEnvValue(env []string, key string) (string, bool) {
	for _, entry := range env {
		if k, v, ok := strings.Cut(entry, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// AC4a：配置了节点 JDK 时 stx-java-proxy 拿到同一个 java（STX_JAVA_BIN）与 JAVA_HOME。
// AC4a: with a configured node JDK, stx-java-proxy gets the same java (STX_JAVA_BIN) and JAVA_HOME.
func TestAppendSTXJavaProxyJavaEnvCustom(t *testing.T) {
	dir := t.TempDir()
	jdk8 := writeFakeProxyJDK(t, filepath.Join(dir, "jdk8"))
	jdk17 := writeFakeProxyJDK(t, filepath.Join(dir, "jdk17"))
	t.Setenv("PATH", filepath.Join(jdk17, "bin"))

	env := appendSTXJavaProxyJavaEnv(javaenv.WithHome(context.Background(), jdk8), []string{"A=1"})
	if v, _ := proxyEnvValue(env, javaenv.EnvJavaBin); v != filepath.Join(jdk8, "bin", "java") {
		t.Fatalf("STX_JAVA_BIN = %q", v)
	}
	if v, _ := proxyEnvValue(env, "JAVA_HOME"); v != jdk8 {
		t.Fatalf("JAVA_HOME = %q", v)
	}
}

// AC4a/AC6：未配置时 stx-java-proxy 使用与 SeaTunnel 相同的 Agent PATH java，不额外设置 JAVA_HOME。
// AC4a/AC6: without config stx-java-proxy uses the same Agent PATH java as SeaTunnel and leaves JAVA_HOME alone.
func TestAppendSTXJavaProxyJavaEnvDetected(t *testing.T) {
	jdk11 := writeFakeProxyJDK(t, filepath.Join(t.TempDir(), "jdk11"))
	t.Setenv("PATH", filepath.Join(jdk11, "bin"))

	env := appendSTXJavaProxyJavaEnv(context.Background(), []string{"A=1"})
	if v, _ := proxyEnvValue(env, javaenv.EnvJavaBin); v != filepath.Join(jdk11, "bin", "java") {
		t.Fatalf("STX_JAVA_BIN = %q", v)
	}
	if _, ok := proxyEnvValue(env, "JAVA_HOME"); ok {
		t.Fatalf("JAVA_HOME should not be set in auto mode: %v", env)
	}

	// 没有 java 时环境不变，脚本回退到 `java`。/ With no java the env is unchanged and the script falls back to `java`.
	t.Setenv("PATH", t.TempDir())
	if env := appendSTXJavaProxyJavaEnv(context.Background(), []string{"A=1"}); len(env) != 1 {
		t.Fatalf("env should be unchanged, got %v", env)
	}
}

// AC4a：stx-java-proxy.sh 必须用 STX_JAVA_BIN 启动，未设置时回退 java。
// AC4a: stx-java-proxy.sh must launch with STX_JAVA_BIN and fall back to java when unset.
func TestSTXJavaProxyScriptUsesSTXJavaBin(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "stx-java-proxy.sh"))
	if err != nil {
		t.Skipf("script not found: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `JAVA_CMD="${STX_JAVA_BIN:-java}"`) || strings.Contains(text, "exec java ") {
		t.Fatal("stx-java-proxy.sh must exec \"${JAVA_CMD}\" derived from STX_JAVA_BIN")
	}
}

// 已运行代理的 java 与配置不一致时视为不匹配；未配置时始终匹配（保持原有复用行为）。
// A running proxy on another java mismatches a configured JDK; no config always matches (original reuse).
func TestSTXJavaProxyJavaMatchesDesired(t *testing.T) {
	if !stxJavaProxyJavaMatchesDesired(os.Getpid(), "") {
		t.Fatal("empty java_home must match")
	}
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc")
	}
	jdk := writeFakeProxyJDK(t, filepath.Join(t.TempDir(), "jdk8"))
	if stxJavaProxyJavaMatchesDesired(os.Getpid(), jdk) {
		t.Fatal("the test binary is not the configured java, so it must mismatch")
	}
}
