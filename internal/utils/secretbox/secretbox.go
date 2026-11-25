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

// Package secretbox 提供敏感凭据的 AES-256-GCM 加密存储，密文带 enc:v1: 前缀；
// 读取时无前缀按历史明文兼容返回，便于平滑升级。
// Package secretbox provides AES-256-GCM encryption for sensitive credentials with an
// enc:v1: prefix; values without the prefix are read back as legacy plaintext for smooth upgrade.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
)

// EncryptedPrefix 密文前缀，用于区分历史明文。
// EncryptedPrefix marks ciphertext values so legacy plaintext stays distinguishable.
const EncryptedPrefix = "enc:v1:"

var (
	mu     sync.RWMutex
	secret string
)

// SetSecret 设置派生密钥用的全局密钥（来自 app.session_secret）；进程启动时调用一次。
// SetSecret installs the process-wide key material (from app.session_secret); call once at startup.
func SetSecret(s string) {
	mu.Lock()
	defer mu.Unlock()
	secret = s
}

// key 从密钥材料派生固定 32 字节 AES-256 密钥。
// key derives a fixed 32-byte AES-256 key from the key material.
func key() ([]byte, error) {
	mu.RLock()
	defer mu.RUnlock()
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("secretbox: secret is not initialized")
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:], nil
}

// Encrypt 加密明文；空串原样返回。无密钥或加密失败时返回错误，绝不静默降级为明文存储。
// Encrypt encrypts plaintext; empty input is returned as-is. It errors instead of silently
// falling back to plaintext when the key is missing or encryption fails.
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if strings.HasPrefix(plaintext, EncryptedPrefix) {
		return plaintext, nil
	}
	k, err := key()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return EncryptedPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密密文；无 enc:v1: 前缀的值按历史明文原样返回（兼容契约）。
// Decrypt decrypts ciphertext; values without the enc:v1: prefix are returned unchanged
// as legacy plaintext (compatibility contract).
func Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, EncryptedPrefix) {
		return value, nil
	}
	k, err := key()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, EncryptedPrefix))
	if err != nil {
		return "", fmt.Errorf("secretbox: invalid ciphertext encoding: %w", err)
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("secretbox: ciphertext too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("secretbox: decrypt failed: %w", err)
	}
	return string(plain), nil
}

// MustDecrypt 是 Decrypt 的便捷包装：失败时返回空串并附错误，供调用方自行决定降级策略。
// MustDecrypt is a convenience wrapper returning the plaintext or empty string plus error,
// letting callers pick their own degradation policy.
func MustDecrypt(value string) (string, error) {
	return Decrypt(value)
}

// IsEncrypted 判断值是否已为密文。
// IsEncrypted reports whether the value is already encrypted.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, EncryptedPrefix)
}
