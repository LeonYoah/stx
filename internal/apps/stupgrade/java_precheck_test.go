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

package stupgrade

import (
	"context"
	"testing"

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
)

func newJavaPrecheckService(t *testing.T, sender *stubAgentCommandSender, pkgVersion string) *Service {
	t.Helper()
	service := newPlanningService(t, nil)
	packagePath := createTestPackage(t, map[string]string{"config/seatunnel.yaml": "seatunnel: default"})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version: pkgVersion, FileName: "apache-seatunnel-" + pkgVersion + "-bin.tar.gz", IsLocal: true,
		LocalPath: packagePath, FileSize: 1024, Checksum: "abc123",
	}})
	service.SetAgentCommandSender(sender)
	return service
}

func countCheckJavaCalls(sender *stubAgentCommandSender) int {
	count := 0
	for _, cmd := range sender.commands {
		if cmd.commandType == "check_java" {
			count++
		}
	}
	return count
}

func hasIssueCode(issues []BlockingIssue, code string) *BlockingIssue {
	for i := range issues {
		if issues[i].Code == code {
			return &issues[i]
		}
	}
	return nil
}

// TestRunPrecheckJDKBelowRequiredBlocksFor31 覆盖 3.1.0 的核心约束：节点 JDK 8 必须在预检阻断，
// 而不是拖到引擎启动才被上游脚本 fail-fast。
// TestRunPrecheckJDKBelowRequiredBlocksFor31 covers the 3.1.0 constraint: a node on JDK 8 must be
// blocked at precheck instead of dying later at the engine launcher.
func TestRunPrecheckJDKBelowRequiredBlocksFor31(t *testing.T) {
	sender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}, javaVersion: 8}
	service := newJavaPrecheckService(t, sender, "3.1.0")

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{ClusterID: 1, TargetVersion: "3.1.0"})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if countCheckJavaCalls(sender) != 1 {
		t.Fatalf("expected one check_java probe, got %+v", sender.commands)
	}
	issue := hasIssueCode(result.Issues, "jdk_below_required")
	if issue == nil || !issue.Blocking {
		t.Fatalf("expected blocking jdk_below_required issue, got %+v", result.Issues)
	}
	if issue.Metadata["detected_java"] != "8" || issue.Metadata["required_java"] != "11+" {
		t.Fatalf("issue metadata = %+v", issue.Metadata)
	}
	if result.Ready {
		t.Fatal("precheck must not be ready with a JDK below the target requirement")
	}
}

// TestRunPrecheckJDK11And17PassFor31 覆盖 3.1.0 下 JDK 11/17 均应通过，不应再被 8/11 矩阵误伤。
// TestRunPrecheckJDK11And17PassFor31 verifies JDK 11 and 17 both pass for 3.1.0 instead of being
// caught by the legacy {8,11} matrix.
func TestRunPrecheckJDK11And17PassFor31(t *testing.T) {
	for _, version := range []int{11, 17} {
		sender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}, javaVersion: version}
		service := newJavaPrecheckService(t, sender, "3.1.0")

		result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{ClusterID: 1, TargetVersion: "3.1.0"})
		if err != nil {
			t.Fatalf("RunPrecheck returned error: %v", err)
		}
		for _, issue := range result.Issues {
			if issue.Code == "jdk_below_required" || issue.Code == "java_unusable" || issue.Code == "jdk_unusual_version" {
				t.Fatalf("java %d: unexpected issue %+v", version, issue)
			}
		}
	}
}

// TestRunPrecheckLegacyMatrixForOlderTargets 覆盖旧目标的兼容性：2.3.12 仍按 {8,11} 判定，
// JDK 8 通过、JDK 17 只警告不阻断。
// TestRunPrecheckLegacyMatrixForOlderTargets keeps the {8,11} matrix for older targets: JDK 8
// passes and JDK 17 only warns without blocking.
func TestRunPrecheckLegacyMatrixForOlderTargets(t *testing.T) {
	sender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}, javaVersion: 8}
	service := newJavaPrecheckService(t, sender, "2.3.12")
	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{ClusterID: 1, TargetVersion: "2.3.12"})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if issue := hasIssueCode(result.Issues, "jdk_below_required"); issue != nil {
		t.Fatalf("jdk 8 must stay valid for 2.3.12: %+v", issue)
	}

	sender17 := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}, javaVersion: 17}
	service17 := newJavaPrecheckService(t, sender17, "2.3.12")
	result17, err := service17.RunPrecheck(context.Background(), &PrecheckRequest{ClusterID: 1, TargetVersion: "2.3.12"})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	issue := hasIssueCode(result17.Issues, "jdk_unusual_version")
	if issue == nil || issue.Blocking {
		t.Fatalf("jdk 17 on 2.3.12 should be a non-blocking warning, got %+v", result17.Issues)
	}
}

// TestRunPrecheckUnusableJDKBlocks 覆盖 JDK 完全不可用（路径缺失/执行失败）时的阻断。
// TestRunPrecheckUnusableJDKBlocks covers blocking when the JDK is unusable at all
// (missing path or exec failure).
func TestRunPrecheckUnusableJDKBlocks(t *testing.T) {
	sender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}, failSubCommand: "check_java"}
	service := newJavaPrecheckService(t, sender, "3.1.0")

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{ClusterID: 1, TargetVersion: "3.1.0"})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	issue := hasIssueCode(result.Issues, "java_unusable")
	if issue == nil || !issue.Blocking {
		t.Fatalf("expected blocking java_unusable issue, got %+v", result.Issues)
	}
	if result.Ready {
		t.Fatal("precheck must not be ready without a usable JDK")
	}
}

// TestRunPrecheckSkipsJavaCheckWhenOffline 覆盖 Agent 不在线时不下发 check_java（已由 node_offline 拦截）。
// TestRunPrecheckSkipsJavaCheckWhenOffline verifies no check_java probe is sent when the node
// is offline (already blocked by node_offline).
func TestRunPrecheckSkipsJavaCheckWhenOffline(t *testing.T) {
	sender := &stubAgentCommandSender{agents: map[uint]string{}}
	service := newJavaPrecheckService(t, sender, "3.1.0")
	// 让节点离线。/ Take the node offline.
	if provider, ok := service.clusterProvider.(*stubClusterProvider); ok {
		provider.nodes[0].IsOnline = false
	}

	if _, err := service.RunPrecheck(context.Background(), &PrecheckRequest{ClusterID: 1, TargetVersion: "3.1.0"}); err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if countCheckJavaCalls(sender) != 0 {
		t.Fatalf("offline node must not be probed, got %+v", sender.commands)
	}
}

func lastCheckJavaHome(sender *stubAgentCommandSender) string {
	for i := len(sender.commands) - 1; i >= 0; i-- {
		if sender.commands[i].commandType == "check_java" || sender.commands[i].params["sub_command"] == "check_java" {
			return sender.commands[i].params["java_home"]
		}
	}
	return ""
}

func setNodeJavaHome(t *testing.T, service *Service, javaHome string) {
	t.Helper()
	provider, ok := service.clusterProvider.(*stubClusterProvider)
	if !ok || len(provider.nodes) == 0 {
		t.Fatal("expected stub cluster provider with nodes")
	}
	provider.nodes[0].JavaHome = javaHome
}

// TestRunPrecheckTargetJavaHomeOverridesNode 覆盖升级页目标 JDK：当前节点仍是 8 时，请求级
// java_home=21 应让 3.1.0 预检通过，且不改写 source_java_home。
// TestRunPrecheckTargetJavaHomeOverridesNode covers the upgrade-page target JDK: with the node
// still on 8, request java_home=21 must let 3.1.0 precheck pass without rewriting source_java_home.
func TestRunPrecheckTargetJavaHomeOverridesNode(t *testing.T) {
	sender := &stubAgentCommandSender{
		agents:            map[uint]string{101: "agent-node-a"},
		javaVersion:       8,
		javaVersionByHome: map[string]int{"/opt/jdk8": 8, "/opt/jdk21": 21},
	}
	service := newJavaPrecheckService(t, sender, "3.1.0")
	setNodeJavaHome(t, service, "/opt/jdk8")

	blocked, err := service.RunPrecheck(context.Background(), &PrecheckRequest{ClusterID: 1, TargetVersion: "3.1.0"})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if hasIssueCode(blocked.Issues, "jdk_below_required") == nil {
		t.Fatalf("expected jdk_below_required when using the node JDK 8, got %+v", blocked.Issues)
	}
	if lastCheckJavaHome(sender) != "/opt/jdk8" {
		t.Fatalf("first probe should use node java_home, got %q", lastCheckJavaHome(sender))
	}

	passed, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:     1,
		TargetVersion: "3.1.0",
		JavaHome:      "/opt/jdk21",
	})
	if err != nil {
		t.Fatalf("RunPrecheck with target JDK returned error: %v", err)
	}
	if issue := hasIssueCode(passed.Issues, "jdk_below_required"); issue != nil {
		t.Fatalf("target JDK 21 must pass 3.1.0 precheck, got %+v", issue)
	}
	if lastCheckJavaHome(sender) != "/opt/jdk21" {
		t.Fatalf("override probe should use /opt/jdk21, got %q", lastCheckJavaHome(sender))
	}
	if len(passed.NodeTargets) != 1 {
		t.Fatalf("expected 1 node target, got %d", len(passed.NodeTargets))
	}
	if passed.NodeTargets[0].JavaHome != "/opt/jdk21" {
		t.Fatalf("target java_home = %q, want /opt/jdk21", passed.NodeTargets[0].JavaHome)
	}
	if derefStringPtr(passed.NodeTargets[0].SourceJavaHome) != "/opt/jdk8" {
		t.Fatalf("source java_home = %q, want /opt/jdk8", derefStringPtr(passed.NodeTargets[0].SourceJavaHome))
	}
}

// TestRunPrecheckNodeJavaHomeEmptyMeansAuto 覆盖节点级空串：显式自动探测，不再回落当前节点 JDK 8。
// TestRunPrecheckNodeJavaHomeEmptyMeansAuto covers a per-node empty string: explicit auto detection
// that does not fall back to the current node JDK 8.
func TestRunPrecheckNodeJavaHomeEmptyMeansAuto(t *testing.T) {
	sender := &stubAgentCommandSender{
		agents:      map[uint]string{101: "agent-node-a"},
		javaVersion: 21,
	}
	service := newJavaPrecheckService(t, sender, "3.1.0")
	setNodeJavaHome(t, service, "/opt/jdk8")

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:     1,
		TargetVersion: "3.1.0",
		NodeJavaHomes: map[uint]string{11: ""},
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if lastCheckJavaHome(sender) != "" {
		t.Fatalf("auto override must probe with empty java_home, got %q", lastCheckJavaHome(sender))
	}
	if issue := hasIssueCode(result.Issues, "jdk_below_required"); issue != nil {
		t.Fatalf("auto-detected JDK 21 must pass, got %+v", issue)
	}
	if result.NodeTargets[0].JavaHome != "" {
		t.Fatalf("target java_home should stay empty for auto, got %q", result.NodeTargets[0].JavaHome)
	}
	if derefStringPtr(result.NodeTargets[0].SourceJavaHome) != "/opt/jdk8" {
		t.Fatalf("source java_home = %q, want /opt/jdk8", derefStringPtr(result.NodeTargets[0].SourceJavaHome))
	}
}

// TestRunPrecheckInvalidTargetJavaHomeBlocks 覆盖非法目标路径：阻断且不下发 check_java。
// TestRunPrecheckInvalidTargetJavaHomeBlocks covers an invalid target path: block and skip check_java.
func TestRunPrecheckInvalidTargetJavaHomeBlocks(t *testing.T) {
	sender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}, javaVersion: 21}
	service := newJavaPrecheckService(t, sender, "3.1.0")

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID:     1,
		TargetVersion: "3.1.0",
		JavaHome:      "opt/jdk21",
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	issue := hasIssueCode(result.Issues, "jdk_path_invalid")
	if issue == nil || !issue.Blocking {
		t.Fatalf("expected blocking jdk_path_invalid, got %+v", result.Issues)
	}
	if countCheckJavaCalls(sender) != 0 {
		t.Fatalf("invalid path must not be probed, got %+v", sender.commands)
	}
}
