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

// Package javaenv 是 Agent 侧唯一的 JDK 解析入口：节点配置了 java_home 时只使用该目录，
// 未配置时沿用原有的 PATH 探测。预检、SeaTunnel 启动、stx-java-proxy 与诊断工具都通过它取 JDK。
// Package javaenv is the Agent's single JDK resolution entry: a configured java_home is the only directory used,
// while an empty one keeps the original PATH detection. Precheck, SeaTunnel startup, stx-java-proxy and the
// diagnostic tools all obtain their JDK through it.
package javaenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ParamJavaHome 是控制面下发节点 JDK 路径的命令参数名。
// ParamJavaHome is the command parameter name the Control Plane uses for a node's JDK path.
const ParamJavaHome = "java_home"

// EnvJavaBin 是传给 stx-java-proxy 启动脚本的 java 可执行文件环境变量；使用 STX 专有名称，避免被 profile 覆盖。
// EnvJavaBin is the java executable env var passed to the stx-java-proxy script; the STX-specific name keeps
// login profiles from overriding it.
const EnvJavaBin = "STX_JAVA_BIN"

// Source 表示 JDK 的来源。
// Source tells where a JDK came from.
type Source string

const (
	// SourceCustom 表示使用节点配置的 java_home。/ SourceCustom means the node's configured java_home is used.
	SourceCustom Source = "custom"
	// SourceDetected 表示未配置，按原有方式自动探测。/ SourceDetected means nothing was configured and detection was used.
	SourceDetected Source = "detected"
)

// 失败原因代码，控制面与前端可据此展示。
// Failure reason codes the Control Plane and frontend can display.
const (
	ReasonNotAbsolute      = "not_absolute"
	ReasonNotFound         = "not_found"
	ReasonNotDirectory     = "not_directory"
	ReasonJavaMissing      = "java_missing"
	ReasonJavaNotExec      = "java_not_executable"
	ReasonJavaNotOnPath    = "java_not_on_path"
	ReasonToolNotAvailable = "tool_not_available"
)

// Runtime 是解析后的 JDK。
// Runtime is a resolved JDK.
type Runtime struct {
	JavaHome string
	JavaBin  string
	Source   Source
}

// ResolveError 描述 JDK 解析失败，包含具体路径与原因。
// ResolveError describes a JDK resolution failure with the concrete path and reason.
type ResolveError struct {
	Path   string
	Reason string
	Err    error
}

func (e *ResolveError) Error() string {
	var zh, en string
	switch e.Reason {
	case ReasonNotAbsolute:
		zh, en = "JDK 路径必须是绝对路径", "the JDK path must be absolute"
	case ReasonNotFound:
		zh, en = "JDK 路径不存在", "the JDK path does not exist"
	case ReasonNotDirectory:
		zh, en = "JDK 路径不是目录", "the JDK path is not a directory"
	case ReasonJavaMissing:
		zh, en = "JDK 路径下没有 bin/java", "bin/java does not exist under the JDK path"
	case ReasonJavaNotExec:
		zh, en = "bin/java 不可执行", "bin/java is not executable"
	case ReasonJavaNotOnPath:
		zh, en = "PATH 中未找到 java", "java was not found in PATH"
	default:
		zh, en = "JDK 不可用", "the JDK is not usable"
	}
	detail := ""
	if e.Err != nil {
		detail = ": " + e.Err.Error()
	}
	return fmt.Sprintf("%s：%s / %s: %s%s", zh, e.Path, en, e.Path, detail)
}

func (e *ResolveError) Unwrap() error { return e.Err }

// ReasonOf 返回错误中的原因代码；非 ResolveError 返回空字符串。
// ReasonOf returns the reason code in err; it is empty for errors that are not a ResolveError.
func ReasonOf(err error) string {
	var resolveErr *ResolveError
	if errors.As(err, &resolveErr) {
		return resolveErr.Reason
	}
	return ""
}

// NormalizeHome 去掉首尾空白并清理路径；空字符串表示未配置。
// NormalizeHome trims whitespace and cleans the path; an empty string means not configured.
func NormalizeHome(home string) string {
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Clean(home)
}

// javaExecutableName 返回当前系统的 java 可执行文件名。
// javaExecutableName returns the java executable name for the current OS.
func javaExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Resolve 解析节点 JDK：home 非空时只校验并使用 <home>/bin/java；为空时按原有方式探测 PATH 上的 java。
// Resolve resolves a node JDK: a non-empty home is validated and only <home>/bin/java is used; an empty home keeps
// the original detection of java on PATH.
func Resolve(home string) (*Runtime, error) {
	home = NormalizeHome(home)
	if home == "" {
		return Detect()
	}
	return ValidateHome(home)
}

// ValidateHome 校验配置的 JDK 目录：必须是绝对路径、存在、是目录，且 bin/java 可执行。
// ValidateHome validates a configured JDK directory: it must be absolute, exist, be a directory and contain an
// executable bin/java.
func ValidateHome(home string) (*Runtime, error) {
	home = NormalizeHome(home)
	if home == "" || !filepath.IsAbs(home) {
		return nil, &ResolveError{Path: home, Reason: ReasonNotAbsolute}
	}
	info, err := os.Stat(home)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ResolveError{Path: home, Reason: ReasonNotFound}
		}
		return nil, &ResolveError{Path: home, Reason: ReasonNotFound, Err: err}
	}
	if !info.IsDir() {
		return nil, &ResolveError{Path: home, Reason: ReasonNotDirectory}
	}
	javaBin := filepath.Join(home, "bin", javaExecutableName("java"))
	binInfo, err := os.Stat(javaBin)
	if err != nil {
		return nil, &ResolveError{Path: javaBin, Reason: ReasonJavaMissing}
	}
	if binInfo.IsDir() || !isExecutable(binInfo) {
		return nil, &ResolveError{Path: javaBin, Reason: ReasonJavaNotExec}
	}
	return &Runtime{JavaHome: home, JavaBin: javaBin, Source: SourceCustom}, nil
}

// Detect 沿用原有探测：取 Agent 进程 PATH 上的 java（SeaTunnel 默认也用它），并推导其 JAVA_HOME 用于展示与工具定位。
// Detect keeps the original detection: java on the Agent process PATH (what SeaTunnel uses by default), with its
// JAVA_HOME derived for display and tool lookup.
func Detect() (*Runtime, error) {
	javaBin, err := exec.LookPath(javaExecutableName("java"))
	if err != nil {
		return nil, &ResolveError{Path: "java", Reason: ReasonJavaNotOnPath, Err: err}
	}
	if abs, absErr := filepath.Abs(javaBin); absErr == nil {
		javaBin = abs
	}
	return &Runtime{JavaHome: deriveHome(javaBin), JavaBin: javaBin, Source: SourceDetected}, nil
}

// deriveHome 由 java 可执行文件推导 JAVA_HOME：环境变量 JAVA_HOME 指向同一个 java 时优先使用它，
// 否则取解析软链后 bin 的上级目录（并去掉 JDK 8 的 jre 子目录）。
// deriveHome derives JAVA_HOME from a java executable: env JAVA_HOME wins when it points at the same java,
// otherwise the parent of the symlink-resolved bin directory is used (dropping the JDK 8 jre sub-directory).
func deriveHome(javaBin string) string {
	real, err := filepath.EvalSymlinks(javaBin)
	if err != nil {
		real = javaBin
	}
	if envHome := NormalizeHome(os.Getenv("JAVA_HOME")); envHome != "" {
		candidate := filepath.Join(envHome, "bin", javaExecutableName("java"))
		if resolved, resolveErr := filepath.EvalSymlinks(candidate); resolveErr == nil && resolved == real {
			return envHome
		}
	}
	return homeFromBin(javaBin)
}

// Tool 返回该 JDK 下的工具路径（如 jcmd、jstack、jmap、jstat、jps）；JDK 8 的 jre 目录会回退到上级 JDK 的 bin。
// 工具不存在时返回错误，不回退到 PATH。
// Tool returns a tool path under this JDK (such as jcmd, jstack, jmap, jstat or jps); a JDK 8 jre directory falls
// back to the parent JDK bin. A missing tool is an error and never falls back to PATH.
func (r *Runtime) Tool(name string) (string, error) {
	if r == nil || strings.TrimSpace(r.JavaHome) == "" {
		return "", &ResolveError{Path: name, Reason: ReasonToolNotAvailable}
	}
	candidates := []string{filepath.Join(r.JavaHome, "bin", javaExecutableName(name))}
	if filepath.Base(r.JavaHome) == "jre" {
		candidates = append(candidates, filepath.Join(filepath.Dir(r.JavaHome), "bin", javaExecutableName(name)))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && isExecutable(info) {
			return candidate, nil
		}
	}
	return "", &ResolveError{Path: candidates[0], Reason: ReasonToolNotAvailable}
}

// LookupTool 按节点 JDK 定位诊断工具：配置了 java_home 时只取 <java_home>/bin；
// 未配置时先取探测到的 JAVA_HOME/bin，再回退到 PATH（与原有行为一致）。
// LookupTool locates a diagnostic tool for a node JDK: a configured java_home only uses <java_home>/bin; without
// one, the detected JAVA_HOME/bin is tried first and PATH is the fallback (the original behavior).
func LookupTool(javaHome, name string) (string, error) {
	javaHome = NormalizeHome(javaHome)
	if javaHome != "" {
		rt, err := ValidateHome(javaHome)
		if err != nil {
			return "", err
		}
		return rt.Tool(name)
	}
	if rt, err := Detect(); err == nil {
		if path, toolErr := rt.Tool(name); toolErr == nil {
			return path, nil
		}
	}
	path, err := exec.LookPath(javaExecutableName(name))
	if err != nil {
		return "", &ResolveError{Path: name, Reason: ReasonToolNotAvailable, Err: err}
	}
	return path, nil
}

// ToolForPID 返回运行中 JVM 自身 JDK 下的工具（读取 /proc/<pid>/exe），保证 jstat 等与目标进程同一个 JDK；
// 无法读取时回退到 LookupTool("")。
// ToolForPID returns a tool from the running JVM's own JDK (via /proc/<pid>/exe) so jstat and friends match the
// target process; it falls back to LookupTool("") when the executable cannot be read.
func ToolForPID(pid int, name string) (string, error) {
	if exe := ProcessExecutable(pid); exe != "" && filepath.Base(exe) == javaExecutableName("java") {
		rt := &Runtime{JavaHome: filepath.Dir(filepath.Dir(exe))}
		if path, toolErr := rt.Tool(name); toolErr == nil {
			return path, nil
		}
	}
	return LookupTool("", name)
}

// ApplyEnv 返回在 env 基础上把 JAVA_HOME 设为 home、并把 <home>/bin 放到 PATH 最前面的新环境；home 为空时原样返回。
// 只作用于这一个子进程，不修改 Agent 自身或全局 shell 的 JAVA_HOME。
// ApplyEnv returns a copy of env with JAVA_HOME set to home and <home>/bin prepended to PATH; an empty home returns
// env unchanged. It only affects that one child process, never the Agent's own or the global shell JAVA_HOME.
func ApplyEnv(env []string, home string) []string {
	home = NormalizeHome(home)
	if home == "" {
		return env
	}
	result := make([]string, 0, len(env)+2)
	currentPath := ""
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "JAVA_HOME":
			continue
		case "PATH":
			currentPath = value
			continue
		}
		result = append(result, entry)
	}
	binDir := filepath.Join(home, "bin")
	path := binDir
	if currentPath != "" {
		path = binDir + string(os.PathListSeparator) + currentPath
	}
	return append(result, "JAVA_HOME="+home, "PATH="+path)
}

func isExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

type contextKey struct{}

// WithHome 把控制面下发的节点 java_home 放进上下文，供同一命令内按需启动的子进程（如 stx-java-proxy）读取；
// 空值不写入。
// WithHome stores the node java_home sent by the Control Plane in the context so child processes started on
// demand within the same command (such as stx-java-proxy) can read it; empty values are not stored.
func WithHome(ctx context.Context, home string) context.Context {
	home = NormalizeHome(home)
	if ctx == nil || home == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, home)
}

// HomeFromContext 返回上下文中的节点 java_home；未设置时为空字符串。
// HomeFromContext returns the node java_home from the context; it is empty when unset.
func HomeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	home, _ := ctx.Value(contextKey{}).(string)
	return home
}

// SameExecutable 判断两个路径解析软链后是否为同一个文件；任一无法解析时返回 false。
// SameExecutable reports whether two paths resolve to the same file after following symlinks; it returns false
// when either cannot be resolved.
func SameExecutable(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return false
	}
	return ra == rb
}

// ProcessExecutable 返回运行中进程的可执行文件（Linux 读取 /proc/<pid>/exe）；无法读取时返回空字符串。
// ProcessExecutable returns a running process's executable (via /proc/<pid>/exe on Linux); it is empty when it
// cannot be read.
func ProcessExecutable(pid int) string {
	if pid <= 0 || runtime.GOOS != "linux" {
		return ""
	}
	exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(exe, " (deleted)")
}

// ShellExportPrefix 返回在 `bash -lc` 命令最前面使用的导出语句，使 profile 加载之后仍以节点 JDK 为准；home 为空时返回空字符串。
// ShellExportPrefix returns export statements to put at the front of a `bash -lc` command so the node JDK still
// wins after profiles load; an empty home returns an empty string.
func ShellExportPrefix(home string) string {
	home = NormalizeHome(home)
	if home == "" {
		return ""
	}
	quoted := "'" + strings.ReplaceAll(home, "'", `'\''`) + "'"
	return "export JAVA_HOME=" + quoted + "; export PATH=\"$JAVA_HOME/bin:$PATH\"; "
}
