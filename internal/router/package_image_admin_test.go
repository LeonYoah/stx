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

package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/admin"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestPackageImageWriteRoutesRequireAdmin 用内存 sqlite 与真实用户行验证管理员中间件：
// 非管理员 403、管理员放行。镜像写接口的供应链闸门行为即此中间件。
// TestPackageImageWriteRoutesRequireAdmin verifies the admin middleware against in-memory
// sqlite with real user rows: non-admin yields 403, admin passes.
func TestPackageImageWriteRoutesRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&auth.User{}))
	adminUser := &auth.User{Username: "img-admin", IsAdmin: true}
	normalUser := &auth.User{Username: "img-user", IsAdmin: false}
	require.NoError(t, database.Create(adminUser).Error)
	require.NoError(t, database.Create(normalUser).Error)
	previous := db.GetGlobalDB()
	db.SetGlobalDB(database)
	defer db.SetGlobalDB(previous)

	build := func(user *auth.User) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyUser, user)
			c.Request = c.Request.WithContext(context.Background())
			c.Next()
		})
		r.POST("/api/v1/packages/images", admin.LoginAdminRequired(), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
		recorder := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/packages/images", nil)
		r.ServeHTTP(recorder, req)
		return recorder
	}
	assert.Equal(t, http.StatusForbidden, build(normalUser).Code, "non-admin must be 403")
	assert.Equal(t, http.StatusOK, build(adminUser).Code, "admin must pass")
}
