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
package processidentity

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

const installDirDigestBytes = 6

// ManagedName 根据安装目录和角色生成稳定且唯一的受管进程名。
// ManagedName returns a stable, unique managed process name from install directory and role.
func ManagedName(installDir, role string) string {
	prefix := "seatunnel"
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	if normalizedRole != "" && normalizedRole != "hybrid" && normalizedRole != "master/worker" {
		prefix += "-" + normalizedRole
	}

	normalizedInstallDir := strings.TrimSpace(installDir)
	if normalizedInstallDir == "" {
		return prefix
	}
	digest := sha256.Sum256([]byte(normalizedInstallDir))
	return fmt.Sprintf("%s@%x", prefix, digest[:installDirDigestBytes])
}
