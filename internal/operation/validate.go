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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var (
	operationIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$`)
	commandPartPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Validate 检查操作登记表和特殊路由例外是否满足公共契约。
// Validate checks whether the operation registry and special-route exceptions satisfy the shared contract.
func Validate(specs []OperationSpec, exceptions []RouteException) error {
	var validationErrors []error
	ids := make(map[string]struct{}, len(specs))
	commands := make(map[string]string, len(specs))
	routes := make(map[string]string, len(specs)+len(exceptions))

	for index, spec := range specs {
		prefix := fmt.Sprintf("operation[%d]", index)
		if !operationIDPattern.MatchString(spec.ID) {
			validationErrors = append(validationErrors, fmt.Errorf("%s has invalid operation_id %q", prefix, spec.ID))
		}
		if _, exists := ids[spec.ID]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("duplicate operation_id %q", spec.ID))
		}
		ids[spec.ID] = struct{}{}

		commandKey, err := validateCommandPath(spec.CommandPath)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("%s: %w", prefix, err))
		} else if existingID, exists := commands[commandKey]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("command path %q is shared by %q and %q", commandKey, existingID, spec.ID))
		} else {
			commands[commandKey] = spec.ID
		}

		routeKey, err := validateRoute(spec.Method, spec.Route)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("%s: %w", prefix, err))
		} else if existingID, exists := routes[routeKey]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("route %q is shared by %q and %q", routeKey, existingID, spec.ID))
		} else {
			routes[routeKey] = spec.ID
		}

		if !isOperationMode(spec.Mode) {
			validationErrors = append(validationErrors, fmt.Errorf("%s has invalid mode %q", prefix, spec.Mode))
		}
		if spec.Mode == ModeServerOnly || spec.Mode == ModeProxy {
			validationErrors = append(validationErrors, fmt.Errorf("%s uses mode %q and must be a route exception", prefix, spec.Mode))
		}
		if !isRiskLevel(spec.Risk) {
			validationErrors = append(validationErrors, fmt.Errorf("%s has invalid risk %q", prefix, spec.Risk))
		}
		if spec.Revision < 1 {
			validationErrors = append(validationErrors, fmt.Errorf("%s has invalid revision %d", prefix, spec.Revision))
		}
		if spec.GeneratedCLI && strings.TrimSpace(spec.Summary) == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%s generated CLI command has no summary", prefix))
		}
		if spec.Risk != RiskR0 && spec.Impact == nil {
			validationErrors = append(validationErrors, fmt.Errorf("%s must describe impact for risk %s", prefix, spec.Risk))
		}
		if spec.Impact != nil {
			if !isRiskLevel(spec.Impact.Level) || spec.Impact.Level != spec.Risk {
				validationErrors = append(validationErrors, fmt.Errorf("%s impact level %q does not match risk %q", prefix, spec.Impact.Level, spec.Risk))
			}
			if strings.TrimSpace(spec.Impact.Message) == "" {
				validationErrors = append(validationErrors, fmt.Errorf("%s impact message is empty", prefix))
			}
		}
		if err := validateInputs(spec.Input); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("%s: %w", prefix, err))
		}
		if err := validateExample(spec); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("%s: %w", prefix, err))
		}
	}

	for index, exception := range exceptions {
		prefix := fmt.Sprintf("exception[%d]", index)
		routeKey, err := validateRoute(exception.Method, exception.Route)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("%s: %w", prefix, err))
		} else if existingID, exists := routes[routeKey]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("route %q is shared by %q and %s", routeKey, existingID, prefix))
		} else {
			routes[routeKey] = prefix
		}
		if exception.Mode != ModeServerOnly && exception.Mode != ModeProxy && exception.Mode != ModeWatch && exception.Mode != ModeDownload {
			validationErrors = append(validationErrors, fmt.Errorf("%s has invalid exception mode %q", prefix, exception.Mode))
		}
		if strings.TrimSpace(exception.Reason) == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%s must include a reason", prefix))
		}
	}

	return errors.Join(validationErrors...)
}

func validateCommandPath(parts []string) (string, error) {
	if len(parts) == 0 {
		return "", errors.New("command path is empty")
	}
	for _, part := range parts {
		if !commandPartPattern.MatchString(part) {
			return "", fmt.Errorf("invalid command path part %q", part)
		}
	}
	return strings.Join(parts, " "), nil
}

func validateRoute(method string, route string) (string, error) {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod != "ANY" {
		if _, exists := map[string]struct{}{
			http.MethodGet: {}, http.MethodPost: {}, http.MethodPut: {}, http.MethodPatch: {},
			http.MethodDelete: {}, http.MethodHead: {}, http.MethodOptions: {},
		}[normalizedMethod]; !exists {
			return "", fmt.Errorf("invalid HTTP method %q", method)
		}
	}
	normalizedRoute := strings.TrimSpace(route)
	if !strings.HasPrefix(normalizedRoute, "/api/v1/") && normalizedRoute != "/api/v1" {
		return "", fmt.Errorf("route %q is outside /api/v1", route)
	}
	return normalizedMethod + " " + normalizedRoute, nil
}

func validateInputs(inputs []InputSpec) error {
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return errors.New("input name is empty")
		}
		key := string(input.Location) + ":" + name
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate input %q", key)
		}
		seen[key] = struct{}{}
		if input.Location != InputPath && input.Location != InputQuery && input.Location != InputHeader && input.Location != InputBody && input.Location != InputFile {
			return fmt.Errorf("input %q has invalid location %q", name, input.Location)
		}
		if strings.TrimSpace(input.Description) == "" {
			return fmt.Errorf("input %q has no description", name)
		}
	}
	return nil
}

func validateExample(spec OperationSpec) error {
	exampleFields := strings.Fields(spec.Example)
	expectedPrefix := append([]string{"stx"}, spec.CommandPath...)
	if len(exampleFields) < len(expectedPrefix) {
		return errors.New("example does not contain the full command path")
	}
	for index, expected := range expectedPrefix {
		if exampleFields[index] != expected {
			return fmt.Errorf("example must start with %q", strings.Join(expectedPrefix, " "))
		}
	}

	var output struct {
		APIVersion  string          `json:"api_version"`
		OperationID string          `json:"operation_id"`
		RequestID   string          `json:"request_id"`
		Data        json.RawMessage `json:"data"`
		ResultMeta  struct {
			Complete *bool `json:"complete"`
		} `json:"result_meta"`
	}
	if err := json.Unmarshal([]byte(spec.OutputExample), &output); err != nil {
		return fmt.Errorf("output example is not valid JSON: %w", err)
	}
	if output.APIVersion == "" || output.RequestID == "" || len(output.Data) == 0 || output.ResultMeta.Complete == nil {
		return errors.New("output example is missing the required result envelope")
	}
	if output.OperationID != spec.ID {
		return fmt.Errorf("output example operation_id %q does not match %q", output.OperationID, spec.ID)
	}
	return nil
}

func isOperationMode(mode OperationMode) bool {
	return mode == ModeNormal || mode == ModeWatch || mode == ModeDownload || mode == ModeServerOnly || mode == ModeProxy
}

func isRiskLevel(level RiskLevel) bool {
	return level == RiskR0 || level == RiskR1 || level == RiskR2 || level == RiskR3
}
