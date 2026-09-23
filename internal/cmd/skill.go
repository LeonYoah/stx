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
	"errors"
	"io"
	"os"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	cliskill "github.com/LeonYoah/stx/internal/cli/skill"
	"github.com/spf13/cobra"
)

type skillCommandOptions struct {
	homeProvider func() (string, error)
	getenv       func(string) string
}

type skillStatusResult struct {
	Language cliskill.Language       `json:"language" yaml:"language"`
	Targets  []cliskill.TargetStatus `json:"targets" yaml:"targets"`
}

type skillChangeResult struct {
	Language cliskill.Language `json:"language,omitempty" yaml:"language,omitempty"`
	Changes  []cliskill.Change `json:"changes" yaml:"changes"`
}

// newSkillCommand 创建本地 Skill 查看、安装、更新、备份和恢复命令。
// newSkillCommand creates local Skill show, install, update, backup, and restore commands.
func newSkillCommand() *cobra.Command {
	return newSkillCommandWithOptions(skillCommandOptions{homeProvider: os.UserHomeDir, getenv: os.Getenv})
}

func newSkillCommandWithOptions(options skillCommandOptions) *cobra.Command {
	command := &cobra.Command{
		Use:   "skill",
		Short: "Manage the bundled STX AI agent Skill",
		Args:  usageArgs(cobra.NoArgs),
	}
	command.AddCommand(
		newSkillShowCommand(options),
		newSkillStatusCommand(options),
		newSkillInstallCommand(options),
		newSkillUpdateCommand(options),
		newSkillBackupCommand(options),
		newSkillRestoreCommand(options),
	)
	return command
}

func newSkillShowCommand(options skillCommandOptions) *cobra.Command {
	var languageValue string
	command := &cobra.Command{
		Use:   "show",
		Short: "Print the bundled STX Skill for the selected language",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			language, err := cliskill.ResolveLanguage(languageValue, options.getenv)
			if err != nil {
				return classifySkillError(err)
			}
			content, err := cliskill.Source(language)
			if err != nil {
				return classifySkillError(err)
			}
			_, err = command.OutOrStdout().Write(content)
			return err
		},
	}
	command.Flags().StringVar(&languageValue, "language", string(cliskill.LanguageAuto), "Skill language: auto, zh-CN, or en")
	return command
}

func newSkillStatusCommand(options skillCommandOptions) *cobra.Command {
	var targetValue, languageValue string
	command := &cobra.Command{
		Use:   "status",
		Short: "Show STX Skill installation and backup status",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			manager, language, targets, err := prepareSkillCommand(options, languageValue, targetValue)
			if err != nil {
				return err
			}
			statuses, err := manager.Status(language, targets)
			if err != nil {
				return classifySkillError(err)
			}
			return renderCommandResult(command, "skill.status", skillStatusResult{Language: language, Targets: statuses})
		},
	}
	addSkillSelectionFlags(command, &targetValue, &languageValue)
	return command
}

func newSkillInstallCommand(options skillCommandOptions) *cobra.Command {
	var targetValue, languageValue string
	command := &cobra.Command{
		Use:   "install",
		Short: "Install missing STX Skill files without overwriting existing files",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			manager, language, targets, err := prepareSkillCommand(options, languageValue, targetValue)
			if err != nil {
				return err
			}
			changes, err := manager.Install(language, targets)
			if err != nil {
				return classifySkillError(err)
			}
			return renderCommandResult(command, "skill.install", skillChangeResult{Language: language, Changes: changes})
		},
	}
	addSkillSelectionFlags(command, &targetValue, &languageValue)
	return command
}

func newSkillUpdateCommand(options skillCommandOptions) *cobra.Command {
	var targetValue, languageValue string
	command := &cobra.Command{
		Use:   "update",
		Short: "Update STX Skill files and back up changed content",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			manager, language, targets, err := prepareSkillCommand(options, languageValue, targetValue)
			if err != nil {
				return err
			}
			changes, err := manager.Update(language, targets)
			if err != nil {
				return classifySkillError(err)
			}
			return renderCommandResult(command, "skill.update", skillChangeResult{Language: language, Changes: changes})
		},
	}
	addSkillSelectionFlags(command, &targetValue, &languageValue)
	return command
}

func newSkillBackupCommand(options skillCommandOptions) *cobra.Command {
	var targetValue string
	command := &cobra.Command{
		Use:   "backup",
		Short: "Back up installed STX Skill files",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			manager, targets, err := prepareSkillTargets(options, targetValue)
			if err != nil {
				return err
			}
			changes, err := manager.Backup(targets)
			if err != nil {
				return classifySkillError(err)
			}
			return renderCommandResult(command, "skill.backup", skillChangeResult{Changes: changes})
		},
	}
	command.Flags().StringVar(&targetValue, "target", "all", "Installation target: all, claude, or agents")
	return command
}

func newSkillRestoreCommand(options skillCommandOptions) *cobra.Command {
	var targetValue, backupID string
	command := &cobra.Command{
		Use:   "restore",
		Short: "Restore STX Skill files from a backup",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			manager, targets, err := prepareSkillTargets(options, targetValue)
			if err != nil {
				return err
			}
			changes, err := manager.Restore(targets, backupID)
			if err != nil {
				return classifySkillError(err)
			}
			return renderCommandResult(command, "skill.restore", skillChangeResult{Changes: changes})
		},
	}
	command.Flags().StringVar(&targetValue, "target", "all", "Installation target: all, claude, or agents")
	command.Flags().StringVar(&backupID, "backup", "", "Backup ID to restore; defaults to the latest backup for each target")
	return command
}

func addSkillSelectionFlags(command *cobra.Command, targetValue, languageValue *string) {
	command.Flags().StringVar(targetValue, "target", "all", "Installation target: all, claude, or agents")
	command.Flags().StringVar(languageValue, "language", string(cliskill.LanguageAuto), "Skill language: auto, zh-CN, or en")
}

func prepareSkillCommand(options skillCommandOptions, languageValue, targetValue string) (*cliskill.Manager, cliskill.Language, []cliskill.Target, error) {
	manager, targets, err := prepareSkillTargets(options, targetValue)
	if err != nil {
		return nil, "", nil, err
	}
	language, err := cliskill.ResolveLanguage(languageValue, options.getenv)
	if err != nil {
		return nil, "", nil, classifySkillError(err)
	}
	return manager, language, targets, nil
}

func prepareSkillTargets(options skillCommandOptions, targetValue string) (*cliskill.Manager, []cliskill.Target, error) {
	if options.homeProvider == nil {
		options.homeProvider = os.UserHomeDir
	}
	home, err := options.homeProvider()
	if err != nil {
		return nil, nil, classifySkillError(err)
	}
	targets, err := cliskill.ParseTargets(targetValue)
	if err != nil {
		return nil, nil, classifySkillError(err)
	}
	return cliskill.NewManager(home), targets, nil
}

func classifySkillError(err error) error {
	switch {
	case errors.Is(err, cliskill.ErrInvalidLanguage), errors.Is(err, cliskill.ErrInvalidTarget):
		return clioutput.WrapError(err, clioutput.CodeUsage, err.Error(), clioutput.ExitUsage, false)
	case errors.Is(err, cliskill.ErrBackupNotFound), errors.Is(err, os.ErrNotExist):
		return clioutput.WrapError(err, clioutput.CodeNotFound, err.Error(), clioutput.ExitNotFound, false)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return clioutput.WrapError(err, clioutput.CodeFileTransfer, err.Error(), clioutput.ExitFileTransfer, true)
	default:
		return clioutput.WrapError(err, clioutput.CodeFileTransfer, err.Error(), clioutput.ExitFileTransfer, false)
	}
}
