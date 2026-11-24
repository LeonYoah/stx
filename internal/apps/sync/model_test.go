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

package sync

import (
	"testing"
)

func TestTaskAccessLevelResolution(t *testing.T) {
	cases := []struct {
		name       string
		definition JSONMap
		want       TaskAccessLevel
	}{
		{name: "nil definition defaults to public readonly", definition: nil, want: TaskAccessLevelPublicReadOnly},
		{name: "empty definition defaults to public readonly", definition: JSONMap{}, want: TaskAccessLevelPublicReadOnly},
		{name: "explicit private", definition: JSONMap{"access_level": "private"}, want: TaskAccessLevelPrivate},
		{name: "explicit public readonly", definition: JSONMap{"access_level": "public_readonly"}, want: TaskAccessLevelPublicReadOnly},
		{name: "explicit public editable", definition: JSONMap{"access_level": "public_editable"}, want: TaskAccessLevelPublicEditable},
		{name: "access level is normalized case-insensitively", definition: JSONMap{"access_level": " PUBLIC_EDITABLE "}, want: TaskAccessLevelPublicEditable},
		{name: "access level wins over contradictory is_public", definition: JSONMap{"access_level": "private", "is_public": true}, want: TaskAccessLevelPrivate},
		{name: "legacy is_public false maps to private", definition: JSONMap{"is_public": false}, want: TaskAccessLevelPrivate},
		{name: "legacy is_public string false maps to private", definition: JSONMap{"is_public": "false"}, want: TaskAccessLevelPrivate},
		{name: "legacy is_public true maps to public readonly", definition: JSONMap{"is_public": true}, want: TaskAccessLevelPublicReadOnly},
		{name: "legacy is_public other truthy value maps to public readonly", definition: JSONMap{"is_public": "yes"}, want: TaskAccessLevelPublicReadOnly},
		{name: "dirty access level falls back to is_public", definition: JSONMap{"access_level": "garbage", "is_public": false}, want: TaskAccessLevelPrivate},
		{name: "non-string access level falls back to is_public", definition: JSONMap{"access_level": 42, "is_public": false}, want: TaskAccessLevelPrivate},
		{name: "empty access level falls back to missing is_public default", definition: JSONMap{"access_level": "  "}, want: TaskAccessLevelPublicReadOnly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := &Task{Definition: tc.definition}
			if got := task.AccessLevel(); got != tc.want {
				t.Fatalf("AccessLevel() = %q, want %q", got, tc.want)
			}
			if got := task.IsPublic(); got != (tc.want != TaskAccessLevelPrivate) {
				t.Fatalf("IsPublic() = %v inconsistent with access level %q", got, tc.want)
			}
		})
	}
}

func TestTaskCanUserEditPublicEditable(t *testing.T) {
	editable := &Task{CreatedBy: 1, Definition: JSONMap{"access_level": "public_editable"}}
	readonly := &Task{CreatedBy: 1, Definition: JSONMap{"access_level": "public_readonly"}}
	private := &Task{CreatedBy: 1, Definition: JSONMap{"access_level": "private", "collaborator_ids": []interface{}{float64(4)}}}

	if !editable.CanUserEdit(9, false) {
		t.Fatalf("public_editable 任务应允许普通用户编辑")
	}
	if readonly.CanUserEdit(9, false) {
		t.Fatalf("public_readonly 任务不应允许普通用户编辑")
	}
	if private.CanUserEdit(9, false) {
		t.Fatalf("private 任务不应允许非共建者编辑")
	}
	if !private.CanUserEdit(4, false) {
		t.Fatalf("private 任务应允许共建者编辑")
	}
	if !readonly.CanUserEdit(9, true) {
		t.Fatalf("管理员始终可以编辑")
	}
	if !readonly.CanUserEdit(1, false) {
		t.Fatalf("所有者始终可以编辑")
	}
	if readonly.CanUserRun(9, false) {
		t.Fatalf("public_readonly 任务不应允许普通用户运行")
	}
	if !editable.CanUserRun(9, false) {
		t.Fatalf("public_editable 任务应允许普通用户运行")
	}
}

func TestNormalizeTaskAccessLevel(t *testing.T) {
	for _, raw := range []string{"private", " public_readonly ", "PUBLIC_EDITABLE"} {
		if _, err := normalizeTaskAccessLevel(raw); err != nil {
			t.Fatalf("normalizeTaskAccessLevel(%q) unexpected error: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "public", "read_only", "editable", "private readonly"} {
		if _, err := normalizeTaskAccessLevel(raw); err != ErrInvalidTaskAccessLevel {
			t.Fatalf("normalizeTaskAccessLevel(%q) = %v, want ErrInvalidTaskAccessLevel", raw, err)
		}
	}
}
