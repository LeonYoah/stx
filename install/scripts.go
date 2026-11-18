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

package install

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// InstallScriptIfChanged 在 binDir 下安装一个脚本：内容相同直接返回 false；不同时先把旧文件
// 改名为 <name>.bak-<UTC 时间戳>（无旧文件跳过备份），新内容写入 .<name>.tmp，chmod 0755 后原子
// rename。备份文件名不以 .sh 结尾，防止被旧脚本误 source。启动自修复与新升级器共用此函数。
// InstallScriptIfChanged installs one script under binDir: identical content is a no-op (false);
// otherwise the old file is renamed to <name>.bak-<UTC timestamp> (skipped when absent), the new
// content goes to .<name>.tmp, gets chmod 0755 and is atomically renamed. The backup name never
// ends in .sh so it cannot be sourced by accident. Shared by the startup layout heal and the
// new upgrader.
func InstallScriptIfChanged(binDir, name string, content []byte, now time.Time) (replaced bool, backup string, err error) {
	dst := filepath.Join(binDir, name)
	if old, readErr := os.ReadFile(dst); readErr == nil && string(old) == string(content) {
		return false, "", nil
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		backup = dst + ".bak-" + now.UTC().Format("20060102T150405Z")
		if err := os.Rename(dst, backup); err != nil {
			return false, "", fmt.Errorf("backup %s: %w", name, err)
		}
	}
	tmp := filepath.Join(binDir, "."+name+".tmp")
	if err := os.WriteFile(tmp, content, 0o755); err != nil {
		return false, backup, fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return false, backup, fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return false, backup, fmt.Errorf("rename %s: %w", tmp, err)
	}
	return true, backup, nil
}
