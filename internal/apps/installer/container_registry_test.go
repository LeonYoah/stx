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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/utils/secretbox"
)

func TestRegistryImageHost(t *testing.T) {
	assert.Equal(t, DockerHubRegistryHost, RegistryImageHost("apache/seatunnel:3.1.0"))
	assert.Equal(t, DockerHubRegistryHost, RegistryImageHost("seatunnel:3.1.0"))
	assert.Equal(t, "harbor.example.com", RegistryImageHost("harbor.example.com/team/seatunnel:3.1.0"))
	assert.Equal(t, "192.168.49.4:5000", RegistryImageHost("192.168.49.4:5000/seatunnel:3.1.0"))
	assert.Equal(t, "localhost", RegistryImageHost("localhost/seatunnel:3.1.0"))
	assert.Equal(t, "localhost:5000", RegistryImageHost("localhost:5000/seatunnel:3.1.0"))
	assert.Equal(t, "", RegistryImageHost(""))
}

func TestNormalizeRegistryHost(t *testing.T) {
	assert.Equal(t, "harbor.example.com", NormalizeRegistryHost("https://harbor.example.com/"))
	assert.Equal(t, "192.168.49.4:5000", NormalizeRegistryHost("http://192.168.49.4:5000/"))
	assert.Equal(t, "harbor.example.com", NormalizeRegistryHost("harbor.example.com"))
}

func newRegistryTestService(t *testing.T) *Service {
	t.Helper()
	secretbox.SetSecret("test-secret")
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&ContainerRegistry{}))
	return &Service{db: database}
}

func TestContainerRegistryCRUD(t *testing.T) {
	svc := newRegistryTestService(t)
	ctx := context.Background()

	// host 清洗 + 凭据加密落库。
	// host normalization + encrypted password at rest.
	row, err := svc.CreateContainerRegistry(ctx, &ContainerRegistryRequest{
		Host: "http://192.168.49.4:5000/", Username: "u", Password: "plaintext-pw-x9",
	})
	require.NoError(t, err)
	assert.Equal(t, "192.168.49.4:5000", row.Host)
	assert.NotContains(t, row.Password, "plaintext-pw-x9")
	assert.True(t, row.HasAuth)
	// 探测会失败（无真实仓库），但不影响创建。
	// the probe fails without a real registry but creation is unaffected.
	assert.Equal(t, "error", row.ProbeStatus)

	// 重复 host 拒绝。
	// duplicate hosts are rejected.
	_, err = svc.CreateContainerRegistry(ctx, &ContainerRegistryRequest{Host: "192.168.49.4:5000"})
	require.Error(t, err)

	// scheme 校验。
	// scheme-less host validation.
	_, err = svc.CreateContainerRegistry(ctx, &ContainerRegistryRequest{Host: "bad host/x"})
	require.Error(t, err)

	// 更新：密码留空保持原值。
	// update: empty password keeps the stored one.
	updated, err := svc.UpdateContainerRegistry(ctx, row.ID, &ContainerRegistryRequest{
		Host: "192.168.49.4:5000", Username: "u2",
	})
	require.NoError(t, err)
	assert.Equal(t, "u2", updated.Username)
	assert.NotEmpty(t, updated.Password, "password must be kept")

	// 清空用户名 = 切匿名，密码一并清除。
	// clearing the username switches to anonymous and drops the password.
	anon, err := svc.UpdateContainerRegistry(ctx, row.ID, &ContainerRegistryRequest{
		Host: "192.168.49.4:5000", Username: "",
	})
	require.NoError(t, err)
	assert.Empty(t, anon.Password)

	// 删除放行。
	// delete passes.
	require.NoError(t, svc.DeleteContainerRegistry(ctx, row.ID))
}

func TestSystemRegistryProtected(t *testing.T) {
	svc := newRegistryTestService(t)
	ctx := context.Background()
	require.NoError(t, svc.db.Create(&ContainerRegistry{Host: DockerHubRegistryHost, IsSystem: true}).Error)
	var row ContainerRegistry
	require.NoError(t, svc.db.Where("host = ?", DockerHubRegistryHost).First(&row).Error)
	_, err := svc.UpdateContainerRegistry(ctx, row.ID, &ContainerRegistryRequest{Host: "x"})
	require.Error(t, err, "system registry must not be editable")
	require.Error(t, svc.DeleteContainerRegistry(ctx, row.ID), "system registry must not be deletable")
}

func TestFindRegistryByHost(t *testing.T) {
	svc := newRegistryTestService(t)
	ctx := context.Background()
	require.NoError(t, svc.db.Create(&ContainerRegistry{Host: "harbor.example.com", Username: "u"}).Error)
	found, err := svc.FindRegistryByHost(ctx, "https://harbor.example.com/")
	require.NoError(t, err)
	assert.Equal(t, "harbor.example.com", found.Host)
	_, err = svc.FindRegistryByHost(ctx, "missing.example.com")
	require.ErrorIs(t, err, errRegistryNotFound)
}
