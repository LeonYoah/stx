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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
)

func TestBindOperatorSharesRequestWithCommandMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/8", nil)
	c.Request.Header.Set("User-Agent", "Mozilla/5.0")
	auth.SetUserToContext(c, &auth.User{ID: 7, Username: "admin"})

	BindOperator(c)

	metadata := CommandMetadataFromContext(c.Request.Context())
	if metadata.RequestID == "" || metadata.OwnerUserID != 7 || metadata.OwnerUsername != "admin" || metadata.ClientType != "web" {
		t.Fatalf("operator metadata = %+v", metadata)
	}
	if c.GetHeader("X-Request-ID") != metadata.RequestID {
		t.Fatalf("request header %q != metadata %q", c.GetHeader("X-Request-ID"), metadata.RequestID)
	}
}

func TestWithCommandMetadataKeepsExistingRequestID(t *testing.T) {
	ctx := WithCommandMetadata(t.Context(), CommandMetadata{RequestID: "request-1", OwnerUserID: 3, ClientType: "cli"})
	ctx = WithCommandMetadata(ctx, CommandMetadata{ExecutionID: "execution-1"})
	metadata := CommandMetadataFromContext(ctx)
	if metadata.RequestID != "request-1" || metadata.ExecutionID != "execution-1" || metadata.OwnerUserID != 3 || metadata.ClientType != "cli" {
		t.Fatalf("merged metadata = %+v", metadata)
	}
}
