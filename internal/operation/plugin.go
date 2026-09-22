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

import "fmt"

// pluginWriteOperationSpecs 登记插件下载、依赖和集群安装写操作，不登记任何删除路由。
// pluginWriteOperationSpecs registers plugin download, dependency, and cluster installation writes without exposing delete routes.
func pluginWriteOperationSpecs() []OperationSpec {
	pluginName := InputSpec{Name: "name", Location: InputPath, Required: true, Description: "Plugin name"}
	clusterID := InputSpec{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}
	version := InputSpec{Name: "version", Location: InputBody, Required: true, Description: "SeaTunnel version"}
	mirror := InputSpec{Name: "mirror", Location: InputBody, Description: "Mirror source: apache, aliyun, or huaweicloud"}
	profileKeys := InputSpec{Name: "profile_keys", Location: InputBody, Description: "Selected dependency profile keys"}
	return []OperationSpec{
		pluginWriteOperation("plugin.refresh", []string{"plugin", "refresh"}, "Refresh the SeaTunnel plugin catalog", "POST", "/api/v1/plugins/refresh", RiskR1,
			"刷新会联网读取插件目录并更新 STX 本地目录缓存。", false, false,
			[]InputSpec{{Name: "version", Location: InputBody, Description: "SeaTunnel version"}, mirror},
			"stx plugin refresh --version 2.3.13 --mirror apache --confirm"),
		pluginWriteOperation("plugin.download.start", []string{"plugin", "download", "start"}, "Download one SeaTunnel plugin", "POST", "/api/v1/plugins/:name/download", RiskR1,
			"下载会占用 STX 服务的网络带宽和本地磁盘空间。", false, true,
			[]InputSpec{pluginName, version, mirror, profileKeys},
			"stx plugin download start jdbc --version 2.3.13 --profile-key mysql --confirm"),
		pluginWriteOperation("plugin.download-all.start", []string{"plugin", "download", "all"}, "Download all SeaTunnel plugins", "POST", "/api/v1/plugins/download-all", RiskR1,
			"批量下载会持续占用 STX 服务的网络带宽和较多本地磁盘空间。", false, true,
			[]InputSpec{version, mirror, InputSpec{Name: "selected_plugin_profiles", Location: InputBody, Description: "JSON file containing profile selections keyed by plugin"}},
			"stx plugin download all --version 2.3.13 --profiles-file ./profiles.json --confirm"),
		pluginWriteOperation("plugin.dependency.add", []string{"plugin", "dependency", "add"}, "Add a Maven dependency for a SeaTunnel plugin", "POST", "/api/v1/plugins/:name/dependencies", RiskR1,
			"新增依赖会改变该插件后续下载和安装时附带的 Jar。", false, false,
			[]InputSpec{pluginName, InputSpec{Name: "seatunnel_version", Location: InputBody, Description: "SeaTunnel version"}, InputSpec{Name: "group_id", Location: InputBody, Required: true, Description: "Maven group ID"}, InputSpec{Name: "artifact_id", Location: InputBody, Required: true, Description: "Maven artifact ID"}, InputSpec{Name: "version", Location: InputBody, Required: true, Description: "Dependency version"}, InputSpec{Name: "target_dir", Location: InputBody, Description: "Target directory"}},
			"stx plugin dependency add jdbc --seatunnel-version 2.3.13 --group-id com.example --artifact-id example-driver --version 1.0.0 --confirm"),
		pluginWriteOperation("plugin.dependency.upload", []string{"plugin", "dependency", "upload"}, "Upload a custom Jar dependency for a SeaTunnel plugin", "POST", "/api/v1/plugins/:name/dependencies/upload", RiskR1,
			"上传会把自定义 Jar 保存到 STX，并让后续插件安装使用该文件。", false, false,
			[]InputSpec{pluginName, InputSpec{Name: "file", Location: InputFile, Required: true, Description: "Jar file"}, InputSpec{Name: "seatunnel_version", Location: InputBody, Description: "SeaTunnel version"}, InputSpec{Name: "group_id", Location: InputBody, Description: "Maven group ID"}, InputSpec{Name: "artifact_id", Location: InputBody, Description: "Maven artifact ID"}, InputSpec{Name: "version", Location: InputBody, Description: "Dependency version"}, InputSpec{Name: "target_dir", Location: InputBody, Description: "Target directory"}},
			"stx plugin dependency upload jdbc ./mysql-driver.jar --seatunnel-version 2.3.13 --confirm"),
		pluginWriteOperation("plugin.dependency.disable", []string{"plugin", "dependency", "disable"}, "Disable an official dependency for a SeaTunnel plugin", "POST", "/api/v1/plugins/:name/dependencies/disables", RiskR1,
			"禁用官方依赖会让该 Jar 不再随插件下载和安装，可能导致连接器无法运行。", false, false,
			[]InputSpec{pluginName, InputSpec{Name: "seatunnel_version", Location: InputBody, Description: "SeaTunnel version"}, InputSpec{Name: "group_id", Location: InputBody, Required: true, Description: "Maven group ID"}, InputSpec{Name: "artifact_id", Location: InputBody, Required: true, Description: "Maven artifact ID"}, InputSpec{Name: "version", Location: InputBody, Required: true, Description: "Dependency version"}, InputSpec{Name: "target_dir", Location: InputBody, Required: true, Description: "Target directory"}},
			"stx plugin dependency disable jdbc --seatunnel-version 2.3.13 --group-id mysql --artifact-id mysql-connector-java --version 8.0.27 --target-dir lib --confirm"),
		pluginWriteOperation("plugin.official-dependency.analyze", []string{"plugin", "official-dependency", "analyze"}, "Analyze official dependencies for a SeaTunnel plugin", "POST", "/api/v1/plugins/:name/official-dependencies/analyze", RiskR1,
			"在线分析会访问外部文档或仓库，并更新 STX 保存的官方依赖信息。", false, false,
			[]InputSpec{pluginName, InputSpec{Name: "version", Location: InputBody, Description: "SeaTunnel version"}, InputSpec{Name: "profile_key", Location: InputBody, Description: "Dependency profile key"}, InputSpec{Name: "force_refresh", Location: InputBody, Description: "Force remote refresh"}},
			"stx plugin official-dependency analyze jdbc --version 2.3.13 --profile-key mysql --confirm"),
		pluginWriteOperation("cluster.plugin.install", []string{"cluster", "plugin", "install"}, "Install a SeaTunnel plugin on a cluster", "POST", "/api/v1/clusters/:id/plugins", RiskR1,
			"安装会把插件和依赖发送到集群节点，并写入节点安装目录。", true, true,
			[]InputSpec{clusterID, InputSpec{Name: "plugin_name", Location: InputBody, Required: true, Description: "Plugin name"}, version, mirror, profileKeys},
			"stx cluster plugin install 6 --plugin jdbc --version 2.3.13 --profile-key mysql --confirm"),
		pluginWriteOperation("cluster.plugin.enable", []string{"cluster", "plugin", "enable"}, "Enable a plugin on a cluster", "PUT", "/api/v1/clusters/:id/plugins/:name/enable", RiskR1,
			"启用会让集群重新使用该插件。", false, false, []InputSpec{clusterID, pluginName},
			"stx cluster plugin enable 6 jdbc --confirm"),
		pluginWriteOperation("cluster.plugin.disable", []string{"cluster", "plugin", "disable"}, "Disable a plugin on a cluster", "PUT", "/api/v1/clusters/:id/plugins/:name/disable", RiskR1,
			"禁用会阻止集群继续使用该插件，引用它的任务可能失败。", false, false, []InputSpec{clusterID, pluginName},
			"stx cluster plugin disable 6 jdbc --confirm"),
	}
}

// pluginWriteOperation 构造由专用 CLI 处理的插件写操作登记。
// pluginWriteOperation builds a plugin write registration handled by dedicated CLI code.
func pluginWriteOperation(id string, commandPath []string, summary, method, route string, risk RiskLevel, impact string, usesAgent, async bool, inputs []InputSpec, example string) OperationSpec {
	inputs = append(inputs,
		InputSpec{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable retry key"},
		InputSpec{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation"},
	)
	return OperationSpec{
		ID: id, CommandPath: commandPath, Summary: summary, GeneratedCLI: false, Method: method, Route: route,
		Mode: ModeNormal, AuthRequired: true, Risk: risk, Revision: 1, UsesAgent: usesAgent, Async: async,
		SupportsPick: true, Impact: &ImpactSpec{Level: risk, Message: impact}, Input: inputs, Example: example,
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{},"result_meta":{"complete":true}}`, id),
	}
}
