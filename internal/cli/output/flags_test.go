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
package output

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestOptionsFromCommandSupportsAliases(t *testing.T) {
	for _, args := range [][]string{{"--output", "yaml"}, {"--format", "yaml"}, {"-f", "yaml"}} {
		command := &cobra.Command{Use: "test", RunE: func(command *cobra.Command, _ []string) error {
			options, err := OptionsFromCommand(command)
			if err != nil {
				return err
			}
			if options.Format != FormatYAML {
				t.Fatalf("别名解析错误 / alias parsing failed: args=%v options=%#v", args, options)
			}
			return nil
		}}
		AddGlobalFlags(command)
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("执行参数 %v 失败 / executing arguments %v failed: %v", args, args, err)
		}
	}
}

func TestOptionsFromCommandRejectsConflictingAliases(t *testing.T) {
	command := &cobra.Command{Use: "test", RunE: func(command *cobra.Command, _ []string) error {
		_, err := OptionsFromCommand(command)
		return err
	}}
	AddGlobalFlags(command)
	command.SetArgs([]string{"--output", "json", "--format", "yaml"})
	err := command.Execute()
	classified := ClassifyError(err)
	if classified.ExitCode != ExitUsage {
		t.Fatalf("冲突别名应返回用法错误 / conflicting aliases must return a usage error: %#v", classified)
	}
}
