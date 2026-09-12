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

// Package session 会话存储属性测试
package session

import (
	"context"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: stx-platform-login, Property 5: Session store consistency**
// **Validates: Requirements 4.3**
// 测试内存会话存储的一致性行为：对于任何会话操作（创建、读取、删除），
// MemoryStore 都应满足相同的基本行为约束。

// TestProperty_SessionStoreSetGetConsistency 测试 Set 后 Get 应返回相同的值
func TestProperty_SessionStoreSetGetConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// 创建内存存储实例
	store := NewMemoryStore()
	ctx := context.Background()

	// 属性：对于任意 key 和 string value，Set 后 Get 应返回相同的值
	properties.Property("Set then Get returns same string value", prop.ForAll(
		func(key string, value string) bool {
			if key == "" {
				return true // 跳过空 key
			}

			// Set 值
			err := store.Set(ctx, key, value, 0)
			if err != nil {
				return false
			}

			// Get 值
			got, err := store.Get(ctx, key)
			if err != nil {
				return false
			}

			// 验证值相等
			return got == value
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestProperty_SessionStoreDeleteRemovesKey 测试 Delete 后 key 不再存在
func TestProperty_SessionStoreDeleteRemovesKey(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	store := NewMemoryStore()
	ctx := context.Background()

	// 属性：对于任意 key，Set 后 Delete 应使 key 不存在
	properties.Property("Delete removes key from store", prop.ForAll(
		func(key string, value string) bool {
			if key == "" {
				return true
			}

			// Set 值
			err := store.Set(ctx, key, value, 0)
			if err != nil {
				return false
			}

			// 验证 key 存在
			exists, err := store.Exists(ctx, key)
			if err != nil || !exists {
				return false
			}

			// Delete key
			err = store.Delete(ctx, key)
			if err != nil {
				return false
			}

			// 验证 key 不存在
			exists, err = store.Exists(ctx, key)
			if err != nil {
				return false
			}

			return !exists
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestProperty_SessionStoreExistsConsistency 测试 Exists 方法的一致性
func TestProperty_SessionStoreExistsConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	store := NewMemoryStore()
	ctx := context.Background()

	// 属性：对于任意 key，Set 后 Exists 应返回 true
	properties.Property("Exists returns true after Set", prop.ForAll(
		func(key string, value string) bool {
			if key == "" {
				return true
			}

			// Set 值
			err := store.Set(ctx, key, value, 0)
			if err != nil {
				return false
			}

			// 验证 Exists 返回 true
			exists, err := store.Exists(ctx, key)
			if err != nil {
				return false
			}

			return exists
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.AlphaString(),
	))

	// 属性：对于任意不存在的 key，Exists 应返回 false
	properties.Property("Exists returns false for non-existent key", prop.ForAll(
		func(key string) bool {
			if key == "" {
				return true
			}

			// 使用新的 store 确保 key 不存在
			freshStore := NewMemoryStore()

			exists, err := freshStore.Exists(ctx, key)
			if err != nil {
				return false
			}

			return !exists
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
	))

	properties.TestingRun(t)
}

// TestProperty_SessionStoreExpiration 测试过期功能
func TestProperty_SessionStoreExpiration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50 // 减少测试次数因为涉及时间等待

	properties := gopter.NewProperties(parameters)

	store := NewMemoryStore()
	ctx := context.Background()

	// 属性：设置短过期时间后，key 应该过期
	properties.Property("Key expires after expiration time", prop.ForAll(
		func(key string, value string) bool {
			if key == "" {
				return true
			}

			// Set 值，过期时间为 10 毫秒
			err := store.Set(ctx, key, value, 10*time.Millisecond)
			if err != nil {
				return false
			}

			// 立即检查应该存在
			exists, err := store.Exists(ctx, key)
			if err != nil || !exists {
				return false
			}

			// 等待过期
			time.Sleep(20 * time.Millisecond)

			// 过期后应该不存在
			exists, err = store.Exists(ctx, key)
			if err != nil {
				return false
			}

			return !exists
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestProperty_SessionStoreGetNotFoundError 测试获取不存在的 key 返回正确错误
func TestProperty_SessionStoreGetNotFoundError(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	ctx := context.Background()

	// 属性：对于任意不存在的 key，Get 应返回 ErrKeyNotFound
	properties.Property("Get returns ErrKeyNotFound for non-existent key", prop.ForAll(
		func(key string) bool {
			if key == "" {
				return true
			}

			// 使用新的 store 确保 key 不存在
			freshStore := NewMemoryStore()

			_, err := freshStore.Get(ctx, key)

			return err == ErrKeyNotFound
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
	))

	properties.TestingRun(t)
}

// TestProperty_SessionStoreOverwrite 测试覆盖写入
func TestProperty_SessionStoreOverwrite(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	store := NewMemoryStore()
	ctx := context.Background()

	// 属性：对于同一个 key，后写入的值应覆盖先写入的值
	properties.Property("Later Set overwrites earlier Set", prop.ForAll(
		func(key string, value1 string, value2 string) bool {
			if key == "" {
				return true
			}

			// 第一次 Set
			err := store.Set(ctx, key, value1, 0)
			if err != nil {
				return false
			}

			// 第二次 Set（覆盖）
			err = store.Set(ctx, key, value2, 0)
			if err != nil {
				return false
			}

			// Get 应返回第二次的值
			got, err := store.Get(ctx, key)
			if err != nil {
				return false
			}

			return got == value2
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}
