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
const RegistryRevision = 2

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
	{
		ID:           "auth.user-info.get",
		CommandPath:  []string{"auth", "user-info", "get"},
		Summary:      "Get current user information",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/auth/user-info",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx auth user-info get",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "auth.user-info.get",
  "request_id": "req_example",
  "data": {"id":1,"username":"admin","is_active":true,"is_admin":true},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "admin.user.list",
		CommandPath:  []string{"admin", "user", "list"},
		Summary:      "List users",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/admin/users",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		AdminOnly:    true,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "current", Location: InputQuery, Required: true, Description: "Page number starting from 1"},
			{Name: "size", Location: InputQuery, Required: true, Description: "Page size from 1 to 100"},
			{Name: "username", Location: InputQuery, Required: false, Description: "Username prefix filter"},
			{Name: "is_active", Location: InputQuery, Required: false, Description: "Active state filter"},
			{Name: "is_admin", Location: InputQuery, Required: false, Description: "Administrator state filter"},
		},
		Example: "stx admin user list --current 1 --size 20",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "admin.user.list",
  "request_id": "req_example",
  "data": {"total":1,"users":[{"id":1,"username":"admin","is_admin":true}]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "admin.user.get",
		CommandPath:  []string{"admin", "user", "get"},
		Summary:      "Get one user",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/admin/users/:id",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		AdminOnly:    true,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "User ID"},
		},
		Example: "stx admin user get 1",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "admin.user.get",
  "request_id": "req_example",
  "data": {"id":1,"username":"admin","is_active":true,"is_admin":true},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "dashboard.overview.get",
		CommandPath:  []string{"dashboard", "overview", "get"},
		Summary:      "Get dashboard overview",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/dashboard/overview",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx dashboard overview get",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "dashboard.overview.get",
  "request_id": "req_example",
  "data": {"stats":{"total_hosts":1},"cluster_summaries":[],"host_summaries":[],"recent_activities":[]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "dashboard.stats.get",
		CommandPath:  []string{"dashboard", "stats", "get"},
		Summary:      "Get dashboard statistics",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/dashboard/overview/stats",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx dashboard stats get",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "dashboard.stats.get",
  "request_id": "req_example",
  "data": {"total_hosts":1,"online_hosts":1,"total_clusters":1,"running_clusters":1},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "dashboard.cluster.list",
		CommandPath:  []string{"dashboard", "cluster", "list"},
		Summary:      "List dashboard cluster summaries",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/dashboard/overview/clusters",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx dashboard cluster list",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "dashboard.cluster.list",
  "request_id": "req_example",
  "data": [{"id":6,"name":"example","status":"running","total_nodes":1}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "dashboard.host.list",
		CommandPath:  []string{"dashboard", "host", "list"},
		Summary:      "List dashboard host summaries",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/dashboard/overview/hosts",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx dashboard host list",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "dashboard.host.list",
  "request_id": "req_example",
  "data": [{"id":10,"name":"node-1","is_online":true,"agent_status":"online"}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "dashboard.activity.list",
		CommandPath:  []string{"dashboard", "activity", "list"},
		Summary:      "List recent dashboard activities",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/dashboard/overview/activities",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx dashboard activity list",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "dashboard.activity.list",
  "request_id": "req_example",
  "data": [{"id":1,"type":"success","message":"example","timestamp":"2026-09-19T00:00:00Z"}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "host.list",
		CommandPath:  []string{"host", "list"},
		Summary:      "List hosts",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/hosts",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "current", Location: InputQuery, Required: false, Description: "Page number starting from 1"},
			{Name: "size", Location: InputQuery, Required: false, Description: "Page size from 1 to 100"},
			{Name: "name", Location: InputQuery, Required: false, Description: "Host name filter"},
			{Name: "host_type", Location: InputQuery, Required: false, Description: "Host type filter"},
			{Name: "ip_address", Location: InputQuery, Required: false, Description: "IP address filter"},
			{Name: "status", Location: InputQuery, Required: false, Description: "Host status filter"},
			{Name: "agent_status", Location: InputQuery, Required: false, Description: "Agent status filter"},
			{Name: "is_online", Location: InputQuery, Required: false, Description: "Online state filter"},
		},
		Example: "stx host list --current 1 --size 20",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.list",
  "request_id": "req_example",
  "data": {"total":1,"hosts":[{"id":1,"name":"node-1"}]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "host.get",
		CommandPath:  []string{"host", "get"},
		Summary:      "Get one host",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/hosts/:id",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Host ID"},
		},
		Example: "stx host get 1",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.get",
  "request_id": "req_example",
  "data": {"id":1,"name":"node-1"},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "host.discovery.process.list",
		CommandPath:  []string{"host", "discovery", "process", "list"},
		Summary:      "List SeaTunnel processes discovered on a host",
		GeneratedCLI: true,
		Method:       "POST",
		Route:        "/api/v1/hosts/:id/discover-processes",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		UsesAgent:    true,
		SupportsPick: true,
		Impact: &ImpactSpec{
			Level:       RiskR0,
			Message:     "Runs a read-only SeaTunnel process scan on the target host through STX Agent.",
			Performance: "Reads local process metadata; it does not stop or modify SeaTunnel processes.",
		},
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Host ID"},
		},
		Example: "stx host discovery process list 10",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.discovery.process.list",
  "request_id": "req_example",
  "data": {"success":true,"message":"process discovery completed / 进程发现完成","processes":[{"pid":12345,"role":"master","install_dir":"/opt/seatunnel","version":"2.3.13","hazelcast_port":5801,"api_port":8080}]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "cluster.list",
		CommandPath:  []string{"cluster", "list"},
		Summary:      "List clusters",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/clusters",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "current", Location: InputQuery, Required: false, Description: "Page number starting from 1"},
			{Name: "size", Location: InputQuery, Required: false, Description: "Page size from 1 to 100"},
			{Name: "name", Location: InputQuery, Required: false, Description: "Cluster name filter"},
			{Name: "status", Location: InputQuery, Required: false, Description: "Cluster status filter"},
			{Name: "deployment_mode", Location: InputQuery, Required: false, Description: "Deployment mode filter"},
		},
		Example: "stx cluster list --current 1 --size 20",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "cluster.list",
  "request_id": "req_example",
  "data": {"total":1,"clusters":[{"id":6,"name":"example"}]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "cluster.get",
		CommandPath:  []string{"cluster", "get"},
		Summary:      "Get one cluster",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/clusters/:id",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
		},
		Example: "stx cluster get 6",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "cluster.get",
  "request_id": "req_example",
  "data": {"id":6,"name":"example","version":"2.3.13"},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "cluster.node.list",
		CommandPath:  []string{"cluster", "node", "list"},
		Summary:      "List cluster nodes",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/clusters/:id/nodes",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
		},
		Example: "stx cluster node list 6",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "cluster.node.list",
  "request_id": "req_example",
  "data": [{"id":1,"cluster_id":6,"role":"master"}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "cluster.status.get",
		CommandPath:  []string{"cluster", "status", "get"},
		Summary:      "Get cluster status",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/clusters/:id/status",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
		},
		Example: "stx cluster status get 6",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "cluster.status.get",
  "request_id": "req_example",
  "data": {"cluster_id":6,"status":"running"},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "config.cluster.list",
		CommandPath:  []string{"config", "cluster", "list"},
		Summary:      "List cluster configs",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/clusters/:id/configs",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
		},
		Example: "stx config cluster list 6",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "config.cluster.list",
  "request_id": "req_example",
  "data": [{"id":1,"cluster_id":6,"config_type":"seatunnel"}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "config.get",
		CommandPath:  []string{"config", "get"},
		Summary:      "Get one config",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/configs/:id",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Config ID"},
		},
		Example: "stx config get 1",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "config.get",
  "request_id": "req_example",
  "data": {"id":1,"cluster_id":6,"config_type":"seatunnel"},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "config.version.list",
		CommandPath:  []string{"config", "version", "list"},
		Summary:      "List config versions",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/configs/:id/versions",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Config ID"},
		},
		Example: "stx config version list 1",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "config.version.list",
  "request_id": "req_example",
  "data": [{"id":1,"config_id":1,"version":1}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "execution.get",
		CommandPath:  []string{"execution", "get"},
		Method:       "GET",
		Route:        "/api/v1/executions/:id",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Shared execution ID"},
		},
		Example: "stx execution get 11111111-1111-1111-1111-111111111111",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "execution.get",
  "request_id": "req_example",
  "data": {"execution_id":"11111111-1111-1111-1111-111111111111","status":"running","cancellable":true},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "execution.wait",
		CommandPath:  []string{"execution", "wait"},
		Method:       "GET",
		Route:        "/api/v1/executions/:id/wait",
		Mode:         ModeWatch,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		Async:        true,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Shared execution ID"},
			{Name: "timeout_seconds", Location: InputQuery, Required: false, Description: "One server wait interval from 1 to 30 seconds"},
		},
		Example: "stx execution wait 11111111-1111-1111-1111-111111111111 --timeout 10m",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "execution.wait",
  "request_id": "req_example",
  "data": {"execution_id":"11111111-1111-1111-1111-111111111111","status":"succeeded","cancellable":false},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "execution.cancel",
		CommandPath:  []string{"execution", "cancel"},
		Method:       "POST",
		Route:        "/api/v1/executions/:id/cancel",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		Async:        true,
		SupportsPick: true,
		Impact: &ImpactSpec{
			Level:       RiskR1,
			Message:     "Cancellation stops later work, but the current safe step may need to finish first.",
			Performance: "The running task can remain active while cancellation is being confirmed.",
		},
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Shared execution ID"},
			{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable retry key for this cancellation request"},
			{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation of the cancellation impact"},
		},
		Example: "stx execution cancel 11111111-1111-1111-1111-111111111111 --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "execution.cancel",
  "request_id": "req_example",
  "data": {"execution_id":"11111111-1111-1111-1111-111111111111","status":"cancelling","cancellable":false},
  "result_meta": {"complete": true}
}`,
	},
}

var routeExceptions = []RouteException{
	{Method: "POST", Route: "/api/v1/oauth/callback", Mode: ModeServerOnly, Reason: "OAuth provider callback consumed by the STX server"},
	{Method: "POST", Route: "/api/v1/hosts/:id/discover", Mode: ModeServerOnly, Reason: "Legacy cluster discovery discards the Agent result and always returns an empty cluster list"},
	{Method: "POST", Route: "/api/v1/hosts/:id/discover/confirm", Mode: ModeServerOnly, Reason: "Legacy cluster import is not wired with a ClusterMatcher and cannot complete successfully"},
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
