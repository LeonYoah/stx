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
	// 命令行内嵌主类名，让 isSTXJavaProxyCommandLine 识别该进程为代理；
	// 透传 "$@" 使 python 进程命令行携带 -DSEATUNNEL_HOME 等参数，归属校验才能判定所属 installDir。
	// The main class name is embedded so isSTXJavaProxyCommandLine recognises this process as the proxy;
	// forwarding "$@" keeps -DSEATUNNEL_HOME etc. on the python cmdline so ownership checks can
	// attribute the process to its installDir.
	// allow_reuse_address 模拟 Java ServerSocket 的 SO_REUSEADDR，否则 TIME_WAIT 会让重启 bind 失败。
	// allow_reuse_address mirrors Java ServerSocket's SO_REUSEADDR so a restart is not
	// blocked by TIME_WAIT sockets.
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
class S(socketserver.TCPServer):
    allow_reuse_address=True
S(("127.0.0.1",port),H).serve_forever()
' "$@"`
	if err := os.WriteFile(javaBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// prepareProxyInstallDir 造最小可用 SeaTunnel 目录：starter jar + 代理 jar + 该 installDir 专属
// stateDir（与 stxJavaProxyServiceStateDir 同一派生规则）。
// prepareProxyInstallDir creates a minimal SeaTunnel dir: starter jar, the proxy jar and the
// installDir-specific stateDir derived exactly like stxJavaProxyServiceStateDir.
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
	stateDir = filepath.Join(home, "logs", "stx-java-proxy", "instances", stxJavaProxyInstallDirInstanceID(installDir))
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
	// mock 脚本与真实 stx-java-proxy.sh 一样把 SEATUNNEL_HOME 注入代理命令行，
	// 保证归属校验能识别进程所属 installDir。
	// Like the real stx-java-proxy.sh, the mock injects SEATUNNEL_HOME into the proxy
	// cmdline so ownership checks can attribute the process to its installDir.
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
class S(socketserver.TCPServer):
    allow_reuse_address=True
S(("127.0.0.1",port),H).serve_forever()
' "-DSEATUNNEL_HOME=${SEATUNNEL_HOME:-}" "-Dstx.java.proxy.seatunnel.home=${SEATUNNEL_HOME:-}"`
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

// TestManagedSTXJavaProxyDualInstallDirIsolation 同机双 installDir 隔离：
// 独立 stateDir 与端口；显式端口冲突报错且不动他集群进程；自动分配跳过他集群端口；
// 重启/停止 A 不伤 B；任一集群停止后 status 明确 running=false、healthy=false、PID=0，
// 且绝不把另一集群的进程错认成自己的。
// TestManagedSTXJavaProxyDualInstallDirIsolation verifies same-host isolation across two
// installDirs: distinct stateDirs and ports; an explicit port conflict errors out without
// touching the other cluster; auto-assignment skips foreign-held candidate ports; restarting
// or stopping A leaves B alive; a stopped cluster reports running=false, healthy=false,
// PID=0 and never claims the other cluster's process.
func TestManagedSTXJavaProxyDualInstallDirIsolation(t *testing.T) {
	home := t.TempDir()
	installDirA := t.TempDir()
	installDirB := t.TempDir()
	jdk := writeFakeJavaProxyJDK(t, filepath.Join(t.TempDir(), "jdk17"), filepath.Join(t.TempDir(), "argv.txt"))
	_, stateDirA := prepareProxyInstallDir(t, home, installDirA)
	_, stateDirB := prepareProxyInstallDir(t, home, installDirB)
	if stateDirA == stateDirB {
		t.Fatalf("install dirs must derive distinct state dirs, both got %s", stateDirA)
	}

	t.Setenv(stxJavaProxyHomeEnvVar, home)
	t.Setenv(stxJavaProxyScriptEnvVar, "")
	t.Setenv(stxJavaProxyEndpointEnvVar, "")
	t.Setenv(stxJavaProxyJarEnvVar, "")
	t.Setenv(stxJavaProxyPortEnvVar, "")
	t.Setenv(stxJavaProxyJvmOptsEnvVar, "")
	t.Setenv(stxJavaProxyExtraClasspathEnvVar, "")

	ctx := javaenv.WithHome(context.Background(), jdk)
	portA := freeTCPPort(t)

	// A：显式端口启动。 / A: start on an explicit port.
	baseA, err := ensureSTXJavaProxyService(ctx, installDirA, "2.3.13", portA)
	if err != nil {
		t.Fatalf("ensure A: %v", err)
	}
	if baseA != stxJavaProxyServiceBaseURL(portA) {
		t.Fatalf("baseA = %q", baseA)
	}
	t.Cleanup(func() { _, _ = StopManagedSTXJavaProxyService(context.Background(), installDirA) })
	t.Cleanup(func() { _, _ = StopManagedSTXJavaProxyService(context.Background(), installDirB) })

	statusA, err := GetManagedSTXJavaProxyServiceStatus(ctx, installDirA)
	if err != nil || !statusA.Running || !statusA.Healthy || statusA.PID <= 0 || statusA.Port != portA || statusA.StateDir != stateDirA {
		t.Fatalf("A status: %+v err=%v", statusA, err)
	}
	pidA := statusA.PID

	// B：显式请求 A 的端口必须报冲突，且 A 毫发无损、B 的 stateDir 不登记 A 的进程。
	// B: explicitly requesting A's port must conflict while A stays untouched and B's
	// stateDir never records A's process.
	if _, err := ensureSTXJavaProxyService(ctx, installDirB, "2.3.13", portA); err == nil || !strings.Contains(err.Error(), "another install dir") {
		t.Fatalf("expected explicit port conflict for B, got %v", err)
	}
	if !stxJavaProxyPIDAlive(pidA) {
		t.Fatal("a conflicting ensure must never kill the other cluster's process")
	}
	if got, _ := os.ReadFile(filepath.Join(stateDirB, "service.pid")); strings.TrimSpace(string(got)) == strconv.Itoa(pidA) {
		t.Fatal("B's stateDir must not record A's pid")
	}

	// B：预置一个被他集群占用的落盘端口，自动分配必须跳过它并申请空闲端口。
	// B: with a persisted port held by the other cluster, auto-assignment must skip it
	// and allocate a free port instead.
	if err := os.WriteFile(filepath.Join(stateDirB, "service.port"), []byte(strconv.Itoa(portA)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseB, err := ensureSTXJavaProxyService(ctx, installDirB, "2.3.13", 0)
	if err != nil {
		t.Fatalf("ensure B: %v", err)
	}
	if baseB == "" || baseB == baseA {
		t.Fatalf("B must get its own endpoint, got %q", baseB)
	}
	statusB, err := GetManagedSTXJavaProxyServiceStatus(ctx, installDirB)
	if err != nil || !statusB.Running || !statusB.Healthy || statusB.PID <= 0 || statusB.StateDir != stateDirB {
		t.Fatalf("B status: %+v err=%v", statusB, err)
	}
	if statusB.Port == portA || statusB.PID == pidA {
		t.Fatalf("B must run on its own port/pid, got %+v", statusB)
	}
	pidB := statusB.PID
	portB := statusB.Port
	if !stxJavaProxyPIDAlive(pidA) {
		t.Fatal("starting B must never disturb A's process")
	}

	// 重启 A（本目录旧代际视为残留先停后拉）：B 不受影响。
	// Restart A (a stale own instance is stopped then relaunched): B stays untouched.
	if _, err := ensureSTXJavaProxyService(ctx, installDirA, "2.3.13", portA); err != nil {
		t.Fatalf("re-ensure A: %v", err)
	}
	statusA, err = GetManagedSTXJavaProxyServiceStatus(ctx, installDirA)
	if err != nil || !statusA.Running || !statusA.Healthy || statusA.Port != portA {
		t.Fatalf("A after re-ensure: %+v err=%v", statusA, err)
	}
	pidA = statusA.PID
	statusB, err = GetManagedSTXJavaProxyServiceStatus(ctx, installDirB)
	if err != nil || !statusB.Running || !statusB.Healthy || statusB.PID != pidB || statusB.Port != portB {
		t.Fatalf("restarting A must not disturb B: %+v err=%v", statusB, err)
	}

	// 停 A：B 依然健康，A 的 status 不错认 B 的进程。
	// Stop A: B stays healthy and A's status never claims B's process.
	if _, err := StopManagedSTXJavaProxyService(ctx, installDirA); err != nil {
		t.Fatalf("stop A: %v", err)
	}
	if stxJavaProxyPIDAlive(pidA) {
		t.Fatalf("A's pid %d must be gone after stop", pidA)
	}
	statusA, err = GetManagedSTXJavaProxyServiceStatus(ctx, installDirA)
	if err != nil || statusA.Running || statusA.Healthy || statusA.PID != 0 {
		t.Fatalf("stopped A must report running=false healthy=false pid=0, got %+v err=%v", statusA, err)
	}
	if statusA.PID == pidB || statusA.Port == portB {
		t.Fatalf("stopped A must never claim B's process/port: %+v", statusA)
	}
	statusB, err = GetManagedSTXJavaProxyServiceStatus(ctx, installDirB)
	if err != nil || !statusB.Running || !statusB.Healthy || statusB.PID != pidB || statusB.Port != portB {
		t.Fatalf("B must stay healthy after A stops: %+v err=%v", statusB, err)
	}

	// 停 B：status 明确 running=false、healthy=false，绝不串台。
	// Stop B: status reports running=false, healthy=false — never the other cluster.
	if _, err := StopManagedSTXJavaProxyService(ctx, installDirB); err != nil {
		t.Fatalf("stop B: %v", err)
	}
	statusB, err = GetManagedSTXJavaProxyServiceStatus(ctx, installDirB)
	if err != nil || statusB.Running || statusB.Healthy || statusB.PID != 0 {
		t.Fatalf("stopped B must report running=false healthy=false pid=0, got %+v err=%v", statusB, err)
	}
}
