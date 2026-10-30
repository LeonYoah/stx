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
	"encoding/json"
	"strings"
)

// Candidate 是主机上探测到的一个 JDK，供向导下拉选择（AC1b / AC1b2）。
// Candidate is one JDK detected on a host for the wizard dropdown (AC1b / AC1b2).
type Candidate struct {
	JavaHome      string `json:"java_home"`
	JavaBin       string `json:"java_bin,omitempty"`
	Version       int    `json:"version"`
	VersionString string `json:"version_string,omitempty"`
	Source        string `json:"source"`
}

// ListParams 生成 list_java_homes 的 Agent 参数。
// ListParams builds Agent parameters for list_java_homes.
func ListParams() map[string]string {
	return map[string]string{"sub_command": "list_java_homes"}
}

// ParseList 解析 Agent list_java_homes 输出；解析失败或旧 Agent 返回空列表（不阻塞，AC1b）。
// ParseList parses the Agent list_java_homes output; parse failures or an old Agent yield an empty list (non-blocking, AC1b).
func ParseList(success bool, output string) []Candidate {
	if !success {
		return nil
	}
	var resp agentResponse
	if json.Unmarshal([]byte(strings.TrimSpace(output)), &resp) != nil {
		return nil
	}
	raw := strings.TrimSpace(resp.Details["jdks"])
	if raw == "" {
		return nil
	}
	var list []Candidate
	if json.Unmarshal([]byte(raw), &list) != nil {
		return nil
	}
	return list
}
