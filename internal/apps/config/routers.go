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
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册配置管理路由
func RegisterRoutes(router *gin.RouterGroup, handler *Handler) {
	// 集群配置路由
	clusters := router.Group("/clusters", auth.LoginRequired())
	{
		clusters.GET("/:id/configs", handler.GetClusterConfigs)
		clusters.POST("/:id/configs/init", handler.InitClusterConfigs)
		clusters.POST("/:id/configs/sync-all", handler.SyncTemplateToAllNodes)
	}

	// 配置操作路由
	configs := router.Group("/configs", auth.LoginRequired())
	{
		configs.POST("/normalize", handler.NormalizeConfig)
		configs.GET("/:id", handler.GetConfig)
		configs.PUT("/:id", handler.UpdateConfig)
		configs.GET("/:id/versions", handler.GetConfigVersions)
		configs.POST("/:id/rollback", handler.RollbackConfig)
		configs.POST("/:id/promote", handler.PromoteConfig)
		configs.POST("/:id/sync", handler.SyncFromTemplate)
		configs.POST("/:id/push", handler.PushConfigToNode)
	}
}
