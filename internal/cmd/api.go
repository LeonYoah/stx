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
	"context"
	"fmt"
	"log"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db/migrator"
	"github.com/LeonYoah/stx/internal/pkg/layoutheal"
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

	// 一键升级由旧版本升级器执行，它不替换 bin/：旧 start.sh 每次重启都会拉起外部监控三件套。
	// 所以新二进制启动时在检测到安装目录布局时先用内嵌脚本自修复 bin/，再在后台窗口内按
	// pid 文件停掉旧三件套（只停进程，不删 observability/ 数据）。非安装布局（开发机、Docker）
	// 会跳过全部动作。
	// The in-app upgrade is executed by the OLD upgrader, which never replaces bin/: the legacy
	// start.sh keeps launching the external monitoring trio on every restart. So on startup the
	// new binary self-heals bin/ from the embedded scripts when an install layout is detected,
	// then stops the trio by pid file in a background window (processes only, observability/
	// data is kept). Non-install layouts (dev machines, Docker) skip everything.
	healResult, healErr := layoutheal.Heal(context.Background(), layoutheal.Options{})
	if healErr != nil {
		log.Printf("[LayoutHeal] WARN 安装布局自修复失败 / install layout heal failed: %v\n", healErr)
	}
	if healResult.InstallDir != "" {
		go layoutheal.ReapLegacyObservability(context.Background(), healResult.InstallDir,
			layoutheal.DefaultReapWindow, layoutheal.DefaultReapInterval)
	}

	router.Serve()
	return nil
}
