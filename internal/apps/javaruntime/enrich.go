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

package javaruntime

import (
	"context"
	"strings"
)

// ParamInstallDir 是命令参数中的安装目录键，用于定位节点。
// ParamInstallDir is the install-directory key in command parameters, used to locate the node.
const ParamInstallDir = "install_dir"

// NodeJavaHomeLookup 按 Agent 与规范化后的安装目录返回该目录下所有节点记录的 java_home（可含空值）。
// NodeJavaHomeLookup returns the java_home of every node record under the normalized install directory of the
// Agent (empty values included).
type NodeJavaHomeLookup func(ctx context.Context, agentID, installDir string) ([]string, error)

// EnrichParams 是节点 JDK 的集中下发规则（AC4c）：命令带 install_dir 且调用方未给 java_home 键时，
// 从 cluster_nodes.java_home（唯一来源）补上该节点的 JDK。只有该目录下的节点记录全部指向同一个非空路径时才补全；
// 未指定、无匹配或有歧义都不补，Agent 保持原有探测（AC6）。返回需要新增的参数，nil 表示不补。
// EnrichParams is the central node-JDK dispatch rule (AC4c): when a command carries install_dir and the caller
// gave no java_home key, the node JDK is filled from cluster_nodes.java_home (the single source). It only fills
// when every node record under that directory points to the same non-empty path; unset, unmatched or ambiguous
// cases add nothing and the Agent keeps its original detection (AC6). It returns parameters to add, nil for none.
func EnrichParams(ctx context.Context, agentID string, params map[string]string, lookup NodeJavaHomeLookup) map[string]string {
	if lookup == nil || strings.TrimSpace(agentID) == "" || params == nil {
		return nil
	}
	if _, explicit := params[ParamJavaHome]; explicit {
		return nil
	}
	installDir := Normalize(params[ParamInstallDir])
	if installDir == "" || installDir == "." {
		return nil
	}
	homes, err := lookup(ctx, agentID, installDir)
	if err != nil || len(homes) == 0 {
		return nil
	}
	selected := Normalize(homes[0])
	for _, home := range homes[1:] {
		if Normalize(home) != selected {
			return nil
		}
	}
	if selected == "" {
		return nil
	}
	return map[string]string{ParamJavaHome: selected}
}
