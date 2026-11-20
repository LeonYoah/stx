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

package javaruntime

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func agentOutput(t *testing.T, success bool, message string, details map[string]string) string {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{"success": success, "message": message, "details": details})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestValidate(t *testing.T) {
	if got, err := Validate("  "); err != nil || got != "" {
		t.Fatalf("empty: %q %v", got, err)
	}
	if got, err := Validate(" /opt/jdk8/ "); err != nil || got != "/opt/jdk8" {
		t.Fatalf("normalize: %q %v", got, err)
	}
	for _, bad := range []string{"opt/jdk", "/opt/jdk\n8", "/" + strings.Repeat("a", MaxJavaHomeLength)} {
		if _, err := Validate(bad); !errors.Is(err, ErrInvalidJavaHome) {
			t.Fatalf("Validate(%q) err = %v", bad, err)
		}
	}
}

func TestCheckParamsAlwaysCarriesJavaHome(t *testing.T) {
	params := CheckParams("")
	if v, ok := params[ParamJavaHome]; !ok || v != "" || params["sub_command"] != "check_java" {
		t.Fatalf("params = %v", params)
	}
	if CheckParams("/opt/jdk8/")[ParamJavaHome] != "/opt/jdk8" {
		t.Fatal("java_home should be normalized")
	}
}

// AC2/AC3：8、11 通过，17 警告，失败带路径；结果记录实际检查的路径与版本。
// AC2/AC3: 8 and 11 pass, 17 warns, failures carry the path; results record the checked path and version.
func TestInterpretCustom(t *testing.T) {
	cases := []struct {
		version string
		status  string
	}{{"8", StatusPassed}, {"11", StatusPassed}, {"17", StatusWarning}, {"21", StatusWarning}, {"10", StatusWarning}}
	for _, tc := range cases {
		out := agentOutput(t, true, "ok", map[string]string{
			"java_home_source": "custom", "java_home": "/opt/jdk", "java_bin": "/opt/jdk/bin/java",
			"installed_version": tc.version, "version_string": tc.version + ".0.1",
		})
		r := Interpret("/opt/jdk", true, out)
		if r.Status != tc.status || r.Version == 0 || r.JavaBin != "/opt/jdk/bin/java" || r.Source != SourceCustom {
			t.Fatalf("version %s: %+v", tc.version, r)
		}
	}

	out := agentOutput(t, false, "JDK 路径不存在：/opt/none / the JDK path does not exist: /opt/none", map[string]string{
		"java_home_source": "custom", "java_home": "/opt/none", "java_bin": "/opt/none/bin/java", "error_reason": "not_found",
	})
	r := Interpret("/opt/none", false, out)
	if r.Status != StatusFailed || r.Reason != "not_found" {
		t.Fatalf("missing: %+v", r)
	}
	msg := r.FailureMessage("node-a")
	if !strings.Contains(msg, "node-a") || !strings.Contains(msg, "/opt/none/bin/java") || !strings.Contains(msg, "/opt/none") {
		t.Fatalf("failure message = %q", msg)
	}
}

// 目标版本感知矩阵：≥3.1.0 要求 11+，低于阻断；更早目标沿用 {8,11} 通过、其余警告。
// The target-aware matrix: ≥3.1.0 requires 11+, below is blocked; older targets keep
// {8,11} passed and everything else warning.
func TestStatusForTarget(t *testing.T) {
	cases := []struct {
		version int
		target  string
		want    string
	}{
		{8, "3.1.0", StatusFailed},
		{10, "3.1.0", StatusFailed},
		{11, "3.1.0", StatusPassed},
		{17, "3.1.0", StatusPassed},
		{21, "3.1.0", StatusPassed},
		{8, "3.2.0", StatusFailed},
		{8, "3.0.0", StatusPassed},
		{11, "3.0.0", StatusPassed},
		{17, "3.0.0", StatusWarning},
		{8, "2.3.12", StatusPassed},
		{17, "2.3.12", StatusWarning},
		// 版本未知（0/-1）保守警告，不阻断。/ Unknown versions degrade to warning, never block.
		{0, "3.1.0", StatusWarning},
		{-1, "3.1.0", StatusWarning},
		// 空目标沿用旧矩阵。/ Empty target keeps the legacy matrix.
		{8, "", StatusPassed},
		{17, "", StatusWarning},
	}
	for _, tc := range cases {
		if got := StatusForTarget(tc.version, tc.target); got != tc.want {
			t.Fatalf("StatusForTarget(%d, %q) = %s, want %s", tc.version, tc.target, got, tc.want)
		}
	}
}

func TestMinSupportedMajor(t *testing.T) {
	for _, tc := range []struct {
		target string
		want   int
	}{
		{"3.1.0", 11}, {"3.1.1", 11}, {"3.2.0", 11}, {"4.0.0", 11},
		{"3.0.0", 8}, {"2.3.12", 8}, {"", 8},
	} {
		if got := MinSupportedMajor(tc.target); got != tc.want {
			t.Fatalf("MinSupportedMajor(%q) = %d, want %d", tc.target, got, tc.want)
		}
	}
}

// 旧 Agent 忽略 java_home 时必须失败，避免静默用错 JDK。
// An old Agent that ignores java_home must fail so the wrong JDK is never used silently.
func TestInterpretOldAgentFailsForCustom(t *testing.T) {
	out := agentOutput(t, true, "ok", map[string]string{"installed_version": "17"})
	r := Interpret("/opt/jdk8", true, out)
	if r.Status != StatusFailed || r.Reason != ReasonAgentUnsupported || r.JavaHome != "/opt/jdk8" {
		t.Fatalf("old agent: %+v", r)
	}
	// 未指定路径时旧 Agent 行为不变。/ Without a path an old Agent behaves as before.
	if r := Interpret("", true, out); r.Status != StatusWarning || r.Source != SourceDetected {
		t.Fatalf("old agent detected: %+v", r)
	}
}
