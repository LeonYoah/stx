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
	"strings"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

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
	Connected     bool                `json:"connected"`
	ServerVersion string              `json:"server_version,omitempty"`
	Message       string              `json:"message,omitempty"`
	RBAC          []k8s.RBACCheckItem `json:"rbac"`
	ElapsedMs     int64               `json:"elapsed_ms"`
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

	version, err := mgr.ServerVersion(c.Request.Context(), uint(id))
	if err != nil {
		result.Message = err.Error()
		result.ElapsedMs = time.Since(start).Milliseconds()
		c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": result})
		return
	}
	result.Connected = true
	result.ServerVersion = version

	items, err := mgr.PrecheckRBAC(c.Request.Context(), uint(id), h.service.defaultK8sNamespace(c.Request.Context(), uint(id)))
	if err != nil {
		result.Message = err.Error()
	} else {
		result.RBAC = items
	}
	result.ElapsedMs = time.Since(start).Milliseconds()

	if result.Connected {
		// 连通成功顺带更新版本号与在线状态；失败只回传结果不改库，避免误标离线。
		// On success we refresh version/online state; failures only report back without
		// touching the DB, to avoid falsely marking hosts offline.
		h.service.updateK8sVersionAfterProbe(c.Request.Context(), uint(id), version)
	}
	c.JSON(http.StatusOK, gin.H{"error_msg": "", "data": result})
}

// ListK8sNamespaces 列出该 Host 可见的 Namespace 供创建向导下拉选择。
// ListK8sNamespaces lists visible namespaces for the creation wizard dropdown.
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
