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

//go:build !linux

package embedts

import "errors"

// errStatfsUnsupported 是非 Linux 平台的占位错误：设计约定非 Linux 一律退回 30GiB。
// errStatfsUnsupported is the non-Linux placeholder: the design falls back to 30GiB there.
var errStatfsUnsupported = errors.New("statfs unsupported on this platform")

// defaultFsTotalBytes 非 Linux 平台不支持 statfs，返回错误让 autoMaxBytes 退回默认值。
// defaultFsTotalBytes is unsupported off Linux; the error makes autoMaxBytes take the fallback.
func defaultFsTotalBytes(_ string) (int64, error) {
	return 0, errStatfsUnsupported
}
