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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/utils/secretbox"
)

// K8sCredentialsLoaderFromRepository 把持久化 Host 转换为解密后的 K8s 凭据。
// 解密失败视为凭据损坏并直接报错，避免用半份凭据误连集群。
// K8sCredentialsLoaderFromRepository converts a persisted Host into decrypted K8s
// credentials; decryption failures are surfaced as corruption errors rather than
// connecting with half-broken credentials.
func K8sCredentialsLoaderFromRepository(repo *Repository) k8s.CredentialsLoader {
	return func(ctx context.Context, hostID uint) (*k8s.Credentials, error) {
		h, err := repo.GetByID(ctx, hostID)
		if err != nil {
			return nil, err
		}
		if h == nil {
			return nil, ErrHostNotFound
		}
		kubeconfig, err := secretbox.Decrypt(h.K8sKubeconfig)
		if err != nil {
			return nil, err
		}
		token, err := secretbox.Decrypt(h.K8sToken)
		if err != nil {
			return nil, err
		}
		authMode := k8s.AuthMode(h.K8sAuthMode)
		if authMode == "" {
			authMode = k8s.AuthModeKubeconfig
		}
		return &k8s.Credentials{
			APIURL:          h.K8sAPIURL,
			AuthMode:        authMode,
			Kubeconfig:      kubeconfig,
			Token:           token,
			CACert:          h.K8sCACert,
			InsecureSkipTLS: h.K8sInsecureSkipTLS,
		}, nil
	}
}

// K8sTestConnectionResult 返回给前端的只读连通性体检结果。
// K8sTestConnectionResult is the read-only connectivity probe result for the UI.
type K8sTestConnectionResult struct {
	Connected         bool                `json:"connected"`
	ServerVersion     string              `json:"server_version,omitempty"`
	Message           string              `json:"message,omitempty"`
	ErrorCode         string              `json:"error_code,omitempty"`
	PermissionMode    k8s.PermissionMode  `json:"permission_mode,omitempty"`
	CanManageRBAC     bool                `json:"can_manage_rbac"`
	CanListNamespaces bool                `json:"can_list_namespaces"`
	ControlPlaneSA    string              `json:"control_plane_sa,omitempty"`
	ServiceAccounts   []string            `json:"service_accounts,omitempty"`
	RBAC              []k8s.RBACCheckItem `json:"rbac"`
	ElapsedMs         int64               `json:"elapsed_ms"`
}

// buildK8sManager 装配共享 ClientManager；loader 解密凭据，manager 做指纹缓存。
// buildK8sManager wires the shared ClientManager: the loader decrypts credentials
// while the manager handles fingerprint-based client caching.
func (h *Handler) buildK8sManager() (*k8s.ClientManager, error) {
	if h.k8sManager != nil {
		return h.k8sManager, nil
	}
	return nil, errK8sManagerNotConfigured
}

// SetK8sClientManager 注入共享 K8sClientManager（router 组装期完成一次）。
// SetK8sClientManager injects the shared K8sClientManager (done once during router wiring).
func (h *Handler) SetK8sClientManager(m *k8s.ClientManager) {
	h.k8sManager = m
}

// TestK8sConnection 执行纯只读连通性体检：版本探测 + SelfSubjectAccessReview 预检，
// 不写入任何 K8s 对象。
// TestK8sConnection runs a strictly read-only probe: version detection plus
// SelfSubjectAccessReview RBAC precheck, without writing any K8s object.
// @Summary 测试 K8s 主机连通性与 RBAC / Test kubernetes host connectivity and RBAC
// @Tags hosts
// @Produce json
// @Param id path int true "主机 ID / Host ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/hosts/{id}/test-connection [post]
func (h *Handler) TestK8sConnection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "invalid host id"})
		return
	}
	mgr, err := h.buildK8sManager()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}
	start := time.Now()
	result := &K8sTestConnectionResult{RBAC: []k8s.RBACCheckItem{}}
	// 为单次连通性体检设置 15s 总截止时间，防止不可达或慢集群无限挂起前端请求。
	// Enforce a 15s overall deadline for the connectivity probe so slow/unreachable clusters fail predictably.
	probeCtx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	version, err := mgr.ServerVersion(probeCtx, uint(id))
	if err != nil {
		result.Message = err.Error()
		result.ErrorCode = k8s.ClassifyK8sError(err)
		result.ElapsedMs = time.Since(start).Milliseconds()
		c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": result})
		return
	}
	result.Connected = true
	result.ServerVersion = version

	targetNS := h.service.defaultK8sNamespace(probeCtx, uint(id))
	items, err := mgr.PrecheckRBAC(probeCtx, uint(id), targetNS)
	if err != nil {
		// 并发 SSAR 遇到超时或网络错误时按连接/体检失败回传，不误标为「权限不足」。
		// Surface SSAR transport/timeout failures as probe errors instead of misreporting insufficient RBAC.
		result.Connected = false
		result.Message = err.Error()
		result.ErrorCode = k8s.ClassifyK8sError(err)
		result.ElapsedMs = time.Since(start).Milliseconds()
		c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": result})
		return
	}
	result.RBAC = items
	result.PermissionMode = k8s.EvaluatePermissionMode(items)
	result.CanManageRBAC = k8s.CanManageRBAC(items)
	result.CanListNamespaces = k8s.CanListNamespaces(items)
	result.ControlPlaneSA = mgr.WhoAmIServiceAccount(probeCtx, uint(id))

	// 探测目标命名空间内已有 ServiceAccount 供最小权限引导直接回填，失败不阻断连通测试。
	// Best-effort list existing ServiceAccounts in the target namespace for minimal-mode SA onboarding.
	if sas, saErr := mgr.ListNamespaceObjects(probeCtx, uint(id), targetNS, "serviceaccounts"); saErr == nil && len(sas) > 0 {
		result.ServiceAccounts = sas
	}
	result.ElapsedMs = time.Since(start).Milliseconds()

	if result.Connected {
		// 连通成功顺带更新版本号、授权模式与在线状态；失败只回传结果不改库，避免误标离线。
		// On success refresh version/permission_mode/online state; failures leave the DB untouched.
		h.service.updateK8sVersionAfterProbe(c.Request.Context(), uint(id), version, string(result.PermissionMode))
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": result})
}

// ListK8sNamespaces 列出该 Host 可见的 Namespace 供创建向导下拉选择。
// 单命名空间最小权限凭据无集群级 namespaces list 权限时，优雅回退为主机配置的默认命名空间。
// ListK8sNamespaces lists visible namespaces for the creation wizard dropdown.
// Single-namespace minimal credentials without cluster-wide list rights fall back to the host default namespace.
// @Summary 列出可见 namespace / List visible kubernetes namespaces
// @Tags hosts
// @Produce json
// @Param id path int true "主机 ID / Host ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/hosts/{id}/k8s/namespaces [get]
func (h *Handler) ListK8sNamespaces(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "invalid host id"})
		return
	}
	mgr, err := h.buildK8sManager()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}
	namespaces, err := mgr.ListNamespaces(c.Request.Context(), uint(id))
	if err != nil {
		if apierrors.IsForbidden(err) {
			fallbackNS := h.service.defaultK8sNamespace(c.Request.Context(), uint(id))
			c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": []string{fallbackNS}})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error_msg": err.Error(), "data": []string{}})
		return
	}
	if namespaces == nil {
		namespaces = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": namespaces})
}

// DiscoverK8sSeaTunnelClusters 扫描 Namespace 内可纳管的 SeaTunnel 集群。
// DiscoverK8sSeaTunnelClusters scans a namespace for adoptable SeaTunnel clusters.
// @Summary 扫描可纳管 SeaTunnel 集群 / Discover adoptable SeaTunnel clusters
// @Tags hosts
// @Produce json
// @Param id path int true "主机 ID / Host ID"
// @Param namespace query string false "Namespace"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/hosts/{id}/k8s/discover [get]
func (h *Handler) DiscoverK8sSeaTunnelClusters(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "invalid host id"})
		return
	}
	namespace := c.Query("namespace")
	if namespace == "" {
		namespace = h.service.defaultK8sNamespace(c.Request.Context(), uint(id))
	}
	mgr, err := h.buildK8sManager()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}
	discoverer := k8s.NewDiscoverer(mgr)
	clusters, err := discoverer.Discover(c.Request.Context(), uint(id), namespace)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error_msg": err.Error(), "data": []*k8s.DiscoveredCluster{}})
		return
	}
	if clusters == nil {
		clusters = []*k8s.DiscoveredCluster{}
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": clusters})
}

// ListK8sNamespaceObjects 按 kind 列出 namespace 内已有对象供向导高级选项下拉。
// ListK8sNamespaceObjects lists existing namespace objects by kind for wizard dropdowns.
// @Summary 列出 namespace 内已有对象 / List existing namespace objects by kind
// @Tags hosts
// @Produce json
// @Param id path int true "主机 ID / Host ID"
// @Param namespace path string true "Namespace"
// @Param kind query string true "secrets|serviceaccounts|pvcs"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/hosts/{id}/k8s/namespaces/{namespace}/objects [get]
func (h *Handler) ListK8sNamespaceObjects(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "invalid host id"})
		return
	}
	namespace := strings.TrimSpace(c.Param("namespace"))
	kind := strings.TrimSpace(c.DefaultQuery("kind", ""))
	if namespace == "" || kind == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": "namespace and kind are required"})
		return
	}
	mgr, err := h.buildK8sManager()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}
	names, err := mgr.ListNamespaceObjects(c.Request.Context(), uint(id), namespace, kind)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error_msg": err.Error(), "data": []string{}})
		return
	}
	if names == nil {
		names = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": names})
}
