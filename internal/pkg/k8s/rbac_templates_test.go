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

// TestRenderMinimalRBACYAML 验证单命名空间控制面模板严格隔离 SeaTunnel 引擎运行身份，
// 且完整覆盖 RBACRequiredRules 全部必需权限而不包含 ClusterRole 或 */proxy。
// TestRenderMinimalRBACYAML verifies single-namespace control-plane YAML omits runner SA,
// covers every rule in RBACRequiredRules, and never emits ClusterRole or */proxy.
func TestRenderMinimalRBACYAML(t *testing.T) {
	raw := RenderMinimalRBACYAML("seatunnel-prod", "stx-manager")
	docs := decodeYAMLDocs(t, raw)
	var managerRole parsedYAMLDoc
	foundManagerSA := false
	for _, doc := range docs {
		if doc.Kind == "ClusterRole" || doc.Kind == "ClusterRoleBinding" {
			t.Fatalf("control-plane template must not contain %s", doc.Kind)
		}
		if doc.Kind == "ServiceAccount" && doc.Metadata.Name == DefaultSTXManagerServiceAccount {
			foundManagerSA = true
			if doc.Metadata.Namespace != "seatunnel-prod" {
				t.Fatalf("manager SA namespace=%q want seatunnel-prod", doc.Metadata.Namespace)
			}
		}
		if doc.Kind == "ServiceAccount" && doc.Metadata.Name == DefaultSeaTunnelRunnerServiceAccount {
			t.Fatalf("control-plane template must not bundle %s", DefaultSeaTunnelRunnerServiceAccount)
		}
		if doc.Kind == "Role" && doc.Metadata.Name == "stx-manager-role" {
			managerRole = doc
		}
	}
	if !foundManagerSA {
		t.Fatalf("expected %s ServiceAccount", DefaultSTXManagerServiceAccount)
	}
	for _, rule := range RBACRequiredRules {
		res := rule.Resource
		if rule.Subresource != "" {
			res = rule.Resource + "/" + rule.Subresource
		}
		if !docCoversRule(managerRole, rule.Group, res, rule.Verb) {
			t.Fatalf("control-plane role missing required rule group=%q resource=%q verb=%q", rule.Group, res, rule.Verb)
		}
	}
	// 默认控制面模板严格不含 delete、secrets，且不包含多余权限。
	// Default control-plane template omits delete, secrets, and unused rules.
	if docCoversRule(managerRole, "batch", "jobs", "delete") || docCoversRule(managerRole, "", "events", "list") || docCoversRule(managerRole, "", "pods", "create") {
		t.Fatalf("control-plane role must not contain unused batch/jobs, events, or pods create rules")
	}
	for _, grpRes := range [][2]string{{"apps", "statefulsets"}, {"apps", "deployments"}, {"", "services"}, {"", "configmaps"}, {"", "pods"}, {"", "secrets"}} {
		if docCoversRule(managerRole, grpRes[0], grpRes[1], "delete") {
			t.Fatalf("default control-plane role must not require delete on %s", grpRes[1])
		}
	}
	if docCoversRule(managerRole, "", "secrets", "get") || docCoversRule(managerRole, "", "services/proxy", "get") || docCoversRule(managerRole, "", "pods/proxy", "get") {
		t.Fatalf("default control-plane role must not include secrets or proxy subresources")
	}
	if cmd := RenderTokenCommand("seatunnel-prod"); cmd != "kubectl -n seatunnel-prod create token stx-manager --duration=8760h" {
		t.Fatalf("unexpected token command: %q", cmd)
	}

	// 可选模块组合：开启 delete/secrets 精确反映在 YAML 中，且仍不包含 runner SA 与 proxy 子资源。
	// Modular options: enabling delete/secrets updates YAML cleanly while keeping runner SA and proxy subresources out.
	bare := RenderMinimalRBACYAMLWithOptions("seatunnel-prod", "stx-manager", MinimalRBACOptions{})
	if strings.Contains(bare, DefaultSeaTunnelRunnerServiceAccount) || strings.Contains(bare, "services/proxy") || strings.Contains(bare, "pods/proxy") || strings.Contains(bare, `resources: ["secrets"]`) || strings.Contains(bare, `"delete"`) {
		t.Fatalf("expected bare control-plane YAML without optional blocks, got:\n%s", bare)
	}
	fullOpt := RenderMinimalRBACYAMLWithOptions("seatunnel-prod", "ops-stx", MinimalRBACOptions{
		IncludeDeleteRules: true,
		IncludeSecretRules: true,
	})
	if strings.Contains(fullOpt, DefaultSeaTunnelRunnerServiceAccount) {
		t.Fatalf("control-plane YAML must never include %s", DefaultSeaTunnelRunnerServiceAccount)
	}
	var optRole parsedYAMLDoc
	for _, doc := range decodeYAMLDocs(t, fullOpt) {
		if doc.Kind == "Role" && doc.Metadata.Name == "ops-stx-role" {
			optRole = doc
		}
	}
	if !docCoversRule(optRole, "apps", "statefulsets", "delete") || !docCoversRule(optRole, "", "secrets", "create") || !docCoversRule(optRole, "", "pods", "delete") {
		t.Fatalf("expected delete and secrets rules when enabled via options")
	}
	if docCoversRule(optRole, "", "services/proxy", "get") || docCoversRule(optRole, "", "pods/proxy", "get") {
		t.Fatalf("control-plane YAML must never include services/proxy or pods/proxy")
	}
	if kcCmd := RenderKubeconfigCommand("seatunnel-prod", "ops-stx"); !strings.Contains(kcCmd, "create token ops-stx") || !strings.Contains(kcCmd, "certificate-authority-data") {
		t.Fatalf("unexpected kubeconfig cmd: %q", kcCmd)
	}
}

// TestRenderSeaTunnelRunnerRBACYAML 验证引擎运行账号模板严格独立且只读。
// TestRenderSeaTunnelRunnerRBACYAML verifies standalone engine runner YAML stays read-only and isolated.
func TestRenderSeaTunnelRunnerRBACYAML(t *testing.T) {
	raw := RenderSeaTunnelRunnerRBACYAML("seatunnel-prod", "st-runner")
	if strings.Contains(raw, DefaultSTXManagerServiceAccount) || strings.Contains(raw, "ClusterRole") {
		t.Fatalf("runner template must not reference %s or ClusterRole", DefaultSTXManagerServiceAccount)
	}
	docs := decodeYAMLDocs(t, raw)
	var runnerRole parsedYAMLDoc
	foundSA, foundBinding := false, false
	for _, doc := range docs {
		if doc.Kind == "ServiceAccount" && doc.Metadata.Name == "st-runner" && doc.Metadata.Namespace == "seatunnel-prod" {
			foundSA = true
		}
		if doc.Kind == "Role" && doc.Metadata.Name == "st-runner-role" {
			runnerRole = doc
		}
		if doc.Kind == "RoleBinding" && doc.Metadata.Name == "st-runner-binding" {
			if doc.RoleRef.Name != "st-runner-role" || len(doc.Subjects) != 1 || doc.Subjects[0].Name != "st-runner" {
				t.Fatalf("unexpected runner binding: %+v", doc)
			}
			foundBinding = true
		}
	}
	if !foundSA || !foundBinding || runnerRole.Kind == "" {
		t.Fatalf("expected standalone runner SA/Role/RoleBinding")
	}
	for _, readRes := range []string{"endpoints", "pods", "services"} {
		for _, verb := range []string{"get", "list", "watch"} {
			if !docCoversRule(runnerRole, "", readRes, verb) {
				t.Fatalf("runner role missing %s %s", readRes, verb)
			}
		}
	}
	for _, forbidden := range []string{"secrets", "statefulsets", "deployments", "roles", "rolebindings"} {
		for _, v := range []string{"get", "list", "create", "update", "delete"} {
			if docCoversRule(runnerRole, "", forbidden, v) || docCoversRule(runnerRole, "apps", forbidden, v) {
				t.Fatalf("runner role must not grant %s %s", forbidden, v)
			}
		}
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
