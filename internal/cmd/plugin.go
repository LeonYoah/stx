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

package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

// addPluginWriteCommands 将插件下载、依赖和集群安装命令挂到现有查询命令树。
// addPluginWriteCommands attaches plugin download, dependency, and cluster installation writes to the existing query tree.
func addPluginWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	pluginCommand := childCommand(root, "plugin")
	if pluginCommand == nil {
		panic("generated plugin command is missing")
	}
	pluginCommand.AddCommand(newPluginRefreshCommand(storeProvider))

	downloadCommand := childCommand(pluginCommand, "download")
	dependencyCommand := childCommand(pluginCommand, "dependency")
	officialDependencyCommand := childCommand(pluginCommand, "official-dependency")
	if downloadCommand == nil || dependencyCommand == nil || officialDependencyCommand == nil {
		panic("generated plugin subcommand is missing")
	}
	downloadCommand.AddCommand(newPluginDownloadStartCommand(storeProvider), newPluginDownloadAllCommand(storeProvider))
	dependencyCommand.AddCommand(
		newPluginDependencyAddCommand(storeProvider),
		newPluginDependencyUploadCommand(storeProvider),
		newPluginDependencyDisableCommand(storeProvider),
	)
	officialDependencyCommand.AddCommand(newPluginOfficialDependencyAnalyzeCommand(storeProvider))

	clusterCommand := childCommand(root, "cluster")
	clusterPluginCommand := childCommand(clusterCommand, "plugin")
	if clusterPluginCommand == nil {
		panic("generated cluster plugin command is missing")
	}
	clusterPluginCommand.AddCommand(
		newClusterPluginInstallCommand(storeProvider),
		newClusterPluginStatusCommand(storeProvider, "enable"),
		newClusterPluginStatusCommand(storeProvider, "disable"),
	)
}

func newPluginRefreshCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var version, mirror string
	command := &cobra.Command{
		Use:     "refresh",
		Short:   "Refresh the SeaTunnel plugin catalog",
		Example: "stx plugin refresh --version 2.3.13 --mirror apache --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			body := map[string]any{}
			setChangedString(command, body, "version", version)
			setChangedString(command, body, "mirror", mirror)
			return executePluginWrite(command, storeProvider, "plugin.refresh", &options, http.MethodPost, "/api/v1/plugins/refresh", body, "stx plugin list")
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version")
	command.Flags().StringVar(&mirror, "mirror", "", "Mirror source: apache, aliyun, or huaweicloud")
	return command
}

func newPluginDownloadStartCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var version, mirror, requestFile string
	var profileKeys []string
	command := &cobra.Command{
		Use:     "start <name>",
		Short:   "Download one SeaTunnel plugin",
		Example: "stx plugin download start jdbc --version 2.3.13 --profile-key mysql --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if strings.TrimSpace(version) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--version is required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"version": strings.TrimSpace(version)}
				setChangedString(command, body, "mirror", mirror)
				if command.Flags().Changed("profile-key") {
					body["profile_keys"] = nonEmptyStrings(profileKeys)
				}
			}
			path := "/api/v1/plugins/" + url.PathEscape(args[0]) + "/download"
			next := "stx plugin download status get " + args[0]
			if bodyVersion, ok := body["version"].(string); ok && strings.TrimSpace(bodyVersion) != "" {
				next += " --version " + strings.TrimSpace(bodyVersion)
			}
			return executePluginWrite(command, storeProvider, "plugin.download.start", &options, http.MethodPost, path, body, next)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version")
	command.Flags().StringVar(&mirror, "mirror", "", "Mirror source: apache, aliyun, or huaweicloud")
	command.Flags().StringSliceVar(&profileKeys, "profile-key", nil, "Dependency profile key; may be repeated")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newPluginDownloadAllCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var version, mirror, profilesFile, requestFile string
	command := &cobra.Command{
		Use:     "all",
		Short:   "Download all SeaTunnel plugins",
		Example: "stx plugin download all --version 2.3.13 --profiles-file ./profiles.json --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if strings.TrimSpace(version) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--version is required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"version": strings.TrimSpace(version)}
				setChangedString(command, body, "mirror", mirror)
				if strings.TrimSpace(profilesFile) != "" {
					var profiles map[string][]string
					if err := readJSONFile(profilesFile, &profiles); err != nil {
						return err
					}
					body["selected_plugin_profiles"] = profiles
				}
			}
			return executePluginWrite(command, storeProvider, "plugin.download-all.start", &options, http.MethodPost, "/api/v1/plugins/download-all", body, "stx plugin download list")
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version")
	command.Flags().StringVar(&mirror, "mirror", "", "Mirror source: apache, aliyun, or huaweicloud")
	command.Flags().StringVar(&profilesFile, "profiles-file", "", "JSON object mapping plugin names to profile keys")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newPluginDependencyAddCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var seatunnelVersion, groupID, artifactID, version, targetDir, requestFile string
	command := &cobra.Command{
		Use:     "add <name>",
		Short:   "Add a Maven dependency for a SeaTunnel plugin",
		Example: "stx plugin dependency add jdbc --seatunnel-version 2.3.13 --group-id com.example --artifact-id example-driver --version 1.0.0 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if strings.TrimSpace(groupID) == "" || strings.TrimSpace(artifactID) == "" || strings.TrimSpace(version) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--group-id, --artifact-id, and --version are required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"group_id": strings.TrimSpace(groupID), "artifact_id": strings.TrimSpace(artifactID), "version": strings.TrimSpace(version)}
				setChangedString(command, body, "seatunnel-version", seatunnelVersion)
				setChangedString(command, body, "target-dir", targetDir)
			}
			path := "/api/v1/plugins/" + url.PathEscape(args[0]) + "/dependencies"
			return executePluginWrite(command, storeProvider, "plugin.dependency.add", &options, http.MethodPost, path, body, "stx plugin dependency list "+args[0])
		},
	}
	addSecureWriteFlags(command, &options)
	addPluginDependencyFlags(command, &seatunnelVersion, &groupID, &artifactID, &version, &targetDir)
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newPluginDependencyUploadCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var seatunnelVersion, groupID, artifactID, version, targetDir string
	command := &cobra.Command{
		Use:     "upload <name> <file>",
		Short:   "Upload a custom Jar dependency for a SeaTunnel plugin",
		Example: "stx plugin dependency upload jdbc ./mysql-driver.jar --seatunnel-version 2.3.13 --confirm",
		Args:    usageArgs(cobra.ExactArgs(2)),
		RunE: func(command *cobra.Command, args []string) error {
			file, err := os.Open(args[1])
			if err != nil {
				return clioutput.WrapError(err, clioutput.CodeUsage, "open dependency file", clioutput.ExitUsage, false)
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				return clioutput.NewError(clioutput.CodeUsage, "dependency file must be a regular file", clioutput.ExitUsage, false)
			}
			client, headers, err := prepareSecureWrite(command, storeProvider, "plugin.dependency.upload", &options, pluginOperationImpact("plugin.dependency.upload"))
			if err != nil {
				return err
			}
			fields := map[string]string{
				"seatunnel_version": strings.TrimSpace(seatunnelVersion), "group_id": strings.TrimSpace(groupID),
				"artifact_id": strings.TrimSpace(artifactID), "version": strings.TrimSpace(version), "target_dir": strings.TrimSpace(targetDir),
			}
			var data any
			path := "/api/v1/plugins/" + url.PathEscape(args[0]) + "/dependencies/upload"
			requestID, err := client.RequestMultipart(command.Context(), http.MethodPost, path, fields, "file", filepath.Base(args[1]), file, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, "plugin.dependency.upload", err)
			}
			return renderWriteResult(command, "plugin.dependency.upload", requestID, data, "stx plugin dependency list "+args[0])
		},
	}
	addSecureWriteFlags(command, &options)
	addPluginDependencyFlags(command, &seatunnelVersion, &groupID, &artifactID, &version, &targetDir)
	return command
}

func newPluginDependencyDisableCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var seatunnelVersion, groupID, artifactID, version, targetDir, requestFile string
	command := &cobra.Command{
		Use:     "disable <name>",
		Short:   "Disable an official dependency for a SeaTunnel plugin",
		Example: "stx plugin dependency disable jdbc --seatunnel-version 2.3.13 --group-id mysql --artifact-id mysql-connector-java --version 8.0.27 --target-dir lib --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if strings.TrimSpace(groupID) == "" || strings.TrimSpace(artifactID) == "" || strings.TrimSpace(version) == "" || strings.TrimSpace(targetDir) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--group-id, --artifact-id, --version, and --target-dir are required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"group_id": strings.TrimSpace(groupID), "artifact_id": strings.TrimSpace(artifactID), "version": strings.TrimSpace(version), "target_dir": strings.TrimSpace(targetDir)}
				setChangedString(command, body, "seatunnel-version", seatunnelVersion)
			}
			path := "/api/v1/plugins/" + url.PathEscape(args[0]) + "/dependencies/disables"
			return executePluginWrite(command, storeProvider, "plugin.dependency.disable", &options, http.MethodPost, path, body, "stx plugin official-dependency list "+args[0])
		},
	}
	addSecureWriteFlags(command, &options)
	addPluginDependencyFlags(command, &seatunnelVersion, &groupID, &artifactID, &version, &targetDir)
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newPluginOfficialDependencyAnalyzeCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var version, profileKey, requestFile string
	var forceRefresh bool
	command := &cobra.Command{
		Use:     "analyze <name>",
		Short:   "Analyze official dependencies for a SeaTunnel plugin",
		Example: "stx plugin official-dependency analyze jdbc --version 2.3.13 --profile-key mysql --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				body = map[string]any{}
				setChangedString(command, body, "version", version)
				setChangedString(command, body, "profile-key", profileKey)
				if command.Flags().Changed("force-refresh") {
					body["force_refresh"] = forceRefresh
				}
			}
			path := "/api/v1/plugins/" + url.PathEscape(args[0]) + "/official-dependencies/analyze"
			return executePluginWrite(command, storeProvider, "plugin.official-dependency.analyze", &options, http.MethodPost, path, body, "stx plugin official-dependency list "+args[0])
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version")
	command.Flags().StringVar(&profileKey, "profile-key", "", "Dependency profile key")
	command.Flags().BoolVar(&forceRefresh, "force-refresh", false, "Force remote refresh")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterPluginInstallCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var pluginName, version, mirror, requestFile string
	var profileKeys []string
	command := &cobra.Command{
		Use:     "install <cluster-id>",
		Short:   "Install a SeaTunnel plugin on a cluster",
		Example: "stx cluster plugin install 6 --plugin jdbc --version 2.3.13 --profile-key mysql --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if strings.TrimSpace(pluginName) == "" || strings.TrimSpace(version) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--plugin and --version are required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"plugin_name": strings.TrimSpace(pluginName), "version": strings.TrimSpace(version)}
				setChangedString(command, body, "mirror", mirror)
				if command.Flags().Changed("profile-key") {
					body["profile_keys"] = nonEmptyStrings(profileKeys)
				}
			}
			next := "stx cluster plugin list " + args[0]
			if name, ok := body["plugin_name"].(string); ok && strings.TrimSpace(name) != "" {
				next = "stx cluster plugin progress get " + args[0] + " " + strings.TrimSpace(name)
			}
			return executePluginWrite(command, storeProvider, "cluster.plugin.install", &options, http.MethodPost, "/api/v1/clusters/"+url.PathEscape(args[0])+"/plugins", body, next)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&pluginName, "plugin", "", "Plugin name")
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version")
	command.Flags().StringVar(&mirror, "mirror", "", "Mirror source: apache, aliyun, or huaweicloud")
	command.Flags().StringSliceVar(&profileKeys, "profile-key", nil, "Dependency profile key; may be repeated")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterPluginStatusCommand(storeProvider authStoreProvider, action string) *cobra.Command {
	var options secureWriteOptions
	operationID := "cluster.plugin." + action
	command := &cobra.Command{
		Use:     action + " <cluster-id> <name>",
		Short:   strings.ToUpper(action[:1]) + action[1:] + " a plugin on a cluster",
		Example: fmt.Sprintf("stx cluster plugin %s 6 jdbc --confirm", action),
		Args:    usageArgs(cobra.ExactArgs(2)),
		RunE: func(command *cobra.Command, args []string) error {
			path := fmt.Sprintf("/api/v1/clusters/%s/plugins/%s/%s", url.PathEscape(args[0]), url.PathEscape(args[1]), action)
			return executePluginWrite(command, storeProvider, operationID, &options, http.MethodPut, path, map[string]any{}, "stx cluster plugin list "+args[0])
		},
	}
	addSecureWriteFlags(command, &options)
	return command
}

// executePluginWrite 使用公共确认、能力检查和结果格式执行插件写请求。
// executePluginWrite executes a plugin write using common confirmation, capability checks, and result rendering.
func executePluginWrite(command *cobra.Command, storeProvider authStoreProvider, operationID string, options *secureWriteOptions, method, path string, body map[string]any, nextCommand string) error {
	client, headers, err := prepareSecureWrite(command, storeProvider, operationID, options, pluginOperationImpact(operationID))
	if err != nil {
		return err
	}
	var data any
	requestID, err := client.RequestWithHeaders(command.Context(), method, path, body, headers, &data)
	if err != nil {
		return handleSecureWriteError(command, operationID, err)
	}
	return renderWriteResult(command, operationID, requestID, data, nextCommand)
}

func pluginOperationImpact(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID && spec.Impact != nil {
			return spec.Impact.Message
		}
	}
	return "this operation changes plugin state"
}

func addPluginDependencyFlags(command *cobra.Command, seatunnelVersion, groupID, artifactID, version, targetDir *string) {
	command.Flags().StringVar(seatunnelVersion, "seatunnel-version", "", "SeaTunnel version")
	command.Flags().StringVar(groupID, "group-id", "", "Maven group ID")
	command.Flags().StringVar(artifactID, "artifact-id", "", "Maven artifact ID")
	command.Flags().StringVar(version, "version", "", "Dependency version")
	command.Flags().StringVar(targetDir, "target-dir", "", "Target directory")
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
