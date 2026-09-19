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

package audit

// ActionsForGroup 把界面上的少数操作类别展开成实际写入的 action。
// ActionsForGroup expands a small UI action category into the actions that are actually stored.
func ActionsForGroup(group string) []string {
	switch group {
	case "create":
		return []string{"create", "add_node", "add_nodes"}
	case "update":
		return []string{"update", "update_node", "enable", "disable", "enable_official_dependency", "disable_official_dependency", "analyze_official_dependency"}
	case "delete":
		return []string{"delete", "remove_node", "delete_local", "delete_dependency"}
	case "lifecycle":
		return []string{"start", "stop", "restart", "start_node", "stop_node", "restart_node", "crashed", "restart_failed"}
	case "install":
		return []string{"install", "uninstall", "download", "download_all", "upload_dependency", "add_dependency"}
	case "diagnose":
		return []string{"diagnostics.file.read", "diagnostics.file.preview", "diagnostics.bundle.download"}
	case "task":
		return []string{"execution.create", "execution.start", "execution.result"}
	default:
		return nil
	}
}
