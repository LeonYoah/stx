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

package version

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"v1.0.0", "1.0.0"},
		{"V1.2.3", "1.2.3"},
		{"1.0.0", "1.0.0"},
		{"  v0.1.0 ", "0.1.0"},
		{"dev", "dev"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := Normalize(tc.in); got != tc.want {
			t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()
	if Compare("v1.0.0", "1.0.0") != 0 {
		t.Fatal("equal versions should compare equal")
	}
	if Compare("1.0.0", "1.0.1") >= 0 {
		t.Fatal("1.0.0 should be below 1.0.1")
	}
	if Compare("1.1.0", "1.0.9") <= 0 {
		t.Fatal("1.1.0 should be above 1.0.9")
	}
	if Compare("1.0.0-dev", "1.0.0") >= 0 {
		t.Fatal("prerelease should sort below release")
	}
}

func TestIsBelowMin(t *testing.T) {
	t.Parallel()
	if !IsBelowMin("0.9.0", "1.0.0") {
		t.Fatal("0.9.0 should be below 1.0.0")
	}
	if IsBelowMin("1.0.0", "1.0.0") {
		t.Fatal("equal version is not below min")
	}
	if IsBelowMin("dev", "1.0.0") {
		t.Fatal("dev builds should skip enforcement")
	}
	if IsBelowMin("1.0.0-dev", "1.0.0") {
		t.Fatal("*-dev builds should skip enforcement")
	}
}
