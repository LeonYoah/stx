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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

var debugPprofNames = map[string]string{
	"heap":         "heap",
	"goroutine":    "goroutine",
	"allocs":       "allocs",
	"threadcreate": "threadcreate",
	"block":        "block",
	"mutex":        "mutex",
	"profile":      "profile",
	"cpu":          "profile",
	"trace":        "trace",
	"cmdline":      "cmdline",
	"symbol":       "symbol",
}

// newDebugCommand 注册 stx debug 组，目前只有 pprof 下载。
// newDebugCommand registers the stx debug group, currently only pprof download.
func newDebugCommand(storeProvider authStoreProvider) *cobra.Command {
	command := &cobra.Command{
		Use:   "debug",
		Short: "Server debug helpers (pprof)",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	command.AddCommand(newDebugPprofCommand(storeProvider))
	return command
}

// newDebugPprofCommand 拉取远端 STX 的 Go pprof 文件；cpu 映射到 profile。
// newDebugPprofCommand downloads a Go pprof file from remote STX; cpu maps to profile.
func newDebugPprofCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, outputPath string
	var seconds int
	command := &cobra.Command{
		Use:   "pprof <name>",
		Short: "Download a Go pprof profile from a remote STX server",
		Long:  "Fetch heap, goroutine, allocs, block, mutex, cpu (profile) or trace from GET /api/v1/debug/pprof/:name. Requires admin and app.pprof.enabled. Binary goes to --file; stdout stays JSON.",
		Example: "stx debug pprof heap --file heap.pprof\n" +
			"stx debug pprof cpu --seconds 30 --file cpu.pprof\n" +
			"stx debug pprof trace --file trace.out",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			requested := strings.ToLower(strings.TrimSpace(args[0]))
			name, ok := debugPprofNames[requested]
			if !ok {
				return clioutput.NewError(clioutput.CodeUsage, "unknown pprof profile "+requested, clioutput.ExitUsage, false)
			}
			if seconds <= 0 {
				seconds = defaultPprofSeconds(name)
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			if err := checkSpecialOperation(command, client, "debug.pprof.get"); err != nil {
				return err
			}
			if strings.TrimSpace(outputPath) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "download destination file is required", clioutput.ExitUsage, false)
			}
			finalPath, err := filepath.Abs(outputPath)
			if err != nil {
				return clioutput.WrapError(err, clioutput.CodeUsage, "resolve output path", clioutput.ExitUsage, false)
			}
			if _, err := os.Stat(finalPath); err == nil {
				return clioutput.NewError(clioutput.CodeConflict, "download target already exists", clioutput.ExitConflict, false)
			} else if !os.IsNotExist(err) {
				return clioutput.WrapError(err, clioutput.CodeFileTransfer, "inspect download target", clioutput.ExitFileTransfer, false)
			}
			if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
				return clioutput.WrapError(err, clioutput.CodeFileTransfer, "create download directory", clioutput.ExitFileTransfer, false)
			}
			tempFile, err := os.CreateTemp(filepath.Dir(finalPath), ".stx-pprof-*.part")
			if err != nil {
				return clioutput.WrapError(err, clioutput.CodeFileTransfer, "create temporary download file", clioutput.ExitFileTransfer, false)
			}
			tempPath := tempFile.Name()
			defer func() { _ = os.Remove(tempPath) }()

			path := "/api/v1/debug/pprof/" + url.PathEscape(name)
			if name == "profile" || name == "trace" {
				path += "?seconds=" + fmt.Sprintf("%d", seconds)
				client = client.WithTimeout(time.Duration(seconds+20) * time.Second)
			}
			requestID, err := client.Download(command.Context(), path, tempFile)
			if closeErr := tempFile.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
			if err := os.Rename(tempPath, finalPath); err != nil {
				return clioutput.WrapError(err, clioutput.CodeFileTransfer, "store downloaded pprof file", clioutput.ExitFileTransfer, false)
			}
			checksum, size, err := checksumFile(finalPath)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "debug.pprof.get", requestID, map[string]any{
				"profile": requested, "file": finalPath, "size": size, "sha256": checksum,
			})
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&outputPath, "file", "", "Destination file path")
	command.Flags().IntVar(&seconds, "seconds", 0, "CPU profile duration (default 30s) or execution-trace duration (default 5s)")
	return command
}

const (
	defaultCPUProfileSeconds = 30
	defaultTraceSeconds      = 5
)

// defaultPprofSeconds 给 CPU profile 默认 30 秒，给 trace 默认 5 秒（30 秒的 trace 又大又扰进程）。
// defaultPprofSeconds uses 30s for CPU profiles and 5s for traces (a 30s trace is large and disruptive).
func defaultPprofSeconds(name string) int {
	if name == "trace" {
		return defaultTraceSeconds
	}
	return defaultCPUProfileSeconds
}
