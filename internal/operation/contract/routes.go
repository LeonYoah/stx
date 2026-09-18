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

// Package contract 提供操作登记表与源码路由、Swagger 的开发期检查。
// Package contract provides development-time checks between the operation registry, source routes, and Swagger.
package contract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Route 描述从 Gin 路由注册代码中读取到的一条路由。
// Route describes one route extracted from Gin registration source code.
type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Source string `json:"source"`
}

var routeMethods = map[string]string{
	"GET":     "GET",
	"POST":    "POST",
	"PUT":     "PUT",
	"PATCH":   "PATCH",
	"DELETE":  "DELETE",
	"HEAD":    "HEAD",
	"OPTIONS": "OPTIONS",
	"Any":     "ANY",
}

// CollectRoutes 读取仓库当前的 /api/v1 Gin 路由注册。
// CollectRoutes reads the repository's current /api/v1 Gin route registrations.
func CollectRoutes(repositoryRoot string) ([]Route, error) {
	collector := &routeCollector{
		repositoryRoot: repositoryRoot,
		fileSet:        token.NewFileSet(),
	}
	if err := collector.collectFunction(
		filepath.Join(repositoryRoot, "internal/router/router.go"),
		"Serve",
		map[string]string{},
	); err != nil {
		return nil, err
	}

	sort.Slice(collector.routes, func(i, j int) bool {
		if collector.routes[i].Path != collector.routes[j].Path {
			return collector.routes[i].Path < collector.routes[j].Path
		}
		if collector.routes[i].Method != collector.routes[j].Method {
			return collector.routes[i].Method < collector.routes[j].Method
		}
		return collector.routes[i].Source < collector.routes[j].Source
	})
	return collector.routes, nil
}

type routeCollector struct {
	repositoryRoot string
	fileSet        *token.FileSet
	routes         []Route
	err            error
}

func (c *routeCollector) collectFunction(filename string, functionName string, initialGroups map[string]string) error {
	parsedFile, err := parser.ParseFile(c.fileSet, filename, nil, 0)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filename, err)
	}
	for _, declaration := range parsedFile.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
		}
		environment := cloneEnvironment(initialGroups)
		c.processStatements(filename, function.Body.List, environment)
		if c.err != nil {
			return c.err
		}
		return nil
	}
	return fmt.Errorf("function %s not found in %s", functionName, filename)
}

func (c *routeCollector) processStatements(filename string, statements []ast.Stmt, environment map[string]string) {
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *ast.AssignStmt:
			c.processAssignment(typed, environment)
		case *ast.ExprStmt:
			c.processExpression(filename, typed.X, environment)
		case *ast.BlockStmt:
			c.processStatements(filename, typed.List, cloneEnvironment(environment))
		case *ast.IfStmt:
			branchEnvironment := cloneEnvironment(environment)
			if typed.Init != nil {
				c.processStatements(filename, []ast.Stmt{typed.Init}, branchEnvironment)
			}
			c.processStatements(filename, typed.Body.List, branchEnvironment)
			if typed.Else != nil {
				c.processStatements(filename, []ast.Stmt{typed.Else}, cloneEnvironment(environment))
			}
		case *ast.ForStmt:
			c.processStatements(filename, typed.Body.List, cloneEnvironment(environment))
		case *ast.RangeStmt:
			c.processStatements(filename, typed.Body.List, cloneEnvironment(environment))
		case *ast.SwitchStmt:
			c.processCaseClauses(filename, typed.Body.List, environment)
		case *ast.TypeSwitchStmt:
			c.processCaseClauses(filename, typed.Body.List, environment)
		}
	}
}

func (c *routeCollector) processCaseClauses(filename string, statements []ast.Stmt, environment map[string]string) {
	for _, statement := range statements {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			continue
		}
		c.processStatements(filename, clause.Body, cloneEnvironment(environment))
	}
}

func (c *routeCollector) processAssignment(assignment *ast.AssignStmt, environment map[string]string) {
	for index, leftExpression := range assignment.Lhs {
		if index >= len(assignment.Rhs) {
			break
		}
		identifier, ok := leftExpression.(*ast.Ident)
		if !ok {
			continue
		}
		if groupPath, ok := evaluateGroupPath(assignment.Rhs[index], environment); ok {
			environment[identifier.Name] = groupPath
			continue
		}
		if value, ok := evaluatePathExpression(assignment.Rhs[index], environment); ok {
			environment[identifier.Name] = value
		}
	}
}

func (c *routeCollector) processExpression(filename string, expression ast.Expr, environment map[string]string) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	if method, isRouteMethod := routeMethods[selector.Sel.Name]; isRouteMethod {
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || len(call.Args) == 0 {
			return
		}
		groupPath, hasGroup := environment[receiver.Name]
		routePath, hasPath := evaluatePathExpression(call.Args[0], environment)
		if !hasGroup || !hasPath {
			return
		}
		fullPath := joinRoutePath(groupPath, routePath)
		if fullPath != "/api/v1" && !strings.HasPrefix(fullPath, "/api/v1/") {
			return
		}
		position := c.fileSet.Position(call.Pos())
		relativeFile, err := filepath.Rel(c.repositoryRoot, filename)
		if err != nil {
			relativeFile = filename
		}
		c.routes = append(c.routes, Route{
			Method: method,
			Path:   fullPath,
			Source: filepath.ToSlash(relativeFile) + ":" + strconv.Itoa(position.Line),
		})
		return
	}

	packageIdentifier, ok := selector.X.(*ast.Ident)
	if !ok || packageIdentifier.Name != "appconfig" || selector.Sel.Name != "RegisterRoutes" || len(call.Args) == 0 {
		return
	}
	routerIdentifier, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return
	}
	basePath, ok := environment[routerIdentifier.Name]
	if !ok {
		return
	}
	if err := c.collectFunction(
		filepath.Join(c.repositoryRoot, "internal/apps/config/routers.go"),
		"RegisterRoutes",
		map[string]string{"router": basePath},
	); err != nil && c.err == nil {
		c.err = err
	}
}

func evaluateGroupPath(expression ast.Expr, environment map[string]string) (string, bool) {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Group" {
		return "", false
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	basePath, ok := environment[receiver.Name]
	if !ok {
		// r 是 Gin 根路由，路径从空字符串开始。
		// r is the Gin root router, whose path starts as an empty string.
		if receiver.Name != "r" {
			return "", false
		}
		basePath = ""
	}
	groupPath, ok := evaluatePathExpression(call.Args[0], environment)
	if !ok {
		return "", false
	}
	return joinRoutePath(basePath, groupPath), true
}

func evaluatePathExpression(expression ast.Expr, environment map[string]string) (string, bool) {
	switch typed := expression.(type) {
	case *ast.BasicLit:
		if typed.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(typed.Value)
		return value, err == nil
	case *ast.Ident:
		value, ok := environment[typed.Name]
		return value, ok
	case *ast.SelectorExpr:
		if selectorPath(typed) == "config.Config.App.APIPrefix" {
			return "/api", true
		}
	case *ast.CallExpr:
		if identifier, ok := typed.Fun.(*ast.Ident); ok && identifier.Name == "normalizeAPIV1RoutePath" && len(typed.Args) >= 2 {
			return evaluatePathExpression(typed.Args[1], environment)
		}
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return "", false
		}
		left, leftOK := evaluatePathExpression(typed.X, environment)
		right, rightOK := evaluatePathExpression(typed.Y, environment)
		if leftOK && rightOK {
			return left + right, true
		}
	}
	return "", false
}

func selectorPath(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		prefix := selectorPath(typed.X)
		if prefix == "" {
			return typed.Sel.Name
		}
		return prefix + "." + typed.Sel.Name
	default:
		return ""
	}
}

func joinRoutePath(basePath string, routePath string) string {
	joined := path.Join("/", basePath, routePath)
	if strings.HasSuffix(routePath, "/") && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	return joined
}

func cloneEnvironment(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
