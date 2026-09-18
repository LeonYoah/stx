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
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/LeonYoah/stx/internal/operation"
)

const ReportVersion = 1

const (
	StatusOperation     = "operation"
	StatusException     = "exception"
	StatusHistoricalGap = "historical_gap"
)

// Report 是路由、Swagger 和操作登记状态的可重复生成报告。
// Report is a reproducible report of routes, Swagger coverage, and operation registration status.
type Report struct {
	Version      int           `json:"version"`
	BaselineDate string        `json:"baseline_date,omitempty"`
	Summary      ReportSummary `json:"summary"`
	Routes       []RouteStatus `json:"routes"`
	SwaggerOnly  []Route       `json:"swagger_only,omitempty"`
}

// ReportSummary 汇总报告中的关键计数。
// ReportSummary summarizes the key counts in a report.
type ReportSummary struct {
	RouteCount            int `json:"route_count"`
	SwaggerPathCount      int `json:"swagger_path_count"`
	SwaggerOperationCount int `json:"swagger_operation_count"`
	SwaggerDocumented     int `json:"swagger_documented_routes"`
	RegisteredOperations  int `json:"registered_operations"`
	RouteExceptions       int `json:"route_exceptions"`
	HistoricalGaps        int `json:"historical_gaps"`
}

// RouteStatus 描述一条源码路由当前的登记和 Swagger 状态。
// RouteStatus describes the current registry and Swagger status of one source route.
type RouteStatus struct {
	Method      string                  `json:"method"`
	Path        string                  `json:"path"`
	Source      string                  `json:"source"`
	Status      string                  `json:"status"`
	OperationID string                  `json:"operation_id,omitempty"`
	Mode        operation.OperationMode `json:"mode,omitempty"`
	Swagger     bool                    `json:"swagger"`
}

// BuildReport 构建当前仓库的操作契约报告。
// BuildReport builds the current repository operation-contract report.
func BuildReport(repositoryRoot string, specs []operation.OperationSpec, exceptions []operation.RouteException) (Report, error) {
	routes, err := CollectRoutes(repositoryRoot)
	if err != nil {
		return Report{}, err
	}
	swaggerOperations, swaggerPathCount, err := loadSwaggerOperations(repositoryRoot)
	if err != nil {
		return Report{}, err
	}

	operationsByRoute := make(map[string]operation.OperationSpec, len(specs))
	for _, spec := range specs {
		operationsByRoute[routeKey(spec.Method, spec.Route)] = spec
	}
	exceptionsByRoute := make(map[string]operation.RouteException, len(exceptions))
	for _, exception := range exceptions {
		exceptionsByRoute[routeKey(exception.Method, exception.Route)] = exception
	}

	report := Report{Version: ReportVersion}
	report.Summary.RouteCount = len(routes)
	report.Summary.SwaggerPathCount = swaggerPathCount
	report.Summary.SwaggerOperationCount = len(swaggerOperations)
	report.Routes = make([]RouteStatus, 0, len(routes))

	seenSourceRoutes := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		key := routeKey(route.Method, route.Path)
		seenSourceRoutes[key] = struct{}{}
		status := RouteStatus{
			Method:  route.Method,
			Path:    route.Path,
			Source:  route.Source,
			Status:  StatusHistoricalGap,
			Swagger: swaggerContains(swaggerOperations, route),
		}
		if status.Swagger {
			report.Summary.SwaggerDocumented++
		}
		if spec, exists := operationsByRoute[key]; exists {
			status.Status = StatusOperation
			status.OperationID = spec.ID
			status.Mode = spec.Mode
			report.Summary.RegisteredOperations++
		} else if exception, exists := exceptionsByRoute[key]; exists {
			status.Status = StatusException
			status.Mode = exception.Mode
			report.Summary.RouteExceptions++
		} else {
			report.Summary.HistoricalGaps++
		}
		report.Routes = append(report.Routes, status)
	}

	for key, swaggerRoute := range swaggerOperations {
		if _, exists := seenSourceRoutes[key]; !exists {
			report.SwaggerOnly = append(report.SwaggerOnly, swaggerRoute)
		}
	}
	sort.Slice(report.SwaggerOnly, func(i, j int) bool {
		if report.SwaggerOnly[i].Path != report.SwaggerOnly[j].Path {
			return report.SwaggerOnly[i].Path < report.SwaggerOnly[j].Path
		}
		return report.SwaggerOnly[i].Method < report.SwaggerOnly[j].Method
	})
	return report, nil
}

// LoadReport 从 JSON 文件读取基线报告。
// LoadReport loads a baseline report from a JSON file.
func LoadReport(filename string) (Report, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		return Report{}, fmt.Errorf("decode report %s: %w", filename, err)
	}
	return report, nil
}

// MarshalReport 生成带稳定缩进和结尾换行的 JSON。
// MarshalReport produces JSON with stable indentation and a trailing newline.
func MarshalReport(report Report) ([]byte, error) {
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

// FindNewHistoricalGaps 返回当前报告中新出现且没有登记或例外说明的路由。
// FindNewHistoricalGaps returns routes that newly appear without an operation or exception entry.
func FindNewHistoricalGaps(previous Report, current Report) []string {
	previousGaps := make(map[string]struct{})
	for _, route := range previous.Routes {
		if route.Status == StatusHistoricalGap {
			previousGaps[route.Method+" "+route.Path] = struct{}{}
		}
	}
	var newGaps []string
	for _, route := range current.Routes {
		if route.Status != StatusHistoricalGap {
			continue
		}
		key := route.Method + " " + route.Path
		if _, exists := previousGaps[key]; !exists {
			newGaps = append(newGaps, key)
		}
	}
	sort.Strings(newGaps)
	return newGaps
}

func loadSwaggerOperations(repositoryRoot string) (map[string]Route, int, error) {
	content, err := os.ReadFile(repositoryRoot + "/docs/swagger.json")
	if err != nil {
		return nil, 0, err
	}
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, 0, fmt.Errorf("decode docs/swagger.json: %w", err)
	}
	operations := make(map[string]Route)
	for routePath, pathItem := range document.Paths {
		for method := range pathItem {
			normalizedMethod := strings.ToUpper(method)
			if normalizedMethod != "GET" && normalizedMethod != "POST" && normalizedMethod != "PUT" && normalizedMethod != "PATCH" && normalizedMethod != "DELETE" && normalizedMethod != "HEAD" && normalizedMethod != "OPTIONS" {
				continue
			}
			route := Route{Method: normalizedMethod, Path: routePath, Source: "docs/swagger.json"}
			operations[routeKey(route.Method, route.Path)] = route
		}
	}
	return operations, len(document.Paths), nil
}

func swaggerContains(swaggerOperations map[string]Route, route Route) bool {
	if route.Method == "ANY" {
		prefix := normalizeRouteParameters(route.Path)
		for _, swaggerRoute := range swaggerOperations {
			if normalizeRouteParameters(swaggerRoute.Path) == prefix {
				return true
			}
		}
		return false
	}
	_, exists := swaggerOperations[routeKey(route.Method, route.Path)]
	return exists
}

func routeKey(method string, routePath string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + normalizeRouteParameters(routePath)
}

func normalizeRouteParameters(routePath string) string {
	parts := strings.Split(routePath, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[index] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}
