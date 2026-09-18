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

package contract

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LeonYoah/stx/internal/operation"
	"github.com/stretchr/testify/require"
)

func TestRouteBaselineMatchesRepository(t *testing.T) {
	repositoryRoot := repositoryRoot(t)
	baseline, err := LoadReport(filepath.Join(repositoryRoot, "internal/operation/testdata/route_baseline.json"))
	require.NoError(t, err)

	current, err := BuildReport(repositoryRoot, operation.Registry(), operation.RouteExceptions())
	require.NoError(t, err)

	baselineByRoute := indexRouteStatuses(baseline.Routes)
	currentByRoute := indexRouteStatuses(current.Routes)
	for key, currentRoute := range currentByRoute {
		baselineRoute, exists := baselineByRoute[key]
		if !exists {
			if currentRoute.Status == StatusHistoricalGap {
				t.Fatalf("新增路由没有操作登记或例外说明 / new route has no operation or exception entry: %s", key)
			}
			t.Fatalf("新增路由已经登记，但基线尚未更新 / new route is registered but the baseline is stale: %s", key)
		}
		baselineRoute.Source = ""
		currentRoute.Source = ""
		require.Equal(t, baselineRoute, currentRoute, "路由契约发生变化 / route contract changed: %s", key)
	}
	for key := range baselineByRoute {
		_, exists := currentByRoute[key]
		require.True(t, exists, "基线仍包含已删除路由 / baseline still contains a removed route: %s", key)
	}

	require.Equal(t, baseline.Summary, current.Summary)
	require.Equal(t, baseline.SwaggerOnly, current.SwaggerOnly)
	require.Equal(t, len(operation.Registry()), current.Summary.RegisteredOperations)
	require.Equal(t, len(operation.RouteExceptions()), current.Summary.RouteExceptions)
}

func TestFindNewHistoricalGaps(t *testing.T) {
	previous := Report{Routes: []RouteStatus{{Method: "GET", Path: "/api/v1/known", Status: StatusHistoricalGap}}}
	current := Report{Routes: []RouteStatus{
		{Method: "GET", Path: "/api/v1/known", Status: StatusHistoricalGap},
		{Method: "POST", Path: "/api/v1/new", Status: StatusHistoricalGap},
		{Method: "GET", Path: "/api/v1/registered", Status: StatusOperation},
	}}

	require.Equal(t, []string{"POST /api/v1/new"}, FindNewHistoricalGaps(previous, current))
}

func indexRouteStatuses(routes []RouteStatus) map[string]RouteStatus {
	result := make(map[string]RouteStatus, len(routes))
	for _, route := range routes {
		result[route.Method+" "+route.Path] = route
	}
	return result
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../../.."))
}
