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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	pb "github.com/LeonYoah/stx/agent"
)

// PackageTransferManager manages package file transfers from Control Plane
// PackageTransferManager 管理从 Control Plane 传输的安装包文件
type PackageTransferManager struct {
	// tempDir is the directory for storing temporary files during transfer
	// tempDir 是传输过程中存储临时文件的目录
	tempDir string

	// packageDir is the directory for storing completed packages
	// packageDir 是存储完成传输的安装包的目录
	packageDir string

	// activeTransfers tracks ongoing transfers by version
	// activeTransfers 按版本跟踪正在进行的传输
	activeTransfers map[string]*packageTransferState
	mu              sync.RWMutex
}

// packageTransferState tracks the state of an ongoing package transfer
// packageTransferState 跟踪正在进行的安装包传输状态
type packageTransferState struct {
	version       string
	fileName      string
	tempPath      string
	file          *os.File
	receivedBytes int64
	totalSize     int64
}

var (
	packageTransferMgr     *PackageTransferManager
	packageTransferMgrOnce sync.Once
)

// GetPackageTransferManager returns the singleton PackageTransferManager instance
// GetPackageTransferManager 返回单例 PackageTransferManager 实例
func GetPackageTransferManager() *PackageTransferManager {
	packageTransferMgrOnce.Do(func() {
		// Default directories / 默认目录
		tempDir := filepath.Join(os.TempDir(), "seatunnel-packages-temp")
		packageDir := filepath.Join(os.TempDir(), "seatunnel-packages")

		// Create directories / 创建目录
		os.MkdirAll(tempDir, 0755)
		os.MkdirAll(packageDir, 0755)

		packageTransferMgr = &PackageTransferManager{
			tempDir:         tempDir,
			packageDir:      packageDir,
			activeTransfers: make(map[string]*packageTransferState),
		}
	})
	return packageTransferMgr
}

// SetDirectories sets the directories for package transfer
// SetDirectories 设置安装包传输的目录
func (m *PackageTransferManager) SetDirectories(tempDir, packageDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tempDir != "" {
		m.tempDir = tempDir
		os.MkdirAll(tempDir, 0755)
	}
	if packageDir != "" {
		m.packageDir = packageDir
		os.MkdirAll(packageDir, 0755)
	}
}

// TransferPackageRequest represents a package transfer chunk request
// TransferPackageRequest 表示安装包传输块请求
type TransferPackageRequest struct {
	Version   string `json:"version"`
	FileName  string `json:"file_name"`
	Chunk     []byte `json:"chunk"` // Base64 encoded in JSON
	Offset    int64  `json:"offset"`
	TotalSize int64  `json:"total_size"`
	IsLast    bool   `json:"is_last"`
	Checksum  string `json:"checksum"` // SHA256, only on last chunk
}

// TransferPackageResponse represents the response for a package transfer chunk
// TransferPackageResponse 表示安装包传输块的响应
type TransferPackageResponse struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	ReceivedBytes int64  `json:"received_bytes"`
	LocalPath     string `json:"local_path,omitempty"` // Set when transfer completes
}

// HandleTransferPackageCommand handles the TRANSFER_PACKAGE command
// HandleTransferPackageCommand 处理 TRANSFER_PACKAGE 命令
func HandleTransferPackageCommand(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	// Parse request from parameters / 从参数解析请求
	req, err := parseTransferPackageRequest(cmd.Parameters)
	if err != nil {
		return &pb.CommandResponse{
			CommandId: cmd.CommandId,
			Status:    pb.CommandStatus_FAILED,
			Error:     fmt.Sprintf("Failed to parse request: %v / 解析请求失败: %v", err, err),
		}, nil
	}

	// Process the chunk / 处理数据块
	mgr := GetPackageTransferManager()
	resp, err := mgr.ReceiveChunk(ctx, req)
	if err != nil {
		return &pb.CommandResponse{
			CommandId: cmd.CommandId,
			Status:    pb.CommandStatus_FAILED,
			Error:     fmt.Sprintf("Failed to receive chunk: %v / 接收数据块失败: %v", err, err),
		}, nil
	}

	// Build response / 构建响应
	respJSON, _ := json.Marshal(resp)

	// Use RUNNING status for intermediate chunks, SUCCESS for last chunk
	// 中间块使用 RUNNING 状态，最后一块使用 SUCCESS 状态
	status := pb.CommandStatus_RUNNING
	if req.IsLast && resp.Success {
		status = pb.CommandStatus_SUCCESS
	}

	progress := int32(0)
	if req.TotalSize > 0 {
		progress = int32(float64(resp.ReceivedBytes) / float64(req.TotalSize) * 100)
	}

	// Report progress / 上报进度
	if reporter != nil {
		reporter.Report(progress, resp.Message)
	}

	return &pb.CommandResponse{
		CommandId: cmd.CommandId,
		Status:    status,
		Progress:  progress,
		Output:    string(respJSON),
	}, nil
}

// parseTransferPackageRequest parses the transfer package request from command parameters
// parseTransferPackageRequest 从命令参数解析传输安装包请求
func parseTransferPackageRequest(params map[string]string) (*TransferPackageRequest, error) {
	req := &TransferPackageRequest{}

	// Required fields / 必需字段
	req.Version = params["version"]
	if req.Version == "" {
		return nil, fmt.Errorf("version is required / version 是必需的")
	}

	req.FileName = params["file_name"]
	if req.FileName == "" {
		req.FileName = fmt.Sprintf("apache-seatunnel-%s-bin.tar.gz", req.Version)
	}

	// Parse chunk data (base64 encoded) / 解析数据块（base64 编码）
	chunkStr := params["chunk"]
	if chunkStr != "" {
		chunk, err := base64.StdEncoding.DecodeString(chunkStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode chunk: %w / 解码数据块失败: %w", err, err)
		}
		req.Chunk = chunk
	}

	// Parse numeric fields / 解析数字字段
	if offsetStr := params["offset"]; offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &req.Offset)
	}

	if totalSizeStr := params["total_size"]; totalSizeStr != "" {
		fmt.Sscanf(totalSizeStr, "%d", &req.TotalSize)
	}

	// Parse boolean / 解析布尔值
	req.IsLast = params["is_last"] == "true"

	// Checksum (only on last chunk) / 校验和（仅最后一块）
	req.Checksum = params["checksum"]

	return req, nil
}

// ReceiveChunk receives a chunk of package data
// ReceiveChunk 接收一块安装包数据
func (m *PackageTransferManager) ReceiveChunk(ctx context.Context, req *TransferPackageRequest) (*TransferPackageResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get or create transfer state / 获取或创建传输状态
	state, exists := m.activeTransfers[req.Version]
	if !exists {
		// First chunk - create new state / 第一块 - 创建新状态
		tempPath := filepath.Join(m.tempDir, req.FileName+".tmp")
		file, err := os.Create(tempPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w / 创建临时文件失败: %w", err, err)
		}

		state = &packageTransferState{
			version:   req.Version,
			fileName:  req.FileName,
			tempPath:  tempPath,
			file:      file,
			totalSize: req.TotalSize,
		}
		m.activeTransfers[req.Version] = state
	}

	// Verify offset / 验证偏移量
	if req.Offset != state.receivedBytes {
		return &TransferPackageResponse{
			Success:       false,
			Message:       fmt.Sprintf("Offset mismatch: expected %d, got %d / 偏移量不匹配：期望 %d，实际 %d", state.receivedBytes, req.Offset, state.receivedBytes, req.Offset),
			ReceivedBytes: state.receivedBytes,
		}, nil
	}

	// Write chunk / 写入数据块
	if len(req.Chunk) > 0 {
		n, err := state.file.Write(req.Chunk)
		if err != nil {
			return nil, fmt.Errorf("failed to write chunk: %w / 写入数据块失败: %w", err, err)
		}
		state.receivedBytes += int64(n)
	}

	// Handle last chunk / 处理最后一块
	if req.IsLast {
		// Close file / 关闭文件
		state.file.Close()

		// Verify checksum if provided / 如果提供了校验和则验证
		if req.Checksum != "" {
			actualChecksum, err := calculateFileChecksum(state.tempPath)
			if err != nil {
				m.cleanupTransfer(req.Version)
				return nil, fmt.Errorf("failed to calculate checksum: %w / 计算校验和失败: %w", err, err)
			}
			if actualChecksum != req.Checksum {
				m.cleanupTransfer(req.Version)
				return &TransferPackageResponse{
					Success:       false,
					Message:       fmt.Sprintf("Checksum mismatch: expected %s, got %s / 校验和不匹配：期望 %s，实际 %s", req.Checksum, actualChecksum, req.Checksum, actualChecksum),
					ReceivedBytes: state.receivedBytes,
				}, nil
			}
		}

		// Move to final location / 移动到最终位置
		finalPath := filepath.Join(m.packageDir, req.FileName)
		if err := os.Rename(state.tempPath, finalPath); err != nil {
			// Try copy if rename fails (cross-device) / 如果重命名失败则尝试复制（跨设备）
			if err := copyFile(state.tempPath, finalPath); err != nil {
				m.cleanupTransfer(req.Version)
				return nil, fmt.Errorf("failed to move package: %w / 移动安装包失败: %w", err, err)
			}
			os.Remove(state.tempPath)
		}

		// Cleanup state / 清理状态
		delete(m.activeTransfers, req.Version)

		return &TransferPackageResponse{
			Success:       true,
			Message:       "Package transfer completed / 安装包传输完成",
			ReceivedBytes: state.receivedBytes,
			LocalPath:     finalPath,
		}, nil
	}

	return &TransferPackageResponse{
		Success:       true,
		Message:       fmt.Sprintf("Chunk received: %d/%d bytes / 数据块已接收：%d/%d 字节", state.receivedBytes, state.totalSize, state.receivedBytes, state.totalSize),
		ReceivedBytes: state.receivedBytes,
	}, nil
}

// cleanupTransfer cleans up a failed transfer
// cleanupTransfer 清理失败的传输
func (m *PackageTransferManager) cleanupTransfer(version string) {
	if state, exists := m.activeTransfers[version]; exists {
		if state.file != nil {
			state.file.Close()
		}
		os.Remove(state.tempPath)
		delete(m.activeTransfers, version)
	}
}

// GetPackagePath returns the path of a transferred package
// GetPackagePath 返回已传输安装包的路径
func (m *PackageTransferManager) GetPackagePath(version string) string {
	fileName := fmt.Sprintf("apache-seatunnel-%s-bin.tar.gz", version)
	return filepath.Join(m.packageDir, fileName)
}

// HasPackage checks if a package exists locally
// HasPackage 检查安装包是否存在于本地
func (m *PackageTransferManager) HasPackage(version string) bool {
	path := m.GetPackagePath(version)
	_, err := os.Stat(path)
	return err == nil
}

// calculateFileChecksum calculates SHA256 checksum of a file
// calculateFileChecksum 计算文件的 SHA256 校验和
func calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// copyFile copies a file from src to dst
// copyFile 将文件从 src 复制到 dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// RegisterPackageHandlers registers package transfer handlers with the executor
// RegisterPackageHandlers 向执行器注册安装包传输处理器
func RegisterPackageHandlers(executor *CommandExecutor) {
	executor.RegisterHandler(pb.CommandType_TRANSFER_PACKAGE, HandleTransferPackageCommand)
}
