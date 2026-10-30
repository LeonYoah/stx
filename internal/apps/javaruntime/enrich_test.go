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
	"context"
	"errors"
	"testing"
)

func staticLookup(homes []string, err error) (NodeJavaHomeLookup, *int, *string) {
	calls := 0
	seenDir := ""
	return func(_ context.Context, _ string, installDir string) ([]string, error) {
		calls++
		seenDir = installDir
		return homes, err
	}, &calls, &seenDir
}

func TestEnrichParamsInjectsNodeJavaHome(t *testing.T) {
	lookup, _, seenDir := staticLookup([]string{"/opt/jdk8", "/opt/jdk8/"}, nil)
	got := EnrichParams(context.Background(), "agent-1", map[string]string{"install_dir": "/opt/seatunnel/"}, lookup)
	if got[ParamJavaHome] != "/opt/jdk8" {
		t.Fatalf("java_home = %q, want /opt/jdk8", got[ParamJavaHome])
	}
	if *seenDir != "/opt/seatunnel" {
		t.Fatalf("lookup dir = %q, want cleaned /opt/seatunnel", *seenDir)
	}
}

// TestEnrichParamsNoopCases 覆盖 AC6：未指定 JDK、显式传值、无安装目录、歧义或查询失败都不补参数。
// TestEnrichParamsNoopCases covers AC6: no JDK set, explicit value, no install dir, ambiguity or lookup errors add nothing.
func TestEnrichParamsNoopCases(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]string
		homes  []string
		err    error
		calls  int
	}{
		{name: "auto node", params: map[string]string{"install_dir": "/opt/st"}, homes: []string{""}, calls: 1},
		{name: "explicit empty wins", params: map[string]string{"install_dir": "/opt/st", "java_home": ""}, homes: []string{"/opt/jdk8"}},
		{name: "explicit value wins", params: map[string]string{"install_dir": "/opt/st", "java_home": "/opt/jdk11"}, homes: []string{"/opt/jdk8"}},
		{name: "no install dir", params: map[string]string{"sub_command": "check_java"}, homes: []string{"/opt/jdk8"}},
		{name: "no node", params: map[string]string{"install_dir": "/opt/st"}, calls: 1},
		{name: "ambiguous", params: map[string]string{"install_dir": "/opt/st"}, homes: []string{"/opt/jdk8", "/opt/jdk11"}, calls: 1},
		{name: "mixed auto and custom", params: map[string]string{"install_dir": "/opt/st"}, homes: []string{"", "/opt/jdk8"}, calls: 1},
		{name: "lookup error", params: map[string]string{"install_dir": "/opt/st"}, homes: []string{"/opt/jdk8"}, err: errors.New("db down"), calls: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup, calls, _ := staticLookup(tc.homes, tc.err)
			if got := EnrichParams(context.Background(), "agent-1", tc.params, lookup); got != nil {
				t.Fatalf("EnrichParams() = %v, want nil", got)
			}
			if *calls != tc.calls {
				t.Fatalf("lookup calls = %d, want %d", *calls, tc.calls)
			}
		})
	}
}
