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

// Package tlsbootstrap provisions gRPC TLS materials when grpc.tls_enabled is explicitly true.
// tlsbootstrap 包在 grpc.tls_enabled 显式为 true 时准备 gRPC TLS 证书材料。
package tlsbootstrap

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/LeonYoah/stx/internal/config"
)

const (
	// DefaultCertDirName is the default cert subdirectory under storage base dir.
	// DefaultCertDirName 是存储根目录下默认的证书子目录名。
	DefaultCertDirName = "certs"

	caCertName     = "ca.crt"
	caKeyName      = "ca.key"
	serverCertName = "server.crt"
	serverKeyName  = "server.key"

	// Default validity for local auto-generated certs (~99 years).
	// 本地自动生成证书的默认有效期（约 99 年）。
	defaultCAValidDays     = "36135" // 99 * 365
	defaultServerValidDays = "36135" // 99 * 365
)

// Result describes the outcome of EnsureGRPCTLS.
// Result 描述 EnsureGRPCTLS 的结果。
type Result struct {
	// TLSEnabled indicates whether gRPC TLS should be enabled after bootstrap.
	// TLSEnabled 表示引导后是否应启用 gRPC TLS。
	TLSEnabled bool

	// CertFile is the server certificate path.
	// CertFile 是服务端证书路径。
	CertFile string

	// KeyFile is the server private key path.
	// KeyFile 是服务端私钥路径。
	KeyFile string

	// CAFile is the CA certificate path for Agent distribution (not mTLS client auth).
	// CAFile 是下发给 Agent 的 CA 证书路径（不是 mTLS 客户端校验用的 ca_file）。
	CAFile string

	// Generated indicates whether new certificates were created in this run.
	// Generated 表示本次是否新生成了证书。
	Generated bool

	// SkippedReason explains why TLS stayed disabled or generation was skipped.
	// SkippedReason 说明为何未开启 TLS 或跳过生成。
	SkippedReason string
}

// Options customizes bootstrap behavior (mainly for tests).
// Options 自定义引导行为（主要用于测试）。
type Options struct {
	// CertDir overrides the certificate directory.
	// CertDir 覆盖证书目录。
	CertDir string

	// ExtraHosts are additional DNS/IP names embedded into the server certificate SAN.
	// ExtraHosts 是写入服务端证书 SAN 的额外 DNS/IP。
	ExtraHosts []string

	// LookPath locates the openssl binary; defaults to exec.LookPath.
	// LookPath 用于查找 openssl；默认使用 exec.LookPath。
	LookPath func(file string) (string, error)

	// CommandRunner runs openssl commands; defaults to exec.Command.
	// CommandRunner 用于执行 openssl；默认使用 exec.Command。
	CommandRunner func(name string, arg ...string) *exec.Cmd
}

// EnsureGRPCTLS prepares unidirectional gRPC TLS materials when explicitly enabled.
// EnsureGRPCTLS 在显式开启时准备单向 gRPC TLS 材料。
//
// Behavior / 行为:
// - grpc.tls_enabled=false（默认）→ 不生成、不强制开启，即使磁盘上已有证书
// - grpc.tls_enabled=true 且证书已存在 → 启用并指向现有文件，不覆盖
// - grpc.tls_enabled=true 且证书缺失 → 本机有 openssl 则自动生成；无则保持关闭并打日志
func EnsureGRPCTLS(opts Options) (Result, error) {
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	runCmd := opts.CommandRunner
	if runCmd == nil {
		runCmd = exec.Command
	}

	certDir := opts.CertDir
	if certDir == "" {
		base := config.GetStorageConfig().BaseDir
		if base == "" {
			base = "./data/storage"
		}
		certDir = filepath.Join(base, DefaultCertDirName)
	}

	caCert := filepath.Join(certDir, caCertName)
	caKey := filepath.Join(certDir, caKeyName)
	serverCert := filepath.Join(certDir, serverCertName)
	serverKey := filepath.Join(certDir, serverKeyName)

	grpcCfg := config.GetGRPCConfig()

	// Opt-in only: default config keeps plaintext gRPC until the operator enables TLS.
	// 仅显式开启：默认配置保持明文 gRPC，直到运维打开 TLS。
	if !grpcCfg.TLSEnabled {
		reason := "grpc.tls_enabled is false; gRPC TLS left disabled"
		log.Printf("[TLS] grpc.tls_enabled=false，保持 gRPC TLS 关闭（不会自动生成证书）。需要加密时请设为 true 后重启 / %s. Set grpc.tls_enabled=true and restart to provision certs.", reason)
		return Result{
			TLSEnabled:    false,
			CertFile:      grpcCfg.CertFile,
			KeyFile:       grpcCfg.KeyFile,
			CAFile:        caCert,
			SkippedReason: reason,
		}, nil
	}

	// If user already configured explicit cert/key that both exist, keep them.
	// 若用户已显式配置且文件存在，则沿用，不覆盖。
	if grpcCfg.CertFile != "" && grpcCfg.KeyFile != "" &&
		fileExists(grpcCfg.CertFile) && fileExists(grpcCfg.KeyFile) {
		caForAgents := deriveCAPath(grpcCfg.CertFile, caCert)
		applyGRPCConfig(true, grpcCfg.CertFile, grpcCfg.KeyFile, grpcCfg.CAFile)
		log.Printf("[TLS] 已发现现有证书，启用 gRPC TLS（不会覆盖）。可用自有证书替换后重启生效 / Existing certs found; gRPC TLS enabled without overwrite. Replace files and restart to use your own certs. cert=%s key=%s ca=%s",
			grpcCfg.CertFile, grpcCfg.KeyFile, caForAgents)
		return Result{
			TLSEnabled:    true,
			CertFile:      grpcCfg.CertFile,
			KeyFile:       grpcCfg.KeyFile,
			CAFile:        caForAgents,
			Generated:     false,
			SkippedReason: "existing user certs",
		}, nil
	}

	// Default layout already complete → enable, do not overwrite.
	// 默认布局证书已齐全 → 启用且不覆盖。
	if fileExists(caCert) && fileExists(serverCert) && fileExists(serverKey) {
		applyGRPCConfig(true, serverCert, serverKey, "")
		log.Printf("[TLS] 已发现自动证书目录，启用 gRPC TLS（不会覆盖）。可替换 %s 下证书后重启 / Auto cert dir found; enabling gRPC TLS without overwrite. Replace certs under %s and restart. dir=%s",
			certDir, certDir, certDir)
		return Result{
			TLSEnabled:    true,
			CertFile:      serverCert,
			KeyFile:       serverKey,
			CAFile:        caCert,
			Generated:     false,
			SkippedReason: "existing auto certs",
		}, nil
	}

	// Incomplete leftover materials must never be overwritten (PRD: do not clobber existing cert files).
	// 残留不完整材料一律不覆盖（PRD：已有证书文件不覆盖）。
	if anyCertMaterialExists(caCert, caKey, serverCert, serverKey) {
		config.Config.GRPC.TLSEnabled = false
		reason := "incomplete cert materials present; refusing overwrite"
		log.Printf("[TLS] 证书目录存在不完整文件，拒绝覆盖且关闭 TLS。请补齐 ca.crt/server.crt/server.key 或清空目录后重启 / Incomplete cert files present under %s; refusing overwrite and leaving gRPC TLS disabled. Complete the set or clear the directory and restart. dir=%s",
			certDir, certDir)
		return Result{
			TLSEnabled:    false,
			CertFile:      grpcCfg.CertFile,
			KeyFile:       grpcCfg.KeyFile,
			CAFile:        caCert,
			SkippedReason: reason,
		}, nil
	}

	opensslPath, err := lookPath("openssl")
	if err != nil || opensslPath == "" {
		// User opted in but materials cannot be created without openssl.
		// 用户已显式开启，但无 openssl 无法创建材料。
		config.Config.GRPC.TLSEnabled = false
		reason := "openssl not found; gRPC TLS left disabled"
		log.Printf("[TLS] 配置要求开启 gRPC TLS，但证书不存在且未检测到 openssl，已保持关闭 / Config requested gRPC TLS, but certs are missing and openssl was not found; TLS left disabled")
		return Result{
			TLSEnabled:    false,
			CertFile:      grpcCfg.CertFile,
			KeyFile:       grpcCfg.KeyFile,
			SkippedReason: reason,
		}, nil
	}

	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create cert dir: %w", err)
	}

	hosts := collectSANHosts(opts.ExtraHosts)
	if err := generateCerts(runCmd, opensslPath, certDir, caCert, caKey, serverCert, serverKey, hosts); err != nil {
		return Result{}, err
	}

	// Unidirectional TLS: leave server CAFile empty so ClientAuth stays off.
	// 单向 TLS：服务端 CAFile 留空，避免误开 mTLS。
	applyGRPCConfig(true, serverCert, serverKey, "")
	log.Printf("[TLS] 已用 openssl 生成证书并开启 gRPC TLS。可用自有证书替换 %s 后重启 / Generated certs with openssl and enabled gRPC TLS. Replace files under %s and restart to use your own. dir=%s san=%v",
		certDir, certDir, certDir, hosts)

	return Result{
		TLSEnabled: true,
		CertFile:   serverCert,
		KeyFile:    serverKey,
		CAFile:     caCert,
		Generated:  true,
	}, nil
}

// AgentCAPath returns the CA certificate path Agents should download when TLS is on.
// AgentCAPath 返回 Agent 在 TLS 开启时应下载的 CA 证书路径。
func AgentCAPath() string {
	grpcCfg := config.GetGRPCConfig()
	if !grpcCfg.TLSEnabled || grpcCfg.CertFile == "" {
		return ""
	}
	base := config.GetStorageConfig().BaseDir
	if base == "" {
		base = "./data/storage"
	}
	defaultCA := filepath.Join(base, DefaultCertDirName, caCertName)
	return deriveCAPath(grpcCfg.CertFile, defaultCA)
}

// applyGRPCConfig writes TLS fields into the global config.
// applyGRPCConfig 将 TLS 字段写入全局配置。
func applyGRPCConfig(enabled bool, certFile, keyFile, caFile string) {
	config.Config.GRPC.TLSEnabled = enabled
	config.Config.GRPC.CertFile = certFile
	config.Config.GRPC.KeyFile = keyFile
	// Preserve intentional mTLS CA only when caller passes non-empty caFile.
	// 仅当调用方显式传入非空 caFile 时保留 mTLS CA。
	config.Config.GRPC.CAFile = caFile
}

// deriveCAPath prefers sibling ca.crt next to the server cert.
// deriveCAPath 优先使用服务端证书同目录下的 ca.crt。
func deriveCAPath(serverCert, fallback string) string {
	sibling := filepath.Join(filepath.Dir(serverCert), caCertName)
	if fileExists(sibling) {
		return sibling
	}
	if fileExists(fallback) {
		return fallback
	}
	return sibling
}

// collectSANHosts builds DNS/IP SAN entries for the server certificate.
// collectSANHosts 构建服务端证书的 DNS/IP SAN 列表。
func collectSANHosts(extra []string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" {
			return
		}
		h = strings.ToLower(h)
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}

	add("localhost")
	add("127.0.0.1")
	add("::1")

	if ext := config.GetExternalURL(); ext != "" {
		if u, err := url.Parse(ext); err == nil && u.Hostname() != "" {
			add(u.Hostname())
		} else {
			// external_url may be host:port without scheme
			// external_url 可能是无 scheme 的 host:port
			host := ext
			if strings.Contains(ext, "://") {
				host = strings.SplitN(ext, "://", 2)[1]
			}
			if i := strings.Index(host, "/"); i >= 0 {
				host = host[:i]
			}
			if h, _, err := net.SplitHostPort(host); err == nil {
				add(h)
			} else {
				add(host)
			}
		}
	}

	for _, h := range extra {
		add(h)
	}
	return out
}

// generateCerts creates a local CA and a server certificate signed by that CA via openssl.
// generateCerts 通过 openssl 创建本地 CA 并用该 CA 签发服务端证书。
func generateCerts(
	runCmd func(name string, arg ...string) *exec.Cmd,
	opensslPath string,
	certDir, caCert, caKey, serverCert, serverKey string,
	hosts []string,
) error {
	run := func(args ...string) error {
		cmd := runCmd(opensslPath, args...)
		cmd.Dir = certDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("openssl %v: %w (%s)", args, err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	// Generate CA key + self-signed CA cert.
	// 生成 CA 私钥与自签 CA 证书。
	if err := run("genrsa", "-out", caKeyName, "2048"); err != nil {
		return err
	}
	if err := run("req", "-x509", "-new", "-nodes",
		"-key", caKeyName,
		"-sha256", "-days", defaultCAValidDays,
		"-out", caCertName,
		"-subj", "/CN=STX-Local-CA"); err != nil {
		return err
	}

	if err := run("genrsa", "-out", serverKeyName, "2048"); err != nil {
		return err
	}
	if err := run("req", "-new",
		"-key", serverKeyName,
		"-out", "server.csr",
		"-subj", "/CN=stx-grpc"); err != nil {
		return err
	}

	extPath := filepath.Join(certDir, "server_ext.cnf")
	if err := os.WriteFile(extPath, []byte(buildExtConfig(hosts)), 0o644); err != nil {
		return fmt.Errorf("write openssl ext config: %w", err)
	}

	if err := run("x509", "-req",
		"-in", "server.csr",
		"-CA", caCertName,
		"-CAkey", caKeyName,
		"-CAcreateserial",
		"-out", serverCertName,
		"-days", defaultServerValidDays,
		"-sha256",
		"-extfile", "server_ext.cnf"); err != nil {
		return err
	}

	// Best-effort cleanup of intermediate files.
	// 尽力清理中间文件。
	_ = os.Remove(filepath.Join(certDir, "server.csr"))
	_ = os.Remove(extPath)
	_ = os.Remove(filepath.Join(certDir, "ca.srl"))

	// Restrict private key permissions.
	// 收紧私钥权限。
	_ = os.Chmod(caKey, 0o600)
	_ = os.Chmod(serverKey, 0o600)
	_ = os.Chmod(caCert, 0o644)
	_ = os.Chmod(serverCert, 0o644)

	return nil
}

// buildExtConfig builds an openssl extension file with SAN entries.
// buildExtConfig 构建带 SAN 条目的 openssl 扩展配置文件。
func buildExtConfig(hosts []string) string {
	var b strings.Builder
	b.WriteString("basicConstraints=CA:FALSE\n")
	b.WriteString("keyUsage=digitalSignature,keyEncipherment\n")
	b.WriteString("extendedKeyUsage=serverAuth\n")
	b.WriteString("subjectAltName=@alt_names\n\n")
	b.WriteString("[alt_names]\n")
	dnsIdx, ipIdx := 1, 1
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			fmt.Fprintf(&b, "IP.%d = %s\n", ipIdx, h)
			ipIdx++
			continue
		}
		fmt.Fprintf(&b, "DNS.%d = %s\n", dnsIdx, h)
		dnsIdx++
	}
	return b.String()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// anyCertMaterialExists reports whether any auto-TLS material file is already on disk.
// anyCertMaterialExists 判断自动 TLS 相关材料文件是否已有任一存在。
func anyCertMaterialExists(paths ...string) bool {
	for _, p := range paths {
		if fileExists(p) {
			return true
		}
	}
	return false
}
