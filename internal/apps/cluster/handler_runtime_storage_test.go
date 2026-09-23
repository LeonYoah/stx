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

package cluster

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGetRuntimeStorageReturnsNotFoundForMissingCluster(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cluster-handler.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("创建测试数据库失败 / creating test database failed: %v", err)
	}
	if err := database.AutoMigrate(&Cluster{}, &ClusterNode{}); err != nil {
		t.Fatalf("迁移测试数据库失败 / migrating test database failed: %v", err)
	}

	service := NewService(NewRepository(database), nil, &ServiceConfig{})
	handler := NewHandler(service, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/clusters/:id/runtime-storage", handler.GetRuntimeStorage)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/999/runtime-storage", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("不存在的集群状态码错误 / missing cluster returned wrong status: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
