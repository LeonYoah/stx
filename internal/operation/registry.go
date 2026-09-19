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
	"strings"
)

// RegistryRevision 是操作登记表的兼容修订号。
// RegistryRevision is the compatibility revision of the operation registry.
const RegistryRevision = 6

var registry = append([]OperationSpec{
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
		ID:           "auth.profile.update",
		CommandPath:  []string{"auth", "profile", "update"},
		Summary:      "Update the current user profile",
		GeneratedCLI: false,
		Method:       "PUT",
		Route:        "/api/v1/auth/profile",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		SupportsPick: true,
		Impact: &ImpactSpec{
			Level:   RiskR1,
			Message: "修改当前用户的邮箱或语言偏好，可再次修改恢复。",
		},
		Input: []InputSpec{
			{Name: "email", Location: InputBody, Required: false, Description: "Email address"},
			{Name: "language", Location: InputBody, Required: false, Description: "Language preference: zh or en"},
		},
		Example: "stx auth profile update --language en --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "auth.profile.update",
  "request_id": "req_example",
  "data": {"id":1,"username":"admin","language":"en"},
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
		ID:           "admin.user.create",
		CommandPath:  []string{"admin", "user", "create"},
		Summary:      "Create a user",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/admin/users",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		AdminOnly:    true,
		Revision:     1,
		SupportsPick: true,
		Impact: &ImpactSpec{
			Level:   RiskR1,
			Message: "创建用户会新增一个可登录 STX 的账号。",
		},
		Input: []InputSpec{
			{Name: "username", Location: InputBody, Required: true, Description: "Username"},
			{Name: "password", Location: InputBody, Required: true, Description: "Password read from hidden terminal input or stdin"},
			{Name: "nickname", Location: InputBody, Required: false, Description: "Display name"},
			{Name: "email", Location: InputBody, Required: false, Description: "Email address"},
			{Name: "is_admin", Location: InputBody, Required: false, Description: "Administrator flag"},
		},
		Example: "stx admin user create --username test-user --password-stdin --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "admin.user.create",
  "request_id": "req_example",
  "data": {"id":2,"username":"test-user","is_active":true,"is_admin":false},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "admin.user.update",
		CommandPath:  []string{"admin", "user", "update"},
		Summary:      "Update a user",
		GeneratedCLI: false,
		Method:       "PUT",
		Route:        "/api/v1/admin/users/:id",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		AdminOnly:    true,
		Revision:     1,
		SupportsPick: true,
		Impact: &ImpactSpec{
			Level:   RiskR1,
			Message: "更新用户可能改变账号状态、管理员权限或登录密码。",
		},
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "User ID"},
			{Name: "nickname", Location: InputBody, Required: false, Description: "Display name"},
			{Name: "email", Location: InputBody, Required: false, Description: "Email address"},
			{Name: "is_active", Location: InputBody, Required: false, Description: "Active flag"},
			{Name: "is_admin", Location: InputBody, Required: false, Description: "Administrator flag"},
			{Name: "password", Location: InputBody, Required: false, Description: "New password read from stdin"},
		},
		Example: "stx admin user update 2 --active=false --admin=false --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "admin.user.update",
  "request_id": "req_example",
  "data": {"id":2,"username":"test-user","is_active":false,"is_admin":false},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "admin.user.delete",
		CommandPath:  []string{"admin", "user", "delete"},
		Summary:      "Delete a user",
		GeneratedCLI: false,
		Method:       "DELETE",
		Route:        "/api/v1/admin/users/:id",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR2,
		AdminOnly:    true,
		Revision:     1,
		Impact: &ImpactSpec{
			Level:   RiskR2,
			Message: "删除用户后无法通过 STX 恢复。",
		},
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "User ID"},
		},
		Example: "stx admin user delete 2 --confirm --idempotency-key delete-user-2",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "admin.user.delete",
  "request_id": "req_example",
  "data": {},
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
		ID:           "host.agent.install-command.get",
		CommandPath:  []string{"host", "agent", "install-command", "get"},
		Summary:      "Get the STX Agent installation command for a host",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/hosts/:id/install-command",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Host ID"},
		},
		Example: "stx host agent install-command get 10",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.agent.install-command.get",
  "request_id": "req_example",
  "data": {"command":"curl -fsSL http://stx.example/api/v1/agent/install.sh | sudo bash"},
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
		ID:           "host.install.status.get",
		CommandPath:  []string{"host", "install", "status", "get"},
		Summary:      "Get host installation status",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/hosts/:id/install/status",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Host ID"},
		},
		Example: "stx host install status get 10",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.install.status.get",
  "request_id": "req_example",
  "data": {"id":"install_example","host_id":"10","status":"running","current_step":"install","progress":60},
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
		ID:           "cluster.plugin.list",
		CommandPath:  []string{"cluster", "plugin", "list"},
		Summary:      "List plugins installed on a cluster",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/clusters/:id/plugins",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
		},
		Example: "stx cluster plugin list 6",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "cluster.plugin.list",
  "request_id": "req_example",
  "data": [{"cluster_id":6,"plugin_name":"jdbc","artifact_id":"connector-jdbc","version":"2.3.13","status":"installed"}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "cluster.plugin.progress.get",
		CommandPath:  []string{"cluster", "plugin", "progress", "get"},
		Summary:      "Get plugin installation progress on a cluster",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/clusters/:id/plugins/:name/progress",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
			{Name: "name", Location: InputPath, Required: true, Description: "Plugin name"},
		},
		Example: "stx cluster plugin progress get 6 jdbc",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "cluster.plugin.progress.get",
  "request_id": "req_example",
  "data": {"plugin_name":"jdbc","status":"completed","progress":100},
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
		ID:           "package.list",
		CommandPath:  []string{"package", "list"},
		Summary:      "List available SeaTunnel packages",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/packages",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx package list",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.list",
  "request_id": "req_example",
  "data": {"versions":["2.3.13"],"recommended_version":"2.3.13","local_packages":[{"version":"2.3.13","is_local":true}]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "package.get",
		CommandPath:  []string{"package", "get"},
		Summary:      "Get one SeaTunnel package",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/packages/:version",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "version", Location: InputPath, Required: true, Description: "SeaTunnel version"},
		},
		Example: "stx package get 2.3.13",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.get",
  "request_id": "req_example",
  "data": {"version":"2.3.13","file_name":"apache-seatunnel-2.3.13-bin.tar.gz","file_size":450628193,"is_local":true},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "package.version.refresh",
		CommandPath:  []string{"package", "version", "refresh"},
		Summary:      "Refresh available SeaTunnel versions",
		GeneratedCLI: true,
		Method:       "POST",
		Route:        "/api/v1/packages/versions/refresh",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx package version refresh",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.version.refresh",
  "request_id": "req_example",
  "data": ["2.3.13"],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "package.upload",
		CommandPath:  []string{"package", "upload"},
		Summary:      "Upload a SeaTunnel package",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/packages/upload",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		Impact: &ImpactSpec{Level: RiskR1, Message: "Writes a package into STX local storage and does not overwrite an existing version.",
			Performance: "Uses network bandwidth and disk I/O while uploading."},
		Input: []InputSpec{
			{Name: "version", Location: InputBody, Required: true, Description: "SeaTunnel version"},
			{Name: "file", Location: InputFile, Required: true, Description: "Package archive path"},
		},
		Example: "stx package upload ./apache-seatunnel-2.3.13-bin.tar.gz --version 2.3.13 --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.upload",
  "request_id": "req_example",
  "data": {"version":"2.3.13","is_local":true},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "package.upload.chunk",
		CommandPath:  []string{"package", "upload", "chunk"},
		Summary:      "Upload one SeaTunnel package chunk",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/packages/upload/chunk",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		Impact: &ImpactSpec{Level: RiskR1, Message: "Writes a package chunk into the STX temporary directory.",
			Performance: "Uses network bandwidth and disk I/O while uploading."},
		Example: "stx package upload chunk ./part-000 --version 2.3.13 --upload-id upload_12345678 --chunk-index 0 --total-chunks 2 --total-size 1024 --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.upload.chunk",
  "request_id": "req_example",
  "data": {"upload_id":"upload_12345678","completed":false,"received_chunks":1,"total_chunks":2},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "package.delete",
		CommandPath:  []string{"package", "delete"},
		Summary:      "Delete a local SeaTunnel package",
		GeneratedCLI: false,
		Method:       "DELETE",
		Route:        "/api/v1/packages/:version",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		Impact:       &ImpactSpec{Level: RiskR1, Message: "Permanently deletes the local package file."},
		Input:        []InputSpec{{Name: "version", Location: InputPath, Required: true, Description: "SeaTunnel version"}},
		Example:      "stx package delete 9.9.91 --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.delete",
  "request_id": "req_example",
  "data": null,
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "package.download.start",
		CommandPath:  []string{"package", "download", "start"},
		Summary:      "Start a SeaTunnel package download",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/packages/download",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		Async:        true,
		Impact: &ImpactSpec{Level: RiskR1, Message: "Downloads a package into STX local storage.",
			Performance: "Uses server network bandwidth and disk I/O until the download finishes."},
		Example: "stx package download start 2.3.13 --mirror apache --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.download.start",
  "request_id": "req_example",
  "data": {"version":"2.3.13","status":"downloading","execution_id":"11111111-1111-1111-1111-111111111111"},
  "result_meta": {"complete": true,"next_command":"stx package download get 2.3.13"}
}`,
	},
	{
		ID:           "package.download.cancel",
		CommandPath:  []string{"package", "download", "cancel"},
		Summary:      "Cancel a SeaTunnel package download",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/packages/download/:version/cancel",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		Impact:       &ImpactSpec{Level: RiskR1, Message: "Stops the active download and removes its partial file."},
		Input:        []InputSpec{{Name: "version", Location: InputPath, Required: true, Description: "SeaTunnel version"}},
		Example:      "stx package download cancel 2.3.13 --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.download.cancel",
  "request_id": "req_example",
  "data": {"version":"2.3.13","status":"cancelled"},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "package.download.list",
		CommandPath:  []string{"package", "download", "list"},
		Summary:      "List SeaTunnel package download tasks",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/packages/downloads",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx package download list",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.download.list",
  "request_id": "req_example",
  "data": [{"version":"2.3.13","status":"completed","progress":100}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "package.download.get",
		CommandPath:  []string{"package", "download", "get"},
		Summary:      "Get a SeaTunnel package download task",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/packages/download/:version",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "version", Location: InputPath, Required: true, Description: "SeaTunnel version"},
		},
		Example: "stx package download get 2.3.13",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.download.get",
  "request_id": "req_example",
  "data": {"version":"2.3.13","status":"completed","progress":100},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "plugin.list",
		CommandPath:  []string{"plugin", "list"},
		Summary:      "List available SeaTunnel plugins",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/plugins",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "version", Location: InputQuery, Required: false, Description: "SeaTunnel version"},
			{Name: "mirror", Location: InputQuery, Required: false, Description: "Mirror source: apache, aliyun, or huaweicloud"},
		},
		Example: "stx plugin list --version 2.3.13 --mirror apache",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "plugin.list",
  "request_id": "req_example",
  "data": {"version":"2.3.13","total":85,"mirror":"apache","source":"database","cache_hit":true,"plugins":[{"name":"jdbc","artifact_id":"connector-jdbc"}]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "plugin.get",
		CommandPath:  []string{"plugin", "get"},
		Summary:      "Get one SeaTunnel plugin",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/plugins/:name",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "name", Location: InputPath, Required: true, Description: "Plugin name"},
			{Name: "version", Location: InputQuery, Required: false, Description: "SeaTunnel version"},
		},
		Example: "stx plugin get jdbc --version 2.3.13",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "plugin.get",
  "request_id": "req_example",
  "data": {"name":"jdbc","artifact_id":"connector-jdbc","version":"2.3.13"},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "plugin.local.list",
		CommandPath:  []string{"plugin", "local", "list"},
		Summary:      "List locally cached SeaTunnel plugins",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/plugins/local",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx plugin local list",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "plugin.local.list",
  "request_id": "req_example",
  "data": [{"name":"jdbc","artifact_id":"connector-jdbc","version":"2.3.13"}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "plugin.download.list",
		CommandPath:  []string{"plugin", "download", "list"},
		Summary:      "List active SeaTunnel plugin downloads",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/plugins/downloads",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Example:      "stx plugin download list",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "plugin.download.list",
  "request_id": "req_example",
  "data": [{"plugin_name":"jdbc","status":"downloading","progress":50}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "plugin.download.status.get",
		CommandPath:  []string{"plugin", "download", "status", "get"},
		Summary:      "Get a SeaTunnel plugin download task",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/plugins/:name/download/status",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "name", Location: InputPath, Required: true, Description: "Plugin name"},
			{Name: "version", Location: InputQuery, Required: true, Description: "SeaTunnel version"},
			{Name: "profile_keys", Location: InputQuery, Required: false, Repeated: true, Description: "Dependency profile key; may be specified more than once"},
		},
		Example: "stx plugin download status get jdbc --version 2.3.13 --profile_keys mysql --profile_keys postgresql",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "plugin.download.status.get",
  "request_id": "req_example",
  "data": {"plugin_name":"jdbc","version":"2.3.13","status":"not_started","progress":0,"selected_profile_keys":["mysql","postgresql"]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "plugin.dependency.list",
		CommandPath:  []string{"plugin", "dependency", "list"},
		Summary:      "List configured dependencies for a SeaTunnel plugin",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/plugins/:name/dependencies",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "name", Location: InputPath, Required: true, Description: "Plugin name"},
			{Name: "version", Location: InputQuery, Required: false, Description: "SeaTunnel version"},
		},
		Example: "stx plugin dependency list jdbc --version 2.3.13",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "plugin.dependency.list",
  "request_id": "req_example",
  "data": [{"plugin_name":"jdbc","group_id":"mysql","artifact_id":"mysql-connector-java","version":"8.0.27"}],
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "plugin.official-dependency.list",
		CommandPath:  []string{"plugin", "official-dependency", "list"},
		Summary:      "List official dependencies for a SeaTunnel plugin",
		GeneratedCLI: true,
		Method:       "GET",
		Route:        "/api/v1/plugins/:name/official-dependencies",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		SupportsPick: true,
		Input: []InputSpec{
			{Name: "name", Location: InputPath, Required: true, Description: "Plugin name"},
			{Name: "version", Location: InputQuery, Required: false, Description: "SeaTunnel version"},
			{Name: "profile_key", Location: InputQuery, Required: false, Description: "Dependency profile key"},
		},
		Example: "stx plugin official-dependency list jdbc --version 2.3.13 --profile_key mysql",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "plugin.official-dependency.list",
  "request_id": "req_example",
  "data": {"plugin_name":"jdbc","seatunnel_version":"2.3.13","dependency_status":"ready_exact","dependency_count":1},
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
}, clusterAdditionalOperationSpecs()...)

// clusterAdditionalOperationSpecs 返回集群模块新增的读写操作登记。
// clusterAdditionalOperationSpecs returns the additional cluster read and write registrations.
func clusterAdditionalOperationSpecs() []OperationSpec {
	return []OperationSpec{
		clusterBodyOperation("cluster.create", []string{"cluster", "create"}, "Create a cluster", "POST", "/api/v1/clusters", RiskR1,
			"创建集群会在 STX 中新增部署定义，但不会自动启动 SeaTunnel 进程。",
			[]InputSpec{
				{Name: "name", Location: InputBody, Required: true, Description: "Cluster name"},
				{Name: "deployment_mode", Location: InputBody, Required: true, Description: "Deployment mode"},
				{Name: "version", Location: InputBody, Required: true, Description: "SeaTunnel version"},
			}, "stx cluster create --name demo --deployment-mode hybrid --version 2.3.13 --confirm"),
		clusterBodyOperation("cluster.update", []string{"cluster", "update"}, "Update a cluster", "PUT", "/api/v1/clusters/:id", RiskR1,
			"修改集群定义会影响后续部署和进程操作，已经运行的进程不会自动重启。",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Fields to update"}},
			"stx cluster update 6 --description test --confirm"),
		clusterGeneratedOperation("cluster.delete", []string{"cluster", "delete"}, "Delete a cluster", "DELETE", "/api/v1/clusters/:id", RiskR2,
			"删除集群会移除集群定义和节点记录；集群必须先停止，force_delete 还会请求 Agent 删除节点安装目录。", "stx cluster delete 6 --confirm", []InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "force_delete", Location: InputQuery, Required: false, Description: "Remove node installation directories after deletion; the cluster must already be stopped"},
			}),
		clusterBodyOperation("cluster.node.add", []string{"cluster", "node", "add"}, "Add a cluster node", "POST", "/api/v1/clusters/:id/nodes", RiskR1,
			"新增节点会修改集群部署定义，并可能在目标主机执行预检查。",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Node definition"}},
			"stx cluster node add 6 --host-id 10 --role master/worker --confirm"),
		clusterBodyOperation("cluster.node.add-batch", []string{"cluster", "node", "add-batch"}, "Add multiple cluster nodes", "POST", "/api/v1/clusters/:id/nodes/batch", RiskR1,
			"批量新增节点会一次修改多个节点定义，并可能在目标主机执行预检查。",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Batch node definition"}},
			"stx cluster node add-batch 6 --request-file nodes.json --confirm"),
		clusterBodyOperation("cluster.node.update", []string{"cluster", "node", "update"}, "Update a cluster node", "PUT", "/api/v1/clusters/:id/nodes/:nodeId", RiskR1,
			"修改节点端口、目录或 JVM 覆盖值会影响该节点后续启动。",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "nodeId", Location: InputPath, Required: true, Description: "Node ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Node fields to update"}},
			"stx cluster node update 6 1 --hazelcast-port 5801 --confirm"),
		clusterGeneratedOperation("cluster.node.remove", []string{"cluster", "node", "remove"}, "Remove a cluster node", "DELETE", "/api/v1/clusters/:id/nodes/:nodeId", RiskR2,
			"移除节点会删除节点定义；如果节点仍在运行，应先停止节点。", "stx cluster node remove 6 1 --confirm", []InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "nodeId", Location: InputPath, Required: true, Description: "Node ID"},
			}),
		clusterBodyOperation("cluster.node.precheck", []string{"cluster", "node", "precheck"}, "Precheck a cluster node", "POST", "/api/v1/clusters/:id/nodes/precheck", RiskR1,
			"节点预检查会连接目标 Agent，并执行目录、端口和运行环境检查。",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Node precheck request"}},
			"stx cluster node precheck 6 --host-id 10 --role master/worker --confirm"),
		clusterGeneratedOperation("cluster.node.logs", []string{"cluster", "node", "logs"}, "Get cluster node logs", "GET", "/api/v1/clusters/:id/nodes/:nodeId/logs", RiskR0,
			"读取较多日志会消耗 Agent、网络和 STX 服务资源。", "stx cluster node logs 6 1 --lines 100 --mode tail", []InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "nodeId", Location: InputPath, Required: true, Description: "Node ID"},
				{Name: "lines", Location: InputQuery, Required: false, Description: "Number of log lines"}, {Name: "mode", Location: InputQuery, Required: false, Description: "Read mode: tail, head, or all"},
				{Name: "filter", Location: InputQuery, Required: false, Description: "Log filter pattern"}, {Name: "date", Location: InputQuery, Required: false, Description: "Rolling log date suffix"},
			}),
		clusterGeneratedOperation("cluster.start", []string{"cluster", "start"}, "Start a cluster", "POST", "/api/v1/clusters/:id/start", RiskR1,
			"启动集群会在所有节点创建 SeaTunnel 进程并占用 CPU、内存和端口。", "stx cluster start 6 --confirm", clusterIDInputs()),
		clusterGeneratedOperation("cluster.stop", []string{"cluster", "stop"}, "Stop a cluster", "POST", "/api/v1/clusters/:id/stop", RiskR1,
			"停止集群会终止 SeaTunnel 进程，并中断该集群正在运行的任务。", "stx cluster stop 6 --confirm", clusterIDInputs()),
		clusterGeneratedOperation("cluster.restart", []string{"cluster", "restart"}, "Restart a cluster", "POST", "/api/v1/clusters/:id/restart", RiskR2,
			"重启集群会短暂中断服务，并重新创建所有节点的 SeaTunnel 进程。", "stx cluster restart 6 --confirm", clusterIDInputs()),
		clusterGeneratedOperation("cluster.node.start", []string{"cluster", "node", "start"}, "Start a cluster node", "POST", "/api/v1/clusters/:id/nodes/:nodeId/start", RiskR1,
			"启动节点会创建 SeaTunnel 进程并占用目标主机资源和端口。", "stx cluster node start 6 1 --confirm", clusterNodeIDInputs()),
		clusterGeneratedOperation("cluster.node.stop", []string{"cluster", "node", "stop"}, "Stop a cluster node", "POST", "/api/v1/clusters/:id/nodes/:nodeId/stop", RiskR1,
			"停止节点会终止目标 SeaTunnel 进程，可能影响集群任务。", "stx cluster node stop 6 1 --confirm", clusterNodeIDInputs()),
		clusterGeneratedOperation("cluster.node.restart", []string{"cluster", "node", "restart"}, "Restart a cluster node", "POST", "/api/v1/clusters/:id/nodes/:nodeId/restart", RiskR2,
			"重启节点会短暂终止并重新创建目标 SeaTunnel 进程。", "stx cluster node restart 6 1 --confirm", clusterNodeIDInputs()),
		clusterGeneratedOperation("cluster.java-proxy.status", []string{"cluster", "java-proxy", "status"}, "Get STX Java Proxy status", "GET", "/api/v1/clusters/:id/stx-java-proxy/status", RiskR0,
			"状态查询会向集群主节点 Agent 发起一次轻量检查。", "stx cluster java-proxy status 6", clusterIDInputs()),
		clusterGeneratedOperation("cluster.java-proxy.logs", []string{"cluster", "java-proxy", "logs"}, "Get STX Java Proxy logs", "GET", "/api/v1/clusters/:id/stx-java-proxy/logs", RiskR0,
			"读取较多日志会消耗 Agent、网络和 STX 服务资源。", "stx cluster java-proxy logs 6 --lines 200", append(clusterIDInputs(), InputSpec{Name: "lines", Location: InputQuery, Required: false, Description: "Number of log lines"})),
		clusterGeneratedOperation("cluster.java-proxy.start", []string{"cluster", "java-proxy", "start"}, "Start STX Java Proxy", "POST", "/api/v1/clusters/:id/stx-java-proxy/start", RiskR1,
			"启动 Java Proxy 会在集群主节点创建 JVM 进程并占用内存和端口。", "stx cluster java-proxy start 6 --confirm", clusterIDInputs()),
		clusterGeneratedOperation("cluster.java-proxy.stop", []string{"cluster", "java-proxy", "stop"}, "Stop STX Java Proxy", "POST", "/api/v1/clusters/:id/stx-java-proxy/stop", RiskR1,
			"停止 Java Proxy 会让依赖该代理的配置检查和运行时查询暂时不可用。", "stx cluster java-proxy stop 6 --confirm", clusterIDInputs()),
		clusterGeneratedOperation("cluster.java-proxy.restart", []string{"cluster", "java-proxy", "restart"}, "Restart STX Java Proxy", "POST", "/api/v1/clusters/:id/stx-java-proxy/restart", RiskR2,
			"重启 Java Proxy 会造成短暂不可用，并重新创建 JVM 进程。", "stx cluster java-proxy restart 6 --confirm", clusterIDInputs()),
	}
}

func clusterBodyOperation(id string, commandPath []string, summary, method, route string, risk RiskLevel, impact string, inputs []InputSpec, example string) OperationSpec {
	return clusterOperation(id, commandPath, summary, method, route, risk, impact, inputs, example, false)
}

func clusterGeneratedOperation(id string, commandPath []string, summary, method, route string, risk RiskLevel, impact string, example string, inputs []InputSpec) OperationSpec {
	if risk != RiskR0 {
		inputs = append(inputs,
			InputSpec{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable retry key"},
			InputSpec{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation"},
		)
	}
	return clusterOperation(id, commandPath, summary, method, route, risk, impact, inputs, example, true)
}

func clusterOperation(id string, commandPath []string, summary, method, route string, risk RiskLevel, impact string, inputs []InputSpec, example string, generated bool) OperationSpec {
	var impactSpec *ImpactSpec
	if strings.TrimSpace(impact) != "" {
		impactSpec = &ImpactSpec{Level: risk, Message: impact}
	}
	return OperationSpec{
		ID: id, CommandPath: commandPath, Summary: summary, GeneratedCLI: generated, Method: method, Route: route,
		Mode: ModeNormal, AuthRequired: true, Risk: risk, Revision: 1, UsesAgent: strings.Contains(route, "/start") || strings.Contains(route, "/stop") || strings.Contains(route, "/restart") || strings.Contains(route, "/logs") || strings.Contains(route, "/precheck") || strings.Contains(route, "/stx-java-proxy/"),
		SupportsPick: true, Impact: impactSpec, Input: inputs, Example: example,
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{},"result_meta":{"complete":true}}`, id),
	}
}

func clusterIDInputs() []InputSpec {
	return []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}}
}

func clusterNodeIDInputs() []InputSpec {
	return []InputSpec{
		{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
		{Name: "nodeId", Location: InputPath, Required: true, Description: "Node ID"},
	}
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
