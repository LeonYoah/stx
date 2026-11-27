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
	Connected           bool                `json:"connected"`
	ServerVersion       string              `json:"server_version,omitempty"`
	Message             string              `json:"message,omitempty"`
	ErrorCode           string              `json:"error_code,omitempty"`
	PermissionMode      k8s.PermissionMode  `json:"permission_mode,omitempty"`
	CanManageRBAC       bool                `json:"can_manage_rbac"`
	CanListNamespaces   bool                `json:"can_list_namespaces"`
	ControlPlaneSA      string              `json:"control_plane_sa,omitempty"`
	ServiceAccounts     []string            `json:"service_accounts,omitempty"`
	CredentialExpiresAt *time.Time          `json:"credential_expires_at,omitempty"`
	RBAC                []k8s.RBACCheckItem `json:"rbac"`
	ElapsedMs           int64               `json:"elapsed_ms"`
}

// K8sDraftTestRequest 是保存前预检 / 凭据轮换测试的请求体（无持久化副作用）。
// K8sDraftTestRequest is the stateless payload for pre-save connectivity/RBAC testing or rotation validation.
type K8sDraftTestRequest struct {
	HostID             uint   `json:"host_id,omitempty"`
	K8sAuthMode        string `json:"k8s_auth_mode"`
	K8sNamespace       string `json:"k8s_namespace"`
	K8sKubeconfig      string `json:"k8s_kubeconfig"`
	K8sContext         string `json:"k8s_context"`
	K8sAPIURL          string `json:"k8s_api_url"`
	K8sToken           string `json:"k8s_token"`
	K8sCACert          string `json:"k8s_ca_cert"`
	K8sInsecureSkipTLS bool   `json:"k8s_insecure_skip_tls"`
}

// InspectK8sKubeconfigRequest 是前端解析 kubeconfig 上下文与便携性警告的请求体。
// InspectK8sKubeconfigRequest carries raw kubeconfig YAML and an optional context preference for inspection.
type InspectK8sKubeconfigRequest struct {
	Kubeconfig string `json:"kubeconfig"`
	Context    string `json:"context,omitempty"`
}

// runK8sProbe 统一执行只读探测流程（15s 截止时间 + 版本探测 + SSAR 权限预检 + 控制面 SA + 候选 runner SA），
// 供已保存主机测试与保存前草稿探测复用。
// runK8sProbe executes the shared read-only probe pipeline (15s timeout, ServerVersion, PrecheckRBAC,
// permission tier evaluation, control-plane SA detection, and runner SA discovery).
func runK8sProbe(ctx context.Context, mgr *k8s.ClientManager, hostID uint, targetNS string, credForMeta *k8s.Credentials) *K8sTestConnectionResult {
	start := time.Now()
	result := &K8sTestConnectionResult{
		RBAC:                []k8s.RBACCheckItem{},
		CredentialExpiresAt: k8s.ExtractCredentialsExpiry(credForMeta),
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	version, err := mgr.ServerVersion(probeCtx, hostID)
	if err != nil {
		result.Message = err.Error()
		result.ErrorCode = k8s.ClassifyK8sError(err)
		result.ElapsedMs = time.Since(start).Milliseconds()
		return result
	}
	result.Connected = true
	result.ServerVersion = version

	items, err := mgr.PrecheckRBAC(probeCtx, hostID, targetNS)
	if err != nil {
		// 并发 SSAR 遇到超时或网络错误时按连接/体检失败回传，不误标为「权限不足」。
		// Surface SSAR transport/timeout failures as probe errors instead of misreporting insufficient RBAC.
		result.Connected = false
		result.Message = err.Error()
		result.ErrorCode = k8s.ClassifyK8sError(err)
		result.ElapsedMs = time.Since(start).Milliseconds()
		return result
	}
	result.RBAC = items
	result.PermissionMode = k8s.EvaluatePermissionMode(items)
	result.CanManageRBAC = k8s.CanManageRBAC(items)
	result.CanListNamespaces = k8s.CanListNamespaces(items)
	result.ControlPlaneSA = mgr.WhoAmIServiceAccount(probeCtx, hostID)

	// 探测目标命名空间内已有 ServiceAccount 供最小权限引导直接回填，失败不阻断连通测试。
	// Best-effort list existing ServiceAccounts in the target namespace for minimal-mode SA onboarding.
	if sas, saErr := mgr.ListNamespaceObjects(probeCtx, hostID, targetNS, "serviceaccounts"); saErr == nil && len(sas) > 0 {
		result.ServiceAccounts = sas
	}
	result.ElapsedMs = time.Since(start).Milliseconds()
	return result
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

	var credMeta *k8s.Credentials
	if h.service != nil && h.service.repo != nil {
		credMeta, _ = K8sCredentialsLoaderFromRepository(h.service.repo)(c.Request.Context(), uint(id))
	}
	targetNS := h.service.defaultK8sNamespace(c.Request.Context(), uint(id))
	result := runK8sProbe(c.Request.Context(), mgr, uint(id), targetNS, credMeta)

	if result.Connected {
		// 连通成功顺带更新版本号、授权模式、凭据到期时间与在线状态；失败只回传结果不改库，避免误标离线。
		// On success refresh version/permission_mode/expiry/online state; failures leave the DB untouched.
		h.service.updateK8sVersionAfterProbe(c.Request.Context(), uint(id), result.ServerVersion, string(result.PermissionMode), result.CredentialExpiresAt)
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": result})
}

// TestK8sDraftConnection 对未落库凭据（或编辑/轮换草稿）执行只读连通性与 RBAC 预检，不写数据库或 K8s。
// TestK8sDraftConnection runs a stateless read-only connectivity and RBAC probe against draft
// or rotation credentials without writing to the database or Kubernetes.
// @Summary 预检未保存 K8s 凭据连通性与 RBAC / Probe draft kubernetes credentials
// @Tags hosts
// @Accept json
// @Produce json
// @Param request body K8sDraftTestRequest true "预检请求 / Draft probe request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/hosts/k8s/test-connection [post]
func (h *Handler) TestK8sDraftConnection(c *gin.Context) {
	var req K8sDraftTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	mgr, err := h.buildK8sManager()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}

	authMode := strings.TrimSpace(req.K8sAuthMode)
	if authMode == "" {
		authMode = "kubeconfig"
	}
	targetNS := strings.TrimSpace(req.K8sNamespace)
	if targetNS == "" && req.HostID > 0 && h.service != nil {
		targetNS = h.service.defaultK8sNamespace(c.Request.Context(), req.HostID)
	}
	if targetNS == "" {
		targetNS = "default"
	}

	var storedCred *k8s.Credentials
	if req.HostID > 0 && h.service != nil && h.service.repo != nil {
		storedCred, _ = K8sCredentialsLoaderFromRepository(h.service.repo)(c.Request.Context(), req.HostID)
	}

	failInvalid := func(msg string) {
		c.JSON(http.StatusOK, gin.H{
			"error_msg": "",
			"data": &K8sTestConnectionResult{
				Connected: false,
				Message:   msg,
				ErrorCode: "invalid_config",
				RBAC:      []k8s.RBACCheckItem{},
			},
		})
	}

	var cred *k8s.Credentials
	switch authMode {
	case "kubeconfig":
		rawKC := strings.TrimSpace(req.K8sKubeconfig)
		if rawKC == "" && storedCred != nil {
			rawKC = storedCred.Kubeconfig
		}
		if rawKC == "" {
			failInvalid(ErrK8sCredentialsRequired.Error())
			return
		}
		apiOverride := strings.TrimSpace(req.K8sAPIURL)
		if apiOverride != "" && !ValidateK8sAPIURL(apiOverride) {
			failInvalid(ErrK8sAPIURLInvalid.Error())
			return
		}
		normalized, serverURL, normErr := k8s.NormalizeKubeconfig(rawKC, req.K8sContext, apiOverride)
		if normErr != nil {
			failInvalid(normErr.Error())
			return
		}
		cred = &k8s.Credentials{
			AuthMode:   k8s.AuthModeKubeconfig,
			APIURL:     serverURL,
			Namespace:  targetNS,
			Kubeconfig: normalized,
		}
	case "token":
		apiURL := strings.TrimSpace(req.K8sAPIURL)
		if apiURL == "" && storedCred != nil {
			apiURL = storedCred.APIURL
		}
		if !ValidateK8sAPIURL(apiURL) {
			failInvalid(ErrK8sAPIURLInvalid.Error())
			return
		}
		tok := strings.TrimSpace(req.K8sToken)
		if tok == "" && storedCred != nil && strings.TrimSpace(req.K8sToken) == "" {
			tok = storedCred.Token
		}
		if tok == "" {
			failInvalid(ErrK8sCredentialsRequired.Error())
			return
		}
		caCert := strings.TrimSpace(req.K8sCACert)
		if !req.K8sInsecureSkipTLS && caCert == "" && storedCred != nil {
			caCert = storedCred.CACert
		}
		cred = &k8s.Credentials{
			AuthMode:        k8s.AuthModeToken,
			APIURL:          strings.TrimRight(apiURL, "/"),
			Namespace:       targetNS,
			Token:           tok,
			CACert:          caCert,
			InsecureSkipTLS: req.K8sInsecureSkipTLS,
		}
	default:
		failInvalid(ErrK8sAuthModeInvalid.Error())
		return
	}

	result := runK8sProbe(c.Request.Context(), mgr.WithCredentials(cred), 0, targetNS, cred)
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": result})
}

// InspectK8sKubeconfig 解析 kubeconfig 上下文列表、API 地址与便携性警告（只读、不入库、不回显凭据）。
// InspectK8sKubeconfig parses kubeconfig contexts, endpoints, and portability warnings without persisting or echoing secrets.
// @Summary 解析 kubeconfig 上下文与警告 / Inspect kubeconfig contexts and warnings
// @Tags hosts
// @Accept json
// @Produce json
// @Param request body InspectK8sKubeconfigRequest true "解析请求 / Inspect request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/hosts/k8s/inspect-kubeconfig [post]
func (h *Handler) InspectK8sKubeconfig(c *gin.Context) {
	var req InspectK8sKubeconfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": err.Error()})
		return
	}
	res := k8s.InspectKubeconfig(req.Kubeconfig, req.Context)
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": res})
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
		// 最小权限环境可能未授予 secrets list；优雅返回空列表让前端退化为手动输入。
		// Minimal RBAC setups may omit secrets list; return an empty list so UI degrades to manual input.
		if apierrors.IsForbidden(err) {
			c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": []string{}})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error_msg": err.Error(), "data": []string{}})
		return
	}
	if names == nil {
		names = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": names})
}
