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
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestScriptTextGormDBDataType(t *testing.T) {
	t.Parallel()

	sqliteDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	got := ScriptText("").GormDBDataType(sqliteDB, &schema.Field{})
	if got != "TEXT" {
		t.Fatalf("sqlite ScriptText type = %q, want TEXT", got)
	}

	// MySQL dialector name path：用名称桩验证分流，不强制本机起 MySQL。
	// MySQL dialector-name path: stub by name without requiring a live MySQL.
	// Dialector 在 *Config 上；DB 通过嵌入 *Config 暴露该字段。
	// Dialector lives on *Config; DB exposes it via embedded *Config.
	mysqlLike := &gorm.DB{Config: &gorm.Config{Dialector: sqliteDialectorName("mysql")}}
	got = ScriptText("").GormDBDataType(mysqlLike, &schema.Field{})
	if got != "MEDIUMTEXT" {
		t.Fatalf("mysql ScriptText type = %q, want MEDIUMTEXT", got)
	}

	pgLike := &gorm.DB{Config: &gorm.Config{Dialector: sqliteDialectorName("postgres")}}
	got = ScriptText("").GormDBDataType(pgLike, &schema.Field{})
	if got != "TEXT" {
		t.Fatalf("postgres ScriptText type = %q, want TEXT", got)
	}
}

func TestScriptTextJSONRoundTrip(t *testing.T) {
	t.Parallel()
	const raw = `"env { parallelism = 1 }"`
	var s ScriptText
	if err := s.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if s.String() != "env { parallelism = 1 }" {
		t.Fatalf("content = %q", s.String())
	}
	out, err := s.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(out) != raw {
		t.Fatalf("MarshalJSON = %s, want %s", out, raw)
	}
}

// sqliteDialectorName 复用 sqlite 实现，但覆盖 Name()，便于单测方言分支。
// sqliteDialectorName reuses the sqlite dialector but overrides Name() for dialect-branch unit tests.
type namedDialector struct {
	gorm.Dialector
	name string
}

func (d namedDialector) Name() string { return d.name }

func sqliteDialectorName(name string) gorm.Dialector {
	return namedDialector{Dialector: sqlite.Open("file::memory:"), name: name}
}
