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

package db

import (
	"fmt"
)

// JSONScanBytes 将数据库驱动返回的 JSON 列值规范为 []byte。
// SQLite TEXT 列常以 string 返回，BLOB/部分驱动以 []byte 返回；两者都接受。
// nil 表示 SQL NULL；非法驱动类型返回错误（非法 JSON 由调用方 Unmarshal 判定）。
//
// JSONScanBytes normalizes a driver JSON column value to []byte.
// SQLite TEXT columns often arrive as string; BLOB / some drivers use []byte — both are accepted.
// nil means SQL NULL; unsupported driver types return an error (invalid JSON is left to Unmarshal).
func JSONScanBytes(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []byte:
		return typed, nil
	case string:
		return []byte(typed), nil
	default:
		return nil, fmt.Errorf("unsupported JSON scan type %T, expected []byte or string", value)
	}
}
