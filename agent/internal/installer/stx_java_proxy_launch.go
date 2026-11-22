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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/agent/internal/javaenv"
	processmanager "github.com/LeonYoah/stx/agent/internal/process"
)

// stxJavaProxyMainClass 是 stx-java-proxy 的入口类，与 scripts/stx-java-proxy.sh 的 APP_MAIN 一致。
// stxJavaProxyMainClass is the proxy entry class, identical to APP_MAIN in scripts/stx-java-proxy.sh.
const stxJavaProxyMainClass = "io.github.leonyoah.stx.proxy.StxJavaProxyApplication"

// stxJavaProxyExtraClasspathEnvVar 兼容脚本的 EXTRA_PROXY_CLASSPATH 追加路径。
// stxJavaProxyExtraClasspathEnvVar keeps parity with the script's EXTRA_PROXY_CLASSPATH append.
const stxJavaProxyExtraClasspathEnvVar = "EXTRA_PROXY_CLASSPATH"

// stxJavaProxyClasspath 组装与脚本一致的 classpath：<installDir>/lib/*:starter.jar:<proxy jar>，
// 末尾兼容 EXTRA_PROXY_CLASSPATH。
// stxJavaProxyClasspath builds the same classpath as the script:
// <installDir>/lib/*:starter.jar:<proxy jar>, honoring EXTRA_PROXY_CLASSPATH at the end.
func stxJavaProxyClasspath(installDir, jarPath string) string {
	classpath := strings.Join([]string{
		filepath.Join(installDir, "lib", "*"),
		filepath.Join(installDir, "starter", "seatunnel-starter.jar"),
		jarPath,
	}, string(os.PathListSeparator))
	if extra := strings.TrimSpace(os.Getenv(stxJavaProxyExtraClasspathEnvVar)); extra != "" {
		classpath += string(os.PathListSeparator) + extra
	}
	return classpath
}

// stxJavaProxyJVMArgs 组装 JVM 参数：未出现 -Xmx 时注入与脚本相同的默认堆 -Xms64m -Xmx512m。
// stxJavaProxyJVMArgs assembles JVM arguments: when no -Xmx is present it injects the same
// default heap as the script (-Xms64m -Xmx512m).
func stxJavaProxyJVMArgs(jvmOpts string) []string {
	args := strings.Fields(jvmOpts)
	hasXmx := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-Xmx") {
			hasXmx = true
			break
		}
	}
	if !hasXmx {
		args = append([]string{"-Xms64m", "-Xmx512m"}, args...)
	}
	return args
}

// buildSTXJavaProxyJavaArgv 组装 <java_home>/bin/java 的完整 argv：JVM 参数 + 端口 / SEATUNNEL_HOME
// 系统属性 + -cp + 主类，与脚本最终 exec 的命令行等价。
// buildSTXJavaProxyJavaArgv builds the full argv for <java_home>/bin/java: JVM flags + port and
// SEATUNNEL_HOME system properties + -cp + main class, equivalent to the script's final exec line.
func buildSTXJavaProxyJavaArgv(installDir, jarPath string, port int, jvmOpts string) []string {
	argv := stxJavaProxyJVMArgs(jvmOpts)
	argv = append(argv,
		fmt.Sprintf("-Dstx.java.proxy.port=%d", port),
		"-DSEATUNNEL_HOME="+installDir,
		"-Dstx.java.proxy.seatunnel.home="+installDir,
		"-cp", stxJavaProxyClasspath(installDir, jarPath),
		stxJavaProxyMainClass,
	)
	return argv
}

// startSTXJavaProxyServiceDirect 不走脚本，直接用已解析的 java 二进制拉起代理：
// stdout/stderr 追加到 stateDir/service.log，stdin 指向 /dev/null，进程脱离会话后立即落盘
// service.pid / service.port / service.jvm_opts，再等健康端点。
// startSTXJavaProxyServiceDirect launches the proxy with the resolved java binary without the
// script: stdout/stderr append to stateDir/service.log, stdin is /dev/null, and after the
// detached start it persists service.pid / service.port / service.jvm_opts before waiting for health.
func startSTXJavaProxyServiceDirect(
	ctx context.Context,
	installDir string,
	jarPath string,
	stateDir string,
	port int,
	jvmOpts string,
	seatunnelVersion string,
) (string, error) {
	installDir = strings.TrimSpace(installDir)
	if installDir == "" {
		return "", fmt.Errorf("stx-java-proxy install dir is empty")
	}
	starterJar := filepath.Join(installDir, "starter", "seatunnel-starter.jar")
	if !fileExists(starterJar) {
		return "", fmt.Errorf("seatunnel-starter.jar not found under %s/starter", installDir)
	}
	if strings.TrimSpace(jarPath) == "" || !fileExists(jarPath) {
		return "", fmt.Errorf("stx-java-proxy jar is unavailable: %s", jarPath)
	}
	rt, err := javaenv.Resolve(javaenv.HomeFromContext(ctx))
	if err != nil || rt == nil || strings.TrimSpace(rt.JavaBin) == "" {
		if err == nil {
			err = fmt.Errorf("no java runtime resolved")
		}
		return "", fmt.Errorf("resolve java for stx-java-proxy: %w", err)
	}
	// 自定义 JDK 的 JavaBin 即 <java_home>/bin/java；自动探测时是 PATH 上解析到的 java。
	// For a custom JDK JavaBin is <java_home>/bin/java; for detection it is the PATH-resolved java.
	javaBin := rt.JavaBin

	logPath := filepath.Join(stateDir, "service.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("open stx-java-proxy log file: %w", err)
	}
	defer logFile.Close()
	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()

	cmd := exec.Command(javaBin, buildSTXJavaProxyJavaArgv(installDir, jarPath, port, jvmOpts)...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = devNull
	processmanager.SetDetachedSession(cmd)
	env := stxJavaProxyServiceEnv(installDir, jarPath, port, seatunnelVersion, jvmOpts)
	env = append(env, fmt.Sprintf("%s=%s", javaenv.EnvJavaBin, javaBin))
	if rt.JavaHome != "" {
		env = append(env, "JAVA_HOME="+rt.JavaHome)
	}
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start managed stx-java-proxy service via %s: %w", javaBin, err)
	}
	pid := cmd.Process.Pid
	// 立即落盘运行态，健康检查失败也保留现场便于诊断。
	// Persist runtime state at once so a failed health check still leaves a diagnosable record.
	_ = os.WriteFile(filepath.Join(stateDir, "service.pid"), []byte(strconv.Itoa(pid)+"\n"), 0o644)
	if err := os.WriteFile(filepath.Join(stateDir, "service.port"), []byte(strconv.Itoa(port)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("persist stx-java-proxy port: %w", err)
	}
	_ = os.WriteFile(filepath.Join(stateDir, "service.jvm_opts"), []byte(jvmOpts+"\n"), 0o644)
	// 后台回收子进程：Release() 会让退出的 java 变成僵尸，kill(0) 仍判活、stop 探测失真；
	// goroutine 里 Wait() 在退出时及时收尸，进程本身仍由 setsid 脱离会话。
	// Reap the child in the background: after Release() an exited java stays a zombie so kill(0)
	// still reports alive and stop detection breaks; Wait() in a goroutine reaps it on exit while
	// the process stays detached through setsid.
	go func() {
		_ = cmd.Wait()
	}()

	baseURL := stxJavaProxyServiceBaseURL(port)
	if err := waitForSTXJavaProxyHealthy(ctx, baseURL, stxJavaProxyStartupWait); err != nil {
		return "", fmt.Errorf("wait for managed stx-java-proxy service on %s: %w", baseURL, err)
	}
	return baseURL, nil
}

// stxJavaProxyServiceEnv 组装代理子进程环境：与脚本路径一致（SEATUNNEL_HOME、代理 home、jar、
// 端口、代际与生效的 JVM 参数）。
// stxJavaProxyServiceEnv assembles the child environment identically to the script path
// (SEATUNNEL_HOME, proxy home, jar, port, epoch and effective JVM opts).
func stxJavaProxyServiceEnv(installDir, jarPath string, port int, seatunnelVersion, jvmOpts string) []string {
	env := append(
		os.Environ(),
		fmt.Sprintf("SEATUNNEL_HOME=%s", installDir),
		fmt.Sprintf("%s=%s", stxJavaProxyHomeEnvVar, resolveSTXJavaProxyHome(installDir)),
		fmt.Sprintf("%s=%s", stxJavaProxyJarEnvVar, jarPath),
		fmt.Sprintf("%s=%d", stxJavaProxyPortEnvVar, port),
		fmt.Sprintf("%s=%s", stxJavaProxyVersionEnvVar, defaultSTXJavaProxyVersion(seatunnelVersion)),
	)
	if strings.TrimSpace(jvmOpts) != "" {
		env = append(env, fmt.Sprintf("%s=%s", stxJavaProxyJvmOptsEnvVar, jvmOpts))
	}
	return env
}
