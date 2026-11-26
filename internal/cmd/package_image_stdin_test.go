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

// TestResolveRegistryPasswordStdin stdin 读取、组合冲突、非 TTY 显式错误三条路径。
// TestResolveRegistryPasswordStdin covers stdin reading, flag-combination conflict,
// and the explicit non-TTY error.
func TestResolveRegistryPasswordStdin(t *testing.T) {
	command := &cobra.Command{}

	// stdin 路径：密码正确读入字段。
	// stdin path: the password lands in the field.
	fields := &containerImageFlags{registryPasswordStdin: true}
	require.NoError(t, resolveRegistryPassword(command, fields, strings.NewReader("s3cr3t\n")))
	assert.Equal(t, "s3cr3t", fields.registryPassword)

	// 组合冲突：--registry-password 与 --registry-password-stdin 互斥。
	// combination conflict: the two flags are mutually exclusive.
	conflict := &containerImageFlags{registryPassword: "plain", registryPasswordStdin: true}
	require.Error(t, resolveRegistryPassword(command, conflict, strings.NewReader("x")))

	// 非 TTY 无输入：凭据区填写时必须显式报错。
	// non-TTY without input: credential fields require an explicit error.
	pipe := &containerImageFlags{registryHost: "reg.example.com", registryUsername: "u"}
	err := resolveRegistryPassword(command, pipe, strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry-password-stdin")

	// 空 stdin 明确报错。
	// empty stdin errors explicitly.
	empty := &containerImageFlags{registryPasswordStdin: true}
	require.Error(t, resolveRegistryPassword(command, empty, strings.NewReader("")))
}
