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

// Package capability 提供当前服务端操作能力查询。
// Package capability provides discovery of operations supported by the current server.
package capability

import (
	"net/http"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/operation"
	stxversion "github.com/LeonYoah/stx/internal/version"
	"github.com/gin-gonic/gin"
)

// Operation 描述一个操作对当前用户是否可用。
// Operation describes whether an operation is available to the current user.
type Operation struct {
	OperationID string                  `json:"operation_id"`
	Revision    int                     `json:"revision"`
	Allowed     bool                    `json:"allowed"`
	DenialCode  string                  `json:"denial_code,omitempty"`
	Mode        operation.OperationMode `json:"mode"`
	Risk        operation.RiskLevel     `json:"risk"`
	Impact      *operation.ImpactSpec   `json:"impact,omitempty"`
}

// Data 是能力查询返回的版本和操作列表。
// Data contains server versions and the supported operation list.
type Data struct {
	APIVersion       string      `json:"api_version"`
	ServerVersion    string      `json:"server_version"`
	GitCommit        string      `json:"git_commit"`
	BuildTime        string      `json:"build_time"`
	MinCLIVersion    string      `json:"min_cli_version"`
	RegistryRevision string      `json:"registry_revision"`
	Operations       []Operation `json:"operations"`
}

// Response 是能力查询的标准 API 响应。
// Response is the standard API response for capability discovery.
type Response struct {
	ErrorMsg string `json:"error_msg"`
	Data     Data   `json:"data"`
}

// List 返回服务端登记操作及其针对当前用户的权限结果。
// List returns registered server operations and their permission result for the current user.
// @Tags capability
// @Produce json
// @Success 200 {object} Response
// @Router /api/v1/capabilities [get]
func List(c *gin.Context) {
	user := auth.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, auth.ErrorResponse{ErrorMsg: "未登录", Data: nil})
		return
	}

	specs := operation.Registry()
	operations := make([]Operation, 0, len(specs))
	for _, spec := range specs {
		allowed, denialCode := permissionForOperation(spec, user)
		item := Operation{
			OperationID: spec.ID,
			Revision:    spec.Revision,
			Allowed:     allowed,
			DenialCode:  denialCode,
			Mode:        spec.Mode,
			Risk:        spec.Risk,
			Impact:      spec.Impact,
		}
		operations = append(operations, item)
	}

	info := stxversion.Current()
	c.JSON(http.StatusOK, Response{Data: Data{
		APIVersion:       "v1",
		ServerVersion:    info.Version,
		GitCommit:        info.GitCommit,
		BuildTime:        info.BuildTime,
		MinCLIVersion:    stxversion.Normalize(stxversion.MinCLIVersion),
		RegistryRevision: operation.RegistryDigest(),
		Operations:       operations,
	}})
}

// permissionForOperation 返回操作对当前用户是否可用及拒绝原因。
// permissionForOperation returns whether an operation is available and why it is denied.
func permissionForOperation(spec operation.OperationSpec, user *auth.User) (bool, string) {
	if spec.AdminOnly && (user == nil || !user.IsAdmin) {
		return false, "admin_required"
	}
	return true, ""
}
