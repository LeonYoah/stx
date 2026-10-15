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

package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PackageChunkStream is the receiving side of the DownloadPackage server stream.
// PackageChunkStream 是 DownloadPackage 服务端流的接收端。
type PackageChunkStream interface {
	Recv() (*pb.PackageChunk, error)
}

// PackageStreamOpener opens a DownloadPackage stream starting at offset.
// PackageStreamOpener 从指定偏移量打开 DownloadPackage 流。
type PackageStreamOpener func(ctx context.Context, transferID string, offset int64) (PackageChunkStream, error)

// PullOptions tunes Pull retry/idle behaviour.
// PullOptions 调整 Pull 的重试与空闲超时行为。
type PullOptions struct {
	// IdleTimeout aborts the current stream when no chunk arrives for this long (per-chunk timeout).
	// IdleTimeout：在该时长内未收到任何数据块则中止当前流（逐块超时）。
	IdleTimeout time.Duration
	// MaxAttempts is the number of consecutive attempts without progress before giving up.
	// MaxAttempts：连续无进展的最大尝试次数。
	MaxAttempts int
	// Backoff is the initial retry backoff (doubled each failed attempt).
	// Backoff：初始重试退避时长（每次失败翻倍）。
	Backoff time.Duration
}

// PullRequest describes a package to pull from the Control Plane.
// PullRequest 描述要从 Control Plane 拉取的安装包。
type PullRequest struct {
	TransferID string
	Version    string
	FileName   string
	TotalSize  int64
	Checksum   string
}

var defaultPullOptions = PullOptions{IdleTimeout: 60 * time.Second, MaxAttempts: 5, Backoff: time.Second}

var (
	streamOpenerMu sync.RWMutex
	streamOpener   PackageStreamOpener
)

// SetPackageStreamOpener installs the function used to open DownloadPackage streams.
// SetPackageStreamOpener 设置用于打开 DownloadPackage 流的函数。
func SetPackageStreamOpener(o PackageStreamOpener) {
	streamOpenerMu.Lock()
	defer streamOpenerMu.Unlock()
	streamOpener = o
}

func getPackageStreamOpener() PackageStreamOpener {
	streamOpenerMu.RLock()
	defer streamOpenerMu.RUnlock()
	return streamOpener
}

// isRetryablePullError reports whether a stream error is worth retrying.
// isRetryablePullError 判断流错误是否值得重试。
func isRetryablePullError(err error) bool {
	if err == nil {
		return true
	}
	switch status.Code(err) {
	case codes.NotFound, codes.InvalidArgument, codes.PermissionDenied, codes.Unauthenticated, codes.Unimplemented:
		return false
	}
	return true
}

// Pull downloads a package over the DownloadPackage stream with per-chunk idle timeout,
// resume-from-offset retries and streaming SHA256 verification, then moves it into packageDir.
// Pull 通过 DownloadPackage 流下载安装包：逐块空闲超时、按偏移量断点续传重试、流式 SHA256 校验，
// 完成后移动到 packageDir。
func (m *PackageTransferManager) Pull(ctx context.Context, open PackageStreamOpener, req PullRequest, opts PullOptions, progress func(received, total int64)) (string, error) {
	if open == nil {
		return "", errors.New("package stream not available / 安装包下载流不可用")
	}
	if req.TransferID == "" || req.FileName == "" {
		return "", errors.New("transfer_id and file_name are required / transfer_id 和 file_name 为必填")
	}
	if filepath.Base(req.FileName) != req.FileName {
		return "", fmt.Errorf("invalid file name: %s / 文件名非法: %s", req.FileName, req.FileName)
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = defaultPullOptions.IdleTimeout
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaultPullOptions.MaxAttempts
	}
	if opts.Backoff <= 0 {
		opts.Backoff = defaultPullOptions.Backoff
	}

	m.mu.RLock()
	tempDir, packageDir := m.tempDir, m.packageDir
	m.mu.RUnlock()
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w / 创建临时目录失败: %w", err, err)
	}
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create package dir: %w / 创建安装包目录失败: %w", err, err)
	}

	tempPath := filepath.Join(tempDir, req.FileName+"."+req.TransferID+".part")
	file, err := os.Create(tempPath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w / 创建临时文件失败: %w", err, err)
	}
	ok := false
	defer func() {
		file.Close()
		if !ok {
			os.Remove(tempPath)
		}
	}()

	hasher := sha256.New()
	var received int64
	total := req.TotalSize
	failures := 0
	backoff := opts.Backoff
	var lastErr error

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		before := received
		done, err := m.pullOnce(ctx, open, req.TransferID, opts.IdleTimeout, file, hasher, &received, &total, progress)
		if done {
			break
		}
		lastErr = err
		if !isRetryablePullError(err) {
			return "", fmt.Errorf("package pull failed: %w / 安装包拉取失败: %w", err, err)
		}
		if received > before {
			// Progress made: reset retry budget. / 有进展：重置重试预算。
			failures = 0
			backoff = opts.Backoff
		} else {
			failures++
		}
		if failures >= opts.MaxAttempts {
			return "", fmt.Errorf("package pull failed after %d attempts at offset %d/%d: %v / 安装包拉取在偏移量 %d/%d 处重试 %d 次后失败: %v",
				failures, received, total, lastErr, received, total, failures, lastErr)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}

	if total > 0 && received != total {
		return "", fmt.Errorf("size mismatch: expected %d, got %d / 大小不匹配：期望 %d，实际 %d", total, received, total, received)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if req.Checksum != "" && actual != req.Checksum {
		return "", fmt.Errorf("checksum mismatch: expected %s, got %s / 校验和不匹配：期望 %s，实际 %s", req.Checksum, actual, req.Checksum, actual)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("failed to sync package: %w / 同步安装包失败: %w", err, err)
	}
	file.Close()

	finalPath := filepath.Join(packageDir, req.FileName)
	if err := os.Rename(tempPath, finalPath); err != nil {
		if err := copyFile(tempPath, finalPath); err != nil {
			return "", fmt.Errorf("failed to move package: %w / 移动安装包失败: %w", err, err)
		}
		os.Remove(tempPath)
	}
	ok = true
	return finalPath, nil
}

// pullOnce runs one stream attempt from *received. It returns done=true when the stream ended
// cleanly with all bytes received.
// pullOnce 从 *received 开始执行一次流式拉取；流正常结束且字节完整时返回 done=true。
func (m *PackageTransferManager) pullOnce(ctx context.Context, open PackageStreamOpener, transferID string, idle time.Duration,
	file *os.File, hasher io.Writer, received, total *int64, progress func(int64, int64)) (bool, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var idleFired atomic.Bool
	timer := time.AfterFunc(idle, func() { idleFired.Store(true); cancel() })
	defer timer.Stop()

	stream, err := open(streamCtx, transferID, *received)
	if err != nil {
		return false, err
	}
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			if *total > 0 && *received < *total {
				return false, fmt.Errorf("stream ended early at %d/%d / 流提前结束于 %d/%d", *received, *total, *received, *total)
			}
			return true, nil
		}
		if err != nil {
			if idleFired.Load() {
				return false, fmt.Errorf("no data for %s, stream aborted / %s 内未收到数据，已中止流", idle, idle)
			}
			return false, err
		}
		timer.Reset(idle)
		if chunk.GetTotalSize() > 0 {
			*total = chunk.GetTotalSize()
		}
		if len(chunk.GetData()) == 0 {
			continue
		}
		if chunk.GetOffset() != *received {
			return false, fmt.Errorf("offset mismatch: expected %d, got %d / 偏移量不匹配：期望 %d，实际 %d", *received, chunk.GetOffset(), *received, chunk.GetOffset())
		}
		if _, err := file.Write(chunk.GetData()); err != nil {
			return false, status.Errorf(codes.InvalidArgument, "failed to write package: %v / 写入安装包失败: %v", err, err)
		}
		hasher.Write(chunk.GetData())
		*received += int64(len(chunk.GetData()))
		if progress != nil {
			progress(*received, *total)
		}
	}
}

// handlePullTransfer serves TRANSFER_PACKAGE in pull mode.
// handlePullTransfer 处理 pull 模式的 TRANSFER_PACKAGE 命令。
func handlePullTransfer(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	p := cmd.Parameters
	total, _ := strconv.ParseInt(p["total_size"], 10, 64)
	req := PullRequest{
		TransferID: p["transfer_id"],
		Version:    p["version"],
		FileName:   p["file_name"],
		TotalSize:  total,
		Checksum:   p["checksum"],
	}
	if req.FileName == "" && req.Version != "" {
		req.FileName = fmt.Sprintf("apache-seatunnel-%s-bin.tar.gz", req.Version)
	}

	lastPct := int32(-1)
	progress := func(received, total int64) {
		if reporter == nil || total <= 0 {
			return
		}
		pct := int32(float64(received) / float64(total) * 100)
		if pct == lastPct {
			return
		}
		lastPct = pct
		reporter.Report(pct, fmt.Sprintf("Package received: %d/%d bytes / 安装包已接收：%d/%d 字节", received, total, received, total))
	}

	path, err := GetPackageTransferManager().Pull(ctx, getPackageStreamOpener(), req, defaultPullOptions, progress)
	if err != nil {
		return &pb.CommandResponse{
			CommandId: cmd.CommandId,
			Status:    pb.CommandStatus_FAILED,
			Error:     err.Error(),
		}, nil
	}
	out, _ := json.Marshal(&TransferPackageResponse{
		Success:       true,
		Message:       "Package transfer completed / 安装包传输完成",
		ReceivedBytes: req.TotalSize,
		LocalPath:     path,
	})
	return &pb.CommandResponse{
		CommandId: cmd.CommandId,
		Status:    pb.CommandStatus_SUCCESS,
		Progress:  100,
		Output:    string(out),
	}, nil
}
