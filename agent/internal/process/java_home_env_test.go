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

package process

import (
	"context"
	"os"
	"strings"
	"testing"
)

func envValue(env []string, key string) (string, int) {
	value, count := "", 0
	for _, entry := range env {
		if k, v, ok := strings.Cut(entry, "="); ok && k == key {
			value = v
			count++
		}
	}
	return value, count
}

// AC4：配置了节点 JDK 时 SeaTunnel 子进程的 JAVA_HOME 与 PATH 指向它（重启走同一 StartProcess 路径）。
// AC4: a configured node JDK sets JAVA_HOME and PATH for the SeaTunnel child (restarts share StartProcess).
func TestBuildStartCommandAppliesNodeJavaHome(t *testing.T) {
	t.Setenv("JAVA_HOME", "/usr/lib/jvm/java-17")
	t.Setenv("PATH", "/usr/bin:/bin")
	cmd := buildStartCommand(context.Background(), &StartParams{InstallDir: "/opt/seatunnel", JavaHome: "/opt/jdk8"})

	if v, n := envValue(cmd.Env, "JAVA_HOME"); v != "/opt/jdk8" || n != 1 {
		t.Fatalf("JAVA_HOME = %q (x%d)", v, n)
	}
	if v, n := envValue(cmd.Env, "PATH"); v != "/opt/jdk8/bin:/usr/bin:/bin" || n != 1 {
		t.Fatalf("PATH = %q (x%d)", v, n)
	}
	// AC5：Agent 自身环境不变。/ AC5: the Agent's own environment is unchanged.
	if os.Getenv("JAVA_HOME") != "/usr/lib/jvm/java-17" {
		t.Fatalf("agent JAVA_HOME changed to %q", os.Getenv("JAVA_HOME"))
	}
}

// AC6：未配置时与原来一样直接继承 Agent 环境。/ AC6: without config the Agent env is inherited as before.
func TestBuildStartCommandWithoutJavaHomeKeepsEnv(t *testing.T) {
	t.Setenv("JAVA_HOME", "/usr/lib/jvm/java-17")
	t.Setenv("PATH", "/usr/bin:/bin")
	cmd := buildStartCommand(context.Background(), &StartParams{InstallDir: "/opt/seatunnel"})

	want := append(os.Environ(), "SEATUNNEL_HOME=/opt/seatunnel")
	if strings.Join(cmd.Env, "\n") != strings.Join(want, "\n") {
		t.Fatalf("env changed without java_home:\n%v", cmd.Env)
	}
}

// AC2/AC4：配置的 JDK 不可用时启动直接失败，不回退到 PATH 上的 java。
// AC2/AC4: an unusable configured JDK fails the start instead of falling back to java on PATH.
func TestStartProcessRejectsInvalidJavaHome(t *testing.T) {
	installDir := t.TempDir()
	if err := os.MkdirAll(installDir+"/bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installDir+"/bin/seatunnel-cluster.sh", []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewProcessManager()
	err := m.StartProcess(context.Background(), "seatunnel-test", &StartParams{InstallDir: installDir, JavaHome: installDir + "/no-jdk"})
	if err == nil || !strings.Contains(err.Error(), installDir+"/no-jdk") {
		t.Fatalf("expected JDK error naming the path, got %v", err)
	}
}
