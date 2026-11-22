//go:build windows
// +build windows

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

package process

import "os/exec"

// SetDetachedSession 在 Windows 上不执行任何操作：Windows 没有 setsid 语义，进程隔离方式不同。
// SetDetachedSession is a no-op on Windows: there is no setsid semantics and process isolation
// works differently.
func SetDetachedSession(cmd *exec.Cmd) {
	// Windows 无 setsid 语义，不执行任何操作。 / No-op on Windows: no setsid semantics.
}
