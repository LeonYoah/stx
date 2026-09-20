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
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryIsValid(t *testing.T) {
	require.NoError(t, Validate(Registry(), RouteExceptions()))
}

func TestValidateRejectsDuplicateOperationID(t *testing.T) {
	specs := Registry()
	duplicate := specs[0]
	duplicate.CommandPath = []string{"health", "duplicate"}
	duplicate.Route = "/api/v1/health/duplicate"

	err := Validate(append(specs, duplicate), nil)
	require.ErrorContains(t, err, "duplicate operation_id")
}

func TestValidateRejectsCommandConflict(t *testing.T) {
	specs := Registry()
	conflict := specs[0]
	conflict.ID = "health.duplicate"
	conflict.Route = "/api/v1/health/duplicate"

	err := Validate(append(specs, conflict), nil)
	require.ErrorContains(t, err, "command path")
}

func TestValidateRejectsInvalidRevision(t *testing.T) {
	specs := Registry()
	specs[0].Revision = 0

	err := Validate(specs, nil)
	require.ErrorContains(t, err, "invalid revision")
}

func TestValidateRejectsGeneratedCLIWithoutSummary(t *testing.T) {
	specs := Registry()
	for index := range specs {
		if specs[index].GeneratedCLI {
			specs[index].Summary = ""
			err := Validate(specs, RouteExceptions())
			require.ErrorContains(t, err, "generated CLI command has no summary")
			return
		}
	}
	t.Fatal("登记表缺少生成式 CLI 操作 / registry has no generated CLI operation")
}

func TestValidateRejectsGeneratedDeleteCLI(t *testing.T) {
	specs := Registry()
	for index := range specs {
		if specs[index].GeneratedCLI {
			specs[index].Method = http.MethodDelete
			err := Validate(specs, RouteExceptions())
			require.Error(t, err)
			require.Contains(t, err.Error(), "DELETE operations cannot be exposed through CLI")
			return
		}
	}
	t.Fatal("测试登记中缺少可生成 CLI 的操作 / generated CLI operation missing from test registry")
}

func TestValidateRejectsRepeatedNonQueryInput(t *testing.T) {
	specs := Registry()
	specs[0].Input = []InputSpec{{
		Name:        "id",
		Location:    InputPath,
		Required:    true,
		Repeated:    true,
		Description: "Resource ID",
	}}

	err := Validate(specs, RouteExceptions())
	require.ErrorContains(t, err, "can only be repeated at query location")
}

func TestRegistryContainsGeneratedCLIReadBatches(t *testing.T) {
	require.Len(t, Registry(), 87)

	expected := map[string]struct{}{
		"auth.user-info.get": {}, "admin.user.list": {}, "admin.user.get": {},
		"dashboard.overview.get": {}, "dashboard.stats.get": {},
		"dashboard.cluster.list": {}, "dashboard.host.list": {}, "dashboard.activity.list": {},
		"host.list": {}, "host.get": {}, "host.agent.install-command.get": {}, "cluster.list": {}, "cluster.get": {},
		"host.discovery.process.list": {},
		"cluster.node.list":           {}, "cluster.status.get": {}, "config.cluster.list": {},
		"cluster.node.logs": {},
		"cluster.start":     {}, "cluster.stop": {}, "cluster.restart": {},
		"cluster.node.start": {}, "cluster.node.stop": {}, "cluster.node.restart": {},
		"cluster.java-proxy.status": {}, "cluster.java-proxy.logs": {},
		"cluster.java-proxy.start": {}, "cluster.java-proxy.stop": {}, "cluster.java-proxy.restart": {},
		"config.get": {}, "config.version.list": {},
		"host.install.status.get": {}, "cluster.plugin.list": {}, "cluster.plugin.progress.get": {},
		"package.list": {}, "package.get": {}, "package.version.refresh": {}, "package.download.list": {}, "package.download.get": {},
		"plugin.list": {}, "plugin.get": {}, "plugin.local.list": {}, "plugin.download.list": {},
		"plugin.download.status.get": {}, "plugin.dependency.list": {}, "plugin.official-dependency.list": {},
		"stupgrade.plan.get": {}, "stupgrade.task.list": {}, "stupgrade.task.get": {},
		"stupgrade.task.steps": {}, "stupgrade.task.logs": {},
	}
	actual := make(map[string]struct{})
	for _, spec := range Registry() {
		if spec.GeneratedCLI {
			actual[spec.ID] = struct{}{}
		}
	}
	require.Equal(t, expected, actual)
}

func TestRegistryContainsClusterWriteOperations(t *testing.T) {
	byID := make(map[string]OperationSpec)
	for _, spec := range Registry() {
		byID[spec.ID] = spec
	}

	for _, operationID := range []string{
		"cluster.create", "cluster.update", "cluster.delete", "cluster.node.add", "cluster.node.add-batch",
		"cluster.node.update", "cluster.node.remove", "cluster.node.precheck", "cluster.start", "cluster.stop",
		"cluster.restart", "cluster.node.start", "cluster.node.stop", "cluster.node.restart",
		"cluster.java-proxy.start", "cluster.java-proxy.stop", "cluster.java-proxy.restart",
	} {
		spec, exists := byID[operationID]
		require.True(t, exists, operationID)
		require.NotEqual(t, RiskR0, spec.Risk, operationID)
		require.NotNil(t, spec.Impact, operationID)
	}

	require.False(t, byID["cluster.create"].GeneratedCLI)
	require.False(t, byID["cluster.node.precheck"].GeneratedCLI)
	require.False(t, byID["cluster.delete"].GeneratedCLI)
	require.Empty(t, byID["cluster.delete"].CommandPath)
	require.False(t, byID["cluster.node.remove"].GeneratedCLI)
	require.Empty(t, byID["cluster.node.remove"].CommandPath)
	require.True(t, byID["cluster.restart"].GeneratedCLI)
}

func TestRegistryNeverGeneratesDeleteOperations(t *testing.T) {
	for _, spec := range Registry() {
		if spec.Method == "DELETE" {
			require.False(t, spec.GeneratedCLI, spec.ID)
			require.Empty(t, spec.CommandPath, spec.ID)
		}
	}
}

func TestRegistryContainsHostInstallOperations(t *testing.T) {
	byID := make(map[string]OperationSpec)
	for _, spec := range Registry() {
		byID[spec.ID] = spec
	}

	precheck := byID["host.precheck"]
	require.Equal(t, RiskR0, precheck.Risk)
	require.True(t, precheck.UsesAgent)
	require.False(t, precheck.GeneratedCLI)

	for _, operationID := range []string{"host.install.start", "host.install.retry", "host.install.cancel"} {
		spec, exists := byID[operationID]
		require.True(t, exists, operationID)
		require.Equal(t, RiskR1, spec.Risk, operationID)
		require.True(t, spec.UsesAgent, operationID)
		require.NotNil(t, spec.Impact, operationID)
		require.False(t, spec.GeneratedCLI, operationID)
	}
}

func TestRegistryPluginDownloadStatusSupportsRepeatedProfiles(t *testing.T) {
	byID := make(map[string]OperationSpec)
	for _, spec := range Registry() {
		byID[spec.ID] = spec
	}

	spec := byID["plugin.download.status.get"]
	require.Equal(t, "/api/v1/plugins/:name/download/status", spec.Route)
	require.Equal(t, []InputSpec{
		{Name: "name", Location: InputPath, Required: true, Description: "Plugin name"},
		{Name: "version", Location: InputQuery, Required: true, Description: "SeaTunnel version"},
		{Name: "profile_keys", Location: InputQuery, Repeated: true, Description: "Dependency profile key; may be specified more than once"},
	}, spec.Input)
}

func TestRegistryContainsDiscoveryOperationAndLegacyExceptions(t *testing.T) {
	byID := make(map[string]OperationSpec)
	for _, spec := range Registry() {
		byID[spec.ID] = spec
	}
	discovery := byID["host.discovery.process.list"]
	require.Equal(t, "POST", discovery.Method)
	require.Equal(t, RiskR0, discovery.Risk)
	require.True(t, discovery.UsesAgent)
	require.NotNil(t, discovery.Impact)

	exceptions := make(map[string]RouteException)
	for _, exception := range RouteExceptions() {
		exceptions[exception.Method+" "+exception.Route] = exception
	}
	require.Len(t, RouteExceptions(), 21)
	require.Contains(t, exceptions, "POST /api/v1/hosts/:id/discover")
	require.Contains(t, exceptions, "POST /api/v1/hosts/:id/discover/confirm")
}

func TestRegistryAdminUserOperations(t *testing.T) {
	byID := make(map[string]OperationSpec)
	for _, spec := range Registry() {
		byID[spec.ID] = spec
	}

	list := byID["admin.user.list"]
	require.True(t, list.AdminOnly)
	require.Equal(t, []InputSpec{
		{Name: "current", Location: InputQuery, Required: true, Description: "Page number starting from 1"},
		{Name: "size", Location: InputQuery, Required: true, Description: "Page size from 1 to 100"},
		{Name: "username", Location: InputQuery, Required: false, Description: "Username prefix filter"},
		{Name: "is_active", Location: InputQuery, Required: false, Description: "Active state filter"},
		{Name: "is_admin", Location: InputQuery, Required: false, Description: "Administrator state filter"},
	}, list.Input)

	get := byID["admin.user.get"]
	require.True(t, get.AdminOnly)
	require.Equal(t, "/api/v1/admin/users/:id", get.Route)

	for _, operationID := range []string{"admin.user.create", "admin.user.update", "admin.user.delete"} {
		spec := byID[operationID]
		require.True(t, spec.AdminOnly)
		require.False(t, spec.GeneratedCLI)
		require.NotNil(t, spec.Impact)
	}
	require.Equal(t, RiskR1, byID["admin.user.create"].Risk)
	require.Equal(t, RiskR1, byID["admin.user.update"].Risk)
	require.Equal(t, RiskR2, byID["admin.user.delete"].Risk)
}

func TestValidateRejectsInvalidHelpExample(t *testing.T) {
	specs := Registry()
	specs[0].Example = "stx cluster list"

	err := Validate(specs, nil)
	require.ErrorContains(t, err, "example must start")
}

func TestValidateRejectsInvalidOutputExample(t *testing.T) {
	specs := Registry()
	specs[0].OutputExample = `{"data": {}}`

	err := Validate(specs, nil)
	require.ErrorContains(t, err, "missing the required result envelope")
}

func TestValidateRejectsNormalRouteException(t *testing.T) {
	exceptions := []RouteException{{
		Method: "GET",
		Route:  "/api/v1/example",
		Mode:   ModeNormal,
		Reason: "example",
	}}

	err := Validate(nil, exceptions)
	require.ErrorContains(t, err, "invalid exception mode")
}
