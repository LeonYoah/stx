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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

const (
	jobGuardCommandName   = "__job-guard"
	guardGenerationPrefix = "generation "
	guardDoneLine         = "done"
)

// jobGuardRetryLimit 是管道 EOF 且没有 done 时按 ref 停止的重试上限，测试可改短。
// jobGuardRetryLimit is how long the guard retries cancel-by-ref after EOF without done; tests may shorten it.
var jobGuardRetryLimit = 30 * time.Minute

func newJobGuardCommand() *cobra.Command {
	command := &cobra.Command{
		Use:    jobGuardCommandName,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runJobGuard(os.NewFile(3, "guard-pipe"))
		},
	}
	return command
}

func runJobGuard(pipe *os.File) error {
	if pipe == nil {
		return nil
	}
	defer pipe.Close()
	ref := strings.TrimSpace(os.Getenv("STX_GUARD_REF"))
	server := strings.TrimSpace(os.Getenv("STX_SERVER"))
	token := strings.TrimSpace(os.Getenv("STX_TOKEN"))
	generation, done, err := readGuardPipe(pipe)
	if err != nil && !isEOF(err) {
		return err
	}
	if done || strings.TrimSpace(ref) == "" {
		return nil
	}
	return retryCancelByRef(server, token, ref, generation)
}

func readGuardPipe(reader io.Reader) (generation int, done bool, err error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == guardDoneLine:
			return generation, true, nil
		case strings.HasPrefix(line, guardGenerationPrefix):
			parsed, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, guardGenerationPrefix)))
			if parseErr == nil {
				generation = parsed
			}
		}
	}
	return generation, false, scanner.Err()
}

func retryCancelByRef(server, token, ref string, generation int) error {
	if strings.TrimSpace(server) == "" || strings.TrimSpace(token) == "" {
		return nil
	}
	client, err := cliClient.New(cliConfig.Resolved{Server: server, Token: token, Timeout: 15 * time.Second}, nil)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(jobGuardRetryLimit)
	backoff := 2 * time.Second
	for time.Now().Before(deadline) {
		headers := map[string]string{
			"Idempotency-Key": fmt.Sprintf("guard-%s-%d-%d", ref, generation, time.Now().UnixNano()),
			"X-STX-Confirm":   "true",
		}
		body := map[string]any{"external_ref": ref, "attach_generation": generation}
		var data any
		_, reqErr := client.RequestWithHeaders(context.Background(), "POST", "/api/v1/sync/jobs/cancel-by-ref", body, headers, &data)
		if reqErr == nil || isIgnorableGuardCancelError(reqErr) {
			return nil
		}
		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
	return nil
}

func writeGuardGeneration(pipe *os.File, generation int) {
	if pipe == nil {
		return
	}
	_, _ = fmt.Fprintf(pipe, "%s%d\n", guardGenerationPrefix, generation)
}

func writeGuardDone(pipe *os.File) {
	if pipe == nil {
		return
	}
	_, _ = fmt.Fprintln(pipe, guardDoneLine)
	_ = pipe.Close()
}

func isEOF(err error) bool {
	return err == io.EOF
}

func isIgnorableGuardCancelError(err error) bool {
	if err == nil {
		return true
	}
	var apiErr *cliClient.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusConflict, http.StatusNotFound:
			return true
		}
	}
	var cliErr *clioutput.CLIError
	if errors.As(err, &cliErr) {
		return cliErr.ExitCode == clioutput.ExitConflict || cliErr.ExitCode == clioutput.ExitNotFound
	}
	return false
}
