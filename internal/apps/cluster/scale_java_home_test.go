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

package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func (env *scaleTestEnv) javaCheckParams(agentID string) []map[string]string {
	env.agent.mu.Lock()
	defer env.agent.mu.Unlock()
	found := make([]map[string]string, 0)
	for _, command := range env.agent.commands {
		if command.agentID == agentID && command.commandType == "check_java" {
			found = append(found, command.params)
		}
	}
	return found
}

// TestPlanScaleOutCustomJavaHome 覆盖 AC1/AC2/AC3：扩容指定 JDK 时只检查该路径，检查项带实际 java 路径，计划目标带上该路径。
// TestPlanScaleOutCustomJavaHome covers AC1/AC2/AC3: a scale-out with a JDK checks only that path, the check shows
// the actual java binary, and the plan target carries the path.
func TestPlanScaleOutCustomJavaHome(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2, JavaHome: "/opt/jdk8/"})
	if plan.Target.JavaHome != "/opt/jdk8" {
		t.Fatalf("target java_home = %q", plan.Target.JavaHome)
	}
	checks := findCheck(plan, "java")
	if len(checks) != 1 || checks[0].Status != ScaleCheckPassed || checks[0].Target != "/opt/jdk8/bin/java" || !strings.Contains(checks[0].Message, "/opt/jdk8/bin/java") {
		t.Fatalf("java check = %+v", checks)
	}
	if params := env.javaCheckParams("agent-2"); len(params) != 1 || params[0]["java_home"] != "/opt/jdk8" {
		t.Fatalf("check_java params = %v", params)
	}
	if !plan.CanSubmit {
		t.Fatalf("plan should be submittable: %v", plan.BlockingReasons)
	}
}

// TestPlanScaleOutCustomJavaHomeVersionAndFailure 覆盖 AC2/AC1c：非 8/11 警告不阻断；路径缺失阻断并带上节点与路径。
// TestPlanScaleOutCustomJavaHomeVersionAndFailure covers AC2/AC1c: versions other than 8/11 warn without blocking;
// a missing path blocks naming the node and path.
func TestPlanScaleOutCustomJavaHomeVersionAndFailure(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.agent.javaVersion["agent-2"] = "17"
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2, JavaHome: "/opt/jdk17"})
	if checks := findCheck(plan, "java"); len(checks) != 1 || checks[0].Status != ScaleCheckWarning || !plan.CanSubmit {
		t.Fatalf("java 17 should warn without blocking: checks=%+v can_submit=%v", checks, plan.CanSubmit)
	}

	env.agent.javaFail["agent-2"] = true
	plan = env.mustPlanOut(t, ScaleOutRequest{HostID: 2, JavaHome: "/opt/missing"})
	checks := findCheck(plan, "java")
	if len(checks) != 1 || checks[0].Status != ScaleCheckFailed || !checks[0].Blocking || plan.CanSubmit {
		t.Fatalf("missing JDK should block: checks=%+v can_submit=%v", checks, plan.CanSubmit)
	}
	if !strings.Contains(checks[0].Message, "/opt/missing/bin/java") || !strings.Contains(checks[0].Message, "t2") {
		t.Fatalf("failure message should name node and path: %s", checks[0].Message)
	}
}

// TestPlanScaleOutDoesNotInheritPeerJavaHome 覆盖 AC1d/AC6：已有节点指定了 JDK，新节点未指定时不继承、仍走自动探测。
// TestPlanScaleOutDoesNotInheritPeerJavaHome covers AC1d/AC6: an existing node has a JDK, and a new node without
// one does not inherit it and keeps auto detection.
func TestPlanScaleOutDoesNotInheritPeerJavaHome(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	nodes, _ := env.svc.repo.GetNodesByClusterID(ctx, env.cluster.ID)
	nodes[0].JavaHome = "/opt/jdk-old"
	if err := env.svc.repo.UpdateNode(ctx, nodes[0]); err != nil {
		t.Fatalf("update peer: %v", err)
	}
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if plan.Target.JavaHome != "" {
		t.Fatalf("new node must not inherit the peer JDK, got %q", plan.Target.JavaHome)
	}
	params := env.javaCheckParams("agent-2")
	if len(params) != 1 {
		t.Fatalf("check_java calls = %v", params)
	}
	if _, ok := params[0]["java_home"]; ok {
		t.Fatalf("auto detection must keep the original empty params, got %v", params[0])
	}
	if checks := findCheck(plan, "java"); len(checks) != 1 || checks[0].Status != ScaleCheckPassed {
		t.Fatalf("java check = %+v", checks)
	}
}

// TestPlanScaleOutJavaHomeFingerprintAndValidation：JDK 参与计划指纹；非法路径作为无效请求拒绝。
// TestPlanScaleOutJavaHomeFingerprintAndValidation: the JDK is part of the plan fingerprint; invalid paths are
// rejected as invalid requests.
func TestPlanScaleOutJavaHomeFingerprintAndValidation(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	auto := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	custom := env.mustPlanOut(t, ScaleOutRequest{HostID: 2, JavaHome: "/opt/jdk8"})
	if auto.Fingerprint == custom.Fingerprint {
		t.Fatal("java_home must change the plan fingerprint")
	}
	_, err := env.scale.PlanScaleOut(context.Background(), env.cluster.ID, &ScaleOutRequest{HostID: 2, JavaHome: "opt/jdk8"}, "en")
	if !errors.Is(err, ErrScaleInvalidRequest) {
		t.Fatalf("relative java_home should be an invalid request, got %v", err)
	}
}

// TestScaleOutInstallPersistsJavaHome 覆盖 AC4/AC4c：扩容执行把 JDK 交给安装器，并写入新节点记录（唯一来源）。
// TestScaleOutInstallPersistsJavaHome covers AC4/AC4c: scale-out passes the JDK to the installer and writes it to
// the new node record (the single source).
func TestScaleOutInstallPersistsJavaHome(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.joinOnStart("agent-2", []string{"10.0.0.1:5801", "10.0.0.2:5801"})
	outReq := ScaleOutRequest{HostID: 2, JavaHome: "/opt/jdk8"}
	plan := env.mustPlanOut(t, outReq)
	req := &ScaleOutSubmitRequest{ScaleOutRequest: outReq, PlanFingerprint: plan.Fingerprint}
	meta := ScaleExecutionRequest{RequestID: "req-jdk", IdempotencyKey: "key-jdk", RequestHash: "hash-jdk"}
	result, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID, req, meta)
	if err != nil {
		t.Fatalf("SubmitScaleOut: %v", err)
	}
	if op := env.loadOp(t, result.OperationID); op.Status != ScaleOperationSucceeded {
		t.Fatalf("operation should succeed, got %s: %s", op.Status, op.Error)
	}
	if len(env.installer.requests) != 1 || env.installer.requests[0].JavaHome != "/opt/jdk8" {
		t.Fatalf("installer requests = %+v", env.installer.requests)
	}
	node, err := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
	if err != nil || node == nil || node.JavaHome != "/opt/jdk8" {
		t.Fatalf("new node java_home = %+v err=%v", node, err)
	}
	peer, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 1, string(NodeRoleMasterWorker))
	if peer.JavaHome != "" {
		t.Fatalf("existing node must stay untouched (AC5), got %q", peer.JavaHome)
	}
}

// TestGetNodeJavaRuntime 覆盖 AC7：详情页查询按节点记录的 JDK 检查，并返回实际路径与版本；未指定时为自动探测。
// TestGetNodeJavaRuntime covers AC7: the detail query checks the JDK from the node record and returns the actual
// path and version; without one it reports auto detection.
func TestGetNodeJavaRuntime(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	nodes, _ := env.svc.repo.GetNodesByClusterID(ctx, env.cluster.ID)
	var custom, auto *ClusterNode
	for _, node := range nodes {
		if node.HostID == 1 {
			custom = node
		} else {
			auto = node
		}
	}
	custom.JavaHome = "/opt/jdk8"
	if err := env.svc.repo.UpdateNode(ctx, custom); err != nil {
		t.Fatalf("update node: %v", err)
	}

	got, err := env.svc.GetNodeJavaRuntime(ctx, env.cluster.ID, custom.ID)
	if err != nil {
		t.Fatalf("GetNodeJavaRuntime: %v", err)
	}
	if got.Configured != "/opt/jdk8" || got.JavaBin != "/opt/jdk8/bin/java" || got.Version != 8 || got.Source != "custom" || got.Status != "passed" {
		t.Fatalf("custom runtime = %+v", got)
	}

	got, err = env.svc.GetNodeJavaRuntime(ctx, env.cluster.ID, auto.ID)
	if err != nil {
		t.Fatalf("GetNodeJavaRuntime auto: %v", err)
	}
	if got.Configured != "" || got.Source != "detected" {
		t.Fatalf("auto runtime = %+v", got)
	}
	if params := env.javaCheckParams("agent-2"); len(params) != 1 || params[0]["java_home"] != "" {
		t.Fatalf("auto check must send an explicit empty java_home: %v", params)
	}

	if _, err := env.svc.GetNodeJavaRuntime(ctx, env.cluster.ID+100, custom.ID); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("wrong cluster should be not found, got %v", err)
	}
}

// TestServiceAddNodeJavaHome 覆盖 AC1：手动新增节点可带 JDK 并写入节点记录，非法路径被拒绝。
// TestServiceAddNodeJavaHome covers AC1: a manually added node can carry a JDK stored on the node record, and
// invalid paths are rejected.
func TestServiceAddNodeJavaHome(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	node, err := env.svc.AddNode(ctx, env.cluster.ID, &AddNodeRequest{HostID: 2, Role: NodeRoleMasterWorker, InstallDir: scaleTestInstallDir, HazelcastPort: 5801, JavaHome: "/opt/jdk11/", SkipPrecheck: true})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	stored, _ := env.svc.repo.GetNodeByID(ctx, node.ID)
	if stored.JavaHome != "/opt/jdk11" {
		t.Fatalf("stored java_home = %q", stored.JavaHome)
	}
	if _, err := env.svc.AddNode(ctx, env.cluster.ID, &AddNodeRequest{HostID: 3, Role: NodeRoleMasterWorker, InstallDir: scaleTestInstallDir, HazelcastPort: 5801, JavaHome: "jdk", SkipPrecheck: true}); err == nil {
		t.Fatal("relative java_home must be rejected")
	}
}
