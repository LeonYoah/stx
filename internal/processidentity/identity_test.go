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
package processidentity

import "testing"

func TestManagedNameSeparatesInstallDirectories(t *testing.T) {
	first := ManagedName("/tmp/seatunnel-2.3.12", "master/worker")
	second := ManagedName("/tmp/seatunnel-2.3.13", "master/worker")
	if first == second {
		t.Fatalf("不同安装目录生成了相同进程名: %q", first)
	}
	if got := ManagedName("/tmp/seatunnel-2.3.12", "master"); got == first {
		t.Fatalf("不同角色生成了相同进程名: %q", got)
	}
}

func TestManagedNameNormalizesHybridRole(t *testing.T) {
	want := ManagedName("/opt/seatunnel", "")
	for _, role := range []string{"hybrid", "master/worker", " HYBRID "} {
		if got := ManagedName("/opt/seatunnel", role); got != want {
			t.Fatalf("角色 %q 的进程名为 %q，期望 %q", role, got, want)
		}
	}
}
