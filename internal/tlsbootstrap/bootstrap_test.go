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

package tlsbootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LeonYoah/stx/internal/config"
)

func TestEnsureGRPCTLS_DefaultDisabledNoGenerate(t *testing.T) {
	config.Config.GRPC.TLSEnabled = false
	config.Config.GRPC.CertFile = ""
	config.Config.GRPC.KeyFile = ""

	res, err := EnsureGRPCTLS(Options{
		CertDir: t.TempDir(),
		LookPath: func(string) (string, error) {
			return "/usr/bin/openssl", nil
		},
		CommandRunner: func(name string, arg ...string) *exec.Cmd {
			t.Fatalf("openssl should not run when tls_enabled=false")
			return exec.Command(name, arg...)
		},
	})
	if err != nil {
		t.Fatalf("EnsureGRPCTLS: %v", err)
	}
	if res.TLSEnabled || res.Generated {
		t.Fatalf("expected TLS left disabled by default: %+v", res)
	}
	if config.Config.GRPC.TLSEnabled {
		t.Fatalf("expected global TLSEnabled=false")
	}
}

func TestEnsureGRPCTLS_NoOpenSSL(t *testing.T) {
	config.Config.GRPC.TLSEnabled = true
	config.Config.GRPC.CertFile = ""
	config.Config.GRPC.KeyFile = ""

	res, err := EnsureGRPCTLS(Options{
		CertDir: t.TempDir(),
		LookPath: func(string) (string, error) {
			return "", exec.ErrNotFound
		},
	})
	if err != nil {
		t.Fatalf("EnsureGRPCTLS: %v", err)
	}
	if res.TLSEnabled {
		t.Fatalf("expected TLS disabled without openssl")
	}
	if config.Config.GRPC.TLSEnabled {
		t.Fatalf("expected global TLSEnabled=false")
	}
	if res.SkippedReason == "" {
		t.Fatalf("expected skipped reason")
	}
}

func TestEnsureGRPCTLS_ExistingCertsNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.crt")
	crt := filepath.Join(dir, "server.crt")
	key := filepath.Join(dir, "server.key")
	for _, p := range []string{ca, crt, key} {
		if err := os.WriteFile(p, []byte("placeholder"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	config.Config.GRPC.TLSEnabled = true
	config.Config.GRPC.CertFile = ""
	config.Config.GRPC.KeyFile = ""

	called := false
	res, err := EnsureGRPCTLS(Options{
		CertDir: dir,
		LookPath: func(string) (string, error) {
			called = true
			return "/usr/bin/openssl", nil
		},
		CommandRunner: func(name string, arg ...string) *exec.Cmd {
			t.Fatalf("openssl should not run when certs exist")
			return exec.Command(name, arg...)
		},
	})
	if err != nil {
		t.Fatalf("EnsureGRPCTLS: %v", err)
	}
	if !res.TLSEnabled || res.Generated {
		t.Fatalf("expected enable without generate: %+v", res)
	}
	if called {
		// LookPath may still be unused because we return early — that's fine.
		_ = called
	}
	if config.Config.GRPC.CertFile != crt || config.Config.GRPC.KeyFile != key {
		t.Fatalf("unexpected paths: cert=%s key=%s", config.Config.GRPC.CertFile, config.Config.GRPC.KeyFile)
	}
	if config.Config.GRPC.CAFile != "" {
		t.Fatalf("server CAFile should stay empty for unidirectional TLS")
	}
}

func TestEnsureGRPCTLS_ExistingCertsIgnoredWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ca.crt", "server.crt", "server.key"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("placeholder"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	config.Config.GRPC.TLSEnabled = false
	config.Config.GRPC.CertFile = ""
	config.Config.GRPC.KeyFile = ""

	res, err := EnsureGRPCTLS(Options{
		CertDir: dir,
		LookPath: func(string) (string, error) {
			return "/usr/bin/openssl", nil
		},
		CommandRunner: func(name string, arg ...string) *exec.Cmd {
			t.Fatalf("openssl should not run when tls_enabled=false")
			return exec.Command(name, arg...)
		},
	})
	if err != nil {
		t.Fatalf("EnsureGRPCTLS: %v", err)
	}
	if res.TLSEnabled || config.Config.GRPC.TLSEnabled {
		t.Fatalf("existing certs must not force-enable TLS when tls_enabled=false: %+v", res)
	}
}

func TestEnsureGRPCTLS_GenerateWithOpenSSL(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not available")
	}

	dir := t.TempDir()
	config.Config.GRPC.TLSEnabled = true
	config.Config.GRPC.CertFile = ""
	config.Config.GRPC.KeyFile = ""
	config.Config.App.ExternalURL = "http://cp.example.com:8000"

	res, err := EnsureGRPCTLS(Options{
		CertDir:    dir,
		ExtraHosts: []string{"10.0.0.1"},
	})
	if err != nil {
		t.Fatalf("EnsureGRPCTLS: %v", err)
	}
	if !res.TLSEnabled || !res.Generated {
		t.Fatalf("expected generated TLS: %+v", res)
	}
	for _, p := range []string{res.CAFile, res.CertFile, res.KeyFile} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	if !config.Config.GRPC.TLSEnabled {
		t.Fatalf("global TLS should be enabled")
	}
}

func TestBuildExtConfigContainsSAN(t *testing.T) {
	cfg := buildExtConfig([]string{"localhost", "127.0.0.1", "cp.example.com"})
	for _, want := range []string{"DNS.1 = localhost", "IP.1 = 127.0.0.1", "DNS.2 = cp.example.com"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("missing %q in %s", want, cfg)
		}
	}
}

func TestAgentCAPath_UsesSiblingCA(t *testing.T) {
	dir := t.TempDir()
	serverCert := filepath.Join(dir, "server.crt")
	ca := filepath.Join(dir, "ca.crt")
	for _, p := range []string{serverCert, ca} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	config.Config.GRPC.TLSEnabled = true
	config.Config.GRPC.CertFile = serverCert
	config.Config.GRPC.KeyFile = filepath.Join(dir, "server.key")
	_ = os.WriteFile(config.Config.GRPC.KeyFile, []byte("k"), 0o600)

	got := AgentCAPath()
	if got != ca {
		t.Fatalf("AgentCAPath=%s want %s", got, ca)
	}
}

func TestAgentCAPath_EmptyWhenTLSOff(t *testing.T) {
	config.Config.GRPC.TLSEnabled = false
	config.Config.GRPC.CertFile = "/tmp/server.crt"
	if got := AgentCAPath(); got != "" {
		t.Fatalf("expected empty CA path when TLS off, got %q", got)
	}
}
