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
	"reflect"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UTCPlugin 是全局时区统一的兜底层：在写入前把模型/Map 中的 time.Time 统一为 UTC，
// 并在执行前把 WHERE 条件里的 time.Time 参数统一为 UTC。这样 SQLite（DATETIME 以文本比较）
// 与 MySQL/PostgreSQL 的存储与比较语义一致，不再依赖进程 time.Local。
// UTCPlugin is the safety net of the unified timezone design: it normalizes time.Time values in models/maps to UTC
// before writes and normalizes time.Time WHERE arguments to UTC before execution, so SQLite (text-compared DATETIME)
// and MySQL/PostgreSQL behave identically regardless of the process time.Local.
type UTCPlugin struct{}

// Name implements gorm.Plugin.
func (UTCPlugin) Name() string { return "stx:utc" }

// Initialize implements gorm.Plugin.
func (UTCPlugin) Initialize(database *gorm.DB) error {
	cb := database.Callback()
	if err := cb.Create().Before("gorm:create").Register("stx:utc_create", normalizeDestCallback); err != nil {
		return err
	}
	if err := cb.Update().Before("gorm:update").Register("stx:utc_update", func(tx *gorm.DB) {
		normalizeDestCallback(tx)
		normalizeWhereCallback(tx)
	}); err != nil {
		return err
	}
	if err := cb.Query().Before("gorm:query").Register("stx:utc_query", normalizeWhereCallback); err != nil {
		return err
	}
	if err := cb.Delete().Before("gorm:delete").Register("stx:utc_delete", normalizeWhereCallback); err != nil {
		return err
	}
	return cb.Row().Before("gorm:row").Register("stx:utc_row", normalizeWhereCallback)
}

var timeType = reflect.TypeOf(time.Time{})

func normalizeDestCallback(tx *gorm.DB) {
	if tx.Statement == nil {
		return
	}
	normalizeValue(reflect.ValueOf(tx.Statement.Dest), 0)
	if tx.Statement.ReflectValue.IsValid() {
		normalizeValue(tx.Statement.ReflectValue, 0)
	}
}

// normalizeValue 递归把可寻址的 time.Time 字段转换为 UTC（深度受限，避免环）。
// normalizeValue recursively converts addressable time.Time values to UTC (depth-limited).
func normalizeValue(v reflect.Value, depth int) {
	if depth > 4 || !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if !v.IsNil() {
			normalizeValue(v.Elem(), depth+1)
		}
	case reflect.Struct:
		if v.Type() == timeType {
			if v.CanSet() {
				t := v.Interface().(time.Time)
				if !t.IsZero() && t.Location() != time.UTC {
					v.Set(reflect.ValueOf(t.UTC()))
				}
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				normalizeValue(v.Field(i), depth+1)
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		for i := 0; i < v.Len(); i++ {
			normalizeValue(v.Index(i), depth+1)
		}
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return
		}
		for _, key := range v.MapKeys() {
			if converted, ok := toUTC(v.MapIndex(key).Interface()); ok {
				v.SetMapIndex(key, reflect.ValueOf(converted))
			}
		}
	}
}

func toUTC(value interface{}) (interface{}, bool) {
	switch t := value.(type) {
	case time.Time:
		return t.UTC(), true
	case *time.Time:
		if t == nil {
			return value, false
		}
		u := t.UTC()
		return &u, true
	}
	return value, false
}

func normalizeWhereCallback(tx *gorm.DB) {
	if tx.Statement == nil {
		return
	}
	c, ok := tx.Statement.Clauses["WHERE"]
	if !ok {
		return
	}
	where, ok := c.Expression.(clause.Where)
	if !ok {
		return
	}
	where.Exprs = normalizeExprs(where.Exprs)
	c.Expression = where
	tx.Statement.Clauses["WHERE"] = c
}

func normalizeVars(vars []interface{}) []interface{} {
	for i, v := range vars {
		if converted, ok := toUTC(v); ok {
			vars[i] = converted
		}
	}
	return vars
}

func normalizeExprs(exprs []clause.Expression) []clause.Expression {
	for i, e := range exprs {
		exprs[i] = normalizeExpr(e)
	}
	return exprs
}

func normalizeExpr(e clause.Expression) clause.Expression {
	switch x := e.(type) {
	case clause.Expr:
		x.Vars = normalizeVars(x.Vars)
		return x
	case clause.NamedExpr:
		x.Vars = normalizeVars(x.Vars)
		return x
	case clause.AndConditions:
		x.Exprs = normalizeExprs(x.Exprs)
		return x
	case clause.OrConditions:
		x.Exprs = normalizeExprs(x.Exprs)
		return x
	case clause.NotConditions:
		x.Exprs = normalizeExprs(x.Exprs)
		return x
	case clause.Eq:
		x.Value, _ = toUTC(x.Value)
		return x
	case clause.Neq:
		x.Value, _ = toUTC(x.Value)
		return x
	case clause.Gt:
		x.Value, _ = toUTC(x.Value)
		return x
	case clause.Gte:
		x.Value, _ = toUTC(x.Value)
		return x
	case clause.Lt:
		x.Value, _ = toUTC(x.Value)
		return x
	case clause.Lte:
		x.Value, _ = toUTC(x.Value)
		return x
	}
	return e
}
