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

// Package install 内嵌安装目录布局文件，供运行时自修复使用。
// Package install embeds install-layout files for runtime self-healing.
package install

import "embed"

// BinScripts 内嵌 bin/ 下的三个控制脚本。
// 执行一键升级的是旧升级器，它不会替换 bin/；新二进制启动时用这份内嵌内容刷新安装目录的脚本。
// 脚本内容没有安装期占位符（BASE_DIR 按脚本位置推算），可以原样写回。
// BinScripts embeds the three control scripts under bin/.
// The in-app upgrade is executed by the OLD upgrader, which never replaces bin/; the new binary
// refreshes the install dir scripts from this embedded copy at startup. The scripts carry no
// install-time placeholders (BASE_DIR resolves from the script location), so they can be written back as-is.
//
//go:embed bin/start.sh bin/stop.sh bin/status.sh
var BinScripts embed.FS
