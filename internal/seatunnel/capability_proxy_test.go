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

package seatunnel

import "testing"

func TestProxyEpochForVersion(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "", want: STXJavaProxyEpochV2},
		{input: "2.3.13", want: STXJavaProxyEpochV2},
		{input: "2.3.99", want: STXJavaProxyEpochV2},
		{input: "3.0.0", want: STXJavaProxyEpochV3},
		{input: "3.0.0-SNAPSHOT", want: STXJavaProxyEpochV3},
		{input: "3.1.0", want: STXJavaProxyEpochV3},
		{input: "v3.0.0", want: STXJavaProxyEpochV3},
		{input: "v2", want: STXJavaProxyEpochV2},
		{input: "v3", want: STXJavaProxyEpochV3},
	}

	for _, tc := range cases {
		got := ProxyEpochForVersion(tc.input)
		if got != tc.want {
			t.Fatalf("ProxyEpochForVersion(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSTXJavaProxyJarFileName(t *testing.T) {
	if got := STXJavaProxyJarFileName("3.0.0"); got != "stx-java-proxy-v3.jar" {
		t.Fatalf("unexpected jar name for 3.0.0: %s", got)
	}
	if got := STXJavaProxyJarFileName("v3"); got != "stx-java-proxy-v3.jar" {
		t.Fatalf("unexpected jar name for epoch v3: %s", got)
	}
	if got := STXJavaProxyJarFileName("2.3.13"); got != "stx-java-proxy-v2.jar" {
		t.Fatalf("unexpected jar name for 2.3.13: %s", got)
	}
}
