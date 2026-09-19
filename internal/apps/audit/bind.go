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

package audit

import (
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// BindOperator 把当前登录用户和请求编号写进请求上下文。
// 同一次 HTTP 操作里下发的 Agent 命令会带上同一个 request_id，审计行才能对上服务器命令。
// BindOperator stores the current user and request ID on the request context.
// Agent commands issued by the same HTTP operation share that request ID with the audit row.
func BindOperator(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	user := auth.GetUserFromContext(c)
	if user == nil || user.ID == 0 {
		return
	}
	requestID := c.GetHeader("X-Request-ID")
	if requestID == "" {
		requestID = uuid.NewString()
		c.Request.Header.Set("X-Request-ID", requestID)
	}
	ctx := WithCommandMetadata(c.Request.Context(), CommandMetadata{
		RequestID:     requestID,
		OwnerUserID:   uint(user.ID),
		OwnerUsername: user.Username,
		ClientType:    auditClientType(c.GetHeader("X-STX-Client"), c.GetHeader("User-Agent")),
	})
	c.Request = c.Request.WithContext(ctx)
}
