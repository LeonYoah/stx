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

package secretbox

import (
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	SetSecret("test-session-secret")
	m.Run()
}

func TestEncryptDecrypt_roundTrip(t *testing.T) {
	cases := []string{
		"simple-token",
		"multi\nline\nkubeconfig\ncontent",
		"中文密钥内容",
		strings.Repeat("x", 8192),
	}
	for _, plain := range cases {
		enc, err := Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt error: %v", err)
		}
		if !IsEncrypted(enc) {
			t.Fatalf("expected enc:v1: prefix, got %q", enc[:min(len(enc), 20)])
		}
		if strings.Contains(enc, plain) && len(plain) < 100 {
			t.Fatalf("ciphertext leaks plaintext")
		}
		dec, err := Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt error: %v", err)
		}
		if dec != plain {
			t.Fatalf("round trip mismatch: got %q want %q", dec, plain)
		}
	}
}

func TestEncrypt_emptyString(t *testing.T) {
	enc, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty error: %v", err)
	}
	if enc != "" {
		t.Fatalf("expected empty string, got %q", enc)
	}
}

func TestEncrypt_alreadyEncrypted_isIdempotent(t *testing.T) {
	enc, err := Encrypt("value")
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	again, err := Encrypt(enc)
	if err != nil {
		t.Fatalf("Encrypt again error: %v", err)
	}
	if again != enc {
		t.Fatalf("double encrypt changed value")
	}
}

func TestDecrypt_legacyPlaintext_compatible(t *testing.T) {
	plain := "legacy-plaintext-kubeconfig"
	dec, err := Decrypt(plain)
	if err != nil {
		t.Fatalf("Decrypt legacy error: %v", err)
	}
	if dec != plain {
		t.Fatalf("legacy plaintext changed: got %q", dec)
	}
}

func TestDecrypt_emptyString(t *testing.T) {
	dec, err := Decrypt("")
	if err != nil {
		t.Fatalf("Decrypt empty error: %v", err)
	}
	if dec != "" {
		t.Fatalf("expected empty string, got %q", dec)
	}
}

func TestDecrypt_wrongSecret_fails(t *testing.T) {
	enc, err := Encrypt("secret-value")
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	SetSecret("another-secret")
	defer SetSecret("test-session-secret")
	if _, err := Decrypt(enc); err == nil {
		t.Fatalf("expected decrypt failure with wrong secret")
	}
}

func TestDecrypt_corruptedCiphertext_fails(t *testing.T) {
	if _, err := Decrypt(EncryptedPrefix + "not-valid-base64!!!"); err == nil {
		t.Fatalf("expected error for invalid base64")
	}
	if _, err := Decrypt(EncryptedPrefix + "c2hvcnQ="); err == nil {
		t.Fatalf("expected error for short ciphertext")
	}
}

func TestEncrypt_randomNonce_uniqueCiphertext(t *testing.T) {
	first, err := Encrypt("same")
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	second, err := Encrypt("same")
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	if first == second {
		t.Fatalf("expected distinct ciphertexts due to random nonce")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
