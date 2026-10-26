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
	"strconv"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

// newSyncEventChannelUpdateCommand 开启、关闭或轮换集群的 SeaTunnel 事件上报通道（R2：改写 seatunnel.yaml，需滚动重启生效）。
// newSyncEventChannelUpdateCommand enables, disables or rotates a cluster's SeaTunnel event report channel (R2: it
// rewrites seatunnel.yaml and needs a rolling restart to take effect).
func newSyncEventChannelUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var enabled string
	var rotateToken bool

	command := &cobra.Command{
		Use:     "update <cluster_id>",
		Short:   "Enable, disable or rotate the SeaTunnel event report channel of a cluster",
		Example: "stx sync event-channel update 1 --enabled=true --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			clusterID, err := strconv.ParseUint(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || clusterID == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "cluster_id must be a positive integer", clioutput.ExitUsage, false)
			}
			body := map[string]any{"rotate_token": rotateToken}
			switch strings.ToLower(strings.TrimSpace(enabled)) {
			case "":
			case "true", "1", "yes":
				body["enabled"] = true
			case "false", "0", "no":
				body["enabled"] = false
			default:
				return clioutput.NewError(clioutput.CodeUsage, "--enabled must be true or false", clioutput.ExitUsage, false)
			}
			if _, ok := body["enabled"]; !ok && !rotateToken {
				return clioutput.NewError(clioutput.CodeUsage, "specify --enabled or --rotate-token", clioutput.ExitUsage, false)
			}
			operationID := "sync.event-channel.update"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "会修改该集群全部节点的 seatunnel.yaml（event-report-http），需滚动重启集群后生效。")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, fmt.Sprintf("/api/v1/sync/event-channels/%d", clusterID), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&enabled, "enabled", "", "Enable (true) or disable (false) event reporting")
	command.Flags().BoolVar(&rotateToken, "rotate-token", false, "Rotate the per-cluster event token (requires a cluster restart)")
	return command
}
