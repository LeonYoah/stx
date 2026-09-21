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

// configWriteOperationSpecs 登记需要专用正文处理的配置命令。
// configWriteOperationSpecs registers config commands that require dedicated request-body handling.
func configWriteOperationSpecs() []OperationSpec {
	return []OperationSpec{
		configBodyOperation("config.normalize", []string{"config", "normalize"}, "Normalize SeaTunnel configuration content", "POST", "/api/v1/configs/normalize", RiskR0, "", false,
			[]InputSpec{{Name: "config_type", Location: InputBody, Required: true, Description: "Configuration type"}, {Name: "content", Location: InputBody, Required: true, Description: "Configuration content"}},
			"stx config normalize --config-type hazelcast-master.yaml --content-file ./hazelcast-master.yaml"),
		configBodyOperation("config.update", []string{"config", "update"}, "Update one configuration", "PUT", "/api/v1/configs/:id", RiskR1,
			"修改配置会创建新版本；节点配置还会尝试写入对应 SeaTunnel 节点。", true,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Configuration ID"}, {Name: "content", Location: InputBody, Required: true, Description: "Configuration content"}, {Name: "comment", Location: InputBody, Description: "Version comment"}},
			"stx config update 1 --content-file ./seatunnel.yaml --comment 'adjust worker settings' --confirm"),
		configBodyOperation("config.rollback", []string{"config", "rollback"}, "Rollback one configuration", "POST", "/api/v1/configs/:id/rollback", RiskR1,
			"回滚会把指定历史版本复制为新版本；节点配置还会尝试写入对应 SeaTunnel 节点。", true,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Configuration ID"}, {Name: "version", Location: InputBody, Required: true, Description: "Historical version number"}, {Name: "comment", Location: InputBody, Description: "Rollback comment"}},
			"stx config rollback 1 --version 2 --confirm"),
		configBodyOperation("config.promote", []string{"config", "promote"}, "Promote a node configuration to the cluster template", "POST", "/api/v1/configs/:id/promote", RiskR2,
			"提升节点配置会替换同类型集群模板，并更新数据库中的所有节点配置。", false,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Configuration ID"}, {Name: "comment", Location: InputBody, Description: "Promotion comment"}},
			"stx config promote 2 --comment 'verified on worker' --confirm"),
		configBodyOperation("config.sync", []string{"config", "sync"}, "Sync one node configuration from the cluster template", "POST", "/api/v1/configs/:id/sync", RiskR1,
			"同步会用集群模板替换节点配置，并尝试写入对应 SeaTunnel 节点。", true,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Configuration ID"}, {Name: "comment", Location: InputBody, Description: "Sync comment"}},
			"stx config sync 2 --confirm"),
		configBodyOperation("config.push", []string{"config", "push"}, "Push one configuration to its node", "POST", "/api/v1/configs/:id/push", RiskR2,
			"推送会直接覆盖目标节点安装目录中的配置文件，可能影响后续启动或重启。", true,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Configuration ID"}, {Name: "install_dir", Location: InputBody, Required: true, Description: "SeaTunnel installation directory"}},
			"stx config push 2 --install-dir /tmp/seatunnel-2.3.13 --confirm"),
		configBodyOperation("config.cluster.init", []string{"config", "cluster", "init"}, "Initialize cluster configurations from one node", "POST", "/api/v1/clusters/:id/configs/init", RiskR1,
			"初始化会从目标节点读取配置，并在 STX 中创建或更新集群模板和节点配置。", true,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "host_id", Location: InputBody, Required: true, Description: "Host ID"}, {Name: "install_dir", Location: InputBody, Required: true, Description: "SeaTunnel installation directory"}},
			"stx config cluster init 6 --host-id 10 --install-dir /tmp/seatunnel-2.3.13 --confirm"),
		configBodyOperation("config.cluster.sync-all", []string{"config", "cluster", "sync-all"}, "Sync one cluster template to all nodes", "POST", "/api/v1/clusters/:id/configs/sync-all", RiskR2,
			"批量同步会更新所有节点的同类型配置，并逐台写入 SeaTunnel 安装目录。", true,
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "config_type", Location: InputBody, Required: true, Description: "Configuration type"}},
			"stx config cluster sync-all 6 --config-type hazelcast-master.yaml --confirm"),
	}
}

// configBodyOperation 构造专用配置命令的登记项，并为风险操作补充公共请求头。
// configBodyOperation builds a dedicated config operation and adds common safety headers for risky writes.
func configBodyOperation(id string, commandPath []string, summary, method, route string, risk RiskLevel, impact string, usesAgent bool, inputs []InputSpec, example string) OperationSpec {
	if risk != RiskR0 {
		inputs = append(inputs,
			InputSpec{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable retry key"},
			InputSpec{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation"},
		)
	}
	var impactSpec *ImpactSpec
	if impact != "" {
		impactSpec = &ImpactSpec{Level: risk, Message: impact}
	}
	return OperationSpec{ID: id, CommandPath: commandPath, Summary: summary, GeneratedCLI: false, Method: method, Route: route, Mode: ModeNormal, AuthRequired: true, Risk: risk, Revision: 1, UsesAgent: usesAgent, SupportsPick: true, Impact: impactSpec, Input: inputs, Example: example,
		OutputExample: `{"api_version":"v1","operation_id":"` + id + `","request_id":"req_example","data":{},"result_meta":{"complete":true}}`}
}
