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

package layoutheal

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultReapWindow 是收尾窗口：新二进制启动后这段时间内持续清理旧三件套。
	// DefaultReapWindow is the reap window: the new binary keeps sweeping the legacy trio for this long after startup.
	DefaultReapWindow = 3 * time.Minute
	// DefaultReapInterval 是轮询间隔。/ DefaultReapInterval is the sweep interval.
	DefaultReapInterval = 5 * time.Second
	// termGrace 是 SIGTERM 后等待退出的最长时间。/ termGrace is the max wait after SIGTERM before SIGKILL.
	termGrace = 10 * time.Second
)

// legacyProc 描述一个旧三件套进程：相对安装目录的 pid 文件与允许的进程名。
// grafana 的 pid 文件可能由 grafana-server 持有，两个名字都认。
// legacyProc describes one legacy trio process: pid file (relative to install dir) and the
// allowed process names. The grafana pid file may be owned by grafana-server, so both names pass.
type legacyProc struct {
	pidFile string
	names   []string
}

var legacyProcs = []legacyProc{
	{pidFile: "observability/prometheus/prometheus.pid", names: []string{"prometheus"}},
	{pidFile: "observability/alertmanager/alertmanager.pid", names: []string{"alertmanager"}},
	{pidFile: "observability/grafana/grafana.pid", names: []string{"grafana", "grafana-server"}},
}

// 以下为可注入的进程操作，测试用 fake /proc 与信号 spy 验证「pid 复用不发信号」。
// The process operations below are injectable so tests can fake /proc and spy on signals
// to prove a reused pid is never signaled.
var (
	procNameOf   = procNameFromProc
	processAlive = func(pid int) bool {
		return syscall.Kill(pid, 0) == nil
	}
	signalProc = func(pid int, sig syscall.Signal) error {
		return syscall.Kill(pid, sig)
	}
	reapTermGrace = termGrace
	reapLogf      = func(level, format string, args ...interface{}) {
		log.Printf("[LayoutHeal] "+level+" "+format+"\n", args...)
	}
)

// ReapLegacyObservability 在启动窗口内按 pid 文件停掉旧外部监控三件套。
//
// 旧升级器不替换 bin/，机器重启或旧服务 restart 后旧 start.sh 仍会把三件套拉起来；
// 新二进制启动后先修布局，再在窗口内轮询 pid 文件逐个收尾：读 pid 后用 /proc/<pid>/exe
// （退化到 /proc/<pid>/cmdline）核对进程名；名不符时绝不发信号，也不删 pid 文件——
// 旧脚本 setsid X & 在 fork 到 exec 之间 exe 会短暂是 bash/setsid，下一轮 exec 完成后
// 名字才能对上，所以名字不符一律留到下一轮重核，直到窗口结束（pid 真被复用时最多
// 多等一个窗口，代价是保留一个无效 pid 文件，不会误伤进程）。名符合先 SIGTERM，
// 等待最多 termGrace 后 SIGKILL，再删 pid 文件。observability/ 数据目录保留。
//
// ReapLegacyObservability stops the legacy monitoring trio by pid file within a startup window.
//
// The old upgrader never replaces bin/, so after a reboot or an old-unit restart the legacy
// start.sh keeps the trio running; the new binary heals the layout first, then sweeps the pid
// files: verify the process name via /proc/<pid>/exe (falling back to /proc/<pid>/cmdline) and
// never signal on a mismatch — the legacy `setsid X &` keeps bash/setsid as exe for the brief
// fork->exec window, so a mismatching name is retried on the next sweep until the window ends
// (a genuinely reused pid costs one extra window and a stale pid file, never a wrong signal).
// On a match send SIGTERM, wait up to termGrace, then SIGKILL and remove the pid file.
// The observability/ data directory is kept.
func ReapLegacyObservability(ctx context.Context, installDir string, window, interval time.Duration) {
	if window <= 0 {
		return
	}
	if interval <= 0 {
		interval = DefaultReapInterval
	}
	done := make([]bool, len(legacyProcs))
	deadline := time.Now().Add(window)
	for {
		allDone := true
		for i, proc := range legacyProcs {
			if done[i] {
				continue
			}
			done[i] = reapOnce(installDir, proc)
			allDone = allDone && done[i]
		}
		// 全部处理完就提前退出；没处理完但窗口到期也退出。
		// Exit early when every pid file is handled; also stop once the window elapsed —
		// a missing pid file keeps polling because the trio may still be starting.
		if allDone || time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// reapOnce 处理一个 pid 文件；返回 true 表示该项收尾结束（已杀、进程不存在或 pid 文件非法已清），
// false 表示尚未完成、下一轮继续（pid 文件缺失、进程身份读不出或进程名暂不符）。
// reapOnce handles one pid file; true means this leg is finished (killed, no process, or an
// invalid pid file removed), false means keep polling (pid file absent, identity unreadable,
// or the name does not match yet).
func reapOnce(installDir string, proc legacyProc) bool {
	pidPath := filepath.Join(installDir, proc.pidFile)
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		removePidFile(pidPath, "非法内容")
		return true
	}
	if !processAlive(pid) {
		removePidFile(pidPath, "进程已退出")
		return true
	}
	name, err := procNameOf(pid)
	if err != nil {
		// 读不到身份宁可等下一轮，也不误发信号。
		// Fail-safe: when the identity cannot be read, wait for the next sweep instead of signaling.
		reapLogf("WARN", "failed to read process identity for %s (pid=%d), retrying: %v", proc.pidFile, pid, err)
		return false
	}
	if !nameAllowed(name, proc.names) {
		// 名不符不发信号、不删文件、不标记完成：可能只是 setsid fork→exec 窗口里 exe
		// 仍是 bash，下一轮 exec 完就能对上；pid 真被复用时也只是多等一个窗口。
		// On a name mismatch: no signal, no pid-file removal, not done — the setsid
		// fork->exec window may still show bash as exe and the next sweep can match;
		// a truly reused pid only costs the remaining window.
		reapLogf("WARN", "%s 里的 pid=%d 实际是 %s（可能 pid 复用或进程尚未 exec），不发信号、下轮重核 / %s holds pid=%d which is %s (pid reuse or pre-exec); not signaling, rechecking next sweep",
			proc.pidFile, pid, name, proc.pidFile, pid, name)
		return false
	}

	if err := signalProc(pid, syscall.SIGTERM); err != nil {
		reapLogf("WARN", "SIGTERM %s(pid=%d) 失败 / SIGTERM %s (pid=%d) failed: %v",
			proc.pidFile, pid, proc.pidFile, pid, err)
		removePidFile(pidPath, "发信号失败")
		return true
	}
	graceDeadline := time.Now().Add(reapTermGrace)
	for time.Now().Before(graceDeadline) {
		if !processAlive(pid) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if processAlive(pid) {
		if err := signalProc(pid, syscall.SIGKILL); err != nil {
			reapLogf("WARN", "SIGKILL %s(pid=%d) 失败 / SIGKILL %s (pid=%d) failed: %v",
				proc.pidFile, pid, proc.pidFile, pid, err)
		}
	}
	removePidFile(pidPath, "已停止")
	reapLogf("INFO", "已停止旧监控进程 %s(pid=%d, name=%s) / stopped legacy monitoring process %s (pid=%d, name=%s)",
		proc.pidFile, pid, name, proc.pidFile, pid, name)
	return true
}

func nameAllowed(name string, allowed []string) bool {
	for _, n := range allowed {
		if name == n {
			return true
		}
	}
	return false
}

// procNameFromProc 读取 /proc 里的进程名：优先 /proc/<pid>/exe 软链 basename，失败退回 cmdline argv[0]。
// procNameFromProc reads the process name from /proc: the /proc/<pid>/exe symlink basename first,
// then argv[0] of /proc/<pid>/cmdline as a fallback.
func procNameFromProc(pid int) (string, error) {
	if exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe")); err == nil && exe != "" {
		return filepath.Base(exe), nil
	}
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return "", err
	}
	argv0 := strings.SplitN(string(raw), "\x00", 2)[0]
	if argv0 == "" {
		return "", os.ErrNotExist
	}
	return filepath.Base(argv0), nil
}

func removePidFile(path, reason string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		reapLogf("WARN", "删除 pid 文件 %s 失败（%s）/ failed to remove pid file %s (%s): %v", path, reason, path, reason, err)
	}
}
