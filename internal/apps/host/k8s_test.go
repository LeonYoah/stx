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

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/utils/secretbox"
	"github.com/gin-gonic/gin"
	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

func setupK8sHostHandlerForTest(t *testing.T, clientset *fake.Clientset) (*Handler, *Service, uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	secretbox.SetSecret("test-secret-k8s-handler")
	t.Cleanup(func() { secretbox.SetSecret("") })

	db, cleanup := setupServiceTestDB(t)
	t.Cleanup(cleanup)

	repo := NewRepository(db)
	svc := NewService(repo, cluster.NewRepository(db), nil)
	ctx := context.Background()

	created, err := svc.Create(ctx, &CreateHostRequest{
		Name:                     "k8s-handler-test",
		HostType:                 HostTypeKubernetes,
		K8sAuthMode:              "token",
		K8sAPIURL:                "https://10.0.0.10:6443",
		K8sNamespace:             "seatunnel-min",
		K8sToken:                 "bearer-tok",
		K8sDefaultServiceAccount: "seatunnel-runner",
	})
	if err != nil {
		t.Fatalf("create k8s host: %v", err)
	}

	mgr := k8s.NewClientManagerWithFactory(
		K8sCredentialsLoaderFromRepository(repo),
		func(*rest.Config) (kubernetes.Interface, error) { return clientset, nil },
	)
	handler := NewHandler(svc, nil)
	handler.SetK8sClientManager(mgr)
	return handler, svc, created.ID
}

// TestK8sDefaultServiceAccountPersistence 验证 k8s_default_service_account 经创建/更新持久化并在 GetHostByID 回传。
// TestK8sDefaultServiceAccountPersistence verifies k8s_default_service_account persists across CRUD and GetHostByID.
func TestK8sDefaultServiceAccountPersistence(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	_, svc, hostID := setupK8sHostHandlerForTest(t, clientset)
	ctx := context.Background()

	info, err := svc.GetHostByID(ctx, hostID)
	if err != nil {
		t.Fatalf("GetHostByID: %v", err)
	}
	if info.K8sNamespace != "seatunnel-min" || info.K8sDefaultServiceAccount != "seatunnel-runner" {
		t.Fatalf("unexpected GetHostByID fields: %+v", info)
	}

	updatedSA := "custom-st-runner"
	updated, err := svc.Update(ctx, hostID, &UpdateHostRequest{K8sDefaultServiceAccount: &updatedSA})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.K8sDefaultServiceAccount != "custom-st-runner" {
		t.Fatalf("updated default SA = %q, want custom-st-runner", updated.K8sDefaultServiceAccount)
	}
}

// TestListK8sNamespacesForbiddenFallback 验证单命名空间最小权限返回 403 时，
// ListK8sNamespaces 返回 HTTP 200 并退化为主机配置的默认命名空间列表。
// TestListK8sNamespacesForbiddenFallback verifies HTTP 200 fallback to host default namespace on 403.
func TestListK8sNamespacesForbiddenFallback(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "", nil)
	})
	handler, _, hostID := setupK8sHostHandlerForTest(t, clientset)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/hosts/%d/k8s/namespaces", hostID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", hostID)}}
	handler.ListK8sNamespaces(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var payload struct {
		ErrorMsg string   `json:"error_msg"`
		Data     []string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0] != "seatunnel-min" {
		t.Fatalf("fallback namespaces = %#v, want [seatunnel-min]", payload.Data)
	}
}

// TestTestK8sConnectionPermissionModes 验证 TestK8sConnection 自动推断 full 与 minimal 授权模式。
// TestTestK8sConnectionPermissionModes verifies automatic permission_mode detection for full vs minimal RBAC.
func TestTestK8sConnectionPermissionModes(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprintf("full=%v", full), func(t *testing.T) {
			clientset := fake.NewSimpleClientset(&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      k8s.DefaultSeaTunnelRunnerServiceAccount,
					Namespace: "seatunnel-min",
				},
			})
			clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				review := action.(k8stesting.CreateAction).GetObject().(*authv1.SelfSubjectAccessReview)
				attr := review.Spec.ResourceAttributes
				allowed := true
				if !full && attr != nil {
					if attr.Resource == "namespaces" || attr.Resource == "roles" || attr.Resource == "rolebindings" || attr.Resource == "endpoints" ||
						(attr.Resource == "serviceaccounts" && attr.Verb == "create") {
						allowed = false
					}
				}
				out := review.DeepCopy()
				out.Status.Allowed = allowed
				return true, out, nil
			})
			handler, svc, hostID := setupK8sHostHandlerForTest(t, clientset)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/hosts/%d/test-connection", hostID), nil)
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", hostID)}}
			handler.TestK8sConnection(c)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			var payload struct {
				Data K8sTestConnectionResult `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !payload.Data.Connected {
				t.Fatalf("expected connected=true, message=%q", payload.Data.Message)
			}
			wantMode := k8s.PermissionModeMinimal
			if full {
				wantMode = k8s.PermissionModeFull
			}
			if payload.Data.PermissionMode != wantMode {
				t.Fatalf("permission_mode = %q, want %q", payload.Data.PermissionMode, wantMode)
			}
			if payload.Data.CanManageRBAC != full || payload.Data.CanListNamespaces != full {
				t.Fatalf("unexpected capabilities flags: %+v", payload.Data)
			}
			if len(payload.Data.ServiceAccounts) != 1 || payload.Data.ServiceAccounts[0] != k8s.DefaultSeaTunnelRunnerServiceAccount {
				t.Fatalf("expected service_accounts=[seatunnel-runner], got %#v", payload.Data.ServiceAccounts)
			}
			persisted, err := svc.Get(context.Background(), hostID)
			if err != nil {
				t.Fatalf("get persisted host: %v", err)
			}
			if persisted.K8sPermissionMode != string(wantMode) || persisted.Status != HostStatusConnected {
				t.Fatalf("persisted permission_mode=%q status=%q, want mode=%q status=%q", persisted.K8sPermissionMode, persisted.Status, wantMode, HostStatusConnected)
			}
		})
	}
}

// TestTestK8sConnectionSSARTimeoutAndIdentity 验证：
// 1) SSAR 网络/超时错误返回 connected=false 与 error_code=timeout，而非误判为 insufficient；
// 2) JWT 令牌自动识别出自定义控制面 SA 名称并随 control_plane_sa 返回。
// TestTestK8sConnectionSSARTimeoutAndIdentity verifies SSAR timeout surfaces error_code=timeout
// (never misreported as insufficient RBAC) and custom JWT SA identity is returned in control_plane_sa.
func TestTestK8sConnectionSSARTimeoutAndIdentity(t *testing.T) {
	t.Run("ssar timeout surfaces error_code=timeout", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, context.DeadlineExceeded
		})
		handler, _, hostID := setupK8sHostHandlerForTest(t, clientset)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/hosts/%d/test-connection", hostID), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", hostID)}}
		handler.TestK8sConnection(c)

		var payload struct {
			Data K8sTestConnectionResult `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload.Data.Connected {
			t.Fatalf("expected connected=false on SSAR timeout, got %+v", payload.Data)
		}
		if payload.Data.ErrorCode != "timeout" {
			t.Fatalf("error_code=%q, want timeout", payload.Data.ErrorCode)
		}
		if payload.Data.PermissionMode != "" {
			t.Fatalf("permission_mode=%q, want empty on timeout", payload.Data.PermissionMode)
		}
	})

	t.Run("custom jwt control_plane_sa detected", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
			review := action.(k8stesting.CreateAction).GetObject().(*authv1.SelfSubjectAccessReview)
			out := review.DeepCopy()
			out.Status.Allowed = true
			return true, out, nil
		})
		handler, svc, hostID := setupK8sHostHandlerForTest(t, clientset)
		// {"sub":"system:serviceaccount:seatunnel-min:ops-stx-ctrl"}
		customJWT := "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6c2VhdHVubmVsLW1pbjpvcHMtc3R4LWN0cmwifQ.sig"
		if _, err := svc.Update(context.Background(), hostID, &UpdateHostRequest{K8sToken: &customJWT}); err != nil {
			t.Fatalf("update custom jwt: %v", err)
		}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/hosts/%d/test-connection", hostID), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", hostID)}}
		handler.TestK8sConnection(c)

		var payload struct {
			Data K8sTestConnectionResult `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !payload.Data.Connected || payload.Data.ControlPlaneSA != "ops-stx-ctrl" {
			t.Fatalf("expected connected=true control_plane_sa=ops-stx-ctrl, got %+v", payload.Data)
		}
	})
}
