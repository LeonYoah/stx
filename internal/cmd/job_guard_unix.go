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
	env := append(os.Environ(),
		"STX_SERVER="+client.Server(),
		"STX_TOKEN="+client.Token(),
		"STX_GUARD_REF="+strings.TrimSpace(externalRef),
	)
	start := func(attr *syscall.SysProcAttr) error {
		cmd := exec.Command(os.Args[0], jobGuardCommandName)
		cmd.Env = env
		cmd.ExtraFiles = []*os.File{reader}
		cmd.SysProcAttr = attr
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return err
		}
		_ = cmd.Process.Release()
		return nil
	}
	// Prefer setsid (detach from Dolphin worker terminal). Some sandboxes reject it;
	// fall back so SHELL submit still proceeds (SIGKILL cancel becomes best-effort).
	if err := start(jobGuardSysProcAttr()); err != nil {
		if retryErr := start(&syscall.SysProcAttr{}); retryErr != nil {
			_ = reader.Close()
			_ = writer.Close()
			return nil, err
		}
	}
	_ = reader.Close()
	return writer, nil
}

// jobGuardSysProcAttr 只做 setsid。setsid 已经脱离控制终端并新建进程组；再 Setpgid 时 Start 会返回 operation not permitted，Dolphin 任务在提交前失败。
// jobGuardSysProcAttr only sets setsid. setsid already detaches the terminal and creates a process group; adding Setpgid makes Start return operation not permitted, so the Dolphin task fails before submit.
func jobGuardSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
