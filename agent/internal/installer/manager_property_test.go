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

package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

// **Feature: stx-agent, Property 8: Checksum Validation**
// **Validates: Requirements 5.2**
//
// Property: For any offline installation request with a specified package path
// and expected checksum, the system SHALL verify the file's checksum matches
// the expected value and reject mismatches with an error.
// 属性：对于任何指定安装包路径和预期校验和的离线安装请求，
// 系统应该验证文件的校验和与预期值匹配，并在不匹配时返回错误。
func TestProperty_ChecksumValidation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random file content / 生成随机文件内容
		contentSize := rapid.IntRange(1, 10000).Draw(rt, "contentSize")
		content := make([]byte, contentSize)
		for i := range content {
			content[i] = byte(rapid.IntRange(0, 255).Draw(rt, "byte"))
		}

		// Create a temporary file with the content / 创建包含内容的临时文件
		tempDir := t.TempDir()
		tempFile := filepath.Join(tempDir, "test-package.tar.gz")
		if err := os.WriteFile(tempFile, content, 0644); err != nil {
			rt.Fatalf("Failed to create temp file: %v", err)
		}

		// Calculate the actual checksum / 计算实际校验和
		hash := sha256.Sum256(content)
		actualChecksum := hex.EncodeToString(hash[:])

		// Create installer manager / 创建安装管理器
		manager := NewInstallerManager()

		// Property 1: Correct checksum should pass verification
		// 属性 1：正确的校验和应该通过验证
		err := manager.VerifyChecksum(tempFile, actualChecksum)
		if err != nil {
			rt.Fatalf("Checksum verification failed for correct checksum: %v", err)
		}

		// Property 2: Uppercase checksum should also pass (case-insensitive)
		// 属性 2：大写校验和也应该通过（不区分大小写）
		err = manager.VerifyChecksum(tempFile, hex.EncodeToString(hash[:]))
		if err != nil {
			rt.Fatalf("Checksum verification failed for uppercase checksum: %v", err)
		}

		// Property 3: Incorrect checksum should fail verification
		// 属性 3：错误的校验和应该验证失败
		// Generate a different checksum by modifying one character
		// 通过修改一个字符生成不同的校验和
		wrongChecksum := generateWrongChecksum(actualChecksum)
		err = manager.VerifyChecksum(tempFile, wrongChecksum)
		if err == nil {
			rt.Fatal("Checksum verification should fail for incorrect checksum")
		}
		if !errors.Is(err, ErrChecksumMismatch) {
			rt.Fatalf("Expected ErrChecksumMismatch, got: %v", err)
		}

		// Property 4: CalculateChecksum should return consistent results
		// 属性 4：CalculateChecksum 应该返回一致的结果
		calculatedChecksum, err := CalculateChecksum(tempFile)
		if err != nil {
			rt.Fatalf("CalculateChecksum failed: %v", err)
		}
		if calculatedChecksum != actualChecksum {
			rt.Fatalf("CalculateChecksum returned inconsistent result: expected %s, got %s", actualChecksum, calculatedChecksum)
		}

		// Property 5: Checksum with whitespace should be trimmed and pass
		// 属性 5：带空格的校验和应该被修剪并通过
		checksumWithSpaces := "  " + actualChecksum + "  \n"
		err = manager.VerifyChecksum(tempFile, checksumWithSpaces)
		if err != nil {
			rt.Fatalf("Checksum verification failed for checksum with whitespace: %v", err)
		}
	})
}

// generateWrongChecksum generates a checksum that differs from the input
// generateWrongChecksum 生成与输入不同的校验和
func generateWrongChecksum(checksum string) string {
	if len(checksum) == 0 {
		return "0000000000000000000000000000000000000000000000000000000000000000"
	}

	// Modify the first character / 修改第一个字符
	chars := []byte(checksum)
	if chars[0] == '0' {
		chars[0] = '1'
	} else {
		chars[0] = '0'
	}
	return string(chars)
}

// TestProperty_ChecksumValidation_NonExistentFile tests checksum validation for non-existent files
// TestProperty_ChecksumValidation_NonExistentFile 测试不存在文件的校验和验证
func TestProperty_ChecksumValidation_NonExistentFile(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random non-existent file path / 生成随机的不存在文件路径
		tempDir := t.TempDir()
		nonExistentFile := filepath.Join(tempDir, rapid.StringMatching(`[a-z]{5,15}\.tar\.gz`).Draw(rt, "filename"))

		// Generate a random checksum / 生成随机校验和
		checksumBytes := make([]byte, 32)
		for i := range checksumBytes {
			checksumBytes[i] = byte(rapid.IntRange(0, 255).Draw(rt, "checksumByte"))
		}
		checksum := hex.EncodeToString(checksumBytes)

		// Property: VerifyChecksum should fail for non-existent files
		// 属性：VerifyChecksum 应该对不存在的文件失败
		manager := NewInstallerManager()
		err := manager.VerifyChecksum(nonExistentFile, checksum)
		if err == nil {
			rt.Fatal("VerifyChecksum should fail for non-existent file")
		}

		// Property: CalculateChecksum should also fail for non-existent files
		// 属性：CalculateChecksum 也应该对不存在的文件失败
		_, err = CalculateChecksum(nonExistentFile)
		if err == nil {
			rt.Fatal("CalculateChecksum should fail for non-existent file")
		}
	})
}
