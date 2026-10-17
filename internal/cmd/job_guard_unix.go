//go:build unix

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
	"os"
	"os/exec"
	"strings"
	"syscall"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
)

func startJobGuard(client *cliClient.Client, externalRef string) (*os.File, error) {
	if client == nil || strings.TrimSpace(externalRef) == "" {
		return nil, nil
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(os.Args[0], jobGuardCommandName)
	cmd.Env = append(os.Environ(),
		"STX_SERVER="+client.Server(),
		"STX_TOKEN="+client.Token(),
		"STX_GUARD_REF="+strings.TrimSpace(externalRef),
	)
	cmd.ExtraFiles = []*os.File{reader}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setpgid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, err
	}
	_ = reader.Close()
	_ = cmd.Process.Release()
	return writer, nil
}
