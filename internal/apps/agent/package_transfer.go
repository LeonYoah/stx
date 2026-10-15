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

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"github.com/google/uuid"
)

const (
	// PackageStreamChunkSize is the raw chunk size used by DownloadPackage (256KB).
	// PackageStreamChunkSize 是 DownloadPackage 使用的原始数据块大小（256KB）。
	PackageStreamChunkSize = 256 * 1024

	// DefaultPackageStallTimeout fails a transfer when no bytes are served for this long.
	// DefaultPackageStallTimeout 在该时长内没有任何数据被拉取时判定传输停滞。
	DefaultPackageStallTimeout = 2 * time.Minute

	// DefaultPackageTransferMaxDuration caps the whole transfer.
	// DefaultPackageTransferMaxDuration 是整个传输的最长时长。
	DefaultPackageTransferMaxDuration = 2 * time.Hour
)

var (
	// ErrPackageServeNotFound is returned when a transfer id is unknown or expired.
	// ErrPackageServeNotFound 表示传输标识不存在或已过期。
	ErrPackageServeNotFound = errors.New("package transfer not found or expired / 安装包传输不存在或已过期")

	// ErrPackageTransferStalled is returned when the Agent stops pulling data.
	// ErrPackageTransferStalled 表示 Agent 停止拉取数据。
	ErrPackageTransferStalled = errors.New("package transfer stalled / 安装包传输停滞")
)

// PackageServe describes one package file an Agent is allowed to pull.
// PackageServe 描述一个允许 Agent 拉取的安装包文件。
type PackageServe struct {
	ID       string
	AgentID  string
	Path     string
	Size     int64
	Checksum string

	served       atomic.Int64 // highest offset served / 已发送的最大偏移量
	lastActivity atomic.Int64 // unix nano of last activity / 最近活动时间
}

// Served returns the highest byte offset streamed so far.
// Served 返回目前已流式发送的最大字节偏移量。
func (p *PackageServe) Served() int64 { return p.served.Load() }

func (p *PackageServe) touch() { p.lastActivity.Store(time.Now().UnixNano()) }

// IdleFor returns how long the serve has been idle.
// IdleFor 返回自最近活动以来的空闲时长。
func (p *PackageServe) IdleFor() time.Duration {
	return time.Since(time.Unix(0, p.lastActivity.Load()))
}

// RegisterPackageServe allows agentID to pull localPath via DownloadPackage.
// RegisterPackageServe 允许 agentID 通过 DownloadPackage 拉取 localPath。
func (m *Manager) RegisterPackageServe(agentID, localPath, checksum string) (*PackageServe, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("package not found: %s / 安装包未找到: %s", localPath, localPath)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("package path is a directory: %s / 安装包路径是目录: %s", localPath, localPath)
	}
	ps := &PackageServe{
		ID:       uuid.New().String(),
		AgentID:  agentID,
		Path:     localPath,
		Size:     info.Size(),
		Checksum: checksum,
	}
	ps.touch()
	m.packageServes.Store(ps.ID, ps)
	return ps, nil
}

// ReleasePackageServe revokes a transfer id.
// ReleasePackageServe 撤销传输标识。
func (m *Manager) ReleasePackageServe(id string) { m.packageServes.Delete(id) }

// StreamPackage streams the registered package to the Agent starting at req.Offset.
// gRPC flow control provides back-pressure, so no per-chunk ack/timeout is needed here.
// StreamPackage 从 req.Offset 开始向 Agent 流式发送已注册的安装包；
// gRPC 自带流控提供背压，无需逐块确认。
func (m *Manager) StreamPackage(ctx context.Context, req *pb.DownloadPackageRequest, send func(*pb.PackageChunk) error) error {
	v, ok := m.packageServes.Load(req.GetTransferId())
	if !ok {
		return ErrPackageServeNotFound
	}
	ps := v.(*PackageServe)
	if ps.AgentID != "" && ps.AgentID != req.GetAgentId() {
		return ErrPackageServeNotFound
	}
	offset := req.GetOffset()
	if offset < 0 || offset > ps.Size {
		return fmt.Errorf("invalid offset %d (size %d) / 偏移量非法 %d（大小 %d）", offset, ps.Size, offset, ps.Size)
	}

	f, err := os.Open(ps.Path)
	if err != nil {
		return fmt.Errorf("failed to open package: %w / 打开安装包失败: %w", err, err)
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek package: %w / 定位安装包失败: %w", err, err)
	}

	ps.touch()
	buf := make([]byte, PackageStreamChunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			if err := send(&pb.PackageChunk{
				Data:      buf[:n],
				Offset:    offset,
				TotalSize: ps.Size,
				Checksum:  ps.Checksum,
			}); err != nil {
				return err
			}
			offset += int64(n)
			if offset > ps.served.Load() {
				ps.served.Store(offset)
			}
			ps.touch()
		}
		if rerr == io.EOF {
			if offset == 0 {
				// Empty file: still tell the agent the size/checksum.
				// 空文件：仍告知 Agent 大小与校验和。
				return send(&pb.PackageChunk{TotalSize: ps.Size, Checksum: ps.Checksum})
			}
			return nil
		}
		if rerr != nil {
			return fmt.Errorf("failed to read package: %w / 读取安装包失败: %w", rerr, rerr)
		}
	}
}

// PackagePullOptions tunes TransferPackage.
// PackagePullOptions 用于调整 TransferPackage 的行为。
type PackagePullOptions struct {
	StallTimeout time.Duration
	MaxDuration  time.Duration
	PollInterval time.Duration
	OnProgress   func(sent, total int64)
	// CachedRemotePath is where the Control Plane believes the Agent already holds this package;
	// the Agent reuses it only if size and SHA256 match.
	// CachedRemotePath 是 CP 认为 Agent 已持有该安装包的路径；仅当大小与 SHA256 一致时 Agent 才复用。
	CachedRemotePath string
}

// PackagePullResult is the outcome of TransferPackage.
// PackagePullResult 是 TransferPackage 的结果。
type PackagePullResult struct {
	LocalPath string // path on the Agent / Agent 上的路径
	Reused    bool   // Agent already had an identical file / Agent 已有相同文件，未重新下载
}

// TransferPackage asks the Agent to pull localPath over the dedicated DownloadPackage stream
// and waits until the Agent reports completion. Returns the package path on the Agent.
// TransferPackage 让 Agent 通过专用 DownloadPackage 流拉取 localPath，并等待 Agent 报告完成，
// 返回安装包在 Agent 上的路径。
func (m *Manager) TransferPackage(ctx context.Context, agentID, version, localPath, checksum string, opts PackagePullOptions) (PackagePullResult, error) {
	if opts.StallTimeout <= 0 {
		opts.StallTimeout = DefaultPackageStallTimeout
	}
	if opts.MaxDuration <= 0 {
		opts.MaxDuration = DefaultPackageTransferMaxDuration
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}

	ps, err := m.RegisterPackageServe(agentID, localPath, checksum)
	if err != nil {
		return PackagePullResult{}, err
	}
	defer m.ReleasePackageServe(ps.ID)

	params := map[string]string{
		"mode":        "pull",
		"transfer_id": ps.ID,
		"version":     version,
		"file_name":   filepath.Base(localPath),
		"total_size":  strconv.FormatInt(ps.Size, 10),
		"checksum":    checksum,
	}
	if opts.CachedRemotePath != "" {
		params["cached_path"] = opts.CachedRemotePath
	}
	commandID, err := m.SendCommandAsync(agentID, pb.CommandType_TRANSFER_PACKAGE, params, opts.MaxDuration)
	if err != nil {
		return PackagePullResult{}, err
	}
	defer m.commands.Delete(commandID)

	deadline := time.Now().Add(opts.MaxDuration)
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return PackagePullResult{}, ctx.Err()
		case <-ticker.C:
		}

		if opts.OnProgress != nil {
			opts.OnProgress(ps.Served(), ps.Size)
		}

		cmdCtx, ok := m.GetCommand(commandID)
		if !ok {
			return PackagePullResult{}, fmt.Errorf("transfer command lost / 传输命令丢失")
		}
		cmdCtx.mu.RLock()
		status, output, cmdErr := cmdCtx.LastStatus, cmdCtx.LastOutput, cmdCtx.LastError
		cmdCtx.mu.RUnlock()

		switch status {
		case pb.CommandStatus_SUCCESS:
			var out struct {
				Success   bool   `json:"success"`
				Message   string `json:"message"`
				LocalPath string `json:"local_path"`
				Reused    bool   `json:"reused"`
			}
			if err := json.Unmarshal([]byte(output), &out); err != nil || !out.Success || out.LocalPath == "" {
				return PackagePullResult{}, fmt.Errorf("invalid transfer result: %s / 传输结果无效: %s", output, output)
			}
			return PackagePullResult{LocalPath: out.LocalPath, Reused: out.Reused}, nil
		case pb.CommandStatus_FAILED, pb.CommandStatus_CANCELLED:
			if cmdErr == "" {
				cmdErr = output
			}
			return PackagePullResult{}, fmt.Errorf("agent failed to pull package: %s / Agent 拉取安装包失败: %s", cmdErr, cmdErr)
		}

		if ps.IdleFor() > opts.StallTimeout {
			return PackagePullResult{}, fmt.Errorf("%w: no data pulled for %s (served %d/%d bytes) / %s 内无数据拉取（已发送 %d/%d 字节）",
				ErrPackageTransferStalled, opts.StallTimeout, ps.Served(), ps.Size, opts.StallTimeout, ps.Served(), ps.Size)
		}
		if time.Now().After(deadline) {
			return PackagePullResult{}, ErrCommandTimeout
		}
	}
}
