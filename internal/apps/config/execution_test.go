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

package config

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestConfigUpdateIsIdempotentForCLI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&Config{}, &ConfigVersion{}, &executionapp.Execution{}, &executionapp.Confirmation{}, &audit.AuditLog{}, &auth.User{}); err != nil {
		t.Fatal(err)
	}
	user := &auth.User{ID: 7, Username: "config-cli", PasswordHash: "unused", IsActive: true, IsAdmin: true}
	if err := database.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(database)
	service := NewService(repo, nil, nil, nil)
	created, err := service.Create(context.Background(), &CreateConfigRequest{ClusterID: 6, ConfigType: ConfigTypeJVMOptions, Content: "-Xms1g", Comment: "initial"}, uint(user.ID))
	if err != nil {
		t.Fatal(err)
	}
	executionService := executionapp.NewService(executionapp.NewRepository(database), executionapp.NewProviderRegistry())
	executionService.SetAuditRepository(audit.NewRepository(database))
	handler := NewHandler(service)
	handler.SetExecutionService(executionService)
	router := gin.New()
	router.Use(func(c *gin.Context) { auth.SetUserToContext(c, user); c.Next() })
	router.PUT("/api/v1/configs/:id", handler.UpdateConfig)
	path := fmt.Sprintf("/api/v1/configs/%d", created.ID)
	body := []byte(`{"content":"-Xms2g","comment":"cli update"}`)
	for range 2 {
		request := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-STX-Client", "cli")
		request.Header.Set("X-STX-Confirm", "true")
		request.Header.Set("Idempotency-Key", "same-key")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("幂等更新失败 / idempotent update failed: code=%d body=%s", response.Code, response.Body.String())
		}
	}
	stored, err := repo.GetByID(context.Background(), created.ID)
	if err != nil || stored.Version != 2 || stored.UpdatedBy != uint(user.ID) {
		t.Fatalf("配置更新结果错误 / invalid config update result: config=%#v err=%v", stored, err)
	}
	var versionCount int64
	if err := database.Model(&ConfigVersion{}).Where("config_id = ?", created.ID).Count(&versionCount).Error; err != nil || versionCount != 2 {
		t.Fatalf("重复请求创建了额外版本 / repeated request created an extra version: count=%d err=%v", versionCount, err)
	}
}
