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
	"github.com/LeonYoah/stx/internal/pkg/locale"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/seatunnel"
	"github.com/gin-gonic/gin"
	"net/http/httputil"
	"net/url"
)

const (
	webUIMasterCacheTTL         = 60 * time.Second
	webUIMasterNegativeCacheTTL = 5 * time.Second
	webUIMasterProbeTimeout     = 1500 * time.Millisecond
	webUIMasterProbePath        = "/metrics/realtime/jobs"
)

type webUIMasterSelector struct {
	mu      sync.Mutex
	entries map[uint]webUIMasterEntry
	client  *http.Client
	now     func() time.Time
}

type webUIMasterEntry struct {
	nodeID    uint
	confirmed bool
	expiresAt time.Time
}

func (s *webUIMasterSelector) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *webUIMasterSelector) httpClient() *http.Client {
	if s.client != nil {
		return s.client
	}
	return &http.Client{Timeout: webUIMasterProbeTimeout}
}

func webUIProxyCandidates(nodes []*NodeInfo) (online []*NodeInfo, fallback *NodeInfo) {
	for _, node := range nodes {
		if node == nil || node.APIPort <= 0 {
			continue
		}
		if node.Role != NodeRoleMaster && node.Role != NodeRoleMasterWorker {
			continue
		}
		if fallback == nil {
			fallback = node
		}
		if node.IsOnline {
			online = append(online, node)
		}
	}
	return online, fallback
}

func webUINodeAddress(node *NodeInfo) string {
	host := strings.TrimSpace(node.HostIP)
	if host == "" {
		host = strings.TrimSpace(node.HostName)
	}
	if host == "" {
		return ""
	}
	return host + ":" + strconv.Itoa(node.APIPort)
}

func (s *webUIMasterSelector) activeMasterNodeID(ctx context.Context, clusterID uint, nodes []*NodeInfo) (uint, bool) {
	online, _ := webUIProxyCandidates(nodes)
	if len(online) == 0 {
		return 0, false
	}
	if len(online) == 1 {
		return online[0].ID, true
	}

	s.mu.Lock()
	if entry, ok := s.entries[clusterID]; ok && s.clock().Before(entry.expiresAt) {
		if !entry.confirmed {
			s.mu.Unlock()
			return 0, false
		}
		for _, node := range online {
			if node.ID == entry.nodeID {
				s.mu.Unlock()
				return entry.nodeID, true
			}
		}
	}
	s.mu.Unlock()

	for _, node := range online {
		if s.probe(ctx, node) {
			s.mu.Lock()
			if s.entries == nil {
				s.entries = map[uint]webUIMasterEntry{}
			}
			s.entries[clusterID] = webUIMasterEntry{nodeID: node.ID, confirmed: true, expiresAt: s.clock().Add(webUIMasterCacheTTL)}
			s.mu.Unlock()
			return node.ID, true
		}
	}
	if ctx.Err() == nil {
		s.mu.Lock()
		if s.entries == nil {
			s.entries = map[uint]webUIMasterEntry{}
		}
		s.entries[clusterID] = webUIMasterEntry{nodeID: online[0].ID, expiresAt: s.clock().Add(webUIMasterNegativeCacheTTL)}
		s.mu.Unlock()
	}
	return 0, false
}

func (s *webUIMasterSelector) pick(ctx context.Context, clusterID uint, nodes []*NodeInfo) *NodeInfo {
	online, fallback := webUIProxyCandidates(nodes)
	if len(online) == 0 {
		return fallback
	}
	if len(online) == 1 {
		return online[0]
	}
	activeID, _ := s.activeMasterNodeID(ctx, clusterID, nodes)
	for _, node := range online {
		if node.ID == activeID {
			return node
		}
	}
	return online[0]
}

func (s *webUIMasterSelector) probe(ctx context.Context, node *NodeInfo) bool {
	address := webUINodeAddress(node)
	if address == "" {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, webUIMasterProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+address+webUIMasterProbePath, nil)
	if err != nil {
		return false
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type WebUIEndpoints struct {
	Available       bool   `json:"available"`
	Reason          string `json:"reason,omitempty"`
	ProxyBaseURL    string `json:"proxy_base_url"`
	NativeBaseURL   string `json:"native_base_url"`
	NodeID          uint   `json:"node_id,omitempty"`
	HostName        string `json:"host_name,omitempty"`
	JobPathTemplate string `json:"job_path_template"`
}

type WebUIEndpointsResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     *WebUIEndpoints `json:"data"`
}

// @Summary 获取 SeaTunnel 集群观测地址
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} WebUIEndpointsResponse
// @Failure 400 {object} WebUIEndpointsResponse
// @Failure 404 {object} WebUIEndpointsResponse
// @Router /api/v1/clusters/{id}/webui-endpoints [get]
func (h *Handler) GetWebUIEndpoints(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, WebUIEndpointsResponse{ErrorMsg: locale.Pick(c.Request.Context(), "无效的集群 ID", "Invalid cluster ID")})
		return
	}
	clusterInfo, err := h.service.Get(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), WebUIEndpointsResponse{ErrorMsg: err.Error()})
		return
	}
	if clusterInfo.IsKubernetes() {
		c.JSON(http.StatusOK, WebUIEndpointsResponse{Data: k8sWebUIEndpoints(uint(clusterID))})
		return
	}
	nodes, err := h.service.GetNodes(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), WebUIEndpointsResponse{ErrorMsg: err.Error()})
		return
	}
	result := &WebUIEndpoints{
		ProxyBaseURL:    "/api/v1/clusters/" + strconv.FormatUint(uint64(clusterID), 10) + "/webui/",
		JobPathTemplate: "#/jobs/{job_id}",
	}
	if !seatunnel.CapabilitiesForVersion(clusterInfo.Version).SupportsHTTPService {
		result.Reason = "version_unsupported"
		c.JSON(http.StatusOK, WebUIEndpointsResponse{Data: result})
		return
	}
	if runtime, ok := clusterInfo.Config["runtime"].(map[string]interface{}); ok {
		if enabled, isBool := runtime["enable_http"].(bool); isBool && !enabled {
			result.Reason = "http_disabled"
			c.JSON(http.StatusOK, WebUIEndpointsResponse{Data: result})
			return
		}
	}
	target := h.webUIMaster.pick(c.Request.Context(), uint(clusterID), nodes)
	switch {
	case target == nil:
		result.Reason = "no_webui_node"
	case webUINodeAddress(target) == "":
		result.Reason = "node_address_missing"
	default:
		result.Available = true
		result.NativeBaseURL = "http://" + webUINodeAddress(target) + "/"
		result.NodeID = target.ID
		result.HostName = target.HostName
	}
	c.JSON(http.StatusOK, WebUIEndpointsResponse{Data: result})
}

// proxyK8sWebUI K8s 集群 WebUI 代理：经 K8sTransport 反向代理到 master Service（apiserver_proxy
// 已指向 active master 由 Service 负载均衡语义承载，单 master session 模式无需选主）。
// proxyK8sWebUI proxies the WebUI of a K8s cluster to the master service through K8sTransport.
func (h *Handler) proxyK8sWebUI(c *gin.Context, clusterObj *Cluster, clusterID uint) {
	cfg := clusterObj.K8sSessionConfig()
	if cfg == nil || h.service.k8sManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_msg": locale.Pick(c.Request.Context(), "K8s 会话配置缺失", "K8s session config missing")})
		return
	}
	transport := k8s.NewTransport(h.service.k8sManager, cfg.HostID, cfg.Namespace, cfg.AccessMode, cfg.ExternalBaseURL)
	port := cfg.MasterHTTPPort
	if port <= 0 {
		port = 8080
	}
	base, err := transport.ServiceBaseURL(c.Request.Context(), cfg.MasterServiceName, port)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}
	rt, err := transport.RoundTripper(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}
	target, err := url.Parse(base)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error_msg": err.Error()})
		return
	}

	prefix := "/api/v1/clusters/" + strconv.FormatUint(uint64(clusterID), 10) + "/webui"
	proxyPath := c.Param("proxyPath")
	if proxyPath == "" {
		proxyPath = "/"
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = rt
	originalDirector := proxy.Director
	// apiserver_proxy 的 base URL 已含 /proxy 前缀路径，重写时需要保留 base 路径再拼接业务路径。
	// The apiserver_proxy base URL already carries the /proxy prefix path, so rewriting must
	// keep the base path before appending the business path.
	basePath := strings.TrimRight(target.Path, "/")
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		if proxyPath == "/" {
			req.URL.Path = basePath + "/"
		} else {
			req.URL.Path = basePath + proxyPath
		}
		req.URL.RawPath = req.URL.EscapedPath()
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Prefix", prefix)
		req.Header.Set("X-Forwarded-Host", c.Request.Host)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		c.JSON(http.StatusBadGateway, gin.H{"error_msg": err.Error()})
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

// k8sWebUIEndpoints K8s 集群观测地址：无裸金属节点概念，代理即真相。
// k8sWebUIEndpoints reports observability endpoints for a K8s cluster: no bare-metal node
// concept, the proxy is the source of truth.
func k8sWebUIEndpoints(clusterID uint) *WebUIEndpoints {
	return &WebUIEndpoints{
		Available:       true,
		ProxyBaseURL:    "/api/v1/clusters/" + strconv.FormatUint(uint64(clusterID), 10) + "/webui/",
		JobPathTemplate: "#/jobs/{job_id}",
	}
}
