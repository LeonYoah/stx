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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/seatunnel"
	"github.com/gin-gonic/gin"
)

const (
	// webUIMasterCacheTTL 是 Web UI 代理目标的缓存时间；页面每个静态资源都经过代理，不能每次都探测。
	// webUIMasterCacheTTL caches the Web UI proxy target; every page asset goes through the proxy, so probing per
	// request is not acceptable.
	webUIMasterCacheTTL = 60 * time.Second
	// webUIMasterProbeTimeout 限制单个节点探测耗时。
	// webUIMasterProbeTimeout bounds one node probe.
	webUIMasterProbeTimeout = 1500 * time.Millisecond
	// webUIMasterProbePath 只有 active master 返回 200（3.x），非 master 返回 404 "Not a master node"。
	// webUIMasterProbePath answers 200 only on the active master (3.x); other nodes answer 404 "Not a master node".
	webUIMasterProbePath = "/metrics/realtime/jobs"
)

// webUIMasterSelector 为 Web UI 反向代理挑选 active master：实时背压等接口只在 active master 上可用。
// webUIMasterSelector picks the active master for the Web UI reverse proxy, because realtime backpressure APIs are
// only served by the active master.
type webUIMasterSelector struct {
	mu      sync.Mutex
	entries map[uint]webUIMasterEntry
	client  *http.Client
	now     func() time.Time
}

type webUIMasterEntry struct {
	nodeID    uint
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

// webUIProxyCandidates 返回可作为 Web UI 代理目标的 master 角色节点：在线节点在前，保持原有顺序。
// webUIProxyCandidates returns master-role nodes usable as Web UI proxy targets: online nodes first, original order kept.
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

// webUINodeAddress 返回节点的 host:port，缺少主机地址时返回空串。
// webUINodeAddress returns host:port of the node, or an empty string when the host address is missing.
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

// pick 选择代理目标：只有多个在线 master 时才探测；探测都失败（例如 2.3.x 没有该接口）时退回第一个在线节点。
// 结果（含退回结果）按集群缓存，避免每个静态资源请求都探测。
// pick selects the proxy target: it only probes when several masters are online and falls back to the first online
// node when every probe fails (for example 2.3.x lacks the endpoint). The result, fallback included, is cached per
// cluster so asset requests do not trigger probes.
func (s *webUIMasterSelector) pick(ctx context.Context, clusterID uint, nodes []*NodeInfo) *NodeInfo {
	online, fallback := webUIProxyCandidates(nodes)
	if len(online) == 0 {
		return fallback
	}
	if len(online) == 1 {
		return online[0]
	}

	s.mu.Lock()
	if entry, ok := s.entries[clusterID]; ok && s.clock().Before(entry.expiresAt) {
		for _, node := range online {
			if node.ID == entry.nodeID {
				s.mu.Unlock()
				return node
			}
		}
	}
	s.mu.Unlock()

	chosen := online[0]
	for _, node := range online {
		if s.probe(ctx, node) {
			chosen = node
			break
		}
	}
	s.mu.Lock()
	if s.entries == nil {
		s.entries = map[uint]webUIMasterEntry{}
	}
	s.entries[clusterID] = webUIMasterEntry{nodeID: chosen.ID, expiresAt: s.clock().Add(webUIMasterCacheTTL)}
	s.mu.Unlock()
	return chosen
}

// probe 判断节点是否为 active master。
// probe reports whether the node is the active master.
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

// WebUIEndpoints 是 SeaTunnel 集群观测（引擎自带 Web UI）的访问地址：经 STX 代理，或浏览器直连原生地址。
// WebUIEndpoints lists how to open the SeaTunnel cluster observation UI (the engine's own Web UI): through the STX
// proxy or directly at the native address.
type WebUIEndpoints struct {
	Available     bool   `json:"available"`
	Reason        string `json:"reason,omitempty"`
	ProxyBaseURL  string `json:"proxy_base_url"`
	NativeBaseURL string `json:"native_base_url"`
	NodeID        uint   `json:"node_id,omitempty"`
	HostName      string `json:"host_name,omitempty"`
	// JobPathTemplate 是作业详情页的 hash 路由，前端把 {job_id} 替换为引擎作业 ID 后拼在基础地址后面。
	// JobPathTemplate is the job detail hash route; the frontend substitutes {job_id} and appends it to a base URL.
	JobPathTemplate string `json:"job_path_template"`
}

// WebUIEndpointsResponse 包装 SeaTunnel 集群观测地址。
// WebUIEndpointsResponse wraps the SeaTunnel cluster observation addresses.
type WebUIEndpointsResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     *WebUIEndpoints `json:"data"`
}

// GetWebUIEndpoints 处理 GET /api/v1/clusters/:id/webui-endpoints：返回代理地址与原生地址（与代理选择同一个节点，优先 active master）。
// GetWebUIEndpoints handles GET /api/v1/clusters/:id/webui-endpoints: it returns the proxy and native addresses
// (the same node the proxy uses, preferring the active master).
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
		c.JSON(http.StatusBadRequest, WebUIEndpointsResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	clusterInfo, err := h.service.Get(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), WebUIEndpointsResponse{ErrorMsg: err.Error()})
		return
	}
	nodes, err := h.service.GetNodes(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), WebUIEndpointsResponse{ErrorMsg: err.Error()})
		return
	}
	result := &WebUIEndpoints{
		ProxyBaseURL:    "/api/v1/clusters/" + strconv.FormatUint(clusterID, 10) + "/webui/",
		JobPathTemplate: "#/jobs/{job_id}",
	}
	// 与集群详情页一致：Web UI 需要 2.3.9+ 且运行配置未关闭 enable_http。
	// Same rule as the cluster detail page: the Web UI needs 2.3.9+ and enable_http not turned off.
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
