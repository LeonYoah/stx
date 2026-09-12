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

// Package session 提供会话管理功能。
package session

import (
	"log"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
)

// Store 全局会话存储实例。
var Store SessionStore

// GinStore 全局 Gin 会话存储实例（用于 HTTP 会话）。
var GinStore sessions.Store

// StoreType 会话存储类型。
type StoreType string

const (
	// StoreTypeMemory 内存存储。
	StoreTypeMemory StoreType = "memory"
)

// InitSessionStore 初始化会话存储。
// STX 默认使用内存 SessionStore + Cookie Gin 会话。
func InitSessionStore() error {
	appConfig := config.Config.App
	log.Println("[Session] 使用内存会话存储")
	return initMemoryStore(appConfig)
}

func initMemoryStore(appConfig config.AppConfig) error {
	Store = NewMemoryStore()

	ginStore := cookie.NewStore([]byte(appConfig.SessionSecret))
	ginStore.Options(sessions.Options{
		Path:     "/",
		Domain:   appConfig.SessionDomain,
		MaxAge:   appConfig.SessionAge,
		HttpOnly: appConfig.SessionHttpOnly,
		Secure:   appConfig.SessionSecure,
	})

	GinStore = ginStore
	return nil
}

// GetStoreType 获取当前会话存储类型。
func GetStoreType() StoreType {
	return StoreTypeMemory
}
