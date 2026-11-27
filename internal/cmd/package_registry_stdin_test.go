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

package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveRegistryPasswordStdin 覆盖 stdin 读取、组合冲突、create 非 TTY 报错以及 update 留空保留原密码。
// TestResolveRegistryPasswordStdin covers stdin reading, flag-combination conflict,
// non-TTY error on create, and empty-means-keep on update.
func TestResolveRegistryPasswordStdin(t *testing.T) {
	command := &cobra.Command{}

	fields := &containerRegistryFlags{passwordStdin: true}
	require.NoError(t, resolveRegistryPassword(command, fields, strings.NewReader("s3cr3t\n"), false))
	assert.Equal(t, "s3cr3t", fields.password)

	conflict := &containerRegistryFlags{password: "plain", passwordStdin: true}
	require.Error(t, resolveRegistryPassword(command, conflict, strings.NewReader("x"), false))

	// create 模式在非 TTY 下填了 username 必须显式给密码。
	// create mode in non-TTY with username requires --password-stdin.
	pipeCreate := &containerRegistryFlags{host: "reg.example.com", username: "u"}
	err := resolveRegistryPassword(command, pipeCreate, strings.NewReader(""), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "password-stdin")

	// update 模式允许不传密码以保留库内既有密码。
	// update mode permits omitting password so backend keeps stored credentials.
	pipeUpdate := &containerRegistryFlags{host: "reg.example.com", username: "u"}
	require.NoError(t, resolveRegistryPassword(command, pipeUpdate, strings.NewReader(""), true))
	assert.Empty(t, pipeUpdate.password)

	empty := &containerRegistryFlags{passwordStdin: true}
	require.Error(t, resolveRegistryPassword(command, empty, strings.NewReader(""), false))

	body := containerRegistryBody(&containerRegistryFlags{
		host:        "harbor.example.com",
		defaultRepo: "bigdata/seatunnel",
		username:    "robot",
		password:    "secret",
	})
	assert.Equal(t, "bigdata/seatunnel", body["default_repo"])
}
