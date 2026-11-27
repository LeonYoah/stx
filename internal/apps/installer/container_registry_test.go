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
	"net/http"
	"net/http/httptest"
	"strings"
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
	assert.Equal(t, "harbor.example.com", RegistryImageHost("Harbor.Example.Com/team/seatunnel:3.1.0"))
	assert.Equal(t, "192.168.49.4:5000", RegistryImageHost("192.168.49.4:5000/seatunnel:3.1.0"))
	assert.Equal(t, "localhost", RegistryImageHost("localhost/seatunnel:3.1.0"))
	assert.Equal(t, "localhost:5000", RegistryImageHost("localhost:5000/seatunnel:3.1.0"))
	assert.Equal(t, "", RegistryImageHost(""))
}

func TestNormalizeAndAuthKey(t *testing.T) {
	assert.Equal(t, "harbor.example.com", NormalizeRegistryHost("https://Harbor.Example.com/"))
	assert.Equal(t, "192.168.49.4:5000", NormalizeRegistryHost("http://192.168.49.4:5000/"))
	assert.Equal(t, "https://index.docker.io/v1/", DockerConfigAuthKey("docker.io"))
	assert.Equal(t, "https://index.docker.io/v1/", DockerConfigAuthKey("index.docker.io"))
	assert.Equal(t, "harbor.example.com", DockerConfigAuthKey("https://harbor.example.com/"))
}

func TestSortImageTags(t *testing.T) {
	got := sortImageTags([]string{"latest", "2.3.12", "3.1.1-hotfix", "3.1.1", "3.1.0", "nightly", "10.0.0"})
	assert.Equal(t, []string{"10.0.0", "3.1.1", "3.1.1-hotfix", "3.1.0", "2.3.12", "latest", "nightly"}, got)
}

func newRegistryTestService(t *testing.T) *Service {
	t.Helper()
	secretbox.SetSecret("test-secret")
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&ContainerRegistry{}))
	return &Service{db: database}
}

func TestContainerRegistryCRUDAndGuards(t *testing.T) {
	svc := newRegistryTestService(t)
	ctx := context.Background()

	// host 清洗 + 默认镜像路径清洗 + 凭据加密落库。
	// host & default_repo normalization + encrypted password at rest.
	row, err := svc.CreateContainerRegistry(ctx, &ContainerRegistryRequest{
		Host: "http://192.168.49.4:5000/", DefaultRepo: " /bigdata/seatunnel/ ", Username: "u", Password: "plaintext-pw-x9",
	})
	require.NoError(t, err)
	assert.Equal(t, "192.168.49.4:5000", row.Host)
	assert.Equal(t, "bigdata/seatunnel", row.DefaultRepo)
	assert.NotContains(t, row.Password, "plaintext-pw-x9")
	assert.True(t, row.HasAuth)
	assert.Equal(t, "error", row.ProbeStatus)

	// 默认镜像路径拒绝携带 tag/digest/协议。
	// default_repo rejects tags, digests, and schemes.
	_, err = svc.CreateContainerRegistry(ctx, &ContainerRegistryRequest{
		Host: "other.example.com", DefaultRepo: "bigdata/seatunnel:3.1.0",
	})
	require.ErrorIs(t, err, errRegistryValidation)

	// 重复 host 拒绝（409 哨兵）。
	// duplicate hosts rejected with ErrRegistryAlreadyExists.
	_, err = svc.CreateContainerRegistry(ctx, &ContainerRegistryRequest{Host: "192.168.49.4:5000"})
	require.ErrorIs(t, err, errRegistryAlreadyExists)

	// scheme/路径校验。
	// path/whitespace host validation.
	_, err = svc.CreateContainerRegistry(ctx, &ContainerRegistryRequest{Host: "bad host/x"})
	require.ErrorIs(t, err, errRegistryValidation)

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
	assert.False(t, anon.HasAuth)

	// 匿名仓库切回认证时未填密码必须报错（杜绝 username!="" && password=="" 半截脏态）。
	// switching anonymous back to authenticated without a password must fail.
	_, err = svc.UpdateContainerRegistry(ctx, row.ID, &ContainerRegistryRequest{
		Host: "192.168.49.4:5000", Username: "u3", Password: "",
	})
	require.ErrorIs(t, err, errRegistryValidation)

	// 删除放行。
	// delete passes.
	require.NoError(t, svc.DeleteContainerRegistry(ctx, row.ID))
}

func TestSystemRegistryProtected(t *testing.T) {
	svc := newRegistryTestService(t)
	ctx := context.Background()
	require.NoError(t, svc.db.Create(&ContainerRegistry{Host: DockerHubRegistryHost, IsSystem: true, ProbeStatus: "public"}).Error)
	var row ContainerRegistry
	require.NoError(t, svc.db.Where("host = ?", DockerHubRegistryHost).First(&row).Error)

	// 系统行允许补充凭据，但禁止改 host 或删除。
	// System row permits adding credentials, while host change and deletion are rejected.
	_, err := svc.UpdateContainerRegistry(ctx, row.ID, &ContainerRegistryRequest{Host: "evil.example.com", Username: "u", Password: "pw"})
	require.ErrorIs(t, err, errSystemRegistryProtected)

	updated, err := svc.UpdateContainerRegistry(ctx, row.ID, &ContainerRegistryRequest{Host: DockerHubRegistryHost, Username: "hubuser", Password: "plaintext-pw-x9"})
	require.NoError(t, err)
	assert.True(t, updated.HasAuth)
	// docker.io 配凭据后会真正发探测请求，无网络/错凭据不再谎报 public。
	// Authenticated docker.io actually probes instead of faking public.
	assert.Equal(t, "error", updated.ProbeStatus)

	require.ErrorIs(t, svc.DeleteContainerRegistry(ctx, row.ID), errSystemRegistryProtected)
}

func TestProbeRegistryTransientEditReusesStoredPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "robot" || pass != "secret-pw" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/_catalog") {
			_, _ = w.Write([]byte(`{"repositories":["team/seatunnel"]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")

	svc := newRegistryTestService(t)
	ctx := context.Background()
	created, err := svc.CreateContainerRegistry(ctx, &ContainerRegistryRequest{
		Host: host, Username: "robot", Password: "secret-pw",
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", created.ProbeStatus)

	// 匿名探测需要鉴权的私仓必须报 error，不再假报 ok。
	// Anonymous probe against an auth-gated private registry must report error rather than ok.
	anonRes, err := svc.ProbeRegistryTransient(ctx, &ContainerRegistryProbeRequest{
		Host: host,
	})
	require.NoError(t, err)
	assert.Equal(t, "error", anonRes.Status)
	assert.NotEmpty(t, anonRes.Error)

	// 编辑态测试连接：传 id 且 password 留空，必须复用库内密码并探测成功，且不扫 /v2/_catalog。
	// Edit-mode transient probe with ID and empty password reuses stored secret and succeeds without sweeping /v2/_catalog.
	res, err := svc.ProbeRegistryTransient(ctx, &ContainerRegistryProbeRequest{
		ID: created.ID, Host: host, Username: "robot", Password: "",
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", res.Status)
	assert.Empty(t, res.Error)
}

func TestFindRegistryByHost(t *testing.T) {
	svc := newRegistryTestService(t)
	ctx := context.Background()
	require.NoError(t, svc.db.Create(&ContainerRegistry{Host: "harbor.example.com", Username: "u"}).Error)
	found, err := svc.FindRegistryByHost(ctx, "https://Harbor.Example.com/")
	require.NoError(t, err)
	assert.Equal(t, "harbor.example.com", found.Host)
	_, err = svc.FindRegistryByHost(ctx, "missing.example.com")
	require.ErrorIs(t, err, errRegistryNotFound)
}
