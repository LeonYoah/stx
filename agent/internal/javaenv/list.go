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
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// CandidateSource 表示列表中某一项 JDK 的来源标签，供前端区分环境变量、扫描目录与 PATH。
// CandidateSource is the source label of a listed JDK so the UI can tell env, scanned dirs and PATH apart.
type CandidateSource string

const (
	// CandidateSourceEnv 来自环境变量 JAVA_HOME。/ CandidateSourceEnv comes from the JAVA_HOME environment variable.
	CandidateSourceEnv CandidateSource = "env"
	// CandidateSourceScan 来自固定扫描目录。/ CandidateSourceScan comes from the fixed scan directories.
	CandidateSourceScan CandidateSource = "scan"
	// CandidateSourcePath 来自 PATH 上的 java（Detect）。/ CandidateSourcePath comes from java on PATH (Detect).
	CandidateSourcePath CandidateSource = "path"
	// CandidateSourceProcess 来自运行中进程的可执行文件。/ CandidateSourceProcess comes from a running process executable.
	CandidateSourceProcess CandidateSource = "process"
)

// Candidate 是探测到的一个可用 JDK（路径 + 版本），供安装/扩容/纳管向导下拉选择（AC1b / AC1b2）。
// Candidate is one detected usable JDK (path + version) for the install/scale/register wizard dropdown (AC1b / AC1b2).
type Candidate struct {
	JavaHome      string          `json:"java_home"`
	JavaBin       string          `json:"java_bin"`
	Version       int             `json:"version"`
	VersionString string          `json:"version_string,omitempty"`
	Source        CandidateSource `json:"source"`
}

// scanGlobs 与 Agent 启动包装脚本中的扫描目录保持一致（AC1b2）。
// scanGlobs matches the scan directories in the Agent startup wrapper (AC1b2).
func scanGlobs() []string {
	globs := []string{
		"/usr/lib/jvm/java-*",
		"/usr/java/*",
		"/opt/java/*",
		"/usr/local/java*",
	}
	if runtime.GOOS == "darwin" {
		globs = append(globs, "/Library/Java/JavaVirtualMachines/*/Contents/Home")
	}
	return globs
}

// List 列出本机 STX 能读到的 JDK：环境 JAVA_HOME、固定扫描目录、以及 PATH 上的 java。
// 探测失败或某个目录无效时跳过该项，返回空列表也不报错（AC1b：不阻塞）。
// List lists JDKs STX can read on this host: env JAVA_HOME, the fixed scan directories, and java on PATH.
// Invalid entries are skipped; an empty list is not an error (AC1b: never blocks).
func List(ctx context.Context) []Candidate {
	if ctx == nil {
		ctx = context.Background()
	}
	seen := make(map[string]struct{})
	var out []Candidate

	add := func(home string, source CandidateSource) {
		home = NormalizeHome(home)
		if home == "" {
			return
		}
		if _, ok := seen[home]; ok {
			return
		}
		rt, err := ValidateHome(home)
		if err != nil {
			return
		}
		version, versionStr, _ := VersionOf(ctx, rt.JavaBin)
		seen[home] = struct{}{}
		out = append(out, Candidate{
			JavaHome:      rt.JavaHome,
			JavaBin:       rt.JavaBin,
			Version:       version,
			VersionString: versionStr,
			Source:        source,
		})
	}

	// 1) 环境 JAVA_HOME 优先展示。/ Prefer env JAVA_HOME in the list.
	add(os.Getenv("JAVA_HOME"), CandidateSourceEnv)

	// 2) 固定扫描目录。/ Fixed scan directories.
	for _, pattern := range scanGlobs() {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		sort.Strings(matches)
		for _, match := range matches {
			add(match, CandidateSourceScan)
		}
	}

	// 3) PATH 上的 java（与 Detect 一致）。/ java on PATH (same as Detect).
	if rt, err := Detect(); err == nil {
		add(rt.JavaHome, CandidateSourcePath)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		return out[i].JavaHome < out[j].JavaHome
	})
	return out
}

// FromProcess 由运行中 JVM 的可执行文件推导 JDK（读 /proc/<pid>/exe）；解析不到返回 (nil, false)（AC1f）。
// 不采用环境 JAVA_HOME，只用进程自己的 java 路径。
// FromProcess derives the JDK from a running JVM executable (via /proc/<pid>/exe); it returns (nil, false) when
// resolution fails (AC1f). Env JAVA_HOME is ignored; only the process's own java path is used.
func FromProcess(pid int) (*Runtime, bool) {
	exe := ProcessExecutable(pid)
	if exe == "" || filepath.Base(exe) != javaExecutableName("java") {
		return nil, false
	}
	home := homeFromBin(exe)
	if home == "" {
		return nil, false
	}
	if _, err := ValidateHome(home); err != nil {
		// 进程 java 可能是 jre 软链；仍返回推导结果供预选。/ The process java may be a jre symlink; still return the derived home for preselection.
		return &Runtime{JavaHome: home, JavaBin: exe, Source: SourceDetected}, true
	}
	return &Runtime{JavaHome: home, JavaBin: exe, Source: SourceDetected}, true
}

// VersionOf 执行 `<javaBin> -version` 并解析主版本号；失败时返回 0。
// VersionOf runs `<javaBin> -version` and parses the major version; failures return 0.
func VersionOf(ctx context.Context, javaBin string) (int, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(javaBin) == "" {
		return 0, "", &ResolveError{Path: "java", Reason: ReasonJavaMissing}
	}
	output, err := exec.CommandContext(ctx, javaBin, "-version").CombinedOutput()
	if err != nil {
		return 0, "", err
	}
	return parseVersionOutput(string(output))
}

var versionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`version "1\.(\d+)`),
	regexp.MustCompile(`version "(\d+)`),
}

func parseVersionOutput(output string) (int, string, error) {
	trimmed := strings.TrimSpace(output)
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		for _, re := range versionPatterns {
			if m := re.FindStringSubmatch(line); len(m) > 1 {
				v, err := strconv.Atoi(m[1])
				if err != nil {
					continue
				}
				return v, line, nil
			}
		}
	}
	return 0, trimmed, &ResolveError{Path: "java", Reason: ReasonJavaMissing}
}

// homeFromBin 只由 java 可执行文件路径推导 JAVA_HOME，不考虑环境变量（供进程解析使用）。
// homeFromBin derives JAVA_HOME only from the java executable path, ignoring the environment (for process resolution).
func homeFromBin(javaBin string) string {
	real, err := filepath.EvalSymlinks(javaBin)
	if err != nil {
		real = javaBin
	}
	home := filepath.Dir(filepath.Dir(real))
	if filepath.Base(home) == "jre" {
		parent := filepath.Dir(home)
		if _, statErr := os.Stat(filepath.Join(parent, "bin", javaExecutableName("java"))); statErr == nil {
			return parent
		}
	}
	return home
}
