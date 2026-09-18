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

package operation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// RegistryRevision 是操作登记表的兼容修订号。
// RegistryRevision is the compatibility revision of the operation registry.
const RegistryRevision = 1

var registry = []OperationSpec{
	{
		ID:           "auth.cli.login",
		CommandPath:  []string{"login"},
		Method:       "POST",
		Route:        "/api/v1/auth/cli/login",
		Mode:         ModeNormal,
		AuthRequired: false,
		Risk:         RiskR0,
		Revision:     1,
		Example:      "stx login --server https://stx.example.com --username admin --password-stdin",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "auth.cli.login",
  "request_id": "req_example",
  "data": {"namespace":"default","token_set":true},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "auth.cli.logout",
		CommandPath:  []string{"logout"},
		Method:       "POST",
		Route:        "/api/v1/auth/cli/logout",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		Example:      "stx logout",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "auth.cli.logout",
  "request_id": "req_example",
  "data": {"namespace":"default","revoked":true},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "auth.cli.whoami",
		CommandPath:  []string{"whoami"},
		Method:       "GET",
		Route:        "/api/v1/auth/cli/whoami",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx whoami",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "auth.cli.whoami",
  "request_id": "req_example",
  "data": {"id":1,"username":"admin"},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "capability.list",
		CommandPath:  []string{"capability", "list"},
		Method:       "GET",
		Route:        "/api/v1/capabilities",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx capability list",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "capability.list",
  "request_id": "req_example",
  "data": {"server_version":"0.1.0","operations":[]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "health.get",
		CommandPath:  []string{"health"},
		Method:       "GET",
		Route:        "/api/v1/health",
		Mode:         ModeNormal,
		AuthRequired: false,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx health",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "health.get",
  "request_id": "req_example",
  "data": {},
  "result_meta": {
    "complete": true
  }
}`,
	},
}

var routeExceptions = []RouteException{
	{Method: "POST", Route: "/api/v1/oauth/callback", Mode: ModeServerOnly, Reason: "OAuth provider callback consumed by the STX server"},
	{Method: "GET", Route: "/api/v1/monitoring/prometheus/discovery", Mode: ModeServerOnly, Reason: "Prometheus HTTP service discovery endpoint"},
	{Method: "POST", Route: "/api/v1/monitoring/alertmanager/webhook", Mode: ModeServerOnly, Reason: "Alertmanager webhook receiver"},
	{Method: "POST", Route: "/api/v1/sync/preview/collect", Mode: ModeServerOnly, Reason: "SeaTunnel preview result callback"},
	{Method: "ANY", Route: "/api/v1/clusters/:id/webui", Mode: ModeProxy, Reason: "SeaTunnel WebUI reverse proxy"},
	{Method: "ANY", Route: "/api/v1/clusters/:id/webui/*proxyPath", Mode: ModeProxy, Reason: "SeaTunnel WebUI reverse proxy"},
	{Method: "ANY", Route: "/api/v1/monitoring/proxy/grafana", Mode: ModeProxy, Reason: "Grafana reverse proxy"},
	{Method: "ANY", Route: "/api/v1/monitoring/proxy/grafana/*proxyPath", Mode: ModeProxy, Reason: "Grafana reverse proxy"},
	{Method: "GET", Route: "/api/v1/diagnostics/tasks/:id/events/stream", Mode: ModeWatch, Reason: "Diagnostic task event stream"},
	{Method: "GET", Route: "/api/v1/st-upgrade/tasks/:id/events/stream", Mode: ModeWatch, Reason: "STX upgrade task event stream"},
	{Method: "GET", Route: "/api/v1/agent/install.sh", Mode: ModeDownload, Reason: "Agent installation script download"},
	{Method: "GET", Route: "/api/v1/agent/uninstall.sh", Mode: ModeDownload, Reason: "Agent uninstallation script download"},
	{Method: "GET", Route: "/api/v1/agent/ca.crt", Mode: ModeDownload, Reason: "Agent CA certificate download"},
	{Method: "GET", Route: "/api/v1/agent/download", Mode: ModeDownload, Reason: "Agent binary download"},
	{Method: "GET", Route: "/api/v1/diagnostics/tasks/:id/html", Mode: ModeDownload, Reason: "Diagnostic HTML report response"},
	{Method: "GET", Route: "/api/v1/diagnostics/tasks/:id/files/*path", Mode: ModeDownload, Reason: "Diagnostic artifact file response"},
	{Method: "GET", Route: "/api/v1/diagnostics/tasks/:id/bundle", Mode: ModeDownload, Reason: "Diagnostic bundle download"},
	{Method: "GET", Route: "/api/v1/stx/install.sh", Mode: ModeDownload, Reason: "STX installation script download"},
	{Method: "GET", Route: "/api/v1/stx/download", Mode: ModeDownload, Reason: "STX release bundle download"},
}

// Registry 返回登记表的副本，调用方不能修改包内数据。
// Registry returns a copy so callers cannot mutate package-owned data.
func Registry() []OperationSpec {
	result := make([]OperationSpec, len(registry))
	copy(result, registry)
	for i := range result {
		result[i].CommandPath = append([]string(nil), result[i].CommandPath...)
		result[i].Input = append([]InputSpec(nil), result[i].Input...)
		if result[i].Impact != nil {
			impact := *result[i].Impact
			result[i].Impact = &impact
		}
	}
	return result
}

// RouteExceptions 返回特殊路由例外的副本。
// RouteExceptions returns a copy of the special-route exceptions.
func RouteExceptions() []RouteException {
	result := make([]RouteException, len(routeExceptions))
	copy(result, routeExceptions)
	return result
}

// RegistryDigest 返回登记表内容的稳定 SHA-256 标识。
// RegistryDigest returns a stable SHA-256 identifier for the registry contents.
func RegistryDigest() string {
	content, err := json.Marshal(registry)
	if err != nil {
		panic(fmt.Sprintf("encode operation registry: %v", err))
	}
	digest := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", digest[:])
}
