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

package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// distributionHazelcastYAML 模拟 SeaTunnel 发行包风格的 hazelcast.yaml（License 注释、空行、2 空格缩进）。
// distributionHazelcastYAML mimics a SeaTunnel distribution hazelcast.yaml (license comment, blank lines,
// 2-space indent).
const distributionHazelcastYAML = `#
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.
#

hazelcast:
  cluster-name: seatunnel
  network:
    rest-api:
      enabled: false
      endpoint-groups:
        CLUSTER_WRITE:
          enabled: true
    join:
      tcp-ip:
        enabled: true
        # 成员列表 / members
        member-list:
          - localhost # default
    port:
      auto-increment: false
      port: 5801

  properties:
    hazelcast.invocation.max.retry.count: 20
    hazelcast.logging.type: log4j2
`

func TestPatchMemberListContentReplacesOnlyMemberListLines(t *testing.T) {
	patched, changed, err := PatchMemberListContent(distributionHazelcastYAML, []string{"10.66.0.1:5801", "10.66.0.2:5801"})
	if err != nil {
		t.Fatalf("PatchMemberListContent: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	want := strings.Replace(distributionHazelcastYAML,
		"          - localhost # default\n",
		"          - 10.66.0.1:5801\n          - 10.66.0.2:5801\n", 1)
	if patched != want {
		t.Fatalf("patched content differs beyond member-list:\n--- got ---\n%s\n--- want ---\n%s", patched, want)
	}
	members, err := ReadMemberList(patched)
	if err != nil || !reflect.DeepEqual(members, []string{"10.66.0.1:5801", "10.66.0.2:5801"}) {
		t.Fatalf("ReadMemberList = %v, %v", members, err)
	}
}

func TestPatchMemberListContentUnchangedWhenMembersMatch(t *testing.T) {
	content := "hazelcast:\n  network:\n    join:\n      tcp-ip:\n        member-list:\n          - 10.0.0.1:5801\n          - 10.0.0.2:5801\n"
	patched, changed, err := PatchMemberListContent(content, []string{" 10.0.0.1:5801 ", "10.0.0.2:5801"})
	if err != nil {
		t.Fatalf("PatchMemberListContent: %v", err)
	}
	if changed || patched != content {
		t.Fatalf("expected unchanged content, changed=%v patched=%q", changed, patched)
	}
}

func TestPatchMemberListContentKeepsFourSpaceInstallerLayout(t *testing.T) {
	// 安装器用 yaml.Marshal 写出 4 空格缩进的文件。
	// The installer writes 4-space indented files via yaml.Marshal.
	content := "hazelcast:\n    network:\n        join:\n            tcp-ip:\n                enabled: true\n                member-list:\n                    - 10.66.0.2:5801\n        port:\n            port: 5801\n"
	patched, changed, err := PatchMemberListContent(content, []string{"10.66.0.1:5801", "10.66.0.2:5801"})
	if err != nil || !changed {
		t.Fatalf("PatchMemberListContent: changed=%v err=%v", changed, err)
	}
	want := "hazelcast:\n    network:\n        join:\n            tcp-ip:\n                enabled: true\n                member-list:\n                    - 10.66.0.1:5801\n                    - 10.66.0.2:5801\n        port:\n            port: 5801\n"
	if patched != want {
		t.Fatalf("got:\n%s\nwant:\n%s", patched, want)
	}
}

func TestPatchMemberListContentFlowSequence(t *testing.T) {
	content := "hazelcast:\n  network:\n    join:\n      tcp-ip:\n        member-list: [localhost] # keep\n        enabled: true\n"
	patched, changed, err := PatchMemberListContent(content, []string{"a:5801", "b:5801"})
	if err != nil || !changed {
		t.Fatalf("PatchMemberListContent: changed=%v err=%v", changed, err)
	}
	want := "hazelcast:\n  network:\n    join:\n      tcp-ip:\n        member-list: [a:5801, b:5801] # keep\n        enabled: true\n"
	if patched != want {
		t.Fatalf("got:\n%s\nwant:\n%s", patched, want)
	}
}

func TestPatchMemberListContentImplicitNullValue(t *testing.T) {
	content := "hazelcast:\n  network:\n    join:\n      tcp-ip:\n        member-list:\n        enabled: true\n"
	patched, changed, err := PatchMemberListContent(content, []string{"a:5801"})
	if err != nil || !changed {
		t.Fatalf("PatchMemberListContent: changed=%v err=%v", changed, err)
	}
	want := "hazelcast:\n  network:\n    join:\n      tcp-ip:\n        member-list:\n          - a:5801\n        enabled: true\n"
	if patched != want {
		t.Fatalf("got:\n%s\nwant:\n%s", patched, want)
	}
}

func TestPatchMemberListContentPreservesCRLF(t *testing.T) {
	content := "hazelcast:\r\n  network:\r\n    join:\r\n      tcp-ip:\r\n        member-list:\r\n          - localhost\r\n    port:\r\n      port: 5801\r\n"
	patched, changed, err := PatchMemberListContent(content, []string{"a:5801", "b:5801"})
	if err != nil || !changed {
		t.Fatalf("PatchMemberListContent: changed=%v err=%v", changed, err)
	}
	want := "hazelcast:\r\n  network:\r\n    join:\r\n      tcp-ip:\r\n        member-list:\r\n          - a:5801\r\n          - b:5801\r\n    port:\r\n      port: 5801\r\n"
	if patched != want {
		t.Fatalf("got %q\nwant %q", patched, want)
	}
}

func TestPatchMemberListContentLastLineWithoutNewline(t *testing.T) {
	content := "hazelcast:\n  network:\n    join:\n      tcp-ip:\n        member-list:\n          - localhost"
	patched, _, err := PatchMemberListContent(content, []string{"a:5801", "b:5801"})
	if err != nil {
		t.Fatalf("PatchMemberListContent: %v", err)
	}
	want := "hazelcast:\n  network:\n    join:\n      tcp-ip:\n        member-list:\n          - a:5801\n          - b:5801"
	if patched != want {
		t.Fatalf("got %q\nwant %q", patched, want)
	}
}

func TestPatchMemberListContentFallsBackForScalarValue(t *testing.T) {
	// 标量值无法按行替换，走 YAML 节点重写，仍须只改 member-list。
	// A scalar value cannot be spliced; the YAML rewrite fallback must still change only member-list.
	content := "hazelcast:\n  cluster-name: seatunnel\n  network:\n    join:\n      tcp-ip:\n        member-list: localhost\n"
	patched, changed, err := PatchMemberListContent(content, []string{"a:5801"})
	if err != nil || !changed {
		t.Fatalf("PatchMemberListContent: changed=%v err=%v", changed, err)
	}
	members, err := ReadMemberList(patched)
	if err != nil || !reflect.DeepEqual(members, []string{"a:5801"}) {
		t.Fatalf("ReadMemberList = %v, %v", members, err)
	}
	if !strings.Contains(patched, "cluster-name: seatunnel") {
		t.Fatalf("other fields lost: %s", patched)
	}
}

func TestPatchMemberListContentErrors(t *testing.T) {
	if _, _, err := PatchMemberListContent(distributionHazelcastYAML, []string{" ", ""}); !errors.Is(err, ErrEmptyMemberList) {
		t.Fatalf("expected ErrEmptyMemberList, got %v", err)
	}
	if _, _, err := PatchMemberListContent("hazelcast:\n  network:\n    port:\n      port: 5801\n", []string{"a:5801"}); !errors.Is(err, ErrMemberListNotFound) {
		t.Fatalf("expected ErrMemberListNotFound, got %v", err)
	}
	if _, _, err := PatchMemberListContent("hazelcast: [", []string{"a:5801"}); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestMemberListConfigTypes(t *testing.T) {
	if got := MemberListConfigTypes("hybrid"); !reflect.DeepEqual(got, []ConfigType{ConfigTypeHazelcast}) {
		t.Fatalf("hybrid types = %v", got)
	}
	want := []ConfigType{ConfigTypeHazelcastMaster, ConfigTypeHazelcastWorker, ConfigTypeHazelcast}
	if got := MemberListConfigTypes("separated"); !reflect.DeepEqual(got, want) {
		t.Fatalf("separated types = %v", got)
	}
	if got := MemberListConfigTypes(""); !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown-mode types = %v", got)
	}
}
