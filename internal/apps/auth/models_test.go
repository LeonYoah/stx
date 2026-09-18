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

// Package auth 用户认证模块属性测试
package auth

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: stx-platform-login, Property 4: Password storage uses bcrypt**
// **Validates: Requirements 2.3**
// 对于任何存储在数据库中的用户密码，存储的值应该是一个有效的 bcrypt 哈希，
// 可以验证原始密码

// TestProperty_PasswordStorageUsesBcrypt 测试密码存储使用 bcrypt
func TestProperty_PasswordStorageUsesBcrypt(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// 属性：对于任意有效密码，SetPassword 后应生成有效的 bcrypt 哈希
	properties.Property("SetPassword generates valid bcrypt hash", prop.ForAll(
		func(password string) bool {
			user := &User{}
			err := user.SetPassword(password, DefaultBcryptCost)
			if err != nil {
				return false
			}

			// 验证生成的哈希是有效的 bcrypt 格式
			return user.IsValidBcryptHash()
		},
		// 生成长度在 6-50 之间的密码（满足最小长度要求）
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) >= 6 && len(s) <= 50 }),
	))

	// 属性：对于任意有效密码，SetPassword 后 CheckPassword 应返回 true
	properties.Property("CheckPassword verifies original password", prop.ForAll(
		func(password string) bool {
			user := &User{}
			err := user.SetPassword(password, DefaultBcryptCost)
			if err != nil {
				return false
			}

			// 验证原始密码
			return user.CheckPassword(password)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) >= 6 && len(s) <= 50 }),
	))

	// 属性：对于任意两个不同的密码，CheckPassword 应正确区分
	properties.Property("CheckPassword rejects wrong password", prop.ForAll(
		func(baseLen int, suffixLen int) bool {
			// 生成固定长度的密码
			password1 := ""
			for i := 0; i < baseLen; i++ {
				password1 += string(rune('a' + (i % 26)))
			}
			// 通过添加后缀确保两个密码不同
			suffix := ""
			for i := 0; i < suffixLen; i++ {
				suffix += "X"
			}
			password2 := password1 + suffix

			user := &User{}
			err := user.SetPassword(password1, DefaultBcryptCost)
			if err != nil {
				return false
			}

			// 使用错误密码验证应返回 false
			return !user.CheckPassword(password2)
		},
		gen.IntRange(6, 20), // 密码长度 6-20
		gen.IntRange(1, 5),  // 后缀长度 1-5
	))

	// 属性：相同密码多次哈希应生成不同的哈希值（bcrypt 使用随机盐）
	properties.Property("Same password generates different hashes", prop.ForAll(
		func(password string) bool {
			user1 := &User{}
			user2 := &User{}

			err := user1.SetPassword(password, DefaultBcryptCost)
			if err != nil {
				return false
			}

			err = user2.SetPassword(password, DefaultBcryptCost)
			if err != nil {
				return false
			}

			// 两次哈希应该不同（因为 bcrypt 使用随机盐）
			// 但两个哈希都应该能验证原始密码
			return user1.PasswordHash != user2.PasswordHash &&
				user1.CheckPassword(password) &&
				user2.CheckPassword(password)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) >= 6 && len(s) <= 50 }),
	))

	properties.TestingRun(t)
}

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "english", input: "en", want: "en"},
		{name: "english uppercase", input: "EN", want: "en"},
		{name: "empty defaults zh", input: "", want: DefaultLanguage},
		{name: "unknown defaults zh", input: "fr", want: DefaultLanguage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeLanguage(tt.input); got != tt.want {
				t.Fatalf("NormalizeLanguage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestUser_ToUserInfo_NormalizesLanguage(t *testing.T) {
	user := &User{
		ID:       1,
		Username: "tester",
		Language: "",
	}

	info := user.ToUserInfo()
	if info == nil {
		t.Fatal("expected user info")
	}
	if info.Language != DefaultLanguage {
		t.Fatalf("expected normalized language %q, got %q", DefaultLanguage, info.Language)
	}
}

// TestProperty_EmptyPasswordRejected 测试空密码被拒绝
func TestProperty_EmptyPasswordRejected(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// 属性：空密码应被拒绝
	properties.Property("Empty password is rejected", prop.ForAll(
		func(_ int) bool {
			user := &User{}
			err := user.SetPassword("", DefaultBcryptCost)
			return err == ErrEmptyCredentials
		},
		gen.Int(), // 使用 int 生成器只是为了运行多次
	))

	// 属性：短密码（少于6位）应被拒绝
	properties.Property("Short password is rejected", prop.ForAll(
		func(length int) bool {
			// 生成指定长度的密码
			password := ""
			for i := 0; i < length; i++ {
				password += "a"
			}
			user := &User{}
			err := user.SetPassword(password, DefaultBcryptCost)
			return err == ErrPasswordTooShort
		},
		gen.IntRange(1, 5), // 生成 1-5 长度的密码
	))

	properties.TestingRun(t)
}

// TestProperty_CheckPasswordWithEmptyInputs 测试空输入的密码验证
func TestProperty_CheckPasswordWithEmptyInputs(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// 属性：空密码验证应返回 false
	properties.Property("CheckPassword returns false for empty password", prop.ForAll(
		func(password string) bool {
			user := &User{}
			err := user.SetPassword(password, DefaultBcryptCost)
			if err != nil {
				return false
			}

			// 空密码验证应返回 false
			return !user.CheckPassword("")
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) >= 6 && len(s) <= 50 }),
	))

	// 属性：空哈希验证应返回 false
	properties.Property("CheckPassword returns false for empty hash", prop.ForAll(
		func(password string) bool {
			user := &User{PasswordHash: ""}

			// 空哈希验证应返回 false
			return !user.CheckPassword(password)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) >= 6 && len(s) <= 50 }),
	))

	properties.TestingRun(t)
}
