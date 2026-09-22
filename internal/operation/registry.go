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
const RegistryRevision = 19

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
		Example: "",
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
		ID:           "host.precheck",
		CommandPath:  []string{"host", "precheck"},
		Summary:      "Run installation precheck on a host",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/hosts/:id/precheck",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR0,
		Revision:     1,
		UsesAgent:    true,
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Host ID"},
			{Name: "install_dir", Location: InputBody, Description: "SeaTunnel installation directory"},
		},
		Impact:  &ImpactSpec{Level: RiskR0, Message: "Runs read-only installation checks on the target host.", Performance: "Reads system resources, disk space, Java information, and port availability."},
		Example: "stx host precheck 10 --install-dir /tmp/seatunnel-2.3.12 --port 15812",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.precheck",
  "request_id": "req_example",
  "data": {"passed":true,"checks":[]},
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:           "host.install.start",
		CommandPath:  []string{"host", "install", "start"},
		Summary:      "Install SeaTunnel on a host",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/hosts/:id/install",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		UsesAgent:    true,
		Async:        true,
		Impact:       &ImpactSpec{Level: RiskR1, Message: "Writes a SeaTunnel installation and configuration files on the target host.", Performance: "Uses server and Agent network bandwidth, CPU, and disk I/O while transferring and extracting the package."},
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Host ID"},
			{Name: "version", Location: InputBody, Required: true, Description: "SeaTunnel version"},
		},
		Example: "stx host install start 10 --version 2.3.12 --install-dir /tmp/seatunnel-2.3.12 --deployment-mode hybrid --node-role master/worker --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.install.start",
  "request_id": "req_example",
  "data": {"id":"install_example","host_id":"10","status":"running","progress":0},
  "result_meta": {"complete": true,"next_command":"stx host install status get 10"}
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
		ID:           "host.install.retry",
		CommandPath:  []string{"host", "install", "retry"},
		Summary:      "Retry a failed installation step",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/hosts/:id/install/retry",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		UsesAgent:    true,
		Impact:       &ImpactSpec{Level: RiskR1, Message: "Retries a failed installation step and may rewrite files on the target host."},
		Input: []InputSpec{
			{Name: "id", Location: InputPath, Required: true, Description: "Host ID"},
			{Name: "step", Location: InputBody, Required: true, Description: "Failed installation step"},
		},
		Example: "stx host install retry 10 --step extract --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.install.retry",
  "request_id": "req_example",
  "data": {"host_id":"10","status":"running"},
  "result_meta": {"complete": true,"next_command":"stx host install status get 10"}
}`,
	},
	{
		ID:           "host.install.cancel",
		CommandPath:  []string{"host", "install", "cancel"},
		Summary:      "Request installation cancellation",
		GeneratedCLI: false,
		Method:       "POST",
		Route:        "/api/v1/hosts/:id/install/cancel",
		Mode:         ModeNormal,
		AuthRequired: true,
		Risk:         RiskR1,
		Revision:     1,
		UsesAgent:    true,
		Impact:       &ImpactSpec{Level: RiskR1, Message: "Requests cancellation of the active installation; the current server implementation may not stop an Agent command that has already started."},
		Input:        []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Host ID"}},
		Example:      "stx host install cancel 10 --confirm",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "host.install.cancel",
  "request_id": "req_example",
  "data": {"host_id":"10","status":"failed","message":"Installation cancelled / 安装已取消"},
  "result_meta": {"complete": true,"next_command":"stx host install status get 10"}
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
		Example:      "",
		OutputExample: `{
  "api_version": "v1",
  "operation_id": "package.delete",
  "request_id": "req_example",
  "data": null,
  "result_meta": {"complete": true}
}`,
	},
	{
		ID:            "package.source.upload",
		CommandPath:   []string{"package", "source", "upload"},
		Summary:       "Upload or replace a SeaTunnel source archive",
		GeneratedCLI:  false,
		Method:        "POST",
		Route:         "/api/v1/packages/:version/source/upload",
		Mode:          ModeNormal,
		AuthRequired:  true,
		Risk:          RiskR1,
		Revision:      1,
		Impact:        &ImpactSpec{Level: RiskR1, Message: "Writes or replaces the source archive associated with an existing runtime package."},
		Example:       "stx package source upload 2.3.13 ./apache-seatunnel-2.3.13-src.tar.gz --confirm",
		OutputExample: `{"api_version":"v1","operation_id":"package.source.upload","request_id":"req_example","data":{"version":"2.3.13","has_source":true},"result_meta":{"complete":true}}`,
	},
	{
		ID:            "package.source.fetch",
		CommandPath:   []string{"package", "source", "fetch"},
		Summary:       "Fetch a SeaTunnel source archive through STX",
		GeneratedCLI:  false,
		Method:        "POST",
		Route:         "/api/v1/packages/:version/source/fetch",
		Mode:          ModeNormal,
		AuthRequired:  true,
		Risk:          RiskR1,
		Revision:      1,
		Impact:        &ImpactSpec{Level: RiskR1, Message: "Downloads source into STX local storage.", Performance: "Uses STX server network bandwidth and disk I/O."},
		Example:       "stx package source fetch 2.3.13 --mirror apache --confirm",
		OutputExample: `{"api_version":"v1","operation_id":"package.source.fetch","request_id":"req_example","data":{"version":"2.3.13","has_source":true},"result_meta":{"complete":true}}`,
	},
	{
		ID:            "package.source.download",
		CommandPath:   []string{"package", "source", "download"},
		Summary:       "Download a SeaTunnel source archive through STX",
		GeneratedCLI:  false,
		Method:        "GET",
		Route:         "/api/v1/packages/:version/source/download",
		Mode:          ModeDownload,
		AuthRequired:  true,
		Risk:          RiskR0,
		Revision:      1,
		Example:       "stx package source download 2.3.13 --file /tmp/apache-seatunnel-2.3.13-src.tar.gz",
		OutputExample: `{"api_version":"v1","operation_id":"package.source.download","request_id":"req_example","data":{"version":"2.3.13","file":"/tmp/apache-seatunnel-2.3.13-src.tar.gz"},"result_meta":{"complete":true}}`,
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
}, additionalOperationSpecs()...)

// additionalOperationSpecs 返回分模块维护的补充操作登记。
// additionalOperationSpecs returns supplemental operation registrations maintained by module.
func additionalOperationSpecs() []OperationSpec {
	specs := clusterAdditionalOperationSpecs()
	specs = append(specs, stUpgradeOperationSpecs()...)
	specs = append(specs, additionalReadOperationSpecs()...)
	specs = append(specs, auditOperationSpecs()...)
	specs = append(specs, configWriteOperationSpecs()...)
	specs = append(specs, monitorOperationSpecs()...)
	specs = append(specs, monitoringReadOperationSpecs()...)
	specs = append(specs, syncOperationSpecs()...)
	return append(specs, diagnosticsReadOperationSpecs()...)
}

// diagnosticsReadOperationSpecs 登记诊断中心普通 JSON 查询命令。
// diagnosticsReadOperationSpecs registers diagnostics queries that return regular JSON responses.
func diagnosticsReadOperationSpecs() []OperationSpec {
	id := []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Resource ID"}}
	lang := InputSpec{Name: "lang", Location: InputQuery, Description: "Response language: zh or en"}
	page := []InputSpec{{Name: "page", Location: InputQuery, Description: "Page number starting from 1"}, {Name: "page_size", Location: InputQuery, Description: "Page size"}}
	timeRange := []InputSpec{{Name: "start_time", Location: InputQuery, Description: "Start time in RFC3339 format"}, {Name: "end_time", Location: InputQuery, Description: "End time in RFC3339 format"}}
	errorCommon := []InputSpec{
		{Name: "cluster_id", Location: InputQuery, Description: "Cluster ID"},
		{Name: "node_id", Location: InputQuery, Description: "Cluster node ID"},
		{Name: "host_id", Location: InputQuery, Description: "Host ID"},
		{Name: "role", Location: InputQuery, Description: "Node role"},
		{Name: "job_id", Location: InputQuery, Description: "SeaTunnel job ID"},
		{Name: "keyword", Location: InputQuery, Description: "Message keyword"},
		{Name: "exception_class", Location: InputQuery, Description: "Java exception class"},
	}
	errorCommon = append(errorCommon, timeRange...)
	errorCommon = append(errorCommon, page...)
	return []OperationSpec{
		diagnosticsReadOperation("diagnostics.bootstrap.get", []string{"diagnostics", "bootstrap", "get"}, "Get diagnostics workspace bootstrap", "/api/v1/diagnostics/bootstrap",
			[]InputSpec{{Name: "source", Location: InputQuery, Description: "Workspace entry source"}, {Name: "alert_id", Location: InputQuery, Description: "Related alert ID"}, {Name: "cluster_id", Location: InputQuery, Description: "Cluster ID"}, lang},
			"stx diagnostics bootstrap get --cluster_id 6"),
		diagnosticsReadOperation("diagnostics.resource.list", []string{"diagnostics", "resource", "list"}, "List registered diagnostic resources", "/api/v1/diagnostics/resources", nil, "stx diagnostics resource list"),
		diagnosticsReadOperation("diagnostics.resource.get", []string{"diagnostics", "resource", "get"}, "Get a registered diagnostic resource", "/api/v1/diagnostics/resources/:code", []InputSpec{{Name: "code", Location: InputPath, Required: true, Description: "Diagnostic resource code"}}, "stx diagnostics resource get thread_dump"),
		{
			ID:           "diagnostics.resource.run",
			CommandPath:  []string{"diagnostics", "resource", "run"},
			Summary:      "Run one diagnostic resource",
			GeneratedCLI: false,
			Method:       "POST",
			Route:        "/api/v1/diagnostics/resources/:code/run",
			Mode:         ModeNormal,
			AuthRequired: true,
			Risk:         RiskR1,
			Revision:     1,
			UsesAgent:    true,
			Async:        true,
			Impact: &ImpactSpec{
				Level:       RiskR1,
				Message:     "只执行指定诊断资源；线程快照可能短暂增加负载，JVM Dump 仅管理员可执行。",
				Performance: "读取日志和配置会增加磁盘与网络读取；JVM Dump 可能暂停目标 JVM 并产生大文件。",
			},
			Input: []InputSpec{
				{Name: "code", Location: InputPath, Required: true, Description: "Diagnostic resource code"},
				{Name: "request", Location: InputBody, Required: true, Description: "Single-resource execution request"},
				{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable key for retrying the same request"},
				{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation of the operation impact"},
			},
			Example: "stx diagnostics resource run thread_dump --cluster-id 6 --node-id 6 --confirm",
			OutputExample: `{
  "api_version": "v1",
  "operation_id": "diagnostics.resource.run",
  "request_id": "req_example",
  "data": {"id": 43, "status": "running", "execution_id": "11111111-1111-1111-1111-111111111111"},
  "result_meta": {"complete": true, "next_command": "stx execution wait 11111111-1111-1111-1111-111111111111"}
}`,
		},
		diagnosticsReadOperation("diagnostics.inspection.list", []string{"diagnostics", "inspection", "list"}, "List inspection reports", "/api/v1/diagnostics/inspections",
			append([]InputSpec{{Name: "cluster_id", Location: InputQuery, Description: "Cluster ID"}, {Name: "status", Location: InputQuery, Description: "Inspection status"}, {Name: "trigger_source", Location: InputQuery, Description: "Inspection trigger source"}, {Name: "severity", Location: InputQuery, Description: "Finding severity"}}, append(timeRange, append(page, lang)...)...),
			"stx diagnostics inspection list --cluster_id 6 --page_size 20"),
		diagnosticsReadOperation("diagnostics.inspection.get", []string{"diagnostics", "inspection", "get"}, "Get inspection report detail", "/api/v1/diagnostics/inspections/:id", append(append([]InputSpec{}, id...), lang), "stx diagnostics inspection get 1"),
		diagnosticsWriteOperation("diagnostics.inspection.run", []string{"diagnostics", "inspection", "run"}, "Run an inspection immediately", "POST", "/api/v1/diagnostics/inspections", RiskR1,
			"立即巡检会读取集群状态、进程事件、告警和近期错误，并保存巡检报告。", false, false,
			[]InputSpec{{Name: "request", Location: InputBody, Required: true, Description: "Inspection request"}},
			"stx diagnostics inspection run --cluster-id 6 --confirm"),
		diagnosticsReadOperation("diagnostics.task.list", []string{"diagnostics", "task", "list"}, "List diagnostic tasks", "/api/v1/diagnostics/tasks",
			append([]InputSpec{{Name: "cluster_id", Location: InputQuery, Description: "Cluster ID"}, {Name: "trigger_source", Location: InputQuery, Description: "Diagnostic trigger source"}, {Name: "status", Location: InputQuery, Description: "Diagnostic task status"}}, append(page, lang)...),
			"stx diagnostics task list --cluster_id 6 --page_size 20"),
		{
			ID:           "diagnostics.task.create",
			CommandPath:  []string{"diagnostics", "task", "create"},
			Summary:      "Create a diagnostic task",
			GeneratedCLI: false,
			Method:       "POST",
			Route:        "/api/v1/diagnostics/tasks",
			Mode:         ModeNormal,
			AuthRequired: true,
			Risk:         RiskR0,
			Revision:     1,
			UsesAgent:    true,
			Async:        true,
			Impact: &ImpactSpec{
				Level:       RiskR0,
				Message:     "默认只创建诊断任务，不会立即在集群上采集；使用 --auto-start 才会启动任务。",
				Performance: "启动后会按所选资源读取集群信息、日志和进程数据；JVM Dump 可能产生较大的文件并影响目标进程。",
			},
			Input: []InputSpec{
				{Name: "request", Location: InputBody, Required: true, Description: "Diagnostic task JSON request"},
				{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable key for retrying the same request"},
				{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation of the operation impact"},
			},
			Example: "stx diagnostics task create --cluster-id 6 --confirm",
			OutputExample: `{
  "api_version": "v1",
  "operation_id": "diagnostics.task.create",
  "request_id": "req_example",
  "data": {"id": 42, "status": "ready", "execution_id": "11111111-1111-1111-1111-111111111111"},
  "result_meta": {"complete": true, "next_command": "stx diagnostics task get 42"}
}`,
		},
		diagnosticsWriteOperation("diagnostics.task.start", []string{"diagnostics", "task", "start"}, "Start an existing diagnostic task", "POST", "/api/v1/diagnostics/tasks/:id/start", RiskR1,
			"启动诊断任务会执行任务中已选择的资源采集步骤，部分步骤可能增加节点负载。", true, true,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Diagnostic task ID"}},
			"stx diagnostics task start 42 --confirm"),
		diagnosticsReadOperation("diagnostics.task.get", []string{"diagnostics", "task", "get"}, "Get diagnostic task", "/api/v1/diagnostics/tasks/:id", append(append([]InputSpec{}, id...), lang), "stx diagnostics task get 1"),
		diagnosticsReadOperation("diagnostics.task.steps", []string{"diagnostics", "task", "steps"}, "List diagnostic task steps", "/api/v1/diagnostics/tasks/:id/steps", append(append([]InputSpec{}, id...), lang), "stx diagnostics task steps 1"),
		diagnosticsReadOperation("diagnostics.task.artifacts", []string{"diagnostics", "task", "artifacts"}, "List diagnostic task artifacts", "/api/v1/diagnostics/tasks/:id/artifacts", append([]InputSpec{}, id...), "stx diagnostics task artifacts 1"),
		diagnosticsReadOperation("diagnostics.task.logs", []string{"diagnostics", "task", "logs"}, "List diagnostic task logs", "/api/v1/diagnostics/tasks/:id/logs",
			append(append([]InputSpec{}, id...), []InputSpec{{Name: "step_code", Location: InputQuery, Description: "Diagnostic step code"}, {Name: "node_execution_id", Location: InputQuery, Description: "Node execution ID"}, {Name: "level", Location: InputQuery, Description: "Log level"}, {Name: "page", Location: InputQuery, Description: "Page number starting from 1"}, {Name: "page_size", Location: InputQuery, Description: "Page size"}, lang}...),
			"stx diagnostics task logs 1 --page_size 50"),
		diagnosticsDownloadOperation("diagnostics.task.bundle.download", []string{"diagnostics", "task", "bundle"}, "Download a diagnostic task bundle", "/api/v1/diagnostics/tasks/:id/bundle", append([]InputSpec{}, id...), "stx diagnostics task bundle 1 --file /tmp/diagnostics-1.zip"),
		diagnosticsDownloadOperation("diagnostics.task.html.download", []string{"diagnostics", "task", "html"}, "Download a diagnostic task HTML report", "/api/v1/diagnostics/tasks/:id/html", append([]InputSpec{}, id...), "stx diagnostics task html 1 --file /tmp/diagnostics-1.html"),
		diagnosticsDownloadOperation("diagnostics.task.file.download", []string{"diagnostics", "task", "file"}, "Download one diagnostic task file", "/api/v1/diagnostics/tasks/:id/files/*path", []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Diagnostic task ID"}, {Name: "path", Location: InputPath, Required: true, Description: "Relative artifact path"}}, "stx diagnostics task file 1 manifest.json --file /tmp/manifest.json"),
		diagnosticsReadOperation("diagnostics.error.group.list", []string{"diagnostics", "error", "group", "list"}, "List SeaTunnel error groups", "/api/v1/diagnostics/errors/groups", errorCommon, "stx diagnostics error group list --cluster_id 6 --page_size 20"),
		diagnosticsReadOperation("diagnostics.error.event.list", []string{"diagnostics", "error", "event", "list"}, "List SeaTunnel error events", "/api/v1/diagnostics/errors/events", append([]InputSpec{{Name: "group_id", Location: InputQuery, Description: "Error group ID"}}, errorCommon...), "stx diagnostics error event list --cluster_id 6 --page_size 20"),
		diagnosticsReadOperation("diagnostics.error.group.get", []string{"diagnostics", "error", "group", "get"}, "Get SeaTunnel error group detail", "/api/v1/diagnostics/errors/groups/:id",
			append(append([]InputSpec{}, id...), append([]InputSpec{{Name: "event_limit", Location: InputQuery, Description: "Maximum recent events to include"}}, errorCommon...)...), "stx diagnostics error group get 1 --event_limit 20"),
		diagnosticsReadOperation("diagnostics.auto-policy.template.list", []string{"diagnostics", "auto-policy", "template", "list"}, "List built-in diagnostic policy templates", "/api/v1/diagnostics/auto-policies/templates", []InputSpec{lang}, "stx diagnostics auto-policy template list"),
		diagnosticsReadOperation("diagnostics.auto-policy.list", []string{"diagnostics", "auto-policy", "list"}, "List diagnostic auto policies", "/api/v1/diagnostics/auto-policies", append([]InputSpec{{Name: "cluster_id", Location: InputQuery, Description: "Cluster ID"}}, page...), "stx diagnostics auto-policy list --cluster_id 6"),
		diagnosticsReadOperation("diagnostics.auto-policy.get", []string{"diagnostics", "auto-policy", "get"}, "Get diagnostic auto policy", "/api/v1/diagnostics/auto-policies/:id", id, "stx diagnostics auto-policy get 1"),
		diagnosticsWriteOperation("diagnostics.auto-policy.create", []string{"diagnostics", "auto-policy", "create"}, "Create a diagnostic auto policy", "POST", "/api/v1/diagnostics/auto-policies", RiskR1,
			"创建自动巡检策略后，启用的条件可能自动发起巡检和诊断任务。", false, false,
			[]InputSpec{{Name: "request", Location: InputBody, Required: true, Description: "Auto-policy request"}},
			"stx diagnostics auto-policy create --cluster-id 6 --name 'daily inspection' --condition SCHEDULED --confirm"),
		diagnosticsWriteOperation("diagnostics.auto-policy.update", []string{"diagnostics", "auto-policy", "update"}, "Update a diagnostic auto policy", "PUT", "/api/v1/diagnostics/auto-policies/:id", RiskR1,
			"修改自动巡检策略会改变后续自动巡检和诊断任务的触发方式。", false, false,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Auto-policy ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Auto-policy update request"}},
			"stx diagnostics auto-policy update 1 --enabled=false --confirm"),
		diagnosticsReadOperation("diagnostics.troubleshooting-memory.list", []string{"diagnostics", "troubleshooting-memory", "list"}, "List troubleshooting memories", "/api/v1/diagnostics/troubleshooting-memories", append([]InputSpec{{Name: "target_type", Location: InputQuery, Description: "Target type"}, {Name: "fingerprint", Location: InputQuery, Description: "Fingerprint"}, {Name: "keyword", Location: InputQuery, Description: "Keyword"}, {Name: "cluster_id", Location: InputQuery, Description: "Cluster ID"}}, page...), "stx diagnostics troubleshooting-memory list"),
		diagnosticsReadOperation("diagnostics.troubleshooting-memory.get", []string{"diagnostics", "troubleshooting-memory", "get"}, "Get troubleshooting memory detail", "/api/v1/diagnostics/troubleshooting-memories/:id", id, "stx diagnostics troubleshooting-memory get 1"),
		{
			ID:           "diagnostics.troubleshooting-memory.create",
			CommandPath:  []string{"diagnostics", "troubleshooting-memory", "create"},
			Summary:      "Create a troubleshooting memory entry",
			GeneratedCLI: false,
			Method:       "POST",
			Route:        "/api/v1/diagnostics/troubleshooting-memories",
			Mode:         ModeNormal,
			AuthRequired: true,
			Risk:         RiskR1,
			Revision:     1,
			SupportsPick: true,
			Impact: &ImpactSpec{
				Level:   RiskR1,
				Message: "新增一条排障经验记录，后续诊断和人工排查可以读取该内容。",
			},
			Input: []InputSpec{
				{Name: "request", Location: InputBody, Required: true, Description: "Troubleshooting memory JSON payload"},
				{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable key for retrying the same request"},
				{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation of the operation impact"},
			},
			Example:       "stx diagnostics troubleshooting-memory create --request-file memory.json --confirm",
			OutputExample: `{"api_version":"v1","operation_id":"diagnostics.troubleshooting-memory.create","request_id":"req_example","data":{"id":1},"result_meta":{"complete":true}}`,
		},
		{
			ID:           "diagnostics.troubleshooting-memory.update",
			CommandPath:  []string{"diagnostics", "troubleshooting-memory", "update"},
			Summary:      "Update a troubleshooting memory entry",
			GeneratedCLI: false,
			Method:       "PUT",
			Route:        "/api/v1/diagnostics/troubleshooting-memories/:id",
			Mode:         ModeNormal,
			AuthRequired: true,
			Risk:         RiskR1,
			Revision:     1,
			SupportsPick: true,
			Impact: &ImpactSpec{
				Level:   RiskR1,
				Message: "修改排障经验记录会改变后续诊断和人工排查读取到的内容。",
			},
			Input: append(append(append([]InputSpec{}, id...),
				InputSpec{Name: "request", Location: InputBody, Required: true, Description: "Troubleshooting memory JSON payload"}),
				InputSpec{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable key for retrying the same request"},
				InputSpec{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation of the operation impact"}),
			Example:       "stx diagnostics troubleshooting-memory update 1 --request-file memory.json --confirm",
			OutputExample: `{"api_version":"v1","operation_id":"diagnostics.troubleshooting-memory.update","request_id":"req_example","data":{"id":1},"result_meta":{"complete":true}}`,
		},
		{
			ID:           "diagnostics.troubleshooting-memory.delete",
			Summary:      "Delete a troubleshooting memory entry",
			GeneratedCLI: false,
			Method:       "DELETE",
			Route:        "/api/v1/diagnostics/troubleshooting-memories/:id",
			Mode:         ModeNormal,
			AuthRequired: true,
			Risk:         RiskR1,
			Revision:     1,
			Impact: &ImpactSpec{
				Level:   RiskR1,
				Message: "删除后该排障沉淀经验不可直接恢复。",
			},
			Input:         id,
			OutputExample: `{"api_version":"v1","operation_id":"diagnostics.troubleshooting-memory.delete","request_id":"req_example","data":{"deleted":true},"result_meta":{"complete":true}}`,
		},
	}
}

// diagnosticsWriteOperation 创建由专用 CLI 处理的诊断写操作登记。
// diagnosticsWriteOperation creates a diagnostics write registration handled by dedicated CLI code.
func diagnosticsWriteOperation(operationID string, commandPath []string, summary, method, route string, risk RiskLevel, impact string, usesAgent, async bool, inputs []InputSpec, example string) OperationSpec {
	inputs = append(inputs,
		InputSpec{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable key for retrying the same request"},
		InputSpec{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation of the operation impact"},
	)
	return OperationSpec{
		ID: operationID, CommandPath: commandPath, Summary: summary, GeneratedCLI: false, Method: method, Route: route,
		Mode: ModeNormal, AuthRequired: true, Risk: risk, Revision: 1, UsesAgent: usesAgent, Async: async,
		SupportsPick: true, Impact: &ImpactSpec{Level: risk, Message: impact}, Input: inputs, Example: example,
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{},"result_meta":{"complete":true}}`, operationID),
	}
}

// diagnosticsReadOperation 创建可由普通 GET 构建器执行的诊断查询。
// diagnosticsReadOperation creates a diagnostics query handled by the regular GET builder.
func diagnosticsReadOperation(operationID string, commandPath []string, summary, route string, inputs []InputSpec, example string) OperationSpec {
	return OperationSpec{
		ID: operationID, CommandPath: commandPath, Summary: summary, GeneratedCLI: true, Method: "GET", Route: route,
		Mode: ModeNormal, AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true, Input: inputs, Example: example,
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{},"result_meta":{"complete":true}}`, operationID),
	}
}

// diagnosticsDownloadOperation 登记诊断资源下载操作，由专用 CLI 处理文件落盘。
// diagnosticsDownloadOperation registers diagnostics downloads handled by the dedicated file-writing CLI.
func diagnosticsDownloadOperation(operationID string, commandPath []string, summary, route string, inputs []InputSpec, example string) OperationSpec {
	return OperationSpec{
		ID: operationID, CommandPath: commandPath, Summary: summary, GeneratedCLI: false, Method: "GET", Route: route,
		Mode: ModeDownload, AuthRequired: true, Risk: RiskR0, Revision: 1, Input: inputs, Example: example,
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{"file":"/tmp/diagnostics-output","size":12,"sha256":"..."},"result_meta":{"complete":true}}`, operationID),
	}
}

// monitorOperationSpecs 登记集群进程监控配置和事件命令。
// monitorOperationSpecs registers cluster process-monitor configuration and event commands.
func monitorOperationSpecs() []OperationSpec {
	clusterID := []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}}
	eventInputs := append(append([]InputSpec{}, clusterID...),
		InputSpec{Name: "event_type", Location: InputQuery, Description: "Process event type"},
		InputSpec{Name: "node_id", Location: InputQuery, Description: "Cluster node ID"},
		InputSpec{Name: "start_time", Location: InputQuery, Description: "Start time in RFC3339 format"},
		InputSpec{Name: "end_time", Location: InputQuery, Description: "End time in RFC3339 format"},
		InputSpec{Name: "page", Location: InputQuery, Description: "Page number starting from 1"},
		InputSpec{Name: "page_size", Location: InputQuery, Description: "Page size"},
	)
	statsInputs := append(append([]InputSpec{}, clusterID...), InputSpec{Name: "since", Location: InputQuery, Description: "Start time in RFC3339 format"})
	return []OperationSpec{
		clusterGeneratedOperation("monitor.config.get", []string{"monitor", "config", "get"}, "Get cluster monitor configuration", "GET", "/api/v1/clusters/:id/monitor-config", RiskR0,
			"读取监控配置不会修改集群。", "stx monitor config get 6", clusterID),
		clusterBodyOperation("monitor.config.update", []string{"monitor", "config", "update"}, "Update cluster monitor configuration", "PUT", "/api/v1/clusters/:id/monitor-config", RiskR1,
			"修改监控和自动重启参数会改变 Agent 对集群进程的检查与恢复行为。",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Monitor configuration fields to update"}},
			"stx monitor config update 6 --monitor-interval 5 --confirm"),
		clusterGeneratedOperation("monitor.event.list", []string{"monitor", "event", "list"}, "List cluster process events", "GET", "/api/v1/clusters/:id/events", RiskR0,
			"读取大量进程事件会增加 STX 数据库查询和输出开销。", "stx monitor event list 6 --page_size 20", eventInputs),
		clusterGeneratedOperation("monitor.event.stats", []string{"monitor", "event", "stats"}, "Get cluster process event statistics", "GET", "/api/v1/clusters/:id/events/stats", RiskR0,
			"统计较长时间范围的进程事件会增加 STX 数据库查询开销。", "stx monitor event stats 6 --since 2026-09-20T00:00:00Z", statsInputs),
	}
}

// monitoringReadOperationSpecs 登记监控中心首批只读 CLI 操作。
// monitoringReadOperationSpecs registers the first read-only monitoring CLI operations.
func monitoringReadOperationSpecs() []OperationSpec {
	clusterID := []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}}
	policyID := []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Alert policy ID"}}
	policyExecutionInputs := append(append([]InputSpec{}, policyID...), monitoringDeliveryFilterInputs(false)...)
	alertInstanceInputs := append([]InputSpec{
		{Name: "source_type", Location: InputQuery, Description: "Alert source: local_process_event or remote_alertmanager"},
		{Name: "cluster_id", Location: InputQuery, Description: "Cluster ID or all"},
		{Name: "severity", Location: InputQuery, Description: "Alert severity: warning or critical"},
		{Name: "status", Location: InputQuery, Description: "Display status: firing, resolved, or closed"},
		{Name: "lifecycle_status", Location: InputQuery, Description: "Lifecycle status: firing or resolved"},
		{Name: "handling_status", Location: InputQuery, Description: "Handling status: pending, acknowledged, silenced, or closed"},
	}, monitoringTimeAndPaginationInputs()...)
	alertInputs := append([]InputSpec{
		{Name: "cluster_id", Location: InputQuery, Description: "Cluster ID"},
		{Name: "status", Location: InputQuery, Description: "Alert status: firing, acknowledged, or silenced"},
	}, monitoringTimeAndPaginationInputs()...)
	remoteAlertInputs := append([]InputSpec{
		{Name: "cluster_id", Location: InputQuery, Description: "Cluster ID or all"},
		{Name: "status", Location: InputQuery, Description: "Remote alert status filter"},
	}, monitoringTimeAndPaginationInputs()...)
	return []OperationSpec{
		monitoringReadOperation("monitoring.overview.get", []string{"monitoring", "overview", "get"}, "Get monitoring overview", "/api/v1/monitoring/overview", nil),
		monitoringReadOperation("monitoring.cluster.overview.get", []string{"monitoring", "cluster", "overview", "get"}, "Get cluster monitoring overview", "/api/v1/monitoring/clusters/:id/overview", clusterID),
		monitoringReadOperation("monitoring.alert-policy.list", []string{"monitoring", "alert-policy", "list"}, "List alert policies", "/api/v1/monitoring/alert-policies", nil),
		monitoringReadOperation("monitoring.alert-policy.execution.list", []string{"monitoring", "alert-policy", "execution", "list"}, "List alert policy executions", "/api/v1/monitoring/alert-policies/:id/executions", policyExecutionInputs),
		monitoringReadOperation("monitoring.alert-instance.list", []string{"monitoring", "alert-instance", "list"}, "List alert instances", "/api/v1/monitoring/alert-instances", alertInstanceInputs),
		monitoringReadOperation("monitoring.alert.list", []string{"monitoring", "alert", "list"}, "List local alerts", "/api/v1/monitoring/alerts", alertInputs),
		monitoringReadOperation("monitoring.remote-alert.list", []string{"monitoring", "remote-alert", "list"}, "List remote alerts", "/api/v1/monitoring/remote-alerts", remoteAlertInputs),
		monitoringReadOperation("monitoring.cluster.rule.list", []string{"monitoring", "cluster", "rule", "list"}, "List cluster alert rules", "/api/v1/monitoring/clusters/:id/rules", clusterID),
		monitoringReadOperation("monitoring.integration.status", []string{"monitoring", "integration", "status"}, "Get monitoring integration status", "/api/v1/monitoring/integration/status", nil),
		monitoringReadOperation("monitoring.alert-policy.bootstrap.get", []string{"monitoring", "alert-policy", "bootstrap", "get"}, "Get alert policy center bootstrap", "/api/v1/monitoring/alert-policies/bootstrap", nil),
		monitoringReadOperation("monitoring.notifiable-user.list", []string{"monitoring", "notifiable-user", "list"}, "List notifiable users", "/api/v1/monitoring/notifiable-users", nil),
		monitoringReadOperation("monitoring.platform-health.get", []string{"monitoring", "platform-health", "get"}, "Get platform health", "/api/v1/monitoring/platform-health", nil),
		monitoringReadOperation("monitoring.notification-channel.list", []string{"monitoring", "notification-channel", "list"}, "List notification channels", "/api/v1/monitoring/notification-channels", nil),
		monitoringReadOperation("monitoring.notification-delivery.list", []string{"monitoring", "notification-delivery", "list"}, "List notification deliveries", "/api/v1/monitoring/notification-deliveries", monitoringDeliveryFilterInputs(true)),
		monitoringReadOperation("monitoring.notification-route.list", []string{"monitoring", "notification-route", "list"}, "List notification routes", "/api/v1/monitoring/notification-routes", nil),
	}
}

// monitoringTimeAndPaginationInputs 返回监控列表共用的时间范围和分页参数。
// monitoringTimeAndPaginationInputs returns the shared time range and pagination inputs for monitoring lists.
func monitoringTimeAndPaginationInputs() []InputSpec {
	return []InputSpec{
		{Name: "start_time", Location: InputQuery, Description: "Start time in RFC3339 format"},
		{Name: "end_time", Location: InputQuery, Description: "End time in RFC3339 format"},
		{Name: "page", Location: InputQuery, Description: "Page number starting from 1"},
		{Name: "page_size", Location: InputQuery, Description: "Page size"},
	}
}

// monitoringDeliveryFilterInputs 返回通知投递和策略执行记录共用的筛选参数。
// monitoringDeliveryFilterInputs returns filters shared by notification deliveries and policy executions.
func monitoringDeliveryFilterInputs(includeResourceIDs bool) []InputSpec {
	inputs := make([]InputSpec, 0, 9)
	if includeResourceIDs {
		inputs = append(inputs,
			InputSpec{Name: "policy_id", Location: InputQuery, Description: "Alert policy ID"},
			InputSpec{Name: "channel_id", Location: InputQuery, Description: "Notification channel ID"},
		)
	}
	inputs = append(inputs,
		InputSpec{Name: "status", Location: InputQuery, Description: "Delivery status: pending, sending, sent, failed, retrying, or canceled"},
		InputSpec{Name: "event_type", Location: InputQuery, Description: "Delivery event: firing, resolved, or test"},
	)
	if includeResourceIDs {
		inputs = append(inputs, InputSpec{Name: "cluster_id", Location: InputQuery, Description: "Cluster ID or all"})
	}
	return append(inputs, monitoringTimeAndPaginationInputs()...)
}

// monitoringReadOperation 创建可由普通 GET 构建器生成的监控查询。
// monitoringReadOperation creates a monitoring query generated by the normal GET builder.
func monitoringReadOperation(id string, commandPath []string, summary, route string, inputs []InputSpec) OperationSpec {
	return OperationSpec{
		ID: id, CommandPath: commandPath, Summary: summary, GeneratedCLI: true, Method: "GET", Route: route,
		Mode: ModeNormal, AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true, Input: inputs,
		Example:       "stx " + strings.Join(commandPath, " ") + monitoringExampleArgs(inputs),
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{},"result_meta":{"complete":true}}`, id),
	}
}

// monitoringExampleArgs 根据路径参数和首个查询参数生成可执行示例。
// monitoringExampleArgs builds an executable example from path inputs and the first query input.
func monitoringExampleArgs(inputs []InputSpec) string {
	args := ""
	for _, input := range inputs {
		if input.Location == InputPath {
			args += " 1"
		}
	}
	for _, input := range inputs {
		if input.Location == InputQuery {
			return args + " --" + input.Name + " " + monitoringExampleValue(input.Name)
		}
	}
	return args
}

// monitoringExampleValue 返回监控查询参数的可执行样例值。
// monitoringExampleValue returns an executable sample value for a monitoring query input.
func monitoringExampleValue(name string) string {
	switch name {
	case "source_type":
		return "local_process_event"
	case "status":
		return "sent"
	case "start_time":
		return "2026-09-20T00:00:00Z"
	default:
		return "1"
	}
}

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
		clusterNonCLIWriteOperation("cluster.delete", "Delete a cluster", "DELETE", "/api/v1/clusters/:id", RiskR2,
			"删除集群会移除集群定义和节点记录；集群必须先停止，force_delete 还会请求 Agent 删除节点安装目录。", []InputSpec{
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
		clusterNonCLIWriteOperation("cluster.node.remove", "Remove a cluster node", "DELETE", "/api/v1/clusters/:id/nodes/:nodeId", RiskR2,
			"移除节点会删除节点定义；如果节点仍在运行，应先停止节点。", []InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "nodeId", Location: InputPath, Required: true, Description: "Node ID"},
			}),
		clusterBodyOperation("cluster.node.precheck", []string{"cluster", "node", "precheck"}, "Precheck a cluster node", "POST", "/api/v1/clusters/:id/nodes/precheck", RiskR1,
			"节点预检查会连接目标 Agent，并执行目录、端口和运行环境检查。",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Node precheck request"}},
			"stx cluster node precheck 6 --host-id 10 --role master/worker --confirm"),
		clusterNonCLIWriteOperation("cluster.runtime-storage.apply", "Apply cluster runtime storage settings", "POST", "/api/v1/clusters/:id/runtime-storage/:kind/apply", RiskR1,
			"保存运行时存储设置会先执行真实读写检查，再生成新的集群配置版本；部分修改需要重启节点后生效。",
			[]InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "kind", Location: InputPath, Required: true, Description: "Runtime storage kind: checkpoint or imap"},
				{Name: "request", Location: InputBody, Required: true, Description: "Runtime storage settings; secrets must be read from protected input"},
			}),
		clusterBodyOperation("cluster.log-mode.update", []string{"cluster", "log-mode", "update"}, "Update cluster job log mode", "POST", "/api/v1/clusters/:id/log-mode", RiskR1,
			"切换作业日志模式会生成新的 log4j2 配置版本并同步到集群节点，需要重启集群后生效。",
			[]InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "mode", Location: InputBody, Required: true, Description: "Job log mode: per_job or mixed"},
			}, "stx cluster log-mode update 6 --mode per_job --confirm"),
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
		clusterGeneratedOperation("cluster.java-proxy.config", []string{"cluster", "java-proxy", "config"}, "Update STX Java Proxy config", "POST", "/api/v1/clusters/:id/stx-java-proxy/config", RiskR2,
			"修改 Java Proxy 配置或内存参数并在生效时触发重启。", "stx cluster java-proxy config 6 --confirm", clusterIDInputs()),
	}
}

// stUpgradeOperationSpecs 返回 SeaTunnel 升级模块的 CLI 与能力登记。
// stUpgradeOperationSpecs returns CLI and capability registrations for SeaTunnel upgrades.
func stUpgradeOperationSpecs() []OperationSpec {
	return []OperationSpec{
		{
			ID: "stupgrade.precheck", CommandPath: []string{"upgrade", "precheck"}, Summary: "Precheck a SeaTunnel upgrade",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/st-upgrade/precheck", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input: []InputSpec{
				{Name: "cluster_id", Location: InputBody, Required: true, Description: "Cluster ID"},
				{Name: "target_version", Location: InputBody, Required: true, Description: "Target SeaTunnel version"},
				{Name: "target_install_dir", Location: InputBody, Required: false, Description: "Target installation directory"},
			},
			Example:       "stx upgrade precheck 8 --target-version 2.3.13 --target-install-dir /tmp/seatunnel-2.3.13-new",
			OutputExample: `{"api_version":"v1","operation_id":"stupgrade.precheck","request_id":"req_example","data":{"ready":true},"result_meta":{"complete":true}}`,
		},
		{
			ID: "stupgrade.plan.create", CommandPath: []string{"upgrade", "plan", "create"}, Summary: "Create a SeaTunnel upgrade plan",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/st-upgrade/plan", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input: []InputSpec{
				{Name: "cluster_id", Location: InputBody, Required: true, Description: "Cluster ID"},
				{Name: "target_version", Location: InputBody, Required: true, Description: "Target SeaTunnel version"},
				{Name: "config_merge_plan", Location: InputBody, Required: true, Description: "Resolved configuration merge plan"},
			},
			Example:       "stx upgrade plan create 8 --target-version 2.3.13 --target-install-dir /tmp/seatunnel-2.3.13-new --config-merge-plan-file merge-plan.json",
			OutputExample: `{"api_version":"v1","operation_id":"stupgrade.plan.create","request_id":"req_example","data":{"plan":{"id":1,"status":"ready"}},"result_meta":{"complete":true,"next_command":"stx upgrade plan execute 1 --confirm"}}`,
		},
		{
			ID: "stupgrade.plan.get", CommandPath: []string{"upgrade", "plan", "get"}, Summary: "Get a SeaTunnel upgrade plan",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/st-upgrade/plans/:id", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input:         []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Upgrade plan ID"}},
			Example:       "stx upgrade plan get 1",
			OutputExample: `{"api_version":"v1","operation_id":"stupgrade.plan.get","request_id":"req_example","data":{"id":1,"status":"ready"},"result_meta":{"complete":true}}`,
		},
		{
			ID: "stupgrade.plan.execute", CommandPath: []string{"upgrade", "plan", "execute"}, Summary: "Execute a SeaTunnel upgrade plan",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/st-upgrade/execute", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR2, Revision: 1, UsesAgent: true, Async: true, SupportsPick: true,
			Impact: &ImpactSpec{
				Level:       RiskR2,
				Message:     "升级会停止 SeaTunnel 集群、切换运行目录，并可能在失败时执行回滚。",
				Performance: "升级期间集群会暂时不可用，并占用磁盘、网络和 Agent 执行资源。",
			},
			Input: []InputSpec{
				{Name: "plan_id", Location: InputBody, Required: true, Description: "Upgrade plan ID"},
				{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable retry key"},
				{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation"},
			},
			Example:       "stx upgrade plan execute 1 --confirm",
			OutputExample: `{"api_version":"v1","operation_id":"stupgrade.plan.execute","request_id":"req_example","data":{"id":1,"status":"pending"},"result_meta":{"complete":true,"next_command":"stx upgrade task wait 1"}}`,
		},
		{
			ID: "stupgrade.task.list", CommandPath: []string{"upgrade", "task", "list"}, Summary: "List SeaTunnel upgrade tasks",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/st-upgrade/tasks", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input: []InputSpec{
				{Name: "cluster_id", Location: InputQuery, Required: false, Description: "Cluster ID"},
				{Name: "page", Location: InputQuery, Required: false, Description: "Page number"},
				{Name: "page_size", Location: InputQuery, Required: false, Description: "Page size"},
			},
			Example:       "stx upgrade task list --cluster_id 8",
			OutputExample: `{"api_version":"v1","operation_id":"stupgrade.task.list","request_id":"req_example","data":{"items":[],"total":0},"result_meta":{"complete":true}}`,
		},
		{
			ID: "stupgrade.task.get", CommandPath: []string{"upgrade", "task", "get"}, Summary: "Get a SeaTunnel upgrade task",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/st-upgrade/tasks/:id", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input:         []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Upgrade task ID"}},
			Example:       "stx upgrade task get 1",
			OutputExample: `{"api_version":"v1","operation_id":"stupgrade.task.get","request_id":"req_example","data":{"id":1,"status":"running"},"result_meta":{"complete":true,"next_command":"stx upgrade task wait 1"}}`,
		},
		{
			ID: "stupgrade.task.steps", CommandPath: []string{"upgrade", "task", "steps"}, Summary: "Get SeaTunnel upgrade task steps",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/st-upgrade/tasks/:id/steps", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input:         []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Upgrade task ID"}},
			Example:       "stx upgrade task steps 1",
			OutputExample: `{"api_version":"v1","operation_id":"stupgrade.task.steps","request_id":"req_example","data":{"task_id":1,"steps":[],"node_executions":[]},"result_meta":{"complete":true}}`,
		},
		{
			ID: "stupgrade.task.logs", CommandPath: []string{"upgrade", "task", "logs"}, Summary: "List SeaTunnel upgrade task logs",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/st-upgrade/tasks/:id/logs", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Impact: &ImpactSpec{Level: RiskR0, Message: "读取大量升级日志会增加 STX 服务和数据库查询开销。"},
			Input: []InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Upgrade task ID"},
				{Name: "step_code", Location: InputQuery, Required: false, Description: "Upgrade step code"},
				{Name: "level", Location: InputQuery, Required: false, Description: "Log level"},
				{Name: "node_execution_id", Location: InputQuery, Required: false, Description: "Node execution ID"},
				{Name: "page", Location: InputQuery, Required: false, Description: "Page number"},
				{Name: "page_size", Location: InputQuery, Required: false, Description: "Page size"},
			},
			Example:       "stx upgrade task logs 1 --page_size 100",
			OutputExample: `{"api_version":"v1","operation_id":"stupgrade.task.logs","request_id":"req_example","data":{"task_id":1,"items":[],"total":0},"result_meta":{"complete":true}}`,
		},
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

// clusterNonCLIWriteOperation 登记服务端保留、但禁止通过 CLI 执行的集群写操作。
// clusterNonCLIWriteOperation registers a server-side cluster write operation that the CLI must not expose.
func clusterNonCLIWriteOperation(id, summary, method, route string, risk RiskLevel, impact string, inputs []InputSpec) OperationSpec {
	return clusterOperation(id, nil, summary, method, route, risk, impact, inputs, "", false)
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
	{Method: "GET", Route: "/api/v1/agent/install.sh", Mode: ModeDownload, Reason: "Agent installation script used by the host setup flow"},
	{Method: "GET", Route: "/api/v1/agent/uninstall.sh", Mode: ModeDownload, Reason: "Agent uninstallation script used by the host setup flow"},
	{Method: "GET", Route: "/api/v1/agent/ca.crt", Mode: ModeDownload, Reason: "Agent CA certificate used by the host setup flow"},
	{Method: "GET", Route: "/api/v1/agent/download", Mode: ModeDownload, Reason: "Agent binary used by the host setup flow"},
	{Method: "GET", Route: "/api/v1/agent/assets/stx-java-proxy.jar", Mode: ModeDownload, Reason: "Java Proxy jar used by the managed runtime setup flow"},
	{Method: "GET", Route: "/api/v1/agent/assets/stx-java-proxy.sh", Mode: ModeDownload, Reason: "Java Proxy launcher used by the managed runtime setup flow"},
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
