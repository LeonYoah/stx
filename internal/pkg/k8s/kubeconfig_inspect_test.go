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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"
)

func makeTestJWT(sub string, expUnix int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payloadJSON := fmt.Sprintf(`{"sub":%q,"exp":%d}`, sub, expUnix)
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return header + "." + payload + ".sig"
}

func makeSelfSignedPEMCert(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "stx-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestInspectAndNormalizeKubeconfig(t *testing.T) {
	expUnix := time.Date(2028, 6, 15, 12, 0, 0, 0, time.UTC).Unix()
	jwtA := makeTestJWT("system:serviceaccount:ns-a:stx-a", expUnix)
	certNotAfter := time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC)
	certB64 := base64.StdEncoding.EncodeToString(makeSelfSignedPEMCert(t, certNotAfter))

	multiYAML := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: prod-cluster
  cluster:
    server: https://10.1.0.5:6443
- name: local-cluster
  cluster:
    server: https://127.0.0.1:6443
    certificate-authority: /Users/mac/.minikube/ca.crt
contexts:
- name: local
  context:
    cluster: local-cluster
    user: local-user
    namespace: seatunnel-local
- name: prod
  context:
    cluster: prod-cluster
    user: prod-user
    namespace: seatunnel-prod
current-context: local
users:
- name: prod-user
  user:
    token: %s
- name: local-user
  user:
    client-certificate-data: %s
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws
`, jwtA, certB64)

	t.Run("inspect detects warnings, expiry, and preferred context", func(t *testing.T) {
		res := InspectKubeconfig(multiYAML, "")
		if !res.Valid || res.SelectedContext != "local" || res.CurrentContext != "local" {
			t.Fatalf("unexpected inspect header: %+v", res)
		}
		if len(res.Contexts) != 2 {
			t.Fatalf("expected 2 contexts, got %d", len(res.Contexts))
		}
		for _, code := range []string{"external_file_refs", "exec_plugin", "loopback_server"} {
			if !slices.Contains(res.Warnings, code) {
				t.Fatalf("expected warning %q in %v", code, res.Warnings)
			}
		}

		resProd := InspectKubeconfig(multiYAML, "prod")
		if resProd.SelectedContext != "prod" {
			t.Fatalf("expected selected_context=prod, got %q", resProd.SelectedContext)
		}
		if len(resProd.Warnings) != 0 {
			t.Fatalf("expected no warnings on clean prod context, got %v", resProd.Warnings)
		}
		var prodCtx KubeconfigContextSummary
		for _, c := range resProd.Contexts {
			if c.Name == "prod" {
				prodCtx = c
			}
		}
		if prodCtx.CredentialKind != "service_account_jwt" || prodCtx.CredentialExpiresAt == nil || prodCtx.CredentialExpiresAt.Unix() != expUnix {
			t.Fatalf("unexpected prod credential summary: %+v", prodCtx)
		}
	})

	t.Run("normalize selects context and overrides server", func(t *testing.T) {
		normalized, serverURL, err := NormalizeKubeconfig(multiYAML, "prod", "https://k8s-intranet.example.com:6443/")
		if err != nil {
			t.Fatalf("NormalizeKubeconfig error: %v", err)
		}
		if serverURL != "https://k8s-intranet.example.com:6443" {
			t.Fatalf("serverURL=%q, want https://k8s-intranet.example.com:6443", serverURL)
		}
		recheck := InspectKubeconfig(normalized, "")
		if !recheck.Valid || recheck.CurrentContext != "prod" {
			t.Fatalf("unexpected normalized inspect: %+v", recheck)
		}
		// Ensure plaintext jwt does not appear in inspect result JSON.
		if strings.Contains(fmt.Sprintf("%+v", recheck), jwtA) {
			t.Fatalf("inspect result must never leak token material")
		}
	})

	t.Run("extracts x509 cert expiry when client-certificate-data is present", func(t *testing.T) {
		exp := ExtractCredentialsExpiry(&Credentials{AuthMode: AuthModeKubeconfig, Kubeconfig: multiYAML})
		if exp == nil || exp.Unix() != certNotAfter.Unix() {
			t.Fatalf("expected x509 expiry %v, got %v", certNotAfter, exp)
		}
	})

	t.Run("invalid kubeconfig fails inspection cleanly", func(t *testing.T) {
		res := InspectKubeconfig("not: [valid", "")
		if res.Valid || res.Error == "" {
			t.Fatalf("expected invalid result with error, got %+v", res)
		}
	})
}
