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
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

type packageWriteOptions = secureWriteOptions

// addPackageWriteCommands 将文件和 JSON 写命令挂到生成的 package 命令树。
// addPackageWriteCommands attaches file and JSON write commands to the generated package tree.
func addPackageWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	packageCommand := childCommand(root, "package")
	if packageCommand == nil {
		panic("generated package command is missing")
	}
	uploadCommand := newPackageUploadCommand(storeProvider)
	uploadCommand.AddCommand(newPackageChunkUploadCommand(storeProvider))
	packageCommand.AddCommand(uploadCommand, newPackageDeleteCommand(storeProvider))
	downloadCommand := childCommand(packageCommand, "download")
	if downloadCommand == nil {
		panic("generated package download command is missing")
	}
	downloadCommand.AddCommand(newPackageDownloadStartCommand(storeProvider), newPackageDownloadCancelCommand(storeProvider))
}

func childCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, command := range parent.Commands() {
		if command.Name() == name {
			return command
		}
	}
	return nil
}

func newPackageUploadCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var version string
	command := &cobra.Command{
		Use:     "upload <file>",
		Short:   "Upload a SeaTunnel package",
		Long:    "Upload a SeaTunnel package archive to STX local storage. The operation uses network bandwidth and disk I/O.",
		Example: "stx package upload ./apache-seatunnel-9.9.91-bin.tar.gz --version 9.9.91 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			file, info, err := openPackageFile(args[0])
			if err != nil {
				return err
			}
			defer file.Close()
			if strings.TrimSpace(version) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--version is required", clioutput.ExitUsage, false)
			}
			client, headers, err := preparePackageWrite(command, storeProvider, "package.upload", &options,
				"upload writes a package to STX local storage")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestMultipart(command.Context(), http.MethodPost, "/api/v1/packages/upload",
				map[string]string{"version": version}, "file", info.Name(), file, headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.upload", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version stored by STX")
	return command
}

func newPackageChunkUploadCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var version, uploadID, originalName string
	var chunkIndex, totalChunks int
	var totalSize int64
	command := &cobra.Command{
		Use:     "chunk <file>",
		Short:   "Upload one SeaTunnel package chunk",
		Example: "stx package upload chunk ./part-000 --version 9.9.92 --upload-id upload_12345678 --chunk-index 0 --total-chunks 2 --total-size 1024 --file-name apache-seatunnel-9.9.92-bin.tar.gz --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(version) == "" || strings.TrimSpace(uploadID) == "" || totalChunks <= 0 || totalSize <= 0 || chunkIndex < 0 || chunkIndex >= totalChunks {
				return clioutput.NewError(clioutput.CodeUsage, "valid --version, --upload-id, --chunk-index, --total-chunks and --total-size are required", clioutput.ExitUsage, false)
			}
			file, info, err := openPackageFile(args[0])
			if err != nil {
				return err
			}
			defer file.Close()
			if strings.TrimSpace(originalName) == "" {
				originalName = fmt.Sprintf("apache-seatunnel-%s-bin.tar.gz", version)
			}
			client, headers, err := preparePackageWrite(command, storeProvider, "package.upload.chunk", &options,
				"chunk upload writes temporary package data to STX")
			if err != nil {
				return err
			}
			// 每个分片使用独立且可重复的幂等键，并保留用户提供的会话前缀。
			// Each chunk needs its own deterministic retry key while keeping the user-supplied key as the session prefix.
			headers["Idempotency-Key"] = fmt.Sprintf("%s-chunk-%d", headers["Idempotency-Key"], chunkIndex)
			fields := map[string]string{
				"version": version, "upload_id": uploadID, "chunk_index": strconv.Itoa(chunkIndex),
				"total_chunks": strconv.Itoa(totalChunks), "total_size": strconv.FormatInt(totalSize, 10), "file_name": originalName,
			}
			var data any
			requestID, err := client.RequestMultipart(command.Context(), http.MethodPost, "/api/v1/packages/upload/chunk",
				fields, "file", info.Name(), file, headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.upload.chunk", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version stored by STX")
	command.Flags().StringVar(&uploadID, "upload-id", "", "Stable upload session ID")
	command.Flags().IntVar(&chunkIndex, "chunk-index", -1, "Zero-based chunk index")
	command.Flags().IntVar(&totalChunks, "total-chunks", 0, "Total number of chunks")
	command.Flags().Int64Var(&totalSize, "total-size", 0, "Original package size in bytes")
	command.Flags().StringVar(&originalName, "file-name", "", "Original .tar.gz package file name")
	return command
}

func newPackageDeleteCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	command := &cobra.Command{
		Use:     "delete <version>",
		Short:   "Delete a local SeaTunnel package",
		Long:    "Permanently delete a local package. The file cannot be recovered by STX.",
		Example: "stx package delete 9.9.91 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.delete", &options,
				"delete permanently removes the local package file")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodDelete,
				"/api/v1/packages/"+url.PathEscape(args[0]), nil, headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.delete", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	return command
}

func newPackageDownloadStartCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var mirror string
	command := &cobra.Command{
		Use:     "start <version>",
		Short:   "Start a SeaTunnel package download",
		Long:    "Start a server-side package download. It uses STX server network bandwidth and disk I/O.",
		Example: "stx package download start 2.3.13 --mirror apache --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			if mirror != "" && mirror != "apache" && mirror != "aliyun" && mirror != "huaweicloud" {
				return clioutput.NewError(clioutput.CodeUsage, "--mirror must be apache, aliyun, or huaweicloud", clioutput.ExitUsage, false)
			}
			client, headers, err := preparePackageWrite(command, storeProvider, "package.download.start", &options,
				"download uses STX server network bandwidth and disk space")
			if err != nil {
				return err
			}
			var data map[string]any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/packages/download",
				map[string]string{"version": args[0], "mirror": mirror}, headers, &data)
			if err != nil {
				return err
			}
			next := fmt.Sprintf("stx package download get %s", args[0])
			if executionID, _ := data["execution_id"].(string); executionID != "" {
				next = "stx execution wait " + executionID
			}
			return renderPackageResult(command, "package.download.start", requestID, data, next)
		},
	}
	addPackageWriteFlags(command, &options)
	command.Flags().StringVar(&mirror, "mirror", "aliyun", "Download mirror: apache, aliyun, or huaweicloud")
	return command
}

func newPackageDownloadCancelCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	command := &cobra.Command{
		Use:     "cancel <version>",
		Short:   "Cancel a SeaTunnel package download",
		Long:    "Stop the active server-side download and remove its partial file.",
		Example: "stx package download cancel 2.3.13 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.download.cancel", &options,
				"cancel stops the active download and removes its partial file")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost,
				"/api/v1/packages/download/"+url.PathEscape(args[0])+"/cancel", nil, headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.download.cancel", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	return command
}

func addPackageWriteFlags(command *cobra.Command, options *packageWriteOptions) {
	addSecureWriteFlags(command, options)
}

func preparePackageWrite(command *cobra.Command, storeProvider authStoreProvider, operationID string, options *packageWriteOptions, impact string) (*cliClient.Client, map[string]string, error) {
	return prepareSecureWrite(command, storeProvider, operationID, options, impact)
}

func openPackageFile(path string) (*os.File, os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, clioutput.WrapError(err, clioutput.CodeFileTransfer, "cannot open upload file", clioutput.ExitFileTransfer, false)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, clioutput.WrapError(err, clioutput.CodeFileTransfer, "cannot inspect upload file", clioutput.ExitFileTransfer, false)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		_ = file.Close()
		return nil, nil, clioutput.NewError(clioutput.CodeFileTransfer, "upload path must be a non-empty regular file", clioutput.ExitFileTransfer, false)
	}
	return file, info, nil
}

func renderPackageResult(command *cobra.Command, operationID, requestID string, data any, nextCommand string) error {
	return renderWriteResult(command, operationID, requestID, data, nextCommand)
}
