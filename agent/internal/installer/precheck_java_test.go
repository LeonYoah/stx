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
	"strings"
	"testing"
)

// CheckJava 版本矩阵：8/11 通过、12~23 警告、24+ 硬失败（JEP 486 移除 Subject.getSubject，
// SeaTunnel 3.x 内嵌 Hadoop 启动必崩）。
// The CheckJava version matrix: 8/11 pass, 12-23 warn, 24+ hard-fail (JEP 486 removed
// Subject.getSubject, so the embedded Hadoop in SeaTunnel 3.x crashes at startup).
func TestCheckJavaVersionMatrix(t *testing.T) {
	cases := []struct {
		version int
		want    CheckStatus
	}{
		{8, CheckStatusPassed},
		{11, CheckStatusPassed},
		{12, CheckStatusWarning},
		{17, CheckStatusWarning},
		{21, CheckStatusWarning},
		{23, CheckStatusWarning},
		{24, CheckStatusFailed},
		{25, CheckStatusFailed},
		{26, CheckStatusFailed},
		{7, CheckStatusFailed},
	}
	for _, tc := range cases {
		prechecker := NewPrecheckerWithProvider(nil, &MockSystemInfoProvider{
			JavaVersion:       tc.version,
			JavaVersionString: "test-jdk",
		})
		item := prechecker.CheckJava(context.Background())
		if item.Status != tc.want {
			t.Fatalf("Java %d: status = %s, want %s", tc.version, item.Status, tc.want)
		}
		if got := item.Details["max_supported_version"]; got != JavaMaxSupportedVersion {
			t.Fatalf("Java %d: max_supported_version = %v, want %d", tc.version, got, JavaMaxSupportedVersion)
		}
	}
}

// JDK 24+ 的失败信息必须点明 JEP 486 与 Subject.getSubject，方便用户理解为何被硬拦截。
// The JDK 24+ failure message must name JEP 486 and Subject.getSubject so users understand
// why the install is blocked.
func TestCheckJavaAboveSupportedExplainsJEP486(t *testing.T) {
	prechecker := NewPrecheckerWithProvider(nil, &MockSystemInfoProvider{
		JavaVersion:       25,
		JavaVersionString: "25.0.1",
	})
	item := prechecker.CheckJava(context.Background())
	if item.Status != CheckStatusFailed {
		t.Fatalf("status = %s, want failed", item.Status)
	}
	for _, want := range []string{"JEP 486", "Subject.getSubject", "23"} {
		if !strings.Contains(item.Message, want) {
			t.Fatalf("message %q should contain %q", item.Message, want)
		}
	}
}
