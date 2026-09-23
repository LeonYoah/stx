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
	"fmt"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db/migrator"
	"github.com/LeonYoah/stx/internal/router"
	"github.com/spf13/cobra"
)

func newServerCommand(serverRunner func() error) *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Start the STX API server",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(_ *cobra.Command, _ []string) error {
			return serverRunner()
		},
	}
}

func newAPICompatibilityCommand(serverRunner func() error) *cobra.Command {
	return &cobra.Command{
		Use:        "api",
		Short:      "Start the STX API server",
		Args:       usageArgs(cobra.NoArgs),
		Hidden:     true,
		Deprecated: "use \"stx server\" instead",
		RunE: func(_ *cobra.Command, _ []string) error {
			return serverRunner()
		},
	}
}

// runServer 只在 server 或兼容 api 命令执行时加载服务端配置、迁移数据库并启动服务。
// runServer loads server configuration, migrates the database, and starts services only for server commands.
func runServer() error {
	if err := config.ServerConfigError(); err != nil {
		return fmt.Errorf("load server configuration: %w", err)
	}
	migrator.Migrate()
	router.Serve()
	return nil
}
