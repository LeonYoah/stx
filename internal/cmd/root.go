/*
 * MIT License
 *
 * Copyright (c) 2025 linux.do
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package cmd

import (
	"io"
	"os"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

// NewRootCommand 创建 STX 根命令，且不会在构造或显示帮助时初始化数据库。
// NewRootCommand creates the STX root command without initializing the database during construction or help rendering.
func NewRootCommand() *cobra.Command {
	return newRootCommand(runServer)
}

func newRootCommand(serverRunner func() error) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "stx",
		Short:         "STX server and remote command line client",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clioutput.WrapError(err, clioutput.CodeUsage, err.Error(), clioutput.ExitUsage, false)
	})
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	clioutput.AddGlobalFlags(rootCmd)
	rootCmd.AddCommand(
		newServerCommand(serverRunner),
		newAPICompatibilityCommand(serverRunner),
		newNamespaceCommand(),
		newCapabilityCommand(),
		newHealthCommand(),
		newExecutionCommand(),
	)
	rootCmd.AddCommand(newAuthCommands()...)
	rootCmd.AddCommand(defaultGeneratedCommands()...)
	addPackageWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addUserWriteCommands(rootCmd, defaultUserWriteCommandOptions())
	addClusterWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addPluginWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addConfigWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addMonitorWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addMonitoringWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addHostWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addHostInstallCommands(rootCmd, cliConfig.NewDefaultStore)
	addUpgradeCommands(rootCmd, cliConfig.NewDefaultStore)
	addDiagnosticsWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addSyncWriteCommands(rootCmd, cliConfig.NewDefaultStore)
	addRuntimeStorageCommands(rootCmd, cliConfig.NewDefaultStore)
	return rootCmd
}

// Execute 运行 STX 根命令。
// Execute runs the STX root command.
func Execute() {
	if exitCode := ExecuteCommand(os.Args[1:], os.Stdout, os.Stderr); exitCode != int(clioutput.ExitSuccess) {
		os.Exit(exitCode)
	}
}

// ExecuteCommand 执行参数并返回稳定退出码，便于真实进程和测试共用。
// ExecuteCommand executes arguments and returns a stable exit code for both real processes and tests.
func ExecuteCommand(args []string, stdout, stderr io.Writer) int {
	return executeCommand(newRootCommand(runServer), args, stdout, stderr)
}

func executeCommand(rootCommand *cobra.Command, args []string, stdout, stderr io.Writer) int {
	rootCommand.SetArgs(args)
	rootCommand.SetOut(stdout)
	rootCommand.SetErr(stderr)
	if _, _, err := rootCommand.Find(args); err != nil {
		classified := clioutput.WrapError(err, clioutput.CodeUsage, err.Error(), clioutput.ExitUsage, false)
		_ = clioutput.NewEventWriter(stderr).EmitError(classified)
		return int(classified.ExitCode)
	}
	if err := rootCommand.Execute(); err != nil {
		classified := clioutput.ClassifyError(err)
		_ = clioutput.NewEventWriter(stderr).EmitError(classified)
		return int(classified.ExitCode)
	}
	return int(clioutput.ExitSuccess)
}

func usageArgs(validator cobra.PositionalArgs) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := validator(command, args); err != nil {
			return clioutput.WrapError(err, clioutput.CodeUsage, err.Error(), clioutput.ExitUsage, false)
		}
		return nil
	}
}
