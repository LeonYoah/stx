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

package capability

import (
	"net/http"

	stxversion "github.com/LeonYoah/stx/internal/version"
	"github.com/gin-gonic/gin"
)

// VersionData 是公开的产品版本信息（不含操作能力列表）。
// VersionData is public product version info without the operation capability list.
type VersionData struct {
	Version       string `json:"version"`
	GitCommit     string `json:"git_commit"`
	BuildTime     string `json:"build_time"`
	MinCLIVersion string `json:"min_cli_version"`
}

// VersionResponse 是产品版本查询的标准 API 响应。
// VersionResponse is the standard API response for product version discovery.
type VersionResponse struct {
	ErrorMsg string      `json:"error_msg"`
	Data     VersionData `json:"data"`
}

// Version 返回当前服务端产品版本，无需登录（版本非敏感）。
// Version returns the current server product version without authentication (version is non-sensitive).
// @Tags capability
// @Produce json
// @Success 200 {object} VersionResponse
// @Router /api/v1/version [get]
func Version(c *gin.Context) {
	info := stxversion.Current()
	c.JSON(http.StatusOK, VersionResponse{Data: VersionData{
		Version:       info.Version,
		GitCommit:     info.GitCommit,
		BuildTime:     info.BuildTime,
		MinCLIVersion: stxversion.Normalize(stxversion.MinCLIVersion),
	}})
}
