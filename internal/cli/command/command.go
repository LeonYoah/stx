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

// Package command 根据操作登记构建普通 STX CLI 命令。
// Package command builds regular STX CLI commands from the operation registry.
package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var routeParameterPattern = regexp.MustCompile(`:([A-Za-z][A-Za-z0-9_-]*)`)

// Client 是登记命令执行时使用的最小远端客户端接口。
// Client is the minimal remote client contract used by generated commands.
type Client interface {
	Capabilities(ctx context.Context) (string, cliClient.CapabilityData, error)
	Request(ctx context.Context, method, path string, requestBody any, result any) (string, error)
	RequestWithHeaders(ctx context.Context, method, path string, requestBody any, headers map[string]string, result any) (string, error)
}

// ClientFactory 根据本地命名空间创建远端客户端。
// ClientFactory creates a remote client for one local namespace.
type ClientFactory func(namespace string) (Client, error)

// queryFlagValue 保存单值或重复查询参数对应的 Cobra flag 值。
// queryFlagValue stores the Cobra flag value for a scalar or repeated query parameter.
type queryFlagValue struct {
	single   *string
	repeated *[]string
}

// writeOptions 保存写操作使用的确认和幂等参数。
// writeOptions stores confirmation and idempotency flags for writes.
type writeOptions struct {
	confirmed      bool
	idempotencyKey string
	confirmationID string
}

// Build 构建登记项对应的 Cobra 顶级命令。
// Build creates top-level Cobra commands for the supplied registry entries.
func Build(specs []operation.OperationSpec, clientFactory ClientFactory) ([]*cobra.Command, error) {
	if clientFactory == nil {
		return nil, fmt.Errorf("client factory is required")
	}

	nodes := make(map[string]*cobra.Command)
	leaves := make(map[string]string)
	rootNames := make([]string, 0)
	for _, item := range specs {
		spec := item
		if err := validateSpec(spec); err != nil {
			return nil, fmt.Errorf("operation %s: %w", spec.ID, err)
		}

		var parent *cobra.Command
		for index, part := range spec.CommandPath {
			path := strings.Join(spec.CommandPath[:index+1], " ")
			current, exists := nodes[path]
			if !exists {
				current = newGroupCommand(part, path)
				nodes[path] = current
				if parent == nil {
					rootNames = append(rootNames, path)
				} else {
					parent.AddCommand(current)
				}
			}
			parent = current
		}

		leafPath := strings.Join(spec.CommandPath, " ")
		if existingID, exists := leaves[leafPath]; exists {
			return nil, fmt.Errorf("command path %q is shared by %q and %q", leafPath, existingID, spec.ID)
		}
		leaves[leafPath] = spec.ID
		configureLeaf(parent, spec, clientFactory)
	}

	sort.Strings(rootNames)
	result := make([]*cobra.Command, 0, len(rootNames))
	for _, name := range rootNames {
		result = append(result, nodes[name])
	}
	return result, nil
}

func newGroupCommand(name, path string) *cobra.Command {
	command := &cobra.Command{
		Use:          name,
		Short:        "STX " + path + " commands",
		SilenceUsage: true,
		Args:         noArgs,
	}
	command.RunE = func(command *cobra.Command, _ []string) error {
		return command.Help()
	}
	return command
}

func configureLeaf(command *cobra.Command, spec operation.OperationSpec, clientFactory ClientFactory) {
	pathInputs := inputsAt(spec, operation.InputPath)
	queryInputs := inputsAt(spec, operation.InputQuery)
	bodyInputs := inputsAt(spec, operation.InputBody)
	queryValues := make(map[string]queryFlagValue, len(queryInputs))
	var namespace string
	var requestFile string
	var write writeOptions

	useParts := []string{command.Name()}
	for _, input := range pathInputs {
		useParts = append(useParts, "<"+input.Name+">")
	}
	command.Use = strings.Join(useParts, " ")
	command.Short = spec.Summary
	command.Long = longDescription(spec)
	command.Example = spec.Example
	command.Args = exactArgs(len(pathInputs))
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	if len(bodyInputs) > 0 {
		command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	}
	if spec.Risk != operation.RiskR0 {
		command.Flags().BoolVar(&write.confirmed, "confirm", false, "Confirm the operation impact")
		command.Flags().StringVar(&write.idempotencyKey, "idempotency-key", "", "Stable key for retrying the same request")
		command.Flags().StringVar(&write.confirmationID, "confirmation-id", "", "One-time confirmation ID returned by STX")
	}
	for _, input := range queryInputs {
		if input.Repeated {
			value := new([]string)
			queryValues[input.Name] = queryFlagValue{repeated: value}
			command.Flags().StringArrayVar(value, input.Name, nil, input.Description)
			continue
		}
		value := new(string)
		queryValues[input.Name] = queryFlagValue{single: value}
		command.Flags().StringVar(value, input.Name, "", input.Description)
	}
	command.RunE = func(command *cobra.Command, args []string) error {
		requestPath, err := buildRequestPath(command, spec, pathInputs, queryInputs, queryValues, args)
		if err != nil {
			return err
		}
		if spec.Risk != operation.RiskR0 && !write.confirmed {
			return clioutput.NewError(clioutput.CodeConflict, "write operation requires --confirm", clioutput.ExitConflict, false)
		}
		requestBody, err := buildRequestBody(command, bodyInputs, requestFile)
		if err != nil {
			return err
		}
		client, err := clientFactory(namespace)
		if err != nil {
			return err
		}
		if err := checkCapability(command.Context(), client, spec); err != nil {
			return err
		}

		headers, err := prepareWriteHeaders(command, spec, &write)
		if err != nil {
			return err
		}

		var data any
		var requestID string
		if len(headers) == 0 {
			requestID, err = client.Request(command.Context(), strings.ToUpper(spec.Method), requestPath, requestBody, &data)
		} else {
			requestID, err = client.RequestWithHeaders(command.Context(), strings.ToUpper(spec.Method), requestPath, requestBody, headers, &data)
		}
		if err != nil {
			return handleConfirmationError(command, spec.ID, err)
		}
		options, err := clioutput.OptionsFromCommand(command)
		if err != nil {
			return err
		}
		result := clioutput.NewResult(spec.ID, requestID, data)
		renderer := clioutput.NewRenderer(command.OutOrStdout(), clioutput.NewEventWriter(command.ErrOrStderr()))
		return renderer.Render(result, options)
	}
}

func validateSpec(spec operation.OperationSpec) error {
	if len(spec.CommandPath) == 0 {
		return fmt.Errorf("command path is empty")
	}
	if strings.TrimSpace(spec.Summary) == "" {
		return fmt.Errorf("command summary is empty")
	}
	if spec.Mode != operation.ModeNormal {
		return fmt.Errorf("generated command only supports normal operations")
	}
	switch strings.ToUpper(strings.TrimSpace(spec.Method)) {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return fmt.Errorf("generated command only supports GET, POST, PUT, PATCH, or DELETE operations")
	}
	bodyInputs := inputsAt(spec, operation.InputBody)
	if len(bodyInputs) > 0 && !methodSupportsJSONBody(spec.Method) {
		return fmt.Errorf("%s operations cannot use a generated JSON request body", strings.ToUpper(strings.TrimSpace(spec.Method)))
	}
	placeholders := routeParameterPattern.FindAllStringSubmatch(spec.Route, -1)
	pathInputs := inputsAt(spec, operation.InputPath)
	if len(placeholders) != len(pathInputs) {
		return fmt.Errorf("route placeholders do not match path inputs")
	}
	for index, placeholder := range placeholders {
		if placeholder[1] != pathInputs[index].Name || !pathInputs[index].Required {
			return fmt.Errorf("route placeholder %q must have a matching required path input", placeholder[1])
		}
	}
	for _, input := range spec.Input {
		if input.Location == operation.InputHeader && isGeneratedSafetyHeader(input.Name) {
			continue
		}
		if input.Location != operation.InputPath && input.Location != operation.InputQuery && input.Location != operation.InputBody {
			return fmt.Errorf("input %q uses unsupported location %q", input.Name, input.Location)
		}
		if input.Repeated && input.Location != operation.InputQuery {
			return fmt.Errorf("input %q can only be repeated at query location", input.Name)
		}
	}
	return nil
}

// buildRequestBody 从完整 JSON 文件读取生成命令的请求正文。
// buildRequestBody reads a generated command request body from a complete JSON file.
func buildRequestBody(command *cobra.Command, bodyInputs []operation.InputSpec, requestFile string) (any, error) {
	if len(bodyInputs) == 0 {
		return nil, nil
	}
	required := false
	for _, input := range bodyInputs {
		if input.Required {
			required = true
			break
		}
	}
	path := strings.TrimSpace(requestFile)
	if path == "" {
		if required {
			return nil, clioutput.NewError(clioutput.CodeUsage, "required flag --request-file is missing", clioutput.ExitUsage, false)
		}
		return nil, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, clioutput.NewError(clioutput.CodeUsage, fmt.Sprintf("read request file: %v", err), clioutput.ExitUsage, false)
	}
	content = []byte(strings.TrimSpace(string(content)))
	if len(content) == 0 || !json.Valid(content) {
		return nil, clioutput.NewError(clioutput.CodeUsage, "request file must contain valid JSON", clioutput.ExitUsage, false)
	}
	return json.RawMessage(content), nil
}

func methodSupportsJSONBody(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

// prepareWriteHeaders 校验写操作，并构造统一安全请求头。
// prepareWriteHeaders validates a write and builds its common safety headers.
func prepareWriteHeaders(command *cobra.Command, spec operation.OperationSpec, options *writeOptions) (map[string]string, error) {
	if spec.Risk == operation.RiskR0 {
		return nil, nil
	}
	if strings.TrimSpace(options.idempotencyKey) == "" {
		options.idempotencyKey = uuid.NewString()
		_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
			Event: "idempotency_key", OperationID: spec.ID, Level: "info", Message: options.idempotencyKey,
		})
	}
	if spec.Impact != nil {
		_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
			Event: "warning", OperationID: spec.ID, Level: "warning", Message: spec.Impact.Message,
		})
	}
	headers := map[string]string{"Idempotency-Key": options.idempotencyKey, "X-STX-Confirm": "true"}
	if strings.TrimSpace(options.confirmationID) != "" {
		headers["X-STX-Confirmation-ID"] = strings.TrimSpace(options.confirmationID)
	}
	return headers, nil
}

// handleConfirmationError 在服务端要求二次确认时输出可重试信息。
// handleConfirmationError emits retry details when the server requires one-time confirmation.
func handleConfirmationError(command *cobra.Command, operationID string, err error) error {
	var apiErr *cliClient.APIError
	if !errors.As(err, &apiErr) || apiErr.ErrorCode != "confirmation_required" || len(apiErr.Data) == 0 {
		return err
	}
	var data struct {
		ConfirmationID string `json:"confirmation_id"`
		RiskLevel      string `json:"risk_level"`
		Impact         string `json:"impact"`
		ExpiresAt      string `json:"expires_at"`
	}
	if decodeErr := json.Unmarshal(apiErr.Data, &data); decodeErr != nil || strings.TrimSpace(data.ConfirmationID) == "" {
		return err
	}
	_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
		Event: "confirmation_required", OperationID: operationID, RequestID: apiErr.RequestID, Level: "warning",
		Message: data.Impact, ConfirmationID: data.ConfirmationID, RiskLevel: data.RiskLevel, ExpiresAt: data.ExpiresAt,
	})
	return err
}

func isGeneratedSafetyHeader(name string) bool {
	switch http.CanonicalHeaderKey(strings.TrimSpace(name)) {
	case "Idempotency-Key", "X-Stx-Confirm", "X-Stx-Confirmation-Id":
		return true
	default:
		return false
	}
}

// longDescription 组合登记操作、影响说明和输出样例，帮助阶段不访问服务端。
// longDescription combines the registered operation, impact notice, and output example without contacting the server.
func longDescription(spec operation.OperationSpec) string {
	sections := []string{fmt.Sprintf("Run registered operation %s.", spec.ID)}
	if spec.Impact != nil {
		impact := fmt.Sprintf("Impact:\nRisk level: %s\n%s", spec.Impact.Level, spec.Impact.Message)
		if strings.TrimSpace(spec.Impact.Performance) != "" {
			impact += "\nPerformance: " + spec.Impact.Performance
		}
		sections = append(sections, impact)
	}
	sections = append(sections, "Output example:\n"+spec.OutputExample)
	return strings.Join(sections, "\n\n")
}

func buildRequestPath(command *cobra.Command, spec operation.OperationSpec, pathInputs, queryInputs []operation.InputSpec, queryValues map[string]queryFlagValue, args []string) (string, error) {
	requestPath := spec.Route
	for index, input := range pathInputs {
		requestPath = strings.Replace(requestPath, ":"+input.Name, url.PathEscape(args[index]), 1)
	}

	query := make(url.Values)
	for _, input := range queryInputs {
		changed := command.Flags().Changed(input.Name)
		flagValue := queryValues[input.Name]
		if input.Repeated {
			values := nonEmptyValues(flagValue.repeated)
			if input.Required && (!changed || len(values) == 0) {
				return "", clioutput.NewError(clioutput.CodeUsage, "required flag --"+input.Name+" is missing", clioutput.ExitUsage, false)
			}
			if changed {
				for _, value := range values {
					query.Add(input.Name, value)
				}
			}
			continue
		}

		value := strings.TrimSpace(*flagValue.single)
		if input.Required && (!changed || value == "") {
			return "", clioutput.NewError(clioutput.CodeUsage, "required flag --"+input.Name+" is missing", clioutput.ExitUsage, false)
		}
		if changed {
			query.Set(input.Name, value)
		}
	}
	if encoded := query.Encode(); encoded != "" {
		requestPath += "?" + encoded
	}
	return requestPath, nil
}

// nonEmptyValues 清理重复查询参数，并保留用户传入的先后顺序。
// nonEmptyValues cleans repeated query values while preserving their input order.
func nonEmptyValues(values *[]string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, 0, len(*values))
	for _, value := range *values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func checkCapability(ctx context.Context, client Client, spec operation.OperationSpec) error {
	requestID, capabilities, err := client.Capabilities(ctx)
	if err != nil {
		return err
	}
	for _, remote := range capabilities.Operations {
		if remote.OperationID != spec.ID {
			continue
		}
		if !remote.Allowed {
			message := "operation is not allowed by the remote STX server"
			if remote.DenialCode != "" {
				message += ": " + remote.DenialCode
			}
			return capabilityError(clioutput.CodePermission, message, clioutput.ExitPermission, requestID)
		}
		if remote.Revision < spec.Revision {
			message := fmt.Sprintf("operation revision is incompatible: local=%d remote=%d", spec.Revision, remote.Revision)
			return capabilityError(clioutput.CodeConflict, message, clioutput.ExitConflict, requestID)
		}
		if remote.Mode != string(spec.Mode) {
			message := fmt.Sprintf("operation mode is incompatible: local=%s remote=%s", spec.Mode, remote.Mode)
			return capabilityError(clioutput.CodeConflict, message, clioutput.ExitConflict, requestID)
		}
		return nil
	}
	return capabilityError(clioutput.CodeNotFound, "operation is not supported by the remote STX server: "+spec.ID, clioutput.ExitNotFound, requestID)
}

func capabilityError(code, message string, exitCode clioutput.ExitCode, requestID string) error {
	return &clioutput.CLIError{Code: code, Message: message, ExitCode: exitCode, RequestID: requestID}
}

func inputsAt(spec operation.OperationSpec, location operation.InputLocation) []operation.InputSpec {
	result := make([]operation.InputSpec, 0)
	for _, input := range spec.Input {
		if input.Location == location {
			result = append(result, input)
		}
	}
	return result
}

func noArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return clioutput.NewError(clioutput.CodeUsage, fmt.Sprintf("accepts 0 arg(s), received %d", len(args)), clioutput.ExitUsage, false)
	}
	return nil
}

func exactArgs(expected int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != expected {
			return clioutput.NewError(clioutput.CodeUsage, fmt.Sprintf("accepts %d arg(s), received %d", expected, len(args)), clioutput.ExitUsage, false)
		}
		return nil
	}
}
