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
	"fmt"

	clicommand "github.com/LeonYoah/stx/internal/cli/command"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

// newGeneratedCommands 从登记表构建普通 API 命令，登记错误属于启动前即可发现的编程错误。
// newGeneratedCommands builds regular API commands from the registry; registry errors are programming errors found before execution.
func newGeneratedCommands(storeProvider authStoreProvider) []*cobra.Command {
	specs := make([]operation.OperationSpec, 0)
	for _, spec := range operation.Registry() {
		if spec.GeneratedCLI {
			specs = append(specs, spec)
		}
	}
	commands, err := clicommand.Build(specs, func(namespace string) (clicommand.Client, error) {
		return clientForNamespace(storeProvider, namespace)
	})
	if err != nil {
		panic(fmt.Sprintf("build generated CLI commands: %v", err))
	}
	return commands
}

func defaultGeneratedCommands() []*cobra.Command {
	return newGeneratedCommands(cliConfig.NewDefaultStore)
}
