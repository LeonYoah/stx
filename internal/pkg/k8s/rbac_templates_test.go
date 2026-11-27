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

package k8s

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type parsedYAMLDoc struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Rules []struct {
		APIGroups []string `yaml:"apiGroups"`
		Resources []string `yaml:"resources"`
		Verbs     []string `yaml:"verbs"`
	} `yaml:"rules"`
	RoleRef struct {
		Kind string `yaml:"kind"`
		Name string `yaml:"name"`
	} `yaml:"roleRef"`
	Subjects []struct {
		Kind      string `yaml:"kind"`
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"subjects"`
}

func decodeYAMLDocs(t *testing.T, content string) []parsedYAMLDoc {
	t.Helper()
	dec := yaml.NewDecoder(bytes.NewBufferString(content))
	var docs []parsedYAMLDoc
	for {
		var doc parsedYAMLDoc
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode yaml failed: %v", err)
		}
		if doc.Kind != "" {
			docs = append(docs, doc)
		}
	}
	return docs
}

func docCoversRule(doc parsedYAMLDoc, group, resource string, verb string) bool {
	for _, r := range doc.Rules {
		hasGroup := false
		for _, g := range r.APIGroups {
			if g == group {
				hasGroup = true
				break
			}
		}
		if !hasGroup {
			continue
		}
		hasResource := false
		for _, res := range r.Resources {
			if res == resource {
				hasResource = true
				break
			}
		}
		if !hasResource {
			continue
		}
		for _, v := range r.Verbs {
			if v == verb {
				return true
			}
		}
	}
	return false
}

// TestRenderMinimalRBACYAML 验证单命名空间最小权限模板隔离 STX 控制面与 SeaTunnel 运行身份，
// 且完整覆盖 RBACRequiredRules 全部必需权限而不包含 ClusterRole。
// TestRenderMinimalRBACYAML verifies the minimal single-namespace YAML isolates STX manager
// from SeaTunnel runner, covers every rule in RBACRequiredRules, and emits no ClusterRole.
func TestRenderMinimalRBACYAML(t *testing.T) {
	raw := RenderMinimalRBACYAML("seatunnel-prod", "st-runner")
	docs := decodeYAMLDocs(t, raw)
	var managerRole, runnerRole parsedYAMLDoc
	foundManagerSA, foundRunnerSA := false, false
	for _, doc := range docs {
		if doc.Kind == "ClusterRole" || doc.Kind == "ClusterRoleBinding" {
			t.Fatalf("minimal template must not contain %s", doc.Kind)
		}
		if doc.Kind == "ServiceAccount" && doc.Metadata.Name == DefaultSTXManagerServiceAccount {
			foundManagerSA = true
			if doc.Metadata.Namespace != "seatunnel-prod" {
				t.Fatalf("manager SA namespace=%q want seatunnel-prod", doc.Metadata.Namespace)
			}
		}
		if doc.Kind == "ServiceAccount" && doc.Metadata.Name == "st-runner" {
			foundRunnerSA = true
		}
		if doc.Kind == "Role" && doc.Metadata.Name == "stx-manager-role" {
			managerRole = doc
		}
		if doc.Kind == "Role" && doc.Metadata.Name == "st-runner-role" {
			runnerRole = doc
		}
	}
	if !foundManagerSA || !foundRunnerSA {
		t.Fatalf("expected both stx-manager and st-runner ServiceAccounts")
	}
	for _, rule := range RBACRequiredRules {
		res := rule.Resource
		if rule.Subresource != "" {
			res = rule.Resource + "/" + rule.Subresource
		}
		if !docCoversRule(managerRole, rule.Group, res, rule.Verb) {
			t.Fatalf("minimal stx-manager role missing required rule group=%q resource=%q verb=%q", rule.Group, res, rule.Verb)
		}
	}
	// 运行账号严格只读 endpoints/pods/services，不可修改 secrets/statefulsets 等资源。
	// Runner role must remain strictly read-only over endpoints/pods/services.
	for _, forbidden := range []string{"secrets", "statefulsets", "deployments", "roles", "rolebindings"} {
		for _, v := range []string{"get", "list", "create", "update", "delete"} {
			if docCoversRule(runnerRole, "", forbidden, v) || docCoversRule(runnerRole, "apps", forbidden, v) {
				t.Fatalf("runner role must not grant %s %s", forbidden, v)
			}
		}
	}
	for _, readRes := range []string{"endpoints", "pods", "services"} {
		for _, verb := range []string{"get", "list", "watch"} {
			if !docCoversRule(runnerRole, "", readRes, verb) {
				t.Fatalf("runner role missing %s %s", readRes, verb)
			}
		}
	}
	if cmd := RenderTokenCommand("seatunnel-prod"); cmd != "kubectl -n seatunnel-prod create token stx-manager --duration=8760h" {
		t.Fatalf("unexpected token command: %q", cmd)
	}
}

// TestRenderFullRBACYAML 验证全面权限模板完整覆盖 RBACRequiredRules 与 RBACExtendedRules。
// TestRenderFullRBACYAML verifies the full YAML covers both required and extended RBAC rules.
func TestRenderFullRBACYAML(t *testing.T) {
	raw := RenderFullRBACYAML("seatunnel")
	docs := decodeYAMLDocs(t, raw)
	var clusterRole parsedYAMLDoc
	for _, doc := range docs {
		if doc.Kind == "ClusterRole" && doc.Metadata.Name == "stx-manager-cluster-role" {
			clusterRole = doc
		}
	}
	if clusterRole.Kind == "" {
		t.Fatal("missing ClusterRole stx-manager-cluster-role")
	}
	for _, rule := range RBACRequiredRules {
		res := rule.Resource
		if rule.Subresource != "" {
			res = rule.Resource + "/" + rule.Subresource
		}
		if !docCoversRule(clusterRole, rule.Group, res, rule.Verb) {
			t.Fatalf("full ClusterRole missing required rule group=%q resource=%q verb=%q", rule.Group, res, rule.Verb)
		}
	}
	for _, rule := range RBACExtendedRules {
		res := rule.Resource
		if rule.Subresource != "" {
			res = rule.Resource + "/" + rule.Subresource
		}
		if !docCoversRule(clusterRole, rule.Group, res, rule.Verb) {
			t.Fatalf("full ClusterRole missing extended rule group=%q resource=%q verb=%q", rule.Group, res, rule.Verb)
		}
	}

	// 验证自定义控制面 SA 时，ServiceAccount / ClusterRole / ClusterRoleBinding / roleRef 全部联动一致。
	// Verify custom control-plane SA keeps ServiceAccount, ClusterRole, ClusterRoleBinding, and roleRef in lockstep.
	customFull := RenderFullRBACYAML("seatunnel-full", "stx-manager-full")
	customDocs := decodeYAMLDocs(t, customFull)
	foundRole, foundBinding := false, false
	for _, doc := range customDocs {
		if doc.Kind == "ClusterRole" && doc.Metadata.Name == "stx-manager-full-cluster-role" {
			foundRole = true
		}
		if doc.Kind == "ClusterRoleBinding" && doc.Metadata.Name == "stx-manager-full-cluster-binding" {
			if doc.RoleRef.Name != "stx-manager-full-cluster-role" {
				t.Fatalf("roleRef.name=%q, want stx-manager-full-cluster-role", doc.RoleRef.Name)
			}
			if len(doc.Subjects) != 1 || doc.Subjects[0].Name != "stx-manager-full" {
				t.Fatalf("unexpected subjects: %+v", doc.Subjects)
			}
			foundBinding = true
		}
	}
	if !foundRole || !foundBinding {
		t.Fatalf("custom full template missing synced ClusterRole/Binding: role=%v binding=%v", foundRole, foundBinding)
	}
	if cmd := RenderTokenCommand("seatunnel-full", "stx-manager-full"); cmd != "kubectl -n seatunnel-full create token stx-manager-full --duration=8760h" {
		t.Fatalf("unexpected custom token cmd: %q", cmd)
	}
	if kcCmd := RenderKubeconfigCommand("seatunnel-full", "stx-manager-full"); !strings.Contains(kcCmd, "create token stx-manager-full") || !strings.Contains(kcCmd, "certificate-authority-data") {
		t.Fatalf("unexpected kubeconfig cmd: %q", kcCmd)
	}
}

// TestParseBearerTokenServiceAccountAndErrorClassification 验证 JWT 身份解析与错误码分类。
// TestParseBearerTokenServiceAccountAndErrorClassification verifies JWT sub parsing and error classification.
func TestParseBearerTokenServiceAccountAndErrorClassification(t *testing.T) {
	// eyJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6c2VhdHVubmVsLWZ1bGw6c3R4LW1hbmFnZXItZnVsbCJ9
	// = {"sub":"system:serviceaccount:seatunnel-full:stx-manager-full"}
	jwt := "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6c2VhdHVubmVsLWZ1bGw6c3R4LW1hbmFnZXItZnVsbCJ9.sig"
	ns, sa := ParseBearerTokenServiceAccount(jwt)
	if ns != "seatunnel-full" || sa != "stx-manager-full" {
		t.Fatalf("got (%q, %q), want (seatunnel-full, stx-manager-full)", ns, sa)
	}
	if ns2, sa2 := ParseBearerTokenServiceAccount("truncated-only-one-segment"); ns2 != "" || sa2 != "" {
		t.Fatalf("expected empty for truncated token, got (%q, %q)", ns2, sa2)
	}
	if got := ClassifyK8sError(context.DeadlineExceeded); got != "timeout" {
		t.Fatalf("DeadlineExceeded -> %q, want timeout", got)
	}
	if got := ClassifyK8sError(apierrors.NewUnauthorized("the server has asked for the client to provide credentials")); got != "unauthorized" {
		t.Fatalf("Unauthorized -> %q, want unauthorized", got)
	}
}

// TestDeployAndUninstall_SingleNamespaceMinimalRBAC 验证单命名空间最小权限下：
// 1) Namespaces().Get 返回 403 时 Deploy 不中断，且 CreateRBAC=false 不创建 Role/RoleBinding/SA；
// 2) 指定 runnerSA（或留空）准确写入 StatefulSet ServiceAccountName；
// 3) Uninstall 遇到 Roles/RoleBindings/SA 403 Forbidden 时容忍跳过并完成其余工作负载回收。
// TestDeployAndUninstall_SingleNamespaceMinimalRBAC verifies single-NS minimal RBAC:
// 1) 403 on Namespaces().Get does not fail Deploy, and CreateRBAC=false skips RBAC creation;
// 2) runner SA or empty SA maps accurately to StatefulSet.Spec.Template.Spec.ServiceAccountName;
// 3) Uninstall ignores 403 on optional per-cluster RBAC cleanup.
func TestDeployAndUninstall_SingleNamespaceMinimalRBAC(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultSeaTunnelRunnerServiceAccount, Namespace: "ns-min"},
	})
	client.PrependReactor("get", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "ns-min", nil)
	})
	for _, res := range []string{"roles", "rolebindings", "serviceaccounts"} {
		resourceName := res
		client.PrependReactor("create", resourceName, func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: resourceName}, "forbidden", nil)
		})
	}
	client.PrependReactor("get", "roles", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: rbacv1.GroupName, Resource: "roles"}, "forbidden", nil)
	})
	client.PrependReactor("get", "rolebindings", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: rbacv1.GroupName, Resource: "rolebindings"}, "forbidden", nil)
	})

	mgr := newTestManager(client)
	sm := NewSessionManager(mgr)
	cfg := testSessionConfig()
	cfg.Namespace = "ns-min"
	cfg.ServiceAccount = DefaultSeaTunnelRunnerServiceAccount

	if _, err := sm.Deploy(context.Background(), &DeployOptions{
		Config:      cfg,
		ClusterCode: "min",
		ClusterID:   9,
		CreateRBAC:  false,
	}); err != nil {
		t.Fatalf("deploy under single-namespace minimal RBAC failed: %v", err)
	}

	masterSTS, err := client.AppsV1().StatefulSets("ns-min").Get(context.Background(), "min-master", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get master sts: %v", err)
	}
	if masterSTS.Spec.Template.Spec.ServiceAccountName != DefaultSeaTunnelRunnerServiceAccount {
		t.Fatalf("master SA=%q want %q", masterSTS.Spec.Template.Spec.ServiceAccountName, DefaultSeaTunnelRunnerServiceAccount)
	}

	// Uninstall 必须容忍 Roles/RoleBindings 403 并成功删除工作负载。
	// Uninstall must tolerate 403 on Roles/RoleBindings and finish reclaiming workloads.
	if err := sm.Uninstall(context.Background(), cfg, "min"); err != nil {
		t.Fatalf("uninstall under single-namespace minimal RBAC failed: %v", err)
	}
	if _, err := client.AppsV1().StatefulSets("ns-min").Get(context.Background(), "min-master", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected master statefulset to be deleted, got %v", err)
	}
	// 预建的 runner SA 未带 stx 归属标签，不得被删除。
	// Pre-created runner SA without STX ownership labels must stay untouched.
	if _, err := client.CoreV1().ServiceAccounts("ns-min").Get(context.Background(), DefaultSeaTunnelRunnerServiceAccount, metav1.GetOptions{}); err != nil {
		t.Fatalf("runner SA must remain intact after uninstall: %v", err)
	}

	// Empty SA with CreateRBAC=false keeps ServiceAccountName empty (uses namespace default).
	cfgEmptySA := testSessionConfig()
	cfgEmptySA.Namespace = "ns-min"
	cfgEmptySA.ServiceAccount = ""
	if _, err := sm.Deploy(context.Background(), &DeployOptions{
		Config:      cfgEmptySA,
		ClusterCode: "min2",
		ClusterID:   10,
		CreateRBAC:  false,
	}); err != nil {
		t.Fatalf("deploy with empty SA and CreateRBAC=false failed: %v", err)
	}
	sts2, err := client.AppsV1().StatefulSets("ns-min").Get(context.Background(), "min2-master", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get sts2: %v", err)
	}
	if strings.TrimSpace(sts2.Spec.Template.Spec.ServiceAccountName) != "" {
		t.Fatalf("expected empty ServiceAccountName when CreateRBAC=false and SA unset, got %q", sts2.Spec.Template.Spec.ServiceAccountName)
	}
}
