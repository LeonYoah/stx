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

func TestValidateReleaseVersion(t *testing.T) {
	valid := []string{"1.2.3", "v1.2.3", "V10.20.30", "1.1.2-beta.1", "v2.0.0-rc1", "v1.0.0-alpha.2.3"}
	for _, v := range valid {
		if err := ValidateReleaseVersion(v); err != nil {
			t.Errorf("ValidateReleaseVersion(%q) = %v, want nil", v, err)
		}
	}
	invalid := []string{
		"", " ", "..", "../1.2.3", "1.2.3/..", "v1.2.3/../../etc", "/etc/passwd", "/1.2.3",
		`..\1.2.3`, `1.2.3\x`, `C:\1.2.3`, "C:1.2.3", "1.2", "1.2.3.4", "v", "latest",
		"1.2.3-", "1.2.3-..", "1.2.3-beta/1", " 1.2.3", "1.2.3 ", "1.2.3\n", "1.2.3-beta_1", "vv1.2.3",
	}
	for _, v := range invalid {
		if err := ValidateReleaseVersion(v); err == nil {
			t.Errorf("ValidateReleaseVersion(%q) = nil, want error", v)
		}
	}
}
