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

package execution

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderConfirm        = "X-STX-Confirm"
	HeaderConfirmationID = "X-STX-Confirmation-ID"
	HeaderRequestID      = "X-Request-ID"
)

// RequestMetadata 保存写操作所需的请求头信息。
// RequestMetadata stores request header values required by write operations.
type RequestMetadata struct {
	RequestID      string
	IdempotencyKey string
	Confirmed      bool
	ConfirmationID string
	ClientType     string
}

// MetadataFromGin 从请求头读取安全执行元数据，并为缺失的请求编号生成 UUID。
// MetadataFromGin reads safe-execution metadata from headers and creates a UUID when the request ID is absent.
func MetadataFromGin(c *gin.Context) RequestMetadata {
	requestID := strings.TrimSpace(c.GetHeader(HeaderRequestID))
	if requestID == "" {
		requestID = uuid.NewString()
	}
	return RequestMetadata{
		RequestID:      requestID,
		IdempotencyKey: strings.TrimSpace(c.GetHeader(HeaderIdempotencyKey)),
		Confirmed:      strings.EqualFold(strings.TrimSpace(c.GetHeader(HeaderConfirm)), "true"),
		ConfirmationID: strings.TrimSpace(c.GetHeader(HeaderConfirmationID)),
		ClientType:     requestClientType(c),
	}
}

func requestClientType(c *gin.Context) string {
	marker := strings.ToLower(strings.TrimSpace(c.GetHeader("X-STX-Client")))
	if marker == "cli" {
		return "cli"
	}
	if marker == "api" {
		return "api"
	}
	if strings.TrimSpace(c.GetHeader("User-Agent")) == "" {
		return "api"
	}
	return "web"
}
