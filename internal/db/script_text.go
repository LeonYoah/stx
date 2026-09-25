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
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// ScriptText 是脚本/大配置内容的方言感知文本类型。
// ScriptText is a dialect-aware text type for scripts and large configuration payloads.
//
// MySQL → MEDIUMTEXT（约 16MB）/ MySQL → MEDIUMTEXT (~16MB)
// PostgreSQL / SQLite → TEXT（足够大）/ PostgreSQL / SQLite → TEXT (effectively unbounded)
//
// 注意：不要再写 gorm:"type:text"，否则会覆盖方言分流。
// Note: do not set gorm:"type:text"; that tag overrides dialect selection.
type ScriptText string

// String 返回底层字符串。
// String returns the underlying string value.
func (s ScriptText) String() string {
	return string(s)
}

// GormDataType 返回通用逻辑类型名。
// GormDataType returns the generic logical data type name.
func (ScriptText) GormDataType() string {
	return "script_text"
}

// GormDBDataType 按当前数据库方言返回真实列类型。
// GormDBDataType returns the concrete column type for the active dialect.
func (ScriptText) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db == nil || db.Dialector == nil {
		return "TEXT"
	}
	switch db.Dialector.Name() {
	case "mysql":
		return "MEDIUMTEXT"
	default:
		// postgres / sqlite / others：TEXT 即可承载长脚本
		// postgres / sqlite / others: TEXT is enough for long scripts
		return "TEXT"
	}
}

// Value 实现 driver.Valuer，写入数据库。
// Value implements driver.Valuer for database writes.
func (s ScriptText) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan 实现 sql.Scanner，从数据库读出。
// Scan implements sql.Scanner for database reads.
func (s *ScriptText) Scan(value interface{}) error {
	if s == nil {
		return fmt.Errorf("ScriptText: Scan on nil receiver")
	}
	switch v := value.(type) {
	case nil:
		*s = ""
		return nil
	case string:
		*s = ScriptText(v)
		return nil
	case []byte:
		*s = ScriptText(v)
		return nil
	default:
		return fmt.Errorf("ScriptText: unsupported Scan type %T", value)
	}
}

// MarshalJSON 将 ScriptText 序列化为 JSON 字符串。
// MarshalJSON serializes ScriptText as a JSON string.
func (s ScriptText) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// UnmarshalJSON 从 JSON 字符串反序列化。
// UnmarshalJSON deserializes from a JSON string.
func (s *ScriptText) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = ScriptText(str)
	return nil
}
