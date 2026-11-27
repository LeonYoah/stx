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
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"k8s.io/client-go/tools/clientcmd"
)

// KubeconfigContextSummary 描述单个 kubeconfig 上下文的结构化摘要（不含明文秘钥）。
// KubeconfigContextSummary describes a single kubeconfig context without exposing secrets.
type KubeconfigContextSummary struct {
	Name                string     `json:"name"`
	Cluster             string     `json:"cluster"`
	User                string     `json:"user"`
	Namespace           string     `json:"namespace,omitempty"`
	Server              string     `json:"server"`
	HasExternalFiles    bool       `json:"has_external_files"`
	HasExecPlugin       bool       `json:"has_exec_plugin"`
	HasLoopbackServer   bool       `json:"has_loopback_server"`
	CredentialExpiresAt *time.Time `json:"credential_expires_at,omitempty"`
	CredentialKind      string     `json:"credential_kind"`
}

// KubeconfigInspectResult 返回 kubeconfig 解析与结构校验结果。
// KubeconfigInspectResult returns kubeconfig parsing and validation findings.
type KubeconfigInspectResult struct {
	Valid           bool                       `json:"valid"`
	Error           string                     `json:"error,omitempty"`
	CurrentContext  string                     `json:"current_context,omitempty"`
	SelectedContext string                     `json:"selected_context,omitempty"`
	Contexts        []KubeconfigContextSummary `json:"contexts"`
	Warnings        []string                   `json:"warnings,omitempty"`
}

// ParseBearerTokenExpiry 解析 ServiceAccount / OIDC JWT 中的 exp 过期时间与凭据类型。
// ParseBearerTokenExpiry parses the exp claim and credential kind from a Kubernetes bearer JWT.
func ParseBearerTokenExpiry(token string) (*time.Time, string) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, "unknown"
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 || parts[1] == "" {
		return nil, "token"
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, "token"
	}
	var claims struct {
		Sub string  `json:"sub"`
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, "token"
	}
	kind := "token"
	if claims.Sub != "" || claims.Exp > 0 {
		kind = "service_account_jwt"
	}
	if claims.Exp > 0 {
		ts := time.Unix(int64(claims.Exp), 0).UTC()
		return &ts, kind
	}
	return nil, kind
}

// ParseX509CertExpiry 从 PEM 客户端证书解析 NotAfter 到期时间。
// ParseX509CertExpiry parses NotAfter from a PEM-encoded client certificate.
func ParseX509CertExpiry(pemBytes []byte) *time.Time {
	if len(pemBytes) == 0 {
		return nil
	}
	rest := pemBytes
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil && !cert.NotAfter.IsZero() {
			ts := cert.NotAfter.UTC()
			return &ts
		}
	}
	return nil
}

// ExtractCredentialsExpiry 尽最大努力提取凭据（Token 或 Kubeconfig）的到期时间。
// ExtractCredentialsExpiry best-effort extracts the earliest expiration time from Credentials.
func ExtractCredentialsExpiry(cred *Credentials) *time.Time {
	if cred == nil {
		return nil
	}
	mode := cred.AuthMode
	if mode == "" {
		mode = AuthModeKubeconfig
	}
	switch mode {
	case AuthModeToken:
		exp, _ := ParseBearerTokenExpiry(cred.Token)
		return exp
	case AuthModeKubeconfig:
		cfg, err := clientcmd.Load([]byte(cred.Kubeconfig))
		if err != nil || cfg == nil || len(cfg.Contexts) == 0 {
			return nil
		}
		ctxName := strings.TrimSpace(cfg.CurrentContext)
		if ctxName == "" && len(cfg.Contexts) == 1 {
			for name := range cfg.Contexts {
				ctxName = name
			}
		}
		ctxObj := cfg.Contexts[ctxName]
		if ctxObj == nil {
			return nil
		}
		authInfo := cfg.AuthInfos[ctxObj.AuthInfo]
		if authInfo == nil {
			return nil
		}
		var earliest *time.Time
		if authInfo.Token != "" {
			if exp, _ := ParseBearerTokenExpiry(authInfo.Token); exp != nil {
				earliest = exp
			}
		}
		if len(authInfo.ClientCertificateData) > 0 {
			if exp := ParseX509CertExpiry(authInfo.ClientCertificateData); exp != nil {
				if earliest == nil || exp.Before(*earliest) {
					earliest = exp
				}
			}
		}
		return earliest
	default:
		return nil
	}
}

func isLoopbackServerURL(server string) bool {
	u, err := url.Parse(strings.TrimSpace(server))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return false
}

func appendUniqueWarning(warnings []string, code string) []string {
	for _, item := range warnings {
		if item == code {
			return warnings
		}
	}
	return append(warnings, code)
}

// InspectKubeconfig 解析 kubeconfig 上下文清单、地址、到期时间与常见打包问题，绝不回显明文凭据。
// InspectKubeconfig parses kubeconfig contexts, endpoints, credential expirations, and portability
// warnings without returning plaintext credentials.
func InspectKubeconfig(raw string, preferredContext string) *KubeconfigInspectResult {
	res := &KubeconfigInspectResult{
		Contexts: []KubeconfigContextSummary{},
	}
	if strings.TrimSpace(raw) == "" {
		res.Error = "k8s: kubeconfig content is required"
		return res
	}
	cfg, err := clientcmd.Load([]byte(raw))
	if err != nil || cfg == nil {
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Error = "k8s: invalid kubeconfig"
		}
		return res
	}
	res.Valid = true
	res.CurrentContext = strings.TrimSpace(cfg.CurrentContext)

	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)

	preferred := strings.TrimSpace(preferredContext)
	selected := ""
	switch {
	case preferred != "" && cfg.Contexts[preferred] != nil:
		selected = preferred
	case res.CurrentContext != "" && cfg.Contexts[res.CurrentContext] != nil:
		selected = res.CurrentContext
	case len(names) == 1:
		selected = names[0]
		if res.CurrentContext == "" {
			res.Warnings = appendUniqueWarning(res.Warnings, "missing_current_context")
		}
	case len(names) > 1:
		res.Warnings = appendUniqueWarning(res.Warnings, "missing_current_context")
	default:
		res.Warnings = appendUniqueWarning(res.Warnings, "no_contexts")
	}
	res.SelectedContext = selected

	var selectedSummary *KubeconfigContextSummary
	for _, name := range names {
		ctxObj := cfg.Contexts[name]
		if ctxObj == nil {
			continue
		}
		clusterObj := cfg.Clusters[ctxObj.Cluster]
		authObj := cfg.AuthInfos[ctxObj.AuthInfo]

		summary := KubeconfigContextSummary{
			Name:           name,
			Cluster:        ctxObj.Cluster,
			User:           ctxObj.AuthInfo,
			Namespace:      strings.TrimSpace(ctxObj.Namespace),
			CredentialKind: "unknown",
		}
		if clusterObj != nil {
			summary.Server = strings.TrimRight(strings.TrimSpace(clusterObj.Server), "/")
			summary.HasLoopbackServer = isLoopbackServerURL(summary.Server)
			if strings.TrimSpace(clusterObj.CertificateAuthority) != "" && len(clusterObj.CertificateAuthorityData) == 0 {
				summary.HasExternalFiles = true
			}
		}
		if authObj != nil {
			if (strings.TrimSpace(authObj.ClientCertificate) != "" && len(authObj.ClientCertificateData) == 0) ||
				(strings.TrimSpace(authObj.ClientKey) != "" && len(authObj.ClientKeyData) == 0) ||
				strings.TrimSpace(authObj.TokenFile) != "" {
				summary.HasExternalFiles = true
			}
			if authObj.Exec != nil || authObj.AuthProvider != nil {
				summary.HasExecPlugin = true
			}
			var earliest *time.Time
			if authObj.Token != "" {
				exp, kind := ParseBearerTokenExpiry(authObj.Token)
				summary.CredentialKind = kind
				earliest = exp
			}
			if len(authObj.ClientCertificateData) > 0 {
				exp := ParseX509CertExpiry(authObj.ClientCertificateData)
				if summary.CredentialKind == "unknown" {
					summary.CredentialKind = "client_cert"
				}
				if exp != nil && (earliest == nil || exp.Before(*earliest)) {
					earliest = exp
				}
			}
			summary.CredentialExpiresAt = earliest
		}
		res.Contexts = append(res.Contexts, summary)
		if name == selected {
			copied := summary
			selectedSummary = &copied
		}
	}

	if selectedSummary != nil {
		if selectedSummary.HasExternalFiles {
			res.Warnings = appendUniqueWarning(res.Warnings, "external_file_refs")
		}
		if selectedSummary.HasExecPlugin {
			res.Warnings = appendUniqueWarning(res.Warnings, "exec_plugin")
		}
		if selectedSummary.HasLoopbackServer {
			res.Warnings = appendUniqueWarning(res.Warnings, "loopback_server")
		}
	}
	return res
}

// NormalizeKubeconfig 校验并规范化 kubeconfig（应用所选上下文与可选 API Server 覆写），
// 仅写入规范化 YAML 与解析出的 server 地址。
// NormalizeKubeconfig validates and normalizes kubeconfig YAML with optional context selection
// and API server URL override, returning the canonical YAML and resolved server URL.
func NormalizeKubeconfig(raw, contextName, serverOverride string) (string, string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", fmt.Errorf("k8s: kubeconfig content is required")
	}
	cfg, err := clientcmd.Load([]byte(raw))
	if err != nil || cfg == nil {
		return "", "", fmt.Errorf("k8s: invalid kubeconfig: %w", err)
	}
	targetCtx := strings.TrimSpace(contextName)
	if targetCtx == "" {
		targetCtx = strings.TrimSpace(cfg.CurrentContext)
	}
	if targetCtx == "" {
		if len(cfg.Contexts) == 1 {
			for name := range cfg.Contexts {
				targetCtx = name
			}
		} else {
			return "", "", fmt.Errorf("k8s: invalid kubeconfig: current-context is required")
		}
	}
	ctxObj := cfg.Contexts[targetCtx]
	if ctxObj == nil {
		return "", "", fmt.Errorf("k8s: invalid kubeconfig: context %q not found", targetCtx)
	}
	cfg.CurrentContext = targetCtx

	override := strings.TrimSpace(serverOverride)
	if override != "" {
		u, parseErr := url.Parse(override)
		if parseErr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return "", "", fmt.Errorf("k8s: api url is required and must be http/https with host")
		}
		clusterObj := cfg.Clusters[ctxObj.Cluster]
		if clusterObj == nil {
			return "", "", fmt.Errorf("k8s: invalid kubeconfig: cluster %q not found", ctxObj.Cluster)
		}
		clusterObj.Server = strings.TrimRight(override, "/")
	}

	out, err := clientcmd.Write(*cfg)
	if err != nil {
		return "", "", fmt.Errorf("k8s: invalid kubeconfig: %w", err)
	}
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(out)
	if err != nil {
		return "", "", fmt.Errorf("k8s: invalid kubeconfig: %w", err)
	}
	return string(out), strings.TrimRight(strings.TrimSpace(restCfg.Host), "/"), nil
}
