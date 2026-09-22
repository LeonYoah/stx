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

package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOAuthPendingStatesPersistAcrossCookieSessionRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		sessionName = "oauth_test"
		state       = "github:test-state"
	)
	secret := []byte("test-secret-for-oauth-state")
	expiresAt := time.Now().Add(time.Minute).Unix()

	writeEngine := gin.New()
	writeEngine.Use(sessions.Sessions(sessionName, cookie.NewStore(secret)))
	writeEngine.GET("/write", func(c *gin.Context) {
		ginSession := sessions.Default(c)
		states := map[string]int64{state: expiresAt}
		require.NoError(t, writeOAuthPendingStates(ginSession, states))
		require.NoError(t, ginSession.Save())
		c.Status(http.StatusNoContent)
	})

	writeReq := httptest.NewRequest(http.MethodGet, "/write", nil)
	writeResp := httptest.NewRecorder()
	writeEngine.ServeHTTP(writeResp, writeReq)
	cookieHeader := writeResp.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookieHeader)
	require.Equal(t, http.StatusNoContent, writeResp.Code)

	readEngine := gin.New()
	readEngine.Use(sessions.Sessions(sessionName, cookie.NewStore(secret)))
	readEngine.GET("/read", func(c *gin.Context) {
		states := loadOAuthPendingStates(sessions.Default(c))
		c.JSON(http.StatusOK, states)
	})

	readReq := httptest.NewRequest(http.MethodGet, "/read", nil)
	readReq.Header.Set("Cookie", cookieHeader)
	readResp := httptest.NewRecorder()
	readEngine.ServeHTTP(readResp, readReq)
	require.Equal(t, http.StatusOK, readResp.Code)

	var got map[string]int64
	require.NoError(t, json.Unmarshal(readResp.Body.Bytes(), &got))
	require.Equal(t, map[string]int64{state: expiresAt}, got)
}

func TestFindOrCreateOAuthUserDoesNotLinkExistingUsername(t *testing.T) {
	t.Setenv("GO_TEST", "1")

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "oauth_test.db")

	config.Config.Database.Enabled = true
	config.Config.Database.Type = db.DatabaseTypeSQLite
	config.Config.Database.SQLitePath = dbPath

	require.NoError(t, db.InitDatabase())
	t.Cleanup(func() {
		require.NoError(t, db.CloseDatabase())
	})

	require.NoError(t, db.DB(ctx).AutoMigrate(&auth.User{}))

	existing := &auth.User{Username: "admin", PasswordHash: "hashed", IsActive: true}
	require.NoError(t, db.DB(ctx).Create(existing).Error)

	info := &OAuthUserInfo{
		Provider:  string(ProviderGitHub),
		ID:        "attacker-id",
		Username:  "admin",
		Name:      "attacker",
		AvatarURL: "https://avatar.example/attacker.png",
		Email:     "attacker@example.com",
	}

	user, err := findOrCreateOAuthUser(ctx, info)
	require.NoError(t, err)
	require.NotEqual(t, existing.ID, user.ID)
	require.Equal(t, "admin_github_attacker-id", user.Username)
	require.Equal(t, "github:attacker-id", user.OAuthID)

	var unchanged auth.User
	require.NoError(t, db.DB(ctx).First(&unchanged, existing.ID).Error)
	require.Empty(t, unchanged.OAuthID)

	// 重复登录测试：使用相同 OAuth ID 再次调用 findOrCreateOAuthUser，必须返回同一用户，不能重复创建新用户
	// Repeated login test: calling findOrCreateOAuthUser again with the same OAuth ID must return the same user and not create a duplicate
	secondUser, err := findOrCreateOAuthUser(ctx, info)
	require.NoError(t, err)
	require.Equal(t, user.ID, secondUser.ID)
	require.Equal(t, user.Username, secondUser.Username)

	var totalOAuthUsers int64
	require.NoError(t, db.DB(ctx).Model(&auth.User{}).Where("oauth_id = ?", "github:attacker-id").Count(&totalOAuthUsers).Error)
	require.Equal(t, int64(1), totalOAuthUsers)
}
