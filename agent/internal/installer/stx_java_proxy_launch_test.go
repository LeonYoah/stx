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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/agent/internal/javaenv"
	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
)

// TestStxJavaProxyClasspath 校验直启 classpath 与脚本一致：lib/*:starter.jar:proxy jar (+EXTRA)。
// TestStxJavaProxyClasspath verifies the direct-launch classpath matches the script:
// lib/*:starter.jar:proxy jar (+EXTRA).
func TestStxJavaProxyClasspath(t *testing.T) {
	sep := string(os.PathListSeparator)
	installDir := filepath.Join("/opt", "seatunnel-3.0.0")
	jar := filepath.Join("/home", "lib", "stx-java-proxy-v3.jar")
	want := filepath.Join(installDir, "lib", "*") + sep + filepath.Join(installDir, "starter", "seatunnel-starter.jar") + sep + jar
	if got := stxJavaProxyClasspath(installDir, jar); got != want {
		t.Fatalf("classpath = %q, want %q", got, want)
	}

	t.Setenv(stxJavaProxyExtraClasspathEnvVar, "/extra/a.jar"+sep+"/extra/b.jar")
	want += sep + "/extra/a.jar" + sep + "/extra/b.jar"
	if got := stxJavaProxyClasspath(installDir, jar); got != want {
		t.Fatalf("classpath with EXTRA = %q, want %q", got, want)
	}
}

// TestStxJavaProxyJVMArgs 无 -Xmx 注入默认堆；已有 -Xmx 不重复注入；保留调用方参数顺序。
// TestStxJavaProxyJVMArgs injects the default heap without -Xmx, never duplicates an existing
// -Xmx, and keeps caller arg order.
func TestStxJavaProxyJVMArgs(t *testing.T) {
	args := stxJavaProxyJVMArgs("")
	if len(args) != 2 || args[0] != "-Xms64m" || args[1] != "-Xmx512m" {
		t.Fatalf("empty opts must inject default heap, got %v", args)
	}
	args = stxJavaProxyJVMArgs("-Xmx256m -Dfoo=bar")
	if len(args) != 2 || args[0] != "-Xmx256m" {
		t.Fatalf("existing -Xmx must not be duplicated, got %v", args)
	}
	args = stxJavaProxyJVMArgs("-XX:+UseG1GC")
	if len(args) != 3 || args[2] != "-XX:+UseG1GC" {
		t.Fatalf("caller opts must be preserved after defaults, got %v", args)
	}
}

// TestBuildSTXJavaProxyJavaArgv 校验完整 argv：端口 / SEATUNNEL_HOME 系统属性、-cp 与主类。
// TestBuildSTXJavaProxyJavaArgv verifies the full argv: port and SEATUNNEL_HOME properties,
// -cp and the main class.
func TestBuildSTXJavaProxyJavaArgv(t *testing.T) {
	installDir := filepath.Join("/opt", "seatunnel-3.0.0")
	jar := filepath.Join("/home", "lib", "stx-java-proxy-v3.jar")
	argv := buildSTXJavaProxyJavaArgv(installDir, jar, 18080, "-Dfoo=1")
	joined := strings.Join(argv, " ")
	for _, marker := range []string{
		"-Dstx.java.proxy.port=18080",
		"-DSEATUNNEL_HOME=" + installDir,
		"-Dstx.java.proxy.seatunnel.home=" + installDir,
		stxJavaProxyMainClass,
	} {
		if !strings.Contains(joined, marker) {
			t.Fatalf("argv %q missing %q", joined, marker)
		}
	}
	cpIdx, mainIdx := -1, -1
	for i, arg := range argv {
		if arg == "-cp" {
			cpIdx = i
		}
		if arg == stxJavaProxyMainClass {
			mainIdx = i
		}
	}
	if cpIdx <= 0 || mainIdx != len(argv)-1 || cpIdx+2 != mainIdx {
		t.Fatalf("argv layout must be '... -cp <classpath> <main>', got %v", argv)
	}
	if argv[cpIdx+1] != stxJavaProxyClasspath(installDir, jar) {
		t.Fatalf("classpath arg = %q", argv[cpIdx+1])
	}
}

// writeFakeJavaProxyJDK 造一个假 JDK：bin/java 记录 argv 后 exec python3 在 STX_JAVA_PROXY_PORT
// 上应答 /healthz 200，模拟健康代理。缺 python3 时跳过。
// writeFakeJavaProxyJDK creates a fake JDK: bin/java records argv then execs python3 answering
// /healthz 200 on STX_JAVA_PROXY_PORT to simulate a healthy proxy. Skips without python3.
func writeFakeJavaProxyJDK(t *testing.T, root, capturePath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake JDK scripts need a POSIX shell / 假 JDK 脚本需要 POSIX shell")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for fake java proxy / 无 python3 无法模拟代理进程")
	}
	javaBin := filepath.Join(root, "bin", "java")
	if err := os.MkdirAll(filepath.Dir(javaBin), 0o755); err != nil {
		t.Fatal(err)
	}
	// 命令行内嵌主类名，让 isSTXJavaProxyCommandLine 识别该进程为代理。
	// The main class name is embedded so isSTXJavaProxyCommandLine recognises this process as the proxy.
	script := `#!/bin/sh
printf '%s\n' "$@" > "` + capturePath + `"
exec python3 -c '
import http.server,os,socketserver
# io.github.leonyoah.stx.proxy.StxJavaProxyApplication
port=int(os.environ.get("STX_JAVA_PROXY_PORT","0"))
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length","2")
        self.end_headers()
        self.wfile.write(b"{}")
    def log_message(self,*a):
        pass
socketserver.TCPServer(("127.0.0.1",port),H).serve_forever()
'`
	if err := os.WriteFile(javaBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// prepareProxyInstallDir 造最小可用 SeaTunnel 目录：starter jar + 代理 jar。
// prepareProxyInstallDir creates a minimal SeaTunnel dir: starter jar plus the proxy jar.
func prepareProxyInstallDir(t *testing.T, home, installDir string) (jarPath, stateDir string) {
	t.Helper()
	starterJar := filepath.Join(installDir, "starter", "seatunnel-starter.jar")
	if err := os.MkdirAll(filepath.Dir(starterJar), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(starterJar, []byte("starter"), 0o644); err != nil {
		t.Fatal(err)
	}
	jarPath = filepath.Join(home, "lib", seatunnelmeta.STXJavaProxyJarFileName("2.3.13"))
	if err := os.MkdirAll(filepath.Dir(jarPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jarPath, []byte("proxy-jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir = filepath.Join(home, "logs", "stx-java-proxy")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return jarPath, stateDir
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// TestStartSTXJavaProxyServiceDirectLifecycle 无 stx-java-proxy.sh 时直启：参数拼接、
// 状态文件落盘、健康探测与优雅停止。
// TestStartSTXJavaProxyServiceDirectLifecycle covers direct launch without stx-java-proxy.sh:
// argv assembly, state file persistence, health wait and graceful stop.
func TestStartSTXJavaProxyServiceDirectLifecycle(t *testing.T) {
	home := t.TempDir()
	installDir := t.TempDir()
	capture := filepath.Join(t.TempDir(), "argv.txt")
	jdk := writeFakeJavaProxyJDK(t, filepath.Join(t.TempDir(), "jdk17"), capture)
	jarPath, stateDir := prepareProxyInstallDir(t, home, installDir)
	port := freeTCPPort(t)

	t.Setenv(stxJavaProxyHomeEnvVar, home)
	t.Setenv(stxJavaProxyScriptEnvVar, "")
	t.Setenv(stxJavaProxyEndpointEnvVar, "")
	t.Setenv(stxJavaProxyJarEnvVar, "")
	t.Setenv(stxJavaProxyPortEnvVar, "")
	t.Setenv(stxJavaProxyJvmOptsEnvVar, "")
	t.Setenv(stxJavaProxyExtraClasspathEnvVar, "")

	ctx := javaenv.WithHome(context.Background(), jdk)
	baseURL, err := startSTXJavaProxyServiceDirect(ctx, installDir, jarPath, stateDir, port, "-Xmx128m", "2.3.13")
	if err != nil {
		t.Fatalf("direct start: %v", err)
	}
	if baseURL != stxJavaProxyServiceBaseURL(port) {
		t.Fatalf("baseURL = %q", baseURL)
	}
	t.Cleanup(func() {
		_, _ = StopManagedSTXJavaProxyService(context.Background(), installDir)
	})

	// 落盘状态：pid 存活、端口与 JVM 参数一致。
	// Persisted state: live pid, matching port and JVM opts.
	pidBytes, err := os.ReadFile(filepath.Join(stateDir, "service.pid"))
	if err != nil {
		t.Fatalf("service.pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 || !stxJavaProxyPIDAlive(pid) {
		t.Fatalf("pid not alive: %q", pidBytes)
	}
	if got, _ := os.ReadFile(filepath.Join(stateDir, "service.port")); strings.TrimSpace(string(got)) != strconv.Itoa(port) {
		t.Fatalf("service.port = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(stateDir, "service.jvm_opts")); strings.TrimSpace(string(got)) != "-Xmx128m" {
		t.Fatalf("service.jvm_opts = %q", got)
	}

	// 传给 java 的 argv：自定义 -Xmx 不重复注入，端口属性 / -cp / 主类齐备。
	// The argv handed to java: the custom -Xmx is not duplicated and port / -cp / main are complete.
	var argvText string
	for i := 0; i < 40; i++ {
		if data, err := os.ReadFile(capture); err == nil && len(data) > 0 {
			argvText = string(data)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if argvText == "" {
		t.Fatal("fake java never captured argv")
	}
	for _, marker := range []string{
		"-Dstx.java.proxy.port=" + strconv.Itoa(port),
		"-DSEATUNNEL_HOME=" + installDir,
		"-Xmx128m",
		"-cp", stxJavaProxyMainClass,
	} {
		if !strings.Contains(argvText, marker) {
			t.Fatalf("captured argv missing %q: %q", marker, argvText)
		}
	}
	if strings.Contains(argvText, "-Xmx512m") {
		t.Fatalf("custom -Xmx must suppress default heap: %q", argvText)
	}

	// 健康状态与优雅停止。
	// Healthy status then a graceful stop.
	status, err := GetManagedSTXJavaProxyServiceStatus(ctx, installDir)
	if err != nil || !status.Healthy || !status.Running || status.PID != pid {
		t.Fatalf("expected healthy managed status, got %+v err=%v", status, err)
	}
	stopped, err := StopManagedSTXJavaProxyService(ctx, installDir)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if stopped.Running {
		t.Fatalf("expected stopped status, got %+v", stopped)
	}
	if stxJavaProxyPIDAlive(pid) {
		t.Fatalf("pid %d still alive after stop", pid)
	}
}

// TestStartSTXJavaProxyServiceScriptOverride 显式 STX_JAVA_PROXY_SCRIPT 仍走脚本路径：
// mock 脚本直接回显端口，验证旧测试契约不受影响。
// TestStartSTXJavaProxyServiceScriptOverride: an explicit STX_JAVA_PROXY_SCRIPT keeps the script
// path — the mock script echoes the port, proving the legacy contract is intact.
func TestStartSTXJavaProxyServiceScriptOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock script needs a POSIX shell / mock 脚本需要 POSIX shell")
	}
	home := t.TempDir()
	installDir := t.TempDir()
	jarPath, stateDir := prepareProxyInstallDir(t, home, installDir)

	// mock 脚本：从 STX_JAVA_PROXY_PORT 环境变量取端口应答健康检查（复用 python3）。
	// The mock script reads STX_JAVA_PROXY_PORT from env and serves health on it (python3 again).
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available / 无 python3")
	}
	marker := filepath.Join(t.TempDir(), "script-used")
	scriptPath := filepath.Join(t.TempDir(), "stx-java-proxy.sh")
	script := `#!/bin/sh
touch "` + marker + `"
exec python3 -c '
import http.server,os,socketserver
# io.github.leonyoah.stx.proxy.StxJavaProxyApplication
port=int(os.environ.get("STX_JAVA_PROXY_PORT","0"))
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length","2")
        self.end_headers()
        self.wfile.write(b"{}")
    def log_message(self,*a):
        pass
socketserver.TCPServer(("127.0.0.1",port),H).serve_forever()
'`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	port := freeTCPPort(t)
	t.Setenv(stxJavaProxyHomeEnvVar, home)
	t.Setenv(stxJavaProxyScriptEnvVar, scriptPath)
	t.Setenv(stxJavaProxyEndpointEnvVar, "")
	t.Setenv(stxJavaProxyJarEnvVar, "")
	t.Setenv(stxJavaProxyPortEnvVar, "")
	t.Setenv(stxJavaProxyJvmOptsEnvVar, "")

	ctx := context.Background()
	baseURL, err := startSTXJavaProxyService(ctx, installDir, "2.3.13", scriptPath, jarPath, stateDir, port)
	if err != nil {
		t.Fatalf("script start: %v", err)
	}
	t.Cleanup(func() {
		_, _ = StopManagedSTXJavaProxyService(context.Background(), installDir)
	})
	if baseURL != stxJavaProxyServiceBaseURL(port) {
		t.Fatalf("baseURL = %q", baseURL)
	}
	if !fileExists(marker) {
		t.Fatal("STX_JAVA_PROXY_SCRIPT path must execute the script")
	}
}
